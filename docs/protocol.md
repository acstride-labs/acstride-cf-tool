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
`rate_limited`, `timeout`, `cancelled`, `response_too_large`, `external_redirect`,
`unsafe_redirect`, `unexpected_redirect`, `partial_failure`, `helper_failed`,
`submission_pending`, `submission_unknown`, `invalid_receipt`, `busy` and
`request_failed`. Localize codes in the host's JSON catalogs; raw messages are
English diagnostics and must not become UI state identifiers.

## Editor API bridge

Only `api contest.list`, `api contest.status --contest ID --handle HANDLE`, and
`api contest.standings --contest ID` are exposed. They return the API result,
not its outer Codeforces envelope. The standings bridge accepts `--all-rows`
for the existing editor's nearby/search filtering, still subject to the 8 MB
network response limit. Otherwise use paging/count/handle filters. Arbitrary
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
