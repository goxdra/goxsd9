package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaByteReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaByteReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated byte builds changed component facts or order")
			}
			assertByteReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaByteReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="p:byte"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="narrowed" type="t:Tight"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:element name="inline"><xs:simpleType><xs:restriction base="p:byte"/></xs:simpleType></xs:element>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="p:byte"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="p:byte"><xs:enumeration value="-128"/><xs:enumeration value="127"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="p:byte"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="p:byte t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="p:byte"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="p:byte"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

//nolint:gocognit,funlen // Check each reference shape against the same immutable graph.
func assertByteReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireByteElement(t, schema, "direct")
	directRef, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct type reference missing")
	}
	assertByteBuiltinReference(t, directRef, elementReferenceTestAttributeLoc(t, root, `type="p:byte"`), version)
	if id, present := direct.TypeID(); present || !id.IsZero() {
		t.Fatalf("direct type ID = %v/%t, want zero", id, present)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "byte")); len(matches) != 0 {
		t.Fatalf("built-in byte became %d components", len(matches))
	}

	later := requireByteDefinition(t, schema, "Later")
	assertByteDefinition(t, later, version, "-100", "100")
	laterBounds, present := later.IntegerBounds()
	if !present {
		t.Fatal("Later has no public integer bounds")
	}
	minimum, hasMinimum := laterBounds.MinInclusiveFacet()
	maximum, hasMaximum := laterBounds.MaxInclusiveFacet()
	if !hasMinimum || !hasMaximum || minimum.Loc() != elementReferenceTestAttributeLoc(t, root, `value="-100"`) || maximum.Loc() != elementReferenceTestAttributeLoc(t, root, `value="100"`) {
		t.Fatalf("Later bound locations = %s/%s, want declared facet locations", minimum.Loc(), maximum.Loc())
	}
	laterBounds.lower.loc = Loc{}
	repeatedBounds, present := later.IntegerBounds()
	repeatedMinimum, hasMinimum := repeatedBounds.MinInclusiveFacet()
	if !present || !hasMinimum || repeatedMinimum.Loc() != minimum.Loc() {
		t.Fatal("mutating public byte bounds changed named definition")
	}
	base, ok := later.BaseReference()
	if !ok {
		t.Fatal("Later base missing")
	}
	assertByteBuiltinReference(t, base, mustSchemaTokenLoc(t, "root.xsd", root, 8, `base="p:byte"`), version)
	for _, test := range []struct{ name, target, min, max string }{
		{"forward", "Later", "-100", "100"},
		{"named", "Derived", "-100", "100"},
		{"narrowed", "Tight", "-100", "2"},
	} {
		element := requireByteElement(t, schema, test.name)
		reference, present := element.TypeReference()
		target := requireByteDefinition(t, schema, test.target)
		id, hasID := reference.ComponentID()
		if !present || !reference.IsNamed() || !hasID || id != target.ID() || reference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:`+test.target+`"`) {
			t.Fatalf("%s reference = %#v/%t, want named %s with use-site Loc", test.name, reference, present, test.target)
		}
		assertIntegerReferenceFacts(t, reference.facts, version, schemaSimpleTypeAtomicByte, "byte", test.min, test.max)
	}
	enumerated := requireByteDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 2 || values[0].Canonical() != "-128" || values[1].Canonical() != "127" {
		t.Fatalf("byte boundary enumeration = %#v", values)
	}
	locations := enumerated.IntegerEnumerationFacets().Locations()
	if len(locations) != 2 || locations[0].IsZero() || locations[1].IsZero() {
		t.Fatalf("enumeration locations = %#v", locations)
	}

	list := requireByteDefinition(t, schema, "List")
	if _, hasBounds := list.IntegerBounds(); hasBounds {
		t.Fatal("byte list inherited atomic item bounds")
	}
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item missing")
	}
	assertByteBuiltinReference(t, item, elementReferenceTestAttributeLoc(t, root, `itemType="p:byte"`), version)
	union := requireByteDefinition(t, schema, "Union")
	if _, hasBounds := union.IntegerBounds(); hasBounds {
		t.Fatal("byte union inherited atomic member bounds")
	}
	members := union.MemberTypes()
	if union.Variety() != SimpleTypeVarietyUnion || len(members) != 2 {
		t.Fatalf("Union variety/members = %q/%d", union.Variety(), len(members))
	}
	assertByteBuiltinReference(t, members[0], elementReferenceTestAttributeLoc(t, root, `memberTypes="p:byte`), version)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("named union member = %#v", members[1])
	}
	memberBounds, hasMemberBounds := members[1].IntegerBounds()
	if !hasMemberBounds {
		t.Fatal("named byte union member lost its own bounds")
	}
	assertIntegerBounds(t, memberBounds, version, "-100", "100")

	inline := requireByteElement(t, schema, "inline")
	inlineRef, ok := inline.TypeReference()
	if !ok || !inlineRef.IsAnonymous() {
		t.Fatalf("inline reference = %#v/%t", inlineRef, ok)
	}
	if id, present := inlineRef.AnonymousID(); !present || id.IsZero() {
		t.Fatalf("inline anonymous ID = %v/%t", id, present)
	}
	anonymous, ok := inlineRef.AnonymousType()
	if !ok {
		t.Fatal("inline anonymous type missing")
	}
	assertByteDefinition(t, anonymous, version, "-128", "127")
	for _, name := range []string{"InlineList", "InlineUnion"} {
		definition := requireByteDefinition(t, schema, name)
		var nested SimpleTypeReference
		if name == "InlineList" {
			nested, ok = definition.ItemType()
		}
		if name == "InlineUnion" {
			members := definition.MemberTypes()
			if len(members) != 1 {
				t.Fatalf("%s members = %d", name, len(members))
			}
			nested, ok = members[0], true
		}
		if !ok || !nested.IsAnonymous() {
			t.Fatalf("%s nested reference = %#v/%t", name, nested, ok)
		}
		nestedDefinition, present := nested.AnonymousType()
		if !present {
			t.Fatalf("%s anonymous type missing", name)
		}
		assertByteDefinition(t, nestedDefinition, version, "-128", "127")
	}
}

