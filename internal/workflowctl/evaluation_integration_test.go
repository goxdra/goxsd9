package workflowctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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

const testExaminerRunID = "examiner-command-flow"

func acceptDevelopmentSignalsForCommandFlow(string, developmentSignalsReport) error {
	return nil
}

func TestEvaluationToMergeCommandFlow(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         &stdout,
		stderr:                         &stderr,
	}

	challenge := requestTestChallenge(t, &application, &stdout)
	attestationJSON, attestationFile := writeTestAttestation(t, backend.head, challenge)

	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record evaluation: %v", err)
	}
	checkRecordedAttestation(t, backend.comments, attestationJSON)
	rejectTamperedReceiptReuse(t, &application, backend, attestationFile)
	rejectLaterTamperedReceipt(t, &application, backend)
	rejectInvalidPullRequestTitle(t, &application, backend)
	rejectInvalidWorkCommitTitle(t, &application, backend)

	commentCount := len(backend.comments)
	backend.comments = append(backend.comments, reusedRunComments(t, backend.head, testExaminerRunID)...)
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted a reused Examiner run ID")
	}
	backend.comments = backend.comments[:commentCount]

	backend.head = "unevaluated-head"
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted an evaluation for an earlier head")
	}
	if backend.merged {
		t.Fatal("unevaluated head reached the merge endpoint")
	}
	backend.head = "evaluated-head"

	stdout.Reset()
	backend.removeSummaryOnNextCommand = true
	if err := application.runPR(backend.finishArgs()); err != nil {
		t.Fatalf("finish evaluated PR: %v", err)
	}
	checkMergeResult(t, backend)
}

func TestEvaluationChallengeConvergesConcurrentEquivalentComment(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.duplicateChallengeOnNextPost = true
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	canonical := requestTestChallenge(t, &application, &stdout)
	if got, want := len(backend.comments), 3; got != want {
		t.Fatalf("converged challenge history comments = %d, want %d", got, want)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.challenges) != 2 || len(history.closures) != 1 {
		t.Fatalf("converged challenge history = %#v, want two challenges and one closure", history)
	}
	outstanding, err := outstandingEvaluationChallenges(history)
	if err != nil {
		t.Fatalf("logical outstanding challenges: %v", err)
	}
	if len(outstanding) != 1 || outstanding[0].challenge.Challenge != canonical.Challenge {
		t.Fatalf("logical outstanding challenges = %#v, want canonical challenge only", outstanding)
	}

	stdout.Reset()
	if statusErr := application.runEvaluation([]string{"status", "14"}); statusErr != nil {
		t.Fatalf("status after challenge convergence: %v", statusErr)
	}
	status := stdout.String()
	for _, expected := range []string{"Trusted challenges: 2", "Outstanding challenges: 1", "Recorded rounds: 0", "No-verdict resolutions: 0", "resolved by controller closure"} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status = %q, want %q", status, expected)
		}
	}

	_, attestationFile := writeTestAttestation(t, backend.head, canonical)
	stdout.Reset()
	if recordErr := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); recordErr != nil {
		t.Fatalf("record canonical attestation after challenge convergence: %v", recordErr)
	}
	history = workflowEvaluationHistory(t, backend, 14)
	if len(history.receipts) != 1 || len(history.closures) != 1 {
		t.Fatalf("recorded converged history = %#v, want one receipt and one closure", history)
	}
	outstanding, err = outstandingEvaluationChallenges(history)
	if err != nil {
		t.Fatalf("logical outstanding challenges after receipt: %v", err)
	}
	if len(outstanding) != 0 {
		t.Fatalf("logical outstanding challenges after receipt = %#v, want none", outstanding)
	}
}

func TestEvaluationChallengeReturnsEarlierCanonicalMarker(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.duplicateChallengeBeforeNextPost = true
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	returned := requestTestChallenge(t, &application, &stdout)
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.challenges) != 2 || len(history.closures) != 1 {
		t.Fatalf("earlier-canonical history = challenges %d closures %d; want 2 and 1",
			len(history.challenges), len(history.closures))
	}
	if returned.Challenge != "concurrent-equivalent-challenge" {
		t.Fatalf("returned challenge = %q, want earlier canonical challenge", returned.Challenge)
	}
	if returned.Challenge == "" || returned.Challenge == "concurrent-equivalent-posted-challenge" {
		t.Fatalf("returned challenge marker was not authenticated canonical output: %#v", returned)
	}
}

func TestEvaluationChallengeInitialPostTransportLossRetriesWithoutRepost(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.postCommentResponseMode = "transport"
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	err := application.runEvaluation([]string{"challenge", "14"})
	if err == nil || !strings.Contains(err.Error(), "do not repost blindly") {
		t.Fatalf("initial challenge transport error = %v, want retry guidance", err)
	}
	if got, want := backend.commentPostCount, 1; got != want {
		t.Fatalf("initial challenge POST count = %d, want %d", got, want)
	}
	if got, want := len(backend.comments), 1; got != want {
		t.Fatalf("initial challenge history comments = %d, want %d", got, want)
	}

	backend.postCommentResponseMode = ""
	stdout.Reset()
	if err := application.runEvaluation([]string{"challenge", "14"}); err != nil {
		t.Fatalf("retry initial challenge transport loss: %v", err)
	}
	if got, want := backend.commentPostCount, 1; got != want {
		t.Fatalf("retry reposted initial challenge = %d POSTs, want %d", got, want)
	}
	if got, want := len(backend.comments), 1; got != want {
		t.Fatalf("retry changed initial challenge history comments = %d, want %d", got, want)
	}
	var returned evaluationChallenge
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &returned); err != nil {
		t.Fatalf("decode retried challenge output: %v", err)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.challenges) != 1 {
		t.Fatalf("retried challenge history = %d challenges, want 1", len(history.challenges))
	}
	if returned.Challenge != history.challenges[0].challenge.Challenge || returned.BodySHA256 == "" || returned.EvidenceSHA256 == "" {
		t.Fatalf("retried challenge output = %#v, want authenticated digest-bound history marker", returned)
	}
}

func TestEvaluationChallengeVerificationReadFailureIsRetryable(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	sentinel := errors.New("simulated challenge verification GET failure")
	failNextPRRead := false
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if failNextPRRead && command == "gh api repos/goxdra/goxsd9/pulls/14" {
			failNextPRRead = false
			return "", sentinel
		}
		output, err := backend.execute(dir, input, name, args...)
		if err == nil && command == "gh api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" {
			failNextPRRead = true
		}
		return output, err
	}

	err := application.runEvaluation([]string{"challenge", "14"})
	if err == nil {
		t.Fatal("challenge verification succeeded after injected GET failure")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("challenge verification disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("challenge verification error = %v, want sentinel cause", err)
	}
	if got, want := backend.commentPostCount, 1; got != want {
		t.Fatalf("challenge POST count = %d, want %d", got, want)
	}
}

func TestEvaluationChallengeConvergenceClosesStaleDuplicateAfterPRAdvance(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	canonical := requestTestChallenge(t, &application, &stdout)
	expireWorkflowChallenge(t, backend, canonical.Challenge)
	appendFinalEquivalentChallenge(t, backend)
	advanceWorkflowPRHead(t, backend, "advanced-head")
	reasonFile := writeResolutionReason(t, "The stale challenge expired before the PR advanced.")
	stdout.Reset()
	if err := application.runEvaluation([]string{"resolve", "14", "--challenge", canonical.Challenge, "--reason-file", reasonFile}); err != nil {
		t.Fatalf("resolve stale challenge after PR advance: %v", err)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.challenges) != 2 || len(history.closures) != 1 || len(history.resolutions) != 1 {
		t.Fatalf("stale challenge history = challenges %d closures %d resolutions %d, want 2, 1, 1",
			len(history.challenges), len(history.closures), len(history.resolutions))
	}
	if got, want := backend.commentPostCount, 3; got != want {
		t.Fatalf("stale challenge resolution POST count = %d, want %d", got, want)
	}

	stdout.Reset()
	if err := application.runEvaluation([]string{"status", "14"}); err != nil {
		t.Fatalf("status after stale challenge resolution: %v", err)
	}
	status := stdout.String()
	for _, expected := range []string{"Trusted challenges: 2", "Outstanding challenges: 0", "No-verdict resolutions: 1", "resolved by no-verdict resolution"} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status after stale challenge resolution = %q, want %q", status, expected)
		}
	}

	stdout.Reset()
	fresh := requestTestChallenge(t, &application, &stdout)
	if fresh.Challenge == canonical.Challenge || fresh.Head != backend.head {
		t.Fatalf("fresh challenge = %#v, want new identity on advanced head", fresh)
	}
	if got, want := backend.commentPostCount, 4; got != want {
		t.Fatalf("stale challenge replacement POST count = %d, want %d", got, want)
	}
}

func expireWorkflowChallenge(t *testing.T, backend *workflowBackend, challengeID string) {
	t.Helper()
	for index := range backend.comments {
		challenge, ok := parseEvaluationChallenge(backend.comments[index].Body)
		if !ok || challenge.Challenge != challengeID {
			continue
		}
		challenge.RequestedAt = time.Now().UTC().Truncate(time.Second).Add(-evaluationChallengeDuration - time.Minute)
		marker, err := json.Marshal(challenge)
		if err != nil {
			t.Fatalf("encode expired challenge: %v", err)
		}
		backend.comments[index].Body = fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n",
			evaluationChallengeMarker, marker, challenge.Head)
		backend.comments[index].CreatedAt = challenge.RequestedAt
		return
	}
	t.Fatalf("challenge %q was not found to expire", challengeID)
}

func advanceWorkflowPRHead(t *testing.T, backend *workflowBackend, head string) {
	t.Helper()
	parsed, err := parsePREvidenceBody(backend.body)
	if err != nil {
		t.Fatalf("parse PR evidence before head advance: %v", err)
	}
	parsed.evidence = testWorkflowPREvidence(head)
	block, err := renderPREvidenceBlock(parsed.evidence)
	if err != nil {
		t.Fatalf("render PR evidence after head advance: %v", err)
	}
	backend.body, err = replacePREvidenceBlock(backend.body, block)
	if err != nil {
		t.Fatalf("replace PR evidence after head advance: %v", err)
	}
	backend.head = head
}

//nolint:gocognit // Keep expected and canonical final-expiry cases together.
func TestEvaluationChallengeFinalReadRejectsExpiredChallenge(t *testing.T) {
	tests := []struct {
		name             string
		canonicalExpired bool
	}{
		{name: "expected challenge", canonicalExpired: false},
		{name: "canonical challenge", canonicalExpired: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			var stdout bytes.Buffer
			application := newResolutionWorkflowApplication(backend, &stdout)
			view, err := application.readPullRequest(backend.root, 14)
			if err != nil {
				t.Fatalf("read PR for expired challenge: %v", err)
			}
			parsedEvidence, err := validatePREvidenceForView(view)
			if err != nil {
				t.Fatalf("validate PR evidence for expired challenge: %v", err)
			}
			bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
			now := time.Now().UTC().Truncate(time.Second)
			expiredAt := now.Add(-evaluationChallengeDuration - time.Minute)
			canonical := evaluationChallenge{
				BodySHA256: bodySHA256, Challenge: "expired-canonical", EvidenceSHA256: evidenceSHA256,
				Head: backend.head, PR: 14, Repository: repositoryKey, RequestedAt: expiredAt,
			}
			challenge := canonical
			if !test.canonicalExpired {
				challenge.Challenge = "expired-expected"
			}
			if test.canonicalExpired {
				challenge = canonical
				challenge.Challenge = "fresh-expected"
				challenge.RequestedAt = now.Add(-time.Minute)
			}
			if test.canonicalExpired {
				appendWorkflowEvaluationComment(t, backend, testEvaluationChallengeComment(t, canonical))
			}
			appendWorkflowEvaluationComment(t, backend, testEvaluationChallengeComment(t, challenge))
			view, err = application.readPullRequest(backend.root, 14)
			if err != nil {
				t.Fatalf("reread PR for expired challenge: %v", err)
			}
			history, err := readEvaluationMutationHistoryForConvergence(14, view.Comments)
			if err != nil {
				t.Fatalf("read expired challenge history: %v", err)
			}
			if err := application.completeEvaluationChallenge(backend.root, 14, challenge, view, history); err == nil ||
				!strings.Contains(err.Error(), "expired before output") {
				t.Fatalf("expired challenge output error = %v, want final expiry rejection", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("expired challenge emitted marker %q", stdout.String())
			}
		})
	}
}

