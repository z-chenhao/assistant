#!/usr/bin/env python3
"""Share one build within a workflow, authenticated by its producer's digest.

This is not a cross-run cache. A consumer requires the full source SHA, tracked
input digest, OS/architecture, Go and Node versions of its own checkout.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile

PAYLOAD = ('bin/erii', 'provider-bridge/node_modules', 'runtime/javascript/node_modules', '.ci-go/host-cache')
META = 'ci-build.json'


def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def context(root):
    from ci_sdk import signature
    def command(*args):
        return subprocess.check_output(args, cwd=root).strip()
    if command('git', 'diff', 'HEAD', '--name-only'):
        raise ValueError('tracked source differs from HEAD')
    return dict(source_sha=command('git', 'rev-parse', 'HEAD').decode(),
                source_inputs_sha256=hashlib.sha256(command('git', 'ls-files', '-s', '-z')).hexdigest(),
                os=platform.system(), arch=platform.machine(),
                image_os=os.environ.get('ImageOS'), image_version=os.environ.get('ImageVersion'),
                go=command('go', 'version').decode(), node=command('node', '--version').decode(),
                sdk_public_sha256=signature(root), workflow_run_id=os.environ.get('GITHUB_RUN_ID'),
                workflow_attempt=os.environ.get('GITHUB_RUN_ATTEMPT'))


def create(root, archive):
    metadata = dict(version=2, context=context(root), binary_sha256=sha(root / PAYLOAD[0]))
    for name in PAYLOAD:
        if not (root / name).exists():
            raise ValueError('build payload missing: ' + name)
    if ((root / PAYLOAD[0]).is_symlink() or
            any(not (root / name).is_dir() or (root / name).is_symlink() for name in PAYLOAD[1:])):
        raise ValueError('payload roots must have their declared types')
    if any(p.is_symlink() or not (p.is_file() or p.is_dir()) for p in (root / PAYLOAD[-1]).rglob('*')):
        raise ValueError('Host build cache cannot contain links or special files')
    with tempfile.TemporaryDirectory(prefix='erii-ci-bundle-') as temp:
        info = Path(temp) / META
        info.write_text(json.dumps(metadata, sort_keys=True) + '\n')
        with tarfile.open(archive, 'w:gz', compresslevel=1, dereference=False) as bundle:
            bundle.add(info, arcname=META)
            for name in PAYLOAD:
                # Go can hard-link a cached executable to its build output.
                # Cache members contain no symlinks and serialize as regular
                # bytes, while npm's declared relative links retain semantics.
                bundle.dereference = name == PAYLOAD[-1]
                bundle.add(root / name, arcname=name)
    return sha(archive)


def restore(root, archive, expected):
    if not re.fullmatch('[0-9a-f]{64}', expected) or sha(archive) != expected:
        raise ValueError('missing or mismatched producer digest')
    wanted = context(root)
    with tempfile.TemporaryDirectory(prefix='erii-ci-restore-') as temp:
        stage = Path(temp)
        with tarfile.open(archive) as bundle:
            for member in bundle.getmembers():
                if any(p in ('', '.', '..') for p in member.name.rstrip('/').split('/')):
                    raise ValueError('noncanonical archive path')
                if member.name != META and not any(member.name == p or member.name.startswith(p + '/') for p in PAYLOAD):
                    raise ValueError('unexpected archive path')
                if not (member.isfile() or member.isdir() or member.issym()):
                    raise ValueError('unexpected archive entry type')
                if member.issym() and (member.name == PAYLOAD[-1] or member.name.startswith(PAYLOAD[-1] + '/')):
                    raise ValueError('Host build cache cannot contain links')
            bundle.extractall(stage, filter='data')
        metadata = json.loads((stage / META).read_text())
        if metadata.get('version') != 2 or metadata.get('context') != wanted:
            raise ValueError('build context mismatch')
        if (stage / PAYLOAD[0]).is_symlink() or any(not (stage / name).is_dir() or (stage / name).is_symlink() for name in PAYLOAD[1:]):
            raise ValueError('payload roots must have their declared types')
        if metadata.get('binary_sha256') != sha(stage / PAYLOAD[0]):
            raise ValueError('binary digest mismatch')
        for name in PAYLOAD:
            if not (stage / name).exists() or (root / name).exists():
                raise ValueError('missing payload or occupied destination: ' + name)
        for name in PAYLOAD:
            destination = root / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.move(stage / name, destination)
    return metadata


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['create', 'restore'])
    parser.add_argument('--archive', type=Path, default=Path('ci-build.tar.gz'))
    parser.add_argument('--digest', default=os.environ.get('BUILD_DIGEST', ''))
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    if args.action == 'create':
        digest = create(root, args.archive)
        if os.environ.get('GITHUB_OUTPUT'):
            with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
                output.write('digest=' + digest + '\n')
        print(json.dumps(dict(archive_sha256=digest, context=context(root))))
    else:
        print(json.dumps(restore(root, args.archive, args.digest)))


if __name__ == '__main__':
    main()
