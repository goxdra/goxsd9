package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type boundedLocalFamily struct {
	name, minimum, maximum, parentMinimum, parentMaximum, childMinimum, childMaximum string
}

func boundedLocalFamilies() []boundedLocalFamily {
	return []boundedLocalFamily{
		{"short", "-32768", "32767", "-100", "100", "-3", "4"},
		{"int", "-2147483648", "2147483647", "-100", "100", "-3", "4"},
		{"byte", "-128", "127", "-100", "100", "-3", "4"},
		{"unsignedLong", "0", "18446744073709551615", "0", "100", "1", "4"},
	}
}

func boundedLocalRoot(version, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:bounded-local" xmlns:o="urn:other" targetNamespace="urn:bounded-local" attributeFormDefault="qualified" version="` + version + `">` + body + `</xs:schema>`
}

func boundedLocalGraph(f boundedLocalFamily, version XSDVersion) (string, map[string]discoveryFixture) {
	root := boundedLocalRoot(string(version), `<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Box"><xs:attribute name="builtin" type="xs:`+f.name+`" use="required"/><xs:attribute name="forward" type="r:Child"/><xs:attribute name="inherited" type="r:Parent"/><xs:attribute name="included" type="r:Included" form="unqualified"/><xs:attribute name="adopted" type="r:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/><xs:attribute name="gone" type="xs:`+f.name+`" use="prohibited"/></xs:complexType><xs:simpleType name="Child"><xs:restriction base="r:Parent"><xs:minInclusive value="`+f.childMinimum+`"/><xs:maxInclusive value="`+f.childMaximum+`"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:`+f.name+`"><xs:minInclusive value="`+f.parentMinimum+`"/><xs:maxInclusive value="`+f.parentMaximum+`"/><xs:totalDigits value="3"/><xs:enumeration value="2"/><xs:enumeration value="3"/></xs:restriction></xs:simpleType>`)
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bounded-local"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType></xs:schema>`
	return root, map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
}

