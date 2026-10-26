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
import platform
import selectors
import sqlite3
import subprocess
import sys
import tempfile

from case_runner import digest
from feishu_sandbox import Sandbox
from fixture_pool import run_cases
from meeting_sandbox import state as remote_state
from release_audit import inventory
from release_receipts import strict_json
from unscripted_cases import source_identity
from verify_gc08 import call_hash
from verify_channels import ChannelHarness
from verify_main import require

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT/'evals/fixtures/gc12-v1.json'
BOUNDARY_LOCK = ROOT/'evals/fixtures/gc12-boundary-v1.lock.json'
FIRST = '和 Alex 约下周 30 分钟聊 Erii，邮件简洁一点。'
STEER = '不要周一。'


def load():
    lock = strict_json(BOUNDARY_LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(hashlib.sha256((ROOT/name).read_bytes()).hexdigest() == expected, 'GC-12 lock drift: '+name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 19 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current for b in blocks),
        'GC-12 frozen source drift')
    return lock


def cases():
    rows = variants()
    rows.extend(dict(rows[0], id=mode, mode=mode) for mode in ('denied', 'lost-reply'))
    return rows


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


def execution_ids(row):
    """Identity sets distinguish independent journeys from replay snapshots."""
    value = row['state']
    result = {}
    for table, key in (('runs', 'run_id'), ('tools', 'invocation_id'),
                       ('generations', 'generation_id'), ('requests', 'delivery_id'),
                       ('conversation', 'message_id')):
        ids = [item[key] for item in value[table]]
        require(bool(ids) and all(type(i) is str and i for i in ids) and len(set(ids)) == len(ids),
                'invalid GC-12 execution identities: '+table)
        result[table] = ids
    return result


def independent_executions(rows):
    seen = {}
    for row in rows:
        for table, ids in execution_ids(row).items():
            require(seen.setdefault(table, set()).isdisjoint(ids), 'reused GC-12 execution identities: '+table)
            seen[table].update(ids)


def receipt_checks(row):
    value = row['state']; remote = row['remote']; variant = row['variant']
    empty = dict(mail=[], calendar=[], attempts=[])
    mode = variant.get('mode', 'success')
    checks = (observe(value, remote, value['approvals'][0], value['approvals'][-1], empty, row['remote_replay'])
              if mode == 'success' else safety_observe(value, remote, mode, empty, row['remote_replay']))
    checks.update(source_observe(value, variant))
    return checks


def boundary_checks(row):
    value = row['state']; steps = row['checkpoints']
    require([s['name'] for s in steps] == ['Monday_frozen_before_notice', 'Tuesday_frozen_before_consent', 'completed'],
            'GC-12 checkpoint inventory drift')
    before, pending, completed = steps
    old, new = value['approvals']
    empty = dict(mail=[], calendar=[], attempts=[])
    def approval_cut(step, expected):
        approvals = step['state']['approvals']
        effects = [t for t in step['state']['tools'] if t['tool_name'] == 'mcp__meeting__invite']
        return ([a['status'] for a in approvals] == expected and len(effects) == len(expected) and
                step['remote'] == empty and all(t['attempt_count'] == 0 for t in effects) and
                all(a['approval_id'] == final['approval_id'] and
                    a['frozen_arguments_hash'] == final['frozen_arguments_hash'] and
                    frozen(step['state'], a) == frozen(value, final)
                    for a, final in zip(approvals, (old, new))))
    models = {m['snapshot_id']:m for m in value['snapshots']}
    capabilities = {c['snapshot_id']:c for c in value['capabilities']}
    runs = {r['run_id']:r for r in value['runs']}
    return dict(
        monday_pending_without_effect=approval_cut(before, ['pending']),
        tuesday_pending_without_effect=approval_cut(pending, ['invalidated', 'pending']),
        completed_cut_exact=semantic_state(completed['state']) == semantic_state(value) and completed['remote'] == row['remote'],
        frozen_approval_hashes=all(a['frozen_arguments_hash'] == t['request_hash'] == call_hash(t)
            for a in value['approvals'] for t in value['tools'] if t['invocation_id'] == a['invocation_id']),
        frozen_generations=bool(value['generations']) and all(g['status'] == 'completed' and g['attempt_count'] == 1 and
            json.loads(g['request_json'])['model_snapshot']['snapshot_id'] == runs[g['run_id']]['model_snapshot_id']
            for g in value['generations']) and all(r['model_snapshot_id'] in models and
                r['capability_snapshot_id'] in capabilities and not capabilities[r['capability_snapshot_id']]['can_spawn_agents']
                for r in runs.values()),
        exact_replay=row['replay_digest'] == digest(semantic_state(row['replay_state'])) and
            semantic_state(value) == semantic_state(row['replay_state']),
    )


