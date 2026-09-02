package goxsd9_test

import (
	"bytes"
	"context"
	"errors"
	"go/format"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep the cross-edition generation and consumer corpus together.
func TestGenerateGoDirectScalarSequencesAcrossEditions(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Empty"><xs:sequence/></xs:complexType>
  <xs:complexType name="One"><xs:sequence><xs:element name="amount" type="xs:integer"/></xs:sequence></xs:complexType>
  <xs:complexType name="Record"><xs:sequence>
    <xs:element name="first-value" type="r:ForwardInteger"/>
    <xs:element name="second" type="xs:decimal"/>
    <xs:element name="third-value" type="r:NamedDecimal"/>
    <xs:element name="fourth" type="o:CrossInteger"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="ForwardInteger"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="NamedDecimal"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" targetNamespace="urn:other">
  <xs:simpleType name="CrossInteger"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.0"
			if policy == goxsd9.Strict11 {
				version = "1.1"
			}
			versionedRoot := strings.Replace(root, `targetNamespace="urn:root"`, `targetNamespace="urn:root" version="`+version+`"`, 1)
			rootSource, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(versionedRoot)))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(rootSource, sequenceTestResolver{documents: map[string]string{"other.xsd": other}}, policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			source, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo second: %v", err)
			}
			if !bytes.Equal(source, second) {
				t.Fatalf("repeated direct-sequence output differs:\nfirst:\n%s\nsecond:\n%s", source, second)
			}
			formatted, err := format.Source(source)
			if err != nil {
				t.Fatalf("format generated direct-sequence source: %v\n%s", err, source)
			}
			if !bytes.Equal(source, formatted) {
				t.Fatalf("generated direct-sequence source is not formatted:\n%s", source)
			}
			for _, fragment := range []string{
				`import Runtime "github.com/goxdra/goxsd9"`,
				"type Empty struct{}",
				"type One struct {\n\tAmount Runtime.StrictInteger\n}",
				"type Record struct {\n\tFirstValue ForwardInteger\n\tSecond     Runtime.StrictDecimal\n\tThirdValue NamedDecimal\n\tFourth     CrossInteger\n}",
				"type ForwardInteger struct {\n\tValue Runtime.StrictInteger\n}",
				"type NamedDecimal struct {\n\tValue Runtime.StrictDecimal\n}",
				"type CrossInteger struct {\n\tValue Runtime.StrictInteger\n}",
			} {
				if !strings.Contains(string(source), fragment) {
					t.Fatalf("generated direct-sequence source is missing %q:\n%s", fragment, source)
				}
			}
			compilePublicGeneratedCode(t, source, `package consumer

import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)

func useGeneratedSequences() {
	var one generated.One
	var record generated.Record
	var _ generated.Empty = generated.Empty{}
	var _ runtime.StrictInteger = one.Amount
	var _ generated.ForwardInteger = record.FirstValue
	var _ runtime.StrictDecimal = record.Second
	var _ generated.NamedDecimal = record.ThirdValue
	var _ generated.CrossInteger = record.Fourth
}
`)
		})
	}
}

