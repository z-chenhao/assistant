#!/usr/bin/env python3
"""Launch, doctor, drive, capture, and clean up the real foundation CLI.

Uses only Python's standard library. No user data, accounts, ports, or services.
"""

import argparse
from contextlib import closing
import json
import os
from pathlib import Path
import selectors
import signal
import sqlite3
import subprocess
import tempfile


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def snapshot(data_root):
    with closing(sqlite3.connect((data_root / "state.db").as_uri() + "?mode=ro", uri=True)) as db:
        return {
            "schema_version": db.execute("PRAGMA user_version").fetchone()[0],
            "journal_mode": db.execute("PRAGMA journal_mode").fetchone()[0],
            "rows": db.execute("SELECT * FROM agent_config").fetchall(),
            "dump": list(db.iterdump()),
        }


def diagnostics(value):
    require(value["readiness"] == "setup_required", "incorrect readiness")
    require(value["agent_ingress_enabled"] is False, "ingress enabled without a model")
    require(value["active_model_profile_id"] is None, "implicit model enabled")
    require(value["schema_version"] == 2, "incorrect schema version")


class Harness:
    def __init__(self, binary, root):
        self.binary = binary
        self.root = root
        self.data_root = root / "data with # and ?"
        self.env = {k: v for k, v in os.environ.items() if not k.startswith("ERII_")}
        self.env["ERII_DATA_ROOT"] = str(self.data_root)
        self.config = root / "config.yaml"
        self.yaml_root = root / "unused-config-root"
        self.config.write_text(f"version: 1\napp:\n  data_root: {json.dumps(str(self.yaml_root))}\n  log_level: info\n", encoding="utf-8")
        self.process = None

    def doctor(self):
        require(self.binary.is_file() and os.access(self.binary, os.X_OK), "build binary first")
        result = self.command("-help")
        require(result.returncode == 0 and "Usage:" in result.stderr, "CLI help unavailable")
        require(not self.data_root.exists(), "doctor unexpectedly created application state")

    def command(self, *args):
        return subprocess.run([str(self.binary), "-config", str(self.config), *args], cwd=self.root, env=self.env,
                              text=True, capture_output=True, timeout=15, check=False)

    def launch(self):
        self.process = subprocess.Popen([str(self.binary), "-config", str(self.config)], cwd=self.root, env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        with selectors.DefaultSelector() as selector:
            selector.register(self.process.stdout, selectors.EVENT_READ)
            require(selector.select(timeout=15), "startup diagnostics timed out")
        line = self.process.stdout.readline()
        require(bool(line), "process exited before startup diagnostics")
        value = json.loads(line)
        diagnostics(value)
        require(self.process.poll() is None, "default command did not remain running")
        return value

    def stop(self, sig=signal.SIGTERM):
        self.process.send_signal(sig)
        out, err = self.process.communicate(timeout=15)
        code = self.process.returncode
        self.process = None
        require(not out and not err, "unexpected shutdown output")
        return code

    def cleanup(self):
        if self.process is not None:
            if self.process.poll() is None:
                self.process.kill()
            self.process.communicate(timeout=15)
            self.process = None


def verify(binary):
    with tempfile.TemporaryDirectory(prefix="erii-verify-") as temp:
        harness = Harness(binary, Path(temp))
        checks = []
        try:
            harness.doctor()
            checks.append("doctor: binary and help ready; no state created")
            first = harness.launch()
            stored = snapshot(harness.data_root)
            require(not harness.yaml_root.exists(), "environment did not override YAML data root")
            require(stored["schema_version"] == 2 and stored["journal_mode"] == "wal", "SQLite setup incorrect")
            require(len(stored["rows"]) == 1, "bootstrap is not a singleton")
            require(stored["rows"][0][1] == first["agent_id"], "CLI disagrees with stored identity")
            live_status = harness.command("status")
            require(live_status.returncode == 0 and json.loads(live_status.stdout) == first,
                    "status failed while foreground process was running")
            require(harness.stop() == 0, "SIGTERM did not exit cleanly")
            checks.append("launch/drive: YAML and environment override, fresh database, setup_required, closed ingress, live status, clean SIGTERM")

            repeated = harness.command("status")
            require(repeated.returncode == 0 and not repeated.stderr, "status command failed")
            require(json.loads(repeated.stdout) == first, "restart changed diagnostics")
            require(snapshot(harness.data_root) == stored, "restart changed stored data")
            checks.append("drive: status restart preserves all rows, timestamps, schema, and identity")

            require(harness.launch() == first, "launch changed diagnostics")
            require(harness.stop(signal.SIGKILL) == -signal.SIGKILL, "kill injection failed")
            restarted = harness.command("status")
            require(restarted.returncode == 0, "restart after kill failed")
            require(json.loads(restarted.stdout) == first, "kill/restart changed diagnostics")
            require(snapshot(harness.data_root) == stored, "kill/restart changed stored data")
            checks.append("drive: SIGKILL then restart preserves committed bootstrap state")

            logs = [json.loads(line) for line in (harness.data_root / "logs/app.jsonl").read_text().splitlines()]
            traces = [json.loads(line) for line in (harness.data_root / "logs/traces.jsonl").read_text().splitlines()]
            startup = [s for s in traces if s["name"] == "app.startup"]
            require(len(startup) == 5, "missing startup traces")
            for parent in startup:
                require(any(child["parent_span_id"] == parent["span_id"] and child["trace_id"] == parent["trace_id"]
                            for child in traces), "missing database span correlation")
                require(any(log.get("trace_id") == parent["trace_id"] for log in logs), "missing log correlation")
            checks.append("evidence: valid JSON logs and correlated startup/database spans")

            with closing(sqlite3.connect(harness.data_root / "state.db")) as db:
                db.execute("PRAGMA user_version = 99")
            future_before = snapshot(harness.data_root)
            rejected = harness.command("status")
            require(rejected.returncode != 0 and not rejected.stdout, "future schema was accepted")
            require("unsupported database schema version 99" in rejected.stderr, "future schema diagnostic missing")
            require(snapshot(harness.data_root) == future_before, "future schema rejection changed data")
            failure_traces = [json.loads(line) for line in (harness.data_root / "logs/traces.jsonl").read_text().splitlines()]
            new_spans = failure_traces[len(traces):]
            require(len(new_spans) == 2 and all(s["status"] == "error" for s in new_spans),
                    "failed startup did not record error spans")
            traces = failure_traces
            checks.append("drive: future schema fails closed without changing schema or rows; error spans recorded")

            invalid = harness.root / "invalid.yaml"
            invalid.write_text("version: 1\napi_key: SECRET_SENTINEL\n", encoding="utf-8")
            rejected = harness.command("-config", str(invalid), "status")
            require(rejected.returncode != 0 and not rejected.stdout, "invalid YAML was accepted")
            require("SECRET_SENTINEL" not in rejected.stderr, "config value leaked in error")
            checks.append("drive: unknown configuration rejected without echoing values")
            return {"result": "passed", "scope": "M0 foundation and event slice; M0 incomplete",
                    "checks": checks, "diagnostics": first, "stored_rows": stored["rows"],
                    "log_records": len(logs), "trace_records": len(traces),
                    "logs": logs, "traces": traces}
        finally:
            harness.cleanup()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    # Never leave a stale passing report after a failed verification.
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.unlink(missing_ok=True)
    report = verify(args.binary.resolve())
    report["checks"].append("cleanup: exact child processes reaped; isolated state removed")
    args.evidence.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({key: value for key, value in report.items() if key not in ("logs", "traces")}, indent=2))
    print(f"Full log/span evidence: {args.evidence}")


if __name__ == "__main__":
    main()
