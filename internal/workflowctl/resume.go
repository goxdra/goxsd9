package workflowctl

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const resumeRecoveryTemplate = "Run `go tool workflowctl pr resume %d --expected-head %s --acknowledge-needs-human` again"
const resumeIntegrationRecoveryTemplate = "Local renewal marker is integrated; run `go tool workflowctl pr resume %d --expected-head %s --acknowledge-needs-human --integrate` again to reconcile issue status"
const resumeUsage = "usage: workflowctl pr resume PR --expected-head SHA --acknowledge-needs-human [--dry-run] [--integrate] (reuse the original expired PR head after local work is resolved and clean)"

type resumeProof struct {
	root            string
	localBranch     string
	issue           int
	pr              int
	expectedHead    string
	observedHead    string
	renewalHead     string
	localHead       string
	runID           string
	runLocalHead    string
	runLocalPresent bool
	already         bool
	pending         bool
	renewalExpired  bool
	priorIntegrated bool
	needsHuman      bool
	projectStatus   string
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
		return usageError("%s", resumeUsage)
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
	integrate := flags.Bool("integrate", false, "integrate a remote renewal after local work is resolved")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("pr resume: %v", parseErr)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*expected) == "" {
		return usageError("%s", resumeUsage)
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
	if *integrate {
		if err := a.prepareResumeLocalIntegration(proof); err != nil {
			return err
		}
	}
	if *dryRun {
		return writeLine(a.stdout, "dry-run: preflight complete; no mutation performed")
	}
	if *integrate {
		return a.integratePullRequestResume(proof)
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
	if ancestryErr := a.validateResumeLocalAncestry(root, local, expectedHead); ancestryErr != nil {
		return resumeProof{}, ancestryErr
	}
	localClaim := claim
	if local != expectedHead {
		validatedLocal, claimErr := a.readResumeExpectedClaim(root, local, issue)
		if claimErr != nil {
			return resumeProof{}, fmt.Errorf("prove unpublished local claim lineage: %w", claimErr)
		}
		if validatedLocal.runID != runID {
			return resumeProof{}, stateError("unpublished local head %s belongs to run %s, expected %s", local, validatedLocal.runID, runID)
		}
		localClaim, claimErr = a.readResumeLocalAuthority(root, local, issue, claim)
		if claimErr != nil {
			return resumeProof{}, fmt.Errorf("prove unpublished local claim authority: %w", claimErr)
		}
	}
	runLocal, err := a.inspectResumeClaimConflicts(root, issue, branch, remote, localBranch, runID,
		resumeRunLocalExpectation{}, lineage)
	if err != nil {
		return resumeProof{}, err
	}
	already := remote != expectedHead
	pending := true
	renewalExpired := false
	priorIntegrated := false
	if already {
		renewal, renewalErr := a.readPRResumeRenewalChain(root, remote, expectedHead, issue, runID)
		if renewalErr != nil {
			return resumeProof{}, retryableOperationIfRecoverable("resume canonical renewal proof", renewalErr)
		}
		renewalExpired = !renewal.lease.After(time.Now().UTC())
		if view.HeadRefOID != remote {
			return resumeProof{}, stateError("PR #%d head %s does not match renewed remote head %s", pr, view.HeadRefOID, remote)
		}
		integrated, prior, integrationErr := a.claimRenewalIntegrationState(root, local, remote, issue)
		if integrationErr != nil {
			return resumeProof{}, integrationErr
		}
		pending = !integrated
		priorIntegrated = prior
		if pending {
			if localClaim.lease.After(renewal.lease) {
				return resumeProof{}, stateError("local claim lease %s exceeds remote renewal lease %s; preserve claim artifacts before integration",
					localClaim.lease.Format(time.RFC3339), renewal.lease.Format(time.RFC3339))
			}
		}
	}
	if !already && view.HeadRefOID != expectedHead {
		return resumeProof{}, stateError("resume heads moved: expected=%s PR=%s remote=%s local=%s", expectedHead, view.HeadRefOID, remote, local)
	}
	if !pending {
		if operationErr := a.validateResumeOperationState(root); operationErr != nil {
			return resumeProof{}, fmt.Errorf("verify integrated claim worktree: %w", operationErr)
		}
	}
	if pending && !issueNeedsHuman(status) && !priorIntegrated {
		return resumeProof{}, stateError("issue #%d must be labeled needs-human before stale PR recovery", issue)
	}
	items, err := a.projectItems(root)
	if err != nil {
		return resumeProof{}, fmt.Errorf("read issue #%d Project status before PR recovery: %w", issue, err)
	}
	item, err := findProjectIssue(items, issue)
	if err != nil {
		return resumeProof{}, err
	}
	if pending && item.Status != "Backlog" && !priorIntegrated {
		return resumeProof{}, stateError("issue #%d Project status %q must be Backlog while PR renewal is pending", issue, item.Status)
	}
	protectedHeads := resumeProtectedHeads(expectedHead, remote)
	if pending {
		protectedHeads = resumeProtectedHeads(expectedHead, remote, local)
	}
	if worktreeErr := validateResumeWorktreeHeads(layout, root, localBranch, issue, local, protectedHeads, lineage); worktreeErr != nil {
		return resumeProof{}, worktreeErr
	}
	proof := resumeProof{root: root, localBranch: localBranch, issue: issue, pr: pr, expectedHead: expectedHead,
		observedHead: remote, renewalHead: local, localHead: local, runID: runID, runLocalHead: runLocal.sha, runLocalPresent: runLocal.present,
		already: already, pending: pending, renewalExpired: renewalExpired, priorIntegrated: priorIntegrated,
		needsHuman: issueNeedsHuman(status), projectStatus: item.Status}
	if already {
		proof.renewalHead = remote
	}
	if err := a.sealPullRequestResumeProof(proof); err != nil {
		return resumeProof{}, err
	}
	return proof, nil
}

// readResumeExpectedClaim validates all-parent ownership, then uses proven
// first-parent authority when present. A side-parent-only claim is the fallback.
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
	if verifyErr := a.verifyRunLocalHistoryRecords(root, branch, records); verifyErr != nil {
		return canonicalClaimCommit{}, verifyErr
	}
	selected, err := selectResumeExpectedClaimRecord(records, expectedHead, issue)
	if err != nil {
		return canonicalClaimCommit{}, err
	}
	authority, err := a.readResumeLocalAuthority(root, expectedHead, issue, selected)
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("prove expected PR head first-parent authority: %w", err)
	}
	return authority, nil
}

