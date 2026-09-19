package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Keep policy, effective-value, and immutable-view assertions together.
func TestSchemaBridgeBuildsNamedComplexTypeAbstractFactsAcrossPolicies(t *testing.T) {
	for _, profile := range abstractComplexTypeTestProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := namedComplexTypeAbstractFactsSchema(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}

			want := []struct {
				name     string
				abstract bool
			}{
				{name: "DirectTrue", abstract: true},
				{name: "DirectOne", abstract: true},
				{name: "DirectFalse", abstract: false},
				{name: "DirectZero", abstract: false},
				{name: "DirectOmitted", abstract: false},
				{name: "Base", abstract: true},
				{name: "DerivedRestriction", abstract: true},
				{name: "DerivedRestrictionOmitted", abstract: false},
				{name: "DerivedExtension", abstract: false},
			}
			for _, expected := range want {
				definition := namedComplexTypeDefinition(t, schema, "urn:root", expected.name)
				if got := definition.IsAbstract(); got != expected.abstract {
					t.Fatalf("%s IsAbstract() = %t, want %t", expected.name, got, expected.abstract)
				}
			}

			var zero ComplexTypeDefinition
			if zero.IsAbstract() {
				t.Fatal("zero ComplexTypeDefinition reports abstract")
			}
			before := schema.Components()
			queried := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "DirectTrue"))
			if len(queried) != 1 {
				t.Fatalf("DirectTrue query count = %d, want 1", len(queried))
			}
			queried[0] = Component{}
			documents := schema.Documents()
			documents[0] = SchemaDocument{}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating schema query results changed completed complex-type facts")
			}
		})
	}
}

func TestSchemaBridgePreservesNamedComplexTypeAbstractFactsAcrossGraphShapes(t *testing.T) {
	for _, profile := range abstractComplexTypeTestProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root" version="` + profile.version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"chameleon.xsd": {
					id: "chameleon.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="Included" abstract="true"/>
</xs:schema>`,
				},
				"other.xsd": {
					id: "other.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + profile.version + `">
  <xs:complexType name="Imported" abstract="1"/>
</xs:schema>`,
				},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discover graph: %v", err)
			}
			if !namedComplexTypeDefinition(t, schema, "urn:root", "Included").IsAbstract() {
				t.Fatal("chameleon included abstract fact was not retained")
			}
			if !namedComplexTypeDefinition(t, schema, "urn:other", "Imported").IsAbstract() {
				t.Fatal("imported abstract fact was not retained")
			}
		})
	}
}

func TestSchemaBridgeRejectsMalformedNamedComplexTypeAbstractAtAttribute(t *testing.T) {
	for _, profile := range abstractComplexTypeTestProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + profile.version + `">
  <xs:complexType name="Broken" abstract="maybe"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("malformed complexType abstract value was accepted")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
			}
			wantLoc := abstractComplexTypeTestLoc(t, root, `abstract="maybe"`)
			if diagnostic.Loc() != wantLoc {
				t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
			}
		})
	}
}

