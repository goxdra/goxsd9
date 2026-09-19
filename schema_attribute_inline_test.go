package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep the ordered inline attribute model contract together.
func TestSchemaGlobalAttributeInlineSimpleTypesPreserveFacts(t *testing.T) {
	rootForVersion := func(version XSDVersion) string {
		return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="atomic">
    <xs:simpleType><xs:restriction base="xs:integer"><xs:minInclusive value="1"/></xs:restriction></xs:simpleType>
  </xs:attribute>
  <xs:attribute name="list">
    <xs:simpleType><xs:list><xs:simpleType><xs:restriction base="xs:language"/></xs:simpleType></xs:list></xs:simpleType>
  </xs:attribute>
  <xs:attribute name="union">
    <xs:simpleType><xs:union memberTypes="r:Forward o:Imported"><xs:simpleType><xs:restriction base="xs:NCName"/></xs:simpleType></xs:union></xs:simpleType>
  </xs:attribute>
  <xs:simpleType name="Forward"><xs:restriction base="xs:anyURI"/></xs:simpleType>
</xs:schema>`
	}
	fixtures := map[string]discoveryFixture{
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`,
		},
	}
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := rootForVersion(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated inline attribute builds changed component facts or order")
			}

			atomic := requireInlineAttribute(t, first, "atomic")
			atomicReference := requireInlineAttributeReference(t, atomic)
			assertAnonymousAttributeReference(t, atomic, atomicReference, root, `<xs:simpleType><xs:restriction base="xs:integer"`)
			if atomicReference.Variety() != SimpleTypeVarietyAtomicRestriction {
				t.Fatalf("atomic reference variety = %q, want atomic restriction", atomicReference.Variety())
			}
			atomicType := requireAnonymousAttributeType(t, atomicReference)
			base, ok := atomicType.BaseReference()
			if !ok || !base.IsBuiltin() || base.Name().Local() != "integer" {
				t.Fatalf("atomic base reference = %#v/%t, want built-in integer", base, ok)
			}
			bounds, ok := atomicType.IntegerBounds()
			if !ok {
				t.Fatal("atomic integer bounds are absent")
			}
			minimum, ok := bounds.MinInclusive()
			if !ok || minimum.Canonical() != "1" {
				t.Fatalf("atomic minInclusive = %q/%t, want 1/true", minimum.Canonical(), ok)
			}

			list := requireInlineAttribute(t, first, "list")
			listReference := requireInlineAttributeReference(t, list)
			assertAnonymousAttributeReference(t, list, listReference, root, `<xs:simpleType><xs:list>`)
			if listReference.Variety() != SimpleTypeVarietyList {
				t.Fatalf("list reference variety = %q, want list", listReference.Variety())
			}
			listType := requireAnonymousAttributeType(t, listReference)
			item, ok := listType.ItemType()
			if !ok || !item.IsAnonymous() || item.Variety() != SimpleTypeVarietyAtomicRestriction {
				t.Fatalf("list item = %#v/%t, want anonymous atomic restriction", item, ok)
			}
			itemType := requireAnonymousAttributeType(t, item)
			itemBase, ok := itemType.BaseReference()
			if !ok || !itemBase.IsBuiltin() || itemBase.Name().Local() != "language" {
				t.Fatalf("list item base = %#v/%t, want built-in language", itemBase, ok)
			}
			listID, listOK := listReference.AnonymousID()
			if itemID, itemOK := item.AnonymousID(); !itemOK || !listOK || itemID == listID {
				t.Fatal("list item did not retain a distinct anonymous identity")
			}

			union := requireInlineAttribute(t, first, "union")
			unionReference := requireInlineAttributeReference(t, union)
			assertAnonymousAttributeReference(t, union, unionReference, root, `<xs:simpleType><xs:union`)
			if unionReference.Variety() != SimpleTypeVarietyUnion {
				t.Fatalf("union reference variety = %q, want union", unionReference.Variety())
			}
			unionType := requireAnonymousAttributeType(t, unionReference)
			members := unionType.MemberTypes()
			if len(members) != 3 {
				t.Fatalf("union member count = %d, want 3", len(members))
			}
			forward := requireSimpleTypeDefinition(t, first, "urn:root", "Forward")
			imported := requireSimpleTypeDefinition(t, first, "urn:other", "Imported")
			wantMembers := []struct {
				name QName
				id   ComponentID
			}{
				{name: mustTestQName(t, "urn:root", "Forward"), id: forward.ID()},
				{name: mustTestQName(t, "urn:other", "Imported"), id: imported.ID()},
			}
			for index, want := range wantMembers {
				if members[index].Kind() != SimpleTypeReferenceNamed || members[index].Name() != want.name {
					t.Fatalf("union member %d = %q/%q, want named %q", index, members[index].Kind(), members[index].Name(), want.name)
				}
				if got, hasID := members[index].ComponentID(); !hasID || got != want.id {
					t.Fatalf("union member %d ID = %v/%t, want %v/true", index, got, hasID, want.id)
				}
			}
			if !members[2].IsAnonymous() {
				t.Fatalf("union member 2 = %#v, want anonymous", members[2])
			}
			memberType := requireAnonymousAttributeType(t, members[2])
			memberBase, ok := memberType.BaseReference()
			if !ok || !memberBase.IsBuiltin() || memberBase.Name().Local() != "NCName" {
				t.Fatalf("union anonymous member base = %#v/%t, want built-in NCName", memberBase, ok)
			}

			components := first.Components()
			before := first.Components()
			before[0] = Component{}
			members[0] = SimpleTypeReference{}
			if !reflect.DeepEqual(components, first.Components()) {
				t.Fatal("mutating returned inline attribute views changed Schema")
			}
			walked := make([]ComponentID, 0, len(components))
			if err := first.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			wantWalk := make([]ComponentID, 0, len(components))
			for _, component := range components {
				wantWalk = append(wantWalk, component.ID())
			}
			if !reflect.DeepEqual(walked, wantWalk) {
				t.Fatalf("Walk IDs = %#v, want %#v", walked, wantWalk)
			}
		})
	}
}

