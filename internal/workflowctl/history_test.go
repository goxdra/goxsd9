package workflowctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryDoesNotWritePartialReportAfterLateError(t *testing.T) {
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: func(_ string, _ io.Reader, name string,
		args ...string,
	) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch {
		case command == "git rev-parse --show-toplevel":
			return "/repo", nil
		case strings.HasPrefix(command, "git log "):
			return "", nil
		case strings.HasPrefix(command, "git rev-list "):
			return "", nil
		case strings.HasPrefix(command, "gh pr list "):
			return "{", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}

	err := application.runHistory([]string{
		"--since", "2026-08-10T00:00:00Z",
		"--until", "2026-08-11T00:00:00Z",
		"--limit", "1",
	})
	if err == nil || !strings.Contains(err.Error(), "decode pull requests") {
		t.Fatalf("runHistory error = %v, want pull request decode error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runHistory wrote partial output after late error:\n%s", stdout.String())
	}
}

func TestGitHistoryFiltersSubsecondCommitterBounds(t *testing.T) {
	window := historyWindow{
		since: time.Date(2026, time.August, 16, 17, 59, 11, 1, time.UTC),
		until: time.Date(2026, time.August, 16, 17, 59, 12, 0, time.UTC),
	}
	output := strings.Join([]string{
		gitMetadataRecord("before", "2026-08-16T17:59:11Z"),
		gitMetadataRecord("exact", "2026-08-16T17:59:11.000000001Z"),
		gitMetadataRecord("after", "2026-08-16T17:59:11.000000002Z"),
		gitMetadataRecord("upper", "2026-08-16T17:59:12.000000001Z"),
	}, "\n")
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name == "git" && len(args) > 0 && args[0] == "log" {
			return output, nil
		}
		if name == "git" && len(args) > 0 && args[0] == "show" {
			return gitRenderedCandidate(args[len(args)-2]), nil
		}
		return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
	}}
	candidates, err := application.collectGitCandidates("/repo", window)
	if err != nil {
		t.Fatalf("collectGitCandidates: %v", err)
	}
	if got := len(candidates); got != 2 {
		t.Fatalf("candidate count = %d, want 2", got)
	}
	if candidates[0].id != "exact" || candidates[1].id != "after" {
		t.Fatalf("candidate IDs = [%s %s], want [exact after]", candidates[0].id, candidates[1].id)
	}
	if !strings.Contains(candidates[0].rendered, "  body for exact\n  second body line") {
		t.Fatalf("rendered commit body missing:\n%s", candidates[0].rendered)
	}
}

func TestDocumentationHistoryUsesExactSubsecondCommitterBounds(t *testing.T) {
	window := historyWindow{
		since: time.Date(2026, time.August, 16, 17, 59, 11, 1, time.UTC),
		until: time.Date(2026, time.August, 16, 17, 59, 12, 0, time.UTC),
	}
	output := strings.Join([]string{
		gitMetadataRecord("before", "2026-08-16T17:59:11Z"),
		gitMetadataRecord("exact", "2026-08-16T17:59:11.000000001Z"),
		gitMetadataRecord("after", "2026-08-16T17:59:11.000000002Z"),
		gitMetadataRecord("upper", "2026-08-16T17:59:12.000000001Z"),
	}, "\n")
	seen := make([]string, 0, 2)
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name == "git" && len(args) > 0 && args[0] == "log" {
			return output, nil
		}
		if name == "git" && len(args) > 0 && args[0] == "show" {
			return gitRenderedCandidate(args[len(args)-2]), nil
		}
		if name == "git" && len(args) > 0 && args[0] == "diff-tree" {
			commit := args[len(args)-2]
			seen = append(seen, commit)
			return "1\t0\tREADME.md\x00", nil
		}
		return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
	}}
	totals, err := application.readDocumentationChurnWindow("/repo", window.since, window.until)
	if err != nil {
		t.Fatalf("readDocumentationChurnWindow: %v", err)
	}
	if !equalStrings(seen, []string{"exact", "after"}) {
		t.Fatalf("documentation candidates = %v, want [exact after]", seen)
	}
	if len(totals) != 1 || totals[0].path != "README.md" || totals[0].additions != 2 {
		t.Fatalf("documentation totals = %#v, want README.md +2", totals)
	}
}

