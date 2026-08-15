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
	Number int    `json:"number"`
	URL    string `json:"html_url"`
}

type mergePullRequestRequest struct {
	CommitMessage string `json:"commit_message"`
	CommitTitle   string `json:"commit_title"`
	MergeMethod   string `json:"merge_method"`
	SHA           string `json:"sha"`
}

const sessionSummaryHeading = "## Session summary"

type mergePullRequestResponse struct {
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
	SHA     string `json:"sha"`
}

type pullRequestFinishAction uint8

const (
	finishReplaceDraftREST pullRequestFinishAction = iota + 1
	finishMergeREST
)

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
	if titleErr := validateCommitTitle(*title); titleErr != nil {
		return usageError("pr open: invalid title: %v", titleErr)
	}
	body, err := readPullRequestBody(*bodyFile, issue)
	if err != nil {
		return usageError("pr open: %v", err)
	}
	return a.createPullRequest(issue, *title, body)
}

func readPullRequestBody(path string, issue int) (string, error) {
	if err := requireRegularFile(path); err != nil {
		return "", err
	}
	// #nosec G304 -- path is an explicit operator-supplied input.
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read PR body: %w", err)
	}
	needle := fmt.Sprintf("closes #%d", issue)
	if !strings.Contains(strings.ToLower(string(body)), needle) {
		return "", fmt.Errorf("PR body must contain %q", "Closes #"+strconv.Itoa(issue))
	}
	if _, err := sessionSummary(string(body)); err != nil {
		return "", err
	}
	return string(body), nil
}

func sessionSummary(body string) (string, error) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	start, end, err := sessionSummaryBounds(lines)
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if err := validateSessionSummaryText(summary); err != nil {
		return "", err
	}
	return summary, nil
}

func sessionSummaryBounds(lines []string) (int, int, error) {
	start := -1
	end := len(lines)
	for index, line := range lines {
		if line == sessionSummaryHeading {
			if start >= 0 {
				return 0, 0, fmt.Errorf("PR body contains more than one %q section", sessionSummaryHeading)
			}
			start = index + 1
			continue
		}
		if start >= 0 && end == len(lines) && (strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ")) {
			end = index
		}
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("PR body must contain %q", sessionSummaryHeading)
	}
	return start, end, nil
}

func validateSessionSummaryText(summary string) error {
	if summary == "" {
		return errors.New("PR session summary must not be empty")
	}
	if strings.Contains(summary, "<!--") || strings.Contains(summary, "-->") {
		return errors.New("PR session summary must not contain HTML comments")
	}
	for _, line := range strings.Split(summary, "\n") {
		markdownLine := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, " "), " "), " ")
		if strings.HasPrefix(markdownLine, "#") || strings.HasPrefix(markdownLine, "```") ||
			strings.HasPrefix(markdownLine, "~~~") {
			return errors.New("PR session summary must use plain text, not Markdown headings or fences")
		}
		if containsMarkdownLink(line) || isMarkdownTableDelimiter(line) {
			return errors.New("PR session summary must use plain text, not Markdown links or tables")
		}
		if strings.TrimRight(line, " \t") != line {
			return errors.New("PR session summary lines must not have trailing whitespace")
		}
	}
	return nil
}

func containsMarkdownLink(line string) bool {
	closingBracket := strings.Index(line, "](")
	if closingBracket < 1 {
		return false
	}
	return strings.LastIndex(line[:closingBracket], "[") >= 0
}

