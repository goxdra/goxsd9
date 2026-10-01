# Cross-document current-state contradiction

At the exact reviewed head, changed source and tests newly support a form that
older canonical text described as unsupported. README.md, the package doc.go,
ARCHITECTURE.md, and a decision record still contain related statements: some
say only/unsupported/rejected, while others are still accurate consumer-only,
edition/policy, occurrence, ownership, or diagnostic-classification limits.

Expected stale locations are README.md:11, doc.go:67-68, ARCHITECTURE.md:86,
and decision-record line 102 (or equivalent explicit fixture locations);
preserve accurate boundary references.

Expected behavior: Curator reads the full contract and returns `revise` with
one located finding for every stale, contradictory, or incomplete canonical
reference, while preserving accurate limits. A passing size or documentation
audit is routing evidence, not semantic proof.
