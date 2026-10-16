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

//nolint:gocognit,funlen // The graph, copied facets, output order, and compilation form one public contract.
func TestGenerateGoGlobalUnsignedLongScalarsAcrossPolicies(t *testing.T) {
	profiles := []struct {
		name, version, importVersion string
		policy                       goxsd9.LanguagePolicy
	}{
		{"Compatibility", "", "", goxsd9.Compatibility},
		{"Compatibility mixed editions", "1.1", "1.0", goxsd9.Compatibility},
		{"Strict10", "1.0", "", goxsd9.Strict10},
		{"Strict11", "1.1", "", goxsd9.Strict11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			version := ""
			if profile.version != "" {
				version = ` version="` + profile.version + `"`
			}
			importVersion := version
			if profile.importVersion != "" {
				importVersion = ` version="` + profile.importVersion + `"`
			}
			rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"` + version + `>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:unsignedLong"/>
  <xs:element name="namedElement" type="r:Named"/>
  <xs:element name="inheritedElement" type="r:Inherited"/>
  <xs:element name="forwardElement" type="r:Forward"/>
  <xs:element name="includedElement" type="r:Included"/>
  <xs:element name="chameleonNamedElement" type="r:ChameleonNamed"/>
  <xs:element name="importedElement" type="o:Imported"/>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Named"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Named"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="9223372036854775808"/><xs:maxInclusive value="18446744073709551615"/><xs:totalDigits value="20"/><xs:enumeration value="9223372036854775808"/><xs:enumeration value="18446744073709551615"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
  <xs:simpleType name="runtime"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
</xs:schema>`
			ordinaryContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:schema>`
			chameleonContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"><xs:element name="chameleonDirect" type="xs:unsignedLong"/><xs:simpleType name="ChameleonNamed"><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:schema>`
			otherContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"` + importVersion + `><xs:simpleType name="Imported"><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:schema>`
			root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", newParseTestReader(rootContents))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, &publicCodegenResolver{contents: map[string]string{
				"root.xsd": rootContents, "ordinary.xsd": ordinaryContents,
				"chameleon.xsd": chameleonContents, "other.xsd": otherContents,
			}}, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			direct := requirePublicUnsignedLongElement(t, schema, "urn:root", "direct")
			directReference, ok := direct.TypeReference()
			if !ok || directReference.Loc() != publicLongMarkerLoc(t, rootContents, `type="xs:unsignedLong"`) {
				t.Fatalf("direct type reference = %v/%t", directReference, ok)
			}
			directBounds, ok := directReference.IntegerBounds()
			if !ok || !publicUnsignedLongBoundsEqual(directBounds, "0") {
				t.Fatalf("direct intrinsic bounds = %v/%t", directBounds.Bounds(), ok)
			}
			definition := requirePublicUnsignedLongDefinition(t, schema, "urn:root", "Named")
			bounds, ok := definition.IntegerBounds()
			if !ok || !publicUnsignedLongBoundsEqual(bounds, "9223372036854775808") {
				t.Fatalf("Named exact bounds = %v/%t", bounds.Bounds(), ok)
			}
			minimumLoc, hasMinimumLoc := bounds.MinInclusiveLoc()
			maximumLoc, hasMaximumLoc := bounds.MaxInclusiveLoc()
			if !hasMinimumLoc || minimumLoc != publicLongMarkerLoc(t, rootContents, `value="9223372036854775808"`) || !hasMaximumLoc || maximumLoc != publicLongMarkerLoc(t, rootContents, `value="18446744073709551615"`) {
				t.Fatalf("Named bound locations = %s/%t and %s/%t", minimumLoc, hasMinimumLoc, maximumLoc, hasMaximumLoc)
			}
			digits, hasDigits := definition.DigitFacets().TotalDigits()
			digitLoc, hasDigitLoc := definition.DigitFacets().TotalDigitsLoc()
			if !hasDigits || digits.Canonical() != "20" || !hasDigitLoc || digitLoc != publicLongMarkerLoc(t, rootContents, `value="20"`) {
				t.Fatalf("Named totalDigits = %s/%t at %s/%t", digits, hasDigits, digitLoc, hasDigitLoc)
			}
			values := definition.IntegerEnumerationFacets().Values()
			locations := definition.IntegerEnumerationFacets().Locations()
			if len(values) != 2 || values[0].Canonical() != "9223372036854775808" || values[1].Canonical() != "18446744073709551615" || len(locations) != 2 || locations[0] != publicByteFacetElementLoc(t, rootContents, "Named", "enumeration", "9223372036854775808") || locations[1] != publicByteFacetElementLoc(t, rootContents, "Named", "enumeration", "18446744073709551615") {
				t.Fatalf("Named ordered enumerations = %v at %v", values, locations)
			}
			copiedBounds := bounds.Bounds()
			copiedBounds[0] = goxsd9.IntegerBoundFacet{}
			values[0] = goxsd9.StrictInteger{}
			if repeated, found := definition.IntegerBounds(); !found || !publicUnsignedLongBoundsEqual(repeated, "9223372036854775808") || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "9223372036854775808" {
				t.Fatal("mutating copied facets changed schema facts")
			}
			inherited := requirePublicUnsignedLongDefinition(t, schema, "urn:root", "Inherited")
			inheritedBounds, ok := inherited.IntegerBounds()
			if !ok || !publicUnsignedLongBoundsEqual(inheritedBounds, "9223372036854775808") || inherited.IntegerEnumerationFacets().Values()[1].Canonical() != "18446744073709551615" {
				t.Fatal("Inherited lost effective bound or enumeration facts")
			}
			inheritedMinLoc, ok := inheritedBounds.MinInclusiveLoc()
			if !ok || inheritedMinLoc != minimumLoc {
				t.Fatalf("Inherited lost facet provenance: %s/%t", inheritedMinLoc, ok)
			}
			inheritedElement := requirePublicUnsignedLongElement(t, schema, "urn:root", "inheritedElement")
			inheritedRef, ok := inheritedElement.TypeReference()
			if !ok {
				t.Fatal("Inherited element type reference missing")
			}
			inheritedRefBounds, ok := inheritedRef.IntegerBounds()
			if !ok || !publicUnsignedLongBoundsEqual(inheritedRefBounds, "9223372036854775808") {
				t.Fatal("Inherited element reference lost copied effective bounds")
			}
			for _, name := range []string{"Later", "Forward", "Included", "ChameleonNamed"} {
				if _, ok := requirePublicUnsignedLongDefinition(t, schema, "urn:root", name).IntegerBounds(); !ok {
					t.Fatalf("%s has no bounds", name)
				}
			}
			if _, ok := requirePublicUnsignedLongDefinition(t, schema, "urn:other", "Imported").IntegerBounds(); !ok {
				t.Fatal("Imported has no bounds")
			}
			first, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := goxsd9.GenerateGo(schema, "generated")
			if err != nil || !bytes.Equal(first, second) {
				t.Fatalf("repeated generation differs or failed: %v", err)
			}
			formatted, err := format.Source(first)
			if err != nil || !bytes.Equal(first, formatted) {
				t.Fatalf("output is not complete formatted Go: %v\n%s", err, first)
			}
			source := string(first)
			for _, fragment := range []string{
				"type Direct struct {\n\tValue Runtime2.StrictInteger\n}",
				"type NamedElement struct {\n\tValue Named\n}",
				"type InheritedElement struct {\n\tValue Inherited\n}",
				"type ImportedElement struct {\n\tValue Imported\n}",
				"type ChameleonDirect struct {\n\tValue Runtime2.StrictInteger\n}",
				"type Named struct {\n\tValue Runtime2.StrictInteger\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("missing %q:\n%s", fragment, first)
				}
			}
			ordered := []string{"Direct", "NamedElement", "InheritedElement", "ForwardElement", "IncludedElement", "ChameleonNamedElement", "ImportedElement", "Inherited", "Forward", "Named", "Later", "Runtime", "Included", "ChameleonDirect", "ChameleonNamed", "Imported"}
			last := -1
			for _, name := range ordered {
				declaration := "type " + name + " "
				if strings.Count(source, declaration) != 1 || strings.Index(source, declaration) <= last {
					t.Fatalf("%s missing, repeated, or out of order:\n%s", name, source)
				}
				last = strings.Index(source, declaration)
			}
			if strings.Contains(source, "uint64") || strings.Contains(source, "int64") || strings.Contains(source, "UnmarshalXML") || strings.Contains(source, "xml:") {
				t.Fatalf("generation narrowed values or added XML helpers:\n%s", source)
			}
			compilePublicGeneratedCode(t, first, `package consumer
import (
	generated "generated.test"
	runtime "github.com/goxdra/goxsd9"
)
func useUnsignedLongScalars() {
	var direct generated.Direct
	var named generated.NamedElement
	var inherited generated.InheritedElement
	var imported generated.ImportedElement
	var chameleon generated.ChameleonDirect
	var _ runtime.StrictInteger = direct.Value
	var _ generated.Named = named.Value
	var _ generated.Inherited = inherited.Value
	var _ generated.Imported = imported.Value
	var _ runtime.StrictInteger = chameleon.Value
}
`)
		})
	}
}

