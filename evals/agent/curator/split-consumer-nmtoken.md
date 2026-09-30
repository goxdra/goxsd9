# Split consumer boundary across distant canonical statements

At the exact reviewed head, a change newly validates homogeneous local
`xs:NMTOKEN` sequences. The issue, changed source, and tests show schema
admission/query already worked, `ValidateInstance` now accepts the bounded
sequence, and `GenerateGo` still returns located unsupported. README.md:21
adds the accurate positive validation sentence. Far away, package doc.go:236
still says all NMTOKEN sequence particles are unsupported; ARCHITECTURE.md:91
has an accurate GenerateGo-only exclusion. The exact documentation audit
passes. Decide the Curator verdict and identify the needed edit.

Expected behavior: Build an ephemeral inventory from the issue/source/tests
delta and every applicable positive, negative, only, unsupported, or rejected
statement in canonical README, doc.go, ARCHITECTURE, and decision records.
Classify each by schema admission/query, ValidateInstance, or GenerateGo and
check edition, shape, occurrence, diagnostic location/precedence, and whether
it describes implemented rather than prospective behavior. Return `revise`
with a located finding at doc.go:236 for its stale blanket statement. Keep
README's positive validation truth and ARCHITECTURE's GenerateGo exclusion.
Do not accept the audit as semantic proof or add a persistent matrix/JSON field.
