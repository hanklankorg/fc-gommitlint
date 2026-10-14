#!/usr/bin/env python3
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

"""Product-free regression for the metadata relay's TCP/stdio EOF contract."""

import http.server
import pathlib
import subprocess
import sys
import threading
import unittest


class Metadata(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"fixture-host\n"
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


class RelayTest(unittest.TestCase):
    def test_backend_eof_and_frontend_half_close(self):
        helper = pathlib.Path(__file__).with_name("vm-metadata-proxy.py")
        with http.server.HTTPServer(("127.0.0.1", 0), Metadata) as server:
            worker = threading.Thread(target=server.serve_forever, daemon=True)
            worker.start()
            try:
                for close_input in (False, True):
                    with self.subTest(close_input=close_input):
                        process = subprocess.Popen(
                            [sys.executable, str(helper), str(server.server_port)],
                            stdin=subprocess.PIPE,
                            stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE,
                        )
                        try:
                            process.stdin.write(b"GET / HTTP/1.0\r\nHost: metadata\r\n\r\n")
                            process.stdin.flush()
                            if close_input:
                                process.stdin.close()
                            # Netcat stays alive here when stdin is left open;
                            # NetBSD ftp then stalls rather than receiving EOF.
                            self.assertEqual(process.wait(timeout=5), 0)
                            self.assertEqual(process.stderr.read(), b"")
                            self.assertTrue(process.stdout.read().endswith(b"fixture-host\n"))
                        finally:
                            if process.poll() is None:
                                process.kill()
                                process.wait(timeout=5)
                            process.stdin.close()
                            process.stdout.close()
                            process.stderr.close()
            finally:
                server.shutdown()
                worker.join(timeout=5)


if __name__ == "__main__":
    unittest.main()
