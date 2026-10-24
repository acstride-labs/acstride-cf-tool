# Changelog

## 0.5.0

- Add persistent anonymous NDJSON reads, a lightweight contest problem list, and ordered first-problem-then-two-worker downloads.
- Publish text/samples before background image completion. Reuse HTTP connections and a two-process curl transport pool; retain existing pacing and account/submission separation.

## 0.2.0

- Add a standalone `codeforces-client` CLI with protocol v1 JSON/NDJSON, request IDs,
  stable error codes, non-interactive parameters and signal/deadline cancellation.
- Add anonymous contest metadata, sanitized statement/sample parsing, rooted
  per-contest/per-problem caches, incremental batch reads and partial failures.
- Add public submissions, explicit-ID verdict watching and compatible rankings
  queries with local paging/handle filtering.
- Add a narrow public API bridge for the VS Code extension and an optional
  adapter to its existing bundled anonymous curl_cffi downloader.
- Add opt-in experimental login/session/language-form operations. Use OS keyring
  storage or caller-owned transient sessions; never persist passwords or fall
  back to plaintext cookies.
- Add source-hash/account/language binding, durable unknown receipts before
  submission, cross-process target locks, exact-source reconciliation and an
  explicit manual-abandon workflow. Never retry a submission POST.
- Retire unsafe legacy login/submission/self-update, limit old local-runner
  time/output and replace its busy memory loop with paced sampling.
- Migrate old dep/vendor to locked Go Modules, preserving history and MIT;
  generate notices for binary distribution and add cross-platform CI/artifacts.
- Add fixture, network-failure, cache, protocol, privacy and receipt tests.

Live acceptance: anonymous contest 1 metadata, A statement/sample, public tourist
submission history and rankings passed on 2026-10-24 in the development network.
Authenticated login, keyring restart persistence against a real account and
actual submissions have **not** been verified. No production auto-submit claim.
