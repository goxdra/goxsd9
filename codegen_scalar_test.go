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
				version: XSDVersion11,
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
				version: XSDVersion11,
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
