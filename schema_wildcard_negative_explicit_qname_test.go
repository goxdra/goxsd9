package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func negativeExplicitQNameSchema(kind, model, version, attributes string) string {
	particle := `<xs:` + model + `><xs:element name="first" type="xs:integer"/>`
	if kind == "any" {
		particle += `<xs:any` + attributes + `/>`
	}
	particle += `</xs:` + model + `>`
	if kind == "anyAttribute" {
		particle += `<xs:anyAttribute` + attributes + `/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:p="urn:p" xmlns:q="urn:p" targetNamespace="urn:root" version="` + version + `"><xs:element name="root" type="t:Record"/><xs:complexType name="Record">` + particle + `</xs:complexType></xs:schema>`
}

func wildcardParticleTestLocOptional(t *testing.T, root, marker string) Loc {
	t.Helper()
	if !strings.Contains(root, marker) {
		return Loc{}
	}
	return wildcardParticleTestLoc(t, root, marker)
}

//nolint:gocognit,funlen // Public location, order, and copied wildcard facts share one fixture.
func negativeExplicitQNameFacts(t *testing.T, root, kind, bounds, mode string, schema Schema) {
	t.Helper()
	components := schema.Components()
	if len(components) != 2 || components[0].Name() != mustTestQName(t, "urn:root", "root") || components[1].Name() != mustTestQName(t, "urn:root", "Record") {
		t.Fatalf("component discovery order = %v", components)
	}
	var walked []ComponentID
	if err := schema.Walk(func(component Component) error {
		walked = append(walked, component.ID())
		return nil
	}); err != nil || !reflect.DeepEqual(walked, []ComponentID{components[0].ID(), components[1].ID()}) {
		t.Fatalf("walk order = %v, error %v", walked, err)
	}
	definition, ok := components[1].ComplexType()
	if !ok {
		t.Fatal("Record has no complex type")
	}
	var terms []Particle
	switch model := definition.Particle().(type) {
	case ChoiceParticle:
		terms = model.Alternatives()
	case SequenceParticle:
		terms = model.Particles()
	default:
		t.Fatalf("direct particle = %T", definition.Particle())
	}
	wantTerms := 1
	if kind == "any" {
		wantTerms = 2
	}
	if len(terms) != wantTerms || terms[0].Loc() != wildcardParticleTestLoc(t, root, `<xs:element name="first"`) ||
		terms[0].Occurrences().String() != "1/1" || definition.Particle().Occurrences().String() != "1/1" {
		t.Fatalf("direct lexical terms = %#v", terms)
	}
	var namespace WildcardNamespaceConstraint
	var qname WildcardQNameConstraint
	if kind == "any" {
		wildcard := directWildcardFromParticle(t, definition.Particle())
		if wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:any `) || wildcard.Occurrences().String() != bounds || wildcard.ProcessContents() != mode {
			t.Fatalf("element wildcard location/bounds/mode = %s/%s/%s", wildcard.Loc(), wildcard.Occurrences(), wildcard.ProcessContents())
		}
		namespace, qname = wildcard.NamespaceConstraint(), wildcard.QNameConstraint()
		if wildcard.NamespaceLoc() != namespace.Loc() || wildcard.ProcessContentsLoc() != wildcardParticleTestLocOptional(t, root, `processContents="`+mode+`"`) {
			t.Fatal("element wildcard location provenance changed")
		}
	}
	if kind == "anyAttribute" {
		wildcard := definitionAnyAttribute(t, definition)
		if wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute `) || wildcard.ProcessContents() != mode || wildcard.ProcessContentsLoc() != wildcardParticleTestLocOptional(t, root, `processContents="`+mode+`"`) {
			t.Fatalf("attribute wildcard location/mode = %s/%s", wildcard.Loc(), wildcard.ProcessContents())
		}
		namespace, qname = wildcard.NamespaceConstraint(), wildcard.QNameConstraint()
		if wildcard.NamespaceLoc() != namespace.Loc() {
			t.Fatal("attribute wildcard namespace location changed")
		}
	}
	if namespace.Variety() != WildcardNamespaceConstraintNot || namespace.LexicalForm() != "##local ##targetNamespace urn:z ##local" ||
		!reflect.DeepEqual(namespace.Namespaces(), []string{"", "urn:root", "urn:z"}) || namespace.Loc() != wildcardParticleTestLoc(t, root, `notNamespace="`) {
		t.Fatalf("negative namespace facts = %#v", namespace)
	}
	wantTokens := []string{"p:z", "q:a", "p:z"}
	wantNames := []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z")}
	if !qname.Present() || qname.LexicalForm() != "p:z q:a p:z" || !reflect.DeepEqual(qname.Tokens(), wantTokens) ||
		!reflect.DeepEqual(qname.Names(), wantNames) || qname.Loc() != wildcardParticleTestLoc(t, root, `notQName="`) ||
		!wildcardQNameBindingIs(qname.NamespaceBindings(), "p", "urn:p") || !wildcardQNameBindingIs(qname.NamespaceBindings(), "q", "urn:p") {
		t.Fatalf("QName facts = %#v", qname)
	}
	namespaces := namespace.Namespaces()
	namespaces[0] = "changed"
	tokens := qname.Tokens()
	tokens[0] = "changed"
	names := qname.Names()
	names[0] = QName{}
	bindings := qname.NamespaceBindings()
	bindings[0].Namespace = "changed"
	if namespace.Namespaces()[0] != "" || qname.Tokens()[0] != "p:z" || !reflect.DeepEqual(qname.Names(), wantNames) ||
		qname.NamespaceBindings()[0].Namespace == "changed" {
		t.Fatal("wildcard query leaked mutable facts")
	}
}

