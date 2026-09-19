package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeExposesOrderedLocalAndReferencedAttributeUses(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" attributeFormDefault="unqualified">
  <xs:complexType name="Record">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:attribute name="first" type="xs:boolean"/>
    <xs:attribute name="second" type="xs:decimal" use="required"/>
    <xs:attribute name="qualified" type="xs:integer" form="qualified"/>
    <xs:attribute ref="r:global" use="required"/>
    <xs:attribute name="prohibited" type="xs:integer" use="prohibited"/>
  </xs:complexType>
  <xs:attribute name="global" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	assertOrderedAttributeUses(t, schema)
}

func assertOrderedAttributeUses(t *testing.T, schema Schema) {
	t.Helper()
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	definition := requireTestComplexTypeDefinition(t, components[0], "Record")
	uses := definition.AttributeUses()
	if len(uses) != 4 {
		t.Fatalf("attribute use count = %d, want 4", len(uses))
	}
	assertOrderedLocalAttributeUses(t, uses)
	assertOrderedAttributeReferenceUse(t, uses[3], components[1].ID())
	assertAttributeUsesDefensive(t, definition, uses)
}

func assertOrderedLocalAttributeUses(t *testing.T, uses []AttributeUse) {
	t.Helper()
	first, ok := uses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("use 0 type = %T, want LocalAttributeUse", uses[0])
	}
	wantFirst := mustTestQName(t, "", "first")
	if first.Name() != wantFirst || first.Use() != AttributeUseOptional {
		t.Fatalf("first use = %q/%q, want unqualified/optional", first.Name(), first.Use())
	}
	firstType, ok := first.TypeReference()
	wantBoolean := mustTestQName(t, testXSDNamespace, "boolean")
	if !ok || !firstType.IsBuiltin() || firstType.Name() != wantBoolean {
		t.Fatalf("first type = %#v/%t, want xs:boolean builtin", firstType, ok)
	}
	second, ok := uses[1].(LocalAttributeUse)
	wantSecond := mustTestQName(t, "", "second")
	if !ok || second.Name() != wantSecond || second.Use() != AttributeUseRequired {
		t.Fatalf("second use = %T/%q/%q, want required unqualified local", uses[1], second.Name(), second.Use())
	}
	qualified, ok := uses[2].(LocalAttributeUse)
	wantQualified := mustTestQName(t, "urn:root", "qualified")
	if !ok || qualified.Name() != wantQualified {
		t.Fatalf("qualified use = %T/%q, want qualified local", uses[2], qualified.Name())
	}
}

func assertOrderedAttributeReferenceUse(t *testing.T, value AttributeUse, wantTarget ComponentID) {
	t.Helper()
	reference, ok := value.(AttributeReferenceUse)
	if !ok {
		t.Fatalf("use 3 type = %T, want AttributeReferenceUse", value)
	}
	wantGlobal := mustTestQName(t, "urn:root", "global")
	if reference.Ref() != wantGlobal || reference.Use() != AttributeUseRequired {
		t.Fatalf("reference = %q/%q, want global/required", reference.Ref(), reference.Use())
	}
	if got := reference.TargetID(); got != wantTarget {
		t.Fatalf("reference target ID = %v, want %v", got, wantTarget)
	}
	if _, ok := value.(LocalAttributeUse); ok {
		t.Fatal("reference use copied local declaration facts")
	}
}

func assertAttributeUsesDefensive(t *testing.T, definition ComplexTypeDefinition, uses []AttributeUse) {
	t.Helper()
	uses[0] = nil
	wantFirst := mustTestQName(t, "", "first")
	copyOfUses := definition.AttributeUses()
	first, ok := copyOfUses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("defensive copy use type = %T, want LocalAttributeUse", copyOfUses[0])
	}
	if got := first.Name(); got != wantFirst {
		t.Fatalf("mutating AttributeUses changed completed definition: %q", got)
	}
}

