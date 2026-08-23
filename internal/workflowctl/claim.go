package workflowctl

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	claimDuration = 4 * time.Hour
)

func (a app) runClaim(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl claim acquire ISSUE | renew | verify | prune ISSUE")
	}
	switch args[0] {
	case "acquire":
		if len(args) != 2 {
			return usageError("usage: workflowctl claim acquire ISSUE")
		}
		number, err := positiveNumber(args[1])
		if err != nil {
			return usageError("claim acquire: %v", err)
		}
		return a.acquireClaim(number)
	case "renew":
		if len(args) != 1 {
			return usageError("usage: workflowctl claim renew")
		}
		return a.renewClaim()
	case "verify":
		if len(args) != 1 {
			return usageError("usage: workflowctl claim verify")
		}
		return a.verifyClaim()
	case "prune":
		return a.pruneHistoricalClaimsCommand(args[1:])
	default:
		return usageError("unknown claim command %q", args[0])
	}
}

func (a app) acquireClaim(number int) error {
	root, err := a.root()
	if err != nil {
		return err
	}
	if _, launchErr := a.checkDevelopLaunch(root); launchErr != nil {
		return launchErr
	}
	if clearErr := a.clearStaleClaims(root, number); clearErr != nil {
		return clearErr
	}
	if claimableErr := a.assertClaimable(root, number); claimableErr != nil {
		return claimableErr
	}
	branch := fmt.Sprintf("agent/issue-%d", number)
	if fetchErr := a.fetchMain(root); fetchErr != nil {
		return fetchErr
	}
	commit, lease, runID, err := a.newClaimCommit(root, number, "origin/main")
	if err != nil {
		return err
	}
	refspec := commit + ":refs/heads/" + branch
	if _, pushErr := a.command(root, "git", "push", "origin", refspec); pushErr != nil {
		return stateError("issue #%d is already claimed or the atomic claim push failed: %v", number, pushErr)
	}
	localBranch := claimLocalBranch(number, runID)
	worktree, err := a.addClaimWorktree(root, branch, localBranch)
	if err != nil {
		return err
	}
	if err := a.recordClaim(root, number, branch, localBranch, worktree, runID, lease); err != nil {
		return err
	}
	return writeLine(a.stdout, "%s", worktree)
}

func (a app) assertClaimable(root string, number int) error {
	items, err := a.projectItems(root)
	if err != nil {
		return err
	}
	item, err := findProjectIssue(items, number)
	if err != nil {
		return err
	}
	status, err := a.readIssueStatus(root, number)
	if err != nil {
		return err
	}
	if status.State != "OPEN" {
		return stateError("issue #%d is %s", number, status.State)
	}
	if item.Status != "Ready" && item.Status != "Picked" {
		return stateError("issue #%d is %s, not Ready", number, item.Status)
	}
	if issueNeedsHuman(status) {
		return stateError("issue #%d needs human attention", number)
	}
	return nil
}

func (a app) clearStaleClaims(root string, number int) error {
	claims, err := a.listRemoteClaims(root)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if claim.number != number {
			continue
		}
		if claim.active {
			return stateError("issue #%d is claimed by %s until %s", number, claim.branch, claim.lease.Format(time.RFC3339))
		}
		if err := a.archiveStaleClaim(root, claim); err != nil {
			return err
		}
	}
	return nil
}

