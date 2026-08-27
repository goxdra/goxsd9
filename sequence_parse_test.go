package goxsd9_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const sequenceTestXSDNamespace = "http://www.w3.org/2001/XMLSchema"

type sequenceTestResolver struct {
	documents map[string]string
}

func (resolver sequenceTestResolver) Resolve(
	ctx context.Context,
	_, schemaLocation string,
) (goxsd9.ResolvedSource, error) {
	contents, ok := resolver.documents[schemaLocation]
	if !ok {
		return goxsd9.ResolvedSource{}, fmt.Errorf("missing sequence test document %q", schemaLocation)
	}
	return goxsd9.NewResolvedSource(ctx, goxsd9.SourceID(schemaLocation), io.NopCloser(strings.NewReader(contents)))
}

//nolint:gocognit,funlen // Keep the public occurrence cases in one table.
func TestParseSchemaExposesExactOrderedSequenceParticles(t *testing.T) {
	tests := []struct {
		name             string
		sequenceAttrs    string
		firstAttrs       string
		wantRange        string
		wantFirstRange   string
		wantElements     []string
		wantSequence     bool
		wantMaximumKind  goxsd9.ParticleOccurrenceMaximumKind
		wantMaximumValue string
	}{
		{
			name:             "omitted defaults",
			wantRange:        "1/1",
			wantElements:     []string{"first", "second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "1",
		},
		{
			name:             "zero minimum",
			sequenceAttrs:    ` minOccurs="0"`,
			wantRange:        "0/1",
			wantElements:     []string{"first", "second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "1",
		},
		{
			name:            "omitted minimum with unbounded maximum",
			sequenceAttrs:   ` maxOccurs="unbounded"`,
			wantRange:       "1/unbounded",
			wantElements:    []string{"first", "second"},
			wantSequence:    true,
			wantMaximumKind: goxsd9.ParticleOccurrenceMaximumUnbounded,
		},
		{
			name:             "finite above uint64",
			sequenceAttrs:    ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`,
			wantRange:        "18446744073709551615/18446744073709551616",
			wantElements:     []string{"first", "second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "18446744073709551616",
		},
		{
			name:             "optional child",
			firstAttrs:       ` minOccurs="0"`,
			wantRange:        "1/1",
			wantFirstRange:   "0/1",
			wantElements:     []string{"first", "second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "1",
		},
		{
			name:            "unbounded maximum",
			sequenceAttrs:   ` minOccurs="18446744073709551616" maxOccurs="unbounded"`,
			wantRange:       "18446744073709551616/unbounded",
			wantElements:    []string{"first", "second"},
			wantSequence:    true,
			wantMaximumKind: goxsd9.ParticleOccurrenceMaximumUnbounded,
		},
		{
			name:          "zero zero sequence absence",
			sequenceAttrs: ` minOccurs="0" maxOccurs="0"`,
			wantSequence:  false,
		},
		{
			name:             "finite child above uint64",
			firstAttrs:       ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`,
			wantRange:        "1/1",
			wantFirstRange:   "18446744073709551615/18446744073709551616",
			wantElements:     []string{"first", "second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "1",
		},
		{
			name:             "unbounded child maximum",
			firstAttrs:       ` minOccurs="1" maxOccurs="unbounded"`,
			wantRange:        "1/1",
			wantFirstRange:   "1/unbounded",
			wantElements:     []string{"first", "second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "1",
		},
		{
			name:             "zero zero child absence",
			firstAttrs:       ` minOccurs="0" maxOccurs="0"`,
			wantRange:        "1/1",
			wantElements:     []string{"second"},
			wantSequence:     true,
			wantMaximumKind:  goxsd9.ParticleOccurrenceMaximumFinite,
			wantMaximumValue: "1",
		},
	}

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema := parseSequenceSchema(t, policy, sequenceTestDocument(test.sequenceAttrs, test.firstAttrs), nil)
				definition := sequenceTestDefinition(t, schema, "urn:sequence")
				particle := definition.Particle()
				if !test.wantSequence {
					if particle != nil {
						t.Fatalf("particle = %T, want occurrence absence", particle)
					}
					return
				}
				sequence, ok := particle.(goxsd9.SequenceParticle)
				if !ok {
					t.Fatalf("particle type = %T, want SequenceParticle", particle)
				}
				wantLegacyBound := uint64(0)
				if test.wantRange == "1/1" {
					wantLegacyBound = 1
				}
				if got := particle.MinOccurs(); got != wantLegacyBound {
					t.Fatalf("legacy sequence minimum = %d, want %d", got, wantLegacyBound)
				}
				if got := particle.MaxOccurs(); got != wantLegacyBound {
					t.Fatalf("legacy sequence maximum = %d, want %d", got, wantLegacyBound)
				}
				if got := sequence.Occurrences().String(); got != test.wantRange {
					t.Fatalf("sequence occurrences = %q, want %q", got, test.wantRange)
				}
				wantMinimum := strings.SplitN(test.wantRange, "/", 2)[0]
				if got := sequence.Occurrences().Minimum().Canonical(); got != wantMinimum {
					t.Fatalf("sequence minimum = %q, want %q", got, wantMinimum)
				}
				maximum := sequence.Occurrences().Maximum()
				if got := maximum.Kind(); got != test.wantMaximumKind {
					t.Fatalf("sequence maximum kind = %v, want %v", got, test.wantMaximumKind)
				}
				if maximum.IsUnbounded() {
					if _, ok := maximum.Finite(); ok {
						t.Fatal("unbounded maximum returned a finite value")
					}
				}
				if !maximum.IsUnbounded() {
					finite, ok := maximum.Finite()
					if !ok || finite.Canonical() != test.wantMaximumValue {
						t.Fatalf("finite maximum = (%q, %t), want %q", finite.Canonical(), ok, test.wantMaximumValue)
					}
				}

				elements := sequence.Elements()
				if got, want := len(elements), len(test.wantElements); got != want {
					t.Fatalf("element count = %d, want %d", got, want)
				}
				for index, wantName := range test.wantElements {
					if got := elements[index].Name().Local(); got != wantName {
						t.Fatalf("element %d local name = %q, want %q", index, got, wantName)
					}
					if got := elements[index].Name().Namespace(); got != "" {
						t.Fatalf("element %d namespace = %q, want unqualified local form", index, got)
					}
					if index == 0 && test.wantFirstRange != "" {
						if got := elements[index].Occurrences().String(); got != test.wantFirstRange {
							t.Fatalf("element %d occurrences = %q, want %q", index, got, test.wantFirstRange)
						}
						continue
					}
					if !elements[index].Occurrences().IsDefault() || elements[index].Occurrences().String() != "1/1" {
						t.Fatalf("element %d default occurrence string = %q", index, elements[index].Occurrences())
					}
				}
				if got := sequence.Loc(); got.Source() != "root.xsd" || got.Line() != 3 || got.Column() != 5 {
					t.Fatalf("sequence location = %s, want root.xsd:3:5", got)
				}
				if len(elements) > 0 {
					wantLine := 4
					if elements[0].Name().Local() == "second" {
						wantLine = 5
					}
					if got := elements[0].Loc(); got.Source() != "root.xsd" || got.Line() != wantLine || got.Column() != 7 {
						t.Fatalf("first element location = %s, want root.xsd:%d:7", got, wantLine)
					}
					if elements[0].Name().Local() == "first" {
						if got := elements[0].DeclaredType().String(); got != "{"+sequenceTestXSDNamespace+"}integer" {
							t.Fatalf("first declared type = %q, want expanded xs:integer", got)
						}
					}
					if elements[len(elements)-1].DeclaredType().String() != "{"+sequenceTestXSDNamespace+"}decimal" {
						t.Fatalf("last declared type = %q, want expanded xs:decimal", elements[len(elements)-1].DeclaredType())
					}
				}

				ownedElements := sequence.Elements()
				ownedElements[0] = goxsd9.ElementParticle{}
				if sequence.Elements()[0].Name().Local() != test.wantElements[0] {
					t.Fatal("mutating the element query changed the completed sequence")
				}
				firstMinimum := sequence.Occurrences().Minimum()
				if got := sequence.Occurrences().Minimum().Canonical(); got != firstMinimum.Canonical() {
					t.Fatal("repeated exact occurrence queries disagree")
				}
			})
		}
	}
}