//nolint:gocognit // Cross the two wildcard kinds and direct models with edition, policy, and bounds.
func TestNegativeExplicitQNameDirectFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, kind := range []string{"any", "anyAttribute"} {
				for _, model := range []string{"choice", "sequence"} {
					for _, bounds := range []string{"", ` minOccurs="2" maxOccurs="4"`} {
						if kind == "anyAttribute" && bounds != "" {
							continue
						}
						t.Run(string(policy)+"/"+version+"/"+kind+"/"+model+"/"+bounds, func(t *testing.T) {
							attributes := ` notNamespace="##local ##targetNamespace urn:z ##local" notQName="p:z q:a p:z"` + bounds
							root := negativeExplicitQNameSchema(kind, model, version, attributes)
							for run := 0; run < 2; run++ {
								schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
								if err != nil {
									t.Fatalf("parse run %d: %v", run, err)
								}
								wantBounds := "1/1"
								if bounds != "" {
									wantBounds = "2/4"
								}
								negativeExplicitQNameFacts(t, root, kind, wantBounds, "strict", schema)
							}
						})
					}
				}
			}
		}
	}
}

//nolint:gocognit // The new mode admission must retain all previously immutable strict facts.
func TestNegativeExplicitQNameLaxSkipDirectFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, kind := range []string{"any", "anyAttribute"} {
				for _, model := range []string{"choice", "sequence"} {
					for _, mode := range []string{"lax", "skip"} {
						bounds := ""
						if kind == "any" {
							bounds = ` minOccurs="2" maxOccurs="18446744073709551617"`
						}
						attributes := ` notNamespace="##local ##targetNamespace urn:z ##local" notQName="p:z q:a p:z" processContents="` + mode + `"` + bounds
						root := negativeExplicitQNameSchema(kind, model, version, attributes)
						for run := 0; run < 2; run++ {
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
							if err != nil {
								t.Fatalf("%s/%s/%s/%s/%s run %d: %v", policy, version, kind, model, mode, run, err)
							}
							wantBounds := "1/1"
							if kind == "any" {
								wantBounds = "2/18446744073709551617"
							}
							negativeExplicitQNameFacts(t, root, kind, wantBounds, mode, schema)
						}
					}
				}
			}
		}
	}
}

