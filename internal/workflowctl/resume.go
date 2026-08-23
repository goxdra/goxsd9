package workflowctl

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const resumeRecoveryTemplate = "Run `go tool workflowctl pr resume %d --expected-head %s --acknowledge-needs-human` again"

type resumeProof struct {
	root         string
	localBranch  string
	issue        int
	pr           int
	expectedHead string
	observedHead string
	renewalHead  string
	runID        string
	already      bool
	pending      bool
	needsHuman   bool
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
	return a.readPullRequestResumeProof(pr, expectedHead)
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
	view, err := a.readPullRequest(root, pr)
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
	if _, diffErr := a.command(root, "git", "diff", "--cached", "--quiet"); diffErr != nil {
		return resumeProof{}, stateError("claim worktree has staged changes; preserve them before recovery")
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return resumeProof{}, err
	}
	if worktreeErr := validateResumeWorktree(layout, root, localBranch, issue, local); worktreeErr != nil {
		return resumeProof{}, worktreeErr
	}
	lease, runID, err := a.readClaimMetadataAt(root, expectedHead)
	if err != nil {
		return resumeProof{}, stateError("expected head %s has no valid claim identity: %v", expectedHead, err)
	}
	if err := a.validateClaimIssueAt(root, expectedHead, issue); err != nil {
		return resumeProof{}, err
	}
	if err := validateClaimLocalBranch(localBranch, issue, runID); err != nil {
		return resumeProof{}, err
	}
	if lease.After(time.Now().UTC()) {
		return resumeProof{}, stateError("claim #%d is active until %s; use claim renew", issue, lease.Format(time.RFC3339))
	}
	if err := a.rejectResumeClaimConflicts(root, issue, branch, remote); err != nil {
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
		if err := a.validateExistingResumeCommit(root, local, expectedHead, issue, runID); err != nil {
			return resumeProof{}, stateError("local head differs from the observed remote claim and is not a retryable renewal: %v", err)
		}
	}
	if !already && !pending && (view.HeadRefOID != expectedHead || local != expectedHead) {
		return resumeProof{}, stateError("resume heads moved: expected=%s PR=%s remote=%s local=%s", expectedHead, view.HeadRefOID, remote, local)
	}
	if !already && !issueNeedsHuman(status) {
		return resumeProof{}, stateError("issue #%d must be labeled needs-human before stale PR recovery", issue)
	}
	proof := resumeProof{root: root, localBranch: localBranch, issue: issue, pr: pr, expectedHead: expectedHead,
		observedHead: remote, renewalHead: local, runID: runID, already: already, pending: pending,
		needsHuman: issueNeedsHuman(status)}
	if err := a.sealPullRequestResumeProof(proof); err != nil {
		return resumeProof{}, err
	}
	return proof, nil
}

func (a app) rejectResumeClaimConflicts(root string, issue int, fixedBranch, fixedHead string) error {
	inventory, err := a.remoteAgentRefInventory(root)
	if err != nil {
		return err
	}
	fixed := 0
	for _, claim := range inventory.claims {
		if claim.number != issue {
			continue
		}
		if claim.branch != fixedBranch {
			return stateError("issue #%d has conflicting claim ref %s", issue, claim.branch)
		}
		if claim.sha != fixedHead {
			return stateError("remote fixed branch moved during proof: expected %s, found %s", fixedHead, claim.sha)
		}
		fixed++
	}
	if fixed != 1 {
		return stateError("issue #%d fixed claim ref inventory is ambiguous", issue)
	}
	for _, claim := range inventory.runLocals {
		if claim.number == issue {
			return stateError("issue #%d has conflicting run-local ref %s (run %s); preserve it and resolve the duplicate before resume", issue, claim.branch, claim.runID)
		}
	}
	for _, ref := range inventory.malformed {
		if strings.HasPrefix(ref.branch, fixedBranch) {
			return stateError("issue #%d has malformed conflicting agent ref %s", issue, ref.branch)
		}
	}
	return nil
}

