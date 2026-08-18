package goxsd9

import (
	"context"
	"errors"
	"testing"
)

//nolint:gocognit,funlen // Keep the complete ordered component contract in one regression test.
func TestDiscoverSchemaBuildsOrderedImmutableDeclarations(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="rootElement"/>
  <xs:include schemaLocation="child.xsd"/>
  <xs:attribute name="rootAttribute"/>
  <xs:simpleType name="rootSimple"/>
  <xs:complexType name="rootComplex"/>
  <xs:group name="rootGroup"/>
  <xs:attributeGroup name="rootAttributes"/>
  <xs:notation name="rootNotation"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
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
  <xs:simpleType name="otherSimple"/>
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
		{source: "root.xsd", ordinal: 1, kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "rootElement"), loc: mustTestLoc(t, "root.xsd", 2, 3)},
		{source: "root.xsd", ordinal: 2, kind: ComponentKindAttributeDeclaration, name: mustTestQName(t, "urn:root", "rootAttribute"), loc: mustTestLoc(t, "root.xsd", 4, 3)},
		{source: "root.xsd", ordinal: 3, kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:root", "rootSimple"), loc: mustTestLoc(t, "root.xsd", 5, 3)},
		{source: "root.xsd", ordinal: 4, kind: ComponentKindComplexTypeDefinition, name: mustTestQName(t, "urn:root", "rootComplex"), loc: mustTestLoc(t, "root.xsd", 6, 3)},
		{source: "root.xsd", ordinal: 5, kind: ComponentKindModelGroupDefinition, name: mustTestQName(t, "urn:root", "rootGroup"), loc: mustTestLoc(t, "root.xsd", 7, 3)},
		{source: "root.xsd", ordinal: 6, kind: ComponentKindAttributeGroupDefinition, name: mustTestQName(t, "urn:root", "rootAttributes"), loc: mustTestLoc(t, "root.xsd", 8, 3)},
		{source: "root.xsd", ordinal: 7, kind: ComponentKindNotationDeclaration, name: mustTestQName(t, "urn:root", "rootNotation"), loc: mustTestLoc(t, "root.xsd", 9, 3)},
		{source: "child.xsd", ordinal: 1, kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "childElement"), loc: mustTestLoc(t, "child.xsd", 2, 3)},
		{source: "other.xsd", ordinal: 1, kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:other", "otherSimple"), loc: mustTestLoc(t, "other.xsd", 2, 3)},
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
			name: "import without parent namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import schemaLocation="child.xsd"/></xs:schema>`,
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"/>`},
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

func TestSchemaBridgeRejectsConditionalCompositionAsUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="http://www.w3.org/2007/XMLSchema-versioning" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd" vc:minVersion="1.1"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"/>`},
	})
	if err == nil {
		t.Fatal("discoverSchema accepted conditional composition")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("conditional composition diagnostic = %s, want schema syntax unsupported", diagnostic)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatal("conditional composition diagnostic does not match ErrUnsupported")
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
