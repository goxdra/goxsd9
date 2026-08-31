package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeBuildsExactNumericEnumerationRestrictions(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="integerBase">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="+0007"/>
      <xs:enumeration value="-0"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="integerInherited">
    <xs:restriction base="t:integerBase"/>
  </xs:simpleType>
  <xs:simpleType name="integerNarrowed">
    <xs:restriction base="t:integerBase">
      <xs:enumeration value="7"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="decimalBase">
    <xs:restriction base="xs:decimal">
      <xs:enumeration value="1.2300"/>
      <xs:enumeration value="-0.00"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="decimalInherited">
    <xs:restriction base="t:decimalBase"/>
  </xs:simpleType>
  <xs:simpleType name="decimalNarrowed">
    <xs:restriction base="t:decimalBase">
      <xs:enumeration value="1.23"/>
      <xs:enumeration value="0"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "xsd10", policy: Strict10, version: XSDVersion10},
		{name: "xsd11", policy: Strict11, version: XSDVersion11},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			integerBase := schemaEnumerationTestDefinition(t, schema, "integerBase")
			assertIntegerEnumerationFacts(t, integerBase.IntegerEnumerationFacets(), test.version, []string{"7", "0"}, []Loc{
				mustTestLoc(t, "root.xsd", 4, 7),
				mustTestLoc(t, "root.xsd", 5, 7),
			})
			integerInherited := schemaEnumerationTestDefinition(t, schema, "integerInherited")
			assertIntegerEnumerationFacts(t, integerInherited.IntegerEnumerationFacets(), test.version, []string{"7", "0"}, []Loc{
				mustTestLoc(t, "root.xsd", 4, 7),
				mustTestLoc(t, "root.xsd", 5, 7),
			})
			integerNarrowed := schemaEnumerationTestDefinition(t, schema, "integerNarrowed")
			assertIntegerEnumerationFacts(t, integerNarrowed.IntegerEnumerationFacets(), test.version, []string{"7"}, []Loc{
				mustTestLoc(t, "root.xsd", 13, 7),
			})

			decimalBase := schemaEnumerationTestDefinition(t, schema, "decimalBase")
			assertDecimalEnumerationFacts(t, decimalBase.DecimalEnumerationFacets(), test.version, []string{"1.23", "0"}, []Loc{
				mustTestLoc(t, "root.xsd", 18, 7),
				mustTestLoc(t, "root.xsd", 19, 7),
			})
			decimalInherited := schemaEnumerationTestDefinition(t, schema, "decimalInherited")
			assertDecimalEnumerationFacts(t, decimalInherited.DecimalEnumerationFacets(), test.version, []string{"1.23", "0"}, []Loc{
				mustTestLoc(t, "root.xsd", 18, 7),
				mustTestLoc(t, "root.xsd", 19, 7),
			})
			decimalNarrowed := schemaEnumerationTestDefinition(t, schema, "decimalNarrowed")
			assertDecimalEnumerationFacts(t, decimalNarrowed.DecimalEnumerationFacets(), test.version, []string{"1.23", "0"}, []Loc{
				mustTestLoc(t, "root.xsd", 27, 7),
				mustTestLoc(t, "root.xsd", 28, 7),
			})

			if integerBase.DecimalEnumerationFacets().HasEnumeration() {
				t.Fatal("integer definition exposes decimal enumeration facets")
			}
			if decimalBase.IntegerEnumerationFacets().HasEnumeration() {
				t.Fatal("decimal definition exposes integer enumeration facets")
			}
			owned := integerBase.IntegerEnumerationFacets().Declarations()
			owned[0] = owned[1]
			if got := integerBase.IntegerEnumerationFacets().Declarations()[0].Value().Canonical(); got != "7" {
				t.Fatalf("integer declarations were not owned: first value = %q", got)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep the edition and schema-graph enumeration matrix together.
func TestSchemaBridgeBuildsOrderedStringEnumerationFactsAcrossGraphs(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := stringEnumerationSchemaRoot(policy.version)
			other := stringEnumerationOtherDocument(policy.version)
			xmlSchema := stringEnumerationXMLSchema(policy.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: other},
				"xml.xsd":   {id: "xml.xsd", contents: xmlSchema},
			}, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			base := schemaEnumerationTestDefinition(t, schema, "base")
			assertStringEnumerationFacts(t, base.StringEnumerationFacets(), policy.version, []string{" first ", "", "first", "first"}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 6, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 7, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 8, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 9, "value"),
			})
			child := schemaEnumerationTestDefinition(t, schema, "child")
			assertStringEnumerationFacts(t, child.StringEnumerationFacets(), policy.version, []string{"", " first "}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 14, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 15, "value"),
			})
			forward := schemaEnumerationTestDefinition(t, schema, "forwardChild")
			assertStringEnumerationFacts(t, forward.StringEnumerationFacets(), policy.version, []string{"forward", ""}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 23, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 24, "value"),
			})
			remote := schemaEnumerationTestDefinition(t, schema, "remoteChild")
			assertStringEnumerationFacts(t, remote.StringEnumerationFacets(), policy.version, []string{" remote "}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 29, "value"),
			})
			remoteBase := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:other", "remote")
			assertStringEnumerationFacts(t, remoteBase.StringEnumerationFacets(), policy.version, []string{" remote ", ""}, []Loc{
				mustSchemaTokenLoc(t, "other.xsd", other, 4, "value"),
				mustSchemaTokenLoc(t, "other.xsd", other, 5, "value"),
			})
			xmlLang := schemaEnumerationTestDefinitionInNamespace(t, schema, "http://www.w3.org/XML/1998/namespace", "lang")
			assertStringEnumerationFacts(t, xmlLang.StringEnumerationFacets(), policy.version, []string{"en", ""}, []Loc{
				mustSchemaTokenLoc(t, "xml.xsd", xmlSchema, 4, "value"),
				mustSchemaTokenLoc(t, "xml.xsd", xmlSchema, 5, "value"),
			})

			unconstrained := schemaEnumerationTestDefinition(t, schema, "unconstrained")
			if unconstrained.StringEnumerationFacets().HasEnumeration() || unconstrained.StringEnumerationFacets().Values() != nil {
				t.Fatal("omitted built-in string enumeration was published as a present facet")
			}
			if base.IntegerEnumerationFacets().HasEnumeration() || base.DecimalEnumerationFacets().HasEnumeration() {
				t.Fatal("string definition exposed a numeric enumeration family")
			}

			components := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "inline"))
			if len(components) != 1 {
				t.Fatalf("inline element matches = %d, want one", len(components))
			}
			declaration, ok := components[0].ElementDeclaration()
			if !ok {
				t.Fatal("inline element declaration view is missing")
			}
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsAnonymous() {
				t.Fatalf("inline element type reference = %#v/%t, want anonymous", reference, ok)
			}
			anonymous, ok := reference.AnonymousType()
			if !ok {
				t.Fatal("inline element anonymous type is missing")
			}
			assertStringEnumerationFacts(t, anonymous.StringEnumerationFacets(), policy.version, []string{"inline", ""}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 38, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 39, "value"),
			})

			namedValues := base.StringEnumerationFacets().Values()
			namedValues[0] = "changed"
			namedFacts := base.StringEnumerationFacets().Values()
			if !reflect.DeepEqual(namedFacts, []string{" first ", "", "first", "first"}) {
				t.Fatalf("named string enumeration changed through returned values: %#v", namedFacts)
			}
			anonymousValues := anonymous.StringEnumerationFacets().Values()
			anonymousValues[0] = "changed"
			anonymousFacts := anonymous.StringEnumerationFacets().Values()
			if !reflect.DeepEqual(anonymousFacts, []string{"inline", ""}) {
				t.Fatalf("anonymous string enumeration changed through returned values: %#v", anonymousFacts)
			}
		})
	}
}

