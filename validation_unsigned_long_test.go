package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const unsignedLongOverflow = "18446744073709551616"

func unsignedLongValidationSchema(t *testing.T, profile unsignedLongPolicyProfile) Schema {
	t.Helper()
	root := schemaUnsignedLongReferenceRoot(profile.version)
	root = strings.Replace(root, "</xs:schema>", `<xs:element name="narrowed" type="t:Tight"/><xs:element name="enum" type="t:Enumerated"/><xs:element name="digits" type="t:Digits"/><xs:element name="exclusive" type="t:Exclusive"/><xs:simpleType name="Digits"><xs:restriction base="xs:unsignedLong"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Exclusive"><xs:restriction base="xs:unsignedLong"><xs:minExclusive value="1"/><xs:maxExclusive value="5"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func unsignedLongDatatypeSpec(version XSDVersion) string {
	return instanceUnsignedLongSpecRef(version)
}

//nolint:gocognit // One policy table checks lexical, exact-value, and facet exits.
func TestValidateUnsignedLongGlobalValuesAndDiagnostics(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema := unsignedLongValidationSchema(t, profile)
			before := schema.Components()
			valid := []struct{ local, value string }{
				{"direct", "0"}, {"direct", unsignedLongMaximum},
				{"direct", strings.Repeat("0", 80) + unsignedLongMaximum},
				{"direct", " \t00042\r\n"}, {"forward", unsignedLongMaximum},
				{"named", "7"}, {"narrowed", "2"}, {"enum", "000"},
				{"enum", "000" + unsignedLongMaximum}, {"digits", "00099"},
				{"exclusive", "2"}, {"exclusive", "4"},
			}
			if profile.version == XSDVersion11 {
				valid = append(valid, struct{ local, value string }{"direct", "+000"}, struct{ local, value string }{"direct", "-000"}, struct{ local, value string }{"named", "+7"})
			}
			for _, test := range valid {
				input := boundedIntegerInstance(test.local, "urn:test", test.value)
				if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("valid %s=%q: %v", test.local, test.value, err)
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("validation mutated completed unsignedLong facts")
			}
			lexical := unsignedLongDatatypeSpec(profile.version)
			cases := []boundedIntegerDiagnosticCase{
				{"direct", unsignedLongOverflow, BoundValueViolationCode, boundedIntegerFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", strings.Repeat("9", 90), BoundValueViolationCode, boundedIntegerFacetSpec(profile.version, "maxInclusive"), Loc{}, true},
				{"direct", "-1", BoundValueViolationCode, boundedIntegerFacetSpec(profile.version, "minInclusive"), Loc{}, true},
				{"direct", "+", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"direct", "1.0", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"direct", "1e2", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"direct", "١", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"direct", "1 2", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"direct", "\u00a01", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"forward", "1.0", InvalidIntegerLexicalCode, lexical, Loc{}, true},
				{"narrowed", "3", BoundValueViolationCode, boundedIntegerFacetSpec(profile.version, "maxInclusive"), longFacetLoc(t, schema, "Tight", "maxInclusive"), true},
				{"enum", "1", EnumerationValueViolationCode, boundedIntegerFacetSpec(profile.version, "enumeration"), Loc{}, true},
				{"digits", "100", DigitFacetValueViolationCode, boundedIntegerFacetSpec(profile.version, "totalDigits"), longDigitLoc(t, schema), true},
				{"exclusive", "1", BoundValueViolationCode, boundedIntegerFacetSpec(profile.version, "minExclusive"), longExclusiveFacetLoc(t, schema, false), true},
				{"exclusive", "5", BoundValueViolationCode, boundedIntegerFacetSpec(profile.version, "maxExclusive"), longExclusiveFacetLoc(t, schema, true), true},
			}
			if profile.version == XSDVersion10 {
				cases[2] = boundedIntegerDiagnosticCase{"direct", "-1", InvalidIntegerLexicalCode, lexical, Loc{}, true}
				cases = append(cases, boundedIntegerDiagnosticCase{"direct", "+0", InvalidIntegerLexicalCode, lexical, Loc{}, true}, boundedIntegerDiagnosticCase{"direct", "-0", InvalidIntegerLexicalCode, lexical, Loc{}, true}, boundedIntegerDiagnosticCase{"forward", "+1", InvalidIntegerLexicalCode, lexical, Loc{}, true})
			}
			for _, test := range cases {
				t.Run(test.local+"/"+test.value, func(t *testing.T) {
					assertBoundedIntegerDiagnosticCase(t, schema, test)
					if test.code == InvalidIntegerLexicalCode {
						assertUnsignedLongLexicalCause(t, schema, test.local, test.value, profile.version)
					}
				})
			}
		})
	}
}

func assertUnsignedLongLexicalCause(t *testing.T, schema Schema, local, lexical string, version XSDVersion) {
	t.Helper()
	input := boundedIntegerInstance(local, "urn:test", lexical)
	d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	wantLoc := mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
	_, parseErr := ParseStrictInteger(lexical, wantLoc)
	if parseErr != nil {
		var underlying Diagnostic
		if !errors.As(d.Unwrap(), &underlying) || underlying.Code() != InvalidIntegerLexicalCode || underlying.Loc() != wantLoc {
			t.Fatalf("%s=%q lexical cause = %v, want original located integer lexical diagnostic", local, lexical, d.Unwrap())
		}
		return
	}
	if version != XSDVersion10 || !errors.Is(d, errInstanceUnsignedLong10Lexical) {
		t.Fatalf("%s=%q lexical cause = %v, want XSD 1.0 unsignedLong spelling failure", local, lexical, d.Unwrap())
	}
}

func TestValidateUnsignedLongComposedGraphRoots(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		root, fixtures := unsignedLongGraphFixtures(profile.version)
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range unsignedLongGraphElementCases() {
			t.Run(profile.name+"/"+entry.local, func(t *testing.T) {
				assertBoundedIntegerGraphRoot(t, schema, entry.namespace, entry.local, entry.named, unsignedLongMaximum, unsignedLongOverflow, profile.version)
			})
		}
	}
}

func TestValidateUnsignedLongCompatibilityMixedGraphUsesGraphPolicy(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="1.1"><xs:include schemaLocation="older.xsd"/><xs:import namespace="urn:other" schemaLocation="newer.xsd"/><xs:element name="newRoot" type="xs:unsignedLong"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"older.xsd": {id: "older.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.0"><xs:element name="oldIncluded" type="xs:unsignedLong"/></xs:schema>`},
		"newer.xsd": {id: "newer.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.1"><xs:element name="newImported" type="xs:unsignedLong"/></xs:schema>`},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Compatibility)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		local, namespace string
	}{
		{"newRoot", "urn:root"},
		{"oldIncluded", "urn:root"},
		{"newImported", "urn:other"},
	} {
		declaration := boundedIntegerElementIn(t, schema, test.namespace, test.local)
		reference, ok := declaration.TypeReference()
		if !ok {
			t.Fatal("built-in reference missing")
		}
		bounds, ok := reference.IntegerBounds()
		if !ok || bounds.Version() != XSDVersion11 {
			t.Fatalf("%s bounds version = %s/%t, want XSD 1.1 graph policy", test.local, bounds.Version(), ok)
		}
		for _, value := range []string{"+0", "-0"} {
			input := boundedIntegerInstance(test.local, test.namespace, value)
			if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("%s Compatibility value %q: %v", test.local, value, err)
			}
		}
	}
}

//nolint:gocognit // Check both empty lexical shapes, owners, and located causes across policies.
func TestValidateUnsignedLongEmptyAndStructureExits(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		schema := unsignedLongValidationSchema(t, profile)
		assertBoundedIntegerEmptyAndStructureExits(t, schema, profile.version, unsignedLongDatatypeSpec(profile.version), unsignedLongOverflow)
		for _, local := range []string{"direct", "forward"} {
			declaration := boundedIntegerElementIn(t, schema, "urn:test", local)
			wantRelated := []Loc{declaration.Loc()}
			if local == "forward" {
				typeID, ok := declaration.TypeID()
				if !ok {
					t.Fatal("named unsignedLong type ID missing")
				}
				target, ok := schema.Lookup(typeID)
				if !ok {
					t.Fatal("named unsignedLong type missing")
				}
				wantRelated = append(wantRelated, target.Loc())
			}
			for _, input := range []string{`<` + local + ` xmlns="urn:test"/>`, boundedIntegerInstance(local, "urn:test", " \t\r\n ")} {
				d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
				wantLoc := mustTestLoc(t, "instance.xml", 1, 1)
				if !strings.HasSuffix(input, "/>") {
					wantLoc = mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
				}
				var underlying Diagnostic
				if d.Class() != FailureInvalid || d.Code() != InvalidIntegerLexicalCode || d.Loc() != wantLoc || d.SpecRef() != unsignedLongDatatypeSpec(profile.version) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.As(d.Unwrap(), &underlying) || underlying.Code() != InvalidIntegerLexicalCode || underlying.Loc() != wantLoc {
					t.Fatalf("%s empty lexical diagnostic = %s, related %v, cause %v", local, d, d.Related(), d.Unwrap())
				}
			}
		}
	}
}

func TestValidateUnsignedLongInlineAndLocalConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		schema := unsignedLongValidationSchema(t, profile)
		input := boundedIntegerInstance("anonymous", "urn:test", "1")
		d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
		declaration := boundedIntegerElementIn(t, schema, "urn:test", "anonymous")
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) || !errors.Is(d, ErrUnsupported) || !errors.Is(d, errInstanceNoDeclaredType) {
			t.Fatalf("global inline diagnostic = %s related %v", d, d.Related())
		}
	}
	assertBoundedIntegerLocalConsumersRemainUnsupported(t, "unsignedLong", "urn:unsigned-long-local")
}

func TestValidateUnsignedLongUsesCopiedExactBounds(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		schema := unsignedLongValidationSchema(t, profile)
		for _, test := range []struct{ local, upper string }{{"direct", unsignedLongMaximum}, {"forward", unsignedLongMaximum}, {"narrowed", "2"}} {
			declaration := boundedIntegerElementIn(t, schema, "urn:test", test.local)
			reference, ok := declaration.TypeReference()
			if !ok {
				t.Fatal("type reference missing")
			}
			bounds, ok := reference.IntegerBounds()
			if !ok || bounds.Version() != profile.version || len(bounds.Bounds()) != 2 || bounds.Bounds()[0].Value().Canonical() != "0" || bounds.Bounds()[1].Value().Canonical() != test.upper {
				t.Fatalf("%s copied bounds = %v", test.local, bounds.Bounds())
			}
			bounds.lower.loc = Loc{}
			again, ok := reference.IntegerBounds()
			if !ok || len(again.Bounds()) != 2 || again.Bounds()[0].Value().Canonical() != "0" {
				t.Fatal("copied bounds mutated schema")
			}
		}
	}
}

//nolint:gocognit // Keep every global type shape's generation boundary in one table.
func TestUnsignedLongGenerationRemainsUnsupportedForGlobalShapes(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct{ name, element, definition string }{
			{"direct", `<xs:element name="value" type="xs:unsignedLong"/>`, ""},
			{"named", `<xs:element name="value" type="t:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`},
			{"inline", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:element>`, ""},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.element + test.definition + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil output and unsupported", output, err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := boundedIntegerElementIn(t, schema, "urn:test", "value").Loc()
				wantSpec := schemaElementTypeSpecRef(profile.version)
				if test.name == "named" {
					wantSpec = schemaSimpleTypeSpecRef(profile.version)
				}
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != wantLoc || d.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("%s generation diagnostic = %s, loc %s want %s spec %q want %q cause %v/%v", test.name, d, d.Loc(), wantLoc, d.SpecRef(), wantSpec, errors.Is(err, ErrUnsupported), errors.Is(err, errCodegenUnsupported))
				}
			})
		}
	}
}
