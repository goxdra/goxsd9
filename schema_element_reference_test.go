package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const elementReferenceTestXSDNamespace = "http://www.w3.org/2001/XMLSchema"

func TestSchemaElementReferenceParticlesResolveInEveryPolicy(t *testing.T) {
	policies := []LanguagePolicy{Compatibility, Strict10, Strict11}
	for _, policy := range policies {
		t.Run(string(policy), func(t *testing.T) {
			elementReferenceTestAssertPolicy(t, policy)
		})
	}
}

func elementReferenceTestAssertPolicy(t *testing.T, policy LanguagePolicy) {
	t.Helper()
	schema, resolver := elementReferenceTestSchema(t, policy)
	elementReferenceTestAssertDiscovery(t, schema, resolver)
	wantNames := []QName{
		mustTestQName(t, "urn:reference-root", "included"),
		mustTestQName(t, "urn:reference-other", "imported"),
		mustTestQName(t, "urn:reference-other", "shadowed"),
	}
	choice := elementReferenceTestComplexType(t, schema, "Choice")
	elementReferenceTestAssertChoice(t, schema, choice, wantNames)
	sequence := elementReferenceTestComplexType(t, schema, "Sequence")
	elementReferenceTestAssertSequence(t, sequence, wantNames)
}

func elementReferenceTestAssertDiscovery(t *testing.T, schema Schema, resolver *discoveryResolver) {
	t.Helper()
	if got, want := len(resolver.calls), 2; got != want {
		t.Fatalf("resolver call count = %d, want %d", got, want)
	}
	if resolver.calls[0].location != "chameleon.xsd" || resolver.calls[1].location != "other.xsd" {
		t.Fatalf("resolver locations = %q, %q, want include/import locations", resolver.calls[0].location, resolver.calls[1].location)
	}
	documents := schema.Documents()
	if got, want := len(documents), 3; got != want {
		t.Fatalf("document count = %d, want %d", got, want)
	}
	if got := documents[1].TargetNamespace(); got != "urn:reference-root" {
		t.Fatalf("chameleon target namespace = %q, want urn:reference-root", got)
	}
}

func elementReferenceTestAssertChoice(t *testing.T, schema Schema, definition ComplexTypeDefinition, wantNames []QName) {
	t.Helper()
	choiceParticle, ok := definition.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("choice particle = %T, want ChoiceParticle", definition.Particle())
	}
	alternatives := choiceParticle.Alternatives()
	if got, want := len(alternatives), 4; got != want {
		t.Fatalf("choice alternative count = %d, want %d", got, want)
	}
	elementReferenceTestAssertChoiceReferences(t, alternatives, wantNames)
	elementReferenceTestAssertChoiceLocal(t, alternatives)
	elementReferenceTestAssertChoiceTarget(t, schema, choiceParticle, alternatives, wantNames[0])
}

func elementReferenceTestAssertChoiceReferences(t *testing.T, alternatives []Particle, wantNames []QName) {
	t.Helper()
	for index, wantName := range wantNames {
		reference := elementReferenceTestReferenceAt(t, alternatives, index)
		if reference.Name() != wantName || reference.Ref() != wantName {
			t.Fatalf("alternative %d reference name = %q/%q, want %q", index, reference.Name(), reference.Ref(), wantName)
		}
		if reference.RefLoc().IsZero() || reference.RefLoc().Source() != "root.xsd" {
			t.Fatalf("alternative %d ref location = %s, want a root.xsd location", index, reference.RefLoc())
		}
		if reference.TargetID().IsZero() {
			t.Fatalf("alternative %d has a zero target ID", index)
		}
	}
	first := elementReferenceTestReferenceAt(t, alternatives, 0)
	second := elementReferenceTestReferenceAt(t, alternatives, 1)
	if got, want := first.Occurrences().String(), "1/1"; got != want {
		t.Fatalf("default reference occurrences = %q, want %q", got, want)
	}
	if got, want := second.Occurrences().String(), "0/unbounded"; got != want {
		t.Fatalf("unbounded reference occurrences = %q, want %q", got, want)
	}
}

func elementReferenceTestAssertChoiceLocal(t *testing.T, alternatives []Particle) {
	t.Helper()
	local := elementReferenceTestElementAt(t, alternatives, 3)
	if local.Name() != mustTestQName(t, "", "local") {
		t.Fatalf("local alternative name = %q, want local", local.Name())
	}
}

