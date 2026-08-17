package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFuzzProcessDurationUsesFixedMargin(t *testing.T) {
	if got, want := fuzzProcessDuration(2*time.Second), 32*time.Second; got != want {
		t.Fatalf("fuzz process duration = %s, want %s", got, want)
	}
}

func TestRunFuzzUsesOfflineSingleWorkerAndCleansSuccessfulSandbox(t *testing.T) {
	root := newFuzzFixture(t)
	var output bytes.Buffer
	capture := fuzzCommandCapture{}
	application := fuzzTestApplication(t, root, &output)
	application.executeCommandWithContextAndEnv = captureFuzzCommand(t, &capture)

	if err := application.runFuzz([]string{
		"--package", ".", "--target", "FuzzFixture", "--duration", "250ms",
	}); err != nil {
		t.Fatalf("runFuzz: %v", err)
	}
	assertFuzzCommandCapture(t, capture, root)
	if got := readFuzzFixtureSentinel(t, root); got != "sentinel" {
		t.Fatalf("repository sentinel = %q, want unchanged sentinel", got)
	}
	wantOutput := strings.Join([]string{
		"package: .",
		"target: FuzzFixture",
		"duration: 250ms",
		"workers: 1",
		"offline: true",
		"result: success",
		"replay command: go test . -count=1 -run='^FuzzFixture$'",
		"",
	}, "\n")
	if output.String() != wantOutput {
		t.Fatalf("success report = %q, want %q", output.String(), wantOutput)
	}
}

type fuzzCommandCapture struct {
	deadlineSet bool
	directory   string
	environment []string
	arguments   []string
}

func captureFuzzCommand(t *testing.T, capture *fuzzCommandCapture) commandContextEnvironmentExecutor {
	t.Helper()
	return func(ctx context.Context, directory string, environment []string, _ io.Reader, name string,
		args ...string,
	) (string, error) {
		if name != "go" {
			t.Fatalf("campaign command = %q, want go", name)
		}
		_, capture.deadlineSet = ctx.Deadline()
		capture.directory = directory
		capture.environment = append([]string(nil), environment...)
		capture.arguments = append([]string(nil), args...)
		for _, temporaryDirectory := range []string{filepath.Join(filepath.Dir(directory), "cache"), filepath.Join(filepath.Dir(directory), "tmp")} {
			if _, err := os.Stat(temporaryDirectory); err != nil {
				t.Fatalf("campaign directory %q was not created: %v", temporaryDirectory, err)
			}
		}
		return "", nil
	}
}

func assertFuzzCommandCapture(t *testing.T, capture fuzzCommandCapture, root string) {
	t.Helper()
	if !capture.deadlineSet {
		t.Fatal("fuzz command did not receive a deadline")
	}
	wantArguments := []string{
		"test", ".", "-run=^$", "-fuzz=^FuzzFixture$", "-fuzztime=250ms", "-parallel=1", "-p=1",
	}
	if !reflect.DeepEqual(capture.arguments, wantArguments) {
		t.Fatalf("campaign arguments = %#v, want %#v", capture.arguments, wantArguments)
	}
	wantEnvironment := append(fuzzGoEnvironment(),
		"GOCACHE="+filepath.Join(filepath.Dir(capture.directory), "cache"),
		"GOTMPDIR="+filepath.Join(filepath.Dir(capture.directory), "tmp"),
	)
	if !reflect.DeepEqual(capture.environment, wantEnvironment) {
		t.Fatalf("campaign environment = %#v, want %#v", capture.environment, wantEnvironment)
	}
	if capture.directory == root || filepath.Base(capture.directory) != "source" {
		t.Fatalf("campaign directory = %q, want temporary source copy outside %q", capture.directory, root)
	}
	if _, err := os.Stat(filepath.Dir(capture.directory)); !os.IsNotExist(err) {
		t.Fatalf("successful campaign sandbox remains: %v", err)
	}
}

func TestRunFuzzRepeatedSuccessesHaveStableReports(t *testing.T) {
	root := newFuzzFixture(t)
	reports := make([]string, 0, 2)
	for range 2 {
		var output bytes.Buffer
		application := fuzzTestApplication(t, root, &output)
		application.executeCommandWithContextAndEnv = func(_ context.Context, _ string, _ []string,
			_ io.Reader, name string, _ ...string,
		) (string, error) {
			if name != "go" {
				t.Fatalf("campaign command = %q, want go", name)
			}
			return "", nil
		}
		if err := application.runFuzz([]string{
			"--package", ".", "--target", "FuzzFixture", "--duration", "250ms",
		}); err != nil {
			t.Fatalf("runFuzz: %v", err)
		}
		reports = append(reports, output.String())
	}
	if reports[0] != reports[1] {
		t.Fatalf("repeated success reports differ:\n%s\n---\n%s", reports[0], reports[1])
	}
}

