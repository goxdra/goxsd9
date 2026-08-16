package workflowctl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type documentationChurn struct {
	path      string
	additions int
	deletions int
}

func (a app) writeDocumentationHistory(root string, since time.Time) error {
	totals, err := a.readDocumentationChurn(root, since)
	if err != nil {
		return err
	}
	return renderDocumentationHistory(a.stdout, totals)
}

func (a app) readDocumentationChurnWindow(root string, since, until time.Time) ([]documentationChurn, error) {
	output, err := a.command(root, "git", "rev-list", "--first-parent", "--since="+formatHistoryTime(since),
		"--until="+formatHistoryTime(until), "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read commits for documentation churn: %w", err)
	}
	if output == "" {
		return nil, nil
	}
	totals := make([]documentationChurn, 0)
	for index, commit := range strings.Split(output, "\n") {
		if commit == "" {
			return nil, fmt.Errorf("parse documentation commit list: empty commit at index %d", index)
		}
		if err := a.addCommitDocumentationChurn(root, commit, &totals); err != nil {
			return nil, err
		}
	}
	sort.Slice(totals, func(left, right int) bool { return totals[left].path < totals[right].path })
	return totals, nil
}

func (a app) readDocumentationChurn(root string, since time.Time) ([]documentationChurn, error) {
	output, err := a.command(root, "git", "rev-list", "--first-parent", "--since="+since.Format(time.RFC3339), "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read commits for documentation churn: %w", err)
	}
	if output == "" {
		return nil, nil
	}
	totals := make([]documentationChurn, 0)
	for _, commit := range strings.Split(output, "\n") {
		if err := a.addCommitDocumentationChurn(root, commit, &totals); err != nil {
			return nil, err
		}
	}
	sort.Slice(totals, func(left, right int) bool { return totals[left].path < totals[right].path })
	return totals, nil
}

func (a app) addCommitDocumentationChurn(root, commit string, totals *[]documentationChurn) error {
	statsOutput, err := a.gitRaw(root, "diff-tree", "--root", "--first-parent", "--no-commit-id",
		"--numstat", "-r", "-z", "--no-renames", commit, "--")
	if err != nil {
		return fmt.Errorf("read documentation churn for %s: %w", commit, err)
	}
	stats, err := parseGitLineStats(statsOutput)
	if err != nil {
		return fmt.Errorf("parse documentation churn for %s: %w", commit, err)
	}
	for _, stat := range stats {
		if !isDurableMarkdown(stat.path) {
			continue
		}
		if stat.binary {
			return fmt.Errorf("durable Markdown %s is binary in commit %s", strconv.Quote(stat.path), commit)
		}
		addDocumentationChurn(totals, stat)
	}
	return nil
}

func addDocumentationChurn(totals *[]documentationChurn, stat gitLineStat) {
	for index := range *totals {
		if (*totals)[index].path != stat.path {
			continue
		}
		(*totals)[index].additions += stat.additions
		(*totals)[index].deletions += stat.deletions
		return
	}
	*totals = append(*totals, documentationChurn{
		path: stat.path, additions: stat.additions, deletions: stat.deletions,
	})
}
