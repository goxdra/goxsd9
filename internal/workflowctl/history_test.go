package workflowctl

import (
	"bytes"
	"fmt"
	"io"
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
