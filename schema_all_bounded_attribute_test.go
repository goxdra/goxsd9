package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func boundedAllGraph(f boundedLocalFamily, version XSDVersion, memberRange string) (string, map[string]discoveryFixture) {
	root, fixtures := boundedLocalGraph(f, version)
	member := `<xs:all><xs:element name="z" type="xs:integer" ` + memberRange + `/><xs:element name="a" type="xs:integer"/></xs:all>`
	root = strings.Replace(root, `<xs:complexType name="Box"><xs:attribute`, `<xs:complexType name="Box">`+member+`<xs:attribute`, 1)
	root = strings.Replace(root, `</xs:schema>`, `<xs:element name="root" type="r:Box"/></xs:schema>`, 1)
	fixture := fixtures["root.xsd"]
	fixture.contents = root
	fixtures["root.xsd"] = fixture
	return root, fixtures
}

func requireBoundedAllUse(t *testing.T, owner ComplexTypeDefinition, index int) LocalAttributeUse {
	t.Helper()
	uses := owner.AttributeUses()
	if index >= len(uses) {
		t.Fatalf("use %d is missing from %d uses", index, len(uses))
	}
	use, ok := uses[index].(LocalAttributeUse)
	if !ok {
		t.Fatalf("use %d = %T, want local", index, uses[index])
	}
	return use
}

func requireBoundedAllReference(t *testing.T, use LocalAttributeUse) SimpleTypeReference {
	t.Helper()
	ref, ok := use.TypeReference()
	if !ok {
		t.Fatal("local bounded use has no type reference")
	}
	return ref
}

