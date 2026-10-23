from pathlib import Path
import tempfile
import subprocess
import unittest

from check_docs import ROOT, broken_links, unindexed_docs


class ProjectLayoutTests(unittest.TestCase):
    def test_reference_links_check_local_files_and_heading_fragments(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'docs').mkdir()
            (root / 'docs/guide name.md').write_text('# Heading\n')
            (root / 'README.md').write_text('''# Here
[source][file] [heading][ ANCHOR   ref ] [self][] [shortcut] ![image][pic]
[file]: docs/missing.md#gone
[anchor ref]: <docs/guide name.md#gone> "Guide"
[self]: #gone
[shortcut]: missing-shortcut.md
[pic]: missing.png
''')
            self.assertEqual(list(broken_links(root)), [
                'README.md: missing docs/missing.md#gone',
                'README.md: missing anchor docs/guide name.md#gone',
                'README.md: missing anchor #gone',
                'README.md: missing missing-shortcut.md',
                'README.md: missing missing.png'])

    def test_unused_external_and_example_references_are_not_local_links(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'README.md').write_text('''# Here
[web][external] [protocol][] [mail]
[inline](#here) [local][first]
`[code][missing]` ``[code ` span][missing]``
~~~md
[fenced][missing]
[hidden]: missing-hidden.md
~~~
    [indented][missing]
<!-- [comment][missing] -->
[hidden] [unresolved]
[external]: https://example.test/no#missing
[protocol]: //example.test/no#missing
[mail]: mailto:example@example.test
[inline]: missing-inline.md
[first]: #here
[FIRST]: missing-shadowed.md
[missing]: missing-example.md
[unused]: missing-unused.md#gone
''')
            self.assertEqual(list(broken_links(root)), [])

    def test_reference_links_reach_docs_with_relative_and_encoded_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'docs').mkdir()
            (root / 'README.md').write_text('[Guide][guide]\n[guide]: docs/guide%20name.md#heading')
            (root / 'docs/guide name.md').write_text('''# Heading
[home][root] [self][] [stable]
[root]: ../README.md
[self]: /docs/guide%20name.md#heading
[stable]: #explicit
<a id="explicit"></a>
''')
            self.assertEqual(list(broken_links(root)), [])
            self.assertEqual(list(unindexed_docs(root)), [])

    def test_markdown_moves_fail_for_missing_local_destinations(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'docs').mkdir()
            (root / 'docs/guide.md').write_text('# Heading\n')
            (root / 'README.md').write_text(
                '[guide](docs/guide.md#heading) [web](https://example.test/missing)\n'
                '```md\n[example](missing-example.md)\n```\n'
                '[broken](docs/moved.md)')
            self.assertEqual(list(broken_links(root)), ['README.md: missing docs/moved.md'])

    def test_agents_and_self_fragments_fail_for_missing_targets(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'README.md').write_text('# Here\n[bad](#absent) [ok](#here)')
            (root / 'AGENTS.md').write_text('[bad](missing.md) [bad anchor](README.md#gone)')
            self.assertEqual(list(broken_links(root)), [
                'AGENTS.md: missing missing.md', 'AGENTS.md: missing anchor README.md#gone',
                'README.md: missing anchor #absent'])

    def test_heading_slugs_explicit_anchors_and_relative_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'docs').mkdir()
            (root / 'README.md').write_text('[guide](docs/guide%20name.md#repeat-2)')
            (root / 'docs/guide name.md').write_text('''# Repeat
## Repeat
## Repeat-1
## Repeat
## A `Code` &amp; **Title**!
中文 标题
---
<a id="stable"></a><a name='legacy'></a>
[collision](#repeat-1-1) [code](#a-code--title) [unicode](#%E4%B8%AD%E6%96%87-%E6%A0%87%E9%A2%98)
[explicit](#stable) [named](#legacy) [relative](../README.md)
[root](/docs/guide%20name.md#repeat) [space](<guide name.md#stable>)
''')
            self.assertEqual(list(broken_links(root)), [])
            (root / 'README.md').write_text('[bad](docs/guide%20name.md#repeat-3)')
            self.assertEqual(list(broken_links(root)), [
                'README.md: missing anchor docs/guide%20name.md#repeat-3'])

    def test_code_examples_and_external_fragments_are_not_local_links(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'README.md').write_text('''# Here
[web](https://example.test/no#missing) [protocol](//example.test/no#missing)
`[example](inline.md)` ``[backtick ` example](inline2.md)``
~~~md
[example](tilde.md)
~~~
````md
```
[example](long-fence.md)
````
    [example](indented.md)
<!-- [example](comment.md) -->
\\[escaped](escaped.md)
''')
            self.assertEqual(list(broken_links(root)), [])

    def test_injected_runtime_is_outside_repository_documentation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'AGENTS.md').write_text('''[bad](repository-missing.md)
<!-- BEGIN MULTICA-RUNTIME (auto-managed; do not edit) -->
[private runtime resource](references/private.md)
<!-- END MULTICA-RUNTIME -->
''')
            self.assertEqual(list(broken_links(root)), ['AGENTS.md: missing repository-missing.md'])

    def test_documentation_must_be_reachable_through_the_index(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'docs/guides').mkdir(parents=True)
            (root / 'README.md').write_text('[docs](docs/README.md)')
            (root / 'docs/README.md').write_text('[guide](guides/active.md)')
            (root / 'docs/guides/active.md').write_text('[back](../README.md)')
            orphan = root / 'docs/guides/orphan.md'
            orphan.write_text('lost guide')
            self.assertEqual(list(unindexed_docs(root)), [
                'docs/guides/orphan.md: unreachable from README.md; update the documentation index'])
            (root / 'docs/README.md').write_text('[active](guides/active.md) [found](guides/orphan.md)')
            self.assertEqual(list(unindexed_docs(root)), [])

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
