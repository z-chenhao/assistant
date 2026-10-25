#!/usr/bin/env python3
"""GC-05 native career journey receipts. Scripted mechanics, never model grades."""
import argparse
import copy
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
from run_m6_cases import personal, execute, save_report, remember, open_bytes
from terminal_driver import Terminal
from unscripted_cases import source_identity
from verify_gc01 import main_checks, native, kill
from verify_gc02 import recorded
from verify_gc03 import exact, recalled_context
from verify_main import require
from verify_memory_maintenance import manage, work, readback
from verify_recall import Embeddings

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / 'evals/fixtures/gc05-journey-v1.json'
REPEATS = 5
CHECKLIST = ['实际产品所有权','决策权/汇报线','现金 runway','股权条款','用户/收入或真实使用证据']


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(FIXTURE.with_suffix('.lock.json').read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes())==expected, 'GC-05 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks)==len({b['line'] for b in blocks})==19 and all(
        b['source']=='evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks),
        'GC-05 frozen block drift')
    return lock


def fixture():
    load()
    return strict_json(FIXTURE.read_bytes())


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT,'m6-v1')
    case = next(c for c in data['cases'] if c['case_id']=='GC-05')
    if variant_id is None: return data,case
    return case,next(v for v in variants(data,case) if v['variant_id']==variant_id)


def profile(current=False):
    return dict(topic='职业',kind='职业画像',profile=fixture()['current_profile'] if current else inputs()[1]['world']['profile'])


def report_source():
    # Current profile comes from the Memory Tool, not fixture constants or a
    # baked corrected Memory ID. Company facts are only the supplied synthetic
    # JD/world; no live freshness, financing or internal-company claim is made.
    return save_report('const as_of='+json.dumps(inputs()[1]['clock'])+';\n'+'''
const world=await tools.mcp__fixture__read({text:"world"});
const hits=(await tools.builtin__memory__search({query:"职业",limit:20})).filter(h=>h.source_type==="memory");
const records=hits.filter(h=>h.content.startsWith("{")).map(h=>({hit:h,value:JSON.parse(h.content)}));
const profiles=records.filter(h=>h.value.kind==="职业画像");
if(profiles.length!==1)throw Error("one current career profile required");
const current=profiles[0];
const project=records.find(h=>h.value.kind==="职业项目");
if(!project)throw Error("one supplied career project required");
const result={as_of,profile:current.value.profile,profile_ref:current.hit.source_ref,offers:world.jobs,
source:{kind:"supplied_synthetic_JD",sampled_at:as_of,live_verified:false},
unknown:["实际产品所有权","决策权/汇报线","现金 runway","股权条款","用户/收入或真实使用证据"],
risks:{cash_difference_cny:world.jobs.A.facts.cash_cny-world.jobs.B.facts.cash_cny,
early_company_risk:"unverified",equity_directly_comparable:false},
recommendation:null,resignation_decided:false,mood_is_stable_preference:false,
interview:{project_ref:project.hit.source_ref,public_summary:project.value.public_summary,verified_metrics:project.value.verified_metrics,
company_financing:null,founder_background:null,reputation:null,skill_gaps:null,preparation_outcome:null}};''','career.json')


def expected_report(current_ref,project_ref,current=False):
    case=inputs()[1]; world=case['world']; project=fixture()['project']
    return dict(as_of=case['clock'],profile=profile(current)['profile'],profile_ref=current_ref,offers=world['jobs'],
        source=dict(kind='supplied_synthetic_JD',sampled_at=case['clock'],live_verified=False),unknown=CHECKLIST,
        risks=dict(cash_difference_cny=world['jobs']['A']['facts']['cash_cny']-world['jobs']['B']['facts']['cash_cny'],
            early_company_risk='unverified',equity_directly_comparable=False),
        recommendation=None,resignation_decided=False,mood_is_stable_preference=False,
        interview=dict(project_ref=project_ref,public_summary=project['public_summary'],verified_metrics=project['verified_metrics'],
            company_financing=None,founder_background=None,reputation=None,skill_gaps=None,preparation_outcome=None))


