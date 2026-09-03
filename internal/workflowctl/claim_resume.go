package workflowctl

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const claimResumeRecoveryTemplate = "Run `go tool workflowctl claim resume %d --expected-head %s --run-id %s --handoff-comment %d --acknowledge-needs-human` again"

// claimResumeProof is the sealed preflight proof plus its explicit renewal
// state. It is passed by value through each phase and never mutated.
type claimResumeProof struct {
	preflight claimResumePreflight
	renewal   claimResumeRenewalPlan
}

// claimResumePreflight is the immutable read-only proof sealed before any
// local-ref or GitHub mutation. The claim worktree is the proof root, so it is
// represented once rather than duplicated as a second worktree field.
type claimResumePreflight struct {
	root             string
	localBranch      string
	fixedBranch      string
	expectedHead     string
	localHead        string
	remoteHead       string
	runID            string
	issue            int
	handoffCommentID int64
	claimCommentID   int64
	handoffBody      string
	claimLease       time.Time
	projectItemID    string
	projectStatus    string
	needsHuman       bool
}

// claimResumeRenewalPlan is a closed set of proof states. A missing renewal
// and a verified existing renewal cannot be represented by parallel booleans.
type claimResumeRenewalPlan interface {
	claimResumeRenewalPlan()
}

type claimResumeNoRenewal struct{}

func (claimResumeNoRenewal) claimResumeRenewalPlan() {}

type claimResumeExistingRenewal struct {
	head string
}

func (claimResumeExistingRenewal) claimResumeRenewalPlan() {}

// claimResumeRenewalProof is a canonical commit proven as the renewal child
// of the expected claim. It is the input to local adoption.
type claimResumeRenewalProof struct {
	head string
}

// claimResumeLocalRenewal is the same canonical child after the local run
// branch is known to point at it. It is the input to remote push/convergence.
type claimResumeLocalRenewal struct {
	head string
}

// claimResumeRenewalResult is the verified local and remote renewal result.
type claimResumeRenewalResult struct {
	head string
}

type claimResumeCommitMetadata struct {
	lease time.Time
	runID string
}

// canonicalClaimCommit is the immutable shape emitted by commit-tree for a
// claim marker.  A marker is deliberately an empty, single-parent commit;
// source changes and merge history are never part of claim ownership state.
type canonicalClaimCommit struct {
	parent  string
	tree    string
	message string
	issue   int
	runID   string
	lease   time.Time
}

type canonicalCommitObject struct {
	parent  string
	tree    string
	message string
}

type commitObject struct {
	parents []string
	tree    string
	message string
}

type claimResumeCommentEvidence struct {
	claimCommentID int64
	claimLease     time.Time
	handoffBody    string
}

type claimResumeClaimComment struct {
	branch      string
	localBranch string
	worktree    string
	runID       string
	lease       time.Time
}

type claimResumeAgentRefs struct {
	claims    []remoteClaim
	runLocals []runLocalRef
	malformed []agentRef
}

type openPullRequestNumber struct {
	Number int `json:"number"`
}

func (a app) resumeClaimCommand(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]")
	}
	issue, err := positiveNumber(args[0])
	if err != nil {
		return usageError("claim resume: %v", err)
	}
	flags := flag.NewFlagSet("claim resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expected-head", "", "expected fixed-branch SHA")
	runID := flags.String("run-id", "", "exact expired claim run ID")
	handoffComment := flags.String("handoff-comment", "", "exact terminal handoff comment ID")
	acknowledged := flags.Bool("acknowledge-needs-human", false, "acknowledge needs-human recovery")
	dryRun := flags.Bool("dry-run", false, "print the proof without mutation")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("claim resume: %v", parseErr)
	}
	if flags.NArg() != 0 {
		return usageError("usage: workflowctl claim resume ISSUE --expected-head SHA --run-id RUN --handoff-comment COMMENT-ID --acknowledge-needs-human [--dry-run]")
	}
	if !*acknowledged {
		return stateError("issue #%d claim recovery requires --acknowledge-needs-human", issue)
	}
	if !validExactCommitSHA(*expected) {
		return usageError("claim resume: --expected-head must be a full 40-character commit SHA")
	}
	if !validRunID(*runID) {
		return usageError("claim resume: --run-id must be a valid run ID")
	}
	commentID, parseErr := strconv.ParseInt(*handoffComment, 10, 64)
	if parseErr != nil || commentID < 1 || strconv.FormatInt(commentID, 10) != *handoffComment {
		return usageError("claim resume: --handoff-comment must be a positive decimal comment ID")
	}
	proof, err := a.readClaimResumeProof(issue, *expected, *runID, commentID)
	if err != nil {
		return retryableOperationIfRecoverable("claim resume proof", err)
	}
	if err := writeLine(a.stdout, "claim resume proof: issue #%d branch %s run %s expected %s handoff-comment %d",
		proof.preflight.issue, proof.preflight.fixedBranch, proof.preflight.runID, proof.preflight.expectedHead, proof.preflight.handoffCommentID); err != nil {
		return fmt.Errorf("write claim resume proof: %w", err)
	}
	if *dryRun {
		return writeLine(a.stdout, "dry-run: preflight complete; no mutation performed")
	}
	return a.applyClaimResume(proof)
}

// readClaimResumeProof is the read-only proof for an acknowledged terminal
// no-PR handoff. A valid renewal child is accepted only for retry convergence.
//
//nolint:gocognit,funlen // The proof intentionally keeps every authority in one ordered seal.
func (a app) readClaimResumeProof(issue int, expectedHead, runID string, handoffCommentID int64) (claimResumeProof, error) {
	root, localBranch, currentIssue, err := a.currentClaim()
	if err != nil {
		return claimResumeProof{}, err
	}
	if currentIssue != issue {
		return claimResumeProof{}, stateError("current claim issue #%d does not match requested issue #%d; no mutation performed", currentIssue, issue)
	}
	if localBranch != claimLocalBranch(issue, runID) {
		return claimResumeProof{}, stateError("local branch %q does not match issue #%d run %s; preserve the claim worktree", localBranch, issue, runID)
	}
	fixedBranch := claimBranch(issue)
	localHead, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return claimResumeProof{}, fmt.Errorf("read local claim head: %w", err)
	}
	if !validExactCommitSHA(localHead) {
		return claimResumeProof{}, stateError("local claim head %q is not a full commit SHA; preserve the claim worktree", localHead)
	}
	if validateErr := a.validateLocalAgentCommit(root, localHead, "local claim head"); validateErr != nil {
		return claimResumeProof{}, validateErr
	}
	inventory, err := a.strictRemoteAgentRefInventory(root)
	if err != nil {
		return claimResumeProof{}, err
	}
	remoteHead, err := claimResumeFixedHead(inventory, issue, fixedBranch)
	if err != nil {
		return claimResumeProof{}, err
	}
	if !validExactCommitSHA(remoteHead) {
		return claimResumeProof{}, stateError("remote fixed claim branch %s has malformed head %q; preserve claim artifacts", fixedBranch, remoteHead)
	}
	metadata, err := a.readExactClaimResumeMetadata(root, expectedHead, issue, runID)
	if err != nil {
		return claimResumeProof{}, err
	}
	evidence, err := a.readClaimResumeEvidence(root, issue, handoffCommentID, claimLocalBranch(issue, runID), runID)
	if err != nil {
		return claimResumeProof{}, err
	}
	if evidence.claimLease != metadata.lease || evidence.claimCommentID < 1 {
		return claimResumeProof{}, stateError("handoff comment %d is not bound to the exact expired claim lease; preserve evidence", handoffCommentID)
	}
	if bindingErr := validateClaimResumeHandoffBindings(evidence.handoffBody, issue, expectedHead, fixedBranch, localBranch, root, runID, metadata.lease); bindingErr != nil {
		return claimResumeProof{}, stateError("handoff comment %d is not bound to the exact claim artifacts; preserve evidence: %w", handoffCommentID, bindingErr)
	}
	if prErr := a.validateNoOpenClaimResumePR(root, fixedBranch, issue); prErr != nil {
		return claimResumeProof{}, prErr
	}
	renewal, err := a.claimResumeRenewalPlan(root, expectedHead, localHead, remoteHead, issue, runID)
	if err != nil {
		return claimResumeProof{}, err
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return claimResumeProof{}, err
	}
	protectedHeads := claimResumeProtectedHeads(expectedHead, renewal)
	if worktreeErr := validateResumeWorktreeHeads(layout, root, localBranch, issue, localHead, protectedHeads); worktreeErr != nil {
		return claimResumeProof{}, worktreeErr
	}
	statusOutput, err := a.command(root, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return claimResumeProof{}, fmt.Errorf("inspect claim worktree cleanliness: %w", err)
	}
	if strings.TrimSpace(statusOutput) != "" {
		return claimResumeProof{}, stateError("claim worktree %s is dirty; preserve its staged, unstaged, and untracked changes", localBranch)
	}
	if refErr := a.validateClaimResumeRefs(root, inventory, issue, fixedBranch, localBranch, runID, expectedHead, localHead, remoteHead); refErr != nil {
		return claimResumeProof{}, refErr
	}
	issueStatus, err := a.readIssueStatus(root, issue)
	if err != nil {
		return claimResumeProof{}, err
	}
	if issueStatus.State != "OPEN" {
		return claimResumeProof{}, stateError("issue #%d is %s; claim recovery requires OPEN and no mutation was performed", issue, issueStatus.State)
	}
	items, err := a.projectItems(root)
	if err != nil {
		return claimResumeProof{}, err
	}
	item, err := canonicalClaimResumeProjectItem(items, issue)
	if err != nil {
		return claimResumeProof{}, err
	}
	if item.Status != "Backlog" && item.Status != "Picked" {
		return claimResumeProof{}, stateError("issue #%d Project status %q is not a resumable Backlog/Picked state; preserve external state", issue, item.Status)
	}
	needsHuman := issueNeedsHuman(issueStatus)
	if _, existing := renewal.(claimResumeExistingRenewal); !existing && (!needsHuman || item.Status != "Backlog") {
		return claimResumeProof{}, stateError("issue #%d requires needs-human and Project Backlog before no-PR claim recovery; no mutation performed", issue)
	}
	return claimResumeProof{
		preflight: claimResumePreflight{
			root: root, localBranch: localBranch, fixedBranch: fixedBranch,
			expectedHead: expectedHead, localHead: localHead, remoteHead: remoteHead,
			runID: runID, issue: issue, handoffCommentID: handoffCommentID,
			claimCommentID: evidence.claimCommentID, handoffBody: evidence.handoffBody,
			claimLease: metadata.lease, projectItemID: item.ID, projectStatus: item.Status,
			needsHuman: needsHuman,
		},
		renewal: renewal,
	}, nil
}

