import copy
import unittest

import ci_profile as profile


class ProfileTests(unittest.TestCase):
    def fixture(self):
        run = dict(id=1, head_branch='branch', event='pull_request', head_sha='a' * 40,
                   created_at='2026-10-18T00:00:00Z', conclusion='cancelled', html_url='run/1')
        successor = dict(run, id=2, head_sha='b' * 40, created_at='2026-10-18T00:01:00Z',
                         conclusion='success')
        job = dict(id=3, name='Verification (codemode)', conclusion='cancelled',
                   started_at='2026-10-18T00:00:10Z', completed_at='2026-10-18T00:01:20Z',
                   steps=[dict(name='Build', started_at='2026-10-18T00:00:11Z',
                               completed_at='2026-10-18T00:00:15Z', conclusion='success')])
        return [run, successor], {'1': [job], '2': []}

    def test_observed_timings_and_unknowns_remain_distinct(self):
        report = profile.summarize(*self.fixture())
        row = report['runs'][0]
        self.assertEqual(row['wall_seconds'], 80)
        self.assertEqual(row['successor_creation_to_stop_seconds'], 20)
        self.assertAlmostEqual(row['runner_minutes_after_successor_creation'], 1 / 3)
        self.assertAlmostEqual(row['runner_minutes'], 70 / 60)
        self.assertIsNone(row['push_to_stop_seconds'])
        self.assertIsNone(row['jobs'][0]['cpu_seconds'])
        self.assertIsNone(row['jobs'][0]['cache_hit'])
        self.assertEqual(row['jobs'][0]['steps'][0]['phase'], 'build')
        self.assertEqual(row['jobs'][0]['steps'][0]['wall_seconds'], 4)
        self.assertEqual(report['by_conclusion']['cancelled']['wall_seconds']['samples'], 1)
        self.assertIsNone(report['runs'][1]['wall_seconds'])

    def test_other_branches_and_events_do_not_measure_cancellation(self):
        for field, value in [('head_branch', 'other'), ('event', 'push'), ('head_sha', 'a' * 40)]:
            runs, jobs = copy.deepcopy(self.fixture())
            runs[1][field] = value
            self.assertIsNone(profile.summarize(runs, jobs)['runs'][0]['successor_creation_to_stop_seconds'])

    def test_statistics_and_phase_labels(self):
        self.assertEqual(profile.stats([None])['samples'], 0)
        self.assertEqual(profile.stats(list(range(1, 21)))['p95'], 19)
        self.assertEqual(profile.stats([1, 2])['p50'], 1.5)
        self.assertEqual(profile.phase('Retain exact-SHA evidence, including failures'), 'upload')
        self.assertEqual(profile.phase('Install pinned Bridge dependencies'), 'setup')


if __name__ == '__main__':
    unittest.main()
