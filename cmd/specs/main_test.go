package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
	manifest, err := specs.ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	tests := []struct {
		version string
		rows    []string
	}{
		{
			version: "1.0",
			rows: []string{
				"xsd10-datatypes-schema\tdependency\thtml-cdata-pre-xsd10-datatypes\t[]\t[]",
				"xml-schema\tdependency\txml\t[]\t[\"http://www.w3.org/2001/xml.xsd\"]",
				"xsd10-schema-for-schemas\tentry\txml\t[\"xsd10-datatypes-schema\",\"xml-schema\"]\t[]",
			},
		},
		{
			version: "1.1",
			rows: []string{
				"xsd11-datatypes-schema\tdependency\txml\t[]\t[]",
				"xml-schema\tdependency\txml\t[]\t[\"http://www.w3.org/2001/xml.xsd\"]",
				"xsd11-schema-for-schemas\tentry\txml\t[\"xsd11-datatypes-schema\",\"xml-schema\"]\t[]",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			assertPinnedBootstrapPlan(t, root, manifest, test.version, test.rows)
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap created .cache: stat error = %v", err)
	}
}

func assertPinnedBootstrapPlan(t *testing.T, root string, manifest specs.Manifest, version string, rows []string) {
	t.Helper()
	output := runBootstrapOutput(t, root, version)
	lines := bootstrapPlanLines(t, output, version, len(rows))
	for index, want := range rows {
		fields := strings.Split(lines[index+2], "\t")
		if got := len(fields); got != 6 {
			t.Fatalf("row %d fields = %d, want 6: %q", index, got, lines[index+2])
		}
		if got := strings.Join(fields[:5], "\t"); got != want {
			t.Fatalf("row %d = %q, want %q", index, got, want)
		}
		assertManifestDigest(t, manifest, fields[0], fields[5], index)
	}
	if repeated := runBootstrapOutput(t, root, version); repeated != output {
		t.Fatalf("repeated run(bootstrap %s) output = %q, want %q", version, repeated, output)
	}
}

func TestBootstrapPrintsDistinctManifestDigestsOnMatchingRows(t *testing.T) {
	manifest := commandManifest()
	for index := range manifest.BootstrapArtifacts {
		manifest.BootstrapArtifacts[index].SHA256 = strings.Repeat(string(rune('a'+index)), 64)
	}
	root := writeCommandManifest(t, manifest)
	for _, test := range []struct {
		version string
		ids     []string
	}{
		{"1.0", []string{"xsd10-datatypes", "xml-schema", "xsd10-entry"}},
		{"1.1", []string{"xsd11-datatypes", "xml-schema", "xsd11-entry"}},
	} {
		t.Run(test.version, func(t *testing.T) {
			output := runBootstrapOutput(t, root, test.version)
			lines := bootstrapPlanLines(t, output, test.version, len(test.ids))
			for index, id := range test.ids {
				fields := strings.Split(lines[index+2], "\t")
				if len(fields) != 6 || fields[0] != id {
					t.Fatalf("row %d = %q, want ID %q and six fields", index, lines[index+2], id)
				}
				assertManifestDigest(t, manifest, id, fields[5], index)
			}
		})
	}
}

func runBootstrapOutput(t *testing.T, root, version string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"bootstrap", "-version", version, "-root", root}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("run(bootstrap %s) code = %d, stderr = %q", version, code, stderr.String())
	}
	return stdout.String()
}

func bootstrapPlanLines(t *testing.T, output, version string, rows int) []string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if got, want := len(lines), rows+2; got != want {
		t.Fatalf("run(bootstrap %s) lines = %d, want %d: %q", version, got, want, output)
	}
	if got, want := lines[0], "version\t"+version; got != want {
		t.Fatalf("version line = %q, want %q", got, want)
	}
	if got, want := lines[1], "id\trole\trepresentation\tdependencies\tlexical_aliases\tmanifest_sha256"; got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
	return lines
}

func assertManifestDigest(t *testing.T, manifest specs.Manifest, id, digest string, index int) {
	t.Helper()
	entry, err := manifest.Find(id)
	if err != nil {
		t.Fatalf("Find(%q) error = %v", id, err)
	}
	if digest != entry.SHA256 {
		t.Fatalf("row %d digest = %q, manifest digest = %q", index, digest, entry.SHA256)
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

func TestBootstrapUsageDescribesManifestDigest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"bootstrap"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(bootstrap) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "manifest_sha256 is the manifest-pinned SHA-256 digest of the raw response") {
		t.Fatalf("usage omits column meaning: %q", stderr.String())
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
			name: "malformed manifest digest",
			setup: func(t *testing.T) string {
				manifest := commandManifest()
				manifest.BootstrapArtifacts[1].SHA256 = "not-a-sha256-digest"
				return writeCommandManifest(t, manifest)
			},
			args: func(root string) []string {
				return []string{"bootstrap", "-version", "1.0", "-root", root}
			},
			wantOutput: "[specs.manifest.validate]",
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
	for _, test := range []struct {
		name   string
		writer *bootstrapWriteError
		cause  string
	}{
		{"writer error", &bootstrapWriteError{err: errors.New("writer failed")}, "writer failed"},
		{"short write", &bootstrapWriteError{}, io.ErrShortWrite.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run([]string{"bootstrap", "-version", "1.0", "-root", root}, test.writer, &stderr)
			if code != 1 {
				t.Fatalf("run(bootstrap write failure) code = %d, want 1; stderr = %q", code, stderr.String())
			}
			if test.writer.calls != 1 {
				t.Fatalf("bootstrap stdout writes = %d, want one attempted write", test.writer.calls)
			}
			if got, want := stderr.String(), "specs: write bootstrap plan: "+test.cause+"\n"; got != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
		})
	}
}

type bootstrapWriteError struct {
	calls int
	err   error
}

func (writer *bootstrapWriteError) Write([]byte) (int, error) {
	writer.calls++
	if writer.err == nil {
		return 0, nil
	}
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
