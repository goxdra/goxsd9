# Compressed support claim leaks a qualifier

At the exact reviewed head, the issue contract, source, and public query tests
show named direct-group owners admit direct built-in `xs:language` and
`xs:anyURI` attribute uses. They do not admit `xs:long` there, whether built-in
or named. Named direct-sequence and grouped-extension owners admit built-in and
supported named effective atomic `xs:long` uses. These uses are query-only:
`ValidateInstance` and `GenerateGo` reject them. Compatibility, Strict10, and
Strict11 have the same admission boundary. The exact documentation audit passes.

README.md:12 says: "Empty/choice/sequence/all and named direct-sequence/grouped
extensions: built-in/named facet-free `xs:language`/`xs:anyURI` and effective
`xs:long`; named direct-group owners: built-ins." Package doc.go:354-375 and
ARCHITECTURE.md:82-85 correctly separate the direct-group language/anyURI case
from long; the relevant decision record also preserves the grouped-extension
long boundary. No other canonical statement is stale. Decide the Curator
verdict and identify the needed correction.

Expected behavior: Curator expands the README claim into datatype, owner,
built-in/named, policy, and schema-query/consumer combinations before deciding.
The trailing "built-ins" phrase implies built-in long support on named
direct-group owners, contrary to the exact-head behavior. Return `revise` with
a finding at README.md:12 that separates direct-group built-in language/anyURI
from long and states that direct-group long is unsupported; preserve the
supported long owners, query-only consumer boundary, and accurate statements
elsewhere. Do not treat the passing audit or exact-head binding as semantic
proof, demand a persistent support table, or add a JSON evidence field.
