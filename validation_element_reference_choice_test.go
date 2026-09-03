package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const (
	validationReferenceChoiceNamespace      = "urn:reference-choice"
	validationReferenceChoiceOtherNamespace = "urn:reference-choice-other"
)

type validationReferenceChoiceAlternativeEvidence struct {
	name         string
	related      []goxsd9.Loc
	withoutFacet []goxsd9.Loc
}

type validationReferenceChoiceEvidence struct {
	declaration  goxsd9.Loc
	complex      goxsd9.Loc
	choice       goxsd9.Loc
	related      []goxsd9.Loc
	alternatives []validationReferenceChoiceAlternativeEvidence
}

//nolint:gocognit,funlen // Keep cross-policy graph coverage, scalar diagnostics, and immutability together.
func TestValidateInstanceSupportsDirectNumericReferenceChoicesAcrossPolicies(t *testing.T) {
	valid := []struct {
		name   string
		child  string
		prefix string
		value  string
	}{
		{name: "backward built-in integer", child: "backward", prefix: "r", value: "-42"},
		{name: "forward named integer", child: "forward", prefix: "r", value: "12345"},
		{name: "included chameleon decimal", child: "included", prefix: "r", value: "12.30"},
		{name: "directly imported named decimal", child: "imported", prefix: "o", value: "1.234"},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationReferenceChoiceSchema(t, policy)
			evidence := validationReferenceChoiceEvidenceFor(t, schema)
			before := schema.Components()

			for _, test := range valid {
				t.Run("valid/"+test.name, func(t *testing.T) {
					input := validationReferenceChoiceInstance(test.prefix, test.child, test.value)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}

			lexical := validationReferenceChoiceInstance("r", "backward", "1.0")
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(lexical))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidIntegerLexicalCode {
				t.Fatalf("reference lexical diagnostic = %s/%q, want invalid integer lexical", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationReferenceChoiceTextLoc(t, lexical, "r", "backward") {
				t.Fatalf("reference lexical location = %s, want instance text location", diagnostic.Loc())
			}
			if diagnostic.SpecRef() != "xsd11-datatypes#integer" {
				t.Fatalf("reference lexical specification = %q, want xsd11-datatypes#integer", diagnostic.SpecRef())
			}
			if !reflect.DeepEqual(diagnostic.Related(), validationReferenceChoiceAlternativeRelated(t, evidence, "backward", false)) {
				t.Fatalf("reference lexical related = %v, want target evidence order", diagnostic.Related())
			}

			forwardFacet := validationReferenceChoiceInstance("r", "forward", "123456")
			diagnostic = validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(forwardFacet))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.DigitFacetValueViolationCode {
				t.Fatalf("reference integer facet diagnostic = %s/%q, want digit facet violation", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationReferenceChoiceTextLoc(t, forwardFacet, "r", "forward") {
				t.Fatalf("reference integer facet location = %s, want instance text location", diagnostic.Loc())
			}
			if diagnostic.SpecRef() != validationReferenceChoiceFacetSpec(policy, "totalDigits") {
				t.Fatalf("reference integer facet specification = %q, want policy-specific totalDigits reference", diagnostic.SpecRef())
			}
			wantRelated := validationReferenceChoiceAlternativeRelated(t, evidence, "forward", true)
			if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
				t.Fatalf("reference integer facet related = %v, want target evidence order %v", diagnostic.Related(), wantRelated)
			}

			decimalLexical := validationReferenceChoiceInstance("o", "imported", "1e+")
			diagnostic = validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(decimalLexical))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidDecimalLexicalCode {
				t.Fatalf("reference decimal lexical diagnostic = %s/%q, want invalid decimal lexical", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationReferenceChoiceTextLoc(t, decimalLexical, "o", "imported") {
				t.Fatalf("reference decimal lexical location = %s, want instance text location", diagnostic.Loc())
			}
			if diagnostic.SpecRef() != validationReferenceChoiceDecimalSpec(policy) {
				t.Fatalf("reference decimal lexical specification = %q, want policy-specific decimal reference", diagnostic.SpecRef())
			}
			if !reflect.DeepEqual(diagnostic.Related(), validationReferenceChoiceAlternativeRelated(t, evidence, "imported", false)) {
				t.Fatalf("reference decimal lexical related = %v, want target evidence order", diagnostic.Related())
			}

			importedFacet := validationReferenceChoiceInstance("o", "imported", "1.2345")
			diagnostic = validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(importedFacet))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.DigitFacetValueViolationCode {
				t.Fatalf("reference decimal facet diagnostic = %s/%q, want digit facet violation", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationReferenceChoiceTextLoc(t, importedFacet, "o", "imported") {
				t.Fatalf("reference decimal facet location = %s, want instance text location", diagnostic.Loc())
			}
			if diagnostic.SpecRef() != validationReferenceChoiceFacetSpec(policy, "fractionDigits") {
				t.Fatalf("reference decimal facet specification = %q, want policy-specific fractionDigits reference", diagnostic.SpecRef())
			}
			if !reflect.DeepEqual(diagnostic.Related(), validationReferenceChoiceAlternativeRelated(t, evidence, "imported", true)) {
				t.Fatalf("reference decimal facet related = %v, want target evidence order", diagnostic.Related())
			}

			unknown := `<r:choiceRoot xmlns:r="` + validationReferenceChoiceNamespace + `" xmlns:o="` + validationReferenceChoiceOtherNamespace + `"><r:unknown>1</r:unknown></r:choiceRoot>`
			diagnostic = validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(unknown))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidInstanceChoiceCode {
				t.Fatalf("reference wrong-alternative diagnostic = %s/%q, want invalid choice", diagnostic, diagnostic.Code())
			}
			if !reflect.DeepEqual(diagnostic.Related(), evidence.related) {
				t.Fatalf("reference wrong-alternative related = %v, want ordered all alternatives", diagnostic.Related())
			}

			repeated := `<r:choiceRoot xmlns:r="` + validationReferenceChoiceNamespace + `" xmlns:o="` + validationReferenceChoiceOtherNamespace + `"><r:backward>-1</r:backward><r:backward>2</r:backward></r:choiceRoot>`
			diagnostic = validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(repeated))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidInstanceChoiceCode {
				t.Fatalf("reference repetition diagnostic = %s/%q, want invalid choice", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationReferenceChoiceLastMarkerLoc(t, repeated, "<r:backward>") {
				t.Fatalf("reference repetition location = %s, want second child", diagnostic.Loc())
			}

			firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(importedFacet)))
			secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(importedFacet)))
			first := validationTestDiagnostic(t, firstErr)
			second := validationTestDiagnostic(t, secondErr)
			if first.Error() != second.Error() || first.Code() != second.Code() || first.Loc() != second.Loc() || first.SpecRef() != second.SpecRef() || !reflect.DeepEqual(first.Related(), second.Related()) {
				t.Fatalf("repeated reference diagnostics differ: first %v, second %v", firstErr, secondErr)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("reference-choice validation mutated the completed schema")
			}
		})
	}
}

