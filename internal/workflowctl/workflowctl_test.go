package workflowctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

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

func TestIssueRelationsDecodeGitHubConnection(t *testing.T) {
	data := []byte(`{"blockedBy":{"nodes":[{"number":2,"state":"OPEN","title":"source"}],"totalCount":1},"blocking":{"nodes":[],"totalCount":0},"createdAt":"2026-08-15T00:00:00Z"}`)
	var relations issueRelations
	if err := json.Unmarshal(data, &relations); err != nil {
		t.Fatalf("decode issue relations: %v", err)
	}
	if got, want := len(relations.BlockedBy.Nodes), 1; got != want {
		t.Fatalf("blockedBy length = %d, want %d", got, want)
	}
	if got, want := relations.BlockedBy.Nodes[0].Number, 2; got != want {
		t.Fatalf("blockedBy issue = %d, want %d", got, want)
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
	body := readyReplacementBody("Closes #1\n", 11, "abc123")
	if !strings.Contains(body, "Closes #1") || !strings.Contains(body, "Replaces draft PR #11") ||
		!strings.Contains(body, "abc123") {
		t.Fatalf("replacement body lost work-packet or provenance data: %q", body)
	}
}
