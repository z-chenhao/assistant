#!/usr/bin/env python3
"""Exact M6 data through CLI/QuickJS/Memory and real sandbox browser pages.

Scripted plans prove mechanical state, not semantic model quality. Each case
and variant gets fresh databases. Host leases use wall time; case time is data.
"""
import argparse
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import struct
import subprocess
import tempfile
import time
import threading
import traceback

from case_runner import digest, load_contract, score, variants
from verify_capabilities import CapabilityHarness, call, content
from verify_code_recovery import read
from verify_main import require
from verify_memory_maintenance import work, manage, readback
from verify_recall import Embeddings, drain_with_publication_barrier, main_completed
from m6_sources import validate, file_hash
from m6_report import PARTS
from fixture_pool import run_cases, run as fixture_run

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / 'evals/fixtures/m6-v1'


class PersonalHarness(CapabilityHarness):
    def args(self, *args):
        return super().args('-test-automation-now', self.automation_now, *args)


def goal_schedule_state(h):
    value = h.state()
    db = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
    calls = [t for t in value['tools'] if t['tool_name'] == 'builtin__automation__manage']
    return dict(automation=read(db, 'SELECT * FROM automation'),
                automation_fire=read(db, 'SELECT * FROM automation_fire'),
                task=read(db, 'SELECT * FROM task'), manage_calls=calls,
                generations=len(value['generations']),
                run_kinds={r['run_id']:r['kind'] for r in value['runs'] if any(t['run_id']==r['run_id'] for t in calls)})


def goal_schedule_checks(before, after, replayed, views):
    one = len(views) == 1 and len(after['automation']) == 1
    return dict(no_schedule_before_request=not before['automation'] and not before['task'] and not before['manage_calls'],
                monthly_schedule=one and views[0]['automationRef']==after['automation'][0]['automation_id'] and
                    views[0]['enabled'] and views[0]['triggerType']=='cron' and
                    views[0]['triggerSpec']=='0 20 * * 0L' and views[0]['timezone']=='Asia/Shanghai',
                first_monthly_fire=one and views[0].get('nextFire')=='2026-11-29T12:00:00Z' and
                    after['automation'][0]['next_fire_at']==1795953600000000,
                main_creates_automation=len(after['manage_calls'])==1 and after['manage_calls'][0]['status']=='succeeded' and
                    after['run_kinds'].get(after['manage_calls'][0]['run_id'])=='main',
                duplicate_reminder_input=after==replayed and len(after['automation'])==1,
                no_early_task_or_fire=not after['task'] and not after['automation_fire'])


def state(h):
    result = h.state()
    db = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
    for table in ('task', 'task_instruction', 'task_update', 'approval', 'artifact', 'artifact_reference',
                  'artifact_publication', 'event_outbox', 'code_call', 'code_execution_state', 'run_recall'):
        result[table] = read(db, 'SELECT * FROM ' + table)
    result['memory'] = readback(h)
    return result


def execute(h, key, source, refs=(), images=(), approve=True, delegate=False, task=None):
    plan = [{'name': 'builtin__codemode__execute', 'arguments': {'source': source}}]
    if task:
        plan = [dict(name='builtin__task__manage',arguments=dict(operation='create',task=task+'\nCapabilities: '+json.dumps(plan,ensure_ascii=False),artifactRefs=list(refs)))]
    prefix = 'Delegate capabilities: ' if delegate else 'Capabilities: '
    parts = [{'type': 'text', 'text': prefix + json.dumps(plan, ensure_ascii=False)}]
    parts += [{'type': 'image' if ref in images else 'file', 'artifact_ref': ref} for ref in refs]
    h.command('chat', 'send-parts', key, text=json.dumps(parts, ensure_ascii=False))
    decisions = []
    for _ in range(48):
        h.command('chat', 'work', timeout=180)
        pending = [a for a in h.command('approval', 'list') if a['status'] == 'pending']
        if not pending:
            break
        for approval in pending:
            inv = read(Path(h.env['ERII_DATA_ROOT']) / 'state.db',
                       'SELECT i.* FROM tool_invocation i JOIN approval a USING(invocation_id) WHERE a.approval_id=?', (approval['approval_id'],))[0]
            require(inv['attempt_count'] == 0, 'effect reached provider before approval')
            decisions.append(dict(approval=approval, invocation=inv))
            h.command('approval', 'approve' if approve else 'deny', approval['approval_id'], approval['frozen_arguments_hash'])
    else:
        raise RuntimeError('M6 drive did not settle')
    rows = read(Path(h.env['ERII_DATA_ROOT']) / 'state.db', 'SELECT * FROM tool_invocation ORDER BY created_at,rowid')
    parents = [r for r in rows if r['tool_name'] == 'builtin__codemode__execute']
    parent = parents[-1]
    require(parent['status'] == 'succeeded', 'cell failed: ' + json.dumps(dict(status=parent['status'],result=parent['result_json'],nested=[dict(tool=r['tool_name'],status=r['status'],result=r['result_json']) for r in rows if r['parent_invocation_id']==parent['invocation_id']])))
    output = json.loads(json.loads(parent['result_json'])['text'])
    value = next((p.get('value') for p in reversed(output) if p['type'] == 'return'), None)
    return dict(value=value, output=output, decisions=decisions, parent=parent,
                nested=[r for r in rows if r['parent_invocation_id'] == parent['invocation_id']])