func requireTestComplexTypeDefinition(t *testing.T, component Component, label string) ComplexTypeDefinition {
	t.Helper()
	definition, ok := component.ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s has no complex type definition", label)
	}
	return definition
}

func TestSchemaBridgeBuildsAttributeOnlyAndSimpleContentBodies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Only"><xs:attribute name="count" type="xs:integer"/></xs:complexType>
  <xs:complexType name="Code"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="kind" type="xs:boolean" use="required"/></xs:extension></xs:simpleContent></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	only, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Only has no complex type definition")
	}
	if only.Particle() != nil {
		t.Fatal("attribute-only body unexpectedly has a particle")
	}
	if got := len(only.AttributeUses()); got != 1 {
		t.Fatalf("attribute-only use count = %d, want 1", got)
	}
	code, ok := components[1].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Code has no complex type definition")
	}
	if code.Particle() != nil {
		t.Fatal("simpleContent body unexpectedly has a particle")
	}
	extension, ok := code.SimpleContentExtension()
	if !ok {
		t.Fatal("simpleContent extension view is absent")
	}
	if extension.Base() != mustTestQName(t, testXSDNamespace, "string") || extension.BaseLoc().IsZero() {
		t.Fatalf("simpleContent base = %q at %s", extension.Base(), extension.BaseLoc())
	}
	base, ok := extension.TypeReference()
	if !ok || !base.IsBuiltin() || base.Name() != extension.Base() {
		t.Fatalf("simpleContent type = %#v/%t, want builtin %q", base, ok, extension.Base())
	}
	if got := len(code.AttributeUses()); got != 1 {
		t.Fatalf("simpleContent use count = %d, want 1", got)
	}
}

func TestSchemaBridgeResolvesImportedAttributeUseByVisibility(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Record"><xs:attribute ref="o:global"/></xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:attribute name="global" type="xs:integer"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type definition")
	}
	uses := definition.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("attribute use count = %d, want 1", len(uses))
	}
	reference, ok := uses[0].(AttributeReferenceUse)
	if !ok {
		t.Fatalf("use type = %T, want AttributeReferenceUse", uses[0])
	}
	if reference.TargetID().Source() != SourceID("other.xsd") {
		t.Fatalf("reference target source = %q, want other.xsd", reference.TargetID().Source())
	}
}

