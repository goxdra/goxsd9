package workflowctl

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const resumeRecoveryTemplate = "Run `go tool workflowctl pr resume %d --expected-head %s --acknowledge-needs-human` again"

type resumeProof struct {
	root            string
	localBranch     string
	issue           int
	pr              int
	expectedHead    string
	observedHead    string
	renewalHead     string
	runID           string
	runLocalHead    string
	runLocalPresent bool
	localAncestor   bool
	already         bool
	pending         bool
	needsHuman      bool
}

type resumeRunLocalObservation struct {
	branch  string
	sha     string
	present bool
}

type resumeRunLocalExpectation struct {
	sha     string
	present bool
	set     bool
}

func (a app) resumePullRequestCommand(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("pr resume: %v", err)
	}
	flags := flag.NewFlagSet("pr resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expected-head", "", "expected PR head SHA")
	acknowledged := flags.Bool("acknowledge-needs-human", false, "acknowledge needs-human recovery")
	dryRun := flags.Bool("dry-run", false, "print the proof without mutation")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("pr resume: %v", parseErr)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*expected) == "" {
		return usageError("usage: workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run]")
	}
	if !*acknowledged {
		return stateError("PR #%d stale recovery requires --acknowledge-needs-human", pr)
	}
	if !validExactCommitSHA(strings.TrimSpace(*expected)) {
		return usageError("pr resume: --expected-head must be a full 40-character commit SHA")
	}
	proof, err := a.preparePullRequestResume(pr, strings.TrimSpace(*expected))
	if err != nil {
		return err
	}
	if err := writeLine(a.stdout, "resume proof: PR #%d issue #%d branch %s run %s expected %s observed %s", proof.pr,
		proof.issue, claimBranch(proof.issue), proof.runID, proof.expectedHead, proof.observedHead); err != nil {
		return fmt.Errorf("write PR resume proof: %w", err)
	}
	if *dryRun {
		return writeLine(a.stdout, "dry-run: preflight complete; no mutation performed")
	}
	return a.applyPullRequestResume(proof)
}

func (a app) preparePullRequestResume(pr int, expectedHead string) (resumeProof, error) {
	proof, err := a.readPullRequestResumeProof(pr, expectedHead)
	if err != nil {
		return resumeProof{}, retryableOperationIfRecoverable("PR resume proof", err)
	}
	return proof, nil
}

