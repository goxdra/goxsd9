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
		return usageError("usage: workflowctl claim acquire ISSUE | resume ISSUE [flags] | renew | verify | prune ISSUE")
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
	case "resume":
		return a.resumeClaimCommand(args[1:])
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
	tree = strings.TrimSpace(tree)
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
		return retryableOperationIfRecoverable("read claim heads", fmt.Errorf("read claim heads: %w", err))
	}
	if proofErr := a.validateRenewClaimHeads(root, local, remote); proofErr != nil {
		return proofErr
	}
	lease, runID, err := a.proveClaimLeaseAndLocalMarkers(root, localBranch, number, local, remote)
	if err != nil {
		return err
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

func (a app) validateRenewClaimHeads(root, local, remote string) error {
	if local == remote {
		return nil
	}
	if _, err := a.command(root, "git", "merge-base", "--is-ancestor", remote, local); err != nil {
		if isGitNonAncestor(err) {
			return stateError("claim branch diverged; local=%s remote=%s; integrate a pending PR renewal before renewing", local, remote)
		}
		return fmt.Errorf("verify claim ancestry before renewal: %w", err)
	}
	if err := a.validateResumeOperationState(root); err != nil {
		return fmt.Errorf("verify claim worktree before renewal: %w", err)
	}
	return nil
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
		return retryableOperationIfRecoverable("read claim heads", fmt.Errorf("read claim heads: %w", err))
	}
	if local != remote {
		if _, ancestorErr := a.command(root, "git", "merge-base", "--is-ancestor", remote, local); ancestorErr != nil {
			if isGitNonAncestor(ancestorErr) {
				return stateError("claim branch moved remotely; local=%s remote=%s", local, remote)
			}
			return fmt.Errorf("verify remote claim ancestry: %w", ancestorErr)
		}
	}
	lease, _, err := a.proveClaimLeaseAndLocalMarkers(root, localBranch, number, local, remote)
	if err != nil {
		return err
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
			if isGitNonAncestor(ancestorErr) {
				return stateError("claim branch diverged; local=%s remote=%s; integrate pending PR renewal before pushing", local, remote)
			}
			return fmt.Errorf("verify claim ancestry for push: %w", ancestorErr)
		}
		if operationErr := a.validateResumeOperationState(root); operationErr != nil {
			return fmt.Errorf("verify claim worktree for push: %w", operationErr)
		}
	}
	lease, _, err := a.proveClaimLeaseAndLocalMarkers(root, branch, number, local, remote)
	if err != nil {
		return fmt.Errorf("verify claim #%d for push: %w", number, err)
	}
	return validateClaimDeadline(number, lease, time.Now().UTC())
}

func (a app) proveClaimLeaseAndLocalMarkers(root, branch string, number int, local, remote string) (time.Time, string, error) {
	marker, err := a.readAuthoritativeClaimMarker(root, remote, number)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("read authoritative claim metadata at %s: %w", remote, err)
	}
	if branchErr := validateClaimLocalBranch(branch, number, marker.runID); branchErr != nil {
		return time.Time{}, "", branchErr
	}
	if local == remote {
		return marker.lease, marker.runID, nil
	}
	if markerErr := a.verifyUnpublishedClaimMarkers(root, branch, number, marker, local, remote); markerErr != nil {
		return time.Time{}, "", markerErr
	}
	localMarker, err := a.readAuthoritativeClaimMarker(root, local, number)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("read prospective local claim authority at %s: %w", local, err)
	}
	if localMarker.runID != marker.runID || localMarker.lease.Before(marker.lease) {
		return time.Time{}, "", stateError("prospective local claim authority at %s regresses remote issue #%d run or lease; preserve claim artifacts", local, number)
	}
	if err := validateClaimDeadline(number, localMarker.lease, time.Now().UTC()); err != nil {
		return time.Time{}, "", err
	}
	return marker.lease, marker.runID, nil
}

func (a app) readAuthoritativeClaimMarker(root, head string, number int) (canonicalClaimCommit, error) {
	proof := &claimAuthorityProof{proven: make(map[string]canonicalClaimCommit), visiting: make(map[string]bool)}
	return a.readAuthoritativeClaimMarkerCached(root, head, number, proof)
}

type claimAuthorityProof struct {
	proven   map[string]canonicalClaimCommit
	visiting map[string]bool
}

