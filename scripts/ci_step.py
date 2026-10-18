#!/usr/bin/env python3
"""Record one CI phase, including failure and graceful cancellation evidence."""
import argparse
import json
import os
from pathlib import Path
import platform
import signal
import threading
import time

from verify_suite import run_command, source_state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--name', required=True)
    parser.add_argument('--phase', choices=['setup', 'build', 'test'], required=True)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command or not args.name.replace('-', '').isalnum():
        parser.error('command and a simple step name required')
    folder = Path('evidence/steps')
    folder.mkdir(parents=True, exist_ok=True)
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopped.set())
    metrics = {}
    source = source_state()
    started = time.monotonic()
    code = 127
    try:
        code = run_command(command, os.environ.copy(), folder / (args.name + '.log'), stopped, metrics)
    finally:
        stable = source == source_state()
        if not stable:
            code = 125
        record = dict(version=1, name=args.name, phase=args.phase, command=command, returncode=code,
                      **source, source_stable=stable,
                      wall_seconds=time.monotonic() - started, runner=platform.platform(),
                      image_os=os.environ.get('ImageOS'), image_version=os.environ.get('ImageVersion'),
                      go_cache_hit=os.environ.get('CI_GO_CACHE_HIT') or None,
                      npm_cache_hit=os.environ.get('CI_NPM_CACHE_HIT') or None, **metrics)
        (folder / (args.name + '.json')).write_text(json.dumps(record, indent=2) + '\n')
        print(json.dumps(record), flush=True)
        if code:
            print((folder / (args.name + '.log')).read_text()[-12000:], flush=True)
    return code


if __name__ == '__main__':
    raise SystemExit(main())
