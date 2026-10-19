#!/usr/bin/env python3
"""Five controlled superseding pushes on isolated ERII-141 workflow replicas.

Never runs against a PR or changes the Required pipeline. Freeze six doc-only
heads first: five hot workloads, then a stop-marker push. All mutations are
checkpointed unknown before dispatch and never retried. Raw failed evidence stays.
"""
import argparse
import json
from pathlib import Path
import re
import subprocess
import time

from ci_profile import gh
from ci_supersede import now, summarize, write

REPO = 'z-chenhao/erii'
BRANCH = 'ERII-141-ordinary-profile'
PATHS = {'.github/workflows/ci-control-cancel-profile.yml',
         '.github/workflows/ci-shadow-cancel-profile.yml'}
MARKER = '[stop-cancel-profile]'


def git(*args):
    return subprocess.check_output(['git', *args], text=True).strip()


def snapshots(source):
    values = gh('api', f'repos/{REPO}/actions/runs?head_sha={source}&event=push&per_page=100')['workflow_runs']
    selected = [r for r in values if r['path'] in PATHS]
    if len(selected) > 2 or len({r['path'] for r in selected}) != len(selected):
        raise ValueError('ambiguous replica workflow registration')
    result = {}
    for item in selected:
        if item['head_sha'] != source or item['run_attempt'] != 1:
            raise ValueError('source/attempt drift; never rerun a sample')
        prefix = f'repos/{REPO}/actions/runs/{item["id"]}/attempts/1'
        run = gh('api', prefix)
        pages = gh('api', '--paginate', '--slurp', prefix + '/jobs?per_page=100')
        result[f'{item["id"]}:1'] = dict(run=run, jobs=[j for p in pages for j in p['jobs']])
    return result


def hot(source):
    deadline, observed = time.monotonic() + 900, None
    while time.monotonic() < deadline:
        values = snapshots(source)
        if any(v['run']['status'] == 'completed' for v in values.values()):
            raise ValueError('workload ended before the controlled cancellation; retain outcome without retry')
        jobs = [j for v in values.values() for j in v['jobs']]
        if any(j.get('conclusion') in ('failure','timed_out','cancelled') for j in jobs):
            raise ValueError('a workload failed before cancellation; do not retry to green')
        code = [j for j in jobs if j['name'] in ('Verification (codemode)', 'Shadow verification (codemode)')]
        ready = (len(values) == 2 and len(code) == 2 and
                 sum(j['status'] == 'in_progress' for j in jobs) >= 10 and
                 all(any(s['name'] == 'CLI verification with timings' and s['status'] == 'in_progress'
                         for s in j['steps']) for j in code))
        observed = (observed or time.monotonic()) if ready else None
        if observed is not None and time.monotonic() - observed >= 30:
            return values
        time.sleep(15)
    raise TimeoutError('replicas did not reach both hot Code Mode phases; no superseding push issued')


def stopped(source):
    deadline = time.monotonic() + 240
    while time.monotonic() < deadline:
        values = snapshots(source)
        if len(values) == 2 and all(v['run']['status'] == 'completed' and
                                  all(j['status'] == 'completed' for j in v['jobs']) for v in values.values()):
            return values
        time.sleep(5)
    raise TimeoutError('old replica jobs did not stop within the observation bound')


def push(source, output):
    if output.exists():
        raise ValueError('existing push record; never retry an unknown mutation')
    before = git('ls-remote', '--exit-code', 'origin', 'refs/heads/' + BRANCH).split()[0]
    subprocess.run(['git','merge-base','--is-ancestor',before,source], check=True)
    if before == source:
        raise ValueError('push is already applied; never retry')
    if git('diff','--name-only',before,source) != 'docs/ci-cancel-probe.md':
        raise ValueError('only the prepared doc delta may supersede a workload')
    record = dict(version=1, branch=BRANCH, before_sha=before, after_sha=source,
                  push_started_at=now(), outcome='unknown')
    write(output, record)
    result = subprocess.run(['git','push','--porcelain','origin',source+':refs/heads/'+BRANCH],
                            capture_output=True, text=True)
    accepted = result.returncode == 0 and any(line.startswith(' \t') for line in result.stdout.splitlines())
    record.update(returncode=0 if accepted else (result.returncode or 1), push_finished_at=now())
    if accepted:
        record.update(outcome='accepted',push_acknowledged_at=record['push_finished_at'])
    write(output,record)
    if not accepted:
        raise RuntimeError('push outcome unknown; inspect remote and checkpoint without retry')
    return record


def run(heads, output):
    if output.exists():
        raise ValueError('existing experiment must be inspected; never replay its mutations')
    if len(heads) != 6 or len(set(heads)) != 6 or any(not re.fullmatch('[0-9a-f]{40}', h) for h in heads):
        raise ValueError('freeze five workload heads and one stop head')
    if git('branch','--show-current') != BRANCH:
        raise ValueError('isolated profile branch required')
    for index, source in enumerate(heads):
        if (MARKER in git('show','-s','--format=%B',source)) != (index == 5):
            raise ValueError('only final frozen head may stop workloads')
        if index and git('diff','--name-only',heads[index-1],source) != 'docs/ci-cancel-probe.md':
            raise ValueError('workload inputs must stay frozen between doc-only pushes')
    state = dict(version=1, heads=heads, started_at=now(), samples=[], stage='running')
    write(output,state)
    previous = None
    for index, source in enumerate(heads):
        record = push(source, output.with_name(output.stem+f'-push-{index+1}.json'))
        if previous is not None:
            raw = stopped(previous)
            report = summarize(record,raw,workflows=PATHS)
            path = output.with_name(output.stem+f'-sample-{index}.json')
            write(path,report)
            state['samples'].append(dict(number=index,report=str(path)))
            write(output,state)
            if not report['measurement_complete']:
                raise ValueError('incomplete cancellation measurement; retain evidence without retry')
            print(json.dumps(dict(sample=index,report=str(path))),flush=True)
        if index < 5:
            values = hot(source)
            write(output.with_name(output.stem+f'-hot-{index+1}.json'),values)
        previous = source
    # Final stop-marker workflows are recorded too; they must not launch workloads.
    final = stopped(heads[-1])
    if any(any(j.get('conclusion') != 'skipped' for j in v['jobs']) for v in final.values()):
        raise ValueError('final stop marker admitted workload unexpectedly')
    write(output.with_name(output.stem+'-stop.json'),final)
    state.update(stage='complete',completed_at=now())
    write(output,state)
    return 0


if __name__ == '__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--head',action='append',required=True)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    raise SystemExit(run(args.head,args.output))
