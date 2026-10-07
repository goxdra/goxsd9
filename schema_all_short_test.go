package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// The public view must preserve ordered graph and datatype provenance together.
//
//nolint:dupl,gocognit,funlen // Edition-specific short facts parallel other bounded integer categories; keep public boundary assertions local.
func TestDirectAllShortGraphFacts(t *testing.T) {
	for _, profile := range []struct {
		policy               LanguagePolicy
		version              XSDVersion
		rangeText, wantRange string
	}{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all" version="1.0">
<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:simpleType name="Direct"><xs:restriction base="xs:short"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/><xs:enumeration value="7"/></xs:restriction></xs:simpleType>
<xs:complexType name="Record"><xs:all><xs:element name="builtin" type="xs:short"/><xs:element name="direct" type="r:Direct" %s/><xs:element name="forward" type="r:Forward"/><xs:element name="included" type="r:Included"/><xs:element name="imported" type="o:Imported"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element ref="r:reference"/></xs:all></xs:complexType>
<xs:element name="root" type="r:Record"/><xs:element name="reference" type="xs:short"/><xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType></xs:schema>`, testXSDNamespace, profile.rangeText)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:short"/></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:short"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:short"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("unstable graph: %v", err)
			}
			var sources []SourceID
			for _, component := range schema.Components() {
				source := component.ID().Source()
				if len(sources) == 0 || sources[len(sources)-1] != source {
					sources = append(sources, source)
				}
			}
			if want := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}; !reflect.DeepEqual(sources, want) {
				t.Fatalf("source order = %v, want %v", sources, want)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if len(members) != 7 || all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all>`, 1) {
				t.Fatalf("all location/member count = %s/%d", all.Loc(), len(members))
			}
			builtin, ok := members[0].(ElementParticle)
			if !ok || builtin.Name() != mustTestQName(t, "", "builtin") || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "short") || builtin.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="builtin"`, 1) {
				t.Fatalf("builtin = %#v", members[0])
			}
			builtinRef, ok := builtin.TypeReference()
			builtinID, hasBuiltinID := builtin.TypeID()
			refID, hasRefID := builtinRef.ComponentID()
			builtinLoc := allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:short"`, 1)
			if !ok || !builtinRef.IsBuiltin() || builtinRef.Name() != mustTestQName(t, testXSDNamespace, "short") || builtinRef.QName() != builtinRef.Name() || builtinRef.Loc() != builtinLoc || builtinRef.VarietyLoc() != builtinLoc || hasBuiltinID || hasRefID || !builtinID.IsZero() || !refID.IsZero() {
				t.Fatalf("builtin type provenance = %#v/%v/%v", builtinRef, builtinID, refID)
			}
			assertAllIntegerBounds(t, builtinRef, profile.version, "-32768", "32767", Loc{}, Loc{})
			assertAllIntegerNamedGraphMembers(t, schema, members, root, fixtures, profile.version, "short", "-32768", "32767")
			direct, isElement := members[1].(ElementParticle)
			if !isElement {
				t.Fatalf("direct member = %T", members[1])
			}
			if direct.Occurrences().String() != profile.wantRange {
				t.Fatalf("direct range = %s", direct.Occurrences())
			}
			ref, ok := members[6].(ElementReferenceParticle)
			if !ok || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:reference"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "reference"))[0].ID() {
				t.Fatalf("reference member = %#v", members[6])
			}
			definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:all", "Direct"))[0].SimpleTypeDefinition()
			digits := definition.DigitFacets()
			total, hasTotal := digits.TotalDigits()
			totalLoc, hasLoc := digits.TotalDigitsLoc()
			values, locations := definition.IntegerEnumerationFacets().Values(), definition.IntegerEnumerationFacets().Locations()
			if !ok || !hasTotal || total.Canonical() != "3" || !hasLoc || totalLoc != allParticleTestTokenLoc(t, "root.xsd", root, `value="3"`, 1) || len(values) != 1 || values[0].Canonical() != "7" || len(locations) != 1 || locations[0] != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:enumeration value="7"`, 1) {
				t.Fatalf("named facets = %v/%v at %s/%v", total, values, totalLoc, locations)
			}
			values[0].value.SetInt64(99)
			locations[0] = Loc{}
			members[1] = nil
			occurrence := direct.Occurrences().Minimum()
			occurrence.value.SetInt64(99)
			directRef, _ := direct.TypeReference()
			bounds, _ := directRef.IntegerBounds()
			minimum, _ := bounds.MinInclusiveFacet()
			copyOfMinimum := minimum.Value()
			copyOfMinimum.value.SetInt64(99)
			fresh, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !ok || fresh.Occurrences().String() != profile.wantRange || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "7" || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller changed completed schema facts")
			}
			freshRef, _ := fresh.TypeReference()
			assertAllIntegerBounds(t, freshRef, profile.version, "-100", "100", allParticleTestTokenLoc(t, "root.xsd", root, `value="-100"`, 1), allParticleTestTokenLoc(t, "root.xsd", root, `value="100"`, 1))
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Keep omission and excluded-shape diagnostics beside all policy cases.
func TestDirectAllShortOmissionAndExclusions(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy)+" omitted member", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="drop" type="xs:short" minOccurs="0" maxOccurs="0"/><xs:element name="namedDrop" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element name="inline" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:element><xs:element name="keep" type="xs:short"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:short"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("members after 0/0 = %#v", members)
			}
			kept, ok := members[0].(ElementParticle)
			if !ok || kept.Name().Local() != "keep" {
				t.Fatalf("members after 0/0 = %#v", members)
			}
		})
		for _, test := range []allScalarDiagnosticCase{
			{"inline short", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named string", `<xs:element name="v" type="r:Text"/>`, `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Text"`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), 1, FailureUnsupported, errSchemaAllMemberScalar},
			{"named list", `<xs:element name="v" type="r:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:short"/></xs:simpleType>`, `type="r:List"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named union", `<xs:element name="v" type="r:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:short"/></xs:simpleType>`, `type="r:Union"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"inline complex", `<xs:element name="v"><xs:complexType/></xs:element>`, "", `<xs:complexType/>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				checkAllScalarDiagnostic(t, profile.policy, test)
			})
		}
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+" owner zero", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:short"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
			if definition.Particle() != nil {
				t.Fatalf("0/0 owner published %T", definition.Particle())
			}
		})
	}
}

//nolint:dupl,gocognit // Alternate exits at the all-member boundary retain their own diagnostics.
func TestDirectAllShortBoundaryDiagnostics(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, defs, primary, related, code, spec string
			primaryOccurrence                                int
			class                                            FailureClass
			cause                                            error
		}{
			{"malformed type QName", `<xs:element name="v" type="r:bad:Short"/>`, "", `type="r:bad:Short"`, "", invalidSchemaConditionalCode, "", 1, FailureInvalid, nil},
			{"unresolved", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"zero unresolved", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong kind", `<xs:element name="v" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:short"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"invalid short facet before zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:maxInclusive value="32768"/></xs:restriction></xs:simpleType>`, `value="32768"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"malformed short facet before zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:maxInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, boundSpecRef(profile.version, BoundMaxInclusive, boundDefinitionRule), 1, FailureInvalid, errInvalidBoundValue},
			{"invalid inline facet before zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:short"><xs:maxInclusive value="32768"/></xs:restriction></xs:simpleType></xs:element>`, "", `value="32768"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"invalid member occurrence", `<xs:element name="v" type="xs:short" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, FailureInvalid, nil},
			{"duplicate", `<xs:element name="v" type="xs:short"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:short"/></xs:simpleType>`, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"cycle before zero", `<xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, `base="r:Two"`, `base="r:One"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"unresolved element ref", `<xs:element ref="r:Missing"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceUnresolved},
			{"zero unresolved element ref", `<xs:element ref="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceUnresolved},
			{"wrong kind element ref", `<xs:element ref="r:Wrong"/>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:short"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceWrongKind},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.defs)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.primaryOccurrence)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %s/%v; want %s/%s at %s with %s", test.name, diagnostic, err, test.class, test.code, wantLoc, test.spec)
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) || test.cause == nil && test.code != invalidSchemaConditionalCode && diagnostic.Unwrap() == nil {
					t.Fatalf("%s related/cause = %v/%v", test.name, diagnostic.Related(), diagnostic.Unwrap())
				}
			})
		}
		for _, test := range []struct{ name, model, marker, code string }{
			{"repeated short member", `<xs:all><xs:element name="v" type="xs:short" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
			{"zero repeated short member", `<xs:all><xs:element name="v" type="xs:short" minOccurs="0" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
			{"zero all owner", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:short"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
		} {
			if profile.policy != Strict10 {
				continue
			}
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(test.model, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("Strict10 occurrence diagnostic = %s/%v", diagnostic, err)
				}
			})
		}
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, test := range []struct {
			name, member, defs, marker, code, spec string
			cause                                  error
		}{
			{"owner zero unresolved", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(XSDVersion11), errSchemaElementTypeUnresolved},
			{"owner zero bad facet", `<xs:element name="v" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:maxInclusive value="32768"/></xs:restriction></xs:simpleType>`, `value="32768"`, InvalidBoundRestrictionCode, boundSpecRef(XSDVersion11, BoundMaxInclusive, boundRestrictionRule), errInvalidBoundRestriction},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0">`+test.member+`</xs:all>`, test.defs)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("0/0 owner diagnostic = %s/%v", diagnostic, err)
				}
			})
		}
	}
}

