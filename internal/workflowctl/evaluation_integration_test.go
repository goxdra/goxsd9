package workflowctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testExaminerRunID = "examiner-command-flow"

func TestEvaluationToMergeCommandFlow(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := app{
		ctx:            context.Background(),
		executeCommand: backend.execute,
		stdout:         &stdout,
		stderr:         &stderr,
	}

	challenge := requestTestChallenge(t, &application, &stdout)
	attestationJSON, attestationFile := writeTestAttestation(t, backend.head, challenge)

	stdout.Reset()
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
		t.Fatalf("record evaluation: %v", err)
	}
	checkRecordedAttestation(t, backend.comments, attestationJSON)
	rejectTamperedReceiptReuse(t, &application, backend, attestationFile)
	rejectLaterTamperedReceipt(t, &application, backend)
	rejectInvalidPullRequestTitle(t, &application, backend)
	rejectInvalidWorkCommitTitle(t, &application, backend)

	commentCount := len(backend.comments)
	backend.comments = append(backend.comments, reusedRunComments(t, backend.head, testExaminerRunID)...)
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted a reused Examiner run ID")
	}
	backend.comments = backend.comments[:commentCount]

	backend.head = "unevaluated-head"
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted an evaluation for an earlier head")
	}
	if backend.merged {
		t.Fatal("unevaluated head reached the merge endpoint")
	}
	backend.head = "evaluated-head"

	stdout.Reset()
	backend.removeSummaryOnNextCommand = true
	if err := application.runPR(backend.finishArgs()); err != nil {
		t.Fatalf("finish evaluated PR: %v", err)
	}
	checkMergeResult(t, backend)
}

func TestAmbiguousMergeResponsesReconcile(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "transport loss", mode: "transport"},
		{name: "malformed JSON", mode: "malformed"},
		{name: "missing SHA", mode: "missing-sha"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			backend.mergeResponseMode = test.mode
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			application := app{
				ctx:            context.Background(),
				executeCommand: backend.execute,
				stdout:         &stdout,
				stderr:         &stderr,
			}
			challenge := requestTestChallenge(t, &application, &stdout)
			_, attestationFile := writeTestAttestation(t, backend.head, challenge)
			if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err != nil {
				t.Fatalf("record evaluation: %v", err)
			}
			stdout.Reset()
			if err := application.runPR(backend.finishArgs()); err != nil {
				t.Fatalf("finish %s merge response: %v", test.mode, err)
			}
			checkMergeResult(t, backend)
			if !strings.Contains(stdout.String(), "merged at evaluated head evaluated-head as merge-commit") {
				t.Fatalf("finish output = %q, want reconciled merge", stdout.String())
			}
		})
	}
}

func TestUnknownMergeOutcomeGuidesRecovery(t *testing.T) {
	ambiguous := errors.New("merge endpoint transport failed")
	reconciliation := errors.New("PR is still open")
	err := mergeOutcomeUnknownError(14, ambiguous, reconciliation)
	if !strings.Contains(err.Error(), "merge state is unknown") || !strings.Contains(err.Error(), "go tool workflowctl pr recover 14") {
		t.Fatalf("mergeOutcomeUnknownError = %v, want recovery guidance", err)
	}
	if !errors.Is(err, ambiguous) || !errors.Is(err, reconciliation) {
		t.Fatalf("mergeOutcomeUnknownError = %v, want both causes", err)
	}
}

func reusedRunComments(t *testing.T, head, runID string) []issueCommentAPI {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	challenge := evaluationChallenge{Challenge: "duplicate-run-challenge", Head: head, PR: 14, RequestedAt: now}
	challengeJSON, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode reused-run challenge: %v", err)
	}
	challengeComment := issueCommentAPI{
		Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeJSON), CreatedAt: now,
	}
	challengeComment.User.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: evaluationFindings{}, Head: head, PR: 14,
		RunID: runID, Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode reused-run attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationJSON)),
		Challenge:         challenge.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    runID,
		Head:              head,
		PR:                14,
		RecordedAt:        now,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             2,
		Verdict:           "pass",
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode reused-run receipt: %v", err)
	}
	receiptComment := issueCommentAPI{Body: evaluationComment(receiptJSON, attestationJSON, report), CreatedAt: now}
	receiptComment.User.Login = trustedActor
	return []issueCommentAPI{challengeComment, receiptComment}
}

