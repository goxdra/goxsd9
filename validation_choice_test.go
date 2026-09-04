package goxsd9_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const (
	validationChoiceRootNamespace  = "urn:choice-root"
	validationChoiceOtherNamespace = "urn:choice-other"
)

func TestValidateInstanceSupportsGlobalDirectScalarChoices(t *testing.T) {
	cases := []struct {
		name  string
		child string
		value string
	}{
		{name: "built-in integer", child: "builtinInteger", value: "-42"},
		{name: "named decimal", child: "namedDecimal", value: "12.30"},
		{name: "forward integer", child: "forwardInteger", value: "12345"},
		{name: "cross-document decimal", child: "crossDecimal", value: "1.234"},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationChoiceSchema(t, policy)
			before := schema.Components()
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					input := validationChoiceInstance(test.child, test.value)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance: %v", err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("direct-choice validation mutated the completed schema")
			}
		})
	}
}

type nonDefaultDirectChoiceCase struct {
	name          string
	choiceAttrs   string
	elementAttrs  string
	wantElementAt bool
}

func TestValidateInstanceKeepsNonDefaultDirectChoiceUnsupported(t *testing.T) {
	cases := []nonDefaultDirectChoiceCase{
		{name: "choice occurrence", choiceAttrs: ` minOccurs="0"`},
		{name: "alternative occurrence", elementAttrs: ` minOccurs="0"`, wantElementAt: true},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					assertNonDefaultDirectChoiceUnsupported(t, policy, test)
				})
			}
		})
	}
}

func assertNonDefaultDirectChoiceUnsupported(t *testing.T, policy goxsd9.LanguagePolicy, test nonDefaultDirectChoiceCase) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationChoiceRootNamespace + `" targetNamespace="` + validationChoiceRootNamespace + `">
  <xs:complexType name="Choice"><xs:choice` + test.choiceAttrs + `><xs:element name="value" type="xs:integer"` + test.elementAttrs + `/></xs:choice></xs:complexType>
  <xs:element name="choiceRoot" type="r:Choice"/>
</xs:schema>`
	schema := validationTestSchemaWithPolicy(t, root, nil, policy)
	before := schema.Components()
	evidence := validationChoiceEvidenceFor(t, schema)
	input := validationChoiceInstance("value", "1")
	diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	assertNonDefaultDirectChoiceDiagnostic(t, diagnostic, policy, test, evidence)
	if !reflect.DeepEqual(before, schema.Components()) {
		t.Fatal("non-default occurrence validation mutated the completed schema")
	}
}

func assertNonDefaultDirectChoiceDiagnostic(t *testing.T, diagnostic goxsd9.Diagnostic, policy goxsd9.LanguagePolicy, test nonDefaultDirectChoiceCase, evidence validationChoiceEvidence) {
	t.Helper()
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
		t.Fatalf("diagnostic = %s/%q/%q, want unsupported/%s/%q", diagnostic, diagnostic.Code(), diagnostic.Feature(), goxsd9.UnsupportedInstanceValidationCode, goxsd9.FeatureInstanceValidation)
	}
	if !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", diagnostic)
	}
	wantLoc := evidence.choice
	wantRelated := []goxsd9.Loc{evidence.declaration, evidence.complex, evidence.choice}
	if test.wantElementAt {
		wantLoc = evidence.byChild["value"][3]
		wantRelated = evidence.byChild["value"]
	}
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
	if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("diagnostic related = %v, want %v", diagnostic.Related(), wantRelated)
	}
	wantSpec := "xsd11-structures#cvc-elt"
	if policy == goxsd9.Strict10 {
		wantSpec = "xsd10-structures#cvc-elt"
	}
	if diagnostic.SpecRef() != wantSpec {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
	}
}

