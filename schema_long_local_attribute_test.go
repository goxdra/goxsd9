package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func longLocalRoot(version, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:long-local" xmlns:o="urn:other" targetNamespace="urn:long-local" attributeFormDefault="qualified" version="` + version + `">` + body + `</xs:schema>`
}

func longDirectOwnerShapes() []struct{ name, particle string } {
	return []struct{ name, particle string }{
		{"empty", ""},
		{"choice", `<xs:choice><xs:element name="x" type="xs:integer"/></xs:choice>`},
		{"sequence", `<xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>`},
		{"all", `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`},
	}
}

//nolint:gocognit,funlen // A single graph checks the public order, exact effective facts, and source provenance.
func TestLongLocalAttributeGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range longDirectOwnerShapes() {
			t.Run(profile.name+"/"+ownerShape.name, func(t *testing.T) {
				root := longLocalRoot(string(profile.version), `<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Box">`+ownerShape.particle+`<xs:attribute name="builtin" type="xs:long" use="required"/><xs:attribute name="forward" type="r:Forward"/><xs:attribute name="included" type="r:Included" form="unqualified"/><xs:attribute name="adopted" type="r:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/><xs:attribute name="gone" type="xs:long" use="prohibited"/></xs:complexType><xs:simpleType name="Forward"><xs:restriction base="r:Parent"><xs:minInclusive value="-3"/><xs:maxInclusive value="4"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>`)
				included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:long-local"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
				chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
				fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("nondeterministic graph: %v", err)
				}
				wantWalk := []string{"root.xsd:Box", "root.xsd:Forward", "root.xsd:Parent", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}
				var walk []string
				if err := schema.Walk(func(c Component) error { walk = append(walk, string(c.ID().Source())+":"+c.Name().Local()); return nil }); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(walk, wantWalk) {
					t.Fatalf("walk = %v", walk)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Box"))
				if len(found) != 1 {
					t.Fatalf("Box = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Box")
				switch ownerShape.name {
				case "choice":
					if _, ok := owner.Particle().(ChoiceParticle); !ok {
						t.Fatalf("choice particle = %T", owner.Particle())
					}
				case "sequence":
					if _, ok := owner.Particle().(SequenceParticle); !ok {
						t.Fatalf("sequence particle = %T", owner.Particle())
					}
				case "all":
					if _, ok := owner.Particle().(AllParticle); !ok {
						t.Fatalf("all particle = %T", owner.Particle())
					}
				case "empty":
					if owner.Particle() != nil {
						t.Fatalf("empty particle = %T", owner.Particle())
					}
				}
				uses := owner.AttributeUses()
				if len(uses) != 5 {
					t.Fatalf("uses = %v", uses)
				}
				want := []struct{ local, lexical, ns, source, min, max, minSource, minNeedle, maxSource, maxNeedle string }{
					{"builtin", "xs:long", testXSDNamespace, "", "-9223372036854775808", "9223372036854775807", "", "", "", ""},
					{"forward", "r:Forward", "urn:long-local", "root.xsd", "-3", "4", "root.xsd", `value="-3"`, "root.xsd", `value="4"`},
					{"included", "r:Included", "urn:long-local", "included.xsd", "-9223372036854775808", "9223372036854775807", "", "", "", ""},
					{"adopted", "r:Adopted", "urn:long-local", "chameleon.xsd", "-9223372036854775808", "9223372036854775807", "", "", "", ""},
					{"foreign", "o:Foreign", "urn:other", "other.xsd", "-9223372036854775808", "9223372036854775807", "", "", "", ""},
				}
				for i, w := range want {
					use, ok := uses[i].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use %d = %T", i, uses[i])
					}
					nameNS := "urn:long-local"
					if i == 2 {
						nameNS = ""
					}
					if use.Name() != mustTestQName(t, nameNS, w.local) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+w.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+w.local+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+w.lexical+`"`) || use.DeclaredType() != mustTestQName(t, w.ns, strings.SplitN(w.lexical, ":", 2)[1]) {
						t.Fatalf("use %d provenance = %#v", i, use)
					}
					ref, hasRef := use.TypeReference()
					if !hasRef || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
						t.Fatalf("use %d ref = %#v/%t", i, ref, hasRef)
					}
					varietyLoc := use.TypeLoc()
					switch i {
					case 1:
						varietyLoc = elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="r:Parent"`)
					case 2:
						varietyLoc = mustSchemaTokenLoc(t, "included.xsd", included, 1, `<xs:restriction base="xs:long"`)
					case 3:
						varietyLoc = mustSchemaTokenLoc(t, "chameleon.xsd", chameleon, 1, `<xs:restriction base="xs:long"`)
					case 4:
						varietyLoc = mustSchemaTokenLoc(t, "other.xsd", other, 1, `<xs:restriction base="xs:long"`)
					}
					if ref.VarietyLoc() != varietyLoc {
						t.Fatalf("use %d variety Loc = %s, want %s", i, ref.VarietyLoc(), varietyLoc)
					}
					id, hasID := use.TypeID()
					refID, refHasID := ref.ComponentID()
					if i == 0 {
						if !ref.IsBuiltin() || hasID || refHasID || !id.IsZero() || !refID.IsZero() || use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
							t.Fatalf("builtin identity/use = %#v", use)
						}
					}
					if i > 0 {
						if !ref.IsNamed() || !hasID || !refHasID || id != refID || id.Source() != SourceID(w.source) || id != componentIDForName(t, schema, use.DeclaredType()) {
							t.Fatalf("named identity %d = %v/%t %v/%t", i, id, hasID, refID, refHasID)
						}
					}
					var minLoc, maxLoc Loc
					if w.minNeedle != "" {
						minLoc = mustSchemaTokenLoc(t, SourceID(w.minSource), root, 1, w.minNeedle)
					}
					if w.maxNeedle != "" {
						maxLoc = mustSchemaTokenLoc(t, SourceID(w.maxSource), root, 1, w.maxNeedle)
					}
					assertAllIntegerBounds(t, ref, profile.version, w.min, w.max, minLoc, maxLoc)
					if i == 1 {
						definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, use.DeclaredType())[0].SimpleTypeDefinition()
						if !ok {
							t.Fatal("named definition missing")
						}
						enumeration := definition.IntegerEnumerationFacets()
						values, locations := enumeration.Values(), enumeration.Locations()
						if len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locations, []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="2"`)}) {
							t.Fatalf("enumeration = %v at %v", values, locations)
						}
						digit, hasDigit := definition.DigitFacets().TotalDigits()
						digitLoc, hasDigitLoc := definition.DigitFacets().TotalDigitsLoc()
						if !hasDigit || digit.Canonical() != "3" || !hasDigitLoc || digitLoc != elementReferenceTestAttributeLoc(t, root, `value="3"`) {
							t.Fatalf("inherited digits = %v", digit)
						}
						values[0].value.SetInt64(99)
						locations[0] = Loc{}
						if definition.IntegerEnumerationFacets().Values()[0].Canonical() != "2" {
							t.Fatal("enumeration copy mutated schema")
						}
						bounds, _ := ref.IntegerBounds()
						for _, bound := range bounds.Bounds() {
							bound.Value().value.SetInt64(0)
						}
						assertAllIntegerBounds(t, ref, profile.version, "-3", "4", minLoc, maxLoc)
					}
				}
				uses[0] = nil
				if owner.AttributeUses()[0].Name().Local() != "builtin" {
					t.Fatal("use slice mutated schema")
				}
				unqualified, ok := owner.AttributeUses()[2].(LocalAttributeUse)
				if !ok || unqualified.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
					t.Fatalf("form loc = %s", unqualified.FormLoc())
				}
			})
		}
	}
}

