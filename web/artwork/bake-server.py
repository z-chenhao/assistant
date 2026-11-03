"""Local, one-shot sprite build surface. No build tooling is loaded by the product."""
import base64
import json
from http.server import SimpleHTTPRequestHandler, HTTPServer
from pathlib import Path

TOOLS = Path(__file__).resolve().parent
ASSETS = TOOLS.parents[1] / "internal/interaction/web/assets/brand"

class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(ASSETS), **kwargs)

    def do_GET(self):
        if self.path in ("/bake.html", "/ghost-bake-mesh.js"):
            body = (TOOLS / self.path[1:]).read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8" if self.path.endswith(".html") else "text/javascript")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        super().do_GET()

    def do_POST(self):
        if self.path != '/save':
            self.send_error(404)
            return
        items = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        for name, data in items.items():
            if name not in {f'ghost-{tone}-{size}.png' for tone in ('charcoal', 'ivory') for size in (96, 256)}:
                self.send_error(400)
                return
            (ASSETS / name).write_bytes(base64.b64decode(data.split(',')[1]))
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'Complete')

HTTPServer(('127.0.0.1', 8789), Handler).serve_forever()
