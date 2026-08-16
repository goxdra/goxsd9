package workflowctl

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMergedPullRequestSHARequiresCompletedProof(t *testing.T) {
	tests := []struct {
		name    string
		view    pullRequestView
		wantErr string
	}{
		{name: "unmerged", view: pullRequestView{MergeCommitSHA: "merge"}, wantErr: "completed merge"},
		{name: "missing sha", view: pullRequestView{Merged: true}, wantErr: "merge commit SHA"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := mergedPullRequestSHA(test.view); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("mergedPullRequestSHA error = %v, want %q", err, test.wantErr)
			}
		})
	}
	merged, err := mergedPullRequestSHA(pullRequestView{Merged: true, MergeCommitSHA: "merge"})
	if err != nil || merged != "merge" {
		t.Fatalf("mergedPullRequestSHA = %q, %v; want merge, nil", merged, err)
	}
}

func TestPostMergeRecoveryErrorDistinguishesCompletedMerge(t *testing.T) {
	cause := errors.New("base changed concurrently")
	err := postMergeRecoveryError(55, "merge-sha", "canonical Git base convergence", cause)
	if !strings.Contains(err.Error(), "Merge completed") || !strings.Contains(err.Error(), "go tool workflowctl pr recover 55") {
		t.Fatalf("postMergeRecoveryError = %v, want completed-merge recovery guidance", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("postMergeRecoveryError = %v, want cause %v", err, cause)
	}
}

func TestRecoveryCleanupPlanRequiresClaimProof(t *testing.T) {
	application := app{}
	layout := repositoryLayout{primaryRoot: "/repo"}
	_, err := application.prepareRecoveryCleanupPlan("/repo", layout, pullRequestView{
		HeadRefName: "agent/issue-55",
	}, 14, "")
	if err == nil || !strings.Contains(err.Error(), "claim head ref") {
		t.Fatalf("prepareRecoveryCleanupPlan error = %v, want missing-head refusal", err)
	}
}

func TestRecoveryUsesImmutableEvaluatedHeadAndRefusesAdvancedPR(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	const evaluatedHead = "evaluated-head"
	recordedAt := mergedAt.Add(-time.Minute)
	view := pullRequestView{
		BaseRefName:    "main",
		Body:           recoveryBody,
		HeadRefName:    "agent/issue-55",
		HeadRefOID:     "advanced-head",
		Merged:         true,
		MergedAt:       &mergedAt,
		MergeCommitSHA: "merge-commit",
		Comments:       recoveryEvaluationHistory(t, 14, evaluatedHead, recordedAt),
	}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 55})

	proof, err := mergeTimeEvaluationProof(view, 14)
	if err != nil {
		t.Fatalf("mergeTimeEvaluationProof error: %v", err)
	}
	got := proof.head
	if got != evaluatedHead {
		t.Fatalf("mergeTimeEvaluationProof head = %q, want %q", got, evaluatedHead)
	}

	application := app{}
	_, err = application.prepareRecoveryCleanupPlan("/repo", repositoryLayout{primaryRoot: "/repo"}, view, 14, got)
	if err == nil || !strings.Contains(err.Error(), "differs from immutable merge-time evaluated head") {
		t.Fatalf("prepareRecoveryCleanupPlan error = %v, want advanced-head refusal", err)
	}
}

func TestRecoveryRejectsReceiptOnlyEvaluationProof(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	view := pullRequestView{
		Merged:         true,
		MergedAt:       &mergedAt,
		MergeCommitSHA: "merge-commit",
		Comments:       []pullRequestComment{recoveryEvaluationReceipt(t, 14, "evaluated-head", mergedAt.Add(-time.Minute))},
	}
	if _, err := mergeTimeEvaluationProof(view, 14); err == nil || !strings.Contains(err.Error(), "matching trusted challenges") {
		t.Fatalf("mergeTimeEvaluationProof error = %v, want receipt-only refusal", err)
	}
}

func TestRecoveryRejectsPassThenFailAtMergeBoundary(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	comments := append(
		recoveryEvaluationRound(t, 14, "evaluated-head", "pass-challenge", "pass-run", 1, "pass", mergedAt.Add(-3*time.Minute), mergedAt.Add(-3*time.Minute)),
		recoveryEvaluationRound(t, 14, "evaluated-head", "fail-challenge", "fail-run", 2, "fail", mergedAt.Add(-time.Minute), mergedAt.Add(-time.Minute))...,
	)
	view := recoveryMergedView(mergedAt, "evaluated-head", comments)
	if _, err := mergeTimeEvaluationProof(view, 14); err == nil || !strings.Contains(err.Error(), "latest trusted evaluation receipt") {
		t.Fatalf("mergeTimeEvaluationProof error = %v, want latest-failure refusal", err)
	}
}

func TestRecoveryRejectsPostMergeReceiptTimestampSkew(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	comments := recoveryEvaluationRound(t, 14, "evaluated-head", "skew-challenge", "skew-run", 1, "pass",
		mergedAt.Add(-time.Minute), mergedAt.Add(time.Minute))
	view := recoveryMergedView(mergedAt, "evaluated-head", comments)
	if _, err := mergeTimeEvaluationProof(view, 14); err == nil || !strings.Contains(err.Error(), "created after the merge boundary") {
		t.Fatalf("mergeTimeEvaluationProof error = %v, want post-merge timestamp refusal", err)
	}
}