//nolint:gocognit,funlen // The one public graph checks all four numeric kinds and ordered copied use facts.
func TestDirectAllBoundedIntegerAttributeGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				memberRange := `minOccurs="0" maxOccurs="1"`
				wantRange := "0/1"
				if profile.policy != Strict10 {
					memberRange = `minOccurs="2" maxOccurs="18446744073709551616"`
					wantRange = "2/18446744073709551616"
				}
				root, fixtures := boundedAllGraph(family, profile.version, memberRange)
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("repeated graph changed components: %v", err)
				}
				for index, source := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
					if schema.Documents()[index].Source() != source {
						t.Fatalf("document %d = %s, want %s", index, schema.Documents()[index].Source(), source)
					}
				}
				var walked []string
				if err := schema.Walk(func(component Component) error {
					walked = append(walked, string(component.ID().Source())+":"+component.Name().Local())
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(walked, []string{"root.xsd:Box", "root.xsd:Child", "root.xsd:Parent", "root.xsd:root", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}) {
					t.Fatalf("walk order = %v", walked)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))
				owner := requireTestComplexTypeDefinition(t, found[0], "Box")
				if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`) {
					t.Fatalf("owner loc = %s", owner.Loc())
				}
				all, ok := owner.Particle().(AllParticle)
				if !ok || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all>`) {
					t.Fatalf("particle = %#v", owner.Particle())
				}
				members := all.Members()
				if len(members) != 2 {
					t.Fatalf("members = %#v", members)
				}
				firstMember, firstOK := members[0].(ElementParticle)
				secondMember, secondOK := members[1].(ElementParticle)
				if !firstOK || !secondOK || firstMember.Name().Local() != "z" || secondMember.Name().Local() != "a" || firstMember.Occurrences().String() != wantRange {
					t.Fatalf("member order/range = %#v", members)
				}
				uses := owner.AttributeUses()
				if len(uses) != 6 {
					t.Fatalf("uses = %d, want six after prohibited omission", len(uses))
				}
				for index, expected := range []struct{ name, lexical, namespace, source, minimum, maximum, minNeedle, maxNeedle string }{
					{"builtin", "xs:" + family.name, testXSDNamespace, "", family.minimum, family.maximum, "", ""},
					{"forward", "r:Child", "urn:bounded-local", "root.xsd", family.childMinimum, family.childMaximum, `value="` + family.childMinimum + `"`, `value="` + family.childMaximum + `"`},
					{"inherited", "r:Parent", "urn:bounded-local", "root.xsd", family.parentMinimum, family.parentMaximum, `value="` + family.parentMinimum + `"`, `value="` + family.parentMaximum + `"`},
					{"included", "r:Included", "urn:bounded-local", "included.xsd", family.minimum, family.maximum, "", ""},
					{"adopted", "r:Adopted", "urn:bounded-local", "chameleon.xsd", family.minimum, family.maximum, "", ""},
					{"foreign", "o:Foreign", "urn:other", "other.xsd", family.minimum, family.maximum, "", ""},
				} {
					use, hasUse := uses[index].(LocalAttributeUse)
					if !hasUse {
						t.Fatalf("use %d = %T", index, uses[index])
					}
					nameNS := "urn:bounded-local"
					if expected.name == "included" {
						nameNS = ""
					}
					declared := mustTestQName(t, expected.namespace, strings.SplitN(expected.lexical, ":", 2)[1])
					if use.Name() != mustTestQName(t, nameNS, expected.name) || use.DeclaredType() != declared || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+expected.name+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+expected.name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+expected.lexical+`"`) {
						t.Fatalf("use %d provenance = %#v", index, use)
					}
					if index == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`)) || index == 3 && use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
						t.Fatalf("use %d flags = %#v", index, use)
					}
					ref, hasRef := use.TypeReference()
					if !hasRef || ref.QName() != declared || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
						t.Fatalf("use %d type = %#v", index, ref)
					}
					id, hasID := use.TypeID()
					refID, refHasID := ref.ComponentID()
					if index == 0 && (!ref.IsBuiltin() || hasID || refHasID || !id.IsZero() || !refID.IsZero() || ref.VarietyLoc() != use.TypeLoc()) {
						t.Fatalf("builtin identity = %#v", ref)
					}
					if index != 0 {
						variety := `<xs:restriction base="xs:` + family.name + `"`
						if index == 1 {
							variety = `<xs:restriction base="r:Parent"`
						}
						declaration := fixtures[expected.source].contents
						if !ref.IsNamed() || !hasID || !refHasID || id != refID || id != componentIDForName(t, schema, declared) || id.Source() != SourceID(expected.source) || ref.VarietyLoc() != mustSchemaTokenLoc(t, SourceID(expected.source), declaration, 1, variety) {
							t.Fatalf("named identity/provenance %d = %#v", index, ref)
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
				freshMember, ok := all.Members()[0].(ElementParticle)
				if owner.AttributeUses()[0].Name().Local() != "builtin" || !ok || freshMember.Name().Local() != "z" {
					t.Fatal("caller changed ordered owner copies")
				}
				use := requireBoundedAllUse(t, owner, 1)
				ref := requireBoundedAllReference(t, use)
				bounds, hasBounds := ref.IntegerBounds()
				if !hasBounds {
					t.Fatal("named use lost integer bounds")
				}
				for _, bound := range bounds.Bounds() {
					value := bound.Value()
					value.value.SetInt64(0)
				}
				fresh := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 1))
				assertAllIntegerBounds(t, fresh, profile.version, family.childMinimum, family.childMaximum, elementReferenceTestAttributeLoc(t, root, `value="`+family.childMinimum+`"`), elementReferenceTestAttributeLoc(t, root, `value="`+family.childMaximum+`"`))
			})
		}
	}
}

//nolint:gocognit // Each family and type shape independently checks both consumer exits.
func TestDirectAllBoundedIntegerAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, shape := range []string{"direct", "named"} {
				t.Run(profile.name+"/"+family.name+"/"+shape, func(t *testing.T) {
					lexical, extra := "xs:"+family.name, ""
					if shape == "named" {
						lexical = "r:T"
						extra = `<xs:simpleType name="T"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
					}
					root := boundedLocalRoot(string(profile.version), `<xs:complexType name="Box"><xs:all><xs:element name="x" type="xs:integer"/></xs:all><xs:attribute name="a" type="`+lexical+`"/></xs:complexType><xs:element name="root" type="r:Box"/>`+extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
					elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:bounded-local"/>`)))
					vd := requireDiagnostic(t, validationErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, errInstanceAttributes) {
						t.Fatalf("ValidateInstance = %v", validationErr)
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

func boundedAllUseRoot(version XSDVersion, all, uses, extra string) string {
	return boundedLocalRoot(string(version), `<xs:complexType name="Box">`+all+uses+`</xs:complexType>`+extra)
}

//nolint:gocognit // Authored all and prohibited-use omissions share this observable owner.
func TestDirectAllBoundedIntegerOuterOmissionKeepsAttributes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		if profile.policy == Strict10 {
			continue
		}
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root := boundedAllUseRoot(profile.version, `<xs:all minOccurs="0" maxOccurs="0"><xs:element ref="r:global"/></xs:all>`, `<xs:attribute name="kept" type="xs:`+family.name+`"/><xs:attribute name="gone" type="xs:`+family.name+`" use="prohibited"/>`, `<xs:element name="global" type="xs:integer"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))
				owner := requireTestComplexTypeDefinition(t, found[0], "Box")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "kept" {
					t.Fatalf("omitted all owner = %T/%#v", owner.Particle(), owner.AttributeUses())
				}
				use := requireBoundedAllUse(t, owner, 0)
				ref, ok := use.TypeReference()
				if !ok || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="kept"`) || ref.Loc() != elementReferenceTestAttributeLoc(t, root, `type="xs:`+family.name+`"`) {
					t.Fatalf("surviving use = %#v", use)
				}
				assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
			})
		}
	}
}

