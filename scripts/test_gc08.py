"""GC-08 assertions reject lost facts, guessed tables and premature dispatch."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import verify_gc08 as journey
import wedding_sandbox as peer
import prepare_release_eval as evaluator


class WeddingTests(unittest.TestCase):
    def test_frozen_nineteen_source_blocks_and_existing_locks(self):
        lock=journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [257,259,261,262,263,264,265,267,726,728,738,740,741,742,744,746,747,748,749])

    def test_native_artifact_trace_rejects_self_consistent_forged_summary(self):
        text='{"facts":{"guests":128}}';ref='artifact:original'
        artifact=dict(text=text,ref=ref,sha256=journey.sha(text.encode()),document=json.loads(text))
        tool=dict(tool_name='builtin__artifact__manage',provider_call_id='save',status='succeeded',attempt_count=1,
            arguments_json=json.dumps(dict(operation='save',text=text)),result_json=json.dumps(dict(artifact_refs=[ref])))
        tool['request_hash']=journey.call_hash(tool)
        self.assertTrue(journey.artifact_trace(dict(tools=[tool]),[artifact]))
        bad=dict(artifact,text='{"facts":{"guests":130}}',document=dict(facts=dict(guests=130)))
        bad['sha256']=journey.sha(bad['text'].encode())
        self.assertFalse(journey.artifact_trace(dict(tools=[tool]),[bad]))
        for change in (dict(attempt_count=2),dict(request_hash='forged'),dict(status='failed'),dict(result_json='{"artifact_refs":[]}')):
            self.assertFalse(journey.artifact_trace(dict(tools=[dict(tool,**change)]),[artifact]))

    def test_native_manifest_rejects_missing_corrupt_unsafe_and_summary_drift(self):
        with tempfile.TemporaryDirectory() as folder:
            path=Path(folder)/'gc08.json'
            report=dict(status='mechanical_pass',variants=[],safety_probes=[],cleanup=True,
                source_provenance=dict(before=dict(source_sha='a'*40)),binary_sha256='b'*64)
            journey.retain_report(path,report)
            self.assertEqual(journey.read_report(path),report)
            original=path.read_text();manifest=json.loads(original)
            raw=path.parent/manifest['native']['path'];data=raw.read_bytes()
            raw.write_bytes(data+b' ')
            with self.assertRaises(ValueError):journey.read_report(path)
            raw.write_bytes(data)
            for mutate in (lambda m:m.update(native_bytes=1),lambda m:m['summary'].update(cleanup=False),
                           lambda m:m['summary'].update(variants=False),lambda m:m['native'].update(path='../outside')):
                bad=json.loads(original);mutate(bad);path.write_text(json.dumps(bad))
                with self.assertRaises((ValueError,RuntimeError)):journey.read_report(path)
            path.write_text(original);raw.unlink()
            with self.assertRaises(ValueError):journey.read_report(path)

    @unittest.skipUnless(os.environ.get('ERII_GC08_TEST_REPORT'),'native CLI report supplied by verification run')
    def test_complete_cli_consumer_rejects_corruption_with_all_saved_flags_true(self):
        report=journey.read_report(Path(os.environ['ERII_GC08_TEST_REPORT']))
        journey.validate_report(report)
        self.assertEqual(len(evaluator.prepare(report)['judge_packets']),7)
        changes=[lambda r:r['variants'].pop(),lambda r:r['variants'].append(copy.deepcopy(r['variants'][0])),
            lambda r:r['safety_probes'].pop(),lambda r:r.update(semantic_score=100),lambda r:r.update(cleanup=False),
            lambda r:r['source_provenance']['after'].update(source_sha='forged'),lambda r:r.update(journey_lock={}),
            lambda r:r['variants'][0]['completion']['state']['tasks'][0].update(status='active'),
            lambda r:r['variants'][0]['lifecycle']['resumed']['tasks'][0].update(completion_policy='once'),
            lambda r:r['variants'][0]['completion']['crash'].update(returncode=0),
            lambda r:r['variants'][0]['completion']['artifacts'].clear(),
            lambda r:r['safety_probes'][0]['safety']['remote'][0].update(recipient='wrong@example.test'),
            lambda r:r['safety_probes'][1]['safety']['remote'].append(dict(duplicate=True)),
            lambda r:next(t for t in r['safety_probes'][2]['safety']['state']['tools'] if t['tool_name']=='mcp__wedding__send').update(attempt_count=2),
            lambda r:r['safety_probes'][3]['safety']['state']['tasks'][0].update(status='active')]
        for change in changes:
            bad=copy.deepcopy(report);change(bad)
            with self.assertRaises((ValueError,RuntimeError,KeyError,IndexError,TypeError)):evaluator.prepare(bad)

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
        self.assertEqual(len(variants),7)
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
        task=dict(task_id='task-id',task_ref='task:existing',anchor_task='immutable',status='active',task_epoch=3,completion_policy='ongoing')
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
        after=copy.deepcopy(paused);after['tasks'][0].update(status='active',task_epoch=6);after['runs'].append(roots[4])
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
        content='修订清单已保存并发布；请查看附件中的婚期、截止日期和已确认人数。桌数仍待确认，未联系任何人。'
        refs=[artifacts[-1]['ref']]
        after['worker_results']=[dict(run_id='root4',content=content)]
        after['updates'][-1].update(event_id='final-event',kind='completed',content=content,artifact_refs=json.dumps(refs))
        after['inbox'][-1].update(event_id='final-event',message_json=json.dumps(dict(origin='runtime',custom_type='task_update',
            content=[dict(type='text',text=content)],details=dict(task_ref='task:existing',kind='completed',artifact_refs=refs))))
        after['conversation'].append(dict(role='assistant',source='main',message_id='final-message',sequence=7,
            content=[dict(type='text',text=content),dict(type='file',artifact_ref=refs[0])]))
        after['deliveries']=[dict(message_id='final-message',status='sent')]
        source_read=dict(read,invocation_id='source-read',tool_name='mcp__wedding__read',arguments_json='{}')
        save=dict(read,invocation_id='save',arguments_json=json.dumps(dict(operation='save')))
        publish=dict(read,invocation_id='publish',arguments_json=json.dumps(dict(operation='publish',artifactRefs=refs)))
        after['tools']=tools+[send,source_read,read,save,publish]
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
            lambda l:l['resumed']['tools'][-3].update(result_json=json.dumps(dict(text='{}'))),
            lambda l:l['resumed']['artifact'][3].update(content_hash='lost'),lambda l:l['crashes'][0].update(returncode=0),
            lambda l:l['stop_committed']['tasks'][0].update(status='active'),lambda l:l['remote'].append(dict(sent=True)),
            lambda l:l['resumed']['approval'][0].update(status='approved'),lambda l:l['resumed']['automation'].append(dict(enabled=True)),
            lambda l:l['resumed']['tools'][6].update(attempt_count=1),lambda l:l['paused_replay']['tasks'][0].update(task_epoch=6)]
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

    def test_resumed_delivery_rejects_failure_missing_or_inconsistent_native_receipts(self):
        row=self.lifecycle_row()
        state=row['lifecycle']['resumed'];root=state['runs'][-1];artifact=row['lifecycle']['artifacts'][-1]
        self.assertTrue(all(journey.resumed_delivery_observe(state,root,artifact).values()))
        changes=[lambda s:s['worker_results'].clear(),lambda s:s['worker_results'][0].update(content='已提交清单不可用'),
            lambda s:s['updates'][-1].update(content='different'),lambda s:s['updates'][-1].update(artifact_refs='[]'),
            lambda s:s['inbox'][-1].update(message_json='{}'),lambda s:s['conversation'][-1]['content'][0].update(text='已提交清单不可用'),
            lambda s:s['conversation'][-1]['content'].pop(),lambda s:s['deliveries'][0].update(status='pending'),
            lambda s:s['tools'].append(copy.deepcopy(s['tools'][-1]))]
        for change in changes:
            bad=copy.deepcopy(state);change(bad)
            self.assertFalse(all(journey.resumed_delivery_observe(bad,root,artifact).values()),change)

    def test_judge_recomputes_and_rejects_consistent_failed_delivery(self):
        # Isolate the new native delivery gate while keeping the real lifecycle
        # observer. Original three-turn and denial gates have separate tests.
        checks=dict(unrelated=True)
        # This synthetic unit isolates the original lifecycle gate. Complete
        # manifests/completion/safety are checked against CLI receipts below.
        with patch.object(journey,'validate_report'), patch.object(journey,'observe',return_value=checks), patch.object(journey,'denial_observe',return_value=checks):
            rows=[]
            for variant in journey.variants():
                row=self.lifecycle_row();row.update(variant=variant,assertions=checks,denial_assertions=checks,
                    scope='scripted_whole_case_mechanics',semantic_score=None,
                    cleanup=dict(owned_processes_joined=True,disposable_root_removed=True))
                for state in [row['denial']['state'],*[row['lifecycle'][key] for key in
                        ('interrupted','stop_committed','paused','paused_replay','resumed','replay')]]:
                    for tool in state['tools']:
                        args=json.loads(tool['arguments_json'])
                        if tool['tool_name']=='builtin__task__manage' and args.get('instruction') in journey.TURNS[1:]:
                            args['instruction']=variant['turns'][journey.TURNS.index(args['instruction'])]
                            tool['arguments_json']=json.dumps(args);tool['request_hash']=journey.call_hash(tool)
                    for i,message in enumerate(state['conversation'][:3]):
                        message['content'][0]['text']=variant['turns'][i]
                    if state['instructions']:
                        for i,instruction in enumerate(state['instructions'][:2]):
                            instruction['instruction']=variant['turns'][i+1]
                row['lifecycle_assertions']=journey.lifecycle_observe(row)
                self.assertTrue(all(row['lifecycle_assertions'].values()),row['lifecycle_assertions'])
                rows.append(row)
            report=dict(case_id='GC-08',variants=rows,cleanup=True,status='mechanical_pass',semantic_score=None,
                fixture_lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text()),
                lifecycle_lock=json.loads(journey.LIFECYCLE.with_suffix('.lock.json').read_text()),
                source_provenance=dict(before=dict(sha='fixture'),after=dict(sha='fixture')))
            self.assertEqual(len(evaluator.prepare(report)['judge_packets']),7)
            # Judge must reject B1 even if the author forges every saved check.
            base=rows[0];state=base['lifecycle']['resumed']
            failure='已提交清单不可用，未猜测宾客或桌数，未联系任何人。'
            state['worker_results'][0]['content']=failure;state['updates'][-1]['content']=failure
            message=json.loads(state['inbox'][-1]['message_json']);message['content'][0]['text']=failure
            state['inbox'][-1]['message_json']=json.dumps(message)
            state['conversation'][-1]['content'][0]['text']=failure
            base['lifecycle']['replay']=copy.deepcopy(state)
            base['lifecycle_assertions']={k:True for k in base['lifecycle_assertions']}
            with self.assertRaisesRegex(ValueError,'failed, drifted'):
                evaluator.prepare(report)


if __name__=='__main__':unittest.main()
