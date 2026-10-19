"""Independent fixtures retain every case and cancel nested owned sessions."""
import json
import os
from pathlib import Path
import signal
import socket
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

import subprocess
import terminal_driver

import fixture_pool as fixtures
import verify_suite as suite


class FixturePoolTests(unittest.TestCase):
    def test_cancelled_terminal_launch_closes_all_pty_descriptors(self):
        descriptors = []
        openpty, dup = terminal_driver.pty.openpty, os.dup
        def allocate():
            pair = openpty()
            descriptors.extend(pair)
            return pair
        def duplicate(fd):
            result = dup(fd)
            descriptors.append(result)
            return result
        scope = fixtures.Scope()
        scope.cancel()
        with patch.object(terminal_driver, 'current', return_value=scope), \
             patch.object(terminal_driver.pty, 'openpty', side_effect=allocate), \
             patch.object(terminal_driver.os, 'dup', side_effect=duplicate):
            with self.assertRaisesRegex(RuntimeError, 'cancelled'):
                terminal_driver.Terminal([sys.executable, '-c', 'pass'], os.environ.copy(), Path.cwd())
        self.assertEqual(len(descriptors), 3)
        for fd in descriptors:
            with self.assertRaises(OSError):
                os.fstat(fd)

    def test_failed_fixture_does_not_stop_an_unrelated_process(self):
        unrelated = subprocess.Popen([sys.executable, '-c', 'import time; print("ready",flush=True); time.sleep(30)'],
                                     stdout=subprocess.PIPE, text=True)
        try:
            self.assertEqual(unrelated.stdout.readline(), 'ready\n')
            def fail(_):
                raise RuntimeError('fixture failure')
            with self.assertRaisesRegex(RuntimeError, 'fixture failure'):
                fixtures.run_cases([1, 2], fail, 2)
            self.assertIsNone(unrelated.poll())
            self.assertIsNone(fixtures.current())
        finally:
            unrelated.kill()
            unrelated.communicate(timeout=5)

    def test_complete_ordered_results_and_bound(self):
        lock, running, peak = threading.Lock(), 0, 0
        def action(value):
            nonlocal running, peak
            with lock:
                running += 1
                peak = max(peak, running)
            result = fixtures.run([sys.executable, '-c', 'import time; time.sleep(.03); print("done")'],
                                  capture_output=True, text=True, timeout=5, check=True)
            with lock:
                running -= 1
            self.assertEqual(result.stdout, 'done\n')
            return value
        self.assertEqual(fixtures.run_cases(list(range(20)), action, 2), list(range(20)))
        self.assertEqual(peak, 2)
        peak = 0
        self.assertEqual(fixtures.run_cases(list(range(20)), action, 4), list(range(20)))
        self.assertEqual(peak, 4)
        self.assertIsNone(fixtures.current())
        for invalid in [0, 3, 8, True, '2']:
            with self.assertRaises(ValueError):
                fixtures.run_cases([], action, invalid)

    def test_cancelled_scope_cannot_launch_a_late_process(self):
        scope = fixtures.Scope()
        scope.cancel()
        with self.assertRaisesRegex(RuntimeError, 'cancelled'):
            scope.start([sys.executable, '-c', 'raise SystemExit(0)'])
        self.assertFalse(scope.children)

    def test_sigterm_joins_two_stubborn_sessions_and_cleans_state(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            state = root / 'state'
            child = ('import json,os,signal,socket,sys,time\nfrom pathlib import Path\n'
                     'signal.signal(signal.SIGTERM, signal.SIG_IGN)\n'
                     's=socket.socket();s.bind(("127.0.0.1",0));s.listen()\n'
                     f'p=Path({str(root)!r})/("ready-"+sys.argv[1]+".json")\n'
                     'p.with_suffix(".tmp").write_text(json.dumps([os.getpid(),s.getsockname()[1]]))\n'
                     'p.with_suffix(".tmp").replace(p)\n'
                     'time.sleep(60)\n')
            program = ('import shutil,sys\nfrom pathlib import Path\n'
                       'from fixture_pool import run_cases,run\n'
                       f'state=Path({str(state)!r});state.mkdir()\n'
                       'def action(index):\n'
                       f' result=run([sys.executable,"-c",{child!r},str(index)],capture_output=True,text=True,timeout=20)\n'
                       ' if result.returncode: raise RuntimeError("cancelled child")\n'
                       ' return index\n'
                       'try: run_cases(list(range(20)),action,2)\n'
                       'finally: shutil.rmtree(state)\n')
            script = root / 'fixture.py'
            script.write_text(program)
            stopped = threading.Event()
            ready = []
            def interrupt():
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline:
                    paths = list(root.glob('ready-*.json'))
                    if len(paths) == 2:
                        ready.extend(json.loads(p.read_text()) for p in paths)
                        break
                    time.sleep(.01)
                stopped.set()
            waiter = threading.Thread(target=interrupt)
            waiter.start()
            metrics = {}
            started = time.monotonic()
            try:
                env = dict(os.environ, PYTHONPATH=str(suite.ROOT / 'scripts'), ERII_PROFILE_DIR=str(root / 'profiles'))
                code = suite.run_command([sys.executable, 'scripts/profile_harness.py', str(script)],
                                         env, root / 'log', stopped, metrics)
                self.assertNotEqual(code, 0)
                self.assertEqual(len(ready), 2, (root / 'log').read_text())
                self.assertEqual(len(list(root.glob('ready-*.json'))), 2, 'queued fixtures started after cancellation')
                self.assertFalse(state.exists(), (root / 'log').read_text())
                self.assertTrue(metrics['owned_group_drained'])
                self.assertLess(time.monotonic() - started, 15)
                self.assertTrue((root / 'profiles/fixture.json').is_file())
                for _, port in ready:
                    with socket.socket() as probe:
                        self.assertNotEqual(probe.connect_ex(('127.0.0.1', port)), 0)
            finally:
                waiter.join(timeout=6)
                for pid, _ in ready:
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass


if __name__ == '__main__':
    unittest.main()
