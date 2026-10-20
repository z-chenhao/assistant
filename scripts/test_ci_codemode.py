import copy
import json
from pathlib import Path
import tempfile
import unittest

from ci_codemode import join
import test_core_cases
from verify_suite import CODEMODE_PARTS, SHARDS, codemode_gates, report_name


class CodeModeJoinTests(unittest.TestCase):
    def fixture(self, root):
        reports = test_core_cases.CoreTests().reports()
        for part in CODEMODE_PARTS:
            folder = root / part
            folder.mkdir()
            rows = []
            for gate in codemode_gates(part):
                for attempt in range(1, 6):
                    rows.append(dict(gate=gate.name, attempt=attempt, returncode=0, owned_group_drained=True))
                    report = copy.deepcopy(reports[attempt - 1])
                    if part == 'contracts':
                        report['results'] = []
                    elif part != 'control':
                        report['results'] = [r for r in report['results'] if r['case_id'] == part]
                        report.pop('conformance')
                        report.pop('ci_cancellation')
                    (folder / report_name(gate, attempt)).write_text(json.dumps(report))
            suite = dict(passed=True, source_stable=True, source_dirty=False, source_sha='a' * 40,
                         source_diff_sha256='empty', binary_sha256='bin', workflow_sha='merge',
                         image_os='ubuntu24', image_version='image', codemode_part=part,
                         elapsed_seconds=10, shards={'codemode': rows})
            (folder / f'suite-codemode-{part}.json').write_text(json.dumps(suite))

    def test_full_join_preserves_inventory_and_all_five_hard_verdicts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            suite, reports, summary = join(root)
            self.assertEqual([(r['gate'], r['attempt']) for r in suite['shards']['codemode']],
                             [(g.name, i) for g in SHARDS['codemode'] for i in range(1, 6)])
            self.assertTrue(summary['passed'])
            self.assertEqual([len(r['results']) for r in reports], [18] * 5)
            self.assertEqual([r['summary']['fragments'] for r in reports], [18] * 5)
            self.assertEqual([r['summary']['fragment_passes'] for r in reports], [18] * 5)
            self.assertEqual([r['summary']['hard_failures'] for r in reports], [0] * 5)
            self.assertTrue(all(r['conformance']['passed'] and r['ci_cancellation']['passed'] for r in reports))
            self.assertEqual(sum(c['runs'] for c in summary['cases'].values()), 90)

    def test_independently_built_same_binary_can_join_across_host_image_rollout(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            path = root / 'GC-17/suite-codemode-GC-17.json'
            suite = json.loads(path.read_text())
            suite['image_version'] = 'new-host-image'
            path.write_text(json.dumps(suite))
            self.assertTrue(join(root)[2]['passed'])
            suite['binary_sha256'] = 'different'
            path.write_text(json.dumps(suite))
            with self.assertRaises(ValueError):
                join(root)

    def test_no_part_or_repetition_can_be_omitted_or_substituted(self):
        mutations = ('missing_part', 'missing_report', 'missing_round', 'duplicate_round', 'reordered',
                     'failed', 'cancelled', 'undrained', 'dirty', 'unstable', 'wrong_part', 'source',
                     'binary', 'fixture', 'missing_variant', 'duplicate_variant',
                     'wrong_case', 'hard_failure', 'assertion_failure', 'contracts_cases',
                     'missing_sc', 'failed_sc', 'missing_cancel', 'failed_cancel', 'cleanup')
        for mutation in mutations:
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                self.fixture(root)
                suite_path = root / 'GC-17/suite-codemode-GC-17.json'
                report_path = root / 'GC-17/core-GC-17-5.json'
                suite = json.loads(suite_path.read_text())
                report = json.loads(report_path.read_text())
                rows = suite['shards']['codemode']
                if mutation == 'missing_part': suite_path.unlink()
                elif mutation == 'missing_report': report_path.unlink()
                elif mutation == 'missing_round': rows.pop()
                elif mutation == 'duplicate_round': rows[-1] = rows[0]
                elif mutation == 'reordered': rows.reverse()
                elif mutation == 'failed': suite['passed'] = False
                elif mutation == 'cancelled': rows[-1]['returncode'] = 130
                elif mutation == 'undrained': rows[-1]['owned_group_drained'] = False
                elif mutation == 'dirty': suite['source_dirty'] = True
                elif mutation == 'unstable': suite['source_stable'] = False
                elif mutation == 'wrong_part': suite['codemode_part'] = 'GC-19'
                elif mutation == 'source': suite['source_sha'] = 'changed'
                elif mutation == 'binary': report['binary_sha256'] = 'changed'
                elif mutation == 'fixture': report['dataset_digest'] = 'changed'
                elif mutation == 'missing_variant': report['results'].pop()
                elif mutation == 'duplicate_variant': report['results'][-1] = report['results'][0]
                elif mutation == 'wrong_case': report['results'][0]['case_id'] = 'GC-19'
                elif mutation == 'hard_failure': report['results'][0]['hard_failure'] = True
                elif mutation == 'assertion_failure': report['results'][0]['assertions']['state'] = False
                elif mutation == 'cleanup': report['cleanup'] = False
                else:
                    report_path = root / 'contracts/core-contracts-5.json'
                    report = json.loads(report_path.read_text())
                    if mutation == 'contracts_cases': report['results'] = [{}]
                    elif mutation == 'missing_sc': report.pop('conformance')
                    elif mutation == 'failed_sc': report['conformance']['passed'] = False
                    elif mutation == 'missing_cancel': report.pop('ci_cancellation')
                    elif mutation == 'failed_cancel': report['ci_cancellation']['passed'] = False
                if suite_path.exists(): suite_path.write_text(json.dumps(suite))
                if report_path.exists(): report_path.write_text(json.dumps(report))
                with self.assertRaises((ValueError, FileNotFoundError)):
                    join(root)


if __name__ == '__main__':
    unittest.main()
