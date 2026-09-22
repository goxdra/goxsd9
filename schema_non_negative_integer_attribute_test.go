package goxsd9

import (
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep the cross-policy global attribute contract together.
func TestSchemaBridgeRetainsGlobalNonNegativeIntegerAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := globalNonNegativeIntegerAttributeSchemaRoot(profile.version)
			fixtures := map[string]discoveryFixture{
				"ordinary.xsd": {
					id:       "ordinary.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Ordinary"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`,
				},
				"chameleon.xsd": {
					id:       "chameleon.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`,
				},
				"other.xsd": {
					id:       "other.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`,
				},
			}
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if first.LanguagePolicy() != profile.policy {
				t.Fatalf("LanguagePolicy = %q, want %q", first.LanguagePolicy(), profile.policy)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated nonNegativeInteger attribute builds changed component facts or order")
			}

			want := []struct {
				name          string
				declaredType  QName
				typeLexical   string
				typeSource    SourceID
				varietySource SourceID
				varietyNeedle string
				minimum       string
				minimumNeedle string
				builtin       bool
			}{
				{name: "direct", declaredType: mustTestQName(t, testXSDNamespace, "nonNegativeInteger"), typeLexical: "xs:nonNegativeInteger", varietySource: "root.xsd", varietyNeedle: `type="xs:nonNegativeInteger"`, minimum: "0", builtin: true},
				{name: "forward", declaredType: mustTestQName(t, "urn:root", "Forward"), typeLexical: "r:Forward", typeSource: "root.xsd", varietySource: "root.xsd", varietyNeedle: `<xs:restriction base="r:Later"/>`, minimum: "0"},
				{name: "imported", declaredType: mustTestQName(t, "urn:other", "Imported"), typeLexical: "o:Imported", typeSource: "other.xsd", varietySource: "other.xsd", varietyNeedle: `<xs:restriction base="xs:nonNegativeInteger"/>`, minimum: "0"},
				{name: "included", declaredType: mustTestQName(t, "urn:root", "Ordinary"), typeLexical: "r:Ordinary", typeSource: "ordinary.xsd", varietySource: "ordinary.xsd", varietyNeedle: `<xs:restriction base="xs:nonNegativeInteger"/>`, minimum: "0"},
				{name: "chameleon", declaredType: mustTestQName(t, "urn:root", "Included"), typeLexical: "r:Included", typeSource: "chameleon.xsd", varietySource: "chameleon.xsd", varietyNeedle: `<xs:restriction base="xs:nonNegativeInteger"/>`, minimum: "0"},
				{name: "narrowed", declaredType: mustTestQName(t, "urn:root", "Narrowed"), typeLexical: "r:Narrowed", typeSource: "root.xsd", varietySource: "root.xsd", varietyNeedle: `<xs:restriction base="r:Later"><xs:minInclusive`, minimum: "10", minimumNeedle: `value="10"`},
			}
			components := make([]Component, 0, len(want))
			for _, component := range first.Components() {
				if component.Kind() == ComponentKindAttributeDeclaration {
					components = append(components, component)
				}
			}
			if len(components) != len(want) {
				t.Fatalf("global attribute count = %d, want %d", len(components), len(want))
			}
			for index, expected := range want {
				component := components[index]
				if component.Name() != mustTestQName(t, "urn:root", expected.name) {
					t.Fatalf("attribute %d name = %q, want %q", index, component.Name(), expected.name)
				}
				if component.ID().Source() != "root.xsd" || component.ID().Ordinal() != uint64(index+1) {
					t.Fatalf("attribute %q identity = %v, want root.xsd ordinal %d", expected.name, component.ID(), index+1)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok {
					t.Fatalf("attribute %q has no declaration view", expected.name)
				}
				if declaration.DeclaredType() != expected.declaredType {
					t.Fatalf("attribute %q declared type = %q, want %q", expected.name, declaration.DeclaredType(), expected.declaredType)
				}
				wantDeclarationLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:attribute name="`+expected.name+`"`, root, fixtures)
				if declaration.Loc() != wantDeclarationLoc {
					t.Fatalf("attribute %q declaration location = %s, want %s", expected.name, declaration.Loc(), wantDeclarationLoc)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("attribute %q type reference is missing", expected.name)
				}
				wantTypeLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `type="`+expected.typeLexical+`"`, root, fixtures)
				if reference.Name() != expected.declaredType || reference.QName() != expected.declaredType || reference.Loc() != wantTypeLoc {
					t.Fatalf("attribute %q reference = %q/%q/%s, want expanded QName and type location", expected.name, reference.Name(), reference.QName(), reference.Loc())
				}
				if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.VarietyLoc() != schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.varietyNeedle, root, fixtures) {
					t.Fatalf("attribute %q reference variety = %q at %s, want atomic restriction at target model", expected.name, reference.Variety(), reference.VarietyLoc())
				}
				wantMinimumLoc := Loc{}
				if expected.minimumNeedle != "" {
					wantMinimumLoc = schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.minimumNeedle, root, fixtures)
				}
				assertGlobalNonNegativeIntegerAttributeFacts(t, reference.facts, profile.version, expected.minimum, wantMinimumLoc)
				if expected.builtin {
					if !reference.IsBuiltin() {
						t.Fatalf("attribute %q reference kind = %q, want built-in", expected.name, reference.Kind())
					}
					if typeID, hasTypeID := declaration.TypeID(); hasTypeID || !typeID.IsZero() {
						t.Fatalf("attribute %q built-in type ID = %v/%t, want zero/false", expected.name, typeID, hasTypeID)
					}
					if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
						t.Fatalf("attribute %q built-in reference ID = %v/%t, want zero/false", expected.name, typeID, hasTypeID)
					}
					continue
				}
				if !reference.IsNamed() {
					t.Fatalf("attribute %q reference kind = %q, want named", expected.name, reference.Kind())
				}
				typeID, hasTypeID := reference.ComponentID()
				wantTypeID := componentIDForName(t, first, expected.declaredType)
				if !hasTypeID || typeID != wantTypeID || typeID.Source() != expected.typeSource {
					t.Fatalf("attribute %q type ID = %v/%t, want %v/true from %q", expected.name, typeID, hasTypeID, wantTypeID, expected.typeSource)
				}
				declarationTypeID, declarationHasTypeID := declaration.TypeID()
				if !declarationHasTypeID || declarationTypeID != typeID {
					t.Fatalf("attribute %q declaration type ID = %v/%t, want reference ID %v/true", expected.name, declarationTypeID, declarationHasTypeID, typeID)
				}
			}

			before := first.Components()
			returned := first.Components()
			returned[0] = Component{}
			found := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "direct"))
			found[0] = Component{}
			documentComponents := first.Documents()[0].Components()
			documentComponents[0] = Component{}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied nonNegativeInteger attribute views changed Schema")
			}
			direct, ok := components[0].AttributeDeclaration()
			if !ok {
				t.Fatal("direct nonNegativeInteger attribute view is missing")
			}
			directReference, ok := direct.TypeReference()
			if !ok || directReference.facts == nil {
				t.Fatal("direct nonNegativeInteger type reference is missing")
			}
			directFacets, ok := directReference.facts.facets.(schemaDigitFacetVariant)
			if !ok {
				t.Fatal("direct nonNegativeInteger reference has non-digit facets")
			}
			minimum, present := directFacets.integerBounds.MinInclusive()
			if !present {
				t.Fatal("direct nonNegativeInteger reference has no effective minInclusive")
			}
			minimum.value.SetInt64(1)
			storedMinimum, present := directFacets.integerBounds.MinInclusive()
			if !present || storedMinimum.Canonical() != "0" {
				t.Fatalf("mutating returned minInclusive changed stored bound to %q", storedMinimum.Canonical())
			}
		})
	}
}

//nolint:gocognit // Keep exact integer facet assertions together.
func assertGlobalNonNegativeIntegerAttributeFacts(t *testing.T, facts *schemaSimpleTypeReferenceComponent, version XSDVersion, wantMinimum string, wantMinimumLoc Loc) {
	t.Helper()
	if facts == nil || facts.atomicKind != schemaSimpleTypeAtomicNonNegativeInteger {
		t.Fatalf("attribute reference facts = %#v, want nonNegativeInteger facts", facts)
	}
	var digits DigitFacets
	var bounds IntegerBoundFacets
	switch facets := facts.facets.(type) {
	case schemaDigitFacetVariant:
		digits = facets.value
		bounds = facets.integerBounds
	case schemaIntegerFacetVariant:
		digits = facets.digits
		bounds = facets.bounds
	default:
		t.Fatalf("attribute reference facets = %T, want integer facts", facts.facets)
	}
	if digits.Kind() != DigitDatatypeInteger || digits.Version() != version {
		t.Fatalf("attribute digit facts = %q/%q, want integer/%q", digits.Kind(), digits.Version(), version)
	}
	fraction, present := digits.FractionDigits()
	if !present || fraction.Canonical() != "0" {
		t.Fatalf("attribute fractionDigits = %q/%t, want 0/true", fraction.Canonical(), present)
	}
	fractionFixed, present := digits.FractionDigitsFixed()
	if !present || !fractionFixed {
		t.Fatalf("attribute fractionDigits fixed = %t/%t, want true/true", fractionFixed, present)
	}
	if _, hasTotalDigits := digits.TotalDigits(); hasTotalDigits {
		t.Fatal("attribute nonNegativeInteger unexpectedly has totalDigits")
	}
	if bounds.Version() != version {
		t.Fatalf("attribute bound version = %q, want %q", bounds.Version(), version)
	}
	minimum, present := bounds.MinInclusive()
	if !present || minimum.Canonical() != wantMinimum {
		t.Fatalf("attribute minInclusive = %q/%t, want %s/true", minimum.Canonical(), present, wantMinimum)
	}
	minimumFacet, present := bounds.MinInclusiveFacet()
	if !present || minimumFacet.Kind() != BoundMinInclusive || minimumFacet.Value().Canonical() != wantMinimum || minimumFacet.Loc() != wantMinimumLoc || minimumFacet.Version() != version {
		t.Fatalf("attribute minInclusive facts = %q/%s/%q/%t, want %s/%s/%q/true", minimumFacet.Value().Canonical(), minimumFacet.Loc(), minimumFacet.Kind(), present, wantMinimum, wantMinimumLoc, version)
	}
	if _, present := bounds.MaxInclusive(); present {
		t.Fatal("attribute nonNegativeInteger unexpectedly has an effective upper bound")
	}
	ordered := bounds.Bounds()
	if len(ordered) != 1 || ordered[0].Kind() != BoundMinInclusive {
		t.Fatalf("attribute ordered integer bounds = %#v, want one minInclusive", ordered)
	}
}

func globalNonNegativeIntegerAttributeSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:nonNegativeInteger"/>
  <xs:attribute name="forward" type="r:Forward"/>
  <xs:attribute name="imported" type="o:Imported"/>
  <xs:attribute name="included" type="r:Ordinary"/>
  <xs:attribute name="chameleon" type="r:Included"/>
  <xs:attribute name="narrowed" type="r:Narrowed"/>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="r:Later"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType>
</xs:schema>`
}
