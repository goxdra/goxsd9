package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type allUnsignedLongFacetShape struct {
	name       string
	prefix     string
	rootBefore string
	rootAfter  string
	external   string
	source     SourceID
}

func allUnsignedLongFacetShapes() []allUnsignedLongFacetShape {
	return []allUnsignedLongFacetShape{
		{name: "direct", prefix: "r:", rootBefore: `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong">%s</xs:restriction></xs:simpleType>`, source: "root.xsd"},
		{name: "forward", prefix: "r:", rootAfter: `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong">%s</xs:restriction></xs:simpleType>`, source: "root.xsd"},
		{name: "chained", prefix: "r:", rootBefore: `<xs:simpleType name="Base"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:simpleType name="Named"><xs:restriction base="r:Base">%s</xs:restriction></xs:simpleType>`, source: "root.xsd"},
		{name: "include", prefix: "r:", rootBefore: `<xs:include schemaLocation="included.xsd"/>`, external: `<xs:schema xmlns:xs="%s" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong">%%s</xs:restriction></xs:simpleType></xs:schema>`, source: "included.xsd"},
		{name: "import", prefix: "o:", rootBefore: `<xs:import namespace="urn:other" schemaLocation="other.xsd"/>`, external: `<xs:schema xmlns:xs="%s" targetNamespace="urn:other"><xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong">%%s</xs:restriction></xs:simpleType></xs:schema>`, source: "other.xsd"},
		{name: "chameleon", prefix: "r:", rootBefore: `<xs:include schemaLocation="chameleon.xsd"/>`, external: `<xs:schema xmlns:xs="%s"><xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong">%%s</xs:restriction></xs:simpleType></xs:schema>`, source: "chameleon.xsd"},
	}
}

