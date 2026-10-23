"""GC-09 scripted whole-case assertions; never a semantic grader.

Uses the existing locked M1 fixture and CLI driver. Text checks recognize the
explicit Fake's short numbered agenda; arbitrary model prose still needs Judge.
"""
import json
import re


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
        no_email_calendar_action=bool(state['tools']) and all(t['tool_name'] == 'builtin__text__echo'
            and t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in state['tools']),
        one_completion=len(replies) == len(state['deliveries']) == 1 and state['deliveries'][0]['status'] == 'sent',
        replay_equal=state == execution.get('replay_state'),
    )
    return checks
