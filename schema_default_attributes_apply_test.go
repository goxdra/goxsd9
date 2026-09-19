package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

type schemaDefaultAttributesApplyValue struct {
	name    string
	present bool
	lexical string
}

func schemaDefaultAttributesApplyValues() []schemaDefaultAttributesApplyValue {
	return []schemaDefaultAttributesApplyValue{
		{name: "omitted"},
		{name: "true", present: true, lexical: "true"},
		{name: "false", present: true, lexical: "false"},
		{name: "one", present: true, lexical: "1"},
		{name: "zero", present: true, lexical: "0"},
	}
}

type schemaDefaultAttributesApplyBody struct {
	name       string
	content    string
	derivation ComplexTypeDerivation
	base       bool
}

type schemaDefaultAttributesApplyGraphSource struct {
	name      string
	component QName
	source    SourceID
}

func schemaDefaultAttributesApplyBodies() []schemaDefaultAttributesApplyBody {
	return []schemaDefaultAttributesApplyBody{
		{
			name: "empty",
		},
		{
			name: "direct sequence",
			content: `    <xs:sequence>
      <xs:element name="value" type="xs:integer"/>
    </xs:sequence>`,
		},
		{
			name: "direct choice",
			content: `    <xs:choice>
      <xs:element name="value" type="xs:integer"/>
    </xs:choice>`,
		},
		{
			name:       "restriction",
			derivation: ComplexTypeDerivationRestriction,
			content: `    <xs:complexContent>
      <xs:restriction base="xs:anyType">
        <xs:anyAttribute namespace="##other" processContents="lax"/>
      </xs:restriction>
    </xs:complexContent>`,
		},
		{
			name:       "extension",
			derivation: ComplexTypeDerivationExtension,
			base:       true,
			content: `    <xs:complexContent>
      <xs:extension base="t:Base">
        <xs:sequence>
          <xs:element name="value" type="xs:integer"/>
        </xs:sequence>
      </xs:extension>
    </xs:complexContent>`,
		},
	}
}

//nolint:gocognit // Keep the policy, version, value, and shape matrix together.
func TestSchemaNamedComplexDefaultAttributesApplyValuesMatchOmission(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	}
	versions := []struct {
		name    string
		version XSDVersion
	}{
		{name: "XSD 1.0", version: XSDVersion10},
		{name: "XSD 1.1", version: XSDVersion11},
	}
	values := schemaDefaultAttributesApplyValues()
	for _, body := range schemaDefaultAttributesApplyBodies() {
		for _, policy := range policies {
			for _, version := range versions {
				t.Run(body.name+"/"+policy.name+"/"+version.name, func(t *testing.T) {
					omitted := schemaDefaultAttributesApplySnapshot(t, body, values[0], policy.policy, version.version)
					for _, value := range values[1:] {
						got := schemaDefaultAttributesApplySnapshot(t, body, value, policy.policy, version.version)
						if !reflect.DeepEqual(got, omitted) {
							t.Fatalf("%s facts differ from omission: got=%#v omitted=%#v", value.name, got, omitted)
						}
					}
				})
			}
		}
	}
}

func schemaDefaultAttributesApplySnapshot(t *testing.T, body schemaDefaultAttributesApplyBody, value schemaDefaultAttributesApplyValue, policy LanguagePolicy, version XSDVersion) schemaFinalDefaultSnapshot {
	t.Helper()
	root := schemaDefaultAttributesApplyRoot(body, value, version)
	queryName := mustTestQName(t, "urn:root", "Item")
	var first schemaFinalDefaultSnapshot
	for iteration := 0; iteration < 2; iteration++ {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("discoverTestSchema: %v", err)
		}
		assertSchemaDefaultAttributesApplyFacts(t, schema, body, queryName)
		current := snapshotSchemaForComponentKind(t, schema, queryName, ComponentKindComplexTypeDefinition)
		if iteration == 0 {
			first = current
			continue
		}
		if !reflect.DeepEqual(current, first) {
			t.Fatalf("repeated schema snapshot changed: first=%#v current=%#v", first, current)
		}
	}
	return first
}

