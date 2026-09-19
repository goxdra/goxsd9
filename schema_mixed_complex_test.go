package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Keep the policy, edition, body, and lexical matrices together.
func TestSchemaComplexTypeMixedFalseValuesMatchOmittedAcrossPoliciesVersionsAndBodies(t *testing.T) {
	for _, body := range schemaMixedComplexBodies() {
		for _, policy := range schemaMixedComplexPolicies() {
			for _, version := range schemaMixedComplexVersions() {
				t.Run(body.name+"/"+policy.name+"/"+version.name, func(t *testing.T) {
					omitted := schemaMixedComplexSnapshot(t, body, schemaMixedComplexValue{}, schemaMixedComplexValue{}, policy.policy, version.version)
					for _, test := range schemaMixedComplexFalseCases(body) {
						got := schemaMixedComplexSnapshot(t, body, test.outer, test.inner, policy.policy, version.version)
						if !reflect.DeepEqual(got, omitted) {
							t.Fatalf("%s mixed facts differ from omitted: got=%#v omitted=%#v", test.name, got, omitted)
						}
					}
				})
			}
		}
	}
}

func schemaMixedComplexSnapshot(t *testing.T, body schemaMixedComplexBody, outer, inner schemaMixedComplexValue, policy LanguagePolicy, version XSDVersion) []Component {
	t.Helper()
	root := schemaMixedComplexSchema(body, outer, inner, version)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	assertSchemaMixedComplexFacts(t, schema, body)
	components := schema.Components()
	if len(components) == 0 {
		t.Fatal("schema has no components")
	}
	wantKind := components[0].Kind()
	components[0] = Component{}
	if gotKind := schema.Components()[0].Kind(); gotKind != wantKind {
		t.Fatalf("mutating Components result changed the completed schema: got kind %q, want %q", gotKind, wantKind)
	}
	return schema.Components()
}

//nolint:gocognit,funlen // Keep the public particle and derivation fact checks together.
func assertSchemaMixedComplexFacts(t *testing.T, schema Schema, body schemaMixedComplexBody) {
	t.Helper()
	name := mustTestQName(t, "urn:root", "Item")
	found := schema.FindKind(ComponentKindComplexTypeDefinition, name)
	if len(found) != 1 {
		t.Fatalf("Item complex type matches = %d, want one", len(found))
	}
	definition, ok := found[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Item complex type view is missing")
	}

	if body.derivation == "restriction" {
		if definition.Particle() != nil || definition.Derivation() != ComplexTypeDerivationRestriction {
			t.Fatalf("restriction facts = particle %T, derivation %q", definition.Particle(), definition.Derivation())
		}
		if definition.Base() != mustTestQName(t, testXSDNamespace, "anyType") {
			t.Fatalf("restriction base = %q, want xs:anyType", definition.Base())
		}
		attribute, attributeOK := definition.AnyAttribute()
		if !attributeOK || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
			t.Fatalf("restriction wildcard = %q/%q/%t, want ##other/lax/present", attribute.Namespace(), attribute.ProcessContents(), attributeOK)
		}
		return
	}

	if body.derivation == "extension" && definition.Derivation() != ComplexTypeDerivationExtension {
		t.Fatalf("extension derivation = %q, want extension", definition.Derivation())
	}
	if body.derivation == "extension" && definition.Base() != mustTestQName(t, "urn:root", "Base") {
		t.Fatalf("extension base = %q, want urn:root:Base", definition.Base())
	}
	particle := definition.Particle()
	if particle == nil {
		t.Fatalf("%s particle is absent", body.name)
	}
	if particle.Occurrences().String() != "1/1" {
		t.Fatalf("%s particle occurrences = %s, want 1/1", body.name, particle.Occurrences())
	}

	switch body.model {
	case "sequence":
		sequence, ok := particle.(SequenceParticle)
		if !ok {
			t.Fatalf("%s particle = %T, want SequenceParticle", body.name, particle)
		}
		elements := sequence.Elements()
		if len(elements) != 2 {
			t.Fatalf("%s element count = %d, want 2", body.name, len(elements))
		}
		if elements[0].Name().Local() != "first" || elements[1].Name().Local() != "second" {
			t.Fatalf("%s element names = %q/%q, want first/second", body.name, elements[0].Name().Local(), elements[1].Name().Local())
		}
		elements[0] = ElementParticle{}
		fresh, ok := definition.Particle().(SequenceParticle)
		if !ok || len(fresh.Elements()) != 2 || fresh.Elements()[0].Name().Local() != "first" {
			t.Fatal("mutating sequence query changed the completed schema")
		}
	case "choice":
		choice, ok := particle.(ChoiceParticle)
		if !ok {
			t.Fatalf("%s particle = %T, want ChoiceParticle", body.name, particle)
		}
		alternatives := choice.Alternatives()
		if len(alternatives) != 2 {
			t.Fatalf("%s alternative count = %d, want 2", body.name, len(alternatives))
		}
		first, firstOK := alternatives[0].(ElementParticle)
		second, secondOK := alternatives[1].(ElementParticle)
		if !firstOK || !secondOK || first.Name().Local() != "first" || second.Name().Local() != "second" {
			t.Fatalf("%s alternative names = %#v, want first/second", body.name, alternatives)
		}
		alternatives[0] = nil
		fresh, ok := definition.Particle().(ChoiceParticle)
		if !ok || len(fresh.Alternatives()) != 2 || fresh.Alternatives()[0] == nil {
			t.Fatal("mutating choice query changed the completed schema")
		}
	default:
		t.Fatalf("unknown mixed complex body model %q", body.model)
	}
}

