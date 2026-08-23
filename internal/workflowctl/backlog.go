package workflowctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

type backlogHealthReport struct {
	Counts   backlogHealthCounts   `json:"counts"`
	Floors   backlogHealthFloors   `json:"floors"`
	Deficits backlogHealthDeficits `json:"deficits"`
	Healthy  bool                  `json:"healthy"`
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
	counts, err := a.readyCounts(root, list)
	if err != nil {
		return err
	}
	report := newBacklogHealthReport(counts)
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
	floors := backlogHealthFloors{Ready: 10, XS: 2, S: 3, M: 2}
	deficits := backlogHealthDeficits{
		Ready: backlogHealthDeficit(floors.Ready, counts.Ready),
		XS:    backlogHealthDeficit(floors.XS, counts.XS),
		S:     backlogHealthDeficit(floors.S, counts.S),
		M:     backlogHealthDeficit(floors.M, counts.M),
	}
	return backlogHealthReport{
		Counts:   counts,
		Floors:   floors,
		Deficits: deficits,
		Healthy:  deficits.Ready == 0 && deficits.XS == 0 && deficits.S == 0 && deficits.M == 0,
	}
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
	return stateError("ready-work buffer is below target: need %v", deficits)
}