func TestParseSchemaExposesEmptySequenceParticle(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" targetNamespace="urn:sequence">
  <xs:complexType name="Record"><xs:sequence/></xs:complexType>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := parseSequenceSchema(t, policy, root, nil)
			definition := sequenceTestDefinition(t, schema, "urn:sequence")
			particle := definition.Particle()
			sequence, ok := particle.(goxsd9.SequenceParticle)
			if !ok {
				t.Fatalf("particle type = %T, want SequenceParticle", particle)
			}
			if got := sequence.Occurrences().String(); got != "1/1" {
				t.Fatalf("sequence occurrences = %q, want 1/1", got)
			}
			if got := sequence.Elements(); len(got) != 0 {
				t.Fatalf("element count = %d, want 0", len(got))
			}
		})
	}
}

//nolint:gocognit // Keep the identity-resolution assertions together.
func TestParseSchemaResolvesForwardAndCrossDocumentSequenceScalarTypes(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Record">
    <xs:sequence>
      <xs:element name="forward" type="r:Amount"/>
      <xs:element name="builtin" type="xs:integer"/>
      <xs:element name="cross" type="o:CrossAmount"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" targetNamespace="urn:other">
  <xs:simpleType name="CrossAmount"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := parseSequenceSchema(t, policy, root, map[string]string{"other.xsd": other})
			definition := sequenceTestDefinition(t, schema, "urn:root")
			sequence, ok := definition.Particle().(goxsd9.SequenceParticle)
			if !ok {
				t.Fatalf("particle type = %T, want SequenceParticle", definition.Particle())
			}
			elements := sequence.Elements()
			if got, want := len(elements), 3; got != want {
				t.Fatalf("element count = %d, want %d", got, want)
			}
			rootAmount := findSequenceComponent(t, schema, "urn:root", "Amount")
			otherAmount := findSequenceComponent(t, schema, "urn:other", "CrossAmount")
			if got := elements[0].DeclaredType().String(); got != "{urn:root}Amount" {
				t.Fatalf("forward declared type = %q, want {urn:root}Amount", got)
			}
			if typeID, ok := elements[0].TypeID(); !ok || typeID != rootAmount.ID() {
				t.Fatalf("forward type ID = (%v, %t), want (%v, true)", typeID, ok, rootAmount.ID())
			}
			if typeID, ok := elements[1].TypeID(); ok || !typeID.IsZero() {
				t.Fatalf("built-in type ID = (%v, %t), want zero,false", typeID, ok)
			}
			if got := elements[2].DeclaredType().String(); got != "{urn:other}CrossAmount" {
				t.Fatalf("cross-document declared type = %q, want {urn:other}CrossAmount", got)
			}
			if typeID, ok := elements[2].TypeID(); !ok || typeID != otherAmount.ID() {
				t.Fatalf("cross-document type ID = (%v, %t), want (%v, true)", typeID, ok, otherAmount.ID())
			}
		})
	}
}

