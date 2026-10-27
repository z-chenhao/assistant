#!/usr/bin/env python3
"""M5 exact datasets through the CLI; real workspaces, QuickJS and SIGKILL.

Five-run repetition belongs to verify_suite. Every run resets all six wording,
noise/order variants. Mechanical evidence never supplies semantic Judge grades.
"""
import argparse
from collections import Counter
from contextlib import closing
import hashlib
import gzip
import json
import os
from pathlib import Path
import shutil
import signal
import sqlite3
import subprocess
import sys
import tempfile
import time

from case_runner import digest, load_contract, score, variants
from core_conformance import run as conformance
from run_cases import timestamp
from verify_capabilities import tree
from verify_codemode import CodeHarness
from verify_code_recovery import read
from verify_main import require
from fixture_pool import run_cases, run as fixture_run

ROOT = Path(__file__).resolve().parents[1]


class CoreHarness(CodeHarness):
    def args(self, *args):
        return [str(self.binary), '-config', str(self.root / 'config.yaml'), '-test-bridge', 'core', *args]

    def setup(self):
        # Exact CSV/attachments appear in the current instruction and accepted
        # Tool arguments. Declare capacity for that complete exchange instead
        # of bypassing production compaction or truncating the locked dataset.
        return super().setup(metadata=dict(name='exact Core fixture', provider_kind='openai_compatible',
                                           endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions',
                                           model_id='fixture-model', context_window=32768, max_output_tokens=1024,
                                           generation_defaults={}, compatibility=dict(tool_result_format='openai'),
                                           capabilities=dict(text_input=True)))


def configure(folder, workspace=None):
    node = shutil.which('node')
    script = ROOT / 'runtime/javascript/src/sidecar.mjs'
    require(node and (script.parents[1] / 'node_modules/quickjs-wasi/quickjs.wasm').is_file(),
            'doctor: pinned Node/QuickJS required')
    text = 'version: 1\njavascript:\n  command: ' + json.dumps(node) + '\n  script: ' + json.dumps(str(script)) + '\n'
    if workspace:
        text += 'workspace:\n  root: ' + json.dumps(str(workspace)) + '\n  mode: edit\n'
    (folder / 'config.yaml').write_text(text)


def execute(h, key, source, work=True):
    plan = [{'name': 'builtin__codemode__execute', 'arguments': {'source': source}}]
    h.send(key, 'Delegate capabilities: ' + json.dumps(plan, ensure_ascii=False))
    if work:
        h.command('chat', 'work', timeout=180)


def approve(h):
    pending = [a for a in h.command('approval', 'list') if a['status'] == 'pending']
    for a in pending:
        h.command('approval', 'approve', a['approval_id'], a['frozen_arguments_hash'])
    return pending


def snapshot(h):
    state = h.state()
    path = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
    for table in ('task', 'task_instruction', 'agent_invocation', 'approval', 'code_call',
                  'code_execution', 'code_execution_state', 'code_manifest', 'code_delivery',
                  'code_store', 'code_store_call', 'code_event', 'event_outbox', 'artifact'):
        state[table] = read(path, 'SELECT * FROM ' + table)
    audit = Path(h.env['ERII_DATA_ROOT']) / 'fake-provider-calls.jsonl'
    state['external_provider_dispatches'] = [json.loads(line) for line in audit.read_text().splitlines()]
    effects = Path(h.env['ERII_DATA_ROOT']) / 'effect-ledger.db'
    state['external_effects'] = read(effects, 'SELECT * FROM effects') if effects.exists() else []
    state['Memory'] = dict(scope='Core fixture does not enable Curator; M6 journey gate owns Memory behavior',
                           records=read(Path(h.env['ERII_DATA_ROOT'])/'memory.db','SELECT * FROM memory_record'))
    return state


def owned_sidecar(process, script):
    rows = subprocess.check_output(['ps', '-axo', 'pid=,ppid=,command='], text=True)
    pids = [int(p[0]) for line in rows.splitlines() if len(p := line.split(None, 2)) == 3
            and int(p[1]) == process.pid and p[2].endswith(str(script))]
    require(len(pids) == 1, 'exact owned Sidecar not identified')
    return pids[0]


