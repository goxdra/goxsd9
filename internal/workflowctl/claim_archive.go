package workflowctl

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type archivedClaimReleaseProof struct {
	root          string
	path          string
	branch        string
	archive       string
	head          string
	currentPath   string
	currentBranch string
	currentHead   string
	issue         int
	runID         string
	present       bool
}

func (a app) releaseArchivedClaimCommand(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl claim release-archived ISSUE --run-id RUN --expected-head SHA [--dry-run]")
	}
	issue, err := positiveNumber(args[0])
	if err != nil {
		return usageError("claim release-archived: %v", err)
	}
	flags := flag.NewFlagSet("claim release-archived", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runID := flags.String("run-id", "", "archived claim run ID")
	head := flags.String("expected-head", "", "exact archived claim head")
	dryRun := flags.Bool("dry-run", false, "prove release without mutation")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("claim release-archived: %v", parseErr)
	}
	if flags.NArg() != 0 || !validRunID(*runID) || !validExactCommitSHA(*head) {
		return usageError("usage: workflowctl claim release-archived ISSUE --run-id RUN --expected-head SHA [--dry-run]")
	}
	proof, err := a.readArchivedClaimReleaseProof(issue, *runID, *head)
	if err != nil {
		return err
	}
	if writeErr := writeLine(a.stdout, "archived claim proof: issue #%d run %s branch %s head %s archive %s worktree %s", issue,
		proof.runID, proof.branch, proof.head, proof.archive, proof.path); writeErr != nil {
		return fmt.Errorf("write archived claim proof: %w", writeErr)
	}
	if *dryRun {
		return writeLine(a.stdout, "dry-run: preflight complete; no mutation performed")
	}
	if !proof.present {
		return writeLine(a.stdout, "archived claim worktree already released; refs preserved")
	}
	fresh, err := a.readArchivedClaimReleaseProof(issue, *runID, *head)
	if err != nil {
		return fmt.Errorf("refresh archived claim proof before removal: %w", err)
	}
	if fresh != proof {
		return stateError("archived claim proof changed before removal; preserve worktree and refs")
	}
	if _, removeErr := a.command(proof.root, "git", "worktree", "remove", "--force", proof.path); removeErr != nil {
		return fmt.Errorf("release clean archived claim worktree %q: %w", proof.path, removeErr)
	}
	confirmed, err := a.readArchivedClaimReleaseProof(issue, *runID, *head)
	if err != nil {
		return fmt.Errorf("verify archived claim release: %w", err)
	}
	if confirmed.present {
		return stateError("archived claim worktree %q remains registered; preserve refs", proof.path)
	}
	return writeLine(a.stdout, "archived claim worktree released; local and remote refs preserved")
}