def select(h,bridge):
    h.bridge=bridge
    h.setup(metadata=dict(name='GC-05 explicit '+bridge,provider_kind='openai_compatible',
        endpoint='http://127.0.0.1:1/v1',wire_api='openai_completions',model_id='gc05-'+bridge+'-fixture',
        context_window=65536,max_output_tokens=4096,generation_defaults={},
        compatibility=dict(tool_result_format='openai'),capabilities=dict(text_input=True,image_input=True)))


def recall(h,key):
    return execute(h,key,'return (await tools.builtin__memory__search({query:"职业",limit:20})).filter(h=>h.source_type==="memory");')


def journey(h,variant_id,row):
    case,variant=inputs(variant_id); extra=fixture()
    row['variant']=variant
    row['old_ref']=remember(h,'career-original',json.dumps(profile(),ensure_ascii=False))
    row['project_ref']=remember(h,'career-project',json.dumps(extra['project'],ensure_ascii=False))
    select(h,'terminal-input')
    h.send('context',variant['context'])
    first=variant['events'][0]
    row['first_receipt']=h.send('event-0',first['at']+' '+first['text'])
    row['input_restart']=kill(h,'after_generation_commit')
    h.command('chat','work');work(h)
    with Terminal(h.args('tui'),h.env,h.root) as terminal:
        terminal.expect('State: ready');terminal.expect('You> ')
        event=variant['events'][1]
        terminal.send(event['at']+' '+event['text'])
        terminal.expect('assistant: ');terminal.expect('You> ')
        require(terminal.stop()==0,'GC-05 TUI failed to stop')
        row['transcript']=terminal.transcript()
    event=variant['events'][2]
    row['last_receipt']=h.send('event-2',event['at']+' '+event['text'])
    h.command('chat','work');work(h)
    select(h,'capabilities')
    row['initial_report']=execute(h,'career-before-change',report_source())
    work(h);row['initial']=native(h)
    # Separate continuation probe; never substitutes for one of the three
    # original events and never turns a day's frustration into a new profile.
    select(h,'terminal-input')
    row['change_receipt']=h.send('direction-change',extra['direction_change'])
    h.command('chat','work');work(h)
    accepted=manage(h,'career-correction',operation='remember',content=json.dumps(profile(True),ensure_ascii=False),supersedes=row['old_ref'])
    row['memory_event_id']=accepted['receipt'].removeprefix('memory-event:')
    row['memory_restart']=kill(h,'after_memory_commit:'+row['memory_event_id'],('memory','work'))
    work(h)
    row['current_ref']='memory:'+next(r['memory_id'] for r in readback(h)['memory']['records'] if r['memory_id'].startswith(row['memory_event_id']))
    select(h,'capabilities')
    row['current_report']=execute(h,'career-after-change',report_source())
    work(h);row['corrected']=native(h)
    row['artifacts']=[]
    for descriptor in row['corrected']['artifact']:
        raw=open_bytes(h,descriptor['artifact_ref'])
        row['artifacts'].append(dict(descriptor=descriptor,text=raw.decode(),sha256=sha(raw),bytes=len(raw)))
    row['revoked_receipt']=h.send('dependent-on-career','请根据职业 compensation_stability 继续。')
    row['forget_restart']=kill(h,'before_generation_dispatch')
    manage(h,'forget-career-profile',operation='forget',memory_ref=row['current_ref'])
    row['fresh_receipt']=h.send('unrelated',extra['unrelated'])
    h.command('chat','work');work(h)
    row['forgotten_recall']=recall(h,'after-forget')
    work(h);row['final']=native(h)
    require(h.send('event-0',first['at']+' '+first['text'])==row['first_receipt'],'GC-05 input receipt drift')
    h.command('chat','work');work(h);row['replayed']=native(h)


def calculation_return(call):
    output=strict_json(strict_json(call["parent"]["result_json"])["text"])
    require(type(output) is list,"GC-05 native report output must be a list")
    returned=[p["value"] for p in output if p["type"]=="return"]
    require(len(returned)==1,"GC-05 requires one native report return")
    require(exact(output,call["output"]) and exact(returned[0],call["value"]),"GC-05 native report output/value drift")
    return returned[0]


