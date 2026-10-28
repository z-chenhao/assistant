#!/usr/bin/env python3
"""Let test fixtures unwind finally blocks on the first termination signal."""
import json
import os
from pathlib import Path
import resource
import runpy
import signal
import sys
import time


def main():
    script = Path(sys.argv[1]).resolve()
    sys.argv = sys.argv[1:]
    sys.path.insert(0, str(script.parent))
    terminating = False

    def terminate(signum, *_):
        nonlocal terminating
        if not terminating:
            terminating = True
            raise SystemExit(128 + signum)

    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, terminate)
    started = time.monotonic()
    try:
        runpy.run_path(str(script), run_name='__main__')
    finally:
        if os.environ.get('ERII_TEST_METRICS_DIR'):
            folder = Path(os.environ['ERII_TEST_METRICS_DIR'])
            folder.mkdir(parents=True, exist_ok=True)
            usage = resource.getrusage(resource.RUSAGE_SELF)
            (folder / (script.stem + '.json')).write_text(json.dumps(dict(
                elapsed_seconds=time.monotonic()-started,
                cpu_seconds=usage.ru_utime+usage.ru_stime)) + '\n')


if __name__ == '__main__':
    main()
