"""The observer must release SQLite handles on success and failed readback."""
import unittest
from unittest.mock import patch

import verify_memory_maintenance as maintenance


class ObserverTests(unittest.TestCase):
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
            env = {'COLLIE_DATA_ROOT': '/disposable-fixture'}

        with patch.object(maintenance.sqlite3, 'connect', connect):
            with self.assertRaisesRegex(RuntimeError, 'injected read failure'):
                maintenance.readback(Harness())
        self.assertEqual([conn.name for conn in opened], ['state.db', 'memory.db'])
        self.assertTrue(all(conn.closed for conn in opened), 'observer leaked a SQLite snapshot')
