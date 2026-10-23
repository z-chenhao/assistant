import copy
import io
import json
import os
import signal
import shlex
from pathlib import Path
import tarfile
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import ci_bundle as bundle
import ci_impact as impact
import ci_feedback
import ci_race

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
                     'docs/guides/codemode.md', 'provider-bridge/package-lock.json', 'AGENTS.md',
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

    def test_early_delivery_fragment_cannot_replace_full_fault_series(self):
        suite = ci_feedback.suite
        full = next(g for g in suite.SHARDS['delivery'] if g.name == 'delivery')
        early = ci_feedback.feedback_gates('delivery')[0]
        full_args, _, _ = suite.command(full, 1, Path('/binary'), Path('/evidence'))
        early_args, _, _ = suite.command(early, 1, Path('/binary'), Path('/evidence'))
        self.assertEqual(full.attempts, 5)
        self.assertNotIn('--feedback', full_args)
        self.assertIn('--feedback', early_args)
        for gates in suite.SHARDS.values():
            self.assertTrue(all(not g.arguments for g in gates), 'full inventory must never select partial evidence')

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
                            image_os='ubuntu24', image_version='pinned image', sdk_public_sha256='c'*64,
                            workflow_run_id='fixed run', workflow_attempt='1')
        self.mock = patch.object(bundle, 'context', return_value=self.context)
        self.mock.start()
        self.addCleanup(self.mock.stop)
        for name in bundle.PAYLOAD:
            path = self.source / name
            if name != bundle.PAYLOAD[0]:
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
                             ('bin/erii', '/tmp/escape')]:
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

    def test_host_cache_round_trips_and_sdk_or_old_format_drift_fails_closed(self):
        digest = bundle.create(self.source, self.archive)
        self.assertEqual(bundle.restore(self.dest, self.archive, digest)['version'], 2)
        self.assertEqual((self.dest / bundle.PAYLOAD[-1] / 'pinned.json').read_text(), '{"version":"pinned"}')
        old = self.root / 'old.tar.gz'
        with tarfile.open(self.archive) as current, tarfile.open(old, 'w:gz') as archive:
            for entry in current:
                data = current.extractfile(entry).read() if entry.isfile() else None
                if entry.name == bundle.META:
                    metadata = json.loads(data)
                    metadata['version'] = 1
                    data = json.dumps(metadata).encode()
                    entry.size = len(data)
                archive.addfile(entry, io.BytesIO(data) if data is not None else None)
        with self.assertRaisesRegex(ValueError, 'build context mismatch'):
            bundle.restore(self.dest, old, bundle.sha(old))
        # A cache with a link is never a source for the executable build payload.
        (self.source / bundle.PAYLOAD[-1] / 'escape').symlink_to('/outside')
        with self.assertRaises(ValueError): bundle.create(self.source, self.archive)

    def test_go_executable_cache_hard_links_serialize_as_regular_bytes(self):
        cache = self.source / bundle.PAYLOAD[-1] / 'cached-executable'
        os.link(self.source / bundle.PAYLOAD[0], cache)
        digest = bundle.create(self.source, self.archive)
        bundle.restore(self.dest, self.archive, digest)
        self.assertEqual((self.dest / bundle.PAYLOAD[-1] / 'cached-executable').read_bytes(), b'built executable')

    def test_cache_link_and_payload_root_traversal_are_rejected_before_extract(self):
        for name, target in [('.ci-go/host-cache/link', '../sdk'),
                             ('.ci-go/host-cache/../../ci-build.json', None)]:
            with self.subTest(name=name):
                with tarfile.open(self.archive, 'w:gz') as archive:
                    entry = tarfile.TarInfo(name)
                    if target:
                        entry.type, entry.linkname = tarfile.SYMTYPE, target
                    archive.addfile(entry, io.BytesIO())
                with self.assertRaises(ValueError):
                    bundle.restore(self.dest, self.archive, bundle.sha(self.archive))
                self.assertFalse((self.dest / '.ci-go').exists())


