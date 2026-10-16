#!/usr/bin/env python3
"""Task launch/doctor/drive/evidence/cleanup through the existing CLI and PTY."""
import argparse
import hashlib
import json
from pathlib import Path
import signal
import sqlite3
import tempfile

from terminal_driver import Terminal
from verify_main import MainHarness, require


def state(h):
    value = h.state()
    with sqlite3.connect(Path(h.env["ERII_DATA_ROOT"]) / "state.db") as db:
        db.row_factory = sqlite3.Row
        for key, query in {
            "tasks": "SELECT * FROM task ORDER BY created_at,task_id",
            "instructions": "SELECT * FROM task_instruction ORDER BY sequence",
            "updates": "SELECT * FROM task_update ORDER BY occurred_at,event_id",
            "inbox": "SELECT * FROM main_inbox ORDER BY sequence",
            "reporter_dedup": "SELECT * FROM event_inbox WHERE consumer='task_reporter' ORDER BY event_id",
            "task_requests": "SELECT * FROM task_request ORDER BY invocation_id",
            "task_invocations": "SELECT * FROM task_invocation ORDER BY invocation_id",
            "worker_results": "SELECT * FROM worker_result ORDER BY created_at,exchange_id",
            "task_events": "SELECT event_id,type,payload_json,published_at FROM event_outbox WHERE type LIKE '%task%' ORDER BY occurred_at,event_id",
        }.items():
            value[key] = [dict(row) for row in db.execute(query)]
    return value


def assertions(value, tasks, roots, updates):
    workers = [r for r in value["runs"] if r["kind"] == "worker"]
    replies = [m for m in value["conversation"] if m["role"] == "assistant"]
    requests = [json.loads(g["request_json"]) for g in value["generations"]]
    run_map = {r["run_id"]: r for r in value["runs"]}
    snapshots = {s["snapshot_id"] for s in value["snapshots"]}
    caps = {c["snapshot_id"]: c for c in value["capabilities"]}
    return {
        "task_count": len(value["tasks"]) == tasks,
        "root_count_and_no_child": len(workers) == roots and all(r["task_id"] and r["parent_run_id"] is None for r in workers),
        "one_completion_per_root": len(value["updates"]) == updates and len({u["root_run_id"] for u in value["updates"]}) == updates,
        "durable_report_dedup": len(value["inbox"]) == updates == len(value["reporter_dedup"]),
        "no_worker_task_controls": all(not any(s["name"].startswith("builtin__task__") for s in json.loads(caps[r["capability_snapshot_id"]]["tools_json"])) for r in workers),
        "main_cannot_spawn": all(not c["can_spawn_agents"] for c in caps.values()),
        "runtime_origin_and_named_details": all(json.loads(b["message_json"])["origin"] == "runtime" and set(json.loads(b["message_json"])["details"]) <= {"task_ref", "kind", "artifact_refs"} for b in value["inbox"]),
        "runtime_never_system": all(m["role"] != "system" for req in requests for m in req["messages"] if m.get("origin") == "runtime"),
        "worker_not_in_conversation": all(m["role"] in ("user", "assistant") and m["source"] != "runtime" for m in value["conversation"]),
        "frozen_snapshots_and_schemas": all(r["model_snapshot_id"] in snapshots and r["capability_snapshot_id"] in caps for r in value["runs"]) and all(
            req["model_snapshot"]["snapshot_id"] == run_map[req["run_id"]]["model_snapshot_id"] and all(
                req["model_snapshot"] == other["model_snapshot"] and req.get("tools") == other.get("tools")
                for other in requests if other["run_id"] == req["run_id"]) for req in requests),
        "one_durable_delivery_per_main_reply": len(value["deliveries"]) == len(replies) == len({d["message_id"] for d in value["deliveries"]}),
        "atomic_provider_acceptance": len(value["usage"]) == len(value["generations"]),
        "ordered_semantic_history": [m["sequence"] for m in value["conversation"]] == list(range(1, len(value["conversation"]) + 1)),
        "no_failed_task_state": all(t["status"] in ("active", "paused", "completed", "cancelled") for t in value["tasks"]),
    }


def check(value, tasks=1, roots=1, updates=1):
    checks = assertions(value, tasks, roots, updates)
    require(all(checks.values()), "Task invariants: " + repr([k for k, v in checks.items() if not v]))
    return checks