//nolint:gocognit,funlen // This proof keeps every fail-closed binding visible before mutation.
func (a app) readPullRequestResumeProof(pr int, expectedHead string) (resumeProof, error) {
	root, localBranch, issue, err := a.currentClaim()
	if err != nil {
		return resumeProof{}, err
	}
	branch := claimBranch(issue)
	if localBranch != branch && !strings.HasPrefix(localBranch, branch+"-run-") {
		return resumeProof{}, stateError("local branch %q is not the fixed issue #%d claim run", localBranch, issue)
	}
	view, err := a.readPullRequestForResume(root, pr)
	if err != nil {
		return resumeProof{}, err
	}
	if view.State != "OPEN" || view.Merged {
		return resumeProof{}, stateError("PR #%d is not an open unmerged PR", pr)
	}
	if view.BaseRefName != "main" {
		return resumeProof{}, stateError("PR #%d targets base %q, not main", pr, view.BaseRefName)
	}
	if view.HeadRefName != branch {
		return resumeProof{}, stateError("PR #%d uses branch %q, not fixed claim branch %q", pr, view.HeadRefName, branch)
	}
	if len(view.ClosingIssuesReferences) > 2 {
		return resumeProof{}, stateError("PR #%d closes %d issues; a work packet permits one primary and one companion",
			pr, len(view.ClosingIssuesReferences))
	}
	if !pullRequestCloses(view, issue) {
		return resumeProof{}, stateError("PR #%d does not close primary issue #%d", pr, issue)
	}
	status, err := a.readIssueStatus(root, issue)
	if err != nil {
		return resumeProof{}, err
	}
	if status.State != "OPEN" {
		return resumeProof{}, stateError("issue #%d must be open before stale PR recovery", issue)
	}
	remote, err := a.remoteClaimHead(root, branch)
	if err != nil {
		return resumeProof{}, err
	}
	local, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return resumeProof{}, fmt.Errorf("read local claim head: %w", err)
	}
	if validateErr := a.validateLocalAgentCommit(root, local, "local claim head"); validateErr != nil {
		return resumeProof{}, validateErr
	}
	stagedErr := a.validateResumeStagedWorktree(root)
	if stagedErr != nil {
		return resumeProof{}, stagedErr
	}
	claim, err := a.readResumeExpectedClaim(root, expectedHead, issue)
	if err != nil {
		return resumeProof{}, retryableOperationIfRecoverable("PR resume expected claim proof", fmt.Errorf("expected head %s has no valid claim ancestry: %w", expectedHead, err))
	}
	if claim.issue != issue {
		return resumeProof{}, stateError("expected head %s claims issue #%d, not issue #%d", expectedHead, claim.issue, issue)
	}
	lease, runID := claim.lease, claim.runID
	if validateErr := validateClaimLocalBranch(localBranch, issue, runID); validateErr != nil {
		return resumeProof{}, validateErr
	}
	if lease.After(time.Now().UTC()) {
		return resumeProof{}, stateError("claim #%d is active until %s; use claim renew", issue, lease.Format(time.RFC3339))
	}
	lineage, err := a.resumeRunLocalLineage(root, expectedHead, issue)
	if err != nil {
		return resumeProof{}, err
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return resumeProof{}, err
	}
	localAncestor, err := a.inspectResumeLocalAncestor(root, local, expectedHead, localBranch)
	if err != nil {
		return resumeProof{}, err
	}
	runLocal, err := a.inspectResumeClaimConflicts(root, issue, branch, remote, localBranch, runID,
		resumeRunLocalExpectation{}, lineage)
	if err != nil {
		return resumeProof{}, err
	}
	already := remote != expectedHead
	pending := local != remote
	if already {
		if pending {
			return resumeProof{}, stateError("resumed remote head %s differs from local head %s", remote, local)
		}
		if err := a.validateExistingResumeCommit(root, remote, expectedHead, issue, runID); err != nil {
			return resumeProof{}, err
		}
		if view.HeadRefOID != remote {
			return resumeProof{}, stateError("PR #%d head %s does not match renewed remote head %s", pr, view.HeadRefOID, remote)
		}
	}
	if pending {
		if remote != expectedHead || view.HeadRefOID != expectedHead {
			return resumeProof{}, stateError("local-only renewal requires PR and remote at expected head %s; PR=%s remote=%s", expectedHead, view.HeadRefOID, remote)
		}
		if !localAncestor {
			if err := a.validateExistingResumeCommit(root, local, expectedHead, issue, runID); err != nil {
				return resumeProof{}, fmt.Errorf("local head differs from the observed remote claim and is not a retryable renewal: %w", err)
			}
		}
	}
	if !already && !pending && (view.HeadRefOID != expectedHead || local != expectedHead) {
		return resumeProof{}, stateError("resume heads moved: expected=%s PR=%s remote=%s local=%s", expectedHead, view.HeadRefOID, remote, local)
	}
	if !already && !issueNeedsHuman(status) {
		return resumeProof{}, stateError("issue #%d must be labeled needs-human before stale PR recovery", issue)
	}
	protectedHeads := resumeProtectedHeads(expectedHead, remote)
	if pending && !localAncestor {
		protectedHeads = resumeProtectedHeads(expectedHead, remote, local)
	}
	if worktreeErr := validateResumeWorktreeHeads(layout, root, localBranch, issue, local, protectedHeads, lineage); worktreeErr != nil {
		return resumeProof{}, worktreeErr
	}
	proof := resumeProof{root: root, localBranch: localBranch, issue: issue, pr: pr, expectedHead: expectedHead,
		observedHead: remote, renewalHead: local, runID: runID, runLocalHead: runLocal.sha, runLocalPresent: runLocal.present,
		localAncestor: localAncestor, already: already, pending: pending,
		needsHuman: issueNeedsHuman(status)}
	if err := a.sealPullRequestResumeProof(proof); err != nil {
		return resumeProof{}, err
	}
	return proof, nil
}

