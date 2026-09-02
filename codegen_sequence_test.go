package goxsd9_test

import (
	"bytes"
	"context"
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