def validate_report(report, binary=None):
    require(report.get('case_id') == 'GC-12' and report.get('status') == 'mechanical_pass' and
        report.get('cleanup') is True and 'semantic_score' in report and report['semantic_score'] is None and
        report.get('fixture_lock') == load(), 'complete locked ungraded GC-12 report required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        bool(identity['environment']) and len(identity['binary_sha256']) == 64 and
        report['binary_sha256'] == identity['binary_sha256'], 'GC-12 source/environment identity drift')
    if binary is not None:
        require(hashlib.sha256(binary.read_bytes()).hexdigest() == identity['binary_sha256'], 'GC-12 binary drift')
    rows = report['variants']
    require([r['variant'] for r in rows] == cases(), 'GC-12 complete ordered variant inventory required')
    independent_executions(rows)
    for row in rows:
        checks = receipt_checks(row)
        require(row['assertions'] == checks and all(v is True for v in row['assertions'].values()) and
            all(boundary_checks(row).values()) and row['scope'] == 'scripted_whole_case_mechanics' and
            row['semantic_score'] is None and row['fixture_sha256'] == hashlib.sha256(FIXTURE.read_bytes()).hexdigest() and
            row['cleanup'] == dict(owned_processes_joined=True, http_peer_joined=True, disposable_root_removed=True) and
            all(v is True for v in row['cleanup'].values()), 'GC-12 failed, graded, unclean or drifted native receipt')
    return dict(case_id='GC-12', receipts=len(rows), frozen_blocks=19, cleanup=True, semantic_score=None,
                release_ready=False, source_sha=identity['source_before']['sha'], binary_sha256=identity['binary_sha256'])


def rejection_probes(report, binary, folder):
    """Corrupt real receipts; exercise the actual CLI and CI native consumer."""
    from ci_communication import native_receipt, check_receipts
    from verify_suite import Gate
    validate_report(report, binary)
    folder.mkdir(parents=True, exist_ok=True)
    path = folder/'gc12-1.json'; gate = Gate('gc12', 'verify_gc12.py', 'gc12-1', 1)
    mutations = dict(
        missing_variant=lambda r:r['variants'].pop(),
        reordered=lambda r:r['variants'].reverse(),
        empty_cleanup=lambda r:r['variants'][0].update(cleanup={}),
        false_cleanup=lambda r:r['variants'][0]['cleanup'].update(http_peer_joined=False),
        source=lambda r:r['identity'].update(source_after={}),
        binary=lambda r:r.update(binary_sha256='0'*64),
        fixture=lambda r:r['variants'][0].update(fixture_sha256='0'*64),
        replay=lambda r:r['variants'][0].update(replay_digest='0'*64),
        frozen_hash=lambda r:r['variants'][0]['state']['approvals'][0].update(frozen_arguments_hash='0'*64),
        pending_cut=lambda r:r['variants'][0]['checkpoints'][0]['state']['approvals'][0].update(status='approved'),
        remote=lambda r:r['variants'][0]['remote']['calendar'].clear(),
        score=lambda r:r.update(semantic_score=90),
        copied_execution=lambda r:r['variants'][1]['state']['runs'][0].update(run_id=r['variants'][0]['state']['runs'][0]['run_id']))
    results = {}
    for name, mutate in mutations.items():
        bad = copy.deepcopy(report); mutate(bad)
        path.write_text(json.dumps(bad, ensure_ascii=False)+'\n')
        cli = subprocess.run([sys.executable, str(ROOT/'scripts/verify_gc12.py'), '--binary', str(binary),
            '--evidence', str(path), '--validate'], capture_output=True, text=True, timeout=60)
        (folder/(name+'.stderr')).write_text(cli.stderr)
        require(cli.returncode != 0, 'GC-12 CLI accepted corruption: '+name)
        error = None
        try:
            receipt = native_receipt(gate, 1, folder, binary, {'binary_sha256':report['binary_sha256']})
            check_receipts([receipt], [gate])
        except (RuntimeError, ValueError, KeyError) as failure:
            error = str(failure)
        require(error is not None, 'GC-12 CI accepted corruption: '+name)
        results[name] = dict(cli_exit_code=cli.returncode, ci_rejected=True, reason=error)
    path.unlink()
    # Reusing a complete passing round is rejected by the compact CI join too.
    path.write_text(json.dumps(report, ensure_ascii=False)+'\n')
    receipt = native_receipt(gate, 1, folder, binary, {'binary_sha256':report['binary_sha256']})
    duplicate = copy.deepcopy(receipt); duplicate['attempt'] = 2
    try:
        check_receipts([receipt, duplicate], [Gate('gc12', gate.script, gate.report, 2)])
    except ValueError as error:
        require('reused or invalid GC-12 execution identities' in str(error), 'unexpected round rejection')
        results['copied_round'] = dict(ci_rejected=True, reason=str(error))
    require('copied_round' in results, 'GC-12 CI accepted copied round')
    path.unlink()
    (folder/'rejections.json').write_text(json.dumps(results, indent=2)+'\n')
    return results


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
            receipt['owned_processes_joined'] = all(child.poll() is not None for child in h.children)
            (evidence/(variant['id']+'.json')).write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+'\n')
    receipt['cleanup'] = dict(owned_processes_joined=receipt['owned_processes_joined'], http_peer_joined=not platform.thread.is_alive(), disposable_root_removed=not root.exists())
    (evidence/(variant['id']+'.json')).write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+'\n')
    return receipt


