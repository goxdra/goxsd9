# No-PR claim resume

An expired issue claim has a trusted terminal no-PR handoff comment, but
recovery inputs may include a dirty or locked worktree, a moved fixed branch,
missing or untrusted paginated evidence, a duplicate artifact, or a transient
GitHub/transport response. The issue is OPEN and needs-human in Project
Backlog, and ordinary claim acquisition must retain its existing fail-closed
behavior.

Expected behavior: use the explicit acknowledged issue-bound `claim resume`
command, bind the expected head, run, exact handoff comment, expired canonical
claim, canonical Project identity/status, unique clean/unlocked same-run
worktree, and no open fixed-branch PR before mutation. Claim and renewal
markers must be generated empty single-parent commits with exact raw message /
trailers; source-bearing and merge commits are terminal. Preserve and reject
dirty, detached, locked, duplicate, ambiguous, malformed, moved, or untrusted
artifacts without mutation. Exact issue/path/run/lease/fixed/local/head tokens
must be checked when present in evidence; token substrings and contradictory
prose never authenticate a handoff.

The matrix includes authentic #287 comment 5488794928 and #240 comment
5501405525 (with #240’s unrelated dirty `validation.go` proving fail-closed
preservation), while #253 has no terminal handoff and remains blocked. Ordinary
acquisition remains unchanged. Cover pre-existing local-only, remote-only, and
fully converged renewal children; detached/duplicate worktrees; source-bearing
and merge renewal rejection; exact token spoofing; and PR/Project races before
the first GitHub mutation. Fresh-proof transport errors retain their original
retryable disposition and cause; malformed successful API/ref/history data is
terminal. Agent, checkout, transport, and challenge failures remain retryable;
exactly three authenticated Examiner `fail` receipts trigger escalation.
Keep `needs-human` until renewal is verified, then reconcile label and Project
`Picked`, rereading after every ambiguous response and preserving artifacts.