//nolint:gocognit // Keep the cross-policy wildcard diagnostic contract together.
func TestValidateInstanceRejectsDirectChoiceAttributeWildcardAcrossPolicies(t *testing.T) {
	for _, test := range []struct {
		name     string
		policy   goxsd9.LanguagePolicy
		version  string
		wantSpec string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: "1.1", wantSpec: "xsd11-structures#cvc-elt"},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0", wantSpec: "xsd10-structures#cvc-elt"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1", wantSpec: "xsd11-structures#cvc-elt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationChoiceRootNamespace + `" targetNamespace="` + validationChoiceRootNamespace + `" version="` + test.version + `">
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice><xs:anyAttribute/></xs:complexType>
</xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, nil, test.policy)
			before := schema.Components()
			evidence := validationChoiceEvidenceFor(t, schema)
			choiceName, err := goxsd9.NewQName(validationChoiceRootNamespace, "Choice")
			if err != nil {
				t.Fatalf("NewQName: %v", err)
			}
			choiceComponents := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, choiceName)
			if len(choiceComponents) != 1 {
				t.Fatalf("Choice definitions = %d, want 1", len(choiceComponents))
			}
			definition, ok := choiceComponents[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Choice has no complex type view")
			}
			wildcard, ok := definition.AnyAttribute()
			if !ok {
				t.Fatal("Choice has no anyAttribute wildcard")
			}

			input := validationChoiceInstance("value", "1")
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
				t.Fatalf("diagnostic = %s/%q/%q, want unsupported instance-validation diagnostic", diagnostic, diagnostic.Code(), diagnostic.Feature())
			}
			if !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
				t.Fatalf("diagnostic does not match ErrUnsupported: %v", diagnostic)
			}
			if diagnostic.Loc() != wildcard.Loc() {
				t.Fatalf("diagnostic location = %s, want wildcard location %s", diagnostic.Loc(), wildcard.Loc())
			}
			wantRelated := []goxsd9.Loc{evidence.declaration, evidence.complex, evidence.choice, wildcard.Loc()}
			if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
				t.Fatalf("diagnostic related = %v, want %v", diagnostic.Related(), wantRelated)
			}
			if diagnostic.SpecRef() != test.wantSpec {
				t.Fatalf("diagnostic spec reference = %q, want %q", diagnostic.SpecRef(), test.wantSpec)
			}
			if diagnostic.Unwrap() == nil {
				t.Fatal("attribute wildcard diagnostic lost its cause")
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("attribute wildcard validation mutated the completed schema")
			}
		})
	}
}

func validationChoiceSchema(t *testing.T, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationChoiceRootNamespace + `" xmlns:o="` + validationChoiceOtherNamespace + `" targetNamespace="` + validationChoiceRootNamespace + `" version="` + version + `">
  <xs:import namespace="` + validationChoiceOtherNamespace + `" schemaLocation="choice-other.xsd"/>
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="builtinInteger" type="xs:integer"/>
    <xs:element name="namedDecimal" type="r:NamedDecimal"/>
    <xs:element name="forwardInteger" type="r:ForwardInteger"/>
    <xs:element name="crossDecimal" type="o:CrossDecimal"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="ForwardInteger"><xs:restriction base="xs:integer"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="NamedDecimal"><xs:restriction base="xs:decimal"><xs:totalDigits value="4"/><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="` + validationChoiceOtherNamespace + `" version="` + version + `"><xs:simpleType name="CrossDecimal"><xs:restriction base="xs:decimal"><xs:fractionDigits value="3"/></xs:restriction></xs:simpleType></xs:schema>`
	return validationTestSchemaWithPolicy(t, root, map[string]validationTestFixture{
		"choice-other.xsd": {id: "choice-other.xsd", contents: other},
	}, policy)
}

func validationChoiceInstance(child, value string) string {
	return fmt.Sprintf(
		"<alias:choiceRoot xmlns:alias=\"%s\">\n  <%s xmlns=\"\"> \n%s\n  </%s>\n</alias:choiceRoot>",
		validationChoiceRootNamespace,
		child,
		value,
		child,
	)
}