func validationReferenceChoiceSchema(t *testing.T, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationReferenceChoiceNamespace + `" xmlns:o="` + validationReferenceChoiceOtherNamespace + `" xmlns="` + validationReferenceChoiceNamespace + `" targetNamespace="` + validationReferenceChoiceNamespace + `" version="` + version + `">
  <xs:include schemaLocation="reference-choice-chameleon.xsd"/>
  <xs:import namespace="` + validationReferenceChoiceOtherNamespace + `" schemaLocation="reference-choice-other.xsd"/>
  <xs:element name="backward" type="xs:integer"/>
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>
    <xs:element ref="r:backward"/>
    <xs:element ref="r:forward"/>
    <xs:element ref="included"/>
    <xs:element ref="o:imported"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="ForwardInteger"><xs:restriction base="xs:integer"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType>
  <xs:element name="forward" type="r:ForwardInteger"/>
</xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `"><xs:element name="included" type="xs:decimal"/></xs:schema>`
	other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationReferenceChoiceOtherNamespace + `" targetNamespace="` + validationReferenceChoiceOtherNamespace + `" version="` + version + `"><xs:simpleType name="ImportedDecimal"><xs:restriction base="xs:decimal"><xs:fractionDigits value="3"/></xs:restriction></xs:simpleType><xs:element name="imported" type="o:ImportedDecimal"/></xs:schema>`
	return validationTestSchemaWithPolicy(t, root, map[string]validationTestFixture{
		"reference-choice-chameleon.xsd": {id: "reference-choice-chameleon.xsd", contents: chameleon},
		"reference-choice-other.xsd":     {id: "reference-choice-other.xsd", contents: other},
	}, policy)
}

