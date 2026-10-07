package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // The public view must preserve ordered graph and datatype provenance together.
func TestDirectAllByteGraphFacts(t *testing.T) {
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
<xs:simpleType name="Direct"><xs:restriction base="xs:byte"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/><xs:enumeration value="7"/></xs:restriction></xs:simpleType>
<xs:complexType name="Record"><xs:all><xs:element name="builtin" type="xs:byte" form="qualified" nillable="true" block="substitution"/><xs:element name="direct" type="r:Direct" %s/><xs:element name="forward" type="r:Forward"/><xs:element name="included" type="r:Included"/><xs:element name="imported" type="o:Imported"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element ref="r:reference"/></xs:all></xs:complexType>
<xs:element name="root" type="r:Record"/><xs:element name="reference" type="xs:byte"/><xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType></xs:schema>`, testXSDNamespace, profile.rangeText)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:byte"/></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:byte"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:byte"/></xs:simpleType></xs:schema>`},
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
			if !ok || builtin.Name() != mustTestQName(t, "urn:all", "builtin") || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "byte") || builtin.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="builtin"`, 1) || !builtin.IsNillable() || !reflect.DeepEqual(builtin.DisallowedSubstitutions(), []string{"substitution"}) || builtin.DisallowedSubstitutionsLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `block="substitution"`, 1) {
				t.Fatalf("builtin = %#v", members[0])
			}
			builtinRef, ok := builtin.TypeReference()
			builtinID, hasBuiltinID := builtin.TypeID()
			refID, hasRefID := builtinRef.ComponentID()
			builtinLoc := allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:byte"`, 1)
			if !ok || !builtinRef.IsBuiltin() || builtinRef.Name() != mustTestQName(t, testXSDNamespace, "byte") || builtinRef.QName() != builtinRef.Name() || builtinRef.Loc() != builtinLoc || builtinRef.VarietyLoc() != builtinLoc || hasBuiltinID || hasRefID || !builtinID.IsZero() || !refID.IsZero() {
				t.Fatalf("builtin type provenance = %#v/%v/%v", builtinRef, builtinID, refID)
			}
			assertAllIntegerBounds(t, builtinRef, profile.version, "-128", "127", Loc{}, Loc{})
			assertByteParticleBuiltinBounds(t, builtinRef, profile.version)
			assertAllIntegerNamedGraphMembers(t, schema, members, root, fixtures, profile.version, "byte", "-128", "127")
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

type byteAllDiagnosticCase struct {
	name, member, definitions, primary, related, code, spec string
	primaryOccurrence                                       int
	class                                                   FailureClass
	cause                                                   error
}

func checkByteAllDiagnostic(t *testing.T, policy LanguagePolicy, test byteAllDiagnosticCase) {
	t.Helper()
	root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.definitions)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	primary := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.primaryOccurrence)
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != primary || diagnostic.SpecRef() != test.spec {
		t.Fatalf("%s diagnostic = %s; want %s/%s at %s with %s", test.name, diagnostic, test.class, test.code, primary, test.spec)
	}
	var related []Loc
	if test.related != "" {
		related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1)}
	}
	if !reflect.DeepEqual(diagnostic.Related(), related) {
		t.Fatalf("%s related locations = %v; want %v", test.name, diagnostic.Related(), related)
	}
	if test.cause != nil && !errors.Is(err, test.cause) {
		t.Fatalf("%s lost cause %v: %v", test.name, test.cause, err)
	}
	if test.name == "invalid occurrence lexical" && diagnostic.Unwrap() == nil {
		t.Fatal("invalid occurrence lost lexical cause")
	}
}

