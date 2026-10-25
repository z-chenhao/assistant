#!/usr/bin/env python3
"""GC-03 original finance journey and independently recomputed native receipts.

Scripted mechanical proof only. No model understanding or financial advice grade.
"""
import argparse
import copy
from decimal import Decimal, ROUND_HALF_UP
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
from verify_main import require
from verify_memory_maintenance import manage, work, readback
from verify_recall import Embeddings

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / 'evals/fixtures/gc03-v1.lock.json'
REPEATS = 5


def exact(value, expected):
    return json.dumps(value,sort_keys=True,ensure_ascii=False,allow_nan=False)==json.dumps(expected,sort_keys=True,ensure_ascii=False,allow_nan=False)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT / name).read_bytes()) == expected, 'GC-03 lock drift: ' + name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT / 'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 21 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current for b in blocks),
        'GC-03 frozen block drift')
    return lock


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT, 'm6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-03')
    if variant_id is None:
        return data, case
    return case, next(v for v in variants(data, case) if v['variant_id'] == variant_id)


def option_record(current=False):
    world = inputs()[1]['world']
    return dict(subject='期权', buyback_usd=250 if current else world['employer_options']['last_buyback_usd'],
        vested_remaining=220 if current else world['employer_options']['vested_remaining'],
        as_of=inputs()[1]['clock'], currency='USD')


def calculation_source():
    # Consume the current version returned by the actual Memory Tool. Do not
    # bake the corrected amounts or Memory ID into the calculation script.
    clock = json.dumps(inputs()[1]['clock'])
    return save_report('const as_of=' + clock + ';\n' + '''const world=await tools.mcp__fixture__read({text:"world"});
const hits=(await tools.builtin__memory__search({query:"期权",limit:20})).filter(h=>h.source_type==="memory");
const current=hits.filter(h=>h.content.startsWith("{")).map(h=>({hit:h,value:JSON.parse(h.content)})).filter(h=>h.value.subject==="期权");
if(current.length!==1)throw Error("one current option record required");
const update=current[0].value;
const usd=Math.round(update.vested_remaining*(update.buyback_usd-world.employer_options.strike_usd)*100)/100;
const cny=Math.round(usd*world.fx.usd_cny*100)/100;
const liquid=world.assets_cny.cash+world.assets_cny.broad_index_funds;
const result={as_of,update,source:{world:"mcp__fixture__read(world)",update_memory_ref:current[0].hit.source_ref},
  assets:{cash_cny:world.assets_cny.cash,broad_index_funds_cny:world.assets_cny.broad_index_funds,
    liquid_cny:liquid,options:{usd,cny,currency:"USD",strike_usd:world.employer_options.strike_usd,
      liquidity:world.employer_options.liquidity,pre_tax:true,tax:null,window_execution_guaranteed:false}},
  fx:world.fx,household:world.household,insurance:null,retirement_target:null,
  assumptions:{returns:0,inflation:0,income:0,fees:null,travel_in_annual_spend:"unknown",forecast_guaranteed:false,
    scope:"static liquid-asset coverage; not a retirement date or promised return"},
  formulas:{usd:"vested_remaining * (buyback_usd - strike_usd)",cny:"usd * usd_cny",liquid_years:"liquid_cny / annual_spend_cny"},
  scenarios:[300000,350000,400000].map(annual_spend_cny=>({annual_spend_cny,liquid_years:liquid/annual_spend_cny}))};''', 'finance.json')


def recall(h, key):
    return execute(h, key, 'return (await tools.builtin__memory__search({query:"期权",limit:20})).filter(h=>h.source_type==="memory");')


def select(h, bridge):
    h.bridge = bridge
    h.setup(metadata=dict(name='GC-03 explicit ' + bridge, provider_kind='openai_compatible',
        endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions', model_id='gc03-' + bridge + '-fixture',
        context_window=65536, max_output_tokens=4096, generation_defaults={},
        compatibility=dict(tool_result_format='openai'), capabilities=dict(text_input=True,image_input=True)))


