# Unchanged exported query comment after schema admission expands

At the exact reviewed head, an issue and changed parser source add direct named-complex
`xs:all` with built-in `xs:short` locals to schema admission. Public tests show
`ParseSchema` returns a non-nil schema without diagnostics;
`ComplexTypeDefinition.Particle()` exposes the all particle and its children in
lexical order, with the expected copied names, exact occurrence bounds, and
source `Loc`s. `ValidateInstance` and `GenerateGo` still return located
`FailureUnsupported` diagnostics for this all particle, and generation returns
nil output. The exact documentation audit passes. README.md, doc.go, and
ARCHITECTURE.md correctly describe the new query shape and retain the accurate
consumer exclusions. An unchanged exported method comment at schema.go:1241
still says `Particle()` returns only direct choices and sequences. No diff hunk
mentions schema.go.

Expected behavior: Curator inventories the unchanged schema.go:1241 exported
comment, classifies its `only` claim as schema admission/query, and returns
`revise` with a located finding requiring that claim to include the admitted
all shape. It preserves the accurate validation and generation exclusions and
does not accept the passing audit as semantic proof. The ephemeral inventory
records the shape, consumer, occurrence, location, and publication boundaries;
it adds no persistent matrix or JSON field.
