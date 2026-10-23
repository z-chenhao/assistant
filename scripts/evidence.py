#!/usr/bin/env python3
"""Private, local-first evidence collection and exact GitHub artifact retrieval."""
import argparse
from contextlib import contextmanager
from datetime import datetime, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import tempfile
import time
import zipfile

from ci_github import gh, read
from file_digest import sha256_file

BUDGET = 1 << 20
EXCLUDED = {'node_modules', '.git', '.cache', 'ms-playwright', 'downloads', 'ci-artifacts'}
ARCHIVES = ('.zip', '.tar', '.tar.gz', '.tgz')


def now():
    return datetime.now(timezone.utc).isoformat()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as stream:
        temporary = Path(stream.name)
        stream.write((json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')) + '\n').encode())
        stream.flush()
        os.fsync(stream.fileno())
    try:
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


@contextmanager
def locked(folder):
    folder.mkdir(parents=True, exist_ok=True)
    with (folder / '.lock').open('a') as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        yield


def relative(name):
    path = PurePosixPath(name)
    if not name or path.is_absolute() or '..' in path.parts or str(path) != name:
        raise ValueError('unsafe relative evidence path')
    return path


def durable_root(root):
    checkout = Path(__file__).resolve().parents[1]
    if not root.is_absolute() or root.resolve().is_relative_to(checkout):
        raise ValueError('evidence root must be absolute and outside this checkout')
    return root.resolve()


def identity(issue, repo, sha, run, attempt):
    if (not re.fullmatch(r'[A-Za-z0-9_-]+', issue) or
            not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo) or
            any(p in ('.', '..') for p in repo.split('/')) or
            not re.fullmatch('[0-9a-f]{40}', sha) or
            not re.fullmatch(r'[A-Za-z0-9_-]+', run) or attempt < 1):
        raise ValueError('issue, repository, full SHA, run and positive attempt required')
    return dict(issue=issue, repository=repo, source_sha=sha, run=run, attempt=attempt)


def describe(path, name):
    if path.is_symlink() or not path.is_file():
        raise ValueError('evidence must be a regular file: ' + name)
    return dict(path=name, size=path.stat().st_size, sha256=sha256_file(path))


def copy_verified(source, dest, row):
    if dest.exists():
        if describe(dest, row['path']) == row:
            return
        raise ValueError('existing evidence hash mismatch: ' + row['path'])
    dest.parent.mkdir(parents=True, exist_ok=True)
    temporary = dest.with_name(dest.name + '.partial')
    shutil.copyfile(source, temporary)
    if describe(temporary, row['path']) != row:
        raise ValueError('source changed during collection')
    os.chmod(temporary, 0o444)
    temporary.replace(dest)


def collect(root, source, ident, records, profile, includes=()):
    if source.is_symlink():
        raise ValueError('symlink evidence source rejected')
    root, source = durable_root(root), source.resolve()
    if root == source or root.is_relative_to(source):
        raise ValueError('durable root must be outside source evidence')
    includes = set(includes)
    for name in includes:
        relative(name)
    folder = root / ident['issue'] / ident['repository'] / ident['source_sha'] / ident['run'] / str(ident['attempt'])
    commands = []
    for record in records:
        value = json.loads(record.read_text())
        if (not isinstance(value.get('command'), list) or not value['command'] or
                not all(isinstance(v, str) for v in value['command']) or
                type(value.get('returncode')) is not int or
                value.get('source_sha') != ident['source_sha']):
            raise ValueError('command record requires command, returncode and matching source_sha')
        commands.append(value)
    if not commands or not profile:
        raise ValueError('command record and explicit non-secret profile required')
    rows, excluded = [], []
    def walk(folder):
        for path in sorted(folder.iterdir()):
            name = path.relative_to(source).as_posix()
            denied = any(part in EXCLUDED for part in path.relative_to(source).parts) or path.name.lower().endswith(ARCHIVES)
            if denied and not any(inc == name or inc.startswith(name + '/') for inc in includes):
                excluded.append(name)
                continue
            if path.is_symlink():
                raise ValueError('symlink evidence rejected: ' + name)
            if path.is_dir():
                walk(path)
            else:
                rows.append(describe(path, name))
    walk(source)
    if not rows or not includes <= {r['path'] for r in rows}:
        raise ValueError('empty evidence or explicitly included file missing')
    content = dict(version=1, identity=ident, profile=profile, commands=commands,
                   files=rows, excluded=excluded)
    with locked(folder):
        manifest = folder / 'manifest.json'
        if manifest.exists():
            previous = verify(manifest)
            if any(previous[k] != v for k, v in content.items()):
                raise ValueError('frozen run evidence changed; use a new run/attempt')
            return manifest
        for row in rows:
            copy_verified(source / row['path'], folder / 'raw' / row['path'], row)
        content.update(created_at=now(), versions=dict(python=platform.python_version(), host=platform.platform()))
        save(manifest, content)
        verify(manifest)
    return manifest


def verify(manifest, expected=None):
    value = json.loads(manifest.read_text())
    ident = value['identity']
    if identity(ident['issue'], ident['repository'], ident['source_sha'], ident['run'], ident['attempt']) != ident:
        raise ValueError('invalid manifest identity')
    if expected is not None and ident != expected:
        raise ValueError('manifest identity mismatch')
    seen = set()
    for row in value['files']:
        name = str(relative(row['path']))
        path = manifest.parent / 'raw' / name
        if name in seen or any(p.is_symlink() for p in [path, *path.parents]):
            raise ValueError('duplicate or symlink evidence path')
        seen.add(name)
        if describe(path, name) != row:
            raise ValueError('evidence hash mismatch: ' + name)
    if not seen:
        raise ValueError('empty manifest')
    return value


def receipt(manifest, output):
    value = verify(manifest)
    result = dict(version=1, identity=value['identity'], manifest=str(manifest.resolve()),
                  manifest_sha256=sha256_file(manifest), files=len(value['files']),
                  raw_bytes=sum(r['size'] for r in value['files']),
                  scope='local supporting evidence; independent exact-SHA review/tests still required')
    encoded = (json.dumps(result, sort_keys=True) + '\n').encode()
    if len(encoded) > BUDGET:
        raise ValueError('routine receipt exceeds 1 MiB')
    output.parent.mkdir(parents=True, exist_ok=True)
    if output.exists() and output.read_bytes() != encoded:
        raise ValueError('receipt output already belongs to other evidence')
    output.write_bytes(encoded)
    return dict(path=str(output), size=len(encoded), sha256=sha256_file(output),
                transferred_bytes=0, note='local file only; upload this once, record provider receipt separately')


def export(manifest, purpose):
    if not purpose.strip():
        raise ValueError('mandatory export purpose required')
    value = verify(manifest)
    folder = manifest.parent / 'export'
    with locked(folder):
        archive, record = folder / 'evidence.zip', folder / 'receipt.json'
        if record.exists():
            prior = json.loads(record.read_text())
            if (prior['manifest_sha256'] != sha256_file(manifest) or prior['purpose'] != purpose or
                    describe(archive, archive.name) != prior['archive']):
                raise ValueError('export identity/hash mismatch')
            return prior
        if archive.exists():
            # Recover a complete export whose receipt commit was interrupted.
            with zipfile.ZipFile(archive) as existing:
                expected = ['manifest.json', *['raw/' + r['path'] for r in value['files']]]
                if existing.namelist() != expected or existing.read('manifest.json') != manifest.read_bytes():
                    raise ValueError('uncommitted export contents mismatch')
                for row in value['files']:
                    digest = hashlib.sha256()
                    with existing.open('raw/' + row['path']) as stream:
                        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                            digest.update(chunk)
                    if digest.hexdigest() != row['sha256']:
                        raise ValueError('uncommitted export hash mismatch')
        else:
            temp = folder / 'evidence.partial'
            with zipfile.ZipFile(temp, 'w', compression=zipfile.ZIP_DEFLATED) as output:
                output.write(manifest, 'manifest.json')
                for row in value['files']:
                    source = manifest.parent / 'raw' / row['path']
                    output.write(source, 'raw/' + row['path'])
                    if describe(source, row['path']) != row:
                        raise ValueError('evidence changed during export')
            temp.replace(archive)
        result = dict(manifest_sha256=sha256_file(manifest), purpose=purpose,
                      archive=describe(archive, archive.name), transferred_bytes=0)
        save(record, result)
        return result


def inventory(repo, run, attempt, run_info=None):
    # The artifact list spans attempts. Names are the existing CI SHA/attempt contract.
    identity('metadata', repo, '0' * 40, str(run), attempt)
    prefix = f'repos/{repo}/actions/runs/{run}'
    info = run_info if run_info is not None else gh('api', prefix + f'/attempts/{attempt}')
    if info['id'] != run or info['run_attempt'] != attempt:
        raise ValueError('run/attempt metadata mismatch')
    sha = info['head_sha']
    identity('metadata', repo, sha, str(run), attempt)
    pages = gh('api', '--paginate', '--slurp', prefix + '/artifacts?per_page=100')
    artifacts = [a for p in pages for a in p['artifacts']
                 if a['name'].startswith('evidence-') and a['name'].endswith(f'-{sha}-{attempt}')]
    return dict(repository=repo, run=run, attempt=attempt, source_sha=sha, artifacts=artifacts)


def artifact_archive(root, metadata, artifact_id, max_bytes=BUDGET):
    matches = [a for a in metadata['artifacts'] if a['id'] == artifact_id]
    if len(matches) != 1:
        raise ValueError('missing/duplicate artifact in selected run attempt')
    item = matches[0]
    repo, run, attempt, sha = (metadata[k] for k in ('repository', 'run', 'attempt', 'source_sha'))
    identity('cache', repo, sha, str(run), attempt)
    workflow = item.get('workflow_run', {})
    if (not item['name'].endswith(f'-{sha}-{attempt}') or workflow.get('id') != run or
            workflow.get('head_sha') != sha or type(artifact_id) is not int or artifact_id < 1):
        raise ValueError('artifact source/run/attempt mismatch')
    digest = item.get('digest') or ''
    if not re.fullmatch('sha256:[0-9a-f]{64}', digest):
        raise ValueError('artifact lacks authoritative SHA256; cannot verify reuse')
    folder = durable_root(root) / 'ci-artifacts' / repo / str(artifact_id)
    with locked(folder):
        archive, checkpoint = folder / 'artifact.zip', folder / 'receipt.json'
        frozen = dict(repository=repo, run=run, attempt=attempt, source_sha=sha,
                      id=artifact_id, name=item['name'], metadata_size=item['size_in_bytes'], digest=digest)
        if checkpoint.exists():
            prior = json.loads(checkpoint.read_text())
            if prior['identity'] != frozen or describe(archive, archive.name) != prior['archive']:
                raise ValueError('cached artifact identity/hash mismatch')
            return archive, dict(prior, cache_hit=True, downloaded_bytes=0)
        # Completed archive, interrupted before receipt commit: verify rather than download twice.
        if archive.exists():
            if sha256_file(archive) != digest[7:]:
                raise ValueError('uncommitted artifact hash mismatch')
            downloaded = 0
        else:
            if item.get('expired') or (item.get('expires_at') and
                    datetime.fromisoformat(item['expires_at'].replace('Z', '+00:00')) <= datetime.now(timezone.utc)):
                raise ValueError('artifact expired and no verified local copy')
            if item['size_in_bytes'] > max_bytes:
                raise ValueError('artifact exceeds explicit download budget; choose smaller/local evidence or obtain bulk authorization')
            transfer = folder / f'download-{time.time_ns()}.json'
            temporary = transfer.with_suffix('.partial')
            journal = dict(identity=frozen, started_at=now(), outcome='incomplete',
                           partial=temporary.name, payload_bytes=None)
            save(transfer, journal)
            # No transparent binary retry: bytes on a failed transfer remain unknown, not zero.
            try:
                with temporary.open('w+b') as stream:
                    read('api', f'repos/{repo}/actions/artifacts/{artifact_id}/zip', stdout=stream, attempts=1)
            except BaseException:
                journal.update(outcome='interrupted_or_failed', payload_bytes=temporary.stat().st_size)
                save(transfer, journal)
                raise
            downloaded = temporary.stat().st_size
            journal.update(outcome='received', payload_bytes=downloaded)
            save(transfer, journal)
            if downloaded > max_bytes or sha256_file(temporary) != digest[7:]:
                raise ValueError('download size budget or artifact SHA256 mismatch')
            temporary.replace(archive)
        result = dict(identity=frozen, archive=describe(archive, archive.name), cache_hit=False,
                      downloaded_bytes=downloaded, size_basis='actual archive payload bytes; excludes HTTP overhead/metadata')
        save(checkpoint, result)
        return archive, result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    c = sub.add_parser('collect')
    c.add_argument('--root', type=Path, required=True)
    c.add_argument('--source', type=Path, required=True)
    for name in ('issue', 'repo', 'sha', 'run', 'profile'):
        c.add_argument('--' + name, required=True)
    c.add_argument('--attempt', type=int, required=True)
    c.add_argument('--record', type=Path, action='append', required=True)
    c.add_argument('--include', action='append', default=[], help='explicit required archive, relative to source')
    for action in ('verify', 'receipt', 'export'):
        p = sub.add_parser(action)
        p.add_argument('--manifest', type=Path, required=True)
        p.add_argument('--manifest-sha256', required=True, help='trusted digest from author receipt')
        if action == 'receipt':
            p.add_argument('--output', type=Path, required=True)
        if action == 'export':
            p.add_argument('--purpose', required=True)
    p = sub.add_parser('ci-list')
    p.add_argument('--repo', required=True)
    p.add_argument('--run', type=int, required=True)
    p.add_argument('--attempt', type=int, required=True)
    p.add_argument('--output', type=Path, required=True)
    p = sub.add_parser('ci-fetch')
    p.add_argument('--root', type=Path, required=True)
    p.add_argument('--metadata', type=Path, required=True)
    p.add_argument('--id', type=int, required=True)
    p.add_argument('--max-bytes', type=int, default=BUDGET)
    args = parser.parse_args()
    if hasattr(args, 'manifest') and sha256_file(args.manifest) != args.manifest_sha256:
        raise ValueError('manifest SHA256 mismatch')
    if args.action == 'collect':
        manifest = collect(args.root, args.source, identity(args.issue, args.repo, args.sha, args.run, args.attempt),
                           args.record, args.profile, args.include)
        result = dict(manifest=str(manifest), sha256=sha256_file(manifest))
    elif args.action == 'verify':
        value = verify(args.manifest)
        result = dict(verified=True, identity=value['identity'], files=len(value['files']))
    elif args.action == 'receipt':
        result = receipt(args.manifest, args.output)
    elif args.action == 'export':
        result = export(args.manifest, args.purpose)
    elif args.action == 'ci-list':
        result = inventory(args.repo, args.run, args.attempt)
        save(args.output, result)
    else:
        path, result = artifact_archive(args.root, json.loads(args.metadata.read_text()), args.id, args.max_bytes)
        result = dict(result, path=str(path))
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
