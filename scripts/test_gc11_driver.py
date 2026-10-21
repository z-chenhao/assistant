"""Unscripted input, frozen comparison conditions and ownership rejection."""
import copy
import json
from pathlib import Path
import tempfile
import subprocess
import sys
import os
import signal
import socket
import time
import unittest
from unittest.mock import patch

from case_runner import digest, load_contract
import gc11_driver as d
from gc11_fixture import reply


class DriverTests(unittest.TestCase):
    def sample(self, variant=None, arm='local', repetition=1):
        variant=variant or d.case_variants()[0]
        prefix=f'{arm}-{variant["variant_id"]}-{repetition}'
        identity=dict(arm=arm,variant_id=variant['variant_id'],repetition=repetition,
            execution_kind='offline_loopback_fixture',profile_id=prefix+'-profile',profile_version=2,
            limits=dict(context_window=8192,max_output_tokens=1024),turn_timeout_seconds=30)
        cap=dict(snapshot_id='cap',created_at=1,data_scope=prefix+'-agent',role='main',can_spawn_agents=0,
                 tools_json=json.dumps([dict(name='mcp__fixture__read',permission='read_only')]))
        model=dict(profile_id=identity['profile_id'],profile_version=2,**identity['limits'],
                   provider_kind='openai_compatible' if arm=='local' else 'openai_account',
                   model_id='local-model' if arm=='local' else 'account-model',
                   wire_api='openai_completions' if arm=='local' else 'openai_responses',
                   generation_defaults={},compatibility={'tool_result_format':'openai'},
                   capabilities={'text_input':True,'image_input':False})
        if arm=='local':model['endpoint_hash']='3'*64
        value=dict(agent_id=prefix+'-agent',capabilities=[cap],conversation=[],tools=[],runs=[],generations=[],
                   task=[],approval=[],automation=[],artifact=[],deliveries=[])
        for i,text in enumerate(d.prompts(variant)):
            rid=f'{prefix}-run-{i}';snapshot=dict(model,snapshot_id=f'{prefix}-model-{i}')
            value['runs'].append(dict(run_id=rid,kind='main',status='completed',model_snapshot_id=snapshot['snapshot_id'],capability_snapshot_id='cap'))
            request=dict(run_id=rid,model_snapshot=snapshot,tools=[dict(name='mcp__fixture__read')],
                         messages=[dict(role='user',text=d.prompts(variant)[0])])
            value['generations'].append(dict(status='completed',request_json=json.dumps(request)))
            value['conversation'].extend([dict(role='user',content=[dict(text=text)]),dict(role='assistant',content=[dict(text='fixture feedback')])])
            value['deliveries'].append(dict(message_id=f'm{i}',status='sent'))
        checks=d.observe(value,value,d.prompts(variant),[],identity)
        checks['source_stable']=True
        conditions=dict(source_sha='a'*40,source_dirty=False,source_diff_sha256='d'*64,driver_sha256='e'*64,
                        bridge_lock_sha256='f'*64,provider_data_sha256='0'*64,binary_sha256='b'*64,fixture_lock=load_contract(d.ROOT,'gc11-v1')[2],
                        config={'owned':'fixture'},bridge_sha256='c'*64,**d.derived_conditions(value,identity,variant))
        return dict(version=1,case_id='GC-11',identity=identity,variant=variant,status='mechanical_pass',
                    state=value,artifacts=[],assertions=checks,conditions=conditions,conditions_digest=digest(conditions),
                    replay_state_digest=digest(value),snapshots=[json.loads(g['request_json'])['model_snapshot'] for g in value['generations']],
                    semantic_score=None,cleanup=dict(owned_processes_joined=True))

    def cohort(self):
        result=[self.sample(v,a,n) for a in ('local','account') for v in d.case_variants() for n in range(1,6)]
        for r in result:
            proof={k:v for k,v in r['conditions'].items() if k in ('source_sha','source_dirty','source_diff_sha256','driver_sha256','bridge_lock_sha256','provider_data_sha256','binary_sha256','bridge_sha256')}
            proof.update(config_sha256='1'*64,manifest_sha256='2'*64)
            r['source_provenance']=dict(before=proof,after=copy.deepcopy(proof))
        return result

    def test_prompts_preserve_two_events_without_plans_or_gold_answers(self):
        for v in d.case_variants():
            actual=d.prompts(v)
            self.assertEqual(actual,[v['context']+'\n'+v['events'][0]['text'],v['events'][1]['text']])
            self.assertNotIn('Capabilities:', '\n'.join(actual))
            self.assertNotIn('5038.49','\n'.join(actual))
        # The HTTP fixture consumes the Bridge's actual raw Tool text format.
        chunks=[]
        body=dict(messages=[dict(role='user',content='original turn'),dict(role='tool',content=json.dumps({'quotes':[{'id':'A'}]}))])
        reply(body,lambda *a:chunks.append(a))
        args=json.loads(chunks[0][0]['tool_calls'][0]['function']['arguments'])
        self.assertEqual(json.loads(args['text']),dict(quote_ids=['A'],current_user_input='original turn'))

    def test_complete_offline_cohort_never_becomes_model_comparison_or_score(self):
        result=d.preflight(self.cohort())
        self.assertEqual(result['trials'],60)
        self.assertFalse(result['eligible_for_model_comparison'])
        self.assertIsNone(result['scores']);self.assertIsNone(result['winner'])

    def rewrite_model(self, report, **changes):
        for generation in report['state']['generations']:
            request=json.loads(generation['request_json'])
            request['model_snapshot'].update(changes)
            generation['request_json']=json.dumps(request)
        report['snapshots']=[json.loads(g['request_json'])['model_snapshot'] for g in report['state']['generations']]
        report['conditions'].update(d.derived_conditions(report['state'],report['identity'],report['variant']))
        report['conditions_digest']=digest(report['conditions'])
        report['replay_state_digest']=digest(report['state'])
        report['assertions']=dict(d.observe(report['state'],report['state'],d.prompts(report['variant']),
                                          report['artifacts'],report['identity']),source_stable=True)

    def test_model_parameters_are_frozen_within_each_arm(self):
        changes=dict(model_id='switched-model',endpoint_hash='4'*64,
                     endpoint='http://another-model/v1',wire_api='another-wire',
                     reasoning_effort='high',generation_defaults={'temperature':0.75},
                     compatibility={'tool_result_format':'provider_specific'},
                     capabilities={'text_input':True,'image_input':True},future_model_parameter='changed')
        for arm in ('local','account'):
            for index in (0,29):
                for field,value in changes.items():
                    with self.subTest(arm=arm,trial=index,field=field):
                        reports=self.cohort()
                        trial=[r for r in reports if r['identity']['arm']==arm][index]
                        # Both turns, raw state, snapshot copies and all witnesses
                        # agree: only cross-trial identity validation catches this.
                        self.rewrite_model(trial,**{field:value})
                        self.assertTrue(all(trial['assertions'].values()))
                        with self.assertRaises(ValueError):d.preflight(reports)

    def test_per_arm_identity_normalizes_only_installation_metadata(self):
        reports=self.cohort()
        for n,r in enumerate(reports):
            r['identity']['profile_version']=n+2
            self.rewrite_model(r,profile_version=n+2,created_at=f'installation-time-{n}',
                               auth_connection_id=f'installation-connection-{n}')
        result=d.preflight(reports)
        models=result['model_identities']
        self.assertEqual(models['local']['model_id'],'local-model')
        self.assertEqual(models['account']['model_id'],'account-model')
        self.assertNotEqual(models['local'],models['account'])
        for model in models.values():
            self.assertTrue({'provider_kind','model_id','wire_api','generation_defaults','compatibility','capabilities'}<=model.keys())
            self.assertFalse({'snapshot_id','profile_id','profile_version','created_at','auth_connection_id'} & model.keys())
        self.assertEqual(len(result['snapshots']['local']),30)
        self.assertEqual(result['snapshots']['account'][29]['model_snapshot']['profile_version'],61)
        self.assertFalse(result['eligible_for_model_comparison'])
        self.assertIsNone(result['scores']);self.assertIsNone(result['winner'])

    def test_synthetic_live_admission_rejects_coherent_model_switch(self):
        # Synthetic receipts plus mocked source identity exercise live admission,
        # not a real model trial or quality evaluation. No model process runs.
        reports=self.cohort();binary=Path(__file__).resolve()
        driver_hash=digest({p:d.file_hash(d.ROOT/p) for p in (
            'scripts/gc11_driver.py','scripts/verify_main.py','scripts/quote_case.py','scripts/case_runner.py')})
        for r in reports:
            r['identity']['execution_kind']='live_model'
            for name,value in dict(driver_sha256=driver_hash,binary_sha256=d.file_hash(binary)).items():
                r['conditions'][name]=value
                for phase in ('before','after'):r['source_provenance'][phase][name]=value
            r['conditions_digest']=digest(r['conditions'])
        with patch.object(d,'source_state',return_value=dict(source_sha='a'*40,source_dirty=False)):
            baseline=d.preflight(reports,binary)
            self.assertTrue(baseline['eligible_for_model_comparison'])
            self.assertIsNone(baseline['scores']);self.assertIsNone(baseline['winner'])
            self.rewrite_model(reports[0],model_id='model-B',endpoint_hash='6'*64,
                               compatibility={'version':'B'})
            with self.assertRaisesRegex(ValueError,'model identity drift within arm: local'):
                d.preflight(reports,binary)

    def test_missing_duplicate_reused_or_drifted_trials_are_rejected(self):
        original=self.cohort()
        for change in ('missing','duplicate','reused_run','input','limit','context','scope','assertion','replay','score','cleanup','model','source','dirty'):
            reports=copy.deepcopy(original);r=reports[-1]
            if change=='missing':reports.pop()
            elif change=='duplicate':reports[-1]=copy.deepcopy(reports[0])
            elif change=='reused_run':r['state']['runs'][0]['run_id']=reports[0]['state']['runs'][0]['run_id']
            elif change=='input':r['conditions']['inputs'][0]+='hidden hint'
            elif change=='limit':r['conditions']['limits']['max_output_tokens']+=1
            elif change=='context':r['conditions']['initial_messages'][0]['text']+='hidden context'
            elif change=='scope':r['state']['capabilities'][0]['data_scope']='other-agent'
            elif change=='assertion':r['assertions'].pop('no_effect_or_approval')
            elif change=='replay':r['replay_state_digest']='stale'
            elif change=='score':r['semantic_score']=100
            elif change=='cleanup':r['cleanup']['owned_processes_joined']=False
            elif change=='model':r['snapshots'][0]['model_id']='unobserved model'
            elif change=='source':r['source_provenance']['after']['source_sha']='changed'
            elif change=='dirty':r['conditions']['source_dirty']=True
            r['conditions_digest']=digest(r['conditions'])
            with self.subTest(change=change),self.assertRaises(ValueError):d.preflight(reports)

    def test_empty_tool_and_artifact_inventories_are_not_invented_case_obligations(self):
        r=self.sample()
        self.assertTrue(all(r['assertions'].values()))
        r['state']['approval']=[dict(status='pending')]
        self.assertFalse(d.observe(r['state'],r['state'],d.prompts(r['variant']),[],r['identity'])['no_effect_or_approval'])

    def test_comparison_cli_rejects_incomplete_rerun_and_clears_stale_output(self):
        folder=d.ROOT/'evidence';folder.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=folder,prefix='gc11-preflight-') as temp:
            root=Path(temp);trials=root/'trials';trials.mkdir()
            for i,r in enumerate(self.cohort()):(trials/f'{i}.json').write_text(json.dumps(r))
            output=root/'comparison.json'
            command=[sys.executable,str(d.ROOT/'scripts/gc11_driver.py'),'--compare',str(trials),'--evidence',str(output)]
            result=subprocess.run(command,text=True,capture_output=True)
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertFalse(json.loads(output.read_text())['eligible_for_model_comparison'])
            original=json.loads((trials/'0.json').read_text())
            switched=copy.deepcopy(original)
            self.rewrite_model(switched,model_id='another-model',endpoint_hash='5'*64,
                               compatibility={'tool_result_format':'provider_specific'})
            (trials/'0.json').write_text(json.dumps(switched))
            rejected=subprocess.run(command,text=True,capture_output=True)
            self.assertNotEqual(rejected.returncode,0)
            self.assertIn('model identity drift within arm: local',rejected.stderr)
            self.assertFalse(output.exists())
            (trials/'0.json').write_text(json.dumps(original))
            self.assertEqual(subprocess.run(command,capture_output=True).returncode,0)
            (trials/'0.json').unlink()
            self.assertNotEqual(subprocess.run(command,capture_output=True).returncode,0)
            self.assertFalse(output.exists())

    def test_variant_cohorts_cannot_hide_different_builds(self):
        reports=self.cohort()
        for r in reports:
            if r['variant']['variant_id']=='wording-2':
                r['conditions']['binary_sha256']='changed'
                r['source_provenance']['before']['binary_sha256']='changed'
                r['source_provenance']['after']['binary_sha256']='changed'
                r['conditions_digest']=digest(r['conditions'])
        with self.assertRaises(ValueError):d.preflight(reports)

    def test_driver_termination_reaps_owned_child_session(self):
        with tempfile.TemporaryDirectory() as temp:
            marker=Path(temp)/'listener.json'
            held=('import json,os,socket,time\nfrom pathlib import Path\n'
                  's=socket.socket();s.bind(("127.0.0.1",0));s.listen()\n'
                  f'Path({str(marker)!r}).write_text(json.dumps([os.getpid(),s.getsockname()[1]]))\n'
                  'time.sleep(60)\n')
            program=('import sys\nsys.path.insert(0,"scripts")\nfrom gc11_driver import owned_execution\n'
                     'from fixture_pool import run,run_cases\n'
                     f'owned_execution(lambda:run_cases([0],lambda _:run([sys.executable,"-c",{held!r}]),1))\n')
            child=subprocess.Popen([sys.executable,'-c',program],cwd=d.ROOT,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
            owned_pid=None
            try:
                deadline=time.monotonic()+5
                while not marker.exists() and child.poll() is None and time.monotonic()<deadline:time.sleep(.01)
                self.assertTrue(marker.exists(),'owned child never ready')
                owned_pid,port=json.loads(marker.read_text())
                child.send_signal(signal.SIGTERM);child.communicate(timeout=5)
                self.assertNotEqual(child.returncode,0)
                with self.assertRaises(OSError):socket.create_connection(('127.0.0.1',port),timeout=.2)
            finally:
                if child.poll() is None:child.terminate()
                child.communicate(timeout=5)
                if owned_pid:
                    try:os.killpg(owned_pid,signal.SIGKILL)
                    except ProcessLookupError:pass

    def test_owned_config_rejects_effects_drift_symlinks_and_wrong_source_before_work(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp).resolve();(root/'data').mkdir();(root/'provider').mkdir()
            binary=d.ROOT/'bin/personal-agent'
            source=load_contract(d.ROOT,'gc11-v1')[0]['cases'][0]['world']
            (root/'provider/mcp-data.json').write_text(json.dumps({'insurance_quotes':source}))
            cfg=dict(version=1,app=dict(data_root=str(root/'data')),
                     llm=dict(provider_bridge_command='node',provider_bridge_script=str(d.ROOT/'provider-bridge/bridge.mjs'),provider_bridge_version='pi-ai-1.3.0-v1'),
                     mcp=[dict(name='fixture',command=str(binary),args=['-test-mcp-server',str(root/'provider')],directory=str(root/'provider'),tools=dict(read='read_only'))])
            config=root/'config.json';config.write_text(json.dumps(cfg))
            identity=dict(version=1,issue='ERII-30',execution_kind='offline_loopback_fixture',arm='local',repetition=1,
                          variant_id='wording-1',turn_timeout_seconds=30,profile_id='fixture-profile',profile_version=2,
                          limits=dict(context_window=8192,max_output_tokens=1024),config_sha256=d.file_hash(config))
            marker=root/'erii-eval.json';marker.write_text(json.dumps(identity))
            self.assertEqual(d.owned(marker,binary)[1]['variant_id'],'wording-1')
            bad=dict(identity,api_key='DO_NOT_PERSIST_SECRET')
            marker.write_text(json.dumps(bad))
            with self.assertRaises(ValueError):d.owned(marker,binary)
            marker.write_text(json.dumps(identity))
            cfg['mcp'][0]['tools']['send']='external_effect';config.write_text(json.dumps(cfg))
            with self.assertRaises(ValueError):d.owned(marker,binary)
            identity['config_sha256']=d.file_hash(config);marker.write_text(json.dumps(identity))
            with self.assertRaises(ValueError):d.owned(marker,binary)
            link=root/'linked.json';link.symlink_to(marker)
            with self.assertRaises(ValueError):d.owned(link,binary)
            cfg['mcp'][0]['tools'].pop('send');config.write_text(json.dumps(cfg))
            identity['config_sha256']=d.file_hash(config);marker.write_text(json.dumps(identity))
            source['quotes'][0]['premium_cny']=0
            (root/'provider/mcp-data.json').write_text(json.dumps({'insurance_quotes':source}))
            with self.assertRaises(ValueError):d.owned(marker,binary)


if __name__=='__main__':unittest.main()
