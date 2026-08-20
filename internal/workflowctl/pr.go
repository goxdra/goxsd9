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
	"unicode"
	"unicode/utf8"
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

const sessionSummaryLimit = 8 * 1024

type mergePullRequestResponse struct {
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
	SHA     string `json:"sha"`
}

type pullRequestFinishAction uint8

type squashSummary string

const (
	finishReplaceDraftREST pullRequestFinishAction = iota + 1
	finishMergeREST
)

func (a app) runPR(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl pr open ISSUE [flags] | finish PR --summary-file FILE | recover PR")
	}
	switch args[0] {
	case "open":
		return a.openPullRequest(args[1:])
	case "finish":
		return a.finishPullRequestCommand(args[1:])
	case "recover":
		return a.recoverPullRequestCommand(args[1:])
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
	return string(body), nil
}

func (a app) createPullRequest(issue int, title, body string) error {
	root, localBranch, claimedIssue, err := a.currentClaim()
	if err != nil {
		return err
	}
	branch := claimBranch(claimedIssue)
	if claimedIssue != issue {
		return stateError("branch %s claims issue #%d, not #%d", localBranch, claimedIssue, issue)
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
	if verifyErr := a.verifyClaimForPush(root, localBranch, claimedIssue); verifyErr != nil {
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

func (a app) finishPullRequestCommand(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl pr finish PR --summary-file FILE")
	}
	number, err := positiveNumber(args[0])
	if err != nil {
		return usageError("pr finish: %v", err)
	}
	flags := flag.NewFlagSet("pr finish", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	summaryFile := flags.String("summary-file", "", "plain-text squash summary")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("pr finish: %v", parseErr)
	}
	if flags.NArg() != 0 || *summaryFile == "" {
		return usageError("usage: workflowctl pr finish PR --summary-file FILE")
	}
	summary, err := readSessionSummary(*summaryFile)
	if err != nil {
		return usageError("pr finish: %v", err)
	}
	return a.finishPullRequest(number, summary)
}

func readSessionSummary(path string) (squashSummary, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect summary path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("summary file must be a regular file")
	}
	// #nosec G304 -- path is an explicit operator-supplied input.
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open summary: %w", err)
	}
	content, readErr := readBoundedRegularSummary(file)
	closeErr := file.Close()
	if readErr != nil {
		return "", errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close summary: %w", closeErr)
	}
	return validateSessionSummary(content)
}

func readBoundedRegularSummary(file *os.File) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect summary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("summary file must be a regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, sessionSummaryLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read summary: %w", err)
	}
	return content, nil
}

func validateSessionSummary(content []byte) (squashSummary, error) {
	if len(content) == 0 {
		return "", errors.New("summary file must not be empty")
	}
	if len(content) > sessionSummaryLimit {
		return "", fmt.Errorf("summary file exceeds %d bytes", sessionSummaryLimit)
	}
	if !utf8.Valid(content) {
		return "", errors.New("summary file must be valid UTF-8")
	}
	summary := strings.TrimSuffix(string(content), "\n")
	if summary == "" {
		return "", errors.New("summary file must not be empty")
	}
	if strings.TrimSpace(summary) != summary {
		return "", errors.New("summary file must not start or end with whitespace")
	}
	for _, value := range summary {
		if isDisallowedSummaryRune(value) {
			return "", errors.New("summary file must contain plain text with Unix line endings")
		}
	}
	if err := validateSummaryLines(summary); err != nil {
		return "", err
	}
	return squashSummary(summary), nil
}

func isDisallowedSummaryRune(value rune) bool {
	if value == '\n' {
		return false
	}
	return unicode.IsControl(value) || unicode.In(value, unicode.Cf, unicode.Zl, unicode.Zp)
}

func validateSummaryLines(summary string) error {
	for _, line := range strings.Split(summary, "\n") {
		if strings.TrimRightFunc(line, unicode.IsSpace) != line {
			return errors.New("summary lines must not have trailing whitespace")
		}
		if isClaimMetadata(line) {
			return errors.New("summary must not contain claim metadata")
		}
	}
	return nil
}

func isClaimMetadata(line string) bool {
	line = strings.TrimLeftFunc(line, unicode.IsSpace)
	return strings.HasPrefix(line, "Agent-Persona:") || strings.HasPrefix(line, "Agent-Run-ID:") ||
		strings.HasPrefix(line, "Agent-Lease-Until:") || strings.HasPrefix(line, "Agent-Issue:")
}

