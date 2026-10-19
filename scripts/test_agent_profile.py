import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from verify_main import MainHarness
from verify_capabilities import CapabilityHarness
from verify_codemode import CodeHarness
from run_core_cases import CoreHarness


class AgentProfileTests(unittest.TestCase):
    def test_every_core_entry_profiles_work_and_preserves_other_cli_arguments(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.dict(os.environ, ERII_AGENT_CPU_PROFILE=str(root/'profiles')):
                for kind in (MainHarness, CapabilityHarness, CodeHarness, CoreHarness):
                    harness = kind(root/'agent', root)
                    first = harness.args('chat','work')
                    second = harness.args('-test-main-hold','boundary','chat','work')
                    self.assertIn('-test-cpu-profile', first, kind.__name__)
                    self.assertIn('-test-cpu-profile', second, kind.__name__)
                    self.assertNotEqual(first[first.index('-test-cpu-profile')+1], second[second.index('-test-cpu-profile')+1])
                    self.assertEqual(first[-2:], ['chat','work'])
                    self.assertNotIn('-test-cpu-profile', harness.args('status'))
                    self.assertNotIn('-test-cpu-profile', harness.args('chat','send','key'))
