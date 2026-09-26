package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the cross-policy graph, location, identity, and copy contract together.
func TestSchemaLongGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := longGlobalAttributeGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated global long attribute builds changed component facts or order")
			}
			if got := len(first.Documents()); got != 4 {
				t.Fatalf("document count = %d, want 4 after repeated/include/import/chameleon discovery", got)
			}

			want := []schemaLongGlobalAttributeCase{
				{
					local:        "direct",
					namespace:    "urn:root",
					declaredType: mustTestQName(t, testXSDNamespace, "long"),
					typeSource:   "root.xsd",
					typeNeedle:   `type="xs:long"`,
				},
				{
					local:          "forward",
					namespace:      "urn:root",
					declaredType:   mustTestQName(t, "urn:root", "ForwardLong"),
					typeSource:     "root.xsd",
					typeNeedle:     `type="r:ForwardLong"`,
					varietySource:  "root.xsd",
					varietyNeedle:  `<xs:restriction base="xs:long"`,
					named:          true,
					boundMin:       "-9223372036854775808",
					boundMax:       "9223372036854775807",
					boundMinNeedle: `value="-9223372036854775808"`,
					boundMaxNeedle: `value="9223372036854775807"`,
				},
				{
					local:         "imported",
					namespace:     "urn:root",
					declaredType:  mustTestQName(t, "urn:other", "ImportedLong"),
					typeSource:    "root.xsd",
					typeNeedle:    `type="o:ImportedLong"`,
					varietySource: "other.xsd",
					varietyNeedle: `<xs:restriction base="xs:long"`,
					named:         true,
				},
				{
					local:         "chameleon",
					namespace:     "urn:root",
					declaredType:  mustTestQName(t, "urn:root", "ChameleonLong"),
					typeSource:    "root.xsd",
					typeNeedle:    `type="r:ChameleonLong"`,
					varietySource: "chameleon.xsd",
					varietyNeedle: `<xs:restriction base="xs:long"`,
					named:         true,
				},
				{
					local:         "narrowed",
					namespace:     "urn:root",
					declaredType:  mustTestQName(t, "urn:root", "NarrowedLong"),
					typeSource:    "root.xsd",
					typeNeedle:    `type="r:NarrowedLong"`,
					varietySource: "root.xsd",
					varietyNeedle: `<xs:restriction base="xs:long"><xs:minInclusive value="-100"`,
					named:         true,
					effectiveBounds: []schemaLongGlobalAttributeBound{
						{kind: BoundMinInclusive, value: "-100", source: "root.xsd", needle: `value="-100"`},
						{kind: BoundMaxExclusive, value: "100", source: "root.xsd", needle: `value="100"`},
					},
				},
				{
					local:         "exclusive",
					namespace:     "urn:root",
					declaredType:  mustTestQName(t, "urn:root", "ExclusiveLong"),
					typeSource:    "root.xsd",
					typeNeedle:    `type="r:ExclusiveLong"`,
					varietySource: "root.xsd",
					varietyNeedle: `<xs:restriction base="xs:long"><xs:minExclusive value="-200"`,
					named:         true,
					effectiveBounds: []schemaLongGlobalAttributeBound{
						{kind: BoundMinExclusive, value: "-200", source: "root.xsd", needle: `value="-200"`},
						{kind: BoundMaxInclusive, value: "200", source: "root.xsd", needle: `value="200"`},
					},
				},
				{
					local:         "includedNamed",
					namespace:     "urn:root",
					declaredType:  mustTestQName(t, "urn:root", "IncludedLong"),
					typeSource:    "ordinary.xsd",
					typeNeedle:    `type="r:IncludedLong"`,
					varietySource: "ordinary.xsd",
					varietyNeedle: `<xs:restriction base="xs:long"`,
					named:         true,
				},
				{
					local:        "includedDirect",
					namespace:    "urn:root",
					declaredType: mustTestQName(t, testXSDNamespace, "long"),
					typeSource:   "ordinary.xsd",
					typeNeedle:   `type="xs:long"`,
				},
				{
					local:        "chameleonDeclared",
					namespace:    "urn:root",
					declaredType: mustTestQName(t, testXSDNamespace, "long"),
					typeSource:   "chameleon.xsd",
					typeNeedle:   `type="xs:long"`,
				},
				{
					local:         "importedNamed",
					namespace:     "urn:other",
					declaredType:  mustTestQName(t, "urn:other", "ImportedLong"),
					typeSource:    "other.xsd",
					typeNeedle:    `type="o:ImportedLong"`,
					varietySource: "other.xsd",
					varietyNeedle: `<xs:restriction base="xs:long"`,
					named:         true,
				},
				{
					local:        "importedDirect",
					namespace:    "urn:other",
					declaredType: mustTestQName(t, testXSDNamespace, "long"),
					typeSource:   "other.xsd",
					typeNeedle:   `type="xs:long"`,
				},
			}
			attributes := globalLongAttributeComponents(first)
			if len(attributes) != len(want) {
				t.Fatalf("global long attribute count = %d, want %d", len(attributes), len(want))
			}
			for index, expected := range want {
				component := attributes[index]
				if component.Name() != mustTestQName(t, expected.namespace, expected.local) {
					t.Fatalf("attribute %d name = %q, want {%s}%s", index, component.Name(), expected.namespace, expected.local)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok {
					t.Fatalf("attribute %q has no declaration view", expected.local)
				}
				if declaration.DeclaredType() != expected.declaredType {
					t.Fatalf("attribute %q declared type = %q, want %q", expected.local, declaration.DeclaredType(), expected.declaredType)
				}
				wantDeclarationLoc := schemaBuiltinReferenceAttributeLoc(t, component.Document(), `<xs:attribute name="`+expected.local+`"`, root, fixtures)
				if declaration.Loc() != wantDeclarationLoc || declaration.ID() != component.ID() {
					t.Fatalf("attribute %q declaration facts = %s/%v, want %s/%v", expected.local, declaration.Loc(), declaration.ID(), wantDeclarationLoc, component.ID())
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Name() != expected.declaredType || reference.QName() != expected.declaredType {
					t.Fatalf("attribute %q type reference = %q/%q/%t, want %q", expected.local, reference.Name(), reference.QName(), ok, expected.declaredType)
				}
				wantTypeLoc := schemaBuiltinReferenceAttributeLoc(t, expected.typeSource, expected.typeNeedle, root, fixtures)
				if reference.Loc() != wantTypeLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("attribute %q reference location/variety = %s/%q, want %s/atomic-restriction", expected.local, reference.Loc(), reference.Variety(), wantTypeLoc)
				}
				if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicLong {
					t.Fatalf("attribute %q facts = %#v, want atomic long", expected.local, reference.facts)
				}
				if expected.named {
					if !reference.IsNamed() || reference.Kind() != SimpleTypeReferenceNamed {
						t.Fatalf("attribute %q reference kind = %q, want named", expected.local, reference.Kind())
					}
					wantTypeID := componentIDForName(t, first, expected.declaredType)
					if typeID, hasTypeID := reference.ComponentID(); !hasTypeID || typeID != wantTypeID || typeID.Source() != expected.varietySource {
						t.Fatalf("attribute %q reference type ID = %v/%t, want %v/true from %q", expected.local, typeID, hasTypeID, wantTypeID, expected.varietySource)
					}
					if typeID, hasTypeID := declaration.TypeID(); !hasTypeID || typeID != wantTypeID {
						t.Fatalf("attribute %q declaration type ID = %v/%t, want %v/true", expected.local, typeID, hasTypeID, wantTypeID)
					}
					wantVarietyLoc := schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.varietyNeedle, root, fixtures)
					if reference.VarietyLoc() != wantVarietyLoc {
						t.Fatalf("attribute %q variety location = %s, want %s", expected.local, reference.VarietyLoc(), wantVarietyLoc)
					}
					if len(expected.effectiveBounds) > 0 {
						assertLongGlobalAttributeEffectiveBounds(t, reference.facts, profile.version, expected.effectiveBounds, root, fixtures)
					}
					if len(expected.effectiveBounds) == 0 {
						assertLongReferenceFacts(t, reference.facts, profile.version, expected.boundMinOrDefault(), expected.boundMaxOrDefault())
					}
					if expected.boundMinNeedle != "" {
						facets, ok := reference.facts.facets.(schemaIntegerFacetVariant)
						if !ok {
							t.Fatalf("attribute %q facets = %T, want located integer facets", expected.local, reference.facts.facets)
						}
						minimum, present := facets.bounds.MinInclusiveFacet()
						if !present || minimum.Loc() != schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.boundMinNeedle, root, fixtures) {
							t.Fatalf("attribute %q minimum facet = %s/%t, want exact source location", expected.local, minimum.Loc(), present)
						}
						maximum, present := facets.bounds.MaxInclusiveFacet()
						if !present || maximum.Loc() != schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.boundMaxNeedle, root, fixtures) {
							t.Fatalf("attribute %q maximum facet = %s/%t, want exact source location", expected.local, maximum.Loc(), present)
						}
					}
					continue
				}
				if !reference.IsBuiltin() || reference.Kind() != SimpleTypeReferenceBuiltin {
					t.Fatalf("attribute %q reference kind = %q, want built-in", expected.local, reference.Kind())
				}
				if reference.VarietyLoc() != wantTypeLoc {
					t.Fatalf("attribute %q built-in variety location = %s, want %s", expected.local, reference.VarietyLoc(), wantTypeLoc)
				}
				if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("attribute %q built-in reference ID = %v/%t, want zero/false", expected.local, typeID, hasTypeID)
				}
				if typeID, hasTypeID := declaration.TypeID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("attribute %q built-in declaration ID = %v/%t, want zero/false", expected.local, typeID, hasTypeID)
				}
				assertLongBuiltinReference(t, reference, wantTypeLoc, profile.version)
			}

			before := first.Components()
			returned := first.Components()
			returned[0] = Component{}
			found := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "forward"))
			if len(found) != 1 {
				t.Fatal("forward attribute lookup did not return one component")
			}
			found[0] = Component{}
			documentComponents := first.Documents()[0].Components()
			documentComponents[0] = Component{}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied global long attribute views changed Schema")
			}

			walked := make([]ComponentID, 0, len(before))
			if err := first.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			wantWalk := make([]ComponentID, 0, len(before))
			for _, component := range before {
				wantWalk = append(wantWalk, component.ID())
			}
			if !reflect.DeepEqual(walked, wantWalk) {
				t.Fatalf("Walk IDs = %#v, want %#v", walked, wantWalk)
			}
		})
	}
}

