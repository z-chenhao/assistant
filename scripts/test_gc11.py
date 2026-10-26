"""GC-11 frozen scope, strict native bytes and ungraded preview contracts."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from case_runner import variants
import prepare_release_eval
import gc11_driver
import verify_gc11 as journey
import verify_suite


class QuoteBoundaryTests(unittest.TestCase):
    @staticmethod
    def build_binary(root):
        # PATH's go may be the maintained CI SDK shell wrapper. Inspect a
        # compiled fixture, never assume the compiler command is a Go binary.
        source = root/'main.go'
        source.write_text('package main\nfunc main() {}\n')
        binary = root/'erii'
        subprocess.run(['go','build','-o',str(binary),str(source)],check=True,capture_output=True)
        return binary

    @staticmethod
    def native_state(prefix):
        return {table:[{key:prefix+'/'+key}] for table,key in
            (('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),
             ('deliveries','delivery_id'),('conversation','message_id'),('artifact','artifact_ref'))}

    def test_native_executions_are_disjoint_but_restart_and_replay_reuse_ids(self):
        rows = [dict(journey=dict(state=self.native_state(str(n)))) for n in range(30)]
        for row in rows:
            row.update(restarts=[dict(state=copy.deepcopy(row['journey']['state']))],replay_state=copy.deepcopy(row['journey']['state']))
        journey.independent_executions(rows)
        for table in rows[0]['journey']['state']:
            for other in (1,5,29):
                with self.subTest(table=table,other=other):
                    bad = copy.deepcopy(rows)
                    bad[other]['journey']['state'][table] = copy.deepcopy(bad[0]['journey']['state'][table])
                    with self.assertRaisesRegex(RuntimeError,'reused GC-11 native execution identity: '+table):
                        journey.independent_executions(bad)
            for value in (None,True,0,''):
                bad = copy.deepcopy(rows)
                key = next(iter(bad[0]['journey']['state'][table][0]))
                bad[0]['journey']['state'][table][0][key] = value
                with self.assertRaisesRegex(RuntimeError,'invalid GC-11 native execution identity: '+table):
                    journey.independent_executions(bad)
            bad = copy.deepcopy(rows)
            bad[0]['journey']['state'][table] *= 2
            with self.assertRaisesRegex(RuntimeError,'reused GC-11 native execution identity: '+table):
                journey.independent_executions(bad)
            bad[0]['journey']['state'][table] = []
            with self.assertRaisesRegex(RuntimeError,'invalid GC-11 native execution identity: '+table):
                journey.independent_executions(bad)

    def test_original_23_blocks_and_variants_remain_locked(self):
        lock = journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [299,301,303,304,305,306,307,309,803,805,806,807,808,809,811,813,815,816,818,820,821,822,823])
        rows = list(variants(*journey.inputs()))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['events'][0]['text'] for r in rows}),3)
        self.assertEqual(sum(r['noise_characters'] > 0 for r in rows),2)
        for row in rows:
            prompts = gc11_driver.prompts(row)
            self.assertEqual(prompts,[row['context']+'\n'+row['events'][0]['text'],row['events'][1]['text']])
            self.assertNotIn('Capabilities:',json.dumps(prompts))
        self.assertEqual(journey.REPEATS,5)
        self.assertEqual(verify_suite.COMMUNICATION_PARTS,('gc01','gc02','gc09','gc10','gc13','channels'))
        self.assertEqual([g.name for g in verify_suite.communication_gates('channels')],
            ['gc12','gc08','gc14','gc16','gc11'])
        planned = [g for part in verify_suite.COMMUNICATION_PARTS for g in verify_suite.communication_gates(part)]
        self.assertCountEqual(planned,verify_suite.SHARDS['communication'])
        self.assertEqual(len(planned),10)
        self.assertEqual(sum(g.attempts for g in planned),14)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script == 'verify_gc11.py'],[('communication',1)])

    def test_native_manifest_rejects_summary_size_hash_path_and_symlink_forgery(self):
        report = dict(status='mechanical_pass',cleanup=True,results=[{'native':'RAW_ONLY'*200000}],
            identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/'gc11.json'; manifest = journey.retain_report(path,report)
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('RAW_ONLY',path.read_text())
            raw = path.parent/manifest['native']['path']
            self.assertGreater(raw.stat().st_size,1<<20)
            self.assertEqual(journey.read_report(path),report)
            mutations = [lambda m:m['summary'].update(receipts=30),lambda m:m['summary'].update(semantic_score=100),
                lambda m:m['summary'].update(cleanup=1),lambda m:m['native'].update(sha256='0'*64),
                lambda m:m['native'].update(path='../outside'),lambda m:m['native'].update(path=str(raw)),
                lambda m:m.update(native_bytes=True),lambda m:m.update(native_bytes=0),lambda m:m.update(version=True)]
            for mutate in mutations:
                changed = copy.deepcopy(manifest); mutate(changed); path.write_text(json.dumps(changed))
                with self.assertRaises((RuntimeError,ValueError)): journey.read_report(path)
            path.write_text(json.dumps(manifest))
            saved = raw.with_suffix('.saved'); raw.rename(saved); raw.symlink_to(saved)
            with self.assertRaises(ValueError): journey.read_report(path)
            raw.unlink(); saved.rename(raw); raw.write_text(raw.read_text()+' ')
            with self.assertRaises(ValueError): journey.read_report(path)

    def test_incomplete_or_graded_report_cannot_become_preview(self):
        for report in (dict(case_id='GC-11',kind='gc11_boundary'),dict(case_id='GC-11',kind='gc11_boundary',status='mechanical_pass',semantic_score=100)):
            with self.assertRaises(RuntimeError): prepare_release_eval.prepare(report)
        with patch.object(journey,'validate_report',return_value=dict(receipts=30)) as validate:
            result = prepare_release_eval.prepare(dict(case_id='GC-11',kind='gc11_boundary'))
        validate.assert_called_once()
        self.assertEqual(result['judge_packets'],[])
        self.assertIs(result['eligible_for_grading'],False)
        self.assertIsNone(result['scores'])
        self.assertIsNone(result['winner'])

    def test_failed_launch_removes_stale_success_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/'gc11.json'; path.write_text('{"status":"mechanical_pass"}')
            failed = subprocess.run([sys.executable,str(journey.ROOT/'scripts/verify_gc11.py'),
                '--binary',str(Path(tmp)/'missing'),'--evidence',str(path)],capture_output=True,text=True)
            self.assertNotEqual(failed.returncode,0)
            self.assertFalse(path.exists())

    def test_copied_id_probe_reaches_real_cli_and_ci_rejection(self):
        # Identity-only synthetic rows exercise the negative consumer path.
        # They are never accepted as successful mechanical observations.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = self.build_binary(root)
            identity = journey.source_identity()
            report = dict(case_id='GC-11',kind='gc11_boundary',status='mechanical_pass',
                fixture_lock=journey.load(),semantic_score=None,eligible_for_grading=False,
                release_ready=False,cleanup=True,identity=dict(source_before=identity,source_after=identity,
                    binary_path=str(binary),binary_sha256=journey.sha(binary.read_bytes()),environment=journey.environment(binary)),
                results=[dict(variant_id=v['variant_id'],repetition=n,
                    journey=dict(state=self.native_state(v['variant_id']+'/'+str(n))))
                    for v in variants(*journey.inputs()) for n in range(1,6)])
            result = journey.repetition_rejection_probes(report,binary,root/'forged')
            self.assertEqual(result['claimed_journeys'],30)
            self.assertEqual(result['distinct_journeys'],6)
            self.assertTrue(result['cli_rejected'])
            self.assertTrue(result['ci_rejected'])
            self.assertNotEqual(result['cli_exit_code'],0)

    def test_resealed_environment_forgery_reaches_real_cli_and_ci_rejection(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self.build_binary(Path(tmp))
            source = journey.source_identity()
            report = dict(case_id='GC-11',kind='gc11_boundary',status='mechanical_pass',
                fixture_lock=journey.load(),semantic_score=None,eligible_for_grading=False,
                release_ready=False,cleanup=True,results=[],identity=dict(source_before=source,source_after=source,
                    binary_path=str(binary),binary_sha256=journey.sha(binary.read_bytes()),environment=journey.environment(binary)))
            results = journey.environment_rejection_probes(report,binary,Path(tmp))
            self.assertEqual(len(results),17)
            self.assertTrue(all(r['cli_rejected'] and r['ci_rejected'] for r in results.values()))

    def test_environment_normalizes_only_binary_path_and_preserves_build_version(self):
        real_output = subprocess.check_output
        def output(args,**kwargs):
            if args[:3] == ['go','version','-m']:
                return args[3]+': go1.26.2\n\tbuild\tvcs.revision=exact\n'
            if args in (['go','version'],['node','--version']):
                return 'runtime version\n'
            return real_output(args,**kwargs)
        with patch.object(journey.subprocess,'check_output',side_effect=output):
            original = journey.environment(Path('/original/erii'))
            relocated = journey.environment(Path('/relocated/erii'))
        self.assertEqual(original,relocated)
        self.assertEqual(original['binary_build'],'go1.26.2\n\tbuild\tvcs.revision=exact\n')

    def test_validator_requires_complete_inventory_identity_cleanup_and_recomputation(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = Path(tmp)/'erii'; binary.write_bytes(b'test identity only')
            identity = journey.source_identity()
            report = dict(case_id='GC-11',status='mechanical_pass',fixture_lock=journey.load(),semantic_score=None,
                eligible_for_grading=False,release_ready=False,cleanup=True,rejection_probes={'native':True},
                identity=dict(source_before=identity,source_after=identity,binary_sha256=journey.sha(binary.read_bytes()),environment={'test':True}),
                kind='gc11_boundary',results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'native':True},
                    journey=dict(state=self.native_state(v['variant_id']+'/'+str(n))),
                    cleanup=dict(disposable_root_removed=True,owned_processes_joined=True))
                    for v in variants(*journey.inputs()) for n in range(1,6)])
            # Native corruption is exercised on real receipts by rejection_probes;
            # this unit test isolates the enclosing inventory/identity contract.
            with patch.object(journey,'observe',return_value={'native':True}), patch.object(journey,'rejection_probes',return_value={'native':True}), \
                    patch.object(journey,'environment',return_value={'test':'observed'}):
                report['identity']['environment'] = {'test':'observed'}
                self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
                mutations = [lambda r:r.update(cleanup=False),lambda r:r.update(semantic_score=0),
                    lambda r:r.update(eligible_for_grading=True),lambda r:r.update(release_ready=True),
                    lambda r:r['identity'].update(binary_sha256='0'*64),lambda r:r['identity'].update(source_after={}),
                    lambda r:r['identity'].update(environment={}),lambda r:r['identity'].update(environment=True),
                    lambda r:r['identity'].update(environment={'test':True}),
                    lambda r:r['identity'].update(environment={'test':'FORGED'}),lambda r:r['results'].pop(),
                    lambda r:r['results'].append(r['results'][0]),lambda r:r['results'].reverse(),
                    lambda r:r['results'][0].update(repetition=True),lambda r:r['results'][0].update(cleanup={}),
                    lambda r:r['results'][0].update(assertions={'native':1}),lambda r:r.update(rejection_probes={})]
                mutations.append(lambda r:r['results'][1].update(journey=copy.deepcopy(r['results'][0]['journey'])))
                for mutate in mutations:
                    changed = copy.deepcopy(report); mutate(changed)
                    with self.assertRaises(RuntimeError): journey.validate_report(changed,binary)
                with patch.object(journey,'observe',return_value={'native':False}):
                    with self.assertRaises(RuntimeError): journey.validate_report(report,binary)


if __name__ == '__main__':
    unittest.main()
