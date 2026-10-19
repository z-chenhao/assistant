#!/usr/bin/env python3
"""Run bounded early regressions in addition to the unchanged full merge gate."""
import argparse
import json
import os
from pathlib import Path
import signal
import threading

import verify_suite as suite

# This private inventory supplies early feedback; it never replaces SHARDS.
GATES = {
    'runtime': {'foundation', 'main', 'tasks'},
    'codemode': {'codemode', 'code-recovery'},
    'security': {'tools', 'artifacts', 'recall', 'memory-maintenance'},
    'delivery': {'delivery'},
    'providers': {'local-provider', 'capabilities', 'terminal-input'},
}


def feedback_gates(name):
    return tuple(suite.Gate(g.name, g.script, g.report,
                           arguments=('--feedback',) if g.name == 'delivery' else ())
                 for g in suite.SHARDS[name] if g.name in GATES[name])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--plan', type=Path, required=True)
    parser.add_argument('--binary', type=Path, default=Path('bin/personal-agent'))
    parser.add_argument('--evidence', type=Path, default=Path('evidence/feedback'))
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())
    source = suite.source_state()
    if (plan.get('source_sha') != source['source_sha'] or not isinstance(plan.get('shards'), list) or
            len(set(plan['shards'])) != len(plan['shards']) or
            any(s not in GATES for s in plan['shards']) or plan.get('full_shards') != sorted(GATES)):
        raise ValueError('invalid or stale impact plan')
    args.evidence.mkdir(parents=True, exist_ok=True)
    (args.evidence / 'logs').mkdir(exist_ok=True)
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopped.set())
    # Always include race-instrumented Tool/schema contracts. The complete race
    # suite remains an independent mandatory job after this layer succeeds.
    code = suite.run_command(['go', 'test', '-race', '-count=1', './internal/tool/...'],
                             os.environ.copy(), args.evidence / 'logs/race.log', stopped)
    rows = {}
    passed = code == 0
    for name in plan['shards']:
        if not passed:
            break
        gates = feedback_gates(name)
        rows[name] = suite.run_shard(name, args.binary.resolve(), args.evidence.resolve(), stopped, gates)
        passed = len(rows[name]) == len(gates) and all(r['returncode'] == 0 for r in rows[name])
    stable = source == suite.source_state()
    passed = passed and not stopped.is_set() and stable
    report = dict(version=1, **source, source_stable=stable, passed=passed, race_returncode=code,
                  impact=plan, shards=rows, scope='early feedback; no full merge evidence')
    (args.evidence / 'feedback.json').write_text(json.dumps(report, indent=2) + '\n')
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
