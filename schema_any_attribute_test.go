package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeExposesDirectAnyAttributeFacts(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "xsd10", policy: Strict10, version: "1.0"},
		{name: "xsd11", policy: Strict11, version: "1.1"},
		{name: "compatibility", policy: Compatibility, version: "1.1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDirectAnyAttributeFacts(t, test.policy, test.version)
		})
	}
}

func assertDirectAnyAttributeFacts(t *testing.T, policy LanguagePolicy, version string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:element name="before"/>
  <xs:complexType name="sequenceType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute processContents="&#x9;lax&#xA;" namespace="&#xA;##other&#x9;"/>
  </xs:complexType>
  <xs:complexType name="choiceType">
    <xs:choice><xs:element name="left" type="xs:integer"/><xs:element name="right" type="xs:integer"/></xs:choice>
    <xs:anyAttribute namespace="&#x9;##other&#xD;" processContents="&#xD;lax&#x9;"/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	components := schema.Components()
	assertAnyAttributeComponentNames(t, components)
	sequence := requireAnyAttributeComplexType(t, components[1], "sequence")
	choice := requireAnyAttributeComplexType(t, components[2], "choice")
	assertAnyAttributeViews(t, root, sequence, choice)
	assertAnyAttributeParticles(t, sequence, choice)
	assertAnyAttributeWalkOrder(t, schema, components)
	assertAnyAttributeComponentCopies(t, schema, components, sequence)
}

func assertAnyAttributeComponentNames(t *testing.T, components []Component) {
	t.Helper()
	if len(components) != 3 {
		t.Fatalf("component count = %d, want 3", len(components))
	}
	for index, wantName := range []string{"before", "sequenceType", "choiceType"} {
		if got := components[index].Name().Local(); got != wantName {
			t.Errorf("component %d name = %q, want %q", index, got, wantName)
		}
	}
}

func requireAnyAttributeComplexType(t *testing.T, component Component, label string) ComplexTypeDefinition {
	t.Helper()
	definition, ok := component.ComplexType()
	if !ok {
		t.Fatalf("%s component type = %T, want ComplexTypeDefinition", label, component)
	}
	return definition
}

func assertAnyAttributeViews(t *testing.T, root string, sequence, choice ComplexTypeDefinition) {
	t.Helper()
	sequenceAttribute, ok := sequence.AnyAttribute()
	if !ok {
		t.Fatal("sequence AnyAttribute is absent")
	}
	assertAnyAttributeFacts(t, sequenceAttribute,
		root,
		"<xs:anyAttribute processContents=",
		"namespace=\"&#xA;##other&#x9;\"",
		"processContents=\"&#x9;lax&#xA;\"",
	)

	choiceAttribute, ok := choice.AnyAttribute()
	if !ok {
		t.Fatal("choice AnyAttribute is absent")
	}
	assertAnyAttributeFacts(t, choiceAttribute,
		root,
		"<xs:anyAttribute namespace=",
		"namespace=\"&#x9;##other&#xD;\"",
		"processContents=\"&#xD;lax&#x9;\"",
	)
}

func assertAnyAttributeParticles(t *testing.T, sequence, choice ComplexTypeDefinition) {
	t.Helper()
	sequenceValue := sequence.Particle()
	sequenceParticle, ok := sequenceValue.(SequenceParticle)
	if !ok {
		t.Fatalf("sequence particle type = %T, want SequenceParticle", sequenceValue)
	}
	if got := len(sequenceParticle.Elements()); got != 1 {
		t.Errorf("sequence element count = %d, want 1", got)
	}

	choiceValue := choice.Particle()
	choiceParticle, ok := choiceValue.(ChoiceParticle)
	if !ok {
		t.Fatalf("choice particle type = %T, want ChoiceParticle", choiceValue)
	}
	if got := len(choiceParticle.Alternatives()); got != 2 {
		t.Errorf("choice element count = %d, want 2", got)
	}
}

