# Scheduled operations

Paseo schedules from clean coordination checkout in America/New_York.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | GPT-6 Sol/medium | `Run $develop for this repository.` |
| Backlog | 10:30 daily | GPT-6 Sol/medium | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | GPT-6 Astra/xhigh | `Run $retro for this repository.` |
Develop needs clean canonical `main`/`origin/main`
and recursive pins; `doctor` enforces; stale jobs run `base-sync`.
Claim Ready; draft PR; squash-merge evaluated head.
Managed-document/source-trigger heads need exact audit and fresh read-only
Curator pass after each push. Renew claims at boundaries/pushes only.
Preserve no-PR worktrees; archive PR-free expirations; escalate open-PRs.
Claim resume:
`go tool workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]`.
Archived sibling: claimant runs `go tool workflowctl claim release-archived ISSUE --run-id RUN --expected-head SHA [--dry-run]`; clean/unlocked submodule proof preserves refs/comments.
PR resume: `go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run] [--integrate]`.
Reuse the original expired SHA. If pending expires, remote CAS extends its empty
same-run chain without changing local work/status. Commit and clean locally,
then `--integrate` the latest marker to remove needs-human and restore Picked.
Pending renew/push reject.
Agent/checkout/transport/challenge failures retry. Three authenticated Examiner
`fail` receipts add `needs-human`/Backlog. Write blocker/evidence Markdown; run
`go tool workflowctl handoff ISSUE --body-file FILE --needs-human`; it proves
OPEN/Project identity, applies `needs-human`/Backlog, posts last. Reread ambiguous phases.
Claim resume binds handoff/comment/run/head, expiry, no-PR, Project, unlocked same-run worktree.
No-source/no-PR handoffs cover PR/pull-request/workflow-path mentions; exceptions are issue-scoped.
Head labels: full SHA. Same-run markers: exact messages, unchanged trees,
one parent, valid refs to oldest acquisition lease. Reject source-bearing
markers/merges/malformed refs; source commits may intervene; leases may differ.
Dirty handoff: `# Dirty no-PR claim handoff: issue #N`, blank, then `Run:`,
`Original claim head:`, `Current claim head:`, `Fixed branch:`, `Local branch:`,
`Worktree:`, `Preserved state SHA-256:` with backticked values; both heads equal
`--expected-head`; end `No source commit or PR was published.` and LF.
`go tool workflowctl claim resume-state` digests the preserved index, tracked,
and nonignored untracked bytes; recheck before mutation. Require
OPEN+needs-human+Backlog; keep `needs-human` through verified renewal, then
Project `Picked`. Child converges idempotently.
`workflowctl sync` updates Project/claim refs; run-local refs inventory.
`base-sync` fast-forwards `main`/pins; neither resets/rebases/stashes/discards.
Set `PR_NUMBER="$(gh pr view --json number --jq '.number')"` and
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"`; use
exact REST SHA for signals/audit/evidence, never `origin/main` or merge-base.
`no-relevant-target`/`not-measured` are valid; policy fuzz measures health, not conformance.
Before evidence/challenge/finish, match REST base/head, recompute signals,
compare canonical JSON, preserve non-owned PR bytes, use `pending`/
`evidence-ready` records. Challenge/finish bind REST base/head, audit,
Curator, triggers, and body/evidence digests.
Challenges/comments/records are immutable.
`go tool workflowctl evaluation resolve PR --challenge ID --reason-file FILE`
records no-verdict after expiry or REST-proven changed head without receipt;
it binds both heads, grants no merge authority. Fresh Examiners reject stale/
malformed/caller-selected results; equivalent receipts form rounds; passes authorize merge.
Cleanup verifies ownership, preserves ambiguous/unrelated refs;
`claim prune ISSUE` requires merged proof. Finish/recovery use SHA-bound REST
and GitHub-effective references. Pass `pr finish` an external plain-text
problem/outcome/rationale/invariants summary:
UTF-8, non-empty, <=8 KiB, LF-only, no surrounding/trailing whitespace, controls,
format/separator characters, generated claim trailers, or PR metadata.
