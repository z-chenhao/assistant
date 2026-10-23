"""Versioned, provider-independent Case result and scoring protocol.

The driver supplies observed evidence. Mechanical assertions take precedence
over a grader. A scripted Fake can prove a fragment, never a semantic score.
The scenario clock is explicit fixture context; Host leases/telemetry retain
real wall time. Missing future systems are recorded, never simulated as passed.
"""
from datetime import datetime
import hashlib
import json
import math


def digest(value):
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True,
                                     separators=(",", ":")).encode()).hexdigest()


def load_contract(root, fixture="m1-v2"):
    lock = json.loads((root / f"evals/fixtures/{fixture}.lock.json").read_text())
    for name, expected in lock["files"].items():
        if hashlib.sha256((root / name).read_bytes()).hexdigest() != expected:
            raise ValueError("fixture/policy changed without a versioned lock update: " + name)
    dataset = json.loads((root / f"evals/fixtures/{fixture}.json").read_text())
    policy = json.loads((root / "evals/scoring-v1.json").read_text())
    for case in dataset["cases"]:
        times = [datetime.fromisoformat(e["at"]) for e in case["events"]]
        if times != sorted(times) or case["clock"] != case["events"][0]["at"]:
            raise ValueError("scenario clock or causal event order drift")
        if any(len(e["wordings"]) < 3 for e in case["events"]):
            raise ValueError("three wording variants required")
    return dataset, policy, lock


def score(policy, assertions, hard_failures, grades=None, applicable=None):
    if not assertions or any(type(value) is not bool for value in assertions.values()):
        raise ValueError("explicit boolean programmatic assertions required")
    if any(value not in policy["hard_failure_categories"] for value in hard_failures):
        raise ValueError("unknown hard failure category")
    if hard_failures or not all(assertions.values()):
        return {"status": "fail", "final_score": 0, "semantic_grading": "cannot override programmatic failure"}
    if grades is None:
        return {"status": "ungraded", "final_score": None, "semantic_grading": "unexecuted; scripted Fake is protocol evidence only"}
    if not applicable or set(grades) != set(applicable) or not set(applicable) <= set(policy["weights"]):
        raise ValueError("grader must cover every explicitly applicable dimension")
    if any(type(v) not in (int, float) or not math.isfinite(v) or not 0 <= v <= 4 for v in grades.values()):
        raise ValueError("dimension scores must be finite in [0,4]")
    total = sum(policy["weights"][key] for key in applicable)
    value = sum(policy["weights"][key] * grades[key] / 4 for key in applicable) / total * 100
    return {"status": "pass" if value >= policy["case_min"] else "fail",
            "final_score": round(value, 2), "semantic_grading": {"grades": grades, "applicable_dimensions": applicable}}


def variants(dataset, case):
    for variant in dataset["variants"]:
        world = case["world"]
        if variant.get("reverse_world_keys"):
            world = dict(reversed(list(world.items())))
        context = {**dataset["common"], "clock": case["clock"], "world": world}
        text = "以下是本次对话明确提供的合成背景：" + json.dumps(context, ensure_ascii=False, separators=(",", ":"))
        noise = ""
        if variant["noise"]:
            phrase = ("无关闲聊：昨天整理了书签。" if variant["noise"] == 1 else "无关闲聊：朋友换了手机壳。")
            # 10–30% irrelevant Conversation characters, with normative facts
            # unchanged. Noise never contains instructions or Gold answers.
            length = len(text) + sum(len(e["wordings"][variant["wording"]]) for e in case["events"])
            while len(noise) < length * .15:
                noise += phrase
            if not .1 <= len(noise) / length <= .3:
                raise ValueError("noise ratio outside normative bounds")
            text += " 无关Conversation：" + noise
        events = [{**{k: v for k, v in e.items() if k != "wordings"}, "text": e["wordings"][variant["wording"]]} for e in case["events"]]
        yield {"variant_id": variant["id"], "seed": 0, "clock": case["clock"],
               "context": text, "noise_characters": len(noise), "events": events,
               "fixture_digest": digest({"case": case, "common": dataset["common"], "variant": variant, "context": text, "events": events})}


