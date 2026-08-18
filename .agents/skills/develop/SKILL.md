---
name: develop
description: Autonomously select, claim, implement, evaluate, merge one goxsd9 packet.
---

# Develop

Complete packet.

## Control plane

Root owns claim, decomposition, lifecycle; avoids repeating delegated
work. Branch/files/handoffs are shared memory; transcript is not.

Children use exact configured roles, `fork_turns: "none"`, and task-local context.
Scribe/Mason default to fresh read-only consultations; omit only with a PR
exemption. Smith alone writes source/tests and owns remediation; root writes
require a recorded narrow mechanical exemption. Curator is fresh per head;
Examiner fresh, challenge-bound.

Handoffs MUST state decisions, evidence locations, risks, next actions; Smith
names changed paths/tests. Preserve Curator/Examiner JSON.

## Protocol

1. From coordination, read `AGENTS.md`; run `go tool workflowctl doctor`.
   It requires canonical clean `main` equal to fetched `origin/main`, with
   recursive pins ready. Repair stale launches with `base-sync`; rerun doctor,
   `sync`, `pick`.
2. Claim the issue with `go tool workflowctl claim acquire ISSUE`. On claim
   loss, preserve state: no edit/push/reuse or Project-status change; defer
   unrelated ready work to a later invocation; never backlog-loop or widen
   scope. Only in that later invocation ask workflowctl for another eligible
   issue and use its worktree.
3. Read issue, `README.md`, `ARCHITECTURE.md`, `PLAN.md`
   phase, relevant decisions. Claim at most one companion first
   for shared implementation/proof.
4. Give Scribe specification question and Mason architecture question,
   with context and handoff contract.
5. Decompose packet; give Smith contract, files, evidence. Smith
   implements, tests, fixes failures. Follow `AGENTS.md`; mechanize.
   Unfinished boundaries need unsupported diagnostics with feature ID, `Loc`,
   versioned specification reference. Do not add untracked TODOs; turn
   actionable discoveries into issues.
6. Renew before pushes and required durable boundaries with `go tool
   workflowctl claim renew`; never wake or poll solely
   to renew.
7. Run `go tool workflowctl check`, fix every failure, update affected
   docs/comments. Do not redo Smith's investigation.
   For `syntax.go` or `datatype.go` changes, run
   `go tool workflowctl develop-signals --base BASE_SHA`; it replays checked-in
   corpora and runs targets for bounded offline single-worker duration.
   Text reports coverage/fuzz status, target names, and
   `no-relevant-target`, not exact values or corpus names. Use `--format json`
   for exact computed affected-package/repository deltas or selected-target
   evidence; request replay evidence separately. Regressions require a
   JSON explanation containing package, reason, computed base/head;
   repository total is context.
   Coverage/fuzz health never represent XSD conformance, catalog inventory,
   or evaluation fuzz excluded from these signals.
8. Commit/push with the `AGENTS.md` title convention. Open a draft PR with
   `go tool workflowctl pr open ISSUE --title TITLE --body-file FILE`; include
   outcome, consultation, verification, conformance, packet issues.
9. On every pushed head run `go tool workflowctl docs audit --base origin/main`.
   Managed documents require a fresh read-only Curator with audit, diff, paths,
   charters, head. Curator checks placement, relevance, duplication,
   history, replacement. Preserve Curator JSON; repeat after remediation.
10. Run `go tool workflowctl evaluation challenge PR`; for every
    managed-document review head, fresh read-only Curator review/result is
    mandatory. Give challenge, PR state, tests, audit, Curator result,
    attestation shape, rubric to a fresh read-only Examiner context,
    challenge-bound. Examiner inspects
    source, reruns audit, rejects stale/missing Curator evidence, returns exact
    `goxsd9/examiner-attestation/v1` JSON with `schema`, `challenge`, `evaluator`,
    `runID`, `pullRequest`, `head`, `verdict`, `summary`, findings; failure
    findings require `location`, `impact`, and `requiredCorrection`. Copy it
    byte-for-byte outside repository; record it with `go tool workflowctl
    evaluation record PR --attestation-file FILE`; never choose or rewrite
    verdict. On failure, Smith fixes findings, checks, pushes, repeats
    Curator/challenge/Examiner. Three failed rounds mark `needs-human`; hand off
    evidence.
11. On matching-head pass, write plain-text summary outside repository
    covering problem, outcome, rationale, and consequential decisions
    or invariants; omit metadata. Separate plain-text summary is intended
    for future development, backlog, and retrospective workflows; do not copy
    or parse any PR Markdown into the squash body. Keep workflow metadata in
    records. Pass it to
    `go tool workflowctl pr finish PR --summary-file FILE`, which verifies
    packet before SHA-bound REST, GraphQL-independent squash merge, converges
    canonical base, cleans only exact refs, clean-claim worktrees, expected-SHA
    branches proven by immutable pre-merge proof: base/head/closure/body
    metadata; recovery refuses drift.
    Convergence/cleanup failure: merge complete; preserve artifacts; run
    idempotent `go tool workflowctl pr recover PR`; recovery requires
    SHA-bound REST merge; remains GraphQL-independent. Use `claim prune ISSUE`
    only with merged proof. Draft replacement: close draft; create
    identical-head ready PR via REST; obtain new challenge, fresh Examiner.

## Waiting and pilot

Waits are logical barriers. Continue while healthy work and lease renewal
permit; never narrow, pressure, spawn a writer, or duplicate work. Interrupt
only for explicit failure, cancellation, invalid scope, or lost lease. Follow up
only for incomplete handoffs or bounded input; timing guides, not an
OpenAI runtime guarantee.

For three packets (mechanical, specification-heavy, remediation), record
aggregate root compactions, peak context, output volume, elapsed time,
Examiner rounds/verdict, and quality across diagnostics, tests, docs, review.
Zero normal-packet root compactions and
under 50% effective root context before review are
optimization signals; quality must not regress. Never require sessions or
telemetry.
## Failure behavior

- Retry transient operations with bounded backoff; three failed recoveries make
  a located, actionable `needs-human` handoff.
- Leave incomplete branches/worktrees recoverable. Never force-push over active
  claim or bypass required check.
- Continue unrelated ready work only in a later invocation; do not backlog-loop.
