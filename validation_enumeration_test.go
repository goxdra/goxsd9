package goxsd9_test

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep value-space, diagnostic, ownership, and repeatability assertions together.
func TestValidateInstanceAppliesNamedNumericEnumerationsExactly(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0">
  <xs:element name="count" type="r:Count"/>
  <xs:element name="amount" type="r:Amount"/>
  <xs:simpleType name="Count">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="+0007"/>
      <xs:enumeration value="-0"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Amount">
    <xs:restriction base="xs:decimal">
      <xs:enumeration value="1.2300"/>
      <xs:enumeration value="-0.00"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	for _, test := range []struct {
		name       string
		policy     goxsd9.LanguagePolicy
		versionRef string
	}{
		{name: "xsd10", policy: goxsd9.Strict10, versionRef: "xsd10-datatypes#cvc-enumeration-valid"},
		{name: "xsd11", policy: goxsd9.Strict11, versionRef: "xsd11-datatypes#cvc-enumeration-valid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, root, nil, test.policy)
			count := validationTestElement(t, schema, "count")
			amount := validationTestElement(t, schema, "amount")
			countTypeComponent := validationTestSimpleType(t, schema, "Count")
			amountTypeComponent := validationTestSimpleType(t, schema, "Amount")
			countType, countTypeOK := countTypeComponent.SimpleTypeDefinition()
			if !countTypeOK {
				t.Fatal("Count has no simple type definition view")
			}
			amountType, amountTypeOK := amountTypeComponent.SimpleTypeDefinition()
			if !amountTypeOK {
				t.Fatal("Amount has no simple type definition view")
			}

			for _, input := range []string{
				`<count xmlns="urn:root">7</count>`,
				`<count xmlns="urn:root">-0</count>`,
				`<amount xmlns="urn:root">1.23</amount>`,
				`<amount xmlns="urn:root">0</amount>`,
			} {
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("ValidateInstance(%q): %v", input, err)
				}
			}

			beforeCount := countType.IntegerEnumerationFacets().Declarations()
			beforeAmount := amountType.DecimalEnumerationFacets().Declarations()
			assertEnumerationInstanceViolation(t, schema, `<count xmlns="urn:root">8</count>`, count, countTypeComponent, test.versionRef, countType.IntegerEnumerationFacets().Locations())
			assertEnumerationInstanceViolation(t, schema, `<amount xmlns="urn:root">2.0</amount>`, amount, amountTypeComponent, test.versionRef, amountType.DecimalEnumerationFacets().Locations())
			if got := countType.IntegerEnumerationFacets().Declarations(); !reflect.DeepEqual(got, beforeCount) {
				t.Fatalf("integer enumeration changed after validation: got %v, want %v", got, beforeCount)
			}
			if got := amountType.DecimalEnumerationFacets().Declarations(); !reflect.DeepEqual(got, beforeAmount) {
				t.Fatalf("decimal enumeration changed after validation: got %v, want %v", got, beforeAmount)
			}
		})
	}
}

