package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type positiveIntegerPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func positiveIntegerPolicyProfiles() []positiveIntegerPolicyProfile {
	return []positiveIntegerPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
}

func TestSchemaPositiveIntegerReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaPositiveIntegerReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated positiveInteger builds changed component facts or order")
			}
			assertPositiveIntegerReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaPositiveIntegerReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="xs:positiveInteger"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="narrowed" type="t:Tight"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:positiveInteger"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Exclusive"><xs:restriction base="xs:positiveInteger"><xs:minExclusive value="1"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:positiveInteger"><xs:enumeration value="+0001"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:positiveInteger"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:positiveInteger t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

func assertPositiveIntegerReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertPositiveIntegerDirectReference(t, schema, root, version)
	assertPositiveIntegerNamedReferenceShapes(t, schema, root, version)
	assertPositiveIntegerCollectionShapes(t, schema, root, version)
	assertPositiveIntegerInlineShapes(t, schema, version)
}

func assertPositiveIntegerDirectReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requirePositiveIntegerElement(t, schema, "direct", "urn:test")
	directReference, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct element type reference is missing")
	}
	assertPositiveIntegerBuiltinReference(
		t,
		directReference,
		elementReferenceTestAttributeLoc(t, root, `type="xs:positiveInteger"`),
		version,
	)
	if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("direct element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "positiveInteger")); len(matches) != 0 {
		t.Fatalf("built-in positiveInteger is visible as %d named component(s)", len(matches))
	}
}

func assertPositiveIntegerNamedReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertPositiveIntegerForwardReference(t, schema, root, version)
	assertPositiveIntegerTighteningAndEnumeration(t, schema, root, version)
}

func assertPositiveIntegerForwardReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	forward := requirePositiveIntegerElement(t, schema, "forward", "urn:test")
	forwardReference, ok := forward.TypeReference()
	if !ok || !forwardReference.IsNamed() || forwardReference.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("forward element type reference = %#v/%t, want named Later", forwardReference, ok)
	}
	later := requirePositiveIntegerDefinition(t, schema, "Later")
	forwardID, forwardIDOK := forwardReference.ComponentID()
	if !forwardIDOK || forwardID != later.ID() {
		t.Fatalf("forward type ID = %v/%t, want Later %v/true", forwardID, forwardIDOK, later.ID())
	}
	assertPositiveIntegerReferenceFacts(t, forwardReference.facts, version, "1")
	if forwardReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:Later"`) {
		t.Fatalf("forward reference location = %s, want type attribute location", forwardReference.Loc())
	}

	base, ok := later.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name().Local() != "positiveInteger" {
		t.Fatalf("Later base reference = %#v/%t, want built-in positiveInteger", base, ok)
	}
	if base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:positiveInteger"`) || base.VarietyLoc() != base.Loc() {
		t.Fatalf("Later base locations = %s/%s, want the base attribute location", base.Loc(), base.VarietyLoc())
	}
	assertPositiveIntegerReferenceFacts(t, base.facts, version, "1")
	digits := later.DigitFacets()
	if digits.Kind() != DigitDatatypeInteger || digits.Version() != version {
		t.Fatalf("Later digit facts = %q/%q, want integer/%q", digits.Kind(), digits.Version(), version)
	}
	total, present := digits.TotalDigits()
	if !present || total.Canonical() != "4" {
		t.Fatalf("Later totalDigits = %q/%t, want 4/true", total.Canonical(), present)
	}
	if loc, ok := digits.TotalDigitsLoc(); !ok || loc != elementReferenceTestAttributeLoc(t, root, `value="4"`) {
		t.Fatalf("Later totalDigits location = %s/%t, want source facet location", loc, ok)
	}
}

