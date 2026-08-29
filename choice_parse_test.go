package goxsd9_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const choiceOccurrenceTestXSDNamespace = "http://www.w3.org/2001/XMLSchema"

type choiceOccurrenceAlternative struct {
	name        string
	occurrences string
}

const (
	choiceOccurrenceDatatypeSpecSuffix = "datatypes#nonNegativeInteger"
	choiceOccurrenceMinimumSpecSuffix  = "structures#p-min_occurs"
	choiceOccurrenceRangeSpecSuffix    = "structures#coss-particle"
)

//nolint:gocognit,funlen // Keep the direct-choice occurrence contract together.
func TestParseSchemaExposesExactOrderedChoiceParticles(t *testing.T) {
	tests := []struct {
		name             string
		choiceAttrs      string
		firstAttrs       string
		secondAttrs      string
		thirdAttrs       string
		wantChoice       string
		wantParticle     bool
		wantAlternatives []choiceOccurrenceAlternative
	}{
		{
			name:         "omitted defaults",
			wantChoice:   "1/1",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "1/1"},
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "1/1"},
			},
		},
		{
			name:         "zero minimum",
			choiceAttrs:  ` minOccurs="0"`,
			wantChoice:   "0/1",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "1/1"},
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "1/1"},
			},
		},
		{
			name:         "omitted minimum with unbounded maximum",
			choiceAttrs:  ` maxOccurs="unbounded"`,
			wantChoice:   "1/unbounded",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "1/1"},
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "1/1"},
			},
		},
		{
			name:         "finite above uint64",
			choiceAttrs:  ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`,
			wantChoice:   "18446744073709551615/18446744073709551616",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "1/1"},
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "1/1"},
			},
		},
		{
			name:         "zero zero choice absence",
			choiceAttrs:  ` minOccurs="0" maxOccurs="0"`,
			wantParticle: false,
		},
		{
			name:         "optional first alternative",
			firstAttrs:   ` minOccurs="0"`,
			wantChoice:   "1/1",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "0/1"},
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "1/1"},
			},
		},
		{
			name:         "finite alternative above uint64",
			secondAttrs:  ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`,
			wantChoice:   "1/1",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "1/1"},
				{name: "second", occurrences: "18446744073709551615/18446744073709551616"},
				{name: "third", occurrences: "1/1"},
			},
		},
		{
			name:         "unbounded alternative maximum",
			thirdAttrs:   ` minOccurs="18446744073709551616" maxOccurs="unbounded"`,
			wantChoice:   "1/1",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "first", occurrences: "1/1"},
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "18446744073709551616/unbounded"},
			},
		},
		{
			name:         "zero zero alternative absence preserves lexical order",
			firstAttrs:   ` minOccurs="0" maxOccurs="0"`,
			wantChoice:   "1/1",
			wantParticle: true,
			wantAlternatives: []choiceOccurrenceAlternative{
				{name: "second", occurrences: "1/1"},
				{name: "third", occurrences: "1/1"},
			},
		},
	}

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema := parseChoiceOccurrenceSchema(t, policy, choiceOccurrenceTestDocument(test.choiceAttrs, test.firstAttrs, test.secondAttrs, test.thirdAttrs))
				before := schema.Components()
				definition := choiceOccurrenceTestDefinition(t, schema)
				particle := definition.Particle()
				if !test.wantParticle {
					if particle != nil {
						t.Fatalf("particle = %T, want occurrence absence", particle)
					}
					return
				}
				choice, ok := particle.(goxsd9.ChoiceParticle)
				if !ok {
					t.Fatalf("particle type = %T, want ChoiceParticle", particle)
				}
				assertChoiceOccurrenceRange(t, choice, test.wantChoice)
				if got := choice.Loc(); got.Source() != "root.xsd" || got.Line() != 3 || got.Column() != 5 {
					t.Fatalf("choice location = %s, want root.xsd:3:5", got)
				}
				alternatives := choice.Alternatives()
				if got, want := len(alternatives), len(test.wantAlternatives); got != want {
					t.Fatalf("alternative count = %d, want %d", got, want)
				}
				for index, want := range test.wantAlternatives {
					element, elementOK := alternatives[index].(goxsd9.ElementParticle)
					if !elementOK {
						t.Fatalf("alternative %d type = %T, want ElementParticle", index, alternatives[index])
					}
					if got := element.Name().Local(); got != want.name {
						t.Fatalf("alternative %d name = %q, want %q", index, got, want.name)
					}
					wantLine := choiceOccurrenceAlternativeLine(want.name)
					if got := element.Loc(); got.Source() != "root.xsd" || got.Line() != wantLine || got.Column() != 7 {
						t.Fatalf("alternative %d location = %s, want root.xsd:%d:7", index, got, wantLine)
					}
					assertChoiceOccurrenceRange(t, element, want.occurrences)
				}

				alternatives[0] = nil
				ownedAlternatives := choice.Alternatives()
				ownedElement, ok := ownedAlternatives[0].(goxsd9.ElementParticle)
				if !ok {
					t.Fatal("choice alternative copy has an unexpected particle type")
				}
				if got := ownedElement.Name().Local(); got != test.wantAlternatives[0].name {
					t.Fatalf("mutating Alternatives changed lexical order to %q", got)
				}
				if got := choice.Occurrences().String(); got != test.wantChoice {
					t.Fatalf("repeated choice occurrence query = %q, want %q", got, test.wantChoice)
				}
				if !reflect.DeepEqual(before, schema.Components()) {
					t.Fatal("choice queries mutated the completed schema")
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Keep precisionDecimal occurrence boundaries and omission order together.
func TestParseSchemaChoicePrecisionDecimalOccurrenceBoundary(t *testing.T) {
	tests := []struct {
		name               string
		choiceAttrs        string
		precisionAttrs     string
		named              bool
		wantParticle       bool
		wantUnsupported    bool
		wantChoice         string
		wantAlternatives   []string
		wantDiagnosticLine int
	}{
		{
			name:             "built-in default occurrence remains supported",
			wantParticle:     true,
			wantChoice:       "1/1",
			wantAlternatives: []string{"precision", "integer"},
		},
		{
			name:             "named restriction default occurrence remains supported",
			named:            true,
			wantParticle:     true,
			wantChoice:       "1/1",
			wantAlternatives: []string{"precision", "integer"},
		},
		{
			name:               "built-in non-default choice is unsupported",
			choiceAttrs:        ` minOccurs="0"`,
			wantUnsupported:    true,
			wantDiagnosticLine: 4,
		},
		{
			name:               "named non-default choice is unsupported",
			choiceAttrs:        ` minOccurs="0"`,
			named:              true,
			wantUnsupported:    true,
			wantDiagnosticLine: 4,
		},
		{
			name:               "built-in non-default alternative is unsupported",
			precisionAttrs:     ` minOccurs="0"`,
			wantUnsupported:    true,
			wantDiagnosticLine: 5,
		},
		{
			name:               "named non-default alternative is unsupported",
			precisionAttrs:     ` minOccurs="0"`,
			named:              true,
			wantUnsupported:    true,
			wantDiagnosticLine: 5,
		},
		{
			name:         "zero-zero choice omits precisionDecimal particle",
			choiceAttrs:  ` minOccurs="0" maxOccurs="0"`,
			wantParticle: false,
		},
		{
			name:             "zero-zero built-in alternative preserves order",
			precisionAttrs:   ` minOccurs="0" maxOccurs="0"`,
			wantParticle:     true,
			wantChoice:       "1/1",
			wantAlternatives: []string{"integer"},
		},
		{
			name:             "zero-zero named alternative remains omitted from non-default choice",
			choiceAttrs:      ` minOccurs="0"`,
			precisionAttrs:   ` minOccurs="0" maxOccurs="0"`,
			named:            true,
			wantParticle:     true,
			wantChoice:       "0/1",
			wantAlternatives: []string{"integer"},
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema, err := parseChoiceOccurrenceSchemaResult(t, policy, choicePrecisionDecimalTestDocument(test.choiceAttrs, test.precisionAttrs, test.named))
				if test.wantUnsupported {
					if err == nil {
						t.Fatal("ParseSchemaWithPolicy succeeded for unsupported precisionDecimal occurrence")
					}
					if got := len(schema.Components()); got != 0 {
						t.Fatalf("unsupported schema returned %d components, want none", got)
					}
					var diagnostic goxsd9.Diagnostic
					if !errors.As(err, &diagnostic) {
						t.Fatalf("error = %v, want located diagnostic", err)
					}
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Feature() != goxsd9.FeatureSchemaSyntax {
						t.Fatalf("diagnostic = %s/%q, want schema-syntax unsupported", diagnostic, diagnostic.Feature())
					}
					if !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("unsupported diagnostic lost ErrUnsupported: %v", err)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != test.wantDiagnosticLine {
						t.Fatalf("diagnostic location = %s, want root.xsd:%d", diagnostic.Loc(), test.wantDiagnosticLine)
					}
					return
				}
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				definition := choiceOccurrenceTestDefinition(t, schema)
				particle := definition.Particle()
				if !test.wantParticle {
					if particle != nil {
						t.Fatalf("particle = %T, want occurrence absence", particle)
					}
					return
				}
				choice, ok := particle.(goxsd9.ChoiceParticle)
				if !ok {
					t.Fatalf("particle type = %T, want ChoiceParticle", particle)
				}
				assertChoiceOccurrenceRange(t, choice, test.wantChoice)
				alternatives := choice.Alternatives()
				if got, want := len(alternatives), len(test.wantAlternatives); got != want {
					t.Fatalf("alternative count = %d, want %d", got, want)
				}
				for index, wantName := range test.wantAlternatives {
					element, ok := alternatives[index].(goxsd9.ElementParticle)
					if !ok {
						t.Fatalf("alternative %d type = %T, want ElementParticle", index, alternatives[index])
					}
					if got := element.Name().Local(); got != wantName {
						t.Fatalf("alternative %d name = %q, want %q", index, got, wantName)
					}
				}
			})
		}
	}
}

