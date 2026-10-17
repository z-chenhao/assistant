#!/usr/bin/env python3
"""Profile an unchanged Python harness, including blocking child/PTY time.

Only function identities and durations are saved; never arguments or locals.
Used by verify_suite.py and by the baseline PATH wrapper documented in ci.md.
"""
import cProfile
import json
import os
from pathlib import Path
import pstats
import runpy
import sys
import time


def main():
    script = Path(sys.argv[1]).resolve()
    sys.argv = sys.argv[1:]
    sys.path.insert(0, str(script.parent))
    profile = cProfile.Profile()
    started = time.monotonic()
    try:
        profile.enable()
        runpy.run_path(str(script), run_name="__main__")
    finally:
        profile.disable()
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
                      functions=sorted(functions, key=lambda f: -f["cumulative_seconds"]))
        # Each invocation owns its directory (including repeated stability runs).
        (folder / (script.stem + ".json")).write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
