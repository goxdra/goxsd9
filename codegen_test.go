package goxsd9_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const (
	codegenPackageNameCode   = "GOXSD9026"
	codegenUnsupportedCode   = "GOXSD9029"
	codegenInvariantCode     = "GOXSD9030"
	codegenSchemaInvalidCode = "GOXSD9032"
)

func TestGenerateGoIsDeterministicOwnedAndCompiling(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:element name="count" type="xs:integer"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
  <xs:element name="amount" type="t:Amount"/>
</xs:schema>`)
	before := schema.Components()

	first, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	second, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated code generation differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if len(first) == 0 {
		t.Fatal("GenerateGo returned empty source")
	}
	want := append([]byte(nil), second...)
	first[0] ^= 0xff
	if !bytes.Equal(second, want) {
		t.Fatal("mutating one returned source changed another returned slice")
	}
	third, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo after returned-byte mutation: %v", err)
	}
	if !bytes.Equal(third, want) {
		t.Fatalf("mutating returned source changed a later result:\nwant:\n%s\ngot:\n%s", want, third)
	}
	assertPublicCodegenComponentsUnchanged(t, before, schema.Components())
	compilePublicGeneratedCode(t, third)
}

func TestGenerateGoUsesCollisionResolvedRuntimeAlias(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test">
  <xs:simpleType name="runtime"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`)

	source, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	for _, fragment := range []string{
		"import Runtime2 \"github.com/goxdra/goxsd9\"",
		"type Runtime struct {\n\tValue Runtime2.StrictInteger\n}",
	} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("generated source is missing %q:\n%s", fragment, source)
		}
	}
	compilePublicGeneratedCode(t, source)
}

//nolint:gocognit,funlen // Keep the cross-version boolean generation corpus and consumer checks together.
func TestGenerateGoGlobalBooleanScalarsAcrossPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := ""
			if test.version != "" {
				version = ` version="` + test.version + `"`
			}
			rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"` + version + `>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:boolean"/>
  <xs:element name="named" type="r:Zero"/>
  <xs:element name="inherited" type="r:Derived"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="cross" type="o:Cross"/>
  <xs:simpleType name="Derived"><xs:restriction base="r:Zero"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Base"/></xs:simpleType>
  <xs:simpleType name="Zero"><xs:restriction base="xs:boolean"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:other"` + version + `>
  <xs:simpleType name="Cross"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{
				contents: map[string]string{"other.xsd": otherContents},
			}, test.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}

			first, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo second: %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("repeated boolean output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated boolean source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated boolean source is not complete go/format output:\n%s", first)
			}
			source := string(first)
			if strings.Contains(source, `github.com/goxdra/goxsd9`) || strings.Contains(source, "import ") {
				t.Fatalf("boolean-only output unexpectedly imports the runtime:\n%s", source)
			}
			for _, fragment := range []string{
				"type Direct struct {\n\tValue bool\n}",
				"type Named struct {\n\tValue Zero\n}",
				"type Inherited struct {\n\tValue Derived\n}",
				"type Forward struct {\n\tValue Forward2\n}",
				"type Cross struct {\n\tValue Cross2\n}",
				"type Derived struct {\n\tValue bool\n}",
				"type Forward2 struct {\n\tValue bool\n}",
				"type Zero struct {\n\tValue bool\n}",
				"type Base struct {\n\tValue bool\n}",
				"type Cross2 struct {\n\tValue bool\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("generated boolean source is missing %q:\n%s", fragment, source)
				}
			}
			compilePublicGeneratedCode(t, first, `package consumer

import generated "generated.test"

func useBooleanScalars() {
	var direct generated.Direct
	var named generated.Named
	var inherited generated.Inherited
	var forward generated.Forward
	var cross generated.Cross
	var _ bool = direct.Value
	var _ generated.Zero = named.Value
	var _ generated.Derived = inherited.Value
	var _ generated.Forward2 = forward.Value
	var _ generated.Cross2 = cross.Value
}
`)
		})
	}
}