//nolint:gocognit // Keep the supported complex-type fact assertions together.
func assertSchemaDefaultAttributesApplyFacts(t *testing.T, schema Schema, body schemaDefaultAttributesApplyBody, queryName QName) {
	t.Helper()
	found := schema.FindKind(ComponentKindComplexTypeDefinition, queryName)
	if len(found) != 1 {
		t.Fatalf("Item complex type matches = %d, want one", len(found))
	}
	definition, ok := found[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Item complex type view is missing")
	}
	if body.derivation != "" {
		if got := definition.Derivation(); got != body.derivation {
			t.Fatalf("derivation = %q, want %q", got, body.derivation)
		}
		if body.derivation == ComplexTypeDerivationExtension && definition.Base() != mustTestQName(t, "urn:root", "Base") {
			t.Fatalf("extension base = %q, want urn:root:Base", definition.Base())
		}
	}
	if body.name == "restriction" {
		if definition.Particle() != nil {
			t.Fatalf("restriction particle = %T, want nil", definition.Particle())
		}
		if definition.Base() != mustTestQName(t, testXSDNamespace, "anyType") {
			t.Fatalf("restriction base = %q, want xs:anyType", definition.Base())
		}
		attribute, ok := definition.AnyAttribute()
		if !ok || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
			t.Fatalf("restriction wildcard = %q/%q/%t, want ##other/lax/present", attribute.Namespace(), attribute.ProcessContents(), ok)
		}
		return
	}
	if body.derivation == "" && definition.Derivation() != "" {
		t.Fatalf("direct derivation = %q, want empty", definition.Derivation())
	}
	switch body.name {
	case "empty":
		if definition.Particle() != nil {
			t.Fatalf("empty particle = %T, want nil", definition.Particle())
		}
	case "direct sequence", "extension":
		sequence, ok := definition.Particle().(SequenceParticle)
		if !ok || len(sequence.Elements()) != 1 || sequence.Elements()[0].Name().Local() != "value" {
			t.Fatalf("%s particle = %T/%v, want one value sequence", body.name, definition.Particle(), definition.Particle())
		}
	case "direct choice":
		choice, ok := definition.Particle().(ChoiceParticle)
		if !ok || len(choice.Alternatives()) != 1 {
			t.Fatalf("direct choice particle = %T, want one alternative", definition.Particle())
		}
		alternative, ok := choice.Alternatives()[0].(ElementParticle)
		if !ok || alternative.Name().Local() != "value" {
			t.Fatalf("direct choice alternative = %#v, want value element", choice.Alternatives()[0])
		}
	default:
		t.Fatalf("unknown defaultAttributesApply body %q", body.name)
	}
}

func schemaDefaultAttributesApplyRoot(body schemaDefaultAttributesApplyBody, value schemaDefaultAttributesApplyValue, version XSDVersion) string {
	attribute := ""
	if value.present {
		attribute = ` defaultAttributesApply="` + value.lexical + `"`
	}
	base := ""
	if body.base {
		base = `  <xs:complexType name="Base"/>` + "\n"
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version) + `">` + "\n" + base + `  <xs:complexType name="Item"` + attribute + `>` + "\n" + body.content + "\n  </xs:complexType>\n</xs:schema>"
}

//nolint:gocognit // Keep the graph source, policy, and value matrix together.
func TestSchemaNamedComplexDefaultAttributesApplyPreservesGraphSourceContext(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	}
	declaringSources := []schemaDefaultAttributesApplyGraphSource{
		{
			name:      "root",
			component: mustTestQName(t, "urn:root", "RootType"),
			source:    "root.xsd",
		},
		{
			name:      "chameleon include",
			component: mustTestQName(t, "urn:root", "IncludedType"),
			source:    "chameleon.xsd",
		},
		{
			name:      "import",
			component: mustTestQName(t, "urn:direct", "ImportedType"),
			source:    "direct.xsd",
		},
	}
	values := schemaDefaultAttributesApplyValues()
	for _, policy := range policies {
		for _, source := range declaringSources {
			t.Run(policy.name+"/"+source.name, func(t *testing.T) {
				omittedRoot, omittedFixtures := schemaDefaultAttributesApplyGraph(source.name, values[0])
				omitted, err := discoverTestSchemaWithPolicy(t, omittedRoot, omittedFixtures, policy.policy)
				if err != nil {
					t.Fatalf("omitted graph: %v", err)
				}
				assertSchemaDefaultAttributesApplyGraphSources(t, omitted, source)
				want := snapshotSchemaForComponentKind(t, omitted, source.component, ComponentKindComplexTypeDefinition)
				for _, value := range values[1:] {
					root, fixtures := schemaDefaultAttributesApplyGraph(source.name, value)
					gotSchema, graphErr := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
					if graphErr != nil {
						t.Fatalf("%s graph: %v", value.name, graphErr)
					}
					assertSchemaDefaultAttributesApplyGraphSources(t, gotSchema, source)
					got := snapshotSchemaForComponentKind(t, gotSchema, source.component, ComponentKindComplexTypeDefinition)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("%s graph facts differ from omission: got=%#v omitted=%#v", value.name, got, want)
					}
				}
			})
		}
	}
}

