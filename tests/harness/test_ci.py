"""Check partition completeness and the fail-closed required status."""
import unittest

import ci
from runner import CORE


class CIChecksTests(unittest.TestCase):
    def test_all_core_gates_run_once(self):
        gates = [gate for group in ci.SYSTEM.values() for gate in group]
        self.assertCountEqual(gates, CORE)
        self.assertEqual(len(gates), len(set(gates)))
        for kind in ('software', 'system'):
            for lane in ci.plan(kind)['include']:
                self.assertTrue(ci.command(lane['lane']))

    def test_required_check_rejects_missing_failed_and_cancelled_work(self):
        for docs in ('true', 'false'):
            selected = 'skipped' if docs == 'true' else 'success'
            valid = dict(scope=dict(result='success', outputs=dict(docs_only=docs)),
                         software=dict(result=selected), system=dict(result=selected))
            self.assertTrue(ci.passed(valid))
            for name in valid:
                with self.subTest(docs=docs, job=name):
                    self.assertFalse(ci.passed({key: value for key, value in valid.items() if key != name}))
                    for status in ('failure', 'cancelled', 'skipped' if selected == 'success' else 'success'):
                        self.assertFalse(ci.passed({**valid, name: dict(result=status)}))
        self.assertFalse(ci.passed(dict(scope=dict(result='success', outputs={}),
                                       software=dict(result='success'), system=dict(result='success'))))