// readResumeExpectedClaim validates the complete expected PR-head object,
// then binds recovery to the nearest canonical claim marker in its ancestry.
// A PR head may contain source changes or merge parents; only the marker that
// establishes claim ownership is required to have the empty, single-parent
// shape.
//
//nolint:gocognit // The expected-head proof keeps object, ancestry, and claim binding ordered.
func (a app) readResumeExpectedClaim(root, expectedHead string, issue int) (canonicalClaimCommit, error) {
	if !validExactCommitSHA(expectedHead) {
		return canonicalClaimCommit{}, stateError("expected PR head %q is not a full commit SHA; preserve claim artifacts", expectedHead)
	}
	if err := a.validateLocalAgentCommit(root, expectedHead, "expected PR head "+expectedHead); err != nil {
		return canonicalClaimCommit{}, err
	}
	object, err := a.gitRaw(root, "cat-file", "commit", expectedHead)
	if err != nil {
		return canonicalClaimCommit{}, retryableOperation("read expected PR head", fmt.Errorf("read expected PR head object at %s: %w", expectedHead, err))
	}
	parsed, err := parseCommitObject(object)
	if err != nil {
		return canonicalClaimCommit{}, terminalOperation("validate expected PR head", stateError("expected PR head %s has malformed commit object; preserve claim artifacts: %w", expectedHead, err))
	}
	for _, parent := range parsed.parents {
		if parentErr := a.validateLocalAgentCommit(root, parent, "expected PR head parent "+parent); parentErr != nil {
			return canonicalClaimCommit{}, parentErr
		}
	}

	history, err := a.command(root, "git", "log", "--format=%H%x00%B%x00", expectedHead)
	if err != nil {
		return canonicalClaimCommit{}, retryableOperation("read PR resume claim ancestry", fmt.Errorf("read claim ancestry at %s: %w", expectedHead, err))
	}
	branch := fmt.Sprintf("agent/issue-%d", issue)
	records, err := splitRunLocalHistory(history, branch)
	if err != nil {
		return canonicalClaimCommit{}, err
	}
	if err := a.verifyRunLocalHistoryRecords(root, branch, records); err != nil {
		return canonicalClaimCommit{}, err
	}
	for _, record := range records {
		identity, canonical, parseErr := parseCanonicalRunLocalClaim(record.message, issue)
		if parseErr != nil {
			return canonicalClaimCommit{}, terminalRunLocalHistoryError(stateError("preserve resume proof: history commit %s has malformed canonical claim marker: %w", record.commit, parseErr))
		}
		if !canonical || identity.issue != issue {
			continue
		}
		observedIssue, observedRunID, lease, parseErr := parseCanonicalClaimMessage(record.message)
		if parseErr != nil {
			return canonicalClaimCommit{}, terminalRunLocalHistoryError(stateError("preserve resume proof: history commit %s has malformed canonical claim marker: %w", record.commit, parseErr))
		}
		if observedIssue != issue {
			return canonicalClaimCommit{}, stateError("expected PR head %s ancestry marker %s claims issue #%d, not issue #%d; preserve claim artifacts", expectedHead, record.commit, observedIssue, issue)
		}
		return canonicalClaimCommit{message: record.message, issue: observedIssue, runID: observedRunID, lease: lease}, nil
	}
	return canonicalClaimCommit{}, stateError("expected PR head %s ancestry has no canonical claim marker for issue #%d; preserve claim artifacts", expectedHead, issue)
}

