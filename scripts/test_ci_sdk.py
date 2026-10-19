"""The SDK provisioner owns only its copy and fails closed before activation."""
import copy
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import ci_sdk as sdk


class SDKTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        folder = Path(self.temp.name)
        self.root, self.original = folder / 'checkout', folder / 'official'
        self.root.mkdir()
        (self.original / 'bin').mkdir(parents=True)
        (self.original / 'src').mkdir()
        (self.original / 'bin/go').write_bytes(b'fixed official Go executable')
        (self.original / 'bin/go').chmod(0o755)
        (self.original / 'src/public.go').write_text('fixed public standard library')
        self.state = dict(source_sha='a'*40, source_dirty=False, source_diff_sha256='b'*64)
        self.environment = folder / 'environment'
        self.pathfile = folder / 'path'
        self.environment.touch()
        self.pathfile.touch()
        self.fail_install = False
        self.bad_target = False
        self.calls = []
        patches = [patch.object(sdk, 'source_state', side_effect=lambda: dict(self.state)),
                   patch.object(sdk.subprocess, 'check_output', side_effect=self.output),
                   patch.object(sdk.subprocess, 'run', side_effect=self.compiler_run),
                   patch.dict(os.environ, dict(GITHUB_ENV=str(self.environment), GITHUB_PATH=str(self.pathfile),
                       SYNTHETIC_CREDENTIAL='must not enter SDK compiler environment'), clear=True)]
        for value in patches:
            value.start()
            self.addCleanup(value.stop)

    def output(self, args, env, **_):
        self.calls.append(args)
        if 'env' in args:
            return json.dumps(dict(GOROOT=str(self.original), GOVERSION='go1.fixed', GOOS='linux', GOARCH='amd64')).encode()
        self.assertNotIn('SYNTHETIC_CREDENTIAL', env)
        self.assertEqual(env['GOPROXY'], 'off')
        self.assertEqual(env['GO111MODULE'], 'off')
        copied = Path(env['GOROOT'])
        values = [dict(Standard=True, ImportPath=name,
                       Target=str(copied / f'pkg/linux_amd64_race/{name}.a')) for name in sdk.PACKAGES]
        values.append(dict(Standard=True, ImportPath='unsafe'))
        if self.bad_target:
            values[0]['Target'] = str(self.original / 'unexpected.a')
        return '\n'.join(json.dumps(value) for value in values)

    def compiler_run(self, args, env, cwd, check):
        self.calls.append(args)
        self.assertTrue(check)
        self.assertNotIn('SYNTHETIC_CREDENTIAL', env)
        self.assertEqual(args[1:3], ['install', '-race'])
        self.assertEqual(set(args[3:]), set(sdk.PACKAGES))
        if self.fail_install:
            raise subprocess.CalledProcessError(7, args)
        for name in sdk.PACKAGES:
            target = Path(env['GOROOT']) / f'pkg/linux_amd64_race/{name}.a'
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('public compiled package: ' + name)
        return subprocess.CompletedProcess(args, 0)

    def test_copy_exports_activation_and_owned_cleanup(self):
        before = sdk.tree_digest(self.original)
        value = sdk.prepare(self.root, host_cache=True)
        self.assertEqual(sdk.doctor(self.root), value)
        self.assertEqual(sdk.signature(self.root), value['sdk_tree_sha256'])
        self.assertEqual(sdk.tree_digest(self.original), before)
        self.assertEqual((self.root / sdk.SDK / 'bin/go').read_bytes(), sdk.WRAPPER)
        self.assertEqual(len(value['exports']), len(sdk.PACKAGES))
        self.assertIn('GOCACHE='+str(self.root/sdk.HOST_CACHE), self.environment.read_text())
        self.assertFalse((self.root / sdk.HOST_CACHE).exists(), 'activation must not occupy the bundle destination')
        (self.root / sdk.HOST_CACHE).mkdir()
        sentinel = self.root / sdk.HOST_CACHE / 'preserve'
        sentinel.write_text('unrelated retained cache')
        sdk.cleanup(self.root)
        self.assertFalse((self.root / sdk.SDK).exists())
        self.assertTrue(sentinel.exists())
        self.assertEqual(sdk.tree_digest(self.original), before)

    def test_occupied_or_linked_destination_preserves_existing_data(self):
        base = self.root / sdk.SDK
        base.mkdir(parents=True)
        (base / 'user').write_text('preserve')
        with self.assertRaises(ValueError): sdk.prepare(self.root)
        self.assertEqual((base / 'user').read_text(), 'preserve')
        base.joinpath('user').unlink()
        base.rmdir()
        base.symlink_to(self.original, target_is_directory=True)
        with self.assertRaises(ValueError): sdk.prepare(self.root)
        self.assertTrue((self.original / 'bin/go').exists())
        self.assertFalse(self.environment.read_text())

    def test_producer_cannot_adopt_an_existing_host_cache(self):
        cache = self.root / sdk.HOST_CACHE
        cache.mkdir(parents=True)
        (cache/'preserve').write_text('previous workflow or user cache')
        with self.assertRaisesRegex(ValueError, 'must start empty'):
            sdk.prepare(self.root, host_cache=True)
        self.assertEqual((cache/'preserve').read_text(), 'previous workflow or user cache')
        self.assertFalse(self.environment.read_text())

    def test_source_sdk_links_are_never_followed(self):
        secret = self.root / 'private'
        secret.write_text('do not copy')
        (self.original / 'src/escape').symlink_to(secret)
        with self.assertRaises(ValueError): sdk.prepare(self.root)
        self.assertFalse((self.root / sdk.SDK).exists())
        self.assertEqual(secret.read_text(), 'do not copy')

    def test_failure_and_bad_public_target_never_activate_or_keep_partial_sdk(self):
        for broken in ['fail_install', 'bad_target']:
            with self.subTest(broken=broken):
                setattr(self, broken, True)
                with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                    sdk.prepare(self.root)
                self.assertFalse((self.root / sdk.SDK).exists())
                self.assertFalse((self.root / sdk.RECEIPT).exists())
                self.assertFalse(self.environment.read_text())
                setattr(self, broken, False)

    def test_dirty_or_drifting_source_never_activates(self):
        self.state['source_dirty'] = True
        with self.assertRaises(ValueError): sdk.prepare(self.root)
        self.assertFalse(self.calls)
        self.state['source_dirty'] = False
        original = copy.deepcopy(self.state)
        with patch.object(sdk, 'source_state', side_effect=[original, dict(original, source_sha='c'*40)]):
            with self.assertRaises(ValueError): sdk.prepare(self.root)
        self.assertFalse((self.root / sdk.SDK).exists())
        self.assertFalse(self.environment.read_text())

    def test_compiler_export_source_and_receipt_drift_fail_closed(self):
        value = sdk.prepare(self.root)
        for relative in ['bin/go', 'bin/go.real', next(iter(value['exports'])), 'src/public.go']:
            path = self.root / sdk.SDK / relative
            before = path.read_bytes()
            path.write_bytes(b'tampered')
            with self.assertRaises(ValueError): sdk.doctor(self.root)
            path.write_bytes(before)
        original = self.original / 'src/public.go'
        original.write_text('SDK source drift')
        with self.assertRaises(ValueError): sdk.doctor(self.root)
        original.write_text('fixed public standard library')
        receipt = self.root / sdk.RECEIPT
        receipt.write_text(json.dumps(dict(value, version=99)))
        with self.assertRaises(ValueError): sdk.doctor(self.root)

    def test_unknown_user_changes_prevent_cleanup(self):
        sdk.prepare(self.root)
        user = self.root / sdk.SDK / 'preserve'
        user.write_text('unrecognized user change')
        with self.assertRaises(ValueError): sdk.cleanup(self.root)
        self.assertEqual(user.read_text(), 'unrecognized user change')

    def test_public_package_inventory_rejects_duplicates_nonstandard_and_traversal(self):
        root = Path('/copied-sdk')
        good = [dict(Standard=True, ImportPath=name, Target=str(root/f'pkg/linux_amd64_race/{name}.a')) for name in sdk.PACKAGES]
        for rows in [good+good[:1], [dict(good[0], Standard=False)]+good[1:],
                     [dict(good[0], ImportPath='../escape')]+good[1:], good[:-1]]:
            with self.assertRaises(ValueError):
                sdk.public_targets('\n'.join(json.dumps(row) for row in rows), root, dict(GOOS='linux',GOARCH='amd64'))

    def test_cli_prepare_honors_cache_activation_and_evidence_flags(self):
        evidence = self.root/'evidence.json'
        with patch.object(sdk, 'ROOT', self.root), patch.object(sys, 'argv',
                ['ci_sdk.py','prepare','--host-cache','--evidence',str(evidence)]), \
                patch.object(sdk.signal,'signal'), \
                contextlib.redirect_stdout(io.StringIO()):
            sdk.main()
        self.assertTrue(evidence.exists())
        self.assertIn('GOCACHE=', self.environment.read_text())
        self.assertEqual(json.loads(evidence.read_text())['source'], self.state)

    def test_cli_run_executes_the_requested_argv_with_validated_sdk_environment(self):
        sdk.prepare(self.root)
        with patch.object(sdk, 'ROOT', self.root), patch.object(sys, 'argv',
                ['ci_sdk.py','run','--','python3','script.py','literal argument']), \
                patch.object(sdk.os,'execvpe',side_effect=RuntimeError('exec handoff')) as execute:
            with self.assertRaisesRegex(RuntimeError,'exec handoff'): sdk.main()
        executable, argv, env = execute.call_args.args
        self.assertEqual(executable,'python3')
        self.assertEqual(argv,['python3','script.py','literal argument'])
        self.assertEqual(env['GOROOT'],str(self.root/sdk.SDK))
        self.assertEqual(env['GOCACHE'],str(self.root/sdk.HOST_CACHE))
        self.assertTrue(env['PATH'].startswith(str(self.root/sdk.SDK/'bin')+os.pathsep))


if __name__ == '__main__':
    unittest.main()
