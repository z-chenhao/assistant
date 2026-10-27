"""GC-17 native oracle shared by the existing Core CLI, CI and Judge.

This is a scripted mechanical journey. It never assigns a semantic score.
"""
import base64
from collections import Counter
import copy
import csv
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import sqlite3
import subprocess
import sys
import tempfile

from case_runner import load_contract, variants, digest
from core_fixtures import feedback, csv_text, CATEGORIES, SEVERITIES, PAIRS
from release_audit import inventory
from release_receipts import strict_json
from verify_gc03 import exact
from verify_main import require

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT/'evals/fixtures/gc17-boundary-v1.lock.json'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def assets():
    rows, _ = feedback()
    result = [dict(name='feedback_100.csv',mediaType='text/csv',text=csv_text(rows))]
    result.extend(dict(name=r['attachment_ref'],mediaType='text/plain',
        text=(ROOT/'evals/fixtures/core-v1'/r['attachment_ref']).read_text()) for r in rows if r['attachment_ref'])
    return result


def prompt(variant, inputs=None):
    return variant['context']+'\n'+variant['events'][0]['text']+'\nFeedback assets:\n'+json.dumps(
        assets() if inputs is None else inputs,ensure_ascii=False)


def source_for(inputs):
    return (ROOT/'internal/llm/fake/feedback.js').read_text().replace('ASSETS',json.dumps(inputs,ensure_ascii=False),1)


def load():
    lock = strict_json(LOCK.read_bytes())
    require(lock['source_manifest_sha256']=='0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556',
            'GC-17 frozen manifest identity drift')
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected,'GC-17 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 21 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks),
        'GC-17 frozen block drift')
    return lock


def native(h):
    from run_core_cases import snapshot
    from verify_children import state
    value = snapshot(h)
    value.update(state(h))
    with sqlite3.connect(Path(h.env['ERII_DATA_ROOT'])/'state.db') as db:
        db.row_factory = sqlite3.Row
        value['tools'] = [dict(r) for r in db.execute('SELECT * FROM tool_invocation ORDER BY created_at,delivery_seq')]
        value['automation'] = [dict(r) for r in db.execute('SELECT * FROM automation')]
    return value


