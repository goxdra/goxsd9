package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestEvaluationReceiptEquivalenceUsesOnlyAuthenticatedFacts(t *testing.T) {
	base := completeEvaluationReceiptFixture()
	changed := base
	changed.RecordedAt = base.RecordedAt.Add(time.Minute)
	if !evaluationReceiptsEquivalent(base, changed) {
		t.Fatal("recording time changed receipt equivalence")
	}

	cases := []struct {
		name   string
		change func(*evaluationReceipt)
	}{
		{name: "PR", change: func(receipt *evaluationReceipt) { receipt.PR++ }},
		{name: "head", change: func(receipt *evaluationReceipt) { receipt.Head = "other-head" }},
		{name: "head ref", change: func(receipt *evaluationReceipt) { receipt.HeadRefName = "other-branch" }},
		{name: "base ref", change: func(receipt *evaluationReceipt) { receipt.BaseRefName = "other-base" }},
		{name: "challenge", change: func(receipt *evaluationReceipt) { receipt.Challenge = "other-challenge" }},
		{name: "attestation", change: func(receipt *evaluationReceipt) { receipt.AttestationSHA256 = strings.Repeat("b", 64) }},
		{name: "evaluator", change: func(receipt *evaluationReceipt) { receipt.Evaluator = "other-evaluator" }},
		{name: "run ID", change: func(receipt *evaluationReceipt) { receipt.EvaluatorRunID = "other-run" }},
		{name: "body digest", change: func(receipt *evaluationReceipt) { receipt.BodySHA256 = strings.Repeat("e", 64) }},
		{name: "evidence digest", change: func(receipt *evaluationReceipt) { receipt.EvidenceSHA256 = strings.Repeat("b", 64) }},
		{name: "report digest", change: func(receipt *evaluationReceipt) { receipt.ReportSHA256 = strings.Repeat("b", 64) }},
		{name: "report transport", change: func(receipt *evaluationReceipt) { receipt.ReportTransport = "other-transport" }},
		{name: "claim proof", change: func(receipt *evaluationReceipt) { receipt.ClaimProofs[0].SHA = "other-sha" }},
		{name: "closing issue", change: func(receipt *evaluationReceipt) { receipt.ClosingIssues[0]++ }},
		{name: "round", change: func(receipt *evaluationReceipt) { receipt.Round++ }},
		{name: "verdict", change: func(receipt *evaluationReceipt) { receipt.Verdict = "fail" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneEvaluationReceipt(base)
			test.change(&candidate)
			if evaluationReceiptsEquivalent(base, candidate) {
				t.Fatal("authenticated field change was treated as equivalent")
			}
		})
	}
}