func TestSchemaBridgeAttributeReferenceRequiresDirectImport(t *testing.T) {
	root := elementReferenceTestRoot(`<xs:complexType name="Record"><xs:attribute ref="o:foreign"/></xs:complexType>`)
	rootDocument := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
	other := elementReferenceTestSyntaxDocument(t, "other.xsd", `<xs:schema xmlns:xs="`+testXSDNamespace+`" targetNamespace="urn:reference-other"><xs:attribute name="foreign" type="xs:integer"/></xs:schema>`)
	schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDocument, other}}, Strict11)
	if err == nil {
		t.Fatal("attribute reference without an import returned a schema")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("attribute reference failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeReferenceNamespaceCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaAttributeReferenceNamespaceCode)
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "ref=") {
		t.Fatalf("diagnostic location = %s, want ref attribute location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != schemaAttributeUseResolveXSD11SpecRef || len(diagnostic.Related()) != 1 {
		t.Fatalf("diagnostic metadata = %s/%q/%v, want attr-use reference and one target", diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaAttributeReferenceNamespace) {
		t.Fatalf("diagnostic lost missing-import cause: %v", err)
	}
}

func TestSchemaBridgeRepairsChameleonAttributeNamespaces(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="chameleon.xsd"/></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `" attributeFormDefault="qualified">
  <xs:attribute name="shared" type="xs:integer"/>
  <xs:complexType name="Record"><xs:attribute name="local" type="xs:integer"/><xs:attribute ref="shared"/></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
	}, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definitionComponents := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
	if len(definitionComponents) != 1 {
		t.Fatalf("Record count = %d, want 1", len(definitionComponents))
	}
	definition, ok := definitionComponents[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type definition")
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("attribute use count = %d, want 2", len(uses))
	}
	wantLocal := mustTestQName(t, "urn:root", "local")
	local, ok := uses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("chameleon local use type = %T, want LocalAttributeUse", uses[0])
	}
	if got := local.Name(); got != wantLocal {
		t.Fatalf("chameleon local name = %q, want {urn:root}local", got)
	}
	wantShared := mustTestQName(t, "urn:root", "shared")
	shared, ok := uses[1].(AttributeReferenceUse)
	if !ok {
		t.Fatalf("chameleon reference use type = %T, want AttributeReferenceUse", uses[1])
	}
	if got := shared.Ref(); got != wantShared {
		t.Fatalf("chameleon reference name = %q, want {urn:root}shared", got)
	}
}

func TestSchemaBridgeBuildsLocalAttributeUsesAcrossLanguagePolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" attributeFormDefault="qualified">
  <xs:complexType name="Record"><xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute name="number" type="xs:integer"/></xs:complexType>
</xs:schema>`
	for _, policy := range []LanguagePolicy{Strict10, Strict11, Compatibility} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			definition := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
			uses := definition.AttributeUses()
			if len(uses) != 2 {
				t.Fatalf("complex type attribute uses = %#v, want two uses", definition.AttributeUses())
			}
			flag, ok := uses[0].(LocalAttributeUse)
			if !ok {
				t.Fatalf("qualified local use type = %T, want LocalAttributeUse", uses[0])
			}
			wantFlag := mustTestQName(t, "urn:root", "flag")
			if got := flag.Name(); got != wantFlag {
				t.Fatalf("qualified local name = %q, want {urn:root}flag", got)
			}
		})
	}
}

func TestSchemaBridgeAppliesXSD11LocalAttributeTargetNamespaceRules(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:complexType name="Record"><xs:attribute name="qualified" targetNamespace="urn:root" type="xs:integer"/></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
	wantQualified := mustTestQName(t, "urn:root", "qualified")
	qualified, ok := definition.AttributeUses()[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("targetNamespace use type = %T, want LocalAttributeUse", definition.AttributeUses()[0])
	}
	if got := qualified.Name(); got != wantQualified {
		t.Fatalf("targetNamespace local name = %q, want {urn:root}qualified", got)
	}

	for _, test := range []struct {
		name   string
		prefix string
		attr   string
		cause  error
	}{
		{
			name:   "missing containing target namespace",
			prefix: `<xs:schema xmlns:xs="` + testXSDNamespace + `">`,
			attr:   ` targetNamespace="urn:root"`,
			cause:  errSchemaAttributeTargetNamespace,
		},
		{
			name:   "mismatched target namespace",
			prefix: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">`,
			attr:   ` targetNamespace="urn:other"`,
			cause:  errSchemaAttributeTargetNamespace,
		},
		{
			name:   "target namespace with form",
			prefix: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">`,
			attr:   ` targetNamespace="urn:root" form="qualified"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) { assertInvalidLocalAttributeTargetNamespace(t, test.prefix, test.attr, test.cause) })
	}
}

func assertInvalidLocalAttributeTargetNamespace(t *testing.T, prefix, attr string, cause error) {
	t.Helper()
	root := prefix + `<xs:complexType name="Record"><xs:attribute name="qualified"` + attr + ` type="xs:integer"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted invalid local targetNamespace")
	}
	if len(schema.Components()) != 0 {
		t.Fatalf("failed schema has %d components", len(schema.Components()))
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic = %s/%s, want invalid composition", diagnostic.Class(), diagnostic.Code())
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost targetNamespace cause: %v", err)
	}
}

func TestSchemaBridgeAllocatesAnonymousLocalAttributeTypeIdentity(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:complexType name="Record"><xs:attribute name="amount"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, ok := schema.Components()[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type definition")
	}
	use, ok := definition.AttributeUses()[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("use type = %T, want LocalAttributeUse", definition.AttributeUses()[0])
	}
	if !use.DeclaredType().IsZero() {
		t.Fatalf("anonymous declared type = %q, want zero QName", use.DeclaredType())
	}
	reference, ok := use.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("anonymous type reference = %#v/%t", reference, ok)
	}
	if _, ok := reference.AnonymousID(); !ok {
		t.Fatal("anonymous local attribute type has no model identity")
	}
	if _, ok := reference.AnonymousType(); !ok {
		t.Fatal("anonymous local attribute type has no public model view")
	}
}

