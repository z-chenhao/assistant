#!/usr/bin/env python3
"""Require both providers parts, retaining the original complete gate inventory."""
import argparse
from collections import Counter
import json
from pathlib import Path

from ci_codemode import IDENTITY
from verify_suite import PROVIDER_PARTS, SHARDS, provider_gates


def join(root):
    suites, rows = {}, []
    expected = [(g.name, i) for g in SHARDS['providers'] for i in range(1, g.attempts + 1)]
    if Counter((g.name, i) for part in PROVIDER_PARTS for g in provider_gates(part)
               for i in range(1, g.attempts + 1)) != Counter(expected):
        raise ValueError('providers part inventory drift')
    for part in PROVIDER_PARTS:
        suite = json.loads((root / part / f'suite-providers-{part}.json').read_text())
        if (suite.get('passed') is not True or suite.get('source_stable') is not True or
                suite.get('source_dirty') is not False or suite.get('selected_gates') is not None or
                suite.get('provider_part') != part or suite.get('complete_inventory') is not False or
                set(suite.get('shards', {})) != {'providers'}):
            raise ValueError('failed, dirty or wrong providers part: ' + part)
        members = suite['shards']['providers']
        planned = provider_gates(part)
        if (Counter((r['gate'], r['attempt']) for r in members) != Counter(
                (g.name, i) for g in planned for i in range(1, g.attempts + 1)) or
                any(r.get('returncode') != 0 or r.get('owned_group_drained') is not True for r in members)):
            raise ValueError('missing, duplicate or failed providers repetition: ' + part)
        for gate in planned:
            series = [r for r in members if r['gate'] == gate.name]
            if [r['attempt'] for r in series] != list(range(1, gate.attempts + 1)):
                raise ValueError('reordered providers repetition: ' + part)
            rows.extend(series)
        suites[part] = suite
    first = suites[PROVIDER_PARTS[0]]
    if (not first.get('source_sha') or not first.get('binary_sha256') or
            any(any(s.get(key) != first.get(key) for key in IDENTITY) for s in suites.values())):
        raise ValueError('providers source/build identity drift')
    order = {pair: i for i, pair in enumerate(expected)}
    rows.sort(key=lambda r: order[r['gate'], r['attempt']])
    return dict(first, complete_inventory=True, provider_part=None,
                shards={'providers': rows}, parts=suites,
                elapsed_seconds=max(s['elapsed_seconds'] for s in suites.values()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parts', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    output = args.evidence / 'suite-providers.json'
    output.unlink(missing_ok=True)
    full = join(args.parts)
    output.write_text(json.dumps(full, indent=2) + '\n')
    print(json.dumps(dict(passed=True, source_sha=full['source_sha'],
                          gates=[dict(gate=g.name, attempts=g.attempts,
                                      wall_seconds=sum(r['elapsed_seconds'] for r in full['shards']['providers']
                                                       if r['gate'] == g.name))
                                 for g in SHARDS['providers']])))


if __name__ == '__main__':
    main()
