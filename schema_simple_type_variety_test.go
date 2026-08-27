package goxsd9

import (
	"errors"
	"testing"
)

//nolint:funlen,gocognit // Keep the complete public variety contract together.
func TestSchemaSimpleTypeVarietiesExposeOrderedResolvedReferences(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="ForwardUnion"><xs:union memberTypes="xs:string t:Atomic"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:union></xs:simpleType>
  <xs:simpleType name="Atomic"><xs:restriction base="xs:decimal"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="NamedList"><xs:list itemType="t:Atomic"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}

	components := schema.Components()
	if got, want := len(components), 4; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	atomicComponent := components[1]
	atomic, ok := atomicComponent.SimpleTypeDefinition()
	if !ok {
		t.Fatal("atomic simple type view is missing")
	}
	if got, want := atomic.Variety(), SimpleTypeVarietyAtomicRestriction; got != want {
		t.Fatalf("atomic variety = %q, want %q", got, want)
	}
	if atomic.VarietyLoc().IsZero() {
		t.Fatal("atomic variety location is missing")
	}
	atomicID := atomic.ID()
	if atomicID.IsZero() {
		t.Fatal("atomic component identity is missing")
	}
	if nodeID, nodeOK := atomic.NodeID(); !nodeOK || nodeID.IsZero() {
		t.Fatal("atomic model-node identity is missing")
	}
	base, ok := atomic.BaseReference()
	if !ok || base.Kind() != SimpleTypeReferenceBuiltin || base.Name().Local() != "decimal" {
		t.Fatalf("atomic base reference = %#v, want built-in decimal", base)
	}
	if _, componentOK := base.ComponentID(); componentOK {
		t.Fatal("built-in base unexpectedly has a component identity")
	}
	if atomic.DigitFacets().Kind() != DigitDatatypeDecimal {
		t.Fatalf("atomic digit kind = %q, want decimal", atomic.DigitFacets().Kind())
	}

	forward, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("forward union view is missing")
	}
	if got, want := forward.Variety(), SimpleTypeVarietyUnion; got != want {
		t.Fatalf("union variety = %q, want %q", got, want)
	}
	members := forward.MemberTypes()
	if got, want := len(members), 3; got != want {
		t.Fatalf("union member count = %d, want %d", got, want)
	}
	if members[0].Kind() != SimpleTypeReferenceBuiltin || members[0].Name().Local() != "string" {
		t.Fatalf("member 0 = %q/%q, want built-in string", members[0].Kind(), members[0].Name())
	}
	if members[1].Kind() != SimpleTypeReferenceNamed || members[1].Name().Local() != "Atomic" {
		t.Fatalf("member 1 = %q/%q, want named Atomic", members[1].Kind(), members[1].Name())
	}
	memberID, ok := members[1].ComponentID()
	if !ok || memberID != atomicID {
		t.Fatalf("member 1 component ID = %v/%t, want %v/true", memberID, ok, atomicID)
	}
	if members[2].Kind() != SimpleTypeReferenceAnonymous {
		t.Fatalf("member 2 kind = %q, want anonymous", members[2].Kind())
	}
	if _, componentOK := members[2].ComponentID(); componentOK {
		t.Fatal("anonymous member unexpectedly has a component identity")
	}
	anonymousID, ok := members[2].AnonymousID()
	if !ok || anonymousID.IsZero() {
		t.Fatal("anonymous member identity is missing")
	}
	anonymous, ok := members[2].AnonymousType()
	if !ok {
		t.Fatal("anonymous member model is missing")
	}
	if !anonymous.IsAnonymous() || !anonymous.Name().IsZero() {
		t.Fatalf("anonymous member identity facts = anonymous:%t name:%q", anonymous.IsAnonymous(), anonymous.Name())
	}
	if anonymous.Loc().IsZero() || anonymous.BaseLoc().IsZero() {
		t.Fatal("anonymous member locations are incomplete")
	}
	if anonymous.Base().Local() != "negativeInteger" || anonymous.Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("anonymous member model = base %q variety %q", anonymous.Base(), anonymous.Variety())
	}

	inlineList, ok := components[2].SimpleTypeDefinition()
	if !ok {
		t.Fatal("inline list view is missing")
	}
	if inlineList.Variety() != SimpleTypeVarietyList {
		t.Fatalf("inline list variety = %q, want list", inlineList.Variety())
	}
	item, ok := inlineList.ItemType()
	if !ok || item.Kind() != SimpleTypeReferenceAnonymous {
		t.Fatalf("inline list item = %q/%t, want anonymous", item.Kind(), ok)
	}
	itemType, ok := item.AnonymousType()
	if !ok || itemType.Base().Local() != "integer" || itemType.Loc().IsZero() {
		t.Fatalf("inline list item model = base %q loc %v", itemType.Base(), itemType.Loc())
	}

	namedList, ok := components[3].SimpleTypeDefinition()
	if !ok || namedList.Variety() != SimpleTypeVarietyList {
		t.Fatalf("named list view = %t/%q", ok, namedList.Variety())
	}
	namedItem, ok := namedList.ItemType()
	if !ok || namedItem.Kind() != SimpleTypeReferenceNamed || namedItem.Name().Local() != "Atomic" {
		t.Fatalf("named list item = %q/%q/%t", namedItem.Kind(), namedItem.Name(), ok)
	}

	members[0] = SimpleTypeReference{}
	again := forward.MemberTypes()
	if len(again) != 3 || again[0].Name().Local() != "string" {
		t.Fatalf("member accessor changed through copied slice: %#v", again)
	}
}

