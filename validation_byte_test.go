package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const validationByteNamespace = "urn:validation-byte"
const validationByteOtherNamespace = "urn:validation-byte-other"
const validationByteHuge = "999999999999999999999999999999999999999999999999999999999999999999999999"

func validationByteSchema(t *testing.T, profile validationNonNegativeIntegerPolicy) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationByteNamespace + `" xmlns:o="` + validationByteOtherNamespace + `" targetNamespace="` + validationByteNamespace + `" version="` + string(profile.version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="` + validationByteOtherNamespace + `" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:byte"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="inherited" type="r:Derived"/>
  <xs:element name="bounded" type="r:Bounded"/>
  <xs:element name="digits" type="r:Digit2"/>
  <xs:element name="enumerated" type="r:Enumerated"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:byte"/></xs:simpleType>
  <xs:simpleType name="Derived"><xs:restriction base="r:Forward"/></xs:simpleType>
  <xs:simpleType name="Bounded"><xs:restriction base="xs:byte"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Digit2"><xs:restriction base="xs:byte"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:byte"><xs:enumeration value="-128"/><xs:enumeration value="127"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]validationTestFixture{
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationByteNamespace + `" targetNamespace="` + validationByteNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:byte"/></xs:simpleType><xs:element name="included" type="r:Included"/></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationByteNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:byte"/></xs:simpleType><xs:element name="chameleon" type="r:Chameleon"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationByteOtherNamespace + `" targetNamespace="` + validationByteOtherNamespace + `"><xs:simpleType name="Imported"><xs:restriction base="xs:byte"/></xs:simpleType><xs:element name="imported" type="o:Imported"/></xs:schema>`},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, profile.policy)
}

func validationByteInstance(element, namespace, value string) string {
	return `<` + element + ` xmlns="` + namespace + `">` + value + `</` + element + `>`
}

func validationByteElement(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(components) != 1 {
		t.Fatalf("element %s:%s = %d components", namespace, local, len(components))
	}
	element, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("missing element view")
	}
	return element
}

func validationByteType(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.SimpleTypeDefinition {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("type %s:%s = %d components", namespace, local, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("missing simple type view")
	}
	return definition
}

//nolint:gocognit // Keep policy, graph, and boundary acceptance in one public behavior table.
func TestValidateInstanceGlobalByteValuesAcrossPolicies(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := validationByteSchema(t, profile)
			before := schema.Components()
			for _, test := range []struct{ name, element, namespace, value string }{
				{"lower inclusive", "direct", validationByteNamespace, "-128"},
				{"upper inclusive", "direct", validationByteNamespace, "+00127"},
				{"negative zero whitespace", "direct", validationByteNamespace, " \t-00\r\n"},
				{"forward named", "forward", validationByteNamespace, "-42"},
				{"inherited named", "inherited", validationByteNamespace, "27"},
				{"named lower bound", "bounded", validationByteNamespace, "-100"},
				{"named upper bound", "bounded", validationByteNamespace, "+100"},
				{"named digit facet", "digits", validationByteNamespace, "-99"},
				{"enumerated value equality", "enumerated", validationByteNamespace, "-0128"},
				{"included", "included", validationByteNamespace, "1"},
				{"chameleon", "chameleon", validationByteNamespace, "2"},
				{"imported", "imported", validationByteOtherNamespace, "3"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationByteInstance(test.element, test.namespace, test.value)
					for range 2 {
						if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
							t.Fatalf("ValidateInstance(%q): %v", input, err)
						}
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("byte validation mutated completed component facts")
			}
		})
	}
}

