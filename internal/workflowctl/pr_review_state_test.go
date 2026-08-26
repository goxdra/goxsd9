package workflowctl

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPRReviewStateMarkerValidation(t *testing.T) {
	pendingBody := testPRReviewStateFrame(t, prReviewStatePending)
	evidenceReadyBody := testPRReviewStateFrame(t, prReviewStateEvidenceReady)
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing", body: "ordinary body", want: "missing"},
		{name: "empty state", body: strings.Replace(pendingBody, prReviewStateToken(prReviewStatePending), "<!-- "+prReviewStateSchema+" -->", 1), want: "empty state"},
		{name: "malformed state", body: strings.Replace(pendingBody, prReviewStateToken(prReviewStatePending), prReviewStateToken("unknown"), 1), want: "unsupported state"},
		{name: "malformed token", body: strings.Replace(pendingBody, prReviewStateToken(prReviewStatePending), "<!-- "+prReviewStateSchema+" pending", 1), want: "unterminated"},
		{name: "duplicate", body: pendingBody + "\n" + prReviewStateToken(prReviewStatePending), want: "appears more than once"},
		{name: "pending", body: pendingBody, want: "pending"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := requirePRReviewStateReady(test.body); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("review-state error = %v, want %q", err, test.want)
			}
		})
	}
	if err := requirePRReviewStateReady(evidenceReadyBody); err != nil {
		t.Fatalf("evidence-ready marker rejected: %v", err)
	}
}

func TestPRReviewStateCanonicalStatusLines(t *testing.T) {
	wantPending := [...]string{
		"Pending exact-base/head development signals.",
		"Pending exact-base documentation audit and Curator result.",
		"Pending evidence update before a fresh challenge-bound Examiner evaluation.",
	}
	wantEvidenceReady := [...]string{
		"Exact-base/head development signals are current in the workflow-owned evidence block.",
		"Exact-base documentation audit and Curator result are current in the workflow-owned evidence block.",
		"Evidence is ready for a fresh challenge-bound Examiner evaluation.",
	}
	for index, spec := range prReviewStateSlotSpecs {
		if spec.pendingStatus != wantPending[index] {
			t.Fatalf("pending status for slot %q = %q, want %q", spec.slot, spec.pendingStatus, wantPending[index])
		}
		if spec.evidenceStatus != wantEvidenceReady[index] {
			t.Fatalf("evidence-ready status for slot %q = %q, want %q", spec.slot, spec.evidenceStatus, wantEvidenceReady[index])
		}
	}
}

func testPRReviewStateFrame(t *testing.T, state string) string {
	return testPRReviewStateFrameOrder(t, state, []int{0, 1, 2})
}

func testPRReviewStateFrameOrder(t *testing.T, state string, order []int) string {
	t.Helper()
	var body strings.Builder
	body.WriteString(prReviewStateToken(state))
	for _, index := range order {
		spec := prReviewStateSlotSpecs[index]
		status, err := prReviewStateStatusLine(spec.slot, state)
		if err != nil {
			t.Fatalf("status line for %s/%s: %v", spec.slot, state, err)
		}
		body.WriteByte('\n')
		body.WriteString(prReviewStateSlotToken(spec.slot))
		body.WriteByte('\n')
		body.WriteString(status)
	}
	return body.String()
}