func TestRecoveryRejectsPRBodyDrift(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	view := recoveryMergedView(mergedAt, "evaluated-head",
		recoveryEvaluationHistory(t, 14, "evaluated-head", mergedAt.Add(-time.Minute)))
	proof, err := mergeTimeEvaluationProof(view, 14)
	if err != nil {
		t.Fatalf("mergeTimeEvaluationProof error: %v", err)
	}
	view.Body += "\nCloses #56\n"
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 56})

	application := app{}
	_, err = application.prepareRecoveryCleanupPlanWithProof("/repo", repositoryLayout{primaryRoot: "/repo"}, view, 14, proof)
	if err == nil || !strings.Contains(err.Error(), "PR body differs") {
		t.Fatalf("prepareRecoveryCleanupPlanWithProof error = %v, want body-drift refusal", err)
	}
}

const recoveryBody = "Closes #55\n"

func recoveryMergedView(mergedAt time.Time, head string, comments []pullRequestComment) pullRequestView {
	view := pullRequestView{
		BaseRefName:    "main",
		Body:           recoveryBody,
		HeadRefName:    "agent/issue-55",
		HeadRefOID:     head,
		Merged:         true,
		MergedAt:       &mergedAt,
		MergeCommitSHA: "merge-commit",
		Comments:       comments,
	}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 55})
	return view
}

func recoveryEvaluationHistory(t *testing.T, number int, head string, recordedAt time.Time) []pullRequestComment {
	t.Helper()
	return recoveryEvaluationRound(t, number, head, "recovery-challenge", "recovery-examiner", 1, "pass", recordedAt, recordedAt)
}

func recoveryEvaluationRound(t *testing.T, number int, head, challengeID, runID string, round int, verdict string,
	recordedAt, commentAt time.Time) []pullRequestComment {
	t.Helper()
	requestedAt := recordedAt.Add(-time.Minute)
	challenge := evaluationChallenge{Challenge: challengeID, Head: head, PR: number, RequestedAt: requestedAt}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("marshal recovery challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker, head),
		CreatedAt: requestedAt,
	}
	comment.Author.Login = trustedActor
	return []pullRequestComment{comment, recoveryEvaluationReceiptWithMetadata(t, number, head, challengeID, runID, round, verdict, recordedAt, commentAt)}
}

func recoveryEvaluationReceipt(t *testing.T, number int, head string, recordedAt time.Time) pullRequestComment {
	t.Helper()
	attestation := evaluationAttestation{
		Challenge: "recovery-challenge",
		Evaluator: "Examiner",
		Findings:  evaluationFindings{},
		Head:      head,
		PR:        number,
		RunID:     "recovery-examiner",
		Schema:    evaluationAttestationSchema,
		Summary:   "No blocking findings.",
		Verdict:   "pass",
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("marshal recovery attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationJSON)),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              head,
		PR:                number,
		RecordedAt:        recordedAt,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             1,
		Verdict:           attestation.Verdict,
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal recovery receipt: %v", err)
	}
	comment := pullRequestComment{
		Body:      evaluationComment(receiptJSON, attestationJSON, report),
		CreatedAt: recordedAt,
	}
	comment.Author.Login = trustedActor
	return comment
}

func recoveryEvaluationReceiptWithMetadata(t *testing.T, number int, head, challengeID, runID string, round int,
	verdict string, recordedAt, commentAt time.Time) pullRequestComment {
	t.Helper()
	findings := evaluationFindings{}
	if verdict == "fail" {
		findings = evaluationFindings{{
			Impact:             "The proof must reject a failing latest receipt.",
			Location:           "internal/workflowctl/recovery.go:1",
			RequiredCorrection: "Use the latest receipt at the merge boundary.",
		}}
	}
	attestation := evaluationAttestation{
		Challenge: challengeID,
		Evaluator: "Examiner",
		Findings:  findings,
		Head:      head,
		PR:        number,
		RunID:     runID,
		Schema:    evaluationAttestationSchema,
		Summary:   "The merge-boundary proof is recorded.",
		Verdict:   verdict,
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("marshal recovery attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationJSON)),
		BaseRefName:       "main",
		Challenge:         attestation.Challenge,
		ClosingIssues:     []int{55},
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              head,
		HeadRefName:       "agent/issue-55",
		BodySHA256:        sha256Hex([]byte(recoveryBody)),
		PR:                number,
		RecordedAt:        recordedAt,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             round,
		Verdict:           attestation.Verdict,
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal recovery receipt: %v", err)
	}
	comment := pullRequestComment{
		Body:      evaluationComment(receiptJSON, attestationJSON, report),
		CreatedAt: commentAt,
	}
	comment.Author.Login = trustedActor
	return comment
}

func TestHistoricalMergedCandidatesRequireMergedProof(t *testing.T) {
	merged := historicalPullRequest{Number: 14, Merged: true, MergeCommitSHA: "merge", State: "closed"}
	merged.Base.Ref = "main"
	merged.Body = "Closes #55"
	unmerged := merged
	unmerged.Number = 15
	unmerged.Merged = false
	unmerged.MergedAt = nil
	unmerged.MergeCommitSHA = ""
	wrongBase := merged
	wrongBase.Number = 16
	wrongBase.Base.Ref = "develop"
	candidates := historicalMergedCandidates([][]historicalPullRequest{{unmerged, wrongBase, merged}}, 55)
	if len(candidates) != 1 || candidates[0].Number != 14 {
		t.Fatalf("historical merged candidates = %#v, want only PR #14", candidates)
	}
}