//nolint:gocognit,funlen // Keep the supported global string graph and consumer gate together.
func TestGenerateGoGlobalStringScalarsAcrossPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := ""
			if test.version != "" {
				version = ` version="` + test.version + `"`
			}
			rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"` + version + `>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:string"/>
  <xs:element name="namedElement" type="r:Named"/>
  <xs:element name="inheritedElement" type="r:Inherited"/>
  <xs:element name="includedElement" type="r:IncludedString"/>
  <xs:element name="importedElement" type="o:ImportedString"/>
  <xs:element name="inlineElement">
    <xs:simpleType>
      <xs:restriction base="xs:string">
        <xs:enumeration value=""/>
        <xs:enumeration value="inline"/>
      </xs:restriction>
    </xs:simpleType>
  </xs:element>
  <xs:simpleType name="Named">
    <xs:restriction base="xs:string">
      <xs:enumeration value="named"/>
      <xs:enumeration value=""/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Named"/></xs:simpleType>
</xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `">
  <xs:simpleType name="IncludedString"><xs:restriction base="xs:string"><xs:enumeration value="included"/></xs:restriction></xs:simpleType>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:other"` + version + `>
  <xs:simpleType name="ImportedString"><xs:restriction base="xs:string"><xs:enumeration value="imported"/></xs:restriction></xs:simpleType>
</xs:schema>`
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{
				contents: map[string]string{
					"chameleon.xsd": chameleonContents,
					"other.xsd":     otherContents,
				},
			}, test.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}

			first, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo second: %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("repeated string output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated string source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated string source is not complete go/format output:\n%s", first)
			}
			source := string(first)
			if strings.Contains(source, `github.com/goxdra/goxsd9`) || strings.Contains(source, "import ") {
				t.Fatalf("string-only output unexpectedly imports the runtime:\n%s", source)
			}
			if strings.Contains(source, "const ") || strings.Contains(source, "xml:") {
				t.Fatalf("string-only output contains runtime validation, constants, or tags:\n%s", source)
			}
			for _, fragment := range []string{
				"type Direct struct {\n\tValue string\n}",
				"type NamedElement struct {\n\tValue Named\n}",
				"type InheritedElement struct {\n\tValue Inherited\n}",
				"type IncludedElement struct {\n\tValue IncludedString\n}",
				"type ImportedElement struct {\n\tValue ImportedString\n}",
				"type InlineElement struct {\n\tValue string\n}",
				"type Named struct {\n\tValue string\n}",
				"type Inherited struct {\n\tValue string\n}",
				"type IncludedString struct {\n\tValue string\n}",
				"type ImportedString struct {\n\tValue string\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("generated string source is missing %q:\n%s", fragment, source)
				}
			}
			orderedNames := []string{"Direct", "NamedElement", "InheritedElement", "IncludedElement", "ImportedElement", "InlineElement", "Named", "Inherited", "IncludedString", "ImportedString"}
			last := -1
			for _, name := range orderedNames {
				position := strings.Index(source, "type "+name+" ")
				if position <= last {
					t.Fatalf("generated declarations do not preserve schema order at %s:\n%s", name, source)
				}
				last = position
			}
			for _, name := range []string{"Direct", "NamedElement", "InheritedElement", "IncludedElement", "ImportedElement", "InlineElement", "Named", "Inherited", "IncludedString", "ImportedString"} {
				if strings.Count(source, "type "+name+" ") != 1 {
					t.Fatalf("generated string source declares %s an unexpected number of times:\n%s", name, source)
				}
			}
			compilePublicGeneratedCode(t, first, `package consumer

import generated "generated.test"

func useStringScalars() {
	var direct generated.Direct
	var named generated.NamedElement
	var inherited generated.InheritedElement
	var included generated.IncludedElement
	var imported generated.ImportedElement
	var inline generated.InlineElement
	var _ string = direct.Value
	var _ generated.Named = named.Value
	var _ generated.Inherited = inherited.Value
	var _ generated.IncludedString = included.Value
	var _ generated.ImportedString = imported.Value
	var _ string = inline.Value
}
`)
		})
	}
}

