import copy
import io
import json
import os
from pathlib import Path
import tarfile
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import ci_bundle as bundle
import ci_impact as impact
import ci_feedback

ROOT = Path(__file__).resolve().parents[1]


class ImpactTests(unittest.TestCase):
    def setUp(self):
        self.manifest = json.loads((ROOT / impact.MANIFEST).read_text())

    def test_every_mapping_and_its_negative_fixture(self):
        self.assertTrue(impact.valid(self.manifest))
        for rule in self.manifest['rules']:
            with self.subTest(prefix=rule['prefix']):
                self.assertTrue((ROOT / rule['example']).is_file(), 'example drift')
                self.assertEqual(impact.select([rule['example']], self.manifest)['shards'], sorted(rule['shards']))
                negative = 'unmapped/' + rule['example']
                self.assertEqual(impact.select([negative], self.manifest)['shards'], impact.ALL)
                # A lookalike directory/file must not inherit the mapping.
                self.assertFalse(impact.matches(rule['prefix'].rstrip('/') + '-other/file', rule['prefix']))

    def test_critical_unknown_and_deleted_paths_fail_closed(self):
        for path in ['scripts/new.py', '.github/workflows/ci.yml', 'Makefile', 'go.sum',
                     'runtime/javascript/src/sidecar.mjs', 'evals/fixtures/new.json',
                     'docs/technical-design.md', 'docs/security.md', 'docs/replay.md',
                     'internal/agent/sqlite/migrations/001.sql', 'docs/schema.md',
                     'docs/codemode.md', 'provider-bridge/package-lock.json', 'AGENTS.md',
                     'new-module/removed.go', 'docs/../new', '/docs/file.md']:
            with self.subTest(path=path):
                self.assertEqual(impact.select([path], self.manifest)['shards'], impact.ALL)

    def test_manifest_missing_drift_ambiguity_and_empty_fail_closed(self):
        for manifest in [None, {}, [], dict(self.manifest, version=2),
                         dict(self.manifest, rules=[{'prefix': 'docs/'}])]:
            self.assertEqual(impact.select(['docs/development.md'], manifest)['shards'], impact.ALL)
        self.assertEqual(impact.select([], self.manifest)['shards'], impact.ALL)
        self.assertEqual(impact.select(['docs/development.md'], self.manifest, False)['shards'], impact.ALL)
        manifest = copy.deepcopy(self.manifest)
        manifest['rules'].append(dict(prefix='docs/development.md', example='docs/development.md', shards=[]))
        self.assertEqual(impact.select(['docs/development.md'], manifest)['shards'], impact.ALL)
        self.assertEqual(set(ci_feedback.GATES), set(impact.ALL))
        for shard, names in ci_feedback.GATES.items():
            self.assertEqual(names, {g.name for g in ci_feedback.suite.SHARDS[shard] if g.name in names})

    def test_real_git_base_trust_drift_deleted_and_unknown_paths(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null')
            def git(*args):
                return subprocess.check_output(['git', '-c', 'core.hooksPath=/dev/null',
                    '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test', *args],
                    cwd=root, env=env, stderr=subprocess.DEVNULL).decode().strip()
            git('init', '-q', '-b', 'main')
            (root / 'scripts').mkdir()
            (root / impact.MANIFEST).write_text(json.dumps(self.manifest))
            (root / 'README.md').write_text('fixture')
            git('add', '.')
            git('commit', '-qm', 'base')
            base = git('rev-parse', 'HEAD')
            (root / 'README.md').unlink()
            git('add', '.')
            git('commit', '-qm', 'delete known documentation')
            result = impact.plan(root, base)
            self.assertEqual(result['paths'], ['README.md'])
            self.assertEqual(result['shards'], [])
            self.assertEqual(result['full_shards'], impact.ALL)
            (root / 'unknown.go').write_text('new input')
            git('add', '.')
            git('commit', '-qm', 'unknown path')
            self.assertEqual(impact.plan(root, base)['shards'], impact.ALL)
            (root / 'unknown.go').unlink()
            (root / impact.MANIFEST).write_text('{}')
            git('add', '.')
            git('commit', '-qm', 'manifest drift')
            self.assertIn('manifest', impact.plan(root, base)['reason'])
            self.assertEqual(impact.plan(root, base)['shards'], impact.ALL)
            self.assertEqual(impact.plan(root, '0' * 40)['shards'], impact.ALL)


class BundleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source, self.dest = self.root / 'producer', self.root / 'consumer'
        self.source.mkdir()
        self.dest.mkdir()
        self.archive = self.root / 'bundle.tar.gz'
        self.context = dict(source_sha='a' * 40, source_inputs_sha256='b' * 64,
                            os='Linux', arch='x86_64', go='go pinned', node='node pinned',
                            image_os='ubuntu24', image_version='pinned image')
        self.mock = patch.object(bundle, 'context', return_value=self.context)
        self.mock.start()
        self.addCleanup(self.mock.stop)
        for name in bundle.PAYLOAD:
            path = self.source / name
            if 'node_modules' in name:
                path.mkdir(parents=True)
                (path / 'pinned.json').write_text('{"version":"pinned"}')
            else:
                path.parent.mkdir(parents=True)
                path.write_bytes(b'built executable')
                path.chmod(0o755)
        binary_link = self.source / bundle.PAYLOAD[1] / '.bin/tool'
        binary_link.parent.mkdir()
        binary_link.symlink_to('../pinned.json')

    def test_matching_build_round_trips_including_hidden_files_and_symlinks(self):
        digest = bundle.create(self.source, self.archive)
        result = bundle.restore(self.dest, self.archive, digest)
        self.assertEqual(result['context'], self.context)
        self.assertEqual((self.dest / bundle.PAYLOAD[0]).read_bytes(), b'built executable')
        self.assertEqual((self.dest / bundle.PAYLOAD[0]).stat().st_mode & 0o777, 0o755)
        self.assertTrue((self.dest / bundle.PAYLOAD[1] / '.bin/tool').is_symlink())

    def test_missing_corrupt_or_untrusted_digest_fails_closed(self):
        digest = bundle.create(self.source, self.archive)
        for value in ['', 'f' * 64, digest.upper()]:
            with self.assertRaises(ValueError):
                bundle.restore(self.dest, self.archive, value)
        self.archive.write_bytes(self.archive.read_bytes() + b'polluted')
        with self.assertRaises(ValueError):
            bundle.restore(self.dest, self.archive, digest)
        with self.assertRaises(OSError):
            bundle.restore(self.dest, self.root / 'missing', digest)

    def test_each_source_platform_and_toolchain_binding_is_required(self):
        digest = bundle.create(self.source, self.archive)
        original = dict(self.context)
        for key in original:
            with self.subTest(key=key):
                self.context[key] = 'different'
                with self.assertRaises(ValueError):
                    bundle.restore(self.dest, self.archive, digest)
                self.context[key] = original[key]
        self.assertFalse((self.dest / bundle.PAYLOAD[0]).exists())

    def test_traversal_and_symlink_escape_fail_even_with_matching_digest(self):
        for name, target in [('../escape', None), ('provider-bridge/node_modules/escape', '/tmp/escape'),
                             ('bin/personal-agent', '/tmp/escape')]:
            with self.subTest(name=name):
                with tarfile.open(self.archive, 'w:gz') as archive:
                    entry = tarfile.TarInfo(name)
                    if target:
                        entry.type = tarfile.SYMTYPE
                        entry.linkname = target
                    archive.addfile(entry, io.BytesIO())
                with self.assertRaises((ValueError, tarfile.FilterError)):
                    bundle.restore(self.dest, self.archive, bundle.sha(self.archive))

    def test_existing_destination_is_never_overwritten(self):
        digest = bundle.create(self.source, self.archive)
        path = self.dest / bundle.PAYLOAD[0]
        path.parent.mkdir()
        path.write_text('user state')
        with self.assertRaises(ValueError):
            bundle.restore(self.dest, self.archive, digest)
        self.assertEqual(path.read_text(), 'user state')


if __name__ == '__main__':
    unittest.main()
