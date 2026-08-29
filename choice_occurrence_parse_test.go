package goxsd9_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const choiceOccurrenceXSDNamespace = "http://www.w3.org/2001/XMLSchema"

const (
	choiceOccurrenceHugeMinimum = "18446744073709551615"
	choiceOccurrenceHugeMaximum = "18446744073709551616"
)

//nolint:gocognit // Keep the exact public direct-choice occurrence table together.
func TestParseSchemaExposesExactDirectChoiceOccurrences(t *testing.T) {
	tests := []struct {
		name        string
		choiceAttrs string
		firstAttrs  string
		secondAttrs string
		wantChoice  string
		wantFirst   string
		wantSecond  string
	}{
		{
			name:       "omitted defaults",
			wantChoice: "1/1",
			wantFirst:  "1/1",
			wantSecond: "1/1",
		},
		{
			name:        "zero minimum choice",
			choiceAttrs: ` minOccurs="0"`,
			wantChoice:  "0/1",
			wantFirst:   "1/1",
			wantSecond:  "1/1",
		},
		{
			name:        "finite choice above uint64",
			choiceAttrs: ` minOccurs="` + choiceOccurrenceHugeMinimum + `" maxOccurs="` + choiceOccurrenceHugeMaximum + `"`,
			wantChoice:  choiceOccurrenceHugeMinimum + "/" + choiceOccurrenceHugeMaximum,
			wantFirst:   "1/1",
			wantSecond:  "1/1",
		},
		{
			name:        "unbounded choice",
			choiceAttrs: ` minOccurs="` + choiceOccurrenceHugeMaximum + `" maxOccurs="unbounded"`,
			wantChoice:  choiceOccurrenceHugeMaximum + "/unbounded",
			wantFirst:   "1/1",
			wantSecond:  "1/1",
		},
		{
			name:       "zero minimum alternative",
			firstAttrs: ` minOccurs="0"`,
			wantChoice: "1/1",
			wantFirst:  "0/1",
			wantSecond: "1/1",
		},
		{
			name:       "finite alternative above uint64",
			firstAttrs: ` minOccurs="` + choiceOccurrenceHugeMinimum + `" maxOccurs="` + choiceOccurrenceHugeMaximum + `"`,
			wantChoice: "1/1",
			wantFirst:  choiceOccurrenceHugeMinimum + "/" + choiceOccurrenceHugeMaximum,
			wantSecond: "1/1",
		},
		{
			name:       "unbounded alternative",
			firstAttrs: ` minOccurs="` + choiceOccurrenceHugeMaximum + `" maxOccurs="unbounded"`,
			wantChoice: "1/1",
			wantFirst:  choiceOccurrenceHugeMaximum + "/unbounded",
			wantSecond: "1/1",
		},
	}

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema := parseChoiceOccurrenceSchema(t, policy, choiceOccurrenceDocument(test.choiceAttrs, test.firstAttrs, test.secondAttrs))
				choice := elementNamespaceChoice(t, schema, "urn:choice", "Choice")
				assertChoiceOccurrenceRange(t, choice, test.wantChoice)
				if got := choice.Loc(); got != choiceOccurrenceLoc(t, 3, 5) {
					t.Fatalf("choice location = %s, want root.xsd:3:5", got)
				}
				alternatives := choice.Alternatives()
				if len(alternatives) != 2 {
					t.Fatalf("alternative count = %d, want 2", len(alternatives))
				}
				assertChoiceOccurrenceRange(t, elementNamespaceAlternative(t, alternatives, 0), test.wantFirst)
				assertChoiceOccurrenceRange(t, elementNamespaceAlternative(t, alternatives, 1), test.wantSecond)
				if got := alternatives[0].Loc(); got != choiceOccurrenceLoc(t, 4, 7) {
					t.Fatalf("first alternative location = %s, want root.xsd:4:7", got)
				}
				if got := alternatives[1].Loc(); got != choiceOccurrenceLoc(t, 5, 7) {
					t.Fatalf("second alternative location = %s, want root.xsd:5:7", got)
				}
			})
		}
	}
}