func requireByteElement(t *testing.T, schema Schema, local string) ElementDeclaration {
	const namespace = "urn:test"
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

func requireByteDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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

func assertByteBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	assertIntegerBuiltinReference(t, reference, wantLoc, version, schemaSimpleTypeAtomicByte, "byte", "-128", "127")
	bounds, present := reference.IntegerBounds()
	if !present || len(bounds.Bounds()) != 2 {
		t.Fatal("byte reference has no public intrinsic bounds")
	}
	bounds.lower.value.value.SetInt64(0)
	bounds.upper.value.value.SetInt64(0)
	copied, present := reference.IntegerBounds()
	if !present {
		t.Fatal("byte reference lost public bounds after copy mutation")
	}
	assertIntegerBounds(t, copied, version, "-128", "127")
}

func assertByteDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMinimum, wantMaximum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicByte {
		t.Fatalf("definition variety/category = %q/%v, want atomic byte", definition.Variety(), definition.facts)
	}
	assertIntegerReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, schemaSimpleTypeAtomicByte, "byte", wantMinimum, wantMaximum)
}

func TestSchemaByteComposesGraphs(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := byteGraphFixtures(profile.version)
			assertIntegerDerivedComposedGraph(t, root, fixtures, profile, byteGraphElementCases(), schemaSimpleTypeAtomicByte, "byte", "-128", "127")
		})
	}
}

