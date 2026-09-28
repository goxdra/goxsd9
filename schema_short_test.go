package goxsd9

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaShortReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaShortReferenceRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated short builds changed component facts or order")
			}
			assertShortReferenceShapes(t, first, root, profile.version)
		})
	}
}

func schemaShortReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="direct" type="p:short"/>
  <xs:element name="forward" type="t:Later"/>
  <xs:element name="narrowed" type="t:Tight"/>
  <xs:element name="named" type="t:Derived"/>
  <xs:element name="inline"><xs:simpleType><xs:restriction base="p:short"/></xs:simpleType></xs:element>
  <xs:simpleType name="Derived"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="p:short"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Later"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="p:short"><xs:enumeration value="-32768"/><xs:enumeration value="32767"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="p:short"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="p:short t:Later"/></xs:simpleType>
  <xs:simpleType name="InlineList"><xs:list><xs:simpleType><xs:restriction base="p:short"/></xs:simpleType></xs:list></xs:simpleType>
  <xs:simpleType name="InlineUnion"><xs:union><xs:simpleType><xs:restriction base="p:short"/></xs:simpleType></xs:union></xs:simpleType>
</xs:schema>`
}

//nolint:gocognit,funlen // Check each reference shape against the same immutable graph.
func assertShortReferenceShapes(t *testing.T, schema Schema, root string, version XSDVersion) {
	t.Helper()
	direct := requireShortElement(t, schema, "direct", "urn:test")
	directRef, ok := direct.TypeReference()
	if !ok {
		t.Fatal("direct type reference missing")
	}
	assertShortBuiltinReference(t, directRef, elementReferenceTestAttributeLoc(t, root, `type="p:short"`), version)
	if id, present := direct.TypeID(); present || !id.IsZero() {
		t.Fatalf("direct type ID = %v/%t, want zero", id, present)
	}
	if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "short")); len(matches) != 0 {
		t.Fatalf("built-in short became %d components", len(matches))
	}

	later := requireShortDefinition(t, schema, "Later")
	assertShortDefinition(t, later, version, "-100", "100")
	base, ok := later.BaseReference()
	if !ok {
		t.Fatal("Later base missing")
	}
	assertShortBuiltinReference(t, base, mustSchemaTokenLoc(t, "root.xsd", root, 8, `base="p:short"`), version)
	for _, test := range []struct{ name, target, min, max string }{
		{"forward", "Later", "-100", "100"},
		{"named", "Derived", "-100", "100"},
		{"narrowed", "Tight", "-100", "2"},
	} {
		element := requireShortElement(t, schema, test.name, "urn:test")
		reference, present := element.TypeReference()
		target := requireShortDefinition(t, schema, test.target)
		id, hasID := reference.ComponentID()
		if !present || !reference.IsNamed() || !hasID || id != target.ID() || reference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:`+test.target+`"`) {
			t.Fatalf("%s reference = %#v/%t, want named %s with use-site Loc", test.name, reference, present, test.target)
		}
		assertIntegerReferenceFacts(t, reference.facts, version, schemaSimpleTypeAtomicShort, "short", test.min, test.max)
	}
	enumerated := requireShortDefinition(t, schema, "Enumerated")
	values := enumerated.IntegerEnumerationFacets().Values()
	if len(values) != 2 || values[0].Canonical() != "-32768" || values[1].Canonical() != "32767" {
		t.Fatalf("short boundary enumeration = %#v", values)
	}
	locations := enumerated.IntegerEnumerationFacets().Locations()
	if len(locations) != 2 || locations[0].IsZero() || locations[1].IsZero() {
		t.Fatalf("enumeration locations = %#v", locations)
	}

	list := requireShortDefinition(t, schema, "List")
	item, ok := list.ItemType()
	if !ok {
		t.Fatal("List item missing")
	}
	assertShortBuiltinReference(t, item, elementReferenceTestAttributeLoc(t, root, `itemType="p:short"`), version)
	union := requireShortDefinition(t, schema, "Union")
	members := union.MemberTypes()
	if union.Variety() != SimpleTypeVarietyUnion || len(members) != 2 {
		t.Fatalf("Union variety/members = %q/%d", union.Variety(), len(members))
	}
	assertShortBuiltinReference(t, members[0], elementReferenceTestAttributeLoc(t, root, `memberTypes="p:short`), version)
	if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Later") {
		t.Fatalf("named union member = %#v", members[1])
	}

	inline := requireShortElement(t, schema, "inline", "urn:test")
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
	assertShortDefinition(t, anonymous, version, "-32768", "32767")
	for _, name := range []string{"InlineList", "InlineUnion"} {
		definition := requireShortDefinition(t, schema, name)
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
		assertShortDefinition(t, nestedDefinition, version, "-32768", "32767")
	}
}