func (a app) finishPullRequest(number int, summary squashSummary) error {
	root, _, claimedIssue, err := a.currentClaim()
	if err != nil {
		return err
	}
	branch := claimBranch(claimedIssue)
	if verifyErr := a.verifyClaim(); verifyErr != nil {
		return verifyErr
	}
	view, err := a.validateFinishPullRequest(root, branch, claimedIssue, number)
	if err != nil {
		return err
	}
	ready := !view.IsDraft
	if view.IsDraft {
		if _, readyErr := a.command(root, "gh", "pr", "ready", strconv.Itoa(number), "--repo", repositoryKey); readyErr == nil {
			ready = true
		}
	}
	action := finishActionFor(view, ready)
	switch action {
	case finishReplaceDraftREST:
		return a.replaceDraftPullRequest(root, number, view)
	case finishMergeREST:
		layout, err := a.repositoryLayout(root)
		if err != nil {
			return err
		}
		plan, err := a.prepareCleanupPlan(root, layout, view, claimedIssue, number)
		if err != nil {
			return err
		}
		return a.mergeReadyPullRequest(root, number, view, summary, plan)
	}
	return stateError("PR #%d has an impossible finish action", number)
}

func (a app) validateFinishPullRequest(root, branch string, claimedIssue, number int) (pullRequestView, error) {
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return pullRequestView{}, err
	}
	if view.State != "OPEN" {
		return pullRequestView{}, stateError("PR #%d is %s", number, view.State)
	}
	if view.HeadRefName != branch {
		return pullRequestView{}, stateError("PR #%d uses branch %s, not claim branch %s", number, view.HeadRefName, branch)
	}
	if view.BaseRefName != "main" {
		return pullRequestView{}, stateError("PR #%d targets base %q, not main", number, view.BaseRefName)
	}
	if titleErr := validateCommitTitle(view.Title); titleErr != nil {
		return pullRequestView{}, stateError("PR #%d has invalid title %q: %v", number, view.Title, titleErr)
	}
	if titleErr := a.validateWorkCommitTitles(root, view.HeadRefOID); titleErr != nil {
		return pullRequestView{}, stateError("PR #%d has invalid work commits: %v", number, titleErr)
	}
	if err := a.validateClosingClaims(root, view, claimedIssue); err != nil {
		return pullRequestView{}, err
	}
	passes, evaluationErr := latestEvaluationPasses(view, number)
	if evaluationErr != nil {
		return pullRequestView{}, stateError("PR #%d has invalid evaluation history: %v", number, evaluationErr)
	}
	if !passes {
		return pullRequestView{}, stateError("PR #%d has no passing evaluation for head %s", number, view.HeadRefOID)
	}
	if _, proofErr := latestPassingEvaluationReceipt(view, number); proofErr != nil {
		return pullRequestView{}, stateError("PR #%d evaluation is not bound to current metadata: %v", number, proofErr)
	}
	if err := a.requirePassingChecks(root, number, view.HeadRefOID); err != nil {
		return pullRequestView{}, err
	}
	return view, nil
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

func (a app) mergeReadyPullRequest(root string, number int, view pullRequestView, summary squashSummary, plan cleanupPlan) error {
	request := mergePullRequestRequest{
		CommitMessage: string(summary),
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
		return a.reconcileMergeOutcome(root, number, view, plan,
			fmt.Errorf("merge PR #%d at %s: %w", number, view.HeadRefOID, err))
	}
	var response mergePullRequestResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return a.reconcileMergeOutcome(root, number, view, plan,
			fmt.Errorf("decode PR #%d merge: %w", number, err))
	}
	if !response.Merged {
		return stateError("PR #%d was not merged: %s", number, response.Message)
	}
	response.SHA = strings.TrimSpace(response.SHA)
	if response.SHA == "" {
		return a.reconcileMergeOutcome(root, number, view, plan,
			stateError("PR #%d merge response reported merged without a merge SHA", number))
	}
	return a.finishMergedPullRequest(root, number, view, response.SHA, plan)
}

func (a app) reconcileMergeOutcome(root string, number int, view pullRequestView, plan cleanupPlan, ambiguous error) error {
	observed, err := a.readPullRequest(root, number)
	if err != nil {
		return mergeOutcomeUnknownError(number, ambiguous, fmt.Errorf("reconcile PR metadata: %w", err))
	}
	mergeSHA, err := mergedPullRequestSHA(observed)
	if err != nil {
		return mergeOutcomeUnknownError(number, ambiguous, err)
	}
	if observed.HeadRefOID != view.HeadRefOID {
		return postMergeRecoveryError(number, mergeSHA, "post-merge claim proof reconciliation", stateError("PR current head %s differs from merge-time evaluated head %s; preserve claim artifacts", observed.HeadRefOID, view.HeadRefOID))
	}
	receipt, metadataErr := latestPassingEvaluationMatchesPR(observed, number)
	if metadataErr != nil {
		return postMergeRecoveryError(number, mergeSHA, "post-merge evaluation proof reconciliation", metadataErr)
	}
	if planErr := cleanupPlanMatchesReceipt(plan, receipt, number); planErr != nil {
		return postMergeRecoveryError(number, mergeSHA, "post-merge claim proof reconciliation", planErr)
	}
	return a.finishMergedPullRequest(root, number, view, mergeSHA, plan)
}

