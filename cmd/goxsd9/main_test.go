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

func TestParseCommandRejectsStdinURISchemaRoots(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{name: "http URI", root: "http://example.test/schema-root"},
		{name: "file URI", root: "file:/tmp/schema-root"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithInput(
				[]string{"parse", "--diagnostics", "json", "--schema-root", test.root, "-"},
				strings.NewReader(schemaDocument(`<xs:element name="root"/>`)),
				&stdout,
				&stderr,
			)
			if code != 1 || stdout.Len() != 0 {
				t.Fatalf("URI schema-root result = code %d, stdout %q", code, stdout.String())
			}
			var envelope diagnosticEnvelope
			decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
			if envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
				t.Fatalf("diagnostic envelope = %#v", envelope)
			}
			diagnostic := envelope.Diagnostics[0]
			if diagnostic.Kind != cliPathPolicyKind || diagnostic.Code != cliPathPolicyCode || diagnostic.SourceID != "schema/stdin" {
				t.Fatalf("diagnostic fields = %#v", diagnostic)
			}
		})
	}
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

func TestValidateCommandReportsSilentScalarSuccess(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	instancePath := filepath.Join(directory, "instance.xml")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/><xs:element name="amount" type="xs:decimal"/>`))

	for _, value := range []string{"<count>42</count>", "<amount>12.50</amount>"} {
		writeTestFile(t, instancePath, value)
		var stdout, stderr bytes.Buffer
		code := runWithInput([]string{"validate", "--schema-root", directory, schemaPath, instancePath}, strings.NewReader("unused"), &stdout, &stderr)
		if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("valid instance %q = code %d, stdout %q, stderr %q", value, code, stdout.String(), stderr.String())
		}
	}
}

func TestValidateCommandRendersLocatedHumanAndJSONDiagnostics(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	instancePath := filepath.Join(directory, "invalid.xml")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	writeTestFile(t, instancePath, `<count>not-an-integer</count>`)

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"validate", schemaPath, instancePath}, strings.NewReader("unused"), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("human invalid = code %d, stdout %q", code, stdout.String())
	}
	if want := "validate stage=validate class=invalid kind=processing"; !strings.HasPrefix(stderr.String(), want) {
		t.Fatalf("human diagnostic = %q, want prefix %q", stderr.String(), want)
	}
	expectedSource := expectedInstanceSourceID(t, instancePath)
	for _, want := range []string{"source_id=" + expectedSource, "code=" + goxsd9.InvalidIntegerLexicalCode} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("human diagnostic = %q, missing %q", stderr.String(), want)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, instancePath}, strings.NewReader("unused"), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("JSON invalid = code %d, stdout %q", code, stdout.String())
	}
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
	if envelope.Command != "validate" || envelope.Stage != "validate" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("JSON envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class == nil || *diagnostic.Class != string(goxsd9.FailureInvalid) || diagnostic.Kind != "processing" || diagnostic.Code != goxsd9.InvalidIntegerLexicalCode || diagnostic.SourceID != expectedSource {
		t.Fatalf("JSON diagnostic = %#v", diagnostic)
	}
	if diagnostic.Location.Line != 1 || diagnostic.Location.Column != 8 {
		t.Fatalf("JSON location = %#v", diagnostic.Location)
	}
}

func TestValidateCommandSeparatesSchemaAndInstanceStages(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, schemaPath, "<not-a-schema/>")
	instance := &trackingReadCloser{reader: strings.NewReader(`<count>not-an-integer</count>`)}
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, "-"}, instance, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || instance.reads != 0 {
		t.Fatalf("schema failure = code %d, stdout %q, reads %d", code, stdout.String(), instance.reads)
	}
	var schemaEnvelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &schemaEnvelope)
	if schemaEnvelope.Command != "validate" || schemaEnvelope.Stage != "parse" || schemaEnvelope.ExitStatus != 1 {
		t.Fatalf("schema envelope = %#v", schemaEnvelope)
	}
	if instance.closes != 0 {
		t.Fatalf("schema failure closed unopened instance stdin %d times", instance.closes)
	}

	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	stdout.Reset()
	stderr.Reset()
	instance = &trackingReadCloser{reader: strings.NewReader(`<count>not-an-integer</count>`)}
	code = runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, "-"}, instance, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || instance.reads == 0 || instance.closes != 1 {
		t.Fatalf("instance failure = code %d, stdout %q, reads %d, closes %d", code, stdout.String(), instance.reads, instance.closes)
	}
	var instanceEnvelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &instanceEnvelope)
	if instanceEnvelope.Command != "validate" || instanceEnvelope.Stage != "validate" || instanceEnvelope.ExitStatus != 1 {
		t.Fatalf("instance envelope = %#v", instanceEnvelope)
	}
	if got := instanceEnvelope.Diagnostics[0].SourceID; got != "instance/stdin" {
		t.Fatalf("instance source ID = %q, want instance/stdin", got)
	}
}

func TestValidateCommandPreservesUnsupportedInstanceDiagnostics(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"validate", "--diagnostics", "json", schemaPath, "-"},
		strings.NewReader(`<count xsi:schemaLocation="urn:example example.xsd" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">1</count>`),
		&stdout,
		&stderr,
	)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("unsupported instance = code %d, stdout %q", code, stdout.String())
	}
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
	if envelope.Command != "validate" || envelope.Stage != "validate" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("unsupported envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class == nil || *diagnostic.Class != string(goxsd9.FailureUnsupported) || diagnostic.Code != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Kind != "processing" {
		t.Fatalf("unsupported diagnostic = %#v", diagnostic)
	}
	if diagnostic.Feature == "" || diagnostic.SpecRef == "" || diagnostic.SourceID != "instance/stdin" {
		t.Fatalf("unsupported diagnostic metadata = %#v", diagnostic)
	}
}

