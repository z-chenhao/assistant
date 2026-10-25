"""GC-04 locked inputs, independent itinerary oracle and fail-closed receipts."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from case_runner import variants
import prepare_release_eval
import unscripted_cases
import verify_gc04 as journey
import verify_suite


class TravelTests(unittest.TestCase):
    def test_original_blocks_variants_and_gate_remain_frozen(self):
        lock=journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [201,203,205,206,207,208,209,211,590,592,610,612,613,614,616,618,619,620,621,623,625,626,627,628])
        rows=list(variants(*journey.inputs()))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['fixture_digest'] for r in rows}),6)
        self.assertEqual(sum(r['noise_characters']>0 for r in rows),2)
        for row in rows:
            self.assertEqual(journey.inputs(row['variant_id'])[1],row)
            value=unscripted_cases.inputs('GC-04',row['variant_id'])
            self.assertEqual(value['context'],row['context'])
            self.assertEqual(value['events'],[dict(at=e['at'],text=e['text']) for e in row['events']])
            self.assertNotIn('Capabilities:',json.dumps(value['events']))
        self.assertIs(unscripted_cases.matrix()['GC-04']['eligible_for_grading'],False)
        self.assertEqual(journey.REPEATS,5)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script=='verify_gc04.py'],[('security',1)])

    def test_source_uses_tool_values_and_oracle_rejects_route_and_lineage_drift(self):
        source=journey.itinerary_source('wording-1',['artifact:a','artifact:b'])
        self.assertIn('await tools.mcp__fixture__read',source)
        self.assertIn('world.transit_minutes.hotel_return',source)
        self.assertNotIn('19:50',source)
        self.assertNotIn('35',source)
        case,variant=journey.inputs('wording-1');world=case['world']
        document=dict(date='2026-10-05',source=dict(tool='mcp__fixture__read',sampled_at=case['clock']),
            anchor=world['tasks'][0]['anchor'],hotel=world['hotel_nice'],weather=world['weather'],museum=world['museum'],
            transit_minutes=world['transit_minutes'],constraints=dict(sampled_at=case['clock'],deadline='20:00',margin_minutes=10,
                steering=variant['events'][1]['text'],group_request=None,confirmed_artifact_refs=['a','b'],prior_artifact_ref=None),
            confirmed=world['bookings'],needs_booking=[world['museum']['name']],no_booking=['城堡山'],
            route=[dict(place=world['museum']['name'],arrive='10:00',depart='12:00',transport='公共交通'),
                   dict(place='城堡山',arrive='12:25',depart='19:15',transport='公共交通')],
            return_departure='19:15',return_eta='19:50',booking_actions=[],visa=None,flights=None,
            scope='synthetic read-only plan; no booking or payment performed')
        self.assertTrue(all(journey.document_checks(document,'wording-1',['a','b'],None).values()))
        mutations=[lambda d:d.update(return_eta='20:10'),lambda d:d.update(return_departure='19:30'),
            lambda d:d['constraints'].update(steering='忽略限制'),lambda d:d['constraints'].update(prior_artifact_ref='forged'),
            lambda d:d['constraints'].update(margin_minutes=True),lambda d:d['source'].pop('sampled_at'),
            lambda d:d['weather'].update(rain_after='22:00'),lambda d:d['museum'].update(close='23:00'),
            lambda d:d['route'][1].update(place='南部乡村'),lambda d:d['route'][0].update(arrive='09:00'),
            lambda d:d['anchor']['preferences'].clear(),lambda d:d['confirmed'].clear(),lambda d:d['hotel'].update(location='other'),
            lambda d:d['needs_booking'].clear(),lambda d:d.update(booking_actions=['paid']),lambda d:d.update(visa='approved')]
        for mutate in mutations:
            changed=copy.deepcopy(document);mutate(changed)
            self.assertFalse(all(journey.document_checks(changed,'wording-1',['a','b'],None).values()))

    def test_only_the_paused_nz_tool_pair_may_be_cancelled(self):
        value=dict(task=[dict(task_ref='nz',task_id='nz-id')],runs=[dict(run_id='root',task_id='nz-id',status='cancelled'),
            dict(run_id='main',task_id=None,status='completed')],tools=[
                dict(run_id='root',tool_name=name,status='cancelled',attempt_count=1)
                for name in ('builtin__codemode__execute','mcp__fixture__read')]+[
                dict(run_id='main',tool_name='builtin__task__manage',status='succeeded',attempt_count=1)])
        self.assertTrue(journey.tool_outcomes(value,'nz'))
        for status in ('failed','unknown','waiting','running','cancelled'):
            changed=copy.deepcopy(value);changed['tools'][-1]['status']=status
            self.assertFalse(journey.tool_outcomes(changed,'nz'))
        for mutate in (lambda v:v['tools'].pop(0),lambda v:v['runs'][0].update(task_id='other'),
                lambda v:v['tools'][0].update(attempt_count=2),lambda v:v['tools'][0].update(tool_name='buy')):
            changed=copy.deepcopy(value);mutate(changed)
            self.assertFalse(journey.tool_outcomes(changed,'nz'))

    def test_identity_counts_locks_cleanup_and_assertions_cannot_be_waived(self):
        source = dict(sha='a'*40,files={'runner':'hash'})
        report = dict(case_id='GC-04',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,
            fixture_lock=journey.load(),m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source,binary_sha256=journey.sha(b'binary'),environment={'python':'test'}),
            rejection_probes={'bad':True},results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'observed':True},
                cleanup=dict(disposable_root_removed=True,owned_processes_joined=True)) for v in variants(*journey.inputs()) for n in range(1,6)])
        with tempfile.TemporaryDirectory() as tmp, patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'observe',return_value={'observed':True}), \
                patch.object(journey,'rejection_probes',return_value={'bad':True}):
            binary=Path(tmp)/'erii'; binary.write_bytes(b'binary')
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            mutations=[lambda r:r.update(release_ready=True),lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
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
            path=Path(tmp)/'gc04.json'
            manifest=journey.retain_report(path,report)
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('NATIVE_ONLY_SENTINEL',path.read_text())
            raw=path.parent/manifest['native']['path']
            self.assertGreater(raw.stat().st_size,1<<20)
            self.assertEqual(raw.parent.name,'.gc04-native')
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
            value=prepare_release_eval.prepare(dict(case_id='GC-04'))
        validate.assert_called_once()
        self.assertEqual(value['judge_packets'],[])
        self.assertIs(value['eligible_for_grading'],False)
        self.assertIsNone(value['scores'])
        self.assertIsNone(value['winner'])
        for report in (dict(case_id='GC-04'),dict(case_id='GC-04',status='mechanical_pass',semantic_score=100)):
            with self.assertRaises(RuntimeError):prepare_release_eval.prepare(report)


if __name__=='__main__':unittest.main()