//nolint:gocognit // Both direct models and policies must preserve 0/0 omission.
func TestNegativeExplicitQNameLaxSkipZeroOccurrenceOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				root := negativeExplicitQNameSchema("any", model, "1.1", ` notNamespace="##local" notQName="p:a" processContents="`+mode+`" minOccurs="0" maxOccurs="0"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("%s/%s/%s 0/0: %v", policy, model, mode, err)
				}
				definition, ok := schema.Components()[1].ComplexType()
				if !ok {
					t.Fatal("Record has no complex type")
				}
				var terms []Particle
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					terms = particle.Alternatives()
				case SequenceParticle:
					terms = particle.Particles()
				default:
					t.Fatalf("direct particle = %T", definition.Particle())
				}
				if len(terms) != 1 || terms[0].Loc() != wildcardParticleTestLoc(t, root, `<xs:element name="first"`) {
					t.Fatalf("%s/%s/%s terms after omission = %#v", policy, model, mode, terms)
				}
			}
		}
	}
}

func TestNegativeExplicitQNameZeroOccurrenceOmission(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		root := negativeExplicitQNameSchema("any", model, "1.1", ` notNamespace="##targetNamespace" notQName="##definedSibling ##defined p:x" minOccurs="0" maxOccurs="0"`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		definition, ok := schema.Components()[1].ComplexType()
		if !ok {
			t.Fatalf("%s has no complex type", model)
		}
		var terms []Particle
		if choice, ok := definition.Particle().(ChoiceParticle); ok {
			terms = choice.Alternatives()
		}
		if sequence, ok := definition.Particle().(SequenceParticle); ok {
			terms = sequence.Particles()
		}
		if len(terms) != 1 {
			t.Fatalf("%s terms after omission = %v", model, terms)
		}
		if _, ok := terms[0].(ElementParticle); !ok {
			t.Fatalf("%s retained unexpected term %T", model, terms[0])
		}
	}
}

//nolint:gocognit // Each alternate exit must retain its code, source locations, cause, and edition reference.
func TestNegativeExplicitQNameDiagnostics(t *testing.T) {
	tests := []struct {
		name, kind, attributes, marker, related, spec, code string
		policy                                              LanguagePolicy
		class                                               FailureClass
		cause                                               error
	}{
		{"local member", "any", ` notNamespace="##local" notQName="p:z local"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"target member", "anyAttribute", ` notNamespace="##targetNamespace" notQName="t:record"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"URI member", "any", ` notNamespace="urn:p" notQName="p:z q:a"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"attribute URI member", "anyAttribute", ` notNamespace="urn:p" notQName="q:a p:z"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"zero before omission", "any", ` notNamespace="##targetNamespace" notQName="t:x" minOccurs="0" maxOccurs="0"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"lax before mode", "any", ` notNamespace="##local" processContents="lax" notQName="local"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"skip before mode", "anyAttribute", ` notNamespace="##local" processContents="skip" notQName="local"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"strict10 element", "any", ` notNamespace="##local" notQName="p:a"`, `notNamespace=`, "", "xsd11-structures#cSchemaDocument", UnsupportedSchemaSyntaxCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"strict10 attribute", "anyAttribute", ` notNamespace="##local" notQName="p:a"`, `notNamespace=`, "", "xsd11-structures#cSchemaDocument", UnsupportedSchemaSyntaxCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"conflict element", "any", ` notNamespace="##local" namespace="##any" notQName="p:a"`, `namespace=`, "", "", invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"conflict attribute", "anyAttribute", ` notNamespace="##local" namespace="##any" notQName="p:a"`, `namespace=`, "", "", invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"malformed element", "any", ` notNamespace="##local" notQName="bad:q:name"`, `notQName=`, "", schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode, Strict11, FailureInvalid, nil},
		{"unbound attribute", "anyAttribute", ` notNamespace="##local" notQName="bad:name"`, `notQName=`, "", schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode, Strict11, FailureInvalid, nil},
		{"bad negative", "any", ` notNamespace="##any" notQName="p:a"`, `notNamespace=`, "", "", invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"bad process", "anyAttribute", ` notNamespace="##local" processContents="other" notQName="p:a"`, `processContents=`, "", "", invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"bad bounds", "any", ` notNamespace="##local" notQName="p:a" minOccurs="2" maxOccurs="1"`, `<xs:any `, `minOccurs=`, "xsd11-structures#coss-particle", invalidSchemaCompositionCode, Strict11, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
		{"symbol local member zero", "any", ` notNamespace="##local" notQName="##definedSibling local" minOccurs="0" maxOccurs="0"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"symbol target member", "anyAttribute", ` notNamespace="##targetNamespace" notQName="##defined t:record"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"symbol URI member", "any", ` notNamespace="urn:p" notQName="##definedSibling p:z"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"symbol lax membership", "any", ` notNamespace="##local" processContents="lax" notQName="##definedSibling local"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"symbol skip membership", "anyAttribute", ` notNamespace="##local" processContents="skip" notQName="##defined local"`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"symbol strict10 element", "any", ` notNamespace="##local" notQName="##definedSibling p:a"`, `notNamespace=`, "", "xsd11-structures#cSchemaDocument", UnsupportedSchemaSyntaxCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"symbol strict10 attribute", "anyAttribute", ` notNamespace="##local" notQName="##defined p:a"`, `notNamespace=`, "", "xsd11-structures#cSchemaDocument", UnsupportedSchemaSyntaxCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"symbol malformed element", "any", ` notNamespace="##local" notQName="##definedSibling bad:q:name"`, `notQName=`, "", schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode, Strict11, FailureInvalid, nil},
		{"symbol unbound attribute", "anyAttribute", ` notNamespace="##local" notQName="##defined bad:name"`, `notQName=`, "", schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode, Strict11, FailureInvalid, nil},
		{"symbol conflict", "any", ` notNamespace="##local" namespace="##any" notQName="##definedSibling p:a"`, `namespace=`, "", "", invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"symbol forbidden sibling", "anyAttribute", ` notNamespace="##local" notQName="##definedSibling p:a"`, `notQName=`, "", "", invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"symbol bounds", "any", ` notNamespace="##local" notQName="##definedSibling p:a" minOccurs="2" maxOccurs="1"`, `<xs:any `, `minOccurs=`, "xsd11-structures#coss-particle", invalidSchemaCompositionCode, Strict11, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
	}
	for _, test := range tests {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(test.name+"/"+model, func(t *testing.T) {
				root := negativeExplicitQNameSchema(test.kind, model, "1.1", test.attributes)
				if test.policy == Strict10 {
					root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wildcardParticleTestLoc(t, root, test.marker) || d.SpecRef() != test.spec {
					t.Fatalf("diagnostic = %s/%s, want %s/%s at %s", d, d.SpecRef(), test.class, test.code, wildcardParticleTestLoc(t, root, test.marker))
				}
				wantRelated := []Loc(nil)
				if test.related != "" {
					wantRelated = []Loc{wildcardParticleTestLoc(t, root, test.related)}
				}
				if test.name == "bad bounds" || test.name == "symbol bounds" {
					wantRelated = []Loc{wildcardParticleTestLoc(t, root, `minOccurs=`), wildcardParticleTestLoc(t, root, `maxOccurs=`)}
				}
				if !reflect.DeepEqual(d.Related(), wantRelated) || test.cause != nil && !errors.Is(err, test.cause) || test.cause == nil && d.Unwrap() != nil {
					t.Fatalf("diagnostic related/cause = %v/%v, want %v/%v", d.Related(), d.Unwrap(), wantRelated, test.cause)
				}
			})
		}
	}
}

