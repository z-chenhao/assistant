"""Offline preparation must minimize disclosure and keep scripted runs ungraded."""
import copy
import json
import unittest

import prepare_release_eval as prep
import quote_case as q


class PreparationTests(unittest.TestCase):
    def test_production_driver_packet_still_excludes_raw_state_and_fake_grades(self):
        from test_gc11_driver import DriverTests
        report=DriverTests().cohort()[0]
        report['state']['private_trace']='HIDDEN_PROVIDER_REQUEST'
        report['replay_state_digest']=q.digest(report['state'])
        prepared=prep.prepare(report)
        self.assertEqual(len(prepared['judge_packets']),1)
        self.assertFalse(prepared['judge_packets'][0]['eligible_for_grading'])
        self.assertNotIn('HIDDEN_PROVIDER_REQUEST',json.dumps(prepared))
        self.assertIsNone(prepared['scores'])
        report['identity']['execution_kind']='live_model';report['driver_mechanics_only']=True
        self.assertFalse(prep.prepare(report)['judge_packets'][0]['eligible_for_grading'])

    def report(self):
        data = json.loads((q.ROOT / 'evals/fixtures/m3-capabilities-v1.json').read_text())['insurance_quotes']
        texts = q.answers(data)
        rows = []
        for v in q.case_variants():
            value = dict(conversation=[m for e,t in zip(v['events'],texts) for m in
                         (dict(role='user',content=[dict(text=e['text']+' HIDDEN_PLAN')]),
                          dict(role='assistant',content=[dict(text=t)]))],
                         tools=[dict(tool_name=n,status='succeeded',attempt_count=1,
                                     result_json=json.dumps(dict(text=json.dumps(data)))) for n in
                                ['mcp__fixture__read','builtin__artifact__save','builtin__text__echo'] * 2],
                         task=[],approval=[],runs=[dict(kind='main',status='completed')]*2,
                         deliveries=[dict(message_id='1',status='sent'),dict(message_id='2',status='sent')],
                         generations=[dict(request_json='HIDDEN_TRACE')])
            artifacts = [dict(ref='artifact:test',text=t,sha256=q.digest_bytes(t)) for t in texts]
            checks = q.observations(value,v['events'],texts,artifacts,value,False,data)
            rows.append(dict(name='GC-11_two_turn_scripted_journey',variant=v,source=data,state=value,
                             artifacts=artifacts,assertions=checks,grading=dict(status='ungraded',final_score=None)))
        return dict(checks=rows,cleanup=True)

    def test_allowed_packet_only_and_no_grades_or_comparison_winner(self):
        result = prep.prepare(self.report())
        self.assertEqual(len(result['judge_packets']),6)
        self.assertTrue(all(not p['eligible_for_grading'] for p in result['judge_packets']))
        self.assertEqual(set(result['judge_packets'][0]['judge_input']),
                         {'goal','allowed_fixture','conversation','final_artifact_summary','programmatic_assertions'})
        self.assertNotIn('HIDDEN_PLAN',json.dumps(result))
        self.assertNotIn('HIDDEN_TRACE',json.dumps(result))
        self.assertIsNone(result['comparison']['scores'])
        self.assertIsNone(result['comparison']['winner'])
        self.assertEqual(result['comparison']['equal_conditions']['repetitions'],5)

    def test_incomplete_false_graded_or_drifted_evidence_fails(self):
        raw = self.report()
        for mutation in ('variant','cleanup','assertion','score','source','duplicate','bytes'):
            report = copy.deepcopy(raw)
            row = report['checks'][0]
            if mutation == 'variant': report['checks'].pop()
            if mutation == 'cleanup': report['cleanup'] = False
            if mutation == 'assertion': row['assertions'].pop('no_purchase_or_approval')
            if mutation == 'score': row['grading']['final_score'] = 100
            if mutation == 'source': row['source']['quotes'][1]['cashback_written'] = True
            if mutation == 'duplicate': report['checks'][-1] = copy.deepcopy(row)
            if mutation == 'bytes': row['artifacts'][0]['text'] = 'corrupt'
            with self.assertRaises(ValueError):
                prep.prepare(report)


if __name__ == '__main__':
    unittest.main()
