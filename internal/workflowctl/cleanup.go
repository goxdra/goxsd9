package workflowctl

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type claimArtifact struct {
	issue        int
	branch       string
	sha          string
	worktreePath string
}

type cleanupPlan struct {
	layout              repositoryLayout
	callerRoot          string
	claims              []claimArtifact
	proofHead           string
	primaryIssue        int
	allowPrimaryMissing bool
	validateArtifacts   bool
}

type mergedPacket struct {
	number   int
	mergeSHA string
	plan     cleanupPlan
}

func (a app) prepareCleanupPlan(root string, layout repositoryLayout, view pullRequestView, primary int, pullRequestNumbers ...int) (cleanupPlan, error) {
	pullRequestNumber := primary
	if len(pullRequestNumbers) != 0 {
		pullRequestNumber = pullRequestNumbers[0]
	}
	claims, proofHead, proofBound, err := a.cleanupClaimsForPlan(root, view, primary, pullRequestNumber)
	if err != nil {
		return cleanupPlan{}, err
	}
	claims, err = attachClaimWorktrees(layout, claims)
	if err != nil {
		return cleanupPlan{}, err
	}
	if len(claims) > 1 {
		proofBound = true
	}
	if proofBound {
		if err := a.validateClaimArtifacts(root, layout, claims, proofHead, primary, false); err != nil {
			return cleanupPlan{}, err
		}
	}
	if !hasWorktreeForRoot(claims, root) {
		return cleanupPlan{}, stateError("current claim worktree %q is not uniquely registered in Git worktree inventory; preserve claims and run go tool workflowctl pr recover %d", root, primary)
	}
	sort.Slice(claims, func(left, right int) bool {
		leftCurrent := samePath(claims[left].worktreePath, root)
		rightCurrent := samePath(claims[right].worktreePath, root)
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		if claims[left].issue != claims[right].issue {
			return claims[left].issue < claims[right].issue
		}
		return claims[left].branch < claims[right].branch
	})
	return cleanupPlan{layout: layout, callerRoot: root, claims: claims, proofHead: proofHead, primaryIssue: primary, validateArtifacts: proofBound}, nil
}

func (a app) cleanupClaimsForPlan(root string, view pullRequestView, primary, pullRequestNumber int) ([]claimArtifact, string, bool, error) {
	if len(view.Comments) == 0 {
		claims, err := a.cleanupClaimsForView(root, view, primary)
		return claims, view.HeadRefOID, false, err
	}
	receipt, err := latestPassingEvaluationReceipt(view, pullRequestNumber)
	if err != nil {
		return nil, "", true, err
	}
	claims, err := a.cleanupClaimsForReceipt(root, view, primary, receipt)
	return claims, receipt.Head, true, err
}

func (a app) cleanupClaimsForReceipt(root string, view pullRequestView, primary int, receipt evaluationReceipt) ([]claimArtifact, error) {
	proof := mergeEvaluationProof{
		claimProofs:   append([]evaluationClaimProof(nil), receipt.ClaimProofs...),
		closingIssues: append([]int(nil), receipt.ClosingIssues...),
		head:          receipt.Head,
		headRefName:   receipt.HeadRefName,
	}
	claims, err := recoveryClaimProofs(proof, primary, receipt.PR)
	if err != nil {
		return nil, err
	}
	artifacts := make([]claimArtifact, 0, len(claims))
	for _, claim := range claims {
		artifacts = append(artifacts, claimArtifact{issue: claim.Issue, branch: claim.Branch, sha: claim.SHA})
	}
	localHead, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read primary claim head before merge: %w", err)
	}
	if localHead != receipt.Head || view.HeadRefOID != receipt.Head {
		return nil, stateError("PR evaluated head %s does not match immutable evaluation head %s; preserve claim artifacts", view.HeadRefOID, receipt.Head)
	}
	if view.HeadRefName != receipt.HeadRefName {
		return nil, stateError("PR head ref %q does not match immutable evaluation head ref %q; preserve claim artifacts", view.HeadRefName, receipt.HeadRefName)
	}
	if !pullRequestCloses(view, primary) {
		return nil, stateError("PR does not close claimed issue #%d", primary)
	}
	return artifacts, nil
}

