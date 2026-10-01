package goxsd9

import "testing"

//nolint:gocognit // Check selected member identity and each typed atomic value.
func TestPrecisionDecimalUnionRetainsFirstMemberValueSemantics(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:simpleType name="Scaled"><xs:restriction base="xs:precisionDecimal"><xs:minScale value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="U"><xs:union memberTypes="Scaled xs:string xs:integer"/></xs:simpleType><xs:element name="value" type="U"/></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	name := mustTestQName(t, "", "value")
	components := schema.FindKind(ComponentKindElementDeclaration, name)
	if len(components) != 1 {
		t.Fatalf("element lookup = %d", len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("element view is absent")
	}
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatal("element type reference is absent")
	}
	loc := mustTestLoc(t, "instance.xml", 1, 1)
	plan, err := instanceVarietyPlanFor(schema, reference, []Loc{declaration.Loc()}, loc, XSDVersion11)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		lexical string
		member  int
		kind    schemaSimpleTypeAtomicKind
	}{
		{"-0.00", 0, schemaSimpleTypeAtomicPrecisionDecimal},
		{"1", 1, schemaSimpleTypeAtomicString},
		{"once", 1, schemaSimpleTypeAtomicString},
	} {
		value, valueErr := validateInstanceVarietyValue(syntaxName{local: "value"}, test.lexical, loc, plan)
		if valueErr != nil {
			t.Fatalf("%q: %v", test.lexical, valueErr)
		}
		if value.activeMember != test.member || value.selected == nil || value.selected.kind != test.kind {
			t.Fatalf("%q selected member %d/%v, want %d/%v", test.lexical, value.activeMember, value.selected, test.member, test.kind)
		}
		if test.kind == schemaSimpleTypeAtomicPrecisionDecimal {
			finite, valid := value.selected.atomic.(precisionDecimalFinite)
			if !valid || finite.sign != precisionDecimalSignNegative || finite.scale.String() != "2" {
				t.Fatalf("%q selected value %#v, want negative zero at scale 2", test.lexical, value.selected.atomic)
			}
		}
		if test.kind == schemaSimpleTypeAtomicString && value.selected.atomic != test.lexical {
			t.Fatalf("%q selected atomic string %#v", test.lexical, value.selected.atomic)
		}
	}
}