func (a app) archiveStaleClaim(root string, claim remoteClaim) error {
	open, err := a.command(root, "gh", "pr", "list", "--repo", repositoryKey, "--head", claim.branch,
		"--state", "open", "--json", "number")
	if err != nil {
		return fmt.Errorf("check stale claim PRs: %w", err)
	}
	if open != "[]" {
		if escalateErr := a.escalateStaleClaim(root, claim); escalateErr != nil {
			return escalateErr
		}
		return stateError("stale claim %s has an open PR and was marked needs-human", claim.branch)
	}
	runID, err := randomRunID()
	if err != nil {
		return err
	}
	archive := fmt.Sprintf("agent/archive/issue-%d-%s", claim.number, strings.TrimPrefix(runID, "run-"))
	if _, err := a.command(root, "git", "push", "origin", claim.sha+":refs/heads/"+archive); err != nil {
		return fmt.Errorf("archive stale claim %s: %w", claim.branch, err)
	}
	leaseArg := "--force-with-lease=refs/heads/" + claim.branch + ":" + claim.sha
	if _, err := a.command(root, "git", "push", leaseArg, "origin", ":refs/heads/"+claim.branch); err != nil {
		return stateError("stale claim %s changed during recovery: %v", claim.branch, err)
	}
	body := fmt.Sprintf("Expired claim `%s` was preserved as `%s` before reassignment.\n", claim.branch, archive)
	if _, err := a.commandInput(root, strings.NewReader(body), "gh", "issue", "comment", strconv.Itoa(claim.number),
		"--repo", repositoryKey, "--body-file", "-"); err != nil {
		return fmt.Errorf("record stale claim recovery: %w", err)
	}
	return nil
}

func (a app) escalateStaleClaim(root string, claim remoteClaim) error {
	status, err := a.readIssueStatus(root, claim.number)
	if err != nil {
		return err
	}
	if !issueNeedsHuman(status) {
		if _, err := a.command(root, "gh", "issue", "edit", strconv.Itoa(claim.number), "--repo", repositoryKey,
			"--add-label", "needs-human"); err != nil {
			return fmt.Errorf("mark stale claim issue #%d needs-human: %w", claim.number, err)
		}
		body := fmt.Sprintf("Claim `%s` expired with an open PR. The branch was preserved and this issue needs human review.\n",
			claim.branch)
		if _, err := a.commandInput(root, strings.NewReader(body), "gh", "issue", "comment", strconv.Itoa(claim.number),
			"--repo", repositoryKey, "--body-file", "-"); err != nil {
			return fmt.Errorf("record stale claim escalation: %w", err)
		}
	}
	return a.setIssueProjectStatus(root, claim.number, "Backlog")
}

func (a app) fetchMain(root string) error {
	if _, err := a.command(root, "git", "fetch", "origin", "main"); err != nil {
		return fmt.Errorf("fetch origin/main: %w", err)
	}
	return nil
}

func (a app) newClaimCommit(root string, number int, parent string) (string, time.Time, string, error) {
	runID, err := randomRunID()
	if err != nil {
		return "", time.Time{}, "", err
	}
	return a.newClaimCommitWithRunID(root, number, parent, runID)
}

func (a app) newClaimCommitWithRunID(root string, number int, parent, runID string) (string, time.Time, string, error) {
	tree, err := a.command(root, "git", "rev-parse", parent+"^{tree}")
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("read claim tree: %w", err)
	}
	if strings.TrimSpace(runID) == "" {
		return "", time.Time{}, "", errors.New("claim run ID must not be empty")
	}
	lease := time.Now().UTC().Add(claimDuration).Truncate(time.Second)
	message := claimMessage(number, runID, lease)
	commit, err := a.commandInput(root, strings.NewReader(message), "git", "commit-tree", tree, "-p", parent)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("create claim commit: %w", err)
	}
	return commit, lease, runID, nil
}

func claimMessage(number int, runID string, lease time.Time) string {
	return fmt.Sprintf("chore(workflow): claim issue #%d\n\nAgent-Persona: Smith\nAgent-Run-ID: %s\nAgent-Lease-Until: %s\nAgent-Issue: %d\n",
		number, runID, lease.Format(time.RFC3339), number)
}

func randomRunID() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate run ID: %w", err)
	}
	return "run-" + hex.EncodeToString(value[:]), nil
}

func (a app) addClaimWorktree(root, branch, localBranch string) (string, error) {
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return "", err
	}
	path := claimWorktreePath(layout.primaryRoot, localBranch)
	if _, err := a.command(root, "git", "worktree", "add", "-b", localBranch, path, "origin/"+branch); err != nil {
		return "", fmt.Errorf("create claim worktree: %w", err)
	}
	return path, nil
}

