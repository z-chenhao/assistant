#!/usr/bin/env python3
"""GC-10 locked local-life journey and strict native receipts; no model grade."""
import argparse
import copy
from contextlib import closing
import hashlib
import json
from pathlib import Path
import platform
import sqlite3
import subprocess
import sys
import tempfile
import threading

from case_runner import load_contract, variants
from fixture_pool import Scope, run_cases
from local_sandbox import state as remote_state
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_cases import timestamp
from terminal_driver import Terminal
from unscripted_cases import source_identity
from verify_gc09 import AgendaHarness, canonical_history
from verify_main import require

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT/'evals/fixtures/gc10-boundary-v1.json'
REPEATS = 5


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(FIXTURE.with_suffix('.lock.json').read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected,'GC-10 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 16 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks),
        'GC-10 frozen source drift')
    return lock


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT,'m6-v1')
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-10')
    if variant_id is None: return data,case
    return case,next(v for v in variants(data,case) if v['variant_id'] == variant_id)


def events(variant):
    data,_ = inputs()
    wording = next(v['wording'] for v in data['variants'] if v['id'] == variant['variant_id'])
    first = variant['events'][0]
    result = [dict(key='initial',text='当前时间：'+first['at']+'。'+first['text'])]
    for e in strict_json(FIXTURE.read_bytes())['events']:
        result.append(dict(key=e['key'],text='当前时间：'+e['at']+'。'+e['wordings'][wording]))
    return result


class LocalHarness(AgendaHarness):
    def args(self,*args):
        return [str(self.binary),'-config',str(self.root/'config.yaml'),'-test-bridge','local',*args]


def native(h):
    value = h.state()
    with closing(sqlite3.connect(h.root/'data/state.db')) as db:
        db.row_factory = sqlite3.Row
        for key,table in (('tasks','task'),('approvals','approval'),('artifacts','artifact'),
                          ('automations','automation'),('claims','run_claim')):
            value[key] = [dict(r) for r in db.execute('SELECT * FROM '+table+' ORDER BY rowid')]
    return value


def drive(binary,variant_id,repetition,failure_path):
    _,variant = inputs(variant_id)
    extra = strict_json(FIXTURE.read_bytes()); timeline = events(variant)
    row = dict(variant_id=variant_id,repetition=repetition,variant=variant,started_at=timestamp(),restarts=[],receipts={})
    with tempfile.TemporaryDirectory(prefix='erii-gc10-') as tmp:
        root = Path(tmp); peer = root/'peer'; peer.mkdir()
        (peer/'source.json').write_text(json.dumps(extra['sources'],ensure_ascii=False))
        (peer/'local_sandbox.py').write_bytes((ROOT/'scripts/local_sandbox.py').read_bytes())
        python = subprocess.check_output(['xcrun','--find','python3'],text=True).strip() if sys.platform == 'darwin' else '/usr/bin/python3'
        (root/'config.yaml').write_text(json.dumps(dict(version=1,mcp=[dict(name='local',command=python,directory=str(peer),
            args=[str(peer/'local_sandbox.py'),str(peer)],tools=dict(weather='read_only'))])))
        h = LocalHarness(binary,root)
        try:
            h.setup(); h.send('context',variant['context'])
            row['receipts']['initial'] = h.send('initial',timeline[0]['text'])
            for boundary in ('after_tool_commit','after_delivery_commit'):
                child = h.hold(boundary); child.kill(); child.communicate(timeout=10)
                row['restarts'].append(dict(boundary=boundary,returncode=child.returncode,state=native(h),remote=remote_state(peer)))
            h.command('chat','work')
            with Terminal(h.args('tui'),h.env,h.root) as terminal:
                terminal.expect('State: ready'); terminal.expect('You> ')
                terminal.send(timeline[1]['text'])
                terminal.expect('assistant: '); terminal.expect('You> ')
                require(terminal.stop() == 0,'GC-10 TUI did not stop cleanly')
                row['transcript'] = terminal.transcript()
            for e in timeline[2:]:
                row['receipts'][e['key']] = h.send(e['key'],e['text'])
                h.command('chat','work')
            row.update(state=native(h),cli_history=h.command('chat','history'),remote=remote_state(peer),
                source_after=strict_json((peer/'source.json').read_bytes()))
            for e in (timeline[0],*timeline[2:]):
                require(h.send(e['key'],e['text']) == row['receipts'][e['key']],'duplicate ingress receipt drift')
            row['drift'] = h.command('chat','send','initial',text='changed parameters',ok=False).returncode
            h.command('chat','work')
            row.update(replay_state=native(h),remote_replay=remote_state(peer))
        except BaseException as error:
            row.update(error=str(error),failure_state=h.failure_state(),remote_failure=remote_state(peer))
            failure_path.write_text(json.dumps(row,ensure_ascii=False)+'\n')
            raise
        finally:
            h.cleanup()
            row['owned_processes_joined'] = all(c.poll() is not None for c in h.children)
    row['cleanup'] = dict(disposable_root_removed=not root.exists(),owned_processes_joined=row['owned_processes_joined'])
    row['finished_at'] = timestamp()
    row['assertions'] = observe(row)
    return row


