# ACStride CF Tool

A non-interactive Codeforces CLI and editor integration core, derived from
[xalanq/cf-tool](https://github.com/xalanq/cf-tool). MIT licensed; the original
copyright and license are retained. This is not an official Codeforces client.

[简体中文](README.zh-CN.md) · [Protocol](docs/protocol.md) · [Architecture](docs/architecture.md)

## Status

Version 0.2 introduces protocol v1, anonymous reads, isolated contest caches,
bounded HTTP requests, OS-keyring sessions and durable experimental submission
receipts. Login/submission are **experimental, opt-in and not verified against a
real account**. Site challenges are detected, never solved or bypassed. A 200
response is not proof of successful submission. Keep the official site available.

## Build

Developers need Go 1.26+. End users need only the platform binary; no Go, Python,
pip, system installation, PATH changes, listening port or background daemon is required.

```sh
go mod download all
go mod verify
go test -race ./...
go vet ./...
go build -trimpath -o dist/acstride-cf ./cmd/acstride-cf
go run ./tools/notices -output dist/THIRD_PARTY_NOTICES.txt
```

Distribute `LICENSE` and generated notices with the binary. CI checks Linux,
macOS and Windows and builds amd64/arm64 artifacts. Cross-compilation alone is
not execution testing of every target. Self-updating is intentionally disabled;
editor-bundled binaries must update with their host extension.

## Anonymous commands

```sh
acstride-cf capabilities --json
acstride-cf doctor --json
acstride-cf contest info 1 --json
acstride-cf problem fetch 1 A --output ./practice --json
acstride-cf contest download 1 --output ./practice --stream --json
acstride-cf submissions list --contest 1 --handle tourist --count 20 --json
acstride-cf standings list 1 --count 100 --json
acstride-cf standings list 1 --handle tourist --json
acstride-cf submission watch --contest 1 --handle tourist --submission-id 123 --json
```

All output is JSON, including errors, even without `--json`. Flags may follow
positionals. Commands never prompt. `--request-id` correlates editor requests.
`--timeout` bounds the total operation (default 120s, max 10m); individual HTTP
requests have a 25s deadline, 8 MB limit, same-origin redirect checks and at least
2.1s between request starts within one client. Editors must serialize separate
CLI processes too. Read retries use backoff only in batch problem downloading;
submission POSTs are never retried. SIGINT/SIGTERM cancel work.

Only public regular contests are supported by the new protocol. Gym, groups,
registration, virtual participation and clarifications are not implemented.
Standings/submissions can be frozen, delayed or unavailable. Recent submission
lookups are bounded to 100 records. Watch tracks an explicit ID, never 'latest'.

### Downloads and cache

`problem fetch` returns sanitized statement HTML/text, limits and samples. It
does not require login. HTML is untrusted data; clients must retain their own
sanitization/CSP. JavaScript, SVG, event handlers and non-Codeforces image/link
URLs are removed. Mathematical notation is preserved as source HTML/text, not
rendered by the CLI. Images remain official HTTPS references, not offline files.
`problem page` returns **raw untrusted HTML** for existing editor parsers.

Cache location:

```text
practice/contests/1/.acstride/problems/A.json
practice/contests/2/.acstride/problems/A.json
```

Valid caches expire after 24 hours. `--refresh` forces a new read. Writes are
atomic and constrained to the chosen root; refresh failure retains the old
cache, but returns the failure rather than pretending the cache is fresh.
Only these cache JSON files are written. No solution or custom-test file is
generated, overwritten or removed. Batch imports retain successes, report each
failure, and return a nonzero exit code on partial failure. Rerunning reuses
successful caches. There is no background contest scheduler; the VS Code
extension owns countdowns, retries across restarts and workspace organization.

`--public-helper /absolute/path/to/cfide-downloader` optionally reuses the
existing editor-bundled curl_cffi reader for **anonymous problem GETs only**.
It never receives passwords or cookies. It is not downloaded or installed at
runtime, is not required for the standard Go transport, and does not guarantee
access through site verification. No browser integration is introduced.

## Experimental sessions and submissions

No passwords are accepted as command-line flags or persisted. `session login
--experimental --credentials-stdin --store` reads a small JSON object with
`username` and `password` from stdin and stores only the resulting session in
the OS keyring. Feed stdin using a secure caller/prompt, not a literal password
in shell history. macOS Keychain, Windows Credential Manager or Linux Secret
Service must be available. No plaintext fallback is used. The CLI never exports
stored cookies in output. Persisted sessions have a maximum local lifetime of
24 hours; they may expire on the server sooner.

An editor may instead pass its own transient protocol-v1 session through
`--session-stdin`; see [protocol](docs/protocol.md). This is an advanced trusted
caller interface, not a recommendation to copy browser cookies.

```sh
acstride-cf session check --experimental
acstride-cf languages list --contest 1 --experimental
acstride-cf session forget
```

The official language list is read from the submission form, not an obsolete
hardcoded C++/Python table. Templates no longer select submission languages.

To submit, save a source file, review its bytes and SHA256, and explicitly pass
`--contest`, `--problem`, `--file`, `--language-id`, `--expected-sha256`,
`--experimental` and `--confirm`. A source change aborts before authentication.
The form's account and offered language must match the request. A private
snapshot and an `unknown` receipt are written **before the one submission POST**.

Every attempted send returns exit 5 with a receipt until it is independently
confirmed. Inspect/reconcile it:

```sh
acstride-cf receipt show RECEIPT_ID
acstride-cf receipt reconcile RECEIPT_ID --experimental
acstride-cf receipt reconcile RECEIPT_ID --submission-id 123 --experimental
```

Reconciliation requires the authenticated account, a new matching public
record (account, contest, problem, language, time window) and **exact source
hash equality** from the owner's submission page. Multiple identical matches
stay unknown unless an explicit ID is verified. A wrong/newest record is never
accepted just because its time is close. Website HTML changes, newline
normalization, inaccessible source, API outages or >100 newer records may
prevent automatic reconciliation; check the official website.

Unknown receipts block another send for that account/contest/problem across
restarts. OS file locks serialize concurrent send/reconcile operations and
release on process exit. `receipt abandon ID --confirm` explicitly acknowledges
an unresolved attempt and permits a future send; it does **not** prove the
original failed and may cause duplicates if used carelessly. Confirm on the
official website first. Receipts/snapshots stay in the user's config directory
(`--state-dir` overrides it); no cleanup runs automatically. Local source
snapshots use private permissions but are not encrypted by this application.

`session forget` removes the local keyring entry only; it does not revoke a
Codeforces server session. Old `~/.cf/session` is not imported by modern commands.

## Legacy compatibility

`go build .` still builds the historical `cf` entry point; new command names
route to protocol v1. Legacy template/local commands remain for compatibility,
not for plugin integration. Legacy password login, unsafe submission and
self-update are retired. Legacy sessions no longer serialize passwords/cookies.
The old local runner has bounded time/output and paced memory polling, but is
not a security sandbox and should not run untrusted programs. The maintained
VS Code runner remains the primary local testing implementation.

Historical upstream instructions are preserved under `docs/upstream-*`; their
login/update promises are not current ACStride instructions. No proprietary
subscription or AI layer has been added to this MIT fork.
