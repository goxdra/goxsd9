# Scheduled operations

Paseo owns scheduling outside this repository. Each job starts from the clean
coordination checkout, uses America/New_York time, and sends one direct prompt.
The checked-in skill is the executable procedure; this page records only the
schedule and operator contract.

| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 02:00, 08:00, 14:00, 20:00 daily | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:00 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 12:00 Sunday | Sol, maximum effort | `Run $retro for this repository.` |

Jobs are non-interactive. Develop selects one unblocked Ready issue, claims its
canonical `agent/issue-N` branch, works in the generated worktree, opens a draft
PR, obtains a fresh internal Examiner evaluation, and squash-merges only the
evaluated head after checks pass. It may include at most one separately claimed
companion issue.

Claims last two hours and are renewed at least every 30 minutes and before
shared writes. If an expired claim has no open PR, `workflowctl` preserves its
tip under `agent/archive/` before allowing reassignment. An expired claim with
an open PR requires a `needs-human` handoff so unfinished reviewed work is not
silently replaced.

Three failed evaluation rounds add `needs-human` and return the issue to
Backlog. Transient commands use bounded retries as directed by the skill;
successful runs continue through merge rather than stopping for approval.

Before each fresh Examiner is spawned, `workflowctl evaluation challenge`
records a one-use challenge bound to the current PR head. Examiner returns the
versioned JSON attestation; `workflowctl` derives the verdict and prose from it
and rejects wrong-head, stale, reused, malformed, or caller-selected verdicts.
The scheduling agent supplies fresh-context separation. Personas share the
repository owner's GitHub credential, so the receipt is process evidence, not
a claim of cryptographic identity isolation between local agents.

The guarded merge itself uses GitHub REST with the evaluated head SHA and
squash method. If the GraphQL-only draft-to-ready transition is unavailable,
`workflowctl` closes the preserved draft and creates an identical-head ready
replacement through REST; that PR receives a new challenge and fresh Examiner
before merge. A Project status update that is unavailable after the merge is
reported and converges on the next `workflowctl sync` run.

The GitHub issue, claim ref, PR checks, challenge, evaluation attestation and
receipt, and merge commit are
the communication record. Use Markdown body files for issue, handoff, and PR
prose so line breaks remain intact.
