package workflowctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type backlogHealthCounts struct {
	Ready int `json:"ready"`
	XS    int `json:"xs"`
	S     int `json:"s"`
	M     int `json:"m"`
}

type backlogHealthFloors struct {
	Ready int `json:"ready"`
	XS    int `json:"xs"`
	S     int `json:"s"`
	M     int `json:"m"`
}

type backlogHealthDeficits struct {
	Ready int `json:"ready"`
	XS    int `json:"xs"`
	S     int `json:"s"`
	M     int `json:"m"`
}

type backlogHealthFinding struct {
	Number  int      `json:"number"`
	Title   string   `json:"title"`
	Missing []string `json:"missing"`
}

type backlogHealthReport struct {
	Counts     backlogHealthCounts    `json:"counts"`
	Floors     backlogHealthFloors    `json:"floors"`
	Deficits   backlogHealthDeficits  `json:"deficits"`
	Incomplete []backlogHealthFinding `json:"incomplete,omitempty"`
	Healthy    bool                   `json:"healthy"`
}

func (a app) runBacklog(args []string) error {
	format, err := parseBacklogHealthArgs(args)
	if err != nil {
		return err
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	list, err := a.projectItems(root)
	if err != nil {
		return err
	}
	incomplete, err := incompleteProjectItems(list)
	if err != nil {
		return err
	}
	counts, err := a.readyCounts(root, list)
	if err != nil {
		return err
	}
	report := newBacklogHealthReportWithFindings(counts, incomplete)
	return a.writeBacklogHealth(report, format)
}

func parseBacklogHealthArgs(args []string) (string, error) {
	const usage = "usage: workflowctl backlog health [--format text|json]"
	if len(args) == 0 || args[0] != "health" {
		return "", usageError(usage)
	}

	flags := flag.NewFlagSet("backlog health", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args[1:]); err != nil {
		return "", usageError("backlog health: %v", err)
	}
	if flags.NArg() != 0 {
		return "", usageError(usage)
	}
	if *format != "text" && *format != "json" {
		return "", usageError("backlog health: unsupported output format %q", *format)
	}
	return *format, nil
}

func (a app) writeBacklogHealth(report backlogHealthReport, format string) error {
	if format == "json" {
		if err := writeBacklogHealthJSON(a.stdout, report); err != nil {
			return err
		}
		return report.healthError()
	}

	if err := report.writeText(a.stdout); err != nil {
		return err
	}
	return report.healthError()
}

func (report backlogHealthReport) healthError() error {
	if report.Healthy {
		return nil
	}
	return report.stateError()
}

func (a app) readyCounts(root string, list projectList) (backlogHealthCounts, error) {
	var counts backlogHealthCounts
	for _, item := range list.Items {
		if item.Status != "Ready" || item.Content.Type != "Issue" || item.Content.Repository != repositoryKey {
			continue
		}
		relations, err := a.issueRelations(root, item.Content.Number)
		if err != nil {
			return backlogHealthCounts{}, err
		}
		if hasOpenIssue(relations.BlockedBy.Nodes) {
			continue
		}
		counts.Ready++
		switch item.Effort {
		case "XS":
			counts.XS++
		case "S":
			counts.S++
		case "M":
			counts.M++
		}
	}
	return counts, nil
}

func newBacklogHealthReport(counts backlogHealthCounts) backlogHealthReport {
	return newBacklogHealthReportWithFindings(counts, nil)
}

func newBacklogHealthReportWithFindings(counts backlogHealthCounts, incomplete []backlogHealthFinding) backlogHealthReport {
	floors := backlogHealthFloors{Ready: 10, XS: 2, S: 3, M: 2}
	deficits := backlogHealthDeficits{
		Ready: backlogHealthDeficit(floors.Ready, counts.Ready),
		XS:    backlogHealthDeficit(floors.XS, counts.XS),
		S:     backlogHealthDeficit(floors.S, counts.S),
		M:     backlogHealthDeficit(floors.M, counts.M),
	}
	findings := make([]backlogHealthFinding, len(incomplete))
	copy(findings, incomplete)
	return backlogHealthReport{
		Counts:     counts,
		Floors:     floors,
		Deficits:   deficits,
		Incomplete: findings,
		Healthy:    len(findings) == 0 && deficits.Ready == 0 && deficits.XS == 0 && deficits.S == 0 && deficits.M == 0,
	}
}

func incompleteProjectItems(list projectList) ([]backlogHealthFinding, error) {
	findings := make([]backlogHealthFinding, 0)
	seen := make(map[int]bool, len(list.Items))
	duplicates := make([]int, 0)
	duplicateSeen := make(map[int]bool)
	for _, item := range list.Items {
		if item.Content.Repository != repositoryKey || item.Content.Type != "Issue" {
			continue
		}
		finding, ok := collectIncompleteProjectFinding(item, seen, duplicateSeen, &duplicates)
		if !ok {
			continue
		}
		findings = append(findings, finding)
	}
	if len(duplicates) != 0 {
		sort.Ints(duplicates)
		return nil, fmt.Errorf("project contains duplicate canonical Issue item for issue #%d", duplicates[0])
	}
	sort.Slice(findings, func(left, right int) bool {
		if findings[left].Number != findings[right].Number {
			return findings[left].Number < findings[right].Number
		}
		leftMissing := strings.Join(findings[left].Missing, "\x00")
		rightMissing := strings.Join(findings[right].Missing, "\x00")
		if leftMissing != rightMissing {
			return leftMissing < rightMissing
		}
		return findings[left].Title < findings[right].Title
	})
	return findings, nil
}

func collectIncompleteProjectFinding(item projectItem, seen, duplicateSeen map[int]bool, duplicates *[]int) (backlogHealthFinding, bool) {
	number := item.Content.Number
	if seen[number] {
		if duplicateSeen[number] {
			return backlogHealthFinding{}, false
		}
		*duplicates = append(*duplicates, number)
		duplicateSeen[number] = true
		return backlogHealthFinding{}, false
	}
	seen[number] = true
	if item.Status == "Done" {
		return backlogHealthFinding{}, false
	}
	missing := missingProjectFields(item)
	if len(missing) == 0 {
		return backlogHealthFinding{}, false
	}
	return backlogHealthFinding{Number: number, Title: item.Title, Missing: missing}, true
}

func missingProjectFields(item projectItem) []string {
	fields := []struct {
		name  string
		value string
	}{
		{name: "Status", value: item.Status},
		{name: "Priority", value: item.Priority},
		{name: "Effort", value: item.Effort},
		{name: "Phase", value: item.Phase},
	}
	missing := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.name)
		}
	}
	sort.Strings(missing)
	return missing
}

