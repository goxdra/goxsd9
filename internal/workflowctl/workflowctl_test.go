package workflowctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestQualityChecksIncludeUnsupportedFeatureRegistry(t *testing.T) {
	checks := (app{}).qualityChecks(t.TempDir(), true)
	for _, check := range checks {
		if check.name == "unsupported feature registry" {
			return
		}
	}
	t.Fatal("quality checks do not include unsupported feature registry")
}

func TestIssueFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   int
		ok     bool
	}{
		{branch: "agent/issue-12", want: 12, ok: true},
		{branch: "agent/issue-12-bootstrap", want: 12, ok: true},
		{branch: "main", want: 0, ok: false},
		{branch: "agent/issue-no", want: 0, ok: false},
	}
	for _, test := range tests {
		got, ok := issueFromBranch(test.branch)
		if got != test.want || ok != test.ok {
			t.Fatalf("issueFromBranch(%q) = (%d, %t), want (%d, %t)", test.branch, got, ok, test.want, test.ok)
		}
	}
}

func TestTrailerTime(t *testing.T) {
	want := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	got, err := trailerTime("subject\n\nAgent-Lease-Until: 2026-08-15T06:00:00Z\n", "Agent-Lease-Until")
	if err != nil {
		t.Fatalf("trailerTime: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("trailerTime = %s, want %s", got, want)
	}
}

func TestEvaluationReceiptRoundTrip(t *testing.T) {
	report := []byte("No findings.")
	recorded := time.Date(2026, time.August, 15, 4, 0, 0, 0, time.UTC)
	receipt := evaluationReceipt{
		Evaluator:    "Examiner",
		Head:         "abc123",
		RecordedAt:   recorded,
		ReportSHA256: fmt.Sprintf("%x", sha256.Sum256(report)),
		Round:        2,
		Verdict:      "pass",
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	body := evaluationComment(marker, nil, string(report))
	got, ok := parseEvaluationReceipt(body)
	if !ok {
		t.Fatal("parseEvaluationReceipt rejected a generated marker")
	}
	if got.Head != "abc123" || got.Round != 2 || got.Verdict != "pass" {
		t.Fatalf("receipt = %#v", got)
	}
	comment := pullRequestComment{Body: body, CreatedAt: recorded}
	comment.Author.Login = trustedActor
	receipts, err := evaluationReceipts([]pullRequestComment{comment})
	if err != nil {
		t.Fatalf("evaluationReceipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("trusted receipts = %d, want 1", len(receipts))
	}
}

func TestDecodeJSONDocuments(t *testing.T) {
	type document struct {
		Page int `json:"page"`
	}
	tests := []struct {
		name    string
		input   string
		want    []document
		wantErr bool
	}{
		{name: "multiple documents", input: `{"page":1}{"page":2}`, want: []document{{Page: 1}, {Page: 2}}},
		{name: "empty document stream", input: "\n\t", wantErr: true},
		{name: "malformed document", input: `{"page":1}{"page":`, wantErr: true},
		{name: "trailing non-json data", input: `{"page":1}trailing`, wantErr: true},
	}
	for _, test := range tests {
		got, err := decodeJSONDocuments[document](test.input)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: decodeJSONDocuments error = %v, want error %t", test.name, err, test.wantErr)
		}
		if test.wantErr {
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s: decodeJSONDocuments = %#v, want %#v", test.name, got, test.want)
		}
	}
}

func TestLatestEvaluationControlsHead(t *testing.T) {
	pass := testEvaluationComment(t, "head", 1, "pass")
	fail := testEvaluationComment(t, "head", 2, "fail")
	view := pullRequestView{Comments: []pullRequestComment{pass, fail}, HeadRefOID: "head"}
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil {
		t.Fatalf("latestEvaluationPasses: %v", err)
	}
	if passes {
		t.Fatal("an earlier pass overrode the latest failing evaluation")
	}
}

func TestLatestStructuredEvaluationPasses(t *testing.T) {
	requested := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	recorded := requested.Add(time.Minute)
	challenge := evaluationChallenge{Challenge: "run-challenge", Head: "head", PR: 11, RequestedAt: requested}
	challengeMarker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	challengeComment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeMarker),
		CreatedAt: requested,
	}
	challengeComment.Author.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: "run-challenge", Evaluator: "Examiner", Findings: []evaluationFinding{}, Head: "head", PR: 11,
		RunID: "examiner-run", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	attestationMarker, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationMarker)),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recorded,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             3,
		Verdict:           attestation.Verdict,
	}
	receiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	evaluationReceiptComment := pullRequestComment{
		Body:      evaluationComment(receiptMarker, attestationMarker, report),
		CreatedAt: recorded,
	}
	evaluationReceiptComment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{challengeComment, evaluationReceiptComment}, HeadRefOID: "head"}
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil || !passes {
		t.Fatal("valid structured evaluation did not pass")
	}
	encodedReport := base64.StdEncoding.EncodeToString([]byte(report))
	changedReport := base64.StdEncoding.EncodeToString([]byte("Changed."))
	view.Comments[1].Body = strings.Replace(view.Comments[1].Body, encodedReport, changedReport, 1)
	if _, err := latestEvaluationPasses(view, 11); err == nil {
		t.Fatal("tampered structured evaluation did not invalidate history")
	}
}

