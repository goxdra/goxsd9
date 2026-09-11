package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluationChallengeHistoryBlocksReplacementAfterHeadAndBodyMove(t *testing.T) {
	backend := newWorkflowBackend(t)
	requested := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
	for index := 0; index < 3; index++ {
		challenge := evaluationChallenge{
			Challenge:   fmt.Sprintf("pr-172-challenge-%d", index+1),
			Head:        "historical-head",
			PR:          14,
			RequestedAt: requested.Add(time.Duration(index) * time.Minute),
		}
		appendWorkflowEvaluationComment(t, backend, testEvaluationChallengeComment(t, challenge))
	}
	backend.head = "moved-head"
	oldEvidence, err := renderPREvidenceBlock(testWorkflowPREvidence("evaluated-head"))
	if err != nil {
		t.Fatalf("render old moved evidence: %v", err)
	}
	newEvidence, err := renderPREvidenceBlock(testWorkflowPREvidence(backend.head))
	if err != nil {
		t.Fatalf("render new moved evidence: %v", err)
	}
	backend.body = strings.Replace(backend.body, string(oldEvidence), string(newEvidence), 1)
	backend.body += "\nBody changed after the historical challenges.\n"

	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	err = application.runEvaluation([]string{"challenge", "14"})
	if err == nil || !strings.Contains(err.Error(), "outstanding trusted Examiner challenge") {
		t.Fatalf("replacement challenge error = %v, want outstanding-history rejection", err)
	}
	if !strings.Contains(err.Error(), "evaluation resolve 14 --challenge pr-172-challenge-1 --reason-file FILE") {
		t.Fatalf("replacement challenge error = %v, want recovery command", err)
	}
	if backend.commentPostCount != 0 {
		t.Fatalf("replacement challenge POST count = %d, want zero", backend.commentPostCount)
	}
	if got := len(backend.comments); got != 3 {
		t.Fatalf("historical challenge count = %d, want 3", got)
	}
}

func TestEvaluationReceiptClosesChallengeBeforeNextChallenge(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	first := requestTestChallenge(t, &application, &stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, first)
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record first evaluation: %v", err)
	}
	stdout.Reset()
	second := requestTestChallenge(t, &application, &stdout)
	if second.Challenge == first.Challenge {
		t.Fatalf("fresh challenge reused closed challenge ID %q", second.Challenge)
	}
	if backend.commentPostCount != 3 {
		t.Fatalf("challenge/receipt/fresh challenge POST count = %d, want 3", backend.commentPostCount)
	}
}

func TestEvaluationResolutionIsBoundIdempotentAndPermitsNextChallenge(t *testing.T) {
	backend := newWorkflowBackend(t)
	requested := time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration - time.Minute)
	challenge, challengeComment := resolutionTestChallenge(t, "expired-resolution", 14, "historical-head", requested)
	appendWorkflowEvaluationComment(t, backend, challengeComment)
	reasonPath := writeResolutionReason(t, "The original challenge expired without a trusted Examiner receipt.")

	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	args := []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", reasonPath}
	if err := application.runEvaluation(args); err != nil {
		t.Fatalf("record no-verdict resolution: %v", err)
	}
	if backend.commentPostCount != 1 {
		t.Fatalf("resolution POST count = %d, want 1", backend.commentPostCount)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.resolutions) != 1 || len(history.receipts) != 0 {
		t.Fatalf("resolution history = %#v, want one resolution and no receipts", history)
	}
	resolution := history.resolutions[0].resolution
	if resolution.Challenge != challenge.Challenge || resolution.PR != challenge.PR || resolution.Head != challenge.Head ||
		resolution.BodySHA256 != challenge.BodySHA256 || resolution.EvidenceSHA256 != challenge.EvidenceSHA256 ||
		resolution.Resolver != trustedActor {
		t.Fatalf("resolution snapshot = %#v, want exact challenge snapshot", resolution)
	}

	stdout.Reset()
	if err := application.runEvaluation(args); err != nil {
		t.Fatalf("idempotent resolution retry: %v", err)
	}
	if backend.commentPostCount != 1 {
		t.Fatalf("idempotent resolution POST count = %d, want 1", backend.commentPostCount)
	}
	conflictingReasonPath := writeResolutionReason(t, "A conflicting reason must fail closed.")
	conflictingArgs := []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", conflictingReasonPath}
	if err := application.runEvaluation(conflictingArgs); err == nil || !strings.Contains(err.Error(), "different no-verdict resolution reason") {
		t.Fatalf("conflicting resolution error = %v, want fail-closed conflict", err)
	}
	if backend.commentPostCount != 1 {
		t.Fatalf("conflicting resolution POST count = %d, want 1", backend.commentPostCount)
	}

	stdout.Reset()
	second := requestTestChallenge(t, &application, &stdout)
	if second.Challenge == challenge.Challenge {
		t.Fatalf("fresh challenge reused resolved challenge ID %q", second.Challenge)
	}
	if backend.commentPostCount != 2 {
		t.Fatalf("resolution/fresh challenge POST count = %d, want 2", backend.commentPostCount)
	}
}

