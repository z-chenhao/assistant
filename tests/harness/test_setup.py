"""Real npm against a deterministic local registry; no accounts or downloads."""
from concurrent.futures import ThreadPoolExecutor
import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import threading
import time
from types import SimpleNamespace
import unittest

import setup as installer


def archive(version):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as bundle:
        data = json.dumps(dict(name='fixture', version=version, scripts={'install': 'exit 99'})).encode()
        entry = tarfile.TarInfo('package/package.json')
        entry.size = len(data)
        bundle.addfile(entry, io.BytesIO(data))
    return output.getvalue()


class Registry(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers.get('Content-Length', 0)))
        self.reply(b'{}')

    def do_GET(self):
        self.reply(self.server.payloads.get(self.path, b'{}'), 200 if self.path in self.server.payloads else 404)

    def reply(self, body, status=200):
        self.server.requests.append((self.command, self.path, len(body)))
        self.send_response(status)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class SetupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Registry)
        self.server.requests = []
        self.server.payloads = {f'/fixture-{v}.tgz': archive(v) for v in ('1.0.0', '2.0.0')}
        thread = threading.Thread(target=self.server.serve_forever)
        thread.start()
        def close():
            self.server.shutdown()
            self.server.server_close()
            thread.join()
        self.addCleanup(close)
        self.env = dict(os.environ, npm_config_cache=str(self.base / 'cache'), npm_config_loglevel='silent',
                        npm_config_userconfig=str(self.base / 'user.npmrc'), npm_config_globalconfig=str(self.base / 'global.npmrc'),
                        npm_config_registry=f'http://127.0.0.1:{self.server.server_port}')
        # CI may itself be offline; each fixture explicitly tests its own mode.
        self.env.pop('COLLIE_SETUP_NETWORK_LOG', None)
        self.env.pop('NODE_OPTIONS', None)
        self.root = self.tree('first')

    def tree(self, name):
        root = self.base / name
        root.mkdir()
        (root / '.node-version').write_text(subprocess.check_output(['node', '-p', 'process.versions.node'], text=True))
        for package in installer.PACKAGES:
            (root / package).mkdir(parents=True)
        self.dependencies(root, '1.0.0')
        return root

    def dependencies(self, root, version, **constraints):
        payload = self.server.payloads[f'/fixture-{version}.tgz']
        integrity = 'sha512-' + base64.b64encode(hashlib.sha512(payload).digest()).decode()
        for package in installer.PACKAGES:
            metadata = dict(name='fixture-consumer', version='1.0.0', dependencies={'fixture': version})
            (root / package / 'package.json').write_text(json.dumps(metadata))
            lock = dict(name='fixture-consumer', version='1.0.0', lockfileVersion=3, packages={
                '': metadata, 'node_modules/fixture': dict(version=version,
                    resolved=f'http://127.0.0.1:{self.server.server_port}/fixture-{version}.tgz',
                    integrity=integrity, hasInstallScript=True, **constraints)})
            (root / package / 'package-lock.json').write_text(json.dumps(lock))

    def install(self, root=None, mode='online', env=None, part='packages'):
        report = dict(steps=[])
        installer.setup(root or self.root, SimpleNamespace(mode=mode, part=part, timeout=20, with_deps=False),
                        report, dict(env or self.env))
        return report

    def payload_requests(self):
        return [row for row in self.server.requests if row[0] == 'GET' and row[1].endswith('.tgz')]

    def test_cold_warm_offline_two_independent_trees_keep_audit_and_integrity(self):
        cold = self.install()
        self.assertEqual(len(self.payload_requests()), 1)
        self.assertEqual(len(self.server.requests), 5)  # one payload, four audits
        self.assertEqual(self.payload_requests()[0][2], len(self.server.payloads['/fixture-1.0.0.tgz']))
        second = self.tree('second')
        self.server.requests.clear()
        warm = self.install(second)
        self.assertEqual(cold['identity'], warm['identity'])
        self.assertEqual(len(self.server.requests), 4)  # audits still online
        self.assertEqual(self.payload_requests(), [])
        self.server.requests.clear()
        for root in (self.root, second):
            self.install(root, mode='offline')
            for package in installer.PACKAGES:
                installed = json.loads((root / package / 'node_modules/fixture/package.json').read_text())
                self.assertEqual(installed['version'], '1.0.0')
        self.assertEqual(self.server.requests, [])
        self.assertNotEqual(os.stat(self.root / installer.PACKAGES[0] / 'node_modules/fixture/package.json').st_ino,
                            os.stat(second / installer.PACKAGES[0] / 'node_modules/fixture/package.json').st_ino)

    def test_lock_change_offline_missing_then_online_fetches_only_missing_object(self):
        before = self.install()
        self.dependencies(self.root, '2.0.0')
        self.server.requests.clear()
        with self.assertRaises(RuntimeError):
            self.install(mode='offline')
        self.assertEqual(self.server.requests, [])
        after = self.install()
        self.assertNotEqual(before['identity']['lock_sha256'], after['identity']['lock_sha256'])
        self.assertEqual([row[1] for row in self.payload_requests()], ['/fixture-2.0.0.tgz'])

    def test_corrupted_cache_is_not_used_offline_and_online_repairs_one_object(self):
        self.install()
        digest = hashlib.sha512(self.server.payloads['/fixture-1.0.0.tgz']).hexdigest()
        path = self.base / 'cache/_cacache/content-v2/sha512' / digest[:2] / digest[2:4] / digest[4:]
        path.chmod(0o600)
        path.write_bytes(b'corrupt')
        self.server.requests.clear()
        with self.assertRaises(RuntimeError):
            self.install(mode='offline')
        self.assertEqual(self.server.requests, [])
        self.install()
        self.assertEqual(len(self.payload_requests()), 1)
        self.assertEqual(path.read_bytes(), self.server.payloads['/fixture-1.0.0.tgz'])

    def test_empty_offline_cache_and_lock_manifest_conflict_fail_without_downloads(self):
        with self.assertRaises(RuntimeError):
            self.install(mode='offline')
        metadata = self.root / installer.PACKAGES[0] / 'package.json'
        value = json.loads(metadata.read_text())
        value['dependencies']['fixture'] = '2.0.0'
        metadata.write_text(json.dumps(value))
        with self.assertRaises(RuntimeError):
            self.install()
        self.assertEqual(self.payload_requests(), [])

    def test_wrong_node_version_and_platform_arch_fail_closed(self):
        pin = self.root / '.node-version'
        correct = pin.read_text()
        pin.write_text('0.0.0')
        with self.assertRaisesRegex(RuntimeError, 'no installation attempted'):
            self.install()
        self.assertEqual(self.server.requests, [])
        pin.write_text(correct)
        for constraints in ({'os': ['not-an-os']}, {'cpu': ['not-an-arch']}):
            self.dependencies(self.root, '1.0.0', **constraints)
            with self.assertRaises(RuntimeError):
                self.install()
        self.assertEqual(self.payload_requests(), [])

    def test_two_worktrees_share_only_native_download_cache_concurrently(self):
        other = self.tree('other')
        self.install()  # complete native cache
        self.server.requests.clear()
        with ThreadPoolExecutor(max_workers=2) as pool:
            jobs = [pool.submit(self.install, root, 'offline') for root in (self.root, other)]
            for job in jobs:
                self.assertEqual(len(job.result()['steps']), 4)
        self.assertEqual(self.server.requests, [])

    def test_selected_ci_packages_keep_unconsumed_dependencies_uninstalled(self):
        report = dict(steps=[])
        installer.setup(self.root, SimpleNamespace(mode='online', part='packages', timeout=20,
                                                   with_deps=False, packages=list(installer.PACKAGES[:2])),
                        report, dict(self.env))
        self.assertEqual([step['name'] for step in report['steps']], list(installer.PACKAGES[:2]))
        self.assertEqual(len(self.payload_requests()), 1)
        for name in installer.PACKAGES[2:]:
            self.assertFalse((self.root / name / 'node_modules').exists())

    def test_same_tree_lock_wait_is_bounded_and_next_owner_recovers(self):
        with installer.worktree_lock(self.root, 1):
            with self.assertRaisesRegex(RuntimeError, 'lock wait timed out'):
                with installer.worktree_lock(self.root, .1):
                    self.fail('two owners')
        with installer.worktree_lock(self.root, .1):
            pass

    def test_stage_timeout_joins_descendants(self):
        pid_file = self.base / 'pid'
        script = ('import subprocess,time; from pathlib import Path; '
                  'p=subprocess.Popen(["sleep","30"]); '
                  f'Path({str(pid_file)!r}).write_text(str(p.pid)); time.sleep(30)')
        started = time.monotonic()
        with self.assertRaises(subprocess.TimeoutExpired):
            installer.run(['python3', '-c', script], self.root, self.env, .3)
        self.assertLess(time.monotonic() - started, 3)
        # A killed child may briefly be a zombie until init reaps it.
        state = subprocess.run(['ps', '-o', 'stat=', '-p', pid_file.read_text()], capture_output=True, text=True)
        self.assertTrue(not state.stdout.strip() or state.stdout.strip().startswith('Z'), state.stdout)

    def test_http_probe_counts_body_bytes_without_changing_streams_or_logging_queries(self):
        log = self.base / 'network.jsonl'
        env = dict(self.env, NODE_OPTIONS='--require=' + str(installer.ROOT / 'scripts/setup_network.cjs'),
                   COLLIE_SETUP_NETWORK_LOG=str(log))
        script = (f'require("http").get("http://127.0.0.1:{self.server.server_port}/fixture-1.0.0.tgz", '
                  'res=>res.resume())')
        installer.run(['node', '-e', script], self.root, env, 10)
        row, = [json.loads(line) for line in log.read_text().splitlines()]
        self.assertEqual(row['response_body_bytes'], len(self.server.payloads['/fixture-1.0.0.tgz']))
        self.assertEqual(row['kind'], 'npm-payload')
        self.assertNotIn('path', row)


