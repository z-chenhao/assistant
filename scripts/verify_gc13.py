#!/usr/bin/env python3
"""GC-13 frozen booking journey; real browser mechanics, never semantic grades."""
import argparse
import base64
import copy
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import threading

from case_runner import load_contract, variants
from fixture_pool import run_cases
from m6_desktop import browser
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_cases import timestamp
from run_m6_cases import open_bytes, save_report
from terminal_driver import Terminal
from unscripted_cases import source_identity as shared_identity
from verify_code_recovery import read
from verify_computer_use import launch
from verify_gc01 import native, kill
from verify_gc03 import exact
# Reuse the existing native cell, approval and Main publication boundary driver.
from verify_gc07 import cell, returned, start_task, PREFIX, PW, file_hash
from verify_gc11 import environment
from verify_suite import GC13_VARIANTS
from verify_main import require
from verify_memory_maintenance import work
from verify_recall import main_completed, worker_requests

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT/'evals/fixtures/gc13-v1.lock.json'
REPEATS = 5
SELECTOR = '#submit[data-restaurant="R1"][data-name="松间·湖滨店"][data-date="2026-10-17"][data-time="19:00"][data-party="2"][data-price-pp="280"][data-deposit="0"][data-total="560"][data-terms="not specified in fixture"][data-addon="0"]'
SUBMIT = PREFIX+PW+'let outcome;try{await pw.locator('+json.dumps(SELECTOR,ensure_ascii=False)+').click();outcome="acknowledged";}catch(e){outcome=e.code;}return outcome;'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected, 'GC-13 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 19 and all(
        b['source']=='evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks), 'GC-13 frozen block drift')
    return lock


def inputs(variant_id=None):
    load()
    data, _, _ = load_contract(ROOT,'m6-v1')
    case = next(c for c in data['cases'] if c['case_id']=='GC-13')
    return (data,case) if variant_id is None else (case,next(v for v in variants(data,case) if v['variant_id']==variant_id))


def source_identity():
    value = shared_identity()
    for name in ('scripts/verify_gc13.py','evals/fixtures/gc13-v1.lock.json'):
        value['files'][name] = sha((ROOT/name).read_bytes())
    return value


def select(h,bridge):
    h.bridge=bridge
    h.args=lambda *args:[str(h.binary),'-config',str(h.root/'config.yaml'),'-test-bridge',bridge,*args]
    h.setup(metadata=dict(name='GC-13 explicit '+bridge,provider_kind='openai_compatible',endpoint='http://127.0.0.1:1/v1',
        wire_api='openai_completions',model_id='gc13-'+bridge+'-fixture',context_window=65536,max_output_tokens=4096,
        generation_defaults={},compatibility=dict(tool_result_format='openai'),capabilities=dict(text_input=True,image_input=True)))


def external(root):
    return strict_json((root/'external-state.json').read_bytes())


def view(url,path,selector):
    return PREFIX+'await tab.goto('+json.dumps(url+path)+');\n'+PW+'''const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);
return {state:view.ax.state,data:JSON.parse(await pw.locator(SELECTOR).textContent()),computer:(await cua.getState()).computerId};'''.replace('SELECTOR',json.dumps(selector))


