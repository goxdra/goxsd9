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

func shortInstance(local, namespace, value string) string {
	return `<` + local + ` xmlns="` + namespace + `">` + value + `</` + local + `>`
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
				input := shortInstance(test.local, "urn:test", test.value)
				for range 2 {
					if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("valid %s=%q: %v", test.local, test.value, err)
					}
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("short validation mutated completed schema facts")
			}
			for _, test := range []struct {
				local, value, code, spec string
				facet                    Loc
				cause                    bool
			}{
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
				input := shortInstance(test.local, "urn:test", test.value)
				err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{requireShortElement(t, schema, test.local, "urn:test").Loc()}
				if test.local != "direct" {
					definitionName := map[string]string{"forward": "Later", "narrowed": "Tight", "enum": "Enumerated", "digits": "Digits"}[test.local]
					wantRelated = append(wantRelated, requireShortDefinition(t, schema, definitionName).Loc())
				}
				if test.local == "enum" {
					wantRelated = append(wantRelated, requireShortDefinition(t, schema, "Enumerated").IntegerEnumerationFacets().Locations()...)
				}
				if test.local != "enum" && !test.facet.IsZero() {
					wantRelated = append(wantRelated, test.facet)
				}
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != shortTextLoc(t, input) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), wantRelated) {
					t.Fatalf("%s=%q: %s; code=%s loc=%s spec=%s related=%v; want %s/%s/%s/%v", test.local, test.value, d, d.Code(), d.Loc(), d.SpecRef(), d.Related(), test.code, shortTextLoc(t, input), test.spec, wantRelated)
				}
				if test.cause && d.Unwrap() == nil {
					t.Fatalf("%s=%q lost facet cause", test.local, test.value)
				}
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

//nolint:gocognit // Graph roots exercise included, chameleon, imported, named, and cyclic discovery facts.
func TestValidateShortComposedGraphRoots(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root, fixtures := shortGraphFixtures(profile.version)
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatalf("%s graph: %v", profile.name, err)
		}
		for _, entry := range shortGraphElementCases() {
			t.Run(profile.name+"/"+entry.local, func(t *testing.T) {
				declaration := shortElementIn(t, schema, entry.namespace, entry.local)
				input := shortInstance(entry.local, entry.namespace, "32767")
				if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("graph short valid value: %v", err)
				}
				input = shortInstance(entry.local, entry.namespace, "32768")
				d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
				wantRelated := []Loc{declaration.Loc()}
				if entry.named {
					typeID, ok := declaration.TypeID()
					if !ok {
						t.Fatal("named graph root has no type ID")
					}
					target, ok := schema.Lookup(typeID)
					if !ok {
						t.Fatal("named graph root target missing")
					}
					wantRelated = append(wantRelated, target.Loc())
				}
				if d.Class() != FailureInvalid || d.Code() != BoundValueViolationCode || d.Loc() != shortTextLoc(t, input) || d.SpecRef() != shortFacetSpec(profile.version, "maxInclusive") || !reflect.DeepEqual(d.Related(), wantRelated) || d.Unwrap() == nil {
					t.Fatalf("graph short diagnostic = %s, related %v, want %v", d, d.Related(), wantRelated)
				}
			})
		}
	}
}

func shortElementIn(t *testing.T, schema Schema, namespace, local string) ElementDeclaration {
	t.Helper()
	matches := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, namespace, local))
	if len(matches) != 1 {
		t.Fatalf("element %s:%s count = %d", namespace, local, len(matches))
	}
	declaration, ok := matches[0].ElementDeclaration()
	if !ok {
		t.Fatal("element declaration missing")
	}
	return declaration
}