func requirePublicUnsignedLongElement(t *testing.T, schema goxsd9.Schema, namespace, name string) goxsd9.ElementDeclaration {
	t.Helper()
	components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, namespace, name))
	if len(components) != 1 {
		t.Fatalf("%s elements = %d, want one", name, len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("%s has no element view", name)
	}
	return declaration
}

func requirePublicUnsignedLongDefinition(t *testing.T, schema goxsd9.Schema, namespace, name string) goxsd9.SimpleTypeDefinition {
	t.Helper()
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, mustPublicNonNegativeIntegerQName(t, namespace, name))
	if len(components) != 1 {
		t.Fatalf("%s definitions = %d, want one", name, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple-type view", name)
	}
	return definition
}

func publicUnsignedLongBoundsEqual(bounds goxsd9.IntegerBoundFacets, lower string) bool {
	minimum, hasMinimum := bounds.MinInclusive()
	maximum, hasMaximum := bounds.MaxInclusive()
	return hasMinimum && minimum.Canonical() == lower && hasMaximum && maximum.Canonical() == "18446744073709551615"
}

func TestGenerateGoGlobalUnsignedLongRejectsNonOrdinaryDeclarationsAcrossPolicies(t *testing.T) {
	testGenerateGoBoundedIntegerFlags(t, "unsignedLong")
}