func TestEvaluationResolutionAllowsStaleHeadBeforeExpiry(t *testing.T) {
	backend := newWorkflowBackend(t)
	requested := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	challenge, challengeComment := resolutionTestChallenge(t, "stale-before-expiry", 14, "historical-head", requested)
	appendWorkflowEvaluationComment(t, backend, challengeComment)
	advanceWorkflowPRHead(t, backend, "advanced-head")
	reasonPath := writeResolutionReason(t, "The challenged head was replaced before the Examiner deadline.")

	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	args := []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", reasonPath}
	if err := application.runEvaluation(args); err != nil {
		t.Fatalf("record stale no-verdict resolution: %v", err)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.resolutions) != 1 || backend.commentPostCount != 1 {
		t.Fatalf("stale resolution history/posts = %d/%d, want one resolution and one POST",
			len(history.resolutions), backend.commentPostCount)
	}
	resolution := history.resolutions[0].resolution
	if resolution.Schema != evaluationResolutionSchemaV2 || resolution.Head != challenge.Head ||
		resolution.ObservedHead != backend.head || resolution.Challenge != challenge.Challenge {
		t.Fatalf("stale resolution = %#v, want historical and observed heads preserved", resolution)
	}

	stdout.Reset()
	if err := application.runEvaluation(args); err != nil {
		t.Fatalf("retry stale no-verdict resolution: %v", err)
	}
	if backend.commentPostCount != 1 {
		t.Fatalf("stale resolution retry POST count = %d, want one", backend.commentPostCount)
	}

	stdout.Reset()
	fresh := requestTestChallenge(t, &application, &stdout)
	if fresh.Challenge == challenge.Challenge || fresh.Head != backend.head {
		t.Fatalf("fresh challenge after stale resolution = %#v, want new current-head challenge", fresh)
	}
}

