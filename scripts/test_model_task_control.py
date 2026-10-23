import unittest

import model_task_control as model


class TaskControlModelTests(unittest.TestCase):
    def test_exhaustive_safety_and_recovery_suffixes(self):
        report = model.explore()
        self.assertEqual(report["result"], "PASS")
        self.assertTrue(report["exhaustive"])
        self.assertGreater(report["recovery_suffixes"], 0)
        self.assertGreater(report["max_depth"], 3)

    def test_missing_fence_has_shortest_counterexample(self):
        report = model.explore(unsafe=True)
        self.assertEqual(report["result"], "COUNTEREXAMPLE")
        self.assertEqual(report["violation"], "stale business write")
        self.assertEqual(report["trace"], ["claim root0", "commit pause",
                                          "late result owner1 root0 (accept)"])
        self.assertFalse(report["exhaustive"])


if __name__ == "__main__":
    unittest.main()
