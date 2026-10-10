package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // The ordered public view carries graph and facet provenance together.
func TestDirectAllNamedNegativeIntegerGraphFacts(t *testing.T) {
	for _, profile := range []struct {
		policy     LanguagePolicy
		version    XSDVersion
		occurs     string
		wantOccurs string
	}{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all" version="1.0">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="Direct"><xs:restriction base="xs:negativeInteger"><xs:minInclusive value="-20"/><xs:maxInclusive value="-2"/><xs:totalDigits value="2"/><xs:enumeration value="-10"/></xs:restriction></xs:simpleType>
  <xs:complexType name="Record"><xs:all minOccurs="0">
    <xs:element name="builtin" type="xs:negativeInteger"/>
    <xs:element name="direct" type="r:Direct" %s/>
    <xs:element name="forward" type="r:Forward"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="imported" type="o:Imported"/>
    <xs:element name="chameleon" type="r:Chameleon"/>
  </xs:all></xs:complexType>
  <xs:element name="root" type="r:Record"/>
  <xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType>
</xs:schema>`, testXSDNamespace, profile.occurs)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("parse named all: %v", err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated graph facts/order changed: %v", err)
			}
			var sources []SourceID
			for _, component := range schema.Components() {
				source := component.ID().Source()
				if len(sources) == 0 || sources[len(sources)-1] != source {
					sources = append(sources, source)
				}
			}
			if want := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}; !reflect.DeepEqual(sources, want) {
				t.Fatalf("discovery source order = %v, want %v", sources, want)
			}
			all := directAllFromSchema(t, schema)
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" {
				t.Fatalf("all location/range = %s/%s", all.Loc(), all.Occurrences())
			}
			members := all.Members()
			if len(members) != 6 {
				t.Fatalf("members = %d, want six", len(members))
			}
			for index, want := range []struct{ local, namespace, typeName, source, variety string }{
				{"direct", "urn:all", "Direct", "root.xsd", `<xs:restriction base="xs:negativeInteger"`},
				{"forward", "urn:all", "Forward", "root.xsd", `<xs:restriction base="r:Direct"`},
				{"included", "urn:all", "Included", "included.xsd", `<xs:restriction base="xs:negativeInteger"`},
				{"imported", "urn:other", "Imported", "other.xsd", `<xs:restriction base="xs:negativeInteger"`},
				{"chameleon", "urn:all", "Chameleon", "chameleon.xsd", `<xs:restriction base="xs:negativeInteger"`},
			} {
				member, ok := members[index+1].(ElementParticle)
				if !ok || member.Name().Local() != want.local || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+want.local+`"`, 1) {
					t.Fatalf("member %d = %#v, want ordered %s", index+1, members[index+1], want.local)
				}
				name := mustTestQName(t, want.namespace, want.typeName)
				reference, ok := member.TypeReference()
				id := componentIDForName(t, schema, name)
				memberID, hasMemberID := member.TypeID()
				referenceID, hasReferenceID := reference.ComponentID()
				if !ok || !reference.IsNamed() || member.DeclaredType() != name || reference.Name() != name || !hasMemberID || memberID != id || !hasReferenceID || referenceID != id || id.Source() != SourceID(want.source) {
					t.Fatalf("member %s type/ID = %q/%#v/%v/%v, want %q/%v", want.local, member.DeclaredType(), reference, memberID, referenceID, name, id)
				}
				prefix := "r:"
				if want.namespace == "urn:other" {
					prefix = "o:"
				}
				declaration := root
				if want.source != "root.xsd" {
					declaration = fixtures[want.source].contents
				}
				if reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="`+prefix+want.typeName+`"`, 1) || reference.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, want.variety, 1) {
					t.Fatalf("member %s provenance = %s/%s", want.local, reference.Loc(), reference.VarietyLoc())
				}
				bounds, hasBounds := reference.IntegerBounds()
				maximum, hasMaximum := bounds.MaxInclusiveFacet()
				wantMax, wantLoc := "-1", Loc{}
				if index < 2 {
					wantMax, wantLoc = "-2", allParticleTestTokenLoc(t, "root.xsd", root, `value="-2"`, 1)
				}
				if !hasBounds || !hasMaximum || bounds.Version() != profile.version || maximum.Value().Canonical() != wantMax || maximum.Loc() != wantLoc {
					t.Fatalf("member %s maximum = %v/%t, want %s at %s", want.local, maximum, hasMaximum, wantMax, wantLoc)
				}
				if index < 2 {
					minimum, present := bounds.MinInclusiveFacet()
					if !present || minimum.Value().Canonical() != "-20" || minimum.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `value="-20"`, 1) {
						t.Fatalf("member %s minimum = %v/%t", want.local, minimum, present)
					}
					definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, name)[0].SimpleTypeDefinition()
					digits := definition.DigitFacets()
					total, present := digits.TotalDigits()
					loc, hasLoc := digits.TotalDigitsLoc()
					if !ok || !present || total.Canonical() != "2" || !hasLoc || loc != allParticleTestTokenLoc(t, "root.xsd", root, `value="2"`, 1) {
						t.Fatalf("member %s totalDigits = %s/%s", want.local, total, loc)
					}
					enumeration := definition.IntegerEnumerationFacets()
					values, locations := enumeration.Values(), enumeration.Locations()
					if len(values) != 1 || values[0].Canonical() != "-10" || len(locations) != 1 || locations[0] != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:enumeration value="-10"`, 1) {
						t.Fatalf("member %s enumeration = %v at %v", want.local, values, locations)
					}
					values[0].value.SetInt64(99)
					locations[0] = Loc{}
					if definition.IntegerEnumerationFacets().Values()[0].Canonical() != "-10" || definition.IntegerEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("caller changed effective enumeration facts")
					}
				}
			}
			builtin, ok := members[0].(ElementParticle)
			if !ok {
				t.Fatalf("built-in member = %T", members[0])
			}
			builtinReference, _ := builtin.TypeReference()
			builtinBounds, _ := builtinReference.IntegerBounds()
			builtinMaximum, _ := builtinBounds.MaxInclusiveFacet()
			builtinID, hasBuiltinID := builtin.TypeID()
			if !builtinReference.IsBuiltin() || hasBuiltinID || !builtinID.IsZero() || builtinMaximum.Value().Canonical() != "-1" || !builtinMaximum.Loc().IsZero() {
				t.Fatalf("built-in member = %#v/%v/%t/%v", builtinReference, builtinID, hasBuiltinID, builtinMaximum)
			}
			first, ok := members[1].(ElementParticle)
			if !ok {
				t.Fatalf("named member = %T", members[1])
			}
			if first.Occurrences().String() != profile.wantOccurs {
				t.Fatalf("named range = %s, want %s", first.Occurrences(), profile.wantOccurs)
			}
			firstReference, _ := first.TypeReference()
			copiedBounds, _ := firstReference.IntegerBounds()
			copiedMaximum, _ := copiedBounds.MaxInclusiveFacet()
			value := copiedMaximum.Value()
			value.value.SetInt64(99)
			minimum := first.Occurrences().Minimum()
			minimum.value.SetInt64(99)
			members[1] = nil
			againFirst, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !ok {
				t.Fatal("copied member lost element shape")
			}
			againReference, _ := againFirst.TypeReference()
			againBounds, _ := againReference.IntegerBounds()
			againMaximum, _ := againBounds.MaxInclusiveFacet()
			if againFirst.Occurrences().String() != profile.wantOccurs || againMaximum.Value().Canonical() != "-2" || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller mutation changed named all facts")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Keep member/owner omission and reference identity in one policy matrix.
