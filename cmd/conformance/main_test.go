package main

import (
	"bytes"
	"context"
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
