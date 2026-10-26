#!/usr/bin/env python3
"""GC-09 original agenda, connectors and batched Worker mapping; no grades."""
import argparse
import copy
from contextlib import closing
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import platform
import sqlite3
import subprocess
import sys
import tempfile

import agenda_case
import agenda_sandbox
from case_runner import load_contract, variants
from fixture_pool import run_cases
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_cases import drive as original_drive, timestamp
from unscripted_cases import source_identity
from verify_main import MainHarness, require
from verify_tasks import state as task_state, assertions as task_assertions

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT/'evals/fixtures/gc09-boundary-v1.json'
REPEATS = 5


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(FIXTURE.with_suffix('.lock.json').read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected,'GC-09 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    require(len(lock['frozen_blocks']) == len({b['line'] for b in lock['frozen_blocks']}) == 17 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in lock['frozen_blocks']),
        'GC-09 frozen source drift')
    return lock


def inputs(variant_id=None):
    data, _, _ = load_contract(ROOT)
    case = next(c for c in data['cases'] if c['case_id'] == 'GC-09')
    if variant_id is None:
        return data, case
    return case, next(v for v in variants(data,case) if v['variant_id'] == variant_id)


def supplement(variant):
    data, _ = inputs()
    index = next(v['wording'] for v in data['variants'] if v['id'] == variant['variant_id'])
    fixture = strict_json(FIXTURE.read_bytes())
    return fixture['delegate'][index],fixture['answers'][index]


class AgendaHarness(MainHarness):
    def args(self,*args):
        return [str(self.binary),'-config',str(self.root/'config.yaml'),'-test-bridge','agenda',*args]

    def history(self):
        # Read-only observation must not initialize every MCP connector. The
        # settled journey separately checks the actual CLI history against this
        # persisted view, including timestamps and the complete message payload.
        with closing(sqlite3.connect(self.root/'data/conversation.db')) as db:
            db.row_factory = sqlite3.Row
            rows = [dict(r) for r in db.execute('SELECT * FROM conversation_message ORDER BY sequence')]
        for row in rows:
            row['content'] = strict_json(row.pop('content_json'))
            row['source_event_id'] = row['source_event_id'] or ''
        return rows


def canonical_history(history):
    rows = copy.deepcopy(history)
    epoch = datetime(1970,1,1,tzinfo=timezone.utc)
    for row in rows:
        instant = datetime.fromisoformat(row['created_at'].replace('Z','+00:00'))
        require(instant.tzinfo is not None,'CLI history timestamp requires timezone')
        delta = instant.astimezone(timezone.utc)-epoch
        row['created_at'] = (delta.days*86400+delta.seconds)*1000000+delta.microseconds
    return rows


def native(h):
    value = task_state(h)
    with closing(sqlite3.connect(h.root/'data/state.db')) as db:
        db.row_factory = sqlite3.Row
        for key,table in (('approvals','approval'),('artifacts','artifact'),('automations','automation')):
            value[key] = [dict(r) for r in db.execute('SELECT * FROM '+table+' ORDER BY rowid')]
    return value


def remote(peer):
    return {key:agenda_sandbox.state(peer/key,key) for key in ('calendar','mail','work')}


def batched(h,key,text,row):
    row['receipt'] = h.send(key,text)
    release = h.root/(key+'.release')
    workers_release = h.root/(key+'.workers-release')
    barriers_seen = []
    def progress():
        # Read complete CLI/native evidence only at the two held boundaries.
        # Polling chat history starts three sandboxed MCP discovery processes;
        # repeated observation must not compete with the work being observed.
        if release.exists() and workers_release.exists():
            return
        if barriers_seen.count('before_worker_generation') == 2 and not workers_release.exists():
            row['concurrent_workers'] = native(h)
            workers_release.touch()
        if release.exists():
            return
        expected = 2 if key == 'delegate' else 4
        with closing(sqlite3.connect(h.root/'data/state.db')) as db:
            queued = db.execute('SELECT count(*) FROM main_inbox').fetchone()[0]
        if queued == expected:
            # Main is held outside a transaction. Roots run concurrently;
            # Reporter queues both results before Main becomes idle.
            row['before_release'] = native(h)
            release.touch()
    result = h.drive_boundaries({'after_delivery_commit':release,'before_worker_generation':workers_release},barriers_seen.append,progress)
    require(result.returncode == 0,'GC-09 batch drive failed: '+result.stderr)
    row['state'] = native(h)


