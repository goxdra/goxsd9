package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

type longPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func longPolicyProfiles() []longPolicyProfile {
	return []longPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
}

func TestSchemaLongReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaLongReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated long builds changed component facts or order")
			}
			assertLongReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaLongReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="xs:long"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="narrowed" type="t:Tight"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:long"><xs:enumeration value="-9223372036854775808"/><xs:enumeration value="9223372036854775807"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:long"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:long t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

func assertLongReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertLongDirectReference(t, schema, root, version)
	assertLongNamedReferences(t, schema, root, version)
	assertLongCollectionReferences(t, schema, root, version)
	assertLongInlineReferences(t, schema, version)
}

func assertLongDirectReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireLongElement(t, schema, "direct", "urn:test")
	reference, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct element type reference is missing")
	}
	assertLongBuiltinReference(t, reference, elementReferenceTestAttributeLoc(t, root, `type="xs:long"`), version)
	if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("direct element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "long")); len(matches) != 0 {
		t.Fatalf("built-in long is visible as %d named component(s)", len(matches))
	}
}

func assertLongNamedReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	forward := requireLongElement(t, schema, "forward", "urn:test")
	forwardReference, ok := forward.TypeReference()
	if !ok || !forwardReference.IsNamed() || forwardReference.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("forward element type reference = %#v/%t, want named Later", forwardReference, ok)
	}
	later := requireLongDefinition(t, schema, "Later")
	forwardID, forwardIDOK := forwardReference.ComponentID()
	if !forwardIDOK || forwardID != later.ID() {
		t.Fatalf("forward type ID = %v/%t, want Later %v/true", forwardID, forwardIDOK, later.ID())
	}
	assertLongReferenceFacts(t, forwardReference.facts, version, "-100", "100")
	if forwardReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:Later"`) {
		t.Fatalf("forward reference location = %s, want type attribute location", forwardReference.Loc())
	}

	base, ok := later.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "long") {
		t.Fatalf("Later base reference = %#v/%t, want built-in long", base, ok)
	}
	if base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:long"`) || base.VarietyLoc() != base.Loc() {
		t.Fatalf("Later base locations = %s/%s, want the base attribute location", base.Loc(), base.VarietyLoc())
	}
	assertLongBuiltinReference(t, base, base.Loc(), version)

	derived := requireLongDefinition(t, schema, "Derived")
	assertLongDefinition(t, derived, version, "-100", "100")
	derivedBase, ok := derived.BaseReference()
	if !ok || !derivedBase.IsNamed() || derivedBase.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Derived base reference = %#v/%t, want named Later", derivedBase, ok)
	}

	tight := requireLongDefinition(t, schema, "Tight")
	assertLongDefinition(t, tight, version, "-100", "2")

	enumerated := requireLongDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 2 || values[0].Canonical() != "-9223372036854775808" || values[1].Canonical() != "9223372036854775807" {
		t.Fatalf("Enumerated values = %#v, want exact signed 64-bit boundaries", values)
	}
	if locations := enumerated.IntegerEnumerationFacets().Locations(); len(locations) != 2 || locations[0].IsZero() || locations[1].IsZero() {
		t.Fatalf("Enumerated locations = %#v, want two source locations", locations)
	}
}

func assertLongCollectionReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	list := requireLongDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item type is missing")
	}
	assertLongBuiltinReference(t, item, elementReferenceTestAttributeLoc(t, root, `itemType="xs:long"`), version)

	union := requireLongDefinition(t, schema, "Union")
	if union.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("Union variety = %q, want union", union.Variety())
	}
	members := union.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("Union member count = %d, want 2", len(members))
	}
	assertLongBuiltinReference(t, members[0], elementReferenceTestAttributeLoc(t, root, `memberTypes="xs:long`), version)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Union member 1 = %#v, want named Later", members[1])
	}
	assertLongReferenceFacts(t, members[1].facts, version, "-100", "100")
}

