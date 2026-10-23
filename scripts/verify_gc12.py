#!/usr/bin/env python3
"""Exact GC-12 source/turns through ChannelHarness/Main/Security/stdio MCP.

Explicit scripted model and independent mail/calendar Fake: no real accounts,
model-quality scores or L2 claim. Owns Launch/Doctor/Drive/Evidence/Cleanup.
"""
import argparse
import copy
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
from pathlib import Path
import selectors
import sqlite3
import subprocess
import sys
import tempfile

from case_runner import digest
from feishu_sandbox import Sandbox
from fixture_pool import run_cases
from meeting_sandbox import state as remote_state
from verify_channels import ChannelHarness
from verify_main import require

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT/'evals/fixtures/gc12-v1.json'
FIRST = '和 Alex 约下周 30 分钟聊 Erii，邮件简洁一点。'
STEER = '不要周一。'


def variants():
    source = json.loads(FIXTURE.read_text())
    result = []
    for name, first in [('base', FIRST), ('wording-1', '下周想和 Alex 聊 Erii，约 30 分钟，邮件写简短些。'),
                        ('wording-2', '请用简洁邮件和 Alex 约下周半小时讨论 Erii。')]:
        result.append(dict(id=name, source=copy.deepcopy(source), first=first, steer=STEER))
    for name in ('contact-order', 'slot-order', 'unrelated-contact'):
        data = copy.deepcopy(source)
        if name == 'contact-order': data['contacts'].reverse()
        elif name == 'slot-order':
            for slots in data['free_slots'].values(): slots.reverse()
        else: data['contacts'].insert(0, dict(name='Alex Smith', email='alex.smith@example.test', relation='不相关联系人'))
        result.append(dict(id=name, source=data, first=FIRST, steer=STEER))
    return result


def native(h):
    value = h.state()
    with sqlite3.connect(h.root/'data/state.db') as db:
        db.row_factory = sqlite3.Row
        for name, query in dict(approvals='SELECT * FROM approval ORDER BY created_at',
                                requests='SELECT * FROM delivery_request ORDER BY created_at',
                                tasks='SELECT * FROM task', artifacts='SELECT * FROM artifact').items():
            value[name] = [dict(r) for r in db.execute(query)]
        errors = dict(db.execute('SELECT invocation_id,error_json FROM tool_invocation'))
        for invocation in value['tools']:
            invocation['error_json'] = errors[invocation['invocation_id']]
    return value


def frozen(value, approval):
    return json.loads(next(t['arguments_json'] for t in value['tools'] if t['invocation_id'] == approval['invocation_id']))


def semantic_state(value):
    result = copy.deepcopy(value)
    for request in result['requests']:
        if request['status'] == 'unknown':
            # Query-only reconciliation records scan metadata after restart.
            # Status, send attempt count, identities and frozen payload stay exact.
            request.pop('updated_at', None)
            request.pop('version', None)
    return result


def observe(value, remote, old, current, before, after):
    action = frozen(value, current)
    tools = [t for t in value['tools'] if t['tool_name'] == 'mcp__meeting__invite']
    completed = [t for t in tools if t['status'] == 'succeeded']
    readbacks = [t for t in value['tools'] if t['tool_name'] == 'mcp__meeting__read' and
                 json.loads(t['arguments_json'])['resource'] == 'status']
    replies = [m for m in value['conversation'] if m['role'] == 'assistant']
    text = '\n'.join(p.get('text', '') for m in replies for p in m['content'])
    mail, calendar = remote['mail'], remote['calendar']
    checks = dict(
        exact_recipient=action['recipient'] == 'alex.chen@example.test',
        exact_tuesday=action['start'] == '2026-10-20T11:00:00+08:00' and
                      action['end'] == '2026-10-20T11:30:00+08:00' and action['timezone'] == 'Asia/Shanghai',
        frozen_content_and_actions=action['subject'] == 'Erii review' and 0 < len(action['body']) < 120 and
            'Erii' in action['body'] and action['action'] == 'send_mail_and_create_calendar',
        monday_invalidated=old['status'] == 'invalidated' and current['status'] == 'approved' and
            '2026-10-19T15:00:00' in frozen(value, old)['start'] and
            old['frozen_arguments_hash'] != current['frozen_arguments_hash'],
        no_unapproved_dispatch=before == dict(mail=[], calendar=[], attempts=[]) and
            len(tools) == 2 and tools[0]['attempt_count'] == 0 and len(completed) == 1 and completed[0]['attempt_count'] == 1 and
            all(t['replay_policy'] == 'never' for t in tools),
        one_mail_and_calendar=len(mail) == len(calendar) == len(remote['attempts']) == 1 and
            mail[0] == dict(recipient=action['recipient'], subject=action['subject'], body=action['body'], status='sent') and
            calendar[0] == dict(attendee=action['recipient'], title=action['subject'], start=action['start'],
                                end=action['end'], timezone=action['timezone'], status='confirmed') and remote['attempts'][0] == action,
        actual_status_readback=len(readbacks) == 1 and readbacks[0]['status'] == 'succeeded' and
            json.loads(json.loads(readbacks[0]['result_json'])['text']) == remote,
        one_completion_notice=sum(d['intent'] == 'completion' for d in value['requests']) == 1 and
            '邮件 1 封、日历会议 1 个' in text and 'alex.chen@example.test' in text and '2026-10-20T11:00:00+08:00' in text,
        duplicate_callback_and_restart=before is not None and after == remote,
        terminal_main_only=all(r['kind'] == 'main' and r['status'] == 'completed' for r in value['runs']) and
            not value['tasks'] and not value['artifacts'],
        interrupted_notice_query_only=[d['status'] for d in value['requests']] == ['unknown', 'sent', 'sent'],
    )
    return checks