type schemaLongGlobalAttributeCase struct {
	local           string
	namespace       string
	declaredType    QName
	typeSource      SourceID
	typeNeedle      string
	varietySource   SourceID
	varietyNeedle   string
	named           bool
	effectiveBounds []schemaLongGlobalAttributeBound
	boundMin        string
	boundMax        string
	boundMinNeedle  string
	boundMaxNeedle  string
}

type schemaLongGlobalAttributeBound struct {
	kind   BoundKind
	value  string
	source SourceID
	needle string
}

func assertLongGlobalAttributeEffectiveBounds(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, expected []schemaLongGlobalAttributeBound, root string, fixtures map[string]discoveryFixture) {
	t.Helper()
	if facts == nil || facts.atomicKind != schemaSimpleTypeAtomicLong {
		t.Fatalf("reference facts = %#v, want atomic long facts", facts)
	}
	facets, ok := facts.facets.(schemaIntegerFacetVariant)
	if !ok {
		t.Fatalf("reference facets = %T, want integer facets", facts.facets)
	}
	if facets.digits.Kind() != DigitDatatypeInteger || facets.digits.Version() != version {
		t.Fatalf("integer digit facts = %q/%q, want integer/%q", facets.digits.Kind(), facets.digits.Version(), version)
	}
	fraction, present := facets.digits.FractionDigits()
	if !present || fraction.Canonical() != "0" {
		t.Fatalf("effective fractionDigits = %q/%t, want 0/true", fraction.Canonical(), present)
	}
	fractionFixed, present := facets.digits.FractionDigitsFixed()
	if !present || !fractionFixed {
		t.Fatalf("effective fractionDigits fixed = %t/%t, want true/true", fractionFixed, present)
	}
	if _, present := facets.digits.TotalDigits(); present {
		t.Fatal("named long restriction unexpectedly has totalDigits")
	}
	actual := facets.bounds.Bounds()
	if len(actual) != len(expected) {
		t.Fatalf("effective bounds = %d, want %d: %#v", len(actual), len(expected), actual)
	}
	for index, want := range expected {
		got := actual[index]
		wantLoc := schemaBuiltinReferenceAttributeLoc(t, want.source, want.needle, root, fixtures)
		if got.Kind() != want.kind || got.Value().Canonical() != want.value || got.Loc() != wantLoc || got.Version() != version || got.Fixed() {
			t.Fatalf("effective bound %d = %s/%q/%s/%q/%t, want %s/%q/%s/%q/false", index, got.Kind(), got.Value().Canonical(), got.Loc(), got.Version(), got.Fixed(), want.kind, want.value, wantLoc, version)
		}
	}
}

