package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestWildcardQNameNamespaceDiagnosticUsesLexicalOrder(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:p" targetNamespace="urn:root"><xs:complexType name="Record"><xs:choice><xs:any namespace="urn:allowed" notQName="p:z p:a"/></xs:choice></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("namespace-inconsistent exclusions unexpectedly parsed")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
		diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="p:z p:a"`) ||
		!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `namespace="urn:allowed"`)}) ||
		diagnostic.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef ||
		!strings.Contains(diagnostic.Message(), "{urn:p}z") || strings.Contains(diagnostic.Message(), "{urn:p}a") ||
		!errors.Is(err, errSchemaWildcardQNameNamespace) {
		t.Fatalf("lexical-first namespace diagnostic = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
	}
}

func TestWildcardQNameConsistencyKeepsEarlierReferenceFailure(t *testing.T) {
	root := directWildcardSchemaWithTerms("1.1", "choice", false,
		` namespace="urn:allowed" notQName="t:excluded"`, `<xs:element ref="t:missing"/>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("unresolved reference unexpectedly parsed")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceUnresolvedCode ||
		diagnostic.Loc() != wildcardParticleTestLoc(t, root, `ref="t:missing"`) ||
		len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaElementReferenceSpecRef(XSDVersion11) ||
		!errors.Is(err, errSchemaElementReferenceUnresolved) || errors.Is(err, errSchemaWildcardQNameNamespace) {
		t.Fatalf("reference-before-wildcard diagnostic = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
	}
}

//nolint:gocognit // Assert each public consumer's exact wildcard boundary for both direct models.
func TestWildcardQNameExplicitExclusionsRemainConsumerUnsupported(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := directWildcardSchemaWithTerms("1.1", model, false,
				` notQName="t:root" minOccurs="0" maxOccurs="2"`,
				`<xs:element name="first" type="xs:integer"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("parse explicit exclusion: %v", err)
			}
			definition := directWildcardDefinition(t, schema)
			wildcard := directWildcardFromParticle(t, definition.Particle())
			wildcardLoc := wildcardParticleTestLoc(t, root, `<xs:any`)
			modelLoc := wildcardParticleTestLoc(t, root, `<xs:`+model+`>`)
			if wildcard.Loc() != wildcardLoc || wildcard.Occurrences().String() != "0/2" ||
				wildcard.NamespaceConstraint().Variety() != WildcardNamespaceConstraintAny ||
				!reflect.DeepEqual(wildcard.QNameConstraint().Names(), []QName{mustTestQName(t, "urn:root", "root")}) {
				t.Fatalf("explicit exclusion wildcard facts = %#v", wildcard)
			}
			instance := `<root xmlns="urn:root"/>`
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
			if validationErr == nil {
				t.Fatal("validation accepted nonzero wildcard")
			}
			validation := requireDiagnostic(t, validationErr)
			wantValidationLoc := wildcardLoc
			wantValidationCause := errInstanceChoiceWildcard
			if model == "sequence" {
				wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
				wantValidationCause = errInstanceSequenceWildcard
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode ||
				validation.Loc() != wantValidationLoc || validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) ||
				!negativeWildcardRelatedHas(validation.Related(), wildcardLoc) ||
				!negativeWildcardRelatedHas(validation.Related(), modelLoc) ||
				!errors.Is(validationErr, wantValidationCause) {
				t.Fatalf("validation diagnostic = %s, related %v", validation, validation.Related())
			}
			generated, generationErr := GenerateGo(schema, "generated")
			if generationErr == nil || generated != nil {
				t.Fatalf("generation = (%q, %v), want nil output and unsupported", generated, generationErr)
			}
			generation := requireDiagnostic(t, generationErr)
			wantGenerationCause := errCodegenDirectChoiceWildcard
			wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
			if model == "sequence" {
				wantGenerationCause = errCodegenDirectSequenceWildcard
				wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
			}
			if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
				generation.Loc() != wildcardLoc || generation.SpecRef() != wantGenerationSpec ||
				!negativeWildcardRelatedHas(generation.Related(), wildcardLoc) ||
				!negativeWildcardRelatedHas(generation.Related(), modelLoc) ||
				!errors.Is(generationErr, wantGenerationCause) {
				t.Fatalf("generation diagnostic = %s, related %v", generation, generation.Related())
			}
		})
	}
}

func TestWildcardQNameValidatedZeroZeroOmitsBeforeConsumers(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := directWildcardSchemaWithTerms("1.1", model, false,
				` notQName="t:root" minOccurs="0" maxOccurs="0"`,
				`<xs:element name="first" type="xs:integer"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("parse validated 0/0: %v", err)
			}
			assertWildcardQNameOmittedTerm(t, directWildcardDefinition(t, schema).Particle())
			assertWildcardQNameOmittedConsumers(t, schema, model)
		})
	}
}

