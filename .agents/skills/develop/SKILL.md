---
name: develop
description: Autonomously select, claim, implement, evaluate, merge one goxsd9 packet.
---

# Develop

## Control plane

Root owns claim/decomposition/lifecycle; do not repeat delegated research,
source inspection, implementation, or test diagnosis absent ambiguity.

Children use exact `.codex/agents/` roles, `fork_turns: "none"`, task-local context.
Scribe/Mason default fresh read-only; exemption requires recorded narrow mechanical
reason. Smith writes source/tests/remediation; root writing needs the same exemption.
Curator is fresh per-head; Examiner fresh/challenge-bound.

Handoffs state decisions, evidence locations, risks, next actions; Smith
names changed paths/tests. Preserve Curator/Examiner JSON.

## Protocol

1. From coordination, read `AGENTS.md`; run `go tool workflowctl doctor`.
   Requires canonical clean `main` equal to fetched `origin/main` plus recursive
   pins; repair stale launches with `base-sync`; rerun doctor, `sync`, `pick`.
   Once clean, set `RUN_EVIDENCE_DIR="$(mktemp -d /var/tmp/goxsd9-develop-evidence.XXXXXXXX)"`.
   Reuse it for body, JSON, coverage, attestation, summary, and handoff paths.
   Preserve dirty files; resolve ownership, never delete/stash.
2. Claim via `go tool workflowctl claim acquire ISSUE`. If lost, do not edit,
   push, reuse, or change Project; ask workflowctl for another eligible issue.
   Never backlog-loop or widen.
3. Read issue, `README.md`, `ARCHITECTURE.md`, `PLAN.md` phase, decisions; claim
   at most one companion for implementation/proof.
4. Give Scribe specification and Mason architecture questions, context, handoff contract.
5. Decompose packet; give Smith issue contract, files, affected phase axes, and
   expected evidence. Mark unaffected axes N/A with rationale; never widen.
   Smith implements/tests/fixes; reports paths/tests. Follow `AGENTS.md`;
   mechanize. Unfinished boundaries need unsupported feature ID, `Loc`, and
   versioned SpecRef; issue actionable discoveries, not TODOs.
6. Normally renew at boundaries/before pushes with `go tool workflowctl claim
   renew`; recovered integration needs checked `pr open` first (see
   `docs/operations.md`). Never poll.
7. Run `go tool workflowctl check`; fix failures and update docs.
8. Commit/push using `AGENTS.md`; open the initial draft PR from that head with
   `go tool workflowctl pr open ISSUE --title TITLE --body-file FILE`, including
   outcome, consultation, verification, conformance, and packet issues.
9. After PR pushes establish `PR_NUMBER`:
   `PR_NUMBER="$(gh pr view --json number --jq '.number')"`; set exact REST
   `BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"`.
   Save `develop-signals --base "$BASE_SHA" --format json` and `docs audit
   --base "$BASE_SHA" --format json` before evidence update; report JSON
   coverage deltas/targets. Policy
   fuzz follows changed boundaries; validate optional repeatable
   `--additional-fuzz PACKAGE:TARGET` at head. Request bounded offline
   single-worker corpus replay. Unlock claim: name exact construct/public
   observable; same fixture at REST base (unsupported/assertion fails) and head
   (passes). If API changed, use stable observable/diagnostic or mark differential
   unmeasured. W3C claim needs executed edition/set/case; otherwise `not-measured`.
   A fragment alone is a Go regression.
   `no-relevant-target` is valid; fuzz is health, not conformance.
   Evidence status: `pending`/`evidence-ready`. Before evidence update,
   challenge, or finish, workflowctl matches exact REST base/head to local
   commits and recomputes v2 signals/policy. Managed changes OR
   triggers require read-only Curator with exact head/runID/pass/no-findings
   before evidence update/challenge/finish/challenge-history convergence. Canonical PR block
   carries sole Examiner-required audit/Curator JSON; give by value or point to
   block. Paths optional, never sole evidence; add no path fields
   or duplicate review state. Reject omitted/stale/forged/unsorted/duplicate/mismatched
   triggers before evidence update, challenge, finish, and challenge-history
   convergence. Classify exact changed paths, never prose: product source triggers;
   `_test.go`, `testdata/`, `evals/`,
   `internal/workflowctl/`, and `cmd/workflowctl/` alone have no trigger and
   retain exact audited `not-required` Curator. Legacy omissions allowed only
   for exact fresh no-trigger diff. Repeat after remediation.
10. Before every challenge, reconcile full PR body with head/evidence/implementation, including
    historical claims; preserve Examiner identity. After edits rerun exact-base
    evidence/audit and fresh Curator when applicable. Same-head retries retain
    newer Curator JSON bytes; recompute evidence/body, then challenge anew.
    Head/runID cannot prove round freshness; binding does not prove prose.
    Run `go tool workflowctl evaluation challenge PR`; give fresh read-only
    Examiner canonical audit/Curator JSON, challenge/state/tests/rubric. Examiner
    inspects source/audit, rejects stale Curator, returns exact
    `goxsd9/examiner-attestation/v1` JSON with located corrective findings.
    Copy bytes unchanged into the run directory; record via `go tool workflowctl
    evaluation record PR --attestation-file FILE`. Never choose verdict. On fail,
    Smith fixes/checks/pushes; repeat Curator/challenge/Examiner. Three
    authenticated fails mean needs-human; transport failures remain retryable.
11. On matching-head pass, write run-directory squash summary:
    problem, outcome, rationale, decisions/invariants; omit metadata/PR Markdown.
    `go tool workflowctl pr finish PR --summary-file FILE` verifies, SHA-bound
    REST merges/converges, and cleans proven refs/worktrees. On cleanup failure
    preserve artifacts; run idempotent `go tool workflowctl
    pr recover PR`. Use `claim prune ISSUE` only with merged proof. Draft
    replacement needs fresh challenge/Examiner on identical-head ready REST PR.
## Waiting and pilot

`running` without error stays healthy across timeouts/compactions. Keep one
agent while lease valid/renewable; elapsed time never warrants interruption,
pressure, narrowing, spawning, duplicate work, or needs-human. Terminal requires explicit failure,
cancellation, invalid scope, or lost lease. Follow up only incomplete handoffs;
renew at durable boundaries, never solely on a timer.

For three packets (mechanical, specification-heavy, remediation), record aggregate
root compactions, peak context, output volume, elapsed time, Examiner rounds/verdict,
and separate diagnostics/test/docs/review quality. Zero normal-packet compactions
and under 50% effective root context before review are optimization signals, never
gates. Quality must not regress; require no sessions or telemetry.
## Failure behavior

- Dirty no-PR: use `claim resume-state` and `docs/operations.md`.
  Unpublished descendants require `--unpublished-local-head`, then `--integrate`.
- Preserve worktrees; never force-push/bypass checks. One bounded reselection;
  no widening.