func (a app) cleanupClaimsForView(root string, view pullRequestView, primary int) ([]claimArtifact, error) {
	if len(view.ClosingIssuesReferences) == 0 {
		return nil, stateError("PR has no closing issue proof for claim cleanup")
	}
	if view.HeadRefOID == "" {
		return nil, stateError("PR has no evaluated head for claim cleanup")
	}
	if issue, ok := issueFromBranch(view.HeadRefName); !ok || issue != primary {
		return nil, stateError("PR head branch %q is not the primary issue #%d claim", view.HeadRefName, primary)
	}
	localHead, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read primary claim head before merge: %w", err)
	}
	if localHead != view.HeadRefOID {
		return nil, stateError("PR evaluated head %s does not match local claim head %s; refuse merge cleanup", view.HeadRefOID, localHead)
	}
	claims := []claimArtifact{{issue: primary, branch: view.HeadRefName, sha: view.HeadRefOID}}
	if len(view.ClosingIssuesReferences) > 1 {
		companions, err := a.companionCleanupClaims(root, view, primary)
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

func (a app) companionCleanupClaims(root string, view pullRequestView, primary int) ([]claimArtifact, error) {
	remoteClaims, err := a.listRemoteClaims(root)
	if err != nil {
		return nil, err
	}
	companions := make([]claimArtifact, 0, 1)
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == primary {
			continue
		}
		candidate, err := a.companionCleanupClaim(root, issue.Number, view.HeadRefOID, remoteClaims)
		if err != nil {
			return nil, err
		}
		companions = append(companions, claimArtifact{issue: issue.Number, branch: candidate.branch, sha: candidate.sha})
	}
	return companions, nil
}

func attachClaimWorktrees(layout repositoryLayout, claims []claimArtifact) ([]claimArtifact, error) {
	claims = append([]claimArtifact(nil), claims...)
	for index := range claims {
		worktree, ok, err := findClaimWorktree(layout, claims[index].branch, claims[index].sha)
		if err != nil {
			return nil, err
		}
		if ok {
			claims[index].worktreePath = worktree.path
		}
	}
	return claims, nil
}

func claimWorktreePath(primaryRoot, branch string) string {
	name := strings.ReplaceAll(strings.TrimPrefix(branch, "agent/"), "/", "-")
	return filepath.Join(primaryRoot+"-worktrees", name)
}

func (a app) companionCleanupClaim(root string, issue int, head string, claims []remoteClaim) (remoteClaim, error) {
	candidates := make([]remoteClaim, 0, 1)
	for _, claim := range claims {
		if claim.number != issue {
			continue
		}
		_, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head)
		if err != nil {
			if isGitNonAncestor(err) {
				continue
			}
			return remoteClaim{}, fmt.Errorf("prove companion issue #%d claim %s is included in evaluated head: %w", issue, claim.branch, err)
		}
		candidates = append(candidates, claim)
	}
	if len(candidates) == 0 {
		return remoteClaim{}, stateError("companion issue #%d has no uniquely proven claim in evaluated head %s", issue, head)
	}
	if len(candidates) > 1 {
		return remoteClaim{}, stateError("companion issue #%d has ambiguous claims in evaluated head %s", issue, head)
	}
	return candidates[0], nil
}

func isGitNonAncestor(err error) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == 1
	}
	return strings.Contains(err.Error(), "exit status 1")
}

func rejectDuplicateClaimArtifacts(claims []claimArtifact) error {
	for left := 0; left < len(claims); left++ {
		for right := left + 1; right < len(claims); right++ {
			if claims[left].branch == claims[right].branch {
				return stateError("claim cleanup has duplicate branch %q; preserve ambiguous state", claims[left].branch)
			}
		}
	}
	return nil
}