//nolint:gocognit // Keep final-read mutation variants and zero-output assertions together.
func TestEvaluationChallengeFinalReadRejectsCurrentStateDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *workflowBackend)
		want   string
	}{
		{name: "head", mutate: func(_ *testing.T, backend *workflowBackend) {
			backend.head = "drifted-head"
		}, want: "head changed"},
		{name: "body", mutate: func(_ *testing.T, backend *workflowBackend) {
			backend.body += "\nFinal body drift.\n"
		}, want: "body or evidence changed"},
		{name: "evidence", mutate: mutateFinalEvaluationEvidence, want: "body or evidence changed"},
		{name: "additional outstanding challenge", mutate: appendFinalOutstandingChallenge, want: "sole current logical outstanding challenge"},
		{name: "unclosed equivalent challenge", mutate: appendFinalEquivalentChallenge, want: "unclosed equivalent challenge"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			var stdout bytes.Buffer
			application := newResolutionWorkflowApplication(backend, &stdout)
			prReads := 0
			application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
				if name == "gh" && strings.Join(args, " ") == "api repos/goxdra/goxsd9/pulls/14" {
					prReads++
					if prReads == 3 {
						test.mutate(t, backend)
					}
				}
				return backend.execute(dir, input, name, args...)
			}

			if err := application.runEvaluation([]string{"challenge", "14"}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("final %s drift error = %v, want %q", test.name, err, test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("final %s drift emitted marker %q", test.name, stdout.String())
			}
			if prReads != 3 {
				t.Fatalf("PR reads = %d, want final-read mutation at read 3", prReads)
			}
			if backend.commentPostCount != 1 {
				t.Fatalf("final %s drift comment POST count = %d, want initial challenge only", test.name, backend.commentPostCount)
			}
		})
	}
}

func mutateFinalEvaluationEvidence(t *testing.T, backend *workflowBackend) {
	t.Helper()
	parsed, err := parsePREvidenceBody(backend.body)
	if err != nil {
		t.Fatalf("parse final evidence: %v", err)
	}
	parsed.evidence.Curator.Summary = "No managed-document change; final evidence drift."
	block, err := renderPREvidenceBlock(parsed.evidence)
	if err != nil {
		t.Fatalf("render final evidence: %v", err)
	}
	backend.body, err = replacePREvidenceBlock(backend.body, block)
	if err != nil {
		t.Fatalf("replace final evidence: %v", err)
	}
}

func appendFinalOutstandingChallenge(t *testing.T, backend *workflowBackend) {
	t.Helper()
	challenge := evaluationChallenge{
		Challenge:      "late-current-challenge",
		Head:           backend.head,
		Repository:     repositoryKey,
		PR:             14,
		BodySHA256:     strings.Repeat("a", 64),
		EvidenceSHA256: strings.Repeat("b", 64),
		RequestedAt:    time.Now().UTC().Truncate(time.Second),
	}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode final outstanding challenge: %v", err)
	}
	comment := issueCommentAPI{
		ID:        int64(len(backend.comments) + 1),
		Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, challenge.Head),
		CreatedAt: challenge.RequestedAt,
	}
	comment.User.Login = trustedActor
	backend.comments = append(backend.comments, comment)
}

func appendFinalEquivalentChallenge(t *testing.T, backend *workflowBackend) {
	t.Helper()
	challenge, ok := parseEvaluationChallenge(backend.comments[0].Body)
	if !ok {
		t.Fatal("parse initial challenge for final equivalent challenge")
	}
	challenge.Challenge = "late-equivalent-challenge"
	challenge.RequestedAt = challenge.RequestedAt.Add(time.Second)
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode final equivalent challenge: %v", err)
	}
	comment := issueCommentAPI{
		ID:        int64(len(backend.comments) + 1),
		Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, challenge.Head),
		CreatedAt: challenge.RequestedAt,
	}
	comment.User.Login = trustedActor
	backend.comments = append(backend.comments, comment)
}

func TestEvaluationChallengeAndReceiptConvergencePhasesProgress(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	canonical := requestTestChallenge(t, &application, &stdout)
	duplicate := canonical
	duplicate.Challenge = "concurrent-equivalent-challenge"
	duplicate.RequestedAt = canonical.RequestedAt.Add(time.Second)
	marker, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatalf("encode duplicate challenge: %v", err)
	}
	duplicateComment := issueCommentAPI{
		ID:        int64(len(backend.comments) + 1),
		Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, duplicate.Head),
		CreatedAt: duplicate.RequestedAt,
	}
	duplicateComment.User.Login = trustedActor
	backend.comments = append(backend.comments, duplicateComment)
	backend.duplicateReceiptPost = true
	_, attestationFile := writeTestAttestation(t, backend.head, canonical)

	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("converge challenge and receipt phases: %v", err)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.challenges) != 2 || len(history.closures) != 1 || len(history.receipts) != 2 ||
		len(history.convergences) != 1 {
		t.Fatalf("two-phase convergence history = challenges %d closures %d receipts %d convergences %d; want 2, 1, 2, 1",
			len(history.challenges), len(history.closures), len(history.receipts), len(history.convergences))
	}
}

func TestEvaluationChallengeClosurePartialRetryProgresses(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	canonical := requestTestChallenge(t, &application, &stdout)
	duplicate := canonical
	duplicate.Challenge = "concurrent-equivalent-challenge"
	duplicate.RequestedAt = canonical.RequestedAt.Add(time.Second)
	marker, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatalf("encode duplicate challenge: %v", err)
	}
	duplicateComment := issueCommentAPI{
		ID:        int64(len(backend.comments) + 1),
		Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, duplicate.Head),
		CreatedAt: duplicate.RequestedAt,
	}
	duplicateComment.User.Login = trustedActor
	backend.comments = append(backend.comments, duplicateComment)
	_, attestationFile := writeTestAttestation(t, backend.head, canonical)

	backend.postCommentResponseMode = "transport"
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil ||
		!strings.Contains(err.Error(), "do not repost blindly") {
		t.Fatalf("partial challenge-closure response error = %v, want retry guidance", err)
	}
	if got, want := len(backend.comments), 3; got != want {
		t.Fatalf("partial challenge-closure history comments = %d, want %d", got, want)
	}

	backend.postCommentResponseMode = ""
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("retry after partial challenge-closure response: %v", err)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.challenges) != 2 || len(history.closures) != 1 || len(history.receipts) != 1 {
		t.Fatalf("partial retry history = challenges %d closures %d receipts %d; want 2, 1, 1",
			len(history.challenges), len(history.closures), len(history.receipts))
	}
}

func TestEvaluationChallengeClosureVerificationReadFailureIsRetryable(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	canonical := requestTestChallenge(t, &application, &stdout)
	appendFinalEquivalentChallenge(t, backend)
	_, attestationFile := writeTestAttestation(t, backend.head, canonical)
	sentinel := errors.New("simulated challenge closure verification GET failure")
	failNextPRRead := false
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if failNextPRRead && command == "gh api repos/goxdra/goxsd9/pulls/14" {
			failNextPRRead = false
			return "", sentinel
		}
		output, err := backend.execute(dir, input, name, args...)
		if err == nil && command == "gh api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" &&
			backend.commentPostCount == 2 {
			failNextPRRead = true
		}
		return output, err
	}

	err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil {
		t.Fatal("challenge closure verification succeeded after injected GET failure")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("challenge closure verification disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("challenge closure verification error = %v, want sentinel cause", err)
	}
	if got, want := backend.commentPostCount, 2; got != want {
		t.Fatalf("challenge closure POST count = %d, want %d", got, want)
	}
}

func TestEvaluationChallengeFinalReadFailureIsRetryable(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)
	sentinel := errors.New("simulated challenge final-reread GET failure")
	postSeen := false
	postPRReads := 0
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if postSeen && command == "gh api repos/goxdra/goxsd9/pulls/14" {
			postPRReads++
			if postPRReads == 2 {
				postSeen = false
				return "", sentinel
			}
		}
		output, err := backend.execute(dir, input, name, args...)
		if err == nil && command == "gh api --method POST repos/goxdra/goxsd9/issues/14/comments --input -" {
			postSeen = true
			postPRReads = 0
		}
		return output, err
	}

	err := application.runEvaluation([]string{"challenge", "14"})
	if err == nil {
		t.Fatal("challenge final reread succeeded after injected GET failure")
	}
	if got := operationDispositionOf(err); got != operationDispositionRetryable {
		t.Fatalf("challenge final reread disposition = %v, want retryable: %v", got, err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("challenge final reread error = %v, want sentinel cause", err)
	}
	if got, want := backend.commentPostCount, 1; got != want {
		t.Fatalf("challenge POST count = %d, want %d", got, want)
	}
}

func TestEvaluationRecordRejectsInvalidAttestationBeforeChallengeClosure(t *testing.T) {
	t.Run("wrong head", func(t *testing.T) {
		backend, application, stdout := newConvergenceWorkflowFixture(t)
		canonical := requestTestChallenge(t, application, stdout)
		appendFinalEquivalentChallenge(t, backend)
		_, attestationFile := writeTestAttestationRun(t, "different-head", canonical, "invalid-head")
		assertEvaluationAttestationRejectedBeforeChallengeClosure(t, backend, application, stdout, attestationFile)
	})
	t.Run("used challenge", func(t *testing.T) {
		backend, application, stdout := newConvergenceWorkflowFixture(t)
		canonical := requestTestChallenge(t, application, stdout)
		_, firstAttestationFile := writeTestAttestationRun(t, backend.head, canonical, "first-run")
		if err := application.runEvaluation([]string{"record", "14", "--attestation-file", firstAttestationFile}); err != nil {
			t.Fatalf("record initial attestation: %v", err)
		}
		appendFinalEquivalentChallenge(t, backend)
		_, attestationFile := writeTestAttestationRun(t, backend.head, canonical, "second-run")
		assertEvaluationAttestationRejectedBeforeChallengeClosure(t, backend, application, stdout, attestationFile)
	})
}

func assertEvaluationAttestationRejectedBeforeChallengeClosure(t *testing.T, backend *workflowBackend,
	application *app, stdout *bytes.Buffer, attestationFile string) {
	t.Helper()
	commentCount := len(backend.comments)
	postCount := backend.commentPostCount
	stdout.Reset()
	err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil {
		t.Fatal("invalid attestation reached challenge convergence")
	}
	if len(backend.comments) != commentCount || backend.commentPostCount != postCount {
		t.Fatalf("invalid attestation mutated challenge history: comments %d->%d posts %d->%d",
			commentCount, len(backend.comments), postCount, backend.commentPostCount)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid attestation emitted output %q", stdout.String())
	}
}

func TestEvaluationRecordReplaysExpiredExactReceipt(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestationRun(t, backend.head, challenge, "expired-replay")
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record initial attestation: %v", err)
	}
	expireWorkflowChallenge(t, backend, challenge.Challenge)
	expiredChallenge, ok := parseEvaluationChallenge(backend.comments[0].Body)
	if !ok {
		t.Fatal("expired challenge marker was not parseable")
	}
	recordedAt := expiredChallenge.RequestedAt.Add(time.Minute)
	backend.comments[1].CreatedAt = recordedAt
	backend.comments[1].Body = replaceTestReceipt(t, backend.comments[1].Body, func(receipt *evaluationReceipt) {
		receipt.RecordedAt = recordedAt
	})
	commentCount := len(backend.comments)
	postCount := backend.commentPostCount
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("replay expired exact attestation: %v", err)
	}
	if len(backend.comments) != commentCount || backend.commentPostCount != postCount {
		t.Fatalf("expired exact replay mutated history: comments %d->%d posts %d->%d",
			commentCount, len(backend.comments), postCount, backend.commentPostCount)
	}
	if !strings.Contains(stdout.String(), "already recorded") {
		t.Fatalf("expired exact replay output = %q, want idempotent retry", stdout.String())
	}
}

func TestEvaluationRecordReplaysExactReceiptOnNoncanonicalChallenge(t *testing.T) {
	backend, application, stdout := newConvergenceWorkflowFixture(t)
	canonical := requestTestChallenge(t, application, stdout)
	appendFinalEquivalentChallenge(t, backend)
	duplicate, ok := parseEvaluationChallenge(backend.comments[1].Body)
	if !ok {
		t.Fatal("noncanonical challenge marker was not parseable")
	}
	recordedAt := duplicate.RequestedAt.Add(time.Minute)
	receiptComment := evaluationTerminalConflictReceiptComment(t, duplicate, "noncanonical-replay", 1, recordedAt)
	receiptComment.ID = 3
	appendWorkflowEvaluationComment(t, backend, receiptComment)
	_, attestationFile := writeTestAttestationRun(t, backend.head, duplicate, "noncanonical-replay")
	commentCount := len(backend.comments)
	postCount := backend.commentPostCount
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("replay exact noncanonical attestation: %v", err)
	}
	if len(backend.comments) != commentCount+1 || backend.commentPostCount != postCount+1 {
		t.Fatalf("noncanonical exact replay convergence = comments %d posts %d, want one closure",
			len(backend.comments), backend.commentPostCount)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.closures) != 1 || len(history.receipts) != 1 {
		t.Fatalf("noncanonical exact replay history = closures %d receipts %d, want one each",
			len(history.closures), len(history.receipts))
	}
	if !strings.Contains(stdout.String(), "already recorded") {
		t.Fatalf("noncanonical exact replay output = %q, want idempotent retry", stdout.String())
	}
	if canonical.Challenge == duplicate.Challenge {
		t.Fatal("noncanonical replay fixture did not retain distinct canonical challenge")
	}
}

