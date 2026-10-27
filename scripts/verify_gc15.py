#!/usr/bin/env python3
"""GC-15 original multimodal journey; recomputable mechanics, no OCR grade."""
import argparse
import base64
import copy
from datetime import date
import hashlib
import json
from pathlib import Path
import platform
import struct
import subprocess
import tempfile
import threading

from case_runner import load_contract, variants
from fixture_pool import run_cases
from m6_sources import validate as validate_source
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_m6_cases import personal, save_bytes, open_bytes
from unscripted_cases import source_identity
from verify_capabilities import call
from verify_code_recovery import read
from verify_gc02 import native
from verify_gc03 import exact
from verify_gc04 import plan
from verify_main import require
from verify_memory_maintenance import work
from verify_recall import Embeddings

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / 'evals/fixtures/m6-v1'
LOCK = ROOT / 'evals/fixtures/gc15-v1.lock.json'
REPEATS = 5


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT / name).read_bytes()) == expected, 'GC-15 lock drift: ' + name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT / 'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 16 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current for b in blocks),
        'GC-15 frozen block drift')
    return lock


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT, 'm6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-15')
    if variant_id is None:
        return data, case
    return case, next(v for v in variants(data, case) if v['variant_id'] == variant_id)


def select(h, bridge):
    h.bridge = bridge
    h.setup(metadata=dict(name='GC-15 explicit '+bridge,provider_kind='openai_compatible',
        endpoint='http://127.0.0.1:1/v1',wire_api='openai_completions',model_id='gc15-'+bridge+'-fixture',
        context_window=65536,max_output_tokens=4096,generation_defaults={},compatibility=dict(tool_result_format='openai'),
        capabilities=dict(text_input=True,image_input=True)))


def kill(h, boundary):
    child=h.hold(boundary)
    before=native(h)
    child.kill(); _,stderr=child.communicate(timeout=10)
    return dict(boundary=boundary,returncode=child.returncode,state=before,
        stderr=h.hold_observation['stderr']+stderr)


def report_source(documents):
    # Extraction is explicitly supplied by the locked region Fake. The real
    # Host selects and decodes image pixels; this never claims OCR inference.
    return 'const documents=' + json.dumps(documents, ensure_ascii=False) + ';\n' + '''
for(const document of documents)image({artifactRef:document.artifactRef});
const field=(name,key)=>documents.find(d=>d.name===name).fields[key];
const value=(name,key)=>field(name,key).visible?field(name,key).value:null;
const vf=key=>value("visa_front.png",key),of=key=>value("flight_order.png",key);
const entry=of("outbound"),exit=entry.slice(0,4)+"-"+of("inbound_day_month");
const days=Number(exit.slice(-2))-Number(entry.slice(-2))+1;
const result={facts:{valid_from:vf("valid_from"),valid_until:vf("valid_until"),
 duration_days:vf("duration_days"),entries:vf("entries"),outbound:entry,inbound_day_month:of("inbound_day_month")},
 calculation:{entry,exit,inclusive_days:days,window_covers:vf("valid_from")<=entry&&exit<=vf("valid_until"),
 within_duration:days<=vf("duration_days"),entry_count:1},
 unknown:{return_year:of("inbound_year"),order_matches_2026:null,
 clarification:"provide a clear screenshot or other order evidence"},sources:documents};
const report="# Visa check\\n\\n## Visible facts\\n"+JSON.stringify(result.facts)+
 "\\n\\n## Calculation (both entry and exit days count)\\n"+JSON.stringify(result.calculation)+
 "\\n\\n## Unknown / clarification required\\n"+JSON.stringify(result.unknown)+
 "\\n\\n## Source regions (pixels, top-left)\\n"+JSON.stringify(result.sources)+"\\n";
const saved=await tools.builtin__artifact__manage({operation:"save",name:"visa_check.md",mediaType:"text/markdown",text:report});
await tools.builtin__artifact__manage({operation:"publish",artifactRefs:saved.artifactRefs});
return {result,report,artifactRefs:saved.artifactRefs};'''


def original_parts(variant, documents):
    return [dict(type='text',text=variant['events'][0]['at']+' '+variant['events'][0]['text']),
        *[dict(type='image',artifact_ref=d['artifactRef']) for d in documents]]


