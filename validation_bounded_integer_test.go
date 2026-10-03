package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func boundedIntegerElementIn(t *testing.T, schema Schema, namespace, local string) ElementDeclaration {
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

func boundedIntegerInstance(local, namespace, value string) string {
	return `<` + local + ` xmlns="` + namespace + `">` + value + `</` + local + `>`
}

func assertBoundedIntegerEmptyAndStructureExits(t *testing.T, schema Schema, version XSDVersion, datatypeSpec, outside string) {
	t.Helper()
	declaration := boundedIntegerElementIn(t, schema, "urn:test", "direct")
	for _, input := range []string{`<direct xmlns="urn:test"/>`, boundedIntegerInstance("direct", "urn:test", " \t\n ")} {
		d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
		want := mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
		if strings.HasSuffix(input, "/>") {
			want = mustTestLoc(t, "instance.xml", 1, 1)
		}
		if d.Class() != FailureInvalid || d.Code() != InvalidIntegerLexicalCode || d.Loc() != want || d.SpecRef() != datatypeSpec || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) {
			t.Fatalf("empty bounded integer diagnostic = %s, related=%v", d, d.Related())
		}
	}
	for _, test := range []struct {
		input, marker string
		cause         error
	}{
		{`<direct xmlns="urn:test" flag="x">` + outside + `</direct>`, `flag=`, errInstanceAttributes},
		{`<direct xmlns="urn:test">` + outside + `<child/></direct>`, `<child`, errInstanceChildElements},
	} {
		d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input))))
		want := mustTestLoc(t, "instance.xml", 1, strings.Index(test.input, test.marker)+1)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != want || d.SpecRef() != instanceValidationSpecRef(version) || !reflect.DeepEqual(d.Related(), []Loc{declaration.Loc()}) || !errors.Is(d, test.cause) || !errors.Is(d, ErrUnsupported) {
			t.Fatalf("bounded integer structure diagnostic = %s", d)
		}
	}
}

func assertBoundedIntegerGraphRoot(t *testing.T, schema Schema, namespace, local string, named bool, upper, outside string, version XSDVersion) {
	t.Helper()
	declaration := boundedIntegerElementIn(t, schema, namespace, local)
	input := boundedIntegerInstance(local, namespace, upper)
	if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
		t.Fatalf("graph valid value: %v", err)
	}
	input = boundedIntegerInstance(local, namespace, outside)
	d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	wantRelated := []Loc{declaration.Loc()}
	if named {
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
	wantLoc := mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
	if d.Class() != FailureInvalid || d.Code() != BoundValueViolationCode || d.Loc() != wantLoc || d.SpecRef() != boundedIntegerFacetSpec(version, "maxInclusive") || !reflect.DeepEqual(d.Related(), wantRelated) || d.Unwrap() == nil {
		t.Fatalf("graph diagnostic = %s, related %v, want %v", d, d.Related(), wantRelated)
	}
}

func boundedIntegerFacetSpec(version XSDVersion, facet string) string {
	prefix := "xsd11"
	if version == XSDVersion10 {
		prefix = "xsd10"
	}
	return prefix + "-datatypes#cvc-" + facet + "-valid"
}

type boundedIntegerDiagnosticCase struct {
	local, value, code, spec string
	facet                    Loc
	cause                    bool
}

func assertBoundedIntegerDiagnosticCase(t *testing.T, schema Schema, test boundedIntegerDiagnosticCase) {
	t.Helper()
	input := boundedIntegerInstance(test.local, "urn:test", test.value)
	d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	wantRelated := []Loc{boundedIntegerElementIn(t, schema, "urn:test", test.local).Loc()}
	if test.local != "direct" {
		definitionName := map[string]string{"forward": "Later", "narrowed": "Tight", "enum": "Enumerated", "digits": "Digits", "exclusive": "Exclusive"}[test.local]
		matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", definitionName))
		if len(matches) != 1 {
			t.Fatalf("simple type %q count = %d", definitionName, len(matches))
		}
		definition, ok := matches[0].SimpleTypeDefinition()
		if !ok {
			t.Fatal("simple type view missing")
		}
		wantRelated = append(wantRelated, definition.Loc())
		if test.local == "enum" {
			wantRelated = append(wantRelated, definition.IntegerEnumerationFacets().Locations()...)
		}
	}
	if test.local != "enum" && !test.facet.IsZero() {
		wantRelated = append(wantRelated, test.facet)
	}
	wantLoc := mustTestLoc(t, "instance.xml", 1, strings.IndexByte(input, '>')+2)
	if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), wantRelated) {
		t.Fatalf("%s=%q: %s; code=%s loc=%s spec=%s related=%v; want %s/%s/%s/%v", test.local, test.value, d, d.Code(), d.Loc(), d.SpecRef(), d.Related(), test.code, wantLoc, test.spec, wantRelated)
	}
	if test.cause && d.Unwrap() == nil {
		t.Fatalf("%s=%q lost facet cause", test.local, test.value)
	}
	cause := boundedIntegerDiagnosticCause(test.code)
	if test.cause && cause != nil && !errors.Is(d, cause) {
		t.Fatalf("%s=%q lost %v: %s", test.local, test.value, cause, d)
	}
}

