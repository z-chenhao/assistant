"""Deterministic Feishu platform Fake: real HTTP, recipient checks and receipts.

No real account or external write. Lifecycle is owned by the verification scope.
"""
from email import policy
from email.parser import BytesParser
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading


class Sandbox:
    def __init__(self):
        self.messages = {}
        self.send_attempts = []
        self.uploads = []
        self.drop_response = False
        self.lock = threading.Lock()
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                if self.headers.get('Authorization') != 'Bearer sandbox-access-token':
                    self.send_error(401)
                    return
                raw = self.rfile.read(int(self.headers.get('Content-Length', '0')))
                with outer.lock:
                    if self.path == '/open-apis/im/v1/files':
                        envelope = BytesParser(policy=policy.default).parsebytes(
                            ('Content-Type: ' + self.headers['Content-Type'] + '\r\n\r\n').encode() + raw)
                        fields = {p.get_param('name', header='content-disposition'): p for p in envelope.iter_parts()}
                        payload = fields['file'].get_payload(decode=True)
                        key = 'sandbox-file-' + hashlib.sha256(payload).hexdigest()
                        outer.uploads.append(dict(name=fields['file'].get_filename(), sha256=hashlib.sha256(payload).hexdigest(), key=key))
                        result = dict(file_key=key)
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
                        if outer.drop_response:
                            self.close_connection = True
                            self.connection.close()
                            return
                    else:
                        self.send_error(404)
                        return
                body = json.dumps(dict(code=0, data=result)).encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_GET(self):
                if self.headers.get('Authorization') != 'Bearer sandbox-access-token':
                    self.send_error(401)
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
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=10)
        if self.thread.is_alive():
            raise RuntimeError('Feishu sandbox failed to stop')

    def evidence(self):
        with self.lock:
            return dict(messages=list(self.messages.values()), send_attempts=list(self.send_attempts), uploads=list(self.uploads))