func assertPositiveIntegerTighteningAndEnumeration(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	tight := requirePositiveIntegerDefinition(t, schema, "Tight")
	assertPositiveIntegerDefinition(t, tight, version, "2")
	tightBase, ok := tight.BaseReference()
	if !ok || !tightBase.IsNamed() || tightBase.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Tight base reference = %#v/%t, want named Later", tightBase, ok)
	}
	if tightBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 8, `base="t:Later"`) {
		t.Fatalf("Tight base location = %s, want base attribute location", tightBase.Loc())
	}
	exclusive := requirePositiveIntegerDefinition(t, schema, "Exclusive")
	exclusiveBounds, hasExclusiveBounds := exclusive.IntegerBounds()
	if !hasExclusiveBounds {
		t.Fatal("Exclusive has no public integer bounds")
	}
	exclusiveFacet, hasExclusiveFacet := exclusiveBounds.MinExclusiveFacet()
	if !hasExclusiveFacet || exclusiveFacet.Value().Canonical() != "1" || exclusiveFacet.Loc() != elementReferenceTestAttributeLoc(t, root, `value="1"`) || exclusiveFacet.Version() != version {
		t.Fatalf("exclusive bound = %#v/%t, want minExclusive=1 at its facet location", exclusiveFacet, hasExclusiveFacet)
	}

	enumerated := requirePositiveIntegerDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 1 || values[0].Canonical() != "1" {
		t.Fatalf("Enumerated values = %#v, want canonical one from +0001", values)
	}
	if locations := enumerated.IntegerEnumerationFacets().Locations(); len(locations) != 1 || locations[0] != elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="+0001"`) {
		t.Fatalf("Enumerated locations = %#v, want the enumeration facet source", locations)
	}
	_ = values[0].value.SetInt64(99)
	if got := enumerated.IntegerEnumerationFacets().Values()[0].Canonical(); got != "1" {
		t.Fatalf("mutating returned enumeration changed stored value to %q", got)
	}
}

func assertPositiveIntegerCollectionShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	list := requirePositiveIntegerDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item type is missing")
	}
	assertPositiveIntegerBuiltinReference(
		t,
		item,
		elementReferenceTestAttributeLoc(t, root, `itemType="xs:positiveInteger"`),
		version,
	)
	if list.Variety() != SimpleTypeVarietyList || item.Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("list/item varieties = %q/%q, want list/atomic", list.Variety(), item.Variety())
	}

	union := requirePositiveIntegerDefinition(t, schema, "Union")
	if union.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("Union variety = %q, want union", union.Variety())
	}
	members := union.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("Union member count = %d, want 2", len(members))
	}
	assertPositiveIntegerBuiltinReference(
		t,
		members[0],
		elementReferenceTestAttributeLoc(t, root, `memberTypes="xs:positiveInteger`),
		version,
	)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Union member 1 = %#v, want named Later", members[1])
	}
	if id, ok := members[1].ComponentID(); !ok || id != requirePositiveIntegerDefinition(t, schema, "Later").ID() {
		t.Fatalf("Union member 1 identity = %v/%t, want Later", id, ok)
	}
}

func assertPositiveIntegerInlineShapes(t *testing.T, schema Schema, version XSDVersion) {
	t.Helper()
	inlineList := requirePositiveIntegerDefinition(t, schema, "InlineList")
	inlineItem, ok := inlineList.ItemType()
	if !ok || !inlineItem.IsAnonymous() {
		t.Fatalf("InlineList item = %#v/%t, want anonymous restriction", inlineItem, ok)
	}
	if nodeID, hasNodeID := inlineItem.AnonymousID(); !hasNodeID || nodeID.IsZero() {
		t.Fatalf("InlineList anonymous identity = %v/%t, want nonzero", nodeID, hasNodeID)
	}
	inlineItemDefinition, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineList item anonymous definition is missing")
	}
	assertPositiveIntegerDefinition(t, inlineItemDefinition, version, "1")
	inlineBase, ok := inlineItemDefinition.BaseReference()
	if !ok || !inlineBase.IsBuiltin() || inlineBase.Name().Local() != "positiveInteger" || inlineBase.Loc().IsZero() {
		t.Fatalf("InlineList anonymous base = %#v/%t, want located built-in positiveInteger", inlineBase, ok)
	}

	inlineUnion := requirePositiveIntegerDefinition(t, schema, "InlineUnion")
	inlineMembers := inlineUnion.MemberTypes()
	if len(inlineMembers) != 1 || !inlineMembers[0].IsAnonymous() {
		t.Fatalf("InlineUnion members = %#v, want one anonymous restriction", inlineMembers)
	}
	if inlineMembers[0].Loc().IsZero() {
		t.Fatal("InlineUnion member lost its source location")
	}
	inlineMemberDefinition, ok := inlineMembers[0].AnonymousType()
	if !ok {
		t.Fatal("InlineUnion anonymous member definition is missing")
	}
	assertPositiveIntegerDefinition(t, inlineMemberDefinition, version, "1")
}

func requirePositiveIntegerElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
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

func requirePositiveIntegerDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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

//nolint:gocognit // Keep built-in provenance, exact facets, and copy checks together.
func assertPositiveIntegerBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	if !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "positiveInteger") || reference.QName() != reference.Name() {
		t.Fatalf("reference = %#v, want built-in xs:positiveInteger", reference)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc {
		t.Fatalf("reference locations = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), wantLoc)
	}
	if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicPositiveInteger {
		t.Fatalf("reference variety/category = %q/%v, want atomic positiveInteger", reference.Variety(), reference.facts)
	}
	if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("built-in reference component ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	publicBounds, hasPublicBounds := reference.IntegerBounds()
	if !hasPublicBounds || publicBounds.Version() != version {
		t.Fatalf("public integer bounds = %v/%t, want version %s", publicBounds, hasPublicBounds, version)
	}
	publicMinimum, hasPublicMinimum := publicBounds.MinInclusiveFacet()
	if !hasPublicMinimum || publicMinimum.Value().Canonical() != "1" || !publicMinimum.Loc().IsZero() || publicMinimum.Kind() != BoundMinInclusive {
		t.Fatalf("public lower bound = %#v/%t, want intrinsic minInclusive=1 at Loc{}", publicMinimum, hasPublicMinimum)
	}
	_ = publicMinimum.value.value.SetInt64(99)
	secondBounds, _ := reference.IntegerBounds()
	secondMinimum, _ := secondBounds.MinInclusive()
	if secondMinimum.Canonical() != "1" {
		t.Fatalf("mutating copied public bound changed reference to %q", secondMinimum.Canonical())
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
		t.Fatal("built-in positiveInteger unexpectedly has totalDigits")
	}
	if _, hasMaxInclusive := facets.integerBounds.MaxInclusive(); hasMaxInclusive {
		t.Fatal("built-in positiveInteger unexpectedly has maxInclusive")
	}
	if _, hasMinExclusive := facets.integerBounds.MinExclusive(); hasMinExclusive {
		t.Fatal("built-in positiveInteger unexpectedly has minExclusive")
	}
	minimum, present := facets.integerBounds.MinInclusive()
	if !present || minimum.Canonical() != "1" {
		t.Fatalf("effective minInclusive = %q/%t, want 1/true", minimum.Canonical(), present)
	}
	minimumFacet, present := facets.integerBounds.MinInclusiveFacet()
	if !present || minimumFacet.Kind() != BoundMinInclusive || minimumFacet.Value().Canonical() != "1" || !minimumFacet.Loc().IsZero() || minimumFacet.Version() != version {
		t.Fatalf("effective minInclusive facts = %q/%s/%q/%t, want one/zero-loc/%q/true", minimumFacet.Value().Canonical(), minimumFacet.Loc(), minimumFacet.Kind(), present, version)
	}
	mutatedMinimum, present := facets.integerBounds.MinInclusive()
	if !present {
		t.Fatal("built-in reference has no effective minInclusive")
	}
	_ = mutatedMinimum.value.SetInt64(99)
	storedMinimum, present := facets.integerBounds.MinInclusive()
	if !present || storedMinimum.Canonical() != "1" {
		t.Fatalf("mutating returned minInclusive changed stored bound to %q", storedMinimum.Canonical())
	}
}

func assertPositiveIntegerReferenceFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, wantMinimum string) {
	t.Helper()
	if facts == nil || facts.atomicKind != schemaSimpleTypeAtomicPositiveInteger {
		t.Fatalf("reference facts = %#v, want positiveInteger facts", facts)
	}
	completed := *facts
	completed.variety = SimpleTypeVarietyAtomicRestriction
	bounds, ok := (SimpleTypeReference{facts: &completed}).IntegerBounds()
	if !ok || bounds.Version() != version {
		t.Fatalf("public reference bounds = %v/%t, want version %s", bounds, ok, version)
	}
	minimum, present := bounds.MinInclusive()
	if !present || minimum.Canonical() != wantMinimum {
		t.Fatalf("effective minInclusive = %q/%t, want %s/true", minimum.Canonical(), present, wantMinimum)
	}
	if _, present := bounds.MaxInclusive(); present {
		t.Fatal("positiveInteger reference unexpectedly has maxInclusive")
	}
}

func assertPositiveIntegerDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMinimum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicPositiveInteger {
		t.Fatalf("definition variety/category = %q/%v, want atomic positiveInteger", definition.Variety(), definition.facts)
	}
	assertPositiveIntegerReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, wantMinimum)
}

