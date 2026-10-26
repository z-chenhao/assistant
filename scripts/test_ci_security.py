import copy
import hashlib
import json
from pathlib import Path
import signal
import sys
import tempfile
import unittest
from unittest.mock import patch

import ci_security
import verify_suite as suite
from test_gate_scheduling import GateSchedulingTests


class SecurityTests(unittest.TestCase):
    def fixture(self, root):
        suites = {}
        for part in suite.SECURITY_PARTS:
            (root / part).mkdir()
            suites[part] = dict(passed=True, source_stable=True, source_dirty=False,
                source_sha='a'*40, source_diff_sha256=hashlib.sha256(b'').hexdigest(),
                binary_sha256='b'*64, workflow_sha=None, run_id=None, run_attempt=None,
                security_part=part, selected_gates=None, complete_inventory=False,
                jobs=1, gate_jobs=2, elapsed_seconds=10, shards={'security': [
                    dict(gate=g.name, attempt=n, returncode=0, owned_group_drained=True,
                         command=suite.command(g, n, Path('/binary'), root / part)[0])
                    for g in suite.security_gates(part) for n in range(1, g.attempts+1)]})
        return suites

    def write(self, root, suites):
        for part, fragment in suites.items():
            (root / part / f'suite-security-{part}.json').write_text(json.dumps(fragment))

    def test_original_and_consumer_inventory_join_without_rewriting_any_gate(self):
        for consumer in (False, True):
            gates = suite.SHARDS['security']
            if consumer:
                gates = (*gates, suite.Gate('gc04', 'verify_gc04.py', 'gc04'))
            with self.subTest(consumer=consumer), patch.dict(suite.SHARDS, security=gates), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                suites = self.fixture(root)
                self.write(root, suites)
                source = {k: suites['cases'][k] for k in ('source_sha', 'source_dirty', 'source_diff_sha256')}
                with patch.object(ci_security, 'source_state', return_value=source), patch.dict(ci_security.os.environ, {}, clear=True):
                    full = ci_security.join(root)
                self.assertEqual([(r['gate'], r['attempt']) for r in full['shards']['security']],
                                 [(g.name, n) for g in gates for n in range(1, g.attempts+1)])
                self.assertEqual(len(full['shards']['security']), 23 if consumer else 22)
                self.assertTrue(full['complete_inventory'])
                self.assertIsNone(full['security_part'])
                self.assertIsNone(full['semantic_score'])
                self.assertFalse(full['release_ready'])
                self.assertEqual(full['parts'], suites)
                selected = [g for p in suite.SECURITY_PARTS for g in suite.security_gates(p)]
                self.assertCountEqual(selected, gates)
                self.assertTrue(all(any(g is original for original in gates) for g in selected))
                GateSchedulingTests().drive('security', gates=suite.security_gates('cases'), first_pair=('gc03','gc05'))
                GateSchedulingTests().drive('security', gates=suite.security_gates('protection'))
        with patch.dict(suite.SHARDS, security=(*gates, suite.Gate('new', '', 'new'))):
            with self.assertRaises(ValueError): suite.security_gates('cases')
        with self.assertRaises(ValueError): suite.security_gates('unknown')

    def test_no_invalid_receipt_can_be_promoted_to_complete(self):
        mutations = ('missing_part', 'extra_part', 'missing_round', 'duplicate', 'reorder', 'gate_order',
            'failure', 'cancelled', 'unclean', 'failed_suite', 'dirty', 'unstable', 'focused', 'missing_focus',
            'wrong_part', 'wrong_shard', 'complete', 'other_part', 'jobs', 'lanes', 'bool_attempt', 'bool_code',
            'command', 'argument', 'report', *ci_security.SECURITY_IDENTITY)
        for change in mutations:
            with self.subTest(change=change), tempfile.TemporaryDirectory() as temp:
                suites = self.fixture(Path(temp))
                fragment = suites['protection']
                rows = fragment['shards']['security']
                if change == 'missing_part': suites.pop('cases')
                elif change == 'extra_part': suites['extra'] = copy.deepcopy(fragment)
                elif change == 'missing_round': rows.pop()
                elif change == 'duplicate': rows.append(copy.deepcopy(rows[-1]))
                elif change == 'reorder': rows[0], rows[1] = rows[1], rows[0]
                elif change == 'gate_order': rows[:] = rows[5:] + rows[:5]
                elif change == 'failure': rows[-1]['returncode'] = 7
                elif change == 'cancelled': rows[-1]['returncode'] = 130
                elif change == 'unclean': rows[-1]['owned_group_drained'] = False
                elif change == 'failed_suite': fragment['passed'] = False
                elif change == 'dirty': fragment['source_dirty'] = True
                elif change == 'unstable': fragment['source_stable'] = False
                elif change == 'focused': fragment['selected_gates'] = ['tools']
                elif change == 'missing_focus': fragment.pop('selected_gates')
                elif change == 'wrong_part': fragment['security_part'] = 'cases'
                elif change == 'wrong_shard': fragment['shards'] = {'runtime': rows}
                elif change == 'complete': fragment['complete_inventory'] = True
                elif change == 'other_part': fragment['codemode_part'] = 'control'
                elif change == 'jobs': fragment['jobs'] = 2
                elif change == 'lanes': fragment['gate_jobs'] = 4
                elif change == 'bool_attempt': rows[0]['attempt'] = True
                elif change == 'bool_code': rows[0]['returncode'] = False
                elif change == 'command': rows[0]['command'][2] = 'scripts/other.py'
                elif change == 'argument': rows[0]['command'].append('--feedback')
                elif change == 'report': rows[0]['command'][6] = '/wrong.json'
                else: fragment[change] = 'drift'
                with self.assertRaises(ValueError): ci_security.validate(suites)
        with tempfile.TemporaryDirectory() as temp:
            suites = self.fixture(Path(temp))
            for fragment in suites.values(): fragment['binary_sha256'] = ''
            with self.assertRaises(ValueError): ci_security.validate(suites)

    def test_current_checkout_and_workflow_run_are_authoritative(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            suites = self.fixture(root)
            self.write(root, suites)
            for field, value in [('source_sha', 'c'*40), ('workflow_sha','c'*40), ('run_id','2'), ('run_attempt','2')]:
                with self.subTest(field=field):
                    with self.assertRaises(ValueError):
                        ci_security.validate(suites, {field: value} if field == 'source_sha' else None,
                                             {field: value} if field != 'source_sha' else None)
            (root / 'extra').mkdir()
            with self.assertRaises(ValueError): ci_security.join(root)

    def test_part_cli_keeps_canonical_verdict_and_other_part_untouched(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'suite-security.json').write_text('canonical sentinel')
            (root / 'suite-security-cases.json').write_text('cases sentinel')
            handlers = {sig: signal.getsignal(sig) for sig in (signal.SIGINT,signal.SIGTERM)}
            try:
                def command(gate, attempt, *_):
                    return [sys.executable, '-c', 'pass'], {}, root / 'logs' / f'{gate.name}-{attempt}.log'
                with patch.object(suite, 'command', side_effect=command), patch.object(sys, 'argv',
                        ['verify_suite.py','--shard','security','--security-part','protection',
                         '--jobs','1','--gate-jobs','2','--binary',sys.executable,'--evidence',str(root)]):
                    self.assertEqual(suite.main(), 0)
                fragment = json.loads((root / 'suite-security-protection.json').read_text())
                self.assertFalse(fragment['complete_inventory'])
                self.assertEqual(fragment['security_part'], 'protection')
                self.assertEqual(len(fragment['shards']['security']),20)
                self.assertEqual((root / 'suite-security.json').read_text(), 'canonical sentinel')
                self.assertEqual((root / 'suite-security-cases.json').read_text(), 'cases sentinel')
            finally:
                for sig, handler in handlers.items(): signal.signal(sig,handler)
        for flags in (['--list'], ['--gate','tools'], ['--shard','runtime']):
            with patch.object(sys,'argv',['verify_suite.py','--security-part','cases',*flags]), self.assertRaises(SystemExit):
                suite.main()

    def test_failed_join_removes_stale_canonical_report(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            output = root / 'suite-security.json'
            output.write_text('stale PASS')
            with patch.object(sys,'argv',['ci_security.py','--parts',str(root/'missing'),'--evidence',str(root)]):
                with self.assertRaises(OSError): ci_security.main()
            self.assertFalse(output.exists())


if __name__ == '__main__':
    unittest.main()
