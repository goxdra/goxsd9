package workflowctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestQualityChecksIncludeUnsupportedFeatureRegistry(t *testing.T) {
	checks := (app{}).qualityChecks(t.TempDir(), true)
	for _, check := range checks {
		if check.name == "unsupported feature registry" {
			return
		}
	}
	t.Fatal("quality checks do not include unsupported feature registry")
}

func TestIssueFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   int
		ok     bool
	}{
		{branch: "agent/issue-12", want: 12, ok: true},
		{branch: "agent/issue-12-bootstrap", want: 12, ok: true},
		{branch: "main", want: 0, ok: false},
		{branch: "agent/issue-no", want: 0, ok: false},
	}
	for _, test := range tests {
		got, ok := issueFromBranch(test.branch)
		if got != test.want || ok != test.ok {
			t.Fatalf("issueFromBranch(%q) = (%d, %t), want (%d, %t)", test.branch, got, ok, test.want, test.ok)
		}
	}
}

func TestTrailerTime(t *testing.T) {
	want := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	got, err := trailerTime("subject\n\nAgent-Lease-Until: 2026-08-15T06:00:00Z\n", "Agent-Lease-Until")
	if err != nil {
		t.Fatalf("trailerTime: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("trailerTime = %s, want %s", got, want)
	}
}

func TestEvaluationReceiptRoundTrip(t *testing.T) {
	report := []byte("No findings.")
	recorded := time.Date(2026, time.August, 15, 4, 0, 0, 0, time.UTC)
	receipt := evaluationReceipt{
		Evaluator:    "Examiner",
		Head:         "abc123",
		RecordedAt:   recorded,
		ReportSHA256: fmt.Sprintf("%x", sha256.Sum256(report)),
		Round:        2,
		Verdict:      "pass",
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	body := evaluationComment(marker, nil, string(report))
	got, ok := parseEvaluationReceipt(body)
	if !ok {
		t.Fatal("parseEvaluationReceipt rejected a generated marker")
	}
	if got.Head != "abc123" || got.Round != 2 || got.Verdict != "pass" {
		t.Fatalf("receipt = %#v", got)
	}
	comment := pullRequestComment{Body: body, CreatedAt: recorded}
	comment.Author.Login = trustedActor
	receipts, err := evaluationReceipts([]pullRequestComment{comment})
	if err != nil {
		t.Fatalf("evaluationReceipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("trusted receipts = %d, want 1", len(receipts))
	}
}

func TestDecodeJSONDocuments(t *testing.T) {
	type document struct {
		Page int `json:"page"`
	}
	tests := []struct {
		name    string
		input   string
		want    []document
		wantErr bool
	}{
		{name: "multiple documents", input: `{"page":1}{"page":2}`, want: []document{{Page: 1}, {Page: 2}}},
		{name: "empty document stream", input: "\n\t", wantErr: true},
		{name: "malformed document", input: `{"page":1}{"page":`, wantErr: true},
		{name: "trailing non-json data", input: `{"page":1}trailing`, wantErr: true},
	}
	for _, test := range tests {
		got, err := decodeJSONDocuments[document](test.input)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: decodeJSONDocuments error = %v, want error %t", test.name, err, test.wantErr)
		}
		if test.wantErr {
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s: decodeJSONDocuments = %#v, want %#v", test.name, got, test.want)
		}
	}
}

func TestLatestEvaluationControlsHead(t *testing.T) {
	pass := testEvaluationComment(t, "head", 1, "pass")
	fail := testEvaluationComment(t, "head", 2, "fail")
	view := pullRequestView{Comments: []pullRequestComment{pass, fail}, HeadRefOID: "head"}
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil {
		t.Fatalf("latestEvaluationPasses: %v", err)
	}
	if passes {
		t.Fatal("an earlier pass overrode the latest failing evaluation")
	}
}

func TestLatestStructuredEvaluationPasses(t *testing.T) {
	requested := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	recorded := requested.Add(time.Minute)
	challenge := evaluationChallenge{Challenge: "run-challenge", Head: "head", PR: 11, RequestedAt: requested}
	challengeMarker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	challengeComment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeMarker),
		CreatedAt: requested,
	}
	challengeComment.Author.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: "run-challenge", Evaluator: "Examiner", Findings: []evaluationFinding{}, Head: "head", PR: 11,
		RunID: "examiner-run", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	attestationMarker, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationMarker)),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recorded,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             3,
		Verdict:           attestation.Verdict,
	}
	receiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	evaluationReceiptComment := pullRequestComment{
		Body:      evaluationComment(receiptMarker, attestationMarker, report),
		CreatedAt: recorded,
	}
	evaluationReceiptComment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{challengeComment, evaluationReceiptComment}, HeadRefOID: "head"}
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil || !passes {
		t.Fatal("valid structured evaluation did not pass")
	}
	view.Comments[1].Body = strings.Replace(view.Comments[1].Body, "No findings.", "Changed.", 1)
	if _, err := latestEvaluationPasses(view, 11); err == nil {
		t.Fatal("tampered structured evaluation did not invalidate history")
	}
}

