#!/usr/bin/env python3
"""Run due M1 fragments through the production terminal/Actor/Loop boundary."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import tempfile
import time

from case_runner import digest, load_contract, result, variants
from terminal_driver import Terminal
from verify_main import MainHarness, assert_final, require


def timestamp():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def drive(binary, folder, case, variant):
    h = MainHarness(binary, folder, fixture="golden")
    try:
        started, tick = timestamp(), time.monotonic()
        h.setup()
        # Fixture context is explicit Conversation data, not a hidden prompt,
        # fabricated connector response, direct domain DB edit, or alternate loop.
        h.send("fixture-context", variant["context"])
        event = variant["events"][0]
        h.send("event-0", "当前时间：" + event["at"] + "。" + event["text"])
        with Terminal(h.args("tui"), h.env, h.root) as t:
            t.expect("State: ready")
            t.expect("assistant: "); t.expect("You> ")
            for event in variant["events"][1:]:
                t.send("当前时间：" + event["at"] + "。" + event["text"])
                t.expect("assistant: "); t.expect("You> ")
            require(t.stop() == 0, "Case terminal did not exit cleanly")
            transcript = t.transcript()
        state = h.state()
        assert_final(state, len(case["events"]) + 1, len(case["events"]))
        return {"started_at": started, "finished_at": timestamp(),
                "wall_time_ms": round((time.monotonic() - tick) * 1000),
                "transcript": transcript, "state": state}
    finally:
        h.cleanup()


def run_suite(dataset, policy, driver):
    """Independent of provider/terminal mechanics; drivers return observed state."""
    results = []
    for case in dataset["cases"]:
        for variant in variants(dataset, case):
            execution = driver(case, variant)
            results.append(result(case, variant, policy, execution))
    due = {case["case_id"] for case in dataset["cases"]}
    return {"version": 1, "milestone": dataset["milestone"], "level": dataset["level"],
            "scope": dataset["scope"], "dataset_digest": digest(dataset), "policy_digest": digest(policy),
            "clock_policy": "fixture clock and event times are frozen explicit model context; lifecycle and telemetry use real wall time",
            "provider": "explicit scripted golden Bridge; no network or real-model quality evidence",
            "results": results,
            "not_applicable_cases": [f"GC-{i:02d}" for i in range(1, 21) if f"GC-{i:02d}" not in due],
            "summary": {"fragments": len(results), "fragment_passes": sum(r["status"] == "fragment_pass" for r in results),
                        "hard_failures": sum(r["hard_failure"] for r in results),
                        "scored_cases": 0, "suite_mean": None, "suite_min": None,
                        "pass@1": None, "pass^5": None,
                        "reason": "no semantic Judge or live model executed; scripted fragments cannot satisfy product score gates"},
            "live_shadow": "inconclusive; unexecuted",
            "cleanup": "owned PTYs/children reaped; isolated fixture databases removed"}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    root = Path(__file__).resolve().parents[1]
    dataset, policy, lock = load_contract(root)
    binary = args.binary.resolve()
    with tempfile.TemporaryDirectory(prefix="erii-cases-") as tmp:
        def driver(case, variant):
            folder = Path(tmp) / (case["case_id"] + "-" + variant["variant_id"])
            folder.mkdir()
            return drive(binary, folder, case, variant)
        report = run_suite(dataset, policy, driver)
    report["contract_lock"] = lock
    report["binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
    report["bridge_digest"] = hashlib.sha256((root / "provider-bridge/package-lock.json").read_bytes()).hexdigest()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report["summary"], ensure_ascii=False))
    if report["summary"]["fragment_passes"] != report["summary"]["fragments"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
