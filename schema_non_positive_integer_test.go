package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type nonPositiveIntegerPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func nonPositiveIntegerPolicyProfiles() []nonPositiveIntegerPolicyProfile {
	return []nonPositiveIntegerPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
}

func TestSchemaNonPositiveIntegerReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaNonPositiveIntegerReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated nonPositiveInteger builds changed component facts or order")
			}
			assertNonPositiveIntegerReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaNonPositiveIntegerReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="xs:nonPositiveInteger"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="narrowed" type="t:Tight"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:nonPositiveInteger"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:maxInclusive value="-2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:nonPositiveInteger"><xs:enumeration value="-0"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:nonPositiveInteger"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:nonPositiveInteger t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

func assertNonPositiveIntegerReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertNonPositiveIntegerDirectReference(t, schema, root, version)
	assertNonPositiveIntegerNamedReferenceShapes(t, schema, root, version)
	assertNonPositiveIntegerCollectionShapes(t, schema, root, version)
	assertNonPositiveIntegerInlineShapes(t, schema, version)
}

func assertNonPositiveIntegerDirectReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireNonPositiveIntegerElement(t, schema, "direct", "urn:test")
	directReference, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct element type reference is missing")
	}
	assertNonPositiveIntegerBuiltinReference(
		t,
		directReference,
		elementReferenceTestAttributeLoc(t, root, `type="xs:nonPositiveInteger"`),
		version,
	)
	if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("direct element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "nonPositiveInteger")); len(matches) != 0 {
		t.Fatalf("built-in nonPositiveInteger is visible as %d named component(s)", len(matches))
	}
}

func assertNonPositiveIntegerNamedReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertNonPositiveIntegerForwardReference(t, schema, root, version)
	assertNonPositiveIntegerTighteningAndEnumeration(t, schema, root, version)
}

func assertNonPositiveIntegerForwardReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	forward := requireNonPositiveIntegerElement(t, schema, "forward", "urn:test")
	forwardReference, ok := forward.TypeReference()
	if !ok || !forwardReference.IsNamed() || forwardReference.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("forward element type reference = %#v/%t, want named Later", forwardReference, ok)
	}
	later := requireNonPositiveIntegerDefinition(t, schema, "Later")
	forwardID, forwardIDOK := forwardReference.ComponentID()
	if !forwardIDOK || forwardID != later.ID() {
		t.Fatalf("forward type ID = %v/%t, want Later %v/true", forwardID, forwardIDOK, later.ID())
	}
	assertNonPositiveIntegerReferenceFacts(t, forwardReference.facts, version, "0")
	if forwardReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:Later"`) {
		t.Fatalf("forward reference location = %s, want type attribute location", forwardReference.Loc())
	}

	base, ok := later.BaseReference()
	if !ok || !base.IsBuiltin() || base.Name().Local() != "nonPositiveInteger" {
		t.Fatalf("Later base reference = %#v/%t, want built-in nonPositiveInteger", base, ok)
	}
	if base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:nonPositiveInteger"`) || base.VarietyLoc() != base.Loc() {
		t.Fatalf("Later base locations = %s/%s, want the base attribute location", base.Loc(), base.VarietyLoc())
	}
	assertNonPositiveIntegerReferenceFacts(t, base.facts, version, "0")
	digits := later.DigitFacets()
	if digits.Kind() != DigitDatatypeInteger || digits.Version() != version {
		t.Fatalf("Later digit facts = %q/%q, want integer/%q", digits.Kind(), digits.Version(), version)
	}
	total, present := digits.TotalDigits()
	if !present || total.Canonical() != "4" {
		t.Fatalf("Later totalDigits = %q/%t, want 4/true", total.Canonical(), present)
	}
}

