package goxsd9_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const attributePrecisionSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1">
  <xs:simpleType name="Positive"><xs:restriction base="xs:precisionDecimal"><xs:minExclusive value="0"/></xs:restriction></xs:simpleType>
  <xs:complexType name="Named"><xs:sequence/><xs:attribute name="value" type="Positive" use="required"/></xs:complexType>
  <xs:complexType name="Text"><xs:simpleContent><xs:extension base="xs:precisionDecimal"><xs:attribute name="value" type="xs:precisionDecimal"/></xs:extension></xs:simpleContent></xs:complexType>
  <xs:element name="direct"><xs:complexType><xs:sequence/><xs:attribute name="value" type="xs:precisionDecimal" use="required"/></xs:complexType></xs:element>
  <xs:element name="named" type="Named"/>
  <xs:element name="inline"><xs:complexType><xs:sequence/><xs:attribute name="value" use="required"><xs:simpleType><xs:restriction base="xs:precisionDecimal"/></xs:simpleType></xs:attribute></xs:complexType></xs:element>
  <xs:element name="leaf"><xs:complexType><xs:sequence/><xs:attribute name="value" type="xs:precisionDecimal" use="required"/></xs:complexType></xs:element>
  <xs:element name="refRoot"><xs:complexType><xs:sequence><xs:element ref="leaf" minOccurs="0" maxOccurs="2"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="text" type="Text"/>
