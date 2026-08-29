package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const validationPrecisionDecimalNamespace = "urn:precision-decimal"

func TestValidateInstanceSupportsPrecisionDecimalBuiltInAndNamedAcrossOptInPolicies(t *testing.T) {
	cases := []struct {
		name    string
		element string
		value   string
	}{
		{name: "built-in collapsed scientific", element: "built", value: " \t+001.20e+2\n "},
		{name: "built-in decimal point", element: "built", value: ".5"},
		{name: "built-in trailing point", element: "built", value: "1."},
		{name: "built-in infinity", element: "built", value: "INF"},
		{name: "built-in positive infinity", element: "built", value: "+INF"},
		{name: "built-in negative infinity", element: "built", value: "-INF"},
		{name: "built-in NaN", element: "built", value: "NaN"},
		{name: "built-in negative zero", element: "built", value: "-0.000"},
		{name: "named finite", element: "plain", value: " \t3.0e-2\n "},
		{name: "named retained scale", element: "scale", value: "3.0e2"},
		{name: "named negative zero", element: "zero", value: "-0.00"},
		{name: "named zero scientific scale", element: "zero", value: "0.0e-1"},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationPrecisionDecimalSchema(t, policy)
			before := schema.Components()
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					input := validationPrecisionDecimalInstance(test.element, test.value)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance: %v", err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("precisionDecimal validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit // Keep lexical diagnostics, schema evidence, and immutability assertions together.
func TestValidateInstanceReportsPrecisionDecimalLexicalDiagnosticsWithSchemaEvidence(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationPrecisionDecimalSchema(t, policy)
			before := schema.Components()
			for _, test := range []struct {
				name     string
				element  string
				typeName string
			}{
				{name: "built-in", element: "built", typeName: ""},
				{name: "named", element: "plain", typeName: "Plain"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationPrecisionDecimalInstance(test.element, "1e+")
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != "XSD2010" {
						t.Fatalf("diagnostic = %s/%q, want invalid/XSD2010", diagnostic, diagnostic.Code())
					}
					if diagnostic.Loc() != validationTestTextLoc(t, input) {
						t.Fatalf("Loc() = %s, want precisionDecimal text location", diagnostic.Loc())
					}
					if diagnostic.SpecRef() != "xsd-precisionDecimal#f-precDecLexmap" {
						t.Fatalf("SpecRef() = %q, want precisionDecimal lexical mapping", diagnostic.SpecRef())
					}
					wantRelated := []goxsd9.Loc{validationPrecisionDecimalElementLoc(t, schema, test.element)}
					if test.typeName != "" {
						wantRelated = append(wantRelated, validationPrecisionDecimalTypeLoc(t, schema, test.typeName))
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}
					if diagnostic.Unwrap() != nil {
						t.Fatalf("lexical diagnostic unexpectedly has a cause: %v", diagnostic.Unwrap())
					}
					if errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatal("lexical diagnostic was classified as unsupported")
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("precisionDecimal lexical validation mutated the completed schema")
			}
		})
	}
}

func TestValidateInstanceAppliesPrecisionDecimalPatternDigitsAndScaleFacets(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationPrecisionDecimalSchema(t, policy)

			for _, value := range []string{" \t3.0\n ", "3.0"} {
				input := validationPrecisionDecimalInstance("patterned", value)
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("pattern accepted normalized value %q: %v", value, err)
				}
			}
			patternInput := validationPrecisionDecimalInstance("patterned", "3.00")
			assertPrecisionDecimalInstanceFacetDiagnostic(
				t,
				schema,
				patternInput,
				"patterned",
				"Pattern",
				"pattern",
				"xsd11-datatypes#cvc-pattern-valid",
			)

			for _, value := range []string{"3.00", "3.0e2", "0", "+INF", "-INF", "NaN"} {
				input := validationPrecisionDecimalInstance("digits", value)
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("totalDigits rejected %q: %v", value, err)
				}
			}
			digitsInput := validationPrecisionDecimalInstance("digits", "3.0001")
			assertPrecisionDecimalInstanceFacetDiagnostic(
				t,
				schema,
				digitsInput,
				"digits",
				"Digits",
				"totalDigits",
				"xsd-precisionDecimal#cvc-totalDigits-valid",
			)

			zeroMinInput := validationPrecisionDecimalInstance("zero", "0.0")
			assertPrecisionDecimalInstanceFacetDiagnostic(
				t,
				schema,
				zeroMinInput,
				"zero",
				"Zero",
				"minScale",
				"xsd-precisionDecimal#cvc-minScale-valid",
			)
			zeroMaxInput := validationPrecisionDecimalInstance("zero", "0.000")
			assertPrecisionDecimalInstanceFacetDiagnostic(
				t,
				schema,
				zeroMaxInput,
				"zero",
				"Zero",
				"maxScale",
				"xsd-precisionDecimal#cvc-maxScale-valid",
			)
		})
	}
}

