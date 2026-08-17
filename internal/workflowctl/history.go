package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const historyCollectionLimit = 1000

type pullRequestSummary struct {
	CreatedAt time.Time `json:"createdAt"`
	MergedAt  time.Time `json:"mergedAt"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
}

type issueSummary struct {
	Number    int       `json:"number"`
	State     string    `json:"state"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
}

type historyWindow struct {
	since time.Time
	until time.Time
}

type gitHistoryCandidate struct {
	id          string
	committedAt time.Time
	rendered    string
}

type historySnapshot struct {
	window        historyWindow
	commits       []string
	documentation []documentationChurn
	pullRequests  []pullRequestSummary
	issues        []issueSummary
}

func (a app) runHistory(args []string) error {
	flags := flag.NewFlagSet("history", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sinceText := flags.String("since", "7d", "inclusive history-window start")
	untilText := flags.String("until", "", "inclusive history-window end (date or RFC3339Nano)")
	limit := flags.Int("limit", 30, "maximum entries per section")
	if err := flags.Parse(args); err != nil {
		return usageError("history: %v", err)
	}
	if flags.NArg() != 0 || *limit < 1 || *limit > 100 {
		return usageError("history accepts --since, --until, and --limit 1..100")
	}

	now := time.Now().UTC()
	since, err := parseSince(*sinceText, now)
	if err != nil {
		return usageError("history: %v", err)
	}
	until := now
	if *untilText != "" {
		until, err = parseUntil(*untilText)
		if err != nil {
			return usageError("history: %v", err)
		}
	}
	if until.Before(since) {
		return usageError("history: --until %s is before --since %s", formatHistoryTime(until),
			formatHistoryTime(since))
	}

	root, err := a.root()
	if err != nil {
		return err
	}
	snapshot, err := a.collectHistory(root, historyWindow{since: since, until: until})
	if err != nil {
		return err
	}
	report, err := renderHistory(snapshot, *limit)
	if err != nil {
		return fmt.Errorf("render history: %w", err)
	}
	if _, err := io.WriteString(a.stdout, report); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}

func (a app) collectHistory(root string, window historyWindow) (historySnapshot, error) {
	candidates, err := a.collectGitCandidates(root, window)
	if err != nil {
		return historySnapshot{}, err
	}
	commits := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		commits = append(commits, candidate.rendered)
	}
	documentation, err := a.readDocumentationChurnCandidates(root, candidates)
	if err != nil {
		return historySnapshot{}, fmt.Errorf("collect documentation history: %w", err)
	}
	pullRequests, err := a.collectPRHistory(root, window)
	if err != nil {
		return historySnapshot{}, err
	}
	issues, err := a.collectIssueHistory(root, window)
	if err != nil {
		return historySnapshot{}, err
	}
	return historySnapshot{
		window:        window,
		commits:       commits,
		documentation: documentation,
		pullRequests:  pullRequests,
		issues:        issues,
	}, nil
}

func (a app) collectGitCandidates(root string, window historyWindow) ([]gitHistoryCandidate, error) {
	output, err := a.command(root, "git", "log", "--first-parent",
		"--pretty=format:%H%x00%cI")
	if err != nil {
		return nil, fmt.Errorf("read git history candidates: %w", err)
	}
	if output == "" {
		return nil, nil
	}
	records := strings.Split(strings.TrimSpace(output), "\n")
	candidates := make([]gitHistoryCandidate, 0, len(records))
	for index, record := range records {
		fields := strings.Split(record, "\x00")
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			return nil, fmt.Errorf("parse git history candidates: malformed record %d", index)
		}
		committedAt, err := time.Parse(time.RFC3339Nano, fields[1])
		if err != nil {
			return nil, fmt.Errorf("parse git history candidates: record %d timestamp: %w", index, err)
		}
		committedAt = committedAt.UTC()
		if !inHistoryWindow(committedAt, window) {
			continue
		}
		rendered, err := a.renderGitCandidate(root, fields[0])
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, gitHistoryCandidate{
			id:          fields[0],
			committedAt: committedAt,
			rendered:    rendered,
		})
	}
	return candidates, nil
}

func (a app) renderGitCandidate(root, id string) (string, error) {
	output, err := a.command(root, "git", "show", "-s", "--date=short",
		"--format=- %h %ad %s%n%w(74,2,2)%b%w(0,0,0)", id, "--")
	if err != nil {
		return "", fmt.Errorf("read git commit %s: %w", id, err)
	}
	if output == "" {
		return "", fmt.Errorf("read git commit %s: empty output", id)
	}
	return output, nil
}