func TestValidateCommandUsageStatusesAndDeterministicDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		sourceID string
	}{
		{name: "unknown flag", args: []string{"validate", "--diagnostics", "json", "--unknown", "schema.xsd", "instance.xml"}, sourceID: "-"},
		{name: "duplicate root", args: []string{"validate", "--diagnostics", "json", "--schema-root", ".", "--schema-root", ".", "schema.xsd", "instance.xml"}, sourceID: "-"},
		{name: "duplicate diagnostics", args: []string{"validate", "--diagnostics", "json", "--diagnostics", "human", "schema.xsd", "instance.xml"}, sourceID: "-"},
		{name: "late flag", args: []string{"validate", "--diagnostics", "json", "schema.xsd", "--diagnostics", "human", "instance.xml"}, sourceID: "-"},
		{name: "extra operand", args: []string{"validate", "--diagnostics", "json", "schema.xsd", "instance.xml", "extra.xml"}, sourceID: "-"},
		{name: "missing operand", args: []string{"validate", "--diagnostics", "json", "schema.xsd"}, sourceID: "-"},
		{name: "empty schema", args: []string{"validate", "--diagnostics", "json", "", "instance.xml"}, sourceID: "-"},
		{name: "empty instance", args: []string{"validate", "--diagnostics", "json", "schema.xsd", ""}, sourceID: "-"},
		{name: "schema stdin without root", args: []string{"validate", "--diagnostics", "json", "-", "instance.xml"}, sourceID: "schema/stdin"},
		{name: "two stdin", args: []string{"validate", "--diagnostics", "json", "--schema-root", ".", "-", "-"}, sourceID: "instance/stdin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var firstOut, firstErr bytes.Buffer
			if code := runWithInput(test.args, strings.NewReader(""), &firstOut, &firstErr); code != 2 || firstOut.Len() != 0 {
				t.Fatalf("first result = code %d, stdout %q", code, firstOut.String())
			}
			assertValidateUsage(t, firstErr.Bytes(), test.sourceID)

			var secondOut, secondErr bytes.Buffer
			if code := runWithInput(test.args, strings.NewReader(""), &secondOut, &secondErr); code != 2 || secondOut.Len() != 0 {
				t.Fatalf("second result = code %d, stdout %q", code, secondOut.String())
			}
			if firstErr.String() != secondErr.String() {
				t.Fatalf("diagnostics differ:\nfirst %q\nsecond %q", firstErr.String(), secondErr.String())
			}
		})
	}
}

func TestValidateCommandStdinIdentityAndSchemaFirstOrdering(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	instancePath := filepath.Join(directory, "instance.xml")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	writeTestFile(t, instancePath, `<count>42</count>`)

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"validate", "--schema-root", directory, "-", instancePath}, strings.NewReader(schemaDocument(`<xs:element name="count" type="xs:integer"/>`)), &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("schema stdin success = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}

	writeTestFile(t, schemaPath, "<not-a-schema/>")
	instance := &trackingReadCloser{reader: strings.NewReader(`<count>42</count>`)}
	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, "-"}, instance, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || instance.reads != 0 || instance.closes != 0 {
		t.Fatalf("schema-first instance stdin = code %d, stdout %q, reads %d, closes %d", code, stdout.String(), instance.reads, instance.closes)
	}
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
	if envelope.Diagnostics[0].SourceID != "schema/root.xsd" || envelope.Stage != "parse" {
		t.Fatalf("schema-first diagnostic = %#v", envelope.Diagnostics[0])
	}
}

