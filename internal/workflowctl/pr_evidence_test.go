package workflowctl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPREvidenceUpdateCommandUsesSourcesAndPatchesOnlyOwnedBlock(t *testing.T) {
	backend := newWorkflowBackend(t)
	prefix := "## Operator note\n\nPreserve these bytes.\n\n"
	backend.body = prefix + backend.body
	sourceEvidence, _ := evidenceTestView(t)
	signals := sourceEvidence.DevelopmentSignals
	signals.Head = backend.head
	signals.Coverage.Head = backend.head
	audit := testWorkflowPREvidence("base-sha", backend.head).DocumentationAudit
	signalsPath := writePREvidenceJSONSource(t, "signals.json", signals)
	auditPath := writePREvidenceJSONSource(t, "audit.json", audit)
	var stdout, stderr bytes.Buffer
	application := app{executeCommand: backend.execute, stdout: &stdout, stderr: &stderr}

	if err := application.runPREvidence([]string{"update", "14", "--signals-file", signalsPath, "--docs-audit-file", auditPath}); err != nil {
		t.Fatalf("update PR evidence: %v", err)
	}
	if backend.bodyPatchCount != 1 {
		t.Fatalf("body PATCH count = %d, want 1", backend.bodyPatchCount)
	}
	if !strings.HasPrefix(backend.body, prefix) {
		t.Fatalf("body update did not preserve non-owned prefix: %q", backend.body)
	}
	view := pullRequestView{BaseRefOID: "base-sha", HeadRefOID: backend.head, Body: backend.body}
	parsed, err := validatePREvidenceForView(view)
	if err != nil {
		t.Fatalf("updated evidence rejected: %v", err)
	}
	if len(parsed.evidence.DevelopmentSignals.Coverage.Packages) != 1 {
		t.Fatalf("source development signals were not written: %#v", parsed.evidence.DevelopmentSignals.Coverage.Packages)
	}
}