//nolint:gocognit // Keep token-family negative cases together.
func TestGenerateGoRejectsTokenDerivedGlobalStringFamilies(t *testing.T) {
	for _, typeName := range []string{"token", "NMTOKEN"} {
		t.Run(typeName, func(t *testing.T) {
			schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test"><xs:element name="item" type="xs:`+typeName+`"/></xs:schema>`)
			output, err := goxsd9.GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("global %s result = (%q, %v), want nil output and error", typeName, output, err)
			}
			diagnostic := requirePublicCodegenDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode {
				t.Fatalf("diagnostic = %s, want unsupported codegen diagnostic", diagnostic)
			}
			if diagnostic.SpecRef() != "xsd11-structures#Element_Declaration_details" {
				t.Fatalf("diagnostic spec reference = %q, want xsd11-structures#Element_Declaration_details", diagnostic.SpecRef())
			}
			if diagnostic.Loc().Source() != "root.xsd" || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("diagnostic location/cause = %s/%v, want root.xsd and unsupported", diagnostic.Loc(), err)
			}
		})
	}
}

func TestGenerateGoMixedNumericAndStringScalarsKeepOneRuntimeImport(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test">
  <xs:element name="label" type="xs:string"/>
  <xs:element name="count" type="xs:integer"/>
</xs:schema>`)
	source, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	generated := string(source)
	if got := strings.Count(generated, `"github.com/goxdra/goxsd9"`); got != 1 {
		t.Fatalf("runtime import count = %d, want one:\n%s", got, generated)
	}
	for _, fragment := range []string{
		"type Label struct {\n\tValue string\n}",
		"type Count struct {\n\tValue Runtime.StrictInteger\n}",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("mixed scalar source is missing %q:\n%s", fragment, generated)
		}
	}
	compilePublicGeneratedCode(t, source, `package consumer

import generated "generated.test"

func useMixedScalars() {
	var _ string = generated.Label{}.Value
	var _ = generated.Count{}.Value
}
`)
}

func TestGenerateGoBooleanComponentReservesRuntimeName(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:simpleType name="runtime"><xs:restriction base="xs:boolean"/></xs:simpleType>
  <xs:element name="count" type="xs:integer"/>
</xs:schema>`)

	source, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	for _, fragment := range []string{
		`import Runtime2 "github.com/goxdra/goxsd9"`,
		"type Runtime struct {\n\tValue bool\n}",
		"type Count struct {\n\tValue Runtime2.StrictInteger\n}",
	} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("generated mixed boolean/numeric source is missing %q:\n%s", fragment, source)
		}
	}
	compilePublicGeneratedCode(t, source, `package consumer

import generated "generated.test"

func useMixedScalars() {
	var flag generated.Runtime
	var count generated.Count
	var _ bool = flag.Value
	var _ = count.Value
}
`)
}

