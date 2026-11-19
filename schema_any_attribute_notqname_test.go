package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func attributeQNameSchema(model, attributes, children string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" xmlns:p="urn:p" xmlns="urn:t" targetNamespace="urn:t"><xs:complexType name="Record"><xs:` + model + `>` + children + `</xs:` + model + `><xs:anyAttribute` + attributes + `/></xs:complexType></xs:schema>`
}

func attributeQNamePlacementSchema(shape, model, wildcard string) string {
	body := `<xs:element name="root"><xs:complexType><xs:` + model + `></xs:` + model + `>` + wildcard + `</xs:complexType></xs:element>`
	if shape == "attributeGroupRef" {
		body = `<xs:attributeGroup name="G">` + wildcard + `</xs:attributeGroup><xs:complexType name="Record"><xs:` + model + `></xs:` + model + `><xs:attributeGroup ref="t:G"/></xs:complexType>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" xmlns:p="urn:p" targetNamespace="urn:t">` + body + `</xs:schema>`
}

func attributeQNameFact(t *testing.T, schema Schema) AnyAttribute {
	t.Helper()
	components := schema.Components()
	var definition ComplexTypeDefinition
	var ok bool
	for _, component := range components {
		if component.Name() == mustTestQName(t, "urn:t", "Record") {
			definition, ok = component.ComplexType()
			break
		}
	}
	if !ok {
		t.Fatal("component is not a complex type")
	}
	wildcard, ok := definition.AnyAttribute()
	if !ok {
		t.Fatal("complex type has no attribute wildcard")
	}
	return wildcard
}

//nolint:gocognit // Exercise policy, model, and namespace combinations through the public query.
func TestAnyAttributeQNameDirectFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"sequence", "choice"} {
			for _, namespace := range []string{"", ` namespace="##targetNamespace urn:p"`} {
				t.Run(string(policy)+"/"+model+"/"+namespace, func(t *testing.T) {
					for _, lexical := range []string{"", ` notQName=" p:z  local p:a local "`} {
						root := attributeQNameSchema(model, namespace+lexical, `<xs:element name="item" type="xs:string"/>`)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err != nil {
							t.Fatalf("parse direct attribute wildcard: %v", err)
						}
						wildcard := attributeQNameFact(t, schema)
						if wildcard.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute`) || wildcard.ProcessContents() != "strict" || !wildcard.ProcessContentsLoc().IsZero() {
							t.Fatalf("wildcard source/mode = %s/%q/%s", wildcard.Loc(), wildcard.ProcessContents(), wildcard.ProcessContentsLoc())
						}
						if lexical == "" {
							if wildcard.QNameConstraint().Present() || len(wildcard.QNameConstraint().Names()) != 0 {
								t.Fatal("absent notQName has exclusions")
							}
							continue
						}
						constraint := wildcard.QNameConstraint()
						wantNames := []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z"), mustTestQName(t, "urn:t", "local")}
						wantTokens := []string{"p:z", "local", "p:a", "local"}
						if !constraint.Present() || constraint.LexicalForm() != "p:z local p:a local" || constraint.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
							!reflect.DeepEqual(constraint.Tokens(), wantTokens) || !reflect.DeepEqual(constraint.Names(), wantNames) {
							t.Fatalf("QName facts = %#v, names %v", constraint, constraint.Names())
						}
						assertWildcardQNameGraphDefaultBinding(t, constraint.NamespaceBindings(), "urn:t", 0, 0)
						if namespace == "" && (!wildcard.NamespaceLoc().IsZero() || wildcard.NamespaceConstraint().Variety() != WildcardNamespaceConstraintAny) {
							t.Fatalf("absent namespace facts = %#v", wildcard.NamespaceConstraint())
						}
						if namespace != "" && (wildcard.NamespaceLoc() != wildcardParticleTestLoc(t, root, `namespace=`) || !reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), []string{"urn:p", "urn:t"})) {
							t.Fatalf("positive namespace facts = %#v", wildcard.NamespaceConstraint())
						}
						tokens, names, bindings := constraint.Tokens(), constraint.Names(), constraint.NamespaceBindings()
						tokens[0], names[0] = "changed", QName{}
						bindings[0].Namespace = "changed"
						again := wildcard.QNameConstraint()
						if !reflect.DeepEqual(again.Tokens(), wantTokens) || !reflect.DeepEqual(again.Names(), wantNames) || again.NamespaceBindings()[0].Namespace == "changed" {
							t.Fatal("public QName facts leaked mutable storage")
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // Exercise the symbolic token and explicit names across direct owners and namespace forms.
func TestAnyAttributeDefinedDirectFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"sequence", "choice"} {
			for _, form := range []struct {
				name, attributes, lexical string
				tokens                    []string
				names                     []QName
			}{
				{"omitted-keyword", ` notQName=" ##defined  ##defined "`, "##defined ##defined", []string{"##defined", "##defined"}, nil},
				{"empty-keyword", ` namespace="" notQName="##defined"`, "##defined", []string{"##defined"}, nil},
				{"positive-keyword", ` namespace="urn:p" notQName="##defined"`, "##defined", []string{"##defined"}, nil},
				{"omitted-mixed", ` notQName=" p:z ##defined t:a ##defined p:z "`, "p:z ##defined t:a ##defined p:z", []string{"p:z", "##defined", "t:a", "##defined", "p:z"}, []QName{mustTestQName(t, "urn:p", "z"), mustTestQName(t, "urn:t", "a")}},
				{"positive-mixed", ` namespace="##targetNamespace urn:p" notQName=" ##defined t:a p:z t:a "`, "##defined t:a p:z t:a", []string{"##defined", "t:a", "p:z", "t:a"}, []QName{mustTestQName(t, "urn:p", "z"), mustTestQName(t, "urn:t", "a")}},
			} {
				t.Run(string(policy)+"/"+model+"/"+form.name, func(t *testing.T) {
					root := attributeQNameSchema(model, form.attributes, `<xs:element name="item" type="xs:string"/>`)
					root = strings.Replace(root, `<xs:complexType name="Record">`, `<xs:attribute name="z" type="xs:token"/><xs:element name="a" type="xs:string"/><xs:complexType name="Record">`, 1)
					root = strings.Replace(root, `</xs:schema>`, `<xs:attribute name="a" type="xs:token"/></xs:schema>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("parse defined attribute wildcard: %v", err)
					}
					wildcard := attributeQNameFact(t, schema)
					for _, component := range schema.Components() {
						if component.Name() != mustTestQName(t, "urn:t", "Record") {
							continue
						}
						definition, ok := component.ComplexType()
						if !ok {
							t.Fatal("Record is not complex type")
						}
						particle := definition.Particle()
						if particle == nil || particle.Loc() != wildcardParticleTestLoc(t, root, `<xs:`+model+`>`) || particle.Occurrences().String() != "1/1" {
							t.Fatalf("direct owner particle = %#v", particle)
						}
						var members []Particle
						switch typed := particle.(type) {
						case SequenceParticle:
							members = typed.Particles()
						case ChoiceParticle:
							members = typed.Alternatives()
						default:
							t.Fatalf("unexpected owner particle %T", particle)
						}
						if len(members) != 1 || members[0].Occurrences().String() != "1/1" || members[0].Loc() != wildcardParticleTestLoc(t, root, `<xs:element name="item"`) {
							t.Fatalf("direct owner members = %#v", members)
						}
						break
					}
					constraint := wildcard.QNameConstraint()
					if !constraint.Present() || constraint.LexicalForm() != form.lexical || constraint.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
						!reflect.DeepEqual(constraint.Tokens(), form.tokens) || !reflect.DeepEqual(constraint.Names(), form.names) || wildcard.ProcessContents() != "strict" {
						t.Fatalf("defined QName facts = %#v, names %v", constraint, constraint.Names())
					}
					assertWildcardQNameGraphDefaultBinding(t, constraint.NamespaceBindings(), "urn:t", 0, 0)
					if form.name == "empty-keyword" && (wildcard.NamespaceConstraint().Variety() != WildcardNamespaceConstraintEnumeration || len(wildcard.NamespaceConstraint().Namespaces()) != 0 || wildcard.NamespaceLoc() != wildcardParticleTestLoc(t, root, `namespace=`)) {
						t.Fatalf("empty namespace facts = %#v", wildcard.NamespaceConstraint())
					}
					if form.name == "positive-mixed" && !reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), []string{"urn:p", "urn:t"}) {
						t.Fatalf("positive namespace facts = %#v", wildcard.NamespaceConstraint())
					}
					tokens, names, bindings := constraint.Tokens(), constraint.Names(), constraint.NamespaceBindings()
					tokens[0] = "changed"
					if len(names) != 0 {
						names[0] = QName{}
					}
					bindings[0].Namespace = "changed"
					if again := wildcard.QNameConstraint(); !reflect.DeepEqual(again.Tokens(), form.tokens) || !reflect.DeepEqual(again.Names(), form.names) || again.NamespaceBindings()[0].Namespace == "changed" {
						t.Fatal("symbolic QName facts leaked mutable storage")
					}
				})
			}
		}
	}
}