func TestEvaluationChallengeClosureDuplicateVisibilityFailsClosed(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := newResolutionWorkflowApplication(backend, &stdout)

	canonical := requestTestChallenge(t, &application, &stdout)
	appendFinalEquivalentChallenge(t, backend)
	_, attestationFile := writeTestAttestation(t, backend.head, canonical)

	backend.duplicateClosurePost = true
	err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil || !strings.Contains(err.Error(), "multiple authenticated controller closures") {
		t.Fatalf("duplicate closure visibility error = %v, want fail-closed duplicate rejection", err)
	}
	commentCount := len(backend.comments)
	postCount := backend.commentPostCount
	if got, want := commentCount, 4; got != want {
		t.Fatalf("duplicate closure visibility comments = %d, want %d", got, want)
	}
	history, err := parseEvaluationHistory(pullRequestCommentsFromAPI(t, backend.comments))
	if err != nil {
		t.Fatalf("parse duplicate closure visibility history: %v", err)
	}
	if got, want := len(history.closures), 2; got != want {
		t.Fatalf("duplicate closure visibility closures = %d, want %d preserved records", got, want)
	}

	stdout.Reset()
	err = application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil || !strings.Contains(err.Error(), "multiple authenticated controller closures") {
		t.Fatalf("duplicate closure retry error = %v, want fail-closed duplicate rejection", err)
	}
	if len(backend.comments) != commentCount || backend.commentPostCount != postCount {
		t.Fatalf("duplicate closure retry mutated history: comments=%d (want %d), posts=%d (want %d)",
			len(backend.comments), commentCount, backend.commentPostCount, postCount)
	}
}

func TestEvaluationChallengeConvergenceRejectsAmbiguousTerminalHistory(t *testing.T) {
	tests := []struct {
		name   string
		append func(*testing.T, *workflowBackend, evaluationChallenge, evaluationChallenge)
		want   string
	}{
		{
			name: "duplicate resolutions",
			append: func(t *testing.T, backend *workflowBackend, canonical, _ evaluationChallenge) {
				resolvedAt := canonical.RequestedAt.Add(evaluationChallengeDuration + time.Hour)
				appendWorkflowEvaluationComment(t, backend,
					evaluationTerminalConflictResolutionComment(t, canonical, resolvedAt, "expired"))
				appendWorkflowEvaluationComment(t, backend,
					evaluationTerminalConflictResolutionComment(t, canonical, resolvedAt.Add(time.Minute), "expired"))
			},
			want: "multiple matching no-verdict resolutions",
		},
		{
			name: "receipt and resolution",
			append: func(t *testing.T, backend *workflowBackend, canonical, duplicate evaluationChallenge) {
				appendWorkflowEvaluationComment(t, backend,
					evaluationTerminalConflictReceiptComment(t, canonical, "terminal-receipt", 1,
						canonical.RequestedAt.Add(time.Hour)))
				resolvedAt := duplicate.RequestedAt.Add(evaluationChallengeDuration + time.Hour)
				appendWorkflowEvaluationComment(t, backend,
					evaluationTerminalConflictResolutionComment(t, duplicate, resolvedAt, "expired"))
			},
			want: "both an attested receipt and a no-verdict resolution",
		},
		{
			name: "receipts across equivalent challenges",
			append: func(t *testing.T, backend *workflowBackend, canonical, duplicate evaluationChallenge) {
				appendWorkflowEvaluationComment(t, backend,
					evaluationTerminalConflictReceiptComment(t, canonical, "terminal-receipt-1", 1,
						canonical.RequestedAt.Add(time.Hour)))
				appendWorkflowEvaluationComment(t, backend,
					evaluationTerminalConflictReceiptComment(t, duplicate, "terminal-receipt-2", 2,
						duplicate.RequestedAt.Add(time.Hour)))
			},
			want: "multiple matching trusted receipts",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			var stdout bytes.Buffer
			application := newResolutionWorkflowApplication(backend, &stdout)
			base := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
			canonical := evaluationChallenge{
				Challenge:      "terminal-canonical",
				Repository:     repositoryKey,
				Head:           backend.head,
				PR:             14,
				BodySHA256:     strings.Repeat("a", 64),
				EvidenceSHA256: strings.Repeat("b", 64),
				RequestedAt:    base,
			}
			duplicate := canonical
			duplicate.Challenge = "terminal-duplicate"
			duplicate.RequestedAt = base.Add(time.Minute)
			canonicalComment := testEvaluationChallengeComment(t, canonical)
			canonicalComment.ID = 1
			duplicateComment := testEvaluationChallengeComment(t, duplicate)
			duplicateComment.ID = 2
			appendWorkflowEvaluationComment(t, backend, canonicalComment)
			appendWorkflowEvaluationComment(t, backend, duplicateComment)
			test.append(t, backend, canonical, duplicate)
			initialComments := len(backend.comments)

			_, attestationFile := writeTestAttestation(t, backend.head, canonical)
			err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ambiguous terminal history error = %v, want %q", err, test.want)
			}
			if len(backend.comments) != initialComments || backend.commentPostCount != 0 {
				t.Fatalf("ambiguous terminal history mutated comments=%d (want %d), posts=%d (want 0)",
					len(backend.comments), initialComments, backend.commentPostCount)
			}
			if stdout.Len() != 0 {
				t.Fatalf("ambiguous terminal history emitted output %q", stdout.String())
			}
		})
	}
}

func evaluationTerminalConflictReceiptComment(t *testing.T, challenge evaluationChallenge, runID string,
	round int, recordedAt time.Time) pullRequestComment {
	t.Helper()
	attestationJSON, _ := writeTestAttestationRun(t, challenge.Head, challenge, runID)
	var attestation evaluationAttestation
	if err := json.Unmarshal(attestationJSON, &attestation); err != nil {
		t.Fatalf("decode terminal conflict attestation: %v", err)
	}
	report := canonicalEvaluationReport(renderEvaluationReport(attestation))
	receipt := evaluationReceipt{
		AttestationSHA256: sha256Hex(attestationJSON),
		BaseRefName:       "main",
		Challenge:         challenge.Challenge,
		ClaimProofs:       []evaluationClaimProof{{Issue: 13, Branch: "agent/issue-13", SHA: challenge.Head}},
		ClosingIssues:     []int{13},
		Evaluator:         "Examiner",
		EvaluatorRunID:    runID,
		Head:              challenge.Head,
		HeadRefName:       "agent/issue-13",
		BodySHA256:        challenge.BodySHA256,
		EvidenceSHA256:    challenge.EvidenceSHA256,
		Repository:        repositoryKey,
		PR:                challenge.PR,
		RecordedAt:        recordedAt,
		ReportSHA256:      sha256Hex(report),
		ReportTransport:   evaluationReportTransportV1,
		Round:             round,
		Verdict:           "pass",
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode terminal conflict receipt: %v", err)
	}
	comment := statusComment(evaluationComment(marker, attestationJSON, string(report)), trustedActor, recordedAt)
	return comment
}

func evaluationTerminalConflictResolutionComment(t *testing.T, challenge evaluationChallenge,
	resolvedAt time.Time, reason string) pullRequestComment {
	t.Helper()
	resolution := evaluationResolution{
		BodySHA256:     challenge.BodySHA256,
		Challenge:      challenge.Challenge,
		EvidenceSHA256: challenge.EvidenceSHA256,
		Head:           challenge.Head,
		Repository:     challenge.Repository,
		PR:             challenge.PR,
		Reason:         reason,
		ResolvedAt:     resolvedAt,
		Resolver:       trustedActor,
		Schema:         evaluationResolutionSchema,
	}
	marker, err := json.Marshal(resolution)
	if err != nil {
		t.Fatalf("encode terminal conflict resolution: %v", err)
	}
	return statusComment(evaluationResolutionComment(marker, reason), trustedActor, resolvedAt)
}

func TestThirdFailureProjectTransitionRetryConvergesWithoutReceipt(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.projectStatus = "Ready"
	var stdout bytes.Buffer
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         &stdout,
		stderr:                         new(bytes.Buffer),
	}
	thirdChallenge, thirdAttestationFile := recordThirdFailureRounds(t, &application, backend, &stdout, true)
	assertPartialThirdFailure(t, backend)

	commentCount := len(backend.comments)
	_, unrelatedAttestationFile := writeTestFailureAttestationRun(t, backend.head, thirdChallenge,
		"unrelated-attestation")
	assertReconciliationRejects(t, &application, backend, unrelatedAttestationFile, commentCount,
		"unrelated attestation")

	junkAttestationFile := filepath.Join(t.TempDir(), "junk-attestation.json")
	if err := os.WriteFile(junkAttestationFile, []byte(`{"schema":"evaluation-attestation/v1","verdict":"fail"}`), 0o600); err != nil {
		t.Fatalf("write junk attestation: %v", err)
	}
	assertReconciliationRejects(t, &application, backend, junkAttestationFile, commentCount,
		"junk attestation")

	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", thirdAttestationFile}); err != nil {
		t.Fatalf("retry exact third-failure attestation: %v", err)
	}
	if len(backend.comments) != commentCount {
		t.Fatalf("exact retry appended a receipt: comments = %d, want %d", len(backend.comments), commentCount)
	}
	if !backend.needsHuman || backend.projectStatus != "Backlog" || backend.projectTransitionAttempts != 2 {
		t.Fatalf("reconciled transition state = label %t, Project %s, attempts %d",
			backend.needsHuman, backend.projectStatus, backend.projectTransitionAttempts)
	}
}

func recordThirdFailureRounds(t *testing.T, application *app, backend *workflowBackend,
	stdout *bytes.Buffer, injectProjectFailure bool,
) (evaluationChallenge, string) {
	t.Helper()
	var thirdChallenge evaluationChallenge
	var thirdAttestationFile string
	for round := 1; round <= 3; round++ {
		stdout.Reset()
		challenge := requestTestChallenge(t, application, stdout)
		if round == 3 {
			thirdChallenge = challenge
			if injectProjectFailure {
				backend.projectTransitionFailures = 1
			}
		}
		_, attestationFile := writeTestFailureAttestationRun(t, backend.head, challenge,
			fmt.Sprintf("examiner-third-failure-%d", round))
		if round == 3 {
			thirdAttestationFile = attestationFile
		}
		stdout.Reset()
		err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
		assertThirdFailureRound(t, round, err, injectProjectFailure)
	}
	return thirdChallenge, thirdAttestationFile
}

func assertThirdFailureRound(t *testing.T, round int, err error, injectProjectFailure bool) {
	t.Helper()
	if round != 3 {
		if err != nil {
			t.Fatalf("record failure round %d: %v", round, err)
		}
		return
	}
	if err == nil {
		t.Fatal("third failure unexpectedly completed transition")
	}
	if injectProjectFailure && !strings.Contains(err.Error(), "project Backlog phase incomplete") {
		t.Fatalf("third failure error = %v, want Project transition failure", err)
	}
}

func assertPartialThirdFailure(t *testing.T, backend *workflowBackend) {
	t.Helper()
	if got, want := len(backend.comments), 6; got != want {
		t.Fatalf("comments after failed transition = %d, want %d", got, want)
	}
	if !backend.needsHuman || backend.projectStatus != "Ready" || backend.projectTransitionAttempts != 1 {
		t.Fatalf("partial transition state = label %t, Project %s, attempts %d",
			backend.needsHuman, backend.projectStatus, backend.projectTransitionAttempts)
	}
}

func assertReconciliationRejects(t *testing.T, application *app, backend *workflowBackend,
	attestationFile string, commentCount int, label string,
) {
	t.Helper()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil {
		t.Fatalf("%s bypassed recorded third-failure reconciliation", label)
	}
	if len(backend.comments) != commentCount || backend.projectTransitionAttempts != 1 {
		t.Fatalf("%s mutated evaluation or Project state", label)
	}
}

func TestThirdFailureTransitionRequiresCurrentOpenCanonicalProject(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*workflowBackend)
		repair  func(*workflowBackend)
	}{
		{name: "closed issue", prepare: func(backend *workflowBackend) { backend.issueState = "closed" }, repair: func(backend *workflowBackend) { backend.issueState = "open" }},
		{name: "missing Project", prepare: func(backend *workflowBackend) { backend.projectMember = false }, repair: func(backend *workflowBackend) { backend.projectMember = true }},
		{name: "changed Project identity", prepare: func(backend *workflowBackend) { backend.projectRepository = "other/repository" }, repair: func(backend *workflowBackend) { backend.projectRepository = repositoryKey }},
		{name: "PullRequest Project item", prepare: func(backend *workflowBackend) { backend.projectType = "PullRequest" }, repair: func(backend *workflowBackend) { backend.projectType = "Issue" }},
		{name: "missing Project item ID", prepare: func(backend *workflowBackend) { backend.projectItemID = "" }, repair: func(backend *workflowBackend) { backend.projectItemID = "item-13" }},
		{name: "malformed Project content", prepare: func(backend *workflowBackend) { backend.projectMalformed = true }, repair: func(backend *workflowBackend) { backend.projectMalformed = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runThirdFailureProofCase(t, test.prepare, test.repair)
		})
	}
}

