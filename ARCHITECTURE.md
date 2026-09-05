# Architecture

## Boundaries

goxsd9 exposes schema parsing, immutable queries/walks, XML validation, and Go
generation. The schema model is the leaf dependency for validation and generation
and has no validator/generator caches.

Runtime implementation uses only standard-library facilities; development
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

Each phase consumes a complete prior result and produces a new one. Local
construction may append to unexported slices or populate lookup tables, but
completed components are never mutated or backpatched. Document identities are
interned before discovery. Repeated includes/imports reuse that identity, so
cycles do not recurse. Acyclic dependencies are processed in stable topological
order.

Maps support lookup, but ordered slices are primary for observable walks and
output. Every fallback ordering uses explicit stable keys.

## Input and resolution

The entrypoint is `ParseSchema(root ResolvedSource, resolver Resolver)`.
The caller creates root `ResolvedSource` with `NewResolvedSource`; the
resolver creates referenced sources and supplies resolution policy. Parsing
closes root and every resolver-supplied stream; it drains and decodes only
unseen identities. Repeated/cyclic identities are closed without decoding.

```go
type Resolver interface {
    Resolve(
        ctx context.Context,
        namespaceURN string,
        schemaLocation string,
    ) (ResolvedSource, error)
}
```

Each result carries an opaque source identity, reader-closer, and child context.
Resolvers can store typed private base-location state there. Discovery passes
parent context to each FIFO call and preserves returned child context for nested
references. Source identities and lexical schema locations remain opaque; the
parser never interprets paths, opens files, or performs network requests.
Resolver calls are sequential.

The decoder captures one-based line and Unicode-code-point columns
while streaming. Syntax nodes and final components retain `Loc` values, not
source bytes or excerpts.

## Diagnostics

Structured diagnostics are deterministic and classify failures as:

- invalid schema or instance input;
- unsupported specification behavior;
- source resolution failure; or
- internal invariant failure.

Each diagnostic has a stable code, primary `Loc`, optional related locations,
and an applicable specification reference. Causes survive package boundaries.
Error-level diagnostics prevent a schema from being returned.

Unsupported features have stable identifiers. Conformance reports aggregate
them to show which implementation work unlocks the most tests.

## Schema model

Raw XSD syntax is internal. The public model contains immutable schema
components with direct `Loc`; queries use component names and identities.
Walks preserve document-discovery and lexical declaration order; unordered sets
use documented stable sorting.

Schema skeleton exposes `Schema`, `SchemaDocument`, `Component`,
`ComponentID`, and expanded `QName`. Documents follow identity-discovery order
(root first, then resolver queue); named declarations follow lexical order.
`Components`, `Documents`, `Find`, and `Walk` return copies. Component IDs combine
source identity with one-based declaration ordinals; lookup maps never define
observable order. Local particles use a scoped model with component facts and
indexes; validator/generator state is on demand.

Model stores facts; primitive status follows type relations. Global `xs:boolean` and atomic
`xs:string` elements retain `DeclaredType`; named/anonymous restrictions expose immutable
boolean-kind and string-enumeration facts. Built-ins lack synthetic IDs.

Named complex types: direct particles; bounded openAttrs restrictions. Direct sequence/choice: local built-in xs:boolean, named boolean-restriction,
integer/decimal; exact finite/unbounded ranges; 0/0 absent. XSD 1.1 choices: precisionDecimal requires default choice/mapped-alternative occurrences; non-precision
ranges may retain non-default/queryable. Local string particles/
Boolean facets, anonymous/nested/broader particles unsupported. Direct reference
particles retain immutable/queryable target facts.
Supported named direct sequence/
choice types expose attribute-free anyAttribute under XSD 1.0, XSD 1.1,
Compatibility; omitted attributes effectively ##any/strict; explicit ##other/lax
remains supported. Wildcard element locations retained; omitted default-attribute
locations zero. Other wildcard/attribute forms; wildcard-bearing validation/
generation unsupported.

## Datatypes

Lexical parsing and values are separate. Context-sensitive values such as QName
retain namespace context.

The strict datatype library implements XSD string enumeration plus lossless
integer/decimal/boolean/precisionDecimal mappings with arbitrary-precision numeric
forms. PrecisionDecimal exposes exact finite/special values and applicable facets; immutable
schema components retain effective facets when named under Compatibility or
Strict11. It remains optional and implementation-defined, not a mandatory XSD 1.1 claim.
Boolean whitespace collapse is datatype behavior; boolean facets unsupported. Temporal
distinctions and broader value spaces remain staged and report unsupported behavior.

## Validation and code generation

`ValidateInstance` supports text-only built-in/named scalar `boolean`/`integer`/
`decimal`/`precisionDecimal` globals and direct named-complex choices with all-default
local scalar alternatives or all-default references to global integer/decimal elements.
Mixed local/reference choices and non-default occurrences remain unsupported/query-only.
Reference targets use immutable `TargetID`/`Lookup`; boolean/`precisionDecimal`
targets remain unsupported. Named types use `TypeID`/`Lookup`; boolean diagnostics use
policy; numeric built-ins retain compatibility/default behavior. Sequences/repetition remain
unsupported. Non-default `precisionDecimal` choice/alternative ranges mapped to a particle
are schema-unsupported. String globals/restrictions, local boolean/string particles,
attributes, and broader structures remain unsupported for validation; locations are primary.

Generation emits deterministic choice switches, global booleans, and default-bounded direct
integer/decimal sequence structs; direct-reference code generation remains unsupported;
string, boolean facets, and local boolean/string particles remain unsupported.

## Conformance

The W3C XSD test suite is pinned as a submodule. Catalog status is preserved
independently from execution outcome: submitted, accepted, stable, queried,
disputed-test, and disputed-spec are not collapsed. The harness separately
reports pass, conformance failure, unsupported, resolution failure, and
internal failure. Queried, disputed-test, and disputed-spec cases remain
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