//nolint:gocognit // Each exit at the root datatype boundary checks its public evidence.
func TestValidateInstanceGlobalByteDiagnosticsAcrossPolicies(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			schema := validationByteSchema(t, profile)
			for _, test := range []struct {
				name, element, value, code, spec string
				related                          []goxsd9.Loc
			}{
				{"below intrinsic", "direct", "-129", goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "minInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"above intrinsic", "direct", "128", goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "maxInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"huge positive", "direct", validationByteHuge, goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "maxInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"huge negative", "direct", "-" + validationByteHuge, goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "minInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"bare sign", "direct", "+", goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"decimal", "direct", "1.0", goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"exponent", "direct", "1e2", goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"unicode digit", "direct", "١", goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"internal whitespace", "direct", "1 2", goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}},
				{"named inherited intrinsic", "forward", "128", goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "maxInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "forward").Loc(), validationByteType(t, schema, validationByteNamespace, "Forward").Loc()}},
				{"named bound", "bounded", "101", goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "maxInclusive"), validationByteBoundRelated(t, schema)},
				{"named digit facet", "digits", "100", goxsd9.DigitFacetValueViolationCode, validationNonNegativeIntegerDigitSpecRef(profile.version), validationByteDigitRelated(t, schema)},
				{"named enumeration", "enumerated", "0", goxsd9.EnumerationValueViolationCode, validationNonNegativeIntegerEnumerationSpecRef(profile.version), validationByteEnumerationRelated(t, schema)},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationByteInstance(test.element, validationByteNamespace, test.value)
					first := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if first.Class() != goxsd9.FailureInvalid || first.Code() != test.code || first.Loc() != validationTestTextLoc(t, input) || first.SpecRef() != test.spec || !reflect.DeepEqual(first.Related(), test.related) {
						t.Fatalf("diagnostic = %v; code=%s loc=%s spec=%s related=%v; want %s/%s/%s/%v", first, first.Code(), first.Loc(), first.SpecRef(), first.Related(), test.code, validationTestTextLoc(t, input), test.spec, test.related)
					}
					if test.code != goxsd9.InvalidIntegerLexicalCode && first.Unwrap() == nil {
						t.Fatal("datatype failure lost its cause")
					}
					second := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if first.Error() != second.Error() || !reflect.DeepEqual(first.Related(), second.Related()) {
						t.Fatalf("repeated byte diagnostic changed: %v / %v", first, second)
					}
				})
			}
		})
	}
}

func validationByteDatatypeSpec(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#byte"
	}
	return "xsd11-datatypes#byte"
}

func validationByteBoundRelated(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	definition := validationByteType(t, schema, validationByteNamespace, "Bounded")
	bound, ok := definition.IntegerBounds()
	if !ok {
		t.Fatal("named byte bounds missing")
	}
	maximum, ok := bound.MaxInclusiveFacet()
	if !ok {
		t.Fatal("named byte upper bound missing")
	}
	return []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "bounded").Loc(), definition.Loc(), maximum.Loc()}
}

func validationByteEnumerationRelated(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	definition := validationByteType(t, schema, validationByteNamespace, "Enumerated")
	return append([]goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "enumerated").Loc(), definition.Loc()}, definition.IntegerEnumerationFacets().Locations()...)
}

func validationByteDigitRelated(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	definition := validationByteType(t, schema, validationByteNamespace, "Digit2")
	facet, ok := definition.DigitFacets().TotalDigitsLoc()
	if !ok {
		t.Fatal("named byte totalDigits location missing")
	}
	return []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "digits").Loc(), definition.Loc(), facet}
}

func TestValidateInstanceByteRootStructureBeforeDatatype(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		schema := validationByteSchema(t, profile)
		for _, input := range []string{
			`<direct xmlns="` + validationByteNamespace + `" flag="x">128</direct>`,
			`<direct xmlns="` + validationByteNamespace + `">128<child/></direct>`,
		} {
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Loc().IsZero() || diagnostic.SpecRef() != validationNonNegativeIntegerStructureSpecRef(profile.version) || !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
				t.Fatalf("byte structural exclusion = %s/%s/%s/%s", diagnostic, diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef())
			}
		}
	}
}

//nolint:gocognit // Keep graph provenance and empty-input exits together across policies.
func TestValidateInstanceByteGraphAndEmptyDiagnostics(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		schema := validationByteSchema(t, profile)
		for _, test := range []struct {
			name, element, namespace, input, code, spec string
			related                                     []goxsd9.Loc
			selfClosed                                  bool
		}{
			{"self closing", "direct", validationByteNamespace, `<direct xmlns="` + validationByteNamespace + `"/>`, goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}, true},
			{"whitespace only", "direct", validationByteNamespace, validationByteInstance("direct", validationByteNamespace, " \t\n "), goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "direct").Loc()}, false},
			{"named lexical", "forward", validationByteNamespace, validationByteInstance("forward", validationByteNamespace, "1.0"), goxsd9.InvalidIntegerLexicalCode, validationByteDatatypeSpec(profile.version), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "forward").Loc(), validationByteType(t, schema, validationByteNamespace, "Forward").Loc()}, false},
			{"included bound", "included", validationByteNamespace, validationByteInstance("included", validationByteNamespace, "128"), goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "maxInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "included").Loc(), validationByteType(t, schema, validationByteNamespace, "Included").Loc()}, false},
			{"chameleon bound", "chameleon", validationByteNamespace, validationByteInstance("chameleon", validationByteNamespace, "-129"), goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "minInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteNamespace, "chameleon").Loc(), validationByteType(t, schema, validationByteNamespace, "Chameleon").Loc()}, false},
			{"imported bound", "imported", validationByteOtherNamespace, validationByteInstance("imported", validationByteOtherNamespace, "128"), goxsd9.BoundValueViolationCode, validationNonNegativeIntegerBoundSpecRef(profile.version, "maxInclusive"), []goxsd9.Loc{validationByteElement(t, schema, validationByteOtherNamespace, "imported").Loc(), validationByteType(t, schema, validationByteOtherNamespace, "Imported").Loc()}, false},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				wantLoc := validationTestTextLoc(t, test.input)
				if test.selfClosed {
					wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
				}
				diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input))))
				if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), test.related) {
					t.Fatalf("graph byte diagnostic = %s; code=%s loc=%s spec=%s related=%v; want %s/%s/%s/%v", diagnostic, diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Related(), test.code, wantLoc, test.spec, test.related)
				}
				if test.code == goxsd9.BoundValueViolationCode && diagnostic.Unwrap() == nil {
					t.Fatal("graph byte bound violation lost its cause")
				}
			})
		}
	}
}