func (test schemaLongGlobalAttributeCase) boundMinOrDefault() string {
	if test.boundMin != "" {
		return test.boundMin
	}
	return "-9223372036854775808"
}

func (test schemaLongGlobalAttributeCase) boundMaxOrDefault() string {
	if test.boundMax != "" {
		return test.boundMax
	}
	return "9223372036854775807"
}

func globalLongAttributeComponents(schema Schema) []Component {
	components := schema.Components()
	attributes := make([]Component, 0, len(components))
	for _, component := range components {
		if component.Kind() == ComponentKindAttributeDeclaration {
			attributes = append(attributes, component)
		}
	}
	return attributes
}

func longGlobalAttributeGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:long"/>
  <xs:attribute name="forward" type="r:ForwardLong"/>
  <xs:attribute name="imported" type="o:ImportedLong"/>
  <xs:attribute name="chameleon" type="r:ChameleonLong"/>
  <xs:attribute name="narrowed" type="r:NarrowedLong"/>
  <xs:attribute name="exclusive" type="r:ExclusiveLong"/>
  <xs:simpleType name="ForwardLong"><xs:restriction base="xs:long"><xs:minInclusive value="-9223372036854775808"/><xs:maxInclusive value="9223372036854775807"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="NarrowedLong"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxExclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ExclusiveLong"><xs:restriction base="xs:long"><xs:minExclusive value="-200"/><xs:maxInclusive value="200"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedLong"><xs:restriction base="xs:long"/></xs:simpleType>
  <xs:attribute name="includedNamed" type="r:IncludedLong"/>
  <xs:attribute name="includedDirect" type="xs:long"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="ChameleonLong"><xs:restriction base="xs:long"/></xs:simpleType><xs:attribute name="chameleonDeclared" type="xs:long"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedLong"><xs:restriction base="xs:long"/></xs:simpleType>
  <xs:attribute name="importedNamed" type="o:ImportedLong"/>
  <xs:attribute name="importedDirect" type="xs:long"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