func (a app) recordClaim(root string, number int, branch, localBranch, worktree, runID string, lease time.Time) error {
	body := fmt.Sprintf("Claim acquired.\n\n- Branch: `%s`\n- Local branch: `%s`\n- Worktree: `%s`\n- Run: `%s`\n- Lease until: `%s`\n",
		branch, localBranch, worktree, runID, lease.Format(time.RFC3339))
	if _, err := a.commandInput(root, strings.NewReader(body), "gh", "issue", "comment", strconv.Itoa(number),
		"--repo", repositoryKey, "--body-file", "-"); err != nil {
		return fmt.Errorf("record claim on issue #%d: %w", number, err)
	}
	return a.setIssueProjectStatus(root, number, "Picked")
}

func (a app) renewClaim() error {
	root, localBranch, number, err := a.currentClaim()
	if err != nil {
		return err
	}
	branch := claimBranch(number)
	if fetchErr := a.fetchClaim(root, branch); fetchErr != nil {
		return fetchErr
	}
	local, remote, err := a.claimHeads(root, branch)
	if err != nil {
		return err
	}
	if local != remote {
		if _, ancestorErr := a.command(root, "git", "merge-base", "--is-ancestor", remote, local); ancestorErr != nil {
			return stateError("claim branch diverged; local=%s remote=%s", local, remote)
		}
	}
	lease, runID, err := a.readClaimMetadata(root)
	if err != nil {
		return stateError("claim #%d has no valid lease: %v", number, err)
	}
	if identityErr := validateClaimLocalBranch(localBranch, number, runID); identityErr != nil {
		return identityErr
	}
	now := time.Now().UTC()
	if freshnessErr := validateClaimDeadline(number, lease, now); freshnessErr != nil {
		return freshnessErr
	}
	commit, lease, _, err := a.newClaimCommitWithRunID(root, number, "HEAD", runID)
	if err != nil {
		return err
	}
	if _, err := a.command(root, "git", "update-ref", "refs/heads/"+localBranch, commit, local); err != nil {
		return fmt.Errorf("advance local claim: %w", err)
	}
	refspec := commit + ":refs/heads/" + branch
	if _, err := a.command(root, "git", "push", "origin", refspec); err != nil {
		_, restoreErr := a.command(root, "git", "update-ref", "refs/heads/"+localBranch, local, commit)
		if restoreErr != nil {
			return fmt.Errorf("renew claim: %w; restore local ref: %w", err, restoreErr)
		}
		return stateError("renew claim: remote branch changed: %v", err)
	}
	return writeLine(a.stdout, "claim #%d renewed until %s", number, lease.Format(time.RFC3339))
}

func (a app) verifyClaim() error {
	root, localBranch, number, err := a.currentClaim()
	if err != nil {
		return err
	}
	branch := claimBranch(number)
	if fetchErr := a.fetchClaim(root, branch); fetchErr != nil {
		return fetchErr
	}
	local, remote, err := a.claimHeads(root, branch)
	if err != nil {
		return err
	}
	if local != remote {
		return stateError("claim branch moved remotely; local=%s remote=%s", local, remote)
	}
	lease, runID, err := a.readClaimMetadata(root)
	if err != nil {
		return stateError("claim #%d has no valid lease: %v", number, err)
	}
	if identityErr := validateClaimLocalBranch(localBranch, number, runID); identityErr != nil {
		return identityErr
	}
	if err := validateClaimDeadline(number, lease, time.Now().UTC()); err != nil {
		return err
	}
	return writeLine(a.stdout, "claim #%d valid until %s", number, lease.Format(time.RFC3339))
}