func findClaimWorktree(layout repositoryLayout, branch, sha string) (gitWorktree, bool, error) {
	expectedPath := claimWorktreePath(layout.primaryRoot, branch)
	var expected gitWorktree
	expectedCount := 0
	var found gitWorktree
	count := 0
	for _, worktree := range layout.worktrees {
		if samePath(worktree.path, expectedPath) {
			expected = worktree
			expectedCount++
		}
		if worktree.branch != "refs/heads/"+branch {
			continue
		}
		found = worktree
		count++
	}
	if expectedCount > 1 {
		return gitWorktree{}, false, stateError("claim worktree path %q is ambiguously registered; preserve claim state", expectedPath)
	}
	if expectedCount == 1 && (expected.branch != "refs/heads/"+branch || expected.head != sha) {
		return gitWorktree{}, false, stateError("claim worktree %q changed branch or head (branch=%q, head=%s); preserve it and run go tool workflowctl pr recover %d", expectedPath, expected.branch, expected.head, issueFromBranchValue(branch))
	}
	if count > 1 {
		return gitWorktree{}, false, stateError("claim branch %q is registered by multiple worktrees; preserve ambiguous state", branch)
	}
	if count == 0 {
		return gitWorktree{}, false, nil
	}
	if !samePath(found.path, expectedPath) {
		return gitWorktree{}, false, stateError("claim worktree %q is external to workflow path %q; preserve it and run go tool workflowctl pr recover %d", found.path, expectedPath, issueFromBranchValue(branch))
	}
	if found.head != sha {
		return gitWorktree{}, false, stateError("claim worktree %q moved from expected head %s to %s; preserve it and run go tool workflowctl pr recover %d", found.path, sha, found.head, issueFromBranchValue(branch))
	}
	return found, true, nil
}

func issueFromBranchValue(branch string) int {
	number, _ := issueFromBranch(branch)
	return number
}

func hasWorktreeForRoot(claims []claimArtifact, root string) bool {
	for _, claim := range claims {
		if claim.worktreePath != "" && samePath(claim.worktreePath, root) {
			return true
		}
	}
	return false
}

func (a app) cleanupClaims(base synchronizedBase, packet mergedPacket) error {
	if packet.number < 1 || packet.mergeSHA == "" {
		return stateError("claim cleanup requires a proven merged packet; preserve claims and run go tool workflowctl pr recover")
	}
	plan := packet.plan
	layout, err := a.repositoryLayout(base.fetched.primary.layout.primaryRoot)
	if err != nil {
		return fmt.Errorf("refresh Git worktree inventory for claim cleanup: %w", err)
	}
	if !samePath(layout.primaryRoot, plan.layout.primaryRoot) {
		return stateError("canonical primary checkout changed during claim cleanup; preserve claims and run go tool workflowctl pr recover")
	}
	worktrees, err := a.claimWorktreesForCleanup(layout, plan)
	if err != nil {
		return err
	}
	if plan.validateArtifacts || len(plan.claims) > 1 {
		if err := a.validateClaimArtifacts(layout.primaryRoot, layout, plan.claims, plan.proofHead, plan.primaryIssue, plan.allowPrimaryMissing); err != nil {
			return err
		}
	}
	if err := a.validateClaimRefs(layout.primaryRoot, plan.claims); err != nil {
		return err
	}
	if err := a.validateClaimWorktrees(layout.primaryRoot, worktrees); err != nil {
		return err
	}
	if err := a.removeClaimWorktrees(layout.primaryRoot, worktrees); err != nil {
		return err
	}
	return a.removeClaimRefs(layout.primaryRoot, plan.claims)
}

