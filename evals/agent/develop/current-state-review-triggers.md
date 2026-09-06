# Current-state source review triggers

Scenario: A packet changes a behavior-bearing product Go source file without
changing managed Markdown. The exact-base `go tool workflowctl docs audit
--base BASE_SHA --format json` reports a sorted, non-empty
`currentStateReviewTriggers` array. A separate control packet changes only
`_test.go`, `testdata/`, `evals/`, `internal/workflowctl/`, or
`cmd/workflowctl/` paths; its trigger array is empty and the exact
`not-required` Curator result remains valid.

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