func isMarkdownTableDelimiter(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	line = strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(line, "|")
	for _, cell := range cells {
		cell = strings.Trim(strings.TrimSpace(cell), ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func (a app) createPullRequest(issue int, title, body string) error {
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
	if titleErr := a.validateWorkCommitTitles(root, "HEAD"); titleErr != nil {
		return stateError("cannot open PR: %v", titleErr)
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
	url, err := a.createDraftPullRequest(root, branch, title, body)
	if err != nil {
		return err
	}
	return writeLine(a.stdout, "%s", url)
}

func (a app) createDraftPullRequest(root, branch, title, body string) (string, error) {
	request := createPullRequestRequest{Base: "main", Body: body, Draft: true, Head: branch, Title: title}
	response, err := a.submitPullRequest(root, request)
	if err != nil {
		return "", fmt.Errorf("create draft PR: %w", err)
	}
	return response.URL, nil
}

func (a app) submitPullRequest(root string, request createPullRequestRequest) (createPullRequestResponse, error) {
	input, err := json.Marshal(request)
	if err != nil {
		return createPullRequestResponse{}, fmt.Errorf("encode PR: %w", err)
	}
	output, err := a.commandInput(root, strings.NewReader(string(input)), "gh", "api", "--method", "POST",
		"repos/"+repositoryKey+"/pulls", "--input", "-")
	if err != nil {
		return createPullRequestResponse{}, fmt.Errorf("create PR: %w", err)
	}
	var response createPullRequestResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return createPullRequestResponse{}, fmt.Errorf("decode PR: %w", err)
	}
	if response.URL == "" || response.Number < 1 {
		return createPullRequestResponse{}, errors.New("PR response has no URL or number")
	}
	return response, nil
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
	summary, messageErr := validateSquashMessage(view, number)
	if messageErr != nil {
		return messageErr
	}
	if titleErr := a.validateWorkCommitTitles(root, view.HeadRefOID); titleErr != nil {
		return stateError("PR #%d has invalid work commits: %v", number, titleErr)
	}
	if err := a.validateClosingClaims(root, view, claimedIssue); err != nil {
		return err
	}
	passes, evaluationErr := latestEvaluationPasses(view, number)
	if evaluationErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, evaluationErr)
	}
	if !passes {
		return stateError("PR #%d has no passing evaluation for head %s", number, view.HeadRefOID)
	}
	if err := a.requirePassingChecks(root, number, view.HeadRefOID); err != nil {
		return err
	}
	ready := !view.IsDraft
	if view.IsDraft {
		if _, readyErr := a.command(root, "gh", "pr", "ready", strconv.Itoa(number), "--repo", repositoryKey); readyErr == nil {
			ready = true
		}
	}
	switch finishActionFor(view, ready) {
	case finishReplaceDraftREST:
		return a.replaceDraftPullRequest(root, number, view)
	case finishMergeREST:
		return a.mergeReadyPullRequest(root, number, view, summary)
	}
	return stateError("PR #%d has an impossible finish action", number)
}

func validateSquashMessage(view pullRequestView, number int) (string, error) {
	if err := validateCommitTitle(view.Title); err != nil {
		return "", stateError("PR #%d has invalid title %q: %v", number, view.Title, err)
	}
	summary, err := sessionSummary(view.Body)
	if err != nil {
		return "", stateError("PR #%d has invalid session summary: %v", number, err)
	}
	return summary, nil
}

func finishActionFor(view pullRequestView, ready bool) pullRequestFinishAction {
	if view.IsDraft && !ready {
		return finishReplaceDraftREST
	}
	return finishMergeREST
}

func (a app) replaceDraftPullRequest(root string, number int, view pullRequestView) error {
	if err := a.updatePullRequestState(root, number, "closed"); err != nil {
		return err
	}
	request := createPullRequestRequest{
		Base: view.BaseRefName, Body: readyReplacementBody(view.Body, number, view.HeadRefOID), Draft: false,
		Head: view.HeadRefName, Title: view.Title,
	}
	replacement, err := a.submitPullRequest(root, request)
	if err != nil {
		if reopenErr := a.updatePullRequestState(root, number, "open"); reopenErr != nil {
			return errors.Join(fmt.Errorf("create ready replacement: %w", err),
				fmt.Errorf("reopen draft PR #%d: %w", number, reopenErr))
		}
		return fmt.Errorf("create ready replacement; draft PR #%d was reopened: %w", number, err)
	}
	body := fmt.Sprintf("Ready replacement: %s at evaluated head `%s`. It requires a fresh Examiner attestation.\n",
		replacement.URL, view.HeadRefOID)
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return fmt.Errorf("ready PR %s created, but recording replacement failed: %w", replacement.URL, err)
	}
	return stateError("draft PR #%d replaced by ready PR #%d at %s; evaluate the replacement, then finish it",
		number, replacement.Number, replacement.URL)
}

func readyReplacementBody(body string, number int, head string) string {
	return strings.TrimSpace(body) + fmt.Sprintf("\n\n## Ready replacement\n\nReplaces draft PR #%d at identical head `%s`. "+
		"A fresh challenge-bound Examiner attestation is required before merge.\n", number, head)
}

func (a app) updatePullRequestState(root string, number int, state string) error {
	payload, err := json.Marshal(struct {
		State string `json:"state"`
	}{State: state})
	if err != nil {
		return fmt.Errorf("encode PR #%d state: %w", number, err)
	}
	if _, err := a.commandInput(root, strings.NewReader(string(payload)), "gh", "api", "--method", "PATCH",
		"repos/"+repositoryKey+"/pulls/"+strconv.Itoa(number), "--input", "-"); err != nil {
		return fmt.Errorf("set PR #%d state to %s: %w", number, state, err)
	}
	return nil
}

func (a app) mergeReadyPullRequest(root string, number int, view pullRequestView, summary string) error {
	request := mergePullRequestRequest{
		CommitMessage: summary,
		CommitTitle:   view.Title + " (#" + strconv.Itoa(number) + ")",
		MergeMethod:   "squash",
		SHA:           view.HeadRefOID,
	}
	input, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode PR #%d merge: %w", number, err)
	}
	output, err := a.commandInput(root, strings.NewReader(string(input)), "gh", "api", "--method", "PUT",
		"repos/"+repositoryKey+"/pulls/"+strconv.Itoa(number)+"/merge", "--input", "-")
	if err != nil {
		return fmt.Errorf("merge PR #%d at %s: %w", number, view.HeadRefOID, err)
	}
	var response mergePullRequestResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return fmt.Errorf("decode PR #%d merge: %w", number, err)
	}
	if !response.Merged || response.SHA == "" {
		return stateError("PR #%d was not merged: %s", number, response.Message)
	}
	for _, issue := range view.ClosingIssuesReferences {
		if err := a.setIssueProjectStatus(root, issue.Number, "Done"); err != nil {
			if writeErr := writeLine(a.stderr, "PR #%d merged; issue #%d Project sync deferred: %v", number,
				issue.Number, err); writeErr != nil {
				return fmt.Errorf("PR merged; report deferred Project sync: %w", writeErr)
			}
			break
		}
	}
	return writeLine(a.stdout, "PR #%d merged at evaluated head %s as %s", number, view.HeadRefOID, response.SHA)
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
	output, err := a.command(root, "gh", "api", "--paginate", endpoint)
	if err != nil {
		return stateError("read PR #%d checks: %v", number, err)
	}
	pages, err := decodeJSONDocuments[checkRunsAPI](output)
	if err != nil {
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
