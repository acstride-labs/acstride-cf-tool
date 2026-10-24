"""Collect notices from the exact installed distributions used to build the downloader."""
import importlib.metadata as metadata
import json
from pathlib import Path
import sys
import sysconfig

destination = Path(sys.argv[1])
destination.mkdir(parents=True, exist_ok=True)
versions = {}
for name in ["curl_cffi", "cffi", "certifi", "pycparser", "pyinstaller"]:
    distribution = metadata.distribution(name)
    versions[name] = distribution.version
    files = [file for file in distribution.files or []
             if any(word in str(file).upper() for word in ["LICENSE", "COPYING", "NOTICE"])
             and ".dist-info/" in str(file)]
    if not files:
        raise RuntimeError("No license files found for " + name)
    for file in files:
        target = destination / name / str(file).split(".dist-info/", 1)[1]
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(Path(distribution.locate_file(file)).read_bytes())

python_license = Path(sysconfig.get_path("stdlib")) / "LICENSE.txt"
if not python_license.is_file():
    python_license = Path(sys.base_prefix) / "LICENSE.txt"
if not python_license.is_file():
    raise RuntimeError("Python license is missing; do not distribute this build")
(destination / "PYTHON-LICENSE.txt").write_bytes(python_license.read_bytes())
(destination / "versions.json").write_text(json.dumps(versions, indent=2), encoding="utf-8")
