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

func TestValidateInstanceSupportsDirectBooleanSequencesAcrossPolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="1.1">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence>
    <xs:element name="builtin" type="xs:boolean"/>
    <xs:element name="named" type="r:Flag"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Flag"><xs:restriction base="r:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
	inputs := []string{
		`<root xmlns="` + validationSequenceNamespace + `"><builtin xmlns="">true</builtin><named xmlns="">false</named></root>`,
		`<root xmlns="` + validationSequenceNamespace + `"><builtin xmlns="">1</builtin><named xmlns="">0</named></root>`,
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, root, nil, policy)
			before := schema.Components()
			for _, input := range inputs {
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("ValidateInstance(%q): %v", input, err)
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("Boolean sequence validation mutated the completed schema")
			}
		})
	}
}

func TestValidateInstanceSupportsBooleanSequenceGraphVisibility(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.1"
			if policy == goxsd9.Strict10 {
				version = "1.0"
			}
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" xmlns:o="urn:sequence-other" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `"><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:sequence-other" schemaLocation="other.xsd"/><xs:element name="root" type="r:Root"/><xs:complexType name="Root"><xs:sequence><xs:element name="forward" type="r:ForwardFlag"/><xs:element name="included" type="r:IncludedFlag"/><xs:element name="imported" type="o:ImportedFlag"/></xs:sequence></xs:complexType><xs:simpleType name="ForwardFlag"><xs:restriction base="r:BaseFlag"/></xs:simpleType><xs:simpleType name="BaseFlag"><xs:restriction base="xs:boolean"/></xs:simpleType></xs:schema>`
			chameleon := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `"><xs:simpleType name="IncludedFlag"><xs:restriction base="xs:boolean"/></xs:simpleType></xs:schema>`
			other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:sequence-other" version="` + version + `"><xs:simpleType name="ImportedFlag"><xs:restriction base="xs:boolean"/></xs:simpleType></xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, map[string]validationTestFixture{
				"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
				"other.xsd":     {id: "other.xsd", contents: other},
			}, policy)
			input := `<root xmlns="` + validationSequenceNamespace + `"><forward xmlns="">1</forward><included xmlns="">false</included><imported xmlns="">0</imported></root>`
			before := schema.Components()
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("graph-visible Boolean sequence validation mutated the completed schema")
			}
		})
	}
}

func TestValidateInstanceHonorsSequenceOccurrences(t *testing.T) {
	for _, profile := range validationSequenceOccurrenceProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			runValidationSequenceOccurrenceProfile(t, profile)
		})
	}
}

func runValidationSequenceOccurrenceProfile(t *testing.T, profile validationSequenceOccurrenceProfile) {
	t.Helper()
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			runValidationSequenceOccurrencePolicy(t, profile, policy)
		})
	}
}

