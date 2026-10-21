package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // The graph fixture checks the copied public facts of every admitted shape.
func TestDirectAllNormalizedStringGraphFacts(t *testing.T) {
	for _, profile := range []struct {
		policy             LanguagePolicy
		version            XSDVersion
		bounds, wantBounds string
	}{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all" version="1.0">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Record"><xs:all minOccurs="0">
    <xs:element name="builtin" type="xs:normalizedString"/>
    <xs:element name="direct" type="r:Direct" %s/>
    <xs:element name="inherited" type="r:Inherited"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="imported" type="o:Imported"/>
    <xs:element name="chameleon" type="r:Chameleon"/>
    <xs:element ref="r:neighbor"/>
  </xs:all></xs:complexType>
  <xs:element name="root" type="r:Record"/>
  <xs:element name="neighbor" type="xs:integer"/>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Direct"/></xs:simpleType>
  <xs:simpleType name="Direct"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>
</xs:schema>`, testXSDNamespace, profile.bounds)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if schema.LanguagePolicy() != profile.policy {
				t.Fatalf("policy = %s", schema.LanguagePolicy())
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("unstable graph facts/order: %v", err)
			}
			documents := schema.Documents()
			for index, source := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
				if len(documents) != 4 || documents[index].Source() != source {
					t.Fatalf("document order = %#v", documents)
				}
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" || len(members) != 7 {
				t.Fatalf("all facts = %s/%s/%d", all.Loc(), all.Occurrences(), len(members))
			}
			for index, want := range []struct{ local, typeName, namespace, source, variety, value, valueSource, valueMarker string }{
				{"direct", "Direct", "urn:all", "root.xsd", `<xs:restriction base="xs:normalizedString"`, "a\nb", "root.xsd", `value="a&#xA;b"`},
				{"inherited", "Inherited", "urn:all", "root.xsd", `<xs:restriction base="r:Direct"`, "a\nb", "root.xsd", `value="a&#xA;b"`},
				{"included", "Included", "urn:all", "included.xsd", `<xs:restriction base="xs:normalizedString"`, "a b", "included.xsd", `value="a b"`},
				{"imported", "Imported", "urn:other", "other.xsd", `<xs:restriction base="xs:normalizedString"`, "", "", ""},
				{"chameleon", "Chameleon", "urn:all", "chameleon.xsd", `<xs:restriction base="xs:normalizedString"`, "", "", ""},
			} {
				member, ok := members[index+1].(ElementParticle)
				name := mustTestQName(t, want.namespace, want.typeName)
				if !ok || member.Name() != mustTestQName(t, "", want.local) || member.DeclaredType() != name || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+want.local+`"`, 1) {
					t.Fatalf("member %d = %#v", index+1, members[index+1])
				}
				reference, hasReference := member.TypeReference()
				id, hasID := member.TypeID()
				refID, hasRefID := reference.ComponentID()
				prefix := "r:"
				if want.namespace == "urn:other" {
					prefix = "o:"
				}
				declaration := root
				if want.source != "root.xsd" {
					declaration = fixtures[want.source].contents
				}
				if !hasReference || !reference.IsNamed() || reference.Name() != name || reference.QName() != name || reference.Variety() != SimpleTypeVarietyAtomicRestriction || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, name) || id.Source() != SourceID(want.source) || reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="`+prefix+want.typeName+`"`, 1) || reference.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, want.variety, 1) {
					t.Fatalf("%s type provenance = %#v/%v/%v", want.local, reference, id, refID)
				}
				whiteSpace, hasWhiteSpace := reference.StringWhiteSpaceFacet()
				wantWhiteSpace := "replace"
				wantWhiteSpaceLoc := Loc{}
				if want.local == "included" {
					wantWhiteSpace = "collapse"
					wantWhiteSpaceLoc = allParticleTestTokenLoc(t, "included.xsd", declaration, `value="collapse"`, 1)
				}
				if !hasWhiteSpace || whiteSpace.Value() != wantWhiteSpace || whiteSpace.Loc() != wantWhiteSpaceLoc || len(reference.StringEnumerationFacets().Values()) != len(reference.StringEnumerationFacets().Locations()) {
					t.Fatalf("%s effective facets = %#v/%v", want.local, whiteSpace, reference.StringEnumerationFacets())
				}
				if want.value != "" {
					values := reference.StringEnumerationFacets().Values()
					locations := reference.StringEnumerationFacets().Locations()
					valueInput := root
					if want.valueSource != "root.xsd" {
						valueInput = fixtures[want.valueSource].contents
					}
					if len(values) == 0 || values[0] != want.value || locations[0] != allParticleTestTokenLoc(t, SourceID(want.valueSource), valueInput, want.valueMarker, 1) {
						t.Fatalf("%s enumeration = %v at %v", want.local, values, locations)
					}
				}
			}
			builtin, ok := members[0].(ElementParticle)
			builtinRef, hasBuiltinRef := builtin.TypeReference()
			if !ok || !hasBuiltinRef || !builtinRef.IsBuiltin() || builtin.Name() != mustTestQName(t, "", "builtin") || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "normalizedString") || builtin.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="builtin"`, 1) || builtinRef.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:normalizedString"`, 1) || builtinRef.QName() != builtin.DeclaredType() {
				t.Fatalf("builtin facts = %#v/%#v", builtin, builtinRef)
			}
			assertNormalizedStringReference(t, builtinRef, builtinRef.Loc())
			if id, hasID := builtin.TypeID(); hasID || !id.IsZero() {
				t.Fatalf("builtin ID = %v/%t", id, hasID)
			}
			if got := members[1].Occurrences().String(); got != profile.wantBounds {
				t.Fatalf("member range = %s", got)
			}
			neighbor, isRef := members[6].(ElementReferenceParticle)
			if !isRef || neighbor.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:neighbor"`, 1) || neighbor.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "neighbor"))[0].ID() {
				t.Fatalf("neighbor = %#v", members[6])
			}
			direct, isDirect := members[1].(ElementParticle)
			if !isDirect {
				t.Fatalf("direct member = %T", members[1])
			}
			directRef, _ := direct.TypeReference()
			values := directRef.StringEnumerationFacets().Values()
			locations := directRef.StringEnumerationFacets().Locations()
			values[0] = "changed"
			locations[0] = Loc{}
			minimum := direct.Occurrences().Minimum()
			minimum.value.SetInt64(9)
			if maximum, finite := direct.Occurrences().Maximum().Finite(); finite {
				maximum.value.SetInt64(9)
			}
			members[1] = nil
			fresh, isFresh := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !isFresh {
				t.Fatal("fresh direct member lost element shape")
			}
			freshRef, _ := fresh.TypeReference()
			if fresh.Occurrences().String() != profile.wantBounds || !reflect.DeepEqual(freshRef.StringEnumerationFacets().Values(), []string{"a\nb", " a  b "}) || freshRef.StringEnumerationFacets().Locations()[0].IsZero() || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller mutation changed completed all facts")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