//nolint:gocognit,funlen // Keep the end-to-end convergence lifecycle assertions together.
func TestEvaluationEquivalentReceiptsConvergeAndRemainInPhysicalHistory(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	appendOrdinaryWorkflowComment(t, backend, "ordinary comment before the first receipt")
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial evaluation: %v", err)
	}
	firstReceipt := backend.comments[len(backend.comments)-1]
	appendOrdinaryWorkflowComment(t, backend, "ordinary comment between equivalent receipts")
	appendEquivalentWorkflowReceipt(t, backend)
	secondReceipt := backend.comments[len(backend.comments)-1]
	physicalBodies := []string{firstReceipt.Body, secondReceipt.Body}
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("converge equivalent evaluations: %v", err)
	}
	if got, want := len(backend.comments), 6; got != want {
		t.Fatalf("physical comment count = %d, want %d including convergence record", got, want)
	}
	if backend.comments[2].Body != physicalBodies[0] || backend.comments[4].Body != physicalBodies[1] ||
		backend.comments[1].Body != "ordinary comment before the first receipt" ||
		backend.comments[3].Body != "ordinary comment between equivalent receipts" {
		t.Fatal("convergence rewrote or deleted an authenticated receipt comment")
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.receipts) != 2 || len(history.convergences) != 1 {
		t.Fatalf("converged history = receipts %d, convergence records %d; want 2 and 1",
			len(history.receipts), len(history.convergences))
	}
	convergence := history.convergences[0]
	if convergence.convergence.Canonical.CommentID != firstReceipt.ID ||
		len(convergence.convergence.Closed) != 1 ||
		convergence.convergence.Closed[0].CommentID != secondReceipt.ID {
		t.Fatalf("convergence sources = %#v, want exact receipt comment IDs", convergence.convergence)
	}
	if strings.Contains(backend.comments[5].Body, `"commentIndex"`) {
		t.Fatal("new convergence source persisted the filtered-slice index")
	}
	receipts, err := evaluationReceipts(physicalWorkflowComments(t, backend))
	if err != nil {
		t.Fatalf("project converged receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].Round != 1 {
		t.Fatalf("logical receipts = %#v, want one round-1 receipt", receipts)
	}
	status, err := evaluationStatusForPR(14, pullRequestView{HeadRefOID: backend.head}, history)
	if err != nil {
		t.Fatalf("project converged status: %v", err)
	}
	if len(status.recordedRounds) != 1 || len(status.challenges) != 1 || !status.challenges[0].resolved {
		t.Fatalf("converged status = %#v, want one resolved logical round", status)
	}
	packets, err := application.collectHistoryEvaluations(backend.root, []pullRequestSummary{
		{Number: 14, MergedAt: time.Now().UTC().Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("collect interleaved converged history: %v", err)
	}
	if len(packets) != 1 || len(packets[0].rounds) != 1 || packets[0].rounds[0].round != 1 {
		t.Fatalf("interleaved history packets = %#v, want one logical round", packets)
	}
	stdout.Reset()
	if err = application.runEvaluation([]string{"status", "14"}); err != nil {
		t.Fatalf("status command over converged history: %v", err)
	}
	if !strings.Contains(stdout.String(), "Recorded rounds: 1") {
		t.Fatalf("status command counted physical duplicates:\n%s", stdout.String())
	}
	packet, err := historyEvaluationPacketForPR(pullRequestSummary{
		Number: 14, MergedAt: time.Now().UTC().Add(time.Hour),
	}, history)
	if err != nil {
		t.Fatalf("project converged history: %v", err)
	}
	if len(packet.rounds) != 1 {
		t.Fatalf("history rounds = %d, want one logical round", len(packet.rounds))
	}
	passes, err := latestEvaluationPasses(pullRequestView{
		BaseRefName: "main",
		Body:        backend.body,
		Comments:    physicalWorkflowComments(t, backend),
		HeadRefName: backend.branch,
		HeadRefOID:  backend.head,
	}, 14)
	if err != nil || !passes {
		t.Fatalf("converged latest evaluation = passes %t, err %v; want pass", passes, err)
	}
	stdout.Reset()
	postCount := backend.commentPostCount
	if err = application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("idempotent convergence retry: %v", err)
	}
	if backend.commentPostCount != postCount || len(backend.comments) != 6 {
		t.Fatalf("idempotent retry mutated history: posts %d->%d comments=%d", postCount,
			backend.commentPostCount, len(backend.comments))
	}
	backend.merged = true
	backend.mergedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	mergedView, err := application.readPullRequest(backend.root, 14)
	if err != nil {
		t.Fatalf("read merged converged PR: %v", err)
	}
	mergeReceipt, err := mergeBoundaryEvaluationReceipt(mergedView, 14)
	if err != nil {
		t.Fatalf("recover merge-boundary proof from converged history: %v", err)
	}
	if mergeReceipt.Round != 1 || mergeReceipt.Verdict != "pass" {
		t.Fatalf("merge-boundary receipt = %#v, want one passing logical round", mergeReceipt)
	}
}

func TestEvaluationReceiptConvergenceVerificationReadFailureIsRetryable(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial evaluation: %v", err)
	}
	appendEquivalentWorkflowReceipt(t, backend)
	if got, want := backend.commentPostCount, 2; got != want {
		t.Fatalf("initial comment POST count = %d, want %d", got, want)
	}

	sentinel := errors.New("simulated convergence verification GET failure")
	failNextPRRead := false
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if failNextPRRead && command == "gh api repos/goxdra/goxsd9/pulls/14" {
			failNextPRRead = false
			return "", sentinel
		}
		output, err := backend.execute(dir, input, name, args...)
		if err == nil && command == "gh api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" &&
			backend.commentPostCount == 3 {
			failNextPRRead = true
		}
		return output, err
	}

	err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil {
		t.Fatal("receipt convergence succeeded after injected GET failure")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("receipt convergence disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("receipt convergence error = %v, want sentinel cause", err)
	}
}

