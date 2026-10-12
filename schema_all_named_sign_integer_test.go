package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type allNamedSignCase struct {
	name, minimum, maximum, enumeration string
}

func allNamedSignCases() []allNamedSignCase {
	return []allNamedSignCase{
		{"positiveInteger", "3", "20", "10"},
		{"nonPositiveInteger", "-20", "-2", "-10"},
	}
}

func assertNamedSignAllMember(t *testing.T, schema Schema, scalar string) {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
	if len(components) != 1 {
		t.Fatalf("Record components = %d", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record is not a complex type")
	}
	all, ok := definition.Particle().(AllParticle)
	if !ok {
		t.Fatalf("Record particle = %T", definition.Particle())
	}
	members := all.Members()
	if len(members) != 1 {
		t.Fatalf("all members = %v", members)
	}
	member, ok := members[0].(ElementParticle)
	if !ok || member.DeclaredType() != mustTestQName(t, "urn:root", "Alias") {
		t.Fatalf("%s all member = %#v", scalar, members[0])
	}
}

//nolint:gocognit,funlen // The public graph view ties lexical order to type and facet provenance.
func TestDirectAllNamedSignIntegerGraphFacts(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, sign := range allNamedSignCases() {
			t.Run(string(profile.policy)+"/"+sign.name, func(t *testing.T) {
				root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all" version="1.0"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:simpleType name="Direct"><xs:restriction base="xs:%s"><xs:minInclusive value="%s"/><xs:maxInclusive value="%s"/><xs:totalDigits value="2"/><xs:enumeration value="%s"/></xs:restriction></xs:simpleType><xs:complexType name="Record"><xs:all minOccurs="0"><xs:element name="builtin" type="xs:%s"/><xs:element name="direct" type="r:Direct" %s/><xs:element name="forward" type="r:Forward"/><xs:element name="included" type="r:Included"/><xs:element name="imported" type="o:Imported"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element ref="r:global"/><xs:element name="omit" type="r:Direct" minOccurs="0" maxOccurs="0"/></xs:all></xs:complexType><xs:element name="root" type="r:Record"/><xs:element name="global" type="r:Direct"/><xs:simpleType name="Forward"><xs:restriction base="r:Direct"/></xs:simpleType></xs:schema>`, testXSDNamespace, sign.name, sign.minimum, sign.maximum, sign.enumeration, sign.name, profile.rangeIn)
				fixtures := map[string]discoveryFixture{
					"root.xsd":      {id: "root.xsd", contents: root},
					"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:` + sign.name + `"/></xs:simpleType></xs:schema>`},
					"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:` + sign.name + `"/></xs:simpleType></xs:schema>`},
					"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:` + sign.name + `"/></xs:simpleType></xs:schema>`},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("repeated graph differs: %v", err)
				}
				var sources []SourceID
				for _, component := range schema.Components() {
					source := component.ID().Source()
					if len(sources) == 0 || sources[len(sources)-1] != source {
						sources = append(sources, source)
					}
				}
				if !reflect.DeepEqual(sources, []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}) {
					t.Fatalf("source order = %v", sources)
				}
				all := directAllFromSchema(t, schema)
				if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" {
					t.Fatalf("all = %s/%s", all.Loc(), all.Occurrences())
				}
				members := all.Members()
				if len(members) != 7 {
					t.Fatalf("members = %d", len(members))
				}
				builtin, ok := members[0].(ElementParticle)
				if !ok || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, sign.name) {
					t.Fatalf("built-in member = %#v", members[0])
				}
				builtinRef, ok := builtin.TypeReference()
				builtinBounds, hasBounds := builtinRef.IntegerBounds()
				builtinMin, hasMin := builtinBounds.MinInclusiveFacet()
				builtinMax, hasMax := builtinBounds.MaxInclusiveFacet()
				if !ok || !builtinRef.IsBuiltin() || !hasBounds || builtinBounds.Version() != profile.version || sign.name == "positiveInteger" && (!hasMin || hasMax || builtinMin.Value().Canonical() != "1" || !builtinMin.Loc().IsZero()) || sign.name == "nonPositiveInteger" && (hasMin || !hasMax || builtinMax.Value().Canonical() != "0" || !builtinMax.Loc().IsZero()) {
					t.Fatalf("intrinsic bounds = %#v/%v", builtinRef, builtinBounds)
				}
				for index, want := range []struct{ local, namespace, typeName, source, variety string }{
					{"direct", "urn:all", "Direct", "root.xsd", `<xs:restriction base="xs:` + sign.name + `"`},
					{"forward", "urn:all", "Forward", "root.xsd", `<xs:restriction base="r:Direct"`},
					{"included", "urn:all", "Included", "included.xsd", `<xs:restriction base="xs:` + sign.name + `"`},
					{"imported", "urn:other", "Imported", "other.xsd", `<xs:restriction base="xs:` + sign.name + `"`},
					{"chameleon", "urn:all", "Chameleon", "chameleon.xsd", `<xs:restriction base="xs:` + sign.name + `"`},
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
						t.Fatalf("member %s provenance = %#v/%v/%v", want.local, ref, memberID, refID)
					}
					bounds, hasBounds := ref.IntegerBounds()
					minimum, hasMinimum := bounds.MinInclusiveFacet()
					maximum, hasMaximum := bounds.MaxInclusiveFacet()
					wantMin, wantMax := "", ""
					wantMinLoc, wantMaxLoc := Loc{}, Loc{}
					if sign.name == "positiveInteger" {
						wantMin = "1"
					}
					if sign.name == "nonPositiveInteger" {
						wantMax = "0"
					}
					if index < 2 {
						wantMin, wantMax = sign.minimum, sign.maximum
						wantMinLoc = allParticleTestTokenLoc(t, "root.xsd", root, `value="`+sign.minimum+`"`, 1)
						wantMaxLoc = allParticleTestTokenLoc(t, "root.xsd", root, `value="`+sign.maximum+`"`, 1)
					}
					if !hasBounds || bounds.Version() != profile.version || hasMinimum != (wantMin != "") || hasMaximum != (wantMax != "") || hasMinimum && (minimum.Value().Canonical() != wantMin || minimum.Loc() != wantMinLoc) || hasMaximum && (maximum.Value().Canonical() != wantMax || maximum.Loc() != wantMaxLoc) {
						t.Fatalf("member %s bounds = %v/%v", want.local, minimum, maximum)
					}
				}
				refMember, ok := members[6].(ElementReferenceParticle)
				if !ok || refMember.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || refMember.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
					t.Fatalf("reference member = %#v", members[6])
				}
				first, ok := members[1].(ElementParticle)
				if !ok {
					t.Fatalf("first named member = %T", members[1])
				}
				if first.Occurrences().String() != profile.rangeOut {
					t.Fatalf("named occurrence = %s", first.Occurrences())
				}
				definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:all", "Direct"))[0].SimpleTypeDefinition()
				total, hasTotal := definition.DigitFacets().TotalDigits()
				loc, hasLoc := definition.DigitFacets().TotalDigitsLoc()
				values, locations := definition.IntegerEnumerationFacets().Values(), definition.IntegerEnumerationFacets().Locations()
				if !ok || !hasTotal || total.Canonical() != "2" || !hasLoc || loc != allParticleTestTokenLoc(t, "root.xsd", root, `value="2"`, 1) || len(values) != 1 || values[0].Canonical() != sign.enumeration || len(locations) != 1 || locations[0] != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:enumeration value="`+sign.enumeration+`"`, 1) {
					t.Fatalf("facets = %v/%v at %s/%v", total, values, loc, locations)
				}
				values[0].value.SetInt64(99)
				locations[0] = Loc{}
				ref, _ := first.TypeReference()
				bounds, _ := ref.IntegerBounds()
				minimum, _ := bounds.MinInclusiveFacet()
				value := minimum.Value()
				value.value.SetInt64(99)
				occur := first.Occurrences().Minimum()
				occur.value.SetInt64(99)
				members[1] = nil
				fresh, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
				if !ok {
					t.Fatal("copied first member lost element shape")
				}
				freshRef, _ := fresh.TypeReference()
				freshBounds, _ := freshRef.IntegerBounds()
				freshMinimum, _ := freshBounds.MinInclusiveFacet()
				if fresh.Occurrences().String() != profile.rangeOut || freshMinimum.Value().Canonical() != sign.minimum || definition.IntegerEnumerationFacets().Values()[0].Canonical() != sign.enumeration || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatal("caller changed copied facts")
				}
				for run := 0; run < 2; run++ {
					var names []string
					if err := schema.Walk(func(component Component) error { names = append(names, component.Name().Local()); return nil }); err != nil || len(names) == 0 || names[0] != "Direct" {
						t.Fatalf("walk %d = %v/%v", run, names, err)
					}
				}
				assertDirectAllConsumerRejection(t, schema, all, profile.version)
			})
		}
	}
}

