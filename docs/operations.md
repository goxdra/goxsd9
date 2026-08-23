# Scheduled operations

Paseo schedules outside this repository. Jobs start from the clean coordination
checkout in America/New_York. Skills are executable procedures; this page
is the operator contract.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:30 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | Sol, maximum effort | `Run $retro for this repository.` |
Jobs are non-interactive. Develop starts from clean canonical `main` matching
fetched `origin/main` with recursive pins; `doctor` enforces; stale launches run
`base-sync`, then relaunch. Develop claims one Ready issue, uses worktree, opens
draft PR, squash-merges evaluated head; one companion. Managed-document heads:
exact audit + fresh read-only Curator review; repeat after each remediation push;
preserve review records in PR evidence; Examiner gates.
Four-hour claims renew at durable boundaries/pushes, never solely for renewal;
no-PR expirations archive under `agent/archive/`; PR resume:
`go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]`.
Three failed rounds add `needs-human`; bounded transient retries continue
successful runs; return issue to Backlog.
`go tool workflowctl sync` updates Project status/claim refs; it does not sync
canonical `main` or recursive submodules.
Run-local refs affect neither ownership, Project status, nor PR heads; inventory-only.
`base-sync` fetches `origin/main`, fast-forwards `main`, updates recursive pins;
no reset/rebase/stash/discard.
After draft exists, set `PR_NUMBER`:
`PR_NUMBER="$(gh pr view --json number --jq '.number')"`; then set
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"` to
use exact REST base SHA for `develop-signals --base "$BASE_SHA"`,
`docs audit --base "$BASE_SHA"`, and `pr evidence update` JSON; never
`origin/main` or local merge-base.
Non-relevant: `no-relevant-target`/`not-measured` valid. Automatic policy fuzz:
bounded offline single-worker; optional repeatable `--additional-fuzz PACKAGE:TARGET`
is current-head validated; separately requested checked-in corpus replay is distinct.
JSON exact deltas/targets; fuzz health, not conformance. Before
evidence update, challenge, or finish, workflowctl resolves exact REST base/head,
requires matching local commits, recomputes complete v2 signals/policy, compares
canonical payload. Evidence updates preserve all non-owned PR body bytes.
Challenge/finish require REST base/head, audit, Curator/no-doc result;
challenges bind body/evidence digests. Before Examiner, `evaluation challenge`
records one-use head-bound challenge; Examiner JSON is versioned;
`workflowctl` rejects wrong-head, stale, reused, malformed, or caller-selected
results. Fresh context; receipts are evidence, not identity proof.
REST squash merge is SHA-bound. Cleanup scopes inventory by packet, proves branch-bounded run identity, rejects inherited-trailer ownership, and preserves artifacts. Exact deletion, idempotent `pr recover`, merged-proof `claim prune`, archives, and unrelated refs remain.

GraphQL fallback: identical-head REST, fresh Examiner.
GitHub-effective references validate close; exact merge proof binds primary to
trusted receipt before close/reread CLOSED and Project Done; `pr recover` retries; `sync` maps CLOSED to Done.

Issue, claim, checks, challenge, attestation, receipt, merge record
communication. Finally, pass a plain-text summary outside the repository
to `workflowctl pr finish PR --summary-file FILE`; it covers problem, outcome,
rationale, invariants, process learning; omit status, commands, claim/review
metadata. It validates squash body; history reads it.
Use Markdown evidence body files.

Summary is non-empty UTF-8 text in a file of at most 8 KiB, LF-only; one final LF
is accepted. Surrounding/line-trailing whitespace, control, format, other
line-separator characters, and generated claim trailers are rejected.
