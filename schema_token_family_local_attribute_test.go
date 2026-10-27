package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func tokenFamilyLocalRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:token-local" targetNamespace="urn:token-local" attributeFormDefault="qualified">` + body + `</xs:schema>`
}

//nolint:gocognit // Each graph source is checked against public identity, facet, and walk facts.
func TestTokenFamilyLocalAttributeGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"empty", "choice", "sequence"} {
				t.Run(profile.name+"/"+kind+"/"+shape, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:include schemaLocation="part.xsd"/><xs:include schemaLocation="part.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Box">` + tokenFamilyDirectParticle(shape) + `<xs:attribute name="forward" type="t:Forward"/><xs:attribute name="included" type="t:Included"/><xs:attribute name="adopted" type="t:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/></xs:complexType><xs:simpleType name="Forward"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="forward"/></xs:restriction></xs:simpleType>`)
					root = strings.Replace(root, `xmlns:t="urn:token-local"`, `xmlns:t="urn:token-local" xmlns:o="urn:other"`, 1)
					part := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:token-local"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`
					chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="adopted"/></xs:restriction></xs:simpleType></xs:schema>`
					other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="foreign"/></xs:restriction></xs:simpleType></xs:schema>`
					fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "part.xsd": {id: "part.xsd", contents: part}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("nondeterministic graph: %v", err)
					}
					wantSources := []SourceID{"root.xsd", "part.xsd", "chameleon.xsd", "other.xsd"}
					for index, document := range schema.Documents() {
						if index >= len(wantSources) || document.Source() != wantSources[index] {
							t.Fatalf("documents = %v", schema.Documents())
						}
					}
					if len(schema.Documents()) != len(wantSources) {
						t.Fatalf("documents = %v", schema.Documents())
					}
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))
					uses := requireTestComplexTypeDefinition(t, found[0], "Box").AttributeUses()
					if len(uses) != 4 {
						t.Fatalf("uses = %v", uses)
					}
					for index, want := range []struct{ name, lexical, namespace, value, source, content string }{
						{"forward", "t:Forward", "urn:token-local", "forward", "root.xsd", root},
						{"included", "t:Included", "urn:token-local", "included", "part.xsd", part},
						{"adopted", "t:Adopted", "urn:token-local", "adopted", "chameleon.xsd", chameleon},
						{"foreign", "o:Foreign", "urn:other", "foreign", "other.xsd", other},
					} {
						use, ok := uses[index].(LocalAttributeUse)
						if !ok || use.Name() != mustTestQName(t, "urn:token-local", want.name) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+want.name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+want.lexical+`"`) || use.DeclaredType() != mustTestQName(t, want.namespace, strings.TrimPrefix(want.lexical, strings.Split(want.lexical, ":")[0]+":")) {
							t.Fatalf("use %d = %#v", index, use)
						}
						id, hasID := use.TypeID()
						ref, hasRef := use.TypeReference()
						if !hasID || !hasRef || !ref.IsNamed() || id.Source() != SourceID(want.source) || id != componentIDForName(t, schema, use.DeclaredType()) {
							t.Fatalf("use %d target = %v/%t ref=%#v", index, id, hasID, ref)
						}
						assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{want.value}, []Loc{mustSchemaTokenLoc(t, SourceID(want.source), want.content, 1, `value="`+want.value+`"`)})
					}
					var walk []string
					if err := schema.Walk(func(component Component) error {
						walk = append(walk, string(component.ID().Source())+":"+component.Name().Local())
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(walk, []string{"root.xsd:Box", "root.xsd:Forward", "part.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}) {
						t.Fatalf("walk = %v", walk)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep independent phase exits beside their exact location and cause assertions.
func TestTokenFamilyLocalAttributeDiagnosticExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"empty", "choice", "sequence"} {
			for _, test := range []struct {
				name, use, extra, primary, related, code, spec string
				class                                          FailureClass
				cause                                          error
			}{
				{"unresolved", `<xs:attribute name="a" type="t:Missing"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong kind", `<xs:attribute name="a" type="t:Wrong"/>`, `<xs:complexType name="Wrong"/>`, `type="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"cycle", `<xs:attribute name="a" type="t:First"/>`, `<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`, `type="t:First"`, "", diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
				{"invalid NMTOKEN", `<xs:attribute name="a" type="t:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="bad/value"/></xs:restriction></xs:simpleType>`, `value="bad/value"`, "", InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errSchemaNMTOKENValueViolation},
				{"bad use", `<xs:attribute name="a" type="xs:token" use="sometimes"/>`, "", `use="sometimes"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
				{"name/ref", `<xs:attribute name="a" ref="t:b" type="xs:token"/>`, "", `ref="t:b"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
				{"type/inline", `<xs:attribute name="a" type="xs:token"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
				{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), FailureUnsupported, errSchemaAttributeTypeUnsupported},
			} {
				t.Run(profile.name+"/"+ownerShape+"/"+test.name, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + test.use + `</xs:complexType>` + test.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if test.name == "cycle" {
						related := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`), elementReferenceTestAttributeLoc(t, root, `base="t:Second"`), elementReferenceTestAttributeLoc(t, root, `base="t:First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`)}
						assertNCNameDiagnostic(t, schema, err, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec, test.cause)
						return
					}
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					if test.cause == nil {
						if schema.storage != nil || err == nil {
							t.Fatalf("composition returned schema: %v", err)
						}
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), related) {
							t.Fatalf("composition diagnostic = %v", err)
						}
						return
					}
					assertNCNameDiagnostic(t, schema, err, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec, test.cause)
				})
			}
		}
	}
}