//nolint:gocognit,funlen // One matrix checks excluded forms across owners and policies.
func TestLongLocalAttributeExcludedShapes(t *testing.T) {
	named := `<xs:simpleType name="Named"><xs:restriction base="xs:long"/></xs:simpleType>`
	//nolint:prealloc // Keep the source cases beside their generated owner variants.
	cases := []struct {
		name, body, extra, primary, related string
		spec                                func(XSDVersion) string
		cause                               error
	}{
		{"inline atomic", `<xs:complexType name="Box"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute></xs:complexType>`, ``, `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"named list", `<xs:complexType name="Box"><xs:attribute name="a" type="r:List"/></xs:complexType>`, `<xs:simpleType name="List"><xs:list itemType="xs:long"/></xs:simpleType>`, `type="r:List"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"named union", `<xs:complexType name="Box"><xs:attribute name="a" type="r:Union"/></xs:complexType>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:long"/></xs:simpleType>`, `type="r:Union"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"inline list", `<xs:complexType name="Box"><xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:long"/></xs:simpleType></xs:attribute></xs:complexType>`, ``, `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"inline union", `<xs:complexType name="Box"><xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:long"/></xs:simpleType></xs:attribute></xs:complexType>`, ``, `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"ref builtin", `<xs:complexType name="Box"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a" type="xs:long"/>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref named", `<xs:complexType name="Box"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a" type="r:Named"/>` + named, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref inline", `<xs:complexType name="Box"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref default", `<xs:complexType name="Box"><xs:attribute ref="r:a" default="1"/></xs:complexType>`, `<xs:attribute name="a" type="xs:long"/>`, `default="1"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"ref fixed", `<xs:complexType name="Box"><xs:attribute ref="r:a" fixed="1"/></xs:complexType>`, `<xs:attribute name="a" type="xs:long"/>`, `fixed="1"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
	}
	for _, shape := range []struct{ name, attr, extra, value string }{
		{"direct", `type="xs:long"`, "", "7"}, {"named", `type="r:Named"`, named, "7"}, {"inline", ``, ``, "7"},
	} {
		for _, valueKind := range []string{"default", "fixed"} {
			attr := `<xs:attribute name="a" ` + shape.attr + ` ` + valueKind + `="` + shape.value + `"/>`
			if shape.name == "inline" {
				attr = `<xs:attribute name="a" ` + valueKind + `="` + shape.value + `"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`
			}
			cases = append(cases, struct {
				name, body, extra, primary, related string
				spec                                func(XSDVersion) string
				cause                               error
			}{shape.name + " " + valueKind, `<xs:complexType name="Box">` + attr + `</xs:complexType>`, shape.extra, valueKind + `="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported})
		}
	}
	original := len(cases)
	for _, owner := range []struct{ name, particle string }{
		{"choice", `<xs:choice><xs:element name="x" type="xs:integer"/></xs:choice>`},
		{"sequence", `<xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>`},
		{"all", `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`},
	} {
		for index := range original {
			variant := cases[index]
			variant.name = owner.name + " " + variant.name
			variant.body = strings.Replace(variant.body, `<xs:complexType name="Box">`, `<xs:complexType name="Box">`+owner.particle, 1)
			cases = append(cases, variant)
		}
	}
	for _, owner := range []struct{ name, body, extra string }{
		{"group", `<xs:group ref="r:Fields"/>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`},
		{"simple content", `<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:long"/></xs:extension></xs:simpleContent>`, ""},
	} {
		for _, target := range []struct{ name, lexical, extra string }{{"direct", "xs:long", ""}, {"named", "r:Named", named}} {
			body := owner.body
			if !strings.Contains(body, `<xs:attribute name="a"`) {
				body += `<xs:attribute name="a" type="` + target.lexical + `"/>`
			}
			if target.name == "named" {
				body = strings.Replace(body, `type="xs:long"`, `type="r:Named"`, 1)
			}
			cases = append(cases, struct {
				name, body, extra, primary, related string
				spec                                func(XSDVersion) string
				cause                               error
			}{owner.name + " " + target.name, `<xs:complexType name="Box">` + body + `</xs:complexType>`, owner.extra + target.extra, `type="` + target.lexical + `"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported})
		}
	}
	cases = append(cases,
		struct {
			name, body, extra, primary, related string
			spec                                func(XSDVersion) string
			cause                               error
		}{"inline complex direct", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="xs:long"/></xs:complexType></xs:element>`, ``, `type="xs:long"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		struct {
			name, body, extra, primary, related string
			spec                                func(XSDVersion) string
			cause                               error
		}{"inline complex named", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="r:Named"/></xs:complexType></xs:element>`, named, `type="r:Named"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
	)
	for _, particle := range []struct{ name, xml string }{{"choice", `<xs:choice><xs:element name="x" type="xs:integer"/></xs:choice>`}, {"sequence", `<xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>`}} {
		for _, target := range []struct{ name, lexical, extra string }{{"builtin", "xs:long", ""}, {"named", "r:Named", named}} {
			cases = append(cases, struct {
				name, body, extra, primary, related string
				spec                                func(XSDVersion) string
				cause                               error
			}{"inline " + particle.name + " " + target.name, `<xs:element name="root"><xs:complexType>` + particle.xml + `<xs:attribute name="a" type="` + target.lexical + `"/></xs:complexType></xs:element>`, target.extra, `type="` + target.lexical + `"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported})
		}
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, tc := range cases {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := longLocalRoot(string(profile.version), tc.body+tc.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if tc.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, tc.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, tc.primary), related, tc.spec(profile.version), tc.cause)
			})
		}
	}
}

