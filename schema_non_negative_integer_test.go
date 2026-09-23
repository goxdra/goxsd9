package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type nonNegativeIntegerPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func nonNegativeIntegerPolicyProfiles() []nonNegativeIntegerPolicyProfile {
	return []nonNegativeIntegerPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
}

func TestSchemaNonNegativeIntegerReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaNonNegativeIntegerReferenceRoot(profile.version)
			schema := discoverRepeatedNonNegativeIntegerReferenceSchema(t, root, profile.policy)
			assertNonNegativeIntegerReferenceShapes(t, schema, root, profile.version)
		})
	}
}

func discoverRepeatedNonNegativeIntegerReferenceSchema(t *testing.T, root string, policy LanguagePolicy) Schema {
	t.Helper()
	first, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	second, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
	}
	if !reflect.DeepEqual(first.Components(), second.Components()) {
		t.Fatal("repeated nonNegativeInteger builds changed component facts or order")
	}
	return first
}

func assertNonNegativeIntegerReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	assertNonNegativeIntegerDirectReference(t, schema, root, version)
	assertNonNegativeIntegerNamedReferences(t, schema, root, version)
	assertNonNegativeIntegerCollectionReferences(t, schema, root, version)
	assertNonNegativeIntegerInlineReferences(t, schema, version)
}

func assertNonNegativeIntegerDirectReference(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireNonNegativeIntegerElement(t, schema, "direct", "urn:test")
	directReference, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct element type reference is missing")
	}
	assertNonNegativeIntegerBuiltinReference(
		t,
		directReference,
		elementReferenceTestAttributeLoc(t, root, `type="xs:nonNegativeInteger"`),
		version,
	)
	if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("direct element type ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "nonNegativeInteger")); len(matches) != 0 {
		t.Fatalf("built-in nonNegativeInteger is visible as %d named component(s)", len(matches))
	}
}

func assertNonNegativeIntegerNamedReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	forward := requireNonNegativeIntegerElement(t, schema, "forward", "urn:test")
	forwardReference, ok := forward.TypeReference()
	if !ok || !forwardReference.IsNamed() || forwardReference.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("forward element type reference = %#v/%t, want named Later", forwardReference, ok)
	}
	forwardID, forwardIDOK := forwardReference.ComponentID()
	later := requireNonNegativeIntegerDefinition(t, schema, "Later")
	if !forwardIDOK || forwardID != later.ID() {
		t.Fatalf("forward type ID = %v/%t, want Later %v/true", forwardID, forwardIDOK, later.ID())
	}
	assertNonNegativeIntegerReferenceFacts(t, forwardReference.facts, version)
	if forwardReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:Later"`) {
		t.Fatalf("forward reference location = %s, want type attribute location", forwardReference.Loc())
	}

	derived := requireNonNegativeIntegerDefinition(t, schema, "Derived")
	assertNonNegativeIntegerDefinition(t, derived, version)
	base, ok := derived.BaseReference()
	if !ok || !base.IsNamed() || base.Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Derived base reference = %#v/%t, want named Later", base, ok)
	}
	if base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="t:Later"`) {
		t.Fatalf("Derived base location = %s, want base attribute location", base.Loc())
	}

	laterValues := later.IntegerEnumerationFacets().Values()
	if len(laterValues) != 1 || laterValues[0].Canonical() != "0" {
		t.Fatalf("Later enumeration values = %#v, want canonical zero from -0", laterValues)
	}
	if locations := later.IntegerEnumerationFacets().Locations(); len(locations) != 1 || locations[0].IsZero() {
		t.Fatalf("Later enumeration locations = %#v, want one source location", locations)
	}
}

func assertNonNegativeIntegerCollectionReferences(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	list := requireNonNegativeIntegerDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item type is missing")
	}
	assertNonNegativeIntegerBuiltinReference(
		t,
		item,
		elementReferenceTestAttributeLoc(t, root, `itemType="xs:nonNegativeInteger"`),
		version,
	)

	union := requireNonNegativeIntegerDefinition(t, schema, "Union")
	members := union.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("Union member count = %d, want 2", len(members))
	}
	assertNonNegativeIntegerBuiltinReference(
		t,
		members[0],
		elementReferenceTestAttributeLoc(t, root, `memberTypes="xs:nonNegativeInteger`),
		version,
	)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("Union member 1 = %#v, want named Later", members[1])
	}
	assertNonNegativeIntegerReferenceFacts(t, members[1].facts, version)
}

