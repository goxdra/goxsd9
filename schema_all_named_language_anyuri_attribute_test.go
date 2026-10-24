package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit // The all member and attribute orders and bounds are independent public facts.
func TestNamedLanguageAnyURIDirectAllOrderAndBounds(t *testing.T) {
	for _, test := range []struct {
		policy                 LanguagePolicy
		firstBounds, wantFirst string
	}{
		{Compatibility, ` minOccurs="2" maxOccurs="3"`, "2/3"},
		{Strict10, ` minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, ` minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		for _, kind := range []string{"language", "anyURI"} {
			t.Run(string(test.policy)+"/"+kind, func(t *testing.T) {
				model := `<xs:all minOccurs="0" maxOccurs="1"><xs:element name="zMember" type="xs:integer"` + test.firstBounds + `/><xs:element name="aMember" type="xs:boolean"/></xs:all>`
				uses := `<xs:attribute name="aUse" type="r:Alias" use="required"/><xs:attribute name="zUse" type="xs:boolean"/>`
				root := languageUseRoot(languageAnyURIAllOwner(model, uses) + `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
				schema, err := parseIDUseSchema(t, root, nil, test.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				all, ok := owner.Particle().(AllParticle)
				if !ok || all.Occurrences().String() != "0/1" || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all`) {
					t.Fatalf("all = %#v", owner.Particle())
				}
				members := all.Members()
				if len(members) != 2 {
					t.Fatalf("members = %#v", members)
				}
				first, firstOK := members[0].(ElementParticle)
				second, secondOK := members[1].(ElementParticle)
				if !firstOK || !secondOK || first.Name().Local() != "zMember" || second.Name().Local() != "aMember" || members[0].Occurrences().String() != test.wantFirst || members[1].Occurrences().String() != "1/1" {
					t.Fatalf("members = %#v", members)
				}
				if members[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="zMember"`) || members[1].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="aMember"`) {
					t.Fatalf("member locations = %s/%s", members[0].Loc(), members[1].Loc())
				}
				attributes := owner.AttributeUses()
				if len(attributes) != 2 || attributes[0].Name().Local() != "aUse" || attributes[1].Name().Local() != "zUse" {
					t.Fatalf("attribute order = %#v", attributes)
				}
				use, ok := attributes[0].(LocalAttributeUse)
				if !ok || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="aUse"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`) || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
					t.Fatalf("named use = %#v", attributes[0])
				}
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				if !hasRef || !hasID || !ref.IsNamed() || ref.IsBuiltin() || ref.QName() != mustTestQName(t, "urn:root", "Alias") || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="xs:`+kind+`"`) || id != componentIDForName(t, schema, ref.QName()) {
					t.Fatalf("named type = %#v/%v/%t", ref, id, hasID)
				}
				members[0] = nil
				attributes[0] = nil
				if all.Members()[0] == nil || owner.AttributeUses()[0] == nil {
					t.Fatal("caller modified all member or attribute use")
				}
				again, repeatErr := parseIDUseSchema(t, root, nil, test.policy)
				if repeatErr != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("unstable completed facts: %v", repeatErr)
				}
			})
		}
	}
}

func TestNamedLanguageAnyURIDirectAllOtherOwnersExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			use := `<xs:attribute name="value" type="r:Alias"/>`
			for _, test := range []struct{ name, body, marker, spec string }{
				{"all extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="item" type="xs:integer"/></xs:all>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:attribute name="value"`, extensionSpecForNormalizedAll(languageUseVersion(policy))},
				{"anonymous all", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="item" type="xs:integer"/></xs:all>` + use + `</xs:complexType></xs:element>`, `<xs:all>`, schemaSyntaxSpecRefForVersion(languageUseVersion(policy))},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(test.body + alias)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, test.marker), nil, test.spec, ErrUnsupported)
				})
			}
		}
	}
}

//nolint:gocognit // Outer and prohibited-use omission must retain earlier particle failures.
func TestNamedLanguageAnyURIDirectAllZeroAndResolutionPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+kind, func(t *testing.T) {
				owner := languageAnyURIAllOwner(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="member" type="xs:integer"/></xs:all>`, `<xs:attribute name="kept" type="r:Alias"/><xs:attribute name="gone" type="r:Alias" use="prohibited"/>`)
				root := languageUseRoot(owner + `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				definition := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				if definition.Particle() != nil || len(definition.AttributeUses()) != 1 || definition.AttributeUses()[0].Name().Local() != "kept" {
					t.Fatalf("zero owner facts = %T/%v", definition.Particle(), definition.AttributeUses())
				}
				broken := languageUseRoot(languageAnyURIAllOwner(`<xs:all minOccurs="0" maxOccurs="0"><xs:element ref="r:missing"/></xs:all>`, `<xs:attribute name="gone" type="r:Missing" use="prohibited"/>`))
				badSchema, badErr := parseIDUseSchema(t, broken, nil, policy)
				assertZeroSchema(t, badSchema)
				d := requireDiagnostic(t, badErr)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, broken, `ref="r:missing"`) || d.SpecRef() != schemaElementReferenceSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(badErr, errSchemaElementReferenceUnresolved) {
					t.Fatalf("particle before omitted use = %v", badErr)
				}
			})
		}
	}
}
