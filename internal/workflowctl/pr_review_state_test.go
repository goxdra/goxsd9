package workflowctl

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPRReviewStateMarkerValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing", body: "ordinary body", want: "missing"},
		{name: "malformed state", body: prReviewStateToken("unknown"), want: "unsupported state"},
		{name: "malformed token", body: "<!-- " + prReviewStateSchema + " pending", want: "unterminated"},
		{name: "duplicate", body: prReviewStateToken(prReviewStatePending) + "\n" + prReviewStateToken(prReviewStateEvidenceReady), want: "appears 2 times"},
		{name: "pending", body: prReviewStateToken(prReviewStatePending), want: "pending"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := requirePRReviewStateReady(test.body); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("review-state error = %v, want %q", err, test.want)
			}
		})
	}
	if err := requirePRReviewStateReady(prReviewStateToken(prReviewStateEvidenceReady)); err != nil {
		t.Fatalf("evidence-ready marker rejected: %v", err)
	}
}

func TestPREvidenceUpdateTransitionsReviewStateAndIsIdempotent(t *testing.T) {
	backend := newWorkflowBackend(t)
	pendingBody, err := replacePRReviewState(backend.body, prReviewStatePending)
	if err != nil {
		t.Fatalf("set pending review state: %v", err)
	}
	const prefix = "Operator bytes before owned marker.\n"
	const suffix = "\nOperator bytes after evidence.\n"
	backend.body = prefix + pendingBody + suffix
	evidence, _ := evidenceTestView(t)
	evidence.Head = backend.head
	evidence.DevelopmentSignals.Head = backend.head
	evidence.DevelopmentSignals.Coverage.Head = backend.head
	evidence.DocumentationAudit = testWorkflowPREvidence(backend.head).DocumentationAudit
	evidence.Curator.Head = backend.head
	sourceBlock, err := renderPREvidenceBlock(evidence)
	if err != nil {
		t.Fatalf("render source evidence: %v", err)
	}
	expectedBody, err := replacePREvidenceBlock(backend.body, sourceBlock)
	if err != nil {
		t.Fatalf("build expected evidence body: %v", err)
	}
	expectedBody, err = replacePRReviewState(expectedBody, prReviewStateEvidenceReady)
	if err != nil {
		t.Fatalf("build expected review-ready body: %v", err)
	}
	signalsPath := writePREvidenceJSONSource(t, "signals.json", evidence.DevelopmentSignals)
	auditPath := writePREvidenceJSONSource(t, "audit.json", evidence.DocumentationAudit)
	application := app{
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         new(bytes.Buffer),
		stderr:                         new(bytes.Buffer),
	}

	if err := application.runPREvidence([]string{"update", "14", "--signals-file", signalsPath, "--docs-audit-file", auditPath}); err != nil {
		t.Fatalf("update pending evidence: %v", err)
	}
	if backend.bodyPatchCount != 1 {
		t.Fatalf("body PATCH count = %d, want 1", backend.bodyPatchCount)
	}
	if err := requirePRReviewStateReady(backend.body); err != nil {
		t.Fatalf("updated review state is not ready: %v", err)
	}
	if backend.body != expectedBody {
		t.Fatalf("update changed bytes outside owned state/evidence: got %q, want %q", backend.body, expectedBody)
	}
	if !strings.HasPrefix(backend.body, prefix) || !strings.HasSuffix(backend.body, suffix) {
		t.Fatalf("update did not preserve arbitrary body bytes: %q", backend.body)
	}
	first := backend.body
	if err := application.runPREvidence([]string{"update", "14", "--signals-file", signalsPath, "--docs-audit-file", auditPath}); err != nil {
		t.Fatalf("repeat evidence update: %v", err)
	}
	if backend.bodyPatchCount != 1 || backend.body != first {
		t.Fatalf("repeat evidence update changed the body or PATCH count: patches=%d", backend.bodyPatchCount)
	}
}

func TestReviewStateGateRejectsBeforeChallengeAndFinishMutation(t *testing.T) {
	tests := []struct {
		name string
		body func(string) string
	}{
		{name: "missing", body: func(body string) string {
			return strings.Replace(body, prReviewStateToken(prReviewStateEvidenceReady)+"\n", "", 1)
		}},
		{name: "malformed", body: func(body string) string {
			return strings.Replace(body, prReviewStateToken(prReviewStateEvidenceReady), "<!-- "+prReviewStateSchema+" unknown -->", 1)
		}},
		{name: "duplicate", body: func(body string) string {
			return body + "\n" + prReviewStateToken(prReviewStateEvidenceReady)
		}},
		{name: "pending", body: func(body string) string {
			return strings.Replace(body, prReviewStateToken(prReviewStateEvidenceReady), prReviewStateToken(prReviewStatePending), 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name+" challenge", func(t *testing.T) {
			assertReviewStateGateRejects(t, test.body, "challenge")
		})
		t.Run(test.name+" finish", func(t *testing.T) {
			assertReviewStateGateRejects(t, test.body, "finish")
		})
	}
}

func assertReviewStateGateRejects(t *testing.T, mutate func(string) string, command string) {
	t.Helper()
	backend := newWorkflowBackend(t)
	backend.body = mutate(backend.body)
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         new(bytes.Buffer),
		stderr:                         new(bytes.Buffer),
	}
	var err error
	if command == "challenge" {
		err = application.runEvaluation([]string{"challenge", "14"})
	}
	if command == "finish" {
		err = application.runPR(backend.finishArgs())
	}
	if err == nil || !strings.Contains(err.Error(), "review state") {
		t.Fatalf("%s error = %v, want review-state rejection", command, err)
	}
	if len(backend.comments) != 0 || backend.bodyPatchCount != 0 || backend.merged || backend.issuePatchCount != 0 || backend.projectDone {
		t.Fatalf("rejected %s mutated state: comments=%d body patches=%d merged=%t issue patches=%d project done=%t",
			command, len(backend.comments), backend.bodyPatchCount, backend.merged, backend.issuePatchCount, backend.projectDone)
	}
}

func TestReviewStateGateAllowsLegitimateExaminerPendingProse(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.body += "\nFresh Examiner receipt/evaluation pending.\n"
	originalBody := backend.body
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         new(bytes.Buffer),
		stderr:                         new(bytes.Buffer),
	}
	if err := application.runEvaluation([]string{"challenge", "14"}); err != nil {
		t.Fatalf("legitimate Examiner-pending prose rejected: %v", err)
	}
	if len(backend.comments) != 1 {
		t.Fatalf("challenge comments = %d, want 1", len(backend.comments))
	}
	if backend.body != originalBody {
		t.Fatal("challenge changed the arbitrary PR body")
	}
}
