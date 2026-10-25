#!/usr/bin/env python3
"""GC-06 locked learning journey and native receipts; mechanics, never grades."""
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
from run_m6_cases import personal, execute, save_report, remember, open_bytes, no_external_writes
from terminal_driver import Terminal
from unscripted_cases import source_identity
from verify_capabilities import call
from verify_gc01 import main_checks
from verify_gc02 import native, kill, recorded
from verify_gc03 import exact, recalled_context
from verify_main import require
from verify_memory_maintenance import manage, work, readback
from verify_recall import Embeddings, drain_with_publication_barrier

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / 'evals/fixtures/gc06-journey-v1.json'
REPEATS = 5
REPLIES = [
    'I am the ticket agent. How can I help you with the name on your train ticket?',
    'Could you give me your ticket number, the name on your identity document, and your ticket type? I need to check the fare rules before confirming any change or refund.',
    '1. 更自然表达：The name on my ticket is incorrect.\n2. 时态与被动：My name was written incorrectly.\n3. 礼貌请求：Could you please help me correct it?']


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(FIXTURE.with_suffix('.lock.json').read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected, 'GC-06 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 19 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks),
        'GC-06 frozen block drift')
    return lock


def fixture():
    load()
    return strict_json(FIXTURE.read_bytes())


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT,'m6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-06')
    if variant_id is None: return data,case
    return case,next(v for v in variants(data,case) if v['variant_id'] == variant_id)


def progress(score=2):
    world = inputs()[1]['world']
    return dict(topic='英语 train',kind='learning_progress',goal=world['learning_goal'],
        mastered=world['mastered'],task_success=score,
        difficulty='independent' if score == 3 else 'guided',permanent_deficit=False)


def report_source(attempt=None):
    # Read the actual latest Memory and synthetic assessment via public Tools.
    # Field completion is an objective fixture test, not a fluency assessment.
    source = '''const world=await tools.mcp__fixture__read({text:"world"});
const hits=(await tools.builtin__memory__search({query:"英语 train",limit:20})).filter(h=>h.source_type==="memory");
const records=hits.filter(h=>h.content.startsWith("{")).map(h=>({hit:h,value:JSON.parse(h.content)})).filter(h=>h.value.kind==="learning_progress");
if(records.length!==1)throw Error("one current learning record required");
const current=records[0];
'''
    if attempt is None:
        source += 'const assessment=null; const next=current.value;\n'
    else:
        source += 'const sample=await tools.mcp__fixture__read({text:'+json.dumps('assessment:'+str(attempt))+'});\n'+'''
const fields=Object.keys(sample.rubric).sort();
const matched=fields.filter(k=>sample.answer[k]===sample.rubric[k]);
const score=matched.length;
const assessment={kind:"objective_field_completion",rubric:sample.rubric,answer:sample.answer,matched,score,total:fields.length,
language_quality_score:null};
const next={...current.value,task_success:score,difficulty:score===fields.length?"independent":"guided",permanent_deficit:false};
'''
    return save_report(source+'''
const result={goal:world.learning_goal,mode:world.mode_preference,source_ref:current.hit.source_ref,
previous:current.value,next,assessment,
explanation:{kind:"explanation",example:"The name on my ticket is incorrect."},
practice:{kind:"role_play",scenario:"train ticket name correction",difficulty:next.difficulty,
questions:["ticket number","name on your identity document","ticket type"],refund_confirmed:false},
corrections:[{priority:1,category:"更自然表达",example:"The name on my ticket is incorrect."},
{priority:2,category:"时态与被动",example:"My name was written incorrectly."},
{priority:3,category:"礼貌请求",example:"Could you please help me correct it?"}],
scope:"scripted transport and objective field completion; no language quality grade"};''','learning.json')


