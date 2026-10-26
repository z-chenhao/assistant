#!/usr/bin/env python3
"""GC-16 exact corpus journey through CLI, parallel Child, MCP and Artifact.

Explicit scripted model only. No semantic grade, internet search or L2 claim.
Launch/Doctor/Cleanup use MainHarness; native receipts retain failures.
"""
import argparse
import copy
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile

from research_sandbox import state as remote_state
from verify_main import MainHarness, require
from verify_children import state as child_state, check as child_check
from verify_suite import source_state
from verify_tasks import assertions as task_assertions

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / 'evals/fixtures/gc16-v1.json'
INPUT = '找出最早公开提出 Anchor Replay 的项目，并确认后来有没有改变定义。只用一手来源。'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def variants():
    source = json.loads(FIXTURE.read_text())
    rows = [dict(id='base', source=copy.deepcopy(source), text=INPUT, seed=1601),
            dict(id='wording-1', source=copy.deepcopy(source), seed=1602,
                 text='Anchor Replay 最早是哪家公开提出的？后续定义变过没有？请只以一手材料作证。'),
            dict(id='wording-2', source=copy.deepcopy(source), seed=1603,
                 text='请追溯 Anchor Replay 的首个公开项目，以及后来版本的定义变化；关键结论仅据一手来源。')]
    for index, name in enumerate(('document-order', 'noise-1', 'noise-2'), 1604):
        data = copy.deepcopy(source)
        if name == 'document-order':
            data['documents'].reverse()
        else:
            noise = dict(id='N1', type='independent_article', date='2023-01-01', title='Atlas geography' if name == 'noise-1' else 'Aster gardening',
                         claim='unrelated to source-code replay', url='https://noise.example.test/note', links=[])
            data['documents'].append(noise)
            data['search']['Anchor Replay'].insert(0 if name == 'noise-1' else 1, 'N1')
        rows.append(dict(id=name, source=data, text=INPUT, seed=index))
    for row in rows:
        row['fixture_sha256']=sha(json.dumps(row['source'],ensure_ascii=False,sort_keys=True,separators=(',',':')).encode())
    return rows


class ResearchHarness(MainHarness):
    def args(self, *args):
        return [str(self.binary), '-config', str(self.root/'config.yaml'), '-test-bridge', 'research', *args]


def native(h):
    value = child_state(h)
    with sqlite3.connect(h.root/'data/state.db') as db:
        db.row_factory = sqlite3.Row
        value['run_times'] = [dict(r) for r in db.execute('SELECT run_id,created_at,updated_at FROM run ORDER BY created_at')]
        value['automation'] = [dict(r) for r in db.execute('SELECT * FROM automation')]
    return value


def report_observe(text, source):
    docs = {d['id']: d for d in source['documents']}
    sections = ('## 事实', '## 推断', '## 未发现证据', '## 后续核查')
    facts = text.split('## 事实', 1)[-1].split('## 推断', 1)[0]
    return dict(
        independent_report=text.startswith('# Anchor Replay') and all(s in text for s in sections) and
            '本地合成语料' in text and '语料外' in text and '后续核查' in text,
        primary_facts=all(docs[i]['date'] in facts and docs[i]['claim'] in facts and
            f"[{i}: {docs[i]['title']}]({docs[i]['url']})" in facts for i in ('D1','D3','D4','D5')),
        earliest_primary='Atlas 最早可作为证据的一手来源日期为 2024-03-01' in facts and '2024-02-10' not in facts,
        rename_vs_extension='改名为 Durable Replay，语义不变' in facts and '属于扩展，不能倒推初版已有' in facts,
        exclude_similar_entity='排除 Aster Step Cache' in facts and '与 Atlas 不等价' in facts,
        lead_is_not_evidence='D2（2024-02-10，二手猜测）只作发现线索' in text and
            docs['D2']['url'] not in text and '未发现公开证据支持早于 2024-03-01' in text)