func TestGenerateGoSupportsBooleanDirectChoiceAlternative(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test">
  <xs:complexType name="Choice"><xs:choice><xs:element name="flag" type="xs:boolean"/></xs:choice></xs:complexType>
</xs:schema>`)
	source, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	if strings.Contains(string(source), `github.com/goxdra/goxsd9`) || strings.Contains(string(source), "import ") {
		t.Fatalf("Boolean-only choice unexpectedly imports the runtime:\n%s", source)
	}
	if !strings.Contains(string(source), "type Flag struct {\n\tFlag bool\n}") {
		t.Fatalf("generated Boolean choice is missing its bool variant:\n%s", source)
	}
	compilePublicGeneratedCode(t, source, `package consumer

import generated "generated.test"

var _ generated.Choice = generated.Flag{}
`)
}

//nolint:gocognit,funlen // Keep the Boolean direct-choice graph golden together.
func TestGenerateGoDirectBooleanChoiceGraphAcrossPolicies(t *testing.T) {
	profiles := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1"},
	}
	var baseline []byte
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			version := ""
			if profile.version != "" {
				version = ` version="` + profile.version + `"`
			}
			rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"` + version + `>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="choice-root" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="line-item" type="xs:boolean"/>
    <xs:element name="LINE_ITEM" type="r:DerivedFlag"/>
    <xs:element name="forward" type="r:ForwardFlag"/>
    <xs:element name="included" type="r:IncludedFlag"/>
    <xs:element name="imported" type="o:ImportedFlag"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="DerivedFlag"><xs:restriction base="r:BaseFlag"/></xs:simpleType>
  <xs:simpleType name="BaseFlag"><xs:restriction base="xs:boolean"/></xs:simpleType>
  <xs:simpleType name="ForwardFlag"><xs:restriction base="r:ForwardBase"/></xs:simpleType>
  <xs:simpleType name="ForwardBase"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `">
  <xs:simpleType name="IncludedFlag"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:other"` + version + `>
  <xs:simpleType name="ImportedFlag"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{contents: map[string]string{
				"chameleon.xsd": chameleonContents,
				"other.xsd":     otherContents,
			}}, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			before := schema.Components()

			first, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo second: %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("repeated Boolean choice output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			if baseline == nil {
				baseline = append([]byte(nil), first...)
			}
			if !bytes.Equal(first, baseline) {
				t.Fatalf("equivalent Boolean choice schemas differ across policies:\nwant:\n%s\ngot:\n%s", baseline, first)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated Boolean choice source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated Boolean choice source is not complete go/format output:\n%s", first)
			}
			source := string(first)
			if strings.Contains(source, `github.com/goxdra/goxsd9`) || strings.Contains(source, "import ") {
				t.Fatalf("Boolean-only direct choice unexpectedly imports the runtime:\n%s", source)
			}
			for _, fragment := range []string{
				"type Choice interface {\n\tisChoice()\n}",
				"type ChoiceRoot struct {\n\tValue Choice\n}",
				"type LineItem struct {\n\tLineItem bool\n}",
				"type LineItem2 struct {\n\tLineItem2 DerivedFlag\n}",
				"type Forward struct {\n\tForward ForwardFlag\n}",
				"type Included struct {\n\tIncluded IncludedFlag\n}",
				"type Imported struct {\n\tImported ImportedFlag\n}",
				"type DerivedFlag struct {\n\tValue bool\n}",
				"type BaseFlag struct {\n\tValue bool\n}",
				"type ForwardFlag struct {\n\tValue bool\n}",
				"type ForwardBase struct {\n\tValue bool\n}",
				"type IncludedFlag struct {\n\tValue bool\n}",
				"type ImportedFlag struct {\n\tValue bool\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("generated Boolean choice source is missing %q:\n%s", fragment, first)
				}
			}
			for _, name := range []string{"LineItem", "LineItem2", "Forward", "Included", "Imported", "DerivedFlag", "BaseFlag", "ForwardFlag", "ForwardBase", "IncludedFlag", "ImportedFlag", "ChoiceRoot", "Choice"} {
				if strings.Count(source, "type "+name+" ") != 1 {
					t.Fatalf("generated Boolean choice declares %s an unexpected number of times:\n%s", name, source)
				}
			}
			assertPublicCodegenComponentsUnchanged(t, before, schema.Components())
			compilePublicGeneratedCode(t, first, `package consumer

import generated "generated.test"

func useBooleanChoice(value generated.Choice) {
	switch value := value.(type) {
	case generated.LineItem:
		var _ bool = value.LineItem
	case generated.LineItem2:
		var _ generated.DerivedFlag = value.LineItem2
	case generated.Forward:
		var _ generated.ForwardFlag = value.Forward
	case generated.Included:
		var _ generated.IncludedFlag = value.Included
	case generated.Imported:
		var _ generated.ImportedFlag = value.Imported
	}
}

var _ generated.Choice = generated.LineItem{}
var _ generated.ChoiceRoot = generated.ChoiceRoot{Value: generated.LineItem{}}
`)
		})
	}
}

