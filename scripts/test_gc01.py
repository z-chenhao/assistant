"""GC-01 source, user-input and strict receipt consumer regressions."""
import copy
import hashlib
from pathlib import Path
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

import verify_gc01 as journey
from case_runner import variants
import unscripted_cases
import verify_suite


class HealthTests(unittest.TestCase):
    def test_morning_requires_each_original_context_and_model_delivery_agreement(self):
        import json
        _, variant, _, _ = journey.inputs('wording-1')
        users=[variant['context']] + ['当前时间：'+e['at']+'。'+e['text'] for e in variant['events']]
        state=dict(conversation=[dict(role='user',content=[dict(text=t)]) for t in users] +
            [dict(role='assistant',content=[dict(text='reply'+str(i))]) for i in range(3)],artifact=[],generations=[
                dict(status='completed',request_json=json.dumps(dict(messages=[dict(role='user',text=t)
                    for t in users[:2+i//2]])),result_json=json.dumps(dict(text='reply'+str(i//2)))) for i in range(6)])
        row=dict(variant=variant,safety=False,state=state,replay_state=copy.deepcopy(state),
            restart=dict(boundary='after_generation_commit',returncode=-9,state=dict(generations=[state['generations'][0]])))
        with patch.object(journey,'result',return_value={'assertions':{}}), patch.object(journey,'main_checks',return_value={}):
            self.assertTrue(all(journey.morning_observe(row,'wording-1').values()))
            for index in range(6):
                changed=copy.deepcopy(row)
                changed['state']['generations'][index]['request_json']='{"messages":[]}'
                self.assertFalse(journey.morning_observe(changed,'wording-1')['context_from_original'])
            changed=copy.deepcopy(row);changed['state']['generations'][-1]['result_json']='{"text":"forged"}'
            self.assertFalse(journey.morning_observe(changed,'wording-1')['delivered_model_text'])

    def test_mechanical_preparation_preserves_the_existing_cli_packet_contract(self):
        import prepare_release_eval
        with patch.object(journey,'validate_report',return_value={'receipts':30}):
            prepared=prepare_release_eval.prepare(dict(case_id='GC-01'))
        self.assertEqual(prepared['judge_packets'],[])
        self.assertEqual(len(prepared['judge_packets']),0)
        self.assertFalse(prepared['eligible_for_grading'])
        self.assertIsNone(prepared['scores'])

    def test_bounded_variants_keep_each_five_round_series_serial_and_output_ordered(self):
        active, peak = set(), 0
        starts = {}
        lock = threading.Lock()
        def drive(binary, item, failure):
            nonlocal peak
            variant, repetition = item
            with lock:
                self.assertNotIn(variant,active)
                active.add(variant);peak=max(peak,len(active))
                starts.setdefault(variant,[]).append(repetition)
            time.sleep(.005)
            with lock:active.remove(variant)
            return dict(variant_id=variant,repetition=repetition,assertions={'observed':True})
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);binary=root/'erii';binary.write_bytes(b'binary')
            output=root/'receipt.json'
            with patch.object(journey,'drive',side_effect=drive), \
                    patch.object(journey,'source_identity',return_value={'sha':'test'}), \
                    patch.object(journey,'rejection_probes',return_value={'corrupt':True}), \
                    patch.object(journey,'validate_report',return_value={'receipts':30}), \
                    patch.object(journey.platform,'platform',return_value='test'), \
                    patch.object(journey.subprocess,'check_output',return_value='test'), \
                    patch('sys.argv',['verify_gc01.py','--binary',str(binary),'--evidence',str(output),'--variant-jobs','2']):
                journey.main()
            import json
            report=json.loads(output.read_text())
            self.assertEqual(peak,2)
            self.assertTrue(all(rounds==[1,2,3,4,5] for rounds in starts.values()))
            self.assertEqual([(r['variant_id'],r['repetition']) for r in report['results']],
                [(v['variant_id'],n) for v in variants(*journey._case_dataset()) for n in range(1,6)])

    def test_twenty_five_frozen_blocks_and_all_original_variants_are_retained(self):
        fixture, lock = journey.load()
        self.assertEqual(len(lock['frozen_blocks']), 25)
        self.assertEqual(lock['manifest_sha256'], '0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([r['line'] for r in lock['frozen_blocks']],
            [159,161,163,164,165,166,167,169,472,474,491,493,494,495,497,499,500,501,502,504,506,507,508,509,511])
        dataset, case = journey._case_dataset()
        all_variants = list(variants(dataset, case))
        self.assertEqual(len(all_variants), 6)
        self.assertEqual(sum(v['noise_characters'] > 0 for v in all_variants), 2)
        for variant in all_variants:
            self.assertEqual(journey.inputs(variant['variant_id'])[1], variant)
            events, _ = journey.journey_inputs(variant['variant_id'])
            self.assertEqual(events[:3], [dict(at=e['at'],text=e['text']) for e in variant['events']])
            self.assertEqual(events[3:9], fixture['events'])
            self.assertEqual(len({e['at'][:10] for e in events}), 7)
        self.assertEqual(journey.REPEATS, 5)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs
            if g.script == 'verify_gc01.py'], [('communication',1)])

    def test_unscripted_week_and_replacement_safety_export_user_data_without_gold(self):
        for variant in variants(*journey._case_dataset()):
            for mode in ('week','safety'):
                value = unscripted_cases.inputs('GC-01',variant['variant_id'],mode)
                self.assertEqual(value['events'], journey.journey_inputs(variant['variant_id'],mode=='safety')[0])
                self.assertEqual(value['context'],variant['context'])
                for event in value['events']:
                    self.assertEqual(set(event), {'at','text'})
                    for token in ('Capabilities:', 'required_text', 'urgent_evaluation', 'builtin__'):
                        self.assertNotIn(token,event['text'])
                if mode == 'safety':
                    self.assertEqual(len(value['events']),2)
                    self.assertNotIn('没有',value['events'][1]['text'])
            self.assertFalse(unscripted_cases.matrix()['GC-01']['eligible_for_grading'])
        with self.assertRaisesRegex(ValueError, 'GC-01 journey only'):
            unscripted_cases.inputs('GC-09','wording-1','week')

    def test_no_work_and_snapshot_oracle_rejects_unrequested_or_unfrozen_state(self):
        import json
        state = dict(task=[],automation=[],approval=[],runs=[dict(run_id='r',kind='main',task_id=None,
            parent_run_id=None,model_snapshot_id='m',capability_snapshot_id='c')],
            capabilities=[dict(snapshot_id='c',tools_json='[]')],tools=[],
            generations=[dict(run_id='r',request_json=json.dumps(dict(model_snapshot=dict(snapshot_id='m'),tools=[])))],
            conversation=[dict(role='assistant',message_id='message')],deliveries=[dict(message_id='message',status='sent')])
        self.assertTrue(all(journey.main_checks(state).values()))
        mutations = [lambda r:r['task'].append({}),lambda r:r['automation'].append({}),
            lambda r:r['approval'].append({}),lambda r:r['runs'][0].update(kind='worker'),
            lambda r:r['tools'].append(dict(run_id='r',tool_name='unadvertised')),
            lambda r:r['deliveries'].append(r['deliveries'][0]),
            lambda r:r['generations'].append(dict(run_id='r',request_json=json.dumps(
                dict(model_snapshot=dict(snapshot_id='m'),tools=[dict(name='drift')]))))]
        for mutate in mutations:
            changed=copy.deepcopy(state);mutate(changed)
            self.assertFalse(all(journey.main_checks(changed).values()))

    def test_strict_consumer_rejects_scores_missing_rounds_locks_identity_cleanup_and_forged_flags(self):
        _, lock = journey.load()
        source = dict(sha='a'*40, files={'source':'hash'})
        report = dict(case_id='GC-01',status='mechanical_pass',semantic_score=None,eligible_for_grading=False,
            fixture_lock=lock,morning_lock=journey.load_contract(journey.ROOT)[2],
            m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source, binary_sha256=hashlib.sha256(b'binary').hexdigest(),
                environment=dict(platform='test')),rejection_probes={'corrupt':True},results=[
                dict(variant_id=v['variant_id'],repetition=n,assertions={'observed':True},
                     cleanup=dict(disposable_root_removed=True,owned_processes_joined=True))
                for v in variants(*journey._case_dataset()) for n in range(1,6)])
        with tempfile.TemporaryDirectory() as tmp, patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'observe',return_value={'observed':True}), \
                patch.object(journey,'rejection_probes',return_value={'corrupt':True}):
            binary=Path(tmp)/'erii';binary.write_bytes(b'binary')
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            mutations=[lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
                lambda r:r.update(eligible_for_grading=True),lambda r:r.update(cleanup=False),
                lambda r:r['results'].pop(),lambda r:r['results'].append(r['results'][0]),
                lambda r:r['results'].reverse(),lambda r:r.update(fixture_lock={}),
                lambda r:r.update(morning_lock={}),lambda r:r.update(m6_lock={}),
                lambda r:r['identity'].update(binary_sha256='0'*64),
                lambda r:r['identity'].update(source_after={}),
                lambda r:r['results'][0].update(assertions={'observed':False}),
                lambda r:r['results'][0].update(assertions={'observed':1}),
                lambda r:r['results'][0].update(repetition=True),
                lambda r:r['results'][0].update(cleanup={}),lambda r:r.update(rejection_probes={})]
            for mutate in mutations:
                changed=copy.deepcopy(report);mutate(changed)
                with self.assertRaises(RuntimeError):journey.validate_report(changed,binary)


if __name__ == '__main__': unittest.main()
