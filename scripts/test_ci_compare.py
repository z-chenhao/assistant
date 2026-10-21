"""Controlled comparisons cannot turn failed or reused CI into passing proof."""
import copy
import unittest

import ci_compare as comparison


class ComparisonTests(unittest.TestCase):
    def experiment(self, samples=5):
        raw = dict(pairs=[], attempts={})
        for number in range(1, samples + 1):
            pair = {}
            for side, run_id in [('before', 1), ('after', 2)]:
                key = f'{run_id}:{number}'
                pair[side] = key
                run = dict(id=run_id, run_attempt=number, head_sha='a' * 40, status='completed',
                           conclusion='success', path='.github/workflows/' + ('ci.yml' if side == 'before' else 'ci-shadow.yml'),
                           head_branch='review', event='pull_request', html_url='run/' + key,
                           created_at='2026-10-18T00:00:00Z', run_started_at='2026-10-19T00:00:00Z')
                reports = {}
                reports['core-summary.json'] = dict(passed=True, ci_cancellation=dict(passed=True, consecutive_runs=5))
                for name, gates in comparison.SHARDS.items():
                    reports['suite-' + name + '.json'] = dict(source_sha='a' * 40, source_dirty=False,
                        source_stable=True, passed=True, binary_sha256='b' * 64,
                        image_os='ubuntu24', image_version='fixed-image', shards={name: [
                            dict(gate=g.name, attempt=n, returncode=0, command=['full-harness'])
                            for g in gates for n in range(1, g.attempts + 1)]})
                    reports['suite-' + name + '.json']['hardware'] = dict(logical_cpus=4)
                names = ['Unit tests and lint', 'Race detector', 'Ubuntu checks'] + [
                    'Verification (' + name + ')' for name in comparison.SHARDS]
                jobs = [dict(id=i, name=name, conclusion='success', labels=['ubuntu-latest'], steps=[],
                             started_at='2026-10-19T00:00:00Z', completed_at='2026-10-19T00:00:40Z')
                        for i, name in enumerate(names)]
                if side == 'after':
                    for job in jobs:
                        job['name'] = {'Unit tests and lint': 'Shadow feedback', 'Race detector': 'Shadow race detector',
                                       'Ubuntu checks': 'CI shadow comparison'}.get(job['name'], 'Shadow ' + job['name'].lower())
                raw['attempts'][key] = dict(run=run, jobs=jobs, reports=reports)
            raw['pairs'].append(pair)
        return raw

    def test_five_complete_pairs_and_offline_statistics(self):
        report = comparison.compare(self.experiment())
        self.assertTrue(report['comparison_complete'], report)
        self.assertEqual(report['eligible_samples'], 5)
        self.assertEqual(report['p50_runner_minutes_reduction'], 0)
        self.assertEqual(report['statistics']['after']['runner_minutes']['variance'], 0)
        self.assertFalse(comparison.compare(self.experiment(1))['comparison_complete'])

    def test_rerun_timing_does_not_include_idle_day_or_claim_queue_latency(self):
        raw = self.experiment()
        result = comparison.timing(raw['attempts']['1:2'])
        self.assertEqual(result['wall_seconds'], 40)
        self.assertEqual(comparison.timing(raw['attempts']['1:1'])['wall_seconds'], 86440)

    def test_reused_attempts_are_not_five_samples(self):
        raw = self.experiment(1)
        raw['pairs'] *= 5
        report = comparison.compare(raw)
        self.assertFalse(report['comparison_complete'])
        self.assertEqual(report['eligible_samples'], 1)
        raw = self.experiment()
        raw['attempts']['2:2']['run']['run_attempt'] = 1
        self.assertFalse(comparison.compare(raw)['comparison_complete'])

    def test_each_missing_or_non_success_job_fails_closed(self):
        for change in ['missing', 'duplicate', 'failure', 'cancelled', 'skipped', 'timed_out']:
            with self.subTest(change=change):
                raw = self.experiment()
                jobs = raw['attempts']['2:1']['jobs']
                if change == 'missing':
                    jobs.pop()
                elif change == 'duplicate':
                    jobs.append(copy.deepcopy(jobs[0]))
                else:
                    jobs[0]['conclusion'] = change
                self.assertFalse(comparison.compare(raw)['comparison_complete'])

    def test_source_binary_image_scope_repetitions_and_partial_reports_fail_closed(self):
        for change in ['source_sha', 'binary_sha256', 'image_version', 'source_dirty', 'source_stable',
                       'passed', 'missing', 'repetition', 'partial']:
            with self.subTest(change=change):
                raw = self.experiment()
                reports = raw['attempts']['2:1']['reports']
                report = reports['suite-security.json']
                if change == 'missing':
                    reports.pop('suite-security.json')
                elif change == 'repetition':
                    report['shards']['security'].pop()
                elif change == 'partial':
                    report['shards']['security'][0]['command'].append('--feedback')
                else:
                    report[change] = {'source_dirty': True, 'source_stable': False, 'passed': False,
                                      'binary_sha256': 'c' * 64, 'source_sha': 'd' * 40,
                                      'image_version': 'other-image'}[change]
                self.assertFalse(comparison.compare(raw)['comparison_complete'])

    def test_internally_matched_pairs_on_different_images_are_not_one_controlled_cohort(self):
        raw = self.experiment()
        for key in ['1:2', '2:2']:
            for name, report in raw['attempts'][key]['reports'].items():
                if name.startswith('suite-'):
                    report['image_version'] = 'different-image'
        report = comparison.compare(raw)
        self.assertEqual(report['eligible_samples'], 5)
        self.assertFalse(report['comparison_complete'])
        self.assertTrue(report['cohort_errors'])

    def test_missing_or_incomplete_nested_cleanup_cannot_count_as_a_sample(self):
        for mutation in ('missing', 'failed', 'four-rounds'):
            raw = self.experiment()
            reports = raw['attempts']['2:1']['reports']
            if mutation == 'missing':
                reports.pop('core-summary.json')
            elif mutation == 'failed':
                reports['core-summary.json']['ci_cancellation']['passed'] = False
            else:
                reports['core-summary.json']['ci_cancellation']['consecutive_runs'] = 4
            self.assertFalse(comparison.compare(raw)['comparison_complete'])

    def test_missing_duration_or_cpu_capacity_cannot_count_as_zero(self):
        for mutation in ('duration', 'capacity', 'capacity-drift'):
            raw = self.experiment()
            data = raw['attempts']['2:1']
            if mutation == 'duration':
                data['jobs'][0]['started_at'] = None
            elif mutation == 'capacity':
                data['reports']['suite-security.json'].pop('hardware')
            else:
                data['reports']['suite-security.json']['hardware']['logical_cpus'] = 2
            self.assertFalse(comparison.compare(raw)['comparison_complete'])


if __name__ == '__main__':
    unittest.main()
