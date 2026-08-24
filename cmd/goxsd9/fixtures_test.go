package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDocumentedScalarCLIWorkflow(t *testing.T) {
	root := documentedRepositoryRoot(t)
	schema := filepath.ToSlash(filepath.Join("examples", "root.xsd"))
	valid := filepath.ToSlash(filepath.Join("examples", "valid.xml"))
	invalid := filepath.ToSlash(filepath.Join("examples", "invalid.xml"))

	parsed := runDocumentedProductCLI(t, root, "parse", schema)
	if parsed.status != 0 {
		t.Fatalf("parse status = %d, stderr = %q", parsed.status, parsed.stderr)
	}
	if parsed.stdout != "documents=1 components=2\n" {
		t.Fatalf("parse stdout = %q, want %q", parsed.stdout, "documents=1 components=2\\n")
	}
	if parsed.stderr != "" {
		t.Fatalf("parse stderr = %q, want empty", parsed.stderr)
	}

	validated := runDocumentedProductCLI(t, root, "validate", schema, valid)
	if validated.status != 0 {
		t.Fatalf("valid validation status = %d, stderr = %q", validated.status, validated.stderr)
	}
	if validated.stdout != "" || validated.stderr != "" {
		t.Fatalf("valid validation streams = stdout %q, stderr %q; want both empty", validated.stdout, validated.stderr)
	}

	invalidResult := runDocumentedProductCLI(t, root, "validate", schema, invalid)
	if invalidResult.status != 1 {
		t.Fatalf("invalid validation status = %d, stderr = %q", invalidResult.status, invalidResult.stderr)
	}
	if invalidResult.stdout != "" {
		t.Fatalf("invalid validation stdout = %q, want empty", invalidResult.stdout)
	}
	wantDiagnostic := "validate stage=validate class=invalid kind=processing source_id=instance/examples/invalid.xml location=1:8 code=XSD2001 related=schema/root.xsd:2:3 spec_ref=xsd11-datatypes#integer invalid xs:integer lexical representation\n"
	if !strings.HasPrefix(invalidResult.stderr, wantDiagnostic) {
		t.Fatalf("invalid validation stderr = %q, want diagnostic prefix %q", invalidResult.stderr, wantDiagnostic)
	}
	if got := strings.TrimPrefix(invalidResult.stderr, wantDiagnostic); got != "exit status 1\n" {
		t.Fatalf("go run wrapper stderr = %q, want %q", got, "exit status 1\\n")
	}
}

func documentedRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(filename), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

type documentedProductCLIResult struct {
	stdout string
	stderr string
	status int
}

func runDocumentedProductCLI(t *testing.T, root string, args ...string) documentedProductCLIResult {
	t.Helper()
	commandArgs := append([]string{"run", "./cmd/goxsd9"}, args...)
	command := exec.CommandContext(context.Background(), "go", commandArgs...) //nolint:gosec // the test invokes the fixed Go product command.
	command.Dir = root
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	status := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("go run ./cmd/goxsd9 %s: %v", strings.Join(args, " "), err)
		}
		status = exitErr.ExitCode()
	}
	return documentedProductCLIResult{stdout: stdout.String(), stderr: stderr.String(), status: status}
}