//nolint:gocognit // One diagnostic matrix checks every admitted owner and policy.
func TestLongLocalAttributeBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range longDirectOwnerShapes() {
			for _, tc := range []struct {
				name, use, extra, primary, related, code string
				class                                    FailureClass
				cause                                    error
				spec                                     func(XSDVersion) string
			}{
				{"unresolved", `<xs:attribute name="a" type="r:Missing"/>`, ``, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
				{"prohibited unresolved", `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, ``, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
				{"wrong kind", `<xs:attribute name="a" type="r:Wrong"/>`, `<xs:complexType name="Wrong"/>`, `type="r:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef},
				{"cycle", `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, "", diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, errSchemaSimpleTypeBaseCycle, schemaAttributeTypeSpecRef},
				{"invalid intrinsic bound", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>`, `value="9223372036854775808"`, "", InvalidBoundRestrictionCode, FailureInvalid, errInvalidBoundRestriction, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }},
				{"malformed bound", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, FailureInvalid, errInvalidBoundValue, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }},
				{"duplicate use", `<xs:attribute name="a" type="xs:long"/><xs:attribute name="a" type="xs:long" use="required"/>`, ``, `<xs:attribute name="a" type="xs:long" use="required"`, `<xs:attribute name="a" type="xs:long"/>`, diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef},
			} {
				t.Run(profile.name+"/"+ownerShape.name+"/"+tc.name, func(t *testing.T) {
					root := longLocalRoot(string(profile.version), `<xs:complexType name="Box">`+ownerShape.particle+tc.use+`</xs:complexType>`+tc.extra)
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
			t.Run(profile.name+"/"+ownerShape.name+"/invisible", func(t *testing.T) {
				root := longLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Box">`+ownerShape.particle+`<xs:attribute name="a" type="o:Hidden"/></xs:complexType>`)
				fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`}, "other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
		}
	}
}

//nolint:gocognit // Composition and both consumers retain separate located failures.
func TestLongLocalAttributeCompositionAndConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, tc := range []struct {
			name, use, primary string
			spec               string
		}{
			{"name ref", `<xs:attribute name="a" ref="r:b" type="xs:long"/>`, `ref="r:b"`, "use"},
			{"type inline", `<xs:attribute name="a" type="xs:long"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, "use"},
			{"invalid use", `<xs:attribute name="a" type="xs:long" use="sometimes"/>`, `use="sometimes"`, "use"},
			{"attribute then model", `<xs:attribute name="a" type="xs:long"/><xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>`, `<xs:sequence`, ""},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := longLocalRoot(string(profile.version), `<xs:complexType name="Box">`+tc.use+`</xs:complexType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("invalid composition admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				spec := ""
				if tc.spec == "use" {
					spec = schemaAttributeUseSpecRef(profile.version)
				}
				if tc.name == "attribute then model" {
					spec = schemaComplexTypeGrammarSpecRef(profile.version)
				}
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, tc.primary) || d.SpecRef() != spec || len(d.Related()) != 0 {
					t.Fatalf("composition = %v related=%v", err, d.Related())
				}
			})
		}
		for _, ownerShape := range longDirectOwnerShapes() {
			for _, kind := range []string{"builtin", "named"} {
				t.Run(profile.name+"/consumers/"+ownerShape.name+"/"+kind, func(t *testing.T) {
					typeName, extra := "xs:long", ""
					if kind == "named" {
						typeName = "r:Named"
						extra = `<xs:simpleType name="Named"><xs:restriction base="xs:long"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>`
					}
					root := longLocalRoot(string(profile.version), `<xs:complexType name="Box">`+ownerShape.particle+`<xs:attribute name="a" type="`+typeName+`"/></xs:complexType><xs:element name="root" type="r:Box"/>`+extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
					elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:long-local" a="1"/>`)))
					vd := requireDiagnostic(t, validationErr)
					validationRelated := []Loc{elementLoc, ownerLoc, useLoc}
					if ownerShape.name == "sequence" {
						validationRelated = []Loc{elementLoc, ownerLoc, elementReferenceTestAttributeLoc(t, root, `<xs:sequence`), useLoc}
					}
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), validationRelated) || !errors.Is(validationErr, errInstanceAttributes) {
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

// The declaration phase rejects duplicate candidates before local type lookup can report ambiguity.
func TestLongLocalAttributeDuplicateCandidateAndAmbiguousReframe(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range longDirectOwnerShapes() {
			t.Run(profile.name+"/"+ownerShape.name, func(t *testing.T) {
				root := longLocalRoot(string(profile.version), `<xs:complexType name="Box">`+ownerShape.particle+`<xs:attribute name="a" type="r:Alias"/></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:long"/></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias" id="second"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias"><xs:restriction`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
				primary := elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`)
				related := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias"><xs:restriction`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias" id="second"`)}
				resolverErr := newSchemaSimpleTypeDiagnostic(diagnosticSchemaSimpleTypeAmbiguousCode, primary, "ambiguous simple type", related, profile.version, errSchemaSimpleTypeBaseAmbiguous)
				reframed := reframeSchemaAttributeReferenceError(&schemaAttributeInput{declaredType: mustTestQName(t, "urn:long-local", "Alias"), typeLoc: primary}, resolverErr, profile.version)
				d := requireDiagnostic(t, reframed)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeAmbiguousCode || d.Loc() != primary || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(reframed, errSchemaAttributeTypeAmbiguous) || !errors.Is(reframed, errSchemaSimpleTypeBaseAmbiguous) {
					t.Fatalf("ambiguous reframe = %v related=%v", reframed, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Policy/label combinations and resolver failure share one boundary.
func TestLongLocalAttributeEditionLabelsAndResolverCause(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/"+label, func(t *testing.T) {
				root := longLocalRoot(label, `<xs:complexType name="Box"><xs:attribute name="a" type="xs:long"/></xs:complexType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Box"))
				use, ok := requireTestComplexTypeDefinition(t, found[0], "Box").AttributeUses()[0].(LocalAttributeUse)
				if !ok {
					t.Fatal("local use missing")
				}
				reference, ok := use.TypeReference()
				if !ok {
					t.Fatal("missing type reference")
				}
				assertAllIntegerBounds(t, reference, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
			})
		}
	}
	root := longLocalRoot("1.1", `<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Box"><xs:attribute name="a" type="xs:long"/></xs:complexType>`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("long local include unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

//nolint:gocognit // The three ordered public uses are checked across policies.
func TestLongLocalAttributeKeepsAdjacentIntegerUse(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := longLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:attribute name="first" type="xs:long"/><xs:attribute name="middle" type="xs:integer"/><xs:attribute name="last" type="xs:long"/></xs:complexType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Box"))
		uses := requireTestComplexTypeDefinition(t, found[0], "Box").AttributeUses()
		if len(uses) != 3 {
			t.Fatalf("uses = %v", uses)
		}
		for index, local := range []string{"first", "middle", "last"} {
			use, ok := uses[index].(LocalAttributeUse)
			if !ok || use.Name().Local() != local {
				t.Fatalf("use %d = %v", index, uses[index])
			}
			ref, ok := use.TypeReference()
			if !ok || !ref.IsBuiltin() {
				t.Fatalf("reference %d = %v", index, ref)
			}
			if index == 1 {
				if ref.Name().Local() != "integer" {
					t.Fatalf("middle reference = %v", ref.Name())
				}
				continue
			}
			assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
		}
	}
}

//nolint:gocognit,funlen // Member and use order, occurrence policy, and omission are separate public facts.
func TestLongDirectAllOrderAndOccurrence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		model := `<xs:all minOccurs="0"><xs:element name="later" type="xs:integer"/><xs:element name="earlier" type="xs:boolean" minOccurs="0"/></xs:all>`
		uses := `<xs:attribute name="first" type="xs:long"/><xs:attribute name="second" type="r:Forward"/>`
		extra := `<xs:simpleType name="Forward"><xs:restriction base="xs:long"><xs:minExclusive value="0"/><xs:maxInclusive value="12"/></xs:restriction></xs:simpleType>`
		root := longLocalRoot(string(profile.version), `<xs:complexType name="Box">`+model+uses+`</xs:complexType>`+extra)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Box"))[0], "Box")
		all, ok := owner.Particle().(AllParticle)
		if !ok || all.Occurrences().String() != "0/1" || len(all.Members()) != 2 {
			t.Fatalf("all facts = %v", owner.Particle())
		}
		firstMember, firstOK := all.Members()[0].(ElementParticle)
		secondMember, secondOK := all.Members()[1].(ElementParticle)
		if !firstOK || !secondOK || firstMember.Name().Local() != "later" || secondMember.Name().Local() != "earlier" || secondMember.Occurrences().String() != "0/1" {
			t.Fatalf("all member facts = %v", all.Members())
		}
		if got := owner.AttributeUses(); len(got) != 2 || got[0].Name().Local() != "first" || got[1].Name().Local() != "second" {
			t.Fatalf("use order = %v", got)
		}
		second, ok := owner.AttributeUses()[1].(LocalAttributeUse)
		if !ok {
			t.Fatalf("second use = %T", owner.AttributeUses()[1])
		}
		ref, _ := second.TypeReference()
		id, hasID := ref.ComponentID()
		if second.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Forward"`) || !ref.IsNamed() || !hasID || id.Source() != "root.xsd" {
			t.Fatalf("forward use = %#v", second)
		}
		bounds, ok := ref.IntegerBounds()
		minimum, hasMin := bounds.MinExclusiveFacet()
		maximum, hasMax := bounds.MaxInclusiveFacet()
		if !ok || !hasMin || !hasMax || minimum.Value().Canonical() != "0" || maximum.Value().Canonical() != "12" || minimum.Loc() != elementReferenceTestAttributeLoc(t, root, `value="0"`) || maximum.Loc() != elementReferenceTestAttributeLoc(t, root, `value="12"`) {
			t.Fatalf("named facets = %v", bounds)
		}
		if profile.policy == Strict10 {
			root = longLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:all><xs:element name="many" type="xs:integer" minOccurs="2" maxOccurs="3"/></xs:all>`+uses+`</xs:complexType>`+extra)
			schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureUnsupported, diagnosticSchemaAllOccurrenceVersionCode, elementReferenceTestAttributeLoc(t, root, `minOccurs="2"`), nil, schemaSyntaxSpecRefForVersion(XSDVersion11), errLanguagePolicyMismatch)
			continue
		}
		root = longLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:all><xs:element name="many" type="xs:integer" minOccurs="2" maxOccurs="3"/></xs:all>`+uses+`</xs:complexType>`+extra)
		schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner = requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Box"))[0], "Box")
		all, ok = owner.Particle().(AllParticle)
		if !ok {
			t.Fatalf("repeated all = %T", owner.Particle())
		}
		if all.Members()[0].Occurrences().String() != "2/3" || len(owner.AttributeUses()) != 2 {
			t.Fatalf("repeated member/use = %v/%v", all.Members(), owner.AttributeUses())
		}
		root = longLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:all minOccurs="0" maxOccurs="0"><xs:element name="gone" type="xs:integer"/></xs:all>`+uses+`</xs:complexType>`+extra)
		schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner = requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Box"))[0], "Box")
		if owner.Particle() != nil || len(owner.AttributeUses()) != 2 || owner.AttributeUses()[1].Name().Local() != "second" {
			t.Fatalf("outer 0/0 = %v/%v", owner.Particle(), owner.AttributeUses())
		}
	}
}

func TestLongDirectAllOtherOwnersRemainUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"builtin", "named"} {
			lexical, extra := "xs:long", ""
			if kind == "named" {
				lexical = "r:Long"
				extra = `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`
			}
			use := `<xs:attribute name="a" type="` + lexical + `"/>`
			for _, shape := range []struct{ name, body, marker, spec string }{
				{"anonymous all", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="x" type="xs:integer"/></xs:all>` + use + `</xs:complexType></xs:element>`, `<xs:all>`, schemaSyntaxSpecRefForVersion(profile.version)},
				{"direct all extension", `<xs:complexType name="Base"/><xs:complexType name="Box"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="x" type="xs:integer"/></xs:all>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:attribute name="a"`, extensionSpecForNormalizedAll(profile.version)},
			} {
				t.Run(profile.name+"/"+kind+"/"+shape.name, func(t *testing.T) {
					root := longLocalRoot(string(profile.version), shape.body+extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, shape.marker), nil, shape.spec, ErrUnsupported)
				})
			}
		}
	}
}

//nolint:gocognit // Each competing particle or type exit is checked under every policy.
func TestLongDirectAllPhasePrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, exit := range []struct {
			name, model, use, extra, marker, related, code, spec string
			class                                                FailureClass
			cause                                                error
		}{
			{"invalid member range before missing use", `<xs:all><xs:element name="bad" type="xs:integer" minOccurs="2" maxOccurs="1"/></xs:all>`, `<xs:attribute name="a" type="r:Missing"/>`, "", `<xs:element name="bad"`, `minOccurs="2"|maxOccurs="1"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
			{"missing member before missing use", `<xs:all><xs:element ref="r:missing"/></xs:all>`, `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), FailureInvalid, errSchemaElementReferenceUnresolved},
			{"wrong member before missing use", `<xs:all><xs:element ref="r:Wrong"/></xs:all>`, `<xs:attribute name="a" type="r:Missing"/>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:long"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.version), FailureInvalid, errSchemaElementReferenceWrongKind},
			{"missing use after omitted member", `<xs:all><xs:element name="gone" type="xs:integer" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"unsupported facet after member", `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:Patterned" use="prohibited"/>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:long"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+exit.name, func(t *testing.T) {
				model := exit.model
				if exit.name == "missing member before missing use" && profile.policy != Strict10 {
					model = strings.Replace(model, `<xs:all>`, `<xs:all minOccurs="0" maxOccurs="0">`, 1)
				}
				root := longLocalRoot(string(profile.version), `<xs:complexType name="Box">`+model+exit.use+`</xs:complexType>`+exit.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if exit.related != "" {
					for _, marker := range strings.Split(exit.related, "|") {
						related = append(related, elementReferenceTestAttributeLoc(t, root, marker))
					}
				}
				assertNCNameDiagnostic(t, schema, err, exit.class, exit.code, elementReferenceTestAttributeLoc(t, root, exit.marker), related, exit.spec, exit.cause)
			})
		}
		t.Run(profile.name+"/malformed outer before missing use", func(t *testing.T) {
			root := longLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:all maxOccurs="maybe"><xs:element name="x" type="xs:integer"/></xs:all><xs:attribute name="a" type="r:Missing"/></xs:complexType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			loc := elementReferenceTestAttributeLoc(t, root, `maxOccurs="maybe"`)
			checkStringAllDiagnostic(t, schema, err, FailureInvalid, invalidSchemaCompositionCode, loc, nil, schemaParticleOccurrenceDatatypeSpecRef(profile.version))
			cause := requireDiagnostic(t, errors.Unwrap(requireDiagnostic(t, err)))
			if cause.Code() != InvalidIntegerLexicalCode || cause.Loc() != loc {
				t.Fatalf("malformed occurrence cause = %v", cause)
			}
		})
	}
}
