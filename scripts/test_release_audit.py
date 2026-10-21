"""Source inventory integrity and refusal to manufacture release acceptance."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import release_audit as audit


class ReleaseAuditTests(unittest.TestCase):
    def setUp(self):
        self.ledger = json.loads((audit.ROOT / 'docs/v1-release-ledger.json').read_text())

    def test_every_source_block_remains_pending_and_scores_unknown(self):
        # Even an added success-looking field cannot certify a release.
        self.ledger['release_ready'] = True
        report = audit.audit(audit.ROOT, self.ledger)
        self.assertGreater(len(report['source_units']), 1000)
        self.assertEqual(report['status'], 'inconclusive')
        self.assertFalse(report['release_ready'])
        self.assertEqual(report['individually_verified_requirements'], 0)
        self.assertTrue(all(r['disposition'] == 'pending_individual_review'
                            for r in report['source_units']))
        self.assertEqual(set(report['golden_cases']), {f'GC-{n:02}' for n in range(1, 21)})
        self.assertIsNone(report['scores']['suite_mean'])
        self.assertTrue(all(v is None for v in report['scores']['dimensions'].values()))

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
