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

const (
	negativeRootNamespace  = "urn:negative-root"
	negativeOtherNamespace = "urn:negative-other"
	negativeHugeValue      = "-123456789012345678901234567890123456789012345678901234567890"
)

type negativeRootPolicy struct {
	name    string
	policy  goxsd9.LanguagePolicy
	version goxsd9.XSDVersion
}

func negativeRootPolicies() []negativeRootPolicy {
	return []negativeRootPolicy{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: goxsd9.XSDVersion11},
		{name: "Strict10", policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
		{name: "Strict11", policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
	}
}

func negativeRootSchema(t *testing.T, profile negativeRootPolicy) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + negativeRootNamespace + `" xmlns:o="` + negativeOtherNamespace + `" targetNamespace="` + negativeRootNamespace + `" version="unrelated-user-label">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="` + negativeOtherNamespace + `" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:negativeInteger"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="inherited" type="r:Derived"/>
  <xs:element name="window" type="r:Window"/>
  <xs:element name="digits" type="r:Digits"/>
  <xs:element name="enumerated" type="r:Enumerated"/>
  <xs:simpleType name="Forward"><xs:restriction base="r:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>
  <xs:simpleType name="Derived"><xs:restriction base="r:Bounded"/></xs:simpleType>
  <xs:simpleType name="Bounded"><xs:restriction base="xs:negativeInteger"><xs:maxInclusive value="-10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Window"><xs:restriction base="xs:negativeInteger"><xs:minInclusive value="-20"/><xs:maxExclusive value="-10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Digits"><xs:restriction base="xs:negativeInteger"><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:negativeInteger"><xs:enumeration value="-7"/><xs:enumeration value="-9"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]validationTestFixture{
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + negativeRootNamespace + `" targetNamespace="` + negativeRootNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:negativeInteger"/></xs:simpleType><xs:element name="included" type="r:Included"/></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + negativeRootNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:negativeInteger"/></xs:simpleType><xs:element name="chameleon" type="r:Chameleon"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + negativeOtherNamespace + `" targetNamespace="` + negativeOtherNamespace + `"><xs:simpleType name="Imported"><xs:restriction base="xs:negativeInteger"/></xs:simpleType><xs:element name="imported" type="o:Imported"/></xs:schema>`},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, profile.policy)
}