def cancel_before_submit(h,row):
    """Main cancels a Root held at actual desktop admission; no external write."""
    publish,worker,computer=[h.root/('cancel-'+name+'.release') for name in ('publish','worker','computer')]
    receipt=start_task(h,'cancel-booking',SUBMIT,'GC-13 cancel before submit')
    baseline=worker_requests(h);seen=dict(after_task_commit=0,before_worker_generation=0);control={};held={}
    db=Path(h.env['ERII_DATA_ROOT'])/'state.db'
    def observe(name):
        if name in seen:seen[name]+=1
        if name=='before_worker_generation' and not publish.exists():publish.touch(exist_ok=False)
        if name=='computer_ready' and 'task_id' not in held:
            held['computer']=h.command('computer','status')
            owner=read(db,'SELECT execution_id FROM computer')[0]['execution_id']
            held['task_id']=read(db,'SELECT r.task_id FROM run r JOIN tool_invocation i USING(run_id) WHERE i.invocation_id=?',(owner,))[0]['task_id']
            held['state']=native(h)
            task=next(t for t in held['state']['task'] if t['task_id']==held['task_id'])
            plan=[dict(name='builtin__task__manage',arguments=dict(operation='cancel',taskRef=task['task_ref']))]
            control['receipt']=h.send('cancel-booking-owner','Capabilities: '+json.dumps(plan))
    def advance():
        if publish.exists() and not worker.exists() and main_completed(h,receipt['run_id']):
            count=worker_requests(h)-baseline
            require(count==0 and all(seen.values()),'GC-13 Worker bypassed Main publication')
            held['scheduling']=dict(worker_requests_before_main_completion=count,boundaries=copy.deepcopy(seen))
            worker.touch(exist_ok=False)
        if control and main_completed(h,control['receipt']['run_id']):
            require(read(db,'SELECT status FROM task WHERE task_id=?',(held['task_id'],))[0]['status']=='cancelled','GC-13 cancel did not commit')
            computer.touch(exist_ok=True)
    try:
        result=h.drive_boundaries({'after_task_commit':publish,'before_worker_generation':worker,'computer_ready':computer},observe,advance)
        require(result.returncode==0 and control and computer.exists(),'GC-13 cancellation boundary failed: '+result.stderr)
        row['cancel']=dict(held,returncode=result.returncode,state=native(h),computer=h.command('computer','status'))
    finally:
        for path in (publish,worker,computer):path.touch(exist_ok=True)


def journey(h,peer,root,variant,row):
    select(h,'terminal-input')
    h.send('context',variant['context'])
    event=variant['events'][0];text=event['at']+' '+event['text']
    row['input_receipt']=h.send('event-0',text)
    row['input_restart']=kill(h,'after_generation_commit')
    h.command('chat','work');work(h)
    select(h,'computer')
    # This is an explicit scripted Child plan, not evidence of model selection.
    child='Capabilities: '+json.dumps([dict(name='builtin__codemode__execute',arguments=dict(source='return '+json.dumps(inputs()[1]['world'],ensure_ascii=False)+';'))],ensure_ascii=False)
    row['confirmation']=cell(h,'final-confirmation','const child=await agent.run({task:'+json.dumps(child,ensure_ascii=False)+'});\n'+
        view(peer['url'],'/booking','#confirmation').replace('return {state:','return {child,state:'),task='GC-13 prepare final confirmation')
    row['before']=external(root)
    row['denied']=cell(h,'denied-submit',SUBMIT,approve=False)
    row['after_denial']=external(root)
    # Separate failure probes leave the original world/wordings and Gold intact.
    row['challenge']=cell(h,'user-challenge',view(peer['url'],'/booking/auth','#challenge'))
    row['handoff']=cell(h,'sensitive-step-handoff','return {needs_user:true,reason:"请用户在浏览器完成验证码或密码；完成前不提交",submitted:false};')
    row['after_challenge']=external(root)
    row['extras']=cell(h,'observe-default-addon',view(peer['url'],'/booking?probe=addons','#confirmation'))
    row['decline']=cell(h,'decline-addon',PREFIX+PW+'await pw.locator("#addon").click();await tab.capabilities.list();const fresh=await tab.capabilities.get("playwright");return JSON.parse(await fresh.locator("#confirmation").textContent());')
    row['ready']=cell(h,'recheck-final',view(peer['url'],'/booking','#confirmation'))
    row['preapproval']=native(h)
    # Original script step 3 arrives only after the final page is observable.
    select(h,'terminal-input')
    with Terminal(h.args('tui'),h.env,h.root) as terminal:
        terminal.expect('State: ready');terminal.expect('You> ')
        terminal.send('用户批准。');terminal.expect('assistant: ');terminal.expect('You> ')
        require(terminal.stop()==0,'GC-13 TUI did not stop')
        row['transcript']=terminal.transcript()
    work(h);select(h,'computer')
    row['submitted']=cell(h,'submit-unknown',SUBMIT,crash_after_approval=True)
    row['unknown_state']=native(h)
    row['after_submit']=external(root)
    row['reconciled']=cell(h,'query-orders',view(peer['url'],'/orders','#orders'))
    row['report']=cell(h,'booking-receipt',PREFIX+'await tab.goto('+json.dumps(peer['url']+'/orders')+');\n'+PW+save_report('''
const orders=JSON.parse(await pw.locator("#orders").textContent());
const result={orders,confirmation:orders[0].order_id,original_invocation:"unknown",verification:"read-only My Orders",live_verified:false};''','booking-receipt.json'))
    cancel_before_submit(h,row)
    row['post_cancel']=cell(h,'post-cancel-orders',view(peer['url'],'/orders','#orders'))
    row['after_cancel']=external(root)
    work(h);row['final']=native(h);row['computer']=h.command('computer','status')
    row['artifacts']=[]
    for descriptor in row['final']['artifact']:
        raw=open_bytes(h,descriptor['artifact_ref'])
        row['artifacts'].append(dict(descriptor=descriptor,data_b64=base64.b64encode(raw).decode()))
    require(h.send('event-0',text)==row['input_receipt'],'GC-13 duplicate receipt drift')
    row['drift_exit']=h.command('chat','send','event-0',text='changed booking',ok=False).returncode
    h.command('chat','work');work(h)
    row['replayed']=native(h);row['external_replayed']=external(root)