def transport(call,value):
    return recorded(call['parent'],value['tools']) and all(recorded(t,value['tools']) for t in call['nested'])


def observe(row):
    # Parse the authoritative persisted Tool return before reading convenience
    # summaries. Contradictory or duplicate return values fail closed.
    initial_return=calculation_return(row['initial_report'])
    current_return=calculation_return(row['current_report'])
    case,variant=inputs(row['variant_id']); final=row['final']; corrected=row['corrected']
    old,current,project=row['old_ref'],row['current_ref'],row['project_ref']
    users=[p['text'] for m in final['conversation'] if m['role']=='user' for p in m['content']]
    originals=[variant['context']]+[e['at']+' '+e['text'] for e in variant['events']]
    positions=[users.index(s) if s in users else -1 for s in originals]
    records=corrected['memory']['memory']['records']
    original=next(r for r in records if 'memory:'+r['memory_id']==old)
    updated=next(r for r in records if 'memory:'+r['memory_id']==current)
    checks=main_checks(final)
    checks.update(exact_original_inputs=row['variant']==variant and min(positions)>=0 and positions==sorted(set(positions)) and all(users.count(s)==1 for s in originals),
        original_model_requests=all(any(s==m.get('text') for g in final['generations'] for m in json.loads(g['request_json'])['messages']) for s in originals),
        tui_input=variant['events'][1]['text'] in row['transcript'],
        separate_direction_change=users.count(fixture()['direction_change'])==1 and users.index(fixture()['direction_change'])>positions[-1],
        original_profile_retained=original['content'].startswith('sealed:v1:') and original['deleted_at'] is None,
        current_supersedes_old=updated['content'].startswith('sealed:v1:') and updated['supersedes_memory_id']==original['memory_id'] and updated['deleted_at'] is None,
        all_tools_once=all(t['status']=='succeeded' and t['attempt_count']==1 for t in final['tools']),
        input_generation_reused=row['input_restart']['returncode']==-9 and row['input_restart']['boundary']=='after_generation_commit' and
            any(g['run_id']==row['first_receipt']['run_id'] and g['status']=='completed' for g in row['input_restart']['state']['generations']) and
            all(g in final['generations'] for g in row['input_restart']['state']['generations'] if g['status']=='completed'),
        memory_crash_recovered=row['memory_restart']['returncode']==-9 and row['memory_restart']['boundary']=='after_memory_commit:'+row['memory_event_id'] and
            updated in row['memory_restart']['state']['memory']['memory']['records'] and sum(r['event_id']==row['memory_event_id'] for r in corrected['memory']['memory']['receipts'])==1,
        replay_equal=final==row['replayed'])
    for name,call,returned,value,ref,is_current in (
            ('initial',row['initial_report'],initial_return,row['initial'],old,False),
            ('current',row['current_report'],current_return,corrected,current,True)):
        nested=call['nested']; expected=expected_report(ref,project,is_current)
        hits=json.loads(json.loads(nested[1]['result_json'])['text'])
        profiles=[h for h in hits if h['source_type']=='memory' and h['content'].startswith('{') and json.loads(h['content']).get('kind')=='职业画像']
        checks[name+'_native_tools']=(transport(call,value) and len(nested)==4 and [t['tool_name'] for t in nested]==[
            'mcp__fixture__read','builtin__memory__search','builtin__artifact__manage','builtin__artifact__manage'] and
            all(t['status']=='succeeded' and t['attempt_count']==1 for t in [call['parent']]+nested) and
            json.loads(call['parent']['arguments_json'])==dict(source=report_source()) and
            json.loads(nested[0]['arguments_json'])==dict(text='world') and json.loads(json.loads(nested[0]['result_json'])['text'])==case['world'])
        checks[name+'_current_recall']=len(profiles)==1 and profiles[0]['source_ref']==ref and exact(json.loads(profiles[0]['content']),profile(is_current)) and any(
            h['source_ref']==project and exact(json.loads(h['content']),fixture()['project']) for h in hits)
        checks[name+'_report']=exact(returned['result'],expected)
        checks[name+'_artifact_transport']=exact(strict_json(nested[2]['result_json'])['artifact_refs'],returned['artifactRefs']) and exact(
            strict_json(nested[3]['arguments_json']),dict(operation='publish',artifactRefs=returned['artifactRefs']))
        replies=[strict_json(g['result_json'])['text'] for g in value['generations'] if
            g['run_id']==call['parent']['run_id'] and g['status']=='completed' and strict_json(g['result_json']).get('text')]
        checks[name+'_visible_delivery']=len(replies)==1 and replies[0].startswith('Observed Tool result: ') and exact(
            strict_json(replies[0].removeprefix('Observed Tool result: ')),call['output']) and sum(
            ''.join(p.get('text','') for p in m['content'])==replies[0] for m in value['conversation'] if m['role']=='assistant')==1
        artifacts=[a for a in row['artifacts'] if a['descriptor']['artifact_ref'] in returned['artifactRefs']]
        checks[name+'_artifact']=len(artifacts)==1 and len(returned['artifactRefs'])==1 and all(
            a['descriptor'] in value['artifact'] and a['sha256']==sha(a['text'].encode())==a['descriptor']['content_hash'] and
            a['bytes']==len(a['text'].encode())==a['descriptor']['byte_size'] and exact(strict_json(a['text']),expected) and
            any(p['artifact_ref']==a['descriptor']['artifact_ref'] for p in value['artifact_publication']) for a in artifacts)
    checks['artifact_inventory']=len(row['artifacts'])==len(corrected['artifact'])==len(final['artifact'])==2
    checks['private_notes_not_delivered']=all(fixture()['project']['private_notes'] not in a['text'] for a in row['artifacts']) and all(
        fixture()['project']['private_notes'] not in json.dumps(m,ensure_ascii=False) for m in final['conversation'] if m['role']=='assistant')
    revoked,fresh=row['revoked_receipt']['run_id'],row['fresh_receipt']['run_id']
    checks['forget_fences_old_owner']=row['forget_restart']['returncode']==-9 and row['forget_restart']['boundary']=='before_generation_dispatch' and any(
        h['source_ref']==current and exact(json.loads(h['content']),profile(True)) for h in recalled_context(row['forget_restart']['state']['generations'],revoked)) and any(
        r['run_id']==revoked and r['status']=='cancelled' and r['error_code']=='memory_forgotten' for r in final['runs']) and any(
        r['run_id']==fresh and r['status']=='completed' for r in final['runs']) and fresh!=revoked
    requests=[json.loads(g['request_json']) for g in final['generations'] if g['run_id'] in (fresh,final['runs'][-1]['run_id'])]
    forgotten=row['forgotten_recall']
    checks['forget_transport']=transport(forgotten,final) and len(forgotten['nested'])==1 and forgotten['nested'][0]['tool_name']=='builtin__memory__search' and exact(
        forgotten['value'],[h for h in json.loads(json.loads(forgotten['nested'][0]['result_json'])['text']) if h['source_type']=='memory'])
    checks['forgotten_profile_absent']=bool(requests) and all(ref not in json.dumps(req,ensure_ascii=False) for ref in (old,current) for req in requests) and all(
        h['source_ref'] not in (old,current) for h in forgotten['value']) and all(
        r['deleted_at'] is not None for r in final['memory']['memory']['records'] if 'memory:'+r['memory_id'] in (old,current))
    return checks