def observe(row):
    value = row['state']; source = row['variant']['source']
    nodes = {r['run_id']: r for r in value['runs']}
    roots = [r for r in nodes.values() if r['kind']=='worker' and r['parent_run_id'] is None]
    children = [r for r in nodes.values() if r['parent_run_id']]
    calls = [t for t in value['tools'] if t['tool_name']=='builtin__agent__run']
    sources = [t for t in value['tools'] if t['tool_name'].startswith('mcp__research__')]
    expected = [dict(operation='search',key=q) for q in ('Anchor Replay','Aster Step Cache')]
    expected += [dict(operation='open',key=i) for i in ('D2','D1','D3','D4','D5')]
    if any(d['id']=='N1' for d in source['documents']): expected.append(dict(operation='open',key='N1'))
    canonical = lambda rows: sorted(json.dumps(r,sort_keys=True) for r in rows)
    documents = {d['id']: d for d in source['documents']}
    transported = []
    for t in sources:
        operation = t['tool_name'].removeprefix('mcp__research__')
        key = json.loads(t['arguments_json'])['key']
        actual = json.loads(json.loads(t['result_json'])['text'])
        want = documents.get(key) if operation=='open' else dict(hits=[dict(id=i,title=documents[i]['title'],url=documents[i]['url']) for i in source['search'][key]])
        transported.append(actual == want)
    artifacts = row['artifacts']; times = {r['run_id']:r for r in value['run_times']}
    checks = task_assertions(value,1,3,1)
    del checks['root_count_and_no_child']
    try:
        child_check(value,children=2)
        checks['child_contracts'] = True
    except RuntimeError:
        checks['child_contracts'] = False
    checks.update(
        original_user=[p['text'] for m in value['conversation'] if m['role']=='user' for p in m['content']]==[row['variant']['text']],
        one_tree=len(value['tasks'])==len(roots)==1 and len(children)==2 and all(
            c['parent_run_id']==roots[0]['run_id'] and c['task_id']==roots[0]['task_id'] and c['depth']==1 for c in children),
        parallel_paths=len(calls)==2 and len({t['provider_exchange_id'] for t in calls})==1 and
            len(children)==2 and max(times[c['run_id']]['created_at'] for c in children)<min(times[c['run_id']]['updated_at'] for c in children),
        child_inputs_minimal=all(set(json.loads(t['arguments_json']))=={'task'} for t in calls) and
            all('D1' not in t['arguments_json'] and '2024-03-01' not in t['arguments_json'] for t in calls),
        isolated_child_context=all(not any('Task anchor:' in m['text'] or 'You are Erii.' in m['text'] for m in json.loads(g['request_json'])['messages'])
            for g in value['generations'] if nodes[g['run_id']]['parent_run_id']),
        parent_only_results=len(calls)==2 and all(set(json.loads(json.loads(t['result_json'])['text']))=={'text','artifactRefs'} for t in calls) and
            len(value['updates'])==len(value['inbox'])==1 and len(value['deliveries'])==2 and all(m['source']=='main' for m in value['conversation'] if m['role']=='assistant'),
        corpus_exact=bool(transported) and all(transported) and canonical(row['remote'])==canonical(expected) and
            canonical([dict(operation=t['tool_name'].removeprefix('mcp__research__'),key=json.loads(t['arguments_json'])['key']) for t in sources])==canonical(expected),
        child_reads_only=all(nodes[t['run_id']]['parent_run_id'] for t in sources),
        two_child_final_sources=len(calls)==2 and {d['id'] for t in calls for d in json.loads(json.loads(json.loads(t['result_json'])['text'])['text'])}==set(documents),
        completed=all(r['status']=='completed' for r in nodes.values()) and value['tasks'][0]['status']=='completed' and
            all(t['status']=='succeeded' and t['attempt_count']==1 for t in value['tools']),
        no_external_actions=not value['approvals'] and not value['automation'] and all(t['tool_name'] in {
            'builtin__task__manage','builtin__agent__run','mcp__research__search','mcp__research__open','builtin__artifact__manage'} for t in value['tools']),
        frozen_tree=bool(children) and all(c['model_snapshot_id']==roots[0]['model_snapshot_id'] for c in children),
        artifact_reopened=len(artifacts)==len(value['artifacts'])==1 and all(a['sha256']==sha(a['text'].encode())==stored['content_hash'] and
            a['ref']==stored['artifact_ref'] and stored['run_id']==roots[0]['run_id'] for a,stored in zip(artifacts,value['artifacts'])),
        artifact_delivered=len(value['publications'])==2 and len({p['artifact_ref'] for p in value['publications']})==1 and
            any('artifact_ref' in json.dumps(m['content']) for m in value['conversation'] if m['role']=='assistant'),
        actual_two_restarts=len(row['restarts'])==2 and all(r['returncode']==-9 for r in row['restarts']) and
            [r['boundary'] for r in row['restarts']]==['after_child_accept_commit','after_report_commit'],
        # Admission hooks run concurrently: the peer can commit before SIGKILL.
        # Either legal cut must recover to exactly two Children without reads.
        admission_then_committed_report=len(row['restarts'])==2 and 1<=len(row['restarts'][0]['before']['child_links'])<=2 and
            all(r['status']=='queued' for r in row['restarts'][0]['before']['runs'] if r['parent_run_id']) and
            row['restarts'][0]['remote']==[] and len(row['restarts'][1]['before']['updates'])==1 and
            canonical(row['restarts'][1]['remote'])==canonical(expected),
        completed_calls_reused=all(next(t for t in value['tools'] if t['invocation_id']==old['invocation_id'])==old
            for checkpoint in row['restarts'] for old in checkpoint['before']['tools'] if old['status']=='succeeded'),
        completed_generations_reused=all(next(g for g in value['generations'] if g['generation_id']==old['generation_id'])==old
            for checkpoint in row['restarts'] for old in checkpoint['before']['generations'] if old['status']=='completed'),
        replay_equal=value==row['replay_state'] and row['remote']==row['remote_replay'],
        leases_released=all(c['token'] is None for c in value['claims']))
    checks.update(report_observe(artifacts[0]['text'] if artifacts else '', source))
    return checks