func runValidationSequenceOccurrencePolicy(t *testing.T, profile validationSequenceOccurrenceProfile, policy goxsd9.LanguagePolicy) {
	t.Helper()
	for _, test := range validationSequenceOccurrenceCases() {
		t.Run(test.name, func(t *testing.T) {
			schema := validationSequenceOccurrenceSchema(t, policy, profile, test.sequenceAttrs, test.firstAttrs, test.secondAttrs)
			input := renderValidationSequenceInput(test.input, profile.values)
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			if test.wantError {
				diagnostic := validationTestDiagnostic(t, err)
				if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidInstanceSequenceCode {
					t.Fatalf("diagnostic = %s/%q, want invalid sequence", diagnostic, diagnostic.Code())
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
		})
	}
}

type validationSequenceOccurrenceProfile struct {
	name       string
	firstType  string
	secondType string
	extra      string
	values     validationSequenceOccurrenceValues
}

type validationSequenceOccurrenceValues struct {
	first   string
	first2  string
	first3  string
	second  string
	second2 string
}

type validationSequenceOccurrenceCase struct {
	name          string
	sequenceAttrs string
	firstAttrs    string
	secondAttrs   string
	input         string
	wantError     bool
}

func validationSequenceOccurrenceProfiles() []validationSequenceOccurrenceProfile {
	return []validationSequenceOccurrenceProfile{
		{
			name:       "numeric",
			firstType:  "xs:integer",
			secondType: "xs:decimal",
			values: validationSequenceOccurrenceValues{
				first: "1", first2: "2", first3: "3", second: "3.25", second2: "4.50",
			},
		},
		{
			name:       "Boolean",
			firstType:  "xs:boolean",
			secondType: "r:Flag",
			extra:      `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`,
			values: validationSequenceOccurrenceValues{
				first: "true", first2: "0", first3: "1", second: "false", second2: "0",
			},
		},
	}
}

func validationSequenceOccurrenceCases() []validationSequenceOccurrenceCase {
	return []validationSequenceOccurrenceCase{
		{
			name:          "optional outer sequence",
			sequenceAttrs: ` minOccurs="0"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"/>`,
		},
		{
			name:       "optional child",
			firstAttrs: ` minOccurs="0"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><second xmlns="">{{second}}</second></root>`,
		},
		{
			name:       "finite repeated child",
			firstAttrs: ` maxOccurs="2"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><first xmlns="">{{first}}</first><first xmlns="">{{first2}}</first><second xmlns="">{{second}}</second></root>`,
		},
		{
			name:       "unbounded child",
			firstAttrs: ` maxOccurs="unbounded"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><first xmlns="">{{first}}</first><first xmlns="">{{first2}}</first><first xmlns="">{{first3}}</first><second xmlns="">{{second}}</second></root>`,
		},
		{
			name:          "repeated outer sequence",
			sequenceAttrs: ` maxOccurs="2"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"><first xmlns="">{{first}}</first><second xmlns="">{{second}}</second><first xmlns="">{{first3}}</first><second xmlns="">{{second2}}</second></root>`,
		},
		{
			name:          "unbounded outer sequence",
			sequenceAttrs: ` maxOccurs="unbounded"`,
			input:         `<root xmlns="` + validationSequenceNamespace + `"><first xmlns="">{{first}}</first><second xmlns="">{{second}}</second><first xmlns="">{{first3}}</first><second xmlns="">{{second2}}</second></root>`,
		},
		{
			name:       "child exact range above uint64",
			firstAttrs: ` minOccurs="18446744073709551616" maxOccurs="18446744073709551616"`,
			input:      `<root xmlns="` + validationSequenceNamespace + `"><first xmlns="">{{first}}</first><second xmlns="">{{second}}</second></root>`,
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
			input:         `<root xmlns="` + validationSequenceNamespace + `"><first xmlns="">{{first}}</first></root>`,
		},
	}
}

func renderValidationSequenceInput(input string, values validationSequenceOccurrenceValues) string {
	return strings.NewReplacer(
		"{{first}}", values.first,
		"{{first2}}", values.first2,
		"{{first3}}", values.first3,
		"{{second}}", values.second,
		"{{second2}}", values.second2,
	).Replace(input)
}

func validationSequenceOccurrenceSchema(t *testing.T, policy goxsd9.LanguagePolicy, profile validationSequenceOccurrenceProfile, sequenceAttrs, firstAttrs, secondAttrs string) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence` + sequenceAttrs + `>
    <xs:element name="first" type="` + profile.firstType + `"` + firstAttrs + `/>
    <xs:element name="second" type="` + profile.secondType + `"` + secondAttrs + `/>
  </xs:sequence></xs:complexType>` + profile.extra + `
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
}

func TestValidateInstancePreservesCorrelatedAdjacentSequenceCandidates(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.1"
			if policy == goxsd9.Strict10 {
				version = "1.0"
			}
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence minOccurs="1" maxOccurs="4">
    <xs:element name="a" type="xs:integer" minOccurs="1" maxOccurs="4"/>
    <xs:element name="a" type="xs:integer" minOccurs="2" maxOccurs="3"/>
  </xs:sequence></xs:complexType>
</xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, nil, policy)
			input := `<root xmlns="` + validationSequenceNamespace + `"><a xmlns="">1</a><a xmlns="">2</a><a xmlns="">3</a><a xmlns="">4</a><a xmlns="">5</a></root>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
		})
	}
}

func TestValidateInstanceReportsDirectSequenceStructure(t *testing.T) {
	for _, profile := range validationSequenceStructureProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			runValidationSequenceStructureProfile(t, profile)
		})
	}
}

func runValidationSequenceStructureProfile(t *testing.T, profile validationSequenceStructureProfile) {
	t.Helper()
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			runValidationSequenceStructurePolicy(t, profile, policy)
		})
	}
}

func runValidationSequenceStructurePolicy(t *testing.T, profile validationSequenceStructureProfile, policy goxsd9.LanguagePolicy) {
	t.Helper()
	wantLexicalSpec := ""
	if profile.lexicalSpec != nil {
		wantLexicalSpec = profile.lexicalSpec(policy)
	}
	schema := validationSequenceStructureSchema(t, policy, profile)
	before := schema.Components()
	for _, test := range validationSequenceStructureCases() {
		t.Run(test.name, func(t *testing.T) {
			runValidationSequenceStructureCase(t, schema, profile, test, wantLexicalSpec)
		})
	}
	if !reflect.DeepEqual(before, schema.Components()) {
		t.Fatal("sequence validation mutated the completed schema")
	}
}

func runValidationSequenceStructureCase(t *testing.T, schema goxsd9.Schema, profile validationSequenceStructureProfile, test validationSequenceStructureCase, wantLexicalSpec string) {
	t.Helper()
	input := renderValidationSequenceStructureText(test.input, profile)
	wantCode := test.wantCode
	if test.lexical {
		wantCode = profile.lexicalCode
	}
	diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if diagnostic.Class() != test.wantClass || diagnostic.Code() != wantCode {
		t.Fatalf("diagnostic = %s/%q, want %s/%q", diagnostic, diagnostic.Code(), test.wantClass, wantCode)
	}
	marker := renderValidationSequenceStructureText(test.wantMarker, profile)
	if marker != "" && diagnostic.Loc() != validationSequenceMarkerLoc(t, input, marker) {
		t.Fatalf("diagnostic Loc() = %s, want marker %q", diagnostic.Loc(), marker)
	}
	if diagnostic.SpecRef() == "" {
		t.Fatalf("diagnostic SpecRef() is empty")
	}
	if test.lexical && wantLexicalSpec != "" && diagnostic.SpecRef() != wantLexicalSpec {
		t.Fatalf("lexical diagnostic SpecRef() = %q, want %q", diagnostic.SpecRef(), wantLexicalSpec)
	}
	if !test.lexical && diagnostic.Unwrap() == nil {
		t.Fatalf("diagnostic lost its structural cause")
	}
}

type validationSequenceStructureProfile struct {
	name        string
	firstName   string
	secondName  string
	firstType   string
	secondType  string
	firstValue  string
	secondValue string
	invalid     string
	extra       string
	lexicalCode string
	lexicalSpec func(goxsd9.LanguagePolicy) string
}

type validationSequenceStructureCase struct {
	name       string
	input      string
	wantCode   string
	wantClass  goxsd9.FailureClass
	wantMarker string
	lexical    bool
}

func validationSequenceStructureProfiles() []validationSequenceStructureProfile {
	return []validationSequenceStructureProfile{
		{
			name:        "numeric",
			firstName:   "integer",
			secondName:  "decimal",
			firstType:   "xs:integer",
			secondType:  "xs:decimal",
			firstValue:  "1",
			secondValue: "1.25",
			invalid:     "1.0",
			extra:       "2",
			lexicalCode: goxsd9.InvalidIntegerLexicalCode,
		},
		{
			name:        "Boolean",
			firstName:   "first",
			secondName:  "second",
			firstType:   "xs:boolean",
			secondType:  "xs:boolean",
			firstValue:  "true",
			secondValue: "false",
			invalid:     "maybe",
			extra:       "1",
			lexicalCode: goxsd9.InvalidBooleanLexicalCode,
			lexicalSpec: func(policy goxsd9.LanguagePolicy) string {
				if policy == goxsd9.Strict10 {
					return "xsd10-datatypes#boolean-lexical-representation"
				}
				return "xsd11-datatypes#boolean-lexical-mapping"
			},
		},
	}
}

func validationSequenceStructureCases() []validationSequenceStructureCase {
	return []validationSequenceStructureCase{
		{
			name:      "missing required child",
			input:     `<root xmlns="` + validationSequenceNamespace + `"><{{firstName}} xmlns="">{{firstValue}}</{{firstName}}></root>`,
			wantCode:  goxsd9.InvalidInstanceSequenceCode,
			wantClass: goxsd9.FailureInvalid,
		},
		{
			name:       "wrong order",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}><{{firstName}} xmlns="">{{firstValue}}</{{firstName}}></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "<{{secondName}}",
		},
		{
			name:       "wrong namespace",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><{{firstName}} xmlns="urn:other">{{firstValue}}</{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: `<{{firstName}} xmlns="urn:other">`,
		},
		{
			name:       "unexpected extra child",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><{{firstName}} xmlns="">{{firstValue}}</{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}><extra xmlns="">{{extra}}</extra></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "<extra",
		},
		{
			name:       "non-whitespace parent text",
			input:      `<root xmlns="` + validationSequenceNamespace + `">text<{{firstName}} xmlns="">{{firstValue}}</{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "text",
		},
		{
			name:       "nested content",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><{{firstName}} xmlns=""><nested/></{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}></root>`,
			wantCode:   goxsd9.InvalidInstanceSequenceCode,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "<nested",
		},
		{
			name:       "root attribute",
			input:      `<root xmlns="` + validationSequenceNamespace + `" id="1"><{{firstName}} xmlns="">{{firstValue}}</{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}></root>`,
			wantCode:   goxsd9.UnsupportedInstanceValidationCode,
			wantClass:  goxsd9.FailureUnsupported,
			wantMarker: `id="1"`,
		},
		{
			name:       "child attribute",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><{{firstName}} xmlns="" id="1">{{firstValue}}</{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}></root>`,
			wantCode:   goxsd9.UnsupportedInstanceValidationCode,
			wantClass:  goxsd9.FailureUnsupported,
			wantMarker: `id="1"`,
		},
		{
			name:       "invalid lexical value",
			input:      `<root xmlns="` + validationSequenceNamespace + `"><{{firstName}} xmlns="">{{invalid}}</{{firstName}}><{{secondName}} xmlns="">{{secondValue}}</{{secondName}}></root>`,
			wantClass:  goxsd9.FailureInvalid,
			wantMarker: "{{invalid}}",
			lexical:    true,
		},
	}
}

