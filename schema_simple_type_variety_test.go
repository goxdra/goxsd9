package goxsd9

import (
	"errors"
	"reflect"
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
	if atomic.IsString() {
		t.Fatal("decimal simple type reported string identity")
	}
	if atomic.VarietyLoc().IsZero() {
		t.Fatal("atomic variety location is missing")
	}
	atomicID := atomic.ID()
	if atomicID.IsZero() {
		t.Fatal("atomic component identity is missing")
	}
	if atomicID.Source() != atomicComponent.Document() || atomicID.Ordinal() == 0 {
		t.Fatalf("atomic component ID facts = %q/%d", atomicID.Source(), atomicID.Ordinal())
	}
	if nodeID, nodeOK := atomic.NodeID(); !nodeOK || nodeID.IsZero() {
		t.Fatal("atomic model-node identity is missing")
	}
	base, ok := atomic.BaseReference()
	if !ok || base.Kind() != SimpleTypeReferenceBuiltin || base.Name().Local() != "decimal" {
		t.Fatalf("atomic base reference = %#v, want built-in decimal", base)
	}
	if base.QName() != base.Name() || base.Variety() != SimpleTypeVarietyAtomicRestriction || base.VarietyLoc().IsZero() {
		t.Fatalf("atomic base reference facts = %q/%q/%s", base.QName(), base.Variety(), base.VarietyLoc())
	}
	if !base.IsBuiltin() || base.IsNamed() || base.IsAnonymous() {
		t.Fatalf("atomic base reference kind predicates are inconsistent")
	}
	if _, hasID := base.ID(); hasID {
		t.Fatal("built-in base unexpectedly has an ID alias")
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
	if forward.IsString() {
		t.Fatal("union simple type reported string identity")
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
	if !members[1].IsNamed() || members[1].IsBuiltin() || members[1].IsAnonymous() || members[1].QName() != members[1].Name() {
		t.Fatal("named union member kind facts are inconsistent")
	}
	memberID, ok := members[1].ComponentID()
	if !ok || memberID != atomicID {
		t.Fatalf("member 1 component ID = %v/%t, want %v/true", memberID, ok, atomicID)
	}
	if aliasID, aliasOK := members[1].ID(); !aliasOK || aliasID != memberID {
		t.Fatalf("member 1 ID alias = %v/%t, want %v/true", aliasID, aliasOK, memberID)
	}
	if _, hasAnonymousType := members[1].AnonymousType(); hasAnonymousType {
		t.Fatal("named union member unexpectedly exposes an anonymous model")
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
	if anonymous.IsString() {
		t.Fatal("negativeInteger anonymous type reported string identity")
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
	if inlineList.IsString() {
		t.Fatal("inline list simple type reported string identity")
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
	if namedList.IsString() {
		t.Fatal("named list simple type reported string identity")
	}
	namedItem, ok := namedList.ItemType()
	if !ok || namedItem.Kind() != SimpleTypeReferenceNamed || namedItem.Name().Local() != "Atomic" {
		t.Fatalf("named list item = %q/%q/%t", namedItem.Kind(), namedItem.Name(), ok)
	}
	if aliasItem, aliasOK := namedList.ListItemType(); !aliasOK || aliasItem.Name() != namedItem.Name() {
		t.Fatalf("named list item alias = %q/%t, want %q/true", aliasItem.Name(), aliasOK, namedItem.Name())
	}
	if aliasMembers := forward.UnionMemberTypes(); len(aliasMembers) != len(members) {
		t.Fatalf("union member alias count = %d, want %d", len(aliasMembers), len(members))
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

//nolint:gocognit // Keep valid deferred list/union restriction coverage together.
func TestSchemaSimpleTypeVarietyDefersListAndUnionRestrictions(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "list restriction",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="base"><xs:list itemType="xs:integer"/></xs:simpleType><xs:simpleType name="item"><xs:restriction base="t:base"><xs:minLength value="3"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
		{
			name: "union restriction",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="base"><xs:union memberTypes="xs:string xs:integer"/></xs:simpleType><xs:simpleType name="item"><xs:restriction base="t:base"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("discoverTestSchema accepted deferred list/union restriction")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverTestSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
				t.Fatalf("diagnostic = %s, want unsupported/%s", diagnostic, UnsupportedSchemaSyntaxCode)
			}
			if diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.SpecRef() != schemaSimpleTypeXSD11SpecRef {
				t.Fatalf("diagnostic feature/spec ref = %q/%q, want %q/%q", diagnostic.Feature(), diagnostic.SpecRef(), FeatureSchemaSyntax, schemaSimpleTypeXSD11SpecRef)
			}
			if diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleTypeRestrictionUnsupported) {
				t.Fatalf("diagnostic lost location or cause: %v", err)
			}
		})
	}
}

func TestSchemaSimpleTypeVarietyAllowsUnionListItems(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="ItemUnion"><xs:union memberTypes="xs:string xs:negativeInteger"/></xs:simpleType>
  <xs:simpleType name="NamedUnionList"><xs:list itemType="t:ItemUnion"/></xs:simpleType>
  <xs:simpleType name="InlineUnionList"><xs:list><xs:simpleType><xs:union memberTypes="xs:string xs:negativeInteger"/></xs:simpleType></xs:list></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}

	named := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "NamedUnionList"))
	if len(named) != 1 {
		t.Fatalf("NamedUnionList matches = %d, want 1", len(named))
	}
	namedDefinition, ok := named[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("NamedUnionList simple type view is missing")
	}
	item, ok := namedDefinition.ItemType()
	if !ok || item.Kind() != SimpleTypeReferenceNamed || item.Name().Local() != "ItemUnion" || item.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("NamedUnionList item = %q/%q/%q/%t, want named union", item.Kind(), item.Name(), item.Variety(), ok)
	}
	itemID, itemIDOK := item.ComponentID()
	unionID := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "ItemUnion"))[0].ID()
	if !itemIDOK || itemID != unionID {
		t.Fatalf("NamedUnionList item ID = %v/%t, want %v/true", itemID, itemIDOK, unionID)
	}

	inline := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "InlineUnionList"))
	if len(inline) != 1 {
		t.Fatalf("InlineUnionList matches = %d, want 1", len(inline))
	}
	inlineDefinition, ok := inline[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("InlineUnionList simple type view is missing")
	}
	inlineItem, ok := inlineDefinition.ItemType()
	if !ok || inlineItem.Kind() != SimpleTypeReferenceAnonymous || inlineItem.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("InlineUnionList item = %q/%q/%t, want anonymous union", inlineItem.Kind(), inlineItem.Variety(), ok)
	}
	anonymous, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineUnionList anonymous union model is missing")
	}
	members := anonymous.MemberTypes()
	if len(members) != 2 || members[0].Name().Local() != "string" || members[1].Name().Local() != "negativeInteger" {
		t.Fatalf("InlineUnionList member order = %#v, want string then negativeInteger", members)
	}
}

