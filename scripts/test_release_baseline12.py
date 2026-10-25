"""Bounded error/config/startup audit; corruption rejects, never release PASS."""
import copy
import re

import test_release_package
from release_package import frozen_digest, identity


class Baseline12Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-12.json'

    def test_reseal_only_moves_all_eighty_unchanged_content_identities(self):
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         '81e65f75d0be884021e2fbf9fc2a13a99613e2bd0cd40d52e6a26b8a3c63cfab')
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         'aece4cdba5e23a174caf8c4ee1f1b6ae65679bd3b17bd0274304446d21154d65')
        self.assertEqual(list(map(identity, self.ledger['original_frozen_blocks'])),
                         list(map(identity, self.ledger['frozen_blocks'])))
        self.assertEqual([r['current_line'] for r in self.ledger['reviews']][::79],
                         [1995, 2153])
        self.ledger['reviews'][0]['original_line'] += 1
        with self.assertRaises(ValueError): self.run_audit()

    def test_fenced_configuration_cannot_hide_a_key_or_its_current_owner(self):
        original = copy.deepcopy(self.ledger)
        for mutation in ('map', 'duty', 'empty', 'extra', 'original_hash'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                row = next(r for r in self.ledger['reviews'] if r['current_line'] == 2040)
                self.assertEqual(len(row['config_mapping']), 28)
                key = 'llm.catalog_cache_ttl'
                if mutation == 'map': del row['config_mapping'][key]
                if mutation == 'duty': row['obligations'].remove(key)
                if mutation == 'empty': row['config_mapping'][key] = ''
                if mutation == 'extra': row['config_mapping']['llm.invented'] = 'fabricated'
                if mutation == 'original_hash': self.ledger['original_frozen_blocks'][0]['sha256'] = '0' * 64
                with self.assertRaises(ValueError): self.run_audit()

    def test_consumer_methods_and_mixed_deferred_auth_stay_visible(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        self.assertEqual(len([r for r in rows.values() if 'interface_mapping' in r]), 14)
        self.assertIn('RunStore.Claim', rows[2094]['interface_mapping'])
        self.assertIn('ActorStore', rows[2094]['interface_mapping']['RunStore.Claim'])
        self.assertIn('DeliveryService.Enqueue', rows[2102]['interface_mapping'])
        deferred = {o for r in rows.values() for o, kind in r['obligation_kinds'].items()
                    if kind == 'deferred'}
        self.assertEqual(deferred, {'Device Code UI-neutral事件', 'Device Authorization延期',
                                    'Device Authorization不计M1阻塞或通过'})
        for row in rows.values():
            self.assertEqual({o for o, k in row['obligation_kinds'].items()
                              if k in ('required_v1', 'policy')},
                             {r['obligation'] for r in row['remaining']})
        for line in (2089, 2094, 2099, 2102):
            with self.subTest(line=line):
                self.setUp()
                row = next(r for r in self.ledger['reviews'] if r['current_line'] == line)
                row['interface_mapping'].pop(next(iter(row['interface_mapping'])))
                with self.assertRaises(ValueError): self.run_audit()

    def test_concrete_differences_have_separate_finite_routes_and_no_pass(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, duty, issue in [
                (2019, 'JS读取retryable', 'ERII-362'),
                (2019, 'JS读取可选details', 'ERII-362'),
                (2040, 'llm.catalog_cache_ttl', 'ERII-363'),
                (2040, 'limits.max_code_host_calls', 'ERII-344'),
                (2095, 'TaskService.Manage', 'ERII-364'),
                (2131, '不打扰进度聚合', 'ERII-355')]:
            route = next(r for r in rows[line]['remaining'] if r['obligation'] == duty)
            self.assertEqual(self.ledger['gaps'][route['gap']]['issue'], issue)
        self.assertIn('sync-error-probe-confirmed', rows[2019]['observations'])
        self.assertIn('retryableType=undefined', rows[2019]['observed'])
        self.assertIn('code-recovery-cli', rows[2153]['observations'])
        self.assertFalse(self.run_audit()['release_ready'])

    def test_document_links_match_canonical_routes(self):
        text = (self.root / 'docs/audits/baseline-12.md').read_text()
        links = set(re.findall(
            r'\[(ERII-\d+)\]\((https://multica\.ai/erii-studio/issues/[0-9a-f-]+)\)', text))
        self.assertEqual(links, {(g['issue'], g['issue_url'])
                                 for g in self.ledger['gaps'].values()})