def feedback_state(h):
    """Read only bounded fixture state before cleanup, without starting a CLI."""
    result = {}
    database = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
    if not database.exists():
        return dict(database='not created')
    queries = {
        'child_status_counts': 'SELECT r.status,count(*) AS count FROM agent_invocation a JOIN run r ON r.run_id=a.child_run_id GROUP BY r.status',
        'run_status_counts': 'SELECT kind,status,wait_reason,count(*) AS count FROM run GROUP BY kind,status,wait_reason',
        'generation_status_counts': 'SELECT status,error_code,count(*) AS count FROM model_generation GROUP BY status,error_code',
        'tool_status_counts': 'SELECT tool_name,status,count(*) AS count FROM tool_invocation GROUP BY tool_name,status',
        'executions': 'SELECT state,desired_state,execution_epoch,claim_expires_at FROM code_execution_state LIMIT 4',
        'recent_trace': 'SELECT event_json,occurred_at FROM run_trace ORDER BY occurred_at DESC LIMIT 8',
    }
    try:
        with closing(sqlite3.connect(database.as_uri() + '?mode=ro', uri=True, timeout=.01)) as db:
            deadline = time.monotonic() + .1
            db.set_progress_handler(lambda: time.monotonic() >= deadline, 100)
            db.row_factory = sqlite3.Row
            for name, query in queries.items():
                result[name] = [dict(row) for row in db.execute(query)]
    except sqlite3.Error as error:
        result['capture_error_type'] = type(error).__name__
    return result


def feedback_case(binary, folder, variant):
    configure(folder)
    h = CoreHarness(binary, folder)
    try:
        h.setup()
        from gc17 import assets as feedback_assets, prompt, source_for, native, observe
        assets = feedback_assets()
        source = source_for(assets)
        h.send('fixture', prompt(variant, assets))
        release = folder / 'release'
        process = h.hold('code_37_completed_8_active', ('-test-main-release', str(release)))
        db = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
        before = read(db, 'SELECT c.call_id,c.invocation_id,a.child_run_id,r.status FROM code_call c JOIN agent_invocation a USING(invocation_id) JOIN run r ON r.run_id=a.child_run_id')
        require(len(before) == 45 and sum(r['status'] == 'completed' for r in before) == 37 and sum(r['status'] in ('queued','running','waiting') for r in before) == 8,
                'fault checkpoint must be exactly 37 committed and 8 active')
        completed = {r['child_run_id'] for r in before if r['status'] == 'completed'}
        generations = read(db, "SELECT generation_id,run_id FROM model_generation WHERE status='completed'")
        done = {g['generation_id'] for g in generations if g['run_id'] in completed}
        require(len(done) == 37, '37 completed feedback calls not observed')
        checkpoint = native(h)
        sidecar = owned_sidecar(process, ROOT / 'runtime/javascript/src/sidecar.mjs')
        os.kill(sidecar, signal.SIGKILL)
        process.send_signal(signal.SIGKILL)
        process.communicate(timeout=15)
        require(process.returncode == -signal.SIGKILL, 'Go SIGKILL not observed')
        h.command('chat', 'work', timeout=180)
        pending = approve(h)
        require(len(pending) == 8, 'active calls did not retain 8 exact boundary approvals')
        h.command('chat', 'work', timeout=180)
        value = native(h)
        sidecar_exit = subprocess.run(['ps','-p',str(sidecar),'-o','stat='],capture_output=True,text=True,timeout=5)
        dispatches = Counter(g['generation_id'] for g in value['external_provider_dispatches'])
        drift = json.loads((Path(h.env['ERII_DATA_ROOT']) / 'f055-drift.json').read_text())
        artifacts = value['artifact']
        output = next(a for a in artifacts if a['name'] == 'feedback_analysis.csv')
        opened = fixture_run(h.args('artifact', 'open', output['artifact_ref']), env=h.env, cwd=h.root,
                                capture_output=True, text=True, timeout=20)
        require(opened.returncode == 0, 'analysis Artifact not openable')
        row = dict(state=value, fault=dict(signals=['Sidecar SIGKILL','Go SIGKILL'],
                    returncodes=[process.returncode],sidecar=dict(pid=sidecar,parent_pid=process.pid,signal_sent=signal.SIGKILL,
                        ps_returncode=sidecar_exit.returncode,ps_stdout=sidecar_exit.stdout,ps_stderr=sidecar_exit.stderr),
                    before=before,checkpoint=checkpoint,
                    completed_provider_dispatch_counts={g:dispatches[g] for g in done}, drift=drift),
                    artifact=dict(ref=output['artifact_ref'],sha256=hashlib.sha256(opened.stdout.encode()).hexdigest(),text=opened.stdout),
                    inputs=assets,source=source,variant=variant)
        h.send('fixture',prompt(variant,assets))
        h.command('chat','work',timeout=180)
        row['replay_state'] = native(h)
        row['assertions'] = observe(row)
        return row
    except BaseException as error:
        failure = dict(fixture=folder.name, error_type=type(error).__name__, error=str(error)[:2000],
                       hold=h.hold_observation, runtime=feedback_state(h))
        print('Core fixture failure: ' + json.dumps(failure), file=sys.stderr, flush=True)
        destination = os.environ.get('ERII_CORE_DIAGNOSTICS')
        if destination:
            try:
                output = Path(destination) / ('failure-' + folder.name + '.json')
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_text(json.dumps(failure, indent=2) + '\n')
                if 'row' in locals():
                    output.with_suffix('.native.json.gz').write_bytes(gzip.compress(
                        json.dumps(row,ensure_ascii=False,separators=(',',':')).encode(),compresslevel=1,mtime=0))
            except OSError as capture_error:
                print('Core failure evidence: ' + type(capture_error).__name__, file=sys.stderr, flush=True)
        raise
    finally:
        h.cleanup()
        if 'row' in locals() and isinstance(row,dict) and 'state' in row:
            row['owned_processes_joined'] = all(c.poll() is not None for c in h.children)


