package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func normalizedStringInlineAttributeGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:inline" xmlns:o="urn:other" targetNamespace="urn:inline" version="` + string(version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="builtin"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType></xs:attribute>
  <xs:attribute name="forward"><xs:simpleType><xs:restriction base="r:Forward"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType></xs:attribute>
  <xs:attribute name="included"><xs:simpleType><xs:restriction base="r:Included"/></xs:simpleType></xs:attribute>
  <xs:attribute name="imported"><xs:simpleType><xs:restriction base="o:Imported"/></xs:simpleType></xs:attribute>
  <xs:attribute name="chameleon"><xs:simpleType><xs:restriction base="r:Chameleon"/></xs:simpleType></xs:attribute>
  <xs:simpleType name="Forward"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value=" other "/></xs:restriction></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="collapse"/></xs:restriction></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
	}
}

//nolint:gocognit,funlen // One graph checks inline ownership, copied facets, and ordered public walks.
func TestNormalizedStringInlineGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := normalizedStringInlineAttributeGraph(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("inline graph: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) || !reflect.DeepEqual(first.Documents(), second.Documents()) {
				t.Fatalf("repeated inline graph changed: %v", err)
			}
			if len(first.Documents()) != 4 {
				t.Fatalf("documents = %d, want 4", len(first.Documents()))
			}
			want := []struct {
				local, lexical, namespace, source, whiteSpace, whiteSource, whiteMarker, enumSource, enumMarker string
				enumLine                                                                                        int
			}{
				{"builtin", "xs:normalizedString", testXSDNamespace, "", "replace", "", "", "root.xsd", `value="a&#xA;b"`, 5},
				{"forward", "r:Forward", "urn:inline", "root.xsd", "replace", "", "", "root.xsd", `value="a&#xA;b"`, 6},
				{"included", "r:Included", "urn:inline", "included.xsd", "collapse", "included.xsd", `value="collapse"`, "", "", 0},
				{"imported", "o:Imported", "urn:other", "other.xsd", "replace", "", "", "", "", 0},
				{"chameleon", "r:Chameleon", "urn:inline", "chameleon.xsd", "replace", "", "", "", "", 0},
			}
			components := first.Components()
			if len(components) != 9 {
				t.Fatalf("components = %d, want 9", len(components))
			}
			var previous SimpleTypeID
			for index, expected := range want {
				component := components[index]
				declaration, ok := component.AttributeDeclaration()
				if !ok || component.Name() != mustTestQName(t, "urn:inline", expected.local) || declaration.DeclaredType() != (QName{}) || declaration.Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:attribute name="`+expected.local+`"`, root, fixtures) {
					t.Fatalf("attribute %d = %#v", index, component)
				}
				reference, ok := declaration.TypeReference()
				inline, hasInline := declaration.InlineSimpleType()
				id, hasID := reference.AnonymousID()
				modelID, hasModelID := inline.NodeID()
				if !ok || !hasInline || !hasID || !hasModelID || id != modelID || id.IsZero() || !reference.IsAnonymous() || reference.QName() != (QName{}) || reference.Variety() != SimpleTypeVarietyAtomicRestriction || inline.Variety() != reference.Variety() || !inline.IsString() || inline.ID() != (ComponentID{}) {
					t.Fatalf("inline identity %s = %#v/%#v", expected.local, reference, inline)
				}
				if index > 0 && id.Ordinal() <= previous.Ordinal() {
					t.Fatalf("anonymous IDs out of order: %v then %v", previous, id)
				}
				previous = id
				wantTypeLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:simpleType><xs:restriction base="`+expected.lexical+`"`, root, fixtures)
				wantBaseLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `base="`+expected.lexical+`"`, root, fixtures)
				base, hasBase := inline.BaseReference()
				baseLocal := strings.SplitN(expected.lexical, ":", 2)[1]
				if reference.Loc() != wantTypeLoc || inline.Loc() != wantTypeLoc || reference.VarietyLoc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:restriction base="`+expected.lexical+`"`, root, fixtures) || !hasBase || base.Name() != mustTestQName(t, expected.namespace, baseLocal) || base.Loc() != wantBaseLoc {
					t.Fatalf("inline locations/base %s = %#v/%#v", expected.local, reference, base)
				}
				if expected.source == "" {
					if !base.IsBuiltin() {
						t.Fatalf("built-in base = %#v", base)
					}
				}
				if expected.source != "" {
					baseID, hasBaseID := base.ComponentID()
					if !hasBaseID || !base.IsNamed() || baseID.Source() != SourceID(expected.source) || baseID != componentIDForName(t, first, base.Name()) {
						t.Fatalf("named base %s = %#v", expected.local, base)
					}
				}
				white, ok := inline.StringWhiteSpaceFacet()
				wantWhiteLoc := Loc{}
				if expected.whiteMarker != "" {
					wantWhiteLoc = schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.whiteSource), expected.whiteMarker, root, fixtures)
				}
				if !ok || white.Value() != expected.whiteSpace || white.Loc() != wantWhiteLoc || white.Fixed() {
					t.Fatalf("whitespace %s = %#v/%t", expected.local, white, ok)
				}
				facets := inline.StringEnumerationFacets()
				if expected.enumMarker != "" {
					wantEnumLoc := mustSchemaTokenLoc(t, SourceID(expected.enumSource), root, expected.enumLine, expected.enumMarker)
					assertStringEnumerationFacts(t, facets, profile.version, []string{"a\nb"}, []Loc{wantEnumLoc})
					values := facets.Values()
					values[0] = "changed"
					locations := facets.Locations()
					locations[0] = Loc{}
					if again := inline.StringEnumerationFacets(); again.Values()[0] != "a\nb" || again.Locations()[0] != wantEnumLoc {
						t.Fatal("mutating copied inline facets changed Schema")
					}
				}
				if expected.enumMarker == "" && facets.HasEnumeration() {
					t.Fatalf("unexpected enumeration for %s: %v", expected.local, facets.Values())
				}
				if bounds, ok := inline.IntegerBounds(); ok {
					t.Fatalf("string bounds = %#v", bounds)
				}
				if found := first.FindKind(ComponentKindAttributeDeclaration, component.Name()); len(found) != 1 || found[0].ID() != component.ID() {
					t.Fatalf("FindKind(%s) = %v", component.Name(), found)
				}
			}
			var walked []ComponentID
			if err := first.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			for index, component := range components {
				if walked[index] != component.ID() {
					t.Fatalf("Walk order %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

//nolint:gocognit // The distinct exits preserve the resolver or facet cause and authored location.
func TestNormalizedStringInlineGlobalAttributeDiagnosticExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, primary, related, code, spec string
			class                                    FailureClass
			cause                                    error
		}{
			{"unresolved base", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Missing"/></xs:simpleType></xs:attribute>`, `base="t:Missing"`, "", diagnosticSchemaSimpleTypeUnresolvedCode, schemaSimpleTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseUnresolved},
			{"wrong kind base", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Wrong"/></xs:simpleType></xs:attribute><xs:element name="Wrong" type="xs:normalizedString"/>`, `base="t:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaSimpleTypeWrongKindCode, schemaSimpleTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseWrongKind},
			{"invalid whitespace", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType></xs:attribute>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"invalid narrowed enumeration", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Base"><xs:enumeration value="other"/></xs:restriction></xs:simpleType></xs:attribute><xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="allowed"/></xs:restriction></xs:simpleType>`, `value="other"`, `value="allowed"`, InvalidEnumerationRestrictionCode, versionedEnumerationSpecRef(profile.version, "enumeration-valid-restriction"), FailureInvalid, errInvalidEnumerationRestriction},
			{"unsupported length", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType></xs:attribute>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
			{"unsupported pattern", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:pattern value=".*"/></xs:restriction></xs:simpleType></xs:attribute>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("inline failure published a schema")
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.primary, root, nil)
				var related []Loc
				if test.related != "" {
					related = []Loc{schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.related, root, nil)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v related %v, want %s at %s related %v spec %s cause %v", err, d.Related(), test.code, wantLoc, related, test.spec, test.cause)
				}
			})
		}
	}
}

//nolint:gocognit // The inline base remains a lexical QName with graph visibility and cycle checks.
func TestNormalizedStringInlineGlobalAttributeGraphFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, relation := range []string{"unimported", "indirect-import"} {
			t.Run(profile.name+"/"+relation, func(t *testing.T) {
				root, fixtures := typeVisibilityTestHiddenGraph(t, string(profile.version), relation, false, true, false, "")
				root = strings.Replace(root, `<xs:element name="item" type="f:Hidden"/>`, `<xs:attribute name="item"><xs:simpleType><xs:restriction base="f:Hidden"/></xs:simpleType></xs:attribute>`, 1)
				foreign := fixtures["foreign.xsd"]
				foreign.contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:simpleType name="Hidden"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`
				fixtures["foreign.xsd"] = foreign
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("invisible inline base published a schema")
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `base="f:Hidden"`, root, fixtures)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleTypeUnresolvedCode || d.Loc() != wantLoc || d.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
					t.Fatalf("hidden inline base = %v related %v", err, d.Related())
				}
			})
		}
		t.Run(profile.name+"/cycle", func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:attribute name="a"><xs:simpleType><xs:restriction base="t:A"/></xs:simpleType></xs:attribute>
  <xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType>
  <xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>
  <xs:simpleType name="Seed"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("cyclic inline base published a schema")
			}
			d := requireDiagnostic(t, err)
			wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:simpleType><xs:restriction base="t:A"`, root, nil)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeCycleCode || d.Loc() != wantLoc || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("inline cycle = %v related %v", err, d.Related())
			}
			for _, marker := range []string{`<xs:attribute name="a"`, `base="t:B"`, `base="t:A"`} {
				loc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", marker, root, nil)
				if !schemaLocationListContains(d.Related(), loc) {
					t.Fatalf("cycle related = %v, missing %s", d.Related(), loc)
				}
			}
		})
	}
}

