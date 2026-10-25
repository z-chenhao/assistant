"""GC-09 frozen scope, strict native bytes and ungraded preview contracts."""
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
import unscripted_cases
import verify_gc09 as journey
import verify_suite


class AgendaBoundaryTests(unittest.TestCase):
    def test_original_seventeen_blocks_and_variants_remain_locked(self):
        lock = journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [271,273,275,276,277,278,279,281,753,755,771,773,775,776,777,778,779])
        rows = list(variants(*journey.inputs()))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['events'][0]['text'] for r in rows}),3)
        self.assertEqual(sum(r['noise_characters'] > 0 for r in rows),2)
        self.assertEqual(len({journey.supplement(r)[0] for r in rows}),3)
        for row in rows:
            original = unscripted_cases.inputs('GC-09',row['variant_id'])
            self.assertEqual(original['context'],row['context'])
            self.assertEqual(original['events'],[dict(at=e['at'],text=e['text']) for e in row['events']])
            self.assertNotIn('GC09 delegate',json.dumps(original))
        self.assertIs(unscripted_cases.matrix()['GC-09']['eligible_for_grading'],False)
        self.assertEqual(journey.REPEATS,5)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script == 'verify_gc09.py'],[('runtime',1)])
        # The connector gate starts real sandboxed stdio peers. Runtime did not
        # need Bubblewrap before this gate; Linux must provision its dependency.
        workflow = (journey.ROOT/'.github/workflows/ci.yml').read_text()
        self.assertIn("sandbox: ${{ matrix.shard == 'runtime' || matrix.shard == 'security' || matrix.shard == 'communication' }}",workflow)

    def test_native_manifest_rejects_summary_size_hash_path_and_symlink_forgery(self):
        report = dict(status='mechanical_pass',cleanup=True,results=[{'native':'RAW_ONLY'*200000}],
            identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/'gc09.json'; manifest = journey.retain_report(path,report)
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
        for report in (dict(case_id='GC-09'),dict(case_id='GC-09',status='mechanical_pass',semantic_score=100)):
            with self.assertRaises(RuntimeError): prepare_release_eval.prepare(report)
        with patch.object(journey,'validate_report',return_value=dict(receipts=30)) as validate:
            result = prepare_release_eval.prepare(dict(case_id='GC-09'))
        validate.assert_called_once()
        self.assertEqual(result['judge_packets'],[])
        self.assertIs(result['eligible_for_grading'],False)
        self.assertIsNone(result['scores'])
        self.assertIsNone(result['winner'])

    def test_failed_launch_removes_stale_success_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/'gc09.json'; path.write_text('{"status":"mechanical_pass"}')
            failed = subprocess.run([sys.executable,str(journey.ROOT/'scripts/verify_gc09.py'),
                '--binary',str(Path(tmp)/'missing'),'--evidence',str(path)],capture_output=True,text=True)
            self.assertNotEqual(failed.returncode,0)
            self.assertFalse(path.exists())

    def test_validator_requires_complete_inventory_identity_cleanup_and_recomputation(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = Path(tmp)/'erii'; binary.write_bytes(b'test identity only')
            identity = journey.source_identity()
            report = dict(case_id='GC-09',status='mechanical_pass',fixture_lock=journey.load(),semantic_score=None,
                eligible_for_grading=False,release_ready=False,cleanup=True,rejection_probes={'native':True},
                identity=dict(source_before=identity,source_after=identity,binary_sha256=journey.sha(binary.read_bytes()),environment={'test':True}),
                results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'native':True},
                    cleanup=dict(disposable_root_removed=True,owned_processes_joined=True))
                    for v in variants(*journey.inputs()) for n in range(1,6)])
            # Native corruption is exercised on real receipts by rejection_probes;
            # this unit test isolates the enclosing inventory/identity contract.
            with patch.object(journey,'observe',return_value={'native':True}), patch.object(journey,'rejection_probes',return_value={'native':True}):
                self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
                mutations = [lambda r:r.update(cleanup=False),lambda r:r.update(semantic_score=0),
                    lambda r:r.update(eligible_for_grading=True),lambda r:r.update(release_ready=True),
                    lambda r:r['identity'].update(binary_sha256='0'*64),lambda r:r['identity'].update(source_after={}),
                    lambda r:r['identity'].update(environment={}),lambda r:r['results'].pop(),
                    lambda r:r['results'].append(r['results'][0]),lambda r:r['results'].reverse(),
                    lambda r:r['results'][0].update(repetition=True),lambda r:r['results'][0].update(cleanup={}),
                    lambda r:r['results'][0].update(assertions={'native':1}),lambda r:r.update(rejection_probes={})]
                for mutate in mutations:
                    changed = copy.deepcopy(report); mutate(changed)
                    with self.assertRaises(RuntimeError): journey.validate_report(changed,binary)
                with patch.object(journey,'observe',return_value={'native':False}):
                    with self.assertRaises(RuntimeError): journey.validate_report(report,binary)


if __name__ == '__main__':
    unittest.main()
