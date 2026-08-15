package workflowctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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
	if err := application.runPR([]string{"finish", "14"}); err == nil {
		t.Fatal("merge accepted a reused Examiner run ID")
	}
	backend.comments = backend.comments[:commentCount]

	backend.head = "unevaluated-head"
	if err := application.runPR([]string{"finish", "14"}); err == nil {
		t.Fatal("merge accepted an evaluation for an earlier head")
	}
	if backend.merged {
		t.Fatal("unevaluated head reached the merge endpoint")
	}
	backend.head = "evaluated-head"

	stdout.Reset()
	if err := application.runPR([]string{"finish", "14"}); err != nil {
		t.Fatalf("finish evaluated PR: %v", err)
	}
	checkMergeResult(t, backend)
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
	challengeComment.User.Login = owner
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
	receiptComment.User.Login = owner
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
	if err := application.runPR([]string{"finish", "14"}); err == nil {
		t.Fatal("later tampered receipt fell back to an earlier pass")
	}
	backend.comments = backend.comments[:len(backend.comments)-1]
}

func rejectInvalidPullRequestTitle(t *testing.T, application *app, backend *workflowBackend) {
	t.Helper()
	original := backend.title
	backend.title = "Invalid title"
	if err := application.runPR([]string{"finish", "14"}); err == nil {
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
	if err := application.runPR([]string{"finish", "14"}); err == nil {
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
	comment.Author.Login = owner
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
		{name: "stale", requested: now.Add(-leaseDuration - time.Second), created: now.Add(-leaseDuration), author: owner},
		{name: "future", requested: now.Add(time.Second), created: now.Add(time.Second), author: owner},
		{name: "untrusted", requested: now, created: now, author: "other-user"},
		{name: "timestamp mismatch", requested: now, created: now.Add(6 * time.Minute), author: owner},
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
	t             *testing.T
	root          string
	branch        string
	head          string
	title         string
	workCommitLog string
	comments      []issueCommentAPI
	mergeRequest  mergePullRequestRequest
	merged        bool
	projectDone   bool
}

func newWorkflowBackend(t *testing.T) *workflowBackend {
	t.Helper()
	return &workflowBackend{
		t: t, root: "/repo", branch: "agent/issue-13", head: "evaluated-head",
		title:         "test(workflow): exercise evaluation flow",
		workCommitLog: framedCommitLog("test(workflow): exercise evaluation flow", "chore(workflow): claim issue #13"),
	}
}

func (b *workflowBackend) execute(dir string, input io.Reader, name string, args ...string) (string, error) {
	b.t.Helper()
	var data []byte
	if input != nil {
		var err error
		data, err = io.ReadAll(input)
		if err != nil {
			return "", fmt.Errorf("read command input: %w", err)
		}
	}
	if name == "git" {
		return b.executeGit(args)
	}
	if name == "gh" {
		return b.executeGitHub(data, args)
	}
	return "", fmt.Errorf("unexpected command in %s: %s %s", dir, name, strings.Join(args, " "))
}

func (b *workflowBackend) executeGit(args []string) (string, error) {
	switch strings.Join(args, " ") {
	case "rev-parse --show-toplevel":
		return b.root, nil
	case "branch --show-current":
		return b.branch, nil
	case "fetch origin refs/heads/agent/issue-13:refs/remotes/origin/agent/issue-13":
		return "", nil
	case "rev-parse HEAD", "rev-parse origin/agent/issue-13":
		return b.head, nil
	case "log -100 --format=%B":
		lease := time.Now().UTC().Add(leaseDuration).Truncate(time.Second)
		return claimMessage(13, "run-test", lease), nil
	case "log --format=%x00%B%x00 origin/main.." + b.head:
		return b.workCommitLog, nil
	default:
		return "", fmt.Errorf("unexpected git command: %s", strings.Join(args, " "))
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
	response := pullRequestAPI{Body: "Closes #13", Draft: false, State: "open", Title: b.title}
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
	comment.User.Login = owner
	b.comments = append(b.comments, comment)
	return `{}`, nil
}

func (b *workflowBackend) merge(data []byte) (string, error) {
	if err := json.Unmarshal(data, &b.mergeRequest); err != nil {
		return "", fmt.Errorf("decode merge request: %w", err)
	}
	b.merged = true
	return `{"merged":true,"sha":"merge-commit"}`, nil
}

func marshalTestResponse(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
