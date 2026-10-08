package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the graph's copied public facts and provenance assertions together.
func TestDirectAllIntGraphFacts(t *testing.T) {
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
<xs:simpleType name="Direct"><xs:restriction base="xs:int"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/><xs:enumeration value="7"/></xs:restriction></xs:simpleType>
<xs:complexType name="Record"><xs:all><xs:element name="builtin" type="xs:int" form="qualified" nillable="true" block="substitution"/><xs:element name="direct" type="r:Direct" %s/><xs:element name="forward" type="r:Forward"/><xs:element name="included" type="r:Included"/><xs:element name="imported" type="o:Imported"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element ref="r:reference"/></xs:all></xs:complexType>
<xs:element name="root" type="r:Record"/><xs:element name="reference" type="xs:int"/><xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType></xs:schema>`, testXSDNamespace, profile.rangeText)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:int"/></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:int"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:int"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("unstable graph: %v", err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if len(members) != 7 || all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all>`, 1) {
				t.Fatalf("all members/location = %d/%s", len(members), all.Loc())
			}
			var sources []SourceID
			for _, component := range schema.Components() {
				source := component.ID().Source()
				if len(sources) == 0 || sources[len(sources)-1] != source {
					sources = append(sources, source)
				}
			}
			if !reflect.DeepEqual(sources, []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}) {
				t.Fatalf("discovery order = %v", sources)
			}
			builtin, ok := members[0].(ElementParticle)
			if !ok || builtin.Name() != mustTestQName(t, "urn:all", "builtin") || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "int") || builtin.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="builtin"`, 1) || !builtin.IsNillable() || !reflect.DeepEqual(builtin.DisallowedSubstitutions(), []string{"substitution"}) || builtin.DisallowedSubstitutionsLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `block="substitution"`, 1) {
				t.Fatalf("built-in declaration = %#v", members[0])
			}
			builtinRef, hasRef := builtin.TypeReference()
			builtinID, hasBuiltinID := builtin.TypeID()
			refID, hasRefID := builtinRef.ComponentID()
			if !hasRef || !builtinRef.IsBuiltin() || builtinRef.Name() != mustTestQName(t, testXSDNamespace, "int") || builtinRef.QName() != builtinRef.Name() || builtinRef.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:int"`, 1) || builtinRef.VarietyLoc() != builtinRef.Loc() || hasBuiltinID || hasRefID || !builtinID.IsZero() || !refID.IsZero() {
				t.Fatalf("built-in reference = %#v", builtinRef)
			}
			assertAllIntegerBounds(t, builtinRef, profile.version, "-2147483648", "2147483647", Loc{}, Loc{})
			assertAllIntegerNamedGraphMembers(t, schema, members, root, fixtures, profile.version, "int", "-2147483648", "2147483647", "-100")
			direct, ok := members[1].(ElementParticle)
			if !ok {
				t.Fatalf("direct member = %T", members[1])
			}
			if direct.Occurrences().String() != profile.wantRange {
				t.Fatalf("occurrences = %s", direct.Occurrences())
			}
			ref, ok := members[6].(ElementReferenceParticle)
			if !ok {
				t.Fatalf("reference member = %T", members[6])
			}
			if ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:reference"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "reference"))[0].ID() {
				t.Fatalf("reference facts = %#v", ref)
			}
			definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:all", "Direct"))[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("named type missing")
			}
			digits := definition.DigitFacets()
			total, hasTotal := digits.TotalDigits()
			totalLoc, hasLoc := digits.TotalDigitsLoc()
			values, locations := definition.IntegerEnumerationFacets().Values(), definition.IntegerEnumerationFacets().Locations()
			if !hasTotal || total.Canonical() != "3" || !hasLoc || totalLoc != allParticleTestTokenLoc(t, "root.xsd", root, `value="3"`, 1) || len(values) != 1 || values[0].Canonical() != "7" || len(locations) != 1 || locations[0] != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:enumeration value="7"`, 1) {
				t.Fatalf("facet provenance = %v/%v at %s/%v", total, values, totalLoc, locations)
			}
			values[0].value.SetInt64(99)
			locations[0] = Loc{}
			members[1] = nil
			minimum := direct.Occurrences().Minimum()
			minimum.value.SetInt64(99)
			directRef, _ := direct.TypeReference()
			bounds, _ := directRef.IntegerBounds()
			minFacet, _ := bounds.MinInclusiveFacet()
			minValue := minFacet.Value()
			minValue.value.SetInt64(99)
			fresh, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !ok {
				t.Fatal("fresh direct member missing")
			}
			if fresh.Occurrences().String() != profile.wantRange || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "7" || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller changed completed facts")
			}
			freshRef, _ := fresh.TypeReference()
			assertAllIntegerBounds(t, freshRef, profile.version, "-100", "100", allParticleTestTokenLoc(t, "root.xsd", root, `value="-100"`, 1), allParticleTestTokenLoc(t, "root.xsd", root, `value="100"`, 1))
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

type allScalarDiagnosticCase struct {
	name, member, definitions, primary, related, code, spec string
	primaryIndex                                            int
	class                                                   FailureClass
	cause                                                   error
}

func checkAllScalarDiagnostic(t *testing.T, policy LanguagePolicy, test allScalarDiagnosticCase) {
	t.Helper()
	root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.definitions)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.primaryIndex) || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
		t.Fatalf("%s: diagnostic = %s/%v", test.name, diagnostic, err)
	}
	var related []Loc
	if test.related != "" {
		related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1)}
	}
	if !reflect.DeepEqual(diagnostic.Related(), related) {
		t.Fatalf("%s: related = %v, want %v", test.name, diagnostic.Related(), related)
	}
}

func TestDirectAllIntBoundaryDiagnostics(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{
		{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11},
	} {
		cases := []allScalarDiagnosticCase{
			{"malformed type QName", `<xs:element name="v" type="r:bad:Int"/>`, "", `type="r:bad:Int"`, "", invalidSchemaConditionalCode, "", 1, FailureInvalid, nil},
			{"unresolved type before omission", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong-kind type", `<xs:element name="v" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:int"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"invalid named facet", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:maxInclusive value="2147483648"/></xs:restriction></xs:simpleType>`, `value="2147483648"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"malformed named facet", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:maxInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, boundSpecRef(profile.version, BoundMaxInclusive, boundDefinitionRule), 1, FailureInvalid, errInvalidBoundValue},
			{"invalid inline facet", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:int"><xs:maxInclusive value="2147483648"/></xs:restriction></xs:simpleType></xs:element>`, "", `value="2147483648"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"invalid occurrence", `<xs:element name="v" type="xs:int" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, FailureInvalid, nil},
			{"duplicate names", `<xs:element name="v" type="xs:int"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:int"/></xs:simpleType>`, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"type cycle before omission", `<xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, `base="r:Two"`, `base="r:One"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"unresolved ref before omission", `<xs:element ref="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceUnresolved},
			{"wrong-kind ref", `<xs:element ref="r:Wrong"/>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:int"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceWrongKind},
			{"inline nonzero", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named string", `<xs:element name="v" type="r:Text"/>`, `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Text"`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), 1, FailureUnsupported, errSchemaAllMemberScalar},
			{"named list", `<xs:element name="v" type="r:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:int"/></xs:simpleType>`, `type="r:List"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named union", `<xs:element name="v" type="r:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:int"/></xs:simpleType>`, `type="r:Union"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"inline complex", `<xs:element name="v"><xs:complexType/></xs:element>`, "", `<xs:complexType/>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
		}
		for _, test := range cases {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) { checkAllScalarDiagnostic(t, profile.policy, test) })
		}
		if profile.policy != Strict10 {
			continue
		}
		for _, test := range []struct{ name, model, marker, code string }{
			{"member repeat", `<xs:all><xs:element name="v" type="xs:int" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
			{"owner zero", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:int"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
		} {
			t.Run(test.name, func(t *testing.T) {
				root := allParticleTestRoot(test.model, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("policy gate = %s/%v", diagnostic, err)
				}
			})
		}
	}
}

//nolint:gocognit // Omission and independent consumer calls share each public fixture.
func TestDirectAllIntOmissionAndConsumers(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{
		{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11},
	} {
		root := allParticleTestRoot(`<xs:all><xs:element name="builtinDrop" type="xs:int" minOccurs="0" maxOccurs="0"/><xs:element name="namedDrop" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element name="inlineDrop" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element><xs:element name="builtin" type="xs:int"/><xs:element name="named" type="r:Named"/><xs:element ref="r:reference"/></xs:all>`, `<xs:element name="root" type="r:Record"/><xs:element name="reference" type="xs:int"/><xs:simpleType name="Named"><xs:restriction base="xs:int"/></xs:simpleType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		all := directAllFromSchema(t, schema)
		members := all.Members()
		if len(members) != 3 {
			t.Fatalf("%s surviving members = %d", profile.policy, len(members))
		}
		for index, name := range []string{"builtin", "named"} {
			member, ok := members[index].(ElementParticle)
			if !ok || member.Name().Local() != name || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+name+`"`, 1) {
				t.Fatalf("%s member %d = %#v", profile.policy, index, members[index])
			}
		}
		ref, ok := members[2].(ElementReferenceParticle)
		if !ok || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:reference"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "reference"))[0].ID() {
			t.Fatalf("%s ref = %#v", profile.policy, members[2])
		}
		assertDirectAllConsumerRejection(t, schema, all, profile.version)
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:int"/></xs:all>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
		if definition.Particle() != nil {
			t.Fatalf("%s zero owner published %T", policy, definition.Particle())
		}
	}
}