//nolint:gocognit // Both consumer diagnostics are checked for each wildcard and direct model.
func TestNegativeExplicitQNameConsumers(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			root := negativeExplicitQNameSchema(kind, model, "1.1", ` notNamespace="##local" notQName="p:a"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatal(err)
			}
			definition, ok := schema.Components()[1].ComplexType()
			if !ok {
				t.Fatal("Record has no complex type")
			}
			wildcardLoc := Loc{}
			if kind == "any" {
				wildcardLoc = wildcardParticleTestLoc(t, root, `<xs:any `)
			}
			if kind == "anyAttribute" {
				wildcardLoc = definitionAnyAttribute(t, definition).Loc()
			}
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
				validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !negativeWildcardRelatedHas(validation.Related(), wildcardLoc) ||
				!errors.Is(validationErr, wantValidationCause) {
				t.Fatalf("%s/%s validation = %s", kind, model, validation)
			}
			generated, generationErr := GenerateGo(schema, "generated")
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
			if generated != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
				generation.Loc() != wildcardLoc || generation.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, wantGenerationCause) {
				t.Fatalf("%s/%s generation = %s, output %q", kind, model, generation, generated)
			}
		}
	}
}

//nolint:gocognit // Both kinds and models share strict-exclusion diagnostics.
func TestNegativeExplicitQNameUnsupportedModes(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, form := range []struct{ name, attributes, marker string }{
				{"lax defined", ` notNamespace="##local" processContents="lax" notQName="##defined"`, `notQName=`},
				{"skip sibling", ` notNamespace="##local" processContents="skip" notQName="##definedSibling"`, `notQName=`},
				{"lax mixed", ` notNamespace="##local" processContents="lax" notQName="##defined p:a"`, `notQName=`},
				{"skip mixed", ` notNamespace="##local" processContents="skip" notQName="##definedSibling p:a"`, `notQName=`},
			} {
				if kind == "anyAttribute" && strings.Contains(form.attributes, "##definedSibling") {
					continue
				}
				root := negativeExplicitQNameSchema(kind, model, "1.1", form.attributes)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantLoc := wildcardParticleTestLoc(t, root, form.marker)
				wantSpec := schemaAnyParticleSpecRef(XSDVersion11)
				wantCause := errSchemaAnyParticleUnsupported
				if kind == "anyAttribute" {
					wantLoc = wildcardParticleTestLoc(t, root, `<xs:anyAttribute `)
					wantSpec = schemaAnyAttributeXSD11SpecRef
					wantCause = errSchemaAnyAttributeUnsupported
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != wantLoc ||
					len(d.Related()) != 0 || d.SpecRef() != wantSpec || !errors.Is(err, wantCause) {
					t.Fatalf("%s/%s/%s unsupported = %s, related %v", kind, model, form.name, d, d.Related())
				}
			}
		}
	}
}

//nolint:gocognit // Check one deterministic include/import cycle with both wildcard kinds.
func TestNegativeExplicitQNameGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1"><xs:include schemaLocation="child.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:complexType name="Included"><xs:choice><xs:any notNamespace="##local" notQName="##definedSibling local local"/></xs:choice></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:p" targetNamespace="urn:other" version="1.1"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:sequence/><xs:anyAttribute notNamespace="##targetNamespace" notQName="##defined p:a p:a"/></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":  {id: "root.xsd", contents: root},
		"child.xsd": {id: "child.xsd", contents: child},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("%s run %d: %v", policy, run, err)
			}
			documents := schema.Documents()
			components := schema.Components()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "child.xsd" || documents[2].Source() != "other.xsd" ||
				len(components) != 2 || components[0].Name() != mustTestQName(t, "urn:root", "Included") || components[1].Name() != mustTestQName(t, "urn:other", "Imported") {
				t.Fatalf("graph discovery order = documents %v, components %v", documents, components)
			}
			included, ok := components[0].ComplexType()
			if !ok {
				t.Fatal("included component has no complex type")
			}
			particle := directWildcardFromParticle(t, included.Particle())
			if particle.NamespaceConstraint().Variety() != WildcardNamespaceConstraintNot ||
				!reflect.DeepEqual(particle.NamespaceConstraint().Namespaces(), []string{""}) ||
				!reflect.DeepEqual(particle.QNameConstraint().Tokens(), []string{"##definedSibling", "local", "local"}) ||
				!reflect.DeepEqual(particle.QNameConstraint().Names(), []QName{mustTestQName(t, "urn:root", "local")}) ||
				particle.NamespaceLoc() != wildcardQNameDocumentLoc(t, "child.xsd", child, `notNamespace=`) ||
				particle.QNameConstraint().Loc() != wildcardQNameDocumentLoc(t, "child.xsd", child, `notQName=`) {
				t.Fatalf("included chameleon facts = %#v", particle)
			}
			imported, ok := components[1].ComplexType()
			if !ok {
				t.Fatal("imported component has no complex type")
			}
			attribute := definitionAnyAttribute(t, imported)
			if !reflect.DeepEqual(attribute.NamespaceConstraint().Namespaces(), []string{"urn:other"}) ||
				!reflect.DeepEqual(attribute.QNameConstraint().Tokens(), []string{"##defined", "p:a", "p:a"}) ||
				!reflect.DeepEqual(attribute.QNameConstraint().Names(), []QName{mustTestQName(t, "urn:p", "a")}) ||
				attribute.NamespaceLoc() != wildcardQNameDocumentLoc(t, "other.xsd", other, `notNamespace=`) ||
				attribute.QNameConstraint().Loc() != wildcardQNameDocumentLoc(t, "other.xsd", other, `notQName=`) {
				t.Fatalf("imported facts = %#v", attribute)
			}
		}
	}
}

func TestNegativeExplicitQNameAnnotationOpaque(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		root := negativeExplicitQNameSchema(kind, "sequence", "1.1", ` notNamespace="##local" notQName="p:a"`)
		wildcard := `<xs:` + kind + ` notNamespace="##local" notQName="p:a"/>`
		annotated := `<xs:` + kind + ` notNamespace="##local" notQName="p:a"><xs:annotation><xs:documentation><xs:any notNamespace="##any" notQName="bad:q:name"/></xs:documentation></xs:annotation></xs:` + kind + `>`
		root = strings.Replace(root, wildcard, annotated, 1)
		if !strings.Contains(root, annotated) {
			t.Fatal("annotation fixture missing")
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("%s annotation: %v", kind, err)
		}
		if len(schema.Components()) != 2 {
			t.Fatal("annotation changed component publication")
		}
	}
}

func TestNegativeExplicitQNameReferencePrecedesMembership(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			qname := "##definedSibling local"
			if kind == "anyAttribute" {
				qname = "##defined local"
			}
			root := negativeExplicitQNameSchema(kind, model, "1.1", ` notNamespace="##local" notQName="`+qname+`"`)
			root = strings.Replace(root, `<xs:element name="first" type="xs:integer"/>`, `<xs:element ref="t:missing"/>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceUnresolvedCode ||
				d.Loc() != wildcardParticleTestLoc(t, root, `ref="t:missing"`) || len(d.Related()) != 0 ||
				d.SpecRef() != schemaElementReferenceSpecRef(XSDVersion11) || !errors.Is(err, errSchemaElementReferenceUnresolved) ||
				errors.Is(err, errSchemaWildcardQNameNamespace) {
				t.Fatalf("%s/%s reference precedence = %s, related %v", kind, model, d, d.Related())
			}
		}
	}
}

