#!/usr/bin/env python3
"""GC-04 original travel journey with native, recomputed mechanical receipts.

Explicit scripted plans are separate from original inputs. No semantic grade.
"""
import argparse
import copy
from datetime import datetime, timedelta
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
from run_m6_cases import personal, save_report, save_bytes, open_bytes, travel_control
from terminal_driver import Terminal
from unscripted_cases import source_identity
from verify_capabilities import call, content
from verify_gc01 import kill
from verify_gc02 import native, recorded
from verify_gc03 import exact
from verify_main import require
from verify_memory_maintenance import work
from verify_recall import Embeddings, drain_with_publication_barrier, main_completed, worker_requests
from verify_tasks import assertions as task_assertions

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / 'evals/fixtures/gc04-v1.lock.json'
REPEATS = 5


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT / name).read_bytes()) == expected, 'GC-04 lock drift: ' + name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT / 'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 24 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current for b in blocks),
        'GC-04 frozen block drift')
    return lock


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT, 'm6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-04')
    if variant_id is None:
        return data, case
    return case, next(v for v in variants(data, case) if v['variant_id'] == variant_id)


def select(h, bridge):
    h.bridge = bridge
    h.setup(metadata=dict(name='GC-04 explicit ' + bridge, provider_kind='openai_compatible',
        endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions', model_id='gc04-' + bridge + '-fixture',
        context_window=65536, max_output_tokens=4096, generation_defaults={},
        compatibility=dict(tool_result_format='openai'), capabilities=dict(text_input=True,image_input=True)))


def plan(name, arguments):
    return dict(name=name, arguments=arguments)


def send_plan(h, key, plans):
    return h.send(key, 'Capabilities: ' + json.dumps(plans,ensure_ascii=False))


def task_ref(h):
    invocation = next(t for t in reversed(h.state()['tools']) if t['tool_name']=='builtin__task__manage')
    return json.loads(content(invocation)['text'])['taskRef']


def instruction(source, text):
    return text + '\nCapabilities: ' + json.dumps([plan('builtin__codemode__execute',dict(source=source))],ensure_ascii=False)


def itinerary_source(variant_id, originals, prior=None):
    # Compute from the read-only Tool result, not a prefilled ETA. The driver
    # supplies the user's 20:00 constraint explicitly; language extraction is
    # ungraded. Keep a 10-minute margin and subtract the actual return transit.
    case, variant = inputs(variant_id)
    config = dict(sampled_at=case['clock'],deadline='20:00',margin_minutes=10,
        steering=variant['events'][1]['text'],group_request=variant['events'][2]['text'] if prior else None,
        confirmed_artifact_refs=originals,prior_artifact_ref=prior)
    return save_report('const config=' + json.dumps(config,ensure_ascii=False) + ';\n' + '''const world=await tools.mcp__fixture__read({text:"world"});
const minutes=t=>Number(t.split(":")[0])*60+Number(t.split(":")[1]);
const clock=n=>String(Math.floor(n/60)).padStart(2,"0")+":"+String(n%60).padStart(2,"0");
const arrival=minutes(config.deadline)-config.margin_minutes;
const departure=arrival-world.transit_minutes.hotel_return;
const result={date:"2026-10-05",source:{tool:"mcp__fixture__read",sampled_at:config.sampled_at},
  anchor:world.tasks[0].anchor,hotel:world.hotel_nice,weather:world.weather,museum:world.museum,
  transit_minutes:world.transit_minutes,constraints:config,
  confirmed:world.bookings,needs_booking:[world.museum.name],no_booking:["城堡山"],
  route:[{place:world.museum.name,arrive:world.museum.open,depart:"12:00",transport:"公共交通"},
         {place:"城堡山",arrive:clock(12*60+world.transit_minutes.castle_hill),depart:clock(departure),transport:"公共交通"}],
  return_departure:clock(departure),return_eta:clock(departure+world.transit_minutes.hotel_return),
  booking_actions:[],visa:null,flights:null,scope:"synthetic read-only plan; no booking or payment performed"};''',
        'itinerary_1005_grouped.json' if prior else 'itinerary_1005.json')



