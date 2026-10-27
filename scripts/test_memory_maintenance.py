"""Maintenance drives preserve failures, approvals and SQLite handle cleanup."""
import unittest
from unittest.mock import Mock, patch

import verify_memory_maintenance as maintenance
from verify_capabilities import call


class MaintenanceHarnessTests(unittest.TestCase):
    def test_delegated_drain_failure_is_not_retried_or_approved(self):
        h = Mock()
        h.send.return_value = {'run_id': 'main'}
        drain = Mock(side_effect=RuntimeError('original storage failure'))
        with self.assertRaisesRegex(RuntimeError, 'original storage failure'):
            call(h, 'cli-fix', 'builtin__workspace__edit', {}, worker=True, drain=drain)
        h.send.assert_called_once()
        drain.assert_called_once_with(h, 'main')
        h.command.assert_not_called()
        h.state.assert_not_called()

    def test_ordered_first_drive_keeps_exact_approval_and_resume(self):
        h = Mock()
        h.send.return_value = {'run_id': 'main'}
        h.command.side_effect = [[{'status': 'pending', 'approval_id': 'approval',
                                  'frozen_arguments_hash': 'frozen-hash'}], {}, {}]
        invocation = {'status': 'succeeded'}
        h.state.side_effect = [{'tools': [{'status': 'waiting', 'attempt_count': 0}]},
                               {'tools': [invocation]}]
        drain = Mock()
        self.assertIs(call(h, 'cli-fix', 'builtin__workspace__edit', {}, worker=True,
                           drain=drain), invocation)
        drain.assert_called_once_with(h, 'main')
        self.assertEqual([item.args for item in h.command.call_args_list],
                         [('approval', 'list'),
                          ('approval', 'approve', 'approval', 'frozen-hash'),
                          ('chat', 'work')])

    def test_closes_each_database_even_when_evidence_read_fails(self):
        class Connection:
            def __init__(self, path):
                self.name = path.name
                self.closed = False

            def execute(self, _):
                if self.name == 'memory.db':
                    raise RuntimeError('injected read failure')
                return []

            def close(self):
                self.closed = True

        opened = []

        def connect(path):
            conn = Connection(path)
            opened.append(conn)
            return conn

        class Harness:
            env = {'ERII_DATA_ROOT': '/disposable-fixture'}

        with patch.object(maintenance.sqlite3, 'connect', connect):
            with self.assertRaisesRegex(RuntimeError, 'injected read failure'):
                maintenance.readback(Harness())
        self.assertEqual([conn.name for conn in opened], ['state.db', 'memory.db'])
        self.assertTrue(all(conn.closed for conn in opened), 'observer leaked a SQLite snapshot')
