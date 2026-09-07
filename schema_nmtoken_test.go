package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the NMTOKEN identity and reference-shape matrix together.
func TestSchemaBuiltinNMTOKENReferencesPreserveIdentityAcrossPolicies(t *testing.T) {
	for _, syntaxVersion := range []XSDVersion{XSDVersion10, XSDVersion11} {
		for _, profile := range tokenPolicyProfiles() {
			t.Run(string(syntaxVersion)+"/"+profile.name, func(t *testing.T) {
				root := nmtokenReferenceSchemaRoot(syntaxVersion)
				other := nmtokenReferenceSchemaOtherDocument(syntaxVersion)
				schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
					"other.xsd": {id: "other.xsd", contents: other},
				}, profile.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}

				if got := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "NMTOKEN")); len(got) != 0 {
					t.Fatalf("xs:NMTOKEN component matches = %d, want no public built-in component", len(got))
				}
				wantComponents := []QName{
					mustTestQName(t, "urn:test", "direct"),
					mustTestQName(t, "urn:test", "anonymous"),
					mustTestQName(t, "urn:test", "NMTOKENAlias"),
					mustTestQName(t, "urn:test", "Forward"),
					mustTestQName(t, "urn:test", "Later"),
					mustTestQName(t, "urn:test", "ImportedAlias"),
					mustTestQName(t, "urn:test", "NMTOKENList"),
					mustTestQName(t, "urn:test", "NamedList"),
					mustTestQName(t, "urn:test", "NMTOKENUnion"),
					mustTestQName(t, "urn:other", "ImportedNMTOKEN"),
				}
				components := schema.Components()
				if len(components) != len(wantComponents) {
					t.Fatalf("component count = %d, want %d", len(components), len(wantComponents))
				}
				for index, want := range wantComponents {
					if components[index].Name() != want {
						t.Fatalf("component %d name = %q, want %q", index, components[index].Name(), want)
					}
					wantSource := SourceID("root.xsd")
					wantOrdinal := uint64(index + 1)
					if index == len(components)-1 {
						wantSource = "other.xsd"
						wantOrdinal = 1
					}
					if components[index].ID().Source() != wantSource || components[index].ID().Ordinal() != wantOrdinal {
						t.Fatalf("component %d ID = %s/%d, want %s/%d", index, components[index].ID().Source(), components[index].ID().Ordinal(), wantSource, wantOrdinal)
					}
				}
				repeated, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
					"other.xsd": {id: "other.xsd", contents: other},
				}, profile.policy)
				if err != nil {
					t.Fatalf("repeated discoverSchema: %v", err)
				}
				if !reflect.DeepEqual(components, repeated.Components()) {
					t.Fatal("repeated NMTOKEN builds changed component facts or order")
				}

				direct := nmtokenElementDefinition(t, schema, "direct")
				directReference, ok := direct.TypeReference()
				if !ok {
					t.Fatal("direct NMTOKEN element has no type reference")
				}
				assertNMTOKENBuiltinReference(t, directReference, mustSchemaTokenLoc(t, "root.xsd", root, 3, `type="xs:NMTOKEN"`))
				if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("direct NMTOKEN element type ID = %v/%t, want zero/false", typeID, hasTypeID)
				}

				alias := nmtokenDefinition(t, schema, "NMTOKENAlias")
				assertNMTOKENDefinition(t, alias)
				aliasBase, ok := alias.BaseReference()
				if !ok {
					t.Fatal("NMTOKEN alias has no base reference")
				}
				assertNMTOKENBuiltinReference(t, aliasBase, mustSchemaTokenLoc(t, "root.xsd", root, 12, `base="xs:NMTOKEN"`))
				assertStringEnumerationFacts(t, alias.StringEnumerationFacets(), profile.version, []string{" alias "}, []Loc{
					mustSchemaTokenLoc(t, "root.xsd", root, 13, `value`),
				})

				anonymousElement := nmtokenElementDefinition(t, schema, "anonymous")
				anonymousReference, ok := anonymousElement.TypeReference()
				if !ok || !anonymousReference.IsAnonymous() {
					t.Fatalf("anonymous NMTOKEN reference = %#v/%t, want anonymous", anonymousReference, ok)
				}
				if anonymousReference.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 5, "<xs:simpleType") || anonymousReference.VarietyLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 6, "<xs:restriction") {
					t.Fatalf("anonymous NMTOKEN reference locations = %s/%s, want inline type/restriction locations", anonymousReference.Loc(), anonymousReference.VarietyLoc())
				}
				if anonymousID, hasAnonymousID := anonymousReference.AnonymousID(); !hasAnonymousID || anonymousID.IsZero() || anonymousID.Source() != "root.xsd" {
					t.Fatalf("anonymous NMTOKEN ID = %v/%t, want root.xsd identity", anonymousID, hasAnonymousID)
				}
				anonymous, ok := anonymousReference.AnonymousType()
				if !ok {
					t.Fatal("anonymous NMTOKEN element has no type view")
				}
				assertNMTOKENDefinition(t, anonymous)
				anonymousBase, ok := anonymous.BaseReference()
				if !ok {
					t.Fatal("anonymous NMTOKEN type has no base reference")
				}
				assertNMTOKENBuiltinReference(t, anonymousBase, mustSchemaTokenLoc(t, "root.xsd", root, 6, `base="xs:NMTOKEN"`))

				forward := nmtokenDefinition(t, schema, "Forward")
				assertNMTOKENDefinition(t, forward)
				forwardBase, ok := forward.BaseReference()
				if !ok || !forwardBase.IsNamed() || forwardBase.Name() != mustTestQName(t, "urn:test", "Later") {
					t.Fatalf("forward NMTOKEN base = %#v/%t, want named Later", forwardBase, ok)
				}
				if forwardBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 17, `base="t:Later"`) {
					t.Fatalf("forward NMTOKEN base location = %s, want use-site", forwardBase.Loc())
				}
				if forwardID, hasID := forwardBase.ComponentID(); !hasID || forwardID != componentIDForName(t, schema, mustTestQName(t, "urn:test", "Later")) {
					t.Fatalf("forward NMTOKEN base ID = %v/%t, want Later identity", forwardID, hasID)
				}

				later := nmtokenDefinition(t, schema, "Later")
				assertNMTOKENDefinition(t, later)
				laterBase, ok := later.BaseReference()
				if !ok {
					t.Fatal("forward target has no base reference")
				}
				assertNMTOKENBuiltinReference(t, laterBase, mustSchemaTokenLoc(t, "root.xsd", root, 20, `base="xs:NMTOKEN"`))

				importedAlias := nmtokenDefinition(t, schema, "ImportedAlias")
				assertNMTOKENDefinition(t, importedAlias)
				importedBase, ok := importedAlias.BaseReference()
				if !ok || !importedBase.IsNamed() || importedBase.Name() != mustTestQName(t, "urn:other", "ImportedNMTOKEN") {
					t.Fatalf("imported NMTOKEN base = %#v/%t, want named imported NMTOKEN", importedBase, ok)
				}
				if importedBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 23, `base="o:ImportedNMTOKEN"`) {
					t.Fatalf("imported NMTOKEN base location = %s, want use-site", importedBase.Loc())
				}
				if importedID, hasID := importedBase.ComponentID(); !hasID || importedID != componentIDForName(t, schema, mustTestQName(t, "urn:other", "ImportedNMTOKEN")) {
					t.Fatalf("imported NMTOKEN base ID = %v/%t, want imported identity", importedID, hasID)
				}

				list := nmtokenDefinition(t, schema, "NMTOKENList")
				if list.Variety() != SimpleTypeVarietyList {
					t.Fatalf("NMTOKEN list variety = %q, want list", list.Variety())
				}
				listItem, ok := list.ItemType()
				if !ok {
					t.Fatal("NMTOKEN list has no item type")
				}
				assertNMTOKENBuiltinReference(t, listItem, mustSchemaTokenLoc(t, "root.xsd", root, 26, `itemType="xs:NMTOKEN"`))

				namedList := nmtokenDefinition(t, schema, "NamedList")
				namedItem, ok := namedList.ItemType()
				if !ok || !namedItem.IsNamed() || namedItem.Name() != mustTestQName(t, "urn:test", "Forward") {
					t.Fatalf("named NMTOKEN list item = %#v/%t, want named Forward", namedItem, ok)
				}
				if namedItem.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 29, `itemType="t:Forward"`) {
					t.Fatalf("named NMTOKEN list item location = %s, want use-site", namedItem.Loc())
				}
				if itemID, hasID := namedItem.ComponentID(); !hasID || itemID != componentIDForName(t, schema, mustTestQName(t, "urn:test", "Forward")) {
					t.Fatalf("named NMTOKEN list item ID = %v/%t, want Forward identity", itemID, hasID)
				}

				union := nmtokenDefinition(t, schema, "NMTOKENUnion")
				if union.Variety() != SimpleTypeVarietyUnion {
					t.Fatalf("NMTOKEN union variety = %q, want union", union.Variety())
				}
				members := union.MemberTypes()
				if len(members) != 3 {
					t.Fatalf("NMTOKEN union member count = %d, want 3", len(members))
				}
				memberLoc := mustSchemaTokenLoc(t, "root.xsd", root, 32, `memberTypes="`)
				assertNMTOKENBuiltinReference(t, members[0], memberLoc)
				if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Forward") || members[1].Loc() != memberLoc {
					t.Fatalf("NMTOKEN union named member = %#v, want ordered named Forward at memberTypes", members[1])
				}
				if !members[2].IsAnonymous() {
					t.Fatalf("NMTOKEN union inline member = %#v, want anonymous", members[2])
				}
				inlineMember, ok := members[2].AnonymousType()
				if !ok {
					t.Fatal("NMTOKEN union inline member has no type view")
				}
				assertNMTOKENDefinition(t, inlineMember)
				inlineBase, ok := inlineMember.BaseReference()
				if !ok {
					t.Fatal("NMTOKEN union inline member has no base reference")
				}
				assertNMTOKENBuiltinReference(t, inlineBase, mustSchemaTokenLoc(t, "root.xsd", root, 34, `base="xs:NMTOKEN"`))

				copiedMembers := union.MemberTypes()
				copiedMembers[0] = SimpleTypeReference{}
				if union.MemberTypes()[0].Kind() != SimpleTypeReferenceBuiltin || union.MemberTypes()[0].Name() != mustTestQName(t, testXSDNamespace, "NMTOKEN") {
					t.Fatal("mutating NMTOKEN union member copy changed the completed schema")
				}
			})
		}
	}
}

