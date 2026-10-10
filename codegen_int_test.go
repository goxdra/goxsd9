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
func TestGenerateGoGlobalIntScalarsAcrossPolicies(t *testing.T) {
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
  <xs:element name="direct" type="xs:int"/>
  <xs:element name="namedElement" type="r:Named"/>
  <xs:element name="narrowedElement" type="r:Narrowed"/>
  <xs:element name="inheritedElement" type="r:Inherited"/>
  <xs:element name="forwardElement" type="r:Forward"/>
  <xs:element name="includedElement" type="r:Included"/>
  <xs:element name="chameleonNamedElement" type="r:ChameleonNamed"/>
  <xs:element name="importedElement" type="o:Imported"/>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Named"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Named"><xs:restriction base="xs:int"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:int"/></xs:simpleType>
  <xs:simpleType name="runtime"><xs:restriction base="xs:int"/></xs:simpleType>
	<xs:simpleType name="Narrowed"><xs:restriction base="xs:int"><xs:minInclusive value="2"/><xs:maxInclusive value="9"/><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
			ordinaryContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:simpleType name="Included"><xs:restriction base="xs:int"/></xs:simpleType>
</xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `">
  <xs:element name="chameleonDirect" type="xs:int"/>
  <xs:simpleType name="ChameleonNamed"><xs:restriction base="xs:int"/></xs:simpleType>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"` + version + `>
  <xs:simpleType name="Imported"><xs:restriction base="xs:int"/></xs:simpleType>
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
				t.Fatal("mutating copied bounds changed named int facts")
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
			if !ok || directReference.Loc() != publicLongMarkerLoc(t, rootContents, `type="xs:int"`) {
				t.Fatalf("direct type reference = %#v/%t, want located built-in int", directReference, ok)
			}
			directBounds, ok := directReference.IntegerBounds()
			if !ok {
				t.Fatal("direct int bounds are missing")
			}
			directMinimum, hasDirectMinimum := directBounds.MinInclusive()
			directMaximum, hasDirectMaximum := directBounds.MaxInclusive()
			if !hasDirectMinimum || directMinimum.Canonical() != "-2147483648" || !hasDirectMaximum || directMaximum.Canonical() != "2147483647" {
				t.Fatalf("direct int bounds = %s/%t, %s/%t", directMinimum, hasDirectMinimum, directMaximum, hasDirectMaximum)
			}
			for _, name := range []string{"Named", "Inherited", "Forward", "ChameleonNamed", "Imported"} {
				namespace := "urn:root"
				if name == "Imported" {
					namespace = "urn:other"
				}
				components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, mustPublicNonNegativeIntegerQName(t, namespace, name))
				if len(components) != 1 {
					t.Fatalf("%s definitions = %d, want one", name, len(components))
				}
				definition, ok := components[0].SimpleTypeDefinition()
				if !ok {
					t.Fatalf("%s has no simple type view", name)
				}
				bounds, ok := definition.IntegerBounds()
				if !ok {
					t.Fatalf("%s has no copied integer bounds", name)
				}
				minimum, hasMinimum := bounds.MinInclusive()
				maximum, hasMaximum := bounds.MaxInclusive()
				if !hasMinimum || minimum.Canonical() != "-2147483648" || !hasMaximum || maximum.Canonical() != "2147483647" {
					t.Fatalf("%s bounds = %s/%t, %s/%t", name, minimum, hasMinimum, maximum, hasMaximum)
				}
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
				t.Fatalf("repeated int output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated int source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated int source is not complete go/format output:\n%s", first)
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
				"type ChameleonNamedElement struct {\n\tValue ChameleonNamed\n}",
				"type ImportedElement struct {\n\tValue Imported\n}",
				"type ChameleonDirect struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Inherited struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Forward struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Named struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Later struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Runtime struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Narrowed struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Included struct {\n\tValue Runtime2.StrictInteger\n}",
				"type ChameleonNamed struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Imported struct {\n\tValue Runtime2.StrictInteger\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("generated int source is missing %q:\n%s", fragment, first)
				}
			}
			if strings.Contains(source, "int64") || strings.Contains(source, "uint64") || strings.Contains(source, "const ") || strings.Contains(source, "xml:") {
				t.Fatalf("generated int source narrowed or added runtime/value helpers:\n%s", source)
			}
			if strings.Count(source, `"github.com/goxdra/goxsd9"`) != 1 {
				t.Fatalf("runtime import count = %d, want one:\n%s", strings.Count(source, `"github.com/goxdra/goxsd9"`), source)
			}

			orderedNames := []string{
				"Direct", "NamedElement", "NarrowedElement", "InheritedElement", "ForwardElement", "IncludedElement", "ChameleonNamedElement", "ImportedElement",
				"Inherited", "Forward", "Named", "Later", "Runtime", "Narrowed", "Included", "ChameleonDirect", "ChameleonNamed", "Imported",
			}
			last := -1
			for _, name := range orderedNames {
				declaration := "type " + name + " "
				if strings.Count(source, declaration) != 1 {
					t.Fatalf("generated int declaration %s count = %d, want one", name, strings.Count(source, declaration))
				}
				position := strings.Index(source, declaration)
				if position <= last {
					t.Fatalf("generated int declarations do not preserve schema order at %s:\n%s", name, source)
				}
				last = position
			}
			compilePublicGeneratedCode(t, first, `package consumer

import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)