func TestPostAndVerifyEvaluationReceiptOperationBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(*testing.T, *evaluationReceiptBoundaryFixture, error)
		want      operationDisposition
		wantCause bool
	}{
		{name: "PR GET transport", prepare: func(_ *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			failNextReceiptPRRead(fixture, sentinel)
		}, want: operationDispositionRetryable, wantCause: true},
		{name: "POST transport with persisted receipt", prepare: func(_ *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionRetryable, wantCause: true},
		{name: "closed PR", prepare: func(_ *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			fixture.backend.merged = true
			fixture.backend.mergedAt = time.Now().UTC().Truncate(time.Second)
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "metadata drift", prepare: func(_ *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			fixture.backend.branch = "agent/issue-13-moved"
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "evidence drift", prepare: func(t *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			mutateFinalEvaluationEvidence(t, fixture.backend)
			fixture.receipt.BodySHA256 = sha256Hex([]byte(fixture.backend.body))
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "malformed history", prepare: func(_ *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			fixture.backend.comments = append(fixture.backend.comments, issueCommentAPI{
				ID: 2, Body: "<!-- " + evaluationMarker + "not-json -->\n" + evaluationReceiptHeading,
				CreatedAt: time.Now().UTC().Truncate(time.Second), User: struct {
					Login string `json:"login"`
				}{Login: trustedActor},
			})
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "untrusted history", prepare: func(_ *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			fixture.backend.comments = append(fixture.backend.comments, issueCommentAPI{
				ID: 2, Body: "<!-- " + evaluationMarker + "untrusted -->", CreatedAt: time.Now().UTC().Truncate(time.Second), User: struct {
					Login string `json:"login"`
				}{Login: owner},
			})
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "invalid logical history", prepare: func(t *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			invalid := fixture.receiptComment
			invalid.ID = 2
			invalid.Body = replaceTestReceipt(t, invalid.Body, func(receipt *evaluationReceipt) {
				receipt.Round = 0
			})
			fixture.backend.comments = append(fixture.backend.comments, invalid)
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "unsafe identity", prepare: func(t *testing.T, fixture *evaluationReceiptBoundaryFixture, sentinel error) {
			unsafe := fixture.receiptComment
			unsafe.ID = 2
			unsafe.Body = replaceTestReceipt(t, unsafe.Body, func(receipt *evaluationReceipt) {
				receipt.EvaluatorRunID = "unsafe-reused-run"
			})
			fixture.backend.comments = append(fixture.backend.comments, unsafe)
			injectReceiptPostTransport(fixture, sentinel)
		}, want: operationDispositionTerminal, wantCause: true},
		{name: "absent exact receipt", prepare: func(t *testing.T, fixture *evaluationReceiptBoundaryFixture, _ error) {
			fixture.body = alternateReceiptBody(t, fixture)
		}, want: operationDispositionRetryable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newEvaluationReceiptBoundaryFixture(t)
			sentinel := errors.New("receipt boundary sentinel")
			test.prepare(t, fixture, sentinel)
			err := fixture.application.postAndVerifyEvaluationReceipt(fixture.backend.root, 14, 13,
				fixture.receipt, fixture.attestation, fixture.attestationJSON, fixture.body)
			if err == nil {
				t.Fatal("postAndVerifyEvaluationReceipt succeeded")
			}
			if got := operationDispositionOf(err); got != test.want {
				t.Fatalf("postAndVerifyEvaluationReceipt disposition = %v, want %v: %v", got, test.want, err)
			}
			if test.wantCause && !errors.Is(err, sentinel) {
				t.Fatalf("postAndVerifyEvaluationReceipt error = %v, want sentinel cause", err)
			}
		})
	}
}

func TestPostAndVerifyEvaluationReceiptPassesTypedConvergenceFailure(t *testing.T) {
	fixture := newEvaluationReceiptBoundaryFixture(t)
	fixture.backend.comments = append(fixture.backend.comments, fixture.receiptComment)
	sentinel := errors.New("receipt convergence boundary GET failure")
	prReads := 0
	fixture.application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if command == "gh api repos/goxdra/goxsd9/pulls/14" {
			prReads++
			if prReads == 2 {
				return "", sentinel
			}
		}
		return fixture.backend.execute(dir, input, name, args...)
	}
	err := fixture.application.postAndVerifyEvaluationReceipt(fixture.backend.root, 14, 13,
		fixture.receipt, fixture.attestation, fixture.attestationJSON, fixture.body)
	if err == nil {
		t.Fatal("postAndVerifyEvaluationReceipt succeeded after convergence GET failure")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("typed convergence disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("typed convergence error = %v, want sentinel cause", err)
	}
}

type evaluationReceiptBoundaryFixture struct {
	backend         *workflowBackend
	application     *app
	receipt         evaluationReceipt
	attestation     evaluationAttestation
	attestationJSON []byte
	receiptComment  issueCommentAPI
	body            string
}

func newEvaluationReceiptBoundaryFixture(t *testing.T) *evaluationReceiptBoundaryFixture {
	t.Helper()
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	attestationJSON, attestationFile := writeTestAttestationRun(t, backend.head, challenge, "receipt-boundary")
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record receipt boundary fixture: %v", err)
	}
	receiptComment := backend.comments[len(backend.comments)-1]
	receipt, ok := parseEvaluationReceipt(receiptComment.Body)
	if !ok {
		t.Fatal("receipt boundary fixture marker was not parseable")
	}
	var attestation evaluationAttestation
	if err := json.Unmarshal(attestationJSON, &attestation); err != nil {
		t.Fatalf("decode receipt boundary attestation: %v", err)
	}
	backend.comments = append([]issueCommentAPI(nil), backend.comments[:1]...)
	backend.commentPostCount = 0
	return &evaluationReceiptBoundaryFixture{
		backend: backend, application: application, receipt: receipt, attestation: attestation,
		attestationJSON: attestationJSON, receiptComment: receiptComment, body: receiptComment.Body,
	}
}

func injectReceiptPostTransport(fixture *evaluationReceiptBoundaryFixture, sentinel error) {
	fixture.backend.postCommentResponseMode = "transport"
	fixture.application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		output, err := fixture.backend.execute(dir, input, name, args...)
		if name == "gh" && strings.Join(args, " ") == "api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" && err != nil {
			return output, sentinel
		}
		return output, err
	}
}

func failNextReceiptPRRead(fixture *evaluationReceiptBoundaryFixture, sentinel error) {
	failNext := true
	fixture.application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if failNext && command == "gh api repos/goxdra/goxsd9/pulls/14" {
			failNext = false
			return "", sentinel
		}
		return fixture.backend.execute(dir, input, name, args...)
	}
}

func alternateReceiptBody(t *testing.T, fixture *evaluationReceiptBoundaryFixture) string {
	t.Helper()
	var alternate evaluationAttestation
	if err := json.Unmarshal(fixture.attestationJSON, &alternate); err != nil {
		t.Fatalf("decode alternate receipt attestation: %v", err)
	}
	alternate.RunID = "receipt-boundary-alternate"
	alternateJSON, err := json.Marshal(alternate)
	if err != nil {
		t.Fatalf("encode alternate receipt attestation: %v", err)
	}
	receipt := fixture.receipt
	receipt.AttestationSHA256 = sha256Hex(alternateJSON)
	receipt.EvaluatorRunID = alternate.RunID
	marker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode alternate receipt: %v", err)
	}
	return evaluationComment(marker, alternateJSON, string(canonicalEvaluationReport(renderEvaluationReport(alternate))))
}

func TestEvaluationConvergenceRejectsAuthenticatedFieldConflict(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial evaluation: %v", err)
	}
	appendEquivalentWorkflowReceipt(t, backend)
	backend.comments[2].Body = replaceTestReceipt(t, backend.comments[2].Body, func(receipt *evaluationReceipt) {
		receipt.BaseRefName = "different-base"
	})
	commentCount := len(backend.comments)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil {
		t.Fatal("authenticated receipt field conflict was converged")
	}
	if len(backend.comments) != commentCount || backend.commentPostCount != 2 {
		t.Fatal("conflicting receipt history was mutated")
	}
}