func elementReferenceTestAssertChoiceTarget(
	t *testing.T,
	schema Schema,
	choiceParticle ChoiceParticle,
	alternatives []Particle,
	wantName QName,
) {
	t.Helper()
	included := schema.FindKind(ComponentKindElementDeclaration, wantName)
	if len(included) != 1 {
		t.Fatalf("included global count = %d, want 1", len(included))
	}
	includedDeclaration, ok := included[0].ElementDeclaration()
	if !ok || includedDeclaration.DeclaredType() != mustTestQName(t, elementReferenceTestXSDNamespace, "integer") {
		t.Fatal("included global declaration facts are incomplete")
	}
	reference := elementReferenceTestReferenceAt(t, alternatives, 0)
	if got, want := reference.TargetID(), included[0].ID(); got != want {
		t.Fatalf("included target ID = %v, want %v", got, want)
	}
	before := schema.Components()
	alternatives[0] = nil
	ownedAlternatives := choiceParticle.Alternatives()
	ownedReference := elementReferenceTestReferenceAt(t, ownedAlternatives, 0)
	if ownedReference.TargetID() != reference.TargetID() {
		t.Fatal("mutating Alternatives changed the reference particle")
	}
	if !reflect.DeepEqual(before, schema.Components()) {
		t.Fatal("particle queries mutated the completed schema")
	}
}

func elementReferenceTestAssertSequence(t *testing.T, definition ComplexTypeDefinition, wantNames []QName) {
	t.Helper()
	sequenceParticle, ok := definition.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("sequence particle = %T, want SequenceParticle", definition.Particle())
	}
	particles := sequenceParticle.Particles()
	if got, want := len(particles), 3; got != want {
		t.Fatalf("sequence particle count = %d, want %d after 0/0 omission", got, want)
	}
	first := elementReferenceTestReferenceAt(t, particles, 0)
	if first.Name() != wantNames[0] || first.Occurrences().String() != "1/1" {
		t.Fatalf("sequence first particle = %#v, want default included reference", particles[0])
	}
	sequenceLocal := elementReferenceTestElementAt(t, particles, 1)
	if sequenceLocal.Name() != mustTestQName(t, "", "sequenceLocal") {
		t.Fatalf("sequence second particle = %#v, want sequenceLocal", particles[1])
	}
	if got, want := sequenceLocal.Occurrences().String(), "0/18446744073709551616"; got != want {
		t.Fatalf("above-uint64 local occurrences = %q, want %q", got, want)
	}
	last := elementReferenceTestReferenceAt(t, particles, 2)
	if last.Name() != mustTestQName(t, "urn:reference-root", "last") || last.Occurrences().String() != "2/unbounded" {
		t.Fatalf("sequence third particle = %#v, want last reference", particles[2])
	}
	if got, want := len(sequenceParticle.Elements()), 1; got != want {
		t.Fatalf("sequence local Elements count = %d, want %d", got, want)
	}
	if got := sequenceParticle.Elements()[0].Name(); got != sequenceLocal.Name() {
		t.Fatalf("sequence Elements name = %q, want %q", got, sequenceLocal.Name())
	}
	particles[0] = nil
	if sequenceParticle.Particles()[0] == nil {
		t.Fatal("mutating Particles changed the completed sequence")
	}
}

func elementReferenceTestReferenceAt(t *testing.T, particles []Particle, index int) ElementReferenceParticle {
	t.Helper()
	reference, ok := particles[index].(ElementReferenceParticle)
	if !ok {
		t.Fatalf("particle %d = %T, want ElementReferenceParticle", index, particles[index])
	}
	return reference
}

func elementReferenceTestElementAt(t *testing.T, particles []Particle, index int) ElementParticle {
	t.Helper()
	element, ok := particles[index].(ElementParticle)
	if !ok {
		t.Fatalf("particle %d = %T, want ElementParticle", index, particles[index])
	}
	return element
}

func TestSchemaElementReferenceDiagnosticsAreLocatedAndVersioned(t *testing.T) {
	cases := []struct {
		name        string
		policy      LanguagePolicy
		root        string
		wantCode    string
		wantSpec    string
		wantCause   error
		wantRelated int
	}{
		{
			name:        "unresolved XSD 1.0",
			policy:      Strict10,
			root:        elementReferenceTestRoot(`<xs:complexType name="Choice"><xs:choice><xs:element ref="r:missing"/></xs:choice></xs:complexType>`),
			wantCode:    diagnosticSchemaElementReferenceUnresolvedCode,
			wantSpec:    schemaElementReferenceXSD10SpecRef,
			wantCause:   errSchemaElementReferenceUnresolved,
			wantRelated: 0,
		},
		{
			name:        "wrong kind XSD 1.1",
			policy:      Strict11,
			root:        elementReferenceTestRoot(`<xs:complexType name="Choice"><xs:choice><xs:element ref="r:notElement"/></xs:choice></xs:complexType><xs:simpleType name="notElement"><xs:restriction base="xs:integer"/></xs:simpleType>`),
			wantCode:    diagnosticSchemaElementReferenceWrongKindCode,
			wantSpec:    schemaElementReferenceXSD11SpecRef,
			wantCause:   errSchemaElementReferenceWrongKind,
			wantRelated: 1,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			elementReferenceTestAssertDiagnosticCase(t, test)
		})
	}

	elementReferenceTestAssertMissingImportDiagnostic(t)
}

