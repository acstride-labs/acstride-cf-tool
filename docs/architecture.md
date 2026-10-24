# Architecture and acceptance boundaries

```text
VS Code UI / terminal caller
        |
  protocol v1 CLI (JSON, flags, signal/deadline, exit code)
        |
  internal/core (no stdout, no prompts, no global account)
     |-- public API reads
     |-- anonymous page -> DOM parser -> safe statement/samples -> rooted cache
     |-- optional anonymous helper adapter (existing bundled curl_cffi)
     |-- opt-in session -> OS keyring / caller-owned transient stdin
     `-- opt-in submission -> target lock -> receipt/snapshot -> single POST
                                             `-> exact-source reconciliation
```

The existing root `client`, `cmd`, `config`, `cookiejar`, `util` packages retain
limited upstream CLI compatibility. Modern commands do not initialize their
globals or read legacy config/session. No new code should depend on those
packages. Password login/submission/self-update in the legacy path are disabled.

Testing includes HTML fixtures, current and older sample markup, malicious HTML,
missing/mismatched samples, authenticated identity, language binding, fixed-origin
redirects, response limits, cancellation/timeouts, anonymous cache reuse, rooted
symlink protection, partial downloads, session validation, durable unknown
receipts, duplicate blocking and exact-source reconciliation. Tests use fake
HTTP servers and fake secret stores, never a real password/submission.

Live public reads must be opt-in and reported separately from fixture coverage.
A successful anonymous download proves that request/environment only. An
experimental login adapter existing in code does not establish that current
Codeforces login, CAPTCHA, 2FA or all contest forms work. Production automatic
submission is not an advertised capability until account-owner testing proves
login, restart persistence, exact receipt identity and interruption behavior.

The VS Code extension continues to own native editors, templates, local runner,
multi-contest scheduling, localization and statement asset caching. Website
accounts/payments/AI are outside this library. No listening ports, browser
automation, browser extension, telemetry or cloud service is introduced.

Vendored upstream dependencies and dep manifests were replaced with versioned
Go modules/checksums. Their previous versions remain in Git history. Generated
license notices accompany distributed binaries. The fork retains MIT; no blanket
noncommercial restriction is applied to upstream MIT code.
