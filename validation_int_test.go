package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const intValidationHuge = "999999999999999999999999999999999999999999999999999999999999999999999999"

func intValidationSchema(t *testing.T, profile longPolicyProfile) Schema {
	t.Helper()
	root := schemaIntReferenceRoot(profile.version)
	root = strings.Replace(root, "</xs:schema>", `<xs:element name="enum" type="t:Enumerated"/><xs:element name="digits" type="t:Digits"/><xs:simpleType name="Digits"><xs:restriction base="p:int"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err != nil {
		t.Fatalf("discover int validation schema: %v", err)
	}
	return schema
}

func intTextLoc(t *testing.T, input string) Loc {
	t.Helper()
	return mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
}

func intDatatypeSpec(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceIntXSD10SpecRef
	}
	return instanceIntXSD11SpecRef
}

func intFacetSpec(version XSDVersion, facet string) string {
	prefix := "xsd11"
	if version == XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-" + facet + "-valid"
}

//nolint:gocognit // One table covers the exact root value and each datatype exit under every policy.
func TestValidateIntGlobalScalarValuesAndDiagnostics(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema := intValidationSchema(t, profile)
			before := schema.Components()
			for _, test := range []struct{ local, value string }{
				{"direct", "-2147483648"}, {"direct", "+002147483647"}, {"direct", " \t-00\r\n"},
				{"direct", strings.Repeat("0", 80) + "2147483647"}, {"direct", " \t+00042\n"},
				{"forward", "-100"}, {"narrowed", "2"}, {"named", "+00100"},
				{"enum", "-02147483648"}, {"digits", "-99"},
			} {
				input := boundedIntegerInstance(test.local, "urn:test", test.value)
				for range 2 {
					if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("valid %s=%q: %v", test.local, test.value, err)
					}
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("int validation mutated completed schema facts")
			}
			for _, test := range []boundedIntegerDiagnosticCase{
				{"direct", "-2147483649", BoundValueViolationCode, intFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "2147483648", BoundValueViolationCode, intFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", intValidationHuge, BoundValueViolationCode, intFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", "-" + intValidationHuge, BoundValueViolationCode, intFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "+", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1.0", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1e2", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "١", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1 2", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "\u00a01", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"forward", "1.0", InvalidIntegerLexicalCode, intDatatypeSpec(profile.version), Loc{}, false},
				{"forward", "101", BoundValueViolationCode, intFacetSpec(profile.version, "maxInclusive"), intFacetLoc(t, schema, "Later", "maxInclusive"), true},
				{"narrowed", "3", BoundValueViolationCode, intFacetSpec(profile.version, "maxInclusive"), intFacetLoc(t, schema, "Tight", "maxInclusive"), true},
				{"enum", "0", EnumerationValueViolationCode, intFacetSpec(profile.version, "enumeration"), requireIntDefinition(t, schema, "Enumerated").IntegerEnumerationFacets().Locations()[0], true},
				{"digits", "100", DigitFacetValueViolationCode, intFacetSpec(profile.version, "totalDigits"), intDigitLoc(t, schema), true},
			} {
				assertBoundedIntegerDiagnosticCase(t, schema, test)
			}
		})
	}
}

func intFacetLoc(t *testing.T, schema Schema, name, bound string) Loc {
	t.Helper()
	bounds, ok := requireIntDefinition(t, schema, name).IntegerBounds()
	if !ok {
		t.Fatal("int bound facts missing")
	}
	if bound == "maxInclusive" {
		facet, present := bounds.MaxInclusiveFacet()
		if !present {
			t.Fatal("int maximum missing")
		}
		return facet.Loc()
	}
	facet, present := bounds.MinInclusiveFacet()
	if !present {
		t.Fatal("int minimum missing")
	}
	return facet.Loc()
}

func intDigitLoc(t *testing.T, schema Schema) Loc {
	t.Helper()
	loc, ok := requireIntDefinition(t, schema, "Digits").DigitFacets().TotalDigitsLoc()
	if !ok {
		t.Fatal("int totalDigits location missing")
	}
	return loc
}

func TestValidateIntComposedGraphRoots(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root, fixtures := intGraphFixtures(profile.version)
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatalf("%s graph: %v", profile.name, err)
		}
		for _, entry := range intGraphElementCases() {
			t.Run(profile.name+"/"+entry.local, func(t *testing.T) {
				assertBoundedIntegerGraphRoot(t, schema, entry.namespace, entry.local, entry.named, "2147483647", "2147483648", profile.version)
			})
		}
	}
}

//nolint:gocognit // Keep policy-specific lexical and structural precedence exits together.
func TestValidateIntEmptyAndStructureExits(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := intValidationSchema(t, profile)
		declaration := requireIntElement(t, schema, "direct", "urn:test")
		for _, input := range []string{`<direct xmlns="urn:test"/>`, boundedIntegerInstance("direct", "urn:test", " \t\n ")} {
			d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			want := intTextLoc(t, input)
			if strings.HasSuffix(input, "/>") {
				want = mustTestLoc(t, "instance.xml", 1, 1)
			}
			if d.Class() != FailureInvalid || d.Code() != InvalidIntegerLexicalCode || d.Loc() != want || d.SpecRef() != intDatatypeSpec(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) {
				t.Fatalf("empty int diagnostic = %s, related=%v", d, d.Related())
			}
		}
		for _, test := range []struct {
			input, marker string
			cause         error
		}{
			{`<direct xmlns="urn:test" flag="x">2147483648</direct>`, `flag=`, errInstanceAttributes},
			{`<direct xmlns="urn:test">2147483648<child/></direct>`, `<child`, errInstanceChildElements},
		} {
			d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input))))
			wantLoc := mustTestLoc(t, "instance.xml", 1, strings.Index(test.input, test.marker)+1)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != wantLoc || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) || !errors.Is(d, test.cause) || !errors.Is(d, ErrUnsupported) {
				t.Fatalf("int structure diagnostic = %s", d)
			}
		}
	}
}

func TestValidateIntGlobalInlineRootRemainsUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := intValidationSchema(t, profile)
		input := boundedIntegerInstance("inline", "urn:test", "42")
		d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
		declaration := requireIntElement(t, schema, "inline", "urn:test")
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) || !errors.Is(d, errInstanceNoDeclaredType) || !errors.Is(d, ErrUnsupported) {
			t.Fatalf("global inline int diagnostic = %s, related %v", d, d.Related())
		}
	}
}

func TestValidateIntLocalAndReferenceConsumersRemainUnsupported(t *testing.T) {
	assertBoundedIntegerLocalConsumersRemainUnsupported(t, "int", "urn:int-local")
}

func TestValidateIntUsesCopiedExactBounds(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := intValidationSchema(t, profile)
		assertBoundedIntegerCopiedBounds(t, schema, "-2147483648", "2147483647")
	}
}