func TestAnyAttributeQNameEmptyLexicalAndOmittedOwner(t *testing.T) {
	for _, model := range []string{"sequence", "choice"} {
		root := attributeQNameSchema(model, ` notQName="  "`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("empty qnameList: %v", err)
		}
		constraint := attributeQNameFact(t, schema).QNameConstraint()
		if !constraint.Present() || constraint.LexicalForm() != "" || len(constraint.Tokens()) != 0 || len(constraint.Names()) != 0 {
			t.Fatalf("empty qnameList facts = %#v", constraint)
		}
		root = strings.Replace(root, `<xs:`+model+`>`, `<xs:`+model+` minOccurs="0" maxOccurs="0">`, 1)
		schema, err = discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("0/0 owner: %v", err)
		}
		definition, ok := schema.Components()[0].ComplexType()
		if !ok || definition.Particle() != nil {
			t.Fatalf("0/0 owner particle = %T, complex type %t", definition.Particle(), ok)
		}
		if !attributeQNameFact(t, schema).QNameConstraint().Present() {
			t.Fatal("0/0 owner discarded attribute wildcard")
		}
	}
}

//nolint:gocognit // Cover both policies, direct owners, and symbolic or explicit exclusions after omission.
func TestAnyAttributeQNameZeroZeroOwnerRetainsNames(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"sequence", "choice"} {
			for _, lexical := range []string{"t:kept t:kept", "##defined t:kept ##defined t:kept"} {
				root := attributeQNameSchema(model, ` notQName="`+lexical+`"`, "")
				root = strings.Replace(root, `<xs:`+model+`>`, `<xs:`+model+` minOccurs="0" maxOccurs="0">`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("0/0 owner %s/%s: %v", policy, model, err)
				}
				definition, ok := schema.Components()[0].ComplexType()
				if !ok || definition.Particle() != nil {
					t.Fatalf("0/0 owner particle = %T, complex type %t", definition.Particle(), ok)
				}
				constraint := attributeQNameFact(t, schema).QNameConstraint()
				if !constraint.Present() || !reflect.DeepEqual(constraint.Tokens(), strings.Fields(lexical)) ||
					!reflect.DeepEqual(constraint.Names(), []QName{mustTestQName(t, "urn:t", "kept")}) ||
					constraint.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) {
					t.Fatalf("0/0 owner lost QName facts: %#v", constraint)
				}
			}
		}
	}
}

