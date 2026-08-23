package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const testSchemaNamespace = "http://www.w3.org/2001/XMLSchema"

func TestParseCommandReportsSingleAndMultiDocumentSuccess(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "schemas", "root.xsd")
	childPath := filepath.Join(directory, "schemas", "child.xsd")
	writeTestFile(t, rootPath, schemaDocument(`<xs:include schemaLocation="child.xsd"/><xs:element name="root"/>`))
	writeTestFile(t, childPath, schemaDocument(`<xs:element name="child"/>`))

	var stdout, stderr bytes.Buffer
	if code := runWithInput([]string{"parse", rootPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("single-file parse code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "documents=2 components=2\n"; got != want {
		t.Fatalf("default-root output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("successful parse stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runWithInput([]string{"parse", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("explicit-root parse code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "documents=2 components=2\n"; got != want {
		t.Fatalf("explicit-root output = %q, want %q", got, want)
	}
}

func TestParseCommandReadsSchemaStdinWithRoleScopedID(t *testing.T) {
	directory := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"parse", "--schema-root", directory, "-"},
		strings.NewReader(schemaDocument(`<xs:element name="root"/>`)),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("stdin parse code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "documents=1 components=1\n"; got != want {
		t.Fatalf("stdin output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stdin success stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"parse", "--diagnostics", "json", "-"}, strings.NewReader(schemaDocument(`<xs:element name="root"/>`)), &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("stdin without root = code %d, stdout %q", code, stdout.String())
	}
	assertJSONDiagnostic(t, stderr.Bytes(), "CLI1001", "schema/stdin", 2)
}

func TestParseCommandPreservesChildSourceIDInDiagnostics(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "root.xsd")
	childPath := filepath.Join(directory, "child.xsd")
	writeTestFile(t, rootPath, schemaDocument(`<xs:include schemaLocation="child.xsd"/>`))
	writeTestFile(t, childPath, "<not-a-schema/>")

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"parse", "--diagnostics", "json", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("child diagnostic = code %d, stdout %q", code, stdout.String())
	}
	var envelope jsonDiagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
	if len(envelope.Diagnostics) != 1 {
		t.Fatalf("diagnostic count = %d, want 1", len(envelope.Diagnostics))
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class != "invalid" || diagnostic.Kind != "processing" || diagnostic.Code != goxsd9.InvalidSchemaRootCode {
		t.Fatalf("diagnostic fields = %#v", diagnostic)
	}
	if diagnostic.SourceID != "schema/child.xsd" || diagnostic.Location.Line != 1 || diagnostic.Location.Column != 1 {
		t.Fatalf("diagnostic location = %#v", diagnostic)
	}
	if diagnostic.Message == "" {
		t.Fatal("diagnostic message is empty")
	}
}

func TestParseCommandRejectsUsageForms(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"parse", "--diagnostics", "json", "--unknown", "schema.xsd"}},
		{name: "duplicate root", args: []string{"parse", "--diagnostics", "json", "--schema-root", ".", "--schema-root", ".", "schema.xsd"}},
		{name: "duplicate diagnostics", args: []string{"parse", "--diagnostics", "json", "--diagnostics", "json", "schema.xsd"}},
		{name: "late flag", args: []string{"parse", "--diagnostics", "json", "schema.xsd", "--diagnostics", "human"}},
		{name: "option followed by option", args: []string{"parse", "--diagnostics", "json", "--schema-root", "--unknown", "schema.xsd"}},
		{name: "extra operand", args: []string{"parse", "--diagnostics", "json", "schema.xsd", "other.xsd"}},
		{name: "missing operand", args: []string{"parse", "--diagnostics", "json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithInput(test.args, strings.NewReader(""), &stdout, &stderr)
			if code != 2 || stdout.Len() != 0 {
				t.Fatalf("usage result = code %d, stdout %q", code, stdout.String())
			}
			assertJSONDiagnostic(t, stderr.Bytes(), "CLI1001", "-", 2)
		})
	}
}

func TestParseCommandDiagnosticFormatsAreDeterministic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"parse", "--unknown", "schema.xsd"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("human usage = code %d, stdout %q", code, stdout.String())
	}
	if got, want := stderr.String(), "parse stage=usage class=- kind=usage source_id=- location=0:0 code=CLI1001 unknown option \"--unknown\"\n"; got != want {
		t.Fatalf("human diagnostic = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"parse", "--diagnostics", "json", "--unknown", "schema.xsd"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("JSON usage = code %d, stdout %q", code, stdout.String())
	}
	wantJSON := "{\"format\":\"goxsd9-diagnostics/v1\",\"command\":\"parse\",\"stage\":\"usage\",\"exit_status\":2,\"diagnostics\":[{\"class\":null,\"kind\":\"usage\",\"code\":\"CLI1001\",\"source_id\":\"-\",\"location\":{\"line\":0,\"column\":0},\"related\":[],\"feature\":\"\",\"spec_ref\":\"\",\"message\":\"unknown option \\\"--unknown\\\"\"}]}\n"
	if got := stderr.String(); got != wantJSON {
		t.Fatalf("JSON diagnostic = %q, want %q", got, wantJSON)
	}
}