def text(message):
    return ''.join(p.get('text','') for p in message['content'])


def observe(row):
    _,variant = inputs(row['variant_id']); timeline = events(variant)
    extra = strict_json(FIXTURE.read_bytes()); sources = extra['sources']; state = row['state']
    runs = state['runs']; tools = state['tools']; generations = state['generations']
    users = [text(m) for m in state['conversation'] if m['role'] == 'user']
    replies = [m for m in state['conversation'] if m['role'] == 'assistant']
    expected_reads = []
    for index,source in enumerate(sources):
        for refresh in ([False] if index == 2 else [False,True]):
            args = dict(location=source['location'],at=source['at'],refresh=refresh)
            expected_reads.append(dict(arguments=args,result=source['current' if refresh else 'cache']))
    actual_reads = [dict(arguments=strict_json(t['arguments_json']),result=strict_json(strict_json(t['result_json'])['text'])) for t in tools]
    requests = [strict_json(g['request_json']) for g in generations]
    run_ids = [r['run_id'] for r in runs]
    initial_tools = [t for t in tools if t['run_id'] == run_ids[0]]
    checks = dict(
        original_and_continuation_inputs=row['variant'] == variant and users == [variant['context']]+[e['text'] for e in timeline],
        tui_location_input=timeline[1]['text'] in row['transcript'],
        exact_model_inputs=len(runs) == 6 and all(
            any([m['text'] for m in req['messages'] if m['role'] == 'user'] == users[:i+2]
                for g,req in zip(generations,requests) if g['run_id'] == r['run_id']) for i,r in enumerate(runs)),
        main_only=len(runs) == 6 and all(r['kind'] == 'main' and r['task_id'] is None and r['parent_run_id'] is None and r['status'] == 'completed' for r in runs),
        no_background_or_effect=not state['tasks'] and not state['approvals'] and not state['artifacts'] and not state['automations'] and
            all(t['tool_name'] == 'mcp__local__weather' and t['replay_policy'] == 'safe' and t['attempt_count'] == 1 and t['status'] == 'succeeded' for t in tools),
        sources_unmodified=row['source_after'] == sources,
        actual_source_reads=actual_reads == row['remote'] == expected_reads,
        scoped_reads=len(tools) == 9 and [sum(t['run_id'] == r['run_id'] for t in tools) for r in runs] == [2,2,1,2,2,0],
        exact_two_hour_boundary=len(tools) == 9 and actual_reads[4]['arguments']['refresh'] is False,
        fresh_after_stale=len(initial_tools) == 2 and strict_json(initial_tools[0]['arguments_json'])['refresh'] is False and
            strict_json(initial_tools[1]['arguments_json'])['refresh'] is True,
        native_tool_results_visible=all(any(m['role'] == 'tool' and strict_json(m['text']) == observed['result'] for req in requests for m in req['messages']) for observed in expected_reads),
        frozen_model_and_tools=bool(state['capabilities']) and all(not c['can_spawn_agents'] for c in state['capabilities']) and all(
            len({json.dumps(dict(model=req['model_snapshot'],tools=req['tools']),sort_keys=True)
                for g,req in zip(generations,requests) if g['run_id'] == r['run_id']}) == 1 for r in runs) and
            all(req['model_snapshot']['snapshot_id'] == runs[run_ids.index(g['run_id'])]['model_snapshot_id'] for g,req in zip(generations,requests)),
        completed_generations=len(generations) == 15 and all(g['status'] == 'completed' and g['attempt_count'] == 1 for g in generations) and len(state['usage']) == len(generations),
        cli_history=canonical_history(row['cli_history']) == state['conversation'],
        ordered_conversation=[m['sequence'] for m in state['conversation']] == list(range(1,14)) and len({m['message_id'] for m in state['conversation']}) == 13,
        one_delivery_per_reply=len(replies) == len(state['deliveries']) == 6 and
            {d['message_id'] for d in state['deliveries']} == {m['message_id'] for m in replies} and all(d['status'] == 'sent' for d in state['deliveries']),
        completed_reply_matches_delivery=all(sum(m['source_event_id'] == g['generation_id'] and text(m) == strict_json(g['result_json'])['text'] for m in replies) == 1
            for g in generations if strict_json(g['result_json']).get('text')),
        no_extra_questions_or_report=len(replies) == 6 and all(len(text(m)) <= 200 and len(text(m).splitlines()) <= 3 and '?' not in text(m) and '？' not in text(m) for m in replies),
        actual_restarts=[r['boundary'] for r in row['restarts']] == ['after_tool_commit','after_delivery_commit'] and all(type(r['returncode']) is int and r['returncode'] == -9 for r in row['restarts']),
        committed_cut=len(row['restarts']) == 2 and len(row['restarts'][0]['state']['tools']) == len(row['restarts'][0]['remote']) == 1 and
            row['restarts'][0]['state']['tools'][0]['status'] == 'succeeded' and len(row['restarts'][1]['remote']) == 2 and
            len([m for m in row['restarts'][1]['state']['conversation'] if m['role'] == 'assistant']) == 1,
        completed_calls_reused=all(t in tools for r in row['restarts'] for t in r['state']['tools'] if t['status'] == 'succeeded'),
        completed_generations_reused=all(g in generations for r in row['restarts'] for g in r['state']['generations'] if g['status'] == 'completed'),
        replay_equal=state == row['replay_state'] and row['remote'] == row['remote_replay'] and row['drift'] != 0,
        released_claims=bool(state['claims']) and all(c['token'] is None for c in state['claims']))
    for index in range(4):
        sample = sources[index]['cache' if index == 2 else 'current']
        reply = text(replies[index]) if len(replies) > index else ''
        checks['current_reply_'+str(index)] = sample['location'] in reply and sample['sampled_at'] in reply and str(sample['weather']['now_c'])+'°C' in reply and all(
            term in reply for term in ('开车','18:30','19:10','21:00','户外','气温下降')) and ('薄卫衣' in reply if index in (0,2) else '防风' in reply and '带伞' in reply)
    initial = text(replies[0]) if replies else ''
    checks['original_clothing'] = all(s in initial for s in ('短袖','薄长裤','薄卫衣')) and '厚外套' not in initial
    failed = text(replies[4]) if len(replies) > 4 else ''
    checks['unavailable_not_stale_fact'] = '刷新失败' in failed and '旧缓存不用于' in failed and '暂不能确认' in failed and '°C' not in failed
    spend = text(replies[5]) if len(replies) > 5 else ''
    checks['cost_escalation'] = all(s in spend for s in ('GC-13','不可退','冻结动作','审批','尚未预订或付款'))
    return checks


