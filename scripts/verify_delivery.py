#!/usr/bin/env python3
"""Delivery Launch/Doctor/Drive/Evidence/Cleanup through the CLI and real PTY.

Uses the maintained MainHarness and Terminal mechanics. No live accounts.
Every crash snapshot reads databases directly, without triggering recovery.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import signal
import sqlite3
import subprocess
import tempfile

from terminal_driver import Terminal
from verify_main import MainHarness, require
from fixture_pool import run_cases


class Harness(MainHarness):
    def __init__(self, binary, root, fixture="main"):
        super().__init__(binary, root, fixture)
        self.config = root / "delivery.yaml"
        self.config.write_text("version: 1\ndelivery:\n  lease_seconds: 1\n  retry_milliseconds: 50\n  max_attempts: 3\n", encoding="utf-8")

    def args(self, *args):
        return [str(self.binary), "-config", str(self.config), "-test-bridge", self.fixture, *args]


def state(h):
    result = {}
    root = Path(h.env["ERII_DATA_ROOT"])
    for filename, queries in {
        "state.db": {
            "requests": "SELECT * FROM delivery_request ORDER BY created_at,delivery_id",
            "terminal": "SELECT * FROM terminal_message ORDER BY rowid",
            "outbox": "SELECT * FROM delivery_outbox ORDER BY delivery_id",
            "runs": "SELECT run_id,kind,status,checkpoint_json FROM run ORDER BY created_at",
            "generations": "SELECT generation_id,status,attempt_count FROM model_generation ORDER BY created_at",
            "intents": "SELECT * FROM conversation_intent ORDER BY sequence",
            "replies": "SELECT * FROM main_reply ORDER BY rowid",
            "events": "SELECT type,payload_json FROM event_outbox WHERE type LIKE 'ai.erii.delivery.%' ORDER BY occurred_at,event_id",
            "approvals": "SELECT * FROM approval ORDER BY created_at",
            "task_updates": "SELECT * FROM task_update ORDER BY occurred_at",
        },
        "conversation.db": {"history": "SELECT * FROM conversation_message ORDER BY sequence"},
    }.items():
        with sqlite3.connect(root / filename) as db:
            db.row_factory = sqlite3.Row
            for key, query in queries.items():
                result[key] = [dict(r) for r in db.execute(query)]
    return result


def assert_delivered(value, count):
    replies = [m for m in value["history"] if m["role"] == "assistant"]
    require(len(replies) == len(value["requests"]) == len(value["terminal"]) == count, "missing or duplicate semantic delivery")
    require(not value["outbox"], "ack did not retire delivery outbox")
    require(all(r["status"] == "sent" and r["channel"] == "tui" and r["idempotency_key"] == r["delivery_id"] for r in value["requests"]), "wrong route/key/status")
    by_id = {r["message_id"]: r for r in value["requests"]}
    semantic = {m["message_id"]: m for m in replies}
    for receipt in value["terminal"]:
        envelope = json.loads(receipt["envelope_json"])
        request = by_id[receipt["message_id"]]
        require(request["destination_ref"] == envelope["message"]["conversation_id"] == receipt["destination_ref"], "wrong destination")
        require(envelope["message"]["content"] == json.loads(semantic[receipt["message_id"]]["content_json"]), "send changed semantic message")
    require(all(r["status"] == "completed" for r in value["runs"]), "Run not completed")


def run(binary, feedback=False, scenario_jobs=1):
    report = {"version": 1, "scope": "M3 durable semantic delivery to the local terminal inbox; stdout redisplay and remote provider exactly-once are not claimed", "checks": [], "crashes": [],
              "evidence_scope": "early_feedback" if feedback else "full", "scenario_jobs": scenario_jobs}
    with tempfile.TemporaryDirectory(prefix="erii-delivery-") as directory:
        root = Path(directory)

        def scenario(name, action, fixture="main"):
            folder = root / name
            folder.mkdir()
            h = Harness(binary, folder, fixture)
            try:
                h.setup()  # Doctor proves explicit Fake, readiness and private state.
                result = action(h)
                print("delivery scenario passed:", name, flush=True)
                return result
            finally:
                h.cleanup()

        boundaries = ("before_conversation_append", "after_conversation_append",
                      "before_delivery_commit", "after_delivery_commit",
                      "before_reply_checkpoint", "after_reply_checkpoint",
                      "before_delivery_send", "after_delivery_send",
                      "before_delivery_ack", "after_delivery_ack")
        # Early feedback adds transport/approval regressions before the full
        # merge tier runs all twenty faults five times. It cannot sign off the
        # full gate; the default invocation always retains that entire series.
        def crash_case(item):
            role, boundary = item
            def crash(h):
                if role == "root":
                    # Freeze the Root before work, let Main acceptance settle,
                    # then deliver that acceptance before faulting the final reply.
                    h.send("delegate", "Delegate: Recover this research report.")
                    held = h.hold("before_worker_generation")
                    h.wait_state(lambda v: any(r["kind"] == "main" and r["status"] == "completed" for r in v["runs"]), "Main acceptance committed", reader=state)
                    h.command("delivery", "work")
                    held.kill(); held.communicate(timeout=10)
                with Terminal(h.args("-test-main-hold", boundary, "tui"), h.env, h.root) as t:
                    if role == "main":
                        t.expect("You> "); t.send("Deliver this final answer.")
                    t.expect('"boundary":"' + boundary + '"')
                    before = state(h)
                    t.stop(signal.SIGKILL)
                    killed = t.transcript()
                with Terminal(h.args("tui"), h.env, h.root) as t:
                    t.expect("Main follow-up:" if role == "root" else "assistant: Processed:")
                    h.wait_state(lambda v: len(v["requests"]) == (2 if role == "root" else 1) and all(r["status"] == "sent" for r in v["requests"]) and all(r["status"] == "completed" for r in v["runs"]), "recovered send and checkpoint", reader=state)
                    require(t.stop() == 0, "recovered terminal cleanup failed")
                    transcript = t.transcript()
                after = state(h); assert_delivered(after, 2 if role == "root" else 1)
                completed = {g["generation_id"]: g for g in before["generations"] if g["status"] == "completed"}
                require(all(g == completed[g["generation_id"]] for g in after["generations"] if g["generation_id"] in completed), "accepted generation replayed")
                # No resubmission or model work is needed to recover transport.
                h.command("delivery", "work"); h.command("chat", "work")
                require(state(h) == after, "duplicate scan changed settled history or sent again")
                return {"role": role, "boundary": boundary, "signal": "SIGKILL", "before": before, "after": after, "killed_transcript": killed, "recovered_transcript": transcript}
            return scenario(role + "-" + boundary, crash, "tasks" if role == "root" else "main")
        cases = [] if feedback else [(role, boundary) for role in ("main", "root") for boundary in boundaries]
        report["crashes"] = run_cases(cases, crash_case, scenario_jobs)
        if not feedback:
            require(len(report["crashes"]) == 20, "complete Main/Root crash inventory required")
            report["checks"].append("20 SIGKILL/restart scenarios: Main and Root final replies through real PTY; Conversation, enqueue, checkpoint, send and ack boundaries")

        for mode in ("retry-once", "ambiguous", "unknown", "unavailable"):
            def failure(h):
                with Terminal(h.args("-test-delivery", mode, "tui"), h.env, h.root) as t:
                    t.expect("You> "); t.send("Recover this transport failure.")
                    expected = "unknown" if mode == "unknown" else "failed" if mode == "unavailable" else "sent"
                    value = h.wait_state(lambda v: v["requests"] and v["requests"][0]["status"] == expected, mode, reader=state)
                    if expected == "sent":
                        t.expect("assistant: Processed:")
                    require(t.stop() == 0, "failure terminal cleanup failed")
                    transcript = t.transcript()
                if expected == "sent": assert_delivered(value, 1)
                if mode == "unknown":
                    attempts = value["requests"][0]["attempt_count"]
                    require(not value["terminal"] and attempts == 1, "unknown was retried")
                    h.command("delivery", "work")
                    later = state(h)
                    require(later["requests"][0]["status"] == "unknown" and later["requests"][0]["attempt_count"] == attempts, "unknown absence caused retry")
                if mode == "unavailable": require(value["requests"][0]["attempt_count"] == 3 and not value["terminal"], "retry limit")
                return {"mode": mode, "state": value, "transcript": transcript}
            report[mode] = scenario(mode, failure)
        report["checks"].append("deterministic recoverable failure, ambiguous accepted send reconciliation, query-only unknown, bounded exponential retry")

        def approval(h, decision):
            with Terminal(h.args("tui"), h.env, h.root) as t:
                t.expect("You> ")
                text = json.dumps({"target": "alex@example.test", "text": "Tuesday review"})
                t.send("Delegate effect: " + text)
                t.expect("Approval required for")
                waiting = h.wait_state(lambda v: v["approvals"] and v["approvals"][0]["status"] == "pending", "Security notice", reader=state)
                notice = next(r for r in waiting["requests"] if r["intent"] == "approval")
                require(len([r for r in waiting["runs"] if r["kind"] == "main"]) == 1 and not waiting["task_updates"], "approval unnecessarily woke Main")
                approval = waiting["approvals"][0]
                command = "/" + decision + " " + approval["approval_id"] + " " + approval["frozen_arguments_hash"]
                t.send(command); t.expect("Approval decision saved.")
                t.expect("Observed Worker update:")
                h.wait_state(lambda v: v["task_updates"] and all(r["status"] == "sent" for r in v["requests"]), "approval resume delivered", reader=state)
                t.send(command); t.expect("Approval decision saved.")
                require(t.stop() == 0, "approval terminal cleanup failed")
                transcript = t.transcript()
            value = state(h)
            require(len([r for r in value["requests"] if r["intent"] == "approval"]) == 1 and value["approvals"][0]["status"] == ("approved" if decision == "approve" else "denied"), "approval callback duplicated notice")
            require(len(value["runs"]) == 3 and len(value["task_updates"]) == 1, "approval created a replacement Worker or duplicate Main")
            require(not any(r["intent"] == "progress" for r in value["requests"]), "progress notification")
            return {"waiting": waiting, "after": value, "notice": notice, "transcript": transcript}
        for decision in ("approve", "deny"):
            report["approval-" + decision] = scenario("approval-" + decision, lambda h: approval(h, decision), "approval")
        report["checks"].append("Security approval notice without Main inference; PTY approve/deny frozen-hash callbacks wake original Root; duplicates create no notice")

        def transport_only(h):
            h.send("one", "Deliver after loss of model configuration")
            child = h.hold("before_reply_checkpoint")
            child.kill(); child.communicate(timeout=10)
            # Ordinary production startup with no provider command or Fake.
            command = [str(h.binary), "-config", str(h.config), "delivery", "work"]
            result = subprocess.run(command, env=h.env, cwd=h.root, capture_output=True, text=True, timeout=15)
            require(result.returncode == 0 and json.loads(result.stdout)[0]["status"] == "sent", "delivery required model readiness")
            require(all(isinstance(json.loads(result.stdout)[0][key], str) and json.loads(result.stdout)[0][key].endswith("Z") for key in ("created_at", "updated_at")), "CLI timestamps are not RFC3339 UTC")
            value = state(h)
            require(len(value["terminal"]) == 1 and len(value["generations"]) == 2, "transport inferred or lost accepted message")
            return {"requests": json.loads(result.stdout), "state": value}
        report["transport-without-provider"] = scenario("transport-without-provider", transport_only)
        report["checks"].append("delivery work accepts persisted message with no configured provider or Fake and no model inference")
    report["checks"].append("cleanup: owned children reaped, PTYs closed, disposable data removed")
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--feedback", action="store_true", help="additional early transport/approval checks; not full merge evidence")
    parser.add_argument("--scenario-jobs", type=int, choices=[1, 2, 4], default=int(os.environ.get("ERII_VERIFY_CASE_JOBS", "1")))
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    report = run(args.binary.resolve(), args.feedback, args.scenario_jobs)
    report["binary_sha256"] = hashlib.sha256(args.binary.read_bytes()).hexdigest()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"checks": report["checks"], "crashes": len(report["crashes"]), "evidence": str(args.evidence)}))
