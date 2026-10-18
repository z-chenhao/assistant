#!/usr/bin/env python3
"""Run the sensitive Bridge race contracts before the remaining full inventory.

Package isolation does not change concurrency inside a test or its deadlines.
The remainder retains Go's normal build/package parallelism.
"""
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import time

from verify_suite import run_command, source_state

BRIDGE = 'github.com/z-chenhao/erii/internal/llm/bridge'


def groups(packages):
    if (not packages or len(set(packages)) != len(packages) or BRIDGE not in packages or
            any(not isinstance(p, str) or not p.startswith('github.com/z-chenhao/erii/') or
                any(c.isspace() for c in p) for p in packages)):
        raise ValueError('invalid or incomplete Go package inventory')
    return [('bridge', [BRIDGE]), ('remainder', [p for p in packages if p != BRIDGE])]


def main():
    source = source_state()
    packages = subprocess.check_output(['go', 'list', './...'], text=True).splitlines()
    plan = groups(packages)
    folder = Path('evidence/race')
    folder.mkdir(parents=True, exist_ok=True)
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopped.set())
    rows = []
    passed = True
    try:
        for name, selected in plan:
            if not selected:
                continue
            command = ['go', 'test', '-race', '-count=1', '-timeout=15m', *selected]
            metrics = {}
            started = time.monotonic()
            code = run_command(command, os.environ.copy(), folder / (name + '.log'), stopped, metrics)
            rows.append(dict(name=name, packages=selected, command=command, returncode=code,
                             wall_seconds=time.monotonic() - started, **metrics))
            if code:
                print((folder / (name + '.log')).read_text()[-12000:], flush=True)
                passed = False
                break
    finally:
        stable = source == source_state()
        executed = [p for row in rows if row['returncode'] == 0 for p in row['packages']]
        passed = passed and stable and not stopped.is_set() and sorted(executed) == sorted(packages)
        report = dict(version=1, **source, source_stable=stable, packages=packages,
                      stages=rows, passed=passed)
        (folder / 'inventory.json').write_text(json.dumps(report, indent=2) + '\n')
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
