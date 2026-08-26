package workflowctl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func TestHistoryEvaluationMetricsUseValidatedReceiptsAndStableOrder(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	firstChallenge := historyTestChallenge(t, "pr42-round1", 42, "head-42", base.Add(time.Hour), trustedActor)
	firstReceipt := historyTestEvaluation(t, firstChallenge, "run-42-1", 1, "fail", 2,
		historyTestCurrentBase64, trustedActor, base.Add(2*time.Hour))
	firstReceipt.Body = legacyTransportMismatch(t, firstReceipt.Body)
	repair := historyTestRepair(t, firstReceipt.Body, base.Add(3*time.Hour))
	secondChallenge := historyTestChallenge(t, "pr42-round2", 42, "head-42", base.Add(4*time.Hour), trustedActor)
	secondReceipt := historyTestEvaluation(t, secondChallenge, "run-42-2", 2, "pass", 0,
		historyTestLegacyRaw, trustedActor, base.Add(5*time.Hour))
	untrustedChallenge := historyTestChallenge(t, "untrusted", 42, "head-42", base.Add(6*time.Hour), owner)
	pr42Comments := []pullRequestComment{secondChallenge, secondReceipt, firstChallenge, firstReceipt, repair, untrustedChallenge}

	pr43Challenge := historyTestChallenge(t, "pr43-round1", 43, "head-43", base.Add(time.Hour), historyLegacyTrustedActor)
	pr43Receipt := historyTestEvaluation(t, pr43Challenge, "run-43-1", 1, "pass", 0,
		historyTestLegacyBase64, historyLegacyTrustedActor, base.Add(2*time.Hour))
	pr43Comments := []pullRequestComment{pr43Challenge, pr43Receipt}

	pr42JSON := historyTestCommentJSON(t, pr42Comments)
	pr43JSON := historyTestCommentJSON(t, pr43Comments)
	seenCommands := make([]string, 0, 2)
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		seenCommands = append(seenCommands, command)
		if command == fmt.Sprintf("gh api --paginate repos/%s/issues/42/comments?per_page=100", repositoryKey) {
			return pr42JSON, nil
		}
		if command == fmt.Sprintf("gh api --paginate repos/%s/issues/43/comments?per_page=100", repositoryKey) {
			return pr43JSON, nil
		}
		return "", fmt.Errorf("unexpected command: %s", command)
	}}
	pullRequests := []pullRequestSummary{
		{Number: 42, CreatedAt: base, MergedAt: base.Add(10 * time.Hour)},
		{Number: 43, CreatedAt: base, MergedAt: base.Add(9 * time.Hour)},
	}
	packets, err := application.collectHistoryEvaluations("/repo", pullRequests)
	if err != nil {
		t.Fatalf("collectHistoryEvaluations: %v", err)
	}
	if got, want := len(packets), 2; got != want {
		t.Fatalf("evaluated packet count = %d, want %d", got, want)
	}
	metrics := historyEvaluationMetricsFor(packets)
	wantMetrics := historyEvaluationMetrics{
		evaluatedPackets: 2, firstPassPackets: 2, remediatedPackets: 0, finalPasses: 1,
		totalRounds: 3, failedRounds: 1, blockingFindings: 2,
	}
	if metrics != wantMetrics {
		t.Fatalf("evaluation metrics = %#v, want %#v", metrics, wantMetrics)
	}
	if got, want := []int{packets[0].rounds[0].round, packets[0].rounds[1].round}, []int{2, 1}; !equalInts(got, want) {
		t.Fatalf("PR #42 validated receipt order = %v, want %v", got, want)
	}
	var rendered bytes.Buffer
	if err := renderEvaluationHistory(&rendered, packets, 1); err != nil {
		t.Fatalf("renderEvaluationHistory: %v", err)
	}
	output := rendered.String()
	for _, want := range []string{
		"Evaluated packets: 2",
		"First-pass packets: 2",
		"Remediated packets: 0",
		"Final passes: 1",
		"Total rounds: 3",
		"Failed rounds: 1",
		"Blocking findings: 2",
		fmt.Sprintf("#42: round 1 fail (2 blocking findings; summary=%s); round 2 pass",
			strconv.Quote("History fail round 1 \\u001e.")),
		"Omitted: 1 evaluated packet(s) beyond --limit 1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("evaluation report missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "#43:") {
		t.Fatalf("post-limit detail included PR #43:\n%s", output)
	}
	if len(seenCommands) != 2 {
		t.Fatalf("history evaluation commands = %v, want two read-only comment reads", seenCommands)
	}
}

