package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func tokenFamilyDirectParticle(shape string) string {
	switch shape {
	case "empty":
		return ""
	case "choice":
		return `<xs:choice><xs:element name="first" type="xs:integer" minOccurs="2" maxOccurs="3"/><xs:element name="gone" type="xs:integer" minOccurs="0" maxOccurs="0"/></xs:choice>`
	case "sequence":
		return `<xs:sequence><xs:element name="first" type="xs:integer" minOccurs="2" maxOccurs="3"/><xs:element name="gone" type="xs:integer" minOccurs="0" maxOccurs="0"/></xs:sequence>`
	}
	panic("unknown test shape")
}

//nolint:gocognit,funlen // The matrix checks each admitted kind and authored owner through public views.
func TestTokenFamilyDirectAttributeFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+shape+"/"+kind, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(shape) + `<xs:attribute name="built" type="xs:` + kind + `" use="required"/><xs:attribute name="named" type="t:Child"/><xs:attribute name="mixed" type="xs:boolean"/><xs:attribute name="goneUse" type="xs:` + kind + `" use="prohibited"/></xs:complexType>` + tokenFamilyLocalAlias(kind))
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
					}
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))
					if len(found) != 1 {
						t.Fatalf("Box = %v", found)
					}
					owner := requireTestComplexTypeDefinition(t, found[0], "Box")
					if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`) {
						t.Fatalf("owner Loc = %s", owner.Loc())
					}
					if owner.Particle().Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:`+shape) || owner.Particle().Occurrences().String() != "1/1" {
						t.Fatalf("outer particle = %T/%s", owner.Particle(), owner.Particle().Occurrences())
					}
					var children []Particle
					switch particle := owner.Particle().(type) {
					case ChoiceParticle:
						if shape != "choice" {
							t.Fatalf("particle = %T", particle)
						}
						children = particle.Alternatives()
					case SequenceParticle:
						if shape != "sequence" {
							t.Fatalf("particle = %T", particle)
						}
						children = particle.Particles()
					default:
						t.Fatalf("particle = %T", particle)
					}
					if len(children) != 1 || children[0].Occurrences().String() != "2/3" || children[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="first"`) {
						t.Fatalf("children = %v", children)
					}
					uses := owner.AttributeUses()
					if len(uses) != 3 {
						t.Fatalf("ordered uses = %v", uses)
					}
					for i, lexical := range []string{"xs:" + kind, "t:Child"} {
						name := []string{"built", "named"}[i]
						use, ok := uses[i].(LocalAttributeUse)
						if !ok || use.Name() != mustTestQName(t, "urn:token-local", name) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) {
							t.Fatalf("use %d = %#v", i, use)
						}
						ref, ok := use.TypeReference()
						if !ok || ref.Loc() != use.TypeLoc() || ref.QName() != use.DeclaredType() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
							t.Fatalf("ref %d = %#v/%t", i, ref, ok)
						}
						white, ok := ref.StringWhiteSpaceFacet()
						if !ok || white.Value() != "collapse" || !white.Fixed() {
							t.Fatalf("whiteSpace %d = %#v/%t", i, white, ok)
						}
						id, hasID := use.TypeID()
						refID, refHasID := ref.ComponentID()
						if i == 0 {
							if !ref.IsBuiltin() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) || hasID || refHasID || !id.IsZero() || !refID.IsZero() || !white.Loc().IsZero() || ref.StringEnumerationFacets().HasEnumeration() || use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
								t.Fatalf("builtin facts = %#v", use)
							}
							continue
						}
						if !ref.IsNamed() || use.DeclaredType() != mustTestQName(t, "urn:token-local", "Child") || !hasID || !refHasID || id != refID || id != componentIDForName(t, schema, use.DeclaredType()) || id.Source() != "root.xsd" || white.Loc() != elementReferenceTestAttributeLoc(t, root, `value="collapse"`) {
							t.Fatalf("named facts = %#v", use)
						}
						assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two"}, []Loc{mustTestLoc(t, "root.xsd", 1, strings.LastIndex(root, `value="two"`)+1)})
						values, locations := ref.StringEnumerationFacets().Values(), ref.StringEnumerationFacets().Locations()
						values[0], locations[0] = "changed", Loc{}
						if !reflect.DeepEqual(ref.StringEnumerationFacets().Values(), []string{"two"}) || ref.StringEnumerationFacets().Locations()[0].IsZero() {
							t.Fatal("enumeration view mutated")
						}
					}
					mixed, ok := uses[2].(LocalAttributeUse)
					if !ok || mixed.Name().Local() != "mixed" || mixed.DeclaredType() != mustTestQName(t, testXSDNamespace, "boolean") {
						t.Fatalf("mixed use = %#v", uses[2])
					}
					uses[0] = nil
					if owner.AttributeUses()[0].Name().Local() != "built" {
						t.Fatal("use view mutated")
					}
					var walk []string
					if err := schema.Walk(func(c Component) error { walk = append(walk, c.Name().Local()); return nil }); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(walk, []string{"Box", "Parent", "Child"}) {
						t.Fatalf("walk = %v", walk)
					}
				})
			}
		}
	}
}

//nolint:gocognit // The two consumers independently reject each newly admitted owner and kind.
func TestTokenFamilyDirectAttributeConsumersReject(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+shape+"/"+kind, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(shape) + `<xs:attribute name="a" type="xs:` + kind + `"/></xs:complexType><xs:element name="root" type="t:Box"/>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`)
					elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					wantValidationRelated := []Loc{elementLoc, ownerLoc, useLoc}
					if shape == "sequence" {
						wantValidationRelated = []Loc{elementLoc, ownerLoc, elementReferenceTestAttributeLoc(t, root, `<xs:sequence`), useLoc}
					}
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<t:root xmlns:t="urn:token-local" a="one"/>`)))
					vd := requireDiagnostic(t, validationErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), wantValidationRelated) || !errors.Is(validationErr, errInstanceAttributes) {
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
}

