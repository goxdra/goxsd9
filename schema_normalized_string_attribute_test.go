package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func normalizedStringAttributeGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:normalizedString"/>
  <xs:attribute name="forward" type="r:Forward"/>
  <xs:attribute name="imported" type="o:Imported"/>
  <xs:attribute name="chameleon" type="r:Chameleon"/>
  <xs:attribute name="narrowed" type="r:Narrowed"/>
  <xs:attribute name="inherited" type="r:Inherited"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Narrowed"/></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"/></xs:simpleType><xs:attribute name="included" type="r:Included"/></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root"><xs:simpleType name="Chameleon"><xs:restriction base="xs:normalizedString"/></xs:simpleType><xs:attribute name="chameleonDirect" type="xs:normalizedString"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"/></xs:simpleType><xs:attribute name="importedDirect" type="xs:normalizedString"/></xs:schema>`},
	}
}

//nolint:gocognit,funlen // One graph proves identity, copied facets, locations, and stable discovery order.
func TestNormalizedStringGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range append([]tokenPolicyProfile{{name: "Compatibility 1.0", policy: Compatibility, version: XSDVersion10}}, tokenPolicyProfiles()...) {
		t.Run(profile.name, func(t *testing.T) {
			facetVersion := profile.version
			if profile.policy == Compatibility {
				facetVersion = XSDVersion11
			}
			root, fixtures := normalizedStringAttributeGraph(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) || len(first.Documents()) != 4 {
				t.Fatalf("repeated graph facts/discovery changed: %v", err)
			}
			want := []struct{ local, namespace, lexical, source, targetSource string }{
				{"direct", "urn:root", "xs:normalizedString", "root.xsd", ""},
				{"forward", "urn:root", "r:Forward", "root.xsd", "root.xsd"},
				{"imported", "urn:root", "o:Imported", "root.xsd", "other.xsd"},
				{"chameleon", "urn:root", "r:Chameleon", "root.xsd", "chameleon.xsd"},
				{"narrowed", "urn:root", "r:Narrowed", "root.xsd", "root.xsd"},
				{"inherited", "urn:root", "r:Inherited", "root.xsd", "root.xsd"},
				{"included", "urn:root", "r:Included", "included.xsd", "included.xsd"},
				{"chameleonDirect", "urn:root", "xs:normalizedString", "chameleon.xsd", ""},
				{"importedDirect", "urn:other", "xs:normalizedString", "other.xsd", ""},
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
			var walked []ComponentID
			if err := first.Walk(func(component Component) error {
				if component.Kind() == ComponentKindAttributeDeclaration {
					walked = append(walked, component.ID())
				}
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			for index, expected := range want {
				component := attributes[index]
				if walked[index] != component.ID() || component.Name() != mustTestQName(t, expected.namespace, expected.local) || component.ID().Source() != SourceID(expected.source) {
					t.Fatalf("attribute %d = %q/%v", index, component.Name(), component.ID())
				}
				found := first.FindKind(ComponentKindAttributeDeclaration, component.Name())
				if len(found) != 1 || found[0].ID() != component.ID() {
					t.Fatalf("FindKind(%s) = %v", component.Name(), found)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.Loc() != schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `<xs:attribute name="`+expected.local+`"`, root, fixtures) {
					t.Fatalf("attribute %s location/view = %s/%t", expected.local, declaration.Loc(), ok)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("attribute %s reference = %#v/%t", expected.local, reference, ok)
				}
				wantQName := mustTestQName(t, testXSDNamespace, "normalizedString")
				if expected.targetSource != "" {
					wantQName = mustTestQName(t, expected.namespace, expected.lexical[2:])
					if expected.lexical[0] == 'o' {
						wantQName = mustTestQName(t, "urn:other", expected.lexical[2:])
					}
				}
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `type="`+expected.lexical+`"`, root, fixtures)
				if declaration.DeclaredType() != wantQName || reference.QName() != wantQName || reference.Loc() != wantLoc {
					t.Fatalf("attribute %s type = %q/%q at %s", expected.local, declaration.DeclaredType(), reference.QName(), reference.Loc())
				}
				id, hasID := declaration.TypeID()
				if expected.targetSource == "" {
					if !reference.IsBuiltin() || hasID || !id.IsZero() || len(reference.StringEnumerationFacets().Values()) != 0 {
						t.Fatalf("built-in %s identity/enumeration = %#v/%v", expected.local, reference, id)
					}
				}
				if expected.targetSource != "" {
					if !reference.IsNamed() || !hasID || id.Source() != SourceID(expected.targetSource) || id != componentIDForName(t, first, wantQName) {
						t.Fatalf("named %s identity = %#v/%v", expected.local, reference, id)
					}
				}
				white, ok := reference.StringWhiteSpaceFacet()
				if !ok || white.Value() != "replace" || white.Fixed() || !white.Loc().IsZero() {
					t.Fatalf("attribute %s whitespace = %#v/%t", expected.local, white, ok)
				}
				if bounds, present := reference.IntegerBounds(); present {
					t.Fatalf("string attribute %s has numeric bounds: %#v", expected.local, bounds)
				}
				if expected.local == "narrowed" || expected.local == "inherited" {
					facets := reference.StringEnumerationFacets()
					assertStringEnumerationFacts(t, facets, facetVersion, []string{"a\nb", " a  b "}, []Loc{
						schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="a&#xA;b"`, root, fixtures),
						schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value=" a  b "`, root, fixtures),
					})
					values := facets.Values()
					values[0] = "changed"
					locs := facets.Locations()
					locs[0] = Loc{}
					again := reference.StringEnumerationFacets()
					if again.Values()[0] != "a\nb" || again.Locations()[0].IsZero() {
						t.Fatal("copied facets mutated reference")
					}
				}
			}
		})
	}
}

//nolint:gocognit // Classify every exit at the attribute type boundary across policies.
func TestNormalizedStringGlobalAttributeIncludedExitDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker, related, code, spec string
			class                                   FailureClass
			cause                                   error
		}{
			{"unresolved", `<xs:attribute name="a" type="t:Missing"/>`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"wrong kind", `<xs:element name="T" type="xs:normalizedString"/><xs:attribute name="a" type="t:T"/>`, `type="t:T"`, `<xs:element name="T"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
			{"invalid enum", `<xs:attribute name="a" type="t:Child"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a  b"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:T"><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>`, `value="a b"`, `value="a  b"`, InvalidEnumerationRestrictionCode, versionedEnumerationSpecRef(profile.version, "enumeration-valid-restriction"), FailureInvalid, errInvalidEnumerationRestriction},
			{"invalid whitespace", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"unsupported length", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
			{"unsupported pattern", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("failure published a schema")
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.marker, root, nil)
				var related []Loc
				if test.related != "" {
					related = []Loc{schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.related, root, nil)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || !errors.Is(err, test.cause) || !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("diagnostic = %v (related %v), want %s at %s related %v, spec %s", err, d.Related(), test.code, wantLoc, related, test.spec)
				}
			})
		}
	}
}