//nolint:gocognit // Each row is a separate exit at the all-plus-attribute mapping boundary.
func TestDirectAllBoundedIntegerAttributeBoundaryDiagnostics(t *testing.T) {
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
				{"unresolved base", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="r:Missing"/></xs:simpleType>`, `base="r:Missing"`, "", diagnosticSchemaSimpleTypeUnresolvedCode, FailureInvalid, errSchemaSimpleTypeBaseUnresolved, schemaSimpleTypeSpecRef},
				{"wrong kind base", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:complexType name="Wrong"/><xs:simpleType name="Bad"><xs:restriction base="r:Wrong"/></xs:simpleType>`, `base="r:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaSimpleTypeWrongKindCode, FailureInvalid, errSchemaSimpleTypeBaseWrongKind, schemaSimpleTypeSpecRef},
				{"cycle", `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, "", diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, errSchemaSimpleTypeBaseCycle, schemaAttributeTypeSpecRef},
				{"malformed bound", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, FailureInvalid, errInvalidBoundValue, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }},
				{"outside intrinsic", `<xs:attribute name="a" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:maxInclusive value="` + boundedLocalAboveMaximum(family) + `"/></xs:restriction></xs:simpleType>`, `value="` + boundedLocalAboveMaximum(family) + `"`, "", InvalidBoundRestrictionCode, FailureInvalid, errInvalidBoundRestriction, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }},
				{"duplicate use", `<xs:attribute name="a" type="xs:` + family.name + `"/><xs:attribute name="a" type="xs:` + family.name + `" use="required"/>`, "", `<xs:attribute name="a" type="xs:` + family.name + `" use="required"`, `<xs:attribute name="a" type="xs:` + family.name + `"/>`, diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, tc.use, tc.extra)
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

//nolint:gocognit // Value, variety, and reference exclusions each retain their own location.
func TestDirectAllBoundedIntegerAttributeShapeExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, kind := range []string{"default", "fixed"} {
				for _, shape := range []string{"direct", "named", "inline", "ref"} {
					t.Run(profile.name+"/"+family.name+"/"+shape+"/"+kind, func(t *testing.T) {
						use, extra := boundedLocalValueUse(family, shape, kind)
						root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, use, extra)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, kind+`="2"`), nil, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported)
					})
				}
			}
			for _, shape := range []struct{ name, use, extra, primary string }{
				{"named list", `<xs:attribute name="a" type="r:T"/>`, `<xs:simpleType name="T"><xs:list itemType="xs:` + family.name + `"/></xs:simpleType>`, `type="r:T"`},
				{"named union", `<xs:attribute name="a" type="r:T"/>`, `<xs:simpleType name="T"><xs:union memberTypes="xs:` + family.name + `"/></xs:simpleType>`, `type="r:T"`},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`},
			} {
				t.Run(profile.name+"/"+family.name+"/"+shape.name, func(t *testing.T) {
					root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, shape.use, shape.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					cause := errSchemaAttributeUseUnsupported
					if strings.HasPrefix(shape.name, "inline") {
						cause = errSchemaAttributeTypeUnsupported
					}
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, shape.primary), nil, schemaAttributeTypeSpecRef(profile.version), cause)
				})
			}
		}
	}
}

func TestDirectAllBoundedIntegerAttributeBroaderFacetExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:Patterned"/>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:`+family.name+`"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:pattern`), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
			})
		}
	}
}

func TestDirectAllUnsignedLongAttributeLexicalPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, lexical := range []string{`+0`, `-0`} {
			t.Run(profile.name+"/"+lexical, func(t *testing.T) {
				root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:SignedZero"/>`, `<xs:simpleType name="SignedZero"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="`+lexical+`"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.policy == Strict10 {
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, InvalidBoundCode, elementReferenceTestAttributeLoc(t, root, `value="`+lexical+`"`), nil, boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule), errSchemaUnsignedLong10FacetLexical)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
				use := requireBoundedAllUse(t, owner, 0)
				ref := requireBoundedAllReference(t, use)
				assertAllIntegerBounds(t, ref, profile.version, "0", "18446744073709551615", elementReferenceTestAttributeLoc(t, root, `value="`+lexical+`"`), Loc{})
			})
		}
	}
}

