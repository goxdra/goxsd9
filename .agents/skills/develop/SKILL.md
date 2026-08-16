---
name: develop
description: Autonomously select, claim, implement, evaluate, and merge one goxsd9 work packet.
---

# Develop

Complete packet; do not stop at planning/PR.

## Control plane

Root owns claim/lease, decomposition/handoffs, push/audit/challenge/merge.
It does not repeat delegated research/source inspection/implementation/test
diagnosis. Branch/worktree, issue/PR, files, and handoffs are shared memory;
transcript is not.

Every child uses its exact role from `.codex/agents/`, `fork_turns: "none"`,
and task-local context. Scribe and Mason are the
default fresh read-only consultations; omit one only for a mechanical
exemption recorded in the PR. Smith is the default sole source writer and owns
routine tests and remediation. Root writing requires a narrow, demonstrably
mechanical exemption recorded in the handoff. Curator is fresh per managed
document head; Examiner is fresh and challenge-bound per review round.

Every child handoff MUST be concise and state decisions, evidence locations, risks, required
next actions; Smith names changed paths/tests. Preserve
Curator/Examiner JSON byte-for-byte.

## Protocol

1. From the coordination checkout, read `AGENTS.md`, then run `go tool
   workflowctl doctor`, `go tool workflowctl sync`, and `go tool workflowctl pick`.
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
   For each managed-document head, give exact audit, diff, paths, charters,
   and head to a fresh read-only Curator. It checks placement, relevance,
   duplication, historical narration, and replacement; deletion is not
   improvement. Preserve existing JSON contract (`runID`, `head`, `verdict`,
   `summary`, `findings`); `revise` findings add `path`, `reason`, and
   `requiredChange`. Repeat audit and Curator after every remediation push; a
   managed-document head never qualifies for exemption.
10. Run `go tool workflowctl evaluation challenge PR`. Give only its challenge,
    issue/PR state, tests, exact audit, Curator result or
    exemption, attestation shape, and eval rubric to a fresh read-only Examiner
    using Luna at maximum effort. Examiner inspects source, reruns the audit,
    rejects stale/missing Curator evidence, and returns the existing
    `goxsd9/examiner-attestation/v1` JSON. Copy it byte-for-byte outside the
    repository; pass: `{"schema":"goxsd9/examiner-attestation/v1","challenge":"run-...","evaluator":"Examiner","runID":"fresh-agent-task-id","pullRequest":11,"head":"exact-head-sha","verdict":"pass","summary":"No blocking findings.","findings":[]}`. Failure fields: location/impact/requiredCorrection. Record with `workflowctl evaluation record PR
    --attestation-file FILE`; never choose
    or rewrite verdict. On failure, Smith fixes every finding, then check,
    push, then fresh Curator for managed changes, challenge, and spawn a
    fresh Examiner. After three failed rounds,
    mark issue `needs-human` and hand off evidence.
11. On a matching-head pass, write a separate plain-text summary outside the
    repository for future development, backlog, and retrospective workflows;
    cover problem, outcome, rationale, and consequential decisions.
    Keep workflow evidence/metadata in existing records; never copy/parse PR
    Markdown into the squash body. Omit status, commands, and review metadata.
    Run
    `go tool workflowctl pr finish PR --summary-file FILE`, which verifies the
    artifact, claim, checks, evaluation, and head before SHA-bound REST squash
    merge. If draft-to-ready GraphQL fails, close preserved draft, make the
    identical-head ready REST replacement, repeat challenge/Examiner, then SHA-bound REST squash-merge it.

## Waiting and pilot

Waits are logical barriers. Poll/status timeouts are observational only: never
narrow, interrupt, pressure, spawn a second writer, or duplicate work after
polls. Keep waiting while child activity continues and its deadline permits
renewal; do not wake or poll solely to renew. Follow up only for
incomplete/ambiguous handoffs or explicit bounded input. Interrupt/recover only
for explicit failure/cancellation, invalid scope, or inability to renew. Timing
is guidance, not an OpenAI runtime guarantee.

For three packets (mechanical, specification-heavy, remediation), record
aggregate root compactions, peak context, output volume, elapsed time, Examiner
rounds/verdict, and quality for diagnostics, tests, docs, and review. Target
zero normal-packet compactions and under 50% effective root context before review
as optimization signals only; quality must not regress. Never require
undocumented `~/.codex/sessions` or CI/merge telemetry.

## Failure behavior

- Retry transient operations with bounded backoff. Three failed recoveries make
  a located, actionable `needs-human` handoff.
- Leave incomplete branches/worktrees recoverable. Never force-push over an
  active claim or bypass a required check.
- Continue unrelated ready work only in a later invocation; do not backlog-loop.
