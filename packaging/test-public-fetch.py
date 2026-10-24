"""Offline protocol test: one transport session serves multiple NDJSON requests."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


class PublicFetchTests(unittest.TestCase):
    def test_persistent_session_and_one_shot_compatibility(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            marker = root / "sessions"
            (root / "certifi.py").write_text("def where(): return __file__\n")
            (root / "curl_cffi.py").write_text('''
import os
from types import SimpleNamespace
class Response:
    status_code = 200
    headers = {}
    def iter_content(self): return [b'<div>statement</div>']
    def close(self): pass
class Session:
    def __init__(self, **kwargs):
        with open(os.environ['SESSION_MARKER'], 'a') as f: f.write('session\\n')
    def __enter__(self): return self
    def __exit__(self, *args): pass
    def get(self, url, **kwargs):
        assert kwargs['allow_redirects'] is False
        return Response()
requests = SimpleNamespace(Session=Session)
''')
            env = dict(os.environ, PYTHONPATH=folder, SESSION_MARKER=str(marker))
            script = Path(__file__).with_name("public-fetch.py")
            payload = '\n'.join(json.dumps({"path": p}) for p in ["/contest/1/problem/A", "/contest/1/problem/B", "/enter"]) + '\n'
            result = subprocess.run([sys.executable, str(script), "--serve"], input=payload, text=True, capture_output=True, env=env, check=True)
            output = [json.loads(line) for line in result.stdout.splitlines()]
            self.assertEqual([v.get('status') for v in output], [200, 200, None])
            self.assertEqual(output[-1]['error'], 'invalid_problem_path')
            self.assertEqual(marker.read_text().splitlines(), ['session'])
            result = subprocess.run([sys.executable, str(script)], input=json.dumps({"path": "/contest/1/problem/A"}), text=True, capture_output=True, env=env, check=True)
            self.assertEqual(json.loads(result.stdout)['status'], 200)


if __name__ == '__main__':
    unittest.main()