def rejection_probes(row):
    def native_change(r,mutate):
        call=r['current_report'];parent=call['parent'];envelope=strict_json(parent['result_json']);output=strict_json(envelope['text'])
        mutate(output[0]['value']);envelope['text']=json.dumps(output,ensure_ascii=False);result=json.dumps(envelope,ensure_ascii=False)
        def replace(value):
            if isinstance(value,dict):
                if value.get('invocation_id')==parent['invocation_id'] and 'result_json' in value:value['result_json']=result
                for child in value.values():replace(child)
            elif isinstance(value,list):
                for child in value:replace(child)
        replace(r);call['output']=copy.deepcopy(output);call['value']=copy.deepcopy(output[0]['value'])
    mutations=dict(
        changed_input=lambda r:r['variant']['events'][0].update(text='replaced'),
        missing_original=lambda r:r['final']['conversation'].pop(0),
        stale_profile=lambda r:native_change(r,lambda v:v['result'].update(profile=profile()['profile'])),
        invented_financing=lambda r:native_change(r,lambda v:v['result']['interview'].update(company_financing='confirmed')),
        private_leak=lambda r:native_change(r,lambda v:v['result']['interview'].update(public_summary=fixture()['project']['private_notes'])),
        fabricated_metric=lambda r:native_change(r,lambda v:v['result']['interview'].update(verified_metrics=['revenue +100%'])),
        immediate_resignation=lambda r:native_change(r,lambda v:v['result'].update(resignation_decided=True)),
        missing_check=lambda r:native_change(r,lambda v:v['result']['unknown'].pop()),
        false_live_source=lambda r:native_change(r,lambda v:v['result']['source'].update(live_verified=True)),
        equity_comparable=lambda r:native_change(r,lambda v:v['result']['risks'].update(equity_directly_comparable=True)),
        native_artifact_ref=lambda r:native_change(r,lambda v:v.update(artifactRefs=['artifact:forged'])),
        cached_return=lambda r:r['current_report']['value']['result'].update(resignation_decided=True),
        corrupt_artifact=lambda r:r['artifacts'][0].update(sha256='0'*64),
        broken_lineage=lambda r:next(m for m in r['corrected']['memory']['memory']['records'] if 'memory:'+m['memory_id']==r['current_ref']).update(supersedes_memory_id=None),
        false_restart=lambda r:r['memory_restart'].update(returncode=0),
        replay_drift=lambda r:r['replayed']['deliveries'].append({}),
        duplicate_attempt=lambda r:r['final']['tools'][0].update(attempt_count=2),
        external_effect=lambda r:r['final']['tools'].append(dict(tool_name='send_cv',run_id=r['final']['runs'][0]['run_id'])),
        revived_owner=lambda r:next(n for n in r['final']['runs'] if n['run_id']==r['revoked_receipt']['run_id']).update(status='completed'),
        deleted_recall=lambda r:r['forgotten_recall']['value'].append(dict(source_ref=r['current_ref'])))
    require(all(observe(row).values()),'GC-05 corruption probes require a passing native baseline')
    result={}
    for name,mutate in mutations.items():
        changed=copy.deepcopy(row)
        try:mutate(changed);result[name]=not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):result[name]=True
        require(result[name],'GC-05 consumer accepted corruption: '+name)
    return result