func TestEvaluationResolutionRejectsMovedBackHeadBeforeExpiry(t *testing.T) {
	backend := newWorkflowBackend(t)
	requested := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	challenge, challengeComment := resolutionTestChallenge(t, "moved-back-before-expiry", 14, "historical-head", requested)
	appendWorkflowEvaluationComment(t, backend, challengeComment)
	advanceWorkflowPRHead(t, backend, "advanced-head")
	advanceWorkflowPRHead(t, backend, challenge.Head)
	reasonPath := writeResolutionReason(t, "The head moved back before the Examiner deadline.")

	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	err := application.runEvaluation([]string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", reasonPath})
	if err == nil || !strings.Contains(err.Error(), "has not expired") {
		t.Fatalf("moved-back resolution error = %v, want same-head pre-expiry rejection", err)
	}
	if backend.commentPostCount != 0 {
		t.Fatalf("moved-back resolution POST count = %d, want zero", backend.commentPostCount)
	}
}

type evaluationResolutionInvalidStateCase struct {
	name  string
	setup func(*testing.T, *workflowBackend) []string
}

func TestEvaluationResolutionRejectsMalformedStateBeforePOST(t *testing.T) {
	runEvaluationResolutionInvalidStateCases(t, []evaluationResolutionInvalidStateCase{
		{
			name: "malformed challenge",
			setup: func(_ *testing.T, backend *workflowBackend) []string {
				now := time.Now().UTC().Truncate(time.Second)
				comment := issueCommentAPI{Body: "<!-- " + evaluationChallengeMarker + "not-json -->", CreatedAt: now}
				comment.User.Login = trustedActor
				backend.comments = []issueCommentAPI{comment}
				return []string{"challenge", "14"}
			},
		},
		{
			name: "untrusted challenge",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				requested := time.Now().UTC().Truncate(time.Second)
				_, comment := resolutionTestChallenge(t, "untrusted-challenge", 14, backend.head, requested)
				comment.Author.Login = owner
				backend.comments = []issueCommentAPI{workflowCommentAPI(comment)}
				return []string{"challenge", "14"}
			},
		},
		{
			name: "malformed resolution",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				challenge, comment := resolutionTestChallenge(t, "malformed-resolution", 14, backend.head,
					time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
				backend.comments = []issueCommentAPI{workflowCommentAPI(comment)}
				resolutionComment := issueCommentAPI{
					Body:      "<!-- " + evaluationResolutionMarker + "not-json -->\n" + evaluationResolutionHeading + "reason\n",
					CreatedAt: challenge.RequestedAt.Add(evaluationChallengeDuration),
				}
				resolutionComment.User.Login = trustedActor
				backend.comments = append(backend.comments, resolutionComment)
				return []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", writeResolutionReason(t, "reason")}
			},
		},
		{
			name: "untrusted resolution",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				challenge, challengeComment := resolutionTestChallenge(t, "untrusted-resolution", 14, backend.head,
					time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
				backend.comments = []issueCommentAPI{workflowCommentAPI(challengeComment)}
				resolutionComment := resolutionTestComment(t, challenge, challenge.RequestedAt.Add(evaluationChallengeDuration), "reason")
				resolutionAPI := workflowCommentAPI(resolutionComment)
				resolutionAPI.User.Login = owner
				backend.comments = append(backend.comments, resolutionAPI)
				return []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", writeResolutionReason(t, "reason")}
			},
		},
	})
}

func TestEvaluationResolutionRejectsAmbiguousStateBeforePOST(t *testing.T) {
	runEvaluationResolutionInvalidStateCases(t, []evaluationResolutionInvalidStateCase{
		{
			name: "mismatched resolution snapshot",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				challenge, challengeComment := resolutionTestChallenge(t, "mismatched-resolution", 14, backend.head,
					time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
				backend.comments = []issueCommentAPI{workflowCommentAPI(challengeComment)}
				mismatched := challenge
				mismatched.Head = "different-historical-head"
				resolutionComment := resolutionTestComment(t, mismatched, mismatched.RequestedAt.Add(evaluationChallengeDuration), "reason")
				backend.comments = append(backend.comments, workflowCommentAPI(resolutionComment))
				return []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", writeResolutionReason(t, "reason")}
			},
		},
		{
			name: "pre-expiry resolution",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				challenge, challengeComment := resolutionTestChallenge(t, "pre-expiry", 14, backend.head,
					time.Now().UTC().Truncate(time.Second).Add(-time.Hour))
				backend.comments = []issueCommentAPI{workflowCommentAPI(challengeComment)}
				return []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", writeResolutionReason(t, "reason")}
			},
		},
		{
			name: "duplicate resolution closure",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				challenge, challengeComment := resolutionTestChallenge(t, "duplicate-resolution", 14, backend.head,
					time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
				backend.comments = []issueCommentAPI{workflowCommentAPI(challengeComment)}
				first := resolutionTestComment(t, challenge, challenge.RequestedAt.Add(evaluationChallengeDuration), "reason")
				second := resolutionTestComment(t, challenge, challenge.RequestedAt.Add(evaluationChallengeDuration+time.Minute), "reason")
				backend.comments = append(backend.comments, workflowCommentAPI(first), workflowCommentAPI(second))
				return []string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", writeResolutionReason(t, "reason")}
			},
		},
		{
			name: "duplicate challenge identity",
			setup: func(t *testing.T, backend *workflowBackend) []string {
				requested := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Minute)
				first, firstComment := resolutionTestChallenge(t, "duplicate-challenge", 14, backend.head, requested)
				second := first
				second.RequestedAt = requested.Add(time.Minute)
				secondComment := testEvaluationChallengeComment(t, second)
				backend.comments = []issueCommentAPI{workflowCommentAPI(firstComment), workflowCommentAPI(secondComment)}
				return []string{"challenge", "14"}
			},
		},
	})
}