func rejectTamperedReceiptReuse(t *testing.T, application *app, backend *workflowBackend, attestationFile string) {
	t.Helper()
	original := backend.comments[1].Body
	backend.comments[1].Body = strings.Replace(original, "No blocking findings", "Changed findings", 1)
	if err := application.runEvaluation([]string{"record", "14", "--attestation-file", attestationFile}); err == nil {
		t.Fatal("tampered history allowed evaluation evidence reuse")
	}
	backend.comments[1].Body = original
}

func rejectLaterTamperedReceipt(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	tampered := backend.comments[1]
	tampered.Body = strings.Replace(tampered.Body, "No blocking findings", "Changed findings", 1)
	tampered.CreatedAt = time.Now().UTC().Truncate(time.Second)
	backend.comments = append(backend.comments, tampered)
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("later tampered receipt fell back to an earlier pass")
	}
	backend.comments = backend.comments[:len(backend.comments)-1]
}

func rejectInvalidPullRequestTitle(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	original := backend.title
	backend.title = "Invalid title"
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted an invalid pull request title")
	}
	if backend.merged {
		t.Fatal("invalid pull request title reached the merge endpoint")
	}
	backend.title = original
}

func rejectInvalidWorkCommitTitle(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	original := backend.workCommitLog
	backend.workCommitLog = framedRawCommitLog(
		"fix(parser): reject invalid XML\ncontinue\n", "chore(workflow): claim issue #13\n")
	if err := application.runPR(backend.finishArgs()); err == nil {
		t.Fatal("merge accepted a work commit added after PR creation with an invalid title")
	}
	if backend.merged {
		t.Fatal("invalid work commit title reached the merge endpoint")
	}
	backend.workCommitLog = original
}

func requestTestChallenge(t *testing.T, application *app, stdout *bytes.Buffer) evaluationChallenge {
	t.Helper()
	if err := application.runEvaluation([]string{"challenge", "14"}); err != nil {
		t.Fatalf("create evaluation challenge: %v", err)
	}
	var challenge evaluationChallenge
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &challenge); err != nil {
		t.Fatalf("decode challenge output: %v", err)
	}
	return challenge
}

func writeTestAttestation(t *testing.T, head string, challenge evaluationChallenge) ([]byte, string) {
	t.Helper()
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge,
		Evaluator: "Examiner",
		Findings:  evaluationFindings{},
		Head:      head,
		PR:        14,
		RunID:     testExaminerRunID,
		Schema:    evaluationAttestationSchema,
		Summary:   "No blocking findings; delimiter --> remains data.",
		Verdict:   "pass",
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(attestation); err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	attestationJSON := encoded.Bytes()
	if !bytes.Contains(attestationJSON, []byte(" -->")) {
		t.Fatal("attestation fixture does not contain the literal comment delimiter")
	}
	path := filepath.Join(t.TempDir(), "attestation.json")
	if err := os.WriteFile(path, attestationJSON, 0o600); err != nil {
		t.Fatalf("write attestation: %v", err)
	}
	return attestationJSON, path
}

