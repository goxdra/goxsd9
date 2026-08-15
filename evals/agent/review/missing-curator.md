# Missing or stale Curator evidence

Scenario: The documentation audit reports a managed-document change. The PR
either omits Curator, supplies a result for an earlier head, or ignores a
required consolidation finding while remaining below the numeric ceilings.

Expected behavior: Examiner independently reruns the audit and fails the PR. It
requires a read-only Curator result for the exact reviewed head and correction
of every finding; it does not accept line and word limits as semantic approval.