func assertIntegerDerivedComposedGraph(t *testing.T, root string, fixtures map[string]discoveryFixture, profile longPolicyProfile, cases []struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, kind schemaSimpleTypeAtomicKind, name, minimum, maximum string) {
	t.Helper()
	first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
	if err != nil {
		t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
	}
	if !reflect.DeepEqual(first.Components(), second.Components()) || len(first.Documents()) != 4 {
		t.Fatal("repeated graph builds changed component facts, order, or discovery count")
	}
	for _, test := range cases {
		assertIntegerDerivedGraphElement(t, first, root, fixtures, test, profile.version, kind, name, minimum, maximum)
	}
}

func byteGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="p:byte"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="p:byte"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="p:byte"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `"><xs:element name="chameleon" type="p:byte"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="p:byte"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="p:byte"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func byteGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="p:byte"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="p:byte"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="p:byte"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="p:byte"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func assertIntegerDerivedGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion, kind schemaSimpleTypeAtomicKind, kindName, minimum, maximum string) {
	t.Helper()
	declaration := requireShortElement(t, schema, test.local, test.namespace)
	reference, present := declaration.TypeReference()
	if !present {
		t.Fatalf("%s type reference is missing", test.local)
	}
	wantLoc := schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures)
	if reference.Loc() != wantLoc {
		t.Fatalf("%s type Loc = %s, want %s", test.local, reference.Loc(), wantLoc)
	}
	if !test.named {
		assertIntegerBuiltinReference(t, reference, wantLoc, version, kind, kindName, minimum, maximum)
		return
	}
	id, hasID := reference.ComponentID()
	if !reference.IsNamed() || !hasID || id.Source() != test.source {
		t.Fatalf("%s target ID = %v/%t, want named source %s", test.local, id, hasID, test.source)
	}
	assertIntegerReferenceFacts(t, reference.facts, version, kind, kindName, minimum, maximum)
}

func TestSchemaByteConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="p:byte"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertIntegerDerivedConsumersUnsupported(t, schema)
		})
	}
}

//nolint:gocognit // Assert each out-of-range exit against its exact source facet and edition.
func TestSchemaByteBoundaryDiagnostics(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		version := profile.version
		if profile.policy == Compatibility {
			version = XSDVersion11
		}
		for _, test := range []struct {
			name, facet, value string
			code               string
			cause              error
			spec               string
		}{
			{"lower inclusive", "minInclusive", "-129", InvalidBoundRestrictionCode, errInvalidBoundRestriction, boundSpecRef(version, BoundMinInclusive, boundRestrictionRule)},
			{"upper inclusive", "maxInclusive", "128", InvalidBoundRestrictionCode, errInvalidBoundRestriction, boundSpecRef(version, BoundMaxInclusive, boundRestrictionRule)},
			{"lower exclusive", "minExclusive", "-129", InvalidBoundRestrictionCode, errInvalidBoundRestriction, boundSpecRef(version, BoundMinExclusive, boundRestrictionRule)},
			{"upper exclusive", "maxExclusive", "128", InvalidBoundRestrictionCode, errInvalidBoundRestriction, boundSpecRef(version, BoundMaxExclusive, boundRestrictionRule)},
			{"lower enumeration", "enumeration", "-129", InvalidEnumerationRestrictionCode, errInvalidEnumerationRestriction, enumerationSpecRef(version, enumerationRestrictionRule)},
			{"upper enumeration", "enumeration", "128", InvalidEnumerationRestrictionCode, errInvalidEnumerationRestriction, enumerationSpecRef(version, enumerationRestrictionRule)},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:simpleType name="Bad"><xs:restriction base="xs:byte"><xs:` + test.facet + ` value="` + test.value + `"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("schema/error = %#v/%v, want no schema and invalid diagnostic", schema, err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, `value="`+test.value+`"`)
				if test.facet == "enumeration" {
					wantLoc = elementReferenceTestAttributeLoc(t, root, `<xs:enumeration`)
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with %s and %v", diagnostic, test.code, wantLoc, test.spec, test.cause)
				}
			})
		}
	}
}

func TestSchemaByteExcludedAdmissionShapes(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct{ name, body, needle string }{
			{"local direct", `<xs:complexType name="T"><xs:sequence><xs:element name="v" type="xs:byte"/></xs:sequence></xs:complexType>`, `type="xs:byte"`},
			{"local named", `<xs:complexType name="T"><xs:choice><xs:element name="v" type="t:Alias"/></xs:choice></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `type="t:Alias"`},
			{"local inline", `<xs:complexType name="T"><xs:sequence><xs:element name="v"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element></xs:sequence></xs:complexType>`, `<xs:simpleType>`},
			{"global attribute direct", `<xs:attribute name="v" type="xs:byte"/>`, `type="xs:byte"`},
			{"global attribute named", `<xs:attribute name="v" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `type="t:Alias"`},
			{"global attribute inline", `<xs:attribute name="v"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`},
			{"local attribute direct", `<xs:complexType name="T"><xs:attribute name="v" type="xs:byte"/></xs:complexType>`, `type="xs:byte"`},
			{"local attribute named", `<xs:complexType name="T"><xs:attribute name="v" type="t:Alias"/></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `type="t:Alias"`},
			{"local attribute inline", `<xs:complexType name="T"><xs:attribute name="v"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:attribute></xs:complexType>`, `<xs:simpleType>`},
			{"local attribute ref target", `<xs:attribute name="v" type="xs:byte"/><xs:complexType name="T"><xs:attribute ref="t:v"/></xs:complexType>`, `type="xs:byte"`},
			{"simple content direct", `<xs:complexType name="T"><xs:simpleContent><xs:extension base="xs:byte"/></xs:simpleContent></xs:complexType>`, `base="xs:byte"`},
			{"simple content named", `<xs:complexType name="T"><xs:simpleContent><xs:extension base="t:Alias"/></xs:simpleContent></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `base="t:Alias"`},
			{"global element default", `<xs:element name="v" type="xs:byte" default="0"/>`, `default="0"`},
			{"global element fixed named", `<xs:element name="v" type="t:Alias" fixed="0"/><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `fixed="0"`},
			{"global element fixed inline", `<xs:element name="v" fixed="0"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`, `fixed="0"`},
			{"global attribute default", `<xs:attribute name="v" type="xs:byte" default="0"/>`, `type="xs:byte"`},
			{"global attribute fixed named", `<xs:attribute name="v" type="t:Alias" fixed="0"/><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `type="t:Alias"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("schema/error = %#v/%v, want no schema and unsupported", schema, err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.needle)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want unsupported at %s", diagnostic, wantLoc)
				}
			})
		}
	}
}

