package workflowctl

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSelectSignalFuzzTargetsUsesStableBoundaryOrder(t *testing.T) {
	got := selectSignalFuzzTargets(map[string]bool{"datatype.go": true, "syntax.go": true})
	if len(got) != 3 {
		t.Fatalf("selected targets = %#v, want three targets", got)
	}
	want := []string{"FuzzDecodeSyntax", "FuzzStrictIntegerCanonicalRoundTrip", "FuzzStrictDecimalCanonicalRoundTrip"}
	for index, target := range got {
		if target.Target != want[index] {
			t.Fatalf("target %d = %q, want %q", index, target.Target, want[index])
		}
	}
	if none := selectSignalFuzzTargets(map[string]bool{"internal/workflowctl/fuzz.go": true}); len(none) != 0 {
		t.Fatalf("internal workflow fuzz targets selected: %#v", none)
	}
}

func TestCheckCoveragePolicyRequiresMatchingConcreteExplanation(t *testing.T) {
	report := testSignalCoverageReport()
	if err := checkCoveragePolicy(report, nil); err == nil || !strings.Contains(err.Error(), "regressed without an explanation") {
		t.Fatalf("unexplained regression error = %v", err)
	}
	explanation := coverageExplanation{
		Schema: coverageExplanationSchema, Package: "example.com/mod/parser", Reason: "new branch is intentionally unsupported",
		Base: report.Packages[0].Base, Head: report.Packages[0].Head,
	}
	if err := checkCoveragePolicy(report, []coverageExplanation{explanation}); err != nil {
		t.Fatalf("matching explanation rejected: %v", err)
	}
	explanation.Head.Covered++
	if err := checkCoveragePolicy(report, []coverageExplanation{explanation}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("forged explanation accepted: %v", err)
	}
}

func TestCheckCoveragePolicyDoesNotUseRepositoryTotalAsGate(t *testing.T) {
	report := testSignalCoverageReport()
	report.Repository.Delta.Percent = -99
	if err := checkCoveragePolicy(report, []coverageExplanation{{
		Schema: coverageExplanationSchema, Package: "example.com/mod/parser", Reason: "known branch",
		Base: report.Packages[0].Base, Head: report.Packages[0].Head,
	}}); err != nil {
		t.Fatalf("repository-only regression rejected: %v", err)
	}
}

func TestCheckCoveragePolicyIgnoresAddedAndRemovedPackages(t *testing.T) {
	for _, test := range []struct {
		name string
		base bool
		head bool
	}{
		{name: "added", head: true},
		{name: "removed", base: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := coverageReport{Packages: []coveragePackageReport{{
				Package: "example.com/mod/packet", Affected: true,
				Base: coverageSideReport{Present: test.base}, Head: coverageSideReport{Present: test.head},
				Delta: coverageDeltaReport{Percent: -100},
			}}}
			if err := checkCoveragePolicy(report, nil); err != nil {
				t.Fatalf("%s package was treated as a percentage regression: %v", test.name, err)
			}
		})
	}
}

