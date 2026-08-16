# Scheduled operations

Paseo schedules outside this repository. Jobs start from the clean coordination
checkout in America/New_York time. Skills are executable procedures; this page
is the operator contract.

| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 02:00, 08:00, 14:00, 20:00 daily | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:00 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 12:00 Sunday | Sol, maximum effort | `Run $retro for this repository.` |

Jobs are non-interactive. Develop claims one unblocked Ready issue on
`agent/issue-N`, uses its worktree, opens a draft PR, and squash-merges only the
evaluated head after checks pass. At most one claimed companion issue is allowed.
Managed-document changes receive a read-only Curator review at the final head;
Examiner remains the authenticated gate.

Claims last two hours and renew every 30 minutes and before shared writes. For
an expired claim without an open PR, `workflowctl` archives its tip under
`agent/archive/` before reassignment. An open PR requires a `needs-human` handoff.

Three failed evaluation rounds add `needs-human` and return the issue to
Backlog. Transient commands use bounded retries as directed by the skill;
successful runs continue through merge rather than stopping for approval.

Before each fresh Examiner, `workflowctl evaluation challenge` records a one-use
challenge bound to the PR head. Examiner returns a versioned JSON attestation;
`workflowctl` derives its verdict and rejects wrong-head, stale, reused,
malformed, or caller-selected results. The scheduler supplies fresh context.
Shared GitHub credentials make the receipt process evidence, not identity proof.

The guarded squash merge uses GitHub REST with the evaluated head SHA. If
draft-to-ready GraphQL is unavailable, `workflowctl` closes the draft and creates
an identical-head ready PR through REST; it needs a new challenge and Examiner.
Failed post-merge Project updates converge on the next `workflowctl sync`.

The GitHub issue, claim ref, PR checks, challenge, evaluation attestation,
receipt, and merge commit are the communication record. At finalization the
agent writes a plain-text summary file outside the repository and passes it to
`workflowctl pr finish PR --summary-file FILE`. It covers the problem, outcome,
rationale, important invariants, and actionable process learning while omitting
status, commands, and claim or review metadata. The command validates artifact
hygiene and uses it as the squash body; the develop and evaluation procedures
own semantic completeness.
Later develop, backlog, and retro runs read it through `workflowctl history`. Use
Markdown body files for issue, handoff, and PR evidence so line breaks remain intact.

The summary artifact contract is UTF-8 plain text in a regular file of at most
8 KiB. LF is the only permitted line separator; one final LF is accepted and
removed. Content must be non-empty, must not start or end with whitespace, and
no line may have trailing Unicode whitespace. Control characters, Unicode format
controls, Unicode line or paragraph separators, and generated `Agent-Persona:`,
`Agent-Run-ID:`, `Agent-Lease-Until:`, or `Agent-Issue:` trailers are rejected.
Markdown punctuation has no special meaning in this artifact.
