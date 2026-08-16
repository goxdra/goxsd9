package workflowctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

type pullRequestSummary struct {
	MergedAt time.Time `json:"mergedAt"`
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
}

type issueSummary struct {
	Number    int       `json:"number"`
	State     string    `json:"state"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
}

func (a app) runHistory(args []string) error {
	flags := flag.NewFlagSet("history", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sinceText := flags.String("since", "7d", "history window")
	limit := flags.Int("limit", 30, "maximum entries per section")
	if err := flags.Parse(args); err != nil {
		return usageError("history: %v", err)
	}
	if flags.NArg() != 0 || *limit < 1 || *limit > 100 {
		return usageError("history accepts --since and --limit 1..100")
	}

	root, err := a.root()
	if err != nil {
		return err
	}
	since, err := parseSince(*sinceText, time.Now())
	if err != nil {
		return usageError("history: %v", err)
	}
	if err := writeLine(a.stdout, "# Repository history since %s", since.Format(time.DateOnly)); err != nil {
		return err
	}
	if err := a.writeGitHistory(root, since, *limit); err != nil {
		return err
	}
	if err := a.writeDocumentationHistory(root, since); err != nil {
		return err
	}
	if err := a.writePRHistory(root, since, *limit); err != nil {
		return err
	}
	return a.writeIssueHistory(root, since, *limit)
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

func (a app) writePRHistory(root string, since time.Time, limit int) error {
	if err := writeLine(a.stdout, "\n## Merged pull requests"); err != nil {
		return err
	}
	output, err := a.command(root, "gh", "pr", "list", "--repo", repositoryKey, "--state", "merged",
		"--limit", strconv.Itoa(limit), "--json", "number,title,mergedAt,url")
	if err != nil {
		return fmt.Errorf("read pull requests: %w", err)
	}
	var items []pullRequestSummary
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return fmt.Errorf("decode pull requests: %w", err)
	}
	written := 0
	for _, item := range items {
		if item.MergedAt.Before(since) {
			continue
		}
		written++
		if err := writeLine(a.stdout, "- #%d %s (%s)", item.Number, item.Title, item.MergedAt.Format(time.DateOnly)); err != nil {
			return err
		}
	}
	if written == 0 {
		return writeLine(a.stdout, "- None")
	}
	return nil
}

func (a app) writeIssueHistory(root string, since time.Time, limit int) error {
	if err := writeLine(a.stdout, "\n## Updated issues"); err != nil {
		return err
	}
	output, err := a.command(root, "gh", "issue", "list", "--repo", repositoryKey, "--state", "all",
		"--limit", strconv.Itoa(limit), "--json", "number,title,state,updatedAt,url")
	if err != nil {
		return fmt.Errorf("read issues: %w", err)
	}
	var items []issueSummary
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return fmt.Errorf("decode issues: %w", err)
	}
	sort.Slice(items, func(left, right int) bool {
		return items[left].UpdatedAt.After(items[right].UpdatedAt)
	})
	written := 0
	for _, item := range items {
		if item.UpdatedAt.Before(since) {
			continue
		}
		written++
		if err := writeLine(a.stdout, "- #%d [%s] %s (%s)", item.Number, strings.ToLower(item.State), item.Title,
			item.UpdatedAt.Format(time.DateOnly)); err != nil {
			return err
		}
	}
	if written == 0 {
		return writeLine(a.stdout, "- None")
	}
	return nil
}

func parseSince(text string, now time.Time) (time.Time, error) {
	if strings.HasSuffix(text, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(text, "d"))
		if err != nil || days < 1 {
			return time.Time{}, fmt.Errorf("invalid day window %q", text)
		}
		return now.AddDate(0, 0, -days), nil
	}
	if value, err := time.Parse(time.DateOnly, text); err == nil {
		return value, nil
	}
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since %q", text)
	}
	return value, nil
}