//nolint:gocognit // Named restrictions must retain exclusive limits, digits, and exact integer copies.
func TestDirectAllBoundedIntegerAttributeExclusiveFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				minimum := "-3"
				if family.name == "unsignedLong" {
					minimum = "1"
				}
				extra := `<xs:simpleType name="Parent"><xs:restriction base="xs:` + family.name + `"><xs:minInclusive value="` + family.parentMinimum + `"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="r:Parent"><xs:minExclusive value="` + minimum + `"/><xs:maxExclusive value="4"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType>`
				root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:Child"/>`, extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
				use := requireBoundedAllUse(t, owner, 0)
				ref, ok := use.TypeReference()
				if !ok || !ref.IsNamed() || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="r:Parent"`) {
					t.Fatalf("named child = %#v", ref)
				}
				bounds, ok := ref.IntegerBounds()
				if !ok || bounds.Version() != profile.version {
					t.Fatalf("bounds = %#v", bounds)
				}
				ordered := bounds.Bounds()
				if len(ordered) != 2 || ordered[0].Kind() != BoundMinExclusive || ordered[0].Value().Canonical() != minimum || ordered[0].Loc() != elementReferenceTestAttributeLoc(t, root, `value="`+minimum+`"`) || ordered[1].Kind() != BoundMaxExclusive || ordered[1].Value().Canonical() != "4" || ordered[1].Loc() != elementReferenceTestAttributeLoc(t, root, `value="4"`) {
					t.Fatalf("exclusive bounds = %#v", ordered)
				}
				definition := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:bounded-local", "Child"))[0]
				facts, ok := definition.SimpleTypeDefinition()
				if !ok {
					t.Fatal("Child facts missing")
				}
				total, hasTotal := facts.DigitFacets().TotalDigits()
				loc, hasLoc := facts.DigitFacets().TotalDigitsLoc()
				values, locations := facts.IntegerEnumerationFacets().Values(), facts.IntegerEnumerationFacets().Locations()
				if !hasTotal || total.Canonical() != "3" || !hasLoc || loc != elementReferenceTestAttributeLoc(t, root, `value="3"`) || len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locations, []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="2"`)}) {
					t.Fatalf("inherited digit/enumeration facts = %s/%v", total.Canonical(), values)
				}
				ordered[0].value.value.SetInt64(0)
				values[0].value.SetInt64(0)
				locations[0] = Loc{}
				fresh := requireBoundedAllReference(t, requireBoundedAllUse(t, owner, 0))
				freshBounds, hasFreshBounds := fresh.IntegerBounds()
				if !hasFreshBounds {
					t.Fatal("copied named use lost integer bounds")
				}
				if freshBounds.Bounds()[0].Value().Canonical() != minimum || facts.IntegerEnumerationFacets().Values()[0].Canonical() != "2" || facts.IntegerEnumerationFacets().Locations()[0].IsZero() {
					t.Fatal("caller changed copied exclusive/facet facts")
				}
			})
		}
	}
}