func assertNonNegativeIntegerInlineReferences(t *testing.T, schema Schema, version XSDVersion) {
	t.Helper()
	inlineList := requireNonNegativeIntegerDefinition(t, schema, "InlineList")
	inlineItem, ok := inlineList.ItemType()
	if !ok || !inlineItem.IsAnonymous() {
		t.Fatalf("InlineList item = %#v/%t, want anonymous restriction", inlineItem, ok)
	}
	inlineItemDefinition, ok := inlineItem.AnonymousType()
	if !ok {
		t.Fatal("InlineList item anonymous definition is missing")
	}
	assertNonNegativeIntegerDefinition(t, inlineItemDefinition, version)
	inlineBase, ok := inlineItemDefinition.BaseReference()
	if !ok || !inlineBase.IsBuiltin() || inlineBase.Name().Local() != "nonNegativeInteger" || inlineBase.Loc().IsZero() {
		t.Fatalf("InlineList anonymous base = %#v/%t, want located built-in nonNegativeInteger", inlineBase, ok)
	}

	inlineUnion := requireNonNegativeIntegerDefinition(t, schema, "InlineUnion")
	inlineMembers := inlineUnion.MemberTypes()
	if len(inlineMembers) != 1 || !inlineMembers[0].IsAnonymous() {
		t.Fatalf("InlineUnion members = %#v, want one anonymous restriction", inlineMembers)
	}
	inlineMemberDefinition, ok := inlineMembers[0].AnonymousType()
	if !ok {
		t.Fatal("InlineUnion anonymous member definition is missing")
	}
	assertNonNegativeIntegerDefinition(t, inlineMemberDefinition, version)
}

func schemaNonNegativeIntegerReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="xs:nonNegativeInteger"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:nonNegativeInteger"><xs:enumeration value="-0"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:nonNegativeInteger t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

func requireNonNegativeIntegerElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
	t.Helper()
	return requireSchemaElementDeclaration(t, schema, local, namespace)
}

func requireSchemaElementDeclaration(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
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

func requireNonNegativeIntegerDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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

func assertNonNegativeIntegerBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	if !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "nonNegativeInteger") || reference.QName() != reference.Name() {
		t.Fatalf("reference = %#v, want built-in xs:nonNegativeInteger", reference)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc {
		t.Fatalf("reference locations = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), wantLoc)
	}
	if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicNonNegativeInteger {
		t.Fatalf("reference variety/category = %q/%v, want atomic nonNegativeInteger", reference.Variety(), reference.facts)
	}
	if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
		t.Fatalf("built-in reference component ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
	assertNonNegativeIntegerReferenceFacts(t, reference.facts, version)
	facets, ok := reference.facts.facets.(schemaDigitFacetVariant)
	if !ok {
		t.Fatalf("built-in reference facets = %T, want fresh digit facts", reference.facts.facets)
	}
	mutatedMinimum, present := facets.integerBounds.MinInclusive()
	if !present {
		t.Fatal("built-in reference has no effective minInclusive")
	}
	_ = mutatedMinimum.value.SetInt64(-1)
	storedMinimum, present := facets.integerBounds.MinInclusive()
	if !present || storedMinimum.Canonical() != "0" {
		t.Fatalf("mutating returned minInclusive changed stored bound to %q", storedMinimum.Canonical())
	}
}

func assertNonNegativeIntegerDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicNonNegativeInteger {
		t.Fatalf("definition variety/category = %q/%v, want atomic nonNegativeInteger", definition.Variety(), definition.facts)
	}
	assertNonNegativeIntegerComponentFacts(t, definition.facts, version)
}

func assertNonNegativeIntegerReferenceFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion) {
	t.Helper()
	if facts == nil {
		t.Fatal("reference facts are nil")
	}
	assertNonNegativeIntegerFacetFacts(t, facts.atomicKind, facts.facets, version)
}

func assertNonNegativeIntegerComponentFacts(t *testing.T, facts *schemaSimpleTypeComponent, version XSDVersion) {
	t.Helper()
	if facts == nil || facts.atomicKind != schemaSimpleTypeAtomicNonNegativeInteger {
		t.Fatalf("facts = %#v, want nonNegativeInteger facts", facts)
	}
	assertNonNegativeIntegerFacetFacts(t, facts.atomicKind, facts.facets, version)
}