func TestSchemaSimpleTypeVarietyAllowsUnionListItemsInStrict10(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="ItemUnion"><xs:union memberTypes="xs:string xs:negativeInteger"/></xs:simpleType>
  <xs:simpleType name="UnionList"><xs:list itemType="t:ItemUnion"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "UnionList"))
	if len(components) != 1 {
		t.Fatalf("UnionList matches = %d, want 1", len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("UnionList simple type view is missing")
	}
	item, ok := definition.ItemType()
	if !ok || item.Kind() != SimpleTypeReferenceNamed || item.Name().Local() != "ItemUnion" || item.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("UnionList item = %q/%q/%q/%t, want named union", item.Kind(), item.Name(), item.Variety(), ok)
	}
}

//nolint:gocognit // Keep the strict XSD 1.0 list/union/list identity proof together.
func TestSchemaSimpleTypeVarietyStrict10AllowsListValuedUnionMember(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="InnerList"><xs:list itemType="xs:integer"/></xs:simpleType>
  <xs:simpleType name="ItemUnion"><xs:union memberTypes="t:InnerList xs:string"/></xs:simpleType>
  <xs:simpleType name="OuterList"><xs:list itemType="t:ItemUnion"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	inner := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "InnerList"))
	union := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "ItemUnion"))
	outer := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "OuterList"))
	if len(inner) != 1 || len(union) != 1 || len(outer) != 1 {
		t.Fatalf("list-union components = %d/%d/%d, want one each", len(inner), len(union), len(outer))
	}

	innerDefinition, ok := inner[0].SimpleTypeDefinition()
	if !ok || innerDefinition.Variety() != SimpleTypeVarietyList {
		t.Fatalf("InnerList definition = %#v/%t, want list", innerDefinition, ok)
	}
	innerItem, ok := innerDefinition.ItemType()
	if !ok || !innerItem.IsBuiltin() || innerItem.Name().Local() != "integer" || innerItem.Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("InnerList item = %q/%q/%q/%t, want built-in atomic integer", innerItem.Kind(), innerItem.Name(), innerItem.Variety(), ok)
	}

	unionDefinition, ok := union[0].SimpleTypeDefinition()
	if !ok || unionDefinition.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("ItemUnion definition = %#v/%t, want union", unionDefinition, ok)
	}
	unionMembers := unionDefinition.MemberTypes()
	if len(unionMembers) != 2 || !unionMembers[0].IsNamed() || unionMembers[0].Name().Local() != "InnerList" || unionMembers[0].Variety() != SimpleTypeVarietyList {
		t.Fatalf("ItemUnion member 0 = %#v, want named list", unionMembers)
	}
	if memberID, hasID := unionMembers[0].ComponentID(); !hasID || memberID != inner[0].ID() {
		t.Fatalf("ItemUnion list member identity = %v/%t, want %v/true", memberID, hasID, inner[0].ID())
	}
	if unionMembers[1].Kind() != SimpleTypeReferenceBuiltin || unionMembers[1].Name().Local() != "string" || unionMembers[1].Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("ItemUnion member 1 = %q/%q/%q, want built-in atomic string", unionMembers[1].Kind(), unionMembers[1].Name(), unionMembers[1].Variety())
	}

	outerDefinition, ok := outer[0].SimpleTypeDefinition()
	if !ok || outerDefinition.Variety() != SimpleTypeVarietyList {
		t.Fatalf("OuterList definition = %#v/%t, want list", outerDefinition, ok)
	}
	outerItem, ok := outerDefinition.ItemType()
	if !ok || !outerItem.IsNamed() || outerItem.Name().Local() != "ItemUnion" || outerItem.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("OuterList item = %q/%q/%q/%t, want named union", outerItem.Kind(), outerItem.Name(), outerItem.Variety(), ok)
	}
	if itemID, hasID := outerItem.ComponentID(); !hasID || itemID != union[0].ID() {
		t.Fatalf("OuterList union item identity = %v/%t, want %v/true", itemID, hasID, union[0].ID())
	}
}