//nolint:gocognit // Keep the policy and unsupported-shape boundary matrix together.
func TestGenerateGoKeepsBooleanChoiceBoundariesUnsupported(t *testing.T) {
	profiles := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		for _, test := range []struct {
			name    string
			body    string
			wantRef string
		}{
			{
				name:    "mixed Boolean and numeric",
				body:    `<xs:complexType name="Choice"><xs:choice><xs:element name="flag" type="xs:boolean"/><xs:element name="count" type="xs:integer"/></xs:choice></xs:complexType>`,
				wantRef: "element-choice",
			},
			{
				name:    "global Boolean reference",
				body:    `<xs:element name="flag" type="xs:boolean"/><xs:complexType name="Choice"><xs:choice><xs:element ref="r:flag"/></xs:choice></xs:complexType>`,
				wantRef: "element-choice",
			},
			{
				name:    "optional outer choice",
				body:    `<xs:complexType name="Choice"><xs:choice minOccurs="0"><xs:element name="flag" type="xs:boolean"/></xs:choice></xs:complexType>`,
				wantRef: "Particle_details",
			},
			{
				name:    "repeated Boolean alternative",
				body:    `<xs:complexType name="Choice"><xs:choice><xs:element name="flag" type="xs:boolean" maxOccurs="2"/></xs:choice></xs:complexType>`,
				wantRef: "Particle_details",
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				version := ""
				if profile.version != "" {
					version = ` version="` + profile.version + `"`
				}
				root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(`<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" xmlns:r="urn:test" targetNamespace="urn:test"`+version+`>`+test.body+`</xs:schema>`))
				if err != nil {
					t.Fatalf("NewResolvedSource: %v", err)
				}
				schema, err := goxsd9.ParseSchemaWithPolicy(root, nil, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				wantRef := "xsd11-structures#" + test.wantRef
				if profile.policy == goxsd9.Strict10 {
					wantRef = "xsd10-structures#" + test.wantRef
				}
				assertPublicUnsupportedCodegen(t, schema, wantRef)
			})
		}
	}
}

//nolint:gocognit // Keep the cross-version generation and external-compile corpus together.
func TestGenerateGoDirectScalarChoiceIsDeterministicFormattedAndExternallyUsable(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="runtime"><xs:choice>
    <xs:element name="line-item" type="xs:integer"/>
    <xs:element name="LINE_ITEM" type="xs:decimal"/>
    <xs:element name="shared" type="o:Shared"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Shared"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:other">
  <xs:simpleType name="Shared"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`

	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{
				contents: map[string]string{"other.xsd": otherContents},
			}, policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}

			first, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo second: %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("repeated direct-choice output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated direct-choice source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated direct-choice source is not complete go/format output:\n%s", first)
			}
			for _, fragment := range []string{
				`import Runtime2 "github.com/goxdra/goxsd9"`,
				"type Runtime interface {\n\tisRuntime()\n}",
				"type LineItem struct {\n\tLineItem Runtime2.StrictInteger\n}",
				"type LineItem2 struct {\n\tLineItem2 Runtime2.StrictDecimal\n}",
				"type Shared3 struct {\n\tShared Shared2\n}",
				"func (Shared3) isRuntime() {}",
				"type Shared struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Shared2 struct {\n\tValue Runtime2.StrictDecimal\n}",
			} {
				if !strings.Contains(string(first), fragment) {
					t.Fatalf("generated direct-choice source is missing %q:\n%s", fragment, first)
				}
			}
			compilePublicGeneratedChoiceCode(t, first)
		})
	}
}

func TestGenerateGoDirectScalarEmptyChoiceNeedsNoRuntimeImport(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test"><xs:complexType name="Choice"><xs:choice/></xs:complexType></xs:schema>`)
	source, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	if strings.Contains(string(source), "import ") {
		t.Fatalf("empty choice generated an unnecessary import:\n%s", source)
	}
	if !strings.Contains(string(source), "type Choice interface {\n\tisChoice()\n}") {
		t.Fatalf("empty choice generated source is missing its interface:\n%s", source)
	}
	compilePublicGeneratedCode(t, source)
}

