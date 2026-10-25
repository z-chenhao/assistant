#!/usr/bin/env python3
"""GC-02 original inputs, versioned goals and recurring reviews; mechanics only.

Uses existing scripted capabilities, CLI/TUI, MCP, Curator, QuickJS and Actor.
No natural-language understanding, value ranking or model quality is graded.
"""
import argparse
import copy
from datetime import datetime
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import tempfile
import threading

from case_runner import load_contract, variants
from fixture_pool import run_cases
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_m6_cases import (personal, state, execute, save_report, remember,
                          open_bytes, no_external_writes)
from terminal_driver import Terminal
from unscripted_cases import source_identity
from verify_capabilities import call
from verify_code_recovery import read
from verify_main import require
from verify_memory_maintenance import manage, work, readback
from verify_recall import Embeddings
from verify_tasks import state as task_state, assertions as task_assertions

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / 'evals/fixtures/gc02-journey-v1.json'
REPEATS = 5


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = json.loads(FIXTURE.with_suffix('.lock.json').read_text())
    for name, expected in lock['files'].items():
        require(sha((ROOT / name).read_bytes()) == expected, 'GC-02 lock drift: ' + name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT / 'evals/golden-cases.md').read_text())}
    require(len(lock['frozen_blocks']) == len({b['line'] for b in lock['frozen_blocks']}) == 24 and
        all(b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current
            for b in lock['frozen_blocks']), 'GC-02 frozen block drift')
    return json.loads(FIXTURE.read_text()), lock


def inputs(variant_id=None):
    data, policy, lock = load_contract(ROOT, 'm6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-02')
    if variant_id is None:
        return data, case
    return case, next(v for v in variants(data, case) if v['variant_id'] == variant_id)


def native(h):
    value = state(h)
    value.update(task_state(h))
    db = h.root / 'data/state.db'
    for table in ('automation', 'automation_fire'):
        value[table] = read(db, 'SELECT * FROM ' + table + ' ORDER BY rowid')
    value['automation_inbox'] = read(db, "SELECT * FROM event_inbox WHERE consumer='automation_actor' ORDER BY rowid")
    return value


def kill(h, boundary, command=('chat', 'work')):
    child = h.hold(boundary, command=command)
    before = native(h)
    child.kill(); child.communicate(timeout=10)
    return dict(boundary=boundary, returncode=child.returncode, state=before)


def artifact_readback(h):
    rows = []
    for descriptor in native(h)['artifact']:
        raw = open_bytes(h, descriptor['artifact_ref'])
        rows.append(dict(descriptor=descriptor, text=raw.decode(), sha256=sha(raw), bytes=len(raw)))
    return rows


def review_source():
    # Query current durable Memory at *each occurrence*. No corrected content or
    # Memory ID is embedded in the schedule, so it cannot replay a stale answer.
    support = json.dumps(load()[0]['support'],ensure_ascii=False)
    return save_report('const support=' + support + ';\n' + '''const goals=(await tools.builtin__memory__search({query:"人生 独立开发",limit:20})).filter(h=>h.source_type==="memory");
const result={goals,support,evidence_refs:goals.map(h=>h.source_ref),ranking:null,scope:"scripted current-memory readback; values remain the user's decision"};''', 'goals-review.json')


def goal_recall(h, key):
    # Keep the real mixed-source ToolInvocation as evidence, but do not echo
    # Conversation/code plans back into later model replies and retrieval.
    observed = execute(h,key,'return (await tools.builtin__memory__search({query:"人生 独立开发",limit:20})).filter(h=>h.source_type==="memory");')
    return observed


