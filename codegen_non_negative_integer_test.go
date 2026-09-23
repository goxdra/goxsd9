package goxsd9_test

import (
	"bytes"
	"context"
	"errors"
	"go/format"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit,funlen // Keep the cross-policy graph, naming, output, and compile checks together.
func TestGenerateGoGlobalNonNegativeIntegerScalarsAcrossPolicies(t *testing.T) {
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
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:nonNegativeInteger"/>
  <xs:element name="namedElement" type="r:Named"/>
  <xs:element name="narrowedElement" type="r:Narrowed"/>
  <xs:element name="inheritedElement" type="r:Inherited"/>
  <xs:element name="forwardElement" type="r:Forward"/>
  <xs:element name="includedElement" type="r:Included"/>
  <xs:element name="importedElement" type="o:Imported"/>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Named"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
  <xs:simpleType name="runtime"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
	<xs:simpleType name="Narrowed"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="2"/><xs:maxInclusive value="9"/><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
			ordinaryContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:simpleType name="Included"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
</xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `">
  <xs:element name="chameleonDirect" type="xs:nonNegativeInteger"/>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"` + version + `>
  <xs:simpleType name="Imported"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>
</xs:schema>`
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{
				contents: map[string]string{
					"ordinary.xsd":  ordinaryContents,
					"chameleon.xsd": chameleonContents,
					"other.xsd":     otherContents,
				},
			}, test.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			narrowedTypes := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, mustPublicNonNegativeIntegerQName(t, "urn:root", "Narrowed"))
			if len(narrowedTypes) != 1 {
				t.Fatalf("Narrowed definitions = %d, want one", len(narrowedTypes))
			}
			narrowed, ok := narrowedTypes[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("Narrowed definition view is missing")
			}
			bounds, ok := narrowed.IntegerBounds()
			if !ok {
				t.Fatal("Narrowed integer bounds are missing")
			}
			minimum, hasMinimum := bounds.MinInclusive()
			maximum, hasMaximum := bounds.MaxInclusive()
			if !hasMinimum || minimum.Canonical() != "2" || !hasMaximum || maximum.Canonical() != "9" {
				t.Fatalf("Narrowed bounds = %q/%t, %q/%t, want 2/true and 9/true", minimum.Canonical(), hasMinimum, maximum.Canonical(), hasMaximum)
			}
			totalDigits, hasTotalDigits := narrowed.DigitFacets().TotalDigits()
			if !hasTotalDigits || totalDigits.Canonical() != "2" {
				t.Fatalf("Narrowed totalDigits = %q/%t, want 2/true", totalDigits.Canonical(), hasTotalDigits)
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
				t.Fatalf("repeated nonNegativeInteger output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated nonNegativeInteger source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated nonNegativeInteger source is not complete go/format output:\n%s", first)
			}

			source := string(first)
			for _, fragment := range []string{
				`import Runtime2 "github.com/goxdra/goxsd9"`,
				"type Direct struct {\n\tValue Runtime2.StrictInteger\n}",
				"type NamedElement struct {\n\tValue Named\n}",
				"type NarrowedElement struct {\n\tValue Narrowed\n}",
				"type InheritedElement struct {\n\tValue Inherited\n}",
				"type ForwardElement struct {\n\tValue Forward\n}",
				"type IncludedElement struct {\n\tValue Included\n}",
				"type ImportedElement struct {\n\tValue Imported\n}",
				"type ChameleonDirect struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Inherited struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Forward struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Named struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Later struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Runtime struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Narrowed struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Included struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Imported struct {\n\tValue Runtime2.StrictInteger\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("generated nonNegativeInteger source is missing %q:\n%s", fragment, first)
				}
			}
			if strings.Contains(source, "int64") || strings.Contains(source, "uint64") || strings.Contains(source, "const ") || strings.Contains(source, "xml:") {
				t.Fatalf("generated nonNegativeInteger source narrowed or added runtime/value helpers:\n%s", source)
			}
			if strings.Count(source, `"github.com/goxdra/goxsd9"`) != 1 {
				t.Fatalf("runtime import count = %d, want one:\n%s", strings.Count(source, `"github.com/goxdra/goxsd9"`), source)
			}

			orderedNames := []string{
				"Direct", "NamedElement", "NarrowedElement", "InheritedElement", "ForwardElement", "IncludedElement", "ImportedElement",
				"Inherited", "Forward", "Named", "Later", "Runtime", "Narrowed", "Included", "ChameleonDirect", "Imported",
			}
			last := -1
			for _, name := range orderedNames {
				position := strings.Index(source, "type "+name+" ")
				if position <= last {
					t.Fatalf("generated nonNegativeInteger declarations do not preserve schema order at %s:\n%s", name, source)
				}
				last = position
			}
			compilePublicGeneratedCode(t, first, `package consumer

import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)

func useNonNegativeIntegerScalars() {
	var direct generated.Direct
	var named generated.NamedElement
	var narrowed generated.NarrowedElement
	var inherited generated.InheritedElement
	var forward generated.ForwardElement
	var included generated.IncludedElement
	var imported generated.ImportedElement
	var chameleon generated.ChameleonDirect
	var _ runtime.StrictInteger = direct.Value
	var _ generated.Named = named.Value
	var _ generated.Narrowed = narrowed.Value
	var _ generated.Inherited = inherited.Value
	var _ generated.Forward = forward.Value
	var _ generated.Included = included.Value
	var _ generated.Imported = imported.Value
	var _ runtime.StrictInteger = chameleon.Value
}
`)
		})
	}
}

