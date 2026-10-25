#!/usr/bin/env python3
"""Compare retained full CI attempts; reject incomplete or unmatched proof.

Read-only. Retain raw API responses and small structural suite reports, including
failed attempts. No test is retried and no workflow or PR state is changed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import zipfile

import ci_profile as profile
from evidence import artifact_archive, inventory
from verify_suite import SHARDS, CODEMODE_PARTS, PROVIDER_PARTS, COMMUNICATION_PARTS, codemode_gates, personal_gates, provider_gates, communication_gates
import m6_report
import ci_race_join
from ci_gate import REQUIRED

ROOT = Path(__file__).resolve().parents[1]
HISTORICAL = "historical-nine-job-shadow-v1"


def attempt(value):
    if not re.fullmatch(r'[1-9][0-9]*:[1-9][0-9]*', value):
        raise argparse.ArgumentTypeError('expected RUN_ID:ATTEMPT')
    return value


def pair(value):
    values = value.split(',')
    if len(values) != 2:
        raise argparse.ArgumentTypeError('expected BEFORE_RUN:ATTEMPT,AFTER_RUN:ATTEMPT')
    return dict(zip(('before', 'after'), map(attempt, values)))


def collect(repo, key, root=None, max_bytes=1 << 20, metadata_only=False):
    # ERII-206 owns metadata, immutable identity, budget and verified local reuse.
    # Do not duplicate its collector or silently fall back to unbounded ZIP reads.
    root = root or os.environ.get('ERII_EVIDENCE_ROOT')
    if not root and not metadata_only:
        raise ValueError('explicit artifact root or ERII_EVIDENCE_ROOT required')
    root = Path(root) if root else None
    run_id, number = map(int, key.split(':'))
    prefix = f'repos/{repo}/actions/runs/{run_id}'
    run = profile.gh('api', prefix + '/attempts/' + str(number))
    metadata = inventory(repo, run_id, number, run_info=run)
    pages = profile.gh('api', '--paginate', '--slurp', prefix + f'/attempts/{number}/jobs?per_page=100')
    jobs = [job for page in pages for job in page['jobs']]
    reports, receipts = {}, []
    if metadata_only or validate_jobs(dict(run=run, jobs=jobs), run['head_sha']):
        return dict(run=run, jobs=jobs, reports=reports, artifacts=metadata['artifacts'], transfers=receipts,
                    collection_mode='metadata-only' if metadata_only else 'ineligible-jobs')
    # Joined suites embed every part receipt. Do not fetch large part archives
    # again, or fast/race artifacts that contain no comparator input.
    for shard in SHARDS:
        name = f'evidence-{shard}-{metadata["source_sha"]}-{number}'
        matches = [a for a in metadata['artifacts'] if a['name'] == name]
        if len(matches) != 1:
            raise ValueError('missing or duplicate exact suite artifact: ' + name)
        archive, receipt = artifact_archive(root, metadata, matches[0]['id'], max_bytes)
        max_bytes -= receipt['downloaded_bytes']
        receipts.append(receipt)
        allowed = {'suite-' + shard + '.json'}
        if shard == 'codemode':
            allowed.add('core-summary.json')
        if shard == 'personal':
            allowed.add('m6-summary.json')
        # This reads selected members AFTER the whole archive was fetched/reused.
        with zipfile.ZipFile(archive) as entries:
            for entry in entries.infolist():
                if entry.filename not in allowed:
                    continue
                if entry.file_size > 16 << 20 or entry.filename in reports:
                    raise ValueError('oversized or duplicate suite report')
                reports[entry.filename] = json.loads(entries.read(entry))
    return dict(run=run, jobs=jobs, artifacts=metadata['artifacts'], reports=reports,
                transfers=receipts)


def topology(text=None):
    """Read maintained literal matrices, including fixed communication parts.

    Deliberately reject unsupported syntax instead of approximating YAML or
    maintaining another manually synchronized required-job table.
    """
    text = text if text is not None else (ROOT / '.github/workflows/ci.yml').read_text()
    blocks = re.findall(r'^  ([a-z_]+):\n(.*?)(?=^  [a-z_]+:|\Z)', text.split('jobs:\n', 1)[1], re.M | re.S)
    if {key for key, _ in blocks} != REQUIRED | {'checks'} or len(blocks) != len(REQUIRED) + 1:
        raise ValueError('workflow job inventory drift')
    names = []
    for key, block in blocks:
        match = re.search(r'^    name: (.+)$', block, re.M)
        if not match:
            raise ValueError('workflow job name missing')
        name = match[1]
        if key == 'verify':
            expected_name = "Verification (${{ matrix.shard }}${{ matrix.communicationpart && format('/{0}', matrix.communicationpart) || '' }})"
            match = re.search(r'^      matrix:\n(.*?)(?=^    steps:)', block, re.M | re.S)
            base = sorted(set(SHARDS) - {'codemode', 'personal', 'providers', 'communication'})
            values = re.findall(r'^        shard: (\[.*\])$', block, re.M)
            if len(values) != 1 or set(json.loads(values[0])) != set(base) or len(json.loads(values[0])) != len(base):
                raise ValueError('workflow and verification inventory drift')
            literal = '        shard: ' + values[0] + '\n        include:\n' + ''.join(
                '          - shard: communication\n            communicationpart: ' + p + '\n'
                for p in COMMUNICATION_PARTS)
            if name != expected_name or not match or match[1] != literal:
                raise ValueError('unsupported or incomplete communication matrix')
            names.extend('Verification (' + v + ')' for v in json.loads(values[0]))
            names.extend('Verification (communication/' + p + ')' for p in COMMUNICATION_PARTS)
            continue
        axis = re.findall(r'\$\{\{ matrix\.([a-z0-9_]+) \}\}', name)
        if axis:
            if len(axis) != 1:
                raise ValueError('unsupported workflow matrix')
            values = re.findall(r'^        ' + axis[0] + r': (\[.*\])$', block, re.M)
            if len(values) != 1 or 'include:' in block or 'exclude:' in block:
                raise ValueError('unsupported workflow matrix')
            values = json.loads(values[0])
            expected = (set(SHARDS) - {'codemode', 'personal', 'providers'} if key == 'verify' else
                        set(CODEMODE_PARTS) if key == 'codemode' else
                        set(m6_report.PARTS) if key == 'personal' else
                        set(PROVIDER_PARTS) if key == 'providers' else
                        set(ci_race_join.PARTS) if key == 'race' else set())
            if set(values) != expected or len(values) != len(expected):
                raise ValueError('workflow and suite part inventory drift')
            names.extend(name.replace('${{ matrix.' + axis[0] + ' }}', v) for v in values)
        else:
            if '${{' in name:
                raise ValueError('unsupported workflow name')
            names.append(name)
    if len(names) != len(set(names)):
        raise ValueError('duplicate workflow job name')
    return set(names)


def validate_jobs(data, source):
    errors = []
    run = data['run']
    if (not re.fullmatch('[0-9a-f]{40}', source) or run.get('head_sha') != source or
            run.get('path') != '.github/workflows/ci.yml' or
            run.get('status') != 'completed' or run.get('conclusion') != 'success'):
        errors.append('workflow/source/outcome mismatch')
    jobs = data['jobs']
    names = [j['name'] for j in jobs]
    required = topology()
    if (set(names) != required or len(names) != len(required) or
            any(j.get('conclusion') != 'success' for j in jobs)):
        errors.append('missing, duplicate or unsuccessful required part/join/job')
    for job in jobs:
        if not job.get('started_at') or not job.get('completed_at'):
            errors.append('missing job duration is not a zero measurement')
        if (job.get('head_sha') != source or job.get('run_id') != run['id'] or
                job.get('run_attempt') != run['run_attempt']):
            errors.append('job source/run/attempt mismatch')
    return errors


def validate(data, source, workflow='ci.yml'):
    errors = validate_jobs(data, source)
    identities = set()

    def suite(report, shard, gates, part=None):
        if (report.get('source_sha') != source or report.get('source_dirty') is not False or
                report.get('source_stable') is not True or report.get('passed') is not True or
                report.get('selected_gates') is not None or 'selected_gates' not in report or
                report.get('complete_inventory') is not (part is None)):
            errors.append(shard + ': missing clean exact-SHA complete suite success')
        rows = report.get('shards', {}).get(shard, [])
        expected = [(g.name, n) for g in gates for n in range(1, g.attempts + 1)]
        if (set(report.get('shards', {})) != {shard} or
                [(r.get('gate'), r.get('attempt')) for r in rows] != expected or
                any(r.get('returncode') != 0 or r.get('owned_group_drained') is not True or
                    any(flag in r.get('command', []) for flag in ('--feedback', '--gate')) for r in rows)):
            errors.append(shard + ': incomplete, focused or undrained gate inventory')
        identity = (report.get('binary_sha256'), report.get('image_os'), report.get('image_version'),
                    report.get('hardware', {}).get('logical_cpus'))
        if (not all(identity) or not re.fullmatch('[0-9a-f]{64}', identity[0] or '') or
                type(identity[3]) is not int or identity[3] < 1):
            errors.append(shard + ': missing binary/image/CPU identity')
        identities.add(identity)
        return rows

    reports = data['reports']
    for shard, gates in SHARDS.items():
        report = reports.get('suite-' + shard + '.json', {})
        rows = suite(report, shard, gates)
        if shard in ('providers', 'communication'):
            expected_parts = PROVIDER_PARTS if shard == 'providers' else COMMUNICATION_PARTS
            part_gates = provider_gates if shard == 'providers' else communication_gates
            part_field = 'provider_part' if shard == 'providers' else 'communication_part'
            parts = report.get('parts', {})
            if set(parts) != set(expected_parts):
                errors.append(shard + ': missing or extra joined part')
            members = []
            for part in expected_parts:
                fragment = parts.get(part, {})
                members.extend(suite(fragment, shard, part_gates(part), part))
                if (fragment.get(part_field) != part or
                        any(fragment.get(k) != report.get(k) for k in
                            ('source_sha', 'source_diff_sha256', 'binary_sha256', 'workflow_sha'))):
                    errors.append(shard + ': part identity drift')
            order = {(g.name, n): i for i, (g, n) in enumerate(
                (g, n) for g in gates for n in range(1, g.attempts + 1))}
            members.sort(key=lambda r: order.get((r.get('gate'), r.get('attempt')), -1))
            if rows != members:
                errors.append(shard + ': reconstructed join inventory drift')
            if shard == 'communication':
                from ci_communication import check_receipts, fixtures, IDENTITY
                manifests = report.get('native_receipts', {})
                if set(manifests) != set(expected_parts):
                    errors.append('communication: missing native manifests')
                for part in expected_parts:
                    manifest = manifests.get(part, {})
                    fragment = parts.get(part, {})
                    if (manifest.get('fixtures') != fixtures() or
                            manifest.get('identity') != {k: fragment.get(k) for k in IDENTITY}):
                        errors.append('communication: native identity or fixture drift')
                    try:
                        check_receipts(manifest.get('receipts', []), communication_gates(part))
                    except (ValueError, KeyError, TypeError):
                        errors.append('communication: failed or incomplete native receipts')
            continue
        if shard not in ('codemode', 'personal'):
            continue
        parts = report.get('parts', {})
        expected = CODEMODE_PARTS if shard == 'codemode' else m6_report.PARTS
        if set(parts) != set(expected):
            errors.append(shard + ': missing or extra joined part')
        for part in expected:
            fragment = parts.get(part, {})
            suite(fragment, shard, codemode_gates(part) if shard == 'codemode' else personal_gates(part), part)
            if (fragment.get(shard + '_part') != part or
                    any(fragment.get(k) != report.get(k) for k in
                        ('source_sha', 'source_diff_sha256', 'binary_sha256', 'workflow_sha'))):
                errors.append(shard + ': part identity drift')
        for row in rows:
            if shard == 'codemode' and row.get('gate') != 'core-cases':
                continue
            members = [r for part in expected if not (shard == 'codemode' and part == 'control')
                       for fragment in [parts.get(part, {})]
                       for r in fragment.get('shards', {}).get(shard, []) if r.get('attempt') == row.get('attempt')]
            if row.get('parts') != members:
                errors.append(shard + ': reconstructed join inventory drift')
    if len(identities) != 1:
        errors.append('binary, runner image or CPU capacity differs between full shards/parts')
    core = reports.get('core-summary.json', {})
    if (core.get('passed') is not True or core.get('ci_cancellation', {}).get('passed') is not True or
            core.get('ci_cancellation', {}).get('consecutive_runs') != 5):
        errors.append('missing five-run Core/nested cancellation proof')
    if reports.get('m6-summary.json', {}).get('passed') is not True:
        errors.append('missing strict M6 join verdict')
    return errors, identities


def job_name(name):
    if name == 'Shadow feedback':
        return 'Unit tests and lint'
    if name == 'CI shadow comparison':
        return 'Ubuntu checks'
    if name.startswith('Shadow '):
        return name.removeprefix('Shadow ').capitalize().replace('verification', 'Verification')
    return name


def validate_historical(data, source, workflow):
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
    result = {key: row[key] for key in ('run_id', 'url', 'source_sha', 'attempt', 'conclusion', 'wall_seconds', 'runner_minutes', 'jobs')}
    verdict = [j for j in data['jobs'] if job_name(j['name']) == 'Ubuntu checks']
    if any(j['wall_seconds'] is None for j in row['jobs']):
        result['runner_minutes'] = None
    result.update(required_verdict_seconds=profile.seconds(run['created_at'], verdict[0].get('completed_at'))
                  if len(verdict) == 1 else None,
                  cpu_seconds=None, cache_hit=None, outbound_payload_bytes=None,
                  dependency_wait_seconds=None)
    return result


def compare(raw):
    historical = raw.get('mode') == HISTORICAL
    if raw.get('mode') not in (None, 'current', HISTORICAL):
        raise ValueError('unknown comparison mode')
    validator = validate_historical if historical else validate
    rows, used, sources, identities = [], set(), set(), set()
    for item in raw['pairs']:
        keys = [item['before'], item['after']]
        before, after = (raw['attempts'][key] for key in keys)
        source = before['run']['head_sha']
        errors, identity = validator(before, source, 'ci.yml')
        errors = ['before: ' + error for error in errors]
        other_errors, other_identity = validator(after, source, 'ci-shadow.yml' if historical else 'ci.yml')
        errors.extend('after: ' + error for error in other_errors)
        for key, data in zip(keys, (before, after)):
            run = data['run']
            if key != f'{run["id"]}:{run["run_attempt"]}':
                errors.append('requested attempt does not match retained API identity')
        if identity != other_identity:
            errors.append('before/after binary or runner image mismatch')
        if not historical:
            fields = {'input_sha256', 'tool_versions', 'lock_sha256', 'cache_state', 'environment'}
            measurement = before.get('measurement_profile', {})
            if (not isinstance(measurement, dict) or not fields <= set(measurement) or
                    not all(measurement.get(k) for k in fields) or
                    any(not re.fullmatch('[0-9a-f]{64}', str(measurement.get(k, '')))
                        for k in ('input_sha256', 'lock_sha256')) or
                    not isinstance(measurement.get('tool_versions'), dict) or
                    measurement != after.get('measurement_profile')):
                errors.append('missing or mismatched frozen input/tools/locks/cache/environment profile')
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
    minimum = 5 if historical else 3
    if not historical and len({json.dumps(raw['attempts'][key].get('measurement_profile'), sort_keys=True)
                               for key in used}) != 1:
        cohort_errors.append('measurement profile drift between independent samples')
    complete = len(rows) >= minimum and len(valid) == len(rows) and not cohort_errors
    return dict(version=2, mode=HISTORICAL if historical else "current",
                comparison_complete=complete, performance_certified=complete and not historical,
                minimum_pairs=minimum, samples=len(rows), eligible_samples=len(valid),
                inventory={name: [(g.name, g.attempts) for g in gates] for name, gates in SHARDS.items()},
                pairs=rows, cohort_errors=cohort_errors, statistics=groups,
                p50_wall_reduction=reduction('wall_seconds') if complete else None,
                p50_runner_minutes_reduction=reduction('runner_minutes') if complete else None,
                limitations=['This compares complete mandatory inventories on one frozen source; it does not prove an ordinary-change cohort.',
                             'CPU/cache/scenario detail stays in workflow artifacts; API durations do not substitute for missing measurements.',
                             'No push timestamp is inferred. Cancellation and cost/latency acceptance require separate measured proof.',
                             'Rerun wall time starts at run_started_at; initial rerun queue latency is unavailable and is not zero.',
                             'Runner CPU model/affinity are retained per shard; hardware variation within a capacity class can contribute noise.',
                             'Five stability repetitions inside one workflow are not independent workflow attempts.',
                             'Current certification requires three independent pairs and a matching frozen measurement profile; historical shadow data never certifies current performance.',
                             'Dependency waits are not separable from queue latency in the jobs API. Step phases are name-based metadata classifications, not instrumented CPU/build/cache measurements.'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pair', type=pair, action='append', default=[])
    parser.add_argument('--repo', default='z-chenhao/erii')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--offline', action='store_true')
    parser.add_argument('--historical-shadow', action='store_true', help='interpret retained nine-job shadow data only')
    parser.add_argument('--artifact-root', type=Path, help='ERII-206 durable evidence/cache root; required online')
    parser.add_argument('--profile', type=Path, help='frozen non-secret measurement profile JSON; required for certification')
    parser.add_argument('--metadata-only', action='store_true',
                        help='retain exact-attempt observations without fetching archives; never certifies performance')
    args = parser.parse_args()
    raw_path = args.output.with_name(args.output.stem + '-raw.json')
    if args.offline and args.metadata_only:
        parser.error('--metadata-only is an online collection option; --offline replays its retained raw JSON')
    if args.offline:
        raw = json.loads(raw_path.read_text())
    else:
        if not args.pair:
            parser.error('at least one explicit before/after attempt pair is required')
        if args.historical_shadow or not (args.artifact_root or args.metadata_only):
            parser.error('online collection requires --artifact-root or --metadata-only; historical data is offline only')
        keys = sorted({key for item in args.pair for key in item.values()})
        values, remaining = [], 1 << 20
        for key in keys:
            value = collect(args.repo, key, args.artifact_root, remaining, metadata_only=args.metadata_only)
            remaining -= sum(r['downloaded_bytes'] for r in value['transfers'])
            if args.profile:
                value['measurement_profile'] = json.loads(args.profile.read_text())
            values.append(value)
        raw = dict(pairs=args.pair, attempts=dict(zip(keys, values)))
        args.output.parent.mkdir(parents=True, exist_ok=True)
        raw_path.write_text(json.dumps(raw, indent=2) + '\n')
    if args.historical_shadow:
        raw['mode'] = HISTORICAL
    report = compare(raw)
    report['raw_sha256'] = hashlib.sha256(raw_path.read_bytes()).hexdigest()
    args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({k: v for k, v in report.items() if k not in ('pairs', 'limitations')}))
    return 0 if report['comparison_complete'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
