"""Reject incomplete cancellation evidence; keep pre-cleanup fixture diagnosis."""
from contextlib import redirect_stderr
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

import ci_cancel
import run_core_cases as core


def receipts():
    return [dict(root=100 + index, sidecar=200 + index,
                 counts=dict(completed=37, active=8, total=45),
                 checkpoint='37 completed / 8 active; checked by feedback_case',
                 descendants={str(100 + index): dict(parent=1, start='root', state='Ss'),
                              str(200 + index): dict(parent=100 + index, start='sidecar', state='S')})
            for index in range(4)]


class CancelTests(unittest.TestCase):
    def test_missing_duplicate_and_forged_checkpoint_receipts_reject(self):
        ci_cancel.validate_ready(receipts())
        for mutation in ('missing', 'duplicate', 'counts', 'boundary', 'sidecar', 'identity', 'row', 'root', 'graph'):
            rows = receipts()
            if mutation == 'missing': rows.pop()
            elif mutation == 'duplicate': rows[1] = rows[0]
            elif mutation == 'counts': rows[0]['counts']['completed'] = 36
            elif mutation == 'boundary': rows[0].pop('checkpoint')
            elif mutation == 'sidecar': rows[0]['descendants']['200']['parent'] = 999
            elif mutation == 'identity': rows[0]['descendants']['100'].pop('start')
            elif mutation == 'row': rows[0] = []
            elif mutation == 'root': rows[0]['root'] = []
            elif mutation == 'graph': rows[0]['descendants'] = []
            with self.subTest(mutation=mutation), self.assertRaises(RuntimeError):
                ci_cancel.validate_ready(rows)

    def test_probe_requires_trigger_drain_state_cleanup_and_make_receipt(self):
        for entry in ('direct', 'make-step'):
            for mutation in ('none', 'missing', 'failed', 'drain', 'state', 'queued', 'make'):
                if mutation == 'make' and entry == 'direct': continue
                with self.subTest(entry=entry, mutation=mutation), tempfile.TemporaryDirectory() as directory:
                    binary = Path(directory) / 'erii'
                    binary.write_bytes(b'fixture-binary')
                    def execute(command, env, log, stopped, metrics):
                        root = log.parent
                        log.write_text('retained fixture stderr')
                        metrics.update(owned_group_drained=mutation != 'drain')
                        if mutation == 'missing':
                            return 1
                        for index, row in enumerate(receipts()):
                            (root / f'ready-{index}.json').write_text(json.dumps(row))
                        self.assertTrue(stopped.wait(2), 'probe failed to request cancellation')
                        if mutation == 'state': (root / 'held-core-leak').mkdir()
                        if mutation == 'queued': (root / 'ready-extra.json').write_text('{}')
                        if entry == 'make-step' and mutation != 'make':
                            (root / 'steps').mkdir()
                            (root / 'steps/cancel-probe.json').write_text(json.dumps(dict(
                                returncode=130, owned_group_drained=True, cancel_received_at='recorded')))
                        return 1 if mutation == 'failed' else 130
                    with patch.object(ci_cancel.suite, 'source_state', return_value={'source_sha': 'a' * 40}), \
                         patch.object(ci_cancel.suite, 'run_command', side_effect=execute), \
                         patch.object(ci_cancel, 'processes', return_value={}):
                        if mutation == 'none':
                            result = ci_cancel.run(binary, entry)
                            self.assertTrue(result['passed'])
                            self.assertEqual(result['entry'], entry)
                        else:
                            with self.assertRaises(ci_cancel.ProbeFailure) as raised:
                                ci_cancel.run(binary, entry)
                            self.assertFalse(raised.exception.evidence['passed'])
                            self.assertEqual(raised.exception.evidence['log_tail'], 'retained fixture stderr')

    def test_cli_keeps_failure_report_after_disposable_state_cleanup(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'failure.json'
            failure = dict(passed=False, ready=[], error='not reached', diagnostics=[{'fixture': '0'}])
            with patch.object(sys, 'argv', ['ci_cancel.py', '--binary', 'unused', '--evidence', str(output)]), \
                 patch.object(ci_cancel, 'run', side_effect=ci_cancel.ProbeFailure(failure)):
                with self.assertRaises(ci_cancel.ProbeFailure): ci_cancel.main()
            self.assertEqual(json.loads(output.read_text()), failure)

    def test_feedback_snapshot_is_written_before_cleanup_and_keeps_original_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / 'diagnostics'
            error = RuntimeError('original checkpoint failure')
            harness = Mock(hold_observation=dict(reason='timeout'))
            harness.setup.side_effect = error
            harness.cleanup.side_effect = lambda: self.assertTrue((destination / 'failure-fixture.json').exists())
            with patch.dict(os.environ, ERII_CORE_DIAGNOSTICS=str(destination)), \
                 patch.object(core, 'configure'), patch.object(core, 'CoreHarness', return_value=harness), \
                 patch.object(core, 'feedback_state', return_value=dict(child_status_counts={'completed': 36})), \
                 redirect_stderr(io.StringIO()):
                with self.assertRaises(RuntimeError) as raised:
                    core.feedback_case(root / 'erii', root / 'fixture', {})
            self.assertIs(raised.exception, error)
            self.assertEqual(json.loads((destination / 'failure-fixture.json').read_text())['hold']['reason'], 'timeout')
            harness.cleanup.assert_called_once()

    def test_unwritable_diagnosis_never_replaces_failure_or_skips_cleanup(self):
        error = RuntimeError('original failure')
        harness = Mock(hold_observation=None)
        harness.setup.side_effect = error
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'file'
            path.write_text('unwritable directory')
            with patch.dict(os.environ, ERII_CORE_DIAGNOSTICS=str(path)), \
                 patch.object(core, 'configure'), patch.object(core, 'CoreHarness', return_value=harness), \
                 patch.object(core, 'feedback_state', return_value={}), redirect_stderr(io.StringIO()):
                with self.assertRaises(RuntimeError) as raised:
                    core.feedback_case(Path('erii'), Path('fixture'), {})
            self.assertIs(raised.exception, error)
            harness.cleanup.assert_called_once()


if __name__ == '__main__':
    unittest.main()
