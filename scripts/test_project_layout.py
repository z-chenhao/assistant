from pathlib import Path
import tempfile
import subprocess
import unittest

from check_docs import ROOT, broken_links


class ProjectLayoutTests(unittest.TestCase):
    def test_markdown_moves_fail_for_missing_local_destinations(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'docs').mkdir()
            (root / 'docs/guide.md').write_text('guide')
            (root / 'README.md').write_text(
                '[guide](docs/guide.md#heading) [web](https://example.test/missing)\n'
                '```md\n[example](missing-example.md)\n```\n'
                '[broken](docs/moved.md)')
            self.assertEqual(list(broken_links(root)), ['README.md: missing docs/moved.md'])

    def test_node_pin_and_frontend_paths_have_one_consistent_value(self):
        import json
        version = (ROOT / '.node-version').read_text().strip()
        runtime = json.loads((ROOT / 'runtime/javascript/package.json').read_text())
        self.assertEqual(runtime['engines']['node'], version)
        self.assertIn('v' + version, (ROOT / 'install.sh').read_text())
        self.assertIn('v' + version, (ROOT / 'cmd/erii/start.go').read_text())
        self.assertIn('node-version-file: .node-version', (ROOT / '.github/actions/ci-inputs/action.yml').read_text())
        for path in ('Makefile', '.github/actions/ci-inputs/action.yml', 'web/build.mjs'):
            self.assertNotIn('internal/interaction/web/client', (ROOT / path).read_text())

    def test_packaging_stays_separate_from_the_required_pr_gate(self):
        workflow = (ROOT / '.github/workflows/release.yml').read_text()
        self.assertIn('--draft', workflow)
        self.assertIn('--verify-tag', workflow)
        self.assertNotIn('pull_request:', workflow)
        self.assertIn('node-version-file: .node-version', workflow)

    def test_startup_owns_node_compile_cache_and_temporary_files(self):
        from verify_startup import install_environment
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            root = parent / 'owned'
            root.mkdir()
            env = install_environment(root)
            subprocess.run(['node', '-e', "require('node:module').enableCompileCache(); require('node:fs')"],
                           env=env, check=True, capture_output=True)
            subprocess.run(['npm', '--version'], env=env, check=True, capture_output=True)
            self.assertEqual(list(parent.iterdir()), [root], 'Node/npm left cache outside its fixture')
            self.assertEqual(Path(env['TMPDIR']).parent, root)
            self.assertEqual(Path(env['NODE_COMPILE_CACHE']).parent, root)
