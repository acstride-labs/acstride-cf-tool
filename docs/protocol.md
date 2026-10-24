# Protocol v1

One CLI invocation is one cancellable operation. No HTTP server or daemon runs.
stdout is UTF-8 JSON/NDJSON; stderr is diagnostics only. No ANSI escapes, prompts,
passwords, cookies or raw server error pages are printed. `problem page` is an
explicit exception for raw HTML as a successful data payload, not diagnostics.

```json
{"protocol":1,"requestId":"editor-42","type":"result","ok":true,"data":{}}
```

```json
{"protocol":1,"requestId":"editor-42","type":"result","ok":false,"error":{"code":"verification_required","message":"...","retryable":false}}
```

`--stream` adds `type: progress` records for batch downloads, followed by exactly
one terminal result. Partial failure includes successful items and per-item
errors in `data` and exits nonzero. Consumers must verify protocol, requestId,
type, exit code and operation-specific schemas, never match English messages.

Exit codes: 0 success; 1 ordinary/partial failure; 2 arguments/confirmation;
3 authentication; 4 verification/access refusal; 5 unknown/pending submission;
130 cancellation. A failed POST returns 5 with an unknown receipt, even if the
underlying cause was timeout/cancellation, to avoid unsafe retries.

Stable codes include `invalid_argument`, `source_changed`, `invalid_language`,
`account_mismatch`, `not_authenticated`, `login_failed`, `session_unavailable`,
`secret_store_unavailable`, `verification_required`, `access_denied`, `not_found`,
`problem_not_ready`, `parse_error`, `invalid_response`, `api_rejected`,
`rate_limited`, `timeout`, `cancelled`, `external_redirect`,
`unsafe_redirect`, `unexpected_redirect`, `partial_failure`, `helper_failed`,
`submission_pending`, `submission_unknown`, `invalid_receipt`, `busy` and
`request_failed`. Localize codes in the host's JSON catalogs; raw messages are
English diagnostics and must not become UI state identifiers.

## Editor API bridge

Only `api contest.list`, `api contest.status --contest ID --handle HANDLE`, and
`api contest.standings --contest ID` are exposed. They return the API result,
not its outer Codeforces envelope. The standings bridge accepts `--all-rows`
for the existing editor's nearby/search filtering, without a fixed response-byte
limit. Otherwise use paging/count/handle filters. Arbitrary
API methods, alternate hosts and credential-bearing URLs are not accepted.

`capabilities` is offline and reports protocol/version/features plus
`automaticSubmissionVerified: false`. `doctor` is offline and does not probe or
prompt for keyring/network access implicitly. Executable absence/incompatibility
is a host packaging error, not a request to install Go or modify PATH.

## Transient sessions

Advanced trusted editors can keep sessions in their own SecretStorage and send
this object on stdin with `--session-stdin --experimental`:

```json
{"protocol":1,"handle":"your-handle","expiresAt":1790000000,"cookies":[{"Name":"JSESSIONID","Value":"REDACTED","Domain":"codeforces.com","Path":"/","Secure":true}]}
```

Use a future expiry at most 24 hours from creation (maximum accepted horizon is
25h for clock skew); the literal timestamp above is illustrative, not valid
credentials. Input is bounded to 64 KB. Do not print or commit it. Cookies are
scoped only to the fixed HTTPS origin. Anonymous operations do not load keyring
sessions or accept cookies. `session login` instead requires credentials JSON
on stdin and stores its resulting session in the OS keyring; it never exports
cookies in its output. Verification and 2FA are not automated.

## Submission state

Caller saves/captures source -> reviews target/language/hash -> confirms ->
CLI validates -> acquires target lock -> checks existing unknown receipts ->
reads authenticated form and baseline -> writes snapshot + unknown receipt ->
sends exactly one POST -> caller reconciles. No request retries or automatic
login-and-resubmit path exists. A crash after receipt creation safely leaves an
unknown receipt, even if no POST actually reached the site.

Reconciliation is read-only on Codeforces (GETs/public API), but updates the
local receipt after exact identity/source verification. `--submission-id` can
disambiguate identical source submissions; it does not bypass verification.
`receipt abandon --confirm` is the only explicit manual release, with a duplicate
submission risk that the host must communicate. Receipt snapshots are local
private files, not cloud uploads or billing artifacts.

## Structured editor problem read (client 0.3+)

`problem read ID INDEX --assets-dir DIR` returns the `Problem` fields directly
plus RFC3339 `fetchedAt`, integer `imageFailures`, and an `assets` array of local
content-addressed filenames. This is a fresh read, separate from `problem fetch`
JSON caches. Omit the directory for no image downloads. Image count and response bytes are not capped; duplicate URLs reuse a successful download. The explicit
asset root is opened with os.Root and writes use atomic replacement, rejecting
symlinks. Partial image failures preserve the statement and samples; failed image
URLs may remain HTTPS references and offline clients should disable them.

Successful image src values use `acstride-asset:SHA256.png` (jpg/gif/webp also
supported). Editors validate the filename against `assets` and map it to local
Webview resources; they must not interpret arbitrary filesystem paths from HTML.
The CLI owns Codeforces DOM extraction and all network reads. A host sanitizer
and CSP remain necessary because process output and cached HTML are untrusted.

`--bundled` resolves the helper under the installation's sibling downloader
folder and fails explicitly if absent. `--public-helper` is an advanced explicit
absolute-path override. The editor uses only `--bundled`. Helper sources and
pinned Python dependencies live in this repository's `packaging/` directory.

Production CLI operations share request-start pacing across processes using an
OS lock and last-start timestamp in the user cache directory. Offline commands
do not create pacing files. A killed process releases its OS lock; timestamps
cannot reserve an unbounded future delay. Each request still honors cancellation.
