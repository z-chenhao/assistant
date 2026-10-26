"""GC-12 observation and external Fake contracts; no synthetic semantic grades."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import meeting_sandbox as peer
import prepare_release_eval as evaluator
import verify_gc12 as journey


class MeetingTests(unittest.TestCase):
    def sample(self):
        args=dict(action='send_mail_and_create_calendar',recipient='alex.chen@example.test',
                  subject='Erii review',body='Alex，聊 Erii，30 分钟。',start='2026-10-20T11:00:00+08:00',
                  end='2026-10-20T11:30:00+08:00',timezone='Asia/Shanghai')
        remote=dict(mail=[dict(recipient=args['recipient'],subject=args['subject'],body=args['body'],status='sent')],
                    calendar=[dict(attendee=args['recipient'],title=args['subject'],start=args['start'],end=args['end'],
                                   timezone=args['timezone'],status='confirmed')],attempts=[args])
        old=dict(status='invalidated',invocation_id='old',frozen_arguments_hash='old-hash')
        new=dict(status='approved',invocation_id='new',frozen_arguments_hash='new-hash')
        tools=[dict(invocation_id='old',tool_name='mcp__meeting__invite',arguments_json=json.dumps(dict(args,start='2026-10-19T15:00:00+08:00')),
                    replay_policy='never',status='failed',attempt_count=0),
               dict(invocation_id='new',tool_name='mcp__meeting__invite',arguments_json=json.dumps(args),
                    replay_policy='never',status='succeeded',attempt_count=1),
               dict(tool_name='mcp__meeting__read',arguments_json='{"resource":"status"}',status='succeeded',
                    result_json=json.dumps(dict(text=json.dumps(remote))))]
        value=dict(tools=tools,requests=[dict(status='unknown',intent='approval'),dict(status='sent',intent='approval'),
                                        dict(status='sent',intent='completion')],
                   runs=[dict(kind='main',status='completed')],tasks=[],artifacts=[],conversation=[dict(role='assistant',content=[
                       dict(text='邮件 1 封、日历会议 1 个 alex.chen@example.test 2026-10-20T11:00:00+08:00')])])
        return value,remote,old,new,dict(mail=[],calendar=[],attempts=[])

    def test_full_case_rejects_generic_dispatch_missing_duplicate_or_wrong_effect(self):
        value,remote,old,new,before=self.sample()
        self.assertTrue(all(journey.observe(value,remote,old,new,before,remote).values()))
        for field in ('mail','calendar','attempts'):
            for change in ('absent','duplicate','wrong'):
                with self.subTest(field=field,change=change):
                    bad=copy.deepcopy(remote)
                    if change=='absent':bad[field]=[]
                    elif change=='duplicate':bad[field]*=2
                    else:bad[field][0].update(recipient='alex.wang@example.test',start='2026-10-19T15:00:00+08:00')
                    self.assertFalse(all(journey.observe(value,bad,old,new,before,bad).values()))
        for name,change in [('status',lambda v:v['tools'].pop()),
                            ('duplicate-completion',lambda v:v['requests'].append(v['requests'][-1])),
                            ('replay',lambda v:v['tools'][1].update(attempt_count=2)),
                            ('premature',lambda v:v['tools'][0].update(attempt_count=1))]:
            with self.subTest(name=name):
                bad=copy.deepcopy(value);change(bad)
                self.assertFalse(all(journey.observe(bad,remote,old,new,before,remote).values()))

    def test_query_only_metadata_never_normalizes_status_attempt_or_payload_drift(self):
        value=self.sample()[0]
        value['requests'][0].update(version=1,updated_at=1,attempt_count=1,message_id='old')
        other=copy.deepcopy(value);other['requests'][0].update(version=10,updated_at=10)
        self.assertEqual(journey.semantic_state(value),journey.semantic_state(other))
        for field,change in dict(status='sent',attempt_count=2,message_id='different').items():
            bad=copy.deepcopy(other);bad['requests'][0][field]=change
            self.assertNotEqual(journey.semantic_state(value),journey.semantic_state(bad))
        self.assertEqual(value['requests'][0]['version'],1)

    def test_stdio_peer_retains_both_actual_effects_and_never_hides_duplicate_dispatch(self):
        args=json.loads(self.sample()[0]['tools'][1]['arguments_json'])
        with tempfile.TemporaryDirectory() as folder:
            root=Path(folder)
            (root/'source.json').write_text(journey.FIXTURE.read_text())
            commands=[dict(id=1,method='initialize'),dict(id=2,method='tools/list'),
                      dict(id=3,method='tools/call',params=dict(name='invite',arguments=args)),
                      dict(id=4,method='tools/call',params=dict(name='read',arguments=dict(resource='status')))]
            run=subprocess.run([sys.executable,str(journey.ROOT/'scripts/meeting_sandbox.py'),folder],
                               input='\n'.join(json.dumps(c) for c in commands)+'\n',capture_output=True,text=True,check=True)
            rows=[json.loads(line) for line in run.stdout.splitlines()]
            state=json.loads(rows[-1]['result']['content'][0]['text'])
            self.assertEqual(state,peer.state(root))
            self.assertEqual([len(state[k]) for k in ('mail','calendar','attempts')],[1,1,1])
            peer.invite(root,args)
            self.assertEqual([len(peer.state(root)[k]) for k in ('mail','calendar','attempts')],[2,2,2])

    def test_variants_preserve_exact_base_and_source_contract(self):
        variants=journey.variants()
        self.assertEqual(len(variants),6)
        self.assertEqual(variants[0]['first'],journey.FIRST)
        self.assertEqual(variants[0]['steer'],'不要周一。')
        for v in variants:
            self.assertEqual(v['source']['recent_thread']['participants'],['user_eval_001','alex.chen@example.test'])
            self.assertNotIn('Capabilities:',v['first'])
            self.assertNotIn('2026-10-20',v['first'])

    def packets(self):
        rows=[]
        cases=journey.variants()
        cases.extend(dict(cases[0],id=mode,mode=mode) for mode in ('denied','lost-reply'))
        for variant in cases:
            value,remote,old,current,before=self.sample()
            value['approvals']=[old,current]
            value['conversation']=[dict(role='user',content=[dict(text=variant['first'])]),
                                   dict(role='user',content=[dict(text=variant['steer'])])]+value['conversation']
            for _ in range(2):
                value['tools'].append(dict(tool_name='mcp__meeting__read',arguments_json='{"resource":"source"}',
                                          result_json=json.dumps(dict(text=json.dumps(variant['source'])))))
            mode=variant.get('mode')
            if mode:
                effect=value['tools'][1]
                effect.update(status='failed' if mode=='denied' else 'unknown',attempt_count=0 if mode=='denied' else 1,
                              error_json=json.dumps(dict(code='APPROVAL_DENIED' if mode=='denied' else 'UNKNOWN_SIDE_EFFECT')))
                if mode=='denied':remote=dict(mail=[],calendar=[],attempts=[]);current['status']='denied'
                checks=journey.safety_observe(value,remote,mode,before,remote)
            else:checks=journey.observe(value,remote,old,current,before,remote)
            checks.update(journey.source_observe(value,variant))
            self.assertTrue(all(checks.values()))
            # Synthetic observations for validator unit tests; these are never
            # retained as evidence of a product execution.
            prefix=variant['id']+'/'
            value['runs']=[dict(run_id=prefix+'run',kind='main',status='completed',
                model_snapshot_id='model',capability_snapshot_id='capability')]
            value['snapshots']=[dict(snapshot_id='model')]
            value['capabilities']=[dict(snapshot_id='capability',can_spawn_agents=False)]
            value['generations']=[dict(generation_id=prefix+'generation',run_id=prefix+'run',status='completed',
                attempt_count=1,request_json=json.dumps(dict(model_snapshot=dict(snapshot_id='model'))))]
            for index,item in enumerate(value['tools']):
                item.update(invocation_id=prefix+'tool/'+str(index),provider_call_id='fixture')
                item['request_hash']=journey.call_hash(item)
            for approval,tool in zip(value['approvals'],value['tools']):
                approval.update(approval_id=tool['invocation_id']+'/approval',invocation_id=tool['invocation_id'],
                    frozen_arguments_hash=tool['request_hash'])
            for table,key in (('requests','delivery_id'),('conversation','message_id')):
                for index,item in enumerate(value[table]):item[key]=prefix+table+'/'+str(index)
            checkpoints=[]
            for name,statuses in [('Monday_frozen_before_notice',['pending']),
                                  ('Tuesday_frozen_before_consent',['invalidated','pending'])]:
                cut=copy.deepcopy(value);cut['approvals']=cut['approvals'][:len(statuses)]
                cut['tools']=cut['tools'][:len(statuses)]
                for approval,status in zip(cut['approvals'],statuses):approval['status']=status
                for tool in cut['tools']:tool['attempt_count']=0
                checkpoints.append(dict(name=name,state=cut,remote=copy.deepcopy(before)))
            checkpoints.append(dict(name='completed',state=copy.deepcopy(value),remote=copy.deepcopy(remote)))
            rows.append(dict(variant=variant,scope='scripted_whole_case_mechanics',assertions=checks,state=value,
                             replay_state=copy.deepcopy(value),remote=remote,remote_replay=copy.deepcopy(remote),
                             replay_digest=journey.digest(journey.semantic_state(value)),checkpoints=checkpoints,
                             fixture_sha256=journey.hashlib.sha256(journey.FIXTURE.read_bytes()).hexdigest(),
                             semantic_score=None,cleanup=dict(owned_processes_joined=True,http_peer_joined=True,disposable_root_removed=True),
                             private_trace='PRIVATE_TRACE_MUST_NOT_LEAK',private_provider='PRIVATE_MODEL_MUST_NOT_LEAK'))
        identity=journey.source_identity()
        return dict(case_id='GC-12',variants=rows,status='mechanical_pass',cleanup=True,semantic_score=None,
            fixture_lock=journey.load(),binary_sha256='b'*64,
            identity=dict(source_before=identity,source_after=identity,binary_sha256='b'*64,environment=dict(test=True)))

    def test_frozen_nineteen_blocks_and_original_inputs_unchanged(self):
        lock=journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [313,315,317,318,319,320,321,323,827,829,840,842,843,844,846,848,849,850,851])
        from verify_suite import SHARDS
        self.assertEqual([(s,g.attempts) for s,gs in SHARDS.items() for g in gs if g.name=='gc12'],[('communication',5)])
        from unscripted_cases import inputs, matrix
        self.assertEqual(matrix()['GC-12']['adapter'],'conversation_fragment')
        self.assertIs(matrix()['GC-12']['eligible_for_grading'],False)
        for variant in journey.variants():
            observed=inputs('GC-12',variant['id'])
            self.assertEqual([e['text'] for e in observed['events']],[variant['first'],variant['steer']])
            self.assertEqual(json.loads(observed['context'].split('：',1)[1]),variant['source'])
            self.assertNotIn('Capabilities:',json.dumps(observed))

    def test_validator_rejects_identity_checkpoint_hash_cleanup_or_reused_execution(self):
        report=self.packets()
        self.assertEqual(journey.validate_report(report)['receipts'],8)
        mutations=[lambda r:r['variants'].reverse(),lambda r:r['identity'].update(source_after={}),
            lambda r:r['identity'].update(environment={}),lambda r:r.update(binary_sha256='0'*64),
            lambda r:r['variants'][0].update(cleanup={}),lambda r:r['variants'][0]['cleanup'].update(http_peer_joined=1),
            lambda r:r['variants'][0].update(replay_digest='0'*64),
            lambda r:r['variants'][0]['checkpoints'].pop(),
            lambda r:r['variants'][0]['checkpoints'][0]['state']['approvals'][0].update(status='approved'),
            lambda r:r['variants'][0]['state']['approvals'][0].update(frozen_arguments_hash='bad'),
            lambda r:r['variants'][0]['state']['generations'][0].update(attempt_count=2),
            lambda r:r['variants'][0]['state']['capabilities'][0].update(can_spawn_agents=True),
            lambda r:r['variants'][0].update(fixture_sha256='0'*64),
            lambda r:r['variants'][1]['state']['runs'][0].update(run_id=r['variants'][0]['state']['runs'][0]['run_id'])]
        for mutate in mutations:
            changed=copy.deepcopy(report);mutate(changed)
            with self.assertRaises((RuntimeError,ValueError,KeyError)):journey.validate_report(changed)
            with self.assertRaises(ValueError):evaluator.prepare(changed)

    def test_judge_packets_are_allowlisted_and_ineligible_for_scoring(self):
        prepared=evaluator.prepare(self.packets())
        self.assertEqual(len(prepared['judge_packets']),6)
        self.assertTrue(all(not p['eligible_for_grading'] for p in prepared['judge_packets']))
        self.assertIsNone(prepared['scores']);self.assertIsNone(prepared['winner'])
        for secret in ('PRIVATE_TRACE_MUST_NOT_LEAK','PRIVATE_MODEL_MUST_NOT_LEAK'):
            self.assertNotIn(secret,json.dumps(prepared))
        for packet in prepared['judge_packets']:
            self.assertEqual(set(packet['judge_input']),{'goal','allowed_fixture','conversation','final_artifact_summary','programmatic_assertions'})

    def test_judge_rejects_missing_failed_graded_or_drifted_gc12_receipts(self):
        for mutation in ('missing','score','failed','source','cleanup','replay','remote','mode'):
            with self.subTest(mutation=mutation):
                report=self.packets();row=report['variants'][0]
                if mutation=='missing':report['variants'].pop()
                if mutation=='score':row['semantic_score']=90
                if mutation=='failed':row['assertions']['no_unapproved_dispatch']=False
                if mutation=='source':row['variant']['source']['clock']='changed'
                if mutation=='cleanup':row['cleanup']['owned_processes_joined']=False
                if mutation=='replay':row['replay_state']['tools'][1]['attempt_count']=2
                if mutation=='remote':row['remote_replay']['calendar']=[]
                if mutation=='mode':report['variants'][-1]['variant']['mode']='success'
                with self.assertRaises(ValueError):evaluator.prepare(report)

    def test_failed_cli_rerun_removes_consumable_success_report(self):
        folder=journey.ROOT/'evidence';folder.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=folder,prefix='gc12-rerun-') as temp:
            root=Path(temp);output=root/'gc12.json';preview=root/'judge.json'
            binary=os.environ.get('ERII_GC12_TEST_BINARY')
            if binary:
                success=subprocess.run([sys.executable,str(journey.ROOT/'scripts/verify_gc12.py'),
                                        '--binary',str(Path(binary).resolve()),'--evidence',str(output)],
                                       capture_output=True,text=True,timeout=180)
                self.assertEqual(success.returncode,0,success.stderr)
                report=json.loads(output.read_text())
                self.assertEqual(len(report['variants']),8)
                self.assertTrue(report['cleanup'])
                self.assertIsNone(report['semantic_score'])
            else:
                # Pure Python CI seeds the complete observation fixture;
                # the explicit binary runs the same check through the product.
                output.write_text(json.dumps(self.packets()))
            prepare=[sys.executable,str(journey.ROOT/'scripts/prepare_release_eval.py'),
                     '--input',str(output),'--evidence',str(preview)]
            accepted=subprocess.run(prepare,capture_output=True,text=True,timeout=30)
            self.assertEqual(accepted.returncode,0,accepted.stderr)
            self.assertEqual(len(json.loads(preview.read_text())['judge_packets']),6)
            missing=root/'deliberately-missing-erii'
            failed=subprocess.run([sys.executable,str(journey.ROOT/'scripts/verify_gc12.py'),
                                   '--binary',str(missing),'--evidence',str(output)],
                                  capture_output=True,text=True,timeout=30)
            self.assertNotEqual(failed.returncode,0)
            self.assertIn(str(missing),failed.stderr)
            self.assertFalse(output.exists(),'failed rerun retained a passing aggregate')
            rejected=subprocess.run(prepare,capture_output=True,text=True,timeout=30)
            self.assertNotEqual(rejected.returncode,0)
            self.assertFalse(preview.exists(),'Judge preparation retained the old preview')


if __name__ == '__main__':
    unittest.main()
