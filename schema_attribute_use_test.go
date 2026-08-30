package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep the complete immutable attribute-use contract together.
func TestSchemaBridgeExposesBoundedAttributeUses(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:attribute name="global" type="xs:integer"/>
  <xs:attribute name="prohibitedGlobal" type="xs:decimal"/>
  <xs:complexType name="Record">
    <xs:attribute name="direct" type="xs:integer"/>
    <xs:attribute name="named" type="t:Amount" use="required"/>
    <xs:attribute name="inline" use="prohibited"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute>
    <xs:attribute ref="t:global" use="required"/>
    <xs:attribute ref="t:prohibitedGlobal" use="prohibited"/>
  </xs:complexType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	typeComponent := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
	if len(typeComponent) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(typeComponent))
	}
	definition, ok := typeComponent[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	uses := definition.AttributeUses()
	if got, want := len(uses), 5; got != want {
		t.Fatalf("attribute use count = %d, want %d", got, want)
	}
	local, ok := uses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("use 0 type = %T, want LocalAttributeUse", uses[0])
	}
	if local.Name() != mustTestQName(t, "", "direct") || local.DeclaredType() != mustTestQName(t, testXSDNamespace, "integer") || local.Use() != AttributeUseOptional {
		t.Fatalf("local use facts = %q/%q/%q", local.Name(), local.DeclaredType(), local.Use())
	}
	if _, typeReferenceOK := local.TypeReference(); !typeReferenceOK {
		t.Fatal("local built-in type reference is missing")
	}
	named, ok := uses[1].(LocalAttributeUse)
	if !ok {
		t.Fatalf("use 1 type = %T, want LocalAttributeUse", uses[1])
	}
	if named.Name() != mustTestQName(t, "", "named") || named.Use() != AttributeUseRequired {
		t.Fatalf("named local use facts = %q/%q", named.Name(), named.Use())
	}
	typeID, ok := named.TypeID()
	if !ok || typeID != schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Amount"))[0].ID() {
		t.Fatalf("named local type ID = %v/%t", typeID, ok)
	}
	prohibitedLocal, ok := uses[2].(LocalAttributeUse)
	if !ok || prohibitedLocal.Use() != AttributeUseProhibited || prohibitedLocal.Name() != mustTestQName(t, "", "inline") {
		t.Fatalf("prohibited local use = %T/%q/%q, want inline/prohibited", uses[2], prohibitedLocal.Name(), prohibitedLocal.Use())
	}
	if _, anonymousOK := prohibitedLocal.TypeReference(); !anonymousOK {
		t.Fatal("prohibited local anonymous type reference is missing")
	}
	reference, ok := uses[3].(AttributeReferenceUse)
	if !ok {
		t.Fatalf("use 3 type = %T, want AttributeReferenceUse", uses[3])
	}
	global := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "global"))
	prohibitedGlobal := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "prohibitedGlobal"))
	if len(global) != 1 || len(prohibitedGlobal) != 1 || reference.Ref() != global[0].Name() || reference.TargetID() != global[0].ID() || reference.Use() != AttributeUseRequired {
		t.Fatalf("reference facts = %q/%v/%v/%q", reference.Ref(), reference.TargetID(), global, reference.Use())
	}
	prohibitedReference, ok := uses[4].(AttributeReferenceUse)
	if !ok || prohibitedReference.TargetID() != prohibitedGlobal[0].ID() || prohibitedReference.Use() != AttributeUseProhibited {
		t.Fatalf("prohibited reference facts = %T/%v/%q, want prohibitedGlobal/prohibited", uses[4], prohibitedReference.TargetID(), prohibitedReference.Use())
	}
	if reference.RefLoc().IsZero() || local.Loc().IsZero() || local.TypeLoc().IsZero() {
		t.Fatal("attribute use locations are incomplete")
	}
	before := definition.AttributeUses()
	before[0] = nil
	if !reflect.DeepEqual(before[1:], definition.AttributeUses()[1:]) {
		t.Fatal("mutating returned attribute-use slice changed its contents")
	}
	if got := len(definition.AttributeUses()); got != 5 {
		t.Fatalf("attribute use count after mutation = %d, want 5", got)
	}
}

