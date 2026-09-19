package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestStringWhiteSpaceFacetParsingKeepsValueAndFixedState(t *testing.T) {
	loc := mustTestLoc(t, "facet.xsd", 4, 9)
	for _, value := range []string{"preserve", "replace", "collapse"} {
		facet, err := ParseStringWhiteSpaceFacetForWithFixed(XSDVersion10, " \t"+value+"\n ", loc, true)
		if err != nil {
			t.Fatalf("ParseStringWhiteSpaceFacetForWithFixed(%q): %v", value, err)
		}
		if facet.Value() != value || facet.Loc() != loc || !facet.Fixed() {
			t.Fatalf("facet = (%q, %s, fixed=%t), want (%q, %s, fixed=true)", facet.Value(), facet.Loc(), facet.Fixed(), value, loc)
		}
		unfixed := facet.WithFixed(false)
		if !facet.Fixed() || unfixed.Fixed() {
			t.Fatal("WithFixed did not preserve independent fixed state")
		}
	}

	_, err := ParseStringWhiteSpaceFacetFor(XSDVersion11, "preserve-ish", loc)
	if err == nil {
		t.Fatal("ParseStringWhiteSpaceFacetFor accepted an invalid whiteSpace value")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidStringWhiteSpaceCode || diagnostic.Loc() != loc || diagnostic.SpecRef() != stringWhiteSpaceXSD11SpecRef {
		t.Fatalf("diagnostic = %s, class=%q code=%q loc=%s spec=%q", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef())
	}
	if !errors.Is(err, errInvalidStringWhiteSpaceValue) {
		t.Fatalf("invalid whiteSpace error lost its cause: %v", err)
	}
}

//nolint:gocognit,funlen // Keep the policy, graph, provenance, and immutability matrix together.
func TestSchemaBridgeBuildsStringWhiteSpaceFactsAcrossPoliciesAndGraphs(t *testing.T) {
	policies := []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", value: Compatibility, version: XSDVersion11},
		{name: "Strict10", value: Strict10, version: XSDVersion10},
		{name: "Strict11", value: Strict11, version: XSDVersion11},
	}
	for _, syntaxVersion := range []XSDVersion{XSDVersion10, XSDVersion11} {
		for _, policy := range policies {
			t.Run(string(syntaxVersion)+"/"+policy.name, func(t *testing.T) {
				root := stringWhiteSpaceSchemaRoot(syntaxVersion)
				other := stringWhiteSpaceOtherDocument(syntaxVersion)
				schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
					"other.xsd": {id: "other.xsd", contents: other},
				}, policy.value)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}

				for _, want := range []struct {
					name  string
					value string
					fixed bool
				}{
					{name: "preserveFalse", value: "preserve"},
					{name: "preserveTrue", value: "preserve", fixed: true},
					{name: "replaceFalse", value: "replace"},
					{name: "replaceTrue", value: "replace", fixed: true},
					{name: "collapseFalse", value: "collapse"},
					{name: "collapseTrue", value: "collapse", fixed: true},
				} {
					definition := schemaEnumerationTestDefinition(t, schema, want.name)
					assertStringWhiteSpaceFacet(t, definition, want.value, want.fixed, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, want.name))
				}

				inherited := schemaEnumerationTestDefinition(t, schema, "inherited")
				assertStringWhiteSpaceFacet(t, inherited, "replace", false, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, "replaceFalse"))
				narrowed := schemaEnumerationTestDefinition(t, schema, "narrowed")
				assertStringWhiteSpaceFacet(t, narrowed, "collapse", false, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, "narrowed"))
				fixedInherited := schemaEnumerationTestDefinition(t, schema, "fixedInherited")
				assertStringWhiteSpaceFacet(t, fixedInherited, "replace", true, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, "replaceTrue"))
				sameFixed := schemaEnumerationTestDefinition(t, schema, "sameFixed")
				assertStringWhiteSpaceFacet(t, sameFixed, "replace", true, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, "sameFixed"))

				forward := schemaEnumerationTestDefinition(t, schema, "forward")
				assertStringWhiteSpaceFacet(t, forward, "replace", false, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, "forwardBase"))
				remoteChild := schemaEnumerationTestDefinition(t, schema, "remoteChild")
				assertStringWhiteSpaceFacet(t, remoteChild, "collapse", false, stringWhiteSpaceTypeValueLoc(t, "other.xsd", other, "remote"))
				remote := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:other", "remote")
				assertStringWhiteSpaceFacet(t, remote, "collapse", false, stringWhiteSpaceTypeValueLoc(t, "other.xsd", other, "remote"))

				plain := schemaEnumerationTestDefinition(t, schema, "plain")
				assertStringWhiteSpaceFacet(t, plain, "preserve", false, Loc{})
				baseReference, ok := plain.BaseReference()
				if !ok || !baseReference.IsBuiltin() || baseReference.facts == nil {
					t.Fatalf("plain base reference = %#v/%t, want a built-in reference with facts", baseReference, ok)
				}
				baseFacets, ok := baseReference.facts.facets.(schemaStringFacetVariant)
				if !ok || baseFacets.whiteSpace == nil {
					t.Fatalf("built-in string facets = %#v/%t, want whiteSpace facts", baseFacets, ok)
				}
				if baseFacets.whiteSpace.Value() != "preserve" || baseFacets.whiteSpace.Fixed() || !baseFacets.whiteSpace.Loc().IsZero() || baseReference.facts.hasID {
					t.Fatalf("built-in string whiteSpace = (%q, fixed=%t, loc=%s), ID=%t; want preserve/false/zero/no ID", baseFacets.whiteSpace.Value(), baseFacets.whiteSpace.Fixed(), baseFacets.whiteSpace.Loc(), baseReference.facts.hasID)
				}

				inline := schemaEnumerationTestInlineElementType(t, schema, "inline")
				assertStringWhiteSpaceFacet(t, inline, "collapse", true, stringWhiteSpaceInlineValueLoc(t, "root.xsd", root))

				combined := schemaEnumerationTestDefinition(t, schema, "combined")
				assertStringWhiteSpaceFacet(t, combined, "replace", false, stringWhiteSpaceTypeValueLoc(t, "root.xsd", root, "combined"))
				assertStringEnumerationFacts(t, combined.StringEnumerationFacets(), policy.version, []string{"first", "second"}, []Loc{
					stringWhiteSpaceTypeEnumerationLoc(t, "root.xsd", root, "combined", 1),
					stringWhiteSpaceTypeEnumerationLoc(t, "root.xsd", root, "combined", 2),
				})

				copied, ok := combined.StringWhiteSpaceFacet()
				if !ok {
					t.Fatal("combined whiteSpace facet is missing on the second read")
				}
				changed := copied.WithFixed(true)
				if !changed.Fixed() || copied.Fixed() {
					t.Fatal("WithFixed did not return an independent immutable view")
				}
				again, ok := combined.StringWhiteSpaceFacet()
				if !ok || again.Fixed() || again.Value() != "replace" || again.Loc() != copied.Loc() {
					t.Fatalf("whiteSpace facts changed through a returned copy = (%q, fixed=%t, loc=%s)", again.Value(), again.Fixed(), again.Loc())
				}
			})
		}
	}
}