func TestHistoryRetainsAttestationSummaryAndSuppressesPassSummary(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	failedSummary := "The exact failed summary remains trusted data."
	passSummary := "The exact pass summary remains hidden in history detail."
	failedChallenge := historyTestChallenge(t, "summary-fail", 60, "head-60", base, trustedActor)
	failedReceipt := historyTestEvaluationWithSummary(t, failedChallenge, "summary-fail-run", 1, "fail", 2,
		failedSummary, historyTestCurrentBase64, trustedActor, base.Add(time.Minute))
	passChallenge := historyTestChallenge(t, "summary-pass", 60, "head-60", base.Add(2*time.Minute), trustedActor)
	passReceipt := historyTestEvaluationWithSummary(t, passChallenge, "summary-pass-run", 2, "pass", 0,
		passSummary, historyTestCurrentBase64, trustedActor, base.Add(3*time.Minute))
	history, err := parseEvaluationHistory([]pullRequestComment{failedChallenge, failedReceipt, passChallenge, passReceipt})
	if err != nil {
		t.Fatalf("parse summary history: %v", err)
	}
	if validationErr := validateEvaluationHistory(history); validationErr != nil {
		t.Fatalf("validate summary history: %v", validationErr)
	}
	packet, err := historyEvaluationPacketForPR(
		pullRequestSummary{Number: 60, MergedAt: base.Add(time.Hour)}, history)
	if err != nil {
		t.Fatalf("historyEvaluationPacketForPR: %v", err)
	}
	if got, want := len(packet.rounds), 2; got != want {
		t.Fatalf("summary round count = %d, want %d", got, want)
	}
	if got := packet.rounds[0].attestationSummary; got != failedSummary {
		t.Fatalf("failed attestation summary = %q, want %q", got, failedSummary)
	}
	if got := packet.rounds[1].attestationSummary; got != passSummary {
		t.Fatalf("pass attestation summary = %q, want %q", got, passSummary)
	}

	var report bytes.Buffer
	if err := renderEvaluationHistory(&report, []historyEvaluationPacket{packet}, 1); err != nil {
		t.Fatalf("render summary history: %v", err)
	}
	output := report.String()
	wantDetail := fmt.Sprintf("#60: round 1 fail (2 blocking findings; summary=%s); round 2 pass",
		strconv.Quote(failedSummary))
	if !strings.Contains(output, wantDetail) {
		t.Fatalf("failed summary detail missing:\n%s", output)
	}
	if strings.Contains(output, passSummary) || strings.Contains(output, "summary="+strconv.Quote(passSummary)) {
		t.Fatalf("pass summary leaked into history detail:\n%s", output)
	}
}

func TestHistoryEscapesAttestationSummaryAsOneReportRecord(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	controlBytes := make([]byte, 0, 32)
	for value := byte(0); value <= 0x1f; value++ {
		controlBytes = append(controlBytes, value)
	}
	summary := "prefix\ncarriage\rcontrols:" + string(controlBytes) + " quote=\" slash=\\ separators=\u2028\u2029"
	challenge := historyTestChallenge(t, "summary-escaping", 61, "head-61", base, trustedActor)
	receipt := historyTestEvaluationWithSummary(t, challenge, "summary-escaping-run", 1, "fail", 1,
		summary, historyTestCurrentBase64, trustedActor, base.Add(time.Minute))
	history, err := parseEvaluationHistory([]pullRequestComment{challenge, receipt})
	if err != nil {
		t.Fatalf("parse escaping history: %v", err)
	}
	if validationErr := validateEvaluationHistory(history); validationErr != nil {
		t.Fatalf("validate escaping history: %v", validationErr)
	}
	packet, err := historyEvaluationPacketForPR(
		pullRequestSummary{Number: 61, MergedAt: base.Add(time.Hour)}, history)
	if err != nil {
		t.Fatalf("historyEvaluationPacketForPR: %v", err)
	}
	if got := packet.rounds[0].attestationSummary; got != summary {
		t.Fatalf("escaped attestation summary = %q, want exact decoded summary %q", got, summary)
	}

	var report bytes.Buffer
	if err := renderEvaluationHistory(&report, []historyEvaluationPacket{packet}, 1); err != nil {
		t.Fatalf("render escaping history: %v", err)
	}
	output := report.String()
	wantDetail := fmt.Sprintf("  - #61: round 1 fail (1 blocking findings; summary=%s)\n", strconv.Quote(summary))
	if !strings.Contains(output, wantDetail) {
		t.Fatalf("escaped summary detail missing:\n%q", output)
	}
	if strings.Contains(output, summary) {
		t.Fatalf("raw summary bytes appeared in report:\n%q", output)
	}
	if got := strings.Count(output, "  - #61:"); got != 1 {
		t.Fatalf("report record count for PR #61 = %d, want 1:\n%q", got, output)
	}
}

