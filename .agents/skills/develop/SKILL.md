---
name: develop
description: Autonomously select, claim, implement, evaluate, and merge one coherent goxsd9 work packet. Use for scheduled development runs and direct requests to advance ready repository work.
---

# Develop

Complete one coherent work packet. Do not stop after planning or opening a PR.

## Protocol

1. From the clean coordination checkout, read `AGENTS.md`, then run:

   ```sh
   go tool workflowctl doctor
   go tool workflowctl sync
   go tool workflowctl pick
   ```

2. Claim the selected primary issue with `go tool workflowctl claim acquire
   ISSUE`. If the atomic claim loses, pick again. Change into the absolute
   worktree path printed by the command.
3. Read the complete issue, `ARCHITECTURE.md`, the current `PLAN.md` phase, and
   relevant decisions. A packet may contain at most one companion issue. Claim
   it before inclusion and include it only when it shares the implementation
   surface or validation proof.
4. Consult Scribe for specification semantics and Mason for architecture by
   default. Spawn them read-only with only task-local context. A mechanical
   change may omit one or both; record the concrete exemption in the PR.
5. Implement the smallest complete outcome. Follow every invariant in
   `AGENTS.md`. Mechanize a repeated or error-prone step and test the mechanism.
   At an unfinished specification boundary, return an unsupported diagnostic
   with stable feature ID, primary `Loc`, and versioned specification reference.
   Turn an independently actionable discovery into an issue; finish small
   necessary work in this packet.
6. Renew the claim before and after long operations and before every push:

   ```sh
   go tool workflowctl claim renew
   ```

7. Run `go tool workflowctl check`. Fix every failure. Update current docs and
   comments affected by the change.
8. Commit and push intentionally using the title convention in `AGENTS.md`.
   Open a draft PR through `go tool workflowctl pr open ISSUE --title TITLE
   --body-file FILE`; its title must also follow that convention because it
   becomes the squash commit. The body must describe the outcome, consultations
   or exemptions, verification, conformance effect, and close every issue in
   the packet.
9. On the final pushed head, run `go tool workflowctl docs audit --base
   origin/main`. If it reports a managed-document change, spawn Curator in a
   fresh read-only context with the issue, exact audit, diff, changed documents,
   and their charters. Curator judges placement, current relevance, duplication,
   historical narration, and replacement; deletion alone is not improvement.
   It returns only `{"runID":"...","head":"...","verdict":"pass",
   "summary":"...","findings":[]}`. A `revise` finding has `path`, `reason`,
   and `requiredChange`; a pass has no findings.
   Fix every finding and repeat checks, push, audit, and Curator on the new head.
   Record the final run ID, head, verdict, and concise outcome in the PR body;
   record a managed-document-unchanged exemption when no review is required.
10. Run `go tool workflowctl evaluation challenge PR`, then spawn Examiner using
   Luna at maximum effort in a new read-only context with no development
   transcript. Give it only the returned challenge, issue contract, PR,
   repository state, test results, exact documentation audit, Curator result or
   exemption, attestation shape below, and eval rubric. Examiner independently
   reruns the audit and rejects missing, stale, or ignored Curator evidence.
   Examiner must inspect source, must not edit, and must return only its JSON
   attestation. `runID` identifies that fresh agent task. A pass has no findings;
   a failure has one object per blocking finding.

   ```json
   {
     "schema": "goxsd9/examiner-attestation/v1",
     "challenge": "run-...",
     "evaluator": "Examiner",
     "runID": "fresh-agent-task-id",
     "pullRequest": 11,
     "head": "exact-head-sha",
     "verdict": "pass",
     "summary": "No blocking findings.",
     "findings": []
   }
   ```

   A failing finding contains `location`, `impact`, and `requiredCorrection`.
11. Copy the Examiner's JSON byte-for-byte to a temporary file outside the
    repository and record it with `go tool workflowctl evaluation record PR
    --attestation-file FILE`. Never choose or alter the verdict in the Smith
    context. On
    failure, fix every finding, re-run checks, push, and spawn a brand-new
    Examiner. After three failed rounds, mark the issue `needs-human`, hand off
    the evidence, and stop this packet.
12. On a matching-head pass, run `go tool workflowctl pr finish PR`. It must
    verify the claim, required checks, evaluation receipt, and head SHA before
    squash-merging through REST and synchronizing the Project. If GitHub cannot
    mark a draft ready, `workflowctl` closes it, opens an identical-head ready
    replacement through REST, and returns that PR number. Run steps 10–12 on
    the replacement without changing the head. Finish it only through the
    SHA-bound REST squash merge; never make finalization depend on GraphQL.

## Failure behavior

- Retry transient operations with bounded backoff. Three failed recovery
  attempts create a located, actionable `needs-human` handoff.
- Leave an incomplete branch and worktree recoverable. Never force-push over an
  active claim or bypass a required check.
- Continue unrelated ready work only in a later invocation. Do not enter an
  unrestricted backlog loop.
- Use body files for GitHub Markdown; never encode newlines in an argument.