func choicePrecisionDecimalTestDocument(choiceAttrs, precisionAttrs string, named bool) string {
	typeName := "xs:precisionDecimal"
	typeDeclaration := ""
	if named {
		typeName = "r:Precision"
		typeDeclaration = `<xs:simpleType name="Precision"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType>`
	}
	return `<xs:schema xmlns:xs="` + choiceOccurrenceTestXSDNamespace + `" xmlns:r="urn:choice-occurrence" targetNamespace="urn:choice-occurrence">
  ` + typeDeclaration + `
  <xs:complexType name="Choice">
    <xs:choice` + choiceAttrs + `>
      <xs:element name="precision" type="` + typeName + `"` + precisionAttrs + `/>
      <xs:element name="integer" type="xs:integer"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
}

func assertChoiceOccurrenceRange(t *testing.T, particle goxsd9.Particle, want string) {
	t.Helper()
	occurrences := particle.Occurrences()
	if got := occurrences.String(); got != want {
		t.Fatalf("occurrences = %q, want %q", got, want)
	}
	parts := strings.SplitN(want, "/", 2)
	if got := occurrences.Minimum().Canonical(); got != parts[0] {
		t.Fatalf("minimum = %q, want %q", got, parts[0])
	}
	assertChoiceMaximum(t, occurrences.Maximum(), parts[1])
	wantLegacyBound := uint64(0)
	if want == "1/1" {
		wantLegacyBound = 1
	}
	if got := particle.MinOccurs(); got != wantLegacyBound {
		t.Fatalf("legacy minimum = %d, want %d", got, wantLegacyBound)
	}
	if got := particle.MaxOccurs(); got != wantLegacyBound {
		t.Fatalf("legacy maximum = %d, want %d", got, wantLegacyBound)
	}
}

func assertChoiceMaximum(t *testing.T, maximum goxsd9.ParticleOccurrenceMaximum, want string) {
	t.Helper()
	if want == "unbounded" {
		if maximum.Kind() != goxsd9.ParticleOccurrenceMaximumUnbounded || !maximum.IsUnbounded() {
			t.Fatalf("maximum = %s/%v, want unbounded", maximum, maximum.Kind())
		}
		if _, ok := maximum.Finite(); ok {
			t.Fatal("unbounded maximum returned a finite value")
		}
		return
	}
	if maximum.Kind() != goxsd9.ParticleOccurrenceMaximumFinite || maximum.IsUnbounded() {
		t.Fatalf("maximum = %s/%v, want finite", maximum, maximum.Kind())
	}
	finite, ok := maximum.Finite()
	if !ok || finite.Canonical() != want {
		t.Fatalf("finite maximum = (%q, %t), want %q", finite.Canonical(), ok, want)
	}
}

func choiceOccurrenceAlternativeLine(name string) int {
	switch name {
	case "first":
		return 4
	case "second":
		return 5
	case "third":
		return 6
	default:
		return 0
	}
}

//nolint:gocognit,funlen // Keep occurrence diagnostics and evidence checks together.
func TestParseSchemaRejectsChoiceOccurrenceErrorsWithLocatedCauses(t *testing.T) {
	tests := []struct {
		name             string
		choiceAttrs      string
		firstAttrs       string
		wantMessage      string
		wantPrimaryLine  int
		wantPrimaryCol   int
		wantRelatedCount int
		wantCode         string
		wantCauseCode    string
		wantCause        bool
		wantSpecSuffix   string
	}{
		{
			name:            "malformed choice minimum",
			choiceAttrs:     ` minOccurs="maybe"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  16,
			wantCauseCode:   goxsd9.InvalidIntegerLexicalCode,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "negative choice maximum",
			choiceAttrs:     ` maxOccurs="-1"`,
			wantMessage:     `attribute "maxOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  16,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "malformed choice maximum",
			choiceAttrs:     ` maxOccurs="1.0"`,
			wantMessage:     `attribute "maxOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  16,
			wantCauseCode:   goxsd9.InvalidIntegerLexicalCode,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "negative choice minimum",
			choiceAttrs:     ` minOccurs="-1"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  16,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "unbounded choice minimum",
			choiceAttrs:     ` minOccurs="unbounded"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  16,
			wantCauseCode:   goxsd9.InvalidIntegerLexicalCode,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceMinimumSpecSuffix,
		},
		{
			name:            "duplicate choice minimum",
			choiceAttrs:     ` minOccurs="0" minOccurs="1"`,
			wantMessage:     `duplicate attribute "minOccurs"`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  30,
			wantCode:        goxsd9.InvalidXMLSyntaxCode,
		},
		{
			name:            "malformed alternative maximum",
			firstAttrs:      ` maxOccurs="1.0"`,
			wantMessage:     `attribute "maxOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  50,
			wantCauseCode:   goxsd9.InvalidIntegerLexicalCode,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "malformed alternative minimum",
			firstAttrs:      ` minOccurs="1.0"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  50,
			wantCauseCode:   goxsd9.InvalidIntegerLexicalCode,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "negative alternative minimum",
			firstAttrs:      ` minOccurs="-1"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  50,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "negative alternative maximum",
			firstAttrs:      ` maxOccurs="-1"`,
			wantMessage:     `attribute "maxOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  50,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceDatatypeSpecSuffix,
		},
		{
			name:            "unbounded alternative minimum",
			firstAttrs:      ` minOccurs="unbounded"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  50,
			wantCauseCode:   goxsd9.InvalidIntegerLexicalCode,
			wantCause:       true,
			wantSpecSuffix:  choiceOccurrenceMinimumSpecSuffix,
		},
		{
			name:            "duplicate alternative maximum",
			firstAttrs:      ` maxOccurs="0" maxOccurs="1"`,
			wantMessage:     `duplicate attribute "maxOccurs"`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  64,
			wantCode:        goxsd9.InvalidXMLSyntaxCode,
		},
		{
			name:             "choice minimum exceeds maximum",
			choiceAttrs:      ` minOccurs="2" maxOccurs="1"`,
			wantMessage:      "particle minOccurs cannot exceed maxOccurs",
			wantPrimaryLine:  3,
			wantPrimaryCol:   5,
			wantRelatedCount: 2,
			wantCause:        true,
			wantSpecSuffix:   choiceOccurrenceRangeSpecSuffix,
		},
		{
			name:             "alternative minimum exceeds maximum",
			firstAttrs:       ` minOccurs="2" maxOccurs="1"`,
			wantMessage:      "particle minOccurs cannot exceed maxOccurs",
			wantPrimaryLine:  4,
			wantPrimaryCol:   7,
			wantRelatedCount: 2,
			wantCause:        true,
			wantSpecSuffix:   choiceOccurrenceRangeSpecSuffix,
		},
	}

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema, err := parseChoiceOccurrenceSchemaResult(t, policy, choiceOccurrenceTestDocument(test.choiceAttrs, test.firstAttrs, "", ""))
				if err == nil {
					t.Fatal("ParseSchemaWithPolicy succeeded for invalid occurrence input")
				}
				if got := len(schema.Components()); got != 0 {
					t.Fatalf("error returned %d components, want none", got)
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
					t.Fatalf("diagnostic = (%q,%q), want invalid/%s", diagnostic.Class(), diagnostic.Code(), wantCode)
				}
				if diagnostic.Message() != test.wantMessage {
					t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantMessage)
				}
				if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != test.wantPrimaryLine || diagnostic.Loc().Column() != test.wantPrimaryCol {
					t.Fatalf("diagnostic location = %s, want root.xsd:%d:%d", diagnostic.Loc(), test.wantPrimaryLine, test.wantPrimaryCol)
				}
				if got := len(diagnostic.Related()); got != test.wantRelatedCount {
					t.Fatalf("related location count = %d, want %d", got, test.wantRelatedCount)
				}
				if test.wantCauseCode != "" {
					var cause goxsd9.Diagnostic
					if !errors.As(diagnostic.Unwrap(), &cause) || cause.Code() != test.wantCauseCode {
						t.Fatalf("diagnostic cause = %v, want %s lexical diagnostic", diagnostic.Unwrap(), test.wantCauseCode)
					}
				}
				if test.wantCause && test.wantCauseCode == "" && diagnostic.Unwrap() == nil {
					t.Fatal("diagnostic lost its occurrence cause")
				}
				if test.wantSpecSuffix != "" {
					wantSpecRef := choiceOccurrenceSpecRef(policy, test.wantSpecSuffix)
					if diagnostic.SpecRef() != wantSpecRef {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpecRef)
					}
				}
			})
		}
	}
}

func choiceOccurrenceSpecRef(policy goxsd9.LanguagePolicy, suffix string) string {
	edition := "xsd11"
	if policy == goxsd9.Strict10 {
		edition = "xsd10"
	}
	return edition + "-" + suffix
}

func choiceOccurrenceTestDocument(choiceAttrs, firstAttrs, secondAttrs, thirdAttrs string) string {
	return `<xs:schema xmlns:xs="` + choiceOccurrenceTestXSDNamespace + `" targetNamespace="urn:choice-occurrence">
  <xs:complexType name="Choice">
    <xs:choice` + choiceAttrs + `>
      <xs:element name="first" type="xs:integer"` + firstAttrs + `/>
      <xs:element name="second" type="xs:decimal"` + secondAttrs + `/>
      <xs:element name="third" type="xs:integer"` + thirdAttrs + `/>
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
	rootSource, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	return goxsd9.ParseSchemaWithPolicy(rootSource, nil, policy)
}

func choiceOccurrenceTestDefinition(t *testing.T, schema goxsd9.Schema) goxsd9.ComplexTypeDefinition {
	t.Helper()
	name, err := goxsd9.NewQName("urn:choice-occurrence", "Choice")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("Choice component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice has no complex type view")
	}
	return definition
}
