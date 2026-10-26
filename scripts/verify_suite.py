#!/usr/bin/env python3
"""Run the complete offline gate inventory, serially within each shard.

Shards own disjoint reports and disposable fixtures. Five-run gates are never
split across workers: one failure prevents that series from passing.
"""
import argparse
import errno
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import platform
import signal
import subprocess
import sys
import threading
import time

import core_report
import m6_report

ROOT = Path(__file__).resolve().parents[1]


@dataclass(frozen=True)
class Gate:
    name: str
    script: str
    report: str
    attempts: int = 1
    arguments: tuple = ()


# Complete baseline inventory, including the initial account check AND its
# five stability runs. No assertion, count, deadline or fault point changes.
SHARDS = {
    "runtime": (
        Gate("foundation", "verify.py", "verification"),
        Gate("operations", "verify_operations.py", "operations",5),
        Gate("capacity", "verify_capacity.py", "capacity"),
        Gate("main", "verify_main.py", "main"),
        Gate("tasks", "verify_tasks.py", "tasks"),
        Gate("automation", "verify_automation.py", "automation", 5),
        Gate("children", "verify_children.py", "children", 5),
        Gate("scheduling", "verify_scheduling.py", "scheduling", 5),
        Gate("spend", "verify_spend.py", "spend", 5),
        Gate("task-control", "verify_task_control.py", "task-control", 5),
        Gate("cases", "run_cases.py", "cases"),
        Gate("task-cases", "run_task_cases.py", "task-cases"),
        Gate("events", "", "events"),
    ),
    "codemode": (Gate("codemode", "verify_codemode.py", "codemode", 5),
                 Gate("computer", "verify_computer.py", "computer", 5),
                 Gate("computer-use", "verify_computer_use.py", "computer-use", 5),
                 Gate("code-recovery", "verify_code_recovery.py", "code-recovery", 5),
                 Gate("core-cases", "run_core_cases.py", "core-cases", 5)),
    "security": (
        Gate("memory-maintenance", "verify_memory_maintenance.py", "memory-maintenance", 5),
        Gate("recall", "verify_recall.py", "recall", 5),
        Gate("tools", "verify_tools.py", "tools", 5),
        Gate("artifacts", "verify_artifacts.py", "artifacts", 5),
        Gate("gc03", "verify_gc03.py", "gc03"),
        Gate("gc05", "verify_gc05.py", "gc05"),
        Gate("gc04", "verify_gc04.py", "gc04"),
        Gate("gc06", "verify_gc06.py", "gc06"),
    ),
    "delivery": (Gate("delivery", "verify_delivery.py", "delivery", 5),
                 Gate("channels", "verify_channels.py", "channels", 5),
                 Gate("gc07", "verify_gc07.py", "gc07")),
    "communication": (Gate("gc12", "verify_gc12.py", "gc12", 5),
                      Gate("gc08", "verify_gc08.py", "gc08"),
                      Gate("gc14", "verify_gc14.py", "gc14"),
                      Gate("gc16", "verify_gc16.py", "gc16"),
                      Gate("gc01", "verify_gc01.py", "gc01"),
                      Gate("gc02", "verify_gc02.py", "gc02"),
                      Gate("gc09", "verify_gc09.py", "gc09"),
                      Gate("gc10", "verify_gc10.py", "gc10"),
                      Gate("gc11", "verify_gc11.py", "gc11"),
                      Gate("gc13-wording", "verify_gc13.py", "gc13-wording", arguments=("--ci-part", "wording")),
                      Gate("gc13-noise", "verify_gc13.py", "gc13-noise", arguments=("--ci-part", "noise"))),
    "personal": (Gate("m6-cases", "run_m6_cases.py", "m6-cases", 5),),
    "providers": (
        Gate("audit-references", "verify_audit_references.py", "audit-reference-history"),
        Gate("startup", "verify_startup.py", "startup"),
        Gate("local-provider", "verify_local_provider.py", "local-provider"),
        Gate("unscripted-entry", "verify_unscripted_cases.py", "unscripted-entry"),
        Gate("account-initial", "verify_account.py", "account"),
        Gate("capabilities", "verify_capabilities.py", "capabilities", 5),
        Gate("tui", "verify_tui.py", "tui", 5),
        Gate("terminal-input", "verify_terminal_input.py", "terminal-input"),
        Gate("pi-bridge", "", "pi-bridge", 5),
        Gate("account", "verify_account.py", "account", 5),
        Gate("models", "", "models"),
    ),
}


