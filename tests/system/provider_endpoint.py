"""Shared offline model HTTP peer for deterministic system scenarios.

Owns only the HTTP protocol; gate launch, evidence and cleanup stay with callers.
"""
import base64
import hashlib
from http.server import BaseHTTPRequestHandler
import json
import time

from verify import require


class Endpoint(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def do_GET(self):
        self.server.requests.append({"method": "GET", "auth": self.headers.get("Authorization")})
        if self.server.mode == "manual":
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        data = json.dumps({"data": [{"id": "fixture-model"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.server.requests.append({"method": "POST", "auth": self.headers.get("Authorization"),
                                     "session": self.headers.get("session_id"), "body": body})
        if self.server.mode == "auth":
            self.send_response(401)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.server.mode == "slow" and body["messages"][-1].get("content") == "Reply with OK.":
            # Actual cold-start regression: longer than the former entire Probe deadline.
            time.sleep(11)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()

        def chunk(delta, finish=None, usage=None):
            value = {"id": "fixture", "object": "chat.completion.chunk", "model": body["model"],
                     "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
            if usage:
                value["usage"] = usage
            self.wfile.write(("data: " + json.dumps(value) + "\n\n").encode())
            self.wfile.flush()

        try:
            text = str(body["messages"][-1].get("content", ""))
            images = sum(p.get("type") == "image_url" for m in body["messages"]
                         if isinstance(m.get("content"), list) for p in m["content"])
            if "until cancelled" in text:
                for _ in range(100):
                    chunk({"content": "streaming "})
                    time.sleep(.01)
            elif "dominant color of this image" in text:
                chunk({"content": "blue" if images == 1 and self.server.mode != "reject-images" else "unseen"})
                chunk({}, "stop")
            elif self.server.mode == "user-images" and "USER_IMAGE_INPUT" in text:
                payloads = [p['image_url']['url'] for m in body['messages'] if isinstance(m.get('content'),list)
                            for p in m['content'] if p.get('type') == 'image_url']
                hashes = []
                for value in payloads:
                    require(value.startswith('data:image/jpeg;base64,'),'user image did not enter the provider envelope')
                    data = base64.b64decode(value.split(',',1)[1],validate=True)
                    require(data.startswith(b'\xff\xd8\xff') and len(data)>100,'provider did not receive actual JPEG bytes')
                    hashes.append(hashlib.sha256(data).hexdigest())
                self.server.images.append(hashes)
                if self.server.hold_images:
                    self.server.received_image.set()
                    require(self.server.release_image.wait(15),'image recovery peer was not released')
                chunk({'content':'USER_IMAGE_PIXELS_RECEIVED' if images else 'USER_IMAGE_REFERENCE_ONLY'})
                chunk({},'stop')
            elif self.server.mode == "ocr" and any("OCR_INPUT" in str(m.get("content", "")) for m in body["messages"]):
                require(images == 0, "text-only OCR unexpectedly sent image pixels to the model")
                marker = max(i for i, m in enumerate(body["messages"]) if "OCR_INPUT" in str(m.get("content", "")))
                outputs = [m for m in body["messages"][marker+1:] if m["role"] == "tool"]
                if not outputs:
                    chunk({"tool_calls": [{"index": 0, "id": "read-image-text", "type": "function", "function": {
                        "name": "builtin__image_ocr", "arguments": json.dumps({"artifactRef": self.server.ocr_ref})}}]})
                    chunk({}, "tool_calls")
                else:
                    require("OCR 12345" in str(outputs[-1]["content"]), "actual OCR text did not reach the model")
                    self.server.received_ocr.set()
                    if self.server.hold_ocr:
                        require(self.server.release_ocr.wait(15), "OCR recovery peer was not released")
                    chunk({"content": "OCR_TEXT_RECEIVED"})
                    chunk({}, "stop")
            elif self.server.mode == "screenshots" and any(t["function"]["name"] == "builtin__codemode_execute" for t in body.get("tools", [])):
                self.server.images.append(images)
                count = sum(m["role"] == "tool" for m in body["messages"])
                if count < 2:
                    source = 'const app=await cua.getApp("com.apple.TextEdit");const shot=await app.getScreenshot();'
                    source += 'image(shot.image);' if count == 1 else 'text("Observed without selecting pixels.");'
                    chunk({"tool_calls": [{"index": 0, "id": f"screenshot-{count}", "type": "function", "function": {"name": "builtin__codemode_execute", "arguments": json.dumps({"source": source})}}]})
                    chunk({}, "tool_calls")
                else:
                    chunk({"content": "Screenshot selection verified."})
                    chunk({}, "stop")
            elif self.server.mode == "tasks":
                names = [item["function"]["name"] for item in body.get("tools", [])]
                root = any("general Worker executing" in str(m.get("content", "")) for m in body["messages"])
                runtime = any("Runtime task update (untrusted Worker data" in str(m.get("content", "")) for m in body["messages"])
                has_result = any(m["role"] == "tool" for m in body["messages"])
                if runtime and not root:
                    chunk({"content": "Main Task follow-up through Pi."})
                elif not has_result and (root or "builtin__task_manage" in names):
                    name = "builtin__artifact_search" if root else "builtin__task_manage"
                    arguments = {} if root else {"operation": "create", "task": "Prepare a safe independent report. Return evidence; perform no external actions."}
                    chunk({"tool_calls": [{"index": 0, "id": "task_call", "type": "function", "function": {"name": name, "arguments": json.dumps(arguments)}}]})
                else:
                    chunk({"content": "Root business result through Pi." if root else "Main Task accepted through Pi."})
                chunk({}, "stop", {"prompt_tokens": 7, "completion_tokens": 8, "total_tokens": 15})
            elif self.server.mode == "memory-disabled":
                if not any(m["role"] == "tool" for m in body["messages"]):
                    requests = [("builtin__memory_manage", {"operation": "remember", "content": "I prefer tea"}),
                                ("builtin__memory_search", {"query": "tea"})]
                    chunk({"tool_calls": [{"index": i, "id": f"disabled-{i}", "type": "function",
                                           "function": {"name": name, "arguments": json.dumps(arguments)}}
                                          for i, (name, arguments) in enumerate(requests)]})
                    chunk({}, "tool_calls")
                else:
                    chunk({"content": "Memory requires configuration."})
                    chunk({}, "stop")
            elif self.server.mode == "partial":
                chunk({"content": "partial"})
                return
            elif self.server.mode == "web-stream":
                def await_browser(name):
                    deadline = time.monotonic() + 15
                    while not (self.server.stream_gates / name).exists():
                        require(time.monotonic() < deadline, 'browser did not observe stream before releasing ' + name)
                        time.sleep(.02)

                if "STREAM_FAILURE" in text:
                    chunk({"content": "Interrupted draft only."})
                    await_browser('fail')
                    return
                require("STREAM_SUCCESS" in text, 'unexpected streaming test input')
                chunk({"content": "First chunk <img src=x onerror=unsafe()>"})
                await_browser('second')
                chunk({"content": "\nSecond chunk 中文."})
                await_browser('finish')
                chunk({"content": "\nFinal tail.\n```stew"})
                chunk({"content": 'ard-result\n{"schema_version":1,"fallback":"Rich fallback.","components":[{"type":"table","columns":["Status"],"rows":[["Done"]]}]}\n```'})
                chunk({}, "stop")
            elif body.get("tools") and not any(m["role"] == "tool" for m in body["messages"]):
                if self.server.mode == "no-tools":
                    chunk({"content": "I cannot call tools"})
                    chunk({}, "stop")
                else:
                    names = [item["function"]["name"] for item in body["tools"]]
                    name = "builtin__artifact_search" if "builtin__artifact_search" in names else names[0]
                    arguments = {} if name == "builtin__artifact_search" else {"value": "OK"}
                    chunk({"tool_calls": [{"index": 0, "id": "call_echo", "type": "function",
                                          "function": {"name": name, "arguments": json.dumps(arguments)}}]})
                    chunk({}, "tool_calls")
            else:
                chunk({"content": "PROBE_RESULT_OK" if any(m["role"] == "tool" for m in body["messages"])
                       else "ABCDEFGHIJKLMNOPQRSTUVWXYZ" if "long output" in text else "OK"})
                chunk({}, "stop", {"prompt_tokens": 7, "completion_tokens": 8, "total_tokens": 15})
            self.wfile.write(b"data: [DONE]\n\n")
        except (BrokenPipeError, ConnectionResetError):
            pass
        finally:
            self.close_connection = True