func (a app) verifyClaimForPush(root, branch string, number int) error {
	remoteBranch := claimBranch(number)
	if err := a.fetchClaim(root, remoteBranch); err != nil {
		return err
	}
	local, remote, err := a.claimHeads(root, remoteBranch)
	if err != nil {
		return err
	}
	if local != remote {
		if _, ancestorErr := a.command(root, "git", "merge-base", "--is-ancestor", remote, local); ancestorErr != nil {
			return stateError("claim branch diverged; local=%s remote=%s", local, remote)
		}
	}
	lease, runID, err := a.readClaimMetadata(root)
	if err != nil {
		return stateError("claim #%d has no valid lease: %v", number, err)
	}
	if identityErr := validateClaimLocalBranch(branch, number, runID); identityErr != nil {
		return identityErr
	}
	return validateClaimDeadline(number, lease, time.Now().UTC())
}

func (a app) readClaimMetadata(root string) (time.Time, string, error) {
	text, err := a.command(root, "git", "log", "-100", "--format=%B")
	if err != nil {
		return time.Time{}, "", fmt.Errorf("read claim metadata: %w", err)
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

func validateClaimDeadline(number int, deadline, now time.Time) error {
	if deadline.After(now) {
		return nil
	}
	return stateError("claim #%d expired at %s", number, deadline.Format(time.RFC3339))
}

func (a app) currentClaim() (string, string, int, error) {
	root, err := a.root()
	if err != nil {
		return "", "", 0, err
	}
	branch, err := a.command(root, "git", "branch", "--show-current")
	if err != nil {
		return "", "", 0, fmt.Errorf("read branch: %w", err)
	}
	number, ok := issueFromBranch(branch)
	if !ok {
		return "", "", 0, stateError("branch %q is not an issue claim", branch)
	}
	return root, branch, number, nil
}

func claimBranch(number int) string {
	return fmt.Sprintf("agent/issue-%d", number)
}

func claimLocalBranch(number int, runID string) string {
	return claimBranch(number) + "-" + runID
}

func validateClaimLocalBranch(branch string, number int, runID string) error {
	if branch == claimBranch(number) {
		return nil
	}
	expected := claimLocalBranch(number, runID)
	if branch != expected {
		return stateError("local claim branch %q does not match Agent-Run-ID %q; expected %q", branch, runID, expected)
	}
	return nil
}

func (a app) fetchClaim(root, branch string) error {
	refspec := "refs/heads/" + branch + ":refs/remotes/origin/" + branch
	if _, err := a.command(root, "git", "fetch", "origin", refspec); err != nil {
		return stateError("fetch claim branch: %v", err)
	}
	return nil
}

func (a app) claimHeads(root, branch string) (string, string, error) {
	local, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	remote, err := a.command(root, "git", "rev-parse", "origin/"+branch)
	if err != nil {
		return "", "", err
	}
	return local, remote, nil
}

func issueFromBranch(branch string) (int, bool) {
	value := strings.TrimPrefix(branch, "agent/issue-")
	if value == branch || value == "" {
		return 0, false
	}
	digits := value
	if index := strings.IndexByte(value, '-'); index >= 0 {
		digits = value[:index]
	}
	number, err := strconv.Atoi(digits)
	return number, err == nil && number > 0
}

func trailerTime(message string) (time.Time, error) {
	value, err := trailerValue(message, "Agent-Lease-Until")
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Agent-Lease-Until: %w", err)
	}
	return parsed, nil
}

func trailerValue(message, name string) (string, error) {
	prefix := name + ":"
	for _, line := range strings.Split(message, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if value == "" {
			return "", fmt.Errorf("empty %s trailer", name)
		}
		return value, nil
	}
	return "", fmt.Errorf("missing %s trailer", name)
}

func positiveNumber(text string) (int, error) {
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("%q is not a positive number", text)
	}
	return number, nil
}

func (a app) setIssueProjectStatus(root string, number int, status string) error {
	items, err := a.projectItems(root)
	if err != nil {
		return err
	}
	item, err := findProjectIssue(items, number)
	if err != nil {
		return err
	}
	if item.Status == status {
		return nil
	}
	return a.setProjectField(root, item.ID, "Status", status)
}
