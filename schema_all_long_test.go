package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:dupl,gocognit,funlen // Bounded integer categories require parallel public fact assertions.
func TestDirectAllLongGraphFacts(t *testing.T) {
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
<xs:simpleType name="Direct"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/><xs:enumeration value="7"/></xs:restriction></xs:simpleType>
<xs:complexType name="Record"><xs:all><xs:element name="builtin" type="xs:long"/><xs:element name="direct" type="r:Direct" %s/><xs:element name="forward" type="r:Forward"/><xs:element name="included" type="r:Included"/><xs:element name="imported" type="o:Imported"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element ref="r:reference"/></xs:all></xs:complexType>
<xs:element name="root" type="r:Record"/><xs:element name="reference" type="xs:long"/><xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType></xs:schema>`, testXSDNamespace, profile.rangeText)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`},
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
			if !ok || builtin.Name() != mustTestQName(t, "", "builtin") || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "long") || builtin.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="builtin"`, 1) {
				t.Fatalf("builtin = %#v", members[0])
			}
			builtinRef, ok := builtin.TypeReference()
			builtinID, hasBuiltinID := builtin.TypeID()
			refID, hasRefID := builtinRef.ComponentID()
			builtinLoc := allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:long"`, 1)
			if !ok || !builtinRef.IsBuiltin() || builtinRef.Name() != mustTestQName(t, testXSDNamespace, "long") || builtinRef.QName() != builtinRef.Name() || builtinRef.Loc() != builtinLoc || builtinRef.VarietyLoc() != builtinLoc || hasBuiltinID || hasRefID || !builtinID.IsZero() || !refID.IsZero() {
				t.Fatalf("builtin type provenance = %#v/%v/%v", builtinRef, builtinID, refID)
			}
			assertAllIntegerBounds(t, builtinRef, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
			assertAllIntegerNamedGraphMembers(t, schema, members, root, fixtures, profile.version, "long", "-9223372036854775808", "9223372036854775807", "-100")
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

func assertAllIntegerBounds(t *testing.T, reference SimpleTypeReference, version XSDVersion, wantMin, wantMax string, minLoc, maxLoc Loc) {
	t.Helper()
	bounds, ok := reference.IntegerBounds()
	if !ok || bounds.Version() != version {
		t.Fatalf("integer bounds/version = %v/%v", bounds, ok)
	}
	minimum, hasMinimum := bounds.MinInclusiveFacet()
	maximum, hasMaximum := bounds.MaxInclusiveFacet()
	if !hasMinimum || !hasMaximum || minimum.Value().Canonical() != wantMin || maximum.Value().Canonical() != wantMax || minimum.Loc() != minLoc || maximum.Loc() != maxLoc || minimum.Version() != version || maximum.Version() != version {
		t.Fatalf("long bounds = %s at %s / %s at %s; want %s at %s / %s at %s", minimum.Value().Canonical(), minimum.Loc(), maximum.Value().Canonical(), maximum.Loc(), wantMin, minLoc, wantMax, maxLoc)
	}
}

func assertAllIntegerNamedGraphMembers(t *testing.T, schema Schema, members []Particle, root string, fixtures map[string]discoveryFixture, version XSDVersion, atomic, intrinsicMin, intrinsicMax, effectiveMin string) {
	t.Helper()
	for index, want := range []struct{ local, namespace, typeName, source, variety string }{
		{"direct", "urn:all", "Direct", "root.xsd", `<xs:restriction base="xs:` + atomic + `"`},
		{"forward", "urn:all", "Forward", "root.xsd", `<xs:restriction base="r:Direct"`},
		{"included", "urn:all", "Included", "included.xsd", `<xs:restriction base="xs:` + atomic + `"`},
		{"imported", "urn:other", "Imported", "other.xsd", `<xs:restriction base="xs:` + atomic + `"`},
		{"chameleon", "urn:all", "Chameleon", "chameleon.xsd", `<xs:restriction base="xs:` + atomic + `"`},
	} {
		member, isElement := members[index+1].(ElementParticle)
		if !isElement || member.Name().Local() != want.local || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+want.local+`"`, 1) {
			t.Fatalf("member %d = %#v", index+1, members[index+1])
		}
		name := mustTestQName(t, want.namespace, want.typeName)
		id := componentIDForName(t, schema, name)
		reference, hasReference := member.TypeReference()
		memberID, hasMemberID := member.TypeID()
		referenceID, hasReferenceID := reference.ComponentID()
		prefix := "r:"
		if want.namespace == "urn:other" {
			prefix = "o:"
		}
		declaration := root
		if want.source != "root.xsd" {
			declaration = fixtures[want.source].contents
		}
		if !hasReference || !reference.IsNamed() || member.DeclaredType() != name || reference.Name() != name || reference.QName() != name || !hasMemberID || !hasReferenceID || memberID != id || referenceID != id || id.Source() != SourceID(want.source) || reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="`+prefix+want.typeName+`"`, 1) || reference.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, want.variety, 1) {
			t.Fatalf("%s type provenance = %#v/%v/%v", want.local, reference, memberID, referenceID)
		}
		minimum, maximum := intrinsicMin, intrinsicMax
		var minLoc, maxLoc Loc
		if index < 2 {
			minimum, maximum = effectiveMin, "100"
			minLoc = allParticleTestTokenLoc(t, "root.xsd", root, `value="`+effectiveMin+`"`, 1)
			maxLoc = allParticleTestTokenLoc(t, "root.xsd", root, `value="100"`, 1)
		}
		assertAllIntegerBounds(t, reference, version, minimum, maximum, minLoc, maxLoc)
	}
}

//nolint:gocognit // Keep the omission and excluded-shape matrix beside its policy cases.
func TestDirectAllLongOmissionAndExclusions(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy)+" omitted member", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="drop" type="xs:long" minOccurs="0" maxOccurs="0"/><xs:element name="inline" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element><xs:element name="keep" type="xs:long"/></xs:all>`, "")
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
		for _, test := range []struct {
			name, member, defs, marker, code, spec string
			cause                                  error
		}{
			{"inline long", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported},
			{"named string", `<xs:element name="v" type="r:Text"/>`, `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Text"`, UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), errSchemaAllMemberScalar},
			{"named list", `<xs:element name="v" type="r:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:long"/></xs:simpleType>`, `type="r:List"`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported},
			{"named union", `<xs:element name="v" type="r:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:long"/></xs:simpleType>`, `type="r:Union"`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported},
			{"inline complex", `<xs:element name="v"><xs:complexType/></xs:element>`, "", `<xs:complexType/>`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.defs)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("excluded %s = %s/%v", test.name, diagnostic, err)
				}
			})
		}
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+" owner zero", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:long"/></xs:all>`, "")
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
func TestDirectAllLongBoundaryDiagnostics(t *testing.T) {
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
			{"unresolved", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"zero unresolved", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong kind", `<xs:element name="v" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:long"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"invalid long facet before zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>`, `value="9223372036854775808"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"malformed long facet before zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, boundSpecRef(profile.version, BoundMaxInclusive, boundDefinitionRule), 1, FailureInvalid, errInvalidBoundValue},
			{"invalid inline facet before zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType></xs:element>`, "", `value="9223372036854775808"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"invalid member occurrence", `<xs:element name="v" type="xs:long" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, FailureInvalid, nil},
			{"duplicate", `<xs:element name="v" type="xs:long"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:long"/></xs:simpleType>`, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"cycle before zero", `<xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, `base="r:Two"`, `base="r:One"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaSimpleTypeBaseCycle},
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
				if !reflect.DeepEqual(diagnostic.Related(), related) || test.cause == nil && diagnostic.Unwrap() == nil {
					t.Fatalf("%s related/cause = %v/%v", test.name, diagnostic.Related(), diagnostic.Unwrap())
				}
			})
		}
		for _, test := range []struct{ name, model, marker, code string }{
			{"repeated long member", `<xs:all><xs:element name="v" type="xs:long" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
			{"zero repeated long member", `<xs:all><xs:element name="v" type="xs:long" minOccurs="0" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
			{"zero all owner", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:long"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
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
			{"owner zero bad facet", `<xs:element name="v" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>`, `value="9223372036854775808"`, InvalidBoundRestrictionCode, boundSpecRef(XSDVersion11, BoundMaxInclusive, boundRestrictionRule), errInvalidBoundRestriction},
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
func TestDirectAllLongReferenceTargetAndConsumers(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, defs string
			wantReference      bool
		}{
			{"builtin", `<xs:element name="v" type="xs:long"/>`, "", false},
			{"named", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:long"/></xs:simpleType>`, false},
			{"ref to long", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:long"/>`, true},
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

func TestDirectAllLongHiddenGraphTypeRemainsUnresolved(t *testing.T) {
	assertAllIntegerHiddenGraphTypeUnresolved(t, "long")
}

func assertAllIntegerHiddenGraphTypeUnresolved(t *testing.T, atomic string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:all"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:all><xs:element name="v" type="h:Hidden"/></xs:all></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
		"hidden.xsd": {id: "hidden.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + atomic + `"/></xs:simpleType></xs:schema>`},
	}
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="h:Hidden"`, 1) || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
				t.Fatalf("hidden integer diagnostic = %s/%v", diagnostic, err)
			}
		})
	}
}