//nolint:gocognit // Keep ordered target and facet evidence reconstruction together.
func validationReferenceChoiceEvidenceFor(t *testing.T, schema goxsd9.Schema) validationReferenceChoiceEvidence {
	t.Helper()
	rootComponent := validationReferenceChoiceComponent(t, schema, goxsd9.ComponentKindElementDeclaration, "choiceRoot")
	complexComponent := validationReferenceChoiceComponent(t, schema, goxsd9.ComponentKindComplexTypeDefinition, "Choice")
	definition, ok := complexComponent.ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice has no complex type definition view")
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatal("Choice has no direct choice particle")
	}
	evidence := validationReferenceChoiceEvidence{
		declaration: rootComponent.Loc(),
		complex:     complexComponent.Loc(),
		choice:      choice.Loc(),
		related:     []goxsd9.Loc{rootComponent.Loc(), complexComponent.Loc(), choice.Loc()},
	}
	for _, particle := range choice.Alternatives() {
		reference, ok := particle.(goxsd9.ElementReferenceParticle)
		if !ok {
			t.Fatalf("Choice alternative = %T, want ElementReferenceParticle", particle)
		}
		target, ok := schema.Lookup(reference.TargetID())
		if !ok {
			t.Fatalf("reference %q target %v is missing", reference.Name(), reference.TargetID())
		}
		evidence.related = append(evidence.related, reference.Loc())
		related := []goxsd9.Loc{rootComponent.Loc(), complexComponent.Loc(), choice.Loc(), reference.Loc(), target.Loc()}
		targetDeclaration, ok := target.ElementDeclaration()
		if !ok {
			t.Fatalf("reference %q target has no element declaration view", reference.Name())
		}
		withoutFacet := append([]goxsd9.Loc(nil), related...)
		typeID, hasTypeID := targetDeclaration.TypeID()
		if hasTypeID {
			typeComponent, typeOK := schema.Lookup(typeID)
			if !typeOK {
				t.Fatalf("reference %q type %v is missing", reference.Name(), typeID)
			}
			if typeComponent.Kind() == goxsd9.ComponentKindSimpleTypeDefinition {
				related = append(related, typeComponent.Loc())
				withoutFacet = append([]goxsd9.Loc(nil), related...)
				typeDefinition, typeDefinitionOK := typeComponent.SimpleTypeDefinition()
				if !typeDefinitionOK {
					t.Fatalf("reference %q type has no simple type view", reference.Name())
				}
				facets := typeDefinition.DigitFacets()
				if facetLoc, present := facets.TotalDigitsLoc(); present {
					if !facetLoc.IsZero() {
						related = append(related, facetLoc)
					}
				}
				if facetLoc, present := facets.FractionDigitsLoc(); present {
					if !facetLoc.IsZero() {
						related = append(related, facetLoc)
					}
				}
			}
		}
		evidence.alternatives = append(evidence.alternatives, validationReferenceChoiceAlternativeEvidence{
			name:         reference.Name().Local(),
			related:      related,
			withoutFacet: withoutFacet,
		})
	}
	return evidence
}

func validationReferenceChoiceAlternativeRelated(t *testing.T, evidence validationReferenceChoiceEvidence, name string, includeFacets bool) []goxsd9.Loc {
	t.Helper()
	for _, alternative := range evidence.alternatives {
		if alternative.name == name {
			if !includeFacets {
				return alternative.withoutFacet
			}
			return alternative.related
		}
	}
	t.Fatalf("reference-choice evidence has no alternative %q", name)
	return nil
}

func validationReferenceChoiceComponent(t *testing.T, schema goxsd9.Schema, kind goxsd9.ComponentKind, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName(validationReferenceChoiceNamespace, local)
	if err != nil {
		t.Fatalf("NewQName(%q): %v", local, err)
	}
	components := schema.FindKind(kind, name)
	if len(components) != 1 {
		t.Fatalf("%s %q components = %d, want one", kind, local, len(components))
	}
	return components[0]
}

func validationReferenceChoiceInstance(prefix, child, value string) string {
	return `<r:choiceRoot xmlns:r="` + validationReferenceChoiceNamespace + `" xmlns:o="` + validationReferenceChoiceOtherNamespace + `"><` + prefix + `:` + child + `>` + value + `</` + prefix + `:` + child + `></r:choiceRoot>`
}

func validationReferenceChoiceTextLoc(t *testing.T, input, prefix, child string) goxsd9.Loc {
	t.Helper()
	return validationReferenceChoiceMarkerTextLoc(t, input, "<"+prefix+":"+child+">")
}

func validationReferenceChoiceLastMarkerLoc(t *testing.T, input, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.LastIndex(input, marker)
	if index < 0 {
		t.Fatalf("input has no marker %q", marker)
	}
	return validationTestLoc(t, "instance.xml", 1, index+1)
}

func validationReferenceChoiceMarkerTextLoc(t *testing.T, input, marker string) goxsd9.Loc {
	t.Helper()
	start := strings.Index(input, marker)
	if start < 0 {
		t.Fatalf("input has no marker %q", marker)
	}
	end := strings.IndexByte(input[start:], '>')
	if end < 0 {
		t.Fatalf("input has no start-tag end for marker %q", marker)
	}
	return validationTestLoc(t, "instance.xml", 1, start+end+2)
}

func validationReferenceChoiceFacetSpec(policy goxsd9.LanguagePolicy, facet string) string {
	version := "xsd11"
	if policy == goxsd9.Strict10 {
		version = "xsd10"
	}
	return version + "-datatypes#cvc-" + facet + "-valid"
}