func TestGenerateGoUnsignedLongExcludedShapesHaveLocatedDiagnostics(t *testing.T) {
	testGenerateGoBoundedIntegerExclusions(t, []codegenBoundedIntegerExclusion{
		{"inline global", `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:element>`, `<xs:element name="value"`, ""},
		{"local sequence", `<xs:complexType name="Container"><xs:sequence><xs:element name="value" type="xs:unsignedLong"/></xs:sequence></xs:complexType>`, `<xs:element name="value"`, ""},
		{"named local sequence", `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:complexType name="Container"><xs:sequence><xs:element name="value" type="t:Value"/></xs:sequence></xs:complexType>`, `<xs:element name="value"`, ""},
		{"local choice", `<xs:complexType name="Container"><xs:choice><xs:element name="value" type="xs:unsignedLong"/></xs:choice></xs:complexType>`, `<xs:element name="value"`, ""},
		{"named local choice", `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:complexType name="Container"><xs:choice><xs:element name="value" type="t:Value"/></xs:choice></xs:complexType>`, `<xs:element name="value"`, ""},
		{"choice ref", `<xs:element name="value" type="xs:unsignedLong"/><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, `ref="t:value"`, ""},
		{"named choice ref", `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:complexType name="Container"><xs:choice><xs:element ref="t:value"/></xs:choice></xs:complexType>`, `ref="t:value"`, ""},
		{"sequence ref", `<xs:element name="value" type="xs:unsignedLong"/><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType>`, `ref="t:value"`, ""},
		{"named sequence ref", `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:complexType name="Container"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:complexType>`, `ref="t:value"`, ""},
		{"attribute", `<xs:attribute name="value" type="xs:unsignedLong"/>`, `<xs:attribute name="value"`, ""},
		{"named attribute", `<xs:attribute name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`, `<xs:attribute name="value"`, ""},
		{"named final", `<xs:simpleType name="Value" final="restriction"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `final="restriction"`},
		{"named union", `<xs:simpleType name="Value"><xs:union memberTypes="xs:unsignedLong"/></xs:simpleType>`, `<xs:simpleType name="Value"`, `<xs:union`},
	})
}

