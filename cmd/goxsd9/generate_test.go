package main

import (
	"bytes"
	"context"
	"errors"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

func TestGenerateCommandMatchesPublicGeneratorForStdoutAndFile(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	contents := schemaDocument(`<xs:element name="count" type="xs:integer"/><xs:simpleType name="amount"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>`)
	writeTestFile(t, schemaPath, contents)
	expected := publicGenerate(t, contents, "generated")

	var firstStdout, firstStderr bytes.Buffer
	args := []string{"generate", "--diagnostics", "json", "--package", "generated", schemaPath}
	if code := runWithInput(args, strings.NewReader(""), &firstStdout, &firstStderr); code != 0 {
		t.Fatalf("stdout generation code = %d, stderr = %q", code, firstStderr.String())
	}
	if firstStderr.Len() != 0 || !bytes.Equal(firstStdout.Bytes(), expected) {
		t.Fatalf("stdout generation = %q, want public output %q", firstStdout.Bytes(), expected)
	}
	assertFormattedGeneratedSource(t, firstStdout.Bytes())

	var secondStdout, secondStderr bytes.Buffer
	if code := runWithInput(args, strings.NewReader(""), &secondStdout, &secondStderr); code != 0 {
		t.Fatalf("repeated stdout generation code = %d, stderr = %q", code, secondStderr.String())
	}
	if !bytes.Equal(firstStdout.Bytes(), secondStdout.Bytes()) || secondStderr.Len() != 0 {
		t.Fatalf("repeated stdout generation changed output or diagnostics")
	}

	outputPath := filepath.Join(directory, "generated.go")
	var fileStdout, fileStderr bytes.Buffer
	fileArgs := []string{"generate", "--package", "generated", "--output", outputPath, schemaPath}
	if code := runWithInput(fileArgs, strings.NewReader(""), &fileStdout, &fileStderr); code != 0 {
		t.Fatalf("file generation code = %d, stderr = %q", code, fileStderr.String())
	}
	if fileStdout.Len() != 0 || fileStderr.Len() != 0 {
		t.Fatalf("file generation streams = stdout %q, stderr %q", fileStdout.String(), fileStderr.String())
	}
	actual, err := os.ReadFile(outputPath) //nolint:gosec // outputPath is created in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("file bytes differ from stdout/public output")
	}

	fileStdout.Reset()
	fileStderr.Reset()
	forceArgs := []string{"generate", "--package", "generated", "--output", outputPath, "--force", schemaPath}
	if code := runWithInput(forceArgs, strings.NewReader(""), &fileStdout, &fileStderr); code != 0 {
		t.Fatalf("forced repeated file generation code = %d, stderr = %q", code, fileStderr.String())
	}
	repeated, err := os.ReadFile(outputPath) //nolint:gosec // outputPath is created in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repeated, expected) {
		t.Fatalf("forced repeated file bytes changed")
	}
}

func TestGenerateCommandPreservesChoiceOrderAndCollisionBehavior(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "choice.xsd")
	contents := schemaDocument(`<xs:complexType name="Choice"><xs:choice><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:decimal"/></xs:choice></xs:complexType><xs:simpleType name="line-item"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="LINE_ITEM"><xs:restriction base="xs:decimal"/></xs:simpleType>`)
	writeTestFile(t, schemaPath, contents)
	expected := publicGenerate(t, contents, "generated")

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"generate", "--package", "generated", "--schema-root", directory, schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("choice generation = code %d, stdout %d bytes, stderr %q", code, stdout.Len(), stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), expected) {
		t.Fatalf("choice output differs from public generator:\n%s", stdout.Bytes())
	}
	assertFormattedGeneratedSource(t, stdout.Bytes())
	source := stdout.String()
	if !strings.Contains(source, "type Choice interface {\n\tisChoice()\n}") {
		t.Fatalf("choice output has no generated type-switch interface:\n%s", source)
	}
	if strings.Index(source, "type First struct") > strings.Index(source, "type Second struct") {
		t.Fatalf("choice alternatives are not in lexical order:\n%s", source)
	}
	if !strings.Contains(source, "type LineItem struct") || !strings.Contains(source, "type LineItem2 struct") {
		t.Fatalf("collision suffixes are missing:\n%s", source)
	}
	compileGeneratedCLI(t, stdout.Bytes(), `func classify(value Choice) string {
	switch value.(type) {
	case First:
		return "first"
	case Second:
		return "second"
	default:
		return "unknown"
	}
}`)
}

