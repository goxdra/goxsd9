package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const validationSequenceNamespace = "urn:sequence"

func TestValidateInstanceSupportsDirectScalarSequences(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="1.1">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence>
    <xs:element name="integer" type="xs:integer"/>
    <xs:element name="decimal" type="xs:decimal"/>
  </xs:sequence></xs:complexType>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, root, nil, policy)
			input := `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">-12</integer><decimal xmlns="">3.140</decimal></root>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep the complete occurrence and policy matrix together.
func TestValidateInstanceHonorsSequenceOccurrences(t *testing.T) {
	cases := []struct {
		name          string
		sequenceAttrs string
		firstAttrs    string
		secondAttrs   string
		input         string
		wantError     bool
	}{
		{
			name:          "optional outer sequence",
			sequenceAttrs: ` minOccurs="0"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"/>`,
		},
		{
			name:       "optional child",
			firstAttrs: ` minOccurs="0"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><decimal xmlns="">1.25</decimal></root>`,
		},
		{
			name:       "finite repeated child",
			firstAttrs: ` maxOccurs="2"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer><integer xmlns="">2</integer><decimal xmlns="">3.25</decimal></root>`,
		},
		{
			name:       "unbounded child",
			firstAttrs: ` maxOccurs="unbounded"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer><integer xmlns="">2</integer><integer xmlns="">3</integer><decimal xmlns="">4.50</decimal></root>`,
		},
		{
			name:          "repeated outer sequence",
			sequenceAttrs: ` maxOccurs="2"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer><decimal xmlns="">2.50</decimal><integer xmlns="">3</integer><decimal xmlns="">4.50</decimal></root>`,
		},
		{
			name:          "unbounded outer sequence",
			sequenceAttrs: ` maxOccurs="unbounded"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer><decimal xmlns="">2.50</decimal><integer xmlns="">3</integer><decimal xmlns="">4.50</decimal></root>`,
		},
		{
			name:       "child exact range above uint64",
			firstAttrs: ` minOccurs="18446744073709551616" maxOccurs="18446744073709551616"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer><decimal xmlns="">2.50</decimal></root>`,
			wantError:  true,
		},
		{
			name:          "outer exact range above uint64",
			sequenceAttrs: ` minOccurs="18446744073709551616" maxOccurs="18446744073709551616"`,
			firstAttrs:    ` minOccurs="0"`,
			secondAttrs:   ` minOccurs="0"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"/>`,
		},
		{
			name:          "outer exact range above uint64 with content",
			sequenceAttrs: ` minOccurs="18446744073709551616" maxOccurs="18446744073709551616"`,
			firstAttrs:    ` minOccurs="0"`,
			secondAttrs:   ` minOccurs="0"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer></root>`,
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					schema := validationSequenceOccurrenceSchema(t, policy, test.sequenceAttrs, test.firstAttrs, test.secondAttrs)
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					if test.wantError {
						if err == nil {
							t.Fatal("ValidateInstance succeeded for an under-counted exact range")
						}
						if diagnostic := validationTestDiagnostic(t, err); diagnostic.Code() != goxsd9.InvalidInstanceSequenceCode {
							t.Fatalf("diagnostic Code() = %q, want %q", diagnostic.Code(), goxsd9.InvalidInstanceSequenceCode)
						}
						return
					}
					if err != nil {
						t.Fatalf("ValidateInstance: %v", err)
					}
				})
			}
		})
	}
}

func validationSequenceOccurrenceSchema(t *testing.T, policy goxsd9.LanguagePolicy, sequenceAttrs, firstAttrs, secondAttrs string) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence` + sequenceAttrs + `>
    <xs:element name="integer" type="xs:integer"` + firstAttrs + `/>
    <xs:element name="decimal" type="xs:decimal"` + secondAttrs + `/>
  </xs:sequence></xs:complexType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
}

