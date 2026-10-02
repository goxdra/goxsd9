package goxsd9

import (
	"errors"
	"testing"
)

//nolint:gocognit // Compare edition-selected bounds, locations, copied facts, and both consumers.
func TestDirectAllBuiltinTokenFactsAndConsumers(t *testing.T) {
	profiles := []struct {
		name, tokenBounds, wantBounds string
		policy                        LanguagePolicy
		version                       XSDVersion
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
    <xs:element name="word" type="xs:token" ` + profile.tokenBounds + `/>
    <xs:element name="last" type="xs:boolean"/>
  </xs:all></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse direct all token: %v", err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" || len(members) != 3 {
				t.Fatalf("all location/range/members = %s/%s/%d, want lexical all, 0/1, and 3", all.Loc(), all.Occurrences(), len(members))
			}
			for index, want := range []string{"first", "word", "last"} {
				member, ok := members[index].(ElementParticle)
				if !ok || member.Name().Local() != want {
					t.Fatalf("member %d = %#v, want %q", index, members[index], want)
				}
			}
			word, ok := members[1].(ElementParticle)
			if !ok {
				t.Fatalf("word member = %T, want ElementParticle", members[1])
			}
			if word.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="word"`, 1) || word.Occurrences().String() != profile.wantBounds {
				t.Fatalf("word location/range = %s/%s, want lexical element/%s", word.Loc(), word.Occurrences(), profile.wantBounds)
			}
			if word.DeclaredType() != mustTestQName(t, testXSDNamespace, "token") {
				t.Fatalf("word declared type = %s, want xs:token", word.DeclaredType())
			}
			reference, ok := word.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:token"`, 1) || reference.Name() != word.DeclaredType() {
				t.Fatalf("word type reference = %#v/%t, want located built-in token", reference, ok)
			}
			if _, hasID := word.TypeID(); hasID {
				t.Fatal("built-in token gained a component ID")
			}
			space, ok := reference.StringWhiteSpaceFacet()
			if !ok || space.Value() != "collapse" {
				t.Fatalf("token whiteSpace = %q/%t, want collapse", space.Value(), ok)
			}
			minimum := word.Occurrences().Minimum()
			minimum.value.SetInt64(9)
			if maximum, finite := word.Occurrences().Maximum().Finite(); finite {
				maximum.value.SetInt64(9)
			}
			members[1] = nil
			if got := all.Members()[1].Occurrences().String(); got != profile.wantBounds {
				t.Fatalf("caller mutation changed word range to %s", got)
			}
			copiedWord, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !ok {
				t.Fatal("copied word member lost element shape")
			}
			if got := copiedWord.DeclaredType(); got != word.DeclaredType() {
				t.Fatalf("caller mutation changed word type to %s", got)
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Each excluded local form must retain its own located failure.
func TestDirectAllTokenAdmissionExclusions(t *testing.T) {
	tests := []struct {
		name, member, extra, primary string
	}{
		{"named token", `<xs:element name="word" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`, `type="r:Named"`},
		{"inline token", `<xs:element name="word"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`},
		{"built-in NMTOKEN", `<xs:element name="word" type="xs:NMTOKEN"/>`, "", `type="xs:NMTOKEN"`},
		{"built-in string", `<xs:element name="word" type="xs:string"/>`, "", `type="xs:string"`},
	}
	for _, test := range tests {
		for _, profile := range []struct {
			policy  LanguagePolicy
			version XSDVersion
		}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
			t.Run(test.name+"/"+string(profile.policy), func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("excluded all member returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := schemaAllLimitedSpecRef(profile.version)
				if test.name == "inline token" {
					wantSpec = newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef()
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded member diagnostic = %s, want located unsupported with spec %s", diagnostic, wantSpec)
				}
				if test.name != "inline token" && !errors.Is(err, errSchemaAllMemberScalar) {
					t.Fatalf("excluded member lost scalar cause: %v", err)
				}
			})
		}
	}
}

func TestDirectAllTokenReferenceRetainsReferenceShape(t *testing.T) {
	root := allParticleTestRoot(`<xs:all><xs:element ref="r:word"/></xs:all>`, `<xs:element name="word" type="xs:token"/><xs:element name="root" type="r:Record"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("parse token reference: %v", err)
	}
	member := directAllFromSchema(t, schema).Members()[0]
	reference, ok := member.(ElementReferenceParticle)
	if !ok || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:word"`, 1) {
		t.Fatalf("token reference = %#v, want located reference particle", member)
	}
	if reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "word"))[0].ID() {
		t.Fatal("token reference lost target identity")
	}
	assertDirectAllConsumerRejection(t, schema, directAllFromSchema(t, schema), XSDVersion11)
}

