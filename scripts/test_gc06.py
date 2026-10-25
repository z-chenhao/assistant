"""GC-06 frozen inputs and fail-closed native receipt consumer."""
import copy
import json
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest
from unittest.mock import patch

from case_runner import variants
import prepare_release_eval
import unscripted_cases
import verify_gc06 as journey
import verify_suite


class LearningTests(unittest.TestCase):
    def test_original_nineteen_blocks_six_variants_and_unscripted_inputs(self):
        lock=journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [229,231,233,234,235,236,237,239,668,670,679,681,682,683,685,687,688,689,690])
        data,case=journey.inputs();rows=list(variants(data,case))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['fixture_digest'] for r in rows}),6)
        self.assertEqual(sum(r['noise_characters']>0 for r in rows),2)
        for row in rows:
            self.assertEqual(journey.inputs(row['variant_id'])[1],row)
            self.assertEqual(len(row['events']),3)
            value=unscripted_cases.inputs('GC-06',row['variant_id'])
            self.assertEqual(value['context'],row['context'])
            self.assertEqual(value['events'],[dict(at=e['at'],text=e['text']) for e in row['events']])
            self.assertNotIn('Capabilities:',json.dumps(value['events']))
        self.assertIs(unscripted_cases.matrix()['GC-06']['eligible_for_grading'],False)
        self.assertEqual(journey.REPEATS,5)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script=='verify_gc06.py'],[('security',1)])

    def test_objective_basis_adapts_without_language_grade_or_permanent_deficit(self):
        source=journey.report_source(0)
        self.assertIn('await tools.builtin__memory__search',source)
        self.assertIn('await tools.mcp__fixture__read',source)
        self.assertNotIn('memory:',source)
        self.assertNotIn(journey.fixture()['private'],source)
        low=journey.expected_report('memory:old',2,0)
        high=journey.expected_report('memory:guided',1,1)
        resume=journey.expected_report('memory:current',3)
        self.assertEqual(low['assessment']['score'],1)
        self.assertEqual(low['next']['difficulty'],'guided')
        self.assertEqual(high['assessment']['score'],3)
        self.assertEqual(high['next']['difficulty'],'independent')
        self.assertEqual(resume['next'],high['next'])
        self.assertEqual(low['next']['mastered'],journey.inputs()[1]['world']['mastered'])
        self.assertEqual(len(low['corrections']),3)
        self.assertFalse(low['practice']['refund_confirmed'])
        self.assertFalse(low['next']['permanent_deficit'])
        self.assertIsNone(low['assessment']['language_quality_score'])
        self.assertIsNone(resume['assessment'])
        self.assertTrue(all(r.isascii() for r in journey.REPLIES[:2]))
        self.assertFalse(journey.exact({'score':1},{'score':True}))

    def test_native_output_cannot_disagree_with_cached_return(self):
        returned=dict(result=journey.expected_report('memory:current',3),artifactRefs=['artifact:learning'])
        output=[dict(type='return',value=returned)]
        receipt=dict(parent=dict(result_json=json.dumps(dict(text=json.dumps(output)))),output=copy.deepcopy(output),value=copy.deepcopy(returned))
        self.assertEqual(journey.native_return(receipt),returned)
        for mutate in (lambda c:c['value']['result']['next'].update(permanent_deficit=True),
                lambda c:c['output'].append(dict(type='text',text='forged')),
                lambda c:c['parent'].update(result_json=json.dumps(dict(text=json.dumps([dict(type='return',value={})]))))):
            changed=copy.deepcopy(receipt);mutate(changed)
            with self.assertRaisesRegex(RuntimeError,'native output/value drift'):journey.native_return(changed)
        for invalid in ([],output*2,dict(value=returned)):
            with self.assertRaisesRegex(RuntimeError,'native'):
                journey.native_return(dict(parent=dict(result_json=json.dumps(dict(text=json.dumps(invalid)))),output=invalid,value=returned))
        for invalid in ('[{"type":"return","value":{},"value":null}]','[{"type":"return","value":NaN}]'):
            with self.assertRaises(ValueError):journey.native_return(dict(parent=dict(result_json=json.dumps(dict(text=invalid)))))

    def test_identity_counts_locks_cleanup_and_assertions_cannot_be_waived(self):
        source = dict(sha='a'*40,files={'runner':'hash'})
        report = dict(case_id='GC-06',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,
            fixture_lock=journey.load(),m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source,binary_sha256=journey.sha(b'binary'),environment={'python':'test'}),
            rejection_probes={'bad':True},results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'observed':True},
                cleanup=dict(disposable_root_removed=True,owned_processes_joined=True)) for v in variants(*journey.inputs()) for n in range(1,6)])
        with tempfile.TemporaryDirectory() as tmp, patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'environment_identity',return_value={'python':'test'}), \
                patch.object(journey,'observe',return_value={'observed':True}), \
                patch.object(journey,'rejection_probes',return_value={'bad':True}):
            binary=Path(tmp)/'erii'; binary.write_bytes(b'binary')
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            mutations=[lambda r:r.update(release_ready=True),lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
                lambda r:r.update(eligible_for_grading=True),lambda r:r.update(cleanup=False),
                lambda r:r.update(fixture_lock={}),lambda r:r.update(m6_lock={}),
                lambda r:r['identity'].update(source_after={}),lambda r:r['identity'].update(binary_sha256='0'*64),
                lambda r:r['identity'].update(environment={}),lambda r:r['identity'].update(environment={'python':'forged'}),lambda r:r['results'].pop(),
                lambda r:r['results'].append(r['results'][0]),lambda r:r['results'].reverse(),
                lambda r:r['results'][0].update(repetition=True),lambda r:r['results'][0].update(cleanup={}),
                lambda r:r['results'][0].update(assertions={'observed':1}),
                lambda r:r.update(rejection_probes={})]
            for mutate in mutations:
                changed=copy.deepcopy(report);mutate(changed)
                with self.assertRaises(RuntimeError):journey.validate_report(changed,binary)
            # A forged all-true assertion cannot mask a failed recomputation.
            with patch.object(journey,'observe',return_value={'observed':False}):
                with self.assertRaises(RuntimeError):journey.validate_report(report,binary)

    def test_small_manifest_keeps_complete_native_bytes_and_rejects_drift_and_unsafe_paths(self):
        report=dict(status='mechanical_pass',cleanup=True,results=[{'payload':'NATIVE_ONLY_SENTINEL' * 100000}],
            identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'gc06.json'
            manifest=journey.retain_report(path,report)
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('NATIVE_ONLY_SENTINEL',path.read_text())
            raw=path.parent/manifest['native']['path']
            self.assertGreater(raw.stat().st_size,1<<20)
            self.assertEqual(raw.parent.name,'.gc06-native')
            self.assertEqual(journey.read_report(path),report)
            mutations=[lambda m:m['summary'].update(receipts=30),lambda m:m['summary'].update(semantic_score=100),
                lambda m:m['summary'].update(cleanup=1),lambda m:m['native'].update(sha256='0'*64),
                lambda m:m['native'].update(path='../outside.json'),lambda m:m['native'].update(path=str(raw)),
                lambda m:m.update(native_bytes=True),lambda m:m.update(native_bytes=0),lambda m:m.update(version=True)]
            for mutate in mutations:
                changed=copy.deepcopy(manifest);mutate(changed);path.write_text(json.dumps(changed))
                with self.assertRaises((ValueError,RuntimeError)):journey.read_report(path)
            path.write_text(json.dumps(manifest))
            actual=raw.with_suffix('.saved');raw.rename(actual);raw.symlink_to(actual)
            with self.assertRaises(ValueError):journey.read_report(path)
            raw.unlink();actual.rename(raw)
            raw.write_text(raw.read_text()+' ')
            with self.assertRaises(ValueError):journey.read_report(path)
            # CI retains the public hash manifest while the pinned uploader
            # excludes dot-directories by default; never opt in to raw upload.
            workflow=(journey.ROOT/'.github/workflows/ci.yml').read_text()
            self.assertNotIn('include-hidden-files: true',workflow)

    def test_failed_cli_cannot_leave_a_previous_success_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'gc06.json'
            path.write_text('{"status":"mechanical_pass"}')
            failed=subprocess.run([sys.executable,str(journey.ROOT/'scripts/verify_gc06.py'),
                '--binary',str(Path(tmp)/'missing-erii'),'--evidence',str(path)],capture_output=True,text=True)
            self.assertNotEqual(failed.returncode,0)
            self.assertFalse(path.exists())
            self.assertIn('FileNotFoundError',failed.stderr)

    def test_preview_never_constructs_judge_packets_or_scores_for_scripted_run(self):
        with patch.object(journey,'validate_report',return_value={'receipts':30}) as validate:
            value=prepare_release_eval.prepare(dict(case_id='GC-06'))
        validate.assert_called_once()
        self.assertEqual(value['judge_packets'],[])
        self.assertIs(value['eligible_for_grading'],False)
        self.assertIsNone(value['scores'])
        self.assertIsNone(value['winner'])
        for report in (dict(case_id='GC-06'),dict(case_id='GC-06',status='mechanical_pass',semantic_score=100)):
            with self.assertRaises(RuntimeError):prepare_release_eval.prepare(report)


if __name__=='__main__':unittest.main()
