"""Offline checks for the opt-in L2 harness's ownership and host-file boundary."""
import json
import base64
import contextlib
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import types
import unittest
import urllib.error
from unittest.mock import Mock, patch

import lume_live
from lume_live import ISSUE, Live, MARKER, VM, decode_details, redact


class LumeLiveTests(unittest.TestCase):
    def initialize(self, root):
        for name in ['runtime', 'store', 'config', 'downloads', 'logs', 'evidence', 'cache']:
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

    def test_multiple_vnc_batches_share_one_connection_and_join_on_exit(self):
        with tempfile.TemporaryDirectory() as directory:
            live = Live.__new__(Live)
            live.root = Path(directory)
            (live.root / 'evidence').mkdir()
            live.details = Mock(return_value={'vncUrl': 'vnc://:test-only-pass@127.0.0.1:5901'})
            client = Mock(spec=['pause', 'keyPress', 'captureScreen', 'mouseMove', 'mousePress', 'disconnect'])
            api = Mock()
            api.connect.return_value = client
            commands = io.StringIO('[["key","shift"],["capture","one.png"]]\n[["type","Fixture_a"]]\nquit\n')
            output = io.StringIO()
            class BaseClient:
                width, height = 1280, 720

                def updateDesktopSize(self, width, height):
                    self.image_size = (width, height)

                def framebufferUpdateRequest(self):
                    return self.width, self.height

            modules = {'vncdotool': types.SimpleNamespace(api=api),
                       'vncdotool.client': types.SimpleNamespace(VNCDoToolClient=BaseClient,
                                                               VNCDoToolFactory=type('BaseFactory', (), {}))}
            with patch.dict(sys.modules, modules), contextlib.redirect_stdout(output):
                live.vnc_session(commands)
            self.assertEqual(api.connect.call_count, 1)
            client.captureScreen.assert_called_once_with(str(live.root / 'evidence/one.png'))
            self.assertEqual([call.args[0] for call in client.keyPress.call_args_list], ['shift', 'shift-F', *'ixture', 'shift-_', 'a'])
            client.disconnect.assert_called_once()
            api.shutdown.assert_called_once()
            self.assertNotIn('test-only-pass', output.getvalue())
            protocol = api.connect.call_args.kwargs['factory_class'].protocol()
            protocol.updateDesktopSize(2048, 1280)
            self.assertEqual(protocol.framebufferUpdateRequest(), (2048, 1280),
                             'every capture must request the entire resized framebuffer')

    def test_crash_cleanup_log_does_not_hide_the_owned_descriptor(self):
        payload = {'name': VM, 'status': 'stopped'}
        text = '[2026-10-20T00:00:00Z] INFO: Cleaned up stale session file\n' + json.dumps([payload])
        self.assertEqual(decode_details(text), payload)
        with self.assertRaisesRegex(RuntimeError, 'owned VM descriptor'):
            decode_details(json.dumps([{'name': 'foreign-vm', 'status': 'stopped'}]))

    def test_guest_readback_rejects_corrupt_content_before_writing(self):
        with tempfile.TemporaryDirectory() as directory:
            live = Live.__new__(Live)
            live.root = Path(directory)
            (live.root / 'evidence').mkdir()
            live.guest = Mock(side_effect=['7', base64.b64encode(b'changed').decode(),
                                          hashlib.sha256(b'correct').hexdigest() + ' fixture'])
            with self.assertRaisesRegex(RuntimeError, 'hash mismatch'):
                live.get_fixture('export.mov')
            self.assertFalse((live.root / 'evidence/export.mov').exists())
            live.guest.reset_mock()
            with self.assertRaisesRegex(RuntimeError, 'owned evidence'):
                live.get_fixture('../outside')
            live.guest.assert_not_called()

    def test_driver_capture_saves_exact_bytes_without_base64_in_report(self):
        with tempfile.TemporaryDirectory() as directory:
            live = Live.__new__(Live)
            live.root = Path(directory)
            (live.root / 'evidence').mkdir()
            screenshot = b'synthetic-image-bytes'
            live.guest = Mock(return_value=json.dumps({
                'capture_id': 'fixture-capture',
                'screenshot_png_b64': base64.b64encode(screenshot).decode()}))
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                live.driver('get_desktop_state', '{}')
            self.assertEqual((live.root / 'evidence/get_desktop_state.png').read_bytes(), screenshot)
            report = json.loads(output.getvalue())
            self.assertEqual(report['capture_id'], 'fixture-capture')
            self.assertEqual(report['screenshot_sha256'], hashlib.sha256(screenshot).hexdigest())
            self.assertNotIn('screenshot_png_b64', report)

    def test_failed_observation_cannot_leave_a_stale_latest_screenshot(self):
        with tempfile.TemporaryDirectory() as directory:
            live = Live.__new__(Live)
            live.root = Path(directory)
            (live.root / 'evidence').mkdir()
            latest = live.root / 'evidence/get_desktop_state.png'
            latest.write_bytes(b'old-screen')
            archive = live.root / 'evidence/capture-old.png'
            archive.write_bytes(b'old-screen')
            live.guest = Mock(side_effect=RuntimeError('session has ended; refused'))
            with self.assertRaisesRegex(RuntimeError, 'session has ended'):
                live.driver('get_desktop_state', '{}')
            self.assertFalse(latest.exists())
            self.assertEqual(archive.read_bytes(), b'old-screen')

    def test_cached_running_state_is_reconciled_only_after_native_resources_release(self):
        with tempfile.TemporaryDirectory() as directory:
            live = Live.__new__(Live)
            live.root = Path(directory)
            live.store = live.root / 'store'
            live.port = 17774
            (live.root / 'evidence').mkdir()
            live.details = Mock(return_value={'status': 'running', 'vncUrl': 'vnc://127.0.0.1:5901'})
            live.command = Mock(return_value='Guest shutdown requested')
            live.stopped_resources = Mock(side_effect=[False, True])
            calls = []
            def request(port, method, path, body=None, **_):
                self.assertEqual(live.stopped_resources.call_count, 2)
                calls.append((method, path, body))
                if method == 'POST':
                    self.assertFalse(body['force'])
                    raise urllib.error.HTTPError('http://fixture', 400, 'already stopped', {},
                        io.BytesIO(json.dumps({'message': f'Virtual machine not running: {VM}'}).encode()))
                return {'status': 'stopped' if len(calls) == 3 else 'running'}
            live.request = request
            with patch.object(lume_live.time, 'sleep'), contextlib.redirect_stdout(io.StringIO()):
                live.stop()
            self.assertEqual([call[0] for call in calls], ['GET', 'POST', 'GET'])
            receipt = json.loads((live.root / 'evidence/stop.json').read_text())
            self.assertEqual(receipt['provider_status'], 'stopped')
            self.assertEqual(receipt['reconciliation'], 'already_not_running_cache_cleared')
            self.assertFalse(receipt['poweroff_before_resource_release'])

    def test_stop_timeout_never_dispatches_native_poweroff_or_claims_a_checkpoint(self):
        live = Live.__new__(Live)
        live.root = Path('/synthetic-owned-test-root')
        live.store = Path('/synthetic-owned-store')
        live.details = Mock(return_value={'status': 'running', 'vncUrl': 'vnc://127.0.0.1:5901'})
        live.command = Mock(return_value='Guest shutdown requested')
        live.stopped_resources = Mock(return_value=False)
        live.request = Mock()
        with contextlib.redirect_stdout(io.StringIO()), self.assertRaisesRegex(RuntimeError, 'no power-off'):
            live.stop(timeout=0)
        live.request.assert_not_called()

    def test_fault_injection_refuses_daemon_and_reused_foreign_pid(self):
        with tempfile.TemporaryDirectory() as directory:
            live = Live.__new__(Live)
            live.root = Path(directory)
            live.store = live.root / 'store'
            live.binary = live.root / 'runtime/lume'
            live.port = 17774
            (live.root / 'evidence').mkdir()
            (live.root / 'active-serve.json').write_text(json.dumps({'pid': 555555, 'operation': 'serve'}))
            live.request = Mock(return_value={'status': 'running'})
            with patch.object(lume_live.subprocess, 'check_output', return_value='{"pid":555555}'), \
                    patch.object(lume_live.os, 'kill') as kill:
                with self.assertRaisesRegex(RuntimeError, 'Multica daemon'):
                    live.crash()
                kill.assert_not_called()
            with patch.object(lume_live.subprocess, 'check_output',
                              side_effect=['{"pid":123}', '/foreign/runtime/lume serve --port 17774']), \
                    patch.object(lume_live.os, 'kill') as kill:
                with self.assertRaisesRegex(RuntimeError, 'owned process'):
                    live.crash()
                kill.assert_not_called()
            self.assertFalse((live.root / 'evidence/crash.json').exists())

    def test_fixture_upload_refuses_a_host_file_alias_before_ssh(self):
        with tempfile.TemporaryDirectory(prefix='collie24-upload-', dir=Path.home()) as directory:
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

        with tempfile.TemporaryDirectory(prefix='collie24-download-', dir=Path.home()) as directory:
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
        with tempfile.TemporaryDirectory(prefix='collie24-boundary-', dir=Path.home()) as directory:
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