//nolint:gocognit // Keep the edition-specific invalid diagnostic matrix together.
func TestSchemaSimpleTypeVarietyRejectsNonAtomicTransitiveUnionListMembers(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="InnerList"><xs:list itemType="xs:integer"/></xs:simpleType>
  <xs:simpleType name="BadUnion"><xs:union memberTypes="t:InnerList xs:string"/></xs:simpleType>
  <xs:simpleType name="BadList"><xs:list itemType="t:BadUnion"/></xs:simpleType>
</xs:schema>`
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("discoverTestSchema accepted a non-atomic transitive list member")
			}
			if schema.storage != nil {
				t.Fatal("discoverTestSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaSimpleTypeBaseCode)
			}
			if diagnostic.Loc().IsZero() || len(diagnostic.Related()) < 2 {
				t.Fatalf("diagnostic locations = %v related %v, want primary and definition/variety locations", diagnostic.Loc(), diagnostic.Related())
			}
			if !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("diagnostic lost invalid-derivation cause: %v", err)
			}
		})
	}
}

func TestSchemaSimpleTypeVarietyRejectsNestedUnionMembersInStrict10(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="Inner"><xs:union memberTypes="xs:string"/></xs:simpleType>
  <xs:simpleType name="Outer"><xs:union memberTypes="t:Inner xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted a nested union member")
	}
	if schema.storage != nil {
		t.Fatal("Strict10 returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaSimpleTypeBaseCode)
	}
	if diagnostic.Loc().IsZero() || len(diagnostic.Related()) < 2 {
		t.Fatalf("diagnostic locations = %v related %v, want primary and definition/variety locations", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
		t.Fatalf("diagnostic lost invalid-derivation cause: %v", err)
	}
}

//nolint:gocognit // Keep named and anonymous nested-union identity coverage together.
func TestSchemaSimpleTypeVarietyPreservesNestedUnionIdentityByEdition(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="Inner"><xs:union memberTypes="xs:string xs:integer"/></xs:simpleType>
  <xs:simpleType name="Outer"><xs:union memberTypes="t:Inner xs:boolean"/></xs:simpleType>
  <xs:simpleType name="NamedUnionList"><xs:list itemType="t:Outer"/></xs:simpleType>
  <xs:simpleType name="AnonymousOuter"><xs:union><xs:simpleType><xs:union memberTypes="xs:string xs:integer"/></xs:simpleType><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:union></xs:simpleType>
  <xs:simpleType name="AnonymousUnionList"><xs:list itemType="t:AnonymousOuter"/></xs:simpleType>
</xs:schema>`
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			inner := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Inner"))
			outer := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Outer"))
			if len(inner) != 1 || len(outer) != 1 {
				t.Fatalf("nested union matches = %d/%d, want one each", len(inner), len(outer))
			}
			innerID := inner[0].ID()
			outerDefinition, ok := outer[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("Outer simple type view is missing")
			}
			outerMembers := outerDefinition.MemberTypes()
			if len(outerMembers) != 2 || outerMembers[0].Kind() != SimpleTypeReferenceNamed || outerMembers[0].Name().Local() != "Inner" || outerMembers[0].Variety() != SimpleTypeVarietyUnion {
				t.Fatalf("Outer member 0 = %#v, want named nested union", outerMembers)
			}
			if memberID, hasID := outerMembers[0].ComponentID(); !hasID || memberID != innerID {
				t.Fatalf("Outer nested union identity = %v/%t, want %v/true", memberID, hasID, innerID)
			}

			namedList := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "NamedUnionList"))
			if len(namedList) != 1 {
				t.Fatalf("NamedUnionList matches = %d, want 1", len(namedList))
			}
			namedListDefinition, ok := namedList[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("NamedUnionList simple type view is missing")
			}
			namedItem, ok := namedListDefinition.ItemType()
			if !ok || namedItem.Name().Local() != "Outer" || namedItem.Variety() != SimpleTypeVarietyUnion {
				t.Fatalf("NamedUnionList item = %q/%q/%t, want named union", namedItem.Name(), namedItem.Variety(), ok)
			}

			anonymousOuter := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "AnonymousOuter"))
			if len(anonymousOuter) != 1 {
				t.Fatalf("AnonymousOuter matches = %d, want 1", len(anonymousOuter))
			}
			anonymousDefinition, ok := anonymousOuter[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("AnonymousOuter simple type view is missing")
			}
			anonymousMembers := anonymousDefinition.MemberTypes()
			if len(anonymousMembers) != 2 || !anonymousMembers[0].IsAnonymous() || anonymousMembers[0].Variety() != SimpleTypeVarietyUnion {
				t.Fatalf("AnonymousOuter member 0 = %#v, want anonymous nested union", anonymousMembers)
			}
			anonymousUnion, ok := anonymousMembers[0].AnonymousType()
			if !ok || anonymousUnion.Variety() != SimpleTypeVarietyUnion || len(anonymousUnion.MemberTypes()) != 2 {
				t.Fatalf("AnonymousOuter nested model = %#v/%t, want union with two atomic members", anonymousUnion, ok)
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

//nolint:gocognit // Keep versioned built-in identity and bound assertions together.
func TestSchemaBuiltInReferencesRetainVersionedBoundsAndBooleanIdentity(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="Integer"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Decimal"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:simpleType name="Boolean"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}

			integer := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Integer"))
			decimal := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Decimal"))
			boolean := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Boolean"))
			if len(integer) != 1 || len(decimal) != 1 || len(boolean) != 1 {
				t.Fatalf("built-in base definitions = %d/%d/%d, want one each", len(integer), len(decimal), len(boolean))
			}

			integerDefinition, ok := integer[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("integer simple type view is missing")
			}
			if integerDefinition.IsString() {
				t.Fatal("integer simple type reported string identity")
			}
			integerReference, ok := integerDefinition.BaseReference()
			if !ok || integerReference.Kind() != SimpleTypeReferenceBuiltin || integerReference.Name().Local() != "integer" {
				t.Fatalf("integer base reference = %#v/%t, want built-in integer", integerReference, ok)
			}
			if _, hasID := integerReference.ComponentID(); hasID {
				t.Fatal("built-in integer reference unexpectedly has a component identity")
			}
			integerFacets, ok := integerReference.facts.facets.(schemaDigitFacetVariant)
			if !ok || integerFacets.integerBounds.Version() != test.version || len(integerFacets.integerBounds.Bounds()) != 0 {
				t.Fatalf("integer reference bounds = %#v/%t, want empty versioned bounds", integerFacets.integerBounds, ok)
			}

			decimalDefinition, ok := decimal[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("decimal simple type view is missing")
			}
			if decimalDefinition.IsString() {
				t.Fatal("decimal simple type reported string identity")
			}
			decimalReference, ok := decimalDefinition.BaseReference()
			if !ok || decimalReference.Kind() != SimpleTypeReferenceBuiltin || decimalReference.Name().Local() != "decimal" {
				t.Fatalf("decimal base reference = %#v/%t, want built-in decimal", decimalReference, ok)
			}
			if _, hasID := decimalReference.ComponentID(); hasID {
				t.Fatal("built-in decimal reference unexpectedly has a component identity")
			}
			decimalFacets, ok := decimalReference.facts.facets.(schemaDigitFacetVariant)
			if !ok || decimalFacets.decimalBounds.Version() != test.version || len(decimalFacets.decimalBounds.Bounds()) != 0 {
				t.Fatalf("decimal reference bounds = %#v/%t, want empty versioned bounds", decimalFacets.decimalBounds, ok)
			}

			booleanDefinition, ok := boolean[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("boolean simple type view is missing")
			}
			if booleanDefinition.IsString() {
				t.Fatal("boolean simple type reported string identity")
			}
			booleanReference, ok := booleanDefinition.BaseReference()
			if !ok || booleanReference.Kind() != SimpleTypeReferenceBuiltin || booleanReference.Name().Local() != "boolean" {
				t.Fatalf("boolean base reference = %#v/%t, want built-in boolean", booleanReference, ok)
			}
			if _, hasID := booleanReference.ComponentID(); hasID {
				t.Fatal("built-in boolean reference unexpectedly has a component identity")
			}
			if _, ok := booleanReference.facts.facets.(schemaBooleanFacetVariant); !ok {
				t.Fatalf("boolean reference facets = %T, want boolean variant", booleanReference.facts.facets)
			}
		})
	}
}

