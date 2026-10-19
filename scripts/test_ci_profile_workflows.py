from pathlib import Path
import re
import unittest
from unittest.mock import patch

import ci_profile_workflows as workflows


class ReplicaTests(unittest.TestCase):
    def test_replicas_preserve_every_phase_command_and_job(self):
        for filename in ('ci.yml','ci-shadow.yml'):
            source=(workflows.ROOT/'.github/workflows'/filename).read_text()
            replica=workflows.replica(source,'cancellation','docs/ci-cancel-probe.md',True)
            phase=lambda text:[line.strip() for line in text.splitlines() if 'scripts/ci_step.py --name' in line]
            self.assertEqual(phase(source),phase(replica))
            self.assertEqual(re.findall(r'^  (fast|race|verify|checks):',source,re.M),
                             re.findall(r'^  (fast|race|verify|checks):',replica,re.M))
            self.assertEqual(replica.count("!contains(github.event.head_commit.message, '[stop-cancel-profile]')"),4)
            self.assertIn('paths: [docs/ci-cancel-probe.md]',replica)
            self.assertNotIn('  pull_request:',replica)

    def test_wrong_branch_cannot_write_profiles(self):
        with patch.object(workflows.subprocess,'check_output',return_value='agent/erii-builder/erii-141\n'), \
             patch.object(workflows,'generate') as generate:
            with self.assertRaisesRegex(ValueError,'diagnostic branch'):workflows.main()
            generate.assert_not_called()