func checkRecordedAttestation(t *testing.T, comments []issueCommentAPI, attestationJSON []byte) {
	t.Helper()
	if got, want := len(comments), 2; got != want {
		t.Fatalf("comments = %d, want %d", got, want)
	}
	for index, comment := range comments {
		if comment.User.Login != trustedActor {
			t.Fatalf("comment %d author = %q, want %q", index, comment.User.Login, trustedActor)
		}
	}
	_, recovered, ok := parseCommentAttestation(comments[1].Body)
	if !ok || !bytes.Equal(recovered, attestationJSON) {
		t.Fatal("recorded comment did not recover the exact Examiner attestation bytes")
	}
	receipt, ok := parseEvaluationReceipt(comments[1].Body)
	if !ok {
		t.Fatal("recorded evaluation receipt is invalid")
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(attestationJSON))
	if receipt.AttestationSHA256 != wantHash {
		t.Fatalf("attestation hash = %s, want %s", receipt.AttestationSHA256, wantHash)
	}
}

func checkMergeResult(t *testing.T, backend *workflowBackend) {
	t.Helper()
	if !backend.merged || !backend.projectDone {
		t.Fatalf("merge state = %t, Project Done = %t", backend.merged, backend.projectDone)
	}
	if backend.mergeRequest.SHA != backend.head || backend.mergeRequest.MergeMethod != "squash" {
		t.Fatalf("merge request = %#v", backend.mergeRequest)
	}
	if backend.mergeRequest.CommitTitle != backend.title+" (#14)" {
		t.Fatalf("merge commit title = %q", backend.mergeRequest.CommitTitle)
	}
	if backend.mergeRequest.CommitMessage != backend.summary {
		t.Fatalf("merge commit message = %q, want %q", backend.mergeRequest.CommitMessage, backend.summary)
	}
	if strings.Contains(backend.mergeRequest.CommitMessage, "Agent-Run-ID") {
		t.Fatal("claim metadata leaked into the squash commit message")
	}
}

func TestReadEvaluationAttestationRejectsMalformedEvidence(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "unknown field", json: `{"schema":"goxsd9/examiner-attestation/v1","extra":true}`},
		{name: "trailing value", json: `{}` + "\n" + `{}`},
		{name: "null findings", json: `{"findings":null}`},
	}
	for _, test := range tests {
		path := filepath.Join(t.TempDir(), "attestation.json")
		if err := os.WriteFile(path, []byte(test.json), 0o600); err != nil {
			t.Fatalf("%s: write attestation: %v", test.name, err)
		}
		if _, _, err := readEvaluationAttestation(path); err == nil {
			t.Fatalf("%s attestation was accepted", test.name)
		}
	}
}

func TestEvaluationAttestationRejectsReusedExaminerRun(t *testing.T) {
	now := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "fresh-challenge", Head: "head", PR: 14, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: now}
	comment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{comment}, HeadRefOID: "head"}
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: evaluationFindings{}, Head: "head", PR: 14,
		RunID: "examiner-reused", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	receipts := []evaluationReceipt{{Challenge: "earlier-challenge", EvaluatorRunID: attestation.RunID, Verdict: "fail"}}
	if err := validateEvaluationAttestation(attestation, 14, view, receipts, now); err == nil {
		t.Fatal("reused Examiner run ID was accepted")
	}
}

func TestEvaluationChallengeRejectsStaleOrUntrustedComments(t *testing.T) {
	now := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		requested time.Time
		created   time.Time
		author    string
	}{
		{name: "stale", requested: now.Add(-evaluationChallengeDuration - time.Second), created: now.Add(-evaluationChallengeDuration), author: trustedActor},
		{name: "future", requested: now.Add(time.Second), created: now.Add(time.Second), author: trustedActor},
		{name: "untrusted", requested: now, created: now, author: "other-user"},
		{name: "timestamp mismatch", requested: now, created: now.Add(6 * time.Minute), author: trustedActor},
	}
	for _, test := range tests {
		challenge := evaluationChallenge{
			Challenge: "challenge", Head: "head", PR: 14, RequestedAt: test.requested,
		}
		marker, err := json.Marshal(challenge)
		if err != nil {
			t.Fatalf("%s: encode challenge: %v", test.name, err)
		}
		comment := pullRequestComment{
			Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: test.created,
		}
		comment.Author.Login = test.author
		if _, ok := trustedEvaluationChallenge([]pullRequestComment{comment}, "challenge", 14, "head", now); ok {
			t.Fatalf("%s challenge was trusted", test.name)
		}
	}
}