func TestEvaluationEvidenceRejectsDuplicateJSONKeys(t *testing.T) {
	raw := []byte(`{"schema":"goxsd9/examiner-attestation/v1","findings":[],"verdict":"fail","verdict":"pass"}`)
	body := fmt.Sprintf("<!-- %s%s -->", evaluationAttestationMarker, raw)
	if _, _, ok := parseCommentAttestation(body); ok {
		t.Fatal("duplicate JSON keys were accepted in an Examiner attestation")
	}
}

func TestLatestEvaluationRejectsOrphanHistoricalReceipt(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	orphan := evaluationChallenge{Challenge: "orphan-challenge", Head: "head", PR: 11, RequestedAt: now}
	current := evaluationChallenge{
		Challenge:   "current-challenge",
		Head:        "head",
		PR:          11,
		RequestedAt: now.Add(4 * time.Minute),
	}
	comments := []pullRequestComment{
		structuredEvaluationComment(t, orphan, "orphan-run", 1, now.Add(time.Minute)),
		testEvaluationChallengeComment(t, current),
		structuredEvaluationComment(t, current, "current-run", 2, now.Add(5*time.Minute)),
	}

	passes, err := latestEvaluationPasses(pullRequestView{Comments: comments, HeadRefOID: "head"}, 11)
	if err == nil {
		t.Fatalf("orphan historical receipt was accepted, passes=%t", passes)
	}
}

func TestEvaluationHistoryRequiresExactlyOneMatchingChallenge(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	tests := []struct {
		name               string
		challenges         []evaluationChallenge
		receiptHead        string
		receiptPR          int
		receiptAt          time.Time
		receiptFirst       bool
		untrustedChallenge bool
	}{
		{
			name: "duplicate",
			challenges: []evaluationChallenge{{
				Challenge: "duplicate-challenge", Head: "head", PR: 11, RequestedAt: now,
			}, {
				Challenge: "duplicate-challenge", Head: "head", PR: 11, RequestedAt: now.Add(time.Second),
			}},
			receiptHead: "head", receiptPR: 11, receiptAt: now.Add(2 * time.Minute),
		},
		{
			name: "wrong head",
			challenges: []evaluationChallenge{{
				Challenge: "wrong-head-challenge", Head: "other-head", PR: 11, RequestedAt: now,
			}},
			receiptHead: "head", receiptPR: 11, receiptAt: now.Add(time.Minute),
		},
		{
			name: "wrong PR",
			challenges: []evaluationChallenge{{
				Challenge: "wrong-pr-challenge", Head: "head", PR: 12, RequestedAt: now,
			}},
			receiptHead: "head", receiptPR: 11, receiptAt: now.Add(time.Minute),
		},
		{
			name: "future",
			challenges: []evaluationChallenge{{
				Challenge: "future-challenge", Head: "head", PR: 11, RequestedAt: now.Add(2 * time.Minute),
			}},
			receiptHead: "head", receiptPR: 11, receiptAt: now.Add(time.Minute),
		},
		{
			name: "posted after receipt",
			challenges: []evaluationChallenge{{
				Challenge: "after-challenge", Head: "head", PR: 11, RequestedAt: now,
			}},
			receiptHead: "head", receiptPR: 11, receiptAt: now.Add(time.Minute), receiptFirst: true,
		},
		{
			name: "untrusted",
			challenges: []evaluationChallenge{{
				Challenge: "untrusted-challenge", Head: "head", PR: 11, RequestedAt: now,
			}},
			receiptHead: "head", receiptPR: 11, receiptAt: now.Add(time.Minute), untrustedChallenge: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertInvalidEvaluationChallengeHistory(t, test.challenges, test.receiptHead, test.receiptPR,
				test.receiptAt, test.receiptFirst, test.untrustedChallenge)
		})
	}
}