func TestParseCommandRejectsLocalPolicyAndResourceFailures(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "root.xsd")
	tests := []struct {
		name     string
		location string
		code     string
		sourceID string
	}{
		{name: "http URI", location: "http://example.test/schema.xsd", code: cliPathPolicyCode, sourceID: "schema/-"},
		{name: "file URI", location: "file:child.xsd", code: cliPathPolicyCode, sourceID: "schema/-"},
		{name: "absolute", location: filepath.Join(directory, "outside.xsd"), code: cliPathPolicyCode, sourceID: "schema/-"},
		{name: "escape", location: "../../outside.xsd", code: cliPathPolicyCode, sourceID: "schema/-"},
		{name: "missing", location: "missing.xsd", code: cliResourceCode, sourceID: "schema/missing.xsd"},
		{name: "directory", location: "child-dir", code: cliResourceCode, sourceID: "schema/child-dir"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runPolicyFailure(t, directory, rootPath, test.location, test.code, test.sourceID) })
	}
}

func TestParseCommandRejectsSymlinkEscape(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "root.xsd")
	outside := filepath.Join(directory, "..", "outside-escape.xsd")
	writeTestFile(t, outside, schemaDocument(`<xs:element name="outside"/>`))
	link := filepath.Join(directory, "escape-link.xsd")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeTestFile(t, rootPath, schemaDocument(`<xs:include schemaLocation="escape-link.xsd"/>`))
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"parse", "--diagnostics", "json", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("symlink result = code %d, stdout %q", code, stdout.String())
	}
	assertJSONDiagnostic(t, stderr.Bytes(), cliPathPolicyCode, "schema/escape-link.xsd", 1)
}

