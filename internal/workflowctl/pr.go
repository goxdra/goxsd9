package workflowctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type pullRequestCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func (a app) runPR(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl pr open ISSUE [flags] | finish PR")
	}
	switch args[0] {
	case "open":
		return a.openPullRequest(args[1:])
	case "finish":
		if len(args) != 2 {
			return usageError("usage: workflowctl pr finish PR")
		}
		number, err := positiveNumber(args[1])
		if err != nil {
			return usageError("pr finish: %v", err)
		}
		return a.finishPullRequest(number)
	default:
		return usageError("unknown pr command %q", args[0])
	}
}

func (a app) openPullRequest(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl pr open ISSUE --title TITLE --body-file FILE")
	}
	issue, err := positiveNumber(args[0])
	if err != nil {
		return usageError("pr open: %v", err)
	}
	flags := flag.NewFlagSet("pr open", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	title := flags.String("title", "", "pull request title")
	bodyFile := flags.String("body-file", "", "pull request body")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("pr open: %v", parseErr)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*title) == "" || *bodyFile == "" {
		return usageError("usage: workflowctl pr open ISSUE --title TITLE --body-file FILE")
	}
	if err := validatePullRequestBody(*bodyFile, issue); err != nil {
		return usageError("pr open: %v", err)
	}
	return a.createPullRequest(issue, *title, *bodyFile)
}

func validatePullRequestBody(path string, issue int) error {
	if err := requireRegularFile(path); err != nil {
		return err
	}
	// #nosec G304 -- path is an explicit operator-supplied input.
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	needle := fmt.Sprintf("closes #%d", issue)
	if !strings.Contains(strings.ToLower(string(body)), needle) {
		return fmt.Errorf("PR body must contain %q", "Closes #"+strconv.Itoa(issue))
	}
	return nil
}

func (a app) createPullRequest(issue int, title, bodyFile string) error {
	root, branch, claimedIssue, err := a.currentClaim()
	if err != nil {
		return err
	}
	if claimedIssue != issue {
		return stateError("branch %s claims issue #%d, not #%d", branch, claimedIssue, issue)
	}
	clean, err := a.command(root, "git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("read worktree status: %w", err)
	}
	if clean != "" {
		return stateError("worktree has uncommitted changes")
	}
	if verifyErr := a.verifyClaimForPush(root, branch, claimedIssue); verifyErr != nil {
		return verifyErr
	}
	if _, pushErr := a.command(root, "git", "push", "origin", "HEAD:refs/heads/"+branch); pushErr != nil {
		return fmt.Errorf("push pull request branch: %w", pushErr)
	}
	if verifyErr := a.verifyClaim(); verifyErr != nil {
		return verifyErr
	}
	output, err := a.command(root, "gh", "pr", "create", "--repo", repositoryKey, "--draft", "--base", "main",
		"--head", branch, "--title", title, "--body-file", bodyFile)
	if err != nil {
		return fmt.Errorf("create draft PR: %w", err)
	}
	return writeLine(a.stdout, "%s", firstLine(output))
}

func (a app) finishPullRequest(number int) error {
	root, branch, claimedIssue, err := a.currentClaim()
	if err != nil {
		return err
	}
	if verifyErr := a.verifyClaim(); verifyErr != nil {
		return verifyErr
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return err
	}
	if view.State != "OPEN" {
		return stateError("PR #%d is %s", number, view.State)
	}
	if view.HeadRefName != branch {
		return stateError("PR #%d uses branch %s, not claim branch %s", number, view.HeadRefName, branch)
	}
	if !pullRequestCloses(view, claimedIssue) {
		return stateError("PR #%d does not close claimed issue #%d", number, claimedIssue)
	}
	if !latestEvaluationPasses(view) {
		return stateError("PR #%d has no passing evaluation for head %s", number, view.HeadRefOID)
	}
	if err := a.requirePassingChecks(root, number); err != nil {
		return err
	}
	if view.IsDraft {
		if _, err := a.command(root, "gh", "pr", "ready", strconv.Itoa(number), "--repo", repositoryKey); err != nil {
			return fmt.Errorf("mark PR #%d ready: %w", number, err)
		}
	}
	if _, err := a.command(root, "gh", "pr", "merge", strconv.Itoa(number), "--repo", repositoryKey, "--squash",
		"--match-head-commit", view.HeadRefOID); err != nil {
		return fmt.Errorf("merge PR #%d: %w", number, err)
	}
	for _, issue := range view.ClosingIssuesReferences {
		if err := a.setIssueProjectStatus(root, issue.Number, "Done"); err != nil {
			return fmt.Errorf("PR merged but issue #%d Project update failed: %w", issue.Number, err)
		}
	}
	return writeLine(a.stdout, "PR #%d merged at evaluated head %s", number, view.HeadRefOID)
}

func pullRequestCloses(view pullRequestView, number int) bool {
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == number {
			return true
		}
	}
	return false
}

func (a app) requirePassingChecks(root string, number int) error {
	output, err := a.command(root, "gh", "pr", "checks", strconv.Itoa(number), "--repo", repositoryKey,
		"--json", "name,state")
	if err != nil {
		return stateError("read PR #%d checks: %v", number, err)
	}
	var checks []pullRequestCheck
	if err := json.Unmarshal([]byte(output), &checks); err != nil {
		return fmt.Errorf("decode PR #%d checks: %w", number, err)
	}
	if len(checks) == 0 {
		return stateError("PR #%d has no reported checks", number)
	}
	for _, check := range checks {
		if check.State != "SUCCESS" && check.State != "SKIPPED" && check.State != "NEUTRAL" {
			return stateError("PR #%d check %q is %s", number, check.Name, check.State)
		}
	}
	return nil
}