class BrowserSetupTests(unittest.TestCase):
    def test_offline_browser_miss_never_calls_installer(self):
        # The same pinned module as production, but an empty isolated binary cache.
        if not (installer.ROOT / 'guest-browser/node_modules/playwright-core').exists():
            self.fail('python3 scripts/setup.py --part packages required before tests')
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            env = dict(os.environ, PLAYWRIGHT_BROWSERS_PATH=tmp)
            report = dict(steps=[])
            with patch.object(installer, 'identity', return_value={}):
                with self.assertRaises(RuntimeError):
                    installer.setup(installer.ROOT,
                                    SimpleNamespace(mode='offline', part='browser', timeout=20, with_deps=False), report, env)
            self.assertEqual([step['name'] for step in report['steps']], ['chromium-doctor'])
            self.assertEqual(list(Path(tmp).iterdir()), [])

    def test_damaged_browser_cache_is_rejected_without_download(self):
        with tempfile.TemporaryDirectory() as tmp:
            env = dict(os.environ, PLAYWRIGHT_BROWSERS_PATH=tmp)
            # Query the pinned platform mapping rather than hardcode a host path.
            script = ('const r=require("./guest-browser/node_modules/playwright-core/lib/coreBundle").registry; '
                      'console.log(JSON.stringify(["chromium","chromium-headless-shell","ffmpeg"].map(n=>{'
                      'const e=r.registry.findExecutable(n);return {path:e.executablePath(),'
                      'marker:r.browserDirectoryToMarkerFilePath(e.directory)}})))')
            entries = json.loads(installer.run(['node', '-e', script], installer.ROOT, env, 10, capture=True))
            for entry in entries:
                path = Path(entry['path'])
                path.parent.mkdir(parents=True)
                path.write_text('#!/bin/sh\nexit 42\n')
                path.chmod(0o755)
                Path(entry['marker']).touch()
            with self.assertRaises(RuntimeError):
                installer.run(['node', 'scripts/setup_browser.cjs'], installer.ROOT, env, 20, capture=True)
            self.assertFalse((Path(tmp) / '.links').exists())


if __name__ == '__main__':
    unittest.main()
