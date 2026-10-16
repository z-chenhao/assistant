import copy
import json
from pathlib import Path
import tempfile
import unittest

from case_runner import digest, load_contract, score, variants, result


class ContractTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(__file__).resolve().parents[1]
        self.dataset, self.policy, self.lock = load_contract(self.root)

    def test_grader_cannot_override_mechanical_or_hard_failure(self):
        for assertions, failures in [({"required": False}, []),
                                     ({"required": True}, ["secret_or_scope_leak"])]:
            self.assertEqual(score(self.policy, assertions, failures, {"O": 4}, ["O"])["final_score"], 0)

    def test_ungraded_fake_has_no_score_and_real_grader_renormalizes(self):
        self.assertIsNone(score(self.policy, {"required": True}, [])["final_score"])
        self.assertEqual(score(self.policy, {"required": True}, [], {"O": 4, "F": 0}, ["O", "F"])["final_score"], 62.5)
        self.assertEqual(score(self.policy, {"required": True}, [], {"O": 3.2}, ["O"])["status"], "pass")
        self.assertEqual(score(self.policy, {"required": True}, [], {"O": 3.19}, ["O"])["status"], "fail")
        self.assertEqual(score(self.policy, {"required": True}, [], {"O": 3.19999}, ["O"])["status"], "fail")

    def test_missing_or_invalid_grades_never_pass(self):
        for grades, applicable in [({}, []), ({"O": 4}, ["O", "F"]), ({"O": 5}, ["O"]),
                                   ({"O": float("nan")}, ["O"]), ({"O": True}, ["O"])]:
            with self.assertRaises(ValueError):
                score(self.policy, {"required": True}, [], grades, applicable)
        with self.assertRaises(ValueError):
            score(self.policy, {}, [])

    def test_variants_preserve_facts_and_have_distinct_provenance(self):
        for case in self.dataset["cases"]:
            before = copy.deepcopy(case)
            fixtures = list(variants(self.dataset, case))
            self.assertEqual(case, before)
            self.assertEqual(len({v["fixture_digest"] for v in fixtures}), 6)
            for v in fixtures:
                self.assertEqual([e["at"] for e in v["events"]], [e["at"] for e in case["events"]])
                for event, original in zip(v["events"], case["events"]):
                    self.assertIn(event["text"], original["wordings"])
                self.assertIn("陈屿", v["context"])
                self.assertIn("杭州", v["context"])
            self.assertEqual(sum(v["noise_characters"] > 0 for v in fixtures), 2)
        # The normative cases' times differ; do not replace them with a single
        # convenient "today", a 9am meeting, or a different local-life goal.
        self.assertEqual(self.dataset["cases"][1]["world"]["calendar"][0]["time"], "10:00-10:30")
        self.assertEqual(self.dataset["cases"][2]["world"]["weather"]["now_c"], 19)

    def test_weight_and_digest_contract(self):
        self.assertEqual(sum(self.policy["weights"].values()), 100)
        self.assertEqual(digest({"a": 1, "b": 2}), digest({"b": 2, "a": 1}))
        self.assertNotEqual(digest(self.dataset), digest({**self.dataset, "version": 999}))

    def test_changed_fixture_or_policy_is_rejected_before_execution(self):
        with tempfile.TemporaryDirectory(prefix="erii-contract-") as directory:
            root = Path(directory)
            for name in ["evals/fixtures/m1-v2.lock.json", *self.lock["files"]]:
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes((self.root / name).read_bytes())
            target = root / "evals/scoring-v1.json"
            target.write_text(json.dumps({**self.policy, "case_min": 1}))
            with self.assertRaises(ValueError):
                load_contract(root)

    def test_task_fragments_use_same_locked_policy_and_variant_contract(self):
        dataset, policy, lock = load_contract(self.root, "m2-v1")
        self.assertEqual(policy, self.policy)
        self.assertEqual({c["case_id"] for c in dataset["cases"]}, {"GC-02", "GC-04", "GC-08", "GC-18"})
        for case in dataset["cases"]:
            generated = list(variants(dataset, case))
            self.assertEqual(len(generated), 6)
            self.assertEqual(len({v["fixture_digest"] for v in generated}), 6)
        self.assertEqual(lock["files"]["evals/scoring-v1.json"], self.lock["files"]["evals/scoring-v1.json"])

    def test_internal_task_permission_does_not_admit_external_tool(self):
        case = {"case_id": "GC-01", "version": 1, "events": [], "forbidden_text": [], "not_applicable": []}
        variant = {"variant_id": "permission", "seed": 0, "fixture_digest": "fixture", "clock": "2026-10-01", "events": []}
        state = {key: [] for key in ("conversation", "tools", "runs", "generations", "snapshots", "usage", "deliveries")}
        execution = {"state": state, "started_at": "start", "finished_at": "end", "wall_time_ms": 0}
        for name, allowed in [("builtin__task__manage", True), ("mcp__mail__send", False)]:
            state["capabilities"] = [{"snapshot_id": "cap", "can_spawn_agents": False,
                                      "tools_json": json.dumps([{"name": name, "permission": "task_control"}])}]
            observed = result(case, variant, self.policy, execution)
            self.assertEqual(observed["assertions"]["no_external_writes"], allowed)


if __name__ == "__main__":
    unittest.main()
