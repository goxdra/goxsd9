package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // The matrix checks both direct owners against the same public facts.
func TestNegativeSymbolicDirectFacts(t *testing.T) {
	forms := []struct {
		name, lexical string
		tokens        []string
		names         []QName
	}{
		{"defined", "##defined", []string{"##defined"}, nil},
		{"sibling", "##definedSibling", []string{"##definedSibling"}, nil},
		{"both", "##definedSibling ##defined ##definedSibling", []string{"##definedSibling", "##defined", "##definedSibling"}, nil},
		{"mixed", "p:z ##defined q:a ##definedSibling p:z", []string{"p:z", "##defined", "q:a", "##definedSibling", "p:z"}, []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z")}},
		{"attribute-mixed", "p:z ##defined q:a ##defined p:z", []string{"p:z", "##defined", "q:a", "##defined", "p:z"}, []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z")}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, kind := range []string{"any", "anyAttribute"} {
					for _, form := range forms {
						if kind == "anyAttribute" && form.name != "defined" && form.name != "attribute-mixed" || kind == "any" && form.name == "attribute-mixed" {
							continue
						}
						for _, process := range []string{"", ` processContents="strict"`} {
							for _, bounds := range []string{"", ` minOccurs="2" maxOccurs="4"`} {
								if kind == "anyAttribute" && bounds != "" {
									continue
								}
								t.Run(string(policy)+"/"+version+"/"+model+"/"+kind+"/"+form.name+process+bounds, func(t *testing.T) {
									attrs := ` notNamespace="##local ##targetNamespace urn:z ##local" notQName="` + form.lexical + `"` + process + bounds
									root := negativeExplicitQNameSchema(kind, model, version, attrs)
									for run := 0; run < 2; run++ {
										schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
										if err != nil {
											t.Fatalf("run %d: %v", run, err)
										}
										components := schema.Components()
										if len(components) != 2 || components[0].Name() != mustTestQName(t, "urn:root", "root") || components[1].Name() != mustTestQName(t, "urn:root", "Record") {
											t.Fatalf("component order = %v", components)
										}
										var walked []ComponentID
										if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil || !reflect.DeepEqual(walked, []ComponentID{components[0].ID(), components[1].ID()}) {
											t.Fatalf("walk order = %v, error %v", walked, err)
										}
										definition, ok := components[1].ComplexType()
										if !ok {
											t.Fatal("missing named complex type")
										}
										var namespace WildcardNamespaceConstraint
										var qname WildcardQNameConstraint
										var gotProcess string
										var processLoc Loc
										if kind == "any" {
											wildcard := directWildcardFromParticle(t, definition.Particle())
											wantBounds := "1/1"
											if bounds != "" {
												wantBounds = "2/4"
											}
											if wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:any `) || wildcard.Occurrences().String() != wantBounds {
												t.Fatalf("wildcard location/bounds = %s/%s", wildcard.Loc(), wildcard.Occurrences())
											}
											namespace, qname, gotProcess, processLoc = wildcard.NamespaceConstraint(), wildcard.QNameConstraint(), wildcard.ProcessContents(), wildcard.ProcessContentsLoc()
										}
										if kind == "anyAttribute" {
											wildcard := definitionAnyAttribute(t, definition)
											if wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute `) {
												t.Fatalf("wildcard Loc = %s", wildcard.Loc())
											}
											namespace, qname, gotProcess, processLoc = wildcard.NamespaceConstraint(), wildcard.QNameConstraint(), wildcard.ProcessContents(), wildcard.ProcessContentsLoc()
										}
										wantProcessLoc := Loc{}
										if process != "" {
											wantProcessLoc = wildcardParticleTestLoc(t, root, `processContents="strict"`)
										}
										if gotProcess != "strict" || processLoc != wantProcessLoc || namespace.Variety() != WildcardNamespaceConstraintNot ||
											namespace.LexicalForm() != "##local ##targetNamespace urn:z ##local" ||
											!reflect.DeepEqual(namespace.Namespaces(), []string{"", "urn:root", "urn:z"}) ||
											namespace.Loc() != wildcardParticleTestLoc(t, root, `notNamespace=`) ||
											!qname.Present() || qname.LexicalForm() != form.lexical || qname.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
											!reflect.DeepEqual(qname.Tokens(), form.tokens) || !reflect.DeepEqual(qname.Names(), form.names) ||
											!wildcardQNameBindingIs(qname.NamespaceBindings(), "p", "urn:p") {
											t.Fatalf("wildcard facts: namespace %#v, QName %#v, process %s/%s", namespace, qname, gotProcess, processLoc)
										}
										namespaceCopy := namespace.Namespaces()
										namespaceCopy[0] = "changed"
										tokensCopy := qname.Tokens()
										tokensCopy[0] = "changed"
										bindingsCopy := qname.NamespaceBindings()
										bindingsCopy[0].Namespace = "changed"
										if len(form.names) > 0 {
											namesCopy := qname.Names()
											namesCopy[0] = QName{}
										}
										if namespace.Namespaces()[0] != "" || qname.Tokens()[0] != form.tokens[0] ||
											!reflect.DeepEqual(qname.Names(), form.names) || qname.NamespaceBindings()[0].Namespace == "changed" {
											t.Fatal("wildcard accessor leaked mutable facts")
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
}

//nolint:gocognit // Consumer diagnostics differ by wildcard kind and model.
func TestNegativeSymbolicConsumers(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			keyword := "##definedSibling"
			if kind == "anyAttribute" {
				keyword = "##defined"
			}
			root := negativeExplicitQNameSchema(kind, model, "1.1", ` notNamespace="##local" notQName="`+keyword+` p:a"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatal(err)
			}
			wildcardMarker := `<xs:any `
			if kind == "anyAttribute" {
				wildcardMarker = `<xs:anyAttribute `
			}
			wildcardLoc := wildcardParticleTestLoc(t, root, wildcardMarker)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			validation := requireDiagnostic(t, validationErr)
			wantValidationLoc := wildcardLoc
			wantValidationCause := ErrUnsupported
			if kind == "any" && model == "choice" {
				wantValidationCause = errInstanceChoiceWildcard
			}
			if kind == "any" && model == "sequence" {
				wantValidationCause = errInstanceSequenceWildcard
			}
			if model == "sequence" {
				wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantValidationLoc ||
				validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !negativeWildcardRelatedHas(validation.Related(), wildcardLoc) || !errors.Is(validationErr, wantValidationCause) {
				t.Fatalf("%s/%s validation = %s related=%v", kind, model, validation, validation.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
			wantGenerationCause := ErrUnsupported
			if kind == "any" {
				wantGenerationCause = errCodegenDirectChoiceWildcard
			}
			if model == "sequence" {
				wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
				if kind == "any" {
					wantGenerationCause = errCodegenDirectSequenceWildcard
				}
			}
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
				generation.Loc() != wildcardLoc || generation.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, wantGenerationCause) {
				t.Fatalf("%s/%s generation = %s, output %q", kind, model, generation, output)
			}
		}
	}
}
