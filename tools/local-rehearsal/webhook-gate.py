#!/usr/bin/env python3
"""Forwards only `POST /webhooks/github` to the local controller.

The Cloudflare quick tunnel exposes every path of whatever it points at. The
controller also serves /healthz and /metrics, which must not be public, so the
tunnel points here instead. This mirrors the AWS design, where the load
balancer listener rule forwards only the webhook path.

Standard library only. Binds to 127.0.0.1.
"""
import http.client
import http.server
import socketserver
import sys

WEBHOOK_PATH = "/webhooks/github"
MAX_BODY = 2 * 1024 * 1024
FORWARD_HEADERS = ("Content-Type", "X-GitHub-Event", "X-GitHub-Delivery", "X-Hub-Signature-256", "User-Agent")


def make_handler(upstream_port):
    class Gate(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        server_version = "webhook-gate"

        def log_message(self, *_args):  # keep request details out of the console
            pass

        def _reply(self, status, body=b"", content_type="text/plain"):
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Connection", "close")
            self.end_headers()
            if body:
                self.wfile.write(body)

        def _not_found(self):
            self._reply(404, b"not found\n")

        do_GET = do_PUT = do_DELETE = do_PATCH = do_HEAD = do_OPTIONS = _not_found

        def do_POST(self):
            if self.path != WEBHOOK_PATH:
                return self._not_found()
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                return self._reply(400, b"bad request\n")
            if length < 0 or length > MAX_BODY:
                return self._reply(413, b"payload too large\n")
            body = self.rfile.read(length)
            headers = {name: self.headers[name] for name in FORWARD_HEADERS if self.headers.get(name)}
            try:
                connection = http.client.HTTPConnection("127.0.0.1", upstream_port, timeout=30)
                connection.request("POST", WEBHOOK_PATH, body=body, headers=headers)
                response = connection.getresponse()
                payload = response.read()
                self._reply(response.status, payload, response.getheader("Content-Type", "text/plain"))
            except OSError:
                self._reply(502, b"controller unavailable\n")

    return Gate


class Server(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit("usage: webhook-gate.py <listen-port> <controller-port>")
    listen, upstream = int(sys.argv[1]), int(sys.argv[2])
    Server(("127.0.0.1", listen), make_handler(upstream)).serve_forever()