def group_report_crash(h, main_run):
    """Use the same Recall/Main ordering before the real Reporter SIGKILL.

    The grouping append is a new Root too. Its publication ordering must be
    explicit even though the preceding itinerary already finished.
    """
    publish=h.root/'group-publish.release'
    worker=h.root/'group-worker.release'
    report=h.root/'group-report.release'
    require(not any(p.exists() for p in (publish,worker,report)), 'occupied group barriers')
    baseline=worker_requests(h)
    seen=dict(after_task_commit=0,before_worker_generation=0,after_report_commit=0)
    receipt={}
    def observe(name):
        require(name in seen,'unexpected grouping barrier')
        seen[name]+=1
        if name=='before_worker_generation':publish.touch(exist_ok=True)
        if name=='after_report_commit':
            child=h.children[-1]
            receipt.update(boundary=name,state=native(h))
            child.kill()
    def advance():
        if publish.exists() and not worker.exists() and main_completed(h,main_run):
            receipt['worker_requests_before_main_completion']=worker_requests(h)-baseline
            require(receipt['worker_requests_before_main_completion']==0,'group Worker dispatched before Main publication')
            worker.touch(exist_ok=False)
    result=h.drive_boundaries(dict(after_task_commit=publish,before_worker_generation=worker,after_report_commit=report),observe,advance)
    require(result.returncode==-9 and all(seen.values()) and worker.exists() and not report.exists(),
        'group report crash failed: '+repr(dict(returncode=result.returncode,stdout=result.stdout,stderr=result.stderr)))
    return (dict(boundary=receipt['boundary'],returncode=result.returncode,state=receipt['state']),
        dict(worker_requests_before_main_completion=receipt['worker_requests_before_main_completion'],boundaries=seen,
             root_recall_frozen_before_main_publication=True,main_completed_before_worker_dispatch=True))


def recover_group(h):
    # Keep the actual failed command outcome. A sibling's cancellation must not
    # erase the first exception or turn an empty stderr into an opaque failure.
    result=h.command('chat','work',ok=False)
    require(result.returncode==0,'group recovery failed: '+repr(dict(returncode=result.returncode,
        stdout=result.stdout[:2000],stderr=result.stderr[:2000])))
    return result

