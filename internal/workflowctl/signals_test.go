package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSelectSignalFuzzTargetsUsesStableBoundaryOrder(t *testing.T) {
	got := selectSignalFuzzTargets(map[string]bool{"datatype.go": true, "syntax.go": true})
	if len(got) != 4 {
		t.Fatalf("selected targets = %#v, want four targets", got)
	}
	want := []string{"FuzzDecodeSyntax", "FuzzStrictIntegerCanonicalRoundTrip", "FuzzStrictDecimalCanonicalRoundTrip", "FuzzStrictBooleanCanonicalRoundTrip"}
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

func TestCoverageExplanationsAreCanonicalAndRejectStaleFacts(t *testing.T) {
	report := testSignalCoverageReport()
	report.Packages[0].Status = "changed"
	report.Base, report.Head = "base", "head"
	first := coverageExplanation{
		Schema: coverageExplanationSchema, Package: "z.example/parser", Reason: "known branch",
		Base: coverageSideReport{Present: true, HasTests: true, Statements: 2, Covered: 2, Percent: 100},
		Head: coverageSideReport{Present: true, HasTests: true, Statements: 2, Covered: 1, Percent: 50},
	}
	second := first
	second.Package = report.Packages[0].Package
	second.Base, second.Head = report.Packages[0].Base, report.Packages[0].Head
	canonical, err := canonicalCoverageExplanations([]coverageExplanation{first, second})
	if err != nil {
		t.Fatalf("canonicalize explanations: %v", err)
	}
	if canonical[0].Package != report.Packages[0].Package || canonical[1].Package != first.Package {
		t.Fatalf("explanation order = %#v", canonical)
	}
	if err := checkCoveragePolicy(report, []coverageExplanation{second}); err != nil {
		t.Fatalf("matching sorted explanation rejected: %v", err)
	}
	stale := second
	stale.Head.Covered++
	if err := checkCoveragePolicy(report, []coverageExplanation{stale}); err == nil {
		t.Fatal("stale explanation facts were accepted")
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
	if want := []string{"-fuzz=^FuzzDecodeSyntax$", "-fuzz=^FuzzStrictIntegerCanonicalRoundTrip$", "-fuzz=^FuzzStrictDecimalCanonicalRoundTrip$", "-fuzz=^FuzzStrictBooleanCanonicalRoundTrip$"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("fuzz target arguments = %#v, want %#v", targets, want)
	}
	text := output.String()
	for _, phrase := range []string{"coverage: ", "fuzz health: selected", "FuzzDecodeSyntax", "FuzzStrictIntegerCanonicalRoundTrip", "FuzzStrictDecimalCanonicalRoundTrip", "FuzzStrictBooleanCanonicalRoundTrip", "workers=1 offline=true", "conformance: not measured"} {
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
		report.Base, report.Head = "base-sha", "head-sha"
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
	application.buildCoverageReport = func(string, string) (coverageReport, error) {
		report := testSignalCoverageReport()
		report.Base, report.Head = "base-sha", "head-sha"
		return report, nil
	}
	application.coverageChangedPaths = func(string, string, string) (map[string]bool, error) { return nil, nil }
	err := application.run([]string{"develop-signals", "--base", "base-sha"})
	if err == nil || !strings.Contains(err.Error(), "regressed without an explanation") {
		t.Fatalf("policy error = %v", err)
	}
}

func TestWriteDevelopmentSignalsSeparatesFuzzCoverageAndConformance(t *testing.T) {
	var output bytes.Buffer
	report := developmentSignalsReport{Selection: "no-relevant-target", CoverageExplanations: []coverageExplanation{}, Fuzz: []signalFuzzReport{}, AdditionalFuzz: []additionalFuzzReport{}}
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

func TestDevelopmentSignalsJSONMakesNoTargetAndUnmeasuredClaimsExplicit(t *testing.T) {
	packages := []coveragePackageReport{}
	report := developmentSignalsReport{
		Schema: developmentSignalsSchema, Base: "base", Head: "head",
		Coverage: coverageReport{
			Base: "base", Head: "head", Packages: packages,
			Affected: coverageTotals(packages, true), Repository: coverageTotals(packages, false),
		},
		CoverageExplanations: []coverageExplanation{}, Fuzz: []signalFuzzReport{}, AdditionalFuzz: []additionalFuzzReport{}, Selection: "no-relevant-target",
		Catalog: noMeasuredDevelopmentSignal, XSDFeatureSupport: noMeasuredDevelopmentSignal,
		ExecutableConformance: noMeasuredDevelopmentSignal,
	}
	var output bytes.Buffer
	if err := writeDevelopmentSignalsJSON(&output, report); err != nil {
		t.Fatalf("write development signals JSON: %v", err)
	}
	var got developmentSignalsReport
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode development signals JSON: %v", err)
	}
	if err := validateDevelopmentSignals(got, "base", "head"); err != nil {
		t.Fatalf("validate development signals: %v", err)
	}
}

func TestDevelopmentSignalsV2RoundTripIsByteStableAndArraysAreNonNil(t *testing.T) {
	packages := []coveragePackageReport{}
	report := developmentSignalsReport{
		Schema: developmentSignalsSchema, Base: "base", Head: "head",
		Coverage: coverageReport{
			Base: "base", Head: "head", Packages: packages,
			Affected: coverageTotals(packages, true), Repository: coverageTotals(packages, false),
		},
		CoverageExplanations: []coverageExplanation{}, Fuzz: []signalFuzzReport{}, AdditionalFuzz: []additionalFuzzReport{},
		Selection: "no-relevant-target", Catalog: noMeasuredDevelopmentSignal,
		XSDFeatureSupport: noMeasuredDevelopmentSignal, ExecutableConformance: noMeasuredDevelopmentSignal,
	}
	var first bytes.Buffer
	if err := writeDevelopmentSignalsJSON(&first, report); err != nil {
		t.Fatalf("write v2 signals: %v", err)
	}
	var decoded developmentSignalsReport
	if err := json.Unmarshal(first.Bytes(), &decoded); err != nil {
		t.Fatalf("decode v2 signals: %v", err)
	}
	if decoded.CoverageExplanations == nil || decoded.Fuzz == nil || decoded.AdditionalFuzz == nil {
		t.Fatal("v2 signal arrays became nil after round trip")
	}
	if err := validateDevelopmentSignals(decoded, "base", "head"); err != nil {
		t.Fatalf("validate v2 signals: %v", err)
	}
	var second bytes.Buffer
	if err := writeDevelopmentSignalsJSON(&second, decoded); err != nil {
		t.Fatalf("rewrite v2 signals: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("v2 signals changed bytes after round trip:\n%s\n---\n%s", first.Bytes(), second.Bytes())
	}
}

func TestDevelopSignalsAdditionalFuzzIsSortedAndCanBeTheOnlyCampaign(t *testing.T) {
	root := newFuzzFixtureWithFiles(t, []fuzzFixtureFile{
		{name: "additional_test.go", content: `package fixture

import "testing"

func FuzzAdditionalBeta(f *testing.F) { f.Fuzz(func(*testing.T, string) {}) }
func FuzzAdditionalAlpha(f *testing.F) { f.Fuzz(func(*testing.T, string) {}) }
`},
	})
	var output bytes.Buffer
	var targets []string
	application := fuzzTestApplication(t, root, &output)
	application.buildCoverageReport = func(string, string) (coverageReport, error) {
		report := testSignalCoverageReport()
		report.Base, report.Head = "base-sha", "head-sha"
		report.Packages[0].Status = "changed"
		report.Packages[0].Head = coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 9, Percent: 90}
		report.Packages[0].Delta = coverageDelta(report.Packages[0].Base, report.Packages[0].Head)
		report.Affected = coverageTotals(report.Packages, true)
		report.Repository = coverageTotals(report.Packages, false)
		return report, nil
	}
	application.coverageChangedPaths = func(string, string, string) (map[string]bool, error) {
		return map[string]bool{"internal/workflowctl/signals.go": true}, nil
	}
	application.executeCommandWithContextAndEnv = func(_ context.Context, _ string, _ []string, _ io.Reader,
		name string, args ...string,
	) (string, error) {
		if name != "go" {
			t.Fatalf("campaign command = %q, want go", name)
		}
		targets = append(targets, args[3])
		return "", nil
	}
	if err := application.run([]string{
		"develop-signals", "--base", "base-sha", "--format", "json",
		"--additional-fuzz", "example.com/fuzzfixture:FuzzAdditionalBeta", "--additional-fuzz", ".:FuzzAdditionalAlpha",
	}); err != nil {
		t.Fatalf("develop-signals with additional fuzz: %v", err)
	}
	if want := []string{"-fuzz=^FuzzAdditionalAlpha$", "-fuzz=^FuzzAdditionalBeta$"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("additional fuzz order = %#v, want %#v", targets, want)
	}
	var report developmentSignalsReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode additional signal report: %v", err)
	}
	if report.Selection != "no-relevant-target" || len(report.Fuzz) != 0 || len(report.AdditionalFuzz) != 2 {
		t.Fatalf("additional-only signal selection = %#v", report)
	}
	if report.AdditionalFuzz[0].Target != "FuzzAdditionalAlpha" || report.AdditionalFuzz[1].Target != "FuzzAdditionalBeta" {
		t.Fatalf("additional fuzz report order = %#v", report.AdditionalFuzz)
	}
	if report.AdditionalFuzz[0].Package != "." || report.AdditionalFuzz[1].Package != "." {
		t.Fatalf("additional fuzz report packages = %#v, want canonical root packages", report.AdditionalFuzz)
	}
	if err := validateDevelopmentSignals(report, "base-sha", "head-sha"); err != nil {
		t.Fatalf("validate additional signal report: %v", err)
	}
}

func TestParseAdditionalFuzzTargetsRejectsUnsafeDuplicatesAndExcess(t *testing.T) {
	for _, value := range []string{"FuzzOnly", ".:", ".:FuzzOne:extra", ".:notFuzz"} {
		if _, err := parseAdditionalFuzzTargets([]string{value}); err == nil {
			t.Fatalf("invalid additional fuzz %q was accepted", value)
		}
	}
	if _, err := parseAdditionalFuzzTargets([]string{".:FuzzOne", ".:FuzzOne"}); err == nil {
		t.Fatal("duplicate additional fuzz target was accepted")
	}
	if _, err := parseAdditionalFuzzTargets([]string{"./:FuzzOne", ".:FuzzOne"}); err == nil {
		t.Fatal("non-canonical duplicate additional fuzz target was accepted")
	}
	values := make([]string, maxAdditionalFuzzTargets+1)
	for index := range values {
		values[index] = fmt.Sprintf(".:FuzzTarget%d", index)
	}
	if _, err := parseAdditionalFuzzTargets(values); err == nil {
		t.Fatal("additional fuzz count limit was not enforced")
	}
}

func TestCanonicalizeAdditionalFuzzTargetsNormalizesRootAliases(t *testing.T) {
	root := newFuzzFixture(t)
	canonical, err := canonicalizeAdditionalFuzzTargets(root, []additionalFuzzTarget{{
		Package: "example.com/fuzzfixture", Target: "FuzzFixture",
	}})
	if err != nil {
		t.Fatalf("canonicalize module-root package: %v", err)
	}
	if len(canonical) != 1 || canonical[0].Package != "." {
		t.Fatalf("canonical module-root package = %#v, want .", canonical)
	}
	if _, err := canonicalizeAdditionalFuzzTargets(root, []additionalFuzzTarget{
		{Package: ".", Target: "FuzzFixture"},
		{Package: "example.com/fuzzfixture", Target: "FuzzFixture"},
	}); err == nil {
		t.Fatal("module-root alias duplicate was accepted")
	}
}

func TestValidateAdditionalFuzzRejectsDescendingTargetsWithinPackage(t *testing.T) {
	valid := func(target string) additionalFuzzReport {
		return additionalFuzzReport{Package: ".", Target: target, Duration: "250ms", Workers: 1, Offline: true, Result: "success"}
	}
	if err := validateAdditionalFuzz(nil, []additionalFuzzReport{valid("FuzzBeta"), valid("FuzzAlpha")}); err == nil {
		t.Fatal("additional fuzz targets in descending same-package order were accepted")
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