def journey(h, variant_id, row):
    case, variant = inputs(variant_id)
    row['variant'] = variant
    row['old_ref'] = remember(h, 'options-baseline', json.dumps(option_record(),ensure_ascii=False))
    row['initial_recall'] = recall(h, 'initial-options')
    select(h, 'terminal-input')
    h.send('context', variant['context'])
    first = variant['events'][0]
    row['first_receipt'] = h.send('event-0', first['at'] + ' ' + first['text'])
    row['input_restart'] = kill(h, 'after_generation_commit')
    h.command('chat','work'); work(h)
    receipt = manage(h, 'options-correction', operation='remember',
        content=json.dumps(option_record(True),ensure_ascii=False), supersedes=row['old_ref'])
    row['memory_event_id'] = receipt['receipt'].removeprefix('memory-event:')
    row['memory_restart'] = kill(h, 'after_memory_commit:' + row['memory_event_id'], ('memory','work'))
    work(h)
    row['current_ref'] = 'memory:' + next(r['memory_id'] for r in readback(h)['memory']['records']
        if r['memory_id'].startswith(row['memory_event_id']))
    with Terminal(h.args('tui'), h.env, h.root) as terminal:
        terminal.expect('State: ready'); terminal.expect('You> ')
        event = variant['events'][1]
        terminal.send(event['at'] + ' ' + event['text'])
        terminal.expect('assistant: '); terminal.expect('You> ')
        require(terminal.stop() == 0, 'GC-03 TUI did not stop')
        row['transcript'] = terminal.transcript()
    event = variant['events'][2]
    row['last_receipt'] = h.send('event-2', event['at'] + ' ' + event['text'])
    h.command('chat','work'); work(h)
    select(h,'capabilities')
    row['calculation'] = execute(h,'finance',calculation_source())
    row['artifacts'] = []
    for descriptor in native(h)['artifact']:
        raw = open_bytes(h,descriptor['artifact_ref'])
        row['artifacts'].append(dict(descriptor=descriptor,text=raw.decode(),sha256=sha(raw),bytes=len(raw)))
    work(h)
    row['calculated'] = native(h)
    # A supplemental privacy probe freezes a dependent old-owner request, then
    # Forget fences it before unrelated input and Recall on fresh Main Runs.
    row['revoked_receipt'] = h.send('dependent-on-options','请根据期权继续分析。')
    row['forget_restart'] = kill(h,'before_generation_dispatch')
    manage(h,'forget-options',operation='forget',memory_ref=row['current_ref'])
    row['fresh_receipt'] = h.send('unrelated','给我一个整理书桌的小建议。')
    h.command('chat','work'); work(h)
    row['forgotten_recall'] = recall(h,'after-forget')
    work(h)
    row['final'] = native(h)
    require(h.send('event-0',first['at'] + ' ' + first['text']) == row['first_receipt'],'input identity drift')
    h.command('chat','work'); work(h)
    row['replayed'] = native(h)


def expected_report(current_ref):
    # Python Decimal independently recomputes the JS numbers from locked input.
    case = inputs()[1]; world = case['world']; update = option_record(True)
    usd = Decimal(update['vested_remaining']) * (Decimal(str(update['buyback_usd'])) - Decimal(str(world['employer_options']['strike_usd'])))
    cny = (usd * Decimal(str(world['fx']['usd_cny']))).quantize(Decimal('.01'),rounding=ROUND_HALF_UP)
    liquid = sum(world['assets_cny'].values())
    return dict(as_of=case['clock'],update=update,
        source=dict(world='mcp__fixture__read(world)',update_memory_ref=current_ref),
        assets=dict(cash_cny=world['assets_cny']['cash'],broad_index_funds_cny=world['assets_cny']['broad_index_funds'],
            liquid_cny=liquid,options=dict(usd=float(usd),cny=float(cny),currency='USD',strike_usd=world['employer_options']['strike_usd'],
                liquidity=world['employer_options']['liquidity'],pre_tax=True,tax=None,window_execution_guaranteed=False)),
        fx=world['fx'],household=world['household'],insurance=None,retirement_target=None,
        assumptions=dict(returns=0,inflation=0,income=0,fees=None,travel_in_annual_spend='unknown',forecast_guaranteed=False,
            scope='static liquid-asset coverage; not a retirement date or promised return'),
        formulas=dict(usd='vested_remaining * (buyback_usd - strike_usd)',cny='usd * usd_cny',liquid_years='liquid_cny / annual_spend_cny'),
        scenarios=[dict(annual_spend_cny=spend,liquid_years=float(Decimal(liquid)/Decimal(spend))) for spend in (300000,350000,400000)])