//nolint:gocognit // Ref inventory and evaluated-lineage filtering are one fail-closed proof boundary.
func (a app) inspectResumeClaimConflicts(root string, issue int, fixedBranch, fixedHead, currentBranch, currentRunID string,
	expectation resumeRunLocalExpectation, lineageGroups ...[]string) (resumeRunLocalObservation, error) {
	inventory, err := a.strictRemoteAgentRefInventory(root)
	if err != nil {
		return resumeRunLocalObservation{}, err
	}
	fixed := 0
	for _, claim := range inventory.claims {
		if claim.number != issue {
			continue
		}
		if claim.branch != fixedBranch {
			return resumeRunLocalObservation{}, stateError("issue #%d has conflicting claim ref %s", issue, claim.branch)
		}
		if claim.sha != fixedHead {
			return resumeRunLocalObservation{}, stateError("remote fixed branch moved during proof: expected %s, found %s", fixedHead, claim.sha)
		}
		fixed++
	}
	if fixed != 1 {
		return resumeRunLocalObservation{}, stateError("issue #%d fixed claim ref inventory is ambiguous", issue)
	}
	lineage := []string(nil)
	lineageProvided := false
	if len(lineageGroups) > 1 {
		return resumeRunLocalObservation{}, stateError("issue #%d resume received multiple evaluated lineages", issue)
	}
	if len(lineageGroups) == 1 {
		lineage = lineageGroups[0]
		lineageProvided = true
	}
	observation := resumeRunLocalObservation{}
	currentRunBranch := claimLocalBranch(issue, currentRunID)
	for _, claim := range inventory.runLocals {
		if claim.number != issue {
			continue
		}
		current := (claim.branch == currentBranch || claim.branch == currentRunBranch) && claim.runID == currentRunID
		if current {
			if observation.present {
				return resumeRunLocalObservation{}, stateError("issue #%d has multiple current run-local refs for run %s", issue, currentRunID)
			}
			if expectation.set && !expectation.present {
				return resumeRunLocalObservation{}, runLocalSourceRace("remote", stateError("issue #%d current run-local ref %s appeared during proof at %s", issue, claim.branch, claim.sha))
			}
			if expectation.present && claim.sha != expectation.sha {
				return resumeRunLocalObservation{}, runLocalSourceRace("remote", stateError("issue #%d current run-local ref %s moved during proof: expected %s, found %s", issue, claim.branch, expectation.sha, claim.sha))
			}
			if claim.sha == "" {
				return resumeRunLocalObservation{}, stateError("issue #%d current run-local ref %s has an empty head", issue, claim.branch)
			}
			if err := a.validateResumeRunLocalHead(root, issue, claim.branch, claim.runID, claim.sha, fixedHead); err != nil {
				return resumeRunLocalObservation{}, err
			}
			observation = resumeRunLocalObservation{branch: claim.branch, sha: claim.sha, present: true}
			continue
		}
		if lineageProvided && !containsRunID(lineage, claim.runID) {
			continue
		}
		return resumeRunLocalObservation{}, stateError("issue #%d has conflicting run-local ref %s (run %s); preserve it and resolve the duplicate before resume", issue, claim.branch, claim.runID)
	}
	if expectation.present && !observation.present {
		return resumeRunLocalObservation{}, runLocalSourceRace("remote", stateError("issue #%d current run-local ref disappeared during proof; expected %s", issue, expectation.sha))
	}
	for _, ref := range inventory.malformed {
		if strings.HasPrefix(ref.branch, fixedBranch) {
			return resumeRunLocalObservation{}, stateError("issue #%d has malformed conflicting agent ref %s", issue, ref.branch)
		}
	}
	return observation, nil
}

func (a app) validateResumeRunLocalHead(root string, issue int, branch, runID, head, expected string) error {
	if err := a.validateLocalAgentCommit(root, head, "run-local ref "+branch); err != nil {
		return err
	}
	if head == expected {
		return nil
	}
	_, err := a.command(root, "git", "merge-base", "--is-ancestor", head, expected)
	if err == nil {
		return nil
	}
	if isGitNonAncestor(err) {
		return stateError("issue #%d has conflicting run-local ref %s (run %s); head %s is not an ancestor of expected fixed head %s, preserve it and resolve the duplicate before resume", issue, branch, runID, head, expected)
	}
	return fmt.Errorf("prove current run-local ref %s at %s is an ancestor of expected fixed head %s: %w", branch, head, expected, err)
}

func (a app) inspectResumeLocalAncestor(root, local, expected, branch string) (bool, error) {
	if err := a.validateLocalAgentCommit(root, local, "local claim head"); err != nil {
		return false, err
	}
	if local == expected {
		return false, nil
	}
	if _, err := a.command(root, "git", "merge-base", "--is-ancestor", local, expected); err != nil {
		if isGitNonAncestor(err) {
			return false, nil
		}
		return false, fmt.Errorf("prove local claim head %s is an ancestor of expected head %s: %w", local, expected, err)
	}
	status, err := a.command(root, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return false, fmt.Errorf("inspect clean local ancestor worktree %s: %w", branch, err)
	}
	if strings.TrimSpace(status) != "" {
		return false, stateError("local claim worktree %s is not clean before fast-forward; preserve its staged, unstaged, and untracked changes", branch)
	}
	return true, nil
}