//nolint:gocognit,funlen // One ordered graph exposes every admitted type form and its copied public facts.
func TestBoundedLocalAttributeGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root, fixtures := boundedLocalGraph(family, profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("unstable graph: %v", err)
				}
				if len(schema.Documents()) != 4 {
					t.Fatalf("documents = %d, want 4", len(schema.Documents()))
				}
				var walk []string
				if err := schema.Walk(func(c Component) error { walk = append(walk, string(c.ID().Source())+":"+c.Name().Local()); return nil }); err != nil {
					t.Fatal(err)
				}
				wantWalk := []string{"root.xsd:Box", "root.xsd:Child", "root.xsd:Parent", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}
				if !reflect.DeepEqual(walk, wantWalk) {
					t.Fatalf("walk = %v, want %v", walk, wantWalk)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))
				if len(found) != 1 {
					t.Fatalf("Box matches = %d", len(found))
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Box")
				if owner.Particle() != nil {
					t.Fatalf("named empty owner particle = %T", owner.Particle())
				}
				uses := owner.AttributeUses()
				if len(uses) != 6 {
					t.Fatalf("uses = %d, want 6", len(uses))
				}
				want := []struct{ local, lexical, namespace, source, minimum, maximum, minNeedle, maxNeedle string }{
					{"builtin", "xs:" + family.name, testXSDNamespace, "", family.minimum, family.maximum, "", ""},
					{"forward", "r:Child", "urn:bounded-local", "root.xsd", family.childMinimum, family.childMaximum, `value="` + family.childMinimum + `"`, `value="` + family.childMaximum + `"`},
					{"inherited", "r:Parent", "urn:bounded-local", "root.xsd", family.parentMinimum, family.parentMaximum, `value="` + family.parentMinimum + `"`, `value="` + family.parentMaximum + `"`},
					{"included", "r:Included", "urn:bounded-local", "included.xsd", family.minimum, family.maximum, "", ""},
					{"adopted", "r:Adopted", "urn:bounded-local", "chameleon.xsd", family.minimum, family.maximum, "", ""},
					{"foreign", "o:Foreign", "urn:other", "other.xsd", family.minimum, family.maximum, "", ""},
				}
				for index, expected := range want {
					use, ok := uses[index].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use %d = %T", index, uses[index])
					}
					nameNS := "urn:bounded-local"
					if expected.local == "included" {
						nameNS = ""
					}
					declared := mustTestQName(t, expected.namespace, strings.SplitN(expected.lexical, ":", 2)[1])
					if use.Name() != mustTestQName(t, nameNS, expected.local) || use.DeclaredType() != declared || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+expected.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+expected.local+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+expected.lexical+`"`) {
						t.Fatalf("use %d provenance = %#v", index, use)
					}
					if index == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`)) {
						t.Fatalf("required use = %#v", use)
					}
					if index == 3 && use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
						t.Fatalf("form Loc = %s", use.FormLoc())
					}
					ref, ok := use.TypeReference()
					if !ok || ref.QName() != declared || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
						t.Fatalf("reference %d = %#v/%t", index, ref, ok)
					}
					if index == 0 {
						id, hasID := use.TypeID()
						refID, refHasID := ref.ComponentID()
						if !ref.IsBuiltin() || hasID || refHasID || !id.IsZero() || !refID.IsZero() || ref.VarietyLoc() != use.TypeLoc() {
							t.Fatalf("built-in identity = %#v", use)
						}
					}
					if index != 0 {
						id, hasID := use.TypeID()
						refID, refHasID := ref.ComponentID()
						if !ref.IsNamed() || !hasID || !refHasID || id != refID || id != componentIDForName(t, schema, declared) || id.Source() != SourceID(expected.source) {
							t.Fatalf("named identity %d = %v/%v", index, id, refID)
						}
						declaration := fixtures[expected.source].contents
						varietyNeedle := `<xs:restriction base="xs:` + family.name + `"`
						if index == 1 {
							varietyNeedle = `<xs:restriction base="r:Parent"`
						}
						if ref.VarietyLoc() != mustSchemaTokenLoc(t, SourceID(expected.source), declaration, 1, varietyNeedle) {
							t.Fatalf("variety %d = %s", index, ref.VarietyLoc())
						}
					}
					var minLoc, maxLoc Loc
					if expected.minNeedle != "" {
						minLoc = elementReferenceTestAttributeLoc(t, root, expected.minNeedle)
					}
					if expected.maxNeedle != "" {
						maxLoc = elementReferenceTestAttributeLoc(t, root, expected.maxNeedle)
					}
					assertAllIntegerBounds(t, ref, profile.version, expected.minimum, expected.maximum, minLoc, maxLoc)
				}
				assertBoundedLocalFacetProvenance(t, schema, root, profile.version)
				uses[0] = nil
				if owner.AttributeUses()[0].Name().Local() != "builtin" {
					t.Fatal("use slice mutated owner")
				}
				copiedUse, ok := owner.AttributeUses()[1].(LocalAttributeUse)
				if !ok {
					t.Fatal("copied named use missing")
				}
				bounds, ok := copiedUse.TypeReference()
				if !ok {
					t.Fatal("copied named type reference missing")
				}
				copiedBounds, _ := bounds.IntegerBounds()
				for _, bound := range copiedBounds.Bounds() {
					bound.Value().value.SetInt64(0)
				}
				assertAllIntegerBounds(t, bounds, profile.version, family.childMinimum, family.childMaximum, elementReferenceTestAttributeLoc(t, root, `value="`+family.childMinimum+`"`), elementReferenceTestAttributeLoc(t, root, `value="`+family.childMaximum+`"`))
			})
		}
	}
}

//nolint:gocognit // This checks inherited digits and ordered enumeration through the public definition view.
func assertBoundedLocalFacetProvenance(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	for _, name := range []string{"Parent", "Child"} {
		found := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:bounded-local", name))
		if len(found) != 1 {
			t.Fatalf("%s matches = %d", name, len(found))
		}
		definition, ok := found[0].SimpleTypeDefinition()
		if !ok {
			t.Fatalf("%s definition missing", name)
		}
		digits := definition.DigitFacets()
		total, hasTotal := digits.TotalDigits()
		loc, hasLoc := digits.TotalDigitsLoc()
		if !hasTotal || total.Canonical() != "3" || !hasLoc || loc != elementReferenceTestAttributeLoc(t, root, `value="3"`) || digits.Version() != version {
			t.Fatalf("%s inherited totalDigits = %s/%s", name, total.Canonical(), loc)
		}
		total.value.SetInt64(0)
		if again, _ := definition.DigitFacets().TotalDigits(); again.Canonical() != "3" {
			t.Fatalf("%s totalDigits copy mutated schema", name)
		}
		fraction, hasFraction := digits.FractionDigits()
		fractionFixed, hasFractionFixed := digits.FractionDigitsFixed()
		fractionLoc, hasFractionLoc := digits.FractionDigitsLoc()
		if !hasFraction || fraction.Canonical() != "0" || !hasFractionFixed || !fractionFixed || !hasFractionLoc || !fractionLoc.IsZero() {
			t.Fatalf("%s intrinsic fractionDigits = %s/%t", name, fraction.Canonical(), hasFraction)
		}
		if name == "Parent" {
			parentValues := definition.IntegerEnumerationFacets().Values()
			parentLocs := definition.IntegerEnumerationFacets().Locations()
			lastTwo := strings.LastIndex(root, `<xs:enumeration value="2"`)
			if lastTwo < 0 {
				t.Fatal("parent enumeration fixture is absent")
			}
			wantFirst := mustTestLoc(t, "root.xsd", 1, lastTwo+1)
			wantSecond := elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="3"`)
			if len(parentValues) != 2 || parentValues[0].Canonical() != "2" || parentValues[1].Canonical() != "3" || !reflect.DeepEqual(parentLocs, []Loc{wantFirst, wantSecond}) {
				t.Fatalf("parent enumeration = %v/%v", parentValues, parentLocs)
			}
		}
	}
	child := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:bounded-local", "Child"))[0]
	definition, _ := child.SimpleTypeDefinition()
	enumeration := definition.IntegerEnumerationFacets()
	values, locations := enumeration.Values(), enumeration.Locations()
	if len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locations, []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="2"`)}) {
		t.Fatalf("enumeration = %v/%v", values, locations)
	}
	values[0].value.SetInt64(99)
	locations[0] = Loc{}
	if definition.IntegerEnumerationFacets().Values()[0].Canonical() != "2" {
		t.Fatal("enumeration copy mutated schema")
	}
}

type boundedLocalExclusion struct {
	name, body, extra, primary, related string
	spec                                func(XSDVersion) string
	cause                               error
}

func boundedLocalUseShapes(f boundedLocalFamily) []boundedLocalExclusion {
	named := `<xs:simpleType name="T"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType>`
	return []boundedLocalExclusion{
		{"direct", `<xs:attribute name="a" type="xs:` + f.name + `"/>`, "", `type="xs:` + f.name + `"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"named", `<xs:attribute name="a" type="r:T"/>`, named, `type="r:T"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + f.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"ref", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:` + f.name + `"/>`, `ref="r:a"`, `<xs:attribute name="a" type="xs:`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
	}
}