//nolint:gocognit // Keep NMTOKEN lexical facts, value-space narrowing, and immutability together.
func TestSchemaNMTOKENEnumerationUsesCollapsedValueSpaceAndImmutableFacts(t *testing.T) {
	for _, syntaxVersion := range []XSDVersion{XSDVersion10, XSDVersion11} {
		for _, profile := range tokenPolicyProfiles() {
			t.Run(string(syntaxVersion)+"/"+profile.name, func(t *testing.T) {
				root := nmtokenEnumerationSchemaRoot(syntaxVersion)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}

				base := nmtokenDefinition(t, schema, "Base")
				assertNMTOKENDefinition(t, base)
				assertStringWhiteSpaceFacet(t, base, "collapse", true, Loc{})
				assertStringEnumerationFacts(t, base.StringEnumerationFacets(), profile.version,
					[]string{"  first  ", "\tfirst\n", ":", "9", "-", "."}, []Loc{
						mustSchemaTokenLoc(t, "root.xsd", root, 4, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 5, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 6, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 7, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 8, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 9, `value`),
					})

				child := nmtokenDefinition(t, schema, "Child")
				assertNMTOKENDefinition(t, child)
				assertStringEnumerationFacts(t, child.StringEnumerationFacets(), profile.version,
					[]string{" first ", "\tfirst\n", ":", "-"}, []Loc{
						mustSchemaTokenLoc(t, "root.xsd", root, 14, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 15, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 16, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 17, `value`),
					})
				assertStringWhiteSpaceFacet(t, child, "collapse", true, mustSchemaTokenLoc(t, "root.xsd", root, 18, `value`))

				inherited := nmtokenDefinition(t, schema, "Inherited")
				assertNMTOKENDefinition(t, inherited)
				if got, want := inherited.StringEnumerationFacets().Values(), base.StringEnumerationFacets().Values(); !reflect.DeepEqual(got, want) {
					t.Fatalf("inherited NMTOKEN values = %#v, want %#v", got, want)
				}

				values := child.StringEnumerationFacets().Values()
				values[0] = "changed"
				declarations := child.StringEnumerationFacets().Declarations()
				declarations[1] = StringEnumerationFacet{}
				locations := child.StringEnumerationFacets().Locations()
				locations[0] = Loc{}
				again := child.StringEnumerationFacets()
				if got, want := again.Values(), []string{" first ", "\tfirst\n", ":", "-"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("NMTOKEN values changed through copies = %#v, want %#v", got, want)
				}
				if got := again.Locations()[0]; got.IsZero() {
					t.Fatal("NMTOKEN enumeration location changed through a copy")
				}
			})
		}
	}

	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "colon", value: ":", want: true},
		{name: "digit", value: "9", want: true},
		{name: "hyphen", value: "-", want: true},
		{name: "period", value: ".", want: true},
		{name: "unicode combining", value: "e\u0301", want: true},
		{name: "middle dot", value: "a\u00b7b", want: true},
		{name: "empty", value: "", want: false},
		{name: "internal space", value: "a b", want: false},
		{name: "slash", value: "/", want: false},
		{name: "control", value: "\t", want: false},
		{name: "invalid UTF-8", value: string([]byte{0xff}), want: false},
	} {
		t.Run("lexical/"+test.name, func(t *testing.T) {
			if got := validXMLNmtoken(test.value); got != test.want {
				t.Fatalf("validXMLNmtoken(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep invalid, narrowing, unsupported, and consumer boundaries together.
func TestSchemaNMTOKENDiagnosticsAndConsumerBoundaries(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			t.Run("invalid lexical after collapse", func(t *testing.T) {
				root := nmtokenDiagnosticSchemaRoot(profile.version, `
  <xs:simpleType name="item">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value=" bad value "/>
    </xs:restriction>
  </xs:simpleType>
`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted an invalid NMTOKEN enumeration")
				}
				assertNMTOKENNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidEnumerationRestrictionCode {
					t.Fatalf("diagnostic = %s/%q, want invalid enumeration restriction", diagnostic, diagnostic.Code())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `value`) {
					t.Fatalf("diagnostic location = %s, want enumeration value location", diagnostic.Loc())
				}
				if diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, "enumeration-valid-restriction") {
					t.Fatalf("diagnostic spec ref = %q, want versioned enumeration restriction", diagnostic.SpecRef())
				}
				if len(diagnostic.Related()) != 0 || !errors.Is(err, errInvalidEnumerationRestriction) || !errors.Is(err, errSchemaNMTOKENValueViolation) {
					t.Fatalf("invalid NMTOKEN diagnostic lost related/cause facts: %v", err)
				}
			})

			t.Run("narrowing retains base locations", func(t *testing.T) {
				root := nmtokenDiagnosticSchemaRoot(profile.version, `
  <xs:simpleType name="base">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value="allowed"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:enumeration value=" other "/>
    </xs:restriction>
  </xs:simpleType>
`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted an out-of-base NMTOKEN enumeration")
				}
				assertNMTOKENNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidEnumerationRestrictionCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 9, `value`) {
					t.Fatalf("narrowing diagnostic = %s/%q at %s, want invalid restriction at child value", diagnostic, diagnostic.Code(), diagnostic.Loc())
				}
				if diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, "enumeration-valid-restriction") {
					t.Fatalf("narrowing spec ref = %q, want versioned enumeration restriction", diagnostic.SpecRef())
				}
				if got, want := diagnostic.Related(), []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 4, `value`)}; !reflect.DeepEqual(got, want) {
					t.Fatalf("narrowing related locations = %v, want %v", got, want)
				}
				if !errors.Is(err, errInvalidEnumerationRestriction) {
					t.Fatalf("narrowing diagnostic lost invalid restriction cause: %v", err)
				}
			})

			for _, facet := range []struct {
				name  string
				value string
			}{
				{name: "pattern", value: ".*"},
				{name: "minLength", value: "1"},
			} {
				t.Run("unsupported "+facet.name, func(t *testing.T) {
					root := nmtokenDiagnosticSchemaRoot(profile.version, `
  <xs:simpleType name="item">
    <xs:restriction base="xs:NMTOKEN">
      <xs:`+facet.name+` value="`+facet.value+`"/>
    </xs:restriction>
  </xs:simpleType>
`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("discoverSchema silently accepted an unsupported NMTOKEN facet")
					}
					assertNMTOKENNoSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Feature() != FeatureDatatypeFacets {
						t.Fatalf("unsupported facet diagnostic = %s/%q/%q/%q, want datatype facet", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, "<xs:"+facet.name) {
						t.Fatalf("unsupported facet location = %s, want facet location", diagnostic.Loc())
					}
					if diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, "decimal") || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported facet diagnostic lost versioned reference or cause: %v", err)
					}
				})
			}

			for _, unsupportedType := range []string{"NMTOKENS", "Name"} {
				t.Run("unsupported built-in "+unsupportedType, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:element name="item" type="xs:` + unsupportedType + `"/>
</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("discoverSchema silently accepted an unsupported Name-family built-in")
					}
					assertNMTOKENNoSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
						t.Fatalf("unsupported built-in diagnostic = %s/%q/%q/%q, want schema syntax", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="xs:`+unsupportedType+`"`) || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(profile.version) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported built-in diagnostic facts are wrong: %v", err)
					}
				})
			}

			t.Run("unsupported assertion", func(t *testing.T) {
				root := nmtokenDiagnosticSchemaRoot(profile.version, `
  <xs:simpleType name="item">
    <xs:restriction base="xs:NMTOKEN">
      <xs:assertion test="true()"/>
    </xs:restriction>
  </xs:simpleType>
`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema silently accepted an unsupported NMTOKEN assertion")
				}
				assertNMTOKENNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Feature() != FeatureID("xsd.assertion") {
					t.Fatalf("assertion diagnostic = %s/%q/%q/%q, want assertion unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, "<xs:assertion") || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("assertion diagnostic location or classification is wrong: %v", err)
				}
			})

			t.Run("global consumer boundaries", func(t *testing.T) {
				root := nmtokenConsumerDirectRoot(profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}
				generated, err := GenerateGo(schema, "generated")
				if generated != nil || err == nil {
					t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no source", generated, err)
				}
				codegenDiagnostic := requireDiagnostic(t, err)
				if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Feature() != FeatureCodegen || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("GenerateGo NMTOKEN diagnostic = %s/%q/%q, want codegen unsupported", codegenDiagnostic, codegenDiagnostic.Class(), codegenDiagnostic.Feature())
				}

				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<item xmlns="urn:test">value</item>`)))
				if validationErr == nil {
					t.Fatal("ValidateInstance silently accepted xs:NMTOKEN")
				}
				validationDiagnostic := requireDiagnostic(t, validationErr)
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
					t.Fatalf("ValidateInstance NMTOKEN diagnostic = %s/%q/%q, want instance-validation unsupported", validationDiagnostic, validationDiagnostic.Class(), validationDiagnostic.Feature())
				}
			})

			t.Run("global attribute boundary", func(t *testing.T) {
				root := nmtokenConsumerAttributeRoot(profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema silently accepted global xs:NMTOKEN attribute use")
				}
				assertNMTOKENNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("global attribute NMTOKEN diagnostic = %s/%q/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="xs:NMTOKEN"`) || diagnostic.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("global attribute NMTOKEN diagnostic facts are wrong: %v", err)
				}
			})

			t.Run("local particle boundary", func(t *testing.T) {
				root := nmtokenConsumerLocalRoot(profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema silently accepted local xs:NMTOKEN scalar use")
				}
				assertNMTOKENNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("local NMTOKEN diagnostic = %s/%q/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `type="xs:NMTOKEN"`) || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(profile.version) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("local NMTOKEN diagnostic facts are wrong: %v", err)
				}
			})
		})
	}
}

func assertNMTOKENBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc) {
	assertStringLikeBuiltinReference(t, reference, "NMTOKEN", schemaSimpleTypeAtomicNMTOKEN, wantLoc)
}

func assertStringLikeBuiltinReference(t *testing.T, reference SimpleTypeReference, local string, atomicKind schemaSimpleTypeAtomicKind, wantLoc Loc) {
	t.Helper()
	wantName := mustTestQName(t, testXSDNamespace, local)
	if !reference.IsBuiltin() || reference.Kind() != SimpleTypeReferenceBuiltin || reference.Name() != wantName || reference.QName() != wantName {
		t.Fatalf("%s reference = %#v, want distinct built-in xs:%s", local, reference, local)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("%s reference facts = %s/%s/%q, want use-site atomic restriction at %s", local, reference.Loc(), reference.VarietyLoc(), reference.Variety(), wantLoc)
	}
	if reference.facts == nil || reference.facts.atomicKind != atomicKind {
		t.Fatalf("%s reference atomic facts = %#v, want private %s category", local, reference.facts, local)
	}
	facets, ok := reference.facts.facets.(schemaStringFacetVariant)
	if !ok || facets.whiteSpace == nil || facets.whiteSpace.Value() != "collapse" || !facets.whiteSpace.Fixed() || !facets.whiteSpace.Loc().IsZero() {
		t.Fatalf("%s reference whiteSpace facts = %#v/%t, want fixed unlocated collapse", local, facets, ok)
	}
	if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() || reference.facts.hasID {
		t.Fatalf("%s reference component ID = %v/%t, want zero/false", local, typeID, hasTypeID)
	}
}

