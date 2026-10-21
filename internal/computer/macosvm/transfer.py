# Guest-only transfer helper. Never accepts Host paths. Every component is
# opened relative to an O_NOFOLLOW directory descriptor, including the root.
import base64, json, os, stat, sys

started = False

def run(a):
    global started
    root = a['root']
    parts = a['path'].split('/')
    if len(parts) != 2 or parts[0] not in ('imports', 'downloads') or parts[1] in ('', '.', '..') or any(c in a['path'] for c in '\\\x00\r\n:'):
        raise ValueError('path')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for component in root.strip('/').split('/'):
            if component in ('', '.', '..'): raise ValueError('root')
            if component == 'EriiTransfer':
                try: os.mkdir(component, 0o700, dir_fd=fd)
                except FileExistsError: pass
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd); fd = child
            if component == "EriiTransfer":
                info=os.fstat(fd)
                if info.st_uid != os.getuid() or info.st_mode & 0o077: raise ValueError("root permissions")
        try: os.mkdir(parts[0], 0o700, dir_fd=fd)
        except FileExistsError: pass
        child = os.open(parts[0], os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
        os.close(fd); fd = child
        info=os.fstat(fd)
        if info.st_uid != os.getuid() or info.st_mode & 0o077: raise ValueError("directory permissions")
        if a['operation'] == 'write':
            data = base64.b64decode(a['data'], validate=True)
            if len(data) > 8 * 1024 * 1024: raise ValueError('size')
            flags = os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW | os.O_NONBLOCK
            flags |= 0 if a['overwrite'] else os.O_EXCL
            out = os.open(parts[1], flags, 0o600, dir_fd=fd)
            try:
                info = os.fstat(out)
                if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1: raise ValueError('file')
                started = True
                os.ftruncate(out, 0)
                view = memoryview(data)
                while view: view = view[os.write(out, view):]
                os.fsync(out)
            finally: os.close(out)
            return {'data': ''}
        source = os.open(parts[1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        try:
            info = os.fstat(source)
            if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_size > 8 * 1024 * 1024: raise ValueError('file')
            with os.fdopen(source, 'rb', closefd=False) as f: data = f.read(8 * 1024 * 1024 + 1)
            if len(data) > 8 * 1024 * 1024: raise ValueError('size')
            return {'data': base64.b64encode(data).decode()}
        finally: os.close(source)
    finally: os.close(fd)

try: print(json.dumps(run(json.loads(sys.argv[1] if len(sys.argv)>1 else sys.stdin.read(12 * 1024 * 1024)))))
except Exception: print(json.dumps({'error': 'UNKNOWN_SIDE_EFFECT' if started else 'PERMISSION_DENIED'}))