def journey(h, variant_id, row):
    case, variant = inputs(variant_id)
    row['variant'] = variant
    world = case['world']
    originals = [save_bytes(h,'itinerary_1003.md','text/markdown',b'Confirmed Seine cruise 2026-10-03 19:00'),
        save_bytes(h,'itinerary_1004.md','text/markdown',b'Confirmed Louvre 2026-10-04 09:30')]
    refs = [a['artifact_ref'] for a in originals]
    row['originals'] = originals
    h.command('chat','send-parts','confirmed-inputs',text=json.dumps([dict(type='text',text='Prior confirmed itinerary sources'),
        *[dict(type='file',artifact_ref=ref) for ref in refs]]))
    france = '法国旅行\nCapabilities: ' + json.dumps([plan('mcp__fixture__echo',dict(text=json.dumps(world['tasks'][0],ensure_ascii=False)))],ensure_ascii=False)
    seed = send_plan(h,'france-seed',[plan('builtin__task__manage',dict(operation='create',task=france,artifactRefs=refs))])
    row['seed_barrier'] = drain_with_publication_barrier(h,seed['run_id'],prefix='seed-')
    row['france_ref'] = task_ref(h)
    provider = h.root / 'provider'; (provider/'hold-barrier').touch()
    nz_source = 'await tools.mcp__fixture__read({text:"world",behavior:"hold"});return "NZ context";'
    nz = send_plan(h,'nz-seed',[plan('builtin__task__manage',dict(operation='create',task=instruction(nz_source,'新西兰旅行')))])
    def nz_task():
        return next(t for t in native(h)['task'] if t['task_ref'] != row['france_ref'])
    def pause():
        row['nz_ref'] = nz_task()['task_ref']
        return [plan('builtin__task__manage',dict(operation='pause',taskRef=row['nz_ref']))]
    row['nz_guard'] = travel_control(h,nz,'nz',pause,lambda:any(r['desired_state']=='task_paused' for r in
        native(h)['runs'] if r['task_id']==nz_task()['task_id']))
    h.command('chat','work'); work(h)
    for name in ('hold-release','hold-accepted'): (provider/name).unlink(missing_ok=True)
    row['initial'] = native(h)
    select(h,'terminal-input')
    h.send('context',variant['context'])
    event = variant['events'][0]
    row['first_receipt'] = h.send('event-0',event['at']+' '+event['text'])
    row['input_restart'] = kill(h,'after_generation_commit')
    h.command('chat','work'); work(h)
    select(h,'capabilities')
    row['search'] = call(h,'find-france','builtin__task__search',dict(query='法国旅行',scope='all',status=['completed']))
    hold_source = 'return await tools.mcp__fixture__read({text:"world",behavior:"hold"});'
    receipt = send_plan(h,'continue-france',[plan('builtin__task__manage',dict(operation='append',taskRef=row['france_ref'],
        instruction=instruction(hold_source,variant['events'][0]['text'])))])
    event = variant['events'][1]
    steering = [plan('builtin__task__manage',dict(operation='append',taskRef=row['france_ref'],
        instruction=instruction(itinerary_source(variant_id,refs),event['text'])))]
    france_id = next(t['task_id'] for t in row['initial']['task'] if t['task_ref']==row['france_ref'])
    # One atomic ingress has two explicit content parts: unchanged original
    # event and scripted driver. Either live drainer can safely own this Main;
    # no race to intercept a generation belonging to a different process.
    row['france_guard'] = travel_control(h,receipt,'france',steering,lambda:any(event['text'] in r['instruction']
        for r in native(h)['task_instruction'] if r['task_id']==france_id),
        input_parts=[dict(type='text',text=event['at']+' '+event['text'])])
    row['steering_receipt'] = row['france_guard']['control_receipt']
    h.command('chat','work'); work(h)
    row['steered'] = native(h)
    prior = next(a['artifact_ref'] for a in row['steered']['artifact'] if a['name']=='itinerary_1005.json')
    select(h,'terminal-input')
    with Terminal(h.args('tui'),h.env,h.root) as terminal:
        terminal.expect('State: ready'); terminal.expect('You> ')
        event = variant['events'][2]
        terminal.send(event['at']+' '+event['text'])
        terminal.expect('assistant: '); terminal.expect('You> ')
        require(terminal.stop()==0,'GC-04 TUI did not stop')
        row['transcript'] = terminal.transcript()
    select(h,'capabilities')
    # Both the prior plan and confirmed sources stay accessible to the new Root.
    group_plan = [plan('builtin__task__manage',dict(operation='append',taskRef=row['france_ref'],
        instruction=instruction(itinerary_source(variant_id,refs,prior),event['text']),artifactRefs=[*refs,prior]))]
    group = h.command('chat','send-parts','group-bookings',text=json.dumps([
        dict(type='text',text='Capabilities: '+json.dumps(group_plan,ensure_ascii=False)),
        *[dict(type='file',artifact_ref=ref) for ref in [*refs,prior]]]))
    row['group_receipt'] = group
    row['report_restart'],row['group_barrier'] = group_report_crash(h,group['run_id'])
    recover_group(h); work(h)
    row['artifacts'] = []
    for descriptor in native(h)['artifact']:
        raw = open_bytes(h,descriptor['artifact_ref'])
        row['artifacts'].append(dict(descriptor=descriptor,text=raw.decode(),sha256=sha(raw),bytes=len(raw)))
    row['final'] = native(h)
    require(h.send('event-0',variant['events'][0]['at']+' '+variant['events'][0]['text'])==row['first_receipt'],'original receipt drift')
    h.command('chat','work'); work(h)
    row['replayed'] = native(h)


def native_document(value, name):
    art = next(a for a in value['artifact'] if a['name']==name)
    parent = next(t for t in value['tools'] if t['run_id']==art['run_id'] and t['tool_name']=='builtin__codemode__execute' and
        name in t['arguments_json'])
    output = strict_json(strict_json(parent['result_json'])['text'])
    returns = [p['value'] for p in output if p['type']=='return']
    require(len(returns)==1,'one native itinerary return required')
    returned = returns[0]
    require(returned['artifactRefs']==[art['artifact_ref']],'native itinerary Artifact reference drift')
    saves = [t for t in value['tools'] if t['run_id']==art['run_id'] and t['tool_name']=='builtin__artifact__manage' and
        strict_json(t['arguments_json']).get('operation')=='save']
    require(len(saves)==1 and exact(strict_json(strict_json(saves[0]['arguments_json'])['text']),returned['result']),
        'native itinerary save/return drift')
    return art, returned['result']


