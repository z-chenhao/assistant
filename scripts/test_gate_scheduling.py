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
    def drive(self, shard, failure=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'logs').mkdir()
            ledger = root / 'ledger.jsonl'
            isolated = {'providers': 'models', 'runtime': 'events'}.get(shard)
            gates = (suite.Gate('first', '', 'first', 5), suite.Gate('second', '', 'second', 5),
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
                for gate in ('first', 'second'):
                    self.assertEqual([e[2] for e in events if e[0] == 'start' and e[1] == gate], list(range(1, 6)))

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
