#!/usr/bin/env python3
"""Require every M6 part and reconstruct the canonical five full reports."""
import argparse
from collections import Counter
import copy
import json
from pathlib import Path

import m6_report
from ci_codemode import IDENTITY
from verify_suite import personal_gates, report_name


def join(root):
    suites, fragments = {}, {}
    for part, pairs in m6_report.PARTS.items():
        folder = root / part
        suite = json.loads((folder / f'suite-personal-{part}.json').read_text())
        if (suite.get('passed') is not True or suite.get('source_stable') is not True or
                suite.get('source_dirty') is not False or suite.get('selected_gates') is not None or
                suite.get('personal_part') != part or
                set(suite.get('shards', {})) != {'personal'}):
            raise ValueError('failed, dirty or wrong M6 part: ' + part)
        gate, = personal_gates(part)
        rows = suite['shards']['personal']
        if ([(r['gate'], r['attempt']) for r in rows] != [(gate.name, i) for i in range(1, 6)] or
                any(r.get('returncode') != 0 or r.get('owned_group_drained') is not True for r in rows)):
            raise ValueError('missing, duplicate, reordered or failed M6 round: ' + part)
        for attempt in range(1, 6):
            report = json.loads((folder / report_name(gate, attempt)).read_text())
            if (report.get('part') != part or report.get('cleanup') is not True or
                    report.get('binary_sha256') != suite['binary_sha256'] or
                    Counter((r['case_id'], r['variant_id']) for r in report['results']) != Counter(pairs)):
                raise ValueError('misplaced M6 variants, binary or cleanup drift: ' + part)
            fragments[part, attempt] = report
        suites[part] = suite
    first = suites[next(iter(m6_report.PARTS))]
    if (not first.get('source_sha') or not first.get('binary_sha256') or
            any(any(s.get(key) != first.get(key) for key in IDENTITY) for s in suites.values())):
        raise ValueError('M6 source/build identity drift')
    expected = [(c, v) for c in m6_report.CASES for v in m6_report.VARIANTS]
    if Counter(pair for pairs in m6_report.PARTS.values() for pair in pairs) != Counter(expected):
        raise ValueError('M6 admission groups omit or duplicate a locked variant')
    reports, rows = [], []
    for attempt in range(1, 6):
        report = copy.deepcopy(fragments[next(iter(m6_report.PARTS)), attempt])
        report.update(part=None, results=[])
        members = []
        for part, suite in suites.items():
            fragment = fragments[part, attempt]
            if any(fragment.get(k) != report.get(k) for k in
                   ('binary_sha256', 'dataset_digest', 'driver_digest', 'contract_lock')):
                raise ValueError('M6 frozen contract drift')
            report['results'].extend(fragment['results'])
            members.append(suite['shards']['personal'][attempt - 1])
        order = {pair: i for i, pair in enumerate(expected)}
        report['results'].sort(key=lambda r: order[r['case_id'], r['variant_id']])
        report['summary'] = dict(fragments=66, fragment_passes=sum(
            r['status'] == 'fragment_pass' for r in report['results']),
            scores=None, semantic_pass_at_1=None, semantic_pass_5=None)
        reports.append(report)
        rows.append(dict(gate='m6-cases', attempt=attempt, returncode=0,
                         owned_group_drained=True, parts=members))
    summary = m6_report.aggregate(reports)
    if summary['passed'] is not True:
        raise ValueError('full M6 mechanical assertions failed')
    full = dict(first, complete_inventory=True, selected_gates=None,
                personal_part=None, shards={'personal': rows}, parts=suites,
                elapsed_seconds=max(s['elapsed_seconds'] for s in suites.values()))
    return full, reports, summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parts', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    for name in ['suite-personal.json', 'm6-summary.json', *[f'm6-cases-{i}.json' for i in range(1, 6)]]:
        (args.evidence / name).unlink(missing_ok=True)
    full, reports, summary = join(args.parts)
    for i, report in enumerate(reports, 1):
        (args.evidence / f'm6-cases-{i}.json').write_text(json.dumps(report, ensure_ascii=False) + '\n')
    if not m6_report.write(args.evidence):
        raise ValueError('full M6 verdict failed')
    (args.evidence / 'suite-personal.json').write_text(json.dumps(full, indent=2) + '\n')
    print(json.dumps(summary))


if __name__ == '__main__':
    main()
