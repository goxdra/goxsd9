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

const precisionVarietySchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1">
<xs:simpleType name="Scaled"><xs:restriction base="xs:precisionDecimal"><xs:minScale value="-2"/><xs:maxScale value="-1"/></xs:restriction></xs:simpleType>
<xs:simpleType name="Numbers"><xs:list itemType="Scaled"/></xs:simpleType>
<xs:simpleType name="Choice"><xs:union memberTypes="Scaled xs:negativeInteger"/></xs:simpleType>
<xs:element name="namedList" type="Numbers"/>
<xs:element name="inlineList"><xs:simpleType><xs:list itemType="Scaled"/></xs:simpleType></xs:element>
<xs:element name="namedUnion" type="Choice"/>
<xs:element name="inlineUnion"><xs:simpleType><xs:union memberTypes="Scaled xs:negativeInteger"/></xs:simpleType></xs:element>
<xs:element name="sequence"><xs:complexType><xs:sequence><xs:element name="one" type="Choice"/><xs:element ref="namedList"/></xs:sequence></xs:complexType></xs:element>
</xs:schema>`

func TestPrecisionDecimalListUnionStrict10PolicyHasNoSchema(t *testing.T) {
	root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(precisionVarietySchema)))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := goxsd9.ParseSchemaWithPolicy(root, validationTestResolver{}, goxsd9.Strict10)
	diagnostic := validationTestDiagnostic(t, err)
	line := strings.Split(precisionVarietySchema, "\n")[1]
	wantLoc := validationTestLoc(t, "root.xsd", 2, strings.Index(line, `<xs:restriction`)+1)
	if len(schema.Components()) != 0 || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != "XSD3030" || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != "xsd11-datatypes#dt-primitive" || !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("Strict10 returned %d components, %s", len(schema.Components()), diagnostic)
	}
}

//nolint:gocognit // Verify both policies, each shape, and independent copied facts.
func TestPrecisionDecimalListUnionPublicValueAndCopiedFacts(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, precisionVarietySchema, nil, policy)
			before := schema.Components()
			for _, test := range []struct{ name, value string }{
				{"namedList", ""},
				{"namedList", " \t 1e2  INF  -0e2 \n"},
				{"inlineList", "2e1 3e2"},
				{"namedUnion", "-1234"},
				{"inlineUnion", "INF"},
			} {
				input := "<" + test.name + ">" + test.value + "</" + test.name + ">"
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("%s %q: %v", test.name, test.value, err)
				}
			}
			input := `<sequence><one>-1234</one><namedList>2e1</namedList></sequence>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("sequence with local union and global list ref: %v", err)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("validation mutated the completed component model")
			}
			name, err := goxsd9.NewQName("", "Choice")
			if err != nil {
				t.Fatal(err)
			}
			components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
			if len(components) != 1 {
				t.Fatalf("Choice lookup = %d components", len(components))
			}
			definition, ok := components[0].SimpleTypeDefinition()
			if !ok || definition.Variety() != goxsd9.SimpleTypeVarietyUnion {
				t.Fatalf("Choice view = %#v", definition)
			}
			members := definition.MemberTypes()
			if len(members) != 2 || members[0].Name().Local() != "Scaled" || members[1].Name().Local() != "negativeInteger" {
				t.Fatalf("copied members = %#v", members)
			}
			members[0] = goxsd9.SimpleTypeReference{}
			again := definition.MemberTypes()
			if len(again) != 2 || again[0].Name().Local() != "Scaled" {
				t.Fatal("caller mutation changed member order or schema facts")
			}
		})
	}
}

//nolint:gocognit // Assert independent list, union, and ordered-content exits.
func TestPrecisionDecimalListUnionLocatedFailuresAndOrderedContent(t *testing.T) {
	schema := validationTestSchemaWithPolicy(t, precisionVarietySchema, nil, goxsd9.Strict11)
	for _, test := range []struct {
		name, input, code, spec string
		innerCode               string
		loc                     int
	}{
		{"list item facet", `<namedList>2e1 132</namedList>`, goxsd9.InvalidInstanceListCode, "xsd11-datatypes#list", "XSD2020", 12},
		{"list non-XML whitespace", "<namedList>1e2\u00a02e2</namedList>", goxsd9.InvalidInstanceListCode, "xsd11-datatypes#list", "XSD2010", 12},
		{"union members", `<namedUnion>+1234</namedUnion>`, goxsd9.InvalidInstanceUnionCode, "xsd11-datatypes#union", "XSD2020", 13},
		{"sequence order", `<sequence><namedList>132</namedList><one>-1234</one></sequence>`, goxsd9.InvalidInstanceSequenceCode, "xsd11-structures#cvc-particle", "", 1},
		{"sequence minimum", `<sequence><one>-1234</one></sequence>`, goxsd9.InvalidInstanceSequenceCode, "xsd11-structures#cvc-particle", "", 1},
		{"sequence maximum", `<sequence><one>-1234</one><namedList>2e1</namedList><namedList>2e1</namedList></sequence>`, goxsd9.InvalidInstanceSequenceCode, "xsd11-structures#cvc-particle", "", 53},
		{"sequence before value", `<sequence><one>+1234</one><namedList>132</namedList><extra/></sequence>`, goxsd9.InvalidInstanceSequenceCode, "xsd11-structures#cvc-particle", "", 53},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.spec || diagnostic.Loc() != validationTestLoc(t, "instance.xml", 1, test.loc) {
				t.Fatalf("diagnostic = %s related=%v, want invalid/%s/%s at column %d", diagnostic, diagnostic.Related(), test.code, test.spec, test.loc)
			}
			if len(diagnostic.Related()) == 0 || errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("diagnostic lost schema provenance or became unsupported: %v", err)
			}
			if test.innerCode != "" {
				var inner goxsd9.Diagnostic
				if !errors.As(diagnostic.Unwrap(), &inner) || inner.Code() != test.innerCode || inner.Loc() != diagnostic.Loc() {
					t.Fatalf("diagnostic lost located item/member cause: outer=%s inner=%s", diagnostic, inner)
				}
				if !validationTestHasRelated(diagnostic.Related(), validationTestLoc(t, "root.xsd", 2, 1)) {
					t.Fatalf("diagnostic has no Scaled type provenance: %v", diagnostic.Related())
				}
				return
			}
			if !validationTestHasRelated(diagnostic.Related(), validationTestLoc(t, "root.xsd", 9, 1)) {
				t.Fatalf("ordered failure has no sequence declaration provenance: %v", diagnostic.Related())
			}
		})
	}
}
