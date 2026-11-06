"""Check partition completeness and the fail-closed required status."""
import unittest

import ci
from runner import CORE


class CIChecksTests(unittest.TestCase):
    def test_connected_apps_and_browser_regressions_are_required_software_lanes(self):
        lanes = {row['lane']: row for row in ci.plan('software')['include']}
        self.assertEqual(ci.command('connected-apps'), 'make test-connected-apps')
        self.assertIn('connected-apps', lanes)
        self.assertIn('make test-browser', ci.command('browser-infra'))
        self.assertEqual(lanes['browser-infra']['browser'], 'true')
        self.assertEqual(lanes['browser-infra']['frontend'], 'true')

    def test_all_core_gates_run_once(self):
        gates = [gate for group in ci.SYSTEM.values() for gate in group]
        self.assertCountEqual(gates + ['channels'], CORE)
        self.assertEqual(len(gates), len(set(gates)))
        for kind in ('software', 'system'):
            for lane in ci.plan(kind)['include']:
                self.assertTrue(ci.command(lane['lane']))
        lanes = {lane['lane'] for lane in ci.plan('system')['include']}
        self.assertEqual({name.removeprefix('channels-') for name in lanes if name.startswith('channels-')}, set(ci.CHANNEL_GROUPS))

    def test_intelligent_ui_uses_required_go_browser_and_race_checks(self):
        self.assertIn('test-go', ci.command('go'))
        self.assertIn('intelligent-ui', ci.SYSTEM['startup-ui'])
        self.assertIn('ui-interactions', ci.SYSTEM['startup-ui'])
        lanes = {row['lane']: row for row in ci.plan('system')['include']}
        self.assertEqual(lanes['startup-ui']['browser'], 'true')
        race = ci.command('capabilities-race')
        for package in ('agent', 'agent/sqlite', 'interaction/web', 'llm/bridge'):
            self.assertIn('./internal/' + package + ' ', ci.RACE)
        self.assertIn(ci.RACE, race)
        self.assertIn('.*UI.*', ci.RACE)
        self.assertIn('ReplyStream.*', ci.RACE)

    def test_memory_privacy_race_and_system_gates_are_required(self):
        lanes = {lane['lane']: lane for lane in ci.plan('system')['include']}
        self.assertIn('memory-context', lanes)
        self.assertEqual(lanes['memory-context']['browser'], 'false')
        command = ci.command('memory-context')
        self.assertIn('make test-memory build', command)
        self.assertEqual(ci.MEMORY_GATES, ('memory-maintenance', 'recall'))
        for gate in ci.MEMORY_GATES:
            self.assertEqual(command.count('--gate ' + gate), 1)
        self.assertNotIn('|| true', command)
        self.assertNotIn('continue-on-error', command)

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
