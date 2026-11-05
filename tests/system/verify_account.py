#!/usr/bin/env python3
"""Launch/doctor/drive/evidence/cleanup for current OAuth through the real CLI.

An owned in-memory SecretStore and local OAuth/Responses server replace external
dependencies; the production Go -> Node -> Pi and SQLite paths remain real.
No credentials or private authorization URLs are written to evidence.
"""
import argparse
import base64
from collections.abc import MutableMapping
from contextlib import contextmanager
import json
import os
import re
from pathlib import Path
import select
import shutil
import signal
import socketserver
from contextlib import closing
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from codemode_peer import runtime_yaml


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


@contextmanager
def native_secret_service():
    """Own a private session bus and password-protected disposable GNOME keyring."""
    require(sys.platform == 'linux', 'native Secret Service fixture requires Linux')
    for command in ('dbus-daemon', 'gnome-keyring-daemon', 'secret-tool', 'gdbus'):
        require(shutil.which(command), 'native Secret Service fixture dependency missing: ' + command)
    with tempfile.TemporaryDirectory(prefix='steward-keyring-') as directory:
        root = Path(directory)
        runtime = root / 'runtime'
        runtime.mkdir(mode=0o700)
        env = dict(os.environ, XDG_DATA_HOME=str(root / 'data'), XDG_CONFIG_HOME=str(root / 'config'),
                   XDG_CACHE_HOME=str(root / 'cache'), XDG_RUNTIME_DIR=str(runtime))
        for key in ('DISPLAY', 'WAYLAND_DISPLAY', 'DBUS_SESSION_BUS_ADDRESS',
                    'DBUS_STARTER_ADDRESS', 'DBUS_STARTER_BUS_TYPE', 'GNOME_KEYRING_CONTROL', 'SSH_AUTH_SOCK'):
            env.pop(key, None)
        children = []
        try:
            bus = subprocess.Popen(['dbus-daemon', '--session', '--nofork', '--nopidfile', '--print-address=1'],
                                   env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
            children.append(bus)
            require(select.select([bus.stdout], [], [], 5)[0], 'private session bus launch timed out')
            address = bus.stdout.readline().strip()
            require(address.startswith('unix:'), 'private session bus unavailable')
            env['DBUS_SESSION_BUS_ADDRESS'] = address
            env['GNOME_KEYRING_CONTROL'] = str(root / 'control')
            def start_keyring():
                keyring = subprocess.Popen(['gnome-keyring-daemon', '--foreground', '--unlock', '--components=secrets',
                                            '--control-directory=' + str(root / 'control')], env=env,
                                           stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
                children.append(keyring)
                keyring.stdin.write(b'owned-synthetic-keyring-password')
                keyring.stdin.close()
                keyring.stdin = None
                deadline = time.monotonic() + 10
                while True:
                    require(keyring.poll() is None, 'private keyring exited during startup')
                    owner = subprocess.run(['gdbus', 'call', '--session', '--dest', 'org.freedesktop.DBus',
                                            '--object-path', '/org/freedesktop/DBus', '--method',
                                            'org.freedesktop.DBus.NameHasOwner', 'org.freedesktop.secrets'],
                                           env=env, capture_output=True, timeout=3)
                    if owner.returncode == 0 and owner.stdout.strip() == b'(true,)':
                        break
                    require(time.monotonic() < deadline, 'private keyring startup timed out')
                    time.sleep(.05)
                return keyring

            keyring = start_keyring()
            env['STEWARD_TEST_SECRET_SERVICE_PID'] = str(keyring.pid)
            def restart_keyring():
                nonlocal keyring
                os.killpg(keyring.pid, signal.SIGTERM)
                keyring.communicate(timeout=5)
                keyring = start_keyring()
                env['STEWARD_TEST_SECRET_SERVICE_PID'] = str(keyring.pid)
            yield env, restart_keyring
        finally:
            for child in reversed(children):
                if child.poll() is None:
                    os.killpg(child.pid, signal.SIGTERM)
                try:
                    child.communicate(timeout=1)
                except subprocess.TimeoutExpired:
                    os.killpg(child.pid, signal.SIGKILL)
                    child.communicate(timeout=1)


class NativeSecrets(MutableMapping):
    """Independent libsecret CLI inspection of this fixture's owned native items.

    The application's production SecretStore never uses this helper. Only test
    expiry control and private assertions read/write the synthetic credentials.
    """
    def __init__(self, database, env, sensitive):
        self.database, self.env, self.sensitive = database, env, sensitive

    def __getitem__(self, ref):
        result = subprocess.run(['secret-tool', 'lookup', 'xdg:schema', 'ai.steward.Credential',
                                 'service', 'steward', 'credential', ref], env=self.env, capture_output=True, timeout=5)
        if result.returncode == 1 and not result.stdout and not result.stderr:
            raise KeyError(ref)
        require(result.returncode == 0, 'owned native credential inspection failed')
        data = result.stdout.removesuffix(b'\n')
        require(data.startswith(b'steward-secret-v1:'), 'owned native credential format invalid')
        value = data[len(b'steward-secret-v1:'):].decode('ascii')
        credential = json.loads(base64.b64decode(value))
        for name in ('access_token', 'refresh_token', 'id_token'):
            if credential.get(name):
                self.sensitive.add(credential[name].encode())
        return value

    def __setitem__(self, ref, value):
        result = subprocess.run(['secret-tool', 'store', '--label=Steward fixture credential',
                                 'xdg:schema', 'ai.steward.Credential', 'service', 'steward', 'credential', ref],
                                env=self.env, input=b'steward-secret-v1:' + value.encode('ascii'), capture_output=True, timeout=5)
        require(result.returncode == 0, 'owned native expiry control failed')

    def __delitem__(self, ref):
        result = subprocess.run(['secret-tool', 'clear', 'xdg:schema', 'ai.steward.Credential',
                                 'service', 'steward', 'credential', ref], env=self.env, capture_output=True, timeout=5)
        require(result.returncode == 0, 'owned native credential deletion failed')

    def __iter__(self):
        if not self.database.exists():
            return
        with closing(sqlite3.connect(self.database)) as db:
            refs = [row[0] for row in db.execute('SELECT credential_ref FROM auth_connection')]
        for ref in refs:
            try:
                self[ref]
                yield ref
            except KeyError:
                pass

    def __len__(self):
        return sum(1 for _ in self)


def verify_locked_keyring(env, source, restart):
    # Only called inside native_secret_service(); never lock a user's session.
    ref = 'credential:steward-native-locked'
    original = b'steward-secret-v1:' + base64.b64encode(b'synthetic-locked-fixture')
    attributes = ['xdg:schema', 'ai.steward.Credential', 'service', 'steward', 'credential', ref]
    stored = subprocess.run(['secret-tool', 'store', '--label=Steward isolated fixture', *attributes],
                            env=env, input=original, capture_output=True, timeout=5)
    require(stored.returncode == 0, 'private locked-keyring fixture unavailable')
    call = ['gdbus', 'call', '--session', '--dest', 'org.freedesktop.secrets',
            '--object-path', '/org/freedesktop/secrets', '--method']
    alias = subprocess.run([*call, 'org.freedesktop.Secret.Service.ReadAlias', 'default'],
                           env=env, capture_output=True, text=True, timeout=5)
    match = re.fullmatch(r"\(objectpath '(/org/freedesktop/secrets/collection/[A-Za-z0-9_]+)',\)\s*", alias.stdout)
    require(alias.returncode == 0 and match, 'private collection alias unavailable')
    locked = subprocess.run([*call, 'org.freedesktop.Secret.Service.Lock', "[objectpath '" + match[1] + "']"],
                            env=env, capture_output=True, timeout=5)
    require(locked.returncode == 0, 'private keyring lock failed')
    tests = subprocess.run(['go', 'test', './internal/llm/secret', '-run', '^TestServiceLockedDenied$', '-count=1', '-timeout', '20s'],
                           env=dict(env, STEWARD_TEST_SECRET_SERVICE_LOCKED='1'), cwd=source,
                           capture_output=True, timeout=30)
    require(tests.returncode == 0, 'locked-keyring operations did not fail closed')
    # Restart the owned daemon with the original password. This also proves
    # persistence across keyring processes, beyond separate Steward processes.
    restart()
    read = subprocess.run(['secret-tool', 'lookup', *attributes], env=env, capture_output=True, timeout=5)
    require(read.returncode == 0 and read.stdout.removesuffix(b'\n') == original,
            'locked-keyring operations changed the stored credential')


class Secrets(socketserver.StreamRequestHandler):
    def handle(self):
        r = json.loads(self.rfile.readline(131072))
        with self.server.lock:
            op, ref = r["op"], r["ref"]
            if op == "put":
                self.server.values[ref] = r["value"]
                credential = json.loads(base64.b64decode(r["value"]))
                for name in ["access_token", "refresh_token", "id_token"]:
                    if credential.get(name):
                        self.server.sensitive.add(credential[name].encode())
            elif op == "delete":
                self.server.values.pop(ref, None)
            value = self.server.values.get(ref)
        self.wfile.write((json.dumps({"ok": op != "get" or value is not None, "value": value}) + "\n").encode())


def account_config(root, bridge, endpoint, binary=None):
    """Route the native Go bridge to the explicitly owned loopback peer."""
    binary = Path(binary or bridge.parent.parent / 'bin/steward').resolve()
    wrapper = root / "bridge.mjs"
    wrapper.write_text("import {spawn} from 'node:child_process';const child=spawn(" + json.dumps(str(binary)) + ",[\"-test-native-model-bridge\"," + json.dumps(endpoint) + "],{stdio:['pipe','pipe','inherit']});process.stdin.pipe(child.stdin);child.stdout.pipe(process.stdout);child.stdin.on('error',()=>{});for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{child.stdin.end();child.kill(signal)});child.on('error',()=>{process.exitCode=1});child.on('exit',code=>{process.exitCode=code??1});\n")
    cfg = root / "config.yaml"
    cfg.write_text(f"version: 1\napp:\n  data_root: {root / 'data'}\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: {wrapper}\n  provider_bridge_version: pi-ai-1.3.0-v1\n")
    cfg.write_text(cfg.read_text()+runtime_yaml())
    return cfg


def verify(binary, bridge, native_env=None):
    require(shutil.which("node"), "Node is missing")
    report = {"protocol": "current-openai-account", "live_shadow": "inconclusive", "device_authorization": "deferred_by_approval", "checks": []}
    report['credential_backend'] = 'native-linux-secret-service' if native_env is not None else 'owned-memory-socket'
    fixture = subprocess.Popen(["node", str(bridge.parent / "test/account-fixture.mjs")], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    require(select.select([fixture.stdout], [], [], 10)[0], "fixture launch timed out")
    endpoint = fixture.stdout.readline().strip()
    children = []
    try:
        with tempfile.TemporaryDirectory(prefix="ea-", dir="/tmp") as directory:
            root = Path(directory)
            socket = root / "secrets.sock"
            server = socketserver.ThreadingUnixStreamServer(str(socket), Secrets)
            server.values, server.lock, server.sensitive = {}, threading.Lock(), set()
            os.chmod(socket, 0o600)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                cfg = account_config(root, bridge, endpoint, binary)
                env = {k: v for k, v in os.environ.items() if not k.startswith("STEWARD_")}
                args = [str(binary), "-config", str(cfg)]
                if native_env is None:
                    args += ['-test-secret-store', str(socket)]
                else:
                    env = {k: v for k, v in native_env.items() if not k.startswith('STEWARD_')}
                    server.values = NativeSecrets(root / 'data/state.db', env, server.sensitive)
                ordinary_output = []

                def api(path, data=None):
                    request = urllib.request.Request(endpoint + path, data=None if data is None else json.dumps(data).encode(), headers={"Content-Type": "application/json"})
                    with urllib.request.urlopen(request, timeout=20) as response:
                        return json.load(response)

                def run(*command, ok=True, text=""):
                    p = subprocess.run(args + list(command), env=env, text=True, input=text, capture_output=True, timeout=25)
                    ordinary_output.append((p.stdout + p.stderr).encode())
                    if native_env is not None:
                        list(server.values)  # Register issued synthetic tokens for leak assertions.
                    require((p.returncode == 0) == ok, "CLI unexpected exit: " + " ".join(command[:2]) + " " + p.stderr[:100])
                    if not ok:
                        require("fixture-access-" not in p.stderr and "fixture-refresh-" not in p.stderr, "credential in error")
                        return None
                    return json.loads(p.stdout)

                def login(account=None):
                    cmd = args + ["model", "login-chatgpt"] + ([account] if account else [])
                    p = subprocess.Popen(cmd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                    children.append(p)
                    require(select.select([p.stdout], [], [], 10)[0], "login URL timeout")
                    event = json.loads(p.stdout.readline())
                    require(event["kind"] == "open_url", "missing UI-neutral auth URL")
                    callback = api("/fixture/authorize", {"url": event["url"]})["callback"]
                    with urllib.request.urlopen(callback, timeout=5) as response:
                        require(response.status == 200, "callback rejected")
                    output, error = p.communicate(timeout=20)
                    require(p.returncode == 0, "login failed " + error[:100])
                    # Authorization URLs are private interaction data. Only final
                    # account metadata and non-URL events are ordinary output.
                    ordinary_output.append(output.encode())
                    return json.loads(output.splitlines()[-1])

                require(run("status")["readiness"] == "setup_required", "implicit provider")
                auth = login()
                aid = auth["connection_id"]
                require(auth["status"] == "connected" and "credential_ref" not in auth, "unsafe account metadata")
                require(len(run("model", "accounts")) == 1, "account picker missing")
                catalog = run("model", "account-catalog", aid)
                require([m["model_id"] for m in catalog] == ["fixture-account-model"], "dynamic visibility filter")
                run("model", "account-add", aid, "hidden-model", ok=False)
                profile = run("model", "account-add", aid, catalog[0]["model_id"])
                probe = run("model", "test", profile["profile_id"])
                require(all(probe["result"].values()), "incomplete account capability probe")
                status = run("model", "select", profile["profile_id"], "1", "1")
                require(status["readiness"] == "ready", "account activation failed")
                require(run("status")["agent_ingress_enabled"], "restart lost readiness")
                report["checks"].append("browser PKCE, verified identity, dynamic model picker, actual Pi Responses probe, activation and restart")
                run("chat", "send", "account-main", text="actual account Main conversation")
                run("chat", "work")
                history = run("chat", "history")
                require([m["role"] for m in history] == ["user", "assistant"] and history[-1]["content"][0]["text"] == "PROBE_RESULT_OK", "account Main/Direct Tool continuation failed")
                with closing(sqlite3.connect(root / "data/state.db")) as main_db, main_db:
                    require(main_db.execute("SELECT count(*) FROM run WHERE status='completed'").fetchone()[0] == 1, "account Main not complete")
                    require(main_db.execute("SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__artifact_search' AND status='succeeded'").fetchone()[0] == 1, "account nested business tool not persisted")
                    require(main_db.execute("SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__codemode_execute' AND status='succeeded'").fetchone()[0] == 1, "account model bypassed Code Mode")
                report["main_history"] = history
                report["checks"].append("actual account CLI Main Actor/Loop/Direct Tool/Conversation and frozen current Responses snapshot")

                # Force near-expiry solely in the in-memory SecretStore, then race
                # separate Host/Bridge processes against the rotating session.
                ref = next(iter(server.values))
                with server.lock:
                    value = json.loads(base64.b64decode(server.values[ref]))
                    value["expires_at"] = "2000-01-01T00:00:00Z"
                    server.values[ref] = base64.b64encode(json.dumps(value).encode()).decode()
                before = api("/fixture/stats")["refreshes"]
                racing = [subprocess.Popen(args + ["model", "refresh", aid], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) for _ in range(6)]
                children.extend(racing)
                for p in racing:
                    output, error = p.communicate(timeout=25)
                    ordinary_output.append((output + error).encode())
                    require(p.returncode == 0, "refresh race failed")
                require(api("/fixture/stats")["refreshes"] == before + 1, "refresh was not single flight")
                require(run("status")["readiness"] == "ready", "successful refresh lost capability proof")
                report["checks"].append("six real processes share one rotating refresh; proof and readiness preserved")

                def reactivate():
                    current = next(p for p in run("model", "list") if p["profile_id"] == profile["profile_id"])
                    config_version = run("status")["config_version"]
                    run("model", "test", current["profile_id"])
                    require(run("model", "select", current["profile_id"], str(current["version"]), str(config_version))["readiness"] == "ready", "repair activation failed")

                before_generation = api("/fixture/stats")["generations"]
                api("/fixture/control", {"models": ["new-account-model"]})
                run("model", "test", profile["profile_id"], ok=False)
                require(api("/fixture/stats")["generations"] == before_generation, "removed account model was inferred")
                require(run("status")["readiness"] == "degraded", "model removal retained readiness")
                api("/fixture/control", {"models": ["fixture-account-model"]})
                reactivate()
                for mode in ["partial", "incomplete"]:
                    api("/fixture/control", {"mode": mode})
                    run("model", "test", profile["profile_id"], ok=False)
                    require(run("status")["readiness"] == "degraded", "incomplete response accepted")
                    api("/fixture/control", {"mode": "success"})
                    reactivate()
                report["checks"].append("removed model rejected before inference; partial and incomplete Responses cannot complete or retain readiness")

                api("/fixture/control", {"mode": "auth-loss"})
                run("model", "account-catalog", aid, ok=False)
                require(run("model", "accounts")[0]["status"] == "reauth_required" and not server.values, "known auth loss retained credentials")
                require(run("status")["readiness"] == "degraded", "known auth loss retained readiness")
                api("/fixture/control", {"mode": "success"})
                login(aid)
                reactivate()
                report["checks"].append("actual 401 revokes account credentials, model proof and readiness; explicit reauthorization repairs connection")

                api("/fixture/control", {"mode": "hold-generation"})
                old_generations = api("/fixture/stats")["generations"]
                old_closed = api("/fixture/stats")["closed"]
                active = subprocess.Popen(args + ["model", "test", profile["profile_id"]], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                children.append(active)
                for _ in range(200):
                    if api("/fixture/stats")["generations"] > old_generations:
                        break
                    time.sleep(.01)
                else:
                    raise RuntimeError("active account stream missing")
                run("model", "logout", aid)
                output, error = active.communicate(timeout=10)
                ordinary_output.append((output + error).encode())
                require(active.returncode != 0 and not output, "logout published an active generation result")
                require(api("/fixture/stats")["closed"] > old_closed, "other Host did not cancel revoked account stream")
                api("/fixture/control", {"mode": "success"})
                login(aid)
                reactivate()
                report["checks"].append("logout in another Host cancels active Pi stream and fences its final result")

                dbpath = root / "data/state.db"
                def readback(query, *params):
                    with closing(sqlite3.connect(dbpath)) as db, db:
                        return db.execute(query, params).fetchall()

                stable_device = readback("SELECT device_id FROM installation_auth")[0][0]
                # Kill after persisted refreshing, while the peer holds the request.
                api("/fixture/control", {"mode": "hold-refresh"})
                with server.lock:
                    value = json.loads(base64.b64decode(server.values[ref]));value["expires_at"] = "2000-01-01T00:00:00Z"
                    server.values[ref] = base64.b64encode(json.dumps(value).encode()).decode()
                p = subprocess.Popen(args + ["model", "refresh", aid], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                children.append(p)
                for _ in range(200):
                    if readback("SELECT status FROM auth_connection WHERE connection_id=?", aid)[0][0] == "refreshing" and api("/fixture/stats")["refreshes"] == before + 2:
                        break
                    time.sleep(.01)
                else:
                    raise RuntimeError("refresh checkpoint missing")
                p.send_signal(signal.SIGKILL);p.communicate(timeout=10)
                status = run("status")
                require(status["readiness"] == "degraded", "crash did not close ingress")
                require(readback("SELECT status FROM auth_connection")[0][0] == "reauth_required", "interrupted rotation not recovered")
                require(not server.values, "orphaned refreshing secret survived recovery")
                report["checks"].append("SIGKILL during token rotation -> restart, fail closed without replay, credential removal and degraded readiness")

                api("/fixture/control", {"mode": "success"})
                again = login(aid)
                require(again["connection_id"] == aid, "reauthorization changed identity")
                require(readback("SELECT device_id FROM installation_auth")[0][0] == stable_device, "installation identity changed")
                run("model", "logout", aid)
                require(not server.values, "logout left credentials")
                require(run("model", "accounts")[0]["status"] == "logged_out", "logout state missing")
                run("model", "logout", aid)
                run("model", "refresh", aid, ok=False)
                report["checks"].append("same-account reauthorization, stable private installation identity, idempotent logout and blocked reuse")

                with server.lock:
                    credential_material = list(server.sensitive)
                private_markers = credential_material + [b"fixture-access-", b"fixture-refresh-", b"fixture-secret-marker", stable_device.encode()]
                require(not any(secret in output for output in ordinary_output for secret in private_markers), "credential or private device identity in ordinary CLI output")
                for path in (root / "data").rglob("*"):
                    if path.is_file() and path.name != "state.db" and path.suffix not in (".lock",):
                        raw = path.read_bytes()
                        require(not any(v in raw for v in private_markers), "private material in log/WAL/trace")
                # device_id is intentionally non-secret configuration in state.db;
                # all credential material is forbidden there and in its WAL/SHM.
                for path in (root / "data").glob("state.db*"):
                    require(not any(v in path.read_bytes() for v in credential_material + private_markers[-4:-1]), "credential persisted in SQLite")
                report["checks"].append("all issued access/refresh/ID tokens absent from SQLite/WAL/SHM, ordinary CLI output, logs and traces; private device ID absent from ordinary output")
                stats = api("/fixture/stats")
                require(stats["request_valid"], "unsupported Responses wire fields")
                report["provider_counts"] = {k: v for k, v in stats.items() if isinstance(v, int)}
                report["schema_version"] = readback("PRAGMA user_version")[0][0]
                report["usage_rows"] = readback("SELECT count(*) FROM provider_usage")[0][0]
                require(report["usage_rows"] >= 5, "missing account generation readback")
            finally:
                for p in children:
                    if p.poll() is None:
                        p.kill();p.communicate(timeout=10)
                server.shutdown();server.server_close();thread.join(timeout=5)
    finally:
        fixture.terminate();fixture.communicate(timeout=20)
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument('--native-secret-service', action='store_true',
                        help='Linux: own an isolated D-Bus/keyring and use the production credential backend')
    options = parser.parse_args()
    bridge = Path(__file__).resolve().parents[2] / 'provider-bridge/bridge.mjs'
    if options.native_secret_service:
        with native_secret_service() as (env, restart):
            test_env = dict(env, STEWARD_TEST_SECRET_SERVICE='1')
            native = subprocess.run(['go', 'test', './internal/llm/secret', '-run', 'TestService', '-count=1', '-timeout', '30s'],
                                    env=test_env, capture_output=True, text=True, timeout=60,
                                    cwd=bridge.parent.parent)
            require(native.returncode == 0, 'native credential component checks failed')
            result = verify(options.binary.resolve(), bridge, env)
            verify_locked_keyring(env, bridge.parent.parent, restart)
            result['checks'].append('locked keyring rejects bounded read/write/delete without diagnostics or credential changes; explicit unlock restores access')
            result['native_component_checks'] = 'passed'
            result['native_fixture'] = 'private session bus and password-protected disposable GNOME keyring'
    else:
        result = verify(options.binary.resolve(), bridge)
    options.evidence.parent.mkdir(parents=True, exist_ok=True)
    options.evidence.write_text(json.dumps(result, indent=2) + "\n")
    print("Current OpenAI account CLI checks passed: " + str(len(result["checks"])))