func TestParseSchemaRejectsNamedAndInheritedAtomicStringDirectSequenceElementsAcrossEditions(t *testing.T) {
	for _, test := range []struct {
		name         string
		policy       goxsd9.LanguagePolicy
		version      string
		declaredType string
		wantSpec     string
	}{
		{
			name:         "Strict10/named",
			policy:       goxsd9.Strict10,
			version:      "1.0",
			declaredType: "Text",
			wantSpec:     "xsd10-structures#schema-document",
		},
		{
			name:         "Strict10/inherited",
			policy:       goxsd9.Strict10,
			version:      "1.0",
			declaredType: "InheritedText",
			wantSpec:     "xsd10-structures#schema-document",
		},
		{
			name:         "Strict11/named",
			policy:       goxsd9.Strict11,
			version:      "1.1",
			declaredType: "Text",
			wantSpec:     "xsd11-structures#cSchemaDocument",
		},
		{
			name:         "Strict11/inherited",
			policy:       goxsd9.Strict11,
			version:      "1.1",
			declaredType: "InheritedText",
			wantSpec:     "xsd11-structures#cSchemaDocument",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" xmlns:r="urn:sequence" targetNamespace="urn:sequence" version="` + test.version + `">
  <xs:complexType name="Record"><xs:sequence><xs:element name="value" type="r:` + test.declaredType + `"/></xs:sequence></xs:complexType>
  <xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>
  <xs:simpleType name="InheritedText"><xs:restriction base="r:Text"/></xs:simpleType>
</xs:schema>`
			schema, err := parseSequenceSchemaResult(t, test.policy, root, nil)
			assertPublicAtomicStringSequenceParseUnsupported(t, schema, err, test.declaredType, test.wantSpec)
		})
	}
}

func assertPublicAtomicStringSequenceParseUnsupported(t *testing.T, schema goxsd9.Schema, err error, declaredType, wantSpec string) {
	t.Helper()
	if err == nil {
		t.Fatal("ParseSchemaWithPolicy accepted an atomic-string local sequence element")
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("ParseSchemaWithPolicy returned a partial schema")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T does not contain a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want unsupported schema-syntax diagnostic", diagnostic)
	}
	if diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || diagnostic.SpecRef() != wantSpec {
		t.Fatalf("diagnostic feature/specification reference = %q/%q, want %q/%q", diagnostic.Feature(), diagnostic.SpecRef(), goxsd9.FeatureSchemaSyntax, wantSpec)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() != 71 {
		t.Fatalf("diagnostic location = %s, want root.xsd:2:71", diagnostic.Loc())
	}
	wantMessage := `element type "{urn:sequence}` + declaredType + `" is not implemented for local sequence elements`
	if diagnostic.Message() != wantMessage {
		t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), wantMessage)
	}
	if !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic lost unsupported cause: %v", err)
	}
}

func TestGenerateGoRejectsUnsupportedDirectSequenceShapes(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantSpec string
	}{
		{
			name:     "non-default wrapper occurrences",
			body:     `<xs:sequence minOccurs="0"><xs:element name="value" type="xs:integer"/></xs:sequence>`,
			wantSpec: "xsd11-structures#Particle_details",
		},
		{
			name:     "non-default child occurrences",
			body:     `<xs:sequence><xs:element name="value" type="xs:integer" minOccurs="0"/></xs:sequence>`,
			wantSpec: "xsd11-structures#Particle_details",
		},
		{
			name:     "boolean child",
			body:     `<xs:sequence><xs:element name="value" type="xs:boolean"/></xs:sequence>`,
			wantSpec: "xsd11-structures#element-sequence",
		},
		{
			name:     "attribute wildcard",
			body:     `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:anyAttribute namespace="##other" processContents="lax"/>`,
			wantSpec: "xsd11-structures#element-sequence",
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				version := "1.0"
				wantSpec := strings.Replace(test.wantSpec, "xsd11", "xsd10", 1)
				if policy == goxsd9.Strict11 {
					version = "1.1"
					wantSpec = test.wantSpec
				}
				root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" xmlns:r="urn:sequence" targetNamespace="urn:sequence" version="` + version + `">
  <xs:element name="global" type="xs:integer"/>
  <xs:complexType name="Record">` + test.body + `</xs:complexType>
</xs:schema>`
				schema, err := parseSequenceSchemaResult(t, policy, root, nil)
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				assertPublicUnsupportedCodegen(t, schema, wantSpec)
			})
		}
	}
}