//nolint:gocognit // Graph order and direct/named facts are checked together.
func TestSchemaPositiveIntegerComposesGraphs(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := positiveIntegerGraphFixtures(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeat graph discovery: %v", err)
			}
			if !reflect.DeepEqual(schema.Components(), repeated.Components()) || !reflect.DeepEqual(schema.Documents(), repeated.Documents()) {
				t.Fatal("repeated/cyclic graph discovery changed public order or facts")
			}
			if got := len(schema.Documents()); got != 4 {
				t.Fatalf("document count = %d, want 4 after repeated/cyclic graph discovery", got)
			}
			for _, test := range positiveIntegerGraphElementCases() {
				assertPositiveIntegerGraphElement(t, schema, root, fixtures, test, profile.version)
			}
		})
	}
}

func positiveIntegerGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="xs:positiveInteger"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="xs:positiveInteger"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="chameleon" type="xs:positiveInteger"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="xs:positiveInteger"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func positiveIntegerGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="xs:positiveInteger"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="xs:positiveInteger"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="xs:positiveInteger"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="xs:positiveInteger"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertPositiveIntegerGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requirePositiveIntegerElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() || reference.Loc() != schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures) {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		if id, ok := reference.ComponentID(); !ok || id.IsZero() {
			t.Fatalf("%s named type identity = %v/%t, want nonzero", test.local, id, ok)
		}
		bounds, ok := reference.IntegerBounds()
		if !ok {
			t.Fatalf("%s named type has no public integer bounds", test.local)
		}
		minimum, ok := bounds.MinInclusiveFacet()
		if !ok || minimum.Value().Canonical() != "1" || !minimum.Loc().IsZero() || minimum.Version() != version {
			t.Fatalf("%s bound = %#v/%t, want intrinsic minInclusive=1", test.local, minimum, ok)
		}
		assertPositiveIntegerReferenceFacts(t, reference.facts, version, "1")
		return
	}
	assertPositiveIntegerBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
}

//nolint:gocognit // One boundary matrix covers each invalid facet exit.
func TestSchemaPositiveIntegerRejectsOutOfBaseRestrictions(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name  string
			body  string
			loc   string
			kind  BoundKind
			code  string
			cause error
		}{
			{name: "zero inclusive bound", body: `<xs:minInclusive value="0"/>`, loc: `value="0"`, kind: BoundMinInclusive, code: InvalidBoundRestrictionCode, cause: errInvalidBoundRestriction},
			{name: "negative inclusive bound", body: `<xs:minInclusive value="-1"/>`, loc: `value="-1"`, kind: BoundMinInclusive, code: InvalidBoundRestrictionCode, cause: errInvalidBoundRestriction},
			{name: "zero exclusive bound", body: `<xs:minExclusive value="0"/>`, loc: `value="0"`, kind: BoundMinExclusive, code: InvalidBoundRestrictionCode, cause: errInvalidBoundRestriction},
			{name: "zero enumeration", body: `<xs:enumeration value="0"/>`, loc: `<xs:enumeration`, code: InvalidEnumerationRestrictionCode, cause: errInvalidEnumerationRestriction},
			{name: "negative enumeration", body: `<xs:enumeration value="-1"/>`, loc: `<xs:enumeration`, code: InvalidEnumerationRestrictionCode, cause: errInvalidEnumerationRestriction},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:simpleType name="Bad"><xs:restriction base="xs:positiveInteger">` + test.body + `</xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertPositiveIntegerNoPartialSchema(t, schema, err)
				if !errors.Is(err, test.cause) {
					t.Fatalf("error = %v, want cause %v", err, test.cause)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.loc)
				if diagnostic.Code() != test.code || diagnostic.Class() != FailureInvalid || diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 {
					t.Fatalf("diagnostic = %s related=%v, want %s at %s without source related location", diagnostic, diagnostic.Related(), test.code, wantLoc)
				}
				wantSpec := enumerationSpecRef(profile.version, enumerationRestrictionRule)
				if test.kind != 0 {
					wantSpec = boundSpecRef(profile.version, test.kind, boundRestrictionRule)
				}
				if diagnostic.SpecRef() != wantSpec {
					t.Fatalf("spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
				}
			})
		}
	}
}

