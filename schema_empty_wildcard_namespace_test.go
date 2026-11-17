package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// The namespace attribute's presence selects enumeration even when its normalized value is empty.
//
//nolint:gocognit,funlen // Keep the edition, process, occurrence, and lexical-presence matrix explicit.
func TestDirectEmptyWildcardNamespaceEnumeration(t *testing.T) {
	forms := []struct {
		name, attributes, marker string
		variety                  WildcardNamespaceConstraintVariety
	}{
		{"omitted", "", "", WildcardNamespaceConstraintAny},
		{"any", ` namespace="##any"`, `namespace="##any"`, WildcardNamespaceConstraintAny},
		{"empty", ` namespace=""`, `namespace=""`, WildcardNamespaceConstraintEnumeration},
		{"whitespace", ` namespace="&#x9;&#xA;&#xD; "`, `namespace="&#x9;&#xA;&#xD; "`, WildcardNamespaceConstraintEnumeration},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, process := range []struct{ name, attributes, want string }{
					{"default", "", "strict"},
					{"strict", ` processContents="strict"`, "strict"},
					{"lax", ` processContents="lax"`, "lax"},
					{"skip", ` processContents="skip"`, "skip"},
				} {
					for _, form := range forms {
						for _, occurrence := range []struct{ name, attributes, want string }{
							{"default", "", "1/1"},
							{"bounded", ` minOccurs="2" maxOccurs="4"`, "2/4"},
							{"omitted", ` minOccurs="0" maxOccurs="0"`, ""},
						} {
							t.Run(string(policy)+"/"+version+"/"+model+"/"+process.name+"/"+form.name+"/"+occurrence.name, func(t *testing.T) {
								root := directWildcardSchema(version, model, false, form.attributes+process.attributes+occurrence.attributes)
								schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
								if err != nil {
									t.Fatalf("parse: %v", err)
								}
								definition := directWildcardDefinition(t, schema)
								particle := definition.Particle()
								var terms []Particle
								switch typed := particle.(type) {
								case ChoiceParticle:
									terms = typed.Alternatives()
								case SequenceParticle:
									terms = typed.Particles()
								default:
									t.Fatalf("particle = %T", particle)
								}
								assertEmptyWildcardTermOrder(t, terms)
								if occurrence.want == "" {
									if len(terms) != 3 {
										t.Fatalf("omitted wildcard left %d terms", len(terms))
									}
									return
								}
								if len(terms) != 4 {
									t.Fatalf("terms = %d, want 4", len(terms))
								}
								wildcard, ok := terms[2].(WildcardParticle)
								if !ok {
									t.Fatalf("term 2 = %T, want wildcard", terms[2])
								}
								wantProcessLoc := Loc{}
								if process.attributes != "" {
									wantProcessLoc = wildcardParticleTestLoc(t, root, process.attributes[1:])
								}
								if wildcard.Loc() != wildcardParticleTestLoc(t, root, "<xs:any") || wildcard.ProcessContents() != process.want || wildcard.ProcessContentsLoc() != wantProcessLoc || wildcard.Occurrences().String() != occurrence.want {
									t.Fatalf("wildcard facts = %v/%s/%s/%s", wildcard.Loc(), wildcard.ProcessContents(), wildcard.ProcessContentsLoc(), wildcard.Occurrences())
								}
								constraint := wildcard.NamespaceConstraint()
								if constraint.Variety() != form.variety || wildcard.Namespace() != constraint.LexicalForm() || constraint.Loc() != wildcard.NamespaceLoc() {
									t.Fatalf("namespace facts = %v/%q/%v", constraint.Variety(), constraint.LexicalForm(), constraint.Loc())
								}
								if form.marker == "" {
									if !constraint.Loc().IsZero() || constraint.LexicalForm() != "##any" {
										t.Fatalf("omitted namespace = %q/%v", constraint.LexicalForm(), constraint.Loc())
									}
								}
								if form.marker != "" && constraint.Loc() != wildcardParticleTestLoc(t, root, form.marker) {
									t.Fatalf("namespace Loc = %v", constraint.Loc())
								}
								if form.variety == WildcardNamespaceConstraintEnumeration && (constraint.LexicalForm() != "" || len(constraint.Namespaces()) != 0) {
									t.Fatalf("empty enumeration = %q/%v", constraint.LexicalForm(), constraint.Namespaces())
								}
								if !reflect.DeepEqual(constraint.Namespaces(), wildcard.NamespaceConstraint().Namespaces()) {
									t.Fatal("non-deterministic namespace query")
								}
								copied := schema.Components()
								copied[0] = Component{}
								if len(schema.Components()) == 0 || schema.Components()[0] == (Component{}) {
									t.Fatal("component query leaked backing storage")
								}
							})
						}
					}
				}
			}
		}
	}
}