func TestSchemaLongGlobalAttributeBoundsAreDefensiveCopies(t *testing.T) {
	root, fixtures := longGlobalAttributeGraphFixtures(XSDVersion11)
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Compatibility)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	component := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "forward"))
	if len(component) != 1 {
		t.Fatalf("forward attribute matches = %d, want one", len(component))
	}
	declaration, ok := component[0].AttributeDeclaration()
	if !ok {
		t.Fatal("forward attribute has no declaration view")
	}
	reference, ok := declaration.TypeReference()
	if !ok || reference.facts == nil {
		t.Fatal("forward attribute has no reference facts")
	}
	facets, ok := reference.facts.facets.(schemaIntegerFacetVariant)
	if !ok {
		t.Fatalf("forward attribute facets = %T, want integer facets", reference.facts.facets)
	}
	minimum, present := facets.bounds.MinInclusive()
	if !present {
		t.Fatal("forward attribute has no minimum bound")
	}
	minimum.value.SetInt64(0)
	maximum, present := facets.bounds.MaxInclusive()
	if !present {
		t.Fatal("forward attribute has no maximum bound")
	}
	maximum.value.SetInt64(0)
	assertLongReferenceFacts(t, reference.facts, XSDVersion11, "-9223372036854775808", "9223372036854775807")
	second, ok := declaration.TypeReference()
	if !ok {
		t.Fatal("repeated forward attribute type reference is missing")
	}
	assertLongReferenceFacts(t, second.facts, XSDVersion11, "-9223372036854775808", "9223372036854775807")
}