func runThirdFailureProofCase(t *testing.T, prepare, repair func(*workflowBackend)) {
	t.Helper()
	backend := newWorkflowBackend(t)
	backend.projectStatus = "Ready"
	prepare(backend)
	var stdout bytes.Buffer
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         &stdout,
		stderr:                         new(bytes.Buffer),
	}
	_, attestationFile := recordThirdFailureRounds(t, &application, backend, &stdout, false)
	if len(backend.comments) != 6 || backend.needsHuman || backend.projectTransitionAttempts != 0 {
		t.Fatalf("invalid proof mutated after recording: comments=%d label=%t Project attempts=%d",
			len(backend.comments), backend.needsHuman, backend.projectTransitionAttempts)
	}
	commentCount := len(backend.comments)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil {
		t.Fatal("exact retry accepted invalid current transition proof")
	}
	if len(backend.comments) != commentCount || backend.needsHuman || backend.projectTransitionAttempts != 0 {
		t.Fatal("invalid exact retry mutated transition state")
	}
	repair(backend)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("retry repaired transition proof: %v", err)
	}
	if len(backend.comments) != commentCount || !backend.needsHuman || backend.projectStatus != "Backlog" ||
		backend.projectTransitionAttempts != 1 {
		t.Fatalf("repaired transition state: comments=%d label=%t Project=%s attempts=%d",
			len(backend.comments), backend.needsHuman, backend.projectStatus, backend.projectTransitionAttempts)
	}
}

func TestEvaluationAcceptsRunSpecificLocalBranchForFixedPRHead(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.localBranch = claimLocalBranch(13, "run-test")
	var stdout bytes.Buffer
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         &stdout,
		stderr:                         new(bytes.Buffer),
	}
	challenge := requestTestChallenge(t, &application, &stdout)
	if challenge.Head != backend.head {
		t.Fatalf("challenge head = %q, want fixed PR head %q", challenge.Head, backend.head)
	}
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record evaluation from run-local worktree: %v", err)
	}
	receipt, ok := parseEvaluationReceipt(backend.comments[len(backend.comments)-1].Body)
	if !ok {
		t.Fatal("run-local evaluation receipt was not parseable")
	}
	if receipt.HeadRefName != backend.branch {
		t.Fatalf("receipt head branch = %q, want published claim branch %q", receipt.HeadRefName, backend.branch)
	}
}

func TestManagedDocumentCuratorValidationPrecedesChallengeAndFinishMutation(t *testing.T) {
	tests := []struct {
		name    string
		command string
		curator func(string) curatorResult
		want    string
	}{
		{
			name: "challenge missing Curator", command: "challenge",
			curator: noCuratorResult,
			want:    "managed-document changes require",
		},
		{
			name: "challenge mismatched Curator", command: "challenge",
			curator: func(_ string) curatorResult { return testPassingCurator("mismatched-head") },
			want:    "exact PR head",
		},
		{
			name: "finish missing Curator", command: "finish",
			curator: noCuratorResult,
			want:    "managed-document changes require",
		},
		{
			name: "finish mismatched Curator", command: "finish",
			curator: func(_ string) curatorResult { return testPassingCurator("mismatched-head") },
			want:    "exact PR head",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			configureManagedDocumentEvidence(t, backend, test.curator(backend.head))
			var stdout bytes.Buffer
			application := app{
				ctx:                            context.Background(),
				executeCommand:                 backend.execute,
				verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
				stdout:                         &stdout,
				stderr:                         new(bytes.Buffer),
			}

			var err error
			switch test.command {
			case "challenge":
				err = application.runEvaluation([]string{"challenge", "14"})
			case "finish":
				err = application.runPR(backend.finishArgs())
			default:
				t.Fatalf("unknown command %q", test.command)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s error = %v, want %q", test.command, err, test.want)
			}
			if len(backend.comments) != 0 || backend.bodyPatchCount != 0 || backend.merged || backend.projectDone || backend.issuePatchCount != 0 || backend.issueState != "open" {
				t.Fatalf("rejected %s mutated GitHub state: comments=%d bodyPatches=%d merged=%t projectDone=%t issueState=%s issuePatches=%d",
					test.command, len(backend.comments), backend.bodyPatchCount, backend.merged, backend.projectDone, backend.issueState, backend.issuePatchCount)
			}
		})
	}
}

func configureManagedDocumentEvidence(t *testing.T, backend *workflowBackend, curator curatorResult) {
	t.Helper()
	parsed, err := parsePREvidenceBody(backend.body)
	if err != nil {
		t.Fatalf("parse workflow PR evidence: %v", err)
	}
	managedChange := testManagedChange()
	managedChange.Status = "D"
	managedChange.Additions = 0
	managedChange.Deletions = 1
	managedChange.Lines = 0
	managedChange.Words = 0
	parsed.evidence.DocumentationAudit.ManagedChanges = []documentationChangeReport{managedChange}
	parsed.evidence.Curator = curator
	block, err := renderPREvidenceBlock(parsed.evidence)
	if err != nil {
		t.Fatalf("render managed-document PR evidence: %v", err)
	}
	updated, err := replacePREvidenceBlock(backend.body, block)
	if err != nil {
		t.Fatalf("replace managed-document PR evidence: %v", err)
	}
	backend.body = updated
	backend.managedDocumentChange = true
}

func TestSignalEvidenceRecomputationPrecedesUpdateChallengeAndFinishMutations(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *workflowBackend, *app)
		want string
	}{
		{name: "update", run: func(t *testing.T, backend *workflowBackend, application *app) {
			signals := staleDevelopmentSignalsReport(backend.head)
			audit := testWorkflowPREvidence(backend.head).DocumentationAudit
			signalsPath := writePREvidenceJSONSource(t, "signals.json", signals)
			auditPath := writePREvidenceJSONSource(t, "audit.json", audit)
			if err := application.runPREvidence([]string{"update", "14", "--signals-file", signalsPath, "--docs-audit-file", auditPath}); err == nil {
				t.Fatal("rewritten old signal payload reached body PATCH")
			}
		}, want: "body PATCH"},
		{name: "challenge", run: func(t *testing.T, _ *workflowBackend, application *app) {
			if err := application.runEvaluation([]string{"challenge", "14"}); err == nil {
				t.Fatal("rewritten old signal payload reached comment POST")
			}
		}, want: "comment POST"},
		{name: "finish", run: func(t *testing.T, backend *workflowBackend, application *app) {
			if err := application.runPR(backend.finishArgs()); err == nil {
				t.Fatal("rewritten old signal payload reached finish mutation")
			}
		}, want: "finish mutation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			makeSignalEvidenceStale(t, backend)
			application := app{
				ctx: context.Background(), executeCommand: backend.execute,
				buildDevelopmentSignalsReport: func(_ string, _ string, _ string, _ time.Duration,
					_ []coverageExplanation, _ []additionalFuzzTarget,
				) (developmentSignalsReport, error) {
					return testWorkflowPREvidence(backend.head).DevelopmentSignals, nil
				},
				stdout: new(bytes.Buffer), stderr: new(bytes.Buffer),
			}
			test.run(t, backend, &application)
			if backend.bodyPatchCount != 0 || len(backend.comments) != 0 || backend.merged || backend.projectDone || backend.issuePatchCount != 0 || backend.issueState != "open" {
				t.Fatalf("%s mutated GitHub state: bodyPatches=%d comments=%d merged=%t projectDone=%t issueState=%s issuePatches=%d",
					test.want, backend.bodyPatchCount, len(backend.comments), backend.merged, backend.projectDone, backend.issueState, backend.issuePatchCount)
			}
		})
	}
}

func TestSignalEvidenceRecomputationAcceptsExactCurrentReport(t *testing.T) {
	backend := newWorkflowBackend(t)
	application := app{
		ctx: context.Background(), executeCommand: backend.execute,
		buildDevelopmentSignalsReport: func(_ string, _ string, _ string, _ time.Duration,
			_ []coverageExplanation, _ []additionalFuzzTarget,
		) (developmentSignalsReport, error) {
			return testWorkflowPREvidence(backend.head).DevelopmentSignals, nil
		},
		stdout: new(bytes.Buffer), stderr: new(bytes.Buffer),
	}
	if err := application.runEvaluation([]string{"challenge", "14"}); err != nil {
		t.Fatalf("exact current signal report rejected: %v", err)
	}
	if len(backend.comments) != 1 {
		t.Fatalf("challenge comment count = %d, want 1", len(backend.comments))
	}
}

func makeSignalEvidenceStale(t *testing.T, backend *workflowBackend) {
	t.Helper()
	parsed, err := parsePREvidenceBody(backend.body)
	if err != nil {
		t.Fatalf("parse workflow evidence: %v", err)
	}
	parsed.evidence.DevelopmentSignals = staleDevelopmentSignalsReport(backend.head)
	block, err := renderPREvidenceBlock(parsed.evidence)
	if err != nil {
		t.Fatalf("render stale workflow evidence: %v", err)
	}
	backend.body, err = replacePREvidenceBlock(backend.body, block)
	if err != nil {
		t.Fatalf("replace stale workflow evidence: %v", err)
	}
	view := pullRequestView{BaseRefOID: "base-sha", HeadRefOID: backend.head, Body: backend.body}
	if _, err := validatePREvidenceForView(view); err != nil {
		t.Fatalf("stale complete workflow evidence is structurally invalid: %v", err)
	}
}

func staleDevelopmentSignalsReport(head string) developmentSignalsReport {
	oldBase, oldHead := "archived-base", "archived-head"
	packageReport := coveragePackageReport{
		Package: "example.com/copied", Status: "changed", Affected: true,
		Base: coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 8, Percent: 80},
		Head: coverageSideReport{Present: true, HasTests: true, Statements: 10, Covered: 7, Percent: 70},
	}
	packageReport.Delta = coverageDelta(packageReport.Base, packageReport.Head)
	packages := []coveragePackageReport{packageReport}
	coverage := coverageReport{
		Base: oldBase, Head: oldHead, Packages: packages,
		Affected: coverageTotals(packages, true), Repository: coverageTotals(packages, false),
	}
	report := developmentSignalsReport{
		Schema: developmentSignalsSchema, Base: oldBase, Head: oldHead, Coverage: coverage,
		CoverageExplanations: []coverageExplanation{{
			Schema: coverageExplanationSchema, Package: packageReport.Package, Reason: "archived coverage regression",
			Base: packageReport.Base, Head: packageReport.Head,
		}},
		Fuzz: []signalFuzzReport{{
			Boundary: "syntax.go", Package: ".", Target: "FuzzDecodeSyntax", Duration: "250ms",
			Workers: 1, Offline: true, Result: "success",
		}},
		AdditionalFuzz: []additionalFuzzReport{}, Selection: "selected",
		Catalog: noMeasuredDevelopmentSignal, XSDFeatureSupport: noMeasuredDevelopmentSignal,
		ExecutableConformance: noMeasuredDevelopmentSignal,
	}
	report.Base = "base-sha"
	report.Head = head
	report.Coverage.Base = report.Base
	report.Coverage.Head = report.Head
	return report
}

func testPassingCurator(head string) curatorResult {
	return curatorResult{
		Schema: curatorResultSchema, RunID: "curator-command-flow", Head: head, Verdict: "pass",
		Summary: "Every managed change is in its canonical home.", Findings: []curatorFinding{},
	}
}

func TestFinishRejectsEvaluationMetadataDrift(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         &stdout,
		stderr:                         new(bytes.Buffer),
	}
	challenge := requestTestChallenge(t, &application, &stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record evaluation: %v", err)
	}
	backend.body += "\nReviewed metadata drift.\n"
	if err := application.runPR(backend.finishArgs()); err == nil || !strings.Contains(err.Error(), "evaluation") {
		t.Fatalf("finish with body drift error = %v, want metadata-bound refusal", err)
	}
	if backend.merged {
		t.Fatal("metadata drift reached the merge endpoint")
	}
}

func TestAmbiguousMergeResponsesReconcile(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "transport loss", mode: "transport"},
		{name: "malformed JSON", mode: "malformed"},
		{name: "missing SHA", mode: "missing-sha"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			backend.mergeResponseMode = test.mode
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			application := app{
				ctx:                            context.Background(),
				executeCommand:                 backend.execute,
				verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
				stdout:                         &stdout,
				stderr:                         &stderr,
			}
			challenge := requestTestChallenge(t, &application, &stdout)
			_, attestationFile := writeTestAttestation(t, backend.head, challenge)
			if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
				t.Fatalf("record evaluation: %v", err)
			}
			stdout.Reset()
			if err := application.runPR(backend.finishArgs()); err != nil {
				t.Fatalf("finish %s merge response: %v", test.mode, err)
			}
			checkMergeResult(t, backend)
			if !strings.Contains(stdout.String(), "merged at evaluated head evaluated-head as merge-commit") {
				t.Fatalf("finish output = %q, want reconciled merge", stdout.String())
			}
		})
	}
}

func TestAmbiguousMergeResponseRejectsPostMergeMetadataDrift(t *testing.T) {
	backend := newWorkflowBackend(t)
	backend.mergeResponseMode = "transport"
	backend.mutatePRBodyAfterMerge = true
	var stdout bytes.Buffer
	application := app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         &stdout,
		stderr:                         new(bytes.Buffer),
	}
	challenge := requestTestChallenge(t, &application, &stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record evaluation: %v", err)
	}
	err := application.runPR(backend.finishArgs())
	if err == nil || !strings.Contains(err.Error(), "body") || !strings.Contains(err.Error(), "pr recover 14") {
		t.Fatalf("ambiguous drift error = %v, want body-drift recovery refusal", err)
	}
	if !backend.merged {
		t.Fatal("ambiguous merge response did not record completed merge")
	}
}

