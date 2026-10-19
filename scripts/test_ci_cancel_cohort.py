import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import ci_cancel_cohort as cohort


class CancellationCohortTests(unittest.TestCase):
    def test_hot_barrier_requires_both_real_phases_ten_jobs_and_thirty_observed_seconds(self):
        clock, reads, sleeps = [0], [], []
        def snapshot(_):
            reads.append(1)
            jobs = [dict(name=name,status='in_progress',steps=[dict(name='CLI verification with timings',status='in_progress')])
                    for name in ('Verification (codemode)','Shadow verification (codemode)')]
            jobs += [dict(name=str(i),status='in_progress',steps=[]) for i in range(8)]
            if len(reads)==1: jobs.pop()
            return {'1:1':dict(run=dict(status='in_progress'),jobs=jobs[:5]),
                    '2:1':dict(run=dict(status='in_progress'),jobs=jobs[5:])}
        def sleep(seconds):
            sleeps.append(seconds);clock[0]+=seconds
        with patch.object(cohort,'snapshots',side_effect=snapshot), \
             patch.object(cohort.time,'monotonic',side_effect=lambda:clock[0]), \
             patch.object(cohort.time,'sleep',side_effect=sleep):
            cohort.hot('a'*40)
        self.assertEqual(sleeps,[15,15,15])

    def test_pre_cancel_failure_is_retained_without_a_push(self):
        with patch.object(cohort,'snapshots',return_value={'1:1':dict(run=dict(status='in_progress'),
                jobs=[dict(conclusion='failure')])}),patch.object(cohort,'push') as push:
            with self.assertRaisesRegex(ValueError,'failed before'):cohort.hot('a'*40)
            push.assert_not_called()

    def test_unknown_push_is_checkpointed_and_never_retried(self):
        with tempfile.TemporaryDirectory() as directory:
            output=Path(directory)/'push.json'
            with patch.object(cohort,'git',side_effect=['a'*40+'\tref','docs/ci-cancel-probe.md']), \
                 patch.object(cohort.subprocess,'run',side_effect=[subprocess.CompletedProcess([],0),
                      subprocess.CompletedProcess([],1,stdout='',stderr='network failure')]) as dispatch:
                with self.assertRaisesRegex(RuntimeError,'unknown'):cohort.push('b'*40,output)
                self.assertEqual(json.loads(output.read_text())['outcome'],'unknown')
                with self.assertRaisesRegex(ValueError,'existing'):cohort.push('b'*40,output)
            self.assertEqual(dispatch.call_count,2)

    def test_code_change_cannot_be_a_cancellation_sample(self):
        with tempfile.TemporaryDirectory() as directory:
            with patch.object(cohort,'git',side_effect=['a'*40+'\tref','internal/runtime.go']), \
                 patch.object(cohort.subprocess,'run',return_value=subprocess.CompletedProcess([],0)) as dispatch:
                with self.assertRaisesRegex(ValueError,'doc delta'):
                    cohort.push('b'*40,Path(directory)/'push.json')
            self.assertEqual(dispatch.call_count,1) # ancestry check only
