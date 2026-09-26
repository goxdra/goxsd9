package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep cross-policy graph, identity, location, and copy checks together.
func TestSchemaIntGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := intGlobalAttributeGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated int attribute builds changed facts or order")
			}
			if got := len(first.Documents()); got != 4 {
				t.Fatalf("document count = %d, want 4 after repeated/cyclic discovery", got)
			}
			want := []struct {
				local, namespace, lexical, source, varietySource, varietyNeedle string
				minimum, maximum                                                string
			}{
				{local: "direct", namespace: "urn:root", lexical: "xs:int", source: "root.xsd"},
				{local: "forward", namespace: "urn:root", lexical: "r:ForwardInt", source: "root.xsd", varietySource: "root.xsd", varietyNeedle: `<xs:restriction base="xs:int"><xs:minInclusive`, minimum: "-2147483648", maximum: "2147483647"},
				{local: "imported", namespace: "urn:root", lexical: "o:ImportedInt", source: "root.xsd", varietySource: "other.xsd", varietyNeedle: `<xs:restriction base="xs:int"`, minimum: "-2147483648", maximum: "2147483647"},
				{local: "chameleon", namespace: "urn:root", lexical: "r:ChameleonInt", source: "root.xsd", varietySource: "chameleon.xsd", varietyNeedle: `<xs:restriction base="xs:int"`, minimum: "-2147483648", maximum: "2147483647"},
				{local: "narrowed", namespace: "urn:root", lexical: "r:NarrowedInt", source: "root.xsd", varietySource: "root.xsd", varietyNeedle: `<xs:restriction base="xs:int"><xs:minExclusive`, minimum: "-3", maximum: "2"},
				{local: "included", namespace: "urn:root", lexical: "r:IncludedInt", source: "ordinary.xsd", varietySource: "ordinary.xsd", varietyNeedle: `<xs:restriction base="xs:int"`, minimum: "-2147483648", maximum: "2147483647"},
				{local: "chameleonDirect", namespace: "urn:root", lexical: "xs:int", source: "chameleon.xsd"},
				{local: "importedDirect", namespace: "urn:other", lexical: "xs:int", source: "other.xsd"},
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
				if !ok || declaration.ID() != component.ID() || declaration.Loc() != schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `<xs:attribute name="`+expected.local+`"`, root, fixtures) {
					t.Fatalf("attribute %s declaration = %#v/%t, want located declaration", expected.local, declaration, ok)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("attribute %s has no type reference", expected.local)
				}
				typeName := mustTestQName(t, testXSDNamespace, "int")
				if expected.varietySource != "" {
					typeName = mustTestQName(t, expected.namespace, expected.lexical[2:])
					if expected.local == "imported" {
						typeName = mustTestQName(t, "urn:other", "ImportedInt")
					}
				}
				wantTypeLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `type="`+expected.lexical+`"`, root, fixtures)
				if declaration.DeclaredType() != typeName || reference.QName() != typeName || reference.Name() != typeName || reference.Loc() != wantTypeLoc {
					t.Fatalf("attribute %s type = %q/%q/%q/%s, want %q at %s", expected.local, declaration.DeclaredType(), reference.QName(), reference.Name(), reference.Loc(), typeName, wantTypeLoc)
				}
				if expected.varietySource == "" {
					assertIntBuiltinReference(t, reference, wantTypeLoc, profile.version)
					if id, present := declaration.TypeID(); present || !id.IsZero() {
						t.Fatalf("built-in attribute type ID = %v/%t, want none", id, present)
					}
					continue
				}
				if !reference.IsNamed() || reference.VarietyLoc() != schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.varietySource), expected.varietyNeedle, root, fixtures) {
					t.Fatalf("named attribute %s kind/variety location = %q/%s", expected.local, reference.Kind(), reference.VarietyLoc())
				}
				wantID := componentIDForName(t, first, typeName)
				if id, present := declaration.TypeID(); !present || id != wantID || id.Source() != SourceID(expected.varietySource) {
					t.Fatalf("named attribute %s target ID = %v/%t, want %v", expected.local, id, present, wantID)
				}
				if id, present := reference.ComponentID(); !present || id != wantID {
					t.Fatalf("named attribute %s reference ID = %v/%t, want %v", expected.local, id, present, wantID)
				}
				if expected.local == "narrowed" {
					assertNarrowedIntAttributeFacts(t, reference, profile.version, root, fixtures)
					continue
				}
				assertIntegerReferenceFacts(t, reference.facts, profile.version, schemaSimpleTypeAtomicInt, "int", expected.minimum, expected.maximum)
			}
			narrowed := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "narrowed"))
			narrowedDeclaration, ok := narrowed[0].AttributeDeclaration()
			if !ok {
				t.Fatal("narrowed declaration view is missing")
			}
			narrowedReference, ok := narrowedDeclaration.TypeReference()
			if !ok {
				t.Fatal("narrowed type reference is missing")
			}
			narrowedFacets, ok := narrowedReference.facts.facets.(schemaIntegerFacetVariant)
			if !ok {
				t.Fatalf("narrowed facets = %T, want integer facets", narrowedReference.facts.facets)
			}
			for _, bound := range narrowedFacets.bounds.Bounds() {
				bound.Value().value.SetInt64(0)
			}
			for _, value := range narrowedFacets.enumeration.Values() {
				value.value.SetInt64(0)
			}
			repeatedReference, ok := narrowedDeclaration.TypeReference()
			if !ok {
				t.Fatal("repeated narrowed type reference is missing")
			}
			assertNarrowedIntAttributeFacts(t, repeatedReference, profile.version, root, fixtures)
			before := first.Components()
			copyComponents := first.Components()
			copyComponents[0] = Component{}
			copyFound := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "forward"))
			copyFound[0] = Component{}
			copyDocument := first.Documents()[0].Components()
			copyDocument[0] = Component{}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied component views changed Schema")
			}
			walked := make([]ComponentID, 0, len(before))
			if err := first.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if len(walked) != len(before) {
				t.Fatalf("walked component count = %d, want %d", len(walked), len(before))
			}
			for index, component := range before {
				if walked[index] != component.ID() {
					t.Fatalf("walked ID %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

func intGlobalAttributeGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:int"/>
  <xs:attribute name="forward" type="r:ForwardInt"/>
  <xs:attribute name="imported" type="o:ImportedInt"/>
  <xs:attribute name="chameleon" type="r:ChameleonInt"/>
  <xs:attribute name="narrowed" type="r:NarrowedInt"/>
  <xs:simpleType name="ForwardInt"><xs:restriction base="xs:int"><xs:minInclusive value="-2147483648"/><xs:maxInclusive value="2147483647"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="NarrowedInt"><xs:restriction base="xs:int"><xs:minExclusive value="-3"/><xs:maxInclusive value="2"/><xs:totalDigits value="1"/><xs:enumeration value="-2"/><xs:enumeration value="0"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedInt"><xs:restriction base="xs:int"/></xs:simpleType>
  <xs:attribute name="included" type="r:IncludedInt"/>
</xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="ChameleonInt"><xs:restriction base="xs:int"/></xs:simpleType><xs:attribute name="chameleonDirect" type="xs:int"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:simpleType name="ImportedInt"><xs:restriction base="xs:int"/></xs:simpleType><xs:attribute name="importedDirect" type="xs:int"/></xs:schema>`},
	}
	return root, fixtures
}

//nolint:gocognit // Keep exact bound, digit, enumeration, and source-location checks together.
func assertNarrowedIntAttributeFacts(t *testing.T, reference SimpleTypeReference, version XSDVersion, root string, fixtures map[string]discoveryFixture) {
	t.Helper()
	if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicInt {
		t.Fatalf("narrowed reference facts = %#v, want atomic int", reference.facts)
	}
	facets, ok := reference.facts.facets.(schemaIntegerFacetVariant)
	if !ok {
		t.Fatalf("narrowed facets = %T, want integer facets", reference.facts.facets)
	}
	bounds := facets.bounds.Bounds()
	if len(bounds) != 2 || bounds[0].Kind() != BoundMinExclusive || bounds[0].Value().Canonical() != "-3" || bounds[1].Kind() != BoundMaxInclusive || bounds[1].Value().Canonical() != "2" {
		t.Fatalf("narrowed bounds = %#v, want (-3, 2]", bounds)
	}
	for index, needle := range []string{`value="-3"`, `value="2"`} {
		if bounds[index].Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", needle, root, fixtures) || bounds[index].Version() != version {
			t.Fatalf("bound %d = %s/%s, want source facet and %s", index, bounds[index].Loc(), bounds[index].Version(), version)
		}
	}
	total, present := facets.digits.TotalDigits()
	if !present || total.Canonical() != "1" {
		t.Fatalf("totalDigits = %q/%t, want 1", total.Canonical(), present)
	}
	if loc, present := facets.digits.TotalDigitsLoc(); !present || loc != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="1"`, root, fixtures) {
		t.Fatalf("totalDigits location = %s/%t, want source facet", loc, present)
	}
	values := facets.enumeration.Values()
	locations := facets.enumeration.Locations()
	if len(values) != 3 || len(locations) != 3 {
		t.Fatalf("enumeration = %v/%v, want three values and locations", values, locations)
	}
	for index, lexical := range []string{"-2", "0", "2"} {
		wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:enumeration value="`+lexical+`"`, root, fixtures)
		if values[index].Canonical() != lexical || locations[index] != wantLoc {
			t.Fatalf("enumeration %d = %q/%s, want %s at %s", index, values[index].Canonical(), locations[index], lexical, wantLoc)
		}
	}
}

//nolint:gocognit // Keep classified diagnostics and source-location checks across all policies together.
func TestSchemaIntGlobalAttributeInvalidAndUnsupportedBoundaries(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, root, needle string
			class              FailureClass
			cause              error
			code               string
		}{
			{name: "below minimum", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:int"><xs:minInclusive value="-2147483649"/></xs:restriction></xs:simpleType></xs:schema>`, needle: `value="-2147483649"`, class: FailureInvalid, cause: errInvalidBoundRestriction},
			{name: "above maximum", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:int"><xs:maxInclusive value="2147483648"/></xs:restriction></xs:simpleType></xs:schema>`, needle: `value="2147483648"`, class: FailureInvalid, cause: errInvalidBoundRestriction},
			{name: "malformed bound", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:int"><xs:minInclusive value="oops"/></xs:restriction></xs:simpleType></xs:schema>`, needle: `value="oops"`, class: FailureInvalid, cause: errInvalidBoundValue},
			{name: "unresolved", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:Missing"/></xs:schema>`, needle: `type="r:Missing"`, class: FailureInvalid, cause: errSchemaAttributeTypeUnresolved, code: diagnosticSchemaAttributeTypeUnresolvedCode},
			{name: "wrong kind", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="T" type="xs:int"/><xs:attribute name="a" type="T"/></xs:schema>`, needle: `type="T"`, class: FailureInvalid, cause: errSchemaAttributeTypeWrongKind, code: diagnosticSchemaAttributeTypeWrongKindCode},
			{name: "default", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="a" type="xs:int" default="1"/></xs:schema>`, needle: `default="1"`, class: FailureUnsupported, cause: errSchemaAttributeValueConstraintUnsupported},
			{name: "fixed named", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:T" fixed="1"/><xs:simpleType name="T"><xs:restriction base="xs:int"/></xs:simpleType></xs:schema>`, needle: `fixed="1"`, class: FailureUnsupported, cause: errSchemaAttributeValueConstraintUnsupported},
			{name: "inline", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:attribute></xs:schema>`, needle: `<xs:simpleType>`, class: FailureUnsupported, cause: ErrUnsupported},
			{name: "local", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="T"><xs:attribute name="a" type="xs:int"/></xs:complexType></xs:schema>`, needle: `type="xs:int"`, class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported},
			{name: "narrower builtin", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="a" type="xs:short"/></xs:schema>`, needle: `type="xs:short"`, class: FailureUnsupported, cause: ErrUnsupported},
			{name: "list", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:list itemType="xs:int"/></xs:simpleType></xs:schema>`, needle: `type="r:T"`, class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported},
			{name: "union", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:union memberTypes="xs:int"/></xs:simpleType></xs:schema>`, needle: `type="r:T"`, class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded int attribute returned a schema or no error")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.needle) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with %v", diagnostic, test.class, test.needle, test.cause)
				}
				if diagnostic.Code() == "" || diagnostic.SpecRef() == "" {
					t.Fatalf("diagnostic = %s, want stable code and specification reference", diagnostic)
				}
				if test.code != "" && diagnostic.Code() != test.code {
					t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
				}
				if test.class == FailureUnsupported && (diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported)) {
					t.Fatalf("diagnostic = %s, want schema-syntax unsupported with cause", diagnostic)
				}
				if errors.Is(test.cause, errSchemaAttributeValueConstraintUnsupported) && errors.Is(err, errSchemaAttributeTypeUnsupported) {
					t.Fatalf("value constraint rejected as unsupported type: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Keep local-use and generator consumer boundaries together across policies.
func TestSchemaIntGlobalAttributeUseAndGeneratorRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="global" type="xs:int"/><xs:complexType name="T"><xs:attribute ref="r:global"/></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("local reference to int attribute returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="r:global"`) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
				t.Fatalf("local reference diagnostic = %s, want located unsupported use", diagnostic)
			}
			if !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="global"`)}) {
				t.Fatalf("related locations = %v, want global declaration", diagnostic.Related())
			}

			root = `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:r"><xs:attribute name="global" type="xs:int"/></xs:schema>`
			schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo = (%q, %v), want no output and unsupported attribute", output, generationErr)
			}
			diagnostic = requireDiagnostic(t, generationErr)
			attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "global"))
			if len(attribute) != 1 || diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != attribute[0].Loc() || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want located unsupported attribute", diagnostic)
			}
		})
	}
}
