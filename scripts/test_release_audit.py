"""Source inventory integrity and refusal to manufacture release acceptance."""
import copy
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import release_audit as audit


class ReleaseAuditTests(unittest.TestCase):
    def setUp(self):
        self.ledger = audit.current_ledger(audit.ROOT)

    def test_import_preserves_complete_checkpoint_ledger_bytes(self):
        self.assertEqual(hashlib.sha256((audit.ROOT / 'docs/v1-release-ledger.json').read_bytes()).hexdigest(),
                         '05063f1a2f83327baa74ae971bd61f31485e66211ec76309b566ff1c747d4181')
        self.assertEqual(len(self.ledger['individual_reviews']), 122)

    def test_current_binding_preserves_every_historical_review_except_line(self):
        historical = json.loads((audit.ROOT / 'docs/v1-release-ledger.json').read_text())
        for before, after in zip(historical['individual_reviews'], self.ledger['individual_reviews']):
            self.assertEqual({k: v for k, v in before.items() if k != 'line'},
                             {k: v for k, v in after.items() if k != 'line'})
        report = audit.audit(audit.ROOT, self.ledger)
        complete = [u for u in report['source_units'] if 'task_manage(complete)' in u['text']]
        self.assertTrue(complete)
        self.assertTrue(all(u['disposition'] == 'pending_individual_review' for u in complete))

    def test_current_binding_rejects_drift_changed_reviews_and_ambiguity(self):
        for mutation in ('none', 'ledger', 'source', 'review', 'ambiguous', 'inventory'):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                (root / 'docs').mkdir()
                text = '## 3. Tasks\n\n- keep this obligation\n'
                row = audit.inventory(text)[0]
                ledger = dict(sources={'source.md': {'sha256': hashlib.sha256(text.encode()).hexdigest()}},
                              individual_reviews=[dict(source='source.md', **row)])
                original = json.dumps(ledger).encode()
                (root / 'docs/v1-release-ledger.json').write_bytes(original)
                binding = dict(version=1, ledger_sha256=hashlib.sha256(original).hexdigest(), sources={})
                current = '\n\n' + text
                if mutation == 'review': current = current.replace('keep this obligation', 'changed obligation')
                if mutation == 'ambiguous': current += '\n- keep this obligation\n'
                binding['sources']['source.md'] = hashlib.sha256(current.encode()).hexdigest()
                if mutation == 'inventory': binding['sources']['extra.md'] = '0' * 64
                (root / 'docs/v1-release-source-bindings.json').write_text(json.dumps(binding))
                (root / 'source.md').write_text(current)
                if mutation == 'source': (root / 'source.md').write_text(current + '\n- unbound change\n')
                if mutation == 'ledger': (root / 'docs/v1-release-ledger.json').write_bytes(original + b'\n')
                if mutation == 'none':
                    bound = audit.current_ledger(root)
                    self.assertEqual(bound['individual_reviews'][0]['line'], row['line'] + 2)
                    self.assertEqual((root / 'docs/v1-release-ledger.json').read_bytes(), original)
                else:
                    with self.assertRaises(ValueError): audit.current_ledger(root)

    def test_partial_reviews_cannot_certify_requirements_or_scores(self):
        # Even an added success-looking field cannot certify a release.
        self.ledger['release_ready'] = True
        report = audit.audit(audit.ROOT, self.ledger)
        self.assertGreater(len(report['source_units']), 1000)
        self.assertEqual(report['status'], 'inconclusive')
        self.assertFalse(report['release_ready'])
        self.assertEqual(report['individually_verified_requirements'], 0)
        self.assertEqual(report['individually_reviewed_blocks'], len(self.ledger['individual_reviews']))
        self.assertGreater(report['individually_reviewed_blocks'], 0)
        self.assertTrue(all(r['disposition'] in ('pending_individual_review', 'partial_mechanical', 'evidence_gap')
                            for r in report['source_units']))
        self.assertEqual(set(report['golden_cases']), {f'GC-{n:02}' for n in range(1, 21)})
        self.assertIsNone(report['scores']['suite_mean'])
        self.assertTrue(all(v is None for v in report['scores']['dimensions'].values()))

    def test_individual_review_rejects_stale_duplicate_missing_or_pass_claim(self):
        for mutation in ('stale', 'duplicate', 'missing', 'pass', 'reference', 'location'):
            ledger = copy.deepcopy(self.ledger)
            review = ledger['individual_reviews'][0]
            if mutation == 'stale': review['sha256'] = '0' * 64
            if mutation == 'duplicate': ledger['individual_reviews'].append(copy.deepcopy(review))
            if mutation == 'missing': del review['remaining']
            if mutation == 'pass': review['disposition'] = 'pass'
            if mutation == 'reference': review['tests'] = ['scripts/test_quote_case.py#missing_test']
            if mutation == 'location': review['line'] += 1
            with self.assertRaises(ValueError):
                audit.audit(audit.ROOT, ledger)

    def test_source_change_unmapped_section_missing_file_or_symbol_fails(self):
        for mutation in ('digest', 'section', 'file', 'symbol', 'pr'):
            with self.subTest(mutation=mutation):
                ledger = copy.deepcopy(self.ledger)
                if mutation == 'digest':
                    ledger['sources']['docs/technical-design.md']['sha256'] = '0' * 64
                elif mutation == 'section':
                    del ledger['sources']['docs/technical-design.md']['sections']['0']
                elif mutation == 'file':
                    ledger['areas']['main']['code'] = ['missing.go']
                elif mutation == 'symbol':
                    ledger['areas']['main']['tests'] = ['internal/agent/loop_test.go#TestAbsent']
                else:
                    ledger['areas']['main']['prs'] = ['999999']
                with self.assertRaises(ValueError):
                    audit.audit(audit.ROOT, ledger)

    def test_inventory_keeps_fenced_contracts_and_individual_list_rows(self):
        text = ('# title\n## 0. Rules\n- first\n  continuation\n- second\n\n'
                '```text\n## literal heading\n\ncontract\n```\n'
                '### 0.1 Child\n| field | value |\n| x | y |\n')
        rows = audit.inventory(text)
        self.assertEqual(len(rows), 5)
        self.assertEqual(rows[0]['text'], '- first\n  continuation')
        self.assertEqual(rows[1]['line'], 5)
        self.assertIn('## literal heading\n\ncontract', rows[2]['text'])
        self.assertEqual(rows[-1]['section'], '0.1')
        with self.assertRaises(ValueError):
            audit.inventory('## 0. Rules\n```text\nunterminated')
        with self.assertRaises(ValueError):
            audit.route('999', {'9': 'computer'})

    def test_cli_requires_complete_returns_nonzero_and_clears_stale_report(self):
        # Use the same operator boundary documented in the feature map.
        folder = audit.ROOT / 'evidence'
        folder.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='release-audit-test-', dir=folder) as temp:
            output = Path(temp) / 'result.json'
            command = [sys.executable, str(audit.ROOT / 'scripts/release_audit.py'),
                       '--evidence', str(output), '--require-complete']
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertFalse(json.loads(output.read_text())['release_ready'])
            ledger = copy.deepcopy(self.ledger)
            ledger['sources']['evals/golden-cases.md']['sha256'] = '0' * 64
            bad = Path(temp) / 'bad.json'
            bad.write_text(json.dumps(ledger))
            result = subprocess.run(command + ['--ledger', str(bad)], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(output.exists(), 'stale result survived a failed re-audit')


if __name__ == '__main__':
    unittest.main()