func assertInvalidEvaluationChallengeHistory(t *testing.T, challenges []evaluationChallenge, receiptHead string,
	receiptPR int, receiptAt time.Time, receiptFirst, untrustedChallenge bool) {
	t.Helper()
	var comments []pullRequestComment
	receipt := structuredEvaluationCommentForTarget(t, challenges[0], receiptHead, receiptPR,
		"challenge-test-run", 1, receiptAt)
	if receiptFirst {
		comments = append(comments, receipt)
	}
	for index, challenge := range challenges {
		comment := testEvaluationChallengeComment(t, challenge)
		if untrustedChallenge && index == 0 {
			comment.Author.Login = owner
		}
		comments = append(comments, comment)
	}
	if !receiptFirst {
		comments = append(comments, receipt)
	}

	if _, err := evaluationReceipts(comments); err == nil {
		t.Fatal("evaluation history accepted an invalid challenge binding")
	}
}

func TestLatestEvaluationRejectsHistoricalIdentifierReuse(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	tests := []struct {
		name               string
		firstChallenge     string
		duplicateChallenge string
		firstRunID         string
		duplicateRunID     string
		latestChallenge    string
		latestRunID        string
	}{
		{
			name:               "challenge",
			firstChallenge:     "reused-challenge",
			duplicateChallenge: "reused-challenge",
			firstRunID:         "first-run",
			duplicateRunID:     "duplicate-run",
			latestChallenge:    "fresh-challenge",
			latestRunID:        "fresh-run",
		},
		{
			name:               "Examiner run ID",
			firstChallenge:     "first-challenge",
			duplicateChallenge: "duplicate-challenge",
			firstRunID:         "reused-run",
			duplicateRunID:     "reused-run",
			latestChallenge:    "fresh-challenge",
			latestRunID:        "fresh-run",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := evaluationChallenge{
				Challenge:   test.firstChallenge,
				Head:        "head",
				PR:          11,
				RequestedAt: now,
			}
			duplicate := evaluationChallenge{
				Challenge:   test.duplicateChallenge,
				Head:        "head",
				PR:          11,
				RequestedAt: now.Add(2 * time.Minute),
			}
			latest := evaluationChallenge{
				Challenge:   test.latestChallenge,
				Head:        "head",
				PR:          11,
				RequestedAt: now.Add(4 * time.Minute),
			}
			comments := []pullRequestComment{
				testEvaluationChallengeComment(t, first),
				structuredEvaluationComment(t, first, test.firstRunID, 1, now.Add(time.Minute)),
			}
			if duplicate.Challenge != first.Challenge {
				comments = append(comments, testEvaluationChallengeComment(t, duplicate))
			}
			comments = append(comments,
				structuredEvaluationComment(t, duplicate, test.duplicateRunID, 2, now.Add(3*time.Minute)),
				testEvaluationChallengeComment(t, latest),
				structuredEvaluationComment(t, latest, test.latestRunID, 3, now.Add(5*time.Minute)),
			)

			passes, err := latestEvaluationPasses(pullRequestView{Comments: comments, HeadRefOID: "head"}, 11)
			if err == nil {
				t.Fatalf("historical %s reuse returned no error, passes=%t", test.name, passes)
			}
		})
	}
}

func testEvaluationChallengeComment(t *testing.T, challenge evaluationChallenge) pullRequestComment {
	t.Helper()
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode evaluation challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker),
		CreatedAt: challenge.RequestedAt,
	}
	comment.Author.Login = trustedActor
	return comment
}

func structuredEvaluationComment(t *testing.T, challenge evaluationChallenge, runID string, round int,
	recordedAt time.Time) pullRequestComment {
	t.Helper()
	return structuredEvaluationCommentForTarget(t, challenge, challenge.Head, challenge.PR, runID, round, recordedAt)
}

