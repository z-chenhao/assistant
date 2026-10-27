#!/usr/bin/env python3
"""GC-16 exact corpus journey through CLI, parallel Child, MCP and Artifact.

Explicit scripted model only. No semantic grade, internet search or L2 claim.
Launch/Doctor/Cleanup use MainHarness; native receipts retain failures.
"""
import argparse
import copy
from datetime import datetime, timezone
import hashlib
import gzip
import io
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile

from release_audit import inventory
from release_receipts import Receipts, strict_json
from unscripted_cases import source_identity
from verify_gc03 import exact
from verify_gc11 import environment
from verify_gc14 import frozen_generations
from research_sandbox import state as remote_state
from verify_main import MainHarness, require
from verify_children import state as child_state, check as child_check
from verify_suite import source_state
from verify_tasks import assertions as task_assertions

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / 'evals/fixtures/gc16-v1.json'
BOUNDARY_LOCK = ROOT / 'evals/fixtures/gc16-boundary-v1.lock.json'
INPUT = '找出最早公开提出 Anchor Replay 的项目，并确认后来有没有改变定义。只用一手来源。'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(BOUNDARY_LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected, 'GC-16 lock drift: '+name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 16 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current for b in blocks),
        'GC-16 frozen block drift')
    return lock


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


def bindings(value, documents, opened):
    """Join provider output, accepted Tool, Child result and reopened report."""
    generations = {g['generation_id']:g for g in value['generations']}
    tools = value['tools']
    for tool in tools:
        generation = generations[tool['provider_exchange_id']]
        emitted = strict_json(generation['result_json'])['tool_calls']
        matches = [c for c in emitted if c['id'] == tool['provider_call_id']]
        if (generation['run_id'] != tool['run_id'] or len(matches) != 1 or
                matches[0]['name'] != tool['tool_name'] or
                not exact(strict_json(matches[0]['arguments']),strict_json(tool['arguments_json']))):
            return False
    if sum(len(strict_json(g['result_json'])['tool_calls']) for g in generations.values()) != len(tools):
        return False
    links = {a['invocation_id']:a['child_run_id'] for a in value['child_links']}
    calls = [t for t in tools if t['tool_name'] == 'builtin__agent__run']
    for call in calls:
        child = links[call['invocation_id']]
        results = [w for w in value['worker_results'] if w['run_id'] == child]
        envelope = strict_json(strict_json(call['result_json'])['text'])
        if len(results) != 1 or not exact(envelope,dict(text=results[0]['content'],artifactRefs=[])):
            return False
        result = results[0]; generation = generations[result['exchange_id']]
        reads = [strict_json(strict_json(t['result_json'])['text']) for t in tools
                 if t['run_id'] == child and t['tool_name'] == 'mcp__research__open']
        if (generation['run_id'] != child or strict_json(generation['result_json'])['text'] != result['content'] or
                not exact(strict_json(result['content']),reads) or
                any(d != documents[d['id']] for d in reads)):
            return False
    saves = [t for t in tools if t['tool_name'] == 'builtin__artifact__manage' and
             strict_json(t['arguments_json'])['operation'] == 'save']
    if len(saves) != 1 or len(opened) != 1:
        return False
    save = saves[0]; report = opened[0]; stored = value['artifacts'][0]
    args = strict_json(save['arguments_json'])
    if (not exact(args,dict(operation='save',name='anchor-replay-research.md',mediaType='text/markdown',text=report['text'])) or
            strict_json(save['result_json'])['artifact_refs'] != [report['ref']] or
            stored['put_key'] != 'tool:'+save['invocation_id'] or stored['run_id'] != save['run_id'] or
            stored['media_type'] != 'text/markdown' or stored['name'] != args['name'] or
            stored['byte_size'] != len(report['text'].encode())):
        return False
    publishes = [t for t in tools if t['tool_name'] == 'builtin__artifact__manage' and
                 strict_json(t['arguments_json'])['operation'] == 'publish']
    return (len(publishes) == 2 and all(strict_json(t['arguments_json'])['artifactRefs'] == [report['ref']] for t in publishes) and
        {(p['run_id'],p['artifact_ref']) for p in value['publications']} == {(t['run_id'],report['ref']) for t in publishes} and
        strict_json(value['updates'][0]['artifact_refs']) == [report['ref']] and
        {d['message_id'] for d in value['deliveries']} == {m['message_id'] for m in value['conversation'] if m['role']=='assistant'} and
        all(d['status'] == 'sent' for d in value['deliveries']))


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
        native_bindings=bindings(value,documents,artifacts),
        frozen_generation_records=frozen_generations(value),
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
            try:
                h.cleanup()
                row['owned_processes_joined']=True
            finally:
                (evidence/(variant['id']+'.json')).write_text(json.dumps(row,ensure_ascii=False,separators=(',',':'))+'\n')
    row['cleanup']=dict(owned_processes_joined=True,disposable_root_removed=not root.exists())
    row['ended_at']=datetime.now(timezone.utc).isoformat()
    (evidence/(variant['id']+'.json')).write_text(json.dumps(row,ensure_ascii=False,separators=(',',':'))+'\n')
    if failure: raise failure
    return row