//nolint:gocognit // Excluded type shapes are checked independently at every affected owner boundary.
func TestBoundedLocalAttributeExcludedShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			shapes := boundedLocalUseShapes(family)
			for _, shape := range shapes[2:] {
				t.Run(profile.name+"/"+family.name+"/empty/"+shape.name, func(t *testing.T) {
					root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box">`+shape.body+`</xs:complexType>`+shape.extra)
					assertBoundedLocalExcluded(t, root, shape, profile)
				})
			}
			for _, owner := range []struct{ name, before, after, extra string }{
				{"group", `<xs:complexType name="Box"><xs:group ref="r:Fields"/>`, `</xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`},
				{"grouped extension", `<xs:complexType name="Box"><xs:complexContent><xs:extension base="r:Base"><xs:group ref="r:Fields"/>`, `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`},
				{"sequence extension", `<xs:complexType name="Box"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>`, `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:complexType name="Base"/>`},
				{"simple content", `<xs:complexType name="Box"><xs:simpleContent><xs:extension base="xs:string">`, `</xs:extension></xs:simpleContent></xs:complexType>`, ""},
				{"inline empty", `<xs:element name="root"><xs:complexType>`, `</xs:complexType></xs:element>`, ""},
			} {
				for _, shape := range shapes {
					if owner.name == "grouped extension" && shape.name == "direct" {
						continue
					}
					t.Run(profile.name+"/"+family.name+"/"+owner.name+"/"+shape.name, func(t *testing.T) {
						root := boundedLocalRoot(string(profile.version), owner.before+shape.body+owner.after+owner.extra+shape.extra)
						assertBoundedLocalExcluded(t, root, shape, profile)
					})
				}
			}
			for _, shape := range shapes[2:] {
				t.Run(profile.name+"/"+family.name+"/all/"+shape.name, func(t *testing.T) {
					root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:all><xs:element name="x" type="xs:integer"/></xs:all>`+shape.body+`</xs:complexType>`+shape.extra)
					assertBoundedLocalExcluded(t, root, shape, profile)
				})
			}
			for _, variety := range []struct{ name, body string }{
				{"named list", `<xs:attribute name="a" type="r:T"/></xs:complexType><xs:simpleType name="T"><xs:list itemType="xs:` + family.name + `"/></xs:simpleType>`},
				{"named union", `<xs:attribute name="a" type="r:T"/></xs:complexType><xs:simpleType name="T"><xs:union memberTypes="xs:` + family.name + `"/></xs:simpleType>`},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + family.name + `"/></xs:simpleType></xs:attribute></xs:complexType>`},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + family.name + `"/></xs:simpleType></xs:attribute></xs:complexType>`},
			} {
				t.Run(profile.name+"/"+family.name+"/"+variety.name, func(t *testing.T) {
					root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box">`+variety.body)
					primary := `<xs:simpleType`
					cause := errSchemaAttributeTypeUnsupported
					if strings.HasPrefix(variety.name, "named") {
						primary = `type="r:T"`
						cause = errSchemaAttributeUseUnsupported
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, primary), nil, schemaAttributeTypeSpecRef(profile.version), cause)
				})
			}
			for _, kind := range []string{"default", "fixed"} {
				for _, shape := range []string{"direct", "named", "inline", "ref"} {
					t.Run(profile.name+"/"+family.name+"/value/"+shape+"/"+kind, func(t *testing.T) {
						body, extra := boundedLocalValueUse(family, shape, kind)
						root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box">`+body+`</xs:complexType>`+extra)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, kind+`="2"`), nil, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported)
					})
				}
			}
		}
	}
}