func structuredEvaluationCommentForTarget(t *testing.T, challenge evaluationChallenge, head string, pr int,
	runID string, round int, recordedAt time.Time) pullRequestComment {
	t.Helper()
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge,
		Evaluator: "Examiner",
		Findings:  evaluationFindings{},
		Head:      head,
		PR:        pr,
		RunID:     runID,
		Schema:    evaluationAttestationSchema,
		Summary:   "No findings.",
		Verdict:   "pass",
	}
	attestationMarker, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode evaluation attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: sha256Hex(attestationMarker),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recordedAt,
		ReportSHA256:      sha256Hex(canonicalEvaluationReport(report)),
		ReportTransport:   evaluationReportTransportV1,
		Round:             round,
		Verdict:           attestation.Verdict,
	}
	receiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode evaluation receipt: %v", err)
	}
	comment := pullRequestComment{
		Body:      evaluationComment(receiptMarker, attestationMarker, report),
		CreatedAt: recordedAt,
	}
	comment.Author.Login = trustedActor
	return comment
}

func TestVersionedEvaluationReportSurvivesVisibleRewrite(t *testing.T) {
	requested := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	recorded := requested.Add(time.Minute)
	challenge := evaluationChallenge{Challenge: "lossless-report", Head: "head", PR: 11, RequestedAt: requested}
	challengeMarker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	challengeComment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeMarker),
		CreatedAt: requested,
	}
	challengeComment.Author.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: evaluationFindings{}, Head: "head", PR: 11,
		RunID: "lossless-report-run", Schema: evaluationAttestationSchema,
		Summary: "No findings; literal \\u001e remains data.", Verdict: "pass",
	}
	attestationMarker, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	canonicalReport := canonicalEvaluationReport(report)
	receipt := evaluationReceipt{
		AttestationSHA256: sha256Hex(attestationMarker),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recorded,
		ReportSHA256:      sha256Hex(canonicalReport),
		ReportTransport:   evaluationReportTransportV1,
		Round:             1,
		Verdict:           attestation.Verdict,
	}
	receiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	receiptComment := pullRequestComment{
		Body:      evaluationComment(receiptMarker, attestationMarker, report),
		CreatedAt: recorded,
	}
	receiptComment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{challengeComment, receiptComment}, HeadRefOID: "head"}

	rewritten := receiptComment
	rewritten.Body = strings.Replace(rewritten.Body, `\u001e`, `\^^`, 1)
	if rewritten.Body == receiptComment.Body {
		t.Fatal("visible report fixture did not contain the rewritten escape")
	}
	view.Comments[1] = rewritten
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil || !passes {
		t.Fatalf("visible report rewrite invalidated canonical evaluation: passes=%t err=%v", passes, err)
	}

	encodedReport := base64.StdEncoding.EncodeToString(canonicalReport)
	tampered := strings.Replace(receiptComment.Body, encodedReport,
		base64.StdEncoding.EncodeToString([]byte("tampered report")), 1)
	view.Comments[1].Body = tampered
	if _, err := latestEvaluationPasses(view, 11); err == nil {
		t.Fatal("tampered encoded report marker was accepted")
	}
}

func TestLatestEvaluationRejectsBareOwnerReceipt(t *testing.T) {
	requested := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	recorded := requested.Add(time.Minute)
	challenge := evaluationChallenge{Challenge: "bot-challenge", Head: "head", PR: 11, RequestedAt: requested}
	challengeMarker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	challengeComment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeMarker),
		CreatedAt: requested,
	}
	challengeComment.Author.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: []evaluationFinding{}, Head: "head", PR: 11,
		RunID: "owner-receipt-run", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	attestationMarker, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationMarker)),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recorded,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             1,
		Verdict:           attestation.Verdict,
	}
	receiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	receiptComment := pullRequestComment{
		Body:      evaluationComment(receiptMarker, attestationMarker, report),
		CreatedAt: recorded,
	}
	receiptComment.Author.Login = owner
	view := pullRequestView{Comments: []pullRequestComment{challengeComment, receiptComment}, HeadRefOID: "head"}
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil {
		t.Fatalf("latestEvaluationPasses: %v", err)
	}
	if passes {
		t.Fatal("bare owner-authored receipt authorized the evaluated head")
	}
}

func TestEvaluationFailureCountIgnoresPassingRounds(t *testing.T) {
	receipts := []evaluationReceipt{{Verdict: "pass"}, {Verdict: "fail"}, {Verdict: "fail"}}
	if got := evaluationFailureCount(receipts); got != 2 {
		t.Fatalf("evaluationFailureCount = %d, want 2", got)
	}
}