def document_checks(document, variant_id, originals, prior):
    case, variant = inputs(variant_id); world = case['world']
    config = dict(sampled_at=case['clock'],deadline='20:00',margin_minutes=10,steering=variant['events'][1]['text'],
        group_request=variant['events'][2]['text'] if prior else None,confirmed_artifact_refs=originals,prior_artifact_ref=prior)
    deadline = datetime.strptime(config['deadline'],'%H:%M')
    arrival = deadline - timedelta(minutes=config['margin_minutes'])
    departure = arrival - timedelta(minutes=world['transit_minutes']['hotel_return'])
    return dict(source_and_sampling=exact(document['source'],dict(tool='mcp__fixture__read',sampled_at=case['clock'])) and
            exact(document['weather'],world['weather']) and exact(document['museum'],world['museum']) and
            exact(document['transit_minutes'],world['transit_minutes']),
        confirmed_preferences_and_hotel=exact(document['anchor'],world['tasks'][0]['anchor']) and
            exact(document['hotel'],world['hotel_nice']) and exact(document['confirmed'],world['bookings']),
        constraint_and_lineage=exact(document['constraints'],config),
        independent_eta=document['return_departure']==departure.strftime('%H:%M') and document['return_eta']==arrival.strftime('%H:%M') and
            document['return_eta']<='19:50',
        route_and_opening=exact(document['route'],[
            dict(place=world['museum']['name'],arrive=world['museum']['open'],depart='12:00',transport='公共交通'),
            dict(place='城堡山',arrive=(datetime.strptime('12:00','%H:%M')+timedelta(minutes=world['transit_minutes']['castle_hill'])).strftime('%H:%M'),
                depart=departure.strftime('%H:%M'),transport='公共交通')]),
        booking_groups=exact(document['needs_booking'],[world['museum']['name']]) and exact(document['no_booking'],['城堡山']),
        no_invented_booking=document['booking_actions']==[] and document['visa'] is None and document['flights'] is None and
            document['date']=='2026-10-05' and document['scope']=='synthetic read-only plan; no booking or payment performed')



def tool_outcomes(value, nz_ref):
    nz_id=next(t['task_id'] for t in value['task'] if t['task_ref']==nz_ref)
    runs={r['run_id']:r for r in value['runs']}
    cancelled=[t for t in value['tools'] if t['status']=='cancelled']
    return len(cancelled)==2 and all(
        t['attempt_count']==1 and (t['status']=='succeeded' or
            t['status']=='cancelled' and runs[t['run_id']]['task_id']==nz_id and
            runs[t['run_id']]['status']=='cancelled' and
            t['tool_name'] in ('builtin__codemode__execute','mcp__fixture__read'))
        for t in value['tools'])

