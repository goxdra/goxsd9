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
	view := pullRequestView{
		BaseRefName:    "main",
		HeadRefName:    "agent/issue-55",
		HeadRefOID:     "advanced-head",
		Merged:         true,
		MergedAt:       &mergedAt,
		MergeCommitSHA: "merge-commit",
		Comments:       []pullRequestComment{recoveryEvaluationReceipt(t, 14, evaluatedHead, mergedAt.Add(-time.Minute))},
	}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 55})

	got, err := mergeTimeEvaluatedHead(view, 14)
	if err != nil {
		t.Fatalf("mergeTimeEvaluatedHead error: %v", err)
	}
	if got != evaluatedHead {
		t.Fatalf("mergeTimeEvaluatedHead = %q, want %q", got, evaluatedHead)
	}

	application := app{}
	_, err = application.prepareRecoveryCleanupPlan("/repo", repositoryLayout{primaryRoot: "/repo"}, view, 14, got)
	if err == nil || !strings.Contains(err.Error(), "differs from immutable merge-time evaluated head") {
		t.Fatalf("prepareRecoveryCleanupPlan error = %v, want advanced-head refusal", err)
	}
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