func assertEmptyWildcardTermOrder(t *testing.T, terms []Particle) {
	t.Helper()
	if len(terms) < 3 {
		t.Fatalf("terms = %d, want at least 3", len(terms))
	}
	first, firstOK := terms[0].(ElementParticle)
	reference, referenceOK := terms[1].(ElementReferenceParticle)
	last, lastOK := terms[len(terms)-1].(ElementParticle)
	if !firstOK || !referenceOK || !lastOK || first.Name().Local() != "first" || reference.Name().Local() != "shared" || last.Name().Local() != "last" {
		t.Fatalf("lexical term order = %T/%T/%T", terms[0], terms[1], terms[len(terms)-1])
	}
}

//nolint:gocognit // Keep the edition, owner, and namespace-presence matrix explicit.
func TestDirectEmptyAnyAttributeNamespaceEnumeration(t *testing.T) {
	forms := []struct {
		name, attributes, marker string
		variety                  WildcardNamespaceConstraintVariety
	}{
		{"omitted", "", "", WildcardNamespaceConstraintAny},
		{"any", ` namespace="##any"`, `namespace="##any"`, WildcardNamespaceConstraintAny},
		{"empty", ` namespace=""`, `namespace=""`, WildcardNamespaceConstraintEnumeration},
		{"whitespace", ` namespace="&#x9;&#xA;&#xD; "`, `namespace="&#x9;&#xA;&#xD; "`, WildcardNamespaceConstraintEnumeration},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, form := range forms {
					t.Run(string(policy)+"/"+version+"/"+model+"/"+form.name, func(t *testing.T) {
						root := directAnyAttributeSchema(version, model, form.attributes+` processContents="strict"`)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err != nil {
							t.Fatalf("parse: %v", err)
						}
						definition := requireAnyAttributeComplexType(t, schema.Components()[1], model)
						attribute, ok := definition.AnyAttribute()
						if !ok {
							t.Fatal("anyAttribute absent")
						}
						constraint := attribute.NamespaceConstraint()
						if constraint.Variety() != form.variety || attribute.Namespace() != constraint.LexicalForm() || attribute.ProcessContents() != "strict" || attribute.Loc() != anyAttributeTestLoc(root, "<xs:anyAttribute") || attribute.ProcessContentsLoc() != anyAttributeTestLoc(root, `processContents="strict"`) {
							t.Fatalf("attribute facts = %#v", attribute)
						}
						if form.marker == "" {
							if !constraint.Loc().IsZero() || constraint.LexicalForm() != "##any" {
								t.Fatalf("omitted namespace = %q/%v", constraint.LexicalForm(), constraint.Loc())
							}
						}
						if form.marker != "" && (constraint.Loc() != anyAttributeTestLoc(root, form.marker) || attribute.NamespaceLoc() != constraint.Loc()) {
							t.Fatalf("namespace Loc = %v", constraint.Loc())
						}
						if form.variety == WildcardNamespaceConstraintEnumeration && (constraint.LexicalForm() != "" || len(constraint.Namespaces()) != 0) {
							t.Fatalf("empty enumeration = %q/%v", constraint.LexicalForm(), constraint.Namespaces())
						}
						if repeated, ok := definition.AnyAttribute(); !ok || !reflect.DeepEqual(repeated, attribute) {
							t.Fatal("repeated query changed attribute")
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Check both consumer exits for every direct model and policy.
func TestEmptyWildcardConsumersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(model, func(t *testing.T) {
				root := directWildcardSchema("1.1", model, false, ` namespace=""`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				validationCause := errInstanceChoiceWildcard
				if model == "sequence" {
					validationCause = errInstanceSequenceWildcard
				}
				if !errors.Is(validationErr, validationCause) || requireDiagnostic(t, validationErr).Code() != UnsupportedInstanceValidationCode {
					t.Fatalf("validation = %v", validationErr)
				}
				wantValidationLoc := wildcardParticleTestLoc(t, root, "<xs:any")
				if model == "sequence" {
					wantValidationLoc = Loc{source: "instance.xml", line: 1, column: 1}
				}
				selectedVersion := XSDVersion11
				if policy == Strict10 {
					selectedVersion = XSDVersion10
				}
				validation := requireDiagnostic(t, validationErr)
				wildcardLoc := wildcardParticleTestLoc(t, root, "<xs:any")
				if validation.Loc() != wantValidationLoc || validation.SpecRef() != instanceValidationSpecRef(selectedVersion) || !negativeWildcardRelatedHas(validation.Related(), wildcardLoc) {
					t.Fatalf("validation = %s, related %v", validation, validation.Related())
				}
				generated, generationErr := GenerateGo(schema, "generated")
				generationCause := errCodegenDirectChoiceWildcard
				if model == "sequence" {
					generationCause = errCodegenDirectSequenceWildcard
				}
				generation := requireDiagnostic(t, generationErr)
				wantSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
				if model == "sequence" {
					wantSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
				}
				if policy == Strict10 {
					wantSpec = codegenDirectChoiceXSD10ElementChoiceSpecRef
					if model == "sequence" {
						wantSpec = codegenDirectSequenceXSD10ElementSequenceSpecRef
					}
				}
				if generated != nil || !errors.Is(generationErr, generationCause) || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wildcardLoc || generation.SpecRef() != wantSpec || !negativeWildcardRelatedHas(generation.Related(), wildcardLoc) {
					t.Fatalf("generation = %q/%v", generated, generationErr)
				}
			})
		}
	}
}

// A nonempty QName exclusion cannot be a member of an empty namespace enumeration.
//
//nolint:gocognit // Keep the QName, process, owner, and omitted-boundary matrix explicit.
func TestEmptyWildcardQNameMembershipPrecedesOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, bounds := range []string{"", ` minOccurs="0" maxOccurs="0"`} {
				for _, mode := range []string{"strict", "lax", "skip"} {
					t.Run(string(policy)+"/"+model+"/"+mode+"/"+bounds, func(t *testing.T) {
						root := directWildcardSchema("1.1", model, false, ` namespace="" processContents="`+mode+`" notQName="t:root"`+bounds)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						assertZeroSchema(t, schema)
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="t:root"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `namespace=""`)}) || diagnostic.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
							t.Fatalf("QName membership = %s, related %v, cause %v", diagnostic, diagnostic.Related(), err)
						}
					})
				}
			}
			root := directAnyAttributeSchema("1.1", model, ` namespace="" notQName="xs:string"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != anyAttributeTestLoc(root, `notQName="xs:string"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{anyAttributeTestLoc(root, `namespace=""`)}) || diagnostic.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
				t.Fatalf("anyAttribute QName membership = %s, related %v, cause %v", diagnostic, diagnostic.Related(), err)
			}
		}
	}
}

