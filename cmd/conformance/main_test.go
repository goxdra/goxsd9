package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInventoryCommandFromRepositoryRoot(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(filename), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	command := exec.CommandContext(context.Background(), "go", "tool", "conformance", "inventory")
	command.Dir = root
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("go tool conformance inventory: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("go tool conformance inventory wrote stderr: %q", stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"W3C XML Schema test catalog inventory\n",
		"origin version kind cases valid invalid other submitted accepted stable queried disputed-test disputed-spec status-missing unusable headline\n",
		"# Catalog metadata only; no schema or instance tests are executed.\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("inventory output missing %q:\n%s", want, output)
		}
	}
}

func TestSchemaCommandRequiresBoundedSelection(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"schema", "-version", "1.0"}, &stdout, &stderr); got != 2 {
		t.Fatalf("schema without selector exit code = %d, want 2", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("schema without selector wrote stdout: %q", stdout.String())
	}
	for _, want := range []string{
		"schema requires -set or -case; full-suite execution is not allowed",
		"go tool conformance schema -version {1.0|1.1}",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("schema usage missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestSchemaCommandExecutesExplicitPinnedCase(t *testing.T) {
	root := t.TempDir()
	resourceRoot := filepath.Join(root, "testdata", "w3c", "xsdtests")
	if err := os.MkdirAll(filepath.Join(resourceRoot, "sets"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeCommandFixture(t, filepath.Join(resourceRoot, "suite.xml"), commandSuite)
	writeCommandFixture(t, filepath.Join(resourceRoot, "extra-suite.xml"), commandExtraSuite)
	writeCommandFixture(t, filepath.Join(resourceRoot, "sets", "run.testSet"), commandTestSet)
	writeCommandFixture(t, filepath.Join(resourceRoot, "sets", "valid.xsd"), commandValidSchema)

	var stdout, stderr bytes.Buffer
	args := []string{
		"schema", "-root", root, "-version", "1.0", "-set", "sets/run.testSet", "-case", "accepted",
	}
	if got := run(args, &stdout, &stderr); got != 0 {
		t.Fatalf("schema command exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", got, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("schema command wrote stderr: %q", stderr.String())
	}
	for _, want := range []string{
		"version: 1.0\npolicy: Strict10\n",
		"name=\"accepted\"",
		"actual=valid",
		"outcome=pass",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("schema output missing %q:\n%s", want, stdout.String())
		}
	}
}

func writeCommandFixture(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

const commandSuite = `<?xml version="1.0"?>
<ts:testSuite xmlns:ts="http://www.w3.org/XML/2004/xml-schema-test-suite/" xmlns:xlink="http://www.w3.org/1999/xlink" name="command" releaseDate="2026-01-01" schemaVersion="fixture">
  <ts:testSetRef xlink:href="sets/run.testSet"/>
</ts:testSuite>
`

const commandExtraSuite = `<?xml version="1.0"?>
<testSuite xmlns="http://www.w3.org/XML/2004/xml-schema-test-suite/" xmlns:xlink="http://www.w3.org/1999/xlink" name="command-extra" releaseDate="2026-01-01" schemaVersion="fixture"/>
`

const commandTestSet = `<?xml version="1.0"?>
<testSet xmlns="http://www.w3.org/XML/2004/xml-schema-test-suite/" xmlns:xlink="http://www.w3.org/1999/xlink" name="run-set" contributor="fixture">
  <testGroup name="run-group">
    <schemaTest name="accepted">
      <schemaDocument xlink:href="valid.xsd"/>
      <expected validity="valid"/>
      <current status="accepted" date="2026-01-01"/>
    </schemaTest>
  </testGroup>
</testSet>
`

const commandValidSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`