func TestRunFuzzRejectsInvalidInputBeforeRepositoryInspection(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
	}{
		{name: "missing package", arguments: []string{"--target", "FuzzFixture", "--duration", "1s"}},
		{name: "missing target", arguments: []string{"--package", ".", "--duration", "1s"}},
		{name: "missing duration", arguments: []string{"--package", ".", "--target", "FuzzFixture"}},
		{name: "zero duration", arguments: []string{"--package", ".", "--target", "FuzzFixture", "--duration=0"}},
		{name: "negative duration", arguments: []string{"--package", ".", "--target", "FuzzFixture", "--duration=-1s"}},
		{name: "unbounded duration", arguments: []string{"--package", ".", "--target", "FuzzFixture", "--duration", "N"}},
		{name: "non fuzz target", arguments: []string{"--package", ".", "--target", "TestFixture", "--duration", "1s"}},
		{name: "target injection", arguments: []string{"--package", ".", "--target", "FuzzFixture;touch-sentinel", "--duration", "1s"}},
		{name: "package wildcard", arguments: []string{"--package", "./...", "--target", "FuzzFixture", "--duration", "1s"}},
		{name: "package injection", arguments: []string{"--package", ".;touch-sentinel", "--target", "FuzzFixture", "--duration", "1s"}},
		{name: "duplicate package", arguments: []string{"--package", ".", "--package", "./internal", "--target", "FuzzFixture", "--duration", "1s"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inspected := false
			application := app{executeCommand: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
				inspected = true
				return "", errors.New("repository inspection was not expected")
			}}
			err := application.runFuzz(test.arguments)
			if err == nil {
				t.Fatal("invalid input was accepted")
			}
			if inspected {
				t.Fatal("invalid input reached repository inspection")
			}
		})
	}
}

func TestRunFuzzRejectsAbsentAmbiguousAndInvalidTargetsBeforeSandbox(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		additional []fuzzFixtureFile
		want       string
	}{
		{name: "absent", target: "FuzzMissing", want: "absent"},
		{
			name:   "ambiguous",
			target: "FuzzFixture",
			additional: []fuzzFixtureFile{{
				name: "duplicate_test.go",
				content: `package fixture

import "testing"

func FuzzFixture(f *testing.F) {}
`,
			}},
			want: "ambiguous",
		},
		{
			name:   "invalid signature",
			target: "FuzzWrong",
			additional: []fuzzFixtureFile{{
				name: "wrong_test.go",
				content: `package fixture

func FuzzWrong(value string) {}
`,
			}},
			want: "invalid fuzz signature",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newFuzzFixtureWithFiles(t, test.additional)
			sandboxCreates := 0
			application := fuzzTestApplication(t, root, io.Discard)
			application.fuzzMakeTempDir = func(string) (string, error) {
				sandboxCreates++
				return t.TempDir(), nil
			}
			err := application.runFuzz([]string{
				"--package", ".", "--target", test.target, "--duration", "1s",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runFuzz error = %v, want %q", err, test.want)
			}
			if sandboxCreates != 0 {
				t.Fatalf("sandbox creation count = %d, want 0", sandboxCreates)
			}
		})
	}
}

func TestRunFuzzFailureRetainsEvidenceAndStableReplay(t *testing.T) {
	root := newFuzzFixture(t)
	replayReports := make([]string, 0, 2)
	for range 2 {
		replayReports = append(replayReports, runFuzzFailureIteration(t, root))
	}
	if replayReports[0] != replayReports[1] {
		t.Fatalf("replay instructions differ: %#v", replayReports)
	}
}