func (a app) validateResumeStagedWorktree(root string) error {
	status, cause := a.resumeStagedWorktreeStatus(root)
	if status == 0 && cause != nil {
		return cause
	}
	if status == 0 {
		return nil
	}
	if status == 1 {
		return terminalOperation("PR resume staged-worktree check", stateError("claim worktree has staged changes; preserve them before recovery"))
	}
	if cause != nil {
		return retryableOperation("PR resume staged-worktree check", fmt.Errorf("git diff --cached --quiet exited with status %d: %w", status, cause))
	}
	return retryableOperation("PR resume staged-worktree check", fmt.Errorf("git diff --cached --quiet exited with status %d", status))
}

func (a app) resumeStagedWorktreeStatus(root string) (int, error) {
	if a.executeCommandCapture != nil {
		result, err := a.commandCaptureWithEnv(root, nil, "git", "diff", "--cached", "--quiet")
		if err != nil {
			return 0, retryableOperation("PR resume staged-worktree check", fmt.Errorf("run git diff --cached --quiet: %w", err))
		}
		return result.status, nil
	}
	_, err := a.command(root, "git", "diff", "--cached", "--quiet")
	if err == nil {
		return 0, nil
	}
	if operationDispositionOf(err) != operationDispositionUnknown {
		return 0, err
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, retryableOperation("PR resume staged-worktree check", fmt.Errorf("run git diff --cached --quiet: %w", err))
	}
	return exitErr.ExitCode(), err
}

func (a app) resumeRunLocalLineage(root, head string, issue int) ([]string, error) {
	history, err := a.command(root, "git", "log", "--format=%H%x00%B%x00", head)
	if err != nil {
		return nil, retryableOperation("read evaluated claim lineage", fmt.Errorf("read evaluated claim lineage at %s: %w", head, err))
	}
	records, err := splitRunLocalHistory(history, fmt.Sprintf("agent/issue-%d", issue))
	if err != nil {
		return nil, err
	}
	if err := a.verifyRunLocalHistoryRecords(root, fmt.Sprintf("agent/issue-%d", issue), records); err != nil {
		return nil, err
	}
	lineage := make([]string, 0)
	for _, record := range records {
		identity, canonical, parseErr := parseCanonicalRunLocalClaim(record.message, issue)
		if parseErr != nil {
			return nil, terminalRunLocalHistoryError(stateError("preserve resume proof: history commit %s has malformed canonical claim marker: %w", record.commit, parseErr))
		}
		if canonical {
			lineage = appendUniqueString(lineage, identity.runID)
		}
	}
	return lineage, nil
}

func containsRunID(lineage []string, runID string) bool {
	for _, current := range lineage {
		if current == runID {
			return true
		}
	}
	return false
}

// sealPullRequestResumeProof makes the last operations in a proof fresh reads of
// every mutable authority. The caller mutates no ref or GitHub state before it.
func (a app) sealPullRequestResumeProof(proof resumeProof) error {
	view, err := a.readPullRequestForResume(proof.root, proof.pr)
	if err != nil {
		return err
	}
	if view.State != "OPEN" || view.Merged || view.BaseRefName != "main" ||
		view.HeadRefName != claimBranch(proof.issue) || view.HeadRefOID != proof.observedHead ||
		len(view.ClosingIssuesReferences) > 2 || !pullRequestCloses(view, proof.issue) {
		return stateError("PR #%d changed while sealing resume proof", proof.pr)
	}
	remote, err := a.remoteClaimHead(proof.root, claimBranch(proof.issue))
	if err != nil {
		return err
	}
	if remote != proof.observedHead {
		return stateError("remote fixed branch moved while sealing proof: expected %s, found %s", proof.observedHead, remote)
	}
	local, err := a.command(proof.root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read local claim head while sealing proof: %w", err)
	}
	if local != proof.renewalHead {
		return stateError("local claim head moved while sealing proof: expected %s, found %s", proof.renewalHead, local)
	}
	stagedErr := a.validateResumeStagedWorktree(proof.root)
	if stagedErr != nil {
		return stagedErr
	}
	lineage, err := a.validateResumeSealWorktree(proof, local)
	if err != nil {
		return err
	}
	status, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		return err
	}
	if status.State != "OPEN" || issueNeedsHuman(status) != proof.needsHuman {
		return stateError("issue #%d state changed while sealing resume proof", proof.issue)
	}
	_, err = a.inspectResumeClaimConflicts(proof.root, proof.issue, claimBranch(proof.issue), remote, proof.localBranch, proof.runID,
		resumeRunLocalExpectation{sha: proof.runLocalHead, present: proof.runLocalPresent, set: true}, lineage)
	return err
}

