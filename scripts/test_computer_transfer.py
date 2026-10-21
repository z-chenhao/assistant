"""Guest helper security: execute its real openat/O_NOFOLLOW boundary locally."""
import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HELPER = Path(__file__).resolve().parents[1] / 'internal/computer/macosvm/transfer.py'


class TransferTests(unittest.TestCase):
    def test_roundtrip_overwrite_traversal_symlinks_hardlinks_and_fifo(self):
        with tempfile.TemporaryDirectory() as temp:
            parent = Path(temp).resolve()
            root = parent / 'EriiTransfer'
            def call(operation, path, data=b'fixture', overwrite=False):
                return json.loads(subprocess.check_output([sys.executable, str(HELPER)], input=json.dumps({
                    'root': str(root), 'operation': operation, 'path': path,
                    'overwrite': overwrite, 'data': base64.b64encode(data).decode()}).encode(), timeout=3))
            self.assertEqual(call('write', 'imports/test'), {'data': ''})
            self.assertEqual(base64.b64decode(call('read', 'imports/test')['data']), b'fixture')
            self.assertIn('error', call('write', 'imports/test'))
            self.assertEqual(call('write', 'imports/test', b'replaced', True), {'data': ''})
            for path in ('../secret', '/etc/passwd', 'imports/../secret', 'imports/a/b', 'imports/..', 'imports/a\\b'):
                self.assertIn('error', call('read', path), path)
            secret = parent / 'private'
            secret.write_bytes(b'private')
            (root / 'imports/symlink').symlink_to(secret)
            os.link(secret, root / 'imports/hardlink')
            os.mkfifo(root / 'imports/fifo')
            for name in ('symlink', 'hardlink', 'fifo'):
                for operation in ('read', 'write'):
                    self.assertIn('error', call(operation, 'imports/' + name, overwrite=True), (operation, name))
            self.assertEqual(secret.read_bytes(), b'private')
            root.chmod(0o777)
            self.assertIn('error', call('read','imports/test'))
            root.chmod(0o700)
            (root / 'downloads').symlink_to(parent)
            self.assertIn('error', call('write', 'downloads/private'))
            self.assertEqual(secret.read_bytes(), b'private')

    def test_root_symlink_is_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            parent = Path(temp).resolve()
            (parent / 'target').mkdir()
            (parent / 'EriiTransfer').symlink_to(parent / 'target')
            result = subprocess.check_output([sys.executable, str(HELPER)], input=json.dumps({
                'root': str(parent / 'EriiTransfer'), 'operation': 'write', 'path': 'imports/x',
                'overwrite': False, 'data': ''}).encode(), timeout=3)
            self.assertIn('error', json.loads(result))
            self.assertEqual(list((parent / 'target').iterdir()), [])