func TestSchemaBridgeBuildsInlineStringEnumerationFromNamedAndAnonymousBases(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := inlineStringEnumerationSchemaRoot(policy.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			named := schemaEnumerationTestInlineElementType(t, schema, "namedInline")
			assertStringEnumerationFacts(t, named.StringEnumerationFacets(), policy.version, []string{"", "named"}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 11, "value"),
				mustSchemaTokenLoc(t, "root.xsd", root, 12, "value"),
			})

			anonymous := schemaEnumerationTestInlineElementType(t, schema, "anonymousInline")
			assertStringEnumerationFacts(t, anonymous.StringEnumerationFacets(), policy.version, []string{"anonymous"}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 24, "value"),
			})
			base, ok := anonymous.BaseReference()
			if !ok || !base.IsAnonymous() {
				t.Fatalf("anonymous inline base = %#v/%t, want anonymous", base, ok)
			}
			anonymousBase, ok := base.AnonymousType()
			if !ok {
				t.Fatal("anonymous inline base model is missing")
			}
			assertStringEnumerationFacts(t, anonymousBase.StringEnumerationFacets(), policy.version, []string{"anonymous"}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 21, "value"),
			})
		})
	}
}

func schemaEnumerationTestInlineElementType(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", local))
	if len(components) != 1 {
		t.Fatalf("%s element matches = %d, want one", local, len(components))
	}
	element, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("%s element declaration view is missing", local)
	}
	definition, ok := element.InlineSimpleType()
	if !ok {
		t.Fatalf("%s inline simple type is missing", local)
	}
	return definition
}

