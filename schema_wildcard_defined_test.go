package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

//nolint:gocognit // The matrix checks each admitted namespace, mode, owner, and lexical form through public facts.
func TestDirectWildcardDefinedPublicFacts(t *testing.T) {
	constraints := []struct {
		name, attribute, lexical string
		variety                  WildcardNamespaceConstraintVariety
		namespaces               []string
		explicit                 string
		nameValue                QName
	}{
		{"omitted", "", "##any", WildcardNamespaceConstraintAny, nil, "t:excluded", mustTestQName(t, "urn:root", "excluded")},
		{"any", ` namespace="##any"`, "##any", WildcardNamespaceConstraintAny, nil, "t:excluded", mustTestQName(t, "urn:root", "excluded")},
		{"other", ` namespace="##other"`, "##other", WildcardNamespaceConstraintAny, nil, "p:excluded", mustTestQName(t, "urn:p", "excluded")},
		{"positive", ` namespace="##targetNamespace urn:p"`, "##targetNamespace urn:p", WildcardNamespaceConstraintEnumeration, []string{"urn:p", "urn:root"}, "t:excluded", mustTestQName(t, "urn:root", "excluded")},
		{"empty", ` namespace=""`, "", WildcardNamespaceConstraintEnumeration, nil, "", QName{}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"strict", "lax", "skip"} {
				for _, namespace := range constraints {
					for _, mixed := range []bool{false, true} {
						if mixed && namespace.explicit == "" {
							continue
						}
						t.Run(string(policy)+"/"+model+"/"+mode+"/"+namespace.name+"/mixed="+strconv.FormatBool(mixed), func(t *testing.T) {
							qname := "##defined"
							wantNames := []QName(nil)
							if mixed {
								qname = namespace.explicit + " ##defined " + namespace.explicit + " ##defined"
								wantNames = []QName{namespace.nameValue}
							}
							raw := strings.ReplaceAll(qname, " ", "  &#xA;")
							root := directWildcardSchema("1.1", model, false, namespace.attribute+` processContents="`+mode+`" notQName="`+raw+`" minOccurs="0" maxOccurs="2"`)
							root = strings.Replace(root, `xmlns:t=`, `xmlns:p="urn:p" xmlns:t=`, 1)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
							if err != nil {
								t.Fatalf("parse symbolic wildcard: %v", err)
							}
							for run := 0; run < 2; run++ {
								wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
								constraint := wildcard.QNameConstraint()
								if wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:any`) || wildcard.Occurrences().String() != "0/2" || wildcard.ProcessContents() != mode ||
									wildcard.ProcessContentsLoc() != wildcardParticleTestLoc(t, root, `processContents="`+mode+`"`) ||
									wildcard.NamespaceConstraint().Variety() != namespace.variety ||
									wildcard.NamespaceConstraint().LexicalForm() != namespace.lexical ||
									!reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), namespace.namespaces) ||
									!constraint.Present() || constraint.LexicalForm() != qname ||
									constraint.Loc() != wildcardParticleTestLoc(t, root, `notQName="`+raw+`"`) ||
									!reflect.DeepEqual(constraint.Tokens(), strings.Fields(qname)) ||
									!reflect.DeepEqual(constraint.Names(), wantNames) ||
									!wildcardQNameBindingIs(constraint.NamespaceBindings(), "t", "urn:root") ||
									!wildcardQNameBindingIs(constraint.NamespaceBindings(), "p", "urn:p") {
									t.Fatalf("run %d symbolic wildcard facts: wildcard=%#v constraint=%#v", run, wildcard, constraint)
								}
								wantNamespaceLoc := Loc{}
								if namespace.attribute != "" {
									wantNamespaceLoc = wildcardParticleTestLoc(t, root, strings.TrimSpace(namespace.attribute))
								}
								if wildcard.NamespaceLoc() != wantNamespaceLoc {
									t.Fatalf("namespace Loc = %v", wildcard.NamespaceLoc())
								}
								if run == 0 {
									tokens := constraint.Tokens()
									tokens[0] = "changed"
									bindings := constraint.NamespaceBindings()
									bindings[0].Namespace = "changed"
									if mixed {
										names := constraint.Names()
										names[0] = QName{}
									}
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestDirectWildcardDefinedForwardGlobalIsSymbolic(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1"><xs:complexType name="Record"><xs:sequence><xs:any notQName="##defined"/></xs:sequence></xs:complexType><xs:element name="later" type="xs:string"/></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s forward declaration: %v", policy, err)
		}
		components := schema.Components()
		if len(components) != 2 || components[0].Name() != mustTestQName(t, "urn:root", "Record") || components[1].Name() != mustTestQName(t, "urn:root", "later") {
			t.Fatalf("%s component order = %v", policy, components)
		}
		wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
		if !reflect.DeepEqual(wildcard.QNameConstraint().Tokens(), []string{"##defined"}) || len(wildcard.QNameConstraint().Names()) != 0 {
			t.Fatalf("%s expanded symbolic exclusion: %#v", policy, wildcard.QNameConstraint())
		}
	}
}

//nolint:gocognit // Each consumer has a distinct public diagnostic location and cause.
func TestDirectWildcardDefinedConsumersAndZeroOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, bounds := range []string{``, ` minOccurs="0" maxOccurs="0"`} {
				root := directWildcardSchemaWithTerms("1.1", model, false, ` notQName="##defined"`+bounds, `<xs:element name="first" type="xs:integer"/>`, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("%s/%s/%s parse: %v", policy, model, bounds, err)
				}
				if bounds != "" {
					assertWildcardQNameOmittedTerm(t, directWildcardDefinition(t, schema).Particle())
					assertWildcardQNameOmittedConsumers(t, schema, model)
					continue
				}
				wildcardLoc := wildcardParticleTestLoc(t, root, `<xs:any`)
				modelLoc := wildcardParticleTestLoc(t, root, `<xs:`+model+`>`)
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				validation := requireDiagnostic(t, validationErr)
				wantLoc := wildcardLoc
				wantCause := errInstanceChoiceWildcard
				if model == "sequence" {
					wantLoc = mustTestLoc(t, "instance.xml", 1, 1)
					wantCause = errInstanceSequenceWildcard
				}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantLoc ||
					validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !negativeWildcardRelatedHas(validation.Related(), wildcardLoc) ||
					!negativeWildcardRelatedHas(validation.Related(), modelLoc) || !errors.Is(validationErr, wantCause) {
					t.Fatalf("validation diagnostic = %s related=%v", validation, validation.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				if output != nil {
					t.Fatalf("generation returned %d bytes", len(output))
				}
				generation := requireDiagnostic(t, generationErr)
				wantGenerationCause := errCodegenDirectChoiceWildcard
				wantSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
				if model == "sequence" {
					wantGenerationCause = errCodegenDirectSequenceWildcard
					wantSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
				}
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wildcardLoc ||
					generation.SpecRef() != wantSpec || !negativeWildcardRelatedHas(generation.Related(), wildcardLoc) ||
					!negativeWildcardRelatedHas(generation.Related(), modelLoc) || !errors.Is(generationErr, wantGenerationCause) {
					t.Fatalf("generation diagnostic = %s related=%v", generation, generation.Related())
				}
			}
		}
	}
}

//nolint:gocognit // Each excluded extension owner and process mode must retain its own diagnostic.
func TestWildcardDefinedExtensionPlacementsStayUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"strict", "lax", "skip"} {
				for _, shape := range []string{"named", "inline"} {
					for _, bounds := range []string{"", ` minOccurs="0" maxOccurs="0"`} {
						content := `<xs:` + model + `><xs:any processContents="` + mode + `" notQName="##defined"` + bounds + `/></xs:` + model + `>`
						owner := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base">` + content + `</xs:extension></xs:complexContent></xs:complexType>`
						if shape == "inline" {
							owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base">` + content + `</xs:extension></xs:complexContent></xs:complexType></xs:element>`
						}
						root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t" version="1.1"><xs:complexType name="Base"/>` + owner + `</xs:schema>`
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						assertZeroSchema(t, schema)
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
							diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="##defined"`) ||
							len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaAnyParticleSpecRef(XSDVersion11) ||
							!errors.Is(err, errSchemaAnyParticleUnsupported) {
							t.Fatalf("%s/%s/%s/%s/%s: extension diagnostic=%s related=%v cause=%v", policy, model, mode, shape, bounds, diagnostic, diagnostic.Related(), err)
						}
					}
				}
			}
		}
	}
}

//nolint:gocognit // Alternate exits share the direct wildcard phase boundary and must stay distinct.
func TestDirectWildcardDefinedDiagnosticExits(t *testing.T) {
	cases := []struct {
		name, attributes, marker, related string
		policy                            LanguagePolicy
		class                             FailureClass
		code, spec                        string
		cause                             error
	}{
		{"strict10 keyword", ` notQName="##defined"`, `notQName="##defined"`, "", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch},
		{"strict10 malformed", ` notQName="##defined bad:q:name"`, `notQName="##defined bad:q:name"`, "", Strict10, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"malformed", ` notQName="##defined bad:q:name"`, `notQName="##defined bad:q:name"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound", ` notQName="##defined bad:item"`, `notQName="##defined bad:item"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"sibling", ` notQName="##defined ##definedSibling"`, `notQName="##defined ##definedSibling"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
		{"negative", ` notNamespace="##local" notQName="##defined"`, `notQName="##defined"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
		{"outside zero", ` namespace="##targetNamespace" notQName="##defined p:item" minOccurs="0" maxOccurs="0"`, `notQName="##defined p:item"`, `namespace="##targetNamespace"`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"empty outside zero", ` namespace="" notQName="##defined t:item" minOccurs="0" maxOccurs="0"`, `notQName="##defined t:item"`, `namespace=""`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"cooccurrence", ` namespace="##any" notNamespace="##local" notQName="##defined"`, `namespace="##any"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, "", nil},
	}
	for _, test := range cases {
		for _, model := range []string{"choice", "sequence"} {
			root := directWildcardSchema("1.1", model, false, test.attributes)
			root = strings.Replace(root, `xmlns:t=`, `xmlns:p="urn:p" xmlns:t=`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc(nil)
			if test.related != "" {
				wantRelated = []Loc{wildcardParticleTestLoc(t, root, test.related)}
			}
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.marker) ||
				!reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != test.spec ||
				test.cause != nil && !errors.Is(err, test.cause) || test.cause == nil && errors.Unwrap(err) != nil {
				t.Fatalf("%s/%s: diagnostic=%s related=%v spec=%q cause=%v", test.name, model, diagnostic, diagnostic.Related(), diagnostic.SpecRef(), err)
			}
		}
	}
	for _, model := range []string{"choice", "sequence"} {
		root := directWildcardSchema("1.1", model, false, ` notQName="##defined" minOccurs="oops"`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		var lexical Diagnostic
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
			diagnostic.Loc() != wildcardParticleTestLoc(t, root, `minOccurs="oops"`) || len(diagnostic.Related()) != 0 ||
			diagnostic.SpecRef() != schemaParticleOccurrenceDatatypeSpecRef(XSDVersion11) ||
			!errors.As(diagnostic.Unwrap(), &lexical) || lexical.Code() != InvalidIntegerLexicalCode ||
			lexical.Loc() != diagnostic.Loc() {
			t.Fatalf("%s occurrence diagnostic=%s cause=%v", model, diagnostic, err)
		}
	}
}

func TestWildcardDefinedBaseCompositionStaysUnsupported(t *testing.T) {
	base := `<xs:complexType name="Base"><xs:sequence><xs:any notQName="##defined"/></xs:sequence></xs:complexType>`
	derived := []struct {
		name, body string
	}{
		{"extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`},
		{"restriction with keyword", `<xs:complexType name="Derived"><xs:complexContent><xs:restriction base="t:Base"><xs:anyAttribute namespace="##any" processContents="strict" notQName="##defined"/></xs:restriction></xs:complexContent></xs:complexType>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, test := range derived {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t" version="1.1">` + base + test.body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{wildcardParticleTestLoc(t, root, `<xs:complexType name="Base"`), wildcardParticleTestLoc(t, root, `<xs:sequence>`)}
			wantSpec := schemaComplexTypeExtensionSpecRef(XSDVersion11)
			if test.name == "restriction with keyword" {
				wantSpec = schemaComplexTypeDerivationSpecRef(XSDVersion11)
			}
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, `base="t:Base"`) ||
				!reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != wantSpec ||
				!errors.Is(err, errSchemaComplexTypeBaseUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseNonEmpty) {
				t.Fatalf("%s/%s composition=%s related=%v cause=%v", policy, test.name, diagnostic, diagnostic.Related(), err)
			}
		}
	}
}
