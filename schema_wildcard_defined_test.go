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
func TestDirectWildcardSymbolicPublicFacts(t *testing.T) {
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
					for _, keyword := range []string{"##defined", "##definedSibling"} {
						for _, mixed := range []bool{false, true} {
							if mixed && namespace.explicit == "" {
								continue
							}
							t.Run(string(policy)+"/"+model+"/"+mode+"/"+namespace.name+"/"+keyword+"/mixed="+strconv.FormatBool(mixed), func(t *testing.T) {
								qname := keyword
								wantNames := []QName(nil)
								if mixed {
									qname = namespace.explicit + " " + keyword + " " + namespace.explicit + " " + keyword
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
}

func TestDirectWildcardSymbolicForwardGlobal(t *testing.T) {
	for _, keyword := range []string{"##defined", "##definedSibling"} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1"><xs:complexType name="Record"><xs:sequence><xs:any notQName="` + keyword + `"/></xs:sequence></xs:complexType><xs:element name="later" type="xs:string"/></xs:schema>`
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
			if !reflect.DeepEqual(wildcard.QNameConstraint().Tokens(), []string{keyword}) || len(wildcard.QNameConstraint().Names()) != 0 {
				t.Fatalf("%s expanded symbolic exclusion: %#v", policy, wildcard.QNameConstraint())
			}
		}
	}
}

//nolint:gocognit // Check public sibling order, reference identity, and locations for both direct models.
func TestDirectWildcardSiblingLocalAndReferenceFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := directWildcardSchema("1.1", model, false, ` notQName="##definedSibling"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s/%s: %v", policy, model, err)
			}
			components := schema.Components()
			if len(components) != 3 || components[0].Name() != mustTestQName(t, "urn:root", "root") ||
				components[1].Name() != mustTestQName(t, "urn:root", "shared") ||
				components[2].Name() != mustTestQName(t, "urn:root", "Record") {
				t.Fatalf("%s/%s: component order = %v", policy, model, components)
			}
			definition := directWildcardDefinition(t, schema)
			var terms []Particle
			if choice, ok := definition.Particle().(ChoiceParticle); ok {
				terms = choice.Alternatives()
			}
			if sequence, ok := definition.Particle().(SequenceParticle); ok {
				terms = sequence.Particles()
			}
			if len(terms) != 4 {
				t.Fatalf("%s/%s: terms = %v", policy, model, terms)
			}
			first, firstOK := terms[0].(ElementParticle)
			reference, refOK := terms[1].(ElementReferenceParticle)
			wildcard, wildOK := terms[2].(WildcardParticle)
			last, lastOK := terms[3].(ElementParticle)
			if !firstOK || !refOK || !wildOK || !lastOK || first.Name() != mustTestQName(t, "", "first") ||
				reference.Ref() != mustTestQName(t, "urn:root", "shared") || reference.TargetID() != components[1].ID() ||
				last.Name() != mustTestQName(t, "", "last") || wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:any`) ||
				!reflect.DeepEqual(wildcard.QNameConstraint().Tokens(), []string{"##definedSibling"}) || len(wildcard.QNameConstraint().Names()) != 0 {
				t.Fatalf("%s/%s: sibling facts changed, terms = %v", policy, model, terms)
			}
			if reference.RefLoc() != wildcardParticleTestLoc(t, root, `ref="t:shared"`) ||
				first.Loc() != wildcardParticleTestLoc(t, root, `<xs:element name="first"`) ||
				last.Loc() != wildcardParticleTestLoc(t, root, `<xs:element name="last"`) {
				t.Fatalf("%s/%s: sibling term locations changed", policy, model)
			}
		}
	}
}

func TestDirectWildcardSiblingVersionLabelDoesNotSelectPolicy(t *testing.T) {
	for _, version := range []string{"1.0", "1.1", "opaque"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			root := directWildcardSchema(version, "sequence", false, ` notQName="##definedSibling"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s/%s: %v", policy, version, err)
			}
			wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
			if !reflect.DeepEqual(wildcard.QNameConstraint().Tokens(), []string{"##definedSibling"}) {
				t.Fatalf("%s/%s: version label changed symbolic tokens", policy, version)
			}
		}
	}
}

func TestDirectWildcardSiblingAnnotationPayloadIsOpaque(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := directWildcardSchemaWithTerms("1.1", model, false, ` notQName="##definedSibling"`, "", "")
			plain := `<xs:any notQName="##definedSibling"/>`
			annotated := `<xs:any notQName="##definedSibling"><xs:annotation><xs:documentation><xs:any notQName="bad:q:name"/></xs:documentation></xs:annotation></xs:any>`
			root = strings.Replace(root, plain, annotated, 1)
			if !strings.Contains(root, annotated) {
				t.Fatal("annotation fixture was not installed")
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s/%s: %v", policy, model, err)
			}
			wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
			if !reflect.DeepEqual(wildcard.QNameConstraint().Tokens(), []string{"##definedSibling"}) || len(wildcard.QNameConstraint().Names()) != 0 ||
				wildcard.QNameConstraint().Loc() != wildcardParticleTestLoc(t, root, `notQName="##definedSibling"`) {
				t.Fatalf("%s/%s: annotation changed wildcard facts", policy, model)
			}
		}
	}
}

func TestDirectWildcardSiblingGraphVisibilityPrecedesQuery(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:import namespace="urn:a" schemaLocation="a.xsd"/><xs:import namespace="urn:b" schemaLocation="b.xsd"/></xs:schema>`
	b := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:b"><xs:element name="target" type="xs:integer"/></xs:schema>`
	for _, model := range []string{"choice", "sequence"} {
		a := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:b" targetNamespace="urn:a"><xs:complexType name="Record"><xs:` + model + `><xs:element ref="b:target"/><xs:any notQName="##definedSibling"/></xs:` + model + `></xs:complexType></xs:schema>`
		fixtures := map[string]discoveryFixture{"a.xsd": {id: "a.xsd", contents: a}, "b.xsd": {id: "b.xsd", contents: b}}
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceNamespaceCode ||
				diagnostic.Loc() != wildcardQNameDocumentLoc(t, "a.xsd", a, `ref="b:target"`) ||
				!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardQNameDocumentLoc(t, "b.xsd", b, `<xs:element name="target"`)}) ||
				diagnostic.SpecRef() != schemaElementReferenceImportSpecRef(XSDVersion11) ||
				!errors.Is(err, errSchemaElementReferenceNamespace) {
				t.Fatalf("%s/%s: graph visibility diagnostic = %s related=%v cause=%v", policy, model, diagnostic, diagnostic.Related(), err)
			}
		}
	}
}