func testEvaluationComment(t *testing.T, head string, round int, verdict string) pullRequestComment {
	t.Helper()
	recorded := time.Date(2026, time.August, 15, 4, 0, round, 0, time.UTC)
	report := []byte(verdict + " report")
	receipt := evaluationReceipt{
		Evaluator:    "Examiner",
		Head:         head,
		RecordedAt:   recorded,
		ReportSHA256: fmt.Sprintf("%x", sha256.Sum256(report)),
		Round:        round,
		Verdict:      verdict,
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	comment := pullRequestComment{Body: evaluationComment(marker, nil, string(report)), CreatedAt: recorded}
	comment.Author.Login = trustedActor
	return comment
}

func TestCandidateOrdering(t *testing.T) {
	created := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	candidates := []pickCandidate{
		{Number: 4, Priority: "P2", Effort: "XS", created: created},
		{Number: 3, Priority: "P1", Effort: "M", Blocking: 1, created: created},
		{Number: 2, Priority: "P1", Effort: "S", Blocking: 2, created: created},
		{Number: 1, Priority: "P1", Effort: "XS", Blocking: 2, created: created},
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidateLess(candidates[left], candidates[right])
	})
	want := []int{1, 2, 3, 4}
	for index := range candidates {
		if candidates[index].Number != want[index] {
			t.Fatalf("candidate %d = #%d, want #%d", index, candidates[index].Number, want[index])
		}
	}
}

func TestIssueRelationsUsesGraphQL(t *testing.T) {
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "gh" {
			t.Fatalf("command = %s, want gh", name)
		}
		want := []string{"api", "graphql", "-f", "query=" + issueRelationsQuery,
			"-f", "owner=" + owner, "-f", "repository=" + repository, "-F", "number=25"}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("arguments = %#v, want %#v", args, want)
		}
		return `{"data":{"repository":{"issue":{"blockedBy":{"nodes":[{"number":2,"state":"OPEN","title":"source"}]},"blocking":{"nodes":[]},"createdAt":"2026-08-15T00:00:00Z"}}}}`, nil
	}}
	relations, err := application.issueRelations("/repo", 25)
	if err != nil {
		t.Fatalf("issueRelations: %v", err)
	}
	if got, want := len(relations.BlockedBy.Nodes), 1; got != want {
		t.Fatalf("blockedBy length = %d, want %d", got, want)
	}
	if got, want := relations.BlockedBy.Nodes[0].Number, 2; got != want {
		t.Fatalf("blockedBy issue = %d, want %d", got, want)
	}
}

func TestIssueRelationsDecoratesGitHubFailure(t *testing.T) {
	failure := errors.New("access denied")
	application := app{executeCommand: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
		return "", failure
	}}
	_, err := application.issueRelations("/repo", 25)
	if !errors.Is(err, failure) {
		t.Fatalf("issueRelations error = %v, want wrapped command failure", err)
	}
	if !strings.Contains(err.Error(), "issue #25 dependencies") {
		t.Fatalf("issueRelations error = %v, want issue context", err)
	}
}

func TestGuardRejectsConcurrencyAndElse(t *testing.T) {
	tests := []string{
		"package example\nfunc f() { go f() }\n",
		"package example\nfunc f() chan int { return nil }\n",
		"package example\nimport \"context\"\nfunc f() { <-context.Background().Done() }\n",
		"package example\nfunc f(ch chan<- int) { ch <- 1 }\n",
		"package example\nfunc f() { select {} }\n",
		"package example\nfunc f(ok bool) { if ok { return } else { return } }\n",
		"package example\nimport \"sync\"\nvar _ sync.Mutex\n",
	}
	for _, source := range tests {
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "guard.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse guard fixture: %v", err)
		}
		if err := guardFile(files, file); err == nil {
			t.Fatalf("guard accepted forbidden source %q", source)
		}
	}
}

func TestIssueInputRejectsUnknownProjectOptions(t *testing.T) {
	bodyFile := t.TempDir() + "/issue.md"
	if err := os.WriteFile(bodyFile, []byte("## Acceptance\n\nProof.\n"), 0o600); err != nil {
		t.Fatalf("write issue body: %v", err)
	}
	tests := []struct {
		name   string
		effort string
		phase  string
	}{
		{name: "effort", effort: "Huge", phase: "Bootstrap"},
		{name: "phase", effort: "S", phase: "Eventually"},
	}
	for _, test := range tests {
		if err := validateIssueInput("title", bodyFile, "workflow", "tooling", "P2", test.effort,
			test.phase, "Backlog"); err == nil {
			t.Fatalf("%s option unexpectedly accepted", test.name)
		}
	}
}

