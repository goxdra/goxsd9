package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // This is the public fact matrix for the paired empty lists.
func TestDirectEmptyNamespaceAndQNameFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, lexical := range []string{"", "&#x9;&#xA;&#xD; "} {
					for _, mode := range []string{"default", "strict", "lax", "skip"} {
						for _, kind := range []string{"any", "anyAttribute"} {
							if kind == "anyAttribute" && mode != "default" && mode != "strict" {
								continue
							}
							t.Run(string(policy)+"/"+version+"/"+model+"/"+kind+"/"+mode+"/"+lexical, func(t *testing.T) {
								attributes := ` namespace="` + lexical + `" notQName="` + lexical + `"`
								if mode != "default" {
									attributes += ` processContents="` + mode + `"`
								}
								root := directWildcardSchema(version, model, false, attributes+` minOccurs="2" maxOccurs="4"`)
								if kind == "anyAttribute" {
									root = directAnyAttributeSchema(version, model, attributes)
								}
								var previous []Component
								for run := 0; run < 2; run++ {
									schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
									if err != nil {
										t.Fatalf("parse: %v", err)
									}
									components := schema.Components()
									if run > 0 && !reflect.DeepEqual(components, previous) {
										t.Fatal("repeated build changed components")
									}
									previous = components
									var walked []ComponentID
									if err := schema.Walk(func(c Component) error { walked = append(walked, c.ID()); return nil }); err != nil {
										t.Fatal(err)
									}
									for i, c := range components {
										if i >= len(walked) || walked[i] != c.ID() {
											t.Fatalf("walk order = %v; components = %v", walked, components)
										}
									}
									if len(walked) != len(components) {
										t.Fatalf("walk count = %d; components = %d", len(walked), len(components))
									}
									copied := schema.Components()
									copied[0] = Component{}
									if schema.Components()[0] == (Component{}) {
										t.Fatal("component slice aliases schema")
									}
									var ns WildcardNamespaceConstraint
									var q WildcardQNameConstraint
									var namespaceLoc, processLoc, wildcardLoc Loc
									var process string
									if kind == "any" {
										wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
										if wildcard.Occurrences().String() != "2/4" {
											t.Fatalf("occurrences = %s", wildcard.Occurrences())
										}
										ns, q = wildcard.NamespaceConstraint(), wildcard.QNameConstraint()
										namespaceLoc, processLoc, wildcardLoc = wildcard.NamespaceLoc(), wildcard.ProcessContentsLoc(), wildcard.Loc()
										process = wildcard.ProcessContents()
									}
									if kind == "anyAttribute" {
										attribute := definitionAnyAttribute(t, requireAnyAttributeComplexType(t, components[1], model))
										ns, q = attribute.NamespaceConstraint(), attribute.QNameConstraint()
										namespaceLoc, processLoc, wildcardLoc = attribute.NamespaceLoc(), attribute.ProcessContentsLoc(), attribute.Loc()
										process = attribute.ProcessContents()
									}
									loc := func(marker string) Loc {
										if kind == "any" {
											return wildcardParticleTestLoc(t, root, marker)
										}
										return anyAttributeTestLoc(root, marker)
									}
									wantProcess := mode
									if mode == "default" {
										wantProcess = "strict"
									}
									wantProcessLoc := Loc{}
									if mode != "default" {
										wantProcessLoc = loc(`processContents="` + mode + `"`)
									}
									if process != wantProcess || processLoc != wantProcessLoc || namespaceLoc != loc(`namespace="`+lexical+`"`) || ns.Loc() != namespaceLoc || ns.Variety() != WildcardNamespaceConstraintEnumeration || ns.LexicalForm() != "" || len(ns.Namespaces()) != 0 {
										t.Fatalf("namespace/process = %q/%v/%s/%s/%v", process, ns, processLoc, namespaceLoc, ns.Namespaces())
									}
									if !q.Present() || q.Loc() != loc(`notQName="`+lexical+`"`) || q.LexicalForm() != "" || len(q.Tokens()) != 0 || len(q.Names()) != 0 {
										t.Fatalf("QName = %#v", q)
									}
									if wildcardLoc != loc("<xs:"+kind) {
										t.Fatalf("wildcard loc = %s", wildcardLoc)
									}
									if bindings := q.NamespaceBindings(); len(bindings) > 0 {
										bindings[0].Namespace = "changed"
										if reflect.DeepEqual(bindings, q.NamespaceBindings()) {
											t.Fatal("QName namespace bindings alias schema")
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

//nolint:gocognit // Keep policy and shape exits explicit for the new pair.
func TestDirectEmptyNamespaceAndQNameBoundaries(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, kind := range []string{"any", "anyAttribute"} {
				root := directWildcardSchema("1.1", model, false, ` namespace="" notQName=""`)
				if kind == "anyAttribute" {
					root = directAnyAttributeSchema("1.1", model, ` namespace="" notQName=""`)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if policy == Strict10 {
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					loc := wildcardParticleTestLoc(t, root, `notQName=""`)
					if kind == "anyAttribute" {
						loc = anyAttributeTestLoc(root, `notQName=""`)
					}
					if d.Class() != FailureUnsupported || d.Loc() != loc || d.Code() != UnsupportedSchemaSyntaxCode || len(d.Related()) != 0 || d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 %s = %s, related %v, cause %v", kind, d, d.Related(), err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s/%s/%s: %v", policy, model, kind, err)
				}
				if kind == "any" {
					wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
					if wildcard.NamespaceConstraint().Variety() != WildcardNamespaceConstraintEnumeration || !wildcard.QNameConstraint().Present() {
						t.Fatal("paired empty facts missing")
					}
					continue
				}
				attribute := definitionAnyAttribute(t, requireAnyAttributeComplexType(t, schema.Components()[1], model))
				if attribute.NamespaceConstraint().Variety() != WildcardNamespaceConstraintEnumeration || !attribute.QNameConstraint().Present() {
					t.Fatal("paired empty facts missing")
				}
			}
		}
	}
}

func TestDirectEmptyNamespaceAndQNameAttributeModesRemainBounded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				root := directAnyAttributeSchema("1.1", model, ` namespace="" notQName="" processContents="`+mode+`"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) || len(d.Related()) != 0 || d.SpecRef() != schemaAnyAttributeSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("%s/%s/%s = %s, related %v, cause %v", policy, model, mode, d, d.Related(), err)
				}
			}
		}
	}
}

//nolint:gocognit // Presence and zero-occurrence omission share the same lexical matrix.
func TestDirectEmptyNamespaceAndQNamePresenceAndOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, kind := range []string{"any", "anyAttribute"} {
				for _, present := range []bool{false, true} {
					attributes := ` namespace=""`
					if present {
						attributes += ` notQName=""`
					}
					root := directWildcardSchema("1.1", model, false, attributes)
					if kind == "anyAttribute" {
						root = directAnyAttributeSchema("1.1", model, attributes)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					var q WildcardQNameConstraint
					if kind == "any" {
						q = directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle()).QNameConstraint()
					}
					if kind == "anyAttribute" {
						q = definitionAnyAttribute(t, requireAnyAttributeComplexType(t, schema.Components()[1], model)).QNameConstraint()
					}
					wantLoc := Loc{}
					if present && kind == "any" {
						wantLoc = wildcardParticleTestLoc(t, root, `notQName=""`)
					}
					if present && kind == "anyAttribute" {
						wantLoc = anyAttributeTestLoc(root, `notQName=""`)
					}
					if q.Present() != present || q.Loc() != wantLoc || len(q.Tokens()) != 0 || len(q.Names()) != 0 {
						t.Fatalf("%s/%s/%s/%t QName facts = %#v", policy, model, kind, present, q)
					}
				}
			}
			for _, lexical := range []string{"", "&#x9;&#xA;&#xD; "} {
				root := directWildcardSchema("1.1", model, false, ` namespace="`+lexical+`" notQName="`+lexical+`" minOccurs="0" maxOccurs="0"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				definition := directWildcardDefinition(t, schema)
				var terms []Particle
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					terms = particle.Alternatives()
				case SequenceParticle:
					terms = particle.Particles()
				default:
					t.Fatalf("particle = %T", particle)
				}
				if len(terms) != 3 {
					t.Fatalf("0/0 wildcard left %d terms", len(terms))
				}
				assertEmptyWildcardTermOrder(t, terms)
			}
		}
	}
}

//nolint:gocognit // Test each public consumer at its own unsupported boundary.
func TestDirectEmptyNamespaceAndQNameConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, kind := range []string{"any", "anyAttribute"} {
				root := directWildcardSchema("1.1", model, false, ` namespace="" notQName=""`)
				if kind == "anyAttribute" {
					root = directAnyAttributeSchema("1.1", model, ` namespace="" notQName=""`)
					root = strings.Replace(root, `targetNamespace="urn:root"`, `xmlns:t="urn:root" targetNamespace="urn:root"`, 1)
					root = strings.Replace(root, `</xs:schema>`, `<xs:element name="root" type="t:directType"/></xs:schema>`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				v := requireDiagnostic(t, validationErr)
				var wildcardLoc Loc
				if kind == "any" {
					wildcardLoc = wildcardParticleTestLoc(t, root, `<xs:any`)
				}
				if kind == "anyAttribute" {
					wildcardLoc = anyAttributeTestLoc(root, `<xs:anyAttribute`)
				}
				wantValidationLoc := mustTestLoc(t, "instance.xml", 1, 1)
				if model == "choice" {
					wantValidationLoc = wildcardLoc
				}
				if v.Class() != FailureUnsupported || v.Code() != UnsupportedInstanceValidationCode || v.Loc() != wantValidationLoc || v.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !negativeWildcardRelatedHas(v.Related(), wildcardLoc) || !errors.Is(validationErr, ErrUnsupported) {
					t.Fatalf("validation = %s, cause %v", v, validationErr)
				}
				if kind == "any" {
					wantCause := errInstanceChoiceWildcard
					if model == "sequence" {
						wantCause = errInstanceSequenceWildcard
					}
					if !errors.Is(validationErr, wantCause) {
						t.Fatalf("validation cause = %v", validationErr)
					}
				}
				output, generationErr := GenerateGo(schema, "generated")
				g := requireDiagnostic(t, generationErr)
				wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
				if model == "sequence" {
					wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
				}
				wantGenerationRelated := wildcardLoc
				if kind == "anyAttribute" {
					wantGenerationRelated = anyAttributeTestLoc(root, `<xs:`+model+`>`)
				}
				if output != nil || g.Class() != FailureUnsupported || g.Code() != diagnosticCodegenUnsupported || g.Loc() != wildcardLoc || g.SpecRef() != wantGenerationSpec || !negativeWildcardRelatedHas(g.Related(), wantGenerationRelated) || !errors.Is(generationErr, ErrUnsupported) {
					t.Fatalf("generation = %q/%s, cause %v", output, g, generationErr)
				}
				if kind == "any" {
					wantCause := errCodegenDirectChoiceWildcard
					if model == "sequence" {
						wantCause = errCodegenDirectSequenceWildcard
					}
					if !errors.Is(generationErr, wantCause) {
						t.Fatalf("generation cause = %v", generationErr)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Retain graph adoption, source locations, and discovery order in one fixture.
func TestDirectEmptyNamespaceAndQNameGraph(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Root"><xs:sequence><xs:any namespace="" notQName=""/></xs:sequence><xs:anyAttribute namespace="" notQName=""/></xs:complexType></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Included"><xs:choice><xs:any namespace="&#x9;" notQName="&#xA;"/></xs:choice><xs:anyAttribute namespace="&#xD;" notQName="&#x9;"/></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Other"><xs:sequence><xs:any namespace="" notQName=""/></xs:sequence><xs:anyAttribute namespace="" notQName=""/></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "other.xsd": {id: "other.xsd", contents: other}}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var previous []Component
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatal(err)
			}
			documents := schema.Documents()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" {
				t.Fatalf("discovery order = %v", documents)
			}
			components := schema.Components()
			if len(components) != 3 || components[0].Name() != mustTestQName(t, "urn:root", "Root") || components[1].Name() != mustTestQName(t, "urn:root", "Included") || components[2].Name() != mustTestQName(t, "urn:other", "Other") || run > 0 && !reflect.DeepEqual(components, previous) {
				t.Fatalf("graph components = %v", components)
			}
			previous = components
			var walked []ComponentID
			if err := schema.Walk(func(c Component) error { walked = append(walked, c.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			for i, component := range components {
				if walked[i] != component.ID() {
					t.Fatalf("walk order = %v", walked)
				}
			}
			for i, source := range []struct {
				text, anyNamespace, anyQName, attributeNamespace, attributeQName, id string
			}{
				{root, `namespace=""`, `notQName=""`, `namespace=""`, `notQName=""`, "root.xsd"},
				{included, `namespace="&#x9;"`, `notQName="&#xA;"`, `namespace="&#xD;"`, `notQName="&#x9;"`, "included.xsd"},
				{other, `namespace=""`, `notQName=""`, `namespace=""`, `notQName=""`, "other.xsd"},
			} {
				definition, ok := components[i].ComplexTypeDefinition()
				if !ok {
					t.Fatalf("component %d has no complex type", i)
				}
				particle := directWildcardFromParticle(t, definition.Particle())
				attribute := definitionAnyAttribute(t, definition)
				for _, facts := range []struct {
					ns                       WildcardNamespaceConstraint
					q                        WildcardQNameConstraint
					marker, namespace, qname string
				}{
					{particle.NamespaceConstraint(), particle.QNameConstraint(), `<xs:any `, source.anyNamespace, source.anyQName},
					{attribute.NamespaceConstraint(), attribute.QNameConstraint(), `<xs:anyAttribute `, source.attributeNamespace, source.attributeQName},
				} {
					start := strings.Index(source.text, facts.marker)
					if start < 0 {
						t.Fatalf("wildcard marker %q absent", facts.marker)
					}
					content := source.text[start:]
					wantNS := Loc{source: SourceID(source.id), line: 1, column: len([]rune(source.text[:start+strings.Index(content, facts.namespace)])) + 1}
					wantQName := Loc{source: SourceID(source.id), line: 1, column: len([]rune(source.text[:start+strings.Index(content, facts.qname)])) + 1}
					if facts.ns.Variety() != WildcardNamespaceConstraintEnumeration || len(facts.ns.Namespaces()) != 0 || facts.ns.Loc() != wantNS || !facts.q.Present() || facts.q.Loc() != wantQName || len(facts.q.Tokens()) != 0 || len(facts.q.Names()) != 0 {
						t.Fatalf("%s wildcard facts = %v/%v", source.id, facts.ns, facts.q)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Alternate exits share the same newly admitted mapping boundary.
func TestDirectEmptyNamespaceAndQNamePriorDiagnostics(t *testing.T) {
	cases := []struct {
		name, attributes, primary, related, code, spec string
		cause                                          error
	}{
		{"malformed namespace", ` namespace="##bogus" notQName=""`, `namespace="##bogus"`, "", invalidSchemaCompositionCode, "", nil},
		{"malformed QName", ` namespace="" notQName="bad:q:name"`, `notQName="bad:q:name"`, "", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound QName", ` namespace="" notQName="missing:item"`, `notQName="missing:item"`, "", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"namespace conflict", ` namespace="" notNamespace="##local" notQName=""`, `namespace=""`, "", invalidSchemaCompositionCode, "", nil},
		{"QName membership", ` namespace="" notQName="xs:string"`, `notQName="xs:string"`, `namespace=""`, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace},
		{"bad process", ` namespace="" notQName="" processContents="oops"`, `processContents="oops"`, "", invalidSchemaCompositionCode, "", nil},
		{"duplicate namespace", ` namespace="" namespace="" notQName=""`, `namespace="" notQName=""`, "", InvalidXMLSyntaxCode, "", nil},
		{"duplicate QName", ` namespace="" notQName="" notQName=""`, `notQName=""/>`, "", InvalidXMLSyntaxCode, "", nil},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, kind := range []string{"any", "anyAttribute"} {
				for _, test := range cases {
					t.Run(string(policy)+"/"+model+"/"+kind+"/"+test.name, func(t *testing.T) {
						root := directWildcardSchema("1.1", model, false, test.attributes)
						if kind == "anyAttribute" {
							root = directAnyAttributeSchema("1.1", model, test.attributes)
						}
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						loc := func(marker string) Loc {
							if kind == "any" {
								return wildcardParticleTestLoc(t, root, marker)
							}
							return anyAttributeTestLoc(root, marker)
						}
						var related []Loc
						if test.related != "" {
							related = []Loc{loc(test.related)}
						}
						if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != loc(test.primary) || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != test.spec {
							t.Fatalf("diagnostic = %s related %v, want %s at %s related %v spec %q", d, d.Related(), test.code, loc(test.primary), related, test.spec)
						}
						if test.cause != nil && !errors.Is(err, test.cause) || test.cause == nil && d.Unwrap() != nil {
							t.Fatalf("cause = %v", d.Unwrap())
						}
					})
				}
			}
		}
	}
}

func TestDirectEmptyNamespaceAndQNameReferencePrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := directWildcardSchemaWithTerms("1.1", model, false,
				` namespace="" notQName="" minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:missing"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceUnresolvedCode || d.Loc() != wildcardParticleTestLoc(t, root, `ref="t:missing"`) || len(d.Related()) != 0 || d.SpecRef() != schemaElementReferenceSpecRef(XSDVersion11) || !errors.Is(err, errSchemaElementReferenceUnresolved) {
				t.Fatalf("%s/%s reference = %s, related %v, cause %v", policy, model, d, d.Related(), err)
			}
		}
	}
}