func TestGenerateCommandReportsUsageStatusTwo(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing package", args: []string{"generate", "--diagnostics", "json", schemaPath}},
		{name: "invalid package", args: []string{"generate", "--diagnostics", "json", "--package", "bad-name", schemaPath}},
		{name: "unknown option", args: []string{"generate", "--diagnostics", "json", "--unknown", "--package", "generated", schemaPath}},
		{name: "late option", args: []string{"generate", "--diagnostics", "json", "--package", "generated", schemaPath, "--force"}},
		{name: "extra operand", args: []string{"generate", "--diagnostics", "json", "--package", "generated", schemaPath, "other.xsd"}},
		{name: "force without output", args: []string{"generate", "--diagnostics", "json", "--force", "--package", "generated", schemaPath}},
		{name: "force stdout", args: []string{"generate", "--diagnostics", "json", "--force", "--package", "generated", "--output", "-", schemaPath}},
		{name: "duplicate output", args: []string{"generate", "--diagnostics", "json", "--package", "generated", "--output", "one.go", "--output", "two.go", schemaPath}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithInput(test.args, strings.NewReader(""), &stdout, &stderr)
			if code != 2 || stdout.Len() != 0 {
				t.Fatalf("usage result = code %d, stdout %q", code, stdout.String())
			}
			var envelope diagnosticEnvelope
			decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
			if envelope.Command != "generate" || envelope.Stage != "usage" || envelope.ExitStatus != 2 || len(envelope.Diagnostics) != 1 {
				t.Fatalf("usage envelope = %#v", envelope)
			}
			diagnostic := envelope.Diagnostics[0]
			if diagnostic.Class != nil || diagnostic.Kind != cliUsageKind || diagnostic.Code != cliUsageCode {
				t.Fatalf("usage diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestGenerateCommandPreservesGenerateDiagnostics(t *testing.T) {
	directory := t.TempDir()
	tests := []generateDiagnosticCase{
		{
			name:        "unsupported component",
			contents:    schemaDocument(`<xs:attribute name="amount"/>`),
			code:        "GOXSD9029",
			class:       goxsd9.FailureUnsupported,
			sourceID:    "schema/unsupported component.xsd",
			wantFeature: true,
		},
		{
			name:     "malformed source",
			contents: "<not-a-schema/>",
			code:     goxsd9.InvalidSchemaRootCode,
			class:    goxsd9.FailureInvalid,
			sourceID: "schema/malformed source.xsd",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { assertGenerateDiagnosticCase(t, directory, test) })
	}
}

type generateDiagnosticCase struct {
	name        string
	contents    string
	code        string
	class       goxsd9.FailureClass
	sourceID    string
	wantFeature bool
}

func assertGenerateDiagnosticCase(t *testing.T, directory string, test generateDiagnosticCase) {
	t.Helper()
	path := filepath.Join(directory, test.name+".xsd")
	writeTestFile(t, path, test.contents)
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"generate", "--diagnostics", "json", "--package", "generated", path}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("failure result = code %d, stdout %q", code, stdout.String())
	}
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
	if envelope.Command != "generate" || envelope.Stage != "generate" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("failure envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class == nil || *diagnostic.Class != string(test.class) || diagnostic.Kind != "processing" || diagnostic.Code != test.code || diagnostic.SourceID != test.sourceID {
		t.Fatalf("failure diagnostic = %#v", diagnostic)
	}
	if test.wantFeature && (diagnostic.Feature == "" || diagnostic.SpecRef == "") {
		t.Fatalf("unsupported diagnostic metadata = %#v", diagnostic)
	}
	if diagnostic.Location.Line == 0 || diagnostic.Location.Column == 0 {
		t.Fatalf("failure diagnostic lost primary location = %#v", diagnostic)
	}
}

func TestGenerateCommandChecksOutputLimitBeforeDelivery(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	packageName := strings.Repeat("a", int(maxGeneratedOutputBytes))

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"generate", "--diagnostics", "json", "--package", packageName, schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("stdout limit result = code %d, stdout %d bytes", code, stdout.Len())
	}
	assertGenerateOutputLimit(t, stderr.Bytes(), "output/stdout")

	outputPath := filepath.Join(directory, "generated.go")
	sentinel := []byte("keep this destination")
	if err := os.WriteFile(outputPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"generate", "--diagnostics", "json", "--package", packageName, "--output", outputPath, "--force", schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("file limit result = code %d, stdout %q", code, stdout.String())
	}
	assertGenerateOutputLimit(t, stderr.Bytes(), string(generatedOutputSourceIDForOperand(outputPath)))
	actual, err := os.ReadFile(outputPath) //nolint:gosec // outputPath is created in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, sentinel) {
		t.Fatalf("output-limit failure changed existing destination")
	}
}