//nolint:gocognit // Exercise each already-modeled byte form at both local consumer gates.
func TestValidateInstanceByteLocalConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range validationNonNegativeIntegerPolicies() {
		for _, consumer := range []string{"choice", "sequence"} {
			for _, shape := range []struct{ name, local, global string }{
				{"direct", `<xs:element name="v" type="xs:byte"/>`, ""},
				{"named", `<xs:element name="v" type="r:Alias"/>`, ""},
				{"ref direct", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:byte"/>`},
				{"ref named", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="r:Alias"/>`},
				{"ref inline", `<xs:element ref="r:target"/>`, `<xs:element name="target"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`},
			} {
				t.Run(profile.name+"/"+consumer+"/"+shape.name, func(t *testing.T) {
					body := `<xs:element name="root" type="r:Root"/>` + shape.global + `<xs:complexType name="Root"><xs:` + consumer + `>` + shape.local + `</xs:` + consumer + `></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`
					source := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationByteNamespace + `" targetNamespace="` + validationByteNamespace + `" version="` + string(profile.version) + `">` + body + `</xs:schema>`
					schema := validationTestSchemaWithPolicy(t, source, nil, profile.policy)
					root := validationByteElement(t, schema, validationByteNamespace, "root")
					complexName, err := goxsd9.NewQName(validationByteNamespace, "Root")
					if err != nil {
						t.Fatal(err)
					}
					components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, complexName)
					if len(components) != 1 {
						t.Fatalf("Root complex definition count = %d", len(components))
					}
					definition, ok := components[0].ComplexTypeDefinition()
					if !ok {
						t.Fatal("missing complex type view")
					}
					childName := "v"
					if strings.HasPrefix(shape.name, "ref") {
						childName = "target"
					}
					input := validationByteInstance("root", validationByteNamespace, "<"+childName+">0</"+childName+">")
					var particleLoc goxsd9.Loc
					var groupLoc goxsd9.Loc
					if consumer == "choice" {
						choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
						if !ok || len(choice.Alternatives()) != 1 {
							t.Fatalf("choice particle = %T", definition.Particle())
						}
						groupLoc = choice.Loc()
						particleLoc = choice.Alternatives()[0].Loc()
					}
					if consumer == "sequence" {
						sequence, ok := definition.Particle().(goxsd9.SequenceParticle)
						if !ok || len(sequence.Particles()) != 1 {
							t.Fatalf("sequence particle = %T", definition.Particle())
						}
						groupLoc = sequence.Loc()
						particleLoc = sequence.Particles()[0].Loc()
					}
					wantPrimary := validationTestLoc(t, "instance.xml", 1, 1)
					if consumer == "choice" && !strings.HasPrefix(shape.name, "ref") {
						wantPrimary = particleLoc
					}
					for range 2 {
						diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
						if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Loc() != wantPrimary || diagnostic.SpecRef() != validationNonNegativeIntegerStructureSpecRef(profile.version) || !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
							t.Fatalf("byte %s %s exclusion = %s; code=%s loc=%s spec=%s", consumer, shape.name, diagnostic, diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef())
						}
						for _, required := range []goxsd9.Loc{root.Loc(), definition.Loc(), groupLoc, particleLoc} {
							if !validationTestHasRelated(diagnostic.Related(), required) {
								t.Fatalf("byte %s %s related = %v, missing %s", consumer, shape.name, diagnostic.Related(), required)
							}
						}
					}
				})
			}
		}
	}
}
