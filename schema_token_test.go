package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the token identity and reference-shape matrix together.
func TestSchemaBuiltinTokenReferencesPreserveIdentityAcrossPolicies(t *testing.T) {
	for _, syntaxVersion := range []XSDVersion{XSDVersion10, XSDVersion11} {
		for _, profile := range tokenPolicyProfiles() {
			t.Run(string(syntaxVersion)+"/"+profile.name, func(t *testing.T) {
				root := tokenReferenceSchemaRoot(syntaxVersion)
				other := tokenReferenceSchemaOtherDocument(syntaxVersion)
				schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
					"other.xsd": {id: "other.xsd", contents: other},
				}, profile.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}

				if got := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "token")); len(got) != 0 {
					t.Fatalf("xs:token component matches = %d, want no public built-in component", len(got))
				}

				direct := tokenElementDefinition(t, schema, "direct")
				directReference, ok := direct.TypeReference()
				if !ok {
					t.Fatal("direct token element has no type reference")
				}
				assertTokenBuiltinReference(t, directReference, mustSchemaTokenLoc(t, "root.xsd", root, 3, `type="xs:token"`))
				if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("direct token element type ID = %v/%t, want zero/false", typeID, hasTypeID)
				}

				attributeComponent := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:test", "directAttribute"))
				if len(attributeComponent) != 1 {
					t.Fatalf("direct token attribute matches = %d, want one", len(attributeComponent))
				}
				attribute, ok := attributeComponent[0].Attribute()
				if !ok {
					t.Fatal("direct token attribute has no declaration view")
				}
				attributeReference, ok := attribute.TypeReference()
				if !ok {
					t.Fatal("direct token attribute has no type reference")
				}
				assertTokenBuiltinReference(t, attributeReference, mustSchemaTokenLoc(t, "root.xsd", root, 12, `type="xs:token"`))

				named := tokenDefinition(t, schema, "TokenAlias")
				if !named.IsString() || named.facts == nil || named.facts.atomicKind != schemaSimpleTypeAtomicToken {
					t.Fatalf("named token facts = string:%t/%v, want string token facts", named.IsString(), named.facts)
				}
				base, ok := named.BaseReference()
				if !ok {
					t.Fatal("named token has no base reference")
				}
				assertTokenBuiltinReference(t, base, mustSchemaTokenLoc(t, "root.xsd", root, 14, `base="xs:token"`))
				assertStringEnumerationFacts(t, named.StringEnumerationFacets(), profile.version, []string{" alias "}, []Loc{
					mustSchemaTokenLoc(t, "root.xsd", root, 15, `value`),
				})

				anonymousElement := tokenElementDefinition(t, schema, "anonymous")
				anonymousReference, ok := anonymousElement.TypeReference()
				if !ok || !anonymousReference.IsAnonymous() {
					t.Fatalf("anonymous token element reference = %#v/%t, want anonymous", anonymousReference, ok)
				}
				if anonymousReference.Loc().IsZero() || anonymousReference.VarietyLoc().IsZero() {
					t.Fatalf("anonymous token reference locations = %s/%s, want use-site locations", anonymousReference.Loc(), anonymousReference.VarietyLoc())
				}
				anonymousType, ok := anonymousReference.AnonymousType()
				if !ok {
					t.Fatal("anonymous token element has no inline type view")
				}
				assertTokenDefinition(t, anonymousType)
				anonymousBase, ok := anonymousType.BaseReference()
				if !ok {
					t.Fatal("anonymous token type has no base reference")
				}
				assertTokenBuiltinReference(t, anonymousBase, mustSchemaTokenLoc(t, "root.xsd", root, 7, `base="xs:token"`))

				forward := tokenDefinition(t, schema, "Forward")
				forwardBase, ok := forward.BaseReference()
				if !ok || !forwardBase.IsNamed() || forwardBase.Name() != mustTestQName(t, "urn:test", "Later") {
					t.Fatalf("forward token base = %#v/%t, want named Later", forwardBase, ok)
				}
				if forwardBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 19, `base="t:Later"`) {
					t.Fatalf("forward token base location = %s, want use-site", forwardBase.Loc())
				}
				forwardID, ok := forwardBase.ComponentID()
				if !ok || forwardID != componentIDForName(t, schema, mustTestQName(t, "urn:test", "Later")) {
					t.Fatalf("forward token base ID = %v/%t, want Later identity", forwardID, ok)
				}
				assertTokenDefinition(t, forward)

				later := tokenDefinition(t, schema, "Later")
				laterBase, ok := later.BaseReference()
				if !ok {
					t.Fatal("forward target has no base reference")
				}
				assertTokenBuiltinReference(t, laterBase, mustSchemaTokenLoc(t, "root.xsd", root, 22, `base="xs:token"`))

				importedAlias := tokenDefinition(t, schema, "ImportedAlias")
				importedBase, ok := importedAlias.BaseReference()
				if !ok || !importedBase.IsNamed() || importedBase.Name() != mustTestQName(t, "urn:other", "ImportedToken") {
					t.Fatalf("imported token base = %#v/%t, want named imported token", importedBase, ok)
				}
				if importedBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 25, `base="o:ImportedToken"`) {
					t.Fatalf("imported token base location = %s, want use-site", importedBase.Loc())
				}
				if importedID, hasID := importedBase.ComponentID(); !hasID || importedID != componentIDForName(t, schema, mustTestQName(t, "urn:other", "ImportedToken")) {
					t.Fatalf("imported token base ID = %v/%t, want imported identity", importedID, hasID)
				}
				assertTokenDefinition(t, importedAlias)

				list := tokenDefinition(t, schema, "TokenList")
				if list.Variety() != SimpleTypeVarietyList {
					t.Fatalf("token list variety = %q, want list", list.Variety())
				}
				listItem, ok := list.ItemType()
				if !ok {
					t.Fatal("token list has no item type")
				}
				assertTokenBuiltinReference(t, listItem, mustSchemaTokenLoc(t, "root.xsd", root, 28, `itemType="xs:token"`))

				namedList := tokenDefinition(t, schema, "NamedList")
				namedItem, ok := namedList.ItemType()
				if !ok || !namedItem.IsNamed() || namedItem.Name() != mustTestQName(t, "urn:test", "Forward") {
					t.Fatalf("named token list item = %#v/%t, want named Forward", namedItem, ok)
				}
				if namedItem.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 31, `itemType="t:Forward"`) {
					t.Fatalf("named token list item location = %s, want use-site", namedItem.Loc())
				}
				if itemID, hasID := namedItem.ComponentID(); !hasID || itemID != componentIDForName(t, schema, mustTestQName(t, "urn:test", "Forward")) {
					t.Fatalf("named token list item ID = %v/%t, want Forward identity", itemID, hasID)
				}

				union := tokenDefinition(t, schema, "TokenUnion")
				if union.Variety() != SimpleTypeVarietyUnion {
					t.Fatalf("token union variety = %q, want union", union.Variety())
				}
				members := union.MemberTypes()
				if len(members) != 3 {
					t.Fatalf("token union member count = %d, want 3", len(members))
				}
				assertTokenBuiltinReference(t, members[0], mustSchemaTokenLoc(t, "root.xsd", root, 34, `memberTypes="`))
				if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Forward") {
					t.Fatalf("token union named member = %#v, want named Forward", members[1])
				}
				if members[1].Loc() != members[0].Loc() {
					t.Fatalf("token union member locations = %s/%s, want shared memberTypes use-site", members[0].Loc(), members[1].Loc())
				}
				if !members[2].IsAnonymous() {
					t.Fatalf("token union inline member = %#v, want anonymous", members[2])
				}
				inlineMember, ok := members[2].AnonymousType()
				if !ok {
					t.Fatal("token union inline member has no type view")
				}
				assertTokenDefinition(t, inlineMember)
				inlineBase, ok := inlineMember.BaseReference()
				if !ok {
					t.Fatal("token union inline member has no base reference")
				}
				assertTokenBuiltinReference(t, inlineBase, mustSchemaTokenLoc(t, "root.xsd", root, 36, `base="xs:token"`))

				copiedMembers := union.MemberTypes()
				if len(copiedMembers) == 0 {
					t.Fatal("union member copy is empty")
				}
				copiedMembers[0] = SimpleTypeReference{}
				if union.MemberTypes()[0].Kind() != SimpleTypeReferenceBuiltin {
					t.Fatal("mutating union member copy changed the completed schema")
				}
			})
		}
	}
}

