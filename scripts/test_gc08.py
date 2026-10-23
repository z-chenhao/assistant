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


if __name__=='__main__':unittest.main()
