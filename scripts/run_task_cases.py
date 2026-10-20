#!/usr/bin/env python3
"""M2 due fragments using the same Case/scoring and terminal drive mechanics."""
import argparse
import hashlib
import json
from pathlib import Path
import tempfile
import time

from case_runner import load_contract, score
from run_cases import run_suite, timestamp
from terminal_driver import Terminal
from verify_main import MainHarness, require
from verify_tasks import state, assertions


def drive(binary, folder, case, variant):
    h = MainHarness(binary, folder, fixture="tasks")
    started, tick = timestamp(), time.monotonic()
    try:
        h.setup()
        h.send("context", variant["context"])
        steps = []
        with Terminal(h.args("tui"), h.env, h.root) as t:
            t.expect("State: ready"); t.expect("Erii: idle")
            for event in variant["events"]:
                t.send(event["text"])
                if event.get("wait_for") == "worker_generation":
                    t.expect("Main accepted the work.")
                    h.wait_state(lambda v: any("Worker waiting for cancellation" in e["event_json"] for e in v["trace"]), "Worker generation accepted")
                else:
                    t.expect("Erii: idle")
                current = state(h)
                steps.append({key: current[key] for key in ("tasks", "instructions", "runs")})
            require(t.stop() == 0, "Case terminal did not exit cleanly")
            transcript = t.transcript()
        return {"state": state(h), "steps": steps, "transcript": transcript,
                "started_at": started, "finished_at": timestamp(),
                "wall_time_ms": round((time.monotonic() - tick) * 1000)}
    finally:
        h.cleanup()


def task_result(case, variant, policy, execution):
    value = execution["state"]
    expected_roots = case["expected_roots"]
    cancelled = case.get("expected_cancelled_roots", 0)
    checks = assertions(value, case["expected_tasks"], expected_roots, expected_roots - cancelled)
    checks.update({
        "retained_instructions": len(value["instructions"]) == case["expected_instructions"],
        "all_work_completed": all(r["status"] == "completed" or (r["status"] == "cancelled" and r["error_code"] == case.get("stop_reason")) for r in value["runs"]),
        "cancelled_roots": sum(r["kind"] == "worker" and r["status"] == "cancelled" for r in value["runs"]) == cancelled,
        "only_explicit_task_actions": all(t["tool_name"] in ("builtin__text__echo", "builtin__task__search", "builtin__task__manage") and t["status"] == "succeeded" for t in value["tools"]),
        "no_external_capabilities": all(s["permission"] == "read_only" or (s["name"] == "builtin__task__manage" and s["permission"] == "task_control") or (c["role"] == "main" and s["name"] == "builtin__automation__manage" and s["permission"] == "task_control") or (s["name"] in ("builtin__artifact__save", "builtin__artifact__publish") and s["permission"] == "artifact_write") or (c["role"] == "worker" and c["can_spawn_agents"] and s["name"] == "builtin__agent__run" and s["permission"] == "spawn_agents" and s.get("replay_policy") == "safe") for c in value["capabilities"] for s in json.loads(c["tools_json"])),
        "no_implicit_task_for_exploration": not case.get("synchronous_first") or len(execution["steps"][0]["tasks"]) == 0,
        "continue_new_epoch": case["expected_instructions"] == 0 or (value["tasks"][0]["task_epoch"] == case.get("expected_epoch", 2) and len({r["run_id"] for r in value["runs"] if r["kind"] == "worker"}) == 2),
        "anchor_immutable": all(task["anchor_task"] == next(t["anchor_task"] for step in execution["steps"] for t in step["tasks"] if t["task_id"] == task["task_id"]) for task in value["tasks"]),
        "source_and_invocation_dedup": len(value["task_requests"]) == case["expected_tasks"] + case["expected_instructions"] + case.get("stop_actions", 0) and len(value["task_invocations"]) == case["expected_tasks"] + case["expected_instructions"] + case.get("stop_actions", 0) + int(case.get("duplicate", False)),
    })
    failures = []
    if not checks["no_external_capabilities"]:
        failures.append("unauthorized_external_effect")
    if not checks["durable_report_dedup"] or not checks["source_and_invocation_dedup"]:
        failures.append("replay_duplicate_or_unknown_retry")
    if not checks["runtime_never_system"] or not checks["no_worker_task_controls"]:
        failures.append("secret_or_scope_leak")
    grading = score(policy, checks, failures)
    return {"case_id": case["case_id"], "case_version": case["version"], "variant_id": variant["variant_id"],
            "level": "L0", "seed": variant["seed"], "fixture_digest": variant["fixture_digest"], "clock": variant["clock"],
            "input_events": variant["events"], "started_at": execution["started_at"], "finished_at": execution["finished_at"],
            "status": "fragment_pass" if grading["status"] == "ungraded" else "fragment_fail",
            "full_case": "not_applicable", "not_applicable": case["not_applicable"], "final_score": grading["final_score"],
            "semantic_grading": grading["semantic_grading"], "hard_failure": bool(failures), "hard_failures": failures,
            "assertions": checks, "model_snapshot": value["snapshots"], "capability_snapshot": value["capabilities"],
            "artifact_refs": [], "trace_ref": "evidence.state.trace", "evidence": execution}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args(); args.evidence.unlink(missing_ok=True)
    root = Path(__file__).resolve().parents[1]
    dataset, policy, lock = load_contract(root, "m2-v2")
    binary = args.binary.resolve()
    with tempfile.TemporaryDirectory(prefix="erii-task-cases-") as tmp:
        def driver(case, variant):
            folder = Path(tmp) / (case["case_id"] + "-" + variant["variant_id"])
            folder.mkdir()
            return drive(binary, folder, case, variant)
        report = run_suite(dataset, policy, driver, task_result)
    report["provider"] = "explicit tasks scripted Bridge; no real-model quality evidence"
    report["conformance"] = {"SC-01": "applicable Task/no-Child fragments", "SC-04": "GC-04 controlled pause/new Root fragments", "SC-05": "GC-08 cancellation/new epoch fragments; stale/tree contracts in Go and task-control evidence", "SC-10": "batch/priority contracts in Go and tasks.json", "SC-12": "frozen snapshots/dedup fragments"}
    report["contract_lock"] = lock
    report["binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report["summary"]))
    if report["summary"]["fragment_passes"] != report["summary"]["fragments"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
