"""Frozen M5–M7, acceptance and deferred-choice audit integrity."""
import copy
from collections import Counter
import re

import test_release_package
from release_package import frozen_digest


class Baseline13Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-13.json'
    expected_blocks = 58

    def test_exact_original_and_adr006_seals_keep_complete_independent_task_review(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '9d86d5732f7addecb9001ad5cf9c9b521f212373c5a57149e8c08cb95661570d')
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         '933565336b762b2dec8e8dc2b9459fda3d41dc3c8515bdc8ccce9bced7729e01')
        changed = [(o, n) for o, n in zip(self.ledger['original_frozen_blocks'],
                                         self.ledger['frozen_blocks'])
                   if o['sha256'] != n['sha256']]
        self.assertEqual(len(changed), 1)
        self.assertEqual((changed[0][0]['line'], changed[0][1]['line']), (2186, 2196))
        task = self.ledger['reviews'][23]
        self.assertEqual(set(task['obligations']), {
            'Task create符合四状态', 'Task append符合四状态', 'Task pause符合四状态',
            'Task cancel符合四状态', 'Task complete符合四状态', '失败Run不创建failed Task状态'})
        self.assertIn('45', task['prs'])
        self.assertIn('go-contracts', task['observations'])
        self.assertIn('internal/agent/sqlite/task_completion_test.go', task['references'])
        original = copy.deepcopy(self.ledger)
        for mutation in ('inherit', 'authorization', 'coordinator', 'old_hash', 'line', 'no_map'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                if mutation == 'inherit': self.ledger['reseal_mapping'][0]['conclusion_inherited'] = True
                if mutation == 'authorization': self.ledger['reseal_mapping'][0]['authorization_comment'] = 'invented'
                if mutation == 'coordinator': self.ledger['reseal_mapping'][0]['coordinator_comment'] = 'invented'
                if mutation == 'old_hash': self.ledger['original_frozen_blocks'][23]['sha256'] = '0' * 64
                if mutation == 'line': self.ledger['reviews'][23]['original_line'] += 1
                if mutation == 'no_map': self.ledger['reseal_mapping'] = []
                with self.assertRaises(ValueError): self.run_audit()

    def test_mixed_deferred_choices_preserve_current_mvp_and_policy_duties(self):
        kinds = Counter(k for r in self.ledger['reviews'] for k in r['obligation_kinds'].values())
        self.assertEqual(kinds, dict(required_v1=292, policy=32, deferred=26, background=2))
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, duty, kind in [
                (2235, 'Compaction最终阈值', 'deferred'),
                (2235, '先实现Compaction算法', 'required_v1'),
                (2236, '审批UI最终选型', 'deferred'),
                (2236, '未知副作用Tool默认审批', 'required_v1'),
                (2236, '未知副作用Tool replay_policy=never', 'required_v1'),
                (2237, 'Artifact最终配额', 'deferred'),
                (2237, 'MVP引用计数', 'required_v1'),
                (2237, 'MVP dry-run清理', 'required_v1'),
                (2240, '模型无预算查询Tool', 'policy'),
                (2241, '单独评审前禁止真实交易Tool', 'policy'),
                (2242, 'v1模块化单体', 'policy')]:
            self.assertEqual(rows[line]['obligation_kinds'][duty], kind)
        self.assertEqual(sum(len(r['remaining']) for r in rows.values()), 324)
        for row in rows.values():
            mandatory = {o for o, k in row['obligation_kinds'].items()
                         if k in ('required_v1', 'policy')}
            self.assertEqual({r['obligation'] for r in row['remaining']}, mandatory)
        # An exemption may not silently discard a current MVP obligation.
        row = rows[2237]
        row['remaining'] = [r for r in row['remaining'] if r['obligation'] != 'MVP引用计数']
        with self.assertRaises(ValueError): self.run_audit()

    def test_known_gaps_are_routed_to_the_existing_finite_owner(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, duty, issue in [
                (2194, 'Provider Bridge无领域DB权限', 'ERII-335'),
                (2199, '1000项在高上限内透明排队', 'ERII-344'),
                (2168, '高安全硬上限基于压测校准', 'ERII-346'),
                (2235, '先实现Compaction算法', 'ERII-352'),
                (2235, '先实现Compaction配置点', 'ERII-363'),
                (2227, '两模型相同运行上限', 'ERII-330'),
                (2226, '发布前桌面兼容L2', 'ERII-331'),
                (2215, '全Go race通过', 'ERII-332')]:
            route = next(r['gap'] for r in rows[line]['remaining'] if r['obligation'] == duty)
            self.assertEqual(self.ledger['gaps'][route]['issue'], issue)

    def test_every_secret_sink_and_operational_choice_survives_decomposition(self):
        security = self.ledger['reviews'][33]
        self.assertEqual(set(security['obligations']),
                         {s + '不入' + sink for s in ['API Key', 'access token', 'refresh token']
                          for sink in ['SQLite', 'YAML', 'Artifact', 'Trace', '错误', '日志']} |
                         {'模型不可读取CredentialRef', '模型不可伪造内部ID扩权'})
        ops = self.ledger['reviews'][53]
        self.assertEqual(len(ops['obligations']), 29)
        for duty in ['DLQ仅无正文receipt', '消费窗口32', '远程NATS强制TLS',
                     '还原先禁用', '启用前确认源实例退役', '启用前确认外部结果',
                     '正常关闭不改变Task状态', '备份不承诺外部副作用exactly-once']:
            self.assertEqual(ops['obligation_kinds'][duty], 'required_v1')

    def test_native_observations_and_document_routes_remain_explicit(self):
        self.assertEqual(set(self.ledger['observations']),
                         {'go-contracts', 'javascript', 'code-recovery', 'children'})
        for name in ('children', 'code-recovery'):
            observation = self.ledger['observations'][name]
            self.assertEqual(observation['binary']['source_sha'], self.ledger['audited_base'])
            self.assertIn('scripts/verify_' + name.replace('-', '_') + '.py', observation['command'])
        text = (self.root / 'docs/audits/baseline-13.md').read_text()
        links = set(re.findall(
            r'\[(ERII-\d+)\]\((https://multica\.ai/erii-studio/issues/[0-9a-f-]+)\)', text))
        self.assertEqual(links, {(g['issue'], g['issue_url']) for g in self.ledger['gaps'].values()})