def journey(h, variant_id, row):
    _, variant = inputs(variant_id)
    regions = strict_json((ASSETS / 'regions.json').read_bytes())
    row['variant'] = variant
    row['command_logs'] = []
    command = h.command
    def observed(*args,text=None,ok=True,timeout=320):
        process=command(*args,text=text,ok=False,timeout=timeout)
        row['command_logs'].append(dict(operation=list(args[:2]),returncode=process.returncode,stderr=process.stderr))
        require(not ok or process.returncode==0,repr(args[:2])+': '+process.stderr)
        return strict_json(process.stdout) if ok else process
    h.command=observed
    row['documents'] = []
    for name, document in regions.items():
        raw = (ASSETS / name).read_bytes()
        descriptor = save_bytes(h,name,'image/png',raw)
        row['documents'].append(dict(name=name,**validate_source(document,descriptor,list(struct.unpack('>II',raw[16:24])))))
    # The local user may import bytes; a model with no explicit grant may not
    # describe them. A separate pre-input Run proves the real scope rejection.
    row['scope_denial'] = call(h,'ungranted-source','builtin__artifact__manage',
        dict(operation='describe',artifactRef=row['documents'][0]['artifactRef']))
    select(h,'terminal-input')
    h.send('context',variant['context']); work(h)
    parts = original_parts(variant,row['documents'])
    row['input_receipt'] = h.command('chat','send-parts','original',text=json.dumps(parts,ensure_ascii=False))
    row['input_restart'] = kill(h,'after_generation_commit')
    h.command('chat','work'); work(h)
    select(h,'capabilities')
    row['plan'] = [plan('builtin__codemode__execute',dict(source=report_source(row['documents'])))]
    h.send('explicit-driver','Capabilities: '+json.dumps(row['plan'],ensure_ascii=False))
    # Metadata hook is inside an open transaction. Do not launch another App
    # (and its migrations) while it is held; SIGKILL first, inspect raw SQLite.
    child = h.hold('artifact_after_metadata')
    child.kill(); _,stderr=child.communicate(timeout=10)
    row['artifact_rollback'] = dict(boundary='artifact_after_metadata',returncode=child.returncode,
        artifacts=read(h.root/'data/state.db','SELECT * FROM artifact'),stderr=h.hold_observation['stderr']+stderr)
    row['artifact_restart'] = kill(h,'after_tool_commit')
    h.command('chat','work'); work(h)
    row['artifacts'] = []
    for d in native(h)['artifact']:
        raw = open_bytes(h,d['artifact_ref'])
        row['artifacts'].append(dict(descriptor=d,data=base64.b64encode(raw).decode(),sha256=sha(raw),bytes=len(raw)))
    # Repeated and drifted ingress are exercised at the actual CLI boundary.
    row['final'] = native(h)
    row['objects'] = sorted(p.name for p in (h.root/'data/artifacts').iterdir())
    row['duplicate_receipt'] = h.command('chat','send-parts','original',text=json.dumps(parts,ensure_ascii=False))
    changed = copy.deepcopy(parts); changed[0]['text'] += ' changed'
    rejected = h.command('chat','send-parts','original',text=json.dumps(changed,ensure_ascii=False),ok=False)
    row['drift_rejection'] = dict(returncode=rejected.returncode,stderr=rejected.stderr)
    h.command('chat','work'); work(h)
    row['replayed'] = native(h)


def returned(value):
    parents = [t for t in value['tools'] if t['tool_name']=='builtin__codemode__execute']
    require(len(parents)==1,'one native GC-15 calculation required')
    outputs = strict_json(strict_json(parents[0]['result_json'])['text'])
    values = [o['value'] for o in outputs if o['type']=='return']
    require(len(values)==1,'one GC-15 native return required')
    return parents[0],values[0]


