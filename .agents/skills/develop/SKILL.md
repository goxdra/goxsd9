---
name: develop
description: Autonomously select, claim, implement, evaluate, merge one goxsd9 packet.
---

# Develop

Complete packet.

## Control plane

Root owns claim/decomposition/lifecycle; do not repeat delegated
research/source-inspection/implementation/test-diagnosis. Branch/files/handoffs
are shared state, not transcript.

Children use exact configured roles from `.codex/agents/`, `fork_turns: "none"`,
and task-local context. Scribe/Mason default fresh read-only; omit only with
recorded narrow demonstrably-mechanical exemption. Smith sole source/test/remediation writer;
root writes require recorded narrow mechanical exemption. Curator fresh per-head;
Examiner fresh/challenge-bound.

Handoffs MUST state decisions, evidence locations, risks, next actions; Smith
names changed paths/tests. Preserve Curator/Examiner JSON.

## Protocol

1. From coordination, read `AGENTS.md`; run `go tool workflowctl doctor`.
   Requires canonical clean `main` equal to fetched `origin/main` plus recursive
   pins; repair stale launches with `base-sync`; rerun doctor, `sync`, `pick`.
2. Claim via `go tool workflowctl claim acquire ISSUE`. If lost, don't
   edit/push/reuse/change Project; ask workflowctl for an eligible issue/worktree.
   Never backlog-loop/widen.
3. Read issue, `README.md`, `ARCHITECTURE.md`, `PLAN.md` phase, decisions; claim
   at most one companion for shared implementation/proof.
4. Give Scribe specification and Mason architecture questions, context, handoff contract.
5. Decompose packet; give Smith contract/files/evidence. For affected phase
   boundaries, matrix only covers sibling axes (edition/policy; named/anonymous/
   inline/ref shape; graph visibility/cycles; supported/invalid/unsupported;
   location/order/provenance), marking N/A. It is handoff-only; cannot widen.
   Smith implements/tests/fixes. Follow `AGENTS.md`; mechanize. Unfinished
   boundaries need unsupported diagnostics with feature ID, `Loc`, and versioned
   specification reference; turn actionable discoveries into issues, not TODOs.
6. Renew before pushes and required durable boundaries with `go tool
   workflowctl claim renew`; never wake or poll solely to renew.
7. Run `go tool workflowctl check`, fix failures/update docs. Do not redo
   Smith's investigation.
8. Commit/push using `AGENTS.md`; open the initial draft PR from that head with
   `go tool workflowctl pr open ISSUE --title TITLE --body-file FILE`, including
   outcome, consultation, verification, conformance, and packet issues.
9. Once a PR exists, after each push establish `PR_NUMBER`:
   `PR_NUMBER="$(gh pr view --json number --jq '.number')"`; set exact REST
   `BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"`.
   Save `develop-signals --base "$BASE_SHA" --format json` and `docs audit
   --base "$BASE_SHA" --format json` before evidence update. Automatic policy
   fuzz follows changed boundaries; validate optional repeatable
   `--additional-fuzz PACKAGE:TARGET` at current head. Request checked-in
   corpus replay separately (bounded, offline, single-worker). JSON
   deltas/targets and `no-relevant-target`/`not-measured` are valid; fuzz is
   health, not conformance. Evidence status must use the exact `pending` and
   `evidence-ready` records; do not infer either state from prose. Before
   evidence update, challenge, or finish, workflowctl
   resolves exact REST base/head, matches local commits, recomputes v2 signals/
   policy, and compares canonical payload. Managed docs require Curator audit,
   diff, paths, charters, and head; check placement, relevance, duplication,
   history, replacement; preserve JSON and repeat after remediation.
10. Before challenging, reread the exact PR body against head/evidence/implementation;
    correct stale claims without normalizing Examiner identity. After edits rerun
    exact-base evidence/docs audit and fresh Curator where applicable, then obtain
    a fresh body-bound challenge and Examiner. Binding proves identity, not prose;
    never reuse stale challenge/evidence. Run `go tool workflowctl evaluation
    challenge PR`; give challenge, state, tests, audits, Curator result, attestation
    shape, and rubric to a fresh read-only challenge-bound Examiner. Examiner
    inspects source/reruns audit, rejects stale Curator evidence, and returns exact
    `goxsd9/examiner-attestation/v1` JSON; findings require location, impact, and
    requiredCorrection. Copy it byte-for-byte outside; record with `go tool
    workflowctl evaluation record PR --attestation-file FILE`; never choose verdict.
    Smith fixes/checks/pushes and repeats. Exactly three authenticated Examiner
    `fail` receipts mark `needs-human`; failures remain retryable. Then write the
    blocker and run `go tool workflowctl handoff ISSUE --body-file FILE --needs-human`;
    workflowctl validates OPEN/Project identity, applies `needs-human`/`Backlog`,
    and posts last. An unresolved challenge after two hours may use `go tool
    workflowctl evaluation resolve PR --challenge ID --reason-file FILE`; this gives
    neither verdict nor merge proof.
11. On matching-head pass, write an external plain-text problem/outcome/rationale/
    invariants summary; omit metadata and PR Markdown. `go tool workflowctl pr
    finish PR --summary-file FILE` verifies the packet, SHA-bound REST squash/
    cleanup, canonical base/refs, and exact merge proof. If cleanup fails, preserve
    artifacts and run idempotent `go tool workflowctl pr recover PR`; REST fallback
    uses the identical head and a fresh Examiner. `claim prune ISSUE` requires
    merged proof; draft replacement closes the draft, creates an identical-head
    ready REST PR, and requires fresh challenge/Examiner.
## Waiting and pilot

Waits are logical barriers: continue while healthy work and lease renewal
permit; never narrow/pressure/spawn/duplicate. Interrupt only for explicit
failure, cancellation, invalid scope, or lost lease. Follow up only incomplete
handoffs/bounded input; timing is guidance, not a runtime guarantee.

For three packets (mechanical, specification-heavy, remediation), record root
compactions, peak context, output volume, elapsed time, Examiner rounds/verdict,
and quality across diagnostics, tests, docs, and review. Zero normal-packet
compactions and under 50% effective context before review are optimization
signals, never gates; quality must not regress. Never require sessions or telemetry.
## Failure behavior

- Transient failures remain retryable. Exactly three authenticated Examiner `fail`
  receipts mark `needs-human`; no-PR recovery requires exact trusted evidence. Never infer.
- Preserve incomplete worktrees; never force-push claim or bypass checks. After
  one bounded reselection, do not backlog-loop or widen scope.
