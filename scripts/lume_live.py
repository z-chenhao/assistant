#!/usr/bin/env python3
"""Opt-in L2 Lume acceptance mechanics; never part of deterministic CI.

Only an explicitly owned test root is accepted. Every Lume process is confined
from the host user's other files. `serve` stays in the foreground; no LaunchAgent
or persistent service is installed. VM credentials never enter report output.
"""
import argparse
import base64
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import subprocess
import sys
import threading
import time
import urllib.parse
import urllib.request

ISSUE = '01a14029-4eab-7782-afba-3980c6c98a7c'
VM = 'erii24-base'
LUME_VERSION = '0.6.1'
MARKER = 'erii24-owned.json'
IPSW_NAME = 'UniversalMac_27.0.1_26A434_Restore.ipsw'
IPSW_URL = ('https://updates.cdn-apple.com/2026FallFCS/'
            '59241290-5d51-4ca8-9df4-31624b9a4eac/' + IPSW_NAME)
IPSW_SIZE = 26637307067
IPSW_SHA256 = '2f016638293c3e641b8b25391a76fbc16563b3711915a5551cf8aa0f5598a5c1'
DOWNLOAD_RANGE_BYTES = 128 * 1024 * 1024


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def redact(text):
    text = re.sub(r'vnc://\S+', '[redacted VNC endpoint]', text)
    return re.sub(r'(?i)(password[=:]\s*)\S+', r'\1[redacted]', text)


