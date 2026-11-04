#!/usr/bin/env python3
"""Owner CLI navigation through a real Task and persisted GitHub admission."""
import argparse
from contextlib import closing
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import sqlite3
import tempfile
import time

from verify_main import MainHarness, require


class EvidenceHarness(MainHarness):
    def args(self, *args):
        return [str(self.binary), "-config", str(self.root / "config.yaml"), "-test-bridge", self.fixture, *args]


def run(binary):
    with tempfile.TemporaryDirectory(prefix="steward-github-evidence-") as temp:
        root = Path(temp)
        h = EvidenceHarness(binary, root, fixture="tasks")
        config = root / "config.yaml"
        base = "version: 1\nmemory:\n  encryption_key_ref: env:EVIDENCE_FIXTURE_KEY\n"
        source = "github:\n  repository: owner/repo\n  pull_number: 7\n  credential_ref: env:EVIDENCE_UNUSED_GITHUB_KEY\n"
        config.write_text(base + source)
        h.env["EVIDENCE_FIXTURE_KEY"] = "S" * 32
        h.env.pop("EVIDENCE_UNUSED_GITHUB_KEY", None)
        data = root / "data"
        scope = "owner/repo#7"
        body = "保存的评审原文 🧭\n" * 500

        def database():
            return closing(sqlite3.connect(data / "state.db"))

        def read(ref, **page):
            return h.command("evidence", "read", text=json.dumps(dict(ref=ref, **page)))

        def denied(ref):
            response = h.command("evidence", "read", text=json.dumps(dict(ref=ref)), ok=False)
            require(response.returncode != 0 and not response.stdout and "NOT_FOUND" in response.stderr,
                    "stale or inaccessible evidence returned content or a distinct error")

        def snapshot():
            result = {}
            for name in ("state.db", "conversation.db", "memory.db"):
                with closing(sqlite3.connect((data / name).as_uri() + "?mode=ro", uri=True)) as db:
                    db.execute("BEGIN")
                    result[name] = hashlib.sha256("\n".join(db.iterdump()).encode()).hexdigest()
            return result

        def observe(version, state=""):
            # Seed exactly the immutable result of an authenticated scan. The
            # real Actor creates any causal association during chat work.
            with database() as db, db:
                generation, epoch = db.execute("SELECT generation,privacy_epoch FROM github_task_binding WHERE scope=?", (scope,)).fetchone()
                kind = "check_run" if state else "review_comment"
                content = dict(event_key=f"{kind}:1:{version}", kind=kind, source_id=1,
                               url="https://github.com/owner/repo/pull/7#discussion_r1", body=body, actor="fixture-reviewer",
                               updated_at=datetime.now(timezone.utc).isoformat())
                if state:
                    content.update(url="https://github.com/owner/repo/actions/runs/1", body="Quality checks",
                                   actor="", state=state, head_sha="a" * 40)
                row = db.execute(
                    "INSERT INTO github_observation(scope,event_key,kind,source_id,content_json,received_at,intent_privacy_epoch,binding_generation)"
                    " VALUES(?,?,?,?,?,?,?,?)",
                    (scope, content["event_key"], content["kind"], 1, json.dumps(content), time.time_ns() // 1000, epoch, generation))
                return f"github:{row.lastrowid}"

        try:
            # A causal Main retains the protected 4 KiB source preview and the
            # Task anchor alongside its frozen catalog.
            h.setup(metadata=dict(name="GitHub evidence fixture", provider_kind="openai_compatible",
                                  endpoint="http://127.0.0.1:1/v1", wire_api="openai_completions",
                                  model_id="fixture-model", context_window=16384, max_output_tokens=1024,
                                  compatibility=dict(tool_result_format="openai"), capabilities=dict(text_input=True)))
            h.send("original", "Ongoing: Review the exact linked PR")
            h.command("chat", "work")
            with database() as db, db:
                task, epoch = db.execute("SELECT task_ref,task_epoch FROM task").fetchone()
                db.execute("INSERT INTO github_watch(scope,repository,pull_number,repository_id,enabled,intent_privacy_epoch,state,last_success_at,version)"
                           " VALUES(?, 'owner/repo', 7, 99, 1, 0, 'healthy', ?, 1)", (scope, time.time_ns() // 1000))
            h.command("github", "bind", text=json.dumps(dict(task_ref=task, task_epoch=epoch, version=0)))
            linked = observe("linked")
            h.command("chat", "work")
            unassigned = observe("not-yet-accepted")
            before = snapshot()
            links = read(task)["links"]
            require(dict(ref=linked, relation="github_observation") in links and not any(x["ref"] == unassigned for x in links),
                    "Task did not distinguish exact causal evidence from same-PR observation")
            offset = 0
            chunks = []
            while True:
                page = read(linked, offset=offset)
                require(page["kind"] == "github_observation" and page["author"] == "fixture-reviewer"
                        and page["offset"] == offset and page["bytes"] <= 4096
                        and page["links"] == [dict(ref=task, relation="associated_task")],
                        "observation page lost its Task, type, or byte boundary")
                chunks.append(page["text"])
                if "next_offset" not in page:
                    break
                require(page["next_offset"] > offset, "observation cursor failed to advance")
                offset = page["next_offset"]
            require("".join(chunks) == body and len(chunks) > 1, "observation version was changed or not paged")
            require(not read(linked, links_offset=1)["links"], "single reverse link repeated after its cursor")
            denied(unassigned)
            config.write_text(base)
            denied(linked)
            require(not any(x["ref"] == linked for x in read(task)["links"]), "missing source scope leaked a Task link")
            config.write_text(base + source.replace("pull_number: 7", "pull_number: 8"))
            denied(linked)
            config.write_text(base + source)
            require(snapshot() == before, "evidence navigation mutated durable state or bindings")

            with database() as db, db:
                db.execute("UPDATE github_watch SET state='interrupted',failure_code='UNAVAILABLE',version=version+1")
            require(read(linked)["kind"] == "github_observation", "temporary poll failure revoked retained evidence")
            with database() as db, db:
                db.execute("UPDATE github_watch SET failure_code='CREDENTIAL_REJECTED',version=version+1")
            denied(linked)
            require(not any(x["ref"] == linked for x in read(task)["links"]), "hard authorization failure leaked a Task link")
            with database() as db, db:
                db.execute("UPDATE github_watch SET state='healthy',failure_code='',version=version+1")

            binding = h.command("github", "binding")
            h.command("github", "unbind", text=json.dumps(dict(version=binding["version"])))
            denied(linked)
            binding = h.command("github", "binding")
            h.command("github", "bind", text=json.dumps(dict(task_ref=task, task_epoch=epoch, version=binding["version"])))
            denied(linked)
            fresh = observe("new-generation", state="failure")
            h.command("chat", "work")
            page = read(fresh)
            require(page["links"] == [dict(ref=task, relation="associated_task")] and page["text"].startswith("State: failure\n"),
                    "new generation did not expose its own admitted check conclusion")
            h.command("memory", "manage", "forget-github-evidence", text=json.dumps(dict(operation="forget", category="all")))
            denied(fresh)
            denied(task)
        finally:
            h.cleanup()
    return dict(passed=True, pages=len(chunks), cleanup=not root.exists(),
                checks=["exact causal Task and observation round-trip", "UTF-8 continuation and link cursor",
                        "same-PR unassigned observation hidden", "missing or different config is read-only",
                        "poll interruption and hard authorization differ", "unbind and rebind exclude old generations",
                        "check conclusion projection and Forget revocation"])


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--evidence", required=True, type=Path)
    args = parser.parse_args()
    report = run(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report))
