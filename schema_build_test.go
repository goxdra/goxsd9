package goxsd9

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit // Keep the complete ordered component contract in one regression test.
func TestDiscoverSchemaBuildsOrderedImmutableDeclarations(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
	  <xs:include schemaLocation="child.xsd"/>
	<xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="rootElement"/>
  <xs:attribute name="rootAttribute"/>
  <xs:complexType name="rootComplex"/>
  <xs:attributeGroup name="rootAttributes"/>
	  <xs:notation name="rootNotation"/>
</xs:schema>`
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootContents)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{
		"child.xsd": {
			id: "child.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="childElement"/>
</xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:complexType name="otherComplex"/>
</xs:schema>`,
		},
	}}

	schema, err := discoverSchema(root, resolver)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	documents := schema.Documents()
	if got, want := len(documents), 3; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	for index, want := range []struct {
		source          SourceID
		targetNamespace string
	}{
		{source: "root.xsd", targetNamespace: "urn:root"},
		{source: "child.xsd", targetNamespace: "urn:root"},
		{source: "other.xsd", targetNamespace: "urn:other"},
	} {
		if got := documents[index].Source(); got != want.source {
			t.Fatalf("document %d source = %q, want %q", index, got, want.source)
		}
		if got := documents[index].TargetNamespace(); got != want.targetNamespace {
			t.Fatalf("document %d target namespace = %q, want %q", index, got, want.targetNamespace)
		}
	}

	wantComponents := []struct {
		source  SourceID
		ordinal uint64
		kind    ComponentKind
		name    QName
		loc     Loc
	}{
		{source: "root.xsd", ordinal: 1, kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "rootElement"), loc: mustTestLoc(t, "root.xsd", 4, 3)},
		{source: "root.xsd", ordinal: 2, kind: ComponentKindAttributeDeclaration, name: mustTestQName(t, "urn:root", "rootAttribute"), loc: mustTestLoc(t, "root.xsd", 5, 3)},
		{source: "root.xsd", ordinal: 3, kind: ComponentKindComplexTypeDefinition, name: mustTestQName(t, "urn:root", "rootComplex"), loc: mustTestLoc(t, "root.xsd", 6, 3)},
		{source: "root.xsd", ordinal: 4, kind: ComponentKindAttributeGroupDefinition, name: mustTestQName(t, "urn:root", "rootAttributes"), loc: mustTestLoc(t, "root.xsd", 7, 3)},
		{source: "root.xsd", ordinal: 5, kind: ComponentKindNotationDeclaration, name: mustTestQName(t, "urn:root", "rootNotation"), loc: mustTestLoc(t, "root.xsd", 8, 4)},
		{source: "child.xsd", ordinal: 1, kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "childElement"), loc: mustTestLoc(t, "child.xsd", 2, 3)},
		{source: "other.xsd", ordinal: 1, kind: ComponentKindComplexTypeDefinition, name: mustTestQName(t, "urn:other", "otherComplex"), loc: mustTestLoc(t, "other.xsd", 2, 3)},
	}
	components := schema.Components()
	if got, want := len(components), len(wantComponents); got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	for index, want := range wantComponents {
		component := components[index]
		if component.Document() != want.source {
			t.Errorf("component %d source = %q, want %q", index, component.Document(), want.source)
		}
		if component.ID().Ordinal() != want.ordinal {
			t.Errorf("component %d ordinal = %d, want %d", index, component.ID().Ordinal(), want.ordinal)
		}
		if component.Kind() != want.kind {
			t.Errorf("component %d kind = %q, want %q", index, component.Kind(), want.kind)
		}
		if component.Name() != want.name {
			t.Errorf("component %d name = %q, want %q", index, component.Name(), want.name)
		}
		if component.Loc() != want.loc {
			t.Errorf("component %d location = %s, want %s", index, component.Loc(), want.loc)
		}
	}
}

//nolint:gocognit // Exercise discovery order, edge provenance, and identity reuse together.
func TestSchemaDiscoveryRetainsOrderedEdgesAndOneIdentityPerSource(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:include schemaLocation="child.xsd"/>
  <xs:include schemaLocation="child.xsd"/>
</xs:schema>`
	rootReader := &discoveryReader{data: []byte(rootContents)}
	root, err := NewResolvedSource(context.Background(), "root.xsd", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{
		"child.xsd": {
			id:       "child.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:element name="child"/></xs:schema>`,
		},
		"root.xsd": {id: "root.xsd", contents: rootContents},
	}}

	discovery, err := discoverSyntax(root, resolver)
	if err != nil {
		t.Fatalf("discoverSyntax: %v", err)
	}
	if got, want := len(discovery.documents), 2; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	if got, want := len(discovery.edges), 3; got != want {
		t.Fatalf("edge count = %d, want %d", got, want)
	}
	for index, edge := range discovery.edges {
		if edge.source == "" || edge.target == "" {
			t.Fatalf("edge %d has incomplete identity: %#v", index, edge)
		}
		if edge.kind != syntaxReferenceInclude {
			t.Fatalf("edge %d kind = %d, want include", index, edge.kind)
		}
		if edge.hasNamespace {
			t.Fatalf("edge %d unexpectedly has an include namespace", index)
		}
	}
	if got, want := discovery.edges[0].source, SourceID("root.xsd"); got != want {
		t.Fatalf("edge 0 source = %q, want %q", got, want)
	}
	if got, want := discovery.edges[2].source, SourceID("child.xsd"); got != want {
		t.Fatalf("edge 2 source = %q, want %q", got, want)
	}

	schema, err := newSchemaFromDiscovery(discovery)
	if err != nil {
		t.Fatalf("newSchemaFromDiscovery: %v", err)
	}
	if got, want := len(schema.Documents()), 2; got != want {
		t.Fatalf("schema document count = %d, want %d", got, want)
	}
	if got, want := len(schema.Components()), 1; got != want {
		t.Fatalf("schema component count = %d, want %d", got, want)
	}
	if got, want := len(resolver.calls), 3; got != want {
		t.Fatalf("resolver calls = %d, want %d", got, want)
	}
}

func TestSchemaBridgeAcceptsImportCycleAndPreservesNamespacePresence(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:child" schemaLocation="child.xsd"/><xs:element name="root"/></xs:schema>`
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootContents)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:element name="child"/></xs:schema>`},
		"root.xsd":  {id: "root.xsd", contents: rootContents},
	}}

	discovery, err := discoverSyntax(root, resolver)
	if err != nil {
		t.Fatalf("discoverSyntax: %v", err)
	}
	if got, want := len(discovery.documents), 2; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	if got, want := len(discovery.edges), 2; got != want {
		t.Fatalf("edge count = %d, want %d", got, want)
	}
	for index, wantNamespace := range []string{"urn:child", "urn:root"} {
		edge := discovery.edges[index]
		if edge.kind != syntaxReferenceImport || !edge.hasNamespace || edge.namespaceURN != wantNamespace {
			t.Fatalf("edge %d = %#v, want explicit import namespace %q", index, edge, wantNamespace)
		}
	}

	schema, err := newSchemaFromDiscovery(discovery)
	if err != nil {
		t.Fatalf("newSchemaFromDiscovery: %v", err)
	}
	if got, want := len(schema.Components()), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

func TestDiscoverSchemaPreservesResolutionCauseWithoutSchema(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="missing.xsd"/></xs:schema>`
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootContents)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolveErr := errors.New("resolver unavailable")
	resolver := &discoveryResolver{failures: map[string]error{"missing.xsd": resolveErr}}

	schema, err := discoverSchema(root, resolver)
	if err == nil {
		t.Fatal("discoverSchema accepted a resolution failure")
	}
	if !errors.Is(err, resolveErr) {
		t.Fatalf("resolution cause was not preserved: %v", err)
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema after resolution failure")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureResolution || diagnostic.Code() != SourceResolveCode {
		t.Fatalf("resolution diagnostic = %s, want source resolution diagnostic", diagnostic)
	}
}

func TestSchemaBridgeAcceptsNoNamespaceComposition(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="child.xsd"/><xs:element name="root"/></xs:schema>`
	schema, err := discoverTestSchema(t, rootContents, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="child"/></xs:schema>`},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(schema.Components()), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	for index, component := range schema.Components() {
		if got := component.Name().Namespace(); got != "" {
			t.Errorf("component %d namespace = %q, want empty", index, got)
		}
	}
}

func TestSchemaBridgeAcceptsExplicitImportFromNoNamespaceParent(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import namespace="urn:child" schemaLocation="child.xsd"/><xs:element name="root"/></xs:schema>`
	schema, err := discoverTestSchema(t, rootContents, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child"><xs:element name="child"/></xs:schema>`},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	documents := schema.Documents()
	if got, want := len(documents), 2; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	if got := documents[0].TargetNamespace(); got != "" {
		t.Fatalf("root target namespace = %q, want no namespace", got)
	}
	if got, want := documents[1].TargetNamespace(), "urn:child"; got != want {
		t.Fatalf("child target namespace = %q, want %q", got, want)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	for index, want := range []struct {
		source  SourceID
		name    QName
		ordinal uint64
	}{
		{source: "root.xsd", name: mustTestQName(t, "", "root"), ordinal: 1},
		{source: "child.xsd", name: mustTestQName(t, "urn:child", "child"), ordinal: 1},
	} {
		component := components[index]
		if component.Document() != want.source {
			t.Errorf("component %d source = %q, want %q", index, component.Document(), want.source)
		}
		if component.Name() != want.name {
			t.Errorf("component %d name = %q, want %q", index, component.Name(), want.name)
		}
		if component.ID().Ordinal() != want.ordinal {
			t.Errorf("component %d ordinal = %d, want %d", index, component.ID().Ordinal(), want.ordinal)
		}
	}
}

func TestSchemaBridgeAcceptsCompositionAnnotation(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include id=" include1 " schemaLocation="child.xsd"><xs:annotation id=" annotation1 "><xs:documentation source=" docs " xml:lang="en">included schema</xs:documentation><xs:appinfo source=" appinfo ">metadata</xs:appinfo></xs:annotation></xs:include></xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:element name="child"/></xs:schema>`},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(schema.Components()), 1; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

func TestSchemaBridgeAcceptsRootAndDeclarationAnnotations(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:annotation id=" rootAnnotation "><xs:documentation>schema documentation</xs:documentation><xs:appinfo>schema metadata</xs:appinfo></xs:annotation><xs:element name="item"><xs:annotation id=" declarationAnnotation "><xs:documentation>element documentation</xs:documentation></xs:annotation></xs:element></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 1; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	if got, want := components[0].Name(), mustTestQName(t, "urn:root", "item"); got != want {
		t.Fatalf("component name = %q, want %q", got, want)
	}
}

func TestSchemaBridgeRejectsAnnotationAttributesWithoutPartialSchema(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "composition annotation unknown attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"><xs:annotation bogus="1"/></xs:include></xs:schema>`,
		},
		{
			name: "root annotation unknown attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation bogus="1"/></xs:schema>`,
		},
		{
			name: "declaration annotation unknown attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item"><xs:annotation bogus="1"/></xs:element></xs:schema>`,
		},
		{
			name: "appinfo unknown attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation><xs:appinfo bogus="1"/></xs:annotation></xs:schema>`,
		},
		{
			name: "documentation unknown attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation><xs:documentation bogus="1"/></xs:annotation></xs:schema>`,
		},
		{
			name: "annotation malformed id",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation id=" bad:id "/></xs:schema>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid annotation attributes")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic source = %q, want root.xsd", diagnostic.Loc().Source())
			}
		})
	}
}

func TestSchemaBridgeCollapsesSchemaValueWhitespace(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="  urn:root  "><xs:import namespace="  urn:child  " schemaLocation=" child.xsd "/><xs:element name="  item  "/></xs:schema>`
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootContents)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{
		" child.xsd ": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="  urn:child  "><xs:element name="  child  "/></xs:schema>`},
	}}
	schema, err := discoverSchema(root, resolver)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(resolver.calls), 1; got != want {
		t.Fatalf("resolver calls = %d, want %d", got, want)
	}
	if got, want := resolver.calls[0].namespaceURN, "urn:child"; got != want {
		t.Fatalf("resolver namespace = %q, want %q", got, want)
	}
	if got, want := resolver.calls[0].location, " child.xsd "; got != want {
		t.Fatalf("resolver schemaLocation = %q, want raw %q", got, want)
	}
	documents := schema.Documents()
	if got, want := documents[0].TargetNamespace(), "urn:root"; got != want {
		t.Fatalf("root target namespace = %q, want %q", got, want)
	}
	if got, want := documents[1].TargetNamespace(), "urn:child"; got != want {
		t.Fatalf("child target namespace = %q, want %q", got, want)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	for index, want := range []QName{
		mustTestQName(t, "urn:root", "item"),
		mustTestQName(t, "urn:child", "child"),
	} {
		if got := components[index].Name(); got != want {
			t.Errorf("component %d name = %q, want %q", index, got, want)
		}
	}
}

func TestSchemaBridgeValidatesCompositionAndAnnotationURIs(t *testing.T) {
	tests := []schemaBridgeDiagnosticCase{
		{
			name:  "include schemaLocation is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="%ZZ"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "import schemaLocation is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import schemaLocation="%ZZ"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "import namespace is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="%ZZ" schemaLocation="child.xsd"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "documentation source is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation><xs:documentation source="%ZZ"/></xs:annotation></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "appinfo source is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation><xs:appinfo source="%ZZ"/></xs:annotation></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "composition xml base is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include xml:base="%ZZ" schemaLocation="child.xsd"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "annotation xml base is an anyURI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation xml:base="%ZZ"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
	}
	assertSchemaBridgeDiagnosticCases(t, tests)
}

func TestSchemaBridgeRejectsUnvalidatedCompositionNodeContent(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "nested include declaration",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"><xs:element name="nested"/></xs:include></xs:schema>`,
		},
		{
			name: "unqualified include namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd" namespace="urn:root"/></xs:schema>`,
		},
		{
			name: "unknown import attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:child" schemaLocation="child.xsd" unexpected="true"/></xs:schema>`,
		},
		{
			name: "nested annotation construct",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"><xs:annotation><xs:element name="nested"/></xs:annotation></xs:include></xs:schema>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid composition node content")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic source = %q, want root.xsd", diagnostic.Loc().Source())
			}
		})
	}
}

