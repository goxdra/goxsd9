package workflowctl

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type historicalPullRequest struct {
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Body string `json:"body"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Merged         bool       `json:"merged"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommitSHA string     `json:"merge_commit_sha"`
	Number         int        `json:"number"`
	State          string     `json:"state"`
}

func (a app) recoverPullRequestCommand(args []string) error {
	if len(args) != 1 {
		return usageError("usage: workflowctl pr recover PR")
	}
	number, err := positiveNumber(args[0])
	if err != nil {
		return usageError("pr recover: %v", err)
	}
	return a.recoverPullRequest(number)
}

func (a app) recoverPullRequest(number int) error {
	root, err := a.root()
	if err != nil {
		return err
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return err
	}
	mergeSHA, err := mergedPullRequestSHA(view)
	if err != nil {
		return stateError("PR #%d is not proven merged; recovery will not remove claims: %v", number, err)
	}
	proof, err := mergeTimeEvaluationProof(view, number)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	base, err := a.synchronizeBase(layout, mergeSHA)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	plan, err := a.prepareRecoveryCleanupPlanWithProof(root, layout, view, number, proof)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	if err := a.reconcileMergedPrimaryIssue(root, number, proof, plan.primaryIssue); err != nil {
		return recoveryNeededError(number, mergeSHA, fmt.Errorf("primary issue reconciliation: %w", err))
	}
	if err := a.setIssueProjectStatus(root, plan.primaryIssue, "Done"); err != nil {
		return recoveryNeededError(number, mergeSHA, fmt.Errorf("primary Project status reconciliation: %w", err))
	}
	packet := mergedPacket{number: number, mergeSHA: mergeSHA, plan: plan}
	if err := a.cleanupClaims(base, packet); err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	return writeLine(a.stdout, "PR #%d was already merged at %s; Git base and proven claim cleanup are complete", number, mergeSHA)
}

func mergeEvaluationProofFromReceipt(receipt evaluationReceipt) mergeEvaluationProof {
	return mergeEvaluationProof{
		bodySHA256:    receipt.BodySHA256,
		baseRefName:   receipt.BaseRefName,
		claimProofs:   append([]evaluationClaimProof(nil), receipt.ClaimProofs...),
		closingIssues: append([]int(nil), receipt.ClosingIssues...),
		head:          receipt.Head,
		headRefName:   receipt.HeadRefName,
	}
}

func (a app) reconcileMergedPrimaryIssue(root string, pullRequestNumber int, proof mergeEvaluationProof,
	expectedPrimary int,
) error {
	if !hasMergeEvaluationMetadata(proof) {
		return stateError("PR #%d lacks immutable primary closure metadata; preserve claim artifacts", pullRequestNumber)
	}
	if proof.baseRefName != "main" {
		return stateError("PR #%d immutable primary closure proof targets base %q; preserve claim artifacts", pullRequestNumber, proof.baseRefName)
	}
	primary, err := recoveryPrimary(proof, pullRequestNumber)
	if err != nil {
		return err
	}
	if expectedPrimary != primary {
		return stateError("PR #%d proven primary issue #%d differs from cleanup primary issue #%d; preserve claim artifacts", pullRequestNumber, primary, expectedPrimary)
	}
	if _, err := recoveryClaimProofs(proof, primary, pullRequestNumber); err != nil {
		return fmt.Errorf("immutable claim proof: %w", err)
	}
	return a.reconcileIssueClosed(root, primary)
}

func recoveryNeededError(number int, mergeSHA string, cause error) error {
	return stateError("PR #%d is already merged at %s, but recovery is still needed: %w. Run `go tool workflowctl pr recover %d` again", number, mergeSHA, cause, number)
}

func mergedPullRequestSHA(view pullRequestView) (string, error) {
	if !view.Merged && view.MergedAt == nil {
		return "", errors.New("github does not report a completed merge")
	}
	if strings.TrimSpace(view.MergeCommitSHA) == "" {
		return "", errors.New("github reported a merge without merge commit SHA")
	}
	return view.MergeCommitSHA, nil
}

type mergeEvaluationProof struct {
	bodySHA256    string
	baseRefName   string
	claimProofs   []evaluationClaimProof
	closingIssues []int
	head          string
	headRefName   string
}