func TestValidateCommandReportsInstanceIdentityPathAndResourceFailures(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	missingPath := filepath.Join(directory, "missing.xml")
	childDirectory := filepath.Join(directory, "instance-dir")
	if err := os.Mkdir(childDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))

	tests := []struct {
		name     string
		operand  string
		code     string
		sourceID string
	}{
		{name: "missing", operand: missingPath, code: cliResourceCode, sourceID: expectedInstanceSourceID(t, missingPath)},
		{name: "directory", operand: childDirectory, code: cliResourceCode, sourceID: expectedInstanceSourceID(t, childDirectory)},
		{name: "URI", operand: "https://example.test/instance.xml", code: cliPathPolicyCode, sourceID: "instance/-"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, test.operand}, strings.NewReader("unused"), &stdout, &stderr)
			if code != 1 || stdout.Len() != 0 {
				t.Fatalf("result = code %d, stdout %q", code, stdout.String())
			}
			var envelope diagnosticEnvelope
			decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
			if envelope.Command != "validate" || envelope.Stage != "validate" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
				t.Fatalf("envelope = %#v", envelope)
			}
			diagnostic := envelope.Diagnostics[0]
			if diagnostic.Code != test.code || diagnostic.SourceID != test.sourceID || diagnostic.Class != nil {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestValidateCommandEnforcesIndependentInstanceLimit(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	instancePath := filepath.Join(directory, "instance.xml")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))

	writeTestFile(t, instancePath, sizedInstance(maxInstanceSourceBytes))
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, instancePath}, strings.NewReader("unused"), &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("exact instance limit = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}

	writeTestFile(t, instancePath, sizedInstance(maxInstanceSourceBytes+1))
	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, instancePath}, strings.NewReader("unused"), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("over instance limit = code %d, stdout %q", code, stdout.String())
	}
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &envelope)
	if envelope.Stage != "validate" || envelope.ExitStatus != 1 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("limit envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Code != cliLimitCode || diagnostic.SourceID != expectedInstanceSourceID(t, instancePath) || diagnostic.Kind != cliLimitKind || diagnostic.Class != nil {
		t.Fatalf("limit diagnostic = %#v", diagnostic)
	}

	stdout.Reset()
	stderr.Reset()
	code = runWithInput([]string{"validate", "--diagnostics", "json", schemaPath, "-"}, strings.NewReader(sizedInstance(maxInstanceSourceBytes+1)), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("stream instance limit = code %d, stdout %q", code, stdout.String())
	}
	var streamEnvelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, stderr.Bytes(), &streamEnvelope)
	if streamEnvelope.Stage != "validate" || streamEnvelope.ExitStatus != 1 || len(streamEnvelope.Diagnostics) != 1 {
		t.Fatalf("stream limit envelope = %#v", streamEnvelope)
	}
	streamDiagnostic := streamEnvelope.Diagnostics[0]
	if streamDiagnostic.Code != cliLimitCode || streamDiagnostic.SourceID != "instance/stdin" || streamDiagnostic.Kind != cliLimitKind || streamDiagnostic.Class != nil {
		t.Fatalf("stream limit diagnostic = %#v", streamDiagnostic)
	}
}

func TestValidateCommandClosesInstanceExactlyOnce(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "root.xsd")
	writeTestFile(t, schemaPath, schemaDocument(`<xs:element name="count" type="xs:integer"/>`))
	instance := &trackingReadCloser{reader: strings.NewReader(`<count>42</count>`)}
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"validate", schemaPath, "-"}, instance, &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("lifecycle result = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if instance.reads == 0 || instance.closes != 1 {
		t.Fatalf("instance lifecycle = reads %d, closes %d", instance.reads, instance.closes)
	}
}

func assertValidateUsage(t *testing.T, data []byte, sourceID string) {
	t.Helper()
	var envelope diagnosticEnvelope
	decodeDiagnosticEnvelope(t, data, &envelope)
	if envelope.Command != "validate" || envelope.Stage != "usage" || envelope.ExitStatus != 2 || len(envelope.Diagnostics) != 1 {
		t.Fatalf("usage envelope = %#v", envelope)
	}
	diagnostic := envelope.Diagnostics[0]
	if diagnostic.Class != nil || diagnostic.Kind != cliUsageKind || diagnostic.Code != cliUsageCode || diagnostic.SourceID != sourceID || diagnostic.Location != (renderedLineColumn{}) {
		t.Fatalf("usage diagnostic = %#v", diagnostic)
	}
}

func expectedInstanceSourceID(t *testing.T, path string) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(instanceIDForPath(root, absolute))
}

func sizedInstance(size int64) string {
	prefix := `<count>`
	suffix := `1</count>`
	if int64(len(prefix)+len(suffix)) > size {
		panic("test instance prefix exceeds requested size")
	}
	return prefix + strings.Repeat(" ", int(size)-len(prefix)-len(suffix)) + suffix
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

type trackingReadCloser struct {
	reader *strings.Reader
	reads  int
	closes int
	err    error
}

func (reader *trackingReadCloser) Read(buffer []byte) (int, error) {
	reader.reads++
	return reader.reader.Read(buffer)
}

func (reader *trackingReadCloser) Close() error {
	reader.closes++
	return reader.err
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
