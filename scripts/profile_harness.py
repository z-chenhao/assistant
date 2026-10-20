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
import resource
import signal
import sys
import threading
import time


def cpu():
    own = resource.getrusage(resource.RUSAGE_SELF)
    children = resource.getrusage(resource.RUSAGE_CHILDREN)
    return own.ru_utime + own.ru_stime + children.ru_utime + children.ru_stime


def main():
    script = Path(sys.argv[1]).resolve()
    sys.argv = sys.argv[1:]
    sys.path.insert(0, str(script.parent))
    # Let finally blocks reap owned peers/PTYs on CI cancellation.
    terminating = False
    def terminate(signum, *_):
        nonlocal terminating
        if terminating:
            return
        terminating = True
        # Make and the group owner can both signal this process. A repeated
        # notification must not raise out of the cleanup already in progress.
        raise SystemExit(128 + signum)
    signal.signal(signal.SIGTERM, terminate)
    signal.signal(signal.SIGINT, terminate)
    profile = cProfile.Profile()
    cases = []
    active = {}
    def trace(frame, event, _):
        if event == "call":
            function = frame.f_code.co_name
            if function not in ("scenario", "driver", "execute", "feedback_case", "coding_case", "engineering_case"):
                return None
            file = Path(frame.f_code.co_filename)
            is_cell = function == "execute" and file.name == "verify_codemode.py"
            is_core = function in ("feedback_case", "coding_case", "engineering_case") and file.name == "run_core_cases.py"
            if (function not in ("scenario", "driver") and not is_cell and not is_core) or file.parent != script.parent:
                return None
            values = frame.f_locals
            name = values.get("key") if is_cell else values.get("name")
            if frame.f_code.co_name == "driver":
                name = values["case"]["case_id"] + "/" + values["variant"]["variant_id"]
            if is_core:
                name = function + "/" + values["variant"]["variant_id"]
            active[id(frame)] = (str(name), time.monotonic(), cpu())
            frame.f_trace_lines = False
            return trace
        if event == "return" and id(frame) in active:
            name, start, usage = active.pop(id(frame))
            cases.append(dict(name=name, elapsed_seconds=time.monotonic() - start,
                              cpu_seconds=cpu() - usage))
        return trace
    started = time.monotonic()
    usage = cpu()
    previous_thread_trace = threading.gettrace()
    try:
        sys.settrace(trace)
        threading.settrace(trace)
        profile.enable()
        runpy.run_path(str(script), run_name="__main__")
    finally:
        profile.disable()
        sys.settrace(None)
        threading.settrace(previous_thread_trace)
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
                      cpu_seconds=cpu() - usage,
                      cpu_scope="self and reaped descendants; scenario times may overlap",
                      function_profile_scope="main thread; worker scenarios use traced intervals",
                      cases=cases,
                      functions=sorted(functions, key=lambda f: -f["cumulative_seconds"]))
        # Each invocation owns its directory (including repeated stability runs).
        (folder / (script.stem + ".json")).write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