func TestSchemaBridgeKeepsNamedNonStringInlineEnumerationUnsupported(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
		specRef string
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10, specRef: "xsd10-datatypes#decimal"},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11, specRef: "xsd11-datatypes#decimal"},
	} {
		t.Run(policy.name, func(t *testing.T) {
			assertNamedNonStringInlineEnumerationUnsupported(t, policy.value, policy.version, policy.specRef)
		})
	}
}

func assertNamedNonStringInlineEnumerationUnsupported(t *testing.T, policy LanguagePolicy, version XSDVersion, specRef string) {
	t.Helper()
	root := namedNonStringInlineEnumerationSchemaRoot(version)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discoverSchema accepted an enumeration on a non-string inline base")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, want unsupported datatype facet", diagnostic)
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 8, "<xs:enumeration") {
		t.Fatalf("diagnostic location = %s, want enumeration location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic SpecRef() = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
	}
}

func namedNonStringInlineEnumerationSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="integerBase">
    <xs:restriction base="xs:integer"/>
  </xs:simpleType>
  <xs:element name="inline">
    <xs:simpleType>
      <xs:restriction base="t:integerBase">
        <xs:enumeration value="1"/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
</xs:schema>`
}

func inlineStringEnumerationSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="namedBase">
    <xs:restriction base="xs:string">
      <xs:enumeration value="named"/>
      <xs:enumeration value=""/>
    </xs:restriction>
  </xs:simpleType>
  <xs:element name="namedInline">
    <xs:simpleType>
      <xs:restriction base="t:namedBase">
        <xs:enumeration value=""/>
        <xs:enumeration value="named"/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
  <xs:element name="anonymousInline">
    <xs:simpleType>
      <xs:restriction>
        <xs:simpleType>
          <xs:restriction base="xs:string">
            <xs:enumeration value="anonymous"/>
          </xs:restriction>
        </xs:simpleType>
        <xs:enumeration value="anonymous"/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
</xs:schema>`
}

func stringEnumerationSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" xmlns:o="urn:other" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:import namespace="http://www.w3.org/XML/1998/namespace" schemaLocation="xml.xsd"/>
  <xs:simpleType name="base">
    <xs:restriction base="xs:string">
      <xs:enumeration value=" first "/>
      <xs:enumeration value=""/>
      <xs:enumeration value="first"/>
      <xs:enumeration value="first"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:enumeration value=""/>
      <xs:enumeration value=" first "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="forwardChild">
    <xs:restriction base="t:forwardBase"/>
  </xs:simpleType>
  <xs:simpleType name="forwardBase">
    <xs:restriction base="xs:string">
      <xs:enumeration value="forward"/>
      <xs:enumeration value=""/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="remoteChild">
    <xs:restriction base="o:remote">
      <xs:enumeration value=" remote "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="unconstrained">
    <xs:restriction base="xs:string"/>
  </xs:simpleType>
  <xs:element name="inline">
    <xs:simpleType>
      <xs:restriction base="xs:string">
        <xs:enumeration value="inline"/>
        <xs:enumeration value=""/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
</xs:schema>`
}

func stringEnumerationOtherDocument(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(version) + `">
  <xs:simpleType name="remote">
    <xs:restriction base="xs:string">
      <xs:enumeration value=" remote "/>
      <xs:enumeration value=""/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

