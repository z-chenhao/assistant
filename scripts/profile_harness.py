#!/usr/bin/env python3
"""Profile an unchanged Python harness, including blocking child/PTY time.

Only function/scenario identities and durations are saved; never payloads.
Used by verify_suite.py and by the baseline PATH wrapper documented in ci.md.
"""
import cProfile
import json
import os
from pathlib import Path
import pstats
import runpy
import signal
import sys
import time


def main():
    script = Path(sys.argv[1]).resolve()
    sys.argv = sys.argv[1:]
    sys.path.insert(0, str(script.parent))
    # Let finally blocks reap owned peers/PTYs on CI cancellation.
    def terminate(*_):
        raise SystemExit(143)
    signal.signal(signal.SIGTERM, terminate)
    profile = cProfile.Profile()
    cases = []
    active = {}
    def trace(frame, event, _):
        if event == "call":
            is_cell = frame.f_code.co_name == "execute" and Path(frame.f_code.co_filename).name == "verify_codemode.py"
            if (frame.f_code.co_name not in ("scenario", "driver") and not is_cell) or Path(frame.f_code.co_filename).parent != script.parent:
                return None
            values = frame.f_locals
            name = values.get("key") if is_cell else values.get("name")
            if frame.f_code.co_name == "driver":
                name = values["case"]["case_id"] + "/" + values["variant"]["variant_id"]
            active[id(frame)] = (str(name), time.monotonic())
            frame.f_trace_lines = False
            return trace
        if event == "return" and id(frame) in active:
            name, start = active.pop(id(frame))
            cases.append(dict(name=name, elapsed_seconds=time.monotonic() - start))
        return trace
    started = time.monotonic()
    try:
        sys.settrace(trace)
        profile.enable()
        runpy.run_path(str(script), run_name="__main__")
    finally:
        profile.disable()
        sys.settrace(None)
        folder = Path(os.environ["ERII_PROFILE_DIR"])
        folder.mkdir(parents=True, exist_ok=True)
        functions = []
        root = script.parent.parent
        for (file, line, function), (primitive, calls, own, cumulative, _) in pstats.Stats(profile).stats.items():
            try:
                file = str(Path(file).relative_to(root))
            except ValueError:
                continue
            functions.append(dict(file=file, line=line, function=function, calls=calls,
                                  primitive_calls=primitive, own_seconds=own,
                                  cumulative_seconds=cumulative))
        report = dict(harness=script.name, elapsed_seconds=time.monotonic() - started,
                      cases=cases,
                      functions=sorted(functions, key=lambda f: -f["cumulative_seconds"]))
        # Each invocation owns its directory (including repeated stability runs).
        (folder / (script.stem + ".json")).write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