CALLS=('confirmation','denied','challenge','handoff','extras','decline','ready','submitted','reconciled','report','post_cancel')


def observe(row):
    case,variant=inputs(row['variant_id']);final=row['final'];tools=final['tools'];runs=final['runs']
    vals={key:returned(row[key],final) for key in CALLS}
    by_run={r['run_id']:r for r in runs};by_tool={t['invocation_id']:t for t in tools}
    original=[variant['context']]+[e['at']+' '+e['text'] for e in variant['events']]
    users=[p['text'] for m in final['conversation'] if m['role']=='user' for p in m['content']]
    positions=[users.index(t) if t in users else -1 for t in original+['用户批准。']]
    orders=[dict(order_id='order_781',restaurant='R1',date='2026-10-17',time='19:00',party=2)]
    unknown=[t for t in tools if t['status']=='unknown'];approvals=final['approval']
    expected=dict(restaurant=case['world']['restaurants'][0],request=case['world']['request'],total_cny=560,
        terms='not specified in fixture',addon_cny=0,login='fixture-session')
    checks=dict(
        original_inputs=exact(row['variant'],variant) and min(positions)>=0 and positions==sorted(set(positions)) and all(users.count(s)==1 for s in original+['用户批准。']),
        original_model_requests=all(any(s==m.get('text') for g in final['generations'] for m in strict_json(g['request_json'])['messages']) for s in original+['用户批准。']),
        final_confirmation=exact(vals['confirmation']['data'],expected) and exact(vals['ready']['data'],expected) and all(
            t in vals['confirmation']['state'] for t in ('松间·湖滨店','2026-10-17','19:00','560','无押金','not specified in fixture')),
        no_submit_before_approval=row['before']['orders']==row['after_denial']['orders']==row['after_challenge']['orders']==[] and
            row['before']['submissions']==row['after_denial']['submissions']==row['after_challenge']['submissions']==0 and vals['denied']=='APPROVAL_DENIED',
        challenge_handoff=exact(vals['challenge']['data'],dict(needs_user=True,kind='captcha_or_password')) and vals['handoff']['needs_user'] is True and
            vals['handoff']['submitted'] is False and any('请用户在浏览器完成验证码或密码' in p.get('text','') for m in final['conversation'] if m['role']=='assistant' for p in m['content']) and
            not any(strict_json(t['arguments_json']).get('method')=='playwright.fill' for t in tools),
        default_addon_removed=vals['extras']['data']['addon_cny']==99 and exact(vals['decline'],expected),
        approval_causal_order='用户批准。' not in json.dumps(row['preapproval']['conversation'],ensure_ascii=False) and '用户批准。' in row['transcript'],
        lost_ack=vals['submitted']=='UNKNOWN_SIDE_EFFECT' and len(unknown)==1 and type(unknown[0]['attempt_count']) is int and unknown[0]['attempt_count']==1 and
            unknown[0]['replay_policy']=='never' and unknown[0] in row['unknown_state']['tools'],
        only_one_exact_order=all(exact(row[k]['orders'],orders) for k in ('after_submit','after_cancel','external_replayed')) and
            row['after_submit']['submissions']==row['after_cancel']['submissions']==row['external_replayed']['submissions']==1,
        readonly_reconciliation=vals['reconciled']['data']==orders and 'order_781' in vals['reconciled']['state'] and all(
            strict_json(t['arguments_json']).get('method') not in ('playwright.click','playwright.fill') for t in row['reconciled']['nested']),
        stable_computer=len({vals[k]['computer'] for k in ('confirmation','challenge','extras','ready','reconciled','post_cancel')})==1,
        input_crash=row['input_restart']['boundary']=='after_generation_commit' and type(row['input_restart']['returncode']) is int and row['input_restart']['returncode']==-9,
        effect_crash=row['submitted']['crash']['boundary']=='after_tool_commit' and row['submitted']['crash']['returncode']==-9 and
            len([t for t in row['submitted']['crash']['state']['tools'] if t['status']=='unknown'])==1,
        replay_and_drift=exact(final,row['replayed']) and type(row['drift_exit']) is int and row['drift_exit']!=0,
        all_runs_settled=all(r['status'] in ('completed','cancelled') for r in runs),
        one_delivery_per_reply=len(final['deliveries'])==len([m for m in final['conversation'] if m['role']=='assistant']) and all(d['status']=='sent' for d in final['deliveries']),
        frozen_requests=bool(final['generations']) and all(strict_json(g['request_json'])['model_snapshot']['snapshot_id']==by_run[g['run_id']]['model_snapshot_id'] for g in final['generations']),
        idle_checkpoint=row['computer']['phase']=='checkpointed',
        no_secret='fixture-only' not in json.dumps(final,ensure_ascii=False))
    for key,selector in (('confirmation','#confirmation'),('challenge','#challenge'),('extras','#confirmation'),('ready','#confirmation'),('reconciled','#orders'),('post_cancel','#orders')):
        reads=[t for t in row[key]['nested'] if strict_json(t['arguments_json']).get('method')=='playwright.textContent']
        checks[key+'_native_dom']=len(reads)==1 and strict_json(reads[0]['arguments_json'])['args'][0]['selector']==selector and exact(
            strict_json(strict_json(strict_json(reads[0]['result_json'])['text'])['value']),vals[key]['data'])
    decisions=[d for key in CALLS for d in row[key]['decisions']]
    checks['frozen_approvals']=len(decisions)==len(approvals) and bool(decisions) and all(
        d['approval']['status']=='pending' and type(d['invocation']['attempt_count']) is int and d['invocation']['attempt_count']==0 and d['approval']['arguments']==d['invocation']['arguments_json']==by_tool[d['invocation']['invocation_id']]['arguments_json'] and
        d['approval']['frozen_arguments_hash']==by_tool[d['invocation']['invocation_id']]['request_hash'] for d in decisions) and all(
        a['frozen_arguments_hash']==by_tool[a['invocation_id']]['request_hash'] and by_tool[a['invocation_id']]['replay_policy']=='never' and
        by_tool[a['invocation_id']]['attempt_count']==(0 if a['status']=='denied' else 1) and a['status'] in ('approved','denied') for a in approvals)
    submit_args=next(d['approval']['arguments'] for d in row['submitted']['decisions'])
    checks['business_parameters_frozen']=SELECTOR in submit_args.replace('\\"','"') and all(term in submit_args for term in ('R1','松间·湖滨店','2026-10-17','19:00','280','560','not specified in fixture','data-addon'))
    checks['unknown_never_reused']=all(t in tools for t in row['unknown_state']['tools'] if t['status'] in ('succeeded','unknown') and t['tool_name']!='builtin__codemode__execute')
    checks['known_tool_outcomes']=all(type(t['attempt_count']) is int and t['status'] in ('succeeded','failed','cancelled','unknown') and
        (t['status']!='failed' or strict_json(t['result_json']).get('error',{}).get('code')=='APPROVAL_DENIED') for t in tools)
    children=[r for r in runs if r['parent_run_id'] is not None]
    checks['worker_tree']=len(children)==1 and children[0]['status']=='completed' and by_run[children[0]['parent_run_id']]['kind']=='worker' and children[0]['model_snapshot_id']==by_run[children[0]['parent_run_id']]['model_snapshot_id'] and all(r['parent_run_id'] is None for r in runs if r['kind']=='main')
    agents=[t for t in tools if t['tool_name']=='builtin__agent__run']
    checks['direct_parent_result']=len(agents)==1 and exact(strict_json(strict_json(agents[0]['result_json'])['text']),vals['confirmation']['child']) and set(vals['confirmation']['child'])=={'text','artifactRefs'}
    caps={c['snapshot_id']:c for c in final['capabilities']}
    checks['child_reduced']=bool(children) and not any(t['name'] in ('builtin__task__manage','builtin__memory__manage','builtin__automation__manage') for t in strict_json(caps[children[0]['capability_snapshot_id']]['tools_json']))
    checks['main_no_child_capability']=all(not c['can_spawn_agents'] and not any(t['name']=='builtin__agent__run' for t in strict_json(c['tools_json'])) for c in caps.values() if c['role']=='main')
    cancel=row['cancel'];task=next(t for t in final['task'] if t['task_id']==cancel['task_id']);old=[r for r in runs if r['task_id']==task['task_id']]
    checks['cancel_fences_epoch']=cancel['returncode']==0 and task['status']=='cancelled' and bool(old) and all(r['status']=='cancelled' and type(r['task_epoch']) is int and r['task_epoch']<task['task_epoch'] for r in old) and not any(
        strict_json(t['arguments_json']).get('method')=='playwright.click' for t in tools if t['run_id'] in {r['run_id'] for r in old})
    checks['publication_barriers']=row['confirmation']['scheduling']['worker_requests_before_main_completion']==cancel['scheduling']['worker_requests_before_main_completion']==0
    checks['task_lifecycle']=len(final['task'])==2 and sorted(t['status'] for t in final['task'])==['cancelled','completed']
    report=vals['report'];expected_report=dict(orders=orders,confirmation='order_781',original_invocation='unknown',verification='read-only My Orders',live_verified=False)
    artifacts=row['artifacts'];descriptors=final['artifact']
    checks['artifact_bytes']=len(artifacts)==len(descriptors) and bool(artifacts) and all(
        a['descriptor'] in descriptors and len(base64.b64decode(a['data_b64'],validate=True))==a['descriptor']['byte_size'] and sha(base64.b64decode(a['data_b64'],validate=True))==a['descriptor']['content_hash'] for a in artifacts)
    receipt=[a for a in artifacts if a['descriptor']['artifact_ref'] in report['artifactRefs']]
    checks['confirmation_from_order_evidence']=exact(report['result'],expected_report) and len(receipt)==1 and exact(strict_json(base64.b64decode(receipt[0]['data_b64'])),expected_report) and any(
        'order_781' in p.get('text','') for m in final['conversation'] if m['role']=='assistant' for p in m['content'])
    checks['real_screenshots']=sum(base64.b64decode(a['data_b64'],validate=True).startswith(b'\x89PNG\r\n\x1a\n') for a in artifacts)==6
    checks['typed_external_counters']=all(type(row[k]['submissions']) is int for k in ('before','after_denial','after_challenge','after_submit','after_cancel','external_replayed'))
    checks['exact_approval_inventory']=len(approvals)==10 and sorted(a['status'] for a in approvals)==['approved']*9+['denied']
    submits=[t for t in tools if strict_json(t['arguments_json']).get('method')=='playwright.click' and
        strict_json(t['arguments_json']).get('args',[{}])[0].get('selector')==SELECTOR]
    checks['exact_submit_inventory']=len(submits)==2 and sorted(t['status'] for t in submits)==['failed','unknown'] and sorted(t['attempt_count'] for t in submits)==[0,1]
    checks['completed_generations_reused']=all(g in final['generations'] for g in row['input_restart']['state']['generations'] if g['status']=='completed')
    checks['model_usage_settled']=len(final['usage'])==len(final['generations']) and all(g['status']=='completed' and type(g['attempt_count']) is int and g['attempt_count']==1 for g in final['generations']) and all(not u['error_code'] for u in final['usage'])
    checks['code_claims_released']=bool(final['code_execution_state']) and all(c['state'] in ('completed','cancelled') and c['claim_token'] is None and c['owner_run_token'] is None for c in final['code_execution_state'])
    shots=[(p['data'],row[key]['parent']['run_id']) for key in CALLS for p in row[key]['output'] if p['type']=='image']
    checks['screenshot_lineage']=len(shots)==6 and all(any(a['artifact_ref']==ref and a['run_id']==run for a in descriptors) for ref,run in shots)
    saves=[t for t in row['report']['nested'] if t['tool_name']=='builtin__artifact__manage' and strict_json(t['arguments_json']).get('operation')=='save']
    checks['receipt_save_lineage']=len(saves)==1 and strict_json(saves[0]['result_json'])['artifact_refs']==report['artifactRefs'] and exact(strict_json(strict_json(saves[0]['arguments_json'])['text']),expected_report) and all(
        a['descriptor']['run_id']==row['report']['parent']['run_id'] and any(r['artifact_ref']==a['descriptor']['artifact_ref'] and r['holder_kind']=='tool' and r['holder_id']==saves[0]['invocation_id'] for r in final['artifact_reference']) for a in receipt)
    return checks


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc13-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    # Stream complete native bytes into a lossless archive. A full series is
    # hundreds of MiB of repeated request/state snapshots; materializing a second
    # giant JSON string or requiring that much free disk is unnecessary.
    uncompressed_bytes = 0
    with raw.open('wb') as target, gzip.GzipFile(fileobj=target,mode='wb',compresslevel=1,mtime=0) as archive:
        for chunk in json.JSONEncoder(ensure_ascii=False).iterencode(report):
            data = chunk.encode('utf-8');uncompressed_bytes += len(data);archive.write(data)
        archive.write(b'\n');uncompressed_bytes += 1
    summary = dict(case_id='GC-13',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=19,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc13_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        native_encoding='gzip',native_uncompressed_bytes=uncompressed_bytes,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','native_encoding','native_uncompressed_bytes','proof_limit'} and
        manifest['kind'] == 'gc13_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-13 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-13 native size mismatch')
    size = manifest['native_uncompressed_bytes']
    require(manifest['native_encoding']=='gzip' and type(size) is int and 0<size<=2<<30,
        'GC-13 bounded gzip native receipt required')
    with gzip.GzipFile(fileobj=io.BytesIO(raw)) as archive:
        decoded = archive.read(size+1)
    require(len(decoded)==size,'GC-13 decompressed size mismatch')
    report = strict_json(decoded)
    expected = dict(case_id='GC-13',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=19,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-13 manifest/native summary drift')
    return report



def rejection_probes(row):
    require(all(observe(row).values()),'GC-13 probes require a passing native receipt')
    mutations=dict(
        original_input=lambda r:r['variant']['events'][0].update(text='changed'),
        original_model=lambda r:r['final']['generations'].clear(),
        altered_return=lambda r:r['ready']['value']['data'].update(total_cny=1),
        wrong_branch=lambda r:r['after_submit']['orders'][0].update(restaurant='R2'),
        duplicated_effect=lambda r:r['after_cancel'].update(submissions=2),
        denial_effect=lambda r:r['after_denial'].update(submissions=1),
        unknown_retry=lambda r:next(t for t in r['final']['tools'] if t['status']=='unknown').update(attempt_count=2),
        boolean_attempt=lambda r:next(t for t in r['final']['tools'] if t['status']=='unknown').update(attempt_count=True),
        false_crash=lambda r:r['submitted']['crash'].update(returncode=0),
        false_input_restart=lambda r:r['input_restart'].update(returncode=0),
        early_worker=lambda r:r['cancel']['scheduling'].update(worker_requests_before_main_completion=1),
        old_epoch=lambda r:next(n for n in r['final']['runs'] if n['task_id']==r['cancel']['task_id']).update(task_epoch=999),
        approval_forgery=lambda r:r['submitted']['decisions'][0]['approval'].update(arguments='forged'),
        artifact_bytes=lambda r:r['artifacts'][0].update(data_b64=base64.b64encode(b'forged').decode()),
        artifact_descriptor=lambda r:r['artifacts'][0]['descriptor'].update(content_hash='0'*64),
        missing_delivery=lambda r:r['final']['deliveries'].pop(),
        replay_drift=lambda r:r['replayed']['tools'].pop(),
        false_drift=lambda r:r.update(drift_exit=0),
        boolean_effect_count=lambda r:r['after_denial'].update(submissions=False),
        retained_code_claim=lambda r:r['final']['code_execution_state'][0].update(claim_token='stale'),
        missing_usage=lambda r:r['final']['usage'].pop(),
        screenshot_owner=lambda r:r['artifacts'][0]['descriptor'].update(run_id='forged'),
        child_escalation=lambda r:next(c for c in r['final']['capabilities'] if c['snapshot_id']==next(n['capability_snapshot_id'] for n in r['final']['runs'] if n['parent_run_id'])).update(tools_json='[{"name":"builtin__task__manage"}]'))
    result={}
    for name,mutate in mutations.items():
        changed=copy.deepcopy(row)
        try:
            mutate(changed);result[name]=not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):result[name]=True
        require(result[name],'GC-13 accepted corruption: '+name)
    return result