func stringEnumerationXMLSchema(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="http://www.w3.org/XML/1998/namespace" version="` + string(version) + `">
  <xs:simpleType name="lang">
    <xs:restriction base="xs:string">
      <xs:enumeration value="en"/>
      <xs:enumeration value=""/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

//nolint:gocognit,funlen // Keep the invalid declaration matrix and provenance assertions together.
func TestSchemaBridgeRejectsInvalidStringEnumerationDeclarations(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			for _, test := range []struct {
				name      string
				body      string
				valueLine int
				code      string
				spec      string
				cause     error
			}{
				{
					name: "missing value",
					body: `
  <xs:simpleType name="item">
    <xs:restriction base="xs:string">
      <xs:enumeration/>
    </xs:restriction>
  </xs:simpleType>
`, valueLine: 4, code: InvalidEnumerationCode, spec: "rf-enumeration", cause: errInvalidEnumerationValue,
				},
				{
					name: "direct simpleType placement",
					body: `
  <xs:simpleType name="item">
    <xs:enumeration value="item"/>
  </xs:simpleType>
`, valueLine: 3, code: InvalidEnumerationCode, spec: "rf-enumeration", cause: errInvalidEnumerationValue,
				},
				{
					name: "list placement",
					body: `
  <xs:simpleType name="item">
    <xs:list>
      <xs:enumeration value="item"/>
    </xs:list>
  </xs:simpleType>
`, valueLine: 4, code: InvalidEnumerationCode, spec: "rf-enumeration", cause: errInvalidEnumerationValue,
				},
				{
					name: "union placement",
					body: `
  <xs:simpleType name="item">
    <xs:union>
      <xs:enumeration value="item"/>
    </xs:union>
  </xs:simpleType>
`, valueLine: 4, code: InvalidEnumerationCode, spec: "rf-enumeration", cause: errInvalidEnumerationValue,
				},
				{
					name: "prohibited inherited value",
					body: `
  <xs:simpleType name="base">
    <xs:restriction base="xs:string">
      <xs:enumeration value="allowed"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:enumeration value="blocked"/>
    </xs:restriction>
  </xs:simpleType>
`, valueLine: 9, code: InvalidEnumerationRestrictionCode, spec: "enumeration-valid-restriction", cause: errInvalidEnumerationRestriction,
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					root := stringEnumerationDiagnosticSchema(policy.version, test.body)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
					if err == nil {
						t.Fatal("discoverSchema accepted invalid string enumeration behavior")
					}
					if schema.storage != nil {
						t.Fatal("discoverSchema returned a partial schema")
					}
					diagnostic := requireDiagnostic(t, err)
					valueLoc := mustSchemaTokenLoc(t, "root.xsd", root, test.valueLine, "<xs:enumeration")
					if test.name == "prohibited inherited value" {
						valueLoc = mustSchemaTokenLoc(t, "root.xsd", root, test.valueLine, "value")
					}
					if diagnostic.Code() != test.code || diagnostic.Loc() != valueLoc || diagnostic.SpecRef() != versionedEnumerationSpecRef(policy.version, test.spec) {
						t.Fatalf("diagnostic = %s, want %s at %s with %s", diagnostic, test.code, valueLoc, versionedEnumerationSpecRef(policy.version, test.spec))
					}
					if !errors.Is(err, test.cause) {
						t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
					}
					if test.name == "prohibited inherited value" {
						related := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 4, "value")}
						if got := diagnostic.Related(); !reflect.DeepEqual(got, related) {
							t.Fatalf("diagnostic Related() = %v, want %v", got, related)
						}
					}
				})
			}
		})
	}
}

func stringEnumerationDiagnosticSchema(version XSDVersion, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">` + body + `</xs:schema>`
}

func TestSchemaBridgeKeepsStringPatternUnsupported(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			assertStringPatternUnsupported(t, policy.value, policy.version)
		})
	}
}

func assertStringPatternUnsupported(t *testing.T, policy LanguagePolicy, version XSDVersion) {
	t.Helper()
	root := stringEnumerationDiagnosticSchema(version, `
  <xs:simpleType name="item">
    <xs:restriction base="xs:string">
      <xs:pattern value="item"/>
      <xs:enumeration value="item"/>
    </xs:restriction>
  </xs:simpleType>
`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discoverSchema accepted an unsupported string pattern")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, want unsupported datatype facet", diagnostic)
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, "<xs:pattern") {
		t.Fatalf("diagnostic Loc() = %s, want pattern location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
	}
}