//nolint:gocognit // Keep canonical digit facts and exact bounds together.
func assertNonNegativeIntegerFacetFacts(t *testing.T, atomicKind schemaSimpleTypeAtomicKind, facetFacts schemaSimpleTypeFacetVariant, version XSDVersion) {
	t.Helper()
	if atomicKind != schemaSimpleTypeAtomicNonNegativeInteger {
		t.Fatalf("atomic kind = %v, want nonNegativeInteger", atomicKind)
	}
	var digits DigitFacets
	var bounds IntegerBoundFacets
	switch facets := facetFacts.(type) {
	case schemaDigitFacetVariant:
		if facets.value.Kind() != DigitDatatypeInteger || facets.value.Version() != version {
			t.Fatalf("digit facts = %q/%q, want integer/%q", facets.value.Kind(), facets.value.Version(), version)
		}
		digits = facets.value
		bounds = facets.integerBounds
	case schemaIntegerFacetVariant:
		if facets.digits.Kind() != DigitDatatypeInteger || facets.digits.Version() != version {
			t.Fatalf("integer facts = %q/%q, want integer/%q", facets.digits.Kind(), facets.digits.Version(), version)
		}
		digits = facets.digits
		bounds = facets.bounds
	default:
		t.Fatalf("facts facets = %T, want integer facts", facetFacts)
	}
	if total, present := digits.TotalDigits(); present || !total.IsZero() {
		t.Fatalf("effective totalDigits = %q/%t, want absent", total.Canonical(), present)
	}
	fraction, present := digits.FractionDigits()
	if !present || fraction.Canonical() != "0" {
		t.Fatalf("effective fractionDigits = %q/%t, want 0/true", fraction.Canonical(), present)
	}
	fractionFixed, fixedPresent := digits.FractionDigitsFixed()
	if !fixedPresent || !fractionFixed {
		t.Fatalf("effective fractionDigits fixed = %t/%t, want true/true", fractionFixed, fixedPresent)
	}
	minimum, present := bounds.MinInclusive()
	if !present || minimum.Canonical() != "0" {
		t.Fatalf("effective minInclusive = %q/%t, want 0/true", minimum.Canonical(), present)
	}
	minimumFacet, present := bounds.MinInclusiveFacet()
	if !present || minimumFacet.Kind() != BoundMinInclusive || minimumFacet.Value().Canonical() != "0" || !minimumFacet.Loc().IsZero() || minimumFacet.Version() != version {
		t.Fatalf("effective minInclusive facts = %q/%s/%q/%t, want zero/zero-loc/%q/true", minimumFacet.Value().Canonical(), minimumFacet.Loc(), minimumFacet.Kind(), present, version)
	}
	if _, present := bounds.MaxInclusive(); present {
		t.Fatal("nonNegativeInteger unexpectedly has an effective upper bound")
	}
}

func TestSchemaNonNegativeIntegerReferencesComposeGraphs(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := nonNegativeIntegerGraphFixtures(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertNonNegativeIntegerGraphSchema(t, schema, root, fixtures, profile.version)
		})
	}
}

func nonNegativeIntegerGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="xs:nonNegativeInteger"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {
			id:       "root.xsd",
			contents: root,
		},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="xs:nonNegativeInteger"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="chameleon" type="xs:nonNegativeInteger"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="xs:nonNegativeInteger"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func assertNonNegativeIntegerGraphSchema(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, version XSDVersion) {
	t.Helper()
	if got := len(schema.Documents()); got != 4 {
		t.Fatalf("document count = %d, want 4 after repeated/cyclic graph discovery", got)
	}
	for _, test := range nonNegativeIntegerGraphElementCases() {
		assertNonNegativeIntegerGraphElement(t, schema, root, fixtures, test, version)
	}
}

func nonNegativeIntegerGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="xs:nonNegativeInteger"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="xs:nonNegativeInteger"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="xs:nonNegativeInteger"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="xs:nonNegativeInteger"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertNonNegativeIntegerGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requireNonNegativeIntegerElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		assertNonNegativeIntegerReferenceFacts(t, reference.facts, version)
		return
	}
	assertNonNegativeIntegerBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
}