func assertNonPositiveIntegerTighteningAndEnumeration(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	tight := requireNonPositiveIntegerDefinition(t, schema, "Tight")
	assertNonPositiveIntegerDefinition(t, tight, version, "-2")
	tightBase, ok := tight.BaseReference()
	if !ok || !tightBase.IsNamed() || tightBase.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Tight base reference = %#v/%t, want named Later", tightBase, ok)
	}
	if tightBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 8, `base="t:Later"`) {
		t.Fatalf("Tight base location = %s, want base attribute location", tightBase.Loc())
	}

	enumerated := requireNonPositiveIntegerDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 1 || values[0].Canonical() != "0" {
		t.Fatalf("Enumerated values = %#v, want canonical zero from -0", values)
	}
	if locations := enumerated.IntegerEnumerationFacets().Locations(); len(locations) != 1 || locations[0].IsZero() {
		t.Fatalf("Enumerated locations = %#v, want one source location", locations)
	}
}

func assertNonPositiveIntegerCollectionShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	list := requireNonPositiveIntegerDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item type is missing")
	}
	assertNonPositiveIntegerBuiltinReference(
		t,
		item,
		elementReferenceTestAttributeLoc(t, root, `itemType="xs:nonPositiveInteger"`),
		version,
	)

	union := requireNonPositiveIntegerDefinition(t, schema, "Union")
	if union.Variety() != SimpleTypeVarietyUnion {
		t.Fatalf("Union variety = %q, want union", union.Variety())
	}
	members := union.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("Union member count = %d, want 2", len(members))
	}
	assertNonPositiveIntegerBuiltinReference(
		t,
		members[0],
		elementReferenceTestAttributeLoc(t, root, `memberTypes="xs:nonPositiveInteger`),
		version,
	)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Union member 1 = %#v, want named Later", members[1])
	}
}

func assertNonPositiveIntegerInlineShapes(t *testing.T, schema Schema, version XSDVersion) {
	t.Helper()
	inlineList := requireNonPositiveIntegerDefinition(t, schema, "InlineList")
	inlineItem, ok := inlineList.ItemType()
	if !ok || !inlineItem.IsAnonymous() {
		t.Fatalf("InlineList item = %#v/%t, want anonymous restriction", inlineItem, ok)
	}
	inlineItemDefinition, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineList item anonymous definition is missing")
	}
	assertNonPositiveIntegerDefinition(t, inlineItemDefinition, version, "0")
	inlineBase, ok := inlineItemDefinition.BaseReference()
	if !ok || !inlineBase.IsBuiltin() || inlineBase.Name().Local() != "nonPositiveInteger" || inlineBase.Loc().IsZero() {
		t.Fatalf("InlineList anonymous base = %#v/%t, want located built-in nonPositiveInteger", inlineBase, ok)
	}

	inlineUnion := requireNonPositiveIntegerDefinition(t, schema, "InlineUnion")
	inlineMembers := inlineUnion.MemberTypes()
	if len(inlineMembers) != 1 || !inlineMembers[0].IsAnonymous() {
		t.Fatalf("InlineUnion members = %#v, want one anonymous restriction", inlineMembers)
	}
	inlineMemberDefinition, ok := inlineMembers[0].AnonymousType()
	if !ok {
		t.Fatal("InlineUnion anonymous member definition is missing")
	}
	assertNonPositiveIntegerDefinition(t, inlineMemberDefinition, version, "0")
}

func requireNonPositiveIntegerElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
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

func requireNonPositiveIntegerDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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
func assertNonPositiveIntegerBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	if !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "nonPositiveInteger") || reference.QName() != reference.Name() {
		t.Fatalf("reference = %#v, want built-in xs:nonPositiveInteger", reference)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc {
		t.Fatalf("reference locations = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), wantLoc)
	}
	if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicNonPositiveInteger {
		t.Fatalf("reference variety/category = %q/%v, want atomic nonPositiveInteger", reference.Variety(), reference.facts)
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
		t.Fatal("built-in nonPositiveInteger unexpectedly has totalDigits")
	}
	if _, hasMinInclusive := facets.integerBounds.MinInclusive(); hasMinInclusive {
		t.Fatal("built-in nonPositiveInteger unexpectedly has minInclusive")
	}
	if _, hasMinExclusive := facets.integerBounds.MinExclusive(); hasMinExclusive {
		t.Fatal("built-in nonPositiveInteger unexpectedly has minExclusive")
	}
	maximum, present := facets.integerBounds.MaxInclusive()
	if !present || maximum.Canonical() != "0" {
		t.Fatalf("effective maxInclusive = %q/%t, want 0/true", maximum.Canonical(), present)
	}
	maximumFacet, present := facets.integerBounds.MaxInclusiveFacet()
	if !present || maximumFacet.Kind() != BoundMaxInclusive || maximumFacet.Value().Canonical() != "0" || !maximumFacet.Loc().IsZero() || maximumFacet.Version() != version {
		t.Fatalf("effective maxInclusive facts = %q/%s/%q/%t, want zero/zero-loc/%q/true", maximumFacet.Value().Canonical(), maximumFacet.Loc(), maximumFacet.Kind(), present, version)
	}
	mutatedMaximum, present := facets.integerBounds.MaxInclusive()
	if !present {
		t.Fatal("built-in reference has no effective maxInclusive")
	}
	_ = mutatedMaximum.value.SetInt64(1)
	storedMaximum, present := facets.integerBounds.MaxInclusive()
	if !present || storedMaximum.Canonical() != "0" {
		t.Fatalf("mutating returned maxInclusive changed stored bound to %q", storedMaximum.Canonical())
	}
}

func assertNonPositiveIntegerReferenceFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, wantMaximum string) {
	t.Helper()
	if facts == nil || facts.atomicKind != schemaSimpleTypeAtomicNonPositiveInteger {
		t.Fatalf("reference facts = %#v, want nonPositiveInteger facts", facts)
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
	maximum, present := bounds.MaxInclusive()
	if !present || maximum.Canonical() != wantMaximum {
		t.Fatalf("effective maxInclusive = %q/%t, want %s/true", maximum.Canonical(), present, wantMaximum)
	}
	if _, present := bounds.MinInclusive(); present {
		t.Fatal("nonPositiveInteger reference unexpectedly has minInclusive")
	}
}

func assertNonPositiveIntegerDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMaximum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicNonPositiveInteger {
		t.Fatalf("definition variety/category = %q/%v, want atomic nonPositiveInteger", definition.Variety(), definition.facts)
	}
	assertNonPositiveIntegerReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, wantMaximum)
}

func TestSchemaNonPositiveIntegerComposesGraphs(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := nonPositiveIntegerGraphFixtures(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			if got := len(schema.Documents()); got != 4 {
				t.Fatalf("document count = %d, want 4 after repeated/cyclic graph discovery", got)
			}
			for _, test := range nonPositiveIntegerGraphElementCases() {
				assertNonPositiveIntegerGraphElement(t, schema, root, fixtures, test, profile.version)
			}
		})
	}
}

func nonPositiveIntegerGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="xs:nonPositiveInteger"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="xs:nonPositiveInteger"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="chameleon" type="xs:nonPositiveInteger"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="xs:nonPositiveInteger"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func nonPositiveIntegerGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="xs:nonPositiveInteger"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="xs:nonPositiveInteger"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="xs:nonPositiveInteger"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="xs:nonPositiveInteger"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertNonPositiveIntegerGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requireNonPositiveIntegerElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		assertNonPositiveIntegerReferenceFacts(t, reference.facts, version, "0")
		return
	}
	assertNonPositiveIntegerBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
}

func TestSchemaNonPositiveIntegerRejectsOutOfBaseRestrictions(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name  string
			body  string
			cause error
		}{
			{name: "positive inclusive bound", body: `<xs:maxInclusive value="1"/>`, cause: errInvalidBoundRestriction},
			{name: "loosened exclusive bound", body: `<xs:maxExclusive value="1"/>`, cause: errInvalidBoundRestriction},
			{name: "positive enumeration", body: `<xs:enumeration value="1"/>`, cause: errInvalidEnumerationRestriction},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:simpleType name="Bad"><xs:restriction base="xs:nonPositiveInteger">` + test.body + `</xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNonPositiveIntegerNoPartialSchema(t, schema, err)
				if !errors.Is(err, test.cause) {
					t.Fatalf("error = %v, want cause %v", err, test.cause)
				}
			})
		}
	}
}