//nolint:gocognit // Omission, refs, and alternate diagnostics form one policy matrix.
func TestDirectAllNamedSignIntegerOmissionAndBoundary(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, sign := range allNamedSignCases() {
			extra := `<xs:simpleType name="Named"><xs:restriction base="xs:` + sign.name + `"/></xs:simpleType><xs:element name="global" type="r:Named"/><xs:element name="root" type="r:Record"/>`
			t.Run(string(profile.policy)+"/"+sign.name+"/omit", func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all><xs:element name="gone" type="r:Named" minOccurs="0" maxOccurs="0"/><xs:element ref="r:global"/><xs:element name="keep" type="r:Named"/></xs:all>`, extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				members := directAllFromSchema(t, schema).Members()
				if len(members) != 2 {
					t.Fatalf("omission/order = %#v", members)
				}
				keep, ok := members[1].(ElementParticle)
				if !ok || keep.DeclaredType() != mustTestQName(t, "urn:all", "Named") {
					t.Fatalf("surviving named member = %#v", members[1])
				}
				ref, ok := members[0].(ElementReferenceParticle)
				if !ok || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
					t.Fatalf("reference = %#v", members[0])
				}
				assertDirectAllConsumerRejection(t, schema, directAllFromSchema(t, schema), profile.version)
			})
			for _, failure := range []allSignFailure{
				{"named duplicate", `<xs:element name="v" type="r:Named"/><xs:element name="v" type="r:Named"/>`, extra, `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, 1, FailureInvalid, errSchemaAllMemberDuplicate, false},
				{"named bad occurrence", `<xs:element name="v" type="r:Named" maxOccurs="many"/>`, extra, `maxOccurs="many"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, 0, FailureInvalid, nil, true},
			} {
				t.Run(string(profile.policy)+"/"+sign.name+"/"+failure.name, func(t *testing.T) { assertAllSignFailure(t, profile, failure) })
			}
		}
	}
}

//nolint:gocognit // Compare the two edition exits with the same named member fixture.
func TestDirectAllNamedSignIntegerOuterZeroAndStrict10MemberGate(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, sign := range allNamedSignCases() {
			t.Run(string(profile.policy)+"/"+sign.name, func(t *testing.T) {
				extra := `<xs:simpleType name="Named"><xs:restriction base="xs:` + sign.name + `"/></xs:simpleType>`
				root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="r:Named"/></xs:all>`, extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.policy == Strict10 {
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="0"`, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 outer zero = %s, cause %v", diagnostic, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))
				if len(components) != 1 {
					t.Fatalf("Record components = %d", len(components))
				}
				definition, ok := components[0].ComplexTypeDefinition()
				if !ok || definition.Particle() != nil {
					t.Fatalf("outer zero published %T", definition.Particle())
				}
			})
		}
	}
	profile := allSignProfiles()[1]
	for _, sign := range allNamedSignCases() {
		t.Run("Strict10/"+sign.name+"/repeated member", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:Named" maxOccurs="2"/></xs:all>`, `<xs:simpleType name="Named"><xs:restriction base="xs:`+sign.name+`"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="2"`, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10 repeated member = %s, cause %v", diagnostic, err)
			}
		})
	}
}