//nolint:gocognit // Keep edition-specific wildcard diagnostic assertions together.
func TestGenerateGoRejectsDirectChoiceAttributeWildcardAcrossEditions(t *testing.T) {
	for _, test := range []struct {
		name     string
		policy   goxsd9.LanguagePolicy
		version  string
		wantSpec string
	}{
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0", wantSpec: "xsd10-structures#element-choice"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1", wantSpec: "xsd11-structures#element-choice"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" targetNamespace="urn:choice" version="` + test.version + `">
  <xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>
</xs:schema>`
			schema, err := parseSequenceSchemaResult(t, test.policy, root, nil)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			name, err := goxsd9.NewQName("urn:choice", "Choice")
			if err != nil {
				t.Fatalf("NewQName: %v", err)
			}
			components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
			if len(components) != 1 {
				t.Fatalf("Choice component count = %d, want 1", len(components))
			}
			definition, ok := components[0].ComplexType()
			if !ok {
				t.Fatal("Choice has no complex type view")
			}
			choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
			if !ok {
				t.Fatalf("Choice particle = %T, want ChoiceParticle", definition.Particle())
			}
			wildcard, ok := definition.AnyAttribute()
			if !ok {
				t.Fatal("Choice has no anyAttribute wildcard")
			}

			source, err := goxsd9.GenerateGo(schema, "generated")
			if source != nil || err == nil {
				t.Fatalf("direct-choice wildcard result = (%q, %v), want nil source and error", source, err)
			}
			diagnostic := requirePublicCodegenDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode {
				t.Fatalf("diagnostic = %s, want unsupported codegen diagnostic", diagnostic)
			}
			if diagnostic.Feature() != goxsd9.FeatureCodegen || diagnostic.SpecRef() != test.wantSpec {
				t.Fatalf("diagnostic feature/specification reference = %q/%q, want %q/%q", diagnostic.Feature(), diagnostic.SpecRef(), goxsd9.FeatureCodegen, test.wantSpec)
			}
			if diagnostic.Loc() != wildcard.Loc() {
				t.Fatalf("diagnostic primary location = %s, want wildcard location %s", diagnostic.Loc(), wildcard.Loc())
			}
			related := diagnostic.Related()
			if len(related) != 1 || related[0] != choice.Loc() {
				t.Fatalf("diagnostic related locations = %v, want [%s]", related, choice.Loc())
			}
			if !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("diagnostic lost unsupported cause: %v", err)
			}
		})
	}
}

func TestGenerateGoRejectsDirectSequenceElementReference(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.0"
			wantSpec := "xsd10-structures#element-sequence"
			if policy == goxsd9.Strict11 {
				version = "1.1"
				wantSpec = "xsd11-structures#element-sequence"
			}
			root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" xmlns:r="urn:sequence" targetNamespace="urn:sequence" version="` + version + `">
  <xs:element name="global" type="xs:integer"/>
  <xs:complexType name="Record"><xs:sequence><xs:element ref="r:global"/></xs:sequence></xs:complexType>
</xs:schema>`
			schema, err := parseSequenceSchemaResult(t, policy, root, nil)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			assertPublicUnsupportedCodegen(t, schema, wantSpec)
		})
	}
}

func TestGenerateGoRejectsNamedBooleanDirectSequenceElement(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.0"
			wantSpec := "xsd10-structures#element-sequence"
			if policy == goxsd9.Strict11 {
				version = "1.1"
				wantSpec = "xsd11-structures#element-sequence"
			}
			root := `<xs:schema xmlns:xs="` + sequenceTestXSDNamespace + `" xmlns:r="urn:sequence" targetNamespace="urn:sequence" version="` + version + `">
  <xs:complexType name="Record"><xs:sequence><xs:element name="value" type="r:Flag"/></xs:sequence></xs:complexType>
  <xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			schema, err := parseSequenceSchemaResult(t, policy, root, nil)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			assertPublicUnsupportedCodegen(t, schema, wantSpec)
		})
	}
}

func TestGenerateGoPreservesDirectChoiceAlongsideSequence(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test">
  <xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice></xs:complexType>
  <xs:complexType name="Record"><xs:sequence><xs:element name="amount" type="xs:decimal"/></xs:sequence></xs:complexType>
</xs:schema>`)
	source, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	for _, fragment := range []string{
		"type Choice interface {\n\tisChoice()\n}",
		"type Record struct {\n\tAmount Runtime.StrictDecimal\n}",
	} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("generated mixed direct-particle source is missing %q:\n%s", fragment, source)
		}
	}
	compilePublicGeneratedCode(t, source)
}