func TestSchemaBridgePreservesLocalAttributePrecisionDecimalPolicyContext(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Record"><xs:attribute name="value" type="xs:precisionDecimal"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted a named precisionDecimal local attribute type")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("precisionDecimal failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.Feature() != FeatureDatatypeFacets {
		t.Fatalf("diagnostic = %s/%q/%q, want precisionDecimal policy diagnostic", diagnostic, diagnostic.Code(), diagnostic.Feature())
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `type="xs:precisionDecimal"`) {
		t.Fatalf("diagnostic location = %s, want local type location", diagnostic.Loc())
	}
	if !errors.Is(err, errSchemaPrecisionDecimalVersion) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("diagnostic lost precisionDecimal policy causes: %v", err)
	}
}

func TestSchemaBridgeReportsLocalAttributeTypeCyclesAtTheUse(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType><xs:complexType name="Record"><xs:attribute name="value" type="r:First"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted a cyclic local attribute type")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("cyclic local attribute type returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeTypeCycleCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaAttributeTypeCycleCode)
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:First"`) || len(diagnostic.Related()) == 0 {
		t.Fatalf("diagnostic location/related = %s/%v, want local type context", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
		t.Fatalf("cycle diagnostic lost simple-type cycle cause: %v", err)
	}
}

func TestSchemaBridgeReframesInlineLocalAttributePrecisionDecimal(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:complexType name="Record"><xs:attribute name="value">
    <xs:simpleType><xs:restriction base="xs:precisionDecimal"/></xs:simpleType>
	  </xs:attribute></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted an inline precisionDecimal local attribute type")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("inline precisionDecimal failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.Feature() != FeatureDatatypeFacets {
		t.Fatalf("diagnostic = %s/%q/%q, want precisionDecimal policy diagnostic", diagnostic, diagnostic.Code(), diagnostic.Feature())
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "<xs:simpleType>") {
		t.Fatalf("diagnostic location = %s, want inline simpleType location", diagnostic.Loc())
	}
	if len(diagnostic.Related()) == 0 || !errors.Is(err, errSchemaPrecisionDecimalVersion) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("inline precisionDecimal metadata/causes = %v/%v", diagnostic.Related(), err)
	}
}

func TestSchemaBridgeReframesInlineLocalAttributeTypeCycle(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType>
  <xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>
  <xs:complexType name="Record"><xs:attribute name="value">
    <xs:simpleType><xs:restriction base="r:First"/></xs:simpleType>
  </xs:attribute></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted an inline cyclic local attribute type")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("inline cyclic local attribute type returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeTypeCycleCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaAttributeTypeCycleCode)
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "<xs:simpleType>") || len(diagnostic.Related()) == 0 {
		t.Fatalf("diagnostic location/related = %s/%v, want inline simpleType context", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
		t.Fatalf("inline cycle diagnostic lost simple-type cycle cause: %v", err)
	}
}

func TestSchemaBridgeReportsLocalAttributeDefaultsAndFixedAsUnsupported(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
		spec   string
	}{
		{name: "Strict10", policy: Strict10, spec: schemaAttributeUseXSD10SpecRef},
		{name: "Strict11", policy: Strict11, spec: schemaAttributeUseXSD11SpecRef},
	} {
		for _, constraint := range []string{"default", "fixed"} {
			t.Run(test.name+"/"+constraint, func(t *testing.T) {
				assertUnsupportedLocalAttributeValue(t, test.policy, test.spec, constraint)
			})
		}
	}
}