func assertNMTOKENDefinition(t *testing.T, definition SimpleTypeDefinition) {
	t.Helper()
	if definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicNMTOKEN || !definition.IsString() {
		t.Fatalf("NMTOKEN definition facts = %#v/string:%t, want private NMTOKEN category and string accessors", definition.facts, definition.IsString())
	}
	whiteSpace, ok := definition.StringWhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "collapse" || !whiteSpace.Fixed() {
		t.Fatalf("NMTOKEN definition whiteSpace = (%q, fixed=%t)/%t, want fixed collapse", whiteSpace.Value(), whiteSpace.Fixed(), ok)
	}
}

func nmtokenElementDefinition(t *testing.T, schema Schema, local string) ElementDeclaration {
	t.Helper()
	components := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", local))
	if len(components) != 1 {
		t.Fatalf("element %q matches = %d, want one", local, len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("element %q has no declaration view", local)
	}
	return declaration
}

func nmtokenDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	return schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:test", local)
}

func assertNMTOKENNoSchema(t *testing.T, schema Schema) {
	t.Helper()
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema after an NMTOKEN error")
	}
}

func nmtokenReferenceSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" xmlns:o="urn:other" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:NMTOKEN"/>
  <xs:element name="anonymous">
    <xs:simpleType>
      <xs:restriction base="xs:NMTOKEN">
        <xs:enumeration value=" inline "/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
  <xs:simpleType name="NMTOKENAlias">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value=" alias "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Forward">
    <xs:restriction base="t:Later"/>
  </xs:simpleType>
  <xs:simpleType name="Later">
    <xs:restriction base="xs:NMTOKEN"/>
  </xs:simpleType>
  <xs:simpleType name="ImportedAlias">
    <xs:restriction base="o:ImportedNMTOKEN"/>
  </xs:simpleType>
  <xs:simpleType name="NMTOKENList">
    <xs:list itemType="xs:NMTOKEN"/>
  </xs:simpleType>
  <xs:simpleType name="NamedList">
    <xs:list itemType="t:Forward"/>
  </xs:simpleType>
  <xs:simpleType name="NMTOKENUnion">
    <xs:union memberTypes="xs:NMTOKEN t:Forward">
      <xs:simpleType>
        <xs:restriction base="xs:NMTOKEN">
          <xs:enumeration value=" union "/>
        </xs:restriction>
      </xs:simpleType>
    </xs:union>
  </xs:simpleType>