//nolint:gocognit // Keep exact facet and related-diagnostic locations together.
func TestSchemaBridgePreservesNumericFacetValueLocations(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="Digits">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="3"/>
      <xs:fractionDigits value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="IntegerBound">
    <xs:restriction base="xs:integer">
      <xs:minInclusive value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="DecimalBound">
    <xs:restriction base="xs:decimal">
      <xs:maxExclusive value="2.0"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}

	digits := schemaEnumerationTestDefinition(t, schema, "Digits").DigitFacets()
	totalDigitsLoc := mustTestLoc(t, "root.xsd", 4, 23)
	if got, ok := digits.TotalDigitsLoc(); !ok || got != totalDigitsLoc {
		t.Fatalf("totalDigits Loc() = %s/%t, want %s/true", got, ok, totalDigitsLoc)
	}
	fractionDigitsLoc := mustTestLoc(t, "root.xsd", 5, 26)
	if got, ok := digits.FractionDigitsLoc(); !ok || got != fractionDigitsLoc {
		t.Fatalf("fractionDigits Loc() = %s/%t, want %s/true", got, ok, fractionDigitsLoc)
	}

	decimalValue, err := ParseStrictDecimal("1234", mustTestLoc(t, "instance.xml", 1, 1), XSDVersion11)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	diagnostic := requireDiagnostic(t, digits.ValidateDecimal(decimalValue, mustTestLoc(t, "instance.xml", 1, 1)))
	if got, want := diagnostic.Related(), []Loc{totalDigitsLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("totalDigits diagnostic Related() = %v, want %v", got, want)
	}
	decimalValue, err = ParseStrictDecimal("1.23", mustTestLoc(t, "instance.xml", 1, 1), XSDVersion11)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	diagnostic = requireDiagnostic(t, digits.ValidateDecimal(decimalValue, mustTestLoc(t, "instance.xml", 1, 1)))
	if got, want := diagnostic.Related(), []Loc{fractionDigitsLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fractionDigits diagnostic Related() = %v, want %v", got, want)
	}

	integerBounds, ok := schemaEnumerationTestDefinition(t, schema, "IntegerBound").IntegerBounds()
	if !ok || len(integerBounds.Bounds()) != 1 {
		t.Fatalf("IntegerBound bounds = %#v/%t, want one bound", integerBounds.Bounds(), ok)
	}
	integerBoundLoc := mustTestLoc(t, "root.xsd", 10, 24)
	if got := integerBounds.Bounds()[0].Loc(); got != integerBoundLoc {
		t.Fatalf("integer bound Loc() = %s, want %s", got, integerBoundLoc)
	}
	integerValue, err := ParseStrictInteger("0", mustTestLoc(t, "instance.xml", 1, 1))
	if err != nil {
		t.Fatalf("ParseStrictInteger: %v", err)
	}
	diagnostic = requireDiagnostic(t, integerBounds.ValidateInteger(integerValue, mustTestLoc(t, "instance.xml", 1, 1)))
	if got, want := diagnostic.Related(), []Loc{integerBoundLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("integer bound diagnostic Related() = %v, want %v", got, want)
	}

	decimalBounds, ok := schemaEnumerationTestDefinition(t, schema, "DecimalBound").DecimalBounds()
	if !ok || len(decimalBounds.Bounds()) != 1 {
		t.Fatalf("DecimalBound bounds = %#v/%t, want one bound", decimalBounds.Bounds(), ok)
	}
	decimalBoundLoc := mustTestLoc(t, "root.xsd", 15, 24)
	if got := decimalBounds.Bounds()[0].Loc(); got != decimalBoundLoc {
		t.Fatalf("decimal bound Loc() = %s, want %s", got, decimalBoundLoc)
	}
	decimalValue, err = ParseStrictDecimal("2.0", mustTestLoc(t, "instance.xml", 1, 1), XSDVersion11)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	diagnostic = requireDiagnostic(t, decimalBounds.ValidateDecimal(decimalValue, mustTestLoc(t, "instance.xml", 1, 1)))
	if got, want := diagnostic.Related(), []Loc{decimalBoundLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decimal bound diagnostic Related() = %v, want %v", got, want)
	}
}

func TestSchemaBridgeResolvesNumericEnumerationsAcrossMixedGraphs(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root" version="1.1">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="rootDecimal">
    <xs:restriction base="o:remoteDecimal">
      <xs:enumeration value="2.00"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.0">
  <xs:simpleType name="remoteDecimal">
    <xs:restriction base="xs:decimal">
      <xs:enumeration value="1.00"/>
      <xs:enumeration value="2"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	definition := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:root", "rootDecimal")
	assertDecimalEnumerationFacts(t, definition.DecimalEnumerationFacets(), XSDVersion10, []string{"2"}, []Loc{
		mustTestLoc(t, "root.xsd", 5, 7),
	})
	remote := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:other", "remoteDecimal")
	assertDecimalEnumerationFacts(t, remote.DecimalEnumerationFacets(), XSDVersion10, []string{"1", "2"}, []Loc{
		mustTestLoc(t, "other.xsd", 4, 7),
		mustTestLoc(t, "other.xsd", 5, 7),
	})
}