//nolint:gocognit,funlen // Keep structural diagnostics, locations, and policies together.
func TestValidateInstanceReportsDirectSequenceStructure(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantCode   string
		wantClass  goxsd9.FailureClass
		wantMarker string
	}{
		{
			name:      "missing required child",
			input:     `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer></root>`,
			wantCode:  goxsd9.InvalidInstanceSequenceCode,
			wantClass: goxsd9.FailureInvalid,
		},
		{
			name:       "wrong order",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><decimal xmlns="">1.25</decimal><integer xmlns="">1</integer></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "<decimal",
		},
		{
			name:       "wrong namespace",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="urn:other">1</integer><decimal xmlns="">1.25</decimal></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: `<integer xmlns="urn:other">`,
		},
		{
			name:       "unexpected extra child",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1</integer><decimal xmlns="">1.25</decimal><extra xmlns="">2</extra></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "<extra",
		},
		{
			name:       "non-whitespace parent text",
			input:      `<root xmlns="` + validationSequenceNamespace + `">text<integer xmlns="">1</integer><decimal xmlns="">1.25</decimal></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "text",
		},
		{
			name:       "nested content",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns=""><nested/></integer><decimal xmlns="">1.25</decimal></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "<nested",
		},
		{
			name:       "root attribute",
			input:      `<root xmlns="` + validationSequenceNamespace + `" id="1"><integer xmlns="">1</integer><decimal xmlns="">1.25</decimal></root>`,
			wantCode:   goxsd9.UnsupportedInstanceValidationCode,
			wantClass:  goxsd9.FailureUnsupported,
			wantMarker: `id="1"`,
		},
		{
			name:       "child attribute",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="" id="1">1</integer><decimal xmlns="">1.25</decimal></root>`,
			wantCode:   goxsd9.UnsupportedInstanceValidationCode,
			wantClass:  goxsd9.FailureUnsupported,
			wantMarker: `id="1"`,
		},
		{
			name:       "malformed integer",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><integer xmlns="">1.0</integer><decimal xmlns="">1.25</decimal></root>`,
			wantCode:   goxsd9.InvalidIntegerLexicalCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "1.0",
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationSequenceSchema(t, policy, false)
			before := schema.Components()
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input))))
					if diagnostic.Class() != test.wantClass || diagnostic.Code() != test.wantCode {
						t.Fatalf("diagnostic = %s/%q, want %s/%q", diagnostic, diagnostic.Code(), test.wantClass, test.wantCode)
					}
					if test.wantMarker != "" && diagnostic.Loc() != validationSequenceMarkerLoc(t, test.input, test.wantMarker) {
						t.Fatalf("diagnostic Loc() = %s, want marker %q", diagnostic.Loc(), test.wantMarker)
					}
					if diagnostic.SpecRef() == "" {
						t.Fatalf("diagnostic SpecRef() is empty")
					}
					if test.wantCode != goxsd9.InvalidIntegerLexicalCode && diagnostic.Unwrap() == nil {
						t.Fatalf("diagnostic lost its structural cause")
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("sequence validation mutated the completed schema")
			}
		})
	}
}

func validationSequenceSchema(t *testing.T, policy goxsd9.LanguagePolicy, qualified bool) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	form := ""
	if qualified {
		form = ` elementFormDefault="qualified"`
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `"` + form + `>
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence>
    <xs:element name="integer" type="xs:integer"/>
    <xs:element name="decimal" type="xs:decimal"/>
  </xs:sequence></xs:complexType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
}

func validationSequenceMarkerLoc(t *testing.T, input, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(input, marker)
	if index < 0 {
		t.Fatalf("input has no marker %q", marker)
	}
	return validationTestLoc(t, "instance.xml", 1, index+1)
}

func TestValidateInstanceSupportsQualifiedNamedAndCrossDocumentSequenceScalars(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.1"
			if policy == goxsd9.Strict10 {
				version = "1.0"
			}
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" xmlns:o="urn:sequence-other" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `" elementFormDefault="qualified">
  <xs:import namespace="urn:sequence-other" schemaLocation="sequence-other.xsd"/>
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence>
    <xs:element name="integer" type="r:NamedInteger"/>
    <xs:element name="decimal" type="o:CrossDecimal"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="NamedInteger"><xs:restriction base="xs:integer"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:sequence-other" version="` + version + `"><xs:simpleType name="CrossDecimal"><xs:restriction base="xs:decimal"><xs:fractionDigits value="3"/></xs:restriction></xs:simpleType></xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, map[string]validationTestFixture{
				"sequence-other.xsd": {id: "sequence-other.xsd", contents: other},
			}, policy)
			input := `<root xmlns="` + validationSequenceNamespace + `"><integer>1234</integer><decimal>1.234</decimal></root>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep the explicit unsupported-shape matrix together.
func TestValidateInstanceKeepsDirectSequenceExclusionsExplicit(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "built-in boolean",
			body: `<xs:complexType name="Root"><xs:sequence><xs:element name="value" type="xs:boolean"/></xs:sequence></xs:complexType>`,
		},
		{
			name: "named boolean",
			body: `<xs:complexType name="Root"><xs:sequence><xs:element name="value" type="r:Flag"/></xs:sequence></xs:complexType><xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`,
		},
		{
			name: "nillable local",
			body: `<xs:complexType name="Root"><xs:sequence><xs:element name="value" type="xs:integer" nillable="true"/></xs:sequence></xs:complexType>`,
		},
		{
			name: "element reference",
			body: `<xs:element name="target" type="xs:integer"/><xs:complexType name="Root"><xs:sequence><xs:element ref="r:target"/></xs:sequence></xs:complexType>`,
		},
		{
			name: "mixed local and reference",
			body: `<xs:element name="target" type="xs:integer"/><xs:complexType name="Root"><xs:sequence><xs:element ref="r:target"/><xs:element name="local" type="xs:integer"/></xs:sequence></xs:complexType>`,
		},
		{
			name: "attribute wildcard",
			body: `<xs:complexType name="Root"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:anyAttribute/></xs:complexType>`,
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					version := "1.1"
					if policy == goxsd9.Strict10 {
						version = "1.0"
					}
					root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `">` + `<xs:element name="root" type="r:Root"/>` + test.body + `</xs:schema>`
					schema := validationTestSchemaWithPolicy(t, root, nil, policy)
					input := `<root xmlns="` + validationSequenceNamespace + `"><value xmlns="">1</value><local xmlns="">1</local><target xmlns="">1</target></root>`
					diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode {
						t.Fatalf("diagnostic = %s/%q, want unsupported instance-validation diagnostic", diagnostic, diagnostic.Code())
					}
					if !errors.Is(diagnostic, goxsd9.ErrUnsupported) || diagnostic.Loc().IsZero() || diagnostic.SpecRef() == "" {
						t.Fatalf("diagnostic evidence = %s/%q/%v, want located specification-backed unsupported", diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Unwrap())
					}
				})
			}
		})
	}
}