def recalled_context(generations, run_id):
    return [hit for g in generations if g['run_id']==run_id
        for m in json.loads(g['request_json'])['messages']
        if m.get('text','').startswith('Retrieved durable context (')
        for hit in json.loads(m['text'].split('\n',1)[1])]


def calculation_return(calculation):
    # The persisted Tool result is authoritative. output/value are retained
    # convenience copies, not independent evidence of what the Tool returned.
    output = strict_json(strict_json(calculation['parent']['result_json'])['text'])
    require(type(output) is list, 'GC-03 native calculation output must be a list')
    returned = [p['value'] for p in output if p['type']=='return']
    require(len(returned)==1, 'GC-03 requires one native calculation return')
    require(exact(output,calculation['output']) and exact(returned[0],calculation['value']),
        'GC-03 native calculation output/value drift')
    return returned[0]


def observe(row):
    returned = calculation_return(row['calculation'])
    case, variant = inputs(row['variant_id'])
    value, final = row['calculated'], row['final']
    old, current = row['old_ref'], row['current_ref']
    users = [p['text'] for m in final['conversation'] if m['role']=='user' for p in m['content']]
    originals = [variant['context']] + [e['at']+' '+e['text'] for e in variant['events']]
    positions = [users.index(text) if text in users else -1 for text in originals]
    records = value['memory']['memory']['records']
    original = next(r for r in records if 'memory:'+r['memory_id']==old)
    corrected = next(r for r in records if 'memory:'+r['memory_id']==current)
    calculation = row['calculation']
    nested = calculation['nested']
    hits = json.loads(json.loads(nested[1]['result_json'])['text'])
    memory_hits = [h for h in hits if h['source_type']=='memory']
    expected = expected_report(current)
    checks = main_checks(final)
    checks.update(
        exact_original_inputs=row['variant']==variant and min(positions)>=0 and positions==sorted(set(positions)) and
            all(users.count(text)==1 for text in originals),
        original_model_requests=all(any(text==m.get('text') for g in final['generations'] for m in json.loads(g['request_json'])['messages']) for text in originals),
        tui_input=variant['events'][1]['text'] in row['transcript'],
        original_source_tool=len(nested)==4 and [t['tool_name'] for t in nested]==[
            'mcp__fixture__read','builtin__memory__search','builtin__artifact__manage','builtin__artifact__manage'] and
            json.loads(json.loads(nested[0]['result_json'])['text'])==case['world'],
        native_tool_transport=recorded(calculation['parent'],value['tools']) and all(recorded(t,value['tools']) for t in nested) and
            json.loads(calculation['parent']['arguments_json'])==dict(source=calculation_source()),
        original_memory=recorded(row['initial_recall']['parent'],value['tools']) and all(recorded(t,value['tools']) for t in row['initial_recall']['nested']) and
            row['initial_recall']['value']==[h for h in json.loads(json.loads(row['initial_recall']['nested'][0]['result_json'])['text']) if h['source_type']=='memory'] and
            any(h['source_ref']==old and exact(json.loads(h['content']),option_record()) for h in row['initial_recall']['value']),
        original_and_supersede=exact(json.loads(next(h['content'] for h in memory_hits if h['source_ref']==current)),option_record(True)) and
            corrected['supersedes_memory_id']==original['memory_id'] and original['deleted_at'] is None and corrected['deleted_at'] is None and
            all(h['source_ref']!=old for h in memory_hits),
        input_generation_reused=row['input_restart']['returncode']==-9 and row['input_restart']['boundary']=='after_generation_commit' and
            any(g['run_id']==row['first_receipt']['run_id'] and g['status']=='completed' for g in row['input_restart']['state']['generations']) and
            all(g in value['generations'] for g in row['input_restart']['state']['generations'] if g['status']=='completed') and
            sum(r['run_id']==row['first_receipt']['run_id'] for r in value['runs'])==1,
        memory_crash_recovered=row['memory_restart']['returncode']==-9 and
            row['memory_restart']['boundary']=='after_memory_commit:'+row['memory_event_id'] and
            corrected in row['memory_restart']['state']['memory']['memory']['records'] and
            sum(r['event_id']==row['memory_event_id'] for r in value['memory']['memory']['receipts'])==1,
        independently_recomputed_finance=exact(returned['result'],expected),
        artifacts_reopened=len(row['artifacts'])==len(value['artifact'])==1 and all(
            a['descriptor'] in value['artifact'] and a['sha256']==sha(a['text'].encode())==a['descriptor']['content_hash'] and
            a['bytes']==len(a['text'].encode())==a['descriptor']['byte_size'] and exact(strict_json(a['text'].encode()),expected) and
            returned['artifactRefs']==[a['descriptor']['artifact_ref']] and any(
                p['artifact_ref']==a['descriptor']['artifact_ref'] for p in value['artifact_publication']) for a in row['artifacts']),
        all_tools_once=all(t['status']=='succeeded' and t['attempt_count']==1 for t in final['tools']),
        final_replay_equal=final==row['replayed'])
    revoked, fresh = row['revoked_receipt']['run_id'], row['fresh_receipt']['run_id']
    checks['forget_fences_old_owner'] = row['forget_restart']['returncode']==-9 and row['forget_restart']['boundary']=='before_generation_dispatch' and \
        any(hit['source_ref']==current and json.loads(hit['content'])==option_record(True) for hit in recalled_context(row['forget_restart']['state']['generations'],revoked)) and \
        any(r['run_id']==revoked and r['status']=='cancelled' and r['error_code']=='memory_forgotten' for r in final['runs']) and \
        any(r['run_id']==fresh and r['status']=='completed' for r in final['runs']) and fresh!=revoked
    requests = [json.loads(g['request_json']) for g in final['generations'] if g['run_id'] in (fresh,final['runs'][-1]['run_id'])]
    contents = [m.get('text','') for request in requests for m in request['messages']]
    forgotten = row['forgotten_recall']
    checks['forget_transport'] = recorded(forgotten['parent'],final['tools']) and len(forgotten['nested'])==1 and \
        forgotten['nested'][0]['tool_name']=='builtin__memory__search' and recorded(forgotten['nested'][0],final['tools']) and \
        forgotten['value']==[h for h in json.loads(json.loads(forgotten['nested'][0]['result_json'])['text']) if h['source_type']=='memory']
    checks['deleted_recall_context_absent'] = bool(requests) and all(
        ref not in json.dumps(request,ensure_ascii=False) for ref in (old,current) for request in requests) and all(
        json.dumps(option_record(updated),ensure_ascii=False) not in text for updated in (False,True) for text in contents) and \
        all(h['source_ref'] not in (old,current) for h in forgotten['value']) and all(
            r['deleted_at'] is not None for r in final['memory']['memory']['records'] if 'memory:'+r['memory_id'] in (old,current))
    return checks