func TestLatestEvaluationRejectsBareOwnerReceipt(t *testing.T) {
	requested := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	recorded := requested.Add(time.Minute)
	challenge := evaluationChallenge{Challenge: "bot-challenge", Head: "head", PR: 11, RequestedAt: requested}
	challengeMarker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	challengeComment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, challengeMarker),
		CreatedAt: requested,
	}
	challengeComment.Author.Login = trustedActor
	attestation := evaluationAttestation{
		Challenge: challenge.Challenge, Evaluator: "Examiner", Findings: []evaluationFinding{}, Head: "head", PR: 11,
		RunID: "owner-receipt-run", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	attestationMarker, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode attestation: %v", err)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationMarker)),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recorded,
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(report))),
		Round:             1,
		Verdict:           attestation.Verdict,
	}
	receiptMarker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	receiptComment := pullRequestComment{
		Body:      evaluationComment(receiptMarker, attestationMarker, report),
		CreatedAt: recorded,
	}
	receiptComment.Author.Login = owner
	view := pullRequestView{Comments: []pullRequestComment{challengeComment, receiptComment}, HeadRefOID: "head"}
	passes, err := latestEvaluationPasses(view, 11)
	if err != nil {
		t.Fatalf("latestEvaluationPasses: %v", err)
	}
	if passes {
		t.Fatal("bare owner-authored receipt authorized the evaluated head")
	}
}

func TestEvaluationFailureCountIgnoresPassingRounds(t *testing.T) {
	receipts := []evaluationReceipt{{Verdict: "pass"}, {Verdict: "fail"}, {Verdict: "fail"}}
	if got := evaluationFailureCount(receipts); got != 2 {
		t.Fatalf("evaluationFailureCount = %d, want 2", got)
	}
}

func testEvaluationComment(t *testing.T, head string, round int, verdict string) pullRequestComment {
	t.Helper()
	recorded := time.Date(2026, time.August, 15, 4, 0, round, 0, time.UTC)
	report := []byte(verdict + " report")
	receipt := evaluationReceipt{
		Evaluator:    "Examiner",
		Head:         head,
		RecordedAt:   recorded,
		ReportSHA256: fmt.Sprintf("%x", sha256.Sum256(report)),
		Round:        round,
		Verdict:      verdict,
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	comment := pullRequestComment{Body: evaluationComment(marker, nil, string(report)), CreatedAt: recorded}
	comment.Author.Login = trustedActor
	return comment
}

func TestCandidateOrdering(t *testing.T) {
	created := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	candidates := []pickCandidate{
		{Number: 4, Priority: "P2", Effort: "XS", created: created},
		{Number: 3, Priority: "P1", Effort: "M", Blocking: 1, created: created},
		{Number: 2, Priority: "P1", Effort: "S", Blocking: 2, created: created},
		{Number: 1, Priority: "P1", Effort: "XS", Blocking: 2, created: created},
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidateLess(candidates[left], candidates[right])
	})
	want := []int{1, 2, 3, 4}
	for index := range candidates {
		if candidates[index].Number != want[index] {
			t.Fatalf("candidate %d = #%d, want #%d", index, candidates[index].Number, want[index])
		}
	}
}

