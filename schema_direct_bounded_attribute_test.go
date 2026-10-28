package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func boundedDirectParticle(kind, occurrence string) string {
	return `<xs:` + kind + occurrence + `><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:integer"/></xs:` + kind + `>`
}

func boundedDirectRoot(version XSDVersion, kind, occurrence, uses, extra string) string {
	return boundedLocalRoot(string(version), `<xs:complexType name="Box">`+boundedDirectParticle(kind, occurrence)+uses+`</xs:complexType><xs:element name="root" type="r:Box"/>`+extra)
}

func boundedDirectGraph(f boundedLocalFamily, version XSDVersion, kind string) (string, map[string]discoveryFixture) {
	root, fixtures := boundedLocalGraph(f, version)
	root = strings.Replace(root, `<xs:complexType name="Box"><xs:attribute`, `<xs:complexType name="Box">`+boundedDirectParticle(kind, ` minOccurs="2" maxOccurs="18446744073709551616"`)+`<xs:attribute`, 1)
	fixture := fixtures["root.xsd"]
	fixture.contents = root
	fixtures["root.xsd"] = fixture
	return root, fixtures
}

//nolint:gocognit,funlen // All admitted families and both authored owners share the same public graph proof.
func TestDirectBoundedIntegerAttributeGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				t.Run(profile.name+"/"+family.name+"/"+kind, func(t *testing.T) {
					root, fixtures := boundedDirectGraph(family, profile.version, kind)
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 4 {
						t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
					}
					for index, source := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
						if schema.Documents()[index].Source() != source {
							t.Fatalf("document %d = %s, want %s", index, schema.Documents()[index].Source(), source)
						}
					}
					var walked []string
					if walkErr := schema.Walk(func(component Component) error {
						walked = append(walked, string(component.ID().Source())+":"+component.Name().Local())
						return nil
					}); walkErr != nil {
						t.Fatal(walkErr)
					}
					if !reflect.DeepEqual(walked, []string{"root.xsd:Box", "root.xsd:Child", "root.xsd:Parent", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}) {
						t.Fatalf("walk = %v", walked)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
					if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`) {
						t.Fatalf("owner Loc = %s", owner.Loc())
					}
					var members []Particle
					switch particle := owner.Particle().(type) {
					case ChoiceParticle:
						if kind != "choice" || particle.Occurrences().String() != "2/18446744073709551616" || particle.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:choice`) {
							t.Fatalf("choice = %v", particle)
						}
						members = particle.Alternatives()
					case SequenceParticle:
						if kind != "sequence" || particle.Occurrences().String() != "2/18446744073709551616" || particle.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:sequence`) {
							t.Fatalf("sequence = %v", particle)
						}
						members = particle.Particles()
					default:
						t.Fatalf("particle = %T", owner.Particle())
					}
					if len(members) != 2 {
						t.Fatalf("members = %v", members)
					}
					first, firstOK := members[0].(ElementParticle)
					second, secondOK := members[1].(ElementParticle)
					if !firstOK || !secondOK || first.Name().Local() != "first" || second.Name().Local() != "second" {
						t.Fatalf("members = %v", members)
					}
					uses := owner.AttributeUses()
					if len(uses) != 6 {
						t.Fatalf("uses = %d", len(uses))
					}
					for index, expected := range []struct{ local, lexical, namespace, source, minimum, maximum, minNeedle, maxNeedle string }{
						{"builtin", "xs:" + family.name, testXSDNamespace, "", family.minimum, family.maximum, "", ""},
						{"forward", "r:Child", "urn:bounded-local", "root.xsd", family.childMinimum, family.childMaximum, `value="` + family.childMinimum + `"`, `value="` + family.childMaximum + `"`},
						{"inherited", "r:Parent", "urn:bounded-local", "root.xsd", family.parentMinimum, family.parentMaximum, `value="` + family.parentMinimum + `"`, `value="` + family.parentMaximum + `"`},
						{"included", "r:Included", "urn:bounded-local", "included.xsd", family.minimum, family.maximum, "", ""},
						{"adopted", "r:Adopted", "urn:bounded-local", "chameleon.xsd", family.minimum, family.maximum, "", ""},
						{"foreign", "o:Foreign", "urn:other", "other.xsd", family.minimum, family.maximum, "", ""},
					} {
						use, ok := uses[index].(LocalAttributeUse)
						if !ok {
							t.Fatalf("use %d = %T", index, uses[index])
						}
						declared := mustTestQName(t, expected.namespace, strings.SplitN(expected.lexical, ":", 2)[1])
						nameNS := "urn:bounded-local"
						if expected.local == "included" {
							nameNS = ""
						}
						if use.Name() != mustTestQName(t, nameNS, expected.local) || use.DeclaredType() != declared || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+expected.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+expected.local+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+expected.lexical+`"`) {
							t.Fatalf("use %d provenance = %v", index, use)
						}
						if index == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`)) || index == 3 && use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
							t.Fatalf("use %d flags = %v", index, use)
						}
						ref, ok := use.TypeReference()
						id, hasID := use.TypeID()
						refID, refHasID := ref.ComponentID()
						if !ok || ref.QName() != declared || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction || id != refID || hasID != refHasID {
							t.Fatalf("type %d = %v", index, ref)
						}
						if index == 0 && (!ref.IsBuiltin() || hasID || !id.IsZero() || ref.VarietyLoc() != use.TypeLoc()) {
							t.Fatalf("builtin type = %v", ref)
						}
						if index > 0 {
							declaration := fixtures[expected.source].contents
							variety := `<xs:restriction base="xs:` + family.name + `"`
							if index == 1 {
								variety = `<xs:restriction base="r:Parent"`
							}
							if !ref.IsNamed() || !hasID || id != componentIDForName(t, schema, declared) || id.Source() != SourceID(expected.source) || ref.VarietyLoc() != mustSchemaTokenLoc(t, SourceID(expected.source), declaration, 1, variety) {
								t.Fatalf("named type %d = %v", index, ref)
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
					members[0] = nil
					if owner.AttributeUses()[0].Name().Local() != "builtin" {
						t.Fatal("use copy mutated owner")
					}
					if kind == "choice" {
						choice, ok := owner.Particle().(ChoiceParticle)
						if !ok {
							t.Fatal("choice owner lost particle")
						}
						fresh, ok := choice.Alternatives()[0].(ElementParticle)
						if !ok || fresh.Name().Local() != "first" {
							t.Fatal("choice copy mutated owner")
						}
					}
					if kind == "sequence" {
						sequence, ok := owner.Particle().(SequenceParticle)
						if !ok {
							t.Fatal("sequence owner lost particle")
						}
						fresh, ok := sequence.Particles()[0].(ElementParticle)
						if !ok || fresh.Name().Local() != "first" {
							t.Fatal("sequence copy mutated owner")
						}
					}
					bounds, _ := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 1)).IntegerBounds()
					for _, bound := range bounds.Bounds() {
						bound.Value().value.SetInt64(0)
					}
					fresh := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 1))
					assertAllIntegerBounds(t, fresh, profile.version, family.childMinimum, family.childMaximum, elementReferenceTestAttributeLoc(t, root, `value="`+family.childMinimum+`"`), elementReferenceTestAttributeLoc(t, root, `value="`+family.childMaximum+`"`))
					var again []string
					if walkErr := schema.Walk(func(component Component) error {
						again = append(again, string(component.ID().Source())+":"+component.Name().Local())
						return nil
					}); walkErr != nil {
						t.Fatal(walkErr)
					}
					if !reflect.DeepEqual(walked, again) {
						t.Fatalf("walk changed: %v", again)
					}
					rebuilt, rebuildErr := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if rebuildErr != nil || !reflect.DeepEqual(schema.Components(), rebuilt.Components()) {
						t.Fatalf("rebuild changed components: %v", rebuildErr)
					}
				})
			}
		}
	}
}

//nolint:gocognit // The two consumers independently refuse every newly queryable family and owner.
func TestDirectBoundedIntegerAttributeConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				for _, shape := range []string{"builtin", "named"} {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/"+shape, func(t *testing.T) {
						lexical, extra := "xs:"+family.name, ""
						if shape == "named" {
							lexical = "r:T"
							extra = `<xs:simpleType name="T"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
						}
						root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="`+lexical+`"/>`, extra)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						if err != nil {
							t.Fatal(err)
						}
						useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
						ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
						rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
						instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:bounded-local" a="2"/>`)))
						vd := requireDiagnostic(t, instanceErr)
						wantRelated := []Loc{rootLoc, ownerLoc, useLoc}
						if kind == "sequence" {
							wantRelated = []Loc{rootLoc, ownerLoc, elementReferenceTestAttributeLoc(t, root, `<xs:sequence`), useLoc}
						}
						if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(instanceErr, errInstanceAttributes) {
							t.Fatalf("ValidateInstance = %v related=%v, want %v", instanceErr, vd.Related(), wantRelated)
						}
						output, generationErr := GenerateGo(schema, "generated")
						gd := requireDiagnostic(t, generationErr)
						if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
							t.Fatalf("GenerateGo = %v/%v", output, generationErr)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Each excluded form is tested at both newly admitted direct owners.
func TestDirectBoundedIntegerAttributeExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				for _, shape := range boundedLocalUseShapes(family)[2:] {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/"+shape.name, func(t *testing.T) {
						root := boundedDirectRoot(profile.version, kind, "", shape.body, shape.extra)
						assertBoundedLocalExcluded(t, root, shape, profile)
					})
				}
				for _, refShape := range []struct{ name, declaration string }{
					{"builtin target", `<xs:attribute name="global" type="xs:` + family.name + `"/>`},
					{"named target", `<xs:attribute name="global" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`},
					{"inline target", `<xs:attribute name="global"><xs:simpleType><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`},
				} {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/ref/"+refShape.name, func(t *testing.T) {
						root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute ref="r:global"/>`, refShape.declaration)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:global"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="global"`)}, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported)
					})
				}
				for _, valueKind := range []string{"default", "fixed"} {
					for _, shape := range []string{"direct", "named", "inline", "ref"} {
						t.Run(profile.name+"/"+family.name+"/"+kind+"/"+shape+"/"+valueKind, func(t *testing.T) {
							use, extra := boundedLocalValueUse(family, shape, valueKind)
							root := boundedDirectRoot(profile.version, kind, "", use, extra)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, valueKind+`="2"`), nil, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported)
						})
					}
				}
				for _, variety := range []struct {
					name, use, extra, primary string
					cause                     error
				}{
					{"named list", `<xs:attribute name="a" type="r:T"/>`, `<xs:simpleType name="T"><xs:list itemType="xs:` + family.name + `"/></xs:simpleType>`, `type="r:T"`, errSchemaAttributeUseUnsupported},
					{"named union", `<xs:attribute name="a" type="r:T"/>`, `<xs:simpleType name="T"><xs:union memberTypes="xs:` + family.name + `"/></xs:simpleType>`, `type="r:T"`, errSchemaAttributeUseUnsupported},
					{"inline atomic", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, errSchemaAttributeTypeUnsupported},
					{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, errSchemaAttributeTypeUnsupported},
					{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, errSchemaAttributeTypeUnsupported},
				} {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/"+variety.name, func(t *testing.T) {
						root := boundedDirectRoot(profile.version, kind, "", variety.use, variety.extra)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, variety.primary), nil, schemaAttributeTypeSpecRef(profile.version), variety.cause)
					})
				}
				root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="r:Patterned"/>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:`+family.name+`"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:pattern`), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
			}
		}
	}
}