func TestDocumentedPositionalFlagOrderParses(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "handoff", run: func() error { return application.runHandoff([]string{"1", "--body-file", "missing"}) }},
		{name: "pr open", run: func() error {
			return application.openPullRequest([]string{"1", "--title", "test(workflow): verify flags", "--body-file", "missing"})
		}},
		{name: "pr finish", run: func() error {
			return application.runPR([]string{"finish", "1", "--summary-file", "missing"})
		}},
		{name: "evaluation", run: func() error {
			return application.recordEvaluation([]string{"1", "--attestation-file", "missing"})
		}},
	}
	for _, test := range tests {
		err := test.run()
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded", test.name)
		}
		if !strings.Contains(err.Error(), "missing") {
			t.Fatalf("%s did not reach body-file validation: %v", test.name, err)
		}
	}
}

func TestPullRequestBodyRemainsMarkdownEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pr.md")
	body := "## Outcome\n\nUse [Markdown](https://example.com).\n\n## Work packet\n\nCloses #33\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write PR body: %v", err)
	}
	got, err := readPullRequestBody(path, 33)
	if err != nil {
		t.Fatalf("read PR body: %v", err)
	}
	if got != body {
		t.Fatalf("PR body = %q, want %q", got, body)
	}
}

func TestSessionSummaryFileIsIndependentFromPRMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{name: "plain text", content: []byte("Explain the outcome and rationale.\n"), want: "Explain the outcome and rationale."},
		{name: "Markdown characters are data", content: []byte("A [link](https://example.com) and # marker."), want: "A [link](https://example.com) and # marker."},
		{name: "empty"},
		{name: "only newline", content: []byte("\n")},
		{name: "leading whitespace", content: []byte(" Explain the outcome.")},
		{name: "extra final newline", content: []byte("Explain the outcome.\n\n")},
		{name: "CRLF", content: []byte("Explain the outcome.\r\n")},
		{name: "trailing spaces", content: []byte("Explain the outcome. \n")},
		{name: "interior Unicode trailing whitespace", content: []byte("Outcome.\u2003\nRationale.")},
		{name: "Unicode line separator", content: []byte("Outcome.\u2028Rationale.")},
		{name: "Unicode format control", content: []byte("Outcome.\u202eRationale.")},
		{name: "claim metadata", content: []byte("Outcome.\nAgent-Run-ID: run-secret")},
		{name: "indented claim metadata", content: []byte("Outcome.\n  Agent-Issue: 33")},
		{name: "invalid UTF-8", content: []byte{0xff}},
		{name: "too large", content: bytes.Repeat([]byte("x"), sessionSummaryLimit+1)},
		{name: "Agent prose is allowed", content: []byte("Agent-based design keeps the boundary explicit."), want: "Agent-based design keeps the boundary explicit."},
	}
	for _, test := range tests {
		path := filepath.Join(t.TempDir(), "summary.txt")
		if err := os.WriteFile(path, test.content, 0o600); err != nil {
			t.Fatalf("%s: write summary: %v", test.name, err)
		}
		got, err := readSessionSummary(path)
		if test.want == "" {
			if err == nil {
				t.Fatalf("%s: invalid summary was accepted", test.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: read summary: %v", test.name, err)
		}
		if string(got) != test.want {
			t.Fatalf("%s: summary = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestFinishRejectsInvalidSummaryBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.txt")
	if err := os.WriteFile(path, []byte("Agent-Run-ID: run-secret\n"), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		t.Fatalf("invalid summary executed %s %v", name, args)
		return "", nil
	}}
	if err := application.runPR([]string{"finish", "34", "--summary-file", path}); err == nil {
		t.Fatal("invalid summary was accepted")
	}
}

func TestGitHistoryIncludesCommitBodiesForWorkflowReaders(t *testing.T) {
	since := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: func(_ string, _ io.Reader, name string,
		args ...string,
	) (string, error) {
		want := "git log --first-parent -n 3 --since=2026-08-08T12:00:00Z --date=short " +
			"--pretty=format:- %h %ad %s%n%w(74,2,2)%b%w(0,0,0)"
		got := name + " " + strings.Join(args, " ")
		if got != want {
			return "", fmt.Errorf("command = %q, want %q", got, want)
		}
		return "- abc123 2026-08-15 fix(workflow): summarize squash commits\n" +
			"  Explain the problem and durable outcome.", nil
	}}
	if err := application.writeGitHistory("/repo", since, 3); err != nil {
		t.Fatalf("write Git history: %v", err)
	}
	if !strings.Contains(stdout.String(), "Explain the problem and durable outcome.") {
		t.Fatalf("history omitted commit body:\n%s", stdout.String())
	}
}

func TestClaimDeadline(t *testing.T) {
	now := time.Date(2026, time.August, 15, 1, 0, 0, 0, time.UTC)
	oldIssueDeadline := time.Date(2026, time.August, 15, 2, 0, 0, 0, time.UTC)
	if err := validateClaimDeadline(1, oldIssueDeadline, now); err != nil {
		t.Fatalf("future deadline rejected after old issuance interval: %v", err)
	}

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "just before deadline", now: oldIssueDeadline.Add(-time.Nanosecond), want: true},
		{name: "at deadline", now: oldIssueDeadline, want: false},
		{name: "after deadline", now: oldIssueDeadline.Add(time.Nanosecond), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateClaimDeadline(1, oldIssueDeadline, test.now)
			if (err == nil) != test.want {
				t.Fatalf("validateClaimDeadline error = %v, want valid %t", err, test.want)
			}
		})
	}
}

