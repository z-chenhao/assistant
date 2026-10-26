"""GC-02 frozen inputs and fail-closed native receipt consumer."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from case_runner import variants
import prepare_release_eval
import unscripted_cases
import verify_gc02 as journey
import verify_suite


class GoalsTests(unittest.TestCase):
    def test_original_twenty_four_blocks_and_six_variants_are_unchanged(self):
        fixture, lock = journey.load()
        self.assertEqual(lock['manifest_sha256'], '0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [173,175,177,178,179,180,181,183,515,517,532,534,535,536,537,539,541,542,543,544,546,548,549,550])
        data, case = journey.inputs()
        rows = list(variants(data,case))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['fixture_digest'] for r in rows}),6)
        self.assertEqual(sum(r['noise_characters'] > 0 for r in rows),2)
        for row in rows:
            self.assertEqual(journey.inputs(row['variant_id'])[1],row)
            self.assertEqual(len(row['events']),4)
            # The maintained real-model entry still sees only original user
            # data, never this deterministic harness's operations or answers.
            value = unscripted_cases.inputs('GC-02',row['variant_id'])
            self.assertEqual(value['context'],row['context'])
            self.assertEqual(value['events'],[dict(at=e['at'],text=e['text']) for e in row['events']])
            self.assertNotIn('Capabilities:',json.dumps(value['events']))
        self.assertIs(unscripted_cases.matrix()['GC-02']['eligible_for_grading'],False)
        self.assertEqual(journey.REPEATS,5)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script=='verify_gc02.py'],[('communication',1)])

    def test_schedule_reads_live_memory_without_embedding_goal_answer_or_reference(self):
        source = journey.review_source()
        self.assertIn('await tools.builtin__memory__search',source)
        self.assertIn('h.source_type===',source)
        self.assertNotIn(journey.load()[0]['current'],source)
        self.assertNotIn('memory:',source)

    def test_identity_counts_locks_cleanup_and_assertions_cannot_be_waived(self):
        source = dict(sha='a'*40,files={'runner':'hash'})
        report = dict(case_id='GC-02',status='mechanical_pass',semantic_score=None,eligible_for_grading=False,
            fixture_lock=journey.load()[1],m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source,binary_sha256=journey.sha(b'binary'),environment={'python':'test'}),
            rejection_probes={'bad':True},results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'observed':True},
                cleanup=dict(disposable_root_removed=True,owned_processes_joined=True)) for v in variants(*journey.inputs()) for n in range(1,6)])
        with tempfile.TemporaryDirectory() as tmp, patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'observe',return_value={'observed':True}), \
                patch.object(journey,'rejection_probes',return_value={'bad':True}):
            binary=Path(tmp)/'erii'; binary.write_bytes(b'binary')
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            mutations=[lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
                lambda r:r.update(eligible_for_grading=True),lambda r:r.update(cleanup=False),
                lambda r:r.update(fixture_lock={}),lambda r:r.update(m6_lock={}),
                lambda r:r['identity'].update(source_after={}),lambda r:r['identity'].update(binary_sha256='0'*64),
                lambda r:r['identity'].update(environment={}),lambda r:r['results'].pop(),
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
            path=Path(tmp)/'gc02.json'
            manifest=journey.retain_report(path,report)
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('NATIVE_ONLY_SENTINEL',path.read_text())
            raw=path.parent/manifest['native']['path']
            self.assertGreater(raw.stat().st_size,1<<20)
            self.assertEqual(raw.parent.name,'.gc02-native')
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

    def test_preview_never_constructs_judge_packets_or_scores_for_scripted_run(self):
        with patch.object(journey,'validate_report',return_value={'receipts':30}) as validate:
            value=prepare_release_eval.prepare(dict(case_id='GC-02'))
        validate.assert_called_once()
        self.assertEqual(value['judge_packets'],[])
        self.assertIs(value['eligible_for_grading'],False)
        self.assertIsNone(value['scores'])
        self.assertIsNone(value['winner'])
        for report in (dict(case_id='GC-02'),dict(case_id='GC-02',status='mechanical_pass',semantic_score=100)):
            with self.assertRaises(RuntimeError):prepare_release_eval.prepare(report)


if __name__=='__main__':unittest.main()