func TestTokenFamilyLocalAttributeResolverCause(t *testing.T) {
	for _, ownerShape := range []string{"empty", "choice", "sequence"} {
		root := tokenFamilyLocalRoot(`<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute name="a" type="xs:token"/></xs:complexType>`)
		for _, profile := range tokenPolicyProfiles() {
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("token type source unavailable")
			schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
		}
	}
}

//nolint:gocognit // Each excluded local form has a distinct authored boundary.
func TestTokenFamilyLocalAttributeValueAndVarietyExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, ownerShape := range []string{"empty", "choice", "sequence"} {
				for _, shape := range []string{"direct", "named", "inline"} {
					for _, constraint := range []string{"default", "fixed"} {
						t.Run(profile.name+"/"+kind+"/"+ownerShape+"/"+shape+"/"+constraint, func(t *testing.T) {
							use, extra := tokenFamilyLocalValueUse(kind, shape, constraint)
							root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + use + `</xs:complexType>` + extra)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, constraint+`="one"`), nil, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported)
						})
					}
				}
				for _, variety := range []string{"list", "union"} {
					for _, shape := range []string{"named", "inline"} {
						t.Run(profile.name+"/"+kind+"/"+ownerShape+"/"+variety+"/"+shape, func(t *testing.T) {
							definition := `<xs:` + variety + ` itemType="xs:` + kind + `"/>`
							if variety == "union" {
								definition = `<xs:union memberTypes="xs:` + kind + `"/>`
							}
							use := `<xs:attribute name="a" type="t:Alias"/>`
							extra := `<xs:simpleType name="Alias">` + definition + `</xs:simpleType>`
							marker := `type="t:Alias"`
							if shape == "inline" {
								use = `<xs:attribute name="a"><xs:simpleType>` + definition + `</xs:simpleType></xs:attribute>`
								extra = ""
								marker = `<xs:simpleType>`
							}
							root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + use + `</xs:complexType>` + extra)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, marker), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported)
						})
					}
				}
			}
		}
	}
}

func tokenFamilyLocalValueUse(kind, shape, constraint string) (use, extra string) {
	value := ` ` + constraint + `="one"`
	switch shape {
	case "direct":
		return `<xs:attribute name="a" type="xs:` + kind + `"` + value + `/>`, ""
	case "named":
		return `<xs:attribute name="a" type="t:Alias"` + value + `/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
	case "inline":
		return `<xs:attribute name="a"` + value + `><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, ""
	}
	panic("invalid test shape")
}