//nolint:gocognit // Model, type, and use failures prove their relative phase order.
func TestDirectBoundedIntegerAttributeBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				for _, tc := range []struct {
					name, use, extra, primary, related, code string
					class                                    FailureClass
					cause                                    error
					spec                                     func(XSDVersion) string
				}{
					{"unresolved", `<xs:attribute name="a" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
					{"prohibited unresolved", `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
					{"wrong kind", `<xs:attribute name="a" type="r:Wrong"/>`, `<xs:complexType name="Wrong"/>`, `type="r:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef},
					{"unresolved base", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="r:Missing"/></xs:simpleType>`, `base="r:Missing"`, "", diagnosticSchemaSimpleTypeUnresolvedCode, FailureInvalid, errSchemaSimpleTypeBaseUnresolved, schemaSimpleTypeSpecRef},
					{"wrong kind base", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:complexType name="Wrong"/><xs:simpleType name="Bad"><xs:restriction base="r:Wrong"/></xs:simpleType>`, `base="r:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaSimpleTypeWrongKindCode, FailureInvalid, errSchemaSimpleTypeBaseWrongKind, schemaSimpleTypeSpecRef},
					{"cycle", `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, "", diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, errSchemaSimpleTypeBaseCycle, schemaAttributeTypeSpecRef},
					{"malformed bound", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, FailureInvalid, errInvalidBoundValue, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }},
					{"malformed digits", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:totalDigits value="0"/></xs:restriction></xs:simpleType>`, `value="0"`, "", InvalidTotalDigitsCode, FailureInvalid, errInvalidTotalDigitsValue, func(v XSDVersion) string { return digitFacetSpecRef(v, totalDigitsKind, digitFacetDefinitionRule) }},
					{"outside intrinsic", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:maxInclusive value="` + boundedLocalAboveMaximum(family) + `"/></xs:restriction></xs:simpleType>`, `value="` + boundedLocalAboveMaximum(family) + `"`, "", InvalidBoundRestrictionCode, FailureInvalid, errInvalidBoundRestriction, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }},
					{"outside enumeration", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:enumeration value="` + boundedLocalAboveMaximum(family) + `"/></xs:restriction></xs:simpleType>`, `<xs:enumeration`, "", InvalidEnumerationRestrictionCode, FailureInvalid, errInvalidEnumerationRestriction, func(v XSDVersion) string { return enumerationSpecRef(v, enumerationRestrictionRule) }},
					{"duplicate use", `<xs:attribute name="a" type="xs:` + family.name + `"/><xs:attribute name="a" type="xs:` + family.name + `" use="required"/>`, "", `<xs:attribute name="a" type="xs:` + family.name + `" use="required"`, `<xs:attribute name="a" type="xs:` + family.name + `"/>`, diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef},
				} {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/"+tc.name, func(t *testing.T) {
						root := boundedDirectRoot(profile.version, kind, "", tc.use, tc.extra)
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
			}
		}
	}
}

