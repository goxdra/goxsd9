package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadCoverageProfileDeduplicatesCoverpkgBlocks(t *testing.T) {
	profile := strings.Join([]string{
		"mode: set",
		"example.com/mod/main.go:1.1,1.2 2 0",
		"example.com/mod/main.go:1.1,1.2 2 1",
		"example.com/mod/main.go:1.1,1.2 2 0",
		"example.com/mod/sub/x.go:2.1,2.2 3 1",
		"example.com/mod/sub/x.go:2.1,2.2 3 1",
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(profile), 0o600); err != nil {
		t.Fatalf("write coverage profile: %v", err)
	}

	got, err := readCoverageProfile(path, []string{"example.com/mod", "example.com/mod/sub"})
	if err != nil {
		t.Fatalf("readCoverageProfile: %v", err)
	}
	want := map[string]coverageCounts{
		"example.com/mod":     {statements: 2, covered: 2},
		"example.com/mod/sub": {statements: 3, covered: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("coverage counts = %#v, want %#v", got, want)
	}
}

func TestAssembleCoverageReportSeparatesAffectedAndRepositoryTotals(t *testing.T) {
	base := coverageSnapshot{
		packages: map[string]coverageGoPackage{
			"example.com/mod/a":       {path: "example.com/mod/a", relativeDir: "a", hasTests: true},
			"example.com/mod/removed": {path: "example.com/mod/removed", relativeDir: "removed", hasTests: true},
			"example.com/mod/same":    {path: "example.com/mod/same", relativeDir: "same"},
		},
		paths: []string{"example.com/mod/a", "example.com/mod/removed", "example.com/mod/same"},
		counts: map[string]coverageCounts{
			"example.com/mod/a":       {statements: 10, covered: 5},
			"example.com/mod/removed": {statements: 4, covered: 2},
			"example.com/mod/same":    {statements: 3},
		},
	}
	head := coverageSnapshot{
		packages: map[string]coverageGoPackage{
			"example.com/mod/a":     {path: "example.com/mod/a", relativeDir: "a", hasTests: true},
			"example.com/mod/added": {path: "example.com/mod/added", relativeDir: "added"},
			"example.com/mod/same":  {path: "example.com/mod/same", relativeDir: "same"},
		},
		paths: []string{"example.com/mod/a", "example.com/mod/added", "example.com/mod/same"},
		counts: map[string]coverageCounts{
			"example.com/mod/a":     {statements: 12, covered: 9},
			"example.com/mod/added": {statements: 5, covered: 0},
			"example.com/mod/same":  {statements: 3},
		},
	}
	report := assembleCoverageReport("base-sha", "head-sha", map[string]bool{"a/a.go": true}, base, head)

	paths := make([]string, 0, len(report.Packages))
	statuses := make(map[string]string)
	for _, packageReport := range report.Packages {
		paths = append(paths, packageReport.Package)
		statuses[packageReport.Package] = packageReport.Status
	}
	if want := []string{"example.com/mod/a", "example.com/mod/added", "example.com/mod/removed", "example.com/mod/same"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("package order = %#v, want %#v", paths, want)
	}
	wantStatuses := map[string]string{
		"example.com/mod/a":       "changed",
		"example.com/mod/added":   "added",
		"example.com/mod/removed": "removed",
		"example.com/mod/same":    "unchanged",
	}
	if !reflect.DeepEqual(statuses, wantStatuses) {
		t.Fatalf("statuses = %#v, want %#v", statuses, wantStatuses)
	}
	if report.Affected.Base.Packages != 2 || report.Affected.Head.Packages != 2 {
		t.Fatalf("affected package totals = %#v", report.Affected)
	}
	if report.Repository.Base.Packages != 3 || report.Repository.Head.Packages != 3 {
		t.Fatalf("repository package totals = %#v", report.Repository)
	}
	if report.Affected.Delta.Statements != 3 || report.Affected.Delta.Covered != 2 {
		t.Fatalf("affected delta = %#v", report.Affected.Delta)
	}
	for _, packageReport := range report.Packages {
		if packageReport.Package == "example.com/mod/same" && packageReport.Affected {
			t.Fatal("unchanged package was marked affected")
		}
	}
}

func TestCoverageRenderingIsByteStable(t *testing.T) {
	report := coverageReport{
		Base: "base-sha", Head: "head-sha",
		Packages: []coveragePackageReport{{
			Package: "example.com/mod", Status: "changed", Affected: true,
			Base:  coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 5, Percent: 50},
			Head:  coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 6, Percent: 60},
			Delta: coverageDeltaReport{Statements: 0, Covered: 1, Percent: 10},
		}},
		Affected: coverageTotalsReport{
			Base:  coverageAggregate{Packages: 1, TestedPackages: 1, Statements: 10, Covered: 5, Percent: 50},
			Head:  coverageAggregate{Packages: 1, TestedPackages: 1, Statements: 10, Covered: 6, Percent: 60},
			Delta: coverageAggregateDelta{Covered: 1, Percent: 10},
		},
	}

	var firstText, secondText bytes.Buffer
	if err := (app{stdout: &firstText}).writeCoverageText(report); err != nil {
		t.Fatalf("first text report: %v", err)
	}
	if err := (app{stdout: &secondText}).writeCoverageText(report); err != nil {
		t.Fatalf("second text report: %v", err)
	}
	if firstText.String() != secondText.String() {
		t.Fatalf("text reports differ:\n%s\n---\n%s", firstText.String(), secondText.String())
	}

	var firstJSON, secondJSON bytes.Buffer
	if err := (app{stdout: &firstJSON}).writeCoverageJSON(report); err != nil {
		t.Fatalf("first JSON report: %v", err)
	}
	if err := (app{stdout: &secondJSON}).writeCoverageJSON(report); err != nil {
		t.Fatalf("second JSON report: %v", err)
	}
	if firstJSON.String() != secondJSON.String() {
		t.Fatalf("JSON reports differ:\n%s\n---\n%s", firstJSON.String(), secondJSON.String())
	}
}

