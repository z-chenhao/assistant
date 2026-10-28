"""Runs INSIDE the sandbox or Guest. Never executed in the Host's filesystem."""
import base64
import fcntl
import hashlib
import json
import os
import selectors
import signal
import stat
import subprocess
import sys
import time

MAX_FILE = 8 * 1024 * 1024
MAX_OUTPUT = 48000
started = False


def interrupted(signum, frame):
    raise KeyboardInterrupt()


for signum in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
    signal.signal(signum, interrupted)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def regular(fd):
    s = os.fstat(fd)
    if not stat.S_ISREG(s.st_mode) or s.st_nlink != 1 or s.st_size > MAX_FILE:
        raise PermissionError()


def parent(root, path, create=False):
    parts = path.split('/')
    if not path or path.startswith('/') or any(p in ('', '.', '..') for p in parts) or '\x00' in path:
        raise PermissionError()
    fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        for p in parts[:-1]:
            if create:
                try:
                    os.mkdir(p, 0o700, dir_fd=fd)
                except FileExistsError:
                    pass
            child = os.open(p, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        return fd, parts[-1]
    except BaseException:
        os.close(fd)
        raise


def file_operation(root, method, a):
    global started
    directory, name = parent(root, a['path'], method == 'write')
    temporary = None
    try:
        data, mode = None, 0o600
        try:
            fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=directory)
        except FileNotFoundError:
            if method != 'write' or 'before_hash' in a:
                raise
        else:
            with os.fdopen(fd, 'rb') as f:
                regular(f.fileno())
                mode = stat.S_IMODE(os.fstat(f.fileno()).st_mode) & 0o777
                data = f.read(MAX_FILE + 1)
                if len(data) > MAX_FILE:
                    raise PermissionError()
        if method == 'read':
            try:
                text = data.decode('utf-8')
            except UnicodeError:
                text = None
            if text is None or a.get('as_artifact', False):
                return dict(path=a['path'], artifact_data=base64.b64encode(data).decode('ascii'))
            lines = text.splitlines(keepends=True)
            offset, limit = a.get('offset', 1), a.get('limit', 200)
            chosen, size = [], 0
            for line in lines[offset-1:offset-1+limit]:
                encoded = line.encode('utf-8')
                if size + len(encoded) > MAX_OUTPUT:
                    if not chosen:
                        # Oversized lines remain recoverable using bash byte ranges.
                        chosen.append(encoded[:MAX_OUTPUT].decode('utf-8', 'ignore'))
                    break
                chosen.append(line)
                size += len(encoded)
            count = len(chosen)
            truncated = ''.join(chosen) != ''.join(lines[offset-1:])
            return dict(path=a['path'], text=''.join(chosen), hash=digest(data),
                        offset=offset, next_offset=offset+count, truncated=truncated)
        if data is None:
            if 'before_hash' in a:
                raise PermissionError()
        elif a.get('before_hash') != digest(data):
            raise PermissionError()
        if method == 'edit':
            text = data.decode('utf-8')
            if not a['old_text'] or text.count(a['old_text']) != 1:
                raise ValueError()
            replacement = text.replace(a['old_text'], a['new_text'], 1).encode('utf-8')
        else:
            replacement = a['content'].encode('utf-8')
        if len(replacement) > MAX_FILE:
            raise ValueError()
        # One serialized environment owner; directory-relative operations reject
        # symlinks. Bash runs are collected before another tool is dispatched.
        temporary = '.erii-write-' + os.urandom(16).hex()
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode, dir_fd=directory)
        with os.fdopen(fd, 'wb') as f:
            f.write(replacement)
            f.flush()
            os.fsync(f.fileno())
        started = True
        if data is None:
            os.link(temporary, name, src_dir_fd=directory, dst_dir_fd=directory, follow_symlinks=False)
            os.unlink(temporary, dir_fd=directory)
        else:
            os.rename(temporary, name, src_dir_fd=directory, dst_dir_fd=directory)
        temporary = None
        os.fsync(directory)
        return dict(path=a['path'], hash=digest(replacement), bytes=len(replacement))
    finally:
        if temporary is not None:
            os.unlink(temporary, dir_fd=directory)
        os.close(directory)


def bash(root, a):
    global started
    output = bytearray()
    truncated, timed_out = False, False
    timeout = a.get('timeout', 60)
    # Never load a personal profile, BASH_ENV, exported function or inherited
    # credential. Toolchains are explicit, read-only operator configuration.
    env = {'HOME': root, 'PATH': os.environ['PATH'], 'TMPDIR': root + '/.tmp',
           'LANG': 'C.UTF-8', 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null',
           'GIT_TERMINAL_PROMPT': '0', 'GOTOOLCHAIN': 'local', 'GOENV': 'off',
           'GOCACHE': root + '/.cache/go-build', 'GOPATH': root + '/.cache/go',
           'GOPROXY': 'off', 'GOSUMDB': 'off'}
    os.makedirs(env['TMPDIR'], mode=0o700, exist_ok=True)
    child = subprocess.Popen(['/bin/bash', '--noprofile', '--norc', '-c', a['command']],
                             cwd=root, env=env, stdin=subprocess.DEVNULL,
                             stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True)
    started = True
    selector = selectors.DefaultSelector()
    selector.register(child.stdout, selectors.EVENT_READ)
    end = time.monotonic() + timeout
    try:
        while selector.get_map() or child.poll() is None:
            remaining = end - time.monotonic()
            if remaining <= 0:
                timed_out = True
                break
            for key, _ in selector.select(min(remaining, .1)):
                data = os.read(key.fd, 8192)
                if not data:
                    selector.unregister(key.fileobj)
                else:
                    available = MAX_OUTPUT - len(output)
                    output.extend(data[:available])
                    truncated |= len(data) > available
            if child.poll() is not None and not selector.get_map():
                break
    finally:
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.wait()
        child.stdout.close()
        selector.close()
    return dict(command=a['command'], exit_code=child.returncode, output=output.decode('utf-8', 'replace'),
                timed_out=timed_out, truncated=truncated)


def run(request):
    root, method, a = request['root'], request['method'], request['args']
    os.makedirs(root, mode=0o700, exist_ok=True)
    # Guest calls (including those from concurrent cells) share one kernel
    # directory lock. This also fences a still-draining timed-out SSH command.
    directory = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        fcntl.flock(directory, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if method == 'bash':
            return bash(root, a)
        return file_operation(root, method, a)
    finally:
        os.close(directory)


try:
    print(json.dumps({'value': run(json.load(sys.stdin))}, ensure_ascii=True))
except BlockingIOError:
    print(json.dumps({'error': 'RESOURCE_LIMIT'}))
except FileNotFoundError:
    print(json.dumps({'error': 'UNKNOWN_SIDE_EFFECT' if started else 'NOT_FOUND'}))
except (ValueError, UnicodeError, KeyError):
    print(json.dumps({'error': 'UNKNOWN_SIDE_EFFECT' if started else 'INVALID_ARGUMENT'}))
except BaseException:
    print(json.dumps({'error': 'UNKNOWN_SIDE_EFFECT' if started else 'PERMISSION_DENIED'}))
