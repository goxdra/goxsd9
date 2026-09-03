package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeBuildsBoundedOpenAttrsRestrictionAcrossPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.0"},
		{name: "strict10", policy: Strict10, version: "1.1"},
		{name: "strict11", policy: Strict11, version: "1.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := boundedOpenAttrsSchema(test.version, true)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			assertBoundedOpenAttrsFacts(t, schema, root)
		})
	}
}

//nolint:gocognit,funlen // Keep the complete immutable fact and provenance check together.
func assertBoundedOpenAttrsFacts(t *testing.T, schema Schema, root string) {
	t.Helper()
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	if got := components[0].Name().Local(); got != "root" {
		t.Fatalf("component 0 name = %q, want root", got)
	}
	if got := components[1].Name().Local(); got != "OpenAttrs" {
		t.Fatalf("component 1 name = %q, want OpenAttrs", got)
	}
	if got := components[0].ID().Ordinal(); got != 1 {
		t.Fatalf("element ordinal = %d, want 1", got)
	}
	if got := components[1].ID().Ordinal(); got != 2 {
		t.Fatalf("complex type ordinal = %d, want 2", got)
	}
	definition, ok := components[1].ComplexType()
	if !ok {
		t.Fatal("OpenAttrs has no complex type view")
	}
	if definition.Component().ID() != components[1].ID() || definition.ID() != components[1].ID() || definition.Name() != components[1].Name() || definition.Loc() != components[1].Loc() {
		t.Fatal("complex type view did not preserve declaration identity")
	}
	baseName := mustTestQName(t, testXSDNamespace, "anyType")
	if got := definition.Base(); got != baseName {
		t.Fatalf("base = %q, want %q", got, baseName)
	}
	if got := definition.BaseLoc(); got != boundedOpenAttrsTestLoc(root, `base="xs:anyType"`) {
		t.Fatalf("base location = %s, want %s", got, boundedOpenAttrsTestLoc(root, `base="xs:anyType"`))
	}
	if got := definition.Derivation(); got != ComplexTypeDerivationRestriction {
		t.Fatalf("derivation = %q, want restriction", got)
	}
	if got := definition.DerivationLoc(); got != boundedOpenAttrsTestLoc(root, "<xs:restriction") {
		t.Fatalf("derivation location = %s, want %s", got, boundedOpenAttrsTestLoc(root, "<xs:restriction"))
	}
	if got := definition.Particle(); got != nil {
		t.Fatalf("particle = %T, want legal empty-content absence", got)
	}
	baseReference, ok := definition.BaseReference()
	if !ok {
		t.Fatal("base reference is absent")
	}
	if baseReference.Kind() != ComplexTypeReferenceBuiltin || !baseReference.IsBuiltin() || baseReference.IsNamed() {
		t.Fatalf("base reference kind = %q, want builtin", baseReference.Kind())
	}
	if baseReference.Name() != baseName || baseReference.QName() != baseName || baseReference.Loc() != definition.BaseLoc() {
		t.Fatalf("base reference facts = %q/%q/%s, want %q/%q/%s", baseReference.Name(), baseReference.QName(), baseReference.Loc(), baseName, baseName, definition.BaseLoc())
	}
	if componentID, componentIDOK := baseReference.ComponentID(); componentIDOK || !componentID.IsZero() {
		t.Fatalf("built-in base reference identity = %v/%v, want zero/absent", componentID, componentIDOK)
	}
	attribute, attributeOK := definition.AnyAttribute()
	if !attributeOK {
		t.Fatal("bounded wildcard is absent")
		return
	}
	if attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
		t.Fatalf("wildcard facts = %q/%q, want ##other/lax", attribute.Namespace(), attribute.ProcessContents())
	}
	if got := attribute.Loc(); got != boundedOpenAttrsTestLoc(root, "<xs:anyAttribute") {
		t.Fatalf("wildcard location = %s, want %s", got, boundedOpenAttrsTestLoc(root, "<xs:anyAttribute"))
	}
	if got := attribute.NamespaceLoc(); got != boundedOpenAttrsTestLoc(root, `namespace="##other"`) {
		t.Fatalf("wildcard namespace location = %s, want %s", got, boundedOpenAttrsTestLoc(root, `namespace="##other"`))
	}
	if got := attribute.ProcessContentsLoc(); got != boundedOpenAttrsTestLoc(root, `processContents="lax"`) {
		t.Fatalf("wildcard processContents location = %s, want %s", got, boundedOpenAttrsTestLoc(root, `processContents="lax"`))
	}
	element, ok := components[0].Element()
	if !ok {
		t.Fatal("global root element view is absent")
	}
	typeID, ok := element.TypeID()
	if !ok || typeID != definition.ID() {
		t.Fatalf("element type identity = %v/%v, want %v/true", typeID, ok, definition.ID())
	}
	original := schema.Components()
	components[0] = Component{}
	components[1] = Component{}
	if !reflect.DeepEqual(original, schema.Components()) {
		t.Fatal("mutating Components result changed the schema")
	}
	for iteration := 0; iteration < 3; iteration++ {
		found := schema.Find(definition.Name())
		if len(found) != 1 {
			t.Fatal("repeated complex type query returned the wrong count")
		}
		repeated, repeatedOK := found[0].ComplexType()
		if !repeatedOK {
			t.Fatal("repeated complex type query lost the base reference")
		}
		if _, referenceOK := repeated.BaseReference(); !referenceOK {
			t.Fatal("repeated complex type query lost the base reference")
		}
		attribute, attributeOK := definition.AnyAttribute()
		if !attributeOK || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
			t.Fatal("repeated wildcard query changed facts")
		}
	}
}

