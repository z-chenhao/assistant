#!/usr/bin/env python3
"""Collect paired full CI/shadow attempts; reject incomplete or unmatched proof.

Read-only. Retain raw API responses and small structural suite reports, including
failed attempts. No test is retried and no workflow or PR state is changed.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile

import ci_profile as profile
from verify_suite import SHARDS


def attempt(value):
    if not re.fullmatch(r'[1-9][0-9]*:[1-9][0-9]*', value):
        raise argparse.ArgumentTypeError('expected RUN_ID:ATTEMPT')
    return value


def pair(value):
    values = value.split(',')
    if len(values) != 2:
        raise argparse.ArgumentTypeError('expected BEFORE_RUN:ATTEMPT,AFTER_RUN:ATTEMPT')
    return dict(zip(('before', 'after'), map(attempt, values)))


def collect(repo, key):
    run_id, number = key.split(':')
    prefix = f'repos/{repo}/actions/runs/{run_id}'
    run = profile.gh('api', prefix + '/attempts/' + number)
    pages = profile.gh('api', '--paginate', '--slurp', prefix + f'/attempts/{number}/jobs?per_page=100')
    jobs = [job for page in pages for job in page['jobs']]
    pages = profile.gh('api', '--paginate', '--slurp', prefix + '/artifacts?per_page=100')
    artifacts = [a for page in pages for a in page['artifacts']
                 if a['name'].startswith('evidence-') and a['name'].endswith('-' + number)]
    reports = {}
    allowed = {'suite-' + name + '.json' for name in SHARDS}
    allowed.add('core-summary.json')
    for artifact in artifacts:
        if artifact.get('expired'):
            continue
        # Read selected ZIP members without extracting paths, executables,
        # corpus, prompt traces or the much larger business-case payloads.
        with tempfile.TemporaryFile() as archive:
            subprocess.run(['gh', 'api', f'repos/{repo}/actions/artifacts/{artifact["id"]}/zip'],
                           stdout=archive, check=True)
            archive.seek(0)
            with zipfile.ZipFile(archive) as entries:
                for entry in entries.infolist():
                    if entry.filename not in allowed:
                        continue
                    if entry.file_size > 16 << 20 or entry.filename in reports:
                        raise ValueError('oversized or duplicate suite report')
                    reports[entry.filename] = json.loads(entries.read(entry))
    return dict(run=run, jobs=jobs, artifacts=artifacts, reports=reports)


def job_name(name):
    if name == 'Shadow feedback':
        return 'Unit tests and lint'
    if name == 'CI shadow comparison':
        return 'Ubuntu checks'
    if name.startswith('Shadow '):
        return name.removeprefix('Shadow ').capitalize().replace('verification', 'Verification')
    return name


def validate(data, source, workflow):
    errors = []
    run = data['run']
    if (run.get('head_sha') != source or not re.fullmatch('[0-9a-f]{40}', source) or
            run.get('conclusion') != 'success' or run.get('status') != 'completed' or
            run.get('path') != '.github/workflows/' + workflow):
        errors.append('workflow/source/outcome mismatch')
    required = {'Unit tests and lint', 'Race detector', 'Ubuntu checks'} | {
        'Verification (' + name + ')' for name in SHARDS}
    names = [job_name(j['name']) for j in data['jobs']]
    if (len(names) != len(required) or set(names) != required or
            any(j.get('conclusion') != 'success' for j in data['jobs'])):
        errors.append('missing, duplicate or unsuccessful required job')
    if any(not j.get('started_at') or not j.get('completed_at') for j in data['jobs']):
        errors.append('missing job duration is not a zero measurement')
    reports = data['reports']
    identities = set()
    for name, gates in SHARDS.items():
        report = reports.get('suite-' + name + '.json', {})
        if (report.get('source_sha') != source or report.get('source_dirty') is not False or
                report.get('source_stable') is not True or report.get('passed') is not True):
            errors.append(name + ': missing clean exact-SHA suite success')
        expected = [(g.name, n) for g in gates for n in range(1, g.attempts + 1)]
        rows = report.get('shards', {}).get(name, [])
        if (set(report.get('shards', {})) != {name} or
                [(r.get('gate'), r.get('attempt')) for r in rows] != expected or
                any(r.get('returncode') != 0 or '--feedback' in r.get('command', []) for r in rows)):
            errors.append(name + ': incomplete or partial gate inventory')
        identity = (report.get('binary_sha256'), report.get('image_os'), report.get('image_version'),
                    report.get('hardware', {}).get('logical_cpus'))
        if not all(identity) or not re.fullmatch('[0-9a-f]{64}', identity[0] or ''):
            errors.append(name + ': missing binary/image identity')
        identities.add(identity)
        if type(report.get('hardware', {}).get('logical_cpus')) is not int or report['hardware']['logical_cpus'] < 1:
            errors.append(name + ': missing runner CPU capacity')
    if len(identities) != 1:
        errors.append('binary, runner image or CPU capacity differs between full shards')
    core = reports.get('core-summary.json', {})
    if (core.get('passed') is not True or core.get('ci_cancellation', {}).get('passed') is not True or
            core.get('ci_cancellation', {}).get('consecutive_runs') != 5):
        errors.append('missing five-run Core/nested cancellation proof')
    return errors, identities


def timing(data):
    run = dict(data['run'])
    # Reruns keep the original created_at. Measure this attempt, not idle time
    # between attempts; dependency/queue delays within the attempt remain.
    if run.get('run_attempt', 1) > 1:
        run['created_at'] = run['run_started_at']
    row = profile.summarize([run], {str(run['id']): data['jobs']})['runs'][0]
    return {key: row[key] for key in ('url', 'source_sha', 'attempt', 'conclusion', 'wall_seconds', 'runner_minutes', 'jobs')}


def compare(raw):
    rows, used, sources, identities = [], set(), set(), set()
    for item in raw['pairs']:
        keys = [item['before'], item['after']]
        before, after = (raw['attempts'][key] for key in keys)
        source = before['run']['head_sha']
        errors, identity = validate(before, source, 'ci.yml')
        errors = ['before: ' + error for error in errors]
        other_errors, other_identity = validate(after, source, 'ci-shadow.yml')
        errors.extend('after: ' + error for error in other_errors)
        for key, data in zip(keys, (before, after)):
            run = data['run']
            if key != f'{run["id"]}:{run["run_attempt"]}':
                errors.append('requested attempt does not match retained API identity')
        if identity != other_identity:
            errors.append('before/after binary or runner image mismatch')
        identities.update(identity | other_identity)
        if len(set(keys)) != 2 or any(key in used for key in keys):
            errors.append('reused workflow attempt is not an independent sample')
        used.update(keys)
        sources.add(source)
        rows.append(dict(**item, source_sha=source, eligible=not errors, errors=errors,
                         before_timing=timing(before), after_timing=timing(after)))
    valid = [row for row in rows if row['eligible']]
    groups = {side: {metric: profile.stats([row[side + '_timing'][metric] for row in valid])
                     for metric in ('wall_seconds', 'runner_minutes')} for side in ('before', 'after')}
    def reduction(metric):
        a, b = (groups[side][metric]['p50'] for side in ('before', 'after'))
        return (a - b) / a if a and b is not None else None
    cohort_errors = []
    if len(sources) != 1 or len(identities) != 1:
        cohort_errors.append('samples must use one frozen source, binary and runner image')
    complete = len(rows) >= 5 and len(valid) == len(rows) and not cohort_errors
    return dict(version=1, comparison_complete=complete, samples=len(rows), eligible_samples=len(valid),
                pairs=rows, cohort_errors=cohort_errors, statistics=groups,
                p50_wall_reduction=reduction('wall_seconds'),
                p50_runner_minutes_reduction=reduction('runner_minutes'),
                limitations=['This compares complete mandatory inventories on one frozen source; it does not prove an ordinary-change cohort.',
                             'CPU/cache/scenario detail stays in workflow artifacts; API durations do not substitute for missing measurements.',
                             'No push timestamp is inferred. Cancellation and cost/latency acceptance require separate measured proof.',
                             'Rerun wall time starts at run_started_at; initial rerun queue latency is unavailable and is not zero.',
                             'Runner CPU model/affinity are retained per shard; hardware variation within a capacity class can contribute noise.',
                             'Five stability repetitions inside one workflow are not five independent workflow attempts.'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pair', type=pair, action='append', default=[])
    parser.add_argument('--repo', default='z-chenhao/erii')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--offline', action='store_true')
    args = parser.parse_args()
    raw_path = args.output.with_name(args.output.stem + '-raw.json')
    if args.offline:
        raw = json.loads(raw_path.read_text())
    else:
        if not args.pair:
            parser.error('at least one explicit before/after attempt pair is required')
        keys = sorted({key for item in args.pair for key in item.values()})
        with ThreadPoolExecutor(max_workers=4) as pool:
            values = list(pool.map(lambda key: collect(args.repo, key), keys))
        raw = dict(pairs=args.pair, attempts=dict(zip(keys, values)))
        args.output.parent.mkdir(parents=True, exist_ok=True)
        raw_path.write_text(json.dumps(raw, indent=2) + '\n')
    report = compare(raw)
    args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({k: v for k, v in report.items() if k not in ('pairs', 'limitations')}))
    return 0 if report['comparison_complete'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