func claimResumeFixedHead(inventory agentRefInventory, issue int, fixedBranch string) (string, error) {
	count := 0
	var head string
	for _, claim := range inventory.claims {
		if claim.number != issue {
			continue
		}
		if claim.branch != fixedBranch {
			return "", stateError("issue #%d has conflicting remote claim ref %s; preserve claim artifacts", issue, claim.branch)
		}
		count++
		head = claim.sha
	}
	if count != 1 {
		return "", stateError("issue #%d fixed claim ref inventory is ambiguous; preserve claim artifacts", issue)
	}
	return head, nil
}

func (a app) readExactClaimResumeMetadata(root, head string, issue int, runID string) (claimResumeCommitMetadata, error) {
	commit, err := a.readCanonicalClaimCommit(root, head, issue, runID, "")
	if err != nil {
		return claimResumeCommitMetadata{}, err
	}
	if commit.lease.After(time.Now().UTC()) {
		return claimResumeCommitMetadata{}, stateError("claim #%d is active until %s; use claim renew", issue, commit.lease.Format(time.RFC3339))
	}
	return claimResumeCommitMetadata{lease: commit.lease, runID: commit.runID}, nil
}

// readCanonicalClaimCommit proves the exact bytes and graph shape of a claim
// marker.  The raw message is intentionally read without command-output
// trimming: a missing final LF, trailing bytes, source-bearing tree, or merge
// parent is a terminal artifact failure.
func (a app) readCanonicalClaimCommit(root, head string, issue int, runID, expectedParent string) (canonicalClaimCommit, error) {
	commit, err := a.readCanonicalClaimIdentity(root, head, expectedParent)
	if err != nil {
		return canonicalClaimCommit{}, err
	}
	if commit.issue != issue || commit.runID != runID {
		return canonicalClaimCommit{}, stateError("claim marker %s metadata binds issue #%d run %s, not issue #%d run %s; preserve claim artifacts", head, commit.issue, commit.runID, issue, runID)
	}
	return commit, nil
}

// readCanonicalClaimIdentity proves a generated claim marker without assuming
// which issue or run it should identify. Callers bind the returned identity to
// their phase-specific expected values.
func (a app) readCanonicalClaimIdentity(root, head, expectedParent string) (canonicalClaimCommit, error) {
	if !validExactCommitSHA(head) {
		return canonicalClaimCommit{}, stateError("claim marker head %q is not a full commit SHA; preserve claim artifacts", head)
	}
	if err := a.validateLocalAgentCommit(root, head, "claim marker "+head); err != nil {
		return canonicalClaimCommit{}, err
	}
	object, err := a.gitRaw(root, "cat-file", "commit", head)
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("read claim marker object at %s: %w", head, err)
	}
	parsed, err := parseCanonicalCommitObject(object, head)
	if err != nil {
		return canonicalClaimCommit{}, stateError("claim marker %s has non-canonical parent shape; preserve claim artifacts: %w", head, err)
	}
	if expectedParent != "" && parsed.parent != expectedParent {
		return canonicalClaimCommit{}, stateError("claim marker %s has parent %s, expected %s; preserve claim artifacts", head, parsed.parent, expectedParent)
	}
	if validateErr := a.validateLocalAgentCommit(root, parsed.parent, "claim marker parent "+parsed.parent); validateErr != nil {
		return canonicalClaimCommit{}, validateErr
	}
	treeOutput, err := a.gitRaw(root, "rev-parse", head+"^{tree}")
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("read claim marker tree at %s: %w", head, err)
	}
	tree, err := parseCanonicalSHA(treeOutput, "claim marker tree")
	if err != nil {
		return canonicalClaimCommit{}, stateError("claim marker %s has malformed tree identity; preserve claim artifacts: %w", head, err)
	}
	if parsed.tree != tree {
		return canonicalClaimCommit{}, stateError("claim marker %s tree header %s disagrees with resolved tree %s; preserve claim artifacts", head, parsed.tree, tree)
	}
	parentTreeOutput, err := a.gitRaw(root, "rev-parse", parsed.parent+"^{tree}")
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("read claim marker parent tree at %s: %w", parsed.parent, err)
	}
	parentTree, err := parseCanonicalSHA(parentTreeOutput, "claim marker parent tree")
	if err != nil {
		return canonicalClaimCommit{}, stateError("claim marker %s parent has malformed tree identity; preserve claim artifacts: %w", head, err)
	}
	if tree != parentTree {
		return canonicalClaimCommit{}, stateError("claim marker %s is source-bearing (tree %s differs from parent tree %s); preserve claim artifacts", head, tree, parentTree)
	}
	observedIssue, observedRunID, lease, parseErr := parseCanonicalClaimMessage(parsed.message)
	if parseErr != nil {
		return canonicalClaimCommit{}, stateError("claim marker %s has non-canonical metadata; preserve claim artifacts: %w", head, parseErr)
	}
	return canonicalClaimCommit{parent: parsed.parent, tree: tree, message: parsed.message, issue: observedIssue, runID: observedRunID, lease: lease}, nil
}

func parseCanonicalCommitObject(object, head string) (canonicalCommitObject, error) {
	parsed, err := parseCommitObject(object)
	if err != nil {
		return canonicalCommitObject{}, err
	}
	if !strings.HasSuffix(parsed.message, "\n") || strings.HasSuffix(parsed.message, "\n\n") || strings.Contains(parsed.message, "\r") {
		return canonicalCommitObject{}, errors.New("commit message is not an exact LF-terminated payload")
	}
	if len(parsed.parents) != 1 {
		return canonicalCommitObject{}, fmt.Errorf("want exactly one parent for %s, found %d", head, len(parsed.parents))
	}
	return canonicalCommitObject{parent: parsed.parents[0], tree: parsed.tree, message: parsed.message}, nil
}

func parseCommitObject(object string) (commitObject, error) {
	separator := strings.Index(object, "\n\n")
	if separator < 0 {
		return commitObject{}, errors.New("commit object has no header/message separator")
	}
	header := object[:separator]
	message := object[separator+2:]
	var tree string
	parents := make([]string, 0, 2)
	for _, line := range strings.Split(header, "\n") {
		switch {
		case strings.HasPrefix(line, "tree "):
			if tree != "" {
				return commitObject{}, errors.New("commit object has duplicate tree headers")
			}
			value := strings.TrimPrefix(line, "tree ")
			if !validExactCommitSHA(value) {
				return commitObject{}, errors.New("commit object has malformed tree header")
			}
			tree = value
		case strings.HasPrefix(line, "parent "):
			value := strings.TrimPrefix(line, "parent ")
			if !validExactCommitSHA(value) {
				return commitObject{}, errors.New("commit object has malformed parent header")
			}
			parents = append(parents, value)
		}
	}
	if tree == "" {
		return commitObject{}, errors.New("commit object has no tree header")
	}
	return commitObject{parents: parents, tree: tree, message: message}, nil
}

func parseCanonicalSHA(output, label string) (string, error) {
	if !strings.HasSuffix(output, "\n") || strings.Contains(output, "\r") {
		return "", fmt.Errorf("%s is not LF-terminated", label)
	}
	value := strings.TrimSuffix(output, "\n")
	if !validExactCommitSHA(value) {
		return "", fmt.Errorf("%s %q is not a full commit SHA", label, value)
	}
	return value, nil
}

func parseCanonicalClaimMessage(message string) (int, string, time.Time, error) {
	if !strings.HasSuffix(message, "\n") || strings.Contains(message, "\r") {
		return 0, "", time.Time{}, errors.New("message is not LF-terminated")
	}
	lines := strings.Split(message, "\n")
	if len(lines) != 7 || lines[1] != "" || lines[2] != "Agent-Persona: Smith" || lines[6] != "" {
		return 0, "", time.Time{}, errors.New("message bytes do not match generated claim format")
	}
	if !strings.HasPrefix(lines[0], "chore(workflow): claim issue #") ||
		!strings.HasPrefix(lines[3], "Agent-Run-ID: ") ||
		!strings.HasPrefix(lines[4], "Agent-Lease-Until: ") ||
		!strings.HasPrefix(lines[5], "Agent-Issue: ") {
		return 0, "", time.Time{}, errors.New("message fields do not match generated claim format")
	}
	issueValue := strings.TrimPrefix(lines[0], "chore(workflow): claim issue #")
	issue, err := positiveNumber(issueValue)
	if err != nil || strconv.Itoa(issue) != issueValue {
		return 0, "", time.Time{}, errors.New("message has malformed issue identity")
	}
	runID := strings.TrimPrefix(lines[3], "Agent-Run-ID: ")
	if !validRunID(runID) {
		return 0, "", time.Time{}, errors.New("message has malformed run identity")
	}
	leaseValue := strings.TrimPrefix(lines[4], "Agent-Lease-Until: ")
	lease, err := time.Parse(time.RFC3339, leaseValue)
	if err != nil || lease.Format(time.RFC3339) != leaseValue {
		return 0, "", time.Time{}, errors.New("message has malformed lease")
	}
	if strings.TrimPrefix(lines[5], "Agent-Issue: ") != issueValue {
		return 0, "", time.Time{}, errors.New("message issue trailers disagree")
	}
	return issue, runID, lease, nil
}