def document_checks(document, documents):
    world = inputs()[1]['world']; visa = world['visa_front']; order = world['flight_order']
    entry = date.fromisoformat(order['outbound'])
    exit = date.fromisoformat(str(entry.year)+'-'+order['inbound_day_month'])
    return dict(visible_facts=exact(document['facts'],dict(**visa,outbound=order['outbound'],inbound_day_month=order['inbound_day_month'])),
        inclusive_window_and_duration=exact(document['calculation'],dict(entry=entry.isoformat(),exit=exit.isoformat(),
            inclusive_days=(exit-entry).days+1,window_covers=date.fromisoformat(visa['valid_from'])<=entry<=exit<=date.fromisoformat(visa['valid_until']),
            within_duration=(exit-entry).days+1<=visa['duration_days'],entry_count=1)),
        unknown_year_and_clarification=exact(document['unknown'],dict(return_year=None,order_matches_2026=None,
            clarification='provide a clear screenshot or other order evidence')),
        precise_source_lineage=exact(document['sources'],documents))


def observe(row):
    _, variant = inputs(row['variant_id'])
    final = row['final']; documents = row['documents']
    parent, output = returned(final); result = output['result']
    checks = document_checks(result,documents)
    regions = strict_json((ASSETS/'regions.json').read_bytes())
    refs = [d['artifactRef'] for d in documents]
    artifacts = {a['descriptor']['artifact_ref']:a for a in row['artifacts']}
    report = next(a for a in final['artifact'] if a['name']=='visa_check.md')
    raw_report = base64.b64decode(artifacts[report['artifact_ref']]['data'],validate=True)
    expected_text = '# Visa check\n\n## Visible facts\n'+json.dumps(result['facts'],ensure_ascii=False,separators=(',',':'))
    for label,key in (('Calculation (both entry and exit days count)','calculation'),
                      ('Unknown / clarification required','unknown'),('Source regions (pixels, top-left)','sources')):
        expected_text += '\n\n## '+label+'\n'+json.dumps(result[key],ensure_ascii=False,separators=(',',':'))
    expected_text += '\n'
    saves = [t for t in final['tools'] if t['tool_name']=='builtin__artifact__manage' and strict_json(t['arguments_json']).get('operation')=='save']
    users = [p for m in final['conversation'] if m['role']=='user' for p in m['content']]
    original = original_parts(variant,documents)[0]['text']
    frozen_original = original + ''.join('\nArtifact reference (content not loaded): '+
        json.dumps(p,separators=(',',':')) for p in original_parts(variant,documents)[1:])
    requests = [strict_json(g['request_json']) for g in final['generations']]
    selected = {ref for req in requests for m in req['messages'] for ref in m.get('image_refs',[])}
    denied = next(t for t in final['tools'] if t['invocation_id']==row['scope_denial']['invocation_id'])
    logs='\n'.join([r['stderr'] for r in row['command_logs']]+[row[k]['stderr'] for k in ('input_restart','artifact_rollback','artifact_restart')])
    forbidden=['valid_from','valid_until','duration_days','visa_front.png','flight_order.png','2026-09-20','2026-10-20']
    forbidden += [base64.b64encode((ASSETS/name).read_bytes()).decode()[:48] for name in regions]
    allowed = {'builtin__codemode__execute','builtin__artifact__manage'}
    checks.update(
        locked_source_regions=len(documents)==2 and set(d['name'] for d in documents)==set(regions) and all(
            exact({k:v for k,v in d.items() if k not in ('name','artifactRef')},regions[d['name']]) for d in documents),
        source_bytes_immutable=len(artifacts)==len(final['artifact'])==3 and all(
            exact(a['descriptor'],d) and sha(base64.b64decode(a['data'],validate=True))==a['sha256']==d['content_hash'] and
            a['bytes']==d['byte_size']==len(base64.b64decode(a['data'],validate=True)) for d in final['artifact'] for a in [artifacts[d['artifact_ref']]]) and
            all(base64.b64decode(artifacts[d['artifactRef']]['data'],validate=True)==(ASSETS/d['name']).read_bytes() for d in documents),
        original_inputs=exact(row['variant'],variant) and users.count(dict(type='text',text=original))==1 and
            users.count(dict(type='text',text=variant['context']))==1 and all(dict(type='image',artifact_ref=ref) in users for ref in refs),
        original_frozen_request=any(frozen_original==m.get('text') for req in requests for m in req['messages']),
        explicit_pixel_selection=selected==set(refs) and all(req['model_snapshot']['capabilities']['image_input'] is True for req in requests),
        source_scope_rejection=exact(denied,row['scope_denial']) and denied['status']=='failed' and
            strict_json(denied['arguments_json'])==dict(operation='describe',artifactRef=refs[0]) and 'NOT_FOUND' in (denied['result_json'] or ''),
        real_native_calculation=strict_json(parent['arguments_json'])==row['plan'][0]['arguments'] and
            exact(row['plan'],[plan('builtin__codemode__execute',dict(source=report_source(documents)))]) and parent['status']=='succeeded',
        report_save_return_readback=len(saves)==1 and strict_json(saves[0]['arguments_json'])==dict(operation='save',name='visa_check.md',mediaType='text/markdown',text=expected_text) and
            output['report']==expected_text and raw_report==expected_text.encode() and output['artifactRefs']==[report['artifact_ref']],
        selected_publication=bool(final['artifact_publication']) and all(p['artifact_ref']==report['artifact_ref'] for p in final['artifact_publication']) and
            sum(p.get('artifact_ref')==report['artifact_ref'] for m in final['conversation'] if m['role']=='assistant' for p in m['content'])==1,
        run_source_grants=all(any(g['artifact_ref']==ref and g['holder_kind']=='run' and g['holder_id']==row['input_receipt']['run_id'] for g in final['artifact_reference']) for ref in refs),
        no_unrelated_memory=not final['memory']['memory']['records'] and not final['memory']['memory']['evidence'] and
            not final['memory']['state']['candidates'] and all(e['operation']=='observe' and e['payload']=='' for e in final['memory']['state']['events']),
        no_sensitive_operational_log=bool(row['command_logs']) and all(text not in logs for text in forbidden),
        frozen_model_and_capabilities=all(g['status']=='completed' and any(r['run_id']==g['run_id'] and
            r['model_snapshot_id']==strict_json(g['request_json'])['model_snapshot']['snapshot_id'] for r in final['runs']) for g in final['generations']) and
            all(r['capability_snapshot_id'] in {c['snapshot_id'] for c in final['capabilities']} for r in final['runs']),
        main_only_no_effects=len(final['runs'])==3 and len(final['tools'])==4 and len(final['generations'])==5 and
            not final['task'] and not final['tasks'] and not final['approval'] and not final['automation'] and
            all(r['kind']=='main' and r['task_id'] is None and r['parent_run_id'] is None and r['status']=='completed' for r in final['runs']) and
            all(t['tool_name'] in allowed and (t['invocation_id']==denied['invocation_id'] or t['status']=='succeeded') for t in final['tools']),
        # Two process deaths resume the incomplete safe script; the rolled-back
        # local save resumes once. Completed save/publication never dispatch again.
        exact_recovery_attempts=all(t['attempt_count']==(3 if t['invocation_id']==parent['invocation_id'] else
            2 if t['invocation_id']==saves[0]['invocation_id'] else 1) for t in final['tools']) and
            all(g['attempt_count']==1 for g in final['generations']),
        three_real_kills=row['input_restart']['returncode']==row['artifact_restart']['returncode']==row['artifact_rollback']['returncode']==-9 and
            row['input_restart']['boundary']=='after_generation_commit' and row['artifact_restart']['boundary']=='after_tool_commit' and
            row['artifact_rollback']['boundary']=='artifact_after_metadata' and len(row['artifact_rollback']['artifacts'])==2 and
            len(row['artifact_restart']['state']['artifact'])==3,
        committed_calls_reused=all(any(g['generation_id']==before['generation_id'] and g['result_json']==before['result_json'] and
            g['request_json']==before['request_json'] and g['attempt_count']==before['attempt_count']==1 for g in final['generations'])
            for crash in ('input_restart','artifact_restart') for before in row[crash]['state']['generations'] if before['status']=='completed') and
            all(any(t['invocation_id']==before['invocation_id'] and t['result_json']==before['result_json'] and
                t['arguments_json']==before['arguments_json'] and t['attempt_count']==before['attempt_count'] for t in final['tools'])
                for before in row['artifact_restart']['state']['tools'] if before['status']=='succeeded'),
        committed_artifact_reused=any(a['artifact_ref']==report['artifact_ref'] and a['content_hash']==report['content_hash'] for a in row['artifact_restart']['state']['artifact']),
        no_orphan_objects=row['objects']==sorted(a['storage_key'] for a in final['artifact']),
        durable_calls=len(final['code_execution_state'])==1 and all(c['state']=='completed' and c['desired_state']=='running' and
            c['execution_epoch']==3 and c['replay_starts']>=2 and c['replay_hits']>=1 for c in final['code_execution_state']) and len(final['code_call'])==2 and
            len({c['call_key'] for c in final['code_call']})==2,
        duplicate_and_drift=exact(row['input_receipt'],row['duplicate_receipt']) and row['drift_rejection']['returncode']!=0 and
            'CONFLICT' in row['drift_rejection']['stderr'].upper(),
        exact_replay=exact(final,row['replayed']))
    return checks


