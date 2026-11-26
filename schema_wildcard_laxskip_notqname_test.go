package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Each admitted combination is paired with its no-notQName control.
func TestWildcardQNameLaxSkipDirectPublicFacts(t *testing.T) {
	constraints := []struct {
		name, attribute, lexical string
		variety                  WildcardNamespaceConstraintVariety
		names                    []QName
	}{
		{"any", ``, "##any", WildcardNamespaceConstraintAny, []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z"), mustTestQName(t, "urn:root", "local")}},
		{"positive", ` namespace="urn:p ##targetNamespace"`, "urn:p ##targetNamespace", WildcardNamespaceConstraintEnumeration, []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z"), mustTestQName(t, "urn:root", "local")}},
		{"other", ` namespace="##other"`, "##other", WildcardNamespaceConstraintAny, []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z")}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []string{"lax", "skip"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, constraint := range constraints {
					for _, bounds := range []string{``, ` minOccurs="2" maxOccurs="4"`} {
						name := string(policy) + "/" + mode + "/" + model + "/" + constraint.name + "/" + bounds
						t.Run(name, func(t *testing.T) {
							qname := `p:z p:a p:z`
							qnameRaw := `&#xA;p:z&#x9;p:a  p:z`
							if constraint.name != "other" {
								qname += ` local`
								qnameRaw += ` &#xD;local`
							}
							attributes := constraint.attribute + ` processContents="` + mode + `"` + bounds
							for _, explicit := range []bool{false, true} {
								candidate := attributes
								if explicit {
									candidate += ` notQName="` + qnameRaw + `"`
								}
								root := directWildcardSchema("1.1", model, false, candidate)
								root = strings.Replace(root, `xmlns:t=`, `xmlns="urn:root" xmlns:p="urn:p" xmlns:t=`, 1)
								schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
								if err != nil {
									t.Fatalf("control=%t: discover schema: %v", !explicit, err)
								}
								particle := directWildcardDefinition(t, schema).Particle()
								wildcard := directWildcardFromParticle(t, particle)
								wantBounds := "1/1"
								if bounds != "" {
									wantBounds = "2/4"
								}
								wantNamespaceLoc := Loc{}
								if constraint.attribute != "" {
									wantNamespaceLoc = wildcardParticleTestLoc(t, root, strings.TrimSpace(constraint.attribute))
								}
								assertDirectWildcardParticle(t, root, particle, model,
									directWildcardOccurrenceCase{wantRange: wantBounds, wantTerm: true},
									directWildcardConstraintCase{
										name: constraint.name, namespaceMarker: strings.TrimSpace(constraint.attribute),
										processContentsMarker: `processContents="` + mode + `"`,
										wantNamespace:         constraint.lexical, wantProcessContents: mode,
									})
								if wildcard.Occurrences().String() != wantBounds || wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:any`) ||
									wildcard.ProcessContents() != mode || wildcard.ProcessContentsLoc() != wildcardParticleTestLoc(t, root, `processContents="`+mode+`"`) ||
									wildcard.NamespaceConstraint().LexicalForm() != constraint.lexical || wildcard.NamespaceConstraint().Variety() != constraint.variety || wildcard.NamespaceLoc() != wantNamespaceLoc {
									t.Fatalf("control=%t: wildcard bounds/mode/namespace/location facts = %#v", !explicit, wildcard)
								}
								qnames := wildcard.QNameConstraint()
								if !explicit {
									if qnames.Present() {
										t.Fatal("no-notQName control acquired an exclusion")
									}
									continue
								}
								if !qnames.Present() || qnames.LexicalForm() != qname || qnames.Loc() != wildcardParticleTestLoc(t, root, `notQName="`+qnameRaw+`"`) ||
									!reflect.DeepEqual(qnames.Tokens(), strings.Split(qname, " ")) || !reflect.DeepEqual(qnames.Names(), constraint.names) {
									t.Fatalf("QName lexical/value facts = %#v", qnames)
								}
								bindings := qnames.NamespaceBindings()
								if !wildcardQNameBindingIs(bindings, "", "urn:root") || !wildcardQNameBindingIs(bindings, "p", "urn:p") {
									t.Fatalf("QName namespace bindings = %#v", bindings)
								}
								tokens := qnames.Tokens()
								tokens[0] = "changed"
								names := qnames.Names()
								names[0] = QName{}
								bindings[0].Namespace = "changed"
								again := wildcard.QNameConstraint()
								if again.Tokens()[0] != "p:z" || !reflect.DeepEqual(again.Names(), constraint.names) || again.NamespaceBindings()[0].Namespace == "changed" {
									t.Fatal("public QName facts changed after slice mutation")
								}
							}
						})
					}
				}
			}
		}
	}
}

func wildcardQNameBindingIs(bindings []IdentityNamespaceBinding, prefix, namespace string) bool {
	for _, binding := range bindings {
		if binding.Prefix == prefix && binding.Namespace == namespace {
			return true
		}
	}
	return false
}

//nolint:gocognit // Each non-direct owner checks the same located boundary before publication.
func TestWildcardQNameLaxSkipDerivedOwnersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []string{"lax", "skip"} {
			for _, model := range []string{"sequence", "choice"} {
				for _, inline := range []bool{false, true} {
					for _, zero := range []bool{false, true} {
						name := string(policy) + "/" + mode + "/" + model
						if inline {
							name += "/inline"
						}
						if zero {
							name += "/zero"
						}
						t.Run(name, func(t *testing.T) {
							bounds := ""
							if zero {
								bounds = ` minOccurs="0" maxOccurs="0"`
							}
							modelXML := `<xs:` + model + `><xs:any namespace="##any" processContents="` + mode + `" notQName="t:x"` + bounds + `/></xs:` + model + `>`
							body := `<xs:complexType name="A"><xs:complexContent><xs:extension base="t:Base">` + modelXML + `</xs:extension></xs:complexContent></xs:complexType>`
							if inline {
								body = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base">` + modelXML + `</xs:extension></xs:complexContent></xs:complexType></xs:element>`
							}
							root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:complexType name="Base"/>` + body + `</xs:schema>`
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
							if err == nil {
								t.Fatal("derived wildcard exclusion published a schema")
							}
							assertZeroSchema(t, schema)
							diagnostic := requireDiagnostic(t, err)
							if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
								diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="t:x"`) ||
								len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaAnyParticleSpecRef(XSDVersion11) ||
								!errors.Is(err, errSchemaAnyParticleUnsupported) {
								t.Fatalf("derived wildcard diagnostic = %s related=%v spec=%q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
							}
						})
					}
				}
			}
		}
	}
}

