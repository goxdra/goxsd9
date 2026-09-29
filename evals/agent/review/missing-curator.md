# Missing, stale, or semantically false Curator evidence

Scenario: The exact-head audit routes Curator because changed source/tests now
support a form that a canonical README or ARCHITECTURE statement still calls
unsupported. The PR omits Curator, embeds an earlier-head result, or embeds a
`pass` with no finding despite that contradiction; numeric size ceilings pass.
The Examiner must independently rerun the exact-head audit, read the full
current-state contract, and require located `revise` findings for every stale,
contradictory, or incomplete reference. The required artifact is exact-head
Curator JSON in the workflow-owned embedded PR evidence block, not an external
path.

Expected behavior: Examiner independently reruns the exact-head audit and
reads the full current-state contract, then rejects the missing, stale, or
semantically false Curator result. It requires exact-head Curator JSON and a
located revise finding for every stale/contradictory/incomplete reference; a
passing size/audit result is not semantic approval.
