"""GC-09 scripted whole-case assertions; never a semantic grader.

Uses the existing locked M1 fixture and CLI driver. Text checks recognize the
explicit Fake's short numbered agenda; arbitrary model prose still needs Judge.
"""
import hashlib
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_steering(root=ROOT):
    lock = json.loads((root/'evals/fixtures/gc09-steering-v1.lock.json').read_text())
    for name, expected in lock['files'].items():
        if hashlib.sha256((root/name).read_bytes()).hexdigest() != expected:
            raise ValueError('GC-09 Steering source/contract drift: '+name)
    return json.loads((root/'evals/fixtures/gc09-steering-v1.json').read_text()), lock


def steering_event(variant):
    from case_runner import load_contract
    data, _, _ = load_contract(ROOT)
    wording = next(v['wording'] for v in data['variants'] if v['id'] == variant['variant_id'])
    fixture, _ = load_steering()
    return '当前时间：'+fixture['at']+'。'+fixture['wordings'][wording]


def steering_text_checks(text):
    fixture, _ = load_steering()
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    return dict(
        bounded_items=0 < len(lines) <= 8 and all(re.match(r'^'+str(i)+r'\.\s*\S', line)
            and len(line) <= 120 for i, line in enumerate(lines, 1)),
        no_completed_work=all(token not in text for token in fixture['completed']+['10:00', '13:30', '发布风险']),
        updated_preparation=bool(lines) and fixture['preparation_window'] in lines[0] and '准备14:00面试' in lines[0],
        personal_window=bool(lines) and fixture['personal_window'] in lines[0] and '私人事务' in lines[0],
        remaining_work=all(token in text for token in ('16:00', '可选', 'T2', '10月20日', 'E2', 'E3')),
        no_fabricated_effect=all(token not in text for token in ('已经发送', '已经取消', '已经创建')),
    )


def steering_observations(variant, execution):
    row = execution.get('steering')
    if not row:
        return dict(steering_receipt=False)
    before, after = execution['state'], row['state']
    _, lock = load_steering()
    event = steering_event(variant)
    replies = [m for m in after['conversation'] if m['role'] == 'assistant']
    checks = steering_text_checks(''.join(p.get('text', '') for p in replies[-1]['content']) if replies else '')
    old_runs = {r['run_id'] for r in before['runs']}
    fresh = [r for r in after['runs'] if r['run_id'] not in old_runs]
    frozen = row['before_kill']
    frozen_contexts = [c for c in frozen['compactions'] if c['run_id'] not in old_runs]
    requests = [json.loads(g['request_json']) for g in after['generations'] if g['run_id'] not in old_runs]
    generations = [g for g in after['generations'] if g['run_id'] not in old_runs]
    expected_users = [p['text'] for m in before['conversation'] if m['role'] == 'user' for p in m['content']]+[event]
    def retained(key, identity):
        index = {r[identity]: r for r in after[key]}
        return all(index.get(r[identity]) == r for r in before[key])
    checks.update(
        locked_event=row.get('fixture_lock') == lock and row.get('event') == event,
        ordered_input_and_retained_history=after['conversation'][:len(before['conversation'])] == before['conversation'] and
            [p['text'] for m in after['conversation'] if m['role'] == 'user' for p in m['content']] == expected_users,
        steering_in_actual_requests=len(requests) == 2 and all(
            [m['text'] for m in r['messages'] if m['role'] == 'user'] == expected_users for r in requests),
        frozen_model_and_tools=len(fresh) == 1 and len(requests) == 2 and all(
            r.get('model_snapshot', {}).get('snapshot_id') == fresh[0]['model_snapshot_id'] and
            r.get('model_snapshot') == requests[0].get('model_snapshot') and r.get('tools') == requests[0].get('tools')
            for r in requests),
        completed_model_result_matches_delivery=len(generations) == 2 and all(
            g['status'] == 'completed' and g['attempt_count'] == 1 for g in generations) and
            json.loads(generations[-1]['result_json'])['text'] == ''.join(p.get('text','') for p in replies[-1]['content']),
        one_new_main=len(fresh) == 1 and fresh[0]['kind'] == 'main' and fresh[0]['task_id'] is None and
            fresh[0]['status'] == 'completed' and len(after['runs']) == len(before['runs'])+1,
        no_new_task_schedule_approval_artifact=all(after.get(k) == before.get(k) == []
            for k in ('tasks','automations','approvals','artifacts')),
        only_safe_echo=len(after['tools']) == len(before['tools'])+2 and all(
            t['tool_name'] == 'mcp__fixture__echo' and t['status'] == 'succeeded' and t['attempt_count'] == 1
            and t['replay_policy'] == 'safe' for t in after['tools']),
        completed_calls_unchanged=retained('tools','invocation_id') and retained('generations','generation_id'),
        one_new_visible_delivery=len(replies) == 2 and len(after['conversation']) == len(before['conversation'])+2 and
            len(after['deliveries']) == 2 and {d['message_id'] for d in after['deliveries']} == {m['message_id'] for m in replies} and
            all(d['status'] == 'sent' for d in after['deliveries']) and retained('deliveries','delivery_id'),
        restart_after_context_freeze=row.get('restart') == dict(boundary='after_compaction_commit', returncode=-9),
        frozen_input_recovered=len(fresh) == 1 and row['receipt']['run_id'] == fresh[0]['run_id'] and
            len(frozen_contexts) == 1 and frozen_contexts[0]['step_index'] == 0 and
            [m['text'] for m in json.loads(frozen_contexts[0]['messages_json']) if m['role'] == 'user'] == expected_users and
            frozen_contexts[0] in after['compactions'] and frozen['generations'] == before['generations'] and
            any(r['run_id'] == fresh[0]['run_id'] and r['status'] == 'running' for r in frozen['runs']),
        replay_equal=after == row.get('replay_state'),
    )
    return checks