func TestDirectAllNamedSignIntegerExcludedOwners(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, sign := range allNamedSignCases() {
			extensionSpec := "xsd11-structures#cos-ct-extends"
			if profile.version == XSDVersion10 {
				extensionSpec = "xsd10-structures#cos-ct-extends"
			}
			for _, test := range []struct {
				name, body, marker, spec string
				occurrence               int
			}{
				{"inline owner", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="r:Named"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, schemaSyntaxSpecRefForVersion(XSDVersion10), 1},
				{"named group", `<xs:group name="Group"><xs:all><xs:element name="v" type="r:Named"/></xs:all></xs:group>`, `<xs:all>`, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), 1},
				{"extension", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="r:Named"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, extensionSpec, 1},
				{"local attribute", `<xs:complexType name="Record"><xs:all><xs:element name="v" type="r:Named"/></xs:all><xs:attribute name="a" type="r:Named"/></xs:complexType>`, `type="r:Named"`, schemaAttributeTypeSpecRef(profile.version), 2},
			} {
				t.Run(string(profile.policy)+"/"+sign.name+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all"><xs:simpleType name="Named"><xs:restriction base="xs:` + sign.name + `"/></xs:simpleType>` + test.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, test.occurrence) || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded owner = %s, spec %s want %s, loc %s want %s, cause %v", diagnostic, diagnostic.SpecRef(), test.spec, diagnostic.Loc(), allParticleTestTokenLoc(t, "root.xsd", root, test.marker, test.occurrence), err)
					}
				})
			}
		}
	}
}
