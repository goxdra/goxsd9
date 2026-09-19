package workflowctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type gitDocumentStatus struct {
	path   string
	status string
}

type gitLineStat struct {
	path      string
	additions int
	deletions int
	binary    bool
}

type documentationChange struct {
	path       string
	status     string
	additions  int
	deletions  int
	stats      documentStats
	rule       documentRule
	registered bool
}

type documentationRange struct {
	base      string
	head      string
	mergeBase string
}

func (a app) auditDocs(root, baseRef string) error {
	return a.auditDocsWithFormat(root, baseRef, "text")
}

func (a app) auditDocsWithFormat(root, baseRef, format string) error {
	if err := validateDocumentRegistry(); err != nil {
		return err
	}
	rangeValue, err := a.documentationAuditRange(root, baseRef)
	if err != nil {
		return err
	}
	changes, fixtures, triggers, err := a.documentationChanges(root, rangeValue.mergeBase, rangeValue.head)
	if err != nil {
		return err
	}
	if format == "json" {
		return a.writeDocumentationAuditJSON(documentationAuditReportFrom(rangeValue, changes, fixtures, triggers))
	}
	return a.writeDocumentationAudit(rangeValue, changes, fixtures, triggers)
}

func (a app) documentationAuditReportForCommits(root, base, head string) (documentationAuditReport, error) {
	clean, err := a.command(root, "git", "status", "--porcelain")
	if err != nil {
		return documentationAuditReport{}, fmt.Errorf("read documentation audit worktree status: %w", err)
	}
	if clean != "" {
		return documentationAuditReport{}, stateError("documentation audit requires a clean worktree")
	}
	if strings.TrimSpace(base) == "" || strings.TrimSpace(head) == "" {
		return documentationAuditReport{}, errors.New("documentation audit commits must not be empty")
	}
	mergeBase, err := a.command(root, "git", "merge-base", base, head)
	if err != nil || strings.TrimSpace(mergeBase) == "" {
		if err == nil {
			err = errors.New("no merge base")
		}
		return documentationAuditReport{}, fmt.Errorf("find documentation audit merge base: %w", err)
	}
	changes, fixtures, triggers, err := a.documentationChanges(root, strings.TrimSpace(mergeBase), head)
	if err != nil {
		return documentationAuditReport{}, err
	}
	return documentationAuditReportFrom(documentationRange{base: base, head: head, mergeBase: strings.TrimSpace(mergeBase)}, changes, fixtures, triggers), nil
}

func (a app) documentationAuditRange(root, baseRef string) (documentationRange, error) {
	clean, err := a.command(root, "git", "status", "--porcelain")
	if err != nil {
		return documentationRange{}, fmt.Errorf("read worktree status: %w", err)
	}
	if clean != "" {
		return documentationRange{}, stateError("documentation audit requires a clean worktree")
	}
	base, err := a.resolveCommit(root, baseRef)
	if err != nil {
		return documentationRange{}, fmt.Errorf("resolve documentation base %q: %w", baseRef, err)
	}
	head, err := a.resolveCommit(root, "HEAD")
	if err != nil {
		return documentationRange{}, fmt.Errorf("resolve documentation head: %w", err)
	}
	if base == head {
		return documentationRange{}, stateError("documentation base %q resolves to HEAD", baseRef)
	}
	mergeBase, err := a.command(root, "git", "merge-base", base, head)
	if err != nil || mergeBase == "" {
		if err == nil {
			err = errors.New("no merge base")
		}
		return documentationRange{}, stateError("documentation base %q is unrelated to HEAD: %v", baseRef, err)
	}
	return documentationRange{base: base, head: head, mergeBase: mergeBase}, nil
}

func (a app) writeDocumentationAudit(rangeValue documentationRange, changes []documentationChange, fixtures, triggers []string) error {
	if err := writeLine(a.stdout, "Documentation audit"); err != nil {
		return err
	}
	if err := writeLine(a.stdout, "Head: %s", rangeValue.head); err != nil {
		return err
	}
	if err := writeLine(a.stdout, "Base: %s", rangeValue.base); err != nil {
		return err
	}
	if err := writeLine(a.stdout, "Merge base: %s", rangeValue.mergeBase); err != nil {
		return err
	}
	if err := a.writeDocumentationChanges(changes); err != nil {
		return err
	}
	if err := a.writeNewEvaluationFixtures(fixtures); err != nil {
		return err
	}
	if err := a.writeCurrentStateReviewTriggers(triggers); err != nil {
		return err
	}
	review := "not required"
	if len(changes) > 0 || len(triggers) > 0 {
		review = "required"
	}
	return writeLine(a.stdout, "\nCurator review: %s", review)
}

func (a app) writeDocumentationAuditJSON(report documentationAuditReport) error {
	encoder := json.NewEncoder(a.stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode documentation audit: %w", err)
	}
	return nil
}