type stringWhiteSpaceInvalidCase struct {
	name       string
	body       string
	code       string
	cause      error
	valueIndex int
	related    int
}

func TestSchemaBridgeRejectsInvalidStringWhiteSpaceRestrictions(t *testing.T) {
	policies := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	cases := []stringWhiteSpaceInvalidCase{
		{
			name: "duplicate",
			body: `
  <xs:simpleType name="duplicate">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="preserve"/>
      <xs:whiteSpace value="replace"/>
    </xs:restriction>
  </xs:simpleType>
`,
			code:       InvalidStringWhiteSpaceCode,
			cause:      errDuplicateStringWhiteSpaceFacet,
			valueIndex: 2,
		},
		{
			name: "malformed",
			body: `
  <xs:simpleType name="malformed">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="preserve-ish"/>
    </xs:restriction>
  </xs:simpleType>
`,
			code:       InvalidStringWhiteSpaceCode,
			cause:      errInvalidStringWhiteSpaceValue,
			valueIndex: 1,
		},
		{
			name: "less restrictive",
			body: `
  <xs:simpleType name="base">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:whiteSpace value="replace"/>
    </xs:restriction>
  </xs:simpleType>
`,
			code:       InvalidStringWhiteSpaceRestrictionCode,
			cause:      errInvalidStringWhiteSpaceRestriction,
			valueIndex: 2,
			related:    1,
		},
		{
			name: "fixed base",
			body: `
  <xs:simpleType name="base">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="replace" fixed="true"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
`,
			code:       InvalidStringWhiteSpaceRestrictionCode,
			cause:      errInvalidStringWhiteSpaceRestriction,
			valueIndex: 2,
			related:    1,
		},
	}
	for _, policy := range policies {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				assertInvalidStringWhiteSpaceCase(t, policy.policy, policy.version, test)
			})
		}
	}
}