func TestSchemaLongGlobalAttributeNamedBoundsAreDefensiveCopiesAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := longGlobalAttributeGraphFixtures(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			for _, test := range []schemaLongGlobalAttributeBoundsCase{
				{
					name: "narrowed",
					bounds: []schemaLongGlobalAttributeBound{
						{kind: BoundMinInclusive, value: "-100", source: "root.xsd", needle: `value="-100"`},
						{kind: BoundMaxExclusive, value: "100", source: "root.xsd", needle: `value="100"`},
					},
				},
				{
					name: "exclusive",
					bounds: []schemaLongGlobalAttributeBound{
						{kind: BoundMinExclusive, value: "-200", source: "root.xsd", needle: `value="-200"`},
						{kind: BoundMaxInclusive, value: "200", source: "root.xsd", needle: `value="200"`},
					},
				},
			} {
				assertLongGlobalAttributeNamedBounds(t, schema, profile.version, test.name, test.bounds, root, fixtures)
			}
		})
	}
}

type schemaLongGlobalAttributeBoundsCase struct {
	name   string
	bounds []schemaLongGlobalAttributeBound
}

func assertLongGlobalAttributeNamedBounds(t *testing.T, schema Schema, version XSDVersion, name string, expected []schemaLongGlobalAttributeBound, root string, fixtures map[string]discoveryFixture) {
	t.Helper()
	component := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", name))
	if len(component) != 1 {
		t.Fatalf("%s attribute matches = %d, want one", name, len(component))
	}
	declaration, ok := component[0].AttributeDeclaration()
	if !ok {
		t.Fatalf("%s attribute has no declaration view", name)
	}
	reference, ok := declaration.TypeReference()
	if !ok || reference.facts == nil {
		t.Fatalf("%s attribute has no type reference facts", name)
	}
	facets, ok := reference.facts.facets.(schemaIntegerFacetVariant)
	if !ok {
		t.Fatalf("%s attribute facets = %T, want integer facets", name, reference.facts.facets)
	}
	for _, bound := range facets.bounds.Bounds() {
		_ = bound.value.value.SetInt64(0)
	}
	for _, bound := range expected {
		zeroLongGlobalAttributeBound(t, facets.bounds, name, bound.kind)
	}
	assertLongGlobalAttributeEffectiveBounds(t, reference.facts, version, expected, root, fixtures)
	repeated, ok := declaration.TypeReference()
	if !ok {
		t.Fatalf("%s repeated type reference is missing", name)
	}
	assertLongGlobalAttributeEffectiveBounds(t, repeated.facts, version, expected, root, fixtures)
}

func zeroLongGlobalAttributeBound(t *testing.T, bounds IntegerBoundFacets, name string, kind BoundKind) {
	t.Helper()
	var value StrictInteger
	var present bool
	switch kind {
	case BoundMinInclusive:
		value, present = bounds.MinInclusive()
	case BoundMinExclusive:
		value, present = bounds.MinExclusive()
	case BoundMaxInclusive:
		value, present = bounds.MaxInclusive()
	case BoundMaxExclusive:
		value, present = bounds.MaxExclusive()
	default:
		t.Fatalf("%s has unknown expected bound %s", name, kind)
	}
	if !present {
		t.Fatalf("%s lost %s bound", name, kind)
	}
	_ = value.value.SetInt64(0)
}

