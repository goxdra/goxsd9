package workflowctl

import (
	"bytes"
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
