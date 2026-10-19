"""Five-round orchestration never retries failures or mutates a changed head."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import ci_experiment as experiment
import test_ci_compare as comparison_fixture


class ExperimentTests(unittest.TestCase):
    def test_five_complete_pairs_and_one_rerun_request_per_workflow(self):
        self.drive()

    def test_failed_sample_stops_instead_of_rerunning_to_green(self):
        self.drive(fail_round=2)

    def test_unknown_rerun_outcome_is_checkpointed_without_retry(self):
        self.drive(unknown=True)

    def drive(self, fail_round=None, unknown=False):
        raw = comparison_fixture.ComparisonTests().experiment()
        attempts = {1: 1, 2: 1}
        commands = []
        watches = []
        def api(*args):
            if args[0] == 'pr':
                return dict(headRefOid='a' * 40, isDraft=True)
            route = args[-1]
            if '?' in route:
                return dict(workflow_runs=[raw['attempts'][f'{i}:1']['run'] for i in (1, 2)])
            run_id = int(route.rsplit('/', 1)[1])
            return raw['attempts'][f'{run_id}:{attempts[run_id]}']['run']
        def command(args, **_):
            commands.append(args)
            if args[1:3] == ['run', 'rerun']:
                if unknown:
                    raise subprocess.CalledProcessError(1, args)
                run_id = int(args[3])
                attempts[run_id] += 1
            else:
                watches.append(len(watches) + 1)
            return subprocess.CompletedProcess(args, 1 if watches and watches[-1] == fail_round else 0)
        def collect(_, key):
            value = raw['attempts'][key]
            if key == f'2:{fail_round}':
                value['run']['conclusion'] = 'failure'
            return value
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'experiment.json'
            with patch.object(experiment, 'gh', side_effect=api), \
                 patch.object(experiment, 'wait_active'), \
                 patch.object(experiment.subprocess, 'run', side_effect=command), \
                 patch.object(experiment.ci_compare, 'collect', side_effect=collect):
                if unknown:
                    with self.assertRaises(subprocess.CalledProcessError):
                        experiment.run(24, 'a' * 40, output)
                else:
                    self.assertEqual(experiment.run(24, 'a' * 40, output), 1 if fail_round else 0)
                state = json.loads(output.read_text())
                if unknown:
                    self.assertEqual(len(state['requests']), 1)
                    self.assertEqual(state['requests'][0]['outcome'], 'unknown')
                    self.assertEqual(len(watches), 1)
                elif fail_round:
                    self.assertEqual(state['stage'], 'failed')
                    self.assertEqual(len(watches), fail_round)
                    self.assertEqual(len(state['requests']), 2)
                else:
                    self.assertEqual(state['stage'], 'complete')
                    self.assertEqual(len(watches), 5)
                    self.assertEqual(len(state['requests']), 8)
                    self.assertEqual(attempts, {1: 5, 2: 5})
                with self.assertRaisesRegex(ValueError, 'already exists'):
                    experiment.run(24, 'a' * 40, output)

    def test_head_must_remain_draft_and_exact(self):
        for state in [dict(headRefOid='b' * 40, isDraft=True), dict(headRefOid='a' * 40, isDraft=False)]:
            with patch.object(experiment, 'gh', return_value=state):
                with self.assertRaises(ValueError):
                    experiment.head(24, 'a' * 40)

    def test_rerun_waits_for_fresh_checks_instead_of_accepting_old_green(self):
        checks = []
        sleeps = []
        def api(*args):
            if args[0] == 'pr' and args[1] == 'view':
                return dict(headRefOid='a' * 40, isDraft=True)
            route = args[-1]
            if '/commits/' in route:
                checks.append(1)
                return dict(check_runs=[dict(details_url='/job/1'), dict(details_url='/job/2')] if len(checks) == 1 else [
                    dict(details_url='/job/1001'), dict(details_url='/job/1002')])
            run_id = int(route.split('/runs/')[1].split('/')[0])
            if '/jobs?' in route:
                return dict(jobs=[dict(id=1000 + run_id)])
            return dict(head_sha='a' * 40, run_attempt=2)
        with patch.object(experiment, 'gh', side_effect=api), \
             patch.object(experiment.time, 'sleep', side_effect=sleeps.append):
            experiment.wait_active(24, 'a' * 40, dict(before='1:2', after='2:2'))
        self.assertEqual(len(checks), 2)
        self.assertEqual(sleeps, [2])

    def test_acknowledged_queued_workflow_waits_without_retrying_or_timing_out_checks(self):
        clock, reads, jobs = [0], [], []
        def api(*args):
            if args[0] == 'pr': return dict(headRefOid='a'*40,isDraft=True)
            route=args[-1]
            if '/commits/' in route: return dict(check_runs=[dict(details_url='/job/1001')])
            if '/jobs?' in route:
                jobs.append(route)
                return dict(jobs=[dict(id=1001)])
            reads.append(route)
            return dict(head_sha='a'*40,run_attempt=2,status='pending' if len(reads)==1 else 'in_progress')
        def sleep(_): clock[0]+=180
        with patch.object(experiment,'gh',side_effect=api), \
             patch.object(experiment.time,'monotonic',side_effect=lambda:clock[0]), \
             patch.object(experiment.time,'sleep',side_effect=sleep), \
             patch.object(experiment.subprocess,'run') as mutation:
            experiment.wait_active(24,'a'*40,dict(before='1:2'))
        self.assertEqual(len(reads),2)
        self.assertEqual(len(jobs),1)
        mutation.assert_not_called()


if __name__ == '__main__':
    unittest.main()
