"""Frozen Claim/quota/Skill/security audit through the maintained consumer."""
import copy

import test_release_package
from release_package import frozen_digest


class Baseline07Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-07.json'

    def test_original_and_resealed_assignment_keep_all_unchanged_identities(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         'd05cf38d322f6c69ad05e499d0caacafc0c520bec0e728170f05a3cf517fcd63')
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         'e4c7b80cf8886c25307f3b225c1970d4ff7234ca5f7e6a0cacb36ac4a0c5ace3')
        self.assertEqual([b['line'] for b in self.ledger['frozen_blocks']][::79], [1122, 1258])
        for old, sealed, review in zip(self.ledger['original_frozen_blocks'],
                                       self.ledger['frozen_blocks'], self.ledger['reviews']):
            self.assertEqual(old['sha256'], sealed['sha256'])
            self.assertEqual(old['line'] + 10, sealed['line'])
            self.assertEqual(review['original_line'], old['line'])
            self.assertEqual(review['current_line'], sealed['line'])
        self.assertEqual(sum(len(r['obligations']) for r in self.ledger['reviews']), 362)
        for mutation in ('hash', 'line', 'drop', 'review_location'):
            with self.subTest(mutation=mutation):
                self.setUp()
                if mutation == 'hash': self.ledger['original_manifest_sha256'] = '0' * 64
                if mutation == 'line': self.ledger['original_frozen_blocks'][0]['line'] += 1
                if mutation == 'drop': self.ledger['original_frozen_blocks'].pop()
                if mutation == 'review_location': self.ledger['reviews'][0]['original_line'] += 1
                with self.assertRaises(ValueError): self.run_audit()

    def test_no_mandatory_duties_disappear_into_background_or_deferral(self):
        for row in self.ledger['reviews']:
            self.assertEqual(set(row['obligations']), set(row['obligation_kinds']))
            required = {o for o, kind in row['obligation_kinds'].items()
                        if kind in ('required_v1', 'policy')}
            self.assertEqual(required, {r['obligation'] for r in row['remaining']})
        original = copy.deepcopy(self.ledger)
        for mutation in ('scope', 'unrouted', 'invalid_kind', 'unexplained_exemption'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                row = self.ledger['reviews'][0]
                duty = row['obligations'][0]
                if mutation == 'scope': del row['obligation_kinds']
                if mutation == 'unrouted': row['remaining'].pop()
                if mutation == 'invalid_kind': row['obligation_kinds'][duty] = 'verified'
                if mutation == 'unexplained_exemption':
                    row['obligation_kinds'][duty] = 'background'
                    row['remaining'].pop(0)
                with self.assertRaises(ValueError): self.run_audit()

    def test_observed_differences_remain_unmet_and_independently_routed(self):
        rows = {r['original_line']: r for r in self.ledger['reviews']}
        for line, gap, issue in [(1120, 'G-cancel', 'ERII-343'),
                                 (1128, 'G-capacity', 'ERII-344'),
                                 (1129, 'G-capacity', 'ERII-344'),
                                 (1172, 'G-layout', 'ERII-345'),
                                 (1193, 'G-layout', 'ERII-345'),
                                 (1218, 'G-performance', 'ERII-346')]:
            self.assertEqual(rows[line]['disposition'], 'evidence_gap')
            self.assertEqual({r['gap'] for r in rows[line]['remaining']}, {gap})
            self.assertEqual(self.ledger['gaps'][gap]['issue'], issue)
        self.assertEqual(rows[1209]['disposition'], 'evidence_gap')
        self.assertEqual({r['gap'] for r in rows[1209]['remaining']}, {'G-model'})
        self.assertEqual(len(rows[1172]['obligations']), 15)
        # Reference-table left columns are background. Right-side product
        # choices remain mandatory; the table cannot exempt its own duties.
        for line in range(1149, 1157):
            self.assertIn('background', rows[line]['obligation_kinds'].values())
            self.assertIn('required_v1', rows[line]['obligation_kinds'].values())
            self.assertTrue(rows[line]['remaining'])
        report = self.run_audit()
        self.assertFalse(report['release_ready'])
        self.assertEqual(report['verified_requirements'], 0)