func requireShortElement(t *testing.T, schema Schema, local, namespace string) ElementDeclaration {
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

func requireShortDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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

func assertShortBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc, version XSDVersion) {
	t.Helper()
	assertIntegerBuiltinReference(t, reference, wantLoc, version, schemaSimpleTypeAtomicShort, "short", "-32768", "32767")
}

func assertShortDefinition(t *testing.T, definition SimpleTypeDefinition, version XSDVersion, wantMinimum, wantMaximum string) {
	t.Helper()
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicShort {
		t.Fatalf("definition variety/category = %q/%v, want atomic short", definition.Variety(), definition.facts)
	}
	assertIntegerReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, version, schemaSimpleTypeAtomicShort, "short", wantMinimum, wantMaximum)
}

func TestSchemaShortComposesGraphs(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := shortGraphFixtures(profile.version)
			assertIntegerDerivedComposedGraph(t, root, fixtures, profile, shortGraphElementCases(), schemaSimpleTypeAtomicShort, "short", "-32768", "32767")
		})
	}
}

func shortGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="p:short"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="p:short"/></xs:simpleType>
  <xs:element name="includedNamed" type="r:IncludedAlias"/>
  <xs:element name="includedDirect" type="p:short"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `"><xs:element name="chameleon" type="p:short"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedAlias"><xs:restriction base="p:short"/></xs:simpleType>
  <xs:element name="importedNamed" type="o:ImportedAlias"/>
  <xs:element name="importedDirect" type="p:short"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func shortGraphElementCases() []struct {
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
		{local: "root", namespace: "urn:root", source: "root.xsd", needle: `type="p:short"`},
		{local: "includedDirect", namespace: "urn:root", source: "ordinary.xsd", needle: `type="p:short"`},
		{local: "chameleon", namespace: "urn:root", source: "chameleon.xsd", needle: `type="p:short"`},
		{local: "importedDirect", namespace: "urn:other", source: "other.xsd", needle: `type="p:short"`},
		{local: "includedNamed", namespace: "urn:root", source: "ordinary.xsd", needle: `type="r:IncludedAlias"`, named: true},
		{local: "importedNamed", namespace: "urn:other", source: "other.xsd", needle: `type="o:ImportedAlias"`, named: true},
	}
}

func TestSchemaShortRejectsInvalidReferencesAndRestrictions(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, body string
			cause      error
		}{
			{name: "below minimum", body: `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:minInclusive value="-32769"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundRestriction},
			{name: "above maximum", body: `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:maxInclusive value="32768"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundRestriction},
			{name: "out of range enumeration", body: `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:enumeration value="32768"/></xs:restriction></xs:simpleType>`, cause: errInvalidEnumerationRestriction},
			{name: "malformed bound", body: `<xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundValue},
			{name: "unresolved base", body: `<xs:simpleType name="Bad"><xs:restriction base="t:Missing"/></xs:simpleType>`, cause: errSchemaSimpleTypeBaseUnresolved},
			{name: "wrong kind base", body: `<xs:element name="NotAType" type="xs:integer"/><xs:simpleType name="Bad"><xs:restriction base="t:NotAType"/></xs:simpleType>`, cause: errSchemaSimpleTypeBaseWrongKind},
			{name: "malformed QName", body: `<xs:element name="bad" type="t:bad:q"/>`},
			{name: "local ref stays element symbol", body: `<xs:complexType name="Root"><xs:sequence><xs:element ref="xs:short"/></xs:sequence></xs:complexType>`, cause: errSchemaElementReferenceUnresolved},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertShortInvalidNoPartialSchema(t, schema, err, test.cause)
			})
		}
	}
}