func TestHistoryEvaluationDetailsSortRoundsAndLimitPackets(t *testing.T) {
	packets := []historyEvaluationPacket{
		{
			number: 70,
			rounds: []historyEvaluationRound{
				{round: 3, verdict: "fail", blockingFindings: 3, findingEvidence: true, attestationSummary: "PR 70 round 3"},
				{round: 1, verdict: "fail", blockingFindings: 1, findingEvidence: true, attestationSummary: "PR 70 round 1"},
			},
		},
		{
			number: 71,
			rounds: []historyEvaluationRound{
				{round: 2, verdict: "fail", blockingFindings: 2, findingEvidence: true, attestationSummary: "PR 71 round 2"},
			},
		},
	}

	metrics := historyEvaluationMetricsFor(packets)
	wantMetrics := historyEvaluationMetrics{evaluatedPackets: 2, totalRounds: 3, failedRounds: 3, blockingFindings: 6}
	if metrics != wantMetrics {
		t.Fatalf("full-window evaluation metrics = %#v, want %#v", metrics, wantMetrics)
	}
	var report bytes.Buffer
	if err := renderEvaluationHistory(&report, packets, 1); err != nil {
		t.Fatalf("render limited evaluation history: %v", err)
	}
	output := report.String()
	wantFirst := "#70: round 1 fail (1 blocking findings; summary=\"PR 70 round 1\"); round 3 fail (3 blocking findings; summary=\"PR 70 round 3\")"
	if !strings.Contains(output, wantFirst) {
		t.Fatalf("round order or first packet detail changed:\n%s", output)
	}
	if strings.Contains(output, "#71:") || strings.Contains(output, "PR 71 round 2") {
		t.Fatalf("limited detail included packet #71:\n%s", output)
	}
	if !strings.Contains(output, "Omitted: 1 evaluated packet(s) beyond --limit 1") {
		t.Fatalf("limited packet omission missing:\n%s", output)
	}
}

func TestHistoryExcludesReceiptRecordedAfterMerge(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	mergedAt := base.Add(2 * time.Hour)
	preMergeChallenge := historyTestChallenge(t, "pre-merge", 44, "head-44", base, trustedActor)
	preMergeReceipt := historyTestEvaluation(t, preMergeChallenge, "pre-merge-run", 1, "pass", 0,
		historyTestCurrentBase64, trustedActor, base.Add(time.Hour))
	lateChallenge := historyTestChallenge(t, "recorded-late", 44, "head-44", base.Add(118*time.Minute), trustedActor)
	lateReceipt := historyTestEvaluation(t, lateChallenge, "recorded-late-run", 2, "fail", 1,
		historyTestCurrentBase64, trustedActor, base.Add(119*time.Minute))
	lateReceipt.Body = replaceTestReceipt(t, lateReceipt.Body, func(receipt *evaluationReceipt) {
		receipt.RecordedAt = mergedAt.Add(time.Minute)
	})
	history, err := parseEvaluationHistory([]pullRequestComment{
		preMergeChallenge, preMergeReceipt, lateChallenge, lateReceipt,
	})
	if err != nil {
		t.Fatalf("parse merge-boundary history: %v", err)
	}
	if validationErr := validateEvaluationHistory(history); validationErr != nil {
		t.Fatalf("validate merge-boundary history: %v", validationErr)
	}
	packet, err := historyEvaluationPacketForPR(
		pullRequestSummary{Number: 44, MergedAt: mergedAt}, history)
	if err != nil {
		t.Fatalf("historyEvaluationPacketForPR: %v", err)
	}
	if len(packet.rounds) != 1 || packet.rounds[0].round != 1 {
		t.Fatalf("pre-merge rounds = %#v, want only round 1", packet.rounds)
	}
	metrics := historyEvaluationMetricsFor([]historyEvaluationPacket{packet})
	if metrics.totalRounds != 1 || metrics.failedRounds != 0 || metrics.blockingFindings != 0 {
		t.Fatalf("merge-boundary metrics = %#v, want only the valid pre-merge pass", metrics)
	}
}