func TestIssueRelationsUsesGraphQL(t *testing.T) {
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "gh" {
			t.Fatalf("command = %s, want gh", name)
		}
		want := []string{"api", "graphql", "-f", "query=" + issueRelationsQuery,
			"-f", "owner=" + owner, "-f", "repository=" + repository, "-F", "number=25"}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("arguments = %#v, want %#v", args, want)
		}
		return `{"data":{"repository":{"issue":{"blockedBy":{"nodes":[{"number":2,"state":"OPEN","title":"source"}]},"blocking":{"nodes":[]},"createdAt":"2026-08-15T00:00:00Z"}}}}`, nil
	}}
	relations, err := application.issueRelations("/repo", 25)
	if err != nil {
		t.Fatalf("issueRelations: %v", err)
	}
	if got, want := len(relations.BlockedBy.Nodes), 1; got != want {
		t.Fatalf("blockedBy length = %d, want %d", got, want)
	}
	if got, want := relations.BlockedBy.Nodes[0].Number, 2; got != want {
		t.Fatalf("blockedBy issue = %d, want %d", got, want)
	}
}

func TestIssueRelationsDecoratesGitHubFailure(t *testing.T) {
	failure := errors.New("access denied")
	application := app{executeCommand: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
		return "", failure
	}}
	_, err := application.issueRelations("/repo", 25)
	if !errors.Is(err, failure) {
		t.Fatalf("issueRelations error = %v, want wrapped command failure", err)
	}
	if !strings.Contains(err.Error(), "issue #25 dependencies") {
		t.Fatalf("issueRelations error = %v, want issue context", err)
	}
}

func TestGuardRejectsConcurrencyAndElse(t *testing.T) {
	tests := []string{
		"package example\nfunc f() { go f() }\n",
		"package example\nfunc f() chan int { return nil }\n",
		"package example\nimport \"context\"\nfunc f() { <-context.Background().Done() }\n",
		"package example\nfunc f(ch chan<- int) { ch <- 1 }\n",
		"package example\nfunc f() { select {} }\n",
		"package example\nfunc f(ok bool) { if ok { return } else { return } }\n",
		"package example\nimport \"sync\"\nvar _ sync.Mutex\n",
	}
	for _, source := range tests {
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "guard.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse guard fixture: %v", err)
		}
		if err := guardFile(files, file); err == nil {
			t.Fatalf("guard accepted forbidden source %q", source)
		}
	}
}

func TestIssueInputRejectsUnknownProjectOptions(t *testing.T) {
	bodyFile := t.TempDir() + "/issue.md"
	if err := os.WriteFile(bodyFile, []byte("## Acceptance\n\nProof.\n"), 0o600); err != nil {
		t.Fatalf("write issue body: %v", err)
	}
	tests := []struct {
		name   string
		effort string
		phase  string
	}{
		{name: "effort", effort: "Huge", phase: "Bootstrap"},
		{name: "phase", effort: "S", phase: "Eventually"},
	}
	for _, test := range tests {
		if err := validateIssueInput("title", bodyFile, "workflow", "tooling", "P2", test.effort,
			test.phase, "Backlog"); err == nil {
			t.Fatalf("%s option unexpectedly accepted", test.name)
		}
	}
}

