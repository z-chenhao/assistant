"""GC-08 assertions reject lost facts, guessed tables and premature dispatch."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import verify_gc08 as journey
import wedding_sandbox as peer
import prepare_release_eval as evaluator


class WeddingTests(unittest.TestCase):
    def documents(self):
        source=journey.variants()[0]['source']
        first=dict(event=source['event'],clock=source['clock'],checklist=copy.deepcopy(source['checklist']),
                   facts=dict(guests=None,table_count=None,per_table=None),summary='先确认 C1，阻塞 C2')
        second=copy.deepcopy(first);second['facts']['guests']=128
        second['checklist'][0]['status']='completed';second['checklist'][1]['status']='active'
        third=copy.deepcopy(second);third['draft']=dict(recipient='banquet@example.test',subject='婚礼桌次确认草稿',
            body='宾客128人，尚未确定桌数。请确认每桌人数及桌次数量。',attachments=[])
        return source,[first,second,third]

    def test_checklist_rejects_lost_facts_unchanged_dependency_guess_and_contact_drift(self):
        source,docs=self.documents()
        self.assertTrue(all(journey.document_observe(docs,source).values()))
        changes=[lambda d:d[1]['facts'].update(guests=None),lambda d:d[2]['facts'].update(table_count=13),
            lambda d:d[2]['facts'].update(per_table=10),lambda d:d[1]['checklist'][1].update(status='blocked'),
            lambda d:d[2]['checklist'][2].update(status='completed'),lambda d:d[0].update(summary='先联系摄影师'),
            lambda d:d[2]['draft'].update(recipient='photo@example.test'),lambda d:d[2]['draft'].update(attachments=['unknown.pdf']),
            lambda d:d[2].update(event=dict(source['event'],date='2026-10-31'))]
        for change in changes:
            bad=copy.deepcopy(docs);change(bad)
            self.assertFalse(all(journey.document_observe(bad,source).values()))
        self.assertFalse(all(journey.document_observe(docs[:2],source).values()))

    def test_approval_binds_call_identity_recipient_body_attachments_and_zero_attempt(self):
        draft=self.documents()[1][-1]['draft']
        tool=dict(invocation_id='invocation',provider_call_id='provider-call',tool_name='mcp__wedding__send',
            arguments_json=json.dumps(draft,ensure_ascii=False,sort_keys=True,separators=(',',':')),
            status='waiting',attempt_count=0,replay_policy='never')
        tool['request_hash']=journey.call_hash(tool)
        approval=dict(invocation_id='invocation',status='pending',frozen_arguments_hash=tool['request_hash'])
        self.assertTrue(journey.approval_observe([tool],[approval],draft))
        for change in [dict(provider_call_id='other'),dict(tool_name='mcp__other__send'),dict(status='succeeded'),
                       dict(attempt_count=1),dict(replay_policy='safe'),dict(arguments_json='{}'),dict(request_hash='wrong')]:
            self.assertFalse(journey.approval_observe([dict(tool,**change)],[approval],draft))
        for change in [dict(status='approved'),dict(invocation_id='other'),dict(frozen_arguments_hash='wrong')]:
            self.assertFalse(journey.approval_observe([tool],[dict(approval,**change)],draft))
        self.assertFalse(journey.approval_observe([tool,tool],[approval],draft))

    def test_stdio_peer_reads_source_and_does_not_hide_wrong_or_duplicate_mail(self):
        with tempfile.TemporaryDirectory() as folder:
            root=Path(folder);source=self.documents()[0];(root/'source.json').write_text(json.dumps(source))
            requests=[dict(id=1,method='initialize'),dict(id=2,method='tools/list'),
                      dict(id=3,method='tools/call',params=dict(name='read',arguments=dict(resource='source')))]
            result=subprocess.run([sys.executable,str(journey.ROOT/'scripts/wedding_sandbox.py'),folder],
                input='\n'.join(json.dumps(r) for r in requests)+'\n',text=True,capture_output=True,check=True)
            self.assertEqual(json.loads(json.loads(result.stdout.splitlines()[-1])['result']['content'][0]['text']),source)
            self.assertEqual(peer.state(root),[])
            wrong=dict(self.documents()[1][-1]['draft'],recipient='wrong@example.test')
            peer.send(root,wrong);peer.send(root,wrong)
            self.assertEqual(peer.state(root),[wrong,wrong])

    def test_lock_variants_and_original_inputs_have_no_plans_or_answers(self):
        variants=journey.variants()
        self.assertEqual(len(variants),6)
        self.assertEqual(variants[0]['turns'],journey.TURNS)
        lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text())
        self.assertEqual(lock['fixture_sha256'],journey.sha(journey.FIXTURE.read_bytes()))
        self.assertEqual(lock['golden_sha256'],journey.sha((journey.ROOT/'evals/golden-cases.md').read_bytes()))
        for v in variants:
            self.assertEqual(v['source']['event']['task_id'],'task_wedding')
            self.assertNotIn('Capabilities:',json.dumps(v))
            self.assertNotIn('Wedding source:',json.dumps(v))

    def test_judge_refuses_partial_graded_or_unclean_evidence(self):
        base=dict(case_id='GC-08',variants=[],cleanup=True,status='mechanical_pass',semantic_score=None,
                  fixture_lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text()))
        for change in [{},dict(semantic_score=100),dict(cleanup=False),dict(status='inconclusive'),dict(fixture_lock={})]:
            with self.assertRaises(ValueError):evaluator.prepare(dict(base,**change))

    def lifecycle_row(self):
        """Small state receipts for rejection tests, not a substitute for CLI runs."""
        source,docs=self.documents();fixture=json.loads(journey.LIFECYCLE.read_text())
        docs.append(copy.deepcopy(docs[1]));docs.append(copy.deepcopy(docs[1]))
        docs[-1]['event']=dict(source['event'],date=fixture['event_date'])
        for item in docs[-1]['checklist']:
            if item['id'] in fixture['deadlines']:item['due']=fixture['deadlines'][item['id']]
        artifacts=[];metadata=[]
        for i,doc in enumerate(docs):
            text=json.dumps(doc);digest=journey.sha(text.encode());ref='artifact:'+str(i)
            artifacts.append(dict(ref=ref,text=text,sha256=digest,document=doc))
            metadata.append(dict(artifact_ref=ref,content_hash=digest,task_id='task-id',run_id='root'+str(i)))
        roots=[dict(run_id='root'+str(i),kind='worker',task_id='task-id',parent_run_id=None,
                    task_epoch=i+1 if i<4 else 6,status='completed',model_snapshot_id='model',capability_snapshot_id='worker') for i in range(5)]
        task=dict(task_id='task-id',task_ref='task:existing',anchor_task='immutable',status='completed',task_epoch=3,completion_policy='once')
        updates=[dict(root_run_id=r['run_id']) for r in roots[:3]]
        actions=[dict(operation='create'),*[dict(operation='append',taskRef='task:existing',instruction=t) for t in journey.TURNS[1:]],
                 dict(operation='append',taskRef='task:existing',instruction=fixture['continue']),dict(operation='pause',taskRef='task:existing'),
                 dict(operation='append',taskRef='task:existing',instruction=fixture['resume'])]
        tools=[dict(invocation_id='control'+str(i),tool_name='builtin__task__manage',run_id='main',arguments_json=json.dumps(a),
                    status='succeeded',attempt_count=1,provider_call_id='wedding-fixture',replay_policy='safe') for i,a in enumerate(actions)]
        for t in tools:t['request_hash']=journey.call_hash(t)
        tools[4]['attempt_count']=2
        send=dict(invocation_id='mail',run_id='root2',tool_name='mcp__wedding__send',arguments_json='{}',status='failed',attempt_count=0)
        state=dict(tasks=[task],runs=roots[:3],artifact=metadata[:3],updates=updates,tools=tools[:3]+[send],
            generations=[],snapshots=[dict(snapshot_id='model')],capabilities=[dict(snapshot_id='worker',role='worker',tools_json='[]')],
            inbox=[dict(message_json=json.dumps(dict(origin='runtime',details={}))) for _ in range(3)],reporter_dedup=[{}]*3,
            conversation=[],deliveries=[],usage=[],instructions=[],task_requests=[],task_invocations=[],approval=[dict(status='denied')],automation=[])
        interrupted=copy.deepcopy(state);interrupted['tasks'][0].update(status='active',task_epoch=4)
        interrupted['runs'].append(dict(roots[3],status='running'));interrupted['artifact']=metadata[:4];interrupted['tools']=tools[:4]+[send]
        paused=copy.deepcopy(interrupted);paused['tasks'][0].update(status='paused',task_epoch=5)
        paused['runs'][-1].update(status='cancelled',error_code='task_paused')
        paused['tools'].append(copy.deepcopy(tools[4]))
        after=copy.deepcopy(paused);after['tasks'][0].update(status='completed',task_epoch=6);after['runs'].append(roots[4])
        after['artifact']=metadata;after['updates'].append(dict(root_run_id='root4'))
        after['inbox'].append(dict(message_json=json.dumps(dict(origin='runtime',details={}))));after['reporter_dedup'].append({})
        inputs=journey.TURNS+[fixture[k] for k in ('continue','pause','resume')]
        after['conversation']=[dict(role='user',source='local',sequence=i+1,content=[dict(text=t)]) for i,t in enumerate(inputs)]
        after['instructions']=[dict(instruction=t) for t in journey.TURNS[1:]+[fixture['continue'],fixture['resume']]]
        after['task_requests']=[{}]*6;after['task_invocations']=[{}]*6
        read=dict(invocation_id='read',run_id='root4',tool_name='builtin__artifact__manage',status='succeeded',attempt_count=1,
                  arguments_json=json.dumps(dict(operation='read',artifactRef=artifacts[3]['ref'],limit=4096)),
                  result_json=json.dumps(dict(text=json.dumps(dict(text=artifacts[3]['text'])))))
        after['tools']=tools+[send,read]
        stop=copy.deepcopy(paused);stop['tools'][-1].update(status='running',attempt_count=1)
        return dict(variant=dict(source=source,turns=journey.TURNS),artifacts=artifacts[:3],denial=dict(state=state),
                    lifecycle=dict(fixture=fixture,interrupted=interrupted,stop_committed=stop,paused=paused,
                        resumed=after,artifacts=artifacts,remote=[],paused_replay=copy.deepcopy(paused),replay=copy.deepcopy(after),
                        crashes=[dict(boundary='before_result_commit',returncode=-9),dict(boundary='after_stop_commit',returncode=-9)]))

    def test_lifecycle_rejects_revived_roots_wrong_epoch_stale_result_and_lost_artifacts(self):
        row=self.lifecycle_row()
        self.assertTrue(all(journey.lifecycle_observe(row).values()))
        changes=[lambda l:l['paused']['tasks'][0].update(status='active'),lambda l:l['paused']['runs'][-1].update(error_code='task_cancelled'),
            lambda l:l['resumed']['runs'][3].update(status='completed'),lambda l:l['resumed']['runs'][-1].update(task_epoch=5),
            lambda l:l['resumed']['tasks'][0].update(task_epoch=5),lambda l:l['resumed']['tasks'][0].update(anchor_task='replaced'),
            lambda l:l['resumed']['updates'][-1].update(root_run_id='root3'),lambda l:l['resumed']['tools'][0].update(attempt_count=2),
            lambda l:l['resumed']['tools'][4].update(arguments_json=json.dumps(dict(operation='append'))),
            lambda l:l['resumed']['tools'][-1].update(result_json=json.dumps(dict(text='{}'))),
            lambda l:l['resumed']['artifact'][3].update(content_hash='lost'),lambda l:l['crashes'][0].update(returncode=0),
            lambda l:l['stop_committed']['tasks'][0].update(status='active'),lambda l:l['remote'].append(dict(sent=True)),
            lambda l:l['resumed']['approval'][0].update(status='approved'),lambda l:l['resumed']['automation'].append(dict(enabled=True)),
            lambda l:l['resumed']['tools'][-2].update(attempt_count=1),lambda l:l['paused_replay']['tasks'][0].update(task_epoch=6)]
        for change in changes:
            bad=copy.deepcopy(row);change(bad['lifecycle'])
            self.assertFalse(all(journey.lifecycle_observe(bad).values()),change)

    def test_only_incomplete_frozen_safe_pause_can_restart_one_attempt(self):
        row=self.lifecycle_row()
        self.assertTrue(journey.lifecycle_observe(row)['incomplete_safe_pause_reattached'])
        for field,value in [('attempt_count',1),('attempt_count',3),('replay_policy','never'),
                            ('request_hash','drift'),('provider_call_id','changed'),('arguments_json','{"operation":"pause","taskRef":"changed"}')]:
            bad=copy.deepcopy(row)
            # Detach this record from any other checkpoint: the test must
            # reject drift independently of complete final-state replay equality.
            bad['lifecycle']['resumed']['tools'][4]=dict(bad['lifecycle']['resumed']['tools'][4],**{field:value})
            self.assertFalse(journey.lifecycle_observe(bad)['incomplete_safe_pause_reattached'])
        bad=copy.deepcopy(row);bad['lifecycle']['paused']['task_requests'].append(dict(duplicate=True))
        self.assertFalse(journey.lifecycle_observe(bad)['incomplete_safe_pause_reattached'])

    def test_lifecycle_rejects_old_dates_repeated_counts_and_guessed_tables(self):
        row=self.lifecycle_row()
        changes=[lambda d:d['event'].update(date='2026-11-01'),lambda d:d['facts'].update(guests=None),
            lambda d:d['facts'].update(table_count=13),lambda d:d['facts'].update(per_table=10),
            lambda d:d['checklist'][1].update(due='2026-10-22'),lambda d:d['checklist'][2].update(status='completed'),
            lambda d:d.update(draft=dict(recipient='banquet@example.test'))]
        for change in changes:
            bad=copy.deepcopy(row);change(bad['lifecycle']['artifacts'][-1]['document'])
            self.assertFalse(all(journey.lifecycle_observe(bad).values()))
        lock=json.loads(journey.LIFECYCLE.with_suffix('.lock.json').read_text())
        self.assertEqual(lock['fixture_sha256'],journey.sha(journey.LIFECYCLE.read_bytes()))
        self.assertNotIn('128',row['lifecycle']['fixture']['resume'])


if __name__=='__main__':unittest.main()