def rejection_probes(row):
    def rehash(r):
        a=r['artifacts'][0];body=json.loads(a['text']);body['assets']['options']['tax']=0
        a['text']=json.dumps(body);a['sha256']=sha(a['text'].encode());a['bytes']=len(a['text'].encode())
        a['descriptor'].update(content_hash=a['sha256'],byte_size=a['bytes'])
    def native_return(r, mutate, sync_summary=False):
        calculation = r['calculation']
        parent = calculation['parent']
        envelope = strict_json(parent['result_json'])
        output = strict_json(envelope['text'])
        returned = next(p['value'] for p in output if p['type']=='return')
        mutate(returned)
        envelope['text'] = json.dumps(output,ensure_ascii=False)
        result_json = json.dumps(envelope,ensure_ascii=False)
        # Keep all persisted copies consistent so rejection must detect the
        # native return/summary or finance/Artifact contradiction itself.
        def replace(value):
            if isinstance(value,dict):
                if value.get('invocation_id')==parent['invocation_id'] and 'result_json' in value:
                    value['result_json'] = result_json
                for child in value.values():
                    replace(child)
            elif isinstance(value,list):
                for child in value:
                    replace(child)
        replace(r)
        if sync_summary:
            calculation['output'] = copy.deepcopy(output)
            calculation['value'] = copy.deepcopy(returned)
    mutations = dict(
        native_stale_price=lambda r:native_return(r,lambda v:v['result']['update'].update(buyback_usd=240)),
        native_tax_certainty=lambda r:native_return(r,lambda v:v['result']['assets']['options'].update(tax=0)),
        native_artifact_ref=lambda r:native_return(r,lambda v:v.update(artifactRefs=['artifact:forged'])),
        synced_native_stale_price=lambda r:native_return(r,lambda v:v['result']['update'].update(buyback_usd=240),True),
        synced_native_tax_certainty=lambda r:native_return(r,lambda v:v['result']['assets']['options'].update(tax=0),True),
        synced_native_artifact_ref=lambda r:native_return(r,lambda v:v.update(artifactRefs=['artifact:forged']),True),
        parsed_output_drift=lambda r:r['calculation']['output'].append(dict(type='text',text='forged')),
        changed_input=lambda r:r['variant']['events'][0].update(text='替换原文'),
        stale_price=lambda r:r['calculation']['value']['result']['update'].update(buyback_usd=240),
        stale_quantity=lambda r:r['calculation']['value']['result']['update'].update(vested_remaining=200),
        liquid_options=lambda r:r['calculation']['value']['result']['assets'].update(liquid_cny=890120.39),
        tax_certainty=lambda r:r['calculation']['value']['result']['assets']['options'].update(tax=0),
        missing_fx_date=lambda r:r['calculation']['value']['result']['fx'].pop('as_of'),
        invented_insurance=lambda r:r['calculation']['value']['result'].update(insurance='已投保'),
        guaranteed_return=lambda r:r['calculation']['value']['result']['assumptions'].update(forecast_guaranteed=True),
        numeric_boolean=lambda r:r['calculation']['value']['result']['assets']['options'].update(pre_tax=1),
        bad_scenario=lambda r:r['calculation']['value']['result']['scenarios'][0].update(liquid_years=100),
        forged_source=lambda r:r['calculation']['nested'][0].update(invocation_id='forged'),
        broken_lineage=lambda r:next(m for m in r['calculated']['memory']['memory']['records'] if 'memory:'+m['memory_id']==r['current_ref']).update(supersedes_memory_id=None),
        false_restart=lambda r:r['memory_restart'].update(returncode=0),
        duplicate_attempt=lambda r:r['final']['tools'][0].update(attempt_count=2),
        external_effect=lambda r:r['final']['tools'].append(dict(tool_name='trade',run_id=r['final']['runs'][0]['run_id'])),
        corrupt_artifact=lambda r:r['artifacts'][0].update(sha256='0'*64),
        rehashed_tax_certainty=rehash,
        revived_owner=lambda r:next(n for n in r['final']['runs'] if n['run_id']==r['revoked_receipt']['run_id']).update(status='completed'),
        deleted_recall=lambda r:r['forgotten_recall']['value'].append(dict(source_ref=r['current_ref'])),
        replay_drift=lambda r:r['replayed']['deliveries'].append({}))
    result = {}
    for name, mutate in mutations.items():
        changed = copy.deepcopy(row)
        try:
            mutate(changed); result[name] = not all(observe(changed).values())
        except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
            result[name] = True
        require(result[name], 'GC-03 oracle accepted corruption: '+name)
    return result


