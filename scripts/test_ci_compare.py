"""Controlled comparisons cannot turn failed or reused CI into passing proof."""
import copy
import unittest

import ci_compare as comparison


class ComparisonTests(unittest.TestCase):
    def experiment(self, samples=5):
        raw = dict(mode=comparison.HISTORICAL, pairs=[], attempts={})
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


class CurrentComparisonTests(unittest.TestCase):
    """Drive genuine strict joins, then mutate their small retained receipts."""
    @classmethod
    def setUpClass(cls):
        import json
        from pathlib import Path
        import tempfile
        import ci_codemode
        import ci_personal
        from test_ci_codemode import CodeModeJoinTests
        from test_ci_personal import PersonalJoinTests
        reports = ComparisonTests().experiment(1)['attempts']['1:1']['reports']
        for shard in ('codemode', 'personal'):
            with tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                fixture = CodeModeJoinTests() if shard == 'codemode' else PersonalJoinTests()
                fixture.fixture(root)
                joined, _, summary = (ci_codemode if shard == 'codemode' else ci_personal).join(root)
                reports['suite-' + shard + '.json'] = joined
                reports['core-summary.json' if shard == 'codemode' else 'm6-summary.json'] = summary
        for name, report in reports.items():
            if not name.startswith('suite-'):
                continue
            for suite in [report, *report.get('parts', {}).values()]:
                suite.update(binary_sha256='b' * 64, hardware=dict(logical_cpus=4),
                             image_os='ubuntu24', image_version='fixed-image', selected_gates=None)
                suite.setdefault('complete_inventory', True)
                for rows in suite['shards'].values():
                    for row in rows:
                        row['owned_group_drained'] = True
        cls.reports = reports

    def experiment(self, samples=3):
        raw = ComparisonTests().experiment(samples)
        raw['mode'] = 'current'
        for value in raw['attempts'].values():
            value['reports'] = copy.deepcopy(self.reports)
            value['run']['path'] = '.github/workflows/ci.yml'
            value['jobs'] = [dict(id=i, name=name, conclusion='success', steps=[], head_sha='a'*40,
                                 run_id=value['run']['id'], run_attempt=value['run']['run_attempt'],
                                 started_at='2026-10-19T00:00:00Z', completed_at='2026-10-19T00:00:40Z')
                             for i, name in enumerate(sorted(comparison.topology()))]
            value['measurement_profile'] = dict(input_sha256='c'*64, tool_versions={'go': 'pinned', 'node': 'pinned'},
                lock_sha256='d'*64, cache_state='warm-offline', environment='isolated fixture')
        return raw

    def test_three_current_pairs_pass_and_two_do_not_certify(self):
        raw = self.experiment()
        report = comparison.compare(raw)
        self.assertTrue(report['performance_certified'], report)
        self.assertEqual(len(comparison.topology()), 21)
        insufficient = comparison.compare(self.experiment(2))
        self.assertFalse(insufficient['performance_certified'])
        self.assertIsNone(insufficient['p50_wall_reduction'])

    def test_every_job_part_and_join_is_required(self):
        for name in comparison.topology():
            for change in ('missing', 'duplicate', 'skipped', 'failure', 'cancelled'):
                with self.subTest(name=name, change=change):
                    raw = self.experiment(1)
                    jobs = raw['attempts']['2:1']['jobs']
                    job = next(j for j in jobs if j['name'] == name)
                    if change == 'missing': jobs.remove(job)
                    elif change == 'duplicate': jobs.append(copy.deepcopy(job))
                    else: job['conclusion'] = change
                    self.assertFalse(comparison.compare(raw)['pairs'][0]['eligible'])

    def test_join_receipt_mutations_fail_closed(self):
        for shard in ('codemode', 'personal'):
            for change in ('missing-part', 'wrong-part', 'source', 'binary', 'image', 'cpu', 'round', 'duplicate',
                           'reorder', 'undrained', 'focused', 'cleanup', 'join-members', 'summary'):
                with self.subTest(shard=shard, change=change):
                    raw = self.experiment(1)
                    data = raw['attempts']['2:1']
                    suite = data['reports']['suite-' + shard + '.json']
                    part = next(iter(suite['parts']))
                    fragment = suite['parts'][part]
                    rows = fragment['shards'][shard]
                    if change == 'missing-part': suite['parts'].pop(part)
                    elif change == 'wrong-part': fragment[shard + '_part'] = 'other'
                    elif change == 'source': fragment['source_sha'] = 'e'*40
                    elif change == 'binary': fragment['binary_sha256'] = 'e'*64
                    elif change == 'image': fragment['image_version'] = 'other'
                    elif change == 'cpu': fragment['hardware']['logical_cpus'] = 2
                    elif change == 'round': rows.pop()
                    elif change == 'duplicate': rows.append(copy.deepcopy(rows[0]))
                    elif change == 'reorder': rows.reverse()
                    elif change == 'undrained': rows[0]['owned_group_drained'] = False
                    elif change == 'focused': fragment['selected_gates'] = ['main']
                    elif change == 'cleanup': suite['shards'][shard][-1]['owned_group_drained'] = False
                    elif change == 'join-members': suite['shards'][shard][-1]['parts'].pop()
                    else: data['reports']['core-summary.json' if shard == 'codemode' else 'm6-summary.json']['passed'] = False
                    self.assertFalse(comparison.compare(raw)['pairs'][0]['eligible'])

    def test_identity_profile_attempt_and_focus_mismatches_fail_closed(self):
        for change in ('source', 'attempt', 'job-sha', 'job-attempt', 'job-missing-identity', 'profile', 'profile-missing', 'focus', 'historical'):
            with self.subTest(change=change):
                raw = self.experiment(1)
                data = raw['attempts']['2:1']
                if change == 'source': data['run']['head_sha'] = 'e'*40
                elif change == 'attempt': data['run']['run_attempt'] = 2
                elif change == 'job-sha': data['jobs'][0]['head_sha'] = 'e'*40
                elif change == 'job-attempt': data['jobs'][0]['run_attempt'] = 2
                elif change == 'job-missing-identity': data['jobs'][0].pop('head_sha')
                elif change == 'profile': data['measurement_profile']['cache_state'] = 'cold'
                elif change == 'profile-missing': data.pop('measurement_profile')
                elif change == 'focus': data['reports']['suite-runtime.json']['complete_inventory'] = False
                else: data['run']['path'] = '.github/workflows/ci-shadow.yml'
                self.assertFalse(comparison.compare(raw)['pairs'][0]['eligible'])

    def test_historical_data_requires_explicit_mode_and_never_certifies(self):
        raw = ComparisonTests().experiment()
        self.assertFalse(comparison.compare(raw)['performance_certified'])
        raw.pop('mode')
        self.assertEqual(comparison.compare(raw)['eligible_samples'], 0)

    def test_profile_drift_between_internally_matching_pairs_rejects_cohort(self):
        raw = self.experiment()
        for key in ('1:2', '2:2'):
            raw['attempts'][key]['measurement_profile']['cache_state'] = 'cold'
        report = comparison.compare(raw)
        self.assertEqual(report['eligible_samples'], 3)
        self.assertFalse(report['performance_certified'])
        raw = self.experiment(1)
        raw['pairs'] *= 3
        self.assertFalse(comparison.compare(raw)['performance_certified'])

    def test_real_21_job_metadata_replays_without_claiming_missing_suite_proof(self):
        import json
        from pathlib import Path
        data = json.loads((Path(__file__).parent / 'fixtures/ci-39915092608-1.json').read_text())
        self.assertEqual(comparison.validate_jobs(data, data['run']['head_sha']), [])
        self.assertEqual(comparison.timing(data)['required_verdict_seconds'], 838)
        self.assertAlmostEqual(comparison.timing(data)['runner_minutes'], 108.4)
        data['reports'] = {}
        self.assertTrue(comparison.validate(data, data['run']['head_sha'])[0])

    def test_topology_drift_cannot_silently_reduce_inventory(self):
        text = (comparison.ROOT / '.github/workflows/ci.yml').read_text()
        for changed in (text.replace('"GC-17", ', ''), text.replace('codemode_join:', 'other_join:'),
                        text.replace('        part: [', '        part: ${{ fromJSON(')):
            with self.assertRaises(ValueError):
                comparison.topology(changed)

    def test_offline_cli_retains_raw_hash_and_noncertification(self):
        import hashlib
        import json
        from pathlib import Path
        import subprocess
        import sys
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'comparison.json'
            raw = output.with_name('comparison-raw.json')
            raw.write_text(json.dumps(self.experiment(1)))
            result = subprocess.run([sys.executable, str(comparison.ROOT / 'scripts/ci_compare.py'),
                                     '--offline', '--output', str(output)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            report = json.loads(output.read_text())
            self.assertFalse(report['performance_certified'])
            self.assertEqual(report['raw_sha256'], hashlib.sha256(raw.read_bytes()).hexdigest())
            self.assertEqual(report['eligible_samples'], 1)


class CollectorBoundaryTests(unittest.TestCase):
    def test_exact_joined_artifacts_share_budget_and_unrelated_archives_are_not_requested(self):
        import json
        from pathlib import Path
        import sys
        import tempfile
        import types
        from unittest.mock import patch
        import zipfile
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            artifacts = [dict(id=i, name=f'evidence-{shard}-' + 'a'*40 + '-1')
                         for i, shard in enumerate(comparison.SHARDS, 1)]
            artifacts.append(dict(id=99, name='evidence-fast-' + 'a'*40 + '-1'))
            metadata = dict(source_sha='a'*40, artifacts=artifacts)
            calls = []
            def archive(cache, meta, artifact_id, budget):
                self.assertIs(cache, root)
                self.assertIs(meta, metadata)
                self.assertEqual(budget, 100 - len(calls)*10)
                calls.append(artifact_id)
                path = root / f'{artifact_id}.zip'
                shard = list(comparison.SHARDS)[artifact_id - 1]
                with zipfile.ZipFile(path, 'w') as stream:
                    stream.writestr('suite-' + shard + '.json', json.dumps({'shard': shard}))
                    stream.writestr('unrelated.json', 'do not read this')
                    if shard == 'codemode': stream.writestr('core-summary.json', '{}')
                    if shard == 'personal': stream.writestr('m6-summary.json', '{}')
                return path, dict(downloaded_bytes=10)
            collector = types.SimpleNamespace(inventory=lambda *a, **k: metadata, artifact_archive=archive)
            with patch.dict(sys.modules, evidence=collector), patch.object(comparison.profile, 'gh',
                    side_effect=[dict(id=1, run_attempt=1, head_sha='a'*40, path='.github/workflows/ci.yml',
                        status='completed', conclusion='success'), [{'jobs': [dict(name=name, conclusion='success', head_sha='a'*40, run_id=1, run_attempt=1,
                        started_at='2026-10-19T00:00:00Z', completed_at='2026-10-19T00:00:01Z')
                        for name in comparison.topology()]}]]):
                value = comparison.collect('z-chenhao/erii', '1:1', root, 100)
            self.assertEqual(calls, list(range(1, 7)))
            self.assertEqual(len(value['reports']), 8)
            self.assertEqual(sum(r['downloaded_bytes'] for r in value['artifact_receipts']), 60)

    def test_retired_experiment_cli_rejects_before_any_github_action(self):
        import subprocess
        import sys
        result = subprocess.run([sys.executable, str(comparison.ROOT / 'scripts/ci_experiment.py'),
                                 '--pr', '1', '--sha', 'a'*40, '--output', 'unused.json'],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn('retired shadow orchestrator', result.stderr)


if __name__ == '__main__':
    unittest.main()
