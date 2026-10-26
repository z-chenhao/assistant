"""Frozen shared contracts, all schema fields and finite implementation owners."""
from collections import Counter
import re

import test_release_package
from release_package import frozen_digest


class GoldenCommon02Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/golden-common-02.json'

    def test_exact_seal_and_boundaries_preserve_all_eighty_sources(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '8915d2e0d8b4fd82564a0f2ce88988f645d7c24ded65f280b7edaa91f73802cc')
        rows = self.ledger['reviews']
        self.assertEqual((rows[0]['section'], rows[0]['line']), ('6', 138))
        self.assertEqual((rows[-1]['section'], rows[-1]['line']), ('15.产品材料', 1202))
        self.assertTrue(all(r['source'] == 'evals/golden-cases.md' and
                            r['line'] == r['current_line'] for r in rows))
        self.assertEqual(rows[0]['text'].split('|')[1].strip(), 'GC-05')
        self.assertEqual(Counter(k for r in rows for k in r['obligation_kinds'].values()),
                         dict(required_v1=415, policy=52, background=9))
        self.assertEqual(sum(len(r['obligations']) for r in rows), 476)
        self.assertTrue(all(r['remaining'] for r in rows if r['disposition'] != 'background'))

    def test_shared_facts_and_both_suggested_schemas_keep_every_leaf(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        fields = {
            445: {'user.id', 'user.name', 'user.timezone', 'user.locale', 'user.city',
                  'user.occupation', 'user.household', 'agent.id', 'agent.language',
                  'clock.mode', 'model.snapshot', 'security.external_writes_require_approval',
                  'security.secrets_are_never_model_visible'},
            1119: {'id', 'version', 'milestone', 'levels', 'fixture_ref', 'initial_events',
                   'injections.when', 'injections.action', 'required_assertions',
                   'forbidden_effects', 'scoring_dimensions', 'repetitions'},
            1137: {'case_id', 'case_version', 'run_id', 'model_snapshot', 'capability_snapshot',
                   'fixture_digest', 'seed', 'started_at', 'finished_at', 'final_score',
                   'hard_failure', 'assertions', 'metrics.wall_time_ms', 'metrics.model_calls',
                   'metrics.tool_calls', 'metrics.child_runs', 'metrics.user_questions',
                   'metrics.approvals', 'metrics.duplicate_effects', 'artifact_refs', 'trace_ref'},
        }
        for line, keys in fields.items():
            row = rows[line]
            self.assertEqual(set(row['field_mapping']), keys)
            self.assertEqual(set(row['obligations']), keys)
            self.assertTrue(all(row['field_mapping'].values()))
        self.assertEqual(rows[445]['obligation_kinds']['model.snapshot'], 'required_v1')
        self.assertIn('ungraded必须null', rows[1137]['field_mapping']['final_score'])
        self.assertEqual({r['gap'] for r in rows[445]['remaining']
                          if r['obligation'] == 'clock.mode'}, {'freeze-preparation'})

    def test_every_sc_and_exact_spend_protection_keep_unmet_mechanism(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        self.assertEqual([rows[line]['text'].split('|')[1].strip()
                          for line in range(1099, 1112)], [f'SC-{n:02}' for n in range(1, 14)])
        duties = set(rows[1113]['obligations'])
        self.assertTrue({'verify_spend连续5次', 'TestSpend连续5次',
                         '最终响应实际费用触界保存结果', '失败响应实际费用触界保存结果',
                         '不重复模型调用', '不产生语义分', '不代表M5重放通过'} <= duties)
        self.assertEqual({r['gap'] for r in rows[1113]['remaining']}, {'sc13'})
        self.assertEqual(self.ledger['gaps']['sc13']['issue'], 'ERII-368')
        self.assertEqual(self.ledger['gaps']['sc13']['responsibility'], 'implementation_prerequisite')
        self.assertIn('TestFollowupBatchWaitsForMainAndUserSteeringWins', rows[1108]['observed'])
        self.assertIn('cancel', {r['gap'] for r in rows[1103]['remaining']})
        self.assertIn('memory-conflict', {r['gap'] for r in rows[1106]['remaining']})

    def test_metrics_subduties_have_preparation_owner_separate_from_consumers(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        for line, count in [(1174, 4), (1175, 5), (1176, 4), (1177, 5), (1178, 5)]:
            self.assertEqual(len(rows[line]['obligations']), count)
            self.assertEqual({r['gap'] for r in rows[line]['remaining']}, {'metrics'})
            self.assertEqual(rows[line]['disposition'], 'evidence_gap')
        gap = self.ledger['gaps']['metrics']
        self.assertEqual(gap['issue'], 'ERII-367')
        self.assertEqual(gap['responsibility'], 'implementation_prerequisite')
        self.assertNotEqual(gap['issue'], self.ledger['gaps']['models']['issue'])
        self.assertNotEqual(gap['issue'], self.ledger['gaps']['release']['issue'])

    def test_native_limits_background_and_finite_routes_stay_inspectable(self):
        rows = {r['line']: r for r in self.ledger['reviews']}
        self.assertIn('不是邮件/文件噪声', rows[1076]['observed'])
        self.assertIn('不等于真正重排非关键事件', rows[1077]['observed'])
        for line in (1085, 1086, 1097, 1098, 1196, 1200, 1201, 1202):
            self.assertEqual(rows[line]['disposition'], 'background')
            self.assertTrue(rows[line]['reason'])
            self.assertFalse(rows[line]['remaining'])
        self.assertEqual(set(self.ledger['observations']),
                         {'scoring', 'release', 'fixtures', 'judge', 'unscripted',
                          'cli', 'systems', 'spend-followup'})
        cli = self.ledger['observations']['cli']
        self.assertEqual(cli['binary']['source_sha'], self.ledger['audited_base'])
        self.assertTrue(any(f['path'].endswith('/success/native.json') for f in cli['files']))
        text = (self.root / 'docs/audits/golden-common-02.md').read_text()
        links = set(re.findall(r'\[(ERII-\d+)\]\((https://multica\.ai/erii-studio/issues/[0-9a-f-]+)\)', text))
        self.assertEqual(links, {(g['issue'], g['issue_url']) for g in self.ledger['gaps'].values()})