func assertBoundedLocalExcluded(t *testing.T, root string, shape boundedLocalExclusion, profile tokenPolicyProfile) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	var related []Loc
	if shape.related != "" {
		related = []Loc{elementReferenceTestAttributeLoc(t, root, shape.related)}
	}
	assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, shape.primary), related, shape.spec(profile.version), shape.cause)
}

func boundedLocalValueUse(f boundedLocalFamily, shape, kind string) (string, string) {
	value := kind + `="2"`
	switch shape {
	case "direct":
		return `<xs:attribute name="a" type="xs:` + f.name + `" ` + value + `/>`, ""
	case "named":
		return `<xs:attribute name="a" type="r:T" ` + value + `/>`, `<xs:simpleType name="T"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType>`
	case "inline":
		return `<xs:attribute name="a" ` + value + `><xs:simpleType><xs:restriction base="xs:` + f.name + `"/></xs:simpleType></xs:attribute>`, ""
	case "ref":
		return `<xs:attribute ref="r:a" ` + value + `/>`, `<xs:attribute name="a" type="xs:` + f.name + `"/>`
	default:
		panic("impossible bounded local value shape")
	}
}

//nolint:gocognit // Each consumer is a separate public refusal for every admitted family and type shape.
func TestBoundedLocalAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, shape := range []string{"direct", "named"} {
				t.Run(profile.name+"/"+family.name+"/"+shape, func(t *testing.T) {
					typeName, extra := "xs:"+family.name, ""
					if shape == "named" {
						typeName = "r:T"
						extra = `<xs:simpleType name="T"><xs:restriction base="xs:` + family.name + `"><xs:minInclusive value="1"/></xs:restriction></xs:simpleType>`
					}
					root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:attribute name="a" type="`+typeName+`"/></xs:complexType><xs:element name="root" type="r:Box"/>`+extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
					elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:bounded-local" a="2"/>`)))
					vd := requireDiagnostic(t, validationErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, errInstanceAttributes) {
						t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
					}
					output, generationErr := GenerateGo(schema, "generated")
					gd := requireDiagnostic(t, generationErr)
					if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
					}
				})
			}
		}
	}
}