def expected_report(ref,previous_score=2,attempt=None):
    world = inputs()[1]['world']; extra = fixture()
    assessment = None; score = previous_score
    if attempt is not None:
        answer = extra['attempts'][attempt]; rubric = extra['rubric']
        matched = [k for k in sorted(rubric) if answer[k] == rubric[k]]; score = len(matched)
        assessment = dict(kind='objective_field_completion',rubric=rubric,answer=answer,matched=matched,
            score=score,total=len(rubric),language_quality_score=None)
    return dict(goal=world['learning_goal'],mode=world['mode_preference'],source_ref=ref,
        previous=progress(previous_score),next=progress(score),assessment=assessment,
        explanation=dict(kind='explanation',example='The name on my ticket is incorrect.'),
        practice=dict(kind='role_play',scenario='train ticket name correction',difficulty=progress(score)['difficulty'],
            questions=['ticket number','name on your identity document','ticket type'],refund_confirmed=False),
        corrections=[dict(priority=i+1,category=c,example=e) for i,(c,e) in enumerate(zip(
            ['更自然表达','时态与被动','礼貌请求'],['The name on my ticket is incorrect.','My name was written incorrectly.','Could you please help me correct it?']))],
        scope='scripted transport and objective field completion; no language quality grade')


def select(h,bridge):
    h.bridge = bridge
    h.setup(metadata=dict(name='GC-06 explicit '+bridge,provider_kind='openai_compatible',
        endpoint='http://127.0.0.1:1/v1',wire_api='openai_completions',model_id='gc06-'+bridge+'-fixture',
        context_window=65536,max_output_tokens=4096,generation_defaults={},
        compatibility=dict(tool_result_format='openai'),capabilities=dict(text_input=True,image_input=True)))


def native_return(call):
    output = strict_json(strict_json(call['parent']['result_json'])['text'])
    require(type(output) is list, 'GC-06 native output must be a list')
    values = [p['value'] for p in output if p['type'] == 'return']
    require(len(values) == 1, 'GC-06 requires one native return')
    require(exact(output,call['output']) and exact(values[0],call['value']), 'GC-06 native output/value drift')
    return values[0]


def transport(call,value):
    return recorded(call['parent'],value['tools']) and all(recorded(t,value['tools']) for t in call['nested'])


def unrelated_task(h,row):
    source = 'return await tools.builtin__memory__search({query:"wedding flowers",limit:20});'
    plan = [dict(name='builtin__codemode__execute',arguments=dict(source=source))]
    create = dict(name='builtin__task__manage',arguments=dict(operation='create',
        task=fixture()['unrelated']+'\nCapabilities: '+json.dumps(plan,ensure_ascii=False)))
    receipt = h.send('unrelated-task','Capabilities: '+json.dumps([create],ensure_ascii=False))
    # Root's independent recall and Main's publication must use the maintained
    # legal ordering. Concurrent pending appends intentionally fail closed in
    # verify_recall.concurrent_append_case; never retry an unknown CLI error.
    row['unrelated_scheduling'] = drain_with_publication_barrier(h,receipt['run_id'],prefix='unrelated-')
    value = native(h)
    workers = {r['run_id'] for r in value['runs'] if r['kind'] == 'worker'}
    parents = [t for t in value['tools'] if t['run_id'] in workers and t['tool_name'] == 'builtin__codemode__execute']
    require(len(parents) == 1,'GC-06 unrelated Task requires one native cell')
    parent = parents[0]
    output = strict_json(strict_json(parent['result_json'])['text'])
    returned = [p['value'] for p in output if p['type'] == 'return']
    require(parent['status'] == 'succeeded' and len(returned) == 1,'GC-06 unrelated Task failed')
    return dict(parent=parent,nested=[t for t in value['tools'] if t['parent_invocation_id'] == parent['invocation_id']],
        output=output,value=returned[0],decisions=[])