func TestGitHistoryPreservesRecordSeparatorsInMessages(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	runGitTest(t, root, "config", "user.name", "Workflow Test")
	runGitTest(t, root, "config", "user.email", "workflow@example.test")
	writeTestDocument(t, root, "README.md", "base\n")
	runGitTest(t, root, "add", "README.md")
	runGitTestWithDates(t, root, "2026-08-15T12:00:00Z", "commit", "--no-gpg-sign", "-m", "base")

	messagePath := filepath.Join(root, "message.txt")
	message := []byte("included\x1esubject\n\nbody before\x1ebody after\n")
	if err := os.WriteFile(messagePath, message, 0o600); err != nil {
		t.Fatalf("write included commit message: %v", err)
	}
	writeTestDocument(t, root, "README.md", "included\n")
	runGitTest(t, root, "add", "README.md")
	runGitTestWithDates(t, root, "2026-08-16T12:00:00Z", "commit", "--no-gpg-sign", "-F", messagePath)
	includedID := runGitTest(t, root, "rev-parse", "HEAD")

	message = []byte("out\x1esubject\n\nbody out before\x1ebody out after\n")
	if err := os.WriteFile(messagePath, message, 0o600); err != nil {
		t.Fatalf("write out-of-window commit message: %v", err)
	}
	writeTestDocument(t, root, "README.md", "out\n")
	runGitTest(t, root, "add", "README.md")
	runGitTestWithDates(t, root, "2026-08-17T12:00:00Z", "commit", "--no-gpg-sign", "-F", messagePath)

	window := historyWindow{
		since: time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC),
		until: time.Date(2026, time.August, 16, 23, 59, 59, 0, time.UTC),
	}
	application := app{ctx: context.Background()}
	candidates, err := application.collectGitCandidates(root, window)
	if err != nil {
		t.Fatalf("collectGitCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].id != includedID {
		t.Fatalf("candidates = %#v, want only included commit %s", candidates, includedID)
	}
	if !strings.Contains(candidates[0].rendered, "included\x1esubject") ||
		!strings.Contains(candidates[0].rendered, "body before\x1ebody after") {
		t.Fatalf("included record separators were not preserved:\n%s", candidates[0].rendered)
	}
	if strings.Contains(candidates[0].rendered, "out\x1esubject") {
		t.Fatalf("out-of-window commit was rendered:\n%s", candidates[0].rendered)
	}

	totals, err := application.readDocumentationChurnCandidates(root, candidates)
	if err != nil {
		t.Fatalf("readDocumentationChurnCandidates: %v", err)
	}
	if len(totals) != 1 || totals[0].path != "README.md" {
		t.Fatalf("documentation totals = %#v, want included README.md change", totals)
	}
}

func TestHistoryFiltersBoundsSortsAndLimitsAfterFiltering(t *testing.T) {
	window := historyWindow{
		since: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		until: time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC),
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch {
		case strings.HasPrefix(command, "gh pr list "):
			return `[
  {"number":12,"title":"same later number","createdAt":"2026-08-09T00:00:00Z","mergedAt":"2026-08-10T12:00:00Z","url":"https://example.test/12"},
  {"number":11,"title":"same earlier number","createdAt":"2026-08-10T00:00:00Z","mergedAt":"2026-08-10T12:00:00Z","url":"https://example.test/11"},
  {"number":10,"title":"at upper bound","createdAt":"2026-08-10T23:00:00Z","mergedAt":"2026-08-11T00:00:00Z","url":"https://example.test/10"},
  {"number":9,"title":"outside","createdAt":"2026-08-08T00:00:00Z","mergedAt":"2026-08-09T23:59:59Z","url":"https://example.test/9"}
]`, nil
		case strings.HasPrefix(command, "gh issue list "):
			return `[
  {"number":4,"title":"same later number","state":"OPEN","updatedAt":"2026-08-10T12:00:00Z","url":"https://example.test/issues/4"},
  {"number":3,"title":"same earlier number","state":"CLOSED","updatedAt":"2026-08-10T12:00:00Z","url":"https://example.test/issues/3"},
  {"number":2,"title":"at upper bound","state":"OPEN","updatedAt":"2026-08-11T00:00:00Z","url":"https://example.test/issues/2"},
  {"number":1,"title":"outside","state":"OPEN","updatedAt":"2026-08-11T00:00:01Z","url":"https://example.test/issues/1"}
]`, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}

	prs, err := application.collectPRHistory("/repo", window)
	if err != nil {
		t.Fatalf("collectPRHistory: %v", err)
	}
	if got := []int{prs[0].Number, prs[1].Number, prs[2].Number}; !equalInts(got, []int{10, 11, 12}) {
		t.Fatalf("PR order = %v, want [10 11 12]", got)
	}
	issues, err := application.collectIssueHistory("/repo", window)
	if err != nil {
		t.Fatalf("collectIssueHistory: %v", err)
	}
	if got := []int{issues[0].Number, issues[1].Number, issues[2].Number}; !equalInts(got, []int{2, 3, 4}) {
		t.Fatalf("issue order = %v, want [2 3 4]", got)
	}

	snapshot := historySnapshot{window: window, pullRequests: prs, issues: issues}
	report, err := renderHistory(snapshot, 1)
	if err != nil {
		t.Fatalf("renderHistory: %v", err)
	}
	if !strings.Contains(report, "- #10 at upper bound") || strings.Contains(report, "- #11 same earlier number") {
		t.Fatalf("post-filter PR limit not applied:\n%s", report)
	}
	if !strings.Contains(report, "Omitted: 2 merged pull request(s) beyond --limit 1") {
		t.Fatalf("PR omission count missing:\n%s", report)
	}
	if !strings.Contains(report, "- #2 [open] at upper bound") || strings.Contains(report, "- #3 [closed] same earlier number") {
		t.Fatalf("post-filter issue limit not applied:\n%s", report)
	}
	if !strings.Contains(report, "Omitted: 2 issue(s) beyond --limit 1") {
		t.Fatalf("issue omission count missing:\n%s", report)
	}
}