func requireInlineAttribute(t *testing.T, schema Schema, local string) AttributeDeclaration {
	t.Helper()
	components := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", local))
	if len(components) != 1 {
		t.Fatalf("attribute %q count = %d, want 1", local, len(components))
	}
	declaration, ok := components[0].AttributeDeclaration()
	if !ok {
		t.Fatalf("attribute %q has no AttributeDeclaration view", local)
	}
	return declaration
}

func requireInlineAttributeReference(t *testing.T, declaration AttributeDeclaration) SimpleTypeReference {
	t.Helper()
	if !declaration.DeclaredType().IsZero() {
		t.Fatalf("inline attribute declared type = %q, want zero QName", declaration.DeclaredType())
	}
	if typeID, ok := declaration.TypeID(); ok || !typeID.IsZero() {
		t.Fatalf("inline attribute type ID = %v/%t, want zero/false", typeID, ok)
	}
	reference, ok := declaration.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("inline attribute type reference = %#v/%t, want anonymous", reference, ok)
	}
	return reference
}

func assertAnonymousAttributeReference(t *testing.T, declaration AttributeDeclaration, reference SimpleTypeReference, root, marker string) {
	t.Helper()
	wantLoc := elementReferenceTestAttributeLoc(t, root, marker)
	if reference.Loc() != wantLoc || reference.VarietyLoc().IsZero() {
		t.Fatalf("attribute reference locations = %s/%s, want %s and non-zero variety location", reference.Loc(), reference.VarietyLoc(), wantLoc)
	}
	anonymousID, ok := reference.AnonymousID()
	if !ok || anonymousID.IsZero() {
		t.Fatal("attribute anonymous reference identity is missing")
	}
	anonymous, ok := reference.AnonymousType()
	if !ok || !anonymous.IsAnonymous() || anonymous.Loc() != reference.Loc() {
		t.Fatalf("attribute anonymous model = %#v/%t, want anonymous model at reference location", anonymous, ok)
	}
	inline, ok := declaration.InlineSimpleType()
	inlineID, inlineIDOK := inline.NodeID()
	if !ok || !inlineIDOK || inlineID != anonymousID || inline.Loc() != reference.Loc() {
		t.Fatalf("attribute inline convenience view = %#v/%t, want reference model", inline, ok)
	}
}

