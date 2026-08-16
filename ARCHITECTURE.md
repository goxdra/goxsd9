# Architecture

## Boundaries

goxsd9 has four user-facing capabilities: schema parsing, immutable schema
queries and walks, XML instance validation, and Go code generation. The schema
model is the leaf dependency: validation and code generation depend on it, but
it contains no validator or generator caches.

The implementation uses the Go standard library. Development-only lint tooling
is an approved exception. Any other dependency requires a human-reviewed issue.

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

Each phase consumes a complete prior result and produces a new result. Local
construction may append to unexported slices or populate lookup tables, but a
completed component is never mutated or backpatched. Document identities are
interned before discovery. Repeated includes and imports reuse that identity,
so document cycles do not recurse. Dependencies required to be acyclic are
processed in stable topological order.

Maps may support lookup, but ordered slices are the primary representation for
observable walks and output. Every fallback ordering uses explicit stable keys.

## Input and resolution

The parser receives an initial `io.ReadCloser` and configuration. It drains that
stream, then asks the configured resolver for each additional byte stream.

```go
type Resolver interface {
    Resolve(
        ctx context.Context,
        namespaceURN string,
        schemaLocation string,
    ) (ResolvedSource, error)
}
```

Each result carries an opaque source identity, a reader-closer, and a child
context. A resolver can store typed private base-location state in that context.
The parser preserves the context but never interprets paths, opens files, or
performs network requests. Resolver calls are sequential.

The decoder captures one-based line and Unicode-code-point column positions as
it streams. Syntax nodes and final components retain `Loc` values, not source
bytes or excerpts.

## Diagnostics

Structured diagnostics are deterministically ordered and classify failures as:

- invalid schema or instance input;
- unsupported specification behavior;
- source resolution failure; or
- internal invariant failure.

Every diagnostic has a stable code, primary `Loc`, optional related locations,
and an applicable specification reference. Errors preserve their causes as
they cross package boundaries. Error-level diagnostics prevent a schema from
being returned.

Unsupported features have stable identifiers. Conformance reports aggregate
those identifiers to show which implementation work unlocks the most tests.

## Schema model

Raw XSD syntax is internal. The public model contains immutable schema
components with direct access to their `Loc`. Query methods use component names
and identities. Walk methods guarantee document-discovery order followed by
lexical declaration order; specification-defined unordered sets use documented
stable sorting.

The schema skeleton exposes `Schema`, `SchemaDocument`, `Component`,
`ComponentID`, and expanded `QName` values. A schema stores documents in
identity-discovery order (the root document first, followed by resolver queue
order) and named schema-level declarations in lexical declaration order within
each document. `Components`, `Documents`, `Find`, and `Walk` preserve that
order; returned slices are copies. Component IDs combine a resolver source
identity with a one-based declaration ordinal, while lookup maps are private
indexes and never define observable order. Local particle components will be
walked through a separate scoped model. The schema stores only component facts
and lookup indexes, leaving validator and code-generator structures to be
calculated on demand.

The model stores fundamental facts, not redundant flags. For example, primitive
status is derived from the type relation rather than stored separately.

Particle alternatives are concrete types so callers and generated code can use
Go type switches for element, sequence, choice, all, wildcard, and future
variants.

## Datatypes

Lexical parsing and value representation are separate. Context-sensitive
lexical values such as QName carry the namespace context needed to construct a
value.

The strict datatype library is the default target. Its current foundation
implements XSD integer and decimal lexical/value mapping with arbitrary
precision and lossless canonical forms. Facets, precision decimal, temporal
distinctions, and the broader value spaces remain staged capabilities and report
unsupported behavior until implemented. The idiomatic library favors Go types
such as `int`, `time.Time`, and `time.Duration`, with documented range or
semantic tradeoffs. Users can supply the same datatype-library interface.

## Validation and code generation

Validation compiles content-model machinery on demand and does not cache it in
`Schema`. It reports instance locations and related schema locations. XPath
support begins with the XSD-required subset and expands toward the full XPath
2.0 dependency set.

Code generation consumes only the public schema model. It produces deterministic
formatted Go, uses type switches for choices, and never depends on map order.

## Conformance

The W3C XSD test suite is pinned as a submodule. Catalog status is preserved
independently from execution outcome: submitted, accepted, stable, queried,
disputed-test, and disputed-spec are not collapsed. The harness separately
reports pass, conformance failure, unsupported, resolution failure, and
internal failure. Queried, disputed-test, and disputed-spec cases remain
visible but affect neither the headline score nor backlog unlock ranking.

Specifications and the distinct XSD 1.0 and 1.1 schema-for-schemas artifacts
are pinned by URL and digest through a versioned manifest. Repository tooling
will download, verify, convert, index, and navigate them while preserving
anchors and examples. Digests cover raw responses. Artifact representations
are explicit: `xml` is consumed as verified, while `html-cdata-pre` removes
only the exact `<pre><![CDATA[` prefix and `]]></pre>` suffix after digest
verification. Manifest aliases map lexical schema locations such as the HTTP
`xml.xsd` import to their pinned HTTPS artifact without changing parser or
resolver semantics.