def journey(h, variant_id, row):
    fixture, _ = load()
    case, variant = inputs(variant_id)
    row['variant'] = variant
    world = case['world']
    row['core_ref'] = remember(h, 'initial-core', world['memory'][0]['content'])
    row['old_ref'] = remember(h, 'initial-exploration', world['memory'][1]['content'])
    row['initial_recall'] = goal_recall(h,'initial-goals')
    row['initial_hits'] = row['initial_recall']['value']
    work(h)
    row['initial'] = native(h)
    def select(bridge):
        h.bridge = bridge
        h.setup(metadata=dict(name='GC-02 explicit ' + bridge, provider_kind='openai_compatible',
            endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions', model_id='gc02-' + bridge + '-fixture',
            context_window=65536, max_output_tokens=4096, generation_defaults={},
            compatibility=dict(tool_result_format='openai'), capabilities=dict(text_input=True,image_input=True)))
    select('terminal-input')
    h.send('context', variant['context'])
    def send_event(index):
        event = variant['events'][index]
        select('terminal-input')
        receipt = h.send('event-' + str(index), event['at'] + ' ' + event['text'])
        h.command('chat', 'work'); work(h)
        return receipt
    row['first_receipt'] = send_event(0)
    select('capabilities')
    row['resumed'] = execute(h, 'resume', save_report('''const world=await tools.mcp__fixture__read({text:"world"});
const session=world.conversation_session_2026_09_20;
const result={resume:session.unanswered_question,confirmed:session.confirmed,exploratory:session.exploratory,
answered_income:null,ranking:null};''', 'interview-resume.json'))
    select('terminal-input')
    with Terminal(h.args('tui'), h.env, h.root) as terminal:
        terminal.expect('State: ready'); terminal.expect('You> ')
        event = variant['events'][1]
        terminal.send(event['at'] + ' ' + event['text'])
        terminal.expect('assistant: '); terminal.expect('You> ')
        require(terminal.stop() == 0, 'GC-02 TUI failed to stop')
        row['transcript'] = terminal.transcript()
    select('capabilities')
    row['constraint_call'] = call(h, 'constraints', 'builtin__memory__manage',
        dict(operation='remember', content=fixture['constraint']))
    work(h)
    send_event(2)
    receipt = manage(h, 'correct-goal', operation='remember', content=fixture['current'], supersedes=row['old_ref'])
    row['memory_event_id'] = receipt['receipt'].removeprefix('memory-event:')
    row['memory_restart'] = kill(h, 'after_memory_commit:' + row['memory_event_id'], ('memory', 'work'))
    work(h)
    records = readback(h)['memory']['records']
    row['current_ref'] = 'memory:' + next(r['memory_id'] for r in records if r['memory_id'].startswith(row['memory_event_id']))
    select('capabilities')
    row['corrected_recall'] = goal_recall(h, 'current-goal')
    row['corrected_hits'] = row['corrected_recall']['value']
    work(h)
    row['before_request'] = native(h)
    send_event(3)
    select('capabilities')
    instruction = '每月复盘当前人生目标；不替用户排序，不联系第三方。\nCapabilities: ' + json.dumps([
        dict(name='builtin__codemode__execute', arguments=dict(source=review_source()))], ensure_ascii=False)
    row['schedule_request'] = dict(operation='create', instruction=instruction,
        triggerType='cron', triggerSpec='0 20 * * 0L', timezone='Asia/Shanghai')
    plan = 'Capabilities: ' + json.dumps([dict(name='builtin__automation__manage', arguments=row['schedule_request'])], ensure_ascii=False)
    row['schedule_receipt'] = h.send('monthly', plan)
    h.command('chat', 'work'); work(h)
    row['schedule_views'] = json.loads(h.command('automation', 'list')['text'])
    row['scheduled'] = native(h)
    require(h.send('monthly', plan) == row['schedule_receipt'], 'duplicate schedule input drift')
    h.command('chat', 'work'); work(h)
    row['schedule_replayed'] = native(h)
    row['reviews'] = []
    for index, date in enumerate(fixture['review_dates']):
        h.automation_now = date
        boundary = 'after_automation_fire_commit' if index == 0 else 'after_report_commit'
        restarted = kill(h, boundary, ('automation', 'tick') if index == 0 else ('chat', 'work'))
        h.command('chat', 'work'); work(h)
        snapshot = native(h)
        h.command('chat', 'work'); work(h)
        row['reviews'].append(dict(at=date, restart=restarted, state=snapshot, replayed=native(h)))
    row['artifacts'] = artifact_readback(h)
    # A persisted but unadmitted next occurrence must be skipped after cancel.
    h.automation_now = fixture['cancel_pending_at']
    h.command('automation', 'tick')
    row['pending_cancel'] = native(h)
    view = json.loads(h.command('automation', 'list')['text'])[0]
    cancelled = h.command('automation', 'manage', 'cancel-monthly', text=json.dumps(
        dict(operation='cancel', automationRef=view['automationRef'], version=view['version'])))
    require(not cancelled.get('error'), 'monthly cancellation failed')
    h.command('chat', 'work'); work(h)
    row['cancelled'] = native(h)
    # Privacy is a supplemental protection probe, after the original journey.
    row['revoked_receipt'] = h.send('dependent-on-goal', '请根据独立开发的人生目标继续。')
    row['forget_restart'] = kill(h, 'before_generation_dispatch')
    manage(h, 'forget-goal', operation='forget', memory_ref=row['current_ref'])
    row['fresh_receipt'] = h.send('unrelated', fixture['unrelated'])
    h.command('chat', 'work'); work(h)
    row['after_forget'] = native(h)
    row['forgotten_recall'] = goal_recall(h, 'forgotten-goal')
    row['forgotten_hits'] = row['forgotten_recall']['value']
    work(h)
    row['final'] = native(h)
    require(h.send('monthly', plan) == row['schedule_receipt'], 'cancelled input receipt drift')
    h.command('automation', 'tick'); h.command('chat', 'work'); work(h)
    row['replayed'] = native(h)