//nolint:gocognit,funlen // Keep located occurrence diagnostics in one table.
func TestParseSchemaRejectsSequenceOccurrenceErrorsWithLocatedCauses(t *testing.T) {
	tests := []struct {
		name             string
		sequenceAttrs    string
		firstAttrs       string
		wantMessage      string
		wantPrimaryLine  int
		wantPrimaryCol   int
		wantRelatedCount int
		wantLexicalCause bool
	}{
		{
			name:             "malformed minimum",
			sequenceAttrs:    ` minOccurs="maybe"`,
			wantMessage:      `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine:  3,
			wantPrimaryCol:   18,
			wantLexicalCause: true,
		},
		{
			name:            "negative maximum",
			sequenceAttrs:   ` maxOccurs="-1"`,
			wantMessage:     `attribute "maxOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 3,
			wantPrimaryCol:  18,
		},
		{
			name:             "minimum exceeds maximum",
			sequenceAttrs:    ` minOccurs="2" maxOccurs="1"`,
			wantMessage:      "particle minOccurs cannot exceed maxOccurs",
			wantPrimaryLine:  3,
			wantPrimaryCol:   5,
			wantRelatedCount: 2,
		},
		{
			name:             "omitted minimum with zero maximum",
			sequenceAttrs:    ` maxOccurs="0"`,
			wantMessage:      "particle minOccurs cannot exceed maxOccurs",
			wantPrimaryLine:  3,
			wantPrimaryCol:   5,
			wantRelatedCount: 1,
		},
		{
			name:             "malformed child maximum",
			firstAttrs:       ` maxOccurs="1.0"`,
			wantMessage:      `attribute "maxOccurs" has an invalid occurrence value`,
			wantPrimaryLine:  4,
			wantPrimaryCol:   50,
			wantLexicalCause: true,
		},
		{
			name:            "negative child minimum",
			firstAttrs:      ` minOccurs="-1"`,
			wantMessage:     `attribute "minOccurs" has an invalid occurrence value`,
			wantPrimaryLine: 4,
			wantPrimaryCol:  50,
		},
		{
			name:             "child minimum exceeds maximum",
			firstAttrs:       ` minOccurs="2" maxOccurs="1"`,
			wantMessage:      "particle minOccurs cannot exceed maxOccurs",
			wantPrimaryLine:  4,
			wantPrimaryCol:   7,
			wantRelatedCount: 2,
		},
	}

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				_, err := parseSequenceSchemaResult(t, policy, sequenceTestDocument(test.sequenceAttrs, test.firstAttrs), nil)
				if err == nil {
					t.Fatal("ParseSchemaWithPolicy succeeded for invalid occurrence input")
				}
				var diagnostic goxsd9.Diagnostic
				if !errors.As(err, &diagnostic) {
					t.Fatalf("error = %v, want located diagnostic", err)
				}
				if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != "XSD3010" {
					t.Fatalf("diagnostic = (%q,%q), want invalid XSD3010", diagnostic.Class(), diagnostic.Code())
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
				if test.wantLexicalCause {
					var cause goxsd9.Diagnostic
					if !errors.As(diagnostic.Unwrap(), &cause) || cause.Code() != "XSD2001" {
						t.Fatalf("diagnostic cause = %v, want XSD2001 lexical diagnostic", diagnostic.Unwrap())
					}
				}
			})
		}
	}
}