func selectResumeExpectedClaimRecord(records []runLocalHistoryRecord, expectedHead string, issue int) (canonicalClaimCommit, error) {
	selected := canonicalClaimCommit{}
	selectedCommit := ""
	for _, record := range records {
		marker, canonical, markerErr := parseResumeExpectedClaimRecord(record, expectedHead, issue)
		if markerErr != nil {
			return canonicalClaimCommit{}, markerErr
		}
		if !canonical {
			continue
		}
		if selectedCommit != "" && selected.runID != marker.runID {
			return canonicalClaimCommit{}, stateError("expected PR head %s ancestry has conflicting canonical claim markers %s (run %s) and %s (run %s); preserve claim artifacts", expectedHead, selectedCommit, selected.runID, record.commit, marker.runID)
		}
		if selectedCommit == "" {
			selected = marker
			selectedCommit = record.commit
		}
	}
	if selectedCommit != "" {
		return selected, nil
	}
	return canonicalClaimCommit{}, stateError("expected PR head %s ancestry has no canonical claim marker for issue #%d; preserve claim artifacts", expectedHead, issue)
}

func parseResumeExpectedClaimRecord(record runLocalHistoryRecord, expectedHead string, issue int) (canonicalClaimCommit, bool, error) {
	identity, canonical, parseErr := parseCanonicalRunLocalClaim(record.message, issue)
	if parseErr != nil {
		return canonicalClaimCommit{}, false, terminalRunLocalHistoryError(stateError("preserve resume proof: history commit %s has malformed canonical claim marker: %w", record.commit, parseErr))
	}
	if !canonical {
		if !isCanonicalClaimMarkerShape(record.message) {
			return canonicalClaimCommit{}, false, nil
		}
		observedIssue, _, _, identityErr := parseCanonicalClaimMessage(record.message)
		if identityErr != nil {
			return canonicalClaimCommit{}, false, terminalRunLocalHistoryError(stateError("preserve resume proof: history commit %s has malformed canonical claim marker: %w", record.commit, identityErr))
		}
		return canonicalClaimCommit{}, false, stateError("expected PR head %s ancestry marker %s claims issue #%d, not issue #%d; preserve claim artifacts", expectedHead, record.commit, observedIssue, issue)
	}
	if identity.issue != issue {
		return canonicalClaimCommit{}, false, stateError("expected PR head %s ancestry marker %s claims issue #%d, not issue #%d; preserve claim artifacts", expectedHead, record.commit, identity.issue, issue)
	}
	observedIssue, observedRunID, lease, parseErr := parseCanonicalClaimMessage(record.message)
	if parseErr != nil {
		return canonicalClaimCommit{}, false, terminalRunLocalHistoryError(stateError("preserve resume proof: history commit %s has malformed canonical claim marker: %w", record.commit, parseErr))
	}
	if observedIssue != issue {
		return canonicalClaimCommit{}, false, stateError("expected PR head %s ancestry marker %s claims issue #%d, not issue #%d; preserve claim artifacts", expectedHead, record.commit, observedIssue, issue)
	}
	return canonicalClaimCommit{message: record.message, issue: observedIssue, runID: observedRunID, lease: lease}, true, nil
}