//nolint:gocognit // Keep choice and alternative absence checks together.
func TestParseSchemaMapsDirectChoiceZeroZeroToAbsenceAndPreservesAlternativeOrder(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy)+"/choice", func(t *testing.T) {
			root := choiceOccurrenceDocument(` minOccurs="0" maxOccurs="0"`, "", "")
			schema := parseChoiceOccurrenceSchema(t, policy, root)
			definition := choiceOccurrenceDefinition(t, schema)
			if particle := definition.Particle(); particle != nil {
				t.Fatalf("zero-zero choice particle = %T, want absence", particle)
			}
		})

		t.Run(string(policy)+"/alternative", func(t *testing.T) {
			root := choiceOccurrenceDocument(
				"",
				` minOccurs="0" maxOccurs="0"`,
				` minOccurs="2" maxOccurs="unbounded"`,
			)
			schema := parseChoiceOccurrenceSchema(t, policy, root)
			choice := elementNamespaceChoice(t, schema, "urn:choice", "Choice")
			alternatives := choice.Alternatives()
			if len(alternatives) != 1 {
				t.Fatalf("retained alternative count = %d, want 1", len(alternatives))
			}
			alternative := elementNamespaceAlternative(t, alternatives, 0)
			if got := alternative.Name().Local(); got != "second" {
				t.Fatalf("retained alternative name = %q, want second", got)
			}
			if got := alternative.Occurrences().String(); got != "2/unbounded" {
				t.Fatalf("retained alternative occurrences = %q, want 2/unbounded", got)
			}
			if got := alternative.Loc(); got != choiceOccurrenceLoc(t, 5, 7) {
				t.Fatalf("retained alternative location = %s, want root.xsd:5:7", got)
			}
			owned := choice.Alternatives()
			owned[0] = nil
			if got := elementNamespaceAlternative(t, choice.Alternatives(), 0).Name().Local(); got != "second" {
				t.Fatalf("mutating alternatives changed retained order to %q", got)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep occurrence failure classes and evidence in one table.
func TestParseSchemaRejectsDirectChoiceOccurrenceErrorsWithLocatedCauses(t *testing.T) {
	tests := []struct {
		name             string
		choiceAttrs      string
		firstAttrs       string
		primaryKind      string
		primaryAttribute string
		primaryOrdinal   int
		related          []choiceOccurrenceAttribute
		wantMessage      string
		wantCode         string
		wantSpecRef      string
		wantLexicalCause bool
		wantNegative     bool
	}{
		{
			name:             "malformed choice minimum",
			choiceAttrs:      ` minOccurs="maybe"`,
			primaryKind:      "choice",
			primaryAttribute: "minOccurs",
			wantMessage:      `attribute "minOccurs" has an invalid occurrence value`,
			wantSpecRef:      "datatypes#nonNegativeInteger",
			wantLexicalCause: true,
		},
		{
			name:             "negative choice maximum",
			choiceAttrs:      ` maxOccurs="-1"`,
			primaryKind:      "choice",
			primaryAttribute: "maxOccurs",
			wantMessage:      `attribute "maxOccurs" has an invalid occurrence value`,
			wantSpecRef:      "datatypes#nonNegativeInteger",
			wantNegative:     true,
		},
		{
			name:             "unbounded choice minimum",
			choiceAttrs:      ` minOccurs="unbounded"`,
			primaryKind:      "choice",
			primaryAttribute: "minOccurs",
			wantMessage:      `attribute "minOccurs" has an invalid occurrence value`,
			wantSpecRef:      "structures#p-min_occurs",
			wantLexicalCause: true,
		},
		{
			name:             "duplicate choice minimum",
			choiceAttrs:      ` minOccurs="0" minOccurs="1"`,
			primaryKind:      "choice",
			primaryAttribute: "minOccurs",
			primaryOrdinal:   1,
			wantMessage:      `duplicate attribute "minOccurs"`,
			wantCode:         "XSD3001",
		},
		{
			name:        "choice minimum exceeds maximum",
			choiceAttrs: ` minOccurs="2" maxOccurs="1"`,
			primaryKind: "choice",
			related: []choiceOccurrenceAttribute{
				{name: "minOccurs"},
				{name: "maxOccurs"},
			},
			wantMessage: "particle minOccurs cannot exceed maxOccurs",
			wantSpecRef: "structures#coss-particle",
		},
		{
			name:        "omitted choice minimum exceeds zero maximum",
			choiceAttrs: ` maxOccurs="0"`,
			primaryKind: "choice",
			related: []choiceOccurrenceAttribute{
				{name: "maxOccurs"},
			},
			wantMessage: "particle minOccurs cannot exceed maxOccurs",
			wantSpecRef: "structures#coss-particle",
		},
		{
			name:             "malformed alternative maximum",
			firstAttrs:       ` maxOccurs="1.0"`,
			primaryKind:      "alternative",
			primaryAttribute: "maxOccurs",
			wantMessage:      `attribute "maxOccurs" has an invalid occurrence value`,
			wantSpecRef:      "datatypes#nonNegativeInteger",
			wantLexicalCause: true,
		},
		{
			name:             "negative alternative minimum",
			firstAttrs:       ` minOccurs="-1"`,
			primaryKind:      "alternative",
			primaryAttribute: "minOccurs",
			wantMessage:      `attribute "minOccurs" has an invalid occurrence value`,
			wantSpecRef:      "datatypes#nonNegativeInteger",
			wantNegative:     true,
		},
		{
			name:             "unbounded alternative minimum",
			firstAttrs:       ` minOccurs="unbounded"`,
			primaryKind:      "alternative",
			primaryAttribute: "minOccurs",
			wantMessage:      `attribute "minOccurs" has an invalid occurrence value`,
			wantSpecRef:      "structures#p-min_occurs",
			wantLexicalCause: true,
		},
		{
			name:             "duplicate alternative maximum",
			firstAttrs:       ` maxOccurs="1" maxOccurs="2"`,
			primaryKind:      "alternative",
			primaryAttribute: "maxOccurs",
			primaryOrdinal:   1,
			wantMessage:      `duplicate attribute "maxOccurs"`,
			wantCode:         "XSD3001",
		},
		{
			name:        "alternative minimum exceeds maximum",
			firstAttrs:  ` minOccurs="2" maxOccurs="1"`,
			primaryKind: "alternative",
			related: []choiceOccurrenceAttribute{
				{name: "minOccurs"},
				{name: "maxOccurs"},
			},
			wantMessage: "particle minOccurs cannot exceed maxOccurs",
			wantSpecRef: "structures#coss-particle",
		},
		{
			name:        "omitted alternative minimum exceeds zero maximum",
			firstAttrs:  ` maxOccurs="0"`,
			primaryKind: "alternative",
			related: []choiceOccurrenceAttribute{
				{name: "maxOccurs"},
			},
			wantMessage: "particle minOccurs cannot exceed maxOccurs",
			wantSpecRef: "structures#coss-particle",
		},
	}

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := choiceOccurrenceDocument(test.choiceAttrs, test.firstAttrs, "")
				schema, err := parseChoiceOccurrenceSchemaResult(t, policy, root)
				if err == nil {
					t.Fatal("ParseSchemaWithPolicy accepted invalid occurrence input")
				}
				if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("ParseSchemaWithPolicy returned a partial schema")
				}
				var diagnostic goxsd9.Diagnostic
				if !errors.As(err, &diagnostic) {
					t.Fatalf("error = %v, want located diagnostic", err)
				}
				wantCode := test.wantCode
				if wantCode == "" {
					wantCode = "XSD3010"
				}
				if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != wantCode {
					t.Fatalf("diagnostic = (%q,%q), want invalid %s", diagnostic.Class(), diagnostic.Code(), wantCode)
				}
				if diagnostic.Message() != test.wantMessage {
					t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantMessage)
				}
				if test.wantSpecRef != "" {
					wantSpecRef := "xsd11-" + test.wantSpecRef
					if policy == goxsd9.Strict10 {
						wantSpecRef = "xsd10-" + test.wantSpecRef
					}
					if diagnostic.SpecRef() != wantSpecRef {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpecRef)
					}
				}
				wantPrimary := choiceOccurrenceLoc(t, 3, 5)
				if test.primaryKind == "alternative" {
					wantPrimary = choiceOccurrenceLoc(t, 4, 7)
				}
				if test.primaryAttribute != "" {
					wantPrimary = choiceOccurrenceAttributeLoc(t, root, test.primaryAttribute, test.primaryOrdinal)
				}
				if diagnostic.Loc() != wantPrimary {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantPrimary)
				}
				if len(diagnostic.Related()) != len(test.related) {
					t.Fatalf("related locations = %v, want %d locations", diagnostic.Related(), len(test.related))
				}
				for index, related := range test.related {
					want := choiceOccurrenceAttributeLoc(t, root, related.name, related.ordinal)
					if diagnostic.Related()[index] != want {
						t.Fatalf("related location %d = %s, want %s", index, diagnostic.Related()[index], want)
					}
				}
				if test.wantLexicalCause {
					var cause goxsd9.Diagnostic
					if !errors.As(diagnostic.Unwrap(), &cause) || cause.Code() != goxsd9.InvalidIntegerLexicalCode {
						t.Fatalf("diagnostic cause = %v, want invalid integer lexical cause", diagnostic.Unwrap())
					}
				}
				if test.wantNegative {
					if diagnostic.Unwrap() == nil || !strings.Contains(diagnostic.Unwrap().Error(), "negative") {
						t.Fatalf("diagnostic cause = %v, want negative occurrence cause", diagnostic.Unwrap())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Keep parser, validator, and generator boundary checks together.
func TestDirectChoiceOccurrenceConsumersRemainExplicitlyUnsupported(t *testing.T) {
	tests := []struct {
		name             string
		choiceAttrs      string
		alternativeAttrs string
		wantAlternative  bool
	}{
		{
			name:             "choice range",
			choiceAttrs:      ` minOccurs="0"`,
			alternativeAttrs: ` minOccurs="0"`,
		},
		{
			name:             "alternative range",
			alternativeAttrs: ` minOccurs="0"`,
			wantAlternative:  true,
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					root := choiceOccurrenceDocumentWithRoot(test.choiceAttrs, test.alternativeAttrs)
					schema := parseChoiceOccurrenceSchema(t, policy, root)
					choice := elementNamespaceChoice(t, schema, "urn:choice", "Choice")
					alternative := elementNamespaceAlternative(t, choice.Alternatives(), 0)
					if test.choiceAttrs != "" && choice.Occurrences().IsDefault() {
						t.Fatal("choice occurrence fixture unexpectedly has default bounds")
					}
					if test.alternativeAttrs != "" && alternative.Occurrences().IsDefault() {
						t.Fatal("alternative occurrence fixture unexpectedly has default bounds")
					}
					wantLoc := choice.Loc()
					if test.wantAlternative {
						wantLoc = alternative.Loc()
					}
					assertChoiceOccurrenceConsumersUnsupported(t, schema, wantLoc)
				})
			}
		})
	}
}