func TestHistoryIgnoresUntrustedEvaluationLookalikes(t *testing.T) {
	requested := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	trustedChallenge := historyTestChallenge(t, "trusted", 50, "head", requested, trustedActor)
	trustedReceipt := historyTestEvaluation(t, trustedChallenge, "trusted-run", 1, "pass", 0,
		historyTestCurrentBase64, trustedActor, requested.Add(time.Minute))
	lookalikeChallenge := historyTestChallenge(t, "lookalike", 50, "head", requested.Add(2*time.Minute), owner)
	lookalikeReceipt := historyTestEvaluation(t, lookalikeChallenge, "lookalike-run", 2, "fail", 3,
		historyTestCurrentBase64, owner, requested.Add(3*time.Minute))
	comments := historyTrustedComments([]pullRequestComment{
		lookalikeChallenge, lookalikeReceipt, trustedChallenge, trustedReceipt,
	})
	if len(comments) != 2 {
		t.Fatalf("trusted history comment count = %d, want 2", len(comments))
	}
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		t.Fatalf("parse filtered history: %v", err)
	}
	if err := validateEvaluationHistory(history); err != nil {
		t.Fatalf("validate filtered history: %v", err)
	}
	if len(history.receipts) != 1 || history.receipts[0].receipt.Round != 1 {
		t.Fatalf("filtered receipts = %#v, want only trusted round 1", history.receipts)
	}
}

func TestHistoryHistoricalActorDoesNotAuthorizeMergePass(t *testing.T) {
	requested := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	challenge := historyTestChallenge(t, "historical", 51, "head", requested, historyLegacyTrustedActor)
	receipt := historyTestEvaluation(t, challenge, "historical-run", 1, "pass", 0,
		historyTestCurrentBase64, historyLegacyTrustedActor, requested.Add(time.Minute))
	passes, err := latestEvaluationPasses(pullRequestView{
		Comments:   []pullRequestComment{challenge, receipt},
		HeadRefOID: "head",
	}, 51)
	if err == nil && passes {
		t.Fatal("historical actor receipt authorized a merge pass")
	}
}

func TestHistoryLegacyReceiptWithoutAttestationDoesNotClaimFindings(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	challenge := historyTestChallenge(t, "legacy-no-attestation", 52, "head", base, trustedActor)
	receipt := historyTestNoAttestation(t, challenge, 1, "fail", base.Add(time.Minute), trustedActor)
	history, err := parseEvaluationHistory([]pullRequestComment{challenge, receipt})
	if err != nil {
		t.Fatalf("parse legacy no-attestation history: %v", err)
	}
	if validationErr := validateEvaluationHistory(history); validationErr != nil {
		t.Fatalf("validate legacy no-attestation history: %v", validationErr)
	}
	packet, err := historyEvaluationPacketForPR(
		pullRequestSummary{Number: 52, MergedAt: base.Add(time.Hour)}, history)
	if err != nil {
		t.Fatalf("historyEvaluationPacketForPR: %v", err)
	}
	metrics := historyEvaluationMetricsFor([]historyEvaluationPacket{packet})
	if metrics.blockingFindings != 0 || metrics.missingFindingEvidenceRounds != 1 {
		t.Fatalf("legacy no-attestation metrics = %#v, want unknown findings", metrics)
	}
	var report bytes.Buffer
	if err := renderEvaluationHistory(&report, []historyEvaluationPacket{packet}, 1); err != nil {
		t.Fatalf("render legacy no-attestation history: %v", err)
	}
	if !strings.Contains(report.String(), "Blocking findings: unavailable") ||
		strings.Contains(report.String(), "Blocking findings: 0") ||
		strings.Contains(report.String(), "summary=") {
		t.Fatalf("legacy no-attestation report claimed zero findings:\n%s", report.String())
	}
}

