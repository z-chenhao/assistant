#!/usr/bin/env python3
"""GC-07 frozen network journey and native receipts; scripted, never grades."""
import argparse
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
import threading

from case_runner import load_contract, variants
from fixture_pool import run_cases
from m6_desktop import browser
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_m6_cases import execute, remember, open_bytes, save_report
from terminal_driver import Terminal
from unscripted_cases import source_identity as shared_identity
from verify_code_recovery import read
from verify_computer_use import launch
from verify_gc01 import native, kill
from verify_gc02 import recorded
from verify_gc03 import exact
from verify_main import require
from verify_memory_maintenance import work
from verify_recall import Embeddings

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / 'evals/fixtures/gc07-v1.lock.json'
REPEATS = 5
PREFIX = 'const tabs=await cua.listTabs();const tab=await cua.getTab(tabs[0].id);\n'
PW = 'await tab.capabilities.list();const pw=await tab.capabilities.get("playwright");\n'


def sha(value):
    return hashlib.sha256(value).hexdigest()


def load():
    lock = strict_json(LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected, 'GC-07 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 20 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks),
        'GC-07 frozen block drift')
    return lock


def inputs(variant_id=None):
    load()
    data, _, _ = load_contract(ROOT,'m6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-07')
    return (data,case) if variant_id is None else (case,next(v for v in variants(data,case) if v['variant_id'] == variant_id))


def source_identity():
    value = shared_identity()
    for name in ('scripts/verify_gc07.py','evals/fixtures/gc07-v1.lock.json'):
        value['files'][name] = sha((ROOT/name).read_bytes())
    return value


def topology(current=True):
    world = copy.deepcopy(inputs()[1]['world'])
    if not current:
        world['network']['dhcp_gateway'] = '192.168.0.1'
    return dict(topic='家庭网络',kind='当前拓扑',world=world)


def cell(h,key,source,crash_after_approval=False,**options):
    # execute()'s last Code return may belong to a nested Child. Select this
    # cell's first newly accepted execution, then bind its persisted native rows.
    db = Path(h.env['ERII_DATA_ROOT'])/'state.db'
    before = {t['invocation_id'] for t in read(db,'SELECT * FROM tool_invocation')}
    crash = None
    if crash_after_approval:
        h.send(key,'Capabilities: '+json.dumps([dict(name='builtin__codemode__execute',arguments=dict(source=source))]))
        h.command('chat','work')
        approvals = [a for a in h.command('approval','list') if a['status']=='pending']
        require(len(approvals)==1,'GC-07 crash probe requires one frozen action')
        decisions = []
        for approval in approvals:
            invocation = read(db,'SELECT * FROM tool_invocation WHERE invocation_id=?',
                (read(db,'SELECT invocation_id FROM approval WHERE approval_id=?',(approval['approval_id'],))[0]['invocation_id'],))[0]
            decisions.append(dict(approval=approval,invocation=invocation))
            require(invocation['attempt_count']==0,'GC-07 crash action dispatched before approval')
            h.command('approval','approve',approval['approval_id'],approval['frozen_arguments_hash'])
        crash = kill(h,'after_tool_commit')
        h.command('chat','work')
        executed = dict(decisions=decisions)
    else:
        executed = execute(h,key,source,**options)
    rows = read(db,'SELECT * FROM tool_invocation ORDER BY created_at,rowid')
    parent = next(t for t in rows if t['invocation_id'] not in before and t['tool_name'] == 'builtin__codemode__execute')
    require(parent['status'] == 'succeeded','GC-07 outer cell failed')
    output = strict_json(strict_json(parent['result_json'])['text'])
    returned = [p['value'] for p in output if p['type'] == 'return']
    require(len(returned) == 1,'GC-07 requires one cell return')
    return dict(parent=parent,output=output,value=returned[0],nested=[t for t in rows if t['parent_invocation_id'] == parent['invocation_id']],
        decisions=executed['decisions'],crash=crash)


def observation(url):
    return PREFIX+'await tab.goto('+json.dumps(url+'/network')+');\n'+PW+'''
const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);
return {state:view.ax.state,topology:JSON.parse(await pw.locator("#topology").textContent()),
network:JSON.parse(await pw.locator("#network").textContent()),computer:(await cua.getState()).computerId};'''