//nolint:gocognit // Keep the cross-policy value-constraint boundary together.
func TestSchemaLongGlobalAttributeValueConstraintsRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, typeCase := range []struct {
			name       string
			prefix     string
			declared   string
			definition string
		}{
			{
				name:     "built-in",
				prefix:   `<xs:schema xmlns:xs="` + testXSDNamespace + `"`,
				declared: "xs:long",
			},
			{
				name:       "named",
				prefix:     `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"`,
				declared:   "r:NamedLong",
				definition: `<xs:simpleType name="NamedLong"><xs:restriction base="xs:long"/></xs:simpleType>`,
			},
		} {
			for _, kind := range []string{"default", "fixed"} {
				t.Run(profile.name+"/"+typeCase.name+"/"+kind, func(t *testing.T) {
					root := typeCase.prefix + ` version="` + string(profile.version) + `"><xs:attribute name="value" type="` + typeCase.declared + `" ` + kind + `="1"/>` + typeCase.definition + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
						t.Fatal("long attribute value constraint was accepted or returned a partial schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
						t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, kind+"=") {
						t.Fatalf("diagnostic location = %s, want %s value location", diagnostic.Loc(), kind)
					}
					if diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(profile.version) {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaAttributeValueConstraintSpecRef(profile.version))
					}
					if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeValueConstraintUnsupported) {
						t.Fatalf("diagnostic lost value-constraint unsupported cause: %v", err)
					}
					if errors.Is(err, errSchemaAttributeTypeUnsupported) {
						t.Fatalf("long type was rejected before the value-constraint boundary: %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep the generation and validation consumer boundaries together.
func TestSchemaLongGlobalAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:attribute name="value" type="xs:long"/>
  <xs:element name="root" type="xs:long"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "value"))
			if len(attribute) != 1 {
				t.Fatalf("long attribute matches = %d, want one", len(attribute))
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want no output and unsupported attribute boundary", output, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || generationDiagnostic.Loc() != attribute[0].Loc() {
				t.Fatalf("GenerateGo diagnostic = %s, want codegen unsupported at global attribute", generationDiagnostic)
			}
			if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("GenerateGo diagnostic lost unsupported cause: %v", generationErr)
			}

			element := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "root"))
			if len(element) != 1 {
				t.Fatalf("long root element matches = %d, want one", len(element))
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test">0</root>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted the long consumer boundary")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("ValidateInstance diagnostic = %s, want unsupported long consumer", validationDiagnostic)
			}
			if validationDiagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(validationDiagnostic.Related(), []Loc{element[0].Loc()}) {
				t.Fatalf("ValidateInstance locations = %s/%v, want instance root/%s", validationDiagnostic.Loc(), validationDiagnostic.Related(), element[0].Loc())
			}
		})
	}
}

func TestSchemaLongGlobalAttributeInvalidRestrictionRemainsLocated(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">
  <xs:attribute name="value" type="r:BadLong"/>
  <xs:simpleType name="BadLong"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("out-of-range named long restriction was accepted or returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `value="9223372036854775808"`) {
				t.Fatalf("diagnostic = %s, want located invalid maxInclusive", diagnostic)
			}
			if !errors.Is(err, errInvalidBoundRestriction) {
				t.Fatalf("diagnostic lost out-of-range restriction cause: %v", err)
			}
		})
	}
}

func TestSchemaLongGlobalAttributeExcludedFamiliesRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name string
			root string
			loc  string
		}{
			{name: "nonNegativeInteger", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:nonNegativeInteger"/></xs:schema>`, loc: "type="},
			{name: "nonPositiveInteger", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:nonPositiveInteger"/></xs:schema>`, loc: "type="},
			{name: "short", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:short"/></xs:schema>`, loc: "type="},
			{name: "byte", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:byte"/></xs:schema>`, loc: "type="},
			{name: "list", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:LongList"/><xs:simpleType name="LongList"><xs:list itemType="xs:long"/></xs:simpleType></xs:schema>`, loc: `type="r:LongList"`},
			{name: "union", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:LongUnion"/><xs:simpleType name="LongUnion"><xs:union memberTypes="xs:long"/></xs:simpleType></xs:schema>`, loc: `type="r:LongUnion"`},
			{name: "inline", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute></xs:schema>`, loc: "<xs:simpleType>"},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("excluded global attribute type %q was accepted or returned a partial schema", test.name)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.loc) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want located unsupported %q", diagnostic, test.name)
				}
			})
		}
	}
}
