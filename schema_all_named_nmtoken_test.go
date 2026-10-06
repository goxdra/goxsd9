package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Public member facts and graph provenance are one admission boundary.
func TestDirectAllNamedNMTOKENGraphFactsAndConsumers(t *testing.T) {
	for _, profile := range []struct {
		policy     LanguagePolicy
		version    XSDVersion
		bounds     string
		wantBounds string
	}{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all" version="%s">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="Direct"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" first "/><xs:enumeration value="second"/></xs:restriction></xs:simpleType>
  <xs:complexType name="Record"><xs:all>
    <xs:element name="direct" type="r:Direct" %s/>
    <xs:element name="forward" type="r:Forward"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="imported" type="o:Imported"/>
    <xs:element name="chameleon" type="r:Chameleon"/>
  </xs:all></xs:complexType>
  <xs:element name="root" type="r:Record"/>
  <xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType>
</xs:schema>`, testXSDNamespace, profile.version, profile.bounds)
			fixtures := map[string]discoveryFixture{
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:simpleType name="Included"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="chameleon"/></xs:restriction></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="imported"/></xs:restriction></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discover named all: %v", err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if len(members) != 5 {
				t.Fatalf("member count = %d, want 5", len(members))
			}
			for index, want := range []struct{ local, namespace, typeName, source, variety, value, valueSource, valueMarker string }{
				{"direct", "urn:all", "Direct", "root.xsd", `<xs:restriction base="xs:NMTOKEN"`, " first ", "root.xsd", `value=" first "`},
				{"forward", "urn:all", "Forward", "root.xsd", `<xs:restriction base="r:Direct"`, " first ", "root.xsd", `value=" first "`},
				{"included", "urn:all", "Included", "included.xsd", `<xs:restriction base="xs:NMTOKEN"`, "included", "included.xsd", `value="included"`},
				{"imported", "urn:other", "Imported", "other.xsd", `<xs:restriction base="xs:NMTOKEN"`, "imported", "other.xsd", `value="imported"`},
				{"chameleon", "urn:all", "Chameleon", "chameleon.xsd", `<xs:restriction base="xs:NMTOKEN"`, "chameleon", "chameleon.xsd", `value="chameleon"`},
			} {
				member, ok := members[index].(ElementParticle)
				if !ok || member.Name().Local() != want.local || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+want.local+`"`, 1) {
					t.Fatalf("member %d = %#v, want ordered root local %s", index, members[index], want.local)
				}
				name := mustTestQName(t, want.namespace, want.typeName)
				reference, ok := member.TypeReference()
				id := componentIDForName(t, schema, name)
				gotID, hasID := member.TypeID()
				refID, refHasID := reference.ComponentID()
				if !ok || !reference.IsNamed() || reference.IsBuiltin() || member.DeclaredType() != name || reference.Name() != name || !hasID || gotID != id || !refHasID || refID != id {
					t.Fatalf("member %s type = %#v, IDs %v/%t and %v/%t, want named %s ID %v", want.local, reference, gotID, hasID, refID, refHasID, name, id)
				}
				definitions := schema.FindKind(ComponentKindSimpleTypeDefinition, name)
				if len(definitions) != 1 {
					t.Fatalf("member %s definitions = %d, want 1", want.local, len(definitions))
				}
				definition, hasDefinition := definitions[0].SimpleTypeDefinition()
				if !hasDefinition || !definition.IsString() || definition.Base() != mustTestQName(t, testXSDNamespace, "NMTOKEN") && want.typeName != "Forward" {
					t.Fatalf("member %s definition = %#v, want effective NMTOKEN", want.local, definition)
				}
				if want.typeName == "Forward" && definition.Base() != mustTestQName(t, "urn:all", "Direct") {
					t.Fatalf("forward NMTOKEN base = %s, want Direct", definition.Base())
				}
				declaration := root
				if want.source != "root.xsd" {
					declaration = fixtures[want.source].contents
				}
				if reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="`+map[bool]string{true: "o:", false: "r:"}[want.namespace == "urn:other"]+want.typeName+`"`, 1) || reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, want.variety, 1) {
					t.Fatalf("member %s reference provenance = %s/%s, want root use and %s declaration", want.local, reference.Loc(), reference.VarietyLoc(), want.source)
				}
				space, ok := reference.StringWhiteSpaceFacet()
				if !ok || space.Value() != "collapse" || !space.Fixed() || !space.Loc().IsZero() {
					t.Fatalf("member %s whiteSpace = %#v/%t, want fixed collapse", want.local, space, ok)
				}
				values := reference.StringEnumerationFacets().Values()
				if len(values) == 0 || values[0] != want.value {
					t.Fatalf("member %s enumeration = %#v, want first %q", want.local, values, want.value)
				}
				locations := reference.StringEnumerationFacets().Locations()
				valueInput := root
				if want.valueSource != "root.xsd" {
					valueInput = fixtures[want.valueSource].contents
				}
				if len(locations) != len(values) || locations[0] != allParticleTestTokenLoc(t, SourceID(want.valueSource), valueInput, want.valueMarker, 1) {
					t.Fatalf("member %s enumeration locations = %#v", want.local, locations)
				}
				if index == 0 && member.Occurrences().String() != profile.wantBounds {
					t.Fatalf("direct member range = %s", member.Occurrences())
				}
			}
			firstMember := requireLocalTokenElementParticle(t, members[0], 0)
			first, hasFirst := firstMember.TypeReference()
			if !hasFirst {
				t.Fatal("named member lost type reference")
			}
			values := first.StringEnumerationFacets().Values()
			values[0] = "changed"
			locations := first.StringEnumerationFacets().Locations()
			locations[0] = Loc{}
			bounds := firstMember.Occurrences()
			minimum := bounds.Minimum()
			minimum.value.SetInt64(9)
			if maximum, finite := bounds.Maximum().Finite(); finite {
				maximum.value.SetInt64(9)
			}
			members[0] = nil
			again := requireLocalTokenElementParticle(t, directAllFromSchema(t, schema).Members()[0], 0)
			refAgain, hasAgain := again.TypeReference()
			if !hasAgain {
				t.Fatal("copied named member lost type reference")
			}
			if !reflect.DeepEqual(refAgain.StringEnumerationFacets().Values(), []string{" first ", "second"}) {
				t.Fatal("caller changed completed enumeration facts")
			}
			if again.Occurrences().String() != profile.wantBounds || refAgain.StringEnumerationFacets().Locations()[0].IsZero() {
				t.Fatal("caller changed completed occurrence or location facts")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

func TestDirectAllNamedNMTOKENOmissionAndRef(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := allParticleTestRoot(`<xs:all><xs:element name="gone" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element ref="r:word"/><xs:element name="keep" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType><xs:element name="word" type="r:Named"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		members := directAllFromSchema(t, schema).Members()
		if len(members) != 2 {
			t.Fatalf("%s members = %#v, want ref and keep", policy, members)
		}
		ref, isRef := members[0].(ElementReferenceParticle)
		keep, isElement := members[1].(ElementParticle)
		if !isRef || !isElement || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:word"`, 1) || keep.Name().Local() != "keep" {
			t.Fatalf("%s members = %#v, want ordered ref and local", policy, members)
		}
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="gone" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		definition, ok := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
		if !ok || definition.Particle() != nil {
			t.Fatalf("%s omitted all owner = %#v", policy, definition.Particle())
		}
	}
}