def text_checks(text):
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    items = [re.sub(r'^\d+\.\s*', '', line) for line in lines]
    first = items[0] if items else ''
    second = items[1] if len(items) > 1 else ''
    windows = re.findall(r'(\d{2}):(\d{2})[–-](\d{2}):(\d{2})', second)
    before_interview = any(8*60+30 <= int(a)*60+int(b) < int(c)*60+int(d) <= 14*60
                           for a, b, c, d in windows)
    return dict(
        bounded_short_items=0 < len(lines) <= 8 and all(
            re.match(r'^'+str(i)+r'\.\s*\S', line) and len(line) <= 120
            for i, line in enumerate(lines, 1)),
        release_first=all(token in first for token in ('E1', 'T1', '发布风险', '10:00周会前', '12:00前')),
        preparation_before_interview=before_interview and '准备' in second and '14:00面试' in second,
        confirmation_without_repeat='E4只核对，无需重复回复' in text,
        lower_priority_mail=all(token not in first+second for token in ('E2', 'E3', '新闻', '酒店')),
        no_fabricated_effect=all(token not in text for token in ('已经发送', '已经取消', '已经创建')),
    )


def observations(variant, execution):
    state = execution['state']
    users = [p['text'] for m in state['conversation'] if m['role'] == 'user' for p in m['content']]
    replies = [p['text'] for m in state['conversation'] if m['role'] == 'assistant' for p in m['content']]
    event = variant['events'][0]
    expected = [variant['context'], '当前时间：'+event['at']+'。'+event['text']]
    requests = [json.loads(g['request_json']) for g in state['generations']]
    checks = text_checks(replies[0] if len(replies) == 1 else '')
    checks.update(
        original_source_and_input=users == expected and bool(requests) and all(
            [m['text'] for m in r['messages'] if m['role'] == 'user'] == expected for r in requests),
        main_only=bool(state['runs']) and all(r['kind'] == 'main' and r['status'] == 'completed'
                                            and r['task_id'] is None for r in state['runs']),
        no_task_schedule_approval_artifact=all(state.get(k) == [] for k in ('tasks','automations','approvals','artifacts')),
        no_email_calendar_action=bool(state['tools']) and all(t['tool_name'] == 'mcp__fixture__echo'
            and t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in state['tools']),
        one_completion=len(replies) == len(state['deliveries']) == 1 and state['deliveries'][0]['status'] == 'sent',
        replay_equal=state == execution.get('replay_state'),
    )
    return checks