func assertUnsupportedLocalAttributeValue(t *testing.T, policy LanguagePolicy, spec, constraint string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Record"><xs:attribute name="value" type="xs:integer" ` + constraint + `="1"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discoverSchema accepted an unsupported local attribute value constraint")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("local attribute value constraint returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.SpecRef() != spec {
		t.Fatalf("diagnostic = %s/%q, want schema-syntax unsupported/%q", diagnostic, diagnostic.SpecRef(), spec)
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
		t.Fatalf("diagnostic lost unsupported local-use cause: %v", err)
	}
}

func TestSchemaBridgeResolvesForwardAndImportedLocalAttributeTypes(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Record"><xs:attribute name="forward" type="r:Amount"/><xs:attribute name="foreign" type="o:ForeignAmount"/></xs:complexType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="ForeignAmount"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, ok := schema.Components()[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type definition")
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("attribute use count = %d, want 2", len(uses))
	}
	forward, ok := uses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("forward use type = %T, want LocalAttributeUse", uses[0])
	}
	foreign, ok := uses[1].(LocalAttributeUse)
	if !ok {
		t.Fatalf("foreign use type = %T, want LocalAttributeUse", uses[1])
	}
	wantForward := mustTestQName(t, "urn:root", "Amount")
	wantForeign := mustTestQName(t, "urn:other", "ForeignAmount")
	if forward.DeclaredType() != wantForward || foreign.DeclaredType() != wantForeign {
		t.Fatalf("declared types = %q/%q, want forward/imported names", forward.DeclaredType(), foreign.DeclaredType())
	}
	forwardID, forwardOK := forward.TypeID()
	foreignID, foreignOK := foreign.TypeID()
	if !forwardOK || !foreignOK || forwardID.IsZero() || foreignID.IsZero() || forwardID == foreignID {
		t.Fatalf("named local type IDs = %v/%t and %v/%t, want distinct identities", forwardID, forwardOK, foreignID, foreignOK)
	}
}

func TestSchemaBridgeClassifiesSimpleContentBaseFailures(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		body  string
		class FailureClass
		code  string
		cause error
	}{
		{
			name:  "unresolved",
			base:  "r:Missing",
			class: FailureInvalid,
			code:  diagnosticSchemaSimpleContentBaseUnresolvedCode,
			cause: errSchemaSimpleContentBaseUnresolved,
		},
		{
			name:  "wrong kind",
			base:  "r:Record",
			class: FailureInvalid,
			code:  diagnosticSchemaSimpleContentBaseWrongKindCode,
			cause: errSchemaSimpleContentBaseWrongKind,
		},
		{
			name:  "identity-only type",
			base:  "xs:language",
			class: FailureUnsupported,
			code:  UnsupportedSchemaSyntaxCode,
			cause: errSchemaSimpleContentBaseUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { assertSimpleContentBaseFailure(t, test) })
	}
}

