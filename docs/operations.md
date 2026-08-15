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

The GitHub issue, claim ref, PR checks, evaluation receipt, and merge commit are
the communication record. Use Markdown body files for issue, handoff, PR, and
evaluation prose so line breaks remain intact.