//nolint:gocognit // Keep the structural diagnostic matrix and evidence checks together.
func TestValidateInstanceReportsGlobalDirectChoiceStructure(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		marker string
		loc    func(*testing.T, string, string) goxsd9.Loc
	}{
		{
			name:  "missing",
			input: `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"/>`,
			loc:   func(t *testing.T, _, _ string) goxsd9.Loc { return validationTestLoc(t, "instance.xml", 1, 1) },
		},
		{
			name:   "unknown child",
			input:  `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><unknown xmlns="">1</unknown></alias:choiceRoot>`,
			marker: "<unknown",
			loc:    validationChoiceMarkerLoc,
		},
		{
			name:   "unknown expanded QName",
			input:  `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><builtinInteger xmlns="` + validationChoiceRootNamespace + `">1</builtinInteger></alias:choiceRoot>`,
			marker: "<builtinInteger",
			loc:    validationChoiceMarkerLoc,
		},
		{
			name:   "second child",
			input:  `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><builtinInteger xmlns="">1</builtinInteger><namedDecimal xmlns="">1.20</namedDecimal></alias:choiceRoot>`,
			marker: "<namedDecimal",
			loc:    validationChoiceMarkerLoc,
		},
		{
			name:   "repeated child",
			input:  `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><builtinInteger xmlns="">1</builtinInteger><builtinInteger xmlns="">2</builtinInteger></alias:choiceRoot>`,
			marker: "<builtinInteger",
			loc:    validationChoiceLastMarkerLoc,
		},
		{
			name:  "non-whitespace parent text",
			input: `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `">text<builtinInteger xmlns="">1</builtinInteger></alias:choiceRoot>`,
			loc:   validationChoiceTextLoc,
		},
		{
			name:   "nested content",
			input:  `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><builtinInteger xmlns=""><nested/></builtinInteger></alias:choiceRoot>`,
			marker: "<nested",
			loc:    validationChoiceMarkerLoc,
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationChoiceSchema(t, policy)
			evidence := validationChoiceEvidenceFor(t, schema)
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureInvalid {
						t.Fatalf("Class() = %q, want %q", diagnostic.Class(), goxsd9.FailureInvalid)
					}
					if diagnostic.Code() != goxsd9.InvalidInstanceChoiceCode {
						t.Fatalf("Code() = %q, want %q", diagnostic.Code(), goxsd9.InvalidInstanceChoiceCode)
					}
					if diagnostic.Loc() != test.loc(t, test.input, test.marker) {
						t.Fatalf("Loc() = %s, want marker location", diagnostic.Loc())
					}
					wantRelated := evidence.related
					if test.name == "nested content" {
						wantRelated = evidence.byChild["builtinInteger"]
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}
					if diagnostic.Unwrap() == nil {
						t.Fatal("direct-choice structural diagnostic lost its cause")
					}
					wantSpec := "xsd11-structures#cvc-elt"
					if policy == goxsd9.Strict10 {
						wantSpec = "xsd10-structures#cvc-elt"
					}
					if diagnostic.SpecRef() != wantSpec {
						t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), wantSpec)
					}
				})
			}
		})
	}
}

type validationChoiceEvidence struct {
	declaration goxsd9.Loc
	complex     goxsd9.Loc
	choice      goxsd9.Loc
	related     []goxsd9.Loc
	byChild     map[string][]goxsd9.Loc
}

func validationChoiceEvidenceFor(t *testing.T, schema goxsd9.Schema) validationChoiceEvidence {
	t.Helper()
	rootName, err := goxsd9.NewQName(validationChoiceRootNamespace, "choiceRoot")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	declarations := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
	if len(declarations) != 1 {
		t.Fatalf("choiceRoot declarations = %d, want 1", len(declarations))
	}
	choiceName, err := goxsd9.NewQName(validationChoiceRootNamespace, "Choice")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	complexTypes := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, choiceName)
	if len(complexTypes) != 1 {
		t.Fatalf("Choice definitions = %d, want 1", len(complexTypes))
	}
	definition, ok := complexTypes[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice has no complex type view")
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatal("Choice has no ChoiceParticle")
	}
	related := make([]goxsd9.Loc, 0, 3+len(choice.Alternatives()))
	related = append(related, declarations[0].Loc(), complexTypes[0].Loc(), choice.Loc())
	byChild := make(map[string][]goxsd9.Loc)
	for _, particle := range choice.Alternatives() {
		element, ok := particle.(goxsd9.ElementParticle)
		if !ok {
			t.Fatalf("choice alternative type = %T, want ElementParticle", particle)
		}
		related = append(related, element.Loc())
		byChild[element.Name().Local()] = []goxsd9.Loc{declarations[0].Loc(), complexTypes[0].Loc(), choice.Loc(), element.Loc()}
	}
	return validationChoiceEvidence{
		declaration: declarations[0].Loc(),
		complex:     complexTypes[0].Loc(),
		choice:      choice.Loc(),
		related:     related,
		byChild:     byChild,
	}
}

func validationChoiceMarkerLoc(t *testing.T, input, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(input, marker)
	if index < 0 {
		t.Fatalf("input has no marker %q: %q", marker, input)
	}
	return validationTestLoc(t, "instance.xml", 1, index+1)
}

