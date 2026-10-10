package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // One graph makes order, type provenance, facets, and copies observable together.
func TestDirectAllNamedNonNegativeIntegerGraphFacts(t *testing.T) {
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
<xs:simpleType name="Direct"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="3"/><xs:maxInclusive value="20"/><xs:totalDigits value="2"/><xs:enumeration value="10"/></xs:restriction></xs:simpleType>
<xs:complexType name="Record"><xs:all><xs:element name="builtin" type="xs:nonNegativeInteger"/><xs:element name="direct" type="r:Direct" %s/><xs:element name="forward" type="r:Forward"/><xs:element name="included" type="r:Included"/><xs:element name="imported" type="o:Imported"/><xs:element name="chameleon" type="r:Chameleon"/></xs:all></xs:complexType>
<xs:element name="root" type="r:Record"/><xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType></xs:schema>`, testXSDNamespace, profile.rangeText)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`},
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
			if len(members) != 6 || all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all>`, 1) {
				t.Fatalf("all = %s/%d members", all.Loc(), len(members))
			}
			builtin, ok := members[0].(ElementParticle)
			if !ok {
				t.Fatalf("builtin shape = %T", members[0])
			}
			builtinRef, _ := builtin.TypeReference()
			builtinBounds, _ := builtinRef.IntegerBounds()
			builtinMin, _ := builtinBounds.MinInclusiveFacet()
			builtinID, hasBuiltinID := builtin.TypeID()
			if !builtinRef.IsBuiltin() || hasBuiltinID || !builtinID.IsZero() || builtinMin.Value().Canonical() != "0" || !builtinMin.Loc().IsZero() || builtinRef.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:nonNegativeInteger"`, 1) {
				t.Fatalf("builtin identity/bound = %#v/%v/%v", builtinRef, builtinID, builtinMin)
			}
			for index, want := range []struct{ local, namespace, typeName, source, variety string }{
				{"direct", "urn:all", "Direct", "root.xsd", `<xs:restriction base="xs:nonNegativeInteger"`},
				{"forward", "urn:all", "Forward", "root.xsd", `<xs:restriction base="r:Direct"`},
				{"included", "urn:all", "Included", "included.xsd", `<xs:restriction base="xs:nonNegativeInteger"`},
				{"imported", "urn:other", "Imported", "other.xsd", `<xs:restriction base="xs:nonNegativeInteger"`},
				{"chameleon", "urn:all", "Chameleon", "chameleon.xsd", `<xs:restriction base="xs:nonNegativeInteger"`},
			} {
				member, hasMember := members[index+1].(ElementParticle)
				if !hasMember || member.Name().Local() != want.local || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+want.local+`"`, 1) {
					t.Fatalf("member %d = %#v", index+1, members[index+1])
				}
				name := mustTestQName(t, want.namespace, want.typeName)
				id := componentIDForName(t, schema, name)
				ref, hasRef := member.TypeReference()
				memberID, hasMemberID := member.TypeID()
				refID, hasRefID := ref.ComponentID()
				prefix := "r:"
				if want.namespace == "urn:other" {
					prefix = "o:"
				}
				declaration := root
				if want.source != "root.xsd" {
					declaration = fixtures[want.source].contents
				}
				if !hasRef || !ref.IsNamed() || member.DeclaredType() != name || ref.Name() != name || !hasMemberID || memberID != id || !hasRefID || refID != id || id.Source() != SourceID(want.source) || ref.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="`+prefix+want.typeName+`"`, 1) || ref.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, want.variety, 1) {
					t.Fatalf("member %s identity/provenance = %#v/%v/%v", want.local, ref, memberID, refID)
				}
				bounds, hasBounds := ref.IntegerBounds()
				minimum, hasMinimum := bounds.MinInclusiveFacet()
				maximum, hasMaximum := bounds.MaxInclusiveFacet()
				wantMin, wantMinLoc := "0", Loc{}
				if index < 2 {
					wantMin, wantMinLoc = "3", allParticleTestTokenLoc(t, "root.xsd", root, `value="3"`, 1)
				}
				if !hasBounds || !hasMinimum || bounds.Version() != profile.version || minimum.Value().Canonical() != wantMin || minimum.Loc() != wantMinLoc || hasMaximum != (index < 2) {
					t.Fatalf("member %s bounds = %v/%v", want.local, minimum, maximum)
				}
				if index < 2 && (maximum.Value().Canonical() != "20" || maximum.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `value="20"`, 1)) {
					t.Fatalf("member %s maximum = %v", want.local, maximum)
				}
			}
			first, hasFirst := members[1].(ElementParticle)
			if !hasFirst {
				t.Fatalf("named member = %T", members[1])
			}
			if first.Occurrences().String() != profile.wantRange {
				t.Fatalf("named range = %s", first.Occurrences())
			}
			definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:all", "Direct"))[0].SimpleTypeDefinition()
			total, hasTotal := definition.DigitFacets().TotalDigits()
			loc, hasLoc := definition.DigitFacets().TotalDigitsLoc()
			values, locations := definition.IntegerEnumerationFacets().Values(), definition.IntegerEnumerationFacets().Locations()
			if !ok || !hasTotal || total.Canonical() != "2" || !hasLoc || loc != allParticleTestTokenLoc(t, "root.xsd", root, `value="2"`, 1) || len(values) != 1 || values[0].Canonical() != "10" || len(locations) != 1 || locations[0] != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:enumeration value="10"`, 1) {
				t.Fatalf("effective facets = %v/%v at %s/%v", total, values, loc, locations)
			}
			values[0].value.SetInt64(99)
			locations[0] = Loc{}
			members[1] = nil
			ref, _ := first.TypeReference()
			bounds, _ := ref.IntegerBounds()
			minimum, _ := bounds.MinInclusiveFacet()
			copied := minimum.Value()
			copied.value.SetInt64(99)
			occur := first.Occurrences().Minimum()
			occur.value.SetInt64(99)
			fresh, hasFresh := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !hasFresh {
				t.Fatal("copied member lost element shape")
			}
			freshRef, _ := fresh.TypeReference()
			freshBounds, _ := freshRef.IntegerBounds()
			freshMinimum, _ := freshBounds.MinInclusiveFacet()
			if fresh.Occurrences().String() != profile.wantRange || freshMinimum.Value().Canonical() != "3" || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "10" || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller changed schema facts")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

