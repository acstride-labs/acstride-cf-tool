"""Build and verify versioned standalone CLI release archives (stdlib only)."""
import argparse
import hashlib
import gzip
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [(system, arch) for system in ("linux", "darwin", "windows") for arch in ("amd64", "arm64")]
NUMBER = r"(?:0|[1-9][0-9]*)"
IDENTIFIER = rf"(?:{NUMBER}|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
TAG = re.compile(rf"v({NUMBER}\.{NUMBER}\.{NUMBER}(?:-{IDENTIFIER}(?:\.{IDENTIFIER})*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?)\Z")


def version(tag):
    match = TAG.fullmatch(tag)
    if not match:
        raise ValueError("Release tag must be vMAJOR.MINOR.PATCH, optionally with a SemVer prerelease/build suffix")
    return match.group(1)


def digest(file):
    with file.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def run(args, **kwargs):
    return subprocess.check_output(args, text=True, cwd=ROOT, **kwargs).strip()


def pack(folder, destination, system):
    # Normalize archive timestamps/ownership so retries of the same build do not
    # differ only because an asset was compressed at a different wall-clock time.
    if system == "windows":
        with zipfile.ZipFile(destination, "w", zipfile.ZIP_DEFLATED) as archive:
            for file in sorted(folder.iterdir()):
                info = zipfile.ZipInfo(f"{folder.name}/{file.name}", (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (0o100000 | (0o755 if file.name in ("acstride-cf", "acstride-cf.exe") else 0o644)) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, file.read_bytes())
    else:
        def normalized(info):
            info.mtime = info.uid = info.gid = 0
            info.uname = info.gname = ""
            info.mode = 0o755 if info.isdir() or Path(info.name).name == "acstride-cf" else 0o644
            return info
        with destination.open("wb") as output:
            with gzip.GzipFile(fileobj=output, mode="wb", filename="", mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode="w") as archive:
                    archive.add(folder, arcname=folder.name, filter=normalized)


def build(tag, system, arch, output):
    release_version = version(tag)
    if (system, arch) not in TARGETS:
        raise ValueError("Unsupported release platform")
    go = os.environ.get("GO", "go")
    revision = run(["git", "rev-parse", "HEAD"])
    output.mkdir(parents=True, exist_ok=True)
    name = f"acstride-cf_{release_version}_{system}_{arch}"
    destination = output / (name + (".zip" if system == "windows" else ".tar.gz"))
    with tempfile.TemporaryDirectory(prefix="acstride-release-") as temporary:
        folder = Path(temporary) / name
        folder.mkdir()
        binary = folder / ("acstride-cf.exe" if system == "windows" else "acstride-cf")
        environment = {**os.environ, "GOOS": system, "GOARCH": arch, "CGO_ENABLED": "0", "GOTOOLCHAIN": "local"}
        subprocess.run([go, "build", "-trimpath", "-ldflags",
                        f"-s -w -X github.com/acstride-labs/acstride-cf-tool/internal/cli.Version={release_version}",
                        "-o", str(binary), "./cmd/acstride-cf"], cwd=ROOT, env=environment, check=True)
        # Notice generation is a build-host operation, even when the binary is cross-compiled.
        host_env = {k: v for k, v in os.environ.items() if k not in ("GOOS", "GOARCH", "GOARM")}
        if Path(go).is_absolute():
            host_env["PATH"] = str(Path(go).parent) + os.pathsep + host_env.get("PATH", "")
        subprocess.run([go, "run", "./tools/notices", "-output", str(folder / "THIRD_PARTY_NOTICES.txt")],
                       cwd=ROOT, env=host_env, check=True)
        shutil.copyfile(ROOT / "LICENSE", folder / "LICENSE")
        shutil.copyfile(ROOT / "README.md", folder / "README.md")
        (folder / "BUILD.json").write_text(json.dumps({"version": release_version, "tag": tag,
            "revision": revision, "protocol": 1, "os": system, "arch": arch,
            "go": run([go, "version"]), "sha256": digest(binary)}, indent=2) + "\n", encoding="utf-8")
        binary.chmod(0o755)
        pack(folder, destination, system)
    print(destination)


def verify_archives(tag, output):
    release_version = version(tag)
    checksums = []
    revisions = set()
    required = {"LICENSE", "README.md", "THIRD_PARTY_NOTICES.txt", "BUILD.json"}
    for system, arch in TARGETS:
        name = f"acstride-cf_{release_version}_{system}_{arch}"
        binary = "acstride-cf.exe" if system == "windows" else "acstride-cf"
        archive_path = output / (name + (".zip" if system == "windows" else ".tar.gz"))
        if system == "windows":
            with zipfile.ZipFile(archive_path) as archive:
                files = {item.filename: archive.read(item) for item in archive.infolist() if not item.is_dir()}
        else:
            with tarfile.open(archive_path) as archive:
                files = {item.name: archive.extractfile(item).read() for item in archive.getmembers() if item.isfile()}
                mode = archive.getmember(f"{name}/{binary}").mode
                if not mode & 0o111:
                    raise ValueError("Unix binary lost executable permission")
        if set(files) != {f"{name}/{file}" for file in required | {binary}}:
            raise ValueError(f"Unexpected archive contents: {archive_path.name}")
        metadata = json.loads(files[f"{name}/BUILD.json"])
        expected = {"version": release_version, "tag": tag, "protocol": 1, "os": system, "arch": arch}
        if any(metadata.get(key) != value for key, value in expected.items()):
            raise ValueError("Archive version/platform metadata mismatch")
        if hashlib.sha256(files[f"{name}/{binary}"]).hexdigest() != metadata["sha256"]:
            raise ValueError("Binary checksum mismatch")
        if not re.fullmatch(r"[a-f0-9]{40}", metadata["revision"]):
            raise ValueError("Missing source revision")
        revisions.add(metadata["revision"])
        checksums.append(f"{digest(archive_path)}  {archive_path.name}\n")
    if len(revisions) != 1:
        raise ValueError("Release archives were built from different commits")
    if revisions != {run(["git", "rev-parse", "HEAD"])}:
        raise ValueError("Release archives do not match the checked-out source revision")
    allowed = {line.split("  ", 1)[1].strip() for line in checksums} | {"SHA256SUMS"}
    if {file.name for file in output.iterdir()} - allowed:
        raise ValueError("Unexpected files in release directory")
    (output / "SHA256SUMS").write_text("".join(checksums), encoding="utf-8")
    print("Verified all six release archives and wrote SHA256SUMS")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=["version", "build", "verify"])
    parser.add_argument("tag")
    parser.add_argument("--os", choices=["linux", "darwin", "windows"])
    parser.add_argument("--arch", choices=["amd64", "arm64"])
    parser.add_argument("--output", type=Path, default=ROOT / "dist/release")
    args = parser.parse_args()
    if args.operation == "version":
        print("version=" + version(args.tag))
    elif args.operation == "build":
        build(args.tag, args.os, args.arch, args.output.resolve())
    else:
        verify_archives(args.tag, args.output.resolve())