def safety_observe(value, remote, mode, before, after):
    effects = [t for t in value['tools'] if t['tool_name'] == 'mcp__meeting__invite']
    final = effects[-1]
    error = json.loads(final['error_json']) if final['error_json'] else {}
    return dict(
        obsolete_approval_rejected=value['approvals'][0]['status'] == 'invalidated' and effects[0]['attempt_count'] == 0,
        approval_rejection_or_unknown=final['replay_policy'] == 'never' and
            (mode == 'denied' and final['status'] == 'failed' and final['attempt_count'] == 0 and
             error.get('code') == 'APPROVAL_DENIED' and value['approvals'][-1]['status'] == 'denied' or
             mode == 'lost-reply' and final['status'] == 'unknown' and final['attempt_count'] == 1 and
             error.get('code') == 'UNKNOWN_SIDE_EFFECT' and value['approvals'][-1]['status'] == 'approved'),
        no_premature_dispatch=before == dict(mail=[], calendar=[], attempts=[]),
        remote_effect_count=all(len(remote[k]) == (0 if mode == 'denied' else 1) for k in ('mail','calendar','attempts')),
        restart_never_redispatches=remote == after and len(effects) == 2,
        one_terminal_delivery=sum(d['intent'] == 'completion' and d['status'] == 'sent' for d in value['requests']) == 1 and
            all(r['status'] == 'completed' for r in value['runs']),
    )


def source_observe(value, variant):
    calls = [t for t in value['tools'] if t['tool_name'] == 'mcp__meeting__read' and
             json.loads(t['arguments_json'])['resource'] == 'source']
    return dict(actual_source_transport=len(calls) == 2 and all(
        json.loads(json.loads(t['result_json'])['text']) == variant['source'] for t in calls),
        original_two_inputs=[p['text'] for m in value['conversation'] if m['role'] == 'user'
                             for p in m['content']] == [variant['first'], variant['steer']])