func assertLongInlineReferences(t *testing.T, schema Schema, version XSDVersion) {
	inlineList := requireLongDefinition(t, schema, "InlineList")
	inlineItem, ok := inlineList.ItemType()
	if !ok || !inlineItem.IsAnonymous() {
		t.Fatalf("InlineList item = %#v/%t, want anonymous restriction", inlineItem, ok)
	}
	inlineItemDefinition, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineList item anonymous definition is missing")
	}
	assertLongDefinition(t, inlineItemDefinition, version, "-9223372036854775808", "9223372036854775807")
	inlineBase, ok := inlineItemDefinition.BaseReference()
	if !ok || !inlineBase.IsBuiltin() || inlineBase.Name().Local() != "long" || inlineBase.Loc().IsZero() {
		t.Fatalf("InlineList anonymous base = %#v/%t, want located built-in long", inlineBase, ok)
	}

	inlineUnion := requireLongDefinition(t, schema, "InlineUnion")
	inlineMembers := inlineUnion.MemberTypes()
	if len(inlineMembers) != 1 || !inlineMembers[0].IsAnonymous() {
		t.Fatalf("InlineUnion members = %#v, want one anonymous restriction", inlineMembers)
	}
	inlineMemberDefinition, ok := inlineMembers[0].AnonymousType()
	if !ok {
		t.Fatal("InlineUnion anonymous member definition is missing")
	}
	assertLongDefinition(t, inlineMemberDefinition, version, "-9223372036854775808", "9223372036854775807")
}

func requireLongElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
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

func requireLongDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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
func assertLongBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	wantName := mustTestQName(t, testXSDNamespace, "long")
	if !reference.IsBuiltin() || reference.Name() != wantName || reference.QName() != wantName {
		t.Fatalf("reference = %#v, want built-in xs:long", reference)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc {
		t.Fatalf("reference locations = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), wantLoc)
	}
	if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicLong {
		t.Fatalf("reference variety/category = %q/%v, want atomic long", reference.Variety(), reference.facts)
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
		t.Fatal("built-in long unexpectedly has totalDigits")
	}
	assertIntegerBounds(t, facets.integerBounds, version, "-9223372036854775808", "9223372036854775807")
	for _, bound := range facets.integerBounds.Bounds() {
		if !bound.Loc().IsZero() {
			t.Fatalf("built-in long bound %s has source location %s", bound.Kind(), bound.Loc())
		}
	}

	minimum, present := facets.integerBounds.MinInclusive()
	if !present {
		t.Fatal("built-in reference has no effective minInclusive")
	}
	_ = minimum.value.SetInt64(0)
	maximum, present := facets.integerBounds.MaxInclusive()
	if !present {
		t.Fatal("built-in reference has no effective maxInclusive")
	}
	_ = maximum.value.SetInt64(0)
	assertIntegerBounds(t, facets.integerBounds, version, "-9223372036854775808", "9223372036854775807")
}

func assertLongReferenceFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, wantMinimum, wantMaximum string) {
	t.Helper()
	assertIntegerReferenceFacts(t, facts, version, schemaSimpleTypeAtomicLong, "long", wantMinimum, wantMaximum)
}

func assertIntegerReferenceFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, wantAtomicKind schemaSimpleTypeAtomicKind, wantKindName, wantMinimum, wantMaximum string) {
	t.Helper()
	if facts == nil || facts.atomicKind != wantAtomicKind {
		t.Fatalf("reference facts = %#v, want %s facts", facts, wantKindName)
	}
	var bounds IntegerBoundFacets
	switch facets := facts.facets.(type) {
	case schemaDigitFacetVariant:
		if facets.value.Kind() != DigitDatatypeInteger || facets.value.Version() != version {
			t.Fatalf("digit facts = %q/%q, want integer/%q", facets.value.Kind(), facets.value.Version(), version)
		}
		bounds = facets.integerBounds
	case schemaIntegerFacetVariant:
		if facets.digits.Kind() != DigitDatatypeInteger || facets.digits.Version() != version {
			t.Fatalf("integer facts = %q/%q, want integer/%q", facets.digits.Kind(), facets.digits.Version(), version)
		}
		bounds = facets.bounds
	default:
		t.Fatalf("reference facets = %T, want integer facts", facts.facets)
	}
	assertIntegerBounds(t, bounds, version, wantMinimum, wantMaximum)
}

func assertLongDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMinimum, wantMaximum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicLong {
		t.Fatalf("definition variety/category = %q/%v, want atomic long", definition.Variety(), definition.facts)
	}
	assertLongReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, wantMinimum, wantMaximum)
}