func TestGenerateGoUnsignedLongInlineLocalStopsAtParseBoundary(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"><xs:complexType name="Container"><xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:element></xs:sequence></xs:complexType></xs:schema>`
		schema, err := parsePublicNonNegativeIntegerSchema(t, root, policy)
		if len(schema.Components()) != 0 || err == nil {
			t.Fatalf("ParseSchemaWithPolicy(%s) = (%v, %v), want nil schema and diagnostic", policy, schema, err)
		}
		diagnostic := publicNonNegativeIntegerDiagnostic(t, err)
		if diagnostic.Code() != "XSD3003" || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Loc() != publicLongMarkerLoc(t, root, `<xs:simpleType>`) {
			t.Fatalf("ParseSchemaWithPolicy(%s) diagnostic = %s", policy, diagnostic)
		}
	}
}

//nolint:gocognit // The same lexical values must be checked under every policy.
func TestGenerateGoUnsignedLongLexicalEditionPolicy(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        goxsd9.LanguagePolicy
		accepted      bool
	}{
		{"Compatibility", "", goxsd9.Compatibility, true},
		{"Strict10", "1.0", goxsd9.Strict10, false},
		{"Strict11", "1.1", goxsd9.Strict11, true},
	} {
		for _, lexical := range []struct{ value, canonical string }{{"-0", "0"}, {"+18446744073709551615", "18446744073709551615"}} {
			t.Run(profile.name+"/"+lexical.value, func(t *testing.T) {
				version := ""
				if profile.version != "" {
					version = ` version="` + profile.version + `"`
				}
				root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:test"` + version + `><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"><xs:enumeration value="` + lexical.value + `"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := parsePublicNonNegativeIntegerSchema(t, root, profile.policy)
				if !profile.accepted {
					if len(schema.Components()) != 0 || err == nil {
						t.Fatalf("Strict10 parsed signed unsignedLong enumeration: %v", err)
					}
					if diagnostic := publicNonNegativeIntegerDiagnostic(t, err); diagnostic.Loc() != publicLongMarkerLoc(t, root, `<xs:enumeration`) {
						t.Fatalf("Strict10 diagnostic location = %s", diagnostic.Loc())
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				definition := requirePublicUnsignedLongDefinition(t, schema, "urn:test", "Value")
				if got := definition.IntegerEnumerationFacets().Values(); len(got) != 1 || got[0].Canonical() != lexical.canonical {
					t.Fatalf("unsignedLong enumeration = %v", got)
				}
				output, err := goxsd9.GenerateGo(schema, "generated")
				if err != nil || output == nil {
					t.Fatalf("GenerateGo = (%q, %v)", output, err)
				}
			})
		}
	}
}

func TestGenerateGoUnsignedLongInvalidFacetHasNoSchema(t *testing.T) {
	for _, profile := range []struct {
		name, version, specPrefix string
		policy                    goxsd9.LanguagePolicy
	}{
		{"Compatibility", "1.1", "xsd11-", goxsd9.Compatibility},
		{"Strict10", "1.0", "xsd10-", goxsd9.Strict10},
		{"Strict11", "1.1", "xsd11-", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" version="` + profile.version + `"><xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="18446744073709551616"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := parsePublicNonNegativeIntegerSchema(t, root, profile.policy)
			if len(schema.Components()) != 0 || err == nil {
				t.Fatalf("ParseSchemaWithPolicy = (%v, %v), want nil schema and error", schema, err)
			}
			diagnostic := publicNonNegativeIntegerDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidBoundRestrictionCode || diagnostic.Loc() != publicLongMarkerLoc(t, root, `value="18446744073709551616"`) || !strings.HasPrefix(diagnostic.SpecRef(), profile.specPrefix) || diagnostic.Unwrap() == nil || errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("invalid bound diagnostic = %s", diagnostic)
			}
		})
	}
}