func TestSchemaComplexTypeMixedFalsePreservesValidationAndGenerationFacts(t *testing.T) {
	values := []schemaMixedComplexValue{
		{},
		{present: true, value: "false"},
		{present: true, value: "0"},
	}
	var wantGenerated string
	var wantInvalid Diagnostic
	for index, value := range values {
		root := schemaMixedComplexExecutableSchema(value)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
		if err != nil {
			t.Fatalf("value %q discoverSchema: %v", value.value, err)
		}
		generated, generationErr := GenerateGo(schema, "generated")
		if generationErr != nil {
			t.Fatalf("value %q GenerateGo: %v", value.value, generationErr)
		}
		validErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>7</value></root>`)))
		if validErr != nil {
			t.Fatalf("value %q valid instance: %v", value.value, validErr)
		}
		invalidErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>bad</value></root>`)))
		if invalidErr == nil {
			t.Fatalf("value %q invalid instance unexpectedly succeeded", value.value)
		}
		invalidDiagnostic := requireDiagnostic(t, invalidErr)
		if index == 0 {
			wantGenerated = string(generated)
			wantInvalid = invalidDiagnostic
			continue
		}
		if string(generated) != wantGenerated {
			t.Fatalf("value %q changed generated source", value.value)
		}
		assertSameSchemaDiagnostic(t, wantInvalid, invalidDiagnostic)
	}
}

