"""GC-14 oracle rejects schedule, source, notification and replay drift."""
import copy
from datetime import datetime
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

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
            with self.assertRaises(ValueError): evaluator.prepare(dict(base,**change))


if __name__ == '__main__':
    unittest.main()