func boundedLocalAboveMaximum(f boundedLocalFamily) string {
	switch f.name {
	case "short":
		return "32768"
	case "int":
		return "2147483648"
	case "byte":
		return "128"
	case "unsignedLong":
		return "18446744073709551616"
	default:
		panic("impossible bounded local family")
	}
}

//nolint:gocognit // These are alternate exits from the admitted local type-resolution boundary.
func TestBoundedLocalAttributeBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, tc := range []struct {
				name, use, extra, primary, related, code string
				class                                    FailureClass
				cause                                    error
				spec                                     func(XSDVersion) string
			}{
				{"unresolved", `<xs:attribute name="a" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
				{"prohibited unresolved", `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
				{"wrong kind", `<xs:attribute name="a" type="r:Wrong"/>`, `<xs:complexType name="Wrong"/>`, `type="r:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef},
				{"prohibited wrong kind", `<xs:attribute name="a" type="r:Wrong" use="prohibited"/>`, `<xs:complexType name="Wrong"/>`, `type="r:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef},
				{"cycle", `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, "", diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, errSchemaSimpleTypeBaseCycle, schemaAttributeTypeSpecRef},
				{"malformed bound", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, FailureInvalid, errInvalidBoundValue, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }},
				{"malformed digits", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:totalDigits value="0"/></xs:restriction></xs:simpleType>`, `value="0"`, "", InvalidTotalDigitsCode, FailureInvalid, errInvalidTotalDigitsValue, func(v XSDVersion) string { return digitFacetSpecRef(v, totalDigitsKind, digitFacetDefinitionRule) }},
				{"outside intrinsic bound", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:maxInclusive value="` + boundedLocalAboveMaximum(family) + `"/></xs:restriction></xs:simpleType>`, `value="` + boundedLocalAboveMaximum(family) + `"`, "", InvalidBoundRestrictionCode, FailureInvalid, errInvalidBoundRestriction, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }},
				{"outside intrinsic enumeration", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:enumeration value="` + boundedLocalAboveMaximum(family) + `"/></xs:restriction></xs:simpleType>`, `<xs:enumeration`, "", InvalidEnumerationRestrictionCode, FailureInvalid, errInvalidEnumerationRestriction, func(v XSDVersion) string { return enumerationSpecRef(v, enumerationRestrictionRule) }},
				{"duplicate use", `<xs:attribute name="a" type="xs:` + family.name + `"/><xs:attribute name="a" type="xs:` + family.name + `" use="required"/>`, "", `<xs:attribute name="a" type="xs:` + family.name + `" use="required"`, `<xs:attribute name="a" type="xs:` + family.name + `"/>`, diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box">`+tc.use+`</xs:complexType>`+tc.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if tc.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, tc.related)}
					}
					if tc.name == "cycle" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`), elementReferenceTestAttributeLoc(t, root, `base="r:Second"`), elementReferenceTestAttributeLoc(t, root, `base="r:First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`)}
					}
					assertNCNameDiagnostic(t, schema, err, tc.class, tc.code, elementReferenceTestAttributeLoc(t, root, tc.primary), related, tc.spec(profile.version), tc.cause)
				})
			}
			t.Run(profile.name+"/"+family.name+"/invisible", func(t *testing.T) {
				root := boundedLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Box"><xs:attribute name="a" type="o:Hidden"/></xs:complexType>`)
				fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`}, "other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:schema>`}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
			t.Run(profile.name+"/"+family.name+"/duplicate candidates", func(t *testing.T) {
				root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:attribute name="a" type="r:T"/></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType><xs:simpleType name="T" id="second"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				first := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="T"><xs:restriction`)
				second := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="T" id="second"`)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, second, []Loc{first}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
				primary := elementReferenceTestAttributeLoc(t, root, `type="r:T"`)
				resolverErr := newSchemaSimpleTypeDiagnostic(diagnosticSchemaSimpleTypeAmbiguousCode, primary, "ambiguous simple type", []Loc{first, second}, profile.version, errSchemaSimpleTypeBaseAmbiguous)
				reframed := reframeSchemaAttributeReferenceError(&schemaAttributeInput{declaredType: mustTestQName(t, "urn:bounded-local", "T"), typeLoc: primary}, resolverErr, profile.version)
				d := requireDiagnostic(t, reframed)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeAmbiguousCode || d.Loc() != primary || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{first, second}) || !errors.Is(reframed, errSchemaAttributeTypeAmbiguous) || !errors.Is(reframed, errSchemaSimpleTypeBaseAmbiguous) {
					t.Fatalf("ambiguous reframe = %v related=%v", reframed, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Policy controls datatype lexical behavior; schema version is a label.
func TestBoundedLocalAttributeEditionAndResolver(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			for _, family := range boundedLocalFamilies() {
				t.Run(profile.name+"/"+label+"/"+family.name, func(t *testing.T) {
					root := boundedLocalRoot(label, `<xs:complexType name="Box"><xs:attribute name="a" type="xs:`+family.name+`"/></xs:complexType>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))
					use, ok := requireTestComplexTypeDefinition(t, found[0], "Box").AttributeUses()[0].(LocalAttributeUse)
					if !ok {
						t.Fatal("local use missing")
					}
					ref, ok := use.TypeReference()
					if !ok {
						t.Fatal("type reference missing")
					}
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
				})
			}
		}
	}
	root := boundedLocalRoot("1.1", `<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Box"><xs:attribute name="a" type="xs:byte"/></xs:complexType>`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("bounded local include unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

//nolint:gocognit // Exact >MaxInt64 bounds, digits, and enumeration share one observable fixture.
func TestUnsignedLongLocalAttributeKeepsArbitraryPrecision(t *testing.T) {
	const minimum = "9223372036854775808"
	const maximum = "18446744073709551614"
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:attribute name="large" type="r:Large"/></xs:complexType><xs:simpleType name="Large"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="`+minimum+`"/><xs:maxInclusive value="`+maximum+`"/><xs:totalDigits value="20"/><xs:enumeration value="`+minimum+`"/></xs:restriction></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))
			use, ok := requireTestComplexTypeDefinition(t, owner[0], "Box").AttributeUses()[0].(LocalAttributeUse)
			if !ok {
				t.Fatal("local use missing")
			}
			ref, ok := use.TypeReference()
			if !ok {
				t.Fatal("named type reference missing")
			}
			assertAllIntegerBounds(t, ref, profile.version, minimum, maximum, elementReferenceTestAttributeLoc(t, root, `value="`+minimum+`"`), elementReferenceTestAttributeLoc(t, root, `value="`+maximum+`"`))
			definition := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:bounded-local", "Large"))
			facts, ok := definition[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("named definition missing")
			}
			total, present := facts.DigitFacets().TotalDigits()
			loc, hasLoc := facts.DigitFacets().TotalDigitsLoc()
			if !present || total.Canonical() != "20" || !hasLoc || loc != elementReferenceTestAttributeLoc(t, root, `value="20"`) {
				t.Fatalf("digits = %s at %s", total.Canonical(), loc)
			}
			values, locations := facts.IntegerEnumerationFacets().Values(), facts.IntegerEnumerationFacets().Locations()
			if len(values) != 1 || values[0].Canonical() != minimum || !reflect.DeepEqual(locations, []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="`+minimum+`"`)}) {
				t.Fatalf("enumeration = %v/%v", values, locations)
			}
		})
	}
}
