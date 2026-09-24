package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep the cross-policy graph, identity, location, and copy contract together.
func TestSchemaUnsignedLongGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := unsignedLongGlobalAttributeGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated global unsignedLong attribute builds changed component facts or order")
			}
			if got := len(first.Documents()); got != 4 {
				t.Fatalf("document count = %d, want 4 after repeated/include/import/chameleon discovery", got)
			}

			want := []schemaUnsignedLongGlobalAttributeCase{
				{
					local:             "direct",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, testXSDNamespace, "unsignedLong"),
					declarationSource: "root.xsd",
					typeSource:        "root.xsd",
					typeNeedle:        `type="xs:unsignedLong"`,
				},
				{
					local:             "forward",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, "urn:root", "ForwardUnsignedLong"),
					declarationSource: "root.xsd",
					typeSource:        "root.xsd",
					typeNeedle:        `type="r:ForwardUnsignedLong"`,
					varietySource:     "root.xsd",
					varietyNeedle:     `<xs:restriction base="xs:unsignedLong"><xs:minInclusive value="0"`,
					named:             true,
					minimum:           unsignedLongMinimum,
					maximum:           unsignedLongMaximum,
					minimumNeedle:     `value="0"`,
					maximumNeedle:     `value="18446744073709551615"`,
				},
				{
					local:             "imported",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, "urn:other", "ImportedUnsignedLong"),
					declarationSource: "root.xsd",
					typeSource:        "root.xsd",
					typeNeedle:        `type="o:ImportedUnsignedLong"`,
					varietySource:     "other.xsd",
					varietyNeedle:     `<xs:restriction base="xs:unsignedLong"`,
					named:             true,
					minimum:           unsignedLongMinimum,
					maximum:           unsignedLongMaximum,
				},
				{
					local:             "chameleon",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, "urn:root", "ChameleonUnsignedLong"),
					declarationSource: "root.xsd",
					typeSource:        "root.xsd",
					typeNeedle:        `type="r:ChameleonUnsignedLong"`,
					varietySource:     "chameleon.xsd",
					varietyNeedle:     `<xs:restriction base="xs:unsignedLong"`,
					named:             true,
					minimum:           unsignedLongMinimum,
					maximum:           unsignedLongMaximum,
				},
				{
					local:             "narrowed",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, "urn:root", "NarrowedUnsignedLong"),
					declarationSource: "root.xsd",
					typeSource:        "root.xsd",
					typeNeedle:        `type="r:NarrowedUnsignedLong"`,
					varietySource:     "root.xsd",
					varietyNeedle:     `<xs:restriction base="xs:unsignedLong"><xs:minInclusive value="1"`,
					named:             true,
					minimum:           "1",
					maximum:           "2",
					minimumNeedle:     `value="1"`,
					maximumNeedle:     `value="2"`,
				},
				{
					local:             "includedNamed",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, "urn:root", "IncludedUnsignedLong"),
					declarationSource: "ordinary.xsd",
					typeSource:        "ordinary.xsd",
					typeNeedle:        `type="r:IncludedUnsignedLong"`,
					varietySource:     "ordinary.xsd",
					varietyNeedle:     `<xs:restriction base="xs:unsignedLong"`,
					named:             true,
					minimum:           unsignedLongMinimum,
					maximum:           unsignedLongMaximum,
				},
				{
					local:             "includedDirect",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, testXSDNamespace, "unsignedLong"),
					declarationSource: "ordinary.xsd",
					typeSource:        "ordinary.xsd",
					typeNeedle:        `type="xs:unsignedLong"`,
				},
				{
					local:             "chameleonDeclared",
					namespace:         "urn:root",
					declaredType:      mustTestQName(t, testXSDNamespace, "unsignedLong"),
					declarationSource: "chameleon.xsd",
					typeSource:        "chameleon.xsd",
					typeNeedle:        `type="xs:unsignedLong"`,
				},
				{
					local:             "importedNamed",
					namespace:         "urn:other",
					declaredType:      mustTestQName(t, "urn:other", "ImportedUnsignedLong"),
					declarationSource: "other.xsd",
					typeSource:        "other.xsd",
					typeNeedle:        `type="o:ImportedUnsignedLong"`,
					varietySource:     "other.xsd",
					varietyNeedle:     `<xs:restriction base="xs:unsignedLong"`,
					named:             true,
					minimum:           unsignedLongMinimum,
					maximum:           unsignedLongMaximum,
				},
				{
					local:             "importedDirect",
					namespace:         "urn:other",
					declaredType:      mustTestQName(t, testXSDNamespace, "unsignedLong"),
					declarationSource: "other.xsd",
					typeSource:        "other.xsd",
					typeNeedle:        `type="xs:unsignedLong"`,
				},
			}
			attributes := unsignedLongGlobalAttributeComponents(first)
			if len(attributes) != len(want) {
				t.Fatalf("global unsignedLong attribute count = %d, want %d", len(attributes), len(want))
			}
			for index, expected := range want {
				component := attributes[index]
				if component.Name() != mustTestQName(t, expected.namespace, expected.local) {
					t.Fatalf("attribute %d name = %q, want {%s}%s", index, component.Name(), expected.namespace, expected.local)
				}
				if component.ID().Source() != expected.declarationSource || component.ID().Ordinal() == 0 {
					t.Fatalf("attribute %q identity = %v, want nonzero ordinal from %q", expected.local, component.ID(), expected.declarationSource)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok {
					t.Fatalf("attribute %q has no declaration view", expected.local)
				}
				if declaration.DeclaredType() != expected.declaredType || declaration.ID() != component.ID() {
					t.Fatalf("attribute %q declaration facts = %q/%v, want %q/%v", expected.local, declaration.DeclaredType(), declaration.ID(), expected.declaredType, component.ID())
				}
				wantDeclarationLoc := schemaBuiltinReferenceAttributeLoc(t, expected.declarationSource, `<xs:attribute name="`+expected.local+`"`, root, fixtures)
				if declaration.Loc() != wantDeclarationLoc {
					t.Fatalf("attribute %q declaration location = %s, want %s", expected.local, declaration.Loc(), wantDeclarationLoc)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Name() != expected.declaredType || reference.QName() != expected.declaredType {
					t.Fatalf("attribute %q type reference = %q/%q/%t, want %q", expected.local, reference.Name(), reference.QName(), ok, expected.declaredType)
				}
				wantTypeLoc := schemaBuiltinReferenceAttributeLoc(t, expected.typeSource, expected.typeNeedle, root, fixtures)
				if reference.Loc() != wantTypeLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("attribute %q reference location/variety = %s/%q, want %s/atomic-restriction", expected.local, reference.Loc(), reference.Variety(), wantTypeLoc)
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
					assertIntegerReferenceFacts(t, reference.facts, profile.version, schemaSimpleTypeAtomicUnsignedLong, "unsignedLong", expected.minimum, expected.maximum)
					if expected.minimumNeedle != "" {
						facets, ok := reference.facts.facets.(schemaIntegerFacetVariant)
						if !ok {
							t.Fatalf("attribute %q facets = %T, want located integer facets", expected.local, reference.facts.facets)
						}
						minimum, present := facets.bounds.MinInclusiveFacet()
						if !present || minimum.Loc() != schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.minimumNeedle, root, fixtures) {
							t.Fatalf("attribute %q minimum facet = %s/%t, want exact source location", expected.local, minimum.Loc(), present)
						}
						maximum, present := facets.bounds.MaxInclusiveFacet()
						if !present || maximum.Loc() != schemaBuiltinReferenceAttributeLoc(t, expected.varietySource, expected.maximumNeedle, root, fixtures) {
							t.Fatalf("attribute %q maximum facet = %s/%t, want exact source location", expected.local, maximum.Loc(), present)
						}
					}
					continue
				}
				if !reference.IsBuiltin() || reference.Kind() != SimpleTypeReferenceBuiltin {
					t.Fatalf("attribute %q reference kind = %q, want built-in", expected.local, reference.Kind())
				}
				if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("attribute %q built-in reference ID = %v/%t, want zero/false", expected.local, typeID, hasTypeID)
				}
				if typeID, hasTypeID := declaration.TypeID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("attribute %q built-in declaration ID = %v/%t, want zero/false", expected.local, typeID, hasTypeID)
				}
				assertUnsignedLongBuiltinReference(t, reference, wantTypeLoc, profile.version)
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
				t.Fatal("mutating copied global unsignedLong attribute views changed Schema")
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