func assertWildcardQNameOmittedTerm(t *testing.T, particle Particle) {
	t.Helper()
	var terms []Particle
	switch typed := particle.(type) {
	case ChoiceParticle:
		terms = typed.Alternatives()
	case SequenceParticle:
		terms = typed.Particles()
	default:
		t.Fatalf("particle = %T, want direct model", particle)
	}
	for _, term := range terms {
		if _, ok := wildcardParticleValue(term); ok {
			t.Fatal("0/0 wildcard remained in public particle")
		}
	}
}

func assertWildcardQNameOmittedConsumers(t *testing.T, schema Schema, model string) {
	t.Helper()
	instance := `<root xmlns="urn:root"><first xmlns="">1</first></root>`
	if model == "sequence" {
		instance = `<root xmlns="urn:root"><first xmlns="">1</first><last xmlns="">2</last></root>`
	}
	if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance))); err != nil {
		t.Fatalf("validate after 0/0 omission: %v", err)
	}
	if generated, err := GenerateGo(schema, "generated"); err != nil || len(generated) == 0 {
		t.Fatalf("generation after 0/0 omission = (%d bytes, %v)", len(generated), err)
	}
}

type wildcardQNameGraphWant struct {
	name       QName
	source     SourceID
	document   string
	lexical    string
	names      []QName
	defaultURI string
}

func TestWildcardQNameComposedGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:p"><xs:complexType name="Included"><xs:choice><xs:any namespace="##targetNamespace urn:p" notQName="local p:item local"/></xs:choice></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns="urn:other" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:sequence><xs:any namespace="##targetNamespace" notQName="local"/></xs:sequence></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":  {id: "root.xsd", contents: root},
		"child.xsd": {id: "child.xsd", contents: child},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	for run := 0; run < 2; run++ {
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err != nil {
			t.Fatalf("run %d: parse composed graph: %v", run, err)
		}
		assertWildcardQNameGraphDocuments(t, schema.Documents(), run)
		assertWildcardQNameGraphComponents(t, schema, child, other, run)
	}
}

func assertWildcardQNameGraphDocuments(t *testing.T, documents []SchemaDocument, run int) {
	t.Helper()
	if len(documents) != 3 {
		t.Fatalf("run %d: documents = %d, want 3 unique identities", run, len(documents))
	}
	for index, want := range []SourceID{"root.xsd", "child.xsd", "other.xsd"} {
		if documents[index].Source() != want {
			t.Fatalf("run %d: document %d source = %q, want %q", run, index, documents[index].Source(), want)
		}
	}
}

