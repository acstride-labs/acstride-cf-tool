# Releasing the standalone CLI

The `Release binaries` workflow in `.github/workflows/release.yml` builds the
modern `codeforces-client` CLI. The existing push/PR CI remains separate.

## Publish a version

1. Merge the release changes, including this workflow, into the default branch.
2. Create a tag on that exact commit, such as `v0.5.0-alpha.1` or `v0.5.0`.
   Tags must use SemVer with a `v` prefix. Existing published tags must never move.
3. On GitHub, create a Release for that tag. Mark Alpha/Beta/RC versions as
   **pre-release**; leave that flag off for a version without a prerelease suffix.
4. Publish the Release. Both normal releases and pre-releases trigger the workflow.
5. Wait for all tests, builds, archive validation and the final upload to succeed.
   Download the files from the Release's **Assets** section. Assets will not be
   available immediately after clicking Publish; inspect the Actions run if absent.

The workflow checks out the tag once, pins its full commit SHA across jobs, runs
Python checks and Go vet/race tests on Linux, macOS and Windows, then cross-compiles
six targets. A Linux amd64 release executable is smoke-tested. Cross-compilation
is not proof that all six architectures have been exercised on physical hardware.

No extra token is required: only the final upload job receives the repository's
`GITHUB_TOKEN` with `contents: write`. Tests and builds have read-only permissions.

Release creation performed by another workflow using `GITHUB_TOKEN` may not trigger
this workflow. Use the manual entry point below in that situation.

## Files and version identity

For each of Linux, macOS (`darwin`) and Windows, both `amd64` and `arm64` are built:

```text
codeforces-client_0.5.0-alpha.1_linux_amd64.tar.gz
codeforces-client_0.5.0-alpha.1_linux_arm64.tar.gz
codeforces-client_0.5.0-alpha.1_darwin_amd64.tar.gz
codeforces-client_0.5.0-alpha.1_darwin_arm64.tar.gz
codeforces-client_0.5.0-alpha.1_windows_amd64.zip
codeforces-client_0.5.0-alpha.1_windows_arm64.zip
codeforces-client_0.5.0-alpha.1_linux_amd64
codeforces-client_0.5.0-alpha.1_linux_arm64
codeforces-client_0.5.0-alpha.1_darwin_amd64
codeforces-client_0.5.0-alpha.1_darwin_arm64
codeforces-client_0.5.0-alpha.1_windows_amd64.exe
codeforces-client_0.5.0-alpha.1_windows_arm64.exe
SHA256SUMS
```

Each archive contains a platform/version-named folder with the executable,
`LICENSE`, `THIRD_PARTY_NOTICES.txt`, `README.md` and `BUILD.json`. The executable's
`--version` / `capabilities` output is injected from the tag, without its `v` prefix.
`BUILD.json` records that same version, tag, source SHA, protocol, platform, Go
compiler and binary checksum. The software release version does not change
`protocol: 1`. The source's default `Version` remains a development-build fallback;
it is not the authority for a tagged release.

Standalone executables are extracted from the verified archives, never rebuilt separately.
Rename the selected download to `codeforces-client` (`codeforces-client.exe` on Windows).
On macOS/Linux, run `chmod +x ./codeforces-client` after downloading. Choose the full
archive for license notices and build metadata. SHA256SUMS covers all 12 downloads.

Verify downloaded files using `sha256sum -c SHA256SUMS` on Linux or
`shasum -a 256 -c SHA256SUMS` on macOS (download all 12 files for the full check).
On PowerShell, compare `Get-FileHash <archive> -Algorithm SHA256` with its entry.

These are **standalone Go CLI archives**. They do not include the optional frozen
Python/curl transport used by the VS Code plugin. That transport remains bundled
with the platform-specific VSIX. See [transport packaging](../packaging/README.md).

## Build first, publish later

Actions → **Release binaries** → **Run workflow**:

- Set `tag` to an existing version tag whose commit contains the release scripts.
- Leave `upload_assets` off to test the entire build without changing any Release.
  Retrieve the `verified-release-assets` Actions artifact for inspection.
- Turn `upload_assets` on to attach files to an **existing** draft or published
  Release with that tag. This never creates a tag or Release and never changes its
  description, title or pre-release flag.

For repositories with **immutable releases**, create the tag and a draft Release,
run this workflow manually with upload enabled, inspect the attached files, then
publish the draft. GitHub prevents adding files after an immutable Release is
published. The subsequent published-event run may skip identical existing assets,
but will not replace them.

## Failure and retries

Uploads start only after every platform has built and the complete archive set
has passed verification. Identical existing assets are skipped after checking
GitHub's SHA256 digest, or downloading and hashing the existing file when needed.
A different file with the same name causes an error; no `--clobber` is used.

If upload is interrupted, use **Re-run failed jobs** so it reuses the already
verified artifacts. Archive timestamps and ownership are normalized for repeatable packaging, but
a different Go toolchain or build environment can still change the bytes. Rebuilding
all archives can therefore produce different files;
the workflow deliberately refuses to replace assets on that basis. Publish a new
version for changed code or artifacts. If a Release was already immutable before
its files were uploaded, create a new version through the draft-first process.

Local validation:

```sh
python packaging/test-release.py
python packaging/release.py version v0.5.0-alpha.1
python packaging/release.py build v0.5.0-alpha.1 --os linux --arch amd64
# After building all six targets:
python packaging/release.py verify v0.5.0-alpha.1
```

Python 3.11+ and the Go version required by `go.mod` are build dependencies only.
The local build supports an explicit Go executable through the `GO` environment variable.