def recorded(call, tools):
    keys = ('invocation_id','run_id','tool_name','arguments_json','request_hash','status','result_json','attempt_count')
    return any(all(call[k] == t[k] for k in keys) for t in tools)


def observe(row):
    fixture, _ = load()
    case, variant = inputs(row['variant_id'])
    before, scheduled, final = row['before_request'], row['scheduled'], row['final']
    old, current = row['old_ref'], row['current_ref']
    users = [p['text'] for m in final['conversation'] if m['role'] == 'user' for p in m['content']]
    originals = [variant['context']] + [e['at'] + ' ' + e['text'] for e in variant['events']]
    # Original inputs form an ordered subsequence among explicitly separate
    # scripted driver messages. No fixture event is silently replaced by a plan.
    positions = [users.index(text) if text in users else -1 for text in originals]
    resumed = row['resumed']['value']['result']
    session = case['world']['conversation_session_2026_09_20']
    records = before['memory']['memory']['records']
    old_record = next(r for r in records if r['memory_id'] == old.removeprefix('memory:'))
    current_record = next(r for r in records if r['memory_id'] == current.removeprefix('memory:'))
    checks = task_assertions(final, 1, 2, 2)
    checks.update(
        exact_original_inputs=row['variant'] == variant and min(positions) >= 0 and positions == sorted(set(positions)) and
            all(users.count(text) == 1 for text in originals),
        original_context_in_request=all(any(text == m.get('text') for g in final['generations'] for m in json.loads(g['request_json'])['messages']) for text in originals),
        unanswered_continuation=resumed == dict(resume=session['unanswered_question'], confirmed=session['confirmed'],
            exploratory=session['exploratory'], answered_income=None, ranking=None),
        source_read_through_tool=len(row['resumed']['nested']) == 3 and
            row['resumed']['nested'][0]['tool_name'] == 'mcp__fixture__read' and
            json.loads(json.loads(row['resumed']['nested'][0]['result_json'])['text']) == case['world'] and
            all(any(t['invocation_id'] == a['invocation_id'] and t['result_json'] == a['result_json'] and
                t['arguments_json'] == a['arguments_json'] for a in before['tools']) for t in row['resumed']['nested']),
        no_early_tracking=not before['task'] and not before['automation'] and not before['automation_fire'] and
            all(t['tool_name'] != 'builtin__automation__manage' for t in before['tools']),
        recall_transport=all(len(r['nested']) == 1 and r['nested'][0]['tool_name'] == 'builtin__memory__search' and
            recorded(r['nested'][0],final['tools']) and recorded(r['parent'],final['tools']) and r['value'] == hits ==
            [h for h in json.loads(json.loads(r['nested'][0]['result_json'])['text']) if h['source_type'] == 'memory']
            for r,hits in ((row['initial_recall'],row['initial_hits']),(row['corrected_recall'],row['corrected_hits']),
                (row['forgotten_recall'],row['forgotten_hits']))),
        initial_contents=all(any(h['source_ref'] == ref and h['content'] == source['content'] for h in row['initial_hits'])
            for ref, source in zip((row['core_ref'],old),case['world']['memory'])),
        initial_tiers=any(r['memory_id'] == row['core_ref'].removeprefix('memory:') and r['tier'] == 'core'
            for r in row['initial']['memory']['memory']['records']) and old_record['tier'] == current_record['tier'] == 'recall',
        constraint_main_tool=row['constraint_call']['status'] == 'succeeded' and
            row['constraint_call']['tool_name'] == 'builtin__memory__manage' and
            json.loads(row['constraint_call']['arguments_json']) == dict(operation='remember',content=fixture['constraint']) and
            row['constraint_call'] in before['tools'],
        real_supersede=current_record['supersedes_memory_id'] == old_record['memory_id'] and
            old_record['deleted_at'] is None and current_record['deleted_at'] is None,
        corrected_memory_current=any(h['source_ref'] == current and h['content'] == fixture['current'] for h in row['corrected_hits']) and
            not any(h['source_ref'] == old for h in row['corrected_hits']),
        memory_crash_recovered=row['memory_restart']['returncode'] == -9 and
            row['memory_restart']['boundary'] == 'after_memory_commit:' + row['memory_event_id'] and
            current_record in row['memory_restart']['state']['memory']['memory']['records'] and
            sum(r['event_id'] == row['memory_event_id'] for r in before['memory']['memory']['receipts']) == 1,
        one_requested_schedule=len(scheduled['automation']) == len(row['schedule_views']) == 1 and
            row['schedule_views'][0]['triggerType'] == 'cron' and row['schedule_views'][0]['triggerSpec'] == '0 20 * * 0L' and
            row['schedule_views'][0]['timezone'] == 'Asia/Shanghai' and row['schedule_views'][0]['enabled'] and
            scheduled['automation'][0]['instruction'] == row['schedule_request']['instruction'] and
            row['schedule_views'][0]['nextFire'] == '2026-11-29T12:00:00Z' and
            scheduled['automation'][0]['next_fire_at'] == 1795953600000000 and not scheduled['task'],
        main_schedule_tool=len([t for t in scheduled['tools'] if t['tool_name'] == 'builtin__automation__manage']) == 1 and
            any(t['tool_name'] == 'builtin__automation__manage' and t['status'] == 'succeeded' and
                json.loads(t['arguments_json']) == row['schedule_request'] and any(
                    r['run_id'] == t['run_id'] and r['kind'] == 'main' for r in scheduled['runs']) for t in scheduled['tools']),
        schedule_replay_equal=scheduled == row['schedule_replayed'],
        no_external_effect_or_approval=not final['approval'] and no_external_writes(final),
        no_child_or_failed_run=all(r['parent_run_id'] is None and r['status'] in ('completed','cancelled') for r in final['runs']),
        delivery_main_only=all(m['source'] == 'main' for m in final['conversation'] if m['role'] == 'assistant') and
            all(d['status'] == 'sent' for d in final['deliveries']),
        tools_once=all(t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in final['tools']),
        final_replay_equal=final == row['replayed'])
    artifacts = row['artifacts']
    checks['artifacts_reopened'] = len(artifacts) == len(row['reviews'][-1]['state']['artifact']) == 3 and all(
        a['descriptor'] in final['artifact'] and a['sha256'] == sha(a['text'].encode()) == a['descriptor']['content_hash'] and
        a['bytes'] == len(a['text'].encode()) == a['descriptor']['byte_size'] and any(
            p['artifact_ref'] == a['descriptor']['artifact_ref'] for p in final['artifact_publication']) for a in artifacts)
    checks['resume_artifact_exact'] = len(artifacts) == 3 and json.loads(artifacts[0]['text']) == resumed
    checks['two_review_receipts'] = len(row['reviews']) == 2
    for index, review in enumerate(row['reviews']):
        value, restarted = review['state'], review['restart']
        workers = [r for r in value['runs'] if r['kind'] == 'worker']
        artifact = artifacts[index + 1]
        body = json.loads(artifact['text'])
        checks.update({f'review_{index}_' + k: v for k, v in dict(
            monthly_occurrence=review['at'] == fixture['review_dates'][index] and
                len(value['automation_fire']) == index + 1 and all(f['state'] == 'admitted' for f in value['automation_fire']) and
                value['automation_fire'][-1]['fire_at'] == int(datetime.fromisoformat(review['at']).timestamp() * 1e6),
            one_ongoing_task=len(value['task']) == 1 and value['task'][0]['status'] == 'active' and
                value['task'][0]['completion_policy'] == 'ongoing' and len(workers) == index + 1 and
                len({r['task_id'] for r in workers}) == 1 and len(value['updates']) == index + 1,
            decision_support=body['support'] == fixture['support'] and body['evidence_refs'] == [h['source_ref'] for h in body['goals']],
            current_goals=body['ranking'] is None and any(h['source_ref'] == current and h['content'] == fixture['current'] for h in body['goals']) and
                all(h['source_ref'] != old for h in body['goals']) and any(h['content'] == fixture['constraint'] for h in body['goals']),
            native_recall=any(t['tool_name'] == 'builtin__memory__search' and t['run_id'] == artifact['descriptor']['run_id'] and
                [h for h in json.loads(json.loads(t['result_json'])['text']) if h['source_type'] == 'memory'] == body['goals'] for t in value['tools']),
            restarted=restarted['returncode'] == -9 and restarted['boundary'] == ('after_automation_fire_commit' if index == 0 else 'after_report_commit') and
                (not restarted['state']['task'] if index == 0 else len(restarted['state']['updates']) == 2),
            completed_calls_reused=all(t in value['tools'] for t in restarted['state']['tools'] if t['status'] == 'succeeded') and
                all(g in value['generations'] for g in restarted['state']['generations'] if g['status'] == 'completed'),
            replay_equal=value == review['replayed']).items()})
    pending, cancelled = row['pending_cancel'], row['cancelled']
    checks['pending_cancel_skipped'] = len(pending['automation_fire']) == len(cancelled['automation_fire']) == 3 and \
        pending['automation_fire'][-1]['state'] == 'pending' and cancelled['automation_fire'][-1]['state'] == 'skipped' and \
        cancelled['automation_fire'][-1]['root_run_id'] is None and not cancelled['automation'][0]['enabled'] and \
        len(cancelled['automation_inbox']) == 3 and pending['runs'] == cancelled['runs'] and pending['updates'] == cancelled['updates']
    revoked, fresh = row['revoked_receipt']['run_id'], row['fresh_receipt']['run_id']
    after = row['after_forget']
    requests = [g['request_json'] for g in final['generations'] if g['run_id'] in (fresh,final['runs'][-1]['run_id'])]
    checks['forget_fences_owner'] = row['forget_restart']['returncode'] == -9 and row['forget_restart']['boundary'] == 'before_generation_dispatch' and \
        any(g['run_id'] == revoked and fixture['current'] in g['request_json'] for g in row['forget_restart']['state']['generations']) and \
        any(r['run_id'] == revoked and r['status'] == 'cancelled' and r['error_code'] == 'memory_forgotten' for r in after['runs']) and \
        any(r['run_id'] == fresh and r['status'] == 'completed' for r in after['runs']) and fresh != revoked and \
        len(after['deliveries']) == len(cancelled['deliveries']) + 1
    checks['deleted_recall_and_context_absent'] = bool(requests) and all(fixture['current'] not in g and case['world']['memory'][1]['content'] not in g for g in requests) and \
        all(h['source_ref'] not in (old,current) for h in row['forgotten_hits']) and all(
            r['deleted_at'] is not None for r in final['memory']['memory']['records'] if 'memory:' + r['memory_id'] in (old,current))
    return checks