def rejection_probes(row):
    require(all(observe(row).values()),'passing native GC-10 receipt required')
    mutations = dict(
        input=lambda r:r['variant']['events'][0].update(text='replaced'),
        source=lambda r:r['source_after'][0]['current'].update(location='伪造'),
        stale_read=lambda r:r['state']['tools'][1].update(arguments_json='{"location":"杭州","at":"2026-10-12T08:10:00+08:00","refresh":false}'),
        source_result=lambda r:r['state']['tools'][0].update(result_json='{"text":"{}"}'),
        remote=lambda r:r['remote'].pop(),
        delivery=lambda r:r['state']['deliveries'].pop(),
        reply=lambda r:r['state']['conversation'][-1]['content'][0].update(text='已预订'),
        history=lambda r:r['cli_history'][-1]['content'][0].update(text='forged'),
        restart=lambda r:r['restarts'][0].update(returncode=0),
        restart_cut=lambda r:r['restarts'][0]['state']['tools'].clear(),
        replay=lambda r:r['replay_state']['tools'][0].update(attempt_count=2),
        tool_attempt=lambda r:r['state']['tools'][0].update(attempt_count=2),
        external_effect=lambda r:r['state']['tools'][0].update(tool_name='book'),
        task=lambda r:r['state']['tasks'].append({'status':'active'}),
        generation=lambda r:r['state']['generations'].pop(),
        snapshot=lambda r:r['state']['runs'][0].update(model_snapshot_id='forged'),
        leases=lambda r:r['state']['claims'][0].update(token='owned'))
    probes = {}
    for name,mutate in mutations.items():
        bad = copy.deepcopy(row)
        try:
            mutate(bad); rejected = not all(observe(bad).values())
        except (ValueError,KeyError,IndexError,TypeError,RuntimeError):
            rejected = True
        require(rejected,'GC-10 corruption accepted: '+name)
        probes[name] = True
    return probes