</xs:schema>`
}

func nmtokenReferenceSchemaOtherDocument(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(version) + `">
  <xs:simpleType name="ImportedNMTOKEN">
    <xs:restriction base="xs:NMTOKEN"/>
  </xs:simpleType>
</xs:schema>`
}

func nmtokenEnumerationSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="Base">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value="  first  "/>
      <xs:enumeration value="&#x9;first&#xA;"/>
      <xs:enumeration value=":"/>
      <xs:enumeration value="9"/>
      <xs:enumeration value="-"/>
      <xs:enumeration value="."/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Child">
    <xs:restriction base="t:Base">
      <xs:enumeration value=" first "/>
      <xs:enumeration value="&#x9;first&#xA;"/>
      <xs:enumeration value=":"/>
      <xs:enumeration value="-"/>
      <xs:whiteSpace value="collapse" fixed="false"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Inherited">
    <xs:restriction base="t:Base"/>
  </xs:simpleType>
</xs:schema>`
}

func nmtokenDiagnosticSchemaRoot(version XSDVersion, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">` + body + `</xs:schema>`
}

func nmtokenConsumerDirectRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="item" type="xs:NMTOKEN"/>
</xs:schema>`
}

func nmtokenConsumerAttributeRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:attribute name="item" type="xs:NMTOKEN"/>
</xs:schema>`
}

func nmtokenConsumerLocalRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:complexType name="Box">
    <xs:sequence>
      <xs:element name="item" type="xs:NMTOKEN"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="box" type="t:Box"/>
</xs:schema>`
}

func schemaSyntaxSpecRefForVersion(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#schema-document"
	}
	return "xsd11-structures#cSchemaDocument"
}