def report_source(url):
    return PREFIX+'await tab.goto('+json.dumps(url+'/network')+');\n'+PW+save_report('''
const topology=JSON.parse(await pw.locator("#topology").textContent());
const network=JSON.parse(await pw.locator("#network").textContent());
const result={diagnosis:"gateway_or_link",topology,network,
restored:["gateway","ordinary","proxy","apple_tv"].every(k=>network[k]===true)&&network.protocols===1,
provider_acknowledged:false,verification:"sandbox_readback",live_verified:false};''','network.json')


def action(selector):
    return PREFIX+PW+'let result="acknowledged";try{await pw.locator('+json.dumps(selector)+').click();}catch(e){result=e.code;}return result;'


def external(root):
    return strict_json((root/'external-state.json').read_bytes())


def select(h,bridge):
    h.bridge = bridge
    h.args = lambda *args: [str(h.binary),'-config',str(h.root/'config.yaml'),'-test-bridge',h.bridge,*args]
    h.setup(metadata=dict(name='GC-07 explicit '+bridge,provider_kind='openai_compatible',
        endpoint='http://127.0.0.1:1/v1',wire_api='openai_completions',model_id='gc07-'+bridge+'-fixture',
        context_window=65536,max_output_tokens=4096,generation_defaults={},
        compatibility=dict(tool_result_format='openai'),capabilities=dict(text_input=True,image_input=True)))