func (a app) validateResumeSealWorktree(proof resumeProof, local string) ([]string, error) {
	layout, err := a.repositoryLayout(proof.root)
	if err != nil {
		return nil, err
	}
	lineage, err := a.resumeRunLocalLineage(proof.root, proof.expectedHead, proof.issue)
	if err != nil {
		return nil, err
	}
	protectedHeads := resumeProtectedHeads(proof.expectedHead, proof.observedHead)
	if !proof.localAncestor && local != proof.expectedHead {
		protectedHeads = resumeProtectedHeads(proof.expectedHead, proof.observedHead, local)
	}
	if worktreeErr := validateResumeWorktreeHeads(layout, proof.root, proof.localBranch, proof.issue, local, protectedHeads, lineage); worktreeErr != nil {
		return nil, worktreeErr
	}
	if !proof.localAncestor {
		return lineage, nil
	}
	localAncestor, err := a.inspectResumeLocalAncestor(proof.root, local, proof.expectedHead, proof.localBranch)
	if err != nil {
		return nil, err
	}
	if !localAncestor {
		return nil, stateError("local ancestor resume proof changed while sealing")
	}
	return lineage, nil
}

func (a app) remoteClaimHead(root, branch string) (string, error) {
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return "", retryableOperation("read remote claim branch "+branch, fmt.Errorf("read remote claim branch %s: %w", branch, err))
	}
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return "", terminalOperation("read remote claim branch "+branch, stateError("remote fixed claim branch %s is absent or ambiguous", branch))
	}
	advertisedBranch, namespaceErr := remoteAgentRefBranch(fields[1])
	if namespaceErr != nil {
		return "", terminalOperation("read remote claim branch "+branch, namespaceErr)
	}
	if advertisedBranch != branch {
		return "", terminalOperation("read remote claim branch "+branch, stateError("remote fixed claim branch %s is absent or ambiguous", branch))
	}
	if err := a.validateRemoteAgentCommit(root, branch, fields[0]); err != nil {
		return "", err
	}
	return fields[0], nil
}

func validateResumeWorktree(layout repositoryLayout, root, branch string, issue int, head string, lineageGroups ...[]string) error {
	return validateResumeWorktreeHeads(layout, root, branch, issue, head, []string{head}, lineageGroups...)
}

//nolint:gocognit // Worktree uniqueness and stale-lineage filtering must be checked together.
func validateResumeWorktreeHeads(layout repositoryLayout, root, branch string, issue int, head string, protectedHeads []string, lineageGroups ...[]string) error {
	if len(protectedHeads) == 0 {
		return stateError("resume has no protected claim heads")
	}
	lineage := []string(nil)
	lineageProvided := false
	if len(lineageGroups) > 1 {
		return stateError("resume received multiple evaluated worktree lineages")
	}
	if len(lineageGroups) == 1 {
		lineage = lineageGroups[0]
		lineageProvided = true
	}
	count := 0
	for _, worktree := range layout.worktrees {
		candidate := strings.TrimPrefix(worktree.branch, "refs/heads/")
		candidateIssue, ok := issueFromBranch(candidate)
		lineageHead := containsResumeHead(protectedHeads, worktree.head)
		if !ok || candidateIssue != issue {
			if !lineageHead {
				continue
			}
			return stateError("detached duplicate/orphan claim worktree %q at %s shares the expected claim lineage; preserve it before recovery", worktree.path, worktree.head)
		}
		if !samePath(worktree.path, root) {
			candidateKind, _, candidateRunID := classifyAgentRef(candidate)
			if lineageProvided && candidateKind == agentRefRunLocal && !containsRunID(lineage, candidateRunID) {
				continue
			}
			return stateError("stale duplicate/orphan claim worktree %q at %s blocks recovery; prove its branch, run ID, head reachability, cleanliness, and archive ref before existing safe cleanup", worktree.path, worktree.head)
		}
		if candidate != branch || worktree.head != head || worktree.locked {
			return stateError("current worktree registration does not match branch %s at %s", branch, head)
		}
		count++
	}
	if count != 1 {
		return stateError("current claim worktree %q is not uniquely registered", root)
	}
	return nil
}