//nolint:gocognit // Check both omission boundaries and same-boundary alternate failures.
func TestDirectAllTokenOmissionAndFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy)+"/zero member", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="word" type="xs:token" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:boolean"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse omitted token: %v", err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("omitted token members = %#v, want keep only", members)
			}
			keep, ok := members[0].(ElementParticle)
			if !ok || keep.Name().Local() != "keep" {
				t.Fatalf("omitted token members = %#v, want keep only", members)
			}
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+"/zero owner", func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="word" type="xs:token"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse omitted all: %v", err)
			}
			definition, _ := schema.Components()[0].ComplexTypeDefinition()
			if definition.Particle() != nil {
				t.Fatalf("omitted owner particle = %T, want nil", definition.Particle())
			}
		})
	}
	failures := []struct {
		name, model, primary, code, spec string
		policy                           LanguagePolicy
		class                            FailureClass
		cause                            error
		related                          []string
	}{
		{"strict10 repeated token", `<xs:all><xs:element name="word" type="xs:token" maxOccurs="2"/></xs:all>`, `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode, "xsd11-structures#cSchemaDocument", Strict10, FailureUnsupported, errLanguagePolicyMismatch, nil},
		{"invalid token bound", `<xs:all><xs:element name="word" type="xs:token" maxOccurs="maybe"/></xs:all>`, `maxOccurs="maybe"`, invalidSchemaCompositionCode, "xsd11-datatypes#nonNegativeInteger", Strict11, FailureInvalid, nil, nil},
		{"token reversed bounds", `<xs:all><xs:element name="word" type="xs:token" minOccurs="2" maxOccurs="1"/></xs:all>`, `<xs:element name="word"`, invalidSchemaCompositionCode, "xsd11-structures#coss-particle", Strict11, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
		{"duplicate token", `<xs:all><xs:element name="word" type="xs:token"/><xs:element name="word" type="xs:token"/></xs:all>`, `<xs:element name="word" type="xs:token"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(XSDVersion11), Strict11, FailureInvalid, errSchemaAllMemberDuplicate, []string{`<xs:element name="word" type="xs:token"`}},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("invalid token all returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			occurrence := 1
			if test.name == "duplicate token" {
				occurrence = 2
			}
			primary := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, occurrence)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != primary || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("token failure = %s, want %s/%s at %s with %s and cause %v", diagnostic, test.class, test.code, primary, test.spec, test.cause)
			}
			if test.name == "invalid token bound" {
				var inner Diagnostic
				if !errors.As(errors.Unwrap(diagnostic), &inner) || inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != primary {
					t.Fatalf("invalid token occurrence lost lexical cause: %v", errors.Unwrap(diagnostic))
				}
			}
			if len(diagnostic.Related()) != len(test.related) {
				t.Fatalf("token failure related = %v, want %d locations", diagnostic.Related(), len(test.related))
			}
			for index, marker := range test.related {
				related := allParticleTestTokenLoc(t, "root.xsd", root, marker, 1)
				if diagnostic.Related()[index] != related {
					t.Fatalf("token failure related[%d] = %s, want %s", index, diagnostic.Related()[index], related)
				}
			}
		})
	}
}
