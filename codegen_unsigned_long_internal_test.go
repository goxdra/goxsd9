package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func codegenUnsignedLongSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, codegenUnsignedLongKind())
}

func TestCodegenUnsignedLongRejectsStaleAndMalformedFactsAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsStaleAndMalformedFacts(t, "unsignedLong", codegenUnsignedLongMinimum, codegenUnsignedLongMaximum, "-1", codegenUnsignedLongSpecRef)
}

//nolint:gocognit // Keep alternate reference exits and their diagnostic invariants together.
func TestGenerateGoUnsignedLongReferenceIntegrityAlternateExits(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, body string
			mutate     func(*testing.T, Schema)
		}{
			{
				name: "built-in atomic kind",
				body: `<xs:element name="value" type="xs:unsignedLong"/>`,
				mutate: func(_ *testing.T, schema Schema) {
					schema.Components()[0].element.typeReference.atomicKind = schemaSimpleTypeAtomicInt
				},
			},
			{
				name: "built-in reference name",
				body: `<xs:element name="value" type="xs:unsignedLong"/>`,
				mutate: func(_ *testing.T, schema Schema) {
					schema.Components()[0].element.typeReference.name = QName{namespace: xsdNamespaceURI, local: "int"}
				},
			},
			{
				name: "built-in facet variant",
				body: `<xs:element name="value" type="xs:unsignedLong"/>`,
				mutate: func(_ *testing.T, schema Schema) {
					schema.Components()[0].element.typeReference.facets = schemaBooleanFacetVariant{}
				},
			},
			{
				name: "named reference atomic kind",
				body: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`,
				mutate: func(_ *testing.T, schema Schema) {
					schema.Components()[0].element.typeReference.atomicKind = schemaSimpleTypeAtomicInt
				},
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				primary := schema.Components()[0].Loc()
				test.mutate(t, schema)
				output, generationErr := GenerateGo(schema, "generated")
				if output != nil || generationErr == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil output and error", output, generationErr)
				}
				diagnostic := requireDiagnostic(t, generationErr)
				if test.name == "built-in reference name" {
					if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != primary || diagnostic.SpecRef() != "" || !errors.Is(generationErr, errCodegenElementType) || len(diagnostic.Related()) != 0 {
						t.Fatalf("generic reference identity diagnostic = %s, want GOXSD9030 at %s with preserved identity cause", diagnostic, primary)
					}
					return
				}
				if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != primary || diagnostic.SpecRef() != codegenUnsignedLongSpecRef(profile.version) || !errors.Is(generationErr, errCodegenSchemaInvariant) {
					t.Fatalf("diagnostic = %s, want unsignedLong GOXSD9030 at %s with cause", diagnostic, primary)
				}
				if test.name == "named reference atomic kind" {
					if !reflect.DeepEqual(diagnostic.Related(), []Loc{schema.Components()[1].Loc()}) {
						t.Fatalf("related = %v, want named definition", diagnostic.Related())
					}
					return
				}
				if len(diagnostic.Related()) != 0 {
					t.Fatalf("related = %v, want none", diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Every effective enumeration exit checks the same edition-selected diagnostic boundary.
func TestGenerateGoNamedUnsignedLongRejectsStaleEnumerationFacetsAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, consumer := range []string{"standalone", "named element"} {
			for _, failure := range []string{"above intrinsic maximum", "below intrinsic minimum", "effective bounds", "totalDigits"} {
				t.Run(profile.name+"/"+consumer+"/"+failure, func(t *testing.T) {
					element := ""
					if consumer == "named element" {
						element = `<xs:element name="value" type="t:Value"/>`
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + element + `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"><xs:totalDigits value="20"/><xs:maxInclusive value="18446744073709551615"/><xs:enumeration value="9223372036854775808"/></xs:restriction></xs:simpleType><xs:simpleType name="Digits"><xs:restriction base="xs:unsignedLong"><xs:totalDigits value="1"/></xs:restriction></xs:simpleType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
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
						t.Fatalf("Value facets = %T", value.simpleType.facets)
					}
					enumerationLoc := valueFacts.enumeration.Locations()[0]
					valueDefinition, ok := value.SimpleTypeDefinition()
					if !ok {
						t.Fatal("Value definition is missing")
					}
					baseLoc := valueDefinition.BaseLoc()
					var extraRelated Loc
					switch failure {
					case "above intrinsic maximum", "below intrinsic minimum":
						outside := "18446744073709551616"
						if failure == "below intrinsic minimum" {
							outside = "-1"
						}
						integer, parseErr := ParseStrictInteger(outside, Loc{})
						if parseErr != nil {
							t.Fatal(parseErr)
						}
						valueFacts.enumeration.values[0].value = integer
					case "effective bounds":
						minimum, minErr := ParseIntegerMinInclusiveFacet("0", Loc{}, profile.version)
						if minErr != nil {
							t.Fatal(minErr)
						}
						boundLoc, hasBoundLoc := valueFacts.bounds.MaxInclusiveLoc()
						if !hasBoundLoc {
							t.Fatal("Value maximum location missing")
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
						digitFacts, ok := digits.simpleType.facets.(schemaIntegerFacetVariant)
						if !ok {
							t.Fatalf("Digits facets = %T", digits.simpleType.facets)
						}
						valueFacts.digits = digitFacts.digits
						var hasDigitLoc bool
						extraRelated, hasDigitLoc = digitFacts.digits.TotalDigitsLoc()
						if !hasDigitLoc {
							t.Fatal("Digits location missing")
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
					if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != primary || diagnostic.SpecRef() != codegenUnsignedLongSpecRef(profile.version) || !errors.Is(generationErr, errCodegenSchemaInvariant) {
						t.Fatalf("diagnostic = %s, want GOXSD9030 at %s with unsignedLong SpecRef and cause", diagnostic, primary)
					}
					digitLoc, hasDigitLoc := valueFacts.digits.TotalDigitsLoc()
					if !hasDigitLoc {
						t.Fatal("Value digit location missing")
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
					if failure == "effective bounds" && !errors.Is(generationErr, errBoundValueViolation) {
						t.Fatalf("effective bound violation lost cause: %s", diagnostic)
					}
					if failure == "totalDigits" && (!errors.Is(generationErr, errDigitFacetValueViolation) || !strings.Contains(diagnostic.Message(), "digit facets")) {
						t.Fatalf("digit violation lost cause/message: %s", diagnostic)
					}
				})
			}
		}
	}
}