func backlogHealthDeficit(floor, count int) int {
	if count >= floor {
		return 0
	}
	return floor - count
}

func writeBacklogHealthJSON(w io.Writer, report backlogHealthReport) error {
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode backlog health: %w", err)
	}
	if err := writeLine(w, "%s", encoded); err != nil {
		return fmt.Errorf("write backlog health: %w", err)
	}
	return nil
}

func (report backlogHealthReport) writeText(w io.Writer) error {
	if err := writeLine(w, "Ready: %d (XS=%d S=%d M=%d)", report.Counts.Ready,
		report.Counts.XS, report.Counts.S, report.Counts.M); err != nil {
		return err
	}
	if err := writeLine(w, "Ready floor: %d (XS=%d S=%d M=%d)", report.Floors.Ready,
		report.Floors.XS, report.Floors.S, report.Floors.M); err != nil {
		return err
	}
	if len(report.Incomplete) != 0 {
		if err := writeLine(w, "Incomplete Project metadata:"); err != nil {
			return err
		}
		for _, finding := range report.Incomplete {
			if err := writeLine(w, "#%d: %s (missing: %s)", finding.Number, strconv.Quote(finding.Title),
				strings.Join(finding.Missing, ", ")); err != nil {
				return err
			}
		}
	}
	if !report.Healthy {
		return nil
	}
	return writeLine(w, "Ready-work buffer: healthy")
}

func (report backlogHealthReport) stateError() error {
	var deficits []string
	if report.Deficits.Ready != 0 {
		deficits = append(deficits, fmt.Sprintf("%d total", report.Deficits.Ready))
	}
	if report.Deficits.XS != 0 {
		deficits = append(deficits, fmt.Sprintf("%d XS", report.Deficits.XS))
	}
	if report.Deficits.S != 0 {
		deficits = append(deficits, fmt.Sprintf("%d S", report.Deficits.S))
	}
	if report.Deficits.M != 0 {
		deficits = append(deficits, fmt.Sprintf("%d M", report.Deficits.M))
	}
	if len(report.Incomplete) != 0 {
		if len(deficits) == 0 {
			return stateError("Project metadata is incomplete: %d item(s)", len(report.Incomplete))
		}
		return stateError("ready-work buffer is below target: need %v; Project metadata is incomplete: %d item(s)",
			deficits, len(report.Incomplete))
	}
	return stateError("ready-work buffer is below target: need %v", deficits)
}