func (a app) collectPRHistory(root string, window historyWindow) ([]pullRequestSummary, error) {
	output, err := a.command(root, "gh", "pr", "list", "--repo", repositoryKey, "--state", "merged",
		"--limit", strconv.Itoa(historyCollectionLimit), "--json", "number,title,createdAt,mergedAt,url")
	if err != nil {
		return nil, fmt.Errorf("read pull requests: %w", err)
	}
	var items []pullRequestSummary
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return nil, fmt.Errorf("decode pull requests: %w", err)
	}
	if items == nil {
		return nil, errors.New("decode pull requests: expected an array")
	}
	if len(items) >= historyCollectionLimit {
		return nil, fmt.Errorf("read pull requests: source returned at least %d entries; completeness is unknown", historyCollectionLimit)
	}
	filtered := make([]pullRequestSummary, 0, len(items))
	for index := range items {
		item := &items[index]
		if err := validatePullRequest(*item, index); err != nil {
			return nil, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.MergedAt = item.MergedAt.UTC()
		if !inHistoryWindow(item.MergedAt, window) {
			continue
		}
		filtered = append(filtered, *item)
	}
	sort.Slice(filtered, func(left, right int) bool {
		if !filtered[left].MergedAt.Equal(filtered[right].MergedAt) {
			return filtered[left].MergedAt.After(filtered[right].MergedAt)
		}
		if filtered[left].Number != filtered[right].Number {
			return filtered[left].Number < filtered[right].Number
		}
		if filtered[left].Title != filtered[right].Title {
			return filtered[left].Title < filtered[right].Title
		}
		return filtered[left].URL < filtered[right].URL
	})
	return filtered, nil
}

func validatePullRequest(item pullRequestSummary, index int) error {
	if item.Number < 1 {
		return fmt.Errorf("parse pull requests: entry %d has invalid number %d", index, item.Number)
	}
	if item.CreatedAt.IsZero() {
		return fmt.Errorf("parse pull requests: PR #%d has missing createdAt", item.Number)
	}
	if item.MergedAt.IsZero() {
		return fmt.Errorf("parse pull requests: PR #%d has missing mergedAt", item.Number)
	}
	if item.CreatedAt.After(item.MergedAt) {
		return fmt.Errorf("parse pull requests: PR #%d has createdAt after mergedAt", item.Number)
	}
	return nil
}

func (a app) collectIssueHistory(root string, window historyWindow) ([]issueSummary, error) {
	output, err := a.command(root, "gh", "issue", "list", "--repo", repositoryKey, "--state", "all",
		"--limit", strconv.Itoa(historyCollectionLimit), "--json", "number,title,state,updatedAt,url")
	if err != nil {
		return nil, fmt.Errorf("read issues: %w", err)
	}
	var items []issueSummary
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return nil, fmt.Errorf("decode issues: %w", err)
	}
	if items == nil {
		return nil, errors.New("decode issues: expected an array")
	}
	if len(items) >= historyCollectionLimit {
		return nil, fmt.Errorf("read issues: source returned at least %d entries; completeness is unknown", historyCollectionLimit)
	}
	filtered := make([]issueSummary, 0, len(items))
	for index := range items {
		item := &items[index]
		if err := validateIssue(*item, index); err != nil {
			return nil, err
		}
		item.UpdatedAt = item.UpdatedAt.UTC()
		if !inHistoryWindow(item.UpdatedAt, window) {
			continue
		}
		filtered = append(filtered, *item)
	}
	sort.Slice(filtered, func(left, right int) bool {
		if !filtered[left].UpdatedAt.Equal(filtered[right].UpdatedAt) {
			return filtered[left].UpdatedAt.After(filtered[right].UpdatedAt)
		}
		if filtered[left].Number != filtered[right].Number {
			return filtered[left].Number < filtered[right].Number
		}
		if filtered[left].Title != filtered[right].Title {
			return filtered[left].Title < filtered[right].Title
		}
		return filtered[left].URL < filtered[right].URL
	})
	return filtered, nil
}

func validateIssue(item issueSummary, index int) error {
	if item.Number < 1 {
		return fmt.Errorf("parse issues: entry %d has invalid number %d", index, item.Number)
	}
	if item.UpdatedAt.IsZero() {
		return fmt.Errorf("parse issues: issue #%d has missing updatedAt", item.Number)
	}
	return nil
}

func renderHistory(snapshot historySnapshot, limit int) (string, error) {
	var report bytes.Buffer
	if err := writeLine(&report, "# Repository history from %s through %s (inclusive)",
		formatHistoryTime(snapshot.window.since), formatHistoryTime(snapshot.window.until)); err != nil {
		return "", err
	}
	if err := renderGitHistory(&report, snapshot.commits, limit); err != nil {
		return "", fmt.Errorf("render Git history: %w", err)
	}
	if err := renderDocumentationHistory(&report, snapshot.documentation); err != nil {
		return "", fmt.Errorf("render documentation history: %w", err)
	}
	if err := renderPRHistory(&report, snapshot.pullRequests, limit); err != nil {
		return "", fmt.Errorf("render pull request history: %w", err)
	}
	if err := renderIssueHistory(&report, snapshot.issues, limit); err != nil {
		return "", fmt.Errorf("render issue history: %w", err)
	}
	return report.String(), nil
}

func renderGitHistory(w io.Writer, commits []string, limit int) error {
	if err := writeLine(w, "\n## First-parent commits"); err != nil {
		return err
	}
	count := minHistoryEntries(len(commits), limit)
	if count == 0 {
		if err := writeLine(w, "- None"); err != nil {
			return err
		}
		return renderOmitted(w, len(commits)-count, "commit", limit)
	}
	for _, commit := range commits[:count] {
		if err := writeLine(w, "%s", commit); err != nil {
			return err
		}
	}
	return renderOmitted(w, len(commits)-count, "commit", limit)
}

