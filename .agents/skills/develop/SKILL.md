---
name: develop
description: Autonomously select, claim, implement, evaluate, and merge one goxsd9 work packet.
---

# Develop

Complete packet; do not stop at planning/PR.

## Control plane

Root owns claim/lease, decomposition/handoffs, push/audit/challenge/merge; it
does not repeat delegated research, source inspection, implementation, or test
diagnosis. Branch/worktree, issue/PR, files, and handoffs are shared memory;
transcript is not.

Every child uses its exact `.codex/agents/` role, `fork_turns: "none"`, and
task-local context. Scribe and Mason are default fresh read-only consultations;
omit one only for a mechanical exemption recorded in PR. Smith is sole source
writer and owns routine tests/remediation. Root writing requires a narrow
mechanical exemption recorded in handoff. Curator is fresh per managed-document
head; Examiner is fresh and challenge-bound per review round.

Every child handoff MUST be concise and state decisions, evidence locations, risks, required next actions;
Smith names changed paths/tests. Preserve Curator/Examiner JSON byte-for-byte.

## Protocol

1. From the coordination checkout, read `AGENTS.md`, then run `go tool workflowctl doctor`.
   It requires this canonical primary on clean `main` equal to freshly fetched
   `origin/main`, with recursive pins ready. For stale/noncanonical launch, run
   `go tool workflowctl base-sync` there, rerun doctor, then `sync` (Project only)
   and `pick`.
2. Claim the issue with `go tool workflowctl claim acquire ISSUE`. If claim
   loses, no edit/push/reuse or Project status change; pick again; use worktree.
3. Read the issue, `README.md`, `ARCHITECTURE.md`, `PLAN.md`
   phase, and relevant decisions. At most one companion issue;
   claim it first and include it for shared implementation or proof.
4. Give Scribe specification question and Mason architecture question,
   with task-local context and handoff contract.
5. Decompose the packet and give Smith implementation contract, files, and
   acceptance evidence. Smith implements outcome, runs
   routine tests, and diagnoses and fixes failures. Follow `AGENTS.md`; mechanize
   repeated work and test it. At unfinished boundaries, return an
   unsupported diagnostic with feature ID, `Loc`, and versioned specification
   reference. Turn actionable discoveries into issues; finish needed work.
6. Renew before pushes and at durable workflow boundaries when remaining time
   requires it with `go tool workflowctl claim renew`; never wake or poll solely
   to renew.
7. Run `go tool workflowctl check`, fix every failure, and update affected
   docs/comments. Do not redo Smith's investigation for a longer
   transcript.
8. Commit and push with the `AGENTS.md` title convention. Open a draft PR with
   `go tool workflowctl pr open ISSUE --title TITLE --body-file FILE`; describe
   outcome, consultation, verification, conformance, and every packet issue.
9. On every pushed head run `go tool workflowctl docs audit --base origin/main`.
   Managed-document heads require a fresh read-only Curator with exact audit,
   diff, paths, charters, and head. Curator checks placement, relevance,
   duplication, history, and replacement; deletion is not improvement. Preserve
   JSON (`runID`, `head`, `verdict`, `summary`, `findings`; revise adds path,
   reason, requiredChange). Repeat after each remediation; no exemption.
10. Run `go tool workflowctl evaluation challenge PR` and give its challenge,
    PR state, tests, audit, Curator result/exemption, attestation shape, and
    rubric to a fresh read-only Examiner context with Luna. Examiner inspects
    source, reruns the audit, rejects stale/missing Curator evidence, and returns exact
    `goxsd9/examiner-attestation/v1` JSON with `schema`, `challenge`, `evaluator`,
    `runID`, `pullRequest`, `head`, `verdict`, `summary`, and `findings`; failure
    findings require `location`, `impact`, and `requiredCorrection`. Copy it
    byte-for-byte outside the repository and record it with `go tool workflowctl
    evaluation record PR --attestation-file FILE`; never choose or rewrite the
    verdict. On failure, Smith fixes findings, checks, pushes, and repeats
    Curator/challenge/Examiner. Three failed rounds mark `needs-human` and hand
    off evidence.
11. On a matching-head pass, write a plain-text summary outside the repository
    covering problem, outcome, rationale, and decisions; omit status, commands,
    review metadata; keep workflow metadata in records. Pass it to
    `go tool workflowctl pr finish PR --summary-file FILE`, which verifies the
    packet before SHA-bound squash, converges canonical base, and cleans only
    exact proof-backed refs, clean claim worktrees, and expected-SHA branches.
    If convergence or cleanup fails, the merge is complete: preserve artifacts
    and run idempotent `go tool workflowctl pr recover PR`. Use `claim prune
    ISSUE` only with merged proof. For draft replacement, close the draft,
    create an identical-head ready PR through REST, then obtain a new challenge
    and fresh Examiner.

## Waiting and pilot

Waits are logical barriers. Poll/status timeouts are observational: continue
while healthy child work and lease renewal permit; never narrow, pressure, spawn
a writer, or duplicate work. Interrupt/recover only for explicit failure or
cancellation, invalid scope, or inability to renew; never wake solely to renew.
Follow up only for incomplete/ambiguous handoffs or bounded input. Timing is
guidance, not a runtime guarantee.

For three packets (mechanical, specification-heavy, remediation), record
compactions, peak context, output, elapsed time,
Examiner rounds/verdict, and quality across diagnostics, tests, docs, and review.
Zero compactions and under 50% context are optimization signals; quality must not
regress. Never require undocumented `~/.codex/sessions` or CI/merge telemetry.
## Failure behavior

- Retry transient operations with bounded backoff; three failed recoveries make
  a located, actionable `needs-human` handoff.
- Leave incomplete branches/worktrees recoverable. Never force-push over an
  active claim or bypass a required check.
- Continue unrelated ready work only in a later invocation; do not backlog-loop.
