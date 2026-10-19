#!/usr/bin/env python3
"""Record one authorized push; collect exact-attempt supersession evidence.

No polling or automatic retry. A failed push has an unknown outcome. Collection
is read-only and rejects incomplete/source-mismatched evidence rather than
turning unknown timestamps into zero latency.
"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess
import sys

from ci_profile import gh

REPO = 'z-chenhao/erii'
WORKFLOWS = {'.github/workflows/ci.yml', '.github/workflows/ci-shadow.yml'}


def now():
    return datetime.now(timezone.utc).isoformat()


def date(value):
    if not isinstance(value, str):
        raise ValueError('missing timestamp')
    result = datetime.fromisoformat(value.replace('Z', '+00:00'))
    if result.tzinfo is None:
        raise ValueError('timestamp lacks timezone')
    return result


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + '.tmp')
    temporary.write_text(json.dumps(value, indent=2) + '\n')
    temporary.replace(path)


def push(pr, output):
    if output.exists():
        raise ValueError('inspect the existing push record; never retry an unknown push')
    before = gh('pr', 'view', str(pr), '--repo', REPO, '--json', 'headRefOid,headRefName,isDraft')
    branch = subprocess.check_output(['git', 'branch', '--show-current'], text=True).strip()
    after = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if not before['isDraft'] or branch != before['headRefName'] or after == before['headRefOid']:
        raise ValueError('requires a new committed head on this Draft PR branch')
    subprocess.run(['git', 'merge-base', '--is-ancestor', before['headRefOid'], after], check=True)
    record = dict(version=1, pr=pr, before_sha=before['headRefOid'], after_sha=after,
                  push_started_at=now(), outcome='unknown')
    write(output, record)
    result = subprocess.run(['git', 'push', '--porcelain', 'origin', after+':refs/heads/'+branch],
                            capture_output=True, text=True)
    sys.stdout.write(result.stdout)
    sys.stderr.write(result.stderr)
    updated = result.returncode == 0 and any(line.startswith(' \t') for line in result.stdout.splitlines())
    record.update(returncode=0 if updated else (result.returncode or 1),
                  git_returncode=result.returncode, push_finished_at=now())
    if updated:
        record.update(outcome='accepted', push_acknowledged_at=record['push_finished_at'])
    write(output, record)
    return record['returncode']


def summarize(record, snapshots):
    errors = []
    if record.get('returncode') != 0:
        errors.append('push outcome is unknown')
    if any(not isinstance(record.get(key), str) or not re.fullmatch('[0-9a-f]{40}', record[key])
           for key in ('before_sha', 'after_sha')):
        errors.append('missing full source identity')
    if record.get('before_sha') == record.get('after_sha'):
        errors.append('push did not change source')
    try:
        started, acknowledged = date(record.get('push_started_at')), date(record.get('push_acknowledged_at'))
        if acknowledged < started:
            raise ValueError('push acknowledgment precedes start')
    except ValueError:
        errors.append('missing or invalid push timestamp interval')
    paths = [value['run'].get('path') for value in snapshots.values()]
    if len(paths) != len(WORKFLOWS) or set(paths) != WORKFLOWS:
        errors.append('missing or duplicate superseded workflow')
    if any(value['run'].get('head_sha') != record.get('before_sha') for value in snapshots.values()):
        errors.append('superseded source mismatch')
    jobs = [job for value in snapshots.values() for job in value['jobs']]
    if (not jobs or any(value['run'].get('status') != 'completed' for value in snapshots.values()) or
            any(job.get('status') != 'completed' or not job.get('completed_at') for job in jobs)):
        errors.append('old workflow or job is incomplete')
    report = dict(version=1, push=record, observed_at=now(), attempts=snapshots,
                  measurement_complete=False, errors=errors,
                  limitations=['API timestamps have one-second precision; client/API clock skew is not independently measured.',
                               'Job completion alone does not prove descendant/port/state cleanup or a final-candidate p95.'])
    if errors:
        return report
    active = [job for job in jobs if job.get('started_at') and
              date(job['started_at']) <= started < date(job['completed_at'])]
    if not any(job.get('conclusion') == 'cancelled' for job in active):
        errors.append('no active superseded cancellation; not a zero-latency sample')
        return report
    stopped = max(date(job['completed_at']) for job in jobs)
    report.update(measurement_complete=True, all_old_stopped_at=stopped.isoformat(),
                  api_precision_allowance_seconds=1,
                  stop_latency_bounds_seconds=[max(0, (stopped-acknowledged).total_seconds()-1),
                                               max(0, (stopped-started).total_seconds()+1)],
                  active_old_jobs=[job['name'] for job in active],
                  runner_minutes_after_push=sum(max(0, (date(job['completed_at']) -
                      max(date(job['started_at']), started)).total_seconds()) / 60
                      for job in jobs if job.get('started_at')) if all(
                      job.get('started_at') or date(job['completed_at']) <= started or
                      job.get('conclusion') == 'skipped' for job in jobs) else None)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    record = commands.add_parser('push')
    record.add_argument('--pr', type=int, required=True)
    record.add_argument('--output', type=Path, required=True)
    collect = commands.add_parser('collect')
    collect.add_argument('--push-record', type=Path, required=True)
    collect.add_argument('--run', action='append', required=True, help='exact RUN_ID:ATTEMPT')
    collect.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.command == 'push':
        return push(args.pr, args.output)
    if args.output.exists() or len(args.run) != len(set(args.run)):
        raise ValueError('use a fresh output and distinct exact attempts')
    snapshots = {}
    for key in args.run:
        if not re.fullmatch('[1-9][0-9]*:[1-9][0-9]*', key):
            raise ValueError('expected RUN_ID:ATTEMPT')
        run_id, number = key.split(':')
        prefix = f'repos/{REPO}/actions/runs/{run_id}/attempts/{number}'
        run = gh('api', prefix)
        pages = gh('api', '--paginate', '--slurp', prefix + '/jobs?per_page=100')
        snapshots[key] = dict(run=run, jobs=[job for page in pages for job in page['jobs']])
    report = summarize(json.loads(args.push_record.read_text()), snapshots)
    write(args.output, report)
    print(json.dumps({key: value for key, value in report.items() if key not in ('push', 'attempts')}))
    return 0 if report['measurement_complete'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
