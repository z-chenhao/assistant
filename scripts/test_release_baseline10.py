"""Frozen Memory/event/progress/organization audit integrity, never a release PASS."""
import copy
from collections import Counter

import test_release_package
from release_package import frozen_digest


class Baseline10Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-10.json'

    def test_both_seals_preserve_identity_order_and_original_location(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '33ebc44545bf9d3718771a4303266cca26f1923c1f234ee2e30474b5067fa911')
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         'd312f7fa24d257858397fcee94246eb2c01910fb249ec971c7a874829c31ef0f')
        self.assertEqual([b['line'] for b in self.ledger['frozen_blocks']][::79], [1739, 1890])
        for old, sealed, review in zip(self.ledger['original_frozen_blocks'],
                                       self.ledger['frozen_blocks'], self.ledger['reviews']):
            self.assertEqual((old['source'], old['section'], old['sha256']),
                             (sealed['source'], sealed['section'], sealed['sha256']))
            self.assertEqual(old['line'] + 10, sealed['line'])
            self.assertEqual(review['original_line'], old['line'])
        for mutation in ('manifest', 'line', 'missing', 'review_location'):
            with self.subTest(mutation=mutation):
                self.setUp()
                if mutation == 'manifest': self.ledger['original_manifest_sha256'] = '0' * 64
                if mutation == 'line': self.ledger['original_frozen_blocks'][0]['line'] += 1
                if mutation == 'missing': self.ledger['original_frozen_blocks'].pop()
                if mutation == 'review_location': self.ledger['reviews'][0]['original_line'] += 1
                with self.assertRaises(ValueError): self.run_audit()

    def test_mandatory_routes_cannot_be_deleted_or_silently_exempted(self):
        self.assertEqual(Counter(k for row in self.ledger['reviews']
                                 for k in row['obligation_kinds'].values()),
                         dict(required_v1=281, policy=13, background=15, deferred=9))
        for row in self.ledger['reviews']:
            self.assertEqual({o for o, k in row['obligation_kinds'].items()
                              if k in ('required_v1', 'policy')},
                             {r['obligation'] for r in row['remaining']})
        original = copy.deepcopy(self.ledger)
        for mutation in ('scope', 'route', 'invalid_scope', 'exemption'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                row = self.ledger['reviews'][0]
                duty = row['obligations'][0]
                if mutation == 'scope': del row['obligation_kinds']
                if mutation == 'route': row['remaining'].pop()
                if mutation == 'invalid_scope': row['obligation_kinds'][duty] = 'verified'
                if mutation == 'exemption':
                    row['obligation_kinds'][duty] = 'background'
                    row['remaining'].pop(0)
                with self.assertRaises(ValueError): self.run_audit()

    def test_missing_production_paths_are_separate_from_passing_storage_contracts(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, gap, issue in [(1739, 'G-curator', 'ERII-353'),
                                 (1741, 'G-aging', 'ERII-354'),
                                 (1748, 'G-aging', 'ERII-354'),
                                 (1808, 'G-progress', 'ERII-355'),
                                 (1810, 'G-progress', 'ERII-355'),
                                 (1840, 'G-layout', 'ERII-356'),
                                 (1890, 'G-layout', 'ERII-356'),
                                 (1801, 'G-trace', 'ERII-357')]:
            self.assertIn(gap, {r['gap'] for r in rows[line]['remaining']})
            self.assertEqual(self.ledger['gaps'][gap]['issue'], issue)
        self.assertEqual(rows[1808]['disposition'], 'evidence_gap')
        self.assertIn('source-inspection', rows[1808]['observations'])
        self.assertIn('不是产品PASS', self.ledger['observations']['source-inspection']['limits'])
        self.assertEqual(rows[1818]['disposition'], 'background')
        self.assertEqual(rows[1819]['disposition'], 'background')
        self.assertEqual(rows[1869]['disposition'], 'background')
        # Explicit future migrations cannot exempt the present adapter/storage.
        for line in (1757, 1773, 1779, 1793, 1821, 1829, 1834):
            self.assertTrue(rows[line]['remaining'])
            self.assertTrue(rows[line]['exemptions'])
        self.assertEqual(len(rows[1778]['obligations']), 12)
        self.assertEqual(len(rows[1811]['obligations']), 11)
        self.assertEqual(len(rows[1840]['obligations']), 29)
        self.assertEqual(self.run_audit()['verified_requirements'], 0)
