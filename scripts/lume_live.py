#!/usr/bin/env python3
"""Opt-in L2 Lume acceptance mechanics; never part of deterministic CI.

Only an explicitly owned test root is accepted. Every Lume process is confined
from the host user's other files. `serve` stays in the foreground; no LaunchAgent
or persistent service is installed. VM credentials never enter report output.
"""
import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import urllib.parse
import urllib.request

ISSUE = '01a14029-4eab-7782-afba-3980c6c98a7c'
VM = 'erii24-base'
LUME_VERSION = '0.6.1'
MARKER = 'erii24-owned.json'


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
        self.binary = root / 'runtime/lume.app/Contents/MacOS/lume'
        self.store = root / 'store'
        self.env = {'PATH': '/usr/bin:/bin:/usr/sbin:/sbin',
                    'XDG_CONFIG_HOME': str(root / 'config'),
                    'LUME_TELEMETRY_ENABLED': 'false'}
        self.profile = root / 'host-isolation.sb'
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
        handle = self.root / 'active-process.json'
        handle.write_text(json.dumps({'pid': process.pid, 'operation': args[0]}))
        try:
            for line in process.stdout:
                print(redact(line), end='', flush=True)
            require(process.wait() == 0, 'Lume command failed; inspect sanitized log')
        finally:
            if process.poll() is None:
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
        require(str(self.store) in json.dumps(config), 'server does not own the test store')
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True)
    parser.add_argument('--port', type=int, default=17774)
    parser.add_argument('operation', choices=['doctor', 'create', 'serve', 'start', 'state', 'guest', 'vnc', 'stop'])
    parser.add_argument('arguments', nargs='*')
    args = parser.parse_args()
    live = Live(args.root)
    if args.operation == 'doctor':
        live.doctor()
    elif args.operation == 'create':
        require(len(args.arguments) == 1, 'create requires a verified local IPSW')
        image = Path(args.arguments[0]).absolute()
        require(image.parent == live.root / 'downloads' and image.is_file() and not image.is_symlink(),
                'IPSW must be in owned downloads')
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
                body.update(force=False, timeout=90)
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