func TestEvaluationChallengeExpiryBoundary(t *testing.T) {
	now := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		requested time.Time
		want      bool
	}{
		{name: "just before expiry", requested: now.Add(-evaluationChallengeDuration + time.Nanosecond), want: true},
		{name: "at expiry", requested: now.Add(-evaluationChallengeDuration), want: false},
		{name: "after expiry", requested: now.Add(-evaluationChallengeDuration - time.Nanosecond), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			challenge := evaluationChallenge{
				Challenge: "boundary-challenge", Head: "head", PR: 14, RequestedAt: test.requested,
			}
			marker, err := json.Marshal(challenge)
			if err != nil {
				t.Fatalf("encode challenge: %v", err)
			}
			comment := pullRequestComment{
				Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: test.requested,
			}
			comment.Author.Login = trustedActor
			_, ok := trustedEvaluationChallenge([]pullRequestComment{comment}, challenge.Challenge, challenge.PR,
				challenge.Head, now)
			if ok != test.want {
				t.Fatalf("challenge trusted = %t, want %t", ok, test.want)
			}
		})
	}
}

func TestClaimReadsIssueStateWhenProjectContentOmitsIt(t *testing.T) {
	tests := []struct {
		name    string
		issue   string
		wantErr bool
	}{
		{name: "open", issue: `{"state":"open","labels":[]}`},
		{name: "closed", issue: `{"state":"closed","labels":[]}`, wantErr: true},
		{name: "needs human", issue: `{"state":"open","labels":[{"name":"needs-human"}]}`, wantErr: true},
	}
	for _, test := range tests {
		application := app{executeCommand: claimStateCommand(t, test.issue)}
		err := application.assertClaimable("/repo", 13)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: assertClaimable error = %v, want error %t", test.name, err, test.wantErr)
		}
	}
}

func claimStateCommand(t *testing.T, issue string) commandExecutor {
	t.Helper()
	return func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "gh project item-list 1 --owner goxdra --format json --limit 500":
			return `{"items":[{"content":{"number":13,"repository":"goxdra/goxsd9"},"status":"Ready"}]}`, nil
		case "gh api repos/goxdra/goxsd9/issues/13":
			return issue, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}
}

type workflowBackend struct {
	t                          *testing.T
	root                       string
	branch                     string
	head                       string
	title                      string
	body                       string
	summary                    string
	summaryFile                string
	workCommitLog              string
	comments                   []issueCommentAPI
	mergeRequest               mergePullRequestRequest
	merged                     bool
	projectDone                bool
	mergeResponseMode          string
	mergeSHA                   string
	mergedAt                   time.Time
	removeSummaryOnNextCommand bool
}