func runEvaluationResolutionInvalidStateCases(t *testing.T, tests []evaluationResolutionInvalidStateCase) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			args := test.setup(t, backend)
			var stdout bytes.Buffer
			application := newResolutionWorkflowApplication(backend, &stdout)
			err := application.runEvaluation(args)
			if err == nil {
				t.Fatal("invalid evaluation state was accepted")
			}
			if backend.commentPostCount != 0 {
				t.Fatalf("invalid evaluation state POST count = %d, want zero", backend.commentPostCount)
			}
		})
	}
}

func TestEvaluationResolutionPostVerificationRejectsUntrustedAuthor(t *testing.T) {
	backend := newWorkflowBackend(t)
	challenge, comment := resolutionTestChallenge(t, "post-verification", 14, backend.head,
		time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
	backend.comments = []issueCommentAPI{workflowCommentAPI(comment)}
	backend.postCommentAuthor = owner

	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	err := application.runEvaluation([]string{"resolve", "14", "--challenge", challenge.Challenge,
		"--reason-file", writeResolutionReason(t, "remote author is not trusted")})
	if err == nil || !strings.Contains(err.Error(), "untrusted evaluation evidence") {
		t.Fatalf("untrusted post verification error = %v, want authenticated-history failure", err)
	}
	if backend.commentPostCount != 1 {
		t.Fatalf("post verification POST count = %d, want one attempted POST", backend.commentPostCount)
	}
}

func TestEvaluationResolutionAmbiguousPostIsRetryable(t *testing.T) {
	backend := newWorkflowBackend(t)
	challenge, comment := resolutionTestChallenge(t, "ambiguous-resolution", 14, backend.head,
		time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
	backend.comments = []issueCommentAPI{workflowCommentAPI(comment)}
	backend.postCommentResponseMode = "transport"
	sentinel := errors.New("simulated lost no-verdict resolution response")
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		output, err := backend.execute(dir, input, name, args...)
		if name == "gh" && strings.Join(args, " ") == "api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" && err != nil {
			return output, sentinel
		}
		return output, err
	}
	reasonFile := writeResolutionReason(t, "The no-verdict resolution response was ambiguous.")
	err := application.runEvaluation([]string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", reasonFile})
	if err == nil {
		t.Fatal("ambiguous no-verdict resolution succeeded")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("ambiguous no-verdict resolution disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("ambiguous no-verdict resolution error = %v, want sentinel cause", err)
	}
	if got, want := backend.commentPostCount, 1; got != want {
		t.Fatalf("ambiguous no-verdict resolution POST count = %d, want %d", got, want)
	}
	if history := workflowEvaluationHistory(t, backend, 14); len(history.resolutions) != 1 {
		t.Fatalf("ambiguous no-verdict resolution history = %#v, want successful mutation preserved", history)
	}
}

func TestEvaluationResolutionVerificationReadFailureIsRetryable(t *testing.T) {
	backend := newWorkflowBackend(t)
	challenge, comment := resolutionTestChallenge(t, "verification-resolution", 14, backend.head,
		time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration-time.Minute))
	backend.comments = []issueCommentAPI{workflowCommentAPI(comment)}
	sentinel := errors.New("simulated no-verdict resolution verification GET failure")
	failCommentsRead := false
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if failCommentsRead && command == "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
			failCommentsRead = false
			return "", sentinel
		}
		output, err := backend.execute(dir, input, name, args...)
		if err == nil && command == "gh api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" {
			failCommentsRead = true
		}
		return output, err
	}
	reasonFile := writeResolutionReason(t, "The no-verdict resolution verification read failed.")
	err := application.runEvaluation([]string{"resolve", "14", "--challenge", challenge.Challenge, "--reason-file", reasonFile})
	if err == nil {
		t.Fatal("no-verdict resolution succeeded after injected verification read failure")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("no-verdict resolution verification disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("no-verdict resolution verification error = %v, want sentinel cause", err)
	}
	if got, want := backend.commentPostCount, 1; got != want {
		t.Fatalf("no-verdict resolution POST count = %d, want %d", got, want)
	}
}

func TestEvaluationExpiredReceiptRejectedBeforePOST(t *testing.T) {
	backend := newWorkflowBackend(t)
	view := pullRequestView{
		BaseRefName: "main", BaseRefOID: "base-sha", Body: backend.body,
		HeadRefName: backend.branch, HeadRefOID: backend.head,
	}
	parsedEvidence, err := validatePREvidenceForView(view)
	if err != nil {
		t.Fatalf("parse current workflow evidence: %v", err)
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	requested := time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration - time.Minute)
	challenge := evaluationChallenge{
		Challenge:      "expired-receipt",
		Head:           backend.head,
		PR:             14,
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		RequestedAt:    requested,
	}
	appendWorkflowEvaluationComment(t, backend, testEvaluationChallengeComment(t, challenge))
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)

	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	err = application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expired receipt error = %v, want stale challenge rejection", err)
	}
	if backend.commentPostCount != 0 {
		t.Fatalf("expired receipt POST count = %d, want zero", backend.commentPostCount)
	}
}

