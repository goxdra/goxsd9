# Keep current PR evidence state machine explicit

Scenario: A retrospective reviews a pull request whose machine evidence has
been corrected, while its old prose still says a future signal or audit will
be added. The workflow-owned `goxsd9/pr-review-state/v1` frame has one global
pending marker and three ordered slots until `pr evidence update` makes the
evidence-ready transition. Stale wording from PRs #144, #146, #188, and #189
is opaque prose or an invalid old frame, not a reason to scan or rewrite text.
Missing, malformed, duplicate, reordered, or pending owned markers reject
open, evidence update, challenge, and finish without mutation. The six exact
canonical pending/evidence-ready status lines are the only structural exception:
each may appear only as a full line at the recorded `statusStart/statusEnd` of
its matching slot, with either state accepted there so evidence update can
converge drift; duplicates and wrong-slot copies reject before mutation. The
template's legitimate `Fresh Examiner receipt/evaluation pending` remains
valid outside owned spans; non-exact, quoted, and freeform prose is opaque and
is never scanned or rewritten.
Correcting a PR body requires a fresh challenge because the challenge binds the
exact body and evidence digests.

Expected behavior: Treat machine evidence and the owned review-state marker as
authoritative. Distinguish stale artifact claims from legitimate pending
Examiner receipt prose. Run evidence update for the correction, then create a
fresh challenge-bound Examiner review; never reuse a challenge from the
corrected body.