func renderValidationSequenceStructureText(input string, profile validationSequenceStructureProfile) string {
	return strings.NewReplacer(
		"{{firstName}}", profile.firstName,
		"{{secondName}}", profile.secondName,
		"{{firstValue}}", profile.firstValue,
		"{{secondValue}}", profile.secondValue,
		"{{invalid}}", profile.invalid,
		"{{extra}}", profile.extra,
	).Replace(input)
}

func validationSequenceStructureSchema(t *testing.T, policy goxsd9.LanguagePolicy, profile validationSequenceStructureProfile) goxsd9.Schema {
	t.Helper()
	version := "1.1"
	if policy == goxsd9.Strict10 {
		version = "1.0"
	}
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence>
    <xs:element name="` + profile.firstName + `" type="` + profile.firstType + `"/>
    <xs:element name="` + profile.secondName + `" type="` + profile.secondType + `"/>
  </xs:sequence></xs:complexType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy)
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

func TestValidateInstanceRejectsMixedBooleanAndNumericSequence(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.1"
			wantSpec := "xsd11-structures#cvc-elt"
			if policy == goxsd9.Strict10 {
				version = "1.0"
				wantSpec = "xsd10-structures#cvc-elt"
			}
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationSequenceNamespace + `" targetNamespace="` + validationSequenceNamespace + `" version="` + version + `">` +
				`<xs:element name="root" type="r:Root"/><xs:complexType name="Root"><xs:sequence>` +
				`<xs:element name="flag" type="xs:boolean"/><xs:element name="count" type="xs:integer"/>` +
				`</xs:sequence></xs:complexType></xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, nil, policy)
			input := `<root xmlns="` + validationSequenceNamespace + `"><flag xmlns="">true</flag><count xmlns="">1</count></root>`
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode {
				t.Fatalf("diagnostic = %s/%q, want unsupported instance validation", diagnostic, diagnostic.Code())
			}
			if diagnostic.SpecRef() != wantSpec || !errors.Is(diagnostic, goxsd9.ErrUnsupported) || diagnostic.Loc() != validationTestLoc(t, "instance.xml", 1, 1) {
				t.Fatalf("diagnostic evidence = %s/%q/%v, want root-located unsupported", diagnostic.Loc(), diagnostic.SpecRef(), diagnostic)
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