//nolint:gocognit // Keep the invalid-base matrix and shared diagnostic assertions together.
func TestSchemaBridgeRejectsInvalidBoundedOpenAttrsBases(t *testing.T) {
	tests := []struct {
		name              string
		root              string
		cause             error
		wantRelatedMarker string
	}{
		{
			name:  "missing base",
			root:  boundedOpenAttrsSchemaWithRestriction(`<xs:restriction><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
			cause: errSchemaComplexTypeBaseRequired,
		},
		{
			name:  "wrong builtin base",
			root:  boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:string"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
			cause: errSchemaComplexTypeBaseWrongKind,
		},
		{
			name:  "unknown base",
			root:  boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="t:Missing"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
			cause: errSchemaComplexTypeBaseUnresolved,
		},
		{
			name: "simple named base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:simpleType name="Simple"><xs:restriction base="xs:string"/></xs:simpleType>
  <xs:complexType name="OpenAttrs"><xs:complexContent><xs:restriction base="t:Simple"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`,
			cause:             errSchemaComplexTypeBaseWrongKind,
			wantRelatedMarker: "<xs:simpleType",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("invalid base unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s/%q, want invalid composition", diagnostic.Class(), diagnostic.Code())
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost base cause %v: %v", test.cause, err)
			}
			if diagnostic.SpecRef() != schemaComplexTypeDerivationXSD11SpecRef {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaComplexTypeDerivationXSD11SpecRef)
			}
			if test.wantRelatedMarker != "" && (len(diagnostic.Related()) == 0 || diagnostic.Related()[0] != boundedOpenAttrsTestLoc(test.root, test.wantRelatedMarker)) {
				t.Fatalf("diagnostic related = %v, want named base location", diagnostic.Related())
			}
		})
	}
}

func TestSchemaBridgeKeepsBoundedOpenAttrsUnsupportedFormsDistinct(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "named complex base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Base"><xs:sequence/></xs:complexType>
  <xs:complexType name="OpenAttrs"><xs:complexContent><xs:restriction base="t:Base"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`,
		},
		{
			name: "extension",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:extension base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension>`),
		},
		{
			name: "empty particle",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:sequence/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "local attribute",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:attribute name="local" type="xs:string"/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "attribute group",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:attributeGroup ref="t:attrs"/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "other wildcard",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##any" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "default wildcard",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:anyAttribute/></xs:restriction>`),
		},
		{
			name: "mixed content",
			root: boundedOpenAttrsSchemaWithContentAttributes(`mixed="true"`, `<xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "direct wildcard only",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:complexType name="OpenAttrs"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>
</xs:schema>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("unsupported form unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic lost sentinel: %v", err)
			}
		})
	}
}

func TestSchemaBridgeRejectsBoundedOpenAttrsValidationAndGeneration(t *testing.T) {
	root := boundedOpenAttrsSchema("1.1", true)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	if validationErr == nil {
		t.Fatal("validation unexpectedly accepted openAttrs content")
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation {
		t.Fatalf("validation diagnostic = %s/%q/%q, want instance unsupported", validationDiagnostic.Class(), validationDiagnostic.Code(), validationDiagnostic.Feature())
	}
	if !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceOpenAttrsType) {
		t.Fatalf("validation diagnostic lost openAttrs cause: %v", validationErr)
	}
	if len(validationDiagnostic.Related()) < 3 {
		t.Fatalf("validation related locations = %v, want restriction facts", validationDiagnostic.Related())
	}

	generated, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || generated != nil {
		t.Fatalf("generation result = (%q, %v), want nil output and unsupported error", generated, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen {
		t.Fatalf("generation diagnostic = %s/%q/%q, want codegen unsupported", generationDiagnostic.Class(), generationDiagnostic.Code(), generationDiagnostic.Feature())
	}
	if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("generation diagnostic lost openAttrs cause: %v", generationErr)
	}
}

func boundedOpenAttrsSchema(version string, withElement bool) string {
	element := ""
	if withElement {
		element = `  <xs:element name="root" type="t:OpenAttrs"/>` + "\n"
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `">
` + element + `  <xs:complexType name="OpenAttrs">
    <xs:complexContent>
      <xs:restriction base="xs:anyType">
        <xs:anyAttribute namespace="##other" processContents="lax"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
</xs:schema>`
}

func boundedOpenAttrsSchemaWithRestriction(restriction string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="OpenAttrs"><xs:complexContent>` + restriction + `</xs:complexContent></xs:complexType>
</xs:schema>`
}

func boundedOpenAttrsSchemaWithContentAttributes(attributes, restriction string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="OpenAttrs" ` + attributes + `><xs:complexContent>` + restriction + `</xs:complexContent></xs:complexType>
</xs:schema>`
}

func boundedOpenAttrsTestLoc(root, marker string) Loc {
	index := strings.Index(root, marker)
	if index < 0 {
		panic("bounded openAttrs test marker not found: " + marker)
	}
	line := 1
	column := 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return Loc{source: "root.xsd", line: line, column: column}
}