def execution_ids(row):
    result = {}
    for table, key in (('runs','run_id'), ('tools','invocation_id'), ('generations','generation_id'),
                       ('deliveries','delivery_id'), ('conversation','message_id')):
        ids = [item[key] for item in row['state'][table]]
        require(bool(ids) and all(type(i) is str and i for i in ids) and len(set(ids)) == len(ids),
                'invalid GC-16 execution identities: '+table)
        result[table] = ids
    return result


def rejection_probes(row):
    require(all(observe(row).values()),'passing native GC-16 receipt required')
    mutations = dict(
        input=lambda r:r['state']['conversation'][0]['content'][0].update(text='replaced'),
        tool_args=lambda r:r['state']['tools'][0].update(arguments_json='{}'),
        tool_result=lambda r:r['state']['tools'][3].update(result_json='{}'),
        child_result=lambda r:r['state']['worker_results'][0].update(content='[]'),
        child_link=lambda r:r['state']['child_links'][0].update(child_run_id='wrong-child'),
        child_epoch=lambda r:next(n for n in r['state']['runs'] if n['parent_run_id']).update(task_epoch=99),
        child_snapshot=lambda r:next(n for n in r['state']['runs'] if n['parent_run_id']).update(model_snapshot_id='wrong-model'),
        capability=lambda r:r['state']['capabilities'][0].update(tools_json='[]'),
        generation=lambda r:r['state']['generations'][-1].update(request_json='{}'),
        missing_source=lambda r:r['remote'].pop(),
        duplicate_source=lambda r:r['remote'].append(r['remote'][0]),
        report_hash=lambda r:r['artifacts'][0].update(sha256='0'*64),
        report_bytes=lambda r:r['artifacts'][0].update(text='forged'),
        artifact_owner=lambda r:r['state']['artifacts'][0].update(run_id='other-root'),
        artifact_key=lambda r:r['state']['artifacts'][0].update(put_key='tool:other'),
        publication=lambda r:r['state']['publications'].pop(),
        delivery=lambda r:r['state']['deliveries'][0].update(message_id='other-message'),
        approval=lambda r:r['state']['approvals'].append(dict(status='approved')),
        automation=lambda r:r['state']['automation'].append(dict(enabled=1)),
        attempt=lambda r:r['state']['tools'][0].update(attempt_count=2),
        unknown=lambda r:r['state']['tools'][0].update(status='unknown'),
        crash=lambda r:r['restarts'][0].update(returncode=0),
        admission=lambda r:r['restarts'][0]['remote'].append(dict(operation='open',key='D1')),
        completed_call=lambda r:r['restarts'][1]['before']['tools'][0].update(arguments_json='{}'),
        replay=lambda r:r['replay_state']['tools'].clear(),
        lease=lambda r:r['state']['claims'][0].update(token='retained'))
    # Synchronize replay and recorded flags to defeat equality-only validation.
    result = {}
    for name, mutate in mutations.items():
        changed = copy.deepcopy(row)
        try:
            mutate(changed)
            if name not in ('replay','completed_call','crash','admission'):
                changed['replay_state'] = copy.deepcopy(changed['state'])
            result[name] = not all(observe(changed).values())
        except (KeyError, IndexError, TypeError, ValueError, RuntimeError, StopIteration):
            result[name] = True
    require(all(result.values()),'GC-16 corruption accepted: '+repr(result))
    return result