func TestDirectAllBoundedIntegerAttributeGraphVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name+"/invisible", func(t *testing.T) {
				root := boundedLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Box"><xs:all><xs:element name="x" type="xs:integer"/></xs:all><xs:attribute name="a" type="o:Hidden"/></xs:complexType>`)
				fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`}, "other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:schema>`}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
			t.Run(profile.name+"/"+family.name+"/duplicate candidate", func(t *testing.T) {
				root := boundedAllUseRoot(profile.version, `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:T"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType><xs:simpleType name="T" id="second"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				first := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="T"><xs:restriction`)
				second := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="T" id="second"`)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, second, []Loc{first}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
			})
		}
	}
}

//nolint:gocognit // Particle and syntax exits must precede a later missing bounded attribute type.
func TestDirectAllBoundedIntegerAttributePrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, tc := range []struct {
				name, all, use, extra, primary, related, code, spec string
				cause                                               error
			}{
				{"malformed use syntax", `<xs:all><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" ref="r:global" type="xs:` + family.name + `"/>`, `<xs:attribute name="global" type="xs:` + family.name + `"/>`, `ref="r:global"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), nil},
				{"malformed occurrence", `<xs:all><xs:element name="x" type="xs:integer" maxOccurs="maybe"/></xs:all>`, `<xs:attribute name="a" type="r:Missing"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), nil},
				{"missing particle ref", `<xs:all><xs:element ref="r:missing" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:attribute name="a" type="r:Missing"/>`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved},
				{"wrong particle ref", `<xs:all><xs:element ref="r:Wrong"/></xs:all>`, `<xs:attribute name="a" type="r:Missing"/>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceWrongKind},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := boundedAllUseRoot(profile.version, tc.all, tc.use, tc.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if tc.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, tc.related)}
					}
					loc := elementReferenceTestAttributeLoc(t, root, tc.primary)
					if tc.cause != nil {
						assertNCNameDiagnostic(t, schema, err, FailureInvalid, tc.code, loc, related, tc.spec, tc.cause)
						return
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != tc.code || d.Loc() != loc || d.SpecRef() != tc.spec || !reflect.DeepEqual(d.Related(), related) {
						t.Fatalf("precedence diagnostic = %v", err)
					}
				})
			}
		}
	}
}

func TestDirectAllBoundedIntegerAttributeEditionAndResolver(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, label := range []string{"1.0", "1.1"} {
				t.Run(profile.name+"/"+family.name+"/"+label, func(t *testing.T) {
					root := boundedLocalRoot(label, `<xs:complexType name="Box"><xs:all><xs:element name="x" type="xs:integer"/></xs:all><xs:attribute name="a" type="xs:`+family.name+`"/></xs:complexType>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
					use := requireBoundedAllUse(t, owner, 0)
					ref := requireBoundedAllReference(t, use)
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
				})
			}
		}
	}
	root := boundedLocalRoot("1.1", `<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Box"><xs:all><xs:element name="x" type="xs:integer"/></xs:all><xs:attribute name="a" type="xs:byte"/></xs:complexType>`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("direct all include unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

func TestDirectAllBoundedIntegerAttributeStrict10ParticleLimits(t *testing.T) {
	for _, family := range boundedLocalFamilies() {
		for _, shape := range []string{"direct", "named"} {
			lexical, extra := "xs:"+family.name, ""
			if shape == "named" {
				lexical = "r:T"
				extra = `<xs:simpleType name="T"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
			}
			for _, tc := range []struct{ name, all, primary, code string }{
				{"member repeat", `<xs:all><xs:element name="x" type="xs:integer" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
				{"outer zero", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="x" type="xs:integer"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
			} {
				t.Run(family.name+"/"+shape+"/"+tc.name, func(t *testing.T) {
					root := boundedAllUseRoot(XSDVersion10, tc.all, `<xs:attribute name="a" type="`+lexical+`"/>`, extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, tc.code, elementReferenceTestAttributeLoc(t, root, tc.primary), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
				})
			}
		}
	}
}

func TestInlineAllBoundedIntegerAttributeOwnerRemainsUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, shape := range boundedLocalUseShapes(family) {
				t.Run(profile.name+"/"+family.name+"/"+shape.name, func(t *testing.T) {
					root := boundedLocalRoot(string(profile.version), `<xs:element name="root"><xs:complexType><xs:all><xs:element name="x" type="xs:integer"/></xs:all>`+shape.body+`</xs:complexType></xs:element>`+shape.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all>`) || len(d.Related()) != 0 || d.SpecRef() != "xsd10-structures#schema-document" || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("inline-all owner diagnostic = %v spec=%q unsupported=%t", err, d.SpecRef(), errors.Is(err, ErrUnsupported))
					}
				})
			}
		}
	}
}

func TestDirectAllBoundedIntegerOuterZeroRetainsDependencyDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		if profile.policy == Strict10 {
			continue
		}
		for _, family := range boundedLocalFamilies() {
			for _, tc := range []struct {
				name, all, use, primary, code, spec string
				cause                               error
			}{
				{"member ref", `<xs:all minOccurs="0" maxOccurs="0"><xs:element ref="r:missing"/></xs:all>`, `<xs:attribute name="a" type="xs:` + family.name + `"/>`, `ref="r:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved},
				{"prohibited type", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="x" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := boundedAllUseRoot(profile.version, tc.all, tc.use, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, tc.code, elementReferenceTestAttributeLoc(t, root, tc.primary), nil, tc.spec, tc.cause)
				})
			}
		}
	}
}
