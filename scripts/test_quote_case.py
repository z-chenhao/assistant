"""Exact quote source and journey evidence cannot silently lose obligations."""
import copy
import json
import unittest

import quote_case as q


class QuoteTests(unittest.TestCase):
    def setUp(self):
        self.data = json.loads((q.ROOT / 'evals/fixtures/m3-capabilities-v1.json').read_text())['insurance_quotes']

    def test_locked_six_variants_keep_two_causal_turns_and_source(self):
        variants = q.case_variants()
        self.assertEqual(len(variants), 6)
        self.assertEqual(len({v['variant_id'] for v in variants}), 6)
        self.assertEqual(sum(v['noise_characters'] > 0 for v in variants), 2)
        for v in variants:
            self.assertEqual(len(v['events']), 2)
            self.assertLess(v['events'][0]['at'], v['events'][1]['at'])
        first, second = q.answers(self.data)
        for fact in ('5150.00', '5038.49', '5738.49', '5680.00', '45%', '35%', '20%', '未写入合同', '车辆检测'):
            self.assertIn(fact, first)
        for fact in ('书面活动规则', '付款', '到账时间', '投诉路径', '不能保证追回'):
            self.assertIn(fact, second)

    def test_missing_service_cashback_certainty_price_or_weights_fail(self):
        for mutate in (lambda d: d['quotes'][1]['services'].pop(),
                       lambda d: d['quotes'][1].update(cashback_written=True),
                       lambda d: d['quotes'][1].update(premium_cny=5038.49),
                       lambda d: d['weights'].update(reliability=0)):
            data = copy.deepcopy(self.data)
            mutate(data)
            with self.assertRaises(ValueError):
                q.answers(data)

    def test_absent_or_extra_turn_tool_artifact_effect_or_replay_rejects(self):
        # Minimal independent readback fixture. Mutations exercise the result
        # validator rather than merely repeating the plan builder's output.
        events = [dict(text='first'), dict(text='second')]
        texts = ['reply1', 'reply2']
        value = dict(conversation=[m for e,t in zip(events,texts) for m in
                     (dict(role='user', content=[dict(text=e['text'])]),
                      dict(role='assistant', content=[dict(text=t)]))],
                     tools=[dict(tool_name=n, status='succeeded', attempt_count=1,
                                 result_json=json.dumps(dict(text=json.dumps(self.data)))) for n in
                            ['mcp__fixture__read','builtin__artifact__save','builtin__text__echo'] * 2],
                     task=[], approval=[], runs=[dict(kind='main', status='completed')] * 2,
                     deliveries=[dict(message_id='1',status='sent'),dict(message_id='2',status='sent')])
        artifacts = [dict(text=t, sha256=q.digest_bytes(t)) for t in texts]
        self.assertTrue(all(q.observations(value, events, texts, artifacts, value, False, self.data).values()))
        for kind in ('turn', 'tool', 'artifact', 'effect', 'approval', 'replay', 'delivery', 'source', 'order', 'unsent'):
            v, a, replay = copy.deepcopy(value), copy.deepcopy(artifacts), copy.deepcopy(value)
            if kind == 'turn': v['conversation'].pop()
            if kind == 'tool': v['tools'].append(dict(tool_name='mcp__fixture__send',status='succeeded',attempt_count=1))
            if kind == 'artifact': a[1]['text'] = 'wrong bytes'
            if kind == 'approval': v['approval'].append(dict(status='pending'))
            if kind == 'replay': replay['tools'].pop()
            if kind == 'delivery': v['deliveries'][1]['message_id'] = '1'
            if kind == 'source': v['tools'][0]['result_json'] = json.dumps(dict(text='{}'))
            if kind == 'order': v['conversation'][1:3] = reversed(v['conversation'][1:3])
            if kind == 'unsent': v['deliveries'][0]['status'] = 'pending'
            self.assertFalse(all(q.observations(v, events, texts, a, replay, kind == 'effect', self.data).values()), kind)


if __name__ == '__main__':
    unittest.main()
