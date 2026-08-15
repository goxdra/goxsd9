# Concise current-state replacement

Scenario: A resolver contract changes. The PR replaces the stale architecture
paragraph with a shorter accurate description, removes a duplicated rule, and
leaves the rationale in the existing decision record without narrating the PR.

Expected behavior: Curator returns `pass` with no findings. It does not demand
that deleted history be preserved or reject the change merely because the
document shrank; the relevant current truth remains in its canonical home.