func assertChoiceOccurrenceConsumersUnsupported(t *testing.T, schema goxsd9.Schema, wantLoc goxsd9.Loc) {
	t.Helper()
	instanceErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:choice"><first>1</first></root>`)))
	instanceDiagnostic := choiceOccurrenceDiagnostic(t, instanceErr)
	if instanceDiagnostic.Class() != goxsd9.FailureUnsupported || instanceDiagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode {
		t.Fatalf("instance diagnostic = (%q,%q), want unsupported/%s", instanceDiagnostic.Class(), instanceDiagnostic.Code(), goxsd9.UnsupportedInstanceValidationCode)
	}
	if instanceDiagnostic.Loc() != wantLoc {
		t.Fatalf("instance diagnostic location = %s, want %s", instanceDiagnostic.Loc(), wantLoc)
	}

	_, generateErr := goxsd9.GenerateGo(schema, "generated")
	generateDiagnostic := choiceOccurrenceDiagnostic(t, generateErr)
	if generateDiagnostic.Class() != goxsd9.FailureUnsupported || generateDiagnostic.Feature() != goxsd9.FeatureCodegen {
		t.Fatalf("generation diagnostic = (%q,%q), want codegen unsupported", generateDiagnostic.Class(), generateDiagnostic.Feature())
	}
	if generateDiagnostic.Loc() != wantLoc {
		t.Fatalf("generation diagnostic location = %s, want %s", generateDiagnostic.Loc(), wantLoc)
	}
}

