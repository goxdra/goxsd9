package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func TestSchemaIntReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaIntReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated int builds changed component facts or order")
			}
			assertIntReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaIntReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="p:int"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="narrowed" type="t:Tight"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:element name="inline"><xs:simpleType><xs:restriction base="p:int"/></xs:simpleType></xs:element>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="p:int"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="p:int"><xs:enumeration value="-2147483648"/><xs:enumeration value="2147483647"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="p:int"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="p:int t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="p:int"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="p:int"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

func assertIntReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertIntDirectReference(t, schema, root, version)
	assertIntNamedReferences(t, schema, root, version)
	assertIntCollectionReferences(t, schema, root, version)
	assertIntInlineReferences(t, schema, version)
}

func assertIntDirectReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireIntElement(t, schema, "direct", "urn:test")
	reference, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct element type reference is missing")
	}
	assertIntBuiltinReference(t, reference, elementReferenceTestAttributeLoc(t, root, `type="p:int"`), version)
	if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("direct element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "int")); len(matches) != 0 {
		t.Fatalf("built-in int is visible as %d named component(s)", len(matches))
	}
}

func assertIntNamedReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	forward := requireIntElement(t, schema, "forward", "urn:test")
	forwardReference, ok := forward.TypeReference()
	if !ok || !forwardReference.IsNamed() || forwardReference.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("forward element type reference = %#v/%t, want named Later", forwardReference, ok)
	}
	later := requireIntDefinition(t, schema, "Later")
	forwardID, forwardIDOK := forwardReference.ComponentID()
	if !forwardIDOK || forwardID != later.ID() {
		t.Fatalf("forward type ID = %v/%t, want Later %v/true", forwardID, forwardIDOK, later.ID())
	}
	assertIntegerReferenceFacts(t, forwardReference.facts, version, schemaSimpleTypeAtomicInt, "int", "-100", "100")
	if forwardReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:Later"`) {
		t.Fatalf("forward reference location = %s, want type attribute location", forwardReference.Loc())
	}

	base, ok := later.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "int") {
		t.Fatalf("Later base reference = %#v/%t, want built-in int", base, ok)
	}
	wantBaseLoc := mustSchemaTokenLoc(t, "root.xsd", root, 8, `base="p:int"`)
	if base.Loc() != wantBaseLoc || base.VarietyLoc() != base.Loc() {
		t.Fatalf("Later base locations = %s/%s, want the base attribute location", base.Loc(), base.VarietyLoc())
	}
	assertIntBuiltinReference(t, base, base.Loc(), version)

	derived := requireIntDefinition(t, schema, "Derived")
	assertIntDefinition(t, derived, version, "-100", "100")
	derivedBase, ok := derived.BaseReference()
	if !ok || !derivedBase.IsNamed() || derivedBase.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Derived base reference = %#v/%t, want named Later", derivedBase, ok)
	}

	tight := requireIntDefinition(t, schema, "Tight")
	assertIntDefinition(t, tight, version, "-100", "2")

	enumerated := requireIntDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 2 || values[0].Canonical() != "-2147483648" || values[1].Canonical() != "2147483647" {
		t.Fatalf("Enumerated values = %#v, want exact signed 32-bit boundaries", values)
	}
	if locations := enumerated.IntegerEnumerationFacets().Locations(); len(locations) != 2 || locations[0].IsZero() || locations[1].IsZero() {
		t.Fatalf("Enumerated locations = %#v, want two source locations", locations)
	}
}

func assertIntCollectionReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	list := requireIntDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item type is missing")
	}
	assertIntBuiltinReference(t, item, elementReferenceTestAttributeLoc(t, root, `itemType="p:int"`), version)

	union := requireIntDefinition(t, schema, "Union")
	if union.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("Union variety = %q, want union", union.Variety())
	}
	members := union.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("Union member count = %d, want 2", len(members))
	}
	assertIntBuiltinReference(t, members[0], elementReferenceTestAttributeLoc(t, root, `memberTypes="p:int`), version)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Union member 1 = %#v, want named Later", members[1])
	}
	assertIntegerReferenceFacts(t, members[1].facts, version, schemaSimpleTypeAtomicInt, "int", "-100", "100")
}