func TestDevelopSignalsCommandReportsSelectedSignalsSequentially(t *testing.T) {
	root := newFuzzFixture(t)
	var output bytes.Buffer
	var targets []string
	application := fuzzTestApplication(t, root, &output)
	application.buildCoverageReport = func(gotRoot, base string) (coverageReport, error) {
		if gotRoot != root || base != "base-sha" {
			t.Fatalf("coverage inputs = %q, %q", gotRoot, base)
		}
		report := testSignalCoverageReport()
		report.Base, report.Head = "base-sha", "head-sha"
		report.Packages[0].Delta.Percent = 10
		return report, nil
	}
	application.coverageChangedPaths = func(gotRoot, base, head string) (map[string]bool, error) {
		if gotRoot != root || base != "base-sha" || head != "head-sha" {
			t.Fatalf("changed-path inputs = %q, %q, %q", gotRoot, base, head)
		}
		return map[string]bool{"syntax.go": true, "datatype.go": true}, nil
	}
	application.executeCommandWithContextAndEnv = func(_ context.Context, _ string, _ []string,
		_ io.Reader, name string, args ...string,
	) (string, error) {
		if name != "go" {
			t.Fatalf("campaign command = %q, want go", name)
		}
		targets = append(targets, args[3])
		return "", nil
	}

	if err := application.run([]string{"develop-signals", "--base", "base-sha", "--duration", "250ms"}); err != nil {
		t.Fatalf("develop-signals: %v", err)
	}
	if want := []string{"-fuzz=^FuzzDecodeSyntax$", "-fuzz=^FuzzStrictIntegerCanonicalRoundTrip$", "-fuzz=^FuzzStrictDecimalCanonicalRoundTrip$"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("fuzz target arguments = %#v, want %#v", targets, want)
	}
	text := output.String()
	for _, phrase := range []string{"coverage: ", "fuzz health: selected", "FuzzDecodeSyntax", "FuzzStrictIntegerCanonicalRoundTrip", "FuzzStrictDecimalCanonicalRoundTrip", "workers=1 offline=true", "conformance: not measured"} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("develop-signals output omitted %q: %s", phrase, text)
		}
	}
}

func TestDevelopSignalsCommandReportsNoRelevantTarget(t *testing.T) {
	root := newFuzzFixture(t)
	var output bytes.Buffer
	application := fuzzTestApplication(t, root, &output)
	application.buildCoverageReport = func(string, string) (coverageReport, error) {
		report := testSignalCoverageReport()
		report.Packages[0].Delta.Percent = 10
		return report, nil
	}
	application.coverageChangedPaths = func(string, string, string) (map[string]bool, error) {
		return map[string]bool{"internal/workflowctl/signals.go": true}, nil
	}
	application.executeCommandWithContextAndEnv = func(context.Context, string, []string, io.Reader, string, ...string) (string, error) {
		t.Fatal("no-target signal unexpectedly ran fuzz")
		return "", nil
	}
	if err := application.run([]string{"develop-signals", "--base", "base-sha"}); err != nil {
		t.Fatalf("develop-signals: %v", err)
	}
	if !strings.Contains(output.String(), "fuzz health: no-relevant-target") {
		t.Fatalf("no-target output = %s", output.String())
	}
}

func TestDevelopSignalsCommandRejectsUnexplainedPolicyRegression(t *testing.T) {
	root := newFuzzFixture(t)
	application := fuzzTestApplication(t, root, io.Discard)
	application.buildCoverageReport = func(string, string) (coverageReport, error) { return testSignalCoverageReport(), nil }
	application.coverageChangedPaths = func(string, string, string) (map[string]bool, error) { return nil, nil }
	err := application.run([]string{"develop-signals", "--base", "base-sha"})
	if err == nil || !strings.Contains(err.Error(), "regressed without an explanation") {
		t.Fatalf("policy error = %v", err)
	}
}

func TestWriteDevelopmentSignalsSeparatesFuzzCoverageAndConformance(t *testing.T) {
	var output bytes.Buffer
	report := developmentSignalsReport{Selection: "no-relevant-target", Fuzz: []signalFuzzReport{}}
	if err := writeDevelopmentSignalsText(&output, report); err != nil {
		t.Fatalf("write signals: %v", err)
	}
	text := output.String()
	for _, phrase := range []string{"coverage:", "fuzz health: no-relevant-target", "conformance: not measured"} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("signals omitted %q: %s", phrase, text)
		}
	}
}

func testSignalCoverageReport() coverageReport {
	return coverageReport{Packages: []coveragePackageReport{{
		Package: "example.com/mod/parser", Affected: true,
		Base:  coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 8, Percent: 80},
		Head:  coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 7, Percent: 70},
		Delta: coverageDeltaReport{Covered: -1, Percent: -10},
	}}, Repository: coverageTotalsReport{Delta: coverageAggregateDelta{Percent: -1}}}
}
