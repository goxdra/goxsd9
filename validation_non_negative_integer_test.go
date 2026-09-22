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
	validationNonNegativeIntegerNamespace      = "urn:non-negative-integer"
	validationNonNegativeIntegerOtherNamespace = "urn:non-negative-integer-other"
)

type validationNonNegativeIntegerPolicy struct {
	name    string
	policy  goxsd9.LanguagePolicy
	version goxsd9.XSDVersion
}

func validationNonNegativeIntegerPolicies() []validationNonNegativeIntegerPolicy {
	return []validationNonNegativeIntegerPolicy{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: goxsd9.XSDVersion11},
		{name: "Strict10", policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
		{name: "Strict11", policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
	}
}

func TestValidateInstanceSupportsGlobalNonNegativeIntegerScalarsAcrossPolicies(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := validationNonNegativeIntegerSchema(t, profile)
			before := schema.Components()
			for _, test := range []struct {
				name    string
				element string
				value   string
			}{
				{name: "direct zero", element: "direct", value: "0"},
				{name: "direct signed zero", element: "direct", value: "-00"},
				{name: "direct leading sign and zeros", element: "direct", value: "\t +000123 \r\n"},
				{name: "direct arbitrary precision", element: "direct", value: validationNonNegativeIntegerHugeValue},
				{name: "forward named", element: "forward", value: "+0007"},
				{name: "inherited named", element: "inherited", value: "42"},
				{name: "included named", element: "included", value: "0008"},
				{name: "imported named", element: "imported", value: "9"},
				{name: "chameleon named", element: "chameleon", value: "10"},
				{name: "bounded exact digits", element: "bounded", value: "+00123"},
				{name: "enumerated signed zero", element: "enumerated", value: "-0"},
				{name: "enumerated value equality", element: "enumerated", value: "+0007"},
				{name: "ordinary integer remains distinct", element: "ordinary", value: "42"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationNonNegativeIntegerInstance(test.element, test.value)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("nonNegativeInteger validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit,funlen // Keep lexical, bound, facet, enumeration, and provenance checks together.
func TestValidateInstanceReportsNonNegativeIntegerDiagnosticsAcrossPolicies(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := validationNonNegativeIntegerSchema(t, profile)
			cases := []struct {
				name       string
				element    string
				value      string
				code       string
				specRef    string
				selfClosed bool
				related    func(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc
			}{
				{
					name:       "direct empty lexical",
					element:    "direct",
					code:       goxsd9.InvalidIntegerLexicalCode,
					specRef:    validationNonNegativeIntegerDatatypeSpecRef(profile.version),
					selfClosed: true,
					related:    validationNonNegativeIntegerElementRelated(),
				},
				{
					name:    "direct bare sign",
					element: "direct",
					value:   "+",
					code:    goxsd9.InvalidIntegerLexicalCode,
					specRef: validationNonNegativeIntegerDatatypeSpecRef(profile.version),
					related: validationNonNegativeIntegerElementRelated(),
				},
				{
					name:    "direct decimal",
					element: "direct",
					value:   "1.0",
					code:    goxsd9.InvalidIntegerLexicalCode,
					specRef: validationNonNegativeIntegerDatatypeSpecRef(profile.version),
					related: validationNonNegativeIntegerElementRelated(),
				},
				{
					name:    "direct exponent",
					element: "direct",
					value:   "1e2",
					code:    goxsd9.InvalidIntegerLexicalCode,
					specRef: validationNonNegativeIntegerDatatypeSpecRef(profile.version),
					related: validationNonNegativeIntegerElementRelated(),
				},
				{
					name:    "direct non ASCII digit",
					element: "direct",
					value:   "١",
					code:    goxsd9.InvalidIntegerLexicalCode,
					specRef: validationNonNegativeIntegerDatatypeSpecRef(profile.version),
					related: validationNonNegativeIntegerElementRelated(),
				},
				{
					name:    "direct negative bound",
					element: "direct",
					value:   "-1",
					code:    goxsd9.BoundValueViolationCode,
					specRef: validationNonNegativeIntegerBoundSpecRef(profile.version, "minInclusive"),
					related: validationNonNegativeIntegerElementRelated(),
				},
				{
					name:    "named enumeration",
					element: "enumerated",
					value:   "8",
					code:    goxsd9.EnumerationValueViolationCode,
					specRef: validationNonNegativeIntegerEnumerationSpecRef(profile.version),
					related: validationNonNegativeIntegerEnumerationRelated("enumerated"),
				},
				{
					name:    "named total digits",
					element: "bounded",
					value:   "1234",
					code:    goxsd9.DigitFacetValueViolationCode,
					specRef: validationNonNegativeIntegerDigitSpecRef(profile.version),
					related: validationNonNegativeIntegerBoundedRelated("bounded"),
				},
				{
					name:    "ordinary integer keeps integer specification",
					element: "ordinary",
					value:   "1.0",
					code:    goxsd9.InvalidIntegerLexicalCode,
					specRef: validationNonNegativeIntegerIntegerSpecRef(profile.version),
					related: validationNonNegativeIntegerTypeRelated("ordinary", "Ordinary"),
				},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					input := validationNonNegativeIntegerInstance(test.element, test.value)
					if test.selfClosed {
						input = validationNonNegativeIntegerSelfClosingInstance(test.element)
					}
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), test.code)
					}
					wantLoc := validationTestTextLoc(t, input)
					if test.selfClosed {
						wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
					}
					if diagnostic.Loc() != wantLoc {
						t.Fatalf("Loc() = %s, want %s", diagnostic.Loc(), wantLoc)
					}
					if diagnostic.SpecRef() != test.specRef {
						t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), test.specRef)
					}
					if !reflect.DeepEqual(diagnostic.Related(), test.related(t, schema)) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), test.related(t, schema))
					}
					if test.code != goxsd9.InvalidIntegerLexicalCode && diagnostic.Unwrap() == nil {
						t.Fatal("diagnostic lost its datatype cause")
					}
					first := diagnostic
					second := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if first.Error() != second.Error() || first.Code() != second.Code() || first.Loc() != second.Loc() || first.SpecRef() != second.SpecRef() || !reflect.DeepEqual(first.Related(), second.Related()) {
						t.Fatalf("repeated diagnostic differs: first %v, second %v", err, second)
					}
				})
			}
		})
	}
}