func latestPassingEvaluationMatchesPR(view pullRequestView, number int) (evaluationReceipt, error) {
	receipt, err := latestPassingEvaluationReceipt(view, number)
	if err != nil {
		return evaluationReceipt{}, fmt.Errorf("post-merge evaluation proof is not valid: %w", err)
	}
	return receipt, nil
}

func cleanupPlanMatchesReceipt(plan cleanupPlan, receipt evaluationReceipt, number int) error {
	if !plan.validateArtifacts {
		return stateError("PR #%d cleanup plan is not bound to immutable claim artifacts; preserve claim artifacts", number)
	}
	if plan.proofHead != receipt.Head {
		return stateError("PR #%d cleanup plan head %q differs from immutable evaluation head %q; preserve claim artifacts", number, plan.proofHead, receipt.Head)
	}
	proof := mergeEvaluationProof{
		claimProofs:   append([]evaluationClaimProof(nil), receipt.ClaimProofs...),
		closingIssues: append([]int(nil), receipt.ClosingIssues...),
		head:          receipt.Head,
		headRefName:   receipt.HeadRefName,
	}
	primary, ok := fixedClaimIssue(receipt.HeadRefName)
	if !ok {
		return stateError("PR #%d immutable evaluation head ref %q is not an issue claim; preserve claim artifacts", number, receipt.HeadRefName)
	}
	if plan.primaryIssue != primary {
		return stateError("PR #%d cleanup plan primary issue #%d differs from immutable evaluation primary issue #%d; preserve claim artifacts", number, plan.primaryIssue, primary)
	}
	claims, err := recoveryClaimProofs(proof, primary, number)
	if err != nil {
		return err
	}
	if len(claims) != len(plan.claims) {
		return stateError("PR #%d cleanup plan no longer matches immutable claim proof; preserve claim artifacts", number)
	}
	for _, expected := range claims {
		matched := false
		for _, actual := range plan.claims {
			if expected.Issue == actual.issue && expected.Branch == actual.branch && expected.SHA == actual.sha {
				matched = true
				break
			}
		}
		if !matched {
			return stateError("PR #%d cleanup plan differs from immutable evaluation claim %s; preserve claim artifacts", number, expected.Branch)
		}
	}
	return nil
}

func mergeOutcomeUnknownError(number int, ambiguous, reconciliation error) error {
	return stateError("PR #%d merge state is unknown after an ambiguous merge response: %w. Do not retry blindly; run `go tool workflowctl pr recover %d` to reconcile it", number, errors.Join(ambiguous, reconciliation), number)
}

func (a app) finishMergedPullRequest(root string, number int, view pullRequestView, mergeSHA string, plan cleanupPlan) error {
	for _, issue := range view.ClosingIssuesReferences {
		if err := a.setIssueProjectStatus(root, issue.Number, "Done"); err != nil {
			if writeErr := writeLine(a.stderr, "PR #%d merged; issue #%d Project sync deferred: %v", number,
				issue.Number, err); writeErr != nil {
				return fmt.Errorf("PR merged; report deferred Project sync: %w", writeErr)
			}
			break
		}
	}
	base, syncErr := a.synchronizeBase(plan.layout, mergeSHA)
	if syncErr != nil {
		return postMergeRecoveryError(number, mergeSHA, "canonical Git base convergence", syncErr)
	}
	packet := mergedPacket{number: number, mergeSHA: mergeSHA, plan: plan}
	if cleanupErr := a.cleanupClaims(base, packet); cleanupErr != nil {
		return postMergeRecoveryError(number, mergeSHA, "claim cleanup", cleanupErr)
	}
	return writeLine(a.stdout, "PR #%d merged at evaluated head %s as %s", number, view.HeadRefOID, mergeSHA)
}

func postMergeRecoveryError(number int, mergeSHA, phase string, cause error) error {
	return stateError("PR #%d merged at %s; %s failed and recovery is needed: %w. Merge completed. Run `go tool workflowctl pr recover %d`", number, mergeSHA, phase, cause, number)
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
	candidates := make([]remoteClaim, 0, 1)
	for _, claim := range claims {
		if claim.number != number || !claim.active {
			continue
		}
		if _, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head); err != nil {
			if isGitNonAncestor(err) {
				continue
			}
			return fmt.Errorf("prove companion issue #%d claim %s is included in evaluated head: %w", number, claim.branch, err)
		}
		candidates = append(candidates, claim)
	}
	if len(candidates) == 1 {
		return nil
	}
	if len(candidates) > 1 {
		return stateError("companion issue #%d has ambiguous active claims in evaluated head %s", number, head)
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