func TestEvaluationConvergenceAmbiguousPostRequiresRetryButDoesNotRepost(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial evaluation: %v", err)
	}
	appendEquivalentWorkflowReceipt(t, backend)
	backend.postCommentResponseMode = "transport"
	commentCount := len(backend.comments)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil ||
		!strings.Contains(err.Error(), "do not repost blindly") {
		t.Fatalf("ambiguous convergence error = %v, want retry guidance", err)
	}
	if len(backend.comments) != commentCount+1 || backend.commentPostCount != 3 {
		t.Fatalf("ambiguous convergence mutation = comments %d posts %d, want one attempted convergence POST",
			len(backend.comments), backend.commentPostCount)
	}
	backend.postCommentResponseMode = ""
	postCount := backend.commentPostCount
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("retry authenticated convergence: %v", err)
	}
	if backend.commentPostCount != postCount || len(backend.comments) != commentCount+1 {
		t.Fatalf("retry reposted authenticated convergence: comments=%d posts=%d", len(backend.comments), backend.commentPostCount)
	}
}

func TestEvaluationConcurrentEquivalentReceiptPostConvergesOnce(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	backend.duplicateReceiptPost = true
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("converge concurrent receipt post: %v", err)
	}
	if backend.commentPostCount != 3 || len(backend.comments) != 4 {
		t.Fatalf("concurrent post reconciliation = posts %d comments %d, want one receipt and one convergence POST",
			backend.commentPostCount, len(backend.comments))
	}
	if _, err := evaluationReceipts(physicalWorkflowComments(t, backend)); err != nil {
		t.Fatalf("concurrent post logical projection: %v", err)
	}
}