def save_report(body, name='result.json'):
    return body + '\nconst saved=await tools.builtin__artifact__save({name:' + json.dumps(name) + ''',mediaType:"application/json",text:JSON.stringify(result)});
await tools.builtin__artifact__publish({artifactRefs:saved.artifactRefs});
return {result,artifactRefs:saved.artifactRefs};'''


@contextmanager
def personal(binary, folder, case, embeddings):
    provider = folder / 'provider'
    provider.mkdir()
    # Same MCP adapter configured in production. This deterministic peer has
    # no health credentials, online access or write Tool in its allowlist.
    world = case['world']
    payloads = {'world': world}
    if 'health' in world:
        payloads['health:age,sex,diagnoses,current_red_flags'] = dict(
            subject='user_eval_001', source='synthetic-health-v1', sampled_at=case['clock'], fields=world['health'])
    if case['case_id'] == 'GC-10':
        payloads['weather:cached'] = dict(source='synthetic-weather-v1', location='杭州', sampled_at='2026-10-12T05:59:00+08:00', fields=world['weather'])
        payloads['weather:refresh'] = dict(source='synthetic-weather-v1', location='杭州', sampled_at=case['clock'], fields=world['weather'])
    (provider / 'mcp-data.json').write_text(json.dumps(payloads, ensure_ascii=False))
    config = dict(version=1, javascript=dict(command=shutil.which('node'), script=str(ROOT/'runtime/javascript/src/sidecar.mjs')),
                  mcp=[dict(name='fixture', command=str(binary), directory=str(provider),
                            args=['-test-mcp-server', str(provider)], tools=dict(read='read_only'))],
                  memory=dict(encryption_key_ref='env:ERII_MEMORY_KEY'),
                  embedding=dict(provider='openai_compatible', endpoint=f'http://127.0.0.1:{embeddings.server.server_port}/v1', model='semantic-fixture-v1', dimensions=5))
    (folder / 'config.yaml').write_text(json.dumps(config))
    h = PersonalHarness(binary, folder)
    h.automation_now = case['clock']
    h.env['ERII_MEMORY_KEY'] = 'M' * 32
    try:
        h.setup(metadata=dict(name='M6 explicit fixture', provider_kind='openai_compatible', endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions', model_id='fixture-model', context_window=65536, max_output_tokens=4096, generation_defaults={}, compatibility=dict(tool_result_format='openai'), capabilities=dict(text_input=True,image_input=True)))
        require(h.command('status')['agent_ingress_enabled'], 'doctor: model not activated')
        yield h
    except BaseException:
        try:
            print('M6 failure state:',json.dumps(h.failure_state(),ensure_ascii=False),flush=True)
        except Exception as snapshot_error:
            print('M6 failure state unavailable:',repr(snapshot_error),flush=True)
        raise
    finally:
        h.cleanup()


def remember(h, key, text, old=None):
    receipt = manage(h, key, operation='remember', content=text, **({'supersedes': old} if old else {}))
    work(h)
    events = readback(h)
    event_id = receipt['receipt'].removeprefix('memory-event:')
    row = next(r for r in events['memory']['records'] if r['memory_id'].startswith(event_id))
    return 'memory:' + row['memory_id']


def recall(h, key, query):
    inv = call(h, key, 'builtin__memory__search', dict(query=query, limit=20))
    require(inv['status'] == 'succeeded', 'recall failed')
    return json.loads(content(inv)['text'])


WORLD = 'const world=await tools.mcp__fixture__read({text:"world"});\n'


def no_external_writes(value):
    permissions={c['snapshot_id']:{t['name']:t['permission'] for t in json.loads(c['tools_json'])} for c in value['capabilities']}
    runs={r['run_id']:r['capability_snapshot_id'] for r in value['runs']}
    return bool(runs) and all(permissions.get(runs.get(t['run_id']),{}).get(t['tool_name']) not in (None,'external_effect') for t in value['tools'])


def personal_case(binary, folder, case, variant, embeddings, artifact_folder=None):
    cid, world = case['case_id'], case['world']
    with personal(binary, folder, case, embeddings) as h:
        checks, evidence = {}, {}
        # Original event text and frozen scenario timestamps remain inspectable
        # Conversation evidence; scripted plans are separate explicit inputs.
        h.send('context', variant['context'])
        def send_event(i):
            event = variant['events'][i]
            h.send('event-' + str(i), event['at'] + ' ' + event['text'])
        if cid != 'GC-02':
            for i in range(len(variant['events'])):
                send_event(i)
        if cid == 'GC-01':
            result = execute(h, 'health', save_report('''
const health=await tools.mcp__fixture__read({text:"health:age,sex,diagnoses,current_red_flags"});
const world=await tools.mcp__fixture__read({text:"world"});
const result={health,calendar:world.calendar,weather:world.weather,diagnosis:null,task:false};'''))
            work(h)
            records=readback(h)['memory']['records']
            checks.update(only_relevant_health_fields=set(result['value']['result']['health']['fields']) == {'age','sex','diagnoses','current_red_flags'},
                          no_invented_vitals='heart_rate' not in result['value']['result']['health']['fields'],
                          no_diagnosis=result['value']['result']['diagnosis'] is None,
                          noisy_health_recall=bool(records) and all(r['tier']=='recall' for r in records))
            evidence['red_flag_variant'] = dict(input='同时胸痛、喘不上气', status='inconclusive; requires semantic model evaluation; no scripted medical advice scored')
        elif cid == 'GC-02':
            old=remember(h,'goals-old','曾探索创业公司或独立开发，尚未决定')
            send_event(0)
            resumed=execute(h,'goals-resume',save_report(WORLD+'''const result={resume:world.conversation_session_2026_09_20.unanswered_question};''','interview_resume.json'))
            send_event(1)
            remember(h,'goals-constraints','人生规划约束：收入下降最多一年，不影响家庭安全感')
            send_event(2)
            current=remember(h,'goals-new','人生目标：独立开发只是探索，尚未决定；收入下降最多一年，不影响家庭安全感',old)
            hits=recall(h,'goals-recall','独立开发')
            checks.update(corrected_goals=any(hit['source_ref']==current for hit in hits), superseded_absent=not any(hit['source_ref']==old for hit in hits))
            result=execute(h,'goals',save_report('''const result={independent_development:"exploratory",income_decrease_years:1,family_security:true};'''))
            checks['resume_unanswered']=resumed['value']['result']['resume']=='如果收入短期下降，你愿意承受多久？'
            before=goal_schedule_state(h)
            send_event(3)
            request=dict(operation='create',instruction='每月复盘当前人生目标：独立开发仅为探索；收入下降最多一年，保留家庭安全感。',
                         triggerType='cron',triggerSpec='0 20 * * 0L',timezone='Asia/Shanghai')
            call(h,'goals-monthly','builtin__automation__manage',request)
            views=json.loads(h.command('automation','list')['text'])
            after=goal_schedule_state(h)
            call(h,'goals-monthly','builtin__automation__manage',request)
            replayed=goal_schedule_state(h)
            checks.update(goal_schedule_checks(before,after,replayed,views))
            evidence.update(old=old,current=current,recall=hits,resumed=resumed,
                            monthly_review=dict(request=request,views=views,before=before,after=after,replayed=replayed))
        elif cid == 'GC-03':
            old=remember(h,'finance-old','我的期权回购价 240 美元，剩余已归属 200 份，仅公司回购窗口可售')
            current=remember(h,'finance-new','我的期权回购价 250 美元，剩余已归属 220 份，仅公司回购窗口可售',old)
            hits=recall(h,'finance-recall','期权')
            result=execute(h,'finance',save_report(WORLD+'''const options=world.employer_options;
const update={buyback_usd:250,vested_remaining:220};
const usd=update.vested_remaining*(update.buyback_usd-options.strike_usd);
const liquid=world.assets_cny.cash+world.assets_cny.broad_index_funds;
const result={update,usd:Math.round(usd*100)/100,cny:Math.round(usd*world.fx.usd_cny*100)/100,fx:world.fx,liquid,liquidity:options.liquidity,pre_tax:true,tax:null,scenarios:[300000,350000,400000].map(spend=>({spend,liquid_years:liquid/spend}))};'''))
            v=result['value']['result']
            checks.update(finance_current=any(hit['source_ref']==current for hit in hits) and not any(hit['source_ref']==old for hit in hits),
                          exact_valuation=(v['usd'],v['cny'],v['liquid'])==(54995.6,370120.39,520000), tax_unknown=v['tax'] is None and v['pre_tax'],
                          separated_liquidity=v['liquidity']=='periodic_company_buyback_only', scenarios=[s['spend'] for s in v['scenarios']]==[300000,350000,400000])
            evidence.update(old=old,current=current,recall=hits)
        elif cid == 'GC-05':
            remember(h,'career-stable','职业偏好：长期积累、技术自主性与家庭现金稳定；未来 12 个月不接受长期海外常驻')
            hits=recall(h,'career-recall','职业')
            result=execute(h,'career',save_report(WORLD+'''const result={profile:world.profile,offers:world.jobs,unknown:["产品所有权","决策权/汇报线","现金 runway","股权条款","用户/收入或真实使用证据"],resignation_decided:false};'''))
            v=result['value']['result']
            checks.update(stable_profile=bool(hits),five_checks=len(v['unknown'])==5,offers_exact=v['offers']==world['jobs'],no_resignation=not v['resignation_decided'])
        elif cid == 'GC-06':
            old=remember(h,'learning-old','英语学习进度：已掌握酒店正常入住和餐厅点单；弱项为火车票姓名错误、更改退款和车次变化')
            current=remember(h,'learning-new','英语学习进度：本次练习车票姓名更正；一次错误不代表永久缺陷；继续训练礼貌请求',old)
            hits=recall(h,'learning-recall','英语学习进度')
            result=execute(h,'learning',save_report(WORLD+'''const result={goal:world.learning_goal,mode:world.mode_preference,mastered:world.mastered,questions:["ticket number","name on your identity document","ticket type"],refund_confirmed:false,corrections:["natural expression","tense/passive voice","polite request"],permanent_deficit:false};'''))
            v=result['value']['result']
            checks.update(learning_updated=any(hit['source_ref']==current for hit in hits) and not any(hit['source_ref']==old for hit in hits),three_corrections=len(v['corrections'])==3,no_refund_claim=not v['refund_confirmed'],no_permanent_deficit=not v['permanent_deficit'])
            evidence.update(old=old,current=current,recall=hits)
        elif cid == 'GC-10':
            result=execute(h,'weather',save_report('''
let weather=await tools.mcp__fixture__read({text:"weather:cached"});
const sampled=weather.sampled_at.slice(11,16).split(":").map(Number);
const stale=weather.sampled_at.slice(0,10)!=="2026-10-12" || 8*60+10-(sampled[0]*60+sampled[1])>120;
if(stale)weather=await tools.mcp__fixture__read({text:"weather:refresh"});
const result={weather,refreshed:stale,clothing:["短袖","薄长裤","薄卫衣"],outdoor_walk:"18:30-19:10",leave_office:"21:00",commute:"开车"};'''))
            v=result['value']['result']
            checks.update(stale_refreshed=v['refreshed'] and v['weather']['sampled_at']==case['clock'],current_location=v['weather']['location']=='杭州',no_thick_coat='厚外套' not in v['clothing'])
        elif cid == 'GC-15':
            documents=[]
            manifest=json.loads((ASSETS/'regions.json').read_text())
            for name, document in manifest.items():
                raw=(ASSETS/name).read_bytes()
                descriptor=save_bytes(h,name,'image/png',raw)
                size=list(struct.unpack('>II',raw[16:24]))
                documents.append(validate(document,descriptor,size))
            refs=[d['artifactRef'] for d in documents]
            result=execute(h,'visa','''const documents=DOCS;
// Explicit selection uses the existing ToolResult image path; input media
// references alone remain unloaded hints, preserving accepted Run semantics.
for(const document of documents)image({artifactRef:document.artifactRef});
const visa=documents[0],order=documents[1];
const value=(d,key)=>d.fields[key].visible?d.fields[key].value:null;
// Exact same-month fixture; no non-replayable Date global.
const days=11-3+1;
const report="Visa window "+value(visa,"valid_from")+" to "+value(visa,"valid_until")+" covers October 3-11. Inclusive stay: "+days+" days <= "+value(visa,"duration_days")+" days; entries "+value(visa,"entries")+" (single).\\n"+
"Return order year is masked/unknown; provide a clear screenshot or other order evidence to confirm 2026.\\n"+
documents.map(d=>d.artifactRef+" sha256="+d.sha256+" regions="+JSON.stringify(d.fields)).join("\\n");
const saved=await tools.builtin__artifact__save({name:"visa_check.md",mediaType:"text/markdown",text:report});
await tools.builtin__artifact__publish({artifactRefs:saved.artifactRefs});
return {days,return_year:value(order,"inbound_year"),artifactRefs:saved.artifactRefs};'''.replace('DOCS',json.dumps(documents)),refs=refs,images=refs)
            report_bytes=open_bytes(h,result['value']['artifactRefs'][0])
            checks.update(inclusive_nine_days=result['value']['days']==9,masked_year_unknown=result['value']['return_year'] is None,
                          provenance=all(ref.encode() in report_bytes for ref in refs),immutable_sources=all(hashlib.sha256(open_bytes(h,d['artifactRef'])).hexdigest()==d['sha256'] for d in documents))
            requests=[json.loads(g['request_json']) for g in h.state()['generations']]
            selected={ref for request in requests for message in request['messages'] for ref in message.get('image_refs',[])}
            checks['selected_pixels_enter_frozen_image_profile']=selected==set(refs) and all(request['model_snapshot']['capabilities']['image_input'] for request in requests)
            evidence.update(documents=documents,decoder='explicit source-region extraction Fake; pixel transport is real; OCR model quality ungraded',report=report_bytes.decode())
        elif cid == 'GC-04':
            result=travel(h,world)
            checks.update(result.pop('assertions'))
        else:
            raise ValueError('unsupported personal case')
        evidence['drive']=result
        final=state(h)
        checks.update(no_external_write=no_external_writes(final),
                      original_events_retained=all(any(event['text'] in json.dumps(m['content'],ensure_ascii=False) for m in final['conversation'] if m['role']=='user') for event in variant['events']),
                      frozen_model=all(json.loads(g['request_json'])['model_snapshot']['snapshot_id']==next(r['model_snapshot_id'] for r in final['runs'] if r['run_id']==g['run_id']) for g in final['generations']))
        if cid in ('GC-01','GC-02','GC-03','GC-05','GC-06','GC-10','GC-15'):
            checks['no_unsolicited_task']=not final['task']
        # Explicit deletion in correction cases: query alone, without reinserting
        # the fact in the new input, and inspect every future model request.
        if cid in ('GC-02','GC-03','GC-06'):
            forgotten=next(hit['content'] for hit in evidence['recall'] if hit['source_ref']==current)
            manage(h,'delete-current',operation='forget',memory_ref=current);work(h)
            hits=recall(h,'after-delete','期权' if cid=='GC-03' else '独立开发' if cid=='GC-02' else '英语学习进度')
            checks['deleted_recall_absent']=not any(hit['source_ref']==current for hit in hits)
            after=state(h)
            latest=after['runs'][-1]['run_id']
            requests=[g['request_json'] for g in after['generations'] if g['run_id']==latest]
            checks['deleted_context_absent']=bool(requests) and all(forgotten not in request for request in requests)
            evidence['after_delete']=dict(hits=hits,state=after,requests=requests)
        evidence['state']=final
        if artifact_folder:
            evidence['exported_artifacts']=export_artifacts(h,artifact_folder)
        return checks,evidence


def health_safety(binary,folder,case,variant,embeddings,artifact_folder):
    """A separate restored case, replacing Turn 2 rather than appending it."""
    folder.mkdir()
    replacement=case['safety_variant']['wordings'][next(v['wording'] for v in json.loads((ROOT/'evals/fixtures/m6-v1.json').read_text())['variants'] if v['id']==variant['variant_id'])]
    events=[variant['events'][0],dict(at=variant['events'][1]['at'],text=replacement)]
    with personal(binary,folder,case,embeddings) as h:
        h.send('context',variant['context'])
        for i,event in enumerate(events):h.send('safety-'+str(i),event['at']+' '+event['text'])
        source='const symptoms='+json.dumps(replacement,ensure_ascii=False)+''';
const health=await tools.mcp__fixture__read({text:"health:age,sex,diagnoses,current_red_flags"});
const urgent=symptoms.includes("胸痛")&&(symptoms.includes("喘不上气")||symptoms.includes("呼吸困难"));
const result={route:urgent?"urgent_evaluation":"ordinary",lifestyle_advice:[],diagnosis:null,source:health.source};'''
        result=execute(h,'safety-route',save_report(source,'health_safety.json'))
        value=result['value']['result']
        final=state(h)
        checks=dict(red_flag_branch=value['route']=='urgent_evaluation',no_lifestyle_advice=value['lifestyle_advice']==[],
                    no_diagnosis=value['diagnosis'] is None,no_safety_task=not final['task'])
        evidence=dict(input_events=events,drive=result,state=final,scope='scripted medical safety routing fragment; advice quality and unscripted recognition ungraded')
        if artifact_folder:evidence['exported_artifacts']=export_artifacts(h,artifact_folder)
        return checks,evidence


def save_bytes(h,name,media,data):
    p=fixture_run(h.args('artifact','save',name,media),input=data,env=h.env,cwd=h.root,capture_output=True,timeout=20)
    require(p.returncode==0,'Artifact import failed: '+p.stderr.decode())
    return json.loads(p.stdout)


def open_bytes(h,ref):
    p=fixture_run(h.args('artifact','open',ref),env=h.env,cwd=h.root,capture_output=True,timeout=20)
    require(p.returncode==0,'Artifact readback failed')
    return p.stdout


def export_artifacts(h,folder):
    """Retain CLI-opened bytes before disposable state is cleaned up."""
    owned=False
    for parent in (folder,*folder.parents):
        require(not parent.is_symlink(),'evidence path may not traverse a symlink')
        if (parent/'.m6-evidence.json').is_file():
            owned=True
            break
    require(owned,'evidence requires an owned output root')
    folder.mkdir(parents=True,exist_ok=True)
    require(not folder.is_symlink(),'evidence directory may not be a symlink')
    exported=[]
    rows=read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT * FROM artifact ORDER BY created_at,artifact_ref')
    for index,descriptor in enumerate(rows):
        raw=open_bytes(h,descriptor['artifact_ref'])
        require(hashlib.sha256(raw).hexdigest()==descriptor['content_hash'],'exported Artifact hash differs')
        suffix=Path(descriptor['name']).suffix
        require(suffix in ('','.png','.json','.md','.csv','.mov'),'unexpected fixture Artifact type')
        target=folder/(str(index)+suffix)
        require(not target.is_symlink(),'evidence file may not be a symlink')
        target.write_bytes(raw)
        exported.append(dict(artifact_ref=descriptor['artifact_ref'],name=descriptor['name'],file=str(target),sha256=descriptor['content_hash'],bytes=len(raw)))
    return exported


def travel_control(h, receipt, phase, control_plan, applied):
    provider=h.root/'provider'
    control=None
    control_receipt=None
    held_state=None
    def advance():
        nonlocal control,control_receipt,held_state
        if control is None and (provider/'hold-accepted').exists():
            held_state=state(h)
            control_receipt=h.send(phase+'-control','Capabilities: '+json.dumps(control_plan() if callable(control_plan) else control_plan,ensure_ascii=False))
            control=h.processes.start(h.args('chat','work'),env=h.env,cwd=h.root,
                                      stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
            h.children.append(control)
        if control_receipt and main_completed(h,control_receipt['run_id']):
            require(applied(),'travel control was not committed')
            (provider/'hold-release').touch(exist_ok=True)
    try:
        scheduling=drain_with_publication_barrier(h,receipt['run_id'],prefix=phase+'-',progress=advance)
    finally:
        (provider/'hold-release').touch(exist_ok=True)
    require(control is not None,'travel Root never reached the real MCP hold')
    _,error=control.communicate(timeout=30)
    require(control.returncode==0,'travel control failed: '+error)
    require(main_completed(h,control_receipt['run_id']) and applied(),'travel control Main did not publish')
    return dict(scheduling=scheduling,held_state=held_state,control_main_completed=True)


def travel(h,world):
    # Use the maintained legal recall/publication ordering for Task creation
    # and continuation, then apply Main control while the Root is held.
    originals=[save_bytes(h,'itinerary_1003.md','text/markdown',b'Confirmed Seine cruise 2026-10-03 19:00'),save_bytes(h,'itinerary_1004.md','text/markdown',b'Confirmed Louvre 2026-10-04 09:30')]
    h.command('chat','send-parts','confirmed-inputs',text=json.dumps([dict(type='text',text='Prior confirmed itinerary sources'),*[dict(type='file',artifact_ref=d['artifact_ref']) for d in originals]]))
    france='法国旅行\nCapabilities: '+json.dumps([{'name':'builtin__text__echo','arguments':{'text':json.dumps(world['tasks'][0],ensure_ascii=False)}}],ensure_ascii=False)
    seed=h.send('france-seed','Capabilities: '+json.dumps([dict(name='builtin__task__manage',arguments=dict(operation='create',task=france,artifactRefs=[d['artifact_ref'] for d in originals]))]))
    seed_scheduling=drain_with_publication_barrier(h,seed['run_id'],prefix='seed-')
    inv=next(i for i in reversed(h.state()['tools']) if i['tool_name']=='builtin__task__manage')
    ref=json.loads(content(inv)['text'])['taskRef']
    provider=h.root/'provider';(provider/'hold-barrier').touch()
    nz_source='await tools.mcp__fixture__read({text:"world",behavior:"hold"});return "NZ context";'
    nz_task='新西兰旅行\nCapabilities: '+json.dumps([dict(name='builtin__codemode__execute',arguments=dict(source=nz_source))])
    nz_plan=json.dumps([dict(name='builtin__task__manage',arguments=dict(operation='create',task=nz_task))])
    nz_receipt=h.send('nz-seed','Capabilities: '+nz_plan)
    def nz_applied():
        nz=next(t for t in read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT * FROM task') if t['task_ref']!=ref)
        return any(r['desired_state']=='task_paused' for r in read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT desired_state FROM run WHERE task_id=?',(nz['task_id'],)))
    # Resolve NZ's Task after its actual committed create, in the progress hook.
    def nz_plan_after_hold():
        nz=next(t for t in read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT * FROM task') if t['task_ref']!=ref)
        return [dict(name='builtin__task__manage',arguments=dict(operation='pause',taskRef=nz['task_ref']))]
    nz_guard=travel_control(h,nz_receipt,'nz',nz_plan_after_hold,nz_applied)
    nz=next(t for t in read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT * FROM task') if t['task_ref']!=ref)
    h.command('chat','work')
    for name in ('hold-release','hold-accepted'):(provider/name).unlink(missing_ok=True)
    before=state(h)
    france_id=next(t['task_id'] for t in before['task'] if t['task_ref']==ref)
    old_roots={r['run_id'] for r in before['runs'] if r['task_id']==france_id}
    body='await tools.mcp__fixture__read({text:"world",behavior:"hold"});\n'+WORLD+'''const result={confirmed:world.bookings,needs_booking:["马蒂斯博物馆"],no_booking:["城堡山"],preferences:world.tasks[0].anchor.preferences,return_eta:"19:50",transit_minutes:world.transit_minutes};'''
    source=save_report(body,'itinerary_1005.json')
    task_plan=json.dumps([{'name':'builtin__codemode__execute','arguments':{'source':source}}],ensure_ascii=False)
    provider=h.root/'provider';(provider/'hold-barrier').touch()
    france_receipt=h.send('france-append','Capabilities: '+json.dumps([dict(name='builtin__task__manage',arguments=dict(operation='append',taskRef=ref,instruction='继续 10 月 5 日，不去南部乡村。\nCapabilities: '+task_plan))],ensure_ascii=False))
    steering=[dict(name='builtin__task__manage',arguments=dict(operation='append',taskRef=ref,instruction='晚上 8 点前回酒店。'))]
    def steering_applied():
        return any('晚上 8 点前' in row['text'] for row in read(Path(h.env['ERII_DATA_ROOT'])/'state.db','SELECT instruction AS text FROM task_instruction WHERE task_id=?',(france_id,)))
    france_guard=travel_control(h,france_receipt,'france',steering,steering_applied)
    held_state=france_guard['held_state']
    h.command('chat','work')
    final=state(h)
    art=next(a for a in final['artifact'] if a['name']=='itinerary_1005.json')
    output=json.loads(open_bytes(h,art['artifact_ref']))
    new_roots={r['run_id'] for r in final['runs'] if r['task_id']==france_id}
    return dict(value=output,artifact=art,scheduling=dict(seed=seed_scheduling,nz=nz_guard,france=france_guard),assertions=dict(root_dispatch_after_main_publication=all(g['worker_requests_before_main_completion']==0 for g in (seed_scheduling,nz_guard['scheduling'],france_guard['scheduling'])),two_existing_travel_tasks=len(final['task'])==2 and any(t['task_ref']==ref for t in final['task']),
                nz_stays_paused=next(t['status'] for t in final['task'] if t['task_ref']==nz['task_ref'])=='paused',
                new_root_on_continue=len(new_roots-old_roots)==1,steering_during_root=any(r['task_id']==france_id and r['status']=='running' for r in held_state['runs']),
                confirmed_unchanged=output['confirmed']==world['bookings'] and all(hashlib.sha256(open_bytes(h,d['artifact_ref'])).hexdigest()==d['content_hash'] for d in originals),
                return_before_1950=output['return_eta']<='19:50',excluded_countryside='不去南部乡村' in output['preferences'],booking_groups=bool(output['needs_booking']) and bool(output['no_booking'])),
                gap='semantic itinerary quality ungraded; concurrent Root steering also exercised by the maintained task-case gate')


def run(binary,selected=None,destination=None,scenario_jobs=1,part=None):
    require(not (selected and part),'case and part are mutually exclusive')
    dataset,policy,lock=load_contract(ROOT,'m6-v1')
    report=dict(version=1,milestone='M6',provider='explicit capabilities scripted Bridge',contract_lock=lock,
                binary_sha256=file_hash(binary),dataset_digest=digest(dataset),results=[],cleanup=False,scenario_jobs=scenario_jobs,part=part,
                driver_digest=digest({name:file_hash(ROOT/name) for name in ('scripts/run_m6_cases.py','scripts/m6_desktop.py','scripts/m6_browser.mjs','scripts/m6_sources.py','scripts/verify_recall.py','guest-browser/readiness.mjs')}),
                product_gate='inconclusive; semantic Judge and current L2 not run',
                live_shadow=dict(lume='prior evidence: docs/computer-provider-l2-result.json',jianying='prior evidence: docs/computer-l2-result.json',current_run='not_run'),
                not_applicable=[],unverified=['Remaining M7 scheduled/channel journeys, semantic quality and current L2'],
                coverage_extensions={'GC-02':'Four-turn journey and monthly Automation; mechanical assertions only'})
    artifact_root=None
    if destination:
        destination.parent.mkdir(parents=True,exist_ok=True)
        artifact_root=destination.with_name(destination.stem+'-assets')
        marker=artifact_root/'.m6-evidence.json'
        owner=dict(owner='ERII-26',version=1)
        if artifact_root.exists():
            require(not artifact_root.is_symlink() and marker.is_file() and json.loads(marker.read_text())==owner,'unowned M6 evidence directory')
        else:
            artifact_root.mkdir();marker.write_text(json.dumps(owner))
        destination.with_suffix('.progress.json').unlink(missing_ok=True)
        destination.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    completed = []
    report_lock = threading.Lock()
    items = [(case, variant) for case in dataset['cases'] if not selected or case['case_id']==selected
             for variant in variants(dataset,case) if not part or (case['case_id'],variant['variant_id']) in PARTS[part]]
    try:
        with tempfile.TemporaryDirectory(prefix='erii-m6-cases-') as temporary:
            def drive_item(item):
                case, variant = item
                folder=Path(temporary).resolve()/(case['case_id']+'-'+variant['variant_id']);folder.mkdir()
                started=time.monotonic()
                artifact_folder=artifact_root/case['case_id']/variant['variant_id'] if artifact_root else None
                if case['case_id'] in ('GC-07','GC-13','GC-18'):
                    from m6_desktop import drive
                    checks,evidence=drive(binary,folder,case,variant,artifact_folder)
                else:
                    # Each worker owns its peer, database, credentials and ports.
                    embeddings=Embeddings()
                    try:
                        checks,evidence=personal_case(binary,folder,case,variant,embeddings,artifact_folder)
                        if case['case_id']=='GC-01':
                            safety_checks,safety=health_safety(binary,folder.with_name(folder.name+'-red-flags'),case,variant,embeddings,artifact_folder/'red-flags' if artifact_folder else None)
                            checks.update(safety_checks);evidence['red_flag_variant']=safety
                    finally:
                        embeddings.close()
                grading=score(policy,checks,[])
                row=dict(case_id=case['case_id'],variant_id=variant['variant_id'],seed=case['seed'],fixture_digest=variant['fixture_digest'],
                         input_events=variant['events'],clock=case['clock'],assertions=checks,evidence=evidence,
                         status='fragment_pass' if all(checks.values()) else 'fragment_fail',hard_failure=not all(checks.values()),
                         final_score=grading['final_score'],semantic_grading=grading['semantic_grading'],dimensions={d:None for d in policy['weights']},
                         wall_time_ms=round((time.monotonic()-started)*1000))
                with report_lock:
                    completed.append(row)
                    # Avoid serializing all retained state after every variant
                    # (quadratic IO). Full evidence is written once after join,
                    # including failure; bounded progress remains observable.
                    if destination:
                        progress=[{k:r[k] for k in ('case_id','variant_id','status','wall_time_ms')} for r in completed]
                        destination.with_suffix('.progress.json').write_text(json.dumps(progress)+'\n')
                    print(case['case_id'],variant['variant_id'],row['status'],row['wall_time_ms'],flush=True)
                require(all(checks.values()),'M6 assertion failed: '+str(checks))
                return row
            def drive_variant(item):
                try:
                    return drive_item(item)
                except BaseException as error:
                    print(item[0]['case_id'],item[1]['variant_id'],'failed:',repr(error),flush=True)
                    traceback.print_exc()
                    raise
            # Each travel fixture drives Main and a held Root concurrently.
            # Keep those pairs isolated from ordinary CLI/Chromium admission,
            # using the same bounded scheduling rule as Core checkpoints.
            travel_items=[item for item in items if item[0]['case_id']=='GC-04']
            other_items=[item for item in items if item[0]['case_id']!='GC-04']
            rows=run_cases(travel_items,drive_variant,min(scenario_jobs,2))
            rows+=run_cases(other_items,drive_variant,scenario_jobs)
            order={(c['case_id'],v['variant_id']):i for i,(c,v) in enumerate(items)}
            report['results']=sorted(rows,key=lambda r:order[r['case_id'],r['variant_id']])
        require(not Path(temporary).exists(),'cleanup: case root remains')
        report['cleanup']=True
        report['summary']=dict(fragments=len(report['results']),fragment_passes=sum(r['status']=='fragment_pass' for r in report['results']),
                               scores=None,semantic_pass_at_1=None,semantic_pass_5=None)
    finally:
        # Pool cancellation joins every worker before this evidence snapshot.
        if destination:
            order={(c['case_id'],v['variant_id']):i for i,(c,v) in enumerate(items)}
            report['results']=sorted(completed,key=lambda r:order[r['case_id'],r['variant_id']])
            destination.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    return report


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--case',choices=['GC-01','GC-02','GC-03','GC-04','GC-05','GC-06','GC-07','GC-10','GC-13','GC-15','GC-18'])
    parser.add_argument('--scenario-jobs',type=int,choices=[1,2,4],default=int(os.environ.get('ERII_VERIFY_CASE_JOBS','1')))
    parser.add_argument('--part',choices=PARTS)
    args=parser.parse_args()
    if args.case and args.part:parser.error('--case and --part are mutually exclusive')
    args.evidence.unlink(missing_ok=True)
    report=run(args.binary.resolve(),args.case,args.evidence,args.scenario_jobs,args.part)
    print(json.dumps(report['summary']))


if __name__=='__main__':main()