# Each part retains its complete five-run series. The join reconstructs the
# original logical Code Mode inventory; no partial job is a merge verdict.
CODEMODE_PARTS = ('control', 'GC-17', 'GC-19', 'GC-20', 'contracts')

# GC-07's native driver belongs to its consumer PR. These fixed selectors are
# usable before registration, without inventing a Case PASS on current main.
DELIVERY_PARTS = ('control', 'wording', 'noise')
GC07_VARIANTS = {
    'wording': ('wording-1', 'wording-2', 'wording-3'),
    'noise': ('noise-1', 'noise-2', 'order-1'),
}


def gc07_pairs(part):
    return [(variant, n) for variant in GC07_VARIANTS[part] for n in range(1, 6)]


def delivery_gates(part):
    if part not in DELIVERY_PARTS:
        raise ValueError('unknown delivery part')
    gates = SHARDS['delivery']
    if ([g.name for g in gates if g.name != 'gc07'] != ['delivery', 'channels'] or
            len([g for g in gates if g.name == 'gc07']) > 1):
        raise ValueError('unassigned or duplicate delivery gate')
    if part == 'control':
        return tuple(g for g in gates if g.name != 'gc07')
    case = next((g for g in gates if g.name == 'gc07'), None)
    if case is None:
        return ()
    if case != Gate('gc07', 'verify_gc07.py', 'gc07'):
        raise ValueError('GC-07 original gate drift')
    return (Gate('gc07-' + part, case.script, 'gc07-' + part,
                 arguments=('--ci-part', part)),)


def codemode_gates(part):
    if part == 'control':
        return tuple(g for g in SHARDS['codemode'] if g.name != 'core-cases')
    if part not in CODEMODE_PARTS:
        raise ValueError('unknown Code Mode part')
    arguments = ('--contracts-only',) if part == 'contracts' else ('--case', part)
    return (Gate('core-' + part, 'run_core_cases.py', 'core-' + part, 5, arguments),)


SECURITY_PARTS = ('finance', 'cases', 'protection')
SECURITY_GATES = {
    'finance': ('gc03', 'gc06'),
    'cases': ('gc04', 'gc05'),
    'protection': ('memory-maintenance', 'recall', 'tools', 'artifacts'),
}


def security_gates(part):
    if part not in SECURITY_PARTS:
        raise ValueError('unknown security part')
    # Every registered gate must belong to a fixed part; unknown additions fail closed.
    gates = SHARDS['security']
    if any(g.name not in sum(SECURITY_GATES.values(), ()) for g in gates):
        raise ValueError('unassigned security gate')
    return tuple(g for g in gates if g.name in SECURITY_GATES[part])


PROVIDER_PARTS = ('cli', 'models')


def provider_gates(part):
    if part not in PROVIDER_PARTS:
        raise ValueError('unknown providers part')
    return tuple(g for g in SHARDS['providers'] if (g.name == 'models') == (part == 'models'))


GC13_VARIANTS = {'wording': ('wording-1','wording-2','wording-3'),
                 'noise': ('noise-1','noise-2','order-1')}
COMMUNICATION_PARTS = ('gc01', 'gc02', 'gc09', 'gc10', 'gc13-wording', 'gc13-noise', 'channels')


def communication_gates(part):
    if part not in COMMUNICATION_PARTS:
        raise ValueError('unknown communication part')
    return tuple(g for g in SHARDS['communication'] if g.name == part or
                 (part == 'channels' and g.name not in COMMUNICATION_PARTS[:-1]))


def personal_gates(part):
    if part not in m6_report.PARTS:
        raise ValueError('unknown M6 part')
    return (Gate('m6-'+part,'run_m6_cases.py','m6-'+part,5,('--part',part)),)


def source_state():
    sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    diff = subprocess.check_output(['git', 'diff', '--binary', 'HEAD'], cwd=ROOT)
    return dict(source_sha=sha, source_dirty=bool(diff),
                source_diff_sha256=hashlib.sha256(diff).hexdigest())


