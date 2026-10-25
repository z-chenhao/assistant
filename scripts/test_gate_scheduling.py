"""Complete independent series overlap, while model/event checks stay isolated."""
import json
import os
from pathlib import Path
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch

import verify_suite as suite


class GateSchedulingTests(unittest.TestCase):
    def drive(self, shard, failure=False, gates=None, first_pair=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'logs').mkdir()
            ledger = root / 'ledger.jsonl'
            isolated = {'providers': 'models', 'runtime': 'events'}.get(shard)
            gates = gates or (suite.Gate('first', '', 'first', 5), suite.Gate('second', '', 'second', 5),
                              *(() if isolated is None else (suite.Gate(isolated, '', isolated),)))
            program = ('import json,os,sys,time\n'
                       f'p={str(ledger)!r}\n'
                       'def record(event):\n'
                       ' with open(p,"a") as out: out.write(json.dumps([event,sys.argv[1],int(sys.argv[2]),time.monotonic()])+"\\n")\n'
                       'record("start")\n'
                       'time.sleep(.03)\n'
                       'record("end")\n'
                       f'raise SystemExit(7 if {failure!r} and sys.argv[1]=="first" and sys.argv[2]=="2" else 0)\n')
            def command(gate, attempt, *_):
                return ([sys.executable, '-c', program, gate.name, str(attempt)], os.environ.copy(),
                        root / 'logs' / f'{gate.name}-{attempt}.log')
            stopped = threading.Event()
            with patch.object(suite, 'command', side_effect=command):
                rows = suite.run_shard(shard, Path('/unused'), root, stopped, gates, gate_jobs=2)
            self.assertFalse(stopped.is_set(), 'one shard failure must not cancel unrelated shards')
            self.assertEqual(json.loads((root / f'timings-{shard}.json').read_text()), rows)
            events = [json.loads(line) for line in ledger.read_text().splitlines()]
            if failure:
                self.assertTrue(any(r['returncode'] == 7 for r in rows))
                self.assertFalse(any(e[1] == isolated for e in events))
                self.assertLess(len(rows), sum(g.attempts for g in gates))
            else:
                if first_pair:
                    self.assertEqual({e[1] for e in events[:2]}, set(first_pair))
                    self.assertTrue(all(e[0] == 'start' for e in events[:2]))
                self.assertEqual([(r['gate'], r['attempt']) for r in rows],
                                 [(g.name, n) for g in gates for n in range(1, g.attempts + 1)])
                self.assertTrue(all(r['returncode'] == 0 and r['owned_group_drained'] for r in rows))
                active, peak = 0, 0
                for kind, gate, _, stamp in sorted(events, key=lambda e: e[3]):
                    active += 1 if kind == 'start' else -1
                    peak = max(peak, active)
                    if gate == isolated and kind == 'start':
                        self.assertEqual(active, 1, 'isolated Go checks overlapped another harness')
                        self.assertGreaterEqual(stamp, max(e[3] for e in events if e[0] == 'end' and e[1] != isolated))
                self.assertEqual(peak, 2)
                self.assertEqual(active, 0)
                for gate in gates:
                    series = [e for e in events if e[1] == gate.name]
                    self.assertEqual([(e[0], e[2]) for e in series],
                                     [(kind, n) for n in range(1, gate.attempts + 1) for kind in ('start', 'end')])

    def test_security_long_cases_start_first_with_unchanged_inventory_and_reports(self):
        # GC-04 is on the frozen consumer PR, not main yet. Exercise that
        # inventory without importing its business implementation into this PR.
        gates = suite.SHARDS['security']
        if not any(g.name == 'gc04' for g in gates):
            gates = (*gates, suite.Gate('gc04', 'verify_gc04.py', 'gc04'))
        self.assertEqual([(g.name, g.attempts, g.arguments) for g in gates],
                         [('memory-maintenance', 5, ()), ('recall', 5, ()),
                          ('tools', 5, ()), ('artifacts', 5, ()),
                          ('gc03', 1, ()), ('gc05', 1, ()), ('gc04', 1, ())])
        ordered = suite.gate_execution_order(gates)
        self.assertEqual([g.name for g in ordered],
                         ['gc03', 'gc05', 'gc04', 'memory-maintenance', 'recall', 'tools', 'artifacts'])
        self.assertCountEqual(ordered, gates)
        for gate in gates[-3:]:
            args, _, _ = suite.command(gate, 1, Path('/binary'), Path('/evidence'))
            self.assertEqual(args[2:], ['scripts/' + gate.script, '--binary', '/binary',
                                       '--evidence', '/evidence/' + gate.report + '.json'])
        self.drive('security', gates=gates, first_pair=('gc03', 'gc05'))

    def test_long_provider_series_starts_first_without_changing_any_gate(self):
        gates = (suite.Gate('short', '', 'short', 5), suite.Gate('pi-bridge', '', 'pi', 5),
                 suite.Gate('another', '', 'another', 5))
        ordered = suite.gate_execution_order(gates)
        self.assertEqual([gate.name for gate in ordered], ['pi-bridge', 'short', 'another'])
        self.assertCountEqual(ordered, gates)
        self.assertEqual([gate.name for gate in gates], ['short','pi-bridge','another'])

    def test_runtime_series_and_events(self):
        self.drive('runtime')

    def test_security_series_remain_five_consecutive_runs_at_capacity_two(self):
        self.drive('security')
        self.drive('security', failure=True)

    def test_provider_series_and_models(self):
        self.drive('providers')

    def test_failure_retains_partial_proof_and_does_not_start_isolated_checks(self):
        self.drive('providers', failure=True)


if __name__ == '__main__':
    unittest.main()
