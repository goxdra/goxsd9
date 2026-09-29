---
name: retro
description: Review goxsd9 repository history, conformance movement, agent failures, and workflow friction, then mechanize improvements. Use for the Sunday scheduled retrospective.
---

# Retro

Turn the last week into concrete system improvements. Git remains the history;
do not add a prose diary to the repository.

## Protocol

1. Read `AGENTS.md`, `PLAN.md`, and `ARCHITECTURE.md`. Run:

   ```sh
   go tool workflowctl doctor
   go tool workflowctl sync
   go tool workflowctl history --since 7d
   go tool workflowctl backlog health
   ```

2. Create one retrospective issue through `workflowctl` using a real Markdown
   body file. Summarize outcomes, not session transcripts.
3. Inspect merged packets, lead time, conformance delta, unsupported clusters,
   repeated Examiner findings, failed recovery, stale claims, documentation
   churn reported by `workflowctl history`, and ready-buffer health.
4. For each repeated mistake, identify the producing step and its owner.
   Mechanize bounded prevention there. State its preconditions and order,
   output ownership, failure behavior, and handoff. Add or strengthen executable
   regressions for the repaired path and safe refusal. Prefer tools or owner
   skills over reminders; keep past incidents in GitHub.
5. Revisit blocked and `needs-human` work. Reorder dependencies, split scope, or
   create missing research issues when evidence supports it.
6. Create documentation cleanup work only when churn reveals an independently
   actionable relevance or consolidation outcome. Update `PLAN.md`,
   `ARCHITECTURE.md`, or a decision record only when current reality changed.
   Repository edits require a normal claimed work packet and evaluated PR.
7. Create and link actionable follow-up issues, synchronize the Project, then
   close the retrospective issue. Do not leave the retro issue as a second
   backlog.