func renderPRHistory(w io.Writer, items []pullRequestSummary, limit int) error {
	if err := writeLine(w, "\n## Merged pull requests"); err != nil {
		return err
	}
	count := minHistoryEntries(len(items), limit)
	if count == 0 {
		if err := writeLine(w, "- None"); err != nil {
			return err
		}
		return renderLeadTime(w, items)
	}
	for _, item := range items[:count] {
		if err := writeLine(w, "- #%d %s (created %s; merged %s; lead time %s)", item.Number,
			item.Title, formatHistoryTime(item.CreatedAt), formatHistoryTime(item.MergedAt),
			item.MergedAt.Sub(item.CreatedAt)); err != nil {
			return err
		}
	}
	if err := renderOmitted(w, len(items)-count, "merged pull request", limit); err != nil {
		return err
	}
	return renderLeadTime(w, items)
}

func renderLeadTime(w io.Writer, items []pullRequestSummary) error {
	median, ok := medianLeadTime(items)
	if !ok {
		return writeLine(w, "Lead time (PR open to merge): count=0 median=n/a")
	}
	return writeLine(w, "Lead time (PR open to merge): count=%d median=%s; even counts use the arithmetic mean of the two middle durations, truncated to nanoseconds.",
		len(items), median)
}

func medianLeadTime(items []pullRequestSummary) (time.Duration, bool) {
	if len(items) == 0 {
		return 0, false
	}
	durations := make([]time.Duration, 0, len(items))
	for _, item := range items {
		durations = append(durations, item.MergedAt.Sub(item.CreatedAt))
	}
	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
	middle := len(durations) / 2
	if len(durations)%2 == 1 {
		return durations[middle], true
	}
	return meanDuration(durations[middle-1], durations[middle]), true
}

func meanDuration(left, right time.Duration) time.Duration {
	return left/2 + right/2 + (left%2+right%2)/2
}

func renderIssueHistory(w io.Writer, items []issueSummary, limit int) error {
	if err := writeLine(w, "\n## Updated issues"); err != nil {
		return err
	}
	count := minHistoryEntries(len(items), limit)
	if count == 0 {
		if err := writeLine(w, "- None"); err != nil {
			return err
		}
		return renderOmitted(w, len(items)-count, "issue", limit)
	}
	for _, item := range items[:count] {
		if err := writeLine(w, "- #%d [%s] %s (%s)", item.Number, strings.ToLower(item.State), item.Title,
			item.UpdatedAt.Format(time.DateOnly)); err != nil {
			return err
		}
	}
	return renderOmitted(w, len(items)-count, "issue", limit)
}

func renderOmitted(w io.Writer, omitted int, kind string, limit int) error {
	if omitted == 0 {
		return nil
	}
	return writeLine(w, "- Omitted: %d %s(s) beyond --limit %d", omitted, kind, limit)
}

func renderDocumentationHistory(w io.Writer, totals []documentationChurn) error {
	if err := writeLine(w, "\n## Documentation churn"); err != nil {
		return err
	}
	if len(totals) == 0 {
		return writeLine(w, "- None")
	}
	for _, total := range totals {
		if err := writeLine(w, "- %s: +%d -%d", strconv.Quote(total.path), total.additions, total.deletions); err != nil {
			return err
		}
	}
	return nil
}

func minHistoryEntries(length, limit int) int {
	if length < limit {
		return length
	}
	return limit
}

func inHistoryWindow(value time.Time, window historyWindow) bool {
	return !value.Before(window.since) && !value.After(window.until)
}

func formatHistoryTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func (a app) writeGitHistory(root string, since time.Time, limit int) error {
	if err := writeLine(a.stdout, "\n## First-parent commits"); err != nil {
		return err
	}
	output, err := a.command(root, "git", "log", "--first-parent", "-n", strconv.Itoa(limit),
		"--since="+since.Format(time.RFC3339), "--date=short",
		"--pretty=format:- %h %ad %s%n%w(74,2,2)%b%w(0,0,0)")
	if err != nil {
		return fmt.Errorf("read git history: %w", err)
	}
	if output == "" {
		return writeLine(a.stdout, "- None")
	}
	return writeLine(a.stdout, "%s", output)
}

func parseSince(text string, now time.Time) (time.Time, error) {
	if strings.HasSuffix(text, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(text, "d"))
		if err != nil || days < 1 {
			return time.Time{}, fmt.Errorf("invalid day window %q", text)
		}
		return now.UTC().AddDate(0, 0, -days), nil
	}
	return parseHistoryTime(text, "--since")
}

func parseUntil(text string) (time.Time, error) {
	return parseHistoryTime(text, "--until")
}

func parseHistoryTime(text, flagName string) (time.Time, error) {
	if value, err := time.Parse(time.DateOnly, text); err == nil {
		return value.UTC(), nil
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s %q", flagName, text)
	}
	return value.UTC(), nil
}
