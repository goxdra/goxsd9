# Scheduled operations

Paseo schedules jobs from clean coordination checkout in America/New_York.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | GPT-6 Sol/medium | `Run $develop for this repository.` |
| Backlog | 10:30 daily | GPT-6 Sol/medium | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | GPT-6 Astra/xhigh | `Run $retro for this repository.` |
`doctor` requires clean canonical `main`, fetched `origin/main`, and recursive
pins; stale jobs run `base-sync`. Develop claims one Ready issue, drafts a PR,
merges evaluated head. Managed/triggered heads require exact audit and
passing Curator after each push. Renew at durable boundaries/pushes.
Preserve no-PR handoffs; archive only expired claims without open PR.
Claim resume:
`go tool workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]`.
PR resume: `go tool workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]`.
Use original expired REST PR SHA. CAS-push empty same-run marker; preserve
local work and needs-human/Backlog. Complete Git operations; commit/clean
local work. Rerun with `--integrate` and original
`--expected-head SHA` to attach marker, restore Picked. Renewal/push reject
pending work.
Transient agent, checkout, transport, and challenge failures remain retryable.
Exactly three authenticated Examiner `fail` receipts add `needs-human` and return
Backlog. Write blocker/evidence Markdown; run
`go tool workflowctl handoff ISSUE --body-file FILE --needs-human`; it proves
OPEN/Project identity, applies `needs-human`/Backlog, then posts last.
Reread incomplete/ambiguous phases before retry.
Claim resume binds exact handoff/comment/run/head, expired claim, no PR, Project
identity/status, and a unique clean/unlocked same-run worktree. Three complete
generic no-PR forms each cover every PR/pull-request/workflow-path mention.
Compatibility exceptions are exact, issue-scoped complete forms. Generic handoffs may
omit head/SHA/commit labels; present labels require one full 40-hex expected SHA;
malformed/ambiguous labels are terminal before mutation.
Claim/renewal markers are exact-message single-parent empty commits;
source-bearing/merge commits and malformed ref namespaces are terminal. Require
`refs/heads/` for remote and `origin/` for tracking refs. Keep `needs-human` until renewal
verification, then converge to Project `Picked`. Initial resume requires
OPEN+needs-human+Backlog before mutation; only a verified renewal child permits
idempotent Backlog/Picked convergence after label removal.
`workflowctl sync` updates Project status/claim refs, not `main`/submodules;
run-local refs are inventory-only. `base-sync` fast-forwards `main`/pins; never
resets/rebases/stashes/discards.
After draft, set `PR_NUMBER="$(gh pr view --json number --jq '.number')"` and
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"`;
use it for signals/audit/evidence, never `origin/main` or merge-base.
`no-relevant-target`/`not-measured` are valid; policy fuzz is health, not conformance.
Before evidence/challenge/finish, match REST base/head, recompute signals,
compare canonical JSON, preserve non-owned PR bytes, and use `pending`/`evidence-ready`
records. Challenge/finish bind REST base/head, audit, Curator, triggers, and
body/evidence digests. Challenges, comments, and records remain immutable.
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
