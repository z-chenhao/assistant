"""Regression protection for gate completeness, failures and cancellation."""
import itertools
import json
import os
from pathlib import Path
import re
import signal
import socket
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

import runner as suite


class ProcessLifecycleTests(unittest.TestCase):
    def test_signal_eperm_can_only_accept_confirmed_absence_on_darwin(self):
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'darwin'), \
             patch.object(suite, 'group_alive', return_value=False) as inspect:
            suite.signal_group(42, signal.SIGKILL)
            inspect.assert_called_once_with(42)
            inspect.return_value = True
            with self.assertRaises(PermissionError):
                suite.signal_group(42, signal.SIGTERM)
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'linux'):
            with self.assertRaises(PermissionError):
                suite.signal_group(42, signal.SIGKILL)


    def test_darwin_eperm_requires_authoritative_group_inspection(self):
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'darwin'), \
             patch.object(suite.subprocess, 'check_output') as inspect:
            for table, alive in [('42 Z\n', False), ('42 S\n', True),
                                 ('42 Z\n42 R\n', True), ('99 S\n', False)]:
                inspect.return_value = table
                self.assertEqual(suite.group_alive(42), alive)
            inspect.return_value = ''
            with self.assertRaises(OSError):
                suite.group_alive(42)
            inspect.side_effect = PermissionError
            with self.assertRaises(PermissionError):
                suite.group_alive(42)
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'linux'):
            with self.assertRaises(PermissionError):
                suite.group_alive(42)


    def test_unverifiable_group_cleanup_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            metrics = {}
            with patch.object(suite, 'group_alive', side_effect=OSError('ps unavailable')):
                code = suite.run_command([sys.executable, '-c', 'pass'], os.environ.copy(),
                                         Path(temp) / 'log', threading.Event(), metrics)
            self.assertNotEqual(code, 0)
            self.assertFalse(metrics['owned_group_drained'])


    def test_cancellation_reaps_owned_process_and_runs_finally(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            script = root / 'fixture.py'
            ready, cleaned = root / 'ready', root / 'cleaned'
            script.write_text('from pathlib import Path\nimport time\n'
                              'def scenario(name):\n try:\n'
                              f'  Path({str(ready)!r}).touch()\n  time.sleep(60)\n finally:\n'
                              f'  Path({str(cleaned)!r}).touch()\nscenario("owned-case")\n')
            stopped = threading.Event()
            def interrupt():
                deadline = time.monotonic() + 5
                while not ready.exists() and time.monotonic() < deadline:
                    time.sleep(.01)
                stopped.set()
            thread = threading.Thread(target=interrupt)
            thread.start()
            env = dict(os.environ, COLLIE_TEST_METRICS_DIR=str(root / 'profile'))
            try:
                code = suite.run_command([sys.executable, 'tests/system/entrypoint.py', str(script)],
                                         env, root / 'log', stopped)
            finally:
                thread.join(timeout=6)
            self.assertNotEqual(code, 0)
            self.assertTrue(ready.exists())
            self.assertTrue(cleaned.exists())
            profile = json.loads((root / 'profile/fixture.json').read_text())
            self.assertGreater(profile['elapsed_seconds'], 0)
            self.assertGreaterEqual(profile['cpu_seconds'], 0)


    def test_cancelled_leader_cannot_leave_child_listening(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            ready = root / 'ready.json'
            state = root / 'state'
            listener = ('import json,os,signal,socket,time\nfrom pathlib import Path\n'
                        'signal.signal(signal.SIGTERM, signal.SIG_IGN)\n'
                        's=socket.socket();s.bind(("127.0.0.1",0));s.listen()\n'
                        f'Path({str(ready)!r}).write_text(json.dumps([os.getpid(),s.getsockname()[1]]))\n'
                        'time.sleep(60)\n')
            leader = ('import subprocess,sys,time,shutil\nfrom pathlib import Path\n'
                      f'Path({str(state)!r}).mkdir()\n'
                      f'subprocess.Popen([sys.executable,"-c",{listener!r}])\n'
                      'try: time.sleep(60)\n'
                      f'finally: shutil.rmtree({str(state)!r})\n')
            fixture = root / 'leader.py'
            fixture.write_text(leader)
            stopped = threading.Event()
            def interrupt():
                deadline = time.monotonic() + 5
                while not ready.exists() and time.monotonic() < deadline:
                    time.sleep(.01)
                stopped.set()
            thread = threading.Thread(target=interrupt)
            thread.start()
            metrics = {}
            started = time.monotonic()
            try:
                code = suite.run_command([sys.executable, 'tests/system/entrypoint.py', str(fixture)],
                                         dict(os.environ, COLLIE_TEST_METRICS_DIR=str(root / 'profile')),
                                         root / 'log', stopped, metrics)
                self.assertNotEqual(code, 0)
                self.assertFalse(state.exists())
                self.assertTrue(ready.exists(), (root / 'log').read_text())
                _, port = json.loads(ready.read_text())
                deadline = time.monotonic() + 2
                while time.monotonic() < deadline:
                    with socket.socket() as probe:
                        if probe.connect_ex(('127.0.0.1', port)) != 0:
                            break
                    time.sleep(.01)
                else:
                    self.fail('cancelled group left a live listener')
                self.assertLess(time.monotonic() - started, 15)
                self.assertGreaterEqual(metrics['cpu_seconds'], 0)
                self.assertLess(metrics['cancel_cleanup_seconds'], 15)
                self.assertTrue(metrics['owned_group_drained'])
            finally:
                thread.join(timeout=6)
                # Clean the regression's deliberate orphan even on the old code.
                if ready.exists():
                    pid, _ = json.loads(ready.read_text())
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
