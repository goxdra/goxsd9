package workflowctl

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// publishClaim advances the fixed claim branch from the current run head.
//
//nolint:gocognit // Publication keeps the pre-push authority and post-push observations in one ordered boundary.
func (a app) publishClaim() error {
	root, localBranch, issue, err := a.currentClaim()
	if err != nil {
		return err
	}
	branch := claimBranch(issue)
	if checkErr := a.verifyPublicationOrigin(root); checkErr != nil {
		return checkErr
	}
	if checkErr := a.verifyPublicationSource(root, ""); checkErr != nil {
		return checkErr
	}
	if checkErr := a.verifyClaimForPush(root, localBranch, issue); checkErr != nil {
		return checkErr
	}
	lease, runID, err := a.readClaimMetadata(root, issue, localBranch)
	if err != nil {
		return fmt.Errorf("read publication claim: %w", err)
	}
	if checkErr := validateClaimLocalBranch(localBranch, issue, runID); checkErr != nil {
		return checkErr
	}
	if checkErr := validateClaimDeadline(issue, lease, time.Now().UTC()); checkErr != nil {
		return checkErr
	}
	local, remote, err := a.claimHeads(root, branch)
	if err != nil {
		return fmt.Errorf("read publication heads: %w", err)
	}
	if !validExactCommitSHA(local) || !validExactCommitSHA(remote) {
		return stateError("publication heads must be full commit SHAs; local=%q remote=%q", local, remote)
	}
	if checkErr := a.verifyPublicationRemoteRun(root, branch, issue, runID, remote); checkErr != nil {
		return checkErr
	}
	if _, ancestryErr := a.command(root, "git", "merge-base", "--is-ancestor", remote, local); ancestryErr != nil {
		if isGitNonAncestor(ancestryErr) {
			return stateError("fixed claim branch %s diverged; local=%s remote=%s", branch, local, remote)
		}
		return fmt.Errorf("prove fixed claim ancestry: %w", ancestryErr)
	}
	previousPR, hadPR, err := a.verifiedOpenPRBeforePublication(root, branch, issue)
	if err != nil {
		return err
	}
	if err := a.verifyPublicationRemoteHead(root, branch, remote); err != nil {
		return err
	}
	if err := a.verifyPublicationSource(root, local); err != nil {
		return err
	}
	if local != remote {
		ref := "refs/heads/" + branch
		if _, err := a.command(root, "git", "push", "origin", local+":"+ref); err != nil {
			return stateError("publish fixed claim branch %s from %s to %s: %v", branch, remote, local, err)
		}
	}
	if err := a.verifyPublicationRemoteHead(root, branch, local); err != nil {
		return err
	}
	if hadPR {
		currentPR, found, err := a.lookupOpenPullRequest(root, branch, issue)
		if err != nil {
			return err
		}
		if !found || currentPR.Number != previousPR.Number {
			return stateError("open PR #%d changed identity after publication on claim branch %s", previousPR.Number, branch)
		}
		if _, err := a.observePublishedPRHead(root, branch, issue, previousPR, currentPR, local); err != nil {
			return err
		}
	}
	return writeLine(a.stdout, "published claim #%d %s to %s at %s", issue, localBranch, branch, local)
}

func (a app) verifyPublicationSource(root, expectedHead string) error {
	status, err := a.command(root, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return fmt.Errorf("read publication worktree status: %w", err)
	}
	if status != "" {
		return stateError("claim worktree has uncommitted changes")
	}
	if expectedHead == "" {
		return nil
	}
	current, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read publication source head: %w", err)
	}
	if current != expectedHead {
		return stateError("claim source head moved: expected %s, found %s", expectedHead, current)
	}
	return nil
}

func (a app) verifyPublicationOrigin(root string) error {
	allowed := []string{
		"https://github.com/" + repositoryKey + ".git",
		"https://github.com/" + repositoryKey,
		"git@github.com:" + repositoryKey + ".git",
		"git@github.com:" + repositoryKey,
		"ssh://git@github.com/" + repositoryKey + ".git",
		"ssh://git@github.com/" + repositoryKey,
	}
	for _, args := range [][]string{{"remote", "get-url", "origin"}, {"remote", "get-url", "--push", "--all", "origin"}} {
		remote, err := a.command(root, "git", args...)
		if err != nil {
			return fmt.Errorf("read publication origin: %w", err)
		}
		matched := false
		for _, candidate := range allowed {
			if remote == candidate {
				matched = true
				break
			}
		}
		if !matched {
			return stateError("publication origin %q is not %s", remote, repositoryKey)
		}
	}
	return nil
}

func (a app) verifyPublicationRemoteRun(root, branch string, issue int, runID, remote string) error {
	marker, err := a.command(root, "git", "log", "-1", "--first-parent", "--format=%H", "--fixed-strings",
		"--grep=Agent-Issue: "+strconv.Itoa(issue), "origin/"+branch)
	if err != nil {
		return fmt.Errorf("read fixed claim run: %w", err)
	}
	if !validExactCommitSHA(marker) {
		return stateError("fixed claim branch %s has no unambiguous issue #%d marker", branch, issue)
	}
	identity, err := a.readCanonicalClaimIdentity(root, marker, "")
	if err != nil {
		return fmt.Errorf("read fixed claim marker %s: %w", marker, err)
	}
	if identity.issue != issue || identity.runID != runID {
		return stateError("fixed claim branch %s is owned by issue #%d run %s, not issue #%d run %s", branch,
			identity.issue, identity.runID, issue, runID)
	}
	if _, err := a.command(root, "git", "merge-base", "--is-ancestor", marker, remote); err != nil {
		return stateError("fixed claim marker %s is outside remote head %s: %v", marker, remote, err)
	}
	return nil
}

func (a app) verifyPublicationRemoteHead(root, branch, expected string) error {
	ref := "refs/heads/" + branch
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", ref)
	if err != nil {
		return fmt.Errorf("read fixed claim branch %s: %w", branch, err)
	}
	actual, found, err := exactRemoteRef(output, ref)
	if err != nil {
		return err
	}
	if !found || actual != expected {
		return stateError("fixed claim branch %s moved: expected %s, found %s", branch, expected, strings.TrimSpace(actual))
	}
	return nil
}