func assertInvalidStringWhiteSpaceCase(t *testing.T, policy LanguagePolicy, version XSDVersion, test stringWhiteSpaceInvalidCase) {
	t.Helper()
	root := stringWhiteSpaceDiagnosticSchema(version, test.body)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discoverSchema accepted invalid string whiteSpace behavior")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.SpecRef() != stringWhiteSpaceSpecRef(version) {
		t.Fatalf("diagnostic = %s, class=%q code=%q spec=%q", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.SpecRef())
	}
	wantLoc := stringWhiteSpaceNthValueLoc(t, "root.xsd", root, test.valueIndex)
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
	wantRelated := []Loc(nil)
	if test.related > 0 {
		wantRelated = []Loc{stringWhiteSpaceNthValueLoc(t, "root.xsd", root, test.related)}
	}
	if got := diagnostic.Related(); !reflect.DeepEqual(got, wantRelated) {
		t.Fatalf("diagnostic related = %v, want %v", got, wantRelated)
	}
	if !errors.Is(err, test.cause) {
		t.Fatalf("diagnostic lost %s cause: %v", test.name, err)
	}
}

func TestSchemaBridgeKeepsStringWhiteSpaceUnsupportedForBoolean(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "Strict10", value: Strict10, version: XSDVersion10},
		{name: "Strict11", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(policy.version) + `">
  <xs:simpleType name="boolean">
    <xs:restriction base="xs:boolean">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err == nil {
				t.Fatal("discoverSchema accepted boolean whiteSpace")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Feature() != FeatureDatatypeFacets {
				t.Fatalf("diagnostic = %s, class=%q code=%q feature=%q", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("boolean whiteSpace diagnostic does not match ErrUnsupported: %v", err)
			}
		})
	}
}

func TestSchemaBridgeClassifiesStringWhiteSpaceOnUnsupportedNumericBases(t *testing.T) {
	for _, profile := range stringWhiteSpacePolicyProfiles() {
		for _, base := range []string{"xs:integer", "xs:decimal"} {
			t.Run(profile.name+"/"+base+"/malformed", func(t *testing.T) {
				assertUnsupportedBaseWhiteSpaceMalformed(t, profile, base)
			})
			t.Run(profile.name+"/"+base+"/valid", func(t *testing.T) {
				assertUnsupportedBaseWhiteSpaceUnsupported(t, profile, base)
			})
		}
	}
}

func TestSchemaBridgeClassifiesStringWhiteSpaceOnOpaqueAtomicBases(t *testing.T) {
	for _, profile := range stringWhiteSpacePolicyProfiles() {
		for _, base := range []string{"xs:language", "xs:NCName", "xs:anyURI", "xs:ID"} {
			t.Run(profile.name+"/"+base+"/malformed", func(t *testing.T) {
				assertUnsupportedBaseWhiteSpaceMalformed(t, profile, base)
			})
			t.Run(profile.name+"/"+base+"/valid", func(t *testing.T) {
				assertUnsupportedBaseWhiteSpaceUnsupported(t, profile, base)
			})
		}
	}
}

func TestSchemaBridgeStringWhiteSpaceCycleReturnsNoSchema(t *testing.T) {
	for _, profile := range stringWhiteSpacePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertStringWhiteSpaceCycleReturnsNoSchema(t, profile)
		})
	}
}

func assertStringWhiteSpaceCycleReturnsNoSchema(t *testing.T, profile stringWhiteSpacePolicyProfile) {
	t.Helper()
	root := stringWhiteSpaceDiagnosticSchema(profile.version, `
  <xs:simpleType name="one">
    <xs:restriction base="t:two">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="two">
    <xs:restriction base="t:one">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err == nil {
		t.Fatal("discoverSchema accepted a cyclic whiteSpace restriction")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema for a cycle")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode {
		t.Fatalf("diagnostic = %s, class=%q code=%q, want invalid simple-type cycle", diagnostic, diagnostic.Class(), diagnostic.Code())
	}
	if diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
		t.Fatalf("cycle spec ref = %q, want %q", diagnostic.SpecRef(), schemaSimpleTypeSpecRef(profile.version))
	}
	if diagnostic.Loc().IsZero() || len(diagnostic.Related()) == 0 {
		t.Fatalf("cycle locations = %s/%v, want primary and related locations", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
		t.Fatalf("cycle diagnostic lost its cause: %v", err)
	}
}

type stringWhiteSpacePolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func stringWhiteSpacePolicyProfiles() []stringWhiteSpacePolicyProfile {
	return []stringWhiteSpacePolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
}

func assertUnsupportedBaseWhiteSpaceMalformed(t *testing.T, profile stringWhiteSpacePolicyProfile, base string) {
	t.Helper()
	root := stringWhiteSpaceUnsupportedBaseSchema(profile.version, base, "preserve-ish")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err == nil {
		t.Fatal("discoverSchema accepted a malformed whiteSpace value on an unsupported base")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema for malformed whiteSpace")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidStringWhiteSpaceCode {
		t.Fatalf("diagnostic = %s, class=%q code=%q, want invalid/%s", diagnostic, diagnostic.Class(), diagnostic.Code(), InvalidStringWhiteSpaceCode)
	}
	if diagnostic.SpecRef() != stringWhiteSpaceSpecRef(profile.version) {
		t.Fatalf("malformed whiteSpace spec ref = %q, want %q", diagnostic.SpecRef(), stringWhiteSpaceSpecRef(profile.version))
	}
	wantLoc := stringWhiteSpaceNthValueLoc(t, "root.xsd", root, 1)
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("malformed whiteSpace location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
	if !errors.Is(err, errInvalidStringWhiteSpaceValue) {
		t.Fatalf("malformed whiteSpace diagnostic lost its cause: %v", err)
	}
}

func assertUnsupportedBaseWhiteSpaceUnsupported(t *testing.T, profile stringWhiteSpacePolicyProfile, base string) {
	t.Helper()
	root := stringWhiteSpaceUnsupportedBaseSchema(profile.version, base, "collapse")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err == nil {
		t.Fatal("discoverSchema accepted whiteSpace on an unsupported base")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema for unsupported whiteSpace")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, class=%q code=%q, want unsupported/%s", diagnostic, diagnostic.Class(), diagnostic.Code(), UnsupportedDatatypeFacetCode)
	}
	if diagnostic.Feature() != FeatureDatatypeFacets {
		t.Fatalf("unsupported whiteSpace feature = %q, want %q", diagnostic.Feature(), FeatureDatatypeFacets)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("unsupported whiteSpace location = %s, want root.xsd location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported whiteSpace diagnostic lost ErrUnsupported: %v", err)
	}
}