def connected_drive(binary,folder,case,variant,row):
    peer = folder/'peer'; peer.mkdir()
    source = dict(calendar=case['world']['calendar'],mail=case['world']['emails'],work=case['world']['tasks'])
    source_bytes = (json.dumps(source,ensure_ascii=False)+'\n').encode()
    for key in source:
        directory = peer/key; directory.mkdir()
        (directory/'source.json').write_bytes(source_bytes)
        (directory/'agenda_sandbox.py').write_bytes((ROOT/'scripts/agenda_sandbox.py').read_bytes())
    python = subprocess.check_output(['xcrun','--find','python3'],text=True).strip() if sys.platform == 'darwin' else '/usr/bin/python3'
    config = dict(version=1,mcp=[dict(name=k,command=python,directory=str(peer/k),
        args=[str(peer/k/'agenda_sandbox.py'),str(peer/k),k],tools=dict(read='read_only',write='external_effect'))
        for k in source])
    (folder/'config.yaml').write_text(json.dumps(config))
    h = AgendaHarness(binary,folder)
    try:
        h.setup()
        h.send('context',variant['context'])
        event = variant['events'][0]
        row['initial_receipt'] = h.send('initial','当前时间：'+event['at']+'。'+event['text'])
        child = h.hold('after_delivery_commit'); child.kill(); child.communicate(timeout=10)
        row['restart'] = dict(boundary='after_delivery_commit',returncode=child.returncode,state=native(h))
        h.command('chat','work')
        row.update(initial=native(h),initial_remote=remote(peer))
        delegate,answer = supplement(variant)
        row['questions'] = {}; batched(h,'delegate',delegate,row['questions'])
        row['answers'] = {}; batched(h,'answers',answer,row['answers'])
        state = native(h)
        for key,text in (('initial','当前时间：'+event['at']+'。'+event['text']),('delegate',delegate),('answers',answer)):
            receipt = h.send(key,text)
            expected = row['initial_receipt'] if key == 'initial' else row['questions' if key == 'delegate' else 'answers']['receipt']
            require(receipt == expected,'duplicate ingress receipt drift')
        h.command('chat','work')
        row.update(state=state,replay_state=native(h),cli_history=h.command('chat','history'),
            remote=remote(peer),source_after={k:(peer/k/'source.json').read_text() for k in source})
        return row
    except BaseException:
        row['failure_state'] = h.failure_state()
        raise
    finally:
        h.cleanup()
        row['owned_processes_joined'] = all(c.poll() is not None for c in h.children)


def text(message):
    return ''.join(p.get('text','') for p in message['content'])


