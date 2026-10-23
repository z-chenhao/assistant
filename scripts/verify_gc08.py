#!/usr/bin/env python3
"""Exact GC-08 three-turn mechanics at CLI/Task/Artifact/approval/MCP boundaries.

The explicit scripted model is not semantic or L2 evidence. Mail stays pending
or is denied; there is no positive consent in this fixture. Failed receipts are
retained. Launch/Doctor/Cleanup use the maintained MainHarness.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile

from verify_main import MainHarness, require
from verify_tasks import state as task_state, assertions as task_assertions
from wedding_sandbox import state as remote_state
from verify_suite import source_state

ROOT=Path(__file__).resolve().parents[1]
FIXTURE=ROOT/'evals/fixtures/gc08-v1.json'
LIFECYCLE=ROOT/'evals/fixtures/gc08-lifecycle-v1.json'
TURNS=['婚礼准备现在最应该处理什么？','宾客人数已经定了，128 人。','给酒店写封邮件确认桌次，但发之前给我看。']


def sha(data): return hashlib.sha256(data).hexdigest()


def call_hash(invocation):
    # Direct Tool calls bind the complete llm.ToolCall, including provider call
    # ID and Tool name (tools.go). This is not a hash of arguments alone.
    return sha(json.dumps(dict(id=invocation['provider_call_id'],name=invocation['tool_name'],
        arguments=invocation['arguments_json']),ensure_ascii=False,sort_keys=True,separators=(',',':')).encode())


def variants():
    source=json.loads(FIXTURE.read_text())
    rows=[dict(id='base',source=copy.deepcopy(source),turns=TURNS.copy()),
          dict(id='wording-1',source=copy.deepcopy(source),turns=['现在婚礼筹备优先做什么？','最终宾客人数确认：128 人。','给酒店草拟桌次确认邮件，发送前先让我审阅。']),
          dict(id='wording-2',source=copy.deepcopy(source),turns=['婚礼事项里眼下最该推进哪一项？','宾客最终为128人，已经确认。','请起草给酒店确认桌次的邮件，先给我看，不要直接发。'])]
    for name in ('checklist-order','contact-order','unrelated-note'):
        data=copy.deepcopy(source)
        if name=='checklist-order': data['checklist'].reverse()
        elif name=='contact-order': data['contacts']=dict(reversed(list(data['contacts'].items())))
        else: data['unrelated_note']='同事周末聚餐，与婚礼依赖无关。'
        rows.append(dict(id=name,source=data,turns=TURNS.copy()))
    return rows


class WeddingHarness(MainHarness):
    def args(self,*args):
        return [str(self.binary),'-config',str(self.root/'config.yaml'),'-test-bridge','wedding',*args]


def native(h):
    value=task_state(h)
    with sqlite3.connect(h.root/'data/state.db') as db:
        db.row_factory=sqlite3.Row
        for name in ('approval','artifact','automation'):
            value[name]=[dict(r) for r in db.execute('SELECT * FROM '+name+' ORDER BY rowid')]
    return value


def document_observe(docs,source):
    canonical=lambda c:{i['id']:i for i in c}
    initial=canonical(source['checklist'])
    draft=docs[-1].get('draft',{}) if docs else {}
    return dict(
        initial_critical_path=len(docs)==3 and canonical(docs[0]['checklist'])==initial and
            docs[0]['facts']==dict(guests=None,table_count=None,per_table=None) and
            docs[0]['summary'].startswith('先确认 C1') and '阻塞 C2' in docs[0]['summary'],
        durable_facts_and_dependency=len(docs)==3 and all(d['event']==source['event'] and d['clock']==source['clock'] and
            d['facts']==dict(guests=128,table_count=None,per_table=None) and
            canonical(d['checklist'])==dict(initial,C1=dict(initial['C1'],status='completed'),C2=dict(initial['C2'],status='active')) for d in docs[1:]),
        no_assumed_tables=draft.get('attachments')==[] and '128' in draft.get('body','') and '尚未确定桌数' in draft.get('body','') and
            '请确认每桌人数及桌次数量' in draft.get('body','') and draft.get('recipient')==source['contacts']['hotel'])


def approval_observe(send,approval,draft):
    return len(send)==len(approval)==1 and approval[0]['status']=='pending' and send[0]['status']=='waiting' and \
        send[0]['attempt_count']==0 and send[0]['replay_policy']=='never' and json.loads(send[0]['arguments_json'])==draft and \
        approval[0]['invocation_id']==send[0]['invocation_id'] and \
        approval[0]['frozen_arguments_hash']==send[0]['request_hash']==call_hash(send[0])


def observe(row):
    variant=row['variant'];steps=row['checkpoints'];value=row['state']
    roots=[r for r in value['runs'] if r['kind']=='worker']
    docs=[a['document'] for a in row['artifacts']]
    source_calls=[t for t in value['tools'] if t['tool_name']=='mcp__wedding__read']
    send=[t for t in value['tools'] if t['tool_name']=='mcp__wedding__send']
    approval=value['approval']
    actions=[json.loads(t['arguments_json']) for t in value['tools'] if t['tool_name']=='builtin__task__manage']
    draft=docs[-1].get('draft',{}) if docs else {}
    checks=task_assertions(value,1,3,2)
    # Third Root deliberately waits for exact consent. First two Roots completed
    # and reported once; pending consent is not a fabricated final completion.
    checks.update(
        three_original_turns=[p['text'] for m in value['conversation'] if m['role']=='user' for p in m['content']]==variant['turns'],
        actual_source_transport=len(source_calls)==1 and json.loads(json.loads(source_calls[0]['result_json'])['text'])==variant['source'],
        same_task_three_epochs=len(roots)==3 and [r['task_epoch'] for r in roots]==[1,2,3] and
            [r['status'] for r in roots]==['completed','completed','waiting'] and
            len(value['tasks'])==1 and value['tasks'][0]['status']=='active' and value['tasks'][0]['task_epoch']==3 and
            len({r['task_id'] for r in roots})==1 and len({r['run_id'] for r in roots})==3,
        main_create_then_append=[a['operation'] for a in actions]==['create','append','append'] and
            [a['instruction'] for a in actions[1:]]==variant['turns'][1:] and
            len(value['task_requests'])==3 and len(value['task_invocations'])==3 and
            len(value['instructions'])==2 and '128' in value['instructions'][0]['instruction'],
        anchor_immutable=all(s['state']['tasks'][0]['anchor_task']==value['tasks'][0]['anchor_task'] for s in steps),
        checklist_artifacts_reopened=len(docs)==3 and len(value['artifact'])==3 and all(
            a['sha256']==sha(a['text'].encode())==stored['content_hash'] and json.loads(a['text'])==a['document'] and
            a['ref']==stored['artifact_ref'] and stored['task_id']==roots[i]['task_id'] and stored['run_id']==roots[i]['run_id']
            for i,(a,stored) in enumerate(zip(row['artifacts'],value['artifact']))),
        frozen_draft_pending=approval_observe(send,approval,draft),
        no_contact_or_reminder=row['remote']==row['remote_replay']==[] and not value['automation'] and all(s['remote']==[] for s in steps),
        actual_restart_and_wrong_hash=row['restart']['returncode']==-9 and row['restart']['boundary']=='before_next_scan' and row['wrong_hash']['exit_code']!=0,
        replay_equal=value==row['replay_state'],
        only_expected_tools=all(t['tool_name'] in {'mcp__wedding__read','mcp__wedding__send','builtin__task__search',
            'builtin__task__manage','builtin__artifact__manage'} and
            (t['tool_name']!='builtin__artifact__manage' or json.loads(t['arguments_json']).get('operation') in ('save','publish')) and
            (t['status']=='succeeded' and t['attempt_count']==1 or t['tool_name']=='mcp__wedding__send' and t['status']=='waiting' and t['attempt_count']==0) for t in value['tools']))
    checks.update(document_observe(docs,variant['source']))
    return checks


def denial_observe(row):
    value=row['denial']['state'];effects=[t for t in value['tools'] if t['tool_name']=='mcp__wedding__send']
    return dict(no_dispatch=row['denial']['remote']==[] and len(effects)==1 and effects[0]['status']=='failed' and effects[0]['attempt_count']==0 and
            json.loads(effects[0]['result_json'])['error']['code']=='APPROVAL_DENIED',
        denied=len(value['approval'])==1 and value['approval'][0]['status']=='denied',
        all_roots_complete=all(r['status']=='completed' for r in value['runs']),
        replay_equal=value==row['denial']['replay_state'],
        one_completion_per_root=len(value['updates'])==len({u['root_run_id'] for u in value['updates']})==3)


def reopen(h):
    artifacts=[]
    for a in native(h)['artifact']:
        opened=h.command('artifact','open',a['artifact_ref'],ok=False)
        require(opened.returncode==0,'Artifact CLI reopen failed')
        artifacts.append(dict(ref=a['artifact_ref'],text=opened.stdout,sha256=sha(opened.stdout.encode()),document=json.loads(opened.stdout)))
    return artifacts


def lifecycle_observe(row):
    """Observe a bounded pause/continue trace; not a general state proof."""
    receipt=row['lifecycle'];fixture=json.loads(LIFECYCLE.read_text())
    before=row['denial']['state'];interrupted=receipt['interrupted'];paused=receipt['paused'];after=receipt['resumed']
    workers=lambda state:[r for r in state['runs'] if r['kind']=='worker']
    old=workers(before);active=workers(interrupted)[-1];stopped=workers(paused)[-1];fresh=workers(after)[-1]
    actions=[json.loads(t['arguments_json']) for t in after['tools'] if t['tool_name']=='builtin__task__manage']
    canonical=lambda c:{i['id']:i for i in c}
    original=canonical(row['artifacts'][1]['document']['checklist'])
    expected=copy.deepcopy(original)
    for item,due in fixture['deadlines'].items():expected[item]['due']=due
    artifacts=receipt['artifacts'];doc=artifacts[-1]['document']
    reads=[t for t in after['tools'] if t['run_id']==fresh['run_id'] and t['tool_name']=='builtin__artifact__manage' and
           json.loads(t['arguments_json']).get('operation')=='read']
    pause_calls=[t for t in receipt['stop_committed']['tools'] if t['tool_name']=='builtin__task__manage' and
                 json.loads(t['arguments_json']).get('operation')=='pause']
    pause=pause_calls[0] if len(pause_calls)==1 else {}
    recovered_pause=next((t for t in after['tools'] if t['invocation_id']==pause.get('invocation_id')), {})
    checks=task_assertions(after,1,5,4)
    checks.update(
        lifecycle_fixture=receipt['fixture']==fixture,
        original_turns_preserved=[p['text'] for m in after['conversation'] if m['role']=='user' for p in m['content']]==
            row['variant']['turns']+[fixture[k] for k in ('continue','pause','resume')],
        main_pause_then_append=[a['operation'] for a in actions]==['create','append','append','append','pause','append'] and
            actions[3]['instruction']==fixture['continue'] and actions[5]['instruction']==fixture['resume'] and
            all(a['taskRef']==before['tasks'][0]['task_ref'] for a in actions[1:]) and
            len(after['task_requests'])==len(after['task_invocations'])==6,
        committed_work_before_pause=len(workers(interrupted))==4 and active['status']=='running' and active['task_epoch']==4 and
            len(interrupted['artifact'])==4 and len(interrupted['updates'])==3 and interrupted['artifact'][:3]==before['artifact'],
        pause_fences_old_root=len(workers(paused))==4 and paused['tasks'][0]['status']=='paused' and paused['tasks'][0]['task_epoch']==5 and
            stopped['run_id']==active['run_id'] and stopped['status']=='cancelled' and stopped['error_code']=='task_paused' and
            stopped['task_epoch']==4 and paused['artifact']==interrupted['artifact'] and paused['updates']==before['updates'],
        continue_new_root=len(workers(after))==5 and fresh['status']=='completed' and fresh['task_epoch']==6 and
            fresh['run_id'] not in {r['run_id'] for r in workers(paused)} and after['tasks'][0]['task_epoch']==6 and
            after['tasks'][0]['status']=='completed' and after['tasks'][0]['completion_policy']=='once',
        old_roots_never_revived=workers(after)[:3]==old and workers(after)[3]==stopped,
        anchor_and_instructions=all(s['tasks'][0]['anchor_task']==before['tasks'][0]['anchor_task'] and len(s['tasks'])==1
            for s in (interrupted,paused,after)) and
            [i['instruction'] for i in after['instructions']]==row['variant']['turns'][1:]+[fixture['continue'],fixture['resume']],
        committed_calls_reused=all(next(t for t in after['tools'] if t['invocation_id']==prior['invocation_id'])==prior
            for prior in interrupted['tools'] if prior['status'] in ('succeeded','failed')) and
            all(next(g for g in after['generations'] if g['generation_id']==prior['generation_id'])==prior
                for prior in interrupted['generations'] if prior['status']=='completed'),
        incomplete_safe_pause_reattached=len(pause_calls)==1 and pause['status']=='running' and pause['attempt_count']==1 and
            recovered_pause.get('status')=='succeeded' and recovered_pause.get('attempt_count')==2 and
            pause['replay_policy']==recovered_pause['replay_policy']=='safe' and
            pause['request_hash']==recovered_pause['request_hash']==call_hash(pause)==call_hash(recovered_pause) and
            pause['run_id']==recovered_pause['run_id'] and pause['arguments_json']==recovered_pause['arguments_json'] and
            receipt['stop_committed']['task_invocations']==paused['task_invocations'] and
            receipt['stop_committed']['task_requests']==paused['task_requests'],
        no_late_result=active['run_id'] not in {u['root_run_id'] for u in after['updates']} and
            {u['root_run_id'] for u in after['updates']}=={r['run_id'] for r in workers(after) if r['status']=='completed'},
        resumed_reads_committed_artifact=len(reads)==1 and json.loads(reads[0]['arguments_json'])==dict(
            operation='read',artifactRef=artifacts[3]['ref'],limit=4096) and reads[0]['status']=='succeeded' and reads[0]['attempt_count']==1 and
            json.loads(json.loads(reads[0]['result_json'])['text']).get('text')==artifacts[3]['text'] and '128' not in fixture['resume'],
        revised_date_and_facts=doc['event']==dict(row['variant']['source']['event'],date=fixture['event_date']) and
            doc['facts']==dict(guests=128,table_count=None,per_table=None) and canonical(doc['checklist'])==expected and 'draft' not in doc,
        retained_artifacts_reopened=len(artifacts)==len(after['artifact'])==5 and artifacts[:3]==row['artifacts'] and
            canonical(artifacts[3]['document']['checklist'])==original and artifacts[3]['document']['facts']==doc['facts'] and
            all(a['sha256']==sha(a['text'].encode())==stored['content_hash'] and a['ref']==stored['artifact_ref'] and
                json.loads(a['text'])==a['document'] and stored['run_id']==workers(after)[i]['run_id'] and
                stored['task_id']==before['tasks'][0]['task_id'] for i,(a,stored) in enumerate(zip(artifacts,after['artifact']))),
        no_new_contact_or_approval=receipt['remote']==[] and after['approval']==before['approval'] and not after['automation'] and
            len([t for t in after['tools'] if t['tool_name']=='mcp__wedding__send'])==1 and
            all(t['attempt_count']==0 and t['status']=='failed' for t in after['tools'] if t['tool_name']=='mcp__wedding__send'),
        lifecycle_tools_allowed=all(t['tool_name'] in {'mcp__wedding__read','mcp__wedding__send','builtin__task__search',
            'builtin__task__manage','builtin__artifact__manage'} and
            (t['tool_name']!='builtin__artifact__manage' or json.loads(t['arguments_json']).get('operation') in ('save','publish','read')) and
            (t['status']=='succeeded' and (t['attempt_count']==1 or t['invocation_id']==pause.get('invocation_id') and t['attempt_count']==2) or t['tool_name']=='mcp__wedding__send' and
                t['status']=='failed' and t['attempt_count']==0) for t in after['tools']),
        actual_crashes=receipt['crashes']==[dict(boundary='before_result_commit',returncode=-9),dict(boundary='after_stop_commit',returncode=-9)],
        durable_pause_intent=receipt['stop_committed']['tasks'][0]['status']=='paused' and receipt['stop_committed']['tasks'][0]['task_epoch']==5,
        paused_replay_equal=paused==receipt['paused_replay'],
        resumed_replay_equal=after==receipt['replay'])
    return checks


def drive_lifecycle(h,row,provider):
    fixture=json.loads(LIFECYCLE.read_text())
    receipt=dict(fixture=fixture,crashes=[])
    # Save/publish an actual fourth checklist, then kill before its result
    # transaction commits. Pause through Main before that Root can resume.
    h.send('gc08-continue',fixture['continue'])
    child=h.hold('before_result_commit');child.kill();child.communicate(timeout=10)
    receipt['crashes'].append(dict(boundary='before_result_commit',returncode=child.returncode))
    receipt['interrupted']=native(h)
    h.send('gc08-pause',fixture['pause'])
    child=h.hold('after_stop_commit');child.kill();child.communicate(timeout=10)
    receipt['crashes'].append(dict(boundary='after_stop_commit',returncode=child.returncode))
    receipt['stop_committed']=native(h)
    h.command('chat','work');receipt['paused']=native(h)
    h.send('gc08-pause',fixture['pause']);h.command('chat','work')
    receipt['paused_replay']=native(h)
    h.send('gc08-resume',fixture['resume']);h.command('chat','work')
    receipt['resumed']=native(h);receipt['artifacts']=reopen(h)
    for key in ('continue','pause','resume'):h.send('gc08-'+key,fixture[key])
    h.command('chat','work');receipt['replay']=native(h);receipt['remote']=remote_state(provider)
    row['lifecycle']=receipt
    row['lifecycle_assertions']=lifecycle_observe(row)
    require(all(row['lifecycle_assertions'].values()),'GC-08 lifecycle assertions: '+json.dumps(row['lifecycle_assertions']))


def drive(binary,evidence,variant):
    receipt=dict(variant=variant,scope='scripted_whole_case_mechanics',semantic_score=None,checkpoints=[])
    failure=None
    with tempfile.TemporaryDirectory(prefix='erii-gc08-') as temp:
        root=Path(temp);provider=root/'provider';provider.mkdir()
        (provider/'wedding_sandbox.py').write_bytes((ROOT/'scripts/wedding_sandbox.py').read_bytes())
        (provider/'source.json').write_text(json.dumps(variant['source'],ensure_ascii=False))
        interpreter=subprocess.run(['xcrun','--find','python3'],capture_output=True,text=True,check=True).stdout.strip() if sys.platform=='darwin' else '/usr/bin/python3'
        (root/'config.yaml').write_text('version: 1\nmcp:\n  - name: wedding\n    command: '+json.dumps(interpreter)+
            '\n    directory: '+json.dumps(str(provider))+'\n    args: ['+json.dumps(str(provider/'wedding_sandbox.py'))+', '+json.dumps(str(provider))+']\n    tools: {read: read_only, send: external_effect}\n')
        h=WeddingHarness(binary,root)
        try:
            h.setup()
            for i,text in enumerate(variant['turns']):
                h.send('gc08-'+str(i),text)
                h.command('chat','work')
                receipt['checkpoints'].append(dict(turn=i+1,state=native(h),remote=remote_state(provider)))
            value=native(h);before=remote_state(provider)
            artifacts=reopen(h)
            pending=h.command('approval','list')
            require(len(pending)==1 and pending[0]['status']=='pending','exact draft approval missing')
            wrong=h.command('approval','approve',pending[0]['approval_id'],'wrong-hash',ok=False)
            require(wrong.returncode!=0,'changed frozen parameters accepted')
            receipt['wrong_hash']=dict(exit_code=wrong.returncode,error=wrong.stderr)
            # An actual SIGKILL of a CLI process while idle/pending at a known
            # scan boundary; restart must keep that same unconsumed approval.
            child=h.hold('before_next_scan');child.kill();child.communicate(timeout=10)
            receipt['restart']=dict(boundary='before_next_scan',returncode=child.returncode)
            for i,text in enumerate(variant['turns']): h.send('gc08-'+str(i),text)
            h.command('chat','work')
            receipt.update(state=value,replay_state=native(h),remote=before,remote_replay=remote_state(provider),artifacts=artifacts)
            receipt['assertions']=observe(receipt)
            require(all(receipt['assertions'].values()),'GC-08 assertions: '+json.dumps(receipt['assertions']))
            # Deny rather than invent user consent. Duplicate denial must not
            # create a send; retain separate terminal readback and replay proof.
            a=pending[0]
            h.command('approval','deny',a['approval_id'],a['frozen_arguments_hash'])
            h.command('chat','work')
            denied=native(h)
            duplicate=h.command('approval','deny',a['approval_id'],a['frozen_arguments_hash'],ok=False)
            h.command('chat','work')
            receipt['denial']=dict(state=denied,replay_state=native(h),duplicate_exit=duplicate.returncode,remote=remote_state(provider))
            receipt['denial_assertions']=denial_observe(receipt)
            require(all(receipt['denial_assertions'].values()),'GC-08 denial failed')
            drive_lifecycle(h,receipt,provider)
        except BaseException as error:
            receipt.update(failure=str(error),failure_state=h.failure_state())
            failure=error
        finally:
            h.cleanup()
            receipt['owned_processes_joined']=True
            (evidence/(variant['id']+'.json')).write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+'\n')
    receipt['cleanup']=dict(owned_processes_joined=True,disposable_root_removed=not root.exists())
    (evidence/(variant['id']+'.json')).write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+'\n')
    if failure: raise failure
    return receipt


def main():
    p=argparse.ArgumentParser();p.add_argument('--binary',type=Path,required=True);p.add_argument('--evidence',type=Path,required=True)
    p.add_argument('--variant',choices=[v['id'] for v in variants()]);args=p.parse_args()
    args.evidence.unlink(missing_ok=True);folder=args.evidence.with_suffix('');folder.mkdir(parents=True,exist_ok=True)
    initial=dict(**source_state(),binary_sha256=sha(args.binary.read_bytes()))
    lock=json.loads(FIXTURE.with_suffix('.lock.json').read_text())
    require(lock==dict(fixture_sha256=sha(FIXTURE.read_bytes()),golden_sha256=sha((ROOT/'evals/golden-cases.md').read_bytes())),'GC-08 fixture lock drift')
    lifecycle_lock=json.loads(LIFECYCLE.with_suffix('.lock.json').read_text())
    require(lifecycle_lock==dict(fixture_sha256=sha(LIFECYCLE.read_bytes()),golden_sha256=lock['golden_sha256']),'GC-08 lifecycle lock drift')
    rows=[drive(args.binary.resolve(),folder,v) for v in variants() if not args.variant or v['id']==args.variant]
    final=dict(**source_state(),binary_sha256=sha(args.binary.read_bytes()))
    require(initial==final,'source or binary changed during GC-08 execution')
    report=dict(source_provenance=dict(before=initial,after=final),case_id='GC-08',status='mechanical_pass',scope='scripted_whole_case_mechanics',semantic_score=None,
        fixture_lock=lock,lifecycle_lock=lifecycle_lock,binary_sha256=sha(args.binary.read_bytes()),variants=rows,cleanup=all(all(r['cleanup'].values()) for r in rows))
    args.evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(dict(status=report['status'],variants=len(rows),cleanup=report['cleanup'])))


if __name__=='__main__':main()
