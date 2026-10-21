import copy
import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import ci_supersede
from ci_supersede import summarize


class SupersessionTests(unittest.TestCase):
    def test_push_is_frozen_and_unknown_outcome_cannot_be_retried(self):
        for outcome in ('accepted', 'failed', 'crash', 'no_update'):
            with self.subTest(outcome=outcome), tempfile.TemporaryDirectory() as directory:
                output = Path(directory)/'push.json'
                before = dict(headRefOid='a'*40, headRefName='review', isDraft=True)
                calls = []
                def execute(command, **kwargs):
                    calls.append(command)
                    if command[1] == 'merge-base':
                        return subprocess.CompletedProcess(command, 0)
                    self.assertEqual(command, ['git','push','--porcelain','origin','b'*40+':refs/heads/review'])
                    self.assertEqual(json.loads(output.read_text())['outcome'], 'unknown')
                    if outcome == 'crash':
                        raise OSError('ambiguous disconnect')
                    text = ' \tref:ref\ta..b\n' if outcome == 'accepted' else '=\tref:ref\tup to date\n'
                    return subprocess.CompletedProcess(command, 1 if outcome == 'failed' else 0, text, '')
                with patch.object(ci_supersede, 'gh', return_value=before), \
                     patch.object(ci_supersede.subprocess, 'check_output', side_effect=['review\n','b'*40+'\n']), \
                     patch.object(ci_supersede.subprocess, 'run', side_effect=execute):
                    if outcome == 'crash':
                        with self.assertRaises(OSError): ci_supersede.push(24, output)
                    else:
                        self.assertEqual(ci_supersede.push(24, output), 0 if outcome == 'accepted' else 1)
                    with self.assertRaises(ValueError): ci_supersede.push(24, output)
                self.assertEqual(len(calls), 2)
                self.assertEqual(json.loads(output.read_text())['outcome'], 'accepted' if outcome == 'accepted' else 'unknown')

    def test_ready_pr_cannot_dispatch_a_push(self):
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(ci_supersede, 'gh', return_value=dict(headRefOid='a'*40, headRefName='review', isDraft=False)), \
             patch.object(ci_supersede.subprocess, 'check_output', side_effect=['review\n','b'*40+'\n']), \
             patch.object(ci_supersede.subprocess, 'run') as execute:
            with self.assertRaises(ValueError): ci_supersede.push(24, Path(directory)/'push.json')
            execute.assert_not_called()

    def fixture(self):
        record = dict(returncode=0, before_sha='a'*40, after_sha='b'*40,
                      push_started_at='2026-10-19T00:00:10Z', push_acknowledged_at='2026-10-19T00:00:13Z')
        job = dict(name='Code Mode', started_at='2026-10-19T00:00:01Z',
                   completed_at='2026-10-19T00:00:20Z', status='completed', conclusion='cancelled')
        snapshots = {str(i)+':1': dict(run=dict(path='.github/workflows/'+name, head_sha='a'*40,
                     status='completed'), jobs=[copy.deepcopy(job)])
                     for i, name in enumerate(('ci.yml',), 1)}
        return record, snapshots

    def test_acknowledgment_interval_and_known_waste(self):
        report = summarize(*self.fixture())
        self.assertTrue(report['measurement_complete'])
        self.assertEqual(report['stop_latency_bounds_seconds'], [6, 11])
        self.assertAlmostEqual(report['runner_minutes_after_push'], 10/60)

    def test_current_workflow_matches_maintained_collector_inventory(self):
        root = Path(ci_supersede.__file__).resolve().parents[1]
        self.assertEqual(ci_supersede.WORKFLOWS, {'.github/workflows/ci.yml'})
        self.assertTrue(all((root / name).is_file() for name in ci_supersede.WORKFLOWS))
        self.assertFalse((root / '.github/workflows/ci-shadow.yml').exists())
        record, snapshots = self.fixture()
        self.assertTrue(summarize(record, snapshots)['measurement_complete'])
        # A second attempt for the same workflow, or an obsolete/unknown
        # workflow, cannot substitute for this exact topology.
        for path in ('.github/workflows/ci.yml', '.github/workflows/ci-shadow.yml',
                     '.github/workflows/unknown.yml'):
            with self.subTest(path=path):
                extra = copy.deepcopy(snapshots['1:1'])
                extra['run']['path'] = path
                self.assertFalse(summarize(record, dict(snapshots, extra=extra))['measurement_complete'])
                if path != '.github/workflows/ci.yml':
                    self.assertFalse(summarize(record, {'1:1': extra})['measurement_complete'])

    def test_collect_cli_writes_current_single_workflow_evidence(self):
        record, snapshots = self.fixture()
        snapshot = snapshots['1:1']
        def read(*args):
            if args == ('api', 'repos/z-chenhao/erii/actions/runs/1/attempts/1'):
                return snapshot['run']
            self.assertEqual(args, ('api', '--paginate', '--slurp',
                                    'repos/z-chenhao/erii/actions/runs/1/attempts/1/jobs?per_page=100'))
            return [{'jobs': snapshot['jobs']}]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            push, output = root / 'push.json', root / 'report.json'
            push.write_text(json.dumps(record))
            with patch.object(ci_supersede, 'gh', side_effect=read), \
                 patch.object(sys, 'argv', ['ci_supersede.py', 'collect', '--push-record', str(push),
                                          '--run', '1:1', '--output', str(output)]), \
                 contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(ci_supersede.main(), 0)
            report = json.loads(output.read_text())
            self.assertTrue(report['measurement_complete'])
            self.assertEqual(set(report['attempts']), {'1:1'})
            self.assertEqual(report['stop_latency_bounds_seconds'], [6, 11])

    def test_unknown_missing_mismatched_and_partial_proof_never_measures_zero(self):
        for mutation in ('unknown', 'missing_sha', 'same_sha', 'missing_ack', 'source', 'missing_workflow', 'duplicate_workflow',
                         'pending_run', 'pending_job', 'missing_completion', 'already_stopped', 'natural_completion'):
            with self.subTest(mutation=mutation):
                record, snapshots = self.fixture()
                first = snapshots['1:1']
                if mutation == 'unknown': record['returncode'] = 1
                elif mutation == 'missing_sha': record['before_sha'] = 'a'
                elif mutation == 'same_sha': record['after_sha'] = record['before_sha']
                elif mutation == 'missing_ack': del record['push_acknowledged_at']
                elif mutation == 'source': first['run']['head_sha'] = 'c'*40
                elif mutation == 'missing_workflow': del snapshots['1:1']
                elif mutation == 'duplicate_workflow': snapshots['1:2'] = copy.deepcopy(first)
                elif mutation == 'pending_run': first['run']['status'] = 'in_progress'
                elif mutation == 'pending_job': first['jobs'][0]['status'] = 'in_progress'
                elif mutation == 'missing_completion': del first['jobs'][0]['completed_at']
                elif mutation == 'already_stopped':
                    for value in snapshots.values(): value['jobs'][0]['completed_at'] = '2026-10-19T00:00:05Z'
                elif mutation == 'natural_completion':
                    for value in snapshots.values(): value['jobs'][0]['conclusion'] = 'success'
                report = summarize(record, snapshots)
                self.assertFalse(report['measurement_complete'])
                self.assertTrue(report['errors'])
                self.assertNotIn('stop_latency_bounds_seconds', report)

    def test_missing_job_start_is_unknown_waste_not_zero(self):
        record, snapshots = self.fixture()
        snapshots['1:1']['jobs'].append(copy.deepcopy(snapshots['1:1']['jobs'][0]))
        del snapshots['1:1']['jobs'][0]['started_at']
        report = summarize(record, snapshots)
        self.assertTrue(report['measurement_complete'])
        self.assertIsNone(report['runner_minutes_after_push'])


if __name__ == '__main__':
    unittest.main()
