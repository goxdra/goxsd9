# Separated and evidenced support claims

At the exact reviewed head, the issue contract, source, and public query tests
show named direct-group owners admit direct built-in `xs:language` and
`xs:anyURI` attribute uses, but no built-in or named `xs:long` uses. Named
direct-sequence and grouped-extension owners admit built-in and supported named
effective atomic `xs:long` uses. All these uses are query-only; both
`ValidateInstance` and `GenerateGo` return located unsupported diagnostics.
Compatibility, Strict10, and Strict11 share this admission boundary. The exact
documentation audit passes.

README.md:12 now says: "Empty/choice/sequence/all and named
direct-sequence/grouped extensions admit built-in/named facet-free
`xs:language`/`xs:anyURI` and effective `xs:long`. Named direct-group owners
admit direct built-in `xs:language`/`xs:anyURI` only; `xs:long` is unsupported
there. These uses are query-only; validation and generation reject."
Package doc.go:354-375, ARCHITECTURE.md:82-85, and the relevant decision record
give the same scoped admission and consumer facts. No other canonical claim is
stale, duplicated, or misplaced. Decide the Curator verdict.

Expected behavior: Curator expands the compressed first sentence and the
separate direct-group sentence into datatype, owner, built-in/named, policy,
and schema-query/consumer combinations, reconciles them with exact-head
evidence, and returns `pass` with no findings. The explicit direct-group
exclusion prevents long from inheriting language/anyURI support. Do not demand
broader long support, unrelated prose edits, a persistent support table, or a
new JSON evidence field.