func TestSchemaBridgeClassifiesAmbiguousAttributeAndSimpleContentReferences(t *testing.T) {
	attributeName := mustTestQName(t, "urn:other", "shared")
	owner := schemaComponentRecord{
		id:   ComponentID{source: "root.xsd", ordinal: 1},
		kind: ComponentKindComplexTypeDefinition,
		name: mustTestQName(t, "urn:root", "Record"),
		loc:  mustTestLoc(t, "root.xsd", 2, 3),
	}
	first := schemaComponentRecord{
		id:   ComponentID{source: "one.xsd", ordinal: 1},
		kind: ComponentKindAttributeDeclaration,
		name: attributeName,
		loc:  mustTestLoc(t, "one.xsd", 2, 3),
	}
	second := schemaComponentRecord{
		id:   ComponentID{source: "two.xsd", ordinal: 1},
		kind: ComponentKindAttributeDeclaration,
		name: attributeName,
		loc:  mustTestLoc(t, "two.xsd", 2, 3),
	}
	records := []schemaComponentRecord{owner, first, second}
	byName := map[QName][]int{attributeName: {1, 2}}
	visibleSources := map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}}
	refLoc := mustTestLoc(t, "root.xsd", 3, 25)
	_, err := resolveSchemaAttributeReferenceUse(
		schemaAttributeUseInput{
			loc:       refLoc,
			reference: &schemaAttributeReferenceInput{name: attributeName, loc: refLoc},
		},
		owner,
		records,
		byName,
		visibleSources,
		make([]schemaAttributeTypeResult, len(records)),
		XSDVersion11,
	)
	if err == nil {
		t.Fatal("ambiguous attribute reference resolved")
	}
	attributeDiagnostic := requireDiagnostic(t, err)
	if attributeDiagnostic.Code() != "XSD3047" || attributeDiagnostic.Loc() != refLoc || !reflect.DeepEqual(attributeDiagnostic.Related(), []Loc{first.loc, second.loc}) {
		t.Fatalf("attribute ambiguity diagnostic = %s/%v/%v, want XSD3047 at ref with both targets", attributeDiagnostic.Code(), attributeDiagnostic.Loc(), attributeDiagnostic.Related())
	}
	if !errors.Is(err, errSchemaAttributeReferenceAmbiguous) {
		t.Fatalf("attribute ambiguity cause is not preserved: %v", err)
	}

	baseName := mustTestQName(t, "urn:other", "Base")
	baseFirst := first
	baseFirst.kind = ComponentKindSimpleTypeDefinition
	baseFirst.name = baseName
	baseSecond := second
	baseSecond.kind = ComponentKindSimpleTypeDefinition
	baseSecond.name = baseName
	baseRecords := []schemaComponentRecord{owner, baseFirst, baseSecond}
	baseLoc := mustTestLoc(t, "root.xsd", 4, 34)
	resolver := &schemaSimpleTypeResolver{
		records:        baseRecords,
		byName:         map[QName][]int{baseName: {1, 2}},
		visibleSources: visibleSources,
		states:         make(map[*schemaSimpleTypeInput]schemaSimpleTypeState),
		inputResults:   make(map[*schemaSimpleTypeInput]schemaSimpleTypeResult),
		results:        make([]schemaSimpleTypeResult, len(baseRecords)),
	}
	_, err = resolveSchemaSimpleContentExtension(
		&schemaComplexTypeSimpleContentBodyInput{base: schemaSimpleTypeReferenceInput{name: baseName, loc: baseLoc}},
		owner,
		baseRecords,
		schemaSimpleTypeResolution{results: resolver.results, resolver: resolver},
		XSDVersion11,
	)
	if err == nil {
		t.Fatal("ambiguous simpleContent base resolved")
	}
	simpleContentDiagnostic := requireDiagnostic(t, err)
	if simpleContentDiagnostic.Code() != "XSD3051" || simpleContentDiagnostic.Loc() != baseLoc || !reflect.DeepEqual(simpleContentDiagnostic.Related(), []Loc{baseFirst.loc, baseSecond.loc}) {
		t.Fatalf("simpleContent ambiguity diagnostic = %s/%v/%v, want XSD3051 at base with both targets", simpleContentDiagnostic.Code(), simpleContentDiagnostic.Loc(), simpleContentDiagnostic.Related())
	}
	if !errors.Is(err, errSchemaSimpleContentBaseAmbiguous) {
		t.Fatalf("simpleContent ambiguity cause is not preserved: %v", err)
	}
}