func TestDirectAllNormalizedStringOmissionAndReferences(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="builtinDrop" type="xs:normalizedString" minOccurs="0" maxOccurs="0"/><xs:element name="namedDrop" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:normalizedString"/><xs:element ref="r:target"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType><xs:element name="target" type="xs:normalizedString"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			keep, ok := members[0].(ElementParticle)
			ref, hasRef := members[1].(ElementReferenceParticle)
			if len(members) != 2 || !ok || keep.Name().Local() != "keep" || !hasRef || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:target"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "target"))[0].ID() {
				t.Fatalf("omission/ref facts = %#v", members)
			}
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="value" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
		if definition.Particle() != nil {
			t.Fatalf("%s 0/0 all published %T", policy, definition.Particle())
		}
	}
}

func TestDirectAllNormalizedStringConsumerShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []struct{ name, member, extra string }{
			{"builtin", `<xs:element name="v" type="xs:normalizedString"/>`, ""},
			{"named", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`},
			{"inherited", `<xs:element name="v" type="r:Child"/>`, `<xs:simpleType name="Child"><xs:restriction base="r:Named"/></xs:simpleType><xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`},
			{"reference", `<xs:element ref="r:target"/>`, `<xs:element name="target" type="xs:normalizedString"/>`},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+shape.member+`</xs:all>`, `<xs:element name="root" type="r:Record"/>`+shape.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				all := directAllFromSchema(t, schema)
				if len(all.Members()) != 1 {
					t.Fatalf("surviving member count = %d", len(all.Members()))
				}
				assertDirectAllConsumerRejection(t, schema, all, profile.version)
			})
		}
	}
}

//nolint:gocognit // Alternate exits retain code, locations, cause, and edition reference.
func TestDirectAllNormalizedStringBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, member, tail, primary, related, code, spec string
			primaryOccurrence                                int
			class                                            FailureClass
			cause                                            error
		}{
			{"zero unresolved", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeUnresolved},
			{"zero wrong kind", `<xs:element name="v" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:normalizedString"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaElementTypeWrongKind},
			{"zero invalid facet", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), 1, FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"zero unsupported facet", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), 1, FailureUnsupported, ErrUnsupported},
			{"zero cycle", `<xs:element name="v" type="r:First" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `base="r:Second"`, `base="r:First"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), 1, FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"duplicate", `<xs:element name="v" type="xs:normalizedString"/><xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, FailureInvalid, errSchemaAllMemberDuplicate},
			{"invalid occurrence", `<xs:element name="v" type="xs:normalizedString" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, FailureInvalid, nil},
			{"invalid range", `<xs:element name="v" type="xs:normalizedString" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="v"`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), 1, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.tail)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.primaryOccurrence)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %v, cause %v", test.name, d, err)
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1)}
				}
				if test.name == "invalid range" {
					related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="1"`, 1))
				}
				if !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("%s related = %v, want %v", test.name, d.Related(), related)
				}
			})
		}
		if profile.policy == Strict10 {
			for _, test := range []struct{ name, model, marker, code string }{
				{"repeated member", `<xs:all><xs:element name="v" type="xs:normalizedString" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
				{"zero repeated member", `<xs:all><xs:element name="v" type="xs:normalizedString" minOccurs="0" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode},
				{"zero outer", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:normalizedString"/></xs:all>`, `maxOccurs="0"`, UnsupportedSchemaSyntaxCode},
			} {
				t.Run(profile.name+"/"+test.name, func(t *testing.T) {
					root := allParticleTestRoot(test.model, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || d.SpecRef() != "xsd11-structures#cSchemaDocument" || len(d.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 all limit = %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Excluded local, owner, and attribute shapes are independent boundaries.
func TestDirectAllNormalizedStringExcludedShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []struct {
			name, body, marker, spec string
			cause                    error
		}{
			{"anonymous builtin derivative", `<xs:complexType name="Record"><xs:all><xs:element name="v"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:element></xs:all></xs:complexType>`, `<xs:simpleType`, schemaSyntaxSpecRefForVersion(profile.version), ErrUnsupported},
			{"anonymous named derivative", `<xs:complexType name="Record"><xs:all><xs:element name="v"><xs:simpleType><xs:restriction base="r:Named"/></xs:simpleType></xs:element></xs:all></xs:complexType>`, `<xs:simpleType`, schemaSyntaxSpecRefForVersion(profile.version), ErrUnsupported},
			{"inline complex member", `<xs:complexType name="Record"><xs:all><xs:element name="v"><xs:complexType/></xs:element></xs:all></xs:complexType>`, `<xs:complexType/>`, schemaSyntaxSpecRefForVersion(profile.version), ErrUnsupported},
			{"inline owner builtin", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="xs:normalizedString"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, schemaSyntaxSpecRefForVersion(XSDVersion10), ErrUnsupported},
			{"inline owner named", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="r:Named"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, schemaSyntaxSpecRefForVersion(XSDVersion10), ErrUnsupported},
			{"extension builtin", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="xs:normalizedString"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, extensionSpecForNormalizedAll(profile.version), ErrUnsupported},
			{"extension named", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="r:Named"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, extensionSpecForNormalizedAll(profile.version), ErrUnsupported},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all"><xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>` + shape.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				occurrence := 1
				if shape.name == "anonymous builtin derivative" || shape.name == "anonymous named derivative" {
					occurrence = 2
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, shape.marker, occurrence) || d.SpecRef() != shape.spec || len(d.Related()) != 0 || !errors.Is(err, shape.cause) {
					t.Fatalf("excluded %s = %v; code=%s loc=%s want=%s spec=%s want=%s related=%v cause=%v", shape.name, err, d.Code(), d.Loc(), allParticleTestTokenLoc(t, "root.xsd", root, shape.marker, 1), d.SpecRef(), shape.spec, d.Related(), d.Unwrap())
				}
			})
		}
		for _, attribute := range []struct {
			name, local, extra, marker, spec string
			cause                            error
		}{
			{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"ref", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:normalizedString"/>`, `ref="r:a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(profile.name+"/attribute "+attribute.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all"><xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType><xs:complexType name="Record"><xs:all><xs:element name="v" type="xs:normalizedString"/></xs:all>` + attribute.local + `</xs:complexType>` + attribute.extra + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				occurrence := 1
				if attribute.name == "direct" || attribute.name == "inline" {
					occurrence = 2
				}
				var related []Loc
				if attribute.name == "ref" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `<xs:attribute name="a"`, 1)}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, attribute.marker, occurrence) || d.SpecRef() != attribute.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, attribute.cause) {
					t.Fatalf("excluded attribute %s = %v; code=%s loc=%s want=%s spec=%s want=%s related=%v cause=%v", attribute.name, err, d.Code(), d.Loc(), allParticleTestTokenLoc(t, "root.xsd", root, attribute.marker, 1), d.SpecRef(), attribute.spec, d.Related(), d.Unwrap())
				}
			})
		}
	}
}

