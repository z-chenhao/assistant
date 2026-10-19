#!/usr/bin/env python3
"""Collect five complete CI/shadow pairs on one frozen Draft PR head.

Every rerun follows a successful complete pair. A failed/unknown request or
failed test stops the experiment; failures are never retried to obtain green.
All work stays in this foreground process. JSON checkpoints survive interruption.
"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess
import time

import ci_compare
from ci_profile import gh

REPO = 'z-chenhao/erii'


def now():
    return datetime.now(timezone.utc).isoformat()


def save(path, value):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(value, indent=2) + '\n')
    temporary.replace(path)


def head(pr, sha):
    value = gh('pr', 'view', str(pr), '--repo', REPO, '--json', 'headRefOid,isDraft')
    if value['headRefOid'] != sha or value['isDraft'] is not True:
        raise ValueError('experiment requires the same frozen Draft PR head')


def wait_active(pr, sha, pair):
    """Wait for accepted attempts to replace old green PR check records."""
    deadline = time.monotonic() + 60
    pending = set(pair.values())
    while pending:
        head(pr, sha)
        checks = gh('api', f'repos/{REPO}/commits/{sha}/check-runs?per_page=100')['check_runs']
        links = [check.get('details_url') or '' for check in checks]
        for key in list(pending):
            run_id, number = key.split(':')
            value = gh('api', f'repos/{REPO}/actions/runs/{run_id}')
            if value['head_sha'] != sha:
                raise ValueError('workflow head drift during registration')
            if value['run_attempt'] != int(number):
                continue
            jobs = gh('api', f'repos/{REPO}/actions/runs/{run_id}/attempts/{number}/jobs?per_page=100')['jobs']
            if any(f'/job/{job["id"]}' in link for job in jobs for link in links):
                pending.remove(key)
        if not pending:
            return
        if time.monotonic() >= deadline:
            raise RuntimeError('accepted attempts did not register fresh PR checks')
        time.sleep(2)


def run(pr, sha, output):
    if output.exists():
        raise ValueError('experiment output already exists; inspect its checkpoint before taking further actions')
    head(pr, sha)
    output.parent.mkdir(parents=True, exist_ok=True)
    state = dict(version=1, pr=pr, source_sha=sha, started_at=now(), rounds=[], requests=[], stage='registration')
    save(output, state)
    deadline = time.monotonic() + 60
    while True:
        runs = gh('api', f'repos/{REPO}/actions/runs?head_sha={sha}&event=pull_request&per_page=100')['workflow_runs']
        selected = {}
        for workflow in ('ci.yml', 'ci-shadow.yml'):
            matching = [r for r in runs if r['path'] == '.github/workflows/' + workflow]
            if len(matching) > 1:
                raise ValueError('ambiguous workflow runs; refuse to choose a favorable sample')
            if matching:
                selected[workflow] = matching[0]
        if len(selected) == 2:
            break
        if time.monotonic() >= deadline:
            raise RuntimeError('new-head workflows did not register within the bounded readiness window')
        time.sleep(2)
    for number in range(1, 6):
        head(pr, sha)
        pair = {}
        for side, workflow in [('before', 'ci.yml'), ('after', 'ci-shadow.yml')]:
            run_id = selected[workflow]['id']
            value = gh('api', f'repos/{REPO}/actions/runs/{run_id}')
            if value['head_sha'] != sha:
                raise ValueError('workflow head drift')
            if number > 1:
                if value['status'] != 'completed' or value['conclusion'] != 'success':
                    raise ValueError('never rerun a failed/incomplete sample to obtain green')
                request = dict(run_id=run_id, target_attempt=value['run_attempt'] + 1, requested_at=now(), outcome='unknown')
                state['requests'].append(request)
                state['stage'] = 'requesting-rerun'
                save(output, state)
                subprocess.run(['gh', 'run', 'rerun', str(run_id), '--repo', REPO], check=True)
                request.update(outcome='accepted', accepted_at=now())
                save(output, state)
                attempt = request['target_attempt']
            else:
                attempt = value['run_attempt']
            pair[side] = f'{run_id}:{attempt}'
        state.update(stage='watching', active_round=number, active_pair=pair)
        save(output, state)
        print(json.dumps(dict(round=number, source_sha=sha, pair=pair, stage='watching')), flush=True)
        wait_active(pr, sha, pair)
        log = output.with_name(output.stem + f'-round-{number}.log')
        with log.open('w') as stream:
            result = subprocess.run(['gh', 'pr', 'checks', str(pr), '--repo', REPO, '--watch', '--interval', '30'],
                                    stdout=stream, stderr=subprocess.STDOUT)
        head(pr, sha)
        # Read exactly these attempts, including failed evidence, not whatever
        # happens to be the latest successful attempt in API history.
        attempts = {key: ci_compare.collect(REPO, key) for key in pair.values()}
        raw_path = output.with_name(output.stem + f'-round-{number}-raw.json')
        save(raw_path, dict(pairs=[pair], attempts=attempts))
        state['rounds'].append(dict(number=number, pair=pair, raw=str(raw_path), watch_returncode=result.returncode))
        state['stage'] = 'collected'
        save(output, state)
        report = ci_compare.compare(dict(pairs=[pair], attempts=attempts))
        if result.returncode or not report['pairs'][0]['eligible']:
            state.update(stage='failed', failure=report['pairs'][0]['errors'])
            save(output, state)
            return 1
    raw = dict(pairs=[], attempts={})
    for row in state['rounds']:
        item = json.loads(Path(row['raw']).read_text())
        raw['pairs'].extend(item['pairs'])
        raw['attempts'].update(item['attempts'])
    raw_path = output.with_name(output.stem + '-comparison-raw.json')
    save(raw_path, raw)
    report = ci_compare.compare(raw)
    report_path = output.with_name(output.stem + '-comparison.json')
    save(report_path, report)
    state.update(stage='complete' if report['comparison_complete'] else 'inconclusive',
                 completed_at=now(), comparison=str(report_path))
    save(output, state)
    print(json.dumps({k: v for k, v in report.items() if k not in ('pairs', 'limitations')}), flush=True)
    return 0 if report['comparison_complete'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pr', type=int, required=True)
    parser.add_argument('--sha', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch('[0-9a-f]{40}', args.sha):
        parser.error('--sha must be a full source SHA')
    return run(args.pr, args.sha, args.output)


if __name__ == '__main__':
    raise SystemExit(main())
