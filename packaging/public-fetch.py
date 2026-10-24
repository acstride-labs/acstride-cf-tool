"""Anonymous, read-only Codeforces problem downloader. No login, Cookie import or POST API."""
import json
import re
import sys
import os
from urllib.parse import urljoin, urlparse

sys.dont_write_bytecode = True
sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")
# Never read another application's curl_cffi account/config/fingerprint cache.
os.environ["IMPERSONATE_CONFIG_DIR"] = os.path.join(
    getattr(sys, "_MEIPASS", os.path.dirname(__file__)), "cfide-private-config")


def main():
    try:
        from curl_cffi import requests
        import curl_cffi
        import certifi
    except ImportError:
        print(json.dumps({"error": "missing_dependency"}))
        return
    if sys.argv[1:] == ["--self-test"]:
        from curl_cffi import Curl
        curl = Curl()
        try:
            assert curl.version().startswith(b"libcurl/")
        finally:
            curl.close()
        assert os.path.isfile(certifi.where())
        print(json.dumps({"protocol": 1, "ready": True,
                          "embedded": bool(getattr(sys, "frozen", False)),
                          "curlCffi": curl_cffi.__version__, "caBundle": True}))
        return
    if sys.argv[1:]:
        print(json.dumps({"error": "invalid_arguments"}))
        return
    request = json.loads(sys.stdin.read(4096))
    path = request.get("path", "")
    if not re.fullmatch(r"/contest/[1-9][0-9]{0,8}/problem/[A-Za-z0-9][A-Za-z0-9_-]{0,31}", path):
        print(json.dumps({"error": "invalid_problem_path"}))
        return
    url = "https://codeforces.com" + path
    with requests.Session(impersonate="chrome", verify=certifi.where()) as session:
        for _ in range(4):
            # Only anonymous, public GETs. Never persist or load any account session.
            response = session.get(url, timeout=20, allow_redirects=False, stream=True)
            try:
                if response.status_code in (301, 302, 303, 307, 308):
                    target = urljoin(url, response.headers.get("location", ""))
                    parsed = urlparse(target)
                    if parsed.scheme != "https" or parsed.netloc != "codeforces.com":
                        print(json.dumps({"error": "external_redirect"}))
                        return
                    if not re.fullmatch(r"/contest/[1-9][0-9]{0,8}/problem/[A-Za-z0-9][A-Za-z0-9_-]{0,31}", parsed.path):
                        print(json.dumps({"error": "unexpected_redirect"}))
                        return
                    url = target
                    continue
                chunks = []
                size = 0
                for chunk in response.iter_content():
                    size += len(chunk)
                    if size > 8_000_000:
                        print(json.dumps({"error": "page_too_large"}))
                        return
                    chunks.append(chunk)
                html = b"".join(chunks).decode("utf-8", errors="replace")
                print(json.dumps({"status": response.status_code, "url": url, "html": html}, ensure_ascii=False))
                return
            finally:
                response.close()
    print(json.dumps({"error": "too_many_redirects"}))


try:
    main()
except Exception:
    # Do not print environment/proxy details or response cookies in diagnostics.
    print(json.dumps({"error": "request_failed"}))