func TestSchemaPositiveIntegerAcceptsPositiveTotalDigits(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="t:Limited"/><xs:simpleType name="Limited"><xs:restriction base="xs:positiveInteger"><xs:totalDigits value="9"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			definition := requirePositiveIntegerDefinition(t, schema, "Limited")
			total, present := definition.DigitFacets().TotalDigits()
			if !present || total.Canonical() != "9" {
				t.Fatalf("totalDigits = %q/%t, want 9/true", total.Canonical(), present)
			}
		})
	}
}

//nolint:gocognit // Each malformed, unresolved, wrong-kind, and cyclic exit has located proof.
func TestSchemaPositiveIntegerRejectsMalformedUnresolvedWrongKindAndElementRefs(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name    string
			root    string
			cause   error
			code    string
			marker  string
			related []string
			spec    string
		}{
			{
				name: "malformed bound", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:positiveInteger"><xs:minInclusive value="not-an-integer"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundValue, code: InvalidBoundCode, marker: `value="not-an-integer"`, spec: "bound",
			},
			{
				name: "unresolved base", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="Bad"><xs:restriction base="t:Missing"/></xs:simpleType></xs:schema>`,
				cause: errSchemaSimpleTypeBaseUnresolved, code: diagnosticSchemaSimpleTypeUnresolvedCode, marker: `base="t:Missing"`,
			},
			{
				name: "wrong kind base", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:element name="NotAType" type="xs:integer"/><xs:simpleType name="Bad"><xs:restriction base="t:NotAType"/></xs:simpleType></xs:schema>`,
				cause: errSchemaSimpleTypeBaseWrongKind, code: diagnosticSchemaSimpleTypeWrongKindCode, marker: `base="t:NotAType"`, related: []string{`<xs:element name="NotAType"`},
			},
			{
				name: "local ref remains element symbol space", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Root"><xs:sequence><xs:element ref="xs:positiveInteger"/></xs:sequence></xs:complexType></xs:schema>`,
				cause: errSchemaElementReferenceUnresolved, code: diagnosticSchemaElementReferenceUnresolvedCode, marker: `ref="xs:positiveInteger"`, spec: "ref",
			},
			{
				name: "cyclic named union", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="A"><xs:union memberTypes="xs:positiveInteger t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType></xs:schema>`,
				cause: errSchemaSimpleTypeBaseCycle, code: diagnosticSchemaSimpleTypeCycleCode, marker: `memberTypes="xs:positiveInteger t:B"`, related: []string{`base="t:A"`},
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				assertPositiveIntegerNoPartialSchema(t, schema, err)
				if !errors.Is(err, test.cause) {
					t.Fatalf("error = %v, want cause %v", err, test.cause)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Code() != test.code {
					t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
				}
				wantLoc := elementReferenceTestAttributeLoc(t, test.root, test.marker)
				wantRelated := positiveIntegerLocs(t, test.root, test.related)
				if diagnostic.Loc() != wantLoc || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("locations = %s/%v, want %s/%v", diagnostic.Loc(), diagnostic.Related(), wantLoc, wantRelated)
				}
				wantSpec := schemaSimpleTypeSpecRef(profile.version)
				switch test.spec {
				case "bound":
					wantSpec = boundSpecRef(profile.version, BoundMinInclusive, boundDefinitionRule)
				case "ref":
					wantSpec = schemaElementReferenceSpecRef(profile.version)
				}
				if diagnostic.SpecRef() != wantSpec {
					t.Fatalf("SpecRef = %q, want %q", diagnostic.SpecRef(), wantSpec)
				}
			})
		}
	}
}

func assertPositiveIntegerNoPartialSchema(t *testing.T, schema Schema, err error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located error and no partial schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
}