func allUnsignedLongFacetFixture(shape allUnsignedLongFacetShape, facet, memberOccurs, ownerOccurs string) (string, map[string]discoveryFixture, string) {
	before := shape.rootBefore
	after := shape.rootAfter
	fixtures := make(map[string]discoveryFixture)
	facetDocument := ""
	if shape.external != "" {
		facetDocument = fmt.Sprintf(shape.external, testXSDNamespace)
		facetDocument = fmt.Sprintf(facetDocument, facet)
		fixtures[string(shape.source)] = discoveryFixture{id: shape.source, contents: facetDocument}
	}
	if shape.external == "" && before != "" {
		before = fmt.Sprintf(before, facet)
	}
	if shape.external == "" && after != "" {
		after = fmt.Sprintf(after, facet)
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all">` + before + `<xs:complexType name="Record"><xs:all ` + ownerOccurs + `><xs:element name="v" type="` + shape.prefix + `Named" ` + memberOccurs + `/></xs:all></xs:complexType>` + after + `</xs:schema>`
	if facetDocument == "" {
		facetDocument = root
	}
	if shape.name == "include" {
		fixtures["root.xsd"] = discoveryFixture{id: "root.xsd", contents: root}
	}
	return root, fixtures, facetDocument
}

//nolint:gocognit // Each edition, facet, lexical form, and graph shape is an independent public result.
func TestDirectAllUnsignedLongFacetLexicalGraphPolicy(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, facet := range []struct {
			name        string
			bound       BoundKind
			enumeration bool
		}{{"minInclusive", BoundMinInclusive, false}, {"maxInclusive", BoundMaxInclusive, false}, {"enumeration", 0, true}} {
			for _, lexical := range []string{"+0", "-0", "+7"} {
				for _, shape := range allUnsignedLongFacetShapes() {
					t.Run(string(profile.policy)+"/"+shape.name+"/"+facet.name+"/"+lexical, func(t *testing.T) {
						facetXML := `<xs:` + facet.name + ` value="` + lexical + `"/>`
						root, fixtures, facetDocument := allUnsignedLongFacetFixture(shape, facetXML, "", "")
						schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
						facetLoc := allParticleTestTokenLoc(t, shape.source, facetDocument, `value="`+lexical+`"`, 1)
						if facet.enumeration {
							facetLoc = allParticleTestTokenLoc(t, shape.source, facetDocument, `<xs:enumeration`, 1)
						}
						if profile.policy == Strict10 {
							assertZeroSchema(t, schema)
							d := requireDiagnostic(t, err)
							code := InvalidBoundCode
							spec := boundSpecRef(profile.version, facet.bound, boundDefinitionRule)
							cause := errInvalidBoundValue
							if facet.enumeration {
								code = InvalidEnumerationCode
								spec = enumerationSpecRef(profile.version, enumerationDefinitionRule)
								cause = errInvalidEnumerationValue
							}
							if d.Class() != FailureInvalid || d.Code() != code || d.Loc() != facetLoc || d.SpecRef() != spec || len(d.Related()) != 0 || !errors.Is(err, cause) || !errors.Is(err, errSchemaUnsignedLong10FacetLexical) {
								t.Fatalf("facet diagnostic = %s/%v", d, err)
							}
							inner := requireNestedDiagnostic(t, d)
							if inner.Class() != FailureInvalid || inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != allParticleTestTokenLoc(t, shape.source, facetDocument, `value="`+lexical+`"`, 1) || inner.SpecRef() != "xsd10-datatypes#unsignedLong-lexical-representation" || len(inner.Related()) != 0 {
								t.Fatalf("lexical cause = %s", inner)
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						member, ok := directAllFromSchema(t, schema).Members()[0].(ElementParticle)
						if !ok || member.Name().Local() != "v" || member.DeclaredType() != mustTestQName(t, map[bool]string{true: "urn:other", false: "urn:all"}[shape.name == "import"], "Named") {
							t.Fatalf("member = %#v", member)
						}
						reference, ok := member.TypeReference()
						id, hasID := reference.ComponentID()
						if !ok || !reference.IsNamed() || !hasID || id.Source() != shape.source || reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="`+shape.prefix+`Named"`, 1) {
							t.Fatalf("reference = %#v/%v", reference, id)
						}
						expected := "0"
						if lexical == "+7" {
							expected = "7"
						}
						if facet.enumeration {
							definition, hasDefinition := schema.FindKind(ComponentKindSimpleTypeDefinition, reference.Name())[0].SimpleTypeDefinition()
							values, locs := definition.IntegerEnumerationFacets().Values(), definition.IntegerEnumerationFacets().Locations()
							if !hasDefinition || len(values) != 1 || values[0].Canonical() != expected || !reflect.DeepEqual(locs, []Loc{facetLoc}) {
								t.Fatalf("enumeration = %v/%v", values, locs)
							}
							return
						}
						bounds, ok := reference.IntegerBounds()
						if !ok {
							t.Fatal("missing integer bounds")
						}
						var actual IntegerBoundFacet
						if facet.name == "maxInclusive" {
							actual, _ = bounds.MaxInclusiveFacet()
						}
						if facet.name == "minInclusive" {
							actual, _ = bounds.MinInclusiveFacet()
						}
						if actual.Value().Canonical() != expected || actual.Loc() != facetLoc || actual.Version() != profile.version {
							t.Fatalf("bound = %s at %s", actual.Value().Canonical(), actual.Loc())
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Exclusive bounds exercise the same lexical exit and the adjacent semantic exit.
func TestDirectAllUnsignedLongExclusiveFacetExits(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, facet := range []struct {
			name string
			kind BoundKind
		}{{"minExclusive", BoundMinExclusive}, {"maxExclusive", BoundMaxExclusive}} {
			for _, lexical := range []string{"+0", "-0", "+7"} {
				t.Run(string(profile.policy)+"/"+facet.name+"/"+lexical, func(t *testing.T) {
					root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong"><xs:`+facet.name+` value="`+lexical+`"/></xs:restriction></xs:simpleType>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					loc := allParticleTestTokenLoc(t, "root.xsd", root, `value="`+lexical+`"`, 1)
					if profile.policy == Strict10 {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != FailureInvalid || d.Code() != InvalidBoundCode || d.Loc() != loc || d.SpecRef() != boundSpecRef(profile.version, facet.kind, boundDefinitionRule) || len(d.Related()) != 0 || !errors.Is(err, errInvalidBoundValue) || !errors.Is(err, errSchemaUnsignedLong10FacetLexical) {
							t.Fatalf("exclusive lexical = %s/%v", d, err)
						}
						inner := requireNestedDiagnostic(t, d)
						if inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != loc || inner.SpecRef() != "xsd10-datatypes#unsignedLong-lexical-representation" {
							t.Fatalf("inner lexical = %s", inner)
						}
						return
					}
					if facet.name == "maxExclusive" && lexical != "+7" {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != FailureInvalid || d.Code() != InvalidBoundCombinationCode || d.Loc() != loc || d.SpecRef() != boundCombinationSpecRef(profile.version, BoundMinInclusive, BoundMaxExclusive) || len(d.Related()) != 0 || !errors.Is(err, errInvalidBoundCombination) {
							t.Fatalf("exclusive range = %s/%v", d, err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					member, isElement := directAllFromSchema(t, schema).Members()[0].(ElementParticle)
					if !isElement {
						t.Fatal("all member is not an element declaration")
					}
					reference, _ := member.TypeReference()
					bounds, _ := reference.IntegerBounds()
					if facet.name == "minExclusive" {
						lower, ok := bounds.MinExclusiveFacet()
						if !ok || lower.Loc() != loc {
							t.Fatalf("lower = %v", lower)
						}
						return
					}
					upper, ok := bounds.MaxExclusiveFacet()
					if !ok || upper.Value().Canonical() != "7" || upper.Loc() != loc {
						t.Fatalf("upper = %v", upper)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Omission and inherited-base cases verify the same restriction boundary.
func TestDirectAllUnsignedLongFacetBeforeOmissionAndInheritance(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, owner, defs, marker string
			code, spec                        string
			cause                             error
		}{
			{"member zero signed", `minOccurs="0" maxOccurs="0"`, "", `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="+0"/></xs:restriction></xs:simpleType>`, `value="+0"`, InvalidBoundCode, boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule), errSchemaUnsignedLong10FacetLexical},
			{"inherited signed", `minOccurs="0" maxOccurs="0"`, "", `<xs:simpleType name="Base"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="-0"/></xs:restriction></xs:simpleType><xs:simpleType name="Named"><xs:restriction base="r:Base"/></xs:simpleType>`, `value="-0"`, InvalidBoundCode, boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule), errSchemaUnsignedLong10FacetLexical},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all `+test.owner+`><xs:element name="v" type="r:Named" `+test.member+`/></xs:all>`, test.defs)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.policy == Strict10 {
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					loc := allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1)
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != loc || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, test.cause) {
						t.Fatalf("zeroed facet = %s/%v", d, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if got := directAllFromSchema(t, schema).Members(); len(got) != 0 {
					t.Fatalf("zeroed member published %d facts", len(got))
				}
			})
		}
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, facet := range []string{"minInclusive", "maxInclusive", "enumeration"} {
			t.Run(string(policy)+"/owner-zero/"+facet, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong"><xs:`+facet+` value="bad"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				code := InvalidBoundCode
				spec := boundSpecRef(XSDVersion11, BoundMinInclusive, boundDefinitionRule)
				loc := allParticleTestTokenLoc(t, "root.xsd", root, `value="bad"`, 1)
				cause := errInvalidBoundValue
				if facet == "maxInclusive" {
					spec = boundSpecRef(XSDVersion11, BoundMaxInclusive, boundDefinitionRule)
				}
				if facet == "enumeration" {
					code = InvalidEnumerationCode
					spec = enumerationSpecRef(XSDVersion11, enumerationDefinitionRule)
					loc = allParticleTestTokenLoc(t, "root.xsd", root, `<xs:enumeration`, 1)
					cause = errInvalidEnumerationValue
				}
				if d.Class() != FailureInvalid || d.Code() != code || d.Loc() != loc || d.SpecRef() != spec || len(d.Related()) != 0 || !errors.Is(err, cause) {
					t.Fatalf("owner-zero facet = %s/%v", d, err)
				}
			})
		}
	}
}

//nolint:gocognit // Check independent value and lexical exits at the same facet boundary.
func TestDirectAllUnsignedLongFacetInvalidValueAndOrder(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, facets, marker, code, spec string
			cause                            error
			signedOnly                       bool
			enumeration                      bool
		}{
			{"bound exceeds intrinsic", `<xs:maxInclusive value="18446744073709551616"/>`, `value="18446744073709551616"`, InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), errInvalidBoundRestriction, false, false},
			{"enumeration exceeds intrinsic", `<xs:enumeration value="18446744073709551616"/>`, `<xs:enumeration`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), errInvalidEnumerationRestriction, false, true},
			{"malformed before signed", `<xs:minInclusive value="bad"/><xs:maxInclusive value="+0"/>`, `value="bad"`, InvalidBoundCode, boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule), errInvalidBoundValue, true, false},
			{"signed before malformed", `<xs:minInclusive value="+0"/><xs:maxInclusive value="bad"/>`, `value="+0"`, InvalidBoundCode, boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule), errSchemaUnsignedLong10FacetLexical, true, false},
		} {
			if test.signedOnly && profile.policy != Strict10 {
				continue
			}
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:Named" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong">`+test.facets+`</xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1)
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != loc || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("alternate facet exit = %s/%v", d, err)
				}
				if test.enumeration && !errors.Is(err, errBoundValueViolation) {
					t.Fatalf("enumeration lost bounds cause: %v", err)
				}
			})
		}
	}
}