def drive(binary, evidence, variant):
    mode = variant.get('mode', 'success')
    with tempfile.TemporaryDirectory(prefix='erii-gc12-') as temp, Sandbox() as platform:
        root = Path(temp)
        provider = root/'provider'
        provider.mkdir()
        # MCP's filesystem sandbox permits only its provider directory. Keep
        # the peer source there; never loosen it to read the workspace/data DB.
        (provider/'meeting_sandbox.py').write_bytes((ROOT/'scripts/meeting_sandbox.py').read_bytes())
        (provider/'source.json').write_text(json.dumps(variant['source'], ensure_ascii=False))
        h = ChannelHarness(binary, root, platform, 'meeting')
        with h.config.open('a') as config:
            # The shipped macOS sandbox allows system/Xcode runtimes, not an
            # arbitrary Homebrew interpreter's framework outside this scope.
            interpreter = subprocess.run(['xcrun', '--find', 'python3'], text=True, capture_output=True,
                                         check=True).stdout.strip() if sys.platform == 'darwin' else '/usr/bin/python3'
            config.write('mcp:\n  - name: meeting\n    command: '+json.dumps(interpreter)+
                         '\n    directory: '+json.dumps(str(provider))+'\n    args: ['+
                         json.dumps(str(provider/'meeting_sandbox.py'))+', '+json.dumps(str(provider))+
                         ']\n    tools: {read: read_only, invite: external_effect}\n')
        receipt = dict(variant=variant, scope='scripted_whole_case_mechanics', semantic_score=None,
                       fixture_sha256=hashlib.sha256(FIXTURE.read_bytes()).hexdigest(), checkpoints=[])
        try:
            h.setup()
            h.start('before_delivery_send')
            h.callback('gc12-first', variant['first'])
            with selectors.DefaultSelector() as ready:
                ready.register(h.process.stdout, selectors.EVENT_READ)
                require(ready.select(20), 'Monday approval did not reach pre-delivery boundary')
            require(json.loads(h.process.stdout.readline())['boundary'] == 'before_delivery_send', 'wrong approval boundary')
            value = native(h)
            old = value['approvals'][0]
            require(old['status'] == 'pending', 'Monday approval missing')
            before = remote_state(provider)
            require(before == dict(mail=[], calendar=[], attempts=[]), 'side effect before approval')
            receipt['checkpoints'].append(dict(name='Monday_frozen_before_notice', state=value, remote=before))
            h.http('/api/message', dict(source_id='gc12-steer', content=[dict(type='text', text=variant['steer'])]))
            h.callback('obsolete-approval', '/approve '+old['approval_id']+' '+old['frozen_arguments_hash'], expected=409)
            # Real server SIGKILL while first approval transport is held. New
            # input is already durable; restart must not revive Monday consent.
            h.stop(kill=True)
            h.start()
            h.wait(lambda s:len(s['approvals']) == 2 and s['approvals'][-1]['status'] == 'pending', 'Tuesday frozen approval after restart')
            value = native(h)
            approval = value['approvals'][-1]
            require(value['approvals'][0]['status'] == 'invalidated', 'Monday consent survived Steering')
            require(remote_state(provider) == before, 'Steering or restart dispatched invitation')
            receipt['checkpoints'].append(dict(name='Tuesday_frozen_before_consent', state=value, remote=before))
            h.http('/approval', data=('approval_id='+approval['approval_id']+'&hash=wrong&decision=approved').encode(),
                   headers={'Content-Type':'application/x-www-form-urlencoded'}, expected=409)
            if mode == 'lost-reply': (provider/'drop-reply').touch()
            command = ('/deny ' if mode == 'denied' else '/approve ')+approval['approval_id']+' '+approval['frozen_arguments_hash']
            with ThreadPoolExecutor(max_workers=6) as pool:
                list(pool.map(lambda n:h.callback('consent-'+str(n%3), command, event='consent-event-'+str(n)), range(6)))
            h.wait_state(lambda s:all(r['status'] == 'completed' for r in s['runs']) and
                         [d['status'] for d in s['requests']] == ['unknown', 'sent', 'sent'],
                         'verified invitation, completion and query-only interrupted notice', reader=native, timeout=20)
            value = native(h)
            remote = remote_state(provider)
            receipt['checkpoints'].append(dict(name='completed', state=value, remote=remote))
            h.stop()
            h.start()
            h.callback('gc12-first', variant['first'], event='first-retry-after-restart')
            h.http('/api/message', dict(source_id='gc12-steer', content=[dict(type='text', text=variant['steer'])]))
            h.callback('last-consent', command)
            replay = native(h)
            require(semantic_state(value) == semantic_state(replay), 'callback/input replay created or changed semantic state')
            remote_replay = remote_state(provider)
            checks = (observe(value, remote, value['approvals'][0], value['approvals'][-1], before, remote_replay)
                      if mode == 'success' else safety_observe(value, remote, mode, before, remote_replay))
            checks.update(source_observe(value, variant))
            receipt.update(assertions=checks, state=value, replay_state=replay, remote=remote,
                           remote_replay=remote_replay,
                           platform=platform.evidence(), replay_digest=digest(semantic_state(replay)))
            require(all(checks.values()), 'GC-12 assertions: '+json.dumps(checks))
        except BaseException:
            receipt['failure_state'] = h.failure_state()
            receipt['failure_native'] = native(h)
            raise
        finally:
            h.stop()
            h.cleanup()
            receipt['owned_processes_joined'] = True
            (evidence/(variant['id']+'.json')).write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+'\n')
    receipt['cleanup'] = dict(owned_processes_joined=True, http_peer_joined=not platform.thread.is_alive(), disposable_root_removed=not root.exists())
    (evidence/(variant['id']+'.json')).write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+'\n')
    return receipt


def run(binary, evidence):
    evidence.mkdir(parents=True, exist_ok=True)
    lock = json.loads(FIXTURE.with_suffix('.lock.json').read_text())
    for path, expected in lock.items():
        require(hashlib.sha256((ROOT/path).read_bytes()).hexdigest() == expected, 'GC-12 locked source drift: '+path)
    cases = variants()
    for mode in ('denied', 'lost-reply'):
        cases.append(dict(cases[0], id=mode, mode=mode))
    results = run_cases(cases, lambda v:drive(binary, evidence, v), 1)
    return dict(case_id='GC-12', status='mechanical_pass', semantic_score=None, live_shadow='inconclusive',
                binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), variants=results,
                cleanup=all(all(r['cleanup'].values()) for r in results))


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--evidence', type=Path, required=True)
    args = p.parse_args()
    result = run(args.binary.resolve(), args.evidence.resolve().with_suffix(''))
    args.evidence.write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps(dict(case='GC-12', variants=len(result['variants']), cleanup=result['cleanup'], semantic_score=None)))
