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

//nolint:gocognit // Keep the complete omission-equivalence matrix together.
func TestSchemaBridgeComplexContentExtensionOpenContentNoneMatchesOmission(t *testing.T) {
	openContents := []struct {
		name  string
		value string
	}{
		{name: "without annotation", value: `<xs:openContent mode="none"/>`},
		{name: "with annotation", value: `<xs:openContent mode="none"><xs:annotation/></xs:openContent>`},
	}
	models := []string{"choice", "sequence", "group"}
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	}
	queryName := mustTestQName(t, "urn:root", "Derived")
	for _, openContent := range openContents {
		for _, model := range models {
			for _, profile := range policies {
				t.Run(openContent.name+"/"+model+"/"+profile.name, func(t *testing.T) {
					withNone := complexContentExtensionOpenContentSchema("1.1", model, openContent.value)
					omitted := strings.Replace(withNone, openContent.value, strings.Repeat(" ", len(openContent.value)), 1)
					omittedSchema, err := discoverTestSchemaWithPolicy(t, omitted, nil, profile.policy)
					if err != nil {
						t.Fatalf("omitted openContent: %v", err)
					}
					noneSchema, err := discoverTestSchemaWithPolicy(t, withNone, nil, profile.policy)
					if err != nil {
						t.Fatalf("mode=none openContent: %v", err)
					}
					omittedSnapshot := snapshotSchemaForComponentKind(t, omittedSchema, queryName, ComponentKindComplexTypeDefinition)
					noneSnapshot := snapshotSchemaForComponentKind(t, noneSchema, queryName, ComponentKindComplexTypeDefinition)
					if !reflect.DeepEqual(noneSnapshot, omittedSnapshot) {
						t.Fatalf("public component snapshot differs: mode=none=%#v omitted=%#v", noneSnapshot, omittedSnapshot)
					}
					assertComplexContentExtensionOpenContentParticle(t, noneSchema, model, withNone)

					components := noneSchema.Components()
					components[0] = Component{}
					definition, ok := noneSchema.FindKind(ComponentKindComplexTypeDefinition, queryName)[0].ComplexTypeDefinition()
					if !ok {
						t.Fatal("derived complex type view is absent after component-copy mutation")
					}
					switch particle := definition.Particle().(type) {
					case ChoiceParticle:
						alternatives := particle.Alternatives()
						if len(alternatives) > 0 {
							alternatives[0] = nil
						}
					case SequenceParticle:
						particles := particle.Particles()
						if len(particles) > 0 {
							particles[0] = nil
						}
					}
					if got := snapshotSchemaForComponentKind(t, noneSchema, queryName, ComponentKindComplexTypeDefinition); !reflect.DeepEqual(got, noneSnapshot) {
						t.Fatalf("mutating returned component or particle copies changed the schema: %#v", got)
					}
				})
			}
		}
	}
}

func TestSchemaBridgeComplexContentExtensionOpenContentNoneHonorsStrict10Policy(t *testing.T) {
	openContent := `<xs:openContent mode="none"><xs:annotation/></xs:openContent>`
	root := complexContentExtensionOpenContentSchema("1.0", "choice", openContent)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted derivation-local openContent mode=none")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s, want located schema-syntax unsupported", diagnostic)
	}
	if diagnostic.Loc() != complexContentTestLoc(t, root, "<xs:openContent") {
		t.Fatalf("diagnostic location = %s, want openContent location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("diagnostic lost unsupported or policy-mismatch cause: %v", err)
	}
}