func negativeRootElement(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName(%q,%q): %v", namespace, local, err)
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

func negativeRootType(t *testing.T, schema goxsd9.Schema, local string) goxsd9.SimpleTypeDefinition {
	t.Helper()
	name, err := goxsd9.NewQName(negativeRootNamespace, local)
	if err != nil {
		t.Fatalf("NewQName(%q): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s types = %d, want one", local, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type view", local)
	}
	return definition
}

func negativeRootInstance(local, value string) string {
	if local == "imported" {
		return `<o:imported xmlns:o="` + negativeOtherNamespace + `">` + value + `</o:imported>`
	}
	return `<` + local + ` xmlns="` + negativeRootNamespace + `">` + value + `</` + local + `>`
}

//nolint:gocognit,funlen // Each policy exercises the same public root and completed-fact contract.
func TestValidateNegativeIntegerGlobalRootsAcrossPolicies(t *testing.T) {
	for _, profile := range negativeRootPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := negativeRootSchema(t, profile)
			before := schema.Components()
			direct := negativeRootElement(t, schema, negativeRootNamespace, "direct")
			reference, ok := direct.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Name().Local() != "negativeInteger" || reference.Loc().IsZero() {
				t.Fatalf("direct type reference = %#v/%t, want located built-in negativeInteger", reference, ok)
			}
			bounds, ok := reference.IntegerBounds()
			maximum, hasMaximum := bounds.MaxInclusiveFacet()
			if !ok || !hasMaximum || bounds.Version() != profile.version || maximum.Kind() != goxsd9.BoundMaxInclusive || maximum.Value().Canonical() != "-1" || !maximum.Loc().IsZero() {
				t.Fatalf("direct bounds = %#v/%t, want intrinsic maxInclusive=-1 in %s", bounds, ok, profile.version)
			}
			copied, ok := reference.IntegerBounds()
			copiedMaximum, hasCopiedMaximum := copied.MaxInclusiveFacet()
			if !ok || !hasCopiedMaximum || copiedMaximum.Value().Canonical() != "-1" {
				t.Fatal("repeated IntegerBounds lost the intrinsic maximum")
			}
			mutated := copied.Bounds()
			replacement, err := goxsd9.ParseIntegerMaxInclusiveFacet("-2", goxsd9.Loc{}, profile.version)
			if err != nil {
				t.Fatal(err)
			}
			mutated[0] = replacement
			fresh, _ := reference.IntegerBounds()
			freshMaximum, _ := fresh.MaxInclusiveFacet()
			if freshMaximum.Value().Canonical() != "-1" {
				t.Fatal("mutating a copied bounds slice changed the completed schema")
			}
			inherited := negativeRootType(t, schema, "Derived")
			inheritedBounds, ok := inherited.IntegerBounds()
			inheritedMaximum, hasInheritedMaximum := inheritedBounds.MaxInclusiveFacet()
			if !ok || !hasInheritedMaximum || inheritedMaximum.Value().Canonical() != "-10" || inheritedMaximum.Loc().Source() != "root.xsd" {
				t.Fatalf("inherited maximum = %#v/%t, want located -10", inheritedMaximum, hasInheritedMaximum)
			}
			for _, graph := range []struct{ local, namespace, source string }{
				{"included", negativeRootNamespace, "included.xsd"},
				{"chameleon", negativeRootNamespace, "chameleon.xsd"},
				{"imported", negativeOtherNamespace, "other.xsd"},
			} {
				declaration := negativeRootElement(t, schema, graph.namespace, graph.local)
				id, hasID := declaration.TypeID()
				reference, hasReference := declaration.TypeReference()
				graphBounds, hasBounds := reference.IntegerBounds()
				graphMaximum, hasMaximum := graphBounds.MaxInclusiveFacet()
				if declaration.Loc().Source() != goxsd9.SourceID(graph.source) || !hasID || id.Source() != goxsd9.SourceID(graph.source) || !hasReference || !hasBounds || !hasMaximum || graphMaximum.Value().Canonical() != "-1" || graphBounds.Version() != profile.version {
					t.Fatalf("%s graph facts = decl %s, type %v, max %s, version %s", graph.local, declaration.Loc(), id, graphMaximum.Value().Canonical(), graphBounds.Version())
				}
			}
			for _, test := range []struct{ name, local, value string }{
				{"direct -1", "direct", "-1"},
				{"direct leading zeros", "direct", "-0001"},
				{"direct XML whitespace", "direct", "\t -0002 \r\n"},
				{"direct arbitrary precision", "direct", negativeHugeValue},
				{"forward named", "forward", "-3"},
				{"inherited named bound", "inherited", "-10"},
				{"window lower edge", "window", "-20"},
				{"window upper edge", "window", "-11"},
				{"included named", "included", "-4"},
				{"chameleon named", "chameleon", "-5"},
				{"imported named", "imported", "-6"},
				{"totalDigits counts value digits", "digits", "-000123"},
				{"enumeration compares values", "enumerated", "-0007"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := negativeRootInstance(test.local, test.value)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("validation changed completed component facts or order")
			}
		})
	}
}

func negativeRootSpec(version goxsd9.XSDVersion, suffix string) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#" + suffix
	}
	return "xsd11-datatypes#" + suffix
}

