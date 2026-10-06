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

//nolint:gocognit,funlen // Keep graph, exact facts, output order, and compile evidence together.
func TestGenerateGoGlobalByteScalarsAcrossPolicies(t *testing.T) {
	tests := []struct {
		name          string
		policy        goxsd9.LanguagePolicy
		version       string
		importVersion string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Compatibility mixed editions", policy: goxsd9.Compatibility, version: "1.1", importVersion: "1.0"},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := ""
			if test.version != "" {
				version = ` version="` + test.version + `"`
			}
			importVersion := version
			if test.importVersion != "" {
				importVersion = ` version="` + test.importVersion + `"`
			}
			rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"` + version + `>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:byte"/>
  <xs:element name="namedElement" type="r:Named"/>
  <xs:element name="narrowedElement" type="r:Narrowed"/>
  <xs:element name="inheritedElement" type="r:Inherited"/>
  <xs:element name="forwardElement" type="r:Forward"/>
  <xs:element name="includedElement" type="r:Included"/>
  <xs:element name="importedElement" type="o:Imported"/>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Named"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Named"><xs:restriction base="xs:byte"><xs:minInclusive value="-10"/><xs:maxInclusive value="10"/><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:byte"/></xs:simpleType>
  <xs:simpleType name="runtime"><xs:restriction base="xs:byte"/></xs:simpleType>
	<xs:simpleType name="Narrowed"><xs:restriction base="xs:byte"><xs:minInclusive value="2"/><xs:maxInclusive value="9"/><xs:totalDigits value="2"/><xs:enumeration value="3"/></xs:restriction></xs:simpleType>
</xs:schema>`
			ordinaryContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
	<xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="Included"><xs:restriction base="xs:byte"/></xs:simpleType>
</xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `">
  <xs:element name="chameleonDirect" type="xs:byte"/>
</xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"` + importVersion + `>
  <xs:simpleType name="Imported"><xs:restriction base="xs:byte"/></xs:simpleType>
</xs:schema>`
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{
				contents: map[string]string{
					"root.xsd":      rootContents,
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
			if !hasMinimumLoc || minimumLoc != publicByteMarkerLoc(t, rootContents, `value="2"`) || !hasMaximumLoc || maximumLoc != publicByteMarkerLoc(t, rootContents, `value="9"`) {
				t.Fatalf("Narrowed bound locations = %s/%t, %s/%t", minimumLoc, hasMinimumLoc, maximumLoc, hasMaximumLoc)
			}
			copied := bounds.Bounds()
			copied[0] = goxsd9.IntegerBoundFacet{}
			repeated, ok := narrowed.IntegerBounds()
			if !ok || repeated.Bounds()[0].Value().Canonical() != "2" {
				t.Fatal("mutating copied bounds changed named byte facts")
			}
			enumeration := narrowed.IntegerEnumerationFacets()
			values := enumeration.Values()
			locations := enumeration.Locations()
			if len(values) != 1 || values[0].Canonical() != "3" || len(locations) != 1 || locations[0] != publicByteMarkerLoc(t, rootContents, `<xs:enumeration value="3"`) {
				t.Fatalf("Narrowed enumeration = %v at %v, want 3 at source location", values, locations)
			}
			values[0] = goxsd9.StrictInteger{}
			if narrowed.IntegerEnumerationFacets().Values()[0].Canonical() != "3" {
				t.Fatal("mutating copied enumeration changed named byte facts")
			}
			narrowedElementComponents := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, "urn:root", "narrowedElement"))
			if len(narrowedElementComponents) != 1 {
				t.Fatalf("narrowed element declarations = %d, want one", len(narrowedElementComponents))
			}
			narrowedElement, ok := narrowedElementComponents[0].ElementDeclaration()
			if !ok {
				t.Fatal("narrowed element view is missing")
			}
			narrowedReference, ok := narrowedElement.TypeReference()
			if !ok {
				t.Fatal("narrowed element type reference is missing")
			}
			narrowedReferenceBounds, ok := narrowedReference.IntegerBounds()
			if !ok || narrowedReferenceBounds.Bounds()[0].Value().Canonical() != "2" {
				t.Fatal("narrowed named reference lost effective byte bounds")
			}
			inheritedTypes := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, mustPublicNonNegativeIntegerQName(t, "urn:root", "Inherited"))
			if len(inheritedTypes) != 1 {
				t.Fatalf("Inherited definitions = %d, want one", len(inheritedTypes))
			}
			inherited, ok := inheritedTypes[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("Inherited definition view is missing")
			}
			inheritedBounds, ok := inherited.IntegerBounds()
			if !ok {
				t.Fatal("Inherited bounds are missing")
			}
			inheritedMinimum, hasInheritedMinimum := inheritedBounds.MinInclusive()
			inheritedMaximum, hasInheritedMaximum := inheritedBounds.MaxInclusive()
			inheritedMinimumLoc, hasInheritedMinimumLoc := inheritedBounds.MinInclusiveLoc()
			inheritedMaximumLoc, hasInheritedMaximumLoc := inheritedBounds.MaxInclusiveLoc()
			inheritedDigits, hasInheritedDigits := inherited.DigitFacets().TotalDigits()
			inheritedDigitsLoc, hasInheritedDigitsLoc := inherited.DigitFacets().TotalDigitsLoc()
			if !hasInheritedMinimum || inheritedMinimum.Canonical() != "-10" || !hasInheritedMaximum || inheritedMaximum.Canonical() != "10" ||
				!hasInheritedMinimumLoc || inheritedMinimumLoc != publicByteMarkerLoc(t, rootContents, `value="-10"`) ||
				!hasInheritedMaximumLoc || inheritedMaximumLoc != publicByteMarkerLoc(t, rootContents, `value="10"`) ||
				!hasInheritedDigits || inheritedDigits.Canonical() != "2" || !hasInheritedDigitsLoc || inheritedDigitsLoc != publicByteMarkerLoc(t, rootContents, `value="2"`) {
				t.Fatalf("Inherited effective byte facets lost base values or locations: bounds=%v digits=%v", inheritedBounds.Bounds(), inherited.DigitFacets())
			}
			inheritedElements := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, "urn:root", "inheritedElement"))
			if len(inheritedElements) != 1 {
				t.Fatalf("inheritedElement declarations = %d, want one", len(inheritedElements))
			}
			inheritedElement, ok := inheritedElements[0].ElementDeclaration()
			if !ok {
				t.Fatal("inheritedElement declaration view is missing")
			}
			inheritedReference, ok := inheritedElement.TypeReference()
			if !ok || inheritedReference.Loc() != publicByteMarkerLoc(t, rootContents, `type="r:Inherited"`) {
				t.Fatal("inheritedElement lost its located named reference")
			}
			inheritedReferenceBounds, ok := inheritedReference.IntegerBounds()
			inheritedReferenceFacets := inheritedReferenceBounds.Bounds()
			if !ok || len(inheritedReferenceFacets) != 2 || inheritedReferenceFacets[0].Value().Canonical() != "-10" || inheritedReferenceFacets[1].Value().Canonical() != "10" {
				t.Fatal("inheritedElement lost its effective byte bounds")
			}
			inheritedReferenceDigits, hasInheritedReferenceDigits := inheritedReference.DigitFacets().TotalDigits()
			inheritedReferenceDigitsLoc, hasInheritedReferenceDigitsLoc := inheritedReference.DigitFacets().TotalDigitsLoc()
			if !hasInheritedReferenceDigits || inheritedReferenceDigits.Canonical() != "2" || !hasInheritedReferenceDigitsLoc || inheritedReferenceDigitsLoc != inheritedDigitsLoc {
				t.Fatal("inheritedElement lost its effective byte digit facet provenance")
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
			if !ok || directReference.Loc() != publicByteMarkerLoc(t, rootContents, `type="xs:byte"`) {
				t.Fatalf("direct type reference = %#v/%t, want located built-in byte", directReference, ok)
			}
			directBounds, ok := directReference.IntegerBounds()
			if !ok {
				t.Fatal("direct byte bounds are missing")
			}
			directMinimum, hasDirectMinimum := directBounds.MinInclusive()
			directMaximum, hasDirectMaximum := directBounds.MaxInclusive()
			if !hasDirectMinimum || directMinimum.Canonical() != "-128" || !hasDirectMaximum || directMaximum.Canonical() != "127" {
				t.Fatalf("direct byte bounds = %s/%t, %s/%t", directMinimum, hasDirectMinimum, directMaximum, hasDirectMaximum)
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
				t.Fatalf("repeated byte output differs:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			formatted, err := format.Source(first)
			if err != nil {
				t.Fatalf("format generated byte source: %v\n%s", err, first)
			}
			if !bytes.Equal(first, formatted) {
				t.Fatalf("generated byte source is not complete go/format output:\n%s", first)
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
					t.Fatalf("generated byte source is missing %q:\n%s", fragment, first)
				}
			}
			if strings.Contains(source, "int64") || strings.Contains(source, "uint64") || strings.Contains(source, "const ") || strings.Contains(source, "xml:") || strings.Contains(source, "func ") {
				t.Fatalf("generated byte source narrowed or added runtime/value helpers:\n%s", source)
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
					t.Fatalf("generated byte declarations do not preserve schema order at %s:\n%s", name, source)
				}
				last = position
			}
			compilePublicGeneratedCode(t, first, `package consumer

import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)

func useByteScalars() {
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

//nolint:gocognit // Keep the policy, type form, and declaration-flag matrix together.
func testGenerateGoGlobalBoundedIntegerRejectsNonOrdinaryDeclarationsAcrossPolicies(t *testing.T, scalar string) {
	policies := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
		wantRef string
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility, wantRef: "xsd11-structures#Element_Declaration_details"},
		{name: "Strict10", policy: goxsd9.Strict10, version: "1.0", wantRef: "xsd10-structures#Element_Declaration_details"},
		{name: "Strict11", policy: goxsd9.Strict11, version: "1.1", wantRef: "xsd11-structures#Element_Declaration_details"},
	}
	flags := []struct {
		name      string
		attribute string
		message   string
	}{
		{name: "abstract", attribute: ` abstract="true"`, message: "abstract=true"},
		{name: "nillable", attribute: ` nillable="true"`, message: "nillable=true"},
	}
	for _, policy := range policies {
		for _, flag := range flags {
			for _, shape := range []struct {
				name, typeName, definition string
			}{
				{"direct", "xs:" + scalar, ""},
				{"named", "t:Value", `<xs:simpleType name="Value"><xs:restriction base="xs:` + scalar + `"/></xs:simpleType>`},
			} {
				t.Run(policy.name+"/"+flag.name+"/"+shape.name, func(t *testing.T) {
					version := ""
					if policy.version != "" {
						version = ` version="` + policy.version + `"`
					}
					root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"` + version + `><xs:element name="value" type="` + shape.typeName + `"` + flag.attribute + `/>` + shape.definition + `</xs:schema>`
					schema, err := parsePublicNonNegativeIntegerSchema(t, root, policy.policy)
					if err != nil {
						t.Fatalf("ParseSchemaWithPolicy: %v", err)
					}
					components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, "urn:test", "value"))
					if len(components) != 1 {
						t.Fatalf("value declarations = %d, want one", len(components))
					}
					declaration, ok := components[0].ElementDeclaration()
					if !ok {
						t.Fatal("value declaration view is missing")
					}
					if flag.name == "abstract" && !declaration.IsAbstract() {
						t.Fatal("value declaration lost abstract=true")
					}
					if flag.name == "nillable" && !declaration.IsNillable() {
						t.Fatal("value declaration lost nillable=true")
					}

					output, generationErr := goxsd9.GenerateGo(schema, "generated")
					if output != nil || generationErr == nil {
						t.Fatalf("GenerateGo result = (%q, %v), want unsupported with nil output", output, generationErr)
					}
					diagnostic := publicNonNegativeIntegerDiagnostic(t, generationErr)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode || diagnostic.Feature() != goxsd9.FeatureCodegen || diagnostic.SpecRef() != policy.wantRef || diagnostic.Loc() != declaration.Loc() || !strings.Contains(diagnostic.Message(), flag.message) || !errors.Is(generationErr, goxsd9.ErrUnsupported) {
						t.Fatalf("GenerateGo diagnostic = %s, want unsupported %s at %s with %s", diagnostic, flag.message, declaration.Loc(), policy.wantRef)
					}
				})
			}
		}
	}
}