func runFuzzFailureIteration(t *testing.T, root string) string {
	t.Helper()
	const corpusName = "hash123"
	var output bytes.Buffer
	var evidenceRoot string
	application := fuzzTestApplication(t, root, &output)
	application.executeCommandWithContextAndEnv = func(_ context.Context, directory string, _ []string,
		_ io.Reader, name string, _ ...string,
	) (string, error) {
		if name != "go" {
			t.Fatalf("campaign command = %q, want go", name)
		}
		evidenceRoot = filepath.Dir(directory)
		corpusDirectory := filepath.Join(directory, "testdata", "fuzz", "FuzzFixture")
		if err := os.MkdirAll(corpusDirectory, 0o700); err != nil {
			t.Fatalf("create fake corpus: %v", err)
		}
		corpusPath := filepath.Join(corpusDirectory, corpusName)
		if err := os.WriteFile(corpusPath, []byte("replay input"), 0o600); err != nil {
			t.Fatalf("write fake corpus: %v", err)
		}
		return "Failing input written to " + corpusPath + "\n", fuzzTestExitError(t, 7)
	}
	err := application.runFuzz([]string{
		"--package", ".", "--target", "FuzzFixture", "--duration", "1s",
	})
	assertFuzzProcessExit(t, err, output.String())
	if !strings.Contains(output.String(), "replay command: go test . -count=1 -run='^FuzzFixture/hash123$'") {
		t.Fatalf("failure report omitted stable corpus replay:\n%s", output.String())
	}
	if evidenceRoot == "" {
		t.Fatal("failure did not report an evidence root")
	}
	if _, err := os.Stat(filepath.Join(evidenceRoot, "source", "testdata", "fuzz", "FuzzFixture", corpusName)); err != nil {
		t.Fatalf("retained corpus stat: %v", err)
	}
	if got := readFuzzFixtureSentinel(t, root); got != "sentinel" {
		t.Fatalf("repository sentinel = %q, want unchanged sentinel", got)
	}
	replay := fuzzReportLine(output.String(), "replay command:")
	if err := os.RemoveAll(evidenceRoot); err != nil {
		t.Fatalf("remove test evidence: %v", err)
	}
	return replay
}

func assertFuzzProcessExit(t *testing.T, err error, output string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "WFZ1006") || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("runFuzz error = %v, want process exit diagnostic", err)
	}
	var diagnostic *fuzzDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.code != fuzzProcessExitCode {
		t.Fatalf("runFuzz diagnostic = %#v, want process exit code", diagnostic)
	}
	if !strings.Contains(err.Error(), "Failing input written to") {
		t.Fatalf("runFuzz error omitted child output: %v", err)
	}
	if output == "" {
		t.Fatal("failure report is empty")
	}
}

func TestRunFuzzReportsTimeoutAndProcessStartFailures(t *testing.T) {
	tests := []struct {
		name       string
		childError error
		wantCode   string
		wantResult string
	}{
		{name: "timeout", childError: context.DeadlineExceeded, wantCode: fuzzTimeoutCode, wantResult: "result: timeout"},
		{name: "start", childError: &exec.Error{Name: "go", Err: os.ErrNotExist}, wantCode: fuzzProcessStartCode, wantResult: "result: process-start-failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runFuzzProcessFailureCase(t, test)
		})
	}
}

func runFuzzProcessFailureCase(t *testing.T, test struct {
	name       string
	childError error
	wantCode   string
	wantResult string
}) {
	t.Helper()
	root := newFuzzFixture(t)
	var output bytes.Buffer
	var evidenceRoot string
	application := fuzzTestApplication(t, root, &output)
	application.executeCommandWithContextAndEnv = func(_ context.Context, directory string, _ []string,
		_ io.Reader, _ string, _ ...string,
	) (string, error) {
		evidenceRoot = filepath.Dir(directory)
		return "fake child output", test.childError
	}
	err := application.runFuzz([]string{
		"--package", ".", "--target", "FuzzFixture", "--duration", "1s",
	})
	if err == nil || !strings.Contains(err.Error(), test.wantCode) {
		t.Fatalf("runFuzz error = %v, want %s", err, test.wantCode)
	}
	if !strings.Contains(output.String(), test.wantResult) {
		t.Fatalf("report = %q, want %q", output.String(), test.wantResult)
	}
	if !strings.Contains(err.Error(), "fake child output") {
		t.Fatalf("diagnostic omitted child output: %v", err)
	}
	if evidenceRoot == "" {
		t.Fatal("failure did not retain an evidence root")
	}
	if err := os.RemoveAll(evidenceRoot); err != nil {
		t.Fatalf("remove test evidence: %v", err)
	}
}

func TestRunFuzzReportsSignaledProcessFailure(t *testing.T) {
	root := newFuzzFixture(t)
	var output bytes.Buffer
	var evidenceRoot string
	application := fuzzTestApplication(t, root, &output)
	application.executeCommandWithContextAndEnv = func(_ context.Context, directory string, _ []string,
		_ io.Reader, _ string, _ ...string,
	) (string, error) {
		evidenceRoot = filepath.Dir(directory)
		return "signal output", fuzzTestSignalError(t)
	}
	err := application.runFuzz([]string{
		"--package", ".", "--target", "FuzzFixture", "--duration", "1s",
	})
	if err == nil || !strings.Contains(err.Error(), fuzzProcessSignalCode) {
		t.Fatalf("runFuzz error = %v, want signal diagnostic", err)
	}
	if !strings.Contains(output.String(), "result: process-signaled") {
		t.Fatalf("signal report = %q", output.String())
	}
	if !strings.Contains(err.Error(), "signal output") {
		t.Fatalf("signal diagnostic omitted child output: %v", err)
	}
	if evidenceRoot == "" {
		t.Fatal("signal failure did not retain evidence")
	}
	if err := os.RemoveAll(evidenceRoot); err != nil {
		t.Fatalf("remove test evidence: %v", err)
	}
}

