# Architecture

## Boundaries

goxsd9 exposes schema parsing, immutable queries/walks, XML validation, and Go
generation. The schema model is validation/generation's leaf dependency without
validator/generator caches.

Runtime uses only standard-library facilities; development
tooling remains outside the library dependency graph.

## Deterministic phase pipeline

```mermaid
flowchart LR
  A["Root byte stream"] --> B["XML and XSD syntax"]
  B --> C["Document discovery queue"]
  C --> D["Component declarations and identities"]
  D --> E["Reference and derivation ordering"]
  E --> F["Schema constraints and facets"]
  F --> G["Immutable Schema"]
  G --> H["Instance validator"]
  G --> I["Go code generator"]
```

Phases consume complete prior results. Local construction may use unexported
slices/tables; completed components are immutable and never backpatched. Identities
are interned before discovery; repeated includes/imports reuse them,
so cycles do not recurse. Acyclic dependencies use stable topological order.

Maps support lookup; ordered slices define observable walks/output, with
stable fallback keys.

## Input and resolution

Entrypoint: `ParseSchema(root ResolvedSource, resolver Resolver)`. Callers create
root sources with `NewResolvedSource`; resolvers create references and supply
policy. Parsing closes all streams; each identity decodes once; repeats/cycles close
without decoding.

```go
type Resolver interface {
    Resolve(
        ctx context.Context,
        namespaceURN string,
        schemaLocation string,
    ) (ResolvedSource, error)
}
```

Each source carries opaque identity, reader-closer, and child context; resolvers
may store typed private base-location state. Discovery passes parent context FIFO
and preserves child context for nested references. Identities and lexical
locations stay opaque: the parser does not interpret paths, open files, or make
network requests. Resolver calls are sequential.

Streaming decode captures one-based line and Unicode-code-point columns; syntax/final
components retain `Loc`, not source bytes/excerpts.

## Diagnostics

Structured diagnostics are deterministic and classify failures as:

- invalid schema or instance input;
- unsupported specification behavior;
- source resolution failure; or
- internal invariant failure.

Diagnostics have stable codes, primary `Loc`, optional related locations, and
specification references; causes survive boundaries, and error-level diagnostics
prevent schema return.

Unsupported features have stable identifiers; conformance reports aggregate them
for unlock ranking.

## Schema model

Raw XSD syntax is internal. Immutable model retains component `Loc`; queries use names/identities.
Walks preserve document-discovery/lexical declaration order; unordered sets use stable sorting.

Schema skeleton exposes `Schema`, `SchemaDocument`, `Component`, `ComponentID`, and expanded `QName`.
Documents follow identity-discovery order (root, queue); named declarations follow lexical order.
`Components`, `Documents`, `Find`, and `Walk` return copies. IDs combine source identity/one-based
declaration ordinals; lookup maps never define observable order. Local particles use scoped component
facts/indexes; validator/generator state is on-demand.

Primitive status: Global scalars retain `DeclaredType`; local explicit built-in/supported-named
token/NMTOKEN refs retain references in supported direct shapes;
named/anonymous restrictions retain immutable boolean-kind/string-enumeration/string-`whiteSpace`; built-ins lack synthetic IDs.
Built-in/named integer/decimal attrs retain immutable value-constraint-facts: kind=default/fixed, normalized-lexical-form,
exact-typed-value, source-location. Named global complex types accept unqualified `mixed="false|0"`; omitted=element-only
(unretained/unconsumed); `mixed="true|1"` unsupported. Malformed/contradictory XSD 1.1 forms; anonymous global complex/other shapes unsupported.
Typed global attributes expose immutable `AttributeDeclaration.IsInheritable()`: `inheritable` omitted=false;
Compatibility/Strict11 accept it, Strict10 reports a mismatch; untyped/inline forms unsupported.
`defaultAttributesApply="true|false|1|0"` is restricted to named globals in XSD 1.1/Compatibility without schema-level
`defaultAttributes`; validated/discarded, no public/validator/generator state. Strict10 reports mismatch.
Root `xpathDefaultNamespace` is inert: Compatibility/Strict11 validate and discard it; malformed values are invalid, Strict10 reports a located mismatch, and XPath constructs remain unsupported.
Schema-level defaults; local non-particle/inline/value/default/fixed/attribute/broader forms and
non-atomic-string/string/boolean/precisionDecimal attrs unsupported.

