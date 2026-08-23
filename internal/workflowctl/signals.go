package workflowctl

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultSignalFuzzDuration = 250 * time.Millisecond
	maxSignalFuzzDuration     = time.Second
	maxAdditionalFuzzTargets  = 8
	coverageExplanationSchema = "goxsd9/coverage-explanation/v1"
	developmentSignalsSchema  = "goxsd9/development-signals/v2"
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
	{Boundary: "datatype.go", Package: ".", Target: "FuzzStrictBooleanCanonicalRoundTrip"},
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

type additionalFuzzTarget struct {
	Package string
	Target  string
}

type additionalFuzzFlagValue struct {
	values []string
}

func (value *additionalFuzzFlagValue) Set(input string) error {
	value.values = append(value.values, input)
	return nil
}

func (value *additionalFuzzFlagValue) String() string {
	return strings.Join(value.values, ",")
}

type additionalFuzzReport struct {
	Package  string `json:"package"`
	Target   string `json:"target"`
	Duration string `json:"duration"`
	Workers  int    `json:"workers"`
	Offline  bool   `json:"offline"`
	Result   string `json:"result"`
}

type developmentSignalsReport struct {
	Schema                string                 `json:"schema"`
	Base                  string                 `json:"base"`
	Head                  string                 `json:"head"`
	Coverage              coverageReport         `json:"coverage"`
	CoverageExplanations  []coverageExplanation  `json:"coverageExplanations"`
	Fuzz                  []signalFuzzReport     `json:"fuzz"`
	AdditionalFuzz        []additionalFuzzReport `json:"additionalFuzz"`
	Selection             string                 `json:"selection"`
	Catalog               string                 `json:"catalog"`
	XSDFeatureSupport     string                 `json:"xsdFeatureSupport"`
	ExecutableConformance string                 `json:"executableConformance"`
}

type coverageExplanation struct {
	Schema  string             `json:"schema"`
	Package string             `json:"package"`
	Reason  string             `json:"reason"`
	Base    coverageSideReport `json:"base"`
	Head    coverageSideReport `json:"head"`
}

