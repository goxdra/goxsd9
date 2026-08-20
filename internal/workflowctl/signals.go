package workflowctl

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	defaultSignalFuzzDuration = 250 * time.Millisecond
	maxSignalFuzzDuration     = time.Second
	coverageExplanationSchema = "goxsd9/coverage-explanation/v1"
	developmentSignalsSchema  = "goxsd9/development-signals/v1"
)

type signalFuzzTarget struct {
	Boundary string `json:"boundary"`
	Package  string `json:"package"`
	Target   string `json:"target"`
}

// This is deliberately an ordered slice: it is the policy's source of truth
// and keeps campaign selection independent of filesystem or map ordering.
var signalFuzzTargets = []signalFuzzTarget{
	{Boundary: "syntax.go", Package: ".", Target: "FuzzDecodeSyntax"},
	{Boundary: "datatype.go", Package: ".", Target: "FuzzStrictIntegerCanonicalRoundTrip"},
	{Boundary: "datatype.go", Package: ".", Target: "FuzzStrictDecimalCanonicalRoundTrip"},
}

type signalFuzzReport struct {
	Boundary string `json:"boundary"`
	Package  string `json:"package"`
	Target   string `json:"target"`
	Duration string `json:"duration"`
	Workers  int    `json:"workers"`
	Offline  bool   `json:"offline"`
	Result   string `json:"result"`
}

type developmentSignalsReport struct {
	Schema                string             `json:"schema"`
	Base                  string             `json:"base"`
	Head                  string             `json:"head"`
	Coverage              coverageReport     `json:"coverage"`
	Fuzz                  []signalFuzzReport `json:"fuzz"`
	Selection             string             `json:"selection"`
	Catalog               string             `json:"catalog"`
	XSDFeatureSupport     string             `json:"xsdFeatureSupport"`
	ExecutableConformance string             `json:"executableConformance"`
}

type coverageExplanation struct {
	Schema  string             `json:"schema"`
	Package string             `json:"package"`
	Reason  string             `json:"reason"`
	Base    coverageSideReport `json:"base"`
	Head    coverageSideReport `json:"head"`
}

func (a app) runDevelopmentSignals(args []string) error { //nolint:gocognit // orchestration owns the ordered signal gate.
	flags := flag.NewFlagSet("develop-signals", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("base", "", "Git base reference")
	duration := flags.Duration("duration", defaultSignalFuzzDuration, "bounded active fuzz duration per target")
	explanationPath := flags.String("coverage-explanation-file", "", "JSON coverage regression explanations")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return usageError("develop-signals: %v", err)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*base) == "" || *duration <= 0 || *duration > maxSignalFuzzDuration || (*format != "text" && *format != "json") {
		return usageError("usage: workflowctl develop-signals --base REF [--duration DURATION] [--coverage-explanation-file FILE] [--format text|json]")
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	buildCoverageReport := a.buildCoverageReport
	if buildCoverageReport == nil {
		buildCoverageReport = a.buildCoverageReportFromRepository
	}
	coverage, err := buildCoverageReport(root, *base)
	if err != nil {
		return fmt.Errorf("build development coverage signal: %w", err)
	}
	explanations, err := readCoverageExplanations(*explanationPath)
	if err != nil {
		return err
	}
	if policyErr := checkCoveragePolicy(coverage, explanations); policyErr != nil {
		return stateError("development coverage gate: %v", policyErr)
	}
	coverageChangedPaths := a.coverageChangedPaths
	if coverageChangedPaths == nil {
		coverageChangedPaths = a.coverageChangedPathsFromRepository
	}
	changed, err := coverageChangedPaths(root, coverage.Base, coverage.Head)
	if err != nil {
		return fmt.Errorf("select development fuzz targets: %w", err)
	}
	targets := selectSignalFuzzTargets(changed)
	report := developmentSignalsReport{
		Schema: developmentSignalsSchema, Base: coverage.Base, Head: coverage.Head, Coverage: coverage,
		Selection: "no-relevant-target", Catalog: noMeasuredDevelopmentSignal,
		XSDFeatureSupport: noMeasuredDevelopmentSignal, ExecutableConformance: noMeasuredDevelopmentSignal,
		Fuzz: make([]signalFuzzReport, 0, len(targets)),
	}
	if len(targets) != 0 {
		report.Selection = "selected"
	}
	for _, target := range targets {
		run, err := newFuzzRun(target.Package, target.Target, duration.String())
		if err != nil {
			return fmt.Errorf("prepare fuzz target %s: %w", target.Target, err)
		}
		campaign := a
		campaign.stdout = io.Discard
		if err := campaign.executeFuzzRun(root, run); err != nil {
			return fmt.Errorf("run fuzz target %s: %w", target.Target, err)
		}
		report.Fuzz = append(report.Fuzz, signalFuzzReport{
			Boundary: target.Boundary, Package: target.Package, Target: target.Target,
			Duration: run.duration.String(), Workers: run.workers, Offline: run.offline, Result: "success",
		})
	}
	if *format == "json" {
		return writeDevelopmentSignalsJSON(a.stdout, report)
	}
	return writeDevelopmentSignalsText(a.stdout, report)
}

func selectSignalFuzzTargets(changed map[string]bool) []signalFuzzTarget {
	selected := make([]signalFuzzTarget, 0, len(signalFuzzTargets))
	for _, target := range signalFuzzTargets {
		if changed[target.Boundary] {
			selected = append(selected, target)
		}
	}
	return selected
}

func readCoverageExplanations(filePath string) ([]coverageExplanation, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filePath) // #nosec G304 -- explicitly supplied evidence file.
	if err != nil {
		return nil, fmt.Errorf("read coverage explanation file: %w", err)
	}
	var explanations []coverageExplanation
	if err := json.Unmarshal(data, &explanations); err != nil {
		return nil, fmt.Errorf("decode coverage explanation file: %w", err)
	}
	return explanations, nil
}