func containsResumeHead(heads []string, candidate string) bool {
	for _, head := range heads {
		if head == candidate {
			return true
		}
	}
	return false
}

func resumeProtectedHeads(heads ...string) []string {
	protected := make([]string, 0, len(heads))
	for _, head := range heads {
		if head == "" || containsResumeHead(protected, head) {
			continue
		}
		protected = append(protected, head)
	}
	return protected
}

func (a app) validateExistingResumeCommit(root, head, expected string, issue int, runID string) error {
	commit, err := a.readCanonicalClaimCommit(root, head, issue, runID, expected)
	if err != nil {
		return retryableOperationIfRecoverable("resume canonical renewal proof", err)
	}
	if !commit.lease.After(time.Now().UTC()) {
		return stateError("observed head %s has an expired claim renewal for existing run %s", head, runID)
	}
	return nil
}

func (a app) applyPullRequestResume(proof resumeProof) error {
	// This is the final read-only proof. No ref or GitHub mutation may precede it.
	fresh, readErr := a.readPullRequestResumeProof(proof.pr, proof.expectedHead)
	if readErr != nil {
		readErr = retryableOperationIfRecoverable("PR resume fresh proof", readErr)
		return fmt.Errorf("PR #%d resume proof changed after preflight; no mutation performed: %w", proof.pr, readErr)
	}
	if proofErr := sameResumeProof(proof, fresh); proofErr != nil {
		proofErr = retryableOperationIfRecoverable("PR resume proof comparison", proofErr)
		return fmt.Errorf("PR #%d resume proof changed after preflight; no mutation performed: %w", proof.pr, proofErr)
	}
	if !fresh.already {
		var mutationErr error
		fresh, mutationErr = a.mutatePullRequestResume(proof, fresh)
		if mutationErr != nil {
			return mutationErr
		}
	}
	if err := a.verifyResumePush(fresh); err != nil {
		err = retryableOperationIfRecoverable("PR resume push verification", err)
		return fmt.Errorf("PR #%d claim push needs reconciliation: %w. "+resumeRecoveryTemplate, proof.pr, err,
			proof.pr, proof.expectedHead)
	}
	if err := a.verifyClaim(); err != nil {
		err = retryableOperationIfRecoverable("PR resume claim verification", err)
		return fmt.Errorf("PR #%d claim renewal needs reconciliation: %w. "+resumeRecoveryTemplate, proof.pr, err,
			proof.pr, proof.expectedHead)
	}
	status, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		err = retryableOperationIfRecoverable("PR resume label status", err)
		return fmt.Errorf("PR #%d label reconciliation failed: %w. "+resumeRecoveryTemplate, proof.pr, err, proof.pr, proof.expectedHead)
	}
	if issueNeedsHuman(status) {
		if _, err := a.command(proof.root, "gh", "issue", "edit", strconv.Itoa(proof.issue), "--repo", repositoryKey,
			"--remove-label", "needs-human"); err != nil {
			err = retryableOperationIfRecoverable("PR resume label mutation", err)
			return fmt.Errorf("PR #%d label reconciliation failed: %w. "+resumeRecoveryTemplate, proof.pr, err,
				proof.pr, proof.expectedHead)
		}
	}
	if err := a.setIssueProjectStatus(proof.root, proof.issue, "Picked"); err != nil {
		err = retryableOperationIfRecoverable("PR resume Project reconciliation", err)
		return fmt.Errorf("PR #%d Project reconciliation failed: %w. "+resumeRecoveryTemplate, proof.pr, err,
			proof.pr, proof.expectedHead)
	}
	return writeLine(a.stdout, "PR #%d resumed for issue #%d; claim verified, needs-human removed, Project Picked", proof.pr, proof.issue)
}

