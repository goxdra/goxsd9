# Scheduled operations

Paseo schedules jobs from clean coordination checkout in America/New_York.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | GPT-6 Sol/medium | `Run $develop for this repository.` |
| Backlog | 10:30 daily | GPT-6 Sol/medium | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | GPT-6 Astra/xhigh | `Run $retro for this repository.` |
Jobs are non-interactive. Develop requires clean canonical `main` matching fetched
`origin/main` and recursive pins; `doctor` enforces this; stale jobs run
`base-sync` before relaunch. It claims one Ready issue/worktree, opens a draft PR,
then squash-merges its evaluated head. Managed-document/source-trigger heads
require exact audit and fresh read-only passing Curator review; preserve evidence
and repeat after every remediation push. Renew four-hour claims at durable
boundaries/pushes, never solely. No-PR handoffs preserve worktrees; archive only
expired claims without open PR; preserve/escalate open-PR expirations.
Claim resume:
`go tool workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]`.
PR resume: `go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]`.
Transient agent, checkout, transport, and challenge failures remain retryable.
Exactly three authenticated Examiner `fail` receipts add `needs-human` and return
Backlog. Write blocker/evidence Markdown; run
`go tool workflowctl handoff ISSUE --body-file FILE --needs-human`; it proves
OPEN/Project identity, applies `needs-human`/Backlog, then posts last.
Reread incomplete/ambiguous phases before retry.
Claim resume binds expired claim, handoff/comment/run/head, no PR,
Project, unique unlocked worktree. Clean forms require no-source/
no-PR evidence covering PR mentions; exceptions stay issue-scoped.
Optional head labels require one 40-hex SHA. Dirty:
`# Dirty no-PR claim handoff: issue #N`, blank; `Run:`,
`Original claim head:`, `Current claim head:`, `Fixed branch:`, `Local branch:`,
`Worktree:`, `Preserved state SHA-256:` with backticked values; end `No source commit or PR was published.` and LF.
`go tool workflowctl claim resume-state` gives digest; recheck staged/unstaged/untracked bytes.
Claim/renewal markers: exact-message empty single-parent commits; source-bearing,
merge, malformed refs fail. Remote `refs/heads/`, tracking `origin/` required.
Keep needs-human until verified renewal; then Project Picked. Initial:
OPEN+needs-human+Backlog; verified child permits idempotent convergence.
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
Challenges, comments, and records remain immutable.
`go tool workflowctl evaluation resolve PR --challenge ID --reason-file FILE`
records no-verdict after expiry, or earlier when REST proves
changed head and no receipt; it binds both heads and grants no merge authority.
Fresh Examiners reject stale/malformed/caller-selected results; equivalent receipts form rounds; passes prove merge.
Cleanup verifies packet-scoped ownership, preserves ambiguity/unrelated refs, and
is exact/idempotent; `claim prune ISSUE` requires merged proof. Finish/recovery
use SHA-bound REST and exact GitHub-effective references. Pass `pr finish` a
plain-text problem/outcome/rationale/invariants summary outside the repository:
UTF-8, non-empty, <=8 KiB, LF-only, no surrounding/trailing whitespace, controls,
format/separator characters, generated claim trailers, or PR metadata.
