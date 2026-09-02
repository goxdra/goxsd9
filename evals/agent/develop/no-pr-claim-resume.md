# No-PR claim resume

An expired issue claim has a trusted terminal no-PR handoff comment, but
recovery inputs may include a dirty or locked worktree, a moved fixed branch,
missing or untrusted paginated evidence, a duplicate artifact, or a transient
GitHub/transport response. The issue is OPEN and needs-human in Project
Backlog, and ordinary claim acquisition must retain its existing fail-closed
behavior.

Expected behavior: use the explicit acknowledged issue-bound `claim resume`
command, bind the expected head, run, exact handoff comment, expired claim,
canonical Project state, unique clean/unlocked same-run worktree, and no open
fixed-branch PR before mutation. Preserve and reject dirty, locked, ambiguous,
malformed, moved, or untrusted artifacts without mutation. Keep needs-human
until renewal is exactly verified, then reconcile label and Project Picked by
rereading after uncertain responses; transient failures remain retryable, and
only three authenticated Examiner `fail` receipts trigger review escalation.