//nolint:gocognit // Each direct owner and excluded authored use is checked separately.
func TestTokenFamilyDirectAttributeInlineAndReferenceExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				for _, shape := range []string{"inline", "ref"} {
					t.Run(profile.name+"/"+ownerShape+"/"+kind+"/"+shape, func(t *testing.T) {
						use, extra, marker, related := tokenFamilyExcludedUse(kind, shape)
						root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + use + `</xs:complexType>` + extra)
						spec, cause := schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
						if shape == "ref" {
							spec, cause = schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported
						}
						if shape == "ref" && kind == "NMTOKEN" {
							marker, related = `type="xs:NMTOKEN"`, ""
							spec, cause = schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
						}
						var relatedLocs []Loc
						if related != "" {
							relatedLocs = []Loc{elementReferenceTestAttributeLoc(t, root, related)}
						}
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, marker), relatedLocs, spec, cause)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Authored direct shape survives valid particle and use omission.
func TestTokenFamilyDirectAttributeOmissionAndResolutionOrder(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+shape+"/"+kind, func(t *testing.T) {
					particle := `<xs:` + shape + ` minOccurs="0" maxOccurs="0"><xs:element name="gone" type="xs:integer"/></xs:` + shape + `>`
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + particle + `<xs:attribute name="goneUse" type="xs:` + kind + `" use="prohibited"/><xs:attribute name="kept" type="xs:` + kind + `"/></xs:complexType>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:token-local", "Box"))
					if len(found) != 1 {
						t.Fatalf("Box = %v", found)
					}
					owner := requireTestComplexTypeDefinition(t, found[0], "Box")
					uses := owner.AttributeUses()
					if owner.Particle() != nil || len(uses) != 1 || uses[0].Name().Local() != "kept" || uses[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="kept"`) {
						t.Fatalf("omitted facts = %T/%v", owner.Particle(), uses)
					}
					badUse := strings.Replace(root, `name="goneUse" type="xs:`+kind+`"`, `name="goneUse" type="t:Missing"`, 1)
					schema, err = discoverTestSchemaWithPolicy(t, badUse, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, badUse, `type="t:Missing"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
					badParticle := strings.Replace(root, `<xs:element name="gone" type="xs:integer"`, `<xs:element ref="t:Missing"`, 1)
					badParticle = strings.Replace(badParticle, `type="xs:`+kind+`"`, `type="t:AlsoMissing"`, 1)
					schema, err = discoverTestSchemaWithPolicy(t, badParticle, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, elementReferenceTestAttributeLoc(t, badParticle, `ref="t:Missing"`), nil, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved)
				})
			}
		}
	}
}

//nolint:gocognit // Authored named facets must fail before owner admission for both derivation depths.
func TestTokenFamilyDirectAttributeNewFacetExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				for _, facet := range []string{"pattern", "length", "minLength", "maxLength"} {
					for _, depth := range []string{"direct", "inherited"} {
						t.Run(profile.name+"/"+ownerShape+"/"+kind+"/"+facet+"/"+depth, func(t *testing.T) {
							value := "2"
							if facet == "pattern" {
								value = ".*"
							}
							extra := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"><xs:` + facet + ` value="` + value + `"/></xs:restriction></xs:simpleType>`
							if depth == "inherited" {
								extra = `<xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `"/></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="t:Parent"><xs:` + facet + ` value="` + value + `"/></xs:restriction></xs:simpleType>`
							}
							root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute name="a" type="t:Alias"/></xs:complexType>` + extra)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:`+facet), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
						})
					}
				}
			}
		}
	}
}