func TestFinishRejectsPostMergeEvaluationReceiptOnBothMergePaths(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
	}{
		{name: "normal merge response", mode: ""},
		{name: "ambiguous merge response", mode: "transport"},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend, application := newPassingFinishFixture(t)
			backend.mergeResponseMode = test.mode
			backend.delayReceiptAfterMerge = true

			err := application.runPR(backend.finishArgs())
			if err == nil || !strings.Contains(err.Error(), "merge boundary") {
				t.Fatalf("post-merge receipt error = %v, want merge-boundary refusal", err)
			}
			if !backend.merged || backend.issueState != "open" || backend.issuePatchCount != 0 || backend.projectDone {
				t.Fatalf("post-merge receipt reconciliation state = merged %t, issue %s, PATCH %d, Project Done %t",
					backend.merged, backend.issueState, backend.issuePatchCount, backend.projectDone)
			}
		})
	}
}

func TestEvaluationRecordRejectsReservedAttestationSequences(t *testing.T) {
	fields := []struct {
		name  string
		apply func(*evaluationAttestation, string)
	}{
		{name: "summary", apply: func(attestation *evaluationAttestation, value string) {
			attestation.Summary = value
		}},
		{name: "finding location", apply: func(attestation *evaluationAttestation, value string) {
			attestation.Findings[0].Location = value
		}},
		{name: "finding impact", apply: func(attestation *evaluationAttestation, value string) {
			attestation.Findings[0].Impact = value
		}},
		{name: "finding required correction", apply: func(attestation *evaluationAttestation, value string) {
			attestation.Findings[0].RequiredCorrection = value
		}},
	}
	for _, field := range fields {
		for _, sequence := range evaluationReservedTextSequences {
			t.Run(field.name+"/"+sequence.name, func(t *testing.T) {
				testReservedAttestationSequence(t, field, sequence)
			})
		}
	}
}

func testReservedAttestationSequence(t *testing.T, field struct {
	name  string
	apply func(*evaluationAttestation, string)
}, sequence struct {
	name  string
	value string
}) {
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
	challenge := requestTestChallenge(t, application, stdout)
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge,
		Evaluator: "Examiner",
		Findings: evaluationFindings{{
			Impact:             "The reserved sequence would corrupt the report.",
			Location:           "internal/workflowctl/evaluation.go:1",
			RequiredCorrection: "Reject the reserved sequence before posting.",
		}},
		Head:    backend.head,
		PR:      14,
		RunID:   "reserved-sequence-test",
		Schema:  evaluationAttestationSchema,
		Summary: "The summary remains data; delimiter --> remains data.",
		Verdict: "fail",
	}
	if field.name == "summary" {
		attestation.Findings = evaluationFindings{}
		attestation.Verdict = "pass"
	}
	field.apply(&attestation, sequence.value+"payload --> remains data.")
	_, attestationFile := writeTestAttestationValue(t, attestation)

	commentCount := len(backend.comments)
	err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile})
	if err == nil {
		t.Fatal("attestation containing a reserved sequence was accepted")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved-sequence error = %v", err)
	}
	if len(backend.comments) != commentCount {
		t.Fatal("rejected attestation appended a comment")
	}
}

func TestEvaluationRepairPreservesFailedRound(t *testing.T) {
	backend, application, stdout := newRepairFixture(t)
	originalBody := backend.comments[1].Body
	backend.comments[1].Body = legacyTransportMismatch(t, originalBody)
	if backend.comments[1].Body == originalBody {
		t.Fatal("repair fixture did not create a visible transport mismatch")
	}

	stdout.Reset()
	if err := application.runEvaluation([]string{"repair", "14", "--round", "1"}); err != nil {
		t.Fatalf("repair evaluation: %v", err)
	}
	assertRepairedFailure(t, backend)
	recordSecondEvaluation(t, application, backend, stdout)
}

func TestEvaluationRepairAllowsPriorHead(t *testing.T) {
	backend, application, stdout := newRepairFixture(t)
	priorHead := backend.head
	backend.comments[1].Body = legacyTransportMismatch(t, backend.comments[1].Body)
	backend.head = "remediated-head"

	stdout.Reset()
	if err := application.runEvaluation([]string{"repair", "14", "--round", "1"}); err != nil {
		t.Fatalf("repair prior-head evaluation: %v", err)
	}
	repair, ok := parseEvaluationRepair(backend.comments[2].Body)
	if !ok {
		t.Fatal("prior-head repair marker was not parseable")
	}
	if repair.Head != priorHead {
		t.Fatalf("repair head = %q, want prior head %q", repair.Head, priorHead)
	}
	receipts, err := evaluationReceipts(pullRequestCommentsFromAPI(t, backend.comments))
	if err != nil {
		t.Fatalf("read prior-head repaired evaluation history: %v", err)
	}
	if len(receipts) != 1 || receipts[0].Head != priorHead || receipts[0].Round != 1 || receipts[0].Verdict != "fail" {
		t.Fatalf("prior-head repaired receipts = %#v", receipts)
	}
}

func assertRepairedFailure(t *testing.T, backend *workflowBackend) {
	t.Helper()
	if got, want := len(backend.comments), 3; got != want {
		t.Fatalf("comments after repair = %d, want %d", got, want)
	}
	receipts := pullRequestCommentsFromAPI(t, backend.comments)
	parsedReceipts, err := evaluationReceipts(receipts)
	if err != nil {
		t.Fatalf("read repaired evaluation history: %v", err)
	}
	if got, want := len(parsedReceipts), 1; got != want {
		t.Fatalf("repaired receipts = %d, want %d", got, want)
	}
	if parsedReceipts[0].Round != 1 || parsedReceipts[0].Verdict != "fail" {
		t.Fatalf("repaired receipt = %#v, want failed round 1", parsedReceipts[0])
	}
	if got, want := evaluationFailureCount(parsedReceipts), 1; got != want {
		t.Fatalf("failed-round count = %d, want %d", got, want)
	}
	repair, ok := parseEvaluationRepair(backend.comments[2].Body)
	if !ok {
		t.Fatal("generated repair marker was not parseable")
	}
	receiptMarker, ok := markerBytes(backend.comments[1].Body, evaluationMarker)
	if !ok {
		t.Fatal("repaired receipt marker was not parseable")
	}
	_, rawAttestation, canonicalReport, ok := parseRepairEvidence(t, backend.comments[1].Body)
	if !ok {
		t.Fatal("repaired attestation evidence was not parseable")
	}
	if repair.OriginalCommentSHA256 != sha256Hex([]byte(backend.comments[1].Body)) ||
		repair.ReceiptMarkerSHA256 != sha256Hex(receiptMarker) ||
		repair.AttestationSHA256 != sha256Hex(rawAttestation) ||
		repair.ReportSHA256 != sha256Hex(canonicalReport) ||
		repair.PR != 14 || repair.Head != backend.head || repair.Round != 1 || repair.Verdict != "fail" {
		t.Fatalf("repair bindings = %#v", repair)
	}
}

func recordSecondEvaluation(t *testing.T, application *app, backend *workflowBackend, stdout *bytes.Buffer) {
	t.Helper()
	stdout.Reset()
	secondChallenge := requestTestChallenge(t, application, stdout)
	_, secondAttestationFile := writeTestAttestationRun(t, backend.head, secondChallenge, "examiner-second-round")
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", secondAttestationFile}); err != nil {
		t.Fatalf("record evaluation after repair: %v", err)
	}
	secondReceipt, ok := parseEvaluationReceipt(backend.comments[len(backend.comments)-1].Body)
	if !ok {
		t.Fatal("second evaluation receipt was not parseable")
	}
	if secondReceipt.Round != 2 {
		t.Fatalf("second evaluation round = %d, want 2 after repaired round 1", secondReceipt.Round)
	}
}

func TestEvaluationRepairRejectsUnsafeEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, backend *workflowBackend)
	}{
		{name: "intact", mutate: func(_ *testing.T, _ *workflowBackend) {}},
		{name: "malformed receipt marker", mutate: func(_ *testing.T, backend *workflowBackend) {
			body := backend.comments[1].Body
			body = strings.Replace(body, " -->", " --x>", 1)
			backend.comments[1].Body = body
		}},
		{name: "tampered attestation", mutate: func(t *testing.T, backend *workflowBackend) {
			body := backend.comments[1].Body
			value, ok := markerBytes(body, evaluationAttestationBase64Marker)
			if !ok {
				t.Fatal("attestation marker missing")
			}
			body = strings.Replace(body, string(value), base64.StdEncoding.EncodeToString([]byte("{}")), 1)
			backend.comments[1].Body = body
		}},
		{name: "tampered receipt", mutate: func(t *testing.T, backend *workflowBackend) {
			backend.comments[1].Body = replaceTestReceipt(t, backend.comments[1].Body, func(receipt *evaluationReceipt) {
				receipt.ReportSHA256 = strings.Repeat("0", sha256.Size*2)
			})
		}},
		{name: "wrong head", mutate: func(t *testing.T, backend *workflowBackend) {
			backend.comments[1].Body = replaceTestReceipt(t, backend.comments[1].Body, func(receipt *evaluationReceipt) {
				receipt.Head = "another-head"
			})
		}},
		{name: "wrong PR", mutate: func(t *testing.T, backend *workflowBackend) {
			backend.comments[1].Body = replaceTestReceipt(t, backend.comments[1].Body, func(receipt *evaluationReceipt) {
				receipt.PR = 99
			})
		}},
		{name: "duplicate receipt", mutate: func(_ *testing.T, backend *workflowBackend) {
			backend.comments = append(backend.comments, backend.comments[1])
		}},
		{name: "untrusted receipt", mutate: func(_ *testing.T, backend *workflowBackend) {
			backend.comments[1].User.Login = owner
		}},
		{name: "orphan repair", mutate: func(t *testing.T, backend *workflowBackend) {
			orphan := testRepairComment(t, backend.comments[1].Body, func(repair *evaluationRepair) {
				repair.Round = 99
			})
			backend.comments = append(backend.comments, orphan)
		}},
		{name: "repair before receipt", mutate: func(t *testing.T, backend *workflowBackend) {
			backend.comments[1].Body = legacyTransportMismatch(t, backend.comments[1].Body)
			repair := testRepairComment(t, backend.comments[1].Body, nil)
			repair.CreatedAt = backend.comments[1].CreatedAt.Add(-time.Second)
			backend.comments = append(backend.comments, repair)
		}},
		{name: "wrong repair binding", mutate: func(t *testing.T, backend *workflowBackend) {
			wrong := testRepairComment(t, backend.comments[1].Body, func(repair *evaluationRepair) {
				repair.OriginalCommentSHA256 = strings.Repeat("f", sha256.Size*2)
			})
			backend.comments = append(backend.comments, wrong)
		}},
		{name: "duplicate repair", mutate: func(t *testing.T, backend *workflowBackend) {
			backend.comments[1].Body = legacyTransportMismatch(t, backend.comments[1].Body)
			repair := testRepairComment(t, backend.comments[1].Body, nil)
			backend.comments = append(backend.comments, repair, repair)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend, application, _ := newRepairFixture(t)
			test.mutate(t, backend)
			commentCount := len(backend.comments)
			if err := application.runEvaluation([]string{"repair", "14", "--round", "1"}); err == nil {
				t.Fatal("unsafe evaluation evidence was accepted")
			}
			if len(backend.comments) != commentCount {
				t.Fatal("rejected repair appended a comment")
			}
		})
	}
}

func TestUnknownMergeOutcomeGuidesRecovery(t *testing.T) {
	ambiguous := errors.New("merge endpoint transport failed")
	reconciliation := errors.New("PR is still open")
	err := mergeOutcomeUnknownError(14, ambiguous, reconciliation)
	if !strings.Contains(err.Error(), "merge state is unknown") || !strings.Contains(err.Error(), "go tool workflowctl pr recover 14") {
		t.Fatalf("mergeOutcomeUnknownError = %v, want recovery guidance", err)
	}
	if !errors.Is(err, ambiguous) || !errors.Is(err, reconciliation) {
		t.Fatalf("mergeOutcomeUnknownError = %v, want both causes", err)
	}
}

func newRepairFixture(t *testing.T) (*workflowBackend, *app, *bytes.Buffer) {
	t.Helper()
	backend := newWorkflowBackend(t)
	backend.localBranch = claimLocalBranch(13, "run-test")
	stdout := new(bytes.Buffer)
	application := &app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         stdout,
		stderr:                         new(bytes.Buffer),
	}
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestFailureAttestation(t, backend.head, challenge)
	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record repair fixture evaluation: %v", err)
	}
	return backend, application, stdout
}

func legacyTransportMismatch(t *testing.T, body string) string {
	t.Helper()
	receiptMarker, ok := markerBytes(body, evaluationMarker)
	if !ok {
		t.Fatal("repair fixture receipt marker missing")
	}
	var receipt evaluationReceipt
	if err := json.Unmarshal(receiptMarker, &receipt); err != nil {
		t.Fatalf("decode repair fixture receipt: %v", err)
	}
	receipt.ReportTransport = ""
	legacyReceiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode legacy repair fixture receipt: %v", err)
	}
	body = strings.Replace(body, string(receiptMarker), string(legacyReceiptMarker), 1)
	reportMarker, ok := markerBytes(body, evaluationReportBase64Marker)
	if !ok {
		t.Fatal("repair fixture report marker missing")
	}
	reportComment := fmt.Sprintf("<!-- %s%s -->\n", evaluationReportBase64Marker, reportMarker)
	body = strings.Replace(body, reportComment, "", 1)
	body = strings.Replace(body, `\u001e`, `\^^`, 1)
	return body
}