//nolint:gocognit // Keep the concurrent-marker projection assertions together.
func TestEvaluationRacingConvergencePostsAreEquivalent(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	backend.duplicateConvergencePost = true
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial evaluation: %v", err)
	}
	appendEquivalentWorkflowReceipt(t, backend)
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("reconcile racing convergence posts: %v", err)
	}
	if got, want := backend.commentPostCount, 3; got != want {
		t.Fatalf("racing convergence POST count = %d, want %d", got, want)
	}
	if got, want := len(backend.comments), 5; got != want {
		t.Fatalf("racing convergence physical comment count = %d, want %d", got, want)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.convergences) != 2 {
		t.Fatalf("racing convergence records = %d, want two authenticated equivalent markers", len(history.convergences))
	}
	receipts, err := evaluationReceipts(physicalWorkflowComments(t, backend))
	if err != nil {
		t.Fatalf("project racing convergence receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].Round != 1 {
		t.Fatalf("racing convergence logical receipts = %#v, want one round-1 receipt", receipts)
	}
	status, err := evaluationStatusForPR(14, pullRequestView{HeadRefOID: backend.head}, history)
	if err != nil {
		t.Fatalf("project racing convergence status: %v", err)
	}
	if len(status.recordedRounds) != 1 || len(status.challenges) != 1 || !status.challenges[0].resolved {
		t.Fatalf("racing convergence status = %#v, want one resolved logical round", status)
	}
	packets, err := application.collectHistoryEvaluations(backend.root, []pullRequestSummary{
		{Number: 14, MergedAt: time.Now().UTC().Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("collect racing convergence history: %v", err)
	}
	if len(packets) != 1 || len(packets[0].rounds) != 1 {
		t.Fatalf("racing convergence history packets = %#v, want one logical round", packets)
	}
	postCount := backend.commentPostCount
	commentCount := len(backend.comments)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("retry racing convergence reconciliation: %v", err)
	}
	if backend.commentPostCount != postCount || len(backend.comments) != commentCount {
		t.Fatalf("racing convergence retry mutated history: posts %d->%d comments %d->%d",
			postCount, backend.commentPostCount, commentCount, len(backend.comments))
	}
}

//nolint:gocognit,funlen // Keep late-receipt supersession and every projection assertion together.
func TestEvaluationLateEquivalentReceiptSupersedesConvergence(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial evaluation: %v", err)
	}
	appendEquivalentWorkflowReceipt(t, backend)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial convergence: %v", err)
	}
	firstConvergence := backend.comments[len(backend.comments)-1]
	appendEquivalentWorkflowReceiptAfterConvergence(t, backend)
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("supersede stale convergence: %v", err)
	}
	if got, want := len(backend.comments), 6; got != want {
		t.Fatalf("late-receipt physical comment count = %d, want %d", got, want)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.receipts) != 3 || len(history.convergences) != 2 {
		t.Fatalf("late-receipt history = receipts %d, convergence records %d; want 3 and 2",
			len(history.receipts), len(history.convergences))
	}
	if got, want := len(history.convergences[0].convergence.Closed), 1; got != want {
		t.Fatalf("stale convergence closure length = %d, want %d", got, want)
	}
	if got, want := len(history.convergences[1].convergence.Closed), 2; got != want {
		t.Fatalf("superseding convergence closure length = %d, want %d", got, want)
	}
	receipts, err := evaluationReceipts(physicalWorkflowComments(t, backend))
	if err != nil {
		t.Fatalf("project late-receipt logical receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].Round != 1 {
		t.Fatalf("late-receipt logical receipts = %#v, want one round-1 receipt", receipts)
	}
	status, err := evaluationStatusForPR(14, pullRequestView{HeadRefOID: backend.head}, history)
	if err != nil {
		t.Fatalf("project late-receipt status: %v", err)
	}
	if len(status.recordedRounds) != 1 || len(status.challenges) != 1 || !status.challenges[0].resolved {
		t.Fatalf("late-receipt status = %#v, want one resolved logical round", status)
	}
	packets, err := application.collectHistoryEvaluations(backend.root, []pullRequestSummary{
		{Number: 14, MergedAt: time.Now().UTC().Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("collect late-receipt history: %v", err)
	}
	if len(packets) != 1 || len(packets[0].rounds) != 1 || packets[0].rounds[0].round != 1 {
		t.Fatalf("late-receipt history packets = %#v, want one logical round", packets)
	}
	postCount := backend.commentPostCount
	commentCount := len(backend.comments)
	err = application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err != nil {
		t.Fatalf("retry superseded convergence: %v", err)
	}
	if backend.commentPostCount != postCount || len(backend.comments) != commentCount {
		t.Fatalf("superseded convergence retry mutated history: posts %d->%d comments %d->%d",
			postCount, backend.commentPostCount, commentCount, len(backend.comments))
	}
	passes, err := latestEvaluationPasses(pullRequestView{
		BaseRefName: "main",
		Body:        backend.body,
		Comments:    physicalWorkflowComments(t, backend),
		HeadRefName: backend.branch,
		HeadRefOID:  backend.head,
	}, 14)
	if err != nil || !passes {
		t.Fatalf("late-receipt latest evaluation = passes %t, err %v; want pass", passes, err)
	}
	backend.merged = true
	backend.mergedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	mergedView, err := application.readPullRequest(backend.root, 14)
	if err != nil {
		t.Fatalf("read late-receipt merged PR: %v", err)
	}
	mergeReceipt, err := mergeBoundaryEvaluationReceipt(mergedView, 14)
	if err != nil {
		t.Fatalf("project late-receipt merge-boundary proof: %v", err)
	}
	if mergeReceipt.Round != 1 || mergeReceipt.Verdict != "pass" {
		t.Fatalf("late-receipt merge-boundary receipt = %#v, want one passing logical round", mergeReceipt)
	}
	if firstConvergence.Body == backend.comments[len(backend.comments)-1].Body {
		t.Fatal("superseding convergence reused the stale closure marker")
	}
}

func newConvergenceWorkflowFixture(t *testing.T) (*workflowBackend, *app, *bytes.Buffer) {
	t.Helper()
	backend := newWorkflowBackend(t)
	stdout := new(bytes.Buffer)
	application := &app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         stdout,
		stderr:                         new(bytes.Buffer),
	}
	return backend, application, stdout
}

func appendEquivalentWorkflowReceipt(t *testing.T, backend *workflowBackend) {
	t.Helper()
	receiptIndex := -1
	for index, comment := range backend.comments {
		if hasMarker(comment.Body, evaluationMarker) {
			receiptIndex = index
		}
	}
	if receiptIndex < 0 {
		t.Fatal("equivalent receipt fixture has no receipt comment")
	}
	duplicate := backend.comments[receiptIndex]
	duplicate.ID = nextWorkflowCommentID(backend.comments)
	duplicate.CreatedAt = backend.comments[receiptIndex].CreatedAt
	backend.comments = append(backend.comments, duplicate)
}

func appendEquivalentWorkflowReceiptAfterConvergence(t *testing.T, backend *workflowBackend) {
	t.Helper()
	receiptIndex := -1
	for index, comment := range backend.comments {
		if hasMarker(comment.Body, evaluationMarker) {
			receiptIndex = index
		}
	}
	if receiptIndex < 0 {
		t.Fatal("late equivalent receipt fixture has no receipt comment")
	}
	duplicate := backend.comments[receiptIndex]
	duplicate.ID = nextWorkflowCommentID(backend.comments)
	duplicate.CreatedAt = time.Now().UTC().Truncate(time.Second)
	duplicate.Body = replaceTestReceipt(t, duplicate.Body, func(receipt *evaluationReceipt) {
		receipt.RecordedAt = duplicate.CreatedAt
	})
	backend.comments = append(backend.comments, duplicate)
}

func appendOrdinaryWorkflowComment(t *testing.T, backend *workflowBackend, body string) {
	t.Helper()
	comment := issueCommentAPI{
		ID:        nextWorkflowCommentID(backend.comments),
		Body:      body,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	comment.User.Login = owner
	backend.comments = append(backend.comments, comment)
}

func nextWorkflowCommentID(comments []issueCommentAPI) int64 {
	var next int64 = 1
	for _, comment := range comments {
		if comment.ID >= next {
			next = comment.ID + 1
		}
	}
	return next
}

func physicalWorkflowComments(t *testing.T, backend *workflowBackend) []pullRequestComment {
	t.Helper()
	return pullRequestCommentsFromAPI(t, backend.comments)
}

func completeEvaluationReceiptFixture() evaluationReceipt {
	return evaluationReceipt{
		AttestationSHA256: strings.Repeat("a", 64),
		BaseRefName:       "main",
		Challenge:         "challenge",
		ClaimProofs: []evaluationClaimProof{{
			Issue: 13, Branch: claimBranch(13), SHA: "head",
		}},
		ClosingIssues:   []int{13},
		Evaluator:       "Examiner",
		EvaluatorRunID:  "run",
		Head:            "head",
		HeadRefName:     claimBranch(13),
		BodySHA256:      strings.Repeat("b", 64),
		EvidenceSHA256:  strings.Repeat("c", 64),
		PR:              14,
		RecordedAt:      time.Date(2026, time.August, 26, 12, 1, 0, 0, time.UTC),
		ReportSHA256:    strings.Repeat("d", 64),
		ReportTransport: evaluationReportTransportV1,
		Round:           1,
		Verdict:         "pass",
	}
}

func cloneEvaluationReceipt(receipt evaluationReceipt) evaluationReceipt {
	clone := receipt
	clone.ClaimProofs = append([]evaluationClaimProof(nil), receipt.ClaimProofs...)
	clone.ClosingIssues = append([]int(nil), receipt.ClosingIssues...)
	return clone
}