</xs:schema>`

//nolint:gocognit // Assert policy, shape, and immutable public facts together.
func TestPrecisionDecimalLocalAttributeShapesAndSimpleContent(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, attributePrecisionSchema, nil, policy)
			before := schema.Components()
			for _, input := range []string{
				`<direct value="  +1.20e2  "/>`,
				`<named value="+INF"/>`,
				`<inline value="NaN"/>`,
				`<refRoot><leaf value="-0"/><leaf value="INF"/></refRoot>`,
				`<text value="NaN"> -0.0 </text>`,
				`<text>+INF</text>`,
			} {
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Errorf("ValidateInstance(%s): %v", input, err)
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("validation mutated schema facts")
			}
		})
	}
	root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(attributePrecisionSchema)))
	if err != nil {
		t.Fatal(err)
	}
	strict10, err := goxsd9.ParseSchemaWithPolicy(root, validationTestResolver{}, goxsd9.Strict10)
	if err == nil || len(strict10.Components()) != 0 {
		t.Fatal("Strict10 accepted precisionDecimal or returned schema")
	}
	diagnostic := validationTestDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Loc().IsZero() || diagnostic.Code() == "" || !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("Strict10 diagnostic = %v", err)
	}
}

func TestPrecisionDecimalLocalAttributeStructurePrecedesValues(t *testing.T) {
	schema := validationTestSchemaWithPolicy(t, attributePrecisionSchema, nil, goxsd9.Strict11)
	direct := precisionAttributeTestElement(t, schema, "direct")
	leaf := precisionAttributeTestElement(t, schema, "leaf")
	refRoot := precisionAttributeTestElement(t, schema, "refRoot")
	refRootType, _ := refRoot.InlineComplexType()
	textElement := precisionAttributeTestElement(t, schema, "text")
	directType, _ := direct.InlineComplexType()
	leafType, _ := leaf.InlineComplexType()
	cases := []struct {
		input, code, primary string
		related              goxsd9.Loc
	}{
		{`<direct/>`, goxsd9.InvalidInstanceAttributeCode, `<direct`, directType.AttributeUses()[0].Loc()},
		{`<direct value="1e+" extra="x"/>`, goxsd9.InvalidInstanceAttributeCode, `extra=`, direct.Loc()},
		{`<direct value="1e+"><child/></direct>`, goxsd9.InvalidInstanceAttributeCode, `<child`, direct.Loc()},
		{`<direct value="1e+">text</direct>`, goxsd9.InvalidInstanceAttributeCode, `text</`, direct.Loc()},
		{`<refRoot><leaf/></refRoot>`, goxsd9.InvalidInstanceAttributeCode, `<leaf`, leafType.AttributeUses()[0].Loc()},
		{`<refRoot><leaf value="1e+"/><leaf value="2"/><leaf value="3"/></refRoot>`, goxsd9.InvalidInstanceSequenceCode, `<leaf value="3"`, refRootType.Loc()},
		{`<text value="1e+"><child/></text>`, goxsd9.InvalidInstanceAttributeCode, `<child`, textElement.Loc()},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(tc.input)))
			d := validationTestDiagnostic(t, err)
			wantLoc := precisionAttributeInstanceLoc(t, tc.input, tc.primary)
			if d.Class() != goxsd9.FailureInvalid || d.Code() != tc.code || d.Loc() != wantLoc || d.SpecRef() == "" || !validationTestHasRelated(d.Related(), tc.related) {
				t.Fatalf("diagnostic = %s related=%v", d, d.Related())
			}
		})
	}
}

func TestPrecisionDecimalLocalAttributeValueDiagnostics(t *testing.T) {
	schema := validationTestSchemaWithPolicy(t, attributePrecisionSchema, nil, goxsd9.Strict11)
	direct := precisionAttributeTestElement(t, schema, "direct")
	named := precisionAttributeTestElement(t, schema, "named")
	inline := precisionAttributeTestElement(t, schema, "inline")
	textElement := precisionAttributeTestElement(t, schema, "text")
	directType, _ := direct.InlineComplexType()
	inlineType, _ := inline.InlineComplexType()
	namedID, _ := named.TypeID()
	namedComponent, _ := schema.Lookup(namedID)
	namedType, _ := namedComponent.ComplexTypeDefinition()
	for _, tc := range []struct {
		input, code, spec, primary string
		related                    goxsd9.Loc
	}{
		{`<direct value="1e+"/>`, "XSD2010", "xsd-precisionDecimal#f-precDecLexmap", `value=`, directType.AttributeUses()[0].Loc()},
		{`<named value="-0"/>`, "XSD2020", "xsd11-datatypes#cvc-minExclusive-valid", `value=`, namedType.AttributeUses()[0].Loc()},
		{`<inline value="1e+"/>`, "XSD2010", "xsd-precisionDecimal#f-precDecLexmap", `value=`, inlineType.AttributeUses()[0].Loc()},
		{`<text value="1e+">0</text>`, "XSD2010", "xsd-precisionDecimal#f-precDecLexmap", `value=`, textElement.Loc()},
		{`<text value="0">1e+</text>`, "XSD2010", "xsd-precisionDecimal#f-precDecLexmap", `1e+</`, textElement.Loc()},
	} {
		t.Run(tc.input, func(t *testing.T) {
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(tc.input)))
			d := validationTestDiagnostic(t, err)
			wantLoc := precisionAttributeInstanceLoc(t, tc.input, tc.primary)
			if d.Class() != goxsd9.FailureInvalid || d.Code() != tc.code || d.SpecRef() != tc.spec || d.Loc() != wantLoc || !validationTestHasRelated(d.Related(), tc.related) {
				t.Fatalf("diagnostic = %s spec=%q related=%v", d, d.SpecRef(), d.Related())
			}
		})
	}
}

func precisionAttributeTestElement(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ElementDeclaration {
	t.Helper()
	for _, component := range schema.Components() {
		if component.Name().Local() != local || component.Kind() != goxsd9.ComponentKindElementDeclaration {
			continue
		}
		declaration, ok := component.ElementDeclaration()
		if ok {
			return declaration
		}
	}
	t.Fatalf("element %q not found", local)
	return goxsd9.ElementDeclaration{}
}

func precisionAttributeInstanceLoc(t *testing.T, input, snippet string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(input, snippet)
	if index < 0 {
		t.Fatalf("snippet %q absent from %q", snippet, input)
	}
	return validationTestLoc(t, "instance.xml", 1, index+1)
}

func TestPrecisionDecimalLocalAttributeMalformedXML(t *testing.T) {
	schema := validationTestSchemaWithPolicy(t, attributePrecisionSchema, nil, goxsd9.Strict11)
	input := `<direct value=1/>`
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	d := validationTestDiagnostic(t, err)
	if d.Class() != goxsd9.FailureInvalid || d.Code() != goxsd9.InvalidInstanceXMLCode || d.Loc() != validationTestLoc(t, "instance.xml", 1, 1) || d.Unwrap() == nil || !strings.Contains(d.Unwrap().Error(), "unquoted") {
		t.Fatalf("malformed attribute diagnostic = %s, cause %v", d, d.Unwrap())
	}
}

//nolint:gocognit // Assert the published nil particle and both consumer gates together.
func TestPrecisionDecimalGroupedExtensionZeroGroupRetainsConsumerGate(t *testing.T) {
	const source = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:t="urn:group" targetNamespace="urn:group" version="1.1">
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="value" type="xs:precisionDecimal" use="required"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:group name="Fields"><xs:sequence/></xs:group>
  <xs:complexType name="Base"/>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, source, nil, policy)
			root := precisionAttributeTestElement(t, schema, "root")
			id, hasID := root.TypeID()
			if !hasID {
				t.Fatal("derived type ID missing")
			}
			component, ok := schema.Lookup(id)
			if !ok {
				t.Fatal("derived type component missing")
			}
			definition, ok := component.ComplexTypeDefinition()
			if !ok || definition.Particle() != nil || len(definition.AttributeUses()) != 1 {
				t.Fatalf("grouped 0/0 facts = particle %T, uses %d", definition.Particle(), len(definition.AttributeUses()))
			}
			input := `<root xmlns="urn:group" value="1"/>`
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			d := validationTestDiagnostic(t, err)
			if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != definition.AttributeUses()[0].Loc() || d.SpecRef() != "xsd11-structures#cvc-elt" || !validationTestHasRelated(d.Related(), definition.Loc()) || d.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("ValidateInstance grouped extension = %s related=%v", d, d.Related())
			}
			output, err := goxsd9.GenerateGo(schema, "excluded")
			generated := validationTestDiagnostic(t, err)
			if output != nil || generated.Class() != goxsd9.FailureUnsupported || generated.Code() != "GOXSD9029" || generated.Loc() != definition.AttributeUses()[0].Loc() || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("GenerateGo grouped extension = %d bytes, %s", len(output), generated)
			}
		})
	}
}

//nolint:gocognit // Check all target-fact exits at the same reference boundary.
func TestPrecisionDecimalSequenceReferenceRejectsTargetElementFacts(t *testing.T) {
	cases := []struct{ name, attributes, constraint, spec, cause string }{
		{"abstract", ` abstract="true"`, "", "xsd11-structures#cvc-elt", "global element abstract and nillable facts"},
		{"nillable", ` nillable="true"`, "", "xsd11-structures#cvc-elt", "global element abstract and nillable facts"},
		{"identity", "", `<xs:unique name="Unique"><xs:selector xpath="."/><xs:field xpath="@value"/></xs:unique>`, "xsd11-structures#Identity-constraint_Definition_details", "identity constraints"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1"><xs:element name="doc"><xs:complexType><xs:sequence><xs:element ref="e" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element><xs:complexType name="E"><xs:sequence/><xs:attribute name="value" type="xs:precisionDecimal" use="required"/></xs:complexType><xs:element name="e" type="E"` + tc.attributes + `>` + tc.constraint + `</xs:element></xs:schema>`
			schema := validationTestSchemaWithPolicy(t, source, nil, goxsd9.Strict11)
			target := precisionAttributeTestElement(t, schema, "e")
			input := `<doc><e value="1"/></doc>`
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			d := validationTestDiagnostic(t, err)
			wantLoc := precisionAttributeInstanceLoc(t, input, `<e `)
			if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != wantLoc || d.SpecRef() != tc.spec || !validationTestHasRelated(d.Related(), target.Loc()) || d.Unwrap() == nil || !strings.Contains(d.Unwrap().Error(), tc.cause) || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("ValidateInstance target facts = %s related=%v", d, d.Related())
			}
			if tc.name == "identity" {
				constraints := target.IdentityConstraints()
				if len(constraints) != 1 || !validationTestHasRelated(d.Related(), constraints[0].Loc()) {
					t.Fatalf("identity related = %v", d.Related())
				}
			}
			output, err := goxsd9.GenerateGo(schema, "excluded")
			generated := validationTestDiagnostic(t, err)
			if output != nil || generated.Class() != goxsd9.FailureUnsupported || generated.Code() != "GOXSD9029" || generated.Loc().IsZero() {
				t.Fatalf("GenerateGo target facts = %d bytes, %s", len(output), generated)
			}
		})
	}
}