type schemaUnsignedLongGlobalAttributeCase struct {
	local             string
	namespace         string
	declaredType      QName
	declarationSource SourceID
	typeSource        SourceID
	typeNeedle        string
	varietySource     SourceID
	varietyNeedle     string
	named             bool
	minimum           string
	maximum           string
	minimumNeedle     string
	maximumNeedle     string
}

func unsignedLongGlobalAttributeComponents(schema Schema) []Component {
	components := schema.Components()
	attributes := make([]Component, 0, len(components))
	for _, component := range components {
		if component.Kind() == ComponentKindAttributeDeclaration {
			attributes = append(attributes, component)
		}
	}
	return attributes
}

func unsignedLongGlobalAttributeGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:unsignedLong"/>
  <xs:attribute name="forward" type="r:ForwardUnsignedLong"/>
  <xs:attribute name="imported" type="o:ImportedUnsignedLong"/>
  <xs:attribute name="chameleon" type="r:ChameleonUnsignedLong"/>
  <xs:attribute name="narrowed" type="r:NarrowedUnsignedLong"/>
  <xs:simpleType name="ForwardUnsignedLong"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="0"/><xs:maxInclusive value="18446744073709551615"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="NarrowedUnsignedLong"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="1"/><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root},
		"ordinary.xsd": {
			id: "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="IncludedUnsignedLong"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
  <xs:attribute name="includedNamed" type="r:IncludedUnsignedLong"/>
  <xs:attribute name="includedDirect" type="xs:unsignedLong"/>
</xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="ChameleonUnsignedLong"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:attribute name="chameleonDeclared" type="xs:unsignedLong"/></xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">
  <xs:simpleType name="ImportedUnsignedLong"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
  <xs:attribute name="importedNamed" type="o:ImportedUnsignedLong"/>
  <xs:attribute name="importedDirect" type="xs:unsignedLong"/>
</xs:schema>`,
		},
	}
	return root, fixtures
}

//nolint:gocognit // Keep the copied named-bound contract together.
func TestSchemaUnsignedLongGlobalAttributeNamedBoundsAreDefensiveCopies(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := unsignedLongGlobalAttributeGraphFixtures(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			components := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "narrowed"))
			if len(components) != 1 {
				t.Fatalf("narrowed attribute matches = %d, want one", len(components))
			}
			declaration, ok := components[0].AttributeDeclaration()
			if !ok {
				t.Fatal("narrowed attribute has no declaration view")
			}
			reference, ok := declaration.TypeReference()
			if !ok || reference.facts == nil {
				t.Fatal("narrowed attribute has no type reference facts")
			}
			facets, ok := reference.facts.facets.(schemaIntegerFacetVariant)
			if !ok {
				t.Fatalf("narrowed attribute facets = %T, want integer facets", reference.facts.facets)
			}
			for _, bound := range facets.bounds.Bounds() {
				_ = bound.value.value.SetInt64(0)
			}
			assertIntegerReferenceFacts(t, reference.facts, profile.version, schemaSimpleTypeAtomicUnsignedLong, "unsignedLong", "1", "2")
			repeated, ok := declaration.TypeReference()
			if !ok {
				t.Fatal("repeated narrowed attribute type reference is missing")
			}
			assertIntegerReferenceFacts(t, repeated.facts, profile.version, schemaSimpleTypeAtomicUnsignedLong, "unsignedLong", "1", "2")
			_ = root
		})
	}
}

//nolint:gocognit // Keep invalid lexical, facet, and reference precedence together.
func TestSchemaUnsignedLongGlobalAttributeInvalidFormsRemainLocated(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct {
			name       string
			root       string
			locNeedle  string
			cause      error
			code       string
			related    string
			specPrefix string
		}{
			{
				name:      "negative restriction",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:Bad"/><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="-1"/></xs:restriction></xs:simpleType></xs:schema>`,
				locNeedle: `value="-1"`,
				cause:     errInvalidBoundRestriction,
			},
			{
				name:      "above maximum restriction",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:Bad"/><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="18446744073709551616"/></xs:restriction></xs:simpleType></xs:schema>`,
				locNeedle: `value="18446744073709551616"`,
				cause:     errInvalidBoundRestriction,
			},
			{
				name:      "malformed restriction",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:Bad"/><xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType></xs:schema>`,
				locNeedle: `value="not-an-integer"`,
				cause:     errInvalidBoundValue,
			},
			{
				name:       "unresolved type",
				root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:Missing"/></xs:schema>`,
				locNeedle:  `type="r:Missing"`,
				cause:      errSchemaAttributeTypeUnresolved,
				code:       diagnosticSchemaAttributeTypeUnresolvedCode,
				specPrefix: "attribute",
			},
			{
				name:       "wrong kind type",
				root:       `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="NotAType" type="xs:integer"/><xs:attribute name="value" type="NotAType"/></xs:schema>`,
				locNeedle:  `type="NotAType"`,
				cause:      errSchemaAttributeTypeWrongKind,
				code:       diagnosticSchemaAttributeTypeWrongKindCode,
				specPrefix: "attribute",
				related:    `<xs:element name="NotAType"`,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("invalid unsignedLong global attribute form was accepted or returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.locNeedle) {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), elementReferenceTestAttributeLoc(t, test.root, test.locNeedle))
				}
				if !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
				}
				if test.code != "" && diagnostic.Code() != test.code {
					t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
				}
				if test.specPrefix == "" {
					if diagnostic.Class() != FailureInvalid || !stringsHasVersionedSpecPrefix(diagnostic.SpecRef(), profile.version) {
						t.Fatalf("diagnostic = %s, want located versioned invalid diagnostic", diagnostic)
					}
				}
				if test.specPrefix != "" {
					if diagnostic.Class() != FailureInvalid || diagnostic.SpecRef() != schemaAttributeTypeSpecRef(profile.version) {
						t.Fatalf("diagnostic = %s, want located attribute type diagnostic", diagnostic)
					}
				}
				if test.related != "" && len(diagnostic.Related()) == 0 {
					t.Fatal("wrong-kind diagnostic has no related declaration location")
				}
			})
		}
	}
}