//nolint:gocognit // Paginated evidence, author, ordering, and path bindings form one authentication check.
func (a app) readClaimResumeEvidence(root string, issue int, handoffCommentID int64, localBranch, runID string) (claimResumeCommentEvidence, error) {
	comments, err := a.readIssueComments(root, issue)
	if err != nil {
		return claimResumeCommentEvidence{}, err
	}
	handoffIndex := -1
	handoffCount := 0
	var handoffBody string
	claimIndex := -1
	var claim claimResumeClaimComment
	claimCount := 0
	fixedBranch := claimBranch(issue)
	for index, comment := range comments {
		if comment.ID == handoffCommentID {
			handoffCount++
			if comment.User.Login != trustedActor {
				return claimResumeCommentEvidence{}, stateError("handoff comment %d is authored by %q, not trusted API actor %q; preserve evidence", handoffCommentID, comment.User.Login, trustedActor)
			}
			if bodyErr := validateTerminalClaimHandoffBody(comment.Body, issue); bodyErr != nil {
				return claimResumeCommentEvidence{}, stateError("handoff comment %d is not an exact terminal no-PR handoff: %w; preserve evidence", handoffCommentID, bodyErr)
			}
			handoffIndex = index
			handoffBody = comment.Body
		}
		if comment.User.Login != trustedActor {
			continue
		}
		parsed, parseErr := parseClaimAcquiredComment(comment.Body)
		if parseErr != nil {
			continue
		}
		if parsed.branch != fixedBranch || parsed.localBranch != localBranch {
			continue
		}
		claimCount++
		claimIndex = index
		claim = parsed
	}
	if handoffCount != 1 {
		return claimResumeCommentEvidence{}, stateError("exact terminal handoff comment %d is absent or duplicated in paginated issue history; preserve evidence", handoffCommentID)
	}
	if claimCount != 1 {
		return claimResumeCommentEvidence{}, stateError("issue #%d has %d trusted generated claim comments bound to local branch %s; preserve evidence", issue, claimCount, localBranch)
	}
	if claim.runID != runID {
		return claimResumeCommentEvidence{}, stateError("generated claim comment run %s does not match requested run %s; preserve evidence", claim.runID, runID)
	}
	if claimIndex >= handoffIndex {
		return claimResumeCommentEvidence{}, stateError("terminal handoff comment %d does not follow the exact generated claim comment; preserve evidence", handoffCommentID)
	}
	claimPath, err := absoluteCleanPath(claim.worktree)
	if err != nil {
		return claimResumeCommentEvidence{}, stateError("generated claim comment has malformed worktree path; preserve evidence")
	}
	rootPath, err := absoluteCleanPath(root)
	if err != nil {
		return claimResumeCommentEvidence{}, fmt.Errorf("resolve claim worktree root: %w", err)
	}
	if !samePath(claimPath, rootPath) {
		return claimResumeCommentEvidence{}, stateError("generated claim worktree %q does not match current worktree %q; preserve evidence", claim.worktree, root)
	}
	if !containsExactPath(handoffBody, claim.worktree) {
		return claimResumeCommentEvidence{}, stateError("terminal handoff comment %d does not name the exact claim worktree; preserve evidence", handoffCommentID)
	}
	return claimResumeCommentEvidence{claimCommentID: comments[claimIndex].ID, claimLease: claim.lease, handoffBody: handoffBody}, nil
}

func parseClaimAcquiredComment(body string) (claimResumeClaimComment, error) {
	lines := strings.Split(body, "\n")
	if len(lines) != 8 || lines[0] != "Claim acquired." || lines[1] != "" || lines[7] != "" {
		return claimResumeClaimComment{}, errors.New("generated claim comment has unexpected bytes")
	}
	branch, err := exactBacktickField(lines[2], "- Branch: ")
	if err != nil {
		return claimResumeClaimComment{}, err
	}
	localBranch, err := exactBacktickField(lines[3], "- Local branch: ")
	if err != nil {
		return claimResumeClaimComment{}, err
	}
	worktree, err := exactBacktickField(lines[4], "- Worktree: ")
	if err != nil {
		return claimResumeClaimComment{}, err
	}
	runID, err := exactBacktickField(lines[5], "- Run: ")
	if err != nil {
		return claimResumeClaimComment{}, err
	}
	leaseValue, err := exactBacktickField(lines[6], "- Lease until: ")
	if err != nil {
		return claimResumeClaimComment{}, err
	}
	if !validRunID(runID) {
		return claimResumeClaimComment{}, errors.New("generated claim comment has invalid run ID")
	}
	lease, err := time.Parse(time.RFC3339, leaseValue)
	if err != nil || lease.Format(time.RFC3339) != leaseValue {
		return claimResumeClaimComment{}, errors.New("generated claim comment has invalid lease")
	}
	return claimResumeClaimComment{branch: branch, localBranch: localBranch, worktree: worktree, runID: runID, lease: lease}, nil
}

func exactBacktickField(line, prefix string) (string, error) {
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "`") {
		return "", fmt.Errorf("generated claim comment field %q is malformed", prefix)
	}
	value := strings.TrimPrefix(line, prefix)
	if len(value) < 2 || value[0] != '`' || strings.Count(value, "`") != 2 {
		return "", fmt.Errorf("generated claim comment field %q is malformed", prefix)
	}
	value = strings.TrimSuffix(strings.TrimPrefix(value, "`"), "`")
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("generated claim comment field %q is empty", prefix)
	}
	return value, nil
}

//nolint:gocognit // Each recorded handoff identity is checked before recovery.
func validateClaimResumeHandoffBindings(body string, issue int, expectedHead, fixedBranch, localBranch, root, runID string, lease time.Time) error {
	if strings.HasPrefix(body, "# Handoff: issue #") {
		handoff, err := parseIssue305TerminalHandoff(body, issue)
		if err != nil {
			return err
		}
		rootPath, err := absoluteCleanPath(root)
		if err != nil {
			return fmt.Errorf("resolve claim worktree root: %w", err)
		}
		worktree, err := absoluteCleanPath(handoff.worktree)
		if err != nil || !samePath(worktree, rootPath) {
			return fmt.Errorf("handoff records worktree path %q, not %q", handoff.worktree, rootPath)
		}
		if handoff.branch != localBranch {
			return fmt.Errorf("handoff records branch %q, not local branch %q", handoff.branch, localBranch)
		}
		if localBranch != claimLocalBranch(issue, runID) {
			return fmt.Errorf("local branch %q does not match issue #%d run %s", localBranch, issue, runID)
		}
		for _, path := range handoffAbsolutePaths(body) {
			if path != handoff.worktree {
				return fmt.Errorf("handoff records conflicting worktree path %q", path)
			}
		}
		for _, branch := range handoffBranches(body) {
			if branch != handoff.branch {
				return fmt.Errorf("handoff records conflicting branch %q", branch)
			}
		}
		return nil
	}
	if err := validateExactIssueMentions(body, issue); err != nil {
		return err
	}
	rootPath, err := absoluteCleanPath(root)
	if err != nil {
		return fmt.Errorf("resolve claim worktree root: %w", err)
	}
	paths := handoffAbsolutePaths(body)
	if len(paths) == 0 {
		return errors.New("handoff does not record the claim worktree path")
	}
	for _, path := range paths {
		cleanPath, pathErr := absoluteCleanPath(path)
		if pathErr != nil || !samePath(cleanPath, rootPath) {
			return fmt.Errorf("handoff records worktree path %q, not %q", path, rootPath)
		}
	}
	for _, branch := range handoffBranches(body) {
		if branch != fixedBranch && branch != localBranch {
			return fmt.Errorf("handoff records conflicting branch %q", branch)
		}
	}
	for _, observed := range handoffRunIDs(body) {
		if observed != runID {
			return fmt.Errorf("handoff records run %q, not %q", observed, runID)
		}
	}
	leases := handoffLeases(body)
	for _, observed := range leases {
		if !observed.Equal(lease) {
			return fmt.Errorf("handoff records lease %s, not %s", observed.Format(time.RFC3339), lease.Format(time.RFC3339))
		}
	}
	if handoffHasLeaseMarker(body) && len(leases) == 0 {
		return errors.New("handoff records a malformed lease")
	}
	for _, observed := range handoffHeads(body) {
		if observed != expectedHead {
			return fmt.Errorf("handoff records head %q, not expected head %s", observed, expectedHead)
		}
	}
	return nil
}