func TestTokenFamilyExcludedOwnerResolvesTypeBeforeGate(t *testing.T) {
	owners := []struct{ name, before, after string }{
		{"all", `<xs:complexType name="Box"><xs:all><xs:element name="item" type="xs:integer"/></xs:all>`, `</xs:complexType>`},
		{"anonymous choice", `<xs:element name="root"><xs:complexType><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice>`, `</xs:complexType></xs:element>`},
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, owner := range owners {
			t.Run(profile.name+"/"+owner.name, func(t *testing.T) {
				root := tokenFamilyLocalRoot(owner.before + `<xs:attribute name="a" type="t:Missing"/>` + owner.after)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="t:Missing"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
		}
	}
}

func TestTokenDirectAttributeReferenceValueConstraintsStayExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"choice", "sequence"} {
			for _, constraint := range []string{"default", "fixed"} {
				t.Run(profile.name+"/"+ownerShape+"/"+constraint, func(t *testing.T) {
					root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + `<xs:attribute ref="t:a" ` + constraint + `="one"/></xs:complexType><xs:attribute name="a" type="xs:token"/>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, constraint+`="one"`), nil, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported)
				})
			}
		}
	}
}

//nolint:gocognit // Facet diagnostics precede inline/ref exclusion at each authored source.
func TestTokenFamilyDirectAttributeFacetShapePrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, ownerShape := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				for _, facet := range []string{"pattern", "length", "minLength", "maxLength"} {
					for _, typeShape := range []string{"inline", "ref"} {
						t.Run(profile.name+"/"+ownerShape+"/"+kind+"/"+facet+"/"+typeShape, func(t *testing.T) {
							value := "2"
							if facet == "pattern" {
								value = ".*"
							}
							restriction := `<xs:restriction base="xs:` + kind + `"><xs:` + facet + ` value="` + value + `"/></xs:restriction>`
							use := `<xs:attribute name="a"><xs:simpleType>` + restriction + `</xs:simpleType></xs:attribute>`
							extra := ""
							if typeShape == "ref" {
								use = `<xs:attribute ref="t:a"/>`
								extra = `<xs:attribute name="a" type="t:Alias"/><xs:simpleType name="Alias">` + restriction + `</xs:simpleType>`
							}
							root := tokenFamilyLocalRoot(`<xs:complexType name="Box">` + tokenFamilyDirectParticle(ownerShape) + use + `</xs:complexType>` + extra)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:`+facet), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
						})
					}
				}
			}
		}
	}
}