//nolint:gocognit // Keep diagnostic code, cause, specification, and location assertions together.
func TestSchemaBridgeRejectsInvalidNumericEnumerationDeclarations(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		policy  LanguagePolicy
		code    string
		loc     Loc
		specRef string
		cause   error
	}{
		{
			name: "invalid integer lexical value",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">
  <xs:simpleType name="item">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="1.2"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			policy:  Strict11,
			code:    InvalidEnumerationCode,
			loc:     mustTestLoc(t, "root.xsd", 4, 7),
			specRef: "xsd11-datatypes#rf-enumeration",
			cause:   errInvalidEnumerationValue,
		},
		{
			name: "derived value outside base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.0">
  <xs:simpleType name="base">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:enumeration value="2"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			policy:  Strict10,
			code:    InvalidEnumerationRestrictionCode,
			loc:     mustTestLoc(t, "root.xsd", 9, 7),
			specRef: "xsd10-datatypes#enumeration-valid-restriction",
			cause:   errInvalidEnumerationRestriction,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, test.policy)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid enumeration behavior")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Code() != test.code || diagnostic.Loc() != test.loc || diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic = %s, want %s at %s with %s", diagnostic, test.code, test.loc, test.specRef)
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
			if test.code == InvalidEnumerationRestrictionCode {
				if got, want := diagnostic.Related(), []Loc{mustTestLoc(t, "root.xsd", 4, 7)}; !reflect.DeepEqual(got, want) {
					t.Fatalf("related locations = %v, want %v", got, want)
				}
			}
		})
	}
}

//nolint:gocognit,funlen // Keep order, classification, location, and cause assertions together.
func TestSchemaBridgeCompletesNumericEnumerationBeforeUnsupportedFacet(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		code        string
		class       FailureClass
		loc         Loc
		cause       error
		unsupported bool
	}{
		{
			name: "excluded facet before malformed enumeration",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">
  <xs:simpleType name="item">
    <xs:restriction base="xs:decimal">
      <xs:pattern value="x"/>
      <xs:enumeration value="not-a-decimal"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			code:  InvalidEnumerationCode,
			class: FailureInvalid,
			loc:   mustTestLoc(t, "root.xsd", 5, 7),
			cause: errInvalidEnumerationValue,
		},
		{
			name: "malformed enumeration before excluded facet",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">
  <xs:simpleType name="item">
    <xs:restriction base="xs:decimal">
      <xs:enumeration value="not-a-decimal"/>
      <xs:pattern value="x"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			code:  InvalidEnumerationCode,
			class: FailureInvalid,
			loc:   mustTestLoc(t, "root.xsd", 4, 7),
			cause: errInvalidEnumerationValue,
		},
		{
			name: "valid enumeration with excluded facet",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">
  <xs:simpleType name="item">
    <xs:restriction base="xs:decimal">
      <xs:pattern value="x"/>
      <xs:enumeration value="1.0"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			code:        UnsupportedDatatypeFacetCode,
			class:       FailureUnsupported,
			loc:         mustTestLoc(t, "root.xsd", 4, 7),
			unsupported: true,
		},
		{
			name: "excluded facet before invalid derived enumeration",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="base">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:pattern value="x"/>
      <xs:enumeration value="2"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			code:  InvalidEnumerationRestrictionCode,
			class: FailureInvalid,
			loc:   mustTestLoc(t, "root.xsd", 10, 7),
			cause: errInvalidEnumerationRestriction,
		},
		{
			name: "invalid derived enumeration before excluded facet",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="base">
    <xs:restriction base="xs:integer">
      <xs:enumeration value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="child">
    <xs:restriction base="t:base">
      <xs:enumeration value="2"/>
      <xs:pattern value="x"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			code:  InvalidEnumerationRestrictionCode,
			class: FailureInvalid,
			loc:   mustTestLoc(t, "root.xsd", 9, 7),
			cause: errInvalidEnumerationRestriction,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("discoverSchema accepted invalid or unsupported numeric facets")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != test.loc {
				t.Fatalf("diagnostic = %s, want %s/%s at %s", diagnostic, test.class, test.code, test.loc)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
			if test.unsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
			}
		})
	}
}