//nolint:gocognit,funlen // The table covers each composition distinction and its diagnostic contract.
func TestSchemaBridgeRejectsCompositionWithoutPartialSchema(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		fixtures map[string]discoveryFixture
		class    FailureClass
		feature  FeatureID
		code     string
	}{
		{
			name:  "empty target namespace",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace=""/>`,
			class: FailureInvalid,
			code:  invalidSchemaTargetNamespaceCode,
		},
		{
			name: "chameleon include",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"/>`},
			},
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name: "include adds namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child"/>`},
			},
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name: "include mismatch",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child"/>`},
			},
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name: "import same namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:root" schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"/>`},
			},
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name: "import namespace mismatch",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:child" schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"/>`},
			},
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name: "omitted import namespace without parent namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"/>`},
			},
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name: "omitted import namespace with explicit child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child"/>`},
			},
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, test.fixtures)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid composition")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
			if test.code != "" && diagnostic.Code() != test.code {
				t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
			}
			if test.feature != "" && diagnostic.Feature() != test.feature {
				t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic source = %q, want root.xsd", diagnostic.Loc().Source())
			}
		})
	}
}

//nolint:gocognit // Invalid-name cases and the unsupported direct-root case share setup.
func TestSchemaBridgeRejectsNamesAndUnsupportedDirectRoots(t *testing.T) {
	nameCases := []struct {
		name string
		root string
	}{
		{name: "missing", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element/></xs:schema>`},
		{name: "empty", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name=""/></xs:schema>`},
		{name: "digit start", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="1item"/></xs:schema>`},
		{name: "colon", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="prefix:item"/></xs:schema>`},
		{name: "prefixed attribute", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:private"><xs:element p:name="item"/></xs:schema>`},
	}
	for _, test := range nameCases {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an invalid declaration name")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaDeclarationNameCode {
				t.Fatalf("diagnostic = %s, want invalid declaration name", diagnostic)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic source = %q, want root.xsd", diagnostic.Loc().Source())
			}
		})
	}

	schema, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`"><xs:redefine/></xs:schema>`, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted redefine")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema for redefine")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != MissingSchemaLocationCode {
		t.Fatalf("redefine diagnostic = %s, want missing schemaLocation", diagnostic)
	}
}

func TestSchemaBridgeRejectsNestedLocalDeclarationsAsUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:element name="global"><xs:complexType><xs:sequence><xs:element name="local"/></xs:sequence></xs:complexType></xs:element><xs:annotation/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted nested local declarations")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("nested local diagnostic = %s, want schema syntax unsupported", diagnostic)
	}
}

func TestSchemaBridgeAcceptsActiveConditionalComposition(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="http://www.w3.org/2007/XMLSchema-versioning" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd" vc:minVersion="1.1"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"/>`},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(schema.Components()), 0; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

func TestSchemaBridgeConditionalInclusionFiltersBeforeGrammarAndResolution(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		fixtures   map[string]discoveryFixture
		components int
		calls      int
	}{
		{
			name:       "max boundary filters declaration",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="kept"/><xs:element name="gone" vc:maxVersion="1.1"><xs:alternative/></xs:element></xs:schema>`,
			components: 1,
		},
		{
			name:       "max boundary filters reference",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:include schemaLocation="missing.xsd" vc:maxVersion="1.1"/></xs:schema>`,
			components: 0,
		},
		{
			name:       "empty unavailable filters",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="gone" vc:typeUnavailable=""/></xs:schema>`,
			components: 0,
		},
		{
			name:       "empty available keeps",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="kept" vc:typeAvailable=""/></xs:schema>`,
			components: 1,
		},
		{
			name:       "root exclusion keeps namespace facts",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" targetNamespace="urn:root" vc:maxVersion="1.1"><xs:alternative/></xs:schema>`,
			components: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(test.root)})
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			resolver := &discoveryResolver{fixtures: test.fixtures}
			schema, err := discoverSchema(root, resolver)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			if got := len(schema.Components()); got != test.components {
				t.Fatalf("component count = %d, want %d", got, test.components)
			}
			if got := len(resolver.calls); got != test.calls {
				t.Fatalf("resolver call count = %d, want %d", got, test.calls)
			}
		})
	}
}

func TestSchemaBridgeConditionalExclusionPrecedesAvailabilityUnsupported(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		components int
		targetNS   string
	}{
		{
			name:       "max version excludes available type",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" xmlns:ext="urn:ext"><xs:element name="gone" vc:maxVersion="1.1" vc:typeAvailable="ext:Type"/></xs:schema>`,
			components: 0,
		},
		{
			name:       "min version excludes available facet",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" xmlns:ext="urn:ext"><xs:element name="gone" vc:minVersion="1.2" vc:facetAvailable="ext:Facet"/></xs:schema>`,
			components: 0,
		},
		{
			name:       "max version excludes unavailable type",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" xmlns:ext="urn:ext"><xs:element name="gone" vc:maxVersion="1.1" vc:typeUnavailable="ext:Type"/></xs:schema>`,
			components: 0,
		},
		{
			name:       "empty unavailable excludes available type",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" xmlns:ext="urn:ext"><xs:element name="gone" vc:typeUnavailable="" vc:typeAvailable="ext:Type"/></xs:schema>`,
			components: 0,
		},
		{
			name:       "root exclusion retains target namespace",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" xmlns:ext="urn:ext" targetNamespace="urn:root" vc:maxVersion="1.1" vc:typeAvailable="ext:Type"><xs:alternative/></xs:schema>`,
			components: 0,
			targetNS:   "urn:root",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			if got := len(schema.Components()); got != test.components {
				t.Fatalf("component count = %d, want %d", got, test.components)
			}
			if test.targetNS != "" && schema.Documents()[0].TargetNamespace() != test.targetNS {
				t.Fatalf("target namespace = %q, want %q", schema.Documents()[0].TargetNamespace(), test.targetNS)
			}
		})
	}

	_, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`" xmlns:vc="`+xsdVersioningNamespaceURI+`" vc:maxVersion="1.1" vc:typeAvailable="bad:prefix:extra"/>`, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted malformed availability despite exclusion")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaConditionalCode {
		t.Fatalf("diagnostic = %s, want invalid conditional lexical diagnostic", diagnostic)
	}
}

func TestSchemaBridgeConditionalInclusionValidatesDecimalAndQNameLexicals(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "malformed decimal",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="item" vc:minVersion="nope"/></xs:schema>`,
		},
		{
			name: "malformed QName",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="item" vc:typeAvailable="prefix:item:extra"/></xs:schema>`,
		},
		{
			name: "unbound QName prefix",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="item" vc:facetUnavailable="missing:item"/></xs:schema>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted malformed conditional lexical form")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
			}
		})
	}
}

func TestSchemaBridgeConditionalAvailabilityIsExplicitlyUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `"><xs:element name="item" vc:typeAvailable="xs:string"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted nonempty type availability")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s, want schema syntax unsupported", diagnostic)
	}
}

func TestSchemaBridgeAcceptsAnnotationForeignPayload(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:annotation><xs:documentation xml:lang="en"><foreign:payload xmlns:foreign="urn:payload"><xs:element bogus="kept-as-payload"/></foreign:payload></xs:documentation><xs:appinfo><foreign:metadata xmlns:foreign="urn:payload">text</foreign:metadata></xs:appinfo></xs:annotation><xs:element name="item"><xs:annotation><xs:documentation><foreign:payload xmlns:foreign="urn:payload"/></xs:documentation></xs:annotation></xs:element></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(schema.Components()), 1; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

func TestSchemaBridgeAcceptsRootAnnotationsAfterCompositionAndGlobals(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"><xs:annotation/></xs:include><xs:annotation><xs:documentation>before globals</xs:documentation></xs:annotation><xs:element name="item"/><xs:annotation><xs:appinfo>after global</xs:appinfo></xs:annotation></xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"/>`},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(schema.Components()), 1; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

func TestSchemaBridgeRejectsActiveTextAndAttributesWithoutPartialSchema(t *testing.T) {
	tests := []string{
		`<xs:schema xmlns:xs="` + testXSDNamespace + `">garbage</xs:schema>`,
		`<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item">garbage</xs:element></xs:schema>`,
		`<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" bogus="1"/></xs:schema>`,
		`<xs:schema xmlns:xs="` + testXSDNamespace + `" xs:bogus="1"/>`,
	}
	for _, root := range tests {
		schema, err := discoverTestSchema(t, root, nil)
		if err == nil {
			t.Fatal("discoverSchema accepted invalid active syntax")
		}
		if schema.storage != nil {
			t.Fatal("discoverSchema returned a partial schema")
		}
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Loc().Source() != "root.xsd" {
			t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
		}
	}
}

func TestSchemaBridgeRecognizedGlobalAttributeIsUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:string"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted an unimplemented global attribute")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s, want schema syntax unsupported", diagnostic)
	}
}

func TestSchemaBridgeAcceptsForeignRootAndGlobalAttributes(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:foreign="urn:foreign" foreign:root="ok" xml:space="preserve"><xs:element name="item" foreign:global="ok" xml:lang="en-419"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got, want := len(schema.Components()), 1; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

type schemaBridgeDiagnosticCase struct {
	name    string
	root    string
	policy  LanguagePolicy
	class   FailureClass
	feature FeatureID
	code    string
}

//nolint:gocognit // Check every diagnostic dimension consistently for each case.
func assertSchemaBridgeDiagnosticCases(t *testing.T, tests []schemaBridgeDiagnosticCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := test.policy
			if policy == "" {
				policy = Compatibility
			}
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid or unsupported syntax")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
			if test.feature != "" && diagnostic.Feature() != test.feature {
				t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
			}
			if test.code != "" && diagnostic.Code() != test.code {
				t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic source = %q, want root.xsd", diagnostic.Loc().Source())
			}
		})
	}
}

