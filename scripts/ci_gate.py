#!/usr/bin/env python3
"""Fail closed unless all required CI jobs succeeded on this workflow run."""
import json
import os
import sys

REQUIRED = {'fast', 'python_compat', 'race', 'race_join', 'verify', 'codemode', 'codemode_join', 'personal', 'personal_join'}


def passed(needs):
    return (isinstance(needs, dict) and set(needs) == REQUIRED and
            all(isinstance(value, dict) and value.get("result") == "success"
                for value in needs.values()))


if __name__ == "__main__":
    needs = json.loads(os.environ["NEEDS_JSON"])
    print(json.dumps(needs, sort_keys=True))
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        import subprocess
        source = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as summary:
            summary.write(f"Tested source: `{source}`\n\n| Job | Result |\n| --- | --- |\n")
            if isinstance(needs, dict):
                for name, value in sorted(needs.items()):
                    result = value.get("result", "missing") if isinstance(value, dict) else "invalid"
                    summary.write(f"| {name} | {result} |\n")
            summary.write("\nFull merge evidence is required; change impact never skips a full shard.\n")
    sys.exit(0 if passed(needs) else 1)