type schemaEnumerationBaseDigitSpaceCase struct {
	name     string
	root     string
	valueLoc Loc
	related  []Loc
	datatype string
}

type schemaEnumerationBaseDigitSpacePolicy struct {
	name    string
	value   LanguagePolicy
	specRef string
}

func TestSchemaBridgeRejectsEnumerationOutsideNamedBaseDigitValueSpace(t *testing.T) {
	for _, test := range schemaEnumerationBaseDigitSpaceCases(t) {
		for _, policy := range schemaEnumerationBaseDigitSpacePolicies() {
			t.Run(test.name+"/"+policy.name, func(t *testing.T) {
				assertSchemaEnumerationBaseDigitSpaceRejected(t, test, policy)
			})
		}
	}
}

func schemaEnumerationBaseDigitSpaceCases(t *testing.T) []schemaEnumerationBaseDigitSpaceCase {
	t.Helper()
	return []schemaEnumerationBaseDigitSpaceCase{
		{
			name: "integer",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="integerBase">
    <xs:restriction base="xs:integer">
      <xs:totalDigits value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="integerChild">
    <xs:restriction base="t:integerBase">
      <xs:enumeration value="12"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			valueLoc: mustTestLoc(t, "root.xsd", 9, 7),
			related:  []Loc{mustTestLoc(t, "root.xsd", 4, 23)},
			datatype: "integer",
		},
		{
			name: "decimal",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">
  <xs:simpleType name="decimalBase">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="1"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="decimalChild">
    <xs:restriction base="t:decimalBase">
      <xs:enumeration value="12"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
			valueLoc: mustTestLoc(t, "root.xsd", 9, 7),
			related:  []Loc{mustTestLoc(t, "root.xsd", 4, 23)},
			datatype: "decimal",
		},
	}
}

func schemaEnumerationBaseDigitSpacePolicies() []schemaEnumerationBaseDigitSpacePolicy {
	return []schemaEnumerationBaseDigitSpacePolicy{
		{name: "xsd10", value: Strict10, specRef: "xsd10-datatypes#enumeration-valid-restriction"},
		{name: "xsd11", value: Strict11, specRef: "xsd11-datatypes#enumeration-valid-restriction"},
	}
}

func assertSchemaEnumerationBaseDigitSpaceRejected(
	t *testing.T,
	test schemaEnumerationBaseDigitSpaceCase,
	policy schemaEnumerationBaseDigitSpacePolicy,
) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.value)
	if err == nil {
		t.Fatal("discoverSchema accepted an enumeration outside the named base value space")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != InvalidEnumerationRestrictionCode || diagnostic.Loc() != test.valueLoc {
		t.Fatalf("diagnostic = %s, want %s at %s", diagnostic, InvalidEnumerationRestrictionCode, test.valueLoc)
	}
	if diagnostic.SpecRef() != policy.specRef {
		t.Fatalf("diagnostic SpecRef() = %q, want %q", diagnostic.SpecRef(), policy.specRef)
	}
	if got := diagnostic.Related(); !reflect.DeepEqual(got, test.related) {
		t.Fatalf("diagnostic Related() = %v, want %v", got, test.related)
	}
	if !errors.Is(err, errInvalidEnumerationRestriction) {
		t.Fatalf("diagnostic does not preserve enumeration restriction cause: %v", err)
	}
	if !errors.Is(err, errDigitFacetValueViolation) {
		t.Fatalf("diagnostic does not preserve %s digit cause: %v", test.datatype, err)
	}
}

//nolint:gocognit // Keep unsupported-facet classification and no-partial-schema assertions together.
func TestSchemaBridgeKeepsExcludedNumericFacetsUnsupported(t *testing.T) {
	for _, facet := range []string{"pattern", "whiteSpace", "length"} {
		t.Run(facet, func(t *testing.T) {
			value := "1"
			if facet == "whiteSpace" {
				value = "collapse"
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:` + facet + ` value="` + value + `"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an excluded numeric facet")
			}
			if schema.storage != nil {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode {
				t.Fatalf("diagnostic = %s, want unsupported datatype facet", diagnostic)
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
			}
		})
	}
}

func schemaEnumerationTestDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	return schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:test", local)
}

func schemaEnumerationTestDefinitionInNamespace(t *testing.T, schema Schema, namespace, local string) SimpleTypeDefinition {
	t.Helper()
	component := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, namespace, local))
	if len(component) != 1 {
		t.Fatalf("%s definitions = %d, want one", local, len(component))
	}
	definition, ok := component[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type definition view", local)
	}
	return definition
}

