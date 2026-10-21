"""Regression protection for gate completeness, failures and cancellation."""
import itertools
import json
import os
from pathlib import Path
import re
import signal
import socket
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

import ci_gate
import verify_suite as suite


class VerificationTests(unittest.TestCase):
    def test_delivery_two_series_overlap_without_splitting_five_rounds(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'logs').mkdir()
            gates = tuple(suite.Gate(name, '', name, 5) for name in ('delivery','channels'))
            def command(gate, attempt, *_):
                other = 'channels' if gate.name == 'delivery' else 'delivery'
                # Each first round must observe its peer in flight. A serial
                # dispatch fails deterministically instead of relying on a
                # fragile wall-time speed threshold.
                script = ('from pathlib import Path\nimport time\n'
                          f'root=Path({str(root)!r})\n'
                          f'name={gate.name!r}; attempt={attempt}\n'
                          'with (root/(name+".rounds")).open("a") as out: out.write(str(attempt)+"\\n")\n'
                          'if attempt==1:\n'
                          ' (root/(name+".started")).touch()\n'
                          ' deadline=time.monotonic()+2\n'
                          f' while not (root/{(other+".started")!r}).exists():\n'
                          '  assert time.monotonic()<deadline, "peer series never admitted"\n'
                          '  time.sleep(.01)\n')
                return [sys.executable,'-c',script], os.environ.copy(), root/'logs'/f'{gate.name}-{attempt}.log'
            with patch.object(suite, 'command', command):
                rows = suite.run_shard('delivery',Path('/binary'),root,threading.Event(),gates,gate_jobs=2)
            self.assertEqual([(r['gate'],r['attempt']) for r in rows],
                             [(name,i) for name in ('delivery','channels') for i in range(1,6)])
            self.assertTrue(all(r['returncode']==0 and r['owned_group_drained'] for r in rows))
            for name in ('delivery','channels'):
                self.assertEqual((root/(name+'.rounds')).read_text().splitlines(),list('12345'))

    def test_signal_eperm_can_only_accept_confirmed_absence_on_darwin(self):
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'darwin'), \
             patch.object(suite, 'group_alive', return_value=False) as inspect:
            suite.signal_group(42, signal.SIGKILL)
            inspect.assert_called_once_with(42)
            inspect.return_value = True
            with self.assertRaises(PermissionError):
                suite.signal_group(42, signal.SIGTERM)
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'linux'):
            with self.assertRaises(PermissionError):
                suite.signal_group(42, signal.SIGKILL)

    def test_darwin_eperm_requires_authoritative_group_inspection(self):
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'darwin'), \
             patch.object(suite.subprocess, 'check_output') as inspect:
            for table, alive in [('42 Z\n', False), ('42 S\n', True),
                                 ('42 Z\n42 R\n', True), ('99 S\n', False)]:
                inspect.return_value = table
                self.assertEqual(suite.group_alive(42), alive)
            inspect.return_value = ''
            with self.assertRaises(OSError):
                suite.group_alive(42)
            inspect.side_effect = PermissionError
            with self.assertRaises(PermissionError):
                suite.group_alive(42)
        with patch.object(suite.os, 'killpg', side_effect=PermissionError), \
             patch.object(suite.sys, 'platform', 'linux'):
            with self.assertRaises(PermissionError):
                suite.group_alive(42)

    def test_pre_optimization_inventory_and_disjoint_evidence(self):
        expected = {
            "verify.py": 1, "verify_local_provider.py": 1, "verify_account.py": 6,
            "verify_startup.py": 1,
            "verify_operations.py":5, "verify_capacity.py":1,
            "verify_main.py": 1, "verify_tasks.py": 1, "verify_children.py": 5,
            "verify_scheduling.py": 5, "verify_spend.py": 5, "verify_task_control.py": 5,
            "verify_tools.py": 5, "verify_artifacts.py": 5, "verify_delivery.py": 5, "verify_channels.py": 5,
            "verify_recall.py": 5,
            "verify_automation.py": 5,
            "verify_memory_maintenance.py": 5,
            "verify_computer.py": 5,
            "verify_computer_use.py": 5,
            "verify_capabilities.py": 5, "run_cases.py": 1, "run_task_cases.py": 1,
            "run_core_cases.py": 5, "verify_code_recovery.py": 5, "verify_tui.py": 5, "verify_terminal_input.py": 1, "verify_codemode.py": 5,
            "run_m6_cases.py": 5,
            "verify_gc12.py": 5,
        }
        counts, paths = {}, set()
        gates = [gate for group in suite.SHARDS.values() for gate in group]
        for gate in gates:
            if gate.script:
                counts[gate.script] = counts.get(gate.script, 0) + gate.attempts
                for attempt in range(1, gate.attempts + 1):
                    path = suite.report_name(gate, attempt)
                    self.assertNotIn(path, paths)
                    paths.add(path)
        self.assertEqual(counts, expected)
        scripts = suite.ROOT / "scripts"
        discovered = {p.name for p in scripts.glob("verify*.py")} - {"verify_suite.py"}
        discovered.update(p.name for p in scripts.glob("run*cases.py"))
        self.assertEqual(set(counts), discovered, "every standalone harness must be registered")
        self.assertEqual([(g.name, g.attempts) for g in gates if not g.script],
                         [("events", 1), ("pi-bridge", 5), ("models", 1)])
        model = next(g for g in gates if g.name == "models")
        args, _, _ = suite.command(model, 1, Path('/binary'), Path('/evidence'))
        self.assertEqual(args, ['go', 'test', '-count=5', '-json', './internal/llm/...', './internal/agent/sqlite'])

    def test_aggregate_rejects_every_non_success_combination(self):
        for states in itertools.product(['success', 'failure', 'cancelled', 'skipped'], repeat=len(ci_gate.REQUIRED)):
            needs = {name: {'result': value} for name, value in zip(sorted(ci_gate.REQUIRED), states)}
            self.assertEqual(ci_gate.passed(needs), all(s == 'success' for s in states))
        good = {name: {'result': 'success'} for name in ci_gate.REQUIRED}
        for name in good:
            self.assertFalse(ci_gate.passed({k: v for k, v in good.items() if k != name}))
        for invalid in [None, [], {}, dict(good, unexpected={'result': 'success'}),
                        dict(good, race={}), dict(good, race='success')]:
            self.assertFalse(ci_gate.passed(invalid))

    def test_all_ci_jobs_keep_the_ten_minute_budget(self):
        for path in sorted((suite.ROOT / '.github/workflows').glob('*.y*ml')):
            text = path.read_text()
            jobs = re.findall(r'^  [a-zA-Z0-9_-]+:\n(    .*?)(?=^  [a-zA-Z0-9_-]+:|\Z)',
                              text.split('jobs:', 1)[1], re.M | re.S)
            self.assertTrue(jobs, str(path))
            for job in jobs:
                deadlines = re.findall(r'^    timeout-minutes: (\d+)$', job, re.M)
                self.assertEqual(len(deadlines), 1, str(path))
                self.assertTrue(0 < int(deadlines[0]) <= 10, str(path))

    def test_workflow_topology_is_non_bypassable(self):
        workflow = (suite.ROOT / '.github/workflows/ci.yml').read_text()
        shards = json.loads(re.search(r'        shard: (\[.*\])', workflow)[1])
        self.assertEqual(set(shards), set(suite.SHARDS) - {'codemode','personal'})
        self.assertEqual(len(shards), 5)
        parts = json.loads(re.search(r'        part: (\[.*\])', workflow)[1])
        self.assertEqual(parts, list(suite.CODEMODE_PARTS))
        checks = workflow[workflow.index('  checks:'):]
        needs = json.loads(re.search(r'    needs: (\[.*\])', checks)[1])
        self.assertEqual(set(needs), ci_gate.REQUIRED)
        for required in ('name: Ubuntu checks', 'if: ${{ always() }}',
                         'NEEDS_JSON: ${{ toJSON(needs) }}', 'run: python3 scripts/ci_gate.py',
                         'cancel-in-progress: true', 'fail-fast: false',
                         'retention-days: 14', 'if-no-files-found: error'):
            self.assertIn(required, workflow)
        for forbidden in ('continue-on-error', 'paths-ignore:', 'fromJSON', 'ci_feedback.py', 'ci_impact.py'):
            self.assertNotIn(forbidden, workflow)
        self.assertFalse((suite.ROOT / '.github/workflows/ci-shadow.yml').exists())
        self.assertEqual(workflow.count('ref: ${{ github.event.pull_request.head.sha || github.sha }}'), 8)
        self.assertIn("sandbox: ${{ matrix.shard == 'providers' || matrix.shard == 'security' || matrix.shard == 'communication' }}", workflow)
        self.assertEqual(workflow.count('uses: ./.github/actions/ci-inputs'), 5)
        action = (suite.ROOT / '.github/actions/ci-inputs/action.yml').read_text()
        self.assertIn("if: inputs.sandbox == 'true'", action)
        self.assertIn('make build', action)
        self.assertNotIn('ci_bundle.py', action)
        # Full gates start independently; no slow test or build producer gates
        # their admission, and there is no cross-image executable reuse.
        for name in ('fast', 'race', 'verify', 'codemode', 'personal'):
            block = workflow.split('  ' + name + ':\n', 1)[1].split('    steps:', 1)[0]
            self.assertNotIn('needs:', block)
        join = workflow.split('  codemode_join:\n', 1)[1].split('  checks:', 1)[0]
        self.assertIn('needs: codemode', join)
        self.assertIn('test "$PART_RESULT" = success', join)
        self.assertIn('python3 scripts/ci_codemode.py --parts evidence/parts --evidence evidence', join)
        import m6_report
        parts = json.loads(re.search(r'        m6part: (\[.*\])', workflow)[1])
        self.assertEqual(parts, list(m6_report.PARTS))
        personal = workflow.split('  personal:\n',1)[1].split('  personal_join:',1)[0]
        self.assertIn('sandbox: "true"',personal,'configured MCP requires the Linux sandbox even for read-only fixtures')
        personal_join = workflow.split('  personal_join:\n',1)[1].split('  codemode:',1)[0]
        self.assertIn('needs: personal',personal_join)
        self.assertIn('test "$PART_RESULT" = success',personal_join)
        self.assertIn('python3 scripts/ci_personal.py --parts evidence/m6-parts --evidence evidence',personal_join)
        self.assertNotIn('ci_bundle.py', workflow)
        self.assertIn('python-version: "3.10.22"', workflow)
        self.assertIn("--name python310 --phase test -- python -m unittest discover -s scripts -p 'test_*.py'", workflow)
        self.assertEqual(workflow.count("temporary-fixtures: ${{ matrix.shard == 'providers' }}"), 1)
        for name in ('fast', 'codemode'):
            block = re.search(r'^  ' + name + r':\n(.*?)(?=^  \w+:|\Z)', workflow, re.M | re.S)[1]
            self.assertNotIn('temporary-fixtures:', block)
        race = workflow.split('  race:\n', 1)[1].split('  verify:\n', 1)[0]
        self.assertIn('temporary-fixtures: "true"', race)
        self.assertLess(race.index('ci_tmpfs.py cleanup'), race.index('Retain exact-SHA'))
        self.assertIn("--name receipt-race-disk --phase test -- go test -race ./internal/agent/sqlite -run '^TestArtifactQueryRecovery' -count=5", workflow)
        verify = workflow.split('  verify:\n', 1)[1].split('  codemode:\n', 1)[0]
        self.assertIn("if: ${{ always() && matrix.shard == 'providers' }}", verify)
        self.assertLess(verify.index('ci_tmpfs.py cleanup'), verify.index('Retain exact-SHA'))
        self.assertIn('temporary-fixtures:', action)
        self.assertIn("inputs.temporary-fixtures == 'true' && 'setup' || 'disk'", action)
        self.assertIn('default: "false"', action.split('  temporary-fixtures:\n', 1)[1].split('runs:', 1)[0])

    def test_codemode_gate_series_parallelize_without_splitting_repetitions(self):
        gates = suite.SHARDS['codemode']
        ordered = suite.gate_execution_order(gates)
        self.assertEqual(ordered[0].name, 'core-cases')
        self.assertCountEqual([g.name for g in ordered], [g.name for g in gates])
        seen, active, peak = [], 0, 0
        lock = threading.Lock()

        def run_shard(name, binary, evidence, stopped, selected, previous=(), gate_jobs=1, record=None):
            nonlocal active, peak
            gate = selected[0]
            with lock:
                active += 1
                peak = max(peak, active)
            try:
                time.sleep(.02)
                rows = [dict(gate=gate.name, attempt=attempt, returncode=0)
                        for attempt in range(1, gate.attempts + 1)]
                if record:
                    record(gate.name, rows)
                seen.append((gate.name, [row['attempt'] for row in rows]))
                return rows
            finally:
                with lock:
                    active -= 1

        with tempfile.TemporaryDirectory() as temp, patch.object(suite, 'run_shard', run_shard):
            rows = suite.concurrent_gates('codemode', Path('/binary'), Path(temp),
                                          threading.Event(), gates, ())
        self.assertEqual(peak, 2)
        self.assertEqual({name: attempts for name, attempts in seen},
                         {gate.name: list(range(1, gate.attempts + 1)) for gate in gates})
        self.assertEqual(len(rows), sum(gate.attempts for gate in gates))

    def test_failed_repetition_stops_and_records_failure(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'logs').mkdir()
            gate = suite.Gate('test', 'fixture.py', 'test', 5)
            def command(*_):
                return ([sys.executable, '-c', 'print("observed failure"); raise SystemExit(9)'],
                        os.environ.copy(), root / 'logs/test.log')
            handlers = {sig: signal.getsignal(sig) for sig in [signal.SIGINT, signal.SIGTERM]}
            try:
                with patch.dict(suite.SHARDS, {'test': (gate,)}, clear=True), patch.object(suite, 'command', command), \
                     patch.object(sys, 'argv', ['verify_suite.py', '--binary', sys.executable, '--evidence', str(root)]):
                    self.assertEqual(suite.main(), 1)
            finally:
                for sig, handler in handlers.items():
                    signal.signal(sig, handler)
            report = json.loads((root / 'suite-all.json').read_text())
            self.assertFalse(report['passed'])
            self.assertEqual(len(report['shards']['test']), 1)
            self.assertEqual(json.loads((root / 'timings-test.json').read_text())[0]['returncode'], 9)

    def test_unverifiable_group_cleanup_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            metrics = {}
            with patch.object(suite, 'group_alive', side_effect=OSError('ps unavailable')):
                code = suite.run_command([sys.executable, '-c', 'pass'], os.environ.copy(),
                                         Path(temp) / 'log', threading.Event(), metrics)
            self.assertNotEqual(code, 0)
            self.assertFalse(metrics['owned_group_drained'])

    def test_source_change_during_gate_cannot_claim_exact_sha_success(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            gate = suite.Gate('fixture', '', 'fixture')
            def command(*_):
                return [sys.executable, '-c', 'pass'], os.environ.copy(), root / 'logs/fixture.log'
            handlers = {sig: signal.getsignal(sig) for sig in [signal.SIGINT, signal.SIGTERM]}
            try:
                with patch.dict(suite.SHARDS, {'test': (gate,)}, clear=True), \
                     patch.object(suite, 'command', command), \
                     patch.object(suite, 'source_state', side_effect=[{'source_sha': 'before'}, {'source_sha': 'after'}]), \
                     patch.object(sys, 'argv', ['verify_suite.py', '--binary', sys.executable, '--evidence', str(root)]):
                    self.assertEqual(suite.main(), 1)
            finally:
                for sig, handler in handlers.items():
                    signal.signal(sig, handler)
            report = json.loads((root / 'suite-all.json').read_text())
            self.assertEqual(report['source_sha'], 'before')
            self.assertFalse(report['source_stable'])
            self.assertFalse(report['passed'])

    def test_failed_core_run_cannot_leave_previous_success_report(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'core-summary.json').write_text('{"passed":true}')
            gate = suite.Gate('core-failure', 'fixture.py', 'core-failure')
            def command(*_):
                return [sys.executable, '-c', 'raise SystemExit(1)'], os.environ.copy(), root / 'logs/fail.log'
            handlers = {sig: signal.getsignal(sig) for sig in [signal.SIGINT, signal.SIGTERM]}
            try:
                with patch.dict(suite.SHARDS, {'codemode': (gate,)}, clear=True), patch.object(suite, 'command', command), \
                     patch.object(sys, 'argv', ['verify_suite.py', '--binary', sys.executable, '--evidence', str(root)]):
                    self.assertEqual(suite.main(), 1)
            finally:
                for sig, handler in handlers.items():
                    signal.signal(sig, handler)
            self.assertFalse((root / 'core-summary.json').exists())
            self.assertFalse(json.loads((root / 'suite-all.json').read_text())['passed'])

    def test_cancellation_reaps_owned_process_and_runs_finally(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            script = root / 'fixture.py'
            ready, cleaned = root / 'ready', root / 'cleaned'
            script.write_text('from pathlib import Path\nimport time\n'
                              'def scenario(name):\n try:\n'
                              f'  Path({str(ready)!r}).touch()\n  time.sleep(60)\n finally:\n'
                              f'  Path({str(cleaned)!r}).touch()\nscenario("owned-case")\n')
            stopped = threading.Event()
            def interrupt():
                deadline = time.monotonic() + 5
                while not ready.exists() and time.monotonic() < deadline:
                    time.sleep(.01)
                stopped.set()
            thread = threading.Thread(target=interrupt)
            thread.start()
            env = dict(os.environ, ERII_PROFILE_DIR=str(root / 'profile'))
            try:
                code = suite.run_command([sys.executable, 'scripts/profile_harness.py', str(script)],
                                         env, root / 'log', stopped)
            finally:
                thread.join(timeout=6)
            self.assertNotEqual(code, 0)
            self.assertTrue(ready.exists())
            self.assertTrue(cleaned.exists())
            profile = json.loads((root / 'profile/fixture.json').read_text())
            self.assertGreater(profile['elapsed_seconds'], 0)
            self.assertEqual([c['name'] for c in profile['cases']], ['owned-case'])
            self.assertGreaterEqual(profile['cpu_seconds'], 0)

    def test_cancelled_leader_cannot_leave_child_listening(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            ready = root / 'ready.json'
            state = root / 'state'
            listener = ('import json,os,signal,socket,time\nfrom pathlib import Path\n'
                        'signal.signal(signal.SIGTERM, signal.SIG_IGN)\n'
                        's=socket.socket();s.bind(("127.0.0.1",0));s.listen()\n'
                        f'Path({str(ready)!r}).write_text(json.dumps([os.getpid(),s.getsockname()[1]]))\n'
                        'time.sleep(60)\n')
            leader = ('import subprocess,sys,time,shutil\nfrom pathlib import Path\n'
                      f'Path({str(state)!r}).mkdir()\n'
                      f'subprocess.Popen([sys.executable,"-c",{listener!r}])\n'
                      'try: time.sleep(60)\n'
                      f'finally: shutil.rmtree({str(state)!r})\n')
            fixture = root / 'leader.py'
            fixture.write_text(leader)
            stopped = threading.Event()
            def interrupt():
                deadline = time.monotonic() + 5
                while not ready.exists() and time.monotonic() < deadline:
                    time.sleep(.01)
                stopped.set()
            thread = threading.Thread(target=interrupt)
            thread.start()
            metrics = {}
            started = time.monotonic()
            try:
                code = suite.run_command([sys.executable, 'scripts/profile_harness.py', str(fixture)],
                                         dict(os.environ, ERII_PROFILE_DIR=str(root / 'profile')),
                                         root / 'log', stopped, metrics)
                self.assertNotEqual(code, 0)
                self.assertFalse(state.exists())
                self.assertTrue(ready.exists(), (root / 'log').read_text())
                _, port = json.loads(ready.read_text())
                deadline = time.monotonic() + 2
                while time.monotonic() < deadline:
                    with socket.socket() as probe:
                        if probe.connect_ex(('127.0.0.1', port)) != 0:
                            break
                    time.sleep(.01)
                else:
                    self.fail('cancelled group left a live listener')
                self.assertLess(time.monotonic() - started, 15)
                self.assertGreaterEqual(metrics['cpu_seconds'], 0)
                self.assertLess(metrics['cancel_cleanup_seconds'], 15)
                self.assertTrue(metrics['owned_group_drained'])
            finally:
                thread.join(timeout=6)
                # Clean the regression's deliberate orphan even on the old code.
                if ready.exists():
                    pid, _ = json.loads(ready.read_text())
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass

    def test_full_suite_isolates_models_after_other_harnesses(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            trace = root / 'order'
            gates = {'runtime': (suite.Gate('slow', '', 'slow'),),
                     'providers': (suite.Gate('fast', '', 'fast'), suite.Gate('models', '', 'models'))}
            def command(gate, *_):
                source = ('import time\nfrom pathlib import Path\n'
                          f'p=Path({str(trace)!r})\n')
                if gate.name == 'models':
                    source += 'assert set(p.read_text().splitlines()) == {"slow", "fast"}\n'
                else:
                    source += f'time.sleep({.3 if gate.name == "slow" else .01})\n'
                    source += f'with p.open("a") as f: f.write({(gate.name + chr(10))!r})\n'
                return [sys.executable, '-c', source], os.environ.copy(), root / 'logs' / (gate.name + '.log')
            handlers = {sig: signal.getsignal(sig) for sig in [signal.SIGINT, signal.SIGTERM]}
            try:
                with patch.dict(suite.SHARDS, gates, clear=True), patch.object(suite, 'command', command), \
                     patch.object(sys, 'argv', ['verify_suite.py', '--binary', sys.executable, '--evidence', str(root)]):
                    self.assertEqual(suite.main(), 0)
            finally:
                for sig, handler in handlers.items():
                    signal.signal(sig, handler)
            result = json.loads((root / 'suite-all.json').read_text())
            self.assertTrue(result['passed'])
            self.assertEqual([r['gate'] for r in result['shards']['providers']], ['fast', 'models'])


if __name__ == '__main__':
    unittest.main()
