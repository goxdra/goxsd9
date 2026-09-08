package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeGlobalNamedOpenContentNoneMatchesOmission(t *testing.T) {
	const openContent = `<xs:openContent mode="none"><xs:annotation/></xs:openContent>`
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "sequence", body: `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`},
	}
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	}
	queryName := mustTestQName(t, "urn:test", "Item")
	for _, test := range tests {
		for _, profile := range policies {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				absentRoot := schemaGlobalNamedOpenContentRoot(strings.Repeat(" ", len(openContent)), test.body)
				noneRoot := schemaGlobalNamedOpenContentRoot(openContent, test.body)
				absentSchema, err := discoverTestSchemaWithPolicy(t, absentRoot, nil, profile.policy)
				if err != nil {
					t.Fatalf("omitted openContent: %v", err)
				}
				noneSchema, err := discoverTestSchemaWithPolicy(t, noneRoot, nil, profile.policy)
				if err != nil {
					t.Fatalf("mode=none openContent: %v", err)
				}
				absentSnapshot := snapshotSchemaForComponentKind(t, absentSchema, queryName, ComponentKindComplexTypeDefinition)
				noneSnapshot := snapshotSchemaForComponentKind(t, noneSchema, queryName, ComponentKindComplexTypeDefinition)
				if !reflect.DeepEqual(noneSnapshot, absentSnapshot) {
					t.Fatalf("public component snapshot differs: mode=none=%#v omitted=%#v", noneSnapshot, absentSnapshot)
				}
				assertGlobalNamedOpenContentFacts(t, noneSchema, test.body != "")
			})
		}
	}
}

func TestSchemaBridgeGlobalNamedOpenContentNoneHonorsStrict10Policy(t *testing.T) {
	root := schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="none"/>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted openContent mode=none")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("Strict10 returned a partial schema for openContent mode=none")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want located schema-syntax unsupported", diagnostic)
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "<xs:openContent") {
		t.Fatalf("diagnostic location = %s, want openContent location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
		t.Fatalf("diagnostic spec ref = %q, want XSD 1.1 schema document", diagnostic.SpecRef())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("diagnostic lost unsupported or policy-mismatch cause: %v", err)
	}
}

func TestSchemaBridgeGlobalNamedOpenContentNoneLoadsThroughIncludeImportGraph(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1"><xs:include schemaLocation="child.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {
			id:       "child.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1"><xs:complexType name="Child"><xs:openContent mode="none"/></xs:complexType></xs:schema>`,
		},
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.1"><xs:complexType name="Other"/></xs:schema>`,
		},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			assertGlobalNamedOpenContentNoneGraph(t, root, fixtures, policy)
		})
	}
}

func assertGlobalNamedOpenContentNoneGraph(t *testing.T, root string, fixtures map[string]discoveryFixture, policy LanguagePolicy) {
	t.Helper()
	discovery, err := discoverTestSyntaxWithPolicy(t, root, fixtures, policy)
	if err != nil {
		t.Fatalf("discoverTestSyntaxWithPolicy: %v", err)
	}
	assertSchemaDiscoveryDocumentOrder(t, discovery, []SourceID{"root.xsd", "child.xsd", "other.xsd"})
	schema, err := newSchemaFromDiscoveryWithPolicy(discovery, policy)
	if err != nil {
		t.Fatalf("newSchemaFromDiscoveryWithPolicy: %v", err)
	}
	assertOpenContentGraphDocuments(t, schema)
	assertOpenContentGraphComponents(t, schema)
}

func assertOpenContentGraphDocuments(t *testing.T, schema Schema) {
	t.Helper()
	documents := schema.Documents()
	if got, want := len(documents), 3; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	for index, want := range []SourceID{"root.xsd", "child.xsd", "other.xsd"} {
		if got := documents[index].Source(); got != want {
			t.Fatalf("document %d source = %q, want %q", index, got, want)
		}
	}
}

func assertOpenContentGraphComponents(t *testing.T, schema Schema) {
	t.Helper()
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	if components[0].Document() != "child.xsd" || components[1].Document() != "other.xsd" {
		t.Fatalf("component documents = %q, %q; want child.xsd, other.xsd", components[0].Document(), components[1].Document())
	}
	assertGlobalNamedOpenContentFacts(t, schema, false)
}