func TestHistoryLeadTimeMedianOddAndEven(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	makePR := func(number int, duration time.Duration) pullRequestSummary {
		return pullRequestSummary{Number: number, CreatedAt: base, MergedAt: base.Add(duration)}
	}
	tests := []struct {
		name  string
		items []pullRequestSummary
		want  time.Duration
	}{
		{name: "odd", items: []pullRequestSummary{makePR(1, 9*time.Second), makePR(2, time.Second), makePR(3, 3*time.Second)}, want: 3 * time.Second},
		{name: "even", items: []pullRequestSummary{makePR(1, time.Second), makePR(2, 3*time.Second)}, want: 2 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := medianLeadTime(test.items)
			if !ok || got != test.want {
				t.Fatalf("medianLeadTime = (%s, %t), want (%s, true)", got, ok, test.want)
			}
		})
	}
}

func TestHistoryRejectsMalformedAPIData(t *testing.T) {
	window := historyWindow{since: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC), until: time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)}
	tests := []struct {
		name    string
		command string
		output  string
		want    string
	}{
		{name: "PR created timestamp", command: "gh pr list", output: `[{"number":1,"title":"bad","createdAt":null,"mergedAt":"2026-08-10T00:00:00Z"}]`, want: "missing createdAt"},
		{name: "issue updated timestamp", command: "gh issue list", output: `[{"number":1,"title":"bad","state":"OPEN","updatedAt":"not-a-time"}]`, want: "decode issues"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				if strings.HasPrefix(name+" "+strings.Join(args, " "), test.command) {
					return test.output, nil
				}
				return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
			}}
			var err error
			switch test.name {
			case "PR created timestamp":
				_, err = application.collectPRHistory("/repo", window)
			case "issue updated timestamp":
				_, err = application.collectIssueHistory("/repo", window)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestHistoryRenderingIsByteIdentical(t *testing.T) {
	window := historyWindow{
		since: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		until: time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC),
	}
	snapshot := historySnapshot{
		window:        window,
		commits:       []string{"- abc 2026-08-10 commit"},
		documentation: []documentationChurn{{path: "README.md", additions: 1, deletions: 0}},
		pullRequests:  []pullRequestSummary{{Number: 1, Title: "first", CreatedAt: window.since, MergedAt: window.since.Add(time.Hour)}},
		issues:        []issueSummary{{Number: 1, Title: "issue", State: "OPEN", UpdatedAt: window.since}},
	}
	first, err := renderHistory(snapshot, 30)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := renderHistory(snapshot, 30)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if first != second {
		t.Fatalf("repeated history render changed bytes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func gitMetadataRecord(id, committedAt string) string {
	return id + "\x00" + committedAt
}

func gitRenderedCandidate(id string) string {
	return "- " + id + " 2026-08-16 " + id + "\n  body for " + id + "\n  second body line"
}

func runGitTestWithDates(t *testing.T, root, date string, args ...string) {
	t.Helper()
	// #nosec G204 -- each test supplies repository-local Git arguments without user input.
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
