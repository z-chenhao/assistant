"""Recall failures retain structural evidence and still clean owned fixtures."""
import json
from contextlib import closing
from pathlib import Path
import sqlite3
import subprocess
import tempfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from urllib.error import HTTPError
from urllib.request import Request, urlopen

import verify_recall as recall
from verify_main import MainHarness
from verify_suite import group_alive


class RecallEvidenceTests(unittest.TestCase):
    def test_successful_recall_drive_requires_barriers_and_never_retries_an_error(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            h = recall.Harness(root/'unused', root, SimpleNamespace(server=SimpleNamespace(server_port=1)))
            for code, message in [(7, 'synthetic CLI rejection'), (0, 'barriers were not exercised')]:
                result = subprocess.CompletedProcess([], code, '', 'synthetic CLI rejection')
                with patch.object(h, 'drive_boundaries', return_value=result) as drive, \
                     patch.object(h, 'command', side_effect=AssertionError('unexpected retry')):
                    with self.assertRaisesRegex(RuntimeError, message):
                        h.drain('main')
                    self.assertEqual(drive.call_count, 1)

    def test_boundary_driver_handles_coalesced_records_in_both_orders_and_preserves_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / 'boundary-cli'
            binary.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys, time
values = [sys.argv[i+1].split('=',1) for i,a in enumerate(sys.argv) if a=='-test-main-barrier']
order = values if os.environ['ORDER']=='forward' else list(reversed(values))
sys.stdout.write(''.join(json.dumps({'boundary':name})+'\\n' for name,_ in order));sys.stdout.flush()
while not all(pathlib.Path(path).exists() for _,path in values):time.sleep(.005)
sys.stderr.write('retained diagnostic')
print(json.dumps({'status':'drained'}))
sys.exit(int(os.environ['EXIT_CODE']))
''')
            binary.chmod(0o700)
            for order, code in [('forward', 0), ('reverse', 7)]:
                work = root / order
                work.mkdir()
                h = MainHarness(binary, work)
                h.env.update(ORDER=order, EXIT_CODE=str(code))
                barriers = {name: work / name for name in ('publish', 'worker')}
                observed = []
                def progress():
                    if len(observed) == 2:
                        for path in barriers.values():
                            path.touch(exist_ok=True)
                try:
                    result = h.drive_boundaries(barriers, observed.append, progress)
                    self.assertEqual(observed, ['publish','worker'] if order == 'forward' else ['worker','publish'])
                    self.assertEqual(result.returncode, code)
                    self.assertEqual(result.stderr, 'retained diagnostic')
                    self.assertIn('"status": "drained"', result.stdout)
                    self.assertTrue(all(not group_alive(c.pid) for c in h.children))
                finally:
                    h.cleanup()
                self.assertTrue(all(c.stdout.closed and c.stderr.closed for c in h.children))

    def test_missing_release_times_out_and_reaps_the_owned_pipe_holding_descendant(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / 'stalled-boundary-cli'
            binary.write_text('''#!/bin/sh
sleep 30 &
echo $! > descendant.pid
printf '%s\\n' '{"boundary":"publish"}'
wait
''')
            binary.chmod(0o700)
            h = MainHarness(binary, root)
            clock = time.monotonic
            started = clock()
            observed = []
            try:
                # Advance past the unchanged twenty-second deadline only after
                # Launch/Doctor. Interpreter startup is not the fault under test.
                with patch('verify_main.time.monotonic', side_effect=lambda: clock() + (21 if observed else 0)):
                    with self.assertRaises(subprocess.TimeoutExpired) as failure:
                        h.drive_boundaries({'publish': root/'never'}, observed.append, lambda: None)
                self.assertEqual(failure.exception.timeout, 20)
                self.assertLess(clock() - started, 3)
                self.assertEqual(observed, ['publish'])
                self.assertTrue((root/'descendant.pid').exists())
                self.assertTrue(all(c.poll() is not None and not group_alive(c.pid) for c in h.children))
            finally:
                h.cleanup()

    def test_embedding_barrier_errors_cannot_masquerade_as_recall_conflicts(self):
        fake = recall.Embeddings()
        def fail(_):
            raise RuntimeError('synthetic barrier failure')
        fake.before_response = fail
        try:
            request = Request(f'http://127.0.0.1:{fake.server.server_port}/v1/embeddings',
                              json.dumps(dict(model='semantic-fixture-v1', input='tea')).encode(),
                              {'Content-Type':'application/json'})
            with self.assertRaises(HTTPError) as failure:
                urlopen(request, timeout=2)
            self.assertEqual(failure.exception.code, 503)
            failure.exception.close()
            self.assertEqual(fake.callback_errors, ['RuntimeError'])
        finally:
            fake.close()

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
