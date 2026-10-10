package goxsd9_test

import (
	"bytes"
	"context"
	"go/format"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit,funlen // Keep graph, exact facts, output order, and compile evidence together.
func TestGenerateGoGlobalLongScalarsAcrossPolicies(t *testing.T) {
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
  <xs:element name="direct" type="xs:long"/>
  <xs:element name="namedElement" type="r:Named"/>
  <xs:element name="narrowedElement" type="r:Narrowed"/>
  <xs:element name="inheritedElement" type="r:Inherited"/>
  <xs:element name="forwardElement" type="r:Forward"/>
  <xs:element name="includedElement" type="r:Included"/>
  <xs:element name="importedElement" type="o:Imported"/>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Named"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Named"><xs:restriction base="xs:long"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:long"/></xs:simpleType>
  <xs:simpleType name="runtime"><xs:restriction base="xs:long"/></xs:simpleType>
	<xs:simpleType name="Narrowed"><xs:restriction base="xs:long"><xs:minInclusive value="2"/><xs:maxInclusive value="9"/><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
			ordinaryContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:simpleType name="Included"><xs:restriction base="xs:long"/></xs:simpleType>
</xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `">
  <xs:element name="chameleonDirect" type="xs:long"/>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"` + version + `>
  <xs:simpleType name="Imported"><xs:restriction base="xs:long"/></xs:simpleType>
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
			minimumLoc, hasMinimumLoc := bounds.MinInclusiveLoc()
			maximumLoc, hasMaximumLoc := bounds.MaxInclusiveLoc()
			if !hasMinimumLoc || minimumLoc != publicLongMarkerLoc(t, rootContents, `value="2"`) || !hasMaximumLoc || maximumLoc != publicLongMarkerLoc(t, rootContents, `value="9"`) {
				t.Fatalf("Narrowed bound locations = %s/%t, %s/%t", minimumLoc, hasMinimumLoc, maximumLoc, hasMaximumLoc)
			}
			copied := bounds.Bounds()
			copied[0] = goxsd9.IntegerBoundFacet{}
			repeated, ok := narrowed.IntegerBounds()
			if !ok || repeated.Bounds()[0].Value().Canonical() != "2" {
				t.Fatal("mutating copied bounds changed named long facts")
			}
			directComponents := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, "urn:root", "direct"))
			if len(directComponents) != 1 {
				t.Fatalf("direct declarations = %d, want one", len(directComponents))
			}
			direct, ok := directComponents[0].ElementDeclaration()
			if !ok {
				t.Fatal("direct element view is missing")
			}
			directReference, ok := direct.TypeReference()
			if !ok || directReference.Loc() != publicLongMarkerLoc(t, rootContents, `type="xs:long"`) {
				t.Fatalf("direct type reference = %#v/%t, want located built-in long", directReference, ok)
			}
			directBounds, ok := directReference.IntegerBounds()
			if !ok {
				t.Fatal("direct long bounds are missing")
			}
			directMinimum, hasDirectMinimum := directBounds.MinInclusive()
			directMaximum, hasDirectMaximum := directBounds.MaxInclusive()
			if !hasDirectMinimum || directMinimum.Canonical() != "-9223372036854775808" || !hasDirectMaximum || directMaximum.Canonical() != "9223372036854775807" {
				t.Fatalf("direct long bounds = %s/%t, %s/%t", directMinimum, hasDirectMinimum, directMaximum, hasDirectMaximum)
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
				t.Fatalf("repeated long output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated long source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated long source is not complete go/format output:\n%s", first)
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
					t.Fatalf("generated long source is missing %q:\n%s", fragment, first)
				}
			}
			if strings.Contains(source, "int64") || strings.Contains(source, "uint64") || strings.Contains(source, "const ") || strings.Contains(source, "xml:") {
				t.Fatalf("generated long source narrowed or added runtime/value helpers:\n%s", source)
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
					t.Fatalf("generated long declarations do not preserve schema order at %s:\n%s", name, source)
				}
				last = position
			}
			compilePublicGeneratedCode(t, first, `package consumer

import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)

func useLongScalars() {
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

func TestGenerateGoGlobalLongRejectsNonOrdinaryDeclarationsAcrossPolicies(t *testing.T) {
	testGenerateGoBoundedIntegerFlags(t, "long")
}

func TestGenerateGoLongExcludedShapesHaveLocatedUnsupportedDiagnostics(t *testing.T) {
	testGenerateGoBoundedIntegerExclusions(t, []codegenBoundedIntegerExclusion{
		{"inline global", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element>`, `<xs:element name="value"`, ""},
		{"local sequence", `<xs:complexType name="Container"><xs:sequence><xs:element name="value" type="xs:long"/></xs:sequence></xs:complexType>`, `<xs:element name="value"`, ""},
		{"local choice", `<xs:complexType name="Container"><xs:choice><xs:element name="value" type="xs:long"/></xs:choice></xs:complexType>`, `<xs:element name="value"`, ""},
		{"choice reference", `<xs:element name="value" type="xs:long"/><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, `ref="t:value"`, ""},
		{"sequence reference", `<xs:element name="value" type="xs:long"/><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType>`, `ref="t:value"`, ""},
		{"global attribute", `<xs:attribute name="value" type="xs:long"/>`, `<xs:attribute name="value"`, ""},
		{"named final", `<xs:simpleType name="Value" final="restriction"><xs:restriction base="xs:long"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `final="restriction"`},
		{"named union", `<xs:simpleType name="Value"><xs:union memberTypes="xs:long"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `<xs:union`},
	})
}

func publicLongMarkerLoc(t *testing.T, source, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(source, marker)
	if index < 0 {
		t.Fatalf("missing location marker %q", marker)
	}
	before := source[:index]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndex(before, "\n")
	loc, err := goxsd9.NewLoc("root.xsd", line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}