func TestDirectAllNamedNonNegativeIntegerDiagnostics(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, extra, marker, code, spec string
			related                                 []string
			occurrence                              int
			class                                   FailureClass
			cause                                   error
		}{
			{"unresolved", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), nil, 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"zero unresolved", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), nil, 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong kind", `<xs:element name="v" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:nonNegativeInteger"/>`, `type="r:Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), []string{`<xs:element name="Wrong"`}, 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"invalid facet before zero", `<xs:element name="v" type="r:Named" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="-1"/></xs:restriction></xs:simpleType>`, `value="-1"`, InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMinInclusive, boundRestrictionRule), nil, 1, FailureInvalid, errInvalidBoundRestriction},
			{"invalid lexical", `<xs:element name="v" type="r:Named" maxOccurs="maybe"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`, `maxOccurs="maybe"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), nil, 1, FailureInvalid, nil},
			{"invalid range", `<xs:element name="v" type="r:Named" minOccurs="2" maxOccurs="1"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`, `<xs:element name="v"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), []string{`minOccurs="2"`, `maxOccurs="1"`}, 1, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
			{"duplicate", `<xs:element name="v" type="r:Named"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), []string{`<xs:element name="v"`}, 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"inline excluded", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), nil, 1, FailureUnsupported, ErrUnsupported},
			{"named excluded", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Named"`, UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), nil, 1, FailureUnsupported, errSchemaAllMemberScalar},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				var related []Loc
				for _, marker := range test.related {
					related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, marker, 1))
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, test.occurrence) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), related) || test.cause != nil && !errors.Is(err, test.cause) || test.name != "inline excluded" && diagnostic.Unwrap() == nil {
					t.Fatalf("diagnostic = %s, related %v, cause %v", diagnostic, diagnostic.Related(), err)
				}
			})
		}
	}
}

//nolint:gocognit // These outcomes distinguish omitted terms, refs, and edition limits.
func TestDirectAllNamedNonNegativeIntegerOmissionReferenceAndPolicy(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="gone" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element ref="r:global"/><xs:element name="keep" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType><xs:element name="global" type="r:Named"/><xs:element name="root" type="r:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if len(members) != 2 {
				t.Fatalf("members = %#v", members)
			}
			reference, isReference := members[0].(ElementReferenceParticle)
			keep, isElement := members[1].(ElementParticle)
			if !isReference || !isElement || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() || keep.Name().Local() != "keep" || keep.DeclaredType() != mustTestQName(t, "urn:all", "Named") {
				t.Fatalf("ref/local order or target = %#v", members)
			}
			members[0] = nil
			if _, ok := directAllFromSchema(t, schema).Members()[0].(ElementReferenceParticle); !ok {
				t.Fatal("caller changed ref shape")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
		if definition.Particle() != nil {
			t.Fatalf("zero owner published %T", definition.Particle())
		}
	}
	for _, test := range []struct{ name, model, marker, code string }{
		{"repeated member", `<xs:all><xs:element name="v" type="r:Named" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
		{"zero owner", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
	} {
		t.Run("Strict10/"+test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10 occurrence = %s, cause %v", diagnostic, err)
			}
		})
	}
}

//nolint:gocognit // Graph and owner-omission exits share the same resolution boundary.
func TestDirectAllNamedNonNegativeIntegerGraphFailuresAndOwnerZeroGates(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		hiddenRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:all"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:all><xs:element name="v" type="h:Hidden"/></xs:all></xs:complexType></xs:schema>`
		hiddenSources := map[string]discoveryFixture{
			"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
			"hidden.xsd": {id: "hidden.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`},
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
				var related []Loc
				if test.related != "" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", test.root, test.related, 1)}
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", test.root, test.primary, 1) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, test.cause) {
					t.Fatalf("graph failure = %s, cause %v", diagnostic, err)
				}
			})
		}
		if profile.policy == Strict10 {
			continue
		}
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="-1"/></xs:restriction></xs:simpleType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidBoundRestrictionCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `value="-1"`, 1) || diagnostic.SpecRef() != boundSpecRef(profile.version, BoundMinInclusive, boundRestrictionRule) || len(diagnostic.Related()) != 0 || !errors.Is(err, errInvalidBoundRestriction) {
			t.Fatalf("zero owner facet gate = %s, cause %v", diagnostic, err)
		}
	}
}

func TestNamedNonNegativeIntegerAllOtherOwnersStayExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		extensionSpec := "xsd11-structures#cos-ct-extends"
		if version == XSDVersion10 {
			extensionSpec = "xsd10-structures#cos-ct-extends"
		}
		for _, test := range []struct{ name, body, primary, spec string }{
			{"inline owner", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="r:Count"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, "xsd10-structures#schema-document"},
			{"extension owner", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="r:Count"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, extensionSpec},
			{"named group", `<xs:group name="Group"><xs:all><xs:element name="v" type="r:Count"/></xs:all></xs:group>`, `<xs:all>`, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef()},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">` + test.body + `<xs:simpleType name="Count"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("owner shape = %s, spec %s, cause %v", diagnostic, diagnostic.SpecRef(), err)
				}
			})
		}
	}
}
