#!/usr/bin/env python3
"""Sample the maintained batch/fairness/restart/cancel harness without changing limits."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import statistics
import time

from verify_scheduling import run


def measure(binary, samples):
    values = []
    peaks = []
    for _ in range(samples):
        started = time.monotonic()
        result = run(binary)
        values.append(time.monotonic() - started)
        peaks.append(result['batch']['observed']['peak_children_per_tree'])
    return dict(version=1, binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                platform=platform.platform(), machine=platform.machine(),
                workload='verify_scheduling: 100 children + fairness + cancel + restart',
                seconds=values, median_seconds=statistics.median(values),
                range_seconds=max(values)-min(values), peak_children=peaks,
                passed=True, limits_changed=False)


if __name__ == '__main__':
    p = argparse.ArgumentParser(__doc__)
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--evidence', type=Path, required=True)
    p.add_argument('--samples', type=int, default=3, choices=range(3, 11))
    a = p.parse_args()
    result = measure(a.binary.resolve(), a.samples)
    a.evidence.parent.mkdir(parents=True, exist_ok=True)
    a.evidence.write_text(json.dumps(result, indent=2)+'\n')
    print(json.dumps(result))