//nolint:gocognit // Compare empty lexical exits and structure precedence under each policy.
func TestValidateShortEmptyAndStructureExits(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := shortValidationSchema(t, profile)
		declaration := requireShortElement(t, schema, "direct", "urn:test")
		for _, input := range []string{`<direct xmlns="urn:test"/>`, shortInstance("direct", "urn:test", " \t\n ")} {
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

//nolint:gocognit // Each admitted local type shape and both consumers retain the existing unsupported gate.
func TestValidateShortLocalAndReferenceConsumersRemainUnsupported(t *testing.T) {
	shapes := []struct{ name, local, global string }{
		{"direct", `<xs:element name="v" type="xs:short"/>`, ""},
		{"named", `<xs:element name="v" type="r:Alias"/>`, ""},
		{"ref direct", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:short"/>`},
		{"ref named", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="r:Alias"/>`},
		{"ref inline", `<xs:element ref="r:target"/>`, `<xs:element name="target"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:element>`},
	}
	for _, profile := range longPolicyProfiles() {
		for _, consumer := range []string{"choice", "sequence"} {
			for _, shape := range shapes {
				t.Run(profile.name+"/"+consumer+"/"+shape.name, func(t *testing.T) {
					body := `<xs:element name="root" type="r:Record"/>` + shape.global + `<xs:complexType name="Record"><xs:` + consumer + `>` + shape.local + `</xs:` + consumer + `></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:short"/></xs:simpleType>`
					source := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:short-local" targetNamespace="urn:short-local" version="` + string(profile.version) + `">` + body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, source, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					root := shortElementIn(t, schema, "urn:short-local", "root")
					definitions := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:short-local", "Record"))
					if len(definitions) != 1 {
						t.Fatalf("Record count = %d", len(definitions))
					}
					definition, ok := definitions[0].ComplexTypeDefinition()
					if !ok {
						t.Fatal("Record view missing")
					}
					var groupLoc, particleLoc Loc
					if choice, ok := definition.Particle().(ChoiceParticle); ok {
						groupLoc, particleLoc = choice.Loc(), choice.Alternatives()[0].Loc()
					}
					if sequence, ok := definition.Particle().(SequenceParticle); ok {
						groupLoc, particleLoc = sequence.Loc(), sequence.Particles()[0].Loc()
					}
					if groupLoc.IsZero() || particleLoc.IsZero() {
						t.Fatal("local particle locations missing")
					}
					child := "v"
					if strings.HasPrefix(shape.name, "ref") {
						child = "target"
					}
					input := shortInstance("root", "urn:short-local", "<"+child+">0</"+child+">")
					wantPrimary := mustTestLoc(t, "instance.xml", 1, 1)
					if consumer == "choice" && !strings.HasPrefix(shape.name, "ref") {
						wantPrimary = particleLoc
					}
					d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != wantPrimary || d.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(d, ErrUnsupported) {
						t.Fatalf("local short diagnostic = %s", d)
					}
					for _, loc := range []Loc{root.Loc(), definition.Loc(), groupLoc, particleLoc} {
						if !hasRelatedLoc(d.Related(), loc) {
							t.Fatalf("local short related = %v, missing %s", d.Related(), loc)
						}
					}
				})
			}
		}
	}
}

func hasRelatedLoc(related []Loc, want Loc) bool {
	for _, loc := range related {
		if loc == want {
			return true
		}
	}
	return false
}

//nolint:gocognit // Check public bound copies and subsequent validation for each root shape.
func TestValidateShortUsesCopiedExactBounds(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		schema := shortValidationSchema(t, profile)
		for _, test := range []struct{ element, lower, upper string }{
			{"direct", "-32768", "32767"},
			{"forward", "-100", "100"},
			{"narrowed", "-100", "2"},
		} {
			declaration := requireShortElement(t, schema, test.element, "urn:test")
			reference, ok := declaration.TypeReference()
			if !ok {
				t.Fatal("short type reference missing")
			}
			bounds, ok := reference.IntegerBounds()
			if !ok {
				t.Fatal("short exact bounds missing")
			}
			first := bounds.Bounds()
			if len(first) != 2 || first[0].Value().Canonical() != test.lower || first[1].Value().Canonical() != test.upper {
				t.Fatalf("%s bounds = %v, want %s..%s", test.element, first, test.lower, test.upper)
			}
			first[0] = IntegerBoundFacet{}
			bounds.lower.loc = Loc{}
			again, ok := reference.IntegerBounds()
			if !ok || len(again.Bounds()) != 2 || again.Bounds()[0].Value().Canonical() != test.lower {
				t.Fatal("mutating copied short bounds changed reference facts")
			}
			input := shortInstance(test.element, "urn:test", test.lower)
			if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("copied bound mutation affected validation: %v", err)
			}
		}
	}
}
