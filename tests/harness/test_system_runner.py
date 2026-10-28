"""Exercise failure, fresh evidence and source drift through actual child processes."""
import json
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

import runner


class SystemRunnerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        scripts = self.root / 'tests/system'
        scripts.mkdir(parents=True)
        shutil.copy2(runner.ROOT / 'tests/system/entrypoint.py', scripts / 'entrypoint.py')
        self.script = scripts / 'fixture.py'
        self.binary = self.root / 'erii'
        self.binary.touch()
        self.output = self.root / 'evidence'

    def drive(self, gates=('fixture',), drift=False):
        registry = {'fixture': 'fixture.py', 'second': 'second.py'}
        before = {'source': 'one'}
        after = {'source': 'changed'} if drift else before
        argv = ['--binary', str(self.binary), '--evidence', str(self.output)]
        for gate in gates:
            argv += ['--gate', gate]
        with patch.object(runner, 'ROOT', self.root), \
             patch.object(runner, 'REGISTRY', registry), \
             patch.object(runner, 'source_state', side_effect=[before, after]):
            return runner.main(argv)

    def write_success(self):
        self.script.write_text('import json,sys\nfrom pathlib import Path\n'
            'Path(sys.argv[sys.argv.index("--evidence")+1]).write_text(json.dumps({"checks":["ran"]}))\n')

    def test_nonzero_child_stops_the_sequence_and_retains_failure(self):
        self.script.write_text('raise SystemExit(7)\n')
        marker = self.root / 'should-not-run'
        self.script.with_name('second.py').write_text(f'from pathlib import Path\nPath({str(marker)!r}).touch()\n')
        self.assertEqual(self.drive(('fixture', 'second')), 1)
        result = json.loads((self.output / 'summary.json').read_text())
        self.assertEqual(result['results'][0]['returncode'], 7)
        self.assertFalse(result['passed'])
        self.assertFalse(marker.exists())

    def test_zero_exit_without_a_report_cannot_pass(self):
        self.script.write_text('pass\n')
        self.assertEqual(self.drive(), 1)
        self.assertFalse(json.loads((self.output / 'summary.json').read_text())['passed'])

    def test_success_is_invalidated_by_source_drift(self):
        self.write_success()
        self.assertEqual(self.drive(drift=True), 1)
        result = json.loads((self.output / 'summary.json').read_text())
        self.assertEqual(result['results'][0]['returncode'], 0)
        self.assertFalse(result['source_stable'])

    def test_previous_evidence_is_preserved_and_rejected(self):
        self.output.mkdir()
        previous = self.output / 'fixture-1.json'
        previous.write_text('previous failed observation')
        with self.assertRaises(SystemExit):
            self.drive()
        self.assertEqual(previous.read_text(), 'previous failed observation')

    def test_success_requires_the_selected_process_to_execute(self):
        self.write_success()
        self.assertEqual(self.drive(), 0)
        result = json.loads((self.output / 'summary.json').read_text())
        self.assertTrue(result['passed'])
        self.assertTrue(result['results'][0]['owned_group_drained'])