func TestNewClaimCommitUsesClaimDuration(t *testing.T) {
	start := time.Now().UTC()
	var message string
	application := app{executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git rev-parse HEAD^{tree}":
			return "tree", nil
		case "git commit-tree tree -p HEAD":
			data, err := io.ReadAll(input)
			if err != nil {
				return "", err
			}
			message = string(data)
			return "commit", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	_, deadline, _, err := application.newClaimCommit("/repo", 1, "HEAD")
	if err != nil {
		t.Fatalf("newClaimCommit: %v", err)
	}
	end := time.Now().UTC()
	if deadline.Before(start.Add(claimDuration-time.Second)) || deadline.After(end.Add(claimDuration)) {
		t.Fatalf("claim deadline = %s, want about %s after issuance", deadline, claimDuration)
	}
	wireTrailer := "Agent-Lease-Until: " + deadline.Format(time.RFC3339)
	if !strings.Contains(message, wireTrailer) {
		t.Fatalf("claim message lacks deadline trailer %q: %s", wireTrailer, message)
	}
}

func TestPullRequestMustCloseClaim(t *testing.T) {
	view := pullRequestView{}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 7})
	if !pullRequestCloses(view, 7) {
		t.Fatal("linked closing issue was not recognized")
	}
	if pullRequestCloses(view, 8) {
		t.Fatal("unlinked issue was recognized as closing")
	}
}

func TestRequiredQualityCheckMustSucceed(t *testing.T) {
	tests := []struct {
		name    string
		checks  []pullRequestCheck
		wantErr bool
	}{
		{name: "success", checks: []pullRequestCheck{{Name: "quality", Status: "completed", Conclusion: "success"}}},
		{name: "unrelated", checks: []pullRequestCheck{{Name: "docs", Status: "completed", Conclusion: "success"}}, wantErr: true},
		{name: "skipped", checks: []pullRequestCheck{{Name: "quality", Status: "completed", Conclusion: "skipped"}}, wantErr: true},
		{name: "neutral", checks: []pullRequestCheck{{Name: "quality", Status: "completed", Conclusion: "neutral"}}, wantErr: true},
		{name: "running", checks: []pullRequestCheck{{Name: "quality", Status: "in_progress"}}, wantErr: true},
	}
	for _, test := range tests {
		pages := []checkRunsAPI{{CheckRuns: test.checks}}
		err := requireQualityCheck(pages)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: requireQualityCheck error = %v, want error %t", test.name, err, test.wantErr)
		}
	}
}

func TestWorkPacketRejectsMoreThanOneCompanion(t *testing.T) {
	view := pullRequestView{HeadRefOID: "head"}
	for _, number := range []int{1, 2, 3} {
		view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
			Number int `json:"number"`
		}{Number: number})
	}
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if err := application.validateClosingClaims("", view, 1); err == nil {
		t.Fatal("work packet with two companion issues was accepted")
	}
}

