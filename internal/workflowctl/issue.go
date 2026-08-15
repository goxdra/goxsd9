package workflowctl

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type intValues []int

func (values *intValues) String() string {
	parts := make([]string, 0, len(*values))
	for _, value := range *values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func (values *intValues) Set(text string) error {
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 {
		return fmt.Errorf("invalid issue number %q", text)
	}
	*values = append(*values, value)
	return nil
}

func (a app) runIssue(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return usageError("usage: workflowctl issue create [flags]")
	}
	return a.createIssue(args[1:])
}

func (a app) createIssue(args []string) error {
	flags := flag.NewFlagSet("issue create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	title := flags.String("title", "", "issue title")
	bodyFile := flags.String("body-file", "", "Markdown body file")
	area := flags.String("area", "", "area label suffix")
	typeName := flags.String("type", "", "type label suffix")
	priority := flags.String("priority", "P2", "Project priority")
	effort := flags.String("effort", "S", "Project effort")
	phase := flags.String("phase", "Bootstrap", "Project phase")
	status := flags.String("status", "Backlog", "Project status")
	var blockedBy intValues
	flags.Var(&blockedBy, "blocked-by", "blocking issue number; repeatable")
	if err := flags.Parse(args); err != nil {
		return usageError("issue create: %v", err)
	}
	if flags.NArg() != 0 {
		return usageError("issue create takes flags only")
	}
	if err := validateIssueInput(*title, *bodyFile, *area, *typeName, *priority, *effort, *phase, *status); err != nil {
		return usageError("issue create: %v", err)
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	url, err := a.createGitHubIssue(root, *title, *bodyFile, *area, *typeName)
	if err != nil {
		return err
	}
	number, err := issueNumberFromURL(url)
	if err != nil {
		return fmt.Errorf("issue created at %s but its number was not understood: %w", url, err)
	}
	if err := a.configureNewIssue(root, url, number, *priority, *effort, *phase, *status, blockedBy); err != nil {
		return fmt.Errorf("issue created at %s but configuration failed: %w", url, err)
	}
	return writeLine(a.stdout, "%s", url)
}

func validateIssueInput(title, bodyFile, area, typeName, priority, effort, phase, status string) error {
	if strings.TrimSpace(title) == "" || bodyFile == "" || area == "" || typeName == "" {
		return errors.New("--title, --body-file, --area, and --type are required")
	}
	if err := requireRegularFile(bodyFile); err != nil {
		return err
	}
	// #nosec G304 -- bodyFile is an explicit operator-supplied input.
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		return fmt.Errorf("read issue body: %w", err)
	}
	if !strings.Contains(string(body), "## Acceptance") {
		return errors.New("issue body must contain an Acceptance section")
	}
	if priorityRank(priority) > 4 {
		return fmt.Errorf("invalid priority %q", priority)
	}
	if effortRank(effort) > 2 && status == "Ready" {
		return errors.New("ready issues must be XS, S, or M")
	}
	if phase == "" || (status != "Backlog" && status != "Ready") {
		return errors.New("invalid phase or status")
	}
	return nil
}

func (a app) createGitHubIssue(root, title, bodyFile, area, typeName string) (string, error) {
	output, err := a.command(root, "gh", "issue", "create", "--repo", repositoryKey, "--title", title,
		"--body-file", bodyFile, "--label", "area/"+area, "--label", "type/"+typeName)
	if err != nil {
		return "", fmt.Errorf("create issue: %w", err)
	}
	return firstLine(output), nil
}

func (a app) configureNewIssue(root, url string, number int, priority, effort, phase, status string, blockedBy []int) error {
	output, err := a.command(root, "gh", "project", "item-add", strconv.Itoa(projectNumber), "--owner", owner,
		"--url", url, "--format", "json")
	if err != nil {
		return fmt.Errorf("add issue to Project: %w", err)
	}
	var item projectItem
	if decodeErr := json.Unmarshal([]byte(output), &item); decodeErr != nil {
		return fmt.Errorf("decode added Project item: %w", decodeErr)
	}
	fields, err := a.projectFields(root)
	if err != nil {
		return err
	}
	values := []struct{ field, option string }{
		{field: "Status", option: status},
		{field: "Priority", option: priority},
		{field: "Effort", option: effort},
		{field: "Phase", option: phase},
	}
	for _, value := range values {
		if err := a.setProjectFieldFromList(root, fields, item.ID, value.field, value.option); err != nil {
			return err
		}
	}
	for _, blocker := range blockedBy {
		if _, err := a.command(root, "gh", "issue", "edit", strconv.Itoa(number), "--repo", repositoryKey,
			"--add-blocked-by", strconv.Itoa(blocker)); err != nil {
			return fmt.Errorf("add dependency on #%d: %w", blocker, err)
		}
	}
	return nil
}

func issueNumberFromURL(url string) (int, error) {
	part := url[strings.LastIndex(url, "/")+1:]
	number, err := strconv.Atoi(part)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("invalid issue URL %q", url)
	}
	return number, nil
}

func (a app) runHandoff(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl handoff ISSUE --body-file FILE")
	}
	number, err := positiveNumber(args[0])
	if err != nil {
		return usageError("handoff: %v", err)
	}
	flags := flag.NewFlagSet("handoff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	bodyFile := flags.String("body-file", "", "Markdown body file")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("handoff: %v", parseErr)
	}
	if flags.NArg() != 0 || *bodyFile == "" {
		return usageError("usage: workflowctl handoff ISSUE --body-file FILE")
	}
	if fileErr := requireRegularFile(*bodyFile); fileErr != nil {
		return fileErr
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	output, err := a.command(root, "gh", "issue", "comment", strconv.Itoa(number), "--repo", repositoryKey,
		"--body-file", *bodyFile)
	if err != nil {
		return fmt.Errorf("comment on issue #%d: %w", number, err)
	}
	return writeLine(a.stdout, "%s", output)
}