//nolint:gocognit // Recursion state and exact first-parent marker selection share one proof boundary.
func (a app) readAuthoritativeClaimMarkerCached(root, head string, number int, proof *claimAuthorityProof) (canonicalClaimCommit, error) {
	if marker, ok := proof.proven[head]; ok {
		return marker, nil
	}
	if proof.visiting[head] {
		return canonicalClaimCommit{}, stateError("claim authority graph revisits commit %s; preserve claim artifacts", head)
	}
	proof.visiting[head] = true
	defer delete(proof.visiting, head)
	history, err := a.command(root, "git", "log", "--first-parent", "--format=%H%x00%B%x00", head)
	if err != nil {
		return canonicalClaimCommit{}, retryableOperation("read claim metadata", fmt.Errorf("read first-parent claim history: %w", err))
	}
	records, err := splitRunLocalHistory(history, claimBranch(number))
	if err != nil {
		return canonicalClaimCommit{}, err
	}
	for index, record := range records {
		if isClaimRenewalIntegrationShape(record.message) {
			marker, markerErr := a.readAdoptedClaimMarker(root, records[index:], number, proof)
			if markerErr == nil {
				proof.proven[head] = marker
			}
			return marker, markerErr
		}
		if !isCanonicalClaimMarkerShape(record.message) {
			continue
		}
		marker, err := a.readCanonicalClaimIdentity(root, record.commit, "")
		if err != nil {
			return canonicalClaimCommit{}, fmt.Errorf("verify authoritative claim marker %s: %w", record.commit, err)
		}
		if marker.message != record.message {
			return canonicalClaimCommit{}, stateError("claim marker %s history disagrees with its Git object; preserve claim artifacts", record.commit)
		}
		if marker.issue != number {
			return canonicalClaimCommit{}, stateError("claim marker %s binds issue #%d, not claim issue #%d; preserve claim artifacts", record.commit, marker.issue, number)
		}
		proof.proven[head] = marker
		return marker, nil
	}
	return canonicalClaimCommit{}, stateError("claim head %s has no canonical first-parent marker for issue #%d; preserve claim artifacts", head, number)
}

func isClaimRenewalIntegrationShape(message string) bool {
	return strings.HasPrefix(message, "chore(workflow): integrate claim renewal #")
}

// readAdoptedClaimMarker accepts the second parent only for the exact empty
// integration commit emitted by pr resume --integrate. The renewed marker's
// parent must be in source ancestry with no conflicting first-parent marker.
//
//nolint:gocognit // The independent commit, parent, tree, and authority proofs must all fail closed.
func (a app) readAdoptedClaimMarker(root string, records []runLocalHistoryRecord, number int, proof *claimAuthorityProof) (canonicalClaimCommit, error) {
	integration := records[0]
	if integration.message != fmt.Sprintf("chore(workflow): integrate claim renewal #%d\n", number) {
		return canonicalClaimCommit{}, stateError("claim integration %s has non-canonical message for issue #%d; preserve claim artifacts", integration.commit, number)
	}
	object, err := a.gitRaw(root, "cat-file", "commit", integration.commit)
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("read claim integration %s: %w", integration.commit, err)
	}
	parsed, err := parseCommitObject(object)
	if err != nil || len(parsed.parents) != 2 || parsed.message != integration.message || len(records) < 2 || parsed.parents[0] != records[1].commit {
		return canonicalClaimCommit{}, stateError("claim integration %s has non-canonical commit shape; preserve claim artifacts: %v", integration.commit, err)
	}
	for _, parent := range parsed.parents {
		if parentErr := a.validateLocalAgentCommit(root, parent, "claim integration parent "+parent); parentErr != nil {
			return canonicalClaimCommit{}, parentErr
		}
	}
	firstTree, err := a.gitRaw(root, "rev-parse", parsed.parents[0]+"^{tree}")
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("read claim integration source tree: %w", err)
	}
	if tree, parseErr := parseCanonicalSHA(firstTree, "claim integration source tree"); parseErr != nil || tree != parsed.tree {
		return canonicalClaimCommit{}, stateError("claim integration %s changes its source parent's tree; preserve claim artifacts: %v", integration.commit, parseErr)
	}
	marker, err := a.readCanonicalClaimIdentity(root, parsed.parents[1], "")
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("verify claim integration renewal %s: %w", parsed.parents[1], err)
	}
	if marker.issue != number {
		return canonicalClaimCommit{}, stateError("claim integration %s renews issue #%d, not issue #%d; preserve claim artifacts", integration.commit, marker.issue, number)
	}
	_, err = a.command(root, "git", "merge-base", "--is-ancestor", marker.parent, parsed.parents[0])
	if err != nil {
		if isGitNonAncestor(err) {
			return canonicalClaimCommit{}, stateError("claim integration %s renewal parent %s is outside local source ancestry; preserve claim artifacts", integration.commit, marker.parent)
		}
		return canonicalClaimCommit{}, fmt.Errorf("prove claim integration source ancestry: %w", err)
	}
	baseMarker, err := a.readAuthoritativeClaimMarkerCached(root, marker.parent, number, proof)
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("verify claim integration original authority at %s: %w", marker.parent, err)
	}
	if baseMarker.runID != marker.runID {
		return canonicalClaimCommit{}, stateError("claim integration %s renewal run %s conflicts with original run %s; preserve claim artifacts", integration.commit, marker.runID, baseMarker.runID)
	}
	if pathErr := a.verifyClaimIntegrationSourcePath(root, integration.commit, marker.parent, records[1:], number, marker.runID, proof); pathErr != nil {
		return canonicalClaimCommit{}, pathErr
	}
	sourceMarker := baseMarker
	for _, record := range records[1:] {
		if !isCanonicalClaimMarkerShape(record.message) && !isClaimRenewalIntegrationShape(record.message) {
			continue
		}
		sourceMarker, err = a.readAuthoritativeClaimMarkerCached(root, record.commit, number, proof)
		if err != nil {
			return canonicalClaimCommit{}, fmt.Errorf("verify claim integration source authority at %s: %w", record.commit, err)
		}
		break
	}
	if sourceMarker.runID != marker.runID {
		return canonicalClaimCommit{}, stateError("claim integration %s renewal run %s conflicts with source run %s; preserve claim artifacts", integration.commit, marker.runID, sourceMarker.runID)
	}
	if marker.lease.Before(sourceMarker.lease) || marker.lease.Before(baseMarker.lease) {
		return canonicalClaimCommit{}, stateError("claim integration %s renewal lease regresses proven authority; preserve claim artifacts", integration.commit)
	}
	return marker, nil
}