func TestDocumentedPositionalFlagOrderParses(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "handoff", run: func() error { return application.runHandoff([]string{"1", "--body-file", "missing"}) }},
		{name: "pr open", run: func() error {
			return application.openPullRequest([]string{"1", "--title", "test(workflow): verify flags", "--body-file", "missing"})
		}},
		{name: "evaluation", run: func() error {
			return application.recordEvaluation([]string{"1", "--attestation-file", "missing"})
		}},
	}
	for _, test := range tests {
		err := test.run()
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded", test.name)
		}
		if !strings.Contains(err.Error(), "missing") {
			t.Fatalf("%s did not reach body-file validation: %v", test.name, err)
		}
	}
}

func TestSessionSummaryExtractsPlainTextSection(t *testing.T) {
	body := "## Session summary\r\n\r\n" +
		"The merge endpoint currently lets GitHub synthesize a noisy body.\r\n\r\n" +
		"Send the reviewed handoff explicitly so later workflows retain why the\r\n" +
		"change was made and which invariant they must preserve.\r\n\r\n" +
		"## Work packet\r\n\r\nCloses #33\r\n"
	want := "The merge endpoint currently lets GitHub synthesize a noisy body.\n\n" +
		"Send the reviewed handoff explicitly so later workflows retain why the\n" +
		"change was made and which invariant they must preserve."
	got, err := sessionSummary(body)
	if err != nil {
		t.Fatalf("extract session summary: %v", err)
	}
	if got != want {
		t.Fatalf("session summary = %q, want %q", got, want)
	}
}

func TestPullRequestBodyRequiresOneNonemptySessionSummary(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		valid bool
	}{
		{name: "valid", body: "## Session summary\n\nExplain the durable outcome.\n\n## Work packet\n\nCloses #33\n", valid: true},
		{name: "missing", body: "## Work packet\n\nCloses #33\n"},
		{name: "empty", body: "## Session summary\n\n## Work packet\n\nCloses #33\n"},
		{name: "duplicate", body: "## Session summary\n\nFirst.\n\n## Session summary\n\nSecond.\n\nCloses #33\n"},
		{name: "comment only", body: "## Session summary\n\n<!-- Replace this. -->\n\nCloses #33\n"},
		{name: "heading", body: "## Session summary\n\n### Work done\n\nExplain it.\n\nCloses #33\n"},
		{name: "fence", body: "## Session summary\n\n```text\nExplain it.\n```\n\nCloses #33\n"},
	}
	for _, test := range tests {
		path := filepath.Join(t.TempDir(), "pr.md")
		if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
			t.Fatalf("%s: write PR body: %v", test.name, err)
		}
		_, err := readPullRequestBody(path, 33)
		if (err == nil) != test.valid {
			t.Fatalf("%s: validation error = %v, valid %t", test.name, err, test.valid)
		}
	}
}

func TestPullRequestOpenRejectsInvalidSummaryBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pr.md")
	if err := os.WriteFile(path, []byte("## Work packet\n\nCloses #33\n"), 0o600); err != nil {
		t.Fatalf("write PR body: %v", err)
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		t.Fatalf("invalid PR body executed %s %v", name, args)
		return "", nil
	}}
	err := application.openPullRequest([]string{
		"33", "--title", "fix(workflow): summarize squash commits", "--body-file", path,
	})
	if err == nil || !strings.Contains(err.Error(), sessionSummaryHeading) {
		t.Fatalf("invalid session summary error = %v", err)
	}
}

func TestGitHistoryIncludesCommitBodiesForWorkflowReaders(t *testing.T) {
	since := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: func(_ string, _ io.Reader, name string,
		args ...string,
	) (string, error) {
		want := "git log --first-parent -n 3 --since=2026-08-08T12:00:00Z --date=short " +
			"--pretty=format:- %h %ad %s%n%w(74,2,2)%b%w(0,0,0)"
		got := name + " " + strings.Join(args, " ")
		if got != want {
			return "", fmt.Errorf("command = %q, want %q", got, want)
		}
		return "- abc123 2026-08-15 fix(workflow): summarize squash commits\n" +
			"  Explain the problem and durable outcome.", nil
	}}
	if err := application.writeGitHistory("/repo", since, 3); err != nil {
		t.Fatalf("write Git history: %v", err)
	}
	if !strings.Contains(stdout.String(), "Explain the problem and durable outcome.") {
		t.Fatalf("history omitted commit body:\n%s", stdout.String())
	}
}

