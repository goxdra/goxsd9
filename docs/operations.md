# Scheduled operations

Paseo schedules jobs from clean coordination checkout in America/New_York.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | GPT-6 Sol/medium | `Run $develop for this repository.` |
| Backlog | 10:30 daily | GPT-6 Sol/medium | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | GPT-6 Astra/xhigh | `Run $retro for this repository.` |
Jobs are non-interactive. Develop needs clean canonical `main`/`origin/main`
and recursive pins; `doctor` enforces; stale jobs run `base-sync`. It claims
one Ready issue, opens draft PR, then squash-merges evaluated head. Managed-
document/source-trigger heads require exact audit and fresh read-only Curator
pass; repeat after remediation. Renew four-hour claims at durable boundaries/
pushes, never solely. No-PR handoffs preserve worktrees; archive expired
PR-free claims; escalate open-PR expirations.
Claim resume:
`go tool workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]`.
Archived no-PR sibling: from the current claim run `go tool workflowctl claim release-archived ISSUE --run-id RUN --expected-head SHA [--dry-run]`; exact clean/unlocked archive proof, including submodules, preserves refs/comments.
PR resume: `go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]`.
Agent/checkout/transport/challenge failures remain retryable. Three
authenticated Examiner `fail` receipts add `needs-human`/Backlog. Write
blocker/evidence Markdown; run
`go tool workflowctl handoff ISSUE --body-file FILE --needs-human`; it proves
OPEN/Project identity, applies `needs-human`/Backlog, then posts last.
Reread ambiguity before retry.
Claim resume binds expired claim, handoff/comment/run/head, no PR, Project, unique unlocked worktree. Clean forms require no-source/
no-PR evidence covering every PR, pull-request, and workflow-path mention;
exceptions stay issue-scoped.
Head labels require full SHA. Dirty:
`# Dirty no-PR claim handoff: issue #N`, blank; `Run:`,
`Original claim head:`, `Current claim head:`, `Fixed branch:`, `Local branch:`,
`Worktree:`, `Preserved state SHA-256:` with backticked values; end `No source commit or PR was published.` and LF.
From the preserved run-local claim worktree, `go tool workflowctl claim resume-state`
gives digest; recheck staged/unstaged/nonignored untracked bytes.
Claim/renewal markers: exact-message empty single-parent commits; source-bearing,
merge, malformed refs fail. Remote `refs/heads/`, tracking `origin/` required.
Keep needs-human until verified renewal; then Project Picked. Initial:
OPEN+needs-human+Backlog; verified child permits idempotent convergence.
`workflowctl sync` updates Project status/claim refs, not `main`/submodules;
run-local refs are inventory-only. `base-sync` fast-forwards `main`/pins; never
resets/rebases/stashes/discards.
Set `PR_NUMBER="$(gh pr view --json number --jq '.number')"` and
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"`; use
exact REST SHA for signals/audit/evidence, never `origin/main` or merge-base.
`no-relevant-target`/`not-measured` are valid; policy fuzz is health, not conformance.
Before evidence/challenge/finish, match REST base/head, recompute signals,
compare canonical JSON, preserve non-owned PR bytes, use exact `pending`/
`evidence-ready` records. Challenge/finish bind exact REST base/head, audit,
Curator, current-state triggers, and body/evidence digests.
Challenges/comments/records are immutable.
`go tool workflowctl evaluation resolve PR --challenge ID --reason-file FILE`
records no-verdict after expiry or REST-proven changed head without receipt;
it binds both heads, grants no merge authority. Fresh Examiners reject stale/
malformed/caller-selected results; equivalent receipts form rounds; passes authorize merge.
Cleanup verifies ownership, preserves ambiguous/unrelated refs, and
is idempotent; `claim prune ISSUE` requires merged proof. Finish/recovery
use SHA-bound REST and exact GitHub-effective references. Pass `pr finish` a
plain-text problem/outcome/rationale/invariants summary outside the repository:
UTF-8, non-empty, <=8 KiB, LF-only, no surrounding/trailing whitespace, controls,
format/separator characters, generated claim trailers, or PR metadata.