class FeedbackSchedulingTests(unittest.TestCase):
    def test_phase_owner_is_the_actions_entry_process(self):
        for filename in ('ci.yml',):
            names = set()
            for line in (ROOT/'.github/workflows'/filename).read_text().splitlines():
                if 'python3 scripts/ci_step.py ' not in line:
                    continue
                command = shlex.split(line.strip().removeprefix('run: '))
                self.assertEqual(command[0], 'exec', 'bash must not intercept the entry PID signal')
                python = command.index('python3')
                if python > 1:
                    self.assertEqual(command[1], 'env')
                    self.assertTrue(all('=' in arg for arg in command[2:python]))
                names.add(command[command.index('--name')+1])
            self.assertTrue({'unit-lint','race','verification'} <= names)

    def test_all_selected_shards_remain_in_early_schedule(self):
        self.assertEqual(ci_feedback.feedback_order(list(ci_feedback.GATES)),
                         ['security','runtime','codemode','providers','delivery','personal','communication'])
        self.assertEqual(ci_feedback.feedback_order(['delivery','runtime']), ['runtime','delivery'])
        self.assertEqual(ci_feedback.feedback_order([]), [])
        for names in (['unknown'], ['security','security']):
            with self.assertRaises(ValueError): ci_feedback.feedback_order(names)
        with patch.dict(ci_feedback.GATES, {'new-shard': set()}):
            with self.assertRaises(ValueError): ci_feedback.feedback_order([])


class RaceInventoryTests(unittest.TestCase):
    def test_all_packages_exactly_once_including_new_packages(self):
        packages = [ci_race.BRIDGE, 'github.com/z-chenhao/erii/internal/agent/sqlite',
                    'github.com/z-chenhao/erii/new/package']
        groups = ci_race.groups(packages)
        self.assertEqual(groups[0], ('bridge', [ci_race.BRIDGE]))
        executed = [p for _, group in groups for p in group]
        self.assertCountEqual(executed, packages)
        self.assertEqual(len(executed), len(set(executed)))

    def test_missing_duplicate_empty_or_foreign_inventory_fails_closed(self):
        for packages in [[], ['github.com/z-chenhao/erii/internal/agent/sqlite'],
                         [ci_race.BRIDGE, ci_race.BRIDGE], [ci_race.BRIDGE, 'foreign/pkg'],
                         [ci_race.BRIDGE, 'github.com/z-chenhao/erii/bad argument']]:
            with self.assertRaises(ValueError):
                ci_race.groups(packages)

    def test_failure_retains_inventory_and_prevents_expensive_remainder(self):
        packages = [ci_race.BRIDGE, 'github.com/z-chenhao/erii/internal/agent/sqlite']
        for first_status in (0, 9):
            with self.subTest(first_status=first_status), tempfile.TemporaryDirectory() as temp:
                previous = Path.cwd()
                handlers = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
                calls = []
                def execute(command, env, output, stopped, metrics):
                    calls.append(command)
                    metrics.update(owned_group_drained=True, cpu_seconds=0.01)
                    if '-list' in command:
                        output.write_text('TestA\nTestB\nTestC\n' + ci_race.SERIAL_SQLITE[0] + '\nok\t' + ci_race.SQLITE + '\t0.01s\n')
                    elif '-run' in command:
                        names = command[command.index('-run') + 1][2:-2].split('|')
                        events = [dict(Action='pass', Package=ci_race.SQLITE, Test=n) for n in names]
                        events.append(dict(Action='pass', Package=ci_race.SQLITE))
                        output.write_text('\n'.join(json.dumps(e) for e in events) + '\n')
                    else:
                        output.write_text('fixture outcome\n')
                    return first_status if len(calls) == 1 else 0
                try:
                    os.chdir(temp)
                    with patch.object(ci_race, 'source_state', return_value={'source_sha': 'frozen'}), \
                         patch.object(ci_race, 'cpu_budget', return_value=4), \
                         patch.object(ci_race.subprocess, 'check_output', return_value='\n'.join(packages)), \
                         patch.object(ci_race, 'run_command', execute):
                        self.assertEqual(ci_race.main(), 0 if first_status == 0 else 1)
                    report = json.loads(Path('evidence/race/inventory.json').read_text())
                    self.assertEqual(report['packages'], packages)
                    self.assertEqual(report['passed'], first_status == 0)
                    self.assertEqual(len(calls), 6 if first_status == 0 else 1)
                    self.assertEqual(report['stages'][0]['returncode'], first_status)
                    for command in calls:
                        self.assertIn('-race', command)
                        self.assertIn('-count=1', command)
                        self.assertIn('-timeout=15m', command)
                finally:
                    os.chdir(previous)
                    for sig, handler in handlers.items():
                        signal.signal(sig, handler)


if __name__ == '__main__':
    unittest.main()
