package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Public member facts and graph provenance are one admission boundary.
func TestDirectAllNamedTokenGraphFactsAndConsumers(t *testing.T) {
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
  <xs:simpleType name="Direct"><xs:restriction base="xs:token"><xs:enumeration value=" first "/><xs:enumeration value="second"/></xs:restriction></xs:simpleType>
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
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:simpleType name="Included"><xs:restriction base="xs:token"><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:token"><xs:enumeration value="chameleon"/></xs:restriction></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:token"><xs:enumeration value="imported"/></xs:restriction></xs:simpleType></xs:schema>`},
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
				{"direct", "urn:all", "Direct", "root.xsd", `<xs:restriction base="xs:token"`, " first ", "root.xsd", `value=" first "`},
				{"forward", "urn:all", "Forward", "root.xsd", `<xs:restriction base="r:Direct"`, " first ", "root.xsd", `value=" first "`},
				{"included", "urn:all", "Included", "included.xsd", `<xs:restriction base="xs:token"`, "included", "included.xsd", `value="included"`},
				{"imported", "urn:other", "Imported", "other.xsd", `<xs:restriction base="xs:token"`, "imported", "other.xsd", `value="imported"`},
				{"chameleon", "urn:all", "Chameleon", "chameleon.xsd", `<xs:restriction base="xs:token"`, "chameleon", "chameleon.xsd", `value="chameleon"`},
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

//nolint:gocognit // Exercise child/owner omission and retained reference shape across policies.
func TestDirectAllNamedTokenOmissionAndReferenceShape(t *testing.T) {
	named := `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy)+"/member", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="gone" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="r:Named"/></xs:all>`, named)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("members = %#v, want keep", members)
			}
			if member := requireLocalTokenElementParticle(t, members[0], 0); member.Name().Local() != "keep" {
				t.Fatalf("member = %s, want keep", member.Name())
			}
		})
		t.Run(string(policy)+"/ref", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element ref="r:word"/></xs:all>`, named+`<xs:element name="word" type="r:Named"/><xs:element name="root" type="r:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			member, ok := directAllFromSchema(t, schema).Members()[0].(ElementReferenceParticle)
			if !ok || member.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:word"`, 1) {
				t.Fatalf("ref = %#v", member)
			}
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+"/owner", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="gone" type="r:Named"/></xs:all>`, named)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			definition, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
			if definition.Particle() != nil {
				t.Fatalf("omitted owner particle = %T", definition.Particle())
			}
		})
	}
}

//nolint:gocognit // Each invalid or excluded exit has its own code, cause, and location.
func TestDirectAllNamedTokenInvalidAndExcludedForms(t *testing.T) {
	for _, test := range []struct {
		name, model, extra, primary, code string
		class                             FailureClass
		cause                             error
	}{
		{"unresolved", `<xs:all><xs:element name="word" type="r:Missing"/></xs:all>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, FailureInvalid, errSchemaElementTypeUnresolved},
		{"wrong kind", `<xs:all><xs:element name="word" type="r:Wrong"/></xs:all>`, `<xs:element name="Wrong" type="xs:token"/>`, `type="r:Wrong"`, diagnosticSchemaElementTypeWrongKindCode, FailureInvalid, errSchemaElementTypeWrongKind},
		{"duplicate", `<xs:all><xs:element name="word" type="r:Named"/><xs:element name="word" type="r:Named"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`, `<xs:element name="word" type="r:Named"`, diagnosticSchemaElementReferenceDuplicateCode, FailureInvalid, errSchemaAllMemberDuplicate},
		{"inline", `<xs:all><xs:element name="word"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:element></xs:all>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, FailureUnsupported, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, test.extra)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("invalid all returned schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			occurrence := 1
			if test.name == "duplicate" {
				occurrence = 2
			}
			if diagnostic.Class() != test.class || test.code != "" && diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, occurrence) || test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("%s diagnostic = %s, cause %v", test.name, diagnostic, err)
			}
			wantSpec := schemaElementTypeSpecRef(XSDVersion11)
			if test.name == "duplicate" {
				wantSpec = schemaAllLimitedSpecRef(XSDVersion11)
				if related := diagnostic.Related(); len(related) != 1 || related[0] != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) {
					t.Fatalf("duplicate related = %v, want first member", related)
				}
			}
			if test.name == "wrong kind" {
				if related := diagnostic.Related(); len(related) != 1 || related[0] != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="Wrong"`, 1) {
					t.Fatalf("wrong-kind related = %v, want declaration", related)
				}
			}
			if test.name == "inline" {
				wantSpec = newSchemaSyntaxUnsupportedForVersion(Loc{}, "", XSDVersion11).SpecRef()
				if !errors.Is(err, ErrUnsupported) {
					t.Fatalf("inline error lost unsupported cause: %v", err)
				}
			}
			if diagnostic.SpecRef() != wantSpec {
				t.Fatalf("%s spec = %q, want %q", test.name, diagnostic.SpecRef(), wantSpec)
			}
		})
	}
}

func TestDirectAllNamedTokenOccurrenceAndZeroSemanticGates(t *testing.T) {
	for _, test := range []struct {
		name, member, extra, primary, code, spec string
		policy                                   XSDVersion
		cause                                    error
	}{
		{"strict10 repeat", `<xs:element name="word" type="r:Named" maxOccurs="2"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode, "xsd11-structures#cSchemaDocument", XSDVersion10, errLanguagePolicyMismatch},
		{"invalid bound", `<xs:element name="word" type="r:Named" maxOccurs="maybe"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`, `maxOccurs="maybe"`, invalidSchemaCompositionCode, "xsd11-datatypes#nonNegativeInteger", XSDVersion11, nil},
		{"zero unresolved", `<xs:element name="word" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(XSDVersion11), XSDVersion11, errSchemaElementTypeUnresolved},
		{"zero wrong kind", `<xs:element name="word" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:token"/>`, `type="r:Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(XSDVersion11), XSDVersion11, errSchemaElementTypeWrongKind},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := Strict11
			if test.policy == XSDVersion10 {
				policy = Strict10
			}
			root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err == nil {
				t.Fatal("invalid named token all returned schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("occurrence/zero gate = %s, want code %s at %s with %s: %v", diagnostic, test.code, test.primary, test.spec, err)
			}
		})
	}
}

//nolint:gocognit // Verify visibility and zero-occurrence cycle exits under each graph policy.
func TestDirectAllNamedTokenGraphVisibilityAndCycle(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{
		{Compatibility, XSDVersion11},
		{Strict10, XSDVersion10},
		{Strict11, XSDVersion11},
	} {
		t.Run(string(profile.policy)+"/hidden import", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" xmlns:h="urn:hidden" targetNamespace="urn:all">
  <xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>
  <xs:complexType name="Record"><xs:all><xs:element name="word" type="h:Hidden"/></xs:all></xs:complexType>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
				"hidden.xsd": {id: "hidden.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err == nil {
				t.Fatal("indirectly imported token was visible")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="h:Hidden"`, 1) || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
				t.Fatalf("hidden token diagnostic = %s, want located unresolved without hidden relation: %v", diagnostic, err)
			}
		})
		t.Run(string(profile.policy)+"/zero cycle", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="word" type="r:One" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("omitted named token cycle returned schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `base="r:Two"`, 1) || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("cycle diagnostic = %s, want located named-type cycle: %v", diagnostic, err)
			}
			if related := diagnostic.Related(); len(related) != 1 || related[0] != allParticleTestTokenLoc(t, "root.xsd", root, `base="r:One"`, 1) {
				t.Fatalf("cycle related = %v, want second base", related)
			}
		})
	}
}