def validate_report(report,binary=None):
    lock = load()
    require(report.get('case_id') == 'GC-10' and report.get('status') == 'mechanical_pass' and
        report.get('fixture_lock') == lock and report.get('semantic_score','missing') is None and
        report.get('eligible_for_grading') is False and report.get('release_ready') is False and report.get('cleanup') is True,
        'complete ungraded GC-10 native report required')
    identity = report['identity']; binary = Path(binary or identity['binary_path'])
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']),'GC-10 source/binary identity drift')
    expected = [(v['variant_id'],n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require([(r['variant_id'],r['repetition']) for r in report['results']] == expected and
        all(type(r['repetition']) is int for r in report['results']),'all six variants and five repetitions required')
    independent_executions(report['results'])
    for row in report['results']:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()),'failed or forged GC-10 native receipt')
    require(report['rejection_probes'] == rejection_probes(report['results'][0]),'missing GC-10 corruption probes')
    return dict(case_id='GC-10',status='mechanical_pass',receipts=30,frozen_blocks=16,semantic_score=None,release_ready=False)


def independent_executions(rows):
    # Only the final inventory: restart cuts and replay must reuse these IDs
    # within a journey. Separate disposable journeys must never reuse them.
    for table,key in (('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),
                      ('deliveries','delivery_id'),('conversation','message_id')):
        seen = set()
        for row in rows:
            ids = [item[key] for item in row['state'][table]]
            require(bool(ids) and all(type(value) is str and value for value in ids),
                'invalid GC-10 native execution identity: '+table+'/'+key)
            require(len(set(ids)) == len(ids) and seen.isdisjoint(ids),
                'reused GC-10 native execution identity: '+table+'/'+key)
            seen.update(ids)


def repetition_rejection_probes(report,binary,folder):
    """Copy real passing journeys, reseal bytes, and exercise both consumers."""
    from ci_communication import native_receipt, check_receipts
    from verify_suite import SHARDS, report_name
    gate = next(g for g in SHARDS['communication'] if g.name == 'gc10')
    folder.mkdir(parents=True,exist_ok=True)
    forged = copy.deepcopy(report)
    for offset in range(0,len(forged['results']),REPEATS):
        for index in range(1,REPEATS):
            row = copy.deepcopy(forged['results'][offset])
            row['repetition'] = index+1
            forged['results'][offset+index] = row
    path = folder/report_name(gate,1)
    retain_report(path,forged)
    expected = 'reused GC-10 native execution identity: runs/run_id'
    scope = Scope()
    try:
        cli = scope.run([sys.executable,str(ROOT/'scripts/verify_gc10.py'),'--binary',str(binary),
            '--evidence',str(path),'--validate'],capture_output=True,text=True,timeout=60)
    finally:
        scope.finish()
    require(cli.returncode != 0 and expected in cli.stderr,'GC-10 copied repetitions not rejected by CLI')
    ci_error = None
    try:
        receipt = native_receipt(gate,1,folder,binary,{})
        check_receipts([receipt],[gate])
    except RuntimeError as error:
        ci_error = str(error)
    require(ci_error == expected,'GC-10 copied repetitions not rejected by CI consumer')
    result = dict(claimed_journeys=30,distinct_journeys=6,cli_exit_code=cli.returncode,
        cli_rejected=True,ci_rejected=True,reason=ci_error)
    (folder/'rejection.json').write_text(json.dumps(result,indent=2)+'\n')
    return result


def summary(report):
    return dict(case_id='GC-10',status=report['status'],receipts=len(report['results']),cleanup=report['cleanup'],
        semantic_score=None,release_ready=False,eligible_for_grading=False,frozen_blocks=16,
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])