def observe(row):
    case,variant = inputs(row['variant_id'])
    original = row['original']; connected = row['connected']
    checks = agenda_case.observations(variant,original)
    checks.update({'steering/'+k:v for k,v in agenda_case.steering_observations(variant,original).items()})
    initial = connected['initial']; state = connected['state']
    checks.update({'task/'+k:v for k,v in task_assertions(state,2,4,4).items()})
    questions = connected['questions']; answers = connected['answers']
    task_by_id = {t['task_id']:t for t in state['tasks']}
    def key(t):
        return 'interview' if 'interview' in t['anchor_task'] else 'release'
    mapping = strict_json(FIXTURE.read_bytes())['mapping']
    roots = [r for r in state['runs'] if r['kind'] == 'worker']
    old = [r for r in roots if r['task_epoch'] == 1]
    fresh = [r for r in roots if r['task_epoch'] == 2]
    actions = [json.loads(t['arguments_json']) for t in state['tools'] if t['tool_name'] == 'builtin__task__manage']
    updates = {u['event_id']:u for u in state['updates']}
    def batch_checks(batch,count):
        before = batch['before_release']; after = batch['state']
        inbox = after['inbox'][count-2:count]
        mains = {i['run_id'] for i in inbox}
        followups = [g for g in after['generations'] if g['run_id'] in mains]
        expected = []
        for i in inbox:
            u = updates[i['event_id']]
            item = json.loads(u['content'])
            item['task_ref'] = task_by_id[u['task_id']]['task_ref']
            if count == 2: item.pop('question')
            expected.append(item)
        canonical = lambda items:sorted(json.dumps(i,sort_keys=True) for i in items)
        output = json.loads(json.loads(followups[-1]['result_json'])['text']) if followups else {}
        result = output.get('tasks' if count == 2 else 'results',[])
        return dict(
            concurrent_workers=len([r for r in batch['concurrent_workers']['runs'] if r['kind'] == 'worker' and
                r['status'] == 'running' and r['task_epoch'] == count//2]) == 2,
            both_queued_while_main_busy=len(before['updates']) == len(before['inbox']) == count and
                all(i['run_id'] is None for i in before['inbox'][count-2:count]) and
                any(r['kind'] == 'main' and r['status'] == 'running' for r in before['runs']),
            one_batched_main=len(inbox) == 2 and len(mains) == 1 and None not in mains and len(followups) == 1 and
                sum(r['kind'] == 'main' for r in after['runs']) == (3 if count == 2 else 5),
            one_question=output.get('question') == 'release 和 interview 各自可用的准备时段？' if count == 2 else 'question' not in output,
            mapped_questions_or_answers=canonical(result) == canonical(expected) and
                len({item['task_ref'] for item in result}) == 2 and
                all(item['key'] == key(task_by_id[updates[i['event_id']]['task_id']]) for item,i in zip(expected,inbox)),
            batch_delivered_once=len([m for m in after['conversation'] if m['role'] == 'assistant' and
                m['source_event_id'] in {g['generation_id'] for g in followups} and text(m) == json.loads(followups[-1]['result_json'])['text']]) == 1 if followups else False)
    for prefix,batch,count in (('questions',questions,2),('answers',answers,4)):
        checks.update({prefix+'/'+k:v for k,v in batch_checks(batch,count).items()})
    requests = [json.loads(g['request_json']) for g in initial['generations']]
    expected_users = [variant['context'],'当前时间：'+variant['events'][0]['at']+'。'+variant['events'][0]['text']]
    reads = initial['tools']
    expected_sources = dict(calendar=case['world']['calendar'],mail=case['world']['emails'],work=case['world']['tasks'])
    checkpoints = [initial,questions['before_release'],questions['state'],answers['before_release'],answers['state'],state]
    checks.update(
        connector_original_input=[text(m) for m in initial['conversation'] if m['role'] == 'user'] == expected_users and
            all([m['text'] for m in r['messages'] if m['role'] == 'user'] == expected_users for r in requests),
        initial_main_only=len(initial['runs']) == 1 and initial['runs'][0]['kind'] == 'main' and
            initial['runs'][0]['status'] == 'completed' and not initial['tasks'] and len(initial['deliveries']) == 1,
        actual_three_connector_reads=len(reads) == 3 and {t['tool_name'] for t in reads} == {'mcp__'+k+'__read' for k in expected_sources} and
            all(t['status'] == 'succeeded' and t['attempt_count'] == 1 and t['replay_policy'] == 'safe' and
                json.loads(t['arguments_json']) == {} and
                json.loads(json.loads(t['result_json'])['text']) == expected_sources[t['tool_name'].split('__')[1]] for t in reads),
        writable_connectors_available=all(any(t['name'] == 'mcp__'+k+'__write' for t in json.loads(c['tools_json']))
            for k in expected_sources for c in initial['capabilities'] if c['role'] == 'main'),
        no_unrequested_effect=all(not s[k] for s in checkpoints for k in ('approvals','artifacts','automations')) and
            all(t['tool_name'] in {'mcp__calendar__read','mcp__mail__read','mcp__work__read','builtin__task__manage','builtin__task__search'} and
                t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in state['tools']),
        remote_read_only=connected['remote'] == connected['initial_remote'] == {k:[dict(name='read',arguments={})] for k in expected_sources} and
            all(json.loads(v) == expected_sources for v in connected['source_after'].values()),
        actual_read_restart=connected['restart']['returncode'] == -9 and connected['restart']['boundary'] == 'after_delivery_commit' and
            bool(connected['restart']['state']['tools']) and all(t in initial['tools'] for t in connected['restart']['state']['tools'] if t['status'] == 'succeeded'),
        append_routes_to_original_task=len(actions) == 4 and [a['operation'] for a in actions] == ['create','create','append','append'] and
            {a['taskRef']:a['instruction'] for a in actions[2:]} == {t['task_ref']:mapping[key(t)] for t in state['tasks']} and
            len(state['instructions']) == 2 and all(i['instruction'] == mapping[key(task_by_id[i['task_id']])] for i in state['instructions']),
        new_roots_preserve_old=len(old) == len(fresh) == 2 and all(r['status'] == 'completed' for r in roots) and
            all(r in state['runs'] for r in questions['state']['runs'] if r['kind'] == 'worker') and
            all(t['status'] == 'active' and t['task_epoch'] == 2 for t in state['tasks']) and
            all(json.loads(u['content']).get('answer') == mapping[key(task_by_id[u['task_id']])] for u in state['updates'] if u['task_epoch'] == 2),
        original_and_supplemental_users=[text(m) for m in state['conversation'] if m['role'] == 'user'] == expected_users+list(supplement(variant)),
        completed_calls_reused=all(t in state['tools'] for t in initial['tools']) and all(g in state['generations'] for g in initial['generations']),
        complete_model_inventory=len(initial['generations']) == 2 and len(state['generations']) == 13 and
            all(len([g for g in state['generations'] if g['run_id'] == r['run_id']]) == 1 for r in roots),
        worker_result_update_matches_generation=len(state['worker_results']) == 4 and all(
            len([g for g in state['generations'] if g['generation_id'] == result['exchange_id'] and
                g['run_id'] == result['run_id'] and json.loads(g['result_json']).get('text') == result['content']]) == 1 and
            len([u for u in state['updates'] if u['root_run_id'] == result['run_id'] and u['content'] == result['content']]) == 1
            for result in state['worker_results']),
        completed_generation_and_delivery=canonical_history(connected['cli_history']) == state['conversation'] and
            all(g['status'] == 'completed' and g['attempt_count'] == 1 for g in state['generations']) and
            all(len([m for m in state['conversation'] if m['role'] == 'assistant' and any(m['source_event_id'] == g['generation_id'] and json.loads(g['result_json']).get('text') == text(m) and not json.loads(g['result_json']).get('tool_calls')
                    for g in state['generations'] if g['run_id'] == r['run_id']) and
                any(d['message_id'] == m['message_id'] and d['status'] == 'sent' for d in state['deliveries'])]) == 1
                for r in state['runs'] if r['kind'] == 'main'),
        answers_in_actual_worker_requests=all(any(mapping[key(task_by_id[r['task_id']])] in m['text']
                for m in json.loads(g['request_json'])['messages']) and
            all(mapping[other] not in m['text'] for other in mapping if other != key(task_by_id[r['task_id']])
                for m in json.loads(g['request_json'])['messages'])
            for r in fresh for g in state['generations'] if g['run_id'] == r['run_id']),
        connected_replay_equal=state == connected['replay_state'],
        joined_processes=connected['owned_processes_joined'] is True and original['cleanup']['owned_processes_joined'] is True)
    initial_text = text(next(m for m in initial['conversation'] if m['role'] == 'assistant'))
    checks.update({'connected_agenda/'+k:v for k,v in agenda_case.text_checks(initial_text).items()})
    return checks


def drive(binary,variant_id,repetition,failure_path):
    case,variant = inputs(variant_id)
    row = dict(variant_id=variant_id,repetition=repetition,started_at=timestamp())
    with tempfile.TemporaryDirectory(prefix='erii-gc09-') as tmp:
        folder = Path(tmp); original = folder/'original'; original.mkdir()
        connected = folder/'connected'; connected.mkdir()
        try:
            row['original'] = original_drive(binary,original,case,variant)
            row['connected'] = {}
            connected_drive(binary,connected,case,variant,row['connected'])
            row['assertions'] = observe(row)
        except BaseException as error:
            failure_path.write_text(json.dumps(dict(row,error=str(error)),ensure_ascii=False)+'\n')
            raise
    row['cleanup'] = dict(disposable_root_removed=not folder.exists(),owned_processes_joined=True)
    row['finished_at'] = timestamp()
    return row


def rejection_probes(row):
    require(all(observe(row).values()),'passing native receipt required for corruption probes')
    probes = {}
    for name in ('source','priority','steering','restart','remote','mapping','epoch','old_root','request','batch','replay','effect','result','missing_generation','worker_result','cli_history'):
        bad = copy.deepcopy(row); c = bad['connected']; state = c['state']
        if name == 'source': c['source_after']['mail'] = '{}'
        elif name == 'priority': bad['original']['state']['conversation'][-1]['content'][0]['text'] = 'E2 first'
        elif name == 'steering': bad['original']['steering']['restart']['returncode'] = 0
        elif name == 'restart': c['restart']['returncode'] = 0
        elif name == 'remote': c['remote']['mail'].append(dict(name='write',arguments={}))
        elif name == 'mapping': state['instructions'][0]['instruction'] = 'wrong task answer'
        elif name == 'epoch': state['tasks'][0]['task_epoch'] = 1
        elif name == 'old_root': next(r for r in state['runs'] if r['kind'] == 'worker')['status'] = 'running'
        elif name == 'request': c['initial']['generations'][0]['request_json'] = '{"messages":[]}'
        elif name == 'batch': c['questions']['state']['inbox'][0]['run_id'] = None
        elif name == 'replay': c['replay_state']['tools'][0]['attempt_count'] = 2
        elif name == 'result': c['questions']['state']['generations'][-1]['result_json'] = '{"text":"[]"}'
        elif name == 'missing_generation': state['generations'] = [g for g in state['generations'] if g['run_id'] != state['worker_results'][0]['run_id']]
        elif name == 'worker_result': state['worker_results'][0]['content'] = '{"key":"wrong"}'
        elif name == 'cli_history': c['cli_history'][-1]['content'][0]['text'] = 'forged visible history'
        else: state['tools'][0]['tool_name'] = 'mcp__mail__write'
        try:
            rejected = not all(observe(bad).values())
        except (ValueError,KeyError,TypeError,IndexError,AttributeError,RuntimeError):
            rejected = True
        require(rejected,'corruption accepted: '+name)
        probes[name] = True
    return probes


def validate_report(report,binary=None):
    lock = load()
    require(report.get('case_id') == 'GC-09' and report.get('status') == 'mechanical_pass' and
        report.get('fixture_lock') == lock and report.get('semantic_score','missing') is None and
        report.get('eligible_for_grading') is False and report.get('release_ready') is False and report.get('cleanup') is True,
        'complete ungraded GC-09 native report required')
    identity = report['identity']; binary = Path(binary or identity['binary_path'])
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and bool(identity['environment']),'GC-09 source/binary identity drift')
    expected = [(v['variant_id'],n) for v in variants(*inputs()) for n in range(1,REPEATS+1)]
    require([(r['variant_id'],r['repetition']) for r in report['results']] == expected and
        all(type(r['repetition']) is int for r in report['results']),'all six variants and five repetitions required')
    for row in report['results']:
        checks = observe(row)
        require(row['assertions'] == checks and all(type(v) is bool and v for v in checks.values()) and
            all(type(v) is bool for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and
            all(type(v) is bool for v in row['cleanup'].values()),'failed or forged GC-09 native receipt')
    require(report['rejection_probes'] == rejection_probes(report['results'][0]),'missing GC-09 corruption probes')
    return dict(case_id='GC-09',status='mechanical_pass',receipts=30,frozen_blocks=17,semantic_score=None,release_ready=False)


def summary(report):
    return dict(case_id='GC-09',status=report['status'],receipts=len(report['results']),cleanup=report['cleanup'],
        semantic_score=None,release_ready=False,eligible_for_grading=False,frozen_blocks=17,
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])


def retain_report(path,report):
    raw = path.parent/'.gc09-native'/path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    data = (json.dumps(report,ensure_ascii=False)+'\n').encode(); raw.write_bytes(data)
    manifest = dict(kind='gc09_native_manifest',version=1,summary=summary(report),
        native=dict(path=str(raw.relative_to(path.parent)),sha256=sha(data)),native_bytes=len(data))
    path.write_text(json.dumps(manifest,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes'} and manifest['kind'] == 'gc09_native_manifest' and
        type(manifest['version']) is int and manifest['version'] == 1,'GC-09 manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'],'GC-09 native size mismatch')
    report = strict_json(raw); expected = summary(report)
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),'GC-09 summary drift')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--validate',action='store_true')
    args = parser.parse_args(); binary = args.binary.resolve()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),binary))); return
    args.evidence.unlink(missing_ok=True); args.evidence.parent.mkdir(parents=True,exist_ok=True)
    report = dict(case_id='GC-09',status='inconclusive',fixture_lock=load(),semantic_score=None,eligible_for_grading=False,
        release_ready=False,cleanup=False,results=[],identity=dict(source_before=source_identity(),binary_path=str(binary),
            binary_sha256=sha(binary.read_bytes()),environment=dict(platform=platform.platform(),python=platform.python_version(),
                node=subprocess.check_output(['node','--version'],text=True).strip(),go=subprocess.check_output(['go','version'],text=True).strip(),
                binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='unscripted_cases.py freeze --case GC-09; original conversation inputs only; connector/concurrent Worker adapter incomplete',
            child_approval_unknown='No Child or authorized effect in this Case. Unrequested mail/calendar writes remain available but unused; unknown external effects inapplicable.'))
    try:
        def series(variant):
            for repetition in range(1,REPEATS+1):
                row = drive(binary,variant['variant_id'],repetition,args.evidence.with_suffix('.failure.json'))
                report['results'].append(row)
                print((row['variant_id'],repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
                require(all(row['assertions'].values()),'GC-09 assertion failure')
        run_cases(list(variants(*inputs())),series,1)
        report['identity']['source_after'] = source_identity()
        report.update(status='mechanical_pass',cleanup=True)
        report['rejection_probes'] = rejection_probes(report['results'][0])
        result = validate_report(report,binary)
    except BaseException:
        report['status'] = 'inconclusive'
        raise
    finally:
        retain_report(args.evidence,report)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
