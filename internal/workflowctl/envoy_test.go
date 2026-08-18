package workflowctl

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvoySurfaceAssetsAreValid(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	if err := validateEnvoySurfaceAssets(root); err != nil {
		t.Fatalf("validateEnvoySurfaceAssets: %v", err)
	}
}

func TestEnvoyPromptRejectsSourceInspection(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	prompt := readEnvoyTestFile(t, root, envoyPromptPath)
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "source command",
			mutate: func(value string) string {
				return strings.Replace(value, "go doc .", "go doc -src .", 1)
			},
			wantErr: "forbidden source",
		},
		{
			name: "source path",
			mutate: func(value string) string {
				return strings.Replace(value, "Record the result", "Read source code at internal/example.go.\n\nRecord the result", 1)
			},
			wantErr: "forbidden source",
		},
		{
			name: "unapproved command",
			mutate: func(value string) string {
				return strings.Replace(value, "go doc .\n```", "go test ./...\n```", 1)
			},
			wantErr: "shell command surface",
		},
		{
			name: "unavailable API",
			mutate: func(value string) string {
				return value + "\nDo not call goxsd9.ParseSchema(...).\n"
			},
			wantErr: "unavailable API",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateEnvoyPrompt(test.mutate(prompt))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateEnvoyPrompt error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestEnvoyPromptAllowsDatatypeNames(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	prompt := readEnvoyTestFile(t, root, envoyPromptPath)
	prompt += "\nThe public datatype names ParseStrictInteger and ValidateDecimalFacets are allowed.\n"
	if err := validateEnvoyPrompt(prompt); err != nil {
		t.Fatalf("validateEnvoyPrompt rejected legitimate datatype names: %v", err)
	}
}

func TestEnvoyReportRejectsLongReusableFormat(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	report := readEnvoyTestFile(t, root, envoyReportPath)
	report += strings.Repeat(" extra", 241)
	if err := validateEnvoyReport(report); err == nil || !strings.Contains(err.Error(), "word") {
		t.Fatalf("validateEnvoyReport error = %v, want word limit failure", err)
	}
}

func TestEnvoyFixtureRejectsMissingCatalogMarker(t *testing.T) {
	sourceRoot := envoyTestRepositoryRoot(t)
	testRoot := t.TempDir()
	copyEnvoyFixture(t, sourceRoot, testRoot)
	path := filepath.Join(testRoot, filepath.FromSlash(envoyFixturePath), filepath.FromSlash(envoyFixtureFiles[0].path))
	data := readTestFile(t, path)
	data = strings.Replace(data, "<testSetRef xlink:href=\"sets/envoy.testSet\"/>", "", 1)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	if err := validateEnvoyFixture(testRoot); err == nil || !strings.Contains(err.Error(), "testSetRef") {
		t.Fatalf("validateEnvoyFixture error = %v, want missing marker failure", err)
	}
}

func TestCheckEnvoyCommandsUsesOnlyDocumentedSurface(t *testing.T) {
	root := "/repository"
	fixtureOutput := "W3C XML Schema test catalog inventory\ntest-sets: 1\ncases: 1\n# Catalog metadata only; no schema or instance tests are executed."
	commands := make([]string, 0, 3)
	application := app{
		ctx: context.Background(),
		executeCommand: func(directory string, _ io.Reader, name string, args ...string) (string, error) {
			commands = append(commands, directory+" "+name+" "+strings.Join(args, " "))
			if name == "go" && len(args) >= 2 && args[0] == "tool" {
				return fixtureOutput, nil
			}
			if name == "go" && len(args) == 2 && args[0] == "doc" && args[1] == "." {
				return "package goxsd9\n", nil
			}
			return "", io.ErrUnexpectedEOF
		},
	}
	if err := application.checkEnvoyCommands(root); err != nil {
		t.Fatalf("checkEnvoyCommands: %v", err)
	}
	if len(commands) != 3 {
		t.Fatalf("commands = %v, want two inventory runs and one go doc run", commands)
	}
	if !strings.HasSuffix(commands[0], "go tool conformance inventory -root .") ||
		!strings.HasSuffix(commands[1], "go tool conformance inventory -root .") ||
		!strings.HasSuffix(commands[2], "go doc .") {
		t.Fatalf("commands = %v, want documented command surface", commands)
	}
}

func TestQualityChecksIncludeEnvoySurface(t *testing.T) {
	checks := (app{}).qualityChecks(t.TempDir(), true)
	for _, check := range checks {
		if check.name == "Envoy surface" {
			return
		}
	}
	t.Fatal("quality checks do not include Envoy surface")
}

func TestEnvoyMarkdownIsNotDurableDocumentation(t *testing.T) {
	if isDurableMarkdown(envoyPromptPath) {
		t.Fatalf("%s is treated as durable documentation", envoyPromptPath)
	}
	if isDurableMarkdown(envoyReportPath) {
		t.Fatalf("%s is treated as durable documentation", envoyReportPath)
	}
}

func envoyTestRepositoryRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get test working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
}

func readEnvoyTestFile(t *testing.T, root, relativePath string) string {
	t.Helper()
	return readTestFile(t, filepath.Join(root, filepath.FromSlash(relativePath)))
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- tests pass paths created under their temporary or repository fixture roots.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func copyEnvoyFixture(t *testing.T, sourceRoot, destinationRoot string) {
	t.Helper()
	for _, fixtureFile := range envoyFixtureFiles {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(envoyFixturePath), filepath.FromSlash(fixtureFile.path))
		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(envoyFixturePath), filepath.FromSlash(fixtureFile.path))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
			t.Fatalf("make fixture directory: %v", err)
		}
		if err := os.WriteFile(destinationPath, []byte(readTestFile(t, sourcePath)), 0o600); err != nil {
			t.Fatalf("copy %s: %v", fixtureFile.path, err)
		}
	}
}