//nolint:dupl,gocognit // Each bounded integer category checks its public query and consumer boundaries.
func TestDirectAllShortReferenceTargetAndConsumers(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, defs string
			wantReference      bool
		}{
			{"builtin", `<xs:element name="v" type="xs:short"/>`, "", false},
			{"named", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:short"/></xs:simpleType>`, false},
			{"ref to short", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:short"/>`, true},
			{"ref to named short", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="r:Named"/><xs:simpleType name="Named"><xs:restriction base="xs:short"/></xs:simpleType>`, true},
			{"ref to int target", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:int"/>`, true},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, `<xs:element name="root" type="r:Record"/>`+test.defs)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				all := directAllFromSchema(t, schema)
				members := all.Members()
				if len(members) != 1 {
					t.Fatalf("members = %d", len(members))
				}
				if test.wantReference {
					reference, ok := members[0].(ElementReferenceParticle)
					if !ok || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:target"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "target"))[0].ID() {
						t.Fatalf("reference facts = %#v", members[0])
					}
				}
				assertDirectAllConsumerRejection(t, schema, all, profile.version)
			})
		}
	}
}

func TestDirectAllShortHiddenGraphTypeRemainsUnresolved(t *testing.T) {
	assertAllIntegerHiddenGraphTypeUnresolved(t, "short")
}

