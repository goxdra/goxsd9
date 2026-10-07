package workflowctl

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// readIntegratedClaimMetadata derives authority from the exact second-parent
// marker, never from trailers in an unpublished source commit.
//
//nolint:gocognit // The marker, integration, and source ancestry form one ordered authority proof.
func (a app) readIntegratedClaimMetadata(root string) (string, time.Time, string, bool, error) {
	head, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", time.Time{}, "", false, fmt.Errorf("read integrated claim head: %w", err)
	}
	history, err := a.readClaimResumeFirstParentHistory(root, head)
	if err != nil {
		return "", time.Time{}, "", false, err
	}
	var newerMarker string
	for _, entry := range history {
		if claimResumeLooksLikeMarker(entry.object.message) && newerMarker == "" {
			newerMarker = entry.head
		}
		if !strings.HasPrefix(entry.object.message, "chore(workflow): integrate renewed claim #") {
			continue
		}
		if len(entry.object.parents) != 2 {
			return "", time.Time{}, "", false, stateError("claim integration %s has noncanonical parent count", entry.head)
		}
		marker := entry.object.parents[1]
		canonical, err := a.readCanonicalClaimIdentity(root, marker, "")
		if err != nil {
			return "", time.Time{}, "", false, fmt.Errorf("read integrated renewal marker: %w", err)
		}
		if err := a.validateClaimResumeIntegration(root, entry.head, entry.object.parents[0], marker,
			canonical.parent, canonical.issue, canonical.runID); err != nil {
			return "", time.Time{}, "", false, err
		}
		if err := a.validateUnpublishedClaimAncestry(root, canonical.parent, entry.object.parents[0]); err != nil {
			return "", time.Time{}, "", false, err
		}
		if newerMarker != "" {
			latest, err := a.readCanonicalClaimIdentity(root, newerMarker, "")
			if err != nil {
				return "", time.Time{}, "", false, fmt.Errorf("read renewed integrated claim marker: %w", err)
			}
			if latest.issue != canonical.issue || latest.runID != canonical.runID || !latest.lease.After(canonical.lease) {
				return "", time.Time{}, "", false, stateError("renewed integrated claim marker %s has conflicting issue, run, or lease", newerMarker)
			}
			return newerMarker, latest.lease, latest.runID, true, nil
		}
		return marker, canonical.lease, canonical.runID, true, nil
	}
	return "", time.Time{}, "", false, nil
}

func claimResumeIntegrationMessage(issue int) string {
	return fmt.Sprintf("chore(workflow): integrate renewed claim #%d\n", issue)
}

func (a app) validateClaimResumeIntegration(root, head, source, marker, anchor string, issue int, runID string) error {
	if !validExactCommitSHA(head) || !validExactCommitSHA(source) || !validExactCommitSHA(marker) {
		return stateError("claim integration has malformed commit identity")
	}
	object, err := a.gitRaw(root, "cat-file", "commit", head)
	if err != nil {
		return fmt.Errorf("read local claim integration %s: %w", head, err)
	}
	commit, err := parseCommitObject(object)
	if err != nil {
		return stateError("local claim integration %s is malformed: %w", head, err)
	}
	if len(commit.parents) != 2 || commit.parents[0] != source || commit.parents[1] != marker ||
		commit.message != claimResumeIntegrationMessage(issue) {
		return stateError("local claim head %s is not the canonical integration of source %s and renewal %s", head, source, marker)
	}
	sourceTree, err := a.command(root, "git", "rev-parse", source+"^{tree}")
	if err != nil {
		return fmt.Errorf("read preserved source tree: %w", err)
	}
	if commit.tree != strings.TrimSpace(sourceTree) {
		return stateError("local integration %s changed the preserved source tree", head)
	}
	return a.validateExistingResumeCommit(root, marker, anchor, issue, runID)
}

func (a app) readFreshUnpublishedResume(proof claimResumeProof) error {
	plan, ok := proof.renewal.(claimResumeUnpublished)
	if !ok {
		return stateError("claim proof has no unpublished source phase")
	}
	fresh, err := a.readClaimResumeProof(proof.preflight.issue, proof.preflight.expectedHead,
		proof.preflight.runID, proof.preflight.handoffCommentID, plan.source)
	if err != nil {
		return err
	}
	if err := sameClaimResumeProof(proof, fresh); err != nil {
		return stateError("unpublished claim proof changed before mutation: %w", err)
	}
	return nil
}