def rejection_probes(row):
    def result_change(r,key,value):
        parent,output=returned(r['final']); output['result'][key]=value
        records=strict_json(strict_json(parent['result_json'])['text'])
        next(p for p in records if p['type']=='return')['value']=output
        parent['result_json']=json.dumps(dict(text=json.dumps(records)))
    def synced(r,mutate):
        parent,output=returned(r['final']); mutate(output['result'])
        text='# Visa check\n\n## Visible facts\n'+json.dumps(output['result']['facts'],ensure_ascii=False,separators=(',',':'))
        for label,key in (('Calculation (both entry and exit days count)','calculation'),
                          ('Unknown / clarification required','unknown'),('Source regions (pixels, top-left)','sources')):
            text+='\n\n## '+label+'\n'+json.dumps(output['result'][key],ensure_ascii=False,separators=(',',':'))
        text+='\n'; output['report']=text
        records=strict_json(strict_json(parent['result_json'])['text'])
        next(p for p in records if p['type']=='return')['value']=output
        parent['result_json']=json.dumps(dict(text=json.dumps(records)))
        for tool in r['final']['tools']:
            arguments=strict_json(tool['arguments_json'])
            if arguments.get('operation')=='save':
                arguments['text']=text;tool['arguments_json']=json.dumps(arguments)
        art=next(a for a in r['final']['artifact'] if a['name']=='visa_check.md')
        art.update(content_hash=sha(text.encode()),byte_size=len(text.encode()))
        opened=next(a for a in r['artifacts'] if a['descriptor']['artifact_ref']==art['artifact_ref'])
        opened.update(data=base64.b64encode(text.encode()).decode(),sha256=art['content_hash'],bytes=len(text.encode()),descriptor=copy.deepcopy(art))
        r['replayed']=copy.deepcopy(r['final'])
    mutations=dict(
        original=lambda r:r['variant']['events'][0].update(text='changed'),
        region=lambda r:r['documents'][0]['fields']['valid_from'].update(region=[0,0,1,1]),
        masked_year=lambda r:result_change(r,'unknown',dict(return_year=2026,order_matches_2026=True,clarification='')),
        confused_duration=lambda r:result_change(r,'facts',dict(valid_until='15 days')),
        forged_calculation=lambda r:result_change(r,'calculation',dict(inclusive_days=8)),
        synced_wrong_stay=lambda r:synced(r,lambda d:d['calculation'].update(inclusive_days=8)),
        synced_guessed_year=lambda r:synced(r,lambda d:d['unknown'].update(return_year=2026,order_matches_2026=True)),
        synced_multiple_entries=lambda r:synced(r,lambda d:d['facts'].update(entries='MULT')),
        synced_validity_duration=lambda r:synced(r,lambda d:d['facts'].update(duration_days=30)),
        altered_original=lambda r:r['artifacts'][0].update(data=base64.b64encode(b'changed').decode()),
        orphan_object=lambda r:r['objects'].append('orphan'),
        empty_publication=lambda r:r['final']['artifact_publication'].clear(),
        missing_source_grants=lambda r:r['final']['artifact_reference'].clear(),
        missing_delivery=lambda r:r['final']['conversation'].pop(),
        scope_success=lambda r:r['scope_denial'].update(status='succeeded'),
        secret_memory=lambda r:r['final']['memory']['memory']['records'].append(dict(content='visa data')),
        sensitive_log=lambda r:r['command_logs'][0].update(stderr='valid_until=2026-10-20'),
        unknown_effect=lambda r:r['final']['tools'][-1].update(status='unknown'),
        duplicate_attempt=lambda r:r['final']['tools'][-1].update(attempt_count=2),
        fake_kill=lambda r:r['artifact_restart'].update(returncode=0),
        lost_rollback=lambda r:r['artifact_rollback']['artifacts'].append({}),
        lost_committed_artifact=lambda r:r['artifact_restart']['state']['artifact'].clear(),
        drift_accepted=lambda r:r['drift_rejection'].update(returncode=0),
        replay=lambda r:r['replayed']['tools'].clear())
    results={}
    for name,mutate in mutations.items():
        changed=copy.deepcopy(row)
        try:
            mutate(changed);results[name]=not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
            results[name]=True
        require(results[name],'GC-15 accepted corrupt native receipt: '+name)
    return results


