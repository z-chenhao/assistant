import json
import os
from pathlib import Path
import sqlite3
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import diagnose_gc19
from diagnose_gc19 import ROOT, overlay_source
from verify_capabilities import CapabilityHarness


class DiagnosticTests(unittest.TestCase):
    def test_compilation_sdk_does_not_leak_into_fixture_execution(self):
        with tempfile.TemporaryDirectory() as temp:
            environment = dict(os.environ, GOROOT='/fixture/public-sdk',
                               ERII_CI_BASE_GOROOT='/host/compiler-sdk', PATH='/fixture/public-sdk/bin:/bin')
            calls = []
            def execute(command, **options):
                calls.append((command, options['env'].copy()))
                if command[0] == 'go':
                    Path(command[command.index('-o') + 1]).write_bytes(b'fixture-binary')
                else:
                    self.assertEqual(json.loads((Path(temp) / 'diagnostic.json').read_text())['diagnostic_status'], 'running')
                return subprocess.CompletedProcess(command, 0)
            with patch.dict(os.environ, environment, clear=True), \
                 patch('sys.argv', ['diagnose_gc19.py', '--evidence', temp]), \
                 patch.object(diagnose_gc19.subprocess, 'check_output', return_value='a' * 40), \
                 patch.object(diagnose_gc19.subprocess, 'run', side_effect=execute):
                self.assertEqual(diagnose_gc19.main(), 0)
            self.assertEqual(len(calls), 2)
            self.assertEqual(calls[0][1]['GOROOT'], '/host/compiler-sdk')
            self.assertEqual(calls[1][1]['GOROOT'], '/fixture/public-sdk')
            self.assertEqual(calls[1][1]['PATH'], environment['PATH'])
            self.assertEqual(calls[1][1]['ERII_VERIFY_CASE_JOBS'], '4')
            self.assertEqual(calls[1][0][-6:], ['--shard', 'codemode', '--codemode-part', 'GC-19', '--jobs', '1'])

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