//nolint:gocognit // Both authored owner shapes and inert version labels must share each omission assertion.
func TestDirectBoundedIntegerAttributeOmissionAndPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				for _, label := range []XSDVersion{XSDVersion10, XSDVersion11} {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/"+string(label), func(t *testing.T) {
						root := boundedDirectRoot(label, kind, ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="kept" type="xs:`+family.name+`"/><xs:attribute name="gone" type="xs:`+family.name+`" use="prohibited"/>`, "")
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						if err != nil {
							t.Fatal(err)
						}
						owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
						if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "kept" {
							t.Fatalf("0/0 owner = %T/%v", owner.Particle(), owner.AttributeUses())
						}
						use := requireBoundedAllUse(t, owner, 0)
						ref := requireBoundedAllReference(t, use)
						if use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="kept"`) || ref.Loc() != use.TypeLoc() {
							t.Fatalf("kept use = %v", use)
						}
						assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
					})
				}
			}
		}
	}
}

//nolint:gocognit // The three lexical forms must be compared across all policies at one boundary.
func TestDirectBoundedIntegerUnsignedLongLexicalPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"choice", "sequence"} {
			for _, lexical := range []string{"+1", "+0", "-0"} {
				root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="r:SignedZero"/>`, `<xs:simpleType name="SignedZero"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="`+lexical+`"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.policy == Strict10 {
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, InvalidBoundCode, elementReferenceTestAttributeLoc(t, root, `value="`+lexical+`"`), nil, boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule), errSchemaUnsignedLong10FacetLexical)
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
				minimum := "0"
				if lexical == "+1" {
					minimum = "1"
				}
				assertAllIntegerBounds(t, requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 0)), profile.version, minimum, "18446744073709551615", elementReferenceTestAttributeLoc(t, root, `value="`+lexical+`"`), Loc{})
			}
		}
	}
}

