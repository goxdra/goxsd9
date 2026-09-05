package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9/internal/specs"
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

func TestBootstrapPrintsPinnedPlans(t *testing.T) {
	root := copyPinnedManifest(t)
	tests := []struct {
		version string
		want    string
	}{
		{
			version: "1.0",
			want: "version\t1.0\n" +
				"id\trole\trepresentation\tdependencies\tlexical_aliases\n" +
				"xsd10-datatypes-schema\tdependency\thtml-cdata-pre-xsd10-datatypes\t[]\t[]\n" +
				"xml-schema\tdependency\txml\t[]\t[\"http://www.w3.org/2001/xml.xsd\"]\n" +
				"xsd10-schema-for-schemas\tentry\txml\t[\"xsd10-datatypes-schema\",\"xml-schema\"]\t[]\n",
		},
		{
			version: "1.1",
			want: "version\t1.1\n" +
				"id\trole\trepresentation\tdependencies\tlexical_aliases\n" +
				"xsd11-datatypes-schema\tdependency\txml\t[]\t[]\n" +
				"xml-schema\tdependency\txml\t[]\t[\"http://www.w3.org/2001/xml.xsd\"]\n" +
				"xsd11-schema-for-schemas\tentry\txml\t[\"xsd11-datatypes-schema\",\"xml-schema\"]\t[]\n",
		},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := []string{"bootstrap", "-version", test.version, "-root", root}
			code := run(args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run(bootstrap %s) code = %d, stderr = %q", test.version, code, stderr.String())
			}
			if got := stdout.String(); got != test.want {
				t.Fatalf("run(bootstrap %s) output = %q, want %q", test.version, got, test.want)
			}

			stdout.Reset()
			stderr.Reset()
			code = run(args, &stdout, &stderr)
			if code != 0 || stdout.String() != test.want || stderr.Len() != 0 {
				t.Fatalf("repeated run(bootstrap %s) = code %d, stdout %q, stderr %q", test.version, code, stdout.String(), stderr.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap created .cache: stat error = %v", err)
	}
}

func TestBootstrapUsageFailuresDoNotWriteStdout(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantOutput string
	}{
		{
			name:       "missing version",
			args:       []string{"bootstrap"},
			wantOutput: "bootstrap requires -version VERSION",
		},
		{
			name:       "bad flag",
			args:       []string{"bootstrap", "-version", "1.0", "-unknown"},
			wantOutput: "flag provided but not defined: -unknown",
		},
		{
			name:       "extra operand",
			args:       []string{"bootstrap", "-version", "1.0", "extra"},
			wantOutput: "bootstrap takes no positional arguments",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 2 {
				t.Fatalf("run(%v) code = %d, want 2; stderr = %q", test.args, code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("run(%v) stdout = %q, want empty", test.args, stdout.String())
			}
			if !strings.Contains(stderr.String(), test.wantOutput) {
				t.Fatalf("run(%v) stderr = %q, want %q", test.args, stderr.String(), test.wantOutput)
			}
		})
	}
}

func TestBootstrapFailuresPreserveDiagnosticsAndLeaveStdoutEmpty(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*testing.T) string
		args       func(string) []string
		wantOutput string
	}{
		{
			name: "unknown version",
			setup: func(t *testing.T) string {
				return writeCommandManifest(t, commandManifest())
			},
			args: func(root string) []string {
				return []string{"bootstrap", "-version", "2.0", "-root", root}
			},
			wantOutput: `[specs.bootstrap.version] unsupported bootstrap XSD version "2.0"`,
		},
		{
			name: "invalid root",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args: func(root string) []string {
				return []string{"bootstrap", "-version", "1.0", "-root", root}
			},
			wantOutput: "inspect specification manifest",
		},
		{
			name: "malformed manifest",
			setup: func(t *testing.T) string {
				root := t.TempDir()
				writeManifestData(t, root, []byte("{"))
				return root
			},
			args: func(root string) []string {
				return []string{"bootstrap", "-version", "1.0", "-root", root}
			},
			wantOutput: "[specs.manifest.decode]",
		},
		{
			name: "bootstrap graph",
			setup: func(t *testing.T) string {
				manifest := commandManifest()
				manifest.BootstrapArtifacts[1].Dependencies = []string{"xsd10-entry"}
				return writeCommandManifest(t, manifest)
			},
			args: func(root string) []string {
				return []string{"bootstrap", "-version", "1.0", "-root", root}
			},
			wantOutput: `[specs.bootstrap.cycle] xsd10-entry`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := test.setup(t)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := run(test.args(root), &stdout, &stderr); code != 1 {
				t.Fatalf("run(bootstrap %s) code = %d, want 1; stderr = %q", test.name, code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("run(bootstrap %s) stdout = %q, want empty", test.name, stdout.String())
			}
			if !strings.Contains(stderr.String(), test.wantOutput) {
				t.Fatalf("run(bootstrap %s) stderr = %q, want %q", test.name, stderr.String(), test.wantOutput)
			}
		})
	}
}

