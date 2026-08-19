package goxsd9

import (
	"context"
	"errors"
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
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("redefine diagnostic = %s, want schema syntax unsupported", diagnostic)
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

//nolint:gocognit,funlen // Keep the direct grammar and attribute matrix explicit.
func TestSchemaBridgeCoversDirectGrammarAndAttributeBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		class   FailureClass
		feature FeatureID
		code    string
	}{
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
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:defaultOpenContent/></xs:schema>`,
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
			name:    "complex type sequence is unsupported",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:sequence/></xs:complexType></xs:schema>`,
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid or unsupported direct syntax")
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
	tests := []struct {
		name  string
		root  string
		class FailureClass
	}{
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid or unsupported lexical syntax")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic source = %q, want root.xsd", diagnostic.Loc().Source())
			}
		})
	}
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
	tests := []struct {
		name  string
		root  string
		class FailureClass
	}{
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
			name:  "simple type model is unsupported after grammar validation",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:string"/></xs:simpleType></xs:schema>`,
			class: FailureUnsupported,
		},
		{
			name:  "group model is unsupported after grammar validation",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="item"><xs:sequence/></xs:group></xs:schema>`,
			class: FailureUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid or unsupported global child model")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
		})
	}
}

func discoverTestSchema(t *testing.T, rootContents string, fixtures map[string]discoveryFixture) (Schema, error) {
	t.Helper()
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootContents)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	resolver := &discoveryResolver{fixtures: fixtures}
	return discoverSchema(root, resolver)
}
