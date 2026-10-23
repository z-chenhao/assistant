#!/usr/bin/env python3
"""Launch/doctor/drive/evidence/cleanup of actual TUI via an owned PTY.

Both provider paths use the real Node/Pi Bridge and deterministic HTTP peers.
No online account, saved credentials, desktop, or test-only UI is used.
"""
import argparse
import json
import os
from pathlib import Path
import re
import select
import signal
import socketserver
import sqlite3
import subprocess
import tempfile
import threading
import urllib.request
from http.server import ThreadingHTTPServer

from terminal_driver import Terminal
from verify_account import Secrets, account_config
from verify_local_provider import Endpoint
from verify_main import MainHarness, assert_final, require
from event_broker import Broker


class TUIHarness(MainHarness):
    def __init__(self, binary, root, config, secrets=None, broker=None):
        super().__init__(binary, root)
        self.config, self.secrets = config, secrets
        if broker is not None:
            self.env.update(ERII_NATS_URL=broker.url, ERII_NATS_STREAM_PREFIX='tui_'+root.name)

    def args(self, *args):
        flags = [str(self.binary), "-config", str(self.config)]
        if self.secrets:
            flags += ["-test-secret-store", str(self.secrets)]
        return flags + list(args)

    def terminal(self, *flags):
        # No subcommand: prove actual default terminal entry, not only `tui`.
        return Terminal(self.args(*flags), self.env, self.root)


def local_setup(t, endpoint, model="fixture-model", manual=False, invalid_limits=False):
    t.expect("Connection [1/2]"); t.send("1")
    t.expect("Endpoint URL"); t.expect("> "); t.send(endpoint)
    t.expect("API key environment variable"); t.expect("> "); t.send("")
    if manual:
        t.expect("Catalog unavailable.")
    else:
        t.expect("Model: fixture-model")
    t.expect("Model ID"); t.expect("> "); t.send(model if manual else "")
    t.expect("Context tokens"); t.expect("> ")
    if invalid_limits:
        t.send("bad"); t.expect("Enter a whole number from 1 to 1048576.")
        t.expect("Context tokens"); t.expect("> ")
    t.send("262144" if invalid_limits else "")
    t.expect("Max output tokens"); t.expect("> ")
    if invalid_limits:
        t.send("65537"); t.expect("Enter a whole number from 1 to 65536.")
        t.expect("Max output tokens"); t.expect("> ")
    t.send("32768" if invalid_limits else "")
    for stage in ("text and streaming", "Tool calls", "Tool result continuation", "cancellation", "output clipping"):
        t.expect("Checking " + stage + "…")
    t.expect("Probe passed:")
    t.expect("Type yes"); t.expect("> "); t.send("yes")
    t.expect("Model selected."); t.expect("You> ")