//nolint:gocognit,funlen // Keep lexical, effective facets, locations, and edition evidence in one matrix.
func TestValidateNegativeIntegerGlobalRootDiagnostics(t *testing.T) {
	for _, profile := range negativeRootPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := negativeRootSchema(t, profile)
			cases := []struct {
				name, local, value, code, spec string
				related                        []goxsd9.Loc
				selfClosing                    bool
			}{
				{"empty", "direct", "", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, true},
				{"bare minus", "direct", "-", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"signless zero", "direct", "0", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"signless positive", "direct", "1", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"plus positive", "direct", "+1", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"decimal", "direct", "-1.0", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"internal whitespace", "direct", "-1 2", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"unicode digit", "direct", "-١", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"non XML whitespace", "direct", "\u00a0-1", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false},
				{"named lexical", "forward", "+1", goxsd9.InvalidIntegerLexicalCode, "negativeInteger", []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "forward").Loc(), negativeRootType(t, schema, "Forward").Loc()}, false},
			}
			minusZeroCode := goxsd9.InvalidIntegerLexicalCode
			minusZeroSpec := "negativeInteger"
			if profile.version == goxsd9.XSDVersion10 {
				minusZeroCode = goxsd9.BoundValueViolationCode
				minusZeroSpec = "cvc-maxInclusive-valid"
			}
			cases = append(cases, struct {
				name, local, value, code, spec string
				related                        []goxsd9.Loc
				selfClosing                    bool
			}{"minus zero", "direct", "-0", minusZeroCode, minusZeroSpec, []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "direct").Loc()}, false})
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					input := negativeRootInstance(test.local, test.value)
					if test.selfClosing {
						input = `<direct xmlns="` + negativeRootNamespace + `"/>`
					}
					assertNegativeRootDiagnostic(t, schema, input, test.code, negativeRootSpec(profile.version, test.spec), test.related, test.selfClosing)
				})
			}
			derived := negativeRootType(t, schema, "Derived")
			bounds, _ := derived.IntegerBounds()
			maximum, _ := bounds.MaxInclusiveFacet()
			assertNegativeRootDiagnostic(t, schema, negativeRootInstance("inherited", "-9"), goxsd9.BoundValueViolationCode, negativeRootSpec(profile.version, "cvc-maxInclusive-valid"), []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "inherited").Loc(), derived.Loc(), maximum.Loc()}, false)
			window := negativeRootType(t, schema, "Window")
			windowBounds, _ := window.IntegerBounds()
			minimum, _ := windowBounds.MinInclusiveFacet()
			windowMaximum, _ := windowBounds.MaxExclusiveFacet()
			assertNegativeRootDiagnostic(t, schema, negativeRootInstance("window", "-21"), goxsd9.BoundValueViolationCode, negativeRootSpec(profile.version, "cvc-minInclusive-valid"), []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "window").Loc(), window.Loc(), minimum.Loc()}, false)
			assertNegativeRootDiagnostic(t, schema, negativeRootInstance("window", "-10"), goxsd9.BoundValueViolationCode, negativeRootSpec(profile.version, "cvc-maxExclusive-valid"), []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "window").Loc(), window.Loc(), windowMaximum.Loc()}, false)
			digits := negativeRootType(t, schema, "Digits")
			digitLoc, _ := digits.DigitFacets().TotalDigitsLoc()
			assertNegativeRootDiagnostic(t, schema, negativeRootInstance("digits", "-1234"), goxsd9.DigitFacetValueViolationCode, negativeRootSpec(profile.version, "cvc-totalDigits-valid"), []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "digits").Loc(), digits.Loc(), digitLoc}, false)
			enumerated := negativeRootType(t, schema, "Enumerated")
			related := []goxsd9.Loc{negativeRootElement(t, schema, negativeRootNamespace, "enumerated").Loc(), enumerated.Loc()}
			related = append(related, enumerated.IntegerEnumerationFacets().Locations()...)
			assertNegativeRootDiagnostic(t, schema, negativeRootInstance("enumerated", "-8"), goxsd9.EnumerationValueViolationCode, negativeRootSpec(profile.version, "cvc-enumeration-valid"), related, false)
		})
	}
}

func assertNegativeRootDiagnostic(t *testing.T, schema goxsd9.Schema, input, code, spec string, related []goxsd9.Loc, selfClosing bool) {
	t.Helper()
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	wantLoc := validationTestTextLoc(t, input)
	if selfClosing {
		wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
	}
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != spec || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.Unwrap() == nil {
		t.Fatalf("diagnostic = %s/%q/%s/%q/%v/%v, want invalid/%s/%s/%q/%v/cause", diagnostic, diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Related(), diagnostic.Unwrap(), code, wantLoc, spec, related)
	}
	second := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if diagnostic.Error() != second.Error() || diagnostic.Code() != second.Code() || diagnostic.Loc() != second.Loc() || diagnostic.SpecRef() != second.SpecRef() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
		t.Fatalf("repeated diagnostic changed: %s / %s", diagnostic, second)
	}
}