func (a app) validateClaimArtifacts(root string, layout repositoryLayout, claims []claimArtifact, head string, primaryIssue int, allowPrimaryMissing bool) error {
	remote, err := a.remoteClaimRefs(root)
	if err != nil {
		return err
	}
	local, err := a.localClaimRefs(root)
	if err != nil {
		return err
	}
	tracking, err := a.localTrackingClaimRefs(root)
	if err != nil {
		return err
	}
	all := append(append(append([]remoteClaim(nil), remote...), local...), tracking...)
	claimBranches, err := claimBranchIndex(claims)
	if err != nil {
		return err
	}
	seenExpected, err := a.validateExpectedClaimArtifacts(root, layout, claims, all, head)
	if err != nil {
		return err
	}
	if err := validateMissingClaimArtifacts(claims, seenExpected, primaryIssue, allowPrimaryMissing); err != nil {
		return err
	}
	if err := validateUnexpectedClaimRefs(all, claimBranches, claims); err != nil {
		return err
	}
	return validateUnexpectedClaimWorktrees(layout, claimBranches, claims)
}

func claimBranchIndex(claims []claimArtifact) (map[string]claimArtifact, error) {
	index := make(map[string]claimArtifact, len(claims))
	for _, claim := range claims {
		if _, exists := index[claim.branch]; exists {
			return nil, stateError("claim cleanup has duplicate branch %q; preserve ambiguous state", claim.branch)
		}
		index[claim.branch] = claim
	}
	return index, nil
}