func TestValidateInstanceKeepsNonNegativeIntegerBoundariesUnsupported(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := validationNonNegativeIntegerSchema(t, profile)
			for _, test := range []struct {
				name  string
				input string
			}{
				{name: "attribute", input: `<direct xmlns="` + validationNonNegativeIntegerNamespace + `" flag="x">0</direct>`},
				{name: "child", input: `<direct xmlns="` + validationNonNegativeIntegerNamespace + `">0<child/></direct>`},
			} {
				t.Run(test.name, func(t *testing.T) {
					assertNonNegativeIntegerBoundaryUnsupported(t, schema, test.input, profile.version)
				})
			}
		})
	}
}

func assertNonNegativeIntegerBoundaryUnsupported(t *testing.T, schema goxsd9.Schema, input string, version goxsd9.XSDVersion) {
	t.Helper()
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureUnsupported {
		t.Fatalf("diagnostic = %s, want unsupported instance validation", diagnostic)
	}
	if diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), goxsd9.UnsupportedInstanceValidationCode)
	}
	if !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic = %s, want ErrUnsupported", diagnostic)
	}
	wantSpecRef := validationNonNegativeIntegerStructureSpecRef(version)
	if diagnostic.Loc().IsZero() || diagnostic.SpecRef() != wantSpecRef {
		t.Fatalf("diagnostic evidence = %s/%q, want located %q", diagnostic.Loc(), diagnostic.SpecRef(), wantSpecRef)
	}
}

