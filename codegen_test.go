package goxsd9_test

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

	"github.com/goxdra/goxsd9"
)

const (
	codegenPackageNameCode   = "GOXSD9026"
	codegenUnsupportedCode   = "GOXSD9029"
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

func compilePublicGeneratedCode(t *testing.T, source []byte) {
	t.Helper()
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
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile generated source: %v\n%s", err, output)
	}
}