func mergeTimeEvaluationProof(view pullRequestView, number int) (mergeEvaluationProof, error) {
	selected, err := mergeBoundaryEvaluationReceipt(view, number)
	if err != nil {
		return mergeEvaluationProof{}, err
	}
	return mergeEvaluationProof{
		bodySHA256:    selected.BodySHA256,
		baseRefName:   selected.BaseRefName,
		claimProofs:   append([]evaluationClaimProof(nil), selected.ClaimProofs...),
		closingIssues: append([]int(nil), selected.ClosingIssues...),
		head:          selected.Head,
		headRefName:   selected.HeadRefName,
	}, nil
}

func mergeBoundaryEvaluationReceipt(view pullRequestView, number int) (evaluationReceipt, error) {
	if view.MergedAt == nil {
		return evaluationReceipt{}, errors.New("github reported a merge without a merge timestamp for immutable evaluation proof")
	}
	if err := rejectUntrustedEvaluationEvidence(view.Comments); err != nil {
		return evaluationReceipt{}, fmt.Errorf("validate immutable pre-merge evaluation proof: %w", err)
	}
	history, err := parseEvaluationHistory(view.Comments)
	if err != nil {
		return evaluationReceipt{}, fmt.Errorf("read immutable pre-merge evaluation proof: %w", err)
	}
	if historyErr := validateEvaluationHistory(history); historyErr != nil {
		return evaluationReceipt{}, fmt.Errorf("read immutable pre-merge evaluation proof: %w", historyErr)
	}
	mergeAt := view.MergedAt.UTC()
	if boundaryErr := validateMergeBoundaryHistory(history, mergeAt); boundaryErr != nil {
		return evaluationReceipt{}, boundaryErr
	}
	selected, err := latestMergeBoundaryReceipt(history)
	if err != nil {
		return evaluationReceipt{}, err
	}
	if err := validateMergeBoundaryReceipt(selected.receipt, number); err != nil {
		return evaluationReceipt{}, err
	}
	return selected.receipt, nil
}

func validateMergeBoundaryHistory(history evaluationHistory, mergeAt time.Time) error {
	if err := validateMergeBoundaryChallenges(history.challenges, mergeAt); err != nil {
		return err
	}
	if err := validateMergeBoundaryReceipts(history.receipts, mergeAt); err != nil {
		return err
	}
	return validateMergeBoundaryResolutions(history.resolutions, mergeAt)
}

func validateMergeBoundaryChallenges(challenges []evaluationChallengeRecord, mergeAt time.Time) error {
	for _, challenge := range challenges {
		if challenge.comment.CreatedAt.IsZero() || challenge.comment.CreatedAt.After(mergeAt) ||
			challenge.challenge.RequestedAt.IsZero() || challenge.challenge.RequestedAt.After(mergeAt) {
			return errors.New("trusted evaluation challenge was created after the merge boundary")
		}
	}
	return nil
}

func validateMergeBoundaryReceipts(receipts []evaluationReceiptRecord, mergeAt time.Time) error {
	for _, record := range receipts {
		if record.comment.CreatedAt.IsZero() || record.comment.CreatedAt.After(mergeAt) {
			return errors.New("trusted evaluation receipt was created after the merge boundary")
		}
		if record.receipt.RecordedAt.After(mergeAt) {
			return errors.New("evaluation receipt record time is after the merge boundary")
		}
	}
	return nil
}

func validateMergeBoundaryResolutions(resolutions []evaluationResolutionRecord, mergeAt time.Time) error {
	for _, record := range resolutions {
		if record.comment.CreatedAt.IsZero() || record.comment.CreatedAt.After(mergeAt) {
			return errors.New("trusted no-verdict resolution was created after the merge boundary")
		}
		if record.resolution.ResolvedAt.After(mergeAt) {
			return errors.New("no-verdict resolution time is after the merge boundary")
		}
	}
	return nil
}

func latestMergeBoundaryReceipt(history evaluationHistory) (evaluationReceiptRecord, error) {
	if len(history.receipts) == 0 {
		return evaluationReceiptRecord{}, errors.New("no trusted evaluation receipt proves an immutable pre-merge head")
	}
	selected := history.receipts[0]
	for _, record := range history.receipts[1:] {
		if record.comment.CreatedAt.After(selected.comment.CreatedAt) {
			selected = record
			continue
		}
		if record.comment.CreatedAt.Equal(selected.comment.CreatedAt) {
			return evaluationReceiptRecord{}, errors.New("pre-merge evaluation proof has ambiguous receipts at the merge boundary")
		}
	}
	if !latestEvaluationReceiptClosesLatestChallenge(history) {
		return evaluationReceiptRecord{}, errors.New("latest challenge was not closed by a passing attested receipt; a no-verdict resolution cannot prove merge")
	}
	return selected, nil
}

