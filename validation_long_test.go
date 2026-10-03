package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const longValidationHuge = "999999999999999999999999999999999999999999999999999999999999999999999999"

func longValidationSchema(t *testing.T, profile longPolicyProfile) Schema {
	t.Helper()
	root := schemaLongReferenceRoot(profile.version)
	root = strings.Replace(root, "</xs:schema>", `<xs:element name="inline"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element><xs:element name="enum" type="t:Enumerated"/><xs:element name="digits" type="t:Digits"/><xs:element name="exclusive" type="t:Exclusive"/><xs:simpleType name="Digits"><xs:restriction base="xs:long"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Exclusive"><xs:restriction base="xs:long"><xs:minExclusive value="-5"/><xs:maxExclusive value="5"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err != nil {
		t.Fatalf("discover long validation schema: %v", err)
	}
	return schema
}

func longDatatypeSpec(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceLongXSD10SpecRef
	}
	return instanceLongXSD11SpecRef
}

func longFacetSpec(version XSDVersion, facet string) string {
	prefix := "xsd11"
	if version == XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-" + facet + "-valid"
}

//nolint:gocognit // One table covers the exact root value and each datatype exit under every policy.
func TestValidateLongGlobalScalarValuesAndDiagnostics(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema := longValidationSchema(t, profile)
			before := schema.Components()
			for _, test := range []struct{ local, value string }{
				{"direct", "-9223372036854775808"}, {"direct", "+009223372036854775807"}, {"direct", " \t-00\r\n"},
				{"direct", strings.Repeat("0", 80) + "9223372036854775807"}, {"direct", " \t+00042\n"},
				{"forward", "-100"}, {"narrowed", "2"}, {"named", "+00100"},
				{"enum", "-09223372036854775808"}, {"digits", "-99"}, {"exclusive", "-4"}, {"exclusive", "4"},
			} {
				input := boundedIntegerInstance(test.local, "urn:test", test.value)
				for range 2 {
					if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("valid %s=%q: %v", test.local, test.value, err)
					}
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("long validation mutated completed schema facts")
			}
			for _, test := range []boundedIntegerDiagnosticCase{
				{"direct", "-9223372036854775809", BoundValueViolationCode, longFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "9223372036854775808", BoundValueViolationCode, longFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", longValidationHuge, BoundValueViolationCode, longFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", "-" + longValidationHuge, BoundValueViolationCode, longFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "+", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1.0", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1e2", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "١", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1 2", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "\u00a01", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"forward", "1.0", InvalidIntegerLexicalCode, longDatatypeSpec(profile.version), Loc{}, false},
				{"forward", "101", BoundValueViolationCode, longFacetSpec(profile.version, "maxInclusive"), longFacetLoc(t, schema, "Later", "maxInclusive"), true},
				{"narrowed", "3", BoundValueViolationCode, longFacetSpec(profile.version, "maxInclusive"), longFacetLoc(t, schema, "Tight", "maxInclusive"), true},
				{"enum", "0", EnumerationValueViolationCode, longFacetSpec(profile.version, "enumeration"), requireLongDefinition(t, schema, "Enumerated").IntegerEnumerationFacets().Locations()[0], true},
				{"digits", "100", DigitFacetValueViolationCode, longFacetSpec(profile.version, "totalDigits"), longDigitLoc(t, schema), true},
				{"exclusive", "-5", BoundValueViolationCode, longFacetSpec(profile.version, "minExclusive"), longExclusiveFacetLoc(t, schema, false), true},
				{"exclusive", "5", BoundValueViolationCode, longFacetSpec(profile.version, "maxExclusive"), longExclusiveFacetLoc(t, schema, true), true},
			} {
				assertBoundedIntegerDiagnosticCase(t, schema, test)
			}
		})
	}
}

func longFacetLoc(t *testing.T, schema Schema, name, bound string) Loc {
	t.Helper()
	bounds, ok := requireLongDefinition(t, schema, name).IntegerBounds()
	if !ok {
		t.Fatal("long bound facts missing")
	}
	if bound == "maxInclusive" {
		facet, present := bounds.MaxInclusiveFacet()
		if !present {
			t.Fatal("long maximum missing")
		}
		return facet.Loc()
	}
	facet, present := bounds.MinInclusiveFacet()
	if !present {
		t.Fatal("long minimum missing")
	}
	return facet.Loc()
}

func longDigitLoc(t *testing.T, schema Schema) Loc {
	t.Helper()
	loc, ok := requireLongDefinition(t, schema, "Digits").DigitFacets().TotalDigitsLoc()
	if !ok {
		t.Fatal("long totalDigits location missing")
	}
	return loc
}

func longExclusiveFacetLoc(t *testing.T, schema Schema, maximum bool) Loc {
	t.Helper()
	bounds, ok := requireLongDefinition(t, schema, "Exclusive").IntegerBounds()
	if !ok {
		t.Fatal("exclusive long bounds missing")
	}
	if maximum {
		facet, present := bounds.MaxExclusiveFacet()
		if !present {
			t.Fatal("exclusive long maximum missing")
		}
		return facet.Loc()
	}
	facet, present := bounds.MinExclusiveFacet()
	if !present {
		t.Fatal("exclusive long minimum missing")
	}
	return facet.Loc()
}

func TestValidateLongComposedGraphRoots(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root, fixtures := longGraphFixtures(profile.version)
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatalf("%s graph: %v", profile.name, err)
		}
		for _, entry := range longGraphElementCases() {
			t.Run(profile.name+"/"+entry.local, func(t *testing.T) {
				assertBoundedIntegerGraphRoot(t, schema, entry.namespace, entry.local, entry.named, "9223372036854775807", "9223372036854775808", profile.version)
			})
		}
	}
}

func TestValidateLongEmptyAndStructureExits(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		assertBoundedIntegerEmptyAndStructureExits(t, longValidationSchema(t, profile), profile.version, longDatatypeSpec(profile.version), "9223372036854775808")
	}
}

func TestValidateLongGlobalInlineRootRemainsUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := longValidationSchema(t, profile)
		input := boundedIntegerInstance("inline", "urn:test", "42")
		d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
		declaration := requireLongElement(t, schema, "inline", "urn:test")
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) || !errors.Is(d, errInstanceNoDeclaredType) || !errors.Is(d, ErrUnsupported) {
			t.Fatalf("global inline long diagnostic = %s, related %v", d, d.Related())
		}
	}
}

func TestValidateLongLocalAndReferenceConsumersRemainUnsupported(t *testing.T) {
	assertBoundedIntegerLocalConsumersRemainUnsupported(t, "long", "urn:long-local")
}

func TestValidateLongUsesCopiedExactBounds(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := longValidationSchema(t, profile)
		assertBoundedIntegerCopiedBounds(t, schema, "-9223372036854775808", "9223372036854775807")
	}
}

//nolint:gocognit // Check each global long representation against the generation gate.
func TestValidateLongGlobalGenerationRemainsUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, shape := range []struct {
			name, declaration, definition string
		}{
			{"direct", `<xs:element name="value" type="xs:long"/>`, ""},
			{"named", `<xs:element name="value" type="t:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType>`},
			{"inline", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element>`, ""},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + shape.declaration + shape.definition + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover %s long schema: %v", shape.name, err)
				}
				declaration := boundedIntegerElementIn(t, schema, "urn:test", "value")
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil output and diagnostic", output, err)
				}
				d := requireDiagnostic(t, err)
				wantSpec := schemaElementTypeSpecRef(profile.version)
				if shape.name == "named" {
					wantSpec = schemaSimpleTypeSpecRef(profile.version)
				}
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != declaration.Loc() || d.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("%s long generation diagnostic = %s", shape.name, d)
				}
			})
		}
	}
}
