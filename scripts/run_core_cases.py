#!/usr/bin/env python3
"""M5 exact datasets through the CLI; real workspaces, QuickJS and SIGKILL.

Five-run repetition belongs to verify_suite. Every run resets all six wording,
noise/order variants. Mechanical evidence never supplies semantic Judge grades.
"""
import argparse
from collections import Counter
import csv
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time

from case_runner import digest, load_contract, score, variants
from core_conformance import run as conformance
from core_fixtures import feedback, csv_text, CATEGORIES, SEVERITIES
from run_cases import timestamp
from verify_capabilities import tree
from verify_codemode import CodeHarness
from verify_code_recovery import read
from verify_main import require

ROOT = Path(__file__).resolve().parents[1]


class CoreHarness(CodeHarness):
    def args(self, *args):
        return [str(self.binary), '-config', str(self.root / 'config.yaml'), '-test-bridge', 'core', *args]


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
    state['Memory'] = 'not_applicable; real adapter due M6'
    return state


def owned_sidecar(process, script):
    rows = subprocess.check_output(['ps', '-axo', 'pid=,ppid=,command='], text=True)
    pids = [int(p[0]) for line in rows.splitlines() if len(p := line.split(None, 2)) == 3
            and int(p[1]) == process.pid and p[2].endswith(str(script))]
    require(len(pids) == 1, 'exact owned Sidecar not identified')
    return pids[0]