func TestRunFuzzReportsCleanupFailure(t *testing.T) {
	root := newFuzzFixture(t)
	var output bytes.Buffer
	var sandbox string
	cleanupFailure := errors.New("cleanup failed")
	application := fuzzTestApplication(t, root, &output)
	application.executeCommandWithContextAndEnv = func(_ context.Context, directory string, _ []string,
		_ io.Reader, _ string, _ ...string,
	) (string, error) {
		sandbox = filepath.Dir(directory)
		return "", nil
	}
	application.fuzzRemoveAll = func(directory string) error {
		if directory != sandbox {
			t.Fatalf("cleanup directory = %q, want %q", directory, sandbox)
		}
		return cleanupFailure
	}
	err := application.runFuzz([]string{
		"--package", ".", "--target", "FuzzFixture", "--duration", "1s",
	})
	if err == nil || !strings.Contains(err.Error(), fuzzCleanupCode) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("runFuzz error = %v, want cleanup diagnostic preserving cause", err)
	}
	if !strings.Contains(output.String(), "result: cleanup-failure") || !strings.Contains(output.String(), "evidence path: "+sandbox) {
		t.Fatalf("cleanup report = %q", output.String())
	}
	if _, err := os.Stat(sandbox); err != nil {
		t.Fatalf("cleanup failure evidence disappeared: %v", err)
	}
	if err := os.RemoveAll(sandbox); err != nil {
		t.Fatalf("remove test sandbox: %v", err)
	}
}

type fuzzFixtureFile struct {
	name    string
	content string
}

func newFuzzFixture(t *testing.T) string {
	return newFuzzFixtureWithFiles(t, nil)
}

func newFuzzFixtureWithFiles(t *testing.T, additional []fuzzFixtureFile) string {
	t.Helper()
	root := t.TempDir()
	writeFuzzFixtureFile(t, root, "go.mod", "module example.com/fuzzfixture\n\ngo 1.26.0\n")
	writeFuzzFixtureFile(t, root, "fuzz_test.go", `package fixture

import "testing"

func FuzzFixture(f *testing.F) {
	f.Add("seed")
	f.Fuzz(func(t *testing.T, value string) {
		_ = value
	})
}
`)
	writeFuzzFixtureFile(t, root, "sentinel.txt", "sentinel")
	for _, file := range additional {
		writeFuzzFixtureFile(t, root, file.name, file.content)
	}
	return root
}

func writeFuzzFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture file %s: %v", name, err)
	}
}

func fuzzTestApplication(t *testing.T, root string, stdout io.Writer) app {
	t.Helper()
	return app{
		ctx:    context.Background(),
		stdout: stdout,
		executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
			if name != "git" || !reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
				t.Fatalf("root command = %s %v", name, args)
			}
			return root, nil
		},
	}
}

func fuzzTestExitError(t *testing.T, status int) error {
	t.Helper()
	if status != 7 {
		t.Fatalf("unsupported fake exit status %d", status)
	}
	// #nosec G204 -- this test intentionally starts a fixed shell command to obtain an ExitError.
	err := exec.CommandContext(context.Background(), "sh", "-c", "exit 7").Run()
	if err == nil {
		t.Fatalf("fake exit command unexpectedly succeeded")
	}
	return err
}

func fuzzTestSignalError(t *testing.T) error {
	t.Helper()
	// #nosec G204 -- this test intentionally starts a fixed shell command to obtain a signal ExitError.
	err := exec.CommandContext(context.Background(), "sh", "-c", "kill -TERM $$").Run()
	if err == nil {
		t.Fatal("fake signal command unexpectedly succeeded")
	}
	return err
}

func readFuzzFixtureSentinel(t *testing.T, root string) string {
	t.Helper()
	// #nosec G304 -- root is a test-owned temporary fixture directory.
	content, err := os.ReadFile(filepath.Join(root, "sentinel.txt"))
	if err != nil {
		t.Fatalf("read fixture sentinel: %v", err)
	}
	return string(content)
}

func fuzzReportLine(report, prefix string) string {
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}
