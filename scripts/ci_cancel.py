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


def processes(timeout=1):
    rows = subprocess.check_output(['ps', '-axo', 'pid=,ppid=,lstart=,stat='], text=True, timeout=timeout)
    result = {}
    for line in rows.splitlines():
        parts = line.split()
        if len(parts) == 8:
            result[int(parts[0])] = dict(parent=int(parts[1]), start=' '.join(parts[2:7]), state=parts[7])
    require(os.getpid() in result, 'process table cannot prove current process identity')
    return result


def descendants(root, timeout=1):
    table = processes(timeout)
    require(root in table, 'held Core process identity is unavailable')
    owned = {root}
    while True:
        children = {pid for pid, value in table.items() if value['parent'] in owned}
        if children <= owned:
            return {str(pid): table[pid] for pid in owned if pid in table}
        owned.update(children)


def process_usage(pids):
    return subprocess.check_output(['ps', '-p', ','.join(map(str, pids)),
        '-o', 'pid=,ppid=,stat=,time=,pcpu='], text=True, timeout=.1)


def before_stop_snapshot(binary, root):
    import run_core_cases as core
    records = []
    for case in sorted(root.glob('held-core-*/*'))[:4]:
        records.append(dict(fixture=case.name,
            runtime=core.feedback_state(core.CoreHarness(binary, case))))
    try:
        owner_path = root / 'owner.json'
        owner = json.loads(owner_path.read_text()) if owner_path.exists() else None
        table = processes(.1)
        graph = descendants(owner['pid'], .1) if owner and table.get(owner['pid'], {}).get('start') == owner['identity']['start'] else {}
        records.append(dict(load_average=os.getloadavg(), descendants=graph,
            process_usage=process_usage(graph) if graph else '',
            process_usage_columns=['pid', 'ppid', 'state', 'cpu_time', 'cpu_percent']))
    except Exception as error:
        records.append(dict(capture_error_type=type(error).__name__))
    return records