def feedback_case(binary, folder, variant):
    configure(folder)
    h = CoreHarness(binary, folder)
    try:
        h.setup()
        h.send('fixture', variant['context'] + '\n' + variant['events'][0]['text'])
        rows, gold = feedback()
        # Import the exact CSV and all five attachments through ArtifactStore.
        assets = [dict(name='feedback_100.csv', mediaType='text/csv', text=csv_text(rows))]
        assets.extend(dict(name=r['attachment_ref'], mediaType='text/plain',
                           text=(ROOT / 'evals/fixtures/core-v1' / r['attachment_ref']).read_text())
                      for r in rows if r['attachment_ref'])
        source = '''
const assets = ASSETS;
const inputs = [];
for (const asset of assets) { const saved = await tools.builtin__artifact__save(asset); inputs.push(...saved.artifactRefs); }
const rows = assets[0].text.trimEnd().split("\\n").slice(1).map(line => {
 const [row_id,text,attachment_ref] = line.split(","); return {row_id,text,attachment_ref};
});
async function classify(row, hold) {
 const result = await agent.run({task:"Classify feedback: " + JSON.stringify({...row, hold}), artifactRefs:inputs});
 return JSON.parse(result.text);
}
const first = await Promise.all(rows.slice(0,37).map(row=>classify(row,false)));
const active = await Promise.all(rows.slice(37,45).map(row=>classify(row,true)));
const rest = await Promise.all(rows.slice(45).map(row=>classify(row,false)));
const results = [...first,...active,...rest].map(row=>({...row,evidence:row.row_id}));
const columns = ["row_id","category","severity","evidence","attachment_ref"];
function quote(x) { return '"' + String(x).replace(/"/g,'""') + '"'; }
const csv = columns.join(",")+"\\n"+results.map(row=>columns.map(c=>quote(row[c])).join(",")).join("\\n")+"\\n";
const saved = await tools.builtin__artifact__save({name:"feedback_analysis.csv",mediaType:"text/csv",text:csv});
await tools.builtin__artifact__publish({artifactRefs:saved.artifactRefs});
return {count:results.length,artifactRefs:saved.artifactRefs};
'''.replace('ASSETS', json.dumps(assets, ensure_ascii=False)).replace('ROWS', json.dumps(rows, ensure_ascii=False))
        execute(h, 'feedback', source, work=False)
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
        sidecar = owned_sidecar(process, ROOT / 'runtime/javascript/src/sidecar.mjs')
        os.kill(sidecar, signal.SIGKILL)
        process.send_signal(signal.SIGKILL)
        process.communicate(timeout=15)
        require(process.returncode == -signal.SIGKILL, 'Go SIGKILL not observed')
        h.command('chat', 'work', timeout=180)
        pending = approve(h)
        require(len(pending) == 8, 'active calls did not retain 8 exact boundary approvals')
        h.command('chat', 'work', timeout=180)
        value = snapshot(h)
        calls = value['agent_invocation']
        invocations = {i['invocation_id']: i for i in value['tools']}
        children = {r['run_id']: r for r in value['runs'] if r['parent_run_id']}
        dispatches = Counter(g['generation_id'] for g in value['external_provider_dispatches'])
        drift = json.loads((Path(h.env['ERII_DATA_ROOT']) / 'f055-drift.json').read_text())
        artifacts = value['artifact']
        output = next(a for a in artifacts if a['name'] == 'feedback_analysis.csv')
        opened = subprocess.run(h.args('artifact', 'open', output['artifact_ref']), env=h.env, cwd=h.root,
                                capture_output=True, text=True, timeout=20)
        require(opened.returncode == 0, 'analysis Artifact not openable')
        classified = list(csv.DictReader(io.StringIO(opened.stdout)))
        answers = {r['row_id']: {k: r[k] for k in ('row_id', 'category', 'severity')} for r in classified}
        originals = {r['row_id']: r for r in rows}
        checks = dict(
            input_artifacts_immutable=all(any(a['name'] == asset['name'] and a['content_hash'] == hashlib.sha256(asset['text'].encode()).hexdigest() for a in artifacts) for asset in assets),
            exact_100_rows=len(classified) == 100 and set(answers) == set(originals),
            exact_row_classification=answers == {r['row_id']: r for r in gold},
            category_distribution=dict(Counter(r['category'] for r in classified)) == CATEGORIES,
            severity_distribution=dict(Counter(r['severity'] for r in classified)) == SEVERITIES,
            row_evidence=all(r['evidence'] == r['row_id'] and r['attachment_ref'] == originals[r['row_id']]['attachment_ref'] for r in classified),
            one_task_one_root=len(value['task']) == 1 and sum(r['worker_position'] == 'root' for r in read(db, 'SELECT worker_position FROM run')) == 1,
            hundred_children=len(children) == len(calls) == 100 and all(r['status'] == 'completed' for r in children.values()),
            accepted_identities_reattached=all(any(a['child_run_id'] == old['child_run_id'] and a['invocation_id'] == old['invocation_id'] for a in calls) for old in before),
            completed_calls_not_redispatched=all(dispatches[g] == 1 for g in done),
            all_generations_dispatched_once=all(n == 1 for n in dispatches.values()),
            f055_drift_rejected=drift['rejection'].get('error', {}).get('code') == 'NON_DETERMINISTIC_REPLAY',
            f055_original_restored=not drift['restored'].get('error') and drift['changed']['callKey'] == drift['original']['callKey'],
            eight_checkpoint_effects_once=len(value['external_effects']) == 8 and len({e['key'] for e in value['external_effects']}) == 8,
            only_original_f055_admitted=sum('F055' in invocations[a['invocation_id']]['arguments_json'] for a in calls) == 1,
            frozen_tree_model=len({r['model_snapshot_id'] for r in value['runs'] if r['kind'] == 'worker'}) == 1,
            child_only_direct_parent=all(invocations[a['invocation_id']]['run_id'] == children[a['child_run_id']]['parent_run_id'] for a in calls),
            one_completion_notice=len([i for i in read(db, 'SELECT * FROM task_update') if i['kind'] == 'completed']) == 1,
            bounded_questions=sum(json.dumps(m['content'], ensure_ascii=False).count('？') for m in value['conversation'] if m['role'] == 'assistant') <= 1,
            no_leaked_execution_claim=all(s['state'] == 'completed' and s['claim_token'] is None for s in value['code_execution_state']),
        )
        return dict(assertions=checks, state=value, fault=dict(signals=['Sidecar SIGKILL','Go SIGKILL'], before=before,
                    completed_provider_dispatch_counts={g:dispatches[g] for g in done}, drift=drift),
                    artifact=dict(ref=output['artifact_ref'], sha256=hashlib.sha256(opened.stdout.encode()).hexdigest(),text=opened.stdout),
                    inputs=assets, source=source)
    finally:
        h.cleanup()


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
        subprocess.run(['git', *args], cwd=repo, env=env, check=True, capture_output=True)
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
        ts = subprocess.run([shutil.which('node'), '--test', str(repo / 'sidecar/contract.test.ts')], cwd=repo,
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
        # Actual product cancellation, including a grandchild Code Mode call.
        child_plan = [{'name':'builtin__codemode__execute','arguments':{'source':'return await agent.run({task:'+json.dumps('Capabilities: '+json.dumps([{'name':'mcp__fixture__send','arguments':{'target':'held','text':'grandchild'}}]))+'});'}}]
        execute(h,'cancel-tree','return await agent.run({task:'+json.dumps('Capabilities: '+json.dumps(child_plan))+'});')
        task = read(db,"SELECT task_id,task_ref FROM task WHERE status='active'")[0]
        h.send('cancel', 'Capabilities: '+json.dumps([{'name':'builtin__task__manage','arguments':{'operation':'cancel','taskRef':task['task_ref']}}]))
        h.command('chat','work',timeout=180)
        cancelled = read(db,'SELECT run_id,parent_run_id,depth,status FROM run WHERE task_id=?',(task['task_id'],))
        h.command('chat','work',timeout=180)
        checks['recursive_product_cancel'] = len(cancelled) == 3 and max(r['depth'] for r in cancelled) == 2 and all(r['status']=='cancelled' for r in cancelled)
        checks['cancel_restart_no_revival'] = not read(db,"SELECT 1 FROM run WHERE task_id=? AND status!='cancelled'",(task['task_id'],))
        return dict(assertions=checks, state=value, source=source, workspace=dict(before=original,after=tree(repo),changed=changed),
                    fault=dict(signal='SIGKILL', migrated_invocations=migrated), cancelled_tree=cancelled,
                    sidecar_test=dict(returncode=ts.returncode,stdout=ts.stdout,stderr=ts.stderr))
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


def run(binary, selected=None):
    dataset, policy, lock = load_contract(ROOT, 'core-v1')
    report = dict(version=1, milestone='M5', provider='explicit deterministic core Bridge',
                  contract_lock=lock, dataset_digest=digest(dataset), binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  results=[], cleanup=False, full_case_status='not_applicable; future adapters and semantic evaluation unexecuted')
    with tempfile.TemporaryDirectory(prefix='erii-core-cases-') as temp:
        for case in dataset['cases']:
            if selected and selected != case['case_id']:
                continue
            for variant in variants(dataset, case):
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
                report['results'].append(row)
                print(f"{case['case_id']} {variant['variant_id']}: {row['status']} ({row['wall_time_ms']}ms)",flush=True)
                require(all(assertions.values()),'core assertions failed: '+json.dumps(assertions))
        report['cleanup']=True
    require(not Path(temp).exists(), 'disposable root leaked')
    if not selected:
        report['conformance'] = conformance(ROOT)
        require(report['conformance']['passed'], 'core conformance failed')
    report['summary']=dict(fragment_passes=sum(r['status']=='fragment_pass' for r in report['results']),
                           fragments=len(report['results']),hard_failures=sum(r['hard_failure'] for r in report['results']),
                           semantic_scores='unexecuted; no Judge', **{'pass@1':None,'pass^5':None})
    return report


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--case',choices=['GC-17','GC-19','GC-20'])
    args=parser.parse_args();args.evidence.unlink(missing_ok=True)
    result=run(args.binary.resolve(),args.case)
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    args.evidence.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(result['summary']))


if __name__=='__main__':
    main()
