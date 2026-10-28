"""Real subprocess regressions for the shared checkpoint reader."""
from pathlib import Path
import sys
import tempfile
import threading
import time
import unittest

from verify_main import MainHarness
from fixture_pool import Scope


class HoldTests(unittest.TestCase):
    def harness(self, root, program):
        harness = MainHarness(Path(sys.executable), root)
        harness.args = lambda *args: [sys.executable, '-c', program]
        self.addCleanup(harness.cleanup)
        return harness

    def test_stderr_backpressure_cannot_prevent_checkpoint(self):
        with tempfile.TemporaryDirectory() as directory:
            harness = self.harness(Path(directory),
                'import os,time\n'
                'os.write(2,b"x"*1048576)\n'
                'os.write(1,b\'{"boundary":"checkpoint"}\\n\')\n'
                'time.sleep(30)\n')
            child = harness.hold('checkpoint', timeout=2)
            self.assertIsNone(child.poll())
            self.assertEqual(harness.hold_observation['bytes_read']['stderr'], 1048576)
            self.assertEqual(len(harness.hold_observation['stderr']), 8000)
            self.assertEqual(harness.hold_observation['reason'], 'reached')

    def test_partial_record_and_live_stderr_obey_the_original_deadline(self):
        with tempfile.TemporaryDirectory() as directory:
            harness = self.harness(Path(directory),
                'import os,time\nos.write(1,b\'{"boundary":\')\ntime.sleep(30)')
            # Bound the pre-fix readline hang so a regression never leaks work.
            watchdog = threading.Timer(1, lambda: harness.children[0].kill())
            watchdog.start()
            try:
                started = time.monotonic()
                with self.assertRaisesRegex(RuntimeError, 'boundary never reached'):
                    harness.hold('checkpoint', timeout=.1)
                self.assertLess(time.monotonic() - started, .5)
                self.assertEqual(harness.hold_observation['stdout'], '{"boundary":')
                self.assertEqual(harness.hold_observation['reason'], 'timeout')
            finally:
                watchdog.cancel()
                watchdog.join()

    def test_invalid_and_early_exit_records_fail_with_bounded_output(self):
        programs = [
            ('import os,time\nos.write(1,b\'{"boundary":"wrong"}\\n\')\ntime.sleep(30)', 'unexpected boundary'),
            ('import os\nos.write(2,b"early exit diagnostic")\nraise SystemExit(7)', 'exited before boundary'),
            ('import os\nos.write(1,b"x"*8001)', 'oversized boundary'),
            ('import os\nos.write(1,b"not json\\n")', 'Expecting value'),
        ]
        for program, message in programs:
            with self.subTest(message=message), tempfile.TemporaryDirectory() as directory:
                harness = self.harness(Path(directory), program)
                with self.assertRaisesRegex((RuntimeError, ValueError), message):
                    harness.hold('checkpoint', timeout=1)
                self.assertFalse(harness.hold_observation['reached'])
                self.assertLessEqual(len(harness.hold_observation['stdout']), 8000)
                if 'early exit diagnostic' in program:
                    self.assertEqual(harness.hold_observation['stderr'], 'early exit diagnostic')

    def test_cancellation_does_not_wait_out_fifteen_second_hold(self):
        with tempfile.TemporaryDirectory() as directory:
            harness = self.harness(Path(directory), 'import time\ntime.sleep(30)')
            harness.processes = Scope()
            timer = threading.Timer(.1, harness.processes.stopped.set)
            timer.start()
            try:
                started = time.monotonic()
                with self.assertRaisesRegex(RuntimeError, 'fixture cancelled'):
                    harness.hold('checkpoint')
                self.assertLess(time.monotonic() - started, .5)
                self.assertEqual(harness.hold_observation['timeout_seconds'], 15)
            finally:
                timer.join()


if __name__ == '__main__':
    unittest.main()
