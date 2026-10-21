#!/usr/bin/env python3
"""Plan/apply conservative branch cleanup; retain worktrees and unique local work."""
import argparse
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, text=True).strip()


def select(local, remote, ancestors, checked_out, prs):
    protected = {'main'} | set(checked_out) | {p['headRefName'] for p in prs if p['state'] == 'OPEN'}
    merged = {(p['headRefName'], p['headRefOid']) for p in prs if p['state'] == 'MERGED'}
    remove, keep = [], []
    for scope, branches in [('local', local), ('remote', remote)]:
        for name, sha in sorted(branches.items()):
            item = dict(scope=scope, branch=name, sha=sha)
            if name in protected:
                item['reason'] = 'main, open PR or checked-out worktree'
                keep.append(item)
            elif sha in ancestors:
                item['reason'] = 'tip is an ancestor of origin/main'
                remove.append(item)
            elif (name, sha) in merged:
                item['reason'] = 'exact tip of a merged GitHub PR'
                remove.append(item)
            else:
                item['reason'] = 'unique local work or no exact merge proof'
                keep.append(item)
    return dict(version=1, remove=remove, keep=keep)


def plan():
    git('fetch', 'origin', 'main')
    local = dict(line.split(' ', 1) for line in git('for-each-ref', '--format=%(refname:short) %(objectname)', 'refs/heads').splitlines())
    remote = {ref.removeprefix('refs/heads/'): sha for sha, ref in
              (line.split() for line in git('ls-remote', '--heads', 'origin').splitlines())}
    worktrees = [line.removeprefix('branch refs/heads/') for line in git('worktree', 'list', '--porcelain').splitlines() if line.startswith('branch refs/heads/')]
    prs = json.loads(subprocess.check_output(['gh', 'pr', 'list', '--state', 'all', '--limit', '1000',
                     '--json', 'state,headRefName,headRefOid'], cwd=ROOT, text=True))
    # Unknown objects are retained unless an exact merged PR supplies proof.
    ancestors = set()
    for sha in set(local.values()) | set(remote.values()):
        result = subprocess.run(['git', 'merge-base', '--is-ancestor', sha, 'origin/main'],
                                cwd=ROOT, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode == 0:
            ancestors.add(sha)
    report = select(local, remote, ancestors, worktrees, prs)
    report['base_sha'] = git('rev-parse', 'origin/main')
    return report


def apply(report):
    # Refresh protection/proofs immediately before mutation. Never reuse a saved
    # plan after a branch tip, worktree or PR state has changed.
    fresh = plan()
    allowed = {(x['scope'], x['branch'], x['sha']) for x in fresh['remove']}
    for item in report['remove']:
        key = item['scope'], item['branch'], item['sha']
        if key not in allowed:
            raise RuntimeError('branch changed or gained protection; regenerate the plan')
        if item['scope'] == 'local':
            if git('rev-parse', 'refs/heads/' + item['branch']) != item['sha']:
                raise RuntimeError('local branch tip changed')
            # Keep Git's own merged/upstream guard, even with exact PR proof.
            result = subprocess.run(['git', 'branch', '-d', '--', item['branch']],
                                    cwd=ROOT, capture_output=True, text=True)
            if result.returncode != 0:
                item['retained'] = 'Git merged-branch guard refused deletion'
                continue
        else:
            ref = 'refs/heads/' + item['branch']
            # A lease makes deletion fail if the remote advances after planning.
            # No commit history is force-updated and active review heads are excluded.
            git('push', '--force-with-lease=' + ref + ':' + item['sha'], 'origin', ':' + ref)
        item['deleted'] = True
    before = set(git('for-each-ref', '--format=%(refname)', 'refs/remotes/origin').splitlines())
    git('fetch', '--prune', 'origin')
    after = set(git('for-each-ref', '--format=%(refname)', 'refs/remotes/origin').splitlines())
    report['pruned_tracking_refs'] = sorted(before - after)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--evidence', type=Path, default=ROOT / 'evidence/branches.json')
    args = parser.parse_args()
    report = plan()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    try:
        if args.apply:
            apply(report)
    finally:
        args.evidence.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(dict(planned=len(report['remove']), deleted=sum(x.get('deleted', False) for x in report['remove']),
                          retained=len(report['keep']), evidence=str(args.evidence))))


if __name__ == '__main__':
    main()
