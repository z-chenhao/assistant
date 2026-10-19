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
import fcntl
import json
import math
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import socket
import subprocess
import sys
import struct
import wave
import threading
import time
import urllib.parse
import urllib.error
import urllib.request

ISSUE = '01a14029-4eab-7782-afba-3980c6c98a7c'
VM = 'erii24-base'
LUME_VERSION = '0.6.1'
TESTED_IMAGE = 'macos-tahoe-cua:26.5.2'
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


def decode_details(text):
    # Lume may prefix a successful JSON response with stale-session cleanup
    # logs after a crash. Accept only the complete final descriptor for our VM.
    lines = text.splitlines(keepends=True)
    for index, line in enumerate(lines):
        if not line.lstrip().startswith(('[', '{')):
            continue
        try:
            value = json.loads(''.join(lines[index:]))
        except json.JSONDecodeError:
            continue
        if isinstance(value, list) and len(value) == 1:
            value = value[0]
        if isinstance(value, dict) and value.get('name') == VM and isinstance(value.get('status'), str):
            return value
    raise RuntimeError('Lume did not return the owned VM descriptor')


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
        for name in ['runtime', 'store', 'config', 'downloads', 'logs', 'evidence', 'cache']:
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

    def guest(self, command, timeout=60):
        return self.command('ssh', '--storage', str(self.store), '--timeout', str(timeout),
                            VM, command, timeout=timeout + 15)

    def details(self):
        return decode_details(self.command('get', VM, '--storage', str(self.store), '--format', 'json'))

    def foreground(self, *args):
        handle = self.root / f'active-{args[0]}.json'
        receipt = self.root / f'completed-{args[0]}.json'
        require(not handle.exists() and not handle.is_symlink(), 'operation already has an owned process handle')
        require(not receipt.is_symlink(), 'operation receipt must not be a symlink')
        process = subprocess.Popen(self.argv(*args), env=self.env, text=True,
                                   stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        handle.write_text(json.dumps({'pid': process.pid, 'operation': args[0]}))
        try:
            for line in process.stdout:
                print(redact(line), end='', flush=True)
            code = process.wait()
            receipt.write_text(json.dumps({'operation': args[0], 'return_code': code}) + '\n')
            require(code == 0 or args[0] == 'serve' and code in {-signal.SIGTERM, -signal.SIGINT},
                    f'Lume {args[0]} exited with code {code}; inspect sanitized log')
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

    def request(self, port, method, path, body=None, timeout=120):
        require(1024 <= port <= 65535, 'invalid test port')
        deadline = time.monotonic() + timeout
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        url = f'http://127.0.0.1:{port}/lume/'
        # Refuse an accidentally selected server with a different VM store.
        with opener.open(url + 'config', timeout=min(5, timeout)) as response:
            config = json.load(response)
        require(isinstance(config, dict) and config.get('defaultLocationName') == 'erii24'
                and config.get('vmLocations') == [{'name': 'erii24', 'path': str(self.store)}],
                'server does not exclusively own the test store')
        payload = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(url + path, data=payload, method=method,
                                         headers={'Content-Type': 'application/json'})
        remaining = deadline - time.monotonic()
        require(remaining > 0, 'Lume request deadline elapsed before dispatch')
        with opener.open(request, timeout=remaining) as response:
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

    def build_media(self):
        source = Path(__file__).with_name('lume_media.swift')
        target = self.root / 'runtime/media-fixture.swift'
        binary = self.root / 'runtime/media-fixture'
        require(not target.is_symlink() and not binary.is_symlink(), 'media helper must not be a symlink')
        target.write_bytes(source.read_bytes())
        cache = self.root / 'cache/swift'
        require(not cache.is_symlink(), 'media compiler cache must not be a symlink')
        cache.mkdir(parents=True, exist_ok=True)
        result = subprocess.run(['/usr/bin/sandbox-exec', '-f', str(self.profile),
                                 '/usr/bin/swiftc', '-parse-as-library', '-module-cache-path', str(cache),
                                 str(target), '-o', str(binary)], env=self.env,
                                capture_output=True, text=True, timeout=120)
        require(result.returncode == 0, redact(result.stderr))
        return binary

    def fixtures(self):
        directory = self.root / 'fixtures'
        require(directory.is_dir() and not directory.is_symlink(), 'owned fixtures directory must exist')
        names = ['red.mov', 'blue.mov', 'tone.wav', 'subtitles.srt']
        require(not any((directory / name).exists() or (directory / name).is_symlink() for name in names),
                'refusing to overwrite existing media fixtures')
        require(not (self.root / 'evidence/fixtures.json').is_symlink(), 'fixture report must not be a symlink')
        binary = self.build_media()
        subprocess.run(['/usr/bin/sandbox-exec', '-f', str(self.profile), str(binary), 'make', str(directory)],
                       env=self.env, check=True, timeout=60)
        samples = b''.join(struct.pack('<h', int(6000 * math.sin(2 * math.pi * 440 * i / 24000)))
                           for i in range(6 * 24000))
        with wave.open(str(directory / 'tone.wav'), 'wb') as output:
            output.setparams((1, 2, 24000, 0, 'NONE', 'not compressed'))
            output.writeframes(samples)
        (directory / 'subtitles.srt').write_text(
            '1\n00:00:00,200 --> 00:00:02,800\nERII-24 alpha\n\n'
            '2\n00:00:03,200 --> 00:00:05,800\nERII-24 beta\n')
        report = {name: {'bytes': (directory / name).stat().st_size,
                         'sha256': hashlib.sha256((directory / name).read_bytes()).hexdigest()} for name in names}
        (self.root / 'evidence/fixtures.json').write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps(report))

    def media(self, operation, filename):
        require(operation in {'inspect', 'verify-export', 'audio'}, 'unsupported media operation')
        source = self.root / 'evidence' / filename
        if not source.exists():
            source = self.root / 'fixtures' / filename
        require(source.parent in {self.root / 'evidence', self.root / 'fixtures'} and not source.parent.is_symlink() and not source.is_symlink()
                and source.is_file() and source.stat().st_size <= 8 * 1024 * 1024, 'media must be owned small evidence')
        binary = self.build_media()
        subprocess.run(['/usr/bin/sandbox-exec', '-f', str(self.profile), str(binary), operation, str(source)],
                       env=self.env, check=True, timeout=60)

    def crash(self):
        require(self.request(self.port, 'GET', f'vms/{VM}?' +
                             urllib.parse.urlencode({'storage': str(self.store)})).get('status') == 'running',
                'fault injection requires the running owned VM')
        handle = self.root / 'active-serve.json'
        require(handle.is_file() and not handle.is_symlink(), 'no owned foreground server handle')
        target = json.loads(handle.read_text())
        pid = target['pid']
        require(target.get('operation') == 'serve' and isinstance(pid, int) and pid > 1 and pid != os.getpid(),
                'invalid owned server handle')
        daemon = json.loads(subprocess.check_output(['multica', 'daemon', 'status', '--output', 'json'], text=True))
        require(pid != daemon['pid'], 'refusing to terminate the Multica daemon')
        command = subprocess.check_output(['ps', '-p', str(pid), '-o', 'command='], text=True).strip()
        require(command == f'{self.binary} serve --port {self.port}', 'server PID no longer matches the owned process')
        receipt_path = self.root / 'evidence/crash.json'
        require(not receipt_path.is_symlink(), 'fault receipt must not be a symlink')
        os.kill(pid, signal.SIGKILL)
        receipt = {'pid': pid, 'signal': 'SIGKILL', 'outcome': 'signal_dispatched',
                   'mode': 'abnormal_fault', 'clean_checkpoint': False}
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
        print(json.dumps(receipt))

    def wait_ready(self, timeout=180):
        deadline = time.monotonic() + timeout
        path = f'vms/{VM}?' + urllib.parse.urlencode({'storage': str(self.store)})
        while time.monotonic() < deadline:
            state = self.request(self.port, 'GET', path, timeout=max(.1, deadline - time.monotonic()))
            if state.get('status') == 'running' and state.get('sshAvailable') is True:
                print(json.dumps({'vm': VM, 'status': 'running', 'ssh_available': True}))
                return
            time.sleep(.5)
        raise RuntimeError('owned Guest did not become ready before deadline')

    def stopped_resources(self, vnc_port):
        directory = self.store / VM
        require(not directory.is_symlink(), 'owned VM directory must not be a symlink')
        config = directory / 'config.json'
        require(config.is_file() and not config.is_symlink(), 'owned VM configuration must be regular')
        with config.open('rb') as handle:
            try:
                fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                return False
            try:
                if (directory / 'sessions.json').exists():
                    return False
                probe = subprocess.run(['/usr/sbin/lsof', '-n', '-P', f'-iTCP:{vnc_port}',
                                        '-sTCP:LISTEN', '-t'], capture_output=True, timeout=5)
                require(probe.returncode in {0, 1} and not probe.stderr.strip(), 'VNC resource inspection failed')
                return probe.returncode == 1 and not probe.stdout.strip()
            finally:
                fcntl.flock(handle, fcntl.LOCK_UN)

    def stop(self, timeout=90):
        receipt_path = self.root / 'evidence/stop.json'
        require(not receipt_path.is_symlink(), 'stop receipt must not be a symlink')
        detail = self.details()
        require(detail.get('status') == 'running', 'stop requires an observed running owned VM')
        parsed = urllib.parse.urlparse(detail['vncUrl'])
        require(parsed.hostname in {'127.0.0.1', 'localhost'} and parsed.port,
                'stop requires the owned loopback VNC resource')
        print(redact(self.command('shutdown', VM, '--storage', str(self.store), '--timeout', '30')), flush=True)
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.stopped_resources(parsed.port):
                break
            time.sleep(.5)
        else:
            raise RuntimeError('Guest resources were not released; no power-off or checkpoint attempted')
        # The native run lock is released only after VM.run's teardown. Do not
        # call /stop before this proof: force=false is still framework power-off.
        # Lume 0.6.1 leaves SharedVM cached after the Guest halts. Its explicit
        # stop clears that cache even when the already-stopped VM returns 400.
        state_path = f'vms/{VM}?' + urllib.parse.urlencode({'storage': str(self.store)})
        state = self.request(self.port, 'GET', state_path, timeout=10)
        reconciliation = 'not_needed'
        if state.get('status') != 'stopped':
            try:
                self.request(self.port, 'POST', f'vms/{VM}/stop',
                             {'storage': str(self.store), 'force': False, 'timeout': 30}, timeout=15)
                reconciliation = 'stop_acknowledged_after_resource_release'
            except urllib.error.HTTPError as error:
                message = json.loads(error.read(4096))
                require(error.code == 400 and message == {'message': f'Virtual machine not running: {VM}'},
                        'unexpected Lume reconciliation failure')
                reconciliation = 'already_not_running_cache_cleared'
            state = self.request(self.port, 'GET', state_path, timeout=10)
        require(state.get('status') == 'stopped', 'Lume stopped state was not confirmed')
        receipt = {'guest_shutdown_requested': True, 'config_lock_released': True,
                   'session_removed': True, 'vnc_listener_released': True,
                   'provider_status': 'stopped', 'reconciliation': reconciliation,
                   'poweroff_before_resource_release': False}
        receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
        print(json.dumps(receipt))

    def vnc_session(self, stream=None, control=False):
        # Import only for explicit live desktop operations, in the dedicated venv.
        from vncdotool import api
        from vncdotool.client import VNCDoToolClient, VNCDoToolFactory

        class ResizingClient(VNCDoToolClient):
            def updateDesktopSize(self, width, height):
                super().updateDesktopSize(width, height)
                # vncdotool 1.4.2 resizes the image but retains ServerInit's
                # fallback dimensions for future framebuffer requests.
                self.width, self.height = width, height

        class Factory(VNCDoToolFactory):
            protocol = ResizingClient

        detail = self.details()
        parsed = urllib.parse.urlparse(detail['vncUrl'])
        require(parsed.hostname in {'127.0.0.1', 'localhost'}, 'VNC must be loopback')
        client = api.connect(f'127.0.0.1::{parsed.port}', password=parsed.password,
                             factory_class=Factory, timeout=30)
        try:
            client.pause(.2)
            print('Guest VNC transport connected; send one JSON action array per line, or quit.', flush=True)
            for line, reply in self.vnc_inputs(stream, control):
                if line.strip() == 'quit':
                    if reply is not None:
                        reply.write(b'{"closed":true}\n')
                        reply.flush()
                    break
                actions = json.loads(line)
                require(isinstance(actions, list), 'expected a VNC action array')
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
                        for character in values[0]:
                            key = 'enter' if character == '\n' else character
                            if character.isupper() or character in '~!@#$%^&*()_+{}|:"<>?':
                                key = 'shift-' + character
                            client.keyPress(key)
                    elif kind == 'pause':
                        client.pause(min(float(values[0]), 10))
                    else:
                        raise RuntimeError('unsupported VNC action')
                receipt = json.dumps({'completed_vnc_actions': len(actions)})
                print(receipt, flush=True)
                if reply is not None:
                    reply.write((receipt + '\n').encode())
                    reply.flush()
        finally:
            try:
                client.disconnect()
            finally:
                api.shutdown()

    def vnc_inputs(self, stream, control):
        if not control:
            for line in stream if stream is not None else sys.stdin:
                yield line, None
            return
        endpoint = self.root / 'runtime/vnc.sock'
        require(not endpoint.exists() and not endpoint.is_symlink(), 'VNC control endpoint already exists')
        with socket.socket(socket.AF_UNIX) as server:
            server.bind(str(endpoint))
            endpoint.chmod(0o600)
            server.listen(1)
            try:
                while True:
                    connection, _ = server.accept()
                    with connection, connection.makefile('rwb') as channel:
                        line = channel.readline(65537)
                        require(len(line) <= 65536, 'VNC action batch is too large')
                        yield line.decode(), channel
            finally:
                endpoint.unlink(missing_ok=True)

    def vnc_control(self, batch):
        endpoint = self.root / 'runtime/vnc.sock'
        require(not endpoint.is_symlink(), 'VNC control endpoint must not be a symlink')
        require(len(batch.encode()) < 65536 and '\n' not in batch, 'invalid VNC action batch')
        with socket.socket(socket.AF_UNIX) as client:
            client.settimeout(90)
            client.connect(str(endpoint))
            client.sendall((batch + '\n').encode())
            with client.makefile('rb') as response:
                receipt = response.readline(65536)
            require(receipt, 'VNC batch ended without acknowledgement')
            print(receipt.decode(), end='')

    def fixture_bytes(self, filename):
        source = self.root / 'fixtures' / filename
        require(source.parent == self.root / 'fixtures' and not source.parent.is_symlink() and not source.is_symlink()
                and source.is_file() and source.stat().st_size <= 2 * 1024 * 1024,
                'put accepts only a small owned synthetic fixture filename')
        return source.read_bytes()

    def driver(self, tool, payload):
        require(re.fullmatch('[a-z_]+', tool), 'invalid Guest Driver tool name')
        require(isinstance(json.loads(payload), dict), 'Guest Driver parameters must be an object')
        if tool in {'get_desktop_state', 'get_window_state'}:
            # A refused/failed new observation must never leave a stale latest
            # screenshot looking like that observation. Archives stay intact.
            for suffix in ['.png', '.json']:
                latest = self.root / 'evidence' / (tool + suffix)
                require(not latest.is_symlink(), 'Driver evidence must not be a symlink')
                latest.unlink(missing_ok=True)
        result = json.loads(self.guest('/Applications/CuaDriver.app/Contents/MacOS/cua-driver call '
                                      + shlex.quote(tool) + ' ' + shlex.quote(payload)))
        if isinstance(result, dict) and 'screenshot_png_b64' in result:
            data = base64.b64decode(result.pop('screenshot_png_b64'), validate=True)
            require(len(data) <= 8 * 1024 * 1024, 'Driver screenshot exceeds evidence limit')
            target = self.root / 'evidence' / (tool + '.png')
            require(not target.is_symlink(), 'Driver evidence must not be a symlink')
            target.write_bytes(data)
            result['screenshot_file'] = target.name
            result['screenshot_sha256'] = hashlib.sha256(data).hexdigest()
            archive = self.root / 'evidence' / ('capture-' + result['screenshot_sha256'][:20] + '.png')
            require(not archive.is_symlink(), 'capture archive must not be a symlink')
            archive.write_bytes(data)
        if tool == 'get_desktop_state' and isinstance(result, dict):
            result['harness_session'] = json.loads(payload).get('session')
        target = self.root / 'evidence' / (tool + '.json')
        require(not target.is_symlink(), 'Driver evidence must not be a symlink')
        target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
        print(redact(json.dumps(result, ensure_ascii=False)))

    def driver_click(self, x, y, count=1):
        require(count in {1, 2}, 'click count must be one or two')
        state = json.loads((self.root / 'evidence/get_desktop_state.json').read_text())
        session = state.get('harness_session')
        require(isinstance(session, str) and session, 'capture must use an explicit Guest Driver session')
        require(0 <= x < state['screenshot_width'] and 0 <= y < state['screenshot_height'],
                'click must be inside the observed Driver screenshot')
        self.driver('click', json.dumps({'target': {'kind': 'desktop', 'display_id': 'primary'},
                                        'x': x, 'y': y, 'count': int(count), 'capture_id': state['capture_id'],
                                        'session': session, 'delivery_mode': 'foreground'}))
        self.driver('get_desktop_state', json.dumps({'session': session}))

    def get_fixture(self, filename):
        target = self.root / 'evidence' / filename
        require(target.parent == self.root / 'evidence' and not target.is_symlink(),
                'get requires a filename in owned evidence')
        source = shlex.quote('/Users/lume/erii24-fixtures/' + filename)
        size = int(self.guest(f'stat -f %z {source}').strip())
        require(0 <= size <= 8 * 1024 * 1024, 'Guest evidence exceeds the transfer limit')
        payload = base64.b64decode(''.join(self.guest(f'base64 -i {source}').split()), validate=True)
        require(len(payload) == size, 'Guest evidence size mismatch')
        digest = hashlib.sha256(payload).hexdigest()
        require(self.guest(f'shasum -a 256 {source}').split()[0] == digest, 'Guest evidence hash mismatch')
        target.write_bytes(payload)
        print(json.dumps({'evidence': filename, 'bytes': size, 'sha256': digest, 'guest_readback': 'matched'}))

    def put_fixture(self, filename):
        payload = self.fixture_bytes(filename)
        target = shlex.quote('/Users/lume/erii24-fixtures/' + filename)
        temporary = shlex.quote('/Users/lume/erii24-fixtures/' + filename + '.b64')
        self.guest(f'mkdir -p /Users/lume/erii24-fixtures && : > {temporary}')
        encoded = base64.b64encode(payload).decode('ascii')
        for start in range(0, len(encoded), 32768):
            self.guest(f"printf '%s' '{encoded[start:start + 32768]}' >> {temporary}")
        result = self.guest(f'base64 -D -i {temporary} > {target} && rm {temporary} && shasum -a 256 {target}')
        digest = hashlib.sha256(payload).hexdigest()
        require(result.split()[0] == digest, 'Guest fixture hash mismatch')
        print(json.dumps({'fixture': filename, 'bytes': len(payload), 'sha256': digest,
                          'guest_readback': 'matched'}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True)
    parser.add_argument('--port', type=int, default=17774)
    parser.add_argument('operation', choices=['doctor', 'fixtures', 'media', 'download', 'create', 'pull-tested-image', 'serve', 'start', 'wait-ready', 'crash', 'state', 'guest', 'guest-script', 'put', 'get', 'driver', 'driver-click', 'vnc-session', 'vnc-control', 'stop', 'poweroff'])
    parser.add_argument('arguments', nargs='*')
    args = parser.parse_args()
    live = Live(args.root)
    live.port = args.port
    if args.operation == 'doctor':
        live.doctor()
    elif args.operation == 'fixtures':
        live.fixtures()
    elif args.operation == 'media':
        require(len(args.arguments) == 2, 'media requires operation and owned filename')
        live.media(*args.arguments)
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
    elif args.operation == 'pull-tested-image':
        require(not (live.store / VM).exists(), 'refusing to overwrite a VM')
        live.foreground('pull', TESTED_IMAGE, VM, '--storage', str(live.store))
    elif args.operation == 'serve':
        live.foreground('serve', '--port', str(args.port))
    elif args.operation == 'crash':
        live.crash()
    elif args.operation == 'wait-ready':
        live.wait_ready()
    elif args.operation == 'guest':
        require(len(args.arguments) == 1, 'guest requires one explicit command')
        print(redact(live.guest(args.arguments[0])), end='')
    elif args.operation == 'guest-script':
        require(len(args.arguments) == 1, 'guest-script requires an owned fixture script filename')
        command = live.fixture_bytes(args.arguments[0]).decode('utf-8')
        print(redact(live.guest(command, timeout=540)), end='')
    elif args.operation == 'vnc-session':
        require(not args.arguments or args.arguments == ['control'], 'vnc-session accepts only control')
        live.vnc_session(control=bool(args.arguments))
    elif args.operation == 'vnc-control':
        require(len(args.arguments) == 1, 'vnc-control requires one JSON action batch or quit')
        live.vnc_control(args.arguments[0])
    elif args.operation == 'driver-click':
        require(len(args.arguments) in {2, 3}, 'driver-click requires observed screenshot x and y, optional click count')
        live.driver_click(*map(float, args.arguments))
    elif args.operation == 'driver':
        require(len(args.arguments) == 2, 'driver requires a tool name and JSON object')
        live.driver(*args.arguments)
    elif args.operation == 'get':
        require(len(args.arguments) == 1, 'get requires one owned Guest evidence filename')
        live.get_fixture(args.arguments[0])
    elif args.operation == 'put':
        require(len(args.arguments) == 1, 'put requires one owned synthetic fixture filename')
        live.put_fixture(args.arguments[0])
    elif args.operation == 'stop':
        live.stop()
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