//nolint:gocognit // The source path must reject each marker before the shared first-parent boundary.
func (a app) verifyClaimIntegrationSourcePath(root, integration, expected string, source []runLocalHistoryRecord, number int, runID string, proof *claimAuthorityProof) error {
	history, err := a.command(root, "git", "rev-list", "--first-parent", expected)
	if err != nil {
		return fmt.Errorf("read claim integration original first-parent ancestry: %w", err)
	}
	expectedChain := make(map[string]bool)
	for _, commit := range strings.Fields(history) {
		if !validExactCommitSHA(commit) {
			return stateError("claim integration %s original first-parent ancestry is malformed; preserve claim artifacts", integration)
		}
		expectedChain[commit] = true
	}
	for _, record := range source {
		shared := expectedChain[record.commit]
		if isCanonicalClaimMarkerShape(record.message) || isClaimRenewalIntegrationShape(record.message) {
			observed, observedErr := a.readAuthoritativeClaimMarkerCached(root, record.commit, number, proof)
			if observedErr != nil {
				return fmt.Errorf("verify claim integration source marker %s: %w", record.commit, observedErr)
			}
			if observed.runID != runID {
				return stateError("claim integration %s crosses a conflicting first-parent marker %s; preserve claim artifacts", integration, record.commit)
			}
		}
		if shared {
			return nil
		}
	}
	return stateError("claim integration %s has no common first-parent source ancestry with %s; preserve claim artifacts", integration, expected)
}

//nolint:gocognit // Both renewal integrations and direct markers need distinct authenticated checks.
func (a app) verifyUnpublishedClaimMarkers(root, branch string, number int, remoteMarker canonicalClaimCommit, local, remote string) error {
	history, err := a.command(root, "git", "log", "--first-parent", "--format=%H%x00%B%x00", remote+".."+local)
	if err != nil {
		return fmt.Errorf("read unpublished claim history: %w", err)
	}
	records, err := splitRunLocalHistory(history, branch)
	if err != nil {
		return err
	}
	for _, record := range records {
		if isClaimRenewalIntegrationShape(record.message) {
			marker, markerErr := a.readAuthoritativeClaimMarker(root, record.commit, number)
			if markerErr != nil {
				return fmt.Errorf("verify unpublished claim integration %s: %w", record.commit, markerErr)
			}
			if marker.head != remoteMarker.head {
				return stateError("unpublished claim integration %s adopts marker %s, not remote authoritative marker %s; preserve claim artifacts", record.commit, marker.head, remoteMarker.head)
			}
			continue
		}
		if !isCanonicalClaimMarkerShape(record.message) {
			continue
		}
		marker, err := a.readCanonicalClaimIdentity(root, record.commit, "")
		if err != nil {
			return fmt.Errorf("verify unpublished claim marker %s: %w", record.commit, err)
		}
		if marker.message != record.message {
			return stateError("unpublished claim marker %s history disagrees with its Git object; preserve claim artifacts", record.commit)
		}
		if marker.issue != number || marker.runID != remoteMarker.runID {
			return stateError("unpublished claim marker %s binds issue #%d run %s, not remote claim issue #%d run %s; preserve claim artifacts",
				record.commit, marker.issue, marker.runID, number, remoteMarker.runID)
		}
	}
	return nil
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
		return retryableOperationIfRecoverable("fetch claim branch", fmt.Errorf("fetch claim branch: %w", err))
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