def journey(h,variant_id,row):
    case,variant = inputs(variant_id); extra = fixture()
    row['variant'] = variant
    row['old_ref'] = remember(h,'initial-learning',json.dumps(progress(),ensure_ascii=False))
    provider = h.root/'provider/mcp-data.json'
    payloads = strict_json(provider.read_bytes())
    payloads.update({'assessment:'+str(i):dict(rubric=extra['rubric'],answer=a) for i,a in enumerate(extra['attempts'])})
    provider.write_text(json.dumps(payloads,ensure_ascii=False))
    select(h,'golden')
    h.send('context',variant['context'])
    first = variant['events'][0]
    row['first_receipt'] = h.send('event-0',first['at']+' '+first['text'])
    row['input_restart'] = kill(h,'after_generation_commit')
    h.command('chat','work'); work(h)
    with Terminal(h.args('tui'),h.env,h.root) as terminal:
        terminal.expect('State: ready'); terminal.expect('You> ')
        event = variant['events'][1]
        terminal.send(event['at']+' '+event['text'])
        terminal.expect('assistant: '); terminal.expect('You> ')
        require(terminal.stop() == 0,'GC-06 TUI failed to stop')
        row['transcript'] = terminal.transcript()
    event = variant['events'][2]
    h.send('event-2',event['at']+' '+event['text'])
    h.command('chat','work'); work(h); row['original'] = native(h)
    select(h,'terminal-input')
    h.send('private-raw',extra['private']); h.command('chat','work'); work(h)
    select(h,'capabilities')
    row['assessments'] = []
    old = row['old_ref']
    for index in range(2):
        assessment = execute(h,'assessment-'+str(index),report_source(index))
        work(h); row['assessments'].append(assessment)
        accepted = manage(h,'learning-progress-'+str(index),operation='remember',
            content=json.dumps(assessment['value']['result']['next'],ensure_ascii=False),supersedes=old)
        event_id = accepted['receipt'].removeprefix('memory-event:')
        if index == 0:
            row['memory_event_id'] = event_id
            row['memory_restart'] = kill(h,'after_memory_commit:'+event_id,('memory','work'))
        work(h)
        old = 'memory:'+next(r['memory_id'] for r in readback(h)['memory']['records'] if r['memory_id'].startswith(event_id))
        row['guided_ref' if index == 0 else 'current_ref'] = old
    # A new CLI process and new Main Run read the latest persisted progress.
    row['continuation'] = execute(h,'next-session',report_source())
    work(h); row['continued'] = native(h)
    select(h,'terminal-input')
    row['pause_receipt'] = h.send('pause-learning',extra['pause'])
    h.command('chat','work'); work(h); row['paused'] = native(h)
    h.automation_now = extra['fire_at']
    h.command('automation','tick'); h.command('chat','work'); work(h)
    row['quiet'] = native(h)
    select(h,'capabilities')
    row['unrelated'] = unrelated_task(h,row)
    work(h); row['unrelated_state'] = native(h)
    # Only the explicit later request permits a review schedule. Its Root uses
    # the current Memory Tool result; no source record is embedded in the plan.
    select(h,'terminal-input')
    row['reminder_receipt'] = h.send('request-reminder',extra['reminder'])
    h.command('chat','work'); work(h)
    select(h,'capabilities')
    instruction = 'Review current learning progress.\nCapabilities: '+json.dumps([
        dict(name='builtin__codemode__execute',arguments=dict(source=report_source()))],ensure_ascii=False)
    h.automation_now = case['clock']
    row['schedule_call'] = call(h,'schedule-review','builtin__automation__manage',dict(
        operation='create',instruction=instruction,triggerType='cron',triggerSpec='0 20 * * *',timezone='Asia/Shanghai'))
    work(h); row['scheduled'] = native(h)
    h.automation_now = extra['fire_at']
    row['fire_restart'] = kill(h,'after_automation_fire_commit',('automation','tick'))
    h.command('chat','work'); work(h); row['reviewed'] = native(h)
    h.command('chat','work'); work(h); row['review_replayed'] = native(h)
    h.automation_now = extra['cancel_at']; h.command('automation','tick')
    row['pending_cancel'] = native(h)
    view = json.loads(h.command('automation','list')['text'])[0]
    row['cancel_receipt'] = h.command('automation','manage','cancel-review',text=json.dumps(dict(
        operation='cancel',automationRef=view['automationRef'],version=view['version'])))
    require(not row['cancel_receipt'].get('error'),'GC-06 reminder cancel failed')
    h.command('chat','work'); work(h); row['cancelled'] = native(h)
    row['artifacts'] = []
    for descriptor in row['cancelled']['artifact']:
        raw = open_bytes(h,descriptor['artifact_ref'])
        row['artifacts'].append(dict(descriptor=descriptor,text=raw.decode(),sha256=sha(raw),bytes=len(raw)))
    row['revoked_receipt'] = h.send('dependent-learning','请根据英语 train 学习进度继续。')
    row['forget_restart'] = kill(h,'before_generation_dispatch')
    manage(h,'forget-learning',operation='forget',memory_ref=row['current_ref'])
    row['fresh_receipt'] = h.send('after-forget','Choose wedding flowers.')
    h.command('chat','work'); work(h)
    row['forgotten'] = execute(h,'forgotten-progress','return (await tools.builtin__memory__search({query:"英语 train",limit:20})).filter(h=>h.source_type==="memory");')
    work(h); row['final'] = native(h)
    require(h.send('event-0',first['at']+' '+first['text']) == row['first_receipt'],'GC-06 ingress receipt drift')
    h.command('chat','work'); work(h); row['replayed'] = native(h)