func TestHistoryNoEvaluationWindowStatesAbsence(t *testing.T) {
	window := historyWindow{
		since: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		until: time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC),
	}
	withMergedPR := historySnapshot{
		window: window,
		pullRequests: []pullRequestSummary{{
			Number: 53, CreatedAt: window.since, MergedAt: window.since.Add(time.Hour), Title: "merged",
		}},
	}
	report, err := renderHistory(withMergedPR, 1)
	if err != nil {
		t.Fatalf("render no-evaluation merged window: %v", err)
	}
	if !strings.Contains(report, "- #53 merged") ||
		!strings.Contains(report, "No validated pre-merge Examiner evaluations in this history window") {
		t.Fatalf("no-evaluation merged window was not explicit:\n%s", report)
	}
	withoutMergedPR := historySnapshot{window: window}
	report, err = renderHistory(withoutMergedPR, 1)
	if err != nil {
		t.Fatalf("render no-merged-PR window: %v", err)
	}
	if !strings.Contains(report, "## Merged pull requests\n- None") ||
		!strings.Contains(report, "No validated pre-merge Examiner evaluations in this history window") {
		t.Fatalf("no-merged-PR window did not distinguish evaluation absence:\n%s", report)
	}
}

func TestHistoryRejectsMalformedTrustedLateEvidenceWithoutPartialReport(t *testing.T) {
	base := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	firstChallenge := historyTestChallenge(t, "late-round1", 54, "head", base, trustedActor)
	firstReceipt := historyTestEvaluation(t, firstChallenge, "late-run1", 1, "pass", 0,
		historyTestCurrentBase64, trustedActor, base.Add(time.Minute))
	lateChallenge := historyTestChallenge(t, "late-round2", 54, "head", base.Add(2*time.Minute), trustedActor)
	lateReceipt := historyTestEvaluation(t, lateChallenge, "late-run2", 2, "pass", 0,
		historyTestCurrentBase64, trustedActor, base.Add(3*time.Minute))
	lateReceipt.Body = replaceTestReceipt(t, lateReceipt.Body, func(receipt *evaluationReceipt) {
		receipt.ReportSHA256 = strings.Repeat("0", 64)
	})
	comments := historyTestCommentJSON(t, []pullRequestComment{firstChallenge, firstReceipt, lateChallenge, lateReceipt})
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
		case strings.HasPrefix(command, "gh pr list "):
			return `[{
  "number":54,
  "title":"late evidence",
  "createdAt":"2026-08-10T00:00:00Z",
  "mergedAt":"2026-08-10T01:00:00Z",
  "url":"https://example.test/54"
}]`, nil
		case strings.HasPrefix(command, "gh api --paginate repos/goxdra/goxsd9/issues/54/comments?per_page=100"):
			return comments, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	err := application.runHistory([]string{
		"--since", "2026-08-10T00:00:00Z",
		"--until", "2026-08-11T00:00:00Z",
		"--limit", "1",
	})
	if err == nil || !strings.Contains(err.Error(), "PR #54") || !strings.Contains(err.Error(), "round 2") {
		t.Fatalf("malformed late evidence error = %v, want PR and round context", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("history wrote partial output after malformed trusted evidence:\n%s", stdout.String())
	}
}

const (
	historyTestCurrentBase64 = "current-base64"
	historyTestLegacyRaw     = "legacy-raw"
	historyTestLegacyBase64  = "legacy-base64"
)

func historyTestChallenge(t *testing.T, id string, number int, head string, requestedAt time.Time, actor string) pullRequestComment {
	t.Helper()
	challenge := evaluationChallenge{Challenge: id, Head: head, PR: number, RequestedAt: requestedAt}
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode history challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      fmt.Sprintf("<!-- %s%s -->\n", evaluationChallengeMarker, marker),
		CreatedAt: requestedAt,
	}
	comment.Author.Login = actor
	return comment
}

func historyTestEvaluation(t *testing.T, challenge pullRequestComment, runID string, round int, verdict string,
	findingCount int, format, actor string, recordedAt time.Time) pullRequestComment {
	return historyTestEvaluationWithSummary(t, challenge, runID, round, verdict, findingCount,
		fmt.Sprintf("History %s round %d \\u001e.", verdict, round), format, actor, recordedAt)
}

