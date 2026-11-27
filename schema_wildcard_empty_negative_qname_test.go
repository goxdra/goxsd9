package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func emptyNegativeQNameAttributes(qname, process string) string {
	attributes := ` notNamespace="&#xA;urn:z&#x9;##targetNamespace urn:a&#xD;##local urn:z"`
	if qname != "omitted" {
		attributes += ` notQName="` + qname + `"`
	}
	if process != "default" {
		attributes += ` processContents="` + process + `"`
	}
	return attributes
}

func assertEmptyNegativeQNameFacts(t *testing.T, root, qname string, namespace WildcardNamespaceConstraint, constraint WildcardQNameConstraint) {
	t.Helper()
	if namespace.Variety() != WildcardNamespaceConstraintNot || namespace.LexicalForm() != "urn:z ##targetNamespace urn:a ##local urn:z" ||
		!reflect.DeepEqual(namespace.Namespaces(), []string{"", "urn:a", "urn:root", "urn:z"}) ||
		namespace.Loc() != wildcardParticleTestLoc(t, root, `notNamespace="`) {
		t.Fatalf("namespace facts = %q/%q/%v/%s", namespace.Variety(), namespace.LexicalForm(), namespace.Namespaces(), namespace.Loc())
	}
	if constraint.Present() != (qname != "omitted") || constraint.LexicalForm() != "" || len(constraint.Tokens()) != 0 || len(constraint.Names()) != 0 {
		t.Fatalf("QName facts = %#v", constraint)
	}
	wantLoc := Loc{}
	if qname != "omitted" {
		wantLoc = wildcardParticleTestLoc(t, root, `notQName="`+qname+`"`)
	}
	if constraint.Loc() != wantLoc {
		t.Fatalf("QName loc = %s, want %s", constraint.Loc(), wantLoc)
	}
	names := namespace.Namespaces()
	names[0] = "mutated"
	if !reflect.DeepEqual(namespace.Namespaces(), []string{"", "urn:a", "urn:root", "urn:z"}) {
		t.Fatal("namespace query was not defensive")
	}
	if len(constraint.NamespaceBindings()) > 0 {
		bindings := constraint.NamespaceBindings()
		bindings[0].Namespace = "mutated"
		if reflect.DeepEqual(constraint.NamespaceBindings(), bindings) {
			t.Fatal("QName binding query was not defensive")
		}
	}
}