func TestValidateInstanceKeepsNonNegativeIntegerReferenceTargetsUnsupported(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			referenceRoot := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNonNegativeIntegerNamespace + `" targetNamespace="` + validationNonNegativeIntegerNamespace + `" version="` + string(profile.version) + `">
  <xs:element name="root" type="r:Choice"/>
  <xs:element name="target" type="xs:nonNegativeInteger"/>
  <xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>
</xs:schema>`
			schema := validationTestSchemaWithPolicy(t, referenceRoot, nil, profile.policy)
			input := `<root xmlns="` + validationNonNegativeIntegerNamespace + `"><target>0</target></root>`
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
				t.Fatalf("reference diagnostic = %s, want unsupported instance validation", diagnostic)
			}
		})
	}
}

func TestValidateInstancePreservesNonNegativeIntegerReaderLifecycle(t *testing.T) {
	profile := validationNonNegativeIntegerPolicies()[0]
	schema := validationNonNegativeIntegerSchema(t, profile)
	input := validationNonNegativeIntegerInstance("direct", "0")
	readErr := errors.New("nonNegativeInteger instance read failed")
	closeErr := errors.New("nonNegativeInteger instance close failed")
	reader := newValidationTestSource(input)
	reader.failAt = len(reader.data)
	reader.readErr = readErr
	reader.closeErr = closeErr
	err := goxsd9.ValidateInstance(schema, "instance.xml", reader)
	if err == nil || !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
		t.Fatalf("read/close error = %v, want both causes", err)
	}
	diagnostics := validationTestDiagnostics(t, err)
	if len(diagnostics) != 2 || diagnostics[0].Code() != goxsd9.SourceReadCode || diagnostics[1].Code() != goxsd9.SourceCloseCode {
		t.Fatalf("read/close diagnostics = %#v, want read then close", diagnostics)
	}
	if !reader.closed || reader.closeCalls != 1 || reader.offset != len(reader.data) {
		t.Fatalf("reader lifecycle = closed %t, close calls %d, offset %d, want closed once and drained", reader.closed, reader.closeCalls, reader.offset)
	}
}

const validationNonNegativeIntegerHugeValue = "123456789012345678901234567890123456789012345678901234567890"

func validationNonNegativeIntegerSchema(t *testing.T, profile validationNonNegativeIntegerPolicy) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNonNegativeIntegerNamespace + `" xmlns:o="` + validationNonNegativeIntegerOtherNamespace + `" targetNamespace="` + validationNonNegativeIntegerNamespace + `" version="` + string(profile.version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="` + validationNonNegativeIntegerOtherNamespace + `" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:nonNegativeInteger"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="inherited" type="r:Derived"/>
  <xs:element name="bounded" type="r:Bounded"/>
  <xs:element name="enumerated" type="r:Enumerated"/>
  <xs:element name="ordinary" type="r:Ordinary"/>
  <xs:simpleType name="Forward"><xs:restriction base="r:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:simpleType name="Derived"><xs:restriction base="r:Forward"/></xs:simpleType>
  <xs:simpleType name="Bounded"><xs:restriction base="xs:nonNegativeInteger"><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:nonNegativeInteger"><xs:enumeration value="-0"/><xs:enumeration value="0007"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Ordinary"><xs:restriction base="xs:integer"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]validationTestFixture{
		"included.xsd": {
			id: "included.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNonNegativeIntegerNamespace + `" targetNamespace="` + validationNonNegativeIntegerNamespace + `">
  <xs:simpleType name="Included"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:element name="included" type="r:Included"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id: "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNonNegativeIntegerNamespace + `">
  <xs:simpleType name="Chameleon"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:element name="chameleon" type="r:Chameleon"/>
</xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationNonNegativeIntegerOtherNamespace + `" targetNamespace="` + validationNonNegativeIntegerOtherNamespace + `">
  <xs:simpleType name="Imported"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:element name="imported" type="o:Imported"/>
</xs:schema>`,
		},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, profile.policy)
}

