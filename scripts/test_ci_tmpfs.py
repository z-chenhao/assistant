import json
import os
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import ci_tmpfs as fixtures

DOCTOR = fixtures.doctor
COMMAND = fixtures.command
INSPECT = fixtures.mount_info


class FixtureStorageTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.parent = Path(self.directory.name).resolve()
        self.receipt = self.parent / 'evidence.json'
        self.environment = self.parent / 'environment'
        self.calls = []
        self.mounted = None
        for item in (patch.dict(os.environ, RUNNER_TEMP=str(self.parent),
                                GITHUB_ENV=str(self.environment), TMPDIR=str(self.parent)),
                     patch.object(fixtures.sys, 'platform', 'linux'),
                     patch.object(fixtures, 'RECEIPT', self.receipt),
                     patch.object(fixtures, 'command', side_effect=self.command),
                     patch.object(fixtures, 'mount_info', side_effect=lambda root: self.mounted),
                     patch.object(fixtures, 'doctor', return_value={'sqlite_wal': True, 'execute': True})):
            item.start()
            self.addCleanup(item.stop)

    def command(self, *args):
        self.calls.append(args)
        if args[2] == 'mount':
            self.mounted = dict(target=args[-1], source=args[-2], fstype='tmpfs',
                                options='rw,nosuid,nodev,mode=700')
        elif args[2] == 'umount':
            self.mounted = None
        else:
            self.fail('unexpected privileged command')
        return ''

    def test_setup_and_cleanup_restore_environment_and_remove_owned_mount(self):
        fixtures.setup()
        root = fixtures.root_path()
        self.assertTrue(root.is_dir())
        self.assertIn('size=2147483648', self.calls[0][6])
        self.assertEqual(self.environment.read_text().splitlines()[-1], 'TMPDIR=' + str(root))
        self.assertFalse(json.loads(self.receipt.read_text())['cleanup'])
        fixtures.cleanup()
        self.assertFalse(root.exists())
        self.assertIsNone(self.mounted)
        self.assertTrue(json.loads(self.receipt.read_text())['cleanup'])
        self.assertEqual(self.environment.read_text().splitlines()[-1], 'TMPDIR=' + str(self.parent))
        fixtures.cleanup()  # Idempotent after an already completed owned cleanup.

    def test_setup_never_mounts_over_an_existing_directory(self):
        root = fixtures.root_path()
        root.mkdir()
        marker = root / 'user-state'
        marker.write_text('keep')
        with self.assertRaises(FileExistsError):
            fixtures.setup()
        self.assertEqual(marker.read_text(), 'keep')
        self.assertEqual(self.calls, [])

    def test_unrecorded_or_changed_mount_is_never_unmounted(self):
        root = fixtures.root_path()
        root.mkdir()
        with self.assertRaisesRegex(ValueError, 'unrecorded'):
            fixtures.cleanup()
        root.rmdir()
        fixtures.setup()
        self.mounted['source'] = 'someone-elses-mount'
        with self.assertRaisesRegex(ValueError, 'ownership'):
            fixtures.cleanup()
        self.assertEqual(len(self.calls), 1)
        self.assertTrue(root.exists())

    def test_leftover_fixture_state_is_cleaned_but_never_reported_successful(self):
        fixtures.setup()
        root = fixtures.root_path()
        (root / 'leaked-state').write_text('fixture')
        # Emulate the kernel removing mount contents on successful unmount.
        def unmount(*args):
            (root / 'leaked-state').unlink()
            return self.command(*args)
        with patch.object(fixtures, 'command', side_effect=unmount):
            with self.assertRaisesRegex(ValueError, 'left disposable state'):
                fixtures.cleanup()
        receipt = json.loads(self.receipt.read_text())
        self.assertFalse(receipt['cleanup'])
        self.assertEqual(receipt['remaining_entries'], ['leaked-state'])
        self.assertFalse(root.exists())

    def test_mount_failure_removes_only_the_exclusive_empty_backing_directory(self):
        with patch.object(fixtures, 'command', side_effect=subprocess.CalledProcessError(1, 'mount')):
            with self.assertRaises(subprocess.CalledProcessError):
                fixtures.setup()
        self.assertFalse(fixtures.root_path().exists())
        self.assertTrue(json.loads(self.receipt.read_text())['cleanup'])

    def test_failed_doctor_unmounts_its_own_mount_and_keeps_setup_failure(self):
        with patch.object(fixtures, 'doctor', side_effect=ValueError('WAL probe failed')):
            with self.assertRaisesRegex(ValueError, 'WAL probe failed'):
                fixtures.setup()
        self.assertFalse(fixtures.root_path().exists())
        self.assertIsNone(self.mounted)
        self.assertTrue(json.loads(self.receipt.read_text())['cleanup'])

    def test_busy_mount_fails_without_force_or_lazy_unmount(self):
        fixtures.setup()
        with patch.object(fixtures, 'command', side_effect=subprocess.CalledProcessError(1, 'umount')) as run:
            with self.assertRaises(subprocess.CalledProcessError):
                fixtures.cleanup()
        self.assertEqual(run.call_args.args, ('sudo', '-n', 'umount', str(fixtures.root_path())))
        self.assertFalse(json.loads(self.receipt.read_text())['cleanup'])

    def test_disappeared_verified_mount_never_reports_success(self):
        fixtures.setup()
        self.mounted = None
        with self.assertRaisesRegex(ValueError, 'disappeared'):
            fixtures.cleanup()
        self.assertFalse(json.loads(self.receipt.read_text())['cleanup'])

    def test_symlinked_root_and_receipt_path_drift_reject_cleanup(self):
        for kind in ('symlink', 'receipt'):
            with self.subTest(kind=kind):
                if self.receipt.exists():
                    self.receipt.unlink()
                root = fixtures.root_path()
                if root.exists():
                    root.rmdir()
                fixtures.setup()
                if kind == 'symlink':
                    root.rmdir()
                    root.symlink_to(self.parent, target_is_directory=True)
                else:
                    receipt = json.loads(self.receipt.read_text())
                    receipt['root'] = str(self.parent)
                    self.receipt.write_text(json.dumps(receipt))
                before = len(self.calls)
                with self.assertRaisesRegex(ValueError, 'path changed'):
                    fixtures.cleanup()
                self.assertEqual(len(self.calls), before)
                if root.is_symlink():
                    root.unlink()

    def test_unsafe_environment_and_non_linux_reject_before_mount(self):
        with patch.dict(os.environ, RUNNER_TEMP=str(self.parent) + '\nother'):
            with self.assertRaises(ValueError):
                fixtures.setup()
        with patch.object(fixtures.sys, 'platform', 'darwin'):
            with self.assertRaisesRegex(ValueError, 'requires Linux'):
                fixtures.setup()
        self.assertEqual(self.calls, [])

    def test_doctor_executes_and_reopens_real_wal_then_cleans_probe_files(self):
        fixtures.setup()
        root = fixtures.root_path()
        receipt = json.loads(self.receipt.read_text())
        receipt['backing'][0] = -1  # Simulate crossing the kernel mount boundary.
        capacity = SimpleNamespace(f_blocks=fixtures.LIMIT // 4096, f_frsize=4096)
        with patch.object(fixtures.os, 'statvfs', return_value=capacity), \
             patch.object(fixtures, 'command', side_effect=COMMAND):
            result = DOCTOR(root, receipt)
        self.assertTrue(result['sqlite_wal'])
        self.assertTrue(result['execute'])
        self.assertTrue(result['fsync'])
        self.assertEqual(list(root.iterdir()), [])
        fixtures.cleanup()

    def test_doctor_rejects_missing_mount_and_unsafe_properties(self):
        fixtures.setup()
        root = fixtures.root_path()
        receipt = json.loads(self.receipt.read_text())
        with self.assertRaisesRegex(ValueError, 'properties'):
            DOCTOR(root, receipt)  # A plain directory cannot pass as tmpfs.
        for options in ('rw,nosuid,nodev,noexec', 'rw,nosuid', 'rw,nodev'):
            with self.subTest(options=options):
                self.mounted['options'] = options
                with self.assertRaisesRegex(ValueError, 'properties'):
                    DOCTOR(root, receipt)
        self.mounted = None
        with self.assertRaisesRegex(ValueError, 'missing'):
            DOCTOR(root, receipt)

    def test_findmnt_does_not_treat_command_failure_or_ambiguous_mount_as_absent(self):
        root = fixtures.root_path()
        for result in (SimpleNamespace(returncode=2, stdout=''),
                       SimpleNamespace(returncode=1, stdout='partial'),
                       SimpleNamespace(returncode=0, stdout=json.dumps({'filesystems': []})),
                       SimpleNamespace(returncode=0, stdout=json.dumps({'filesystems': [{'target': '/other'}]}))):
            with patch.object(fixtures.subprocess, 'run', return_value=result):
                with self.assertRaises(ValueError):
                    INSPECT(root)
        with patch.object(fixtures.subprocess, 'run', return_value=SimpleNamespace(returncode=1, stdout='')):
            self.assertIsNone(INSPECT(root))

    def test_disk_conformance_records_real_device_and_rejects_ram_or_unknown_storage(self):
        for filesystem, source in (('ext4', '/dev/root'), ('tmpfs', 'tmpfs'),
                                   ('overlay', 'overlay'), ('ext4', 'unknown')):
            with self.subTest(filesystem=filesystem, source=source):
                response = json.dumps({'filesystems': [dict(target='/', source=source,
                                                            fstype=filesystem, options='rw')]})
                with patch.object(fixtures, 'command', return_value=response):
                    if filesystem == 'ext4' and source == '/dev/root':
                        fixtures.disk()
                        receipt = json.loads(self.receipt.read_text())
                        self.assertEqual(receipt['storage'], 'disk')
                        self.assertFalse(receipt['managed_mount'])
                        with self.assertRaisesRegex(ValueError, 'receipt already exists'):
                            fixtures.disk()
                    else:
                        with self.assertRaisesRegex(ValueError, 'ordinary block filesystem'):
                            fixtures.disk()