func TestGenerateGoGlobalByteRejectsNonOrdinaryDeclarationsAcrossPolicies(t *testing.T) {
	testGenerateGoGlobalBoundedIntegerRejectsNonOrdinaryDeclarationsAcrossPolicies(t, "byte")
}

func TestGenerateGoByteExcludedShapesHaveLocatedUnsupportedDiagnostics(t *testing.T) {
	tests := []codegenBoundedIntegerExclusionCase{
		{"inline global", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`, `<xs:element name="value"`, ""},
		{"local sequence", `<xs:complexType name="Container"><xs:sequence><xs:element name="value" type="xs:byte"/></xs:sequence></xs:complexType>`, `<xs:element name="value"`, ""},
		{"named local sequence", `<xs:complexType name="Container"><xs:sequence><xs:element name="value" type="t:Value"/></xs:sequence></xs:complexType><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:element name="value"`, ""},
		{"local choice", `<xs:complexType name="Container"><xs:choice><xs:element name="value" type="xs:byte"/></xs:choice></xs:complexType>`, `<xs:element name="value"`, ""},
		{"named local choice", `<xs:complexType name="Container"><xs:choice><xs:element name="value" type="t:Value"/></xs:choice></xs:complexType><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:element name="value"`, ""},
		{"choice reference", `<xs:element name="value" type="xs:byte"/><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, `ref="t:value"`, ""},
		{"named choice reference", `<xs:element name="value" type="t:Value"/><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`, `ref="t:value"`, ""},
		{"inline choice reference", `<xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType><xs:element name="value"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`, `ref="t:value"`, `<xs:element name="value"`},
		{"sequence reference", `<xs:element name="value" type="xs:byte"/><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType>`, `ref="t:value"`, ""},
		{"named sequence reference", `<xs:element name="value" type="t:Value"/><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`, `ref="t:value"`, ""},
		{"inline sequence reference", `<xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType><xs:element name="value"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element>`, `ref="t:value"`, `<xs:element name="value"`},
		{"global attribute", `<xs:attribute name="value" type="xs:byte"/>`, `<xs:attribute name="value"`, ""},
		{"named global attribute", `<xs:attribute name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:attribute name="value"`, ""},
		{"named final", `<xs:simpleType name="Value" final="restriction"><xs:restriction base="xs:byte"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `final="restriction"`},
		{"named list", `<xs:simpleType name="Value"><xs:list itemType="xs:byte"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `<xs:list`},
		{"named union", `<xs:simpleType name="Value"><xs:union memberTypes="xs:byte"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `<xs:union`},
		{"short direct stays excluded", `<xs:element name="value" type="xs:short"/>`, `<xs:element name="value"`, ""},
		{"short named stays excluded", `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:short"/></xs:simpleType>`, `<xs:element name="value"`, ""},
		{"int direct stays excluded", `<xs:element name="value" type="xs:int"/>`, `<xs:element name="value"`, ""},
		{"int named stays excluded", `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:int"/></xs:simpleType>`, `<xs:element name="value"`, ""},
	}
	testGenerateGoBoundedIntegerExclusions(t, tests)
}

type codegenBoundedIntegerExclusionCase struct {
	name, body, marker, related string
}

//nolint:gocognit // Keep the policy and exclusion diagnostic matrix together.
func testGenerateGoBoundedIntegerExclusions(t *testing.T, tests []codegenBoundedIntegerExclusionCase) {
	for _, profile := range []struct {
		name, version, specPrefix string
		policy                    goxsd9.LanguagePolicy
	}{
		{"Compatibility", "", "xsd11-", goxsd9.Compatibility},
		{"Strict10", ` version="1.0"`, "xsd10-", goxsd9.Strict10},
		{"Strict11", ` version="1.1"`, "xsd11-", goxsd9.Strict11},
	} {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"` + profile.version + `>` + test.body + `</xs:schema>`
				schema, err := parsePublicNonNegativeIntegerSchema(t, root, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				output, err := goxsd9.GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil unsupported output", output, err)
				}
				diagnostic := publicNonNegativeIntegerDiagnostic(t, err)
				index := strings.Index(root, test.marker)
				if index < 0 {
					t.Fatalf("missing location marker %q", test.marker)
				}
				wantLoc, err := goxsd9.NewLoc("root.xsd", 1, index+1)
				if err != nil {
					t.Fatalf("NewLoc: %v", err)
				}
				if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode || diagnostic.Loc() != wantLoc || diagnostic.Unwrap() == nil || !strings.HasPrefix(diagnostic.SpecRef(), profile.specPrefix) {
					t.Fatalf("diagnostic = %s, want GOXSD9029 at %s with a preserved cause", diagnostic, wantLoc)
				}
				if test.related != "" {
					wantRelated := publicByteMarkerLoc(t, root, test.related)
					found := false
					for _, related := range diagnostic.Related() {
						if related == wantRelated {
							found = true
						}
					}
					if !found {
						t.Fatalf("related locations = %v, want %s", diagnostic.Related(), wantRelated)
					}
				}
			})
		}
	}
}

func publicByteMarkerLoc(t *testing.T, source, marker string) goxsd9.Loc {
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