//nolint:gocognit // Keep all bound kinds and enumeration identity cases in one proof table.
func TestValidateInstanceAppliesPrecisionDecimalBoundsAndEnumeration(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationPrecisionDecimalSchema(t, policy)
			for _, test := range []struct {
				name     string
				typeName string
				valid    []string
				invalid  []string
				facet    string
				specRef  string
			}{
				{
					name:     "minInclusive",
					typeName: "MinInclusive",
					valid:    []string{"3", "4", "+INF"},
					invalid:  []string{"2", "NaN"},
					facet:    "minInclusive",
					specRef:  "xsd11-datatypes#cvc-minInclusive-valid",
				},
				{
					name:     "minExclusive",
					typeName: "MinExclusive",
					valid:    []string{"3.1", "+INF"},
					invalid:  []string{"3", "-INF", "NaN"},
					facet:    "minExclusive",
					specRef:  "xsd11-datatypes#cvc-minExclusive-valid",
				},
				{
					name:     "maxInclusive",
					typeName: "MaxInclusive",
					valid:    []string{"3", "2", "-INF"},
					invalid:  []string{"4", "NaN"},
					facet:    "maxInclusive",
					specRef:  "xsd11-datatypes#cvc-maxInclusive-valid",
				},
				{
					name:     "maxExclusive",
					typeName: "MaxExclusive",
					valid:    []string{"2.9", "-INF"},
					invalid:  []string{"3", "+INF", "NaN"},
					facet:    "maxExclusive",
					specRef:  "xsd11-datatypes#cvc-maxExclusive-valid",
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					for _, value := range test.valid {
						input := validationPrecisionDecimalInstance(strings.ToLower(test.name), value)
						if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
							t.Fatalf("bound rejected %q: %v", value, err)
						}
					}
					for _, value := range test.invalid {
						input := validationPrecisionDecimalInstance(strings.ToLower(test.name), value)
						assertPrecisionDecimalInstanceFacetDiagnostic(t, schema, input, strings.ToLower(test.name), test.typeName, test.facet, test.specRef)
					}
				})
			}

			for _, value := range []string{"3.00", "-0.000", "NaN", "INF"} {
				input := validationPrecisionDecimalInstance("enumerated", value)
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("enumeration rejected equivalent or identical value %q: %v", value, err)
				}
			}
			for _, value := range []string{"4", "-INF"} {
				input := validationPrecisionDecimalInstance("enumerated", value)
				assertPrecisionDecimalInstanceFacetDiagnostic(
					t,
					schema,
					input,
					"enumerated",
					"Enumerated",
					"enumeration",
					"xsd11-datatypes#cvc-enumeration-valid",
				)
			}
		})
	}
}