//nolint:gocognit,funlen // Keep each owner fixture beside its exact membership and unsupported diagnostics.
func TestNegativeExplicitQNameExcludedOwnerMembership(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, shape := range []string{"inline", "ref", "extension", "restriction", "inline-restriction", "named-empty"} {
				if kind == "any" && shape == "named-empty" {
					continue
				}
				for _, excluded := range []string{"t:x", "p:x"} {
					for _, prefix := range []string{"", "##defined "} {
						if kind == "any" && prefix != "" {
							prefix = "##definedSibling "
						}
						wildcard := `<xs:` + kind + ` notNamespace="##targetNamespace" notQName="` + prefix + excluded + `"/>`
						modelXML := `<xs:` + model + `>`
						if kind == "any" {
							modelXML += wildcard
						}
						modelXML += `</xs:` + model + `>`
						if kind == "anyAttribute" {
							modelXML += wildcard
						}
						body := ""
						switch shape {
						case "named-empty":
							body = `<xs:complexType name="Record">` + wildcard + `</xs:complexType>`
						case "inline":
							body = `<xs:element name="root"><xs:complexType>` + modelXML + `</xs:complexType></xs:element>`
						case "ref":
							if kind == "any" {
								body = `<xs:group name="G">` + modelXML + `</xs:group><xs:complexType name="Record"><xs:group ref="t:G"/></xs:complexType>`
							}
							if kind == "anyAttribute" {
								body = `<xs:attributeGroup name="G">` + wildcard + `</xs:attributeGroup><xs:complexType name="Record"><xs:` + model + `/><xs:attributeGroup ref="t:G"/></xs:complexType>`
							}
						case "extension":
							body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base">` + modelXML + `</xs:extension></xs:complexContent></xs:complexType>`
						case "restriction":
							body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:restriction base="t:Base">` + modelXML + `</xs:restriction></xs:complexContent></xs:complexType>`
						case "inline-restriction":
							body = `<xs:complexType name="Base"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + modelXML + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
						}
						root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:p="urn:p" targetNamespace="urn:root" version="1.1">` + body + `</xs:schema>`
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if excluded == "p:x" {
							marker := `<xs:anyAttribute`
							spec := schemaAnyAttributeXSD11SpecRef
							var cause error
							cause = errSchemaAnyAttributeUnsupported
							if kind == "any" {
								marker = `notQName=`
								spec = schemaAnyParticleSpecRef(XSDVersion11)
								cause = errSchemaAnyParticleUnsupported
								if shape == "restriction" || shape == "inline-restriction" {
									marker = `notNamespace=`
									spec = "xsd11-structures#cSchemaDocument"
									cause = nil
								}
								if shape == "ref" || shape == "inline" && model == "choice" {
									marker = `notNamespace=`
									cause = nil
									spec = "xsd11-structures#cSchemaDocument"
								}
							}
							if kind == "anyAttribute" && shape == "extension" {
								marker = `<xs:extension`
								spec = schemaComplexTypeExtensionSpecRef(XSDVersion11)
								cause = nil
							}
							if kind == "anyAttribute" && (shape == "restriction" || shape == "inline-restriction") {
								marker = `<xs:` + model
								spec = "xsd11-structures#cSchemaDocument"
								cause = nil
							}
							if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
								d.Loc() != wildcardParticleTestLoc(t, root, marker) || len(d.Related()) != 0 ||
								d.SpecRef() != spec || cause != nil && !errors.Is(err, cause) || cause == nil && d.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
								t.Fatalf("%s/%s/%s supported QName escaped owner gate: %s related %v spec %s cause %v", kind, model, shape, d, d.Related(), d.SpecRef(), d.Unwrap())
							}
							continue
						}
						if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
							!reflect.DeepEqual(d.Related(), []Loc{wildcardParticleTestLoc(t, root, `notNamespace=`)}) ||
							d.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
							t.Fatalf("%s/%s/%s membership before owner = %s related %v", kind, model, shape, d, d.Related())
						}
					}
				}
			}
		}
	}
}

func negativeExplicitQNameAllOrOpenRoot(owner, attributes string) string {
	wildcard := `<xs:any` + attributes + `/>`
	body := ""
	switch owner {
	case "named-all":
		body = `<xs:complexType name="R"><xs:all>` + wildcard + `</xs:all></xs:complexType>`
	case "anonymous-all":
		body = `<xs:element name="R"><xs:complexType><xs:all>` + wildcard + `</xs:all></xs:complexType></xs:element>`
	case "named-open":
		body = `<xs:complexType name="R"><xs:openContent>` + wildcard + `</xs:openContent></xs:complexType>`
	case "anonymous-open":
		body = `<xs:element name="R"><xs:complexType><xs:openContent>` + wildcard + `</xs:openContent></xs:complexType></xs:element>`
	case "default-open":
		body = `<xs:defaultOpenContent>` + wildcard + `</xs:defaultOpenContent><xs:complexType name="R"><xs:sequence/></xs:complexType>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:p="urn:p" targetNamespace="urn:root" version="1.1">` + body + `</xs:schema>`
}

//nolint:gocognit // Assert exact invalid membership and unchanged unsupported diagnostics for every excluded owner.
func TestNegativeExplicitQNameAllAndOpenOwnerMembership(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, owner := range []string{"named-all", "anonymous-all", "named-open", "anonymous-open", "default-open"} {
			for _, qname := range []string{"t:x", "p:x"} {
				root := negativeExplicitQNameAllOrOpenRoot(owner, ` notNamespace="##targetNamespace" notQName="##definedSibling `+qname+`"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if qname == "t:x" {
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
						!reflect.DeepEqual(d.Related(), []Loc{wildcardParticleTestLoc(t, root, `notNamespace=`)}) ||
						d.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
						t.Fatalf("%s/%s invalid membership = %s related %v spec %s cause %v", policy, owner, d, d.Related(), d.SpecRef(), d.Unwrap())
					}
					continue
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
					d.Loc() != wildcardParticleTestLoc(t, root, `notNamespace=`) || len(d.Related()) != 0 ||
					d.SpecRef() != "xsd11-structures#cSchemaDocument" || d.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("%s/%s valid membership owner gate = %s related %v spec %s cause %v", policy, owner, d, d.Related(), d.SpecRef(), d.Unwrap())
				}
			}
		}
	}
}

