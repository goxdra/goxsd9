package workflowctl

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestRecoveryPrimaryOnlyRequiresEvaluatedHeadObjectBeforeRunLocalProof(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	view := recoveryMergedView(mergedAt, recoveryEvaluationHistory(t, 14, "evaluated-head", mergedAt.Add(-time.Minute)))
	proof, err := mergeTimeEvaluationProof(view, 14)
	if err != nil {
		t.Fatalf("mergeTimeEvaluationProof: %v", err)
	}
	commands := []string{}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "git cat-file -e evaluated-head^{commit}":
			return "", errors.New("evaluated head object is absent")
		case "git fetch --no-tags origin refs/pull/14/head":
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	_, err = application.prepareRecoveryCleanupPlanWithProof("/repo", repositoryLayout{primaryRoot: "/repo"}, view, 14, proof)
	if err == nil || !strings.Contains(err.Error(), "evaluated head for run-local proof") {
		t.Fatalf("primary-only recovery without evaluated object = %v, want proof-head refusal", err)
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "git log ") || strings.Contains(command, "run-local-proof") {
			t.Fatalf("run-local history was inspected without the evaluated object: %v", commands)
		}
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
	view := recoveryMergedView(mergedAt, comments)
	if _, err := mergeTimeEvaluationProof(view, 14); err == nil || !strings.Contains(err.Error(), "latest trusted evaluation receipt") {
		t.Fatalf("mergeTimeEvaluationProof error = %v, want latest-failure refusal", err)
	}
}

func TestRecoveryRejectsUnresolvedEarlierChallengeBeforePassingLaterReceipt(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	comments := make([]pullRequestComment, 0, 3)
	comments = append(comments,
		recoveryEvaluationChallengeComment(t, 14, "evaluated-head", "earlier-challenge", mergedAt.Add(-3*time.Minute)),
	)
	comments = append(comments,
		recoveryEvaluationRound(t, 14, "evaluated-head", "later-challenge", "later-run", 1, "pass",
			mergedAt.Add(-time.Minute), mergedAt.Add(-time.Minute))...,
	)
	view := recoveryMergedView(mergedAt, comments)
	if _, err := mergeTimeEvaluationProof(view, 14); err == nil || !strings.Contains(err.Error(), "outstanding trusted Examiner challenge") {
		t.Fatalf("mergeTimeEvaluationProof error = %v, want unresolved-earlier-challenge refusal", err)
	}
}

func TestRecoveryRejectsPostMergeReceiptTimestampSkew(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	comments := recoveryEvaluationRound(t, 14, "evaluated-head", "skew-challenge", "skew-run", 1, "pass",
		mergedAt.Add(-time.Minute), mergedAt.Add(time.Minute))
	view := recoveryMergedView(mergedAt, comments)
	if _, err := mergeTimeEvaluationProof(view, 14); err == nil || !strings.Contains(err.Error(), "created after the merge boundary") {
		t.Fatalf("mergeTimeEvaluationProof error = %v, want post-merge timestamp refusal", err)
	}
}