func historyTestEvaluationWithSummary(t *testing.T, challenge pullRequestComment, runID string, round int, verdict string,
	findingCount int, summary, format, actor string, recordedAt time.Time) pullRequestComment {
	t.Helper()
	parsedChallenge, ok := parseEvaluationChallenge(challenge.Body)
	if !ok {
		t.Fatal("history challenge fixture was not parseable")
	}
	findings := make(evaluationFindings, findingCount)
	for index := range findings {
		findings[index] = evaluationFinding{
			Impact:             fmt.Sprintf("History impact %d", index+1),
			Location:           fmt.Sprintf("history/location/%d", index+1),
			RequiredCorrection: fmt.Sprintf("History correction %d", index+1),
		}
	}
	attestation := evaluationAttestation{
		Challenge: parsedChallenge.Challenge,
		Evaluator: "Examiner",
		Findings:  findings,
		Head:      parsedChallenge.Head,
		PR:        parsedChallenge.PR,
		RunID:     runID,
		Schema:    evaluationAttestationSchema,
		Summary:   summary,
		Verdict:   verdict,
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		t.Fatalf("encode history attestation: %v", err)
	}
	report := canonicalEvaluationReport(renderEvaluationReport(attestation))
	receipt := evaluationReceipt{
		AttestationSHA256: sha256Hex(attestationJSON),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        recordedAt,
		ReportSHA256:      sha256Hex(report),
		Round:             round,
		Verdict:           verdict,
	}
	if format == historyTestCurrentBase64 {
		receipt.ReportTransport = evaluationReportTransportV1
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode history receipt: %v", err)
	}
	body := historyTestEvaluationBody(t, receiptJSON, attestationJSON, report, format)
	comment := pullRequestComment{Body: body, CreatedAt: recordedAt}
	comment.Author.Login = actor
	return comment
}

func historyTestEvaluationBody(t *testing.T, receiptJSON, attestationJSON, report []byte, format string) string {
	t.Helper()
	receiptMarker := fmt.Sprintf("<!-- %s%s -->\n", evaluationMarker, receiptJSON)
	reportText := fmt.Sprintf("%s%s\n", evaluationReceiptHeading, report)
	switch format {
	case historyTestCurrentBase64:
		return evaluationComment(receiptJSON, attestationJSON, string(report))
	case historyTestLegacyRaw:
		return receiptMarker + fmt.Sprintf("<!-- %s%s -->\n", evaluationAttestationMarker, attestationJSON) + reportText
	case historyTestLegacyBase64:
		encoded := base64.StdEncoding.EncodeToString(attestationJSON)
		return receiptMarker + fmt.Sprintf("<!-- %s%s -->\n", evaluationAttestationBase64Marker, encoded) + reportText
	default:
		t.Fatalf("unknown history evaluation fixture format %q", format)
		return ""
	}
}

func historyTestNoAttestation(t *testing.T, challenge pullRequestComment, round int, verdict string,
	recordedAt time.Time, actor string) pullRequestComment {
	t.Helper()
	parsedChallenge, ok := parseEvaluationChallenge(challenge.Body)
	if !ok {
		t.Fatal("history no-attestation challenge fixture was not parseable")
	}
	report := canonicalEvaluationReport(strings.ToUpper(verdict) + " legacy report")
	receipt := evaluationReceipt{
		Evaluator:    "Examiner",
		Head:         parsedChallenge.Head,
		RecordedAt:   recordedAt,
		ReportSHA256: sha256Hex(report),
		Round:        round,
		Verdict:      verdict,
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encode history no-attestation receipt: %v", err)
	}
	body := fmt.Sprintf("<!-- %s%s -->\n%s%s\n", evaluationMarker, receiptJSON, evaluationReceiptHeading, report)
	comment := pullRequestComment{Body: body, CreatedAt: recordedAt}
	comment.Author.Login = actor
	return comment
}

func historyTestRepair(t *testing.T, body string, createdAt time.Time) pullRequestComment {
	t.Helper()
	apiComment := testRepairComment(t, body, nil)
	comment := pullRequestComment{
		Body:      apiComment.Body,
		CreatedAt: createdAt,
	}
	comment.Author.Login = apiComment.User.Login
	return comment
}

func historyTestCommentJSON(t *testing.T, comments []pullRequestComment) string {
	t.Helper()
	responses := make([]issueCommentAPI, 0, len(comments))
	for _, comment := range comments {
		response := issueCommentAPI{Body: comment.Body, CreatedAt: comment.CreatedAt}
		response.User.Login = comment.Author.Login
		responses = append(responses, response)
	}
	data, err := json.Marshal(responses)
	if err != nil {
		t.Fatalf("encode history comments: %v", err)
	}
	return string(data)
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