//nolint:gocognit // Keep both equivalent boolean lexical forms and all policies together.
func TestSchemaComplexTypeMixedTrueAndOneRemainLocatedUnsupported(t *testing.T) {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		t.Fatal("schema syntax feature is not registered")
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, value := range []string{"true", "1"} {
				t.Run(policy.name+"/"+version.name+"/"+value, func(t *testing.T) {
					root := schemaMixedComplexSchema(schemaMixedComplexBody{name: "direct sequence", model: "sequence"}, schemaMixedComplexValue{present: true, value: value}, schemaMixedComplexValue{}, version.version)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("mixed content unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
						t.Fatalf("diagnostic = %s/%q/%q, want located schema-syntax unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
					}
					if diagnostic.Loc() != schemaMixedComplexLoc(t, root, `mixed="`+value+`"`) {
						t.Fatalf("diagnostic location = %s, want mixed attribute", diagnostic.Loc())
					}
					if diagnostic.SpecRef() != feature.SpecRef() || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("diagnostic provenance = %q/%v, want %q/unsupported", diagnostic.SpecRef(), err, feature.SpecRef())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep the policy, edition, and outer/inner malformed matrix together.
func TestSchemaComplexTypeMixedMalformedValuesRemainInvalid(t *testing.T) {
	cases := []struct {
		name  string
		body  schemaMixedComplexBody
		outer schemaMixedComplexValue
		inner schemaMixedComplexValue
	}{
		{name: "outer", body: schemaMixedComplexBody{name: "direct sequence", model: "sequence"}, outer: schemaMixedComplexValue{present: true, value: "maybe"}},
		{name: "inner", body: schemaMixedComplexBody{name: "restriction", derivation: "restriction"}, inner: schemaMixedComplexValue{present: true, value: "maybe"}},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, test := range cases {
				t.Run(policy.name+"/"+version.name+"/"+test.name, func(t *testing.T) {
					root := schemaMixedComplexSchema(test.body, test.outer, test.inner, version.version)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("malformed mixed value unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
						t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
					}
					if diagnostic.Loc() != schemaMixedComplexLoc(t, root, `mixed="maybe"`) {
						t.Fatalf("diagnostic location = %s, want malformed mixed attribute", diagnostic.Loc())
					}
					if diagnostic.Feature() != "" || diagnostic.SpecRef() != "" || errors.Is(err, ErrUnsupported) {
						t.Fatalf("malformed value was classified as unsupported: %s", diagnostic)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep XSD 1.1 agreement and the Strict10 boundary explicit.
func TestSchemaComplexTypeMixedAgreementPreservesPolicyBoundary(t *testing.T) {
	cases := []struct {
		name  string
		outer string
		inner string
	}{
		{name: "false versus true", outer: "false", inner: "true"},
		{name: "true versus false", outer: "true", inner: "false"},
		{name: "zero versus one", outer: "0", inner: "1"},
		{name: "one versus zero", outer: "1", inner: "0"},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, test := range cases {
				t.Run(policy.name+"/"+version.name+"/"+test.name, func(t *testing.T) {
					body := schemaMixedComplexBody{name: "restriction", derivation: "restriction"}
					root := schemaMixedComplexSchema(body, schemaMixedComplexValue{present: true, value: test.outer}, schemaMixedComplexValue{present: true, value: test.inner}, version.version)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("contradictory mixed values unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if policy.policy == Strict10 {
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("Strict10 diagnostic = %s, want unsupported", diagnostic)
						}
						return
					}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != schemaMixedComplexLoc(t, root, `mixed="`+test.inner+`"`) {
						t.Fatalf("XSD 1.1 diagnostic = %s, want invalid disagreement at inner mixed", diagnostic)
					}
					if errors.Is(err, ErrUnsupported) {
						t.Fatalf("XSD 1.1 disagreement retained unsupported classification: %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep staged unsupported behavior and later-invalid precedence together.
func TestSchemaComplexTypeMixedUnsupportedDoesNotHideLaterInvalidChild(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, value := range []string{"true", "1"} {
				t.Run(policy.name+"/"+version.name+"/"+value, func(t *testing.T) {
					root := schemaMixedComplexInvalidSequenceSchema(value, version.version)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("later invalid child unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
						t.Fatalf("diagnostic = %s, want invalid later child", diagnostic)
					}
					if diagnostic.Loc() != schemaMixedComplexLoc(t, root, `abstract="true"`) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("later invalid child diagnostic = %s, want invalid child location without unsupported cause", diagnostic)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep the named/anonymous/inline/content boundary matrix together.
func TestSchemaComplexTypeMixedFalseDoesNotCrossInlineOrUnsupportedBoundaries(t *testing.T) {
	cases := []struct {
		name string
		root string
	}{
		{
			name: "inline",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="root"><xs:complexType mixed="false"><xs:sequence/></xs:complexType></xs:element></xs:schema>`,
		},
		{
			name: "anonymous global",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType mixed="false"><xs:sequence/></xs:complexType></xs:schema>`,
		},
		{
			name: "simpleContent",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Item" mixed="false"><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType></xs:schema>`,
		},
		{
			name: "unsupported nested model",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Item" mixed="false"><xs:sequence><xs:choice/></xs:sequence></xs:complexType></xs:schema>`,
		},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.policy)
				if err == nil {
					t.Fatal("boundary form unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if test.name == "anonymous global" {
					if diagnostic.Class() != FailureInvalid {
						t.Fatalf("anonymous global diagnostic = %s, want invalid", diagnostic)
					}
					return
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("boundary diagnostic = %s, want unsupported", diagnostic)
				}
			})
		}
	}
}

type schemaMixedComplexBody struct {
	name       string
	model      string
	derivation string
}

func schemaMixedComplexBodies() []schemaMixedComplexBody {
	return []schemaMixedComplexBody{
		{name: "direct sequence", model: "sequence"},
		{name: "direct choice", model: "choice"},
		{name: "restriction", derivation: "restriction"},
		{name: "extension sequence", model: "sequence", derivation: "extension"},
		{name: "extension choice", model: "choice", derivation: "extension"},
	}
}

type schemaMixedComplexValue struct {
	present bool
	value   string
}

type schemaMixedComplexFalseCase struct {
	name  string
	outer schemaMixedComplexValue
	inner schemaMixedComplexValue
}

func schemaMixedComplexFalseCases(body schemaMixedComplexBody) []schemaMixedComplexFalseCase {
	if body.derivation == "" {
		return []schemaMixedComplexFalseCase{
			{name: "outer false", outer: schemaMixedComplexValue{present: true, value: "false"}},
			{name: "outer zero", outer: schemaMixedComplexValue{present: true, value: "0"}},
		}
	}
	return []schemaMixedComplexFalseCase{
		{name: "outer false", outer: schemaMixedComplexValue{present: true, value: "false"}},
		{name: "outer zero", outer: schemaMixedComplexValue{present: true, value: "0"}},
		{name: "inner false", inner: schemaMixedComplexValue{present: true, value: "false"}},
		{name: "inner zero", inner: schemaMixedComplexValue{present: true, value: "0"}},
		{name: "outer false inner zero", outer: schemaMixedComplexValue{present: true, value: "false"}, inner: schemaMixedComplexValue{present: true, value: "0"}},
		{name: "outer zero inner false", outer: schemaMixedComplexValue{present: true, value: "0"}, inner: schemaMixedComplexValue{present: true, value: "false"}},
	}
}

func schemaMixedComplexPolicies() []struct {
	name   string
	policy LanguagePolicy
} {
	return []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
}

func schemaMixedComplexVersions() []struct {
	name    string
	version XSDVersion
} {
	return []struct {
		name    string
		version XSDVersion
	}{
		{name: "XSD 1.0", version: XSDVersion10},
		{name: "XSD 1.1", version: XSDVersion11},
	}
}

func schemaMixedComplexSchema(body schemaMixedComplexBody, outer, inner schemaMixedComplexValue, version XSDVersion) string {
	outerAttribute := schemaMixedComplexAttribute(outer)
	opening := `<xs:complexType name="Item"` + outerAttribute + `>`
	if body.derivation == "" {
		return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version) + `">` + opening + schemaMixedComplexDirectModel(body.model) + `</xs:complexType></xs:schema>`
	}
	complexContentOpening := `<xs:complexContent` + schemaMixedComplexAttribute(inner) + `>`
	if body.derivation == "restriction" {
		return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version) + `">` + opening + complexContentOpening + `<xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType></xs:schema>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version) + `"><xs:complexType name="Base"/>` + opening + complexContentOpening + `<xs:extension base="t:Base">` + schemaMixedComplexDirectModel(body.model) + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
}

func schemaMixedComplexDirectModel(model string) string {
	return `<xs:` + model + `><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:decimal"/></xs:` + model + `>`
}

func schemaMixedComplexAttribute(value schemaMixedComplexValue) string {
	const width = len(` mixed="false"`)
	if !value.present {
		return strings.Repeat(" ", width)
	}
	attribute := ` mixed="` + value.value + `"`
	padding := width - len(attribute)
	if padding < 0 {
		padding = 0
	}
	return attribute + strings.Repeat(" ", padding)
}

func schemaMixedComplexExecutableSchema(value schemaMixedComplexValue) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" elementFormDefault="qualified" version="1.1"><xs:element name="root" type="t:Item"/><xs:complexType name="Item"` + schemaMixedComplexAttribute(value) + `><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:complexType></xs:schema>`
}

func schemaMixedComplexInvalidSequenceSchema(value string, version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + string(version) + `"><xs:complexType name="Item" mixed="` + value + `"><xs:sequence><xs:element name="value" type="xs:integer" abstract="true"/></xs:sequence></xs:complexType></xs:schema>`
}

func schemaMixedComplexLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.Index(root, marker)
	if index < 0 {
		t.Fatalf("mixed complex fixture does not contain location marker %q", marker)
	}
	line := 1
	column := 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, "root.xsd", line, column)
}