func TestRecoveryRejectsPRBodyDrift(t *testing.T) {
	mergedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	view := recoveryMergedView(mergedAt,
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

func TestLatestPassingEvaluationRejectsCurrentPRMetadataDrift(t *testing.T) {
	recordedAt := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	base := recoveryMergedView(recordedAt.Add(time.Minute), recoveryEvaluationHistory(t, 14, "evaluated-head", recordedAt))
	tests := []struct {
		name   string
		mutate func(*pullRequestView)
		want   string
	}{
		{name: "base", mutate: func(view *pullRequestView) { view.BaseRefName = "develop" }, want: "current PR base"},
		{name: "head ref", mutate: func(view *pullRequestView) { view.HeadRefName = "agent/issue-56" }, want: "current PR head ref"},
		{name: "closure", mutate: func(view *pullRequestView) {
			view.Body += "\nCloses #56\n"
		}, want: "current PR closure"},
		{name: "body", mutate: func(view *pullRequestView) {
			view.Body += "\nAdditional reviewed context.\n"
		}, want: "current PR body"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			view := base
			test.mutate(&view)
			if _, err := latestPassingEvaluationReceipt(view, 14); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("latestPassingEvaluationReceipt error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRecoveryRequiresImmutableCompanionClaimProof(t *testing.T) {
	proof := mergeEvaluationProof{
		head:          "evaluated-head",
		headRefName:   "agent/issue-55",
		closingIssues: []int{55, 56},
	}
	if _, err := recoveryClaimProofs(proof, 55, 14); err == nil || !strings.Contains(err.Error(), "companion") {
		t.Fatalf("recoveryClaimProofs error = %v, want missing companion proof", err)
	}
	proof.claimProofs = []evaluationClaimProof{
		{Issue: 55, Branch: "agent/issue-55", SHA: "evaluated-head"},
		{Issue: 56, Branch: "agent/issue-56", SHA: "companion-head"},
	}
	claims, err := recoveryClaimProofs(proof, 55, 14)
	if err != nil || len(claims) != 2 {
		t.Fatalf("recoveryClaimProofs valid proof = %#v, %v; want two claims", claims, err)
	}
	proof.claimProofs[1].SHA = ""
	if _, err := recoveryClaimProofs(proof, 55, 14); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("recoveryClaimProofs malformed proof error = %v, want malformed refusal", err)
	}
}

func TestClaimProofsOnlyReceiptIsNotCompleteMetadata(t *testing.T) {
	receipt := evaluationReceipt{
		ClaimProofs: []evaluationClaimProof{{Issue: 55, Branch: "agent/issue-55", SHA: "head"}},
	}
	if validEvaluationReceiptMetadata(receipt) {
		t.Fatal("ClaimProofs-only receipt was accepted as complete metadata")
	}
	proof := mergeEvaluationProof{head: "head", headRefName: "agent/issue-55", closingIssues: []int{55}, claimProofs: []evaluationClaimProof{}}
	if _, err := recoveryClaimProofs(proof, 55, 14); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("non-nil empty claim proof = %v, want incomplete-proof refusal", err)
	}
}

const recoveryBody = "Closes #55\n"

func recoveryMergedView(mergedAt time.Time, comments []pullRequestComment) pullRequestView {
	view := pullRequestView{
		BaseRefName:    "main",
		Body:           recoveryBody,
		HeadRefName:    "agent/issue-55",
		HeadRefOID:     "evaluated-head",
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
	comment := recoveryEvaluationChallengeComment(t, number, head, challengeID, requestedAt)
	return []pullRequestComment{comment, recoveryEvaluationReceiptWithMetadata(t, number, head, challengeID, runID, round, verdict, recordedAt, commentAt)}
}

func recoveryEvaluationChallengeComment(t *testing.T, number int, head, challengeID string, requestedAt time.Time) pullRequestComment {
	t.Helper()
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
	return comment
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

func TestHistoricalMergedCandidatesUseEffectiveClosingReferences(t *testing.T) {
	canonical := historicalPullRequest{Number: 14, Merged: true, MergeCommitSHA: "merge", State: "closed"}
	canonical.Base.Ref = "main"
	canonical.Body = "Closes #55"
	inline := canonical
	inline.Number = 15
	inline.Body = "`Closes #55`"
	fenced := canonical
	fenced.Number = 16
	fenced.Body = "```\nCloses #55\n```"
	quoted := canonical
	quoted.Number = 17
	quoted.Body = "> Closes #55"
	candidates := historicalMergedCandidates([][]historicalPullRequest{{inline, fenced, quoted, canonical}}, 55)
	if len(candidates) != 1 || candidates[0].Number != 14 {
		t.Fatalf("historical effective closing candidates = %#v, want only PR #14", candidates)
	}
}
