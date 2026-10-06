# No-PR claim resume

An expired issue claim has a trusted terminal no-PR handoff comment, but
recovery inputs may include a dirty or locked worktree, a moved fixed branch,
missing or untrusted paginated evidence, a duplicate artifact, or a transient
GitHub/transport response. The issue is OPEN and needs-human in Project
Backlog, and ordinary claim acquisition must retain its existing fail-closed
behavior.

Expected behavior: use the explicit acknowledged issue-bound `claim resume`
command, bind the expected head, run, exact handoff comment, expired canonical
claim, canonical Project identity/status, unique unlocked same-run
worktree with its exact local-state proof, and no open fixed-branch PR before
mutation. Claim and renewal markers must be generated empty single-parent
commits with exact raw message / trailers; source-bearing markers and merges in
the claim chain are terminal. Source-changing work may intervene between
markers; renewal leases may differ. Preserve and reject detached, locked,
duplicate, ambiguous, malformed, moved, or untrusted artifacts without mutation.
Clean forms need strict clean proof; authenticated dirty forms need an exact
handoff-bound state digest. Reject changed or unbound dirty state without
mutation. Check exact issue/path/run/lease/fixed/local tokens when present in
evidence; a generic handoff may omit head/SHA/commit labels, but any present
recognized label must carry one full 40-hex expected SHA.
Malformed or ambiguous labels are terminal before mutation. Generic no-PR
authentication uses only the finite complete forms recorded by workflowctl;
every PR, pull-request, or workflow-path mention must be wholly covered by an
approved form. The #287 form is generic; historical #240/#305 compatibility
remains isolated, and token substrings or contradictory prose never authenticate a handoff.
Forms are case-insensitive and permit only historical line-wrap whitespace;
punctuation, word boundaries, conjunctions, and clause boundaries stay exact.

The reusable matrix accepts authentic clean and dirty terminal handoffs, blocks
missing or mismatched evidence, and preserves rejected local artifacts. Ordinary
acquisition remains unchanged. Cover pre-existing local-only, remote-only, and
fully converged renewal children; detached/duplicate worktrees; source-bearing
and merge renewal rejection; dirty recovery from a prior same-run renewal with
an archived sibling and idempotent Project retry; exact token spoofing; and
PR/Project races before the first GitHub mutation. Fresh-proof transport errors
retain their original retryable disposition and cause; malformed successful
API/ref/history data is terminal. Agent, checkout, transport, and challenge
failures remain retryable;
exactly three authenticated Examiner `fail` receipts trigger escalation.
Keep `needs-human` until renewal is verified, then reconcile label and Project
`Picked`, rereading after every ambiguous response and preserving artifacts.