//nolint:gocognit // Check conflict precedence across supported policies, owners, and 0/0 omission.
func TestAnyAttributeQNameNamespaceConflictAndPolicy(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"sequence", "choice"} {
			for _, omitted := range []bool{false, true} {
				for _, lexical := range []string{"t:z p:a", "##defined t:z p:a"} {
					root := attributeQNameSchema(model, ` namespace="urn:p" notQName="`+lexical+`"`, "")
					if omitted {
						root = strings.Replace(root, `<xs:`+model+`>`, `<xs:`+model+` minOccurs="0" maxOccurs="0">`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
						diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
						!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `namespace=`)}) ||
						diagnostic.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef ||
						!strings.Contains(diagnostic.Message(), "{urn:t}z") || !errors.Is(err, errSchemaWildcardQNameNamespace) {
						t.Fatalf("namespace conflict %s/%s omitted=%t: %s, related %v", policy, model, omitted, diagnostic, diagnostic.Related())
					}
				}
			}
		}
	}
	for _, lexical := range []string{"t:a", "##defined", "##defined t:a"} {
		root := attributeQNameSchema("sequence", ` notQName="`+lexical+`"`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
			diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
			t.Fatalf("Strict10 edition mismatch = %s, spec %q, cause %v", diagnostic, diagnostic.SpecRef(), err)
		}
	}
}