def journey(h,peer,browser_root,variant,row):
    config = strict_json((h.root/'config.yaml').read_bytes())
    config.update(memory=dict(encryption_key_ref='env:ERII_MEMORY_KEY'),embedding=dict(provider='openai_compatible',
        endpoint=row.pop('embedding_endpoint'),model='semantic-fixture-v1',dimensions=5))
    (h.root/'config.yaml').write_text(json.dumps(config))
    h.env['ERII_MEMORY_KEY'] = 'M'*32
    select(h,'computer')
    row['old_ref'] = remember(h,'old-topology',json.dumps(topology(False),ensure_ascii=False))
    row['current_ref'] = remember(h,'current-topology',json.dumps(topology(),ensure_ascii=False),row['old_ref'])
    select(h,'terminal-input')
    h.send('context',variant['context'])
    first = variant['events'][0]
    row['input_receipt'] = h.send('event-0',first['at']+' '+first['text'])
    row['input_restart'] = kill(h,'after_generation_commit')
    h.command('chat','work');work(h)
    select(h,'computer')
    child_plan = 'Capabilities: '+json.dumps([dict(name='builtin__codemode__execute',arguments=dict(source='return (await tools.builtin__memory__search({query:"家庭网络",limit:20})).filter(h=>h.source_type==="memory"&&h.content.startsWith("{")&&JSON.parse(h.content).kind==="当前拓扑");'))],ensure_ascii=False)
    row['diagnosis'] = cell(h,'diagnose',
        'const child=await agent.run({task:'+json.dumps(child_plan,ensure_ascii=False)+'});\n'+
        'const hits=(await tools.builtin__memory__search({query:"家庭网络",limit:20})).filter(h=>h.source_type==="memory"&&h.content.startsWith("{"));\n'+
        observation(peer['url']).replace('return {state:', 'return {child,hits,diagnosis:"gateway_or_link",vpn_provider_fault:null,state:'),task='GC-07 diagnosis')
    row['before'] = external(browser_root)
    row['diagnosed'] = native(h)
    select(h,'terminal-input')
    event = variant['events'][1]
    with Terminal(h.args('tui'),h.env,h.root) as terminal:
        terminal.expect('State: ready');terminal.expect('You> ')
        terminal.send(event['at']+' '+event['text'])
        terminal.expect('assistant: ');terminal.expect('You> ')
        require(terminal.stop() == 0,'GC-07 TUI did not stop')
        row['transcript'] = terminal.transcript()
    work(h)
    select(h,'computer')
    row['denied'] = cell(h,'denied-repair',action('#repair-fail'),approve=False)
    row['after_denial'] = external(browser_root)
    row['failed'] = cell(h,'failed-repair',action('#repair-fail'))
    row['failure_observation'] = cell(h,'observe-failure',observation(peer['url']))
    row['after_failure'] = external(browser_root)
    row['rollback'] = cell(h,'rollback',action('#rollback'))
    row['rollback_observation'] = cell(h,'observe-rollback',observation(peer['url']))
    row['after_rollback'] = external(browser_root)
    row['unknown'] = cell(h,'repair-unknown',action('#repair-unknown'),crash_after_approval=True)
    row['unknown_state'] = native(h)
    # Every CLI work call is a new process. Persisted unknown stays unknown;
    # reconciliation reads business state without retrying that invocation.
    row['verified'] = cell(h,'verify-all-paths',observation(peer['url']))
    row['after_repair'] = external(browser_root)
    row['report'] = cell(h,'network-report',report_source(peer['url']))
    # A held old desktop owner competes with an independent Task. The second
    # waits at the desktop barrier; Main then cancels the first before it resumes.
    held_plan = json.dumps([dict(name='builtin__codemode__execute',arguments=dict(source=PREFIX+PW+'await pw.locator("#repair-fail").click();return "must not write";'))])
    contender_source = 'await cua.listTabs();return "observed_after_release";'
    pending = {}
    def contender():
        db = Path(h.env['ERII_DATA_ROOT'])/'state.db'
        pending['before'] = {t['invocation_id'] for t in read(db,'SELECT * FROM tool_invocation')}
        plan = [dict(name='builtin__task__manage',arguments=dict(operation='create',task='GC-07 independent desktop observation\nCapabilities: '+json.dumps([
            dict(name='builtin__codemode__execute',arguments=dict(source=contender_source))])))]
        h.send('desktop-contender','Capabilities: '+json.dumps(plan))
        child = h.processes.start(h.args('chat','work'),stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,env=h.env,cwd=h.root)
        h.children.append(child);pending['child'] = child
        row['contended_state'] = h.wait_state(lambda state:any(t['invocation_id'] not in pending['before'] and t['status']=='waiting' and t['tool_name']=='builtin__computer__discover' for t in state['tools']),
            'independent Task waits for desktop',reader=native,timeout=20)
    original_hold = h.hold
    def hold_and_contend(*args,**kwargs):
        child = original_hold(*args,**kwargs)
        contender()
        return child
    h.hold = hold_and_contend
    try:
        row['cancel'] = h.cancel_at('computer_ready',held_plan)
    finally:
        h.hold = original_hold
    row['ownership_after_cancel'] = read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT * FROM computer')[0]
    child = pending['child'];_,err = child.communicate(timeout=30)
    require(child.returncode == 0,'GC-07 contender failed: '+err)
    h.processes.retire(child)
    rows = read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT * FROM tool_invocation ORDER BY created_at,rowid')
    parent = next(t for t in rows if t['invocation_id'] not in pending['before'] and t['tool_name']=='builtin__codemode__execute')
    output = strict_json(strict_json(parent['result_json'])['text'])
    row['contender'] = dict(parent=parent,output=output,value=next(p['value'] for p in output if p['type']=='return'),
        nested=[t for t in rows if t['parent_invocation_id']==parent['invocation_id']])
    row['cancelled'] = h.cancelled_state(row['cancel']['task_id'])
    row['after_cancel'] = external(browser_root)
    work(h)
    row['final'] = native(h)
    row['actions'] = strict_json((browser_root/'network-actions.json').read_bytes())
    row['computer'] = h.command('computer','status')
    row['artifacts'] = []
    for descriptor in row['final']['artifact']:
        raw = open_bytes(h,descriptor['artifact_ref'])
        row['artifacts'].append(dict(descriptor=descriptor,sha256=sha(raw),bytes=len(raw),
            text=raw.decode() if descriptor['media_type']=='application/json' else None,
            png=raw.startswith(b'\x89PNG\r\n\x1a\n'),data_b64=base64.b64encode(raw).decode()))
    require(h.send('event-0',first['at']+' '+first['text']) == row['input_receipt'],'GC-07 input receipt drift')
    h.command('chat','work');work(h)
    row['replayed'] = native(h)
    row['actions_replayed'] = strict_json((browser_root/'network-actions.json').read_bytes())


def returned(call,final):
    output = strict_json(strict_json(call['parent']['result_json'])['text'])
    values = [p['value'] for p in output if p['type'] == 'return']
    require(len(values) == 1 and exact(output,call['output']) and exact(values[0],call['value']), 'GC-07 native return drift')
    require(recorded(call['parent'],final['tools']) and all(recorded(t,final['tools']) for t in call['nested']), 'GC-07 native Tool drift')
    return values[0]