def execution_ids(row):
    state=row['final']
    return {name:[r[key] for r in state[table]] for name,table,key in (
        ('runs','runs','run_id'),('tools','tools','invocation_id'),('generations','generations','generation_id'),
        ('messages','conversation','message_id'),('deliveries','deliveries','delivery_id'),('artifacts','artifact','artifact_ref'))}


def pairs(part=None):
    require(part is None or part in GC13_VARIANTS,'unknown GC-13 CI part')
    ids = sum(GC13_VARIANTS.values(), ()) if part is None else GC13_VARIANTS[part]
    return [(key,n) for key in ids for n in range(1,REPEATS+1)]


def validate_report(report,binary=ROOT/'bin/erii',*,part=None):
    expected=pairs(part)
    require(report.get('ci_part')==part,'GC-13 report/selector drift')
    require(report.get('case_id')=='GC-13' and report.get('status')=='mechanical_pass' and report.get('release_ready') is False and
        'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and report.get('cleanup') is True and
        report.get('fixture_lock')==load() and report.get('m6_lock')==load_contract(ROOT,'m6-v1')[2], 'complete locked ungraded GC-13 receipts required')
    ident=report['identity']
    require(ident['source_before']==ident['source_after']==source_identity() and ident['binary_sha256']==sha(binary.read_bytes()) and
        exact(ident['environment'],environment(binary)), 'GC-13 source/binary/environment drift')
    rows=report['results']
    require([(r['variant_id'],r['repetition']) for r in rows]==expected and all(type(r['repetition']) is int for r in rows),'GC-13 all selected variants and five ordered repetitions required')
    seen={}
    for row in rows:
        checks=observe(row)
        require(exact(row['assertions'],checks) and all(type(v) is bool and v for v in checks.values()) and exact(
            row['cleanup'],dict(disposable_root_removed=True,owned_processes_joined=True)), 'failed/forged GC-13 native receipt')
        for key,ids in execution_ids(row).items():
            require(bool(ids) and all(type(i) is str and i for i in ids) and len(set(ids))==len(ids) and not seen.setdefault(key,set()).intersection(ids),'GC-13 copied/invalid execution identities: '+key)
            seen[key].update(ids)
        require(isinstance(row['started_at'],str) and row['started_at']<row['finished_at'],'GC-13 execution timestamps required')
    require(exact(report.get('rejection_probes'),rejection_probes(rows[0])),'GC-13 corruption probes required')
    return dict(case_id='GC-13',status='mechanical_pass',receipts=len(rows),frozen_blocks=19,cleanup=True,semantic_score=None,eligible_for_grading=False,release_ready=False)