// sealPullRequestResumeProof makes the last operations in a proof fresh reads of
// every mutable authority. The caller mutates no ref or GitHub state before it.
func (a app) sealPullRequestResumeProof(proof resumeProof) error {
	view, err := a.readPullRequest(proof.root, proof.pr)
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
	if _, diffErr := a.command(proof.root, "git", "diff", "--cached", "--quiet"); diffErr != nil {
		return stateError("claim worktree gained staged changes while sealing proof")
	}
	layout, err := a.repositoryLayout(proof.root)
	if err != nil {
		return err
	}
	if worktreeErr := validateResumeWorktree(layout, proof.root, proof.localBranch, proof.issue, local); worktreeErr != nil {
		return worktreeErr
	}
	status, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		return err
	}
	if status.State != "OPEN" || issueNeedsHuman(status) != proof.needsHuman {
		return stateError("issue #%d state changed while sealing resume proof", proof.issue)
	}
	return a.rejectResumeClaimConflicts(proof.root, proof.issue, claimBranch(proof.issue), remote)
}

func (a app) remoteClaimHead(root, branch string) (string, error) {
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("read remote claim branch %s: %w", branch, err)
	}
	fields := strings.Fields(output)
	if len(fields) != 2 || fields[1] != "refs/heads/"+branch {
		return "", stateError("remote fixed claim branch %s is absent or ambiguous", branch)
	}
	return fields[0], nil
}

func (a app) readClaimMetadataAt(root, head string) (time.Time, string, error) {
	text, err := a.command(root, "git", "log", "-100", "--format=%B", head)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("read claim metadata at %s: %w", head, err)
	}
	lease, err := trailerTime(text)
	if err != nil {
		return time.Time{}, "", err
	}
	runID, err := trailerValue(text, "Agent-Run-ID")
	if err != nil {
		return time.Time{}, "", err
	}
	return lease, runID, nil
}

func (a app) validateClaimIssueAt(root, head string, expected int) error {
	text, err := a.command(root, "git", "log", "-100", "--format=%B", head)
	if err != nil {
		return fmt.Errorf("read claim issue at %s: %w", head, err)
	}
	value, err := trailerValue(text, "Agent-Issue")
	if err != nil {
		return stateError("expected head %s has no claim issue identity: %v", head, err)
	}
	issue, err := positiveNumber(value)
	if err != nil || issue != expected {
		return stateError("expected head %s claims issue %q, not issue #%d", head, value, expected)
	}
	return nil
}