//nolint:gocognit // The anonymous reference and inherited built-in bound are checked together.
func TestSchemaPositiveIntegerGlobalInlineReference(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:element name="inline"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:element></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover inline global element: %v", err)
			}
			declaration := requirePositiveIntegerElement(t, schema, "inline", "urn:test")
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsAnonymous() {
				t.Fatalf("inline type reference = %#v/%t, want anonymous", reference, ok)
			}
			anonymous, ok := reference.AnonymousType()
			if !ok || anonymous.Base() != mustTestQName(t, testXSDNamespace, "positiveInteger") {
				t.Fatalf("anonymous type = %#v/%t, want positiveInteger restriction", anonymous, ok)
			}
			base, ok := anonymous.BaseReference()
			if !ok {
				t.Fatal("inline type has no base reference")
			}
			assertPositiveIntegerBuiltinReference(t, base, elementReferenceTestAttributeLoc(t, root, `base="xs:positiveInteger"`), profile.version)
			bounds, ok := anonymous.IntegerBounds()
			if !ok {
				t.Fatal("anonymous type has no integer bounds")
			}
			inclusive, ok := bounds.MinInclusiveFacet()
			if !ok || inclusive.Value().Canonical() != "1" || !inclusive.Loc().IsZero() {
				t.Fatalf("anonymous minInclusive = %#v/%t, want intrinsic 1 at Loc{}", inclusive, ok)
			}
		})
	}
}