//nolint:gocognit // Every issue token is checked in one pass to prevent substring spoofing.
func validateExactIssueMentions(body string, issue int) error {
	lower := strings.ToLower(body)
	count := 0
	for offset := 0; offset < len(lower); {
		relative := strings.Index(lower[offset:], "issue #")
		if relative < 0 {
			break
		}
		start := offset + relative + len("issue #")
		if start >= len(lower) || lower[start] < '0' || lower[start] > '9' {
			return errors.New("handoff contains a malformed issue identity")
		}
		end := start
		for end < len(lower) && lower[end] >= '0' && lower[end] <= '9' {
			end++
		}
		if end < len(lower) && isIssueTokenContinuation(lower[end]) {
			return errors.New("handoff contains a malformed issue identity")
		}
		offset = end
	}
	for offset := 0; offset < len(lower); {
		relative := strings.IndexByte(lower[offset:], '#')
		if relative < 0 {
			break
		}
		start := offset + relative + 1
		if start >= len(lower) || lower[start] < '0' || lower[start] > '9' {
			offset = start
			continue
		}
		end := start
		for end < len(lower) && lower[end] >= '0' && lower[end] <= '9' {
			end++
		}
		if end < len(lower) && isIssueTokenContinuation(lower[end]) {
			return errors.New("handoff contains a malformed issue identity")
		}
		observed, err := strconv.Atoi(lower[start:end])
		if err != nil || observed != issue {
			return fmt.Errorf("handoff mentions issue #%s, not issue #%d", lower[start:end], issue)
		}
		count++
		offset = end
	}
	if count == 0 {
		return fmt.Errorf("handoff does not identify issue #%d", issue)
	}
	return nil
}

func isIssueTokenContinuation(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value == '_'
}

func handoffBacktickValues(body string) []string {
	values := make([]string, 0, 4)
	for offset := 0; offset < len(body); {
		start := strings.IndexByte(body[offset:], '`')
		if start < 0 {
			break
		}
		start += offset + 1
		end := strings.IndexByte(body[start:], '`')
		if end < 0 {
			break
		}
		end += start
		values = append(values, body[start:end])
		offset = end + 1
	}
	return values
}

func handoffTokens(body string) []string {
	values := make([]string, 0, len(strings.Fields(body)))
	for _, value := range strings.Fields(body) {
		value = strings.Trim(value, "`\"'()[]{}<>,.;:")
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func handoffAbsolutePaths(body string) []string {
	paths := make([]string, 0, 2)
	appendPath := func(value string) {
		value = strings.Trim(value, "`\"'()[]{}<>,.;:")
		if strings.HasPrefix(value, "/") {
			paths = append(paths, value)
		}
	}
	for _, value := range handoffBacktickValues(body) {
		appendPath(value)
	}
	for _, value := range handoffTokens(body) {
		appendPath(value)
	}
	return paths
}

func handoffBranches(body string) []string {
	branches := make([]string, 0, 2)
	appendBranch := func(value string) {
		value = strings.Trim(value, "`\"'()[]{}<>,.;:")
		if strings.HasPrefix(value, "agent/issue-") {
			branches = append(branches, value)
		}
	}
	for _, value := range handoffBacktickValues(body) {
		appendBranch(value)
	}
	for _, value := range handoffTokens(body) {
		appendBranch(value)
	}
	return branches
}

func handoffRunIDs(body string) []string {
	runs := make([]string, 0, 1)
	appendRun := func(value string) {
		value = strings.Trim(value, "`\"'()[]{}<>,.;:")
		if strings.HasPrefix(value, "run-") {
			runs = append(runs, value)
		}
	}
	for _, value := range handoffBacktickValues(body) {
		appendRun(value)
	}
	for _, value := range handoffTokens(body) {
		if strings.Contains(value, "/") {
			continue
		}
		appendRun(value)
	}
	return runs
}

func handoffLeases(body string) []time.Time {
	leases := make([]time.Time, 0, 1)
	appendLease := func(value string) {
		value = strings.Trim(value, "`\"'()[]{}<>,.;:")
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil || parsed.Format(time.RFC3339) != value {
			return
		}
		leases = append(leases, parsed)
	}
	for _, value := range handoffBacktickValues(body) {
		appendLease(value)
	}
	for _, value := range handoffTokens(body) {
		appendLease(value)
	}
	return leases
}

func handoffHasLeaseMarker(body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range []string{"lease until", "agent-lease-until", "valid through", "expires"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func handoffHeads(body string) []string {
	heads := make([]string, 0, 1)
	appendHead := func(value string) {
		value = strings.Trim(value, "`\"'()[]{}<>,.;:")
		if len(value) < 7 || !isHexString(value) {
			return
		}
		heads = append(heads, value)
	}
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(rawLine, "- "))
		label, found := handoffHeadLabel(line)
		if !found {
			continue
		}
		remainder := strings.TrimSpace(line[len(label):])
		for _, value := range handoffBacktickValues(remainder) {
			appendHead(value)
		}
		for _, value := range handoffTokens(remainder) {
			if strings.Contains(value, "-") || strings.Contains(value, "/") {
				continue
			}
			appendHead(value)
		}
	}
	return heads
}

func handoffHeadLabel(line string) (string, bool) {
	for _, label := range []string{"expected head", "claim head", "fixed head", "expected sha", "claim sha", "expected commit", "commit sha", "commit-sha", "head", "sha", "commit"} {
		if len(line) <= len(label) || !strings.EqualFold(line[:len(label)], label) {
			continue
		}
		separator := line[len(label)]
		if separator == ':' || separator == ' ' || separator == '\t' {
			return line[:len(label)], true
		}
	}
	return "", false
}

func isHexString(value string) bool {
	for _, current := range value {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') && (current < 'A' || current > 'F') {
			return false
		}
	}
	return value != ""
}

func containsExactPath(body, path string) bool {
	for _, observed := range handoffAbsolutePaths(body) {
		if observed == path {
			return true
		}
	}
	return false
}

type issue305TerminalHandoff struct {
	worktree string
	branch   string
}

// parseIssue305TerminalHandoff accepts the one authenticated legacy handoff
// grammar. Its evidence is intentionally section- and field-shaped: arbitrary
// prose must not be allowed to manufacture a no-PR recovery proof.
//
//nolint:gocognit,funlen // The approved legacy grammar is intentionally explicit and fail-closed.
func parseIssue305TerminalHandoff(body string, issue int) (issue305TerminalHandoff, error) {
	if issue != 305 {
		return issue305TerminalHandoff{}, fmt.Errorf("titled terminal handoff grammar is approved only for issue #305, not issue #%d", issue)
	}
	if !utf8.ValidString(body) || body == "" || !strings.HasSuffix(body, "\n") || strings.HasSuffix(body, "\n\n") {
		return issue305TerminalHandoff{}, errors.New("body is not an exact UTF-8 LF-terminated comment")
	}
	lines := strings.Split(body, "\n")
	if len(lines) < 2 || lines[0] != "# Handoff: issue #305" {
		return issue305TerminalHandoff{}, errors.New("body has a malformed issue #305 handoff title")
	}
	for _, line := range lines {
		if strings.TrimRight(line, " \t") != line {
			return issue305TerminalHandoff{}, errors.New("body contains trailing whitespace")
		}
		for _, value := range line {
			if value == '\r' || (value < 0x20 && value != '\t') {
				return issue305TerminalHandoff{}, errors.New("body contains control bytes")
			}
		}
	}
	wantHeadings := []string{"## Block", "## Decisions and evidence", "## Risks", "## Next actions"}
	headings := make([]string, 0, len(wantHeadings))
	headingIndexes := make([]int, 0, len(wantHeadings))
	for index, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		headings = append(headings, line)
		headingIndexes = append(headingIndexes, index)
	}
	if len(headings) != len(wantHeadings) {
		return issue305TerminalHandoff{}, errors.New("body headings do not match the approved issue #305 handoff")
	}
	for index, heading := range wantHeadings {
		if headings[index] != heading {
			return issue305TerminalHandoff{}, errors.New("body headings do not match the approved issue #305 handoff")
		}
		sectionEnd := len(lines) - 1
		if index+1 < len(headingIndexes) {
			sectionEnd = headingIndexes[index+1]
		}
		nonEmpty := false
		for _, sectionLine := range lines[headingIndexes[index]+1 : sectionEnd] {
			if strings.TrimSpace(sectionLine) != "" {
				nonEmpty = true
				break
			}
		}
		if !nonEmpty {
			return issue305TerminalHandoff{}, fmt.Errorf("issue #305 handoff section %q is empty", heading)
		}
	}
	if err := validateExactIssueMentions(body, issue); err != nil {
		return issue305TerminalHandoff{}, err
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		for _, label := range []string{"head", "sha", "commit", "lease", "run"} {
			if strings.HasPrefix(strings.ToLower(trimmed), label+":") {
				return issue305TerminalHandoff{}, fmt.Errorf("body contains an unapproved %s binding field", label)
			}
		}
	}
	for _, phrase := range []string{"expected head", "claim head", "fixed head", "expected sha", "claim sha", "expected commit", "commit sha", "head is", "sha is", "commit is", "lease until", "lease is", "agent-run-id", "run id", "run is"} {
		if containsHandoffWords(body, phrase) {
			return issue305TerminalHandoff{}, fmt.Errorf("body contains an unapproved %s binding", phrase)
		}
	}
	if len(handoffLeases(body)) != 0 || len(handoffRunIDs(body)) != 0 {
		return issue305TerminalHandoff{}, errors.New("body contains an unapproved lease or run binding")
	}
	for _, token := range handoffTokens(strings.ToLower(body)) {
		if len(token) == 40 && isHexString(token) {
			return issue305TerminalHandoff{}, errors.New("body contains an unapproved full commit binding")
		}
	}
	if !containsHandoffWords(body, "claimed packet could not reach implementation") ||
		!containsHandoffWords(body, "worktree remained clean throughout") ||
		!containsHandoffWords(body, "no implementation changes") {
		return issue305TerminalHandoff{}, errors.New("body lacks the approved issue #305 blocker and no-source evidence")
	}
	if !containsHandoffWords(body, "with no diff commit push pr check evidence challenge or examiner receipt") ||
		!containsHandoffWords(body, "no pr or review lifecycle has started") {
		return issue305TerminalHandoff{}, errors.New("body lacks the approved issue #305 no-PR evidence")
	}
	if err := rejectUnapprovedIssue305PRTokens(body); err != nil {
		return issue305TerminalHandoff{}, err
	}
	decisionStart := headingIndexes[1] + 1
	decisionEnd := headingIndexes[2]
	var worktree string
	var branch string
	for index := decisionStart; index < decisionEnd; index++ {
		line := strings.TrimSpace(lines[index])
		if line == "- The preserved issue worktree is" {
			if worktree != "" || index+1 >= decisionEnd {
				return issue305TerminalHandoff{}, errors.New("body has an ambiguous preserved issue worktree field")
			}
			value := strings.TrimSpace(lines[index+1])
			if !strings.HasPrefix(value, "`") || !strings.HasSuffix(value, "`.") || strings.Count(value, "`") != 2 {
				return issue305TerminalHandoff{}, errors.New("body has a malformed preserved issue worktree field")
			}
			worktree = value[1 : len(value)-2]
			if !strings.HasPrefix(worktree, "/") {
				return issue305TerminalHandoff{}, errors.New("body preserved issue worktree is not absolute")
			}
			index++
			continue
		}
		if strings.HasPrefix(line, "Its branch is `") {
			if branch != "" {
				return issue305TerminalHandoff{}, errors.New("body has an ambiguous preserved issue branch field")
			}
			const prefix = "Its branch is `"
			value := strings.TrimPrefix(line, prefix)
			closeIndex := strings.IndexByte(value, '`')
			const suffix = ", with no diff, commit,"
			if closeIndex <= 0 || !strings.HasSuffix(value, suffix) || closeIndex+1 > len(value)-len(suffix) {
				return issue305TerminalHandoff{}, errors.New("body has a malformed preserved issue branch field")
			}
			if value[closeIndex+1:] != suffix {
				return issue305TerminalHandoff{}, errors.New("body has a malformed preserved issue branch field")
			}
			branch = value[:closeIndex]
			if !strings.HasPrefix(branch, "agent/issue-305-run-") {
				return issue305TerminalHandoff{}, errors.New("body preserved issue branch is not a run-local branch")
			}
		}
	}
	if worktree == "" || branch == "" {
		return issue305TerminalHandoff{}, errors.New("body lacks the preserved issue worktree and branch fields")
	}
	for _, path := range handoffAbsolutePaths(body) {
		if path != worktree {
			return issue305TerminalHandoff{}, fmt.Errorf("body contains an unapproved worktree path %q", path)
		}
	}
	for _, observed := range handoffBranches(body) {
		if observed != branch {
			return issue305TerminalHandoff{}, fmt.Errorf("body contains an unapproved branch %q", observed)
		}
	}
	return issue305TerminalHandoff{worktree: worktree, branch: branch}, nil
}