func boundedIntegerDiagnosticCause(code string) error {
	switch code {
	case BoundValueViolationCode:
		return errBoundValueViolation
	case EnumerationValueViolationCode:
		return errEnumerationValueViolation
	case DigitFacetValueViolationCode:
		return errDigitFacetValueViolation
	default:
		return nil
	}
}

//nolint:gocognit // One table checks each admitted local shape at both consumer boundaries.
func assertBoundedIntegerLocalConsumersRemainUnsupported(t *testing.T, datatype, namespace string) {
	t.Helper()
	shapes := []struct{ name, local, global string }{
		{"direct", `<xs:element name="v" type="xs:` + datatype + `"/>`, ""},
		{"named", `<xs:element name="v" type="r:Alias"/>`, ""},
		{"ref direct", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:` + datatype + `"/>`},
		{"ref named", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="r:Alias"/>`},
		{"ref inline", `<xs:element ref="r:target"/>`, `<xs:element name="target"><xs:simpleType><xs:restriction base="xs:` + datatype + `"/></xs:simpleType></xs:element>`},
	}
	for _, profile := range longPolicyProfiles() {
		for _, consumer := range []string{"choice", "sequence"} {
			for _, shape := range shapes {
				t.Run(profile.name+"/"+consumer+"/"+shape.name, func(t *testing.T) {
					body := `<xs:element name="root" type="r:Record"/>` + shape.global + `<xs:complexType name="Record"><xs:` + consumer + `>` + shape.local + `</xs:` + consumer + `></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:` + datatype + `"/></xs:simpleType>`
					source := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="` + namespace + `" targetNamespace="` + namespace + `" version="` + string(profile.version) + `">` + body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, source, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					root := boundedIntegerElementIn(t, schema, namespace, "root")
					definitions := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, "Record"))
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
					input := boundedIntegerInstance("root", namespace, "<"+child+">0</"+child+">")
					wantPrimary := mustTestLoc(t, "instance.xml", 1, 1)
					if consumer == "choice" && !strings.HasPrefix(shape.name, "ref") {
						wantPrimary = particleLoc
					}
					d := requireDiagnostic(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != wantPrimary || d.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(d, ErrUnsupported) {
						t.Fatalf("local %s diagnostic = %s", datatype, d)
					}
					for _, loc := range []Loc{root.Loc(), definition.Loc(), groupLoc, particleLoc} {
						if !hasRelatedLoc(d.Related(), loc) {
							t.Fatalf("local %s related = %v, missing %s", datatype, d.Related(), loc)
						}
					}
				})
			}
		}
	}
}

func assertBoundedIntegerCopiedBounds(t *testing.T, schema Schema, intrinsicLower, intrinsicUpper string) {
	t.Helper()
	for _, test := range []struct{ element, lower, upper string }{
		{"direct", intrinsicLower, intrinsicUpper},
		{"forward", "-100", "100"},
		{"narrowed", "-100", "2"},
	} {
		declaration := boundedIntegerElementIn(t, schema, "urn:test", test.element)
		reference, ok := declaration.TypeReference()
		if !ok {
			t.Fatal("type reference missing")
		}
		bounds, ok := reference.IntegerBounds()
		if !ok {
			t.Fatal("exact bounds missing")
		}
		first := bounds.Bounds()
		if len(first) != 2 || first[0].Value().Canonical() != test.lower || first[1].Value().Canonical() != test.upper {
			t.Fatalf("%s bounds = %v, want %s..%s", test.element, first, test.lower, test.upper)
		}
		first[0] = IntegerBoundFacet{}
		bounds.lower.loc = Loc{}
		again, ok := reference.IntegerBounds()
		if !ok || len(again.Bounds()) != 2 || again.Bounds()[0].Value().Canonical() != test.lower {
			t.Fatal("mutating copied bounds changed reference facts")
		}
		input := boundedIntegerInstance(test.element, "urn:test", test.lower)
		if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
			t.Fatalf("copied bound mutation affected validation: %v", err)
		}
	}
}
