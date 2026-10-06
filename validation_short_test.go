package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const shortValidationHuge = "999999999999999999999999999999999999999999999999999999999999999999999999"

func shortValidationSchema(t *testing.T, profile longPolicyProfile) Schema {
	t.Helper()
	root := schemaShortReferenceRoot(profile.version)
	root = strings.Replace(root, "</xs:schema>", `<xs:element name="enum" type="t:Enumerated"/><xs:element name="digits" type="t:Digits"/><xs:simpleType name="Digits"><xs:restriction base="p:short"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err != nil {
		t.Fatalf("discover short validation schema: %v", err)
	}
	return schema
}

func shortTextLoc(t *testing.T, input string) Loc {
	t.Helper()
	return mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
}

func shortDatatypeSpec(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceShortXSD10SpecRef
	}
	return instanceShortXSD11SpecRef
}

func shortFacetSpec(version XSDVersion, facet string) string {
	prefix := "xsd11"
	if version == XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-" + facet + "-valid"
}

//nolint:gocognit // One table covers the exact root value and each datatype exit under every policy.
func TestValidateShortGlobalScalarValuesAndDiagnostics(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema := shortValidationSchema(t, profile)
			before := schema.Components()
			for _, test := range []struct{ local, value string }{
				{"direct", "-32768"}, {"direct", "+0032767"}, {"direct", " \t-00\r\n"},
				{"forward", "-100"}, {"narrowed", "2"}, {"named", "+00100"},
				{"enum", "-032768"}, {"digits", "-99"},
			} {
				input := boundedIntegerInstance(test.local, "urn:test", test.value)
				for range 2 {
					if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("valid %s=%q: %v", test.local, test.value, err)
					}
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("short validation mutated completed schema facts")
			}
			for _, test := range []boundedIntegerDiagnosticCase{
				{"direct", "-32769", BoundValueViolationCode, shortFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "32768", BoundValueViolationCode, shortFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", shortValidationHuge, BoundValueViolationCode, shortFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", "-" + shortValidationHuge, BoundValueViolationCode, shortFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "+", InvalidIntegerLexicalCode, shortDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1.0", InvalidIntegerLexicalCode, shortDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1e2", InvalidIntegerLexicalCode, shortDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "١", InvalidIntegerLexicalCode, shortDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "1 2", InvalidIntegerLexicalCode, shortDatatypeSpec(profile.version), Loc{}, false},
				{"direct", "\u00a01", InvalidIntegerLexicalCode, shortDatatypeSpec(profile.version), Loc{}, false},
				{"forward", "101", BoundValueViolationCode, shortFacetSpec(profile.version, "maxInclusive"), shortFacetLoc(t, schema, "Later", "maxInclusive"), true},
				{"narrowed", "3", BoundValueViolationCode, shortFacetSpec(profile.version, "maxInclusive"), shortFacetLoc(t, schema, "Tight", "maxInclusive"), true},
				{"enum", "0", EnumerationValueViolationCode, shortFacetSpec(profile.version, "enumeration"), requireShortDefinition(t, schema, "Enumerated").IntegerEnumerationFacets().Locations()[0], true},
				{"digits", "100", DigitFacetValueViolationCode, shortFacetSpec(profile.version, "totalDigits"), shortDigitLoc(t, schema), true},
			} {
				assertBoundedIntegerDiagnosticCase(t, schema, test)
			}
		})
	}
}

func shortFacetLoc(t *testing.T, schema Schema, name, bound string) Loc {
	t.Helper()
	bounds, ok := requireShortDefinition(t, schema, name).IntegerBounds()
	if !ok {
		t.Fatal("short bound facts missing")
	}
	if bound == "maxInclusive" {
		facet, present := bounds.MaxInclusiveFacet()
		if !present {
			t.Fatal("short maximum missing")
		}
		return facet.Loc()
	}
	facet, present := bounds.MinInclusiveFacet()
	if !present {
		t.Fatal("short minimum missing")
	}
	return facet.Loc()
}

func shortDigitLoc(t *testing.T, schema Schema) Loc {
	t.Helper()
	loc, ok := requireShortDefinition(t, schema, "Digits").DigitFacets().TotalDigitsLoc()
	if !ok {
		t.Fatal("short totalDigits location missing")
	}
	return loc
}

func TestValidateShortComposedGraphRoots(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root, fixtures := shortGraphFixtures(profile.version)
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatalf("%s graph: %v", profile.name, err)
		}
		for _, entry := range shortGraphElementCases() {
			t.Run(profile.name+"/"+entry.local, func(t *testing.T) {
				assertBoundedIntegerGraphRoot(t, schema, entry.namespace, entry.local, entry.named, "32767", "32768", profile.version)
			})
		}
	}
}

//nolint:gocognit // Keep policy-specific lexical and structural precedence exits together.
func TestValidateShortEmptyAndStructureExits(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := shortValidationSchema(t, profile)
		declaration := requireShortElement(t, schema, "direct", "urn:test")
		for _, input := range []string{`<direct xmlns="urn:test"/>`, boundedIntegerInstance("direct", "urn:test", " \t\n ")} {
			d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			want := shortTextLoc(t, input)
			if strings.HasSuffix(input, "/>") {
				want = mustTestLoc(t, "instance.xml", 1, 1)
			}
			if d.Class() != FailureInvalid || d.Code() != InvalidIntegerLexicalCode || d.Loc() != want || d.SpecRef() != shortDatatypeSpec(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) {
				t.Fatalf("empty short diagnostic = %s, related=%v", d, d.Related())
			}
		}
		for _, input := range []string{`<direct xmlns="urn:test" flag="x">32768</direct>`, `<direct xmlns="urn:test">32768<child/></direct>`} {
			d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc().IsZero() || d.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(d, ErrUnsupported) {
				t.Fatalf("short structure diagnostic = %s", d)
			}
		}
	}
}

func TestValidateShortLocalAndReferenceConsumersRemainUnsupported(t *testing.T) {
	assertBoundedIntegerLocalConsumersRemainUnsupported(t, "short", "urn:short-local")
}

func hasRelatedLoc(related []Loc, want Loc) bool {
	for _, loc := range related {
		if loc == want {
			return true
		}
	}
	return false
}

func TestValidateShortUsesCopiedExactBounds(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := shortValidationSchema(t, profile)
		assertBoundedIntegerCopiedBounds(t, schema, "-32768", "32767")
	}
}