func (a app) runDevelopmentSignals(args []string) error { //nolint:gocognit // command parsing and ordered signal orchestration share one gate.
	flags := flag.NewFlagSet("develop-signals", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("base", "", "Git base reference")
	duration := flags.Duration("duration", defaultSignalFuzzDuration, "bounded active fuzz duration per target")
	explanationPath := flags.String("coverage-explanation-file", "", "JSON coverage regression explanations")
	format := flags.String("format", "text", "output format: text or json")
	var additionalFlags additionalFuzzFlagValue
	flags.Var(&additionalFlags, "additional-fuzz", "additional fuzz target PACKAGE:TARGET (repeatable)")
	if err := flags.Parse(args); err != nil {
		return usageError("develop-signals: %v", err)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*base) == "" || *duration <= 0 || *duration > maxSignalFuzzDuration || (*format != "text" && *format != "json") {
		return usageError("usage: workflowctl develop-signals --base REF [--duration DURATION] [--coverage-explanation-file FILE] [--additional-fuzz PACKAGE:TARGET ...] [--format text|json]")
	}
	additional, err := parseAdditionalFuzzTargets(additionalFlags.values)
	if err != nil {
		return usageError("develop-signals: %v", err)
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	additional, err = canonicalizeAdditionalFuzzTargets(root, additional)
	if err != nil {
		return usageError("develop-signals: %v", err)
	}
	explanations, err := readCoverageExplanations(*explanationPath)
	if err != nil {
		return err
	}
	for _, target := range additional {
		run, runErr := newFuzzRun(target.Package, target.Target, duration.String())
		if runErr != nil {
			return usageError("develop-signals: additional fuzz %s:%s: %v", target.Package, target.Target, runErr)
		}
		if _, runErr = validateFuzzRun(root, run); runErr != nil {
			return usageError("develop-signals: additional fuzz %s:%s: %v", target.Package, target.Target, runErr)
		}
	}
	build := a.buildDevelopmentSignalsReport
	if build == nil {
		build = a.buildDevelopmentSignalsReportFromRepository
	}
	report, err := build(root, *base, "", *duration, explanations, additional)
	if err != nil {
		return fmt.Errorf("build development signals: %w", err)
	}
	if *format == "json" {
		return writeDevelopmentSignalsJSON(a.stdout, report)
	}
	return writeDevelopmentSignalsText(a.stdout, report)
}

func (a app) buildDevelopmentSignalsReportFromRepository(root, base, expectedHead string, //nolint:gocognit // deterministic signal phases stay in one builder.
	duration time.Duration, explanations []coverageExplanation, additional []additionalFuzzTarget,
) (developmentSignalsReport, error) {
	if duration <= 0 || duration > maxSignalFuzzDuration {
		return developmentSignalsReport{}, fmt.Errorf("fuzz duration %s is outside the bounded range", duration)
	}
	canonicalAdditional, err := canonicalizeAdditionalFuzzTargets(root, additional)
	if err != nil {
		return developmentSignalsReport{}, err
	}
	additional = canonicalAdditional
	coverageBuilder := a.buildCoverageReport
	if coverageBuilder == nil {
		coverageBuilder = a.buildCoverageReportFromRepository
	}
	resolvedBase := base
	if a.buildCoverageReport == nil {
		resolved, resolveErr := a.resolveCommit(root, base)
		if resolveErr != nil {
			return developmentSignalsReport{}, fmt.Errorf("resolve development coverage base %q: %w", base, resolveErr)
		}
		resolvedBase = resolved
	}
	coverage, err := coverageBuilder(root, base)
	if err != nil {
		return developmentSignalsReport{}, fmt.Errorf("build development coverage signal: %w", err)
	}
	if strings.TrimSpace(coverage.Base) == "" || strings.TrimSpace(coverage.Head) == "" {
		return developmentSignalsReport{}, errors.New("development coverage signal has empty base or head")
	}
	if coverage.Base != resolvedBase {
		return developmentSignalsReport{}, fmt.Errorf("development coverage base %q does not match requested base %q", coverage.Base, resolvedBase)
	}
	if expectedHead != "" && coverage.Head != expectedHead {
		return developmentSignalsReport{}, fmt.Errorf("development coverage head %q does not match expected head %q", coverage.Head, expectedHead)
	}
	canonicalExplanations, err := canonicalCoverageExplanations(explanations)
	if err != nil {
		return developmentSignalsReport{}, err
	}
	if policyErr := checkCoveragePolicy(coverage, canonicalExplanations); policyErr != nil {
		return developmentSignalsReport{}, policyErr
	}
	changedPaths := a.coverageChangedPaths
	if changedPaths == nil {
		changedPaths = a.coverageChangedPathsFromRepository
	}
	changed, err := changedPaths(root, coverage.Base, coverage.Head)
	if err != nil {
		return developmentSignalsReport{}, fmt.Errorf("select development fuzz targets: %w", err)
	}
	automatic := selectSignalFuzzTargets(changed)
	if err := rejectAdditionalFuzzDuplicates(automatic, additional); err != nil {
		return developmentSignalsReport{}, err
	}
	report := makeDevelopmentSignalsReport(coverage, canonicalExplanations, automatic, additional)
	for _, target := range automatic {
		run, err := a.executeDevelopmentFuzz(root, additionalFuzzTarget{Package: target.Package, Target: target.Target}, duration, false)
		if err != nil {
			return developmentSignalsReport{}, fmt.Errorf("run fuzz target %s: %w", target.Target, err)
		}
		report.Fuzz = append(report.Fuzz, signalFuzzReport{
			Boundary: target.Boundary, Package: target.Package, Target: target.Target,
			Duration: run.duration.String(), Workers: run.workers, Offline: run.offline, Result: "success",
		})
	}
	for _, target := range additional {
		run, err := a.executeDevelopmentFuzz(root, target, duration, true)
		if err != nil {
			return developmentSignalsReport{}, fmt.Errorf("run additional fuzz target %s:%s: %w", target.Package, target.Target, err)
		}
		report.AdditionalFuzz = append(report.AdditionalFuzz, additionalFuzzReport{
			Package: target.Package, Target: target.Target, Duration: run.duration.String(),
			Workers: run.workers, Offline: run.offline, Result: "success",
		})
	}
	return report, nil
}

func makeDevelopmentSignalsReport(coverage coverageReport, explanations []coverageExplanation,
	automatic []signalFuzzTarget, additional []additionalFuzzTarget,
) developmentSignalsReport {
	selection := "no-relevant-target"
	if len(automatic) != 0 {
		selection = "selected"
	}
	return developmentSignalsReport{
		Schema: developmentSignalsSchema, Base: coverage.Base, Head: coverage.Head, Coverage: coverage,
		CoverageExplanations: explanations, Selection: selection, Catalog: noMeasuredDevelopmentSignal,
		XSDFeatureSupport: noMeasuredDevelopmentSignal, ExecutableConformance: noMeasuredDevelopmentSignal,
		Fuzz: make([]signalFuzzReport, 0, len(automatic)), AdditionalFuzz: make([]additionalFuzzReport, 0, len(additional)),
	}
}

func (a app) executeDevelopmentFuzz(root string, target additionalFuzzTarget, duration time.Duration,
	validateTarget bool,
) (fuzzRun, error) {
	run, err := newFuzzRun(target.Package, target.Target, duration.String())
	if err != nil {
		return fuzzRun{}, fmt.Errorf("prepare fuzz target %s:%s: %w", target.Package, target.Target, err)
	}
	if validateTarget {
		if _, err := validateFuzzRun(root, run); err != nil {
			return fuzzRun{}, fmt.Errorf("validate fuzz target %s:%s: %w", target.Package, target.Target, err)
		}
	}
	campaign := a
	campaign.stdout = io.Discard
	if err := campaign.executeFuzzRun(root, run); err != nil {
		return fuzzRun{}, err
	}
	return run, nil
}

func parseAdditionalFuzzTargets(values []string) ([]additionalFuzzTarget, error) {
	if len(values) > maxAdditionalFuzzTargets {
		return nil, fmt.Errorf("at most %d additional fuzz targets are allowed", maxAdditionalFuzzTargets)
	}
	targets := make([]additionalFuzzTarget, 0, len(values))
	for _, value := range values {
		target, err := parseAdditionalFuzzTarget(value)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	sort.Slice(targets, func(left, right int) bool {
		if targets[left].Package != targets[right].Package {
			return targets[left].Package < targets[right].Package
		}
		return targets[left].Target < targets[right].Target
	})
	for index := 1; index < len(targets); index++ {
		if targets[index-1] == targets[index] {
			return nil, fmt.Errorf("duplicate additional fuzz target %s:%s", targets[index].Package, targets[index].Target)
		}
	}
	return targets, nil
}

func parseAdditionalFuzzTarget(value string) (additionalFuzzTarget, error) {
	separator := strings.IndexByte(value, ':')
	if separator <= 0 || separator == len(value)-1 || strings.Count(value, ":") != 1 {
		return additionalFuzzTarget{}, fmt.Errorf("additional fuzz target %q must use PACKAGE:TARGET", value)
	}
	packageName := value[:separator]
	targetName := value[separator+1:]
	if err := validateFuzzPackageName(packageName); err != nil {
		return additionalFuzzTarget{}, fmt.Errorf("additional fuzz target %q: %w", value, err)
	}
	if packageName == "./" {
		packageName = "."
	}
	if err := validateFuzzTargetName(targetName); err != nil {
		return additionalFuzzTarget{}, fmt.Errorf("additional fuzz target %q: %w", value, err)
	}
	return additionalFuzzTarget{Package: packageName, Target: targetName}, nil
}

func canonicalizeAdditionalFuzzTargets(root string, targets []additionalFuzzTarget) ([]additionalFuzzTarget, error) {
	canonical := make([]additionalFuzzTarget, 0, len(targets))
	for _, target := range targets {
		relative, err := fuzzPackageRelativeDir(root, target.Package)
		if err != nil {
			return nil, fmt.Errorf("canonicalize additional fuzz package %q: %w", target.Package, err)
		}
		target.Package = "."
		if relative != "." {
			target.Package = "./" + filepath.ToSlash(strings.TrimPrefix(relative, "./"))
		}
		canonical = append(canonical, target)
	}
	sort.Slice(canonical, func(left, right int) bool {
		if canonical[left].Package != canonical[right].Package {
			return canonical[left].Package < canonical[right].Package
		}
		return canonical[left].Target < canonical[right].Target
	})
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1] == canonical[index] {
			return nil, fmt.Errorf("duplicate additional fuzz target %s:%s", canonical[index].Package, canonical[index].Target)
		}
	}
	return canonical, nil
}

func rejectAdditionalFuzzDuplicates(automatic []signalFuzzTarget, additional []additionalFuzzTarget) error {
	for _, extra := range additional {
		for _, target := range automatic {
			if target.Package == extra.Package && target.Target == extra.Target {
				return fmt.Errorf("additional fuzz target %s:%s duplicates automatic policy target", extra.Package, extra.Target)
			}
		}
	}
	return nil
}

func additionalFuzzTargetsFromReport(report developmentSignalsReport) ([]additionalFuzzTarget, error) {
	if err := validateAdditionalFuzz(report.Fuzz, report.AdditionalFuzz); err != nil {
		return nil, err
	}
	targets := make([]additionalFuzzTarget, 0, len(report.AdditionalFuzz))
	for _, result := range report.AdditionalFuzz {
		targets = append(targets, additionalFuzzTarget{Package: result.Package, Target: result.Target})
	}
	return targets, nil
}

func developmentSignalsCampaignDuration(report developmentSignalsReport) (time.Duration, error) {
	duration := time.Duration(0)
	setDuration := func(value string) error {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 || parsed > maxSignalFuzzDuration {
			return fmt.Errorf("campaign duration %q is outside the bounded range", value)
		}
		if duration != 0 && duration != parsed {
			return errors.New("all automatic and additional fuzz campaigns must use one duration")
		}
		duration = parsed
		return nil
	}
	for _, result := range report.Fuzz {
		if err := setDuration(result.Duration); err != nil {
			return 0, err
		}
	}
	for _, result := range report.AdditionalFuzz {
		if err := setDuration(result.Duration); err != nil {
			return 0, err
		}
	}
	if duration == 0 {
		duration = defaultSignalFuzzDuration
	}
	return duration, nil
}

func canonicalCoverageExplanations(explanations []coverageExplanation) ([]coverageExplanation, error) {
	if explanations == nil {
		explanations = []coverageExplanation{}
	}
	canonical := make([]coverageExplanation, len(explanations))
	copy(canonical, explanations)
	for index := range canonical {
		explanation := &canonical[index]
		if explanation.Schema != coverageExplanationSchema || explanation.Package == "" ||
			strings.TrimSpace(explanation.Package) != explanation.Package || strings.TrimSpace(explanation.Reason) == "" ||
			strings.TrimSpace(explanation.Reason) != explanation.Reason {
			return nil, errors.New("each explanation requires the versioned schema, package, and concrete reason")
		}
	}
	sort.Slice(canonical, func(left, right int) bool { return canonical[left].Package < canonical[right].Package })
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].Package == canonical[index].Package {
			return nil, fmt.Errorf("duplicate explanation for package %q", canonical[index].Package)
		}
	}
	return canonical, nil
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
		return []coverageExplanation{}, nil
	}
	var explanations []coverageExplanation
	if err := readStructuredJSONFile(filePath, "coverage explanation file", &explanations); err != nil {
		return nil, err
	}
	return canonicalCoverageExplanations(explanations)
}

func checkCoveragePolicy(report coverageReport, explanations []coverageExplanation) error {
	canonical, err := canonicalCoverageExplanations(explanations)
	if err != nil {
		return err
	}
	byPackage, err := indexCoverageExplanations(canonical)
	if err != nil {
		return err
	}
	if err := checkExplainedCoverageRegressions(report, byPackage); err != nil {
		return err
	}
	return rejectUnusedCoverageExplanations(report, canonical)
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
	for _, fuzz := range report.AdditionalFuzz {
		if err := writeLine(output, "additional fuzz: %s:%s %s workers=%d offline=%t", fuzz.Package, fuzz.Target, fuzz.Result, fuzz.Workers, fuzz.Offline); err != nil {
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
