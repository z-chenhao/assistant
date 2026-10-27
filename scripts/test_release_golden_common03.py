"""Frozen reference exemptions and the mixed v1/deferred evaluation boundaries."""
from collections import Counter
import re

import test_release_package
from release_package import frozen_digest


class GoldenCommon03Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/golden-common-03.json'
    expected_blocks = 18

    def test_exact_eighteen_sources_and_all_subduties(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '64a9b52fbe9d60100a4263f8799611c925001895b41b0c6ddd1bcd15c400f60e')
        rows = self.ledger['reviews']
        self.assertEqual([r['line'] for r in rows],
                         [*range(1203, 1209), *range(1212, 1219), *range(1222, 1226), 1227])
        self.assertTrue(all(r['source'] == 'evals/golden-cases.md' and
                            r['current_line'] == r['line'] for r in rows))
        self.assertEqual(Counter(k for r in rows for k in r['obligation_kinds'].values()),
                         dict(background=14, policy=20, deferred=1, required_v1=1))
        self.assertEqual(sum(len(r['obligations']) for r in rows), 36)
        self.assertEqual(Counter(r['disposition'] for r in rows),
                         dict(background=13, policy=3, partial_mechanical=2))

    def test_every_reference_retains_an_explicit_non_normative_reason(self):
        for row in self.ledger['reviews'][:13]:
            self.assertEqual(row['disposition'], 'background')
            self.assertRegex(row['text'], r'^- \[.+\]\(https://.+\)$')
            self.assertEqual(len(row['obligations']), 1)
            self.assertEqual(set(row['exemptions']), set(row['obligations']))
            self.assertTrue(row['reason'])
            self.assertFalse(row['remaining'])
            self.assertFalse(row['observations'])
            self.assertIn('未获取网页', row['proof_limit'])

    def test_four_protected_effects_keep_both_fake_and_live_stop_boundaries(self):
        row = next(r for r in self.ledger['reviews'] if r['line'] == 1223)
        categories = ['真实付款', '真实对外发布', '真实医疗诊断', '金融交易']
        self.assertEqual(set(row['obligations']),
                         {c + suffix for c in categories for suffix in
                          ('的Golden Case仅在Fake/沙箱执行', '的Live Shadow停在最终审批前')})
        for duty in row['obligations']:
            self.assertEqual({r['gap'] for r in row['remaining'] if r['obligation'] == duty},
                             {'live' if 'Live Shadow' in duty else 'release'})
        self.assertIn('不能证明四类动作', row['proof_limit'])

    def test_deferred_peer_cooperation_does_not_exempt_the_v1_tree(self):
        row = next(r for r in self.ledger['reviews'] if r['line'] == 1224)
        self.assertEqual(row['obligation_kinds'], {
            '多个平等Erii的组织协作属于长期形态': 'deferred',
            '一期只测试一个Agent内的Root/Child Run树': 'required_v1'})
        self.assertEqual({r['obligation'] for r in row['remaining']},
                         {'一期只测试一个Agent内的Root/Child Run树'})
        self.assertEqual(row['observations'], ['tree'])
        self.assertIn('不是所有Case', row['proof_limit'])

    def test_observable_judging_and_case_admission_keep_their_limits(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        self.assertEqual(set(rows[1225]['obligations']), {
            '禁止依靠隐藏Chain-of-Thought打分', '只评可观察结果',
            '只评可观察状态', '只评可观察证据', '只评可观察轨迹事件'})
        self.assertIn('嵌套内容', rows[1225]['proof_limit'])
        self.assertEqual({r['gap'] for r in rows[1225]['remaining']}, {'models', 'release'})
        self.assertIn('至少引入新用户价值/权限边界/故障模式之一',
                      ''.join(rows[1227]['obligations']))
        self.assertIn('“或”条件', rows[1227]['proof_limit'])
        self.assertIn('不意味着永远禁止新增Case', rows[1227]['proof_limit'])
        self.assertEqual(set(self.ledger['gaps']), {'models', 'live', 'release'})
        self.assertEqual(set(self.ledger['observations']),
                         {'release', 'judge', 'inputs', 'scoring', 'tree'})
        text = (self.root / 'docs/audits/golden-common-03.md').read_text()
        links = set(re.findall(r'\[(ERII-\d+)\]\((https://multica\.ai/erii-studio/issues/[0-9a-f-]+)\)', text))
        self.assertEqual(links, {(g['issue'], g['issue_url']) for g in self.ledger['gaps'].values()})