func elementReferenceTestAssertDiagnosticCase(t *testing.T, test struct {
	name        string
	policy      LanguagePolicy
	root        string
	wantCode    string
	wantSpec    string
	wantCause   error
	wantRelated int
}) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, test.policy)
	if err == nil {
		t.Fatal("reference failure returned a schema")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("reference failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.wantCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.wantCode)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, "ref=") {
		t.Fatalf("diagnostic location = %s, want ref attribute location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != test.wantSpec {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.wantSpec)
	}
	if len(diagnostic.Related()) != test.wantRelated {
		t.Fatalf("diagnostic related count = %d, want %d", len(diagnostic.Related()), test.wantRelated)
	}
	if !errors.Is(err, test.wantCause) {
		t.Fatalf("diagnostic cause does not match %v: %v", test.wantCause, err)
	}
}

func elementReferenceTestAssertMissingImportDiagnostic(t *testing.T) {
	t.Helper()
	root := elementReferenceTestRoot(`<xs:complexType name="Choice"><xs:choice><xs:element ref="o:foreign"/></xs:choice></xs:complexType>`)
	rootDocument := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
	otherDocument := elementReferenceTestSyntaxDocument(t, "other.xsd", `<xs:schema xmlns:xs="`+elementReferenceTestXSDNamespace+`" targetNamespace="urn:reference-other"><xs:element name="foreign" type="xs:integer"/></xs:schema>`)
	schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDocument, otherDocument}}, Strict11)
	if err == nil {
		t.Fatal("foreign reference without import returned a schema")
	}
	if schema.storage != nil {
		t.Fatal("foreign reference failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaElementReferenceNamespaceCode || diagnostic.SpecRef() != schemaElementReferenceImportXSD11SpecRef {
		t.Fatalf("foreign reference diagnostic = %s/%q, want namespace/import diagnostic", diagnostic, diagnostic.SpecRef())
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "ref=") {
		t.Fatalf("foreign reference location = %s, want ref attribute location", diagnostic.Loc())
	}
	targetLoc := Loc{}
	for _, node := range otherDocument.root.children {
		child, childOK := node.(*syntaxElement)
		if !childOK || child.name.local != "element" {
			continue
		}
		targetLoc = child.loc
		break
	}
	if len(diagnostic.Related()) != 1 || diagnostic.Related()[0] != targetLoc {
		t.Fatalf("foreign reference related locations = %v, want imported target declaration", diagnostic.Related())
	}
	if !errors.Is(err, errSchemaElementReferenceNamespace) {
		t.Fatalf("foreign reference cause does not match: %v", err)
	}
}

