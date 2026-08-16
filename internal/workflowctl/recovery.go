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
	view, err := a.readPullRequestMetadata(root, number)
	if err != nil {
		return err
	}
	mergeSHA, err := mergedPullRequestSHA(view)
	if err != nil {
		return stateError("PR #%d is not proven merged; recovery will not remove claims: %v", number, err)
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	base, err := a.synchronizeBase(layout, mergeSHA)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	plan, err := a.prepareRecoveryCleanupPlan(root, layout, view, number)
	if err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	packet := mergedPacket{number: number, mergeSHA: mergeSHA, plan: plan}
	if err := a.cleanupClaims(base, packet); err != nil {
		return recoveryNeededError(number, mergeSHA, err)
	}
	return writeLine(a.stdout, "PR #%d was already merged at %s; Git base and proven claim cleanup are complete", number, mergeSHA)
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

func (a app) prepareRecoveryCleanupPlan(root string, layout repositoryLayout, view pullRequestView, pullRequestNumber int) (cleanupPlan, error) {
	if view.HeadRefOID == "" || view.HeadRefName == "" {
		return cleanupPlan{}, stateError("merged PR #%d has no claim head ref and SHA; preserve claim artifacts", pullRequestNumber)
	}
	if view.BaseRefName != "main" {
		return cleanupPlan{}, stateError("merged PR #%d targets base %q, not main; preserve claim artifacts", pullRequestNumber, view.BaseRefName)
	}
	primary, ok := issueFromBranch(view.HeadRefName)
	if !ok {
		return cleanupPlan{}, stateError("merged PR #%d head branch %q is not an issue claim; preserve claim artifacts", pullRequestNumber, view.HeadRefName)
	}
	if len(view.ClosingIssuesReferences) == 0 {
		return cleanupPlan{}, stateError("merged PR #%d has no closing issue proof; preserve claim artifacts", pullRequestNumber)
	}
	if len(view.ClosingIssuesReferences) > 2 {
		return cleanupPlan{}, stateError("merged PR #%d closes %d issues; preserve ambiguous claim artifacts", pullRequestNumber, len(view.ClosingIssuesReferences))
	}
	if !pullRequestCloses(view, primary) {
		return cleanupPlan{}, stateError("merged PR #%d does not close primary issue #%d; preserve claim artifacts", pullRequestNumber, primary)
	}
	claims, err := a.recoveryClaims(root, view, primary, pullRequestNumber)
	if err != nil {
		return cleanupPlan{}, err
	}
	claims, err = attachClaimWorktrees(layout, claims)
	if err != nil {
		return cleanupPlan{}, err
	}
	sort.Slice(claims, func(left, right int) bool {
		if claims[left].issue != claims[right].issue {
			return claims[left].issue < claims[right].issue
		}
		return claims[left].branch < claims[right].branch
	})
	return cleanupPlan{layout: layout, callerRoot: root, claims: claims}, nil
}

func (a app) recoveryClaims(root string, view pullRequestView, primary, pullRequestNumber int) ([]claimArtifact, error) {
	claims := []claimArtifact{{issue: primary, branch: view.HeadRefName, sha: view.HeadRefOID}}
	if len(view.ClosingIssuesReferences) > 1 {
		companions, err := a.recoveryCompanionClaims(root, view, primary, pullRequestNumber)
		if err != nil {
			return nil, err
		}
		claims = append(claims, companions...)
	}
	if err := rejectDuplicateClaimArtifacts(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func (a app) recoveryCompanionClaims(root string, view pullRequestView, primary, pullRequestNumber int) ([]claimArtifact, error) {
	remoteClaims, err := a.remoteClaimRefs(root)
	if err != nil {
		return nil, err
	}
	localClaims, err := a.localClaimRefs(root)
	if err != nil {
		return nil, err
	}
	remoteClaims = appendClaimRefs(remoteClaims, localClaims)
	if hasHistoricalCompanionCandidate(remoteClaims, view, primary) {
		if err := a.ensureRecoveryHead(root, pullRequestNumber, view.HeadRefOID); err != nil {
			return nil, err
		}
	}
	companions := make([]claimArtifact, 0, 1)
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == primary {
			continue
		}
		candidate, found, err := a.historicalCompanionClaim(root, issue.Number, view.HeadRefOID, remoteClaims)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		companions = append(companions, claimArtifact{issue: issue.Number, branch: candidate.branch, sha: candidate.sha})
	}
	return companions, nil
}

func hasHistoricalCompanionCandidate(claims []remoteClaim, view pullRequestView, primary int) bool {
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == primary {
			continue
		}
		for _, claim := range claims {
			if claim.number == issue.Number {
				return true
			}
		}
	}
	return false
}

func (a app) ensureRecoveryHead(root string, pullRequestNumber int, head string) error {
	if _, err := a.command(root, "git", "cat-file", "-e", head+"^{commit}"); err == nil {
		return nil
	}
	ref := "refs/pull/" + strconv.Itoa(pullRequestNumber) + "/head"
	if _, err := a.command(root, "git", "fetch", "--no-tags", "origin", ref); err != nil {
		return fmt.Errorf("fetch merged PR #%d head for companion proof: %w", pullRequestNumber, err)
	}
	if _, err := a.command(root, "git", "cat-file", "-e", head+"^{commit}"); err != nil {
		return fmt.Errorf("verify merged PR #%d head for companion proof: %w", pullRequestNumber, err)
	}
	return nil
}

func (a app) historicalCompanionClaim(root string, issue int, head string, remoteClaims []remoteClaim) (remoteClaim, bool, error) {
	candidates := make([]remoteClaim, 0, 1)
	for _, claim := range remoteClaims {
		if claim.number != issue {
			continue
		}
		included, err := a.historicalClaimIncluded(root, claim, head)
		if err != nil {
			return remoteClaim{}, false, err
		}
		if included {
			candidates = append(candidates, claim)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true, nil
	}
	if len(candidates) > 1 {
		return remoteClaim{}, false, stateError("historical companion issue #%d has ambiguous merged claim proof", issue)
	}
	return remoteClaim{}, false, nil
}

func (a app) historicalClaimIncluded(root string, claim remoteClaim, head string) (bool, error) {
	remote, err := a.remoteClaimPresent(root, claim.branch)
	if err != nil {
		return false, err
	}
	if remote {
		if _, err := a.command(root, "git", "fetch", "--no-tags", "origin", "refs/heads/"+claim.branch); err != nil {
			return false, fmt.Errorf("fetch historical claim %s for proof: %w", claim.branch, err)
		}
	}
	if _, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head); err != nil {
		if isGitNonAncestor(err) {
			return false, nil
		}
		return false, fmt.Errorf("prove historical claim %s: %w", claim.branch, err)
	}
	return true, nil
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
		number, ok := issueFromBranch(branch)
		if !ok {
			continue
		}
		claims = append(claims, remoteClaim{branch: branch, number: number, sha: fields[1]})
	}
	sort.Slice(claims, func(left, right int) bool {
		return claims[left].branch < claims[right].branch
	})
	return claims, nil
}

func appendClaimRefs(primary, additional []remoteClaim) []remoteClaim {
	result := append([]remoteClaim(nil), primary...)
	for _, candidate := range additional {
		found := false
		for _, current := range result {
			if current.branch == candidate.branch {
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].branch < result[right].branch
	})
	return result
}

func (a app) remoteClaimPresent(root, branch string) (bool, error) {
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return false, fmt.Errorf("inspect remote historical claim %s: %w", branch, err)
	}
	_, present, err := exactRemoteRef(output, "refs/heads/"+branch)
	return present, err
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