def bindings(row):
    """Join the actual accepted Child/CodeCall/result/Artifact/Delivery records."""
    value = row['state']; tools = {t['invocation_id']:t for t in value['tools']}
    generations = {g['generation_id']:g for g in value['generations']}
    nodes = {r['run_id']:r for r in value['runs']}
    links = value['child_links']; originals = {r['row_id']:r for r in feedback()[0]}
    gold = {r['row_id']:r for r in feedback()[1]}
    codes = {c['invocation_id']:c for c in value['code_call']}
    root = next(r for r in nodes.values() if r['kind']=='worker' and not r['parent_run_id'])
    output = row['artifact']; parsed = list(csv.DictReader(io.StringIO(output['text'])))
    adopted = {r['row_id']:r for r in parsed}
    seen = set()
    execution = value['code_execution'][0]['execution_id']
    wrapper = next(t for t in tools.values() if t['tool_name']=='builtin__codemode__execute')
    manifest=value['code_manifest'][0]
    request=strict_json(manifest['request_json'])
    capability=next(c for c in value['capabilities'] if c['snapshot_id']==root['capability_snapshot_id'])
    if (manifest['execution_id']!=execution or execution!=wrapper['invocation_id'] or
            manifest['manifest_hash']!=sha(manifest['request_json'].encode()) or
            manifest['input_hash']!=sha(manifest['input_json'].encode()) or
            manifest['snapshot_hash']!=sha((root['capability_snapshot_id']+':'+root['model_snapshot_id']+':'+capability['tools_json']).encode()) or
            request['source']!=row['source'] or request['sourceHash']!=sha(row['source'].encode()) or
            request['executionId']!=execution or request['parentInvocationId']!=execution or request['runId']!=root['run_id'] or
            not exact(request['tools'],strict_json(capability['tools_json']))):
        return False
    if (len(codes)!=108 or any(c['execution_id']!=execution or c['call_id']!=execution+'/'+c['call_key'] or
            tools[c['invocation_id']]['parent_invocation_id']!=wrapper['invocation_id'] or
            c['request_hash']!=tools[c['invocation_id']]['request_hash'] for c in codes.values())):
        return False
    for link in links:
        tool = tools[link['invocation_id']]; child = nodes[link['child_run_id']]
        args = strict_json(tool['arguments_json'])
        task = strict_json(args['task'].removeprefix('Classify feedback: '))
        key = task['row_id']; number = int(key[1:])
        if (key in seen or set(args) != {'task','artifactRefs'} or
                not exact(task,dict(**originals[key],hold=38 <= number <= 45)) or
                args['artifactRefs'] != [a['artifact_ref'] for a in value['artifact'] if a['name'] != 'feedback_analysis.csv'] or
                tool['tool_name'] != 'builtin__agent__run' or tool['run_id'] != root['run_id'] or
                child['parent_run_id'] != root['run_id'] or child['task_epoch'] != root['task_epoch'] or
                codes[tool['invocation_id']]['request_hash'] != tool['request_hash'] or
                not re.fullmatch('[0-9a-f]{64}',codes[tool['invocation_id']]['call_key'])):
            return False
        seen.add(key)
        results = [w for w in value['worker_results'] if w['run_id'] == child['run_id']]
        if len(results) != 1:
            return False
        result = results[0]; generation = generations[result['exchange_id']]
        answer = dict(**gold[key],evidence=originals[key]['text'],attachment_ref=originals[key]['attachment_ref'])
        if (generation['run_id'] != child['run_id'] or strict_json(generation['result_json'])['text'] != result['content'] or
                not exact(strict_json(result['content']),answer) or
                not exact(strict_json(strict_json(tool['result_json'])['text']),dict(text=result['content'],artifactRefs=[])) or
                adopted[key] != {**answer,'evidence':key}):
            return False
    if seen != set(originals):
        return False
    # Direct calls join the model exchange. Nested calls join the fixed Code
    # execution and manifest instead; they are not model-emitted Tool calls.
    for tool in tools.values():
        if tool['parent_invocation_id']:
            if tool['invocation_id'] not in codes:
                return False
            continue
        generation = generations[tool['provider_exchange_id']]
        emitted = strict_json(generation['result_json'])['tool_calls']
        matches = [c for c in emitted if c['id'] == tool['provider_call_id']]
        if (generation['run_id'] != tool['run_id'] or len(matches) != 1 or matches[0]['name'] != tool['tool_name'] or
                not exact(strict_json(matches[0]['arguments']),strict_json(tool['arguments_json']))):
            return False
    saves = [t for t in tools.values() if t['tool_name']=='builtin__artifact__manage' and
             strict_json(t['arguments_json'])['operation']=='save']
    if len(saves) != 7:
        return False
    for tool in saves:
        args = strict_json(tool['arguments_json'])
        expected = next((a for a in row['inputs'] if a['name']==args['name']),
                        dict(name='feedback_analysis.csv',mediaType='text/csv',text=output['text']))
        stored = [a for a in value['artifact'] if a['name']==args['name']]
        if (len(stored) != 1 or not exact(args,dict(operation='save',**expected))):
            return False
        stored = stored[0]
        if (stored['run_id'] != root['run_id'] or stored['put_key'] != 'tool:'+tool['invocation_id'] or
                stored['content_hash'] != sha(expected['text'].encode()) or stored['byte_size'] != len(expected['text'].encode()) or
                stored['media_type'] != expected['mediaType'] or
                strict_json(tool['result_json'])['artifact_refs'] != [stored['artifact_ref']]):
            return False
    stored = next(a for a in value['artifact'] if a['name']=='feedback_analysis.csv')
    if stored['artifact_ref'] != output['ref'] or stored['content_hash'] != output['sha256']:
        return False
    publishes = [t for t in tools.values() if t['tool_name']=='builtin__artifact__manage' and
                 strict_json(t['arguments_json'])['operation']=='publish']
    from verify_gc16 import delivered_report
    main_messages=[m for m in value['conversation'] if m['source']!='security']
    main_ids={m['message_id'] for m in main_messages}
    return (len(publishes)==2 and {t['run_id'] for t in publishes} >= {root['run_id']} and
        all(strict_json(t['arguments_json'])['artifactRefs']==[output['ref']] for t in publishes) and
        len(value['publications'])==2 and {(p['run_id'],p['artifact_ref']) for p in value['publications']} ==
            {(t['run_id'],output['ref']) for t in publishes} and
        strict_json(value['updates'][0]['artifact_refs'])==[output['ref']] and
        delivered_report(dict(value,artifacts=[stored],conversation=main_messages,
            deliveries=[d for d in value['deliveries'] if d['message_id'] in main_ids]),output['ref'],publishes))