//nolint:gocognit // Check separate global type shapes and both public consumers.
func TestSchemaByteGlobalConsumersByTypeShape(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct{ name, body, codegenLoc string }{
			{"direct", `<xs:element name="value" type="xs:byte"/>`, `<xs:element name="value"`},
			{"named", `<xs:element name="value" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:element name="value"`},
			{"inline", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`, `<xs:element name="value"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
				}
				declaration := requireByteElement(t, schema, "value")
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil output and unsupported", output, err)
				}
				codegen := requireDiagnostic(t, err)
				if codegen.Class() != FailureUnsupported || codegen.Code() != diagnosticCodegenUnsupported || codegen.Loc() != elementReferenceTestAttributeLoc(t, root, test.codegenLoc) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("GenerateGo diagnostic = %s, want located unsupported", codegen)
				}
				related := []Loc{declaration.Loc()}
				if test.name == "named" {
					related = append(related, elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias"`))
				}
				assertGlobalIntegerDerivedValidationUnsupported(t, schema, "value", related)
			})
		}
	}
}

//nolint:gocognit // Keep reference identity and both consumer failures together.
func TestSchemaByteLocalElementReferenceQueryAndConsumers(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="root" type="t:Record"/><xs:complexType name="Record"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType><xs:element name="value" type="xs:byte"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
			if len(matches) != 1 {
				t.Fatalf("Record definitions = %d, want 1", len(matches))
			}
			definition, ok := matches[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Record complex type view missing")
			}
			choice, ok := definition.Particle().(ChoiceParticle)
			if !ok || len(choice.Alternatives()) != 1 {
				t.Fatalf("Record particle = %T, want one choice alternative", definition.Particle())
			}
			reference, ok := choice.Alternatives()[0].(ElementReferenceParticle)
			target := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "value"))
			if !ok || len(target) != 1 || reference.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="t:value"`) || reference.TargetID() != target[0].ID() {
				t.Fatalf("byte element ref = %#v/%t, want located target ID", reference, ok)
			}
			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("GenerateGo = (%q, %v), want nil unsupported output", output, err)
			}
			codegen := requireDiagnostic(t, err)
			if codegen.Class() != FailureUnsupported || codegen.Code() != diagnosticCodegenUnsupported || codegen.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="t:value"`) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want located unsupported", codegen)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><value>0</value></root>`)))
			if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("ValidateInstance = %v, want unsupported", validationErr)
			}
			validation := requireDiagnostic(t, validationErr)
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) {
				t.Fatalf("ValidateInstance diagnostic = %s, want located unsupported", validation)
			}
		})
	}
}

