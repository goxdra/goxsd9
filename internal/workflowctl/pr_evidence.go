package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	prEvidenceSchema            = "goxsd9/pr-evidence/v1"
	documentationAuditSchema    = "goxsd9/docs-audit/v1"
	curatorResultSchema         = "goxsd9/curator-result/v1"
	prEvidenceStartMarker       = "goxsd9-pr-evidence-start"
	prEvidenceEndMarker         = "goxsd9-pr-evidence-end"
	noManagedDocumentChange     = "no-managed-document-change"
	noMeasuredDevelopmentSignal = "not-measured"
)

type documentationAuditReport struct {
	Schema             string                      `json:"schema"`
	Base               string                      `json:"base"`
	Head               string                      `json:"head"`
	MergeBase          string                      `json:"mergeBase"`
	ManagedChanges     []documentationChangeReport `json:"managedChanges"`
	EvaluationFixtures []string                    `json:"evaluationFixtures"`
}

type documentationChangeReport struct {
	Path       string `json:"path"`
	Status     string `json:"status"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Lines      int    `json:"lines"`
	Words      int    `json:"words"`
	Charter    string `json:"charter"`
	MaxLines   int    `json:"maxLines"`
	MaxWords   int    `json:"maxWords"`
	Registered bool   `json:"registered"`
}

type curatorFinding struct {
	Location           string `json:"location"`
	Impact             string `json:"impact"`
	RequiredCorrection string `json:"requiredCorrection"`
}

type curatorResult struct {
	Schema   string           `json:"schema"`
	RunID    string           `json:"runID"`
	Head     string           `json:"head"`
	Verdict  string           `json:"verdict"`
	Summary  string           `json:"summary"`
	Findings []curatorFinding `json:"findings"`
	Reason   string           `json:"reason,omitempty"`
}

type prEvidence struct {
	Schema             string                   `json:"schema"`
	Base               string                   `json:"base"`
	Head               string                   `json:"head"`
	DevelopmentSignals developmentSignalsReport `json:"developmentSignals"`
	DocumentationAudit documentationAuditReport `json:"documentationAudit"`
	Curator            curatorResult            `json:"curator"`
}

type parsedPREvidence struct {
	evidence prEvidence
	block    []byte
}

func evidenceMarkerToken(marker string) string {
	return "<!-- " + marker + " -->"
}

func noCuratorResult(head string) curatorResult {
	return curatorResult{
		Schema: curatorResultSchema, Head: head, Verdict: "not-required",
		Summary:  "No managed-document change; Curator review is not required.",
		Findings: []curatorFinding{}, Reason: noManagedDocumentChange,
	}
}

func documentationAuditReportFrom(rangeValue documentationRange, changes []documentationChange,
	fixtures []string,
) documentationAuditReport {
	report := documentationAuditReport{
		Schema: documentationAuditSchema, Base: rangeValue.base, Head: rangeValue.head,
		MergeBase:          rangeValue.mergeBase,
		ManagedChanges:     make([]documentationChangeReport, 0, len(changes)),
		EvaluationFixtures: append([]string(nil), fixtures...),
	}
	for _, change := range changes {
		report.ManagedChanges = append(report.ManagedChanges, documentationChangeReportFrom(change))
	}
	if report.EvaluationFixtures == nil {
		report.EvaluationFixtures = []string{}
	}
	return report
}

func documentationChangeReportFrom(change documentationChange) documentationChangeReport {
	return documentationChangeReport{
		Path: change.path, Status: change.status, Additions: change.additions, Deletions: change.deletions,
		Lines: change.stats.lines, Words: change.stats.words, Charter: change.rule.charter,
		MaxLines: change.rule.maxLines, MaxWords: change.rule.maxWords, Registered: change.registered,
	}
}

func validateDocumentationAuditReport(report documentationAuditReport, base, head string) error {
	if report.Schema != documentationAuditSchema {
		return fmt.Errorf("schema is %q, want %q", report.Schema, documentationAuditSchema)
	}
	if strings.TrimSpace(report.Base) == "" || report.Base != base {
		return fmt.Errorf("base is %q, want exact PR base %q", report.Base, base)
	}
	if strings.TrimSpace(report.Head) == "" || report.Head != head {
		return fmt.Errorf("head is %q, want exact PR head %q", report.Head, head)
	}
	if strings.TrimSpace(report.MergeBase) == "" {
		return errors.New("mergeBase is empty")
	}
	if report.ManagedChanges == nil || report.EvaluationFixtures == nil {
		return errors.New("managedChanges and evaluationFixtures must be JSON arrays")
	}
	if err := validateDocumentationChangeReports(report.ManagedChanges); err != nil {
		return err
	}
	return validateEvaluationFixtures(report.EvaluationFixtures)
}

func validateDocumentationChangeReports(changes []documentationChangeReport) error {
	for index, change := range changes {
		if index > 0 && changes[index-1].Path >= change.Path {
			return fmt.Errorf("managedChanges are not sorted and unique at %q", change.Path)
		}
		if err := validateDocumentationChangeShape(change); err != nil {
			return err
		}
		if err := validateDocumentationChangeFacts(change); err != nil {
			return err
		}
	}
	return nil
}

func validateDocumentationChangeShape(change documentationChangeReport) error {
	if err := validateGitPath(change.Path); err != nil {
		return fmt.Errorf("managed change %q: %w", change.Path, err)
	}
	if !isDurableMarkdown(change.Path) {
		return fmt.Errorf("managed change %q is not a durable Markdown path", change.Path)
	}
	if change.Status != "A" && change.Status != "D" && change.Status != "M" && change.Status != "T" {
		return fmt.Errorf("managed change %q has unsupported status %q", change.Path, change.Status)
	}
	if change.Additions < 0 || change.Deletions < 0 || change.Lines < 0 || change.Words < 0 ||
		change.MaxLines < 0 || change.MaxWords < 0 {
		return fmt.Errorf("managed change %q has a negative count", change.Path)
	}
	return nil
}

func validateDocumentationChangeFacts(change documentationChangeReport) error {
	rule, registered := documentRuleFor(change.Path)
	if change.Registered != registered {
		return fmt.Errorf("managed change %q has registered=%t, want %t", change.Path, change.Registered, registered)
	}
	if !registered {
		return validateUnregisteredDocumentationChange(change)
	}
	if change.Charter != rule.charter || change.MaxLines != rule.maxLines || change.MaxWords != rule.maxWords {
		return fmt.Errorf("managed change %q has stale registry facts", change.Path)
	}
	if change.Status == "D" {
		if change.Lines != 0 || change.Words != 0 {
			return fmt.Errorf("deleted managed change %q has current document statistics", change.Path)
		}
		return nil
	}
	if change.Lines > change.MaxLines || change.Words > change.MaxWords {
		return fmt.Errorf("managed change %q exceeds its registered document limits", change.Path)
	}
	return nil
}

func validateUnregisteredDocumentationChange(change documentationChangeReport) error {
	if change.Status != "D" || change.Charter != "" || change.MaxLines != 0 || change.MaxWords != 0 ||
		change.Lines != 0 || change.Words != 0 {
		return fmt.Errorf("unregistered deleted change %q has inconsistent document facts", change.Path)
	}
	return nil
}

func validateEvaluationFixtures(fixtures []string) error {
	for index, fixture := range fixtures {
		if index > 0 && fixtures[index-1] >= fixture {
			return fmt.Errorf("evaluationFixtures are not sorted and unique at %q", fixture)
		}
		if !strings.HasPrefix(fixture, "evals/agent/") {
			return fmt.Errorf("evaluation fixture %q is outside evals/agent", fixture)
		}
		if err := validateGitPath(fixture); err != nil {
			return fmt.Errorf("evaluation fixture %q: %w", fixture, err)
		}
	}
	return nil
}

func validateCuratorResult(result curatorResult, audit documentationAuditReport, head string) error {
	if result.Schema != curatorResultSchema {
		return fmt.Errorf("schema is %q, want %q", result.Schema, curatorResultSchema)
	}
	if result.Head != head {
		return fmt.Errorf("head is %q, want exact PR head %q", result.Head, head)
	}
	if strings.TrimSpace(result.Summary) == "" || result.Findings == nil {
		return errors.New("summary and findings array are required")
	}
	if len(audit.ManagedChanges) == 0 {
		if result.RunID != "" || result.Verdict != "not-required" || result.Reason != noManagedDocumentChange || len(result.Findings) != 0 {
			return errors.New("curator is not required only with the exact audited no-managed-document-change reason")
		}
		return nil
	}
	if strings.TrimSpace(result.RunID) == "" || result.Verdict != "pass" || result.Reason != "" {
		return errors.New("managed-document changes require a passing Curator result with a run ID")
	}
	for index, finding := range result.Findings {
		if strings.TrimSpace(finding.Location) == "" || strings.TrimSpace(finding.Impact) == "" ||
			strings.TrimSpace(finding.RequiredCorrection) == "" {
			return fmt.Errorf("curator finding %d is missing location, impact, or required correction", index+1)
		}
	}
	if len(result.Findings) != 0 {
		return errors.New("a passing Curator result must contain no findings")
	}
	return nil
}

func validatePREvidence(evidence prEvidence, view pullRequestView) error {
	if evidence.Schema != prEvidenceSchema {
		return fmt.Errorf("schema is %q, want %q", evidence.Schema, prEvidenceSchema)
	}
	if strings.TrimSpace(view.BaseRefOID) == "" {
		return errors.New("PR metadata has no exact REST base SHA")
	}
	if evidence.Base != view.BaseRefOID {
		return fmt.Errorf("base is %q, want exact REST base SHA %q", evidence.Base, view.BaseRefOID)
	}
	if evidence.Head != view.HeadRefOID {
		return fmt.Errorf("head is %q, want exact PR head %q", evidence.Head, view.HeadRefOID)
	}
	if err := validateDevelopmentSignals(evidence.DevelopmentSignals, evidence.Base, evidence.Head); err != nil {
		return fmt.Errorf("development signals: %w", err)
	}
	if err := validateDocumentationAuditReport(evidence.DocumentationAudit, evidence.Base, evidence.Head); err != nil {
		return fmt.Errorf("documentation audit: %w", err)
	}
	return validateCuratorResult(evidence.Curator, evidence.DocumentationAudit, evidence.Head)
}

func validateDevelopmentSignals(report developmentSignalsReport, expectedBase, expectedHead string) error {
	if report.Schema != developmentSignalsSchema {
		return fmt.Errorf("schema is %q, want %q", report.Schema, developmentSignalsSchema)
	}
	if strings.TrimSpace(report.Base) == "" || strings.TrimSpace(report.Head) == "" {
		return errors.New("base and head are required")
	}
	if report.Base != expectedBase {
		return fmt.Errorf("base is %q, want exact PR base %q", report.Base, expectedBase)
	}
	if report.Head != expectedHead {
		return fmt.Errorf("head is %q, want exact PR head %q", report.Head, expectedHead)
	}
	if report.CoverageExplanations == nil {
		return errors.New("coverageExplanations must be a JSON array")
	}
	if report.Fuzz == nil {
		return errors.New("fuzz must be a JSON array")
	}
	if report.AdditionalFuzz == nil {
		return errors.New("additionalFuzz must be a JSON array")
	}
	if report.Catalog != noMeasuredDevelopmentSignal || report.XSDFeatureSupport != noMeasuredDevelopmentSignal ||
		report.ExecutableConformance != noMeasuredDevelopmentSignal {
		return errors.New("catalog, XSD feature support, and executable conformance must remain explicitly not-measured")
	}
	if err := validateCoverageReport(report.Coverage, report.Base, report.Head); err != nil {
		return fmt.Errorf("coverage: %w", err)
	}
	if err := validateCoverageExplanations(report.CoverageExplanations); err != nil {
		return fmt.Errorf("coverage explanations: %w", err)
	}
	if err := validateSignalFuzz(report.Selection, report.Fuzz); err != nil {
		return err
	}
	return validateAdditionalFuzz(report.Fuzz, report.AdditionalFuzz)
}

func validateSignalFuzz(selection string, fuzz []signalFuzzReport) error {
	if selection != "selected" && selection != "no-relevant-target" {
		return fmt.Errorf("selection is %q, want selected or no-relevant-target", selection)
	}
	if selection == "no-relevant-target" && len(fuzz) != 0 {
		return errors.New("no-relevant-target must have an empty fuzz result")
	}
	if selection == "selected" && len(fuzz) == 0 {
		return errors.New("selected must have at least one fuzz result")
	}
	previous := -1
	for _, result := range fuzz {
		policyIndex := signalFuzzTargetIndex(result.Boundary, result.Package, result.Target)
		if policyIndex < 0 {
			return fmt.Errorf("fuzz result %q is not a configured target", result.Target)
		}
		if policyIndex <= previous {
			return errors.New("fuzz results are not in deterministic target order or contain duplicates")
		}
		previous = policyIndex
		if err := validateSignalFuzzResult(result); err != nil {
			return err
		}
	}
	return nil
}

func validateSignalFuzzResult(result signalFuzzReport) error {
	if result.Workers != 1 || !result.Offline || result.Result != "success" {
		return fmt.Errorf("fuzz result %q does not prove bounded offline single-worker success", result.Target)
	}
	duration, err := time.ParseDuration(result.Duration)
	if err != nil || duration <= 0 || duration > maxSignalFuzzDuration {
		return fmt.Errorf("fuzz result %q has invalid duration %q", result.Target, result.Duration)
	}
	return nil
}

func validateCoverageExplanations(explanations []coverageExplanation) error {
	if _, err := canonicalCoverageExplanations(explanations); err != nil {
		return err
	}
	for index, explanation := range explanations {
		if index > 0 && explanations[index-1].Package >= explanation.Package {
			return fmt.Errorf("coverage explanations are not sorted and unique at %q", explanation.Package)
		}
	}
	return nil
}

func validateAdditionalFuzz(automatic []signalFuzzReport, additional []additionalFuzzReport) error {
	if len(additional) > maxAdditionalFuzzTargets {
		return fmt.Errorf("additionalFuzz contains %d results; maximum is %d", len(additional), maxAdditionalFuzzTargets)
	}
	previous := additionalFuzzTarget{}
	for index, result := range additional {
		current, err := validateAdditionalFuzzShape(index, result)
		if err != nil {
			return err
		}
		if index > 0 && (previous.Package > current.Package ||
			(previous.Package == current.Package && previous.Target >= current.Target)) {
			return fmt.Errorf("additional fuzz results are not sorted and unique at %s:%s", current.Package, current.Target)
		}
		previous = current
		if signalFuzzTargetIndexByPackageTarget(automatic, current.Package, current.Target) >= 0 {
			return fmt.Errorf("additional fuzz target %s:%s duplicates an automatic result", current.Package, current.Target)
		}
		if err := validateAdditionalFuzzResult(result); err != nil {
			return err
		}
	}
	return nil
}

func validateAdditionalFuzzShape(index int, result additionalFuzzReport) (additionalFuzzTarget, error) {
	if result.Package == "./" {
		return additionalFuzzTarget{}, fmt.Errorf("additional fuzz result %d uses non-canonical package %q", index+1, result.Package)
	}
	if err := validateFuzzPackageName(result.Package); err != nil {
		return additionalFuzzTarget{}, fmt.Errorf("additional fuzz result %d: %w", index+1, err)
	}
	if err := validateFuzzTargetName(result.Target); err != nil {
		return additionalFuzzTarget{}, fmt.Errorf("additional fuzz result %d: %w", index+1, err)
	}
	return additionalFuzzTarget{Package: result.Package, Target: result.Target}, nil
}

func validateAdditionalFuzzResult(result additionalFuzzReport) error {
	if result.Workers != 1 || !result.Offline || result.Result != "success" {
		return fmt.Errorf("additional fuzz result %q does not prove bounded offline single-worker success", result.Target)
	}
	duration, err := time.ParseDuration(result.Duration)
	if err != nil || duration <= 0 || duration > maxSignalFuzzDuration {
		return fmt.Errorf("additional fuzz result %q has invalid duration %q", result.Target, result.Duration)
	}
	return nil
}

func signalFuzzTargetIndexByPackageTarget(fuzz []signalFuzzReport, packageName, targetName string) int {
	for index, result := range fuzz {
		if result.Package == packageName && result.Target == targetName {
			return index
		}
	}
	return -1
}

func signalFuzzTargetIndex(boundary, packageName, targetName string) int {
	for index, target := range signalFuzzTargets {
		if target.Boundary == boundary && target.Package == packageName && target.Target == targetName {
			return index
		}
	}
	return -1
}

func validateCoverageReport(report coverageReport, base, head string) error {
	if report.Base != base || report.Head != head {
		return fmt.Errorf("report base/head are %q/%q, want %q/%q", report.Base, report.Head, base, head)
	}
	if report.Packages == nil {
		return errors.New("packages must be a JSON array")
	}
	for index, packageReport := range report.Packages {
		previous := ""
		if index > 0 {
			previous = report.Packages[index-1].Package
		}
		if err := validateCoveragePackage(packageReport, index, previous); err != nil {
			return err
		}
	}
	if report.Affected != coverageTotals(report.Packages, true) {
		return errors.New("affected coverage totals are inconsistent with package results")
	}
	if report.Repository != coverageTotals(report.Packages, false) {
		return errors.New("repository coverage totals are inconsistent with package results")
	}
	return nil
}

func validateCoveragePackage(packageReport coveragePackageReport, index int, previous string) error {
	if strings.TrimSpace(packageReport.Package) == "" {
		return fmt.Errorf("package %d has no name", index+1)
	}
	if index > 0 && previous >= packageReport.Package {
		return fmt.Errorf("packages are not sorted and unique at %q", packageReport.Package)
	}
	if err := validateCoverageSide(packageReport.Base, packageReport.Package+" base"); err != nil {
		return err
	}
	if err := validateCoverageSide(packageReport.Head, packageReport.Package+" head"); err != nil {
		return err
	}
	return validateCoveragePackageFacts(packageReport)
}

func validateCoveragePackageFacts(packageReport coveragePackageReport) error {
	expectedDelta := coverageDelta(packageReport.Base, packageReport.Head)
	if packageReport.Delta != expectedDelta {
		return fmt.Errorf("package %q has inconsistent coverage delta", packageReport.Package)
	}
	expectedStatus := coveragePackageStatus(packageReport.Base.Present, packageReport.Head.Present, packageReport.Affected)
	if packageReport.Status != expectedStatus {
		return fmt.Errorf("package %q has status %q, want %q", packageReport.Package, packageReport.Status, expectedStatus)
	}
	if packageReport.Status == "added" || packageReport.Status == "removed" {
		if !packageReport.Affected {
			return fmt.Errorf("package %q is added or removed but not affected", packageReport.Package)
		}
	}
	return nil
}

func validateCoverageSide(side coverageSideReport, label string) error {
	if !side.Present {
		if side != (coverageSideReport{}) {
			return fmt.Errorf("%s is absent but contains coverage facts", label)
		}
		return nil
	}
	if side.Statements < 0 || side.Covered < 0 || side.Covered > side.Statements || side.Percent < 0 || side.Percent > 100 ||
		math.IsNaN(side.Percent) || math.IsInf(side.Percent, 0) {
		return fmt.Errorf("%s has invalid coverage counts", label)
	}
	if side.Percent != coveragePercent(side.Covered, side.Statements) {
		return fmt.Errorf("%s has an inconsistent percentage", label)
	}
	return nil
}

func parsePREvidenceBody(body string) (parsedPREvidence, error) {
	startToken := evidenceMarkerToken(prEvidenceStartMarker)
	endToken := evidenceMarkerToken(prEvidenceEndMarker)
	startCount := strings.Count(body, startToken)
	endCount := strings.Count(body, endToken)
	if startCount != 1 || endCount != 1 {
		return parsedPREvidence{}, fmt.Errorf("PR body must contain exactly one evidence start and end marker (got %d and %d)", startCount, endCount)
	}
	start := strings.Index(body, startToken)
	end := strings.Index(body, endToken)
	if start < 0 || end < start+len(startToken) {
		return parsedPREvidence{}, errors.New("PR evidence markers are out of order")
	}
	payload := body[start+len(startToken) : end]
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return parsedPREvidence{}, errors.New("PR evidence block is empty")
	}
	evidence, err := decodePREvidenceJSON([]byte(trimmed))
	if err != nil {
		return parsedPREvidence{}, err
	}
	blockEnd := end + len(endToken)
	return parsedPREvidence{evidence: evidence, block: []byte(body[start:blockEnd])}, nil
}

func decodePREvidenceJSON(data []byte) (prEvidence, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return prEvidence{}, fmt.Errorf("decode PR evidence: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var evidence prEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return prEvidence{}, fmt.Errorf("decode PR evidence: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return prEvidence{}, fmt.Errorf("decode PR evidence trailer: %w", err)
	}
	return evidence, nil
}

func renderPREvidenceBlock(evidence prEvidence) ([]byte, error) {
	data, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("encode PR evidence: %w", err)
	}
	startToken := evidenceMarkerToken(prEvidenceStartMarker)
	endToken := evidenceMarkerToken(prEvidenceEndMarker)
	block := make([]byte, 0, len(startToken)+len(data)+len(endToken)+2)
	block = append(block, startToken...)
	block = append(block, '\n')
	block = append(block, data...)
	block = append(block, '\n')
	block = append(block, endToken...)
	return block, nil
}

func replacePREvidenceBlock(body string, block []byte) (string, error) {
	startToken := evidenceMarkerToken(prEvidenceStartMarker)
	endToken := evidenceMarkerToken(prEvidenceEndMarker)
	startCount := strings.Count(body, startToken)
	endCount := strings.Count(body, endToken)
	if startCount == 0 && endCount == 0 {
		return appendPREvidenceBlock(body, block), nil
	}
	if startCount != 1 || endCount != 1 {
		return "", errors.New("PR body has duplicate or incomplete evidence markers")
	}
	if _, err := parsePREvidenceBody(body); err != nil {
		return "", fmt.Errorf("existing PR evidence block is invalid: %w", err)
	}
	start := strings.Index(body, startToken)
	end := strings.Index(body, endToken) + len(endToken)
	return body[:start] + string(block) + body[end:], nil
}

func appendPREvidenceBlock(body string, block []byte) string {
	if body == "" {
		return string(block)
	}
	if strings.HasSuffix(body, "\n") {
		return body + "\n" + string(block)
	}
	return body + "\n\n" + string(block)
}

func readStructuredJSONFile(path, label string, target any) error {
	if err := requireRegularFile(path); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	// #nosec G304 -- the path is an explicit operator-supplied evidence file.
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return fmt.Errorf("decode %s trailer: %w", label, err)
	}
	return nil
}

func readPREvidenceSources(signalsPath, auditPath, curatorPath string, view pullRequestView) (prEvidence, error) {
	var signals developmentSignalsReport
	if err := readStructuredJSONFile(signalsPath, "development signals", &signals); err != nil {
		return prEvidence{}, err
	}
	var audit documentationAuditReport
	if err := readStructuredJSONFile(auditPath, "documentation audit", &audit); err != nil {
		return prEvidence{}, err
	}
	curator := curatorResult{}
	hasCurator := strings.TrimSpace(curatorPath) != ""
	if !hasCurator {
		curator = noCuratorResult(view.HeadRefOID)
	}
	if hasCurator {
		if err := readStructuredJSONFile(curatorPath, "Curator result", &curator); err != nil {
			return prEvidence{}, err
		}
	}
	evidence := prEvidence{
		Schema: prEvidenceSchema, Base: view.BaseRefOID, Head: view.HeadRefOID,
		DevelopmentSignals: signals, DocumentationAudit: audit, Curator: curator,
	}
	if err := validatePREvidence(evidence, view); err != nil {
		return prEvidence{}, stateError("PR evidence is invalid: %v", err)
	}
	return evidence, nil
}

func validatePREvidenceForView(view pullRequestView) (parsedPREvidence, error) {
	parsed, err := parsePREvidenceBody(view.Body)
	if err != nil {
		return parsedPREvidence{}, err
	}
	if err := validatePREvidence(parsed.evidence, view); err != nil {
		return parsedPREvidence{}, err
	}
	return parsed, nil
}

func (a app) validatePREvidenceForPR(root string, number int, view pullRequestView) (parsedPREvidence, error) {
	parsed, err := validatePREvidenceForView(view)
	if err != nil {
		return parsedPREvidence{}, stateError("PR #%d evidence is invalid: %v", number, err)
	}
	if err := a.validatePREvidenceForExactHead(root, number, view, parsed.evidence); err != nil {
		return parsedPREvidence{}, err
	}
	return parsed, nil
}

func (a app) validatePREvidenceForExactHead(root string, number int, view pullRequestView, evidence prEvidence) error {
	if err := a.verifyDevelopmentSignalsForExactHead(root, evidence.DevelopmentSignals); err != nil {
		return stateError("PR #%d development signals could not be independently recomputed: %v", number, err)
	}
	audit, err := a.documentationAuditReportForCommits(root, evidence.DocumentationAudit.Base,
		evidence.DocumentationAudit.Head)
	if err != nil {
		return stateError("PR #%d documentation audit could not be verified: %v", number, err)
	}
	if !documentationAuditReportsEqual(audit, evidence.DocumentationAudit) {
		return stateError("PR #%d documentation audit result does not match the exact current PR diff", number)
	}
	if evidence.Base != view.BaseRefOID || evidence.Head != view.HeadRefOID {
		return stateError("PR #%d evidence changed while validating the exact REST head", number)
	}
	return nil
}

func (a app) verifyDevelopmentSignalsForExactHead(root string, expected developmentSignalsReport) error {
	if err := validateDevelopmentSignals(expected, expected.Base, expected.Head); err != nil {
		return err
	}
	localHead, err := a.resolveCommit(root, "HEAD")
	if err != nil {
		return fmt.Errorf("resolve local HEAD: %w", err)
	}
	if localHead != expected.Head {
		return fmt.Errorf("local HEAD %q does not match exact REST head %q", localHead, expected.Head)
	}
	localBase, err := a.resolveCommit(root, expected.Base)
	if err != nil {
		return fmt.Errorf("resolve local PR base %q: %w", expected.Base, err)
	}
	if localBase != expected.Base {
		return fmt.Errorf("local PR base %q does not match exact REST base %q", localBase, expected.Base)
	}
	verify := a.verifyDevelopmentSignalsReport
	if verify != nil {
		return verify(root, expected)
	}
	duration, err := developmentSignalsCampaignDuration(expected)
	if err != nil {
		return err
	}
	additional, err := additionalFuzzTargetsFromReport(expected)
	if err != nil {
		return err
	}
	build := a.buildDevelopmentSignalsReport
	if build == nil {
		build = a.buildDevelopmentSignalsReportFromRepository
	}
	fresh, err := build(root, expected.Base, expected.Head, duration, expected.CoverageExplanations, additional)
	if err != nil {
		return err
	}
	if freshErr := validateDevelopmentSignals(fresh, expected.Base, expected.Head); freshErr != nil {
		return fmt.Errorf("fresh development signals are invalid: %w", freshErr)
	}
	want, err := canonicalDevelopmentSignalsJSON(expected)
	if err != nil {
		return err
	}
	got, err := canonicalDevelopmentSignalsJSON(fresh)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, got) {
		return errors.New("freshly recomputed development signals differ from the complete v2 payload")
	}
	return nil
}

func canonicalDevelopmentSignalsJSON(report developmentSignalsReport) ([]byte, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("encode development signals for comparison: %w", err)
	}
	return data, nil
}

func documentationAuditReportsEqual(left, right documentationAuditReport) bool {
	if left.Schema != right.Schema || left.Base != right.Base || left.Head != right.Head || left.MergeBase != right.MergeBase ||
		len(left.ManagedChanges) != len(right.ManagedChanges) || len(left.EvaluationFixtures) != len(right.EvaluationFixtures) {
		return false
	}
	for index := range left.ManagedChanges {
		if left.ManagedChanges[index] != right.ManagedChanges[index] {
			return false
		}
	}
	for index := range left.EvaluationFixtures {
		if left.EvaluationFixtures[index] != right.EvaluationFixtures[index] {
			return false
		}
	}
	return true
}

func currentPREvidenceDigest(view pullRequestView, parsed parsedPREvidence) (string, string) {
	return sha256Hex([]byte(view.Body)), sha256Hex(parsed.block)
}

func (a app) runPREvidence(args []string) error {
	if len(args) == 0 || args[0] != "update" {
		return usageError("usage: workflowctl pr evidence update PR --signals-file FILE --docs-audit-file FILE [--curator-file FILE]")
	}
	if len(args) < 2 {
		return usageError("usage: workflowctl pr evidence update PR --signals-file FILE --docs-audit-file FILE [--curator-file FILE]")
	}
	number, err := positiveNumber(args[1])
	if err != nil {
		return usageError("pr evidence update: %v", err)
	}
	flags := flag.NewFlagSet("pr evidence update", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	signalsPath := flags.String("signals-file", "", "development signals JSON")
	auditPath := flags.String("docs-audit-file", "", "documentation audit JSON")
	curatorPath := flags.String("curator-file", "", "Curator result JSON")
	if err := flags.Parse(args[2:]); err != nil {
		return usageError("pr evidence update: %v", err)
	}
	if flags.NArg() != 0 || *signalsPath == "" || *auditPath == "" {
		return usageError("usage: workflowctl pr evidence update PR --signals-file FILE --docs-audit-file FILE [--curator-file FILE]")
	}
	return a.updatePREvidence(number, *signalsPath, *auditPath, *curatorPath)
}

func (a app) updatePREvidence(number int, signalsPath, auditPath, curatorPath string) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if _, stateErr := parsePRReviewStateMarker(view.Body); stateErr != nil {
		return stateError("PR #%d evidence update refused: %v", number, stateErr)
	}
	evidence, err := readPREvidenceSources(signalsPath, auditPath, curatorPath, view)
	if err != nil {
		return err
	}
	if evidenceErr := a.validatePREvidenceForExactHead(root, number, view, evidence); evidenceErr != nil {
		return evidenceErr
	}
	block, err := renderPREvidenceBlock(evidence)
	if err != nil {
		return err
	}
	updatedBody, err := replacePREvidenceBlock(view.Body, block)
	if err != nil {
		return stateError("PR #%d evidence update refused: %v", number, err)
	}
	updatedBody, err = replacePRReviewState(updatedBody, prReviewStateEvidenceReady)
	if err != nil {
		return stateError("PR #%d evidence update refused: %v", number, err)
	}
	if updatedBody == view.Body {
		return writeLine(a.stdout, "PR #%d evidence is already current at %s", number, view.HeadRefOID)
	}
	if updateErr := a.updatePullRequestBody(root, number, updatedBody); updateErr != nil {
		return updateErr
	}
	updated, err := a.readPullRequest(root, number)
	if err != nil {
		return fmt.Errorf("verify PR #%d evidence update: %w", number, err)
	}
	if updated.BaseRefOID != view.BaseRefOID || updated.HeadRefOID != view.HeadRefOID {
		return stateError("PR #%d changed base or head during evidence update; fresh evidence is required", number)
	}
	if updated.Body != updatedBody {
		return stateError("PR #%d evidence update was not preserved by GitHub", number)
	}
	if stateErr := requirePRReviewStateReady(updated.Body); stateErr != nil {
		return stateError("PR #%d evidence update has invalid review state after reread: %v", number, stateErr)
	}
	if _, err := validatePREvidenceForView(updated); err != nil {
		return stateError("PR #%d evidence update is invalid after reread: %v", number, err)
	}
	return writeLine(a.stdout, "PR #%d evidence updated for %s", number, view.HeadRefOID)
}

func (a app) updatePullRequestBody(root string, number int, body string) error {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return fmt.Errorf("encode PR #%d body update: %w", number, err)
	}
	if _, err := a.commandInput(root, strings.NewReader(string(payload)), "gh", "api", "--method", "PATCH",
		"repos/"+repositoryKey+"/pulls/"+strconv.Itoa(number), "--input", "-"); err != nil {
		return fmt.Errorf("update PR #%d body: %w", number, err)
	}
	return nil
}
