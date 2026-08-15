package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
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
	receipt := []byte(`{"head":"abc123","round":2,"verdict":"pass"}`)
	body := evaluationComment(receipt, []byte("No findings."))
	got, ok := parseEvaluationReceipt(body)
	if !ok {
		t.Fatal("parseEvaluationReceipt rejected a generated marker")
	}
	if got.Head != "abc123" || got.Round != 2 || got.Verdict != "pass" {
		t.Fatalf("receipt = %#v", got)
	}
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

func TestDocumentedPositionalFlagOrderParses(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "handoff", run: func() error { return application.runHandoff([]string{"1", "--body-file", "missing"}) }},
		{name: "pr open", run: func() error {
			return application.openPullRequest([]string{"1", "--title", "title", "--body-file", "missing"})
		}},
		{name: "evaluation", run: func() error {
			return application.recordEvaluation([]string{"1", "--verdict", "pass", "--body-file", "missing"})
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