def runner_hardware():
    info = dict(logical_cpus=os.cpu_count(), affinity_cpus=None, cpu_model=platform.processor() or None)
    try:
        if hasattr(os, 'sched_getaffinity'):
            info['affinity_cpus'] = len(os.sched_getaffinity(0))
        if sys.platform == 'linux':
            for line in Path('/proc/cpuinfo').read_text().splitlines():
                key, separator, value = line.partition(':')
                if separator and key.strip() == 'model name':
                    info['cpu_model'] = value.strip()
                    break
        elif sys.platform == 'darwin':
            info['cpu_model'] = subprocess.check_output(['sysctl', '-n', 'machdep.cpu.brand_string'], text=True).strip()
    except (OSError, subprocess.CalledProcessError) as error:
        info['unavailable'] = type(error).__name__
    return info


def report_name(gate, attempt):
    suffix = f"-{attempt}" if gate.attempts > 1 else ""
    if gate.name == "tui" and attempt == 1:
        suffix = ""  # Preserve the existing feature-map path.
    return gate.report + suffix + ".json"


def command(gate, attempt, binary, evidence):
    env = os.environ.copy()
    if gate.script:
        env["ERII_PROFILE_DIR"] = str(evidence / "profiles" / f"{gate.name}-{attempt}")
        args = [sys.executable, "scripts/profile_harness.py", "scripts/" + gate.script,
                "--binary", str(binary), "--evidence", str(evidence / report_name(gate, attempt)), *gate.arguments]
        output = evidence / "logs" / f"{gate.name}-{attempt}.log"
    elif gate.name == "pi-bridge":
        args = ["node", "--test", "--test-reporter=tap",
                *map(str, sorted(Path("provider-bridge/test").glob("*.test.mjs")))]
        output = evidence / "pi-bridge.tap"
    elif gate.name == "models":
        args = ["go", "test", "-count=5", "-json", "./internal/llm/...", "./internal/agent/sqlite"]
        output = evidence / "models.jsonl"
    else:
        args = ["go", "test", "-count=1", "-v", "./internal/event/integration"]
        env.update(ERII_TEST_BINARY=str(binary), ERII_EVENT_EVIDENCE=str(evidence / "events.json"))
        output = evidence / "logs" / "events-1.log"
    return args, env, output