func TestSchemaBridgeGlobalNamedOpenContentNoneStrict10GraphDiagnosticIsDeterministic(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.0"><xs:include schemaLocation="child.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {
			id:       "child.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1"><xs:complexType name="Child"><xs:openContent mode="none"/></xs:complexType></xs:schema>`,
		},
	}
	var first string
	for run := 0; run < 2; run++ {
		first = assertGlobalNamedOpenContentNoneStrict10GraphDiagnostic(t, root, fixtures["child.xsd"].contents, run, first)
	}
}

func assertGlobalNamedOpenContentNoneStrict10GraphDiagnostic(t *testing.T, root, child string, run int, first string) string {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: child},
	}, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted graph openContent mode=none")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("Strict10 returned a partial graph schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "child.xsd", child, 1, "<xs:openContent") {
		t.Fatalf("diagnostic location = %s, want child openContent location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
		t.Fatalf("diagnostic spec ref = %q, want XSD 1.1 schema document", diagnostic.SpecRef())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("diagnostic lost unsupported or policy-mismatch cause: %v", err)
	}
	if run == 0 {
		return diagnostic.Error()
	}
	if got := diagnostic.Error(); got != first {
		t.Fatalf("diagnostic changed between runs: first=%q current=%q", first, got)
	}
	return first
}

func TestSchemaBridgeGlobalOpenContentUnsupportedFormsRemainExplicit(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "interleave",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="interleave"><xs:any/></xs:openContent>`, ""),
		},
		{
			name: "suffix",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="suffix"><xs:any/></xs:openContent>`, ""),
		},
		{
			name: "default mode",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent><xs:any namespace="##any"/></xs:openContent>`, ""),
		},
		{
			name: "derivation-local",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1"><xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:openContent mode="none"/><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:schema>`,
		},
		{
			name: "inline",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:element name="item"><xs:complexType><xs:openContent mode="none"/></xs:complexType></xs:element></xs:schema>`,
		},
		{
			name: "defaultOpenContent",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:defaultOpenContent mode="suffix"><xs:any/></xs:defaultOpenContent></xs:schema>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			assertOpenContentUnsupported(t, schema, err)
		})
	}
}

func TestSchemaBridgeGlobalOpenContentNoneMalformedFormsRemainInvalid(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "none with any",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="none"><xs:any/></xs:openContent>`, ""),
		},
		{
			name: "invalid mode",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="bad"/>`, ""),
		},
		{
			name: "duplicate openContent",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="none"/><xs:openContent mode="none"/>`, ""),
		},
		{
			name: "annotation duplicate",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="none"><xs:annotation/><xs:annotation/></xs:openContent>`, ""),
		},
		{
			name: "forbidden child",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="none"><xs:sequence/></xs:openContent>`, ""),
		},
		{
			name: "misplaced after model",
			root: schemaGlobalNamedOpenContentRoot(`<xs:sequence/><xs:openContent mode="none"/>`, ""),
		},
		{
			name: "malformed child after unsupported child",
			root: schemaGlobalNamedOpenContentRoot(`<xs:openContent mode="none"><xs:unknown/><xs:sequence/></xs:openContent>`, ""),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertGlobalOpenContentNoneMalformed(t, test.root)
		})
	}
}

func assertGlobalOpenContentNoneMalformed(t *testing.T, root string) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err == nil {
		t.Fatal("accepted malformed openContent form")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("returned a partial schema for malformed openContent form")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
	}
	if errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("invalid openContent form retained an unsupported cause: %v", err)
	}
}

func schemaGlobalNamedOpenContentRoot(openContent, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="1.1"><xs:complexType name="Item">` + openContent + body + `</xs:complexType></xs:schema>`
}

func assertGlobalNamedOpenContentFacts(t *testing.T, schema Schema, hasParticle bool) {
	t.Helper()
	components := schema.Components()
	if len(components) == 0 {
		t.Fatal("schema has no complex type component")
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("complex type definition view is missing")
	}
	if _, ok := definition.AnyAttribute(); ok {
		t.Fatal("inert openContent exposed an attribute wildcard")
	}
	if hasParticle {
		if _, ok := definition.Particle().(SequenceParticle); !ok {
			t.Fatalf("particle = %T, want SequenceParticle", definition.Particle())
		}
		return
	}
	if definition.Particle() != nil {
		t.Fatalf("empty complex type particle = %T, want nil", definition.Particle())
	}
}

func assertOpenContentUnsupported(t *testing.T, schema Schema, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("accepted unsupported openContent form")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("returned a partial schema for unsupported openContent form")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported cause was not preserved: %v", err)
	}
}
