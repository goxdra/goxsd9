package workflowctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type claimArtifact struct {
	issue        int
	branch       string
	sha          string
	localBranch  string
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

type runLocalRefCandidate struct {
	branch          string
	sha             string
	remotePresent   bool
	trackingPresent bool
	localPresent    bool
}

type provenRunLocalRef struct {
	branch          string
	sha             string
	remotePresent   bool
	trackingPresent bool
	localPresent    bool
	preserved       []runLocalRefCandidate
}

type runLocalSourceRaceError struct {
	source string
	err    error
}

func (e *runLocalSourceRaceError) Error() string {
	return e.err.Error()
}

func (e *runLocalSourceRaceError) Unwrap() error {
	return e.err
}

// runLocalCleanupResult keeps inventory observations separate from the
// evaluated deletion set.  Preserved observations are never mutation targets;
// retaining them here also gives callers a deterministic account of residue
// that belongs to an older run.
type runLocalCleanupResult struct {
	proven    []provenRunLocalRef
	preserved []runLocalRefCandidate
}

type runLocalCandidateFailure struct {
	candidate runLocalRefCandidate
	err       error
}

type runLocalLineage struct {
	identities []runLocalHistoryIdentity
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
	if len(claims) > 1 {
		proofBound = true
	}
	plan := cleanupPlan{
		layout:            layout,
		callerRoot:        root,
		claims:            claims,
		proofHead:         proofHead,
		primaryIssue:      primary,
		validateArtifacts: proofBound,
	}
	if proofBound {
		claims, _, err = a.prepareProofBoundCleanupPlanWithEvidence(root, layout, plan)
		if err != nil {
			return cleanupPlan{}, err
		}
		plan.claims = claims
	}
	if !proofBound {
		claims, err = attachClaimWorktrees(layout, claims)
		if err != nil {
			return cleanupPlan{}, err
		}
		plan.claims = claims
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
	plan.claims = claims
	return plan, nil
}

func (a app) prepareProofBoundCleanupPlanWithEvidence(root string, layout repositoryLayout, plan cleanupPlan) ([]claimArtifact, runLocalCleanupResult, error) {
	evidence, err := a.prepareRunLocalCleanupResult(root, mergedPacket{plan: plan})
	if err != nil {
		return nil, runLocalCleanupResult{}, err
	}
	claims, err := attachClaimWorktrees(layout, plan.claims, evidence.proven)
	if err != nil {
		return nil, runLocalCleanupResult{}, err
	}
	claims, err = attachProvenRunLocalWorktrees(layout, claims, evidence.proven, root)
	if err != nil {
		return nil, runLocalCleanupResult{}, err
	}
	err = a.validateClaimArtifactsWithEvidence(root, layout, claims, plan.proofHead, plan.primaryIssue, false, evidence, true)
	if err != nil {
		return nil, runLocalCleanupResult{}, err
	}
	return claims, evidence, nil
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
	if issue, ok := fixedClaimIssue(view.HeadRefName); !ok || issue != primary {
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

func attachClaimWorktrees(layout repositoryLayout, claims []claimArtifact, provenGroups ...[]provenRunLocalRef) ([]claimArtifact, error) {
	proven, err := selectProvenRunLocalRefs(provenGroups...)
	if err != nil {
		return nil, err
	}
	claims = append([]claimArtifact(nil), claims...)
	for index := range claims {
		claims[index], err = attachClaimWorktree(layout, claims[index], proven)
		if err != nil {
			return nil, err
		}
	}
	return claims, nil
}

func attachClaimWorktree(layout repositoryLayout, claim claimArtifact, proven []provenRunLocalRef) (claimArtifact, error) {
	branch, exactBranch := claimWorktreeLookup(claim, proven)
	worktree, found, err := findClaimWorktreeForBranch(layout, branch, claim.sha, exactBranch)
	if err != nil {
		return claimArtifact{}, err
	}
	if !found {
		return claim, nil
	}
	claim.localBranch = strings.TrimPrefix(worktree.branch, "refs/heads/")
	claim.worktreePath = worktree.path
	return claim, nil
}

func claimWorktreeLookup(claim claimArtifact, proven []provenRunLocalRef) (string, bool) {
	branch := claim.localBranch
	if branch == "" {
		branch = provenRunLocalBranchForIssue(proven, claim.issue, claim.sha)
	}
	if branch == "" {
		return claim.branch, true
	}
	kind, _, _ := classifyAgentRef(branch)
	return branch, kind == agentRefClaim
}

func findClaimWorktreeForBranch(layout repositoryLayout, branch, sha string, exactBranch bool) (gitWorktree, bool, error) {
	if exactBranch {
		return findClaimWorktreeOnExactBranch(layout, branch, sha)
	}
	return findClaimWorktree(layout, branch, sha)
}

func provenRunLocalBranchForIssue(refs []provenRunLocalRef, issue int, sha string) string {
	branch := ""
	for _, ref := range refs {
		if refIssue(ref.branch) != issue || ref.sha != sha {
			continue
		}
		if branch != "" && branch != ref.branch {
			return ""
		}
		branch = ref.branch
	}
	return branch
}

func attachProvenRunLocalWorktrees(layout repositoryLayout, claims []claimArtifact, refs []provenRunLocalRef, callerRoot string) ([]claimArtifact, error) {
	claims = append([]claimArtifact(nil), claims...)
	for _, ref := range refs {
		if !ref.localPresent {
			continue
		}
		worktree, found, err := findClaimWorktree(layout, ref.branch, ref.sha)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if !attachProvenRunLocalWorktree(claims, ref, worktree, callerRoot) {
			return nil, stateError("proven run-local ref %s has no matching immutable claim; preserve claim artifacts", ref.branch)
		}
	}
	return claims, nil
}

func attachProvenRunLocalWorktree(claims []claimArtifact, ref provenRunLocalRef, worktree gitWorktree, callerRoot string) bool {
	issue := refIssue(ref.branch)
	for index := range claims {
		if claims[index].issue != issue {
			continue
		}
		if claims[index].worktreePath == "" || samePath(worktree.path, callerRoot) {
			claims[index].localBranch = strings.TrimPrefix(worktree.branch, "refs/heads/")
			claims[index].worktreePath = worktree.path
		}
		return true
	}
	return false
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
	return findClaimWorktreeWithBranchMatch(layout, branch, sha, false)
}

func findClaimWorktreeOnExactBranch(layout repositoryLayout, branch, sha string) (gitWorktree, bool, error) {
	return findClaimWorktreeWithBranchMatch(layout, branch, sha, true)
}

func findClaimWorktreeWithBranchMatch(layout repositoryLayout, branch, sha string, exactBranch bool) (gitWorktree, bool, error) {
	expectedPath := claimWorktreePath(layout.primaryRoot, branch)
	var legacyMismatch gitWorktree
	legacyMismatchCount := 0
	var found gitWorktree
	count := 0
	for _, worktree := range layout.worktrees {
		candidate, exact, mismatch := claimWorktreeCandidateWithBranchMatch(layout, worktree, branch, sha, exactBranch)
		if mismatch {
			legacyMismatch = worktree
			legacyMismatchCount++
			continue
		}
		if !exact {
			continue
		}
		found = candidate
		count++
	}
	if count > 1 {
		return gitWorktree{}, false, stateError("claim branch %q is registered by multiple worktrees; preserve ambiguous state", branch)
	}
	if count == 0 {
		if legacyMismatchCount > 1 {
			return gitWorktree{}, false, stateError("claim worktree path %q is ambiguously registered; preserve claim state", expectedPath)
		}
		if legacyMismatchCount == 1 {
			return gitWorktree{}, false, stateError("claim worktree %q moved from expected head %s to %s; preserve it and run go tool workflowctl pr recover %d", expectedPath, sha, legacyMismatch.head, issueFromBranchValue(branch))
		}
		return gitWorktree{}, false, nil
	}
	localBranch := strings.TrimPrefix(found.branch, "refs/heads/")
	expected := claimWorktreePath(layout.primaryRoot, localBranch)
	if !samePath(found.path, expected) {
		return gitWorktree{}, false, stateError("claim worktree %q is external to workflow path %q; preserve it and run go tool workflowctl pr recover %d", found.path, expected, issueFromBranchValue(branch))
	}
	return found, true, nil
}

func claimWorktreeCandidateWithBranchMatch(layout repositoryLayout, worktree gitWorktree, branch, sha string, exactBranch bool) (gitWorktree, bool, bool) {
	localBranch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	if exactBranch && localBranch != branch {
		return gitWorktree{}, false, false
	}
	if !exactBranch && !claimLocalBranchMatches(branch, localBranch) {
		return gitWorktree{}, false, false
	}
	if worktree.head == sha {
		return worktree, true, false
	}
	expectedPath := claimWorktreePath(layout.primaryRoot, branch)
	return gitWorktree{}, false, localBranch == branch && samePath(worktree.path, expectedPath)
}

func claimLocalBranchMatches(branch, localBranch string) bool {
	if localBranch == branch {
		return true
	}
	kind, number, _ := classifyAgentRef(branch)
	localKind, localNumber, _ := classifyAgentRef(localBranch)
	return kind == agentRefClaim && localKind == agentRefRunLocal && number == localNumber
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
	runLocalEvidence, err := a.prepareRunLocalCleanupResult(layout.primaryRoot, packet)
	if err != nil {
		return err
	}
	worktrees, err := a.claimWorktreesForCleanup(layout, plan, runLocalEvidence.proven)
	if err != nil {
		return err
	}
	if plan.validateArtifacts || len(plan.claims) > 1 {
		artifactErr := a.validateClaimArtifactsWithEvidence(layout.primaryRoot, layout, plan.claims, plan.proofHead, plan.primaryIssue, plan.allowPrimaryMissing, runLocalEvidence, false)
		if artifactErr != nil {
			return artifactErr
		}
	}
	refErr := a.validateClaimRefs(layout.primaryRoot, plan.claims, runLocalEvidence.proven)
	if refErr != nil {
		return refErr
	}
	if err := a.validateClaimWorktrees(layout.primaryRoot, worktrees, runLocalEvidence.proven); err != nil {
		return err
	}
	if err := a.removeClaimWorktrees(layout.primaryRoot, worktrees, runLocalEvidence.proven); err != nil {
		return err
	}
	if err := a.removeClaimRefs(layout.primaryRoot, plan.claims, runLocalEvidence.proven); err != nil {
		return err
	}
	return a.removeRunLocalRefs(layout.primaryRoot, runLocalEvidence.proven)
}

func (a app) prepareRunLocalCleanupResult(root string, packet mergedPacket) (runLocalCleanupResult, error) {
	candidates, err := a.runLocalCleanupCandidates(root, packet)
	if err != nil {
		return runLocalCleanupResult{}, err
	}
	matched, preserved, err := a.proveRunLocalCleanupCandidates(root, packet, candidates)
	if err != nil {
		return runLocalCleanupResult{}, err
	}
	if len(matched) == 0 {
		if err := rejectAmbiguousRunLocalCandidates(candidates, packet.plan.claims); err != nil {
			return runLocalCleanupResult{}, err
		}
	}
	if err := rejectAmbiguousProvenRunLocalCandidates(matched, packet.plan.claims); err != nil {
		return runLocalCleanupResult{}, err
	}
	return runLocalCleanupResult{proven: matched, preserved: preserved}, nil
}

func (a app) runLocalCleanupCandidates(root string, packet mergedPacket) ([]runLocalRefCandidate, error) {
	inventory, err := a.remoteAgentRefInventory(root)
	if err != nil {
		return nil, err
	}
	local, malformedLocal, err := a.localRunLocalRefs(root)
	if err != nil {
		return nil, err
	}
	tracking, malformedTracking, err := a.localTrackingRunLocalRefs(root)
	if err != nil {
		return nil, err
	}
	inventory.runLocals = filterRunLocalRefsForClaims(inventory.runLocals, packet.plan.claims)
	inventory.malformed = filterMalformedRunLocalRefsForClaims(inventory.malformed, packet.plan.claims)
	local = filterRunLocalRefsForClaims(local, packet.plan.claims)
	malformedLocal = filterMalformedRunLocalRefsForClaims(malformedLocal, packet.plan.claims)
	tracking = filterRunLocalRefsForClaims(tracking, packet.plan.claims)
	malformedTracking = filterMalformedRunLocalRefsForClaims(malformedTracking, packet.plan.claims)
	malformedErr := rejectMalformedRunLocalRefs(inventory.malformed, malformedLocal, malformedTracking)
	if malformedErr != nil {
		return nil, malformedErr
	}
	return mergeRunLocalCandidates(inventory.runLocals, local, tracking), nil
}

func (a app) proveRunLocalCleanupCandidates(root string, packet mergedPacket, candidates []runLocalRefCandidate) ([]provenRunLocalRef, []runLocalRefCandidate, error) {
	matched := make([]provenRunLocalRef, 0, len(candidates))
	preserved := make([]runLocalRefCandidate, 0, len(candidates))
	failed := make([]runLocalCandidateFailure, 0, len(candidates))
	lineages := make(map[int]runLocalLineage)
	for _, candidate := range candidates {
		matchedCandidate, found, preservedCandidate, failure, err := a.proveRunLocalCleanupCandidate(root, packet, candidate, lineages)
		if err != nil {
			return nil, nil, err
		}
		if preservedCandidate {
			preserved = append(preserved, candidate)
			continue
		}
		if !found {
			if failure != nil {
				failed = append(failed, *failure)
			}
			continue
		}
		matched = append(matched, matchedCandidate)
	}
	for _, failure := range failed {
		index := provenRunLocalRefIndex(matched, failure.candidate.branch)
		if index < 0 {
			return nil, nil, failure.err
		}
		matched[index].preserved = append(matched[index].preserved, failure.candidate)
		preserved = append(preserved, failure.candidate)
	}
	return matched, preserved, nil
}

func (a app) proveRunLocalCleanupCandidate(root string, packet mergedPacket, candidate runLocalRefCandidate,
	lineages map[int]runLocalLineage) (provenRunLocalRef, bool, bool, *runLocalCandidateFailure, error) {
	issue := refIssue(candidate.branch)
	lineage, found := lineages[issue]
	if !found {
		var err error
		lineage, err = a.evaluatedRunLocalLineage(root, packet, candidate)
		if err != nil {
			return provenRunLocalRef{}, false, false, nil, err
		}
		lineages[issue] = lineage
	}
	if !lineage.contains(refRunID(candidate.branch)) {
		return provenRunLocalRef{}, false, true, nil, nil
	}
	matched, found, err := a.validateRunLocalCandidate(root, packet, candidate)
	if err == nil {
		return matched, found, false, nil, nil
	}
	if isRunLocalSourceRace(err) {
		return provenRunLocalRef{}, false, false, nil, err
	}
	var stateErr *exitError
	if !errors.As(err, &stateErr) {
		return provenRunLocalRef{}, false, false, nil, err
	}
	return provenRunLocalRef{}, false, false, &runLocalCandidateFailure{candidate: candidate, err: err}, nil
}

func (a app) evaluatedRunLocalLineage(root string, packet mergedPacket, candidate runLocalRefCandidate) (runLocalLineage, error) {
	issue := refIssue(candidate.branch)
	if issue < 1 || packet.plan.proofHead == "" {
		return runLocalLineage{}, stateError("preserve run-local ref %s: immutable evaluated claim proof is unavailable", candidate.branch)
	}
	history, err := a.evaluatedClaimHistory(root, candidate, packet.plan.proofHead)
	if err != nil {
		return runLocalLineage{}, fmt.Errorf("read evaluated claim lineage for run-local ref %s at %s: %w", candidate.branch, packet.plan.proofHead, err)
	}
	records, err := splitRunLocalHistory(history, candidate.branch)
	if err != nil {
		return runLocalLineage{}, err
	}
	if err := a.verifyRunLocalHistoryRecords(root, candidate.branch, records); err != nil {
		return runLocalLineage{}, err
	}
	lineage := runLocalLineage{identities: make([]runLocalHistoryIdentity, 0)}
	for _, record := range records {
		identity, canonical, parseErr := parseCanonicalRunLocalClaim(record.message, issue)
		if parseErr != nil {
			return runLocalLineage{}, terminalRunLocalHistoryError(stateError("preserve run-local ref %s at history commit %s: malformed canonical claim marker: %w", candidate.branch, record.commit, parseErr))
		}
		if !canonical {
			continue
		}
		lineage.identities = appendUniqueRunLocalIdentity(lineage.identities, identity)
	}
	return lineage, nil
}

func (lineage runLocalLineage) contains(runID string) bool {
	for _, identity := range lineage.identities {
		if identity.runID == runID {
			return true
		}
	}
	return false
}

func refRunID(branch string) string {
	_, _, runID := classifyAgentRef(branch)
	return runID
}

func appendUniqueRunLocalIdentity(values []runLocalHistoryIdentity, value runLocalHistoryIdentity) []runLocalHistoryIdentity {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func rejectAmbiguousProvenRunLocalCandidates(refs []provenRunLocalRef, claims []claimArtifact) error {
	candidates := make([]runLocalRefCandidate, 0, len(refs))
	for _, ref := range refs {
		candidates = append(candidates, runLocalRefCandidate{
			branch:          ref.branch,
			sha:             ref.sha,
			remotePresent:   ref.remotePresent,
			trackingPresent: ref.trackingPresent,
			localPresent:    ref.localPresent,
		})
	}
	return rejectAmbiguousRunLocalCandidates(candidates, claims)
}

func provenRunLocalRefIndex(refs []provenRunLocalRef, branch string) int {
	for index, ref := range refs {
		if ref.branch == branch {
			return index
		}
	}
	return -1
}

func filterRunLocalRefsForClaims(refs []runLocalRef, claims []claimArtifact) []runLocalRef {
	filtered := make([]runLocalRef, 0, len(refs))
	for _, ref := range refs {
		if !runLocalIssueInClaims(ref.branch, claims) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func filterMalformedRunLocalRefsForClaims(refs []agentRef, claims []claimArtifact) []agentRef {
	filtered := make([]agentRef, 0, len(refs))
	for _, ref := range refs {
		if !runLocalIssueInClaims(ref.branch, claims) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func runLocalIssueInClaims(branch string, claims []claimArtifact) bool {
	issue, ok := runLocalRefIssue(branch)
	if !ok {
		return false
	}
	for _, claim := range claims {
		if claim.issue == issue {
			return true
		}
	}
	return false
}

func runLocalRefIssue(branch string) (int, bool) {
	kind, number, _ := classifyAgentRef(branch)
	if kind != agentRefRunLocal && kind != agentRefMalformed {
		return 0, false
	}
	if number > 0 {
		return number, true
	}
	const prefix = "agent/issue-"
	value := strings.TrimPrefix(branch, prefix)
	if value == branch || value == "" {
		return 0, false
	}
	digits := value
	if index := strings.IndexByte(value, '-'); index >= 0 {
		digits = value[:index]
	}
	if digits == "" {
		return 0, false
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
}

func (a app) validateRunLocalCandidate(root string, packet mergedPacket, candidate runLocalRefCandidate) (provenRunLocalRef, bool, error) {
	claim, found, err := runLocalClaimForRef(candidate, packet.plan.claims)
	if err != nil {
		return provenRunLocalRef{}, false, err
	}
	if !found {
		return provenRunLocalRef{}, false, nil
	}
	if !packet.plan.validateArtifacts || packet.plan.proofHead == "" {
		return provenRunLocalRef{}, false, stateError("preserve run-local ref %s: immutable evaluated claim proof is unavailable", candidate.branch)
	}
	proofErr := a.validateRunLocalProof(root, candidate, claim, packet.plan.proofHead)
	if proofErr != nil {
		return provenRunLocalRef{}, false, proofErr
	}
	currentErr := a.validateCurrentRunLocalRef(root, candidate)
	if currentErr != nil {
		return provenRunLocalRef{}, false, currentErr
	}
	openPRErr := a.validateNoOpenRunLocalPR(root, candidate.branch)
	if openPRErr != nil {
		return provenRunLocalRef{}, false, openPRErr
	}
	return provenRunLocalRef{
		branch:          candidate.branch,
		sha:             candidate.sha,
		remotePresent:   candidate.remotePresent,
		trackingPresent: candidate.trackingPresent,
		localPresent:    candidate.localPresent,
	}, true, nil
}

func rejectMalformedRunLocalRefs(groups ...[]agentRef) error {
	for _, refs := range groups {
		for _, ref := range refs {
			return stateError("preserve malformed run-local ref %s at %s", ref.branch, ref.sha)
		}
	}
	return nil
}

func mergeRunLocalCandidates(groups ...[]runLocalRef) []runLocalRefCandidate {
	refs := make([]runLocalRef, 0)
	for _, group := range groups {
		refs = append(refs, group...)
	}
	sort.Slice(refs, func(left, right int) bool {
		if refs[left].branch != refs[right].branch {
			return refs[left].branch < refs[right].branch
		}
		if refs[left].sha != refs[right].sha {
			return refs[left].sha < refs[right].sha
		}
		return refs[left].source < refs[right].source
	})
	merged := make([]runLocalRefCandidate, 0, len(refs))
	for _, ref := range refs {
		if len(merged) == 0 || merged[len(merged)-1].branch != ref.branch || merged[len(merged)-1].sha != ref.sha {
			merged = append(merged, runLocalRefCandidate{
				branch:          ref.branch,
				sha:             ref.sha,
				remotePresent:   ref.source == claimRefRemote,
				trackingPresent: ref.source == claimRefTracking,
				localPresent:    ref.source == claimRefLocal,
			})
			continue
		}
		current := &merged[len(merged)-1]
		current.remotePresent = current.remotePresent || ref.source == claimRefRemote
		current.trackingPresent = current.trackingPresent || ref.source == claimRefTracking
		current.localPresent = current.localPresent || ref.source == claimRefLocal
	}
	return merged
}

func runLocalClaimForRef(ref runLocalRefCandidate, claims []claimArtifact) (claimArtifact, bool, error) {
	var match claimArtifact
	matches := 0
	for _, claim := range claims {
		if claim.issue != refIssue(ref.branch) {
			continue
		}
		match = claim
		matches++
	}
	if matches > 1 {
		return claimArtifact{}, false, stateError("preserve run-local ref %s: multiple immutable claims match issue #%d", ref.branch, refIssue(ref.branch))
	}
	if matches == 1 {
		return match, true, nil
	}
	for _, claim := range claims {
		if claim.sha == ref.sha {
			return claimArtifact{}, false, stateError("preserve run-local ref %s: parsed issue #%d differs from immutable claim issue #%d", ref.branch, refIssue(ref.branch), claim.issue)
		}
	}
	return claimArtifact{}, false, nil
}

func refIssue(branch string) int {
	_, number, _ := classifyAgentRef(branch)
	return number
}

func (a app) validateRunLocalProof(root string, ref runLocalRefCandidate, claim claimArtifact, proofHead string) error {
	kind, _, runID := classifyAgentRef(ref.branch)
	if kind != agentRefRunLocal || runID == "" {
		return stateError("preserve run-local ref %s: run identity is malformed", ref.branch)
	}
	history, err := a.runLocalHistoryProof(root, ref, proofHead)
	if err != nil {
		return err
	}
	if err := validateRunLocalHistoryIdentities(ref.branch, runID, claim.issue, history.identities); err != nil {
		return err
	}
	if ref.sha == claim.sha {
		return nil
	}
	return a.validateRunLocalAncestor(root, ref, history, claim, proofHead)
}

func (a app) validateRunLocalAncestor(root string, ref runLocalRefCandidate, history runLocalHistoryProof, claim claimArtifact, proofHead string) error {
	if ref.sha == proofHead {
		return stateError("preserve run-local ref %s: expected immutable claim head %s or a strict ancestor, found evaluated head %s", ref.branch, claim.sha, ref.sha)
	}
	if _, err := a.command(root, "git", "merge-base", "--is-ancestor", ref.sha, proofHead); err != nil {
		if isGitNonAncestor(err) {
			return stateError("preserve run-local ref %s: expected immutable claim head %s or a proven ancestor, found %s which is not an ancestor of immutable evaluated head %s", ref.branch, claim.sha, ref.sha, proofHead)
		}
		return fmt.Errorf("preserve run-local ref %s: expected immutable claim head %s or a proven ancestor; prove %s is an ancestor of immutable evaluated head %s: %w", ref.branch, claim.sha, ref.sha, proofHead, err)
	}
	if history.anchor == "" {
		return stateError("preserve run-local ref %s: canonical claim anchor is unavailable", ref.branch)
	}
	if history.anchor == ref.sha {
		return a.validateRunLocalAnchorCandidate(root, ref, history.anchor)
	}
	return a.validateRunLocalAfterAnchor(root, ref, history.anchor)
}

func (a app) validateRunLocalAnchorCandidate(root string, ref runLocalRefCandidate, anchor string) error {
	parent, err := a.command(root, "git", "rev-parse", anchor+"^")
	if err != nil {
		return fmt.Errorf("read parent of canonical claim anchor %s for run-local ref %s: %w", anchor, ref.branch, err)
	}
	parent = strings.TrimSpace(parent)
	if parent == "" || parent == ref.sha {
		return stateError("preserve run-local ref %s: candidate %s does not follow canonical claim anchor", ref.branch, ref.sha)
	}
	_, err = a.command(root, "git", "merge-base", "--is-ancestor", parent, ref.sha)
	if err != nil {
		if isGitNonAncestor(err) {
			return stateError("preserve run-local ref %s at %s: candidate does not follow canonical claim anchor %s", ref.branch, ref.sha, anchor)
		}
		return fmt.Errorf("prove run-local ref %s at %s follows canonical claim anchor %s: %w", ref.branch, ref.sha, anchor, err)
	}
	return nil
}

func (a app) validateRunLocalAfterAnchor(root string, ref runLocalRefCandidate, anchor string) error {
	_, err := a.command(root, "git", "merge-base", "--is-ancestor", anchor, ref.sha)
	if err == nil {
		return nil
	}
	if isGitNonAncestor(err) {
		return stateError("preserve run-local ref %s at %s: candidate is before canonical claim anchor %s", ref.branch, ref.sha, anchor)
	}
	return fmt.Errorf("prove run-local ref %s at %s follows canonical claim anchor %s: %w", ref.branch, ref.sha, anchor, err)
}

func validateRunLocalHistoryIdentities(branch, runID string, issue int, identities []runLocalHistoryIdentity) error {
	issueRunIDs := make([]string, 0, len(identities))
	runIssues := make([]int, 0, len(identities))
	for _, identity := range identities {
		if identity.issue == issue {
			issueRunIDs = appendUniqueString(issueRunIDs, identity.runID)
		}
		if identity.runID == runID {
			runIssues = appendUniqueInt(runIssues, identity.issue)
		}
	}
	if len(issueRunIDs) > 1 {
		return stateError("preserve run-local ref %s: evaluated head history has conflicting runs for issue #%d", branch, issue)
	}
	if len(runIssues) > 1 {
		return stateError("preserve run-local ref %s: evaluated head history has conflicting issues for run %s", branch, runID)
	}
	if len(issueRunIDs) == 1 && issueRunIDs[0] != runID {
		return stateError("preserve run-local ref %s: run identity %q differs from evaluated-head claim run %q", branch, runID, issueRunIDs[0])
	}
	if len(runIssues) == 1 && runIssues[0] != issue {
		return stateError("preserve run-local ref %s: claim issue #%d differs from evaluated-head metadata issue #%d", branch, issue, runIssues[0])
	}
	if len(issueRunIDs) == 0 || len(runIssues) == 0 {
		return stateError("preserve run-local ref %s: evaluated head history has no matching Agent-Run-ID and Agent-Issue", branch)
	}
	return nil
}

type runLocalHistoryIdentity struct {
	runID string
	issue int
}

type runLocalHistoryProof struct {
	identities []runLocalHistoryIdentity
	anchor     string
}

func (a app) runLocalHistoryProof(root string, ref runLocalRefCandidate, proofHead string) (runLocalHistoryProof, error) {
	history, err := a.evaluatedClaimHistory(root, ref, proofHead)
	if err != nil {
		return runLocalHistoryProof{}, err
	}
	records, err := splitRunLocalHistory(history, ref.branch)
	if err != nil {
		return runLocalHistoryProof{}, err
	}
	if err := a.verifyRunLocalHistoryRecords(root, ref.branch, records); err != nil {
		return runLocalHistoryProof{}, err
	}
	return parseBoundedRunLocalHistoryProofRecords(records, ref.branch)
}

func (a app) evaluatedClaimHistory(root string, ref runLocalRefCandidate, proofHead string) (string, error) {
	format := "%H%x00%B%x00"
	history, err := a.command(root, "git", "log", "--format="+format, proofHead)
	if err == nil {
		return history, nil
	}
	if operationDispositionOf(err) != operationDispositionUnknown {
		return "", err
	}
	fetchRef := "refs/workflowctl/run-local-proof/" + ref.branch
	existing, checkErr := a.command(root, "git", "for-each-ref", "--format=%(objectname)", fetchRef)
	if checkErr != nil {
		return "", retryableOperation("read evaluated claim history", fmt.Errorf("inspect temporary evaluated claim ref %s after history command: %w", fetchRef, errors.Join(err, checkErr)))
	}
	if strings.TrimSpace(existing) != "" {
		return "", stateError("preserve run-local ref %s: temporary evaluated claim ref %s already exists", ref.branch, fetchRef)
	}
	fetchSpec := "refs/heads/" + ref.branch + ":" + fetchRef
	_, fetchErr := a.command(root, "git", "fetch", "--no-tags", "--no-write-fetch-head", "origin", fetchSpec)
	if fetchErr != nil {
		cleanupErr := a.deleteTemporaryRunLocalProofRef(root, fetchRef)
		return "", retryableOperation("read evaluated claim history", fmt.Errorf("read evaluated claim history for run-local ref %s at %s: %w", ref.branch, proofHead, errors.Join(err, fmt.Errorf("fetch run-local claim: %w", errors.Join(fetchErr, cleanupErr)))))
	}
	fetchedSHA, fetchRefErr := a.command(root, "git", "rev-parse", fetchRef)
	if fetchRefErr != nil {
		cleanupErr := a.deleteTemporaryRunLocalProofRef(root, fetchRef)
		return "", retryableOperation("read evaluated claim history", fmt.Errorf("read temporary evaluated claim ref %s: %w", fetchRef, errors.Join(fetchRefErr, cleanupErr)))
	}
	fetchedSHA = strings.TrimSpace(fetchedSHA)
	if !validExactCommitSHA(fetchedSHA) {
		cleanupErr := a.deleteTemporaryRunLocalProofRef(root, fetchRef)
		return "", terminalRunLocalHistoryError(stateError("temporary evaluated claim ref %s returned malformed head %q; preserve claim artifacts: %v", fetchRef, fetchedSHA, cleanupErr))
	}
	history, historyErr := a.command(root, "git", "log", "--format="+format, proofHead)
	cleanupErr := a.deleteRefIfExact(root, fetchRef, fetchedSHA, "temporary evaluated claim ref "+ref.branch)
	if historyErr != nil {
		return "", retryableOperation("read evaluated claim history", fmt.Errorf("read evaluated claim history for run-local ref %s at %s after fetch: %w", ref.branch, proofHead, errors.Join(historyErr, cleanupErr)))
	}
	if cleanupErr != nil {
		return "", retryableOperation("read evaluated claim history cleanup", cleanupErr)
	}
	return history, nil
}

func (a app) deleteTemporaryRunLocalProofRef(root, ref string) error {
	output, err := a.command(root, "git", "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return fmt.Errorf("inspect temporary evaluated claim ref %s for cleanup: %w", ref, err)
	}
	sha := strings.TrimSpace(output)
	if sha == "" {
		return nil
	}
	return a.deleteRefIfExact(root, ref, sha, "temporary evaluated claim ref")
}

type runLocalHistoryRecord struct {
	commit  string
	message string
}

func terminalRunLocalHistoryError(err error) error {
	if err == nil || operationDispositionOf(err) != operationDispositionUnknown {
		return err
	}
	return terminalOperation("run-local history", err)
}

func splitRunLocalHistory(history, branch string) ([]runLocalHistoryRecord, error) {
	fields := strings.Split(history, "\x00")
	if len(fields) == 1 && strings.TrimSpace(fields[0]) == "" {
		return nil, terminalRunLocalHistoryError(stateError("preserve run-local ref %s: evaluated head history is empty", branch))
	}
	records := make([]runLocalHistoryRecord, 0, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		// git log places its record-separator newline before each later hash.
		commit := strings.TrimPrefix(fields[index], "\n")
		if !validExactCommitSHA(commit) {
			return nil, terminalRunLocalHistoryError(stateError("preserve run-local ref %s: evaluated head history contains malformed commit token %q; preserve claim artifacts", branch, commit))
		}
		records = append(records, runLocalHistoryRecord{commit: commit, message: fields[index+1]})
	}
	if len(fields)%2 != 1 || strings.TrimSpace(fields[len(fields)-1]) != "" {
		return nil, terminalRunLocalHistoryError(stateError("preserve run-local ref %s: evaluated head history is malformed", branch))
	}
	return records, nil
}

func (a app) verifyRunLocalHistoryRecords(root, branch string, records []runLocalHistoryRecord) error {
	for _, record := range records {
		if err := a.validateLocalAgentCommit(root, record.commit, "run-local history commit "+record.commit+" for "+branch); err != nil {
			return err
		}
		if !isCanonicalClaimMarkerShape(record.message) {
			continue
		}
		marker, err := a.readCanonicalClaimIdentity(root, record.commit, "")
		if err != nil {
			return retryableOperationIfRecoverable("verify run-local history marker", fmt.Errorf("verify canonical claim marker %s for %s: %w", record.commit, branch, err))
		}
		if marker.message != record.message {
			return terminalOperation("verify run-local history marker", stateError("preserve run-local ref %s: history commit %s message differs from its canonical Git object", branch, record.commit))
		}
	}
	return nil
}

func isCanonicalClaimMarkerShape(message string) bool {
	lines := strings.Split(message, "\n")
	return len(lines) > 0 && strings.HasPrefix(lines[0], "chore(workflow): claim issue #")
}

func parseRunLocalHistoryRecords(records []runLocalHistoryRecord, branch string) ([]runLocalHistoryIdentity, error) {
	identities := make([]runLocalHistoryIdentity, 0, len(records))
	for _, record := range records {
		identity, found, err := parseRunLocalHistoryRecord(record.message, branch, record.commit)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

func parseBoundedRunLocalHistory(history, branch string) ([]runLocalHistoryIdentity, error) {
	proof, err := parseBoundedRunLocalHistoryProof(history, branch)
	if err != nil {
		return nil, err
	}
	return proof.identities, nil
}

func parseBoundedRunLocalHistoryProof(history, branch string) (runLocalHistoryProof, error) {
	kind, _, runID := classifyAgentRef(branch)
	if kind != agentRefRunLocal || runID == "" {
		return runLocalHistoryProof{}, terminalRunLocalHistoryError(stateError("preserve run-local ref %s: run identity is malformed", branch))
	}
	records, err := splitRunLocalHistory(history, branch)
	if err != nil {
		return runLocalHistoryProof{}, err
	}
	return parseBoundedRunLocalHistoryProofRecords(records, branch)
}

func parseBoundedRunLocalHistoryProofRecords(records []runLocalHistoryRecord, branch string) (runLocalHistoryProof, error) {
	kind, issue, runID := classifyAgentRef(branch)
	if kind != agentRefRunLocal || runID == "" {
		return runLocalHistoryProof{}, terminalRunLocalHistoryError(stateError("preserve run-local ref %s: run identity is malformed", branch))
	}
	anchor, err := findRunLocalHistoryAnchor(records, branch, runID, issue)
	if err != nil {
		return runLocalHistoryProof{}, terminalRunLocalHistoryError(err)
	}
	identities, err := parseRunLocalHistoryRecords(records[:anchor+1], branch)
	if err != nil {
		return runLocalHistoryProof{}, terminalRunLocalHistoryError(err)
	}
	return runLocalHistoryProof{identities: identities, anchor: records[anchor].commit}, nil
}

func findRunLocalHistoryAnchor(records []runLocalHistoryRecord, branch, runID string, issue int) (int, error) {
	anchor := -1
	malformedIndex := -1
	var malformedErr error
	for index, record := range records {
		identity, canonical, parseErr := parseCanonicalRunLocalClaim(record.message, issue)
		if parseErr != nil {
			malformedIndex, malformedErr = earliestMalformedRunLocalAnchor(malformedIndex, malformedErr, index, record, branch, parseErr)
			continue
		}
		if canonical && identity.runID == runID && identity.issue == issue {
			anchor = index
		}
	}
	if anchor < 0 {
		if malformedErr != nil {
			return 0, malformedErr
		}
		return 0, stateError("preserve run-local ref %s: evaluated head history has no canonical claim marker for run %s and issue #%d", branch, runID, issue)
	}
	if malformedIndex >= 0 && malformedIndex <= anchor {
		return 0, malformedErr
	}
	return anchor, nil
}

func earliestMalformedRunLocalAnchor(currentIndex int, currentErr error, index int, record runLocalHistoryRecord, branch string, parseErr error) (int, error) {
	if currentIndex >= 0 && currentIndex <= index {
		return currentIndex, currentErr
	}
	return index, stateError("preserve run-local ref %s at history commit %s: malformed canonical claim marker: %w", branch, record.commit, parseErr)
}

func parseCanonicalRunLocalClaim(message string, issue int) (runLocalHistoryIdentity, bool, error) {
	lines := strings.Split(message, "\n")
	expectedSubject := fmt.Sprintf("chore(workflow): claim issue #%d", issue)
	if len(lines) == 0 || lines[0] != expectedSubject {
		return runLocalHistoryIdentity{}, false, nil
	}
	if len(lines) != 7 || lines[1] != "" || lines[2] != "Agent-Persona: Smith" {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker does not have the generated message shape")
	}
	runID, ok := exactHistoryTrailerLine(lines[3], "Agent-Run-ID")
	if !ok || !validRunID(runID) {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker has an invalid Agent-Run-ID trailer")
	}
	lease, ok := exactHistoryTrailerLine(lines[4], "Agent-Lease-Until")
	if !ok {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker has an invalid Agent-Lease-Until trailer")
	}
	parsedLease, err := time.Parse(time.RFC3339, lease)
	if err != nil || parsedLease.Format(time.RFC3339) != lease {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker has an invalid Agent-Lease-Until trailer")
	}
	issueValue, ok := exactHistoryTrailerLine(lines[5], "Agent-Issue")
	if !ok {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker has an invalid Agent-Issue trailer")
	}
	parsedIssue, err := canonicalHistoryIssue(issueValue)
	if err != nil || parsedIssue != issue {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker has an invalid Agent-Issue trailer")
	}
	if lines[6] != "" {
		return runLocalHistoryIdentity{}, true, errors.New("claim marker does not have the generated message shape")
	}
	return runLocalHistoryIdentity{runID: runID, issue: parsedIssue}, true, nil
}

func exactHistoryTrailerLine(line, name string) (string, bool) {
	prefix := name + ": "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(line, prefix)
	if value == "" || strings.TrimSpace(value) != value {
		return "", false
	}
	return value, true
}

func parseRunLocalHistoryRecord(message, branch, commit string) (runLocalHistoryIdentity, bool, error) {
	runIDs, err := historyTrailerValues(message, "Agent-Run-ID")
	if err != nil {
		return runLocalHistoryIdentity{}, false, stateError("preserve run-local ref %s at history commit %s: %w", branch, commit, err)
	}
	issues, err := historyTrailerValues(message, "Agent-Issue")
	if err != nil {
		return runLocalHistoryIdentity{}, false, stateError("preserve run-local ref %s at history commit %s: %w", branch, commit, err)
	}
	if len(runIDs) == 0 && len(issues) == 0 {
		return runLocalHistoryIdentity{}, false, nil
	}
	if len(runIDs) != 1 || len(issues) != 1 {
		return runLocalHistoryIdentity{}, false, stateError("preserve run-local ref %s at history commit %s: Agent-Run-ID and Agent-Issue are ambiguous", branch, commit)
	}
	issue, err := canonicalHistoryIssue(issues[0])
	if err != nil {
		return runLocalHistoryIdentity{}, false, stateError("preserve run-local ref %s at history commit %s: %w", branch, commit, err)
	}
	return runLocalHistoryIdentity{runID: runIDs[0], issue: issue}, true, nil
}

func historyTrailerValues(message, name string) ([]string, error) {
	prefix := name + ":"
	values := make([]string, 0, 1)
	for _, line := range strings.Split(message, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if value == "" {
			return nil, fmt.Errorf("empty %s trailer", name)
		}
		values = append(values, value)
	}
	return values, nil
}

func canonicalHistoryIssue(value string) (int, error) {
	issue, err := strconv.Atoi(value)
	if err != nil || issue < 1 || strconv.Itoa(issue) != value {
		return 0, fmt.Errorf("invalid Agent-Issue trailer %q", value)
	}
	return issue, nil
}

func appendUniqueString(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueInt(values []int, value int) []int {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func (a app) validateCurrentRunLocalRef(root string, ref runLocalRefCandidate) error {
	if ref.remotePresent {
		if err := a.validateCurrentRemoteRunLocalRef(root, ref); err != nil {
			return err
		}
	}
	if ref.trackingPresent {
		if err := a.validateCurrentObservedRunLocalRef(root, ref, "tracking", "refs/remotes/origin/"+ref.branch, "remote-tracking run-local ref "+ref.branch); err != nil {
			return err
		}
	}
	if ref.localPresent {
		if err := a.validateCurrentObservedRunLocalRef(root, ref, "local", "refs/heads/"+ref.branch, "local run-local ref "+ref.branch); err != nil {
			return err
		}
	}
	return nil
}

func (a app) validateCurrentRemoteRunLocalRef(root string, ref runLocalRefCandidate) error {
	remoteRef := "refs/heads/" + ref.branch
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", remoteRef)
	if err != nil {
		return retryableOperation("inspect run-local ref "+ref.branch+" before cleanup", fmt.Errorf("inspect run-local ref %s before cleanup: %w", ref.branch, err))
	}
	remoteSHA, present, err := exactRemoteRef(output, remoteRef)
	if err != nil {
		return err
	}
	if !present {
		return runLocalSourceRace("remote", stateError("preserve run-local ref %s: remote ref disappeared before cleanup; run go tool workflowctl pr recover", ref.branch))
	}
	if remoteSHA != ref.sha {
		return runLocalSourceRace("remote", stateError("preserve run-local ref %s: expected %s, found %s; run go tool workflowctl pr recover", ref.branch, ref.sha, remoteSHA))
	}
	return nil
}

func (a app) validateCurrentObservedRunLocalRef(root string, ref runLocalRefCandidate, source, name, description string) error {
	if err := a.validateObservedRefExact(root, name, ref.sha, description); err != nil {
		var stateErr *exitError
		if errors.As(err, &stateErr) {
			return runLocalSourceRace(source, err)
		}
		return err
	}
	return nil
}

func runLocalSourceRace(source string, err error) error {
	if err == nil {
		return nil
	}
	return &runLocalSourceRaceError{source: source, err: err}
}

func isRunLocalSourceRace(err error) bool {
	var race *runLocalSourceRaceError
	return errors.As(err, &race)
}

func (a app) validateObservedRefExact(root, ref, expected, description string) error {
	actual, err := a.command(root, "git", "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", description, err)
	}
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return stateError("preserve %s: expected %s, found missing; run go tool workflowctl pr recover", description, expected)
	}
	if actual != expected {
		return stateError("preserve %s: expected %s, found %s; run go tool workflowctl pr recover", description, expected, actual)
	}
	return nil
}

func (a app) validateNoOpenRunLocalPR(root, branch string) error {
	output, err := a.command(root, "gh", "pr", "list", "--repo", repositoryKey, "--head", branch,
		"--state", "open", "--json", "number")
	if err != nil {
		return fmt.Errorf("check open PRs for run-local ref %s: %w", branch, err)
	}
	var pullRequests []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(output), &pullRequests); err != nil {
		return fmt.Errorf("decode open PRs for run-local ref %s: %w", branch, err)
	}
	if len(pullRequests) != 0 {
		return stateError("preserve run-local ref %s: an open PR uses this ref", branch)
	}
	return nil
}

func rejectAmbiguousRunLocalCandidates(refs []runLocalRefCandidate, claims []claimArtifact) error {
	for left := 0; left < len(refs); left++ {
		leftIssue := refIssue(refs[left].branch)
		for right := left + 1; right < len(refs); right++ {
			if refs[left].branch == refs[right].branch ||
				leftIssue != refIssue(refs[right].branch) || refs[left].sha != refs[right].sha {
				continue
			}
			for _, claim := range claims {
				if claim.issue == leftIssue {
					return stateError("preserve ambiguous run-local refs %s and %s at candidate %s", refs[left].branch, refs[right].branch, refs[left].sha)
				}
			}
		}
	}
	return nil
}

func (a app) validateClaimArtifacts(root string, layout repositoryLayout, claims []claimArtifact, head string, primaryIssue int, allowPrimaryMissing bool, validated ...[]provenRunLocalRef) error {
	proven, err := selectProvenRunLocalRefs(validated...)
	if err != nil {
		return err
	}
	return a.validateClaimArtifactsWithEvidence(root, layout, claims, head, primaryIssue,
		allowPrimaryMissing, runLocalCleanupResult{proven: proven}, len(validated) == 0)
}

func (a app) validateClaimArtifactsWithEvidence(root string, layout repositoryLayout, claims []claimArtifact, head string,
	primaryIssue int, allowPrimaryMissing bool, evidence runLocalCleanupResult, preMerge bool) error {
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
	all = append(all, provenRunLocalArtifacts(evidence.proven)...)
	all = append(all, preservedRunLocalArtifacts(evidence)...)
	claimBranches, err := claimBranchIndex(claims)
	if err != nil {
		return err
	}
	if inferErr := inferClaimLocalBranchesWithEvidence(claims, all, evidence.proven, evidence.preserved); inferErr != nil {
		return inferErr
	}
	seenExpected, err := a.validateExpectedClaimArtifactsWithEvidence(root, layout, claims, all, head, evidence)
	if err != nil {
		return err
	}
	if err := validateMissingClaimArtifacts(claims, seenExpected, primaryIssue, allowPrimaryMissing); err != nil {
		return err
	}
	if err := validateUnexpectedClaimRefsWithEvidence(all, claimBranches, claims, evidence.proven, evidence.preserved); err != nil {
		return err
	}
	return validateUnexpectedClaimWorktreesWithEvidence(layout, claimBranches, claims, evidence.proven, evidence.preserved, preMerge)
}

func provenRunLocalArtifacts(refs []provenRunLocalRef) []remoteClaim {
	artifacts := make([]remoteClaim, 0, len(refs)*3)
	for _, ref := range refs {
		if ref.remotePresent {
			artifacts = append(artifacts, remoteClaim{branch: ref.branch, sha: ref.sha, source: claimRefRemote})
		}
		if ref.localPresent {
			artifacts = append(artifacts, remoteClaim{branch: ref.branch, sha: ref.sha, source: claimRefLocal})
		}
		if ref.trackingPresent {
			artifacts = append(artifacts, remoteClaim{branch: ref.branch, sha: ref.sha, source: claimRefTracking})
		}
	}
	sort.Slice(artifacts, func(left, right int) bool {
		if artifacts[left].branch != artifacts[right].branch {
			return artifacts[left].branch < artifacts[right].branch
		}
		if artifacts[left].sha != artifacts[right].sha {
			return artifacts[left].sha < artifacts[right].sha
		}
		return artifacts[left].source < artifacts[right].source
	})
	return artifacts
}

func preservedRunLocalArtifacts(evidence runLocalCleanupResult) []remoteClaim {
	candidates := make([]runLocalRefCandidate, 0, len(evidence.preserved))
	for _, candidate := range evidence.preserved {
		candidates = appendUniqueRunLocalCandidate(candidates, candidate)
	}
	for _, ref := range evidence.proven {
		for _, candidate := range ref.preserved {
			candidates = appendUniqueRunLocalCandidate(candidates, candidate)
		}
	}
	artifacts := make([]remoteClaim, 0, len(candidates)*3)
	for _, candidate := range candidates {
		if candidate.remotePresent {
			artifacts = append(artifacts, remoteClaim{branch: candidate.branch, sha: candidate.sha, source: claimRefRemote})
		}
		if candidate.localPresent {
			artifacts = append(artifacts, remoteClaim{branch: candidate.branch, sha: candidate.sha, source: claimRefLocal})
		}
		if candidate.trackingPresent {
			artifacts = append(artifacts, remoteClaim{branch: candidate.branch, sha: candidate.sha, source: claimRefTracking})
		}
	}
	sort.Slice(artifacts, func(left, right int) bool {
		if artifacts[left].branch != artifacts[right].branch {
			return artifacts[left].branch < artifacts[right].branch
		}
		if artifacts[left].sha != artifacts[right].sha {
			return artifacts[left].sha < artifacts[right].sha
		}
		return artifacts[left].source < artifacts[right].source
	})
	return artifacts
}

func appendUniqueRunLocalCandidate(candidates []runLocalRefCandidate, candidate runLocalRefCandidate) []runLocalRefCandidate {
	for _, current := range candidates {
		if current == candidate {
			return candidates
		}
	}
	return append(candidates, candidate)
}

func claimBranchIndex(claims []claimArtifact) (map[string]claimArtifact, error) {
	index := make(map[string]claimArtifact, len(claims)*2)
	for _, claim := range claims {
		if _, exists := index[claim.branch]; exists {
			return nil, stateError("claim cleanup has duplicate branch %q; preserve ambiguous state", claim.branch)
		}
		index[claim.branch] = claim
		if claim.localBranch == "" || claim.localBranch == claim.branch {
			continue
		}
		if _, exists := index[claim.localBranch]; exists {
			return nil, stateError("claim cleanup has duplicate local branch %q; preserve ambiguous state", claim.localBranch)
		}
		index[claim.localBranch] = claim
	}
	return index, nil
}

func selectProvenRunLocalRefs(groups ...[]provenRunLocalRef) ([]provenRunLocalRef, error) {
	if len(groups) > 1 {
		return nil, stateError("claim cleanup received multiple proven run-local target sets; preserve claim artifacts")
	}
	if len(groups) == 1 {
		return groups[0], nil
	}
	return nil, nil
}

func (a app) validateExpectedClaimArtifactsWithEvidence(root string, layout repositoryLayout, claims []claimArtifact,
	all []remoteClaim, head string, evidence runLocalCleanupResult) (map[string]bool, error) {
	seen := make(map[string]bool, len(claims))
	for _, claim := range claims {
		present, err := a.validateExpectedClaimArtifact(root, layout, claim, all, head, evidence.proven)
		if err != nil {
			return nil, err
		}
		if present {
			seen[claim.branch] = true
		}
	}
	return seen, nil
}

func (a app) validateExpectedClaimArtifact(root string, layout repositoryLayout, claim claimArtifact, all []remoteClaim, head string, provenGroups ...[]provenRunLocalRef) (bool, error) {
	proven, err := selectProvenRunLocalRefs(provenGroups...)
	if err != nil {
		return false, err
	}
	found, err := validateExpectedClaimRemoteSources(all, claim)
	if err != nil {
		return false, err
	}
	localFound, err := validateExpectedLocalClaim(all, claim, proven)
	if err != nil {
		return false, err
	}
	worktreeFound, err := validateExpectedClaimWorktree(layout, claim, proven, len(provenGroups) == 0)
	if err != nil {
		return false, err
	}
	present := found || localFound || worktreeFound
	if err := a.validateClaimAncestor(root, claim, present, head); err != nil {
		return false, err
	}
	return present, nil
}

func validateExpectedClaimRemoteSources(all []remoteClaim, claim claimArtifact) (bool, error) {
	sha, found, err := exactClaimSHAFromSources(all, claim.branch, claimRefRemote, claimRefTracking)
	if err != nil {
		return false, err
	}
	if found && sha != claim.sha {
		return false, stateError("claim %s moved from immutable merge-time head %s to %s; preserve claim artifacts and run go tool workflowctl pr recover", claim.branch, claim.sha, sha)
	}
	return found, nil
}

func validateExpectedClaimWorktree(layout repositoryLayout, claim claimArtifact, proven []provenRunLocalRef, allowUnproven bool) (bool, error) {
	worktreeBranch := claim.localBranch
	if worktreeBranch == "" {
		worktreeBranch = claim.branch
	}
	localKind, _, _ := classifyAgentRef(worktreeBranch)
	if claim.localBranch == "" || localKind == agentRefClaim {
		_, found, err := findClaimWorktreeOnExactBranch(layout, worktreeBranch, claim.sha)
		return found, err
	}
	if !allowUnproven && !provenRunLocalRefAt(proven, worktreeBranch, claim.sha) {
		return false, nil
	}
	_, found, err := findClaimWorktree(layout, worktreeBranch, claim.sha)
	return found, err
}

func validateExpectedLocalClaim(all []remoteClaim, claim claimArtifact, provenGroups ...[]provenRunLocalRef) (bool, error) {
	proven, err := selectProvenRunLocalRefs(provenGroups...)
	if err != nil {
		return false, err
	}
	if claim.localBranch == "" {
		return false, nil
	}
	localSHA, found, err := exactClaimSHAFromSources(all, claim.localBranch, claimRefLocal)
	if err != nil {
		return false, err
	}
	if found && localSHA != claim.sha {
		if provenRunLocalRefAt(proven, claim.localBranch, localSHA) {
			return true, nil
		}
		return false, stateError("local claim %s moved from immutable merge-time head %s to %s; preserve claim artifacts and run go tool workflowctl pr recover", claim.localBranch, claim.sha, localSHA)
	}
	return found, nil
}

//nolint:gocognit // Branch inference must reject duplicates while preserving deterministic artifact order.
func inferClaimLocalBranchesWithEvidence(claims []claimArtifact, all []remoteClaim, proven []provenRunLocalRef, preserved []runLocalRefCandidate) error {
	for index := range claims {
		if claims[index].localBranch != "" {
			continue
		}
		var candidate string
		for _, artifact := range all {
			kind, _, _ := classifyAgentRef(artifact.branch)
			if artifact.source != claimRefLocal || kind != agentRefRunLocal ||
				!claimLocalBranchMatches(claims[index].branch, artifact.branch) {
				continue
			}
			if preservedRunLocalObservation(artifact, proven) {
				continue
			}
			if preservedRunLocalCandidate(artifact, preserved) {
				continue
			}
			if candidate != "" && candidate != artifact.branch {
				return stateError("claim %s has ambiguous proven local branches %q and %q; preserve claim artifacts", claims[index].branch, candidate, artifact.branch)
			}
			candidate = artifact.branch
		}
		if candidate != "" {
			claims[index].localBranch = candidate
		}
	}
	return nil
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

func validateUnexpectedClaimRefs(all []remoteClaim, expected map[string]claimArtifact, claims []claimArtifact, validated ...[]provenRunLocalRef) error {
	proven, err := selectProvenRunLocalRefs(validated...)
	if err != nil {
		return err
	}
	return validateUnexpectedClaimRefsWithEvidence(all, expected, claims, proven, nil)
}

func validateUnexpectedClaimRefsWithEvidence(all []remoteClaim, expected map[string]claimArtifact, claims []claimArtifact,
	proven []provenRunLocalRef, preserved []runLocalRefCandidate) error {
	for _, artifact := range all {
		if err := validateUnexpectedClaimRefWithEvidence(artifact, expected, claims, proven, preserved); err != nil {
			return err
		}
	}
	return nil
}

func validateUnexpectedClaimRefWithEvidence(artifact remoteClaim, expected map[string]claimArtifact, claims []claimArtifact,
	proven []provenRunLocalRef, preserved []runLocalRefCandidate) error {
	kind, issue, _ := classifyAgentRef(artifact.branch)
	if kind == agentRefRunLocal {
		return validateUnexpectedRunLocalRefWithEvidence(artifact, issue, claims, proven, preserved)
	}
	return validateUnexpectedClaimArtifact(artifact, expected, claims)
}

func validateUnexpectedRunLocalRefWithEvidence(artifact remoteClaim, issue int, claims []claimArtifact, proven []provenRunLocalRef,
	preserved []runLocalRefCandidate) error {
	if provenRunLocalArtifactAllowed(artifact, proven) {
		return nil
	}
	if preservedRunLocalArtifact(artifact.branch, proven) && preservedRunLocalObservation(artifact, proven) {
		return nil
	}
	if preservedRunLocalCandidate(artifact, preserved) {
		return nil
	}
	if hasClaimIssue(claims, issue) {
		return stateError("run-local claim artifact %s at %s has no immutable evaluated-head proof; preserve claim artifacts", artifact.branch, artifact.sha)
	}
	return nil
}

func validateUnexpectedClaimArtifact(artifact remoteClaim, expected map[string]claimArtifact, claims []claimArtifact) error {
	claim, ok := expected[artifact.branch]
	if ok {
		if artifact.source == claimRefLocal && artifact.branch == claim.branch && claim.localBranch != artifact.branch {
			return nil
		}
		if artifact.sha != claim.sha {
			return stateError("claim %s has conflicting artifact heads %s and %s; preserve ambiguous claim state", artifact.branch, claim.sha, artifact.sha)
		}
		return nil
	}
	if claimIssue, issueOK := issueFromBranch(artifact.branch); issueOK && hasClaimIssue(claims, claimIssue) {
		return stateError("leftover claim artifact %s at %s has no immutable merge-time proof; preserve claim artifacts", artifact.branch, artifact.sha)
	}
	return nil
}

func validateUnexpectedClaimWorktrees(layout repositoryLayout, expected map[string]claimArtifact, claims []claimArtifact, validated ...[]provenRunLocalRef) error {
	proven, err := selectProvenRunLocalRefs(validated...)
	if err != nil {
		return err
	}
	return validateUnexpectedClaimWorktreesWithEvidence(layout, expected, claims, proven, nil, len(validated) == 0)
}

func validateUnexpectedClaimWorktreesWithEvidence(layout repositoryLayout, expected map[string]claimArtifact, claims []claimArtifact,
	proven []provenRunLocalRef, preserved []runLocalRefCandidate, preMerge bool) error {
	for _, worktree := range layout.worktrees {
		if err := validateUnexpectedClaimWorktreeWithEvidence(worktree, expected, claims, proven, preserved, preMerge); err != nil {
			return err
		}
	}
	return nil
}

func validateUnexpectedClaimWorktreeWithEvidence(worktree gitWorktree, expected map[string]claimArtifact, claims []claimArtifact,
	proven []provenRunLocalRef, preserved []runLocalRefCandidate, preMerge bool) error {
	branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	kind, issue, _ := classifyAgentRef(branch)
	if kind == agentRefRunLocal {
		return validateRunLocalClaimWorktree(worktree, issue, expected, claims, proven, preserved, preMerge)
	}
	return validateFixedClaimWorktree(worktree, branch, expected, claims)
}

func validateRunLocalClaimWorktree(worktree gitWorktree, issue int, expected map[string]claimArtifact,
	claims []claimArtifact, proven []provenRunLocalRef, preserved []runLocalRefCandidate, preMerge bool) error {
	if preMerge && expectedClaimWorktreeAtExactHead(worktree, expected) {
		return nil
	}
	if provenRunLocalWorktreeAllowed(worktree, proven) {
		return nil
	}
	if preservedRunLocalArtifact(strings.TrimPrefix(worktree.branch, "refs/heads/"), proven) &&
		preservedRunLocalWorktreeArtifact(worktree, proven) {
		return nil
	}
	if preservedRunLocalWorktreeCandidate(worktree, preserved) {
		return nil
	}
	if !hasClaimIssue(claims, issue) {
		return nil
	}
	return stateError("leftover run-local claim worktree %q has no immutable evaluated-head proof; preserve claim artifacts", worktree.path)
}

func validateFixedClaimWorktree(worktree gitWorktree, branch string, expected map[string]claimArtifact, claims []claimArtifact) error {
	issue, ok := issueFromBranch(branch)
	if !ok || !hasClaimIssue(claims, issue) {
		return nil
	}
	if _, expected := expected[branch]; !expected {
		return stateError("leftover claim worktree %q has no immutable merge-time proof; preserve claim artifacts", worktree.path)
	}
	return nil
}

func expectedClaimWorktreeAtExactHead(worktree gitWorktree, expected map[string]claimArtifact) bool {
	branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	claim, found := expected[branch]
	return found && claim.localBranch == branch && claim.sha == worktree.head
}

func provenRunLocalArtifactAllowed(artifact remoteClaim, refs []provenRunLocalRef) bool {
	for _, ref := range refs {
		if ref.branch != artifact.branch || ref.sha != artifact.sha {
			continue
		}
		switch artifact.source {
		case claimRefRemote:
			return ref.remotePresent
		case claimRefLocal:
			return ref.localPresent
		case claimRefTracking:
			return ref.trackingPresent
		default:
			return false
		}
	}
	return false
}

// preservedRunLocalArtifact reports a same-issue run-local observation that
// belongs to a different run than an already proven current target.  It is
// intentionally independent of source (remote, tracking, or local): every
// source is only an observation of the same preserved branch and is never a
// deletion authority by itself.
func preservedRunLocalArtifact(branch string, refs []provenRunLocalRef) bool {
	kind, issue, runID := classifyAgentRef(branch)
	if kind != agentRefRunLocal || len(refs) == 0 {
		return false
	}
	sameIssue := false
	for _, ref := range refs {
		refKind, refIssueNumber, refRunID := classifyAgentRef(ref.branch)
		if refKind != agentRefRunLocal || refIssueNumber != issue {
			continue
		}
		sameIssue = true
		if ref.branch == branch || refRunID == runID {
			return false
		}
	}
	return sameIssue
}

func preservedRunLocalObservation(artifact remoteClaim, refs []provenRunLocalRef) bool {
	for _, ref := range refs {
		for _, preserved := range ref.preserved {
			if preserved.branch != artifact.branch || preserved.sha != artifact.sha {
				continue
			}
			switch artifact.source {
			case claimRefRemote:
				return preserved.remotePresent
			case claimRefLocal:
				return preserved.localPresent
			case claimRefTracking:
				return preserved.trackingPresent
			default:
				return false
			}
		}
	}
	return false
}

func preservedRunLocalCandidate(artifact remoteClaim, candidates []runLocalRefCandidate) bool {
	for _, candidate := range candidates {
		if candidate.branch != artifact.branch || candidate.sha != artifact.sha {
			continue
		}
		switch artifact.source {
		case claimRefRemote:
			return candidate.remotePresent
		case claimRefLocal:
			return candidate.localPresent
		case claimRefTracking:
			return candidate.trackingPresent
		default:
			return false
		}
	}
	return false
}

func preservedRunLocalWorktreeArtifact(worktree gitWorktree, refs []provenRunLocalRef) bool {
	branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	for _, ref := range refs {
		for _, preserved := range ref.preserved {
			if preserved.localPresent && preserved.branch == branch && preserved.sha == worktree.head {
				return true
			}
		}
	}
	return false
}

func preservedRunLocalWorktreeCandidate(worktree gitWorktree, candidates []runLocalRefCandidate) bool {
	branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	for _, candidate := range candidates {
		if candidate.localPresent && candidate.branch == branch && candidate.sha == worktree.head {
			return true
		}
	}
	return false
}

func provenRunLocalWorktreeAllowed(worktree gitWorktree, refs []provenRunLocalRef) bool {
	branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	for _, ref := range refs {
		if ref.localPresent && ref.branch == branch && ref.sha == worktree.head {
			return true
		}
	}
	return false
}

func (a app) localTrackingClaimRefs(root string) ([]remoteClaim, error) {
	output, err := a.command(root, "git", "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/remotes/origin/agent/issue-*")
	if err != nil {
		return nil, retryableOperation("list remote-tracking claim refs", fmt.Errorf("list remote-tracking claim refs: %w", err))
	}
	var claims []remoteClaim
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, terminalOperation("list remote-tracking claim refs", fmt.Errorf("remote-tracking claim ref listing contains malformed entry %q", line))
		}
		branch, namespaceErr := trackingAgentRefBranch(fields[0])
		if namespaceErr != nil {
			return nil, terminalOperation("list remote-tracking claim refs", namespaceErr)
		}
		kind, number, _ := classifyAgentRef(branch)
		if kind != agentRefClaim {
			continue
		}
		claims = append(claims, remoteClaim{branch: branch, number: number, sha: fields[1], source: claimRefTracking})
	}
	sort.Slice(claims, func(left, right int) bool {
		if claims[left].branch != claims[right].branch {
			return claims[left].branch < claims[right].branch
		}
		return claims[left].sha < claims[right].sha
	})
	return claims, nil
}

func (a app) localRunLocalRefs(root string) ([]runLocalRef, []agentRef, error) {
	return a.listRunLocalRefs(root, "refs/heads/agent/issue-*", claimRefLocal)
}

func (a app) localTrackingRunLocalRefs(root string) ([]runLocalRef, []agentRef, error) {
	return a.listRunLocalRefs(root, "refs/remotes/origin/agent/issue-*", claimRefTracking)
}

func (a app) listRunLocalRefs(root, namespace string, source claimRefSource) ([]runLocalRef, []agentRef, error) {
	label := sourceName(source)
	output, err := a.command(root, "git", "for-each-ref", "--format=%(refname:short) %(objectname)", namespace)
	if err != nil {
		return nil, nil, retryableOperation("list "+label+" run-local refs", fmt.Errorf("list %s run-local refs: %w", label, err))
	}
	var refs []runLocalRef
	var malformed []agentRef
	for _, line := range strings.Split(output, "\n") {
		ref, badRef, present, parseErr := parseRunLocalRefLine(line, source)
		if parseErr != nil {
			return nil, nil, terminalOperation("list "+label+" run-local refs", parseErr)
		}
		if !present {
			continue
		}
		if badRef.branch != "" {
			malformed = append(malformed, badRef)
			continue
		}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(left, right int) bool {
		if refs[left].branch != refs[right].branch {
			return refs[left].branch < refs[right].branch
		}
		return refs[left].sha < refs[right].sha
	})
	sort.Slice(malformed, func(left, right int) bool {
		if malformed[left].branch != malformed[right].branch {
			return malformed[left].branch < malformed[right].branch
		}
		return malformed[left].sha < malformed[right].sha
	})
	return refs, malformed, nil
}

func parseRunLocalRefLine(line string, source claimRefSource) (runLocalRef, agentRef, bool, error) {
	if strings.TrimSpace(line) == "" {
		return runLocalRef{}, agentRef{}, false, nil
	}
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return runLocalRef{}, agentRef{}, false, fmt.Errorf("%s run-local ref listing contains malformed entry %q", sourceName(source), line)
	}
	branch, err := agentRefBranchForSource(fields[0], source)
	if err != nil {
		return runLocalRef{}, agentRef{}, false, err
	}
	kind, number, runID := classifyAgentRef(branch)
	if kind == agentRefRunLocal {
		return runLocalRef{branch: branch, number: number, runID: runID, sha: fields[1], source: source}, agentRef{}, true, nil
	}
	if kind == agentRefMalformed {
		return runLocalRef{}, agentRef{branch: branch, sha: fields[1]}, true, nil
	}
	return runLocalRef{}, agentRef{}, false, nil
}

func exactClaimSHAFromSources(claims []remoteClaim, branch string, sources ...claimRefSource) (string, bool, error) {
	var found string
	for _, claim := range claims {
		if claim.branch != branch || !claimRefSourceAllowed(claim.source, sources) {
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

func claimRefSourceAllowed(source claimRefSource, allowed []claimRefSource) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if source == candidate {
			return true
		}
	}
	return false
}

func hasClaimIssue(claims []claimArtifact, issue int) bool {
	for _, claim := range claims {
		if claim.issue == issue {
			return true
		}
	}
	return false
}

func (a app) validateClaimRefs(root string, claims []claimArtifact, provenGroups ...[]provenRunLocalRef) error {
	proven, err := selectProvenRunLocalRefs(provenGroups...)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if err := a.validateRemoteClaimRef(root, claim); err != nil {
			return err
		}
		if err := a.validateRefExact(root, "refs/remotes/origin/"+claim.branch, claim.sha, "remote-tracking claim "+claim.branch); err != nil {
			return err
		}
		localBranch := claim.localBranch
		if localBranch == "" {
			localBranch = claim.branch
		}
		if provenRunLocalRefAtBranch(proven, localBranch) {
			continue
		}
		if err := a.validateRefExact(root, "refs/heads/"+localBranch, claim.sha, "local claim "+localBranch); err != nil {
			return err
		}
	}
	return nil
}

func (a app) validateRemoteClaimRef(root string, claim claimArtifact) error {
	ref := "refs/heads/" + claim.branch
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", ref)
	if err != nil {
		return retryableOperation("inspect remote claim "+claim.branch+" before cleanup", fmt.Errorf("inspect remote claim %s before cleanup: %w", claim.branch, err))
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

func (a app) validateClaimWorktrees(root string, worktrees []gitWorktree, provenRunLocalRefs []provenRunLocalRef) error {
	for _, worktree := range worktrees {
		if err := validateProvenRunLocalWorktree(worktree, provenRunLocalRefs); err != nil {
			return err
		}
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

func (a app) removeClaimWorktrees(root string, worktrees []gitWorktree, provenRunLocalRefs []provenRunLocalRef) error {
	for _, worktree := range worktrees {
		if err := validateProvenRunLocalWorktree(worktree, provenRunLocalRefs); err != nil {
			return err
		}
		if _, err := a.command(root, "git", "worktree", "remove", "--force", worktree.path); err != nil {
			return fmt.Errorf("remove clean claim worktree %q: %w", worktree.path, err)
		}
	}
	return nil
}

func validateProvenRunLocalWorktree(worktree gitWorktree, refs []provenRunLocalRef) error {
	branch := strings.TrimPrefix(worktree.branch, "refs/heads/")
	kind, _, _ := classifyAgentRef(branch)
	if kind != agentRefRunLocal {
		return nil
	}
	if provenRunLocalWorktreeAllowed(worktree, refs) {
		return nil
	}
	return stateError("preserve run-local claim worktree %q: immutable run identity proof is unavailable", worktree.path)
}

func (a app) removeClaimRefs(root string, claims []claimArtifact, provenRunLocalRefs []provenRunLocalRef) error {
	for _, claim := range claims {
		if err := a.deleteRemoteClaim(root, claim); err != nil {
			return err
		}
	}
	for _, claim := range claims {
		if err := a.removeClaimLocalRef(root, claim, provenRunLocalRefs); err != nil {
			return err
		}
	}
	return nil
}

func (a app) removeClaimLocalRef(root string, claim claimArtifact, provenRunLocalRefs []provenRunLocalRef) error {
	branch := claim.localBranch
	if branch == "" {
		branch = claim.branch
	}
	kind, _, _ := classifyAgentRef(branch)
	if kind != agentRefRunLocal {
		return a.deleteLocalClaim(root, claim)
	}
	if provenRunLocalRefAtBranch(provenRunLocalRefs, branch) {
		return nil
	}
	output, err := a.command(root, "git", "for-each-ref", "--format=%(objectname)", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("inspect local run-local ref %s before cleanup: %w", branch, err)
	}
	if strings.TrimSpace(output) != "" {
		return stateError("preserve local run-local ref %s: immutable run identity proof is unavailable", branch)
	}
	return nil
}

func (a app) removeRunLocalRefs(root string, refs []provenRunLocalRef) error {
	for _, ref := range refs {
		if err := a.validateCurrentProvenRunLocalRef(root, ref); err != nil {
			return err
		}
	}
	for _, ref := range refs {
		if err := a.removeRunLocalRef(root, ref); err != nil {
			return err
		}
	}
	return nil
}

func (a app) removeRunLocalRef(root string, ref provenRunLocalRef) error {
	if ref.remotePresent {
		remoteRef := "refs/heads/" + ref.branch
		output, err := a.command(root, "git", "ls-remote", "--heads", "origin", remoteRef)
		if err != nil {
			return retryableOperation("inspect run-local ref "+ref.branch+" before deletion", fmt.Errorf("inspect run-local ref %s before deletion: %w", ref.branch, err))
		}
		remoteSHA, present, err := exactRemoteRef(output, remoteRef)
		if err != nil {
			return err
		}
		if !present {
			return runLocalSourceRace("remote", stateError("preserve run-local ref %s: remote ref disappeared before deletion; run go tool workflowctl pr recover", ref.branch))
		}
		if remoteSHA != ref.sha {
			return runLocalSourceRace("remote", stateError("preserve run-local ref %s: expected %s, found %s; run go tool workflowctl pr recover", ref.branch, ref.sha, remoteSHA))
		}
		lease := "--force-with-lease=" + remoteRef + ":" + ref.sha
		_, pushErr := a.command(root, "git", "push", lease, "origin", ":"+remoteRef)
		if pushErr != nil {
			return fmt.Errorf("delete exact run-local ref %s: %w", ref.branch, pushErr)
		}
	}
	if ref.trackingPresent {
		if err := a.deleteRefIfExact(root, "refs/remotes/origin/"+ref.branch, ref.sha, "remote-tracking run-local ref "+ref.branch); err != nil {
			return err
		}
	}
	if ref.localPresent {
		return a.deleteRefIfExact(root, "refs/heads/"+ref.branch, ref.sha, "local run-local ref "+ref.branch)
	}
	return nil
}

func (a app) validateCurrentProvenRunLocalRef(root string, ref provenRunLocalRef) error {
	return a.validateCurrentRunLocalRef(root, runLocalRefCandidate{
		branch:          ref.branch,
		sha:             ref.sha,
		remotePresent:   ref.remotePresent,
		trackingPresent: ref.trackingPresent,
		localPresent:    ref.localPresent,
	})
}

func provenRunLocalRefAtBranch(refs []provenRunLocalRef, branch string) bool {
	for _, ref := range refs {
		if ref.branch == branch {
			return true
		}
	}
	return false
}

func provenRunLocalRefAt(refs []provenRunLocalRef, branch, sha string) bool {
	for _, ref := range refs {
		if ref.branch == branch && ref.sha == sha {
			return true
		}
	}
	return false
}

func (a app) claimWorktreesForCleanup(layout repositoryLayout, plan cleanupPlan, provenGroups ...[]provenRunLocalRef) ([]gitWorktree, error) {
	provenRunLocalRefs, err := selectProvenRunLocalRefs(provenGroups...)
	if err != nil {
		return nil, err
	}
	worktrees := make([]gitWorktree, 0, len(plan.claims)+len(provenRunLocalRefs))
	for _, claim := range plan.claims {
		worktree, found, err := claimWorktreeInLayout(layout, claim)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		worktrees = appendUniqueWorktree(worktrees, worktree)
	}
	for _, ref := range provenRunLocalRefs {
		if !ref.localPresent {
			continue
		}
		worktree, found, err := findClaimWorktree(layout, ref.branch, ref.sha)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		worktrees = appendUniqueWorktree(worktrees, worktree)
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

func appendUniqueWorktree(worktrees []gitWorktree, candidate gitWorktree) []gitWorktree {
	for _, worktree := range worktrees {
		if samePath(worktree.path, candidate.path) {
			return worktrees
		}
	}
	return append(worktrees, candidate)
}

func claimWorktreeInLayout(layout repositoryLayout, claim claimArtifact) (gitWorktree, bool, error) {
	if claim.worktreePath == "" {
		return gitWorktree{}, false, nil
	}
	branch := claim.localBranch
	if branch == "" {
		branch = claim.branch
	}
	if kind, _, _ := classifyAgentRef(branch); kind == agentRefRunLocal {
		return gitWorktree{}, false, nil
	}
	kind, _, _ := classifyAgentRef(branch)
	var found gitWorktree
	var ok bool
	var err error
	if kind == agentRefClaim {
		found, ok, err = findClaimWorktreeOnExactBranch(layout, branch, claim.sha)
	}
	if kind != agentRefClaim {
		found, ok, err = findClaimWorktree(layout, branch, claim.sha)
	}
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
		return retryableOperation("inspect remote claim "+claim.branch+" before cleanup", fmt.Errorf("inspect remote claim %s before cleanup: %w", claim.branch, err))
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
	if _, err := remoteAgentRefBranch(ref); err != nil {
		return "", false, terminalOperation("parse remote claim listing", err)
	}
	var found string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return "", false, terminalOperation("parse remote claim listing", fmt.Errorf("remote claim listing contains malformed entry %q", line))
		}
		if _, err := remoteAgentRefBranch(fields[1]); err != nil {
			return "", false, terminalOperation("parse remote claim listing", err)
		}
		if fields[1] != ref {
			return "", false, terminalOperation("parse remote claim listing", fmt.Errorf("remote claim listing contains unexpected ref %q", line))
		}
		if found != "" {
			return "", false, terminalOperation("parse remote claim listing", fmt.Errorf("remote claim %s is ambiguous", ref))
		}
		found = fields[0]
	}
	return found, found != "", nil
}

func (a app) deleteLocalClaim(root string, claim claimArtifact) error {
	branch := claim.localBranch
	if branch == "" {
		branch = claim.branch
	}
	return a.deleteRefIfExact(root, "refs/heads/"+branch, claim.sha, "local claim "+branch)
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
