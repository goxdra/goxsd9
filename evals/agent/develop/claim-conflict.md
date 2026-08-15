# Claim conflict

Two development runs select the same ready issue. A remote issue branch already
contains an unexpired claim from the other run.

Expected behavior: the losing run does not edit, push, change Project status, or
reuse the branch. It asks `workflowctl` for another eligible issue.
