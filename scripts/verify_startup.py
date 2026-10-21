#!/usr/bin/env python3
"""Install a native release; drive bare erii through the maintained real PTY.

Launch/Doctor/Drive/Evidence/Cleanup includes actual Pi/QuickJS dependencies,
local JetStream, explicit model setup, Tool execution and SIGKILL/restart.
Only the model HTTP peer is deterministic. No online account or user state.
"""
import argparse
import hashlib
from http.server import ThreadingHTTPServer
import json
import os
from pathlib import Path
import signal
import tarfile
import tempfile
import threading

from terminal_driver import Terminal
from fixture_pool import run as owned_run, run_cases
from verify_local_provider import Endpoint
from verify_main import MainHarness, assert_final, require
from verify_tui import local_setup

ROOT = Path(__file__).resolve().parents[1]


class StartupHarness(MainHarness):
    def args(self, *args):
        return [str(self.binary), *args]


def verify_install():
    report = dict(version=1, checks=[], live_model='not_run')
    with tempfile.TemporaryDirectory(prefix='erii-start-') as tmp:
        root = Path(tmp)
        env = {k: v for k, v in os.environ.items() if not k.startswith('ERII_')}
        env['npm_config_offline'] = 'true'
        # CI's copied public SDK is immutable. Product builds use its original
        # Host SDK, as in ci-inputs; do not populate the fixture SDK's archives.
        base_sdk = os.environ.get('ERII_CI_BASE_GOROOT')
        if base_sdk:
            env['GOROOT'] = base_sdk
            env['PATH'] = str(Path(base_sdk) / 'bin') + os.pathsep + env['PATH']
        def run(*args):
            result = owned_run(args, cwd=ROOT, env=env, text=True,
                                    capture_output=True, timeout=120)
            require(result.returncode == 0, result.stdout + result.stderr)
            return result.stdout

        # Package through the same source installer used by contributors.
        package_log = run('sh', str(ROOT / 'scripts/package.sh'), str(root / 'dist'))
        archive, = (root / 'dist').glob('*.tar.gz')
        require(archive.with_suffix('.gz.sha256').read_text().split()[0] == hashlib.sha256(archive.read_bytes()).hexdigest(),
                'native package checksum does not match archive')
        with tarfile.open(archive) as bundle:
            bundle.extractall(root / 'unpacked', filter='data')
        installer = root / 'unpacked/erii/install.sh'
        prefix = root / 'prefix with spaces'
        install_log = run('sh', str(installer), '--prefix', str(prefix))
        binary = prefix / 'bin/erii'
        folder = root / 'workspace'
        folder.mkdir()
        h = StartupHarness(binary, folder)
        diagnosed = h.command('doctor', ok=False)
        report['doctor'] = diagnosed.stdout
        require(diagnosed.returncode == 0 and 'FAIL' not in report['doctor'], 'installed doctor failed')
        require(not (folder / 'data').exists(), 'doctor created application state')
        report['binary_sha256'] = hashlib.sha256(binary.read_bytes()).hexdigest()
        report['archive_sha256'] = hashlib.sha256(archive.read_bytes()).hexdigest()
        report['checks'].append('source install -> native archive -> release install -> read-only doctor; launch from unrelated cwd with spaces in install path')

        # Refuse an unrelated executable before changing any installation.
        protected = root / 'protected'
        (protected / 'bin').mkdir(parents=True)
        sentinel = protected / 'bin/erii'
        sentinel.write_text('user executable')
        rejected = owned_run(['sh', str(installer), '--prefix', str(protected)],
                                  cwd=folder, env=env, capture_output=True, text=True, timeout=10)
        require(rejected.returncode != 0 and sentinel.read_text() == 'user executable',
                'installer overwrote an unrelated executable')
        require(not (protected / 'lib').exists(), 'rejected install left partial state')

        server = ThreadingHTTPServer(('127.0.0.1', 0), Endpoint)
        server.daemon_threads = True
        server.requests, server.mode = [], 'success'
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            # Bare erii, no YAML, no preselected model, no external broker.
            with Terminal(h.args(), h.env, folder) as terminal:
                terminal.expect('State: setup_required')
                require(h.command('status')['readiness'] == 'setup_required', 'startup selected an implicit model')
                local_setup(terminal, f'http://127.0.0.1:{server.server_port}/v1')
                terminal.send('One-command startup verification')
                terminal.expect('assistant: PROBE_RESULT_OK')
                terminal.expect('You> ')
                before = h.state()
                assert_final(before, 1)
                require(len(before['tools']) == 1, 'startup bypassed the shared Tool executor')
                require(terminal.stop(signal.SIGKILL) == -signal.SIGKILL, 'missing crash evidence')
                report['first_launch'] = terminal.transcript()
            with Terminal(h.args(), h.env, folder) as terminal:
                terminal.expect('State: ready')
                terminal.expect('assistant: PROBE_RESULT_OK')
                terminal.expect('You> ')
                require(h.state() == before, 'restart duplicated or changed completed state')
                require(terminal.stop() == 0, 'foreground cleanup failed')
                report['restart'] = terminal.transcript()
            # Reinstallation changes only the runtime, retaining the data root.
            run('sh', str(installer), '--prefix', str(prefix))
            require(h.state() == before, 'reinstallation changed application data')
            report['state'] = before
            report['checks'].append('explicit local setup -> real Pi Probe/select -> Main -> Direct Tool -> durable reply; SIGKILL and bare-command restart preserve completed state; /quit joins runtime; reinstall preserves user data')
            report['checks'].append('unmanaged executable protected; no shell-profile or credential writes')
        finally:
            h.cleanup()
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)
            require(not thread.is_alive(), 'local model peer did not stop')
        report['package_log'] = package_log
        report['install_log'] = install_log
    require(not root.exists(), 'startup fixtures leaked')
    report['cleanup'] = 'all owned processes joined; temporary installation, broker, database and archive removed'
    return report


def verify():
    # Reuse the project process owner for failed installs, timeouts and PTYs too.
    # Retirement checks the entire owned session, including Node descendants.
    return run_cases([None], lambda _: verify_install(), jobs=1)[0]


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    # The suite supplies its binary argument; this gate builds and installs the
    # native package from the same checkout to exercise the real installer.
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--evidence', type=Path, default=ROOT / 'evidence/startup.json')
    args = parser.parse_args()
    result = verify()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(result, indent=2) + '\n')
    print('Startup verification passed:', args.evidence)
