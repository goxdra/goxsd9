package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func shortGlobalAttributeGraphFixtures(version XSDVersion) (string, map[string]discoveryFixture) {
	root, fixtures := intGlobalAttributeGraphFixtures(version)
	replace := strings.NewReplacer("Int", "Short", "int", "short", "-2147483648", "-32768", "2147483647", "32767")
	root = replace.Replace(root)
	for _, source := range []string{"root.xsd", "ordinary.xsd", "chameleon.xsd", "other.xsd"} {
		fixture := fixtures[source]
		fixture.contents = replace.Replace(fixture.contents)
		fixtures[source] = fixture
	}
	return root, fixtures
}

func shortAttributePolicyProfiles() []longPolicyProfile {
	return append([]longPolicyProfile{{name: "Compatibility XSD 1.0", policy: Compatibility, version: XSDVersion10}}, longPolicyProfiles()...)
}

//nolint:gocognit,funlen // Exercise ordering, provenance, and copied facts in one graph.
func TestSchemaShortGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			builtinVersion := profile.version
			if profile.policy == Compatibility {
				builtinVersion = XSDVersion11
			}
			root, fixtures := shortGlobalAttributeGraphFixtures(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) || len(first.Documents()) != 4 {
				t.Fatal("repeated short attribute graph changed facts, order, or discovery count")
			}
			want := []struct {
				local, namespace, lexical, source, typeSource string
				minimum, maximum                              string
			}{
				{"direct", "urn:root", "xs:short", "root.xsd", "", "-32768", "32767"},
				{"forward", "urn:root", "r:ForwardShort", "root.xsd", "root.xsd", "-32768", "32767"},
				{"imported", "urn:root", "o:ImportedShort", "root.xsd", "other.xsd", "-32768", "32767"},
				{"chameleon", "urn:root", "r:ChameleonShort", "root.xsd", "chameleon.xsd", "-32768", "32767"},
				{"narrowed", "urn:root", "r:NarrowedShort", "root.xsd", "root.xsd", "-3", "2"},
				{"included", "urn:root", "r:IncludedShort", "ordinary.xsd", "ordinary.xsd", "-32768", "32767"},
				{"chameleonDirect", "urn:root", "xs:short", "chameleon.xsd", "", "-32768", "32767"},
				{"importedDirect", "urn:other", "xs:short", "other.xsd", "", "-32768", "32767"},
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
				typeName := mustTestQName(t, testXSDNamespace, "short")
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
					assertShortBuiltinReference(t, reference, wantLoc, builtinVersion)
					if id, present := declaration.TypeID(); present || !id.IsZero() {
						t.Fatalf("built-in short type ID = %v/%t, want none", id, present)
					}
				}
				if expected.typeSource != "" {
					wantID := componentIDForName(t, first, typeName)
					id, present := declaration.TypeID()
					if !reference.IsNamed() || !present || id != wantID || id.Source() != SourceID(expected.typeSource) {
						t.Fatalf("named short attribute %s ID = %v/%t, want %v", expected.local, id, present, wantID)
					}
					if expected.local == "narrowed" {
						assertNarrowedShortAttributeFacts(t, reference, builtinVersion, root, fixtures)
					}
					if expected.local != "narrowed" {
						assertIntegerReferenceFacts(t, reference.facts, builtinVersion, schemaSimpleTypeAtomicShort, "short", expected.minimum, expected.maximum)
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
		})
	}
}

func assertNarrowedShortAttributeFacts(t *testing.T, reference SimpleTypeReference, version XSDVersion, root string, fixtures map[string]discoveryFixture) {
	t.Helper()
	if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicShort {
		t.Fatalf("narrowed atomic kind = %#v, want short", reference.facts)
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
	values := facets.enumeration.Values()
	locations := facets.enumeration.Locations()
	for index, lexical := range []string{"-2", "0", "2"} {
		if len(values) != 3 || len(locations) != 3 || values[index].Canonical() != lexical || locations[index] != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:enumeration value="`+lexical+`"`, root, fixtures) {
			t.Fatalf("enumeration = %v/%v, want three ordered values and locations", values, locations)
		}
	}
}

//nolint:gocognit // Check classification and the primary source location for each boundary.
func TestSchemaShortGlobalAttributeExclusionsAcrossPolicies(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		for _, test := range []struct {
			name, body, needle string
			class              FailureClass
			cause              error
		}{
			{"below minimum", `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:short"><xs:minInclusive value="-32769"/></xs:restriction></xs:simpleType>`, `value="-32769"`, FailureInvalid, errInvalidBoundRestriction},
			{"above maximum", `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:short"><xs:maxInclusive value="32768"/></xs:restriction></xs:simpleType>`, `value="32768"`, FailureInvalid, errInvalidBoundRestriction},
			{"malformed bound", `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:short"><xs:minInclusive value="oops"/></xs:restriction></xs:simpleType>`, `value="oops"`, FailureInvalid, errInvalidBoundValue},
			{"local", `<xs:complexType name="T"><xs:attribute name="a" type="xs:short"/></xs:complexType>`, `type="xs:short"`, FailureUnsupported, errSchemaAttributeTypeUnsupported},
			{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded short attribute returned a schema or no error")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.needle) || diagnostic.Code() == "" || diagnostic.SpecRef() == "" || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with %v", diagnostic, test.class, test.needle, test.cause)
				}
				if test.class == FailureUnsupported && (diagnostic.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported)) {
					t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Keep reference-use and generator consumer boundaries together across policies.
func TestSchemaShortGlobalAttributeReferenceAndConsumerRemainUnsupported(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="global" type="xs:short" default="1"/><xs:complexType name="T"><xs:attribute ref="r:global"/></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("local short attribute reference returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="r:global"`) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
				t.Fatalf("local reference diagnostic = %s, want located unsupported use", diagnostic)
			}
			if !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="global"`)}) {
				t.Fatalf("related locations = %v, want global declaration", diagnostic.Related())
			}
			root = `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:r"><xs:attribute name="global" type="xs:short" fixed="1"/></xs:schema>`
			schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("GenerateGo = (%q, %v), want no output and unsupported attribute", output, err)
			}
			diagnostic = requireDiagnostic(t, err)
			attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "global"))
			if len(attribute) != 1 || diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != attribute[0].Loc() || !errors.Is(err, errCodegenUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want located unsupported attribute", diagnostic)
			}
		})
	}
}
