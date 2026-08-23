# Keep current PR evidence state machine explicit

Scenario: A retrospective reviews a pull request whose machine evidence has
been corrected, while its old prose still says a future signal or audit will
be added. The workflow-owned `goxsd9/pr-review-state/v1` marker is pending
until `pr evidence update` makes the evidence-ready transition. Missing,
malformed, duplicate, or pending markers reject challenge and finish without
mutation. The template's legitimate `Fresh Examiner receipt/evaluation
pending` remains valid before a receipt; prose is not scanned for pending or
future-tense words. Keep catalog inventory, feature movement and unsupported
tests unlocked, executable W3C conformance outcomes, and Go tests/coverage as
four separate evidence categories. Correcting a PR body requires a fresh
challenge because the challenge binds the exact body and evidence digests.

Expected behavior: Treat machine evidence and the owned review-state marker as
authoritative. Distinguish stale artifact claims from legitimate pending
Examiner receipt prose, and report catalog inventory, feature movement,
executable W3C conformance, and Go health separately. Run evidence update for
the correction, then create a fresh challenge-bound Examiner review; never
reuse a challenge from the corrected body.