func TestSchemaBridgeAttributeDiagnosticCodeFamilyIsStable(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{name: "precisionDecimal policy", code: diagnosticSchemaPrecisionDecimalVersionCode, want: "XSD3030"},
		{name: "block and unresolved attribute reference", code: diagnosticSchemaBlockCode, want: "XSD3045"},
		{name: "wrong-kind attribute reference", code: diagnosticSchemaAttributeReferenceWrongKindCode, want: "XSD3046"},
		{name: "ambiguous attribute reference", code: diagnosticSchemaAttributeReferenceAmbiguousCode, want: "XSD3047"},
		{name: "attribute reference namespace", code: diagnosticSchemaAttributeReferenceNamespaceCode, want: "XSD3048"},
		{name: "unresolved simpleContent base", code: diagnosticSchemaSimpleContentBaseUnresolvedCode, want: "XSD3049"},
		{name: "wrong-kind simpleContent base", code: diagnosticSchemaSimpleContentBaseWrongKindCode, want: "XSD3050"},
		{name: "ambiguous simpleContent base", code: diagnosticSchemaSimpleContentBaseAmbiguousCode, want: "XSD3051"},
		{name: "duplicate attribute use", code: diagnosticSchemaAttributeUseDuplicateCode, want: "XSD3052"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.code != test.want {
				t.Fatalf("diagnostic code = %q, want stable %q", test.code, test.want)
			}
		})
	}
}

func assertSimpleContentBaseFailure(t *testing.T, test struct {
	name  string
	base  string
	body  string
	class FailureClass
	code  string
	cause error
}) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:complexType name="Record"><xs:simpleContent><xs:extension base="` + test.base + `">` + test.body + `</xs:extension></xs:simpleContent></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted an invalid simpleContent base")
	}
	if len(schema.Components()) != 0 {
		t.Fatalf("failed schema has %d components", len(schema.Components()))
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
		t.Fatalf("diagnostic = %s/%s, want %s/%s", diagnostic.Class(), diagnostic.Code(), test.class, test.code)
	}
	if !errors.Is(err, test.cause) {
		t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.SpecRef() == "" {
		t.Fatalf("diagnostic location/spec = %s/%q", diagnostic.Loc(), diagnostic.SpecRef())
	}
}