func (a app) mutatePullRequestResume(proof, fresh resumeProof) (resumeProof, error) {
	status, statusErr := a.readIssueStatus(fresh.root, fresh.issue)
	if statusErr != nil {
		statusErr = retryableOperationIfRecoverable("PR resume issue status", statusErr)
		return resumeProof{}, fmt.Errorf("PR #%d issue proof failed immediately before mutation; no mutation performed: %w. "+resumeRecoveryTemplate,
			fresh.pr, statusErr, fresh.pr, fresh.expectedHead)
	}
	if status.State != "OPEN" || !issueNeedsHuman(status) {
		return resumeProof{}, stateError("issue #%d must remain open and labeled needs-human immediately before PR #%d resume mutation; no mutation performed. "+resumeRecoveryTemplate,
			fresh.issue, fresh.pr, fresh.pr, fresh.expectedHead)
	}
	if fresh.localAncestor {
		if _, err := a.command(fresh.root, "git", "merge", "--ff-only", fresh.expectedHead); err != nil {
			err = retryableOperationIfRecoverable("PR resume local fast-forward", err)
			return resumeProof{}, fmt.Errorf("fast-forward local claim worktree to expected head %s: %w", fresh.expectedHead, err)
		}
	}
	commit := fresh.renewalHead
	if !fresh.pending || fresh.localAncestor {
		var createErr error
		commit, _, _, createErr = a.newClaimCommitWithRunID(fresh.root, fresh.issue, fresh.observedHead, fresh.runID)
		if createErr != nil {
			return resumeProof{}, retryableOperationIfRecoverable("PR resume renewal commit", createErr)
		}
		if _, updateErr := a.command(fresh.root, "git", "update-ref", "refs/heads/"+fresh.localBranch, commit, fresh.observedHead); updateErr != nil {
			updateErr = retryableOperationIfRecoverable("PR resume local ref update", updateErr)
			return resumeProof{}, fmt.Errorf("advance local claim for PR resume: %w", updateErr)
		}
	}
	lease := "--force-with-lease=refs/heads/" + claimBranch(fresh.issue) + ":" + fresh.observedHead
	refspec := commit + ":refs/heads/" + claimBranch(fresh.issue)
	if _, pushErr := a.command(fresh.root, "git", "push", lease, "origin", refspec); pushErr != nil {
		pushErr = retryableOperationIfRecoverable("PR resume claim push", pushErr)
		return resumeProof{}, fmt.Errorf("PR #%d claim push response was ambiguous: %w. "+resumeRecoveryTemplate, proof.pr, pushErr,
			proof.pr, proof.expectedHead)
	}
	fresh.renewalHead = commit
	return fresh, nil
}

func sameResumeProof(before, after resumeProof) error {
	if before.root != after.root || before.localBranch != after.localBranch || before.issue != after.issue ||
		before.pr != after.pr || before.expectedHead != after.expectedHead || before.observedHead != after.observedHead ||
		before.renewalHead != after.renewalHead || before.runID != after.runID || before.localAncestor != after.localAncestor ||
		before.runLocalHead != after.runLocalHead ||
		before.runLocalPresent != after.runLocalPresent || before.already != after.already || before.pending != after.pending ||
		before.needsHuman != after.needsHuman {
		return stateError("bound PR/ref/local/worktree/issue proof no longer matches")
	}
	return nil
}

func (a app) verifyResumePush(proof resumeProof) error {
	view, err := a.readPullRequestForResume(proof.root, proof.pr)
	if err != nil {
		return err
	}
	remote, err := a.remoteClaimHead(proof.root, claimBranch(proof.issue))
	if err != nil {
		return err
	}
	if view.State != "OPEN" || view.Merged || view.HeadRefName != claimBranch(proof.issue) ||
		view.HeadRefOID != proof.renewalHead || remote != proof.renewalHead {
		return stateError("post-push heads disagree: renewed=%s PR=%s remote=%s", proof.renewalHead, view.HeadRefOID, remote)
	}
	return a.validateExistingResumeCommit(proof.root, proof.renewalHead, proof.expectedHead, proof.issue, proof.runID)
}