func validateMergeBoundaryReceipt(receipt evaluationReceipt, number int) error {
	if receipt.PR != number || receipt.Verdict != "pass" || receipt.AttestationSHA256 == "" {
		return errors.New("latest trusted evaluation receipt at the merge boundary is not a passing proof")
	}
	if !validEvaluationReceiptMetadata(receipt) || !hasEvaluationReceiptMetadata(receipt) {
		return errors.New("latest trusted evaluation receipt lacks immutable pull request metadata")
	}
	if len(receipt.ClosingIssues) > 1 && receipt.ClaimProofs == nil {
		return errors.New("latest trusted evaluation receipt lacks immutable companion claim proof")
	}
	return nil
}

func hasEvaluationReceiptMetadata(receipt evaluationReceipt) bool {
	return receipt.BaseRefName != "" && len(receipt.ClosingIssues) != 0 && receipt.HeadRefName != "" &&
		receipt.BodySHA256 != ""
}

func (a app) prepareRecoveryCleanupPlan(root string, layout repositoryLayout, view pullRequestView, pullRequestNumber int, evaluatedHead string) (cleanupPlan, error) {
	return a.prepareRecoveryCleanupPlanWithProof(root, layout, view, pullRequestNumber, mergeEvaluationProof{head: evaluatedHead})
}

func (a app) prepareRecoveryCleanupPlanWithProof(root string, layout repositoryLayout, view pullRequestView, pullRequestNumber int, proof mergeEvaluationProof) (cleanupPlan, error) {
	if err := validateRecoveryMetadata(view, proof, pullRequestNumber); err != nil {
		return cleanupPlan{}, err
	}
	primary, err := recoveryPrimary(proof, pullRequestNumber)
	if err != nil {
		return cleanupPlan{}, err
	}
	claims, err := a.recoveryClaims(proof, primary, pullRequestNumber)
	if err != nil {
		return cleanupPlan{}, err
	}
	err = a.ensureRecoveryHead(root, pullRequestNumber, proof.head)
	if err != nil {
		return cleanupPlan{}, err
	}
	claims, err = attachClaimWorktrees(layout, claims)
	if err != nil {
		return cleanupPlan{}, err
	}
	if len(claims) > 1 {
		if err := a.validateClaimArtifacts(root, layout, claims, proof.head, primary, true); err != nil {
			return cleanupPlan{}, err
		}
	}
	sort.Slice(claims, func(left, right int) bool {
		if claims[left].issue != claims[right].issue {
			return claims[left].issue < claims[right].issue
		}
		return claims[left].branch < claims[right].branch
	})
	return cleanupPlan{layout: layout, callerRoot: root, claims: claims, proofHead: proof.head, primaryIssue: primary, allowPrimaryMissing: true, validateArtifacts: true}, nil
}

func validateRecoveryMetadata(view pullRequestView, proof mergeEvaluationProof, pullRequestNumber int) error {
	if view.HeadRefOID == "" || view.HeadRefName == "" {
		return stateError("merged PR #%d has no claim head ref and SHA; preserve claim artifacts", pullRequestNumber)
	}
	if proof.head == "" {
		return stateError("merged PR #%d has no immutable evaluated head proof; preserve claim artifacts", pullRequestNumber)
	}
	if view.HeadRefOID != proof.head {
		return stateError("merged PR #%d current PR head %s differs from immutable merge-time evaluated head %s; preserve claim artifacts", pullRequestNumber, view.HeadRefOID, proof.head)
	}
	if !hasMergeEvaluationMetadata(proof) {
		return stateError("merged PR #%d lacks immutable base, head-ref, closure, or PR-body proof; preserve claim artifacts", pullRequestNumber)
	}
	if view.HeadRefName != proof.headRefName {
		return stateError("merged PR #%d current head ref %q differs from immutable merge-time head ref %q; preserve claim artifacts", pullRequestNumber, view.HeadRefName, proof.headRefName)
	}
	if view.BaseRefName != proof.baseRefName {
		return stateError("merged PR #%d current base %q differs from immutable merge-time base %q; preserve claim artifacts", pullRequestNumber, view.BaseRefName, proof.baseRefName)
	}
	if proof.baseRefName != "main" {
		return stateError("merged PR #%d targets base %q, not main; preserve claim artifacts", pullRequestNumber, proof.baseRefName)
	}
	if sha256Hex([]byte(view.Body)) != proof.bodySHA256 {
		return stateError("merged PR #%d PR body differs from immutable merge-time body proof; preserve claim artifacts", pullRequestNumber)
	}
	if !sameIssueNumbers(closingIssueNumbers(view.Body), proof.closingIssues) {
		return stateError("merged PR #%d closure references differ from immutable merge-time proof; preserve claim artifacts", pullRequestNumber)
	}
	return nil
}