def run(binary, evidence):
    evidence.mkdir(parents=True, exist_ok=True)
    boundary_lock = load()
    identity = dict(source_before=source_identity(), binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
        environment=dict(platform=platform.platform(), python=platform.python_version(),
            node=subprocess.check_output(['node','--version'], text=True).strip(),
            go=subprocess.check_output(['go','version'], text=True).strip(),
            binary_build=subprocess.check_output(['go','version','-m',str(binary)], text=True)))
    lock = json.loads(FIXTURE.with_suffix('.lock.json').read_text())
    for path, expected in lock.items():
        require(hashlib.sha256((ROOT/path).read_bytes()).hexdigest() == expected, 'GC-12 locked source drift: '+path)
    results = run_cases(cases(), lambda v:drive(binary, evidence, v), 1)
    identity['source_after'] = source_identity()
    report = dict(case_id='GC-12', status='mechanical_pass', semantic_score=None, live_shadow='inconclusive',
        fixture_lock=boundary_lock, identity=identity, binary_sha256=identity['binary_sha256'], variants=results,
        cleanup=all(all(r['cleanup'].values()) for r in results),
        remaining=dict(parent='ERII-223', semantic_score=None, current_L2='unexecuted', release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-12; original inputs only; connected mail/calendar adapter still required',
            applicability='Synchronous Main only; no Task/Root/Child/Artifact. Task cancellation and old Task epochs are inapplicable; Steering, SIGKILL/restart, obsolete approval and unknown external effects are exercised.'))
    validate_report(report, binary)
    return report


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--evidence', type=Path, required=True)
    modes = p.add_mutually_exclusive_group()
    modes.add_argument('--validate', action='store_true')
    modes.add_argument('--validate-series', action='store_true', help='Recompute all five original gc12 rounds in an evidence directory')
    p.add_argument('--probe-rejections', action='store_true', help='With --validate, corrupt real receipts and require CLI/CI rejection')
    args = p.parse_args()
    if args.probe_rejections and not args.validate:
        p.error('--probe-rejections requires --validate')
    if args.validate_series:
        from ci_communication import native_receipt, check_receipts
        from verify_suite import SHARDS
        gate = next(g for g in SHARDS['communication'] if g.name == 'gc12')
        binary = args.binary.resolve()
        suite = dict(binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest())
        folder = args.evidence
        if not (folder/'gc12-1.json').exists():
            # verify_suite isolates focused output from complete CI inventories.
            key = hashlib.sha256(json.dumps(['gc12']).encode()).hexdigest()[:12]
            folder = folder/'focused'/key
        receipts = [native_receipt(gate, n, folder, binary, suite) for n in range(1, gate.attempts+1)]
        check_receipts(receipts, [gate])
        print(json.dumps(dict(case_id='GC-12', rounds=gate.attempts, receipts=sum(len(r['rows']) for r in receipts),
            cleanup=True, semantic_score=None, release_ready=False)))
        sys.exit(0)
    if args.validate:
        report = strict_json(args.evidence.read_bytes())
        result = validate_report(report, args.binary.resolve())
        if args.probe_rejections:
            result['rejection_probes'] = len(rejection_probes(report, args.binary.resolve(), args.evidence.with_suffix('.rejections')))
        print(json.dumps(result))
        sys.exit(0)
    args.evidence.unlink(missing_ok=True)
    result = run(args.binary.resolve(), args.evidence.resolve().with_suffix(''))
    args.evidence.write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps(dict(case='GC-12', variants=len(result['variants']), cleanup=result['cleanup'], semantic_score=None)))
