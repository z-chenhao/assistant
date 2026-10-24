import json
import io
from contextlib import redirect_stderr
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

import verify_memory_maintenance as driver


class FailureEvidenceTests(unittest.TestCase):
    def test_unwritable_evidence_preserves_original_error_and_cleanup(self):
        error = RuntimeError('original setup failure')
        output = Mock()
        output.write_text.side_effect = OSError('private path')
        embeddings = Mock()
        log = io.StringIO()
        with patch.object(driver, 'prepare', side_effect=error), patch.object(driver, 'Embeddings', return_value=embeddings), \
                redirect_stderr(log):
            with self.assertRaises(RuntimeError) as raised:
                driver.run(Path('/unused/erii'), output)
        self.assertIs(raised.exception, error)
        self.assertIn('OSError', log.getvalue())
        self.assertNotIn('private path', log.getvalue())
        embeddings.close.assert_called_once()

    def test_failure_snapshot_precedes_cleanup_and_preserves_original_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp)/'failure.json'
            error = RuntimeError('storage unavailable')
            h = Mock(env={})
            h.command.side_effect = error
            h.failure_state.return_value = dict(runs=[dict(status='running')])
            h.cleanup.side_effect = lambda: self.assertTrue(output.exists())
            embeddings = Mock()
            embeddings.server.server_port = 1
            with patch.object(driver, 'prepare', return_value=h), patch.object(driver, 'Embeddings', return_value=embeddings), \
                    patch.object(driver, 'readback', return_value=dict(state=dict(compaction=[]))):
                with self.assertRaises(RuntimeError) as raised:
                    driver.run(Path('/unused/erii'), output)
            self.assertIs(raised.exception, error)
            receipt = json.loads(output.read_text())
            self.assertIs(receipt['passed'], False)
            self.assertEqual(receipt['runtime']['runs'][0]['status'], 'running')
            self.assertEqual(receipt['memory']['state']['compaction'], [])
            h.cleanup.assert_called_once()
            embeddings.close.assert_called_once()

    def test_snapshot_read_error_does_not_replace_original_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp)/'failure.json'
            error = RuntimeError('original failure')
            h = Mock(env={})
            h.command.side_effect = error
            h.failure_state.side_effect = ValueError('damaged state')
            embeddings = Mock()
            embeddings.server.server_port = 1
            with patch.object(driver, 'prepare', return_value=h), patch.object(driver, 'Embeddings', return_value=embeddings), \
                    patch.object(driver, 'readback', side_effect=OSError('unreadable')):
                with self.assertRaises(RuntimeError) as raised:
                    driver.run(Path('/unused/erii'), output)
            self.assertIs(raised.exception, error)
            receipt = json.loads(output.read_text())
            self.assertEqual(receipt['runtime'], dict(capture_error_type='ValueError'))
            self.assertEqual(receipt['memory'], dict(capture_error_type='OSError'))
            self.assertNotIn('damaged state', output.read_text())
            self.assertNotIn('unreadable', output.read_text())


if __name__ == '__main__':
    unittest.main()