def result(case, variant, policy, execution):
    state = execution["state"]
    replies = [m for m in state["conversation"] if m["role"] == "assistant"]
    texts = ["".join(p.get("text", "") for p in m["content"]) for m in replies]
    questions = [text.count("？") + text.count("?") for text in texts]
    caps = state["capabilities"]
    tools = state["tools"]
    runs = state["runs"]
    requests = [json.loads(g["request_json"]) for g in state["generations"]]
    cap_by_id = {c["snapshot_id"]: c for c in caps}
    model_by_id = {m["snapshot_id"]: m for m in state["snapshots"]}
    assertions = {
        "one_reply_per_event": len(replies) == len(case["events"]),
        "ordered_semantic_history": [m["sequence"] for m in state["conversation"]] == list(range(1, len(state["conversation"]) + 1)),
        "no_task_or_child": bool(runs) and all(r["kind"] == "main" and r["task_id"] is None and r["parent_run_id"] is None for r in runs),
        "main_cannot_spawn": bool(caps) and all(not c["can_spawn_agents"] for c in caps),
        "no_external_writes": bool(caps) and all((t["permission"] == "read_only" or (c.get("role") == "main" and t["name"] == "builtin__memory__manage" and t["permission"] == "memory_control") or (t["name"] == "builtin__task__manage" and t["permission"] == "task_control") or (c.get("role") == "main" and t["name"] == "builtin__automation__manage" and t["permission"] == "task_control") or (t["name"] in ("builtin__artifact__manage",) and t["permission"] == "artifact_write")) for c in caps for t in json.loads(c["tools_json"])),
        "only_pure_tools": bool(tools) and all(t["tool_name"] == "mcp__fixture__echo" and t["replay_policy"] == "safe" and t["status"] == "succeeded" for t in tools),
        "no_duplicate_effects": all(t["attempt_count"] == 1 for t in tools),
        "terminal_runs": all(r["status"] == "completed" for r in runs),
        "complete_exchanges": len(requests) == 2 * len(case["events"]) and all(g["status"] == "completed" for g in state["generations"]),
        "frozen_snapshots": bool(runs) and all(r["model_snapshot_id"] in model_by_id and r["capability_snapshot_id"] in cap_by_id for r in runs) and all(
            req["model_snapshot"]["snapshot_id"] == next(r["model_snapshot_id"] for r in runs if r["run_id"] == req["run_id"])
            and all(req["model_snapshot"] == other["model_snapshot"] and req["tools"] == other["tools"]
                    for other in requests if other["run_id"] == req["run_id"])
            for req in requests),
        "atomic_usage": len(state["usage"]) == len(state["generations"]),
        "delivery_intents": len(state["deliveries"]) == len(replies) and len({d["message_id"] for d in state["deliveries"]}) == len(replies),
        "trace_not_in_conversation": all(p["type"] == "text" for m in state["conversation"] for p in m["content"]),
        "required_facts": len(texts) == len(case["events"]) and all(all(fact in text for fact in e["required_text"]) for e, text in zip(case["events"], texts)),
        "bounded_questions": len(questions) == len(case["events"]) and all(n <= e["max_questions"] for e, n in zip(case["events"], questions)),
        "no_forbidden_claims": all(fact not in text for text in texts for fact in case["forbidden_text"]),
        "bounded_items": all(len(text.splitlines()) <= case.get("max_items", 100) for text in texts),
    }
    failures = []
    if not assertions["no_task_or_child"] or not assertions["no_external_writes"]:
        failures.append("unauthorized_external_effect")
    if not assertions["no_duplicate_effects"]:
        failures.append("replay_duplicate_or_unknown_retry")
    if not assertions["no_forbidden_claims"]:
        failures.append("fabricated_completion")
    grading = score(policy, assertions, failures)
    return {"case_id": case["case_id"], "case_version": case["version"],
            "variant_id": variant["variant_id"], "level": "L0", "seed": variant["seed"],
            "fixture_digest": variant["fixture_digest"], "clock": variant["clock"],
            "input_events": variant["events"], "started_at": execution["started_at"], "finished_at": execution["finished_at"],
            "run_id": runs[0]["run_id"] if runs else None,
            "model_snapshot": state["snapshots"], "capability_snapshot": caps,
            "status": "fragment_pass" if grading["status"] == "ungraded" else "fragment_fail",
            "full_case": "not_applicable", "not_applicable": case["not_applicable"],
            "final_score": grading["final_score"], "semantic_grading": grading["semantic_grading"],
            "hard_failure": bool(failures), "hard_failures": failures, "assertions": assertions,
            "metrics": {"wall_time_ms": execution["wall_time_ms"], "model_calls": len(requests),
                        "tool_calls": len(tools), "child_runs": 0, "approvals": 0,
                        "user_questions": sum(questions), "duplicate_effects": sum(t["attempt_count"] > 1 for t in tools)},
            "artifact_refs": [], "trace_ref": "evidence.state.trace", "evidence": execution,
            "unimplemented_state": {name: "not_applicable" for name in ("Task", "Approval", "Memory", "Artifact", "external_effect_ledger")}}