func writePREvidenceJSONSource(t *testing.T, name string, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestPREvidencePreservesNonOwnedBodyAndIsIdempotent(t *testing.T) {
	evidence, view := evidenceTestView(t)
	block, err := renderPREvidenceBlock(evidence)
	if err != nil {
		t.Fatalf("render evidence: %v", err)
	}
	body := "before\n\nCloses #14\n"
	first, err := replacePREvidenceBlock(body, block)
	if err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	if !strings.HasPrefix(first, body) || !strings.Contains(first, string(block)) {
		t.Fatalf("body update did not preserve the original bytes: %q", first)
	}
	second, err := replacePREvidenceBlock(first, block)
	if err != nil {
		t.Fatalf("repeat evidence update: %v", err)
	}
	if second != first {
		t.Fatalf("identical evidence update changed bytes:\n%s\n---\n%s", first, second)
	}
	view.Body = first
	if _, err := validatePREvidenceForView(view); err != nil {
		t.Fatalf("generated evidence rejected: %v", err)
	}
	if _, err := replacePREvidenceBlock(first+"\n"+string(block), block); err == nil {
		t.Fatal("duplicate evidence block was accepted")
	}
}

func TestPREvidenceRejectsStaleAndCopiedResults(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*prEvidence, *pullRequestView)
		want   string
	}{
		{name: "stale base", mutate: func(evidence *prEvidence, _ *pullRequestView) {
			evidence.Base = "old-base"
		}, want: "exact REST base"},
		{name: "stale head", mutate: func(evidence *prEvidence, _ *pullRequestView) {
			evidence.Head = "old-head"
		}, want: "exact PR head"},
		{name: "stale nested signal base", mutate: func(evidence *prEvidence, _ *pullRequestView) {
			evidence.DevelopmentSignals.Base = "old-base"
		}, want: "exact PR base"},
		{name: "copied coverage delta", mutate: func(evidence *prEvidence, _ *pullRequestView) {
			evidence.DevelopmentSignals.Coverage.Packages[0].Delta.Covered++
		}, want: "inconsistent coverage delta"},
		{name: "stale curator head", mutate: func(evidence *prEvidence, _ *pullRequestView) {
			evidence.DocumentationAudit.ManagedChanges = []documentationChangeReport{testManagedChange()}
			evidence.Curator = curatorResult{
				Schema: curatorResultSchema, RunID: "curator-run", Head: "old-head", Verdict: "pass",
				Summary: "No findings.", Findings: []curatorFinding{},
			}
		}, want: "exact PR head"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence, view := evidenceTestView(t)
			test.mutate(&evidence, &view)
			block, err := renderPREvidenceBlock(evidence)
			if err != nil {
				t.Fatalf("render evidence: %v", err)
			}
			view.Body = string(block)
			if _, err := validatePREvidenceForView(view); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("evidence error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPREvidenceCuratorContractFollowsManagedChanges(t *testing.T) {
	evidence, view := evidenceTestView(t)
	evidence.DocumentationAudit.ManagedChanges = []documentationChangeReport{testManagedChange()}
	evidence.Curator = noCuratorResult(view.HeadRefOID)
	if err := validatePREvidence(evidence, view); err == nil || !strings.Contains(err.Error(), "managed-document changes require") {
		t.Fatalf("missing Curator error = %v", err)
	}
	evidence.Curator = curatorResult{
		Schema: curatorResultSchema, RunID: "curator-run", Head: view.HeadRefOID, Verdict: "pass",
		Summary: "Every managed change is in its canonical home.", Findings: []curatorFinding{},
	}
	if err := validatePREvidence(evidence, view); err != nil {
		t.Fatalf("valid Curator result rejected: %v", err)
	}
	evidence.Curator.Verdict = "revise"
	if err := validatePREvidence(evidence, view); err == nil || !strings.Contains(err.Error(), "passing Curator") {
		t.Fatalf("revise Curator result accepted: %v", err)
	}
}

func TestPREvidenceNoDocumentChangeRequiresAuditedReason(t *testing.T) {
	evidence, view := evidenceTestView(t)
	evidence.Curator.Reason = "not-applicable"
	if err := validatePREvidence(evidence, view); err == nil || !strings.Contains(err.Error(), "exact audited") {
		t.Fatalf("wrong no-Curator reason accepted: %v", err)
	}
	evidence.Curator = noCuratorResult(view.HeadRefOID)
	if err := validatePREvidence(evidence, view); err != nil {
		t.Fatalf("valid no-Curator evidence rejected: %v", err)
	}
}

func TestPREvidenceRejectsUnknownDuplicateJSONAndMalformedMarkers(t *testing.T) {
	unknown := `{"schema":"goxsd9/pr-evidence/v1","extra":true}`
	duplicate := `{"schema":"goxsd9/pr-evidence/v1","schema":"goxsd9/pr-evidence/v1"}`
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "unknown", body: evidenceBodyWithJSON(unknown)},
		{name: "duplicate", body: evidenceBodyWithJSON(duplicate)},
		{name: "missing end", body: evidenceMarkerToken(prEvidenceStartMarker) + "\n{}"},
		{name: "missing start", body: "{}\n" + evidenceMarkerToken(prEvidenceEndMarker)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parsePREvidenceBody(test.body); err == nil {
				t.Fatal("malformed evidence was accepted")
			}
		})
	}
}