func TestPRReviewStateLifecycleRejectsMalformedOwnedFrames(t *testing.T) {
	valid := testPRReviewStateFrame(t, prReviewStatePending)
	developmentMarker := prReviewStateSlotToken(prReviewStateSlotSpecs[0].slot)
	developmentStatus, err := prReviewStateStatusLine(prReviewStateSlotSpecs[0].slot, prReviewStatePending)
	if err != nil {
		t.Fatalf("development status: %v", err)
	}
	developmentReadyStatus, err := prReviewStateStatusLine(prReviewStateSlotSpecs[0].slot, prReviewStateEvidenceReady)
	if err != nil {
		t.Fatalf("development evidence-ready status: %v", err)
	}
	tests := []struct {
		name string
		body string
	}{
		{name: "global marker missing", body: strings.TrimPrefix(valid, prReviewStateToken(prReviewStatePending)+"\n")},
		{name: "slot marker missing", body: strings.Replace(valid, developmentMarker+"\n"+developmentStatus+"\n", "", 1)},
		{name: "global marker empty", body: strings.Replace(valid, prReviewStateToken(prReviewStatePending), "<!-- "+prReviewStateSchema+" -->", 1)},
		{name: "global state unknown", body: strings.Replace(valid, prReviewStateToken(prReviewStatePending), prReviewStateToken("unknown"), 1)},
		{name: "version unknown", body: strings.Replace(valid, prReviewStateToken(prReviewStatePending), "<!-- goxsd9/pr-review-state/v2 pending -->", 1)},
		{name: "global marker unterminated", body: strings.Replace(valid, prReviewStateToken(prReviewStatePending), "<!-- "+prReviewStateSchema+" pending", 1)},
		{name: "slot order changed", body: testPRReviewStateFrameOrder(t, prReviewStatePending, []int{1, 0, 2})},
		{name: "global marker duplicated", body: valid + "\n" + prReviewStateToken(prReviewStatePending)},
		{name: "slot marker duplicated", body: valid + "\n" + developmentMarker + "\n" + developmentStatus},
		{name: "marker used as status", body: strings.Replace(valid, developmentStatus, developmentMarker, 1)},
		{name: "unknown slot", body: valid + "\n<!-- " + prReviewStateSchema + " slot receipt -->\nUnknown."},
		{name: "marker embedded in prose", body: strings.Replace(valid, prReviewStateToken(prReviewStatePending), "Prefix "+prReviewStateToken(prReviewStatePending), 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parsePRReviewStateLifecycle(test.body); err == nil {
				t.Fatal("malformed owned lifecycle frame was accepted")
			}
		})
	}
	if _, err := parsePRReviewStateLifecycle(prReviewStateToken(prReviewStateEvidenceReady)); err == nil {
		t.Fatal("old global-only lifecycle frame was accepted")
	}
	stale := strings.Replace(testPRReviewStateFrame(t, prReviewStateEvidenceReady), developmentReadyStatus, "Legacy development signal wording.", 1)
	if _, err := parsePRReviewStateLifecycle(stale); err != nil {
		t.Fatalf("structurally valid stale lifecycle rejected: %v", err)
	}
	if err := requirePRReviewStateReady(stale); err == nil || !strings.Contains(err.Error(), "non-canonical status") {
		t.Fatalf("stale evidence-ready lifecycle error = %v, want non-canonical status", err)
	}
}

func TestPRReviewStateLifecycleReplacementIsCanonicalAndBytePreserving(t *testing.T) {
	pending := testPRReviewStateFrame(t, prReviewStatePending)
	ready := testPRReviewStateFrame(t, prReviewStateEvidenceReady)
	opaque := "Rationale with Pending and evidence-ready wording, links, and headings.\n" +
		"<!-- goxsd9-pr-evidence-start -->\n{\"opaque\":true}\n<!-- goxsd9-pr-evidence-end -->"
	body := "Author prefix.\n" + pending + "\n" + opaque + "\nAuthor suffix.\n"
	want := "Author prefix.\n" + ready + "\n" + opaque + "\nAuthor suffix.\n"

	got, err := replacePRReviewState(body, prReviewStateEvidenceReady)
	if err != nil {
		t.Fatalf("replace lifecycle state: %v", err)
	}
	if got != want {
		t.Fatalf("lifecycle replacement changed opaque bytes:\ngot:\n%s\nwant:\n%s", got, want)
	}
	repeated, err := replacePRReviewState(got, prReviewStateEvidenceReady)
	if err != nil {
		t.Fatalf("repeat lifecycle replacement: %v", err)
	}
	if repeated != got {
		t.Fatalf("repeat lifecycle replacement changed bytes:\ngot:\n%s\nwant:\n%s", repeated, got)
	}
	parsed, err := parsePRReviewStateLifecycle(got)
	if err != nil {
		t.Fatalf("canonical lifecycle rejected: %v", err)
	}
	if parsed.global.state != prReviewStateEvidenceReady {
		t.Fatalf("lifecycle state = %q, want %q", parsed.global.state, prReviewStateEvidenceReady)
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
		{name: "empty state", body: func(body string) string {
			return strings.Replace(body, prReviewStateToken(prReviewStateEvidenceReady), "<!-- "+prReviewStateSchema+" -->", 1)
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
		{name: "stale owned status", body: func(body string) string {
			return strings.Replace(body,
				"Exact-base/head development signals are current in the workflow-owned evidence block.",
				"Legacy development signal wording.", 1)
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
