package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIReportsUsageAndSearchesExplicitIndex(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "demo.index")
	index := "# goxsd9-spec-index/v1\nsource\tanchor\toccurrence\tlevel\ttitle\ndemo\tintro\t1\t1\tIntroduction\n"
	if err := os.WriteFile(indexPath, []byte(index), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"search", "-index", indexPath, "-query", "intro"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(search) code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "demo#intro\tIntroduction\n"; got != want {
		t.Fatalf("run(search) output = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"search", "-index", indexPath, "intro"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(search positional) code = %d, stderr = %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"build", "-id", "demo", "demo"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(build duplicate ID) code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "via a flag or a positional argument") {
		t.Fatalf("run(build duplicate ID) stderr = %q", stderr.String())
	}
}

func TestCLIReportsStableSearchFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"search", "-index", filepath.Join(t.TempDir(), "missing.index"), "query"},
		&stdout, &stderr)
	if code != 1 {
		t.Fatalf("run(search missing index) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "[specs.search.read]") {
		t.Fatalf("run(search missing index) stderr = %q", stderr.String())
	}
}
