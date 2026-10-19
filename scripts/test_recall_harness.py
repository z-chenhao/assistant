"""Recall failures retain structural evidence and still clean owned fixtures."""
import json
from contextlib import closing
from pathlib import Path
import sqlite3
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import verify_recall as recall


class RecallEvidenceTests(unittest.TestCase):
    def test_diagnostics_are_read_only_and_exclude_corpus_and_provider_payloads(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            secret = 'SYNTHETIC_PRIVATE_CONTENT'
            with closing(sqlite3.connect(root / 'memory.db')) as db, db:
                db.execute('CREATE TABLE recall_document(active,vector_json,content,user_id,agent_id)')
                db.execute("INSERT INTO recall_document VALUES(1,NULL,?,'local','erii')", (secret,))
                db.execute("INSERT INTO recall_document VALUES(1,NULL,?,'foreign','other')", (secret,))
            with closing(sqlite3.connect(root / 'state.db')) as db, db:
                db.execute('CREATE TABLE run(kind,status,error_code,prompt,agent_id)')
                db.execute('INSERT INTO run VALUES(?,?,NULL,?,?)', ('worker', 'running', secret, 'erii'))
                db.execute('CREATE TABLE run_recall(hits_json)')
            before = {p: p.read_bytes() for p in root.iterdir()}
            h = SimpleNamespace(env={'ERII_DATA_ROOT': str(root)})
            embeddings = SimpleNamespace(requests=[{'input': secret}], fail=False)
            evidence = recall.failure_state(h, embeddings, 'ordinary', 'drain')
            self.assertEqual(evidence['databases']['memory']['index_groups'], [(1, 1, 2)])
            self.assertEqual(evidence['databases']['memory']['authorized_pending_vectors'], 1)
            self.assertEqual(evidence['databases']['state']['run_groups'], [('worker', 'running', None, 1)])
            self.assertNotIn(secret, json.dumps(evidence))
            self.assertEqual(before, {p: p.read_bytes() for p in root.iterdir()})

    def test_failed_drive_writes_evidence_raises_and_cleans_http_and_roots(self):
        roots, servers = [], []
        class Harness(recall.Harness):
            def __init__(self, binary, root, embeddings):
                super().__init__(binary, root, embeddings)
                roots.append(root)
                servers.append(embeddings)
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'failing-cli'
            binary.write_text('#!/usr/bin/env python3\nimport sys\nsys.stderr.write("synthetic failure body")\nsys.exit(7)\n')
            binary.chmod(0o700)
            evidence = Path(directory) / 'recall.json'
            with patch.object(recall, 'Harness', Harness):
                with self.assertRaisesRegex(RuntimeError, 'synthetic failure body'):
                    recall.run(binary, evidence)
            result = json.loads(evidence.read_text())
            self.assertFalse(result['passed'])
            self.assertEqual(result['failure']['scenario'], 'ordinary')
            self.assertEqual(result['failure']['stage'], 'setup')
            self.assertNotIn('synthetic failure body', evidence.read_text())
            self.assertTrue(roots and all(not root.exists() for root in roots))
            self.assertTrue(all(not server.thread.is_alive() and server.server.fileno() == -1 for server in servers))


if __name__ == '__main__':
    unittest.main()