// readResumeLocalAuthority uses the first-parent marker when one exists. A
// source branch may carry the expected claim only on a side parent.
func (a app) readResumeLocalAuthority(root, head string, issue int, expected canonicalClaimCommit) (canonicalClaimCommit, error) {
	history, err := a.command(root, "git", "log", "--first-parent", "--format=%H%x00%B%x00", head)
	if err != nil {
		return canonicalClaimCommit{}, fmt.Errorf("read local first-parent claim history: %w", err)
	}
	records, err := splitRunLocalHistory(history, claimBranch(issue))
	if err != nil {
		return canonicalClaimCommit{}, err
	}
	for _, record := range records {
		if !isCanonicalClaimMarkerShape(record.message) && !isClaimRenewalIntegrationShape(record.message) {
			continue
		}
		marker, markerErr := a.readAuthoritativeClaimMarker(root, head, issue)
		if markerErr != nil {
			return canonicalClaimCommit{}, markerErr
		}
		if marker.runID != expected.runID {
			return canonicalClaimCommit{}, stateError("local first-parent claim run %s conflicts with expected run %s; preserve claim artifacts",
				marker.runID, expected.runID)
		}
		return marker, nil
	}
	return expected, nil
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

func (a app) validateResumeLocalAncestry(root, local, expected string) error {
	if err := a.validateLocalAgentCommit(root, local, "local claim head"); err != nil {
		return err
	}
	if local == expected {
		return nil
	}
	_, err := a.command(root, "git", "merge-base", "--is-ancestor", expected, local)
	if err == nil {
		return nil
	}
	if !isGitNonAncestor(err) {
		return fmt.Errorf("prove local claim ancestry from expected head %s to %s: %w", expected, local, err)
	}
	_, err = a.command(root, "git", "merge-base", "--is-ancestor", local, expected)
	if err == nil {
		return stateError("local claim head %s predates expected PR head %s; preserve it before recovery", local, expected)
	}
	if !isGitNonAncestor(err) {
		return fmt.Errorf("prove whether local claim head %s predates expected head %s: %w", local, expected, err)
	}
	return stateError("local claim head %s does not descend from expected PR head %s", local, expected)
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
	if local != proof.localHead {
		return stateError("local claim head moved while sealing proof: expected %s, found %s", proof.localHead, local)
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
	items, err := a.projectItems(proof.root)
	if err != nil {
		return fmt.Errorf("read issue #%d Project status while sealing PR recovery: %w", proof.issue, err)
	}
	item, err := findProjectIssue(items, proof.issue)
	if err != nil {
		return err
	}
	if item.Status != proof.projectStatus {
		return stateError("issue #%d Project status changed while sealing resume proof", proof.issue)
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
	if local != proof.expectedHead {
		protectedHeads = resumeProtectedHeads(proof.expectedHead, proof.observedHead, local)
	}
	if worktreeErr := validateResumeWorktreeHeads(layout, proof.root, proof.localBranch, proof.issue, local, protectedHeads, lineage); worktreeErr != nil {
		return nil, worktreeErr
	}
	if err := a.validateResumeLocalAncestry(proof.root, local, proof.expectedHead); err != nil {
		return nil, err
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

// readPRResumeRenewalChain keeps the original expired head as the anchor for
// every remote-only retry. Each link is a canonical empty same-run commit.
func (a app) readPRResumeRenewalChain(root, head, expected string, issue int, runID string) (canonicalClaimCommit, error) {
	current := head
	latest := canonicalClaimCommit{}
	newer := canonicalClaimCommit{}
	for current != expected {
		marker, err := a.readCanonicalClaimIdentity(root, current, "")
		if err != nil {
			return canonicalClaimCommit{}, fmt.Errorf("prove PR renewal chain at %s: %w", current, err)
		}
		if marker.issue != issue || marker.runID != runID {
			return canonicalClaimCommit{}, stateError("PR renewal marker %s binds issue #%d run %s, not issue #%d run %s; preserve claim artifacts",
				current, marker.issue, marker.runID, issue, runID)
		}
		if newer.head != "" && newer.lease.Before(marker.lease) {
			return canonicalClaimCommit{}, stateError("PR renewal chain lease regresses at %s; preserve claim artifacts", newer.head)
		}
		if latest.head == "" {
			latest = marker
		}
		newer = marker
		current = marker.parent
	}
	if latest.head == "" {
		return canonicalClaimCommit{}, stateError("PR renewal head %s has no canonical marker after original head %s", head, expected)
	}
	return latest, nil
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
	if !fresh.already || fresh.renewalExpired {
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
	if !fresh.pending && !fresh.needsHuman && fresh.projectStatus == "Picked" {
		return writeLine(a.stdout, "PR #%d is already integrated for issue #%d; claim and Project Picked", fresh.pr, fresh.issue)
	}
	if !fresh.pending {
		return writeLine(a.stdout, "PR #%d local renewal marker is integrated for issue #%d; status reconciliation pending; rerun with --integrate and original expected head", fresh.pr, fresh.issue)
	}
	return writeLine(a.stdout, "PR #%d remote claim renewed for issue #%d; local integration pending; issue and Project status preserved", fresh.pr, fresh.issue)
}

func (a app) integratePullRequestResume(proof resumeProof) error {
	fresh, err := a.readPullRequestResumeProof(proof.pr, proof.expectedHead)
	if err != nil {
		return fmt.Errorf("PR #%d integration proof changed; no mutation performed: %w", proof.pr, err)
	}
	if comparisonErr := sameResumeProof(proof, fresh); comparisonErr != nil {
		return fmt.Errorf("PR #%d integration proof changed; no mutation performed: %w", proof.pr, comparisonErr)
	}
	if worktreeErr := a.prepareResumeLocalIntegration(fresh); worktreeErr != nil {
		return worktreeErr
	}
	integrated, _, err := a.claimRenewalIntegrationState(fresh.root, fresh.localHead, fresh.renewalHead, fresh.issue)
	if err != nil {
		return err
	}
	if !integrated {
		if err := a.advanceResumeLocalIntegration(fresh); err != nil {
			return err
		}
	}
	if err := a.verifyResumePush(fresh); err != nil {
		return fmt.Errorf("verify remote renewal after local integration: %w", err)
	}
	return a.finishPullRequestResume(fresh)
}

func (a app) prepareResumeLocalIntegration(proof resumeProof) error {
	if !proof.already {
		return stateError("PR #%d has no remote renewal marker to integrate; run pr resume without --integrate first", proof.pr)
	}
	if proof.renewalExpired {
		return stateError("PR #%d remote renewal expired; run pr resume without --integrate using original expected head before local integration", proof.pr)
	}
	return a.validateResumeIntegrationWorktree(proof.root)
}

func (a app) advanceResumeLocalIntegration(proof resumeProof) error {
	tree, err := a.command(proof.root, "git", "rev-parse", proof.localHead+"^{tree}")
	if err != nil {
		return fmt.Errorf("read preserved local tree: %w", err)
	}
	tree = strings.TrimSpace(tree)
	message := fmt.Sprintf("chore(workflow): integrate claim renewal #%d\n", proof.issue)
	commit, err := a.commandInput(proof.root, strings.NewReader(message), "git", "commit-tree", tree,
		"-p", proof.localHead, "-p", proof.renewalHead)
	if err != nil {
		return fmt.Errorf("create local renewal integration: %w", err)
	}
	adopted, err := a.readAuthoritativeClaimMarker(proof.root, commit, proof.issue)
	if err != nil {
		return fmt.Errorf("prove local renewal integration before ref update: %w", err)
	}
	if adopted.head != proof.renewalHead {
		return stateError("local integration %s adopts claim %s, expected %s; no ref mutation performed", commit, adopted.head, proof.renewalHead)
	}
	if _, err := a.command(proof.root, "git", "update-ref", "refs/heads/"+proof.localBranch, commit, proof.localHead); err != nil {
		return fmt.Errorf("advance local integration with expected head %s: %w", proof.localHead, err)
	}
	return nil
}

func (a app) validateResumeIntegrationWorktree(root string) error {
	if err := a.validateResumeOperationState(root); err != nil {
		return err
	}
	status, err := a.command(root, "git", "--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return fmt.Errorf("inspect local integration worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return stateError("local integration has staged, unstaged, or untracked work; commit or resolve it before continuing")
	}
	return nil
}

func (a app) validateResumeOperationState(root string) error {
	for _, state := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "REBASE_HEAD", "rebase-apply", "rebase-merge", "sequencer"} {
		path, err := a.command(root, "git", "rev-parse", "--git-path", state)
		if err != nil {
			return fmt.Errorf("locate %s state: %w", state, err)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		_, statErr := os.Stat(path)
		if statErr == nil {
			return stateError("local %s operation is unfinished; complete it before continuing", state)
		}
		if !os.IsNotExist(statErr) {
			return fmt.Errorf("inspect %s state: %w", state, statErr)
		}
	}
	return nil
}

func (a app) finishPullRequestResume(proof resumeProof) error {
	if err := a.validateResumeIntegrationWorktree(proof.root); err != nil {
		return err
	}
	local, err := a.command(proof.root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read integrated claim head: %w", err)
	}
	integrated, _, err := a.claimRenewalIntegrationState(proof.root, local, proof.renewalHead, proof.issue)
	if err != nil {
		return err
	}
	if !integrated {
		return stateError("local head %s does not include remote renewal %s", local, proof.renewalHead)
	}
	claim, err := a.readPRResumeRenewalChain(proof.root, proof.renewalHead, proof.expectedHead, proof.issue, proof.runID)
	if err != nil {
		return err
	}
	if deadlineErr := validateClaimDeadline(proof.issue, claim.lease, time.Now().UTC()); deadlineErr != nil {
		deadlineErr = retryableOperationIfRecoverable("PR resume claim verification", deadlineErr)
		return fmt.Errorf("PR #%d local renewal marker is integrated, but the claim lease expired before status reconciliation: %w", proof.pr, deadlineErr)
	}
	status, err := a.readIssueStatus(proof.root, proof.issue)
	if err != nil {
		err = retryableOperationIfRecoverable("PR resume label status", err)
		return fmt.Errorf("PR #%d label reconciliation failed: %w. "+resumeIntegrationRecoveryTemplate, proof.pr, err, proof.pr, proof.expectedHead)
	}
	if issueNeedsHuman(status) {
		if _, err := a.command(proof.root, "gh", "issue", "edit", strconv.Itoa(proof.issue), "--repo", repositoryKey,
			"--remove-label", "needs-human"); err != nil {
			err = retryableOperationIfRecoverable("PR resume label mutation", err)
			return fmt.Errorf("PR #%d label reconciliation failed: %w. "+resumeIntegrationRecoveryTemplate, proof.pr, err,
				proof.pr, proof.expectedHead)
		}
	}
	if err := a.setIssueProjectStatus(proof.root, proof.issue, "Picked"); err != nil {
		err = retryableOperationIfRecoverable("PR resume Project reconciliation", err)
		return fmt.Errorf("PR #%d Project reconciliation failed: %w. "+resumeIntegrationRecoveryTemplate, proof.pr, err,
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
	if status.State != "OPEN" || (!issueNeedsHuman(status) && !fresh.priorIntegrated) {
		return resumeProof{}, stateError("issue #%d must remain open and labeled needs-human immediately before PR #%d resume mutation; no mutation performed. "+resumeRecoveryTemplate,
			fresh.issue, fresh.pr, fresh.pr, fresh.expectedHead)
	}
	commit, renewalLease, _, createErr := a.newClaimCommitWithRunID(fresh.root, fresh.issue, fresh.observedHead, fresh.runID)
	if createErr != nil {
		return resumeProof{}, retryableOperationIfRecoverable("PR resume renewal commit", createErr)
	}
	expectedClaim, claimErr := a.readResumeExpectedClaim(fresh.root, fresh.expectedHead, fresh.issue)
	if claimErr != nil {
		return resumeProof{}, fmt.Errorf("prove expected claim lease before PR renewal push: %w", claimErr)
	}
	remoteClaim := expectedClaim
	if fresh.already {
		remoteClaim, claimErr = a.readPRResumeRenewalChain(fresh.root, fresh.observedHead, fresh.expectedHead, fresh.issue, fresh.runID)
		if claimErr != nil {
			return resumeProof{}, fmt.Errorf("prove remote claim lease before PR renewal push: %w", claimErr)
		}
	}
	localClaim, claimErr := a.readResumeLocalAuthority(fresh.root, fresh.localHead, fresh.issue, expectedClaim)
	if claimErr != nil {
		return resumeProof{}, fmt.Errorf("prove local claim lease before PR renewal push: %w", claimErr)
	}
	if !renewalLease.After(remoteClaim.lease) || renewalLease.Before(localClaim.lease) {
		return resumeProof{}, stateError("new PR renewal lease %s does not advance remote claim lease %s or cover local claim lease %s; no remote mutation performed",
			renewalLease.Format(time.RFC3339), remoteClaim.lease.Format(time.RFC3339), localClaim.lease.Format(time.RFC3339))
	}
	lease := "--force-with-lease=refs/heads/" + claimBranch(fresh.issue) + ":" + fresh.observedHead
	refspec := commit + ":refs/heads/" + claimBranch(fresh.issue)
	if _, pushErr := a.command(fresh.root, "git", "push", lease, "origin", refspec); pushErr != nil {
		pushErr = retryableOperationIfRecoverable("PR resume claim push", pushErr)
		candidate := fresh
		candidate.renewalHead = commit
		if verifyErr := a.verifyResumePush(candidate); verifyErr != nil {
			return resumeProof{}, fmt.Errorf("PR #%d claim push response was ambiguous: %w; reconciliation: %v. "+resumeRecoveryTemplate,
				proof.pr, pushErr, verifyErr, proof.pr, proof.expectedHead)
		}
	}
	fresh.renewalHead = commit
	fresh.already = true
	fresh.pending = true
	fresh.renewalExpired = false
	return fresh, nil
}

func sameResumeProof(before, after resumeProof) error {
	if before.root != after.root || before.localBranch != after.localBranch || before.issue != after.issue ||
		before.pr != after.pr || before.expectedHead != after.expectedHead || before.observedHead != after.observedHead ||
		before.renewalHead != after.renewalHead || before.localHead != after.localHead || before.runID != after.runID ||
		before.runLocalHead != after.runLocalHead ||
		before.runLocalPresent != after.runLocalPresent || before.already != after.already || before.pending != after.pending ||
		before.renewalExpired != after.renewalExpired || before.priorIntegrated != after.priorIntegrated ||
		before.needsHuman != after.needsHuman || before.projectStatus != after.projectStatus {
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
	claim, err := a.readPRResumeRenewalChain(proof.root, proof.renewalHead, proof.expectedHead, proof.issue, proof.runID)
	if err != nil {
		return err
	}
	return validateClaimDeadline(proof.issue, claim.lease, time.Now().UTC())
}