//nolint:funlen // Keep the direct grammar and attribute matrix explicit.
func TestSchemaBridgeCoversDirectGrammarAndAttributeBoundaries(t *testing.T) {
	tests := []schemaBridgeDiagnosticCase{
		{
			name:    "unknown XSD root child is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:unknown/></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "known forbidden root child is invalid",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:sequence/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "empty target namespace is located",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace=" 	"/>`,
			class: FailureInvalid,
			code:  invalidSchemaTargetNamespaceCode,
		},
		{
			name:  "invalid root name is rejected",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" name="bad:name"/>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "invalid root id is rejected",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" id="bad:id"/>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "recognized root attribute is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" attributeFormDefault="qualified"/>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "default open content is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:defaultOpenContent mode="interleave"><xs:any/></xs:defaultOpenContent></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "attribute default is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="item" default="value"/></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "attribute unknown is invalid",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="item" bogus="value"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "simple type final is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item" final="#all"><xs:restriction base="xs:string"/></xs:simpleType></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "complex type mixed is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item" mixed="true"/></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "complex type unknown is invalid",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item" bogus="true"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "group reference is structurally forbidden",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="item" ref="xs:group"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "attribute group reference is structurally forbidden",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attributeGroup name="item" ref="xs:group"/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "notation public is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="item" public="public"/></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "element sequence is structurally forbidden",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item"><xs:sequence/></xs:element></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "complex type nested sequence model is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:sequence><xs:choice/></xs:sequence></xs:complexType></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "unknown global child is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item"><xs:unknown/></xs:element></xs:schema>`,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "notation element is structurally forbidden",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="item"><xs:element/></xs:notation></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
	}
	assertSchemaBridgeDiagnosticCases(t, tests)
}

//nolint:funlen // Keep occurrence, lexical, and structural choice boundaries together.
func TestSchemaBridgeClassifiesChoiceParticleBoundaries(t *testing.T) {
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Choice">%s</xs:complexType></xs:schema>`
	assertSchemaBridgeDiagnosticCases(t, []schemaBridgeDiagnosticCase{
		{
			name:  "choice min occurrence exceeds omitted max default",
			root:  fmt.Sprintf(base, `<xs:choice minOccurs="2"><xs:element name="value" type="xs:integer"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "choice omitted min default exceeds max occurrence",
			root:  fmt.Sprintf(base, `<xs:choice maxOccurs="0"><xs:element name="value" type="xs:integer"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "element min occurrence exceeds omitted max default",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value" type="xs:integer" minOccurs="2"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "element omitted min default exceeds max occurrence",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value" type="xs:integer" maxOccurs="0"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "nested element form policy remains unsupported",
			root:    fmt.Sprintf(base, `<xs:choice><xs:sequence><xs:element name="value" type="xs:integer" form="unqualified"/></xs:sequence></xs:choice>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "nested sequence is unsupported",
			root:    fmt.Sprintf(base, `<xs:choice><xs:sequence/></xs:choice>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "wildcard is unsupported",
			root:    fmt.Sprintf(base, `<xs:choice><xs:any/></xs:choice>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "element reference is unsupported",
			root:    fmt.Sprintf(base, `<xs:choice><xs:element ref="value"/></xs:choice>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "inline type is unsupported",
			root:    fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:choice>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "empty inline simple type is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType/></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline simple type restriction QName is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="bad:base:QName"/></xs:simpleType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:  "inline simple type duplicate model children are invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:integer"/><xs:restriction base="xs:decimal"/></xs:simpleType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline simple type forbidden child is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType><xs:choice/></xs:simpleType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline simple type name attribute is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline simple type final attribute is lexically invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:simpleType final="bogus"><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "malformed inline complex type is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:complexType><xs:choice><xs:element type="xs:integer"/></xs:choice></xs:complexType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaDeclarationNameCode,
		},
		{
			name:  "inline complex type boolean attribute is lexically invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:complexType mixed="maybe"><xs:choice><xs:element name="nested" type="xs:integer"/></xs:choice></xs:complexType></xs:element></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "valid inline complex type is unsupported",
			root:    fmt.Sprintf(base, `<xs:choice><xs:element name="value"><xs:complexType><xs:choice><xs:element name="nested" type="xs:integer"/></xs:choice></xs:complexType></xs:element></xs:choice>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "direct all is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:all/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "choice annotation after particles is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value" type="xs:integer"/><xs:annotation/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "malformed occurrence is invalid",
			root:  fmt.Sprintf(base, `<xs:choice minOccurs="maybe">`+`<xs:element name="value" type="xs:integer"/>`+`</xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "malformed form is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value" type="xs:integer" form="maybe"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "malformed local NCName is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="bad:name" type="xs:integer"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaDeclarationNameCode,
		},
		{
			name:  "malformed local type QName is invalid",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element name="value" type="bad:type:extra"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:  "choice element requires a name or ref",
			root:  fmt.Sprintf(base, `<xs:choice><xs:element type="xs:integer"/></xs:choice>`),
			class: FailureInvalid,
			code:  invalidSchemaDeclarationNameCode,
		},
	})
}

//nolint:gocognit,funlen // Keep version classification and no-partial-schema assertions together.
func TestSchemaBridgeClassifiesVersionedChoiceElementSyntax(t *testing.T) {
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="%s"><xs:complexType name="Choice"><xs:choice>%s</xs:choice></xs:complexType></xs:schema>`
	tests := []struct {
		name     string
		version  XSDVersion
		policy   LanguagePolicy
		particle string
		class    FailureClass
		code     string
		specRef  string
	}{
		{
			name:     "XSD 1.1 local target namespace requires a containing target",
			version:  XSDVersion11,
			particle: `<xs:element name="value" type="xs:integer" targetNamespace="urn:qualified"/>`,
			class:    FailureInvalid,
			code:     invalidSchemaCompositionCode,
			specRef:  schemaElementTargetNamespaceXSD11SpecRef,
		},
		{
			name:     "XSD 1.0 local target namespace is a strict mismatch",
			version:  XSDVersion10,
			policy:   Strict10,
			particle: `<xs:element name="value" type="xs:integer" targetNamespace="urn:qualified"/>`,
			class:    FailureUnsupported,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "XSD 1.1 element alternative is unsupported",
			version:  XSDVersion11,
			particle: `<xs:element name="value"><xs:alternative id="alternative" test="@kind = 'integer'" type="xs:integer" xpathDefaultNamespace="##local"/></xs:element>`,
			class:    FailureUnsupported,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "XSD 1.0 element alternative is a strict mismatch",
			version:  XSDVersion10,
			policy:   Strict10,
			particle: `<xs:element name="value"><xs:alternative type="xs:integer"/></xs:element>`,
			class:    FailureUnsupported,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "target namespace lexical failure remains invalid",
			version:  XSDVersion11,
			particle: `<xs:element name="value" type="xs:integer" targetNamespace="%ZZ"/>`,
			class:    FailureInvalid,
			code:     invalidSchemaCompositionCode,
		},
		{
			name:     "alternative QName lexical failure remains invalid",
			version:  XSDVersion11,
			particle: `<xs:element name="value"><xs:alternative type="bad:prefix:extra"/></xs:element>`,
			class:    FailureInvalid,
			code:     invalidSchemaConditionalCode,
		},
		{
			name:     "alternative XPath namespace lexical failure remains invalid",
			version:  XSDVersion11,
			particle: `<xs:element name="value"><xs:alternative type="xs:integer" xpathDefaultNamespace="%ZZ"/></xs:element>`,
			class:    FailureInvalid,
			code:     invalidSchemaCompositionCode,
		},
		{
			name:     "alternative structure failure remains invalid",
			version:  XSDVersion11,
			particle: `<xs:element name="value"><xs:alternative/></xs:element>`,
			class:    FailureInvalid,
			code:     invalidSchemaCompositionCode,
		},
		{
			name:     "target namespace and form conflict remains invalid",
			version:  XSDVersion11,
			particle: `<xs:element name="value" type="xs:integer" targetNamespace="urn:qualified" form="qualified"/>`,
			class:    FailureInvalid,
			code:     invalidSchemaCompositionCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := fmt.Sprintf(base, test.version, test.particle)
			policy := test.policy
			if policy == "" {
				policy = Strict11
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err == nil {
				t.Fatal("discoverSchema accepted versioned choice syntax")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
			}
			if test.specRef != "" && diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specRef)
			}
		})
	}
}

func TestSchemaBridgeValidatesInlineTypesInChoiceAlternatives(t *testing.T) {
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Choice"><xs:choice>%s</xs:choice></xs:complexType></xs:schema>`
	tests := []schemaBridgeDiagnosticCase{
		{
			name:  "empty alternative inline simple type is invalid",
			root:  fmt.Sprintf(base, `<xs:element name="value"><xs:alternative><xs:simpleType/></xs:alternative></xs:element>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "valid alternative inline simple type is unsupported",
			root:    fmt.Sprintf(base, `<xs:element name="value"><xs:alternative><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:alternative></xs:element>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
	}
	assertSchemaBridgeDiagnosticCases(t, tests)
}

//nolint:funlen // Keep the issue's direct-choice grammar matrix together.
func TestSchemaBridgePreflightsReachableInlineSyntax(t *testing.T) {
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `"%s><xs:complexType name="Choice"><xs:choice>%s</xs:choice></xs:complexType></xs:schema>`
	wrapper := func(options, body string) string {
		return fmt.Sprintf(base, options, `<xs:element name="value">`+body+`</xs:element>`)
	}
	assertSchemaBridgeDiagnosticCases(t, []schemaBridgeDiagnosticCase{
		{
			name:  "list requires one source",
			root:  wrapper("", `<xs:simpleType><xs:list/></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "list itemType malformed",
			root:  wrapper("", `<xs:simpleType><xs:list itemType="bad:q:name"/></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:  "list itemType unbound",
			root:  wrapper("", `<xs:simpleType><xs:list itemType="missing:Type"/></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:  "list sources are exclusive",
			root:  wrapper("", `<xs:simpleType><xs:list itemType="xs:integer"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:list></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "valid list itemType is unsupported",
			root:    wrapper("", `<xs:simpleType><xs:list itemType="xs:integer"/></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "valid list inline type is unsupported",
			root:    wrapper("", `<xs:simpleType><xs:list><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:list></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "list foreign child is invalid",
			root:  wrapper(` xmlns:f="urn:foreign"`, `<xs:simpleType><xs:list itemType="xs:integer"><f:payload/></xs:list></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "union requires a source",
			root:  wrapper("", `<xs:simpleType><xs:union/></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "union memberTypes malformed",
			root:  wrapper("", `<xs:simpleType><xs:union memberTypes="xs:integer bad:q:name"/></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:    "valid union memberTypes is unsupported",
			root:    wrapper("", `<xs:simpleType><xs:union memberTypes="xs:integer xs:decimal"/></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "valid union memberTypes and inline members are unsupported",
			root:    wrapper("", `<xs:simpleType><xs:union memberTypes="xs:integer"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:union></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "valid union inline members are unsupported",
			root:    wrapper("", `<xs:simpleType><xs:union><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:union></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "union memberTypes cannot be empty with inline members",
			root:  wrapper("", `<xs:simpleType><xs:union memberTypes=""><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:union></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "totalDigits zero is invalid",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:totalDigits value="0"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  InvalidTotalDigitsCode,
		},
		{
			name:  "fractionDigits negative is invalid",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:fractionDigits value="-1"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  InvalidFractionDigitsCode,
		},
		{
			name:  "facet value is required",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "local simpleType final is prohibited",
			root:  wrapper("", `<xs:simpleType final="#all"><xs:restriction base="xs:decimal"/></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "facet fixed boolean is validated",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern value="x" fixed="maybe"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "facet id is validated",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern value="x" id="bad:id"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "valid pattern retains datatype facet feature",
			root:    wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
		},
		{
			name:  "assertion facet test is required",
			root:  wrapper(` version="1.1"`, `<xs:simpleType><xs:restriction base="xs:decimal"><xs:assertion/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "valid assertion facet is unsupported",
			root:    wrapper(` version="1.1"`, `<xs:simpleType><xs:restriction base="xs:decimal"><xs:assertion test="true()"/></xs:restriction></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureID("xsd.assertion"),
			code:    UnsupportedDatatypeFacetCode,
		},
		{
			name:  "restriction inline type cannot follow assertion",
			root:  wrapper(` version="1.1"`, `<xs:simpleType><xs:restriction base="xs:decimal"><xs:assertion test="true()"/><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "explicitTimezone is a strict XSD 1.0 mismatch",
			root:    wrapper(` version="1.0"`, `<xs:simpleType><xs:restriction base="xs:dateTime"><xs:explicitTimezone value="required"/></xs:restriction></xs:simpleType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
		},
		{
			name:  "minScale cannot be negative",
			root:  wrapper(` version="1.1"`, `<xs:simpleType><xs:restriction base="xs:decimal"><xs:minScale value="-1"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "unsupported facet cannot hide malformed later facet",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern value="x"/><xs:totalDigits value="-1"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  InvalidTotalDigitsCode,
		},
		{
			name:    "restriction inline type may precede a facet",
			root:    wrapper("", `<xs:simpleType><xs:restriction><xs:simpleType><xs:restriction base="xs:string"><xs:pattern value="inner"/></xs:restriction></xs:simpleType><xs:pattern value="outer"/></xs:restriction></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
		},
		{
			name:  "restriction inline type cannot hide a malformed later facet",
			root:  wrapper("", `<xs:simpleType><xs:restriction><xs:simpleType><xs:restriction base="xs:string"><xs:pattern value="inner"/></xs:restriction></xs:simpleType><xs:totalDigits value="0"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  InvalidTotalDigitsCode,
		},
		{
			name:   "restriction foreign child is invalid in XSD 1.0",
			root:   wrapper(` version="1.0" xmlns:f="urn:foreign"`, `<xs:simpleType><xs:restriction base="xs:string"><f:pattern/></xs:restriction></xs:simpleType>`),
			policy: Strict10,
			class:  FailureInvalid,
			code:   invalidSchemaCompositionCode,
		},
		{
			name:    "restriction foreign child is unsupported in XSD 1.1",
			root:    wrapper(` version="1.1" xmlns:f="urn:foreign"`, `<xs:simpleType><xs:restriction base="xs:string"><f:pattern/></xs:restriction></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:    "restriction inline type may precede a facet",
			root:    wrapper("", `<xs:simpleType><xs:restriction><xs:simpleType><xs:restriction base="xs:string"><xs:pattern value="inner"/></xs:restriction></xs:simpleType><xs:pattern value="outer"/></xs:restriction></xs:simpleType>`),
			class:   FailureUnsupported,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
		},
		{
			name:  "restriction inline type cannot hide a malformed later facet",
			root:  wrapper("", `<xs:simpleType><xs:restriction><xs:simpleType><xs:restriction base="xs:string"><xs:pattern value="inner"/></xs:restriction></xs:simpleType><xs:totalDigits value="0"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  InvalidTotalDigitsCode,
		},
		{
			name:  "simpleContent requires derivation",
			root:  wrapper("", `<xs:complexType><xs:simpleContent/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "simpleContent extension is unsupported",
			root:    wrapper("", `<xs:complexType><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "simpleContent base QName is validated",
			root:  wrapper("", `<xs:complexType><xs:simpleContent><xs:extension base="bad:q:name"/></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:  "simpleContent rejects a model particle",
			root:  wrapper("", `<xs:complexType><xs:simpleContent><xs:extension base="xs:string"><xs:sequence/></xs:extension></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "complexContent foreign child is invalid",
			root:  wrapper(` xmlns:f="urn:foreign"`, `<xs:complexType><xs:complexContent><f:payload/></xs:complexContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "all annotation must precede elements",
			root:  wrapper("", `<xs:complexType><xs:all><xs:element name="item" type="xs:string"/><xs:annotation/></xs:all></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "all minOccurs is restricted to zero or one",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:all minOccurs="2"><xs:element name="item" type="xs:string"/></xs:all></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:   "all maxOccurs rejects unbounded",
			root:   wrapper(` version="1.0"`, `<xs:complexType><xs:all maxOccurs="unbounded"><xs:element name="item" type="xs:string"/></xs:all></xs:complexType>`),
			policy: Strict10,
			class:  FailureInvalid,
			code:   invalidSchemaCompositionCode,
		},
		{
			name:    "XSD 1.1 all permits zero min and max occurrences",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:all minOccurs="0" maxOccurs="0"><xs:element name="item" type="xs:string"/></xs:all></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "XSD 1.1 all rejects effective min one with max zero",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:all minOccurs="1" maxOccurs="0"><xs:element name="item" type="xs:string"/></xs:all></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "XSD 1.0 all zero max occurrences is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:all minOccurs="0" maxOccurs="0"><xs:element name="item" type="xs:string"/></xs:all></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:   "XSD 1.0 all child occurrences are restricted",
			root:   wrapper(` version="1.0"`, `<xs:complexType><xs:all><xs:element name="item" type="xs:string" minOccurs="2"/></xs:all></xs:complexType>`),
			policy: Strict10,
			class:  FailureInvalid,
			code:   invalidSchemaCompositionCode,
		},
		{
			name:    "XSD 1.0 all any child is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:all><xs:any/></xs:all></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:    "XSD 1.0 all group child is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:all><xs:group ref="xs:group"/></xs:all></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:    "XSD 1.1 all permits any children as unfinished",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:all><xs:any/></xs:all></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "XSD 1.1 all permits group children as unfinished",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:all><xs:group ref="xs:group"/></xs:all></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "XSD 1.1 all group one occurrences are unfinished",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:all><xs:group ref="xs:group" minOccurs="1" maxOccurs="1"/></xs:all></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "XSD 1.1 all group rejects zero minOccurs",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:all><xs:group ref="xs:group" minOccurs="0"/></xs:all></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "all unsupported child cannot hide malformed later element",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:all><xs:any/><xs:element name="item" abstract="true"/></xs:all></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "group annotation must be unique",
			root:  wrapper("", `<xs:complexType><xs:group ref="xs:group"><xs:annotation/><xs:annotation/></xs:group></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "simpleContent rejects outer mixed",
			root:  wrapper("", `<xs:complexType mixed="true"><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "simpleContent permits outer mixed in XSD 1.0",
			root:    wrapper(` version="1.0"`, `<xs:complexType mixed="true"><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "simpleContent permits outer mixed false",
			root:    wrapper("", `<xs:complexType mixed="false"><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "complexContent mixed values must agree",
			root:  wrapper("", `<xs:complexType mixed="true"><xs:complexContent mixed="false"><xs:extension base="xs:string"/></xs:complexContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "complexContent mixed mismatch is unfinished in XSD 1.0",
			root:    wrapper(` version="1.0"`, `<xs:complexType mixed="true"><xs:complexContent mixed="false"><xs:extension base="xs:string"/></xs:complexContent></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "complexContent matching mixed values are unsupported",
			root:    wrapper("", `<xs:complexType mixed="true"><xs:complexContent mixed="true"><xs:extension base="xs:string"/></xs:complexContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "simpleContent restriction inline type is preflighted",
			root:    wrapper("", `<xs:complexType><xs:simpleContent><xs:restriction base="xs:string"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType><xs:pattern value="x"/></xs:restriction></xs:simpleContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
		},
		{
			name:  "simpleContent annotation follows inline type",
			root:  wrapper("", `<xs:complexType><xs:simpleContent><xs:restriction base="xs:string"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType><xs:annotation/></xs:restriction></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "simpleContent inline type follows anyAttribute",
			root:  wrapper("", `<xs:complexType><xs:simpleContent><xs:restriction base="xs:string"><xs:anyAttribute/><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:restriction></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "simpleContent facet follows anyAttribute",
			root:  wrapper("", `<xs:complexType><xs:simpleContent><xs:restriction base="xs:string"><xs:anyAttribute/><xs:pattern value="x"/></xs:restriction></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "complexContent base is required",
			root:  wrapper("", `<xs:complexType><xs:complexContent><xs:extension/></xs:complexContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "complexContent extension is unsupported",
			root:    wrapper("", `<xs:complexType><xs:complexContent><xs:extension base="xs:string"/></xs:complexContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "complexContent restriction openContent requires a model",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:complexContent><xs:restriction base="xs:string"><xs:openContent mode="none"/></xs:restriction></xs:complexContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "complexContent attributes are preflighted",
			root:    wrapper("", `<xs:complexType><xs:complexContent><xs:extension base="xs:string"><xs:attribute name="item" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "simpleContent assertions are unsupported in XSD 1.1",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:simpleContent><xs:extension base="xs:string"><xs:assert test="true()"/><xs:assert test="true()"/></xs:extension></xs:simpleContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:   "openContent is invalid in XSD 1.0",
			root:   wrapper(` version="1.0"`, `<xs:complexType><xs:openContent/></xs:complexType>`),
			policy: Strict10,
			class:  FailureInvalid,
			code:   invalidSchemaCompositionCode,
		},
		{
			name:  "openContent requires any in XSD 1.1",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="suffix"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "openContent none rejects any",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="none"><xs:any/></xs:openContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "openContent none remains unsupported without any",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="none"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "openContent default requires any",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:openContent/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "openContent suffix with any is unsupported",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="suffix"><xs:any namespace="##any"/></xs:openContent></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "openContent any forbids occurrences",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="suffix"><xs:any minOccurs="0"/></xs:openContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "openContent mode is validated",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="bad"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline complexType abstract is invalid",
			root:  wrapper("", `<xs:complexType abstract="true"><xs:sequence/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline complexType block is invalid",
			root:  wrapper("", `<xs:complexType block="extension"><xs:sequence/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "inline complexType final is invalid",
			root:  wrapper("", `<xs:complexType final="extension"><xs:sequence/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "inline complexType defaultAttributesApply is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType defaultAttributesApply="true"><xs:sequence/></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:    "inline complexType defaultAttributesApply is unsupported in XSD 1.1",
			root:    wrapper(` version="1.1"`, `<xs:complexType defaultAttributesApply="true"><xs:sequence/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "inline complexType mixed remains unsupported",
			root:    wrapper("", `<xs:complexType mixed="true"><xs:sequence/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "local attribute requires name or ref",
			root:  wrapper("", `<xs:complexType><xs:attribute type="xs:string"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "local attribute QName is validated",
			root:  wrapper("", `<xs:complexType><xs:attribute name="item" type="bad:q:name"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:    "local attribute declaration is unsupported",
			root:    wrapper("", `<xs:complexType><xs:attribute name="item" type="xs:string"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "local attribute default requires optional use",
			root:  wrapper("", `<xs:complexType><xs:attribute ref="xml:lang" use="required" default="en"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "local attribute default with optional use is unsupported",
			root:    wrapper("", `<xs:complexType><xs:attribute ref="xml:lang" use="optional" default="en"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "local attribute fixed rejects prohibited use",
			root:  wrapper("", `<xs:complexType><xs:attribute name="item" use="prohibited" fixed="en"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "local attribute fixed prohibited use is unfinished in XSD 1.0",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:attribute name="item" use="prohibited" fixed="en"/></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "local attribute targetNamespace is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:attribute name="item" targetNamespace="urn:item"/></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:    "local attribute targetNamespace is unsupported in XSD 1.1",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:attribute name="item" targetNamespace="urn:item"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "attributeGroup ref is required",
			root:  wrapper("", `<xs:complexType><xs:attributeGroup/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "attributeGroup ref QName is validated",
			root:  wrapper("", `<xs:complexType><xs:attributeGroup ref="bad:q:name"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:    "attributeGroup ref is unsupported",
			root:    wrapper("", `<xs:complexType><xs:attributeGroup ref="xs:group"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "anyAttribute processContents is validated",
			root:  wrapper("", `<xs:complexType><xs:anyAttribute processContents="bad"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "anyAttribute notQName is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:anyAttribute notQName="xs:integer"/></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:    "anyAttribute wildcard is unsupported",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:anyAttribute notQName="xs:integer"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "anyAttribute namespace and notQName are unsupported",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:anyAttribute namespace="##any" notQName="##defined"/></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "anyAttribute rejects definedSibling notQName",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:anyAttribute notQName="##definedSibling"/></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "any accepts definedSibling notQName",
			root:    wrapper(` version="1.1"`, `<xs:complexType><xs:choice><xs:any notQName="##definedSibling"/></xs:choice></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "any particle notQName is a strict mismatch",
			root:    wrapper(` version="1.0"`, `<xs:complexType><xs:choice><xs:any notQName="xs:integer"/></xs:choice></xs:complexType>`),
			policy:  Strict10,
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
		},
		{
			name:  "local element abstract is forbidden",
			root:  wrapper("", `<xs:complexType><xs:choice><xs:element name="item" abstract="true"/></xs:choice></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "local element substitutionGroup is forbidden",
			root:  wrapper("", `<xs:complexType><xs:choice><xs:element name="item" substitutionGroup="xs:item"/></xs:choice></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:    "local element block remains unsupported",
			root:    wrapper("", `<xs:complexType><xs:choice><xs:element name="item" block="extension"/></xs:choice></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "local element nillable remains unsupported",
			root:    wrapper("", `<xs:complexType><xs:choice><xs:element name="item" nillable="true"/></xs:choice></xs:complexType>`),
			class:   FailureUnsupported,
			feature: FeatureSchemaSyntax,
		},
		{
			name:  "malformed descendant wins after unsupported facet",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern value="x"/><xs:enumeration unknown="y"/></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "list unsupported child cannot hide malformed later sibling",
			root:  wrapper("", `<xs:simpleType><xs:list itemType="xs:integer"><xs:unknown/><xs:annotation/></xs:list></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "union unsupported child cannot hide malformed later sibling",
			root:  wrapper("", `<xs:simpleType><xs:union memberTypes="xs:integer"><xs:unknown/><xs:annotation/></xs:union></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "facet unsupported child cannot hide malformed later sibling",
			root:  wrapper("", `<xs:simpleType><xs:restriction base="xs:decimal"><xs:pattern value="x"><xs:unknown/><xs:annotation/></xs:pattern></xs:restriction></xs:simpleType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "complex derivation unsupported child cannot hide malformed later sibling",
			root:  wrapper("", `<xs:complexType><xs:simpleContent><xs:extension base="xs:string"><xs:unknown/><xs:sequence/></xs:extension></xs:simpleContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "openContent unsupported child cannot hide malformed later sibling",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:openContent mode="none"><xs:unknown/><xs:sequence/></xs:openContent></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "local attribute unsupported child cannot hide malformed later sibling",
			root:  wrapper("", `<xs:complexType><xs:attribute name="item"><xs:unknown/><xs:sequence/></xs:attribute></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "alternative unsupported child cannot hide malformed later sibling",
			root:  wrapper(` version="1.1"`, `<xs:complexType><xs:choice><xs:element name="item"><xs:alternative type="xs:string"><xs:unknown/><xs:choice/></xs:alternative></xs:element></xs:choice></xs:complexType>`),
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
	})
}

func TestSchemaBridgePreservesEarliestNestedRestrictionUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="value"><xs:simpleType>
      <xs:restriction>
        <xs:simpleType>
          <xs:restriction base="xs:decimal">
            <xs:pattern value="inner"/>
          </xs:restriction>
        </xs:simpleType>
        <xs:pattern value="outer"/>
      </xs:restriction>
    </xs:simpleType></xs:element>
  </xs:choice></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted unsupported nested restriction syntax")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, want nested datatype facet unsupported", diagnostic)
	}
	if diagnostic.SpecRef() != "xsd11-datatypes#decimal" {
		t.Fatalf("diagnostic spec ref = %q, want xsd11-datatypes#decimal", diagnostic.SpecRef())
	}
	if got, want := diagnostic.Loc(), mustTestLoc(t, "root.xsd", 7, 13); got != want {
		t.Fatalf("diagnostic location = %s, want nested pattern at %s", got, want)
	}
}

//nolint:gocognit // Keep target classification, locations, causes, and refs together.
func TestSchemaBridgePreservesChoiceElementTypeDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		policy     LanguagePolicy
		class      FailureClass
		code       string
		cause      error
		feature    FeatureID
		specRef    string
		relatedMin int
	}{
		{
			name:    "unresolved",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing" version="1.0"><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="m:Missing"/></xs:choice></xs:complexType></xs:schema>`,
			policy:  Strict10,
			class:   FailureInvalid,
			code:    diagnosticSchemaElementTypeUnresolvedCode,
			cause:   errSchemaElementTypeUnresolved,
			specRef: schemaElementTypeXSD10SpecRef,
		},
		{
			name:       "wrong kind",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="target"/><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="target"/></xs:choice></xs:complexType></xs:schema>`,
			class:      FailureInvalid,
			code:       diagnosticSchemaElementTypeWrongKindCode,
			cause:      errSchemaElementTypeWrongKind,
			specRef:    schemaElementTypeXSD11SpecRef,
			relatedMin: 1,
		},
		{
			name:       "ambiguous",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType><xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="Amount"/></xs:choice></xs:complexType></xs:schema>`,
			class:      FailureInvalid,
			code:       diagnosticSchemaGlobalDuplicateCode,
			cause:      errSchemaGlobalDeclarationDuplicate,
			specRef:    schemaGlobalDuplicateXSD11SpecRef,
			relatedMin: 1,
		},
		{
			name:    "named complex type",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Target"/><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="Target"/></xs:choice></xs:complexType></xs:schema>`,
			class:   FailureUnsupported,
			code:    UnsupportedSchemaSyntaxCode,
			feature: FeatureSchemaSyntax,
		},
		{
			name:    "unsupported built-in scalar",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:string"/></xs:choice></xs:complexType></xs:schema>`,
			class:   FailureUnsupported,
			code:    UnsupportedSchemaSyntaxCode,
			feature: FeatureSchemaSyntax,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := test.policy
			if policy == "" {
				policy = Compatibility
			}
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
			if err == nil {
				t.Fatal("discoverSchema accepted an invalid or unsupported local type")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
			}
			if test.feature != "" && diagnostic.Feature() != test.feature {
				t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
			}
			if test.specRef != "" && diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specRef)
			}
			if len(diagnostic.Related()) < test.relatedMin {
				t.Fatalf("related locations = %v, want at least %d", diagnostic.Related(), test.relatedMin)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
			}
		})
	}
}

func TestSchemaBridgeAcceptsInertRootMetadata(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" id="schema-id" version="" xml:lang="en-419"/>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got := len(schema.Components()); got != 0 {
		t.Fatalf("component count = %d, want 0", got)
	}
}

func TestSchemaBridgeRejectsRootAndGlobalLexicalBoundaries(t *testing.T) {
	tests := []schemaBridgeDiagnosticCase{
		{
			name:  "root name is not an attribute",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" name="schema-name"/>`,
			class: FailureInvalid,
		},
		{
			name:  "root defaultAttributesApply is not an attribute",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributesApply="true"/>`,
			class: FailureInvalid,
		},
		{
			name:  "root blockDefault validates before unsupported",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="bogus"/>`,
			class: FailureInvalid,
		},
		{
			name:  "root defaultAttributes validates QName",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributes="missing:Defaults"/>`,
			class: FailureInvalid,
		},
		{
			name:  "root xpathDefaultNamespace validates URI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xpathDefaultNamespace="%ZZ"/>`,
			class: FailureInvalid,
		},
		{
			name:  "element abstract validates boolean",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" abstract="maybe"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "element block validates list",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" block="bogus"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "element default and fixed are exclusive",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" default="one" fixed="two"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "complex type defaultAttributesApply validates boolean",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item" defaultAttributesApply="maybe"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "notation system validates URI",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="item" system="http://[bad"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "valid unimplemented boolean remains unsupported",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" abstract="true"/></xs:schema>`,
			class: FailureUnsupported,
		},
	}
	assertSchemaBridgeDiagnosticCases(t, tests)
}

func TestSchemaBridgeIgnoresForeignLocalNameCollisions(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:foreign" p:targetNamespace="urn:wrong" p:name="wrong"><xs:element name="item" p:name="foreign-name"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if got := schema.Documents()[0].TargetNamespace(); got != "" {
		t.Fatalf("target namespace = %q, want empty namespace", got)
	}
	if got := len(schema.Components()); got != 1 {
		t.Fatalf("component count = %d, want 1", got)
	}

	_, err = discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`" xmlns:p="urn:foreign"><xs:element p:name="foreign-only"/></xs:schema>`, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted a declaration without an unqualified name")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaDeclarationNameCode {
		t.Fatalf("diagnostic = %s, want invalid declaration name", diagnostic)
	}
}

func TestSchemaBridgeRejectsDuplicateGlobalElements(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing" targetNamespace="urn:root">
  <xs:element name="item"/>
  <xs:element name="item"/>
  <xs:element name="unresolved" type="m:Missing"/>
</xs:schema>`
	firstLoc := mustTestLoc(t, "root.xsd", 2, 3)
	laterLoc := mustTestLoc(t, "root.xsd", 3, 3)
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		specRef string
	}{
		{name: "Compatibility", policy: Compatibility, specRef: schemaElementDuplicateXSD11SpecRef},
		{name: "Strict10", policy: Strict10, specRef: schemaElementDuplicateXSD10SpecRef},
		{name: "Strict11", policy: Strict11, specRef: schemaElementDuplicateXSD11SpecRef},
	} {
		t.Run(test.name, func(t *testing.T) {
			var first Diagnostic
			for iteration := 0; iteration < 3; iteration++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
				diagnostic := requireSchemaElementDuplicateDiagnostic(t, schema, err, laterLoc, []Loc{firstLoc}, test.specRef)
				if iteration == 0 {
					first = diagnostic
					continue
				}
				assertSameSchemaDiagnostic(t, first, diagnostic)
			}
		})
	}

	root = `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:include schemaLocation="child.xsd"/>
  <xs:element name="item"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="item"/>
</xs:schema>`},
	}, Strict11)
	if diagnostic := requireSchemaElementDuplicateDiagnostic(t, schema, err, mustTestLoc(t, "child.xsd", 2, 3), []Loc{mustTestLoc(t, "root.xsd", 3, 3)}, schemaElementDuplicateXSD11SpecRef); diagnostic.Code() != diagnosticSchemaElementDuplicateCode {
		t.Fatalf("composed diagnostic code = %q, want %q", diagnostic.Code(), diagnosticSchemaElementDuplicateCode)
	}
}

func TestSchemaBridgeAcceptsGlobalElementNamesAcrossNamespaces(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:import namespace="urn:child" schemaLocation="child.xsd"/>
  <xs:element name="item"/>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child">
  <xs:element name="item"/>
</xs:schema>`},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	if components[0].Name().Local() != components[1].Name().Local() {
		t.Fatalf("local names = %q/%q, want equal names", components[0].Name().Local(), components[1].Name().Local())
	}
	if components[0].Name().Namespace() == components[1].Name().Namespace() {
		t.Fatalf("namespaces = %q/%q, want distinct namespaces", components[0].Name().Namespace(), components[1].Name().Namespace())
	}
}

func TestSchemaBridgeRejectsDuplicateNonElementGlobalDeclarations(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		message string
	}{
		{
			name: "attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:attribute name="item"/>
  <xs:attribute name="item"/>
</xs:schema>`,
			message: `global attribute declaration "{urn:test}item" is duplicated`,
		},
		{
			name: "simple types",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:simpleType name="item"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="item"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`,
			message: `global type definition "{urn:test}item" is duplicated`,
		},
		{
			name: "complex types",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:complexType name="item"/>
  <xs:complexType name="item"/>
</xs:schema>`,
			message: `global type definition "{urn:test}item" is duplicated`,
		},
		{
			name: "attribute groups",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:attributeGroup name="item"/>
  <xs:attributeGroup name="item"/>
</xs:schema>`,
			message: `global attribute group definition "{urn:test}item" is duplicated`,
		},
		{
			name: "notations",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:notation name="item"/>
  <xs:notation name="item"/>
</xs:schema>`,
			message: `global notation declaration "{urn:test}item" is duplicated`,
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		specRef string
	}{
		{name: "XSD 1.0", value: Strict10, specRef: schemaGlobalDuplicateXSD10SpecRef},
		{name: "XSD 1.1", value: Strict11, specRef: schemaGlobalDuplicateXSD11SpecRef},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				firstLoc := mustTestLoc(t, "root.xsd", 2, 3)
				laterLoc := mustTestLoc(t, "root.xsd", 3, 3)
				var first Diagnostic
				for iteration := 0; iteration < 3; iteration++ {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.value)
					diagnostic := requireSchemaDuplicateDiagnostic(t, schema, err, laterLoc, []Loc{firstLoc}, policy.specRef, test.message, errSchemaGlobalDeclarationDuplicate)
					if iteration == 0 {
						first = diagnostic
						continue
					}
					assertSameSchemaDiagnostic(t, first, diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Keep every composed non-element symbol-space contract together.
func TestSchemaBridgeRejectsComposedDuplicateNonElementGlobalDeclarations(t *testing.T) {
	tests := []struct {
		name                string
		earliestDeclaration string
		laterDeclaration    string
		message             string
	}{
		{
			name:                "attributes",
			earliestDeclaration: "  <xs:attribute name=\"item\"/>\n",
			laterDeclaration:    "  <xs:attribute name=\"item\"/>\n",
			message:             `global attribute declaration "{urn:test}item" is duplicated`,
		},
		{
			name:                "shared types",
			earliestDeclaration: "  <xs:simpleType name=\"item\"><xs:restriction base=\"xs:integer\"/></xs:simpleType>\n",
			laterDeclaration:    "  <xs:complexType name=\"item\"/>\n",
			message:             `global type definition "{urn:test}item" is duplicated`,
		},
		{
			name:                "attribute groups",
			earliestDeclaration: "  <xs:attributeGroup name=\"item\"/>\n",
			laterDeclaration:    "  <xs:attributeGroup name=\"item\"/>\n",
			message:             `global attribute group definition "{urn:test}item" is duplicated`,
		},
		{
			name:                "notations",
			earliestDeclaration: "  <xs:notation name=\"item\"/>\n",
			laterDeclaration:    "  <xs:notation name=\"item\"/>\n",
			message:             `global notation declaration "{urn:test}item" is duplicated`,
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		specRef string
	}{
		{name: "Strict10", value: Strict10, specRef: schemaGlobalDuplicateXSD10SpecRef},
		{name: "Strict11", value: Strict11, specRef: schemaGlobalDuplicateXSD11SpecRef},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:include schemaLocation="earliest.xsd"/>
  <xs:include schemaLocation="later.xsd"/>
</xs:schema>`
				fixtures := map[string]discoveryFixture{
					"later.xsd": {id: "later.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
` + test.laterDeclaration + `</xs:schema>`},
					"earliest.xsd": {id: "earliest.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
` + test.earliestDeclaration + `</xs:schema>`},
				}
				firstLoc := mustTestLoc(t, "earliest.xsd", 2, 3)
				laterLoc := mustTestLoc(t, "later.xsd", 2, 3)
				var first Diagnostic
				for iteration := 0; iteration < 3; iteration++ {
					discovery, discoveryErr := discoverTestSyntaxWithPolicy(t, root, fixtures, policy.value)
					if discoveryErr != nil {
						t.Fatalf("discoverSyntax: %v", discoveryErr)
					}
					assertSchemaDiscoveryDocumentOrder(t, discovery, []SourceID{"root.xsd", "earliest.xsd", "later.xsd"})
					schema, err := newSchemaFromDiscoveryWithPolicy(discovery, policy.value)
					diagnostic := requireSchemaDuplicateDiagnostic(t, schema, err, laterLoc, []Loc{firstLoc}, policy.specRef, test.message, errSchemaGlobalDeclarationDuplicate)
					if iteration == 0 {
						first = diagnostic
						continue
					}
					assertSameSchemaDiagnostic(t, first, diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Keep unsupported model-group metadata checks together.
func TestSchemaBridgeKeepsComposedModelGroupCollisionsAtUnsupportedBoundary(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:include schemaLocation="earliest.xsd"/>
  <xs:include schemaLocation="later.xsd"/>
</xs:schema>`
	group := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:group name="item">
    <xs:sequence/>
  </xs:group>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"later.xsd":    {id: "later.xsd", contents: group},
		"earliest.xsd": {id: "earliest.xsd", contents: group},
	}
	for _, policy := range []struct {
		name  string
		value LanguagePolicy
	}{
		{name: "Strict10", value: Strict10},
		{name: "Strict11", value: Strict11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			var first Diagnostic
			for iteration := 0; iteration < 3; iteration++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.value)
				if err == nil || schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("composed model-group collision was accepted or returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
				}
				if diagnostic.Loc() != mustTestLoc(t, "earliest.xsd", 3, 5) {
					t.Fatalf("diagnostic location = %s, want earliest model child", diagnostic.Loc())
				}
				if diagnostic.Message() != "global group child <sequence> is not implemented" {
					t.Fatalf("diagnostic message = %q, want explicit group boundary", diagnostic.Message())
				}
				if diagnostic.SpecRef() != "xsd10-structures#schema-document" {
					t.Fatalf("diagnostic spec ref = %q, want schema document reference", diagnostic.SpecRef())
				}
				if len(diagnostic.Related()) != 0 {
					t.Fatalf("diagnostic related locations = %v, want none at unsupported boundary", diagnostic.Related())
				}
				if !errors.Is(err, ErrUnsupported) || errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
					t.Fatalf("diagnostic does not preserve the unsupported boundary: %v", err)
				}
				if iteration == 0 {
					first = diagnostic
					continue
				}
				assertSameSchemaDiagnostic(t, first, diagnostic)
			}
		})
	}
}

//nolint:gocognit,funlen // Exercise graph and declaration order in one deterministic table.
func TestSchemaBridgeCollisionOrderFollowsReversedDiscoveryAndDeclarations(t *testing.T) {
	tests := []struct {
		name          string
		root          string
		fixtures      map[string]discoveryFixture
		documents     []SourceID
		primarySource SourceID
		primaryLine   int
		primaryColumn int
		relatedSource SourceID
		relatedLine   int
		relatedColumn int
		message       string
	}{
		{
			name: "reversed include order",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:include schemaLocation="later.xsd"/>
  <xs:include schemaLocation="earliest.xsd"/>
</xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"earliest.xsd": {id: "earliest.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:attribute name="item"/>
</xs:schema>`},
				"later.xsd": {id: "later.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:attribute name="item"/>
</xs:schema>`},
			},
			documents:     []SourceID{"root.xsd", "later.xsd", "earliest.xsd"},
			primarySource: "earliest.xsd", primaryLine: 2, primaryColumn: 3,
			relatedSource: "later.xsd", relatedLine: 2, relatedColumn: 3,
			message: `global attribute declaration "{urn:test}item" is duplicated`,
		},
		{
			name: "reversed import order",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:import namespace="urn:test" schemaLocation="later.xsd"/>
  <xs:import namespace="urn:test" schemaLocation="earliest.xsd"/>
</xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"earliest.xsd": {id: "earliest.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:simpleType name="item"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`},
				"later.xsd": {id: "later.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:complexType name="item"/>
</xs:schema>`},
			},
			documents:     []SourceID{"root.xsd", "later.xsd", "earliest.xsd"},
			primarySource: "earliest.xsd", primaryLine: 2, primaryColumn: 3,
			relatedSource: "later.xsd", relatedLine: 2, relatedColumn: 3,
			message: `global type definition "{urn:test}item" is duplicated`,
		},
		{
			name: "declaration order",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:include schemaLocation="declarations.xsd"/>
</xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"declarations.xsd": {id: "declarations.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:simpleType name="item"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:attribute name="middle"/>
  <xs:complexType name="item"/>
</xs:schema>`},
			},
			documents:     []SourceID{"root.xsd", "declarations.xsd"},
			primarySource: "declarations.xsd", primaryLine: 4, primaryColumn: 3,
			relatedSource: "declarations.xsd", relatedLine: 2, relatedColumn: 3,
			message: `global type definition "{urn:test}item" is duplicated`,
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		specRef string
	}{
		{name: "Strict10", value: Strict10, specRef: schemaGlobalDuplicateXSD10SpecRef},
		{name: "Strict11", value: Strict11, specRef: schemaGlobalDuplicateXSD11SpecRef},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				primary := mustTestLoc(t, test.primarySource, test.primaryLine, test.primaryColumn)
				related := mustTestLoc(t, test.relatedSource, test.relatedLine, test.relatedColumn)
				var first Diagnostic
				for iteration := 0; iteration < 3; iteration++ {
					discovery, discoveryErr := discoverTestSyntaxWithPolicy(t, test.root, test.fixtures, policy.value)
					if discoveryErr != nil {
						t.Fatalf("discoverSyntax: %v", discoveryErr)
					}
					assertSchemaDiscoveryDocumentOrder(t, discovery, test.documents)
					schema, err := newSchemaFromDiscoveryWithPolicy(discovery, policy.value)
					diagnostic := requireSchemaDuplicateDiagnostic(t, schema, err, primary, []Loc{related}, policy.specRef, test.message, errSchemaGlobalDeclarationDuplicate)
					if iteration == 0 {
						first = diagnostic
						continue
					}
					assertSameSchemaDiagnostic(t, first, diagnostic)
				}
			})
		}
	}
}

func TestSchemaBridgeRejectsXMLNamespaceAliases(t *testing.T) {
	for _, declaration := range []string{
		`xmlns:xmlish="` + xmlNamespaceURI + `"`,
		`xmlns="` + xmlNamespaceURI + `"`,
	} {
		t.Run(declaration, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" ` + declaration + `/>`
			schema, err := discoverTestSchema(t, root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an XML namespace alias")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidXMLSyntaxCode {
				t.Fatalf("diagnostic = %s, want invalid XML syntax", diagnostic)
			}
		})
	}
}

func TestSchemaBridgeRejectsRootConstructsAfterDeclarations(t *testing.T) {
	for _, child := range []string{"redefine", "override", "defaultOpenContent"} {
		t.Run(child, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item"/><xs:` + child + `/></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an out-of-order schema child")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s, want invalid composition", diagnostic)
			}
		})
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="extension"><xs:element name="item"/><xs:redefine/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted an out-of-order child with unsupported root metadata")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic = %s, want invalid composition before unsupported metadata", diagnostic)
	}
}

func TestSchemaBridgeEnforcesGlobalChildModels(t *testing.T) {
	tests := []schemaBridgeDiagnosticCase{
		{
			name:  "empty simple type",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "simple type unsupported attribute does not hide missing child",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item" final="#all"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "empty group",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="item"/></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "element simple and complex types are exclusive",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item"><xs:simpleType/><xs:complexType/></xs:element></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "attribute simple type is unique",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="item"><xs:simpleType/><xs:simpleType/></xs:attribute></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "complex content alternatives are exclusive",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:simpleContent/><xs:sequence/></xs:complexType></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "complex attributes precede no later model",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:attribute name="nested"/><xs:sequence/></xs:complexType></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "complex attributes precede no later simple content",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:attribute name="nested"/><xs:simpleContent/></xs:complexType></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "complex attribute group precedes no later complex content",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:attributeGroup/><xs:complexContent/></xs:complexType></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "complex attributes precede no later open content",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:attribute name="nested"/><xs:openContent/></xs:complexType></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "group model is unique",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="item"><xs:sequence/><xs:choice/></xs:group></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "attribute group anyAttribute is last",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attributeGroup name="item"><xs:anyAttribute/><xs:attribute name="nested"/></xs:attributeGroup></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "simple type name is required before unsupported child",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "group model is unsupported after grammar validation",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="item"><xs:sequence/></xs:group></xs:schema>`,
			class: FailureUnsupported,
		},
	}
	assertSchemaBridgeDiagnosticCases(t, tests)
}

func TestSchemaBridgeValidatesSimpleTypeRestrictionDiagnostics(t *testing.T) {
	tests := []schemaBridgeDiagnosticCase{
		{
			name:  "restriction requires a base",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction/></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "restriction id is an NCName",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal" id="bad:id"/></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "restriction rejects XSD attributes",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal" xs:unknown="value"/></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "restriction annotation follows facets",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="2"/><xs:annotation/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "restriction totalDigits is unique",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="2"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "totalDigits requires a value",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "totalDigits fixed is boolean",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="2" fixed="maybe"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "totalDigits id is an NCName",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="2" id="bad:id"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
		{
			name:  "restriction rejects a list child",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:list/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaCompositionCode,
		},
	}
	assertSchemaBridgeDiagnosticCases(t, tests)
}

func TestSchemaBridgePreservesSimpleTypeFacetResolutionDiagnostics(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.0"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:fractionDigits value="-1"/></xs:restriction></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("discoverSchema accepted an invalid fractionDigits value")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidFractionDigitsCode {
		t.Fatalf("diagnostic = %s, want invalid fractionDigits diagnostic", diagnostic)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %v, want root.xsd location", diagnostic.Loc())
	}
}

func assertSchemaDiscoveryDocumentOrder(t *testing.T, discovery syntaxDiscoveryResult, want []SourceID) {
	t.Helper()
	if got := len(discovery.documents); got != len(want) {
		t.Fatalf("discovered document count = %d, want %d", got, len(want))
	}
	for index, wantSource := range want {
		if got := discovery.documents[index].source; got != wantSource {
			t.Fatalf("discovered document %d source = %q, want %q", index, got, wantSource)
		}
	}
}

func discoverTestSchema(t *testing.T, rootContents string, fixtures map[string]discoveryFixture) (Schema, error) {
	return discoverTestSchemaWithPolicy(t, rootContents, fixtures, Compatibility)
}

func discoverTestSyntaxWithPolicy(t *testing.T, rootContents string, fixtures map[string]discoveryFixture, policy LanguagePolicy) (syntaxDiscoveryResult, error) {
	t.Helper()
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootContents)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{fixtures: fixtures}
	return discoverSyntaxWithPolicy(root, resolver, policy)
}

func discoverTestSchemaWithPolicy(t *testing.T, rootContents string, fixtures map[string]discoveryFixture, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	discovery, err := discoverTestSyntaxWithPolicy(t, rootContents, fixtures, policy)
	if err != nil {
		return Schema{}, err
	}
	return newSchemaFromDiscoveryWithPolicy(discovery, policy)
}

func TestSchemaBridgeBuildsImmutableIntegerAndDecimalSimpleTypes(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="derived"><xs:restriction base="t:base"><xs:fractionDigits value="2"/><xs:totalDigits value="7"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="base"><xs:restriction base="xs:decimal"><xs:totalDigits value="7" fixed="true"/><xs:fractionDigits value="4"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="whole"><xs:restriction base="xs:integer"><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 3; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	assertSimpleTypeViewFacts(t, components)
	assertDerivedSimpleType(t, components, mustTestQName(t, "urn:test", "base"))
	assertDecimalSimpleType(t, components[1])
	assertIntegerSimpleType(t, components[2])
}

func assertSimpleTypeViewFacts(t *testing.T, components []Component) {
	t.Helper()
	for index, component := range components {
		if component.Kind() != ComponentKindSimpleTypeDefinition {
			t.Fatalf("component %d kind = %q, want simple type", index, component.Kind())
		}
		definition, definitionOK := component.SimpleType()
		if !definitionOK {
			t.Fatalf("component %d has no simple type view", index)
		}
		if definition.ID() != component.ID() || definition.Name() != component.Name() || definition.Loc() != component.Loc() {
			t.Fatalf("component %d type view does not preserve generic facts", index)
		}
		if definition.BaseLoc().IsZero() {
			t.Fatalf("component %d base location is zero", index)
		}
	}
}

func assertDerivedSimpleType(t *testing.T, components []Component, baseName QName) {
	t.Helper()
	derived, derivedOK := components[0].SimpleTypeDefinition()
	if !derivedOK {
		t.Fatal("derived simple type view is missing")
	}
	if got, want := derived.Base(), baseName; got != want {
		t.Fatalf("derived base = %q, want %q", got, want)
	}
	baseID, hasBaseID := derived.BaseID()
	if !hasBaseID || baseID != components[1].ID() {
		t.Fatalf("derived base ID = (%v, %t), want (%v, true)", baseID, hasBaseID, components[1].ID())
	}
	derivedFacets := derived.DigitFacets()
	derivedTotal, derivedTotalPresent := derivedFacets.TotalDigits()
	assertSchemaFacetValue(t, derivedTotal, derivedTotalPresent, "7", "derived totalDigits")
	derivedFraction, derivedFractionPresent := derivedFacets.FractionDigits()
	assertSchemaFacetValue(t, derivedFraction, derivedFractionPresent, "2", "derived fractionDigits")
	derivedTotalLoc, derivedTotalLocPresent := derivedFacets.TotalDigitsLoc()
	if !derivedTotalLocPresent || derivedTotalLoc.IsZero() || derivedTotalLoc.Source() != "root.xsd" {
		t.Fatalf("derived totalDigits location = (%v, %t), want a root.xsd location", derivedTotalLoc, derivedTotalLocPresent)
	}
}

func assertDecimalSimpleType(t *testing.T, component Component) {
	t.Helper()
	base, baseOK := component.SimpleType()
	if !baseOK {
		t.Fatal("base simple type view is missing")
	}
	if got, want := base.Base(), mustTestQName(t, testXSDNamespace, "decimal"); got != want {
		t.Fatalf("base built-in QName = %q, want %q", got, want)
	}
	if baseID, hasBaseID := base.BaseID(); hasBaseID || !baseID.IsZero() {
		t.Fatalf("base built-in ID = (%v, %t), want zero,false", baseID, hasBaseID)
	}
	baseFacets := base.DigitFacets()
	baseTotal, baseTotalPresent := baseFacets.TotalDigits()
	assertSchemaFacetValue(t, baseTotal, baseTotalPresent, "7", "base totalDigits")
	baseFraction, baseFractionPresent := baseFacets.FractionDigits()
	assertSchemaFacetValue(t, baseFraction, baseFractionPresent, "4", "base fractionDigits")
	if fixed, hasFixed := baseFacets.TotalDigitsFixed(); !hasFixed || !fixed {
		t.Fatalf("base totalDigits fixed = (%t, %t), want true,true", fixed, hasFixed)
	}
}

func assertIntegerSimpleType(t *testing.T, component Component) {
	t.Helper()
	whole, wholeOK := component.SimpleType()
	if !wholeOK {
		t.Fatal("integer simple type view is missing")
	}
	wholeFacets := whole.DigitFacets()
	wholeTotal, wholeTotalPresent := wholeFacets.TotalDigits()
	assertSchemaFacetValue(t, wholeTotal, wholeTotalPresent, "3", "integer totalDigits")
	wholeFraction, wholeFractionPresent := wholeFacets.FractionDigits()
	assertSchemaFacetValue(t, wholeFraction, wholeFractionPresent, "0", "integer fractionDigits")
	if fixed, hasFixed := wholeFacets.FractionDigitsFixed(); !hasFixed || !fixed {
		t.Fatalf("integer fractionDigits fixed = (%t, %t), want true,true", fixed, hasFixed)
	}
	if wholeFractionLoc, ok := wholeFacets.FractionDigitsLoc(); !ok || !wholeFractionLoc.IsZero() {
		t.Fatalf("integer inherited fractionDigits location = (%v, %t), want zero,true", wholeFractionLoc, ok)
	}
}

func TestSchemaBridgeResolvesForwardCrossDocumentSimpleTypeBases(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root" version="1.0">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="rootType"><xs:restriction base="o:otherType"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.1"><xs:simpleType name="otherType"><xs:restriction base="xs:integer"><xs:totalDigits value="8"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
	}, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	rootType, ok := components[0].SimpleType()
	if !ok {
		t.Fatal("root simple type view is missing")
	}
	baseID, ok := rootType.BaseID()
	if !ok || baseID.Source() != "other.xsd" || baseID.Ordinal() != 1 {
		t.Fatalf("cross-document base ID = (%v, %t), want other.xsd:1", baseID, ok)
	}
	rootTotal, rootTotalPresent := rootType.DigitFacets().TotalDigits()
	assertSchemaFacetValue(t, rootTotal, rootTotalPresent, "4", "cross-document totalDigits")
	rootFraction, rootFractionPresent := rootType.DigitFacets().FractionDigits()
	assertSchemaFacetValue(t, rootFraction, rootFractionPresent, "0", "inherited integer fractionDigits")
	if got, want := rootType.DigitFacets().Version(), XSDVersion10; got != want {
		t.Fatalf("cross-document facet version = %q, want %q", got, want)
	}
	baseType, ok := components[1].SimpleType()
	if !ok {
		t.Fatal("imported simple type view is missing")
	}
	if got, want := baseType.DigitFacets().Version(), XSDVersion10; got != want {
		t.Fatalf("imported facet version = %q, want %q", got, want)
	}
}

func TestSchemaBridgeUsesRestrictionNamespaceContextAndVersion(t *testing.T) {
	for _, version := range []XSDVersion{XSDVersion10, XSDVersion11} {
		t.Run(string(version), func(t *testing.T) {
			policy := Strict11
			if version == XSDVersion10 {
				policy = Strict10
			}
			assertRestrictionNamespaceContextAndVersion(t, version, policy)
		})
	}
}

func assertRestrictionNamespaceContextAndVersion(t *testing.T, version XSDVersion, policy LanguagePolicy) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:d="urn:not-the-datatype-namespace" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="item"><xs:restriction xmlns:d="` + testXSDNamespace + `" base="d:decimal"><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, definitionOK := schema.Components()[0].SimpleType()
	if !definitionOK {
		t.Fatal("simple type view is missing")
	}
	if got, want := definition.Base(), mustTestQName(t, testXSDNamespace, "decimal"); got != want {
		t.Fatalf("base QName = %q, want %q", got, want)
	}
	if got, want := definition.DigitFacets().Version(), version; got != want {
		t.Fatalf("facet version = %q, want %q", got, want)
	}
	if loc := definition.BaseLoc(); loc.IsZero() || loc.Source() != "root.xsd" {
		t.Fatalf("base location = %v, want root.xsd location", loc)
	}
}

type schemaSimpleTypeBaseFailureCase struct {
	name       string
	root       string
	fixtures   map[string]discoveryFixture
	policy     LanguagePolicy
	class      FailureClass
	code       string
	cause      error
	feature    FeatureID
	specRef    string
	relatedMin int
}

func TestSchemaBridgeRejectsSimpleTypeBaseFailuresWithoutSchema(t *testing.T) {
	tests := []schemaSimpleTypeBaseFailureCase{
		{
			name:    "unresolved",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing" version="1.0"><xs:simpleType name="item"><xs:restriction base="m:missing"/></xs:simpleType></xs:schema>`,
			policy:  Strict10,
			class:   FailureInvalid,
			code:    diagnosticSchemaSimpleTypeUnresolvedCode,
			cause:   errSchemaSimpleTypeBaseUnresolved,
			specRef: "xsd10-structures#Simple_Type_Definitions",
		},
		{
			name:  "malformed base QName",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="bad:base:QName"/></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaConditionalCode,
		},
		{
			name:       "wrong kind",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item"/><xs:simpleType name="derived"><xs:restriction base="item"/></xs:simpleType></xs:schema>`,
			class:      FailureInvalid,
			code:       diagnosticSchemaSimpleTypeWrongKindCode,
			cause:      errSchemaSimpleTypeBaseWrongKind,
			specRef:    "xsd11-structures#Simple_Type_Definition",
			relatedMin: 1,
		},
		{
			name:       "ambiguous",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="item"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="derived"><xs:restriction base="item"/></xs:simpleType></xs:schema>`,
			class:      FailureInvalid,
			code:       diagnosticSchemaGlobalDuplicateCode,
			cause:      errSchemaGlobalDeclarationDuplicate,
			specRef:    schemaGlobalDuplicateXSD11SpecRef,
			relatedMin: 1,
		},
		{
			name:       "cyclic",
			root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="one"><xs:restriction base="two"/></xs:simpleType><xs:simpleType name="two"><xs:restriction base="one"/></xs:simpleType></xs:schema>`,
			class:      FailureInvalid,
			code:       diagnosticSchemaSimpleTypeCycleCode,
			cause:      errSchemaSimpleTypeBaseCycle,
			specRef:    "xsd11-structures#Simple_Type_Definition",
			relatedMin: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSimpleTypeBaseFailure(t, test)
		})
	}
}

func assertSimpleTypeBaseFailure(t *testing.T, test schemaSimpleTypeBaseFailureCase) {
	t.Helper()
	policy := test.policy
	if policy == "" {
		policy = Compatibility
	}
	schema, err := discoverTestSchemaWithPolicy(t, test.root, test.fixtures, policy)
	if err == nil {
		t.Fatal("discoverSchema accepted an invalid or unsupported base")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
		t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
	}
	if test.cause != nil && !errors.Is(err, test.cause) {
		t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
	}
	if test.feature != "" && diagnostic.Feature() != test.feature {
		t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
	}
	if test.specRef != "" && diagnostic.SpecRef() != test.specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specRef)
	}
	if len(diagnostic.Related()) < test.relatedMin {
		t.Fatalf("related locations = %v, want at least %d", diagnostic.Related(), test.relatedMin)
	}
}

func TestSchemaBridgeReusesDigitFacetRestrictionDiagnostics(t *testing.T) {
	tests := []struct {
		name  string
		root  string
		class FailureClass
		code  string
	}{
		{
			name:  "non monotonic totalDigits",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="base"><xs:restriction base="xs:decimal"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType><xs:simpleType name="derived"><xs:restriction base="base"><xs:totalDigits value="6"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  InvalidDigitFacetRestrictionCode,
		},
		{
			name:  "fixed totalDigits",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="base"><xs:restriction base="xs:decimal"><xs:totalDigits value="5" fixed="true"/></xs:restriction></xs:simpleType><xs:simpleType name="derived"><xs:restriction base="base"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  InvalidDigitFacetRestrictionCode,
		},
		{
			name:  "cross facet",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="2"/><xs:fractionDigits value="3"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  InvalidDigitFacetCombinationCode,
		},
		{
			name:  "integer fractionDigits",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:integer"><xs:fractionDigits value="1"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  InvalidDigitFacetRestrictionCode,
		},
		{
			name:  "invalid totalDigits value",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="0"/></xs:restriction></xs:simpleType></xs:schema>`,
			class: FailureInvalid,
			code:  InvalidTotalDigitsCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an invalid digit restriction")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
			}
			if diagnostic.Loc().IsZero() {
				t.Fatal("digit diagnostic lost its primary location")
			}
		})
	}
}

func TestSchemaBridgeReportsUnsupportedSimpleTypeFeatures(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		policy  LanguagePolicy
		feature FeatureID
		code    string
		specRef string
	}{
		{
			name:    "XSD 1.0 datatype facet",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.0"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType></xs:schema>`,
			policy:  Strict10,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
			specRef: "xsd10-datatypes#decimal",
		},
		{
			name:    "XSD 1.1 datatype facet",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType></xs:schema>`,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
			specRef: "xsd11-datatypes#decimal",
		},
		{
			name:    "foreign facet remains schema syntax",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><f:pattern/></xs:restriction></xs:simpleType></xs:schema>`,
			feature: FeatureSchemaSyntax,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertUnsupportedSimpleTypeFeature(t, test.root, test.policy, test.feature, test.code, test.specRef)
		})
	}
}

func assertUnsupportedSimpleTypeFeature(t *testing.T, root string, policy LanguagePolicy, feature FeatureID, code, specRef string) {
	t.Helper()
	if policy == "" {
		policy = Compatibility
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discoverSchema accepted unsupported simple type behavior")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported {
		t.Fatalf("diagnostic class = %q, want unsupported", diagnostic.Class())
	}
	if diagnostic.Feature() != feature {
		t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), feature)
	}
	if code != "" && diagnostic.Code() != code {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), code)
	}
	if specRef != "" && diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
	}
}

//nolint:gocognit // Keep the named list/union build-boundary matrix together.
func TestSchemaBridgeBuildsNamedSimpleTypeModelsAtBuildBoundary(t *testing.T) {
	models := []struct {
		name     string
		typeName string
		child    string
	}{
		{name: "list", typeName: "List", child: `<xs:list itemType="xs:boolean"/>`},
		{name: "union", typeName: "Union", child: `<xs:union memberTypes="xs:boolean"/>`},
	}
	versions := []struct {
		name    string
		version XSDVersion
		specRef string
	}{
		{name: "XSD 1.0", version: XSDVersion10, specRef: "xsd10-structures#schema-document"},
		{name: "XSD 1.1", version: XSDVersion11, specRef: "xsd11-structures#cSchemaDocument"},
	}
	for _, model := range models {
		for _, version := range versions {
			t.Run(model.name+"/"+version.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="` + model.typeName + `">` + model.child + `</xs:simpleType></xs:schema>`
				simpleType := schemaBuildTestRootChild(t, root)
				declaration, present, err := schemaDocumentDeclaration(simpleType, "", version.version)
				if err != nil {
					t.Fatalf("schemaDocumentDeclaration: %v", err)
				}
				if !present || declaration.simpleType == nil {
					t.Fatal("schemaDocumentDeclaration did not retain the named simple type model")
				}
				switch model.name {
				case "list":
					if _, ok := declaration.simpleType.model.(*schemaSimpleTypeListModelInput); !ok {
						t.Fatalf("simple type model = %T, want list", declaration.simpleType.model)
					}
				case "union":
					if _, ok := declaration.simpleType.model.(*schemaSimpleTypeUnionModelInput); !ok {
						t.Fatalf("simple type model = %T, want union", declaration.simpleType.model)
					}
				}
			})
		}
	}
}

func TestSchemaBridgeKeepsMalformedNamedSimpleTypeModelsInvalidAtBuildBoundary(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "missing source",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="List"><xs:list/></xs:simpleType></xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "malformed itemType",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="List"><xs:list itemType="bad:q:name"/></xs:simpleType></xs:schema>`,
			code: invalidSchemaConditionalCode,
		},
		{
			name: "both sources",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="List"><xs:list itemType="xs:boolean"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:list></xs:simpleType></xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "union missing source",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Union"><xs:union/></xs:simpleType></xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "union malformed memberTypes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Union"><xs:union memberTypes="bad:q:name"/></xs:simpleType></xs:schema>`,
			code: invalidSchemaConditionalCode,
		},
		{
			name: "union memberTypes is empty",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Union"><xs:union memberTypes=""/></xs:simpleType></xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			simpleType := schemaBuildTestRootChild(t, test.root)
			_, present, err := schemaDocumentDeclaration(simpleType, "", XSDVersion11)
			if err == nil {
				t.Fatal("schemaDocumentDeclaration accepted a malformed named list")
			}
			if present {
				t.Fatal("schemaDocumentDeclaration returned a declaration with an error")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if errors.Is(err, ErrUnsupported) {
				t.Fatalf("malformed list retained an unsupported classification: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep both strict-policy inline boolean cases together.
func TestSchemaBridgeBuildsGlobalInlineBooleanSimpleType(t *testing.T) {
	tests := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "XSD 1.0", policy: Strict10},
		{name: "XSD 1.1", policy: Strict11},
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="root"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element>
</xs:schema>`
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			elements := schema.Components()
			if len(elements) != 1 {
				t.Fatalf("component count = %d, want 1", len(elements))
			}
			declaration, ok := elements[0].ElementDeclaration()
			if !ok {
				t.Fatal("global element view is missing")
			}
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsAnonymous() {
				t.Fatalf("type reference = %#v/%t, want anonymous", reference, ok)
			}
			anonymous, ok := reference.AnonymousType()
			if !ok || !anonymous.IsBoolean() {
				t.Fatalf("anonymous boolean model = %#v/%t, want boolean", anonymous, ok)
			}
		})
	}
}

func TestSchemaBridgeKeepsMalformedGlobalInlineSimpleTypesInvalidAtBuildBoundary(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "empty simpleType",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="root"><xs:simpleType/></xs:element></xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "malformed restriction base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="root"><xs:simpleType><xs:restriction base="bad:q:name"/></xs:simpleType></xs:element></xs:schema>`,
			code: invalidSchemaConditionalCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			element := schemaBuildTestRootChild(t, test.root)
			_, present, err := schemaDocumentDeclaration(element, "", XSDVersion11)
			if err == nil {
				t.Fatal("schemaDocumentDeclaration accepted a malformed global inline simple type")
			}
			if present {
				t.Fatal("schemaDocumentDeclaration returned a declaration with an error")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if errors.Is(err, ErrUnsupported) {
				t.Fatalf("malformed inline simple type retained an unsupported classification: %v", err)
			}
		})
	}
}

func schemaBuildTestRootChild(t *testing.T, root string) *syntaxElement {
	t.Helper()
	document, err := decodeResolvedSyntaxForDiscovery(newTestSource(t, &trackingSource{data: []byte(root)}))
	if err != nil {
		t.Fatalf("decodeResolvedSyntaxForDiscovery: %v", err)
	}
	for _, node := range document.root.children {
		child, ok := node.(*syntaxElement)
		if ok {
			return child
		}
	}
	t.Fatal("schema test root has no element child")
	return nil
}

func TestSchemaBridgeBuildsInlineSimpleTypeRestriction(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:restriction></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, ok := schema.Components()[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("simple type view is missing")
	}
	base, ok := definition.BaseReference()
	if !ok || !base.IsAnonymous() {
		t.Fatalf("base reference = %#v, want anonymous", base)
	}
	anonymous, ok := base.AnonymousType()
	if !ok || !anonymous.IsAnonymous() || anonymous.Base().Local() != "integer" {
		t.Fatalf("anonymous base model = %#v/%t", anonymous, ok)
	}
	if definition.Base().IsZero() == false {
		t.Fatal("anonymous base unexpectedly exposed a named QName")
	}
}

func TestSchemaBridgeBuildsTypedGlobalElementsWithoutSyntheticBuiltinIDs(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:element name="whole" type="  xs:integer  "/>
  <xs:element name="amount" type=" t:Amount "/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 3; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	whole, ok := components[0].Element()
	if !ok {
		t.Fatal("built-in integer element view is missing")
	}
	if got, want := whole.DeclaredType(), mustTestQName(t, testXSDNamespace, "integer"); got != want {
		t.Fatalf("integer declared type = %q, want %q", got, want)
	}
	if typeID, hasTypeID := whole.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("integer type ID = (%v, %t), want zero,false", typeID, hasTypeID)
	}
	amount, ok := components[1].ElementDeclaration()
	if !ok {
		t.Fatal("named decimal element view is missing")
	}
	if got, want := amount.DeclaredType(), mustTestQName(t, "urn:test", "Amount"); got != want {
		t.Fatalf("named declared type = %q, want %q", got, want)
	}
	targetID := components[2].ID()
	if got, hasTypeID := amount.TypeID(); !hasTypeID || got != targetID {
		t.Fatalf("named type ID = (%v, %t), want (%v, true)", got, hasTypeID, targetID)
	}
	if got := amount.Component().ID(); got != components[1].ID() {
		t.Fatalf("element view component ID = %v, want %v", got, components[1].ID())
	}
	if got := amount.Name(); got != components[1].Name() {
		t.Fatalf("element view name = %q, want %q", got, components[1].Name())
	}
	if got := amount.Loc(); got != components[1].Loc() {
		t.Fatalf("element view location = %s, want %s", got, components[1].Loc())
	}
	if _, ok := components[2].Element(); ok {
		t.Fatal("simple type unexpectedly has an element view")
	}
}

//nolint:gocognit,funlen // Keep ordered facts, identity isolation, and immutability together.
func TestSchemaBridgeBuildsOrderedChoiceParticlesWithoutLocalComponentIDs(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element name="first" type="xs:integer"/>
      <xs:element name="second" type="t:Amount"/>
      <xs:element name="third" type="xs:decimal"/>
    </xs:choice>
  </xs:complexType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("global component count = %d, want %d", got, want)
	}
	if components[0].ID().Ordinal() != 1 || components[1].ID().Ordinal() != 2 {
		t.Fatalf("global component IDs = %v, %v, want ordinals 1,2", components[0].ID(), components[1].ID())
	}
	if got := schema.Find(mustTestQName(t, "", "first")); len(got) != 0 {
		t.Fatalf("local element is globally discoverable: %v", got)
	}
	if _, ok := schema.Lookup(ComponentID{source: "root.xsd", ordinal: 3}); ok {
		t.Fatal("local element received a global component ID")
	}

	definition, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("complex type view is missing")
	}
	alias, aliasOK := components[0].ComplexTypeDefinition()
	if !aliasOK || alias.ID() != definition.ID() {
		t.Fatal("complex type aliases do not expose the same component")
	}
	particle := definition.Particle()
	choice, ok := particle.(ChoiceParticle)
	if !ok {
		t.Fatalf("particle type = %T, want ChoiceParticle", particle)
	}
	if choice.Loc() != mustTestLoc(t, "root.xsd", 3, 5) {
		t.Fatalf("choice location = %s, want root.xsd:3:5", choice.Loc())
	}
	if choice.MinOccurs() != 1 || choice.MaxOccurs() != 1 {
		t.Fatalf("choice occurrence bounds = %d/%d, want 1/1", choice.MinOccurs(), choice.MaxOccurs())
	}
	alternatives := choice.Alternatives()
	if got, want := len(alternatives), 3; got != want {
		t.Fatalf("alternative count = %d, want %d", got, want)
	}
	alternatives[0] = nil
	if len(choice.Alternatives()) != 3 {
		t.Fatal("mutating alternatives changed the completed choice")
	}
	wantNames := []string{"first", "second", "third"}
	for index, alternative := range choice.Alternatives() {
		element, ok := alternative.(ElementParticle)
		if !ok {
			t.Fatalf("alternative %d type = %T, want ElementParticle", index, alternative)
		}
		if element.Name() != mustTestQName(t, "", wantNames[index]) {
			t.Fatalf("alternative %d name = %q, want no-namespace %q", index, element.Name(), wantNames[index])
		}
		if element.MinOccurs() != 1 || element.MaxOccurs() != 1 {
			t.Fatalf("alternative %d occurrence bounds = %d/%d, want 1/1", index, element.MinOccurs(), element.MaxOccurs())
		}
		if element.Loc() != mustTestLoc(t, "root.xsd", 4+index, 7) {
			t.Fatalf("alternative %d location = %s, want root.xsd:%d:7", index, element.Loc(), 4+index)
		}
		switch index {
		case 0:
			if element.DeclaredType() != mustTestQName(t, testXSDNamespace, "integer") {
				t.Fatalf("integer declared type = %q", element.DeclaredType())
			}
			if typeID, hasTypeID := element.TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("built-in type ID = (%v, %t), want zero,false", typeID, hasTypeID)
			}
		case 1:
			if element.DeclaredType() != mustTestQName(t, "urn:root", "Amount") {
				t.Fatalf("named declared type = %q", element.DeclaredType())
			}
			if typeID, hasTypeID := element.TypeID(); !hasTypeID || typeID != components[1].ID() {
				t.Fatalf("named type ID = (%v, %t), want (%v, true)", typeID, hasTypeID, components[1].ID())
			}
		case 2:
			if element.DeclaredType() != mustTestQName(t, testXSDNamespace, "decimal") {
				t.Fatalf("decimal declared type = %q", element.DeclaredType())
			}
		}
	}
}

func TestSchemaBridgeAcceptsEmptyChoiceParticle(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Empty"><xs:choice/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, ok := schema.Components()[0].ComplexType()
	if !ok {
		t.Fatal("empty choice complex type view is missing")
	}
	choice, ok := definition.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("particle type = %T, want ChoiceParticle", definition.Particle())
	}
	if alternatives := choice.Alternatives(); len(alternatives) != 0 {
		t.Fatalf("empty choice alternatives = %d, want zero", len(alternatives))
	}
}

func TestSchemaBridgeResolvesForwardCrossDocumentChoiceElementTypes(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="value" type="o:Amount"/></xs:choice></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`,
		},
	})
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("global component count = %d, want %d", got, want)
	}
	choice, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("cross-document complex type view is missing")
	}
	particle, ok := choice.Particle().(ChoiceParticle)
	if !ok {
		t.Fatal("cross-document particle type is not ChoiceParticle")
	}
	alternatives := particle.Alternatives()
	if len(alternatives) != 1 {
		t.Fatalf("cross-document alternative count = %d, want 1", len(alternatives))
	}
	alternative, ok := alternatives[0].(ElementParticle)
	if !ok {
		t.Fatal("cross-document alternative type is not ElementParticle")
	}
	if typeID, hasTypeID := alternative.TypeID(); !hasTypeID || typeID != components[1].ID() {
		t.Fatalf("cross-document type ID = (%v, %t), want (%v, true)", typeID, hasTypeID, components[1].ID())
	}
}

//nolint:gocognit,funlen // Keep target classification, locations, causes, and refs together.
func TestSchemaBridgeRejectsGlobalElementTypeTargetsWithoutSchema(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		policy      LanguagePolicy
		class       FailureClass
		code        string
		feature     FeatureID
		specRef     string
		cause       error
		primary     Loc
		related     []Loc
		unsupported bool
	}{
		{
			name: "unresolved",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing" version="1.0">
  <xs:element name="item" type="m:Missing"/>
</xs:schema>`,
			policy:  Strict10,
			class:   FailureInvalid,
			code:    diagnosticSchemaElementTypeUnresolvedCode,
			specRef: schemaElementTypeXSD10SpecRef,
			cause:   errSchemaElementTypeUnresolved,
			primary: mustTestLoc(t, "root.xsd", 2, 27),
		},
		{
			name: "malformed QName",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" type="bad:Type:extra"/>
</xs:schema>`,
			class:   FailureInvalid,
			code:    invalidSchemaConditionalCode,
			primary: mustTestLoc(t, "root.xsd", 2, 27),
		},
		{
			name: "unbound QName",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" type="missing:Type"/>
</xs:schema>`,
			class:   FailureInvalid,
			code:    invalidSchemaConditionalCode,
			primary: mustTestLoc(t, "root.xsd", 2, 27),
		},
		{
			name: "empty QName",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" type="   "/>
</xs:schema>`,
			class:   FailureInvalid,
			code:    invalidSchemaCompositionCode,
			primary: mustTestLoc(t, "root.xsd", 2, 27),
		},
		{
			name: "wrong kind",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="candidate"/>
  <xs:element name="item" type="candidate"/>
</xs:schema>`,
			class:   FailureInvalid,
			code:    diagnosticSchemaElementTypeWrongKindCode,
			specRef: schemaElementTypeXSD11SpecRef,
			cause:   errSchemaElementTypeWrongKind,
			primary: mustTestLoc(t, "root.xsd", 3, 27),
			related: []Loc{mustTestLoc(t, "root.xsd", 2, 3)},
		},
		{
			name: "ambiguous",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:element name="item" type="Amount"/>
</xs:schema>`,
			class:   FailureInvalid,
			code:    diagnosticSchemaGlobalDuplicateCode,
			specRef: schemaGlobalDuplicateXSD11SpecRef,
			cause:   errSchemaGlobalDeclarationDuplicate,
			primary: mustTestLoc(t, "root.xsd", 3, 3),
			related: []Loc{mustTestLoc(t, "root.xsd", 2, 3)},
		},
		{
			name: "simple and complex definitions are ambiguous",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:complexType name="Amount"/>
  <xs:element name="item" type="Amount"/>
</xs:schema>`,
			class:   FailureInvalid,
			code:    diagnosticSchemaGlobalDuplicateCode,
			specRef: schemaGlobalDuplicateXSD11SpecRef,
			cause:   errSchemaGlobalDeclarationDuplicate,
			primary: mustTestLoc(t, "root.xsd", 3, 3),
			related: []Loc{mustTestLoc(t, "root.xsd", 2, 3)},
		},
		{
			name: "named complex type is unsupported",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="Complex"/>
  <xs:element name="item" type="Complex"/>
</xs:schema>`,
			class:       FailureUnsupported,
			feature:     FeatureSchemaSyntax,
			primary:     mustTestLoc(t, "root.xsd", 3, 27),
			unsupported: true,
		},
		{
			name: "string built-in is unsupported",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.0">
  <xs:element name="item" type="xs:string"/>
</xs:schema>`,
			class:       FailureUnsupported,
			feature:     FeatureSchemaSyntax,
			primary:     mustTestLoc(t, "root.xsd", 2, 27),
			unsupported: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := test.policy
			if policy == "" {
				policy = Compatibility
			}
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
			if err == nil {
				t.Fatal("discoverSchema accepted an invalid or unsupported element type")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
			if test.code != "" && diagnostic.Code() != test.code {
				t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
			}
			if test.feature != "" && diagnostic.Feature() != test.feature {
				t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
			}
			if test.specRef != "" && diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specRef)
			}
			if diagnostic.Loc() != test.primary {
				t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), test.primary)
			}
			if test.related != nil && !reflect.DeepEqual(diagnostic.Related(), test.related) {
				t.Fatalf("related locations = %v, want %v", diagnostic.Related(), test.related)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
			}
			if test.unsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
			}
		})
	}
}

func TestSchemaBridgePreservesExistingElementExclusions(t *testing.T) {
	tests := []struct {
		name  string
		root  string
		class FailureClass
	}{
		{
			name:  "type and inline type",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer"><xs:simpleType/></xs:element></xs:schema>`,
			class: FailureInvalid,
		},
		{
			name:  "type and default",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" default="1"/></xs:schema>`,
			class: FailureUnsupported,
		},
		{
			name:  "default and fixed remain invalid",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" default="1" fixed="2"/></xs:schema>`,
			class: FailureInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an excluded element construct")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
			if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported exclusion does not match ErrUnsupported: %v", err)
			}
		})
	}
}

func TestSchemaBridgeDoesNotCompleteElementFromFailedSimpleType(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" type="Amount"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:fractionDigits value="-1"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err == nil {
		t.Fatal("discoverSchema accepted a failed named element type")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != InvalidFractionDigitsCode {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), InvalidFractionDigitsCode)
	}
}

//nolint:gocognit,funlen // Keep typed accessors, effective bounds, and ownership checks together.
func TestSchemaBridgeBuildsTypedOrderedBoundsAndImmutableAccessors(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="intMinInclusive"><xs:restriction base="xs:integer"><xs:minInclusive value="10" fixed="true"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="intMinExclusive"><xs:restriction base="xs:integer"><xs:minExclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="intMaxInclusive"><xs:restriction base="xs:integer"><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="intMaxExclusive"><xs:restriction base="xs:integer"><xs:maxExclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="decimalMinInclusive"><xs:restriction base="xs:decimal"><xs:minInclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="decimalMinExclusive"><xs:restriction base="xs:decimal"><xs:minExclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="decimalMaxInclusive"><xs:restriction base="xs:decimal"><xs:maxInclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="decimalMaxExclusive"><xs:restriction base="xs:decimal"><xs:maxExclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="emptyInteger"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="emptyDecimal"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}

	tests := []struct {
		name        string
		integer     bool
		kind        BoundKind
		value       string
		fixed       bool
		wantValue   string
		wantVersion XSDVersion
	}{
		{name: "intMinInclusive", integer: true, kind: BoundMinInclusive, value: "10", fixed: true, wantValue: "10", wantVersion: XSDVersion11},
		{name: "intMinExclusive", integer: true, kind: BoundMinExclusive, value: "10", wantValue: "10", wantVersion: XSDVersion11},
		{name: "intMaxInclusive", integer: true, kind: BoundMaxInclusive, value: "10", wantValue: "10", wantVersion: XSDVersion11},
		{name: "intMaxExclusive", integer: true, kind: BoundMaxExclusive, value: "10", wantValue: "10", wantVersion: XSDVersion11},
		{name: "decimalMinInclusive", kind: BoundMinInclusive, value: "10.00", wantValue: "10.0", wantVersion: XSDVersion11},
		{name: "decimalMinExclusive", kind: BoundMinExclusive, value: "10.00", wantValue: "10.0", wantVersion: XSDVersion11},
		{name: "decimalMaxInclusive", kind: BoundMaxInclusive, value: "10.00", wantValue: "10.0", wantVersion: XSDVersion11},
		{name: "decimalMaxExclusive", kind: BoundMaxExclusive, value: "10.00", wantValue: "10.0", wantVersion: XSDVersion11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			component := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", test.name))
			if len(component) != 1 {
				t.Fatalf("simple type matches = %d, want 1", len(component))
			}
			definition, ok := component[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("simple type view is missing")
			}
			if test.integer {
				bounds, present := definition.IntegerBounds()
				if !present || bounds.Version() != test.wantVersion {
					t.Fatalf("IntegerBounds() = %v/%t, want version %q and present", bounds.Version(), present, test.wantVersion)
				}
				if _, present := definition.DecimalBounds(); present {
					t.Fatal("decimal bounds unexpectedly present on integer type")
				}
				integer := bounds.Bounds()
				if len(integer) != 1 || integer[0].Kind() != test.kind || integer[0].Value().Canonical() != test.wantValue || integer[0].Fixed() != test.fixed || integer[0].Loc().IsZero() {
					t.Fatalf("integer bounds = %#v, want one exact located bound", integer)
				}
				first := bounds.Bounds()
				first[0] = first[0].WithFixed(!test.fixed)
				second := bounds.Bounds()
				if second[0].Fixed() != test.fixed {
					t.Fatal("mutating returned integer bound slice changed schema state")
				}
				return
			}

			bounds, present := definition.DecimalBounds()
			if !present || bounds.Version() != test.wantVersion {
				t.Fatalf("DecimalBounds() = %v/%t, want version %q and present", bounds.Version(), present, test.wantVersion)
			}
			if _, present := definition.IntegerBounds(); present {
				t.Fatal("integer bounds unexpectedly present on decimal type")
			}
			decimal := bounds.Bounds()
			if len(decimal) != 1 || decimal[0].Kind() != test.kind || decimal[0].Value().Canonical() != test.wantValue || decimal[0].Fixed() != test.fixed || decimal[0].Loc().IsZero() {
				t.Fatalf("decimal bounds = %#v, want one exact located bound", decimal)
			}
			first := bounds.Bounds()
			first[0] = first[0].WithFixed(!test.fixed)
			second := bounds.Bounds()
			if second[0].Fixed() != test.fixed {
				t.Fatal("mutating returned decimal bound slice changed schema state")
			}
		})
	}

	for _, test := range []struct {
		name    string
		kind    DigitDatatype
		version XSDVersion
	}{
		{name: "emptyInteger", kind: DigitDatatypeInteger, version: XSDVersion11},
		{name: "emptyDecimal", kind: DigitDatatypeDecimal, version: XSDVersion11},
	} {
		component := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", test.name))
		if len(component) != 1 {
			t.Fatalf("%s matches = %d, want 1", test.name, len(component))
		}
		definition, ok := component[0].SimpleTypeDefinition()
		if !ok {
			t.Fatalf("%s simple type view is missing", test.name)
		}
		if definition.DigitFacets().Kind() != test.kind {
			t.Fatalf("%s digit kind = %q, want %q", test.name, definition.DigitFacets().Kind(), test.kind)
		}
		if test.kind == DigitDatatypeInteger {
			bounds, present := definition.IntegerBounds()
			if !present || bounds.Version() != test.version || len(bounds.Bounds()) != 0 {
				t.Fatalf("%s empty integer bounds = %v/%t, want version %q and no bounds", test.name, bounds.Version(), present, test.version)
			}
			continue
		}
		bounds, present := definition.DecimalBounds()
		if !present || bounds.Version() != test.version || len(bounds.Bounds()) != 0 {
			t.Fatalf("%s empty decimal bounds = %v/%t, want version %q and no bounds", test.name, bounds.Version(), present, test.version)
		}
	}
}

//nolint:gocognit // Keep cross-document effective-bound evidence together.
func TestSchemaBridgeInheritsBoundsAcrossForwardAndImportedBases(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="1.0">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="ForwardDerived"><xs:restriction base="r:ForwardBase"><xs:maxExclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ForwardBase"><xs:restriction base="xs:integer"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="CrossDerived"><xs:restriction base="o:CrossBase"><xs:minExclusive value="-1.00"/></xs:restriction></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.1">
  <xs:simpleType name="CrossBase"><xs:restriction base="xs:decimal"><xs:maxInclusive value="100.00"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}

	forward := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "ForwardDerived"))
	if len(forward) != 1 {
		t.Fatalf("ForwardDerived matches = %d, want 1", len(forward))
	}
	forwardDefinition, ok := forward[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("ForwardDerived simple type view is missing")
	}
	forwardBounds, present := forwardDefinition.IntegerBounds()
	if !present || len(forwardBounds.Bounds()) != 2 {
		t.Fatalf("ForwardDerived bounds = %#v/%t, want inherited lower and local upper", forwardBounds.Bounds(), present)
	}
	if got := forwardBounds.Bounds()[0].Value().Canonical(); got != "10" {
		t.Fatalf("ForwardDerived lower bound = %q, want 10", got)
	}
	if got := forwardBounds.Bounds()[1].Value().Canonical(); got != "100" {
		t.Fatalf("ForwardDerived upper bound = %q, want 100", got)
	}
	baseID, baseIDPresent := forwardDefinition.BaseID()
	if !baseIDPresent || baseID.Source() != "root.xsd" || baseID.Ordinal() != 2 {
		t.Fatalf("ForwardDerived base ID = %v/%t, want root.xsd:2", baseID, baseIDPresent)
	}

	cross := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "CrossDerived"))
	if len(cross) != 1 {
		t.Fatalf("CrossDerived matches = %d, want 1", len(cross))
	}
	crossDefinition, ok := cross[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("CrossDerived simple type view is missing")
	}
	crossBounds, present := crossDefinition.DecimalBounds()
	if !present || len(crossBounds.Bounds()) != 2 {
		t.Fatalf("CrossDerived bounds = %#v/%t, want local lower and inherited upper", crossBounds.Bounds(), present)
	}
	if got := crossBounds.Bounds()[0].Value().Canonical(); got != "-1.0" {
		t.Fatalf("CrossDerived lower bound = %q, want -1.0", got)
	}
	if got := crossBounds.Bounds()[1].Value().Canonical(); got != "100.0" {
		t.Fatalf("CrossDerived upper bound = %q, want 100.0", got)
	}
	crossBaseID, crossBaseIDPresent := crossDefinition.BaseID()
	if !crossBaseIDPresent || crossBaseID.Source() != "other.xsd" || crossBaseID.Ordinal() != 1 {
		t.Fatalf("CrossDerived base ID = %v/%t, want other.xsd:1", crossBaseID, crossBaseIDPresent)
	}
}

