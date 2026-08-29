package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestEvaluationStatusNoChallengeIgnoresProseAndLookalikes(t *testing.T) {
	comments := []pullRequestComment{
		statusComment("The Examiner challenge is still being discussed; this is prose, not workflow state.", owner,
			time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)),
		statusComment("<!-- workflowctl-evaluation-challenge-not-a-marker {\"challenge\":\"fake\"} -->", owner,
			time.Date(2026, time.August, 20, 10, 1, 0, 0, time.UTC)),
	}
	backend := newEvaluationStatusBackend(t, 175, "status-head", comments)
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	if err := application.runEvaluation([]string{"status", "175"}); err != nil {
		t.Fatalf("status without trusted challenge: %v", err)
	}
	for _, want := range []string{
		"State: no trusted challenges",
		"Trusted challenges: 0",
		"Outstanding challenges: 0",
		"Resolved challenges: 0",
		"Recorded rounds: 0",
		"Recorded fail verdicts: 0",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestEvaluationStatusRejectsUntrustedStructuredMarker(t *testing.T) {
	challenge := evaluationChallenge{
		Challenge:   "untrusted-status-challenge",
		Head:        "status-head",
		PR:          175,
		RequestedAt: time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC),
	}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := statusComment(fmt.Sprintf("<!-- %s%s -->", evaluationChallengeMarker, marker), owner,
		challenge.RequestedAt)
	backend := newEvaluationStatusBackend(t, 175, "status-head", []pullRequestComment{comment})
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	err = application.runEvaluation([]string{"status", "175"})
	if err == nil || !strings.Contains(err.Error(), "untrusted evaluation evidence") {
		t.Fatalf("untrusted marker error = %v, want rejection", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("untrusted marker wrote a report:\n%s", stdout.String())
	}
}

func TestEvaluationStatusResolvesChallengesAndPreservesVerdicts(t *testing.T) {
	base := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
	passChallenge := historyTestChallenge(t, "status-pass", 175, "status-head", base, trustedActor)
	passReceipt := historyTestEvaluation(t, passChallenge, "status-pass-run", 1, "pass", 0,
		historyTestCurrentBase64, trustedActor, base.Add(time.Minute))
	failChallenge := historyTestChallenge(t, "status-fail", 175, "status-head", base.Add(2*time.Minute), trustedActor)
	failReceipt := historyTestEvaluation(t, failChallenge, "status-fail-run", 2, "fail", 1,
		historyTestCurrentBase64, trustedActor, base.Add(3*time.Minute))
	backend := newEvaluationStatusBackend(t, 175, "status-head", []pullRequestComment{
		passChallenge, passReceipt, failChallenge, failReceipt,
	})
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	if err := application.runEvaluation([]string{"status", "175"}); err != nil {
		t.Fatalf("status with resolved challenges: %v", err)
	}
	for _, want := range []string{
		"State: resolved challenges",
		"Current head: status-head",
		"Trusted challenges: 2",
		"Outstanding challenges: 0",
		"Resolved challenges: 2",
		"Recorded rounds: 2",
		"Recorded pass verdicts: 1",
		"Recorded fail verdicts: 1",
		"status-pass (head=status-head, requested=2026-08-20T10:00:00Z): resolved by round 1 (verdict: pass)",
		"status-fail (head=status-head, requested=2026-08-20T10:02:00Z): resolved by round 2 (verdict: fail)",
		"- round 1: pass\n- round 2: fail",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status missing %q:\n%s", want, stdout.String())
		}
	}
	if first, second := strings.Index(stdout.String(), "status-pass"), strings.Index(stdout.String(), "status-fail"); first < 0 || second < 0 || first > second {
		t.Fatalf("challenge order was not preserved:\n%s", stdout.String())
	}
}