//nolint:gocognit // Each consumer has a distinct public diagnostic location and cause.
func TestDirectWildcardSymbolicConsumersAndZeroOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, keyword := range []string{"##defined", "##definedSibling"} {
				for _, bounds := range []string{``, ` minOccurs="0" maxOccurs="0"`} {
					root := directWildcardSchemaWithTerms("1.1", model, false, ` notQName="`+keyword+`"`+bounds, `<xs:element name="first" type="xs:integer"/>`, "")
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
}

//nolint:gocognit // Each excluded extension owner and process mode must retain its own diagnostic.
func TestWildcardSymbolicExtensionPlacementsStayUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"strict", "lax", "skip"} {
				for _, keyword := range []string{"##defined", "##definedSibling"} {
					for _, shape := range []string{"named", "inline"} {
						for _, bounds := range []string{"", ` minOccurs="0" maxOccurs="0"`} {
							content := `<xs:` + model + `><xs:any processContents="` + mode + `" notQName="` + keyword + `"` + bounds + `/></xs:` + model + `>`
							owner := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base">` + content + `</xs:extension></xs:complexContent></xs:complexType>`
							if shape == "inline" {
								owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base">` + content + `</xs:extension></xs:complexContent></xs:complexType></xs:element>`
							}
							root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t" version="1.1"><xs:complexType name="Base"/>` + owner + `</xs:schema>`
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
							assertZeroSchema(t, schema)
							diagnostic := requireDiagnostic(t, err)
							if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
								diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="`+keyword+`"`) ||
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
}

//nolint:gocognit // Alternate exits share the direct wildcard phase boundary and must stay distinct.
func TestDirectWildcardSymbolicDiagnosticExits(t *testing.T) {
	cases := []struct {
		name, attributes, marker, related string
		policy                            LanguagePolicy
		class                             FailureClass
		code, spec                        string
		cause                             error
	}{
		{"strict10 keyword", ` notQName="##defined"`, `notQName="##defined"`, "", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch},
		{"strict10 sibling", ` notQName="##definedSibling" minOccurs="0" maxOccurs="0"`, `notQName="##definedSibling"`, "", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch},
		{"strict10 malformed", ` notQName="##defined bad:q:name"`, `notQName="##defined bad:q:name"`, "", Strict10, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"strict10 malformed sibling", ` notQName="##definedSibling bad:q:name"`, `notQName="##definedSibling bad:q:name"`, "", Strict10, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"malformed", ` notQName="##defined bad:q:name"`, `notQName="##defined bad:q:name"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"malformed sibling", ` notQName="##definedSibling bad:q:name"`, `notQName="##definedSibling bad:q:name"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound", ` notQName="##defined bad:item"`, `notQName="##defined bad:item"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound sibling", ` notQName="##definedSibling bad:item"`, `notQName="##definedSibling bad:item"`, "", Strict11, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"negative", ` notNamespace="##local" notQName="##defined"`, `notQName="##defined"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
		{"negative sibling", ` notNamespace="##local" notQName="##definedSibling"`, `notQName="##definedSibling"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
		{"negative sibling QName", ` notNamespace="##local" notQName="##definedSibling p:item"`, `notQName="##definedSibling p:item"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
		{"negative sibling member", ` notNamespace="##local" notQName="##definedSibling local"`, `notQName="##definedSibling local"`, "", Strict11, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported},
		{"outside zero", ` namespace="##targetNamespace" notQName="##defined p:item" minOccurs="0" maxOccurs="0"`, `notQName="##defined p:item"`, `namespace="##targetNamespace"`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"outside sibling zero", ` namespace="##targetNamespace" notQName="##definedSibling p:item" minOccurs="0" maxOccurs="0"`, `notQName="##definedSibling p:item"`, `namespace="##targetNamespace"`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"empty outside sibling zero", ` namespace="" notQName="##definedSibling t:item" minOccurs="0" maxOccurs="0"`, `notQName="##definedSibling t:item"`, `namespace=""`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"empty outside zero", ` namespace="" notQName="##defined t:item" minOccurs="0" maxOccurs="0"`, `notQName="##defined t:item"`, `namespace=""`, Strict11, FailureInvalid, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"cooccurrence", ` namespace="##any" notNamespace="##local" notQName="##defined"`, `namespace="##any"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, "", nil},
		{"cooccurrence sibling", ` namespace="##any" notNamespace="##local" notQName="##definedSibling"`, `namespace="##any"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, "", nil},
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
		root := directWildcardSchema("1.1", model, false, ` notQName="##definedSibling" minOccurs="oops"`)
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

func TestWildcardSymbolicBaseCompositionStaysUnsupported(t *testing.T) {
	for _, keyword := range []string{"##defined", "##definedSibling"} {
		base := `<xs:complexType name="Base"><xs:sequence><xs:any notQName="` + keyword + `"/></xs:sequence></xs:complexType>`
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
}