def observe(row):
    # Parse persisted returns before trusting summaries or assertion flags.
    returned = [native_return(c) for c in row['assessments']+[row['continuation']]]
    case,variant = inputs(row['variant_id']); extra = fixture()
    original,final = row['original'],row['final']
    old,guided,current = row['old_ref'],row['guided_ref'],row['current_ref']
    users = [p['text'] for m in original['conversation'] if m['role'] == 'user' for p in m['content']]
    expected_inputs = [variant['context']]+[e['at']+' '+e['text'] for e in variant['events']]
    replies = [''.join(p.get('text','') for p in m['content']) for m in original['conversation'] if m['role'] == 'assistant']
    checks = main_checks(original)
    checks.update(exact_original_inputs=exact(row['variant'],variant) and users == expected_inputs,
        original_model_requests=all(any(s == m.get('text') for g in original['generations'] for m in strict_json(g['request_json'])['messages']) for s in expected_inputs),
        tui_input=variant['events'][1]['text'] in row['transcript'],
        delivered_scripted_turns=replies == REPLIES and [strict_json(g['result_json'])['text'] for g in original['generations'] if
            g['status'] == 'completed' and strict_json(g['result_json']).get('text')] == replies,
        english_then_three_chinese=all(s.isascii() for s in replies[:2]) and replies == REPLIES,
        input_generation_reused=row['input_restart']['returncode'] == -9 and row['input_restart']['boundary'] == 'after_generation_commit' and
            any(g['run_id'] == row['first_receipt']['run_id'] and g['status'] == 'completed' for g in row['input_restart']['state']['generations']) and
            all(g in final['generations'] for g in row['input_restart']['state']['generations'] if g['status'] == 'completed'),
        all_tools_once=all(t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in final['tools']),
        no_approval_or_effect=not final['approval'] and no_external_writes(final),
        no_child=all(r['parent_run_id'] is None for r in final['runs']),
        final_replay_equal=exact(final,row['replayed']))
    checks.update({'final_'+k:v for k,v in main_checks(final).items() if k not in ('main_only','no_task_or_automation')})
    records = row['continued']['memory']['memory']['records']
    lineage = [next(m for m in records if 'memory:'+m['memory_id'] == ref) for ref in (old,guided,current)]
    checks['progress_lineage'] = all(m['content'].startswith('sealed:v1:') and m['deleted_at'] is None for m in lineage) and all(
        lineage[i]['supersedes_memory_id'] == lineage[i-1]['memory_id'] for i in (1,2))
    checks['memory_crash_recovered'] = row['memory_restart']['returncode'] == -9 and row['memory_restart']['boundary'] == 'after_memory_commit:'+row['memory_event_id'] and \
        lineage[1] in row['memory_restart']['state']['memory']['memory']['records'] and sum(
            r['event_id'] == row['memory_event_id'] for r in row['continued']['memory']['memory']['receipts']) == 1
    for index,(call,body,ref,previous,attempt) in enumerate(zip(row['assessments']+[row['continuation']],returned,
            (old,guided,current),(2,1,3),(0,1,None))):
        prefix = 'report_'+str(index)+'_'; nested = call['nested']; expected = expected_report(ref,previous,attempt)
        names = ['mcp__fixture__read','builtin__memory__search']+([] if attempt is None else ['mcp__fixture__read'])+['builtin__artifact__manage']*2
        hits = strict_json(strict_json(nested[1]['result_json'])['text'])
        progress_hits = [h for h in hits if h['source_type'] == 'memory' and h['content'].startswith('{') and strict_json(h['content']).get('kind') == 'learning_progress']
        checks[prefix+'native_tools'] = transport(call,row['continued']) and [t['tool_name'] for t in nested] == names and exact(
            strict_json(call['parent']['arguments_json']),dict(source=report_source(attempt))) and exact(
            strict_json(nested[0]['arguments_json']),dict(text='world')) and exact(strict_json(strict_json(nested[0]['result_json'])['text']),case['world'])
        checks[prefix+'current_recall'] = len(progress_hits) == 1 and progress_hits[0]['source_ref'] == ref and exact(strict_json(progress_hits[0]['content']),progress(previous))
        if attempt is not None:
            checks[prefix+'objective_basis'] = exact(strict_json(nested[2]['arguments_json']),dict(text='assessment:'+str(attempt))) and exact(
                strict_json(strict_json(nested[2]['result_json'])['text']),dict(rubric=extra['rubric'],answer=extra['attempts'][attempt]))
        checks[prefix+'recomputed_report'] = exact(body['result'],expected)
        checks[prefix+'artifact_transport'] = exact(strict_json(nested[-2]['result_json'])['artifact_refs'],body['artifactRefs']) and exact(
            strict_json(nested[-1]['arguments_json']),dict(operation='publish',artifactRefs=body['artifactRefs']))
        artifacts = [a for a in row['artifacts'] if a['descriptor']['artifact_ref'] in body['artifactRefs']]
        checks[prefix+'artifact'] = len(artifacts) == len(body['artifactRefs']) == 1 and exact(strict_json(artifacts[0]['text']),expected)
        delivered = [strict_json(g['result_json'])['text'] for g in final['generations'] if g['run_id'] == call['parent']['run_id'] and
            g['status'] == 'completed' and strict_json(g['result_json']).get('text')]
        checks[prefix+'visible_delivery'] = len(delivered) == 1 and delivered[0].startswith('Observed Tool result: ') and exact(
            strict_json(delivered[0].removeprefix('Observed Tool result: ')),call['output']) and any(
            ''.join(p.get('text','') for p in m['content']) == delivered[0] for m in final['conversation'] if m['role'] == 'assistant')
    checks['artifacts_reopened'] = len(row['artifacts']) == len(final['artifact']) == 4 and all(
        a['descriptor'] in final['artifact'] and a['sha256'] == sha(a['text'].encode()) == a['descriptor']['content_hash'] and
        a['bytes'] == len(a['text'].encode()) == a['descriptor']['byte_size'] and any(
            p['artifact_ref'] == a['descriptor']['artifact_ref'] for p in final['artifact_publication']) for a in row['artifacts'])
    checks['pause_quiet'] = extra['pause'] in json.dumps(row['paused']['conversation'],ensure_ascii=False) and exact(row['paused'],row['quiet']) and \
        not row['quiet']['task'] and not row['quiet']['automation']
    unrelated = row['unrelated']; worker = unrelated['parent']['run_id']; value = row['unrelated_state']
    checks['task_publication_order'] = exact(row['unrelated_scheduling'],dict(worker_requests_before_main_completion=0,
        boundaries=dict(after_task_commit=1,before_worker_generation=1),root_recall_frozen_before_main_publication=True,
        main_completed_before_worker_dispatch=True))
    checks['unrelated_task_privacy'] = transport(unrelated,value) and len(unrelated['nested']) == 1 and unrelated['nested'][0]['tool_name'] == 'builtin__memory__search' and exact(
        strict_json(unrelated['nested'][0]['arguments_json']),dict(query='wedding flowers',limit=20)) and exact(
        native_return(unrelated),strict_json(strict_json(unrelated['nested'][0]['result_json'])['text'])) and any(
        r['run_id'] == worker and r['kind'] == 'worker' and r['status'] == 'completed' for r in value['runs']) and all(
        extra['private'] not in g['request_json'] for g in value['generations'] if g['run_id'] == worker) and extra['private'] not in json.dumps(unrelated['value'])
    scheduled,reviewed = row['scheduled'],row['reviewed']
    schedule = row['schedule_call']; request = strict_json(schedule['arguments_json'])
    checks['explicit_schedule'] = recorded(schedule,scheduled['tools']) and request['operation'] == 'create' and request['triggerType'] == 'cron' and request['triggerSpec'] == '0 20 * * *' and \
        request['timezone'] == 'Asia/Shanghai' and request['instruction'] == 'Review current learning progress.\nCapabilities: '+json.dumps([
            dict(name='builtin__codemode__execute',arguments=dict(source=report_source()))],ensure_ascii=False) and len(scheduled['automation']) == 1 and \
        scheduled['automation'][0]['next_fire_at'] == int(datetime.fromisoformat(extra['fire_at']).timestamp()*1e6) and any(
            extra['reminder'] == p.get('text') for m in scheduled['conversation'] if m['role'] == 'user' for p in m['content'])
    fire = reviewed['automation_fire']
    root = fire[0]['root_run_id']
    review_artifacts = [a for a in row['artifacts'] if a['descriptor']['run_id'] == root]
    checks['reminder_current_progress'] = len(fire) == len(review_artifacts) == 1 and fire[0]['state'] == 'admitted' and exact(
        strict_json(review_artifacts[0]['text']),expected_report(current,3)) and any(
        r['run_id'] == root and r['kind'] == 'worker' and r['status'] == 'completed' and r['parent_run_id'] is None for r in reviewed['runs']) and any(
        t['tool_name'] == 'builtin__memory__search' and t['run_id'] == root and any(h['source_ref'] == current for h in strict_json(strict_json(t['result_json'])['text'])) for t in reviewed['tools']) and \
        any(u['root_run_id'] == root for u in reviewed['updates'])
    checks['reminder_task_ownership'] = len(reviewed['task']) == 2 and any(
        t['task_id'] == reviewed['automation'][0]['task_id'] and t['completion_policy'] == 'ongoing' and t['status'] == 'active' for t in reviewed['task']) and any(
        r['run_id'] == root and r['task_id'] == reviewed['automation'][0]['task_id'] for r in reviewed['runs'])
    checks['fire_recovered_once'] = row['fire_restart']['returncode'] == -9 and row['fire_restart']['boundary'] == 'after_automation_fire_commit' and \
        row['fire_restart']['state']['automation_fire'][0]['state'] == 'pending' and exact(reviewed,row['review_replayed'])
    pending,cancelled = row['pending_cancel'],row['cancelled']
    checks['pending_cancel_skipped'] = len(pending['automation_fire']) == len(cancelled['automation_fire']) == 2 and \
        pending['automation_fire'][-1]['state'] == 'pending' and cancelled['automation_fire'][-1]['state'] == 'skipped' and cancelled['automation_fire'][-1]['root_run_id'] is None and \
        not cancelled['automation'][0]['enabled'] and len(cancelled['automation_inbox']) == 2 and pending['runs'] == cancelled['runs'] and pending['updates'] == cancelled['updates']
    revoked,fresh = row['revoked_receipt']['run_id'],row['fresh_receipt']['run_id']
    checks['forget_fences_owner'] = row['forget_restart']['returncode'] == -9 and row['forget_restart']['boundary'] == 'before_generation_dispatch' and any(
        h['source_ref'] == current for h in recalled_context(row['forget_restart']['state']['generations'],revoked)) and any(
        r['run_id'] == revoked and r['status'] == 'cancelled' and r['error_code'] == 'memory_forgotten' for r in final['runs']) and any(
        r['run_id'] == fresh and r['status'] == 'completed' for r in final['runs'])
    forgotten = row['forgotten']
    requests = [g['request_json'] for g in final['generations'] if g['run_id'] in (fresh,forgotten['parent']['run_id'])]
    checks['forgotten_absent'] = transport(forgotten,final) and exact(native_return(forgotten),[h for h in strict_json(strict_json(forgotten['nested'][0]['result_json'])['text']) if h['source_type'] == 'memory']) and \
        all(h['source_ref'] not in (old,guided,current) for h in forgotten['value']) and bool(requests) and all(ref not in req for ref in (old,guided,current) for req in requests) and all(
        m['deleted_at'] is not None for m in final['memory']['memory']['records'] if 'memory:'+m['memory_id'] in (old,guided,current))
    return checks


