import copy
import json
from pathlib import Path
import tempfile
import unittest

import ci_providers
from verify_suite import PROVIDER_PARTS, SHARDS, command, provider_gates


class ProvidersTests(unittest.TestCase):
    def fixture(self, root):
        suites = {}
        for part in PROVIDER_PARTS:
            folder = root / part
            folder.mkdir()
            suites[part] = dict(passed=True, source_stable=True, source_dirty=False,
                                source_sha='a'*40, binary_sha256='b'*64, workflow_sha='c'*40,
                                source_diff_sha256='d'*64, complete_inventory=False,
                                selected_gates=None, provider_part=part, elapsed_seconds=12,
                                shards={'providers': [dict(gate=g.name, attempt=i, returncode=0,
                                                           owned_group_drained=True, elapsed_seconds=1)
                                                      for g in provider_gates(part)
                                                      for i in range(1, g.attempts+1)]})
        return suites

    def write(self, root, suites):
        for part, suite in suites.items():
            (root / part / f'suite-providers-{part}.json').write_text(json.dumps(suite))

    def test_parts_keep_original_commands_counts_and_full_inventory(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            suites = self.fixture(root)
            self.write(root, suites)
            full = ci_providers.join(root)
            self.assertTrue(full['complete_inventory'])
            self.assertIsNone(full['provider_part'])
            self.assertEqual([(r['gate'], r['attempt']) for r in full['shards']['providers']],
                             [(g.name, i) for g in SHARDS['providers'] for i in range(1, g.attempts+1)])
        for part in PROVIDER_PARTS:
            for gate in provider_gates(part):
                original = next(g for g in SHARDS['providers'] if g.name == gate.name)
                self.assertEqual(gate, original)
                self.assertEqual(command(gate, 1, Path('/binary'), Path('/evidence')),
                                 command(original, 1, Path('/binary'), Path('/evidence')))
        self.assertEqual([g.name for g in provider_gates('models')], ['models'])
        with self.assertRaises(ValueError):
            provider_gates('unknown')

    def test_missing_failed_dirty_drifted_and_invalid_rounds_fail_closed(self):
        mutations = ('missing_part', 'missing_round', 'duplicate', 'reordered', 'failed_round',
                     'leaked_group', 'failed_suite', 'dirty', 'unstable', 'focused',
                     'wrong_part', 'wrong_shard', 'complete', 'missing_identity',
                     'source_sha', 'binary_sha256', 'workflow_sha', 'source_diff_sha256')
        for mutation in mutations:
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                suites = self.fixture(root)
                suite = suites['cli']
                rows = suite['shards']['providers']
                if mutation == 'missing_part': del suites['models']
                elif mutation == 'missing_round': rows.pop()
                elif mutation == 'duplicate': rows.append(copy.deepcopy(rows[-1]))
                elif mutation == 'reordered': rows[3], rows[4] = rows[4], rows[3]
                elif mutation == 'failed_round': rows[-1]['returncode'] = 130
                elif mutation == 'leaked_group': rows[-1]['owned_group_drained'] = False
                elif mutation == 'failed_suite': suite['passed'] = False
                elif mutation == 'dirty': suite['source_dirty'] = True
                elif mutation == 'unstable': suite['source_stable'] = False
                elif mutation == 'focused': suite['selected_gates'] = ['startup']
                elif mutation == 'wrong_part': suite['provider_part'] = 'models'
                elif mutation == 'wrong_shard': suite['shards'] = {'runtime': rows}
                elif mutation == 'complete': suite['complete_inventory'] = True
                elif mutation == 'missing_identity':
                    for value in suites.values(): value['binary_sha256'] = None
                else: suite[mutation] = 'drift'
                self.write(root, suites)
                with self.assertRaises((ValueError, OSError)):
                    ci_providers.join(root)


if __name__ == '__main__':
    unittest.main()