//nolint:gocognit // Check both consumer diagnostics independently for each admitted model.
func TestAnyAttributeQNameConsumersRemainUnsupported(t *testing.T) {
	for _, model := range []string{"sequence", "choice"} {
		for _, lexical := range []string{"t:excluded", "##defined", "##defined t:excluded"} {
			root := attributeQNameSchema(model, ` notQName="`+lexical+`"`, "")
			root = strings.Replace(root, `</xs:schema>`, `<xs:element name="root" type="t:Record"/></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("parse consumer fixture: %v", err)
			}
			wildcard := attributeQNameFact(t, schema)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:t"/>`)))
			if validationErr == nil {
				t.Fatal("validation accepted attribute wildcard")
			}
			validation := requireDiagnostic(t, validationErr)
			wantValidationLoc := mustTestLoc(t, "instance.xml", 1, 1)
			if model == "choice" {
				wantValidationLoc = wildcard.Loc()
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantValidationLoc ||
				validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) ||
				!negativeWildcardRelatedHas(validation.Related(), wildcard.Loc()) || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %s, related %v", validation, validation.Related())
			}
			generated, generationErr := GenerateGo(schema, "generated")
			if generationErr == nil || generated != nil {
				t.Fatalf("generation = (%q, %v), want nil unsupported", generated, generationErr)
			}
			generation := requireDiagnostic(t, generationErr)
			wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
			if model == "sequence" {
				wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
			}
			if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wildcard.Loc() ||
				generation.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("generation diagnostic = %s", generation)
			}
		}
	}
}

//nolint:gocognit // Graph order, identities, and chameleon QName facts share one fixture.
func TestAnyAttributeQNameGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:t"><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:p"><xs:complexType name="Included"><xs:sequence/><xs:anyAttribute namespace="##targetNamespace urn:p" notQName="local p:a local"/></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns="urn:other" targetNamespace="urn:other"><xs:import namespace="urn:t" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:choice/><xs:anyAttribute notQName="item"/></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":     {id: "root.xsd", contents: root},
		"included.xsd": {id: "included.xsd", contents: included},
		"other.xsd":    {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("graph parse: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" {
				t.Fatalf("graph document order = %v", documents)
			}
			components := schema.Components()
			if len(components) != 2 || components[0].Name() != mustTestQName(t, "urn:t", "Included") || components[1].Name() != mustTestQName(t, "urn:other", "Imported") {
				t.Fatalf("graph component order = %v", components)
			}
			walked := make([]ComponentID, 0, len(components))
			if err := schema.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("walk graph: %v", err)
			}
			if len(walked) != len(components) || components[0].ID() != walked[0] || components[1].ID() != walked[1] {
				t.Fatalf("walk IDs %v differ from component order", walked)
			}
			for index, want := range []struct {
				source, document, lexical string
				names                     []QName
			}{
				{"included.xsd", included, "local p:a local", []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:t", "local")}},
				{"other.xsd", other, "item", []QName{mustTestQName(t, "urn:other", "item")}},
			} {
				definition, ok := components[index].ComplexType()
				if !ok {
					t.Fatal("graph component is not complex type")
				}
				wildcard, ok := definition.AnyAttribute()
				if !ok {
					t.Fatal("graph component lacks wildcard")
				}
				constraint := wildcard.QNameConstraint()
				if wildcard.Loc().Source() != SourceID(want.source) || constraint.Loc() != wildcardQNameDocumentLoc(t, SourceID(want.source), want.document, `notQName=`) ||
					constraint.LexicalForm() != want.lexical || !reflect.DeepEqual(constraint.Names(), want.names) {
					t.Fatalf("graph fact %d = %#v", index, constraint)
				}
			}
		}
	}
}