func TestLeaseFreshness(t *testing.T) {
	now := time.Date(2026, time.August, 15, 4, 30, 0, 0, time.UTC)
	lease := now.Add(leaseDuration - renewalInterval)
	if err := validateLeaseFresh(1, lease, now); err != nil {
		t.Fatalf("fresh lease rejected: %v", err)
	}
	if err := validateLeaseFresh(1, lease.Add(-time.Second), now); err == nil {
		t.Fatal("stale renewal interval accepted")
	}
}

func TestLateClaimRenewalIsRejected(t *testing.T) {
	now := time.Date(2026, time.August, 15, 4, 31, 0, 0, time.UTC)
	lease := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	if err := validateLeaseFresh(1, lease, now); err == nil {
		t.Fatal("claim missed its renewal heartbeat but was accepted")
	}
}

func TestPullRequestMustCloseClaim(t *testing.T) {
	view := pullRequestView{}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 7})
	if !pullRequestCloses(view, 7) {
		t.Fatal("linked closing issue was not recognized")
	}
	if pullRequestCloses(view, 8) {
		t.Fatal("unlinked issue was recognized as closing")
	}
}

func TestRequiredQualityCheckMustSucceed(t *testing.T) {
	tests := []struct {
		name    string
		checks  []pullRequestCheck
		wantErr bool
	}{
		{name: "success", checks: []pullRequestCheck{{Name: "quality", Status: "completed", Conclusion: "success"}}},
		{name: "unrelated", checks: []pullRequestCheck{{Name: "docs", Status: "completed", Conclusion: "success"}}, wantErr: true},
		{name: "skipped", checks: []pullRequestCheck{{Name: "quality", Status: "completed", Conclusion: "skipped"}}, wantErr: true},
		{name: "neutral", checks: []pullRequestCheck{{Name: "quality", Status: "completed", Conclusion: "neutral"}}, wantErr: true},
		{name: "running", checks: []pullRequestCheck{{Name: "quality", Status: "in_progress"}}, wantErr: true},
	}
	for _, test := range tests {
		pages := []checkRunsAPI{{CheckRuns: test.checks}}
		err := requireQualityCheck(pages)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: requireQualityCheck error = %v, want error %t", test.name, err, test.wantErr)
		}
	}
}

func TestWorkPacketRejectsMoreThanOneCompanion(t *testing.T) {
	view := pullRequestView{HeadRefOID: "head"}
	for _, number := range []int{1, 2, 3} {
		view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
			Number int `json:"number"`
		}{Number: number})
	}
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if err := application.validateClosingClaims("", view, 1); err == nil {
		t.Fatal("work packet with two companion issues was accepted")
	}
}

func TestEvaluationAttestationIsBoundToChallengeAndHead(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "run-challenge", Head: "head", PR: 11, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker),
		CreatedAt: now,
	}
	comment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{comment}, HeadRefOID: "head"}
	attestation := evaluationAttestation{
		Challenge: "run-challenge",
		Evaluator: "Examiner",
		Findings:  []evaluationFinding{},
		Head:      "head",
		PR:        11,
		RunID:     "examiner-fresh-context",
		Schema:    evaluationAttestationSchema,
		Summary:   "No blocking findings.",
		Verdict:   "pass",
	}
	if err := validateEvaluationAttestation(attestation, 11, view, nil, now); err != nil {
		t.Fatalf("valid attestation rejected: %v", err)
	}
	attestation.Head = "other"
	if err := validateEvaluationAttestation(attestation, 11, view, nil, now); err == nil {
		t.Fatal("wrong-head attestation accepted")
	}
}