def execution_ids(row):
    value=row['final']
    pairs=(('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),('deliveries','delivery_id'))
    result=set()
    for table,key in pairs:
        ids=[r[key] for r in value[table]]
        require(ids and len(ids)==len(set(ids)) and all(isinstance(i,str) and i for i in ids),'GC-15 native IDs missing/duplicated')
        result.update((table,i) for i in ids)
    return result


def drive(binary,item,failure_evidence):
    variant_id,repetition=item
    row=dict(variant_id=variant_id,repetition=repetition)
    embeddings=Embeddings()
    closed=False
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc15-') as temp:
            root=Path(temp)
            with personal(binary,root,inputs(variant_id)[0],embeddings) as h:
                try:
                    journey(h,variant_id,row)
                except BaseException as error:
                    row['failure']=dict(kind=type(error).__name__,message=str(error))
                    row['failure_state']=h.failure_state()
                    raise
        embeddings.close()
        closed=True
        row['cleanup']=dict(disposable_root_removed=not root.exists(),owned_processes_joined=True,embedding_peer_joined=True)
        row['assertions']=observe(row)
        require(all(row['assertions'].values()),'GC-15 native assertion failure: '+str([k for k,v in row['assertions'].items() if not v]))
    except BaseException:
        failure_evidence.write_text(json.dumps(row,ensure_ascii=False)+'\n')
        raise
    finally:
        if not closed: embeddings.close()
    return row


def validate_report(report, binary=ROOT / 'bin/erii'):
    lock = load()
    expected = [(v['variant_id'], n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require(report.get('case_id') == 'GC-15' and report.get('status') == 'mechanical_pass' and
        report.get('release_ready') is False and 'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        report.get('fixture_lock') == lock and report.get('m6_lock') == load_contract(ROOT,'m6-v1')[2] and report.get('cleanup') is True,
        'complete locked ungraded GC-15 receipts required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']), 'GC-15 source/binary identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    used=set()
    for row in rows:
        identifiers=execution_ids(row)
        require(not used.intersection(identifiers),'GC-15 copied/relabelled native execution')
        used.update(identifiers)
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True,embedding_peer_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-15 native receipt')
    require(report.get('rejection_probes') == rejection_probes(rows[0]), 'missing/forged GC-15 corruption probes')
    return dict(case_id='GC-15',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=16)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc15-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-15',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=16,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc15_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc15_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-15 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-15 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-15',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=16,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-15 manifest/native summary drift')
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
    native_folder = args.evidence.parent / '.gc15-native'
    native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-15',status='inconclusive',release_ready=False,semantic_score=None,eligible_for_grading=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=dict(
            platform=platform.platform(),python=platform.python_version(),node=subprocess.check_output(['node','--version'],text=True).strip(),
            go=subprocess.check_output(['go','version'],text=True).strip(),binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-15; original text fragment only; native image ingress/OCR adapter incomplete',
            child_approval_unknown='Synchronous Main; no Task/Child, external effect or approval requested. Their cancellation/epoch/unknown conformance gates remain required and are not this Case execution evidence.'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder / (args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex: report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-15 native assertion failure')
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