func TestDirectBoundedIntegerResolverCause(t *testing.T) {
	root := boundedLocalRoot("1.1", `<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Box">`+boundedDirectParticle("choice", "")+`<xs:attribute name="a" type="xs:byte"/></xs:complexType>`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("bounded direct include unavailable")
	schema, parseErr := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, parseErr, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

//nolint:gocognit // The named inherited restriction retains exclusive limits and copied facet provenance.
func TestDirectBoundedIntegerInheritedExclusiveFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				minimum := "-3"
				if family.name == "unsignedLong" {
					minimum = "1"
				}
				extra := `<xs:simpleType name="Parent"><xs:restriction base="xs:` + family.name + `"><xs:minInclusive value="` + family.parentMinimum + `"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="r:Parent"><xs:minExclusive value="` + minimum + `"/><xs:maxExclusive value="4"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType>`
				root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="r:Child"/>`, extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
				ref := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 0))
				bounds, ok := ref.IntegerBounds()
				ordered := bounds.Bounds()
				if !ok || bounds.Version() != profile.version || len(ordered) != 2 || ordered[0].Kind() != BoundMinExclusive || ordered[0].Value().Canonical() != minimum || ordered[0].Loc() != elementReferenceTestAttributeLoc(t, root, `value="`+minimum+`"`) || ordered[1].Kind() != BoundMaxExclusive || ordered[1].Value().Canonical() != "4" || ordered[1].Loc() != elementReferenceTestAttributeLoc(t, root, `value="4"`) {
					t.Fatalf("exclusive bounds = %v", ordered)
				}
				definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:bounded-local", "Child"))[0].SimpleTypeDefinition()
				if !ok {
					t.Fatal("Child definition missing")
				}
				total, hasTotal := definition.DigitFacets().TotalDigits()
				digitLoc, hasLoc := definition.DigitFacets().TotalDigitsLoc()
				values, locations := definition.IntegerEnumerationFacets().Values(), definition.IntegerEnumerationFacets().Locations()
				if !hasTotal || total.Canonical() != "3" || !hasLoc || digitLoc != elementReferenceTestAttributeLoc(t, root, `value="3"`) || len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locations, []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="2"`)}) {
					t.Fatalf("copied facets = %v/%v", total, values)
				}
				ordered[0].Value().value.SetInt64(0)
				values[0].value.SetInt64(0)
				locations[0] = Loc{}
				fresh, _ := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 0)).IntegerBounds()
				if fresh.Bounds()[0].Value().Canonical() != minimum || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "2" || definition.IntegerEnumerationFacets().Locations()[0] != elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="2"`) {
					t.Fatal("caller mutated inherited facts")
				}
			}
		}
	}
}

func TestDirectBoundedIntegerUnsignedLongUpperPrecision(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"choice", "sequence"} {
			root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="large" type="r:Large"/>`, `<xs:simpleType name="Large"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="9223372036854775808"/><xs:maxInclusive value="18446744073709551615"/><xs:totalDigits value="20"/><xs:enumeration value="18446744073709551615"/></xs:restriction></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
			ref := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 0))
			assertAllIntegerBounds(t, ref, profile.version, "9223372036854775808", "18446744073709551615", elementReferenceTestAttributeLoc(t, root, `value="9223372036854775808"`), elementReferenceTestAttributeLoc(t, root, `value="18446744073709551615"`))
			definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:bounded-local", "Large"))[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("Large definition missing")
			}
			if got := definition.IntegerEnumerationFacets().Values(); len(got) != 1 || got[0].Canonical() != "18446744073709551615" {
				t.Fatalf("exact enumeration = %v", got)
			}
		}
	}
}

//nolint:gocognit // Graph visibility, duplicate declarations, and model-before-use errors are separate exits.
func TestDirectBoundedIntegerPhaseAndVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"choice", "sequence"} {
			for _, tc := range []struct {
				name, root, primary, related, code, spec string
				class                                    FailureClass
				cause                                    error
			}{
				{"model before use", boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="r:Missing"/>`, ""), `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"model occurrence", boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:`+kind+` minOccurs="bad"><xs:element name="first" type="xs:integer"/></xs:`+kind+`><xs:attribute name="a" type="r:Missing"/></xs:complexType>`), `minOccurs="bad"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil},
				{"omitted model ref", boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:`+kind+` minOccurs="0" maxOccurs="0"><xs:element ref="r:Missing"/></xs:`+kind+`><xs:attribute name="a" type="xs:byte"/></xs:complexType>`), `ref="r:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), FailureInvalid, errSchemaElementReferenceUnresolved},
				{"nested group unsupported", boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:`+kind+`><xs:group ref="r:Fields"/></xs:`+kind+`><xs:attribute name="a" type="r:Missing"/></xs:complexType><xs:group name="Fields"><xs:choice/></xs:group>`), `ref="r:Fields"`, "", UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, ErrUnsupported},
				{"omitted prohibited type", boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:`+kind+` minOccurs="0" maxOccurs="0"><xs:element name="first" type="xs:integer"/></xs:`+kind+`><xs:attribute name="gone" type="r:Missing" use="prohibited"/></xs:complexType>`), `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"duplicate global", boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="r:T"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:byte"/></xs:simpleType><xs:simpleType name="T" id="second"><xs:restriction base="xs:byte"/></xs:simpleType>`), `<xs:simpleType name="T" id="second"`, `<xs:simpleType name="T"><xs:restriction`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), FailureInvalid, errSchemaGlobalDeclarationDuplicate},
			} {
				t.Run(profile.name+"/"+kind+"/"+tc.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, tc.root, nil, profile.policy)
					var related []Loc
					if tc.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, tc.root, tc.related)}
					}
					if tc.name == "model occurrence" {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != elementReferenceTestAttributeLoc(t, tc.root, tc.primary) || d.SpecRef() != tc.spec || len(d.Related()) != 0 || d.Unwrap() == nil {
							t.Fatalf("occurrence = %v", err)
						}
						return
					}
					assertNCNameDiagnostic(t, schema, err, tc.class, tc.code, elementReferenceTestAttributeLoc(t, tc.root, tc.primary), related, tc.spec, tc.cause)
				})
			}
			root := boundedLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Box">`+boundedDirectParticle(kind, "")+`<xs:attribute name="a" type="o:Hidden"/></xs:complexType>`)
			fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`}, "other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:byte"/></xs:simpleType></xs:schema>`}}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
		}
	}
}

func TestDirectBoundedIntegerAnonymousOwnersRemainExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"choice", "sequence"} {
				for _, shape := range boundedLocalUseShapes(family) {
					t.Run(profile.name+"/"+family.name+"/"+kind+"/"+shape.name, func(t *testing.T) {
						root := boundedLocalRoot(string(profile.version), `<xs:element name="root"><xs:complexType>`+boundedDirectParticle(kind, "")+shape.body+`</xs:complexType></xs:element>`+shape.extra)
						assertBoundedLocalExcluded(t, root, shape, profile)
					})
				}
			}
		}
	}
}

func TestDirectBoundedIntegerLongControl(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"choice", "sequence"} {
			for _, shape := range []string{"builtin", "named"} {
				lexical, extra := "xs:long", ""
				if shape == "named" {
					lexical, extra = "r:Long", `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`
				}
				root := boundedDirectRoot(profile.version, kind, "", `<xs:attribute name="a" type="`+lexical+`"/>`, extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
				ref := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 0))
				assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
			}
		}
	}
}
