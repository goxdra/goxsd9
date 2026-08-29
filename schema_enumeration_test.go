package goxsd9

import (
	"errors"
	"reflect"
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
			related:  []Loc{mustTestLoc(t, "root.xsd", 4, 7)},
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
			related:  []Loc{mustTestLoc(t, "root.xsd", 4, 7)},
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