def observe(row):
    case, variant = inputs(row['variant_id'])
    initial, steered, final = row['initial'], row['steered'], row['final']
    first, doc1 = native_document(final,'itinerary_1005.json')
    last, doc2 = native_document(final,'itinerary_1005_grouped.json')
    refs = [a['artifact_ref'] for a in row['originals']]
    checks = task_assertions(final,2,4,3)
    for prefix, document, prior in (('steered',doc1,None),('grouped',doc2,first['artifact_ref'])):
        checks.update({prefix+'_'+k:v for k,v in document_checks(document,row['variant_id'],refs,prior).items()})
    users = [p['text'] for m in final['conversation'] if m['role']=='user' for p in m['content'] if p['type']=='text']
    originals = [variant['context']]+[e['at']+' '+e['text'] for e in variant['events']]
    positions = [users.index(t) if t in users else -1 for t in originals]
    steering_input = originals[2] + 'Capabilities: ' + json.dumps([plan('builtin__task__manage',
        dict(operation='append',taskRef=row['france_ref'],instruction=instruction(
            itinerary_source(row['variant_id'],refs),variant['events'][1]['text'])))],ensure_ascii=False)
    france = lambda value:next(t for t in value['task'] if t['task_ref']==row['france_ref'])
    roots = lambda value:[r for r in value['runs'] if r['task_id']==france(value)['task_id']]
    search = strict_json(strict_json(row['search']['result_json'])['text'])
    checks.update(
        expected_tool_outcomes=tool_outcomes(final,row['nz_ref']),
        consistent_native_views=all(value[a]==value[b] for value in (initial,steered,final)
            for a,b in (('task','tasks'),('task_instruction','instructions'),('task_update','updates'))),
        exact_original_inputs=row['variant']==variant and min(positions)>=0 and positions==sorted(set(positions)) and
            all(users.count(t)==1 for t in originals),
        originals_in_frozen_requests=all(any(t==m.get('text') or
            (t==originals[2] and m.get('text')==steering_input)
            for g in final['generations'] for m in strict_json(g['request_json'])['messages']) for t in originals),
        tui_input=variant['events'][2]['text'] in row['transcript'],
        actual_task_search=recorded(row['search'],final['tools']) and row['search']['tool_name']=='builtin__task__search' and
            strict_json(row['search']['arguments_json'])==dict(query='法国旅行',scope='all',status=['completed']) and len(search)==1 and
            search[0]['taskRef']==row['france_ref'] and search[0]['status']=='completed',
        initial_existing_tasks=len(initial['task'])==2 and france(initial)['status']=='completed' and
            next(t for t in initial['task'] if t['task_ref']==row['nz_ref'])['status']=='paused',
        paused_old_root_fenced=all(r['status']=='cancelled' and r['error_code']=='task_paused' for r in final['runs'] if
            r['task_id']==next(t['task_id'] for t in final['task'] if t['task_ref']==row['nz_ref'])) and
            next(t for t in final['task'] if t['task_ref']==row['nz_ref'])==next(t for t in initial['task'] if t['task_ref']==row['nz_ref']),
        new_roots_and_epochs=len(roots(initial))==1 and len(roots(steered))==2 and len(roots(final))==3 and
            [r['task_epoch'] for r in roots(final)]==[1,2,3] and all(r['status']=='completed' for r in roots(final)),
        anchor_immutable=france(initial)['anchor_task']==france(steered)['anchor_task']==france(final)['anchor_task'],
        steering_while_unfinished=any(r['run_id']==first['run_id'] and r['status']=='running' for r in row['france_guard']['held_state']['runs']) and
            any(r['run_id']==row['steering_receipt']['run_id'] and r['status']=='completed' for r in steered['runs']) and
            len([i for i in steered['task_instruction'] if i['task_id']==france(final)['task_id']])==2 and
            any(variant['events'][1]['text'] in m.get('text','') for g in final['generations'] if g['run_id']==first['run_id']
                for m in strict_json(g['request_json'])['messages']),
        root_publication_barriers=all(g['worker_requests_before_main_completion']==0 and g['root_recall_frozen_before_main_publication'] is True and
            g['main_completed_before_worker_dispatch'] is True and all(g['boundaries'].values()) for g in
            (row['seed_barrier'],row['nz_guard']['scheduling'],row['france_guard']['scheduling'],row['group_barrier'])),
        readonly_world_tool=all(any(t['run_id']==a['run_id'] and t['tool_name']=='mcp__fixture__read' and t['status']=='succeeded' and
            strict_json(t['arguments_json'])==dict(text='world') and strict_json(strict_json(t['result_json'])['text'])==case['world'] for t in final['tools']) for a in (first,last)),
        artifacts_reopened=len(row['artifacts'])==len(final['artifact'])==4 and all(
            a['descriptor']==d and a['sha256']==sha(a['text'].encode())==d['content_hash'] and a['bytes']==len(a['text'].encode())
            for a,d in zip(row['artifacts'],final['artifact'])) and
            exact(strict_json(next(a['text'] for a in row['artifacts'] if a['descriptor']['artifact_ref']==first['artifact_ref'])),doc1) and
            exact(strict_json(next(a['text'] for a in row['artifacts'] if a['descriptor']['artifact_ref']==last['artifact_ref'])),doc2),
        confirmed_artifacts_immutable=initial['artifact']==final['artifact'][:2] and all(any(
            all(d[k]==a['descriptor'][k] for k in ('artifact_ref','name','media_type','byte_size','content_hash','version'))
            for a in row['artifacts']) for d in row['originals']) and
            [a['text'] for a in row['artifacts'][:2]]==['Confirmed Seine cruise 2026-10-03 19:00','Confirmed Louvre 2026-10-04 09:30'],
        group_lineage_accessible=any(r['artifact_ref']==first['artifact_ref'] and r['holder_kind']=='task' and r['holder_id']==last['task_id'] for r in final['artifact_reference']),
        main_observes_artifacts=all(any(a['artifact_ref'] in m.get('text','') for g in final['generations']
            if next(r['kind'] for r in final['runs'] if r['run_id']==g['run_id'])=='main'
            for m in strict_json(g['request_json'])['messages'] if m.get('origin')=='runtime') for a in (first,last)),
        published_task_results=all(any(u['root_run_id']==a['run_id'] and a['artifact_ref'] in strict_json(u['artifact_refs']) for u in final['task_update']) for a in (first,last)),
        input_and_report_kill=row['input_restart']['returncode']==row['report_restart']['returncode']==-9 and
            row['input_restart']['boundary']=='after_generation_commit' and row['report_restart']['boundary']=='after_report_commit',
        committed_generation_reused=all(any(
            g['generation_id']==before['generation_id'] and g['result_json']==before['result_json'] and
            g['attempt_count']==before['attempt_count']==1 for g in final['generations'])
            for before in row['input_restart']['state']['generations'] if before['status']=='completed'),
        committed_report_reused=all(any(u['event_id']==before['event_id'] and u['root_run_id']==before['root_run_id']
            for u in final['task_update']) for before in row['report_restart']['state']['task_update']) and
            any(r['run_id']==last['run_id'] and r['status']=='completed' for r in row['report_restart']['state']['runs']),
        no_unauthorized_action=not final['approval'] and not final['automation'] and all(t['tool_name'] in {
            'mcp__fixture__echo','mcp__fixture__read','builtin__task__manage','builtin__task__search',
            'builtin__codemode__execute','builtin__artifact__manage'} for t in final['tools']),
        no_duplicate_dispatch=all(t['attempt_count']<=1 for t in final['tools']) and all(g['attempt_count']<=1 for g in final['generations']),
        exact_replay=exact(final,row['replayed']))
    return checks


