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

type claimResumeProof struct {
	root             string
	localBranch      string
	fixedBranch      string
	worktree         string
	expectedHead     string
	localHead        string
	remoteHead       string
	renewalHead      string
	runID            string
	issue            int
	handoffCommentID int64
	claimCommentID   int64
	handoffBody      string
	claimLease       time.Time
	projectItemID    string
	projectStatus    string
	needsHuman       bool
	renewalPresent   bool
}

type claimResumeCommitMetadata struct {
	lease time.Time
	runID string
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
		proof.issue, proof.fixedBranch, proof.runID, proof.expectedHead, proof.handoffCommentID); err != nil {
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
	inventory, err := a.remoteAgentRefInventory(root)
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
	if prErr := a.validateNoOpenClaimResumePR(root, fixedBranch, issue); prErr != nil {
		return claimResumeProof{}, prErr
	}
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return claimResumeProof{}, err
	}
	if worktreeErr := validateResumeWorktree(layout, root, localBranch, issue, localHead); worktreeErr != nil {
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
	renewalHead, renewalPresent, err := a.claimResumeRenewalHead(root, expectedHead, localHead, remoteHead, issue, runID)
	if err != nil {
		return claimResumeProof{}, err
	}
	needsHuman := issueNeedsHuman(issueStatus)
	if !renewalPresent && (!needsHuman || item.Status != "Backlog") {
		return claimResumeProof{}, stateError("issue #%d requires needs-human and Project Backlog before no-PR claim recovery; no mutation performed", issue)
	}
	return claimResumeProof{
		root: root, localBranch: localBranch, fixedBranch: fixedBranch, worktree: root,
		expectedHead: expectedHead, localHead: localHead, remoteHead: remoteHead, renewalHead: renewalHead,
		runID: runID, issue: issue, handoffCommentID: handoffCommentID, claimCommentID: evidence.claimCommentID,
		handoffBody: evidence.handoffBody, claimLease: metadata.lease, projectItemID: item.ID,
		projectStatus: item.Status, needsHuman: needsHuman, renewalPresent: renewalPresent,
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
	message, err := a.command(root, "git", "log", "-1", "--format=%B", head)
	if err != nil {
		return claimResumeCommitMetadata{}, fmt.Errorf("read exact claim metadata at %s: %w", head, err)
	}
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	lines := strings.Split(message, "\n")
	if len(lines) != 7 || lines[0] != fmt.Sprintf("chore(workflow): claim issue #%d", issue) || lines[1] != "" || lines[2] != "Agent-Persona: Smith" {
		return claimResumeCommitMetadata{}, stateError("expected head %s does not contain the exact expired Smith claim metadata; preserve claim artifacts", head)
	}
	if !strings.HasPrefix(lines[3], "Agent-Run-ID: ") || !strings.HasPrefix(lines[4], "Agent-Lease-Until: ") ||
		!strings.HasPrefix(lines[5], "Agent-Issue: ") || lines[6] != "" {
		return claimResumeCommitMetadata{}, stateError("expected head %s has malformed exact claim metadata; preserve claim artifacts", head)
	}
	observedRunID := strings.TrimPrefix(lines[3], "Agent-Run-ID: ")
	leaseValue := strings.TrimPrefix(lines[4], "Agent-Lease-Until: ")
	issueValue := strings.TrimPrefix(lines[5], "Agent-Issue: ")
	if observedRunID != runID || !validRunID(observedRunID) || issueValue != strconv.Itoa(issue) {
		return claimResumeCommitMetadata{}, stateError("expected head %s claim metadata does not match issue #%d and run %s; preserve claim artifacts", head, issue, runID)
	}
	lease, err := time.Parse(time.RFC3339, leaseValue)
	if err != nil || lease.Format(time.RFC3339) != leaseValue {
		return claimResumeCommitMetadata{}, stateError("expected head %s has malformed claim lease; preserve claim artifacts", head)
	}
	if lease.After(time.Now().UTC()) {
		return claimResumeCommitMetadata{}, stateError("claim #%d is active until %s; use claim renew", issue, lease.Format(time.RFC3339))
	}
	return claimResumeCommitMetadata{lease: lease, runID: observedRunID}, nil
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
	if !strings.Contains(handoffBody, claim.worktree) {
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
	if !strings.HasPrefix(body, "## Blocker\n\n") || !strings.Contains(body, "\n## Evidence\n\n") {
		return errors.New("body must contain the exact Blocker and Evidence sections")
	}
	headings := make([]string, 0, 4)
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		headings = append(headings, line)
	}
	wantHeadings := [][]string{
		{"## Blocker", "## Evidence", "## Decisions and risks", "## Next action"},
		{"## Blocker", "## Evidence", "## Risk and next action"},
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
	if !strings.Contains(body, fmt.Sprintf("Issue #%d", issue)) {
		return errors.New("body does not identify the requested issue")
	}
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "claim") {
		return errors.New("body lacks claim-state evidence")
	}
	folded := strings.Join(strings.Fields(lower), " ")
	cleanEvidence := strings.Contains(folded, "worktree was clean") || strings.Contains(folded, "clean worktree") ||
		strings.Contains(folded, "worktree clean") || strings.Contains(folded, "only the generated claim commit") ||
		strings.Contains(folded, "only the generated workflow claim commit") ||
		strings.Contains(folded, "worktree preserved")
	if !cleanEvidence || (!strings.Contains(body, "No implementation") && !strings.Contains(body, "No source")) {
		return errors.New("body lacks preserved worktree/no-source evidence")
	}
	noPR := false
	for _, paragraph := range strings.Split(body, "\n\n") {
		foldedParagraph := strings.Join(strings.Fields(paragraph), " ")
		for _, marker := range []string{"No implementation", "No source"} {
			markerIndex := strings.Index(foldedParagraph, marker)
			if markerIndex >= 0 && strings.Contains(foldedParagraph[markerIndex:], "PR") {
				noPR = true
				break
			}
		}
		if noPR {
			break
		}
	}
	if !noPR {
		return errors.New("body lacks an explicit no-PR statement")
	}
	return nil
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

func (a app) claimResumeRenewalHead(root, expectedHead, localHead, remoteHead string, issue int, runID string) (string, bool, error) {
	if localHead == expectedHead && remoteHead == expectedHead {
		return "", false, nil
	}
	renewal := ""
	if localHead != expectedHead {
		if err := a.validateExistingResumeCommit(root, localHead, expectedHead, issue, runID); err != nil {
			return "", false, stateError("local claim head %s is moved or not the unique renewal child; preserve it: %w", localHead, err)
		}
		renewal = localHead
	}
	if remoteHead != expectedHead {
		if err := a.validateExistingResumeCommit(root, remoteHead, expectedHead, issue, runID); err != nil {
			return "", false, stateError("remote fixed claim head %s is moved or not the unique renewal child; preserve it: %w", remoteHead, err)
		}
		if renewal != "" && renewal != remoteHead {
			return "", false, stateError("local and remote renewal children disagree (%s versus %s); preserve both artifacts", renewal, remoteHead)
		}
		renewal = remoteHead
	}
	return renewal, true, nil
}

//nolint:gocognit // Ref parsing and deterministic partitioning are one artifact check.
func (a app) claimResumeAgentRefs(root, namespace string, source claimRefSource) (claimResumeAgentRefs, error) {
	output, err := a.command(root, "git", "for-each-ref", "--format=%(refname:short) %(objectname)", namespace)
	if err != nil {
		return claimResumeAgentRefs{}, fmt.Errorf("list %s claim refs: %w", sourceName(source), err)
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
	if before.root != after.root || before.localBranch != after.localBranch || before.fixedBranch != after.fixedBranch ||
		before.worktree != after.worktree || before.expectedHead != after.expectedHead || before.localHead != after.localHead ||
		before.remoteHead != after.remoteHead || before.renewalHead != after.renewalHead || before.runID != after.runID ||
		before.issue != after.issue || before.handoffCommentID != after.handoffCommentID || before.claimCommentID != after.claimCommentID ||
		before.handoffBody != after.handoffBody || !before.claimLease.Equal(after.claimLease) || before.projectItemID != after.projectItemID ||
		before.projectStatus != after.projectStatus || before.needsHuman != after.needsHuman || before.renewalPresent != after.renewalPresent {
		return stateError("bound issue, handoff, ref, claim, Project, or worktree proof no longer matches")
	}
	return nil
}

func (a app) applyClaimResume(proof claimResumeProof) error {
	// This is the fresh read-only seal. No ref or GitHub mutation precedes it.
	fresh, err := a.readClaimResumeProof(proof.issue, proof.expectedHead, proof.runID, proof.handoffCommentID)
	if err != nil {
		return stateError("issue #%d claim resume proof changed before renewal; no mutation performed: %w", proof.issue, err)
	}
	if err := sameClaimResumeProof(proof, fresh); err != nil {
		return stateError("issue #%d claim resume proof changed before renewal; no mutation performed: %w", proof.issue, err)
	}
	if !fresh.renewalPresent {
		if err := a.createClaimResumeRenewal(&fresh); err != nil {
			return err
		}
	}
	if fresh.renewalPresent {
		if err := a.adoptClaimResumeLocalRenewal(&fresh); err != nil {
			return err
		}
		if err := a.pushClaimResumeRenewal(&fresh); err != nil {
			return err
		}
	}
	if err := a.verifyClaimResumeRenewal(fresh); err != nil {
		return stateError("issue #%d claim renewal needs reconciliation: %w. "+claimResumeRecoveryTemplate, proof.issue,
			retryableOperationIfRecoverable("claim resume renewal verification", err),
			proof.issue, proof.expectedHead, proof.runID, proof.handoffCommentID)
	}
	if err := a.reconcileClaimResumeIssue(fresh); err != nil {
		return err
	}
	return writeLine(a.stdout, "issue #%d claim resumed; claim verified, needs-human removed, Project Picked", proof.issue)
}

func (a app) createClaimResumeRenewal(proof *claimResumeProof) error {
	if proof.renewalPresent {
		return nil
	}
	commit, _, _, err := a.newClaimCommitWithRunID(proof.root, proof.issue, proof.expectedHead, proof.runID)
	if err != nil {
		return stateError("issue #%d could not create the renewal commit; retry: %w", proof.issue, claimResumeRetry(*proof, "renewal commit", err))
	}
	proof.renewalHead = commit
	if err := a.adoptClaimResumeLocalRenewal(proof); err != nil {
		return err
	}
	return a.pushClaimResumeRenewal(proof)
}

func (a app) adoptClaimResumeLocalRenewal(proof *claimResumeProof) error {
	if proof.localHead == proof.renewalHead {
		return nil
	}
	if proof.localHead != proof.expectedHead {
		return stateError("local claim head moved before renewal adoption: expected %s or child %s, found %s; preserve artifacts", proof.expectedHead, proof.renewalHead, proof.localHead)
	}
	if _, err := a.command(proof.root, "git", "update-ref", "refs/heads/"+proof.localBranch, proof.renewalHead, proof.expectedHead); err != nil {
		observed, readErr := a.readClaimResumeLocalHead(proof.root, proof.localBranch)
		if readErr == nil && observed == proof.renewalHead && a.validateExistingResumeCommit(proof.root, observed, proof.expectedHead, proof.issue, proof.runID) == nil {
			proof.localHead, proof.renewalHead, proof.renewalPresent = observed, observed, true
			return nil
		}
		return stateError("issue #%d local renewal CAS response was ambiguous; no external state changed; %w. "+claimResumeRecoveryTemplate,
			proof.issue, retryableOperationIfRecoverable("claim resume local renewal CAS", errors.Join(err, readErr)), proof.issue, proof.expectedHead, proof.runID, proof.handoffCommentID)
	}
	proof.localHead, proof.renewalPresent = proof.renewalHead, true
	return nil
}

func (a app) readClaimResumeLocalHead(root, branch string) (string, error) {
	head, err := a.command(root, "git", "rev-parse", "refs/heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("read local claim ref %s: %w", branch, err)
	}
	if !validExactCommitSHA(head) {
		return "", stateError("local claim ref %s returned malformed head %q", branch, head)
	}
	return head, nil
}

func (a app) pushClaimResumeRenewal(proof *claimResumeProof) error {
	if proof.remoteHead == proof.renewalHead {
		return nil
	}
	if proof.remoteHead != proof.expectedHead {
		return stateError("remote fixed claim branch moved before renewal push: expected %s, found %s; preserve artifacts", proof.expectedHead, proof.remoteHead)
	}
	lease := "--force-with-lease=refs/heads/" + proof.fixedBranch + ":" + proof.expectedHead
	refspec := proof.renewalHead + ":refs/heads/" + proof.fixedBranch
	pushOutput, pushErr := a.command(proof.root, "git", "push", lease, "origin", refspec)
	remote, readErr := a.remoteClaimHead(proof.root, proof.fixedBranch)
	if readErr == nil && remote == proof.renewalHead {
		proof.remoteHead = remote
		return nil
	}
	if readErr != nil {
		return stateError("issue #%d renewal push response was ambiguous; reread failed: %w. "+claimResumeRecoveryTemplate,
			proof.issue, retryableOperationIfRecoverable("claim resume renewal push", errors.Join(pushErr, readErr)), proof.issue, proof.expectedHead, proof.runID, proof.handoffCommentID)
	}
	if remote != proof.expectedHead {
		return stateError("remote fixed claim branch moved during renewal push: expected %s or valid child %s, found %s; preserve artifacts", proof.expectedHead, proof.renewalHead, remote)
	}
	if pushErr == nil {
		pushErr = errors.New("push returned without advancing the fixed claim branch")
	}
	return stateError("issue #%d renewal push response was ambiguous; remote remains at expected head; %v %w. "+claimResumeRecoveryTemplate,
		proof.issue, pushOutput, claimResumeRetry(*proof, "renewal push", pushErr), proof.issue, proof.expectedHead, proof.runID, proof.handoffCommentID)
}

func (a app) verifyClaimResumeRenewal(proof claimResumeProof) error {
	local, err := a.readClaimResumeLocalHead(proof.root, proof.localBranch)
	if err != nil {
		return err
	}
	if local != proof.renewalHead {
		return stateError("local renewal ref moved: expected %s, found %s; preserve artifacts", proof.renewalHead, local)
	}
	currentHead, err := a.command(proof.root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read renewed worktree head: %w", err)
	}
	if currentHead != proof.renewalHead {
		return stateError("renewed worktree head moved: expected %s, found %s; preserve artifacts", proof.renewalHead, currentHead)
	}
	status, err := a.command(proof.root, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return fmt.Errorf("inspect renewed claim worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return stateError("claim worktree became dirty during renewal; preserve its changes")
	}
	if metadataErr := a.validateExistingResumeCommit(proof.root, proof.renewalHead, proof.expectedHead, proof.issue, proof.runID); metadataErr != nil {
		return metadataErr
	}
	layout, err := a.repositoryLayout(proof.root)
	if err != nil {
		return err
	}
	if worktreeErr := validateResumeWorktree(layout, proof.root, proof.localBranch, proof.issue, proof.renewalHead); worktreeErr != nil {
		return worktreeErr
	}
	inventory, err := a.remoteAgentRefInventory(proof.root)
	if err != nil {
		return err
	}
	remote, err := claimResumeFixedHead(inventory, proof.issue, proof.fixedBranch)
	if err != nil {
		return err
	}
	if remote != proof.renewalHead {
		return stateError("remote fixed claim branch is not the verified renewal child: expected %s, found %s", proof.renewalHead, remote)
	}
	if err := a.validateNoOpenClaimResumePR(proof.root, proof.fixedBranch, proof.issue); err != nil {
		return err
	}
	return a.validateClaimResumeRefs(proof.root, inventory, proof.issue, proof.fixedBranch, proof.localBranch, proof.runID,
		proof.expectedHead, proof.renewalHead, proof.renewalHead)
}

func (a app) reconcileClaimResumeIssue(proof claimResumeProof) error {
	status, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		return stateError("issue #%d label reconciliation could not read state; retry: %w", proof.issue, claimResumeRetry(proof, "label read", err))
	}
	if status.State != "OPEN" {
		return stateError("issue #%d changed to %s during claim recovery; preserve renewed artifacts", proof.issue, status.State)
	}
	status, err = a.reconcileClaimResumeNeedsHuman(proof, status)
	if err != nil {
		return err
	}
	if issueNeedsHuman(status) {
		return stateError("issue #%d still has needs-human; Project status will not be changed", proof.issue)
	}
	if reconcileErr := a.reconcileClaimResumeProject(proof); reconcileErr != nil {
		return reconcileErr
	}
	finalStatus, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		return stateError("issue #%d final state reread failed; retry: %w", proof.issue, claimResumeRetry(proof, "final issue read", err))
	}
	if finalStatus.State != "OPEN" || issueNeedsHuman(finalStatus) {
		return stateError("issue #%d final state is not OPEN without needs-human; preserve renewed artifacts", proof.issue)
	}
	return nil
}

func (a app) reconcileClaimResumeNeedsHuman(proof claimResumeProof, status issueStatus) (issueStatus, error) {
	if !issueNeedsHuman(status) {
		return status, nil
	}
	_, editErr := a.command(proof.root, "gh", "issue", "edit", strconv.Itoa(proof.issue), "--repo", repositoryKey,
		"--remove-label", "needs-human")
	if editErr == nil {
		latest, readErr := a.readIssueStatus(proof.root, proof.issue)
		if readErr != nil {
			return status, stateError("issue #%d needs-human label removal needs reconciliation; retry: %w", proof.issue,
				claimResumeRetry(proof, "needs-human label", readErr))
		}
		if latest.State != "OPEN" {
			return status, stateError("issue #%d changed to %s after label removal; preserve renewed artifacts", proof.issue, latest.State)
		}
		if issueNeedsHuman(latest) {
			return status, stateError("issue #%d still has needs-human after label removal response; retry: %w", proof.issue,
				claimResumeRetry(proof, "needs-human label", errors.New("label remains present")))
		}
		return latest, nil
	}
	latest, readErr := a.readIssueStatus(proof.root, proof.issue)
	if readErr == nil {
		if latest.State != "OPEN" {
			return status, stateError("issue #%d changed to %s after ambiguous label response; preserve renewed artifacts", proof.issue, latest.State)
		}
		if !issueNeedsHuman(latest) {
			return latest, nil
		}
	}
	return status, stateError("issue #%d needs-human label response was ambiguous; retry: %w", proof.issue,
		claimResumeRetry(proof, "needs-human label", errors.Join(editErr, readErr)))
}

func (a app) reconcileClaimResumeProject(proof claimResumeProof) error {
	items, err := a.projectItems(proof.root)
	if err != nil {
		return stateError("issue #%d Project reconciliation read failed; retry: %w", proof.issue, claimResumeRetry(proof, "Project read", err))
	}
	item, err := canonicalClaimResumeProjectItem(items, proof.issue)
	if err != nil {
		return err
	}
	if item.ID != proof.projectItemID {
		return stateError("issue #%d canonical Project item changed from %s to %s; preserve renewed artifacts", proof.issue, proof.projectItemID, item.ID)
	}
	if item.Status == "Picked" {
		return nil
	}
	if item.Status != "Backlog" {
		return stateError("issue #%d Project status moved to %q during recovery; preserve renewed artifacts", proof.issue, item.Status)
	}
	setErr := a.setProjectField(proof.root, item.ID, "Status", "Picked")
	items, readErr := a.projectItems(proof.root)
	if readErr == nil {
		latest, itemErr := canonicalClaimResumeProjectItem(items, proof.issue)
		if itemErr == nil && latest.ID == proof.projectItemID && latest.Status == "Picked" {
			return nil
		}
		if itemErr != nil {
			return itemErr
		}
		if latest.ID != proof.projectItemID {
			return stateError("issue #%d canonical Project item changed after status response; preserve renewed artifacts", proof.issue)
		}
		if latest.Status != "Backlog" {
			return stateError("issue #%d Project status is %q after status response; preserve renewed artifacts", proof.issue, latest.Status)
		}
	}
	return stateError("issue #%d Project Picked response was ambiguous; retry: %w", proof.issue,
		claimResumeRetry(proof, "Project Picked", errors.Join(setErr, readErr)))
}

func claimResumeRetry(proof claimResumeProof, operation string, err error) error {
	if err == nil {
		err = errors.New("external response was ambiguous")
	}
	return retryableOperation("claim resume "+operation,
		fmt.Errorf("issue #%d claim resume %s needs reconciliation: %w. "+claimResumeRecoveryTemplate,
			proof.issue, operation, err, proof.issue, proof.expectedHead, proof.runID, proof.handoffCommentID))
}