def rejection_probes(row):
    def artifact_change(r):
        a = r['artifacts'][1]; body = json.loads(a['text']); body['ranking'] = ['独立开发优先']
        a['text'] = json.dumps(body,ensure_ascii=False); a['sha256'] = sha(a['text'].encode()); a['bytes'] = len(a['text'].encode())
        a['descriptor'].update(content_hash=a['sha256'],byte_size=a['bytes'])
    mutations = dict(
        changed_input=lambda r:r['variant']['events'][0].update(text='替换输入'),
        missing_constraint=lambda r:r['constraint_call'].update(status='failed'),
        fabricated_answer=lambda r:r['resumed']['value']['result'].update(answered_income='一年'),
        forged_source=lambda r:r['resumed']['nested'][0].update(invocation_id='forged'),
        stale_memory=lambda r:r['corrected_hits'].append(dict(source_ref=r['old_ref'],content='stale')),
        broken_lineage=lambda r:next(m for m in r['before_request']['memory']['memory']['records'] if 'memory:'+m['memory_id']==r['current_ref']).update(supersedes_memory_id=None),
        false_restart=lambda r:r['memory_restart'].update(returncode=0),
        wrong_timezone=lambda r:r['schedule_views'][0].update(timezone='UTC'),
        wrong_first_fire=lambda r:r['scheduled']['automation'][0].update(next_fire_at=0),
        early_tracking=lambda r:r['before_request']['automation'].append({}),
        duplicate_schedule=lambda r:r['scheduled']['automation'].append(r['scheduled']['automation'][0]),
        missing_month=lambda r:r['reviews'].pop(),
        duplicate_attempt=lambda r:r['final']['tools'][0].update(attempt_count=2),
        corrupt_artifact=lambda r:r['artifacts'][1].update(sha256='0'*64),
        rehashed_ranking=artifact_change,
        cancelled_fire_admitted=lambda r:r['cancelled']['automation_fire'][-1].update(state='admitted'),
        revived_owner=lambda r:next(n for n in r['after_forget']['runs'] if n['run_id']==r['revoked_receipt']['run_id']).update(status='completed'),
        deleted_recall=lambda r:r['forgotten_hits'].append(dict(source_ref=r['current_ref'])),
        replay_drift=lambda r:r['replayed']['deliveries'].append({}))
    result = {}
    for name, mutate in mutations.items():
        changed = copy.deepcopy(row)
        try:
            mutate(changed)
            result[name] = not all(observe(changed).values())
        except (KeyError, IndexError, TypeError, ValueError, RuntimeError, StopIteration):
            result[name] = True
        require(result[name], 'GC-02 oracle accepted corruption: ' + name)
    return result


