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
4. Give Scribe specification and Mason architecture questions, context,
   handoff contract.
5. Decompose packet; give Smith contract, files, and evidence. For affected
   phase boundaries, Smith's handoff matrix covers only affected sibling axes
   (edition/policy; named/anonymous/inline/ref shape; graph visibility/cycles;
   supported/invalid/explicit unsupported; location/order/provenance), marking
   N/A with rationale. It is handoff-only and cannot widen packet. Smith
   implements, tests, fixes failures. Follow `AGENTS.md`; mechanize. Unfinished
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
   corpus replay separately (bounded, offline, single-worker). JSON deltas/
   targets and `no-relevant-target`/`not-measured` are valid; fuzz is health,
   not conformance. Before evidence update, challenge, or finish, workflowctl
   resolves exact REST base/head, matches local commits, recomputes v2 signals/
   policy; managed docs/source triggers need exact-head read-only Curator pass
   (runID/pass/no-findings). Reject omitted/stale/forged/unsorted/duplicate/
   mismatched triggers before update/challenge/finish/convergence. Classify
   exact changed paths only; never scan prose/infer behavior. Legacy omission
   is valid only on exact fresh no-trigger diff. Repeat remediation.
10. Before challenging, reread the exact full PR body against current
    head/evidence/implementation; correct stale freeform claims (including
    historical issue-class claims) without normalizing Examiner
    identity. After any body edit, rerun exact-base evidence and documentation
    audit plus fresh Curator review when applicable, then obtain a fresh
    body-bound challenge and Examiner attestation. Machine binding proves
    identity, not prose truth; never reuse a challenge or stale evidence.
    Run `go tool workflowctl evaluation challenge PR`; give challenge, PR state,
    tests, audit, Curator result, attestation shape, rubric to fresh
    read-only, challenge-bound Examiner. Examiner inspects source, reruns audit, rejects
    stale/missing Curator evidence, returns exact
    `goxsd9/examiner-attestation/v1` JSON; failure findings require location,
    impact, and requiredCorrection. Copy it byte-for-byte outside repository;
    record with `go tool workflowctl evaluation record PR --attestation-file FILE`;
    never choose/rewrite verdict. On failure Smith fixes, checks, pushes, and
    repeats Curator/challenge/Examiner. Three failed rounds mark `needs-human`;
    hand off evidence.
11. On matching-head pass, write a separate plain-text summary outside
    repository for future development/backlog/retrospective; cover
    problem/outcome/rationale/consequential decisions/invariants and omit
    metadata. Do not copy/parse PR Markdown into the squash body; keep metadata
    in records. Use `go tool workflowctl pr finish PR --summary-file FILE`, which
    verifies the packet. Finishing uses SHA-bound REST squash, not GraphQL; it
    converges canonical base, cleans exact refs/claim worktrees, and proves
    expected-SHA branches via immutable base/head/closure/body metadata. If
    convergence/cleanup fails after merge, preserve artifacts and run idempotent
    `go tool workflowctl pr recover PR` (SHA-bound REST, GraphQL-independent).
    Use `claim prune ISSUE` only with merged proof. Draft replacement closes the
    draft, creates an identical-head ready REST PR, and requires fresh
    challenge/Examiner.

## Waiting and pilot

Waits are logical barriers: continue while healthy work and lease renewal
permit; never narrow/pressure/spawn/duplicate. Interrupt only for explicit
failure, cancellation, invalid scope, or lost lease. Follow up only incomplete
handoffs/bounded input; timing is guidance, not a runtime guarantee.

For three packets (mechanical, specification-heavy, remediation), record
aggregate root compactions, peak context, output volume, elapsed time, Examiner
rounds/verdict, and quality across diagnostics, tests, docs, and review. Zero
normal-packet compactions and under 50% effective context before review are
optimization signals, never gates; quality must not regress. Never require
sessions or telemetry.
## Failure behavior

- Three failures require `go tool workflowctl handoff ISSUE --body-file FILE --needs-human`:
  validate body, OPEN/Project, label, Backlog, comment last; never infer
  Markdown/challenges without receipts.
- Preserve incomplete worktrees; never force-push claim or bypass checks. After
  one bounded reselection, do not backlog-loop or widen scope.
