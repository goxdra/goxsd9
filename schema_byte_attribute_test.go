package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func byteGlobalAttributeGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root, fixtures := intGlobalAttributeGraphFixtures(version)
	replace := strings.NewReplacer("Int", "Byte", "int", "byte", "-2147483648", "-128", "2147483647", "127")
	root = replace.Replace(root)
	for _, source := range []string{"root.xsd", "ordinary.xsd", "chameleon.xsd", "other.xsd"} {
		fixture := fixtures[source]
		fixture.contents = replace.Replace(fixture.contents)
		fixtures[source] = fixture
	}
	return root, fixtures
}

func byteAttributePolicyProfiles() []longPolicyProfile {
	return append([]longPolicyProfile{{name: "Compatibility XSD 1.0", policy: Compatibility, version: XSDVersion10}}, longPolicyProfiles()...)
}

//nolint:gocognit,funlen // Exercise ordering, provenance, and copied facts in one graph.
func TestSchemaByteGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			builtinVersion := profile.version
			if profile.policy == Compatibility {
				builtinVersion = XSDVersion11
			}
			root, fixtures := byteGlobalAttributeGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) || len(first.Documents()) != 4 {
				t.Fatal("repeated byte attribute graph changed facts, order, or discovery count")
			}
			want := []struct {
				local, namespace, lexical, source, typeSource string
				minimum, maximum                              string
			}{
				{"direct", "urn:root", "xs:byte", "root.xsd", "", "-128", "127"},
				{"forward", "urn:root", "r:ForwardByte", "root.xsd", "root.xsd", "-128", "127"},
				{"imported", "urn:root", "o:ImportedByte", "root.xsd", "other.xsd", "-128", "127"},
				{"chameleon", "urn:root", "r:ChameleonByte", "root.xsd", "chameleon.xsd", "-128", "127"},
				{"narrowed", "urn:root", "r:NarrowedByte", "root.xsd", "root.xsd", "-3", "2"},
				{"included", "urn:root", "r:IncludedByte", "ordinary.xsd", "ordinary.xsd", "-128", "127"},
				{"chameleonDirect", "urn:root", "xs:byte", "chameleon.xsd", "", "-128", "127"},
				{"importedDirect", "urn:other", "xs:byte", "other.xsd", "", "-128", "127"},
			}
			attributes := make([]Component, 0, len(want))
			for _, component := range first.Components() {
				if component.Kind() == ComponentKindAttributeDeclaration {
					attributes = append(attributes, component)
				}
			}
			if len(attributes) != len(want) {
				t.Fatalf("attribute count = %d, want %d", len(attributes), len(want))
			}
			for index, expected := range want {
				component := attributes[index]
				if component.Name() != mustTestQName(t, expected.namespace, expected.local) || component.ID().Source() != SourceID(expected.source) {
					t.Fatalf("attribute %d = %q/%v, want %s from %s", index, component.Name(), component.ID(), expected.local, expected.source)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.Loc() != schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `<xs:attribute name="`+expected.local+`"`, root, fixtures) {
					t.Fatalf("attribute %s declaration = %#v/%t", expected.local, declaration, ok)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("attribute %s has no type reference", expected.local)
				}
				typeName := mustTestQName(t, testXSDNamespace, "byte")
				if expected.typeSource != "" {
					typeNamespace := "urn:root"
					if expected.typeSource == "other.xsd" {
						typeNamespace = "urn:other"
					}
					typeName = mustTestQName(t, typeNamespace, expected.lexical[2:])
				}
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `type="`+expected.lexical+`"`, root, fixtures)
				if declaration.DeclaredType() != typeName || reference.QName() != typeName || reference.Loc() != wantLoc {
					t.Fatalf("attribute %s type = %q/%q at %s, want %q at %s", expected.local, declaration.DeclaredType(), reference.QName(), reference.Loc(), typeName, wantLoc)
				}
				if expected.typeSource == "" {
					assertByteBuiltinReference(t, reference, wantLoc, builtinVersion)
					if id, present := declaration.TypeID(); present || !id.IsZero() {
						t.Fatalf("built-in byte type ID = %v/%t, want none", id, present)
					}
				}
				if expected.typeSource != "" {
					wantID := componentIDForName(t, first, typeName)
					id, present := declaration.TypeID()
					if !reference.IsNamed() || !present || id != wantID || id.Source() != SourceID(expected.typeSource) {
						t.Fatalf("named byte attribute %s ID = %v/%t, want %v", expected.local, id, present, wantID)
					}
					if referenceID, present := reference.ComponentID(); !present || referenceID != wantID {
						t.Fatalf("named byte reference %s ID = %v/%t, want %v", expected.local, referenceID, present, wantID)
					}
					if expected.local == "narrowed" {
						if reference.VarietyLoc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:restriction base="xs:byte"><xs:minExclusive`, root, fixtures) {
							t.Fatalf("narrowed byte variety location = %s, want restriction", reference.VarietyLoc())
						}
						if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicByte {
							t.Fatalf("narrowed byte reference facts = %#v", reference.facts)
						}
						assertPublicNarrowedByteAttributeFacts(t, first, typeName, root, fixtures)
					}
					if expected.local != "narrowed" {
						assertIntegerReferenceFacts(t, reference.facts, builtinVersion, schemaSimpleTypeAtomicByte, "byte", expected.minimum, expected.maximum)
					}
				}
				bounds, present := reference.IntegerBounds()
				if !present {
					t.Fatalf("attribute %s has no public integer bounds", expected.local)
				}
				ordered := bounds.Bounds()
				if len(ordered) != 2 || ordered[0].Value().Canonical() != expected.minimum || ordered[1].Value().Canonical() != expected.maximum {
					t.Fatalf("attribute %s bounds = %#v, want %s..%s", expected.local, ordered, expected.minimum, expected.maximum)
				}
				bounds.lower.value.value.SetInt64(0)
				bounds.upper.value.value.SetInt64(0)
				repeated, present := reference.IntegerBounds()
				if !present || repeated.Bounds()[0].Value().Canonical() != expected.minimum || repeated.Bounds()[1].Value().Canonical() != expected.maximum {
					t.Fatalf("attribute %s bounds changed through a public copy", expected.local)
				}
			}
			before := first.Components()
			components := first.Components()
			components[0] = Component{}
			found := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "narrowed"))
			found[0] = Component{}
			document := first.Documents()[0].Components()
			document[0] = Component{}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied byte component views changed Schema")
			}
			walked := make([]ComponentID, 0, len(before))
			if err := first.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if len(walked) != len(before) {
				t.Fatalf("walked byte component count = %d, want %d", len(walked), len(before))
			}
			for index, component := range before {
				if walked[index] != component.ID() {
					t.Fatalf("walked ID %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

//nolint:gocognit // Verify public facet values, locations, and copies together.
func assertPublicNarrowedByteAttributeFacts(t *testing.T, schema Schema, name QName, root string, fixtures map[string]discoveryFixture) {
	t.Helper()
	components := schema.FindKind(ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("narrowed byte type count = %d, want one", len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("narrowed byte simple type view missing")
	}
	bounds, ok := definition.IntegerBounds()
	if !ok {
		t.Fatal("narrowed byte public bounds missing")
	}
	ordered := bounds.Bounds()
	if len(ordered) != 2 || ordered[0].Kind() != BoundMinExclusive || ordered[0].Value().Canonical() != "-3" || ordered[1].Kind() != BoundMaxInclusive || ordered[1].Value().Canonical() != "2" {
		t.Fatalf("narrowed byte public bounds = %v, want (-3, 2]", ordered)
	}
	for index, needle := range []string{`value="-3"`, `value="2"`} {
		if ordered[index].Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", needle, root, fixtures) {
			t.Fatalf("bound %d location = %s, want %s", index, ordered[index].Loc(), needle)
		}
	}
	digits := definition.DigitFacets()
	if total, present := digits.TotalDigits(); !present || total.Canonical() != "1" {
		t.Fatalf("narrowed byte totalDigits = %q/%t, want 1", total.Canonical(), present)
	}
	if loc, present := digits.TotalDigitsLoc(); !present || loc != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="1"`, root, fixtures) {
		t.Fatalf("narrowed byte totalDigits location = %s/%t", loc, present)
	}
	enumeration := definition.IntegerEnumerationFacets()
	values := enumeration.Values()
	locations := enumeration.Locations()
	if len(values) != 3 || len(locations) != 3 {
		t.Fatalf("narrowed byte enumeration = %v/%v, want three values", values, locations)
	}
	for index, lexical := range []string{"-2", "0", "2"} {
		if values[index].Canonical() != lexical || locations[index] != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:enumeration value="`+lexical+`"`, root, fixtures) {
			t.Fatalf("enumeration %d = %s at %s, want %s", index, values[index].Canonical(), locations[index], lexical)
		}
	}
	ordered[0].value.value.SetInt64(0)
	values[0].value.SetInt64(0)
	boundsCopy, ok := definition.IntegerBounds()
	if !ok || boundsCopy.Bounds()[0].Value().Canonical() != "-3" || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "-2" {
		t.Fatal("mutating public byte facet views changed Schema")
	}
}

//nolint:gocognit // Check each exit at the attribute type boundary.
func TestSchemaByteGlobalAttributeExclusionsAcrossPolicies(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		wantVersion := profile.version
		if profile.policy == Compatibility {
			wantVersion = XSDVersion11
		}
		for _, test := range []struct {
			name, body, needle string
			class              FailureClass
			cause              error
			code, spec         string
			related            string
		}{
			{name: "below minimum", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:byte"><xs:minInclusive value="-129"/></xs:restriction></xs:simpleType>`, needle: `value="-129"`, class: FailureInvalid, cause: errInvalidBoundRestriction, code: InvalidBoundRestrictionCode, spec: boundSpecRef(wantVersion, BoundMinInclusive, boundRestrictionRule)},
			{name: "above maximum", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:byte"><xs:maxInclusive value="128"/></xs:restriction></xs:simpleType>`, needle: `value="128"`, class: FailureInvalid, cause: errInvalidBoundRestriction, code: InvalidBoundRestrictionCode, spec: boundSpecRef(wantVersion, BoundMaxInclusive, boundRestrictionRule)},
			{name: "malformed bound", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:byte"><xs:minInclusive value="oops"/></xs:restriction></xs:simpleType>`, needle: `value="oops"`, class: FailureInvalid, cause: errInvalidBoundValue, code: InvalidBoundCode, spec: boundSpecRef(wantVersion, BoundMinInclusive, boundDefinitionRule)},
			{name: "unresolved", body: `<xs:attribute name="a" type="r:Missing"/>`, needle: `type="r:Missing"`, class: FailureInvalid, cause: errSchemaAttributeTypeUnresolved, code: diagnosticSchemaAttributeTypeUnresolvedCode, spec: schemaAttributeTypeSpecRef(wantVersion)},
			{name: "wrong kind", body: `<xs:element name="T" type="xs:byte"/><xs:attribute name="a" type="r:T"/>`, needle: `type="r:T"`, class: FailureInvalid, cause: errSchemaAttributeTypeWrongKind, code: diagnosticSchemaAttributeTypeWrongKindCode, spec: schemaAttributeTypeSpecRef(wantVersion), related: `<xs:element name="T"`},
			{name: "named list", body: `<xs:attribute name="a" type="r:L"/><xs:simpleType name="L"><xs:list itemType="xs:byte"/></xs:simpleType>`, needle: `type="r:L"`, class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(wantVersion)},
			{name: "local direct", body: `<xs:complexType name="T"><xs:attribute name="a" type="xs:byte"/></xs:complexType>`, needle: `type="xs:byte"`, class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(wantVersion)},
			{name: "local named", body: `<xs:complexType name="T"><xs:attribute name="a" type="r:B"/></xs:complexType><xs:simpleType name="B"><xs:restriction base="xs:byte"/></xs:simpleType>`, needle: `type="r:B"`, class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(wantVersion)},
			{name: "global inline", body: `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:attribute>`, needle: `<xs:simpleType>`, class: FailureUnsupported, cause: ErrUnsupported, code: UnsupportedSchemaSyntaxCode},
			{name: "local inline", body: `<xs:complexType name="T"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:attribute></xs:complexType>`, needle: `<xs:simpleType>`, class: FailureUnsupported, cause: ErrUnsupported, code: UnsupportedSchemaSyntaxCode},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded byte attribute returned a schema or no error")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.needle) || diagnostic.Code() != test.code || diagnostic.SpecRef() == "" || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with %v", diagnostic, test.class, test.needle, test.cause)
				}
				if test.spec != "" && diagnostic.SpecRef() != test.spec {
					t.Fatalf("diagnostic spec = %q, want %q", diagnostic.SpecRef(), test.spec)
				}
				if test.related != "" && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
					t.Fatalf("diagnostic related = %v, want %s", diagnostic.Related(), test.related)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want unsupported cause", diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Keep reference-use and generator consumer boundaries together across policies.
func TestSchemaByteGlobalAttributeReferenceAndConsumerRemainUnsupported(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			wantVersion := profile.version
			if profile.policy == Compatibility {
				wantVersion = XSDVersion11
			}
			for _, typeShape := range []struct{ name, body string }{
				{"direct", `<xs:attribute name="global" type="xs:byte"/>`},
				{"named", `<xs:attribute name="global" type="r:Byte"/><xs:simpleType name="Byte"><xs:restriction base="xs:byte"/></xs:simpleType>`},
			} {
				t.Run(typeShape.name+" reference", func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r">` + typeShape.body + `<xs:complexType name="T"><xs:attribute ref="r:global"/></xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("local byte attribute reference returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="r:global"`) || diagnostic.SpecRef() != schemaAttributeUseSpecRef(wantVersion) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
						t.Fatalf("local reference diagnostic = %s, want located unsupported use", diagnostic)
					}
					if !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="global"`)}) {
						t.Fatalf("related locations = %v, want global declaration", diagnostic.Related())
					}
				})
			}
			for _, typeShape := range []struct{ name, body string }{
				{"direct", `<xs:attribute name="global" type="xs:byte"/>`},
				{"named", `<xs:attribute name="global" type="r:Byte"/><xs:simpleType name="Byte"><xs:restriction base="xs:byte"/></xs:simpleType>`},
			} {
				t.Run(typeShape.name+" generation", func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r">` + typeShape.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
					}
					output, err := GenerateGo(schema, "generated")
					if output != nil || err == nil {
						t.Fatalf("GenerateGo = (%q, %v), want no output and unsupported attribute", output, err)
					}
					diagnostic := requireDiagnostic(t, err)
					attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "global"))
					if len(attribute) != 1 || diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != attribute[0].Loc() || !errors.Is(err, errCodegenUnsupported) {
						t.Fatalf("GenerateGo diagnostic = %s, want located unsupported attribute", diagnostic)
					}
				})
			}
		})
	}
}
