"""Owned, file-backed JetStream fixture shared by real interactive surfaces.

Build only the pinned repository test peer. No server enters the product binary,
and no online broker, credential or fixed port is needed. EOF shuts it down.
"""
import json
import os
from pathlib import Path
import selectors
import subprocess
import sys
import tempfile

from fixture_pool import Scope, current

ROOT = Path(__file__).resolve().parents[2]


class Broker:
    def __init__(self, root):
        self.root = Path(root)
        self.scope = current()
        self.owned_scope = self.scope is None
        if self.owned_scope:
            self.scope = Scope()
        self.child = None

    def __enter__(self):
        self.temp = tempfile.TemporaryDirectory(prefix='broker-', dir=self.root)
        folder = Path(self.temp.name)
        peer = folder / 'peer'
        try:
            self.scope.run(['go', 'test', '-c', '-o', str(peer), './internal/event/integration'],
                           cwd=ROOT, capture_output=True, check=True, timeout=120)
            env = {k: v for k, v in os.environ.items() if not k.startswith('ERII_')}
            env.update(ERII_BROKER_FIXTURE='1', TMPDIR=str(folder))
            self.child = self.scope.start([str(peer), '-test.run=^TestBrokerFixture$', '-test.timeout=10m'],
                                          stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                          text=True, env=env, cwd=folder)
            with selectors.DefaultSelector() as select:
                select.register(self.child.stdout, selectors.EVENT_READ)
                if not select.select(10):
                    raise RuntimeError('owned event broker handshake timed out')
            ready = json.loads(self.child.stdout.readline())
            if ready.get('version') != 1 or not ready.get('url', '').startswith('nats://127.0.0.1:'):
                raise RuntimeError('invalid owned event broker handshake')
            self.url = ready['url']
            return self
        except BaseException:
            self.__exit__(*sys.exc_info())
            raise

    def __exit__(self, kind, error, tb):
        cleanup_error = None
        if self.child is not None:
            try:
                _, stderr = self.child.communicate(timeout=10)
                if self.child.returncode != 0:
                    raise RuntimeError('owned event broker did not stop cleanly: ' + stderr)
            except BaseException as cause:
                self.child.kill()
                self.child.communicate(timeout=10)
                cleanup_error = cause
            finally:
                self.scope.retire(self.child)
        if self.owned_scope:
            self.scope.finish()
        self.temp.cleanup()
        if cleanup_error is not None and error is None:
            raise cleanup_error
        return False
