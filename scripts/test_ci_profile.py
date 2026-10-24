import copy
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import ci_profile as profile
import profile_harness


class ProfileTests(unittest.TestCase):
    def test_trace_fast_path_avoids_filesystem_paths_and_preserves_every_scenario_kind(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            script = root/'fixture.py'
            def frame(name, file, values=None):
                return SimpleNamespace(f_code=SimpleNamespace(co_name=name, co_filename=str(file)),
                                       f_locals=values or {}, f_trace_lines=True)
            def exercise(*_args, **_kwargs):
                trace = install.call_args.args[0]
                with patch.object(profile_harness, 'Path', side_effect=AssertionError('unrelated call resolved a path')):
                    for name in ('read_bytes', 'open', 'digest', 'run', '__new__'):
                        self.assertIsNone(trace(frame(name, '/unrelated.py'), 'call', None))
                for name, file, values in [
                    ('scenario','fixture.py',{'name':'ordinary'}),
                    ('driver','fixture.py',{'case':{'case_id':'GC-17'},'variant':{'variant_id':'wording-1'}}),
                    ('execute','verify_codemode.py',{'key':'cell'}),
                    ('feedback_case','run_core_cases.py',{'variant':{'variant_id':'one'}}),
                    ('coding_case','run_core_cases.py',{'variant':{'variant_id':'two'}}),
                    ('engineering_case','run_core_cases.py',{'variant':{'variant_id':'three'}}),
                ]:
                    current = frame(name, root/file, values)
                    self.assertIs(trace(current, 'call', None), trace)
                    self.assertFalse(current.f_trace_lines)
                    trace(current, 'return', None)
                self.assertIsNone(trace(frame('execute',root/'other.py'), 'call', None))
                self.assertIsNone(trace(frame('scenario',root/'foreign/fixture.py'), 'call', None))
            with patch.object(profile_harness.sys, 'argv', ['profile_harness.py', str(script)]), \
                 patch.dict(os.environ, ERII_PROFILE_DIR=str(root/'profiles')), \
                 patch.object(profile_harness.sys, 'settrace') as install, \
                 patch.object(profile_harness.runpy, 'run_path', side_effect=exercise):
                profile_harness.main()
            result = json.loads((root/'profiles/fixture.json').read_text())
            self.assertEqual([r['name'] for r in result['cases']],
                             ['ordinary','GC-17/wording-1','cell','feedback_case/one','coding_case/two','engineering_case/three'])
            self.assertTrue(all(r['elapsed_seconds'] >= 0 and r['cpu_seconds'] >= 0 for r in result['cases']))

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
        self.assertEqual(profile.phase('Verify the copied SDK stayed immutable'), 'cleanup')
        self.assertEqual(profile.phase('Clean disposable fixture storage'), 'cleanup')


if __name__ == '__main__':
    unittest.main()