//nolint:gocognit // Verify zero omission and the preceding syntax gate across policies.
func TestSchemaByteZeroLocalParticleOmitsAfterGates(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:sequence><xs:element name="omitted" type="xs:byte" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Record"))
			if len(matches) != 1 {
				t.Fatalf("Record definitions = %d, want 1", len(matches))
			}
			definition, ok := matches[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Record complex type view missing")
			}
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok || len(sequence.Elements()) != 0 {
				t.Fatalf("Record particle = %#v, want sequence without omitted byte child", definition.Particle())
			}
			invalid := strings.Replace(root, `type="xs:byte"`, `type="bad:q:name"`, 1)
			partial, err := discoverTestSchemaWithPolicy(t, invalid, nil, profile.policy)
			if err == nil || partial.storage != nil {
				t.Fatalf("invalid zero byte particle = %#v/%v, want no schema", partial, err)
			}
			if diagnostic := requireDiagnostic(t, err); diagnostic.Class() != FailureInvalid || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, invalid, `type="bad:q:name"`) {
				t.Fatalf("invalid zero particle diagnostic = %s, want located invalid QName", diagnostic)
			}
		})
	}
}

func TestSchemaByteInheritedBoundDiagnostics(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		version := profile.version
		if profile.policy == Compatibility {
			version = XSDVersion11
		}
		for _, test := range []struct {
			facet, value string
			kind         BoundKind
		}{
			{"minInclusive", "-11", BoundMinInclusive},
			{"maxInclusive", "11", BoundMaxInclusive},
		} {
			t.Run(profile.name+"/"+test.facet, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:simpleType name="Base"><xs:restriction base="xs:byte"><xs:minInclusive value="-10"/><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType><xs:simpleType name="Bad"><xs:restriction base="t:Base"><xs:` + test.facet + ` value="` + test.value + `"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("schema/error = %#v/%v, want no schema and invalid restriction", schema, err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, `value="`+test.value+`"`)
				wantRelated := []Loc{
					elementReferenceTestAttributeLoc(t, root, `value="-10"`),
					elementReferenceTestAttributeLoc(t, root, `value="10"`),
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidBoundRestrictionCode || diagnostic.Loc() != wantLoc || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != boundSpecRef(version, test.kind, boundRestrictionRule) || !errors.Is(err, errInvalidBoundRestriction) {
					t.Fatalf("diagnostic = %s related %v, want bound restriction at %s related %v", diagnostic, diagnostic.Related(), wantLoc, wantRelated)
				}
			})
		}
	}
}
