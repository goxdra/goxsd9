# Corrected exported comments after support expands

At the exact reviewed head, a source-triggered packet adds direct named-complex
`xs:all` with built-in `xs:short` locals to schema queries and adds
default-occurrence built-in `xs:byte` direct choices to `GenerateGo`.
Public tests show non-nil schemas with copied child names, exact bounds,
lexical order, and source `Loc`s; `ComplexTypeDefinition.Particle()` returns
the all particle; generation emits non-nil compiling Go with a choice type
switch. Unsupported all-particle consumers and non-default choice occurrences
still return located diagnostics, with nil generation output. README.md,
doc.go, ARCHITECTURE.md, and relevant decision records are current. The
exported method comment at schema.go:1241 now includes direct `xs:all`, and
the exported function comment at codegen.go:3 now includes the supported
default-occurrence `xs:byte` choice. The unchanged doc.go:210 exclusion for
generating direct `xs:all` remains accurate. The exact documentation audit
passes, and the ephemeral inventory finds no other relevant stale claims.

Expected behavior: Curator inventories both exported comments and the
canonical documents, separates schema admission/query from generation, and
returns `pass` with no findings. It preserves the accurate direct-`xs:all`
generation exclusion and non-default-occurrence limit. It does not ask for
deletion of those limits or a persistent support matrix.