func TestSchemaBridgeRejectsUnsupportedAndInvalidAttributeUses(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		policy    LanguagePolicy
		class     FailureClass
		code      string
		cause     error
		locMarker string
	}{
		{
			name:      "local string is unsupported",
			body:      `<xs:attribute name="label" type="xs:string"/>`,
			policy:    Strict11,
			class:     FailureUnsupported,
			code:      UnsupportedSchemaSyntaxCode,
			cause:     errSchemaAttributeUseUnsupported,
			locMarker: `type="xs:string"`,
		},
		{
			name:      "identity-only local type is unsupported",
			body:      `<xs:attribute name="lang" type="xs:language"/>`,
			policy:    Strict11,
			class:     FailureUnsupported,
			code:      UnsupportedSchemaSyntaxCode,
			cause:     errSchemaAttributeUseUnsupported,
			locMarker: `type="xs:language"`,
		},
		{
			name:      "duplicate expanded names",
			body:      `<xs:attribute name="item" type="xs:integer"/><xs:attribute name="item" type="xs:decimal"/>`,
			policy:    Strict10,
			class:     FailureInvalid,
			code:      diagnosticSchemaAttributeUseDuplicateCode,
			cause:     errSchemaAttributeUseDuplicate,
			locMarker: `name="item" type="xs:decimal"`,
		},
		{
			name:      "unresolved reference",
			body:      `<xs:attribute ref="r:missing"/>`,
			policy:    Strict11,
			class:     FailureInvalid,
			code:      diagnosticSchemaAttributeReferenceUnresolvedCode,
			cause:     errSchemaAttributeReferenceUnresolved,
			locMarker: `ref="r:missing"`,
		},
		{
			name:      "wrong kind reference",
			body:      `<xs:attribute ref="r:Record"/>`,
			policy:    Strict11,
			class:     FailureInvalid,
			code:      diagnosticSchemaAttributeReferenceWrongKindCode,
			cause:     errSchemaAttributeReferenceWrongKind,
			locMarker: `ref="r:Record"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { assertInvalidOrUnsupportedAttributeUse(t, test) })
	}
}

func assertInvalidOrUnsupportedAttributeUse(t *testing.T, test struct {
	name      string
	body      string
	policy    LanguagePolicy
	class     FailureClass
	code      string
	cause     error
	locMarker string
}) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:complexType name="Record">` + test.body + `</xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
	if err == nil {
		t.Fatal("discoverSchema accepted invalid or unsupported attribute uses")
	}
	if len(schema.Components()) != 0 {
		t.Fatalf("failed schema has %d components", len(schema.Components()))
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
		t.Fatalf("diagnostic = %s/%s, want %s/%s", diagnostic.Class(), diagnostic.Code(), test.class, test.code)
	}
	if !errors.Is(err, test.cause) {
		t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want root.xsd location", diagnostic.Loc())
	}
	if test.locMarker != "" && diagnostic.Loc().Column() == 0 {
		t.Fatal("diagnostic has no column location")
	}
}

func TestSchemaBridgeProhibitedAttributeUsesAreNotMaterialized(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:attribute name="global" type="xs:integer"/>
  <xs:complexType name="Record"><xs:attribute name="local" type="xs:integer" use="prohibited"/><xs:attribute ref="r:global" use="prohibited"/></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, ok := schema.Components()[1].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type definition")
	}
	if uses := definition.AttributeUses(); uses != nil {
		t.Fatalf("prohibited uses = %#v, want nil", uses)
	}
}

func TestSchemaBridgeAttributeUseOrderingIsIndependentOfMapTraversal(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:complexType name="Record"><xs:attribute name="a" type="xs:integer"/><xs:attribute name="b" type="xs:decimal"/></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition, ok := schema.Components()[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type definition")
	}
	first := definition.AttributeUses()
	second := definition.AttributeUses()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated AttributeUses calls differ: %#v/%#v", first, second)
	}
	firstUse, ok := first[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("first use type = %T, want LocalAttributeUse", first[0])
	}
	secondUse, ok := first[1].(LocalAttributeUse)
	if !ok {
		t.Fatalf("second use type = %T, want LocalAttributeUse", first[1])
	}
	wantFirst := mustTestQName(t, "", "a")
	wantSecond := mustTestQName(t, "", "b")
	if firstUse.Name() != wantFirst || secondUse.Name() != wantSecond {
		t.Fatalf("attribute use order = %q, %q", first[0].Name(), first[1].Name())
	}
}

func TestSchemaBridgeAttributeBearingTypesRemainOutsideConsumers(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Record"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:attribute name="label" type="xs:integer"/></xs:complexType>
  <xs:element name="root" type="r:Record"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	instance := `<r:root xmlns:r="urn:root"><r:value>1</r:value></r:root>`
	err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
	if err == nil {
		t.Fatal("ValidateInstance accepted a type with attribute uses")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Feature() != FeatureInstanceValidation {
		t.Fatalf("validation diagnostic = %s/%q/%q, want explicit unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
	}
	if !errors.Is(err, errInstanceAttributes) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("validation diagnostic lost attribute-use cause: %v", err)
	}
	_, generateErr := GenerateGo(schema, "generated")
	if generateErr == nil {
		t.Fatal("GenerateGo accepted a type with attribute uses")
	}
	diagnostic = requireDiagnostic(t, generateErr)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureCodegen || !errors.Is(generateErr, ErrUnsupported) || !errors.Is(generateErr, errCodegenUnsupported) {
		t.Fatalf("codegen diagnostic = %s/%q, want explicit unsupported", diagnostic, diagnostic.Feature())
	}
}
