// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs, builds
// supported schema-level components and simple-type restrictions, and exposes
// deterministic queries and walks. A missing schema version defaults to XSD
// 1.1. Chameleon includes, redefine/override/defaultOpenContent, assertions,
// and unsupported datatype facets return explicit unsupported diagnostics.
// Paths and URLs are never opened by this package. Parsing drains and closes
// the root and every resolved source.
//
// XML instance validation and Go code generation remain under construction.
package goxsd9