//nolint:gocognit // Keep lexical preservation, value-space narrowing, and copies together.
func TestSchemaTokenEnumerationUsesCollapsedValueSpaceAndImmutableLexicalFacts(t *testing.T) {
	for _, syntaxVersion := range []XSDVersion{XSDVersion10, XSDVersion11} {
		for _, profile := range tokenPolicyProfiles() {
			t.Run(string(syntaxVersion)+"/"+profile.name, func(t *testing.T) {
				root := tokenEnumerationSchemaRoot(syntaxVersion)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}

				base := tokenDefinition(t, schema, "Base")
				assertTokenDefinition(t, base)
				assertStringWhiteSpaceFacet(t, base, "collapse", true, Loc{})
				assertStringEnumerationFacts(t, base.StringEnumerationFacets(), profile.version,
					[]string{"  first  ", "", "first", "   "}, []Loc{
						mustSchemaTokenLoc(t, "root.xsd", root, 4, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 5, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 6, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 7, `value`),
					})

				child := tokenDefinition(t, schema, "Child")
				assertTokenDefinition(t, child)
				assertStringEnumerationFacts(t, child.StringEnumerationFacets(), profile.version,
					[]string{"first", "   ", " first "}, []Loc{
						mustSchemaTokenLoc(t, "root.xsd", root, 12, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 13, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 14, `value`),
					})
				assertStringWhiteSpaceFacet(t, child, "collapse", true, mustSchemaTokenLoc(t, "root.xsd", root, 15, `value`))

				inherited := tokenDefinition(t, schema, "Inherited")
				assertTokenDefinition(t, inherited)
				assertStringEnumerationFacts(t, inherited.StringEnumerationFacets(), profile.version,
					[]string{"  first  ", "", "first", "   "}, []Loc{
						mustSchemaTokenLoc(t, "root.xsd", root, 4, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 5, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 6, `value`),
						mustSchemaTokenLoc(t, "root.xsd", root, 7, `value`),
					})

				facets := child.StringEnumerationFacets()
				values := facets.Values()
				values[0] = "changed"
				locations := facets.Locations()
				locations[0] = Loc{}
				declarations := facets.Declarations()
				declarations[0] = StringEnumerationFacet{}
				again := child.StringEnumerationFacets()
				if got, want := again.Values(), []string{"first", "   ", " first "}; !reflect.DeepEqual(got, want) {
					t.Fatalf("token enumeration changed through copies = %#v, want %#v", got, want)
				}
				if got := again.Locations()[0]; got.IsZero() {
					t.Fatal("token enumeration location changed through a copy")
				}

				whiteSpace, ok := child.StringWhiteSpaceFacet()
				if !ok {
					t.Fatal("token whiteSpace facet is missing")
				}
				changedWhiteSpace := whiteSpace.WithFixed(false)
				if changedWhiteSpace.Fixed() || !whiteSpace.Fixed() {
					t.Fatal("token whiteSpace copy did not preserve independent fixed state")
				}
				againWhiteSpace, ok := child.StringWhiteSpaceFacet()
				if !ok || !againWhiteSpace.Fixed() || againWhiteSpace.Value() != "collapse" {
					t.Fatalf("token whiteSpace changed through a copy = (%q, fixed=%t)", againWhiteSpace.Value(), againWhiteSpace.Fixed())
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Keep policy-specific causes, locations, and no-schema assertions together.
func TestSchemaTokenRestrictionsPreserveDiagnosticsAndRejectExcludedFacets(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			cases := []struct {
				name        string
				body        string
				code        string
				class       FailureClass
				feature     FeatureID
				cause       error
				primaryLine int
				primaryMark string
				specSuffix  string
				relatedLine int
				relatedMark string
			}{
				{
					name: "invalid whitespace value",
					body: `
  <xs:simpleType name="item">
    <xs:restriction base="xs:token">
      <xs:whiteSpace value="preserve-ish"/>
    </xs:restriction>
  </xs:simpleType>
				`, code: InvalidStringWhiteSpaceCode, class: FailureInvalid, cause: errInvalidStringWhiteSpaceValue, primaryLine: 4, primaryMark: `value`, specSuffix: "rf-whiteSpace",
				},
				{
					name: "less restrictive whitespace",
					body: `
  <xs:simpleType name="base">
    <xs:restriction base="xs:token">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:whiteSpace value="replace"/>
    </xs:restriction>
  </xs:simpleType>
				`, code: InvalidStringWhiteSpaceRestrictionCode, class: FailureInvalid, cause: errInvalidStringWhiteSpaceRestriction, primaryLine: 9, primaryMark: `value`, specSuffix: "rf-whiteSpace", relatedLine: 4, relatedMark: `value`,
				},
				{
					name: "enumeration outside collapsed value space",
					body: `
  <xs:simpleType name="base">
    <xs:restriction base="xs:token">
      <xs:enumeration value=" first "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:enumeration value="second"/>
    </xs:restriction>
  </xs:simpleType>
				`, code: InvalidEnumerationRestrictionCode, class: FailureInvalid, cause: errInvalidEnumerationRestriction, primaryLine: 9, primaryMark: `value`, specSuffix: "enumeration-valid-restriction", relatedLine: 4, relatedMark: `value`,
				},
			}

			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					root := tokenDiagnosticSchemaRoot(profile.version, test.body)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("discoverSchema accepted an invalid token restriction")
					}
					assertTokenNoSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Feature() != test.feature {
						t.Fatalf("diagnostic = %s/%q/%q/%q, want %q/%q/%q", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature(), test.class, test.code, test.feature)
					}
					primary := mustSchemaTokenLoc(t, "root.xsd", root, test.primaryLine, test.primaryMark)
					if diagnostic.Loc() != primary {
						t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), primary)
					}
					if diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, test.specSuffix) {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), tokenDiagnosticSpecRef(profile.version, test.specSuffix))
					}
					if !errors.Is(err, test.cause) {
						t.Fatalf("diagnostic lost %v cause: %v", test.cause, err)
					}
					wantRelated := []Loc(nil)
					if test.relatedLine != 0 {
						wantRelated = []Loc{mustSchemaTokenLoc(t, "root.xsd", root, test.relatedLine, test.relatedMark)}
					}
					if got := diagnostic.Related(); !reflect.DeepEqual(got, wantRelated) {
						t.Fatalf("diagnostic related = %v, want %v", got, wantRelated)
					}
				})
			}

			for _, facet := range []struct {
				name  string
				value string
			}{
				{name: "length", value: "1"},
				{name: "minLength", value: "1"},
				{name: "maxLength", value: "1"},
				{name: "pattern", value: ".*"},
			} {
				t.Run("unsupported "+facet.name, func(t *testing.T) {
					body := `
  <xs:simpleType name="item">
    <xs:restriction base="xs:token">
      <xs:` + facet.name + ` value="` + facet.value + `"/>
    </xs:restriction>
  </xs:simpleType>
`
					root := tokenDiagnosticSchemaRoot(profile.version, body)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("discoverSchema silently accepted an excluded token facet")
					}
					assertTokenNoSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Feature() != FeatureDatatypeFacets {
						t.Fatalf("diagnostic = %s/%q/%q/%q, want datatype-facet unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, "<xs:"+facet.name) {
						t.Fatalf("diagnostic location = %s, want facet location", diagnostic.Loc())
					}
					if diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, "decimal") {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), tokenDiagnosticSpecRef(profile.version, "decimal"))
					}
					if !errors.Is(err, ErrUnsupported) {
						t.Fatalf("diagnostic lost unsupported classification: %v", err)
					}
				})
			}

			t.Run("assertion", func(t *testing.T) {
				body := `
  <xs:simpleType name="item">
    <xs:restriction base="xs:token">
      <xs:assertion test="true()"/>
    </xs:restriction>
  </xs:simpleType>
`
				root := tokenDiagnosticSchemaRoot(profile.version, body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema silently accepted an excluded token assertion")
				}
				assertTokenNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Feature() != FeatureID("xsd.assertion") {
					t.Fatalf("assertion diagnostic = %s/%q/%q/%q, want assertion unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, "<xs:assertion") || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("assertion diagnostic location or classification is wrong: %v", err)
				}
			})
		})
	}
}