func replaceTestReceipt(t *testing.T, body string, mutate func(*evaluationReceipt)) string {
	t.Helper()
	value, ok := markerBytes(body, evaluationMarker)
	if !ok {
		t.Fatal("receipt marker missing")
	}
	var receipt evaluationReceipt
	if err := json.Unmarshal(value, &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	mutate(&receipt)
	replacement, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	return strings.Replace(body, string(value), string(replacement), 1)
}

func testRepairComment(t *testing.T, body string, mutate func(*evaluationRepair)) issueCommentAPI {
	t.Helper()
	receiptMarker, ok := markerBytes(body, evaluationMarker)
	if !ok {
		t.Fatal("repair test receipt marker missing")
	}
	var receipt evaluationReceipt
	if err := json.Unmarshal(receiptMarker, &receipt); err != nil {
		t.Fatalf("decode repair test receipt: %v", err)
	}
	attestation, rawAttestation, canonicalReport, ok := parseRepairEvidence(t, body)
	if !ok {
		t.Fatal("decode repair test attestation")
	}
	if attestation.PR != receipt.PR || attestation.Head != receipt.Head ||
		attestation.Verdict != receipt.Verdict {
		t.Fatal("repair test attestation does not match receipt")
	}
	repair := evaluationRepair{
		AttestationSHA256:     sha256Hex(rawAttestation),
		Challenge:             receipt.Challenge,
		Evaluator:             receipt.Evaluator,
		EvaluatorRunID:        receipt.EvaluatorRunID,
		Head:                  receipt.Head,
		OriginalCommentSHA256: sha256Hex([]byte(body)),
		ReceiptMarkerSHA256:   sha256Hex(receiptMarker),
		PR:                    receipt.PR,
		ReportSHA256:          sha256Hex(canonicalReport),
		Round:                 receipt.Round,
		Schema:                evaluationRepairSchema,
		Verdict:               receipt.Verdict,
	}
	if mutate != nil {
		mutate(&repair)
	}
	marker, err := json.Marshal(repair)
	if err != nil {
		t.Fatalf("encode repair test marker: %v", err)
	}
	comment := issueCommentAPI{Body: evaluationRepairComment(marker, repair.Round), CreatedAt: time.Now().UTC()}
	comment.User.Login = trustedActor
	return comment
}

func parseRepairEvidence(t *testing.T, body string) (evaluationAttestation, []byte, []byte, bool) {
	t.Helper()
	attestation, rawAttestation, ok := parseCommentAttestation(body)
	if !ok {
		return evaluationAttestation{}, nil, nil, false
	}
	return attestation, rawAttestation, canonicalEvaluationReport(renderEvaluationReport(attestation)), true
}

func pullRequestCommentsFromAPI(t *testing.T, comments []issueCommentAPI) []pullRequestComment {
	t.Helper()
	converted := make([]pullRequestComment, 0, len(comments))
	for _, comment := range comments {
		convertedComment := pullRequestComment{ID: comment.ID, Body: comment.Body, CreatedAt: comment.CreatedAt}
		convertedComment.Author.Login = comment.User.Login
		converted = append(converted, convertedComment)
	}
	return converted
}

func reusedRunComments(t *testing.T, head, runID string) []issueCommentAPI {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	challenge := evaluationChallenge{Challenge: "duplicate-run-challenge", Head: head, PR: 14, RequestedAt: now}
	challengeJSON, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode reused-run challenge: %v", err)
	}
	challengeComment := issueCommentAPI{
		Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeJSON), CreatedAt: now,
	}
	challengeComment.User.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: evaluationFindings{}, Head: head, PR: 14,
		RunID: runID, Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode reused-run attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationJSON)),
		Challenge:         challenge.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    runID,
		Head:              head,
		PR:                14,
		RecordedAt:        now,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             2,
		Verdict:           "pass",
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode reused-run receipt: %v", err)
	}
	body := evaluationComment(receiptJSON, attestationJSON, report)
	reportMarker, ok := markerBytes(body, evaluationReportBase64Marker)
	if !ok {
		t.Fatal("legacy evaluation fixture report marker missing")
	}
	reportComment := fmt.Sprintf("<!-- %s%s -->\n", evaluationReportBase64Marker, reportMarker)
	body = strings.Replace(body, reportComment, "", 1)
	receiptComment := issueCommentAPI{Body: body, CreatedAt: now}
	receiptComment.User.Login = trustedActor
	return []issueCommentAPI{challengeComment, receiptComment}
}

func rejectTamperedReceiptReuse(t *testing.T, application *app, backend *workflowBackend, attestationFile string) {
	t.Helper()
	original := backend.comments[1].Body
	backend.comments[1].Body = strings.Replace(original, "No blocking findings", "Changed findings", 1)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil {
		t.Fatal("tampered history allowed evaluation evidence reuse")
	}
	backend.comments[1].Body = original
}

func rejectLaterTamperedReceipt(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	tampered := backend.comments[1]
	tampered.Body = strings.Replace(tampered.Body, "No blocking findings", "Changed findings", 1)
	tampered.CreatedAt = time.Now().UTC().Truncate(time.Second)
	backend.comments = append(backend.comments, tampered)
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("later tampered receipt fell back to an earlier pass")
	}
	backend.comments = backend.comments[:len(backend.comments)-1]
}

func rejectInvalidPullRequestTitle(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	original := backend.title
	backend.title = "Invalid title"
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted an invalid pull request title")
	}
	if backend.merged {
		t.Fatal("invalid pull request title reached the merge endpoint")
	}
	backend.title = original
}

func rejectInvalidWorkCommitTitle(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	original := backend.workCommitLog
	backend.workCommitLog = framedRawCommitLog(
		"fix(parser): reject invalid XML\ncontinue\n", "chore(workflow): claim issue #13\n")
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted a work commit added after PR creation with an invalid title")
	}
	if backend.merged {
		t.Fatal("invalid work commit title reached the merge endpoint")
	}
	backend.workCommitLog = original
}

func requestTestChallenge(t *testing.T, application *app, stdout *bytes.Buffer) evaluationChallenge {
	t.Helper()
	if err := application.runEvaluation([]string{"challenge", "14"}); err != nil {
		t.Fatalf("create evaluation challenge: %v", err)
	}
	var challenge evaluationChallenge
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &challenge); err != nil {
		t.Fatalf("decode challenge output: %v", err)
	}
	return challenge
}

func writeTestAttestation(t *testing.T, head string, challenge evaluationChallenge) ([]byte, string) {
	t.Helper()
	return writeTestAttestationValue(t, evaluationAttestation{
		Challenge: challenge.Challenge,
		Evaluator: "Examiner",
		Findings:  evaluationFindings{},
		Head:      head,
		PR:        14,
		RunID:     testExaminerRunID,
		Schema:    evaluationAttestationSchema,
		Summary:   "No blocking findings; delimiter --> remains data.",
		Verdict:   "pass",
	})
}

func writeTestFailureAttestation(t *testing.T, head string, challenge evaluationChallenge) ([]byte, string) {
	return writeTestFailureAttestationRun(t, head, challenge, "examiner-failed-round")
}

func writeTestFailureAttestationRun(t *testing.T, head string, challenge evaluationChallenge, runID string) ([]byte, string) {
	t.Helper()
	return writeTestAttestationValue(t, evaluationAttestation{
		Challenge: challenge.Challenge,
		Evaluator: "Examiner",
		Findings: evaluationFindings{{
			Impact:             "The transport projection changed.",
			Location:           "internal/workflowctl/evaluation.go:1",
			RequiredCorrection: "Preserve the canonical report marker.",
		}},
		Head:    head,
		PR:      14,
		RunID:   runID,
		Schema:  evaluationAttestationSchema,
		Summary: "Blocking finding; literal \\u001e; delimiter --> remains data.",
		Verdict: "fail",
	})
}

func writeTestAttestationRun(t *testing.T, head string, challenge evaluationChallenge, runID string) ([]byte, string) {
	t.Helper()
	return writeTestAttestationValue(t, evaluationAttestation{
		Challenge: challenge.Challenge,
		Evaluator: "Examiner",
		Findings:  evaluationFindings{},
		Head:      head,
		PR:        14,
		RunID:     runID,
		Schema:    evaluationAttestationSchema,
		Summary:   "No blocking findings; delimiter --> remains data.",
		Verdict:   "pass",
	})
}

func writeTestAttestationValue(t *testing.T, attestation evaluationAttestation) ([]byte, string) {
	t.Helper()
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(attestation); err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	attestationJSON := encoded.Bytes()
	if !bytes.Contains(attestationJSON, []byte(" -->")) {
		t.Fatal("attestation fixture does not contain the literal comment delimiter")
	}
	path := filepath.Join(t.TempDir(), "attestation.json")
	if err := os.WriteFile(path, attestationJSON, 0o600); err != nil {
		t.Fatalf("write attestation: %v", err)
	}
	return attestationJSON, path
}

func checkRecordedAttestation(t *testing.T, comments []issueCommentAPI, attestationJSON []byte) {
	t.Helper()
	if got, want := len(comments), 2; got != want {
		t.Fatalf("comments = %d, want %d", got, want)
	}
	for index, comment := range comments {
		if comment.User.Login != trustedActor {
			t.Fatalf("comment %d author = %q, want %q", index, comment.User.Login, trustedActor)
		}
	}
	_, recovered, ok := parseCommentAttestation(comments[1].Body)
	if !ok || !bytes.Equal(recovered, attestationJSON) {
		t.Fatal("recorded comment did not recover the exact Examiner attestation bytes")
	}
	receipt, ok := parseEvaluationReceipt(comments[1].Body)
	if !ok {
		t.Fatal("recorded evaluation receipt is invalid")
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(attestationJSON))
	if receipt.AttestationSHA256 != wantHash {
		t.Fatalf("attestation hash = %s, want %s", receipt.AttestationSHA256, wantHash)
	}
}

func checkMergeResult(t *testing.T, backend *workflowBackend) {
	t.Helper()
	if !backend.merged || !backend.projectDone {
		t.Fatalf("merge state = %t, Project Done = %t", backend.merged, backend.projectDone)
	}
	if backend.issueState != "closed" {
		t.Fatalf("primary issue state = %q, want closed", backend.issueState)
	}
	if backend.mergeRequest.SHA != backend.head || backend.mergeRequest.MergeMethod != "squash" {
		t.Fatalf("merge request = %#v", backend.mergeRequest)
	}
	if backend.mergeRequest.CommitTitle != backend.title+" (#14)" {
		t.Fatalf("merge commit title = %q", backend.mergeRequest.CommitTitle)
	}
	if backend.mergeRequest.CommitMessage != backend.summary {
		t.Fatalf("merge commit message = %q, want %q", backend.mergeRequest.CommitMessage, backend.summary)
	}
	if strings.Contains(backend.mergeRequest.CommitMessage, "Agent-Run-ID") {
		t.Fatal("claim metadata leaked into the squash commit message")
	}
}

func newPassingFinishFixture(t *testing.T) (*workflowBackend, *app) {
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
	challenge := requestTestChallenge(t, application, stdout)
	_, attestationFile := writeTestAttestation(t, backend.head, challenge)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record evaluation: %v", err)
	}
	stdout.Reset()
	return backend, application
}

func TestFinishAlreadyClosedPrimarySkipsExplicitClose(t *testing.T) {
	backend, application := newPassingFinishFixture(t)
	backend.issueState = "closed"
	if err := application.runPR(backend.finishArgs()); err != nil {
		t.Fatalf("finish already-closed primary: %v", err)
	}
	checkMergeResult(t, backend)
	if backend.issuePatchCount != 0 {
		t.Fatalf("already-closed primary received %d PATCH attempts", backend.issuePatchCount)
	}
}

func TestFinishRetriesAmbiguousPrimaryCloseThroughRecovery(t *testing.T) {
	backend, application := newPassingFinishFixture(t)
	backend.issuePatchMode = "transport"
	err := application.runPR(backend.finishArgs())
	if err == nil || !strings.Contains(err.Error(), "primary issue reconciliation") || !strings.Contains(err.Error(), "pr recover 14") {
		t.Fatalf("ambiguous primary close error = %v, want receipt-bound recovery guidance", err)
	}
	if !backend.merged || backend.issuePatchCount != 1 || backend.projectDone {
		t.Fatalf("ambiguous primary close state = merged %t, PATCH %d, Project Done %t", backend.merged, backend.issuePatchCount, backend.projectDone)
	}
	backend.issuePatchMode = ""
	if err := application.recoverPullRequest(14); err != nil {
		t.Fatalf("recover after ambiguous primary close: %v", err)
	}
	checkMergeResult(t, backend)
	if backend.issuePatchCount != 1 {
		t.Fatalf("primary issue PATCH count = %d, want one explicit close", backend.issuePatchCount)
	}
}

