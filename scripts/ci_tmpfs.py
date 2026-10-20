#!/usr/bin/env python3
"""Own one bounded, executable tmpfs for disposable provider fixtures only.

Unit/lint and the complete race inventory retain ordinary disk storage. This
changes neither SQLite's journal/sync policy nor any test command or count.
"""
import argparse
from contextlib import closing
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import uuid

RECEIPT = Path('evidence/fixture-storage.json')
LIMIT = 2 * 1024 ** 3


def command(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout


def root_path():
    parent = Path(os.environ['RUNNER_TEMP'])
    if not parent.is_absolute() or any(c in str(parent) for c in '\r\n'):
        raise ValueError('unsafe runner temporary directory')
    parent = parent.resolve(strict=True)
    if parent.stat().st_uid != os.getuid():
        raise ValueError('runner temporary directory has another owner')
    return parent / 'erii-provider-fixtures'


def write(receipt):
    RECEIPT.parent.mkdir(parents=True, exist_ok=True)
    temporary = RECEIPT.with_suffix('.tmp')
    temporary.write_text(json.dumps(receipt, indent=2) + '\n')
    temporary.replace(RECEIPT)


def export(name, value):
    if any(c in value for c in '\r\n'):
        raise ValueError('unsafe environment value')
    with open(os.environ['GITHUB_ENV'], 'a') as stream:
        stream.write(name + '=' + value + '\n')


def mount_info(root):
    result = subprocess.run(['findmnt', '--json', '--mountpoint', str(root),
                             '--output', 'TARGET,SOURCE,FSTYPE,OPTIONS'],
                            capture_output=True, text=True)
    if result.returncode == 1 and not result.stdout.strip():
        return None
    if result.returncode:
        raise ValueError('cannot establish fixture mount identity')
    rows = json.loads(result.stdout)['filesystems']
    if len(rows) != 1 or rows[0]['target'] != str(root):
        raise ValueError('ambiguous fixture mount')
    return rows[0]


def owned(root, receipt, info):
    if (root.is_symlink() or receipt['root'] != str(root) or
            info['source'] != receipt['source'] or info['fstype'] != 'tmpfs'):
        raise ValueError('fixture mount ownership changed')


def doctor(root, receipt):
    info = mount_info(root)
    if info is None:
        raise ValueError('fixture tmpfs is missing')
    owned(root, receipt, info)
    stat = root.stat()
    options = set(info['options'].split(','))
    size = os.statvfs(root)
    if (stat.st_dev == receipt['backing'][0] or stat.st_uid != os.getuid() or
            stat.st_mode & 0o777 != 0o700 or 'noexec' in options or
            not {'nodev', 'nosuid', 'rw'} <= options or
            size.f_blocks * size.f_frsize != LIMIT):
        raise ValueError('fixture mount properties changed')
    with tempfile.TemporaryDirectory(dir=root) as directory:
        path = Path(directory)
        probe = path / 'exec-probe'
        probe.write_text('#!/bin/sh\nexit 0\n')
        probe.chmod(0o700)
        command(str(probe))
        with probe.open('rb') as stream:
            os.fsync(stream.fileno())
        database = path / 'probe.db'
        with closing(sqlite3.connect(database)) as db, db:
            if db.execute('PRAGMA journal_mode=WAL').fetchone()[0] != 'wal':
                raise ValueError('fixture filesystem rejected WAL')
            db.execute('CREATE TABLE proof(value TEXT)')
            db.execute("INSERT INTO proof VALUES ('committed')")
        with closing(sqlite3.connect(database)) as db:
            if db.execute('SELECT value FROM proof').fetchone() != ('committed',):
                raise ValueError('fixture committed bytes were lost')
    return dict(mount=info, byte_limit=LIMIT, execute=True, fsync=True, sqlite_wal=True)


def cleanup():
    root = root_path()
    if not RECEIPT.exists():
        if root.exists() or root.is_symlink():
            raise ValueError('unrecorded fixture directory; refusing cleanup')
        return
    receipt = json.loads(RECEIPT.read_text())
    if receipt['root'] != str(root) or root.is_symlink():
        raise ValueError('fixture path changed')
    info = mount_info(root)
    remaining = []
    if info is None and receipt.get('doctor') and not receipt.get('cleanup'):
        raise ValueError('verified fixture mount disappeared before cleanup')
    if info is not None:
        owned(root, receipt, info)
        remaining = sorted(p.name for p in root.iterdir())
        # Never use force/lazy unmount: a live user of this mount fails cleanup.
        command('sudo', '-n', 'umount', str(root))
    if root.exists():
        stat = root.stat()
        if [stat.st_dev, stat.st_ino] != receipt['backing']:
            raise ValueError('fixture backing directory changed')
        root.rmdir()
    export('TMPDIR', receipt['previous_tmpdir'])
    receipt.update(cleanup=not remaining, remaining_entries=remaining)
    write(receipt)
    if remaining:
        raise ValueError('provider fixtures left disposable state')


def setup():
    if sys.platform != 'linux':
        raise ValueError('CI tmpfs requires Linux')
    root = root_path()
    if RECEIPT.exists():
        raise ValueError('fixture receipt already exists')
    previous = os.environ.get('TMPDIR') or tempfile.gettempdir()
    if not Path(previous).is_absolute() or any(c in previous for c in '\r\n'):
        raise ValueError('unsafe previous temporary directory')
    export('TMPDIR', previous)  # Prove the Actions environment file before mounting.
    # Exclusive creation prevents mounting over existing or user-owned state.
    root.mkdir(mode=0o700)
    stat = root.stat()
    receipt = dict(version=1, root=str(root), source='erii-fixtures-' + uuid.uuid4().hex,
                   backing=[stat.st_dev, stat.st_ino], previous_tmpdir=previous,
                   cleanup=False, scope='providers only; unit/lint and race remain on disk')
    try:
        write(receipt)
    except BaseException:
        root.rmdir()
        raise
    try:
        command('sudo', '-n', 'mount', '-t', 'tmpfs', '-o',
                f'size={LIMIT},mode=0700,uid={os.getuid()},gid={os.getgid()},nodev,nosuid',
                receipt['source'], str(root))
        receipt['doctor'] = doctor(root, receipt)
        write(receipt)
        export('TMPDIR', str(root))
    except BaseException:
        cleanup()
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('setup', 'cleanup'))
    args = parser.parse_args()
    (setup if args.action == 'setup' else cleanup)()
    if RECEIPT.exists():
        print(RECEIPT.read_text(), end='')


if __name__ == '__main__':
    main()