func (a app) writeDocumentationChanges(changes []documentationChange) error {
	if err := writeLine(a.stdout, "\nManaged document changes:"); err != nil {
		return err
	}
	if len(changes) == 0 {
		if err := writeLine(a.stdout, "- None"); err != nil {
			return err
		}
	}
	for _, change := range changes {
		if err := a.writeDocumentationChange(change); err != nil {
			return err
		}
	}
	return nil
}

func (a app) writeNewEvaluationFixtures(fixtures []string) error {
	if err := writeLine(a.stdout, "\nNew agent evaluation fixtures:"); err != nil {
		return err
	}
	if len(fixtures) == 0 {
		if err := writeLine(a.stdout, "- None"); err != nil {
			return err
		}
	}
	for _, fixture := range fixtures {
		if err := writeLine(a.stdout, "- %s", strconv.Quote(fixture)); err != nil {
			return err
		}
	}
	return nil
}

func (a app) writeCurrentStateReviewTriggers(triggers []string) error {
	if err := writeLine(a.stdout, "\nCurrent-state review triggers:"); err != nil {
		return err
	}
	if len(triggers) == 0 {
		return writeLine(a.stdout, "- None")
	}
	for _, trigger := range triggers {
		if err := writeLine(a.stdout, "- %s", strconv.Quote(trigger)); err != nil {
			return err
		}
	}
	return nil
}

func (a app) resolveCommit(root, ref string) (string, error) {
	commit, err := a.command(root, "git", "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	if commit == "" || strings.Contains(commit, "\n") {
		return "", errors.New("git returned an invalid commit ID")
	}
	return commit, nil
}

func (a app) documentationChanges(root, base, head string) ([]documentationChange, []string, []string, error) {
	statusOutput, err := a.gitRaw(root, "diff", "--name-status", "-z", "--no-renames", base, head, "--")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read documentation paths: %w", err)
	}
	statuses, err := parseGitDocumentStatuses(statusOutput)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse documentation paths: %w", err)
	}
	statOutput, err := a.gitRaw(root, "diff", "--numstat", "-z", "--no-renames", base, head, "--")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read documentation line changes: %w", err)
	}
	lineStats, err := parseGitLineStats(statOutput)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse documentation line changes: %w", err)
	}
	changes := make([]documentationChange, 0, len(statuses))
	fixtures := make([]string, 0)
	triggers := make([]string, 0)
	for _, status := range statuses {
		if isCurrentStateReviewTriggerPath(status.path) {
			triggers = append(triggers, status.path)
		}
		change, fixture, managed, classifyErr := classifyDocumentationChange(root, status, lineStats)
		if classifyErr != nil {
			return nil, nil, nil, classifyErr
		}
		if fixture != "" {
			fixtures = append(fixtures, fixture)
		}
		if managed {
			changes = append(changes, change)
		}
	}
	sort.Slice(changes, func(left, right int) bool { return changes[left].path < changes[right].path })
	sort.Strings(fixtures)
	sort.Strings(triggers)
	return changes, fixtures, triggers, nil
}

func isCurrentStateReviewTriggerPath(pathValue string) bool {
	if !strings.HasSuffix(pathValue, ".go") || strings.HasSuffix(pathValue, "_test.go") {
		return false
	}
	if strings.HasPrefix(pathValue, "internal/workflowctl/") || strings.HasPrefix(pathValue, "cmd/workflowctl/") {
		return false
	}
	for _, segment := range strings.Split(pathValue, "/") {
		if segment == "testdata" || segment == "evals" {
			return false
		}
	}
	return true
}

func classifyDocumentationChange(root string, status gitDocumentStatus,
	lineStats []gitLineStat,
) (documentationChange, string, bool, error) {
	if !isMarkdownPath(status.path) {
		return documentationChange{}, "", false, nil
	}
	if strings.HasPrefix(status.path, "evals/agent/") {
		if status.status == "A" {
			return documentationChange{}, status.path, false, nil
		}
		return documentationChange{}, "", false, nil
	}
	if !isDurableMarkdown(status.path) {
		return documentationChange{}, "", false, nil
	}
	lineStat, ok := gitLineStatFor(lineStats, status.path)
	if !ok {
		return documentationChange{}, "", false,
			fmt.Errorf("documentation path %s has no line statistics", strconv.Quote(status.path))
	}
	if lineStat.binary {
		return documentationChange{}, "", false,
			fmt.Errorf("managed Markdown %s is binary", strconv.Quote(status.path))
	}
	rule, registered := documentRuleFor(status.path)
	if !registered && status.status != "D" {
		return documentationChange{}, "", false,
			fmt.Errorf("new or modified durable Markdown %s is not registered", strconv.Quote(status.path))
	}
	change := documentationChange{
		path: status.path, status: status.status, additions: lineStat.additions, deletions: lineStat.deletions,
		rule: rule, registered: registered,
	}
	if status.status == "D" {
		return change, "", true, nil
	}
	stats, err := readDocumentStats(filepath.Join(root, filepath.FromSlash(status.path)))
	if err != nil {
		return documentationChange{}, "", false, err
	}
	if err := validateDocumentStats(status.path, stats, rule); err != nil {
		return documentationChange{}, "", false, err
	}
	change.stats = stats
	return change, "", true, nil
}

