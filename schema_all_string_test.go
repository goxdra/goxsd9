package goxsd9

import (
	"errors"
	"testing"
)

//nolint:gocognit // Public type identity, order, occurrence, location, and copied views belong to one admission boundary.
func TestDirectAllBuiltinStringFactsAndConsumers(t *testing.T) {
	profiles := []struct {
		name, bounds, wantBounds string
		policy                   LanguagePolicy
		version                  XSDVersion
	}{
		{"compatibility", `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616", Compatibility, XSDVersion11},
		{"strict10", `minOccurs="0" maxOccurs="1"`, "0/1", Strict10, XSDVersion10},
		{"strict11", `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded", Strict11, XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">
  <xs:element name="root" type="r:Record"/>
  <xs:complexType name="Record"><xs:all minOccurs="0">
    <xs:element name="first" type="xs:integer"/>
    <xs:element name="text" type="xs:string" ` + profile.bounds + `/>
    <xs:element ref="r:global"/>
    <xs:element name="last" type="xs:boolean"/>
  </xs:all></xs:complexType>
  <xs:element name="global" type="xs:string"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse direct all string: %v", err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" || len(members) != 4 {
				t.Fatalf("all facts = %s/%s/%d, want lexical all, 0/1, four members", all.Loc(), all.Occurrences(), len(members))
			}
			for index, want := range []string{"first", "text", "global", "last"} {
				name, _ := schemaAllMemberNameAndLoc(members[index])
				if name.Local() != want {
					t.Fatalf("member %d name = %s, want %s", index, name, want)
				}
			}
			text, ok := members[1].(ElementParticle)
			if !ok {
				t.Fatalf("text member = %T, want ElementParticle", members[1])
			}
			wantName := mustTestQName(t, testXSDNamespace, "string")
			if text.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="text"`, 1) || text.Name() != mustTestQName(t, "", "text") || text.Occurrences().String() != profile.wantBounds || text.DeclaredType() != wantName {
				t.Fatalf("text facts = %s/%s/%s/%s, want lexical text and xs:string", text.Loc(), text.Name(), text.Occurrences(), text.DeclaredType())
			}
			reference, ok := text.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.IsNamed() || reference.Name() != wantName || reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:string"`, 1) {
				t.Fatalf("text type reference = %#v/%t, want located built-in xs:string", reference, ok)
			}
			if _, hasID := text.TypeID(); hasID {
				t.Fatal("built-in string gained a component ID")
			}
			if _, hasID := reference.ComponentID(); hasID {
				t.Fatal("built-in string reference gained a component ID")
			}
			space, ok := reference.StringWhiteSpaceFacet()
			if !ok || space.Value() != "preserve" {
				t.Fatalf("string whiteSpace = %q/%t, want preserve", space.Value(), ok)
			}
			global, ok := members[2].(ElementReferenceParticle)
			if !ok || global.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || global.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
				t.Fatalf("global member = %#v, want located element reference", members[2])
			}
			minimum := text.Occurrences().Minimum()
			minimum.value.SetInt64(9)
			if maximum, finite := text.Occurrences().Maximum().Finite(); finite {
				maximum.value.SetInt64(9)
			}
			members[1] = nil
			again, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !ok || again.Occurrences().String() != profile.wantBounds || again.DeclaredType() != wantName {
				t.Fatalf("caller changed all string facts: %#v", again)
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Check child and owner omission across all applicable language policies.
func TestDirectAllBuiltinStringZeroOmission(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="gone" type="xs:string" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:string"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse omitted string: %v", err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("member count = %d, want one", len(members))
			}
			keep, ok := members[0].(ElementParticle)
			if !ok || keep.Name().Local() != "keep" || keep.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="keep"`, 1) {
				t.Fatalf("retained member = %#v, want keep", members[0])
			}
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="gone" type="xs:string"/></xs:all>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s omitted all: %v", policy, err)
		}
		definition, ok := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
		if !ok || definition.Particle() != nil {
			t.Fatalf("%s omitted owner = %#v", policy, definition.Particle())
		}
	}
}

//nolint:gocognit // Adjacent invalid and unsupported exits must retain their own diagnostics.
func TestDirectAllBuiltinStringAdjacentFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, members, extra, primary, related, code, spec string
			class                                              FailureClass
			cause                                              error
			primaryOccurrence                                  int
		}{
			{"duplicate", `<xs:element name="text" type="xs:string"/><xs:element name="text" type="xs:string"/>`, "", `<xs:element name="text"`, `<xs:element name="text"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), FailureInvalid, errSchemaAllMemberDuplicate, 2},
			{"named string", `<xs:element name="text" type="r:Text"/>`, `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Text"`, "", UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), FailureUnsupported, errSchemaAllMemberScalar, 1},
			{"inline string", `<xs:element name="text"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), FailureUnsupported, ErrUnsupported, 1},
			{"invalid maximum", `<xs:element name="text" type="xs:string" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil, 1},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.members+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.primaryOccurrence) || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, cause %v", diagnostic, err)
				}
				if test.related == "" {
					if len(diagnostic.Related()) != 0 {
						t.Fatalf("unexpected related locations: %v", diagnostic.Related())
					}
					return
				}
				if len(diagnostic.Related()) != 1 || diagnostic.Related()[0] != allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1) {
					t.Fatalf("related locations = %v", diagnostic.Related())
				}
			})
		}
	}
}
