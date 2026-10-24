"""Evidence durability, immutable identities, failure and recovery contracts."""
import copy
from datetime import datetime, timedelta, timezone
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import evidence as ev


class EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name).resolve()
        self.source, self.root = self.base / 'worktree', self.base / 'durable'
        self.source.mkdir()
        self.ident = ev.identity('ERII-206', 'owner/repo', 'a' * 40, '42', 1)
        self.record = self.base / 'command.json'
        ev.save(self.record, dict(command=['make', 'verify'], returncode=1, source_sha='a' * 40))
        (self.source / 'result.json').write_text('{"passed":false}')

    def collect(self, **kwargs):
        return ev.collect(self.root, self.source, self.ident, [self.record], 'explicit-fake-v1', **kwargs)

    def metadata(self):
        archive = self.base / 'remote.zip'
        with zipfile.ZipFile(archive, 'w') as output:
            output.writestr('suite-runtime.json', '{"passed":true}')
        self.payload = archive.read_bytes()
        item = dict(id=7, name='evidence-runtime-' + 'a' * 40 + '-1',
                    digest='sha256:' + hashlib.sha256(self.payload).hexdigest(),
                    size_in_bytes=len(self.payload), expired=False,
                    expires_at=(datetime.now(timezone.utc) + timedelta(days=1)).isoformat(),
                    workflow_run=dict(id=42, head_sha='a' * 40))
        return dict(repository='owner/repo', run=42, attempt=1, source_sha='a' * 40, artifacts=[item])

    def download(self, *_args, stdout, **kwargs):
        stdout.write(self.payload)

    def test_full_raw_retained_after_worktree_cleanup_and_failure_not_hidden(self):
        manifest = self.collect()
        self.assertEqual(ev.verify(manifest)['commands'][0]['returncode'], 1)
        (self.source / 'result.json').unlink()
        self.source.rmdir()
        self.assertEqual(ev.verify(manifest)['files'][0]['size'], 16)
        receipt = self.base / 'receipt.json'
        self.assertLess(ev.receipt(manifest, receipt)['size'], ev.BUDGET)
        self.assertEqual(ev.receipt(manifest, receipt)['transferred_bytes'], 0)

    def test_repeat_frozen_content_and_interrupted_copy_resume(self):
        original = ev.copy_verified
        def crash(source, dest, row):
            original(source, dest, row)
            raise InterruptedError('after file commit before manifest')
        with patch.object(ev, 'copy_verified', side_effect=crash):
            with self.assertRaises(InterruptedError): self.collect()
        manifest = self.collect()
        digest = ev.sha256_file(manifest)
        self.assertEqual(self.collect(), manifest)
        self.assertEqual(ev.sha256_file(manifest), digest)
        (self.source / 'result.json').write_text('changed')
        with self.assertRaisesRegex(ValueError, 'frozen'): self.collect()

    def test_manifest_hash_paths_and_wrong_identity_rejected(self):
        manifest = self.collect()
        with self.assertRaisesRegex(ValueError, 'identity'): ev.verify(manifest, dict(self.ident, attempt=2))
        raw = manifest.parent / 'raw/result.json'
        raw.chmod(0o644)
        raw.write_text('corrupt')
        with self.assertRaisesRegex(ValueError, 'hash mismatch'): ev.verify(manifest)
        value = json.loads(manifest.read_text())
        value['files'][0]['path'] = '../outside'
        ev.save(manifest, value)
        with self.assertRaisesRegex(ValueError, 'unsafe'): ev.verify(manifest)

    def test_exclusions_explicit_required_archive_export_once_and_no_symlink(self):
        (self.source / 'node_modules').mkdir()
        (self.source / 'node_modules/huge').write_bytes(b'x' * 1000)
        (self.source / 'old.zip').write_bytes(b'old archive')
        (self.source / 'required.zip').write_bytes(b'mandatory original bytes')
        manifest = self.collect(includes=['required.zip'])
        value = ev.verify(manifest)
        self.assertEqual(value['excluded'], ['node_modules', 'old.zip'])
        self.assertIn('required.zip', [r['path'] for r in value['files']])
        exported = ev.export(manifest, 'required release consumer')
        self.assertEqual(ev.export(manifest, 'required release consumer'), exported)
        with zipfile.ZipFile(manifest.parent / 'export/evidence.zip') as archive:
            self.assertEqual(archive.read('raw/required.zip'), b'mandatory original bytes')
            self.assertIn('raw/result.json', archive.namelist())
        with self.assertRaises(ValueError): ev.export(manifest, 'different purpose')
        (self.source / 'link').symlink_to(self.record)
        with self.assertRaisesRegex(ValueError, 'symlink'): self.collect()

    def test_ci_cold_warm_missing_expired_and_attempt_drift(self):
        metadata = self.metadata()
        with patch.object(ev, 'read', side_effect=self.download) as download:
            path, cold = ev.artifact_archive(self.root, metadata, 7)
            _, warm = ev.artifact_archive(self.root, metadata, 7)
            self.assertEqual(download.call_count, 1)
            self.assertEqual(cold['downloaded_bytes'], len(self.payload))
            self.assertEqual(warm['downloaded_bytes'], 0)
            self.assertEqual(path.read_bytes(), self.payload)
            for bad in [dict(metadata, attempt=2), dict(metadata, artifacts=[])]:
                with self.assertRaises(ValueError): ev.artifact_archive(self.root, bad, 7)
            # Expired remote remains usable only with the verified immutable local archive.
            metadata['artifacts'][0]['expired'] = True
            self.assertTrue(ev.artifact_archive(self.root, metadata, 7)[1]['cache_hit'])
            path.unlink()
            with self.assertRaises((ValueError, FileNotFoundError)): ev.artifact_archive(self.root, metadata, 7)
        with patch.object(ev, 'read') as download:
            with self.assertRaisesRegex(ValueError, 'expired'):
                ev.artifact_archive(self.base / 'cold', metadata, 7)
            download.assert_not_called()

    def test_ci_corruption_missing_digest_budget_and_crash_before_receipt(self):
        metadata = self.metadata()
        with patch.object(ev, 'read', side_effect=self.download) as download:
            with self.assertRaisesRegex(ValueError, 'budget'):
                ev.artifact_archive(self.root, metadata, 7, max_bytes=1)
            download.assert_not_called()
            original_save = ev.save
            def crash_receipt(path, value):
                if path.name == 'receipt.json':
                    raise InterruptedError()
                original_save(path, value)
            with patch.object(ev, 'save', side_effect=crash_receipt):
                with self.assertRaises(InterruptedError): ev.artifact_archive(self.root, metadata, 7)
            path, recovered = ev.artifact_archive(self.root, metadata, 7)
            self.assertEqual(download.call_count, 1)
            self.assertEqual(recovered['downloaded_bytes'], 0)
            path.write_bytes(b'corrupted')
            with self.assertRaisesRegex(ValueError, 'hash mismatch'): ev.artifact_archive(self.root, metadata, 7)
        for digest in ['', 'sha256:' + 'b' * 64]:
            bad = copy.deepcopy(metadata)
            bad['artifacts'][0]['digest'] = digest
            with patch.object(ev, 'read', side_effect=self.download):
                with self.assertRaises(ValueError): ev.artifact_archive(self.base / ('bad' + digest[:2]), bad, 7)

    def test_interrupted_download_has_no_committed_cache_and_next_call_recovers(self):
        metadata = self.metadata()
        def crash(*args, stdout, **kwargs):
            stdout.write(b'partial')
            raise InterruptedError()
        with patch.object(ev, 'read', side_effect=crash):
            with self.assertRaises(InterruptedError): ev.artifact_archive(self.root, metadata, 7)
        self.assertFalse(list(self.root.rglob('receipt.json')))
        with patch.object(ev, 'read', side_effect=self.download):
            path, result = ev.artifact_archive(self.root, metadata, 7)
        self.assertEqual(path.read_bytes(), self.payload)
        self.assertEqual(result['downloaded_bytes'], len(self.payload))

    def test_inventory_filters_attempt_source_and_validates_run(self):
        metadata = self.metadata()
        item = metadata['artifacts'][0]
        stale = dict(item, id=8, name=item['name'][:-1] + '2')
        with patch.object(ev, 'gh', side_effect=[dict(id=42, run_attempt=1, head_sha='a' * 40),
                                               [dict(artifacts=[item, stale])]]):
            self.assertEqual(ev.inventory('owner/repo', 42, 1)['artifacts'], [item])
        with patch.object(ev, 'gh', return_value=dict(id=42, run_attempt=2)):
            with self.assertRaises(ValueError): ev.inventory('owner/repo', 42, 1)

    def test_concurrent_readers_download_one_immutable_object(self):
        from concurrent.futures import ThreadPoolExecutor
        metadata = self.metadata()
        with patch.object(ev, 'read', side_effect=self.download) as download:
            with ThreadPoolExecutor(max_workers=4) as pool:
                results = list(pool.map(lambda _: ev.artifact_archive(self.root, metadata, 7), range(4)))
        self.assertEqual(download.call_count, 1)
        self.assertEqual(sorted(r['downloaded_bytes'] for _, r in results), [0, 0, 0, len(self.payload)])

    def test_export_receipt_crash_reuses_complete_archive_without_repackaging(self):
        manifest = self.collect()
        original_save = ev.save
        def crash(path, value):
            if path.name == 'receipt.json':
                raise InterruptedError()
            original_save(path, value)
        with patch.object(ev, 'save', side_effect=crash):
            with self.assertRaises(InterruptedError): ev.export(manifest, 'required final consumer')
        archive = manifest.parent / 'export/evidence.zip'
        before = archive.stat().st_mtime_ns, ev.sha256_file(archive)
        result = ev.export(manifest, 'required final consumer')
        self.assertEqual((archive.stat().st_mtime_ns, result['archive']['sha256']), before)


