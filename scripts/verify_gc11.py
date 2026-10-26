#!/usr/bin/env python3
"""GC-11 frozen quote journey, committed recovery and recomputable native receipts."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import platform
import sqlite3
import subprocess
import sys
import tempfile
import threading

from case_runner import digest, load_contract, variants
from fixture_pool import Scope, run_cases
import quote_case
from release_audit import inventory
from release_receipts import Receipts, strict_json
from run_cases import timestamp
from unscripted_cases import source_identity
from verify_capabilities import CapabilityHarness, content, prepare
from verify_main import require

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT/'evals/fixtures/gc11-boundary-v1.lock.json'
REPEATS = 5


def sha(data):
    return hashlib.sha256(data).hexdigest()


def inputs():
    dataset, _, _ = load_contract(ROOT, 'gc11-v1')
    return dataset, dataset['cases'][0]


def load():
    lock = strict_json(FIXTURE.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected, 'GC-11 lock drift: '+name)
    current = {(b['line'],b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 23 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'],b['sha256']) in current for b in blocks),
        'GC-11 frozen source drift')
    return lock


class QuoteHarness(CapabilityHarness):
    def state(self):
        value = super().state()
        with sqlite3.connect(Path(self.env['ERII_DATA_ROOT'])/'state.db') as db:
            db.row_factory = sqlite3.Row
            for name in ('approval','task','artifact','artifact_reference','artifact_publication','automation','run_claim','event_outbox'):
                value[name] = [dict(r) for r in db.execute('SELECT * FROM '+name)]
        return value

    def command(self, *args, **kwargs):
        if args == ('chat','work') and not getattr(self,'recovery_driven',False):
            self.recovery_driven = True
            self.restarts = []
            for boundary in ('after_tool_commit','after_delivery_commit'):
                child = self.hold(boundary)
                child.kill(); child.communicate(timeout=10)
                self.restarts.append(dict(boundary=boundary,returncode=child.returncode,state=self.state()))
        return super().command(*args, **kwargs)


def drive(binary, variant_id, repetition, failure_path):
    variant = next(v for v in quote_case.case_variants() if v['variant_id'] == variant_id)
    row = dict(variant_id=variant_id,repetition=repetition,started_at=timestamp())
    with tempfile.TemporaryDirectory(prefix='erii-gc11-') as temp:
        root = Path(temp)
        # Reuse the original fixture/configuration; no separate Tool pipeline.
        prepared = prepare(binary,root)
        # Preparation finishes all setup commands. The observer uses those same
        # ready stores/configuration and owns the subsequent recovery processes.
        prepared.cleanup()
        h = QuoteHarness(binary,root,fixture='capabilities')
        h.env = prepared.env
        try:
            row['journey'] = quote_case.drive(h,variant,content)
            row['restarts'] = h.restarts
            row['source_after'] = strict_json((root/'provider/mcp-data.json').read_bytes())['insurance_quotes']
            row['effect_file_exists'] = (root/'provider/mcp-effects.jsonl').exists()
            row['drift_exit'] = h.command('chat','send','quote-turn-1',text='changed parameters',ok=False).returncode
            h.command('chat','work')
            row['after_drift'] = h.state()
        except BaseException as error:
            row.update(error=str(error),failure_state=h.failure_state())
            failure_path.write_text(json.dumps(row,ensure_ascii=False)+'\n')
            raise
        finally:
            h.cleanup()
            joined = all(c.poll() is not None for c in h.children)
    row.update(finished_at=timestamp(),cleanup=dict(owned_processes_joined=joined,disposable_root_removed=not root.exists()))
    row['assertions'] = observe(row)
    return row


def observe(row):
    variant = next(v for v in quote_case.case_variants() if v['variant_id'] == row['variant_id'])
    source = inputs()[1]['world']; texts = quote_case.answers(source)
    journey = row['journey']; state = journey['state']; tools = state['tools']
    runs = state['runs']; requests = [strict_json(g['request_json']) for g in state['generations']]
    checks = quote_case.observations(state,variant['events'],texts,journey['artifacts'],journey['replay_state'],row['effect_file_exists'],source)
    users = [''.join(p.get('text','') for p in m['content']) for m in state['conversation'] if m['role'] == 'user']
    replies = [m for m in state['conversation'] if m['role'] == 'assistant']
    stored = {a['artifact_ref']:a for a in state['artifact']}
    caps = {c['snapshot_id']:c for c in state['capabilities']}
    run_by_id = {r['run_id']:r for r in runs}
    permissions = {k:{t['name']:t['permission'] for t in strict_json(c['tools_json'])} for k,c in caps.items()}
    cuts = row['restarts']
    checks.update(
        original_inputs=journey['variant'] == variant and journey['source'] == source == row['source_after'] and
            users == [quote_case.input_text(variant,i,t) for i,t in enumerate(texts)],
        ungraded=journey['grading']['status'] == 'ungraded' and journey['grading']['final_score'] is None,
        exact_tool_parameters=len(tools) == 6 and all(
            [dict(name=t['tool_name'],arguments=strict_json(t['arguments_json'])) for t in tools[i*3:i*3+3]] ==
            [dict(name='mcp__fixture__read',arguments=dict(text='insurance_quotes')),
             dict(name='builtin__artifact__manage',arguments=dict(operation='save',name=f'quote-comparison-{i+1}.md',mediaType='text/markdown',text=text)),
             dict(name='mcp__fixture__echo',arguments=dict(text=text))] for i,text in enumerate(texts)),
        actual_tool_outputs=all(strict_json(tools[i*3+2]['result_json']) == dict(text=text) and
            strict_json(tools[i*3+1]['result_json'])['artifact_refs'] == [journey['artifacts'][i]['ref']] for i,text in enumerate(texts)),
        artifact_native_identity=len(stored) == 2 and len(journey['artifacts']) == 2 and len(state['artifact_reference']) == 2 and
            {a['ref'] for a in journey['artifacts']} == set(stored) and all(
                a['sha256'] == stored[a['ref']]['content_hash'] == sha(a['text'].encode()) and
                len(a['text'].encode()) == stored[a['ref']]['byte_size'] and stored[a['ref']]['run_id'] == runs[i]['run_id'] and
                any(ref['artifact_ref'] == a['ref'] and ref['holder_kind'] == 'tool' and ref['holder_id'] == tools[i*3+1]['invocation_id'] for ref in state['artifact_reference'])
                for i,a in enumerate(journey['artifacts'])),
        no_task_child_automation=not state['task'] and not state['automation'] and all(
            r['task_id'] is None and r['parent_run_id'] is None and r['depth'] == 0 for r in runs),
        actual_permissions=all(permissions[run_by_id[t['run_id']]['capability_snapshot_id']][t['tool_name']] in ('read_only','artifact_write') and
            t['replay_policy'] == 'safe' for t in tools) and not state['approval'],
        frozen_snapshots=bool(requests) and bool(caps) and all(c['role'] == 'main' and not c['can_spawn_agents'] for c in caps.values()) and all(
            req['model_snapshot']['snapshot_id'] == run_by_id[g['run_id']]['model_snapshot_id']
            for g,req in zip(state['generations'],requests)) and all(
            len({digest(dict(model=req['model_snapshot'],tools=req['tools'])) for g,req in zip(state['generations'],requests) if g['run_id'] == r['run_id']}) == 1 for r in runs),
        actual_model_inputs=all([m['text'] for m in req['messages'] if m['role'] == 'user'][-1] == users[next(i for i,r in enumerate(runs) if r['run_id'] == g['run_id'])] for g,req in zip(state['generations'],requests)),
        complete_generations=len(state['generations']) == 8 and len(state['usage']) == 8 and all(g['status'] == 'completed' and g['attempt_count'] == 1 for g in state['generations']),
        reply_delivery_identity={d['message_id'] for d in state['deliveries']} == {m['message_id'] for m in replies},
        ordered_messages=[m['sequence'] for m in state['conversation']] == list(range(1,5)),
        actual_crashes=[c['boundary'] for c in cuts] == ['after_tool_commit','after_delivery_commit'] and all(type(c['returncode']) is int and c['returncode'] == -9 for c in cuts),
        committed_cuts=len(cuts) == 2 and len(cuts[0]['state']['tools']) == 1 and cuts[0]['state']['tools'][0]['status'] == 'succeeded' and
            len(cuts[1]['state']['tools']) == 3 and len(cuts[1]['state']['deliveries']) == 1 and cuts[1]['state']['deliveries'][0]['status'] == 'pending',
        committed_calls_reused=all(t in tools for c in cuts for t in c['state']['tools'] if t['status'] == 'succeeded'),
        committed_generations_reused=all(g in state['generations'] for c in cuts for g in c['state']['generations'] if g['status'] == 'completed'),
        duplicate_and_drift=journey['replay_state_digest'] == digest(state) and state == row['after_drift'] and type(row['drift_exit']) is int and row['drift_exit'] != 0,
        claims_released=bool(state['run_claim']) and all(c['token'] is None for c in state['run_claim']))
    return checks


def rejection_probes(row):
    require(all(observe(row).values()),'passing native GC-11 receipt required')
    mutations = dict(
        input=lambda r:r['journey']['variant']['events'][0].update(text='replaced'),
        source=lambda r:r['source_after']['quotes'][1].update(cashback_written=True),
        tool_args=lambda r:r['journey']['state']['tools'][0].update(arguments_json='{}'),
        tool_result=lambda r:r['journey']['state']['tools'][0].update(result_json='{"text":"{}"}'),
        echo_result=lambda r:r['journey']['state']['tools'][2].update(result_json='{}'),
        save_result=lambda r:r['journey']['state']['tools'][1].update(result_json='{"artifact_refs":[]}'),
        artifact=lambda r:r['journey']['state']['artifact'][0].update(content_hash='0'*64),
        artifact_owner=lambda r:r['journey']['state']['artifact_reference'][0].update(holder_id='forged'),
        readback=lambda r:r['journey']['artifacts'][0].update(text='forged'),
        delivery=lambda r:r['journey']['state']['deliveries'].pop(),
        reply=lambda r:r['journey']['state']['conversation'][-1]['content'][0].update(text='purchased'),
        restart=lambda r:r['restarts'][0].update(returncode=0),
        restart_cut=lambda r:r['restarts'][0]['state']['tools'].clear(),
        replay=lambda r:r['journey']['replay_state']['tools'][0].update(attempt_count=2),
        tool_attempt=lambda r:r['journey']['state']['tools'][0].update(attempt_count=2),
        effect=lambda r:r.update(effect_file_exists=True),
        approval=lambda r:r['journey']['state']['approval'].append({'status':'pending'}),
        task=lambda r:r['journey']['state']['task'].append({'status':'active'}),
        generation=lambda r:r['journey']['state']['generations'].pop(),
        snapshot=lambda r:r['journey']['state']['runs'][0].update(model_snapshot_id='forged'),
        lease=lambda r:r['journey']['state']['run_claim'][0].update(token='owned'),
        graded=lambda r:r['journey']['grading'].update(final_score=100))
    probes = {}
    for name,mutate in mutations.items():
        bad = copy.deepcopy(row)
        try:
            mutate(bad); rejected = not all(observe(bad).values())
        except (ValueError,KeyError,IndexError,TypeError,RuntimeError):
            rejected = True
        require(rejected,'GC-11 corruption accepted: '+name)
        probes[name] = True
    return probes


def independent_executions(rows):
    for table,key in (('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),
                      ('deliveries','delivery_id'),('conversation','message_id'),('artifact','artifact_ref')):
        seen = set()
        for row in rows:
            ids = [item[key] for item in row['journey']['state'][table]]
            require(bool(ids) and all(type(value) is str and value for value in ids),'invalid GC-11 native execution identity: '+table+'/'+key)
            require(len(set(ids)) == len(ids) and seen.isdisjoint(ids),'reused GC-11 native execution identity: '+table+'/'+key)
            seen.update(ids)


def validate_report(report,binary=None):
    lock = load()
    require(report.get('case_id') == 'GC-11' and report.get('kind') == 'gc11_boundary' and report.get('status') == 'mechanical_pass' and
        report.get('fixture_lock') == lock and report.get('semantic_score','missing') is None and
        report.get('eligible_for_grading') is False and report.get('release_ready') is False and report.get('cleanup') is True,
        'complete ungraded GC-11 native report required')
    identity = report['identity']; binary = Path(binary or identity['binary_path'])
    require(identity['source_before'] == identity['source_after'] == source_identity() and identity['binary_sha256'] == sha(binary.read_bytes()) and
        bool(identity['environment']),'GC-11 source/binary identity drift')
    expected = [(v['variant_id'],n) for v in quote_case.case_variants() for n in range(1,REPEATS+1)]
    require([(r['variant_id'],r['repetition']) for r in report['results']] == expected and all(type(r['repetition']) is int for r in report['results']),
        'all six GC-11 variants and five repetitions required')
    independent_executions(report['results'])
    for row in report['results']:
        checks = observe(row)
        require(row['assertions'] == checks and all(v is True for v in checks.values()) and all(v is True for v in row['assertions'].values()) and
            row['cleanup'] == dict(disposable_root_removed=True,owned_processes_joined=True) and all(v is True for v in row['cleanup'].values()),
            'failed or forged GC-11 native receipt')
    require(report['rejection_probes'] == rejection_probes(report['results'][0]),'missing GC-11 corruption probes')
    return dict(case_id='GC-11',status='mechanical_pass',receipts=30,frozen_blocks=23,semantic_score=None,release_ready=False)


def repetition_rejection_probes(report,binary,folder):
    """Copy real passing journeys, reseal bytes, and exercise both consumers."""
    from ci_communication import native_receipt, check_receipts
    from verify_suite import SHARDS, report_name
    gate = next(g for g in SHARDS['communication'] if g.name == 'gc11')
    folder.mkdir(parents=True,exist_ok=True)
    forged = copy.deepcopy(report)
    for offset in range(0,len(forged['results']),REPEATS):
        for index in range(1,REPEATS):
            row = copy.deepcopy(forged['results'][offset])
            row['repetition'] = index+1
            forged['results'][offset+index] = row
    path = folder/report_name(gate,1)
    retain_report(path,forged)
    expected = 'reused GC-11 native execution identity: runs/run_id'
    scope = Scope()
    try:
        cli = scope.run([sys.executable,str(ROOT/'scripts/verify_gc11.py'),'--binary',str(binary),
            '--evidence',str(path),'--validate'],capture_output=True,text=True,timeout=60)
    finally:
        scope.finish()
    require(cli.returncode != 0 and expected in cli.stderr,'GC-11 copied repetitions not rejected by CLI')
    ci_error = None
    try:
        receipt = native_receipt(gate,1,folder,binary,{})
        check_receipts([receipt],[gate])
    except RuntimeError as error:
        ci_error = str(error)
    require(ci_error == expected,'GC-11 copied repetitions not rejected by CI consumer')
    result = dict(claimed_journeys=30,distinct_journeys=6,cli_exit_code=cli.returncode,
        cli_rejected=True,ci_rejected=True,reason=ci_error)
    (folder/'rejection.json').write_text(json.dumps(result,indent=2)+'\n')
    return result


def summary(report):
    return dict(case_id='GC-11',status=report['status'],receipts=len(report['results']),cleanup=report['cleanup'],
        semantic_score=None,release_ready=False,eligible_for_grading=False,frozen_blocks=23,
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])


def retain_report(path,report):
    raw = path.parent/'.gc11-native'/path.name
    raw.parent.mkdir(parents=True,exist_ok=True)
    data = (json.dumps(report,ensure_ascii=False)+'\n').encode(); raw.write_bytes(data)
    manifest = dict(kind='gc11_native_manifest',version=1,summary=summary(report),
        native=dict(path=str(raw.relative_to(path.parent)),sha256=sha(data)),native_bytes=len(data))
    path.write_text(json.dumps(manifest,indent=2)+'\n')
    return manifest


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes'} and manifest['kind'] == 'gc11_native_manifest' and
        type(manifest['version']) is int and manifest['version'] == 1,'GC-11 manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'],'GC-11 native size mismatch')
    report = strict_json(raw); expected = summary(report)
    require(manifest['summary'] == expected and all(type(manifest['summary'][k]) is type(v) for k,v in expected.items()),'GC-11 summary drift')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--validate',action='store_true')
    parser.add_argument('--variant-jobs',type=int,choices=(1,2),default=2)
    args = parser.parse_args(); binary = args.binary.resolve()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),binary))); return
    args.evidence.unlink(missing_ok=True); args.evidence.parent.mkdir(parents=True,exist_ok=True)
    report = dict(case_id='GC-11',kind='gc11_boundary',status='inconclusive',fixture_lock=load(),semantic_score=None,eligible_for_grading=False,
        release_ready=False,cleanup=False,results=[],identity=dict(source_before=source_identity(),binary_path=str(binary),
            binary_sha256=sha(binary.read_bytes()),environment=dict(platform=platform.platform(),python=platform.python_version(),
                node=subprocess.check_output(['node','--version'],text=True).strip(),go=subprocess.check_output(['go','version'],text=True).strip(),
                binary_build=subprocess.check_output(['go','version','-m',str(binary)],text=True))),
        remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',release_ready=False,
            unscripted='gc11_driver.py --trial; approved isolated profile and independent semantic grading required',
            task_child_approval_unknown='Two short synchronous Main turns. No authorized purchase, Task or Child. Buying transfers to GC-13; Task cancel/old epoch and unknown external effects are inapplicable here.'))
    order = {(v['variant_id'],n):i*REPEATS+n for i,v in enumerate(quote_case.case_variants()) for n in range(1,REPEATS+1)}
    mutex = threading.Lock()
    try:
        def series(variant):
            for repetition in range(1,REPEATS+1):
                row = drive(binary,variant['variant_id'],repetition,args.evidence.with_suffix('.'+variant['variant_id']+'.failure.json'))
                with mutex:
                    report['results'].append(row)
                    print((row['variant_id'],repetition),{k:v for k,v in row['assertions'].items() if not v},flush=True)
                require(all(row['assertions'].values()),'GC-11 assertion failure')
        run_cases(quote_case.case_variants(),series,args.variant_jobs)
        report['results'].sort(key=lambda r:order[r['variant_id'],r['repetition']])
        report['identity']['source_after'] = source_identity()
        report.update(status='mechanical_pass',cleanup=True)
        report['rejection_probes'] = rejection_probes(report['results'][0])
        result = validate_report(report,binary)
        report['repetition_rejection_probes'] = repetition_rejection_probes(report,binary,
            args.evidence.parent/'.gc11-native'/(args.evidence.stem+'-copied-repetitions'))
    except BaseException:
        report['status'] = 'inconclusive'
        raise
    finally:
        report['results'].sort(key=lambda r:order[r['variant_id'],r['repetition']])
        retain_report(args.evidence,report)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