MIGRATION = '''package migration
import ("encoding/json";"os")
func rewrite(path, from, to string) error {
 b,err:=os.ReadFile(path);if err!=nil{return err}
 var state map[string]json.RawMessage
 if err=json.Unmarshal(b,&state);err!=nil{return err}
 if state[from]==nil { if state[to]!=nil{return nil};state[to]=json.RawMessage(`[]`) } else { state[to]=state[from];delete(state,from) }
 b,err=json.Marshal(state);if err!=nil{return err}
 temp:=path+".migrating"
 if err=os.WriteFile(temp,b,0600);err!=nil{return err}
 return os.Rename(temp,path)
}
func Up(path string) error {return rewrite(path,"worker_job","run_tree")}
func Down(path string) error {return rewrite(path,"run_tree","worker_job")}
'''


def engineering_case(binary, folder, variant):
    repo = folder / 'repo'
    shutil.copytree(ROOT / 'evals/fixtures/engineering-v1', repo)
    # Known fixture repository, no hooks or user configuration.
    env = dict(PATH='/usr/bin:/bin', HOME='/nonexistent', GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null')
    for args in [('init','-q','-b','feature/eval'), ('add','.'), ('-c','user.name=Fixture','-c','user.email=fixture@example.test','commit','-qm','fixture')]:
        fixture_run(['git', *args], cwd=repo, env=env, check=True, capture_output=True)
    configure(folder, repo)
    h = CoreHarness(binary, folder)
    try:
        h.setup(); h.send('fixture', variant['context'] + '\n' + variant['events'][0]['text'])
        original = tree(repo)
        scheduler = (repo / 'internal/run/scheduler.go').read_text().replace('return Run{}', 'return Run{Task:task, Parent:parent, Epoch:epoch}')
        service = (repo / 'internal/run/service.go').read_text()
        fixed = service.replace('tree[i].Cancelled = true', 'if !tree[i].Cancelled { tree[i].Cancelled = true; Cancel(tree,tree[i].ID) }')
        source = '''
const rules=await tools.builtin__workspace__read({path:"AGENTS.md"});
async function edit(path,text) {
 const doc=await tools.builtin__workspace__read({path});
 return await tools.builtin__workspace__edit({path,before_hash:doc.hash,rules_hash:rules.hash,text});
}
const red=await tools.builtin__workspace__test({});
store("red",red);
await edit("internal/migration/schema.go",MIGRATION);
store("fixture_stage","migration_complete");
await edit("internal/run/scheduler.go",SCHEDULER);
const review=await agent.run({task:"Review cancellation"});
store("review",review.text);
if (!review.text.includes("grandchild")) throw new Error("review missed defect");
await edit("internal/run/service.go",FIXED);
const green=await tools.builtin__workspace__test({});
store("green",green);
if (!green.passed) throw new Error("engineering checks failed");
return {review:review.text,passed:green.passed};
'''.replace('MIGRATION',json.dumps(MIGRATION)).replace('SCHEDULER',json.dumps(scheduler)).replace('FIXED',json.dumps(fixed))
        execute(h, 'engineering', source)
        require(len(approve(h)) == 1, 'migration must require exact edit approval')
        release = folder / 'release'
        process = h.hold('code_migration_completed_scheduler_pending', ('-test-main-release', str(release)), timeout=180)
        db = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
        migrated = read(db, "SELECT * FROM tool_invocation WHERE tool_name='builtin__workspace__edit' AND status='succeeded'")
        require(len(migrated) == 1 and json.loads(migrated[0]['arguments_json'])['path'] == 'internal/migration/schema.go', 'kill not between migration and scheduler')
        migration_hash = hashlib.sha256((repo / 'internal/migration/schema.go').read_bytes()).hexdigest()
        require((repo / 'internal/run/scheduler.go').read_text() != scheduler, 'scheduler already completed at crash')
        process.send_signal(signal.SIGKILL); process.communicate(timeout=15)
        require(process.returncode == -signal.SIGKILL, 'engineering Go not SIGKILLed')
        h.command('chat', 'work', timeout=180)
        require(len(approve(h)) == 1, 'scheduler frozen approval missing')
        h.command('chat', 'work', timeout=180)
        require(len(approve(h)) == 1, 'Root repair exact approval missing')
        h.command('chat', 'work', timeout=180)
        value = snapshot(h)
        store = {r['key']:json.loads(r['value_json']) for r in value['code_store']}
        edits = [i for i in value['tools'] if i['tool_name'] == 'builtin__workspace__edit']
        changed = sorted(p for p in original if tree(repo).get(p) != original[p])
        ts = fixture_run([shutil.which('node'), '--test', str(repo / 'sidecar/contract.test.ts')], cwd=repo,
                            capture_output=True, text=True, timeout=30)
        checks = dict(red_reproduced=not store['red']['passed'] and 'late result from grandchild' in store['red']['output'],
                      fixture_tests_green=store['green']['passed'] and store['green']['command'] == 'go test -race ./...',
                      sidecar_tests=ts.returncode == 0,
                      independent_child_review='grandchild' in store['review'] and 'BLOCKED' in store['review'] and len(value['agent_invocation']) == 1,
                      migration_reused=len([e for e in edits if json.loads(e['arguments_json'])['path'] == 'internal/migration/schema.go']) == 1 and all(e['attempt_count'] == 1 for e in edits),
                      migration_bytes_preserved=hashlib.sha256((repo / 'internal/migration/schema.go').read_bytes()).hexdigest() == migration_hash,
                      only_root_writes=all(next(r for r in value['runs'] if r['run_id'] == e['run_id'])['parent_run_id'] is None for e in edits),
                      one_task=len(value['task']) == 1,
                      no_compatibility_path=changed == ['internal/migration/schema.go','internal/run/scheduler.go','internal/run/service.go'] and 'worker_job' not in (repo / 'internal/run/service.go').read_text())
        # Auxiliary cancellation conformance owns a separate DB/Agent so its
        # Task cannot contaminate the normative one-Task engineering snapshot.
        probe = cancellation_probe(binary, folder / 'cancel-probe')
        checks.update(probe['assertions'])
        value = snapshot(h)
        checks['one_task_final'] = len(value['task']) == 1
        return dict(assertions=checks, state=value, source=source, workspace=dict(before=original,after=tree(repo),changed=changed),
                    fault=dict(signal='SIGKILL', migrated_invocations=migrated), cancellation_probe=probe,
                    sidecar_test=dict(returncode=ts.returncode,stdout=ts.stdout,stderr=ts.stderr))
    finally:
        h.cleanup()


def cancellation_probe(binary, folder):
    folder.mkdir()
    configure(folder)
    h = CoreHarness(binary, folder)
    try:
        h.setup()
        db = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
        child_plan = [{'name':'builtin__codemode__execute','arguments':{'source':'return await agent.run({task:'+json.dumps('Capabilities: '+json.dumps([{'name':'mcp__fixture__send','arguments':{'target':'held','text':'grandchild'}}]))+'});'}}]
        execute(h,'cancel-tree','return await agent.run({task:'+json.dumps('Capabilities: '+json.dumps(child_plan))+'});')
        task = read(db,"SELECT task_id,task_ref FROM task WHERE status='active'")[0]
        h.send('cancel', 'Capabilities: '+json.dumps([{'name':'builtin__task__manage','arguments':{'operation':'cancel','taskRef':task['task_ref']}}]))
        h.command('chat','work',timeout=180)
        cancelled = read(db,'SELECT run_id,parent_run_id,depth,status FROM run WHERE task_id=?',(task['task_id'],))
        h.command('chat','work',timeout=180)
        value = snapshot(h)
        checks = dict(recursive_product_cancel=len(cancelled) == 3 and max(r['depth'] for r in cancelled) == 2 and all(r['status']=='cancelled' for r in cancelled),
                      cancel_restart_no_revival=not read(db,"SELECT 1 FROM run WHERE task_id=? AND status!='cancelled'",(task['task_id'],)),
                      cancellation_probe_one_task=len(value['task']) == 1)
        require(all(checks.values()), 'isolated cancellation conformance failed')
        return dict(scope='separate isolated SC-05 instance; not a GC-20 business Task', assertions=checks, state=value, cancelled_tree=cancelled)
    finally:
        h.cleanup()


def coding_case(binary, folder, variant):
    # Reuse maintained launch/doctor/drive mechanics rather than another shell.
    from verify_capabilities import run
    context = variant['context'] + '\n' + variant['events'][0]['text']
    observed = run(binary, 'audit', context)
    edited = run(binary, 'coding', context)
    return dict(assertions=dict(audit_unchanged=bool(observed['checks']), dirty_workspace_edit=bool(edited['checks']),
                                cleanup=observed['cleanup'] and edited['cleanup']), audit=observed, edit=edited,
                input_events=variant['events'], context=variant['context'])


def run_case_groups(dataset, selected, action, scenario_jobs):
    rows = []
    for case in dataset['cases']:
        if selected and selected != case['case_id']:
            continue
        # Keep bounded checkpoint work clear of compiler-heavy fixtures.
        # The original fifteen-second hold deadline is never extended.
        capacity = min(scenario_jobs, 2) if case['case_id'] == 'GC-17' else scenario_jobs
        items = [(case, variant) for variant in variants(dataset, case)]
        rows.extend(run_cases(items, action, capacity))
    return rows


def run(binary, selected=None, scenario_jobs=1, contracts_only=False):
    require(not (selected and contracts_only), 'case and contracts-only are mutually exclusive')
    dataset, policy, lock = load_contract(ROOT, 'core-v1')
    from unscripted_cases import source_identity
    from verify_gc11 import environment
    identity = dict(source_before=source_identity(),binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                    environment=environment(binary)) if not contracts_only and selected in (None,'GC-17') else None
    report = dict(version=1, milestone='M5', provider='explicit deterministic core Bridge',
                  contract_lock=lock, dataset_digest=digest(dataset), binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  results=[], cleanup=False, scenario_jobs=scenario_jobs, full_case_status='not_applicable; future adapters and semantic evaluation unexecuted')
    with tempfile.TemporaryDirectory(prefix='erii-core-cases-') as temp:
        def variant_case(item):
            case, variant = item
            folder=Path(temp)/(case['case_id']+'-'+variant['variant_id']); folder.mkdir()
            started, tick=timestamp(),time.monotonic()
            evidence={'GC-17':feedback_case,'GC-19':coding_case,'GC-20':engineering_case}[case['case_id']](binary,folder,variant)
            assertions=evidence['assertions']
            grading=score(policy,assertions,[])
            row=dict(case_id=case['case_id'],case_version=1,variant_id=variant['variant_id'],seed=case['seed'],
                     fixture_digest=digest({'variant':variant, 'locked_files':lock['files']}),input_events=variant['events'],clock=variant['clock'],
                     started_at=started,finished_at=timestamp(),wall_time_ms=round((time.monotonic()-tick)*1000),
                     assertions=assertions,status='fragment_pass' if all(assertions.values()) else 'fragment_fail',
                     hard_failure=not all(assertions.values()),final_score=grading['final_score'],
                     semantic_grading=grading['semantic_grading'],dimensions={d:None for d in policy['weights']},evidence=evidence)
            print(f"{case['case_id']} {variant['variant_id']}: {row['status']} ({row['wall_time_ms']}ms)",flush=True)
            if not all(assertions.values()) and case['case_id']=='GC-17':
                failed = Path(os.environ['ERII_CORE_DIAGNOSTICS'])/(variant['variant_id']+'.native.json.gz')
                failed.parent.mkdir(parents=True,exist_ok=True)
                failed.write_bytes(gzip.compress(json.dumps(evidence,ensure_ascii=False,separators=(',',':')).encode(),compresslevel=1,mtime=0))
            require(all(assertions.values()),'core assertions failed: '+json.dumps(assertions))
            return row
        if not contracts_only:
            report['results'] = run_case_groups(dataset, selected, variant_case, scenario_jobs)
        report['cleanup']=True
    require(not Path(temp).exists(), 'disposable root leaked')
    if identity:
        from gc17 import pack, load, validate_report
        identity['source_after'] = source_identity()
        for result in report['results']:
            if result['case_id']=='GC-17':
                native = result['evidence']
                native.update(identity=identity,boundary_lock=load(),semantic_score=None,release_ready=False,
                    scope='scripted_whole_case_mechanics',eligible_for_grading=False,
                    cleanup=dict(owned_processes_joined=native['owned_processes_joined'],disposable_root_removed=not Path(temp).exists()))
                result['evidence'] = pack(native)
        try:
            validate_report(report,binary)
        except BaseException:
            destination=os.environ.get('ERII_CORE_DIAGNOSTICS')
            if destination:
                failed=Path(destination)/'complete-native-failure.json.gz'
                failed.parent.mkdir(parents=True,exist_ok=True)
                failed.write_bytes(gzip.compress(json.dumps(report,ensure_ascii=False,separators=(',',':')).encode(),compresslevel=1,mtime=0))
            raise
    if not selected:
        report['conformance'] = conformance(ROOT)
        require(report['conformance']['passed'], 'core conformance failed')
        from ci_cancel import run as cancel_probe
        report['ci_cancellation'] = cancel_probe(binary)
    report['summary']=dict(fragment_passes=sum(r['status']=='fragment_pass' for r in report['results']),
                           fragments=len(report['results']),hard_failures=sum(r['hard_failure'] for r in report['results']),
                           semantic_scores='unexecuted; no Judge', **{'pass@1':None,'pass^5':None})
    return report


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    selection=parser.add_mutually_exclusive_group()
    selection.add_argument('--case',choices=['GC-17','GC-19','GC-20'])
    selection.add_argument('--contracts-only',action='store_true')
    parser.add_argument('--validate',action='store_true',help='Recompute all six GC-17 native receipts without running a model')
    parser.add_argument("--scenario-jobs",type=int,choices=[1,2,4],default=int(os.environ.get("ERII_VERIFY_CASE_JOBS","1")))
    args=parser.parse_args()
    if args.validate:
        from gc17 import validate_report
        print(json.dumps(validate_report(json.loads(args.evidence.read_text()),args.binary.resolve())))
        return
    args.evidence.unlink(missing_ok=True)
    os.environ.setdefault('ERII_CORE_DIAGNOSTICS',str(args.evidence.parent/('.'+args.evidence.stem+'-failures')))
    result=run(args.binary.resolve(),args.case,args.scenario_jobs,args.contracts_only)
    if any(r['case_id']=='GC-17' for r in result['results']):
        from gc17 import consumer_probes
        result['gc17_consumer_probes']=consumer_probes(result,args.binary.resolve(),
            args.evidence.parent/('.'+args.evidence.stem+'-consumer-probes'))
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    args.evidence.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(result['summary']))


if __name__=='__main__':
    main()