class Live:
    def __init__(self, root):
        root = Path(root).absolute()
        require(not any(p.is_symlink() for p in [root, *root.parents]),
                'test root must not contain symlinks')
        require(root.is_dir(), 'create the explicitly authorized test root first')
        self.root = root
        marker = root / MARKER
        require(marker.is_file() and not marker.is_symlink(), 'missing ownership marker')
        require(json.loads(marker.read_text()) == {'issue_id': ISSUE, 'vm': VM},
                'test root belongs to a different operation')
        for name in ['runtime', 'store', 'config', 'downloads', 'logs', 'evidence']:
            require((root / name).is_dir() and not (root / name).is_symlink(),
                    'owned directories must exist and must not be symlinks')
        self.binary = root / 'runtime/lume.app/Contents/MacOS/lume'
        self.store = root / 'store'
        self.env = {'PATH': '/usr/bin:/bin:/usr/sbin:/sbin',
                    'XDG_CONFIG_HOME': str(root / 'config'),
                    'LUME_TELEMETRY_ENABLED': 'false'}
        self.profile = root / 'host-isolation.sb'
        require(not self.profile.is_symlink(), 'isolation profile must not be a symlink')
        home = Path.home()
        require(home != root and home in root.parents, 'expected dedicated storage below the current user')
        # Host VM and credential directories cannot be read, even by SSH fallback.
        self.profile.write_text(
            '(version 1)\n(allow default)\n'
            f'(deny file-read* file-write* (subpath {json.dumps(str(home))}))\n'
            f'(allow file-read* file-write* (subpath {json.dumps(str(root))}))\n'
            f'(allow file-read-metadata (literal {json.dumps(str(home))}))\n')

    def argv(self, *args):
        return ['/usr/bin/sandbox-exec', '-f', str(self.profile), str(self.binary), *args]

    def command(self, *args, timeout=60):
        result = subprocess.run(self.argv(*args), env=self.env, text=True,
                                capture_output=True, timeout=timeout)
        require(result.returncode == 0, redact(result.stderr + result.stdout))
        return result.stdout

    def foreground(self, *args):
        process = subprocess.Popen(self.argv(*args), env=self.env, text=True,
                                   stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        handle = self.root / f'active-{args[0]}.json'
        handle.write_text(json.dumps({'pid': process.pid, 'operation': args[0]}))
        try:
            for line in process.stdout:
                print(redact(line), end='', flush=True)
            code = process.wait()
            require(code == 0 or args[0] == 'serve' and code in {-signal.SIGTERM, -signal.SIGINT},
                    'Lume command failed; inspect sanitized log')
        finally:
            if process.poll() is None:
                if shutil.which('multica'):
                    daemon = json.loads(subprocess.check_output(
                        ['multica', 'daemon', 'status', '--output', 'json'], text=True))
                    require(process.pid != daemon['pid'], 'refusing to terminate the Multica daemon')
                process.terminate()
                try:
                    process.wait(timeout=30)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=15)
            handle.unlink(missing_ok=True)

    def request(self, port, method, path, body=None):
        require(1024 <= port <= 65535, 'invalid test port')
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        url = f'http://127.0.0.1:{port}/lume/'
        # Refuse an accidentally selected server with a different VM store.
        with opener.open(url + 'config', timeout=5) as response:
            config = json.load(response)
        require(isinstance(config, dict) and config.get('defaultLocationName') == 'erii24'
                and config.get('vmLocations') == [{'name': 'erii24', 'path': str(self.store)}],
                'server does not exclusively own the test store')
        payload = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(url + path, data=payload, method=method,
                                         headers={'Content-Type': 'application/json'})
        with opener.open(request, timeout=120) as response:
            return json.load(response)

    def doctor(self):
        signature = subprocess.run(['codesign', '--verify', '--deep', '--strict', str(self.binary.parent.parent.parent)],
                                    capture_output=True, text=True)
        require(signature.returncode == 0, 'Lume signature verification failed')
        version = self.command('--version').strip()
        require(version == LUME_VERSION, 'unexpected Lume version')
        config = self.command('config', 'get')
        require(str(self.store) in config and 'Telemetry enabled: false' in config,
                'configuration is not isolated')
        print(json.dumps({'lume_version': version, 'signature': 'verified',
                          'store': str(self.store), 'telemetry': False,
                          'host_personal_files': 'denied by Seatbelt',
                          'service_lifetime': 'foreground only'}, indent=2))

    def download(self):
        """Resume only hashed committed ranges; verify Apple's full-image digest."""
        image = self.root / 'downloads' / IPSW_NAME
        journal = self.root / 'downloads' / 'ipsw-ranges.json'
        require(not image.is_symlink() and not journal.is_symlink(), 'unsafe download destination')
        completed = json.loads(journal.read_text()) if journal.exists() else {}
        chunk = DOWNLOAD_RANGE_BYTES
        lock = threading.Lock()
        descriptor = os.open(image, os.O_RDWR | os.O_CREAT, 0o600)

        def digest_range(start, length):
            digest = hashlib.sha256()
            for position in range(start, start + length, 4 * 1024 * 1024):
                digest.update(os.pread(descriptor, min(4 * 1024 * 1024, start + length - position), position))
            return digest.hexdigest()

        def fetch(start):
            end = min(start + chunk, IPSW_SIZE) - 1
            key = str(start)
            if key in completed and digest_range(start, end - start + 1) == completed[key]:
                return
            part = self.root / 'downloads' / f'ipsw.range-{start}'
            header = part.with_suffix(part.suffix + '.headers')
            require(not part.is_symlink() and not header.is_symlink(), 'unsafe range destination')
            try:
                subprocess.run(['curl', '--disable', '--fail', '--silent', '--show-error', '--location',
                                '--retry', '2', '--connect-timeout', '20', '--max-time', '600',
                                '--range', f'{start}-{end}', '--dump-header', str(header),
                                '--output', str(part), IPSW_URL], check=True)
                require(part.stat().st_size == end - start + 1, 'incorrect image range size')
                require(f'content-range: bytes {start}-{end}/{IPSW_SIZE}' in header.read_text().lower(),
                        'server did not honor the exact image range')
                with part.open('rb') as source:
                    offset = start
                    while block := source.read(4 * 1024 * 1024):
                        view = memoryview(block)
                        while view:
                            written = os.pwrite(descriptor, view, offset)
                            require(written > 0, 'image write made no progress')
                            offset += written
                            view = view[written:]
                os.fsync(descriptor)
                with lock:
                    completed[key] = digest_range(start, end - start + 1)
                    temporary = journal.with_suffix('.tmp')
                    require(not temporary.is_symlink(), 'unsafe download journal destination')
                    temporary.write_text(json.dumps(completed))
                    temporary.replace(journal)
                print(f'image ranges committed: {len(completed)}/{(IPSW_SIZE + chunk - 1) // chunk}', flush=True)
            finally:
                part.unlink(missing_ok=True)
                header.unlink(missing_ok=True)

        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                list(pool.map(fetch, range(0, IPSW_SIZE, chunk)))
            os.ftruncate(descriptor, IPSW_SIZE)
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
        with image.open('rb') as source:
            actual = hashlib.file_digest(source, 'sha256').hexdigest()
        require(actual == IPSW_SHA256, 'Apple restore image SHA-256 mismatch')
        print(json.dumps({'image': IPSW_NAME, 'bytes': IPSW_SIZE, 'sha256': actual}), flush=True)

    def vnc(self, actions):
        # Import only for explicit live desktop operations, in the dedicated venv.
        from vncdotool import api
        details = json.loads(self.command('get', VM, '--storage', str(self.store), '--format', 'json'))
        detail = details[0] if isinstance(details, list) else details
        parsed = urllib.parse.urlparse(detail['vncUrl'])
        require(parsed.hostname in {'127.0.0.1', 'localhost'}, 'VNC must be loopback')
        client = api.connect(f'127.0.0.1::{parsed.port}', password=parsed.password, timeout=30)
        try:
            for action in actions:
                kind, *values = action
                if kind == 'capture':
                    target = self.root / 'evidence' / values[0]
                    require(target.parent == self.root / 'evidence' and not target.is_symlink(),
                            'capture must be a filename in test evidence')
                    client.captureScreen(str(target))
                elif kind == 'click':
                    client.mouseMove(int(values[0]), int(values[1]))
                    client.mousePress(1)
                elif kind == 'key':
                    client.keyPress(values[0])
                elif kind == 'type':
                    client.type(values[0])
                elif kind == 'pause':
                    client.pause(min(float(values[0]), 10))
                else:
                    raise RuntimeError('unsupported VNC action')
        finally:
            client.disconnect()
            api.shutdown()

    def put_fixture(self, filename):
        source = self.root / 'fixtures' / filename
        require(source.parent == self.root / 'fixtures' and not source.parent.is_symlink() and not source.is_symlink()
                and source.is_file() and source.stat().st_size <= 2 * 1024 * 1024,
                'put accepts only a small owned synthetic fixture filename')
        payload = source.read_bytes()
        target = shlex.quote('/Users/lume/erii24-fixtures/' + filename)
        temporary = shlex.quote('/Users/lume/erii24-fixtures/' + filename + '.b64')
        self.command('ssh', VM, '--storage', str(self.store),
                     f'mkdir -p /Users/lume/erii24-fixtures && : > {temporary}')
        encoded = base64.b64encode(payload).decode('ascii')
        for start in range(0, len(encoded), 32768):
            self.command('ssh', VM, '--storage', str(self.store),
                         f"printf '%s' '{encoded[start:start + 32768]}' >> {temporary}")
        result = self.command('ssh', VM, '--storage', str(self.store),
                              f'base64 -D -i {temporary} > {target} && rm {temporary} && shasum -a 256 {target}')
        digest = hashlib.sha256(payload).hexdigest()
        require(result.split()[0] == digest, 'Guest fixture hash mismatch')
        print(json.dumps({'fixture': filename, 'bytes': len(payload), 'sha256': digest,
                          'guest_readback': 'matched'}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True)
    parser.add_argument('--port', type=int, default=17774)
    parser.add_argument('operation', choices=['doctor', 'download', 'create', 'serve', 'start', 'state', 'guest', 'put', 'vnc', 'stop', 'poweroff'])
    parser.add_argument('arguments', nargs='*')
    args = parser.parse_args()
    live = Live(args.root)
    if args.operation == 'doctor':
        live.doctor()
    elif args.operation == 'download':
        live.download()
    elif args.operation == 'create':
        require(len(args.arguments) == 1, 'create requires a verified local IPSW')
        image = Path(args.arguments[0]).absolute()
        require(image.parent == live.root / 'downloads' and image.is_file() and not image.is_symlink(),
                'IPSW must be in owned downloads')
        with image.open('rb') as source:
            require(hashlib.file_digest(source, 'sha256').hexdigest() == IPSW_SHA256,
                    'IPSW digest must match the pinned Apple image')
        require(not (live.store / VM).exists(), 'refusing to overwrite a VM')
        live.foreground('create', VM, '--storage', str(live.store), '--cpu', '4', '--memory', '8GB',
                        '--disk-size', '64GB', '--display', '1280x800', '--ipsw', str(image), '--unattended', 'tahoe')
    elif args.operation == 'serve':
        live.foreground('serve', '--port', str(args.port))
    elif args.operation == 'guest':
        require(len(args.arguments) == 1, 'guest requires one explicit command')
        print(redact(live.command('ssh', VM, '--storage', str(live.store), args.arguments[0], timeout=600)), end='')
    elif args.operation == 'vnc':
        require(len(args.arguments) == 1, 'vnc requires JSON actions')
        live.vnc(json.loads(args.arguments[0]))
    elif args.operation == 'put':
        require(len(args.arguments) == 1, 'put requires one owned synthetic fixture filename')
        live.put_fixture(args.arguments[0])
    elif args.operation == 'stop':
        # Lume's HTTP /stop powers the VM off even with force=false. The
        # dedicated synthetic Guest's shutdown command requests OS shutdown;
        # never treat a provider process exit alone as graceful Guest shutdown.
        print(redact(live.command('shutdown', VM, '--storage', str(live.store), '--timeout', '30')))
        for _ in range(180):
            state = live.request(args.port, 'GET', f'vms/{VM}?' + urllib.parse.urlencode({'storage': str(live.store)}))
            if state.get('status') == 'stopped':
                print(json.dumps({'guest_shutdown_requested': True, 'provider_status': 'stopped'}))
                break
            time.sleep(.5)
        else:
            raise RuntimeError('Guest did not stop; no forced power-off or checkpoint attempted')
    else:
        path = f'vms/{VM}'
        body = {'storage': str(live.store)}
        method = 'GET'
        if args.operation == 'state':
            path += '?' + urllib.parse.urlencode(body)
            body = None
        else:
            method = 'POST'
            path += '/run' if args.operation == 'start' else '/stop'
            if args.operation == 'start':
                body.update(noDisplay=True, sharedDirectories=[], clipboard=False)
            else:
                body.update(force=True, timeout=0)
        value = live.request(args.port, method, path, body)
        # Never persist a VNC URL or raw provider authentication fields.
        if isinstance(value, dict):
            value.pop('vncUrl', None)
        print(redact(json.dumps(value, indent=2)))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(redact(str(error)), file=sys.stderr)
        raise SystemExit(1)