def observe(row):
    case,variant = inputs(row['variant_id']);final = row['final']
    names = ('diagnosis','denied','failed','failure_observation','rollback','rollback_observation','unknown','verified','report','contender')
    values = {name:returned(row[name],final) for name in names}
    original = [variant['context']]+[e['at']+' '+e['text'] for e in variant['events']]
    users = [p['text'] for m in final['conversation'] if m['role']=='user' for p in m['content']]
    positions = [users.index(s) if s in users else -1 for s in original]
    network = dict(ethernet='Not Connected',gateway=False,ordinary=False,proxy=False,apple_tv=False,protocols=1)
    fixed = dict(ethernet='Connected',gateway=True,ordinary=True,proxy=True,apple_tv=True,protocols=1)
    diagnosis = values['diagnosis'];verified = values['verified']
    hits = [h for h in diagnosis['hits'] if strict_json(h['content']).get('kind') == '当前拓扑']
    expected_topology = dict(network=case['world']['network'],devices=case['world']['devices'],interface='Ethernet',ip=case['world']['network']['mac_gateway'])
    tools = final['tools'];runs = final['runs'];by_run = {r['run_id']:r for r in runs}
    approvals = final['approval']
    actions = row['actions']
    checks = dict(
        original_inputs=exact(row['variant'],variant) and min(positions)>=0 and positions==sorted(set(positions)) and all(users.count(s)==1 for s in original),
        original_model_requests=all(any(s==m.get('text') for g in final['generations'] for m in strict_json(g['request_json'])['messages']) for s in original),
        tui_input=variant['events'][1]['text'] in row['transcript'],
        current_topology=len(hits)==1 and hits[0]['source_ref']==row['current_ref'] and exact(strict_json(hits[0]['content']),topology()) and
            exact({k:v for k,v in diagnosis['topology'].items() if k!='physical_link'},expected_topology),
        link_before_action=exact(diagnosis['network'],network) and diagnosis['topology']['physical_link']=='Not Connected' and
            diagnosis['diagnosis']=='gateway_or_link' and diagnosis['vpn_provider_fault'] is None and
            'interrupts gateway' in diagnosis['state'] and exact(row['before']['network'],network),
        no_effect_during_diagnosis=not any(t['tool_name']=='builtin__computer__invoke' and 'click' in t['arguments_json'] for t in row['diagnosed']['tools']),
        denial_no_effect=values['denied']=='APPROVAL_DENIED' and exact(row['before'],row['after_denial']),
        failure_observed=values['failed']=='acknowledged' and values['failure_observation']['network']==row['after_failure']['network'] and
            row['after_failure']['network']==dict(network,ethernet='Connected',protocols=2),
        safe_rollback=values['rollback']=='acknowledged' and exact(row['after_rollback'],row['before']) and exact(values['rollback_observation']['network'],network),
        four_paths_one_protocol=exact(verified['network'],fixed) and exact(row['after_repair']['network'],fixed) and
            verified['topology']['physical_link']=='Connected',
        unknown_not_claimed=values['unknown']=='UNKNOWN_SIDE_EFFECT' and values['report']['result']['provider_acknowledged'] is False and
            values['report']['result']['live_verified'] is False and values['report']['result']['verification']=='sandbox_readback',
        action_ledger=[a['action'] for a in actions]==['failed-repair','rollback','repair-unknown'] and
            exact(actions[0]['before'],network) and exact(actions[1]['before'],row['after_failure']['network']) and exact(actions[2]['before'],network),
        restart_reuses_generation=row['input_restart']['returncode']==-9 and row['input_restart']['boundary']=='after_generation_commit' and
            all(g in final['generations'] for g in row['input_restart']['state']['generations'] if g['status']=='completed'),
        native_replay=exact(final,row['replayed']) and exact(actions,row['actions_replayed']),
        cancelled_owner=row['cancel']['old_owner_exit']==0 and row['cancelled']['task'][0]=='cancelled' and
            all(r[1]=='cancelled' and r[2]=='task_cancelled' for r in row['cancelled']['runs']) and
            exact(row['after_cancel'],row['after_repair']) and row['ownership_after_cancel']['execution_id'] in (None,row['contender']['parent']['invocation_id']),
        desktop_contention=values['contender']=='observed_after_release' and any(t['status']=='waiting' and t['tool_name']=='builtin__computer__discover' and t['attempt_count']==0 for t in row['contended_state']['tools']) and len({r['task_id'] for r in row['contended_state']['runs'] if r['kind']=='worker'})>=2,
        desktop_released=row['computer']['phase']=='checkpointed',
        one_delivery_per_reply=len(final['deliveries'])==len([m for m in final['conversation'] if m['role']=='assistant']) and all(d['status']=='sent' for d in final['deliveries']),
        frozen_requests=all(strict_json(g['request_json'])['model_snapshot']['snapshot_id']==by_run[g['run_id']]['model_snapshot_id'] for g in final['generations']),
        no_secret='fixture-only' not in json.dumps(final,ensure_ascii=False) and 'M'*32 not in json.dumps(final,ensure_ascii=False))
    for name in ('diagnosis','failure_observation','rollback_observation','verified','report'):
        call = row[name]
        view = values[name]['result'] if name=='report' else values[name]
        reads = {}
        for t in call['nested']:
            args = strict_json(t['arguments_json'])
            if args.get('method')=='playwright.textContent':
                reads[args['args'][0]['selector']] = strict_json(strict_json(strict_json(t['result_json'])['text'])['value'])
        checks[name+'_native_dom'] = exact(reads,{'#network':view['network'],'#topology':view['topology']})
    decisions = [d for name in names for d in row[name].get('decisions',[])]
    checks['approval_pending_no_dispatch'] = len(decisions)==len(approvals)==9 and all(
        d['approval']['status']=='pending' and d['invocation']['attempt_count']==0 and
        d['approval']['arguments']==d['invocation']['arguments_json']==next(t['arguments_json'] for t in tools if t['invocation_id']==d['invocation']['invocation_id']) and
        d['approval']['frozen_arguments_hash']==next(a['frozen_arguments_hash'] for a in approvals if a['approval_id']==d['approval']['approval_id']) for d in decisions)
    checks['known_tool_outcomes'] = all(t['status'] in ('succeeded','failed','cancelled','unknown') and
        (t['status']!='failed' or strict_json(t['result_json']).get('error',{}).get('code')=='APPROVAL_DENIED') and
        (t['attempt_count']>=1 if t['tool_name'] in ('builtin__codemode__execute','builtin__agent__run') else t['attempt_count'] in (0,1)) for t in tools)
    checks['snapshot_lineage'] = all(c['model_snapshot_id']==by_run[c['parent_run_id']]['model_snapshot_id'] for c in runs if c['parent_run_id'] is not None)
    old = next(m for m in final['memory']['memory']['records'] if 'memory:'+m['memory_id']==row['old_ref'])
    current = next(m for m in final['memory']['memory']['records'] if 'memory:'+m['memory_id']==row['current_ref'])
    checks['memory_lineage'] = current['supersedes_memory_id']==old['memory_id'] and all(m['content'].startswith('sealed:v1:') for m in (old,current))
    unknowns = [t for t in tools if t['status']=='unknown']
    crash = row['unknown']['crash']
    checks['effect_crash_recovered'] = crash['returncode']==-9 and crash['boundary']=='after_tool_commit' and len([t for t in crash['state']['tools'] if t['status']=='unknown'])==1 and all(t in final['tools'] for t in crash['state']['tools'] if t['status'] in ('succeeded','unknown') and t['tool_name']!='builtin__codemode__execute')
    checks['unknown_never_retry'] = len(unknowns)==1 and unknowns[0]['attempt_count']==1 and unknowns[0] in row['unknown_state']['tools'] and unknowns[0]['replay_policy']=='never'
    checks['frozen_approval'] = len(approvals)==9 and sorted(a['status'] for a in approvals)==['approved']*8+['denied'] and all(
        a['frozen_arguments_hash']==next(t['request_hash'] for t in tools if t['invocation_id']==a['invocation_id']) and
        next(t['replay_policy'] for t in tools if t['invocation_id']==a['invocation_id'])=='never' and
        next(t['attempt_count'] for t in tools if t['invocation_id']==a['invocation_id'])==(0 if a['status']=='denied' else 1) for a in approvals)
    children = [r for r in runs if r['parent_run_id'] is not None]
    checks['direct_parent_child'] = (len(children)==1 and children[0]['kind']=='worker' and children[0]['status']=='completed' and
        by_run[children[0]['parent_run_id']]['kind']=='worker' and children[0]['model_snapshot_id']==by_run[children[0]['parent_run_id']]['model_snapshot_id'])
    agent_calls = [t for t in row['diagnosis']['nested'] if t['tool_name']=='builtin__agent__run']
    checks['child_result_to_parent'] = (len(agent_calls)==1 and exact(strict_json(strict_json(agent_calls[0]['result_json'])['text']),diagnosis['child']) and
        row['old_ref'] not in diagnosis['child']['text'] and row['current_ref'] in diagnosis['child']['text'])
    child_caps = [strict_json(c['tools_json']) for c in final['capabilities'] if children and c['snapshot_id']==children[0]['capability_snapshot_id']]
    checks['child_reduced'] = len(child_caps)==1 and not any(t['name'] in ('builtin__task__manage','builtin__memory__manage','builtin__automation__manage') for t in child_caps[0])
    checks['main_no_child'] = all(r['parent_run_id'] is None for r in runs if r['kind']=='main') and all(
        by_run[t['run_id']]['kind']=='worker' for t in tools if t['tool_name']=='builtin__agent__run')
    report = values['report'];report_refs = report['artifactRefs']
    artifact = next(a for a in row['artifacts'] if a['descriptor']['artifact_ref'] in report_refs)
    expected = dict(diagnosis='gateway_or_link',topology=verified['topology'],network=fixed,restored=True,
        provider_acknowledged=False,verification='sandbox_readback',live_verified=False)
    checks['report_readback'] = exact(report['result'],expected) and exact(strict_json(artifact['text']),expected) and artifact['sha256']==sha(artifact['text'].encode())
    checks['artifacts_reopened'] = len(row['artifacts'])==len(final['artifact']) and all(a['sha256']==a['descriptor']['content_hash'] and a['bytes']==a['descriptor']['byte_size'] for a in row['artifacts']) and sum(a['png'] for a in row['artifacts'])==4 and all(sha(base64.b64decode(a['data_b64'],validate=True))==a['sha256'] and len(base64.b64decode(a['data_b64'],validate=True))==a['bytes'] for a in row['artifacts'])
    checks['artifact_native_descriptors'] = all(any(exact(a['descriptor'],d) for d in final['artifact']) and
        a['png'] == base64.b64decode(a['data_b64'],validate=True).startswith(b'\x89PNG\r\n\x1a\n') for a in row['artifacts'])
    checks['artifact_secret_absent'] = all(b'fixture-only' not in base64.b64decode(a['data_b64'],validate=True) and b'M'*32 not in base64.b64decode(a['data_b64'],validate=True) for a in row['artifacts'])
    return checks