//nolint:gocognit // Each excluded shape has its own use location and cause.
func TestNormalizedStringGlobalAttributeExcludedShapesAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, primary, related, spec string
			causes                             []error
		}{
			{"direct default", `<xs:attribute name="a" type="xs:normalizedString" default="a"/>`, `default="a"`, "", schemaAttributeValueConstraintSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeValueConstraintUnsupported}},
			{"direct fixed", `<xs:attribute name="a" type="xs:normalizedString" fixed="a"/>`, `fixed="a"`, "", schemaAttributeValueConstraintSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeValueConstraintUnsupported}},
			{"named default", `<xs:attribute name="a" type="t:T" default="a"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `default="a"`, "", schemaAttributeValueConstraintSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeValueConstraintUnsupported}},
			{"named fixed", `<xs:attribute name="a" type="t:T" fixed="a"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `fixed="a"`, "", schemaAttributeValueConstraintSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeValueConstraintUnsupported}},
			{"global inline direct", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported}},
			{"global inline named", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:T"/></xs:simpleType></xs:attribute><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:simpleType`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported}},
			{"global inline default", `<xs:attribute name="a" default="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `default="a"`, "", schemaAttributeValueConstraintSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeValueConstraintUnsupported}},
			{"global inline fixed", `<xs:attribute name="a" fixed="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `fixed="a"`, "", schemaAttributeValueConstraintSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeValueConstraintUnsupported}},
			{"global inline list member", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `itemType="xs:normalizedString"`, `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported}},
			{"global inline union member", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `memberTypes="xs:normalizedString"`, `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported}},
			{"named list", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:list itemType="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported}},
			{"named union", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:union memberTypes="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported}},
			{"local direct", `<xs:complexType name="Box"><xs:attribute name="a" type="xs:normalizedString"/></xs:complexType>`, `type="xs:normalizedString"`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported, errSchemaAttributeUseUnsupported}},
			{"local named", `<xs:complexType name="Box"><xs:attribute name="a" type="t:T"/></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported, errSchemaAttributeUseUnsupported}},
			{"local inline", `<xs:complexType name="Box"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute></xs:complexType>`, `<xs:simpleType`, "", schemaAttributeTypeSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeTypeUnsupported, errSchemaAttributeUseUnsupported}},
			{"local ref direct", `<xs:attribute name="a" type="xs:normalizedString"/><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeReferenceUnsupported}},
			{"local ref named", `<xs:attribute name="a" type="t:T"/><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), []error{ErrUnsupported, errSchemaAttributeReferenceUnsupported}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded attribute shape published a schema")
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.primary, root, nil)
				var related []Loc
				if test.related != "" {
					related = []Loc{schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.related, root, nil)}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != wantLoc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("diagnostic = %v related %v; want unsupported at %s related %v spec %s", err, d.Related(), wantLoc, related, test.spec)
				}
				for _, cause := range test.causes {
					if !errors.Is(err, cause) {
						t.Fatalf("diagnostic %v lost cause %v", err, cause)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Graph visibility and cycle exits require exact located causes.
func TestNormalizedStringGlobalAttributeGraphFailureBoundaries(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, relation := range []string{"unimported", "indirect-import"} {
			t.Run(profile.name+"/"+relation, func(t *testing.T) {
				root, fixtures := typeVisibilityTestHiddenGraph(t, string(profile.version), relation, false, true, false, "")
				root = replaceNormalizedStringVisibilityElement(root)
				foreign := fixtures["foreign.xsd"]
				foreign.contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:simpleType name="Hidden"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`
				fixtures["foreign.xsd"] = foreign
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("hidden attribute target returned a schema")
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `type="f:Hidden"`, root, fixtures)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != wantLoc || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
					t.Fatalf("hidden target diagnostic = %v related %v", err, d.Related())
				}
			})
		}
		t.Run(profile.name+"/cycle", func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:attribute name="a" type="t:A"/>
  <xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType>
  <xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>
  <xs:simpleType name="Seed"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("cyclic attribute type returned a schema")
			}
			d := requireDiagnostic(t, err)
			wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `type="t:A"`, root, nil)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeCycleCode || d.Loc() != wantLoc || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) || len(d.Related()) == 0 {
				t.Fatalf("cycle diagnostic = %v related %v", err, d.Related())
			}
			for _, marker := range []string{`<xs:attribute name="a"`, `base="t:B"`, `base="t:A"`} {
				loc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", marker, root, nil)
				if !schemaLocationListContains(d.Related(), loc) {
					t.Fatalf("cycle related = %v, want %s", d.Related(), loc)
				}
			}
		})
	}
}

