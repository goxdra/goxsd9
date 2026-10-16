# Workflow capture contract

Recovery orchestration needs a stable representation of completed workflow
commands and their later evidence reads. Capture is read-only and cannot replay
a command whose publication or challenge may already have succeeded. Normal
workflow permission, claim, exact-head review, and merge gates remain in their
controllers.

`go tool workflowctl capture pr PR [--expected-head SHA]
[--command-result-file FILE]` emits one `goxsd9/workflow-capture/v1` JSON
object. Optional input is a previously recorded `goxsd9/command-result/v1`
object with nonempty `operation`, base64 `stdout` and `stderr`, and numeric
`exitStatus`. Empty streams use empty base64 strings. The command record is
copied unchanged in meaning; capture never executes it. If a later read fails,
`command` remains in JSON, `captureError` gives the cause, `pr` is absent,
and capture exits nonzero. Status 0 in `command` refers only to the previous
command.

`pr` contains PR number/URL, exact REST `base`/`head` SHAs and ref names,
base64 body bytes, `bodySHA256`, and ordered comments. Each comment has
numeric ID, author login, timestamp, and base64 body bytes. The controller
reads all supported concatenated JSON comment pages, rejects malformed
trailing data, and rereads the PR and comments to reject source drift.

When evidence markers exist, `pr.evidence` is the raw unfenced parsed
`goxsd9/pr-evidence/v1` object. `pr.evidenceBlock` is base64 for the exact
bytes from start through end marker, and `pr.evidenceSHA256` hashes those
marker-inclusive bytes. `bodySHA256` hashes the complete PR body.
`pr.receipts` is an ordered array of authenticated logical receipts. Each
preserves its comment ID, typed receipt fields, full base64 comment body,
and decoded attestation bytes. `receiptMarker` is base64 for the raw JSON
payload inside the receipt HTML marker, excluding its delimiters. No terminal
newline is added.
Absent evidence omits its fields; absent receipts use an empty array.

`go tool workflowctl capture artifact NAME --file FILE --base SHA --head SHA`
validates raw JSON through the controller's shared types and exact identities.
Names: `development-signals`, `documentation-audit`, `pr-evidence`, and
`curator-result`. Curator capture also requires `--audit-file FILE` to
validate the decision against the matching audit. Development signals and PR
evidence use `base` and `head`; documentation audit adds `mergeBase`.
Curator result has `head` only, while the command's `--base` binds its audit.
Callers must not substitute `baseSHA` or `sourceSHA`. Missing required
arrays and unknown or trailing fields fail validation.

`go tool workflowctl capture git-object --commit SHA --path PATH` resolves one
tracked tree entry at that exact commit. A mode `160000` entry yields
`kind: "gitlink"`, its mode, and pinned object ID without opening the
submodule. Regular files and tracked symlinks yield `kind: "blob"` and exact
base64 `bytes`. Blob size is checked before the read and limited to 8 MiB.
Capture does not enumerate Project items, run builds, or recompute signals,
coverage, or conformance.
