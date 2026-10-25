"""Frozen CUA transfer/context/Memory audit integrity, never a product PASS."""
import copy
from collections import Counter

import test_release_package
from release_package import frozen_digest


class Baseline09Tests(test_release_package.ReleasePackageTests):
    ledger_path = 'docs/audits/baseline-09.json'

    def test_original_and_adr006_seal_keep_exact_identity_and_order(self):
        self.assertEqual(frozen_digest(self.ledger['frozen_blocks']),
                         '680dd54a6cbc5587ceb348b70adda1956f38a0b03ffe9e622204e587991c5f46')
        self.assertEqual(frozen_digest(self.ledger['original_frozen_blocks']),
                         'e4c26626df402e3f0f7e9ecf731901b9b17e33fa25cd62049a350d6148fdab53')
        self.assertEqual([b['line'] for b in self.ledger['frozen_blocks']][::79], [1528, 1738])
        for old, sealed, review in zip(self.ledger['original_frozen_blocks'],
                                       self.ledger['frozen_blocks'], self.ledger['reviews']):
            self.assertEqual(old['sha256'], sealed['sha256'])
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

    def test_every_mandatory_duty_has_a_route_and_exemptions_have_reasons(self):
        scopes = Counter(k for row in self.ledger['reviews']
                         for k in row['obligation_kinds'].values())
        self.assertEqual(scopes, dict(required_v1=256, policy=5, background=12, deferred=11))
        for row in self.ledger['reviews']:
            required = {o for o, k in row['obligation_kinds'].items()
                        if k in ('required_v1', 'policy')}
            self.assertEqual(required, {r['obligation'] for r in row['remaining']})
        original = copy.deepcopy(self.ledger)
        for mutation in ('scope', 'route', 'invalid_scope', 'exemption'):
            with self.subTest(mutation=mutation):
                self.ledger = copy.deepcopy(original)
                row = self.ledger['reviews'][1]
                duty = row['obligations'][0]
                if mutation == 'scope': del row['obligation_kinds']
                if mutation == 'route': row['remaining'].pop()
                if mutation == 'invalid_scope': row['obligation_kinds'][duty] = 'verified'
                if mutation == 'exemption':
                    row['obligation_kinds'][duty] = 'background'
                    row['remaining'].pop(0)
                with self.assertRaises(ValueError): self.run_audit()

    def test_real_gaps_are_distinct_from_mechanical_success(self):
        rows = {r['current_line']: r for r in self.ledger['reviews']}
        for line, gap, issue in [(1531, 'G-api', 'ERII-349'),
                                 (1599, 'G-token', 'ERII-352'),
                                 (1601, 'G-token', 'ERII-352'),
                                 (1738, 'G-curator', 'ERII-353')]:
            self.assertIn(gap, {r['gap'] for r in rows[line]['remaining']})
            self.assertEqual(self.ledger['gaps'][gap]['issue'], issue)
        self.assertEqual(len(rows[1538]['obligations']), 16)
        self.assertGreaterEqual(len(rows[1612]['obligations']), 10)
        self.assertGreaterEqual(len(rows[1738]['obligations']), 13)
        self.assertEqual(rows[1528]['disposition'], 'deferred')
        self.assertEqual(rows[1622]['disposition'], 'background')
        self.assertTrue(rows[1612]['remaining'])
        self.assertTrue(rows[1613]['remaining'])
        self.assertIn('curator-probe', rows[1738]['observations'])
        self.assertIn('不是产品通过', self.ledger['observations']['curator-probe']['limits'])
        self.assertEqual(self.run_audit()['verified_requirements'], 0)

    def test_transfer_boundary_observation_binds_host_guest_and_json_rejection(self):
        row = next(r for r in self.ledger['reviews'] if r['current_line'] == 1574)
        observation = self.ledger['observations']['transfer-boundary']
        for source in ('internal/computer/macosvm/use.go', 'internal/computer/macosvm/transfer.py'):
            self.assertIn(source, row['references'])
            self.assertEqual(observation['sources'][source], self.ledger['references'][source]['sha256'])
        self.assertIn('transfer-boundary', row['observations'])
        self.assertEqual(observation['limit_bytes'], 8 * 1024 * 1024)
        cases = observation['observations']
        self.assertEqual({(c['operation'], c['input_bytes']) for c in cases},
                         {(op, size) for op in ('read', 'write') for size in (8388608, 8388609)})
        for case in cases:
            # The helper returns zero even on refusal; the JSON is authoritative.
            self.assertEqual(case['exit_code'], 0)
            if case['input_bytes'] == 8388608:
                self.assertEqual(case['result'], 'accepted')
                self.assertEqual(case['output_bytes'], case['input_bytes'])
            else:
                self.assertEqual(case['result'], {'error': 'PERMISSION_DENIED'})
        self.assertIn('Host WriteFile仅源码核对', row['proof_limit'])
        self.assertTrue(row['remaining'])