//nolint:gocognit // Each row is an independently excluded public schema shape.
func TestSchemaPositiveIntegerExcludedSchemaShapes(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name     string
			body     string
			marker   string
			specKind string
			cause    error
			related  string
		}{
			{name: "local direct", body: `<xs:complexType name="Root"><xs:sequence><xs:element name="item" type="xs:positiveInteger"/></xs:sequence></xs:complexType>`, marker: `type="xs:positiveInteger"`},
			{name: "local named", body: `<xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:complexType name="Root"><xs:sequence><xs:element name="item" type="t:Alias"/></xs:sequence></xs:complexType>`, marker: `type="t:Alias"`},
			{name: "local inline", body: `<xs:complexType name="Root"><xs:sequence><xs:element name="item"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:element></xs:sequence></xs:complexType>`, marker: `<xs:simpleType>`},
			{name: "global attribute inline", specKind: "inline", body: `<xs:attribute name="item"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:attribute>`, marker: `<xs:simpleType>`},
			{name: "global attribute unrelated integer kind", specKind: "attribute", body: `<xs:simpleType name="Alias"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="1"/></xs:restriction></xs:simpleType><xs:attribute name="item" type="t:Alias"/>`, marker: `type="t:Alias"`, cause: errSchemaAttributeTypeUnsupported},
			{name: "global attribute list variety", specKind: "attribute", body: `<xs:simpleType name="Alias"><xs:list itemType="xs:positiveInteger"/></xs:simpleType><xs:attribute name="item" type="t:Alias"/>`, marker: `type="t:Alias"`, cause: errSchemaAttributeTypeUnsupported},
			{name: "global attribute union variety", specKind: "attribute", body: `<xs:simpleType name="Alias"><xs:union memberTypes="xs:positiveInteger"/></xs:simpleType><xs:attribute name="item" type="t:Alias"/>`, marker: `type="t:Alias"`, cause: errSchemaAttributeTypeUnsupported},
			{name: "attribute value inline", specKind: "inline", body: `<xs:attribute name="item" default="1"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:attribute>`, marker: `<xs:simpleType>`},
			{name: "local attribute direct", specKind: "attribute", body: `<xs:complexType name="Root"><xs:attribute name="item" type="xs:positiveInteger"/></xs:complexType>`, marker: `type="xs:positiveInteger"`, cause: errSchemaAttributeUseUnsupported},
			{name: "local attribute named", specKind: "attribute", body: `<xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:complexType name="Root"><xs:attribute name="item" type="t:Alias"/></xs:complexType>`, marker: `type="t:Alias"`, cause: errSchemaAttributeUseUnsupported},
			{name: "local attribute inline", specKind: "attribute", body: `<xs:complexType name="Root"><xs:attribute name="item"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:attribute></xs:complexType>`, marker: `<xs:simpleType>`},
			{name: "local attribute ref target", specKind: "attributeUse", body: `<xs:attribute name="item" type="xs:positiveInteger"/><xs:complexType name="Root"><xs:attribute ref="t:item"/></xs:complexType>`, marker: `ref="t:item"`, cause: errSchemaAttributeReferenceUnsupported, related: `<xs:attribute name="item"`},
			{name: "local attribute ref named target", specKind: "attributeUse", body: `<xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:attribute name="item" type="t:Alias"/><xs:complexType name="Root"><xs:attribute ref="t:item"/></xs:complexType>`, marker: `ref="t:item"`, cause: errSchemaAttributeReferenceUnsupported, related: `<xs:attribute name="item"`},
			{name: "simpleContent direct", specKind: "simpleContent", body: `<xs:complexType name="Root"><xs:simpleContent><xs:extension base="xs:positiveInteger"/></xs:simpleContent></xs:complexType>`, marker: `base="xs:positiveInteger"`},
			{name: "simpleContent named", specKind: "simpleContent", body: `<xs:complexType name="Root"><xs:simpleContent><xs:extension base="t:Alias"/></xs:simpleContent></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`, marker: `base="t:Alias"`},
			{name: "wider family", body: `<xs:element name="item" type="xs:unsignedInt"/>`, marker: `type="xs:unsignedInt"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("excluded shape returned schema/error %v/%v", schema, err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want schema syntax unsupported at %s", diagnostic, wantLoc)
				}
				wantSpec := positiveIntegerFeatureSpecRef(t, FeatureSchemaSyntax, profile.version)
				switch test.specKind {
				case "attribute":
					wantSpec = schemaAttributeTypeSpecRef(profile.version)
				case "value":
					wantSpec = schemaAttributeValueConstraintSpecRef(profile.version)
				case "attributeUse":
					wantSpec = schemaAttributeUseSpecRef(profile.version)
				case "inline":
					wantSpec = "xsd10-structures#schema-document"
				case "simpleContent":
					wantSpec = schemaSimpleContentSpecRef(profile.version)
				}
				if diagnostic.SpecRef() != wantSpec {
					t.Fatalf("spec ref = %q, want selected schema syntax reference", diagnostic.SpecRef())
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
				}
				if test.related != "" {
					wantRelated := elementReferenceTestAttributeLoc(t, root, test.related)
					if !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) {
						t.Fatalf("related locations = %v, want [%s]", diagnostic.Related(), wantRelated)
					}
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations = %v", diagnostic.Related())
				}
			})
		}
	}
}

func positiveIntegerFeatureSpecRef(t *testing.T, id FeatureID, version XSDVersion) string {
	t.Helper()
	feature, ok := LookupUnsupportedFeature(id)
	if !ok {
		t.Fatalf("unsupported feature %q is unregistered", id)
	}
	for _, reference := range feature.References() {
		if reference.Version() == string(version) {
			return reference.Source()
		}
	}
	t.Fatalf("feature %q has no %s specification reference", id, version)
	return ""
}

//nolint:gocognit // Check all four type shapes against both public consumers.
func TestSchemaPositiveIntegerConsumerShapes(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name              string
			body              string
			instance          string
			generationAt      string
			generationRelated []string
			validationRelated []string
			generationSpec    string
		}{
			{name: "direct", body: `<xs:element name="value" type="xs:positiveInteger"/>`, instance: `<value xmlns="urn:test">1</value>`, generationAt: `<xs:element name="value"`, validationRelated: []string{`<xs:element name="value"`}, generationSpec: "element"},
			{name: "named", body: `<xs:element name="value" type="t:Named"/><xs:simpleType name="Named"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`, instance: `<value xmlns="urn:test">1</value>`, generationAt: `<xs:element name="value"`, generationRelated: []string{`<xs:simpleType name="Named"`, `base="xs:positiveInteger"`}, validationRelated: []string{`<xs:element name="value"`, `<xs:simpleType name="Named"`}, generationSpec: "simple"},
			{name: "inline", body: `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:element>`, instance: `<value xmlns="urn:test">1</value>`, generationAt: `<xs:element name="value"`, generationRelated: []string{`<xs:simpleType>`}, validationRelated: []string{`<xs:element name="value"`}, generationSpec: "element"},
			{name: "ref", body: `<xs:element name="value" type="xs:positiveInteger"/><xs:element name="root" type="t:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, instance: `<root xmlns="urn:test"><value>1</value></root>`, generationAt: `ref="t:value"`, generationRelated: []string{`<xs:element ref="t:value"`, `<xs:element name="value"`}, validationRelated: []string{`<xs:element name="root"`, `<xs:complexType name="Choice"`, `<xs:choice>`, `<xs:element ref="t:value"`, `<xs:element name="value"`}, generationSpec: "choice"},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover consumer schema: %v", err)
				}
				generated, generationErr := GenerateGo(schema, "generated")
				if generated != nil || generationErr == nil {
					t.Fatalf("GenerateGo result = (%q, %v), want nil output and unsupported", generated, generationErr)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.instance)))
				if validationErr == nil {
					t.Fatal("ValidateInstance accepted positiveInteger")
				}
				generationDiagnostic := requireDiagnostic(t, generationErr)
				validationDiagnostic := requireDiagnostic(t, validationErr)
				generationLoc := elementReferenceTestAttributeLoc(t, root, test.generationAt)
				generationRelated := positiveIntegerLocs(t, root, test.generationRelated)
				if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != generationLoc || !reflect.DeepEqual(generationDiagnostic.Related(), generationRelated) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %s related=%v, want unsupported at %s related=%v", generationDiagnostic, generationDiagnostic.Related(), generationLoc, generationRelated)
				}
				generationSpec := schemaElementTypeSpecRef(profile.version)
				switch test.generationSpec {
				case "simple":
					generationSpec = schemaSimpleTypeSpecRef(profile.version)
				case "choice":
					generationSpec = "xsd" + strings.ReplaceAll(string(profile.version), ".", "") + "-structures#element-choice"
				}
				if generationDiagnostic.SpecRef() != generationSpec {
					t.Fatalf("generation SpecRef = %q, want %q", generationDiagnostic.SpecRef(), generationSpec)
				}
				validationLoc := mustTestLoc(t, "instance.xml", 1, 1)
				validationRelated := positiveIntegerLocs(t, root, test.validationRelated)
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc() != validationLoc || !reflect.DeepEqual(validationDiagnostic.Related(), validationRelated) || !errors.Is(validationErr, positiveIntegerValidationCause(test.name)) {
					t.Fatalf("validation diagnostic = %s related=%v, want unsupported at %s related=%v", validationDiagnostic, validationDiagnostic.Related(), validationLoc, validationRelated)
				}
				if validationDiagnostic.SpecRef() != instanceValidationSpecRef(profile.version) {
					t.Fatalf("validation SpecRef = %q, want %q", validationDiagnostic.SpecRef(), instanceValidationSpecRef(profile.version))
				}
			})
		}
	}
}