def drive(binary, item, failure_evidence):
    variant_id, repetition = item
    row = dict(variant_id=variant_id, repetition=repetition)
    embeddings = Embeddings()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc02-') as temp:
            root = Path(temp)
            with personal(binary, root, inputs(variant_id)[0], embeddings) as h:
                try:
                    journey(h, variant_id, row)
                except BaseException:
                    row['failure_state'] = h.failure_state()
                    raise
        row['cleanup'] = dict(disposable_root_removed=not root.exists(), owned_processes_joined=True)
        row['assertions'] = observe(row)
    except BaseException:
        failure_evidence.write_text(json.dumps(row,ensure_ascii=False)+'\n')
        raise
    finally:
        embeddings.close()
    return row


def validate_report(report, binary=ROOT / 'bin/erii'):
    _, lock = load()
    expected = [(v['variant_id'], n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require(report.get('case_id') == 'GC-02' and report.get('status') == 'mechanical_pass' and
        'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        report.get('fixture_lock') == lock and report.get('m6_lock') == load_contract(ROOT,'m6-v1')[2] and report.get('cleanup') is True,
        'complete locked ungraded GC-02 receipts required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']), 'GC-02 source/binary identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    for row in rows:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-02 native receipt')
    require(report.get('rejection_probes') == rejection_probes(rows[0]), 'missing/forged GC-02 corruption probes')
    return dict(case_id='GC-02',status='mechanical_pass',semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=24)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc02-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-02',status=report['status'],semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=24,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc02_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc02_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-02 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-02 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-02',status=report['status'],semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=24,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-02 manifest/native summary drift')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--validate',action='store_true')
    parser.add_argument('--variant-jobs',type=int,choices=(1,2,4),default=2)
    args = parser.parse_args(); binary = args.binary.resolve()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),binary))); return
    args.evidence.unlink(missing_ok=True)
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    native_folder = args.evidence.parent / '.gc02-native'
    native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-02',status='inconclusive',semantic_score=None,eligible_for_grading=False,
        fixture_lock=load()[1],m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=dict(
            platform=platform.platform(),python=platform.python_version(),node=subprocess.check_output(['node','--version'],text=True).strip(),
            go=subprocess.check_output(['go','version'],text=True).strip(),binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py --case GC-02; input fragments only, Memory/schedule adapter incomplete',
            child_approval_unknown='no Child, external effect or approval requested; inventories must remain empty'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder / (args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex: report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-02 native assertion failure')
    try:
        run_cases(ids,series,args.variant_jobs)
        report['results'].sort(key=lambda r:order[r['variant_id'],r['repetition']])
        report['identity']['source_after'] = source_identity()
        report.update(status='mechanical_pass',cleanup=True)
        report['rejection_probes'] = rejection_probes(report['results'][0])
        summary = validate_report(report,binary)
    except BaseException:
        report['status'] = 'inconclusive'
        raise
    finally:
        report['results'].sort(key=lambda r:order[r['variant_id'],r['repetition']])
        retain_report(args.evidence,report)
    print(json.dumps(summary))


if __name__ == '__main__':
    main()