//nolint:gocognit // Keep simpleContent base identity and ordered-use assertions together.
func TestSchemaBridgeExposesSimpleContentExtension(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:complexType name="Price">
    <xs:simpleContent>
      <xs:extension base="t:Amount">
        <xs:attribute name="currency" type="xs:integer"/>
        <xs:attribute name="code" type="xs:decimal" use="required"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	typeComponent := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Price"))
	if len(typeComponent) != 1 {
		t.Fatalf("Price component count = %d, want 1", len(typeComponent))
	}
	definition, ok := typeComponent[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Price has no complex type view")
	}
	if definition.Particle() != nil {
		t.Fatal("simpleContent extension unexpectedly has a particle")
	}
	extension, ok := definition.SimpleContentExtension()
	if !ok || extension.Base() != mustTestQName(t, "urn:test", "Amount") || extension.BaseLoc().IsZero() {
		t.Fatalf("simpleContent extension = %#v/%t", extension, ok)
	}
	baseType, ok := extension.TypeReference()
	if !ok || !baseType.IsNamed() {
		t.Fatalf("simpleContent base type reference = %#v/%t", baseType, ok)
	}
	amount := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Amount"))
	if len(amount) != 1 {
		t.Fatalf("Amount component count = %d, want 1", len(amount))
	}
	baseID, baseOK := baseType.ComponentID()
	if !baseOK || baseID != amount[0].ID() {
		t.Fatalf("simpleContent base type ID = %v/%t, want %v/true", baseID, baseOK, amount[0].ID())
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("simpleContent attribute use count = %d, want 2", len(uses))
	}
	for index, want := range []string{"currency", "code"} {
		local, ok := uses[index].(LocalAttributeUse)
		if !ok || local.Name().Local() != want {
			t.Fatalf("simpleContent use %d = %T/%q, want local %q", index, uses[index], local.Name(), want)
		}
	}
}

//nolint:gocognit // Keep both policy paths and builtin-reference identity assertions together.
func TestSchemaBridgeExposesBuiltinStringSimpleContentBaseInBothPolicies(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, policy := range policies {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Text"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="length" type="xs:integer"/></xs:extension></xs:simpleContent></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Text"))
			if len(components) != 1 {
				t.Fatalf("Text component count = %d, want 1", len(components))
			}
			definition, ok := components[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Text has no complex type view")
			}
			extension, ok := definition.SimpleContentExtension()
			if !ok {
				t.Fatal("Text has no simpleContent extension")
			}
			reference, ok := extension.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "string") {
				t.Fatalf("string base reference = %#v/%t, want builtin xs:string", reference, ok)
			}
			if _, ok := extension.TypeID(); ok {
				t.Fatal("builtin string simpleContent base has a synthetic component ID")
			}
			if uses := definition.AttributeUses(); len(uses) != 1 {
				t.Fatalf("string simpleContent attribute use count = %d, want 1", len(uses))
			}
		})
	}
}

//nolint:gocognit // Keep precisionDecimal identity and edition-policy assertions together.
func TestSchemaBridgeExposesPrecisionDecimalLocalAttributeUsesInStrict11(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1"><xs:simpleType name="PDecimal"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType><xs:complexType name="Record"><xs:attribute name="builtin" type="xs:precisionDecimal" use="required"/><xs:attribute name="named" type="t:PDecimal"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
	if len(components) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("precisionDecimal attribute use count = %d, want 2", len(uses))
	}
	builtin, ok := uses[0].(LocalAttributeUse)
	if !ok || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "precisionDecimal") || builtin.Use() != AttributeUseRequired {
		t.Fatalf("builtin precisionDecimal use = %T/%q/%q, want required xs:precisionDecimal", uses[0], builtin.DeclaredType(), builtin.Use())
	}
	builtinReference, ok := builtin.TypeReference()
	if !ok || !builtinReference.IsBuiltin() || builtinReference.Name() != builtin.DeclaredType() {
		t.Fatalf("builtin precisionDecimal reference = %#v/%t, want builtin declared type", builtinReference, ok)
	}
	if _, builtinTypeIDOK := builtin.TypeID(); builtinTypeIDOK {
		t.Fatal("builtin precisionDecimal local use has a synthetic component ID")
	}
	named, ok := uses[1].(LocalAttributeUse)
	if !ok || named.DeclaredType() != mustTestQName(t, "urn:test", "PDecimal") || named.Use() != AttributeUseOptional {
		t.Fatalf("named precisionDecimal use = %T/%q/%q, want optional t:PDecimal", uses[1], named.DeclaredType(), named.Use())
	}
	namedReference, ok := named.TypeReference()
	if !ok || !namedReference.IsNamed() || namedReference.Name() != named.DeclaredType() {
		t.Fatalf("named precisionDecimal reference = %#v/%t, want named declared type", namedReference, ok)
	}
	pdecimal := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "PDecimal"))
	if len(pdecimal) != 1 {
		t.Fatalf("PDecimal component count = %d, want 1", len(pdecimal))
	}
	if typeID, ok := named.TypeID(); !ok || typeID != pdecimal[0].ID() {
		t.Fatalf("named precisionDecimal type ID = %v/%t, want %v/true", typeID, ok, pdecimal[0].ID())
	}
}

