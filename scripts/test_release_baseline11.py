"""Frozen persistence audit: field coverage, ADR006 mapping and finite gap routes."""
import copy
from collections import Counter
import re

import test_release_package
from release_package import frozen_digest


class Baseline11Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-11.json'

    def test_original_seal_and_single_changed_task_require_independent_disposition(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '46a87bc1728c849591bf6c45905e56fc3e83a849264dacdda83d92e3d8b04fca')
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         '415396f2aa95fb071574356fe6cef9f03f8b81e729e0f8c9966bf9395886b534')
        self.assertEqual([r['line'] for r in self.ledger['reviews']][::79], [1891, 1994])
        changed = [(old, new) for old, new in zip(self.ledger['original_frozen_blocks'],
                                                self.ledger['frozen_blocks'])
                   if old['sha256'] != new['sha256']]
        self.assertEqual(len(changed), 1)
        self.assertEqual(changed[0][0]['line'], 1939)
        self.assertEqual(changed[0][1]['line'], 1949)
        task = next(r for r in self.ledger['reviews'] if r['line'] == 1949)
        self.assertIn('Task.completion_policy', task['obligations'])
        self.assertIn('invariant-contracts', task['observations'])
        self.assertIn('tasks-cli', task['observations'])
        original = copy.deepcopy(self.ledger)
        for mutation in ('inherit', 'authorization', 'old_hash', 'original_line', 'no_map'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                if mutation == 'inherit': self.ledger['reseal_mapping'][0]['conclusion_inherited'] = True
                if mutation == 'authorization': self.ledger['reseal_mapping'][0]['authorization_comment'] = 'invented'
                if mutation == 'old_hash': self.ledger['original_frozen_blocks'][42]['sha256'] = '0' * 64
                if mutation == 'original_line': self.ledger['reviews'][42]['original_line'] += 1
                if mutation == 'no_map': self.ledger['reseal_mapping'] = []
                with self.assertRaises(ValueError): self.run_audit()

    def test_every_minimum_field_including_optional_fields_has_its_own_duty(self):
        tables = [r for r in self.ledger['reviews'] if 'field_mapping' in r]
        self.assertEqual(len(tables), 19)
        for row in tables:
            record, raw = re.findall(r'`([^`]+)`', row['text'])
            self.assertEqual(set(row['field_mapping']), set(raw.split(', ')))
            self.assertEqual(set(row['obligations']), {record + '.' + f for f in raw.split(', ')})
        original = copy.deepcopy(self.ledger)
        for mutation in ('map', 'duty', 'optional', 'empty', 'extra'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                row = next(r for r in self.ledger['reviews'] if r['line'] == 1945)
                if mutation == 'map': del row['field_mapping']
                if mutation == 'duty': row['obligations'].pop()
                if mutation == 'optional': del row['field_mapping']['active_model_profile_id?']
                if mutation == 'empty': row['field_mapping']['agent_id'] = ''
                if mutation == 'extra': row['field_mapping']['invented'] = 'state.db:invented'
                with self.assertRaises(ValueError): self.run_audit()

    def test_known_contract_differences_keep_separate_finite_routes(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, duty, issue in [
                (1954, 'CodeExecutionState.source_ref', 'ERII-358'),
                (1954, 'CodeExecutionState.input_ref?', 'ERII-358'),
                (1955, 'CodeCall.output_ref?', 'ERII-358'),
                (1956, 'CodeStoreEntry.value_ref', 'ERII-358'),
                (1975, '每Agent仅一个非终态Main', 'ERII-359'),
                (1977, 'Active Profile引用ready', 'ERII-360'),
                (1993, '对应EventOutbox原子', 'ERII-361')]:
            route = next(r for r in rows[line]['remaining'] if r['obligation'] == duty)
            self.assertEqual(self.ledger['gaps'][route['gap']]['issue'], issue)
        self.assertIn('desired_state', rows[1975]['observed'])
        self.assertIn('degraded', rows[1977]['observed'])
        self.assertIn('abandon', rows[1993]['observed'])
        for row in self.ledger['reviews']:
            self.assertEqual({o for o, k in row['obligation_kinds'].items()
                              if k in ('required_v1', 'policy')},
                             {r['obligation'] for r in row['remaining']})
        self.assertEqual(Counter(k for r in self.ledger['reviews']
                                 for k in r['obligation_kinds'].values()),
                         dict(required_v1=444, policy=36, background=7))

    def test_document_links_match_canonical_issue_routes(self):
        text = (self.root / 'docs/audits/baseline-11.md').read_text()
        links = set(re.findall(
            r'\[(ERII-\d+)\]\((https://multica\.ai/erii-studio/issues/[0-9a-f-]+)\)', text))
        self.assertEqual(links, {(g['issue'], g['issue_url'])
                                 for g in self.ledger['gaps'].values()})
