---
name: backlog
description: Reconcile GitHub tracking, refine issue dependencies and priorities, and maintain the goxsd9 ready-work buffer. Use for the daily scheduled backlog-curation run.
---

# Backlog

Leave a healthy, ordered supply of executable work. Do not implement product
code during this workflow.

## Protocol

1. Read `AGENTS.md`, `PLAN.md`, and `ARCHITECTURE.md`. Run:

   ```sh
   go tool workflowctl doctor
   go tool workflowctl sync
   go tool workflowctl history
   go tool workflowctl backlog health
   ```

2. Reconcile Project state with closed issues, active claim refs, PRs, and
   `needs-human` labels. Issues and refs are authoritative; Project fields are
   the view.
3. Inspect new issues, unsupported feature clusters, conformance gaps, failed
   evaluations, deferred TODO issues, blocked chains, and current phase gaps.
   Search before creating work. Do not duplicate an existing outcome.
4. Make every candidate executable: one outcome, bounded scope, acceptance
   proof, relevant `area/*` and `type/*` labels, priority, effort, phase, and
   specification references when applicable.
5. Use native `blocked by` edges for ordering and native sub-issues for
   decomposition. Split `L` and `XL`; only `XS`, `S`, and `M` may become Ready.
6. Maintain at least eight unblocked Ready issues, including two `XS`, three
   `S`, and two `M`. Prefer current-phase work, issues that unblock the most
   downstream outcomes, user value, and unsupported features that unlock the
   most undisputed conformance tests.
7. Revisit blocked and `needs-human` work. Split, reframe, or supply missing
   evidence when repository facts permit it. Do not guess missing authority or
   credentials.
8. Run `go tool workflowctl sync` and `go tool workflowctl backlog health`
   again. Stop only when drift is resolved and the ready floor is met, or when a
   concise `needs-human` issue explains why it cannot be met.

Use `go tool workflowctl issue create` and body files for all created prose.
Keep status limited to Backlog, Ready, Picked, and Done.
