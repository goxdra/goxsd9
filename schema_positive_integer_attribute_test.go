package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // The graph fixture checks one ordered public component contract.
func TestSchemaPositiveIntegerGlobalAttributeGraph(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := positiveIntegerAttributeGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated discovery changed component facts or order")
			}
			if len(first.Documents()) != 4 {
				t.Fatalf("document count = %d, want 4 after repeated and cyclic discovery", len(first.Documents()))
			}
			want := []struct {
				local, namespace, lexical, source, typeName, typeNamespace, typeSource, boundSource, boundMarker, minimum string
			}{
				{local: "direct", namespace: "urn:root", lexical: "xs:positiveInteger", source: "root.xsd", minimum: "1"},
				{local: "forward", namespace: "urn:root", lexical: "r:Forward", source: "root.xsd", typeName: "Forward", typeNamespace: "urn:root", typeSource: "root.xsd", minimum: "1"},
				{local: "narrowed", namespace: "urn:root", lexical: "r:Narrowed", source: "root.xsd", typeName: "Narrowed", typeNamespace: "urn:root", typeSource: "root.xsd", boundSource: "root.xsd", boundMarker: `value="2"`, minimum: "2"},
				{local: "included", namespace: "urn:root", lexical: "r:Included", source: "ordinary.xsd", typeName: "Included", typeNamespace: "urn:root", typeSource: "ordinary.xsd", minimum: "1"},
				{local: "chameleon", namespace: "urn:root", lexical: "c:Chameleon", source: "chameleon.xsd", typeName: "Chameleon", typeNamespace: "urn:root", typeSource: "chameleon.xsd", minimum: "1"},
				{local: "chameleonDirect", namespace: "urn:root", lexical: "xs:positiveInteger", source: "chameleon.xsd", minimum: "1"},
				{local: "imported", namespace: "urn:other", lexical: "o:Imported", source: "other.xsd", typeName: "Imported", typeNamespace: "urn:other", typeSource: "other.xsd", minimum: "1"},
			}
			var attributes []Component
			for _, component := range first.Components() {
				if component.Kind() == ComponentKindAttributeDeclaration {
					attributes = append(attributes, component)
				}
			}
			if len(attributes) != len(want) {
				t.Fatalf("attributes = %d, want %d", len(attributes), len(want))
			}
			for index, expected := range want {
				component := attributes[index]
				if component.Name() != mustTestQName(t, expected.namespace, expected.local) || component.ID().Source() != SourceID(expected.source) {
					t.Fatalf("attribute %d = %q/%v, want %s from %s", index, component.Name(), component.ID(), expected.local, expected.source)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.ID() != component.ID() || declaration.Loc() != schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `<xs:attribute name="`+expected.local+`"`, root, fixtures) {
					t.Fatalf("attribute %s declaration = %#v/%t", expected.local, declaration, ok)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("attribute %s has no type reference", expected.local)
				}
				name := mustTestQName(t, testXSDNamespace, "positiveInteger")
				if expected.typeName != "" {
					name = mustTestQName(t, expected.typeNamespace, expected.typeName)
				}
				loc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `type="`+expected.lexical+`"`, root, fixtures)
				if declaration.DeclaredType() != name || reference.Name() != name || reference.QName() != name || reference.Loc() != loc {
					t.Fatalf("attribute %s type = %q/%q/%q at %s, want %q at %s", expected.local, declaration.DeclaredType(), reference.Name(), reference.QName(), reference.Loc(), name, loc)
				}
				id, present := declaration.TypeID()
				refID, refPresent := reference.ComponentID()
				if expected.typeName == "" {
					if !reference.IsBuiltin() || present || refPresent || !id.IsZero() || !refID.IsZero() || reference.VarietyLoc() != loc {
						t.Fatalf("built-in reference %s = %#v with IDs %v/%v", expected.local, reference, id, refID)
					}
				}
				if expected.typeName != "" {
					target := componentIDForName(t, first, name)
					if !reference.IsNamed() || !present || !refPresent || id != target || refID != target || target.Source() != SourceID(expected.typeSource) {
						t.Fatalf("named reference %s IDs = %v/%t and %v/%t, want %v", expected.local, id, present, refID, refPresent, target)
					}
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || bounds.Version() != profile.version || len(bounds.Bounds()) != 1 {
					t.Fatalf("attribute %s bounds = %#v/%t", expected.local, bounds, ok)
				}
				minimum, ok := bounds.MinInclusiveFacet()
				if !ok || minimum.Value().Canonical() != expected.minimum || minimum.Kind() != BoundMinInclusive {
					t.Fatalf("attribute %s minimum = %#v/%t", expected.local, minimum, ok)
				}
				boundLoc := Loc{}
				if expected.boundSource != "" {
					boundLoc = schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.boundSource), expected.boundMarker, root, fixtures)
				}
				if minimum.Loc() != boundLoc {
					t.Fatalf("attribute %s bound location = %s, want %s", expected.local, minimum.Loc(), boundLoc)
				}
			}
			walked := make([]ComponentID, 0, len(first.Components()))
			if err := first.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for index, component := range first.Components() {
				if walked[index] != component.ID() {
					t.Fatalf("walked component %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

func positiveIntegerAttributeGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:positiveInteger"/>
  <xs:attribute name="forward" type="r:Forward"/>
  <xs:attribute name="narrowed" type="r:Narrowed"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="r:Forward"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="Included"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:attribute name="included" type="r:Included"/>
</xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:c="urn:root">
  <xs:simpleType name="Chameleon"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:attribute name="chameleon" type="c:Chameleon"/>
  <xs:attribute name="chameleonDirect" type="xs:positiveInteger"/>
</xs:schema>`},
		"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="Imported"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:attribute name="imported" type="o:Imported"/>
</xs:schema>`},
	}
}