func TestFinishRecoversAfterPrimaryReadFailure(t *testing.T) {
	backend, application := newPassingFinishFixture(t)
	backend.issueReadFailures = 1
	err := application.runPR(backend.finishArgs())
	if err == nil || !strings.Contains(err.Error(), "primary issue reconciliation") || !strings.Contains(err.Error(), "pr recover 14") {
		t.Fatalf("primary read failure = %v, want recovery guidance", err)
	}
	if !backend.merged || backend.issuePatchCount != 0 || backend.projectDone {
		t.Fatalf("primary read failure state = merged %t, PATCH %d, Project Done %t", backend.merged, backend.issuePatchCount, backend.projectDone)
	}
	if err := application.recoverPullRequest(14); err != nil {
		t.Fatalf("recover after primary read failure: %v", err)
	}
	checkMergeResult(t, backend)
}

func TestReadEvaluationAttestationRejectsMalformedEvidence(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "unknown field", json: `{"schema":"goxsd9/examiner-attestation/v1","extra":true}`},
		{name: "trailing value", json: `{}` + "\n" + `{}`},
		{name: "null findings", json: `{"findings":null}`},
	}
	for _, test := range tests {
		path := filepath.Join(t.TempDir(), "attestation.json")
		if err := os.WriteFile(path, []byte(test.json), 0o600); err != nil {
			t.Fatalf("%s: write attestation: %v", test.name, err)
		}
		if _, _, err := readEvaluationAttestation(path); err == nil {
			t.Fatalf("%s attestation was accepted", test.name)
		}
	}
}

func TestEvaluationAttestationRejectsReusedExaminerRun(t *testing.T) {
	now := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "fresh-challenge", Head: "head", PR: 14, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: now}
	comment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{comment}, HeadRefOID: "head"}
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: evaluationFindings{}, Head: "head", PR: 14,
		RunID: "examiner-reused", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	receipts := []evaluationReceipt{{Challenge: "earlier-challenge", EvaluatorRunID: attestation.RunID, Verdict: "fail"}}
	if err := validateEvaluationAttestation(attestation, 14, view, receipts, now); err == nil {
		t.Fatal("reused Examiner run ID was accepted")
	}
}

func TestEvaluationChallengeRejectsStaleOrUntrustedComments(t *testing.T) {
	now := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		requested time.Time
		created   time.Time
		author    string
	}{
		{name: "stale", requested: now.Add(-evaluationChallengeDuration - time.Second), created: now.Add(-evaluationChallengeDuration), author: trustedActor},
		{name: "future", requested: now.Add(time.Second), created: now.Add(time.Second), author: trustedActor},
		{name: "untrusted", requested: now, created: now, author: "other-user"},
		{name: "timestamp mismatch", requested: now, created: now.Add(6 * time.Minute), author: trustedActor},
	}
	for _, test := range tests {
		challenge := evaluationChallenge{
			Challenge: "challenge", Head: "head", PR: 14, RequestedAt: test.requested,
		}
		marker, err := json.Marshal(challenge)
		if err != nil {
			t.Fatalf("%s: encode challenge: %v", test.name, err)
		}
		comment := pullRequestComment{
			Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: test.created,
		}
		comment.Author.Login = test.author
		if _, ok := trustedEvaluationChallenge([]pullRequestComment{comment}, "challenge", 14, "head", now); ok {
			t.Fatalf("%s challenge was trusted", test.name)
		}
	}
}

func TestEvaluationChallengeExpiryBoundary(t *testing.T) {
	now := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		requested time.Time
		want      bool
	}{
		{name: "just before expiry", requested: now.Add(-evaluationChallengeDuration + time.Nanosecond), want: true},
		{name: "at expiry", requested: now.Add(-evaluationChallengeDuration), want: false},
		{name: "after expiry", requested: now.Add(-evaluationChallengeDuration - time.Nanosecond), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			challenge := evaluationChallenge{
				Challenge: "boundary-challenge", Head: "head", PR: 14, RequestedAt: test.requested,
			}
			marker, err := json.Marshal(challenge)
			if err != nil {
				t.Fatalf("encode challenge: %v", err)
			}
			comment := pullRequestComment{
				Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: test.requested,
			}
			comment.Author.Login = trustedActor
			_, ok := trustedEvaluationChallenge([]pullRequestComment{comment}, challenge.Challenge, challenge.PR,
				challenge.Head, now)
			if ok != test.want {
				t.Fatalf("challenge trusted = %t, want %t", ok, test.want)
			}
		})
	}
}

func TestClaimReadsIssueStateWhenProjectContentOmitsIt(t *testing.T) {
	tests := []struct {
		name    string
		issue   string
		wantErr bool
	}{
		{name: "open", issue: `{"state":"open","labels":[]}`},
		{name: "closed", issue: `{"state":"closed","labels":[]}`, wantErr: true},
		{name: "needs human", issue: `{"state":"open","labels":[{"name":"needs-human"}]}`, wantErr: true},
	}
	for _, test := range tests {
		application := app{executeCommand: claimStateCommand(t, test.issue)}
		err := application.assertClaimable("/repo", 13)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: assertClaimable error = %v, want error %t", test.name, err, test.wantErr)
		}
	}
}

func claimStateCommand(t *testing.T, issue string) commandExecutor {
	t.Helper()
	return func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "gh project item-list 1 --owner goxdra --format json --limit 500":
			return `{"items":[{"content":{"number":13,"repository":"goxdra/goxsd9","type":"Issue"},"id":"item-13","status":"Ready"}]}`, nil
		case "gh api repos/goxdra/goxsd9/issues/13":
			return issue, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}
}

type workflowBackend struct {
	t                                *testing.T
	root                             string
	primaryRoot                      string
	branch                           string
	localBranch                      string
	head                             string
	title                            string
	body                             string
	summary                          string
	summaryFile                      string
	workCommitLog                    string
	comments                         []issueCommentAPI
	commentPostCount                 int
	duplicateReceiptPost             bool
	duplicateConvergencePost         bool
	duplicateClosurePost             bool
	postCommentResponseMode          string
	postCommentAuthor                string
	closePRBeforeNextCommentPost     bool
	duplicateChallengeOnNextPost     bool
	duplicateChallengeBeforeNextPost bool
	needsHuman                       bool
	projectMember                    bool
	projectItemID                    string
	projectType                      string
	projectRepository                string
	projectNumber                    int
	projectMalformed                 bool
	projectTransitionFailures        int
	projectTransitionAttempts        int
	mergeRequest                     mergePullRequestRequest
	merged                           bool
	projectDone                      bool
	projectStatus                    string
	issueState                       string
	issueReadFailures                int
	issuePatchCount                  int
	issuePatchMode                   string
	mergeResponseMode                string
	mutatePRBodyAfterMerge           bool
	delayReceiptAfterMerge           bool
	managedDocumentChange            bool
	bodyPatchCount                   int
	mergeSHA                         string
	mergedAt                         time.Time
	removeSummaryOnNextCommand       bool
}

func newWorkflowBackend(t *testing.T) *workflowBackend {
	t.Helper()
	fixtureRoot := t.TempDir()
	primaryRoot := filepath.Join(fixtureRoot, "primary")
	root := claimWorktreePath(primaryRoot, "agent/issue-13")
	for _, path := range []string{primaryRoot, root} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("create workflow fixture worktree %q: %v", path, err)
		}
	}
	summary := "GitHub currently derives squash bodies from branch commits, so claim\n" +
		"renewals obscure the implementation outcome. Send this reviewed summary\n" +
		"explicitly so future workflow sessions receive the durable rationale."
	summaryFile := filepath.Join(t.TempDir(), "summary.txt")
	if err := os.WriteFile(summaryFile, []byte(summary+"\n"), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	body := testPRReviewStateFrame(t, prReviewStateEvidenceReady) + "\n## Outcome\n\nExercise evaluation flow.\n\n## Work packet\n\nCloses #13\n"
	evidence := testWorkflowPREvidence("evaluated-head")
	block, err := renderPREvidenceBlock(evidence)
	if err != nil {
		t.Fatalf("render workflow PR evidence: %v", err)
	}
	body += "\n" + string(block)
	return &workflowBackend{
		t: t, root: root, primaryRoot: primaryRoot, branch: "agent/issue-13", localBranch: "agent/issue-13", head: "evaluated-head",
		body:              body,
		summary:           summary,
		summaryFile:       summaryFile,
		title:             "test(workflow): exercise evaluation flow",
		workCommitLog:     framedCommitLog("test(workflow): exercise evaluation flow", "chore(workflow): claim issue #13"),
		mergeSHA:          "merge-commit",
		issueState:        "open",
		projectMember:     true,
		projectItemID:     "item-13",
		projectType:       "Issue",
		projectRepository: repositoryKey,
		projectNumber:     13,
	}
}

func testWorkflowPREvidence(head string) prEvidence {
	base := "base-sha"
	packages := []coveragePackageReport{}
	coverage := coverageReport{
		Base: base, Head: head, Packages: packages,
		Affected: coverageTotals(packages, true), Repository: coverageTotals(packages, false),
	}
	return prEvidence{
		Schema: prEvidenceSchema, Base: base, Head: head,
		DevelopmentSignals: developmentSignalsReport{
			Schema: developmentSignalsSchema, Base: base, Head: head, Coverage: coverage,
			CoverageExplanations: []coverageExplanation{}, Fuzz: []signalFuzzReport{}, AdditionalFuzz: []additionalFuzzReport{}, Selection: "no-relevant-target",
			Catalog: noMeasuredDevelopmentSignal, XSDFeatureSupport: noMeasuredDevelopmentSignal,
			ExecutableConformance: noMeasuredDevelopmentSignal,
		},
		DocumentationAudit: documentationAuditReport{
			Schema: documentationAuditSchema, Base: base, Head: head, MergeBase: "merge-sha",
			ManagedChanges: []documentationChangeReport{}, EvaluationFixtures: []string{},
		},
		Curator: noCuratorResult(head),
	}
}

func (b *workflowBackend) finishArgs() []string {
	return []string{"finish", "14", "--summary-file", b.summaryFile}
}

func (b *workflowBackend) execute(dir string, input io.Reader, name string, args ...string) (string, error) {
	b.t.Helper()
	if b.removeSummaryOnNextCommand {
		if err := os.Remove(b.summaryFile); err != nil {
			return "", fmt.Errorf("remove summary after validation: %w", err)
		}
		b.removeSummaryOnNextCommand = false
	}
	var data []byte
	if input != nil {
		var err error
		data, err = io.ReadAll(input)
		if err != nil {
			return "", fmt.Errorf("read command input: %w", err)
		}
	}
	if name == "git" {
		return b.executeGit(dir, args)
	}
	if name == "gh" {
		return b.executeGitHub(data, args)
	}
	return "", fmt.Errorf("unexpected command in %s: %s %s", dir, name, strings.Join(args, " "))
}

func (b *workflowBackend) executeGit(dir string, args []string) (string, error) {
	command := strings.Join(args, " ")
	if output, ok := b.executeGitBase(dir, command); ok {
		return output, nil
	}
	return b.executeGitClaim(dir, command)
}

func (b *workflowBackend) executeGitBase(dir, command string) (string, bool) {
	if output, ok := b.executeGitArtifact(command); ok {
		return output, true
	}
	if dir == b.primaryRoot && command == "rev-parse HEAD" {
		return b.mergeSHA, true
	}
	switch command {
	case "rev-parse --show-toplevel":
		return b.root, true
	case "rev-parse --path-format=absolute --git-common-dir":
		return filepath.Join(b.primaryRoot, ".git"), true
	case "worktree list --porcelain":
		return fmt.Sprintf("worktree %s\nHEAD merge-commit\nbranch refs/heads/main\n\n"+
			"worktree %s\nHEAD evaluated-head\nbranch refs/heads/agent/issue-13\n", b.primaryRoot, b.root), true
	case "-C " + b.primaryRoot + " rev-parse --path-format=absolute --git-dir":
		return filepath.Join(b.primaryRoot, ".git"), true
	case "-C " + b.root + " rev-parse --path-format=absolute --git-dir":
		return filepath.Join(b.primaryRoot, ".git", "worktrees", filepath.Base(b.root)), true
	case "-C " + b.root + " status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
		return "", true
	case "status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
		return "", true
	case "branch --show-current":
		if dir == b.primaryRoot {
			return "main", true
		}
		return b.localBranch, true
	case "fetch origin main":
		return "", true
	case "rev-parse origin/main":
		return b.mergeSHA, true
	case "rev-list --left-right --count HEAD...origin/main":
		return "0 0", true
	case "config --get-regexp ^submodule\\..*\\.update$":
		return "", true
	case "merge-base --is-ancestor merge-commit merge-commit":
		return "", true
	case "submodule update --init --recursive", "submodule status --recursive", "submodule foreach --recursive --quiet git status --porcelain=v1 --untracked-files=all":
		return "", true
	case "merge --ff-only origin/main":
		return "", true
	case "worktree remove --force " + b.root:
		return "", true
	case "ls-remote --heads origin refs/heads/agent/issue-13":
		return "evaluated-head refs/heads/agent/issue-13", true
	case "push --force-with-lease=refs/heads/agent/issue-13:evaluated-head origin :refs/heads/agent/issue-13":
		return "", true
	case "for-each-ref --format=%(objectname) refs/remotes/origin/agent/issue-13", "for-each-ref --format=%(objectname) refs/heads/agent/issue-13":
		return "evaluated-head", true
	case "update-ref -d refs/remotes/origin/agent/issue-13 evaluated-head", "update-ref -d refs/heads/agent/issue-13 evaluated-head":
		return "", true
	default:
		return "", false
	}
}

