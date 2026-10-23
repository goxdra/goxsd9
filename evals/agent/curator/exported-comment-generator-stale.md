# Unchanged exported generator comment after choice support expands

At the exact reviewed head, an issue and changed generation source add
default-occurrence direct choices of built-in `xs:byte` locals to `GenerateGo`.
Public tests show `ParseSchema` returns a non-nil schema with ordered child
names, exact bounds, and source `Loc`s; `GenerateGo` returns non-nil,
compiling Go with a choice type switch and no diagnostic. Other occurrences
still yield a located unsupported diagnostic and nil output. README.md,
doc.go, ARCHITECTURE.md, and a decision record accurately describe the new
generation shape. An accurate doc.go:210 exclusion says generation still
rejects direct `xs:all`. The exact documentation audit passes. The unchanged
exported function comment at codegen.go:3 still says `GenerateGo` supports
only Boolean/integer/decimal direct choices. No diff hunk mentions codegen.go.

Expected behavior: Curator inventories codegen.go:3 even though it was
unchanged, classifies its `only` claim as generation, and returns `revise`
with a located finding requiring the default-occurrence `xs:byte` choice
exception. It keeps the accurate `xs:all` exclusion and other-occurrence
limit. The passing audit and accurate Markdown do not justify a pass while
the exported function comment conflicts with public output behavior.