//nolint:gocognit // The graph fixture checks discovery, chameleon mapping, cycles, and declaration-kind separation together.
func TestAnyAttributeDefinedGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:o" schemaLocation="other.xsd"/><xs:attribute name="before" type="xs:token"/><xs:element name="decoy" type="xs:string"/><xs:complexType name="Record"><xs:sequence/><xs:anyAttribute notQName="##defined"/></xs:complexType><xs:attribute name="after" type="xs:token"/></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:p"><xs:complexType name="Included"><xs:choice/><xs:anyAttribute namespace="##targetNamespace urn:p" notQName="##defined local p:out"/></xs:complexType><xs:attribute name="local" type="xs:token"/></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:o" targetNamespace="urn:o"><xs:import namespace="urn:t" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:sequence/><xs:anyAttribute namespace="" notQName="##defined"/></xs:complexType><xs:attribute name="remote" type="xs:token"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":     {id: "root.xsd", contents: root},
		"included.xsd": {id: "included.xsd", contents: included},
		"other.xsd":    {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var firstNames []QName
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("defined graph parse: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" {
				t.Fatalf("defined graph documents = %v", documents)
			}
			components := schema.Components()
			names := make([]QName, 0, len(components))
			for _, component := range components {
				names = append(names, component.Name())
			}
			if run == 0 {
				firstNames = names
			}
			if run != 0 && !reflect.DeepEqual(names, firstNames) {
				t.Fatalf("graph component order changed: %v / %v", firstNames, names)
			}
			for _, want := range []struct {
				name, namespace, source, document, lexical string
				names                                      []QName
			}{
				{"Record", "urn:t", "root.xsd", root, "##defined", nil},
				{"Included", "urn:t", "included.xsd", included, "##defined local p:out", []QName{mustTestQName(t, "urn:p", "out"), mustTestQName(t, "urn:t", "local")}},
				{"Imported", "urn:o", "other.xsd", other, "##defined", nil},
			} {
				var found bool
				for _, component := range components {
					if component.Name() != mustTestQName(t, want.namespace, want.name) {
						continue
					}
					definition, ok := component.ComplexType()
					if !ok {
						t.Fatalf("%s is not complex type", want.name)
					}
					wildcard, ok := definition.AnyAttribute()
					if !ok {
						t.Fatalf("%s lacks attribute wildcard", want.name)
					}
					constraint := wildcard.QNameConstraint()
					if constraint.Loc() != wildcardQNameDocumentLoc(t, SourceID(want.source), want.document, `notQName=`) ||
						constraint.LexicalForm() != want.lexical || !reflect.DeepEqual(constraint.Tokens(), strings.Fields(want.lexical)) ||
						!reflect.DeepEqual(constraint.Names(), want.names) {
						t.Fatalf("%s QName facts = %#v, names %v", want.name, constraint, constraint.Names())
					}
					found = true
					break
				}
				if !found {
					t.Fatalf("missing graph type %s", want.name)
				}
			}
		}
	}
}

func TestAnyAttributeQNameExcludedForms(t *testing.T) {
	for _, test := range []struct {
		name, model, attributes, marker string
		class                           FailureClass
		code, spec                      string
		cause                           error
	}{
		{"lax", "sequence", ` processContents="lax" notQName="t:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"lax-defined", "sequence", ` processContents="lax" notQName="##defined"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"skip", "choice", ` processContents="skip" notQName="t:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"skip-defined", "choice", ` processContents="skip" notQName="##defined"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"other", "sequence", ` namespace="##other" notQName="p:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"other-defined", "sequence", ` namespace="##other" notQName="##defined"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"negative", "choice", ` notNamespace="##local" notQName="t:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"negative-defined", "choice", ` notNamespace="##local" notQName="##defined"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"sibling", "sequence", ` notQName="##definedSibling"`, `notQName=`, FailureInvalid, invalidSchemaCompositionCode, "", nil},
		{"malformed", "choice", ` notQName="bad:q:name"`, `notQName=`, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"malformed-after-defined", "choice", ` processContents="lax" notQName="##defined bad:q:name"`, `notQName=`, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound", "sequence", ` notQName="missing:item"`, `notQName=`, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound-after-defined", "sequence", ` namespace="##other" notQName="##defined missing:item"`, `notQName=`, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := attributeQNameSchema(test.model, test.attributes, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.marker) ||
				diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("%s diagnostic = %s, related %v, spec %q", test.name, diagnostic, diagnostic.Related(), diagnostic.SpecRef())
			}
		})
	}
}

