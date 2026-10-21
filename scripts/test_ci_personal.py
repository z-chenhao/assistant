import copy
import json
from pathlib import Path
import tempfile
import unittest

from ci_personal import join
import m6_report
import test_m6_cases
from verify_suite import personal_gates, report_name


class PersonalJoinTests(unittest.TestCase):
    def fixture(self, root):
        reports = test_m6_cases.M6Cases().reports()
        for part, pairs in m6_report.PARTS.items():
            folder = root / part
            folder.mkdir()
            gate, = personal_gates(part)
            rows = []
            for attempt, full in enumerate(reports, 1):
                report = copy.deepcopy(full)
                report.update(part=part, results=[r for r in report['results']
                    if (r['case_id'], r['variant_id']) in pairs])
                (folder / report_name(gate, attempt)).write_text(json.dumps(report))
                rows.append(dict(gate=gate.name, attempt=attempt, returncode=0, owned_group_drained=True))
            suite = dict(passed=True, source_stable=True, source_dirty=False, source_sha='a'*40,
                         source_diff_sha256='empty', binary_sha256='binary', workflow_sha='merge',
                         personal_part=part, elapsed_seconds=10, shards={'personal': rows})
            (folder/f'suite-personal-{part}.json').write_text(json.dumps(suite))

    def test_join_preserves_every_variant_and_five_consecutive_verdicts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            suite, reports, summary = join(root)
            self.assertTrue(summary['passed'])
            self.assertIsNone(summary['suite_mean'])
            self.assertEqual([len(r['results']) for r in reports], [66]*5)
            self.assertEqual([(r['gate'], r['attempt']) for r in suite['shards']['personal']],
                             [('m6-cases', i) for i in range(1, 6)])
            self.assertTrue(all(len(r['parts']) == 7 for r in suite['shards']['personal']))
            self.assertEqual(sum(c['runs'] for c in summary['cases'].values()), 330)

    def test_no_missing_failed_drifted_or_substituted_part_can_pass(self):
        mutations = ('missing_part', 'missing_report', 'missing_round', 'duplicate_round', 'reordered',
                     'failed', 'cancelled', 'undrained', 'dirty', 'unstable', 'wrong_part', 'source',
                     'binary', 'fixture', 'driver', 'lock', 'missing_variant', 'duplicate_variant',
                     'wrong_case', 'hard_failure', 'assertion_failure', 'score', 'cleanup')
        for mutation in mutations:
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                self.fixture(root)
                suite_path = root/'desktop-1/suite-personal-desktop-1.json'
                report_path = root/'desktop-1/m6-desktop-1-5.json'
                suite = json.loads(suite_path.read_text())
                report = json.loads(report_path.read_text())
                rows = suite['shards']['personal']
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
                elif mutation == 'wrong_part': report['part'] = 'desktop-2'
                elif mutation == 'source': suite['source_sha'] = 'other'
                elif mutation == 'binary': report['binary_sha256'] = 'other'
                elif mutation == 'fixture': report['dataset_digest'] = 'other'
                elif mutation == 'driver': report['driver_digest'] = 'other'
                elif mutation == 'lock': report['contract_lock'] = {'drift':True}
                elif mutation == 'missing_variant': report['results'].pop()
                elif mutation == 'duplicate_variant': report['results'][-1] = report['results'][0]
                elif mutation == 'wrong_case': report['results'][0]['case_id'] = 'GC-13'
                elif mutation == 'hard_failure': report['results'][0]['hard_failure'] = True
                elif mutation == 'assertion_failure': report['results'][0]['assertions']['observed'] = False
                elif mutation == 'score': report['results'][0]['final_score'] = 100
                elif mutation == 'cleanup': report['cleanup'] = False
                if suite_path.exists(): suite_path.write_text(json.dumps(suite))
                if report_path.exists(): report_path.write_text(json.dumps(report))
                with self.assertRaises((ValueError, FileNotFoundError)):
                    join(root)


if __name__ == '__main__':
    unittest.main()