func schemaDefaultAttributesApplyGraph(declaringSource string, value schemaDefaultAttributesApplyValue) (string, map[string]discoveryFixture) {
	attribute := schemaDefaultAttributesApplyAttribute(value)
	rootAttribute := ""
	includedAttribute := ""
	importedAttribute := ""
	switch declaringSource {
	case "root":
		rootAttribute = attribute
	case "chameleon include":
		includedAttribute = attribute
	case "import":
		importedAttribute = attribute
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1">` + "\n" +
		`  <xs:include schemaLocation="chameleon.xsd"/>` + "\n" +
		`  <xs:import namespace="urn:direct" schemaLocation="direct.xsd"/>` + "\n" +
		`  <xs:complexType name="RootType"` + rootAttribute + `>` + "\n" +
		`    <xs:sequence>` + "\n" +
		`      <xs:element name="rootValue" type="xs:integer"/>` + "\n" +
		`    </xs:sequence>` + "\n" +
		`  </xs:complexType>` + "\n" +
		`</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"chameleon.xsd": {
			id: "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">` + "\n" +
				`  <xs:complexType name="IncludedType"` + includedAttribute + `>` + "\n" +
				`    <xs:choice>` + "\n" +
				`      <xs:element name="includedValue" type="xs:integer"/>` + "\n" +
				`    </xs:choice>` + "\n" +
				`  </xs:complexType>` + "\n" +
				`</xs:schema>`,
		},
		"direct.xsd": {
			id: "direct.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:direct" version="1.1">` + "\n" +
				`  <xs:complexType name="ImportedType"` + importedAttribute + `>` + "\n" +
				`    <xs:sequence>` + "\n" +
				`      <xs:element name="importedValue" type="xs:integer"/>` + "\n" +
				`    </xs:sequence>` + "\n" +
				`  </xs:complexType>` + "\n" +
				`</xs:schema>`,
		},
	}
	return root, fixtures
}

func schemaDefaultAttributesApplyAttribute(value schemaDefaultAttributesApplyValue) string {
	if !value.present {
		return ""
	}
	return ` defaultAttributesApply="` + value.lexical + `"`
}

func assertSchemaDefaultAttributesApplyGraphSources(t *testing.T, schema Schema, source schemaDefaultAttributesApplyGraphSource) {
	t.Helper()
	documents := schema.Documents()
	wantDocuments := []SourceID{"root.xsd", "chameleon.xsd", "direct.xsd"}
	if len(documents) != len(wantDocuments) {
		t.Fatalf("document count = %d, want %d", len(documents), len(wantDocuments))
	}
	for index, want := range wantDocuments {
		if got := documents[index].Source(); got != want {
			t.Fatalf("document %d source = %q, want %q", index, got, want)
		}
	}
	found := schema.FindKind(ComponentKindComplexTypeDefinition, source.component)
	if len(found) != 1 {
		t.Fatalf("%s component matches = %d, want one", source.name, len(found))
	}
	if got := found[0].Document(); got != source.source {
		t.Fatalf("%s component source = %q, want %q", source.name, got, source.source)
	}
	if _, ok := found[0].ComplexTypeDefinition(); !ok {
		t.Fatalf("%s component has no complex type definition", source.name)
	}
}

//nolint:gocognit // Keep all valid Strict10 mismatch forms and provenance assertions together.
func TestSchemaNamedComplexDefaultAttributesApplyStrict10RetainsLocatedMismatch(t *testing.T) {
	body := schemaDefaultAttributesApplyBodies()[1]
	for _, value := range schemaDefaultAttributesApplyValues()[1:] {
		t.Run(value.name, func(t *testing.T) {
			root := schemaDefaultAttributesApplyRoot(body, value, XSDVersion10)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			if err == nil {
				t.Fatal("Strict10 accepted defaultAttributesApply or returned no diagnostic")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want located schema-syntax mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
				t.Fatalf("diagnostic spec ref = %q, want XSD 1.1 schema document", diagnostic.SpecRef())
			}
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, "defaultAttributesApply") {
				t.Fatalf("diagnostic location = %s, want defaultAttributesApply location", diagnostic.Loc())
			}
			if diagnostic.Message() != `global complexType attribute "defaultAttributesApply" is an XSD 1.1-only construct` {
				t.Fatalf("diagnostic message = %q, want Strict10 mismatch", diagnostic.Message())
			}
			if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("diagnostic lost unsupported or policy-mismatch cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep malformed lexical forms and policy classification assertions together.
func TestSchemaNamedComplexDefaultAttributesApplyMalformedValuesRemainInvalid(t *testing.T) {
	malformed := []struct {
		name    string
		lexical string
	}{
		{name: "empty", lexical: ""},
		{name: "word", lexical: "maybe"},
		{name: "wrong case", lexical: "TRUE"},
		{name: "boolean list", lexical: "true false"},
	}
	policies := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	body := schemaDefaultAttributesApplyBodies()[1]
	for _, policy := range policies {
		for _, test := range malformed {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := schemaDefaultAttributesApplyRoot(body, schemaDefaultAttributesApplyValue{present: true, lexical: test.lexical}, policy.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil {
					t.Fatal("malformed defaultAttributesApply unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
					t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, "defaultAttributesApply") {
					t.Fatalf("diagnostic location = %s, want defaultAttributesApply location", diagnostic.Loc())
				}
				if diagnostic.Message() != `attribute "defaultAttributesApply" has an invalid boolean value` {
					t.Fatalf("diagnostic message = %q, want invalid boolean", diagnostic.Message())
				}
				if diagnostic.Feature() != "" || diagnostic.SpecRef() != "" || errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("malformed value was classified as unsupported or mismatch: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Keep the explicit unsupported boundary matrix together.
func TestSchemaDefaultAttributesApplyPreservesUnsupportedBoundaries(t *testing.T) {
	t.Run("schema root defaultAttributes", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributes="Defaults">` + "\n" +
			`  <xs:complexType name="Item" defaultAttributesApply="false"/>` + "\n" +
			`</xs:schema>`
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			t.Run(string(policy), func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("schema root defaultAttributes unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "defaultAttributes") {
					t.Fatalf("diagnostic location = %s, want root defaultAttributes", diagnostic.Loc())
				}
				if diagnostic.Message() != `schema root attribute "defaultAttributes" is not implemented` {
					t.Fatalf("diagnostic message = %q, want root unsupported message", diagnostic.Message())
				}
				if !errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("root defaultAttributes lost unsupported classification: %v", err)
				}
			})
		}
	})

	t.Run("inline complex type", func(t *testing.T) {
		values := schemaDefaultAttributesApplyValues()[1:]
		policies := []struct {
			name    string
			policy  LanguagePolicy
			version XSDVersion
		}{
			{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
			{name: "Strict10", policy: Strict10, version: XSDVersion10},
			{name: "Strict11", policy: Strict11, version: XSDVersion11},
		}
		for _, policy := range policies {
			for _, value := range values {
				t.Run(policy.name+"/"+value.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(policy.version) + `">` + "\n" +
						`  <xs:element name="root">` + "\n" +
						`    <xs:complexType` + schemaDefaultAttributesApplyAttribute(value) + `>` + "\n" +
						`      <xs:sequence>` + "\n" +
						`        <xs:element name="value" type="xs:integer"/>` + "\n" +
						`      </xs:sequence>` + "\n" +
						`    </xs:complexType>` + "\n" +
						`  </xs:element>` + "\n" +
						`</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("inline defaultAttributesApply unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
						t.Fatalf("diagnostic = %s/%q/%q, want inline unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
					}
					if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, "defaultAttributesApply") {
						t.Fatalf("diagnostic location = %s, want inline defaultAttributesApply", diagnostic.Loc())
					}
					if !errors.Is(err, ErrUnsupported) {
						t.Fatalf("inline diagnostic lost unsupported classification: %v", err)
					}
					if policy.policy == Strict10 && !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 inline diagnostic lost policy mismatch: %v", err)
					}
					if policy.policy != Strict10 && errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("XSD 1.1 inline diagnostic was classified as mismatch: %v", err)
					}
				})
			}
		}
	})

	t.Run("anonymous global complex type", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + "\n" +
			`  <xs:complexType defaultAttributesApply="false"><xs:sequence/></xs:complexType>` + "\n" +
			`</xs:schema>`
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(string(policy), func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("anonymous global complex type unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaDeclarationNameCode {
					t.Fatalf("diagnostic = %s, want missing-name invalid diagnostic", diagnostic)
				}
				if errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("anonymous global retained unsupported or mismatch cause: %v", err)
				}
			})
		}
	})

	t.Run("unsupported named body", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + "\n" +
			`  <xs:complexType name="Item" defaultAttributesApply="false">` + "\n" +
			`    <xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent>` + "\n" +
			`  </xs:complexType>` + "\n" +
			`</xs:schema>`
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			t.Run(string(policy), func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("unsupported complex type body unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s/%q/%q, want body unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, "<xs:extension") {
					t.Fatalf("diagnostic location = %s, want extension element", diagnostic.Loc())
				}
				if !errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("unsupported body diagnostic provenance = %v", err)
				}
			})
		}
	})
}