func TestAnyAttributeQNameExcludedOwners(t *testing.T) {
	for _, shape := range []string{"empty", "inline", "attributeGroup"} {
		for _, model := range []string{"sequence", "choice"} {
			for _, lexical := range []string{"t:item", "##defined"} {
				root := attributeQNameSchema(model, ` notQName="`+lexical+`"`, "")
				marker := `<xs:anyAttribute`
				switch shape {
				case "empty":
					root = strings.Replace(root, `<xs:`+model+`></xs:`+model+`>`, "", 1)
				case "inline":
					root = strings.Replace(root, `<xs:complexType name="Record">`, `<xs:element name="root"><xs:complexType>`, 1)
					root = strings.Replace(root, `</xs:complexType>`, `</xs:complexType></xs:element>`, 1)
				case "attributeGroup":
					root = strings.Replace(root, `<xs:complexType name="Record"><xs:`+model+`></xs:`+model+`>`, `<xs:attributeGroup name="Record">`, 1)
					root = strings.Replace(root, `</xs:complexType>`, `</xs:attributeGroup>`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, marker) || len(diagnostic.Related()) != 0 ||
					diagnostic.SpecRef() != schemaAnyAttributeXSD11SpecRef || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("%s/%s/%s diagnostic = %s", shape, model, lexical, diagnostic)
				}
			}
		}
	}
}

func TestAnyAttributeQNameExcludedPlacementMatrix(t *testing.T) {
	forms := []struct{ name, attributes string }{
		{"defined", ` notQName="##defined"`},
		{"defined-mixed", ` notQName="##defined t:item"`},
		{"lax", ` processContents="lax" notQName="t:item"`},
		{"lax-defined", ` processContents="lax" notQName="##defined"`},
		{"skip", ` processContents="skip" notQName="t:item"`},
		{"skip-defined", ` processContents="skip" notQName="##defined"`},
		{"other", ` namespace="##other" notQName="p:item"`},
		{"other-defined", ` namespace="##other" notQName="##defined"`},
		{"negative", ` notNamespace="##local" notQName="t:item"`},
		{"negative-defined", ` notNamespace="##local" notQName="##defined"`},
	}
	for _, shape := range []string{"inline", "attributeGroupRef"} {
		for _, model := range []string{"sequence", "choice"} {
			for _, form := range forms {
				wildcard := `<xs:anyAttribute` + form.attributes + `/>`
				root := attributeQNamePlacementSchema(shape, model, wildcard)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute`) || len(diagnostic.Related()) != 0 ||
					diagnostic.SpecRef() != schemaAnyAttributeXSD11SpecRef || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("%s/%s/%s diagnostic = %s, related %v, spec %q", shape, model, form.name, diagnostic, diagnostic.Related(), diagnostic.SpecRef())
				}
			}
		}
	}
}

func TestAnyAttributeQNameInvalidPlacementMatrix(t *testing.T) {
	for _, shape := range []string{"inline", "attributeGroupRef"} {
		for _, model := range []string{"sequence", "choice"} {
			for _, form := range []struct{ name, lexical, code, spec string }{
				{"sibling", "##definedSibling", invalidSchemaCompositionCode, ""},
				{"malformed", "bad:q:name", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef},
				{"malformed-after-defined", "##defined bad:q:name", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef},
				{"unbound", "missing:item", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef},
				{"unbound-after-defined", "##defined missing:item", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef},
			} {
				wildcard := `<xs:anyAttribute notQName="` + form.lexical + `"/>`
				root := attributeQNamePlacementSchema(shape, model, wildcard)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != form.code ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != form.spec {
					t.Fatalf("%s/%s/%s diagnostic = %s, spec %q", shape, model, form.name, diagnostic, diagnostic.SpecRef())
				}
			}
		}
	}
}

func TestAnyAttributeQNameRestrictionDoesNotDropBaseExclusions(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, lexical := range []string{"t:excluded", "##defined", "##defined t:excluded"} {
			root := namedRestrictionSchema("1.1", "urn:n", `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute notQName="`+lexical+`"/></xs:complexType>`+
				namedRestrictionOwner("t:Base", namedRestrictionWildcard("##any", "strict")))
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute namespace="##any"`) ||
				diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) ||
				!errors.Is(err, errSchemaComplexTypeBaseUnsupported) ||
				!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `notQName=`), wildcardParticleTestLoc(t, root, `<xs:anyAttribute notQName=`)}) {
				t.Fatalf("restriction lost exclusions: %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
			}
		}
	}
}