func TestSchemaBridgeComplexContentExtensionOpenContentUnsupportedFormsRemainExplicit(t *testing.T) {
	base := `<xs:complexType name="Base"/>`
	tests := []struct {
		name   string
		root   string
		marker string
	}{
		{
			name:   "restriction",
			root:   complexContentDerivationRoot(`<xs:restriction base="xs:anyType"><xs:openContent mode="none"/><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:restriction>`, ""),
			marker: "<xs:openContent",
		},
		{
			name:   "non-none mode",
			root:   complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="suffix"><xs:any/></xs:openContent><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, base),
			marker: "<xs:openContent",
		},
		{
			name:   "assertion",
			root:   complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="none"/><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:assert test="true()"/></xs:extension>`, base),
			marker: "<xs:assert",
		},
		{
			name:   "built-in base",
			root:   complexContentExtensionRoot(`<xs:extension base="xs:anyType"><xs:openContent mode="none"/><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, ""),
			marker: `base="xs:anyType"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("unsupported complex-content openContent form unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
			}
			if diagnostic.Loc() != complexContentTestLoc(t, test.root, test.marker) {
				t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), complexContentTestLoc(t, test.root, test.marker))
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic lost cause: %v", err)
			}
		})
	}
}

func TestSchemaBridgeComplexContentExtensionOpenContentMalformedFormsRemainInvalid(t *testing.T) {
	base := `<xs:complexType name="Base"/>`
	tests := []struct {
		name string
		root string
	}{
		{
			name: "none with any",
			root: complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="none"><xs:any/></xs:openContent><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, base),
		},
		{
			name: "invalid mode",
			root: complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="bad"/><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, base),
		},
		{
			name: "duplicate openContent",
			root: complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="none"/><xs:openContent mode="none"/><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, base),
		},
		{
			name: "duplicate annotation",
			root: complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="none"><xs:annotation/><xs:annotation/></xs:openContent><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, base),
		},
		{
			name: "misplaced after model",
			root: complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:openContent mode="none"/></xs:extension>`, base),
		},
		{
			name: "malformed later sibling",
			root: complexContentExtensionRoot(`<xs:extension base="t:Base"><xs:openContent mode="none"><xs:unknown/><xs:sequence/></xs:openContent><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension>`, base),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Compatibility)
			if err == nil {
				t.Fatal("malformed complex-content openContent form unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
			}
			if errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("invalid openContent form retained an unsupported cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep direct particle-shape and identity assertions together.
func assertComplexContentExtensionOpenContentParticle(t *testing.T, schema Schema, model, root string) {
	t.Helper()
	derived := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
	if len(derived) != 1 {
		t.Fatalf("derived complex type count = %d, want one", len(derived))
	}
	definition, ok := derived[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("derived complex type definition is absent")
	}
	particle := definition.Particle()
	if particle == nil {
		t.Fatal("derived extension particle is absent")
	}
	wantLoc := "<xs:" + model
	if model == "group" {
		wantLoc = `<xs:group ref="t:Added"`
	}
	if particle.Loc() != complexContentTestLoc(t, root, wantLoc) {
		t.Fatalf("particle location = %s, want %s", particle.Loc(), complexContentTestLoc(t, root, wantLoc))
	}
	if model == "group" {
		group, ok := particle.(ModelGroupReferenceParticle)
		if !ok || group.Ref() != mustTestQName(t, "urn:root", "Added") {
			t.Fatalf("particle = %#v, want direct Added group reference", particle)
		}
		groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Added"))
		if len(groups) != 1 || group.TargetID() != groups[0].ID() {
			t.Fatalf("group target identity = %v, want %v", group.TargetID(), groups[0].ID())
		}
		return
	}
	switch model {
	case "choice":
		if _, ok := particle.(ChoiceParticle); !ok {
			t.Fatalf("particle = %T, want choice", particle)
		}
	case "sequence":
		if _, ok := particle.(SequenceParticle); !ok {
			t.Fatalf("particle = %T, want sequence", particle)
		}
	default:
		t.Fatalf("unknown extension model %q", model)
	}
}

func complexContentExtensionOpenContentSchema(version, model, openContent string) string {
	if model == "group" {
		return complexContentExtensionGroupSchema(version, openContent)
	}
	root := complexContentExtensionSchema(version, model)
	return strings.Replace(root, `<xs:extension base="t:Base">`, `<xs:extension base="t:Base">`+openContent, 1)
}

func complexContentExtensionGroupSchema(version, openContent string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `">
  <xs:group name="Added"><xs:choice><xs:element ref="t:target"/></xs:choice></xs:group>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base">` + openContent + `<xs:group ref="t:Added" minOccurs="0" maxOccurs="2"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="target" type="xs:decimal"/>
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
}

func complexContentDerivationRoot(derivation, suffix string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="Derived"><xs:complexContent>` + derivation + `</xs:complexContent></xs:complexType>
  ` + suffix + `
</xs:schema>`
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