func stringWhiteSpaceUnsupportedBaseSchema(version XSDVersion, base, value string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="item">
    <xs:restriction base="` + base + `">
      <xs:whiteSpace value="` + value + `"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

func assertStringWhiteSpaceFacet(t *testing.T, definition SimpleTypeDefinition, value string, fixed bool, loc Loc) {
	t.Helper()
	facet, ok := definition.StringWhiteSpaceFacet()
	if !ok {
		t.Fatalf("%s string whiteSpace facet is absent", definition.Name())
	}
	if facet.Value() != value || facet.Fixed() != fixed || facet.Loc() != loc {
		t.Fatalf("%s string whiteSpace = (%q, fixed=%t, loc=%s), want (%q, fixed=%t, loc=%s)", definition.Name(), facet.Value(), facet.Fixed(), facet.Loc(), value, fixed, loc)
	}
}

func stringWhiteSpaceSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" xmlns:o="urn:other" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="preserveFalse">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="preserve" fixed="false"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="preserveTrue">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="preserve" fixed="true"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="replaceFalse">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="replace" fixed="false"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="replaceTrue">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="replace" fixed="true"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="collapseFalse">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="collapse" fixed="false"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="collapseTrue">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="collapse" fixed="true"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="inherited">
    <xs:restriction base="t:replaceFalse"/>
  </xs:simpleType>
  <xs:simpleType name="narrowed">
    <xs:restriction base="t:replaceFalse">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="fixedInherited">
    <xs:restriction base="t:replaceTrue"/>
  </xs:simpleType>
  <xs:simpleType name="sameFixed">
    <xs:restriction base="t:replaceTrue">
      <xs:whiteSpace value="replace"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="forward">
    <xs:restriction base="t:forwardBase"/>
  </xs:simpleType>
  <xs:simpleType name="forwardBase">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="replace"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="remoteChild">
    <xs:restriction base="o:remote"/>
  </xs:simpleType>
  <xs:simpleType name="plain">
    <xs:restriction base="xs:string"/>
  </xs:simpleType>
  <xs:simpleType name="combined">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="replace"/>
      <xs:enumeration value="first"/>
      <xs:enumeration value="second"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:element name="inline">
    <xs:simpleType>
      <xs:restriction base="xs:string">
        <xs:whiteSpace value="collapse" fixed="true"/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
</xs:schema>`
}

func stringWhiteSpaceOtherDocument(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(version) + `">
  <xs:simpleType name="remote">
    <xs:restriction base="xs:string">
      <xs:whiteSpace value="collapse"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

func stringWhiteSpaceDiagnosticSchema(version XSDVersion, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">` + body + `</xs:schema>`
}

func stringWhiteSpaceTypeValueLoc(t *testing.T, source SourceID, contents, typeName string) Loc {
	t.Helper()
	lines := strings.Split(contents, "\n")
	marker := `name="` + typeName + `"`
	inType := false
	for index, line := range lines {
		if strings.Contains(line, "<xs:simpleType") && strings.Contains(line, marker) {
			inType = true
			continue
		}
		if !inType {
			continue
		}
		if strings.Contains(line, "</xs:simpleType>") {
			break
		}
		if strings.Contains(line, "<xs:whiteSpace") {
			return mustSchemaTokenLoc(t, source, contents, index+1, "value")
		}
	}
	t.Fatalf("source simpleType %q does not contain whiteSpace", typeName)
	return Loc{}
}

func stringWhiteSpaceTypeEnumerationLoc(t *testing.T, source SourceID, contents, typeName string, ordinal int) Loc {
	t.Helper()
	lines := strings.Split(contents, "\n")
	marker := `name="` + typeName + `"`
	inType := false
	seen := 0
	for index, line := range lines {
		if strings.Contains(line, "<xs:simpleType") && strings.Contains(line, marker) {
			inType = true
			continue
		}
		if !inType {
			continue
		}
		if strings.Contains(line, "</xs:simpleType>") {
			break
		}
		if !strings.Contains(line, "<xs:enumeration") {
			continue
		}
		seen++
		if seen == ordinal {
			return mustSchemaTokenLoc(t, source, contents, index+1, "value")
		}
	}
	t.Fatalf("source simpleType %q does not contain enumeration %d", typeName, ordinal)
	return Loc{}
}

func stringWhiteSpaceNthValueLoc(t *testing.T, source SourceID, contents string, ordinal int) Loc {
	t.Helper()
	lines := strings.Split(contents, "\n")
	seen := 0
	for index, line := range lines {
		if !strings.Contains(line, "<xs:whiteSpace") {
			continue
		}
		seen++
		if seen == ordinal {
			return mustSchemaTokenLoc(t, source, contents, index+1, "value")
		}
	}
	t.Fatalf("source does not contain whiteSpace value %d", ordinal)
	return Loc{}
}

func stringWhiteSpaceInlineValueLoc(t *testing.T, source SourceID, contents string) Loc {
	t.Helper()
	lines := strings.Split(contents, "\n")
	inElement := false
	for index, line := range lines {
		if strings.Contains(line, `<xs:element name="inline"`) {
			inElement = true
			continue
		}
		if !inElement {
			continue
		}
		if strings.Contains(line, "</xs:element>") {
			break
		}
		if strings.Contains(line, "<xs:whiteSpace") {
			return mustSchemaTokenLoc(t, source, contents, index+1, "value")
		}
	}
	t.Fatal("inline element does not contain whiteSpace")
	return Loc{}
}
