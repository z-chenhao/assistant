#!/usr/bin/env python3
"""Locked development installs using native npm/Playwright download caches.

No node_modules reuse, hosted cache transport, or application runtime policy.
"""
import argparse
from contextlib import contextmanager
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import signal
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
PACKAGES = ('provider-bridge', 'runtime/javascript', 'guest-browser', 'web')


def run(command, root, env, timeout, capture=False):
    """Bound the whole stage, including native install locks and descendants."""
    with subprocess.Popen(command, cwd=root, env=env, start_new_session=True,
                          stdout=subprocess.PIPE if capture else None, text=True) as child:
        try:
            output, _ = child.communicate(timeout=timeout)
            if child.returncode:
                raise RuntimeError(f'{command[0]} stage failed (exit {child.returncode})')
            return output.strip() if capture else None
        finally:
            # A downloader/lifecycle descendant must not outlive a failed stage.
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass


@contextmanager
def worktree_lock(root, timeout):
    path = root / 'evidence/setup.lock'
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('a') as lock:
        deadline = time.monotonic() + timeout
        while True:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                if time.monotonic() >= deadline:
                    raise RuntimeError('another setup owns this worktree; lock wait timed out')
                time.sleep(.05)
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def identity(root, env, timeout):
    node = json.loads(run(['node', '-p', 'JSON.stringify({version:process.version,platform:process.platform,arch:process.arch})'],
                          root, env, timeout, capture=True))
    expected = 'v' + (root / '.node-version').read_text().strip()
    if node['version'] != expected:
        raise RuntimeError(f'Node {expected} required; found {node["version"]}; no installation attempted')
    return dict(node=node, npm=run(['npm', '--version'], root, env, timeout, capture=True),
                lock_sha256={name: hashlib.sha256((root / name / 'package-lock.json').read_bytes()).hexdigest()
                             for name in PACKAGES})


def setup(root, args, report, env):
    env['npm_config_update_notifier'] = 'false'
    report['identity'] = identity(root, env, args.timeout)
    report['npm_cache'] = run(['npm', 'config', 'get', 'cache'], root, env, args.timeout, capture=True)
    # Native revision directories and install locks handle cross-worktree reuse.
    # Never GC a browser another checkout may still be using.
    env['PLAYWRIGHT_SKIP_BROWSER_GC'] = '1'
    env['PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT'] = '30000'

    def step(name, command, capture=False):
        item = dict(name=name, outcome='failed')
        report['steps'].append(item)
        started = time.monotonic()
        print(f'setup: {name} ({args.mode})', flush=True)
        try:
            output = run(command, root, env, args.timeout, capture=capture)
            item['outcome'] = 'passed'
            return output
        finally:
            item['seconds'] = round(time.monotonic() - started, 3)

    if args.part != 'browser':
        for name in getattr(args, 'packages', None) or PACKAGES:
            step(name, ['npm', 'ci', '--prefix', name, '--ignore-scripts', '--prefer-offline',
                        '--offline=' + str(args.mode == 'offline').lower(),
                        '--fetch-retries=0', '--fetch-timeout=30000'])
    if args.part != 'packages':
        cli = str(root / 'guest-browser/node_modules/playwright-core/cli.js')
        # Also check the lock when CI requests only the browser half of setup.
        lock = json.loads((root / 'guest-browser/package-lock.json').read_text())
        installed = json.loads((root / 'guest-browser/node_modules/playwright-core/package.json').read_text())
        if installed['version'] != lock['packages']['node_modules/playwright-core']['version']:
            raise RuntimeError('Playwright differs from lockfile; run the packages setup first')
        if args.mode == 'online':
            step('chromium-install', ['node', cli, 'install', *(['--with-deps'] if args.with_deps else []), 'chromium'])
        # Offline never calls install (Playwright has no offline install flag).
        report['browsers'] = json.loads(step('chromium-doctor', ['node', str(root / 'scripts/setup_browser.cjs')], capture=True))


def network_summary(path):
    rows = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
    return dict(requests=len(rows), response_body_bytes=sum(row['response_body_bytes'] for row in rows),
                by_kind={kind: dict(requests=sum(row['kind'] == kind for row in rows),
                                    objects=len({row['object_sha256'] for row in rows if row['kind'] == kind
                                                 and row['status'] is not None and 200 <= row['status'] < 300
                                                 and row['response_body_bytes'] > 0}),
                                    response_body_bytes=sum(row['response_body_bytes'] for row in rows if row['kind'] == kind))
                         for kind in sorted({row['kind'] for row in rows})},
                scope='Node http(s).request/get attempts and received response-body bytes only; '
                      'objects are distinct successful nonempty URL paths (query excluded); '
                      'excludes fetch/undici, TLS/IP, OS package managers, browser subprocess traffic and bytes sent')


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mode', choices=('online', 'offline'), default='online')
    parser.add_argument('--part', choices=('all', 'packages', 'browser'), default='all')
    parser.add_argument('--packages', nargs='+', choices=PACKAGES, help='Install only these packages (CI consumers); default: all four')
    parser.add_argument('--with-deps', action='store_true', help='Online only: install Ubuntu browser system libraries')
    parser.add_argument('--timeout', type=float, default=180, help='Maximum seconds per stage, including native lock waits')
    parser.add_argument('--evidence', type=Path, default=ROOT / 'evidence/setup.json')
    parser.add_argument('--measure-network', action='store_true', help='Record Node HTTP response bodies; see evidence scope')
    args = parser.parse_args(argv)
    if not math.isfinite(args.timeout) or args.timeout <= 0 or args.timeout > 600:
        parser.error('--timeout must be > 0 and <= 600')
    if args.packages and args.part != 'packages':
        parser.error('--packages requires --part packages')
    if args.with_deps and (args.mode == 'offline' or args.part == 'packages'):
        parser.error('--with-deps requires online browser setup')
    report = dict(version=1, mode=args.mode, part=args.part, outcome='failed', steps=[])
    env = dict(os.environ)
    started = time.monotonic()
    try:
        with worktree_lock(ROOT, args.timeout):
            args.evidence = args.evidence.resolve()
            args.evidence.parent.mkdir(parents=True, exist_ok=True)
            network = args.evidence.with_suffix('.network.jsonl')
            if args.measure_network:
                network.write_text('')
                env['ERII_SETUP_NETWORK_LOG'] = str(network)
                env['NODE_OPTIONS'] = (env.get('NODE_OPTIONS', '') + ' --require=' +
                                       json.dumps(str(ROOT / 'scripts/setup_network.cjs'))).strip()
            try:
                report['source_sha'] = run(['git', 'rev-parse', 'HEAD'], ROOT, env, args.timeout, capture=True)
                setup(ROOT, args, report, env)
                report['outcome'] = 'passed'
            except (OSError, RuntimeError, ValueError, subprocess.TimeoutExpired) as error:
                report['error'] = str(error)
                print(f'setup: failed: {error}. See CONTRIBUTING.md for cache recovery.', flush=True)
            finally:
                report['seconds'] = round(time.monotonic() - started, 3)
                if args.measure_network:
                    report['network'] = network_summary(network)
                args.evidence.write_text(json.dumps(report, indent=2) + '\n')
                print(f'setup: {report["outcome"]}; evidence: {args.evidence}', flush=True)
    except (OSError, RuntimeError) as error:
        # Do not overwrite a report while another setup still owns this tree.
        report['outcome'] = 'failed'
        print(f'setup: failed: {error}', flush=True)
    return 0 if report['outcome'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())