func TestValidateNegativeIntegerRootStructurePrecedesValue(t *testing.T) {
	for _, profile := range negativeRootPolicies() {
		schema := negativeRootSchema(t, profile)
		for _, local := range []string{"direct", "forward"} {
			declaration := negativeRootElement(t, schema, negativeRootNamespace, local)
			related := []goxsd9.Loc{declaration.Loc()}
			if local == "forward" {
				related = append(related, negativeRootType(t, schema, "Forward").Loc())
			}
			for _, test := range []struct{ name, input, marker string }{
				{"attribute", `<` + local + ` xmlns="` + negativeRootNamespace + `" flag="x">0</` + local + `>`, `flag=`},
				{"child", negativeRootInstance(local, `0<child/>`), `<child`},
			} {
				t.Run(profile.name+"/"+local+"/"+test.name, func(t *testing.T) {
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					d := validationTestDiagnostic(t, err)
					want := validationTestLoc(t, "instance.xml", 1, strings.Index(test.input, test.marker)+1)
					if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != want || d.SpecRef() != negativeRootStructureSpec(profile.version) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, goxsd9.ErrUnsupported) || d.Unwrap() == nil {
						t.Fatalf("structure diagnostic = %s related %v, want unsupported at %s with %v", d, d.Related(), want, related)
					}
				})
			}
		}
	}
}

func negativeRootStructureSpec(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-structures#cvc-elt"
	}
	return "xsd11-structures#cvc-elt"
}

//nolint:gocognit,funlen // Compare every local shape at each unchanged consumer boundary.
func TestNegativeIntegerLocalConsumersRemainUnsupported(t *testing.T) {
	shapes := []struct{ name, local, global string }{
		{"direct", `<xs:element name="v" type="xs:negativeInteger"/>`, ""},
		{"named", `<xs:element name="v" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`},
		{"inline", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`, ""},
		{"ref direct", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:negativeInteger"/>`},
		{"ref named", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`},
		{"ref inline", `<xs:element ref="r:target"/>`, `<xs:element name="target"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`},
	}
	for _, profile := range negativeRootPolicies() {
		for _, model := range []string{"choice", "sequence"} {
			for _, shape := range shapes {
				t.Run(profile.name+"/"+model+"/"+shape.name, func(t *testing.T) {
					source := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + negativeRootNamespace + `" targetNamespace="` + negativeRootNamespace + `"><xs:element name="root" type="r:Record"/>` + shape.global + `<xs:complexType name="Record"><xs:` + model + `>` + shape.local + `</xs:` + model + `></xs:complexType></xs:schema>`
					schema := validationTestSchemaWithPolicy(t, source, nil, profile.policy)
					root := negativeRootElement(t, schema, negativeRootNamespace, "root")
					definition := negativeRootComplexType(t, schema, "Record")
					var particle goxsd9.Particle
					switch group := definition.Particle().(type) {
					case goxsd9.ChoiceParticle:
						particle = group.Alternatives()[0]
					case goxsd9.SequenceParticle:
						particle = group.Particles()[0]
					default:
						t.Fatalf("Record particle = %T", group)
					}
					child := "v"
					if strings.HasPrefix(shape.name, "ref") {
						child = "target"
					}
					input := negativeRootInstance("root", `<`+child+`>-1</`+child+`>`)
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					d := validationTestDiagnostic(t, err)
					want := validationTestLoc(t, "instance.xml", 1, 1)
					if (model == "choice" && !strings.HasPrefix(shape.name, "ref")) || shape.name == "inline" {
						want = particle.Loc()
					}
					if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != want || d.SpecRef() != negativeRootStructureSpec(profile.version) || d.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("local diagnostic = %s, want unsupported at %s", d, want)
					}
					if !validationTestHasRelated(d.Related(), root.Loc()) || !validationTestHasRelated(d.Related(), definition.Loc()) || !validationTestHasRelated(d.Related(), particle.Loc()) {
						t.Fatalf("local related = %v, missing root/definition/particle", d.Related())
					}
					output, generationErr := goxsd9.GenerateGo(schema, "generated")
					generation := validationTestDiagnostic(t, generationErr)
					wantGenerationLoc := particle.Loc()
					if reference, ok := particle.(goxsd9.ElementReferenceParticle); ok {
						wantGenerationLoc = reference.RefLoc()
					}
					if output != nil || generation.Class() != goxsd9.FailureUnsupported || generation.Code() != "GOXSD9029" || generation.Loc() != wantGenerationLoc || generation.Unwrap() == nil || !errors.Is(generationErr, goxsd9.ErrUnsupported) {
						t.Fatalf("local generation = %q, %s, want nil unsupported at %s", output, generation, wantGenerationLoc)
					}
				})
			}
		}
	}
}