func recoveryPrimary(proof mergeEvaluationProof, pullRequestNumber int) (int, error) {
	primary, ok := fixedClaimIssue(proof.headRefName)
	if !ok {
		return 0, stateError("merged PR #%d head branch %q is not an issue claim; preserve claim artifacts", pullRequestNumber, proof.headRefName)
	}
	if len(proof.closingIssues) == 0 {
		return 0, stateError("merged PR #%d has no closing issue proof; preserve claim artifacts", pullRequestNumber)
	}
	if len(proof.closingIssues) > 2 {
		return 0, stateError("merged PR #%d closes %d issues; preserve ambiguous claim artifacts", pullRequestNumber, len(proof.closingIssues))
	}
	if !containsNumber(proof.closingIssues, primary) {
		return 0, stateError("merged PR #%d does not close primary issue #%d; preserve claim artifacts", pullRequestNumber, primary)
	}
	return primary, nil
}

func hasMergeEvaluationMetadata(proof mergeEvaluationProof) bool {
	return proof.baseRefName != "" && proof.headRefName != "" && proof.bodySHA256 != "" && len(proof.closingIssues) != 0
}

func sameIssueNumbers(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (a app) recoveryClaims(proof mergeEvaluationProof, primary, pullRequestNumber int) ([]claimArtifact, error) {
	proofs, err := recoveryClaimProofs(proof, primary, pullRequestNumber)
	if err != nil {
		return nil, err
	}
	claims := make([]claimArtifact, 0, len(proofs))
	for _, claim := range proofs {
		claims = append(claims, claimArtifact{issue: claim.Issue, branch: claim.Branch, sha: claim.SHA})
	}
	if err := rejectDuplicateClaimArtifacts(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func recoveryClaimProofs(proof mergeEvaluationProof, primary, pullRequestNumber int) ([]evaluationClaimProof, error) {
	if proof.claimProofs == nil {
		if len(proof.closingIssues) > 1 {
			return nil, stateError("merged PR #%d lacks immutable companion branch and SHA proof; preserve claim artifacts", pullRequestNumber)
		}
		return []evaluationClaimProof{{Issue: primary, Branch: proof.headRefName, SHA: proof.head}}, nil
	}
	if len(proof.claimProofs) != len(proof.closingIssues) {
		return nil, stateError("merged PR #%d has incomplete immutable claim proof; preserve claim artifacts", pullRequestNumber)
	}
	if err := validateRecoveryClaimProofs(proof.claimProofs, proof.closingIssues, pullRequestNumber); err != nil {
		return nil, err
	}
	primaryClaim, ok := claimProofForIssue(proof.claimProofs, primary)
	if !ok || primaryClaim.Branch != proof.headRefName || primaryClaim.SHA != proof.head {
		return nil, stateError("merged PR #%d primary immutable claim proof changed; preserve claim artifacts", pullRequestNumber)
	}
	return append([]evaluationClaimProof(nil), proof.claimProofs...), nil
}

func validateRecoveryClaimProofs(proofs []evaluationClaimProof, issues []int, pullRequestNumber int) error {
	seen := make(map[int]struct{}, len(proofs))
	for _, claim := range proofs {
		if claim.Issue < 1 || claim.Branch == "" || claim.SHA == "" {
			return stateError("merged PR #%d has malformed immutable claim proof; preserve claim artifacts", pullRequestNumber)
		}
		issue, ok := fixedClaimIssue(claim.Branch)
		if !ok || issue != claim.Issue {
			return stateError("merged PR #%d immutable claim proof branch %q does not match issue #%d; preserve claim artifacts", pullRequestNumber, claim.Branch, claim.Issue)
		}
		if _, ok := seen[claim.Issue]; ok {
			return stateError("merged PR #%d has duplicate immutable claim proof for issue #%d; preserve claim artifacts", pullRequestNumber, claim.Issue)
		}
		seen[claim.Issue] = struct{}{}
	}
	for _, issue := range issues {
		if _, ok := seen[issue]; !ok {
			return stateError("merged PR #%d lacks immutable claim proof for issue #%d; preserve claim artifacts", pullRequestNumber, issue)
		}
	}
	return nil
}

func claimProofForIssue(proofs []evaluationClaimProof, issue int) (evaluationClaimProof, bool) {
	for _, proof := range proofs {
		if proof.Issue == issue {
			return proof, true
		}
	}
	return evaluationClaimProof{}, false
}

func (a app) ensureRecoveryHead(root string, pullRequestNumber int, head string) error {
	if _, err := a.command(root, "git", "cat-file", "-e", head+"^{commit}"); err == nil {
		return nil
	}
	ref := "refs/pull/" + strconv.Itoa(pullRequestNumber) + "/head"
	if _, err := a.command(root, "git", "fetch", "--no-tags", "origin", ref); err != nil {
		return fmt.Errorf("fetch merged PR #%d evaluated head for run-local proof: %w", pullRequestNumber, err)
	}
	if _, err := a.command(root, "git", "cat-file", "-e", head+"^{commit}"); err != nil {
		return fmt.Errorf("verify merged PR #%d evaluated head for run-local proof: %w", pullRequestNumber, err)
	}
	return nil
}

func (a app) localClaimRefs(root string) ([]remoteClaim, error) {
	output, err := a.command(root, "git", "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/agent/issue-*")
	if err != nil {
		return nil, fmt.Errorf("list local claim refs: %w", err)
	}
	var claims []remoteClaim
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		branch := fields[0]
		kind, number, _ := classifyAgentRef(branch)
		if kind != agentRefClaim {
			continue
		}
		claims = append(claims, remoteClaim{branch: branch, number: number, sha: fields[1], source: claimRefLocal})
	}
	sort.Slice(claims, func(left, right int) bool {
		if claims[left].branch != claims[right].branch {
			return claims[left].branch < claims[right].branch
		}
		return claims[left].sha < claims[right].sha
	})
	return claims, nil
}

func (a app) pruneHistoricalClaimsCommand(args []string) error {
	if len(args) != 1 {
		return usageError("usage: workflowctl claim prune ISSUE")
	}
	issue, err := positiveNumber(args[0])
	if err != nil {
		return usageError("claim prune: %v", err)
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	output, err := a.command(root, "gh", "api", "--paginate", "repos/"+repositoryKey+"/pulls?state=closed&base=main&per_page=100")
	if err != nil {
		return fmt.Errorf("list closed PRs for historical claim #%d proof: %w", issue, err)
	}
	pages, err := decodeJSONDocuments[[]historicalPullRequest](output)
	if err != nil {
		return fmt.Errorf("decode closed PRs for historical claim #%d proof: %w", issue, err)
	}
	candidates := historicalMergedCandidates(pages, issue)
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].Number < candidates[right].Number
	})
	if len(candidates) == 0 {
		return stateError("historical claim #%d has no uniquely proven merged PR; preserve artifacts", issue)
	}
	if len(candidates) > 1 {
		return stateError("historical claim #%d matches multiple merged PRs; preserve ambiguous artifacts", issue)
	}
	return a.recoverPullRequest(candidates[0].Number)
}

func historicalMergedCandidates(pages [][]historicalPullRequest, issue int) []historicalPullRequest {
	candidates := make([]historicalPullRequest, 0, 1)
	for _, page := range pages {
		for _, pull := range page {
			if !historicalPullRequestIsMerged(pull) || !closingIssueNumbersContain(pull.Body, issue) {
				continue
			}
			candidates = append(candidates, pull)
		}
	}
	return candidates
}

func historicalPullRequestIsMerged(pull historicalPullRequest) bool {
	if pull.Number < 1 || strings.ToUpper(pull.State) != "CLOSED" || pull.Base.Ref != "main" {
		return false
	}
	if !pull.Merged && pull.MergedAt == nil {
		return false
	}
	return strings.TrimSpace(pull.MergeCommitSHA) != ""
}

func closingIssueNumbersContain(body string, issue int) bool {
	for _, number := range closingIssueNumbers(body) {
		if number == issue {
			return true
		}
	}
	return false
}
