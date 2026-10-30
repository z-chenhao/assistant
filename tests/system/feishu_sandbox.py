"""Deterministic Feishu platform Fake: real HTTP, recipient checks and receipts.

No real account or external write. Lifecycle is owned by the verification scope.
"""
from email import policy
from email.parser import BytesParser
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
import time


class Sandbox:
    def __init__(self, send_delay=0, upload_delay=0, managed_auth=False, token_ttl=7200):
        self.managed_auth = managed_auth
        self.token_ttl = token_ttl
        self.auth_requests = 0
        self.auth_rejections = 0
        self.current_token = None if managed_auth else 'sandbox-access-token'
        self.token_expiry = 0
        self.messages = {}
        self.send_attempts = []
        self.uploads = []
        self.drop_response = False
        self.send_delay = send_delay
        self.upload_delay = upload_delay
        self.operations = []
        self.send_started = threading.Event()
        self.upload_started = threading.Event()
        self.stopping = threading.Event()
        self.lock = threading.Lock()
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def json_reply(self, value, status=200):
                body = json.dumps(value).encode()
                self.send_response(status)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def authorized(self):
                with outer.lock:
                    valid = outer.current_token is not None and self.headers.get('Authorization') == 'Bearer ' + outer.current_token
                    valid = valid and (not outer.managed_auth or time.monotonic() < outer.token_expiry)
                    if not valid:
                        outer.auth_rejections += 1
                if not valid:
                    self.json_reply(dict(code=99991663, msg='token rejected'), 401)
                return valid

            def do_POST(self):
                raw = self.rfile.read(int(self.headers.get('Content-Length', '0')))
                if self.path == '/open-apis/auth/v3/tenant_access_token/internal':
                    if not outer.managed_auth:
                        self.send_error(404)
                        return
                    q = json.loads(raw)
                    if q != dict(app_id='sandbox-app', app_secret='sandbox-app-secret'):
                        self.json_reply(dict(code=10003, msg='application credentials rejected'))
                        return
                    with outer.lock:
                        outer.auth_requests += 1
                        outer.current_token = 'sandbox-managed-token-' + str(outer.auth_requests)
                        outer.token_expiry = time.monotonic() + outer.token_ttl
                        token = outer.current_token
                    self.json_reply(dict(code=0, tenant_access_token=token, expire=outer.token_ttl))
                    return
                if not self.authorized():
                    return
                drop = False
                with outer.lock:
                    if self.path == '/open-apis/im/v1/files':
                        envelope = BytesParser(policy=policy.default).parsebytes(
                            ('Content-Type: ' + self.headers['Content-Type'] + '\r\n\r\n').encode() + raw)
                        fields = {p.get_param('name', header='content-disposition'): p for p in envelope.iter_parts()}
                        payload = fields['file'].get_payload(decode=True)
                        key = 'sandbox-file-' + hashlib.sha256(payload).hexdigest()
                        outer.uploads.append(dict(name=fields['file'].get_filename(), sha256=hashlib.sha256(payload).hexdigest(), key=key))
                        result = dict(file_key=key)
                        kind, delay = 'upload', outer.upload_delay
                    elif self.path == '/open-apis/im/v1/messages?receive_id_type=chat_id':
                        message = json.loads(raw)
                        outer.send_attempts.append(message)
                        if message['receive_id'] != 'bound-chat':
                            self.send_error(403)
                            return
                        key = message['uuid']
                        old = outer.messages.get(key)
                        if old and old != message:
                            self.send_error(409)
                            return
                        outer.messages[key] = message
                        result = dict(message_id='remote-' + key)
                        kind, delay = 'send', outer.send_delay
                        drop = outer.drop_response
                    else:
                        self.send_error(404)
                        return
                    operation = dict(kind=kind, delay_seconds=delay, started_at=time.time(), finished_at=None)
                    outer.operations.append(operation)
                    (outer.send_started if kind == 'send' else outer.upload_started).set()
                # Delay only the platform acknowledgement, after recording the
                # effect. Never hold the ledger lock while a client is waiting.
                outer.stopping.wait(delay)
                with outer.lock:
                    operation['finished_at'] = time.time()
                if drop:
                    self.close_connection = True
                    self.connection.close()
                    return
                body = json.dumps(dict(code=0, data=result)).encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                try:
                    self.wfile.write(body)
                except (BrokenPipeError, ConnectionResetError):
                    pass  # A fenced/cancelled client can leave a real effect.

            def do_GET(self):
                if not self.authorized():
                    return
                if self.path == '/open-apis/bot/v3/info/':
                    self.json_reply(dict(code=0, bot=dict(open_id='sandbox-bot', activate_status=2)))
                    return
                if self.path == '/open-apis/im/v1/messages/media-message/resources/file-key?type=file':
                    body = b'Inbound attachment from the authenticated Feishu owner.\n'
                    self.send_response(200)
                    self.send_header('Content-Type', 'text/plain')
                    self.send_header('Content-Length', str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                self.send_error(404)

        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)

    def __enter__(self):
        self.thread.start()
        self.url = 'http://127.0.0.1:' + str(self.server.server_port)
        return self

    def __exit__(self, *_):
        self.stopping.set()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=10)
        if self.thread.is_alive():
            raise RuntimeError('Feishu sandbox failed to stop')

    def evidence(self):
        with self.lock:
            return dict(messages=list(self.messages.values()), send_attempts=list(self.send_attempts), uploads=list(self.uploads), operations=[dict(op) for op in self.operations])