def drive(binary, evidence, variant):
    row = dict(variant=variant, scope='scripted_whole_case_mechanics', semantic_score=None, restarts=[],
               started_at=datetime.now(timezone.utc).isoformat())
    failure = None
    with tempfile.TemporaryDirectory(prefix='erii-gc16-') as temp:
        root=Path(temp); peer=root/'provider'; peer.mkdir()
        (peer/'research_sandbox.py').write_bytes((ROOT/'scripts/research_sandbox.py').read_bytes())
        (peer/'source.json').write_text(json.dumps(variant['source'], ensure_ascii=False))
        python = subprocess.run(['xcrun','--find','python3'],text=True,capture_output=True,check=True).stdout.strip() if sys.platform=='darwin' else '/usr/bin/python3'
        (root/'config.yaml').write_text('version: 1\nmcp:\n  - name: research\n    command: '+json.dumps(python)+
            '\n    directory: '+json.dumps(str(peer))+'\n    args: ['+json.dumps(str(peer/'research_sandbox.py'))+', '+json.dumps(str(peer))+']\n    tools: {search: read_only, open: read_only}\n')
        h=ResearchHarness(binary,root)
        try:
            h.setup(); h.send('gc16',variant['text'])
            for boundary in ('after_child_accept_commit','after_report_commit'):
                child=h.hold(boundary,timeout=20); child.kill(); child.communicate(timeout=10)
                row['restarts'].append(dict(boundary=boundary,returncode=child.returncode,before=native(h),remote=remote_state(peer)))
            h.command('chat','work'); value=native(h)
            child_check(value,children=2)
            artifacts=[]
            for a in value['artifacts']:
                opened=h.command('artifact','open',a['artifact_ref'],ok=False)
                require(opened.returncode==0,'research Artifact CLI open failed')
                artifacts.append(dict(ref=a['artifact_ref'],text=opened.stdout,sha256=sha(opened.stdout.encode())))
            remote=remote_state(peer)
            h.send('gc16',variant['text']); h.command('chat','work')
            row.update(state=value,replay_state=native(h),remote=remote,remote_replay=remote_state(peer),artifacts=artifacts)
            row['assertions']=observe(row)
            require(all(row['assertions'].values()),'GC-16 failed: '+json.dumps(row['assertions']))
        except BaseException as error:
            row.update(failure=str(error),failure_state=h.failure_state(),remote_failure=remote_state(peer))
            failure=error
        finally:
            h.cleanup()
            row['owned_processes_joined']=True
            (evidence/(variant['id']+'.json')).write_text(json.dumps(row,ensure_ascii=False,indent=2)+'\n')
    row['cleanup']=dict(owned_processes_joined=True,disposable_root_removed=not root.exists())
    row['ended_at']=datetime.now(timezone.utc).isoformat()
    (evidence/(variant['id']+'.json')).write_text(json.dumps(row,ensure_ascii=False,indent=2)+'\n')
    if failure: raise failure
    return row


def main():
    p=argparse.ArgumentParser(); p.add_argument('--binary',type=Path,required=True); p.add_argument('--evidence',type=Path,required=True)
    p.add_argument('--variant',choices=[v['id'] for v in variants()]); args=p.parse_args()
    args.evidence.unlink(missing_ok=True); folder=args.evidence.with_suffix(''); folder.mkdir(parents=True,exist_ok=True)
    before=dict(**source_state(),binary_sha256=sha(args.binary.read_bytes()))
    lock=json.loads(FIXTURE.with_suffix('.lock.json').read_text())
    require(lock==dict(fixture_sha256=sha(FIXTURE.read_bytes()),golden_sha256=sha((ROOT/'evals/golden-cases.md').read_bytes())),'GC-16 fixture lock drift')
    rows=[drive(args.binary.resolve(),folder,v) for v in variants() if not args.variant or args.variant==v['id']]
    after=dict(**source_state(),binary_sha256=sha(args.binary.read_bytes()))
    require(before==after,'source or binary changed during GC-16')
    report=dict(case_id='GC-16',status='mechanical_pass',scope='scripted_whole_case_mechanics',semantic_score=None,fixture_lock=lock,
                source_provenance=dict(before=before,after=after),variants=rows,cleanup=all(all(r['cleanup'].values()) for r in rows))
    args.evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(dict(status=report['status'],variants=len(rows),cleanup=report['cleanup'])))


if __name__=='__main__': main()
