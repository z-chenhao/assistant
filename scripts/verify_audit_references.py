"""Drive the five audit CLIs through squash, fresh clones and consumer merges.

All Git mutations stay in owned disposable repositories. File-protocol clones
transfer reachable objects only; no remote service or product account is used.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

from release_package import REFERENCE_LEDGER_ORIGINALS, digest, git

ROOT = Path(__file__).resolve().parents[1]
NAMES = tuple(REFERENCE_LEDGER_ORIGINALS)
IDENTITY = ('-c', 'user.name=Synthetic Audit Fixture', '-c', 'user.email=fixture@example.invalid')


def run(root, source_sha, base_sha, raw_root=None):
    checks = []
    if raw_root:
        raw_root.mkdir(parents=True, exist_ok=True)

    def command(checkout, stage, args, expected=0):
        result = subprocess.run(args, cwd=checkout, capture_output=True, text=True,
                                env=dict(os.environ, PYTHONPATH=str(checkout / 'scripts')))
        record = dict(stage=stage, command=args, exit_code=result.returncode,
                      expected_exit=expected, stdout=result.stdout, stderr=result.stderr)
        checks.append(record)
        if result.returncode != expected:
            raise ValueError('audit history fixture failed: ' + json.dumps(record))
        return record

    def audits(checkout, stage, expected=0):
        for name in NAMES:
            report = checkout / 'evidence' / (name + '.json')
            record = command(checkout, stage, [sys.executable, 'scripts/release_audit.py',
                             '--package-ledger', 'docs/audits/' + name + '.json',
                             '--evidence', str(report)], expected)
            if expected:
                if report.exists() or 'stale current reference' not in record['stderr']:
                    raise ValueError('drift was not rejected or stale output survived')
                continue
            data = report.read_bytes()
            package = json.loads(data)['package_audit']
            if (package['release_ready'] or package['verified_requirements'] != 0 or
                    any(o['status'] != 'historical_only' or o['current_native_revalidated']
                        for o in package['observation_applicability'].values())):
                raise ValueError('historical observation was promoted by identity maintenance')
            record['audit_sha256'] = digest(data)
            if raw_root:
                shutil.copyfile(report, raw_root / (stage + '-' + name + '.json'))

    def assert_absent(checkout, revisions):
        for revision in sorted(revisions):
            found = subprocess.run(['git', '-C', str(checkout), 'cat-file', '-e', revision],
                                   capture_output=True)
            if found.returncode == 0:
                raise ValueError('fresh clone retained a pre-squash source commit')

    git(root, 'merge-base', '--is-ancestor', base_sha, source_sha)
    with tempfile.TemporaryDirectory(prefix='erii-audit-history-') as temp:
        folder = Path(temp)
        producer, consumer, main = folder / 'producer.git', folder / 'consumer', folder / 'main'
        producer.mkdir()
        git(producer, 'init', '--bare', '-q')
        git(producer, 'fetch', '-q', '--no-tags', root.resolve().as_uri(), source_sha)
        tree = git(producer, 'rev-parse', source_sha + '^{tree}').decode().strip()
        squash = git(producer, *IDENTITY, 'commit-tree', tree, '-p', base_sha,
                     '-m', 'Synthetic registered squash').decode().strip()
        git(producer, 'update-ref', 'refs/heads/main', squash)
        git(producer, 'symbolic-ref', 'HEAD', 'refs/heads/main')
        git(folder, 'clone', '-q', '--no-local', '--single-branch', producer.as_uri(), str(consumer))
        if git(consumer, 'rev-parse', 'HEAD^{tree}').decode().strip() != tree or git(consumer, 'status', '--porcelain'):
            raise ValueError('squash tree or checkout identity changed')
        ledgers = {name: json.loads((consumer / ('docs/audits/' + name + '.json')).read_text()) for name in NAMES}
        old_sources = {l['reference_maintenance']['current']['source_sha'] for l in ledgers.values()}
        assert_absent(consumer, old_sources)
        audits(consumer, 'squashed-main')
        paths = {ref.partition('#')[0] for l in ledgers.values()
                 for ref in l['reference_maintenance']['current']['references']}
        for path in sorted(paths):
            target = consumer / path
            marker = b'\n// Synthetic consumer edit\n' if target.suffix == '.go' else b'\n# Synthetic consumer edit\n'
            target.write_bytes(target.read_bytes() + marker)
        audits(consumer, 'consumer-drift', 1)
        git(consumer, 'add', '--', *sorted(paths))
        git(consumer, *IDENTITY, 'commit', '-qm', 'Synthetic consumer shared changes')
        current = git(consumer, 'rev-parse', 'HEAD').decode().strip()
        for name in NAMES:
            proposal = consumer / 'evidence' / (name + '-proposal.json')
            command(consumer, 'consumer-refresh', [sys.executable, 'scripts/maintain_audit_references.py',
                    '--ledger', 'docs/audits/' + name + '.json', '--source-sha', current,
                    '--reason', 'Synthetic consumer identity refresh; no native PASS', '--output', str(proposal)])
            refreshed = json.loads(proposal.read_text())
            if ({k: v for k, v in refreshed.items() if k != 'reference_maintenance'} !=
                    {k: v for k, v in ledgers[name].items() if k != 'reference_maintenance'}):
                raise ValueError('consumer refresh changed the original audit')
            shutil.copyfile(proposal, consumer / ('docs/audits/' + name + '.json'))
        git(consumer, 'add', '--', *['docs/audits/' + name + '.json' for name in NAMES])
        git(consumer, *IDENTITY, 'commit', '-qm', 'Synthetic consumer refresh receipts')
        # Main advances independently, then the consumer really merges it.
        advance = git(producer, *IDENTITY, 'commit-tree', tree, '-p', squash,
                      '-m', 'Synthetic main advance').decode().strip()
        git(producer, 'update-ref', 'refs/heads/main', advance, squash)
        git(consumer, 'fetch', '-q', producer.as_uri(), 'main')
        git(consumer, *IDENTITY, 'merge', '--no-edit', advance)
        merged = git(consumer, 'rev-parse', 'HEAD').decode().strip()
        if git(consumer, 'rev-parse', 'HEAD^2').decode().strip() != advance:
            raise ValueError('consumer did not merge the advanced main')
        audits(consumer, 'consumer-main-merge')
        # The consumer is itself delivered by squash, then cloned again. The
        # newly captured source commit is also absent in this second main.
        git(producer, 'fetch', '-q', consumer.as_uri(), merged)
        next_tree = git(producer, 'rev-parse', merged + '^{tree}').decode().strip()
        next_squash = git(producer, *IDENTITY, 'commit-tree', next_tree, '-p', advance,
                          '-m', 'Synthetic consumer squash').decode().strip()
        git(producer, 'update-ref', 'refs/heads/main', next_squash, advance)
        git(folder, 'clone', '-q', '--no-local', '--single-branch', producer.as_uri(), str(main))
        assert_absent(main, old_sources | {current})
        audits(main, 'consumer-squashed-main')
        command(main, 'merged-regression', [sys.executable, '-m', 'unittest',
                'test_release_baseline12.Baseline12Tests.test_all_frozen_blocks_retained_without_certification'])
    return dict(source_sha=source_sha, base_sha=base_sha, passed=True,
                first_squash=squash, consumer_merge=merged, second_squash=next_squash,
                old_source_commits_absent=sorted(old_sources), consumer_source_commit_absent=current,
                checks=checks, cleanup=True, release_ready=False,
                limits='Synthetic Git history and audit identity mechanisms only; no product/native/semantic PASS.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, help='Common suite argument; this gate checks Git/audit identity only')
    parser.add_argument('--evidence', type=Path, default=ROOT / 'evidence/audit-reference-history.json')
    parser.add_argument('--raw-reports', action='store_true', help='Retain full adjacent audit reports for local inspection')
    args = parser.parse_args()
    sha = git(ROOT, 'rev-parse', 'HEAD').decode().strip()
    base = next(iter(REFERENCE_LEDGER_ORIGINALS.values()))['commit']
    result = run(ROOT, sha, base, args.evidence.with_suffix('') if args.raw_reports else None)
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(passed=result['passed'], checks=len(result['checks']), evidence=str(args.evidence))))


if __name__ == '__main__':
    main()
