package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const (
	unsignedLongMinimum = "0"
	unsignedLongMaximum = "18446744073709551615"
)

type unsignedLongPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func unsignedLongPolicyProfiles() []unsignedLongPolicyProfile {
	return []unsignedLongPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
}

func TestSchemaUnsignedLongReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaUnsignedLongReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated unsignedLong builds changed component facts or order")
			}
			assertUnsignedLongReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaUnsignedLongReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="xs:unsignedLong"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:element name="anonymous"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:element>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="0"/><xs:maxInclusive value="18446744073709551615"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:unsignedLong"><xs:enumeration value="0"/><xs:enumeration value="18446744073709551615"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:unsignedLong"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:unsignedLong t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

func assertUnsignedLongReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertUnsignedLongDirectReference(t, schema, root, version)
	assertUnsignedLongNamedReferences(t, schema, root, version)
	assertUnsignedLongAnonymousElement(t, schema, root, version)
	assertUnsignedLongCollectionReferences(t, schema, root, version)
}

func assertUnsignedLongDirectReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireUnsignedLongElement(t, schema, "direct", "urn:test")
	reference, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct element type reference is missing")
	}
	assertUnsignedLongBuiltinReference(t, reference, elementReferenceTestAttributeLoc(t, root, `type="xs:unsignedLong"`), version)
	if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("direct element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "unsignedLong")); len(matches) != 0 {
		t.Fatalf("built-in unsignedLong is visible as %d named component(s)", len(matches))
	}
}

func assertUnsignedLongNamedReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	forward := requireUnsignedLongElement(t, schema, "forward", "urn:test")
	forwardReference, ok := forward.TypeReference()
	if !ok || !forwardReference.IsNamed() || forwardReference.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("forward element type reference = %#v/%t, want named Later", forwardReference, ok)
	}
	later := requireUnsignedLongDefinition(t, schema, "Later")
	forwardID, forwardIDOK := forwardReference.ComponentID()
	if !forwardIDOK || forwardID != later.ID() {
		t.Fatalf("forward type ID = %v/%t, want Later %v/true", forwardID, forwardIDOK, later.ID())
	}
	assertUnsignedLongReferenceFacts(t, forwardReference.facts, version, unsignedLongMaximum)
	if forwardReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:Later"`) {
		t.Fatalf("forward reference location = %s, want type attribute location", forwardReference.Loc())
	}

	base, ok := later.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "unsignedLong") {
		t.Fatalf("Later base reference = %#v/%t, want built-in unsignedLong", base, ok)
	}
	if base.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 7, `base="xs:unsignedLong"`) || base.VarietyLoc() != base.Loc() {
		t.Fatalf("Later base locations = %s/%s, want the base attribute location", base.Loc(), base.VarietyLoc())
	}
	assertUnsignedLongBuiltinReference(t, base, base.Loc(), version)

	derived := requireUnsignedLongDefinition(t, schema, "Derived")
	assertUnsignedLongDefinition(t, derived, version, unsignedLongMaximum)
	derivedBase, ok := derived.BaseReference()
	if !ok || !derivedBase.IsNamed() || derivedBase.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Derived base reference = %#v/%t, want named Later", derivedBase, ok)
	}

	tight := requireUnsignedLongDefinition(t, schema, "Tight")
	assertUnsignedLongDefinition(t, tight, version, "2")

	enumerated := requireUnsignedLongDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 2 || values[0].Canonical() != unsignedLongMinimum || values[1].Canonical() != unsignedLongMaximum {
		t.Fatalf("Enumerated values = %#v, want exact unsignedLong boundaries", values)
	}
	if locations := enumerated.IntegerEnumerationFacets().Locations(); len(locations) != 2 || locations[0].IsZero() || locations[1].IsZero() {
		t.Fatalf("Enumerated locations = %#v, want two source locations", locations)
	}
}

func assertUnsignedLongAnonymousElement(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	anonymous := requireUnsignedLongElement(t, schema, "anonymous", "urn:test")
	reference, ok := anonymous.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("anonymous element type reference = %#v/%t, want anonymous", reference, ok)
	}
	if reference.Loc().IsZero() {
		t.Fatal("anonymous type reference location is zero")
	}
	if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("anonymous type reference component ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	definition, ok := reference.AnonymousType()
	if !ok {
		t.Fatal("anonymous element type definition is missing")
	}
	assertUnsignedLongDefinition(t, definition, version, unsignedLongMaximum)
	base, ok := definition.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "unsignedLong") {
		t.Fatalf("anonymous base reference = %#v/%t, want built-in unsignedLong", base, ok)
	}
	if base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:unsignedLong"`) || base.VarietyLoc() != base.Loc() {
		t.Fatalf("anonymous base locations = %s/%s, want base attribute location", base.Loc(), base.VarietyLoc())
	}
}

func assertUnsignedLongCollectionReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	list := requireUnsignedLongDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item type is missing")
	}
	assertUnsignedLongBuiltinReference(t, item, elementReferenceTestAttributeLoc(t, root, `itemType="xs:unsignedLong"`), version)

	union := requireUnsignedLongDefinition(t, schema, "Union")
	if union.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("Union variety = %q, want union", union.Variety())
	}
	members := union.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("Union member count = %d, want 2", len(members))
	}
	assertUnsignedLongBuiltinReference(t, members[0], elementReferenceTestAttributeLoc(t, root, `memberTypes="xs:unsignedLong`), version)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Union member 1 = %#v, want named Later", members[1])
	}
	assertUnsignedLongReferenceFacts(t, members[1].facts, version, unsignedLongMaximum)

	inlineList := requireUnsignedLongDefinition(t, schema, "InlineList")
	inlineItem, ok := inlineList.ItemType()
	if !ok || !inlineItem.IsAnonymous() {
		t.Fatalf("InlineList item = %#v/%t, want anonymous restriction", inlineItem, ok)
	}
	inlineItemDefinition, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineList item anonymous definition is missing")
	}
	assertUnsignedLongDefinition(t, inlineItemDefinition, version, unsignedLongMaximum)

	inlineUnion := requireUnsignedLongDefinition(t, schema, "InlineUnion")
	inlineMembers := inlineUnion.MemberTypes()
	if len(inlineMembers) != 1 || !inlineMembers[0].IsAnonymous() {
		t.Fatalf("InlineUnion members = %#v, want one anonymous restriction", inlineMembers)
	}
	inlineMemberDefinition, ok := inlineMembers[0].AnonymousType()
	if !ok {
		t.Fatal("InlineUnion anonymous member definition is missing")
	}
	assertUnsignedLongDefinition(t, inlineMemberDefinition, version, unsignedLongMaximum)
}

func requireUnsignedLongElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
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

func requireUnsignedLongDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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

//nolint:gocognit // Keep exact bounds, source provenance, and copy checks together.
func assertUnsignedLongBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	wantName := mustTestQName(t, testXSDNamespace, "unsignedLong")
	if !reference.IsBuiltin() || reference.Name() != wantName || reference.QName() != wantName {
		t.Fatalf("reference = %#v, want built-in xs:unsignedLong", reference)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc {
		t.Fatalf("reference locations = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), wantLoc)
	}
	if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicUnsignedLong {
		t.Fatalf("reference variety/category = %q/%v, want atomic unsignedLong", reference.Variety(), reference.facts)
	}
	if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("built-in reference component ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	facets, ok := reference.facts.facets.(schemaDigitFacetVariant)
	if !ok {
		t.Fatalf("built-in reference facets = %T, want fresh digit facts", reference.facts.facets)
	}
	if facets.value.Kind() != DigitDatatypeInteger || facets.value.Version() != version {
		t.Fatalf("built-in digit facts = %q/%q, want integer/%q", facets.value.Kind(), facets.value.Version(), version)
	}
	fraction, present := facets.value.FractionDigits()
	if !present || fraction.Canonical() != "0" {
		t.Fatalf("built-in fractionDigits = %q/%t, want 0/true", fraction.Canonical(), present)
	}
	fractionFixed, present := facets.value.FractionDigitsFixed()
	if !present || !fractionFixed {
		t.Fatalf("built-in fractionDigits fixed = %t/%t, want true/true", fractionFixed, present)
	}
	if _, hasTotalDigits := facets.value.TotalDigits(); hasTotalDigits {
		t.Fatal("built-in unsignedLong unexpectedly has totalDigits")
	}
	assertIntegerBounds(t, facets.integerBounds, version, unsignedLongMinimum, unsignedLongMaximum)
	for _, bound := range facets.integerBounds.Bounds() {
		if !bound.Loc().IsZero() {
			t.Fatalf("built-in unsignedLong bound %s has source location %s", bound.Kind(), bound.Loc())
		}
	}

	minimum, present := facets.integerBounds.MinInclusive()
	if !present {
		t.Fatal("built-in reference has no effective minInclusive")
	}
	_ = minimum.value.SetInt64(1)
	maximum, present := facets.integerBounds.MaxInclusive()
	if !present {
		t.Fatal("built-in reference has no effective maxInclusive")
	}
	_ = maximum.value.SetInt64(0)
	assertIntegerBounds(t, facets.integerBounds, version, unsignedLongMinimum, unsignedLongMaximum)
}

func assertUnsignedLongReferenceFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, wantMaximum string) {
	t.Helper()
	assertIntegerReferenceFacts(t, facts, version, schemaSimpleTypeAtomicUnsignedLong, "unsignedLong", unsignedLongMinimum, wantMaximum)
}

func assertUnsignedLongDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMaximum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicUnsignedLong {
		t.Fatalf("definition variety/category = %q/%v, want atomic unsignedLong", definition.Variety(), definition.facts)
	}
	assertUnsignedLongReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, wantMaximum)
}

