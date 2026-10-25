"""GC-16 corpus boundary and oracle must reject missing/unsupported evidence."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

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

    def test_gate_runs_with_required_mcp_sandbox(self):
        self.assertEqual([(name,g.attempts) for name,gates in suite.SHARDS.items()
            for g in gates if g.script=='verify_gc16.py'],[('communication',1)])


if __name__=='__main__': unittest.main()