func rejectUnapprovedIssue305PRTokens(body string) error {
	tokens := handoffTokens(strings.ToLower(body))
	for index, token := range tokens {
		if strings.Contains(token, "pull-request") || strings.Contains(token, "pull_request") || token == "prs" {
			return errors.New("body contains an unapproved pull-request assertion")
		}
		if token == "pull" && index+1 < len(tokens) && tokens[index+1] == "request" {
			return errors.New("body contains an unapproved pull-request assertion")
		}
		if token != "pr" {
			continue
		}
		firstAllowed := index >= 4 && index+1 < len(tokens) &&
			tokens[index-4] == "no" && tokens[index-3] == "diff" &&
			tokens[index-2] == "commit" && tokens[index-1] == "push" && tokens[index+1] == "check"
		secondAllowed := index >= 1 && index+2 < len(tokens) &&
			tokens[index-1] == "no" && tokens[index+1] == "or" && tokens[index+2] == "review"
		if !firstAllowed && !secondAllowed {
			return errors.New("body contains an unapproved PR assertion")
		}
	}
	return nil
}

//nolint:funlen,gocognit // Exact historical handoff grammar is deliberately fail-closed.
func validateTerminalClaimHandoffBody(body string, issue int) error {
	if !utf8.ValidString(body) || body == "" || !strings.HasSuffix(body, "\n") || strings.HasSuffix(body, "\n\n") {
		return errors.New("body is not an exact UTF-8 LF-terminated comment")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimRight(line, " \t") != line {
			return errors.New("body contains trailing whitespace")
		}
		for _, value := range line {
			if value == '\r' || (value < 0x20 && value != '\t') {
				return errors.New("body contains control bytes")
			}
		}
	}
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		return errors.New("body has no terminal handoff sections")
	}
	hasTitle := strings.HasPrefix(lines[0], "# Handoff: issue #")
	if hasTitle {
		titleIssue, titleErr := strconv.Atoi(strings.TrimPrefix(lines[0], "# Handoff: issue #"))
		if titleErr != nil || titleIssue < 1 || strconv.Itoa(titleIssue) != strings.TrimPrefix(lines[0], "# Handoff: issue #") {
			return errors.New("body has a malformed handoff issue title")
		}
		if titleIssue != issue {
			return fmt.Errorf("body handoff title identifies issue #%d, not issue #%d", titleIssue, issue)
		}
	}
	if !hasTitle && (!strings.HasPrefix(body, "## Blocker\n\n") || !strings.Contains(body, "\n## Evidence\n\n")) {
		return errors.New("body must contain the exact Blocker and Evidence sections")
	}
	headings := make([]string, 0, 4)
	for _, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		headings = append(headings, line)
	}
	wantHeadings := [][]string{{"## Blocker", "## Evidence", "## Decisions and risks", "## Next action"}, {"## Blocker", "## Evidence", "## Risk and next action"}}
	if hasTitle {
		wantHeadings = [][]string{{"## Block", "## Decisions and evidence", "## Risks", "## Next actions"}}
	}
	headingMatch := false
	for _, want := range wantHeadings {
		if len(headings) != len(want) {
			continue
		}
		match := true
		for index := range want {
			if headings[index] != want[index] {
				match = false
				break
			}
		}
		if match {
			headingMatch = true
			break
		}
	}
	if !headingMatch {
		return errors.New("body headings do not match a terminal blocker handoff")
	}
	if hasTitle {
		_, err := parseIssue305TerminalHandoff(body, issue)
		return err
	}
	if err := validateExactIssueMentions(body, issue); err != nil {
		return err
	}
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "claim") {
		return errors.New("body lacks claim-state evidence")
	}
	folded := strings.Join(strings.Fields(lower), " ")
	cleanEvidence := containsHandoffWords(folded, "worktree was clean") || containsHandoffWords(folded, "clean worktree") ||
		containsHandoffWords(folded, "worktree clean") || containsHandoffWords(folded, "only the generated claim commit") ||
		containsHandoffWords(folded, "only the generated workflow claim commit") ||
		containsHandoffWords(folded, "worktree preserved") || containsHandoffWords(folded, "worktree remained clean") ||
		containsHandoffWords(folded, "clean throughout")
	noSourceEvidence := containsHandoffWords(folded, "no implementation") || containsHandoffWords(folded, "no source")
	if !cleanEvidence || !noSourceEvidence {
		return errors.New("body lacks preserved worktree/no-source evidence")
	}
	if containsHandoffWords(folded, "unclean worktree") || containsHandoffWords(folded, "worktree was unclean") ||
		containsHandoffWords(folded, "dirty worktree") || containsHandoffWords(folded, "worktree was dirty") ||
		containsHandoffWords(folded, "worktree is dirty") || containsHandoffWords(folded, "worktree is unclean") {
		return errors.New("body contradicts the clean worktree evidence")
	}
	noPR := false
	for _, paragraph := range strings.Split(body, "\n\n") {
		foldedParagraph := strings.Join(strings.Fields(strings.ToLower(paragraph)), " ")
		if containsHandoffNoPR(foldedParagraph) {
			noPR = true
		}
		if (containsHandoffWords(foldedParagraph, "no implementation") || containsHandoffWords(foldedParagraph, "no source")) && containsHandoffPR(foldedParagraph) {
			noPR = true
		}
		if noPR {
			break
		}
	}
	if !noPR {
		return errors.New("body lacks an explicit no-PR statement")
	}
	for _, contradiction := range []string{"open pr", "a pr was", "pr was opened", "pull request was opened", "created a pr", "created a pull request", "pr exists", "pull request exists", "there is a pr", "there is an open pull request"} {
		if containsHandoffWords(folded, contradiction) {
			return errors.New("body contradicts the no-PR statement")
		}
	}
	return nil
}

func containsHandoffPR(text string) bool {
	if containsHandoffWords(text, "pull request") {
		return true
	}
	return containsHandoffWords(text, "pr")
}

func containsHandoffNoPR(text string) bool {
	return containsHandoffWords(text, "no pr") || containsHandoffWords(text, "no pull request")
}