func TestGenerateCommandOutputDestinationSafety(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	contents := schemaDocument(`<xs:element name="count" type="xs:integer"/>`)
	writeTestFile(t, schemaPath, contents)
	expected := publicGenerate(t, contents, "generated")

	outputPath := filepath.Join(directory, "generated.go")
	sentinel := []byte("original")
	if err := os.WriteFile(outputPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"generate", "--diagnostics", "json", "--package", "generated", "--output", outputPath, schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("existing destination result = code %d, stdout %q", code, stdout.String())
	}
	assertGenerateOutputError(t, stderr.Bytes(), generatedOutputSourceIDForOperand(outputPath))
	assertFileBytes(t, outputPath, sentinel)

	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"generate", "--diagnostics", "json", "--package", "generated", "--output", outputPath, "--force", schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("forced destination result = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	assertFileBytes(t, outputPath, expected)

	outsidePath := filepath.Join(directory, "outside.go")
	outside := []byte("outside")
	if err := os.WriteFile(outsidePath, outside, 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(directory, "link.go")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"generate", "--diagnostics", "json", "--package", "generated", "--output", symlinkPath, "--force", schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("symlink destination result = code %d, stdout %q", code, stdout.String())
	}
	assertGenerateOutputError(t, stderr.Bytes(), generatedOutputSourceIDForOperand(symlinkPath))
	assertFileBytes(t, outsidePath, outside)
	linkInfo, err := os.Lstat(symlinkPath)
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink destination was replaced: err=%v info=%v", err, linkInfo)
	}

	missingParentPath := filepath.Join(directory, "missing", "generated.go")
	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"generate", "--diagnostics", "json", "--package", "generated", "--output", missingParentPath, schemaPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("missing parent result = code %d, stdout %q", code, stdout.String())
	}
	assertGenerateOutputError(t, stderr.Bytes(), generatedOutputSourceIDForOperand(missingParentPath))
	if _, err := os.Stat(filepath.Dir(missingParentPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing output parent was created or returned unexpected error: %v", err)
	}
}

func TestGenerateOutputTransactionCleansFailedTemporaryFiles(t *testing.T) {
	directory := t.TempDir()
	destinationPath := filepath.Join(directory, "generated.go")
	sentinel := []byte("safe")
	if err := os.WriteFile(destinationPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(destinationPath)
	if err != nil {
		t.Fatal(err)
	}

	tests := []outputTransactionFailureCase{
		{name: "write", writeErr: errors.New("write failed")},
		{name: "close", closeErr: errors.New("close failed")},
		{name: "rename", renameErr: errors.New("rename failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertFailedOutputTransaction(t, directory, destinationPath, info, sentinel, test)
		})
	}
}

type outputTransactionFailureCase struct {
	name      string
	writeErr  error
	closeErr  error
	renameErr error
}

func assertFailedOutputTransaction(t *testing.T, directory, destinationPath string, info os.FileInfo, sentinel []byte, test outputTransactionFailureCase) {
	t.Helper()
	temporary := &fakeOutputTemp{path: filepath.Join(directory, ".temporary-"+test.name), writeErr: test.writeErr, closeErr: test.closeErr}
	ops := &fakeOutputOps{destination: destinationPath, info: info, temporary: temporary, renameErr: test.renameErr}
	destination := outputDestination{path: destinationPath, sourceID: "output/generated.go"}
	err := writeGeneratedFileWithOps(destination, []byte("new"), true, ops)
	if err == nil {
		t.Fatal("failed transaction unexpectedly succeeded")
	}
	var cli *cliError
	if !errors.As(err, &cli) || cli.code != cliOutputCode || cli.kind != cliOutputKind || cli.sourceID != destination.sourceID {
		t.Fatalf("transaction error = %v, want CLI1005 output diagnostic", err)
	}
	if test.writeErr != nil || test.closeErr != nil {
		if ops.renameCalls != 0 {
			t.Fatalf("unexpected rename call count = %d", ops.renameCalls)
		}
	}
	if test.renameErr != nil && ops.renameCalls != 1 {
		t.Fatalf("rename call count = %d, want 1", ops.renameCalls)
	}
	if !ops.removed {
		t.Fatal("temporary output was not cleaned")
	}
	assertFileBytes(t, destinationPath, sentinel)
}

func TestGenerateCommandReusesSchemaStdinResolverAndSourceLimits(t *testing.T) {
	directory := t.TempDir()
	childPath := filepath.Join(directory, "child.xsd")
	writeTestFile(t, childPath, schemaDocument(`<xs:simpleType name="Child"><xs:restriction base="xs:integer"/></xs:simpleType>`))
	root := schemaDocument(`<xs:include schemaLocation="child.xsd"/>`)

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"generate", "--diagnostics", "json", "--schema-root", directory, "--package", "generated", "-"}, strings.NewReader(root), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "type Child struct") {
		t.Fatalf("stdin/resolver generation = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"generate", "--diagnostics", "json", "--schema-root", directory, "--package", "generated", "-"}, strings.NewReader(sizedSchema("", int(maxSchemaSourceBytes+1))), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("stdin source limit = code %d, stdout %d bytes", code, stdout.Len())
	}
	assertGenerateLimit(t, stderr.Bytes(), "schema/stdin")

	writeTestFile(t, childPath, sizedSchema("", int(maxSchemaSourceBytes+1)))
	writeTestFile(t, filepath.Join(directory, "root.xsd"), root)
	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"generate", "--diagnostics", "json", "--schema-root", directory, "--package", "generated", filepath.Join(directory, "root.xsd")}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("resolver source limit = code %d, stdout %d bytes", code, stdout.Len())
	}
	assertGenerateLimit(t, stderr.Bytes(), "schema/child.xsd")
}