def observe(row):
    from verify_gc14 import frozen_generations
    from verify_children import check
    value = row['state']; before = row['fault']['before']; checkpoint = row['fault']['checkpoint']
    rows, gold = feedback(); originals = {r['row_id']:r for r in rows}
    classified = list(csv.DictReader(io.StringIO(row['artifact']['text'])))
    answers = {r['row_id']:{k:r[k] for k in ('row_id','category','severity')} for r in classified}
    children = {r['run_id']:r for r in value['runs'] if r['parent_run_id']}
    invocations = {i['invocation_id']:i for i in value['tools']}; calls = value['agent_invocation']
    completed = {r['child_run_id'] for r in before if r['status']=='completed'}
    done = {g['generation_id'] for g in checkpoint['generations'] if g['run_id'] in completed and g['status']=='completed'}
    dispatches = Counter(g['generation_id'] for g in value['external_provider_dispatches'])
    drift = row['fault']['drift']; duplicates = {}
    for r in rows:
        duplicates.setdefault(r['text'],[]).append(int(r['row_id'][1:]))
    checks = dict(
        exact_duplicate_pairs=sorted(sorted(ids) for ids in duplicates.values() if len(ids)>1)==sorted([list(p) for p in PAIRS]),
        input_artifacts_immutable=exact(row['inputs'],assets()) and all(any(a['name']==asset['name'] and a['content_hash']==sha(asset['text'].encode()) for a in value['artifact']) for asset in assets()),
        exact_100_rows=len(classified)==100 and set(answers)==set(originals),
        exact_row_classification=answers=={r['row_id']:r for r in gold},
        category_distribution=dict(Counter(r['category'] for r in classified))==CATEGORIES,
        severity_distribution=dict(Counter(r['severity'] for r in classified))==SEVERITIES,
        row_evidence=all(r['evidence']==r['row_id'] and r['attachment_ref']==originals[r['row_id']]['attachment_ref'] for r in classified),
        one_task_one_root=len(value['task'])==1 and sum(r['kind']=='worker' and not r['parent_run_id'] for r in value['runs'])==1,
        hundred_children=len(children)==len(calls)==100 and all(r['status']=='completed' for r in children.values()),
        accepted_identities_reattached=len(before)==45 and all(any(a['child_run_id']==old['child_run_id'] and a['invocation_id']==old['invocation_id'] for a in calls) for old in before),
        completed_calls_not_redispatched=len(done)==37 and all(dispatches[g]==1 for g in done),
        all_generations_dispatched_once=bool(dispatches) and all(n==1 for n in dispatches.values()),
        f055_drift_rejected=drift['rejection'].get('error',{}).get('code')=='NON_DETERMINISTIC_REPLAY',
        f055_original_restored=not drift['restored'].get('error') and drift['changed']['callKey']==drift['original']['callKey'] and
            drift['changed']['arguments']!=drift['original']['arguments'] and 'F055' in str(drift['original']['arguments']),
        eight_checkpoint_effects_once=len(value['external_effects'])==8 and len({e['key'] for e in value['external_effects']})==8,
        only_original_f055_admitted=sum('F055' in invocations[a['invocation_id']]['arguments_json'] for a in calls)==1,
        frozen_tree_model=len({r['model_snapshot_id'] for r in value['runs'] if r['kind']=='worker'})==1,
        child_only_direct_parent=all(invocations[a['invocation_id']]['run_id']==children[a['child_run_id']]['parent_run_id'] for a in calls),
        one_completion_notice=len(value['updates'])==1 and value['updates'][0]['kind']=='completed',
        bounded_questions=sum(json.dumps(m['content'],ensure_ascii=False).count('？')+json.dumps(m['content']).count('?') for m in value['conversation'] if m['role']=='assistant')<=1,
        no_leaked_execution_claim=bool(value['code_execution_state']) and all(s['state']=='completed' and s['claim_token'] is None for s in value['code_execution_state']),
        original_user=[p['text'] for m in value['conversation'] if m['role']=='user' for p in m['content']]==[prompt(row['variant'])],
        exact_checkpoint=len(completed)==37 and sum(r['status'] in ('queued','running','waiting') for r in before)==8 and
            all(any(c['invocation_id']==b['invocation_id'] and c['child_run_id']==b['child_run_id'] for c in checkpoint['child_links']) for b in before),
        actual_crashes=exact(row['fault']['signals'],['Sidecar SIGKILL','Go SIGKILL']) and exact(row['fault']['returncodes'],[-9]) and
            row['fault']['sidecar']['signal_sent']==9 and row['fault']['sidecar']['pid']!=row['fault']['sidecar']['parent_pid'] and
            (row['fault']['sidecar']['ps_returncode']==1 or
             (row['fault']['sidecar']['ps_returncode']==0 and row['fault']['sidecar']['ps_stdout'].strip().startswith('Z'))),
        committed_results_reused=all(g in value['generations'] for g in checkpoint['generations'] if g['status']=='completed') and
            all(w in value['worker_results'] for w in checkpoint['worker_results']),
        complete_replay=exact(value,row['replay_state']),
        frozen_generations=frozen_generations(value),
        no_retry_or_unknown=all(t['status']=='succeeded' and t['attempt_count']>=1 and
            (t['tool_name'] in ('builtin__codemode__execute','builtin__agent__run') or t['attempt_count']==1) for t in value['tools']) and
            all(t in value['tools'] for t in checkpoint['tools'] if t['status']=='succeeded'),
        source_manifest=len(value['code_manifest'])==len(value['code_execution'])==1 and
            value['code_manifest'][0]['source']==row['source']==source_for(assets()) and
            value['code_execution'][0]['source_hash']==sha(row['source'].encode()),
        native_bindings=bindings(row),
        artifact_readback=sha(row['artifact']['text'].encode())==row['artifact']['sha256'],
        no_automation=not value['automation'])
    try:
        check(value,children=100)
        checks['child_contracts'] = True
    except RuntimeError:
        checks['child_contracts'] = False
    approvals = value['approval']; effects = [t for t in value['tools'] if t['tool_name']=='mcp__fixture__send']
    checks['frozen_checkpoint_approvals'] = len(approvals)==len(effects)==8 and all(
        a['status']=='approved' and a['frozen_arguments_hash']==invocations[a['invocation_id']]['request_hash'] and
        invocations[a['invocation_id']]['tool_name']=='mcp__fixture__send' for a in approvals)
    ledger={e['key']:e for e in value['external_effects']}
    checks['external_effect_bindings'] = len(ledger)==8 and all(t['idempotency_key'] in ledger and
        exact(strict_json(ledger[t['idempotency_key']]['args']),strict_json(t['arguments_json'])) and
        ledger[t['idempotency_key']]['result']==strict_json(t['result_json'])['text'] and
        strict_json(t['arguments_json'])['target']=='feedback-checkpoint' for t in effects)
    wrapper = next(t for t in value['tools'] if t['tool_name']=='builtin__codemode__execute')
    summary = strict_json(strict_json(wrapper['result_json'])['text'])
    checks['row_consistent_summary'] = exact(summary,[dict(type='return',value=dict(count=100,categories=CATEGORIES,
        severities=SEVERITIES,artifactRefs=[row['artifact']['ref']]))]) and any(
            w['content']=='反馈分析汇总（资料不足保留待确认）：'+strict_json(wrapper['result_json'])['text']
            for w in value['worker_results'] if w['run_id']==wrapper['run_id'])
    checks['security_delivery_identity'] = (all(d['status']=='sent' and
        d['delivery_id']==d['idempotency_key']=='delivery:'+d['message_id'] for d in value['deliveries']) and
        len(value['deliveries'])==10 and {m['source_event_id'] for m in value['conversation'] if m['source']=='security'}=={
            'approval:'+a['approval_id'] for a in approvals})
    active=set();peak=0;ended=set()
    for event in sorted(value['trace'],key=lambda t:(t['occurred_at'],t['sequence'])):
        if event['run_id'] not in children:
            continue
        kind=strict_json(event['event_json'])['kind']
        if kind=='agent_start':
            active.add(event['run_id']);peak=max(peak,len(active))
        elif kind=='agent_end':
            active.discard(event['run_id']);ended.add(event['run_id'])
    checks['host_bounded_lifetimes'] = not active and ended==set(children) and 1<peak<=16
    checks['completed_task'] = exact(value['task'],value['tasks']) and value['task'][0]['status']=='completed'
    return checks