func TestParseSchemaRejectsPrecisionDecimalSequenceElements(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" targetNamespace="urn:sequence">
  <xs:complexType name="Record">
    <xs:sequence>
      <xs:element name="value" type="xs:precisionDecimal"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict11, goxsd9.Compatibility} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := parseSequenceSchemaResult(t, policy, root, nil)
			assertPrecisionDecimalSequenceUnsupported(t, schema, err)
		})
	}
}

func assertPrecisionDecimalSequenceUnsupported(t *testing.T, schema goxsd9.Schema, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("ParseSchemaWithPolicy accepted precisionDecimal in a direct sequence")
	}
	if components := schema.Components(); len(components) != 0 {
		t.Fatalf("error returned partial schema with %d components", len(components))
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v, want located diagnostic", err)
	}
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = (%q,%q,%q), want registered schema-syntax unsupported", diagnostic.Class(), diagnostic.Feature(), diagnostic.Code())
	}
	if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
		t.Fatalf("diagnostic spec ref = %q, want xsd11 schema document", diagnostic.SpecRef())
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 4 || diagnostic.Loc().Column() != 32 {
		t.Fatalf("diagnostic location = %s, want root.xsd:4:32", diagnostic.Loc())
	}
	if !strings.Contains(diagnostic.Message(), "precisionDecimal") {
		t.Fatalf("diagnostic message = %q, want precisionDecimal", diagnostic.Message())
	}
	if !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic error = %v, want ErrUnsupported", err)
	}
}

func sequenceTestDocument(sequenceAttrs, firstAttrs string) string {
	return `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" targetNamespace="urn:sequence">
  <xs:complexType name="Record">
    <xs:sequence` + sequenceAttrs + `>
      <xs:element name="first" type="xs:integer"` + firstAttrs + `/>
      <xs:element name="second" type="xs:decimal"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
}

func parseSequenceSchema(t *testing.T, policy goxsd9.LanguagePolicy, root string, documents map[string]string) goxsd9.Schema {
	t.Helper()
	schema, err := parseSequenceSchemaResult(t, policy, root, documents)
	if err != nil {
		t.Fatalf("ParseSchemaWithPolicy: %v", err)
	}
	return schema
}

func parseSequenceSchemaResult(t *testing.T, policy goxsd9.LanguagePolicy, root string, documents map[string]string) (goxsd9.Schema, error) {
	t.Helper()
	rootSource, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	var resolver goxsd9.Resolver
	if len(documents) != 0 {
		resolver = sequenceTestResolver{documents: documents}
	}
	return goxsd9.ParseSchemaWithPolicy(rootSource, resolver, policy)
}

func sequenceTestDefinition(t *testing.T, schema goxsd9.Schema, namespace string) goxsd9.ComplexTypeDefinition {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, "Record")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	return definition
}

func findSequenceComponent(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s component count = %d, want 1", name, len(components))
	}
	return components[0]
}