func TestSchemaNonNegativeIntegerRejectsNegativeRestrictions(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		for _, test := range []struct {
			name string
			body string
		}{
			{name: "negative bound", body: `<xs:minInclusive value="-1"/>`},
			{name: "negative enumeration", body: `<xs:enumeration value="-1"/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:simpleType name="Bad"><xs:restriction base="xs:nonNegativeInteger">` + test.body + `</xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("discoverTestSchemaWithPolicy accepted a negative nonNegativeInteger restriction or returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
					t.Fatalf("diagnostic = %s, want located invalid restriction", diagnostic)
				}
			})
		}
	}
}

func TestSchemaNonNegativeIntegerExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertSchemaIntegerDerivedExcludedShapes(t, profile.policy, "nonNegativeInteger", "-0")
		})
	}
}

func TestSchemaNonNegativeIntegerValidationRemainsUnsupported(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		for _, test := range []struct {
			name          string
			rootElement   string
			body          string
			namedRootType bool
		}{
			{
				name:        "built-in root",
				rootElement: "value",
				body:        `<xs:element name="value" type="xs:nonNegativeInteger"/>`,
			},
			{
				name:          "named root",
				rootElement:   "named",
				body:          `<xs:element name="named" type="t:Named"/><xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`,
				namedRootType: true,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
				}
				declaration := requireSchemaElementDeclaration(t, schema, test.rootElement, "urn:test")
				wantRelated := []Loc{declaration.Loc()}
				if test.namedRootType {
					definition := requireNonNegativeIntegerDefinition(t, schema, "Named")
					wantRelated = append(wantRelated, definition.Loc())
				}
				assertNonNegativeIntegerValidationUnsupported(t, schema, test.rootElement, wantRelated)
			})
		}
	}
}

//nolint:gocognit // Keep the query-only inline model and both consumer boundaries together.
func TestSchemaNonNegativeIntegerGlobalInlineConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:element name="value"><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:element>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}

			declaration := requireNonNegativeIntegerElement(t, schema, "value", "urn:test")
			wantDeclarationLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element name="value"`)
			wantTypeLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:simpleType>")
			wantVarietyLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:restriction")
			if declaration.Loc() != wantDeclarationLoc || !declaration.DeclaredType().IsZero() {
				t.Fatalf("declaration location/type = %s/%q, want %s/zero", declaration.Loc(), declaration.DeclaredType(), wantDeclarationLoc)
			}
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsAnonymous() || reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantVarietyLoc {
				t.Fatalf("type reference = %#v/%t, want anonymous reference with exact locations", reference, ok)
			}
			anonymous, ok := reference.AnonymousType()
			if !ok {
				t.Fatal("anonymous nonNegativeInteger definition is missing")
			}
			if anonymous.Loc() != wantTypeLoc || anonymous.VarietyLoc() != wantVarietyLoc || !anonymous.IsAnonymous() || !anonymous.ID().IsZero() {
				t.Fatalf("anonymous definition ownership/locations = %s/%s/%t/%v, want exact query-only model", anonymous.Loc(), anonymous.VarietyLoc(), anonymous.IsAnonymous(), anonymous.ID())
			}
			assertNonNegativeIntegerDefinition(t, anonymous, profile.version)
			base, ok := anonymous.BaseReference()
			if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "nonNegativeInteger") {
				t.Fatalf("anonymous base reference = %#v/%t, want built-in nonNegativeInteger", base, ok)
			}

			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", output, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || generationDiagnostic.Loc() != wantDeclarationLoc || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want unsupported at %s with preserved cause", generationDiagnostic, wantDeclarationLoc)
			}
			if !reflect.DeepEqual(generationDiagnostic.Related(), []Loc{wantTypeLoc}) {
				t.Fatalf("GenerateGo related locations = %v, want [%s]", generationDiagnostic.Related(), wantTypeLoc)
			}

			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<value xmlns="urn:test">0</value>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted a global inline nonNegativeInteger target")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || validationDiagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceNoDeclaredType) {
				t.Fatalf("ValidateInstance diagnostic = %s, want unsupported at instance root with preserved cause", validationDiagnostic)
			}
			if !reflect.DeepEqual(validationDiagnostic.Related(), []Loc{wantDeclarationLoc}) {
				t.Fatalf("ValidateInstance related locations = %v, want [%s]", validationDiagnostic.Related(), wantDeclarationLoc)
			}
		})
	}
}