//nolint:gocognit // Keep choice selection, located diagnostics, repeatability, and immutability together.
func TestValidateInstanceSupportsPrecisionDecimalDirectChoiceAlternative(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationChoiceRootNamespace + `" targetNamespace="` + validationChoiceRootNamespace + `">
  <xs:complexType name="Choice"><xs:choice><xs:element name="precision" type="xs:precisionDecimal"/></xs:choice></xs:complexType>
  <xs:element name="choiceRoot" type="r:Choice"/>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, root, nil, policy)
			evidence := validationChoiceEvidenceFor(t, schema)
			before := schema.Components()

			valid := `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><precision xmlns=""> ` + "\t-0.00\n " + `</precision></alias:choiceRoot>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(valid))); err != nil {
				t.Fatalf("ValidateInstance(valid): %v", err)
			}

			invalid := `<alias:choiceRoot xmlns:alias="` + validationChoiceRootNamespace + `"><precision xmlns="">1e+</precision></alias:choiceRoot>`
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(invalid)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != "XSD2010" {
				t.Fatalf("diagnostic = %s/%q, want invalid/XSD2010", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationChoiceElementTextLoc(t, invalid, "precision") {
				t.Fatalf("Loc() = %s, want selected precisionDecimal text location", diagnostic.Loc())
			}
			if diagnostic.SpecRef() != "xsd-precisionDecimal#f-precDecLexmap" {
				t.Fatalf("SpecRef() = %q, want precisionDecimal lexical mapping", diagnostic.SpecRef())
			}
			if !reflect.DeepEqual(diagnostic.Related(), evidence.byChild["precision"]) {
				t.Fatalf("Related() = %v, want %v", diagnostic.Related(), evidence.byChild["precision"])
			}
			if errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatal("direct-choice precisionDecimal lexical failure was classified as unsupported")
			}

			secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(invalid)))
			second := validationTestDiagnostic(t, secondErr)
			if diagnostic.Error() != second.Error() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
				t.Fatalf("repeated direct-choice diagnostics differ: first %v, second %v", err, secondErr)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("direct-choice precisionDecimal validation mutated the completed schema")
			}
		})
	}
}

func validationPrecisionDecimalSchema(t *testing.T, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:t="` + validationPrecisionDecimalNamespace + `" targetNamespace="` + validationPrecisionDecimalNamespace + `" version="1.1">
  <xs:element name="built" type="xs:precisionDecimal"/>
  <xs:element name="plain" type="t:Plain"/>
  <xs:element name="scale" type="t:Scale"/>
  <xs:element name="zero" type="t:Zero"/>
  <xs:element name="patterned" type="t:Pattern"/>
  <xs:element name="digits" type="t:Digits"/>
  <xs:element name="mininclusive" type="t:MinInclusive"/>
  <xs:element name="minexclusive" type="t:MinExclusive"/>
  <xs:element name="maxinclusive" type="t:MaxInclusive"/>
  <xs:element name="maxexclusive" type="t:MaxExclusive"/>
  <xs:element name="enumerated" type="t:Enumerated"/>
  <xs:simpleType name="Plain"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType>
  <xs:simpleType name="Scale"><xs:restriction base="xs:precisionDecimal"><xs:totalDigits value="3"/><xs:minScale value="-1"/><xs:maxScale value="-1"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Zero"><xs:restriction base="xs:precisionDecimal"><xs:totalDigits value="1"/><xs:minScale value="2"/><xs:maxScale value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Pattern"><xs:restriction base="xs:precisionDecimal"><xs:pattern value="3\.0"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Digits"><xs:restriction base="xs:precisionDecimal"><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="MinInclusive"><xs:restriction base="xs:precisionDecimal"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="MinExclusive"><xs:restriction base="xs:precisionDecimal"><xs:minExclusive value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="MaxInclusive"><xs:restriction base="xs:precisionDecimal"><xs:maxInclusive value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="MaxExclusive"><xs:restriction base="xs:precisionDecimal"><xs:maxExclusive value="3"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Enumerated"><xs:restriction base="xs:precisionDecimal"><xs:enumeration value="3.0"/><xs:enumeration value="0.00"/><xs:enumeration value="NaN"/><xs:enumeration value="+INF"/></xs:restriction></xs:simpleType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
}

func validationPrecisionDecimalInstance(element, value string) string {
	return `<` + element + ` xmlns="` + validationPrecisionDecimalNamespace + `">` + value + `</` + element + `>`
}

func validationPrecisionDecimalElementLoc(t *testing.T, schema goxsd9.Schema, name string) goxsd9.Loc {
	t.Helper()
	qualified, err := goxsd9.NewQName(validationPrecisionDecimalNamespace, name)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", name, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, qualified)
	if len(components) != 1 {
		t.Fatalf("%s declarations = %d, want 1", name, len(components))
	}
	return components[0].Loc()
}