func TestSchemaNegativeIntegerBoundsPropagateThroughNamedAndAnonymousRestrictions(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="NamedNegative"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>
  <xs:simpleType name="DerivedNegative"><xs:restriction base="t:NamedNegative"/></xs:simpleType>
  <xs:simpleType name="AnonymousNegative"><xs:union><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}

	for _, name := range []string{"NamedNegative", "DerivedNegative"} {
		components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", name))
		if len(components) != 1 {
			t.Fatalf("%s matches = %d, want 1", name, len(components))
		}
		definition, ok := components[0].SimpleTypeDefinition()
		if !ok {
			t.Fatalf("%s simple type view is missing", name)
		}
		assertNegativeIntegerUpperBound(t, name, definition)
	}

	components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "AnonymousNegative"))
	if len(components) != 1 {
		t.Fatalf("AnonymousNegative matches = %d, want 1", len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("AnonymousNegative simple type view is missing")
	}
	members := definition.MemberTypes()
	if len(members) != 1 {
		t.Fatalf("AnonymousNegative member count = %d, want 1", len(members))
	}
	anonymous, ok := members[0].AnonymousType()
	if !ok {
		t.Fatal("AnonymousNegative member does not expose its anonymous type")
	}
	assertNegativeIntegerUpperBound(t, "AnonymousNegative member", anonymous)
}