func assertAnyAttributeWalkOrder(t *testing.T, schema Schema, components []Component) {
	t.Helper()
	walked := make([]ComponentID, 0, len(components))
	err := schema.Walk(func(component Component) error {
		walked = append(walked, component.ID())
		return nil
	})
	if err != nil {
		t.Fatalf("walk schema: %v", err)
	}
	for index, component := range components {
		if walked[index] != component.ID() {
			t.Errorf("walk item %d ID = %v, want %v", index, walked[index], component.ID())
		}
	}
}

func assertAnyAttributeComponentCopies(t *testing.T, schema Schema, components []Component, sequence ComplexTypeDefinition) {
	t.Helper()
	originalComponents := schema.Components()
	components[0] = Component{}
	components[1] = Component{}
	if got := schema.Components()[0].Name().Local(); got != "before" {
		t.Errorf("mutating Components result changed schema: first name = %q", got)
	}
	if !reflect.DeepEqual(originalComponents, schema.Components()) {
		t.Error("schema component results are not stable after caller mutation")
	}
	attribute, ok := sequence.AnyAttribute()
	if !ok || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
		t.Errorf("repeated AnyAttribute query = %#v, %v", attribute, ok)
	}
}

func TestSchemaBridgePreservesAnyAttributeIncludedSource(t *testing.T) {
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.1">
  <xs:include schemaLocation="child.xsd"/>
</xs:schema>`
	child := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="includedType">
    <xs:sequence/>
    <xs:anyAttribute namespace="##other" processContents="lax"/>
  </xs:complexType>
</xs:schema>`

	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: child},
	}, Strict11)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	if got := len(schema.Documents()); got != 2 {
		t.Fatalf("document count = %d, want 2", got)
	}

	components := schema.Components()
	if len(components) != 1 {
		t.Fatalf("component count = %d, want 1", len(components))
	}
	complexType, ok := components[0].ComplexType()
	if !ok {
		t.Fatalf("component type = %T, want ComplexTypeDefinition", components[0])
	}
	attribute, ok := complexType.AnyAttribute()
	if !ok {
		t.Fatal("included AnyAttribute is absent")
	}
	if got := attribute.Loc().Source(); got != "child.xsd" {
		t.Errorf("AnyAttribute source = %q, want child.xsd", got)
	}
	if got := attribute.NamespaceLoc().Source(); got != "child.xsd" {
		t.Errorf("namespace source = %q, want child.xsd", got)
	}
	if got := attribute.ProcessContentsLoc().Source(); got != "child.xsd" {
		t.Errorf("processContents source = %q, want child.xsd", got)
	}
}

func TestSchemaBridgeRejectsExcludedAnyAttributeForms(t *testing.T) {
	tests := []struct {
		name       string
		policy     LanguagePolicy
		version    string
		attributes string
		wantSpec   string
	}{
		{name: "defaults", policy: Strict10, version: "1.0", attributes: "", wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "default_namespace", policy: Strict11, version: "1.1", attributes: `processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "default_process_contents", policy: Strict11, version: "1.1", attributes: `namespace="##other"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "any_namespace", policy: Strict10, version: "1.0", attributes: `namespace="##any" processContents="lax"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "uri", policy: Strict10, version: "1.0", attributes: `namespace="urn:other" processContents="lax"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "uri_list", policy: Strict10, version: "1.0", attributes: `namespace="urn:one urn:two" processContents="lax"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "namespace_list", policy: Strict11, version: "1.1", attributes: `namespace="##local ##targetNamespace" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "strict", policy: Strict10, version: "1.0", attributes: `namespace="##other" processContents="strict"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "skip", policy: Compatibility, version: "1.1", attributes: `namespace="##other" processContents="skip"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "not_namespace", policy: Strict11, version: "1.1", attributes: `notNamespace="##local" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "not_qname", policy: Strict11, version: "1.1", attributes: `notQName="xs:string" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertExcludedAnyAttributeForm(t, test.policy, test.version, test.attributes, test.wantSpec)
		})
	}
}