def drive(binary,item,failure_evidence):
    variant_id,repetition=item;case,variant=inputs(variant_id)
    row=dict(variant_id=variant_id,repetition=repetition,variant=variant,started_at=timestamp())
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc13-') as temp:
            root=Path(temp).resolve()
            with browser(root,case['world']) as (peer,browser_root),launch(binary,root,browser_peer=peer['endpoint']) as h:
                try:journey(h,peer,browser_root,variant,row)
                except BaseException:
                    row['failure_state']=h.failure_state()
                    calls=browser_root/'browser-calls.jsonl'
                    row['browser_calls']=[strict_json(line) for line in calls.read_bytes().splitlines()] if calls.exists() else []
                    raise
            joined=all(c.poll() is not None for c in h.children)
        row.update(finished_at=timestamp(),cleanup=dict(disposable_root_removed=not root.exists(),owned_processes_joined=joined))
        row['assertions']=observe(row)
    except BaseException as error:
        failure_evidence.parent.mkdir(parents=True,exist_ok=True)
        row['error']=str(error)
        with gzip.open(failure_evidence,'wt',encoding='utf-8',compresslevel=1) as target:json.dump(row,target,ensure_ascii=False)
        summary=dict(kind='gc13_failure',variant_id=variant_id,repetition=repetition,error=str(error)[:2048],stages=[k for k in CALLS if k in row])
        failure_evidence.parent.parent.joinpath(failure_evidence.stem+'.summary.json').write_text(json.dumps(summary,ensure_ascii=False)+'\n')
        raise
    return row


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True);parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--validate',action='store_true')
    parser.add_argument('--ci-part',choices=tuple(GC13_VARIANTS))
    parser.add_argument('--variant-jobs',type=int,choices=(1,2,4),default=int(os.environ.get('ERII_VERIFY_CASE_JOBS','2')))
    args=parser.parse_args();binary=args.binary.resolve()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),binary,part=args.ci_part)));return
    args.evidence.unlink(missing_ok=True);args.evidence.parent.mkdir(parents=True,exist_ok=True)
    folder=args.evidence.parent/'.gc13-native';folder.mkdir(exist_ok=True)
    report=dict(case_id='GC-13',ci_part=args.ci_part,status='inconclusive',release_ready=False,eligible_for_grading=False,semantic_score=None,cleanup=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=environment(binary)),
        remaining=dict(parent='ERII-223',current_L2='unexecuted',semantic_score=None,release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-13; original input fragment only; approved booking adapter still required',
            proof_limit='Real Chromium/production browser adapter, explicit Fake model/Lume/SSH; choice, handoff and report are scripted. No real account/VM or semantic quality claim.'))
    ids=list(dict.fromkeys(key for key,_ in pairs(args.ci_part)));mutex=threading.Lock()
    def series(variant_id):
        for n in range(1,REPEATS+1):
            row=drive(binary,(variant_id,n),folder/(variant_id+f'-{n}.failure.gz'))
            with mutex:report['results'].append(row)
            print((variant_id,n),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-13 native assertion failure')
    try:
        run_cases(ids,series,args.variant_jobs)
        report['results'].sort(key=lambda r:(ids.index(r['variant_id']),r['repetition']))
        report['identity']['source_after']=source_identity();report.update(status='mechanical_pass',cleanup=True)
        report['rejection_probes']=rejection_probes(report['results'][0]);summary=validate_report(report,binary,part=args.ci_part)
    finally:
        retain_report(args.evidence,report)
    print(json.dumps(summary))


if __name__=='__main__':main()
