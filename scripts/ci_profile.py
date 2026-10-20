#!/usr/bin/env python3
"""Collect GitHub CI timing evidence without treating unavailable data as zero.

Requires gh read access. Raw API responses are retained beside the report so
classification and statistics can be independently reproduced offline.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime
import json
import math
from pathlib import Path
import statistics
from ci_github import gh


def seconds(start, end):
    if not start or not end:
        return None
    return max(0, (datetime.fromisoformat(end.replace('Z', '+00:00')) -
                   datetime.fromisoformat(start.replace('Z', '+00:00'))).total_seconds())


def stats(values):
    values = sorted(v for v in values if v is not None)
    if not values:
        return dict(samples=0, p50=None, p95=None, variance=None)
    return dict(samples=len(values), p50=statistics.median(values),
                p95=values[math.ceil(.95 * len(values)) - 1],
                variance=statistics.variance(values) if len(values) > 1 else None)


def phase(name):
    name = name.lower()
    if 'upload' in name or 'retain' in name:
        return 'upload'
    if 'build' in name or 'bundle' in name:
        return 'build'
    if any(s in name for s in ('verification', 'detector', 'lint', 'tests', 'feedback')):
        return 'test'
    return 'setup'


def summarize(runs, jobs_by_run):
    rows = []
    # GitHub does not expose the webhook's push timestamp here. A successor's
    # run creation is explicitly a proxy, never a claim of push-to-stop latency.
    for run in runs:
        jobs = jobs_by_run[str(run['id'])]
        successors = [r for r in runs if r['id'] != run['id'] and
                      r['head_branch'] == run['head_branch'] and
                      r['event'] == run['event'] and r['head_sha'] != run['head_sha'] and
                      r['created_at'] > run['created_at']]
        successor = min(successors, key=lambda r: r['created_at']) if successors else None
        normalized = []
        for job in jobs:
            steps = [dict(name=s['name'], conclusion=s.get('conclusion'),
                          phase=phase(s['name']),
                          wall_seconds=seconds(s.get('started_at'), s.get('completed_at')))
                     for s in job.get('steps', [])]
            normalized.append(dict(id=job['id'], name=job['name'],
                                   conclusion=job.get('conclusion'), runner=job.get('labels'),
                                   wall_seconds=seconds(job.get('started_at'), job.get('completed_at')),
                                   initial_queue_seconds=seconds(run['created_at'], job.get('started_at')),
                                   cpu_seconds=None, cache_hit=None, steps=steps))
        stops = [j['completed_at'] for j in jobs if j.get('completed_at')]
        stop = max(stops) if stops else None
        wall = seconds(run['created_at'], stop)
        wasted = None
        proxy = None
        if run.get('conclusion') == 'cancelled' and successor:
            pushed = successor['created_at']
            proxy = seconds(pushed, stop) if stop and stop >= pushed else None
            wasted = sum(seconds(max(pushed, j['started_at']), j['completed_at']) or 0
                         for j in jobs if j.get('started_at') and j.get('completed_at') and
                         j['completed_at'] >= pushed) / 60
        rows.append(dict(run_id=run['id'], url=run['html_url'], source_sha=run['head_sha'],
                         branch=run['head_branch'], event=run['event'],
                         attempt=run.get('run_attempt'), conclusion=run.get('conclusion'),
                         wall_seconds=wall,
                         runner_minutes=sum(j['wall_seconds'] or 0 for j in normalized) / 60,
                         push_to_stop_seconds=None, successor_creation_to_stop_seconds=proxy,
                         runner_minutes_after_successor_creation=wasted, jobs=normalized))
    grouped = {}
    for outcome in sorted({r['conclusion'] or 'incomplete' for r in rows}):
        group = [r for r in rows if (r['conclusion'] or 'incomplete') == outcome]
        grouped[outcome] = dict(wall_seconds=stats([r['wall_seconds'] for r in group]),
                                runner_minutes=stats([r['runner_minutes'] for r in group]),
                                successor_creation_to_stop_seconds=stats([
                                    r['successor_creation_to_stop_seconds'] for r in group]))
    return dict(version=1, runs=rows, by_conclusion=grouped,
                distinct_shas=len({r['source_sha'] for r in rows}),
                limitations=['API history has no CPU/cache-hit or harness timings; null means unmeasured.',
                             'Job initial queue includes dependency waits. Runner-minutes use job lifetime, not billed minutes.',
                             'Successor run creation is a cancellation proxy; exact push-to-stop requires webhook timing.',
                             'Historical samples are observational, not a controlled before/after experiment.'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default='z-chenhao/erii')
    parser.add_argument('--limit', type=int, default=30)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--offline', action='store_true')
    args = parser.parse_args()
    raw = args.output.parent / (args.output.stem + '-raw.json')
    if args.offline:
        data = json.loads(raw.read_text())
    else:
        runs = gh('api', f'repos/{args.repo}/actions/workflows/ci.yml/runs?per_page={args.limit}')['workflow_runs']
        def collect(run):
            pages = gh('api', '--paginate', '--slurp',
                       f'repos/{args.repo}/actions/runs/{run["id"]}/attempts/{run["run_attempt"]}/jobs?per_page=100')
            return str(run['id']), [job for page in pages for job in page['jobs']]
        with ThreadPoolExecutor(max_workers=4) as pool:
            jobs = dict(pool.map(collect, runs))
        data = dict(runs=runs, jobs=jobs)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        raw.write_text(json.dumps(data, indent=2) + '\n')
    report = summarize(data['runs'], data['jobs'])
    args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(dict(distinct_shas=report['distinct_shas'], by_conclusion=report['by_conclusion'])))


if __name__ == '__main__':
    main()