def drive(binary, item, failure_evidence):
    variant_id, repetition = item
    row = dict(variant_id=variant_id,repetition=repetition)
    embeddings = Embeddings()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-gc03-') as temp:
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
    require(report.get('case_id') == 'GC-03' and report.get('status') == 'mechanical_pass' and
        report.get('release_ready') is False and 'semantic_score' in report and report['semantic_score'] is None and report.get('eligible_for_grading') is False and
        report.get('fixture_lock') == lock and report.get('m6_lock') == load_contract(ROOT,'m6-v1')[2] and report.get('cleanup') is True,
        'complete locked ungraded GC-03 receipts required')
    identity = report['identity']
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']), 'GC-03 source/binary identity drift')
    rows = report['results']
    require([(r['variant_id'],r['repetition']) for r in rows] == expected and all(type(r['repetition']) is int for r in rows),
        'all six variants and five ordered repetitions required')
    for row in rows:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()), 'failed or forged GC-03 native receipt')
    require(report.get('rejection_probes') == rejection_probes(rows[0]), 'missing/forged GC-03 corruption probes')
    return dict(case_id='GC-03',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,receipts=len(rows),frozen_blocks=21)


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            value.update(block)
    return value.hexdigest()


def retain_report(path, report):
    # The pinned CI uploader excludes hidden files. Keep all native bytes on
    # disk; only the small public manifest is collected/uploaded automatically.
    raw = path.parent / '.gc03-native' / path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    raw.write_text(json.dumps(report,ensure_ascii=False)+'\n')
    summary = dict(case_id='GC-03',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=21,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    manifest = dict(kind='gc03_native_manifest',version=1,summary=summary,
        native=dict(path=str(raw.relative_to(path.parent)),sha256=file_hash(raw)),native_bytes=raw.stat().st_size,
        proof_limit='Native bytes required for recomputation; summary alone cannot certify a Case or release.')
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','proof_limit'} and
        manifest['kind'] == 'gc03_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-03 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-03 native size mismatch')
    report = strict_json(raw)
    expected = dict(case_id='GC-03',status=report['status'],release_ready=False,semantic_score=None,eligible_for_grading=False,
        receipts=len(report['results']),frozen_blocks=21,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),
        'GC-03 manifest/native summary drift')
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
    native_folder = args.evidence.parent / '.gc03-native'
    native_folder.mkdir(exist_ok=True)
    report = dict(case_id='GC-03',status='inconclusive',release_ready=False,semantic_score=None,eligible_for_grading=False,
        fixture_lock=load(),m6_lock=load_contract(ROOT,'m6-v1')[2],results=[],cleanup=False,
        identity=dict(source_before=source_identity(),binary_sha256=sha(binary.read_bytes()),environment=dict(
            platform=platform.platform(),python=platform.python_version(),node=subprocess.check_output(['node','--version'],text=True).strip(),
            go=subprocess.check_output(['go','version'],text=True).strip(),binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-03; original input fragments only; asset/Memory adapter incomplete',
            child_approval_unknown='synchronous Main only; no Task, Child, financial effect or approval; unknown effects are inapplicable'))
    ids = [v['variant_id'] for v in variants(*inputs())]
    order = {(v,n):i*REPEATS+n for i,v in enumerate(ids) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    def series(variant_id):
        for repetition in range(1,REPEATS+1):
            row = drive(binary,(variant_id,repetition),native_folder / (args.evidence.stem+'-'+variant_id+'.failure.json'))
            with mutex: report['results'].append(row)
            print((variant_id,repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
            require(all(row['assertions'].values()),'GC-03 native assertion failure')
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