func assertIntegerEnumerationFacts(t *testing.T, facets IntegerEnumerationFacets, version XSDVersion, values []string, locations []Loc) {
	t.Helper()
	if !facets.HasEnumeration() || facets.Len() != len(values) || facets.Version() != version {
		t.Fatalf("integer enumeration facts = (has=%t, len=%d, version=%q), want (true, %d, %q)", facets.HasEnumeration(), facets.Len(), facets.Version(), len(values), version)
	}
	gotValues := facets.Values()
	gotLocations := facets.Locations()
	if len(gotValues) != len(values) || !reflect.DeepEqual(gotLocations, locations) {
		t.Fatalf("integer enumeration facts = (%v, %v), want (%v, %v)", gotValues, gotLocations, values, locations)
	}
	for index, value := range values {
		if gotValues[index].Canonical() != value {
			t.Fatalf("integer value %d = %q, want %q", index, gotValues[index].Canonical(), value)
		}
		if declaration := facets.Declarations()[index]; declaration.Loc() != locations[index] || declaration.Value().Canonical() != value {
			t.Fatalf("integer declaration %d = (%q, %s), want (%q, %s)", index, declaration.Value().Canonical(), declaration.Loc(), value, locations[index])
		}
	}
}

func assertDecimalEnumerationFacts(t *testing.T, facets DecimalEnumerationFacets, version XSDVersion, values []string, locations []Loc) {
	t.Helper()
	if !facets.HasEnumeration() || facets.Len() != len(values) || facets.Version() != version {
		t.Fatalf("decimal enumeration facts = (has=%t, len=%d, version=%q), want (true, %d, %q)", facets.HasEnumeration(), facets.Len(), facets.Version(), len(values), version)
	}
	gotValues := facets.Values()
	gotLocations := facets.Locations()
	if len(gotValues) != len(values) || !reflect.DeepEqual(gotLocations, locations) {
		t.Fatalf("decimal enumeration facts = (%v, %v), want (%v, %v)", gotValues, gotLocations, values, locations)
	}
	for index, value := range values {
		want, err := ParseStrictDecimalFor(version, value, Loc{})
		if err != nil {
			t.Fatalf("ParseStrictDecimalFor(%q): %v", value, err)
		}
		if !gotValues[index].Equal(want) {
			t.Fatalf("decimal value %d = %q, want value equivalent to %q", index, gotValues[index].Canonical(), value)
		}
		if declaration := facets.Declarations()[index]; declaration.Loc() != locations[index] || !declaration.Value().Equal(want) {
			t.Fatalf("decimal declaration %d = (%q, %s), want value equivalent to (%q, %s)", index, declaration.Value().Canonical(), declaration.Loc(), value, locations[index])
		}
	}
}

func assertStringEnumerationFacts(t *testing.T, facets StringEnumerationFacets, version XSDVersion, values []string, locations []Loc) {
	t.Helper()
	if !facets.HasEnumeration() || facets.Len() != len(values) || facets.Version() != version {
		t.Fatalf("string enumeration facts = (has=%t, len=%d, version=%q), want (true, %d, %q)", facets.HasEnumeration(), facets.Len(), facets.Version(), len(values), version)
	}
	if got := facets.Values(); !reflect.DeepEqual(got, values) {
		t.Fatalf("string enumeration values = %#v, want %#v", got, values)
	}
	if got := facets.Locations(); !reflect.DeepEqual(got, locations) {
		t.Fatalf("string enumeration locations = %#v, want %#v", got, locations)
	}
	declarations := facets.Declarations()
	for index, value := range values {
		if declarations[index].Value() != value || declarations[index].Loc() != locations[index] {
			t.Fatalf("string declaration %d = (%q, %s), want (%q, %s)", index, declarations[index].Value(), declarations[index].Loc(), value, locations[index])
		}
	}
}

func mustSchemaTokenLoc(t *testing.T, source SourceID, contents string, line int, token string) Loc {
	t.Helper()
	lines := strings.Split(contents, "\n")
	if line < 1 || line > len(lines) {
		t.Fatalf("line %d is outside source with %d lines", line, len(lines))
	}
	column := strings.Index(lines[line-1], token)
	if column < 0 {
		t.Fatalf("source line %d does not contain %q: %q", line, token, lines[line-1])
	}
	return mustTestLoc(t, source, line, column+1)
}
