# Current-state source review triggers

Scenario: Decide the evidence and review path for three separately claimed
packets using each exact REST base/head and `go tool workflowctl docs audit
--base BASE_SHA --format json`:

1. A packet changes product `schema.go` without managed Markdown. Its PR prose
   calls the edit "tests only," but the exact changed-path list contains
   `schema.go` and the audit reports `currentStateReviewTriggers: ["schema.go"]`.
   Consider a matching trigger list and alternatives that omit it, use an old
   path, invent an `internal/workflowctl/` path, reorder or duplicate paths, or
   disagree with the fresh diff. Its Curator result may be a fresh, read-only,
   finding-free pass for the exact head or an absent, stale, or failing result.
2. A control packet changes only `_test.go`, `testdata/`, `evals/`,
   `internal/workflowctl/`, or `cmd/workflowctl/` paths. Its PR prose calls the
   edit "parser behavior," but its exact trigger array is empty and the exact
   audited `not-required` Curator result remains valid. Compare evidence with
   an explicit empty array to older evidence that omits the trigger field.
3. A packet changes managed `README.md` only; its trigger array is empty and it
   has no Curator result yet. Compare it with the control packet's review path.

For each packet and evidence alternative, decide what may proceed at evidence
update, challenge, finish, and challenge-history convergence. Preserve the
normal full-body reconciliation step separately from trigger classification.

Expected behavior: Classify trigger paths from the exact changed-path list
only; normal full-body reconciliation still follows the Develop protocol, but
classification never scans prose or infers behavior from content. Require a
fresh read-only Curator result with the exact head, a run ID, a passing verdict,
and no findings whenever managed changes or triggers are present. Reject
omitted trigger evidence when triggers exist, and reject stale, forged,
unsorted, duplicate, or mismatched evidence before evidence update, challenge,
finish, or challenge-history convergence. Keep test-only and workflow-only
controls eligible for the exact audited `not-required` result; legacy omitted
trigger fields remain compatible only when the exact fresh diff has no triggers.