func assertWildcardQNameGraphComponents(t *testing.T, schema Schema, child, other string, run int) {
	t.Helper()
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("run %d: components = %d, want 2", run, len(components))
	}
	wantComponents := []wildcardQNameGraphWant{
		{mustTestQName(t, "urn:root", "Included"), "child.xsd", child, "local p:item local", []QName{mustTestQName(t, "urn:p", "item"), mustTestQName(t, "urn:root", "local")}, ""},
		{mustTestQName(t, "urn:other", "Imported"), "other.xsd", other, "local", []QName{mustTestQName(t, "urn:other", "local")}, "urn:other"},
	}
	walked := make([]ComponentID, 0, len(components))
	if err := schema.Walk(func(component Component) error {
		walked = append(walked, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("run %d: walk schema: %v", run, err)
	}
	for index, want := range wantComponents {
		component := components[index]
		if component.Name() != want.name || component.ID() != walked[index] {
			t.Fatalf("run %d: component %d = %s/%v, want %s/%v", run, index, component.Name(), component.ID(), want.name, walked[index])
		}
		assertWildcardQNameGraphComponent(t, component, want, run, index)
	}
}

func assertWildcardQNameGraphComponent(t *testing.T, component Component, want wildcardQNameGraphWant, run, index int) {
	t.Helper()
	definition, ok := component.ComplexType()
	if !ok {
		t.Fatalf("run %d: component %d is not a complex type", run, index)
	}
	wildcard := directWildcardFromParticle(t, definition.Particle())
	constraint := wildcard.QNameConstraint()
	wantLoc := wildcardQNameDocumentLoc(t, want.source, want.document, `notQName="`+want.lexical+`"`)
	if !constraint.Present() || constraint.LexicalForm() != want.lexical || constraint.Loc() != wantLoc ||
		wildcard.Loc().Source() != want.source || !reflect.DeepEqual(constraint.Names(), want.names) {
		t.Fatalf("run %d: component %d QName facts = %#v, wildcard %s", run, index, constraint, wildcard.Loc())
	}
	if got := constraint.Tokens(); len(got) == 0 || got[0] != "local" {
		t.Fatalf("run %d: component %d lexical tokens = %v", run, index, got)
	}
	bindings := constraint.NamespaceBindings()
	assertWildcardQNameGraphDefaultBinding(t, bindings, want.defaultURI, run, index)
	if len(bindings) > 0 {
		bindings[0].Namespace = "changed"
	}
	tokens := constraint.Tokens()
	tokens[0] = "changed"
	names := constraint.Names()
	names[0] = QName{}
	again := wildcard.QNameConstraint()
	if again.Tokens()[0] != "local" || !reflect.DeepEqual(again.Names(), want.names) ||
		len(bindings) > 0 && again.NamespaceBindings()[0].Namespace == "changed" {
		t.Fatalf("run %d: component %d leaked mutable QName facts", run, index)
	}
}

func assertWildcardQNameGraphDefaultBinding(t *testing.T, bindings []IdentityNamespaceBinding, wantURI string, run, index int) {
	t.Helper()
	foundDefault := false
	for _, binding := range bindings {
		if binding.Prefix == "" {
			foundDefault = true
			if binding.Namespace != wantURI {
				t.Fatalf("run %d: component %d default binding = %q, want %q", run, index, binding.Namespace, wantURI)
			}
		}
	}
	if wantURI != "" && !foundDefault {
		t.Fatalf("run %d: component %d missing default binding", run, index)
	}
}

func wildcardQNameDocumentLoc(t *testing.T, source SourceID, document, marker string) Loc {
	t.Helper()
	index := strings.Index(document, marker)
	if index < 0 {
		t.Fatalf("document %q has no marker %q", source, marker)
	}
	line := 1
	column := 1
	for _, character := range document[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, source, line, column)
}

//nolint:gocognit // Keep unsupported notQName variants at each affected placement in one matrix.
func TestWildcardQNameExcludedShapes(t *testing.T) {
	forms := []struct {
		name          string
		attributes    string
		marker        string
		broaderMarker string
	}{
		{"defined", ` notQName="##defined"`, `notQName="##defined"`, ""},
		{"defined sibling", ` notQName="##definedSibling"`, `notQName="##definedSibling"`, ""},
		{"defined with QName", ` notQName="xs:string ##defined"`, `notQName="xs:string ##defined"`, ""},
		{"defined sibling with QName", ` notQName="xs:string ##definedSibling"`, `notQName="xs:string ##definedSibling"`, ""},
		{"defined zero", ` notQName="##defined" minOccurs="0" maxOccurs="0"`, `notQName="##defined"`, ""},
		{"lax", ` processContents="lax" notQName="xs:string"`, `notQName="xs:string"`, `processContents="lax"`},
		{"skip", ` processContents="skip" notQName="xs:string"`, `notQName="xs:string"`, `processContents="skip"`},
		{"negative namespace", ` notNamespace="##local" notQName="xs:string"`, `notQName="xs:string"`, `notNamespace="##local"`},
	}
	for _, form := range forms {
		for _, shape := range []string{"named", "inline", "group_ref"} {
			t.Run(form.name+"/"+shape, func(t *testing.T) {
				content := `<xs:choice><xs:any` + form.attributes + `/></xs:choice>`
				body := `<xs:complexType name="Record">` + content + `</xs:complexType>`
				if shape == "inline" {
					body = `<xs:element name="root"><xs:complexType>` + content + `</xs:complexType></xs:element>`
				}
				if shape == "group_ref" {
					body = `<xs:group name="G">` + content + `</xs:group><xs:complexType name="Record"><xs:group ref="t:G"/></xs:complexType>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">` + body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				if err == nil {
					t.Fatal("excluded notQName shape unexpectedly parsed")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				marker := form.marker
				if shape != "named" && form.broaderMarker != "" {
					marker = form.broaderMarker
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, marker) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded shape diagnostic = %s, want located unsupported", diagnostic)
				}
				wantSpec := "xsd10-structures#schema-document"
				if shape == "group_ref" {
					wantSpec = "xsd11-structures#cSchemaDocument"
				}
				if shape == "named" {
					wantSpec = schemaAnyParticleSpecRef(XSDVersion11)
					if !errors.Is(err, errSchemaAnyParticleUnsupported) {
						t.Fatalf("named direct wildcard lost cause: %v", err)
					}
				}
				if diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 {
					t.Fatalf("excluded shape spec/related = %q/%v, want %q/no related", diagnostic.SpecRef(), diagnostic.Related(), wantSpec)
				}
			})
		}
	}
}

func TestWildcardQNameAttributeWildcardStaysUnsupported(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		t.Run(shape, func(t *testing.T) {
			body := `<xs:complexType name="Record"><xs:anyAttribute notQName="xs:string"/></xs:complexType>`
			if shape == "inline" {
				body = `<xs:element name="root"><xs:complexType><xs:anyAttribute notQName="xs:string"/></xs:complexType></xs:element>`
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">` + body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("attribute wildcard notQName unexpectedly parsed")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute`) ||
				!errors.Is(err, ErrUnsupported) {
				t.Fatalf("attribute wildcard diagnostic = %s", diagnostic)
			}
		})
	}
}