func (a app) writeDocumentationChange(change documentationChange) error {
	label := documentationStatusLabel(change.status)
	if !change.registered {
		return writeLine(a.stdout, "- %s [%s, unregistered]: +%d -%d",
			strconv.Quote(change.path), label, change.additions, change.deletions)
	}
	if change.status == "D" {
		return writeLine(a.stdout, "- %s [%s]: +%d -%d; charter: %s",
			strconv.Quote(change.path), label, change.additions, change.deletions, change.rule.charter)
	}
	return writeLine(a.stdout, "- %s [%s]: +%d -%d; %d/%d lines; %d/%d words; charter: %s",
		strconv.Quote(change.path), label, change.additions, change.deletions, change.stats.lines,
		change.rule.maxLines, change.stats.words, change.rule.maxWords, change.rule.charter)
}

func documentationStatusLabel(status string) string {
	switch status {
	case "A":
		return "added"
	case "D":
		return "deleted"
	case "M":
		return "modified"
	case "T":
		return "type changed"
	}
	return "unknown"
}

func parseGitDocumentStatuses(output string) ([]gitDocumentStatus, error) {
	fields, err := parseNULFields(output)
	if err != nil {
		return nil, err
	}
	if len(fields)%2 != 0 {
		return nil, errors.New("name-status output has a truncated record")
	}
	statuses := make([]gitDocumentStatus, 0, len(fields)/2)
	for index := 0; index < len(fields); index += 2 {
		status := fields[index]
		pathValue := fields[index+1]
		if status != "A" && status != "D" && status != "M" && status != "T" {
			return nil, fmt.Errorf("unsupported Git document status %q", status)
		}
		if err := validateGitPath(pathValue); err != nil {
			return nil, err
		}
		statuses = append(statuses, gitDocumentStatus{path: pathValue, status: status})
	}
	sort.Slice(statuses, func(left, right int) bool { return statuses[left].path < statuses[right].path })
	for index := 1; index < len(statuses); index++ {
		if statuses[index-1].path == statuses[index].path {
			return nil, fmt.Errorf("duplicate Git document path %s", strconv.Quote(statuses[index].path))
		}
	}
	return statuses, nil
}

func parseGitLineStats(output string) ([]gitLineStat, error) {
	records, err := parseNULFields(output)
	if err != nil {
		return nil, err
	}
	stats := make([]gitLineStat, 0, len(records))
	for _, record := range records {
		stat, parseErr := parseGitLineStat(record)
		if parseErr != nil {
			return nil, parseErr
		}
		stats = append(stats, stat)
	}
	sort.Slice(stats, func(left, right int) bool { return stats[left].path < stats[right].path })
	for index := 1; index < len(stats); index++ {
		if stats[index-1].path == stats[index].path {
			return nil, fmt.Errorf("duplicate numstat path %s", strconv.Quote(stats[index].path))
		}
	}
	return stats, nil
}

func parseGitLineStat(record string) (gitLineStat, error) {
	fields := strings.SplitN(record, "\t", 3)
	if len(fields) != 3 {
		return gitLineStat{}, errors.New("numstat output has a malformed record")
	}
	if pathErr := validateGitPath(fields[2]); pathErr != nil {
		return gitLineStat{}, pathErr
	}
	stat := gitLineStat{path: fields[2]}
	if fields[0] == "-" || fields[1] == "-" {
		if fields[0] != "-" || fields[1] != "-" {
			return gitLineStat{}, errors.New("numstat output has inconsistent binary markers")
		}
		stat.binary = true
		return stat, nil
	}
	additions, err := strconv.Atoi(fields[0])
	if err != nil || additions < 0 {
		return gitLineStat{}, fmt.Errorf("invalid numstat addition count %q", fields[0])
	}
	deletions, err := strconv.Atoi(fields[1])
	if err != nil || deletions < 0 {
		return gitLineStat{}, fmt.Errorf("invalid numstat deletion count %q", fields[1])
	}
	stat.additions = additions
	stat.deletions = deletions
	return stat, nil
}

func gitLineStatFor(stats []gitLineStat, target string) (gitLineStat, bool) {
	index := sort.Search(len(stats), func(index int) bool { return stats[index].path >= target })
	if index == len(stats) || stats[index].path != target {
		return gitLineStat{}, false
	}
	return stats[index], true
}

func parseNULFields(output string) ([]string, error) {
	if output == "" {
		return nil, nil
	}
	if !strings.HasSuffix(output, "\x00") {
		return nil, errors.New("NUL-delimited output has no final delimiter")
	}
	fields := strings.Split(output, "\x00")
	if fields[len(fields)-1] != "" {
		return nil, errors.New("NUL-delimited output has invalid framing")
	}
	return fields[:len(fields)-1], nil
}

func validateGitPath(value string) error {
	if value == "" || path.IsAbs(value) || path.Clean(value) != value || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("invalid repository path %s", strconv.Quote(value))
	}
	return nil
}