def pack(value):
    data = json.dumps(value,ensure_ascii=False,separators=(',',':')).encode()
    compressed = gzip.compress(data,compresslevel=1,mtime=0)
    return dict(kind='gc17_native',sha256=sha(compressed),bytes=len(compressed),uncompressed_bytes=len(data),
                data=base64.b64encode(compressed).decode())


def unpack(value):
    require(set(value)=={'kind','sha256','bytes','uncompressed_bytes','data'} and value['kind']=='gc17_native',
            'GC-17 compressed native receipt required')
    require(type(value['bytes']) is int and 0<value['bytes']<=512<<20 and type(value['data']) is str and
            len(value['data'])==4*((value['bytes']+2)//3),'GC-17 bounded compressed bytes required')
    data = base64.b64decode(value['data'],validate=True)
    require(len(data)==value['bytes'] and sha(data)==value['sha256'],'GC-17 native hash/size drift')
    size=value['uncompressed_bytes']
    require(type(size) is int and 0<size<=512<<20,'GC-17 bounded native bytes required')
    with gzip.GzipFile(fileobj=io.BytesIO(data)) as stream:
        raw=stream.read(size+1)
    require(len(raw)==size,'GC-17 decompressed size drift')
    return strict_json(raw)


def validate_result(result, binary=None):
    from unscripted_cases import source_identity
    from verify_gc11 import environment
    row = unpack(result['evidence'])
    dataset, _, lock = load_contract(ROOT,'core-v1')
    case = dataset['cases'][0]
    variant = next(v for v in variants(dataset,case) if v['variant_id']==result['variant_id'])
    identity = row['identity']
    require(exact(row['variant'],variant) and exact(row['boundary_lock'],load()) and
        exact(identity['source_before'],identity['source_after']) and exact(identity['source_before'],source_identity()) and
        re.fullmatch('[0-9a-f]{64}',identity['binary_sha256']) and
        all(type(identity['environment'][k]) is str and identity['environment'][k] for k in ('platform','python','node','go','binary_build')),
        'GC-17 source/variant/build/environment drift')
    require(identity['environment']['node']=='v'+(ROOT/'.node-version').read_text().strip() and
        re.fullmatch(r'3\.(?:1[0-9]|[2-9][0-9])\.\d+',identity['environment']['python']) and
        identity['environment']['go'].startswith('go version '+identity['environment']['binary_build'].splitlines()[0]+' '),
        'GC-17 invalid pinned environment/build identity')
    if binary is not None:
        require(sha(Path(binary).read_bytes())==identity['binary_sha256'] and exact(environment(binary),identity['environment']),
                'GC-17 current binary/environment drift')
    require(exact(row['cleanup'],dict(owned_processes_joined=True,disposable_root_removed=True)) and
        row['owned_processes_joined'] is True and row['scope']=='scripted_whole_case_mechanics' and row['eligible_for_grading'] is False and
        row.get('semantic_score','missing') is None and row.get('release_ready') is False,'GC-17 cleanup/grade drift')
    checks=observe(row)
    require(exact(checks,row['assertions']) and exact(checks,result['assertions']) and all(v is True for v in checks.values()) and
        result['status']=='fragment_pass' and result['hard_failure'] is False and result['final_score'] is None and
        exact(result['dimensions'],{d:None for d in 'OFPATSRC'}) and result['seed']==1701 and
        result['case_version']==1 and result['clock']==variant['clock'] and exact(result['input_events'],variant['events']) and
        result['fixture_digest']==digest({'variant':variant,'locked_files':lock['files']}), 'GC-17 failed or forged native receipt')
    return row


def validate_report(report, binary=None, seen=None):
    require(report.get('cleanup') is True,'GC-17 report cleanup required')
    rows = [r for r in report['results'] if r['case_id']=='GC-17']
    dataset, _, lock=load_contract(ROOT,'core-v1')
    require([r['variant_id'] for r in rows]==[v['variant_id'] for v in variants(dataset,dataset['cases'][0])] and
        report['contract_lock']==lock and report['dataset_digest']==digest(dataset),'GC-17 complete ordered six variants required')
    seen=set() if seen is None else seen
    for result in rows:
        row=validate_result(result,binary)
        require(row['identity']['binary_sha256']==report['binary_sha256'],'GC-17 binary/report drift')
        for table,key in [('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),
                          ('conversation','message_id'),('deliveries','delivery_id')]:
            ids=[(table,r[key]) for r in row['state'][table]]
            require(bool(ids) and len(ids)==len(set(ids)) and seen.isdisjoint(ids),'GC-17 duplicated execution identity: '+table)
            seen.update(ids)
    return dict(case_id='GC-17',receipts=6,semantic_score=None,eligible_for_grading=False,release_ready=False)


def mutations():
    return dict(
        wrong_delivered_ref=lambda r:r['state']['conversation'][-1]['content'][-1].update(artifact_ref='artifact:'+'0'*64),
        missing_file=lambda r:r['state']['conversation'][-1]['content'].pop(),
        duplicate_file=lambda r:r['state']['conversation'][-1]['content'].append(copy.deepcopy(r['state']['conversation'][-1]['content'][-1])),
        extra_file=lambda r:r['state']['conversation'][-1]['content'].append(dict(type='file',artifact_ref='artifact:'+'0'*64)),
        wrong_file_type=lambda r:r['state']['conversation'][-1]['content'][-1].update(type='image'),
        early_file=lambda r:r['state']['conversation'][1]['content'].append(copy.deepcopy(r['state']['conversation'][-1]['content'][-1])),
        wrong_reply_text=lambda r:r['state']['conversation'][-1]['content'][0].update(text='forged'),
        wrong_reply_generation=lambda r:r['state']['conversation'][-1].update(source_event_id=r['state']['conversation'][1]['source_event_id']),
        wrong_publication=lambda r:r['state']['publications'][-1].update(run_id='wrong-main'),
        unsent_delivery=lambda r:r['state']['deliveries'][-1].update(status='pending'),
        wrong_delivery_key=lambda r:r['state']['deliveries'][-1].update(idempotency_key='other-key'),
        wrong_csv=lambda r:r['artifact'].update(text=r['artifact']['text'].replace('F099','F098')),
        artifact_hash=lambda r:next(a for a in r['state']['artifact'] if a['name']=='feedback_analysis.csv').update(content_hash='0'*64),
        artifact_owner=lambda r:next(a for a in r['state']['artifact'] if a['name']=='feedback_analysis.csv').update(run_id='other-root'),
        source_key=lambda r:r['state']['code_call'][0].update(call_key='0'*64),
        forged_child_result=lambda r:r['state']['worker_results'][0].update(content='{}'),
        forged_child_link=lambda r:r['state']['child_links'][0].update(child_run_id='other-child'),
        old_epoch=lambda r:next(c for c in r['state']['runs'] if c['parent_run_id']).update(task_epoch=99),
        changed_model=lambda r:next(c for c in r['state']['runs'] if c['parent_run_id']).update(model_snapshot_id='other-model'),
        expanded_capability=lambda r:r['state']['capabilities'][0].update(tools_json='[]'),
        approval_hash=lambda r:r['state']['approval'][0].update(frozen_arguments_hash='0'*64),
        repeated_effect=lambda r:r['state']['external_effects'].append(copy.deepcopy(r['state']['external_effects'][0])),
        changed_effect=lambda r:r['state']['external_effects'][0].update(args='{}'),
        unknown=lambda r:r['state']['tools'][0].update(status='unknown'),
        redispatch=lambda r:r['state']['external_provider_dispatches'].append(copy.deepcopy(r['state']['external_provider_dispatches'][0])),
        wrong_cut=lambda r:r['fault']['before'].pop(),
        fake_crash=lambda r:r['fault'].update(returncodes=[0,0]),
        erased_drift=lambda r:r['fault']['drift']['rejection'].clear(),
        forged_source=lambda r:r['state']['code_manifest'][0].update(source='return 100;'))


def consumer_probes(report, binary, folder):
    """Positive control plus replay-synchronized, recomputed, resealed attacks."""
    from ci_codemode import validate_gc17
    from prepare_release_eval import prepare
    folder.mkdir(parents=True,exist_ok=True)
    result = {}
    with tempfile.TemporaryDirectory(prefix='erii-gc17-consumers-') as temporary:
        path = Path(temporary)/'candidate.json'
        control = json.dumps(report,ensure_ascii=False,separators=(',',':')).encode()
        for name, mutate in [('control',None),*mutations().items()]:
            candidate = copy.deepcopy(report)
            if mutate:
                target = candidate['results'][0]
                row = unpack(target['evidence'])
                mutate(row)
                row['replay_state'] = copy.deepcopy(row['state'])
                try:
                    checks = observe(row)
                except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
                    checks = dict(invalid_native_state=False)
                row['assertions']=target['assertions']=checks
                # Preserve forged passing claims; the consumer must recheck.
                target['evidence']=pack(row)
            data=json.dumps(candidate,ensure_ascii=False,separators=(',',':')).encode()
            path.write_bytes(data)
            # Retain every byte without repeating the unchanged five receipts
            # in each corruption. Reconstruction is exact, checked below.
            retained = data if mutate is None else json.dumps(dict(base_sha256=sha(control),
                replacement_result=candidate['results'][0]),ensure_ascii=False,separators=(',',':')).encode()
            archive = folder/(name+'.json.gz')
            archive.write_bytes(gzip.compress(retained,compresslevel=1,mtime=0))
            restored = strict_json(gzip.decompress(archive.read_bytes()))
            if mutate:
                require(restored['base_sha256']==sha(control),'GC-17 corruption base drift')
                rebuilt = strict_json(control)
                rebuilt['results'][0] = restored['replacement_result']
                require(json.dumps(rebuilt,ensure_ascii=False,separators=(',',':')).encode()==data,'GC-17 corruption retention drift')
            command=[sys.executable,str(ROOT/'scripts/run_core_cases.py'),'--binary',str(binary),'--evidence',str(path),'--validate']
            cli=subprocess.run(command,capture_output=True,text=True,timeout=30)
            accepted=dict(cli=cli.returncode==0)
            for consumer, action in [('ci',lambda:validate_gc17(strict_json(path.read_bytes()))),
                                     ('judge',lambda:prepare(strict_json(path.read_bytes())))]:
                try:
                    action();accepted[consumer]=True
                except (RuntimeError,ValueError,KeyError,IndexError,TypeError,StopIteration):
                    accepted[consumer]=False
            result[name]=dict(accepted=accepted,sha256=sha(data),command=command,returncode=cli.returncode,
                             stdout=cli.stdout,stderr=cli.stderr)
            require(all(accepted.values()) if mutate is None else not any(accepted.values()),
                    'GC-17 consumer control/rejection failure: '+name+' '+str(accepted))
    (folder/'results.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    return {name:r['accepted'] for name,r in result.items()}