//nolint:gocognit // Keep anonymous ownership and collection checks together.
func assertIntInlineReferences(t *testing.T, schema Schema, version XSDVersion) {
	inline := requireIntElement(t, schema, "inline", "urn:test")
	if inline.DeclaredType() != (QName{}) {
		t.Fatalf("inline declared type = %q, want zero QName", inline.DeclaredType())
	}
	if typeID, hasTypeID := inline.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("inline element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	inlineReference, ok := inline.TypeReference()
	if !ok || !inlineReference.IsAnonymous() {
		t.Fatalf("inline type reference = %#v/%t, want anonymous", inlineReference, ok)
	}
	if typeID, hasTypeID := inlineReference.ComponentID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("inline reference component ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if anonymousID, hasAnonymousID := inlineReference.AnonymousID(); !hasAnonymousID || anonymousID.IsZero() {
		t.Fatalf("inline reference anonymous ID = %v/%t, want nonzero model identity", anonymousID, hasAnonymousID)
	}
	definition, ok := inlineReference.AnonymousType()
	if !ok {
		t.Fatal("inline anonymous definition is missing")
	}
	assertIntDefinition(t, definition, version, "-2147483648", "2147483647")
	base, ok := definition.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "int") || base.Loc().IsZero() {
		t.Fatalf("inline anonymous base = %#v/%t, want located built-in int", base, ok)
	}

	inlineList := requireIntDefinition(t, schema, "InlineList")
	inlineItem, ok := inlineList.ItemType()
	if !ok || !inlineItem.IsAnonymous() {
		t.Fatalf("InlineList item = %#v/%t, want anonymous restriction", inlineItem, ok)
	}
	inlineItemDefinition, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineList item anonymous definition is missing")
	}
	assertIntDefinition(t, inlineItemDefinition, version, "-2147483648", "2147483647")

	inlineUnion := requireIntDefinition(t, schema, "InlineUnion")
	inlineMembers := inlineUnion.MemberTypes()
	if len(inlineMembers) != 1 || !inlineMembers[0].IsAnonymous() {
		t.Fatalf("InlineUnion members = %#v, want one anonymous restriction", inlineMembers)
	}
	inlineMemberDefinition, ok := inlineMembers[0].AnonymousType()
	if !ok {
		t.Fatal("InlineUnion anonymous member definition is missing")
	}
	assertIntDefinition(t, inlineMemberDefinition, version, "-2147483648", "2147483647")
}

func requireIntElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
	t.Helper()
	matches := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, namespace, local))
	if len(matches) != 1 {
		t.Fatalf("element %s:%s matches = %d, want 1", namespace, local, len(matches))
	}
	declaration, ok := matches[0].ElementDeclaration()
	if !ok {
		t.Fatalf("element %s:%s has no declaration view", namespace, local)
	}
	return declaration
}

func requireIntDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	t.Helper()
	matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", local))
	if len(matches) != 1 {
		t.Fatalf("simple type %q matches = %d, want 1", local, len(matches))
	}
	definition, ok := matches[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("simple type %q has no definition view", local)
	}
	return definition
}

func assertIntBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	assertIntegerBuiltinReference(t, reference, wantLoc, version, schemaSimpleTypeAtomicInt, "int", "-2147483648", "2147483647")
}

func assertIntDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMinimum, wantMaximum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicInt {
		t.Fatalf("definition variety/category = %q/%v, want atomic int", definition.Variety(), definition.facts)
	}
	assertIntegerReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, schemaSimpleTypeAtomicInt, "int", wantMinimum, wantMaximum)
}

//nolint:gocognit // Keep repeated graph discovery and ordered reference checks together.
func TestSchemaIntComposesGraphs(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := intGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated graph builds changed component facts or order")
			}
			if got := len(first.Documents()); got != 4 {
				t.Fatalf("document count = %d, want 4 after repeated/cyclic graph discovery", got)
			}
			for _, test := range intGraphElementCases() {
				assertIntGraphElement(t, first, root, fixtures, test, profile.version)
			}
		})
	}
}

func intGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="p:int"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="p:int"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="p:int"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `"><xs:element name="chameleon" type="p:int"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="p:int"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="p:int"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func intGraphElementCases() []struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
} {
	return []struct {
		local     string
		namespace string
		source    SourceID
		needle    string
		named     bool
	}{
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="p:int"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="p:int"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="p:int"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="p:int"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertIntGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requireIntElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		assertIntegerReferenceFacts(t, reference.facts, version, schemaSimpleTypeAtomicInt, "int", "-2147483648", "2147483647")
		return
	}
	assertIntBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
}

func TestSchemaIntRejectsInvalidReferencesAndRestrictions(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name  string
			root  string
			cause error
		}{
			{
				name:  "below minimum",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:minInclusive value="-2147483649"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundRestriction,
			},
			{
				name:  "above maximum",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:maxInclusive value="2147483648"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundRestriction,
			},
			{
				name:  "out of range enumeration",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:enumeration value="2147483648"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidEnumerationRestriction,
			},
			{
				name:  "malformed bound",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundValue,
			},
			{
				name:  "unresolved base",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="Bad"><xs:restriction base="t:Missing"/></xs:simpleType></xs:schema>`,
				cause: errSchemaSimpleTypeBaseUnresolved,
			},
			{
				name:  "wrong kind base",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:element name="NotAType" type="xs:integer"/><xs:simpleType name="Bad"><xs:restriction base="t:NotAType"/></xs:simpleType></xs:schema>`,
				cause: errSchemaSimpleTypeBaseWrongKind,
			},
			{
				name: "malformed QName",
				root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:element name="bad" type="t:bad:q"/></xs:schema>`,
			},
			{
				name:  "local ref remains element symbol space",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Root"><xs:sequence><xs:element ref="p:int"/></xs:sequence></xs:complexType></xs:schema>`,
				cause: errSchemaElementReferenceUnresolved,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				assertIntInvalidNoPartialSchema(t, schema, err, test.cause)
			})
		}
	}
}

func assertIntInvalidNoPartialSchema(t *testing.T, schema Schema, err error, cause error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located error and no partial schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func TestSchemaIntExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertSchemaIntegerDerivedExcludedShapes(t, profile.policy, "int", "0")
			assertSchemaIntegerDerivedGlobalAttributeExcluded(t, profile.policy, "int")
		})
	}
}

func TestSchemaIntConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="p:int"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertIntegerDerivedConsumersUnsupported(t, schema)
		})
	}
}