//nolint:gocognit // Cross policy, edition, model, mode, presence, and bounds with the same facts.
func TestEmptyNegativeQNameElementWildcardFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, process := range []string{"default", "strict", "lax", "skip"} {
					for _, qname := range []string{"omitted", "", "&#x9;&#xA;&#xD; "} {
						for _, bounds := range []string{"", ` minOccurs="2" maxOccurs="4"`} {
							t.Run(string(policy)+"/"+version+"/"+model+"/"+process+"/"+qname+"/"+bounds, func(t *testing.T) {
								attributes := emptyNegativeQNameAttributes(qname, process) + bounds
								root := directWildcardSchema(version, model, false, attributes)
								var previous []Component
								for run := 0; run < 2; run++ {
									schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
									if err != nil {
										t.Fatalf("parse: %v", err)
									}
									components := schema.Components()
									if len(components) == 0 || run > 0 && !reflect.DeepEqual(components, previous) {
										t.Fatalf("component order/facts changed: %v", components)
									}
									previous = components
									wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
									assertEmptyNegativeQNameFacts(t, root, qname, wildcard.NamespaceConstraint(), wildcard.QNameConstraint())
									wantProcess := process
									if process == "default" {
										wantProcess = "strict"
									}
									wantRange := "1/1"
									if bounds != "" {
										wantRange = "2/4"
									}
									if wildcard.ProcessContents() != wantProcess || wildcard.Occurrences().String() != wantRange || wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:any `) {
										t.Fatalf("wildcard process/bounds/loc = %s/%s/%s", wildcard.ProcessContents(), wildcard.Occurrences(), wildcard.Loc())
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

//nolint:gocognit // Cross edition, model, process mode, and QName presence.
func TestEmptyNegativeQNameAttributeWildcardFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, process := range []string{"default", "strict", "lax", "skip"} {
					for _, qname := range []string{"omitted", "", "&#x9;&#xA;&#xD; "} {
						attributes := emptyNegativeQNameAttributes(qname, process)
						root := strings.Replace(negativeAttributeSchema(model, attributes), `targetNamespace="urn:t"`, `targetNamespace="urn:root" version="`+version+`"`, 1)
						root = strings.Replace(root, `xmlns:t="urn:t"`, `xmlns:t="urn:root"`, 1)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err != nil {
							t.Fatalf("%s/%s/%s/%s/%s: parse: %v", policy, version, model, process, qname, err)
						}
						definition := requireAnyAttributeComplexType(t, schema.Components()[0], model)
						wildcard := definitionAnyAttribute(t, definition)
						assertEmptyNegativeQNameFacts(t, root, qname, wildcard.NamespaceConstraint(), wildcard.QNameConstraint())
						wantProcess := process
						if process == "default" {
							wantProcess = "strict"
						}
						processLoc := Loc{}
						if process != "default" {
							processLoc = anyAttributeTestLoc(root, `processContents="`+process+`"`)
						}
						if wildcard.ProcessContents() != wantProcess || wildcard.ProcessContentsLoc() != processLoc || wildcard.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) ||
							!reflect.DeepEqual(definitionAnyAttribute(t, definition), wildcard) {
							t.Fatalf("attribute wildcard facts changed: %#v", wildcard)
						}
					}
				}
			}
		}
	}
}

func TestEmptyNegativeQNameElementZeroBoundsAndAnnotation(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		root := directWildcardSchemaWithTerms("1.1", model, false,
			` notNamespace="##local" notQName="&#x9;" minOccurs="0" maxOccurs="0"`, "", "")
		root = strings.Replace(root, `/>`+`<xs:element name="last"`, `><xs:annotation><xs:documentation>opaque</xs:documentation></xs:annotation></xs:any><xs:element name="last"`, 1)
		if !strings.Contains(root, `<xs:annotation><xs:documentation>opaque</xs:documentation></xs:annotation>`) {
			t.Fatal("annotation fixture was not installed")
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("%s: parse: %v", model, err)
		}
		definition := directWildcardDefinition(t, schema)
		terms := []Particle{}
		switch particle := definition.Particle().(type) {
		case ChoiceParticle:
			terms = particle.Alternatives()
		case SequenceParticle:
			terms = particle.Particles()
		}
		if len(terms) != 1 {
			t.Fatalf("omission left %d terms, want last element only", len(terms))
		}
	}
}

func TestEmptyNegativeQNameConsumersRemainUnsupported(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		root := directWildcardSchemaWithTerms("1.1", model, false, ` notNamespace="##local" notQName=""`, "", "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatal(err)
		}
		wildcard := directWildcardFromParticle(t, directWildcardDefinition(t, schema).Particle())
		validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
		v := requireDiagnostic(t, validationErr)
		wantValidationLoc := wildcard.Loc()
		wantValidationCause := errInstanceChoiceWildcard
		if model == "sequence" {
			wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
			wantValidationCause = errInstanceSequenceWildcard
		}
		if v.Class() != FailureUnsupported || v.Code() != UnsupportedInstanceValidationCode || v.Loc() != wantValidationLoc ||
			v.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(validationErr, wantValidationCause) || !negativeWildcardRelatedHas(v.Related(), wildcard.Loc()) {
			t.Fatalf("validation: %s, related %v", v, v.Related())
		}
		output, generationErr := GenerateGo(schema, "generated")
		g := requireDiagnostic(t, generationErr)
		wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
		wantGenerationCause := errCodegenDirectChoiceWildcard
		if model == "sequence" {
			wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
			wantGenerationCause = errCodegenDirectSequenceWildcard
		}
		if output != nil || g.Class() != FailureUnsupported || g.Code() != diagnosticCodegenUnsupported || g.Loc() != wildcard.Loc() ||
			g.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, wantGenerationCause) {
			t.Fatalf("generation: %s, output %q", g, output)
		}
	}
}

//nolint:gocognit // Keep alternate syntax exits and their diagnostic metadata together.
func TestEmptyNegativeQNameAlternateExits(t *testing.T) {
	cases := []struct {
		name, kind, attributes, marker, spec string
		policy                               LanguagePolicy
		class                                FailureClass
		code                                 string
		cause                                error
		related                              []string
	}{
		{"element-strict10", "any", ` notNamespace="##local" notQName=""`, `notNamespace="##local"`, "xsd11-structures#cSchemaDocument", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, errLanguagePolicyMismatch, nil},
		{"attribute-strict10", "anyAttribute", ` notNamespace="##local" notQName=""`, `notNamespace="##local"`, "xsd11-structures#cSchemaDocument", Strict10, FailureUnsupported, UnsupportedSchemaSyntaxCode, errLanguagePolicyMismatch, nil},
		{"element-empty-negative", "any", ` notNamespace="&#x9;" notQName=""`, `notNamespace="&#x9;"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, nil, nil},
		{"attribute-empty-negative", "anyAttribute", ` notNamespace="&#x9;" notQName=""`, `notNamespace="&#x9;"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, nil, nil},
		{"element-conflict", "any", ` notNamespace="##local" notQName="" namespace="##any"`, `namespace="##any"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, nil, nil},
		{"attribute-conflict", "anyAttribute", ` notNamespace="##local" notQName="" namespace="##any"`, `namespace="##any"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, nil, nil},
		{"element-occurrence", "any", ` notNamespace="##local" notQName="" minOccurs="2" maxOccurs="1"`, `<xs:any `, "xsd11-structures#coss-particle", Strict11, FailureInvalid, invalidSchemaCompositionCode, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
		{"element-process", "any", ` notNamespace="##local" notQName="" processContents="bad"`, `processContents="bad"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, nil, nil},
		{"attribute-process", "anyAttribute", ` notNamespace="##local" notQName="" processContents="bad"`, `processContents="bad"`, "", Strict11, FailureInvalid, invalidSchemaCompositionCode, nil, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := negativeAttributeSchema("sequence", test.attributes)
			if test.kind == "any" {
				root = directWildcardSchema("1.1", "sequence", false, test.attributes)
			}
			if test.policy == Strict10 {
				root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := wildcardParticleTestLoc(t, root, test.marker)
			var related []Loc
			for _, marker := range test.related {
				related = append(related, wildcardParticleTestLoc(t, root, marker))
			}
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc ||
				!reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != test.spec {
				t.Fatalf("diagnostic = %s, related %v, spec %q; want %s/%s at %s related %v spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), test.class, test.code, wantLoc, related, test.spec)
			}
			if test.cause != nil && !errors.Is(err, test.cause) || test.cause == nil && diagnostic.Unwrap() != nil {
				t.Fatalf("cause = %v, want %v", diagnostic.Unwrap(), test.cause)
			}
		})
	}
}

func TestEmptyNegativeQNameElementExtensionRemainsUnsupported(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		root := directWildcardSchema("1.1", model, true, ` notNamespace="##local" notQName=""`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
			diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName=""`) || len(diagnostic.Related()) != 0 ||
			diagnostic.SpecRef() != schemaAnyParticleSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAnyParticleUnsupported) {
			t.Fatalf("%s extension = %s", model, diagnostic)
		}
	}
}

func TestEmptyNegativeQNameAttributeRestrictionRemainsUnsupported(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		wildcard := `<xs:anyAttribute notNamespace="##local" notQName=""/>`
		owner := namedRestrictionOwner("t:Base", wildcard)
		if shape == "inline" {
			owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
		}
		base := `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute namespace="##any" processContents="strict"/></xs:complexType>`
		root := namedRestrictionSchema("1.1", "urn:t", owner+base)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantRelated := []Loc{anyAttributeTestLoc(root, `<xs:anyAttribute namespace="##any"`)}
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
			diagnostic.Loc() != anyAttributeTestLoc(root, `notQName=""`) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) ||
			diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
			t.Fatalf("%s restriction = %s, related %v", shape, diagnostic, diagnostic.Related())
		}
	}
}

func TestEmptyNegativeQNameElementRestrictionRemainsUnsupported(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		model := `<xs:sequence><xs:any notNamespace="##local" notQName=""/></xs:sequence>`
		owner := namedRestrictionOwner("t:Base", model)
		if shape == "inline" {
			owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + model + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
		}
		base := `<xs:complexType name="Base"><xs:sequence><xs:any namespace="##any"/></xs:sequence></xs:complexType>`
		root := namedRestrictionSchema("1.1", "urn:t", owner+base)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
			diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notNamespace="##local"`) ||
			len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" ||
			diagnostic.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s restriction = %s", shape, diagnostic)
		}
	}
}

func TestEmptyNegativeQNameMalformedXMLKeepsCause(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		attributes := ` notNamespace="##local" notQName="" notQName="&#x9;"`
		root := negativeAttributeSchema("sequence", attributes)
		if kind == "any" {
			root = directWildcardSchema("1.1", "sequence", false, attributes)
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidXMLSyntaxCode ||
			diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="&#x9;"`) ||
			len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "" || diagnostic.Unwrap() != nil {
			t.Fatalf("%s duplicate XML = %s, related %v, cause %v", kind, diagnostic, diagnostic.Related(), err)
		}
	}
}