func assertShortInvalidNoPartialSchema(t *testing.T, schema Schema, err error, cause error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located error and no partial schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() == "" || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func TestSchemaShortConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:element name="value" type="p:short"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertIntegerDerivedConsumersUnsupported(t, schema)
		})
	}
}

func TestSchemaShortExcludedShapes(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, body, needle string
		}{
			{name: "local inline", body: `<xs:complexType name="T"><xs:choice><xs:element name="v"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`, needle: `<xs:simpleType>`},
			{name: "local attribute", body: `<xs:complexType name="T"><xs:attribute name="v" type="xs:short"/></xs:complexType>`, needle: `type="xs:short"`},
			{name: "simple content", body: `<xs:complexType name="T"><xs:simpleContent><xs:extension base="xs:short"/></xs:simpleContent></xs:complexType>`, needle: `base="xs:short"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("excluded %s returned schema/error = %#v/%v", test.name, schema, err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.needle) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want located schema unsupported", diagnostic)
				}
			})
		}
	}
}

func TestSchemaShortExclusiveBoundaryRestriction(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct{ facet, value string }{
			{facet: "minExclusive", value: "-32769"},
			{facet: "maxExclusive", value: "32768"},
		} {
			t.Run(profile.name+"/"+test.facet, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Bad"><xs:restriction base="xs:short"><xs:` + test.facet + ` value="` + test.value + `"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertShortInvalidNoPartialSchema(t, schema, err, errInvalidBoundRestriction)
				if diagnostic := requireDiagnostic(t, err); diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `value="`+test.value+`"`) {
					t.Fatalf("diagnostic = %s, want source facet location", diagnostic)
				}
			})
		}
	}
}

// The byte declaration is extracted from the pinned W3C datatype schema corpus.
//
//nolint:gocognit // Keep corpus provenance, repeated builds, reference facts, and the byte boundary together.
func TestSchemaShortPinnedCorpusFragment(t *testing.T) {
	const corpusPath = "testdata/w3c/xsdtests/msData/additional/test73722_dt.xsd"
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read pinned datatype schema: %v", err)
	}
	const startMarker = `<xs:simpleType name="byte" id="byte">`
	const endMarker = `</xs:simpleType>`
	start := strings.Index(string(data), startMarker)
	if start < 0 {
		t.Fatal("pinned byte declaration missing")
	}
	suffix := string(data[start:])
	end := strings.Index(suffix, endMarker)
	if end < 0 {
		t.Fatal("pinned byte declaration is incomplete")
	}
	fragment := suffix[:end+len(endMarker)]
	if !strings.Contains(fragment, `<xs:restriction base="xs:short">`) {
		t.Fatal("pinned byte declaration no longer references built-in xs:short")
	}
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `">` + fragment + `<xs:element name="value" type="xs:byte"/></xs:schema>`
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse bounded pinned byte declaration: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeat bounded pinned byte declaration: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("pinned byte declaration changed component facts or order")
			}
			definition := requireShortDefinition(t, first, "byte")
			assertShortDefinition(t, definition, profile.version, "-128", "127")
			base, ok := definition.BaseReference()
			if !ok {
				t.Fatal("pinned byte restriction has no base reference")
			}
			assertShortBuiltinReference(t, base, elementReferenceTestAttributeLoc(t, root, `base="xs:short"`), profile.version)
			declaration := requireShortElement(t, first, "value", "urn:test")
			reference, present := declaration.TypeReference()
			if !present {
				t.Fatal("pinned replay element has no type reference")
			}
			assertByteBuiltinReference(t, reference, elementReferenceTestAttributeLoc(t, root, `type="xs:byte"`), profile.version)
		})
	}
}
