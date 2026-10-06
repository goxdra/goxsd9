//nolint:dupl // Distinct datatype enumeration matrices retain exact bounds and SpecRefs.
package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Exercise every enumeration exit at the named-short integrity gate.
func TestGenerateGoNamedShortRejectsStaleEnumerationFacetsAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, consumer := range []string{"standalone", "named element"} {
			for _, failure := range []string{"short range", "effective bounds", "totalDigits"} {
				t.Run(profile.name+"/"+consumer+"/"+failure, func(t *testing.T) {
					element := ""
					if consumer == "named element" {
						element = `<xs:element name="value" type="t:Value"/>`
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + element + `<xs:simpleType name="Value"><xs:restriction base="xs:short"><xs:totalDigits value="2"/><xs:maxInclusive value="20"/><xs:enumeration value="12"/></xs:restriction></xs:simpleType><xs:simpleType name="Digits"><xs:restriction base="xs:short"><xs:totalDigits value="1"/></xs:restriction></xs:simpleType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
					}
					components := schema.Components()
					valueIndex := 0
					if consumer == "named element" {
						valueIndex = 1
					}
					value := components[valueIndex]
					digits := components[valueIndex+1]
					valueFacts, ok := value.simpleType.facets.(schemaIntegerFacetVariant)
					if !ok {
						t.Fatalf("Value facets = %T, want integer facets", value.simpleType.facets)
					}
					enumerationLoc := valueFacts.enumeration.Locations()[0]
					valueDefinition, hasDefinition := value.SimpleTypeDefinition()
					if !hasDefinition {
						t.Fatal("Value has no simple type view")
					}
					baseLoc := valueDefinition.BaseLoc()
					var extraRelated Loc
					switch failure {
					case "short range":
						outside, parseErr := ParseStrictInteger("32768", Loc{})
						if parseErr != nil {
							t.Fatal(parseErr)
						}
						valueFacts.enumeration.values[0].value = outside
					case "effective bounds":
						minimum, minErr := ParseIntegerMinInclusiveFacet(codegenShortMinimum, Loc{}, profile.version)
						if minErr != nil {
							t.Fatal(minErr)
						}
						boundLoc, hasBoundLoc := valueFacts.bounds.MaxInclusiveLoc()
						if !hasBoundLoc {
							t.Fatal("Value has no maxInclusive location")
						}
						maximum, maxErr := ParseIntegerMaxInclusiveFacet("9", boundLoc, profile.version)
						if maxErr != nil {
							t.Fatal(maxErr)
						}
						bounds, boundsErr := NewIntegerBoundFacets([]IntegerBoundFacet{minimum, maximum}, profile.version)
						if boundsErr != nil {
							t.Fatal(boundsErr)
						}
						valueFacts.bounds = bounds
						extraRelated = boundLoc
					case "totalDigits":
						digitFacts, digitOK := digits.simpleType.facets.(schemaIntegerFacetVariant)
						if !digitOK {
							t.Fatalf("Digits facets = %T, want integer facets", digits.simpleType.facets)
						}
						valueFacts.digits = digitFacts.digits
						var hasDigitLoc bool
						extraRelated, hasDigitLoc = digitFacts.digits.TotalDigitsLoc()
						if !hasDigitLoc {
							t.Fatal("Digits has no totalDigits location")
						}
					default:
						t.Fatalf("unknown failure %q", failure)
					}
					value.simpleType.facets = valueFacts
					output, generationErr := GenerateGo(schema, "generated")
					if output != nil || generationErr == nil {
						t.Fatalf("GenerateGo = (%q, %v), want nil output and error", output, generationErr)
					}
					diagnostic := requireDiagnostic(t, generationErr)
					primary := value.Loc()
					if consumer == "named element" {
						primary = components[0].Loc()
					}
					if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != primary || diagnostic.SpecRef() != codegenShortSpecRef(profile.version) || !errors.Is(generationErr, errCodegenSchemaInvariant) {
						t.Fatalf("diagnostic = %s, want GOXSD9030 at %s with short SpecRef and cause", diagnostic, primary)
					}
					digitLoc, hasDigitLoc := valueFacts.digits.TotalDigitsLoc()
					if !hasDigitLoc {
						t.Fatal("Value has no effective totalDigits location")
					}
					wantRelated := []Loc{baseLoc, digitLoc, enumerationLoc}
					if consumer == "named element" {
						wantRelated = append([]Loc{value.Loc()}, wantRelated...)
					}
					if failure == "effective bounds" {
						wantRelated = append(wantRelated, extraRelated)
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("related = %v, want %v", diagnostic.Related(), wantRelated)
					}
					if failure == "totalDigits" && (!errors.Is(generationErr, errDigitFacetValueViolation) || !strings.Contains(diagnostic.Message(), "digit facets")) {
						t.Fatalf("totalDigits violation lost cause/message: %s", diagnostic)
					}
					if failure == "effective bounds" && !errors.Is(generationErr, errBoundValueViolation) {
						t.Fatalf("bound violation lost cause: %s", diagnostic)
					}
				})
			}
		}
	}
}
