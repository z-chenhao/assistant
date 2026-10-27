"""GC-16 corpus boundary and oracle must reject missing/unsupported evidence."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import prepare_release_eval as evaluator
import research_sandbox as peer
import verify_gc16 as journey
import verify_suite as suite


class ResearchTests(unittest.TestCase):
    def test_lock_original_input_and_six_variants_preserve_all_five_documents(self):
        variants = journey.variants()
        self.assertEqual(len(variants), 6)
        self.assertEqual(variants[0]['text'], journey.INPUT)
        self.assertEqual(len({v['seed'] for v in variants}), 6)
        base = {d['id']:d for d in variants[0]['source']['documents']}
        for v in variants:
            self.assertEqual({d['id']:d for d in v['source']['documents'] if d['id'] in base}, base)
            self.assertNotIn('Capabilities:', v['text'])
        lock = json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text())
        self.assertEqual(lock, dict(fixture_sha256=journey.sha(journey.FIXTURE.read_bytes()),
            golden_sha256=journey.sha((journey.ROOT/'evals/golden-cases.md').read_bytes())))

    def test_peer_serves_exact_documents_and_retains_duplicate_and_unknown_requests(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp); source=journey.variants()[0]['source']
            (root/'source.json').write_text(json.dumps(source))
            requests=[dict(id=1,method='initialize'), dict(id=2,method='tools/list'),
                dict(id=3,method='tools/call',params=dict(name='search',arguments=dict(key='Anchor Replay'))),
                dict(id=4,method='tools/call',params=dict(name='open',arguments=dict(key='D1')))]
            response=subprocess.run([sys.executable,str(journey.ROOT/'scripts/research_sandbox.py'),tmp],
                input='\n'.join(json.dumps(r) for r in requests)+'\n',capture_output=True,text=True,check=True)
            lines=[json.loads(s) for s in response.stdout.splitlines()]
            self.assertEqual(json.loads(lines[-1]['result']['content'][0]['text']),source['documents'][0])
            peer.read(root,'open','D1')
            with self.assertRaises(KeyError):peer.read(root,'open','not-found')
            self.assertEqual(peer.state(root),[dict(operation='search',key='Anchor Replay'),
                dict(operation='open',key='D1'),dict(operation='open',key='D1'),dict(operation='open',key='not-found')])

    def test_report_oracle_refuses_missing_or_false_primary_claims_and_citations(self):
        source=journey.variants()[0]['source']
        text='# Anchor Replay 研究报告\n本地合成语料；不穷尽语料外资料。\n## 事实\n'
        for d in source['documents']:
            if d['id']=='D2': continue
            text+=f"{d['date']} {d['claim']} [{d['id']}: {d['title']}]({d['url']})\n"
        text+='Atlas 最早可作为证据的一手来源日期为 2024-03-01\n改名为 Durable Replay，语义不变\n属于扩展，不能倒推初版已有\n排除 Aster Step Cache，与 Atlas 不等价\n'
        text+='## 推断\n范围有限。\n## 未发现证据\n未发现公开证据支持早于 2024-03-01；D2（2024-02-10，二手猜测）只作发现线索。\n## 后续核查\n'
        self.assertTrue(all(journey.report_observe(text,source).values()))
        for changed in [text.replace('2024-03-01','2024-02-10'),text.replace('语义不变','完全重定义'),
            text.replace('属于扩展，不能倒推初版已有','初版已有'),text.replace('与 Atlas 不等价','两者等价'),
            text.replace(source['documents'][0]['url'],'https://unverified.example.test'),
            text.replace('## 推断','## 结论'),text.replace('只作发现线索','用作最早证据')]:
            self.assertFalse(all(journey.report_observe(changed,source).values()))

    def test_judge_rejects_incomplete_scored_or_drifted_receipts(self):
        base=dict(case_id='GC-16',variants=[],cleanup=True,status='mechanical_pass',semantic_score=None,
            fixture_lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text()))
        for change in ({},dict(semantic_score=100),dict(cleanup=False),dict(fixture_lock={})):
            with self.assertRaises(ValueError):evaluator.prepare(dict(base,**change))

    def test_frozen_manifest_and_strict_consumer_are_present(self):
        lock=journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [369,371,373,374,375,376,377,379,932,934,943,945,947,948,949,950])
        with self.assertRaises(RuntimeError):journey.validate_report({})

    def fixture(self, source, binary):
        rows=[]
        for variant in journey.variants():
            prefix=variant['id']
            state={table:[{key:prefix+'-'+key}] for table,key in
                (('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),
                 ('deliveries','delivery_id'),('conversation','message_id'))}
            rows.append(dict(variant=variant,state=state,assertions=dict(observed=True),
                scope='scripted_whole_case_mechanics',semantic_score=None,owned_processes_joined=True,
                cleanup=dict(disposable_root_removed=True,owned_processes_joined=True)))
        proof=dict(source_sha='a'*40,source_dirty=False,source_diff_sha256='b'*64,binary_sha256=journey.sha(binary.read_bytes()))
        return dict(case_id='GC-16',status='mechanical_pass',scope='scripted_whole_case_mechanics',
            release_ready=False,semantic_score=None,eligible_for_grading=False,cleanup=True,
            fixture_lock=json.loads(journey.FIXTURE.with_suffix('.lock.json').read_text()),boundary_lock=journey.load(),
            source_provenance=dict(before=proof,after=proof),identity=dict(source_before=source,source_after=source,
                binary_path=str(binary),binary_sha256=proof['binary_sha256'],environment=dict(go='test')),
            rejection_probes=dict(bad=True),variants=rows)

    def test_strict_consumer_rejects_identity_inventory_copy_and_flags(self):
        source=dict(sha='a'*40,files={'runner':'hash'})
        with tempfile.TemporaryDirectory() as temp,patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'environment',return_value=dict(go='test')), \
                patch.object(journey,'observe',return_value=dict(observed=True)), \
                patch.object(journey,'rejection_probes',return_value=dict(bad=True)):
            binary=Path(temp)/'erii';binary.write_bytes(b'binary');report=self.fixture(source,binary)
            proof=report['source_provenance']['before']
            with patch.object(journey,'source_state',return_value={k:v for k,v in proof.items() if k!='binary_sha256'}):
                self.assertEqual(journey.validate_report(report,binary)['receipts'],6)
                mutations=[lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
                    lambda r:r.update(release_ready=True),lambda r:r.update(eligible_for_grading=True),
                    lambda r:r.update(cleanup=False),lambda r:r.update(boundary_lock={}),lambda r:r.update(fixture_lock={}),
                    lambda r:r['identity'].update(source_after={}),lambda r:r['identity'].update(binary_sha256='0'*64),
                    lambda r:r['identity'].update(environment={}),lambda r:r['identity'].update(environment=dict(go=1)),
                    lambda r:r['source_provenance']['after'].update(source_sha='b'*40),
                    lambda r:r['variants'].pop(),lambda r:r['variants'].reverse(),
                    lambda r:r['variants'][0].update(failure='failed'),lambda r:r['variants'][0].update(semantic_score=99),
                    lambda r:r['variants'][0].update(owned_processes_joined=False),
                    lambda r:r['variants'][0].update(cleanup=dict(owned_processes_joined=1,disposable_root_removed=True)),
                    lambda r:r['variants'][0].update(assertions=dict(observed=1)),
                    lambda r:r['variants'][1].update(state=copy.deepcopy(r['variants'][0]['state'])),
                    lambda r:r['variants'][0]['state']['runs'].append(copy.deepcopy(r['variants'][0]['state']['runs'][0])),
                    lambda r:r.update(rejection_probes={})]
                for mutate in mutations:
                    changed=copy.deepcopy(report);mutate(changed)
                    with self.assertRaises(RuntimeError):journey.validate_report(changed,binary)
                with patch.object(journey,'observe',return_value=dict(observed=False)):
                    with self.assertRaises(RuntimeError):journey.validate_report(report,binary)

    def test_manifest_rejects_tamper_path_escape_and_boolean_sizes(self):
        report=dict(status='mechanical_pass',cleanup=True,variants=[dict(payload='NATIVE_SENTINEL'*1000)],
            identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/'gc16.json';journey.write_report(path,report)
            manifest=json.loads(path.read_text());raw=path.parent/manifest['native']['path']
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('NATIVE_SENTINEL',path.read_text())
            self.assertEqual(journey.read_report(path),report)
            for mutate in (lambda m:m['summary'].update(receipts=30),lambda m:m['summary'].update(cleanup=1),
                    lambda m:m['summary'].update(semantic_score=100),lambda m:m['native'].update(sha256='0'*64),
                    lambda m:m['native'].update(path='../outside.json'),lambda m:m['native'].update(path=str(raw)),
                    lambda m:m.update(native_bytes=True),lambda m:m.update(native_uncompressed_bytes=True),
                    lambda m:m.update(native_uncompressed_bytes=1),lambda m:m.update(version=True)):
                changed=copy.deepcopy(manifest);mutate(changed);path.write_text(json.dumps(changed))
                with self.assertRaises((RuntimeError,ValueError)):journey.read_report(path)
            path.write_text(json.dumps(manifest));saved=raw.with_suffix('.saved');raw.rename(saved);raw.symlink_to(saved)
            with self.assertRaises(ValueError):journey.read_report(path)
            raw.unlink();saved.rename(raw);raw.write_bytes(raw.read_bytes()+b' ')
            with self.assertRaises(ValueError):journey.read_report(path)

    def test_unscripted_fragment_contains_only_original_question(self):
        import unscripted_cases as producer
        for variant in journey.variants():
            value=producer.inputs('GC-16',variant['id'])
            self.assertEqual(value['events'],[dict(at=value['clock'],text=variant['text'])])
            self.assertEqual(value['context'],'')
            self.assertEqual(value['lock'],journey.load())
            for hidden in ('Capabilities:','D1','2024-03-01','Atlas'):
                self.assertNotIn(hidden,value['context']+json.dumps(value['events'],ensure_ascii=False))
        self.assertEqual(producer.matrix()['GC-16']['adapter'],'conversation_fragment')
        self.assertFalse(producer.matrix()['GC-16']['eligible_for_grading'])
        with self.assertRaisesRegex(ValueError,'unknown variant'):producer.inputs('GC-16','invented')

    def test_exact_main_file_publish_and_sent_delivery_join(self):
        ref='artifact:report';publishes=[dict(run_id='root'),dict(run_id='final')]
        value=dict(conversation=[],generations=[],runs=[dict(run_id=r,kind=k) for r,k in
            (('root','worker'),('initial','main'),('final','main'))],artifacts=[dict(run_id='root')],deliveries=[])
        for run,text in (('initial','starting'),('final','done')):
            event=run+'-generation';message='reply:'+event
            parts=[dict(type='text',text=text)]
            if run=='final':parts.append(dict(type='file',artifact_ref=ref))
            value['conversation'].append(dict(message_id=message,source_event_id=event,role='assistant',source='main',content=parts))
            value['generations'].append(dict(generation_id=event,run_id=run,result_json=json.dumps(dict(text=text,tool_calls=[]))))
            value['deliveries'].append(dict(delivery_id='delivery:'+message,message_id=message,
                idempotency_key='delivery:'+message,status='sent'))
        self.assertTrue(journey.delivered_report(value,ref,publishes))
        mutations=[lambda v:v['conversation'][-1]['content'][-1].update(artifact_ref='artifact:wrong'),
            lambda v:v['conversation'][-1]['content'].pop(),
            lambda v:v['conversation'][-1]['content'].append(dict(type='file',artifact_ref=ref)),
            lambda v:v['conversation'][-1]['content'][-1].update(type='image'),
            lambda v:v['conversation'][0]['content'].append(dict(type='file',artifact_ref=ref)),
            lambda v:v['conversation'][-1].update(source_event_id='initial-generation'),
            lambda v:v['deliveries'][-1].update(status='pending'),
            lambda v:v['deliveries'][-1].update(idempotency_key='wrong'),
            lambda v:v['runs'][-1].update(kind='worker')]
        for mutate in mutations:
            changed=copy.deepcopy(value);mutate(changed)
            self.assertFalse(journey.delivered_report(changed,ref,publishes))
        for changed in ([dict(run_id='root'),dict(run_id='initial')],
                        [dict(run_id='other-root'),dict(run_id='final')]):
            self.assertFalse(journey.delivered_report(value,ref,changed))

    @unittest.skipUnless(os.environ.get('ERII_GC16_NATIVE_ROW'),'requires retained actual CLI receipt')
    def test_actual_corruption_recomputed_without_trusting_flags(self):
        row=json.loads(Path(os.environ['ERII_GC16_NATIVE_ROW']).read_text())
        self.assertTrue(all(journey.observe(row).values()))
        probes=journey.rejection_probes(row)
        self.assertEqual(len(probes),38)
        self.assertTrue(all(probes.values()))

    @unittest.skipUnless(os.environ.get('ERII_GC16_NATIVE_ROW'),'requires retained actual CLI receipt')
    def test_wrong_delivered_reference_rejects_synchronized_native_receipt(self):
        row=json.loads(Path(os.environ['ERII_GC16_NATIVE_ROW']).read_text())
        self.assertTrue(all(journey.observe(row).values()))
        final=row['state']['conversation'][-1]
        next(p for p in final['content'] if p['type']=='file')['artifact_ref']='artifact:'+'0'*64
        row['replay_state']=copy.deepcopy(row['state'])
        row['assertions']=journey.observe(row)
        self.assertFalse(all(row['assertions'].values()))

    def test_gate_runs_with_required_mcp_sandbox(self):
        self.assertEqual([(name,g.attempts) for name,gates in suite.SHARDS.items()
            for g in gates if g.script=='verify_gc16.py'],[('communication',1)])


if __name__=='__main__': unittest.main()