func validationChoiceLastMarkerLoc(t *testing.T, input, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.LastIndex(input, marker)
	if index < 0 {
		t.Fatalf("input has no marker %q: %q", marker, input)
	}
	return validationTestLoc(t, "instance.xml", 1, index+1)
}

func validationChoiceTextLoc(t *testing.T, input, _ string) goxsd9.Loc {
	t.Helper()
	end := strings.IndexByte(input, '>')
	return validationTestLoc(t, "instance.xml", 1, end+2)
}

//nolint:gocognit // Keep lexical, facet, location, cause, and reference checks together.
func TestValidateInstanceReportsDirectChoiceScalarDiagnostics(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationChoiceSchema(t, policy)
			evidence := validationChoiceEvidenceFor(t, schema)

			lexical := `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><builtinInteger xmlns="">1.0</builtinInteger></alias:choiceRoot>`
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(lexical))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidIntegerLexicalCode {
				t.Fatalf("lexical diagnostic = %s (%q), want invalid integer lexical", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationChoiceElementTextLoc(t, lexical, "builtinInteger") {
				t.Fatalf("lexical Loc() = %s, want selected scalar text location", diagnostic.Loc())
			}
			if diagnostic.SpecRef() != "xsd11-datatypes#integer" {
				t.Fatalf("lexical SpecRef() = %q, want xsd11-datatypes#integer", diagnostic.SpecRef())
			}
			if !reflect.DeepEqual(diagnostic.Related(), evidence.byChild["builtinInteger"]) {
				t.Fatalf("lexical Related() = %v, want %v", diagnostic.Related(), evidence.byChild["builtinInteger"])
			}

			facet := `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><namedDecimal xmlns="">0.123</namedDecimal></alias:choiceRoot>`
			diagnostic = validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(facet))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.DigitFacetValueViolationCode {
				t.Fatalf("facet diagnostic = %s (%q), want digit facet violation", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationChoiceElementTextLoc(t, facet, "namedDecimal") {
				t.Fatalf("facet Loc() = %s, want selected scalar text location", diagnostic.Loc())
			}
			wantSpec := "xsd11-datatypes#cvc-fractionDigits-valid"
			if policy == goxsd9.Strict10 {
				wantSpec = "xsd10-datatypes#cvc-fractionDigits-valid"
			}
			if diagnostic.SpecRef() != wantSpec {
				t.Fatalf("facet SpecRef() = %q, want %q", diagnostic.SpecRef(), wantSpec)
			}
			if diagnostic.Unwrap() == nil {
				t.Fatal("facet diagnostic lost its cause")
			}
			typeName, err := goxsd9.NewQName(validationChoiceRootNamespace, "NamedDecimal")
			if err != nil {
				t.Fatalf("NewQName: %v", err)
			}
			types := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, typeName)
			if len(types) != 1 {
				t.Fatalf("NamedDecimal definitions = %d, want 1", len(types))
			}
			definition, ok := types[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("NamedDecimal has no simple type view")
			}
			fractionLoc, ok := definition.DigitFacets().FractionDigitsLoc()
			if !ok {
				t.Fatal("NamedDecimal has no fractionDigits location")
			}
			wantRelated := append([]goxsd9.Loc{}, evidence.byChild["namedDecimal"]...)
			wantRelated = append(wantRelated, types[0].Loc(), fractionLoc)
			if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
				t.Fatalf("facet Related() = %v, want %v", diagnostic.Related(), wantRelated)
			}
		})
	}
}

func validationChoiceElementTextLoc(t *testing.T, input, local string) goxsd9.Loc {
	t.Helper()
	start := strings.Index(input, "<"+local)
	if start < 0 {
		t.Fatalf("input has no <%s> start tag: %q", local, input)
	}
	end := strings.IndexByte(input[start:], '>')
	if end < 0 {
		t.Fatalf("input has no <%s> start-tag end: %q", local, input)
	}
	return validationTestLoc(t, "instance.xml", 1, start+end+2)
}