func TestGenerateGoRejectsEmptyComplexTypeAsUnsupported(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test"><xs:complexType name="Choice"/></xs:schema>`)
	output, err := goxsd9.GenerateGo(schema, "generated")
	if output != nil || err == nil {
		t.Fatalf("empty complex type result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requirePublicCodegenDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode {
		t.Fatalf("diagnostic = %s, want unsupported codegen diagnostic %s", diagnostic, codegenUnsupportedCode)
	}
	if !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic lost unsupported sentinel: %v", err)
	}
}

func TestGenerateGoPreservesInvalidPackageDiagnostic(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`"><xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`)

	output, err := goxsd9.GenerateGo(schema, "bad-name")
	if output != nil || err == nil {
		t.Fatalf("invalid package result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requirePublicCodegenDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != codegenPackageNameCode {
		t.Fatalf("diagnostic = %s, want invalid package diagnostic %s", diagnostic, codegenPackageNameCode)
	}
	if !diagnostic.Loc().IsZero() {
		t.Fatalf("invalid package diagnostic location = %s, want zero location", diagnostic.Loc())
	}
}

func TestGenerateGoRejectsZeroSchema(t *testing.T) {
	output, err := goxsd9.GenerateGo(goxsd9.Schema{}, "generated")
	if output != nil || err == nil {
		t.Fatalf("zero schema result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requirePublicCodegenDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != codegenSchemaInvalidCode {
		t.Fatalf("diagnostic = %s, want invalid schema diagnostic %s", diagnostic, codegenSchemaInvalidCode)
	}
	if !diagnostic.Loc().IsZero() {
		t.Fatalf("zero schema diagnostic location = %s, want zero location", diagnostic.Loc())
	}
}

func TestGenerateGoPreservesUnsupportedComponentDiagnostic(t *testing.T) {
	schema := parsePublicCodegenSchema(t, `<xs:schema xmlns:xs="`+parseTestXSDNamespace+`" targetNamespace="urn:test"><xs:attribute name="amount"/></xs:schema>`)
	assertPublicUnsupportedCodegen(t, schema, "")
}

func assertPublicUnsupportedCodegen(t *testing.T, schema goxsd9.Schema, wantSpec string) {
	t.Helper()
	output, err := goxsd9.GenerateGo(schema, "generated")
	if output != nil || err == nil {
		t.Fatalf("unsupported component result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requirePublicCodegenDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode {
		t.Fatalf("diagnostic = %s, want unsupported codegen diagnostic %s", diagnostic, codegenUnsupportedCode)
	}
	if diagnostic.Feature() != goxsd9.FeatureCodegen || diagnostic.SpecRef() == "" {
		t.Fatalf("diagnostic feature/specification reference = %q/%q, want codegen feature and reference", diagnostic.Feature(), diagnostic.SpecRef())
	}
	if wantSpec != "" && diagnostic.SpecRef() != wantSpec {
		t.Fatalf("diagnostic specification reference = %q, want %q", diagnostic.SpecRef(), wantSpec)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want located root.xsd diagnostic", diagnostic.Loc())
	}
	if !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("unsupported diagnostic lost its classification cause: %v", err)
	}
}

func parsePublicCodegenSchema(t *testing.T, contents string) goxsd9.Schema {
	t.Helper()
	root, err := goxsd9.NewResolvedSource(
		context.Background(),
		"root.xsd",
		newParseTestReader(contents),
	)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	schema, err := goxsd9.ParseSchema(root, nil)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	return schema
}

func requirePublicCodegenDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T does not contain a Diagnostic: %v", err, err)
	}
	return diagnostic
}

func assertPublicCodegenComponentsUnchanged(t *testing.T, before, after []goxsd9.Component) {
	t.Helper()
	if len(after) != len(before) {
		t.Fatalf("schema component count after byte mutation = %d, want %d", len(after), len(before))
	}
	for index := range before {
		if after[index].ID() != before[index].ID() ||
			after[index].Kind() != before[index].Kind() ||
			after[index].Name() != before[index].Name() ||
			after[index].Loc() != before[index].Loc() {
			t.Fatalf("schema component %d changed after byte mutation: got %#v, want %#v", index, after[index], before[index])
		}
	}
}