func TestSchemaElementReferenceDownstreamConsumersRejectWithoutCopying(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + elementReferenceTestXSDNamespace + `" xmlns:r="urn:reference-root" targetNamespace="urn:reference-root">
  <xs:complexType name="Choice"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:complexType>
  <xs:element name="root" type="r:Choice"/>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, _ := elementReferenceTestSchemaFromRoot(t, root, Strict11)

	instance := `<r:root xmlns:r="urn:reference-root"><r:item>1</r:item></r:root>`
	instanceSource, err := NewResolvedSource(context.Background(), "instance.xml", io.NopCloser(strings.NewReader(instance)))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	err = ValidateInstance(schema, instanceSource.SourceID(), instanceSource.stream())
	if err == nil {
		t.Fatal("validation accepted a reference particle")
	}
	validationDiagnostic := requireDiagnostic(t, err)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Feature() != FeatureInstanceValidation || validationDiagnostic.Code() != UnsupportedInstanceValidationCode {
		t.Fatalf("validation diagnostic = %s, want explicit unsupported validation", validationDiagnostic)
	}
	if validationDiagnostic.Loc().Source() != "root.xsd" || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errInstanceChoiceTarget) {
		t.Fatalf("validation diagnostic evidence/cause is incomplete: %v", err)
	}

	_, err = GenerateGo(schema, "reference_example")
	if err == nil {
		t.Fatal("code generation accepted a reference particle")
	}
	codegenDiagnostic := requireDiagnostic(t, err)
	if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Feature() != FeatureCodegen || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
		t.Fatalf("codegen diagnostic = %s, want explicit unsupported code generation", codegenDiagnostic)
	}
	if codegenDiagnostic.Loc().Source() != "root.xsd" || codegenDiagnostic.SpecRef() == "" {
		t.Fatalf("codegen diagnostic location/spec = %s/%q, want located specification-backed failure", codegenDiagnostic.Loc(), codegenDiagnostic.SpecRef())
	}
}

func elementReferenceTestSchema(t *testing.T, policy LanguagePolicy) (Schema, *discoveryResolver) {
	root := `<xs:schema xmlns:xs="` + elementReferenceTestXSDNamespace + `" xmlns:r="urn:reference-root" xmlns:o="urn:reference-other" xmlns:p="urn:reference-root" xmlns="urn:reference-root" targetNamespace="urn:reference-root">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:reference-other" schemaLocation="other.xsd"/>
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element ref="included"/>
      <xs:element ref="o:imported" minOccurs="0" maxOccurs="unbounded"/>
      <xs:element ref="p:shadowed" xmlns="urn:reference-other" xmlns:p="urn:reference-other"/>
      <xs:element name="local" type="xs:integer"/>
    </xs:choice>
  </xs:complexType>
  <xs:complexType name="Sequence">
    <xs:sequence>
      <xs:element ref="included"/>
      <xs:element ref="zero" minOccurs="0" maxOccurs="0"/>
      <xs:element name="sequenceLocal" type="xs:integer" minOccurs="0" maxOccurs="18446744073709551616"/>
      <xs:element ref="last" minOccurs="2" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	return elementReferenceTestSchemaFromRoot(t, root, policy)
}

func elementReferenceTestSchemaFromRoot(t *testing.T, root string, policy LanguagePolicy) (Schema, *discoveryResolver) {
	t.Helper()
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{
		"chameleon.xsd": {
			id: "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + elementReferenceTestXSDNamespace + `">
  <xs:element name="included" type="xs:integer"/>
  <xs:element name="zero" type="xs:integer"/>
  <xs:element name="last" type="xs:integer"/>
</xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + elementReferenceTestXSDNamespace + `" targetNamespace="urn:reference-other">
  <xs:element name="imported" type="xs:integer"/>
  <xs:element name="shadowed" type="xs:integer"/>
</xs:schema>`,
		},
	}}
	rootSource, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(root)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	discovery, err := discoverSyntaxWithPolicy(rootSource, resolver, policy)
	if err != nil {
		t.Fatalf("discoverSyntaxWithPolicy: %v", err)
	}
	schema, err := newSchemaFromDiscoveryWithPolicy(discovery, policy)
	if err != nil {
		t.Fatalf("newSchemaFromDiscoveryWithPolicy: %v", err)
	}
	return schema, resolver
}

func elementReferenceTestComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	t.Helper()
	component := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:reference-root", local))
	if len(component) != 1 {
		t.Fatalf("complex type %q count = %d, want 1", local, len(component))
	}
	definition, ok := component[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("complex type %q has no definition view", local)
	}
	return definition
}

func elementReferenceTestRoot(model string) string {
	return `<xs:schema xmlns:xs="` + elementReferenceTestXSDNamespace + `" xmlns:r="urn:reference-root" xmlns:o="urn:reference-other" targetNamespace="urn:reference-root">` + model + `</xs:schema>`
}

func elementReferenceTestAttributeLoc(t *testing.T, input, needle string) Loc {
	t.Helper()
	offset := strings.Index(input, needle)
	if offset < 0 {
		t.Fatalf("input does not contain %q", needle)
	}
	line := 1
	column := 1
	for _, character := range input[:offset] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, "root.xsd", line, column)
}

func elementReferenceTestSyntaxDocument(t *testing.T, source SourceID, contents string) *syntaxDocument {
	t.Helper()
	resolved, err := NewResolvedSource(context.Background(), source, io.NopCloser(strings.NewReader(contents)))
	if err != nil {
		t.Fatalf("NewResolvedSource(%q): %v", source, err)
	}
	document, err := decodeResolvedSyntaxForDiscovery(resolved)
	if err != nil {
		t.Fatalf("decodeResolvedSyntaxForDiscovery(%q): %v", source, err)
	}
	return document
}