// applyUnpublishedClaimResume publishes only the empty renewal marker. Local
// source integration is an explicit later phase, including for resolved merges.
//
//nolint:gocognit // Each phase rereads the sealed proof before the next mutation.
func (a app) applyUnpublishedClaimResume(proof claimResumeProof, integrate bool) error {
	plan, ok := proof.renewal.(claimResumeUnpublished)
	if !ok {
		return stateError("claim proof has no unpublished source phase")
	}
	marker := plan.head
	if marker == "" {
		created, err := a.createClaimResumeRenewal(claimResumeProof{preflight: proof.preflight, renewal: claimResumeNoRenewal{}})
		if err != nil {
			return err
		}
		marker = created.head
		if err := a.readFreshUnpublishedResume(proof); err != nil {
			return claimResumeProofFailure(proof, "source proof changed before renewal push", err)
		}
		if _, err := a.pushClaimResumeRenewal(proof, claimResumeLocalRenewal{head: marker}); err != nil {
			return err
		}
	}
	fresh, err := a.readClaimResumeProof(proof.preflight.issue, proof.preflight.expectedHead,
		proof.preflight.runID, proof.preflight.handoffCommentID, plan.source)
	if err != nil {
		return claimResumeProofFailure(proof, "remote renewal needs reconciliation", err)
	}
	remotePlan, ok := fresh.renewal.(claimResumeUnpublished)
	if !ok || remotePlan.head != marker || remotePlan.source != plan.source {
		return stateError("remote renewal does not match the original anchor and preserved source; no integration performed")
	}
	if !integrate && fresh.preflight.localHead == plan.source {
		return writeLine(a.stdout, "issue #%d remote claim renewed at %s; source remains unpublished at %s; rerun with --integrate to restore local push ancestry", proof.preflight.issue, marker, plan.source)
	}
	if fresh.preflight.localHead == plan.source {
		if integrateErr := a.integrateUnpublishedClaim(fresh, marker); integrateErr != nil {
			return integrateErr
		}
	}
	verified, err := a.readClaimResumeProof(proof.preflight.issue, proof.preflight.expectedHead,
		proof.preflight.runID, proof.preflight.handoffCommentID, plan.source)
	if err != nil {
		return claimResumeProofFailure(proof, "local integration needs reconciliation", err)
	}
	if verified.preflight.localHead == plan.source {
		return stateError("local source was not integrated with renewed claim marker")
	}
	if err := a.reconcileClaimResumeIssue(verified, claimResumeRenewalResult{head: marker}); err != nil {
		return err
	}
	return writeLine(a.stdout, "issue #%d claim resumed; source preserved locally, renewal integrated, needs-human removed, Project Picked", proof.preflight.issue)
}

func (a app) integrateUnpublishedClaim(proof claimResumeProof, marker string) error {
	plan, ok := proof.renewal.(claimResumeUnpublished)
	if !ok {
		return stateError("claim proof has no unpublished source phase")
	}
	if plan.head != marker || proof.preflight.localHead != plan.source {
		return stateError("local integration requires the exact unpublished source and remote marker")
	}
	if err := a.readFreshUnpublishedResume(proof); err != nil {
		return claimResumeProofFailure(proof, "source proof changed before local integration", err)
	}
	tree, err := a.command(proof.preflight.root, "git", "rev-parse", plan.source+"^{tree}")
	if err != nil {
		return fmt.Errorf("read unpublished source tree: %w", err)
	}
	tree = strings.TrimSpace(tree)
	message := claimResumeIntegrationMessage(proof.preflight.issue)
	commit, err := a.commandInput(proof.preflight.root, strings.NewReader(message), "git", "commit-tree", tree,
		"-p", plan.source, "-p", marker)
	if err != nil {
		return claimResumeRetry(proof, "local integration commit", err)
	}
	if err := a.validateClaimResumeIntegration(proof.preflight.root, commit, plan.source, marker, proof.preflight.expectedHead,
		proof.preflight.issue, proof.preflight.runID); err != nil {
		return err
	}
	if err := a.readFreshUnpublishedResume(proof); err != nil {
		return claimResumeProofFailure(proof, "source proof changed before integration CAS", err)
	}
	ref := "refs/heads/" + proof.preflight.localBranch
	_, updateErr := a.command(proof.preflight.root, "git", "update-ref", ref, commit, plan.source)
	if updateErr != nil {
		observed, readErr := a.readClaimResumeLocalHead(proof.preflight.root, proof.preflight.localBranch)
		if readErr != nil {
			return claimResumeRetry(proof, "local integration CAS", errors.Join(updateErr, readErr))
		}
		if observed != commit {
			return stateError("local claim head moved during integration CAS: %s; preserve artifacts: %w", observed, updateErr)
		}
	}
	if err := a.validateClaimResumeIntegration(proof.preflight.root, commit, plan.source, marker, proof.preflight.expectedHead,
		proof.preflight.issue, proof.preflight.runID); err != nil {
		return err
	}
	return a.verifyClaimResumeLocalState(proof)
}
