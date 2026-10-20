from collections import Counter
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import tempfile
import threading
import sys
import time
import unittest
from unittest.mock import patch

import ci_race as race
from verify_suite import run_command


def events(names):
    rows = [dict(Action='pass', Package=race.SQLITE, Test=n, Elapsed=.01) for n in names]
    rows.append(dict(Action='pass', Package=race.SQLITE))
    return rows


class SQLiteRaceInventoryTests(unittest.TestCase):
    def test_discovery_includes_examples_and_fuzz_seeds_but_not_benchmarks(self):
        listing = 'TestA\nExampleB\nFuzzC\nBenchmarkD\nok\t' + race.SQLITE + '\t0.01s\n'
        self.assertEqual(race.test_names(listing), ['TestA', 'ExampleB', 'FuzzC'])
        for broken in ('', 'TestA\n', listing + 'TestLate\n',
                       listing.replace('TestA\n', 'TestA\nTestA\n'),
                       listing.replace('TestA', 'TestA|TestInjected'),
                       listing.replace('ok\t', 'unknown\t')):
            with self.subTest(broken=broken), self.assertRaises(ValueError):
                race.test_names(broken)

    def test_partitions_cover_every_root_exactly_once_at_every_capacity(self):
        for count in range(1, 20):
            names = [f'Test{i}' for i in range(count)]
            for workers in (1, 2, 3):
                groups = race.partitions(names, workers)
                self.assertEqual(Counter(n for g in groups for n in g), Counter(names))
                self.assertLessEqual(len(groups), workers)
        for names, workers in (([], 3), (['TestA', 'TestA'], 3), (['TestA'], 0)):
            with self.assertRaises(ValueError):
                race.partitions(names, workers)

    def test_missing_duplicate_foreign_failed_or_partial_execution_rejects_verdict(self):
        names = ['TestA', 'ExampleB', 'FuzzC']
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'events.log'
            good = events(names)
            path.write_text('\n'.join(map(json.dumps, good)))
            self.assertEqual(set(race.sqlite_verdict(path, names)), set(names))
            broken = [good[1:], good + [good[0]], good[:-1],
                      good + [dict(Action='pass', Package=race.SQLITE, Test='TestUnexpected')],
                      [dict(good[0], Action='fail'), *good[1:]],
                      [dict(good[0], Package='foreign/pkg'), *good[1:]],
                      [*good[:-1], dict(good[-1], Action='skip')]]
            for rows in broken:
                path.write_text('\n'.join(map(json.dumps, rows)))
                with self.subTest(rows=rows), self.assertRaises(ValueError):
                    race.sqlite_verdict(path, names)

    def test_cpu_budget_respects_affinity_parent_limit_and_four_core_ceiling(self):
        with patch.object(race.os, 'cpu_count', return_value=16), \
             patch.object(race.os, 'sched_getaffinity', return_value=set(range(8)), create=True):
            for configured, expected in (('4', 4), ('2', 2), ('1', 1)):
                with patch.dict(os.environ, GOMAXPROCS=configured):
                    self.assertEqual(race.cpu_budget(), expected)
            with patch.dict(os.environ, GOMAXPROCS='0'), self.assertRaises(ValueError):
                race.cpu_budget()

    def test_real_dispatch_bounds_workers_and_audits_complete_selection(self):
        names = [*race.SERIAL_SQLITE, 'TestA', 'TestB', 'TestC', 'ExampleD', 'FuzzE', 'TestF']
        lock, barrier = threading.Lock(), threading.Barrier(4)
        active = peak = 0
        isolated_done = threading.Event()
        calls = []
        def execute(command, env, output, stopped, metrics):
            nonlocal active, peak
            metrics.update(owned_group_drained=True, cpu_seconds=.01)
            calls.append(command)
            if '-list' in command:
                output.write_text('\n'.join(names) + '\nok\t' + race.SQLITE + '\t0.01s\n')
                return 0
            selected = command[command.index('-run') + 1][2:-2].split('|') if '-run' in command else None
            if selected == race.SERIAL_SQLITE:
                self.assertEqual(active, 0)
                self.assertEqual(env['GOMAXPROCS'], '4')
                output.write_text('\n'.join(map(json.dumps, events(selected))))
                isolated_done.set()
                return 0
            self.assertTrue(isolated_done.is_set())
            self.assertEqual(env['GOMAXPROCS'], '1')
            with lock:
                active += 1
                peak = max(peak, active)
            barrier.wait(timeout=3)
            if '-run' in command:
                selected = command[command.index('-run') + 1][2:-2].split('|')
                output.write_text('\n'.join(map(json.dumps, events(selected))))
            else:
                self.assertIn('-p=1', command)
                output.write_text('other package passed\n')
            with lock:
                active -= 1
            return 0
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(race, 'run_command', side_effect=execute), \
             patch.object(race, 'cpu_budget', return_value=4):
            result = race.run_remainder([race.SQLITE, 'github.com/z-chenhao/erii/new/pkg'],
                                        Path(directory), threading.Event())
        self.assertEqual(result['returncode'], 0)
        self.assertTrue(result['selection_complete'])
        self.assertTrue(result['owned_group_drained'])
        self.assertEqual(peak, 4)
        self.assertEqual(Counter(n for w in result['workers'] for n in w.get('tests', {})), Counter(names))
        for command in calls:
            for required in ('-race', '-count=1', '-timeout=15m'):
                self.assertIn(required, command)

    def test_locally_valid_partition_cannot_omit_a_global_root(self):
        names = [*race.SERIAL_SQLITE, 'TestA', 'TestB']
        def execute(command, env, output, stopped, metrics):
            metrics.update(owned_group_drained=True, cpu_seconds=.01)
            if '-list' in command:
                output.write_text('\n'.join(names) + '\nok\t' + race.SQLITE + '\t0.01s\n')
            else:
                selected = command[command.index('-run') + 1][2:-2].split('|')
                output.write_text('\n'.join(map(json.dumps, events(selected))))
            return 0
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(race, 'run_command', side_effect=execute), \
             patch.object(race, 'partitions', return_value=[['TestA']]), \
             patch.object(race, 'cpu_budget', return_value=4):
            result = race.run_remainder([race.SQLITE], Path(directory), threading.Event())
        self.assertNotEqual(result['returncode'], 0)
        self.assertFalse(result['selection_complete'])

    def test_cancellation_drains_all_four_real_owned_process_groups(self):
        with tempfile.TemporaryDirectory() as directory:
            folder, stopped = Path(directory), threading.Event()
            def execute(command, env, output, stop, metrics):
                if '-list' in command:
                    metrics.update(owned_group_drained=True, cpu_seconds=0)
                    output.write_text('TestA\nTestB\nTestC\n' + race.SERIAL_SQLITE[0] + '\nok\t' + race.SQLITE + '\t0.01s\n')
                    return 0
                selected = command[command.index('-run') + 1][2:-2].split('|') if '-run' in command else None
                if selected == race.SERIAL_SQLITE:
                    metrics.update(owned_group_drained=True, cpu_seconds=0)
                    output.write_text('\n'.join(map(json.dumps, events(selected))))
                    return 0
                ready = folder / (output.stem + '.ready')
                source = ('import os,signal,time; from pathlib import Path; '
                          'signal.signal(signal.SIGTERM,signal.SIG_IGN); '
                          f'Path({str(ready)!r}).write_text(str(os.getpid())); '
                          'time.sleep(60)')
                return run_command([sys.executable, '-c', source], env, output, stop, metrics)
            with patch.object(race, 'run_command', side_effect=execute), \
                 patch.object(race, 'cpu_budget', return_value=4), \
                 ThreadPoolExecutor(max_workers=1) as pool:
                future = pool.submit(race.run_remainder,
                                     [race.SQLITE, 'github.com/z-chenhao/erii/new/pkg'], folder, stopped)
                try:
                    deadline = time.monotonic() + 3
                    while len(list(folder.glob('*.ready'))) != 4 and time.monotonic() < deadline:
                        time.sleep(.02)
                    self.assertEqual(len(list(folder.glob('*.ready'))), 4)
                finally:
                    stopped.set()
                    result = future.result(timeout=12)
            self.assertNotEqual(result['returncode'], 0)
            self.assertTrue(result['owned_group_drained'])
            self.assertEqual(len(result['workers']), 5)
            for worker in result['workers']:
                self.assertEqual(worker['returncode'], 0 if worker['name'] == 'sqlite-serial' else 130)
                self.assertTrue(worker['owned_group_drained'])