func TestDirectAllByteOmissionAndDiagnostics(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy)+"/member omission", func(t *testing.T) {
			model := `<xs:all><xs:element name="drop" type="xs:byte" minOccurs="0" maxOccurs="0"/><xs:element name="namedDrop" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element name="inline" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element><xs:element name="keep" type="xs:byte"/></xs:all>`
			definitions := `<xs:simpleType name="Named"><xs:restriction base="xs:byte"/></xs:simpleType>`
			schema, err := discoverTestSchemaWithPolicy(t, allParticleTestRoot(model, definitions), nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("0/0 member count = %d", len(members))
			}
			kept, ok := members[0].(ElementParticle)
			if !ok || kept.Name().Local() != "keep" {
				t.Fatalf("surviving member = %#v", members[0])
			}
		})
		for _, test := range []byteAllDiagnosticCase{
			{"malformed type QName", `<xs:element name="v" type="r:bad:Byte"/>`, "", `type="r:bad:Byte"`, "", invalidSchemaConditionalCode, "", 1, FailureInvalid, nil},
			{"inline anonymous byte", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named string", `<xs:element name="v" type="r:Text"/>`, `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Text"`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), 1, FailureUnsupported, errSchemaAllMemberScalar},
			{"built-in int", `<xs:element name="v" type="xs:int"/>`, "", `type="xs:int"`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), 1, FailureUnsupported, errSchemaAllMemberScalar},
			{"inline complex", `<xs:element name="v"><xs:complexType/></xs:element>`, "", `<xs:complexType/>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named list", `<xs:element name="v" type="r:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:byte"/></xs:simpleType>`, `type="r:List"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"named union", `<xs:element name="v" type="r:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:byte"/></xs:simpleType>`, `type="r:Union"`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1, FailureUnsupported, ErrUnsupported},
			{"unresolved type at zero", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong kind", `<xs:element name="v" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:byte"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"duplicate types", `<xs:element name="v" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:simpleType name="Alias"`, `<xs:simpleType name="Alias"`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), 2, FailureInvalid, errSchemaGlobalDeclarationDuplicate},
			{"invalid byte facet at zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:byte"><xs:maxInclusive value="128"/></xs:restriction></xs:simpleType>`, `value="128"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"invalid occurrence lexical", `<xs:element name="v" type="xs:byte" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, FailureInvalid, nil},
			{"malformed byte facet at zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:byte"><xs:maxInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, boundSpecRef(profile.version, BoundMaxInclusive, boundDefinitionRule), 1, FailureInvalid, errInvalidBoundValue},
			{"inline invalid facet at zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:byte"><xs:maxInclusive value="128"/></xs:restriction></xs:simpleType></xs:element>`, "", `value="128"`, "", InvalidBoundRestrictionCode, boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
			{"cyclic base at zero", `<xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, `base="r:Two"`, `base="r:One"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"duplicate all names", `<xs:element name="v" type="xs:byte"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"unresolved element ref", `<xs:element ref="r:missing"/>`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceUnresolved},
			{"unresolved element ref at zero", `<xs:element ref="r:missing" minOccurs="0" maxOccurs="0"/>`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceUnresolved},
			{"wrong kind element ref", `<xs:element ref="r:Wrong"/>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:byte"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.version), 1, FailureInvalid, errSchemaElementReferenceWrongKind},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) { checkByteAllDiagnostic(t, profile.policy, test) })
		}
	}
}

//nolint:gocognit // Edition-specific omission and policy exits share the same byte boundary.
func TestDirectAllByteOwnerZeroAndPolicyLimits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+"/owner zero", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:byte"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
			if definition.Particle() != nil {
				t.Fatalf("0/0 all owner published %T", definition.Particle())
			}
		})
		for _, test := range []byteAllDiagnosticCase{
			{"unresolved child before owner omission", `<xs:element name="v" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(XSDVersion11), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"bad byte facet before owner omission", `<xs:element name="v" type="r:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:byte"><xs:maxInclusive value="128"/></xs:restriction></xs:simpleType>`, `value="128"`, "", InvalidBoundRestrictionCode, boundSpecRef(XSDVersion11, BoundMaxInclusive, boundRestrictionRule), 1, FailureInvalid, errInvalidBoundRestriction},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0">`+test.member+`</xs:all>`, test.definitions)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("0/0 owner child diagnostic = %s/%v", d, err)
				}
			})
		}
	}
	for _, test := range []struct{ name, model, marker, code string }{
		{"repeated byte", `<xs:all><xs:element name="v" type="xs:byte" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
		{"zero repeated byte", `<xs:all><xs:element name="v" type="xs:byte" minOccurs="0" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
		{"zero all", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:byte"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || d.SpecRef() != "xsd11-structures#cSchemaDocument" || len(d.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10 byte occurrence diagnostic = %s/%v", d, err)
			}
		})
	}
}

//nolint:gocognit // Each member shape exercises both independent consumer boundaries.
func TestDirectAllByteConsumersByMemberShape(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, sample := range []struct{ label, member, definition string }{
			{"built-in byte", `<xs:element name="v" type="xs:byte"/>`, ""},
			{"named byte", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:byte"/></xs:simpleType>`},
			{"ref to byte", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:byte"/>`},
			{"ref to excluded int", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:int"/>`},
		} {
			t.Run(string(profile.policy)+"/"+sample.label, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+sample.member+`</xs:all>`, `<xs:element name="root" type="r:Record"/>`+sample.definition)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				all := directAllFromSchema(t, schema)
				members := all.Members()
				if len(members) != 1 {
					t.Fatalf("member count = %d", len(members))
				}
				if sample.label == "ref to byte" || sample.label == "ref to excluded int" {
					reference, ok := members[0].(ElementReferenceParticle)
					if !ok || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:target"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "target"))[0].ID() {
						t.Fatalf("ref facts = %#v", members[0])
					}
				}
				assertDirectAllConsumerRejection(t, schema, all, profile.version)
			})
		}
	}
}

func TestDirectAllByteHiddenGraphTypeIsUnresolved(t *testing.T) {
	assertAllIntegerHiddenGraphTypeUnresolved(t, "byte")
}

//nolint:dupl // Bounded integer categories check their own public owner exclusions.
func TestDirectAllByteOtherOwnerShapesRemainUnsupported(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct{ name, body, primary, spec10, spec11 string }{
			{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="xs:byte"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, "xsd10-structures#schema-document", "xsd10-structures#schema-document"},
			{"extension owner", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="xs:byte"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends"},
			{"named group", `<xs:group name="Group"><xs:all><xs:element name="v" type="xs:byte"/></xs:all></xs:group>`, `<xs:all>`, "xsd10-structures#schema-document", "xsd11-structures#cSchemaDocument"},
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
					t.Fatalf("excluded byte owner diagnostic = %s/%v; want %s, spec %q, related %v", diagnostic, err, allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1), wantSpec, diagnostic.Related())
				}
			})
		}
	}
}