func TestEvaluationStatusKeepsResolutionSeparateFromVerdicts(t *testing.T) {
	history, resolutionChallenge, resolutionComments := evaluationResolutionSeparationFixture(t)
	projection, err := evaluationStatusForPR(175, pullRequestView{HeadRefOID: "status-head"}, history)
	if err != nil {
		t.Fatalf("project mixed resolution status: %v", err)
	}
	if len(projection.recordedRounds) != 1 || len(projection.resolutions) != 1 ||
		!projection.challenges[1].resolvedByResolution {
		t.Fatalf("mixed status projection = %#v, want one round and one resolution", projection)
	}
	status := renderEvaluationStatus(175, projection)
	for _, want := range []string{
		"Recorded rounds: 1",
		"No-verdict resolutions: 1",
		"Recorded pass verdicts: 0",
		"Recorded fail verdicts: 1",
		"resolved by no-verdict resolution (reason: \"Examiner context expired before a receipt was returned.\")",
	} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q:\n%s", want, status)
		}
	}
	resolutionOnlyView := pullRequestView{Comments: resolutionComments, HeadRefOID: resolutionChallenge.Head}
	passes, err := latestEvaluationPasses(resolutionOnlyView, 175)
	if err != nil || passes {
		t.Fatalf("no-verdict resolution authorized evaluation pass: passes=%t err=%v", passes, err)
	}
	_, mergeProofErr := latestPassingEvaluationReceipt(resolutionOnlyView, 175)
	if mergeProofErr == nil {
		t.Fatal("no-verdict resolution authorized merge proof")
	}
}

func TestEvaluationHistoryKeepsResolutionSeparateFromVerdicts(t *testing.T) {
	history, resolutionChallenge, _ := evaluationResolutionSeparationFixture(t)
	packet, err := historyEvaluationPacketForPR(pullRequestSummary{
		Number: 175, MergedAt: resolutionChallenge.RequestedAt.Add(4 * time.Hour),
	}, history)
	if err != nil {
		t.Fatalf("project history packet: %v", err)
	}
	metrics := historyEvaluationMetricsForPacket(packet)
	if metrics.totalRounds != 1 || metrics.failedRounds != 1 || metrics.noVerdictResolutions != 1 ||
		metrics.evaluatedPackets != 1 || metrics.finalPasses != 0 {
		t.Fatalf("mixed history metrics = %#v, want one fail and one separate resolution", metrics)
	}
	var report bytes.Buffer
	if err := renderEvaluationHistory(&report, []historyEvaluationPacket{packet}, 1); err != nil {
		t.Fatalf("render mixed history: %v", err)
	}
	for _, want := range []string{"round 1 fail", "no-verdict resolution challenge resolved-without-verdict", "No-verdict resolutions: 1"} {
		if !strings.Contains(report.String(), want) {
			t.Fatalf("history missing %q:\n%s", want, report.String())
		}
	}
}