func TestSchemaNonPositiveIntegerAcceptsPositiveTotalDigits(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="t:Limited"/><xs:simpleType name="Limited"><xs:restriction base="xs:nonPositiveInteger"><xs:totalDigits value="9"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			definition := requireNonPositiveIntegerDefinition(t, schema, "Limited")
			total, present := definition.DigitFacets().TotalDigits()
			if !present || total.Canonical() != "9" {
				t.Fatalf("totalDigits = %q/%t, want 9/true", total.Canonical(), present)
			}
		})
	}
}

func TestSchemaNonPositiveIntegerRejectsMalformedUnresolvedWrongKindAndElementRefs(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name  string
			root  string
			cause error
		}{
			{
				name:  "malformed bound",
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:nonPositiveInteger"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType></xs:schema>`,
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
				root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Root"><xs:sequence><xs:element ref="xs:nonPositiveInteger"/></xs:sequence></xs:complexType></xs:schema>`,
				cause: errSchemaElementReferenceUnresolved,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				assertNonPositiveIntegerNoPartialSchema(t, schema, err)
				if !errors.Is(err, test.cause) {
					t.Fatalf("error = %v, want cause %v", err, test.cause)
				}
			})
		}
	}
}

func assertNonPositiveIntegerNoPartialSchema(t *testing.T, schema Schema, err error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located error and no partial schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
}

func assertSchemaIntegerDerivedExcludedShapes(t *testing.T, policy LanguagePolicy, atomicName, defaultValue string) {
	t.Helper()
	for _, test := range []struct {
		name string
		root string
	}{
		{
			name: "local direct particle",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Root"><xs:sequence><xs:element name="item" type="xs:` + atomicName + `"/></xs:sequence></xs:complexType></xs:schema>`,
		},
		{
			name: "local named particle",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:simpleType name="Alias"><xs:restriction base="xs:` + atomicName + `"/></xs:simpleType><xs:complexType name="Root"><xs:sequence><xs:element name="item" type="t:Alias"/></xs:sequence></xs:complexType></xs:schema>`,
		},
		{
			name: "attribute value constraint",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:` + atomicName + `" default="` + defaultValue + `"/></xs:schema>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatalf("discoverTestSchemaWithPolicy accepted an excluded %s shape or returned a schema", atomicName)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic = %s, want located unsupported with preserved cause", diagnostic)
			}
		})
	}
}

func assertSchemaIntegerDerivedGlobalAttributeExcluded(t *testing.T, policy LanguagePolicy, atomicName string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:` + atomicName + `"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("discoverTestSchemaWithPolicy accepted excluded global attribute %q or returned a schema", atomicName)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic = %s, want located unsupported global attribute", diagnostic)
	}
}

func TestSchemaNonPositiveIntegerExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertSchemaIntegerDerivedExcludedShapes(t, profile.policy, "nonPositiveInteger", "0")
			assertSchemaIntegerDerivedGlobalAttributeExcluded(t, profile.policy, "nonPositiveInteger")
		})
	}
}

//nolint:gocognit // Keep generation and validation unsupported outcomes together.
func TestSchemaNonPositiveIntegerConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="xs:nonPositiveInteger"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no source", output, err)
			}
			codegenDiagnostic := requireDiagnostic(t, err)
			if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Code() != diagnosticCodegenUnsupported || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want explicit unsupported", codegenDiagnostic)
			}

			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<value xmlns="urn:test">0</value>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted a nonPositiveInteger global element")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("ValidateInstance diagnostic = %s, want explicit unsupported", validationDiagnostic)
			}
		})
	}
}
