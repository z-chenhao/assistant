#!/usr/bin/env python3
"""Owned HTTP endpoint -> actual Go CLI -> actual pinned Node/Pi Bridge.

Launch/doctor/drive/evidence/cleanup are one foreground, disposable operation.
"""
import argparse
from contextlib import closing
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import tempfile
import threading
import time
from verify import STATE_SCHEMA_VERSION


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


class Endpoint(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def do_GET(self):
        self.server.requests.append({"method": "GET", "auth": self.headers.get("Authorization")})
        if self.server.mode == "manual":
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        data = json.dumps({"data": [{"id": "fixture-model"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.server.requests.append({"method": "POST", "auth": self.headers.get("Authorization"),
                                     "session": self.headers.get("session_id"), "body": body})
        if self.server.mode == "auth":
            self.send_response(401)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.server.mode == "slow" and body["messages"][-1].get("content") == "Reply with OK.":
            # Actual cold-start regression: longer than the former entire Probe deadline.
            time.sleep(11)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()

        def chunk(delta, finish=None, usage=None):
            value = {"id": "fixture", "object": "chat.completion.chunk", "model": body["model"],
                     "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
            if usage:
                value["usage"] = usage
            self.wfile.write(("data: " + json.dumps(value) + "\n\n").encode())
            self.wfile.flush()

        try:
            text = str(body["messages"][-1].get("content", ""))
            images = sum(p.get("type") == "image_url" for m in body["messages"]
                         if isinstance(m.get("content"), list) for p in m["content"])
            if "until cancelled" in text:
                for _ in range(100):
                    chunk({"content": "streaming "})
                    time.sleep(.01)
            elif "dominant color of this image" in text:
                chunk({"content": "blue" if images == 1 and self.server.mode != "reject-images" else "unseen"})
                chunk({}, "stop")
            elif self.server.mode == "screenshots" and any(t["function"]["name"] == "builtin__codemode__execute" for t in body.get("tools", [])):
                self.server.images.append(images)
                count = sum(m["role"] == "tool" for m in body["messages"])
                if count < 2:
                    source = 'const app=await cua.getApp("com.apple.TextEdit");const shot=await app.getScreenshot();'
                    source += 'image(shot.image);' if count == 1 else 'text("Observed without selecting pixels.");'
                    chunk({"tool_calls": [{"index": 0, "id": f"screenshot-{count}", "type": "function", "function": {"name": "builtin__codemode__execute", "arguments": json.dumps({"source": source})}}]})
                    chunk({}, "tool_calls")
                else:
                    chunk({"content": "Screenshot selection verified."})
                    chunk({}, "stop")
            elif self.server.mode == "tasks":
                names = [item["function"]["name"] for item in body.get("tools", [])]
                root = any("general Worker executing" in str(m.get("content", "")) for m in body["messages"])
                runtime = any("Runtime task update (untrusted Worker data" in str(m.get("content", "")) for m in body["messages"])
                has_result = any(m["role"] == "tool" for m in body["messages"])
                if runtime and not root:
                    chunk({"content": "Main Task follow-up through Pi."})
                elif not has_result and (root or "builtin__task__manage" in names):
                    name = "builtin__artifact__search" if root else "builtin__task__manage"
                    arguments = {} if root else {"operation": "create", "task": "Prepare a safe independent report. Return evidence; perform no external actions."}
                    chunk({"tool_calls": [{"index": 0, "id": "task_call", "type": "function", "function": {"name": name, "arguments": json.dumps(arguments)}}]})
                else:
                    chunk({"content": "Root business result through Pi." if root else "Main Task accepted through Pi."})
                chunk({}, "stop", {"prompt_tokens": 7, "completion_tokens": 8, "total_tokens": 15})
            elif self.server.mode == "memory-disabled":
                if not any(m["role"] == "tool" for m in body["messages"]):
                    requests = [("builtin__memory__manage", {"operation": "remember", "content": "I prefer tea"}),
                                ("builtin__memory__search", {"query": "tea"})]
                    chunk({"tool_calls": [{"index": i, "id": f"disabled-{i}", "type": "function",
                                           "function": {"name": name, "arguments": json.dumps(arguments)}}
                                          for i, (name, arguments) in enumerate(requests)]})
                    chunk({}, "tool_calls")
                else:
                    chunk({"content": "Memory requires configuration."})
                    chunk({}, "stop")
            elif self.server.mode == "partial":
                chunk({"content": "partial"})
                return
            elif body.get("tools") and not any(m["role"] == "tool" for m in body["messages"]):
                if self.server.mode == "no-tools":
                    chunk({"content": "I cannot call tools"})
                    chunk({}, "stop")
                else:
                    names = [item["function"]["name"] for item in body["tools"]]
                    name = "builtin__artifact__search" if "builtin__artifact__search" in names else names[0]
                    arguments = {} if name == "builtin__artifact__search" else {"value": "OK"}
                    chunk({"tool_calls": [{"index": 0, "id": "call_echo", "type": "function",
                                          "function": {"name": name, "arguments": json.dumps(arguments)}}]})
                    chunk({}, "tool_calls")
            else:
                chunk({"content": "PROBE_RESULT_OK" if any(m["role"] == "tool" for m in body["messages"])
                       else "ABCDEFGHIJKLMNOPQRSTUVWXYZ" if "long output" in text else "OK"})
                chunk({}, "stop", {"prompt_tokens": 7, "completion_tokens": 8, "total_tokens": 15})
            self.wfile.write(b"data: [DONE]\n\n")
        except (BrokenPipeError, ConnectionResetError):
            pass
        finally:
            self.close_connection = True


def verify(binary, script):
    require(shutil.which("node") is not None, "Node is missing")
    require((script.parent / "node_modules/@earendil-works/pi-ai").is_dir(), "run npm ci --prefix provider-bridge")
    server = ThreadingHTTPServer(("127.0.0.1", 0), Endpoint)
    server.daemon_threads = True
    server.requests, server.mode = [], "success"
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    report = {"bridge": "pi-ai-1.3.0-v1", "checks": []}
    try:
        with tempfile.TemporaryDirectory(prefix="erii-local-") as directory:
            root = Path(directory)
            endpoint = f"http://127.0.0.1:{server.server_port}/v1"
            env = {k: v for k, v in os.environ.items() if not k.startswith("ERII_")}
            env["ERII_DATA_ROOT"] = str(root / "data")
            env["ERII_FIXTURE_KEY"] = "SYNTHETIC_LOCAL_KEY_NEVER_PERSIST"
            config = root / "config.yaml"
            config.write_text("version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: " + json.dumps(str(script)) + "\n  provider_bridge_version: pi-ai-1.3.0-v1\n")

            def command(*args, ok=True, text=None):
                p = subprocess.run([str(binary), "-config", str(config), *args], cwd=root, env=env,
                                   input=text, text=True, capture_output=True, timeout=20)
                require((p.returncode == 0) == ok, f"CLI {args[0:2]} unexpected result: {p.stderr}")
                require("SYNTHETIC_LOCAL_KEY_NEVER_PERSIST" not in p.stdout + p.stderr, "credential in CLI output")
                return json.loads(p.stdout) if ok else p.stderr

            require(command("status")["readiness"] == "setup_required", "implicit model")
            require(command("model", "discover", endpoint)[0]["model_id"] == "fixture-model", "catalog")
            profile = command("model", "add", endpoint, "fixture-model")
            id_ = profile["profile_id"]
            command("model", "select", id_, "1", "1", ok=False)
            require(command("status")["readiness"] == "setup_required", "unverified activation")
            proof = command("model", "test", id_)
            require(all(proof["result"].values()), "probe did not exercise every behavior")
            require(command("model", "select", id_, "1", "1")["readiness"] == "ready", "select")
            require(command("status")["readiness"] == "ready", "restart readiness")
            command("chat", "send", "local-main", text="actual local conversation")
            command("chat", "work")
            history = command("chat", "history")
            require([m["role"] for m in history] == ["user", "assistant"], "local Main history")
            require(history[-1]["content"][0]["text"] == "PROBE_RESULT_OK", "local Main did not continue after Direct Tool")
            require(any(m["role"] == "system" for r in server.requests if r.get("body") for m in r["body"].get("messages", [])), "Main System Prompt missing from actual wire")
            report["main_history"] = history
            surface = next(r["body"]["tools"] for r in reversed(server.requests) if r.get("body", {}).get("tools") and any(t["function"]["name"] == "builtin__artifact__search" for t in r["body"]["tools"]))
            names = [t["function"]["name"] for t in surface]
            require(sorted(n for n in names if n.startswith("builtin__artifact__")) == ["builtin__artifact__manage", "builtin__artifact__search"], "production Artifact surface not consolidated")
            require("builtin__text__echo" not in names and "mcp__fixture__echo" not in names, "fixture echo leaked to production")
            require("builtin__memory__search" in names and "builtin__memory__manage" in names, "Memory tools hidden")
            report["production_surface"] = surface
            server.mode = "memory-disabled"
            command("chat", "send", "disabled-memory", text="Remember that I prefer tea and search for tea.")
            command("chat", "work")
            with closing(sqlite3.connect(root / "data/state.db")) as db:
                failures = db.execute("SELECT tool_name,status,error_json FROM tool_invocation WHERE tool_name LIKE 'builtin__memory__%' ORDER BY tool_name").fetchall()
                require(len(failures) == 2 and all(row[1] == "failed" and json.loads(row[2])["code"] == "PROVIDER_UNAVAILABLE" for row in failures), "unconfigured Memory falsely succeeded")
                require(db.execute("SELECT count(*) FROM memory_mutation").fetchone()[0] == 0, "disabled memory queued a mutation")
            report["unconfigured_memory"] = failures
            report["checks"].append("production manage/search schemas; fixture echo absent; discoverable unconfigured Memory fails without mutation")
            server.mode = "success"
            report["checks"].append("actual CLI Main Actor/Loop/Direct Tool/Conversation through local Node/Pi with System Prompt")
            require(all(r["auth"] is None for r in server.requests), "fabricated Authorization")
            report["checks"] += ["catalog", "manual CLI add", "unverified rejection", "real five-part probe", "atomic select", "restart ready", "no fabricated key"]

            server.mode = "tasks"
            task_start = len(server.requests)
            command("chat", "send", "local-task", text="Delegate an independent report in the background.")
            command("chat", "work")
            task_history = command("chat", "history")
            require(task_history[-1]["content"][0]["text"] == "Main Task follow-up through Pi.", "actual Pi Task follow-up missing")
            task_requests = [r["body"] for r in server.requests[task_start:] if r.get("body")]
            roots = [r for r in task_requests if any("general Worker executing" in str(m.get("content", "")) for m in r["messages"])]
            require(roots and all(not any(t["function"]["name"].startswith("builtin__task__") or t["function"]["name"] == "builtin__memory__manage" for t in r["tools"]) for r in roots), "Root Task or Memory control exposed on actual Pi wire")
            require(all(any(t["function"]["name"] == "builtin__memory__search" for t in r["tools"]) for r in roots), "Root read-only Memory search missing")
            require(all("actual local conversation" not in json.dumps(r["messages"]) for r in roots), "Root inherited Main Conversation")
            require(any(any(m["role"] == "user" and "Runtime task update (untrusted Worker data" in str(m.get("content", "")) for m in r["messages"]) for r in task_requests), "runtime framing lost at actual Pi boundary")
            with closing(sqlite3.connect(root / "data/state.db")) as db:
                require(db.execute("SELECT count(*) FROM task WHERE status='completed'").fetchone()[0] == 1, "actual Pi Task missing")
                require(db.execute("SELECT count(*) FROM main_inbox").fetchone()[0] == 1, "actual Pi Reporter dedup missing")
            report["task_requests"] = task_requests
            report["task_history"] = task_history
            report["checks"].append("actual Node/Pi Task create -> Root echo/result -> runtime-framed Reporter -> one Main follow-up; role/context isolation")

            # A failed retest must revoke proof and close ingress after endpoint failure.
            server.mode = "no-tools"
            require("MODEL_INCOMPATIBLE" in command("model", "test", id_, ok=False), "missing tools accepted")
            require(command("status")["readiness"] == "degraded", "failed retest left ingress open")
            report["checks"].append("failed retest revokes active proof")

            server.mode = "manual"
            require("MODEL_NOT_FOUND" in command("model", "discover", endpoint, ok=False), "missing catalog")
            manual = command("model", "add", endpoint, "manual-id")["profile_id"]
            require(all(command("model", "test", manual)["result"].values()), "manual model probe")
            report["checks"].append("manual ID independent of catalog")

            server.mode = "success"
            auth = command("model", "login", "ERII_FIXTURE_KEY")
            keyed = command("model", "add", endpoint, "fixture-model", auth["connection_id"])["profile_id"]
            start = len(server.requests)
            command("model", "catalog", keyed)
            command("model", "test", keyed)
            require(all(r["auth"] == "Bearer " + env["ERII_FIXTURE_KEY"] for r in server.requests[start:]), "key missing/wrong")
            del env["ERII_FIXTURE_KEY"]
            require("AUTH_REQUIRED" in command("model", "test", keyed, ok=False), "missing key not rejected")
            report["checks"] += ["explicit environment credential", "missing credential rejection"]

            server.mode = "partial"
            require("INVALID_PROVIDER_RESPONSE" in command("model", "test", manual, ok=False), "partial stream completion")
            report["checks"].append("partial stream is not completion")

            with closing(sqlite3.connect(root / "data/state.db")) as db:
                require(db.execute("pragma user_version").fetchone()[0] == STATE_SCHEMA_VERSION, "schema")
                rows = db.execute("SELECT snapshot_id,run_id,input_tokens,output_tokens,latency_ms,error_code,snapshot_json FROM provider_usage").fetchall()
                require(rows and any(r[2:4] == (7, 8) for r in rows), "usage not recorded")
                require(any(r[5] == "GENERATION_CANCELLED" and r[2] is None for r in rows), "unknown cancellation usage fabricated")
                require(any(r[5] == "INVALID_PROVIDER_RESPONSE" for r in rows), "failed usage missing")
                require(all(r[4] >= 0 and json.loads(r[6])["snapshot_id"] == r[0] for r in rows), "snapshot attribution")
                require("SYNTHETIC_LOCAL_KEY_NEVER_PERSIST" not in "\n".join(db.iterdump()), "credential in SQLite")
                report["usage"] = [{"snapshot_id": r[0], "run_id": r[1], "input_tokens": r[2], "output_tokens": r[3], "latency_ms": r[4], "error_code": r[5]} for r in rows]
            for file in (root / "data/logs").glob("*"):
                require(b"SYNTHETIC_LOCAL_KEY_NEVER_PERSIST" not in file.read_bytes(), "credential in log")
            report["checks"] += ["SQLite usage and failure readback", "no credentials in DB or logs"]
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=3)
    report["cleanup"] = "all CLI children reaped; endpoint stopped; disposable state removed"
    return report


def verify_screenshot_projection(binary, script):
    """Normal CLI Test/Select -> Run -> explicit image -> actual Pi HTTP.

    Uses the existing Computer fixture for Guest mechanics; no database edits
    can inject capabilities into the profile or its frozen snapshot.
    """
    from verify_computer_use import launch
    server = ThreadingHTTPServer(("127.0.0.1", 0), Endpoint)
    server.requests, server.images, server.mode = [], [], "screenshots"
    server.daemon_threads = True
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    report = {"checks": [], "cleanup": False}
    try:
        for enabled in (True, False):
            server.images.clear()
            with tempfile.TemporaryDirectory(prefix="erii-image-profile-") as temporary:
                with launch(binary, Path(temporary).resolve()) as h:
                    config = json.loads((h.root / "config.yaml").read_text())
                    config["llm"] = {"provider_bridge_command": "node", "provider_bridge_script": str(script), "provider_bridge_version": "pi-ai-1.3.0-v1"}
                    (h.root / "config.yaml").write_text(json.dumps(config))
                    h.args = lambda *a: [str(h.binary), "-config", str(h.root / "config.yaml"), *a]
                    metadata = {"name": "Screenshot profile", "provider_kind": "openai_compatible", "endpoint": f"http://127.0.0.1:{server.server_port}/v1", "wire_api": "openai_completions", "model_id": "fixture-model", "context_window": 65536, "max_output_tokens": 4096, "compatibility": {"tool_result_format": "openai"}, "capabilities": {"text_input": True, "image_input": enabled}}
                    profile = h.setup(metadata=metadata)
                    h.send("screenshot-profile", "Observe two screenshots; explicitly select only the second image.")
                    h.command("chat", "work")
                    with closing(sqlite3.connect(Path(h.env["ERII_DATA_ROOT"]) / "state.db")) as db:
                        requests = [json.loads(r[0]) for r in db.execute("SELECT request_json FROM model_generation ORDER BY created_at")]
                        require(len(requests) == 3, "screenshot Run exchanges")
                        require(all(r["model_snapshot"]["capabilities"]["image_input"] == enabled for r in requests), "normally selected image capability lost or gained")
                        selected = [sum(len(m.get("image_refs", [])) for m in r["messages"]) for r in requests]
                        require(selected == [0, 0, 1], "explicit screenshot selection not persisted")
                        require("base64" not in json.dumps(requests), "pixels entered durable generation requests")
                    require(server.images == ([0, 0, 1] if enabled else [0, 0, 0]), "selected screenshot pixels missing or text-only fallback leaked pixels")
                    report["checks"].append({"image_input": enabled, "selected_refs": selected, "provider_image_counts": list(server.images)})
                    if enabled:
                        server.mode = "reject-images"
                        failed = h.command("model", "test", profile, ok=False)
                        require("MODEL_INCOMPATIBLE" in failed.stderr, "failed image probe admitted")
                        require(h.command("status")["readiness"] == "degraded", "failed image retest left ingress open")
                        server.mode = "screenshots"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=3)
    report["cleanup"] = True
    return report


def verify_live_probe(binary, script, endpoint, model, context_tokens, output_tokens):
    """Opt-in local Live Shadow. Never selects a profile or opens user state."""
    from urllib.parse import urlsplit
    url = urlsplit(endpoint)
    require(url.scheme in ("http", "https") and url.hostname in ("127.0.0.1", "localhost", "::1")
            and not (url.username or url.password or url.query or url.fragment), "live probe requires a local endpoint without credentials")
    with tempfile.TemporaryDirectory(prefix="erii-live-probe-") as directory:
        root = Path(directory)
        env = {k: v for k, v in os.environ.items() if not k.startswith("ERII_")}
        env["ERII_DATA_ROOT"] = str(root / "data")
        config = root / "config.yaml"
        config.write_text("version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: " + json.dumps(str(script)) + "\n  provider_bridge_version: pi-ai-1.3.0-v1\n")

        def command(*args, text=None):
            return subprocess.run([str(binary), "-config", str(config), *args], cwd=root, env=env,
                                  input=text, text=True, capture_output=True, timeout=800)

        require(json.loads(command("status").stdout)["readiness"] == "setup_required", "live probe state isolation failed")
        catalog = command("model", "discover", endpoint)
        require(catalog.returncode == 0 and any(m["model_id"] == model for m in json.loads(catalog.stdout)), "live model not discovered")
        profile = command("model", "add", text=json.dumps({"name": model, "provider_kind": "openai_compatible",
            "endpoint": endpoint, "wire_api": "openai_completions", "model_id": model,
            "context_window": context_tokens, "max_output_tokens": output_tokens,
            "compatibility": {"tool_result_format": "openai"}, "capabilities": {"text_input": True}}))
        require(profile.returncode == 0, "live profile metadata rejected")
        started = time.monotonic()
        probe = command("model", "test", json.loads(profile.stdout)["profile_id"])
        elapsed = time.monotonic() - started
        # Only normalized Host errors/proof and content-free usage; never model output.
        with closing(sqlite3.connect(root / "data/state.db")) as db:
            usage = [dict(zip(("latency_ms", "error_code"), row)) for row in db.execute("SELECT latency_ms,error_code FROM provider_usage ORDER BY rowid")]
            require(db.execute("SELECT count(*) FROM run").fetchone()[0] == 0, "probe created Agent work")
        report = {"model": model, "context_tokens": context_tokens, "output_tokens": output_tokens,
                  "catalog": "discovered", "elapsed_seconds": round(elapsed, 3), "exit_code": probe.returncode,
                  "proof": json.loads(probe.stdout) if probe.returncode == 0 else None,
                  "error": probe.stderr.strip() if probe.returncode else None, "usage": usage,
                  "readiness": json.loads(command("status").stdout)["readiness"]}
    report["cleanup"] = "CLI children reaped; isolated state removed; no active profile selected"
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", default="bin/erii")
    parser.add_argument("--evidence", default="evidence/local-provider.json")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--live-endpoint', help='opt-in local Live Shadow; isolated Probe only')
    parser.add_argument('--live-model')
    parser.add_argument('--context-tokens', type=int, default=262144)
    parser.add_argument('--output-tokens', type=int, default=32768)
    args = parser.parse_args()
    Path(args.evidence).unlink(missing_ok=True)
    if args.live_endpoint:
        require(args.live_model, '--live-model is required')
        result = verify_live_probe(Path(args.binary).resolve(), Path('provider-bridge/bridge.mjs').resolve(),
                                   args.live_endpoint, args.live_model, args.context_tokens, args.output_tokens)
    else:
        result = verify(Path(args.binary).resolve(), Path('provider-bridge/bridge.mjs').resolve())
        result['screenshot_projection'] = verify_screenshot_projection(Path(args.binary).resolve(), Path('provider-bridge/bridge.mjs').resolve())
    Path(args.evidence).parent.mkdir(parents=True, exist_ok=True)
    Path(args.evidence).write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"checks": len(result.get("checks", [])), "usage_rows": len(result["usage"]), "cleanup": result["cleanup"]}))