func TestTokenFamilyLocalAttributeFacetFailuresAndVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"empty", "choice", "sequence"} {
			for _, test := range []struct {
				name, extra, primary, related, code, spec string
				class                                     FailureClass
				cause                                     error
			}{
				{"NMTOKEN invalid lexical", `<xs:simpleType name="Alias"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="two words"/></xs:restriction></xs:simpleType>`, `value="two words"`, "", InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errSchemaNMTOKENValueViolation},
				{"NMTOKEN narrowing", `<xs:simpleType name="Parent"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="t:Parent"><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`, `value="two"`, `value="one"`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errInvalidEnumerationRestriction},
				{"token narrowing", `<xs:simpleType name="Parent"><xs:restriction base="xs:token"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="t:Parent"><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`, `value="two"`, `value="one"`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errInvalidEnumerationRestriction},
				{"whiteSpace widening", `<xs:simpleType name="Alias"><xs:restriction base="xs:token"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceRestriction},
				{"unsupported length", `<xs:simpleType name="Alias"><xs:restriction base="xs:token"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
			} {
				t.Run(profile.name+"/"+ownerShape+"/"+test.name, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute name="a" type="t:Alias"/></xs:complexType>` + test.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					assertNCNameDiagnostic(t, schema, err, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec, test.cause)
				})
			}
			root := tokenFamilyLocalRoot(`<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute name="a" type="o:Hidden"/></xs:complexType>`)
			root = strings.Replace(root, `xmlns:t="urn:token-local"`, `xmlns:t="urn:token-local" xmlns:o="urn:other"`, 1)
			fixtures := map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
				"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
		}
	}
}

func TestTokenFamilyLocalAttributeDuplicateTypePrecedesAmbiguousUse(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"empty", "choice", "sequence"} {
			root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute name="a" type="t:Alias"/></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:token"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias" id="second"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias"><xs:restriction`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
		}
	}
}

//nolint:gocognit // Each global target shape preserves its own refusal boundary.
func TestTokenFamilyLocalAttributeReferencedTargetsStayExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, ownerShape := range []string{"empty", "choice", "sequence"} {
				for _, targetShape := range []string{"direct", "named", "inline"} {
					t.Run(profile.name+"/"+kind+"/"+ownerShape+"/"+targetShape, func(t *testing.T) {
						target := `<xs:attribute name="a" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
						primary := `ref="t:a"`
						var related []Loc
						spec, cause := schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported
						if targetShape == "direct" {
							target = `<xs:attribute name="a" type="xs:` + kind + `"/>`
						}
						if targetShape == "inline" {
							target = `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`
						}
						root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute ref="t:a"/></xs:complexType>` + target)
						if kind == "token" {
							related = []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}
						}
						if kind == "NMTOKEN" {
							primary, spec, cause = `type="t:Alias"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
							if targetShape == "direct" {
								primary = `type="xs:NMTOKEN"`
							}
							if targetShape == "inline" {
								primary = `<xs:simpleType>`
							}
						}
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, primary), related, spec, cause)
					})
				}
			}
		}
	}
}

func tokenFamilyLocalAlias(kind string) string {
	return `<xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:whiteSpace value="collapse"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`
}

//nolint:gocognit // Check the copied public facts for both atomic kinds under every policy.
func TestTokenFamilyLocalAttributeFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := tokenFamilyLocalRoot(`<xs:complexType name="Box"><xs:attribute name="built" type="xs:` + kind + `"/><xs:attribute name="named" type="t:Child" use="required"/><xs:attribute name="gone" type="xs:` + kind + `" use="prohibited"/></xs:complexType>` + tokenFamilyLocalAlias(kind))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))
				if len(found) != 1 {
					t.Fatalf("Box components = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Box")
				if owner.Particle() != nil {
					t.Fatalf("empty owner particle = %T", owner.Particle())
				}
				uses := owner.AttributeUses()
				if len(uses) != 2 {
					t.Fatalf("ordered uses = %v", uses)
				}
				for index, lexical := range []string{"xs:" + kind, "t:Child"} {
					use, ok := uses[index].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use %d = %T", index, uses[index])
					}
					name := []string{"built", "named"}[index]
					if use.Name() != mustTestQName(t, "urn:token-local", name) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) {
						t.Fatalf("use %d name/location = %#v", index, use)
					}
					ref, ok := use.TypeReference()
					if !ok || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() {
						t.Fatalf("use %d reference = %#v/%t", index, ref, ok)
					}
					white, hasWhite := ref.StringWhiteSpaceFacet()
					if !hasWhite || white.Value() != "collapse" || !white.Fixed() {
						t.Fatalf("use %d whiteSpace = %#v/%t", index, white, hasWhite)
					}
					if index == 0 {
						if !ref.IsBuiltin() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) || ref.StringEnumerationFacets().HasEnumeration() || !white.Loc().IsZero() {
							t.Fatalf("built-in reference = %#v", ref)
						}
						if id, hasID := use.TypeID(); hasID || !id.IsZero() {
							t.Fatalf("built-in type ID = %v/%t", id, hasID)
						}
						continue
					}
					if !ref.IsNamed() || use.DeclaredType() != mustTestQName(t, "urn:token-local", "Child") || use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
						t.Fatalf("named use = %#v", use)
					}
					if white.Loc() != elementReferenceTestAttributeLoc(t, root, `value="collapse"`) {
						t.Fatalf("named whiteSpace location = %s", white.Loc())
					}
					id, hasID := use.TypeID()
					refID, refHasID := ref.ComponentID()
					if !hasID || !refHasID || id != refID || id != componentIDForName(t, schema, mustTestQName(t, "urn:token-local", "Child")) {
						t.Fatalf("named target IDs = %v/%t, %v/%t", id, hasID, refID, refHasID)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two"}, []Loc{mustTestLoc(t, "root.xsd", 1, strings.LastIndex(root, `value="two"`)+1)})
					values := ref.StringEnumerationFacets().Values()
					values[0] = "changed"
					locations := ref.StringEnumerationFacets().Locations()
					locations[0] = Loc{}
					if !reflect.DeepEqual(ref.StringEnumerationFacets().Values(), []string{"two"}) || ref.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("enumeration view mutated schema")
					}
				}
				uses[0] = nil
				if owner.AttributeUses()[0].Name().Local() != "built" {
					t.Fatal("attribute-use view mutated schema")
				}
			})
		}
	}
}

//nolint:gocognit // Each owner and local-use shape needs a separate refusal assertion.
func TestTokenFamilyLocalAttributeExclusions(t *testing.T) {
	owners := []struct{ name, before, after, extra string }{
		{"empty inline", `<xs:element name="root"><xs:complexType>`, `</xs:complexType></xs:element>`, ""},
		{"choice inline", `<xs:element name="root"><xs:complexType><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice>`, `</xs:complexType></xs:element>`, ""},
		{"sequence inline", `<xs:element name="root"><xs:complexType><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence>`, `</xs:complexType></xs:element>`, ""},
		{"all", `<xs:complexType name="Box"><xs:all><xs:element name="item" type="xs:integer"/></xs:all>`, `</xs:complexType>`, ""},
		{"group", `<xs:complexType name="Box"><xs:group ref="t:Fields"/>`, `</xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:group>`},
		{"simple content", `<xs:complexType name="Box"><xs:simpleContent><xs:extension base="xs:string">`, `</xs:extension></xs:simpleContent></xs:complexType>`, ""},
		{"grouped extension", `<xs:complexType name="Base"/><xs:complexType name="Box"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/>`, `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:group>`},
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, owner := range owners {
				for _, shape := range []string{"direct", "named", "inline", "ref"} {
					if owner.name == "grouped extension" && (shape == "direct" || shape == "named") {
						continue
					}
					if owner.name == "group" && shape == "direct" {
						continue
					}
					t.Run(profile.name+"/"+kind+"/"+owner.name+"/"+shape, func(t *testing.T) {
						use, extra, marker, related := tokenFamilyExcludedUse(kind, shape)
						root := tokenFamilyLocalRoot(owner.before + use + owner.after + owner.extra + extra)
						if shape == "ref" && kind == "NMTOKEN" {
							marker, related = `type="xs:NMTOKEN"`, ""
						}
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						var relatedLocs []Loc
						if related != "" {
							relatedLocs = []Loc{elementReferenceTestAttributeLoc(t, root, related)}
						}
						spec, cause := schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported
						if shape == "inline" {
							cause = errSchemaAttributeTypeUnsupported
						}
						if shape == "ref" {
							spec, cause = schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported
						}
						if shape == "ref" && kind == "NMTOKEN" {
							spec, cause = schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
						}
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, marker), relatedLocs, spec, cause)
					})
				}
			}
		}
	}
}

func tokenFamilyExcludedUse(kind, shape string) (use, extra, marker, related string) {
	switch shape {
	case "direct":
		return `<xs:attribute name="a" type="xs:` + kind + `"/>`, "", `type="xs:` + kind + `"`, ""
	case "named":
		return `<xs:attribute name="a" type="t:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `type="t:Alias"`, ""
	case "inline":
		return `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, ""
	case "ref":
		return `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `ref="t:a"`, `<xs:attribute name="a"`
	}
	panic("invalid test shape")
}

//nolint:gocognit // Both consumers reject both admitted kinds under each policy.
func TestTokenFamilyLocalAttributeConsumersRefuse(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := tokenFamilyLocalRoot(`<xs:complexType name="Box"><xs:attribute name="a" type="xs:` + kind + `"/></xs:complexType><xs:element name="root" type="t:Box"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
				ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
				elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<t:root xmlns:t="urn:token-local" a="one"/>`)))
				vd := requireDiagnostic(t, validationErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, errInstanceAttributes) {
					t.Fatalf("validation = %v", validationErr)
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
