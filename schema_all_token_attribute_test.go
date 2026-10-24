package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // One public query proves both token kinds and the independent particle and use orders.
func TestDirectAllTokenFamilyAttributeFactsAndCopies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				memberBounds, wantBounds := ` minOccurs="0" maxOccurs="1"`, "0/1"
				if profile.policy != Strict10 {
					memberBounds, wantBounds = ` minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"
				}
				root := tokenFamilyLocalRoot(`<xs:complexType name="Box"><xs:all minOccurs="0"><xs:element name="second" type="xs:integer"` + memberBounds + `/><xs:element name="first" type="xs:boolean"/></xs:all><xs:attribute name="built" type="xs:` + kind + `" use="required"/><xs:attribute name="named" type="t:Child"/><xs:attribute name="gone" type="xs:` + kind + `" use="prohibited"/><xs:attribute name="after" type="xs:boolean"/></xs:complexType><xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:whiteSpace value="collapse"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("unstable components: %v", err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))
				if len(found) != 1 || found[0].ID().Source() != "root.xsd" {
					t.Fatalf("owner = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Box")
				all, ok := owner.Particle().(AllParticle)
				if !ok || owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`) || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all minOccurs="0"`) || all.Occurrences().String() != "0/1" {
					t.Fatalf("owner/all = %#v/%#v", owner, owner.Particle())
				}
				members := all.Members()
				if len(members) != 2 || members[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="second"`) || members[0].Occurrences().String() != wantBounds || members[1].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="first"`) {
					t.Fatalf("member order/bounds = %v", members)
				}
				uses := owner.AttributeUses()
				if len(uses) != 3 || uses[0].Name().Local() != "built" || uses[1].Name().Local() != "named" || uses[2].Name().Local() != "after" {
					t.Fatalf("use order = %v", uses)
				}
				for index, lexical := range []string{"xs:" + kind, "t:Child"} {
					use, ok := uses[index].(LocalAttributeUse)
					name := []string{"built", "named"}[index]
					if !ok || use.Name() != mustTestQName(t, "urn:token-local", name) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) {
						t.Fatalf("use %d = %#v", index, uses[index])
					}
					ref, hasRef := use.TypeReference()
					if !hasRef || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
						t.Fatalf("reference %d = %#v/%t", index, ref, hasRef)
					}
					white, hasWhite := ref.StringWhiteSpaceFacet()
					if !hasWhite || white.Value() != "collapse" || !white.Fixed() {
						t.Fatalf("whitespace %d = %#v/%t", index, white, hasWhite)
					}
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if index == 0 {
						if !ref.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) || !white.Loc().IsZero() || ref.StringEnumerationFacets().HasEnumeration() || use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
							t.Fatalf("builtin = %#v", use)
						}
						continue
					}
					if !ref.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, mustTestQName(t, "urn:token-local", "Child")) || id.Source() != "root.xsd" || use.Use() != AttributeUseOptional || !use.UseLoc().IsZero() || white.Loc() != elementReferenceTestAttributeLoc(t, root, `value="collapse"`) || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="t:Parent"`) {
						t.Fatalf("named = %#v", use)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two"}, []Loc{mustTestLoc(t, "root.xsd", 1, strings.LastIndex(root, `value="two"`)+1)})
					values, locations := ref.StringEnumerationFacets().Values(), ref.StringEnumerationFacets().Locations()
					values[0], locations[0] = "changed", Loc{}
					if ref.StringEnumerationFacets().Values()[0] != "two" || ref.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("caller changed enumeration facts")
					}
				}
				uses[0], members[0] = nil, nil
				minimum := all.Members()[0].Occurrences().Minimum()
				minimum.value.SetInt64(9)
				if owner.AttributeUses()[0] == nil || all.Members()[0] == nil || all.Members()[0].Occurrences().String() != wantBounds {
					t.Fatal("caller changed completed facts")
				}
				var walked []ComponentID
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				if len(walked) != len(schema.Components()) {
					t.Fatalf("walk = %v", walked)
				}
				for index, component := range schema.Components() {
					if walked[index] != component.ID() {
						t.Fatalf("walk %d = %v, want %v", index, walked[index], component.ID())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Both consumers independently reject built-in and named effective token types.
func TestDirectAllTokenFamilyAttributeConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"builtin", "named"} {
				t.Run(profile.name+"/"+kind+"/"+shape, func(t *testing.T) {
					lexical, extra := "xs:"+kind, ""
					if shape == "named" {
						lexical = "t:Alias"
						extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`
					}
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box"><xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="a" type="` + lexical + `"/></xs:complexType><xs:element name="root" type="t:Box"/>` + extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
					elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<t:root xmlns:t="urn:token-local" a="one"><t:item>1</t:item></t:root>`)))
					vd := requireDiagnostic(t, instanceErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, errInstanceAttributes) {
						t.Fatalf("validation = %v", instanceErr)
					}
					output, generationErr := GenerateGo(schema, "generated")
					gd := requireDiagnostic(t, generationErr)
					if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation = %v/%v", output, generationErr)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Omission of a particle and of a use are independent for both atomic kinds.
func TestDirectAllTokenFamilyAttributeOmissionAndDependencyOrder(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			model := `<xs:all><xs:element name="goneMember" type="xs:integer" minOccurs="0" maxOccurs="0"/><xs:element name="keptMember" type="xs:boolean"/></xs:all>`
			if profile.policy != Strict10 {
				model = `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="goneMember" type="xs:integer"/></xs:all>`
			}
			root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + model + `<xs:attribute name="goneUse" type="xs:` + kind + `" use="prohibited"/><xs:attribute name="kept" type="xs:` + kind + `"/></xs:complexType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))[0], "Box")
			if profile.policy == Strict10 {
				all, ok := owner.Particle().(AllParticle)
				if !ok || len(all.Members()) != 1 || all.Members()[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="keptMember"`) {
					t.Fatalf("strict10 retained members = %v", owner.Particle())
				}
			}
			if profile.policy != Strict10 && owner.Particle() != nil {
				t.Fatalf("0/0 all = %T, want nil", owner.Particle())
			}
			if uses := owner.AttributeUses(); len(uses) != 1 || uses[0].Name().Local() != "kept" || uses[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="kept"`) {
				t.Fatalf("retained use = %v", uses)
			}
			badUse := strings.Replace(root, `name="goneUse" type="xs:`+kind+`"`, `name="goneUse" type="t:Missing"`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, badUse, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, badUse, `type="t:Missing"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			badParticle := strings.Replace(root, `<xs:element name="goneMember" type="xs:integer"`, `<xs:element ref="t:Missing"`, 1)
			badParticle = strings.Replace(badParticle, `name="kept" type="xs:`+kind+`"`, `name="kept" type="t:AlsoMissing"`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, badParticle, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, elementReferenceTestAttributeLoc(t, badParticle, `ref="t:Missing"`), nil, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved)
		}
	}
}

func TestDirectAllTokenFamilyAttributeKeepsIDCardinality(t *testing.T) {
	for _, kind := range []string{"token", "NMTOKEN"} {
		for _, shape := range []string{"builtin", "named"} {
			lexical, extra := "xs:"+kind, ""
			if shape == "named" {
				lexical = "t:Alias"
				extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			}
			root := tokenFamilyLocalRoot(`<xs:complexType name="Box"><xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="word" type="` + lexical + `"/><xs:attribute name="firstID" type="xs:ID"/><xs:attribute name="goneID" type="xs:ID" use="prohibited"/></xs:complexType>` + extra)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))[0], "Box")
			if uses := owner.AttributeUses(); len(uses) != 2 || uses[0].Name().Local() != "word" || uses[1].Name().Local() != "firstID" {
				t.Fatalf("one ID and token use = %v", uses)
			}
			bad := strings.Replace(root, `use="prohibited"`, `use="required"`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, bad, nil, Strict10)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeUseIDDuplicateCode, elementReferenceTestAttributeLoc(t, bad, `<xs:attribute name="goneID"`), []Loc{elementReferenceTestAttributeLoc(t, bad, `<xs:attribute name="firstID"`)}, "xsd10-structures#cos-ct-props-correct", errSchemaAttributeUseIDDuplicate)
		}
	}
}

func TestDirectAllTokenFamilyAttributeExcludedOwners(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"builtin", "named"} {
				lexical, extra := "xs:"+kind, ""
				if shape == "named" {
					lexical = "t:Alias"
					extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
				}
				use := `<xs:attribute name="a" type="` + lexical + `"/>`
				for _, owner := range []struct {
					name, body, marker, spec string
				}{
					{"anonymous all", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="item" type="xs:integer"/></xs:all>` + use + `</xs:complexType></xs:element>`, `<xs:all>`, schemaSyntaxSpecRefForVersion(XSDVersion10)},
					{"extension all", `<xs:complexType name="Base"/><xs:complexType name="Box"><xs:complexContent><xs:extension base="t:Base"><xs:all><xs:element name="item" type="xs:integer"/></xs:all>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:attribute name="a"`, extensionSpecForNormalizedAll(profile.version)},
				} {
					t.Run(profile.name+"/"+kind+"/"+shape+"/"+owner.name, func(t *testing.T) {
						root := tokenFamilyLocalRoot(owner.body + extra)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, owner.marker), nil, owner.spec, ErrUnsupported)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Particle syntax and references keep priority over the newly admitted use gate.
func TestDirectAllTokenFamilyAttributeParticlePrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, test := range []struct {
				name, model, marker, code, spec string
				class                           FailureClass
				cause                           error
			}{
				{"malformed occurrence", `<xs:all maxOccurs="maybe"><xs:element name="item" type="xs:integer"/></xs:all>`, `maxOccurs="maybe"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil},
				{"invalid outer bound", `<xs:all maxOccurs="2"><xs:element name="item" type="xs:integer"/></xs:all>`, `maxOccurs="2"`, invalidSchemaCompositionCode, schemaAllLimitedSpecRef(profile.version), FailureInvalid, nil},
				{"missing member target", `<xs:all><xs:element ref="t:MissingMember"/></xs:all>`, `ref="t:MissingMember"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), FailureInvalid, errSchemaElementReferenceUnresolved},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + test.model + `<xs:attribute name="a" type="t:Missing"/></xs:complexType>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var causes []error
					if test.cause != nil {
						causes = append(causes, test.cause)
					}
					checkStringAllDiagnostic(t, schema, err, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), nil, test.spec, causes...)
					if test.name == "malformed occurrence" {
						inner := requireDiagnostic(t, errors.Unwrap(requireDiagnostic(t, err)))
						if inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) {
							t.Fatalf("occurrence cause = %v", inner)
						}
					}
				})
			}
		}
	}
}
