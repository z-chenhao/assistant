from datetime import datetime, timezone
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

import ci_step


class StepCancellationTests(unittest.TestCase):
    def test_uncooperative_child_stops_before_actions_sigterm_escalation(self):
        root = Path(__file__).resolve().parents[1]
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            ready = folder / 'ready'
            steps = folder / 'steps'
            program = ('import os,signal,time\nfrom pathlib import Path\n'
                       'signal.signal(signal.SIGTERM,signal.SIG_IGN)\n'
                       f'Path({str(ready)!r}).write_text(str(os.getpid()))\n'
                       'time.sleep(60)\n')
            with (folder / 'log').open('w') as log:
                entry = subprocess.Popen([sys.executable, 'scripts/ci_step.py', '--name', 'stubborn',
                    '--phase', 'test', '--evidence', str(steps), '--', sys.executable, '-c', program],
                    cwd=root,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
                try:
                    deadline = time.monotonic() + 5
                    while not ready.exists() and time.monotonic() < deadline:
                        time.sleep(.01)
                    self.assertTrue(ready.exists())
                    os.kill(entry.pid,signal.SIGINT)
                    self.assertEqual(entry.wait(timeout=7.5),130)
                    record=json.loads((steps/'stubborn.json').read_text())
                    self.assertIsNotNone(record['cancel_received_at'])
                    self.assertTrue(record['owned_group_drained'])
                    self.assertLess(record['cancel_cleanup_seconds'],7.5)
                finally:
                    if entry.poll() is None:
                        os.kill(entry.pid,signal.SIGTERM)
                        try: entry.wait(timeout=12)
                        except subprocess.TimeoutExpired:
                            entry.kill()
                            if ready.exists():
                                try: os.killpg(int(ready.read_text()),signal.SIGKILL)
                                except ProcessLookupError: pass
                            entry.wait(timeout=2)

    def test_first_signal_is_recorded_separately_from_cleanup(self):
        times = [datetime(2026, 10, 19, 1, minute, tzinfo=timezone.utc) for minute in (0, 1, 2)]
        handlers = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
        previous = Path.cwd()
        def execute(command, env, output, stopped, metrics):
            self.assertEqual(command, ['fixture-command'])
            self.assertFalse(stopped.is_set())
            signal.getsignal(signal.SIGTERM)(signal.SIGTERM, None)
            self.assertTrue(stopped.is_set())
            receipt = json.loads(Path('evidence/steps/fixture-cancel.json').read_text())
            self.assertEqual(receipt['cancel_received_at'], times[1].isoformat())
            self.assertEqual(receipt['signal'], signal.SIGTERM)
            self.assertFalse(Path('evidence/steps/fixture.json').exists())
            signal.getsignal(signal.SIGINT)(signal.SIGINT, None)
            metrics.update(cpu_seconds=0.1, cancel_cleanup_seconds=0.2, owned_group_drained=True)
            output.write_text('fixture cancelled\n')
            return 130
        try:
            with tempfile.TemporaryDirectory() as directory:
                os.chdir(directory)
                with patch.object(sys, 'argv', ['ci_step.py', '--name', 'fixture', '--phase', 'test', '--', 'fixture-command']), \
                     patch.object(ci_step, 'source_state', return_value={'source_sha': '1' * 40}), \
                     patch.object(ci_step, 'run_command', side_effect=execute), \
                     patch.object(ci_step, 'datetime') as clock:
                    clock.now.side_effect = times
                    self.assertEqual(ci_step.main(), 130)
                record = json.loads(Path('evidence/steps/fixture.json').read_text())
                self.assertEqual(record['started_at'], times[0].isoformat())
                self.assertEqual(record['cancel_received_at'], times[1].isoformat())
                self.assertEqual(record['finished_at'], times[2].isoformat())
                self.assertEqual(record['returncode'], 130)
                self.assertEqual(record['cancel_cleanup_seconds'], 0.2)
                self.assertTrue(record['owned_group_drained'])
                self.assertTrue(record['source_stable'])
                os.chdir(previous)
        finally:
            os.chdir(previous)
            for sig, handler in handlers.items():
                signal.signal(sig, handler)


if __name__ == '__main__':
    unittest.main()