func TestEvaluationChallengeBindsBodyAndEvidenceDigests(t *testing.T) {
	evidence, view := evidenceTestView(t)
	block, err := renderPREvidenceBlock(evidence)
	if err != nil {
		t.Fatalf("render evidence: %v", err)
	}
	view.Body = "before\n" + string(block) + "\nafter\n"
	parsed, err := validatePREvidenceForView(view)
	if err != nil {
		t.Fatalf("validate evidence: %v", err)
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsed)
	requested := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
	challenge := evaluationChallenge{
		Challenge: "challenge", Head: view.HeadRefOID, PR: 14,
		BodySHA256: bodySHA256, EvidenceSHA256: evidenceSHA256, RequestedAt: requested,
	}
	comment := testEvaluationChallengeComment(t, challenge)
	view.Comments = []pullRequestComment{comment}
	if _, ok := trustedEvaluationChallengeForView(view, challenge.Challenge, challenge.PR, requested); !ok {
		t.Fatal("valid body/evidence-bound challenge was rejected")
	}
	view.Body += "same-head edit\n"
	if _, ok := trustedEvaluationChallengeForView(view, challenge.Challenge, challenge.PR, requested); ok {
		t.Fatal("same-head body edit reused the challenge")
	}
}

func evidenceTestView(t *testing.T) (prEvidence, pullRequestView) {
	t.Helper()
	base, head := "base-sha", "head-sha"
	packages := []coveragePackageReport{{
		Package: "example.com/mod", Status: "changed", Affected: true,
		Base:  coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 5, Percent: 50},
		Head:  coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 6, Percent: 60},
		Delta: coverageDeltaReport{Statements: 0, Covered: 1, Percent: 10},
	}}
	coverage := coverageReport{
		Base: base, Head: head, Packages: packages,
		Affected: coverageTotals(packages, true), Repository: coverageTotals(packages, false),
	}
	evidence := prEvidence{
		Schema: prEvidenceSchema, Base: base, Head: head,
		DevelopmentSignals: developmentSignalsReport{
			Schema: developmentSignalsSchema, Base: base, Head: head, Coverage: coverage,
			Fuzz: []signalFuzzReport{}, Selection: "no-relevant-target",
			Catalog: noMeasuredDevelopmentSignal, XSDFeatureSupport: noMeasuredDevelopmentSignal,
			ExecutableConformance: noMeasuredDevelopmentSignal,
		},
		DocumentationAudit: documentationAuditReport{
			Schema: documentationAuditSchema, Base: base, Head: head, MergeBase: "merge-base-sha",
			ManagedChanges: []documentationChangeReport{}, EvaluationFixtures: []string{},
		},
		Curator: noCuratorResult(head),
	}
	view := pullRequestView{BaseRefName: "main", BaseRefOID: base, HeadRefName: "agent/issue-14", HeadRefOID: head}
	return evidence, view
}

func testManagedChange() documentationChangeReport {
	rule, ok := documentRuleFor("README.md")
	if !ok {
		panic("README.md is not registered")
	}
	return documentationChangeReport{
		Path: "README.md", Status: "M", Additions: 1, Deletions: 1, Lines: 3, Words: 5,
		Charter: rule.charter, MaxLines: rule.maxLines, MaxWords: rule.maxWords, Registered: true,
	}
}

func evidenceBodyWithJSON(data string) string {
	return fmt.Sprintf("%s\n%s\n%s", evidenceMarkerToken(prEvidenceStartMarker), data,
		evidenceMarkerToken(prEvidenceEndMarker))
}

func TestPREvidenceJSONRoundTripIsCanonical(t *testing.T) {
	evidence, _ := evidenceTestView(t)
	block, err := renderPREvidenceBlock(evidence)
	if err != nil {
		t.Fatalf("render evidence: %v", err)
	}
	parsed, err := parsePREvidenceBody(string(block))
	if err != nil {
		t.Fatalf("parse rendered evidence: %v", err)
	}
	second, err := renderPREvidenceBlock(parsed.evidence)
	if err != nil {
		t.Fatalf("rerender evidence: %v", err)
	}
	if !bytes.Equal(block, second) {
		t.Fatalf("rendered evidence was not byte-stable:\n%s\n---\n%s", block, second)
	}
	var decoded map[string]any
	if err := json.Unmarshal(block, &decoded); err == nil {
		t.Fatal("comment markers were unexpectedly valid as JSON")
	}
}
