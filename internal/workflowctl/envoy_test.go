package workflowctl

import (
	"bytes"
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

func TestEnvoyPromptRejectsSourcePrivateAndStaleInstructions(t *testing.T) {
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
				return value + "\nRead source code at internal/example.go.\n"
			},
			wantErr: "forbidden source",
		},
		{
			name: "private command path",
			mutate: func(value string) string {
				return value + "\nRun go run ./cmd/goxsd9/internal/private.\n"
			},
			wantErr: "forbidden source",
		},
		{
			name: "unapproved command",
			mutate: func(value string) string {
				return strings.Replace(value, "go run .\ngo run ./cmd/goxsd9 parse", "go test ./...\ngo run ./cmd/goxsd9 parse", 1)
			},
			wantErr: "shell command surface",
		},
		{
			name: "private API",
			mutate: func(value string) string {
				return value + "\nCall private API NewValidator().\n"
			},
			wantErr: "forbidden source/private",
		},
		{
			name: "private import",
			mutate: func(value string) string {
				return value + "\nImport github.com/goxdra/goxsd9/internal/example.\n"
			},
			wantErr: "forbidden source/private",
		},
		{
			name: "stale public API claim",
			mutate: func(value string) string {
				return value + "\n`ParseSchema` is unavailable and unevaluated.\n"
			},
			wantErr: "stale claim",
		},
		{
			name: "unavailable legacy API",
			mutate: func(value string) string {
				return value + "\nDo not call goxsd9.ValidateSchema(...).\n"
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

func TestEnvoyPromptRequiresSupportedAPIExecutionAndFixedCommands(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	prompt := readEnvoyTestFile(t, root, envoyPromptPath)
	if err := validateEnvoyPrompt(prompt); err != nil {
		t.Fatalf("validateEnvoyPrompt: %v", err)
	}
	commands, err := envoyShellCommands(prompt)
	if err != nil {
		t.Fatalf("envoyShellCommands: %v", err)
	}
	want := envoyShellCommandSurface()
	if !sameStrings(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestEnvoyReportRejectsMissingBoundaryStaleAndConflatedEvidence(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	report := readEnvoyTestFile(t, root, envoyReportPath)
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "missing source boundary",
			mutate: func(value string) string {
				return strings.Replace(value, "Source inspection: `not used`", "Source inspection: `used`", 1)
			},
			wantErr: "missing required report text",
		},
		{
			name: "inventory execution status",
			mutate: func(value string) string {
				return strings.Replace(value, "Inventory result: `unevaluated`", "Inventory result: `pass`", 1)
			},
			wantErr: "missing required report text",
		},
		{
			name: "inventory execution conflation",
			mutate: func(value string) string {
				return value + "\nInventory is execution evidence.\n"
			},
			wantErr: "conflates catalog inventory",
		},
		{
			name: "stale generator claim",
			mutate: func(value string) string {
				return value + "\n`GenerateGo` is unevaluated.\n"
			},
			wantErr: "stale claim",
		},
		{
			name: "missing API class",
			mutate: func(value string) string {
				return strings.Replace(value, "- Failure class: `api`\n", "", 1)
			},
			wantErr: "missing required report text",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateEnvoyReport(test.mutate(report))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateEnvoyReport error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestEnvoyReportRejectsLongReusableFormat(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	report := readEnvoyTestFile(t, root, envoyReportPath)
	report = strings.TrimSuffix(report, "\n")
	report += strings.Repeat(" extra", 241)
	if err := validateEnvoyReport(report); err == nil || !strings.Contains(err.Error(), "word") {
		t.Fatalf("validateEnvoyReport error = %v, want word limit failure", err)
	}
}

func TestEnvoyFixtureRejectsMissingCatalogAndConsumerMarkers(t *testing.T) {
	sourceRoot := envoyTestRepositoryRoot(t)
	tests := []struct {
		name string
		path string
		from string
		to   string
		want string
	}{
		{
			name: "catalog marker",
			path: "testdata/w3c/xsdtests/suite.xml",
			from: "<testSetRef xlink:href=\"sets/envoy.testSet\"/>",
			to:   "",
			want: "testSetRef",
		},
		{
			name: "consumer generation marker",
			path: "consumer/main.go",
			from: "goxsd9.GenerateGo",
			to:   "goxsd9.Generate",
			want: "goxsd9.GenerateGo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testRoot := t.TempDir()
			copyEnvoyFixture(t, sourceRoot, testRoot)
			path := filepath.Join(testRoot, filepath.FromSlash(envoyFixturePath), filepath.FromSlash(test.path))
			data := readTestFile(t, path)
			data = strings.ReplaceAll(data, test.from, test.to)
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatalf("rewrite fixture: %v", err)
			}
			err := validateEnvoyFixture(testRoot)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateEnvoyFixture error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEnvoyFixtureRejectsPrivateConsumerImportAndGeneratedFile(t *testing.T) {
	sourceRoot := envoyTestRepositoryRoot(t)
	tests := []struct {
		name   string
		mutate func(string) error
		want   string
	}{
		{
			name: "private import",
			mutate: func(root string) error {
				path := filepath.Join(root, filepath.FromSlash(envoyFixturePath), "consumer", "main.go")
				data := readTestFile(t, path)
				data = strings.Replace(data, "\"github.com/goxdra/goxsd9\"", "\"github.com/goxdra/goxsd9/internal/fixture\"", 1)
				return os.WriteFile(path, []byte(data), 0o600)
			},
			want: "not public",
		},
		{
			name: "persistent generated file",
			mutate: func(root string) error {
				path := filepath.Join(root, filepath.FromSlash(envoyFixturePath), "consumer", "generated.go")
				return os.WriteFile(path, []byte("package generated\n"), 0o600)
			},
			want: "unexpected file",
		},
		{
			name: "persistent CLI generated file",
			mutate: func(root string) error {
				path := filepath.Join(root, filepath.FromSlash(envoyFixturePath), envoyCLIFixturePath, "generated.go")
				return os.WriteFile(path, []byte("package generated\n"), 0o600)
			},
			want: "unexpected file",
		},
		{
			name: "noncanonical CLI input",
			mutate: func(root string) error {
				path := filepath.Join(root, filepath.FromSlash(envoyFixturePath), envoyCLIFixturePath, "schema.xsd")
				data := readTestFile(t, path)
				return os.WriteFile(path, []byte(data+"\n"), 0o600)
			},
			want: "differs from canonical",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testRoot := t.TempDir()
			copyEnvoyFixture(t, sourceRoot, testRoot)
			if err := test.mutate(testRoot); err != nil {
				t.Fatalf("mutate fixture: %v", err)
			}
			err := validateEnvoyFixture(testRoot)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateEnvoyFixture error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCheckEnvoyCommandsUsesFixedRepeatedPublicSurface(t *testing.T) {
	root := "/repository"
	fixtureRoot := filepath.Join(root, filepath.FromSlash(envoyFixturePath))
	consumerRoot := filepath.Join(fixtureRoot, filepath.FromSlash(envoyConsumerPath))
	commands := make([]string, 0, 15)
	application := envoyTestCheckApplication(&commands)
	if err := application.checkEnvoyCommands(root); err != nil {
		t.Fatalf("checkEnvoyCommands: %v", err)
	}
	cliCommands := envoyCLICommands()
	want := make([]string, 0, 6+2*len(cliCommands))
	want = append(want,
		fixtureRoot+" go tool conformance inventory -root .",
		fixtureRoot+" go tool conformance inventory -root .",
		root+" go doc .",
		root+" go doc .",
		consumerRoot+" go run .",
		consumerRoot+" go run .",
	)
	for _, command := range cliCommands {
		want = append(want, root+" go "+strings.Join(command.args, " "))
		want = append(want, root+" go "+strings.Join(command.args, " "))
	}
	if len(commands) != len(want)+1 {
		t.Fatalf("commands = %v, want %d command calls plus compilation", commands, len(want))
	}
	if !sameStrings(commands[:len(want)], want) {
		t.Fatalf("commands = %v, want prefix %v", commands, want)
	}
	if !strings.HasSuffix(commands[len(want)], " go test ./...") {
		t.Fatalf("final command = %q; want temporary generated consumer compilation", commands[len(want)])
	}
}

func envoyTestCheckApplication(commands *[]string) app {
	return app{
		ctx: context.Background(),
		executeCommandCapture: func(directory string, _ []string, name string, args ...string) (commandCaptureResult, error) {
			if commands != nil {
				*commands = append(*commands, directory+" "+name+" "+strings.Join(args, " "))
			}
			return envoyTestCapturedCommand(name, args)
		},
	}
}

func envoyTestCapturedCommand(name string, args []string) (commandCaptureResult, error) {
	if name != "go" {
		return commandCaptureResult{}, io.ErrUnexpectedEOF
	}
	if len(args) == 2 && args[0] == "test" && args[1] == "./..." {
		return commandCaptureResult{}, nil
	}
	if len(args) >= 2 && args[0] == "tool" {
		return commandCaptureResult{stdout: envoyTestInventoryOutput()}, nil
	}
	if len(args) == 2 && args[0] == "doc" && args[1] == "." {
		return commandCaptureResult{stdout: envoyTestDocsOutput()}, nil
	}
	if len(args) == 2 && args[0] == "run" && args[1] == "." {
		return commandCaptureResult{stdout: envoyTestConsumerOutput("123")}, nil
	}
	for _, command := range envoyCLICommands() {
		if sameStrings(args, command.args) {
			return envoyTestCLICommandResult(command.step), nil
		}
	}
	return commandCaptureResult{}, io.ErrUnexpectedEOF
}

func TestCheckEnvoyCommandsRejectsNondeterministicConsumerOutput(t *testing.T) {
	consumerRuns := 0
	application := app{
		ctx: context.Background(),
		executeCommandCapture: func(_ string, _ []string, name string, args ...string) (commandCaptureResult, error) {
			result, err := envoyTestCapturedCommand(name, args)
			if err != nil {
				return commandCaptureResult{}, err
			}
			if len(args) == 2 && args[0] == "run" && args[1] == "." {
				consumerRuns++
				if consumerRuns == 2 {
					result.stdout = envoyTestConsumerOutput("124")
				}
			}
			return result, nil
		},
	}
	err := application.checkEnvoyCommands("/repository")
	if err == nil || !strings.Contains(err.Error(), "not deterministic") {
		t.Fatalf("checkEnvoyCommands error = %v, want nondeterministic output failure", err)
	}
}

func TestCheckEnvoyCommandsRejectsUnexpectedCapturedStderr(t *testing.T) {
	application := envoyTestCheckApplication(nil)
	capture := application.executeCommandCapture
	application.executeCommandCapture = func(directory string, env []string, name string, args ...string) (commandCaptureResult, error) {
		result, err := capture(directory, env, name, args...)
		if err == nil && len(args) >= 2 && args[0] == "tool" {
			result.stderr = "unexpected stderr\n"
		}
		return result, err
	}
	err := application.checkEnvoyCommands("/repository")
	if err == nil || !strings.Contains(err.Error(), "inventory metadata stderr") {
		t.Fatalf("checkEnvoyCommands error = %v, want captured-stderr failure", err)
	}
}

func TestCheckEnvoyCommandsRejectsUnexpectedCapturedStatus(t *testing.T) {
	application := envoyTestCheckApplication(nil)
	capture := application.executeCommandCapture
	application.executeCommandCapture = func(directory string, env []string, name string, args ...string) (commandCaptureResult, error) {
		result, err := capture(directory, env, name, args...)
		if err == nil && len(args) == 2 && args[0] == "doc" && args[1] == "." {
			result.status = 7
		}
		return result, err
	}
	err := application.checkEnvoyCommands("/repository")
	if err == nil || !strings.Contains(err.Error(), "public documentation command status") {
		t.Fatalf("checkEnvoyCommands error = %v, want captured-status failure", err)
	}
}

func TestEnvoyConsumerOutputRequiresCompilationEvidence(t *testing.T) {
	output := envoyTestConsumerOutput("123")
	output = strings.Replace(output, "step=compile-generated result=pass", "step=compile-generated result=blocked", 1)
	if err := validateEnvoyConsumerOutput(output); err == nil || !strings.Contains(err.Error(), "compile step") {
		t.Fatalf("validateEnvoyConsumerOutput error = %v, want compilation evidence failure", err)
	}
}

func TestEnvoyReportRejectsMissingStatusMergedStreamsAndStaleCLIClaim(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	report := readEnvoyTestFile(t, root, envoyReportPath)
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "missing status",
			mutate: func(value string) string {
				return strings.Replace(value, "; process status=`0`", "", 1)
			},
			wantErr: "process status",
		},
		{
			name: "merged streams",
			mutate: func(value string) string {
				return value + "\n- Product CLI evidence: combined output was recorded.\n"
			},
			wantErr: "merges stdout and stderr",
		},
		{
			name: "stale CLI claim",
			mutate: func(value string) string {
				return value + "\nProduct CLI is unevaluated.\n"
			},
			wantErr: "stale claim",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateEnvoyReport(test.mutate(report))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateEnvoyReport error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestEnvoyPromptRejectsMergedStreamsAndStaleCLIClaim(t *testing.T) {
	root := envoyTestRepositoryRoot(t)
	prompt := readEnvoyTestFile(t, root, envoyPromptPath)
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "merged streams",
			mutate: func(value string) string {
				return value + "\nMerge stdout and stderr before reporting.\n"
			},
			wantErr: "merges stdout and stderr",
		},
		{
			name: "stale CLI claim",
			mutate: func(value string) string {
				return value + "\nProduct CLI is unevaluated.\n"
			},
			wantErr: "stale claim",
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

func TestEnvoyCLICommandValidationRequiresSeparateStatusAndStreams(t *testing.T) {
	commands := envoyCLICommands()
	for _, command := range commands {
		t.Run(command.step, func(t *testing.T) {
			result := envoyTestCLICommandResult(command.step)
			if err := validateEnvoyCLICommand(command, result); err != nil {
				t.Fatalf("validateEnvoyCLICommand: %v", err)
			}
			result.status = command.expectedStatus + 1
			if err := validateEnvoyCLICommand(command, result); err == nil || !strings.Contains(err.Error(), "status") {
				t.Fatalf("wrong status error = %v", err)
			}
		})
	}
}

func TestCommandCaptureSeparatesStreamsAndStatus(t *testing.T) {
	t.Setenv("ENVOY_CAPTURE_TEST", "wrong")
	application := app{ctx: context.Background()}
	result, err := application.commandCaptureWithEnv(t.TempDir(), []string{"ENVOY_CAPTURE_TEST=ok"},
		"sh", "-c", "printf '%s' \"$ENVOY_CAPTURE_TEST\"; printf stderr >&2; exit 7")
	if err != nil {
		t.Fatalf("commandCaptureWithEnv: %v", err)
	}
	if result.stdout != "ok" || result.stderr != "stderr" || result.status != 7 {
		t.Fatalf("capture = %#v, want separate stdout, stderr, and status", result)
	}
}

func TestSameCommandCaptureComparesEveryField(t *testing.T) {
	base := commandCaptureResult{stdout: "out", stderr: "err", status: 7}
	mutations := []commandCaptureResult{
		{stdout: "changed", stderr: base.stderr, status: base.status},
		{stdout: base.stdout, stderr: "changed", status: base.status},
		{stdout: base.stdout, stderr: base.stderr, status: 8},
	}
	for _, mutation := range mutations {
		if sameCommandCapture(base, mutation) {
			t.Fatalf("sameCommandCapture(%#v, %#v) = true; want changed field to differ", base, mutation)
		}
	}
}

func TestCompileEnvoyGeneratedSourceRejectsAlteration(t *testing.T) {
	source := []byte("package generated\n")
	application := app{
		ctx: context.Background(),
		executeCommandCapture: func(directory string, _ []string, _ string, _ ...string) (commandCaptureResult, error) {
			path := filepath.Join(directory, "generated.go")
			// #nosec G304 -- the test reads the fixed generated.go in its temporary module.
			data, err := os.ReadFile(path)
			if err != nil {
				return commandCaptureResult{}, err
			}
			if !bytes.Equal(data, source) {
				return commandCaptureResult{}, io.ErrUnexpectedEOF
			}
			if err := os.WriteFile(path, []byte("package altered\n"), 0o600); err != nil {
				return commandCaptureResult{}, err
			}
			return commandCaptureResult{}, nil
		},
	}
	err := application.compileEnvoyGeneratedSource("/repository", source)
	if err == nil || !strings.Contains(err.Error(), "changed during compilation") {
		t.Fatalf("compileEnvoyGeneratedSource error = %v, want alteration failure", err)
	}
}

func envoyTestCLICommandResult(step string) commandCaptureResult {
	switch step {
	case "parse":
		return commandCaptureResult{stdout: "documents=1 components=2\n"}
	case "validate-valid":
		return commandCaptureResult{}
	case "validate-invalid":
		return commandCaptureResult{stderr: envoyCLIInvalidStderr, status: 1}
	case "generate":
		return commandCaptureResult{stdout: envoyTestCLIGeneratedOutput()}
	default:
		return commandCaptureResult{status: 99}
	}
}

func envoyTestCLIGeneratedOutput() string {
	return "package envoygenerated\n\nimport Runtime \"github.com/goxdra/goxsd9\"\n\ntype Count struct {\n\tValue Runtime.StrictInteger\n}\n\ntype Amount struct {\n\tValue Runtime.StrictDecimal\n}\n"
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

func envoyTestInventoryOutput() string {
	return "W3C XML Schema test catalog inventory\ntest-sets: 1\ncases: 1\nvalid invalid other submitted accepted stable queried disputed-test disputed-spec status-missing unusable headline\n# Catalog metadata only; no schema or instance tests are executed."
}

func envoyTestDocsOutput() string {
	return "package goxsd9\nfunc ParseSchema(root ResolvedSource, resolver Resolver) (Schema, error)\nfunc ValidateInstance(schema Schema, sourceID SourceID, reader io.ReadCloser) error\nfunc GenerateGo(schema Schema, packageName string) ([]byte, error)\ntype Diagnostic struct\nconst InvalidIntegerLexicalCode = \"XSD2001\"\ntype FailureClass string\nfunc (diagnostic Diagnostic) Loc() Loc\nfunc (diagnostic Diagnostic) Related() []Loc\nfunc (diagnostic Diagnostic) SpecRef() string\n"
}

func envoyTestConsumerOutput(bytes string) string {
	return "step=parse result=pass\nstep=validate-valid result=pass\nstep=validate-invalid result=pass class=invalid code=XSD2001 loc=invalid.xml:1:34 related=schema.xsd:2:3 spec_ref=xsd11-datatypes#integer\nstep=generate-repeat result=pass bytes=" + bytes + "\nstep=compile-generated result=pass"
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
	fixtureFiles := make([]envoyFixtureFile, 0, len(envoyFixtureFiles)+len(envoyCLIFixtureFiles))
	fixtureFiles = append(fixtureFiles, envoyFixtureFiles...)
	fixtureFiles = append(fixtureFiles, envoyCLIFixtureFiles...)
	for _, fixtureFile := range fixtureFiles {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(envoyFixturePath), filepath.FromSlash(fixtureFile.path))
		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(envoyFixturePath), filepath.FromSlash(fixtureFile.path))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
			t.Fatalf("make fixture directory: %v", err)
		}
		if err := os.WriteFile(destinationPath, []byte(readTestFile(t, sourcePath)), 0o600); err != nil {
			t.Fatalf("copy %s: %v", fixtureFile.path, err)
		}
	}
	for _, relativePath := range []string{"examples/root.xsd", "examples/valid.xml", "examples/invalid.xml"} {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relativePath))
		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
			t.Fatalf("make canonical example directory: %v", err)
		}
		if err := os.WriteFile(destinationPath, []byte(readTestFile(t, sourcePath)), 0o600); err != nil {
			t.Fatalf("copy canonical example %s: %v", relativePath, err)
		}
	}
}