func TestEmptyWildcardQNameStrict10PolicyExit(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		for _, attribute := range []bool{false, true} {
			root := directWildcardSchema("1.1", model, false, ` namespace="" notQName="t:root"`)
			if attribute {
				root = directAnyAttributeSchema("1.1", model, ` namespace="" notQName="xs:string"`)
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			marker := `notQName="t:root"`
			if attribute {
				marker = `notQName="xs:string"`
			}
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, marker) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("policy diagnostic = %s, cause %v", diagnostic, err)
			}
		}
	}
}

//nolint:gocognit // Check discovery order and both wildcard source locations across the graph.
func TestEmptyWildcardNamespaceGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Root"><xs:sequence><xs:any namespace=""/></xs:sequence><xs:anyAttribute namespace=""/></xs:complexType></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Included"><xs:choice><xs:any namespace="&#x9;"/></xs:choice><xs:anyAttribute namespace="&#xA;"/></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Other"><xs:sequence><xs:any namespace=""/></xs:sequence><xs:anyAttribute namespace=""/></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "other.xsd": {id: "other.xsd", contents: other}}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
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
			if len(components) != 3 || components[0].Name() != mustTestQName(t, "urn:root", "Root") || components[1].Name() != mustTestQName(t, "urn:root", "Included") || components[2].Name() != mustTestQName(t, "urn:other", "Other") {
				t.Fatalf("component order = %v", components)
			}
			for _, test := range []struct {
				namespace, local, source string
				content                  string
				namespaceMarker          string
				attributeMarker          string
			}{
				{"urn:root", "Root", "root.xsd", root, `namespace=""`, `namespace=""`},
				{"urn:root", "Included", "included.xsd", included, `namespace="&#x9;"`, `namespace="&#xA;"`},
				{"urn:other", "Other", "other.xsd", other, `namespace=""`, `namespace=""`},
			} {
				components := schema.Find(mustTestQName(t, test.namespace, test.local))
				if len(components) != 1 {
					t.Fatalf("%s components = %d", test.local, len(components))
				}
				definition, ok := components[0].ComplexTypeDefinition()
				if !ok {
					t.Fatalf("%s has no complex type definition", test.local)
				}
				wildcard := directWildcardFromParticle(t, definition.Particle())
				attribute, ok := definition.AnyAttribute()
				if !ok {
					t.Fatal("anyAttribute absent")
				}
				for index, constraint := range []WildcardNamespaceConstraint{wildcard.NamespaceConstraint(), attribute.NamespaceConstraint()} {
					wantLoc := anyAttributeTestLocForSource(SourceID(test.source), test.content, test.namespaceMarker)
					if index == 1 {
						wantLoc = Loc{source: SourceID(test.source), line: 1, column: len([]rune(test.content[:strings.LastIndex(test.content, test.attributeMarker)])) + 1}
					}
					if constraint.Variety() != WildcardNamespaceConstraintEnumeration || len(constraint.Namespaces()) != 0 || constraint.Loc() != wantLoc {
						t.Fatalf("%s constraint = %v/%v/%v, want %v", test.local, constraint.Variety(), constraint.Namespaces(), constraint.Loc(), wantLoc)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Check both consumer exits for each attribute owner and policy.
func TestEmptyAnyAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := attributeQNameSchema(model, ` namespace=""`, "")
			root = strings.Replace(root, `</xs:schema>`, `<xs:element name="root" type="t:Record"/></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			wildcard := attributeQNameFact(t, schema)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:t"/>`)))
			wantValidationLoc := Loc{source: "instance.xml", line: 1, column: 1}
			if model == "choice" {
				wantValidationLoc = wildcard.Loc()
			}
			validation := requireDiagnostic(t, validationErr)
			selectedVersion := XSDVersion11
			if policy == Strict10 {
				selectedVersion = XSDVersion10
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantValidationLoc || !negativeWildcardRelatedHas(validation.Related(), wildcard.Loc()) || validation.SpecRef() != instanceValidationSpecRef(selectedVersion) || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation = %s, related %v", validation, validation.Related())
			}
			generated, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
			if model == "sequence" {
				wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
			}
			if policy == Strict10 {
				wantGenerationSpec = codegenDirectChoiceXSD10ElementChoiceSpecRef
				if model == "sequence" {
					wantGenerationSpec = codegenDirectSequenceXSD10ElementSequenceSpecRef
				}
			}
			if generated != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wildcard.Loc() || generation.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("generation = %q/%s", generated, generation)
			}
		}
	}
}