def rejection_probes(row):
    def change_return(r,mutate):
        calculation = r['assessments'][0]
        envelope = strict_json(calculation['parent']['result_json'])
        output = strict_json(envelope['text']); mutate(output[0]['value'])
        envelope['text'] = json.dumps(output,ensure_ascii=False)
        result = json.dumps(envelope,ensure_ascii=False)
        def replace(value):
            if isinstance(value,dict):
                if value.get('invocation_id') == calculation['parent']['invocation_id'] and 'result_json' in value:
                    value['result_json'] = result
                for child in value.values(): replace(child)
            elif isinstance(value,list):
                for child in value: replace(child)
        replace(r)
        calculation['output'] = copy.deepcopy(output); calculation['value'] = copy.deepcopy(output[0]['value'])
    mutations = dict(
        changed_input=lambda r:r['variant']['events'][0].update(text='replaced'),
        missing_input=lambda r:r['original']['conversation'].pop(0),
        language_drift=lambda r:next(m for m in r['original']['conversation'] if m['role'] == 'assistant')['content'][0].update(text='中文角色扮演'),
        cached_return=lambda r:r['assessments'][0]['value']['result'].update(mode='中文'),
        native_mode=lambda r:change_return(r,lambda v:v['result'].update(mode='中文')),
        repeated_mastered=lambda r:change_return(r,lambda v:v['result']['practice'].update(scenario='hotel check-in')),
        refund_claim=lambda r:change_return(r,lambda v:v['result']['practice'].update(refund_confirmed=True)),
        missing_correction=lambda r:change_return(r,lambda v:v['result']['corrections'].pop()),
        permanent_deficit=lambda r:change_return(r,lambda v:v['result']['next'].update(permanent_deficit=True)),
        invented_grade=lambda r:change_return(r,lambda v:v['result']['assessment'].update(language_quality_score=100)),
        objective_score=lambda r:change_return(r,lambda v:v['result']['assessment'].update(score=3)),
        difficulty_drift=lambda r:change_return(r,lambda v:v['result']['next'].update(difficulty='independent')),
        stale_source=lambda r:change_return(r,lambda v:v['result'].update(source_ref='memory:stale')),
        forged_artifact_ref=lambda r:change_return(r,lambda v:v.update(artifactRefs=['artifact:forged'])),
        broken_lineage=lambda r:next(m for m in r['continued']['memory']['memory']['records'] if 'memory:'+m['memory_id'] == r['current_ref']).update(supersedes_memory_id=None),
        false_restart=lambda r:r['memory_restart'].update(returncode=0),
        corrupt_artifact=lambda r:r['artifacts'][0].update(sha256='0'*64),
        unsolicited_reminder=lambda r:r['quiet']['automation'].append({}),
        wrong_timezone=lambda r:r['schedule_call'].update(arguments_json=r['schedule_call']['arguments_json'].replace('Asia/Shanghai','UTC')),
        wrong_fire=lambda r:r['scheduled']['automation'][0].update(next_fire_at=0),
        false_fire_restart=lambda r:r['fire_restart'].update(returncode=0),
        cancelled_admission=lambda r:r['cancelled']['automation_fire'][-1].update(state='admitted'),
        raw_private_recall=lambda r:next(g for g in r['unrelated_state']['generations'] if g['run_id'] == r['unrelated']['parent']['run_id']).update(request_json=fixture()['private']),
        bypassed_publication=lambda r:r['unrelated_scheduling'].update(main_completed_before_worker_dispatch=False),
        early_worker_dispatch=lambda r:r['unrelated_scheduling'].update(worker_requests_before_main_completion=1),
        duplicate_attempt=lambda r:r['final']['tools'][0].update(attempt_count=2),
        external_effect=lambda r:r['final']['tools'].append(dict(tool_name='send_ticket',run_id=r['final']['runs'][0]['run_id'])),
        revived_owner=lambda r:next(n for n in r['final']['runs'] if n['run_id'] == r['revoked_receipt']['run_id']).update(status='completed'),
        deleted_recall=lambda r:r['forgotten']['value'].append(dict(source_ref=r['current_ref'])),
        replay_drift=lambda r:r['replayed']['deliveries'].append({}))
    require(all(observe(row).values()),'GC-06 corruption probes require a passing baseline')
    result = {}
    for name,mutate in mutations.items():
        changed = copy.deepcopy(row)
        try:
            mutate(changed); result[name] = not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
            result[name] = True
        require(result[name],'GC-06 consumer accepted corruption: '+name)
    return result