func checkCoveragePolicy(report coverageReport, explanations []coverageExplanation) error {
	byPackage, err := indexCoverageExplanations(explanations)
	if err != nil {
		return err
	}
	if err := checkExplainedCoverageRegressions(report, byPackage); err != nil {
		return err
	}
	return rejectUnusedCoverageExplanations(report, explanations)
}

func indexCoverageExplanations(explanations []coverageExplanation) (map[string]coverageExplanation, error) {
	byPackage := make(map[string]coverageExplanation, len(explanations))
	for _, explanation := range explanations {
		if explanation.Schema != coverageExplanationSchema || explanation.Package == "" || strings.TrimSpace(explanation.Reason) == "" {
			return nil, errors.New("each explanation requires the versioned schema, package, and concrete reason")
		}
		if _, exists := byPackage[explanation.Package]; exists {
			return nil, fmt.Errorf("duplicate explanation for package %q", explanation.Package)
		}
		byPackage[explanation.Package] = explanation
	}
	return byPackage, nil
}

func checkExplainedCoverageRegressions(report coverageReport, explanations map[string]coverageExplanation) error {
	for _, packageReport := range report.Packages {
		if !packageReport.Affected || !coveragePackageRegressed(packageReport) {
			continue
		}
		explanation, exists := explanations[packageReport.Package]
		if !exists {
			return fmt.Errorf("affected package %q regressed without an explanation", packageReport.Package)
		}
		if explanation.Base != packageReport.Base || explanation.Head != packageReport.Head {
			return fmt.Errorf("explanation for package %q does not match computed base/head evidence", packageReport.Package)
		}
	}
	return nil
}

func rejectUnusedCoverageExplanations(report coverageReport, explanations []coverageExplanation) error {
	for _, explanation := range explanations {
		packageName := explanation.Package
		found := false
		for _, packageReport := range report.Packages {
			if packageReport.Package == packageName && packageReport.Affected && coveragePackageRegressed(packageReport) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("explanation for package %q does not identify an affected regression", packageName)
		}
	}
	return nil
}

func coveragePackageRegressed(packageReport coveragePackageReport) bool {
	return packageReport.Base.Present && packageReport.Head.Present && packageReport.Delta.Percent < 0
}

func writeDevelopmentSignalsJSON(output io.Writer, report developmentSignalsReport) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode development signals: %w", err)
	}
	return nil
}

func writeDevelopmentSignalsText(output io.Writer, report developmentSignalsReport) error {
	if err := writeLine(output, "Development signals"); err != nil {
		return err
	}
	if err := writeLine(output, "coverage: affected-package and repository deltas reported; policy: pass"); err != nil {
		return err
	}
	if err := writeLine(output, "fuzz health: %s", report.Selection); err != nil {
		return err
	}
	for _, fuzz := range report.Fuzz {
		if err := writeLine(output, "fuzz: %s %s %s workers=%d offline=%t", fuzz.Boundary, fuzz.Target, fuzz.Result, fuzz.Workers, fuzz.Offline); err != nil {
			return err
		}
	}
	if err := writeLine(output, "catalog: %s", report.Catalog); err != nil {
		return err
	}
	if err := writeLine(output, "XSD feature support: %s", report.XSDFeatureSupport); err != nil {
		return err
	}
	if err := writeLine(output, "executable conformance: %s", report.ExecutableConformance); err != nil {
		return err
	}
	return writeLine(output, "conformance: not measured by these signals")
}
