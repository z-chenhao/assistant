#!/usr/bin/env python3
"""Run every race package; isolate Bridge and audit SQLite test partitions.

SQLite tests own separate stores/temp roots. Separate processes also isolate
test globals and environment. Every discovered root runs exactly once, with
unchanged race/count/deadline flags and bounded CPU admission.
"""
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import threading
import time

from verify_suite import run_command, source_state

BRIDGE = 'github.com/z-chenhao/erii/internal/llm/bridge'
SQLITE = 'github.com/z-chenhao/erii/internal/agent/sqlite'
RACE = ['go', 'test', '-race', '-count=1', '-timeout=15m']


def groups(packages):
    if (not packages or len(set(packages)) != len(packages) or
            not {BRIDGE, SQLITE} <= set(packages) or
            any(not isinstance(p, str) or not p.startswith('github.com/z-chenhao/erii/') or
                any(c.isspace() for c in p) for p in packages)):
        raise ValueError('invalid or incomplete Go package inventory')
    return [('bridge', [BRIDGE]), ('remainder', [p for p in packages if p != BRIDGE])]


def cpu_budget():
    limits = [4, os.cpu_count() or 1]
    if hasattr(os, 'sched_getaffinity'):
        limits.append(len(os.sched_getaffinity(0)))
    if os.environ.get('GOMAXPROCS'):
        limits.append(int(os.environ['GOMAXPROCS']))
    if min(limits) < 1:
        raise ValueError('invalid race CPU budget')
    return min(limits)


def test_names(text):
    names, summaries = [], 0
    for line in text.splitlines():
        if re.fullmatch(r'(Test|Example|Fuzz)[A-Za-z0-9_]*', line):
            if summaries:
                raise ValueError('tests follow the discovery terminator')
            names.append(line)
        elif re.fullmatch(r'Benchmark[A-Za-z0-9_]*', line):
            continue  # Ordinary go test does not execute benchmarks.
        elif line.split()[:2] == ['ok', SQLITE]:
            summaries += 1
        elif not line.strip():
            continue
        else:
            raise ValueError('unrecognized Go test discovery output')
    if not names or len(names) != len(set(names)) or summaries != 1:
        raise ValueError('missing or duplicate SQLite test inventory')
    return names


def partitions(names, workers):
    if not names or len(names) != len(set(names)) or workers < 1:
        raise ValueError('invalid SQLite partition inventory')
    return [names[i::workers] for i in range(min(workers, len(names)))]


def sqlite_verdict(path, expected):
    observed, packages, outcomes = Counter(), [], {}
    for line in path.read_text().splitlines():
        event = json.loads(line)
        if not isinstance(event, dict):
            raise ValueError('malformed Go test event')
        name, action = event.get('Test'), event.get('Action')
        if name and '/' not in name and action in ('pass', 'skip', 'fail'):
            if event.get('Package') != SQLITE:
                raise ValueError('foreign SQLite test evidence')
            observed[name] += 1
            outcomes[name] = dict(outcome=action, elapsed_seconds=event.get('Elapsed'))
        elif not name and event.get('Package') == SQLITE and action in ('pass', 'skip', 'fail'):
            packages.append(action)
    if (observed != Counter(expected) or packages != ['pass'] or
            any(row['outcome'] == 'fail' for row in outcomes.values())):
        raise ValueError('missing, duplicate, unexpected or failed SQLite tests')
    return outcomes


def run_remainder(selected, folder, stopped):
    started = time.monotonic()
    discovery, metrics = RACE + ['-list', '.', SQLITE], {}
    listing = folder / 'sqlite-list.log'
    code = run_command(discovery, os.environ.copy(), listing, stopped, metrics)
    row = dict(name='remainder', packages=selected, discovery=discovery,
               discovery_metrics=metrics, returncode=code, workers=[])
    if code:
        return dict(row, wall_seconds=time.monotonic() - started)
    names = test_names(listing.read_text())
    budget = cpu_budget()
    slices = partitions(names, max(1, min(3, budget - 1)))
    jobs = [(f'sqlite-{i + 1}', RACE + ['-json', '-run', '^(' + '|'.join(group) + ')$', SQLITE], group)
            for i, group in enumerate(slices)]
    others = [p for p in selected if p != SQLITE]
    if others:
        jobs.append(('other-packages', RACE + ['-p=1', *others], None))
    def drive(job):
        name, command, expected = job
        output, metrics = folder / (name + '.log'), {}
        env = dict(os.environ, GOMAXPROCS='1')
        began = time.monotonic()
        code = run_command(command, env, output, stopped, metrics)
        result = dict(name=name, command=command, returncode=code,
                      wall_seconds=time.monotonic() - began, gomaxprocs=1, **metrics)
        if expected is not None:
            result['expected_tests'] = expected
            if code == 0:
                try:
                    result['tests'] = sqlite_verdict(output, expected)
                except (ValueError, OSError, KeyError) as error:
                    result.update(returncode=125, evidence_error=str(error))
        if result['returncode']:
            stopped.set()
        print(f"[race] {name}: {result['wall_seconds']:.2f}s (exit {result['returncode']})", flush=True)
        return result
    with ThreadPoolExecutor(max_workers=budget) as pool:
        workers = list(pool.map(drive, jobs))
    observed = Counter(name for worker in workers for name in worker.get('tests', {}))
    drained = metrics.get('owned_group_drained') is True and all(
        worker.get('owned_group_drained') is True for worker in workers)
    # wait4 in run_command measures each owned child independently, so summing
    # concurrent workers does not double-count global RUSAGE_CHILDREN deltas.
    costs = [metrics.get('cpu_seconds'), *[w.get('cpu_seconds') for w in workers]]
    return dict(row, expected_tests=names, cpu_budget=budget, workers=workers,
                selection_complete=observed == Counter(names),
                returncode=0 if drained and observed == Counter(names) and all(w['returncode'] == 0 for w in workers) else 1,
                owned_group_drained=drained,
                cpu_seconds=sum(costs) if all(c is not None for c in costs) else None,
                wall_seconds=time.monotonic() - started)


def main():
    source = source_state()
    packages = subprocess.check_output(['go', 'list', './...'], text=True).splitlines()
    plan = groups(packages)
    folder = Path('evidence/race')
    folder.mkdir(parents=True, exist_ok=True)
    for name in ('bridge', 'remainder', 'sqlite-list', 'sqlite-1', 'sqlite-2', 'sqlite-3', 'other-packages'):
        (folder / (name + '.log')).unlink(missing_ok=True)
    (folder / 'inventory.json').unlink(missing_ok=True)
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopped.set())
    rows = []
    passed = True
    try:
        for name, selected in plan:
            if not selected:
                continue
            if name == 'remainder':
                row = run_remainder(selected, folder, stopped)
            else:
                command, metrics = RACE + selected, {}
                started = time.monotonic()
                code = run_command(command, os.environ.copy(), folder / (name + '.log'), stopped, metrics)
                row = dict(name=name, packages=selected, command=command, returncode=code,
                           wall_seconds=time.monotonic() - started, **metrics)
            rows.append(row)
            if row['returncode']:
                passed = False
                break
    finally:
        stable = source == source_state()
        executed = [p for row in rows if row['returncode'] == 0 for p in row['packages']]
        passed = (passed and stable and not stopped.is_set() and sorted(executed) == sorted(packages)
                  and all(row.get('owned_group_drained') is True for row in rows))
        report = dict(version=1, **source, source_stable=stable, packages=packages,
                      stages=rows, passed=passed)
        (folder / 'inventory.json').write_text(json.dumps(report, indent=2) + '\n')
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
