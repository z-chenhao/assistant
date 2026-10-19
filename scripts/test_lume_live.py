"""Offline checks for the opt-in L2 harness's ownership and host-file boundary."""
import json
import contextlib
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest.mock import Mock, patch

import lume_live
from lume_live import ISSUE, Live, MARKER, VM, redact


class LumeLiveTests(unittest.TestCase):
    def initialize(self, root):
        for name in ['runtime', 'store', 'config', 'downloads', 'logs', 'evidence']:
            (root / name).mkdir()
        (root / MARKER).write_text(json.dumps({'issue_id': ISSUE, 'vm': VM}))

    def test_unowned_directory_refuses_before_writing(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            before = list(root.iterdir())
            with self.assertRaisesRegex(RuntimeError, 'ownership marker'):
                Live(root)
            self.assertEqual(list(root.iterdir()), before)

    def test_symlink_owned_store_is_rejected_before_profile_write(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            self.initialize(root)
            (root / 'store').rmdir()
            (root / 'store').symlink_to(root / 'evidence', target_is_directory=True)
            with self.assertRaisesRegex(RuntimeError, 'must not be symlinks'):
                Live(root)
            self.assertFalse((root / 'host-isolation.sb').exists())

    def test_vnc_authentication_is_not_printed(self):
        text = 'connect vnc://:test-only-pass@127.0.0.1:5901 password=test-only-pass'
        self.assertNotIn('test-only-pass', redact(text))

    def test_fixture_upload_refuses_a_host_file_alias_before_ssh(self):
        with tempfile.TemporaryDirectory(prefix='erii24-upload-', dir=Path.home()) as directory:
            outer = Path(directory)
            root = outer / 'test-root'
            root.mkdir()
            self.initialize(root)
            outside = outer / 'synthetic-host-probe'
            outside.write_bytes(b'must remain outside Guest')
            (root / 'fixtures').mkdir()
            (root / 'fixtures' / 'alias').symlink_to(outside)
            live = Live(root)
            live.command = Mock()
            with self.assertRaisesRegex(RuntimeError, 'owned synthetic fixture'):
                live.put_fixture('alias')
            live.command.assert_not_called()
            self.assertEqual(outside.read_bytes(), b'must remain outside Guest')

    def test_other_server_store_refuses_before_lifecycle_request(self):
        calls = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                calls.append(('GET', self.path))
                value = {'defaultLocationName': 'personal',
                         'vmLocations': [{'name': 'personal', 'path': '/synthetic-personal-store'}],
                         'unrelated': '/synthetic-owned-store'}
                body = json.dumps(value).encode()
                self.send_response(200)
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_POST(self):
                calls.append(('POST', self.path))
                self.send_error(500)

        with ThreadingHTTPServer(('127.0.0.1', 0), Handler) as server:
            thread = threading.Thread(target=server.serve_forever)
            thread.start()
            try:
                live = Live.__new__(Live)
                live.store = Path('/synthetic-owned-store')
                with self.assertRaisesRegex(RuntimeError, 'exclusively own'):
                    live.request(server.server_port, 'POST', f'vms/{VM}/run', {})
                self.assertEqual(calls, [('GET', '/lume/config')])
            finally:
                server.shutdown()
                thread.join(timeout=5)
                self.assertFalse(thread.is_alive())

    def test_image_resume_reuses_hashed_ranges_and_repairs_corruption(self):
        content = bytes(range(256)) * 512
        calls = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                bounds = self.headers['Range'].removeprefix('bytes=').split('-')
                start, end = map(int, bounds)
                calls.append(start)
                body = content[start:end + 1]
                self.send_response(206)
                self.send_header('Content-Length', str(len(body)))
                self.send_header('Content-Range', f'bytes {start}-{end}/{len(content)}')
                self.end_headers()
                self.wfile.write(body)

        with tempfile.TemporaryDirectory(prefix='erii24-download-', dir=Path.home()) as directory:
            root = Path(directory)
            self.initialize(root)
            live = Live(root)
            with ThreadingHTTPServer(('127.0.0.1', 0), Handler) as server:
                thread = threading.Thread(target=server.serve_forever)
                thread.start()
                try:
                    with patch.multiple(lume_live, IPSW_URL=f'http://127.0.0.1:{server.server_port}/image',
                                        IPSW_SIZE=len(content), DOWNLOAD_RANGE_BYTES=16384,
                                        IPSW_SHA256=hashlib.sha256(content).hexdigest()), contextlib.redirect_stdout(io.StringIO()):
                        live.download()
                        self.assertCountEqual(calls, list(range(0, len(content), 16384)))
                        image = root / 'downloads' / lume_live.IPSW_NAME
                        self.assertEqual(image.read_bytes(), content)
                        calls.clear()
                        live.download()
                        self.assertEqual(calls, [], 'completed ranges must be reused')
                        with image.open('r+b') as output:
                            output.write(b'corrupt!')
                        live.download()
                        self.assertEqual(calls, [0], 'only the corrupted range may be fetched')
                        self.assertEqual(image.read_bytes(), content)
                        calls.clear()
                        journal = root / 'downloads' / 'ipsw-ranges.json'
                        receipts = json.loads(journal.read_text())
                        receipts.pop('16384')
                        journal.write_text(json.dumps(receipts))
                        live.download()
                        self.assertEqual(calls, [16384], 'file length cannot replace a committed receipt')
                        with patch.object(lume_live, 'IPSW_SHA256', '0' * 64):
                            with self.assertRaisesRegex(RuntimeError, 'SHA-256 mismatch'):
                                live.download()
                finally:
                    server.shutdown()
                    thread.join(timeout=5)
                    self.assertFalse(thread.is_alive())

    @unittest.skipUnless(sys.platform == 'darwin', 'macOS Seatbelt is a live-host boundary')
    def test_seatbelt_denies_owned_host_probe_outside_test_store(self):
        # Both files are disposable fixtures; this never probes a personal file.
        with tempfile.TemporaryDirectory(prefix='erii24-boundary-', dir=Path.home()) as directory:
            outer = Path(directory)
            root = outer / 'test-root'
            root.mkdir()
            self.initialize(root)
            outside = outer / 'synthetic-host-probe'
            outside.write_text('synthetic fixture')
            live = Live(root)
            denied = subprocess.run(['/usr/bin/sandbox-exec', '-f', str(live.profile),
                                     '/bin/cat', str(outside)], capture_output=True)
            self.assertNotEqual(denied.returncode, 0)
            self.assertNotIn(b'synthetic fixture', denied.stdout)
            inside = root / 'evidence' / 'guest-probe'
            inside.write_text('allowed fixture')
            allowed = subprocess.run(['/usr/bin/sandbox-exec', '-f', str(live.profile),
                                      '/bin/cat', str(inside)], capture_output=True)
            self.assertEqual(allowed.returncode, 0, allowed.stderr)
            self.assertEqual(allowed.stdout, b'allowed fixture')


if __name__ == '__main__':
    unittest.main()