func stringsHasVersionedSpecPrefix(specRef string, version XSDVersion) bool {
	return len(specRef) > len(versionedSpecPrefix(version)) && specRef[:len(versionedSpecPrefix(version))] == versionedSpecPrefix(version)
}

//nolint:gocognit // Keep unsupported global-attribute shapes and value precedence together.
func TestSchemaUnsignedLongGlobalAttributeUnsupportedBoundaries(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct {
			name          string
			root          string
			locNeedle     string
			cause         error
			valueBoundary bool
		}{
			{
				name:          "default built-in",
				root:          `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:unsignedLong" default="0"/></xs:schema>`,
				locNeedle:     `default="0"`,
				cause:         errSchemaAttributeValueConstraintUnsupported,
				valueBoundary: true,
			},
			{
				name:          "fixed named",
				root:          `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:Value" fixed="0"/><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:schema>`,
				locNeedle:     `fixed="0"`,
				cause:         errSchemaAttributeValueConstraintUnsupported,
				valueBoundary: true,
			},
			{
				name:      "local inline",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:attribute></xs:schema>`,
				locNeedle: "<xs:simpleType>",
			},
			{
				name:      "narrower unsigned builtin",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:unsignedInt"/></xs:schema>`,
				locNeedle: "type=",
			},
			{
				name:      "list",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:ValueList"/><xs:simpleType name="ValueList"><xs:list itemType="xs:unsignedLong"/></xs:simpleType></xs:schema>`,
				locNeedle: `type="r:ValueList"`,
				cause:     errSchemaAttributeTypeUnsupported,
			},
			{
				name:      "union",
				root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:ValueUnion"/><xs:simpleType name="ValueUnion"><xs:union memberTypes="xs:unsignedLong"/></xs:simpleType></xs:schema>`,
				locNeedle: `type="r:ValueUnion"`,
				cause:     errSchemaAttributeTypeUnsupported,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("unsupported unsignedLong global attribute shape was accepted or returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
				}
				if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.locNeedle) {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), elementReferenceTestAttributeLoc(t, test.root, test.locNeedle))
				}
				if !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic lost unsupported cause: %v", err)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost unsupported cause %v: %v", test.cause, err)
				}
				if test.valueBoundary {
					if diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(profile.version) {
						t.Fatalf("value-constraint diagnostic SpecRef() = %q, want %q", diagnostic.SpecRef(), schemaAttributeValueConstraintSpecRef(profile.version))
					}
					if errors.Is(err, errSchemaAttributeTypeUnsupported) {
						t.Fatalf("unsignedLong type was rejected before value-constraint boundary: %v", err)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Keep the generation consumer boundary together.
func TestSchemaUnsignedLongGlobalAttributeConsumerBoundaryRemainsUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `"><xs:attribute name="value" type="xs:unsignedLong"/><xs:element name="root" type="xs:unsignedLong"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "value"))
			if len(attribute) != 1 {
				t.Fatalf("unsignedLong attribute matches = %d, want one", len(attribute))
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want no output and unsupported attribute boundary", output, generationErr)
			}
			diagnostic := requireDiagnostic(t, generationErr)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Feature() != FeatureCodegen || diagnostic.Loc() != attribute[0].Loc() {
				t.Fatalf("GenerateGo diagnostic = %s, want codegen unsupported at global attribute", diagnostic)
			}
			if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("GenerateGo diagnostic lost unsupported cause: %v", generationErr)
			}
		})
	}
}