func negativeRootComplexType(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ComplexTypeDefinition {
	t.Helper()
	name, err := goxsd9.NewQName(negativeRootNamespace, local)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s complex definitions = %d", local, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("complex definition view missing")
	}
	return definition
}

func TestNegativeIntegerGlobalInlineAndGenerationStayUnsupported(t *testing.T) {
	for _, profile := range negativeRootPolicies() {
		for _, shape := range []struct {
			name, element, extra string
			inline               bool
		}{
			{"direct", `<xs:element name="value" type="xs:negativeInteger"/>`, "", false},
			{"named", `<xs:element name="value" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, false},
			{"inline", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`, "", true},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				source := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + negativeRootNamespace + `" targetNamespace="` + negativeRootNamespace + `">` + shape.element + shape.extra + `</xs:schema>`
				schema := validationTestSchemaWithPolicy(t, source, nil, profile.policy)
				declaration := negativeRootElement(t, schema, negativeRootNamespace, "value")
				if shape.inline {
					input := negativeRootInstance("value", "-1")
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					d := validationTestDiagnostic(t, err)
					if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != validationTestLoc(t, "instance.xml", 1, 1) || d.SpecRef() != negativeRootStructureSpec(profile.version) || !reflect.DeepEqual(d.Related(), []goxsd9.Loc{declaration.Loc()}) || d.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("inline validation diagnostic = %s related %v", d, d.Related())
					}
				}
				output, err := goxsd9.GenerateGo(schema, "generated")
				d := validationTestDiagnostic(t, err)
				if output != nil || d.Class() != goxsd9.FailureUnsupported || d.Code() != "GOXSD9029" || d.Loc() != declaration.Loc() || d.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("generation = %q, %s, want nil unsupported at %s", output, d, declaration.Loc())
				}
			})
		}
	}
}

