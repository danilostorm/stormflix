"""Exercise the measurement against actual HTTP responses, including login HTML."""
import contextlib
import importlib.util
import io
import json
import os
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("measure_home", Path(__file__).with_name("measure-home-slo.py"))
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)


class HomeMeasurementTest(unittest.TestCase):
    def measure(self, body, cache="hit", content_type="application/json"):
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(200)
                self.send_header("Content-Type", content_type)
                self.send_header("X-StormFlix-Home-Cache", cache)
                self.send_header("X-StormFlix-Catalog-Revision", "12")
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_):
                pass

        with ThreadingHTTPServer(("127.0.0.1", 0), Handler) as server:
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            env = {"STORMFLIX_BASE_URL": f"http://127.0.0.1:{server.server_port}",
                   "STORMFLIX_COOKIE": "test-only", "STORMFLIX_SAMPLES": "5", "STORMFLIX_WARMUP": "1"}
            try:
                with patch.dict(os.environ, env), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                    return measure.main()
            finally:
                server.shutdown()
                thread.join()

    def test_fast_login_html_is_not_success(self):
        self.assertNotEqual(self.measure(b"<html>Login</html>", content_type="text/html"), 0)

    def test_fast_error_json_is_not_success(self):
        self.assertNotEqual(self.measure(b'{"error":"unauthorized"}'), 0)

    def test_fast_unknown_or_miss_cache_is_not_success(self):
        for cache in ["unknown", "miss", "stale"]:
            with self.subTest(cache=cache):
                self.assertNotEqual(self.measure(json.dumps({"rows": []}).encode(), cache), 0)

    def test_stable_authenticated_hits_pass(self):
        self.assertEqual(self.measure(b'{"rows":[]}'), 0)


if __name__ == "__main__":
    unittest.main()
