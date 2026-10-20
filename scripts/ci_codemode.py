#!/usr/bin/env python3
"""Join all Required Code Mode parts into the unchanged full Core verdict."""
import argparse
from collections import Counter
import copy
import json
from pathlib import Path

import core_report
from verify_suite import CODEMODE_PARTS, SHARDS, codemode_gates, report_name

IDENTITY = ('source_sha', 'source_dirty', 'source_diff_sha256', 'binary_sha256',
            'workflow_sha')


def join(root):
    suites, cores = {}, {}
    for part in CODEMODE_PARTS:
        folder = root / part
        suite = json.loads((folder / f'suite-codemode-{part}.json').read_text())
        if (suite.get('passed') is not True or suite.get('source_stable') is not True or
                suite.get('source_dirty') is not False or suite.get('codemode_part') != part or
                set(suite.get('shards', {})) != {'codemode'}):
            raise ValueError('failed, dirty or wrong Code Mode part: ' + part)
        rows = suite['shards']['codemode']
        gates = codemode_gates(part)
        expected = Counter((g.name, i) for g in gates for i in range(1, g.attempts + 1))
        if (Counter((r['gate'], r['attempt']) for r in rows) != expected or
                any(r.get('returncode') != 0 or r.get('owned_group_drained') is not True for r in rows)):
            raise ValueError('missing, duplicate or failed repetition: ' + part)
        for gate in gates:
            attempts = [r['attempt'] for r in rows if r['gate'] == gate.name]
            if attempts != list(range(1, gate.attempts + 1)):
                raise ValueError('reordered repetition: ' + part)
            for i in attempts:
                report = json.loads((folder / report_name(gate, i)).read_text())
                if part != 'control':
                    if report.get('binary_sha256') != suite['binary_sha256'] or report.get('cleanup') is not True:
                        raise ValueError('Core binary/cleanup drift: ' + part)
                    cores[part, i] = report
        suites[part] = suite
    first = suites['control']
    if any(any(s.get(key) != first.get(key) for key in IDENTITY) for s in suites.values()):
        raise ValueError('source or build drift between parts')
    if not first.get('source_sha') or not first.get('binary_sha256'):
        raise ValueError('source/build identity missing')
    reports = []
    for attempt in range(1, 6):
        report = copy.deepcopy(cores['contracts', attempt])
        if report.get('results') != []:
            raise ValueError('contracts part contains case variants')
        report['results'] = []
        for case in CODEMODE_PARTS[1:4]:
            fragment = cores[case, attempt]
            if any(fragment.get(key) != report.get(key) for key in ('binary_sha256', 'dataset_digest', 'contract_lock')):
                raise ValueError('frozen Core fixture drift')
            if (len(fragment['results']) != 6 or
                    {r['case_id'] for r in fragment['results']} != {case}):
                raise ValueError('missing or misplaced Core variants')
            report['results'].extend(fragment['results'])
        reports.append(report)
    summary = core_report.aggregate(reports)
    if summary['passed'] is not True:
        raise ValueError('full Core hard assertions failed')
    rows = copy.deepcopy(first['shards']['codemode'])
    for attempt in range(1, 6):
        members = [r for p, s in suites.items() if p != 'control'
                   for r in s['shards']['codemode'] if r['attempt'] == attempt]
        rows.append(dict(gate='core-cases', attempt=attempt, returncode=0,
                         owned_group_drained=True, parts=members))
    if Counter((r['gate'], r['attempt']) for r in rows) != Counter(
            (g.name, i) for g in SHARDS['codemode'] for i in range(1, g.attempts + 1)):
        raise ValueError('logical Code Mode inventory drift')
    full = dict(first, codemode_part=None, shards={'codemode': rows},
                parts=suites, elapsed_seconds=max(s['elapsed_seconds'] for s in suites.values()))
    return full, reports, summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parts', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    for name in ['suite-codemode.json', 'core-summary.json', *[f'core-cases-{i}.json' for i in range(1, 6)]]:
        (args.evidence / name).unlink(missing_ok=True)
    full, reports, summary = join(args.parts)
    for i, report in enumerate(reports, 1):
        (args.evidence / f'core-cases-{i}.json').write_text(json.dumps(report, indent=2) + '\n')
    # Use the same canonical writer as the unpartitioned local suite.
    if not core_report.write(args.evidence):
        raise ValueError('full Core verdict failed')
    (args.evidence / 'suite-codemode.json').write_text(json.dumps(full, indent=2) + '\n')
    print(json.dumps(summary))


if __name__ == '__main__':
    main()
