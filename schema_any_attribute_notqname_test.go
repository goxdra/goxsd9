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

func TestAnyAttributeQNameZeroZeroOwnerRetainsNames(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"sequence", "choice"} {
			root := attributeQNameSchema(model, ` notQName="t:kept t:kept"`, "")
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
			if !constraint.Present() || !reflect.DeepEqual(constraint.Tokens(), []string{"t:kept", "t:kept"}) ||
				!reflect.DeepEqual(constraint.Names(), []QName{mustTestQName(t, "urn:t", "kept")}) ||
				constraint.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) {
				t.Fatalf("0/0 owner lost QName facts: %#v", constraint)
			}
		}
	}
}

//nolint:gocognit // Check conflict precedence across supported policies, owners, and 0/0 omission.
func TestAnyAttributeQNameNamespaceConflictAndPolicy(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"sequence", "choice"} {
			for _, omitted := range []bool{false, true} {
				root := attributeQNameSchema(model, ` namespace="urn:p" notQName="t:z p:a"`, "")
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
	root := attributeQNameSchema("sequence", ` notQName="t:a"`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
		diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("Strict10 edition mismatch = %s, spec %q, cause %v", diagnostic, diagnostic.SpecRef(), err)
	}
}

//nolint:gocognit // Check both consumer diagnostics independently for each admitted model.
func TestAnyAttributeQNameConsumersRemainUnsupported(t *testing.T) {
	for _, model := range []string{"sequence", "choice"} {
		root := attributeQNameSchema(model, ` notQName="t:excluded"`, "")
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

func TestAnyAttributeQNameExcludedForms(t *testing.T) {
	for _, test := range []struct {
		name, model, attributes, marker string
		class                           FailureClass
		code, spec                      string
		cause                           error
	}{
		{"defined", "sequence", ` notQName="##defined"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"lax", "sequence", ` processContents="lax" notQName="t:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"skip", "choice", ` processContents="skip" notQName="t:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"other", "sequence", ` namespace="##other" notQName="p:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"negative", "choice", ` notNamespace="##local" notQName="t:item"`, `<xs:anyAttribute`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"sibling", "sequence", ` notQName="##definedSibling"`, `notQName=`, FailureInvalid, invalidSchemaCompositionCode, "", nil},
		{"malformed", "choice", ` notQName="bad:q:name"`, `notQName=`, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
		{"unbound", "sequence", ` notQName="missing:item"`, `notQName=`, FailureInvalid, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, nil},
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
			root := attributeQNameSchema(model, ` notQName="t:item"`, "")
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
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, marker) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("%s/%s diagnostic = %s", shape, model, diagnostic)
			}
		}
	}
}

func TestAnyAttributeQNameExcludedPlacementMatrix(t *testing.T) {
	forms := []struct{ name, attributes string }{
		{"defined", ` notQName="##defined"`},
		{"lax", ` processContents="lax" notQName="t:item"`},
		{"skip", ` processContents="skip" notQName="t:item"`},
		{"other", ` namespace="##other" notQName="p:item"`},
		{"negative", ` notNamespace="##local" notQName="t:item"`},
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
				{"unbound", "missing:item", invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef},
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
		root := namedRestrictionSchema("1.1", "urn:n", `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute notQName="t:excluded"/></xs:complexType>`+
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

func TestAnyAttributeQNameDerivedRestrictionValidatesBaseFirst(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			wildcard := `<xs:anyAttribute namespace="##any" processContents="strict" notQName="t:excluded"/>`
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

func TestAnyAttributeQNameDerivedRestrictionComparisonUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		derivedWildcard := `<xs:anyAttribute namespace="##targetNamespace" processContents="strict" notQName="t:excluded"/>`
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

func TestAnyAttributeQNameExtensionDoesNotDropBaseExclusions(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := namedRestrictionSchema("1.1", "urn:n", `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute notQName="t:excluded"/></xs:complexType>`+
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
