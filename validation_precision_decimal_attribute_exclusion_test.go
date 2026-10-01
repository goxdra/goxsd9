package goxsd9_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

// Each non-precision local use remains outside the newly admitted validator path.
func TestPrecisionDecimalAttributePacketExcludesOtherScalarUsesByShapeAndConsumer(t *testing.T) {
	const prefix = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1">`
	const suffix = `</xs:schema>`
	cases := []struct{ name, schema, instance string }{
		{"direct", prefix + `<xs:element name="root"><xs:complexType><xs:sequence/><xs:attribute name="value" type="xs:boolean" use="required"/></xs:complexType></xs:element>` + suffix, `<root value="true"/>`},
		{"named", prefix + `<xs:complexType name="T"><xs:sequence/><xs:attribute name="value" type="xs:boolean" use="required"/></xs:complexType><xs:element name="root" type="T"/>` + suffix, `<root value="true"/>`},
		{"inline", prefix + `<xs:element name="root"><xs:complexType><xs:sequence/><xs:attribute name="value" use="required"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute></xs:complexType></xs:element>` + suffix, `<root value="true"/>`},
		{"ref", prefix + `<xs:element name="root"><xs:complexType><xs:sequence><xs:element ref="leaf"/></xs:sequence></xs:complexType></xs:element><xs:element name="leaf"><xs:complexType><xs:sequence/><xs:attribute name="value" type="xs:boolean" use="required"/></xs:complexType></xs:element>` + suffix, `<root><leaf value="true"/></root>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, tc.schema, nil, goxsd9.Strict11)
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(tc.instance)))
			d := validationTestDiagnostic(t, err)
			wantValidationLoc := validationTestLoc(t, "instance.xml", 1, 1)
			if tc.name == "named" {
				wantValidationLoc = precisionAttributeSchemaLoc(t, tc.schema, `<xs:attribute name="value"`)
			}
			if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != wantValidationLoc || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("ValidateInstance diagnostic = %s related=%v", d, d.Related())
			}
			output, err := goxsd9.GenerateGo(schema, "excluded")
			generated := validationTestDiagnostic(t, err)
			wantGenerationLoc := precisionAttributeSchemaLoc(t, tc.schema, `<xs:element name="root"`)
			if tc.name == "named" {
				wantGenerationLoc = wantValidationLoc
			}
			if output != nil || generated.Class() != goxsd9.FailureUnsupported || generated.Code() != "GOXSD9029" || generated.Loc() != wantGenerationLoc || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("GenerateGo = %d bytes, %s", len(output), generated)
			}
		})
	}
}

func precisionAttributeSchemaLoc(t *testing.T, schema, snippet string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(schema, snippet)
	if index < 0 {
		t.Fatalf("schema snippet %q absent", snippet)
	}
	return validationTestLoc(t, "root.xsd", 1, index+1)
}

// Local references to global attributes stay outside the local-use value path.
func TestPrecisionDecimalAttributePacketExcludesGlobalAttributeReferences(t *testing.T) {
	const prefix = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1"><xs:attribute name="g" type="xs:precisionDecimal"/>`
	const use = `<xs:attribute ref="g" use="required"/>`
	const suffix = `</xs:schema>`
	cases := []struct{ name, schema, instance string }{
		{"direct", prefix + `<xs:element name="root"><xs:complexType><xs:sequence/>` + use + `</xs:complexType></xs:element>` + suffix, `<root g="1"/>`},
		{"named", prefix + `<xs:complexType name="T"><xs:sequence/>` + use + `</xs:complexType><xs:element name="root" type="T"/>` + suffix, `<root g="1"/>`},
		{"ref", prefix + `<xs:element name="root"><xs:complexType><xs:sequence><xs:element ref="leaf"/></xs:sequence></xs:complexType></xs:element><xs:element name="leaf"><xs:complexType><xs:sequence/>` + use + `</xs:complexType></xs:element>` + suffix, `<root><leaf g="1"/></root>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, tc.schema, nil, goxsd9.Strict11)
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(tc.instance)))
			d := validationTestDiagnostic(t, err)
			wantValidationLoc := validationTestLoc(t, "instance.xml", 1, 1)
			if tc.name == "named" {
				wantValidationLoc = precisionAttributeSchemaLoc(t, tc.schema, use)
			}
			if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != wantValidationLoc || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("ValidateInstance = %s related=%v", d, d.Related())
			}
			output, err := goxsd9.GenerateGo(schema, "excluded")
			generated := validationTestDiagnostic(t, err)
			wantGenerationLoc := precisionAttributeSchemaLoc(t, tc.schema, `<xs:attribute name="g"`)
			if tc.name == "named" {
				wantGenerationLoc = wantValidationLoc
			}
			if output != nil || generated.Class() != goxsd9.FailureUnsupported || generated.Code() != "GOXSD9029" || generated.Loc() != wantGenerationLoc || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("GenerateGo = %d bytes, %s", len(output), generated)
			}
		})
	}
}

// List, union, and assertion facts are rejected before an attribute consumer
// can be constructed. No schema can be passed to validation or generation.
//
//nolint:gocognit // Every excluded variety is checked across all admitted owner shapes.
func TestPrecisionDecimalAttributePacketExcludesMemberValuesAndAssertionsAtAdmission(t *testing.T) {
	for _, kind := range []string{"list", "union", "assertion"} {
		for _, shape := range []string{"direct", "named", "inline", "ref"} {
			t.Run(kind+"/"+shape, func(t *testing.T) {
				source := precisionAttributeExcludedSchema(kind, shape)
				root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(source)))
				if err != nil {
					t.Fatal(err)
				}
				schema, err := goxsd9.ParseSchemaWithPolicy(root, validationTestResolver{}, goxsd9.Strict11)
				d := validationTestDiagnostic(t, err)
				code := "XSD3003"
				primary := `type="T"`
				if shape == "inline" {
					primary = `<xs:simpleType>`
				}
				if kind == "assertion" {
					code = "XSD2019"
					primary = `<xs:assertion`
				}
				wantLoc := precisionAttributeSchemaLoc(t, source, primary)
				if len(schema.Components()) != 0 || d.Class() != goxsd9.FailureUnsupported || d.Code() != code || d.Loc() != wantLoc || !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("ParseSchema returned %d components, %s", len(schema.Components()), d)
				}
			})
		}
	}
}

func precisionAttributeExcludedSchema(kind, shape string) string {
	const prefix = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1">`
	const suffix = `</xs:schema>`
	var typeDefinition, inlineType string
	switch kind {
	case "list":
		typeDefinition = `<xs:simpleType name="T"><xs:list itemType="xs:precisionDecimal"/></xs:simpleType>`
		inlineType = `<xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType>`
	case "union":
		typeDefinition = `<xs:simpleType name="T"><xs:union memberTypes="xs:precisionDecimal"/></xs:simpleType>`
		inlineType = `<xs:simpleType><xs:union memberTypes="xs:precisionDecimal"/></xs:simpleType>`
	case "assertion":
		typeDefinition = `<xs:simpleType name="T"><xs:restriction base="xs:precisionDecimal"><xs:assertion test="true()"/></xs:restriction></xs:simpleType>`
		inlineType = `<xs:simpleType><xs:restriction base="xs:precisionDecimal"><xs:assertion test="true()"/></xs:restriction></xs:simpleType>`
	}
	attribute := `<xs:attribute name="value" type="T" use="required"/>`
	if shape == "inline" {
		attribute = `<xs:attribute name="value" use="required">` + inlineType + `</xs:attribute>`
		typeDefinition = ""
	}
	body := `<xs:complexType><xs:sequence/>` + attribute + `</xs:complexType>`
	switch shape {
	case "direct", "inline":
		return prefix + typeDefinition + `<xs:element name="root">` + body + `</xs:element>` + suffix
	case "named":
		return prefix + typeDefinition + `<xs:complexType name="C"><xs:sequence/>` + attribute + `</xs:complexType><xs:element name="root" type="C"/>` + suffix
	case "ref":
		return prefix + typeDefinition + `<xs:element name="root"><xs:complexType><xs:sequence><xs:element ref="leaf"/></xs:sequence></xs:complexType></xs:element><xs:element name="leaf">` + body + `</xs:element>` + suffix
	}
	return ""
}