func TestBootstrapReportsStdoutWriteFailure(t *testing.T) {
	root := writeCommandManifest(t, commandManifest())
	stdout := &bootstrapWriteError{err: errors.New("writer failed")}
	var stderr bytes.Buffer
	code := run([]string{"bootstrap", "-version", "1.0", "-root", root}, stdout, &stderr)
	if code != 1 {
		t.Fatalf("run(bootstrap write failure) code = %d, want 1; stderr = %q", code, stderr.String())
	}
	if stdout.calls != 1 {
		t.Fatalf("bootstrap stdout writes = %d, want 1", stdout.calls)
	}
	if !strings.Contains(stderr.String(), "write bootstrap plan: writer failed") {
		t.Fatalf("run(bootstrap write failure) stderr = %q", stderr.String())
	}
}

type bootstrapWriteError struct {
	calls int
	err   error
}

func (writer *bootstrapWriteError) Write([]byte) (int, error) {
	writer.calls++
	return 0, writer.err
}

func copyPinnedManifest(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testRepositoryRoot(t), "specs", "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(pinned manifest) error = %v", err)
	}
	root := t.TempDir()
	writeManifestData(t, root, data)
	return root
}

func testRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func writeCommandManifest(t *testing.T, manifest specs.Manifest) string {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	root := t.TempDir()
	writeManifestData(t, root, data)
	return root
}

func writeManifestData(t *testing.T, root string, data []byte) {
	t.Helper()
	manifestDirectory := filepath.Join(root, "specs")
	if err := os.MkdirAll(manifestDirectory, 0o750); err != nil {
		t.Fatalf("MkdirAll(manifest directory) error = %v", err)
	}
	// #nosec G703 -- test roots are temporary directories created by testing.
	if err := os.WriteFile(filepath.Join(manifestDirectory, "manifest.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
}

func commandManifest() specs.Manifest {
	digest := strings.Repeat("a", 64)
	return specs.Manifest{
		FormatVersion: 1,
		Specifications: []specs.Source{{
			ID:          "example-spec",
			Title:       "Example specification",
			URL:         "https://www.w3.org/TR/2020/example/",
			SHA256:      digest,
			XSDVersions: []string{"1.1"},
		}},
		Errata: []specs.Source{{
			ID:          "example-errata",
			Title:       "Example errata",
			URL:         "https://www.w3.org/2004/03/example-errata",
			SHA256:      digest,
			XSDVersions: []string{"1.0"},
		}},
		BootstrapArtifacts: []specs.BootstrapArtifact{
			{
				ID:             "xsd10-entry",
				Title:          "XSD 1.0 entry",
				URL:            "https://www.w3.org/TR/2004/REC-example/XMLSchema.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.0"},
				Representation: "xml",
				Entry:          true,
				Dependencies:   []string{"xsd10-datatypes", "xml-schema"},
			},
			{
				ID:             "xsd10-datatypes",
				Title:          "XSD 1.0 datatypes",
				URL:            "https://www.w3.org/TR/2004/REC-example/datatypes.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.0"},
				Representation: "html-cdata-pre",
			},
			{
				ID:             "xsd11-entry",
				Title:          "XSD 1.1 entry",
				URL:            "https://www.w3.org/TR/2012/REC-example/XMLSchema.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.1"},
				Representation: "xml",
				Entry:          true,
				Dependencies:   []string{"xsd11-datatypes", "xml-schema"},
			},
			{
				ID:             "xsd11-datatypes",
				Title:          "XSD 1.1 datatypes",
				URL:            "https://www.w3.org/TR/2012/REC-example/datatypes.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.1"},
				Representation: "xml",
			},
			{
				ID:             "xml-schema",
				Title:          "XML namespace schema",
				URL:            "https://www.w3.org/2001/xml.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.0", "1.1"},
				Representation: "xml",
				Aliases:        []string{"http://www.w3.org/2001/xml.xsd"},
			},
		},
	}
}
