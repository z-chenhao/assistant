#!/usr/bin/env python3
"""Join all three fixed security parts; preserve every original gate receipt."""
import argparse
from collections import Counter
import json
import hashlib
import os
from pathlib import Path
import re

from ci_codemode import IDENTITY
from verify_suite import SECURITY_PARTS, SHARDS, security_gates, report_name, source_state

SECURITY_IDENTITY = (*IDENTITY, 'run_id', 'run_attempt')


def validate(suites, source=None, run=None):
    if set(suites) != set(SECURITY_PARTS):
        raise ValueError('exactly all three security parts required')
    expected = [(g.name, n) for g in SHARDS['security'] for n in range(1, g.attempts + 1)]
    if Counter((g.name, n) for p in SECURITY_PARTS for g in security_gates(p)
               for n in range(1, g.attempts + 1)) != Counter(expected):
        raise ValueError('security part inventory drift')
    rows = []
    first = suites[SECURITY_PARTS[0]]
    for part in SECURITY_PARTS:
        suite = suites[part]
        if (suite.get('passed') is not True or suite.get('source_stable') is not True or
                suite.get('source_dirty') is not False or suite.get('selected_gates', []) is not None or
                suite.get('complete_inventory') is not False or suite.get('security_part') != part or
                set(suite.get('shards', {})) != {'security'} or
                any(suite.get(k) is not None for k in ('codemode_part', 'personal_part', 'provider_part', 'communication_part')) or
                suite.get('jobs') != 1 or suite.get('gate_jobs') != 2):
            raise ValueError('failed, focused, dirty or wrong security part: ' + part)
        if (any(k not in suite or suite[k] != first.get(k) for k in SECURITY_IDENTITY) or
                not re.fullmatch('[0-9a-f]{40}', suite.get('source_sha', '')) or
                not re.fullmatch('[0-9a-f]{64}', suite.get('binary_sha256', '')) or
                suite.get('source_diff_sha256') != hashlib.sha256(b'').hexdigest() or
                (source is not None and any(suite.get(k) != v for k, v in source.items())) or
                (run is not None and any(suite.get(k) != v for k, v in run.items()))):
            raise ValueError('security source/build/workflow/run/attempt drift')
        members = suite['shards']['security']
        gates = security_gates(part)
        if ([(r['gate'], r['attempt']) for r in members] !=
                [(g.name, n) for g in gates for n in range(1, g.attempts + 1)] or
                any(type(r['attempt']) is not int or type(r.get('returncode')) is not int or
                    r['returncode'] != 0 or r.get('owned_group_drained') is not True for r in members)):
            raise ValueError('missing, duplicate, reordered, failed or unclean security repetition')
        # Freeze the original harness arguments as well as its repetition count.
        for row in members:
            gate = next(g for g in gates if g.name == row['gate'])
            command = row.get('command', [])
            if (len(command) != 7 + len(gate.arguments) or
                    command[1:4] != ['scripts/profile_harness.py', 'scripts/' + gate.script, '--binary'] or
                    command[5] != '--evidence' or Path(command[6]).name != report_name(gate, row['attempt']) or
                    command[7:] != list(gate.arguments)):
                raise ValueError('security command/parameter drift')
        rows.extend(members)
    order = {pair: i for i, pair in enumerate(expected)}
    rows.sort(key=lambda r: order[r['gate'], r['attempt']])
    return dict(first, complete_inventory=True, security_part=None, shards={'security': rows},
                parts=suites, elapsed_seconds=max(s['elapsed_seconds'] for s in suites.values()),
                semantic_score=None, release_ready=False)


def join(root):
    if {p.name for p in root.iterdir()} != set(SECURITY_PARTS):
        raise ValueError('missing or unexpected security part directory')
    suites = {part: json.loads((root / part / f'suite-security-{part}.json').read_text())
              for part in SECURITY_PARTS}
    run = {key: os.environ.get(env) for key, env in
           (('workflow_sha', 'GITHUB_SHA'), ('run_id', 'GITHUB_RUN_ID'), ('run_attempt', 'GITHUB_RUN_ATTEMPT'))}
    return validate(suites, source_state(), run)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parts', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    output = args.evidence / 'suite-security.json'
    output.unlink(missing_ok=True)
    full = join(args.parts)
    output.write_text(json.dumps(full, indent=2) + '\n')
    print(json.dumps(dict(passed=True, source_sha=full['source_sha'],
                          invocations=len(full['shards']['security']), semantic_score=None, release_ready=False)))


if __name__ == '__main__':
    main()
