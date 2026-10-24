#!/usr/bin/env python3
"""Run due M1 fragments through the production terminal/Actor/Loop boundary."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import sqlite3
import tempfile
import time

from case_runner import digest, load_contract, result, variants
from terminal_driver import Terminal
from verify_main import MainHarness, assert_final, require
from agenda_case import (observations as agenda_observations, load_steering,
                         steering_event, steering_observations)


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
        def readback():
            state = h.state()
            if case['case_id'] == 'GC-09':
                with sqlite3.connect(folder/'data/state.db') as db:
                    db.row_factory = sqlite3.Row
                    for key, table in (('tasks','task'), ('automations','automation'),
                                       ('approvals','approval'), ('artifacts','artifact'),
                                       ('compactions','context_compaction')):
                        state[key] = [dict(row) for row in db.execute('SELECT * FROM '+table)]
            return state
        state = readback()
        assert_final(state, len(case["events"]) + 1, len(case["events"]))
        execution = {"started_at": started, "finished_at": timestamp(),
                "wall_time_ms": round((time.monotonic() - tick) * 1000),
                "transcript": transcript, "state": state}
        if case['case_id'] == 'GC-09':
            h.send('event-0', '当前时间：'+event['at']+'。'+event['text'])
            h.command('chat', 'work')
            execution['replay_state'] = readback()
            # Preserve the original locked journey as its own readback. Drive a
            # separately locked same-day update through a new real Main Run.
            steering_started, steering_tick = timestamp(), time.monotonic()
            text = steering_event(variant)
            receipt = h.send('steering', text)
            child = h.hold('after_compaction_commit')
            child.kill(); child.communicate(timeout=10)
            frozen = readback()
            h.command('chat', 'work')
            updated = readback()
            require(h.send('steering', text) == receipt, 'Steering duplicate receipt drift')
            h.command('chat', 'work')
            execution['steering'] = dict(event=text, fixture_lock=load_steering()[1], receipt=receipt,
                started_at=steering_started, finished_at=timestamp(), wall_time_ms=round((time.monotonic()-steering_tick)*1000),
                before_kill=frozen, restart=dict(boundary='after_compaction_commit', returncode=child.returncode),
                state=updated, replay_state=readback())
        return execution
    finally:
        h.cleanup()
        if 'execution' in locals():
            execution['cleanup'] = dict(owned_processes_joined=all(c.poll() is not None for c in h.children))


def run_suite(dataset, policy, driver, result_builder=result):
    """Independent of provider/terminal mechanics; drivers return observed state."""
    results = []
    for case in dataset["cases"]:
        for variant in variants(dataset, case):
            execution = driver(case, variant)
            row = result_builder(case, variant, policy, execution)
            if case['case_id'] == 'GC-09':
                checks = agenda_observations(variant, execution)
                checks.update({'steering_'+key: value for key, value in steering_observations(variant, execution).items()})
                row['assertions'].update(checks)
                row['scope'] = 'scripted_whole_case_mechanics'
                row['full_case'] = 'inconclusive; unscripted evaluation and semantic grades required'
                row['not_applicable'] = []
                row['unimplemented_state'] = {}
                if not all(checks.values()):
                    row.update(status='fragment_fail', final_score=0,
                               semantic_grading='cannot override programmatic failure')
            results.append(row)
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
    report['cleanup_checks'] = dict(disposable_root_removed=not Path(tmp).exists(),
        owned_processes_joined=all(r['evidence']['cleanup']['owned_processes_joined'] for r in report['results']))
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