func requireAnonymousAttributeType(t *testing.T, reference SimpleTypeReference) SimpleTypeDefinition {
	t.Helper()
	definition, ok := reference.AnonymousType()
	if !ok {
		t.Fatalf("reference %#v has no anonymous type", reference)
	}
	return definition
}

func requireSimpleTypeDefinition(t *testing.T, schema Schema, namespace, local string) SimpleTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("simple type %s:%s count = %d, want 1", namespace, local, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("simple type %s:%s has no definition view", namespace, local)
	}
	return definition
}

func TestSchemaGlobalAttributeInlineSimpleTypeValueConstraintIsUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" default="1"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("inline attribute value constraint was accepted or returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.SpecRef() != schemaAttributeValueConstraintXSD11SpecRef {
		t.Fatalf("diagnostic = %s/%q/%q/%q, want unsupported attribute value constraint", diagnostic, diagnostic.Class(), diagnostic.Feature(), diagnostic.SpecRef())
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "default=") {
		t.Fatalf("diagnostic location = %s, want default location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeValueConstraintUnsupported) {
		t.Fatalf("diagnostic lost unsupported value-constraint cause: %v", err)
	}
}

//nolint:gocognit // Keep the diagnostic matrix and invariant checks together.
func TestSchemaGlobalAttributeInlineSimpleTypeDiagnosticsRemainStable(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		class       FailureClass
		code        string
		cause       error
		primary     string
		related     string
		spec        string
		unsupported bool
	}{
		{
			name:    "malformed child",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value"><xs:simpleType/></xs:attribute></xs:schema>`,
			class:   FailureInvalid,
			code:    invalidSchemaCompositionCode,
			primary: "<xs:simpleType/>",
		},
		{
			name:    "unresolved member",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value"><xs:simpleType><xs:union memberTypes="r:Missing"/></xs:simpleType></xs:attribute></xs:schema>`,
			class:   FailureInvalid,
			code:    diagnosticSchemaSimpleTypeUnresolvedCode,
			cause:   errSchemaSimpleTypeBaseUnresolved,
			primary: "memberTypes=",
			spec:    schemaSimpleTypeSpecRef(XSDVersion11),
		},
		{
			name:    "wrong-kind member",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value"><xs:simpleType><xs:union memberTypes="r:Element"/></xs:simpleType></xs:attribute><xs:element name="Element"/></xs:schema>`,
			class:   FailureInvalid,
			code:    diagnosticSchemaSimpleTypeWrongKindCode,
			cause:   errSchemaSimpleTypeBaseWrongKind,
			primary: "memberTypes=",
			related: "<xs:element name=\"Element\"",
			spec:    schemaSimpleTypeSpecRef(XSDVersion11),
		},
		{
			name:        "unsupported inline string",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:attribute></xs:schema>`,
			class:       FailureUnsupported,
			code:        UnsupportedSchemaSyntaxCode,
			cause:       errSchemaAttributeTypeUnsupported,
			primary:     `<xs:simpleType><xs:restriction base="xs:string"`,
			spec:        schemaAttributeTypeXSD11SpecRef,
			unsupported: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("invalid or unsupported inline attribute type returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
			}
			if test.spec != "" && diagnostic.SpecRef() != test.spec {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.spec)
			}
			if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.primary) {
				t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), elementReferenceTestAttributeLoc(t, test.root, test.primary))
			}
			if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, test.root, test.related)) {
				t.Fatalf("diagnostic related locations = %v, want %s", diagnostic.Related(), test.related)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
			if test.unsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic lost ErrUnsupported: %v", err)
			}
		})
	}
}