func newWorkflowBackend(t *testing.T) *workflowBackend {
	t.Helper()
	summary := "GitHub currently derives squash bodies from branch commits, so claim\n" +
		"renewals obscure the implementation outcome. Send this reviewed summary\n" +
		"explicitly so future workflow sessions receive the durable rationale."
	summaryFile := filepath.Join(t.TempDir(), "summary.txt")
	if err := os.WriteFile(summaryFile, []byte(summary+"\n"), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	return &workflowBackend{
		t: t, root: "/primary-worktrees/issue-13", branch: "agent/issue-13", head: "evaluated-head",
		body:          "## Outcome\n\nExercise evaluation flow.\n\n## Work packet\n\nCloses #13\n",
		summary:       summary,
		summaryFile:   summaryFile,
		title:         "test(workflow): exercise evaluation flow",
		workCommitLog: framedCommitLog("test(workflow): exercise evaluation flow", "chore(workflow): claim issue #13"),
		mergeSHA:      "merge-commit",
	}
}

func (b *workflowBackend) finishArgs() []string {
	return []string{"finish", "14", "--summary-file", b.summaryFile}
}

func (b *workflowBackend) execute(dir string, input io.Reader, name string, args ...string) (string, error) {
	b.t.Helper()
	if b.removeSummaryOnNextCommand {
		if err := os.Remove(b.summaryFile); err != nil {
			return "", fmt.Errorf("remove summary after validation: %w", err)
		}
		b.removeSummaryOnNextCommand = false
	}
	var data []byte
	if input != nil {
		var err error
		data, err = io.ReadAll(input)
		if err != nil {
			return "", fmt.Errorf("read command input: %w", err)
		}
	}
	if name == "git" {
		return b.executeGit(dir, args)
	}
	if name == "gh" {
		return b.executeGitHub(data, args)
	}
	return "", fmt.Errorf("unexpected command in %s: %s %s", dir, name, strings.Join(args, " "))
}

func (b *workflowBackend) executeGit(dir string, args []string) (string, error) {
	command := strings.Join(args, " ")
	if output, ok := b.executeGitBase(dir, command); ok {
		return output, nil
	}
	return b.executeGitClaim(dir, command)
}

func (b *workflowBackend) executeGitBase(dir, command string) (string, bool) {
	if dir == "/primary" && command == "rev-parse HEAD" {
		return b.mergeSHA, true
	}
	switch command {
	case "rev-parse --show-toplevel":
		return b.root, true
	case "rev-parse --path-format=absolute --git-common-dir":
		return "/primary/.git", true
	case "worktree list --porcelain":
		return "worktree /primary\nHEAD merge-commit\nbranch refs/heads/main\n\n" +
			"worktree /primary-worktrees/issue-13\nHEAD evaluated-head\nbranch refs/heads/agent/issue-13\n", true
	case "-C /primary rev-parse --path-format=absolute --git-dir":
		return "/primary/.git", true
	case "-C /primary-worktrees/issue-13 rev-parse --path-format=absolute --git-dir":
		return "/primary/.git/worktrees/issue-13", true
	case "-C /primary-worktrees/issue-13 status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
		return "", true
	case "status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
		return "", true
	case "branch --show-current":
		if dir == "/primary" {
			return "main", true
		}
		return b.branch, true
	case "fetch origin main":
		return "", true
	case "rev-parse origin/main":
		return b.mergeSHA, true
	case "rev-list --left-right --count HEAD...origin/main":
		return "0 0", true
	case "merge-base --is-ancestor merge-commit merge-commit":
		return "", true
	case "submodule update --init --recursive", "submodule status --recursive", "submodule foreach --recursive --quiet git status --porcelain=v1 --untracked-files=all":
		return "", true
	case "merge --ff-only origin/main":
		return "", true
	case "worktree remove /primary-worktrees/issue-13":
		return "", true
	case "ls-remote --heads origin refs/heads/agent/issue-13":
		return "evaluated-head refs/heads/agent/issue-13", true
	case "push --force-with-lease=refs/heads/agent/issue-13:evaluated-head origin :refs/heads/agent/issue-13":
		return "", true
	case "for-each-ref --format=%(objectname) refs/remotes/origin/agent/issue-13", "for-each-ref --format=%(objectname) refs/heads/agent/issue-13":
		return "evaluated-head", true
	case "update-ref -d refs/remotes/origin/agent/issue-13 evaluated-head", "update-ref -d refs/heads/agent/issue-13 evaluated-head":
		return "", true
	default:
		return "", false
	}
}

func (b *workflowBackend) executeGitClaim(dir, command string) (string, error) {
	switch command {
	case "fetch origin refs/heads/agent/issue-13:refs/remotes/origin/agent/issue-13":
		return "", nil
	case "rev-parse HEAD", "rev-parse origin/agent/issue-13":
		return b.head, nil
	case "log -100 --format=%B":
		lease := time.Now().UTC().Add(claimDuration).Truncate(time.Second)
		return claimMessage(13, "run-test", lease), nil
	case "log --format=%x00%B%x00 origin/main.." + b.head:
		return b.workCommitLog, nil
	default:
		return "", fmt.Errorf("unexpected git command: %s in %s", command, dir)
	}
}

func (b *workflowBackend) executeGitHub(input []byte, args []string) (string, error) {
	joined := strings.Join(args, " ")
	switch joined {
	case "api repos/goxdra/goxsd9/pulls/14":
		return b.pullRequestJSON()
	case "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100":
		return b.commentsJSON()
	case "api --method POST repos/goxdra/goxsd9/issues/14/comments --input -":
		return b.postComment(input)
	case "api --paginate repos/goxdra/goxsd9/commits/evaluated-head/check-runs?per_page=100":
		return "{\"check_runs\":[{\"conclusion\":\"success\",\"name\":\"docs\",\"status\":\"completed\"}]}" +
			"{\"check_runs\":[{\"conclusion\":\"success\",\"name\":\"quality\",\"status\":\"completed\"}]}", nil
	case "api --method PUT repos/goxdra/goxsd9/pulls/14/merge --input -":
		return b.merge(input)
	case "project item-list 1 --owner goxdra --format json --limit 500":
		return `{"items":[{"content":{"number":13,"repository":"goxdra/goxsd9"},"id":"item-13"}],"totalCount":1}`, nil
	case "project field-list 1 --owner goxdra --format json":
		return `{"fields":[{"id":"status-id","name":"Status","options":[{"id":"done-id","name":"Done"}]}]}`, nil
	case "project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-13 --field-id status-id --single-select-option-id done-id":
		b.projectDone = true
		return "", nil
	default:
		return "", fmt.Errorf("unexpected gh command: %s", joined)
	}
}

func (b *workflowBackend) pullRequestJSON() (string, error) {
	response := pullRequestAPI{Body: b.body, Draft: false, State: "open", Title: b.title}
	if b.merged {
		response.Merged = true
		response.MergedAt = &b.mergedAt
		response.MergeCommitSHA = b.mergeSHA
		response.State = "closed"
	}
	response.Base.Ref = "main"
	response.Head.Ref = b.branch
	response.Head.SHA = b.head
	response.URL = "https://github.com/goxdra/goxsd9/pull/14"
	return marshalTestResponse(response)
}

func (b *workflowBackend) commentsJSON() (string, error) {
	if len(b.comments) < 2 {
		return marshalTestResponse(b.comments)
	}
	first, err := marshalTestResponse(b.comments[:1])
	if err != nil {
		return "", err
	}
	second, err := marshalTestResponse(b.comments[1:])
	if err != nil {
		return "", err
	}
	return first + second, nil
}

func (b *workflowBackend) postComment(data []byte) (string, error) {
	var request struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return "", fmt.Errorf("decode comment request: %w", err)
	}
	comment := issueCommentAPI{Body: request.Body, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	comment.User.Login = trustedActor
	b.comments = append(b.comments, comment)
	return `{}`, nil
}

func (b *workflowBackend) merge(data []byte) (string, error) {
	if err := json.Unmarshal(data, &b.mergeRequest); err != nil {
		return "", fmt.Errorf("decode merge request: %w", err)
	}
	b.merged = true
	b.mergedAt = time.Now().UTC().Truncate(time.Second)
	switch b.mergeResponseMode {
	case "transport":
		return "", errors.New("simulated lost merge response")
	case "malformed":
		return `{"merged":`, nil
	case "missing-sha":
		return `{"merged":true}`, nil
	default:
		return fmt.Sprintf(`{"merged":true,"sha":%q}`, b.mergeSHA), nil
	}
}

func marshalTestResponse(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
