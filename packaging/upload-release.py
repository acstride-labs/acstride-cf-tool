"""Attach verified files to an existing Release; retries never overwrite assets."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
from urllib.parse import quote
from release import digest, version, verify_archives, run


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def upload(tag, folder):
    version(tag)
    verify_archives(tag, folder)
    repository = os.environ["GITHUB_REPOSITORY"]
    release = json.loads(gh("api", f"repos/{repository}/releases/tags/{quote(tag, safe='')}"))
    if release["tag_name"] != tag:
        raise ValueError("Release tag mismatch")
    expected_prerelease = "-" in version(tag).split("+", 1)[0]
    if release["prerelease"] != expected_prerelease:
        raise ValueError("Set the Release prerelease flag to match its SemVer tag before uploading")
    commit = json.loads(gh("api", f"repos/{repository}/commits/{quote(tag, safe='')}"))["sha"]
    if commit != run(["git", "rev-parse", "HEAD"]):
        raise ValueError("Release tag moved after the build; refusing to attach assets to a different commit")
    existing = {item["name"]: item for item in release["assets"]}
    pending = []
    for file in sorted(folder.iterdir()):
        if not file.is_file():
            raise ValueError("Unexpected directory in release assets")
        asset = existing.get(file.name)
        if asset:
            expected = "sha256:" + digest(file)
            actual = asset.get("digest")
            if not actual:
                with tempfile.TemporaryDirectory(prefix="acstride-asset-") as temporary:
                    gh("release", "download", tag, "--repo", repository, "--pattern", file.name, "--dir", temporary)
                    actual = "sha256:" + digest(Path(temporary) / file.name)
            if actual != expected:
                raise ValueError(f"Existing asset differs: {file.name}. Published files are never overwritten.")
            print(f"Already uploaded and verified: {file.name}")
        else:
            pending.append(str(file))
    if pending:
        if release.get("immutable"):
            raise ValueError("Release is immutable. Upload assets to a draft Release using the manual workflow before publishing.")
        gh("release", "upload", tag, *pending, "--repo", repository)
    print(f"Release assets ready: https://github.com/{repository}/releases/tag/{tag}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("--directory", type=Path, default=Path("dist/release"))
    args = parser.parse_args()
    upload(args.tag, args.directory)
