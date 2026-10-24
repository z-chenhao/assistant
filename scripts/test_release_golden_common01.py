"""Frozen common Golden conditions, rubric, hard failures and finite owners."""
from collections import Counter
import copy
import re

import test_release_package
from release_package import frozen_digest


class GoldenCommon01Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/golden-common-01.json'
    expected_blocks = 80

    def test_exact_original_seal_ends_at_gc04_without_inheriting_legacy_pass(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '3421f37dee8becf70cb964d7eb7a998f3a4bb19c006d0e2bcb3bbbb7cabd9d9e')
        rows = self.ledger['reviews']
        self.assertEqual((rows[0]['section'], rows[0]['line']), ('1', 10))
        self.assertEqual((rows[-1]['section'], rows[-1]['line']), ('6', 137))
        self.assertTrue(all(r['source'] == 'evals/golden-cases.md' for r in rows))
        self.assertEqual(rows[-1]['text'].split('|')[1].strip(), 'GC-04')
        self.assertTrue(all(r['current_line'] == r['line'] for r in rows))
        self.assertNotIn('original_manifest_sha256', self.ledger)

    def test_every_mandatory_duty_survives_and_background_has_reasons(self):
        kinds = Counter(k for r in self.ledger['reviews'] for k in r['obligation_kinds'].values())
        self.assertEqual(kinds, dict(required_v1=252, policy=50, background=57))
        self.assertEqual(sum(len(r['obligations']) for r in self.ledger['reviews']), 359)
        self.assertEqual({g['issue'] for g in self.ledger['gaps'].values()},
                         {f'ERII-{n}' for n in list(range(247, 260)) + list(range(323, 333)) + [343]})
        row = self.ledger['reviews'][0]
        row['remaining'].pop(0)
        with self.assertRaises(ValueError): self.run_audit()
        self.ledger = copy.deepcopy(self.original)
        row = next(r for r in self.ledger['reviews'] if r['exemptions'])
        row['exemptions'].clear()
        with self.assertRaises(ValueError): self.run_audit()

    def test_common_fixture_and_variants_keep_all_twenty_finite_case_owners(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        for line, duties in {56: ['每个Case有确定性版本'], 61: ['L1任务完成'],
                             70: ['每场景至少3个措辞变体', '每场景至少2个噪声变体',
                                  '不得只调优唯一Prompt']}.items():
            for duty in duties:
                owners = {r['gap'] for r in rows[line]['remaining'] if r['obligation'] == duty}
                self.assertEqual(owners, {f'GC{n:02}' for n in range(1, 21)})
        self.assertEqual(set(rows[68]['obligations']), {
            '每次固定ModelSnapshot', '每次固定Agent配置', '每次固定Tool版本',
            '每次固定Skill版本', '每次固定数据快照', '每次固定随机种子'})
        self.assertEqual(set(rows[69]['obligations']), {
            'L0/L1保存Run树', '保存Task状态', '保存ToolInvocation', '保存Approval',
            '保存Outbox', '保存Artifact清单', '保存最终外部状态快照'})
        self.assertIn('Provider RNG seed不可控', rows[68]['proof_limit'])

    def test_all_eight_rubrics_and_hard_failures_remain_distinct(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        for line, dimension, weight in zip(range(87, 95), 'OFPATSRC', [25, 15, 10, 10, 15, 10, 10, 5]):
            self.assertIn(f'{dimension}权重{weight}', rows[line]['obligations'])
        self.assertEqual([len(rows[line]['obligations']) for line in range(102, 110)],
                         [6, 6, 7, 5, 6, 3, 4, 3])
        self.assertEqual(set(rows[107]['obligations']), {
            'Durable Replay不得重做已完成Child', 'Durable Replay不得重做已完成Tool',
            '不得重试unknown外部副作用'})
        self.assertEqual(set(rows[109]['obligations']), {
            'Forget后不得依赖已删Memory回答', '当前输入独立合法证据例外', '其他合法来源独立证据例外'})
        self.assertEqual(set(rows[114]['obligations']),
                         {f'GC{n}硬断言连续5次' for n in [12, 13, 17, 19, 20]} |
                         {f'SC{n:02}硬断言连续5次' for n in range(3, 8)})
        self.assertTrue(all(self.ledger['gaps'][r['gap']]['issue'] == 'ERII-343'
                            for r in rows[108]['remaining']))

    def test_source_bound_mechanisms_are_ungraded_and_all_native_bytes_indexed(self):
        self.assertEqual(set(self.ledger['observations']), {
            'scoring-contracts', 'fixture-contracts', 'release-contracts',
            'judge-packets', 'unscripted-contracts', 'unscripted-cli'})
        obs = self.ledger['observations']['unscripted-cli']
        self.assertEqual(obs['binary']['source_sha'], self.ledger['audited_base'])
        self.assertEqual(len(obs['files']), 34)  # log, summary, 32 retained raw receipts
        self.assertTrue(any(f['path'].endswith('/success/native.json') for f in obs['files']))
        self.assertEqual(self.ledger['merged_prs']['32']['state'], 'MERGED')
        self.assertEqual(self.ledger['merged_prs']['46']['state'], 'MERGED')
        text = (self.root / 'docs/audits/golden-common-01.md').read_text()
        links = set(re.findall(
            r'\[(ERII-\d+)\]\((https://multica\.ai/erii-studio/issues/[0-9a-f-]+)\)', text))
        self.assertEqual(links, {(g['issue'], g['issue_url']) for g in self.ledger['gaps'].values()})