def validate_report(report, binary=None):
    require(report.get('case_id') == 'GC-16' and report.get('status') == 'mechanical_pass' and
        report.get('scope') == 'scripted_whole_case_mechanics' and report.get('cleanup') is True and
        report.get('semantic_score','missing') is None and report.get('eligible_for_grading') is False and
        report.get('release_ready') is False and exact(report.get('boundary_lock'),load()) and
        exact(report.get('fixture_lock'),strict_json(FIXTURE.with_suffix('.lock.json').read_bytes())),
        'complete locked ungraded GC-16 receipts required')
    identity = report['identity']; binary = Path(binary or identity['binary_path'])
    require(exact(identity['source_before'],identity['source_after']) and identity['source_before'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and exact(identity['environment'], environment(binary)),
        'GC-16 source/binary/environment identity drift')
    proof = dict(**source_state(),binary_sha256=identity['binary_sha256'])
    require(exact(report.get('source_provenance'),dict(before=proof,after=proof)), 'GC-16 source provenance drift')
    rows = report['variants']
    require(exact([r['variant'] for r in rows],variants()), 'GC-16 complete ordered six-variant inventory required')
    seen = {}
    for row in rows:
        require('failure' not in row and row.get('scope') == 'scripted_whole_case_mechanics' and
            row.get('semantic_score','missing') is None and row.get('owned_processes_joined') is True and
            exact(row.get('cleanup'),dict(owned_processes_joined=True,disposable_root_removed=True)),
            'GC-16 failed, graded or unclean native receipt')
        checks = observe(row)
        require(exact(row['assertions'],checks) and all(v is True for v in checks.values()), 'GC-16 failed or forged native assertions')
        for table, ids in execution_ids(row).items():
            require(seen.setdefault(table,set()).isdisjoint(ids), 'GC-16 reused execution identities: '+table)
            seen[table].update(ids)
    require(exact(report.get('rejection_probes'),rejection_probes(rows[0])), 'GC-16 corruption probes missing or drifted')
    return summary(report)


def summary(report):
    return dict(case_id='GC-16',status=report['status'],receipts=len(report['variants']),frozen_blocks=16,
        semantic_score=None,eligible_for_grading=False,release_ready=False,cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'],binary_sha256=report['identity']['binary_sha256'])


def write_report(path, report):
    raw = path.parent / ('.'+path.stem+'-native.json.gz')
    data = json.dumps(report,ensure_ascii=False,separators=(',',':')).encode()+b'\n'
    raw.write_bytes(gzip.compress(data,compresslevel=1,mtime=0))
    manifest = dict(kind='gc16_native_manifest',version=1,summary=summary(report),
        native=dict(path=raw.name,sha256=sha(raw.read_bytes())),native_bytes=raw.stat().st_size,
        native_uncompressed_bytes=len(data))
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','native_uncompressed_bytes'} and
        manifest['kind'] == 'gc16_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-16 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-16 native size mismatch')
    size = manifest['native_uncompressed_bytes']
    require(type(size) is int and 0 < size <= 512<<20, 'GC-16 bounded native receipt required')
    with gzip.GzipFile(fileobj=io.BytesIO(raw)) as archive:
        data = archive.read(size+1)
    require(len(data) == size, 'GC-16 decompressed size mismatch')
    report = strict_json(data)
    require(exact(manifest['summary'],summary(report)), 'GC-16 manifest/native summary drift')
    return report


def main():
    p=argparse.ArgumentParser(); p.add_argument('--binary',type=Path,required=True); p.add_argument('--evidence',type=Path,required=True)
    p.add_argument('--variant',choices=[v['id'] for v in variants()]); p.add_argument('--validate',action='store_true'); args=p.parse_args()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),args.binary))); return
    args.evidence.unlink(missing_ok=True); folder=args.evidence.with_suffix(''); folder.mkdir(parents=True,exist_ok=True)
    before=dict(**source_state(),binary_sha256=sha(args.binary.read_bytes()))
    identity=dict(source_before=source_identity(),binary_sha256=before['binary_sha256'],
        binary_path=str(args.binary.resolve()),environment=environment(args.binary))
    boundary=load(); lock=strict_json(FIXTURE.with_suffix('.lock.json').read_bytes())
    require(lock==dict(fixture_sha256=sha(FIXTURE.read_bytes()),golden_sha256=sha((ROOT/'evals/golden-cases.md').read_bytes())),'GC-16 fixture lock drift')
    report=dict(case_id='GC-16',status='inconclusive',scope='scripted_whole_case_mechanics',semantic_score=None,
        fixture_lock=lock,boundary_lock=boundary,source_provenance=dict(before=before),identity=identity,variants=[],cleanup=False,
        eligible_for_grading=False,release_ready=False,remaining=dict(parent='ERII-223',semantic_score=None,current_L2='unexecuted',
            unscripted='unscripted_cases.py freeze --case GC-16; original input fragment; connected source/Child/Artifact adapter remains due',
            safety='Read-only research requests no external effect or cancellation. Dedicated cancel/old-epoch/unknown conformance gates remain required.'))
    try:
        for variant in variants():
            if not args.variant or args.variant==variant['id']:
                report['variants'].append(drive(args.binary.resolve(),folder,variant))
        after=dict(**source_state(),binary_sha256=sha(args.binary.read_bytes()))
        identity['source_after']=source_identity()
        require(before==after,'source or binary changed during GC-16')
        report.update(status='mechanical_pass',source_provenance=dict(before=before,after=after),
            cleanup=all(all(r['cleanup'].values()) for r in report['variants']))
        report['rejection_probes']=rejection_probes(report['variants'][0])
        if not args.variant: validate_report(report,args.binary)
    except BaseException:
        report['status']='inconclusive'
        raise
    finally:
        write_report(args.evidence,report)
    print(json.dumps(summary(report)))


if __name__=='__main__': main()
