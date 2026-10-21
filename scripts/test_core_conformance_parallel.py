import json
import subprocess
import threading
import unittest
from collections import Counter
from pathlib import Path
from unittest.mock import patch

import core_conformance as core


class ConformanceSchedulingTests(unittest.TestCase):
    def drive(self, mutation=None, cpus=4):
        calls, active, peak = [], 0, 0
        lock = threading.Lock()
        barrier = threading.Barrier(4 if cpus >= 4 else 2 if cpus >= 2 else 1)

        def execute(command, **kwargs):
            nonlocal active, peak
            selected = command[command.index('-run') + 1][2:-2].split('|')
            with lock:
                calls.append((command, kwargs, selected))
                active += 1
                peak = max(peak, active)
                index = len(calls)
            barrier.wait(timeout=3)
            events = [dict(Test=name, Action='pass') for name in selected]
            if index == 1:
                if mutation == 'missing': events.pop()
                if mutation == 'duplicate': events.append(events[0])
                if mutation == 'skip': events[0]['Action'] = 'skip'
                if mutation == 'fail': events[0]['Action'] = 'fail'
            with lock:
                active -= 1
            return subprocess.CompletedProcess(command, 1 if mutation == 'exit' and index == 1 else 0,
                                               '\n'.join(map(json.dumps, events)), '')

        with patch.object(core, 'cpu_budget', return_value=cpus), \
             patch.object(core, 'fixture_run', side_effect=execute):
            result = core.run(Path('.'))
        return result, calls, peak

    def test_complete_inventory_runs_once_with_bounded_cpu_and_unchanged_contracts(self):
        result, calls, peak = self.drive()
        self.assertTrue(result['passed'])
        self.assertEqual(peak, 4)
        expected = {name for names in core.TESTS.values() for name in names}
        self.assertEqual(Counter(name for _, _, names in calls for name in names), Counter(expected))
        for command, kwargs, _ in calls:
            for flag in ('-race', '-count=1', '-json', '-p=1'):
                self.assertIn(flag, command)
            self.assertEqual(kwargs['timeout'], 300)
            self.assertEqual(kwargs['env']['GOMAXPROCS'], '1')
            self.assertEqual(command[-2:], ['./internal/agent/sqlite', './internal/memory/sqlite'])

    def test_missing_duplicate_skip_failure_and_nonzero_exit_remain_failures(self):
        for mutation in ('missing', 'duplicate', 'skip', 'fail', 'exit'):
            with self.subTest(mutation=mutation):
                self.assertFalse(self.drive(mutation)[0]['passed'])

    def test_admission_respects_small_cpu_budgets(self):
        for cpus, workers in ((1, 1), (2, 2), (3, 2)):
            with self.subTest(cpus=cpus):
                result, calls, peak = self.drive(cpus=cpus)
                self.assertTrue(result['passed'])
                self.assertEqual(len(calls), workers)
                self.assertEqual(peak, workers)


if __name__ == '__main__':
    unittest.main()