func positiveIntegerLocs(t *testing.T, source string, markers []string) []Loc {
	t.Helper()
	if len(markers) == 0 {
		return nil
	}
	locations := make([]Loc, 0, len(markers))
	for _, marker := range markers {
		locations = append(locations, elementReferenceTestAttributeLoc(t, source, marker))
	}
	return locations
}

func positiveIntegerValidationCause(shape string) error {
	if shape == "inline" {
		return errInstanceNoDeclaredType
	}
	return errInstanceUnsupportedType
}

//nolint:gocognit // Each element constraint shape preserves its existing boundary diagnostic.
func TestSchemaPositiveIntegerElementValueConstraintsRemainUnsupported(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name   string
			body   string
			marker string
			class  FailureClass
			code   string
		}{
			{name: "direct default", body: `<xs:element name="value" type="xs:positiveInteger" default="1"/>`, marker: `default="1"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
			{name: "named fixed", body: `<xs:element name="value" type="t:Alias" fixed="1"/><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`, marker: `fixed="1"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
			{name: "inline default", body: `<xs:element name="value" default="1"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:element>`, marker: `default="1"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
			{name: "ref default", body: `<xs:element name="target" type="xs:positiveInteger"/><xs:complexType name="Root"><xs:choice><xs:element ref="t:target" default="1"/></xs:choice></xs:complexType>`, marker: `default="1"`, class: FailureInvalid, code: invalidSchemaCompositionCode},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("value-constrained positiveInteger returned schema/error %v/%v", schema, err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 {
					t.Fatalf("diagnostic = %s related=%v, want %s/%s at %s", diagnostic, diagnostic.Related(), test.class, test.code, wantLoc)
				}
				wantSpec := "xsd10-structures#schema-document"
				if test.class == FailureInvalid {
					wantSpec = ""
				}
				if diagnostic.SpecRef() != wantSpec || diagnostic.Unwrap() != nil || errors.Is(err, ErrUnsupported) != (test.class == FailureUnsupported) {
					t.Fatalf("diagnostic provenance = %q/%v/%t, want %q/nil/%t", diagnostic.SpecRef(), diagnostic.Unwrap(), errors.Is(err, ErrUnsupported), wantSpec, test.class == FailureUnsupported)
				}
			})
		}
	}
}