func TestWildcardQNameStrictNamedExtensionRemainsQueryable(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:complexType name="Base"/><xs:complexType name="A"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:any namespace="##any" processContents="strict" notQName="t:x"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s: strict extension: %v", policy, err)
		}
		components := schema.Find(mustTestQName(t, "urn:t", "A"))
		if len(components) != 1 {
			t.Fatalf("%s: A components = %d", policy, len(components))
		}
		definition, ok := components[0].ComplexType()
		if !ok {
			t.Fatal("A has no complex type view")
		}
		wildcard := directWildcardFromParticle(t, definition.Particle())
		if wildcard.ProcessContents() != "strict" || wildcard.QNameConstraint().Loc() != wildcardParticleTestLoc(t, root, `notQName="t:x"`) ||
			!reflect.DeepEqual(wildcard.QNameConstraint().Names(), []QName{mustTestQName(t, "urn:t", "x")}) {
			t.Fatalf("strict extension wildcard = %#v", wildcard)
		}
	}
}

//nolint:gocognit // Exercise alternate exits at the same direct wildcard boundary.
func TestWildcardQNameLaxSkipDiagnosticExits(t *testing.T) {
	tests := []struct {
		name, attributes, marker, related string
		policy                            LanguagePolicy
		class                             FailureClass
		code, spec                        string
		cause                             error
	}{
		{"malformed lax", ` processContents="lax" notQName="bad:q:name"`, `notQName="bad:q:name"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound skip", ` processContents="skip" notQName="bad:item"`, `notQName="bad:item"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"outside lax", ` namespace="##targetNamespace" processContents="lax" notQName="xs:item"`, `notQName="xs:item"`, `namespace="##targetNamespace"`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"outside skip zero", ` namespace="##targetNamespace" processContents="skip" notQName="xs:item" minOccurs="0" maxOccurs="0"`, `notQName="xs:item"`, `namespace="##targetNamespace"`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"strict10 lax", ` processContents="lax" notQName="xs:item"`, `notQName="xs:item"`, "", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch},
		{"strict10 skip zero", ` processContents="skip" notQName="xs:item" minOccurs="0" maxOccurs="0"`, `notQName="xs:item"`, "", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch},
		{"strict10 malformed skip", ` processContents="skip" notQName="bad:q:name"`, `notQName="bad:q:name"`, "", Strict10, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"negative skip", ` notNamespace="##local" processContents="skip" notQName="xs:item"`, `notQName="xs:item"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
	}
	for _, test := range tests {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(test.name+"/"+model, func(t *testing.T) {
				root := directWildcardSchema("1.1", model, false, test.attributes)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
				if err == nil {
					t.Fatal("diagnostic exit published a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc(nil)
				if test.related != "" {
					wantRelated = []Loc{wildcardParticleTestLoc(t, root, test.related)}
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.marker) ||
					!reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != test.spec ||
					test.cause != nil && !errors.Is(err, test.cause) || test.cause == nil && errors.Unwrap(err) != nil {
					t.Fatalf("diagnostic = %s related=%v spec=%q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
				}
			})
		}
	}
}
