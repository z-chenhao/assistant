#!/usr/bin/env python3
"""Require both exact-source race parts and re-read SQLite discovery/events.

Parts change placement only. No test/count/race/deadline or cleanup obligation
is removed. The ordinary make test-race command still runs the whole inventory.
"""
import argparse
from collections import Counter
import json
from pathlib import Path
import subprocess

import ci_race as race
from verify_suite import source_state

PARTS = ('packages', 'sqlite')


def join(parts, packages, source):
    if source['source_dirty'] or {p.name for p in parts.iterdir()} != set(PARTS):
        raise ValueError('clean source and exactly two race parts required')
    selected, reports = [], {}
    for part in PARTS:
        root = parts/part/'race'
        report = json.loads((root/'inventory.json').read_text())
        plan = race.groups(packages, part)
        expected = [p for _, group in plan for p in group]
        if (report.get('version') != 1 or report.get('part') != part or report.get('passed') is not True or
                report.get('source_stable') is not True or report.get('packages') != packages or
                any(report.get(key) != value for key, value in source.items()) or
                report.get('selected_packages') != expected or
                [(r['name'], r['packages']) for r in report['stages']] != plan):
            raise ValueError('failed, missing or drifted race inventory')
        for stage in report['stages']:
            if stage['returncode'] != 0 or stage.get('owned_group_drained') is not True:
                raise ValueError('race stage failed or left processes')
            if stage['name'] != 'remainder':
                command = race.RACE + (['-p=1'] if stage['name'] == 'other-packages' else []) + stage['packages']
                if stage['command'] != command or not (root/(stage['name']+'.log')).read_text().strip():
                    raise ValueError('package race command or output drift')
                continue
            names = race.test_names((root/'sqlite-list.log').read_text())
            if (stage['discovery'] != race.RACE+['-list','.',race.SQLITE] or
                    stage['expected_tests'] != names or not stage['selection_complete'] or
                    stage['isolated_tests'] != race.SERIAL_SQLITE or
                    stage['discovery_metrics'].get('owned_group_drained') is not True):
                raise ValueError('SQLite discovery drift or incomplete selection')
            observed = []
            workers = stage['workers']
            budget = stage['cpu_budget']
            if (type(budget) is not int or not 1 <= budget <= 4 or
                    not 2 <= len(workers) <= min(4, budget+1) or workers[0]['name'] != 'sqlite-serial' or
                    workers[0]['expected_tests'] != race.SERIAL_SQLITE or
                    len({w['name'] for w in workers}) != len(workers)):
                raise ValueError('SQLite worker inventory drift')
            for worker in workers:
                expected_tests = worker['expected_tests']
                if (worker['name'] not in ('sqlite-serial','sqlite-1','sqlite-2','sqlite-3') or
                        not expected_tests or len(expected_tests) != len(set(expected_tests)) or
                        worker['gomaxprocs'] != (budget if worker['name'] == 'sqlite-serial' else 1) or
                        worker['returncode'] != 0 or worker.get('owned_group_drained') is not True or
                        worker['command'] != race.RACE+['-json','-run','^('+'|'.join(expected_tests)+')$',race.SQLITE]):
                    raise ValueError('SQLite race worker failed or command drifted')
                outcomes = race.sqlite_verdict(root/(worker['name']+'.log'), expected_tests)
                if outcomes != worker['tests']:
                    raise ValueError('SQLite event outcomes drifted')
                observed.extend(expected_tests)
            if Counter(observed) != Counter(names):
                raise ValueError('missing or duplicate SQLite roots across workers')
        selected.extend(expected)
        reports[part] = report
    if Counter(selected) != Counter(packages):
        raise ValueError('missing or duplicate race packages across parts')
    return dict(version=1, **source, passed=True, packages=packages, parts=reports)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parts', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    packages = subprocess.check_output(['go','list','./...'], text=True).splitlines()
    report = join(args.parts, packages, source_state())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps(dict(passed=True, parts=list(report['parts']), packages=len(packages))))


if __name__ == '__main__': main()
