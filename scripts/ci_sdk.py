#!/usr/bin/env python3
"""Provision a private CI SDK with public std archives; never modify the source SDK.

Workspace caches stay private per invocation. Only the already readable SDK
contains shared objects, compiled from fixed public standard-library roots.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time

from verify_suite import source_state

ROOT = Path(__file__).resolve().parents[1]
SDK = Path('.ci-go/sdk')
RECEIPT = Path('.ci-go/sdk.json')
HOST_CACHE = Path('.ci-go/host-cache')
PACKAGES = ['testing', 'encoding/json', 'testing/internal/testdeps', 'runtime/race', 'runtime/cgo']
WRAPPER = b'#!/bin/sh\nGODEBUG=installgoroot=all\nexport GODEBUG\nexec "${0}.real" "$@"\n'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def tree_digest(root):
    result = hashlib.sha256()
    if root.is_symlink() or not root.is_dir():
        raise ValueError('SDK root must be a real directory')
    for path in sorted(root.rglob('*')):
        if path.is_symlink() or not (path.is_dir() or path.is_file()):
            raise ValueError('SDK contains a link or special file')
        if path.is_file():
            row = [path.relative_to(root).as_posix(), path.stat().st_mode & 0o111, digest(path)]
            result.update(json.dumps(row, separators=(',', ':')).encode() + b'\n')
    return result.hexdigest()


def identity():
    base = os.environ.get('ERII_CI_BASE_GOROOT')
    command = str(Path(base) / 'bin/go') if base else 'go'
    env = dict(os.environ, GOTOOLCHAIN='local', GOENV='off')
    if base:
        env['GOROOT'] = base
    value = json.loads(subprocess.check_output([command, 'env', '-json', 'GOROOT', 'GOVERSION', 'GOOS', 'GOARCH'], env=env))
    original = Path(value.pop('GOROOT')).resolve()
    if base and original != Path(base).resolve():
        raise ValueError('base SDK path drift')
    if set(value) != {'GOVERSION', 'GOOS', 'GOARCH'} or not all(value.values()):
        raise ValueError('incomplete SDK identity')
    value['go_binary_sha256'] = digest(original / 'bin/go')
    value['source_sdk_sha256'] = tree_digest(original)
    return original, value


def objects(raw):
    decoder = json.JSONDecoder()
    while raw.strip():
        value, end = decoder.raw_decode(raw.lstrip())
        yield value
        raw = raw.lstrip()[end:]


def public_targets(raw, sdk, info):
    targets, seen = {}, set()
    prefix = f'pkg/{info["GOOS"]}_{info["GOARCH"]}_race/'
    for value in objects(raw):
        name = value.get('ImportPath', '')
        if (value.get('Standard') is not True or not name or name.startswith(('-', '/')) or
                '..' in name or any(c.isspace() for c in name) or name in seen):
            raise ValueError('not a unique public standard-library package')
        seen.add(name)
        target = value.get('Target')
        if name == 'unsafe' and not target:
            continue
        relative = prefix + name + '.a'
        if target != str(sdk / relative):
            raise ValueError('standard-library target escaped its SDK')
        targets[name] = relative
    if not all(name in targets for name in PACKAGES):
        raise ValueError('missing fixed standard-library roots')
    return targets


def publish(root, original, host_cache):
    values = dict(GOROOT=str(root / SDK), GOTOOLCHAIN='local', ERII_CI_BASE_GOROOT=str(original))
    if host_cache:
        values['GOCACHE'] = str(root / HOST_CACHE)
    if any('\n' in v or '\r' in v for v in values.values()):
        raise ValueError('unsafe SDK environment path')
    if os.environ.get('GITHUB_ENV'):
        with open(os.environ['GITHUB_ENV'], 'a') as stream:
            stream.write(''.join(f'{key}={value}\n' for key, value in values.items()))
    if os.environ.get('GITHUB_PATH'):
        with open(os.environ['GITHUB_PATH'], 'a') as stream:
            stream.write(str(root / SDK / 'bin') + '\n')


def prepare(root, host_cache=False):
    state = source_state()
    if state['source_dirty']:
        raise ValueError('SDK preparation requires frozen clean source')
    sdk, receipt = root / SDK, root / RECEIPT
    base = sdk.parent
    if base.is_symlink() or sdk.exists() or sdk.is_symlink() or receipt.exists() or receipt.is_symlink():
        raise ValueError('SDK destination is occupied or linked')
    if host_cache and ((root / HOST_CACHE).exists() or (root / HOST_CACHE).is_symlink()):
        raise ValueError('producer Host cache must start empty')
    original, info = identity()
    if original == sdk:
        raise ValueError('source SDK cannot be the disposable SDK')
    base.mkdir(parents=True, exist_ok=True)
    sdk.mkdir()
    completed = False
    started = time.monotonic()
    try:
        shutil.copytree(original, sdk, dirs_exist_ok=True, symlinks=True)
        tree_digest(sdk)  # Reject links before executing anything from the copy.
        copied = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='bootstrap-', dir=base) as temp:
            env = dict(PATH=str(sdk / 'bin') + ':/usr/bin:/bin', HOME=temp, TMPDIR=temp,
                       GOROOT=str(sdk), GOCACHE=str(Path(temp) / 'cache'), GOPATH=str(Path(temp) / 'gopath'),
                       GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GOENV='off', GOWORK='off',
                       GO111MODULE='off', CGO_ENABLED='1', GODEBUG='installgoroot=all')
            command = [str(sdk / 'bin/go')]
            raw = subprocess.check_output(command + ['list', '-deps', '-race', '-json', *PACKAGES],
                                          env=env, cwd=temp, text=True)
            targets = public_targets(raw, sdk, info)
            subprocess.run(command + ['install', '-race', *targets], env=env, cwd=temp, check=True)
        exports = {path: digest(sdk / path) for path in sorted(targets.values())}
        go = sdk / 'bin/go'
        go.rename(go.with_name('go.real'))
        go.write_bytes(WRAPPER)
        go.chmod(0o755)
        if digest(original / 'bin/go') != info['go_binary_sha256'] or state != source_state():
            raise ValueError('source SDK or checkout changed during preparation')
        value = dict(version=1, source=state, identity=info, roots=PACKAGES, exports=exports,
                     original_root=str(original), sdk_tree_sha256=tree_digest(sdk),
                     copy_seconds=copied-started, prepare_seconds=time.monotonic()-started)
        receipt.write_text(json.dumps(value, indent=2) + '\n')
        doctor(root)
        publish(root, original, host_cache)
        completed = True
        return value
    finally:
        if not completed:
            shutil.rmtree(sdk)
            receipt.unlink(missing_ok=True)


def doctor(root):
    sdk, path = root / SDK, root / RECEIPT
    if sdk.parent.is_symlink() or path.is_symlink():
        raise ValueError('linked SDK state')
    value = json.loads(path.read_text())
    keys = {'version', 'source', 'identity', 'roots', 'exports', 'original_root',
            'sdk_tree_sha256', 'copy_seconds', 'prepare_seconds'}
    if (set(value) != keys or value['version'] != 1 or value['source'] != source_state() or
            value['roots'] != PACKAGES or tree_digest(sdk) != value['sdk_tree_sha256']):
        raise ValueError('SDK receipt/source/content mismatch')
    # Never execute a path supplied by the receipt.
    original, expected = identity()
    if value['original_root'] != str(original) or value['identity'] != expected:
        raise ValueError('SDK identity mismatch')
    if ((sdk / 'bin/go').read_bytes() != WRAPPER or
            digest(sdk / 'bin/go.real') != expected['go_binary_sha256']):
        raise ValueError('SDK compiler mismatch')
    prefix = f'pkg/{expected["GOOS"]}_{expected["GOARCH"]}_race/'
    if not value['exports'] or any(not name.startswith(prefix) or '..' in name or
            not name.endswith('.a') or digest(sdk / name) != sha for name, sha in value['exports'].items()):
        raise ValueError('SDK public exports mismatch')
    return value


def signature(root):
    value = doctor(root)
    return value['sdk_tree_sha256']


def cleanup(root):
    value = doctor(root)
    shutil.rmtree(root / SDK)
    (root / RECEIPT).unlink()
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'doctor', 'cleanup', 'run'])
    parser.add_argument('--host-cache', action='store_true', help='Producer only: activate the within-attempt Host build cache')
    parser.add_argument('--evidence', type=Path)
    raw = sys.argv[1:]
    split = raw.index('--') if '--' in raw else len(raw)
    args = parser.parse_args(raw[:split])
    if args.action == 'run':
        value = doctor(ROOT)
        command = raw[split+1:]
        if not command:
            parser.error('run requires a command after --')
        env = dict(os.environ, GOROOT=str(ROOT / SDK), GOTOOLCHAIN='local',
                   GOCACHE=str(ROOT / HOST_CACHE), ERII_CI_BASE_GOROOT=value['original_root'])
        env['PATH'] = str(ROOT / SDK / 'bin') + os.pathsep + env.get('PATH', '')
        os.execvpe(command[0], command, env)
    def terminate(*_):
        raise SystemExit(143)
    signal.signal(signal.SIGTERM, terminate)
    if args.action == 'prepare':
        value = prepare(ROOT, args.host_cache)
    else:
        value = cleanup(ROOT) if args.action == 'cleanup' else doctor(ROOT)
    report = {key: value for key, value in value.items() if key != 'original_root'}
    report['scope'] = 'CI-only copied public SDK; workspace writable caches remain per invocation'
    if args.evidence:
        args.evidence.parent.mkdir(parents=True, exist_ok=True)
        args.evidence.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in ('version', 'source', 'identity', 'sdk_tree_sha256', 'prepare_seconds', 'scope')},
                     sort_keys=True))


if __name__ == '__main__':
    main()