func assertExcludedAnyAttributeForm(t *testing.T, policy LanguagePolicy, version, attributes, wantSpec string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="unsupportedType">
    <xs:sequence/>
    <xs:anyAttribute ` + attributes + `/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discover schema succeeded, want unsupported diagnostic")
	}
	assertZeroSchema(t, schema)
	assertAnyAttributeUnsupportedDiagnostic(t, err, wantSpec)
}

func TestSchemaBridgeRejectsMalformedAnyAttributeBeforeUnsupportedClassification(t *testing.T) {
	tests := []struct {
		name       string
		policy     LanguagePolicy
		version    string
		attributes string
		child      string
		wantCode   string
		wantMarker string
	}{
		{name: "namespace_composition", policy: Strict11, version: "1.1", attributes: `namespace="##any" notNamespace="##local"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##any"`},
		{name: "namespace_token", policy: Strict11, version: "1.1", attributes: `namespace="##bad" processContents="lax"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##bad"`},
		{name: "namespace_list_composition", policy: Strict11, version: "1.1", attributes: `namespace="##other urn:extra" processContents="lax"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##other urn:extra"`},
		{name: "process_contents_enum", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="relaxed"`, wantCode: invalidSchemaCompositionCode, wantMarker: `processContents="relaxed"`},
		{name: "not_namespace_token", policy: Strict11, version: "1.1", attributes: `notNamespace="##bad" processContents="lax"`, wantCode: invalidSchemaCompositionCode, wantMarker: `notNamespace="##bad"`},
		{name: "not_qname_lexical", policy: Strict11, version: "1.1", attributes: `notQName="bad:q:name" processContents="lax"`, wantCode: invalidSchemaConditionalCode, wantMarker: `notQName="bad:q:name"`},
		{name: "forbidden_attribute", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="lax" bogus="x"`, wantCode: invalidSchemaCompositionCode, wantMarker: `bogus="x"`},
		{name: "invalid_child", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="lax"`, child: `<xs:element/>`, wantCode: invalidSchemaCompositionCode, wantMarker: `<xs:element/>`},
		{name: "strict10_mismatch_does_not_hide_child_error", policy: Strict10, version: "1.0", attributes: `notQName="xs:string"`, child: `<xs:element/>`, wantCode: invalidSchemaCompositionCode, wantMarker: `<xs:element/>`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertMalformedAnyAttribute(t, test.policy, test.version, test.attributes, test.child, test.wantCode, test.wantMarker)
		})
	}
}

func assertMalformedAnyAttribute(t *testing.T, policy LanguagePolicy, version, attributes, child, wantCode, wantMarker string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="invalidType">
    <xs:sequence/>
    <xs:anyAttribute ` + attributes + `>` + child + `</xs:anyAttribute>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discover schema succeeded, want invalid diagnostic")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Class(); got != FailureInvalid {
		t.Errorf("diagnostic class = %v, want %v", got, FailureInvalid)
	}
	if got := diagnostic.Code(); got != wantCode {
		t.Errorf("diagnostic code = %q, want %q", got, wantCode)
	}
	if errors.Is(err, errSchemaAnyAttributeUnsupported) {
		t.Error("invalid diagnostic retained unsupported anyAttribute cause")
	}
	if got := diagnostic.Loc(); got != anyAttributeTestLoc(root, wantMarker) {
		t.Errorf("diagnostic location = %v, want %v", got, anyAttributeTestLoc(root, wantMarker))
	}
}

func TestSchemaBridgeKeepsAnyAttributeShapeBoundariesUnsupported(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		wantWildcardCause bool
	}{
		{
			name:              "any_attribute_only",
			body:              `<xs:complexType name="onlyWildcard"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name: "anonymous_inline",
			body: `<xs:element name="inline"><xs:complexType><xs:sequence/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType></xs:element>`,
		},
		{
			name:              "attribute_group",
			body:              `<xs:attributeGroup name="group"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:attributeGroup>`,
			wantWildcardCause: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.1">` + test.body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("discover schema succeeded, want unsupported diagnostic")
			}
			assertZeroSchema(t, schema)
			if test.wantWildcardCause {
				assertAnyAttributeUnsupportedDiagnostic(t, err, schemaAnyAttributeXSD11SpecRef)
				return
			}
			assertUnsupportedSchemaSyntaxDiagnostic(t, err)
		})
	}
}