func runPolicyFailure(t *testing.T, directory, rootPath, location, code, sourceID string) {
	t.Helper()
	writeTestFile(t, rootPath, schemaDocument(fmt.Sprintf(`<xs:include schemaLocation=%q/>`, location)))
	if location == "child-dir" {
		if err := os.Mkdir(filepath.Join(directory, "child-dir"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	result := runWithInput([]string{"parse", "--diagnostics", "json", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr)
	if result != 1 || stdout.Len() != 0 {
		t.Fatalf("policy result = code %d, stdout %q", result, stdout.String())
	}
	assertJSONDiagnostic(t, stderr.Bytes(), code, sourceID, 1)
}

func TestParseCommandEnforcesSourceAndTotalBoundaries(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, rootPath, sizedSchema("", int(maxSchemaSourceBytes)))
	var stdout, stderr bytes.Buffer
	if code := runWithInput([]string{"parse", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exact source boundary code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "documents=1 components=0\n"; got != want {
		t.Fatalf("exact source boundary output = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	writeTestFile(t, rootPath, sizedSchema("", int(maxSchemaSourceBytes+1)))
	if code := runWithInput([]string{"parse", "--diagnostics", "json", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 1 || stdout.Len() != 0 {
		t.Fatalf("over source boundary = code %d, stdout %q", code, stdout.String())
	}
	assertJSONDiagnostic(t, stderr.Bytes(), cliLimitCode, "schema/root.xsd", 1)

	for index := 0; index < 4; index++ {
		path := filepath.Join(directory, fmt.Sprintf("child%d.xsd", index))
		writeTestFile(t, path, sizedSchema("", int(maxSchemaSourceBytes)))
	}
	writeTestFile(t, rootPath, sizedSchema(`<xs:include schemaLocation="child0.xsd"/><xs:include schemaLocation="child1.xsd"/><xs:include schemaLocation="child2.xsd"/>`, int(maxSchemaSourceBytes)))
	stdout.Reset()
	stderr.Reset()
	if code := runWithInput([]string{"parse", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exact total boundary code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "documents=4 components=0\n"; got != want {
		t.Fatalf("exact total boundary output = %q, want %q", got, want)
	}

	writeTestFile(t, filepath.Join(directory, "child3.xsd"), sizedSchema("", int(maxSchemaSourceBytes)))
	writeTestFile(t, rootPath, sizedSchema(`<xs:include schemaLocation="child0.xsd"/><xs:include schemaLocation="child1.xsd"/><xs:include schemaLocation="child2.xsd"/><xs:include schemaLocation="child3.xsd"/>`, int(maxSchemaSourceBytes)))
	stdout.Reset()
	stderr.Reset()
	if code := runWithInput([]string{"parse", "--diagnostics", "json", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 1 || stdout.Len() != 0 {
		t.Fatalf("over total boundary = code %d, stdout %q", code, stdout.String())
	}
	assertJSONDiagnostic(t, stderr.Bytes(), cliLimitCode, "schema/child3.xsd", 1)
}

func TestParseCommandEnforcesResolverCallBoundary(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "root.xsd")
	childPath := filepath.Join(directory, "child.xsd")
	writeTestFile(t, childPath, schemaDocument(""))
	writeTestFile(t, rootPath, repeatedIncludes(maxResolverCalls))
	var stdout, stderr bytes.Buffer
	if code := runWithInput([]string{"parse", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exact resolver boundary code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "documents=2 components=0\n"; got != want {
		t.Fatalf("exact resolver boundary output = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	writeTestFile(t, rootPath, repeatedIncludes(maxResolverCalls+1))
	if code := runWithInput([]string{"parse", "--diagnostics", "json", "--schema-root", directory, rootPath}, strings.NewReader(""), &stdout, &stderr); code != 1 || stdout.Len() != 0 {
		t.Fatalf("over resolver boundary = code %d, stdout %q", code, stdout.String())
	}
	assertJSONDiagnostic(t, stderr.Bytes(), cliLimitCode, "schema/child.xsd", 1)
}

func TestParseCommandClosesBoundedSourceExactlyOnce(t *testing.T) {
	underlying := &countingReadCloser{Reader: strings.NewReader(schemaDocument(""))}
	limited := newLimitedSource(underlying, "schema/root.xsd", &schemaBudget{}, false)
	root, err := goxsd9.NewResolvedSource(context.Background(), "schema/root.xsd", limited)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goxsd9.ParseSchema(root, nil); err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	if underlying.closeCount != 1 {
		t.Fatalf("underlying close count = %d, want 1", underlying.closeCount)
	}
	if err := limited.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if underlying.closeCount != 1 {
		t.Fatalf("underlying close count after second close = %d, want 1", underlying.closeCount)
	}
}

func TestParseCommandReportsOutputFailureWithoutRetry(t *testing.T) {
	directory := t.TempDir()
	rootPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, rootPath, schemaDocument(""))
	stdout := &shortWriter{}
	var stderr bytes.Buffer
	code := runWithInput([]string{"parse", rootPath}, strings.NewReader(""), stdout, &stderr)
	if code != 1 || stdout.writes != 1 {
		t.Fatalf("output failure = code %d, writes %d", code, stdout.writes)
	}
	for _, want := range []string{"code=CLI1005", "kind=output", "source_id=output/stdout"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("output failure diagnostic = %q, missing %q", stderr.String(), want)
		}
	}
}

type jsonDiagnosticEnvelope struct {
	Format      string           `json:"format"`
	Command     string           `json:"command"`
	Stage       string           `json:"stage"`
	ExitStatus  int              `json:"exit_status"`
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
}

type jsonDiagnostic struct {
	Class    string         `json:"class"`
	Kind     string         `json:"kind"`
	Code     string         `json:"code"`
	SourceID string         `json:"source_id"`
	Location jsonLineColumn `json:"location"`
	Message  string         `json:"message"`
}

type jsonLineColumn struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

func assertJSONDiagnostic(t *testing.T, data []byte, code, sourceID string, status int) {
	t.Helper()
	var envelope struct {
		Format      string `json:"format"`
		Stage       string `json:"stage"`
		ExitStatus  int    `json:"exit_status"`
		Diagnostics []struct {
			Code     string         `json:"code"`
			SourceID string         `json:"source_id"`
			Location jsonLineColumn `json:"location"`
		} `json:"diagnostics"`
	}
	decodeDiagnosticEnvelope(t, data, &envelope)
	if envelope.Format != "goxsd9-diagnostics/v1" || envelope.Stage == "" || envelope.ExitStatus != status {
		t.Fatalf("envelope = %#v", envelope)
	}
	if len(envelope.Diagnostics) != 1 {
		t.Fatalf("diagnostic count = %d, want 1", len(envelope.Diagnostics))
	}
	if got := envelope.Diagnostics[0]; got.Code != code || got.SourceID != sourceID {
		t.Fatalf("diagnostic = %#v, want code/source %q/%q", got, code, sourceID)
	}
}

func decodeDiagnosticEnvelope(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode diagnostics %q: %v", data, err)
	}
}

func schemaDocument(contents string) string {
	return `<xs:schema xmlns:xs="` + testSchemaNamespace + `">` + contents + `</xs:schema>`
}

func sizedSchema(contents string, size int) string {
	open := schemaDocument(contents)
	if len(open) > size {
		panic("test schema prefix exceeds requested size")
	}
	padding := size - len(open)
	return open[:len(open)-len(`</xs:schema>`)] + strings.Repeat(" ", padding) + `</xs:schema>`
}

func repeatedIncludes(count int) string {
	var builder strings.Builder
	builder.WriteString(`<xs:schema xmlns:xs="` + testSchemaNamespace + `">`)
	for index := 0; index < count; index++ {
		builder.WriteString(`<xs:include schemaLocation="child.xsd"/>`)
	}
	builder.WriteString(`</xs:schema>`)
	return builder.String()
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

type countingReadCloser struct {
	*strings.Reader
	closeCount int
}

func (reader *countingReadCloser) Close() error {
	reader.closeCount++
	return nil
}

type shortWriter struct {
	writes int
}

func (writer *shortWriter) Write(buffer []byte) (int, error) {
	writer.writes++
	if len(buffer) == 0 {
		return 0, nil
	}
	return len(buffer) - 1, nil
}

var _ io.ReadCloser = (*countingReadCloser)(nil)