func TestSchemaBridgeRejectsBuiltinPrecisionDecimalLocalUseInStrict10(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Record"><xs:attribute name="value" type="xs:precisionDecimal"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("discoverSchema accepted XSD 1.1 precisionDecimal in Strict10")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("Strict10 precisionDecimal failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic = %s, want located XSD3030 precisionDecimal policy diagnostic", diagnostic)
	}
	if !errors.Is(err, errSchemaPrecisionDecimalVersion) {
		t.Fatal("Strict10 precisionDecimal diagnostic lost errSchemaPrecisionDecimalVersion")
	}
}

func TestSchemaBridgeRejectsNamedPrecisionDecimalLocalUseInStrict10(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="PDecimal"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType><xs:complexType name="Record"><xs:attribute name="value" type="t:PDecimal"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("discoverSchema accepted a named XSD 1.1 precisionDecimal type in Strict10")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("Strict10 named precisionDecimal failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic = %s, want located XSD3030 precisionDecimal policy diagnostic", diagnostic)
	}
	if !errors.Is(err, errSchemaPrecisionDecimalVersion) {
		t.Fatal("Strict10 named precisionDecimal diagnostic lost errSchemaPrecisionDecimalVersion")
	}
}

func TestSchemaBridgeKeepsGlobalPrecisionDecimalPolicyNarrow(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:attribute name="value" type="xs:precisionDecimal"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema widened global attribute precisionDecimal support")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("global precisionDecimal failure returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.SpecRef() != schemaAttributeTypeXSD11SpecRef {
		t.Fatalf("diagnostic = %s/%q, want global attribute-type unsupported", diagnostic, diagnostic.SpecRef())
	}
	if !errors.Is(err, errSchemaAttributeTypeUnsupported) {
		t.Fatal("global precisionDecimal diagnostic lost errSchemaAttributeTypeUnsupported")
	}
}

func TestSchemaBridgeRejectsProhibitedWrongKindAttributeReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="NotAnAttribute"><xs:restriction base="xs:integer"/></xs:simpleType><xs:complexType name="Record"><xs:attribute ref="t:NotAnAttribute" use="prohibited"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted a prohibited reference to a non-attribute")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("wrong-kind prohibited reference returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeReferenceWrongKindCode || diagnostic.SpecRef() != schemaAttributeUseResolveXSD11SpecRef {
		t.Fatalf("diagnostic = %s/%q, want wrong-kind attribute-reference diagnostic", diagnostic, diagnostic.SpecRef())
	}
	if len(diagnostic.Related()) == 0 || !errors.Is(err, errSchemaAttributeReferenceWrongKind) {
		t.Fatalf("wrong-kind prohibited reference lost related location or cause: %v", err)
	}
}

