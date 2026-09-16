# Scheduled operations

Paseo schedules jobs from a clean coordination checkout in America/New_York.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:30 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | Sol, maximum effort | `Run $retro for this repository.` |
Jobs are non-interactive. Develop requires clean canonical `main` matching fetched
`origin/main` and recursive pins; `doctor` enforces this and stale jobs run
`base-sync` before relaunch. It claims one Ready issue, uses a worktree, opens a
draft PR, and squash-merges the evaluated head. Managed-document/source-trigger
heads require exact audit and fresh read-only passing Curator review; preserve
evidence and repeat after every remediation push.
Four-hour claims renew at durable boundaries/pushes, never solely for renewal.
No-PR handoffs preserve worktrees; archive only expired claims without open PR;
preserve and escalate open-PR expirations.
Claim resume:
`go tool workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]`.
PR resume: `go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]`.
Transient agent, checkout, transport, and challenge failures remain retryable.
Exactly three authenticated Examiner `fail` receipts add `needs-human` and return
Backlog. Write blocker/evidence Markdown, then run
`go tool workflowctl handoff ISSUE --body-file FILE --needs-human`; it proves
OPEN plus Project identity, applies `needs-human`/Backlog, and posts last.
Reread incomplete or ambiguous phases before retrying.
Claim resume binds exact handoff/comment/run/head, expired claim, no PR, Project
identity/status, and a unique clean/unlocked same-run worktree. Generic handoffs
may omit head/SHA/commit labels; labels must carry one full 40-hex expected SHA;
malformed/ambiguous labels are terminal before mutation.
Claim/renewal markers are exact-message single-parent empty commits;
source-bearing/merge commits and malformed ref namespaces are terminal. Require
`refs/heads/` for remote and `origin/` for tracking refs. Keep `needs-human` until renewal
verification, then converge to Project `Picked`. Initial resume requires
OPEN+needs-human+Backlog before mutation; only a verified renewal child permits
idempotent convergence from Backlog/Picked even with the label removed.
`workflowctl sync` updates Project status/claim refs, not `main`/submodules;
run-local refs are inventory-only. `base-sync` fast-forwards `main`/pins; never
resets/rebases/stashes/discards.
After draft, set `PR_NUMBER="$(gh pr view --json number --jq '.number')"` and
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"`; use
exact REST SHA for signals/audit/evidence, never `origin/main` or merge-base.
`no-relevant-target`/`not-measured` are valid; policy fuzz is health, not conformance.
Before evidence/challenge/finish, resolve/match REST base/head, recompute signals,
compare canonical JSON, preserve non-owned PR bytes, and use exact `pending`/
`evidence-ready` records. Challenge/finish bind exact REST base/head, audit,
Curator, current-state triggers, and body/evidence digests.
Unresolved challenges survive snapshots; original comments and authenticated
records remain immutable. One-use challenges expire after two hours;
`go tool workflowctl evaluation resolve PR --challenge ID --reason-file FILE`
records authenticated-no-verdict: no verdict or merge authority. Fresh Examiner
contexts reject wrong-head/stale/reused/malformed/caller-selected results;
complete-equivalent trusted receipts form rounds and a pass is merge proof.
Cleanup verifies packet-scoped ownership, preserves ambiguity/unrelated refs, and
is exact/idempotent; `claim prune ISSUE` requires merged proof. Finish/recovery
use SHA-bound REST and exact GitHub-effective references. Pass `pr finish` a
plain-text problem/outcome/rationale/invariants summary outside the repository:
UTF-8, non-empty, <=8 KiB, LF-only, no surrounding/trailing whitespace, controls,
format/separator characters, generated claim trailers, or PR metadata.