func assertIntegerBounds(t *testing.T, bounds IntegerBoundFacets, version XSDVersion, wantMinimum, wantMaximum string) {
	t.Helper()
	minimum, present := bounds.MinInclusive()
	if !present || minimum.Canonical() != wantMinimum {
		t.Fatalf("effective minInclusive = %q/%t, want %s/true", minimum.Canonical(), present, wantMinimum)
	}
	minimumFacet, present := bounds.MinInclusiveFacet()
	if !present || minimumFacet.Kind() != BoundMinInclusive || minimumFacet.Value().Canonical() != wantMinimum || minimumFacet.Version() != version {
		t.Fatalf("effective minInclusive facts = %q/%s/%q/%t, want %s/%q/true", minimumFacet.Value().Canonical(), minimumFacet.Loc(), minimumFacet.Kind(), present, wantMinimum, version)
	}
	maximum, present := bounds.MaxInclusive()
	if !present || maximum.Canonical() != wantMaximum {
		t.Fatalf("effective maxInclusive = %q/%t, want %s/true", maximum.Canonical(), present, wantMaximum)
	}
	maximumFacet, present := bounds.MaxInclusiveFacet()
	if !present || maximumFacet.Kind() != BoundMaxInclusive || maximumFacet.Value().Canonical() != wantMaximum || maximumFacet.Version() != version {
		t.Fatalf("effective maxInclusive facts = %q/%s/%q/%t, want %s/%q/true", maximumFacet.Value().Canonical(), maximumFacet.Loc(), maximumFacet.Kind(), present, wantMaximum, version)
	}
	ordered := bounds.Bounds()
	if len(ordered) != 2 || ordered[0].Kind() != BoundMinInclusive || ordered[1].Kind() != BoundMaxInclusive {
		t.Fatalf("ordered integer bounds = %#v, want minInclusive then maxInclusive", ordered)
	}
}

//nolint:gocognit // Keep repeated graph discovery and ordered reference checks together.
func TestSchemaLongComposesGraphs(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := longGraphFixtures(profile.version)
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
			for _, test := range longGraphElementCases() {
				assertLongGraphElement(t, first, root, fixtures, test, profile.version)
			}
		})
	}
}

func longGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="xs:long"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="xs:long"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="xs:long"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="chameleon" type="xs:long"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="xs:long"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="xs:long"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func longGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="xs:long"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="xs:long"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="xs:long"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="xs:long"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertLongGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requireLongElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		assertLongReferenceFacts(t, reference.facts, version, "-9223372036854775808", "9223372036854775807")
		return
	}
	assertLongBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
}

func TestSchemaLongRejectsInvalidReferencesAndRestrictions(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name  string
			root  string
			cause error
		}{
			{
				name:  "below minimum",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:minInclusive value="-9223372036854775809"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundRestriction,
			},
			{
				name:  "above maximum",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidBoundRestriction,
			},
			{
				name:  "out of range enumeration",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:enumeration value="9223372036854775808"/></xs:restriction></xs:simpleType></xs:schema>`,
				cause: errInvalidEnumerationRestriction,
			},
			{
				name:  "malformed bound",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType></xs:schema>`,
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
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Root"><xs:sequence><xs:element ref="xs:long"/></xs:sequence></xs:complexType></xs:schema>`,
				cause: errSchemaElementReferenceUnresolved,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				assertLongInvalidNoPartialSchema(t, schema, err, test.cause)
			})
		}
	}
}

func assertLongInvalidNoPartialSchema(t *testing.T, schema Schema, err error, cause error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located error and no partial schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func TestSchemaLongExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertSchemaIntegerDerivedExcludedShapes(t, profile.policy, "long", "0")
		})
	}
}

func TestSchemaLongConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="xs:long"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertNonNegativeIntegerConsumersUnsupported(t, schema)
		})
	}
}

func TestSchemaLongDoesNotAdmitNarrowerBuiltins(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="xs:int"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverTestSchemaWithPolicy admitted an unrelated narrower built-in")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic = %s, want located unsupported diagnostic", diagnostic)
			}
		})
	}
}