//nolint:gocognit,funlen // Keep the named nonNegativeInteger phase gates and location evidence together.
func TestGenerateGoNamedNonNegativeIntegerGatesAcrossPolicies(t *testing.T) {
	tests := []struct {
		name        string
		typeBody    string
		withElement bool
		wantRelated string
	}{
		{
			name:        "final",
			typeBody:    `<xs:simpleType name="Value" final="restriction"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`,
			withElement: true,
			wantRelated: "final=\"restriction\"",
		},
		{
			name:        "variety",
			typeBody:    `<xs:simpleType name="Value"><xs:union memberTypes="xs:nonNegativeInteger"/></xs:simpleType>`,
			withElement: false,
			wantRelated: "<xs:union",
		},
		{
			name:        "effective-facet",
			typeBody:    `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>`,
			withElement: false,
			wantRelated: `base="xs:unsignedLong"`,
		},
	}
	for _, policy := range []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1"},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				version := ""
				if policy.version != "" {
					version = ` version="` + policy.version + `"`
				}
				element := ""
				if test.withElement {
					element = `<xs:element name="value" type="t:Value"/>`
				}
				root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"` + version + `>` + element + test.typeBody + `</xs:schema>`
				schema, err := parsePublicNonNegativeIntegerSchema(t, root, policy.policy)
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				var declaration goxsd9.ElementDeclaration
				if test.withElement {
					components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, "urn:test", "value"))
					if len(components) != 1 {
						t.Fatalf("value declarations = %d, want one", len(components))
					}
					var declarationOK bool
					declaration, declarationOK = components[0].ElementDeclaration()
					if !declarationOK {
						t.Fatal("value declaration view is missing")
					}
				}
				typeComponents := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, mustPublicNonNegativeIntegerQName(t, "urn:test", "Value"))
				if len(typeComponents) != 1 {
					t.Fatalf("Value definitions = %d, want one", len(typeComponents))
				}
				definition, ok := typeComponents[0].SimpleTypeDefinition()
				if !ok {
					t.Fatal("Value definition view is missing")
				}
				output, generationErr := goxsd9.GenerateGo(schema, "generated")
				if output != nil || generationErr == nil {
					t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", output, generationErr)
				}
				diagnostic := publicNonNegativeIntegerDiagnostic(t, generationErr)
				wantPrimary := definition.Component().Loc()
				if test.withElement {
					wantPrimary = declaration.Loc()
				}
				if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode || diagnostic.Feature() != goxsd9.FeatureCodegen || diagnostic.Loc() != wantPrimary || !errors.Is(generationErr, goxsd9.ErrUnsupported) {
					t.Fatalf("GenerateGo diagnostic = %s, want located unsupported gate", diagnostic)
				}
				wantRelated := definition.FinalLoc()
				if test.name == "variety" {
					wantRelated = definition.VarietyLoc()
				}
				if test.name == "effective-facet" {
					wantRelated = definition.BaseLoc()
				}
				related := false
				for _, location := range diagnostic.Related() {
					if location == wantRelated {
						related = true
						break
					}
				}
				if wantRelated.IsZero() || !related {
					t.Fatalf("GenerateGo related locations = %v, want %s from %s", diagnostic.Related(), wantRelated, test.wantRelated)
				}
			})
		}
	}
}

func parsePublicNonNegativeIntegerSchema(t *testing.T, root string, policy goxsd9.LanguagePolicy) (goxsd9.Schema, error) {
	t.Helper()
	source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(root))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	return goxsd9.ParseSchemaWithPolicy(source, nil, policy)
}

func mustPublicNonNegativeIntegerQName(t *testing.T, namespace, local string) goxsd9.QName {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	return name
}

func publicNonNegativeIntegerDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostic")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v, want diagnostic", err)
	}
	return diagnostic
}