def rejection_probes(row):
    def native_return(r):
        art = next(a for a in r['final']['artifact'] if a['name']=='itinerary_1005.json')
        tool = next(t for t in r['final']['tools'] if t['run_id']==art['run_id'] and t['tool_name']=='builtin__codemode__execute' and art['name'] in t['arguments_json'])
        output = strict_json(strict_json(tool['result_json'])['text'])
        next(p['value'] for p in output if p['type']=='return')['result']['return_eta']='20:10'
        tool['result_json']=json.dumps(dict(text=json.dumps(output)))
    def rehash(r):
        a=next(a for a in r['artifacts'] if a['descriptor']['name']=='itinerary_1005.json')
        doc=strict_json(a['text']);doc['return_eta']='20:10';a['text']=json.dumps(doc)
        a.update(sha256=sha(a['text'].encode()),bytes=len(a['text'].encode()));a['descriptor']['content_hash']=a['sha256']
        next(d for d in r['final']['artifact'] if d['artifact_ref']==a['descriptor']['artifact_ref'])['content_hash']=a['sha256']
    def synced(r,mutate):
        art,document=native_document(r['final'],'itinerary_1005.json')
        mutate(document)
        # Change every persisted representation consistently. The independent
        # locked-world oracle must still reject it, not just a cached summary.
        text=json.dumps(document,ensure_ascii=False)
        for tool in r['final']['tools']:
            if tool['run_id']!=art['run_id']:continue
            if tool['tool_name']=='builtin__codemode__execute' and art['name'] in tool['arguments_json']:
                output=strict_json(strict_json(tool['result_json'])['text'])
                next(p['value'] for p in output if p['type']=='return')['result']=document
                tool['result_json']=json.dumps(dict(text=json.dumps(output)))
            if tool['tool_name']=='builtin__artifact__manage':
                args=strict_json(tool['arguments_json'])
                if args.get('operation')=='save':
                    args['text']=text;tool['arguments_json']=json.dumps(args)
        art.update(content_hash=sha(text.encode()),byte_size=len(text.encode()))
        opened=next(a for a in r['artifacts'] if a['descriptor']['artifact_ref']==art['artifact_ref'])
        opened.update(text=text,sha256=art['content_hash'],bytes=len(text.encode()),descriptor=copy.deepcopy(art))
        r['replayed']=copy.deepcopy(r['final'])
    mutations=dict(
        changed_input=lambda r:r['variant']['events'][0].update(text='替换原文'),
        native_eta=native_return,
        synced_late_eta=lambda r:synced(r,lambda d:d.update(return_eta='20:10')),
        synced_missing_sample=lambda r:synced(r,lambda d:d['source'].pop('sampled_at')),
        synced_countryside=lambda r:synced(r,lambda d:d['route'][1].update(place='南部乡村')),
        synced_changed_confirmed=lambda r:synced(r,lambda d:d['confirmed'].clear()),
        synced_booking_conflation=lambda r:synced(r,lambda d:d['needs_booking'].clear()),
        synced_wrong_transit=lambda r:synced(r,lambda d:d['transit_minutes'].update(hotel_return=5)),
        synced_false_purchase=lambda r:synced(r,lambda d:d.update(booking_actions=['paid'])),
        rehashed_artifact=rehash,
        unexpected_tool_failure=lambda r:r['final']['tools'][0].update(status='failed'),
        unknown_tool_outcome=lambda r:r['final']['tools'][0].update(status='unknown'),
        native_view_drift=lambda r:r['final']['task'].clear(),
        forged_source=lambda r:r['search'].update(invocation_id='forged'),
        wrong_task=lambda r:r.update(france_ref=r['nz_ref']),
        new_third_task=lambda r:r['final']['tasks'].append(r['final']['tasks'][0]),
        revived_paused_owner=lambda r:next(n for n in r['final']['runs'] if n['error_code']=='task_paused').update(status='completed'),
        wrong_epoch=lambda r:next(n for n in r['final']['runs'] if n['task_epoch']==2).update(task_epoch=1),
        discarded_steering=lambda r:r['steered']['task_instruction'].clear(),
        group_prepublication_dispatch=lambda r:r['group_barrier'].update(worker_requests_before_main_completion=1),
        prepublication_dispatch=lambda r:r['france_guard']['scheduling'].update(worker_requests_before_main_completion=1),
        corrupt_artifact=lambda r:r['artifacts'][0].update(sha256='0'*64),
        false_restart=lambda r:r['report_restart'].update(returncode=0),
        duplicate_attempt=lambda r:r['final']['tools'][0].update(attempt_count=2),
        unauthorized_action=lambda r:r['final']['tools'].append(dict(tool_name='buy_ticket')),
        missing_publication=lambda r:r['final']['task_update'].clear(),
        missing_delivery=lambda r:r['final']['deliveries'].clear(),
        lost_lineage=lambda r:r['final']['artifact_reference'].clear(),
        replay_drift=lambda r:r['replayed']['deliveries'].append({}))
    results={}
    for name,mutate in mutations.items():
        changed=copy.deepcopy(row)
        try:
            mutate(changed);results[name]=not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
            results[name]=True
        require(results[name],'GC-04 oracle accepted corruption: '+name)
    return results