def group_alive(pgid):
    """Ignore exited zombies; they own no running work or listening sockets."""
    try:
        os.killpg(pgid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        # Darwin can return EPERM as an orphaned group disappears. Confirm
        # absence/zombie state with ps; an unreadable process table still fails
        # closed in run_command. Other platforms retain their strict error.
        if sys.platform != 'darwin':
            raise
    rows = subprocess.check_output(['ps', '-axo', 'pgid=,stat='], text=True)
    if not rows.strip():
        raise OSError(errno.EIO, 'process table is empty')
    return any(int(parts[0]) == pgid and not parts[1].startswith('Z')
               for row in rows.splitlines() if len(parts := row.split()) == 2)


def signal_group(pgid, sig):
    try:
        os.killpg(pgid, sig)
    except ProcessLookupError:
        pass
    except PermissionError:
        if sys.platform != 'darwin' or group_alive(pgid):
            raise


def run_command(args, env, output, stopped, metrics=None):
    """Reap owned groups on interruption; Python harnesses unwind cleanup."""
    metrics = metrics if metrics is not None else {}
    metrics.update(cpu_seconds=None, cancel_cleanup_seconds=None)
    if stopped.is_set():
        return 130
    interrupted = None
    with output.open("a") as log:
        with subprocess.Popen(args, cwd=ROOT, env=env, stdout=log,
                              stderr=subprocess.STDOUT, start_new_session=True) as child:
            try:
                while True:
                    pid, status, usage = os.wait4(child.pid, os.WNOHANG)
                    if pid:
                        child.returncode = os.waitstatus_to_exitcode(status)
                        metrics['cpu_seconds'] = usage.ru_utime + usage.ru_stime
                        break
                    if stopped.is_set() and interrupted is None:
                        interrupted = time.monotonic()
                        signal_group(child.pid, signal.SIGTERM)
                    # Leave time to drain and retain evidence before Actions'
                    # 7.5-second SIGINT grace expires at the entry process.
                    if interrupted is not None and time.monotonic() - interrupted > 5:
                        signal_group(child.pid, signal.SIGKILL)
                    time.sleep(.05)
            finally:
                # The group leader can exit before an uncooperative child.
                # Waiting only for the leader used to leave that child alive.
                signal_group(child.pid, signal.SIGKILL)
                deadline = time.monotonic() + 2
                try:
                    while group_alive(child.pid) and time.monotonic() < deadline:
                        time.sleep(.02)
                    metrics['owned_group_drained'] = not group_alive(child.pid)
                except (OSError, subprocess.CalledProcessError):
                    metrics['owned_group_drained'] = False
                if interrupted is not None:
                    metrics['cancel_cleanup_seconds'] = time.monotonic() - interrupted
            if not metrics['owned_group_drained']:
                return 125
            return child.returncode if interrupted is None else 130


def run_shard(name, binary, evidence, stopped, gates=None, previous=(), gate_jobs=1, record=None):
    gates = SHARDS[name] if gates is None else gates
    if gate_jobs == 2 and name in ('runtime', 'codemode', 'providers', 'security', 'delivery', 'communication'):
        return concurrent_gates(name, binary, evidence, stopped, gates, previous)
    rows = list(previous)
    for gate in gates:
        for attempt in range(1, gate.attempts + 1):
            args, env, output = command(gate, attempt, binary, evidence)
            started = time.monotonic()
            metrics = {}
            try:
                code = run_command(args, env, output, stopped, metrics)
            except OSError as error:
                output.write_text(str(error) + "\n")
                code = 127
            row = dict(gate=gate.name, attempt=attempt, command=args, returncode=code,
                       elapsed_seconds=time.monotonic() - started, log=str(output), **metrics)
            rows.append(row)
            if record is not None:
                record(gate.name, rows)
            else:
                (evidence / f"timings-{name}.json").write_text(json.dumps(rows, indent=2) + "\n")
            print(f"[{name}] {gate.name} {attempt}/{gate.attempts}: {row['elapsed_seconds']:.2f}s (exit {code})", flush=True)
            if code:
                if output.exists():
                    print(output.read_text()[-8000:], file=sys.stderr)
                return rows
    return rows


def gate_execution_order(gates):
    # Measured long series go first. GC-01 and GC-12 retain the initial pair;
    # GC-02 must take the next free slot before short gates. Its former queue
    # tail exceeded the CI deadline even with all 30 rounds finished. Security's
    # GC-03/GC-04 likewise start before the shorter protection series. GC-09
    # and GC-10 follow GC-02 and precede short communication gates. Keep two
    # workers, report inventory order and every series' serial repetitions.
    def priority(gate):
        if gate.name in ('core-cases','pi-bridge','gc01','gc03','gc04'): return 0
        if gate.name in ('gc12','gc05','gc06'): return 1
        if gate.name in ('gc02','gc09','gc10','gc11'): return 2
        return 3
    return sorted(gates,key=priority)


def concurrent_gates(name, binary, evidence, stopped, gates, previous):
    """Independent series overlap; every five-run series stays on one worker."""
    isolated = tuple(g for g in gates if g.name in ('models', 'events'))
    parallel = tuple(g for g in gates if g not in isolated)
    lock, failed = threading.Lock(), threading.Event()
    by_gate = {}
    class Stop:
        def is_set(self):
            return stopped.is_set() or failed.is_set()
    stop = Stop()
    def joined():
        return list(previous) + [row for g in parallel for row in by_gate.get(g.name, ())]
    def record(gate, rows):
        with lock:
            by_gate[gate] = list(rows)
            path = evidence / f'timings-{name}.json'
            temporary = path.with_suffix('.tmp')
            temporary.write_text(json.dumps(joined(), indent=2) + '\n')
            temporary.replace(path)
    def drive(gate):
        if stop.is_set():
            return
        try:
            rows = run_shard(name, binary, evidence, stop, (gate,), record=record)
        except BaseException:
            failed.set()
            raise
        if len(rows) != gate.attempts or any(r['returncode'] for r in rows):
            failed.set()
    with ThreadPoolExecutor(max_workers=2) as pool:
        list(pool.map(drive, gate_execution_order(parallel)))
    rows = joined()
    if not stop.is_set() and isolated:
        rows = run_shard(name, binary, evidence, stopped, isolated, rows)
    return rows


def catalogue():
    """Describe the same inventory that execution consumes, without launching it."""
    return [dict(shard=shard, gate=gate.name, script=gate.script or None,
                 attempts=gate.attempts,
                 reports=[report_name(gate, n) for n in range(1, gate.attempts + 1)])
            for shard, gates in SHARDS.items() for gate in gates]


def select_gates(parser, shard, selected):
    plans = {name: gates for name, gates in SHARDS.items() if not shard or name == shard}
    if selected:
        if len(selected) != len(set(selected)):
            parser.error('--gate must not contain duplicates')
        available = {g.name for gates in plans.values() for g in gates}
        if set(selected) - available:
            parser.error('--gate is outside the selected inventory: ' + ', '.join(sorted(set(selected) - available)))
        plans = {name: tuple(g for g in gates if g.name in selected) for name, gates in plans.items()}
        plans = {name: gates for name, gates in plans.items() if gates}
    return plans


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--list', action='store_true', help='print the gate catalogue as JSON; no binary or fixtures needed')
    parser.add_argument('--gate', action='append', help='run a named gate with its full repetition count; repeat to select several')
    parser.add_argument("--shard", choices=SHARDS)
    parser.add_argument('--codemode-part', choices=CODEMODE_PARTS)
    parser.add_argument('--personal-part', choices=m6_report.PARTS)
    parser.add_argument('--provider-part', choices=PROVIDER_PARTS)
    parser.add_argument('--security-part', choices=SECURITY_PARTS)
    parser.add_argument('--delivery-part', choices=DELIVERY_PARTS)
    parser.add_argument('--communication-part', choices=COMMUNICATION_PARTS)
    parser.add_argument("--jobs", type=int, default=2)
    parser.add_argument("--gate-jobs", type=int, choices=[1, 2], default=int(os.environ.get('ERII_VERIFY_GATE_JOBS', '1')))
    parser.add_argument("--binary", type=Path, default=Path("bin/erii"))
    parser.add_argument("--evidence", type=Path, default=Path("evidence"))
    args = parser.parse_args()
    if args.list:
        if args.gate or args.codemode_part or args.personal_part or args.provider_part or args.communication_part or args.security_part or args.delivery_part:
            parser.error('--list cannot be combined with gate/part execution')
        print(json.dumps([row for row in catalogue() if not args.shard or row['shard'] == args.shard], indent=2))
        return 0
    if args.gate and (args.codemode_part or args.personal_part or args.provider_part or args.communication_part or args.security_part or args.delivery_part):
        parser.error('--gate cannot be combined with a CI part')
    plans = select_gates(parser, args.shard, args.gate)
    if args.codemode_part and args.shard != 'codemode':
        parser.error('--codemode-part requires --shard codemode')
    if args.personal_part and args.shard != 'personal':
        parser.error('--personal-part requires --shard personal')
    if args.provider_part and args.shard != 'providers':
        parser.error('--provider-part requires --shard providers')
    if args.communication_part and args.shard != 'communication':
        parser.error('--communication-part requires --shard communication')
    if args.security_part and args.shard != 'security':
        parser.error('--security-part requires --shard security')
    if args.delivery_part and args.shard != 'delivery':
        parser.error('--delivery-part requires --shard delivery')
    if sum(bool(p) for p in (args.codemode_part, args.personal_part, args.provider_part,
                            args.communication_part, args.security_part, args.delivery_part)) > 1:
        parser.error('only one CI part can be selected')
    if args.jobs < 1:
        parser.error("--jobs must be positive")
    if args.gate_jobs not in (1, 2):
        parser.error('--gate-jobs must be 1 or 2')
    os.chdir(ROOT)
    binary, evidence = args.binary.resolve(), args.evidence.resolve()
    if not binary.is_file():
        parser.error("binary missing; run make build")
    source = source_state()
    binary_digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    names = list(plans)
    if args.codemode_part:
        plans['codemode'] = codemode_gates(args.codemode_part)
    if args.personal_part:
        plans['personal'] = personal_gates(args.personal_part)
    if args.provider_part:
        plans['providers'] = provider_gates(args.provider_part)
    if args.communication_part:
        plans['communication'] = communication_gates(args.communication_part)
    if args.security_part:
        plans['security'] = security_gates(args.security_part)
    if args.delivery_part:
        plans['delivery'] = delivery_gates(args.delivery_part)
    part = args.codemode_part or args.personal_part or args.provider_part or args.communication_part or args.security_part or args.delivery_part
    report_suffix = (args.shard or 'all') + ('-' + part if part else '')
    if args.gate:
        # Focused runs cannot overwrite or masquerade as a complete shard verdict.
        key = hashlib.sha256(json.dumps(sorted(args.gate)).encode()).hexdigest()[:12]
        evidence = evidence / 'focused' / key
        report_suffix = 'focused'
    evidence.mkdir(parents=True, exist_ok=True)
    (evidence / "logs").mkdir(exist_ok=True)
    (evidence / ('suite-' + report_suffix + '.json')).unlink(missing_ok=True)
    if "codemode" in names:
        (evidence / "core-summary.json").unlink(missing_ok=True)
    if "personal" in names:
        (evidence / "m6-summary.json").unlink(missing_ok=True)
    # Delete only reports owned by the selected shards; never user state.
    for name in names:
        (evidence / f"timings-{name}.json").unlink(missing_ok=True)
        for gate in plans[name]:
            for attempt in range(1, gate.attempts + 1):
                _, _, output = command(gate, attempt, binary, evidence)
                output.unlink(missing_ok=True)
                (evidence / report_name(gate, attempt)).unlink(missing_ok=True)
                if gate.script:
                    (evidence / "profiles" / f"{gate.name}-{attempt}" /
                     (Path(gate.script).stem + ".json")).unlink(missing_ok=True)
        if name == "runtime":
            (evidence / "relay.json").unlink(missing_ok=True)
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stopped.set())
    started = time.monotonic()
    # Repeated Go conformance contains real 100 ms recovery deadlines. Keep it
    # isolated from unrelated local harness I/O, as it is on its CI runner.
    deferred = None
    if not args.shard and "providers" in plans:
        deferred = next((g for g in plans["providers"] if g.name == "models"), None)
        plans["providers"] = tuple(g for g in plans["providers"] if g != deferred)
    with ThreadPoolExecutor(max_workers=args.jobs) as pool:
        results = list(pool.map(lambda name: run_shard(name, binary, evidence, stopped, plans[name], gate_jobs=args.gate_jobs), names))
    if deferred is not None and not stopped.is_set():
        index = names.index("providers")
        prefix = results[index]
        if len(prefix) == sum(g.attempts for g in plans["providers"]) and all(r["returncode"] == 0 for r in prefix):
            results[index] = run_shard("providers", binary, evidence, stopped, (deferred,), prefix)
    passed = not stopped.is_set() and all(
        len(rows) == sum(g.attempts for g in plans[name]) + (deferred.attempts if deferred and name == 'providers' else 0) and all(r["returncode"] == 0 for r in rows)
        for name, rows in zip(names, results))
    stable = source == source_state() and binary_digest == hashlib.sha256(binary.read_bytes()).hexdigest()
    passed = passed and stable
    if passed and "codemode" in names and not args.codemode_part and not args.gate:
        passed = core_report.write(evidence)
    if passed and "personal" in names and not args.personal_part and not args.gate:
        passed = m6_report.write(evidence)
    report = dict(version=1, **source, source_stable=stable,
                  workflow_sha=os.environ.get("GITHUB_SHA"), platform=platform.platform(), jobs=args.jobs,
                  gate_jobs=args.gate_jobs, selected_gates=args.gate,
                  complete_inventory=not bool(args.gate or part),
                  codemode_part=args.codemode_part, personal_part=args.personal_part,
                  provider_part=args.provider_part, communication_part=args.communication_part,
                  security_part=args.security_part, delivery_part=args.delivery_part,
                  run_id=os.environ.get("GITHUB_RUN_ID"), run_attempt=os.environ.get("GITHUB_RUN_ATTEMPT"),
                  image_os=os.environ.get('ImageOS'), image_version=os.environ.get('ImageVersion'),
                  hardware=runner_hardware(),
                  binary_sha256=binary_digest,
                  elapsed_seconds=time.monotonic() - started, passed=passed,
                  shards=dict(zip(names, results)))
    (evidence / ('suite-' + report_suffix + '.json')).write_text(json.dumps(report, indent=2) + "\n")
    return 0 if passed else 1


if __name__ == "__main__":
    sys.exit(main())