class ExistingCollectorTests(unittest.TestCase):
    def test_comparison_collects_only_canonical_reports_and_reuses_immutable_archive(self):
        import ci_compare as compare
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            archive = base / 'artifact.zip'
            expected = {'suite-' + shard + '.json': {'passed': True} for shard in compare.SHARDS}
            expected.update({'core-summary.json': {'passed': True}, 'm6-summary.json': {'passed': True}})
            with zipfile.ZipFile(archive, 'w') as output:
                for name, report in expected.items():
                    output.writestr(name, json.dumps(report))
                output.writestr('irrelevant.log', 'complete raw log stays in archive')
            sha = 'a' * 40
            payload = archive.read_bytes()
            item = dict(size_in_bytes=len(payload), expired=False,
                        digest='sha256:' + hashlib.sha256(payload).hexdigest(),
                        workflow_run=dict(id=42, head_sha=sha))
            artifacts = [dict(item, id=i, name=f'evidence-{shard}-{sha}-1')
                         for i, shard in enumerate(compare.SHARDS, 1)]
            meta = dict(repository='owner/repo', run=42, attempt=1, source_sha=sha,
                        artifacts=[*artifacts, dict(item, id=98, name=f'evidence-fast-{sha}-1'),
                                   dict(item, id=99, name=f'evidence-codemode-GC-17-{sha}-1')])
            run = dict(id=42, run_attempt=1, head_sha=sha, path='.github/workflows/ci.yml',
                       status='completed', conclusion='success')
            jobs = [dict(name=name, run_id=42, run_attempt=1, head_sha=sha, conclusion='success',
                         started_at='2026-10-19T00:00:00Z', completed_at='2026-10-19T00:00:01Z')
                    for name in compare.topology()]
            def download(*_args, stdout, **_kwargs): stdout.write(payload)
            with patch.dict('os.environ', ERII_EVIDENCE_ROOT=str(base / 'durable')), \
                 patch.object(compare.profile, 'gh', side_effect=[run, [dict(jobs=jobs)]] * 2), \
                 patch.object(compare, 'inventory', return_value=meta), \
                 patch.object(ev, 'read', side_effect=download) as fetch:
                cold = compare.collect('owner/repo', '42:1')
                warm = compare.collect('owner/repo', '42:1')
            self.assertEqual(fetch.call_count, len(compare.SHARDS))
            self.assertEqual(cold['reports'], expected)
            self.assertEqual(cold['reports'], warm['reports'])
            self.assertEqual(sum(t['downloaded_bytes'] for t in cold['transfers']), len(payload)*len(compare.SHARDS))
            self.assertTrue(all(t['downloaded_bytes'] == 0 for t in warm['transfers']))

