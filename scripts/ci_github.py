"""Bounded retries for evidence reads, never workflow mutations or test failures."""
import json
import re
import subprocess
import time


class ReadError(RuntimeError):
    """Safe diagnostic: provider stderr may contain signed download URLs."""


def read(*args, stdout=None):
    api = (args[:1] == ('api',) and len(args) > 1 and
           all(arg in ('--paginate', '--slurp') or
               (arg.startswith('repos/') and not any(c.isspace() for c in arg))
               for arg in args[1:]) and
           sum(arg.startswith('repos/') for arg in args[1:]) == 1)
    view = args[:2] == ('pr', 'view')
    watch = args[:2] == ('pr', 'checks') and '--watch' in args
    if not (api or view or watch):
        raise ValueError('only evidence GET, PR view and check observation may retry')
    if view or watch:
        allowed = {'--repo', '--json', '--watch', '--interval'}
        if any(arg.startswith('-') and arg not in allowed for arg in args[2:]):
            raise ValueError('unsupported PR read option')
    for attempt in range(3):
        if stdout is not None and not watch:
            stdout.seek(0)
            stdout.truncate()
        result = subprocess.run(['gh', *args], stdout=stdout or subprocess.PIPE,
                                stderr=subprocess.PIPE)
        if result.returncode == 0:
            return result
        error = result.stderr.decode('utf-8', errors='replace')
        transient = re.search(r'\bEOF\b|unexpected end of (?:file|JSON)|connection reset|'
                              r'TLS handshake timeout|i/o timeout|context deadline exceeded', error, re.I)
        if not transient:
            if watch and not error.strip():
                return result  # completed failing checks are evidence, not transport errors
            raise ReadError('GitHub evidence read failed (non-transient; stderr withheld)')
        if attempt == 2:
            raise ReadError('GitHub evidence read failed after three transport attempts (stderr withheld)')
        time.sleep(1 + attempt)


def gh(*args):
    return json.loads(read(*args).stdout)
