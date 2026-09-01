package goxsd9

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodegenScalarSourceIsDeterministicLocatedAndCompiling(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="total" type="xs:integer"/>
  <xs:element name="amountElement" type="r:Amount"/>
  <xs:element name="otherElement" type="o:OtherAmount"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="whole-number"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.0">
  <xs:simpleType name="OtherAmount"><xs:restriction base="xs:decimal"><xs:totalDigits value="9"/></xs:restriction></xs:simpleType>
</xs:schema>`,
		},
	})
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	names := mustScalarCodegenNaming(t, schema)

	first, err := emitCodegen(schema, names)
	if err != nil {
		t.Fatalf("emitCodegen: %v", err)
	}
	second, err := emitCodegen(schema, names)
	if err != nil {
		t.Fatalf("emitCodegen second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated code generation differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	source := string(first)
	for _, fragment := range []string{
		"import Runtime \"github.com/goxdra/goxsd9\"",
		"type Total struct {\n\tValue Runtime.StrictInteger\n}",
		"type AmountElement struct {\n\tValue Amount\n}",
		"type OtherElement struct {\n\tValue OtherAmount\n}",
		"type Amount struct {\n\tValue Runtime.StrictDecimal\n}",
		"type WholeNumber struct {\n\tValue Runtime.StrictInteger\n}",
		"type OtherAmount struct {\n\tValue Runtime.StrictDecimal\n}",
	} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("generated source is missing %q:\n%s", fragment, source)
		}
	}
	if strings.Contains(source, "float32") || strings.Contains(source, "float64") || strings.Contains(source, "int64") {
		t.Fatalf("generated source contains an inexact or fixed-width numeric type:\n%s", source)
	}
	if strings.Index(source, "type AmountElement") > strings.Index(source, "type Amount struct") {
		t.Fatalf("generated declarations do not preserve schema order:\n%s", source)
	}

	compileGeneratedCode(t, first)
}

//nolint:gocognit,funlen // Keep coordinated scalar-plan and schema-fact mutations together.
func TestCodegenScalarSourceRejectsStaleBooleanPlanAtRenderBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(Schema, *codegenSourcePlan)
	}{
		{
			name: "field type",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].fieldType = "Runtime.StrictInteger"
			},
		},
		{
			name: "target primitive",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].target.scalarKind = codegenSourceScalarInteger
			},
		},
		{
			name: "runtime requirement",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.useRuntime = true
			},
		},
		{
			name: "declaration order",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0], plan.declarations[1] = plan.declarations[1], plan.declarations[0]
			},
		},
		{
			name: "reallocated component name",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.names.components[0].identifier = "Changed"
				plan.names.componentByID[plan.names.components[0].id] = "Changed"
				plan.declarations[0].name = "Changed"
			},
		},
		{
			name: "stale schema target",
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				schema.Components()[0].element.declaredType = mustTestQName(t, testXSDNamespace, "integer")
			},
		},
		{
			name: "stale boolean element reference atomic kind",
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				schema.Components()[0].element.typeReference.atomicKind = schemaSimpleTypeAtomicInteger
			},
		},
		{
			name: "stale boolean base facts",
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				schema.Components()[1].simpleType.baseReference.facets = schemaStringFacetVariant{}
			},
		},
		{
			name: "stale boolean base atomic kind",
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				schema.Components()[1].simpleType.baseReference.atomicKind = schemaSimpleTypeAtomicInteger
			},
		},
		{
			name: "stale boolean definition facets",
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				schema.Components()[1].simpleType.facets = schemaStringFacetVariant{}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`" targetNamespace="urn:test">
  <xs:element name="direct" type="xs:boolean"/>
  <xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`, nil)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			plan, err := planCodegenSource(schema, mustScalarCodegenNaming(t, schema))
			if err != nil {
				t.Fatalf("planCodegenSource: %v", err)
			}
			test.mutate(schema, &plan)
			output, err := renderCodegenSource(plan, schema)
			if output != nil || err == nil {
				t.Fatalf("stale boolean plan result = (%q, %v), want nil output and error", output, err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
				t.Fatalf("diagnostic = %s, want internal codegen invariant %s", diagnostic, diagnosticCodegenInvariant)
			}
			if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic location = %s, want a root.xsd location", diagnostic.Loc())
			}
			if !errors.Is(err, errCodegenSchemaInvariant) && !errors.Is(err, errCodegenElementType) {
				t.Fatalf("stale boolean plan error lost its internal cause: %v", err)
			}
		})
	}
}

func TestCodegenScalarSourceRejectsStaleNamedBooleanFacetsForElementAtRenderBoundary(t *testing.T) {
	schema, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`" xmlns:o="urn:other" targetNamespace="urn:test">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="flag" type="o:Flag"/>
</xs:schema>`, map[string]discoveryFixture{
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`,
		},
	})
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	plan, err := planCodegenSource(schema, mustScalarCodegenNaming(t, schema))
	if err != nil {
		t.Fatalf("planCodegenSource: %v", err)
	}
	components := schema.Components()
	if len(components) != 2 || components[0].Kind() != ComponentKindElementDeclaration || components[1].Kind() != ComponentKindSimpleTypeDefinition {
		t.Fatalf("schema components = %#v, want forward element followed by named simple type", components)
	}
	components[1].simpleType.facets = schemaStringFacetVariant{}

	output, err := renderCodegenSource(plan, schema)
	if output != nil || err == nil {
		t.Fatalf("stale named boolean definition result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = %s, want internal codegen invariant %s", diagnostic, diagnosticCodegenInvariant)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want a root.xsd location", diagnostic.Loc())
	}
	if !errors.Is(err, errCodegenSchemaInvariant) {
		t.Fatalf("stale named boolean definition error lost its internal cause: %v", err)
	}
}

