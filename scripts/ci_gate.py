#!/usr/bin/env python3
"""Fail closed unless all required CI jobs succeeded on this workflow run."""
import json
import os
import sys

REQUIRED = {"fast", "race", "verify"}


def passed(needs):
    return (isinstance(needs, dict) and set(needs) == REQUIRED and
            all(isinstance(value, dict) and value.get("result") == "success"
                for value in needs.values()))


if __name__ == "__main__":
    needs = json.loads(os.environ["NEEDS_JSON"])
    print(json.dumps(needs, sort_keys=True))
    sys.exit(0 if passed(needs) else 1)