func evaluationResolutionSeparationFixture(t *testing.T) (evaluationHistory, evaluationChallenge, []pullRequestComment) {
	t.Helper()
	base := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
	failureChallenge := historyTestChallenge(t, "failure-round", 175, "status-head", base, trustedActor)
	failureReceipt := historyTestEvaluation(t, failureChallenge, "failure-run", 1, "fail", 1,
		historyTestCurrentBase64, trustedActor, base.Add(time.Minute))
	resolutionChallenge, resolutionChallengeComment := resolutionTestChallenge(t, "resolved-without-verdict", 175,
		"expired-head", base.Add(-evaluationChallengeDuration-time.Minute))
	resolutionComment := resolutionTestComment(t, resolutionChallenge, base.Add(time.Minute), "Examiner context expired before a receipt was returned.")
	comments := []pullRequestComment{
		failureChallenge, failureReceipt, resolutionChallengeComment, resolutionComment,
	}
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		t.Fatalf("parse mixed resolution history: %v", err)
	}
	if historyValidationErr := validateEvaluationHistory(history); historyValidationErr != nil {
		t.Fatalf("validate mixed resolution history: %v", historyValidationErr)
	}
	return history, resolutionChallenge, []pullRequestComment{resolutionChallengeComment, resolutionComment}
}

func TestEvaluationResolutionReservedTextAndParserDoNotPanic(t *testing.T) {
	reserved := []string{
		evaluationResolutionMarker,
		evaluationResolutionHeading,
		evaluationMarker,
		evaluationReceiptHeading,
	}
	for _, value := range reserved {
		if _, err := validateEvaluationResolutionReason("reason " + value); err == nil {
			t.Fatalf("reserved resolution text %q was accepted", value)
		}
	}
	for _, body := range []string{
		"",
		"<!-- " + evaluationResolutionMarker + "not-json -->",
		evaluationResolutionHeading,
		"<!-- " + evaluationResolutionMarker + strings.Repeat("x", 10000) + " -->",
	} {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("resolution parser panicked for %q: %v", body, recovered)
				}
			}()
			_, _ = parseEvaluationResolution(body)
			if _, err := parseEvaluationHistory([]pullRequestComment{{Body: body, CreatedAt: time.Now().UTC()}}); err != nil {
				return
			}
		}()
	}
}

func TestEvaluationResolutionMarkersPreserveV1AndValidateV2(t *testing.T) {
	requested := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
	challenge, _ := resolutionTestChallenge(t, "versioned-resolution", 14, "historical-head", requested)
	v1Comment := resolutionTestComment(t, challenge, requested.Add(evaluationChallengeDuration), "expired")
	if strings.Contains(v1Comment.Body, "observedHead") {
		t.Fatalf("v1 resolution gained an observed-head field: %q", v1Comment.Body)
	}
	v1, ok := parseEvaluationResolution(v1Comment.Body)
	if !ok || v1.Schema != evaluationResolutionSchema || v1.ObservedHead != "" {
		t.Fatalf("parsed v1 resolution = %#v, want legacy schema without observed head", v1)
	}

	v2Resolution := v1
	v2Resolution.ObservedHead = "current-head"
	v2Resolution.Schema = evaluationResolutionSchemaV2
	v2Marker, err := json.Marshal(v2Resolution)
	if err != nil {
		t.Fatalf("encode v2 resolution: %v", err)
	}
	v2Comment := statusComment(evaluationResolutionCommentV2(v2Marker, "expired"), trustedActor,
		v2Resolution.ResolvedAt)
	v2, ok := parseEvaluationResolution(v2Comment.Body)
	if !ok || v2.Schema != evaluationResolutionSchemaV2 || v2.ObservedHead != "current-head" ||
		!evaluationResolutionCommentIsValid(v2Comment) {
		t.Fatalf("parsed v2 resolution = %#v, want valid observed-head record", v2)
	}

	v1WithObserved := v1
	v1WithObserved.ObservedHead = "current-head"
	v1Marker, err := json.Marshal(v1WithObserved)
	if err != nil {
		t.Fatalf("encode invalid v1 resolution: %v", err)
	}
	if _, ok := parseEvaluationResolution(evaluationResolutionComment(v1Marker, "expired")); ok {
		t.Fatal("v1 resolution with an observed head was accepted")
	}
}