func TestSchemaSimpleTypeVarietyReferencesAreCopied(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="item"><xs:union memberTypes="xs:string t:later"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:union></xs:simpleType><xs:simpleType name="later"><xs:restriction base="xs:decimal"/></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	definition, ok := schema.Components()[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("simple type view is missing")
	}
	members := definition.MemberTypes()
	if len(members) != 3 {
		t.Fatalf("member count = %d, want 3", len(members))
	}
	firstLoc := members[0].Loc()
	members[0] = SimpleTypeReference{}
	if got := definition.MemberTypes()[0].Loc(); got != firstLoc {
		t.Fatalf("member location changed through copied slice: got %v, want %v", got, firstLoc)
	}
	if _, ok := definition.ItemType(); ok {
		t.Fatal("union unexpectedly exposes list item")
	}
}

//nolint:gocognit // Keep the focused invalid-reference matrix together.
func TestSchemaSimpleTypeVarietyRejectsInvalidReferencesAndDerivations(t *testing.T) {
	tests := []struct {
		name  string
		root  string
		code  string
		cause error
	}{
		{
			name:  "unresolved union member",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test"><xs:simpleType name="item"><xs:union memberTypes="t:missing"/></xs:simpleType></xs:schema>`,
			code:  diagnosticSchemaSimpleTypeUnresolvedCode,
			cause: errSchemaSimpleTypeBaseUnresolved,
		},
		{
			name:  "list item cannot be a list",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="nested"><xs:list itemType="xs:integer"/></xs:simpleType><xs:simpleType name="item"><xs:list itemType="t:nested"/></xs:simpleType></xs:schema>`,
			code:  diagnosticSchemaSimpleTypeBaseCode,
			cause: errSchemaSimpleTypeInvalidDerivation,
		},
		{
			name:  "restriction cannot derive from list",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="base"><xs:list itemType="xs:integer"/></xs:simpleType><xs:simpleType name="item"><xs:restriction base="t:base"/></xs:simpleType></xs:schema>`,
			code:  diagnosticSchemaSimpleTypeBaseCode,
			cause: errSchemaSimpleTypeInvalidDerivation,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err == nil {
				t.Fatal("discoverTestSchema accepted invalid simple-type input")
			}
			if schema.storage != nil {
				t.Fatal("discoverTestSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if diagnostic.Loc().IsZero() {
				t.Fatal("diagnostic lost its primary location")
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
		})
	}
}

func TestSchemaSimpleTypePrecisionDecimalPolicyRemainsBounded(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.0"><xs:simpleType name="item"><xs:union memberTypes="xs:precisionDecimal xs:string"/></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("discoverTestSchema accepted precisionDecimal under Strict10")
	}
	if schema.storage != nil {
		t.Fatal("discoverTestSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.Class() != FailureUnsupported {
		t.Fatalf("diagnostic = %s, want XSD3030 unsupported", diagnostic)
	}
	if !errors.Is(err, errSchemaPrecisionDecimalVersion) {
		t.Fatalf("diagnostic lost precisionDecimal policy cause: %v", err)
	}
}

//nolint:gocognit // Keep the complete anonymous element reference contract together.
func TestGlobalElementInlineSimpleTypePreservesAnonymousReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:element name="inline">
    <xs:simpleType>
      <xs:restriction base="xs:integer"/>
    </xs:simpleType>
  </xs:element>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	components := schema.Components()
	if len(components) != 1 {
		t.Fatalf("component count = %d, want 1", len(components))
	}
	element, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("global element view is missing")
	}
	if !element.DeclaredType().IsZero() {
		t.Fatalf("inline element declared type = %q, want zero QName", element.DeclaredType())
	}
	if typeID, hasTypeID := element.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("inline element type ID = (%v, %t), want zero,false", typeID, hasTypeID)
	}
	reference, ok := element.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("element type reference = %#v/%t, want anonymous", reference, ok)
	}
	if reference.Loc().IsZero() || reference.VarietyLoc().IsZero() {
		t.Fatal("anonymous element reference locations are incomplete")
	}
	if _, hasComponentID := reference.ComponentID(); hasComponentID {
		t.Fatal("anonymous element reference unexpectedly has a component ID")
	}
	anonymousID, ok := reference.AnonymousID()
	if !ok || anonymousID.IsZero() {
		t.Fatal("anonymous element reference identity is missing")
	}
	anonymous, ok := reference.AnonymousType()
	if !ok {
		t.Fatal("anonymous element type model is missing")
	}
	if !anonymous.IsAnonymous() || anonymous.Name() != (QName{}) {
		t.Fatalf("anonymous element model identity = anonymous:%t name:%q", anonymous.IsAnonymous(), anonymous.Name())
	}
	if anonymous.Loc() != reference.Loc() {
		t.Fatalf("anonymous model location = %s, want reference location %s", anonymous.Loc(), reference.Loc())
	}
	if anonymous.Base().Local() != "integer" || anonymous.BaseLoc().IsZero() {
		t.Fatalf("anonymous element base = %q at %s, want integer with location", anonymous.Base(), anonymous.BaseLoc())
	}
	inline, ok := element.InlineSimpleType()
	if !ok {
		t.Fatal("inline element convenience view is missing its model identity")
	}
	if nodeID, hasNodeID := inline.NodeID(); !hasNodeID || nodeID.IsZero() {
		t.Fatal("inline element convenience view is missing its model identity")
	}
	if inline.Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("inline element variety = %q, want atomic restriction", inline.Variety())
	}
}
