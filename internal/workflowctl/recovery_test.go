package workflowctl

import (
	"errors"
	"strings"
	"testing"
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
	}, 14)
	if err == nil || !strings.Contains(err.Error(), "claim head ref") {
		t.Fatalf("prepareRecoveryCleanupPlan error = %v, want missing-head refusal", err)
	}
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