def drive(binary, item, failure_evidence):
    variant_id, repetition = item
    row = dict(variant_id=variant_id,repetition=repetition)
    embeddings = Embeddings()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc04-') as temp:
            root = Path(temp)
            with personal(binary,root,inputs(variant_id)[0],embeddings) as h:
                try:
                    journey(h,variant_id,row)
                except BaseException as error:
                    row['failure']=dict(kind=type(error).__name__,message=str(error))
                    print('GC-04 first failure:',variant_id,repetition,row['failure'],flush=True)
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


def validate_report(report, binary=ROOT / 'bin/erii'):
    lock = load()
    expected = [(v['variant_id'], n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require(report.get('case_id') == 'GC-04' and report.get('status') == 'mechanical_pass' and
        report.get('release_ready') is False and 'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        report.get('fixture_lock') == lock and report.get('m6_lock') == load_contract(ROOT,'m6-v1')[2] and report.get('cleanup') is True,
        'complete locked ungraded GC-04 receipts required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']), 'GC-04 source/binary identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    for row in rows:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-04 native receipt')
    require(report.get('rejection_probes') == rejection_probes(rows[0]), 'missing/forged GC-04 corruption probes')
    return dict(case_id='GC-04',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=24)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc04-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-04',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=24,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc04_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc04_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-04 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-04 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-04',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=24,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-04 manifest/native summary drift')
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
    native_folder = args.evidence.parent / '.gc04-native'
    native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-04',status='inconclusive',release_ready=False,semantic_score=None,eligible_for_grading=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=dict(
            platform=platform.platform(),python=platform.python_version(),node=subprocess.check_output(['node','--version'],text=True).strip(),
            go=subprocess.check_output(['go','version'],text=True).strip(),binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-04; original fragments only; existing-Task/steering adapter incomplete',
            child_approval_unknown='No Child or purchase requested by frozen script. Read-only itinerary only; approval/unknown and tree cancellation remain dedicated conformance gates, not travel execution evidence.'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder / (args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex: report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-04 native assertion failure')
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
