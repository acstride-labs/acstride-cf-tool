"""Offline release packaging and publication regression checks."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import release

spec = importlib.util.spec_from_file_location("upload_release", Path(__file__).with_name("upload-release.py"))
uploader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(uploader)


class ReleaseTests(unittest.TestCase):
    def test_semver(self):
        for tag in ["v0.5.0", "v0.5.0-alpha.1", "v1.0.0-beta.2", "v1.0.0-rc.1+build.42"]:
            self.assertEqual(release.version(tag), tag[1:])
        for tag in ["main", "1.0.0", "v01.0.0", "v1.0", "v1.0.0-alpha.01", "v1.0.0\n", "v1.0.0;echo bad"]:
            with self.assertRaises(ValueError):
                release.version(tag)

    def archives(self, folder):
        tag = "v0.5.0-alpha.1"
        revision = release.run(["git", "rev-parse", "HEAD"])
        for system, arch in release.TARGETS:
            name = f"codeforces-client_0.5.0-alpha.1_{system}_{arch}"
            with tempfile.TemporaryDirectory() as temporary:
                stage = Path(temporary) / name
                stage.mkdir()
                binary = stage / ("codeforces-client.exe" if system == "windows" else "codeforces-client")
                binary.write_bytes(b"test binary")
                binary.chmod(0o755)
                for file in ["LICENSE", "README.md", "THIRD_PARTY_NOTICES.txt"]:
                    (stage / file).write_text("test notice", encoding="utf-8")
                (stage / "BUILD.json").write_text(json.dumps({"version": tag[1:], "tag": tag, "revision": revision,
                    "protocol": 1, "os": system, "arch": arch, "sha256": release.digest(binary)}), encoding="utf-8")
                release.pack(stage, folder / (name + (".zip" if system == "windows" else ".tar.gz")), system)
        return tag

    def test_archives_ignore_file_modification_times(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            folder = root / "package"
            folder.mkdir()
            file = folder / "codeforces-client"
            file.write_bytes(b"same program")
            file.chmod(0o755)
            for system, extension in [("darwin", ".tar.gz"), ("windows", ".zip")]:
                first, second = root / ("first" + extension), root / ("second" + extension)
                release.pack(folder, first, system)
                import os
                os.utime(file, (2_000_000_000, 2_000_000_000))
                release.pack(folder, second, system)
                self.assertEqual(first.read_bytes(), second.read_bytes())

    def test_complete_archives_and_checksums(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            release.verify_archives(tag, folder)
            for line in (folder / "SHA256SUMS").read_text().splitlines():
                checksum, name = line.split("  ")
                self.assertEqual(checksum, release.digest(folder / name))
            self.assertEqual(len((folder / "SHA256SUMS").read_text().splitlines()), 12)
            (folder / "unrelated.txt").write_text("must not publish")
            with self.assertRaisesRegex(ValueError, "Unexpected files"):
                release.verify_archives(tag, folder)

    def test_standalone_binary_must_match_archive(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            release.verify_archives(tag, folder)
            for system, arch in release.TARGETS:
                direct = folder / (f"codeforces-client_{tag[1:]}_{system}_{arch}" + (".exe" if system == "windows" else ""))
                self.assertEqual(direct.read_bytes(), b"test binary")
            direct.write_bytes(b"changed binary")
            with self.assertRaisesRegex(ValueError, "differs from archive"):
                release.verify_archives(tag, folder)

    def test_missing_platform_blocks_release(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            next(folder.glob("*.zip")).unlink()
            with self.assertRaises(FileNotFoundError):
                release.verify_archives(tag, folder)

    def test_upload_retries_skip_identical_assets_and_reject_changes(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            release.verify_archives(tag, folder)
            metadata = {"tag_name": tag, "prerelease": True, "immutable": True,
                "assets": [{"name": file.name, "digest": "sha256:" + release.digest(file)} for file in folder.iterdir()]}
            with patch.dict("os.environ", {"GITHUB_REPOSITORY": "acstride-labs/codeforces-client"}):
                with patch.object(uploader, "gh", side_effect=[json.dumps(metadata), json.dumps({"sha": release.run(["git", "rev-parse", "HEAD"])})]) as gh:
                    uploader.upload(tag, folder)
                    self.assertEqual(gh.call_count, 2)
                metadata["assets"][0]["digest"] = "sha256:wrong"
                with patch.object(uploader, "gh", side_effect=[json.dumps(metadata), json.dumps({"sha": release.run(["git", "rev-parse", "HEAD"])})]):
                    with self.assertRaisesRegex(ValueError, "never overwritten"):
                        uploader.upload(tag, folder)

    def test_immutable_release_requires_draft_upload_first(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            metadata = {"tag_name": tag, "prerelease": True, "immutable": True, "assets": []}
            with patch.dict("os.environ", {"GITHUB_REPOSITORY": "acstride-labs/codeforces-client"}), patch.object(uploader, "gh", side_effect=[json.dumps(metadata), json.dumps({"sha": release.run(["git", "rev-parse", "HEAD"])})]):
                with self.assertRaisesRegex(ValueError, "draft Release"):
                    uploader.upload(tag, folder)

    def test_uploads_all_assets_to_existing_release(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            metadata = {"tag_name": tag, "prerelease": True, "immutable": False, "assets": []}
            responses = [json.dumps(metadata), json.dumps({"sha": release.run(["git", "rev-parse", "HEAD"])}), ""]
            with patch.dict("os.environ", {"GITHUB_REPOSITORY": "acstride-labs/codeforces-client"}), patch.object(uploader, "gh", side_effect=responses) as gh:
                uploader.upload(tag, folder)
                args = gh.call_args.args
                self.assertEqual(args[:3], ("release", "upload", tag))
                self.assertEqual(len(args[3:-2]), 13)
                self.assertNotIn("--clobber", args)

    def test_tag_move_blocks_upload(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            responses = [json.dumps({"tag_name": tag, "prerelease": True, "assets": []}), json.dumps({"sha": "0" * 40})]
            with patch.dict("os.environ", {"GITHUB_REPOSITORY": "acstride-labs/codeforces-client"}), patch.object(uploader, "gh", side_effect=responses):
                with self.assertRaisesRegex(ValueError, "tag moved"):
                    uploader.upload(tag, folder)

    def test_prerelease_channel_is_not_changed_implicitly(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            tag = self.archives(folder)
            with patch.dict("os.environ", {"GITHUB_REPOSITORY": "acstride-labs/codeforces-client"}), patch.object(uploader, "gh", return_value=json.dumps({"tag_name": tag, "prerelease": False, "assets": []})):
                with self.assertRaisesRegex(ValueError, "prerelease flag"):
                    uploader.upload(tag, folder)


if __name__ == "__main__":
    unittest.main()