func TestDirectAllNamedNegativeIntegerOmissionAndReference(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="gone" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element ref="r:global"/><xs:element name="keep" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType><xs:element name="global" type="r:Named"/><xs:element name="root" type="r:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 2 {
				t.Fatalf("omitted named member left %#v", members)
			}
			reference, isReference := members[0].(ElementReferenceParticle)
			keep, isElement := members[1].(ElementParticle)
			if !isReference || !isElement || reference.Name().Local() != "global" || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() || keep.Name().Local() != "keep" {
				t.Fatalf("ordered ref/named local = %#v", members)
			}
			members[0] = nil
			if _, ok := directAllFromSchema(t, schema).Members()[0].(ElementReferenceParticle); !ok {
				t.Fatal("caller changed referenced member")
			}
			version := XSDVersion11
			if policy == Strict10 {
				version = XSDVersion10
			}
			assertDirectAllConsumerRejection(t, schema, directAllFromSchema(t, schema), version)
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+" owner zero", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
			if definition.Particle() != nil {
				t.Fatalf("zero owner published %T", definition.Particle())
			}
		})
	}
}

func TestDirectAllNegativeIntegerReferenceKeepsTargetSeparate(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="named" type="r:Debt"/><xs:element ref="r:excluded"/></xs:all>`, `<xs:simpleType name="Debt"><xs:restriction base="xs:negativeInteger"/></xs:simpleType><xs:element name="excluded" type="xs:nonPositiveInteger"/><xs:element name="root" type="r:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if len(members) != 2 {
				t.Fatalf("member count = %d, want two", len(members))
			}
			named, isNamed := members[0].(ElementParticle)
			reference, isReference := members[1].(ElementReferenceParticle)
			if !isNamed || !isReference || named.DeclaredType() != mustTestQName(t, "urn:all", "Debt") || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:excluded"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "excluded"))[0].ID() {
				t.Fatalf("named/ref members = %#v", members)
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Each alternate resolution, facet, policy, and shape exit is observable.
func TestDirectAllNamedNegativeIntegerBoundaryFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, extra, primary, related, code, spec string
			occurrence                                        int
			class                                             FailureClass
			cause                                             error
		}{
			{"unresolved", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"zero unresolved", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong kind", `<xs:element name="v" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:negativeInteger"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"zero wrong kind", `<xs:element name="v" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:negativeInteger"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"invalid bound", `<xs:element name="v" type="r:Named" maxOccurs="maybe"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, FailureInvalid, nil},
			{"invalid facet before zero", `<xs:element name="v" type="r:Named" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"><xs:maxInclusive value="0"/></xs:restriction></xs:simpleType>`, `value="0"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"invalid inline facet before zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:negativeInteger"><xs:maxInclusive value="0"/></xs:restriction></xs:simpleType></xs:element>`, "", `value="0"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"duplicate", `<xs:element name="v" type="r:Named"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"inline", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), 1, FailureUnsupported, errSchemaAllMemberScalar},
			{"direct excluded", `<xs:element name="v" type="xs:language"/>`, "", `type="xs:language"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named excluded", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Named"`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), 1, FailureUnsupported, errSchemaAllMemberScalar},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.occurrence)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %s, cause %v; want %s/%s at %s", test.name, diagnostic, err, test.class, test.code, wantLoc)
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("%s related = %v, want %v", test.name, diagnostic.Related(), related)
				}
				if test.name == "invalid bound" && diagnostic.Unwrap() == nil {
					t.Fatal("invalid occurrence lost lexical cause")
				}
			})
		}
		if profile.policy == Strict10 {
			for _, test := range []struct{ name, model, marker string }{
				{"member repeat", `<xs:all><xs:element name="v" type="r:Named" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`},
				{"zero member repeat", `<xs:all><xs:element name="v" type="r:Named" minOccurs="0" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`},
				{"owner zero", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, `maxOccurs="0"`},
			} {
				t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
					root := allParticleTestRoot(test.model, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1)
					wantCode := diagnosticSchemaAllOccurrenceVersionCode
					if test.name == "owner zero" {
						wantCode = UnsupportedSchemaSyntaxCode
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != wantCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 named all occurrence = %s, cause %v", diagnostic, err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // One table checks the same diagnostic contract for graph visibility and cycles.
func TestDirectAllNamedNegativeIntegerHiddenAndCyclicGraphs(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		hiddenRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:all"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:all><xs:element name="v" type="h:Hidden"/></xs:all></xs:complexType></xs:schema>`
		hiddenSources := map[string]discoveryFixture{
			"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
			"hidden.xsd": {id: "hidden.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:schema>`},
		}
		cycleRoot := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`)
		for _, test := range []struct {
			name, root, primary, related, code, spec string
			fixtures                                 map[string]discoveryFixture
			cause                                    error
		}{
			{"hidden", hiddenRoot, `type="h:Hidden"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), hiddenSources, errSchemaElementTypeUnresolved},
			{"zero cycle", cycleRoot, `base="r:Two"`, `base="r:One"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), nil, errSchemaSimpleTypeBaseCycle},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, test.fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", test.root, test.primary, 1) || diagnostic.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %s, cause %v", test.name, diagnostic, err)
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", test.root, test.related, 1)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("%s related = %v, want %v", test.name, diagnostic.Related(), related)
				}
			})
		}
	}
}

func TestDirectAllNamedNegativeIntegerOwnerZeroStillChecksMembers(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, extra, marker, code, spec string
			cause                                   error
		}{
			{"unresolved", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), errSchemaElementTypeUnresolved},
			{"invalid facet", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"><xs:maxInclusive value="0"/></xs:restriction></xs:simpleType>`, `value="0"`, InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), errInvalidBoundRestriction},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0">`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("omitted owner diagnostic = %s, cause %v", diagnostic, err)
				}
			})
		}
	}
}
