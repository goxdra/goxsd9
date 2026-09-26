package goxsd9

import (
	"errors"
	"reflect"
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

//nolint:gocognit // Keep repeated graph discovery and ordered reference checks together.
func TestSchemaShortComposesGraphs(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := shortGraphFixtures(profile.version)
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
			for _, test := range shortGraphElementCases() {
				assertShortGraphElement(t, first, root, fixtures, test, profile.version)
			}
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

func assertShortGraphElement(t *testing.T, schema Schema, root string, fixtures map[string]discoveryFixture, test struct {
	local     string
	namespace string
	source    SourceID
	needle    string
	named     bool
}, version XSDVersion) {
	t.Helper()
	declaration := requireShortElement(t, schema, test.local, test.namespace)
	reference, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s type reference is missing", test.local)
	}
	if test.named {
		if !reference.IsNamed() {
			t.Fatalf("%s type reference = %#v, want named", test.local, reference)
		}
		if reference.Loc() != schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures) {
			t.Fatalf("%s type Loc = %s, want use-site Loc", test.local, reference.Loc())
		}
		id, hasID := reference.ComponentID()
		if !hasID || id.Source() != test.source {
			t.Fatalf("%s target ID = %v/%t, want source %s", test.local, id, hasID, test.source)
		}
		assertIntegerReferenceFacts(t, reference.facts, version, schemaSimpleTypeAtomicShort, "short", "-32768", "32767")
		return
	}
	assertShortBuiltinReference(t, reference, schemaBuiltinReferenceAttributeLoc(t, test.source, test.needle, root, fixtures), version)
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

func TestSchemaShortExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:short" default="0"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatalf("attribute value constraint schema/error = %#v/%v, want unsupported without schema", schema, err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic = %s, want located unsupported with preserved cause", diagnostic)
			}
		})
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

//nolint:gocognit // Keep direct and forward attribute identity, location, and facet checks together.
func TestSchemaShortGlobalAttributeReferences(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:attribute name="direct" type="xs:short"/>
  <xs:attribute name="forward" type="t:Narrow"/>
  <xs:simpleType name="Narrow"><xs:restriction base="xs:short"><xs:minExclusive value="-32768"/><xs:maxExclusive value="32767"/></xs:restriction></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			for _, test := range []struct {
				name, typeNeedle, min, max string
				builtin                    bool
			}{
				{name: "direct", typeNeedle: `type="xs:short"`, min: "-32768", max: "32767", builtin: true},
				{name: "forward", typeNeedle: `type="t:Narrow"`, min: "-32768", max: "32767"},
			} {
				components := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", test.name))
				if len(components) != 1 {
					t.Fatalf("attribute %s components = %d, want 1", test.name, len(components))
				}
				attribute, ok := components[0].AttributeDeclaration()
				if !ok {
					t.Fatalf("attribute %s has no declaration", test.name)
				}
				reference, ok := attribute.TypeReference()
				if !ok || reference.Loc() != elementReferenceTestAttributeLoc(t, root, test.typeNeedle) {
					t.Fatalf("attribute %s reference = %#v/%t, want located reference", test.name, reference, ok)
				}
				if test.builtin {
					assertShortBuiltinReference(t, reference, reference.Loc(), profile.version)
					continue
				}
				if !reference.IsNamed() || reference.Name() != mustTestQName(t, "urn:test", "Narrow") {
					t.Fatalf("attribute %s reference = %#v, want named Narrow", test.name, reference)
				}
				if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicShort {
					t.Fatalf("named attribute type facts = %#v, want short", reference.facts)
				}
				bounds, present := reference.IntegerBounds()
				if !present || bounds.Version() != profile.version {
					t.Fatalf("named attribute bounds = %#v/%t, want version %s", bounds, present, profile.version)
				}
				minimum, hasMinimum := bounds.MinExclusive()
				maximum, hasMaximum := bounds.MaxExclusive()
				if !hasMinimum || !hasMaximum || minimum.Canonical() != test.min || maximum.Canonical() != test.max {
					t.Fatalf("named attribute exclusive bounds = %s/%t, %s/%t, want %s/%s", minimum.Canonical(), hasMinimum, maximum.Canonical(), hasMaximum, test.min, test.max)
				}
				if id, present := reference.ComponentID(); !present || id.IsZero() {
					t.Fatalf("named attribute type ID = %v/%t, want nonzero", id, present)
				}
			}
		})
	}
}

func TestSchemaShortExcludedShapes(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, body, needle string
		}{
			{name: "local direct", body: `<xs:complexType name="T"><xs:sequence><xs:element name="v" type="xs:short"/></xs:sequence></xs:complexType>`, needle: `type="xs:short"`},
			{name: "local named", body: `<xs:complexType name="T"><xs:choice><xs:element name="v" type="t:ShortType"/></xs:choice></xs:complexType><xs:simpleType name="ShortType"><xs:restriction base="xs:short"/></xs:simpleType>`, needle: `type="t:ShortType"`},
			{name: "local inline", body: `<xs:complexType name="T"><xs:choice><xs:element name="v"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`, needle: `<xs:simpleType>`},
			{name: "local attribute", body: `<xs:complexType name="T"><xs:attribute name="v" type="xs:short"/></xs:complexType>`, needle: `type="xs:short"`},
			{name: "simple content", body: `<xs:complexType name="T"><xs:simpleContent><xs:extension base="xs:short"/></xs:simpleContent></xs:complexType>`, needle: `base="xs:short"`},
			{name: "byte", body: `<xs:element name="v" type="xs:byte"/>`, needle: `type="xs:byte"`},
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

// This bounded fragment follows the short declaration in the pinned W3C datatypes graph.
func TestSchemaShortPinnedCorpusFragment(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:simpleType name="short"><xs:restriction base="xs:int"><xs:minInclusive value="-32768"/><xs:maxInclusive value="32767"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="byte"><xs:restriction base="t:short"><xs:minInclusive value="-128"/><xs:maxInclusive value="127"/></xs:restriction></xs:simpleType>
</xs:schema>`
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("pinned short fragment: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated pinned short fragment: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("pinned short fragment changed component facts or order")
			}
			for _, test := range []struct{ name, min, max string }{{"short", "-32768", "32767"}, {"byte", "-128", "127"}} {
				definition := requireShortDefinition(t, first, test.name)
				assertIntegerReferenceFacts(t, &schemaSimpleTypeReferenceComponent{atomicKind: definition.facts.atomicKind, facets: definition.facts.facets}, profile.version, schemaSimpleTypeAtomicInt, "int", test.min, test.max)
			}
		})
	}
}
