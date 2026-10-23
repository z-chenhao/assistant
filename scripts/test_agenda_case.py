import copy
import json
import unittest
from pathlib import Path

import agenda_case as agenda
from case_runner import digest, load_contract, variants


class AgendaTests(unittest.TestCase):
    text = ('1. 先处理E1/T1发布风险说明，在10:00周会前或会后立即完成，12:00前确认。\n'
            '2. 13:30–14:00准备14:00面试，E4只核对，无需重复回复。\n'
            '3. E2新闻和已确认的E3酒店邮件稍后看；16:00分享可选，T2旅行手册稍后推进。')

    def fixture(self):
        data, _, _ = load_contract(Path(__file__).resolve().parents[1])
        case = next(c for c in data['cases'] if c['case_id'] == 'GC-09')
        return case, list(variants(data, case))

    def sample(self):
        v = self.fixture()[1][0]
        event = v['events'][0]
        users = [v['context'], '当前时间：'+event['at']+'。'+event['text']]
        messages = [dict(role='user', text=t) for t in users]
        state = dict(conversation=[dict(role='user', content=[dict(text=t)]) for t in users]+
                     [dict(role='assistant', content=[dict(text=self.text)])],
                     generations=[dict(request_json=json.dumps(dict(messages=messages)))],
                     runs=[dict(kind='main', status='completed', task_id=None)],
                     tools=[dict(tool_name='builtin__text__echo', status='succeeded', attempt_count=1)],
                     deliveries=[dict(status='sent')], tasks=[], automations=[], approvals=[], artifacts=[])
        return v, dict(state=state, replay_state=copy.deepcopy(state))

    def test_rejects_keyword_only_agenda_and_wrong_priority_deadline_or_window(self):
        self.assertTrue(all(agenda.text_checks(self.text).values()))
        legacy = '1. 先做发布风险说明，在10:00周会前或会后立即完成，12:00前确认。\n2. 14:00面试前留准备时间，E4只核对。\n3. 新闻和已确认的酒店邮件稍后看。'
        self.assertFalse(all(agenda.text_checks(legacy).values()))
        for text in [self.text.replace('12:00前', '13:00前'), self.text.replace('13:30–14:00', '14:30–15:00'),
                     self.text.replace('E1/T1', 'E2/E3'), self.text.replace('无需重复回复', '已经发送回复'),
                     self.text+'\n'+'\n'.join(f'{i}. 多余条目' for i in range(4, 10)),
                     self.text.replace('2. ', '2. 酒店新闻优先：')]:
            with self.subTest(text=text): self.assertFalse(all(agenda.text_checks(text).values()))

    def test_rejects_injected_source_side_effect_task_duplicate_and_replay_drift(self):
        v, execution = self.sample()
        self.assertTrue(all(agenda.observations(v, execution).values()))
        for key in ('tasks', 'automations', 'approvals', 'artifacts'):
            bad = copy.deepcopy(execution); bad['state'][key].append(dict(id='unexpected'))
            bad['replay_state'] = copy.deepcopy(bad['state'])
            self.assertFalse(all(agenda.observations(v, bad).values()))
        for mutation in ('request', 'input', 'tool', 'delivery', 'replay', 'worker'):
            bad = copy.deepcopy(execution)
            if mutation == 'request': bad['state']['generations'][0]['request_json'] = '{"messages":[]}'
            elif mutation == 'input': bad['state']['conversation'][0]['content'][0]['text'] += ' hidden plan'
            elif mutation == 'tool': bad['state']['tools'][0]['tool_name'] = 'mcp__mail__send'
            elif mutation == 'delivery': bad['state']['deliveries'] *= 2
            elif mutation == 'worker': bad['state']['runs'][0]['kind'] = 'worker'
            else: bad['replay_state']['tools'][0]['attempt_count'] = 2
            if mutation != 'replay': bad['replay_state'] = copy.deepcopy(bad['state'])
            with self.subTest(mutation=mutation): self.assertFalse(all(agenda.observations(v, bad).values()))

    def test_all_locked_variants_retain_full_source_and_original_event(self):
        case, values = self.fixture()
        self.assertEqual(len(values), 6)
        self.assertEqual(len({v['events'][0]['text'] for v in values}), 3)
        self.assertEqual(sum(v['noise_characters'] > 0 for v in values), 2)
        self.assertEqual(len(case['world']['calendar']), 3)
        self.assertEqual(len(case['world']['emails']), 4)
        self.assertEqual(len(case['world']['tasks']), 2)
        for v in values:
            for token in ('T1','T2','E1','E2','E3','E4','16:00-16:30'):
                self.assertIn(token, v['context'])
            self.assertNotIn('Capabilities:', v['context'])

    def packets(self):
        from run_cases import run_suite
        data, policy, lock = load_contract(Path(__file__).resolve().parents[1])
        case, _ = self.fixture()
        def drive(_, variant):
            event = variant['events'][0]
            users = [variant['context'], '当前时间：'+event['at']+'。'+event['text']]
            state = self.sample()[1]['state']
            state['conversation'] = [dict(role='user', content=[dict(type='text', text=t)]) for t in users]+[
                dict(role='assistant', content=[dict(type='text', text=self.text)])]
            for i, m in enumerate(state['conversation'], 1): m['sequence'] = i
            state['runs'][0].update(run_id='r', parent_run_id=None, model_snapshot_id='m', capability_snapshot_id='c')
            state['snapshots'] = [dict(snapshot_id='m')]
            state['capabilities'] = [dict(snapshot_id='c', can_spawn_agents=False,
                tools_json=json.dumps([dict(name='builtin__text__echo', permission='read_only')]))]
            request = dict(run_id='r', model_snapshot=dict(snapshot_id='m'), tools=[],
                           messages=[dict(role='user', text=t) for t in users])
            state['generations'] = [dict(status='completed', request_json=json.dumps(request)) for _ in range(2)]
            state['tools'] = [dict(tool_name='builtin__text__echo', status='succeeded', replay_policy='safe',
                                   attempt_count=1) for _ in range(2)]
            state['usage'] = [{}, {}]
            state['deliveries'][0]['message_id'] = 'reply'
            return dict(state=state, replay_state=copy.deepcopy(state), started_at='start', finished_at='end',
                        wall_time_ms=1, cleanup=dict(owned_processes_joined=True),
                        private_trace='PRIVATE_TRACE', private_provider='PRIVATE_PROVIDER')
        report = run_suite(dict(data, cases=[case]), policy, drive)
        report.update(contract_lock=lock, dataset_digest=digest(data),
                      cleanup_checks=dict(owned_processes_joined=True, disposable_root_removed=True))
        return report

    def test_judge_recomputes_receipts_and_never_grades_or_leaks_private_fields(self):
        from prepare_release_eval import prepare
        report = self.packets()
        preview = prepare(report)
        self.assertEqual(len(preview['judge_packets']), 6)
        self.assertTrue(all(p['eligible_for_grading'] is False for p in preview['judge_packets']))
        self.assertIsNone(preview['scores'])
        self.assertIsNone(preview['winner'])
        self.assertNotIn('PRIVATE_', json.dumps(preview))
        for mutation in ('missing', 'graded', 'cleanup', 'replay', 'input', 'lock', 'assertion'):
            bad = copy.deepcopy(report)
            row = bad['results'][0]
            if mutation == 'missing': bad['results'].pop()
            elif mutation == 'graded': row['final_score'] = 100
            elif mutation == 'cleanup': bad['cleanup_checks']['disposable_root_removed'] = False
            elif mutation == 'replay': row['evidence']['replay_state']['tools'][0]['attempt_count'] = 2
            elif mutation == 'input': row['evidence']['state']['conversation'][0]['content'][0]['text'] += ' plan'
            elif mutation == 'lock': bad['contract_lock'] = {}
            else: row['assertions']['release_first'] = False
            with self.subTest(mutation=mutation), self.assertRaises(ValueError): prepare(bad)


if __name__ == '__main__': unittest.main()