func compilePublicGeneratedCode(t *testing.T, source []byte, consumerSources ...string) {
	t.Helper()
	if len(consumerSources) > 1 {
		t.Fatal("compilePublicGeneratedCode accepts at most one consumer source")
	}
	moduleRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	temporary := t.TempDir()
	goMod := fmt.Sprintf("module generated.test\n\ngo 1.26.0\n\nrequire github.com/goxdra/goxsd9 v0.0.0\n\nreplace github.com/goxdra/goxsd9 => %s\n", moduleRoot)
	writeErr := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(goMod), 0o600)
	if writeErr != nil {
		t.Fatalf("write generated go.mod: %v", writeErr)
	}
	writeErr = os.WriteFile(filepath.Join(temporary, "generated.go"), source, 0o600)
	if writeErr != nil {
		t.Fatalf("write generated.go: %v", writeErr)
	}
	if len(consumerSources) == 1 {
		consumerDirectory := filepath.Join(temporary, "consumer")
		if writeErr := os.Mkdir(consumerDirectory, 0o700); writeErr != nil {
			t.Fatalf("create generated consumer directory: %v", writeErr)
		}
		if writeErr := os.WriteFile(filepath.Join(consumerDirectory, "consumer.go"), []byte(consumerSources[0]), 0o600); writeErr != nil {
			t.Fatalf("write generated consumer: %v", writeErr)
		}
	}
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile generated source: %v\n%s", err, output)
	}
}

func compilePublicGeneratedChoiceCode(t *testing.T, source []byte) {
	t.Helper()
	moduleRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	temporary := t.TempDir()
	goMod := fmt.Sprintf("module generated.test\n\ngo 1.26.0\n\nrequire github.com/goxdra/goxsd9 v0.0.0\n\nreplace github.com/goxdra/goxsd9 => %s\n", moduleRoot)
	writeErr := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(goMod), 0o600)
	if writeErr != nil {
		t.Fatalf("write generated choice go.mod: %v", writeErr)
	}
	writeErr = os.WriteFile(filepath.Join(temporary, "generated.go"), source, 0o600)
	if writeErr != nil {
		t.Fatalf("write generated choice source: %v", writeErr)
	}
	consumer := `package consumer

import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)

func selectChoice(choice generated.Runtime) {
	switch value := choice.(type) {
	case generated.LineItem:
		var _ runtime.StrictInteger = value.LineItem
	case generated.LineItem2:
		var _ runtime.StrictDecimal = value.LineItem2
	case generated.Shared3:
		var _ generated.Shared2 = value.Shared
	default:
		panic("unhandled generated choice variant")
	}
}

var _ generated.Runtime = generated.LineItem{}
var _ generated.Runtime = generated.LineItem2{}
var _ generated.Runtime = generated.Shared3{}
`
	consumerDirectory := filepath.Join(temporary, "consumer")
	writeErr = os.Mkdir(consumerDirectory, 0o700)
	if writeErr != nil {
		t.Fatalf("create generated choice consumer directory: %v", writeErr)
	}
	writeErr = os.WriteFile(filepath.Join(consumerDirectory, "consumer.go"), []byte(consumer), 0o600)
	if writeErr != nil {
		t.Fatalf("write generated choice consumer: %v", writeErr)
	}
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile external generated choice source: %v\n%s", err, output)
	}
}

type publicCodegenResolver struct {
	contents map[string]string
}

func (resolver *publicCodegenResolver) Resolve(ctx context.Context, _, schemaLocation string) (goxsd9.ResolvedSource, error) {
	contents, ok := resolver.contents[schemaLocation]
	if !ok {
		return goxsd9.ResolvedSource{}, fmt.Errorf("no public codegen fixture for %q", schemaLocation)
	}
	return goxsd9.NewResolvedSource(ctx, goxsd9.SourceID(schemaLocation), newParseTestReader(contents))
}
