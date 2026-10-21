#!/usr/bin/env python3
"""Cancel four real held Core fixtures; inspect Go/Node descendants and state."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import signal
import shlex
import subprocess
import sys
import tempfile
import threading
import time

import verify_suite as suite
from verify_main import require


def processes():
    rows = subprocess.check_output(['ps', '-axo', 'pid=,ppid=,lstart=,stat='], text=True)
    result = {}
    for line in rows.splitlines():
        parts = line.split()
        if len(parts) == 8:
            result[int(parts[0])] = dict(parent=int(parts[1]), start=' '.join(parts[2:7]), state=parts[7])
    require(os.getpid() in result, 'process table cannot prove current process identity')
    return result


def descendants(root):
    table = processes()
    require(root in table, 'held Core process identity is unavailable')
    owned = {root}
    while True:
        children = {pid for pid, value in table.items() if value['parent'] in owned}
        if children <= owned:
            return {str(pid): table[pid] for pid in owned if pid in table}
        owned.update(children)


def child(binary, folder):
    import fixture_pool
    import run_core_cases as core
    dataset, _, _ = core.load_contract(core.ROOT, 'core-v1')
    variant = next(core.variants(dataset, next(c for c in dataset['cases'] if c['case_id'] == 'GC-17')))
    original = core.owned_sidecar
    def hold(process, script):
        sidecar = original(process, script)
        record = dict(root=process.pid, sidecar=sidecar, descendants=descendants(process.pid),
                      checkpoint='37 completed / 8 active; checked by feedback_case')
        require(str(sidecar) in record['descendants'], 'Sidecar absent from held Core ownership graph')
        path = folder / f'ready-{process.pid}.json'
        path.with_suffix('.tmp').write_text(json.dumps(record))
        path.with_suffix('.tmp').replace(path)
        while True:
            fixture_pool.current().check()
            time.sleep(.01)
    core.owned_sidecar = hold
    with tempfile.TemporaryDirectory(prefix='held-core-', dir=folder) as directory:
        root = Path(directory)
        def drive(index):
            case = root / str(index)
            case.mkdir()
            return core.feedback_case(binary, case, variant)
        fixture_pool.run_cases(list(range(20)), drive, 4)


def run(binary, entry='direct'):
    if entry not in ('direct', 'make-step'):
        raise ValueError('unknown cancellation entry')
    source = suite.source_state()
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    with tempfile.TemporaryDirectory(prefix='erii-ci-cancel-') as directory:
        root = Path(directory)
        stopped = threading.Event()
        ready, trigger = [], []
        def interrupt():
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline and not stopped.is_set():
                paths = list(root.glob('ready-*.json'))
                if len(paths) == 4:
                    ready.extend(json.loads(p.read_text()) for p in paths)
                    trigger.append(time.monotonic())
                    break
                time.sleep(.02)
            stopped.set()
        waiter = threading.Thread(target=interrupt)
        waiter.start()
        metrics = {}
        env = dict(os.environ, ERII_PROFILE_DIR=str(root / 'profiles'))
        command = [sys.executable, 'scripts/profile_harness.py',
                   'scripts/ci_cancel.py', '--child', '--binary', str(binary), '--evidence', str(root)]
        if entry == 'make-step':
            makefile = root / 'Makefile'
            makefile.write_text('.PHONY: fixture\nfixture:\n\t' + shlex.join(command) + '\n')
            command = [sys.executable, 'scripts/ci_step.py', '--name', 'cancel-probe', '--phase', 'test',
                       '--evidence', str(root / 'steps'), '--', 'make', '-f', str(makefile), 'fixture']
        try:
            code = suite.run_command(command, env, root / 'probe.log', stopped, metrics)
            table = processes()
            leaked = [int(pid) for item in ready for pid, prior in item['descendants'].items()
                      if (current := table.get(int(pid))) and current['start'] == prior['start'] and
                      not current['state'].startswith('Z')]
            elapsed = time.monotonic() - trigger[0] if trigger else None
            evidence = dict(returncode=code, ready=ready, metrics=metrics,
                            cleanup_seconds=elapsed, leaked_pids=leaked,
                            remaining_state=[p.name for p in root.glob('held-core-*')],
                            log_tail=(root / 'probe.log').read_text()[-8000:])
            require(len(ready) == 4, 'real Core fixtures did not reach all checkpoints: ' + json.dumps(evidence))
            require(len(list(root.glob('ready-*.json'))) == 4, 'queued Core fixtures started after cancellation')
            require(code not in (0, 125) and metrics.get('owned_group_drained') is True,
                    'cancellation did not fail with a drained owning group: ' + json.dumps(evidence))
            require(not leaked and not evidence['remaining_state'] and elapsed < 15,
                    'nested Core cancellation cleanup failed: ' + json.dumps(evidence))
            evidence['entry'] = entry
            if entry == 'make-step':
                phase = json.loads((root / 'steps/cancel-probe.json').read_text())
                require(phase['returncode'] == 130 and phase['owned_group_drained'] is True and
                        phase['cancel_received_at'] is not None,
                        'Make phase did not retain cancellation receipt and cleanup proof')
                evidence['phase'] = phase
            require(source == suite.source_state() and binary_digest == hashlib.sha256(binary.read_bytes()).hexdigest(),
                    'source or executable changed during cancellation probe')
            evidence.update(**source, source_stable=True, binary_sha256=binary_digest)
            evidence.update(passed=True, scope='CI harness cancellation; not business Task cancel')
            return evidence
        finally:
            stopped.set()
            waiter.join(timeout=21)
            # A failed probe remains failed. Remove only captured descendants
            # whose start token still matches; never search by executable name.
            table = processes()
            for item in ready:
                for pid, prior in item['descendants'].items():
                    current = table.get(int(pid))
                    if current and current['start'] == prior['start'] and not current['state'].startswith('Z'):
                        try:
                            os.kill(int(pid), signal.SIGKILL)
                        except ProcessLookupError:
                            pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--entry', choices=['direct', 'make-step'], default='direct')
    parser.add_argument('--child', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.child:
        child(args.binary.resolve(), args.evidence)
    else:
        args.evidence.unlink(missing_ok=True)
        report = run(args.binary.resolve(), args.entry)
        args.evidence.parent.mkdir(parents=True, exist_ok=True)
        args.evidence.write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps({k: v for k, v in report.items() if k not in ('log_tail', 'ready')}))


if __name__ == '__main__':
    main()