func TestDirectAllShortLocalFormAndLexicalOrder(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := allParticleTestRoot(`<xs:all><xs:element name="qualified" form="qualified" type="xs:short"/><xs:element name="local" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:short"/></xs:simpleType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		members := directAllFromSchema(t, schema).Members()
		if len(members) != 2 {
			t.Fatalf("%s members = %d", policy, len(members))
		}
		qualified, firstOK := members[0].(ElementParticle)
		local, secondOK := members[1].(ElementParticle)
		if !firstOK || !secondOK || qualified.Name() != mustTestQName(t, "urn:all", "qualified") || local.Name() != mustTestQName(t, "", "local") {
			t.Fatalf("%s ordered names = %#v", policy, members)
		}
		if qualified.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="qualified"`, 1) || local.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="local"`, 1) {
			t.Fatalf("%s declaration locations = %s/%s", policy, qualified.Loc(), local.Loc())
		}
	}
}

//nolint:dupl // Each category checks its own public exclusion at these owner boundaries.
func TestDirectAllShortOtherOwnerShapesRemainUnsupported(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct{ name, body, primary, spec10, spec11 string }{
			{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="xs:short"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, "xsd10-structures#schema-document", "xsd10-structures#schema-document"},
			{"extension owner", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="xs:short"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends"},
			{"named group", `<xs:group name="Group"><xs:all><xs:element name="v" type="xs:short"/></xs:all></xs:group>`, `<xs:all>`, "xsd10-structures#schema-document", "xsd11-structures#cSchemaDocument"},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := test.spec11
				if profile.version == XSDVersion10 {
					wantSpec = test.spec10
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded short owner diagnostic = %s/%v; want %s, spec %q, related %v", diagnostic, err, allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1), wantSpec, diagnostic.Related())
				}
			})
		}
	}
}
