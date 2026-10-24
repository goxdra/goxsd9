package goxsd9

import (
	"errors"
	"strings"
	"testing"
)

func tokenAllGroupBoundaryRoot(kind, shape, owner, model string, target bool) string {
	lexical, extra := "xs:"+kind, ""
	if shape == "named" {
		lexical = "t:Alias"
		extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
	}
	body := model + `<xs:attribute name="a" type="` + lexical + `"/>`
	if target {
		extra += `<xs:group name="Fields"><xs:sequence/></xs:group>`
	}
	if owner == "named" {
		return tokenFamilyLocalRoot(`<xs:complexType name="Box">` + body + `</xs:complexType>` + extra)
	}
	return tokenFamilyLocalRoot(`<xs:element name="root"><xs:complexType>` + body + `</xs:complexType></xs:element>` + extra)
}

//nolint:gocognit // Each lexical and child exit checks the same public diagnostic contract.
func TestDirectAllTokenFamilyGroupSyntaxDiagnosticExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"builtin", "named"} {
				for _, owner := range []string{"named", "anonymous"} {
					for _, test := range []struct {
						name, model, marker, code string
						spec                      func(XSDVersion) string
					}{
						{"malformed ref", `<xs:group ref="t:bad:name"/>`, `ref="t:bad:name"`, invalidSchemaConditionalCode, schemaGroupParticleSpecRef},
						{"empty ref", `<xs:group ref=""/>`, `ref=""`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"unbound ref", `<xs:group ref="missing:Fields"/>`, `ref="missing:Fields"`, invalidSchemaConditionalCode, schemaGroupParticleSpecRef},
						{"missing ref", `<xs:group/>`, `<xs:group/>`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"bad id", `<xs:group id="bad:id" ref="t:Fields"/>`, `id="bad:id"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"forbidden attribute", `<xs:group bogus="1" ref="t:Fields"/>`, `bogus="1"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"forbidden attribute after ref", `<xs:group ref="t:Fields" bogus="1"/>`, `bogus="1"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"forbidden qualified attribute", `<xs:group xs:bogus="1" ref="t:Fields"/>`, `xs:bogus="1"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"bad xml language", `<xs:group xml:lang="" ref="t:Fields"/>`, `xml:lang=""`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"bad xml base", `<xs:group xml:base="%GG" ref="t:Fields"/>`, `xml:base="%GG"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"nonwhitespace text", `<xs:group ref="t:Fields">oops</xs:group>`, `oops`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"forbidden child", `<xs:group ref="t:Fields"><xs:element/></xs:group>`, `<xs:element/>`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"duplicate annotation", `<xs:group ref="t:Fields"><xs:annotation/><xs:annotation id="second"/></xs:group>`, `<xs:annotation id="second"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"bad annotation id", `<xs:group ref="t:Fields"><xs:annotation id="bad:id"/></xs:group>`, `id="bad:id"`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"bad annotation child", `<xs:group ref="t:Fields"><xs:annotation><xs:element/></xs:annotation></xs:group>`, `<xs:element/>`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef},
						{"malformed minimum", `<xs:group ref="t:Fields" minOccurs="bad"/>`, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef},
						{"malformed maximum", `<xs:group ref="t:Fields" maxOccurs="bad"/>`, `maxOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef},
						{"unbounded minimum", `<xs:group ref="t:Fields" minOccurs="unbounded"/>`, `minOccurs="unbounded"`, invalidSchemaCompositionCode, schemaParticleMinimumSpecRef},
					} {
						t.Run(profile.name+"/"+kind+"/"+shape+"/"+owner+"/"+test.name, func(t *testing.T) {
							root := tokenAllGroupBoundaryRoot(kind, shape, owner, test.model, true)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							checkStringAllDiagnostic(t, schema, err, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), nil, test.spec(profile.version))
							outer := requireDiagnostic(t, err)
							if test.name == "malformed minimum" || test.name == "malformed maximum" || test.name == "unbounded minimum" {
								inner := requireDiagnostic(t, errors.Unwrap(outer))
								if inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != outer.Loc() {
									t.Fatalf("occurrence lexical cause = %v", inner)
								}
								return
							}
							if outer.Unwrap() != nil {
								t.Fatalf("unexpected cause: %v", outer.Unwrap())
							}
						})
					}
				}
			}
		}
	}
}

func TestDirectAllTokenFamilyGroupOccurrenceRangeDiagnostic(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"builtin", "named"} {
				for _, owner := range []string{"named", "anonymous"} {
					t.Run(profile.name+"/"+kind+"/"+shape+"/"+owner, func(t *testing.T) {
						root := tokenAllGroupBoundaryRoot(kind, shape, owner, `<xs:group ref="t:Fields" minOccurs="2" maxOccurs="1"/>`, true)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						checkStringAllDiagnostic(t, schema, err, FailureInvalid, invalidSchemaCompositionCode, elementReferenceTestAttributeLoc(t, root, `<xs:group ref="t:Fields"`), []Loc{elementReferenceTestAttributeLoc(t, root, `minOccurs="2"`), elementReferenceTestAttributeLoc(t, root, `maxOccurs="1"`)}, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Named and anonymous group owners take distinct unsupported and resolution paths.
func TestDirectAllTokenFamilyGroupDependencyAndAnnotationExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"builtin", "named"} {
				for _, owner := range []string{"named", "anonymous"} {
					t.Run(profile.name+"/"+kind+"/"+shape+"/"+owner+"/unresolved bound ref", func(t *testing.T) {
						root := tokenAllGroupBoundaryRoot(kind, shape, owner, `<xs:group ref="t:Missing"/>`, false)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						loc := elementReferenceTestAttributeLoc(t, root, `ref="t:Missing"`)
						if owner == "anonymous" {
							assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, loc, nil, schemaSyntaxSpecRefForVersion(profile.version), ErrUnsupported)
							return
						}
						assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaModelGroupReferenceUnresolvedCode, loc, nil, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved)
					})
					for _, test := range []struct {
						name, model       string
						anonymousRefFirst bool
					}{
						{"group xml base first", `<xs:group xml:base="child/" ref="t:Fields"/>`, false},
						{"group xml base last", `<xs:group ref="t:Fields" xml:base="child/"/>`, true},
						{"annotation xml base", `<xs:group ref="t:Fields"><xs:annotation xml:base="child/"/></xs:group>`, true},
					} {
						t.Run(profile.name+"/"+kind+"/"+shape+"/"+owner+"/"+test.name, func(t *testing.T) {
							root := tokenAllGroupBoundaryRoot(kind, shape, owner, test.model, true)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
							loc, spec := elementReferenceTestAttributeLoc(t, root, `xml:base="child/"`), "xmlbase#matching"
							if owner == "anonymous" && test.anonymousRefFirst {
								loc, spec = elementReferenceTestAttributeLoc(t, root, `ref="t:Fields"`), schemaSyntaxSpecRefForVersion(profile.version)
							}
							checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, loc, nil, spec, ErrUnsupported)
							if diagnostic := requireDiagnostic(t, err); diagnostic.Unwrap() != nil {
								t.Fatalf("unsupported cause = %v", diagnostic.Unwrap())
							}
						})
					}
				}
			}
		}
	}
}

//nolint:gocognit // The four direct model kinds share one parent ordering boundary.
func TestDirectAllTokenFamilyComplexTypeModelOrderDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"builtin", "named"} {
				for _, owner := range []string{"named", "anonymous"} {
					for _, later := range []struct{ name, model string }{
						{"group", `<xs:group ref="t:Fields"/>`},
						{"all", `<xs:all/>`},
						{"sequence", `<xs:sequence/>`},
						{"choice", `<xs:choice/>`},
					} {
						for _, first := range []struct{ name, model string }{
							{"after all", `<xs:all/>`},
							{"after group", `<xs:group ref="t:Fields"/>`},
							{"after attribute", ``},
						} {
							t.Run(profile.name+"/"+kind+"/"+shape+"/"+owner+"/"+first.name+"/"+later.name, func(t *testing.T) {
								lexical := "xs:" + kind
								if shape == "named" {
									lexical = "t:Alias"
								}
								model := first.model + `<xs:attribute name="a" type="` + lexical + `"/>` + later.model
								if first.name != "after attribute" {
									model = first.model + later.model
								}
								root := tokenAllGroupBoundaryRoot(kind, shape, owner, model, true)
								schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
								ownerEnd := strings.Index(root, `</xs:complexType>`)
								loc := mustTestLoc(t, "root.xsd", 1, strings.LastIndex(root[:ownerEnd], later.model)+1)
								checkStringAllDiagnostic(t, schema, err, FailureInvalid, invalidSchemaCompositionCode, loc, nil, schemaComplexTypeGrammarSpecRef(profile.version))
								if diagnostic := requireDiagnostic(t, err); diagnostic.Unwrap() != nil {
									t.Fatalf("model order cause/related = %v/%v", diagnostic.Unwrap(), diagnostic.Related())
								}
							})
						}
					}
				}
			}
		}
	}
}
