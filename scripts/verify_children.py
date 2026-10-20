#!/usr/bin/env python3
"""Direct delegation launch/doctor/drive/evidence/cleanup using the maintained CLI harness."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import tempfile

from fixture_pool import run_cases

from terminal_driver import Terminal
from verify_main import require
from verify_artifacts import ArtifactHarness
from verify_tasks import state as task_state


def state(h):
    value = task_state(h)
    with sqlite3.connect(Path(h.env["ERII_DATA_ROOT"]) / "state.db") as db:
        db.row_factory = sqlite3.Row
        for name, query in {
            "child_links": "SELECT * FROM agent_invocation ORDER BY invocation_id",
            "trees": "SELECT * FROM run_tree ORDER BY root_run_id",
            "claims": "SELECT * FROM run_claim ORDER BY run_id",
            "approvals": "SELECT * FROM approval ORDER BY created_at,approval_id",
            "artifacts": "SELECT * FROM artifact ORDER BY created_at",
            "artifact_pins": "SELECT * FROM artifact_reference ORDER BY artifact_ref,holder_kind,holder_id",
            "publications": "SELECT * FROM artifact_publication ORDER BY run_id,artifact_ref",
            "child_events": "SELECT * FROM event_outbox WHERE type LIKE '%run_ready%' ORDER BY occurred_at,event_id",
        }.items():
            value[name] = [dict(row) for row in db.execute(query)]
    return value


def check(value, children=2, completed=True):
    workers = [r for r in value["runs"] if r["kind"] == "worker"]
    nodes = {r["run_id"]: r for r in value["runs"]}
    caps = {c["snapshot_id"]: c for c in value["capabilities"]}
    child_nodes = [r for r in workers if r["parent_run_id"]]
    require(len(value["tasks"]) == 1 and len(workers) == children + 1, "Task delta or Run count")
    require(len(value["child_links"]) == children == value["trees"][0]["descendants"], "admission counter/link mismatch")
    for c in value["capabilities"]:
        specs = json.loads(c["tools_json"])
        if c["role"] == "main":
            require(not c["can_spawn_agents"] and not any(s["name"] == "builtin__agent__run" for s in specs), "Main delegation exposed")
        else:
            require(not any(s["name"].startswith("builtin__task__") for s in specs), "Worker Task control")
    for child in child_nodes:
        parent = nodes[child["parent_run_id"]]
        cc, pc = caps[child["capability_snapshot_id"]], caps[parent["capability_snapshot_id"]]
        require(child["task_id"] == parent["task_id"] and child["task_epoch"] == parent["task_epoch"] and child["root_run_id"] == parent["root_run_id"] and child["depth"] == parent["depth"] + 1, "ancestry drift")
        require(child["model_snapshot_id"] == parent["model_snapshot_id"], "Child model changed")
        require(cc["data_scope"] == pc["data_scope"] and cc["skill_set_hash"] == pc["skill_set_hash"] and all(s in json.loads(pc["tools_json"]) for s in json.loads(cc["tools_json"])), "Child expanded frozen capability")
        for gen in value["generations"]:
            if gen["run_id"] == child["run_id"]:
                messages = json.loads(gen["request_json"])["messages"]
                require(not any("Task anchor:" in m["text"] or "Main private conversation" in m["text"] or "You are Erii." in m["text"] for m in messages), "Child context inherited")
    if completed:
        require(all(r["status"] in ("completed", "failed") for r in value["runs"]), "tree left unfinished")
        require(len(value["updates"]) == 1 and len(value["inbox"]) == 1 and sum(m["source"] == "main" and m["role"] == "assistant" for m in value["conversation"]) == 2 and len(value["deliveries"]) == sum(m["role"] == "assistant" for m in value["conversation"]), "Child bypassed direct Parent")
        for inv in value["tools"]:
            if inv["tool_name"] == "builtin__agent__run" and inv["status"] == "succeeded":
                result = json.loads(json.loads(inv["result_json"])["text"])
                require(set(result) == {"text", "artifactRefs"} and isinstance(result["artifactRefs"], list), "AgentResult Host state leak")
    require(all(c["token"] is None for c in value["claims"]), "leases retained")


def run(binary, scenario_jobs=1):
    report = {"version": 1, "scope": "ERII-17 Direct Tool Child admission and result path; M4 scheduling and M5 CodeCall/JavaScript remain due later", "checks": [], "crashes": [], "scenario_jobs": scenario_jobs}
    with tempfile.TemporaryDirectory(prefix="erii-children-") as directory:
        root = Path(directory)

        def scenario(name, action):
            folder = root / name
            folder.mkdir()
            h = ArtifactHarness(binary, folder, fixture="children")
            try:
                if name == "large-result":
                    h.setup(metadata={"name": "large-output", "provider_kind": "openai_compatible",
                                      "endpoint": "http://127.0.0.1:1/v1", "wire_api": "openai_completions",
                                      "model_id": "fixture-model", "context_window": 8192, "max_output_tokens": 8192,
                                      "generation_defaults": {}, "compatibility": {"tool_result_format": "openai"},
                                      "capabilities": {"text_input": True}})
                else:
                    h.setup()
                return action(h)
            finally:
                h.cleanup()

        def terminal(h):
            with Terminal(h.args("tui"), h.env, h.root) as t:
                t.expect("You> ")
                t.send("Delegate: Child tree. Main private conversation.")
                t.expect("Main follow-up:")
                t.expect("Erii: idle")
                require(t.stop() == 0, "TUI cleanup")
                transcript = t.transcript()
            value = state(h)
            check(value)
            return {"transcript": transcript, "state": value}

        report["terminal"] = scenario("terminal", terminal)
        report["checks"].append("PTY -> Main -> one Task/Root -> Child -> Grandchild -> direct Parent results -> one Main follow-up")

        def artifacts(h):
            h.send("one", "Delegate: Child artifact")
            h.command("chat", "work")
            value = state(h)
            check(value, children=1)
            inv = next(i for i in value["tools"] if i["tool_name"] == "builtin__agent__run")
            result = json.loads(json.loads(inv["result_json"])["text"])
            require(len(result["artifactRefs"]) == 1 and result["artifactRefs"] == json.loads(inv["arguments_json"])["artifactRefs"], "explicit Artifact input/result lost")
            require(len(value["artifacts"]) == 1 and len(value["publications"]) == 2, "child Artifact duplicated or not returned to Root")
            return value

        def large_result(h):
            h.send("one", "Delegate: Child large")
            h.command("chat", "work")
            value = state(h)
            check(value, children=1)
            invocation = next(i for i in value["tools"] if i["tool_name"] == "builtin__agent__run")
            result = json.loads(json.loads(invocation["result_json"])["text"])
            require(len(result["artifactRefs"]) == 1 and len(result["text"]) < 16384 and len(value["artifacts"]) == 1, "large Child result not normalized")
            content = h.output(result["artifactRefs"][0])
            require(content == ("bounded detail " * 1800).encode(), "large Child Artifact lost content")
            parent_requests = [json.loads(g["request_json"]) for g in value["generations"] if g["run_id"] == invocation["run_id"]]
            require(all("bounded detail " * 1800 not in m["text"] for request in parent_requests for m in request["messages"]), "large Child result entered Parent model context")
            require(not value["publications"], "large result auto-published to user")
            return {"state": value, "artifact_bytes": len(content), "artifact_sha256": hashlib.sha256(content).hexdigest()}
        report["large_result"] = scenario("large-result", large_result)
        report["checks"].append("Large Child text uses the shared Artifact/result transaction; CLI reads full bytes and Parent sees only the stable two-field envelope")

        report["artifacts"] = scenario("artifacts", artifacts)
        report["checks"].append("Scoped explicit ArtifactRef -> Child read/publish -> AgentResult -> Root publication -> Main delivery")

        for name, text, expected, children in (
            ("invalid", "Delegate: Child invalid", "INVALID_ARGUMENT", 0),
            ("inaccessible", "Delegate: Child inaccessible", "NOT_FOUND", 0),
            ("failure", "Delegate: Child failure", "PROVIDER_UNAVAILABLE", 1),
        ):
            def reject(h):
                h.send("one", text)
                h.command("chat", "work")
                value = state(h)
                check(value, children=children)
                results = [json.loads(i["result_json"]) for i in value["tools"] if i["tool_name"] == "builtin__agent__run"]
                require(len(results) == 1 and results[0]["error"]["code"] == expected and results[0]["error"]["retryable"] == (expected == "PROVIDER_UNAVAILABLE"), "wrong Host error")
                return value
            report[name] = scenario(name, reject)

        def forged(h):
            h.send("one", "Forge child")
            h.command("chat", "work")
            value = state(h)
            require(not value["tasks"] and not value["child_links"] and len(value["runs"]) == 1, "Main forged Child")
            require(json.loads(value["tools"][0]["result_json"])["error"]["code"] == "NOT_FOUND", "hidden schema leaked")
            return value
        report["forged"] = scenario("forged", forged)
        report["checks"].append("Main forged call, forbidden model parameter, inaccessible Artifact, Child model failure all fail through the canonical pipeline")

        def approval(h):
            h.send("one", "Delegate: Child approval")
            h.command("chat", "work")
            waiting = state(h)
            check(waiting, children=1, completed=False)
            require(len(waiting["approvals"]) == 1 and waiting["approvals"][0]["status"] == "pending", "Child approval not persisted")
            pending = waiting["approvals"][0]
            h.command("approval", "approve", pending["approval_id"], pending["frozen_arguments_hash"])
            h.command("chat", "work")
            value = state(h)
            check(value, children=1)
            effect = [i for i in value["tools"] if i["tool_name"] == "mcp__fixture__send"]
            require(len(effect) == 1 and effect[0]["attempt_count"] == 1 and effect[0]["status"] == "succeeded", "approval duplicated action")
            return {"waiting": waiting, "recovered": value}
        report["approval"] = scenario("approval", approval)
        report["checks"].append("Child approval survives CLI exit; waiting Parent/Child retain no leases; frozen action executes once after approval")

        def unknown(h):
            h.send("one", "Delegate: Child unknown")
            h.command("chat", "work")
            pending = state(h)["approvals"][0]
            h.command("approval", "approve", pending["approval_id"], pending["frozen_arguments_hash"])
            h.command("chat", "work")
            value = state(h)
            check(value, children=1)
            require(value["tasks"][0]["status"] == "active" and value["updates"][0]["kind"] == "unknown", "unknown effect became ordinary failure/success")
            require(sum(i["status"] == "unknown" for i in value["tools"]) == 2, "Child uncertainty not propagated through Agent Tool")
            require(any(r["parent_run_id"] and r["status"] == "failed" for r in value["runs"]), "Child model-failure fixture missing")
            h.command("chat", "work")
            require(state(h) == value, "unknown effect retried on recovery")
            return value
        report["unknown"] = scenario("unknown", unknown)
        report["checks"].append("Unknown Child external effect remains unknown at direct Parent even when Child model fails; restart never retries it")

        def cancel_tree(h):
            with Terminal(h.args("tui"), h.env, h.root) as t:
                t.expect("You> ")
                t.send("Delegate: Child hold")
                t.expect("Main accepted the work.")
                before = h.wait_state(lambda v: any(g["status"] == "pending" and any(m["text"] == "Hold child" for m in json.loads(g["request_json"])["messages"]) for g in v["generations"]), "Child model pending", reader=state)
                t.send("Cancel: Child hold")
                t.expect("Main accepted the work.")
                t.expect("Erii: idle")
                require(t.stop() == 0, "cancel TUI cleanup")
                transcript = t.transcript()
            after = state(h)
            require(after["tasks"][0]["status"] == "cancelled" and after["tasks"][0]["task_epoch"] == 2, "Task not fenced on cancel")
            require(all(r["status"] == "cancelled" for r in after["runs"] if r["kind"] == "worker"), "cancel did not reach Child")
            require(not after["updates"] and all(c["token"] is None for c in after["claims"]), "cancel leaked follow-up or lease")
            return {"before": before, "after": after, "transcript": transcript}
        report["cancel"] = scenario("cancel", cancel_tree)
        report["checks"].append("Live TUI cancellation fences the Task epoch and stops a real Child model call plus waiting Root")

        def crash_case(boundary):
            def crash(h):
                h.send("one", "Delegate: Child tree")
                process = h.hold(boundary)
                process.kill()
                process.communicate(timeout=10)
                before = state(h)
                h.command("chat", "work")
                after = state(h)
                check(after)
                old = {g["generation_id"]: g for g in before["generations"] if g["status"] == "completed"}
                require(all(g["attempt_count"] == old[g["generation_id"]]["attempt_count"] and g["request_hash"] == old[g["generation_id"]]["request_hash"] for g in after["generations"] if g["generation_id"] in old), "completed generation replayed")
                h.command("chat", "work")
                require(state(h) == after, "duplicate wake/drain changed terminal state")
                return {"boundary": boundary, "before_kill": before, "recovered": after}
            return scenario(boundary, crash)
        cases = ("before_child_accept_commit", "after_child_accept_commit", "before_result_commit", "after_result_commit", "before_tool_commit", "after_tool_commit")
        report['crashes'] = run_cases(cases, crash_case, scenario_jobs)
        require({r['boundary'] for r in report['crashes']} == set(cases) and len(report['crashes']) == 6, 'complete Child crash inventory required')
        report["checks"].append("Six SIGKILL/restart boundaries preserve one Child per call and reuse committed generations/results")
    report["cleanup"] = "all owned PTYs/children reaped; isolated databases and effect ledgers removed"
    return report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--scenario-jobs", type=int, choices=[1,2,4], default=int(os.environ.get("ERII_VERIFY_CASE_JOBS","1")))
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    binary = args.binary.resolve()
    report = run(binary, args.scenario_jobs)
    report["binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"checks": report["checks"], "crash_boundaries": len(report["crashes"]), "evidence": str(args.evidence)}))


if __name__ == "__main__":
    main()