func assertIntegerDerivedConsumersUnsupported(t *testing.T, schema Schema) {
	t.Helper()
	output, err := GenerateGo(schema, "generated")
	if output != nil || err == nil {
		t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no source", output, err)
	}
	codegenDiagnostic := requireDiagnostic(t, err)
	if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Code() != diagnosticCodegenUnsupported || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("GenerateGo diagnostic = %s, want explicit unsupported", codegenDiagnostic)
	}
	declaration := requireSchemaElementDeclaration(t, schema, "value", "urn:test")
	assertIntegerDerivedValidationUnsupported(t, schema, declaration)
}

func assertIntegerDerivedValidationUnsupported(t *testing.T, schema Schema, declaration ElementDeclaration) {
	t.Helper()
	assertGlobalIntegerDerivedValidationUnsupported(t, schema, "value", []Loc{declaration.Loc()})
}

func assertNonNegativeIntegerValidationUnsupported(t *testing.T, schema Schema, rootElement string, wantRelated []Loc) {
	t.Helper()
	assertGlobalIntegerDerivedValidationUnsupported(t, schema, rootElement, wantRelated)
}

func assertGlobalIntegerDerivedValidationUnsupported(t *testing.T, schema Schema, rootElement string, wantRelated []Loc) {
	t.Helper()
	input := `<` + rootElement + ` xmlns="urn:test">0</` + rootElement + `>`
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	if validationErr == nil {
		t.Fatalf("ValidateInstance accepted an excluded global element %q", rootElement)
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
		t.Fatalf("ValidateInstance diagnostic = %s, want explicit unsupported instance validation", validationDiagnostic)
	}
	wantLoc := mustTestLoc(t, "instance.xml", 1, 1)
	if validationDiagnostic.Loc() != wantLoc {
		t.Fatalf("ValidateInstance diagnostic location = %s, want %s", validationDiagnostic.Loc(), wantLoc)
	}
	if !reflect.DeepEqual(validationDiagnostic.Related(), wantRelated) {
		t.Fatalf("ValidateInstance related locations = %v, want %v", validationDiagnostic.Related(), wantRelated)
	}
}

//nolint:gocognit // Keep all-policy stale-fact mutations and invariant checks together.
func TestCodegenNonNegativeIntegerRejectsNonCanonicalBuiltinFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *schemaSimpleTypeFacetVariant, XSDVersion)
	}{
		{
			name: "maxInclusive",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
				mutateCodegenNonNegativeIntegerUpperBound(t, facets, BoundMaxInclusive, version)
			},
		},
		{
			name: "maxExclusive",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
				mutateCodegenNonNegativeIntegerUpperBound(t, facets, BoundMaxExclusive, version)
			},
		},
		{
			name: "wrong facet variant",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, _ XSDVersion) {
				mutateCodegenNonNegativeIntegerFacetVariant(t, facets)
			},
		},
		{
			name: "totalDigits",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
				mutateCodegenNonNegativeIntegerTotalDigits(t, facets, version)
			},
		},
		{
			name: "nonzero fractionDigits",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
				mutateCodegenNonNegativeIntegerFractionDigits(t, facets, version, true)
			},
		},
		{
			name: "nonfixed fractionDigits",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
				mutateCodegenNonNegativeIntegerFractionDigits(t, facets, version, false)
			},
		},
		{
			name: "incompatible digit kind",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, _ XSDVersion) {
				mutateCodegenNonNegativeIntegerDigitKind(t, facets)
			},
		},
		{
			name: "incompatible digit version",
			mutate: func(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
				mutateCodegenNonNegativeIntegerDigitVersion(t, facets, version)
			},
		},
	}
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="xs:nonNegativeInteger"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
				}
				plan, err := planCodegenSource(schema, mustScalarCodegenNaming(t, schema))
				if err != nil {
					t.Fatalf("planCodegenSource: %v", err)
				}
				components := schema.Components()
				test.mutate(t, &components[0].element.typeReference.facets, profile.version)
				output, err := renderCodegenSource(plan, schema)
				if output != nil || err == nil {
					t.Fatalf("non-canonical nonNegativeInteger result = (%q, %v), want nil output and internal error", output, err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" || !errors.Is(err, errCodegenSchemaInvariant) {
					t.Fatalf("diagnostic = %s, want located internal codegen invariant with preserved cause", diagnostic)
				}
			})
		}
	}
}

