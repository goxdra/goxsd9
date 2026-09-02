# Scheduled operations

Paseo schedules externally. Jobs start from clean coordination checkout in America/New_York.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:30 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | Sol, maximum effort | `Run $retro for this repository.` |
Jobs are non-interactive. Develop starts from clean canonical `main` matching
fetched `origin/main` with recursive pins; `doctor` enforces; stale launches
`base-sync`, then relaunch. Develop claims one Ready issue, uses worktree, opens
draft PR, squash-merges evaluated head; one companion. Managed-document heads:
exact audit + fresh read-only Curator review; repeat after each remediation push;
preserve review records in PR evidence; Examiner gates.
Four-hour claims renew at durable boundaries/pushes, never solely for renewal;
no-PR terminal handoffs preserve their claim worktree; other expired no-PR claims
archive; acknowledged claim resume:
`go tool workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]`.
PR resume: `go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]`.
Transient agent, checkout, transport, and challenge failures remain retryable;
exactly three authenticated Examiner `fail` receipts add `needs-human`, then
return the issue to Backlog. Claim resume requires the trusted exact paginated
handoff comment, expired exact claim, expected fixed-branch SHA, no open PR,
canonical Backlog, and one clean unlocked same-run worktree; it removes
`needs-human` and restores Picked only after renewal verification.
`go tool workflowctl sync` updates Project status/claim refs; canonical `main` and
recursive submodules are not synced.
Run-local refs affect neither ownership, Project status, nor PR heads; inventory-only.
`base-sync` fetches `origin/main`, fast-forwards `main`, updates recursive pins;
no reset/rebase/stash/discard.
After draft, set `PR_NUMBER`:
`PR_NUMBER="$(gh pr view --json number --jq '.number')"`; set
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"` to
use exact REST base SHA for `develop-signals --base "$BASE_SHA"`,
`docs audit --base "$BASE_SHA"`, and `pr evidence update` JSON; never
`origin/main` or local merge-base.
Signals may be `no-relevant-target`/`not-measured`; bounded single-worker policy
fuzz and optional `--additional-fuzz` are health, not conformance; corpus replay
is separate. Before evidence/challenge/finish, resolve exact REST base/head,
match commits, recompute v2 signals, and compare canonical JSON. Evidence
updates rewrite owned slots and preserve non-owned PR bytes.
Challenge/finish require REST base/head, audit, and Curator/no-doc result;
challenges bind body/evidence digests. Before Examiner, record a one-use
head-bound challenge; reject wrong-head, stale, reused, malformed, or
caller-selected results. Fresh context; receipts are evidence, not identity.
Unresolved challenges survive; only complete-equivalent trusted receipts form
rounds. Resolution grants no verdict; an authenticated passing receipt remains
merge proof. Cleanup inventories claims, preserves unrelated artifacts, and is
exact/idempotent. REST fallback uses identical head and fresh Examiner.
Close only after GitHub-effective refs and exact merge proof bind a trusted
receipt; recover retries; sync maps CLOSED to Done.
Communications cover issue/claim/check/challenge/attestation/receipt/merge
records. After draft, pass an external plain-text problem/outcome/rationale/
invariants summary to `workflowctl pr finish PR --summary-file FILE`; omit
metadata/status/commands/claim/review. Use Markdown evidence body files.
Summary: non-empty UTF-8 text; max 8 KiB, LF-only; one final LF accepted.
Reject surrounding/line-trailing whitespace, controls, Unicode format characters, other line-separators, generated claim-trailers.
