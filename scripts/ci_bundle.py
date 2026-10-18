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

PAYLOAD = ('bin/personal-agent', 'provider-bridge/node_modules', 'runtime/javascript/node_modules')
META = 'ci-build.json'


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def context(root):
    def command(*args):
        return subprocess.check_output(args, cwd=root).strip()
    if command('git', 'diff', 'HEAD', '--name-only'):
        raise ValueError('tracked source differs from HEAD')
    return dict(source_sha=command('git', 'rev-parse', 'HEAD').decode(),
                source_inputs_sha256=hashlib.sha256(command('git', 'ls-files', '-s', '-z')).hexdigest(),
                os=platform.system(), arch=platform.machine(),
                go=command('go', 'version').decode(), node=command('node', '--version').decode())


def create(root, archive):
    metadata = dict(version=1, context=context(root), binary_sha256=sha(root / PAYLOAD[0]))
    for name in PAYLOAD:
        if not (root / name).exists():
            raise ValueError('build payload missing: ' + name)
    with tempfile.TemporaryDirectory(prefix='erii-ci-bundle-') as temp:
        info = Path(temp) / META
        info.write_text(json.dumps(metadata, sort_keys=True) + '\n')
        with tarfile.open(archive, 'w:gz', compresslevel=1, dereference=False) as bundle:
            bundle.add(info, arcname=META)
            for name in PAYLOAD:
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
                if member.name != META and not any(member.name == p or member.name.startswith(p + '/') for p in PAYLOAD):
                    raise ValueError('unexpected archive path')
                if not (member.isfile() or member.isdir() or member.issym()):
                    raise ValueError('unexpected archive entry type')
            bundle.extractall(stage, filter='data')
        metadata = json.loads((stage / META).read_text())
        if metadata.get('version') != 1 or metadata.get('context') != wanted:
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