func validateResumeWorktree(layout repositoryLayout, root, branch string, issue int, head string) error {
	count := 0
	for _, worktree := range layout.worktrees {
		candidate := strings.TrimPrefix(worktree.branch, "refs/heads/")
		candidateIssue, ok := issueFromBranch(candidate)
		if !ok || candidateIssue != issue {
			continue
		}
		if !samePath(worktree.path, root) {
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

func (a app) validateExistingResumeCommit(root, head, expected string, issue int, runID string) error {
	parent, err := a.command(root, "git", "rev-parse", head+"^")
	if err != nil || parent != expected {
		return stateError("observed head %s is not the unique renewal child of expected PR head %s", head, expected)
	}
	headTree, err := a.command(root, "git", "rev-parse", head+"^{tree}")
	if err != nil {
		return fmt.Errorf("read resumed claim tree: %w", err)
	}
	parentTree, err := a.command(root, "git", "rev-parse", expected+"^{tree}")
	if err != nil {
		return fmt.Errorf("read expected claim tree: %w", err)
	}
	lease, observedRun, err := a.readClaimMetadataAt(root, head)
	if err != nil || headTree != parentTree || observedRun != runID || !lease.After(time.Now().UTC()) {
		return stateError("observed head %s is not a valid empty claim renewal for existing run %s", head, runID)
	}
	message, err := a.command(root, "git", "log", "-1", "--format=%B", head)
	if err != nil || strings.TrimSpace(message) != strings.TrimSpace(claimMessage(issue, runID, lease)) {
		return stateError("observed head %s is not the standard claim metadata commit", head)
	}
	return nil
}

//nolint:gocognit // Mutation and deterministic recovery boundaries remain explicit and ordered.
func (a app) applyPullRequestResume(proof resumeProof) error {
	// This is the final read-only proof. No ref or GitHub mutation may precede it.
	fresh, readErr := a.readPullRequestResumeProof(proof.pr, proof.expectedHead)
	if readErr != nil {
		return stateError("PR #%d resume proof changed after preflight; no mutation performed: %v", proof.pr, readErr)
	}
	if proofErr := sameResumeProof(proof, fresh); proofErr != nil {
		return stateError("PR #%d resume proof changed after preflight; no mutation performed: %v", proof.pr, proofErr)
	}
	if !fresh.already {
		status, statusErr := a.readIssueStatus(fresh.root, fresh.issue)
		if statusErr != nil {
			return stateError("PR #%d issue proof failed immediately before mutation; no mutation performed: %v. "+resumeRecoveryTemplate,
				fresh.pr, statusErr, fresh.pr, fresh.expectedHead)
		}
		if status.State != "OPEN" || !issueNeedsHuman(status) {
			return stateError("issue #%d must remain open and labeled needs-human immediately before PR #%d resume mutation; no mutation performed. "+resumeRecoveryTemplate,
				fresh.issue, fresh.pr, fresh.pr, fresh.expectedHead)
		}
		commit := fresh.renewalHead
		if !fresh.pending {
			var createErr error
			commit, _, _, createErr = a.newClaimCommitWithRunID(fresh.root, fresh.issue, fresh.observedHead, fresh.runID)
			if createErr != nil {
				return createErr
			}
			if _, updateErr := a.command(fresh.root, "git", "update-ref", "refs/heads/"+fresh.localBranch, commit, fresh.observedHead); updateErr != nil {
				return fmt.Errorf("advance local claim for PR resume: %w", updateErr)
			}
		}
		lease := "--force-with-lease=refs/heads/" + claimBranch(fresh.issue) + ":" + fresh.observedHead
		refspec := commit + ":refs/heads/" + claimBranch(fresh.issue)
		if _, pushErr := a.command(fresh.root, "git", "push", lease, "origin", refspec); pushErr != nil {
			return stateError("PR #%d claim push response was ambiguous: %v. "+resumeRecoveryTemplate, proof.pr, pushErr,
				proof.pr, proof.expectedHead)
		}
		fresh.renewalHead = commit
	}
	if err := a.verifyResumePush(fresh); err != nil {
		return stateError("PR #%d claim push needs reconciliation: %v. "+resumeRecoveryTemplate, proof.pr, err,
			proof.pr, proof.expectedHead)
	}
	if err := a.verifyClaim(); err != nil {
		return stateError("PR #%d claim renewal needs reconciliation: %v. "+resumeRecoveryTemplate, proof.pr, err,
			proof.pr, proof.expectedHead)
	}
	status, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		return stateError("PR #%d label reconciliation failed: %v. "+resumeRecoveryTemplate, proof.pr, err, proof.pr, proof.expectedHead)
	}
	if issueNeedsHuman(status) {
		if _, err := a.command(proof.root, "gh", "issue", "edit", strconv.Itoa(proof.issue), "--repo", repositoryKey,
			"--remove-label", "needs-human"); err != nil {
			return stateError("PR #%d label reconciliation failed: %v. "+resumeRecoveryTemplate, proof.pr, err,
				proof.pr, proof.expectedHead)
		}
	}
	if err := a.setIssueProjectStatus(proof.root, proof.issue, "Picked"); err != nil {
		return stateError("PR #%d Project reconciliation failed: %v. "+resumeRecoveryTemplate, proof.pr, err,
			proof.pr, proof.expectedHead)
	}
	return writeLine(a.stdout, "PR #%d resumed for issue #%d; claim verified, needs-human removed, Project Picked", proof.pr, proof.issue)
}

func sameResumeProof(before, after resumeProof) error {
	if before.root != after.root || before.localBranch != after.localBranch || before.issue != after.issue ||
		before.pr != after.pr || before.expectedHead != after.expectedHead || before.observedHead != after.observedHead ||
		before.renewalHead != after.renewalHead || before.runID != after.runID || before.already != after.already ||
		before.pending != after.pending || before.needsHuman != after.needsHuman {
		return stateError("bound PR/ref/local/worktree/issue proof no longer matches")
	}
	return nil
}

func (a app) verifyResumePush(proof resumeProof) error {
	view, err := a.readPullRequest(proof.root, proof.pr)
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