def retain_report(path,report):
    raw = path.parent/'.gc10-native'/path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    data = (json.dumps(report,ensure_ascii=False)+'\n').encode(); raw.write_bytes(data)
    manifest = dict(kind='gc10_native_manifest',version=1,summary=summary(report),
        native=dict(path=str(raw.relative_to(path.parent)),sha256=sha(data)),native_bytes=len(data))
    path.write_text(json.dumps(manifest,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes'} and manifest['kind'] == 'gc10_native_manifest' and
        type(manifest['version']) is int and manifest['version'] == 1,'GC-10 manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'],'GC-10 native size mismatch')
    report = strict_json(raw); expected = summary(report)
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),'GC-10 summary drift')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--validate',action='store_true')
    parser.add_argument('--variant-jobs',type=int,choices=(1,2),default=2,
        help='Independent fixture workers; each variant retains five serial repetitions')
    args = parser.parse_args(); binary = args.binary.resolve()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),binary))); return
    args.evidence.unlink(missing_ok=True); args.evidence.parent.mkdir(parents=True,exist_ok=True)
    report = dict(case_id='GC-10',status='inconclusive',fixture_lock=load(),semantic_score=None,eligible_for_grading=False,
        release_ready=False,cleanup=False,results=[],identity=dict(source_before=source_identity(),binary_path=str(binary),
            binary_sha256=sha(binary.read_bytes()),environment=dict(platform=platform.platform(),python=platform.python_version(),
                node=subprocess.check_output(['node','--version'],text=True).strip(),go=subprocess.check_output(['go','version'],text=True).strip(),
                binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-10; original conversation inputs only; connected freshness/location/failure adapter still required before whole-case live grading',
            child_approval_unknown='Synchronous Main only; no Task/Child/Artifact or authorized external action. Significant cost stops at GC-13 handoff; unknown effects, cancel and old Task epochs are inapplicable.'))
    order = {(v['variant_id'],n):i*REPEATS+n for i,v in enumerate(variants(*inputs())) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    try:
        def series(variant):
            for repetition in range(1,REPEATS+1):
                failure = args.evidence.with_suffix('.'+variant['variant_id']+'.failure.json')
                row = drive(binary,variant['variant_id'],repetition,failure)
                with mutex:
                    report['results'].append(row)
                    print((row['variant_id'],repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
                require(all(row['assertions'].values()),'GC-10 assertion failure')
        run_cases(list(variants(*inputs())),series,args.variant_jobs)
        report['results'].sort(key=lambda r:order[r['variant_id'],r['repetition']])
        report['identity']['source_after'] = source_identity()
        report.update(status='mechanical_pass',cleanup=True)
        report['rejection_probes'] = rejection_probes(report['results'][0])
        result = validate_report(report,binary)
        report['repetition_rejection_probes'] = repetition_rejection_probes(report,binary,
            args.evidence.parent/'.gc10-native'/(args.evidence.stem+'-copied-repetitions'))
    except BaseException:
        report['status'] = 'inconclusive'
        raise
    finally:
        report['results'].sort(key=lambda r:order[r['variant_id'],r['repetition']])
        retain_report(args.evidence,report)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