def verify(binary, bridge):
    report = {"version": 1, "checks": [], "live_shadow": "inconclusive; unexecuted"}
    with tempfile.TemporaryDirectory(prefix="et-", dir="/tmp") as tmp, Broker(Path(tmp)) as broker:
        root = Path(tmp)
        server = ThreadingHTTPServer(("127.0.0.1", 0), Endpoint)
        server.daemon_threads = True
        server.requests, server.mode = [], "success"
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            for manual in (False, True):
                folder = root / ("manual" if manual else "discovery")
                folder.mkdir()
                cfg = folder / "config.yaml"
                cfg.write_text(f"version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: {bridge}\n  provider_bridge_version: pi-ai-1.3.0-v1\n")
                h = TUIHarness(binary, folder, cfg, broker=broker)
                server.mode = "manual" if manual else "success"
                with h.terminal() as t:
                    t.expect("State: setup_required")
                    local_setup(t, f"http://127.0.0.1:{server.server_port}/v1", manual=manual)
                    t.send("terminal Main conversation")
                    t.expect("assistant: PROBE_RESULT_OK"); t.expect("You> ")
                    state = h.state()
                    assert_final(state, 1)
                    require(len(state["tools"]) == 1, "terminal bypassed Direct Tool")
                    require(t.stop(signal.SIGKILL) == -signal.SIGKILL, "kill evidence missing")
                    transcript = t.transcript()
                with h.terminal() as t:
                    t.expect("State: ready")
                    t.expect("assistant: PROBE_RESULT_OK"); t.expect("You> ")
                    require(h.state() == state, "reopen duplicated semantic state")
                    t.send("/models"); t.expect("fixture-model ready"); t.expect("You> ")
                    t.send("/setup"); t.expect("Connection [1/2]"); t.send("/cancel")
                    t.expect("Setup cancelled."); t.expect("You> ")
                    require(h.command("status")["readiness"] == "ready", "cancel cleared active model")
                    pid = h.command("model", "list")[0]["profile_id"]
                    t.send("/select " + pid); t.expect("Type yes"); t.expect("> "); t.send("/cancel")
                    t.expect("Setup cancelled."); t.expect("You> ")
                    # Failed Probe must revoke readiness. Repair stays explicit.
                    server.mode = "no-tools"
                    t.send("/test " + pid); t.expect("Connection test failed: model probe Tool calls: MODEL_INCOMPATIBLE")
                    t.expect("Retry with /test " + pid); t.expect("You> ")
                    t.send("must reject after failed probe"); t.expect("State: degraded."); t.expect("You> ")
                    require(h.state() == state, "failed Probe accepted new Main work")
                    server.mode = "manual" if manual else "success"
                    t.send("/test " + pid); t.expect("Probe passed."); t.expect("You> ")
                    t.send("/select " + pid); t.expect("Type yes"); t.expect("> ")
                    # Another writer wins while confirmation is open. The
                    # captured version must reject this stale action.
                    d = h.command("status")
                    current = h.command("model", "list")[0]
                    h.command("model", "select", pid, str(current["version"]), str(d["config_version"]))
                    t.send("yes"); t.expect("Operation failed: metadata version conflict"); t.expect("You> ")
                    require(h.state() == state, "profile controls changed old Main evidence")
                    require(t.stop() == 0, "quit failed")
                    transcript += t.transcript()
                report[folder.name] = {"transcript": transcript, "state": state, "restart": "SIGKILL; same history and authoritative rows"}
                # Remove the configured Bridge: selected proof must not enable ingress.
                degraded = folder / "degraded.yaml"
                degraded.write_text("version: 1\n")
                h.config = degraded
                with h.terminal("tui") as t:
                    t.expect("State: degraded"); t.expect("assistant: PROBE_RESULT_OK"); t.expect("You> ")
                    t.send("must not create work"); t.expect("Use /setup to repair"); t.expect("You> ")
                    require(h.state() == state, "degraded ingress created work")
                    require(t.stop(signal.SIGTERM) == 0, "prompt shutdown hung or errored")
                    report[folder.name]["degraded_transcript"] = t.transcript()
                report["checks"].append(folder.name + ": actual PTY setup -> Pi Probe -> select -> Main -> pure Tool -> reply; SIGKILL reopen; failed Probe/degraded ingress; selection cancellation and stale confirmation conflict")

            folder = root / "cold-local"
            folder.mkdir()
            cfg = folder / "config.yaml"
            cfg.write_text(f"version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: {bridge}\n  provider_bridge_version: pi-ai-1.3.0-v1\n")
            h = TUIHarness(binary, folder, cfg, broker=broker)
            server.mode = "slow"
            with h.terminal() as t:
                local_setup(t, f"http://127.0.0.1:{server.server_port}/v1", invalid_limits=True)
                profile = h.command("model", "list")[0]
                require(profile["context_window"] == 262144 and profile["max_output_tokens"] == 32768, "setup changed accepted limits")
                require(h.command("status")["readiness"] == "ready", "cold model setup not ready")
                require(t.stop() == 0, "cold model terminal did not close")
                report["cold_local"] = {"transcript": t.transcript(), "profile": profile}
            report["checks"].append("actual PTY single-model Enter picker; invalid numeric fields retry in place; 11-second cold Pi response passes all checks with visible progress; exact 262144/32768 limits retained")

            folder = root / "cancel"
            folder.mkdir()
            cfg = folder / "config.yaml"
            cfg.write_text("version: 1\n")
            h = TUIHarness(binary, folder, cfg, broker=broker)
            with h.terminal() as t:
                t.expect("Connection [1/2]"); t.send("/cancel")
                t.expect("Setup cancelled."); t.expect("You> ")
                t.send("no implicit provider"); t.expect("Use /setup to repair"); t.expect("You> ")
                require(h.command("status")["readiness"] == "setup_required", "cancel enabled ingress")
                with sqlite3.connect(folder / "data/state.db") as db:
                    require(db.execute("SELECT count(*) FROM run").fetchone()[0] == 0, "cancel created a Run")
                require(t.stop(signal.SIGTERM) == 0, "idle prompt shutdown failed")
                report["cancel"] = t.transcript()
            report["checks"].append("fresh setup cancellation leaves setup_required; no implicit provider or Run; SIGTERM at idle prompt joins input reader")
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=5)

        folder = root / "steering"
        folder.mkdir()
        h = MainHarness(binary, folder)
        old = h.setup()
        with Terminal(h.args("-test-main-hold", "after_generation_commit", "tui"), h.env, h.root) as t:
            t.expect("You> "); t.send("original terminal work")
            t.expect('"boundary":"after_generation_commit"')
            t.send("terminal steering one"); t.expect("Erii: busy"); t.expect("You> ")
            t.send("terminal steering two"); t.expect("Erii: busy"); t.expect("You> ")
            before = h.state()
            require(len(before["runs"]) == 1 and len(before["inputs"]) == 3, "busy terminal failed to route Steering to current Main")
            new = h.setup("new-model")
            require(new != old, "profile switch missing")
            require(t.stop(signal.SIGKILL) == -signal.SIGKILL, "held terminal kill missing")
        with Terminal(h.args("tui"), h.env, h.root) as t:
            t.expect("assistant: "); t.expect("You> ")
            after = h.state()
            assert_final(after, 3)
            require(all(s["profile_id"] == old for s in after["snapshots"]), "held Main hot-switched model")
            t.send("new terminal run"); t.expect("assistant: "); t.expect("You> ")
            final = h.state()
            assert_final(final, 4, 2)
            require(final["snapshots"][-1]["profile_id"] == new, "new Main ignored selected profile")
            require(t.stop() == 0, "recovered terminal quit failed")
            report["steering"] = {"before": before, "after": after, "final": final, "transcript": t.transcript()}
        report["checks"].append("busy PTY accepts two Steering lines into one held Main; actual SIGKILL restart reuses accepted generation; old Snapshot retained; new Run uses new profile")

        fixture = subprocess.Popen(["node", str(bridge.parent / "test/account-fixture.mjs")], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            require(select.select([fixture.stdout], [], [], 10)[0], "OAuth fixture unavailable")
            endpoint = fixture.stdout.readline().strip()

            def api(path, data):
                request = urllib.request.Request(endpoint + path, data=json.dumps(data).encode(), headers={"Content-Type": "application/json"})
                with urllib.request.urlopen(request, timeout=10) as response:
                    return json.load(response)

            folder = root / "account"
            folder.mkdir()
            sock = folder / "secret.sock"
            secrets = socketserver.ThreadingUnixStreamServer(str(sock), Secrets)
            secrets.values, secrets.lock, secrets.sensitive = {}, threading.Lock(), set()
            os.chmod(sock, 0o600)
            worker = threading.Thread(target=secrets.serve_forever, daemon=True)
            worker.start()
            try:
                cfg = account_config(folder, bridge, endpoint)
                h = TUIHarness(binary, folder, cfg, sock, broker=broker)
                # Signal during login, before any callback/credential acceptance.
                with h.terminal() as t:
                    t.expect("Connection [1/2]"); t.send("2")
                    t.expect("Account ID"); t.expect("> "); t.send("")
                    t.expect("https://auth.openai.com/")
                    require(t.stop(signal.SIGINT) == 0, "login cancellation hung")
                require(not secrets.values and h.command("status")["readiness"] == "setup_required", "cancelled login persisted credentials or activation")
                require(not h.command("model", "accounts"), "cancelled login created account")
                with h.terminal() as t:
                    t.expect("Connection [1/2]"); t.send("2")
                    t.expect("Account ID"); t.expect("> "); t.send("")
                    event = t.expect("\r\n")
                    url = re.search(r"https://auth\.openai\.com/[^\s]+", event)
                    require(url, "private browser URL not presented")
                    callback = api("/fixture/authorize", {"url": url.group()})["callback"]
                    with urllib.request.urlopen(callback, timeout=5) as response:
                        require(response.status == 200, "callback failed")
                    t.expect("Model: fixture-account-model")
                    t.expect("Model ID"); t.expect("> "); t.send("hidden-model")
                    t.expect("Choose a listed model by number or ID."); t.expect("> "); t.send("1")
                    t.expect("Probe passed:"); t.expect("Type yes"); t.expect("> "); t.send("yes")
                    t.expect("Model selected."); t.expect("You> ")
                    t.send("ChatGPT terminal conversation"); t.expect("assistant: PROBE_RESULT_OK"); t.expect("You> ")
                    state = h.state()
                    assert_final(state, 1)
                    require(state["snapshots"][0]["model_id"] == "fixture-account-model", "wrong account model")
                    require(t.stop(signal.SIGKILL) == -signal.SIGKILL, "account kill missing")
                    transcript = t.transcript()
                aid = h.command("model", "accounts")[0]["connection_id"]
                with h.terminal() as t:
                    t.expect("State: ready"); t.expect("assistant: PROBE_RESULT_OK"); t.expect("You> ")
                    require(h.state() == state, "account restart changed state")
                    t.send("/logout " + aid); t.expect("Account logged out."); t.expect("You> ")
                    t.send("blocked after logout"); t.expect("State: degraded."); t.expect("You> ")
                    require(h.state() == state, "logout ingress created work")
                    require(t.stop() == 0, "account quit failed")
                    transcript += t.transcript()
                require(not secrets.values, "logout retained credentials")
                report["account"] = {"transcript": transcript, "state": state, "restart": "SIGKILL; same history and authoritative rows"}
                encoded = json.dumps(report).encode()
                with secrets.lock:
                    sensitive = list(secrets.sensitive)
                require(not any(value in encoded for value in sensitive), "credential in terminal evidence")
                require(b"code_challenge=" not in encoded, "private authorization URL in evidence")
                for path in (folder / "data").rglob("*"):
                    if path.is_file():
                        require(not any(value in path.read_bytes() for value in sensitive), "credential persisted outside SecretStore")
                report["checks"].append("ChatGPT PTY login cancellation; actual browser/PKCE/identity/catalog/Pi Probe/Main/Tool/reply; SIGKILL reopen; logout closes ingress; credentials and private URLs excluded from evidence")
            finally:
                secrets.shutdown(); secrets.server_close(); worker.join(timeout=5)
        finally:
            fixture.terminate(); fixture.communicate(timeout=20)
    report["checks"].append("cleanup: every owned PTY/child/peer/secret socket closed; disposable databases removed")
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    report = verify(args.binary.resolve(), Path(__file__).resolve().parents[1] / "provider-bridge/bridge.mjs")
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print("TUI checks passed: " + str(len(report["checks"])))