//nolint:gocognit // Keep direct-choice selection, membership, and evidence assertions together.
func TestValidateInstanceAppliesNumericEnumerationsThroughDirectChoice(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0">
  <xs:element name="item" type="r:Choice"/>
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element name="number" type="r:ChoiceInteger"/>
      <xs:element name="amount" type="r:ChoiceDecimal"/>
    </xs:choice>
  </xs:complexType>
  <xs:simpleType name="ChoiceInteger">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="7"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ChoiceDecimal">
    <xs:restriction base="xs:decimal">
      <xs:enumeration value="1.20"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	schema := validationTestSchemaWithPolicy(t, root, nil, goxsd9.Strict11)
	valid := []string{
		`<item xmlns="urn:root"><number xmlns="">+007</number></item>`,
		`<item xmlns="urn:root"><amount xmlns="">1.2</amount></item>`,
	}
	for _, input := range valid {
		if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
			t.Fatalf("ValidateInstance(%q): %v", input, err)
		}
	}

	item := validationTestElement(t, schema, "item")
	choiceTypeComponent := validationTestComplexType(t, schema, "Choice")
	choiceType, ok := choiceTypeComponent.ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice has no complex type definition view")
	}
	choiceIntegerComponent := validationTestSimpleType(t, schema, "ChoiceInteger")
	choiceInteger, ok := choiceIntegerComponent.SimpleTypeDefinition()
	if !ok {
		t.Fatal("ChoiceInteger has no simple type definition view")
	}
	choiceParticle, ok := choiceType.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatal("Choice does not expose a choice particle")
	}
	alternatives := choiceParticle.Alternatives()
	if len(alternatives) == 0 {
		t.Fatal("Choice has no alternatives")
	}
	numberParticle, ok := alternatives[0].(goxsd9.ElementParticle)
	if !ok {
		t.Fatal("Choice first alternative is not an element particle")
	}
	input := `<item xmlns="urn:root"><number xmlns="">8</number></item>`
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	if diagnostic.Code() != goxsd9.EnumerationValueViolationCode {
		t.Fatalf("Code() = %q, want %q", diagnostic.Code(), goxsd9.EnumerationValueViolationCode)
	}
	if diagnostic.SpecRef() != "xsd11-datatypes#cvc-enumeration-valid" {
		t.Fatalf("SpecRef() = %q, want XSD 1.1 enumeration reference", diagnostic.SpecRef())
	}
	if got, want := diagnostic.Loc(), validationTestNestedTextLoc(t, "instance.xml", input, "number"); got != want {
		t.Fatalf("Loc() = %s, want selected scalar text location %s", got, want)
	}
	if diagnostic.Unwrap() == nil {
		t.Fatal("enumeration diagnostic lost its cause")
	}
	enumerationLocations := choiceInteger.IntegerEnumerationFacets().Locations()
	related := diagnostic.Related()
	if len(related) < len(enumerationLocations) {
		t.Fatalf("Related() = %v, want enumeration locations %v", related, enumerationLocations)
	}
	if got := related[len(related)-len(enumerationLocations):]; !reflect.DeepEqual(got, enumerationLocations) {
		t.Fatalf("Related() suffix = %v, want ordered enumeration locations %v", got, enumerationLocations)
	}
	if !validationTestHasRelated(related, item.Loc()) || !validationTestHasRelated(related, choiceTypeComponent.Loc()) || !validationTestHasRelated(related, numberParticle.Loc()) {
		t.Fatalf("Related() = %v, want global, choice, and selected particle locations", related)
	}
}

func assertEnumerationInstanceViolation(
	t *testing.T,
	schema goxsd9.Schema,
	input string,
	element goxsd9.ElementDeclaration,
	typeComponent goxsd9.Component,
	specRef string,
	enumerationLocations []goxsd9.Loc,
) {
	t.Helper()
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	if diagnostic.Code() != goxsd9.EnumerationValueViolationCode {
		t.Fatalf("Code() = %q, want %q", diagnostic.Code(), goxsd9.EnumerationValueViolationCode)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if diagnostic.Loc() != validationTestTextLoc(t, "instance.xml", input) {
		t.Fatalf("Loc() = %s, want instance text location", diagnostic.Loc())
	}
	if diagnostic.Unwrap() == nil {
		t.Fatal("enumeration diagnostic lost its cause")
	}
	wantRelated := append([]goxsd9.Loc{element.Loc(), typeComponent.Loc()}, enumerationLocations...)
	if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
	}
}

func validationTestElement(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName("urn:root", local)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(components) != 1 {
		t.Fatalf("%s declarations = %d, want one", local, len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("%s has no element declaration view", local)
	}
	return declaration
}

func validationTestSimpleType(t *testing.T, schema goxsd9.Schema, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName("urn:root", local)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s definitions = %d, want one", local, len(components))
	}
	return components[0]
}

func validationTestComplexType(t *testing.T, schema goxsd9.Schema, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName("urn:root", local)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s definitions = %d, want one", local, len(components))
	}
	return components[0]
}

func validationTestNestedTextLoc(t *testing.T, source goxsd9.SourceID, input, local string) goxsd9.Loc {
	t.Helper()
	start := strings.Index(input, "<"+local)
	if start < 0 {
		t.Fatalf("input has no %s element: %q", local, input)
	}
	endOffset := strings.IndexByte(input[start:], '>')
	if endOffset < 0 {
		t.Fatalf("input has no end to %s start tag: %q", local, input)
	}
	return validationTestLoc(t, source, 1, start+endOffset+2)
}