func FuzzEvaluationResolutionParserDoesNotPanic(f *testing.F) {
	f.Add("reason")
	f.Add(evaluationResolutionHeading)
	f.Add("<!-- " + evaluationResolutionMarker + "not-json -->")
	f.Fuzz(func(t *testing.T, input string) {
		t.Helper()
		body := "<!-- " + evaluationResolutionMarker + input + " -->\n" + input
		_, _ = parseEvaluationResolution(body)
		if _, err := parseEvaluationHistory([]pullRequestComment{{Body: body, CreatedAt: time.Now().UTC()}}); err != nil {
			return
		}
		if _, err := validateEvaluationResolutionReason(input); err != nil {
			return
		}
	})
}

func newResolutionWorkflowApplication(backend *workflowBackend, stdout *bytes.Buffer) app {
	return app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         stdout,
		stderr:                         new(bytes.Buffer),
	}
}

func resolutionTestChallenge(t *testing.T, id string, number int, head string, requested time.Time) (evaluationChallenge, pullRequestComment) {
	t.Helper()
	challenge := evaluationChallenge{
		Challenge:      id,
		Head:           head,
		PR:             number,
		BodySHA256:     strings.Repeat("a", 64),
		EvidenceSHA256: strings.Repeat("b", 64),
		RequestedAt:    requested,
	}
	return challenge, testEvaluationChallengeComment(t, challenge)
}

func resolutionTestComment(t *testing.T, challenge evaluationChallenge, resolvedAt time.Time, reason string) pullRequestComment {
	t.Helper()
	resolution := evaluationResolution{
		BodySHA256:     challenge.BodySHA256,
		Challenge:      challenge.Challenge,
		EvidenceSHA256: challenge.EvidenceSHA256,
		Head:           challenge.Head,
		PR:             challenge.PR,
		Reason:         reason,
		ResolvedAt:     resolvedAt,
		Resolver:       trustedActor,
		Schema:         evaluationResolutionSchema,
	}
	marker, err := json.Marshal(resolution)
	if err != nil {
		t.Fatalf("encode resolution fixture: %v", err)
	}
	return statusComment(evaluationResolutionComment(marker, reason), trustedActor, resolvedAt)
}

func workflowCommentAPI(comment pullRequestComment) issueCommentAPI {
	apiComment := issueCommentAPI{ID: comment.ID, Body: comment.Body, CreatedAt: comment.CreatedAt}
	apiComment.User.Login = comment.Author.Login
	return apiComment
}

func appendWorkflowEvaluationComment(t *testing.T, backend *workflowBackend, comment pullRequestComment) {
	t.Helper()
	backend.comments = append(backend.comments, workflowCommentAPI(comment))
}

func writeResolutionReason(t *testing.T, reason string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(path, []byte(reason+"\n"), 0o600); err != nil {
		t.Fatalf("write resolution reason: %v", err)
	}
	return path
}

//nolint:unparam // All workflow fixtures exercise the fixed PR #14 backend.
func workflowEvaluationHistory(t *testing.T, backend *workflowBackend, number int) evaluationHistory {
	t.Helper()
	comments := pullRequestCommentsFromAPI(t, backend.comments)
	history, err := readEvaluationMutationHistory(number, comments)
	if err != nil {
		t.Fatalf("read workflow evaluation history: %v", err)
	}
	return history
}