func (b *workflowBackend) executeGitArtifact(command string) (string, bool) {
	if command == "merge-base --is-ancestor "+b.head+" "+b.head {
		return "", true
	}
	if command == "cat-file -e "+b.head+"^{commit}" {
		return "", true
	}
	if command == "merge-base base-sha "+b.head {
		return "merge-sha", true
	}
	if command == "diff --name-status -z --no-renames merge-sha "+b.head+" --" {
		if b.managedDocumentChange {
			return "D\x00README.md\x00", true
		}
		return "", true
	}
	if command == "diff --numstat -z --no-renames merge-sha "+b.head+" --" {
		if b.managedDocumentChange {
			return "0\t1\tREADME.md\x00", true
		}
		return "", true
	}
	switch command {
	case "status --porcelain":
		return "", true
	case "merge-base --is-ancestor evaluated-head evaluated-head":
		return "", true
	case "cat-file -e evaluated-head^{commit}", "fetch --no-tags origin refs/pull/14/head":
		return "", true
	case "merge-base base-sha evaluated-head":
		return "merge-sha", true
	case "diff --name-status -z --no-renames merge-sha evaluated-head --":
		if b.managedDocumentChange {
			return "D\x00README.md\x00", true
		}
		return "", true
	case "diff --numstat -z --no-renames merge-sha evaluated-head --":
		if b.managedDocumentChange {
			return "0\t1\tREADME.md\x00", true
		}
		return "", true
	case "ls-remote --heads origin refs/heads/agent/*":
		return "evaluated-head refs/heads/agent/issue-13", true
	case "for-each-ref --format=%(refname:short) %(objectname) refs/remotes/origin/agent/issue-*":
		return "origin/agent/issue-13 evaluated-head", true
	case "for-each-ref --format=%(refname:short) %(objectname) refs/heads/agent/issue-*":
		return "agent/issue-13 evaluated-head", true
	default:
		return "", false
	}
}

func (b *workflowBackend) executeGitClaim(dir, command string) (string, error) {
	switch command {
	case "fetch origin refs/heads/agent/issue-13:refs/remotes/origin/agent/issue-13":
		return "", nil
	case "rev-parse HEAD", "rev-parse origin/agent/issue-13":
		return b.head, nil
	case "rev-parse --verify --end-of-options HEAD^{commit}":
		return b.head, nil
	case "rev-parse --verify --end-of-options base-sha^{commit}":
		return "base-sha", nil
	case "log -100 --format=%B":
		lease := time.Now().UTC().Add(claimDuration).Truncate(time.Second)
		return claimMessage(13, "run-test", lease), nil
	case "log --format=%x00%B%x00 origin/main.." + b.head:
		return b.workCommitLog, nil
	default:
		return "", fmt.Errorf("unexpected git command: %s in %s", command, dir)
	}
}

func (b *workflowBackend) executeGitHub(input []byte, args []string) (string, error) {
	joined := strings.Join(args, " ")
	if output, handled, err := b.executeGitHubIssue13(input, joined); handled {
		return output, err
	}
	if strings.HasPrefix(joined, "api --paginate repos/goxdra/goxsd9/commits/") &&
		strings.HasSuffix(joined, "/check-runs?per_page=100") {
		return "{\"check_runs\":[{\"conclusion\":\"success\",\"name\":\"docs\",\"status\":\"completed\"}]}" +
			"{\"check_runs\":[{\"conclusion\":\"success\",\"name\":\"quality\",\"status\":\"completed\"}]}", nil
	}
	switch joined {
	case "api repos/goxdra/goxsd9/pulls/14":
		return b.pullRequestJSON()
	case "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100":
		return b.commentsJSON()
	case "api --method POST repos/goxdra/goxsd9/issues/14/comments --input -":
		return b.postComment(input)
	case "api --method PATCH repos/goxdra/goxsd9/pulls/14 --input -":
		var request struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(input, &request); err != nil {
			return "", fmt.Errorf("decode body update request: %w", err)
		}
		b.body = request.Body
		b.bodyPatchCount++
		return `{}`, nil
	case "api --method PATCH repos/goxdra/goxsd9/issues/13 --input -":
		var request struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(input, &request); err != nil {
			return "", fmt.Errorf("decode issue close request: %w", err)
		}
		if request.State != "closed" {
			return "", fmt.Errorf("unexpected issue state mutation %q", request.State)
		}
		b.issuePatchCount++
		b.issueState = "closed"
		if b.issuePatchMode == "transport" {
			return "", errors.New("simulated lost issue close response")
		}
		return `{"state":"closed"}`, nil
	case "api --paginate repos/goxdra/goxsd9/commits/evaluated-head/check-runs?per_page=100":
		return "{\"check_runs\":[{\"conclusion\":\"success\",\"name\":\"docs\",\"status\":\"completed\"}]}" +
			"{\"check_runs\":[{\"conclusion\":\"success\",\"name\":\"quality\",\"status\":\"completed\"}]}", nil
	case "api --method PUT repos/goxdra/goxsd9/pulls/14/merge --input -":
		return b.merge(input)
	default:
		return "", fmt.Errorf("unexpected gh command: %s", joined)
	}
}

func (b *workflowBackend) executeGitHubIssue13(_ []byte, joined string) (string, bool, error) {
	switch joined {
	case "api repos/goxdra/goxsd9/issues/13":
		if b.issueReadFailures > 0 {
			b.issueReadFailures--
			return "", true, errors.New("simulated primary issue read failure")
		}
		labels := "[]"
		if b.needsHuman {
			labels = `[{"name":"needs-human"}]`
		}
		return fmt.Sprintf(`{"state":%q,"labels":%s}`, b.issueState, labels), true, nil
	case "issue edit 13 --repo goxdra/goxsd9 --add-label needs-human":
		b.needsHuman = true
		return "", true, nil
	case "project item-list 1 --owner goxdra --format json --limit 500":
		return b.projectItemsJSON(), true, nil
	case "project field-list 1 --owner goxdra --format json":
		return `{"fields":[{"id":"status-id","name":"Status","options":[{"id":"backlog-id","name":"Backlog"},{"id":"done-id","name":"Done"}]}]}`, true, nil
	case "project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-13 --field-id status-id --single-select-option-id backlog-id":
		b.projectTransitionAttempts++
		if b.projectTransitionFailures > 0 {
			b.projectTransitionFailures--
			return "", true, errors.New("simulated needs-human Project transition failure")
		}
		b.projectStatus = "Backlog"
		return "", true, nil
	case "project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-13 --field-id status-id --single-select-option-id done-id":
		b.projectDone = true
		b.projectStatus = "Done"
		return "", true, nil
	default:
		return "", false, nil
	}
}

func (b *workflowBackend) projectItemsJSON() string {
	if !b.projectMember {
		return `{"items":[],"totalCount":0}`
	}
	if b.projectMalformed {
		return fmt.Sprintf(`{"items":[{"content":{"repository":%q,"type":%q},"id":%q,"status":%q}],"totalCount":1}`,
			b.projectRepository, b.projectType, b.projectItemID, b.projectStatus)
	}
	return fmt.Sprintf(`{"items":[{"content":{"number":%d,"repository":%q,"type":%q},"id":%q,"status":%q}],"totalCount":1}`,
		b.projectNumber, b.projectRepository, b.projectType, b.projectItemID, b.projectStatus)
}

func (b *workflowBackend) pullRequestJSON() (string, error) {
	response := pullRequestAPI{Body: b.body, Draft: false, State: "open", Title: b.title}
	if b.merged {
		response.Merged = true
		response.MergedAt = &b.mergedAt
		response.MergeCommitSHA = b.mergeSHA
		response.State = "closed"
	}
	response.Base.Ref = "main"
	response.Base.SHA = "base-sha"
	response.Head.Ref = b.branch
	response.Head.SHA = b.head
	response.URL = "https://github.com/goxdra/goxsd9/pull/14"
	return marshalTestResponse(response)
}

func (b *workflowBackend) commentsJSON() (string, error) {
	if len(b.comments) < 2 {
		return marshalTestResponse(b.comments)
	}
	first, err := marshalTestResponse(b.comments[:1])
	if err != nil {
		return "", err
	}
	second, err := marshalTestResponse(b.comments[1:])
	if err != nil {
		return "", err
	}
	return first + second, nil
}

//nolint:gocognit // Exercise the distinct duplicate and transport simulations.
func (b *workflowBackend) postComment(data []byte) (string, error) {
	var request struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return "", fmt.Errorf("decode comment request: %w", err)
	}
	comment := issueCommentAPI{ID: int64(len(b.comments) + 1), Body: request.Body, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	b.commentPostCount++
	comment.User.Login = b.postCommentAuthor
	if comment.User.Login == "" {
		comment.User.Login = trustedActor
	}
	b.maybeCloseBeforeCommentPost()
	if b.duplicateChallengeBeforeNextPost {
		b.duplicateChallengeBeforeNextPost = false
		challenge, ok := parseEvaluationChallenge(request.Body)
		if !ok {
			return "", errors.New("posted challenge fixture is malformed")
		}
		challenge.Challenge = "concurrent-equivalent-challenge"
		challenge.RequestedAt = challenge.RequestedAt.Add(-time.Second)
		marker, err := json.Marshal(challenge)
		if err != nil {
			return "", fmt.Errorf("encode concurrent challenge fixture: %w", err)
		}
		duplicate := issueCommentAPI{
			ID:        comment.ID,
			Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, challenge.Head),
			CreatedAt: challenge.RequestedAt,
		}
		duplicate.User.Login = trustedActor
		b.comments = append(b.comments, duplicate)
		comment.ID = int64(len(b.comments) + 1)
	}
	b.comments = append(b.comments, comment)
	b.maybeDuplicateClosure(request.Body, comment)
	if b.duplicateReceiptPost && hasMarker(request.Body, evaluationMarker) {
		duplicate := comment
		duplicate.ID++
		b.comments = append(b.comments, duplicate)
	}
	if b.duplicateConvergencePost && hasMarker(request.Body, evaluationConvergenceMarker) {
		duplicate := comment
		duplicate.ID++
		b.comments = append(b.comments, duplicate)
	}
	if b.duplicateChallengeOnNextPost {
		b.duplicateChallengeOnNextPost = false
		challenge, ok := parseEvaluationChallenge(request.Body)
		if !ok {
			return "", errors.New("posted challenge fixture is malformed")
		}
		challenge.Challenge = "concurrent-equivalent-challenge"
		challenge.RequestedAt = challenge.RequestedAt.Add(time.Second)
		marker, err := json.Marshal(challenge)
		if err != nil {
			return "", fmt.Errorf("encode concurrent challenge fixture: %w", err)
		}
		duplicate := issueCommentAPI{
			ID:        comment.ID + 1,
			Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, challenge.Head),
			CreatedAt: comment.CreatedAt,
		}
		duplicate.User.Login = trustedActor
		b.comments = append(b.comments, duplicate)
	}
	if b.postCommentResponseMode == "transport" {
		return "", errors.New("simulated lost comment response")
	}
	return `{}`, nil
}

func (b *workflowBackend) maybeDuplicateClosure(body string, comment issueCommentAPI) {
	if !b.duplicateClosurePost || !hasMarker(body, evaluationChallengeClosureMarker) {
		return
	}
	b.duplicateClosurePost = false
	duplicate := comment
	duplicate.ID++
	b.comments = append(b.comments, duplicate)
}

func (b *workflowBackend) maybeCloseBeforeCommentPost() {
	if !b.closePRBeforeNextCommentPost {
		return
	}
	b.closePRBeforeNextCommentPost = false
	b.merged = true
	b.mergedAt = time.Now().UTC().Truncate(time.Second)
}

func (b *workflowBackend) merge(data []byte) (string, error) {
	if err := json.Unmarshal(data, &b.mergeRequest); err != nil {
		return "", fmt.Errorf("decode merge request: %w", err)
	}
	b.merged = true
	b.mergedAt = time.Now().UTC().Truncate(time.Second)
	if b.delayReceiptAfterMerge {
		b.delayEvaluationReceipt()
	}
	if b.mutatePRBodyAfterMerge {
		b.body += "\nReviewed metadata drift.\n"
	}
	switch b.mergeResponseMode {
	case "transport":
		return "", errors.New("simulated lost merge response")
	case "malformed":
		return `{"merged":`, nil
	case "missing-sha":
		return `{"merged":true}`, nil
	default:
		return fmt.Sprintf(`{"merged":true,"sha":%q}`, b.mergeSHA), nil
	}
}

func (b *workflowBackend) delayEvaluationReceipt() {
	b.t.Helper()
	if len(b.comments) < 2 {
		b.t.Fatal("delayed receipt fixture has no recorded receipt")
	}
	delayedAt := b.mergedAt.Add(time.Second)
	b.comments[1].CreatedAt = delayedAt
	b.comments[1].Body = replaceTestReceipt(b.t, b.comments[1].Body, func(receipt *evaluationReceipt) {
		receipt.RecordedAt = delayedAt
	})
}

func marshalTestResponse(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
