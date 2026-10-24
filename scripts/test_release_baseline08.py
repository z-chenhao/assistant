"""Frozen Security/Computer/CUA dispositions through the maintained consumer."""
import copy
from collections import Counter

import test_release_package
from release_package import frozen_digest


class Baseline08Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-08.json'

    def test_original_and_resealed_identities_and_locations(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '12528dae09407c3daa20caf9c1d1c5166920d70b50a3346b4ab875cd93d077e8')
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         '22f68315002ca724f5ce132ee02b9a8c1678f19d4decc3101c42bfa541cb30ee')
        self.assertEqual([b['line'] for b in self.ledger['frozen_blocks']][::79], [1259, 1526])
        for old, sealed, review in zip(self.ledger['original_frozen_blocks'],
                                       self.ledger['frozen_blocks'], self.ledger['reviews']):
            self.assertEqual(old['sha256'], sealed['sha256'])
            self.assertEqual(old['line'] + 10, sealed['line'])
            self.assertEqual(review['original_line'], old['line'])
        for mutation in ('hash', 'line', 'drop', 'review_location'):
            with self.subTest(mutation=mutation):
                self.setUp()
                if mutation == 'hash': self.ledger['original_manifest_sha256'] = '0' * 64
                if mutation == 'line': self.ledger['original_frozen_blocks'][0]['line'] += 1
                if mutation == 'drop': self.ledger['original_frozen_blocks'].pop()
                if mutation == 'review_location': self.ledger['reviews'][0]['original_line'] += 1
                with self.assertRaises(ValueError): self.run_audit()

    def test_mandatory_duties_keep_routes_and_exemptions_keep_reasons(self):
        scopes = Counter(k for row in self.ledger['reviews']
                         for k in row['obligation_kinds'].values())
        self.assertEqual(scopes, dict(required_v1=366, policy=18, background=7, deferred=11))
        for row in self.ledger['reviews']:
            required = {o for o, kind in row['obligation_kinds'].items()
                        if kind in ('required_v1', 'policy')}
            self.assertEqual(required, {r['obligation'] for r in row['remaining']})
        original = copy.deepcopy(self.ledger)
        for mutation in ('missing_scope', 'missing_route', 'invalid_kind', 'unexplained'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                row = self.ledger['reviews'][0]
                duty = row['obligations'][0]
                if mutation == 'missing_scope': del row['obligation_kinds']
                if mutation == 'missing_route': row['remaining'].pop()
                if mutation == 'invalid_kind': row['obligation_kinds'][duty] = 'verified'
                if mutation == 'unexplained':
                    row['obligation_kinds'][duty] = 'background'
                    row['remaining'].pop(0)
                with self.assertRaises(ValueError): self.run_audit()

    def test_real_gaps_cannot_be_replaced_by_fixture_success(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, gap, issue in [(1341, 'G-readiness', 'ERII-347'),
                                 (1326, 'G-browser-fixture', 'ERII-348'),
                                 (1445, 'G-api', 'ERII-349'),
                                 (1453, 'G-api', 'ERII-349'),
                                 (1482, 'G-tab-policy', 'ERII-350')]:
            self.assertIn(gap, {r['gap'] for r in rows[line]['remaining']})
            self.assertEqual(self.ledger['gaps'][gap]['issue'], issue)
        # Preserve the large typed contracts, not just a title-level review.
        for line, minimum in [(1349, 20), (1392, 29), (1453, 16), (1506, 6)]:
            self.assertGreaterEqual(len(rows[line]['obligations']), minimum)
        # Warm is optional; Durable, real compatibility and permission safety
        # stay mandatory. Table separators remain inspectable source blocks.
        self.assertTrue(rows[1290]['remaining'])
        self.assertTrue(rows[1317]['remaining'])
        self.assertEqual(rows[1525]['disposition'], 'background')
        self.assertFalse(self.run_audit()['release_ready'])