func validationReferenceChoiceDecimalSpec(policy goxsd9.LanguagePolicy) string {
	version := "xsd11"
	if policy == goxsd9.Strict10 {
		version = "xsd10"
	}
	return version + "-datatypes#decimal"
}

type validationReferenceChoiceUnsupportedCase struct {
	name               string
	body               string
	wantInstanceLoc    bool
	wantTargetEvidence bool
}

//nolint:gocognit,funlen // Keep the explicit unsupported-shape matrix and location checks together.
func TestValidateInstanceKeepsReferenceChoiceExclusionsExplicit(t *testing.T) {
	cases := []validationReferenceChoiceUnsupportedCase{
		{
			name:            "boolean target",
			body:            `<xs:element name="target" type="xs:boolean"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "named boolean target",
			body:            `<xs:element name="target" type="r:Flag"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType><xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "built-in precisionDecimal target",
			body:            `<xs:element name="target" type="xs:precisionDecimal"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "named precisionDecimal target",
			body:            `<xs:element name="target" type="r:Precision"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType><xs:simpleType name="Precision"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "abstract target",
			body:            `<xs:element name="target" type="xs:integer" abstract="true"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "nillable target",
			body:            `<xs:element name="target" type="xs:decimal" nillable="true"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:               "reference occurrence",
			body:               `<xs:element name="target" type="xs:integer"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target" minOccurs="0"/></xs:choice></xs:complexType>`,
			wantTargetEvidence: false,
		},
		{
			name:               "choice occurrence",
			body:               `<xs:element name="target" type="xs:integer"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice minOccurs="0"><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantTargetEvidence: false,
		},
		{
			name:               "mixed local and reference alternatives",
			body:               `<xs:element name="target" type="xs:integer"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/><xs:element name="local" type="xs:integer"/></xs:choice></xs:complexType>`,
			wantTargetEvidence: false,
		},
		{
			name:               "reference sequence",
			body:               `<xs:element name="target" type="xs:integer"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:sequence><xs:element ref="r:target"/></xs:sequence></xs:complexType>`,
			wantTargetEvidence: false,
		},
		{
			name:               "reference choice attributes",
			body:               `<xs:element name="target" type="xs:integer"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>`,
			wantTargetEvidence: false,
		},
		{
			name:            "anonymous inline target",
			body:            `<xs:element name="target"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "broader complex target",
			body:            `<xs:element name="target" type="r:Target"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType><xs:complexType name="Target"><xs:sequence/></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
		{
			name:            "substitution-group target",
			body:            `<xs:element name="head" type="xs:integer"/><xs:element name="target" type="xs:integer" substitutionGroup="r:head"/><xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>`,
			wantInstanceLoc: true, wantTargetEvidence: true,
		},
	}
	for index := range cases {
		test := cases[index]
		for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
			if strings.Contains(test.name, "precisionDecimal") && policy == goxsd9.Strict10 {
				continue
			}
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				schema := validationReferenceChoiceShapeSchema(t, test.body, policy)
				before := schema.Components()
				input := validationReferenceChoiceInstance("r", "target", "1")
				err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
				diagnostic := validationTestDiagnostic(t, err)
				if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
					t.Fatalf("diagnostic = %s/%q/%q, want explicit unsupported instance validation", diagnostic, diagnostic.Code(), diagnostic.Feature())
				}
				if !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
				}
				if diagnostic.Loc().IsZero() || diagnostic.SpecRef() == "" {
					t.Fatalf("diagnostic evidence = %s/%q, want located specification-backed failure", diagnostic.Loc(), diagnostic.SpecRef())
				}
				if test.wantInstanceLoc && diagnostic.Loc().Source() != "instance.xml" {
					t.Fatalf("diagnostic location = %s, want instance-primary location", diagnostic.Loc())
				}
				if test.wantTargetEvidence {
					evidence := validationReferenceChoiceSingleEvidence(t, schema)
					if len(diagnostic.Related()) < len(evidence) || !reflect.DeepEqual(diagnostic.Related()[:len(evidence)], evidence) {
						t.Fatalf("diagnostic related = %v, want target evidence prefix %v", diagnostic.Related(), evidence)
					}
				}
				if !reflect.DeepEqual(before, schema.Components()) {
					t.Fatal("unsupported reference-choice validation mutated the completed schema")
				}
			})
		}
	}
}

func validationReferenceChoiceShapeSchema(t *testing.T, body string, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationReferenceChoiceNamespace + `" targetNamespace="` + validationReferenceChoiceNamespace + `" version="` + version + `">` + body + `</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
}

func validationReferenceChoiceSingleEvidence(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	evidence := validationReferenceChoiceEvidenceFor(t, schema)
	if len(evidence.alternatives) != 1 {
		t.Fatalf("reference-choice alternatives = %d, want one", len(evidence.alternatives))
	}
	return evidence.alternatives[0].related
}
