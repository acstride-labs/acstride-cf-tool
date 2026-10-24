# Anonymous public transport

This repository owns all Codeforces request/parsing behavior, including
`public-fetch.py`, its locked `downloader-requirements.txt` and license collector.
The helper accepts only anonymous public problem GETs. It never receives account
credentials or cookies and never runs a browser, server or runtime installer.

The VS Code extension's developer/CI scripts fetch this same pinned source tree
and freeze the helper with PyInstaller into a private runtime with curl_cffi,
certifi CA data and dependency notices. Go and Python are build dependencies only.
Generated executables/runtimes are ignored, never committed to either repository.

Editor package layout:

```text
resources/cf-tool/codeforces-client                 # codeforces-client.exe on Windows
resources/downloader/codeforces-workbench-downloader/
  codeforces-workbench-downloader                          # codeforces-workbench-downloader.exe on Windows
  ...private runtime/native libraries/CA data
```

`problem read ... --bundled` resolves that install-relative helper inside the
client; the extension never invokes the helper or interprets website HTML.
The helper adapter strips Python/native-library/account configuration injection
variables from its child environment. Standard proxy variables remain available.

Standalone CLI artifacts contain the Go executable and notices, not the optional
frozen helper. They use Go HTTP unless a helper is explicitly selected. Editor
VSIX artifacts include both and verify target, source revision, hashes,
standalone startup and extracted-package startup before distribution.