func containsHandoffWords(text, phrase string) bool {
	observed := handoffTokens(strings.ToLower(text))
	want := strings.Fields(strings.ToLower(phrase))
	if len(want) == 0 || len(observed) < len(want) {
		return false
	}
	for start := 0; start+len(want) <= len(observed); start++ {
		match := true
		for index := range want {
			if observed[start+index] != want[index] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (a app) validateNoOpenClaimResumePR(root, fixedBranch string, issue int) error {
	output, err := a.command(root, "gh", "pr", "list", "--repo", repositoryKey, "--head", fixedBranch, "--state", "open", "--json", "number")
	if err != nil {
		return retryableOperation("claim resume open-PR read", fmt.Errorf("check open PRs for issue #%d: %w", issue, err))
	}
	var prs []openPullRequestNumber
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" {
		return stateError("decode open PRs for issue #%d: empty or null response; preserve claim artifacts", issue)
	}
	if err := json.Unmarshal([]byte(trimmed), &prs); err != nil {
		return stateError("decode open PRs for issue #%d: %w; preserve claim artifacts", issue, err)
	}
	if len(prs) != 0 {
		return stateError("issue #%d fixed claim branch %s has %d open PR(s); no-PR recovery is blocked and artifacts are preserved", issue, fixedBranch, len(prs))
	}
	return nil
}

func canonicalClaimResumeProjectItem(list projectList, issue int) (projectItem, error) {
	matches := make([]projectItem, 0, 1)
	for _, item := range list.Items {
		if item.Content.Number != issue || item.Content.Repository != repositoryKey {
			continue
		}
		if item.Content.Type != "Issue" || strings.TrimSpace(item.ID) == "" {
			return projectItem{}, stateError("issue #%d has a malformed or non-Issue canonical Project item; preserve external state", issue)
		}
		matches = append(matches, item)
	}
	if len(matches) != 1 {
		return projectItem{}, stateError("issue #%d has %d canonical Project items; expected exactly one and preserved external state", issue, len(matches))
	}
	return matches[0], nil
}

func (a app) claimResumeRenewalPlan(root, expectedHead, localHead, remoteHead string, issue int, runID string) (claimResumeRenewalPlan, error) {
	if localHead == expectedHead && remoteHead == expectedHead {
		return claimResumeNoRenewal{}, nil
	}
	renewal := ""
	if localHead != expectedHead {
		if err := a.validateExistingResumeCommit(root, localHead, expectedHead, issue, runID); err != nil {
			return nil, retryableOperationIfRecoverable("claim resume local renewal proof",
				fmt.Errorf("local claim head %s is moved or not the unique renewal child; preserve it: %w", localHead, err))
		}
		renewal = localHead
	}
	if remoteHead != expectedHead {
		if err := a.validateExistingResumeCommit(root, remoteHead, expectedHead, issue, runID); err != nil {
			return nil, retryableOperationIfRecoverable("claim resume remote renewal proof",
				fmt.Errorf("remote fixed claim head %s is moved or not the unique renewal child; preserve it: %w", remoteHead, err))
		}
		if renewal != "" && renewal != remoteHead {
			return nil, stateError("local and remote renewal children disagree (%s versus %s); preserve both artifacts", renewal, remoteHead)
		}
		renewal = remoteHead
	}
	if !validExactCommitSHA(renewal) {
		return nil, stateError("claim resume renewal head %q is malformed; preserve claim artifacts", renewal)
	}
	return claimResumeExistingRenewal{head: renewal}, nil
}

func claimResumeProtectedHeads(expected string, plan claimResumeRenewalPlan) []string {
	heads := []string{expected}
	if existing, ok := plan.(claimResumeExistingRenewal); ok {
		heads = append(heads, existing.head)
	}
	return heads
}

//nolint:gocognit // Ref parsing and deterministic partitioning are one artifact check.
func (a app) claimResumeAgentRefs(root, namespace string, source claimRefSource) (claimResumeAgentRefs, error) {
	output, err := a.command(root, "git", "for-each-ref", "--format=%(refname:short) %(objectname)", namespace)
	if err != nil {
		return claimResumeAgentRefs{}, retryableOperation("list "+sourceName(source)+" claim refs", fmt.Errorf("list %s claim refs: %w", sourceName(source), err))
	}
	refs := claimResumeAgentRefs{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return claimResumeAgentRefs{}, stateError("%s claim ref listing contains malformed entry %q; preserve artifacts", sourceName(source), line)
		}
		if err := a.validateLocalAgentCommit(root, fields[1], sourceName(source)+" claim ref "+fields[0]); err != nil {
			return claimResumeAgentRefs{}, err
		}
		branch := fields[0]
		if source == claimRefTracking {
			branch = strings.TrimPrefix(branch, "origin/")
		}
		kind, number, runID := classifyAgentRef(branch)
		switch kind {
		case agentRefClaim:
			refs.claims = append(refs.claims, remoteClaim{branch: branch, number: number, sha: fields[1], source: source})
		case agentRefRunLocal:
			refs.runLocals = append(refs.runLocals, runLocalRef{branch: branch, number: number, runID: runID, sha: fields[1], source: source})
		case agentRefMalformed:
			refs.malformed = append(refs.malformed, agentRef{branch: branch, sha: fields[1]})
		case agentRefArchive, agentRefUnrelated:
			continue
		}
	}
	sort.Slice(refs.claims, func(left, right int) bool {
		if refs.claims[left].branch != refs.claims[right].branch {
			return refs.claims[left].branch < refs.claims[right].branch
		}
		return refs.claims[left].sha < refs.claims[right].sha
	})
	sort.Slice(refs.runLocals, func(left, right int) bool {
		if refs.runLocals[left].branch != refs.runLocals[right].branch {
			return refs.runLocals[left].branch < refs.runLocals[right].branch
		}
		return refs.runLocals[left].sha < refs.runLocals[right].sha
	})
	sort.Slice(refs.malformed, func(left, right int) bool {
		if refs.malformed[left].branch != refs.malformed[right].branch {
			return refs.malformed[left].branch < refs.malformed[right].branch
		}
		return refs.malformed[left].sha < refs.malformed[right].sha
	})
	return refs, nil
}

func sourceName(source claimRefSource) string {
	switch source {
	case claimRefRemote:
		return "remote"
	case claimRefLocal:
		return "local"
	case claimRefTracking:
		return "remote-tracking"
	default:
		return "remote"
	}
}

//nolint:gocognit,funlen // Ref sources must be checked as one immutable proof.
func (a app) validateClaimResumeRefs(root string, remoteInventory agentRefInventory, issue int, fixedBranch, localBranch, runID, expectedHead, localHead, remoteHead string) error {
	if len(remoteInventory.malformed) != 0 {
		return stateError("remote agent ref %s is malformed; preserve all claim artifacts", remoteInventory.malformed[0].branch)
	}
	remoteRuns := 0
	for _, ref := range remoteInventory.runLocals {
		if ref.number != issue {
			continue
		}
		if ref.branch != localBranch || (ref.sha != localHead && ref.sha != remoteHead) {
			return stateError("issue #%d has conflicting remote run-local ref %s; preserve it before recovery", issue, ref.branch)
		}
		remoteRuns++
	}
	if remoteRuns > 1 {
		return stateError("issue #%d has duplicate remote run-local refs for %s; preserve artifacts", issue, localBranch)
	}
	local, err := a.claimResumeAgentRefs(root, "refs/heads/agent/issue-*", claimRefLocal)
	if err != nil {
		return err
	}
	tracking, err := a.claimResumeAgentRefs(root, "refs/remotes/origin/agent/issue-*", claimRefTracking)
	if err != nil {
		return err
	}
	if len(local.malformed) != 0 || len(tracking.malformed) != 0 {
		refs := local.malformed
		if len(refs) == 0 {
			refs = tracking.malformed
		}
		source := claimRefLocal
		if len(local.malformed) == 0 {
			source = claimRefTracking
		}
		return stateError("%s agent ref %s is malformed; preserve all claim artifacts", sourceName(source), refs[0].branch)
	}
	localClaims := 0
	for _, ref := range local.claims {
		if ref.number == issue {
			localClaims++
		}
	}
	if localClaims != 0 {
		return stateError("issue #%d has a local fixed claim ref; preserve the ambiguous artifact before recovery", issue)
	}
	localRuns := 0
	for _, ref := range local.runLocals {
		if ref.number != issue {
			continue
		}
		if ref.branch != localBranch || ref.sha != localHead {
			return stateError("issue #%d has a moved or conflicting local run-local ref %s; preserve it before recovery", issue, ref.branch)
		}
		localRuns++
	}
	if localRuns != 1 {
		return stateError("issue #%d has %d local run-local refs for %s; expected one unique ref", issue, localRuns, localBranch)
	}
	trackingClaims := 0
	for _, ref := range tracking.claims {
		if ref.number != issue {
			continue
		}
		if ref.branch != fixedBranch || ref.sha != remoteHead {
			return stateError("issue #%d remote-tracking fixed claim ref moved; preserve it before recovery", issue)
		}
		trackingClaims++
	}
	if trackingClaims > 1 {
		return stateError("issue #%d has duplicate remote-tracking fixed claim refs; preserve artifacts", issue)
	}
	for _, ref := range tracking.runLocals {
		if ref.number != issue {
			continue
		}
		if ref.branch != localBranch || ref.sha != localHead {
			return stateError("issue #%d has a moved remote-tracking run-local ref %s; preserve it before recovery", issue, ref.branch)
		}
	}
	if remoteHead == expectedHead && localHead != expectedHead {
		if err := a.validateExistingResumeCommit(root, localHead, expectedHead, issue, runID); err != nil {
			return err
		}
	}
	return nil
}

func validExactCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, current := range value {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') && (current < 'A' || current > 'F') {
			return false
		}
	}
	return true
}

func sameClaimResumeProof(before, after claimResumeProof) error {
	beforePreflight, afterPreflight := before.preflight, after.preflight
	if beforePreflight.root != afterPreflight.root || beforePreflight.localBranch != afterPreflight.localBranch ||
		beforePreflight.fixedBranch != afterPreflight.fixedBranch || beforePreflight.expectedHead != afterPreflight.expectedHead ||
		beforePreflight.localHead != afterPreflight.localHead || beforePreflight.remoteHead != afterPreflight.remoteHead ||
		beforePreflight.runID != afterPreflight.runID || beforePreflight.issue != afterPreflight.issue ||
		beforePreflight.handoffCommentID != afterPreflight.handoffCommentID || beforePreflight.claimCommentID != afterPreflight.claimCommentID ||
		beforePreflight.handoffBody != afterPreflight.handoffBody || !beforePreflight.claimLease.Equal(afterPreflight.claimLease) ||
		beforePreflight.projectItemID != afterPreflight.projectItemID || beforePreflight.projectStatus != afterPreflight.projectStatus ||
		beforePreflight.needsHuman != afterPreflight.needsHuman || !sameClaimResumeRenewalPlan(before.renewal, after.renewal) {
		return stateError("bound issue, handoff, ref, claim, Project, or worktree proof no longer matches")
	}
	return nil
}