//nolint:gocognit // Keep alternate root exits and policy evidence in one table.
func TestNegativeIntegerRootLookupAndDeclarationGates(t *testing.T) {
	for _, profile := range negativeRootPolicies() {
		for _, test := range []struct {
			name, element, input, code, spec string
			unsupported, identity            bool
		}{
			{"unknown root", `<xs:element name="value" type="xs:negativeInteger"/>`, `<missing xmlns="` + negativeRootNamespace + `">-1</missing>`, goxsd9.UnknownInstanceSchemaRootCode, "cvc-elt", false, false},
			{"abstract", `<xs:element name="value" type="xs:negativeInteger" abstract="true"/>`, negativeRootInstance("value", "0"), goxsd9.UnsupportedInstanceValidationCode, "cvc-elt", true, false},
			{"nillable", `<xs:element name="value" type="xs:negativeInteger" nillable="true"/>`, negativeRootInstance("value", "0"), goxsd9.UnsupportedInstanceValidationCode, "cvc-elt", true, false},
			{"identity", `<xs:element name="value" type="xs:negativeInteger"><xs:unique name="u"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:unique></xs:element>`, negativeRootInstance("value", "0"), goxsd9.UnsupportedInstanceValidationCode, "Identity-constraint_Definition_details", true, true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				source := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="` + negativeRootNamespace + `">` + test.element + `</xs:schema>`
				schema := validationTestSchemaWithPolicy(t, source, nil, profile.policy)
				err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
				d := validationTestDiagnostic(t, err)
				wantClass := goxsd9.FailureInvalid
				if test.unsupported {
					wantClass = goxsd9.FailureUnsupported
				}
				wantSpec := "xsd11-structures#" + test.spec
				if profile.version == goxsd9.XSDVersion10 {
					wantSpec = "xsd10-structures#" + test.spec
				}
				wantRelated := []goxsd9.Loc(nil)
				if test.unsupported {
					declaration := negativeRootElement(t, schema, negativeRootNamespace, "value")
					wantRelated = []goxsd9.Loc{declaration.Loc()}
					if test.identity {
						wantRelated = append(wantRelated, declaration.IdentityConstraints()[0].Loc())
					}
				}
				if d.Class() != wantClass || d.Code() != test.code || d.Loc() != validationTestLoc(t, "instance.xml", 1, 1) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), wantRelated) || d.Unwrap() == nil {
					t.Fatalf("gate diagnostic = %s related %v, want %s/%s/%s/%v", d, d.Related(), wantClass, test.code, wantSpec, wantRelated)
				}
				if test.unsupported && !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("gate diagnostic lost ErrUnsupported: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Check attribute admission and each consumer for every shape.
func TestNegativeIntegerAttributeShapesStayOutsideConsumers(t *testing.T) {
	for _, profile := range negativeRootPolicies() {
		for _, shape := range []struct {
			name, attribute, extra string
			schemaUnsupported      bool
		}{
			{"direct", `<xs:attribute name="value" type="xs:negativeInteger"/>`, "", false},
			{"named", `<xs:attribute name="value" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, false},
			{"inline", `<xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:attribute>`, "", true},
			{"ref", `<xs:attribute ref="r:global"/>`, `<xs:attribute name="global" type="xs:negativeInteger"/>`, false},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				source := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + negativeRootNamespace + `" targetNamespace="` + negativeRootNamespace + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Record">` + shape.attribute + `</xs:complexType>` + shape.extra + `</xs:schema>`
				if shape.schemaUnsupported {
					rootSource, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(source)))
					if err != nil {
						t.Fatal(err)
					}
					schema, err := goxsd9.ParseSchemaWithPolicy(rootSource, nil, profile.policy)
					d := validationTestDiagnostic(t, err)
					want := validationTestLoc(t, "root.xsd", 1, strings.Index(source, `<xs:simpleType>`)+1)
					if !reflect.DeepEqual(schema, goxsd9.Schema{}) || d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedSchemaSyntaxCode || d.Loc() != want || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("inline attribute parse = %#v, %s, want nil schema/unsupported at %s", schema, d, want)
					}
					return
				}
				schema := validationTestSchemaWithPolicy(t, source, nil, profile.policy)
				root := negativeRootElement(t, schema, negativeRootNamespace, "root")
				definition := negativeRootComplexType(t, schema, "Record")
				uses := definition.AttributeUses()
				if len(uses) != 1 || uses[0].Loc().IsZero() {
					t.Fatalf("attribute uses = %#v, want one located use", uses)
				}
				input := `<root xmlns="` + negativeRootNamespace + `" value="-1"/>`
				err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
				d := validationTestDiagnostic(t, err)
				wantRelated := []goxsd9.Loc{root.Loc(), definition.Loc(), uses[0].Loc()}
				if d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedInstanceValidationCode || d.Loc() != uses[0].Loc() || d.SpecRef() != negativeRootStructureSpec(profile.version) || !reflect.DeepEqual(d.Related(), wantRelated) || d.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("attribute validation = %s related %v, want unsupported at %s", d, d.Related(), uses[0].Loc())
				}
				output, err := goxsd9.GenerateGo(schema, "generated")
				generated := validationTestDiagnostic(t, err)
				if output != nil || generated.Class() != goxsd9.FailureUnsupported || generated.Code() != "GOXSD9029" || generated.Loc() != uses[0].Loc() || generated.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("attribute generation = %q, %s, want nil unsupported at %s", output, generated, uses[0].Loc())
				}
			})
		}
	}
}