func publicGenerate(t *testing.T, contents, packageName string) []byte {
	t.Helper()
	root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(contents)))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := goxsd9.ParseSchema(root, nil)
	if err != nil {
		t.Fatalf("public ParseSchema: %v", err)
	}
	generated, err := goxsd9.GenerateGo(schema, packageName)
	if err != nil {
		t.Fatalf("public GenerateGo: %v", err)
	}
	return generated
}

func assertFormattedGeneratedSource(t *testing.T, source []byte) {
	t.Helper()
	formatted, err := format.Source(source)
	if err != nil {
		t.Fatalf("generated source is not gofmt-valid: %v\n%s", err, source)
	}
	if !bytes.Equal(formatted, source) {
		t.Fatalf("generated source is not complete gofmt output")
	}
}

func compileGeneratedCLI(t *testing.T, source []byte, extra string) {
	t.Helper()
	root := documentedRepositoryRoot(t)
	temporary := t.TempDir()
	module := "module generated.test\n\ngo 1.26.0\n\nrequire github.com/goxdra/goxsd9 v0.0.0\n\nreplace github.com/goxdra/goxsd9 => " + root + "\n"
	if err := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "generated.go"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "use.go"), []byte("package generated\n\n"+extra+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile generated CLI source: %v\n%s", err, output)
	}
}

func assertGenerateOutputLimit(t *testing.T, data []byte, sourceID string) {
	t.Helper()
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, data, &envelope)
	if envelope.Command != "generate" || envelope.Stage != "output" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("output-limit envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class != nil || diagnostic.Kind != cliLimitKind || diagnostic.Code != cliLimitCode || diagnostic.SourceID != sourceID {
		t.Fatalf("output-limit diagnostic = %#v", diagnostic)
	}
}

func assertGenerateLimit(t *testing.T, data []byte, sourceID string) {
	t.Helper()
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, data, &envelope)
	if envelope.Command != "generate" || envelope.Stage != "generate" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("source-limit envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class != nil || diagnostic.Kind != cliLimitKind || diagnostic.Code != cliLimitCode || diagnostic.SourceID != sourceID {
		t.Fatalf("source-limit diagnostic = %#v", diagnostic)
	}
}

func assertGenerateOutputError(t *testing.T, data []byte, sourceID goxsd9.SourceID) {
	t.Helper()
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, data, &envelope)
	if envelope.Command != "generate" || envelope.Stage != "output" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("output-error envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class != nil || diagnostic.Kind != cliOutputKind || diagnostic.Code != cliOutputCode || diagnostic.SourceID != string(sourceID) {
		t.Fatalf("output-error diagnostic = %#v", diagnostic)
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path) //nolint:gosec // callers pass paths created in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s bytes = %q, want %q", path, got, want)
	}
}

type fakeOutputTemp struct {
	path     string
	data     bytes.Buffer
	writeErr error
	closeErr error
}

func (file *fakeOutputTemp) Write(data []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return file.data.Write(data)
}

func (file *fakeOutputTemp) Name() string {
	return file.path
}

func (file *fakeOutputTemp) Close() error {
	return file.closeErr
}

type fakeOutputOps struct {
	destination string
	info        os.FileInfo
	temporary   *fakeOutputTemp
	renameErr   error
	renameCalls int
	removed     bool
}

func (ops *fakeOutputOps) lstat(path string) (os.FileInfo, error) {
	if path == ops.destination {
		return ops.info, nil
	}
	return nil, os.ErrNotExist
}

func (ops *fakeOutputOps) createTemp(string, string) (outputTempFile, error) {
	return ops.temporary, nil
}

func (ops *fakeOutputOps) rename(string, string) error {
	ops.renameCalls++
	return ops.renameErr
}

func (ops *fakeOutputOps) remove(string) error {
	ops.removed = true
	return nil
}