func TestEvaluationAttestationIsBoundToChallengeAndHead(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "run-challenge", Head: "head", PR: 11, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker),
		CreatedAt: now,
	}
	comment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{comment}, HeadRefOID: "head"}
	attestation := evaluationAttestation{
		Challenge: "run-challenge",
		Evaluator: "Examiner",
		Findings:  []evaluationFinding{},
		Head:      "head",
		PR:        11,
		RunID:     "examiner-fresh-context",
		Schema:    evaluationAttestationSchema,
		Summary:   "No blocking findings.",
		Verdict:   "pass",
	}
	if err := validateEvaluationAttestation(attestation, 11, view, nil, now); err != nil {
		t.Fatalf("valid attestation rejected: %v", err)
	}
	attestation.Head = "other"
	if err := validateEvaluationAttestation(attestation, 11, view, nil, now); err == nil {
		t.Fatal("wrong-head attestation accepted")
	}
}

func TestEvaluationChallengeRejectsBareOwnerActor(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "bot-challenge", Head: "head", PR: 11, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker),
		CreatedAt: now,
	}
	comment.Author.Login = owner
	if _, ok := trustedEvaluationChallenge([]pullRequestComment{comment}, challenge.Challenge, challenge.PR,
		challenge.Head, now); ok {
		t.Fatal("bare owner-authored challenge was trusted")
	}
}

func TestEvaluationAttestationRejectsCallerVerdictAndReusedChallenge(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if err := application.recordEvaluation([]string{"11", "--verdict", "pass", "--body-file", "report"}); err == nil {
		t.Fatal("caller-selected verdict flags were accepted")
	}

	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "run-used", Head: "head", PR: 11, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: now}
	comment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{comment}, HeadRefOID: "head"}
	attestation := evaluationAttestation{
		Challenge: "run-used", Evaluator: "Examiner", Findings: []evaluationFinding{}, Head: "head", PR: 11,
		RunID: "examiner-run", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	receipts := []evaluationReceipt{{Challenge: "run-used", Verdict: "fail"}}
	if err := validateEvaluationAttestation(attestation, 11, view, receipts, now); err == nil {
		t.Fatal("reused evaluation challenge was accepted")
	}
}

func TestEvaluationFindingsMustBePresentArray(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		decodeFail bool
		valid      bool
	}{
		{name: "omitted", json: `{"verdict":"pass"}`},
		{name: "null", json: `{"verdict":"pass","findings":null}`, decodeFail: true},
		{name: "empty", json: `{"verdict":"pass","findings":[]}`, valid: true},
	}
	for _, test := range tests {
		var attestation evaluationAttestation
		err := json.Unmarshal([]byte(test.json), &attestation)
		if test.decodeFail {
			if err == nil {
				t.Fatalf("%s findings decoded", test.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s findings decode: %v", test.name, err)
		}
		err = validateEvaluationFindings(attestation)
		if (err == nil) != test.valid {
			t.Fatalf("%s findings validation error = %v, valid %t", test.name, err, test.valid)
		}
	}
}

func TestPullRequestFinishStrategyHasRESTFallback(t *testing.T) {
	draft := pullRequestView{IsDraft: true}
	if action := finishActionFor(draft, true); action != finishMergeREST {
		t.Fatalf("ready transition action = %d, want REST merge", action)
	}
	if action := finishActionFor(draft, false); action != finishReplaceDraftREST {
		t.Fatalf("failed ready transition action = %d, want REST replacement", action)
	}
	if action := finishActionFor(pullRequestView{}, false); action != finishMergeREST {
		t.Fatalf("ready PR action = %d, want REST merge", action)
	}
	body := readyReplacementBody("## Outcome\n\nExplain the result.\n\nCloses #1\n", 11, "abc123")
	if !strings.Contains(body, "Closes #1") || !strings.Contains(body, "Replaces draft PR #11") ||
		!strings.Contains(body, "abc123") {
		t.Fatalf("replacement body lost work-packet or provenance data: %q", body)
	}
}

func TestReadyReplacementStillRequiresSummaryArtifact(t *testing.T) {
	body := readyReplacementBody("## Outcome\n\nExplain the result.\n\nCloses #1\n", 11, "abc123")
	if !strings.Contains(body, "Closes #1") {
		t.Fatalf("replacement body lost work-packet evidence: %q", body)
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		t.Fatalf("missing replacement summary executed %s %v", name, args)
		return "", nil
	}}
	if err := application.runPR([]string{"finish", "12"}); err == nil {
		t.Fatal("replacement finish accepted no summary artifact")
	}
}