func TestNegativeExplicitQNameAllAndOpenOwnerAlternateExits(t *testing.T) {
	tests := []struct {
		name, attributes, marker, spec, code string
	}{
		{"malformed QName", ` notNamespace="##targetNamespace" notQName="bad:q:name"`, `notQName=`, schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode},
		{"unbound QName", ` notNamespace="##targetNamespace" notQName="bad:x"`, `notQName=`, schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode},
		{"conflicting namespace", ` notNamespace="##targetNamespace" namespace="##any" notQName="t:x"`, `namespace=`, "", invalidSchemaCompositionCode},
		{"invalid negative", ` notNamespace="##any" notQName="t:x"`, `notNamespace=`, "", invalidSchemaCompositionCode},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, owner := range []string{"named-all", "anonymous-all", "named-open", "anonymous-open", "default-open"} {
			for _, test := range tests {
				root := negativeExplicitQNameAllOrOpenRoot(owner, test.attributes)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != wildcardParticleTestLoc(t, root, test.marker) ||
					len(d.Related()) != 0 || d.SpecRef() != test.spec || d.Unwrap() != nil {
					t.Fatalf("%s/%s/%s alternate exit = %s related %v spec %s cause %v", policy, owner, test.name, d, d.Related(), d.SpecRef(), d.Unwrap())
				}
			}
		}
	}
}

