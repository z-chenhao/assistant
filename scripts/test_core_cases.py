import copy
from collections import Counter
import json
from pathlib import Path
import unittest

from case_runner import load_contract, score, variants
from core_fixtures import feedback, csv_text, CATEGORIES, SEVERITIES, EDGES, PAIRS
from core_boundaries import exercise
from core_conformance import TESTS, judge
from core_report import aggregate

ROOT = Path(__file__).resolve().parents[1]


class CoreTests(unittest.TestCase):
    def test_seed1701_exact_gold_and_independent_text_rows(self):
        rows, gold = feedback()
        self.assertEqual(Counter(r['category'] for r in gold), CATEGORIES)
        self.assertEqual(Counter(r['severity'] for r in gold), SEVERITIES)
        self.assertEqual([r['row_id'] for r in rows], [f'F{i:03d}' for i in range(1,101)])
        self.assertEqual(sum(bool(r['attachment_ref']) for r in rows),5)
        self.assertTrue(all(set(r)=={'row_id','text','attachment_ref'} for r in rows))
        for i, (c,s,text) in EDGES.items():
            self.assertEqual(rows[i-1]['text'],text)
            self.assertEqual(gold[i-1],dict(row_id=f'F{i:03d}',category=c,severity=s))
        groups = {}
        for row in rows:
            groups.setdefault(row['text'], []).append(int(row['row_id'][1:]))
        self.assertEqual(sorted(sorted(ids) for ids in groups.values() if len(ids) > 1), sorted([list(pair) for pair in PAIRS]))
        for a,b in PAIRS:
            self.assertEqual(rows[a-1]['text'],rows[b-1]['text'])
            self.assertNotEqual(rows[a-1]['row_id'],rows[b-1]['row_id'])
        self.assertEqual((ROOT/'evals/fixtures/core-v1/feedback_100.csv').read_text(),csv_text(rows))
        self.assertEqual(json.loads((ROOT/'evals/fixtures/core-v1/feedback_gold.json').read_text()),gold)

    def test_fixture_lock_and_six_distinct_variants(self):
        data,policy,lock=load_contract(ROOT,'core-v1')
        self.assertEqual([c['case_id'] for c in data['cases']],['GC-17','GC-19','GC-20'])
        for case in data['cases']:
            generated=list(variants(data,case))
            self.assertEqual(len({v['fixture_digest'] for v in generated}),6)
            self.assertEqual(sum(v['noise_characters']>0 for v in generated),2)
        self.assertIn('evals/fixtures/engineering-v1/internal/run/service.go',lock['files'])
        self.assertEqual(score(policy,{'exact_gold':False},[],{'O':4},['O'])['final_score'],0)

    def test_conformance_skip_missing_or_failure_never_pass(self):
        events=[dict(Test=t,Action='pass') for tests in TESTS.values() for t in tests]
        self.assertTrue(judge(events,0))
        self.assertFalse(judge(events[1:],0))
        self.assertFalse(judge(events,1))
        for action in ['skip','fail']:
            bad=copy.deepcopy(events);bad[0]['Action']=action
            self.assertFalse(judge(bad,0))
        boundaries=exercise()
        self.assertEqual(set(boundaries),{'SC-08','SC-11'})
        self.assertTrue(all(all(row['assertions'].values()) for row in boundaries.values()))
        self.assertTrue(all(row['status']=='boundary_fake' for row in boundaries.values()))

    def reports(self):
        results=[dict(case_id=case,variant_id=f'v{i}',fixture_digest=f'd-{case}-{i}',
                      status='fragment_pass',assertions={'state':True},hard_failure=False)
                 for case in ['GC-17','GC-19','GC-20'] for i in range(6)]
        return [copy.deepcopy(dict(cleanup=True,binary_sha256='bin',dataset_digest='data',contract_lock={},results=results,
                                   conformance=dict(passed=True,requirements={},boundary_fakes={}),
                                   ci_cancellation=dict(passed=True))) for _ in range(5)]

    def test_five_run_summary_rechecks_hard_assertions_and_never_invents_scores(self):
        reports=self.reports()
        good=aggregate(reports)
        self.assertTrue(good['passed'])
        self.assertEqual(good['cases']['GC-17']['hard_assertions']['pass^5'],1)
        self.assertIsNone(good['cases']['GC-17']['final_score'])
        self.assertTrue(all(v is None for v in good['cases']['GC-17']['dimensions'].values()))
        reports[-1]['results'][0]['assertions']['state']=False
        reports[-1]['results'][0]['final_score']=100
        bad=aggregate(reports)
        self.assertFalse(bad['passed'])
        self.assertEqual(bad['cases']['GC-17']['hard_assertions']['pass@1'],1)
        self.assertLess(bad['cases']['GC-17']['hard_assertions']['pass^5'],1)

    def test_summary_rejects_missing_duplicate_and_changed_evidence(self):
        for mutation in ['missing_run','missing_variant','duplicate','binary','fixture','conformance','cleanup','assertions','ci_cancel_missing','ci_cancel_failed']:
            reports=self.reports()
            if mutation=='missing_run':reports.pop()
            elif mutation=='missing_variant':reports[0]['results'].pop()
            elif mutation=='duplicate':reports[0]['results'][-1]=reports[0]['results'][0]
            elif mutation=='binary':reports[0]['binary_sha256']='changed'
            elif mutation=='fixture':reports[0]['results'][0]['fixture_digest']='changed'
            elif mutation=='conformance':reports[0]['conformance']['passed']=False
            elif mutation=='cleanup':reports[0]['cleanup']=False
            elif mutation=='assertions':reports[0]['results'][0]['assertions']['extra']=True
            elif mutation=='ci_cancel_missing':reports[0].pop('ci_cancellation')
            elif mutation=='ci_cancel_failed':reports[0]['ci_cancellation']['passed']=False
            with self.subTest(mutation=mutation),self.assertRaises(ValueError):aggregate(reports)


if __name__=='__main__':unittest.main()
