"""Selection is a drive convenience, never a replacement for complete evidence."""
import argparse
import contextlib
import io
import json
import os
import re
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import verify_suite as suite


class SelectionTests(unittest.TestCase):
    def test_catalogue_is_the_execution_inventory_without_a_binary(self):
        result = subprocess.run([sys.executable, 'scripts/verify_suite.py', '--list',
                                 '--binary', '/missing/binary'], cwd=suite.ROOT,
                                capture_output=True, text=True, check=True)
        rows = json.loads(result.stdout)
        expected = [(name, gate.name, gate.attempts)
                    for name, gates in suite.SHARDS.items() for gate in gates]
        self.assertEqual([(r['shard'], r['gate'], r['attempts']) for r in rows], expected)
        self.assertEqual(len({r['gate'] for r in rows}), len(rows))
        guide = (suite.ROOT / 'docs/guides/verification.md').read_text()
        for row in rows:
            self.assertIn(row['gate'], guide, 'a capability is missing from the central guide')

    def test_selection_rejects_unknown_duplicate_and_wrong_shard(self):
        parser = argparse.ArgumentParser()
        for shard, names in [(None, ['missing']), (None, ['main', 'main']),
                             ('security', ['main'])]:
            with self.subTest(shard=shard, names=names), contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as error:
                    suite.select_gates(parser, shard, names)
                self.assertEqual(error.exception.code, 2)
        plan = suite.select_gates(parser, None, ['artifacts', 'main'])
        self.assertEqual({n: [g.name for g in gs] for n, gs in plan.items()},
                         {'runtime': ['main'], 'security': ['artifacts']})
        self.assertEqual(plan['security'][0].attempts, 5)

    def test_focused_rounds_keep_counts_stop_on_failure_and_preserve_full_evidence(self):
        for failure in (False, True):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                canonical = root / 'suite-security.json'
                canonical.write_text('previous complete evidence')
                gate = suite.Gate('fixture', '', 'fixture', 5)
                def command(_gate, attempt, _binary, evidence):
                    return [sys.executable, '-c', 'raise SystemExit(9)' if failure and attempt == 3 else 'pass'], os.environ.copy(), evidence / 'logs/fixture.log'
                handlers = {s: signal.getsignal(s) for s in (signal.SIGINT, signal.SIGTERM)}
                try:
                    with patch.dict(suite.SHARDS, {'security': (gate,)}, clear=True), \
                         patch.object(suite, 'command', command), \
                         patch.object(sys, 'argv', ['verify_suite.py', '--gate', 'fixture',
                                                   '--binary', sys.executable, '--evidence', str(root)]):
                        self.assertEqual(suite.main(), 1 if failure else 0)
                finally:
                    for sig, handler in handlers.items():
                        signal.signal(sig, handler)
                report_path, = root.glob('focused/*/suite-focused.json')
                report = json.loads(report_path.read_text())
                self.assertEqual([r['attempt'] for r in report['shards']['security']],
                                 [1, 2, 3] if failure else [1, 2, 3, 4, 5])
                self.assertEqual(report['passed'], not failure)
                self.assertFalse(report['complete_inventory'])
                self.assertEqual(report['selected_gates'], ['fixture'])
                self.assertEqual(canonical.read_text(), 'previous complete evidence')
                self.assertTrue(all(r['owned_group_drained'] for r in report['shards']['security']))

    def test_ci_parts_cannot_mix_with_focused_selection(self):
        for part in ('--codemode-part', '--personal-part'):
            with patch.object(sys, 'argv', ['verify_suite.py', '--gate', 'main', part, 'control' if part == '--codemode-part' else 'context']), \
                 contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as error:
                    suite.main()
                self.assertEqual(error.exception.code, 2)

    def test_ci_installs_only_consumed_browser_and_frontend_dependencies(self):
        workflow = (suite.ROOT / '.github/workflows/ci.yml').read_text()
        action = (suite.ROOT / '.github/actions/ci-inputs/action.yml').read_text()
        install = re.search(r'    - name: Install pinned dependencies\n      shell: bash\n      run: \|\n(.*?)(?=    - )',
                            action, re.S)[1]
        install = '\n'.join(line[8:] for line in install.splitlines())
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            fake = root / 'python3'
            fake.write_text(f'#!{sys.executable}\nimport json,os,sys\n'
                            'open(os.environ["CAPTURE_ARGS"],"w").write(json.dumps(sys.argv[1:]))\n')
            fake.chmod(0o755)
            capture = root / 'args.json'
            for browser in (False, True):
                for frontend in (False, True):
                    command = install.replace('${{ inputs.browser }}', str(browser).lower()).replace(
                        '${{ inputs.frontend }}', str(frontend).lower())
                    subprocess.run(['bash', '-eu', '-c', command], check=True,
                                   env=dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                                            CAPTURE_ARGS=str(capture)))
                    expected = ['scripts/setup.py', '--part', 'packages', '--packages',
                                'provider-bridge', 'runtime/javascript']
                    if browser:
                        expected.append('guest-browser')
                    if frontend:
                        expected.append('web')
                    self.assertEqual(json.loads(capture.read_text()), expected)
        for name in ('fast', 'verify', 'personal', 'race', 'codemode'):
            # Extract the entire job (two-space names, not nested step names).
            block = re.search(r'^  ' + name + r':\n(.*?)(?=^  \w+:|\Z)', workflow, re.M | re.S)[1]
            if name == 'fast':
                self.assertIn('frontend: "true"', block)
                self.assertIn('browser: "true"', block)
            else:
                self.assertNotIn('frontend:', block)
            if name == 'verify':
                self.assertIn("browser: ${{ matrix.shard == 'delivery' }}", block)
            elif name == 'personal':
                self.assertIn("browser: ${{ matrix.m6part != 'context' && matrix.m6part != 'memory' }}", block)
            elif name != 'fast':
                self.assertNotIn('browser:', block)


if __name__ == '__main__':
    unittest.main()
