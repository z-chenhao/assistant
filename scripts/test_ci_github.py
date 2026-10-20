"""Evidence reads recover transport loss without rerunning tests or leaking URLs."""
import io
import subprocess
import unittest
from unittest.mock import patch

import ci_github as github


class GitHubReadTests(unittest.TestCase):
    def test_transient_json_read_recovers(self):
        with patch.object(github.subprocess, 'run', side_effect=[
                subprocess.CompletedProcess([], 1, b'', b'Get https://signed.invalid/?secret=value: EOF'),
                subprocess.CompletedProcess([], 0, b'{"ok":true}', b'')]) as dispatch, \
                patch.object(github.time, 'sleep'):
            self.assertEqual(github.gh('api', 'repos/owner/repo/actions/runs/1'), {'ok': True})
        self.assertEqual(dispatch.call_count, 2)

    def test_partial_binary_download_is_replaced(self):
        stream = io.BytesIO()
        count = []
        def download(_, stdout, **kwargs):
            self.assertEqual(stream.getvalue(), b'')
            count.append(1)
            stdout.write(b'partial' if len(count) == 1 else b'complete ZIP')
            return subprocess.CompletedProcess([], int(len(count) == 1), None, b'EOF' if len(count) == 1 else b'')
        with patch.object(github.subprocess, 'run', side_effect=download), patch.object(github.time, 'sleep'):
            github.read('api', 'repos/owner/repo/actions/artifacts/1/zip', stdout=stream)
        self.assertEqual(stream.getvalue(), b'complete ZIP')

    def test_transport_failure_is_bounded_and_redacted(self):
        with patch.object(github.subprocess, 'run', return_value=subprocess.CompletedProcess(
                [], 1, b'', b'Get https://signed.invalid/?secret=value: EOF')) as dispatch, \
                patch.object(github.time, 'sleep'):
            with self.assertRaises(github.ReadError) as failure:
                github.read('api', 'repos/owner/repo/actions/runs/1')
        self.assertEqual(dispatch.call_count, 3)
        self.assertNotIn('secret', str(failure.exception))

    def test_auth_failure_and_failed_checks_are_never_retried(self):
        for args, error in [(('api', 'repos/owner/repo'), b'HTTP 403'),
                            (('pr', 'checks', '24', '--watch'), b'')]:
            with patch.object(github.subprocess, 'run', return_value=subprocess.CompletedProcess([], 1, b'', error)) as dispatch:
                if error:
                    with self.assertRaises(github.ReadError): github.read(*args)
                else:
                    self.assertEqual(github.read(*args).returncode, 1)
            self.assertEqual(dispatch.call_count, 1)

    def test_mutations_never_dispatch(self):
        with patch.object(github.subprocess, 'run') as dispatch:
            for args in [('run', 'rerun', '1'), ('api', '-X', 'POST', 'repos/owner/repo'),
                         ('api', 'graphql'), ('api', 'repos/owner/repo', '-f', 'x=y'),
                         ('pr', 'edit', '24'), ('pr', 'view', '24', '--web')]:
                with self.assertRaises(ValueError): github.read(*args)
            dispatch.assert_not_called()