func TestRunCoverageRejectsDirtyWorktree(t *testing.T) {
	var calls []string
	application := app{
		ctx: context.Background(),
		executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			switch name + " " + strings.Join(args, " ") {
			case "git rev-parse --show-toplevel":
				return "/repo", nil
			case "git status --porcelain":
				return " M internal/workflowctl/coverage.go", nil
			default:
				return "", fmt.Errorf("unexpected command: %s", calls[len(calls)-1])
			}
		},
	}
	if err := application.runCoverage([]string{"--base", "base"}); err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("dirty coverage error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("commands after dirty check = %#v", calls)
	}
}

func TestRunCoverageRejectsInvalidBaseBeforeTests(t *testing.T) {
	var calls []string
	application := app{
		ctx: context.Background(),
		executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
			command := name + " " + strings.Join(args, " ")
			calls = append(calls, command)
			switch command {
			case "git rev-parse --show-toplevel":
				return "/repo", nil
			case "git status --porcelain":
				return "", nil
			case "git rev-parse --verify --end-of-options missing^{commit}":
				return "", errors.New("unknown revision")
			default:
				return "", fmt.Errorf("unexpected command: %s", command)
			}
		},
	}
	err := application.runCoverage([]string{"--base", "missing"})
	if err == nil || !strings.Contains(err.Error(), `resolve coverage base "missing"`) {
		t.Fatalf("invalid base error = %v", err)
	}
	for _, call := range calls {
		if strings.Contains(call, "go ") || strings.Contains(call, "worktree") {
			t.Fatalf("invalid base ran execution command %q", call)
		}
	}
}

func TestRunCoverageReturnsTestAndCleanupFailures(t *testing.T) {
	var calls []string
	var stdout bytes.Buffer
	application := app{
		ctx:    context.Background(),
		stdout: &stdout,
		executeCommand: func(dir string, _ io.Reader, name string, args ...string) (string, error) {
			command := name + " " + strings.Join(args, " ")
			calls = append(calls, command)
			switch {
			case command == "git rev-parse --show-toplevel":
				return "/repo", nil
			case command == "git status --porcelain":
				return "", nil
			case command == "git rev-parse --verify --end-of-options base^{commit}":
				return "base-sha", nil
			case command == "git rev-parse --verify --end-of-options HEAD^{commit}":
				return "head-sha", nil
			case strings.HasPrefix(command, "git diff --name-only"):
				return "", nil
			case strings.HasPrefix(command, "git worktree add "):
				return "", nil
			case strings.HasPrefix(command, "git worktree remove "):
				return "", errors.New("cleanup failed")
			default:
				return "", fmt.Errorf("unexpected git command in %s: %s", dir, command)
			}
		},
		executeCommandWithEnv: func(dir string, env []string, _ io.Reader, name string, args ...string) (string, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			if name != "go" || len(args) == 0 {
				return "", fmt.Errorf("unexpected environment command %s", name)
			}
			if args[0] == "list" {
				return fmt.Sprintf("{\"Dir\":%q,\"ImportPath\":\"example.com/mod\"}\n", dir), nil
			}
			if args[0] == "test" {
				return "", errors.New("test failed")
			}
			return "", fmt.Errorf("unexpected Go command: %s", strings.Join(args, " "))
		},
	}
	err := application.runCoverage([]string{"--base", "base"})
	if err == nil || !strings.Contains(err.Error(), "test failed") || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("test and cleanup error = %v", err)
	}
	if strings.Contains(stdout.String(), "Coverage delta") {
		t.Fatal("failed coverage emitted a partial report")
	}
	if !containsCoverageCall(calls, "git worktree remove") {
		t.Fatalf("cleanup was not attempted: %#v", calls)
	}
}

func containsCoverageCall(calls []string, prefix string) bool {
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			return true
		}
	}
	return false
}