//nolint:gocognit // Keep repeated graph discovery and ordered reference checks together.
func TestSchemaUnsignedLongComposesGraphs(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := unsignedLongGraphFixtures(profile.version)
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
			for _, test := range unsignedLongGraphElementCases() {
				assertUnsignedLongGraphElement(t, first, root, fixtures, test, profile.version)
			}
		})
	}
}

func unsignedLongGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="xs:unsignedLong"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="xs:unsignedLong"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="chameleon" type="xs:unsignedLong"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="xs:unsignedLong"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func unsignedLongGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="xs:unsignedLong"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="xs:unsignedLong"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="xs:unsignedLong"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="xs:unsignedLong"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertUnsignedLongGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requireUnsignedLongElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		assertUnsignedLongReferenceFacts(t, reference.facts, version, unsignedLongMaximum)
		return
	}
	assertUnsignedLongBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
}

func TestSchemaUnsignedLongRejectsInvalidReferencesAndRestrictions(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct {
			name  string
			root  string
			cause error
		}{
			{
				name:  "below minimum",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="-1"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundRestriction,
			},
			{
				name:  "above maximum",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="18446744073709551616"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundRestriction,
			},
			{
				name:  "negative enumeration",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:enumeration value="-1"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidEnumerationRestriction,
			},
			{
				name:  "above maximum enumeration",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:enumeration value="18446744073709551616"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidEnumerationRestriction,
			},
			{
				name:  "malformed bound",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType></xs:schema>`,
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
				name:  "local ref remains element symbol space",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Root"><xs:sequence><xs:element ref="xs:unsignedLong"/></xs:sequence></xs:complexType></xs:schema>`,
				cause: errSchemaElementReferenceUnresolved,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				assertUnsignedLongInvalidNoPartialSchema(t, schema, err, test.cause, profile.version)
			})
		}
	}
}

func assertUnsignedLongInvalidNoPartialSchema(t *testing.T, schema Schema, err error, cause error, version XSDVersion) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located error and no partial schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() || diagnostic.SpecRef() == "" {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic with a specification reference", diagnostic)
	}
	if !strings.HasPrefix(diagnostic.SpecRef(), versionedSpecPrefix(version)) {
		t.Fatalf("diagnostic SpecRef() = %q, want %s prefix", diagnostic.SpecRef(), versionedSpecPrefix(version))
	}
	if !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func versionedSpecPrefix(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-"
	}
	return "xsd11-"
}

func TestSchemaUnsignedLongExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertSchemaIntegerDerivedExcludedShapes(t, profile.policy, "unsignedLong", "0")
			assertSchemaIntegerDerivedGlobalAttributeExcluded(t, profile.policy, "unsignedLong")
		})
	}
}

func TestSchemaUnsignedLongConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="xs:unsignedLong"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}

			assertIntegerDerivedConsumersUnsupported(t, schema)
		})
	}
}

func TestSchemaUnsignedLongDoesNotAdmitNarrowerBuiltins(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="xs:unsignedInt"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverTestSchemaWithPolicy admitted an out-of-scope narrower unsigned builtin")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic = %s, want located unsupported diagnostic", diagnostic)
			}
		})
	}
}