func sameClaimResumeRenewalPlan(before, after claimResumeRenewalPlan) bool {
	switch beforePlan := before.(type) {
	case claimResumeNoRenewal:
		_, ok := after.(claimResumeNoRenewal)
		return ok
	case claimResumeExistingRenewal:
		afterPlan, ok := after.(claimResumeExistingRenewal)
		return ok && beforePlan.head == afterPlan.head
	default:
		return false
	}
}

func (a app) applyClaimResume(proof claimResumeProof) error {
	// This is the fresh read-only seal. No ref or GitHub mutation precedes it.
	fresh, err := a.readClaimResumeProof(proof.preflight.issue, proof.preflight.expectedHead, proof.preflight.runID, proof.preflight.handoffCommentID)
	if err != nil {
		err = retryableOperationIfRecoverable("claim resume fresh proof", err)
		return claimResumeProofFailure(proof, "claim resume proof changed before renewal; no mutation performed", err)
	}
	if proofErr := sameClaimResumeProof(proof, fresh); proofErr != nil {
		return stateError("issue #%d claim resume proof changed before renewal; no mutation performed: %w", proof.preflight.issue, proofErr)
	}
	var renewalProof claimResumeRenewalProof
	switch plan := fresh.renewal.(type) {
	case claimResumeNoRenewal:
		renewalProof, err = a.createClaimResumeRenewal(fresh)
	case claimResumeExistingRenewal:
		renewalProof = claimResumeRenewalProof(plan)
	default:
		err = stateError("issue #%d has an invalid renewal proof state; preserve claim artifacts", fresh.preflight.issue)
	}
	if err != nil {
		return err
	}
	localRenewal, err := a.adoptClaimResumeLocalRenewal(fresh, renewalProof)
	if err != nil {
		return err
	}
	renewal, err := a.pushClaimResumeRenewal(fresh, localRenewal)
	if err != nil {
		return err
	}
	if err := a.verifyClaimResumeRenewal(fresh, renewal); err != nil {
		verificationErr := retryableOperationIfRecoverable("claim resume renewal verification", err)
		return claimResumeProofFailure(fresh, "claim renewal needs reconciliation", fmt.Errorf("%w. "+claimResumeRecoveryTemplate,
			verificationErr, fresh.preflight.issue, fresh.preflight.expectedHead, fresh.preflight.runID, fresh.preflight.handoffCommentID))
	}
	if err := a.reconcileClaimResumeIssue(fresh, renewal); err != nil {
		return err
	}
	return writeLine(a.stdout, "issue #%d claim resumed; claim verified, needs-human removed, Project Picked", proof.preflight.issue)
}

func claimResumeProofFailure(proof claimResumeProof, message string, err error) error {
	issue := proof.preflight.issue
	if err == nil {
		return stateError("issue #%d %s", issue, message)
	}
	if operationDispositionOf(err) == operationDispositionRetryable {
		return fmt.Errorf("issue #%d %s: %w", issue, message, err)
	}
	return stateError("issue #%d %s: %w", issue, message, err)
}

func (a app) createClaimResumeRenewal(proof claimResumeProof) (claimResumeRenewalProof, error) {
	if _, ok := proof.renewal.(claimResumeNoRenewal); !ok {
		return claimResumeRenewalProof{}, stateError("issue #%d already has a renewal proof; preserve its canonical child", proof.preflight.issue)
	}
	commit, _, _, err := a.newClaimCommitWithRunID(proof.preflight.root, proof.preflight.issue, proof.preflight.expectedHead, proof.preflight.runID)
	if err != nil {
		return claimResumeRenewalProof{}, claimResumeProofFailure(proof, "could not create the renewal commit; retry", claimResumeRetry(proof, "renewal commit", err))
	}
	if !validExactCommitSHA(commit) {
		return claimResumeRenewalProof{}, stateError("issue #%d renewal commit returned malformed head %q; preserve claim artifacts", proof.preflight.issue, commit)
	}
	if _, validateErr := a.readCanonicalClaimCommit(proof.preflight.root, commit, proof.preflight.issue, proof.preflight.runID, proof.preflight.expectedHead); validateErr != nil {
		return claimResumeRenewalProof{}, retryableOperationIfRecoverable("claim resume renewal commit validation", validateErr)
	}
	return claimResumeRenewalProof{head: commit}, nil
}

func (a app) adoptClaimResumeLocalRenewal(proof claimResumeProof, renewal claimResumeRenewalProof) (claimResumeLocalRenewal, error) {
	if proof.preflight.localHead == renewal.head {
		return claimResumeLocalRenewal(renewal), nil
	}
	if proof.preflight.localHead != proof.preflight.expectedHead {
		return claimResumeLocalRenewal{}, stateError("local claim head moved before renewal adoption: expected %s or child %s, found %s; preserve artifacts", proof.preflight.expectedHead, renewal.head, proof.preflight.localHead)
	}
	if _, err := a.command(proof.preflight.root, "git", "update-ref", "refs/heads/"+proof.preflight.localBranch, renewal.head, proof.preflight.expectedHead); err != nil {
		observed, readErr := a.readClaimResumeLocalHead(proof.preflight.root, proof.preflight.localBranch)
		if readErr == nil && observed == renewal.head {
			if validateErr := a.validateExistingResumeCommit(proof.preflight.root, observed, proof.preflight.expectedHead, proof.preflight.issue, proof.preflight.runID); validateErr == nil {
				return claimResumeLocalRenewal(renewal), nil
			}
		}
		if readErr != nil && operationDispositionOf(readErr) == operationDispositionTerminal {
			return claimResumeLocalRenewal{}, stateError("issue #%d local renewal CAS response was ambiguous; preserve artifacts: %w", proof.preflight.issue, readErr)
		}
		return claimResumeLocalRenewal{}, claimResumeRetry(proof, "local renewal CAS", errors.Join(err, readErr))
	}
	return claimResumeLocalRenewal(renewal), nil
}

func (a app) readClaimResumeLocalHead(root, branch string) (string, error) {
	head, err := a.command(root, "git", "rev-parse", "refs/heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("read local claim ref %s: %w", branch, err)
	}
	if !validExactCommitSHA(head) {
		return "", stateError("local claim ref %s returned malformed head %q", branch, head)
	}
	if err := a.validateLocalAgentCommit(root, head, "local claim ref "+branch); err != nil {
		return "", err
	}
	return head, nil
}

func (a app) pushClaimResumeRenewal(proof claimResumeProof, renewal claimResumeLocalRenewal) (claimResumeRenewalResult, error) {
	if proof.preflight.remoteHead == renewal.head {
		return claimResumeRenewalResult(renewal), nil
	}
	if proof.preflight.remoteHead != proof.preflight.expectedHead {
		return claimResumeRenewalResult{}, stateError("remote fixed claim branch moved before renewal push: expected %s, found %s; preserve artifacts", proof.preflight.expectedHead, proof.preflight.remoteHead)
	}
	lease := "--force-with-lease=refs/heads/" + proof.preflight.fixedBranch + ":" + proof.preflight.expectedHead
	refspec := renewal.head + ":refs/heads/" + proof.preflight.fixedBranch
	pushOutput, pushErr := a.command(proof.preflight.root, "git", "push", lease, "origin", refspec)
	remote, readErr := a.remoteClaimHead(proof.preflight.root, proof.preflight.fixedBranch)
	if readErr == nil && remote == renewal.head {
		return claimResumeRenewalResult(renewal), nil
	}
	if readErr != nil {
		failure := retryableOperationIfRecoverable("claim resume renewal push", errors.Join(pushErr, readErr))
		return claimResumeRenewalResult{}, claimResumeProofFailure(proof, "renewal push response was ambiguous; reread failed", failure)
	}
	if remote != proof.preflight.expectedHead {
		return claimResumeRenewalResult{}, stateError("remote fixed claim branch moved during renewal push: expected %s or valid child %s, found %s; preserve artifacts", proof.preflight.expectedHead, renewal.head, remote)
	}
	if pushErr == nil {
		pushErr = errors.New("push returned without advancing the fixed claim branch")
	}
	failure := claimResumeRetry(proof, "renewal push", pushErr)
	return claimResumeRenewalResult{}, claimResumeProofFailure(proof, fmt.Sprintf("renewal push response was ambiguous; remote remains at expected head; %v", pushOutput), failure)
}