Complexes: `IsAbstract()` (non-inherited); named global complex types: explicit non-empty `final`; `Final()`: canonical extension-then-restriction order; `FinalLoc()`: source location; XSD 1.0/1.1; Compatibility; `final=extension` or `#all` rejects extension derivation.
Model-group refs/extensions retain IDs/locations; consumers reject. Direct sequence/choice `anyAttribute`: omitted/default `##any`/strict or explicit `##any`/strict/`##other`/lax; locations; omitted=0. Default-effective `xs:any` wildcards retain namespace/process/ranges/locations; `0/0` absent; nonzero rejected. Inert `openContent mode="none"` supports named globals and bounded attribute-free extensions under Compatibility/Strict11; Strict10 reports a located mismatch. Other open-content/restriction/simpleContent/inline/broader derivations unsupported; malformed=invalid.
Model groups expose ordered global-element facts; consumers reject.

## Datatypes

Lexical parsing and values are separate. Context-sensitive values such as QName
retain namespace context.

The datatype library implements XSD string enumeration plus lossless
integer/decimal/boolean/precisionDecimal mappings with arbitrary-precision numeric
forms. PrecisionDecimal exposes exact finite/special values and applicable facets; immutable
schema components retain effective facets when named under Compatibility or
Strict11. It remains optional and implementation-defined, not a mandatory XSD 1.1 claim.
Boolean whitespace collapse is datatype behavior; boolean facets unsupported. Temporal
distinctions and broader value spaces remain staged and report unsupported behavior.

## Validation and code generation

`ValidateInstance` supports root globals with built-in or named `boolean`/`token`/`NMTOKEN`/`integer`/`decimal`/`precisionDecimal` types, plus named
complexes with direct choices/sequences. Choices accept default-occurrence local Boolean/integer/decimal/precisionDecimal elements and
default-occurrence references only to global integer/decimal elements. Homogeneous Boolean/numeric sequences honor finite/unbounded and
above-`uint64` ranges under all policies; mixed sequences remain unsupported. References use `TargetID`; model groups rejected.
`token`/`NMTOKEN` collapse XML whitespace before effective enumeration; NMTOKEN enforces repository XML NameChar policy; raw facts unchanged.
Global Boolean references, local token/NMTOKEN particles, strings, lists/unions, attributes, and structures remain validation-unsupported. Direct `xs:any` is
query-only: nonzero terms are rejected with edition-selected diagnostics; `0/0` absent.

Generation: named/inherited global boolean/integer/decimal/string/token/NMTOKEN scalars, inline anonymous global string/token/NMTOKEN elements, numeric choices,
default all-Boolean choices and default-bounded numeric/all-Boolean sequences; mixed Boolean/numeric sequences,
other choices, and repeated/other references remain unsupported; only default global integer/decimal references generate.

## Conformance

W3C XSD test suite is pinned as a submodule. Catalog status is independent
of execution: submitted, accepted, stable, queried, disputed-test, and disputed-spec
remain distinct. The harness reports pass, conformance failure, unsupported,
resolution failure, and internal failure. These cases remain
visible but affect neither the headline score nor backlog unlock ranking.

Specifications pin XSD 1.0/1.1 schema-for-schemas artifacts by URL and
raw-response digest in a manifest. Tooling converts, indexes, and navigates
artifacts. `xml` is consumed unchanged; `html-cdata-pre` removes the exact
`<pre><![CDATA[`/`]]></pre>` wrapper.
`html-cdata-pre-xsd10-datatypes` removes that wrapper after digest verification,
requires the pinned XSD 1.0 envelope, moves its one post-DTD declaration
through `?>` before the unchanged DTD, and performs complete XML validation
without opening external DTDs. Manifest aliases map schema locations such as
HTTP `xml.xsd` to the pinned HTTPS artifact without
changing parser or resolver semantics.