func extensionSpecForNormalizedAll(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#cos-ct-extends"
	}
	return "xsd11-structures#cos-ct-extends"
}

func TestDirectAllNormalizedStringHiddenGraphTypeRemainsUnresolved(t *testing.T) {
	assertAllIntegerHiddenGraphTypeUnresolved(t, "normalizedString")
}

//nolint:gocognit // Both edition paths assert the public all-member boundary.
func TestDirectAllNormalizedStringTargetNamespacePolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="qualified" type="xs:normalizedString" targetNamespace="urn:all"/><xs:element name="omitted" type="xs:normalizedString" targetNamespace="urn:all" minOccurs="0" maxOccurs="0"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `targetNamespace="urn:all"`, 2) || d.SpecRef() != schemaSyntaxSpecRefForVersion(XSDVersion11) || len(d.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("Strict10 targetNamespace = %v; code=%s loc=%s want=%s spec=%s related=%v cause=%v", err, d.Code(), d.Loc(), allParticleTestTokenLoc(t, "root.xsd", root, `targetNamespace="urn:all"`, 1), d.SpecRef(), d.Related(), d.Unwrap())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			qualified, ok := members[0].(ElementParticle)
			if len(members) != 1 || !ok || qualified.Name() != mustTestQName(t, "urn:all", "qualified") {
				t.Fatalf("targetNamespace/0/0 facts = %#v", members)
			}
		})
	}
}