func TestSchemaTokenRestrictionCyclesRemainLocatedAndProduceNoSchema(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := tokenDiagnosticSchemaRoot(profile.version, `
  <xs:simpleType name="one">
    <xs:restriction base="t:two"/>
  </xs:simpleType>
  <xs:simpleType name="two">
    <xs:restriction base="t:one"/>
  </xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("discoverSchema accepted a cyclic token restriction")
			}
			assertTokenNoSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
				t.Fatalf("cycle diagnostic = %s/%q/%q/%q, want located simple-type cycle", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.SpecRef())
			}
			if diagnostic.Loc().IsZero() || len(diagnostic.Related()) == 0 || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("cycle diagnostic lost locations or cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep direct, named, local-particle, and consumer gates together.
func TestSchemaTokenConsumerBoundariesRemainExplicitlyUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			for _, test := range []struct {
				name string
				root string
				body string
			}{
				{name: "direct", root: tokenConsumerDirectRoot(profile.version), body: `<item xmlns="urn:test">value</item>`},
				{name: "named", root: tokenConsumerNamedRoot(profile.version), body: `<item xmlns="urn:test">value</item>`},
			} {
				t.Run(test.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err != nil {
						t.Fatalf("discoverSchema: %v", err)
					}

					generated, err := GenerateGo(schema, "generated")
					if generated != nil || err == nil {
						t.Fatalf("GenerateGo result = (%q, %v), want explicit unsupported with no source", generated, err)
					}
					codegenDiagnostic := requireDiagnostic(t, err)
					if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Code() != diagnosticCodegenUnsupported || codegenDiagnostic.Feature() != FeatureCodegen {
						t.Fatalf("GenerateGo diagnostic = %s/%q/%q/%q, want codegen unsupported", codegenDiagnostic, codegenDiagnostic.Class(), codegenDiagnostic.Code(), codegenDiagnostic.Feature())
					}
					if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
						t.Fatalf("GenerateGo diagnostic lost unsupported cause: %v", err)
					}

					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.body)))
					if validationErr == nil {
						t.Fatal("ValidateInstance silently accepted xs:token")
					}
					validationDiagnostic := requireDiagnostic(t, validationErr)
					if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation {
						t.Fatalf("ValidateInstance diagnostic = %s/%q/%q/%q, want instance-validation unsupported", validationDiagnostic, validationDiagnostic.Class(), validationDiagnostic.Code(), validationDiagnostic.Feature())
					}
					if !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceUnsupportedType) {
						t.Fatalf("ValidateInstance diagnostic lost unsupported cause: %v", validationErr)
					}
				})
			}

			t.Run("local particle", func(t *testing.T) {
				root := tokenConsumerLocalRoot(profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema silently accepted local xs:token scalar use")
				}
				assertTokenNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("local token diagnostic = %s/%q/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `type="xs:token"`) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("local token diagnostic location or cause is wrong: %v", err)
				}
			})
		})
	}
}

type tokenPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func tokenPolicyProfiles() []tokenPolicyProfile {
	return []tokenPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
}

func assertTokenBuiltinReference(t *testing.T, reference SimpleTypeReference, wantLoc Loc) {
	t.Helper()
	wantName := mustTestQName(t, testXSDNamespace, "token")
	if !reference.IsBuiltin() || reference.Kind() != SimpleTypeReferenceBuiltin || reference.Name() != wantName || reference.QName() != wantName {
		t.Fatalf("token reference = %#v, want built-in xs:token", reference)
	}
	if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("token reference facts = %s/%s/%q, want use-site atomic restriction at %s", reference.Loc(), reference.VarietyLoc(), reference.Variety(), wantLoc)
	}
	if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicToken {
		t.Fatalf("token reference atomic facts = %#v, want private token category", reference.facts)
	}
	facets, ok := reference.facts.facets.(schemaStringFacetVariant)
	if !ok || facets.whiteSpace == nil || facets.whiteSpace.Value() != "collapse" || !facets.whiteSpace.Fixed() || !facets.whiteSpace.Loc().IsZero() {
		t.Fatalf("token reference whiteSpace facts = %#v/%t, want fixed unlocated collapse", facets, ok)
	}
	if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() || reference.facts.hasID {
		t.Fatalf("token reference component ID = %v/%t, want zero/false", typeID, hasTypeID)
	}
}

func assertTokenDefinition(t *testing.T, definition SimpleTypeDefinition) {
	t.Helper()
	if definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicToken || !definition.IsString() {
		t.Fatalf("token definition facts = %#v/string:%t, want private token category and string accessors", definition.facts, definition.IsString())
	}
	whiteSpace, ok := definition.StringWhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "collapse" || !whiteSpace.Fixed() {
		t.Fatalf("token definition whiteSpace = (%q, fixed=%t)/%t, want fixed collapse", whiteSpace.Value(), whiteSpace.Fixed(), ok)
	}
}

func tokenElementDefinition(t *testing.T, schema Schema, local string) ElementDeclaration {
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

func tokenDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	return schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:test", local)
}

func assertTokenNoSchema(t *testing.T, schema Schema) {
	t.Helper()
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema after a token error")
	}
}

func tokenDiagnosticSpecRef(version XSDVersion, suffix string) string {
	return "xsd" + string(version[0]) + string(version[2]) + "-datatypes#" + suffix
}

func tokenReferenceSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" xmlns:o="urn:other" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:token"/>
  <xs:element name="named" type="t:TokenAlias"/>
  <xs:element name="anonymous">
    <xs:simpleType>
      <xs:restriction base="xs:token">
        <xs:enumeration value=" inline "/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
  <xs:attribute name="directAttribute" type="xs:token"/>
  <xs:simpleType name="TokenAlias">
    <xs:restriction base="xs:token">
      <xs:enumeration value=" alias "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Forward">
    <xs:restriction base="t:Later"/>
  </xs:simpleType>
  <xs:simpleType name="Later">
    <xs:restriction base="xs:token"/>
  </xs:simpleType>
  <xs:simpleType name="ImportedAlias">
    <xs:restriction base="o:ImportedToken"/>
  </xs:simpleType>
  <xs:simpleType name="TokenList">
    <xs:list itemType="xs:token"/>
  </xs:simpleType>
  <xs:simpleType name="NamedList">
    <xs:list itemType="t:Forward"/>
  </xs:simpleType>
  <xs:simpleType name="TokenUnion">
    <xs:union memberTypes="xs:token t:Forward">
      <xs:simpleType>
        <xs:restriction base="xs:token">
          <xs:enumeration value=" union "/>
        </xs:restriction>
      </xs:simpleType>
    </xs:union>
  </xs:simpleType>
</xs:schema>`
}

func tokenReferenceSchemaOtherDocument(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(version) + `">
  <xs:simpleType name="ImportedToken">
    <xs:restriction base="xs:token"/>
  </xs:simpleType>
</xs:schema>`
}

func tokenEnumerationSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="Base">
    <xs:restriction base="xs:token">
      <xs:enumeration value="  first  "/>
      <xs:enumeration value=""/>
      <xs:enumeration value="first"/>
      <xs:enumeration value="   "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Child">
    <xs:restriction base="t:Base">
      <xs:enumeration value="first"/>
      <xs:enumeration value="   "/>
      <xs:enumeration value=" first "/>
      <xs:whiteSpace value="collapse" fixed="false"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Inherited">
    <xs:restriction base="t:Base"/>
  </xs:simpleType>
</xs:schema>`
}

func tokenDiagnosticSchemaRoot(version XSDVersion, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">` + body + `</xs:schema>`
}

func tokenConsumerDirectRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="item" type="xs:token"/>
</xs:schema>`
}

func tokenConsumerNamedRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="item" type="t:Token"/>
  <xs:simpleType name="Token">
    <xs:restriction base="xs:token"/>
  </xs:simpleType>
</xs:schema>`
}

func tokenConsumerLocalRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:complexType name="Box">
    <xs:sequence>
      <xs:element name="item" type="xs:token"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="box" type="t:Box"/>
</xs:schema>`
}