func replaceNormalizedStringVisibilityElement(root string) string {
	return strings.Replace(root, `<xs:element name="item" type="f:Hidden"/>`, `<xs:attribute name="item" type="f:Hidden"/>`, 1)
}

//nolint:gocognit // Exercise both supported reference shapes at the generator boundary.
func TestNormalizedStringGlobalAttributeGenerationStaysUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct{ name, body string }{
			{"direct", `<xs:attribute name="a" type="xs:normalizedString"/>`},
			{"named", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("query schema: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = (%q, %v), want unsupported and nil output", output, err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:attribute name="a"`, root, nil)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != wantLoc || d.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %v related %v", err, d.Related())
				}
			})
		}
	}
}

func TestNormalizedStringNamedAttributeRetainsAuthoredCollapse(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:attribute name="a" type="t:T"/>
  <xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "a"))
			if len(attribute) != 1 {
				t.Fatalf("attribute count = %d", len(attribute))
			}
			declaration, _ := attribute[0].AttributeDeclaration()
			reference, _ := declaration.TypeReference()
			white, ok := reference.StringWhiteSpaceFacet()
			wantWhiteLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="collapse"`, root, nil)
			if !ok || white.Value() != "collapse" || white.Loc() != wantWhiteLoc {
				t.Fatalf("authored whiteSpace = %#v/%t; want collapse at %s", white, ok, wantWhiteLoc)
			}
			facets := reference.StringEnumerationFacets()
			if got := facets.Values(); len(got) != 1 || got[0] != "a b" || facets.Locations()[0] != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="a b"`, root, nil) {
				t.Fatalf("authored enumeration = %v at %v", got, facets.Locations())
			}
		})
	}
}

// Ambiguous named simple-type candidates are intercepted by duplicate-declaration
// checks in ordinary discovery; the attribute reframe still owns this resolver exit.
func TestNormalizedStringAttributeAmbiguousResolverExit(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			primary := mustTestLoc(t, "root.xsd", 2, 24)
			related := []Loc{mustTestLoc(t, "first.xsd", 2, 3), mustTestLoc(t, "second.xsd", 3, 3)}
			input := &schemaAttributeInput{declaredType: mustTestQName(t, "urn:test", "T"), typeLoc: primary}
			resolverErr := newSchemaSimpleTypeDiagnostic(
				diagnosticSchemaSimpleTypeAmbiguousCode,
				primary,
				"ambiguous simple type",
				related,
				profile.version,
				errSchemaSimpleTypeBaseAmbiguous,
			)
			err := reframeSchemaAttributeReferenceError(input, resolverErr, profile.version)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeAmbiguousCode || d.Loc() != primary || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, errSchemaAttributeTypeAmbiguous) || !errors.Is(err, errSchemaSimpleTypeBaseAmbiguous) {
				t.Fatalf("ambiguous attribute reframe = %v related %v", err, d.Related())
			}
		})
	}
}

//nolint:gocognit // Local value constraints are excluded before type/use mapping in every shape.
func TestNormalizedStringLocalAttributeValueConstraintsStayExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, constraint := range []string{"default", "fixed"} {
			for _, shape := range []struct{ name, local, rest string }{
				{"direct", `<xs:attribute name="a" type="xs:normalizedString" ` + constraint + `="a"/>`, ""},
				{"named", `<xs:attribute name="a" type="t:T" ` + constraint + `="a"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`},
				{"inline", `<xs:attribute name="a" ` + constraint + `="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, ""},
				{"ref", `<xs:attribute ref="t:a" ` + constraint + `="a"/>`, `<xs:attribute name="a" type="xs:normalizedString"/>`},
			} {
				t.Run(profile.name+"/"+constraint+"/"+shape.name, func(t *testing.T) {
					root := normalizedStringRoot("\n  <xs:complexType name=\"Box\">" + shape.local + "</xs:complexType>" + shape.rest + "\n")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("local attribute value constraint published a schema")
					}
					d := requireDiagnostic(t, err)
					wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", constraint+`="a"`, root, nil)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != wantLoc || d.SpecRef() != schemaAttributeUseSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
						t.Fatalf("local value diagnostic = %v related %v; want use at %s", err, d.Related(), wantLoc)
					}
				})
			}
		}
	}
}