func assertNegativeIntegerUpperBound(t *testing.T, name string, definition SimpleTypeDefinition) {
	t.Helper()
	bounds, present := definition.IntegerBounds()
	if !present {
		t.Fatalf("%s IntegerBounds() is absent", name)
	}
	maximum, present := bounds.MaxInclusive()
	if !present || maximum.Canonical() != "-1" {
		t.Fatalf("%s maxInclusive = %q/%t, want -1/true", name, maximum.Canonical(), present)
	}
}

//nolint:gocognit // Keep duplicate reference order and repeated-build checks together.
func TestSchemaSimpleTypeVarietyRetainsDuplicateReferencesDeterministically(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="Atomic"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="ListA"><xs:list itemType="t:Atomic"/></xs:simpleType>
  <xs:simpleType name="ListB"><xs:list itemType="t:Atomic"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="t:Atomic t:Atomic xs:string"/></xs:simpleType>
</xs:schema>`
	var first []Component
	for iteration := 0; iteration < 3; iteration++ {
		schema, err := discoverTestSchema(t, root, nil)
		if err != nil {
			t.Fatalf("discoverTestSchema iteration %d: %v", iteration, err)
		}
		components := schema.Components()
		if iteration == 0 {
			first = components
		}
		if iteration > 0 && !reflect.DeepEqual(first, components) {
			t.Fatalf("repeated schema build %d changed component facts", iteration)
		}

		atomic, ok := components[0].SimpleTypeDefinition()
		if !ok {
			t.Fatal("atomic simple type view is missing")
		}
		atomicID := atomic.ID()
		for _, name := range []string{"ListA", "ListB"} {
			definition, definitionOK := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", name))[0].SimpleTypeDefinition()
			if !definitionOK {
				t.Fatalf("%s simple type view is missing", name)
			}
			item, itemOK := definition.ItemType()
			if !itemOK || item.Kind() != SimpleTypeReferenceNamed || item.Name().Local() != "Atomic" {
				t.Fatalf("%s item reference = %#v/%t, want named Atomic", name, item, itemOK)
			}
			if itemID, hasID := item.ComponentID(); !hasID || itemID != atomicID {
				t.Fatalf("%s item ID = %v/%t, want %v/true", name, itemID, hasID, atomicID)
			}
		}

		union, ok := components[3].SimpleTypeDefinition()
		if !ok {
			t.Fatal("union simple type view is missing")
		}
		members := union.MemberTypes()
		if len(members) != 3 {
			t.Fatalf("union member count = %d, want 3", len(members))
		}
		for index := 0; index < 2; index++ {
			if members[index].Kind() != SimpleTypeReferenceNamed || members[index].Name().Local() != "Atomic" {
				t.Fatalf("union member %d = %q/%q, want named Atomic", index, members[index].Kind(), members[index].Name())
			}
			memberID, hasID := members[index].ComponentID()
			if !hasID || memberID != atomicID {
				t.Fatalf("union member %d ID = %v/%t, want %v/true", index, memberID, hasID, atomicID)
			}
		}
		if members[2].Kind() != SimpleTypeReferenceBuiltin || members[2].Name().Local() != "string" {
			t.Fatalf("union member 2 = %q/%q, want built-in string", members[2].Kind(), members[2].Name())
		}
	}
}

func TestSchemaInlinePrecisionDecimalPolicyBoundary(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="inline"><xs:simpleType><xs:restriction base="xs:precisionDecimal"/></xs:simpleType></xs:element>
</xs:schema>`
	strict10, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted inline precisionDecimal")
	}
	if strict10.storage != nil {
		t.Fatal("Strict10 returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode {
		t.Fatalf("Strict10 diagnostic = %s, want precisionDecimal policy mismatch", diagnostic)
	}
	if !errors.Is(err, errSchemaPrecisionDecimalVersion) {
		t.Fatalf("Strict10 diagnostic lost precisionDecimal cause: %v", err)
	}

	strict11, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("Strict11 rejected inline precisionDecimal: %v", err)
	}
	elements := strict11.Components()
	if len(elements) != 1 {
		t.Fatalf("Strict11 component count = %d, want 1", len(elements))
	}
	declaration, ok := elements[0].ElementDeclaration()
	if !ok {
		t.Fatal("Strict11 global element view is missing")
	}
	reference, ok := declaration.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("Strict11 type reference = %#v/%t, want anonymous", reference, ok)
	}
	anonymous, ok := reference.AnonymousType()
	if !ok || !anonymous.HasPrecisionDecimalFacets() {
		t.Fatalf("Strict11 anonymous type = %#v/%t, want precisionDecimal facets", anonymous, ok)
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

//nolint:gocognit // Keep the zero-value API contract assertions together.
func TestSimpleTypeModelZeroViewsRemainSafe(t *testing.T) {
	var id SimpleTypeID
	if id.Source() != "" || id.Ordinal() != 0 || !id.IsZero() {
		t.Fatalf("zero simple type ID facts = %q/%d/%t", id.Source(), id.Ordinal(), id.IsZero())
	}

	var reference SimpleTypeReference
	if reference.Kind() != "" || !reference.Name().IsZero() || !reference.QName().IsZero() || !reference.Loc().IsZero() || reference.Variety() != "" || !reference.VarietyLoc().IsZero() {
		t.Fatal("zero simple type reference facts are not empty")
	}
	if componentID, ok := reference.ComponentID(); ok || !componentID.IsZero() {
		t.Fatalf("zero reference component ID = %v/%t", componentID, ok)
	}
	if componentID, ok := reference.ID(); ok || !componentID.IsZero() {
		t.Fatalf("zero reference ID alias = %v/%t", componentID, ok)
	}
	if anonymousID, ok := reference.AnonymousID(); ok || !anonymousID.IsZero() {
		t.Fatalf("zero reference anonymous ID = %v/%t", anonymousID, ok)
	}
	if _, ok := reference.AnonymousType(); ok || reference.IsBuiltin() || reference.IsNamed() || reference.IsAnonymous() {
		t.Fatal("zero reference kind predicates are not empty")
	}

	var declaration ElementDeclaration
	if !declaration.DeclaredType().IsZero() {
		t.Fatal("zero element declaration has a declared type")
	}
	if componentID, ok := declaration.TypeID(); ok || !componentID.IsZero() {
		t.Fatalf("zero element type ID = %v/%t", componentID, ok)
	}
	if _, ok := declaration.TypeReference(); ok {
		t.Fatal("zero element declaration has a type reference")
	}
	if _, ok := declaration.InlineSimpleType(); ok {
		t.Fatal("zero element declaration has an inline type")
	}

	var definition SimpleTypeDefinition
	if definition.ID() != (ComponentID{}) || !definition.Name().IsZero() || !definition.Loc().IsZero() || definition.IsAnonymous() || definition.Variety() != "" || !definition.VarietyLoc().IsZero() || !definition.Base().IsZero() || !definition.BaseLoc().IsZero() {
		t.Fatal("zero simple type definition facts are not empty")
	}
	if nodeID, ok := definition.NodeID(); ok || !nodeID.IsZero() {
		t.Fatalf("zero definition node ID = %v/%t", nodeID, ok)
	}
	if componentID, ok := definition.BaseID(); ok || !componentID.IsZero() {
		t.Fatalf("zero definition base ID = %v/%t", componentID, ok)
	}
	if _, ok := definition.BaseReference(); ok {
		t.Fatal("zero definition has a base reference")
	}
	if _, ok := definition.ItemType(); ok {
		t.Fatal("zero definition has a list item")
	}
	if _, ok := definition.ListItemType(); ok {
		t.Fatal("zero definition has a list item alias")
	}
	if definition.MemberTypes() != nil || definition.UnionMemberTypes() != nil || definition.DigitFacets().Kind() != "" || definition.HasPrecisionDecimalFacets() {
		t.Fatal("zero definition has non-empty variety facts")
	}
	if definition.IsString() {
		t.Fatal("zero definition reported string identity")
	}
	if totalDigits, ok := definition.PrecisionDecimalFacets().TotalDigits(); ok || !totalDigits.IsZero() {
		t.Fatal("zero definition has precisionDecimal facets")
	}
}
