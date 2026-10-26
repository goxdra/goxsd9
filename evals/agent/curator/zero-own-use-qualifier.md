# Missing zero-use qualifier in current architecture prose

At the exact reviewed head, source and public tests establish two bounded
named grouped-extension alternatives. The older use-free optional-reference
sequence-base path requires `len(derivedUses)==0`, after valid omitted and
prohibited uses normalize away. Its base has a default 1/1 sequence with one
0/1 global-element reference. Six direct/extension-base tests across
Compatibility, Strict10, and Strict11 add one effective Boolean local use to
the derived group and get XSD3003 at the base reference with no schema. The
separate newer path admits an immediate completed sequence-extension base with
one optional direct local ID; the copied chain has its own final/name uses.
README.md and package doc.go state the older zero-own-use limit. The exact
documentation audit passes.

ARCHITECTURE.md:74-76 literally says: "Named grouped extensions admit use-free
optional-ref sequences or immediate completed sequence-extension bases
(default 1/1 sequence, one 0/1 global-element ref, one optional direct local
ID)." It omits the zero-effective-own-use condition on the first alternative.
An inventory calls the first alternative the "Z" category and accurately
describes source/tests, but does not quote a zero-use condition from this
architecture sentence. Decide the Curator verdict and needed correction.

Expected behavior: Curator returns `revise` with a located finding at
ARCHITECTURE.md:74-76. It must read the literal current-state claim and require
the first use-free sequence-base alternative to say **zero effective own uses**,
while retaining default 1/1 sequence and one 0/1 global-element ref. It must
keep the separate immediate one-optional-direct-local-ID base exception and
its own uses distinct. A category name, tests, source predicate, passing audit,
or correct qualifier in another document cannot silently add words missing
from ARCHITECTURE.md. Do not demand expanded support or a persistent matrix.