func validationPrecisionDecimalTypeLoc(t *testing.T, schema goxsd9.Schema, name string) goxsd9.Loc {
	t.Helper()
	qualified, err := goxsd9.NewQName(validationPrecisionDecimalNamespace, name)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", name, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, qualified)
	if len(components) != 1 {
		t.Fatalf("%s definitions = %d, want 1", name, len(components))
	}
	return components[0].Loc()
}

//nolint:gocognit // Keep the explicit public facet-location mapping in declaration order.
func validationPrecisionDecimalFacetLoc(t *testing.T, schema goxsd9.Schema, typeName, facetName string) goxsd9.Loc {
	t.Helper()
	qualified, err := goxsd9.NewQName(validationPrecisionDecimalNamespace, typeName)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", typeName, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, qualified)
	if len(components) != 1 {
		t.Fatalf("%s definitions = %d, want 1", typeName, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type view", typeName)
	}
	facets := definition.PrecisionDecimalFacets()
	switch facetName {
	case "totalDigits":
		loc, ok := facets.TotalDigitsLoc()
		if ok {
			return loc
		}
	case "minScale":
		loc, ok := facets.MinScaleLoc()
		if ok {
			return loc
		}
	case "maxScale":
		loc, ok := facets.MaxScaleLoc()
		if ok {
			return loc
		}
	case "pattern":
		patterns := facets.PatternDeclarations()
		if len(patterns) == 1 {
			return patterns[0].Loc()
		}
	case "enumeration":
		members := facets.EnumerationDeclarations()
		if len(members) > 0 {
			return members[0].Loc()
		}
	case "minInclusive":
		facet, ok := facets.MinInclusiveFacet()
		if ok {
			return facet.Loc()
		}
	case "minExclusive":
		facet, ok := facets.MinExclusiveFacet()
		if ok {
			return facet.Loc()
		}
	case "maxInclusive":
		facet, ok := facets.MaxInclusiveFacet()
		if ok {
			return facet.Loc()
		}
	case "maxExclusive":
		facet, ok := facets.MaxExclusiveFacet()
		if ok {
			return facet.Loc()
		}
	}
	t.Fatalf("%s facet %q is missing", typeName, facetName)
	return goxsd9.Loc{}
}

func validationPrecisionDecimalFacetLocations(t *testing.T, schema goxsd9.Schema, typeName, facetName string) []goxsd9.Loc {
	t.Helper()
	qualified, err := goxsd9.NewQName(validationPrecisionDecimalNamespace, typeName)
	if err != nil {
		t.Fatalf("NewQName(%s): %v", typeName, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, qualified)
	if len(components) != 1 {
		t.Fatalf("%s definitions = %d, want 1", typeName, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type view", typeName)
	}
	facets := definition.PrecisionDecimalFacets()
	if facetName != "enumeration" {
		return []goxsd9.Loc{validationPrecisionDecimalFacetLoc(t, schema, typeName, facetName)}
	}
	members := facets.EnumerationDeclarations()
	locations := make([]goxsd9.Loc, 0, len(members))
	for _, member := range members {
		locations = append(locations, member.Loc())
	}
	return locations
}

func assertPrecisionDecimalInstanceFacetDiagnostic(t *testing.T, schema goxsd9.Schema, input, element, typeName, facetName, specRef string) {
	t.Helper()
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.PrecisionDecimalFacetValueViolationCode {
		t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), goxsd9.PrecisionDecimalFacetValueViolationCode)
	}
	if diagnostic.Loc() != validationTestTextLoc(t, input) {
		t.Fatalf("Loc() = %s, want precisionDecimal text location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	facetLocations := validationPrecisionDecimalFacetLocations(t, schema, typeName, facetName)
	wantRelated := make([]goxsd9.Loc, 0, 2+len(facetLocations))
	wantRelated = append(wantRelated,
		validationPrecisionDecimalElementLoc(t, schema, element),
		validationPrecisionDecimalTypeLoc(t, schema, typeName),
	)
	wantRelated = append(wantRelated, facetLocations...)
	if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
	}
	if diagnostic.Unwrap() == nil {
		t.Fatal("precisionDecimal facet diagnostic lost its cause")
	}
	if errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatal("precisionDecimal facet failure was classified as unsupported")
	}
}