func useIntScalars() {
	var direct generated.Direct
	var named generated.NamedElement
	var narrowed generated.NarrowedElement
	var inherited generated.InheritedElement
	var forward generated.ForwardElement
	var included generated.IncludedElement
	var chameleonNamed generated.ChameleonNamedElement
	var imported generated.ImportedElement
	var chameleon generated.ChameleonDirect
	var _ runtime.StrictInteger = direct.Value
	var _ generated.Named = named.Value
	var _ generated.Narrowed = narrowed.Value
	var _ generated.Inherited = inherited.Value
	var _ generated.Forward = forward.Value
	var _ generated.Included = included.Value
	var _ generated.ChameleonNamed = chameleonNamed.Value
	var _ generated.Imported = imported.Value
	var _ runtime.StrictInteger = chameleon.Value
}
`)
		})
	}
}

func TestGenerateGoGlobalIntRejectsNonOrdinaryDeclarationsAcrossPolicies(t *testing.T) {
	testGenerateGoBoundedIntegerFlags(t, "int")
}

func TestGenerateGoIntExcludedShapesHaveLocatedUnsupportedDiagnostics(t *testing.T) {
	testGenerateGoBoundedIntegerExclusions(t, []codegenBoundedIntegerExclusion{
		{"inline global", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element>`, `<xs:element name="value"`, ""},
		{"local sequence", `<xs:complexType name="Container"><xs:sequence><xs:element name="value" type="xs:int"/></xs:sequence></xs:complexType>`, `<xs:element name="value"`, ""},
		{"named local sequence", `<xs:simpleType name="Value"><xs:restriction base="xs:int"/></xs:simpleType><xs:complexType name="Container"><xs:sequence><xs:element name="value" type="t:Value"/></xs:sequence></xs:complexType>`, `<xs:element name="value"`, ""},
		{"local choice", `<xs:complexType name="Container"><xs:choice><xs:element name="value" type="xs:int"/></xs:choice></xs:complexType>`, `<xs:element name="value"`, ""},
		{"named local choice", `<xs:simpleType name="Value"><xs:restriction base="xs:int"/></xs:simpleType><xs:complexType name="Container"><xs:choice><xs:element name="value" type="t:Value"/></xs:choice></xs:complexType>`, `<xs:element name="value"`, ""},
		{"choice reference", `<xs:element name="value" type="xs:int"/><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, `ref="t:value"`, ""},
		{"named choice reference", `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:int"/></xs:simpleType><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, `ref="t:value"`, ""},
		{"sequence reference", `<xs:element name="value" type="xs:int"/><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType>`, `ref="t:value"`, ""},
		{"named sequence reference", `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:int"/></xs:simpleType><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType>`, `ref="t:value"`, ""},
		{"global attribute", `<xs:attribute name="value" type="xs:int"/>`, `<xs:attribute name="value"`, ""},
		{"named global attribute", `<xs:attribute name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:int"/></xs:simpleType>`, `<xs:attribute name="value"`, ""},
		{"named final", `<xs:simpleType name="Value" final="restriction"><xs:restriction base="xs:int"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `final="restriction"`},
		{"named union", `<xs:simpleType name="Value"><xs:union memberTypes="xs:int"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `<xs:union`},
		{"short root", `<xs:element name="value" type="xs:short"/>`, `<xs:element name="value"`, ""},
		{"derived short", `<xs:simpleType name="Value"><xs:restriction base="xs:short"/></xs:simpleType>`, `<xs:simpleType name="Value"`, ""},
		{"unsignedLong root", `<xs:element name="value" type="xs:unsignedLong"/>`, `<xs:element name="value"`, ""},
		{"derived unsignedLong", `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`, `<xs:simpleType name="Value"`, ""},
		{"negativeInteger root", `<xs:element name="value" type="xs:negativeInteger"/>`, `<xs:element name="value"`, ""},
		{"derived negativeInteger", `<xs:simpleType name="Value"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, `<xs:simpleType name="Value"`, ""},
		{"nonPositiveInteger root", `<xs:element name="value" type="xs:nonPositiveInteger"/>`, `<xs:element name="value"`, ""},
		{"derived nonPositiveInteger", `<xs:simpleType name="Value"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`, `<xs:simpleType name="Value"`, ""},
		{"positiveInteger root", `<xs:element name="value" type="xs:positiveInteger"/>`, `<xs:element name="value"`, ""},
		{"derived positiveInteger", `<xs:simpleType name="Value"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`, `<xs:simpleType name="Value"`, ""},
	})
}

func TestGenerateGoIntInlineLocalStopsAtParseBoundary(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"><xs:complexType name="Container"><xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element></xs:sequence></xs:complexType></xs:schema>`
		schema, err := parsePublicNonNegativeIntegerSchema(t, root, policy)
		if len(schema.Components()) != 0 || err == nil {
			t.Fatalf("ParseSchemaWithPolicy(%s) = (%v, %v), want nil schema and diagnostic", policy, schema, err)
		}
		diagnostic := publicNonNegativeIntegerDiagnostic(t, err)
		if diagnostic.Code() != "XSD3003" || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Loc() != publicLongMarkerLoc(t, root, `<xs:simpleType>`) {
			t.Fatalf("ParseSchemaWithPolicy(%s) diagnostic = %s, want XSD3003 at inline type", policy, diagnostic)
		}
	}
}