func TestCodegenScalarSourceAcceptsCollisionResolvedRuntimeImportAlias(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:simpleType name="runtime"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	names := mustScalarCodegenNaming(t, schema)
	if got, ok := names.componentName(schema.Components()[0].ID()); !ok || got != "Runtime" {
		t.Fatalf("componentName(runtime) = %q, %t, want Runtime, true", got, ok)
	}
	if got, ok := names.importAlias(codegenRuntimeImportPath); !ok || got != "Runtime2" {
		t.Fatalf("runtime import alias = %q, %t, want Runtime2, true", got, ok)
	}

	source, err := emitCodegen(schema, names)
	if err != nil {
		t.Fatalf("emitCodegen: %v", err)
	}
	if source == nil {
		t.Fatal("emitCodegen returned nil source without an error")
	}
	for _, fragment := range []string{
		"import Runtime2 \"github.com/goxdra/goxsd9\"",
		"type Runtime struct {\n\tValue Runtime2.StrictInteger\n}",
	} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("generated source is missing %q:\n%s", fragment, source)
		}
	}

	compileGeneratedCode(t, source)
}

func TestCodegenScalarSourceFormatFailureUsesFormatDiagnostic(t *testing.T) {
	output, err := renderCodegenSource(codegenSourcePlan{packageName: "generated package"})
	if output != nil {
		t.Fatalf("format failure returned partial source: %s", output)
	}
	if err == nil {
		t.Fatal("format failure unexpectedly succeeded")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenFormat {
		t.Fatalf("diagnostic = %s, want internal format diagnostic %s", diagnostic, diagnosticCodegenFormat)
	}
	if !errors.Is(err, errCodegenFormat) {
		t.Fatalf("format diagnostic lost formatting cause: %v", err)
	}
}

func TestCodegenScalarUntypedElementUsesGraphPolicyReference(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		policy  LanguagePolicy
		wantRef string
	}{
		{name: "Strict10", version: "1.0", policy: Strict10, wantRef: "xsd10-structures#Element_Declaration_details"},
		{name: "Strict11", version: "1.1", policy: Strict11, wantRef: "xsd11-structures#Element_Declaration_details"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertCodegenScalarUntypedElement(t, test.version, test.policy, test.wantRef)
		})
	}
}