func TestEvaluationChallengeRejectsBareOwnerActor(t *testing.T) {
	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "bot-challenge", Head: "head", PR: 11, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker),
		CreatedAt: now,
	}
	comment.Author.Login = owner
	if _, ok := trustedEvaluationChallenge([]pullRequestComment{comment}, challenge.Challenge, challenge.PR,
		challenge.Head, now); ok {
		t.Fatal("bare owner-authored challenge was trusted")
	}
}

func TestEvaluationAttestationRejectsCallerVerdictAndReusedChallenge(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if err := application.recordEvaluation([]string{"11", "--verdict", "pass", "--body-file", "report"}); err == nil {
		t.Fatal("caller-selected verdict flags were accepted")
	}

	now := time.Date(2026, time.August, 15, 5, 30, 0, 0, time.UTC)
	challenge := evaluationChallenge{Challenge: "run-used", Head: "head", PR: 11, RequestedAt: now}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode challenge: %v", err)
	}
	comment := pullRequestComment{Body: fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker), CreatedAt: now}
	comment.Author.Login = trustedActor
	view := pullRequestView{Comments: []pullRequestComment{comment}, HeadRefOID: "head"}
	attestation := evaluationAttestation{
		Challenge: "run-used", Evaluator: "Examiner", Findings: []evaluationFinding{}, Head: "head", PR: 11,
		RunID: "examiner-run", Schema: evaluationAttestationSchema, Summary: "No findings.", Verdict: "pass",
	}
	receipts := []evaluationReceipt{{Challenge: "run-used", Verdict: "fail"}}
	if err := validateEvaluationAttestation(attestation, 11, view, receipts, now); err == nil {
		t.Fatal("reused evaluation challenge was accepted")
	}
}

func TestEvaluationFindingsMustBePresentArray(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		decodeFail bool
		valid      bool
	}{
		{name: "omitted", json: `{"verdict":"pass"}`},
		{name: "null", json: `{"verdict":"pass","findings":null}`, decodeFail: true},
		{name: "empty", json: `{"verdict":"pass","findings":[]}`, valid: true},
	}
	for _, test := range tests {
		var attestation evaluationAttestation
		err := json.Unmarshal([]byte(test.json), &attestation)
		if test.decodeFail {
			if err == nil {
				t.Fatalf("%s findings decoded", test.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s findings decode: %v", test.name, err)
		}
		err = validateEvaluationFindings(attestation)
		if (err == nil) != test.valid {
			t.Fatalf("%s findings validation error = %v, valid %t", test.name, err, test.valid)
		}
	}
}

func TestPullRequestFinishStrategyHasRESTFallback(t *testing.T) {
	draft := pullRequestView{IsDraft: true}
	if action := finishActionFor(draft, true); action != finishMergeREST {
		t.Fatalf("ready transition action = %d, want REST merge", action)
	}
	if action := finishActionFor(draft, false); action != finishReplaceDraftREST {
		t.Fatalf("failed ready transition action = %d, want REST replacement", action)
	}
	if action := finishActionFor(pullRequestView{}, false); action != finishMergeREST {
		t.Fatalf("ready PR action = %d, want REST merge", action)
	}
	summary := "Explain the durable outcome and rationale."
	body := readyReplacementBody("## Session summary\n\n"+summary+"\n\n## Work packet\n\nCloses #1\n", 11, "abc123")
	if !strings.Contains(body, "Closes #1") || !strings.Contains(body, "Replaces draft PR #11") ||
		!strings.Contains(body, "abc123") {
		t.Fatalf("replacement body lost work-packet or provenance data: %q", body)
	}
	got, err := sessionSummary(body)
	if err != nil {
		t.Fatalf("extract replacement session summary: %v", err)
	}
	if got != summary {
		t.Fatalf("replacement session summary = %q, want %q", got, summary)
	}
}
