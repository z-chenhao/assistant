#!/usr/bin/env python3
"""Default-launch PTY byte boundaries, overflow recovery, and mode restoration."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import signal
import tempfile
import termios

from terminal_driver import Terminal
from verify_main import MainHarness, require


def verify(binary):
    report = {"version": 1, "platform": platform.platform(), "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
              "scope": "Actual terminal -> Main byte preservation, rejection and recovery; explicit Fake, no model-quality claim", "cases": []}
    samples = [("review-ascii", "BEGIN-" + "x" * 2000 + "-END", False),
               ("review-utf8", "BEGIN-" + "困" * 600 + "-END", False),
               ("ascii-limit-with-echo", "x" * 16384, True),
               ("utf8-below-limit", "困" * 5461, False),
               ("utf8-limit", "困" * 5461 + "x", False),
               ("ascii-overflow", "x" * 16385, False),
               ("utf8-overflow", "困" * 5461 + "xy", False),
               ("large-overflow", "困" * 22000, False)]
    with tempfile.TemporaryDirectory(prefix="collie-terminal-input-") as tmp:
        root = Path(tmp)
        for name, text, echo in samples:
            folder = root / name
            folder.mkdir()
            h = MainHarness(binary, folder, fixture="terminal-input")
            # Isolate the 16 KiB input boundary from model context capacity.
            # Current input plus frozen rules/Tool schemas must fit unchanged.
            # Small-profile rejection is covered by context regression tests.
            h.setup(metadata=dict(name='terminal byte-boundary fixture', provider_kind='openai_compatible',
                                  endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions',
                                  model_id='fixture-model', context_window=32768, max_output_tokens=1024,
                                  generation_defaults={}, compatibility=dict(tool_result_format='openai'),
                                  capabilities=dict(text_input=True)))
            with Terminal(h.args(), h.env, h.root, echo=echo) as t:
                t.expect("You> ")
                require(not termios.tcgetattr(t.input_observer)[3] & termios.ICANON, "kernel canonical line limit still enabled")
                require((fcntl.fcntl(t.input_observer, fcntl.F_GETFL) & os.O_NONBLOCK) == (t.original_flags & os.O_NONBLOCK), "input changed stdout's shared nonblocking flag")
                t.send(text)
                accepted = len(text.encode()) <= 16384
                if accepted:
                    try:
                        t.expect("assistant: "); t.expect("You> ")
                    except RuntimeError as error:
                        failed = h.state()
                        raise RuntimeError(f"{name}: runs {[(r['status'], r['error_code']) for r in failed['runs']]}; tools {len(failed['tools'])}; generations {[(g['status'], g['error_code']) for g in failed['generations']]}") from error
                    state = h.state()
                    users = [m for m in state["conversation"] if m["role"] == "user"]
                    require(len(users) == 1 and users[0]["content"][0]["text"] == text, "valid input lost or changed")
                    require(len(state["inputs"]) == 1 and all(r["status"] == "completed" for r in state["runs"]),
                            f"{name}: input did not complete through Main: {[(r['status'], r['error_code']) for r in state['runs']]}")
                else:
                    t.expect("Input rejected: line exceeds 16 KiB"); t.expect("You> ")
                    before = h.state()
                    require(not before["inputs"] and not before["conversation"] and not before["runs"], "oversized prefix reached domain state")
                    t.send("recovery control"); t.expect("assistant: Input received."); t.expect("You> ")
                    state = h.state()
                    users = [m for m in state["conversation"] if m["role"] == "user"]
                    require(len(users) == 1 and users[0]["content"][0]["text"] == "recovery control", "overflow poisoned next input")
                require(b"\x07" not in t.output, "terminal dropped bytes with bells")
                require(t.stop() == 0, "terminal quit failed")
                report["cases"].append({"name": name, "input_bytes": len(text.encode()), "accepted": accepted,
                                        "echo": echo, "original_termios_restored": True, "state": state, "transcript": t.transcript()})

        for echo in (False, True):
            folder = root / f"editing-{echo}"
            folder.mkdir()
            h = MainHarness(binary, folder)
            h.setup()
            with Terminal(h.args(), h.env, h.root, echo=echo) as t:
                t.expect("You> ")
                t.send_bytes(("困惑\x7f醒\n").encode())
                t.expect("assistant: Processed: 困醒"); t.expect("You> ")
                t.send_bytes(("x" * 17000 + "\x15cleared control\n").encode())
                t.expect("assistant: Processed: cleared control"); t.expect("You> ")
                t.send_bytes(b"\xffinvalid\n")
                t.expect("Input rejected: line is not valid UTF-8"); t.expect("You> ")
                t.send("after invalid UTF-8"); t.expect("assistant: Processed: after invalid UTF-8"); t.expect("You> ")
                state = h.state()
                users = [m["content"][0]["text"] for m in state["conversation"] if m["role"] == "user"]
                require(users == ["困醒", "cleared control", "after invalid UTF-8"], "editing/invalid byte recovery changed semantic input")
                t.send_bytes(b"\x04")
                require(t.process.wait(timeout=20) == 0, "Ctrl-D did not terminate")
                # stop() also verifies termios and flags, even for an exited child.
                require(t.stop(signal.SIGTERM) == 0, "EOF restoration failed")
                report["cases"].append({"name": f"editing-{echo}", "echo": echo, "state": state, "original_termios_restored": True, "transcript": t.transcript()})

        for sig in (signal.SIGINT, signal.SIGTERM):
            folder = root / f"signal-{sig}"
            folder.mkdir()
            h = MainHarness(binary, folder)
            h.setup()
            with Terminal(h.args(), h.env, h.root, echo=True) as t:
                t.expect("You> "); t.send_bytes("unfinished 困".encode())
                require(t.stop(sig) == 0, "partial input signal shutdown failed")
                state = h.state()
                require(not state["inputs"] and not state["conversation"], "unfinished line accepted during shutdown")
                report["cases"].append({"name": f"signal-{sig}", "original_termios_restored": True, "state": state, "transcript": t.transcript()})
    report["cleanup"] = "owned PTYs and children closed; isolated state removed"
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    report = verify(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print("Terminal input PTY checks passed: " + str(len(report["cases"])))