// Empty lists do not open unrelated owners; each excluded shape still returns no schema.
//
//nolint:gocognit // Keep explicit excluded owners beside their located diagnostics.
func TestEmptyWildcardNamespaceExcludedShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, shape := range []struct{ name, body, marker string }{
				{"extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:` + model + `><xs:any namespace=""/></xs:` + model + `></xs:extension></xs:complexContent></xs:complexType>`, `namespace=""`},
				{"inline", `<xs:element name="root"><xs:complexType><xs:` + model + `><xs:any namespace=""/></xs:` + model + `></xs:complexType></xs:element>`, `namespace=""`},
				{"group", `<xs:group name="G"><xs:` + model + `><xs:any namespace=""/></xs:` + model + `></xs:group>`, `namespace=""`},
				{"group_ref", `<xs:group name="G"><xs:` + model + `><xs:any namespace=""/></xs:` + model + `></xs:group><xs:complexType name="Record"><xs:group ref="t:G"/></xs:complexType>`, `namespace=""`},
				{"restriction", `<xs:complexType name="Base"><xs:sequence/></xs:complexType><xs:complexType name="Record"><xs:complexContent><xs:restriction base="t:Base"><xs:` + model + `><xs:any namespace=""/></xs:` + model + `></xs:restriction></xs:complexContent></xs:complexType>`, `namespace=""`},
				{"inline_restriction", `<xs:complexType name="Base"><xs:sequence/></xs:complexType><xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base"><xs:` + model + `><xs:any namespace=""/></xs:` + model + `></xs:restriction></xs:complexContent></xs:complexType></xs:element>`, `namespace=""`},
			} {
				t.Run(string(policy)+"/"+model+"/"+shape.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">` + shape.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
						t.Fatalf("%s diagnostic = %s", shape.name, diagnostic)
					}
					if diagnostic.Loc() != wildcardParticleTestLoc(t, root, shape.marker) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s location/cause = %v/%v", shape.name, diagnostic.Loc(), err)
					}
					selectedVersion := XSDVersion11
					if policy == Strict10 {
						selectedVersion = XSDVersion10
					}
					wantSpec := schemaAnyParticleSpecRef(selectedVersion)
					wantWildcardCause := true
					if shape.name == "group" || shape.name == "group_ref" || strings.Contains(shape.name, "restriction") {
						wantWildcardCause = false
						wantSpec = "xsd11-structures#cSchemaDocument"
						if policy == Strict10 {
							wantSpec = "xsd10-structures#schema-document"
						}
					}
					if shape.name == "inline" && model == "choice" {
						wantWildcardCause = false
						wantSpec = "xsd10-structures#schema-document"
					}
					if diagnostic.SpecRef() != wantSpec || errors.Is(err, errSchemaAnyParticleUnsupported) != wantWildcardCause {
						t.Fatalf("%s spec/cause = %q/%v", shape.name, diagnostic.SpecRef(), err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep direct, inline, reference, and derivation exclusions explicit.
func TestEmptyAnyAttributeNamespaceExcludedShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, shape := range []struct{ name, body string }{
				{"named_no_model", `<xs:complexType name="Record"><xs:anyAttribute namespace=""/></xs:complexType>`},
				{"inline", `<xs:element name="root"><xs:complexType><xs:` + model + `/><xs:anyAttribute namespace=""/></xs:complexType></xs:element>`},
				{"attribute_group_ref", `<xs:attributeGroup name="G"><xs:anyAttribute namespace=""/></xs:attributeGroup><xs:complexType name="Record"><xs:` + model + `/><xs:attributeGroup ref="t:G"/></xs:complexType>`},
				{"extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:` + model + `/><xs:anyAttribute namespace=""/></xs:extension></xs:complexContent></xs:complexType>`},
				{"restriction", `<xs:complexType name="Base"><xs:` + model + `/><xs:anyAttribute namespace="##any"/></xs:complexType><xs:complexType name="Record"><xs:complexContent><xs:restriction base="t:Base"><xs:anyAttribute namespace="" processContents="strict"/></xs:restriction></xs:complexContent></xs:complexType>`},
				{"inline_restriction", `<xs:complexType name="Base"><xs:` + model + `/><xs:anyAttribute namespace="##any"/></xs:complexType><xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base"><xs:anyAttribute namespace="" processContents="strict"/></xs:restriction></xs:complexContent></xs:complexType></xs:element>`},
			} {
				t.Run(string(policy)+"/"+model+"/"+shape.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">` + shape.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := anyAttributeTestLoc(root, `<xs:anyAttribute`)
					if strings.Contains(shape.name, "restriction") {
						wantLoc = Loc{source: "root.xsd", line: 1, column: len([]rune(root[:strings.LastIndex(root, `<xs:anyAttribute`)])) + 1}
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s diagnostic = %s, cause %v", shape.name, diagnostic, err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep syntax and occurrence precedence checks adjacent to their causes.
func TestEmptyWildcardInvalidSyntaxAndOccurrenceBeforeOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, test := range []struct {
				name, attributes, marker string
			}{
				{"namespace_conflict", ` namespace="" notNamespace="##local" minOccurs="0" maxOccurs="0"`, `namespace=""`},
				{"bad_minimum", ` namespace="" minOccurs="oops" maxOccurs="0"`, `minOccurs="oops"`},
				{"reversed_bounds", ` namespace="" minOccurs="2" maxOccurs="1"`, `<xs:any`},
			} {
				t.Run(string(policy)+"/"+model+"/"+test.name, func(t *testing.T) {
					root := directWildcardSchema("1.1", model, false, test.attributes)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.marker) {
						t.Fatalf("diagnostic = %s, want %s", diagnostic, test.marker)
					}
					selectedVersion := XSDVersion11
					if policy == Strict10 {
						selectedVersion = XSDVersion10
					}
					switch test.name {
					case "namespace_conflict":
						if diagnostic.SpecRef() != "" || len(diagnostic.Related()) != 0 || diagnostic.Unwrap() != nil {
							t.Fatalf("namespace conflict = %s, related %v, cause %v", diagnostic, diagnostic.Related(), diagnostic.Unwrap())
						}
					case "bad_minimum":
						var lexical Diagnostic
						if diagnostic.SpecRef() != schemaParticleOccurrenceDatatypeSpecRef(selectedVersion) || len(diagnostic.Related()) != 0 || !errors.As(diagnostic.Unwrap(), &lexical) || lexical.Code() != InvalidIntegerLexicalCode || lexical.Loc() != diagnostic.Loc() {
							t.Fatalf("invalid minimum = %s, related %v, cause %v", diagnostic, diagnostic.Related(), diagnostic.Unwrap())
						}
					case "reversed_bounds":
						wantRelated := []Loc{wildcardParticleTestLoc(t, root, `minOccurs="2"`), wildcardParticleTestLoc(t, root, `maxOccurs="1"`)}
						if diagnostic.SpecRef() != schemaParticleCorrectSpecRef(selectedVersion) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errParticleOccurrenceMinimumExceedsMaximum) {
							t.Fatalf("reversed bounds = %s, related %v, cause %v", diagnostic, diagnostic.Related(), err)
						}
					}
				})
			}
		}
	}
}

func TestEmptyAnyAttributeDefaultProcess(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := directAnyAttributeSchema("1.1", model, ` namespace=""`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			definition := requireAnyAttributeComplexType(t, schema.Components()[1], model)
			attribute, ok := definition.AnyAttribute()
			if !ok {
				t.Fatal("anyAttribute absent")
			}
			if attribute.ProcessContents() != "strict" || !attribute.ProcessContentsLoc().IsZero() || attribute.NamespaceConstraint().Variety() != WildcardNamespaceConstraintEnumeration || len(attribute.NamespaceConstraint().Namespaces()) != 0 || attribute.NamespaceLoc() != anyAttributeTestLoc(root, `namespace=""`) {
				t.Fatalf("default-process attribute = %#v", attribute)
			}
		}
	}
}

func TestEmptyWildcardReferenceFailurePrecedesQNameMembership(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := directWildcardSchemaWithTerms("1.1", model, false, ` namespace="" notQName="t:root" minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:missing"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceUnresolvedCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, `ref="t:missing"`) || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaElementReferenceSpecRef(XSDVersion11) || !errors.Is(err, errSchemaElementReferenceUnresolved) || errors.Is(err, errSchemaWildcardQNameNamespace) {
				t.Fatalf("reference precedence = %s, related %v, cause %v", diagnostic, diagnostic.Related(), err)
			}
		}
	}
}

//nolint:gocognit // Keep each QName combination's phase exit and location explicit.
func TestEmptyWildcardQNameCombinationsStayUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, qname := range []string{"", "##defined", "##definedSibling"} {
				root := directWildcardSchema("1.1", model, false, ` namespace="" notQName="`+qname+`"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="`+qname+`"`) || diagnostic.SpecRef() != schemaAnyParticleSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAnyParticleUnsupported) {
					t.Fatalf("any QName combination = %s, cause %v", diagnostic, err)
				}
				attributeRoot := directAnyAttributeSchema("1.1", model, ` namespace="" notQName="`+qname+`"`)
				attributeSchema, attributeErr := discoverTestSchemaWithPolicy(t, attributeRoot, nil, policy)
				assertZeroSchema(t, attributeSchema)
				attributeDiagnostic := requireDiagnostic(t, attributeErr)
				if qname == "##definedSibling" {
					if attributeDiagnostic.Class() != FailureInvalid || attributeDiagnostic.Code() != invalidSchemaCompositionCode || attributeDiagnostic.Loc() != anyAttributeTestLoc(attributeRoot, `notQName="##definedSibling"`) || attributeDiagnostic.SpecRef() != "" || attributeDiagnostic.Unwrap() != nil {
						t.Fatalf("forbidden anyAttribute token = %s", attributeDiagnostic)
					}
					continue
				}
				if attributeDiagnostic.Class() != FailureUnsupported || attributeDiagnostic.Code() != UnsupportedSchemaSyntaxCode || attributeDiagnostic.Loc() != anyAttributeTestLoc(attributeRoot, `<xs:anyAttribute`) || attributeDiagnostic.SpecRef() != schemaAnyAttributeSpecRef(XSDVersion11) || !errors.Is(attributeErr, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("attribute QName combination = %s, cause %v", attributeDiagnostic, attributeErr)
				}
			}
		}
	}
}