def drive(binary, item, failure_evidence):
    variant_id, repetition = item
    row = dict(variant_id=variant_id,repetition=repetition)
    embeddings = Embeddings()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc05-') as temp:
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


def validate_report(report, binary=ROOT / 'bin/erii'):
    lock = load()
    expected = [(v['variant_id'], n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require(report.get('case_id') == 'GC-05' and report.get('status') == 'mechanical_pass' and
        report.get('release_ready') is False and 'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        report.get('fixture_lock') == lock and report.get('m6_lock') == load_contract(ROOT,'m6-v1')[2] and report.get('cleanup') is True,
        'complete locked ungraded GC-05 receipts required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']), 'GC-05 source/binary identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    for row in rows:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-05 native receipt')
    require(report.get('rejection_probes') == rejection_probes(rows[0]), 'missing/forged GC-05 corruption probes')
    return dict(case_id='GC-05',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=19)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc05-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-05',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=19,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc05_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc05_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-05 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-05 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-05',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=19,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-05 manifest/native summary drift')
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
    native_folder = args.evidence.parent / '.gc05-native'
    native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-05',status='inconclusive',release_ready=False,semantic_score=None,eligible_for_grading=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=dict(
            platform=platform.platform(),python=platform.python_version(),node=subprocess.check_output(['node','--version'],text=True).strip(),
            go=subprocess.check_output(['go','version'],text=True).strip(),binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-05; original input fragments only; historical recall/opportunity-source adapter incomplete',
            child_approval_unknown='synchronous Main only; no Task, Child, career external effect or approval; unknown effects are inapplicable'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder / (args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex: report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-05 native assertion failure')
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