//nolint:gocognit // Keep precisionDecimal occurrence boundary cases together.
func TestParseSchemaDoesNotOptIntoPrecisionDecimalChoiceOccurrences(t *testing.T) {
	tests := []struct {
		name             string
		declaredType     string
		choiceAttrs      string
		alternativeAttrs string
		wantLoc          goxsd9.Loc
		wantMessage      string
	}{
		{
			name:         "choice range",
			declaredType: "xs:precisionDecimal",
			choiceAttrs:  ` minOccurs="0"`,
			wantLoc:      choiceOccurrenceLoc(t, 4, 5),
			wantMessage:  "direct choice occurrence ranges with precisionDecimal alternatives are not implemented",
		},
		{
			name:             "alternative range",
			declaredType:     "c:Prec",
			alternativeAttrs: ` minOccurs="0"`,
			wantLoc:          choiceOccurrenceLoc(t, 5, 7),
			wantMessage:      "precisionDecimal choice alternative occurrence ranges are not implemented",
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := precisionDecimalChoiceOccurrenceDocument(test.choiceAttrs, test.alternativeAttrs, test.declaredType)
				schema, err := parseChoiceOccurrenceSchemaResult(t, policy, root)
				if err == nil {
					t.Fatal("ParseSchemaWithPolicy accepted a non-default precisionDecimal occurrence")
				}
				if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("ParseSchemaWithPolicy returned a partial schema")
				}
				diagnostic := choiceOccurrenceDiagnostic(t, err)
				if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode {
					t.Fatalf("diagnostic = (%q,%q,%q), want schema-syntax unsupported", diagnostic.Class(), diagnostic.Feature(), diagnostic.Code())
				}
				if diagnostic.Message() != test.wantMessage {
					t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantMessage)
				}
				if diagnostic.Loc() != test.wantLoc {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), test.wantLoc)
				}
				if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
					t.Fatalf("diagnostic spec ref = %q, want xsd11-structures#cSchemaDocument", diagnostic.SpecRef())
				}
				if !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
				}
			})
		}
	}
}