func mutateCodegenNonNegativeIntegerUpperBound(t *testing.T, facets *schemaSimpleTypeFacetVariant, upperKind BoundKind, version XSDVersion) {
	t.Helper()
	digitFacets, ok := (*facets).(schemaDigitFacetVariant)
	if !ok {
		t.Fatalf("nonNegativeInteger facets = %T, want digit facts", *facets)
	}
	minimum, err := ParseIntegerMinInclusiveFacet("0", mustTestLoc(t, "root.xsd", 1, 1), version)
	if err != nil {
		t.Fatalf("ParseIntegerMinInclusiveFacet: %v", err)
	}
	var maximum IntegerBoundFacet
	switch upperKind {
	case BoundMaxInclusive:
		maximum, err = ParseIntegerMaxInclusiveFacet("1", mustTestLoc(t, "root.xsd", 1, 2), version)
	case BoundMaxExclusive:
		maximum, err = ParseIntegerMaxExclusiveFacet("1", mustTestLoc(t, "root.xsd", 1, 2), version)
	case BoundMinInclusive, BoundMinExclusive:
		t.Fatalf("upper bound kind = %q, want max bound", upperKind)
	}
	if err != nil {
		t.Fatalf("parse upper bound: %v", err)
	}
	bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum, maximum}, version)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets: %v", err)
	}
	digitFacets.integerBounds = bounds
	*facets = digitFacets
}

func mutateCodegenNonNegativeIntegerTotalDigits(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
	t.Helper()
	digitFacets := requireCodegenNonNegativeIntegerDigitFacets(t, facets)
	total, err := ParseTotalDigitsFacetFor(version, "1", mustTestLoc(t, "root.xsd", 1, 3))
	if err != nil {
		t.Fatalf("ParseTotalDigitsFacetFor: %v", err)
	}
	digitFacets.value.totalDigits = &total
	*facets = digitFacets
}

func mutateCodegenNonNegativeIntegerFractionDigits(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion, nonzero bool) {
	t.Helper()
	digitFacets := requireCodegenNonNegativeIntegerDigitFacets(t, facets)
	fraction, err := ParseFractionDigitsFacetFor(version, "0", mustTestLoc(t, "root.xsd", 1, 3))
	if err != nil {
		t.Fatalf("ParseFractionDigitsFacetFor: %v", err)
	}
	if nonzero {
		fraction, err = ParseFractionDigitsFacetFor(version, "1", mustTestLoc(t, "root.xsd", 1, 3))
		if err != nil {
			t.Fatalf("ParseFractionDigitsFacetFor nonzero: %v", err)
		}
	}
	if !nonzero {
		fraction.fixed = false
	}
	digitFacets.value.fractionDigits = &fraction
	*facets = digitFacets
}

func mutateCodegenNonNegativeIntegerDigitKind(t *testing.T, facets *schemaSimpleTypeFacetVariant) {
	t.Helper()
	digitFacets := requireCodegenNonNegativeIntegerDigitFacets(t, facets)
	digitFacets.value.kind = DigitDatatypeDecimal
	*facets = digitFacets
}

func mutateCodegenNonNegativeIntegerDigitVersion(t *testing.T, facets *schemaSimpleTypeFacetVariant, version XSDVersion) {
	t.Helper()
	digitFacets := requireCodegenNonNegativeIntegerDigitFacets(t, facets)
	if version == XSDVersion10 {
		digitFacets.value.version = XSDVersion11
		*facets = digitFacets
		return
	}
	digitFacets.value.version = XSDVersion10
	*facets = digitFacets
}

func requireCodegenNonNegativeIntegerDigitFacets(t *testing.T, facets *schemaSimpleTypeFacetVariant) schemaDigitFacetVariant {
	t.Helper()
	digitFacets, ok := (*facets).(schemaDigitFacetVariant)
	if !ok {
		t.Fatalf("nonNegativeInteger facets = %T, want digit facts", *facets)
	}
	return digitFacets
}

func mutateCodegenNonNegativeIntegerFacetVariant(t *testing.T, facets *schemaSimpleTypeFacetVariant) {
	t.Helper()
	digitFacets, ok := (*facets).(schemaDigitFacetVariant)
	if !ok {
		t.Fatalf("nonNegativeInteger facets = %T, want digit facts", *facets)
	}
	*facets = schemaIntegerFacetVariant{digits: digitFacets.value, bounds: digitFacets.integerBounds}
}