// TestSchemaBridgeExposesIBM... mirrors the supported simpleContent portion of
// the pinned IBM D3_3_4 fixture. The fixture's unrelated unbounded sequence is
// intentionally left outside this bounded schema-model test.
func TestSchemaBridgeExposesIBMD3_3_4SimpleContentShape(t *testing.T) {
	root := `<schema xmlns="` + testXSDNamespace + `" xmlns:sv="urn:ibm" targetNamespace="urn:ibm" version="1.1"><simpleType name="decType"><restriction base="precisionDecimal"/></simpleType><complexType name="elementType"><simpleContent><extension base="string"><attribute name="scaleType" type="sv:decType"/><attribute name="value" type="precisionDecimal" use="required"/></extension></simpleContent></complexType></schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:ibm", "elementType"))
	if len(components) != 1 {
		t.Fatalf("elementType component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("elementType has no complex type view")
	}
	extension, ok := definition.SimpleContentExtension()
	if !ok {
		t.Fatal("elementType has no simpleContent extension")
	}
	base, ok := extension.TypeReference()
	if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "string") {
		t.Fatalf("IBM simpleContent base = %#v/%t, want builtin string", base, ok)
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 || uses[0].Name().Local() != "scaleType" || uses[1].Name().Local() != "value" {
		t.Fatalf("IBM attribute-use order = %d/%q/%q, want scaleType,value", len(uses), uses[0].Name().Local(), uses[1].Name().Local())
	}
}

func TestSchemaBridgeAppliesQualifiedAttributeDefault(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" attributeFormDefault="qualified">
  <xs:complexType name="Record"><xs:attribute name="qualified" type="xs:integer"/></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
	if len(components) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok || len(definition.AttributeUses()) != 1 {
		t.Fatalf("Record definition = %#v/%t", definition, ok)
	}
	qualified, qualifiedOK := definition.AttributeUses()[0].(LocalAttributeUse)
	if !qualifiedOK {
		t.Fatal("qualified attribute use is not local")
	}
	if got := qualified.Name(); got != mustTestQName(t, "urn:test", "qualified") {
		t.Fatalf("qualified default name = %q", got)
	}
}

func TestSchemaBridgeResolvesForwardAndCrossDocumentAttributeReferences(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Record">
    <xs:attribute ref="r:foreign"/>
    <xs:attribute ref="r:foreignRequired" use="required"/>
  </xs:complexType>
</xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:attribute name="foreign" type="xs:decimal"/>
  <xs:attribute name="foreignRequired" type="xs:decimal"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: foreign},
	}, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	record := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
	foreignAttribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:other", "foreign"))
	if len(record) != 1 || len(foreignAttribute) != 1 {
		t.Fatalf("resolved components = %d/%d, want one complex type and one global attribute", len(record), len(foreignAttribute))
	}
	definition, ok := record[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("reference use count = %d, want 2", len(uses))
	}
	wantNames := []QName{
		mustTestQName(t, "urn:other", "foreign"),
		mustTestQName(t, "urn:other", "foreignRequired"),
	}
	for index, value := range uses {
		reference, ok := value.(AttributeReferenceUse)
		if !ok {
			t.Fatalf("use %d type = %T, want AttributeReferenceUse", index, value)
		}
		if reference.Ref() != wantNames[index] {
			t.Fatalf("use %d target = %v/%q, want %v/%q", index, reference.TargetID(), reference.Ref(), foreignAttribute[0].ID(), foreignAttribute[0].Name())
		}
	}
}

func TestSchemaBridgeRejectsDuplicateExpandedAttributeUseNames(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:complexType name="Record">
    <xs:attribute name="item" type="xs:integer"/>
    <xs:attribute name="item" type="xs:decimal" use="prohibited"/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted duplicate expanded local attribute names")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("duplicate local attribute names returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeUseDuplicateCode || diagnostic.SpecRef() != schemaAttributeUseXSD11SpecRef {
		t.Fatalf("diagnostic = %s/%q, want duplicate attribute-use diagnostic", diagnostic, diagnostic.SpecRef())
	}
	if diagnostic.Loc() != mustTestLoc(t, "root.xsd", 4, 5) || !reflect.DeepEqual(diagnostic.Related(), []Loc{mustTestLoc(t, "root.xsd", 3, 5)}) {
		t.Fatalf("duplicate locations = %s/%v, want later/earlier local declarations", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaAttributeUseDuplicate) {
		t.Fatal("duplicate local attribute diagnostic lost its cause")
	}
}

func TestSchemaBridgeResolvesAndRetainsProhibitedReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
	  <xs:complexType name="Record"><xs:attribute ref="t:later" use="prohibited"/></xs:complexType>
  <xs:attribute name="later" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	record := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
	if len(record) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(record))
	}
	definition, ok := record[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	uses := definition.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("prohibited reference count = %d, want 1", len(uses))
	}
	prohibited, ok := uses[0].(AttributeReferenceUse)
	global := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "later"))
	if !ok || len(global) != 1 || prohibited.Use() != AttributeUseProhibited || prohibited.TargetID() != global[0].ID() {
		t.Fatalf("prohibited reference = %T/%q/%v, want retained prohibited target", uses[0], prohibited.Use(), prohibited.TargetID())
	}
}

func TestSchemaBridgeReportsProhibitedUnresolvedAttributeReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:complexType name="Record"><xs:attribute ref="missing" use="prohibited"/></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted an unresolved prohibited reference")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("unresolved prohibited reference returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeReferenceUnresolvedCode {
		t.Fatalf("diagnostic = %s, want unresolved attribute-reference diagnostic", diagnostic)
	}
	if diagnostic.Loc().IsZero() || diagnostic.SpecRef() != schemaAttributeUseResolveXSD11SpecRef {
		t.Fatalf("diagnostic metadata = %s/%q, want located XSD 1.1 attr-use resolution", diagnostic.Loc(), diagnostic.SpecRef())
	}
	if !errors.Is(err, errSchemaAttributeReferenceUnresolved) {
		t.Fatal("unresolved prohibited reference lost its cause")
	}
}

//nolint:gocognit // Keep invalid, unsupported, and zero-schema contracts together.
func TestSchemaBridgeRejectsBoundedAttributeUseFailuresWithoutSchema(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		code     string
		class    FailureClass
		wantSpec string
	}{
		{
			name:  "both name and ref",
			body:  `<xs:attribute name="item" ref="item" type="xs:integer"/>`,
			code:  invalidSchemaCompositionCode,
			class: FailureInvalid,
		},
		{
			name:  "invalid use",
			body:  `<xs:attribute name="item" type="xs:integer" use="sometimes"/>`,
			code:  invalidSchemaCompositionCode,
			class: FailureInvalid,
		},
		{
			name:  "ref has type",
			body:  `<xs:attribute ref="item" type="xs:integer"/>`,
			code:  invalidSchemaCompositionCode,
			class: FailureInvalid,
		},
		{
			name:     "default excluded",
			body:     `<xs:attribute name="item" type="xs:integer" default="1"/>`,
			code:     UnsupportedSchemaSyntaxCode,
			class:    FailureUnsupported,
			wantSpec: schemaAttributeUseXSD11SpecRef,
		},
		{
			name:     "string excluded",
			body:     `<xs:attribute name="item" type="xs:string"/>`,
			code:     UnsupportedSchemaSyntaxCode,
			class:    FailureUnsupported,
			wantSpec: schemaAttributeTypeXSD11SpecRef,
		},
		{
			name:     "missing type excluded",
			body:     `<xs:attribute name="item"/>`,
			code:     UnsupportedSchemaSyntaxCode,
			class:    FailureUnsupported,
			wantSpec: schemaAttributeUseXSD11SpecRef,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Record">` + test.body + `</xs:complexType><xs:attribute name="item" type="xs:integer"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("discoverSchema accepted an excluded or malformed attribute use")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("attribute-use failure returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want %s/%q", diagnostic, test.class, test.code)
			}
			if test.wantSpec != "" && diagnostic.SpecRef() != test.wantSpec {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.wantSpec)
			}
			if diagnostic.Loc().IsZero() {
				t.Fatal("attribute-use diagnostic is not located")
			}
		})
	}
}

//nolint:gocognit // Keep both XSD editions and both excluded constraints together.
func TestSchemaBridgeReportsLocalDefaultAndFixedAsLocatedUnsupported(t *testing.T) {
	policies := []struct {
		name  string
		value LanguagePolicy
		spec  string
	}{
		{name: "Strict10", value: Strict10, spec: schemaAttributeUseXSD10SpecRef},
		{name: "Strict11", value: Strict11, spec: schemaAttributeUseXSD11SpecRef},
	}
	constraints := []string{"default", "fixed"}
	for _, policy := range policies {
		for _, constraint := range constraints {
			t.Run(policy.name+"/"+constraint, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Record"><xs:attribute name="item" type="xs:integer" ` + constraint + `="1"/></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err == nil {
					t.Fatal("discoverSchema accepted an excluded local attribute constraint")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("local attribute constraint failure returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
					t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
				}
				if diagnostic.SpecRef() != policy.spec || diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Column() == 0 {
					t.Fatalf("diagnostic metadata = %s/%q, want located %s attribute-use diagnostic", diagnostic.Loc(), diagnostic.SpecRef(), policy.spec)
				}
				if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
					t.Fatalf("unsupported local attribute constraint lost its causes: %v", err)
				}
			})
		}
	}
}

func TestSchemaBridgeRetainsDirectParticleAndAttributeUses(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Record"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice><xs:attribute name="label" type="xs:decimal"/></xs:complexType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
	if len(components) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	if _, particleOK := definition.Particle().(ChoiceParticle); !particleOK {
		t.Fatalf("Record particle = %T, want ChoiceParticle", definition.Particle())
	}
	uses := definition.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("Record attribute use count = %d, want 1", len(uses))
	}
	local, ok := uses[0].(LocalAttributeUse)
	if !ok || local.Name() != mustTestQName(t, "", "label") {
		t.Fatalf("Record attribute use = %T/%q, want local label", uses[0], local.Name())
	}
}