// readArchivedClaimReleaseProof binds one run-local branch to a canonical
// claim marker and the single remote archive at its exact head.
//
//nolint:gocognit,funlen // The proof seals all ownership and worktree checks before removal.
func (a app) readArchivedClaimReleaseProof(issue int, runID, head string) (archivedClaimReleaseProof, error) {
	root, currentBranch, _, err := a.currentClaim()
	if err != nil {
		return archivedClaimReleaseProof{}, err
	}
	currentKind, currentBranchIssue, _ := classifyAgentRef(currentBranch)
	if currentKind != agentRefRunLocal || currentBranchIssue < 1 || currentBranch == claimLocalBranch(issue, runID) {
		return archivedClaimReleaseProof{}, stateError("archived release requires a different current claim worktree")
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return archivedClaimReleaseProof{}, err
	}
	branch := claimLocalBranch(issue, runID)
	path := claimWorktreePath(layout.primaryRoot, branch)
	if samePath(root, path) || samePath(layout.primaryRoot, path) {
		return archivedClaimReleaseProof{}, stateError("archived worktree %q is the current or primary checkout", path)
	}
	localHead, err := a.command(root, "git", "for-each-ref", "--format=%(objectname)", "refs/heads/"+branch)
	if err != nil {
		return archivedClaimReleaseProof{}, fmt.Errorf("inspect archived local branch %s: %w", branch, err)
	}
	if localHead != head {
		return archivedClaimReleaseProof{}, stateError("archived local branch %s has head %q, expected %s; preserve artifacts", branch, localHead, head)
	}
	marker, err := a.readCanonicalClaimCommit(root, head, issue, runID, "")
	if err != nil {
		return archivedClaimReleaseProof{}, fmt.Errorf("prove archived run marker %s: %w", head, err)
	}
	if marker.lease.After(time.Now().UTC()) {
		return archivedClaimReleaseProof{}, stateError("archived run %s is active until %s; preserve artifacts", runID, marker.lease.Format(time.RFC3339))
	}
	inventory, err := a.strictRemoteAgentRefInventory(root)
	if err != nil {
		return archivedClaimReleaseProof{}, err
	}
	archive, err := exactArchivedClaimRef(inventory, issue, head)
	if err != nil {
		return archivedClaimReleaseProof{}, err
	}
	fixedHead, err := claimResumeFixedHead(inventory, issue, claimBranch(issue))
	if err != nil {
		return archivedClaimReleaseProof{}, err
	}
	if prErr := a.validateNoOpenClaimResumePR(root, claimBranch(issue), issue); prErr != nil {
		return archivedClaimReleaseProof{}, prErr
	}
	proof := archivedClaimReleaseProof{root: root, path: path, branch: branch, archive: archive,
		head: head, issue: issue, runID: runID}
	for _, worktree := range layout.worktrees {
		candidate := strings.TrimPrefix(worktree.branch, "refs/heads/")
		candidateIssue, sameIssue := issueFromBranch(candidate)
		if !samePath(worktree.path, path) {
			if worktree.head == head {
				return archivedClaimReleaseProof{}, stateError("ambiguous issue #%d worktree %q at archived head %s", issue, worktree.path, head)
			}
			if !sameIssue || candidateIssue != issue {
				continue
			}
			if proof.currentPath != "" || worktree.head != fixedHead || worktree.locked || worktree.bare || worktree.prunable != "" {
				return archivedClaimReleaseProof{}, stateError("ambiguous issue #%d current worktree %q at %s; preserve artifacts", issue, worktree.path, worktree.head)
			}
			kind, _, currentRunID := classifyAgentRef(candidate)
			if kind != agentRefRunLocal || currentRunID == runID || !samePath(worktree.path, claimWorktreePath(layout.primaryRoot, candidate)) {
				return archivedClaimReleaseProof{}, stateError("issue #%d current worktree %q has unproven run identity", issue, worktree.path)
			}
			if _, markerErr := a.readCanonicalClaimCommit(root, worktree.head, issue, currentRunID, ""); markerErr != nil {
				return archivedClaimReleaseProof{}, fmt.Errorf("prove current issue #%d run %s: %w", issue, currentRunID, markerErr)
			}
			proof.currentPath, proof.currentBranch, proof.currentHead = worktree.path, candidate, worktree.head
			continue
		}
		if proof.present || worktree.branch != "refs/heads/"+branch || worktree.head != head || worktree.locked || worktree.bare || worktree.prunable != "" {
			return archivedClaimReleaseProof{}, stateError("archived worktree %q is locked, detached, moved, or ambiguous; preserve artifacts", path)
		}
		proof.present = true
	}
	if proof.currentPath == "" {
		return archivedClaimReleaseProof{}, stateError("issue #%d has no unique current claim worktree at fixed head %s", issue, fixedHead)
	}
	if !proof.present {
		return proof, nil
	}
	status, err := a.command(root, "git", "-C", path, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return archivedClaimReleaseProof{}, fmt.Errorf("inspect archived worktree %q: %w", path, err)
	}
	if status != "" {
		return archivedClaimReleaseProof{}, stateError("archived worktree %q is dirty; preserve its files", path)
	}
	submodules, err := a.command(root, "git", "-C", path, "submodule", "foreach", "--recursive", "--quiet",
		"git status --porcelain=v1 --untracked-files=all")
	if err != nil {
		return archivedClaimReleaseProof{}, fmt.Errorf("inspect archived worktree %q submodules: %w", path, err)
	}
	if submodules != "" {
		return archivedClaimReleaseProof{}, stateError("archived worktree %q has dirty submodules; preserve their files", path)
	}
	return proof, nil
}

func exactArchivedClaimRef(inventory agentRefInventory, issue int, head string) (string, error) {
	prefix := "agent/archive/issue-" + strconv.Itoa(issue) + "-"
	var match string
	for _, ref := range inventory.archives {
		if !strings.HasPrefix(ref.branch, prefix) || ref.sha != head {
			continue
		}
		if !archiveSuffixIsGenerated(strings.TrimPrefix(ref.branch, prefix)) {
			return "", stateError("issue #%d archive ref %s is malformed; preserve ambiguous artifacts", issue, ref.branch)
		}
		if match != "" {
			return "", stateError("issue #%d has multiple archive refs at %s; preserve ambiguous artifacts", issue, head)
		}
		match = ref.branch
	}
	if match == "" {
		return "", stateError("issue #%d has no matching remote archive ref at %s; preserve artifacts", issue, head)
	}
	return match, nil
}

func archiveSuffixIsGenerated(suffix string) bool {
	if len(suffix) != 16 {
		return false
	}
	for _, char := range suffix {
		if char < '0' || char > '9' {
			if char < 'a' || char > 'f' {
				return false
			}
		}
	}
	return true
}

func (a app) archivedRunLocalRef(root string, inventory agentRefInventory, ref runLocalRef) (bool, error) {
	if !hasExactArchivedClaimRef(inventory, ref.number, ref.sha) {
		return false, nil
	}
	if _, err := a.readCanonicalClaimCommit(root, ref.sha, ref.number, ref.runID, ""); err != nil {
		return false, err
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return false, err
	}
	for _, worktree := range layout.worktrees {
		if worktree.branch == "refs/heads/"+ref.branch || worktree.head == ref.sha {
			return false, nil
		}
	}
	return true, nil
}

func hasExactArchivedClaimRef(inventory agentRefInventory, issue int, head string) bool {
	_, err := exactArchivedClaimRef(inventory, issue, head)
	return err == nil
}