func TestEvaluationStatusLegacyReceiptCountsWithoutResolvingChallenge(t *testing.T) {
	requested := time.Date(2026, time.August, 20, 10, 30, 0, 0, time.UTC)
	challenge := historyTestChallenge(t, "legacy-status", 175, "status-head", requested, trustedActor)
	receipt := historyTestNoAttestation(t, challenge, 1, "fail", requested.Add(time.Minute), trustedActor)
	backend := newEvaluationStatusBackend(t, 175, "status-head", []pullRequestComment{challenge, receipt})
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	if err := application.runEvaluation([]string{"status", "175"}); err != nil {
		t.Fatalf("status with legacy receipt: %v", err)
	}
	for _, want := range []string{
		"Outstanding challenges: 1",
		"Resolved challenges: 0",
		"Recorded rounds: 1",
		"Recorded fail verdicts: 1",
		"legacy-status (head=status-head, requested=2026-08-20T10:30:00Z): outstanding",
		"- round 1: fail",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("legacy status missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestEvaluationStatusReportsChallengeOnlyHistoryWithoutFailures(t *testing.T) {
	base := time.Date(2026, time.August, 20, 11, 0, 0, 0, time.UTC)
	comments := []pullRequestComment{
		historyTestChallenge(t, "pr-172-challenge-1", 172, "pr-172-head", base, trustedActor),
		historyTestChallenge(t, "pr-172-challenge-2", 172, "pr-172-head", base.Add(time.Minute), trustedActor),
		historyTestChallenge(t, "pr-172-challenge-3", 172, "pr-172-head", base.Add(2*time.Minute), trustedActor),
	}
	backend := newEvaluationStatusBackend(t, 172, "pr-172-head", comments)
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	if err := application.runEvaluation([]string{"status", "172"}); err != nil {
		t.Fatalf("status with challenge-only history: %v", err)
	}
	for _, want := range []string{
		"State: outstanding challenges",
		"Trusted challenges: 3",
		"Outstanding challenges: 3",
		"Resolved challenges: 0",
		"Recorded rounds: 0",
		"Recorded pass verdicts: 0",
		"Recorded fail verdicts: 0",
		"pr-172-challenge-1",
		"pr-172-challenge-2",
		"pr-172-challenge-3",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("challenge-only status missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "round 1:") || strings.Contains(stdout.String(), "three failed") {
		t.Fatalf("challenge-only status invented recorded failures:\n%s", stdout.String())
	}
}

func TestEvaluationStatusKeepsPriorHeadChallengeOutstanding(t *testing.T) {
	requested := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	challenge := historyTestChallenge(t, "prior-head-status", 175, "old-head", requested, trustedActor)
	backend := newEvaluationStatusBackend(t, 175, "new-head", []pullRequestComment{challenge})
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	if err := application.runEvaluation([]string{"status", "175"}); err != nil {
		t.Fatalf("status with prior-head challenge: %v", err)
	}
	for _, want := range []string{
		"Current head: new-head",
		"Outstanding challenges: 1",
		"Resolved challenges: 0",
		"prior-head-status (head=old-head",
		": outstanding",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("prior-head status missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestEvaluationStatusRejectsInvalidHistoryWithoutPartialOutput(t *testing.T) {
	base := time.Date(2026, time.August, 20, 13, 0, 0, 0, time.UTC)
	validChallengeValue := evaluationChallenge{
		Challenge:   "valid-status",
		Head:        "status-head",
		PR:          175,
		RequestedAt: base,
	}
	validChallenge := testEvaluationChallengeComment(t, validChallengeValue)
	tests := []struct {
		name     string
		comments []pullRequestComment
		want     string
	}{
		{
			name: "malformed challenge",
			comments: []pullRequestComment{statusComment(
				"<!-- "+evaluationChallengeMarker+"not-json -->", trustedActor, base)},
			want: "trusted evaluation challenge marker is malformed",
		},
		{
			name:     "challenge targets another PR",
			comments: []pullRequestComment{historyTestChallenge(t, "wrong-status-pr", 176, "status-head", base, trustedActor)},
			want:     "targets PR #176, want PR #175",
		},
		{
			name: "duplicate challenge identity",
			comments: []pullRequestComment{
				validChallenge,
				historyTestChallenge(t, "valid-status", 175, "status-head", base.Add(time.Minute), trustedActor),
			},
			want: "has duplicate trusted markers",
		},
		{
			name: "mismatched receipt",
			comments: []pullRequestComment{
				validChallenge,
				structuredEvaluationCommentForTarget(t, validChallengeValue, "other-head", 175,
					"mismatched-status-run", 1, base.Add(time.Minute)),
			},
			want: "has 0 matching trusted challenges",
		},
		{
			name: "malformed repair",
			comments: []pullRequestComment{statusComment(
				"<!-- "+evaluationRepairMarker+"not-json -->", trustedActor, base)},
			want: "trusted evaluation repair marker is malformed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newEvaluationStatusBackend(t, 175, "status-head", test.comments)
			var stdout bytes.Buffer
			application := app{stdout: &stdout, executeCommand: backend.execute}
			err := application.runEvaluation([]string{"status", "175"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("status error = %v, want %q", err, test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("status wrote partial report:\n%s", stdout.String())
			}
		})
	}
}

func TestEvaluationStatusArgumentsOpenStateReadOnlyAndDeterministic(t *testing.T) {
	base := time.Date(2026, time.August, 20, 14, 0, 0, 0, time.UTC)
	challenge := historyTestChallenge(t, "deterministic-status", 175, "status-head", base, trustedActor)
	backend := newEvaluationStatusBackend(t, 175, "status-head", []pullRequestComment{challenge})
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	for _, args := range [][]string{
		{"status"},
		{"status", "175", "extra"},
		{"status", "0"},
	} {
		if err := application.runEvaluation(args); err == nil || exitCode(err) != 2 {
			t.Fatalf("status args %v error = %v, want usage error", args, err)
		}
	}
	if len(backend.commands) != 0 {
		t.Fatalf("invalid status arguments issued commands: %v", backend.commands)
	}

	if err := application.runEvaluation([]string{"status", "175"}); err != nil {
		t.Fatalf("first deterministic status: %v", err)
	}
	first := stdout.String()
	stdout.Reset()
	if err := application.runEvaluation([]string{"status", "175"}); err != nil {
		t.Fatalf("second deterministic status: %v", err)
	}
	if got := stdout.String(); got != first {
		t.Fatalf("status output changed between reads:\nfirst:\n%s\nsecond:\n%s", first, got)
	}
	wantCommands := []string{
		"git rev-parse --show-toplevel",
		"gh api repos/goxdra/goxsd9/pulls/175",
		"gh api --paginate repos/goxdra/goxsd9/issues/175/comments?per_page=100",
	}
	got := backend.commands
	if len(got) != len(wantCommands)*2 {
		t.Fatalf("status commands = %v, want two read-only passes", got)
	}
	for index, command := range got {
		if command != wantCommands[index%len(wantCommands)] {
			t.Fatalf("status command %d = %q, want %q", index, command, wantCommands[index%len(wantCommands)])
		}
	}
}

func TestEvaluationStatusRejectsClosedPR(t *testing.T) {
	backend := newEvaluationStatusBackend(t, 175, "status-head", nil)
	backend.state = "closed"
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: backend.execute}

	err := application.runEvaluation([]string{"status", "175"})
	if err == nil || exitCode(err) != 3 || !strings.Contains(err.Error(), "PR #175 is CLOSED") {
		t.Fatalf("closed PR error = %v, want stable state error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("closed PR wrote a report:\n%s", stdout.String())
	}
}

func TestEvaluationStatusHelpAdvertisesCommand(t *testing.T) {
	var output bytes.Buffer
	if err := (app{stdout: &output}).usage(); err != nil {
		t.Fatalf("usage: %v", err)
	}
	for _, want := range []string{
		"workflowctl evaluation status PR",
		"workflowctl evaluation resolve PR --challenge ID --reason-file FILE",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("usage omits %q:\n%s", want, output.String())
		}
	}
}

func statusComment(body, author string, createdAt time.Time) pullRequestComment {
	comment := pullRequestComment{Body: body, CreatedAt: createdAt}
	comment.Author.Login = author
	return comment
}

type evaluationStatusBackend struct {
	number   int
	state    string
	head     string
	comments []issueCommentAPI
	commands []string
}

func newEvaluationStatusBackend(t *testing.T, number int, head string, comments []pullRequestComment) *evaluationStatusBackend {
	t.Helper()
	apiComments := make([]issueCommentAPI, 0, len(comments))
	for _, comment := range comments {
		apiComment := issueCommentAPI{ID: comment.ID, Body: comment.Body, CreatedAt: comment.CreatedAt}
		apiComment.User.Login = comment.Author.Login
		apiComments = append(apiComments, apiComment)
	}
	return &evaluationStatusBackend{number: number, state: "open", head: head, comments: apiComments}
}

func (b *evaluationStatusBackend) execute(_ string, input io.Reader, name string, args ...string) (string, error) {
	if input != nil {
		return "", fmt.Errorf("status issued input to %s %s", name, strings.Join(args, " "))
	}
	command := name + " " + strings.Join(args, " ")
	b.commands = append(b.commands, command)
	switch command {
	case "git rev-parse --show-toplevel":
		return "/status-root", nil
	case fmt.Sprintf("gh api repos/goxdra/goxsd9/pulls/%d", b.number):
		response := pullRequestAPI{State: b.state}
		response.Head.SHA = b.head
		return marshalTestResponse(response)
	case fmt.Sprintf("gh api --paginate repos/goxdra/goxsd9/issues/%d/comments?per_page=100", b.number):
		return b.commentsJSON()
	default:
		return "", fmt.Errorf("unexpected status command: %s", command)
	}
}

func (b *evaluationStatusBackend) commentsJSON() (string, error) {
	if len(b.comments) < 2 {
		return marshalTestResponse(b.comments)
	}
	middle := len(b.comments) / 2
	first, err := marshalTestResponse(b.comments[:middle])
	if err != nil {
		return "", err
	}
	second, err := marshalTestResponse(b.comments[middle:])
	if err != nil {
		return "", err
	}
	return first + second, nil
}

func exitCode(err error) int {
	var exitErr *exitError
	if !errors.As(err, &exitErr) {
		return 0
	}
	return exitErr.code
}