//nolint:gocognit // Keep repeated diagnostics and immutability checks together.
func TestValidateInstanceRejectsEmptyModeledChoiceDeterministically(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationEmptyChoiceSchema(t, policy)
			before := schema.Components()
			evidence := validationEmptyChoiceEvidence(t, schema)
			input := `<alias:emptyRoot xmlns:alias="` + validationChoiceRootNamespace + `"/>`
			first := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			second := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			for _, diagnostic := range []goxsd9.Diagnostic{first, second} {
				if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidInstanceChoiceCode {
					t.Fatalf("empty-choice diagnostic = %s (%q), want invalid %q", diagnostic, diagnostic.Code(), goxsd9.InvalidInstanceChoiceCode)
				}
				if diagnostic.Loc() != validationTestLoc(t, "instance.xml", 1, 1) {
					t.Fatalf("empty-choice Loc() = %s, want instance.xml:1:1", diagnostic.Loc())
				}
				if !reflect.DeepEqual(diagnostic.Related(), evidence) {
					t.Fatalf("empty-choice Related() = %v, want %v", diagnostic.Related(), evidence)
				}
				if diagnostic.Unwrap() == nil {
					t.Fatal("empty-choice diagnostic lost its cause")
				}
				wantSpec := "xsd11-structures#cvc-elt"
				if policy == goxsd9.Strict10 {
					wantSpec = "xsd10-structures#cvc-elt"
				}
				if diagnostic.SpecRef() != wantSpec {
					t.Fatalf("empty-choice SpecRef() = %q, want %q", diagnostic.SpecRef(), wantSpec)
				}
			}
			if first.Error() != second.Error() || !reflect.DeepEqual(first.Related(), second.Related()) || first.SpecRef() != second.SpecRef() {
				t.Fatalf("repeated empty-choice diagnostics differ: first %v, second %v", first, second)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("empty-choice validation mutated the completed schema")
			}
		})
	}
}

func validationEmptyChoiceSchema(t *testing.T, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationChoiceRootNamespace + `" targetNamespace="` + validationChoiceRootNamespace + `"><xs:complexType name="Empty"><xs:choice/></xs:complexType><xs:element name="emptyRoot" type="r:Empty"/></xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
}

func validationEmptyChoiceEvidence(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	rootName, err := goxsd9.NewQName(validationChoiceRootNamespace, "emptyRoot")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	root := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
	if len(root) != 1 {
		t.Fatalf("emptyRoot declarations = %d, want 1", len(root))
	}
	typeName, err := goxsd9.NewQName(validationChoiceRootNamespace, "Empty")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	types := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, typeName)
	if len(types) != 1 {
		t.Fatalf("Empty definitions = %d, want 1", len(types))
	}
	definition, ok := types[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Empty has no complex type view")
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatal("Empty has no ChoiceParticle")
	}
	return []goxsd9.Loc{root[0].Loc(), types[0].Loc(), choice.Loc()}
}

//nolint:gocognit // Keep unsupported direct-choice shape classifications together.
func TestSchemaBuildKeepsDirectChoiceUnsupportedShapes(t *testing.T) {
	cases := []struct {
		name  string
		model string
	}{
		{name: "nested sequence", model: `<xs:choice><xs:sequence/></xs:choice>`},
		{name: "wildcard", model: `<xs:choice><xs:any/></xs:choice>`},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationChoiceRootNamespace + `" targetNamespace="` + validationChoiceRootNamespace + `"><xs:complexType name="Choice">` + test.model + `</xs:complexType><xs:element name="choiceRoot" type="r:Choice"/></xs:schema>`
					source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
					if err != nil {
						t.Fatalf("NewResolvedSource: %v", err)
					}
					schema, err := goxsd9.ParseSchemaWithPolicy(source, validationTestResolver{fixtures: nil}, policy)
					if err == nil {
						t.Fatal("ParseSchemaWithPolicy accepted an unsupported direct-choice shape")
					}
					if len(schema.Components()) != 0 {
						t.Fatalf("failed parse returned %d components, want none", len(schema.Components()))
					}
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Feature() != goxsd9.FeatureSchemaSyntax {
						t.Fatalf("diagnostic = %s (%q), want schema-syntax unsupported", diagnostic, diagnostic.Feature())
					}
					if diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode {
						t.Fatalf("Code() = %q, want %q", diagnostic.Code(), goxsd9.UnsupportedSchemaSyntaxCode)
					}
					if diagnostic.Loc().IsZero() || diagnostic.SpecRef() == "" {
						t.Fatalf("unsupported diagnostic evidence = location %s, spec %q", diagnostic.Loc(), diagnostic.SpecRef())
					}
					if !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
					}
				})
			}
		})
	}
}