func TestValidateInstanceSequencePreservesSemanticAndReaderLifecycleErrors(t *testing.T) {
	schema := validationSequenceSchema(t, goxsd9.Strict11, false)
	semanticInput := `<root xmlns="` + validationSequenceNamespace + `"><decimal xmlns="">1.25</decimal><integer xmlns="">1</integer></root>`
	closeErr := errors.New("sequence close failed")
	closeReader := newValidationTestSource(semanticInput)
	closeReader.closeErr = closeErr
	err := goxsd9.ValidateInstance(schema, "instance.xml", closeReader)
	if err == nil || !errors.Is(err, closeErr) {
		t.Fatalf("close failure = %v, want semantic and close causes", err)
	}
	diagnostics := validationTestDiagnostics(t, err)
	if len(diagnostics) != 2 || diagnostics[0].Code() != goxsd9.InvalidInstanceSequenceCode || diagnostics[1].Code() != goxsd9.SourceCloseCode {
		t.Fatalf("semantic/close diagnostics = %#v, want sequence then close", diagnostics)
	}
	if !closeReader.closed || closeReader.closeCalls != 1 || closeReader.offset != len(closeReader.data) {
		t.Fatalf("close lifecycle = closed %t, calls %d, offset %d, want closed once and fully consumed", closeReader.closed, closeReader.closeCalls, closeReader.offset)
	}

	readErr := errors.New("sequence read failed")
	readCloseErr := errors.New("sequence read close failed")
	readPrefix := `<root xmlns="` + validationSequenceNamespace + `"><decimal xmlns="">1.25</decimal>`
	readInput := readPrefix + `<integer xmlns="">1</integer></root>`
	readReader := newValidationTestSource(readInput)
	readReader.failAt = len(readPrefix)
	readReader.readErr = readErr
	readReader.closeErr = readCloseErr
	err = goxsd9.ValidateInstance(schema, "instance.xml", readReader)
	if err == nil || !errors.Is(err, readErr) || !errors.Is(err, readCloseErr) {
		t.Fatalf("read failure = %v, want semantic, read, and close causes", err)
	}
	diagnostics = validationTestDiagnostics(t, err)
	if len(diagnostics) != 3 || diagnostics[0].Code() != goxsd9.InvalidInstanceSequenceCode || diagnostics[1].Code() != goxsd9.SourceReadCode || diagnostics[2].Code() != goxsd9.SourceCloseCode {
		t.Fatalf("semantic/read/close diagnostics = %#v, want sequence, read, close", diagnostics)
	}
	if !readReader.closed || readReader.closeCalls != 1 || readReader.offset != readReader.failAt {
		t.Fatalf("read lifecycle = closed %t, calls %d, offset %d, want closed once at failure", readReader.closed, readReader.closeCalls, readReader.failAt)
	}
}