func validationNonNegativeIntegerInstance(element, value string) string {
	if element == "imported" {
		return `<o:imported xmlns:o="` + validationNonNegativeIntegerOtherNamespace + `">` + value + `</o:imported>`
	}
	return `<` + element + ` xmlns="` + validationNonNegativeIntegerNamespace + `">` + value + `</` + element + `>`
}

func validationNonNegativeIntegerSelfClosingInstance(element string) string {
	return `<` + element + ` xmlns="` + validationNonNegativeIntegerNamespace + `"/>`
}

func validationNonNegativeIntegerElementRelated() func(*testing.T, goxsd9.Schema) []goxsd9.Loc {
	return func(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
		t.Helper()
		return []goxsd9.Loc{validationNonNegativeIntegerElement(t, schema, "direct").Loc()}
	}
}

func validationNonNegativeIntegerEnumerationRelated(element string) func(*testing.T, goxsd9.Schema) []goxsd9.Loc {
	return func(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
		t.Helper()
		declaration := validationNonNegativeIntegerElement(t, schema, element)
		typeComponent := validationNonNegativeIntegerType(t, schema, "Enumerated")
		definition, ok := typeComponent.SimpleTypeDefinition()
		if !ok {
			t.Fatal("Enumerated simple type view is missing")
		}
		return append([]goxsd9.Loc{declaration.Loc(), typeComponent.Loc()}, definition.IntegerEnumerationFacets().Locations()...)
	}
}

func validationNonNegativeIntegerTypeRelated(element, typeName string) func(*testing.T, goxsd9.Schema) []goxsd9.Loc {
	return func(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
		t.Helper()
		return []goxsd9.Loc{
			validationNonNegativeIntegerElement(t, schema, element).Loc(),
			validationNonNegativeIntegerType(t, schema, typeName).Loc(),
		}
	}
}

func validationNonNegativeIntegerBoundedRelated(element string) func(*testing.T, goxsd9.Schema) []goxsd9.Loc {
	return func(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
		t.Helper()
		declaration := validationNonNegativeIntegerElement(t, schema, element)
		typeComponent := validationNonNegativeIntegerType(t, schema, "Bounded")
		definition, ok := typeComponent.SimpleTypeDefinition()
		if !ok {
			t.Fatal("Bounded simple type view is missing")
		}
		facets, ok := definition.DigitFacets().TotalDigitsLoc()
		if !ok {
			t.Fatal("Bounded totalDigits location is missing")
		}
		return []goxsd9.Loc{declaration.Loc(), typeComponent.Loc(), facets}
	}
}

func validationNonNegativeIntegerElement(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName(validationNonNegativeIntegerNamespace, local)
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

func validationNonNegativeIntegerType(t *testing.T, schema goxsd9.Schema, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName(validationNonNegativeIntegerNamespace, local)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s simple types = %d, want one", local, len(components))
	}
	return components[0]
}

func validationNonNegativeIntegerDatatypeSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#nonNegativeInteger"
	}
	return "xsd11-datatypes#nonNegativeInteger"
}

func validationNonNegativeIntegerIntegerSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#integer"
	}
	return "xsd11-datatypes#integer"
}

func validationNonNegativeIntegerStructureSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-structures#cvc-elt"
	}
	return "xsd11-structures#cvc-elt"
}

func validationNonNegativeIntegerBoundSpecRef(version goxsd9.XSDVersion, kind string) string {
	prefix := "xsd11"
	if version == goxsd9.XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-" + kind + "-valid"
}

func validationNonNegativeIntegerEnumerationSpecRef(version goxsd9.XSDVersion) string {
	prefix := "xsd11"
	if version == goxsd9.XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-enumeration-valid"
}

func validationNonNegativeIntegerDigitSpecRef(version goxsd9.XSDVersion) string {
	prefix := "xsd11"
	if version == goxsd9.XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-totalDigits-valid"
}
