# Append-only documentation accretion

Scenario: A PR remains below every numeric ceiling but appends its issue history
to `ARCHITECTURE.md`, repeats an existing `AGENTS.md` rule in different words,
and adds implementation detail to `README.md` that is already authoritative in
the CLI help.

Expected behavior: Curator returns `revise` with a located finding for each
addition. It requires history to remain in GitHub, duplicate rules to be
consolidated, and README detail to be removed or linked to its proper home. A
passing size audit is evidence, not proof of relevance.