func assertCodegenScalarUntypedElement(t *testing.T, version string, policy LanguagePolicy, wantRef string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + version + `"><xs:element name="item"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	names := mustScalarCodegenNaming(t, schema)
	output, err := emitCodegen(schema, names)
	if output != nil {
		t.Fatalf("unsupported untyped element returned partial source: %s", output)
	}
	if err == nil {
		t.Fatal("unsupported untyped element unexpectedly generated source")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported {
		t.Fatalf("diagnostic = %s, want located codegen unsupported diagnostic", diagnostic)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want a root.xsd location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != wantRef {
		t.Fatalf("diagnostic specification reference = %q, want %q", diagnostic.SpecRef(), wantRef)
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
		t.Fatalf("unsupported diagnostic lost feature or cause: %v", err)
	}
}

func TestCodegenScalarSourceUsesNamingIdentity(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:simpleType name="type"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="TYPE"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	names := mustScalarCodegenNaming(t, schema)
	if got, ok := names.componentName(schema.Components()[0].ID()); !ok || got != "XType" {
		t.Fatalf("keyword component name = %q, %t, want XType, true", got, ok)
	}
	if got, ok := names.componentName(schema.Components()[1].ID()); !ok || got != "XType2" {
		t.Fatalf("case-fold component name = %q, %t, want XType2, true", got, ok)
	}
	source, err := emitCodegen(schema, names)
	if err != nil {
		t.Fatalf("emitCodegen: %v", err)
	}
	if !strings.Contains(string(source), "type XType struct") || !strings.Contains(string(source), "type XType2 struct") {
		t.Fatalf("generated source did not use allocated names:\n%s", source)
	}
}

func TestCodegenScalarSourceRejectsUnsupportedComponents(t *testing.T) {
	unsupportedSchema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "unsupported.xsd",
		rootLoc: mustTestLoc(t, "unsupported.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindAttributeDeclaration,
			name: mustTestQName(t, "urn:test", "amount"),
			loc:  mustTestLoc(t, "unsupported.xsd", 2, 3),
		}},
	}})
	unsupportedNames := mustScalarCodegenNaming(t, unsupportedSchema)
	output, err := emitCodegen(unsupportedSchema, unsupportedNames)
	if output != nil {
		t.Fatalf("unsupported component returned partial source: %s", output)
	}
	if err == nil {
		t.Fatal("unsupported component unexpectedly generated source")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported {
		t.Fatalf("diagnostic = %s, want codegen unsupported diagnostic", diagnostic)
	}
	if diagnostic.Feature() != FeatureCodegen {
		t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), FeatureCodegen)
	}
	if diagnostic.Loc() != mustTestLoc(t, "unsupported.xsd", 2, 3) {
		t.Fatalf("diagnostic location = %s, want unsupported.xsd:2:3", diagnostic.Loc())
	}
	if diagnostic.SpecRef() == "" || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
		t.Fatalf("unsupported diagnostic lost feature, spec reference, or cause: %v", err)
	}
}

func TestCodegenScalarSourceRejectsZeroSchemaMissingRuntimeAndMismatchedNaming(t *testing.T) {
	validSchema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "valid.xsd",
		rootLoc: mustTestLoc(t, "valid.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindSimpleTypeDefinition,
			name: mustTestQName(t, "urn:test", "amount"),
			loc:  mustTestLoc(t, "valid.xsd", 2, 3),
			simpleType: &schemaSimpleTypeInput{
				base:    mustTestQName(t, testXSDNamespace, "decimal"),
				baseLoc: mustTestLoc(t, "valid.xsd", 2, 27),
			},
		}},
	}})

	missingRuntimeNames, err := newCodegenNaming(codegenNamingInput{
		packageName: "generated",
		schema:      validSchema,
	})
	if err != nil {
		t.Fatalf("newCodegenNaming without runtime import: %v", err)
	}
	output, err := emitCodegen(validSchema, missingRuntimeNames)
	if output != nil || err == nil {
		t.Fatalf("zero naming import result = (%q, %v), want nil output and error", output, err)
	}
	if !errors.Is(err, errInvalidCodegenName) {
		diagnostic := requireDiagnostic(t, err)
		t.Fatalf("missing runtime import diagnostic = %s, want invalid alias validation", diagnostic)
	}

	validNames := mustScalarCodegenNaming(t, validSchema)
	output, err = emitCodegen(Schema{}, validNames)
	if output != nil || err == nil {
		t.Fatalf("zero schema result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticCodegenSchemaInvalid || !errors.Is(err, errCodegenSchemaEmpty) {
		t.Fatalf("zero schema diagnostic = %s, want located invalid schema diagnostic", diagnostic)
	}

	otherSchema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "other.xsd",
		rootLoc: mustTestLoc(t, "other.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindSimpleTypeDefinition,
			name: mustTestQName(t, "urn:test", "other"),
			loc:  mustTestLoc(t, "other.xsd", 2, 3),
			simpleType: &schemaSimpleTypeInput{
				base:    mustTestQName(t, testXSDNamespace, "integer"),
				baseLoc: mustTestLoc(t, "other.xsd", 2, 27),
			},
		}},
	}})
	output, err = emitCodegen(otherSchema, validNames)
	if output != nil || err == nil {
		t.Fatalf("mismatched naming result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic = requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticCodegenNamingInvalid || diagnostic.Loc() != otherSchema.Components()[0].Loc() {
		t.Fatalf("mismatched naming diagnostic = %s, want located invalid alignment diagnostic", diagnostic)
	}
}

func mustScalarCodegenNaming(t *testing.T, schema Schema) codegenNaming {
	t.Helper()
	names, err := newCodegenNaming(codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		importAliases: []codegenImportAliasRequest{{
			identity: codegenRuntimeImportPath,
			alias:    "runtime",
		}},
	})
	if err != nil {
		t.Fatalf("newCodegenNaming: %v", err)
	}
	return names
}

func compileGeneratedCode(t *testing.T, source []byte) {
	t.Helper()
	moduleRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	temporary := t.TempDir()
	goMod := fmt.Sprintf("module generated.test\n\ngo 1.26.0\n\nrequire %s v0.0.0\n\nreplace %s => %s\n", codegenRuntimeImportPath, codegenRuntimeImportPath, moduleRoot)
	writeErr := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(goMod), 0o600)
	if writeErr != nil {
		t.Fatalf("write generated go.mod: %v", writeErr)
	}
	writeErr = os.WriteFile(filepath.Join(temporary, "generated.go"), source, 0o600)
	if writeErr != nil {
		t.Fatalf("write generated.go: %v", writeErr)
	}
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile generated source: %v\n%s", err, output)
	}
}