type choiceOccurrenceAttribute struct {
	name    string
	ordinal int
}

func choiceOccurrenceDocument(choiceAttrs, firstAttrs, secondAttrs string) string {
	return `<xs:schema xmlns:xs="` + choiceOccurrenceXSDNamespace + `" targetNamespace="urn:choice">
  <xs:complexType name="Choice">
    <xs:choice` + choiceAttrs + `>
      <xs:element name="first" type="xs:integer"` + firstAttrs + `/>
      <xs:element name="second" type="xs:decimal"` + secondAttrs + `/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
}

func choiceOccurrenceDocumentWithRoot(choiceAttrs, firstAttrs string) string {
	return `<xs:schema xmlns:xs="` + choiceOccurrenceXSDNamespace + `" xmlns:c="urn:choice" targetNamespace="urn:choice">
  <xs:element name="root" type="c:Choice"/>
  <xs:complexType name="Choice">
    <xs:choice` + choiceAttrs + `>
      <xs:element name="first" type="xs:integer"` + firstAttrs + `/>
      <xs:element name="second" type="xs:decimal"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
}

func precisionDecimalChoiceOccurrenceDocument(choiceAttrs, alternativeAttrs, declaredType string) string {
	return `<xs:schema xmlns:xs="` + choiceOccurrenceXSDNamespace + `" xmlns:c="urn:choice" targetNamespace="urn:choice">
  <xs:simpleType name="Prec"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType>
  <xs:complexType name="Choice">
    <xs:choice` + choiceAttrs + `>
      <xs:element name="precision" type="` + declaredType + `"` + alternativeAttrs + `/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
}

func parseChoiceOccurrenceSchema(t *testing.T, policy goxsd9.LanguagePolicy, root string) goxsd9.Schema {
	t.Helper()
	schema, err := parseChoiceOccurrenceSchemaResult(t, policy, root)
	if err != nil {
		t.Fatalf("ParseSchemaWithPolicy: %v", err)
	}
	return schema
}

func parseChoiceOccurrenceSchemaResult(t *testing.T, policy goxsd9.LanguagePolicy, root string) (goxsd9.Schema, error) {
	t.Helper()
	return parseElementNamespaceSchemaResult(t, policy, root, nil)
}

func choiceOccurrenceDefinition(t *testing.T, schema goxsd9.Schema) goxsd9.ComplexTypeDefinition {
	t.Helper()
	name, err := goxsd9.NewQName("urn:choice", "Choice")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("Choice component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice has no complex type definition")
	}
	return definition
}

func assertChoiceOccurrenceRange(t *testing.T, particle goxsd9.Particle, want string) {
	t.Helper()
	occurrences := particle.Occurrences()
	if got := occurrences.String(); got != want {
		t.Fatalf("occurrences = %q, want %q", got, want)
	}
	parts := strings.SplitN(want, "/", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid test occurrence range %q", want)
	}
	if got := occurrences.Minimum().Canonical(); got != parts[0] {
		t.Fatalf("minimum = %q, want %q", got, parts[0])
	}
	maximum := occurrences.Maximum()
	if parts[1] == "unbounded" {
		if !maximum.IsUnbounded() || maximum.Kind() != goxsd9.ParticleOccurrenceMaximumUnbounded {
			t.Fatalf("maximum = %s (%v), want unbounded", maximum, maximum.Kind())
		}
		if _, ok := maximum.Finite(); ok {
			t.Fatal("unbounded maximum returned a finite value")
		}
		return
	}
	if maximum.IsUnbounded() || maximum.Kind() != goxsd9.ParticleOccurrenceMaximumFinite {
		t.Fatalf("maximum = %s (%v), want finite", maximum, maximum.Kind())
	}
	finite, ok := maximum.Finite()
	if !ok || finite.Canonical() != parts[1] {
		t.Fatalf("finite maximum = (%q,%t), want %q", finite.Canonical(), ok, parts[1])
	}
}

func choiceOccurrenceLoc(t *testing.T, line, column int) goxsd9.Loc {
	t.Helper()
	loc, err := goxsd9.NewLoc("root.xsd", line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func choiceOccurrenceAttributeLoc(t *testing.T, root, name string, ordinal int) goxsd9.Loc {
	t.Helper()
	if ordinal < 0 {
		t.Fatalf("attribute ordinal = %d, want non-negative", ordinal)
	}
	searchFrom := 0
	offset := -1
	for index := 0; index <= ordinal; index++ {
		found := strings.Index(root[searchFrom:], name+"=")
		if found < 0 {
			t.Fatalf("root has fewer than %d %s attributes", ordinal+1, name)
		}
		offset = searchFrom + found
		searchFrom = offset + len(name)
	}
	line := 1 + strings.Count(root[:offset], "\n")
	lineStart := strings.LastIndex(root[:offset], "\n") + 1
	return choiceOccurrenceLoc(t, line, offset-lineStart+1)
}

func choiceOccurrenceDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("operation unexpectedly succeeded")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v, want diagnostic", err)
	}
	return diagnostic
}