def environment(binary):
    return dict(platform=platform.platform(),python=platform.python_version(),
        node=subprocess.check_output(['node','--version'],text=True).strip(),
        go=subprocess.check_output(['go','version'],text=True).strip(),
        binary_build='\n'.join(subprocess.check_output(['go','version','-m',str(binary)],text=True).splitlines()[1:]))


def validate_report(report, binary=ROOT / 'bin/erii'):
    lock = load()
    expected = [(v['variant_id'], n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require(report.get('case_id') == 'GC-07' and report.get('status') == 'mechanical_pass' and
        report.get('release_ready') is False and 'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        report.get('fixture_lock') == lock and report.get('m6_lock') == load_contract(ROOT,'m6-v1')[2] and report.get('cleanup') is True,
        'complete locked ungraded GC-07 receipts required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and exact(identity['environment'],environment(binary)), 'GC-07 source/binary identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    for row in rows:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-07 native receipt')
    require(report.get('rejection_probes') == rejection_probes(rows[0]), 'missing/forged GC-07 corruption probes')
    return dict(case_id='GC-07',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=20)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc07-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-07',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=20,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc07_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc07_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-07 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-07 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-07',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=20,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-07 manifest/native summary drift')
    return report


def rejection_probes(row):
    require(all(observe(row).values()),'GC-07 corruption probes require passing native baseline')
    def native_change(r):
        call=r['verified'];parent=call['parent'];envelope=strict_json(parent['result_json']);output=strict_json(envelope['text'])
        next(p for p in output if p['type']=='return')['value']['network']['gateway']=False
        envelope['text']=json.dumps(output,ensure_ascii=False);changed=json.dumps(envelope,ensure_ascii=False)
        for t in r['final']['tools']+r['replayed']['tools']:
            if t['invocation_id']==parent['invocation_id']:t['result_json']=changed
        parent['result_json']=changed;call['output']=output;call['value']=next(p['value'] for p in output if p['type']=='return')
    mutations = dict(
        native_forgery=native_change,
        artifact_bytes=lambda r:r['artifacts'][0].update(data_b64=base64.b64encode(b'forged PNG').decode()),
        artifact_descriptor=lambda r:r['artifacts'][0]['descriptor'].update(visibility='public'),
        original_input=lambda r:r['variant']['events'][0].update(text='changed'),
        missing_conversation=lambda r:r['final']['conversation'].pop(0),
        native_return=lambda r:r['verified']['value']['network'].update(gateway=False),
        stale_topology=lambda r:r['diagnosis']['value']['hits'].clear(),
        duplicate_effect=lambda r:r['actions'].append(r['actions'][-1]),
        dirty_config=lambda r:r['after_repair']['network'].update(protocols=2),
        failed_rollback=lambda r:r['after_rollback']['network'].update(ordinary=True),
        denial_effect=lambda r:r['after_denial']['network'].update(gateway=True),
        unknown_retry=lambda r:next(t for t in r['final']['tools'] if t['status']=='unknown').update(attempt_count=2),
        unapproved=lambda r:next(a for a in r['final']['approval'] if a['status']=='denied').update(status='approved'),
        child_task_control=lambda r:next(c for c in r['final']['capabilities'] if c['snapshot_id']==next(n['capability_snapshot_id'] for n in r['final']['runs'] if n['parent_run_id'] is not None)).update(tools_json='[{"name":"builtin__task__manage"}]'),
        cancelled_write=lambda r:r['after_cancel']['network'].update(protocols=2),
        false_effect_crash=lambda r:r['unknown']['crash'].update(returncode=0),
        false_restart=lambda r:r['input_restart'].update(returncode=0),
        replay_drift=lambda r:r['replayed']['deliveries'].append({}),
        artifact_hash=lambda r:r['artifacts'][0].update(sha256='0'*64),
        secret_leak=lambda r:r['final']['trace'].append(dict(secret='fixture-only')))
    result = {}
    for name,mutate in mutations.items():
        changed = copy.deepcopy(row)
        try:
            mutate(changed);result[name] = not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):result[name] = True
        require(result[name],'GC-07 consumer accepted corruption: '+name)
    return result


def drive(binary,item,failure_evidence):
    variant_id,repetition = item
    case,variant = inputs(variant_id)
    row = dict(variant_id=variant_id,repetition=repetition,variant=variant)
    embeddings = Embeddings()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc07-') as temp:
            root = Path(temp).resolve()
            with browser(root,case['world']) as (peer,browser_root), launch(binary,root,browser_peer=peer['endpoint']) as h:
                row['embedding_endpoint'] = f'http://127.0.0.1:{embeddings.server.server_port}/v1'
                try:journey(h,peer,browser_root,variant,row)
                except BaseException:
                    row['failure_state'] = h.failure_state()
                    raise
        row['cleanup'] = dict(disposable_root_removed=not root.exists(),owned_processes_joined=True)
        row['assertions'] = observe(row)
    except BaseException:
        failure_evidence.parent.mkdir(parents=True,exist_ok=True)
        failure_evidence.write_text(json.dumps(row,ensure_ascii=False)+'\n')
        raise
    finally:embeddings.close()
    return row


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--validate',action='store_true')
    parser.add_argument('--variant-jobs',type=int,choices=(1,2,4),default=int(os.environ.get('ERII_VERIFY_CASE_JOBS','2')))
    args = parser.parse_args();binary = args.binary.resolve()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),binary)));return
    args.evidence.unlink(missing_ok=True)
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    native_folder = args.evidence.parent/'.gc07-native';native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-07',status='inconclusive',release_ready=False,semantic_score=None,eligible_for_grading=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=environment(binary)),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-07; original input fragments; device/network adapter still required',
            proof_limit='Real Chromium/production browser adapter; explicit Fake Lume/SSH. Diagnostic choice and report are scripted. No real home network/native macOS or model-quality claim.'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder/(args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex:report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-07 native assertion failure')
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


if __name__ == '__main__':main()