func TestAnyAttributeQNameDerivedRestrictionValidatesBaseFirst(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			for _, lexical := range []string{"t:excluded", "##defined", "##defined t:excluded"} {
				wildcard := `<xs:anyAttribute namespace="##any" processContents="strict" notQName="` + lexical + `"/>`
				owner := namedRestrictionOwner("t:Missing", wildcard)
				if shape == "inline" {
					owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Missing">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
				}
				root := namedRestrictionSchema("1.1", "urn:n", owner)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, `base="t:Missing"`) || len(diagnostic.Related()) != 0 ||
					diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
					t.Fatalf("%s/%s base diagnostic = %s, related %v", shape, policy, diagnostic, diagnostic.Related())
				}
			}
		}
	}
}

func TestAnyAttributeQNameDerivedRestrictionComparisonUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, lexical := range []string{"t:excluded", "##defined", "##defined t:excluded"} {
			derivedWildcard := `<xs:anyAttribute namespace="##targetNamespace" processContents="strict" notQName="` + lexical + `"/>`
			root := namedRestrictionSchema("1.1", "urn:n", namedRestrictionOwner("t:Base", derivedWildcard)+namedRestrictionPositiveSetBase())
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
				!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `<xs:anyAttribute namespace="##targetNamespace urn:outside"`)}) ||
				diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) ||
				!errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
				t.Fatalf("derived QName comparison = %s, related %v", diagnostic, diagnostic.Related())
			}
		}
	}
}

func TestAnyAttributeQNameExtensionDoesNotDropBaseExclusions(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, lexical := range []string{"t:excluded", "##defined", "##defined t:excluded"} {
			root := namedRestrictionSchema("1.1", "urn:n", `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute notQName="`+lexical+`"/></xs:complexType>`+
				`<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, `base="t:Base"`) ||
				diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) ||
				!errors.Is(err, errSchemaComplexTypeBaseUnsupported) ||
				!negativeWildcardRelatedHas(diagnostic.Related(), wildcardParticleTestLoc(t, root, `<xs:anyAttribute`)) {
				t.Fatalf("extension lost exclusions: %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
			}
		}
	}
}

func TestAnyAttributeDefinedGroupedExtensionDoesNotCompose(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:element name="entry" type="xs:string"/><xs:group name="G"><xs:choice/></xs:group><xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:G"/></xs:extension></xs:complexContent></xs:complexType><xs:complexType name="Base"><xs:sequence><xs:element ref="t:entry" minOccurs="0"/></xs:sequence><xs:anyAttribute notQName="##defined"/></xs:complexType></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
			diagnostic.Loc() != wildcardParticleTestLoc(t, root, `base="t:Base"`) ||
			!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `<xs:complexType name="Base"`), wildcardParticleTestLoc(t, root, `<xs:anyAttribute`)}) ||
			diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
			t.Fatalf("grouped extension lost keyword: %s, related %v", diagnostic, diagnostic.Related())
		}
	}
}

func FuzzSchemaAnyAttributeNotQName(f *testing.F) {
	for _, seed := range []string{"t:local", "p:z t:local p:z", "", "##defined", "##definedSibling", "bad:q:name", "missing:item"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, lexical string) {
		escaped := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;").Replace(lexical)
		root := attributeQNameSchema("sequence", ` notQName="`+escaped+`"`, "")
		first, firstErr := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		second, secondErr := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("parse success changed for %q", lexical)
		}
		if firstErr != nil {
			assertZeroSchema(t, first)
			assertZeroSchema(t, second)
			left, right := requireDiagnostic(t, firstErr), requireDiagnostic(t, secondErr)
			if left.Class() != right.Class() || left.Code() != right.Code() || left.Loc() != right.Loc() || left.SpecRef() != right.SpecRef() ||
				!reflect.DeepEqual(left.Related(), right.Related()) {
				t.Fatalf("diagnostic changed for %q: %s / %s", lexical, left, right)
			}
			return
		}
		left := attributeQNameFact(t, first).QNameConstraint()
		right := attributeQNameFact(t, second).QNameConstraint()
		if !reflect.DeepEqual(left.Tokens(), right.Tokens()) || !reflect.DeepEqual(left.Names(), right.Names()) || left.Loc() != right.Loc() ||
			!reflect.DeepEqual(left.NamespaceBindings(), right.NamespaceBindings()) {
			t.Fatalf("QName facts changed for %q: %#v / %#v", lexical, left, right)
		}
	})
}