//nolint:gocognit // Assert distinct exit codes, causes, and locations for named all members.
func TestDirectAllNamedNMTOKENFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, extra, primary, code, spec string
			class                                    FailureClass
			cause                                    error
			related                                  string
			occurrence                               int
		}{
			{"unresolved", `<xs:element name="word" type="r:Missing"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), FailureInvalid, errSchemaElementTypeUnresolved, "", 1},
			{"zero unresolved", `<xs:element name="word" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), FailureInvalid, errSchemaElementTypeUnresolved, "", 1},
			{"wrong kind", `<xs:element name="word" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:NMTOKEN"/>`, `type="r:Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), FailureInvalid, errSchemaElementTypeWrongKind, `<xs:element name="Wrong"`, 1},
			{"zero wrong kind", `<xs:element name="word" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:NMTOKEN"/>`, `type="r:Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), FailureInvalid, errSchemaElementTypeWrongKind, `<xs:element name="Wrong"`, 1},
			{"duplicate", `<xs:element name="word" type="r:Named"/><xs:element name="word" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`, `<xs:element name="word"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), FailureInvalid, errSchemaAllMemberDuplicate, `<xs:element name="word"`, 2},
			{"inline", `<xs:element name="word"><xs:simpleType><xs:restriction base="xs:NMTOKEN"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), FailureUnsupported, ErrUnsupported, "", 1},
			{"invalid bound", `<xs:element name="word" type="r:Named" maxOccurs="maybe"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`, `maxOccurs="maybe"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil, "", 1},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.occurrence) || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %s, cause %v", test.name, diagnostic, err)
				}
				if test.related == "" && len(diagnostic.Related()) == 0 {
					return
				}
				if len(diagnostic.Related()) != 1 || diagnostic.Related()[0] != allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1) {
					t.Fatalf("%s related = %v", test.name, diagnostic.Related())
				}
			})
		}
		if profile.policy == Strict10 {
			root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:Named" maxOccurs="2"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="2"`, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10 repeated NMTOKEN = %s, cause %v", diagnostic, err)
			}
		}
	}
}

func TestDirectAllNamedNMTOKENGraphFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy)+"/hidden", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:all"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:all><xs:element name="v" type="h:Hidden"/></xs:all></xs:complexType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
				"hidden.xsd": {id: "hidden.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="h:Hidden"`, 1) || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
				t.Fatalf("hidden NMTOKEN = %s, cause %v", diagnostic, err)
			}
		})
		t.Run(string(profile.policy)+"/zero cycle", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `base="r:Two"`, 1) || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("NMTOKEN zero cycle = %s, cause %v", diagnostic, err)
			}
			if related := diagnostic.Related(); len(related) != 1 || related[0] != allParticleTestTokenLoc(t, "root.xsd", root, `base="r:One"`, 1) {
				t.Fatalf("NMTOKEN zero cycle related = %v", related)
			}
		})
	}
}

//nolint:gocognit // These exits must retain their datatype and shape diagnostics before omission.
func TestDirectAllNamedNMTOKENZeroAndFacetGates(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy)+"/valid inline zero", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="gone" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:NMTOKEN"/></xs:simpleType></xs:element><xs:element name="keep" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("omitted inline NMTOKEN members = %#v", members)
			}
			member, ok := members[0].(ElementParticle)
			if !ok || member.Name().Local() != "keep" {
				t.Fatalf("omitted inline NMTOKEN members = %#v", members)
			}
		})
		for _, test := range []struct {
			name, member, extra, primary string
		}{
			{"named invalid enumeration", `<xs:element name="gone" type="r:Named" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="bad value"/></xs:restriction></xs:simpleType>`, `value="bad value"`},
			{"inline invalid enumeration", `<xs:element name="gone" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="bad value"/></xs:restriction></xs:simpleType></xs:element>`, "", `value="bad value"`},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidEnumerationRestrictionCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, "enumeration-valid-restriction") || len(diagnostic.Related()) != 0 || !errors.Is(err, errInvalidEnumerationRestriction) || !errors.Is(err, errSchemaNMTOKENValueViolation) {
					t.Fatalf("invalid zero NMTOKEN = %s, cause %v", diagnostic, err)
				}
			})
		}
		t.Run(string(profile.policy)+"/named string remains excluded", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:string"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="r:Named"`, 1) || diagnostic.SpecRef() != schemaAllLimitedSpecRef(profile.version) || !errors.Is(err, errSchemaAllMemberScalar) {
				t.Fatalf("named string diagnostic = %s, cause %v", diagnostic, err)
			}
		})
	}
}