func TestDirectAllIntHiddenTypeRemainsUnresolved(t *testing.T) {
	assertAllIntegerHiddenGraphTypeUnresolved(t, "int")
}

func TestDirectAllIntExcludedOwners(t *testing.T) {
	assertAllIntegerExcludedOwners(t, "int")
}

//nolint:gocognit // Each excluded owner and member shape needs a located public diagnostic.
func assertAllIntegerExcludedOwners(t *testing.T, atomic string) {
	t.Helper()
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{
		{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11},
	} {
		for _, owner := range []struct{ name, format, marker, spec10, spec11 string }{
			{"inline complex", `<xs:element name="root"><xs:complexType><xs:all>%s</xs:all></xs:complexType></xs:element>`, `<xs:all>`, "xsd10-structures#schema-document", "xsd10-structures#schema-document"},
			{"extension", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all>%s</xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends"},
			{"named group", `<xs:group name="Group"><xs:all>%s</xs:all></xs:group>`, `<xs:all>`, "xsd10-structures#schema-document", "xsd11-structures#cSchemaDocument"},
		} {
			for _, member := range []struct{ name, xml string }{
				{"builtin", `<xs:element name="v" type="xs:int"/>`},
				{"named", `<xs:element name="v" type="r:Named"/>`},
				{"ref", `<xs:element ref="r:target"/>`},
				{"inline", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element>`},
			} {
				t.Run(string(profile.policy)+"/"+owner.name+"/"+member.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">` + fmt.Sprintf(owner.format, member.xml) + `<xs:simpleType name="Named"><xs:restriction base="xs:int"/></xs:simpleType><xs:element name="target" type="xs:int"/></xs:schema>`
					root = strings.ReplaceAll(root, "xs:int", "xs:"+atomic)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					spec := owner.spec11
					if profile.version == XSDVersion10 {
						spec = owner.spec10
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, owner.marker, 1) || diagnostic.SpecRef() != spec || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded owner/member = %s/%v", diagnostic, err)
					}
				})
			}
		}
	}
}