//nolint:gocognit // Keep bound diagnostic and no-partial-schema checks together.
func TestSchemaBridgeRejectsInvalidBoundRestrictionsWithoutSchema(t *testing.T) {
	tests := []struct {
		name  string
		root  string
		code  string
		cause error
	}{
		{
			name:  "opposite lower bounds in one step",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:integer"><xs:minInclusive value="5"/><xs:minExclusive value="6"/></xs:restriction></xs:simpleType></xs:schema>`,
			code:  InvalidBoundCombinationCode,
			cause: errInvalidBoundCombination,
		},
		{
			name:  "empty equal exclusive interval",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:minInclusive value="5.00"/><xs:maxExclusive value="5.0"/></xs:restriction></xs:simpleType></xs:schema>`,
			code:  InvalidBoundCombinationCode,
			cause: errInvalidBoundCombination,
		},
		{
			name:  "less restrictive derived lower bound",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="base"><xs:restriction base="xs:integer"><xs:minInclusive value="5"/></xs:restriction></xs:simpleType><xs:simpleType name="derived"><xs:restriction base="base"><xs:minInclusive value="4"/></xs:restriction></xs:simpleType></xs:schema>`,
			code:  InvalidBoundRestrictionCode,
			cause: errInvalidBoundRestriction,
		},
		{
			name:  "fixed derived upper bound changes value",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="base"><xs:restriction base="xs:decimal"><xs:maxExclusive value="5.00" fixed="true"/></xs:restriction></xs:simpleType><xs:simpleType name="derived"><xs:restriction base="base"><xs:maxExclusive value="4.00"/></xs:restriction></xs:simpleType></xs:schema>`,
			code:  InvalidBoundRestrictionCode,
			cause: errInvalidBoundRestriction,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an invalid bound restriction")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if diagnostic.Loc().IsZero() || len(diagnostic.Related()) == 0 {
				t.Fatalf("diagnostic evidence = %s/%v, want primary and related locations", diagnostic, diagnostic.Related())
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
			}
		})
	}
}

func assertSchemaFacetValue(t *testing.T, value StrictInteger, present bool, want, label string) {
	t.Helper()
	if !present {
		t.Fatalf("%s is absent", label)
	}
	if got := value.Canonical(); got != want {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
}
