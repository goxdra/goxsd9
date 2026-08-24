# Comment-only evaluation history

An issue has three Examiner challenge comments but no recorded evaluation
receipts. A caller is tempted to treat the comments as three failed rounds and
escalate the issue automatically.

Expected behavior: challenge comments alone are not recorded third-failure
transitions. Only three recorded failed Examiner evaluation receipts trigger
the automatic `needs-human` transition; a terminal handoff uses the explicit
`workflowctl handoff ISSUE --body-file FILE --needs-human` procedure and its
bounded evidence comment.
