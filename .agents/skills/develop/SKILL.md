---
name: develop
description: Autonomously select, claim, implement, evaluate, and merge one goxsd9 work packet.
---

# Develop

Complete packet; do not stop at planning/PR.

## Control plane

Root owns claim/lease, decomposition, and lifecycle; it does not repeat
delegated work. Branch/worktree, issue/PR, files, and handoffs are shared
memory; transcript is not.

Every child uses its exact role, `fork_turns: "none"`, and local context.
Scribe/Mason are default fresh read-only consultations; omit only with a PR
exemption. Smith owns source, tests, and remediation. Curator is fresh per
document head; Examiner is fresh and challenge-bound.

Every child handoff MUST state decisions, evidence locations, risks, and next
actions; Smith names changed paths/tests. Preserve Curator/Examiner JSON.

## Protocol

1. From coordination, read `AGENTS.md`, then run `go tool workflowctl doctor`.
   It requires canonical clean `main` equal to fetched `origin/main`, with
   recursive pins ready. Repair stale launches with `base-sync`, rerun doctor,
   then `sync` and `pick`.
2. Claim the issue with `go tool workflowctl claim acquire ISSUE`. If claim
   loses, no edit/push/reuse or Project status change; pick again; use worktree.
3. Read the issue, `README.md`, `ARCHITECTURE.md`, `PLAN.md`
   phase, and relevant decisions. At most one companion issue;
   claim it first and include it for shared implementation or proof.
4. Give Scribe specification question and Mason architecture question,
   with task-local context and handoff contract.
5. Decompose the packet and give Smith the contract, files, and evidence. Smith
   implements, tests, and fixes failures. Follow `AGENTS.md`; mechanize repetition.
   At unfinished boundaries, return an
   unsupported diagnostic with feature ID, `Loc`, and versioned specification
   reference. Turn actionable discoveries into issues; finish needed work.
6. Renew before pushes and at durable workflow boundaries when remaining time
   requires it with `go tool workflowctl claim renew`; never wake or poll solely
   to renew.
7. Run `go tool workflowctl check`, fix every failure, and update affected
   docs/comments. Do not redo Smith's investigation for a longer
   transcript.
   When the packet changes `syntax.go` or `datatype.go`, run
   `go tool workflowctl develop-signals --base BASE_SHA --format text`; this
   replays checked-in parser/datatype corpora and runs each selected
   target for the bounded default duration, offline and with one worker. The
   reports affected-package and repository coverage deltas, fuzz
   health, and `no-relevant-target`. Affected-package
   regressions require a JSON explanation file containing the
   package, concrete reason, and the command-computed base/head values; the
   repository total is context. Coverage and fuzz health never represent
   XSD conformance, catalog inventory, or evaluation fuzz excluded from
   these signals.
8. Commit/push with the `AGENTS.md` title convention. Open a draft PR with
   `go tool workflowctl pr open ISSUE --title TITLE --body-file FILE`; include
   outcome, consultation, verification, conformance, and packet issues.
9. On every pushed head run `go tool workflowctl docs audit --base origin/main`.
   Managed documents require a fresh read-only Curator with audit, diff, paths,
   charters, and head. Curator checks placement, relevance, duplication,
   history, and replacement. Preserve its JSON and repeat after remediation.
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
    covering problem, outcome, rationale, and decisions; omit metadata;
    keep workflow metadata in records. Pass it to
    `go tool workflowctl pr finish PR --summary-file FILE`, which verifies the
    packet before SHA-bound squash, converges canonical base, and cleans only
    exact refs, clean claim worktrees, and expected-SHA branches proven by
    immutable pre-merge proof with base/head/closure/body metadata; recovery
    refuses drift.
    If convergence or cleanup fails, the merge is complete: preserve artifacts
    and run idempotent `go tool workflowctl pr recover PR`. Use `claim prune
    ISSUE` only with merged proof. For draft replacement, close the draft,
    create an identical-head ready PR through REST, then obtain a new challenge
    and fresh Examiner.

## Waiting and pilot

Waits are logical barriers. Continue while healthy work and lease renewal
permit; never narrow, pressure, spawn a writer, or duplicate work. Interrupt
only for explicit failure, cancellation, invalid scope, or lost lease. Follow up
only for incomplete handoffs or bounded input; timing is guidance.

For three packets (mechanical, specification-heavy, remediation), record
compactions, context, output, elapsed time, Examiner verdict, and quality across
diagnostics, tests, docs, and review. Zero compactions and under 50% context are
optimization signals; quality must not regress. Never require undocumented
sessions or telemetry.
## Failure behavior

- Retry transient operations with bounded backoff; three failed recoveries make
  a located, actionable `needs-human` handoff.
- Leave incomplete branches/worktrees recoverable. Never force-push over an
  active claim or bypass a required check.
- Continue unrelated ready work only in a later invocation; do not backlog-loop.