func TestNegativeExplicitQNameAllAndOpenOwnerStrict10(t *testing.T) {
	for _, owner := range []string{"named-all", "anonymous-all", "named-open", "anonymous-open", "default-open"} {
		root := negativeExplicitQNameAllOrOpenRoot(owner, ` notNamespace="##targetNamespace" notQName="t:x"`)
		root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
			d.Loc() != wildcardParticleTestLoc(t, root, `notNamespace=`) || len(d.Related()) != 0 ||
			d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
			t.Fatalf("%s Strict10 = %s related %v spec %s cause %v", owner, d, d.Related(), d.SpecRef(), d.Unwrap())
		}
	}
}

func TestNegativeExplicitQNameAllAndOpenOwnerResolutionPrecedesMembership(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, owner := range []string{"named-all", "anonymous-all", "named-open", "anonymous-open", "default-open"} {
			root := negativeExplicitQNameAllOrOpenRoot(owner, ` notNamespace="##targetNamespace" notQName="t:x"`)
			root = strings.Replace(root, `version="1.1">`, `version="1.1"><xs:include schemaLocation="missing.xsd"/>`, 1)
			source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("resolver unavailable")
			resolver := &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}
			schema, err := ParseSchemaWithPolicy(source, resolver, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != wildcardParticleTestLoc(t, root, `<xs:include`) ||
				len(d.Related()) != 0 || d.SpecRef() != "" || !errors.Is(err, cause) || len(resolver.calls) != 1 {
				t.Fatalf("%s/%s resolution precedence = %s related %v calls %v cause %v", policy, owner, d, d.Related(), resolver.calls, err)
			}
		}
	}
}
