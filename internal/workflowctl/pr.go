package workflowctl

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type pullRequestCheck struct {
	Conclusion string `json:"conclusion"`
	Name       string `json:"name"`
	Status     string `json:"status"`
}

type checkRunsAPI struct {
	CheckRuns []pullRequestCheck `json:"check_runs"`
}

type createPullRequestRequest struct {
	Base  string `json:"base"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
	Head  string `json:"head"`
	Title string `json:"title"`
}

type createPullRequestResponse struct {
	URL string `json:"html_url"`
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
	url, err := a.createDraftPullRequest(root, branch, title, bodyFile)
	if err != nil {
		return err
	}
	return writeLine(a.stdout, "%s", url)
}

func (a app) createDraftPullRequest(root, branch, title, bodyFile string) (string, error) {
	// #nosec G304 -- bodyFile is an explicit operator-supplied input.
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		return "", fmt.Errorf("read PR body: %w", err)
	}
	request := createPullRequestRequest{Base: "main", Body: string(body), Draft: true, Head: branch, Title: title}
	input, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode draft PR: %w", err)
	}
	output, err := a.commandInput(root, strings.NewReader(string(input)), "gh", "api", "--method", "POST",
		"repos/"+repositoryKey+"/pulls", "--input", "-")
	if err != nil {
		return "", fmt.Errorf("create draft PR: %w", err)
	}
	var response createPullRequestResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return "", fmt.Errorf("decode draft PR: %w", err)
	}
	if response.URL == "" {
		return "", errors.New("draft PR response has no URL")
	}
	return response.URL, nil
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
	if err := a.validateClosingClaims(root, view, claimedIssue); err != nil {
		return err
	}
	if !latestEvaluationPasses(view) {
		return stateError("PR #%d has no passing evaluation for head %s", number, view.HeadRefOID)
	}
	if err := a.requirePassingChecks(root, number, view.HeadRefOID); err != nil {
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

func (a app) validateClosingClaims(root string, view pullRequestView, primary int) error {
	if !pullRequestCloses(view, primary) {
		return stateError("PR does not close claimed issue #%d", primary)
	}
	if len(view.ClosingIssuesReferences) > 2 {
		return stateError("PR closes %d issues; a work packet permits one primary and one companion",
			len(view.ClosingIssuesReferences))
	}
	if len(view.ClosingIssuesReferences) == 1 {
		return nil
	}
	claims, err := a.listRemoteClaims(root)
	if err != nil {
		return err
	}
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == primary {
			continue
		}
		if err := a.validateCompanionClaim(root, issue.Number, view.HeadRefOID, claims); err != nil {
			return err
		}
	}
	return nil
}

func (a app) validateCompanionClaim(root string, number int, head string, claims []remoteClaim) error {
	for _, claim := range claims {
		if claim.number != number || !claim.active {
			continue
		}
		if _, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head); err != nil {
			return stateError("companion issue #%d claim %s is not included in evaluated head %s", number,
				claim.branch, head)
		}
		return nil
	}
	return stateError("companion issue #%d has no active claim", number)
}

func (a app) requirePassingChecks(root string, number int, head string) error {
	endpoint := "repos/" + repositoryKey + "/commits/" + head + "/check-runs?per_page=100"
	output, err := a.command(root, "gh", "api", "--paginate", "--slurp", endpoint)
	if err != nil {
		return stateError("read PR #%d checks: %v", number, err)
	}
	var pages []checkRunsAPI
	if err := json.Unmarshal([]byte(output), &pages); err != nil {
		return fmt.Errorf("decode PR #%d checks: %w", number, err)
	}
	if err := requireQualityCheck(pages); err != nil {
		return stateError("PR #%d: %v", number, err)
	}
	return nil
}

func requireQualityCheck(pages []checkRunsAPI) error {
	for _, page := range pages {
		for _, check := range page.CheckRuns {
			if check.Name != "quality" {
				continue
			}
			if check.Status != "completed" || check.Conclusion != "success" {
				return fmt.Errorf("required check %q is %s/%s", check.Name, check.Status, check.Conclusion)
			}
			return nil
		}
	}
	return errors.New("required check \"quality\" is missing")
}