class ProcessRecoveryTests(unittest.TestCase):
    def test_sigkill_partial_download_then_foreground_resume(self):
        import os
        import signal
        import subprocess
        import sys
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            payload = b'real subprocess archive payload'
            sha = 'a' * 40
            meta = base / 'meta.json'
            ev.save(meta, dict(repository='owner/repo', run=42, attempt=1, source_sha=sha,
                              artifacts=[dict(id=7, name=f'evidence-runtime-{sha}-1', expired=False,
                                              size_in_bytes=len(payload), workflow_run=dict(id=42, head_sha=sha),
                                              digest='sha256:' + hashlib.sha256(payload).hexdigest())]))
            executable = base / 'gh'
            executable.write_text('#!' + sys.executable + '\n' + '''import os, signal, sys
if os.environ.get('EVIDENCE_KILL') == '1':
    sys.stdout.buffer.write(b'partial'); sys.stdout.buffer.flush()
    os.kill(os.getppid(), signal.SIGKILL)
else:
    sys.stdout.buffer.write(b'real subprocess archive payload')
''')
            executable.chmod(0o755)
            env = dict(os.environ, PATH=str(base) + os.pathsep + os.environ['PATH'], EVIDENCE_KILL='1')
            root = base / 'durable'
            command = [sys.executable, str(Path(ev.__file__).resolve()), 'ci-fetch', '--root', str(root),
                       '--metadata', str(meta), '--id', '7']
            killed = subprocess.run(command, env=env, capture_output=True)
            self.assertEqual(killed.returncode, -signal.SIGKILL)
            self.assertEqual([p.read_bytes() for p in root.rglob('*.partial')], [b'partial'])
            self.assertFalse(list(root.rglob('receipt.json')))
            env['EVIDENCE_KILL'] = '0'
            resumed = subprocess.run(command, env=env, capture_output=True, text=True, check=True)
            self.assertEqual(json.loads(resumed.stdout)['downloaded_bytes'], len(payload))
            warmed = subprocess.run(command, env=env, capture_output=True, text=True, check=True)
            self.assertEqual(json.loads(warmed.stdout)['downloaded_bytes'], 0)

    def test_root_requires_absolute_external_location(self):
        for root in (Path('evidence'), Path(ev.__file__).resolve().parents[1] / 'evidence'):
            with self.assertRaises(ValueError): ev.durable_root(root)


if __name__ == '__main__':
    unittest.main()