//nolint:gocognit // Verify each reference-resolution exit at the global attribute boundary.
func TestSchemaPositiveIntegerGlobalAttributeReferenceDiagnostics(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker, related string
			code                        string
			cause                       error
			base                        bool
		}{
			{name: "unresolved type", body: `<xs:attribute name="item" type="t:Missing"/>`, marker: `type="t:Missing"`, code: diagnosticSchemaAttributeTypeUnresolvedCode, cause: errSchemaAttributeTypeUnresolved},
			{name: "wrong kind", body: `<xs:element name="Other" type="xs:positiveInteger"/><xs:attribute name="item" type="t:Other"/>`, marker: `type="t:Other"`, related: `<xs:element name="Other"`, code: diagnosticSchemaAttributeTypeWrongKindCode, cause: errSchemaAttributeTypeWrongKind},
			{name: "unresolved base", body: `<xs:attribute name="item" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="t:Missing"/></xs:simpleType>`, marker: `base="t:Missing"`, code: diagnosticSchemaSimpleTypeUnresolvedCode, cause: errSchemaSimpleTypeBaseUnresolved, base: true},
			{name: "wrong kind base", body: `<xs:element name="Other" type="xs:positiveInteger"/><xs:attribute name="item" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="t:Other"/></xs:simpleType>`, marker: `base="t:Other"`, related: `<xs:element name="Other"`, code: diagnosticSchemaSimpleTypeWrongKindCode, cause: errSchemaSimpleTypeBaseWrongKind, base: true},
			{name: "cycle", body: `<xs:attribute name="item" type="t:A"/><xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>`, marker: `type="t:A"`, code: diagnosticSchemaAttributeTypeCycleCode, cause: errSchemaSimpleTypeBaseCycle},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertPositiveIntegerNoPartialSchema(t, schema, err)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := schemaAttributeTypeSpecRef(profile.version)
				if test.base {
					wantSpec = schemaSimpleTypeSpecRef(profile.version)
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with cause %v", diagnostic, test.code, test.marker, test.cause)
				}
				if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related = %v, want %q", diagnostic.Related(), test.related)
				}
			})
		}
	}
}

//nolint:gocognit // Both type shapes and all policies must keep the same consumer boundary.
func TestSchemaPositiveIntegerGlobalAttributeGenerationRemainsUnsupported(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct{ name, body string }{
			{name: "direct", body: `<xs:attribute name="item" type="xs:positiveInteger"/>`},
			{name: "named", body: `<xs:attribute name="item" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = (%q, %v), want unsupported and nil output", output, err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="item"`) || !errors.Is(err, errCodegenUnsupported) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("generation diagnostic = %s, want unsupported at attribute declaration", diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Test deep copies through both built-in and named attribute views.
func TestSchemaPositiveIntegerGlobalAttributeBoundsAreDefensiveCopies(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := positiveIntegerAttributeGraphFixtures(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct{ name, minimum string }{{"direct", "1"}, {"narrowed", "2"}} {
				matches := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", want.name))
				if len(matches) != 1 {
					t.Fatalf("attribute %s count = %d", want.name, len(matches))
				}
				declaration, ok := matches[0].AttributeDeclaration()
				if !ok {
					t.Fatalf("attribute %s view missing", want.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("attribute %s reference missing", want.name)
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || bounds.lower == nil {
					t.Fatalf("attribute %s bound missing", want.name)
				}
				bounds.lower.value.value.SetInt64(99)
				for _, facet := range bounds.Bounds() {
					facet.value.value.SetInt64(99)
				}
				again, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("attribute %s second reference missing", want.name)
				}
				againBounds, _ := again.IntegerBounds()
				minimum, ok := againBounds.MinInclusive()
				if !ok || minimum.Canonical() != want.minimum {
					t.Fatalf("attribute %s stored minimum = %q/%t, want %s", want.name, minimum.Canonical(), ok, want.minimum)
				}
			}
		})
	}
}
