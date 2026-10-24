import json
import os
from pathlib import Path
import sqlite3
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from diagnose_gc19 import ROOT, overlay_source
from verify_capabilities import CapabilityHarness


class DiagnosticTests(unittest.TestCase):
    def test_overlay_preserves_transaction_and_guards_source_drift(self):
        source = (ROOT / 'internal/agent/sqlite/store.go').read_text()
        changed = overlay_source(source)
        self.assertIn('result != nil && os.Getenv("ERII_SQL_FIXTURE_DIAGNOSTIC") == "1"', changed)
        self.assertIn('code = sqliteErr.Code()', changed)
        self.assertIn(source[source.index('\n\ttx, err := s.db.BeginTx(ctx, nil)', source.index('func (s *Store) Transaction')):], changed)
        with self.assertRaisesRegex(ValueError, 'source drift'):
            overlay_source(changed)

    def test_failed_command_captures_wal_before_cleanup_without_retry_or_secrets(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            binary = root / 'erii'
            binary.write_bytes(b'fixture-binary')
            fixture = root / 'fixture'
            data = fixture / 'data'
            data.mkdir(parents=True)
            (data / 'credentials').write_text('DO_NOT_COPY')
            evidence = root / 'evidence'
            h = CapabilityHarness(binary, fixture)
            with sqlite3.connect(data / 'state.db') as db:
                db.execute('PRAGMA journal_mode=WAL')
                db.execute('CREATE TABLE observed(value)')
                db.execute("INSERT INTO observed VALUES('committed')")
                db.commit()
                result = subprocess.CompletedProcess([], 1, '', 'FIXTURE_TX_ERROR code=2067')
                with patch.dict(os.environ, ERII_CAPABILITY_DIAGNOSTICS=str(evidence)), \
                     patch('verify_capabilities.subprocess.run', return_value=result) as command:
                    with self.assertRaisesRegex(RuntimeError, 'FIXTURE_TX_ERROR code=2067'):
                        h.command('chat', 'send', 'test-source', text='DO_NOT_COPY_INPUT')
                    command.assert_called_once()
            folder, = evidence.iterdir()
            record = json.loads((folder / 'command.json').read_text())
            self.assertEqual(record['returncode'], 1)
            self.assertEqual({p.name for p in folder.iterdir()}, {'state.db', 'command.json'})
            with sqlite3.connect((folder / 'state.db').as_uri() + '?immutable=1', uri=True) as backup:
                self.assertEqual(backup.execute('SELECT value FROM observed').fetchone(), ('committed',))
            self.assertNotIn('DO_NOT_COPY', (folder / 'command.json').read_text())


if __name__ == '__main__':
    unittest.main()