def run(binary):
    report = {"version": 1, "scope": "ERII-11 Task/Root/Reporter implementation; pause/cancel, Child, Artifact and external delivery remain due later", "checks": [], "crashes": []}
    with tempfile.TemporaryDirectory(prefix="erii-tasks-") as directory:
        root = Path(directory)

        def scenario(name, action):
            folder = root / name
            folder.mkdir()
            h = MainHarness(binary, folder, fixture="tasks")
            try:
                h.setup()
                return action(h)
            finally:
                h.cleanup()

        def terminal(h):
            with Terminal(h.args("tui"), h.env, h.root) as t:
                t.expect("State: ready"); t.expect("You> ")
                t.send("Delegate: Wedding checklist with venue dependencies.")
                t.expect("Main follow-up:"); t.expect("Erii: idle")
                t.send("Continue: Wedding | Use the new date.")
                t.expect("Main follow-up:"); t.expect("Erii: idle")
                t.send("/status"); t.expect("completed")
                require(t.stop() == 0, "terminal did not stop cleanly")
                transcript = t.transcript()
            value = state(h)
            check(value, roots=2, updates=2)
            require(value["tasks"][0]["task_epoch"] == 2 and len(value["instructions"]) == 1, "continue did not append/new epoch")
            return {"transcript": transcript, "state": value}

        report["terminal"] = scenario("terminal", terminal)
        report["checks"].append("PTY -> Main create/search/append -> independent Root -> durable Reporter -> Main reply; new epoch and immutable anchor")

        def responsive(h):
            with Terminal(h.args("-test-main-hold", "before_worker_generation", "tui"), h.env, h.root) as t:
                t.expect("You> ")
                t.send("Delegate: Hold this long report until restart.")
                t.expect_all(['"boundary":"before_worker_generation"', "Main accepted the work."])
                t.send("Urgent synchronous question")
                t.expect("Main synchronous answer.")
                before = state(h)
                require(any(r["kind"] == "worker" and r["status"] == "running" for r in before["runs"]), "Root was not still held")
                t.stop(signal.SIGKILL)
                transcript = t.transcript()
            h.command("chat", "work")
            after = state(h); check(after)
            return {"transcript": transcript, "before_kill": before, "recovered": after}

        report["responsive"] = scenario("responsive", responsive)
        report["checks"].append("Committed Main acceptance and new answer visible on PTY while Root is held; actual kill/restart completes it")

        def ambiguous(h):
            h.send("two", "Delegate many: Travel Kyoto itinerary. | Travel Osaka itinerary.")
            h.command("chat", "work")
            h.send("ambiguous", "Continue: Travel | Change the dates.")
            h.command("chat", "work")
            value = state(h); check(value, tasks=2, roots=2, updates=2)
            require(not value["instructions"] and any("Please clarify" in p.get("text", "") for m in value["conversation"] for p in m["content"]), "ambiguous search changed a Task")
            return value

        report["ambiguous"] = scenario("ambiguous", ambiguous)
        report["checks"].append("Ambiguous Task search preserves both candidates and asks once without appending")

        def failure(h):
            h.send("failure", "Delegate: Fail worker: report unavailable.")
            h.command("chat", "work")
            value = state(h); check(value)
            require(value["tasks"][0]["status"] == "active" and value["updates"][0]["kind"] == "failed", "Root failure failed/completed Task")
            return value

        report["failure"] = scenario("failure", failure)
        report["checks"].append("Failed Root persists one failed update; Task stays active and Main receives it")

        for boundary in ("before_task_commit", "after_task_commit", "before_result_commit", "after_result_commit",
                         "before_report_commit", "after_report_commit", "before_followup_commit", "after_followup_commit"):
            def crash(h):
                receipt = h.send("duplicate", "Duplicate: Recover the wedding checklist.")
                require(receipt == h.send("duplicate", "Duplicate: Recover the wedding checklist."), "duplicate ingress receipt drifted")
                child = h.hold(boundary)
                child.kill(); child.communicate(timeout=10)
                before = state(h)
                h.command("chat", "work")
                after = state(h); check(after)
                require(all(r["status"] == "completed" for r in after["runs"]), "recovery left nonterminal work")
                require(len(after["task_requests"]) == 1 and len(after["task_invocations"]) == 2, "duplicate source/Tool created another Task")
                prior = {g["generation_id"]: g for g in before["generations"] if g["status"] == "completed"}
                require(all(g["attempt_count"] == prior[g["generation_id"]]["attempt_count"] and g["request_hash"] == prior[g["generation_id"]]["request_hash"] for g in after["generations"] if g["generation_id"] in prior), "completed model call was replayed")
                h.command("chat", "work")
                require(state(h) == after, "duplicate/restart drain changed terminal state")
                return {"boundary": boundary, "before_kill": before, "recovered": after}
            report["crashes"].append(scenario(boundary, crash))
        report["checks"].append("Eight SIGKILL boundaries around atomic Task/Root, result, Reporter Inbox and batched Main admission; completed calls reused")
    report["cleanup"] = "owned PTYs/children reaped; disposable databases removed"
    return report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args(); args.evidence.unlink(missing_ok=True)
    binary = args.binary.resolve()
    report = run(binary)
    report["binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"checks": report["checks"], "crash_boundaries": len(report["crashes"]), "evidence": str(args.evidence)}))


if __name__ == "__main__":
    main()