func TestSchemaBridgeKeepsPinnedOpenAttrsDerivationUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema" version="1.1">
  <xs:complexType name="openAttrs">
    <xs:complexContent>
      <xs:restriction base="xs:anyType">
        <xs:anyAttribute namespace="##other" processContents="lax"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
</xs:schema>`

	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("pinned openAttrs fragment succeeded, want unsupported derivation boundary")
	}
	assertZeroSchema(t, schema)
	assertAnyAttributeUnsupportedDiagnostic(t, err, schemaAnyAttributeXSD11SpecRef)
}

func assertAnyAttributeFacts(t *testing.T, attribute AnyAttribute, source, elementMarker, namespaceMarker, processContentsMarker string) {
	t.Helper()
	if got := attribute.Namespace(); got != "##other" {
		t.Errorf("namespace = %q, want ##other", got)
	}
	if got := attribute.ProcessContents(); got != "lax" {
		t.Errorf("processContents = %q, want lax", got)
	}
	if got := attribute.Loc(); got != anyAttributeTestLoc(source, elementMarker) {
		t.Errorf("element location = %v, want %v", got, anyAttributeTestLoc(source, elementMarker))
	}
	if got := attribute.NamespaceLoc(); got != anyAttributeTestLoc(source, namespaceMarker) {
		t.Errorf("namespace location = %v, want %v", got, anyAttributeTestLoc(source, namespaceMarker))
	}
	if got := attribute.ProcessContentsLoc(); got != anyAttributeTestLoc(source, processContentsMarker) {
		t.Errorf("processContents location = %v, want %v", got, anyAttributeTestLoc(source, processContentsMarker))
	}
}

func assertAnyAttributeUnsupportedDiagnostic(t *testing.T, err error, wantSpec string) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Class(); got != FailureUnsupported {
		t.Errorf("diagnostic class = %v, want %v", got, FailureUnsupported)
	}
	if got := diagnostic.Code(); got != UnsupportedSchemaSyntaxCode {
		t.Errorf("diagnostic code = %q, want %q", got, UnsupportedSchemaSyntaxCode)
	}
	if got := diagnostic.Feature(); got != FeatureSchemaSyntax {
		t.Errorf("diagnostic feature = %v, want %v", got, FeatureSchemaSyntax)
	}
	if got := diagnostic.SpecRef(); got != wantSpec {
		t.Errorf("diagnostic spec ref = %q, want %q", got, wantSpec)
	}
	if !errors.Is(err, errSchemaAnyAttributeUnsupported) {
		t.Error("diagnostic does not retain anyAttribute unsupported cause")
	}
	if diagnostic.Loc().IsZero() {
		t.Error("diagnostic location is zero")
	}
}

func assertUnsupportedSchemaSyntaxDiagnostic(t *testing.T, err error) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Class(); got != FailureUnsupported {
		t.Errorf("diagnostic class = %v, want %v", got, FailureUnsupported)
	}
	if got := diagnostic.Code(); got != UnsupportedSchemaSyntaxCode {
		t.Errorf("diagnostic code = %q, want %q", got, UnsupportedSchemaSyntaxCode)
	}
	if got := diagnostic.Feature(); got != FeatureSchemaSyntax {
		t.Errorf("diagnostic feature = %v, want %v", got, FeatureSchemaSyntax)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Error("diagnostic does not retain unsupported sentinel")
	}
	if diagnostic.Loc().IsZero() {
		t.Error("diagnostic location is zero")
	}
}

func anyAttributeTestLoc(source, marker string) Loc {
	index := strings.Index(source, marker)
	if index < 0 {
		panic("test marker not found: " + marker)
	}
	line := 1
	column := 1
	for _, character := range source[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return Loc{source: "root.xsd", line: line, column: column}
}