def drive(binary, item, failure_evidence):
    variant_id, repetition = item
    row = dict(variant_id=variant_id,repetition=repetition)
    embeddings = Embeddings()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc06-') as temp:
            root = Path(temp)
            with personal(binary,root,inputs(variant_id)[0],embeddings) as h:
                try:
                    journey(h,variant_id,row)
                except BaseException:
                    row['failure_state'] = h.failure_state()
                    raise
        row['cleanup'] = dict(disposable_root_removed=not root.exists(),owned_processes_joined=True)
        row['assertions'] = observe(row)
    except BaseException:
        failure_evidence.write_text(json.dumps(row,ensure_ascii=False)+'\n')
        raise
    finally:
        embeddings.close()
    return row


def environment_identity(binary):
    return dict(platform=platform.platform(),python=platform.python_version(),
        node=subprocess.check_output(['node','--version'],text=True).strip(),
        go=subprocess.check_output(['go','version'],text=True).strip(),
        binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))


def validate_report(report, binary=ROOT / 'bin/erii'):
    lock = load()
    expected = [(v['variant_id'], n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require(report.get('case_id') == 'GC-06' and report.get('status') == 'mechanical_pass' and
        report.get('release_ready') is False and 'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        exact(report.get('fixture_lock'),lock) and exact(report.get('m6_lock'),load_contract(ROOT,'m6-v1')[2]) and report.get('cleanup') is True,
        'complete locked ungraded GC-06 receipts required')
    identity = report['identity']
    require(exact(identity['source_before'],identity['source_after']) and exact(identity['source_before'],source_identity()) and
        identity['binary_sha256'] == sha(binary.read_bytes()) and exact(identity['environment'],environment_identity(binary)), 'GC-06 source/binary/environment identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    for row in rows:
        checks = observe(row)
        require(exact(row['assertions'],checks) and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-06 native receipt')
    require(exact(report.get('rejection_probes'),rejection_probes(rows[0])), 'missing/forged GC-06 corruption probes')
    return dict(case_id='GC-06',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=19)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc06-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-06',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=19,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc06_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc06_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-06 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-06 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-06',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=19,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-06 manifest/native summary drift')
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
    native_folder = args.evidence.parent / '.gc06-native'
    native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-06',status='inconclusive',release_ready=False,semantic_score=None,eligible_for_grading=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=environment_identity(binary)),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-06; original input fragments only; durable learning-memory/assessment/schedule adapter incomplete',
            child_approval_unknown='Main learning plus unrelated Task and explicit reminder Root; no Child or external effect; approval/unknown effects inapplicable'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder / (args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex: report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-06 native assertion failure')
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