//nolint:gocognit // Keep validator and generator diagnostic contracts symmetric.
func TestValidateInstanceRejectsAbstractNamedComplexTypeAcrossPolicies(t *testing.T) {
	for _, profile := range abstractComplexTypeTestProfiles() {
		for _, model := range []string{"sequence", "choice"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := abstractNamedComplexTypeConsumerSchema(profile.version, model)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover schema: %v", err)
				}
				definition := namedComplexTypeDefinition(t, schema, "urn:root", "Abstract")
				particle := definition.Particle()
				if particle == nil {
					t.Fatal("abstract consumer complex type has no particle")
				}
				rootComponent := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "root"))[0]
				related := []Loc{rootComponent.Loc(), definition.Loc(), particle.Loc()}
				children := abstractComplexTypeParticleChildren(t, particle)
				for _, child := range children {
					related = appendInstanceRelated(related, child.Loc())
				}

				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
				if instanceErr == nil {
					t.Fatal("ValidateInstance accepted an abstract named complex type")
				}
				instanceDiagnostic := requireDiagnostic(t, instanceErr)
				if instanceDiagnostic.Class() != FailureUnsupported || instanceDiagnostic.Code() != UnsupportedInstanceValidationCode || instanceDiagnostic.Feature() != FeatureInstanceValidation {
					t.Fatalf("instance diagnostic = %s/%q/%q, want unsupported instance-validation diagnostic", instanceDiagnostic, instanceDiagnostic.Code(), instanceDiagnostic.Feature())
				}
				if instanceDiagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) {
					t.Fatalf("instance diagnostic location = %s, want instance.xml:1:1", instanceDiagnostic.Loc())
				}
				if !reflect.DeepEqual(instanceDiagnostic.Related(), related) {
					t.Fatalf("instance related = %v, want %v", instanceDiagnostic.Related(), related)
				}
				if instanceDiagnostic.SpecRef() != profile.instanceSpecRef() {
					t.Fatalf("instance spec ref = %q, want %q", instanceDiagnostic.SpecRef(), profile.instanceSpecRef())
				}
				if !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAbstractComplexType) {
					t.Fatalf("instance diagnostic lost abstract unsupported cause: %v", instanceErr)
				}

				_, generateErr := GenerateGo(schema, "generated")
				if generateErr == nil {
					t.Fatal("GenerateGo accepted an abstract named complex type")
				}
				generateDiagnostic := requireDiagnostic(t, generateErr)
				if generateDiagnostic.Class() != FailureUnsupported || generateDiagnostic.Code() != diagnosticCodegenUnsupported || generateDiagnostic.Feature() != FeatureCodegen {
					t.Fatalf("codegen diagnostic = %s/%q/%q, want unsupported codegen diagnostic", generateDiagnostic, generateDiagnostic.Code(), generateDiagnostic.Feature())
				}
				if generateDiagnostic.Loc() != definition.Loc() {
					t.Fatalf("codegen diagnostic location = %s, want %s", generateDiagnostic.Loc(), definition.Loc())
				}
				wantCodegenRelated := []Loc{definition.Loc()}
				if !reflect.DeepEqual(generateDiagnostic.Related(), wantCodegenRelated) {
					t.Fatalf("codegen related = %v, want %v", generateDiagnostic.Related(), wantCodegenRelated)
				}
				if generateDiagnostic.SpecRef() != profile.codegenSpecRef() {
					t.Fatalf("codegen spec ref = %q, want %q", generateDiagnostic.SpecRef(), profile.codegenSpecRef())
				}
				if !errors.Is(generateErr, ErrUnsupported) || !errors.Is(generateErr, errCodegenUnsupported) {
					t.Fatalf("codegen diagnostic lost abstract unsupported cause: %v", generateErr)
				}
			})
		}
	}
}

type abstractComplexTypeTestProfile struct {
	name    string
	policy  LanguagePolicy
	version string
}

func abstractComplexTypeTestProfiles() []abstractComplexTypeTestProfile {
	return []abstractComplexTypeTestProfile{
		{name: "Compatibility", policy: Compatibility, version: "1.1"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
}

func (profile abstractComplexTypeTestProfile) instanceSpecRef() string {
	if profile.policy == Strict10 {
		return instanceComplexTypeXSD10SpecRef
	}
	return instanceComplexTypeXSD11SpecRef
}

func (profile abstractComplexTypeTestProfile) codegenSpecRef() string {
	if profile.policy == Strict10 {
		return codegenComplexTypeXSD10SpecRef
	}
	return codegenComplexTypeXSD11SpecRef
}

func namedComplexTypeAbstractFactsSchema(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + version + `">
	<xs:complexType name="DerivedRestriction" abstract="true"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
	<xs:complexType name="DerivedRestrictionOmitted"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
  <xs:complexType name="DerivedExtension"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="extra" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="DirectTrue" abstract="true"/>
  <xs:complexType name="DirectOne" abstract="1"/>
  <xs:complexType name="DirectFalse" abstract="false"/>
  <xs:complexType name="DirectZero" abstract="0"/>
  <xs:complexType name="DirectOmitted"/>
  <xs:complexType name="Base" abstract="true"/>
</xs:schema>`
}

func abstractNamedComplexTypeConsumerSchema(version, model string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + version + `">
  <xs:element name="root" type="r:Abstract"/>
  <xs:complexType name="Abstract" abstract="true"><xs:` + model + `>
    <xs:element name="value" type="xs:integer"/>
  </xs:` + model + `></xs:complexType>
</xs:schema>`
}

func namedComplexTypeDefinition(t *testing.T, schema Schema, namespace, local string) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("%s complex type query count = %d, want 1", local, len(components))
	}
	definition, ok := components[0].ComplexType()
	if !ok {
		t.Fatalf("%s has no completed complex-type view", local)
	}
	return definition
}

func abstractComplexTypeParticleChildren(t *testing.T, particle Particle) []Particle {
	t.Helper()
	switch typed := particle.(type) {
	case SequenceParticle:
		return typed.Particles()
	case ChoiceParticle:
		return typed.Alternatives()
	default:
		t.Fatalf("particle type = %T, want sequence or choice", particle)
		return nil
	}
}

func abstractComplexTypeTestLoc(t *testing.T, input, marker string) Loc {
	t.Helper()
	index := strings.Index(input, marker)
	if index < 0 {
		t.Fatalf("fixture does not contain location marker %q", marker)
	}
	line := 1
	column := 1
	for _, character := range input[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, "root.xsd", line, column)
}
