import unittest

import model_task_control as model


class TaskControlModelTests(unittest.TestCase):
    def test_one_shot_completion_uses_root_outcome_after_child_drains(self):
        # Paired with TestTaskControlModelCompletionUsesRootOutcome in SQLite.
        for root_status in ("completed", "failed"):
            for child_status in ("completed", "failed"):
                for child_first in (False, True):
                    with self.subTest(root=root_status, child=child_status,
                                      child_first=child_first):
                        state = model.State()
                        trace = ["claim root0", "admit child owner1", "claim child"]
                        ends = [f"{root_status} root0", f"{child_status} child"]
                        trace += list(reversed(ends)) if child_first else ends
                        for index, action in enumerate(trace):
                            state, violation = next(
                                (n, v) for step, n, v in model.transitions(state)
                                if step == action)
                            self.assertIsNone(violation)
                            model.invariant(state)
                            if index < len(trace) - 1:
                                self.assertEqual(state.task, "active")
                        expected = "completed" if root_status == "completed" else "active"
                        self.assertEqual(state.task, expected)

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