func (a app) validateExpectedClaimArtifacts(root string, layout repositoryLayout, claims []claimArtifact, all []remoteClaim, head string) (map[string]bool, error) {
	seen := make(map[string]bool, len(claims))
	for _, claim := range claims {
		sha, found, err := exactClaimSHA(all, claim.branch)
		if err != nil {
			return nil, err
		}
		_, worktreeFound, err := findClaimWorktree(layout, claim.branch, claim.sha)
		if err != nil {
			return nil, err
		}
		if found && sha != claim.sha {
			return nil, stateError("claim %s moved from immutable merge-time head %s to %s; preserve claim artifacts and run go tool workflowctl pr recover", claim.branch, claim.sha, sha)
		}
		if found || worktreeFound {
			seen[claim.branch] = true
		}
		if err := a.validateClaimAncestor(root, claim, found || worktreeFound, head); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

func (a app) validateClaimAncestor(root string, claim claimArtifact, present bool, head string) error {
	if !present || head == "" {
		return nil
	}
	_, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head)
	if err == nil {
		return nil
	}
	if isGitNonAncestor(err) {
		return stateError("claim %s at %s is not an ancestor of immutable evaluated head %s; preserve claim artifacts", claim.branch, claim.sha, head)
	}
	return fmt.Errorf("prove claim %s at %s is included in immutable evaluated head: %w", claim.branch, claim.sha, err)
}

func validateMissingClaimArtifacts(claims []claimArtifact, seen map[string]bool, primaryIssue int, allowPrimaryMissing bool) error {
	if len(seen) == 0 {
		return nil
	}
	for _, claim := range claims {
		if !seen[claim.branch] {
			if allowPrimaryMissing && claim.issue == primaryIssue {
				continue
			}
			return stateError("immutable claim artifact %s is missing; preserve remaining claim artifacts and run go tool workflowctl pr recover", claim.branch)
		}
	}
	return nil
}

func validateUnexpectedClaimRefs(all []remoteClaim, expected map[string]claimArtifact, claims []claimArtifact) error {
	for _, artifact := range all {
		claim, ok := expected[artifact.branch]
		if ok {
			if artifact.sha != claim.sha {
				return stateError("claim %s has conflicting artifact heads %s and %s; preserve ambiguous claim state", artifact.branch, claim.sha, artifact.sha)
			}
			continue
		}
		if claimIssue, issueOK := issueFromBranch(artifact.branch); issueOK && hasClaimIssue(claims, claimIssue) {
			return stateError("leftover claim artifact %s at %s has no immutable merge-time proof; preserve claim artifacts", artifact.branch, artifact.sha)
		}
	}
	return nil
}

func validateUnexpectedClaimWorktrees(layout repositoryLayout, expected map[string]claimArtifact, claims []claimArtifact) error {
	for _, worktree := range layout.worktrees {
		branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
		issue, ok := issueFromBranch(branch)
		if !ok || !hasClaimIssue(claims, issue) {
			continue
		}
		if _, expected := expected[branch]; !expected {
			return stateError("leftover claim worktree %q has no immutable merge-time proof; preserve claim artifacts", worktree.path)
		}
	}
	return nil
}

func (a app) localTrackingClaimRefs(root string) ([]remoteClaim, error) {
	output, err := a.command(root, "git", "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/remotes/origin/agent/issue-*")
	if err != nil {
		return nil, fmt.Errorf("list remote-tracking claim refs: %w", err)
	}
	var claims []remoteClaim
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		branch := strings.TrimPrefix(fields[0], "origin/")
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

func exactClaimSHA(claims []remoteClaim, branch string) (string, bool, error) {
	var found string
	for _, claim := range claims {
		if claim.branch != branch {
			continue
		}
		if found == "" {
			found = claim.sha
			continue
		}
		if found != claim.sha {
			return "", false, stateError("claim %s has conflicting artifact heads %s and %s; preserve ambiguous claim state", branch, found, claim.sha)
		}
	}
	return found, found != "", nil
}

func hasClaimIssue(claims []claimArtifact, issue int) bool {
	for _, claim := range claims {
		if claim.issue == issue {
			return true
		}
	}
	return false
}

func (a app) validateClaimRefs(root string, claims []claimArtifact) error {
	for _, claim := range claims {
		if err := a.validateRemoteClaimRef(root, claim); err != nil {
			return err
		}
		if err := a.validateRefExact(root, "refs/remotes/origin/"+claim.branch, claim.sha, "remote-tracking claim "+claim.branch); err != nil {
			return err
		}
		if err := a.validateRefExact(root, "refs/heads/"+claim.branch, claim.sha, "local claim "+claim.branch); err != nil {
			return err
		}
	}
	return nil
}

func (a app) validateRemoteClaimRef(root string, claim claimArtifact) error {
	ref := "refs/heads/" + claim.branch
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", ref)
	if err != nil {
		return fmt.Errorf("inspect remote claim %s before cleanup: %w", claim.branch, err)
	}
	remoteSHA, present, err := exactRemoteRef(output, ref)
	if err != nil {
		return err
	}
	if present && remoteSHA != claim.sha {
		return stateError("preserve remote claim %s: expected %s, found %s; run go tool workflowctl pr recover %d", claim.branch, claim.sha, remoteSHA, claim.issue)
	}
	return nil
}

func (a app) validateClaimWorktrees(root string, worktrees []gitWorktree) error {
	for _, worktree := range worktrees {
		if worktree.locked {
			return stateError("preserve locked claim worktree %q; run go tool workflowctl pr recover after unlocking it", worktree.path)
		}
		status, err := a.command(root, "git", "-C", worktree.path, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
		if err != nil {
			return fmt.Errorf("inspect claim worktree %q before cleanup: %w", worktree.path, err)
		}
		if status != "" {
			return stateError("preserve dirty claim worktree %q; run go tool workflowctl pr recover after cleaning it", worktree.path)
		}
	}
	return nil
}

func (a app) removeClaimWorktrees(root string, worktrees []gitWorktree) error {
	for _, worktree := range worktrees {
		if _, err := a.command(root, "git", "worktree", "remove", "--force", worktree.path); err != nil {
			return fmt.Errorf("remove clean claim worktree %q: %w", worktree.path, err)
		}
	}
	return nil
}

func (a app) removeClaimRefs(root string, claims []claimArtifact) error {
	for _, claim := range claims {
		if err := a.deleteRemoteClaim(root, claim); err != nil {
			return err
		}
	}
	for _, claim := range claims {
		if err := a.deleteLocalClaim(root, claim); err != nil {
			return err
		}
	}
	return nil
}

func (a app) claimWorktreesForCleanup(layout repositoryLayout, plan cleanupPlan) ([]gitWorktree, error) {
	worktrees := make([]gitWorktree, 0, len(plan.claims))
	for _, claim := range plan.claims {
		worktree, found, err := claimWorktreeInLayout(layout, claim)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		worktrees = append(worktrees, worktree)
	}
	sort.Slice(worktrees, func(left, right int) bool {
		leftCurrent := samePath(worktrees[left].path, plan.callerRoot)
		rightCurrent := samePath(worktrees[right].path, plan.callerRoot)
		if leftCurrent != rightCurrent {
			return !leftCurrent
		}
		return worktrees[left].path < worktrees[right].path
	})
	return worktrees, nil
}

func claimWorktreeInLayout(layout repositoryLayout, claim claimArtifact) (gitWorktree, bool, error) {
	if claim.worktreePath == "" {
		return gitWorktree{}, false, nil
	}
	found, ok, err := findClaimWorktree(layout, claim.branch, claim.sha)
	if err != nil {
		return gitWorktree{}, false, err
	}
	if !ok {
		return gitWorktree{}, false, nil
	}
	if !samePath(found.path, claim.worktreePath) {
		return gitWorktree{}, false, stateError("claim worktree %q changed path to %q; preserve claim state and run go tool workflowctl pr recover %d", claim.worktreePath, found.path, claim.issue)
	}
	return found, true, nil
}

func (a app) deleteRemoteClaim(root string, claim claimArtifact) error {
	ref := "refs/heads/" + claim.branch
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", ref)
	if err != nil {
		return fmt.Errorf("inspect remote claim %s before cleanup: %w", claim.branch, err)
	}
	remoteSHA, present, err := exactRemoteRef(output, ref)
	if err != nil {
		return err
	}
	if present && remoteSHA != claim.sha {
		return stateError("preserve remote claim %s: expected %s, found %s; run go tool workflowctl pr recover %d", claim.branch, claim.sha, remoteSHA, claim.issue)
	}
	if present {
		lease := "--force-with-lease=" + ref + ":" + claim.sha
		if _, err := a.command(root, "git", "push", lease, "origin", ":"+ref); err != nil {
			return fmt.Errorf("delete exact remote claim %s: %w", claim.branch, err)
		}
	}
	return a.deleteRefIfExact(root, "refs/remotes/origin/"+claim.branch, claim.sha, "remote-tracking claim "+claim.branch)
}

func exactRemoteRef(output, ref string) (string, bool, error) {
	var found string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || fields[1] != ref {
			return "", false, fmt.Errorf("remote claim listing contains unexpected ref %q", line)
		}
		if found != "" {
			return "", false, fmt.Errorf("remote claim %s is ambiguous", ref)
		}
		found = fields[0]
	}
	return found, found != "", nil
}

func (a app) deleteLocalClaim(root string, claim claimArtifact) error {
	return a.deleteRefIfExact(root, "refs/heads/"+claim.branch, claim.sha, "local claim "+claim.branch)
}

func (a app) deleteRefIfExact(root, ref, expected, description string) error {
	if err := a.validateRefExact(root, ref, expected, description); err != nil {
		return err
	}
	if _, err := a.command(root, "git", "update-ref", "-d", ref, expected); err != nil {
		actual, readErr := a.command(root, "git", "for-each-ref", "--format=%(objectname)", ref)
		if readErr == nil && strings.TrimSpace(actual) == "" {
			return nil
		}
		return fmt.Errorf("delete exact %s: %w", description, err)
	}
	return nil
}

func (a app) validateRefExact(root, ref, expected, description string) error {
	actual, err := a.command(root, "git", "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", description, err)
	}
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return nil
	}
	if actual != expected {
		return stateError("preserve %s: expected %s, found %s; run go tool workflowctl pr recover", description, expected, actual)
	}
	return nil
}