func (a app) verifyClaimResumeRenewal(proof claimResumeProof, renewal claimResumeRenewalResult) error {
	local, err := a.readClaimResumeLocalHead(proof.preflight.root, proof.preflight.localBranch)
	if err != nil {
		return err
	}
	if local != renewal.head {
		return stateError("local renewal ref moved: expected %s, found %s; preserve artifacts", renewal.head, local)
	}
	currentHead, err := a.command(proof.preflight.root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read renewed worktree head: %w", err)
	}
	if currentHead != renewal.head {
		return stateError("renewed worktree head moved: expected %s, found %s; preserve artifacts", renewal.head, currentHead)
	}
	status, err := a.command(proof.preflight.root, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return fmt.Errorf("inspect renewed claim worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return stateError("claim worktree became dirty during renewal; preserve its changes")
	}
	if metadataErr := a.validateExistingResumeCommit(proof.preflight.root, renewal.head, proof.preflight.expectedHead, proof.preflight.issue, proof.preflight.runID); metadataErr != nil {
		return metadataErr
	}
	layout, err := a.repositoryLayout(proof.preflight.root)
	if err != nil {
		return err
	}
	if worktreeErr := validateResumeWorktreeHeads(layout, proof.preflight.root, proof.preflight.localBranch, proof.preflight.issue, renewal.head,
		claimResumeProtectedHeads(proof.preflight.expectedHead, claimResumeExistingRenewal(renewal))); worktreeErr != nil {
		return worktreeErr
	}
	inventory, err := a.strictRemoteAgentRefInventory(proof.preflight.root)
	if err != nil {
		return err
	}
	remote, err := claimResumeFixedHead(inventory, proof.preflight.issue, proof.preflight.fixedBranch)
	if err != nil {
		return err
	}
	if remote != renewal.head {
		return stateError("remote fixed claim branch is not the verified renewal child: expected %s, found %s", renewal.head, remote)
	}
	if err := a.validateNoOpenClaimResumePR(proof.preflight.root, proof.preflight.fixedBranch, proof.preflight.issue); err != nil {
		return err
	}
	return a.validateClaimResumeRefs(proof.preflight.root, inventory, proof.preflight.issue, proof.preflight.fixedBranch, proof.preflight.localBranch, proof.preflight.runID,
		proof.preflight.expectedHead, renewal.head, renewal.head)
}

type claimResumeReconciliationTarget struct {
	status issueStatus
	item   projectItem
}

type claimResumeReconciliationPhase uint8

const (
	claimResumeLabelPhase claimResumeReconciliationPhase = iota
	claimResumeProjectPhase
)

// claimResumeReconciliationState keeps the immutable recovery proof and the
// verified renewal result together with the current ordered mutation phase.
type claimResumeReconciliationState struct {
	proof   claimResumeProof
	renewal claimResumeRenewalResult
	phase   claimResumeReconciliationPhase
}

func newClaimResumeReconciliationState(proof claimResumeProof, renewal claimResumeRenewalResult) claimResumeReconciliationState {
	return claimResumeReconciliationState{proof: proof, renewal: renewal, phase: claimResumeLabelPhase}
}

func (state claimResumeReconciliationState) afterLabel() claimResumeReconciliationState {
	return claimResumeReconciliationState{proof: state.proof, renewal: state.renewal, phase: claimResumeProjectPhase}
}

// readClaimResumeReconciliationTarget is the immutable read-only target used
// immediately before every GitHub mutation.  It binds issue state, the
// no-open-PR condition, and canonical Project identity/status together.
func (a app) readClaimResumeReconciliationTarget(state claimResumeReconciliationState) (claimResumeReconciliationTarget, error) {
	proof := state.proof
	status, err := a.readIssueStatus(proof.preflight.root, proof.preflight.issue)
	if err != nil {
		return claimResumeReconciliationTarget{}, claimResumeRetry(proof, "issue state read", err)
	}
	if status.State != "OPEN" {
		return claimResumeReconciliationTarget{}, stateError("issue #%d changed to %s during claim recovery; preserve renewed artifacts", proof.preflight.issue, status.State)
	}
	if noPRErr := a.validateNoOpenClaimResumePR(proof.preflight.root, proof.preflight.fixedBranch, proof.preflight.issue); noPRErr != nil {
		return claimResumeReconciliationTarget{}, noPRErr
	}
	items, err := a.projectItems(proof.preflight.root)
	if err != nil {
		return claimResumeReconciliationTarget{}, claimResumeRetry(proof, "Project read", err)
	}
	item, err := canonicalClaimResumeProjectItem(items, proof.preflight.issue)
	if err != nil {
		return claimResumeReconciliationTarget{}, err
	}
	if item.ID != proof.preflight.projectItemID {
		return claimResumeReconciliationTarget{}, stateError("issue #%d canonical Project item changed from %s to %s; preserve renewed artifacts", proof.preflight.issue, proof.preflight.projectItemID, item.ID)
	}
	if item.Status != "Backlog" && item.Status != "Picked" {
		return claimResumeReconciliationTarget{}, stateError("issue #%d Project status moved to %q during recovery; preserve renewed artifacts", proof.preflight.issue, item.Status)
	}
	if state.phase == claimResumeLabelPhase {
		if _, initial := proof.renewal.(claimResumeNoRenewal); initial && (!issueNeedsHuman(status) || item.Status != "Backlog") {
			return claimResumeReconciliationTarget{}, stateError("issue #%d must remain OPEN+needs-human with Project Backlog before first recovery mutation", proof.preflight.issue)
		}
	}
	return claimResumeReconciliationTarget{status: status, item: item}, nil
}

//nolint:gocognit // Label and Project convergence are ordered mutation boundaries.
func (a app) reconcileClaimResumeIssue(proof claimResumeProof, renewal claimResumeRenewalResult) error {
	state := newClaimResumeReconciliationState(proof, renewal)
	target, err := a.readClaimResumeReconciliationTarget(state)
	if err != nil {
		return err
	}
	if issueNeedsHuman(target.status) {
		if _, reconcileErr := a.reconcileClaimResumeNeedsHuman(proof, target.status); reconcileErr != nil {
			return reconcileErr
		}
		state = state.afterLabel()
		target, err = a.readClaimResumeReconciliationTarget(state)
		if err != nil {
			return err
		}
	}
	if issueNeedsHuman(target.status) {
		return stateError("issue #%d still has needs-human; Project status will not be changed", proof.preflight.issue)
	}
	if target.item.Status == "Picked" {
		return nil
	}
	// Reread the whole target after label convergence so a Project/PR race
	// cannot turn a partially reconciled claim into a picked issue.
	target, err = a.readClaimResumeReconciliationTarget(state.afterLabel())
	if err != nil {
		return err
	}
	if issueNeedsHuman(target.status) {
		return stateError("issue #%d still has needs-human; Project status will not be changed", proof.preflight.issue)
	}
	if target.item.Status == "Picked" {
		return nil
	}
	if err := a.setProjectField(proof.preflight.root, target.item.ID, "Status", "Picked"); err != nil {
		items, readErr := a.projectItems(proof.preflight.root)
		if readErr == nil {
			latest, itemErr := canonicalClaimResumeProjectItem(items, proof.preflight.issue)
			if itemErr == nil && latest.ID == proof.preflight.projectItemID && latest.Status == "Picked" {
				return nil
			}
			if itemErr != nil {
				return itemErr
			}
		}
		return claimResumeMutationFailure(proof, "Project Picked", err, readErr)
	}
	latest, readErr := a.readClaimResumeReconciliationTarget(state.afterLabel())
	if readErr != nil {
		return readErr
	}
	if issueNeedsHuman(latest.status) || latest.item.Status != "Picked" {
		return stateError("issue #%d Project Picked response was not verified; preserve renewed artifacts", proof.preflight.issue)
	}
	return nil
}

func (a app) reconcileClaimResumeNeedsHuman(proof claimResumeProof, status issueStatus) (issueStatus, error) {
	if !issueNeedsHuman(status) {
		return status, nil
	}
	_, editErr := a.command(proof.preflight.root, "gh", "issue", "edit", strconv.Itoa(proof.preflight.issue), "--repo", repositoryKey,
		"--remove-label", "needs-human")
	if editErr == nil {
		latest, readErr := a.readIssueStatus(proof.preflight.root, proof.preflight.issue)
		if readErr != nil {
			return status, claimResumeProofFailure(proof, "needs-human label removal needs reconciliation; retry", claimResumeRetry(proof, "needs-human label", readErr))
		}
		if latest.State != "OPEN" {
			return status, stateError("issue #%d changed to %s after label removal; preserve renewed artifacts", proof.preflight.issue, latest.State)
		}
		if issueNeedsHuman(latest) {
			return status, claimResumeProofFailure(proof, "still has needs-human after label removal response; retry", claimResumeRetry(proof, "needs-human label", errors.New("label remains present")))
		}
		return latest, nil
	}
	latest, readErr := a.readIssueStatus(proof.preflight.root, proof.preflight.issue)
	if readErr == nil {
		if latest.State != "OPEN" {
			return status, stateError("issue #%d changed to %s after ambiguous label response; preserve renewed artifacts", proof.preflight.issue, latest.State)
		}
		if !issueNeedsHuman(latest) {
			return latest, nil
		}
	}
	if readErr != nil && operationDispositionOf(readErr) == operationDispositionTerminal {
		return status, stateError("issue #%d needs-human label response could not be trusted; preserve renewed artifacts: %w", proof.preflight.issue, readErr)
	}
	return status, claimResumeRetry(proof, "needs-human label", errors.Join(editErr, readErr))
}

func claimResumeMutationFailure(proof claimResumeProof, operation string, mutationErr, readErr error) error {
	if readErr != nil && operationDispositionOf(readErr) == operationDispositionTerminal {
		return stateError("issue #%d %s response could not be trusted; preserve renewed artifacts: %w", proof.preflight.issue, operation, readErr)
	}
	return claimResumeRetry(proof, operation, errors.Join(mutationErr, readErr))
}

func claimResumeRetry(proof claimResumeProof, operation string, err error) error {
	if err == nil {
		err = errors.New("external response was ambiguous")
	}
	message := fmt.Errorf("issue #%d claim resume %s needs reconciliation: %w. "+claimResumeRecoveryTemplate,
		proof.preflight.issue, operation, err, proof.preflight.issue, proof.preflight.expectedHead, proof.preflight.runID, proof.preflight.handoffCommentID)
	return retryableOperationIfRecoverable("claim resume "+operation, message)
}