def child(binary, folder):
    import fixture_pool
    import run_core_cases as core
    dataset, _, _ = core.load_contract(core.ROOT, 'core-v1')
    variant = next(core.variants(dataset, next(c for c in dataset['cases'] if c['case_id'] == 'GC-17')))
    original = core.owned_sidecar
    (folder / 'owner.json').write_text(json.dumps(dict(pid=os.getpid(), identity=processes()[os.getpid()])))
    def hold(process, script):
        sidecar = original(process, script)
        record = dict(root=process.pid, sidecar=sidecar, descendants=descendants(process.pid),
                      counts=dict(completed=37, active=8, total=45),
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


class ProbeFailure(RuntimeError):
    def __init__(self, evidence):
        self.evidence = evidence
        super().__init__('real Core cancellation probe failed: ' + json.dumps(evidence))


def validate_ready(ready):
    require(isinstance(ready, list) and len(ready) == 4, 'real Core fixtures did not reach all checkpoints')
    require(all(isinstance(item, dict) and type(item.get('root')) is int and
                type(item.get('sidecar')) is int for item in ready), 'invalid Core process receipt')
    require(len({item.get('root') for item in ready}) == 4 and
            len({item.get('sidecar') for item in ready}) == 4, 'duplicate Core process receipt')
    for item in ready:
        root, sidecar = item.get('root'), item.get('sidecar')
        graph = item.get('descendants', {})
        require(root > 1 and sidecar > 1 and root != sidecar and isinstance(graph, dict) and
                str(root) in graph and str(sidecar) in graph and
                all(isinstance(value, dict) for value in graph.values()) and
                graph[str(sidecar)].get('parent') == root and
                all(value.get('start') and value.get('state') for value in graph.values()) and
                item.get('counts') == dict(completed=37, active=8, total=45) and
                item.get('checkpoint') == '37 completed / 8 active; checked by feedback_case',
                'invalid Core checkpoint or ownership receipt')


def run(binary, entry='direct'):
    if entry not in ('direct', 'make-step'):
        raise ValueError('unknown cancellation entry')
    source = suite.source_state()
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    with tempfile.TemporaryDirectory(prefix='erii-ci-cancel-') as directory:
        root = Path(directory)
        stopped = threading.Event()
        ready, trigger, before_stop = [], [], []
        def interrupt():
            deadline = time.monotonic() + 20
            try:
                while time.monotonic() < deadline and not stopped.is_set():
                    paths = list(root.glob('ready-*.json'))
                    if len(paths) == 4:
                        records = [json.loads(p.read_text()) for p in paths]
                        validate_ready(records)
                        ready.extend(records)
                        trigger.append(time.monotonic())
                        break
                    if not before_stop and time.monotonic() >= deadline - 2:
                        # Bounded snapshots inside the original ready deadline,
                        # before signalling or deleting disposable fixture data.
                        before_stop.extend(before_stop_snapshot(binary, root))
                    time.sleep(.02)
            except Exception as error:
                before_stop.append(dict(capture_error_type=type(error).__name__, error=str(error)[:2000]))
            finally:
                stopped.set()
        waiter = threading.Thread(target=interrupt)
        waiter.start()
        metrics = {}
        env = dict(os.environ, ERII_PROFILE_DIR=str(root / 'profiles'),
                   ERII_CORE_DIAGNOSTICS=str(root / 'diagnostics'))
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
            leaked = [int(pid) for item in ready for pid, prior in item.get('descendants', {}).items()
                      if (current := table.get(int(pid))) and current['start'] == prior['start'] and
                      not current['state'].startswith('Z')]
            elapsed = time.monotonic() - trigger[0] if trigger else None
            evidence = dict(returncode=code, ready=ready, metrics=metrics,
                            cleanup_seconds=elapsed, leaked_pids=leaked,
                            remaining_state=[p.name for p in root.glob('held-core-*')],
                            before_stop=before_stop,
                            diagnostics=[json.loads(p.read_text()) for p in sorted((root / 'diagnostics').glob('*.json'))],
                            log_tail=(root / 'probe.log').read_text()[-8000:])
            evidence['entry'] = entry
            if entry == 'make-step':
                phase_path = root / 'steps/cancel-probe.json'
                evidence['phase'] = json.loads(phase_path.read_text()) if phase_path.exists() else None
            evidence.update(**source, source_stable=source == suite.source_state(),
                            binary_sha256=binary_digest, passed=False,
                            scope='CI harness cancellation; not business Task cancel')
            try:
                validate_ready(ready)
                require(len(list(root.glob('ready-*.json'))) == 4, 'queued Core fixtures started after cancellation')
                require(trigger and code == 130 and metrics.get('owned_group_drained') is True,
                        'cancellation did not fail with a drained owning group')
                require(not leaked and not evidence['remaining_state'] and elapsed < 15,
                        'nested Core cancellation cleanup failed')
                if entry == 'make-step':
                    phase = evidence['phase']
                    require(phase and phase['returncode'] == 130 and phase['owned_group_drained'] is True and
                            phase['cancel_received_at'] is not None,
                            'Make phase did not retain cancellation receipt and cleanup proof')
                require(evidence['source_stable'] and binary_digest == hashlib.sha256(binary.read_bytes()).hexdigest(),
                        'source or executable changed during cancellation probe')
            except RuntimeError as error:
                evidence['error'] = str(error)
                raise ProbeFailure(evidence) from error
            evidence['passed'] = True
            return evidence
        finally:
            stopped.set()
            waiter.join(timeout=21)
            # A failed probe remains failed. Remove only captured descendants
            # whose start token still matches; never search by executable name.
            table = processes()
            for item in ready:
                for pid, prior in item.get('descendants', {}).items():
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
        try:
            report = run(args.binary.resolve(), args.entry)
        except ProbeFailure as error:
            args.evidence.parent.mkdir(parents=True, exist_ok=True)
            args.evidence.write_text(json.dumps(error.evidence, indent=2) + '\n')
            raise
        args.evidence.parent.mkdir(parents=True, exist_ok=True)
        args.evidence.write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps({k: v for k, v in report.items() if k not in ('log_tail', 'ready', 'diagnostics', 'before_stop')}))


if __name__ == '__main__':
    main()