func TestNormalizedStringInlineGlobalAttributeAuthoredReplace(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="replace"/><xs:enumeration value=" a&#xA;b "/></xs:restriction></xs:simpleType></xs:attribute>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("authored replace: %v", err)
			}
			declaration, ok := schema.Components()[0].AttributeDeclaration()
			inline, hasInline := declaration.InlineSimpleType()
			white, hasWhite := inline.StringWhiteSpaceFacet()
			wantWhite := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="replace"`, root, nil)
			if !ok || !hasInline || !hasWhite || white.Value() != "replace" || white.Loc() != wantWhite || white.Fixed() {
				t.Fatalf("authored whitespace = %#v", white)
			}
			assertStringEnumerationFacts(t, inline.StringEnumerationFacets(), profile.version, []string{" a\nb "}, []Loc{schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value=" a&#xA;b "`, root, nil)})
		})
	}
}

//nolint:gocognit // Each consumer and the local reference retain their own boundary.
func TestNormalizedStringInlineGlobalAttributeConsumersStayUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>
  <xs:element name="root" type="xs:integer"/>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("inline global schema: %v", err)
			}
			instance := `<t:root xmlns:t="urn:test" a="text">1</t:root>`
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted an attribute on the instance")
			}
			validation := requireDiagnostic(t, validationErr)
			wantInstanceLoc := mustTestLoc(t, "instance.xml", 1, strings.Index(instance, `a="text"`)+1)
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantInstanceLoc || !errors.Is(validationErr, errInstanceAttributes) {
				t.Fatalf("validation diagnostic = %v", validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo returned %q, %v", output, generationErr)
			}
			generation := requireDiagnostic(t, generationErr)
			attributeLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:attribute name="a"`, root, nil)
			if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != attributeLoc || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation diagnostic = %v", generationErr)
			}
			refRoot := normalizedStringRoot(`
  <xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>
  <xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>
`)
			refSchema, refErr := discoverTestSchemaWithPolicy(t, refRoot, nil, profile.policy)
			if refErr == nil || refSchema.storage != nil {
				t.Fatal("local ref to inline normalizedString returned a schema")
			}
			refDiagnostic := requireDiagnostic(t, refErr)
			refLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `ref="t:a"`, refRoot, nil)
			ownerLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:attribute name="a"`, refRoot, nil)
			if refDiagnostic.Class() != FailureUnsupported || refDiagnostic.Code() != UnsupportedSchemaSyntaxCode || refDiagnostic.Loc() != refLoc || refDiagnostic.SpecRef() != schemaAttributeUseSpecRef(profile.version) || !reflect.DeepEqual(refDiagnostic.Related(), []Loc{ownerLoc}) || !errors.Is(refErr, errSchemaAttributeReferenceUnsupported) {
				t.Fatalf("local ref diagnostic = %v related %v", refErr, refDiagnostic.Related())
			}
		})
	}
}
