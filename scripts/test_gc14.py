"""GC-14 oracle rejects schedule, source, notification and replay drift."""
import copy
from datetime import datetime
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import monitor_sandbox as peer
import prepare_release_eval as evaluator
import verify_gc14 as journey


class MonitorTests(unittest.TestCase):
    def state(self, prices=None):
        source = journey.variants()[0]['source']
        if prices is not None:
            source['daily_prices'] = dict(zip(sorted(source['daily_prices']), prices))
        value = dict(automation_fire=[], runs=[], tools=[], task_update=[], deliveries=[dict(message_id='setup', status='sent')],
                     conversation=[dict(role='assistant')], task=[dict(task_id='task', status='active', completion_policy='ongoing')])
        value['tools'].append(dict(tool_name='mcp__fixture__read', run_id='main'))
        entered = False
        for day, price in sorted(source['daily_prices'].items()):
            below = price < source['threshold_cny']; notify = below and not entered; entered = below
            value['automation_fire'].append(dict(fire_at=int(datetime.fromisoformat(day+'T08:00:00+08:00').timestamp()*1e6),
                state='admitted', below_value=1800, observed_value=price, notify=notify, root_run_id=day))
            value['runs'].append(dict(run_id=day, kind='worker', status='completed', task_id='task', depth=0, parent_run_id=None))
            for site in ('site-a', 'site-b'):
                value['tools'].append(dict(tool_name='mcp__fixture__read', run_id=day, arguments_json=json.dumps(dict(text=day+':'+site)),
                    result_json=json.dumps(dict(text=json.dumps(journey.sources(source)[day+':'+site])))))
                value['tools'].append(dict(tool_name='builtin__automation__observe', run_id=day, arguments_json=json.dumps(dict(value=price))))
            if notify:
                value['task_update'].append(dict(root_run_id=day,
                    content=f'航班 HKG-CDG 2026-12-01：{day} 核验价格 {price} CNY；阈值 1800 CNY。'))
                value['deliveries'].append(dict(message_id=day, status='sent'))
                value['conversation'].append(dict(role='assistant'))
        return source, value

    def test_exact_six_days_reject_changed_schedule_threshold_source_and_extra_notifications(self):
        source, value = self.state()
        self.assertTrue(all(journey.price_observe(value, source, 6).values()))
        mutations = [lambda v:v['automation_fire'][0].update(fire_at=v['automation_fire'][0]['fire_at']+8*3600*1000000),
            lambda v:v['automation_fire'][0].update(observed_value=1750),
            lambda v:v['automation_fire'][2].update(notify=False),
            lambda v:v['automation_fire'][3].update(notify=True),
            lambda v:v['automation_fire'][0].update(below_value=1801),
            lambda v:v['automation_fire'].append(v['automation_fire'][0]),
            lambda v:v['runs'][0].update(task_id='other'),
            lambda v:v['runs'][0].update(status='waiting'),
            lambda v:v['task'][0].update(status='completed'),
            lambda v:v['deliveries'].append(v['deliveries'][1]),
            lambda v:v['task_update'][0].update(content='价格1700美元'),
            lambda v:v['tools'][1].update(result_json=json.dumps(dict(text='{"currency":"USD"}'))),
            lambda v:v['tools'][2].update(arguments_json='{"value":1749}'),
            lambda v:v['tools'].pop()]
        for mutate in mutations:
            changed = copy.deepcopy(value); mutate(changed)
            self.assertFalse(all(journey.price_observe(changed, source, 6).values()))

    def test_threshold_is_strict_and_reentry_is_required(self):
        for prices, dates in [([1800]*6, []), ([1799]*6, ['2026-10-16']),
                              ([1799,1800,1799,1800,1799,1800], ['2026-10-16','2026-10-18','2026-10-20'])]:
            source, value = self.state(prices)
            self.assertTrue(all(journey.price_observe(value, source, 6).values()))
            self.assertEqual([journey.day_of(f['fire_at']) for f in value['automation_fire'] if f['notify']], dates)

    def test_peer_returns_exact_bytes_and_retains_duplicate_and_unknown_reads(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder); values = journey.sources(journey.variants()[0]['source'])
            (root/'source.json').write_text(json.dumps(values))
            requests = [dict(id=1,method='initialize'), dict(id=2,method='tools/list'),
                dict(id=3,method='tools/call',params=dict(name='read',arguments=dict(text='2026-10-18:site-b')))]
            result = subprocess.run([sys.executable,str(journey.ROOT/'scripts/monitor_sandbox.py'),folder],
                input='\n'.join(json.dumps(r) for r in requests)+'\n',text=True,capture_output=True,check=True)
            self.assertEqual(json.loads(json.loads(result.stdout.splitlines()[-1])['result']['content'][0]['text']),values['2026-10-18:site-b'])
            peer.read(root,'2026-10-18:site-b')
            with self.assertRaises(KeyError): peer.read(root,'missing')
            self.assertEqual(peer.state(root),['2026-10-18:site-b','2026-10-18:site-b','missing'])

    def test_lock_and_six_variants_preserve_original_six_day_world(self):
        variants = journey.variants()
        self.assertEqual(len(variants),6)
        self.assertEqual(variants[0]['text'],journey.INPUT)
        lock = json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text())
        self.assertEqual(lock,dict(fixture_sha256=journey.sha(journey.FIXTURE.read_bytes()),
            golden_sha256=journey.sha((journey.ROOT/'evals/golden-cases.md').read_bytes())))
        for v in variants:
            self.assertEqual(v['source']['daily_prices'],variants[0]['source']['daily_prices'])
            self.assertNotIn('Capabilities:',json.dumps(v))

    def test_judge_refuses_partial_graded_drifted_or_unclean_receipts(self):
        base = dict(case_id='GC-14', variants=[], cleanup=True, status='mechanical_pass', semantic_score=None,
                    fixture_lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text()))
        for change in [{},dict(semantic_score=100),dict(cleanup=False),dict(status='inconclusive'),dict(fixture_lock={})]:
            with self.assertRaises(RuntimeError): evaluator.prepare(dict(base,**change))

    def test_frozen_package_keeps_all_sixteen_original_blocks_and_files(self):
        lock = journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual(len(lock['frozen_blocks']),16)
        self.assertEqual({b['section'] for b in lock['frozen_blocks']},{'7.GC-14','8.15'})

    def test_identity_validation_rejects_the_reproduced_forged_provenance(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary=Path(tmp)/'binary';binary.write_bytes(b'explicit binary fixture')
            identity=dict(source_before={'sha':'a'*40},source_after={'sha':'a'*40},
                binary_path=str(binary),binary_sha256=journey.sha(binary.read_bytes()),environment={'go':'fixture'})
            report=dict(case_id='GC-14',status='mechanical_pass',scope='scripted_whole_case_mechanics',
                cleanup=True,semantic_score=None,eligible_for_grading=False,release_ready=False,
                boundary_lock=journey.load(),fixture_lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text()),identity=identity,
                source_provenance={'before':{'forged':True},'after':{'forged':True}},variants=[])
            with patch.object(journey,'source_identity',return_value={'sha':'a'*40}),patch.object(journey,'environment',return_value={'go':'fixture'}):
                for change in ({}, {'binary_sha256':'0'*64}, {'environment':{}}, {'source_after':{'sha':'b'*40}}):
                    value=copy.deepcopy(report);value['identity'].update(change)
                    with self.subTest(change=change),self.assertRaisesRegex(RuntimeError,'identity drift|provenance drift'):
                        journey.validate_report(value,binary)

    def test_native_manifest_rejects_corruption_summary_size_duplicates_and_escape(self):
        report=dict(status='mechanical_pass',variants=[],cleanup=True,
            identity=dict(source_before={'sha':'a'*40},binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'report.json';journey.write_report(path,report)
            self.assertEqual(journey.read_report(path),report)
            original=json.loads(path.read_text());raw=path.parent/original['native']['path'];data=raw.read_bytes()
            for name,change in [('hash',{'native':dict(original['native'],sha256='0'*64)}),
                                ('size',{'native_bytes':1}),('decoded_size',{'native_uncompressed_bytes':1}),
                                ('bool_version',{'version':True}),('summary',{'summary':dict(original['summary'],cleanup=1)}),
                                ('escape',{'native':dict(original['native'],path='../escape')})]:
                path.write_text(json.dumps(dict(original,**change)))
                with self.subTest(name=name),self.assertRaises((ValueError,RuntimeError)):
                    journey.read_report(path)
            path.write_text(json.dumps(original));raw.write_bytes(data+b'corrupt')
            with self.assertRaisesRegex(ValueError,'hash mismatch'):journey.read_report(path)
            raw.write_bytes(data);path.write_text('{"kind":"gc14_native_manifest","kind":"forged"}')
            with self.assertRaisesRegex(ValueError,'duplicate JSON key'):journey.read_report(path)
            path.write_text(json.dumps(original));raw.rename(raw.with_suffix('.saved'));raw.symlink_to(raw.with_suffix('.saved'))
            with self.assertRaisesRegex(ValueError,'symlink'):journey.read_report(path)


if __name__ == '__main__':
    unittest.main()
