package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const (
	backlogReadyDeficit = 1 << iota
	backlogXSDeficit
	backlogSDeficit
	backlogMDeficit
)

type backlogFixture struct {
	list          projectList
	relations     map[int]issueRelations
	selected      []int
	blockedNumber int
}

type backlogErrorFixture struct {
	name       string
	projectOut string
	dependency bool
}

type backlogRunResult struct {
	code           int
	output         string
	err            error
	dependencyCall []int
}

func TestBacklogHealthFormatsAllDeficitCombinations(t *testing.T) {
	for mask := 0; mask < 1<<4; mask++ {
		t.Run(fmt.Sprintf("mask-%02d", mask), func(t *testing.T) {
			fixture, want := backlogHealthFixture(mask)

			textResult := runBacklogFixture(t, []string{"backlog", "health"}, fixture)
			assertBacklogResult(t, textResult, want, fixture, "text")

			jsonResult := runBacklogFixture(t, []string{"backlog", "health", "--format", "json"}, fixture)
			assertBacklogResult(t, jsonResult, want, fixture, "json")

			var jsonReport backlogHealthReport
			if err := json.Unmarshal([]byte(jsonResult.output), &jsonReport); err != nil {
				t.Fatalf("decode JSON report: %v", err)
			}
			textCounts := parseBacklogTextCounts(t, textResult.output)
			if textCounts != jsonReport.Counts || jsonReport.Healthy != want.Healthy {
				t.Fatalf("text and JSON disagree: text=%#v JSON=%#v", textCounts, jsonReport)
			}
		})
	}
}

func TestBacklogHealthRejectsInvalidArgumentsBeforeCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing health", args: []string{"backlog"}},
		{name: "unknown health command", args: []string{"backlog", "status"}},
		{name: "unknown flag", args: []string{"backlog", "health", "--unexpected"}},
		{name: "missing format value", args: []string{"backlog", "health", "--format"}},
		{name: "unsupported format", args: []string{"backlog", "health", "--format", "yaml"}},
		{name: "positional argument", args: []string{"backlog", "health", "extra"}},
		{name: "positional after format", args: []string{"backlog", "health", "--format", "json", "extra"}},
		{name: "unknown flag after format", args: []string{"backlog", "health", "--format", "json", "--unexpected"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			application := app{
				stdout: bytes.NewBuffer(nil),
				executeCommand: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
					calls++
					return "", errors.New("external command should not run")
				},
			}
			err := application.run(test.args)
			if got := backlogExitCode(err); got != 2 {
				t.Fatalf("exit code = %d, want usage code 2 (err=%v)", got, err)
			}
			if calls != 0 {
				t.Fatalf("external command calls = %d, want 0", calls)
			}
		})
	}
}

func TestBacklogHealthUsageAdvertisesFormat(t *testing.T) {
	var output bytes.Buffer
	if err := (app{stdout: &output}).usage(); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if want := "workflowctl backlog health [--format text|json]"; !strings.Contains(output.String(), want) {
		t.Fatalf("usage omits %q:\n%s", want, output.String())
	}
}

func TestBacklogHealthTransportAndDecodeErrorsDoNotRender(t *testing.T) {
	tests := []backlogErrorFixture{
		{name: "project transport"},
		{name: "project decode", projectOut: "{"},
		{name: "dependency transport", dependency: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertBacklogHealthError(t, test)
		})
	}
}

func assertBacklogHealthError(t *testing.T, test backlogErrorFixture) {
	t.Helper()
	fixture, _ := backlogHealthFixture(0)
	sentinel := errors.New(test.name)
	output, err := runBacklogHealthError(t, test, fixture, sentinel)
	if test.projectOut == "{" {
		if err == nil || !strings.Contains(err.Error(), "decode Project items") {
			t.Fatalf("error = %v, want Project decode context", err)
		}
	}
	if test.projectOut != "{" && !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want wrapped %q", err, sentinel)
	}
	if backlogExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want ordinary exit code 1", backlogExitCode(err))
	}
	if output != "" {
		t.Fatalf("output = %q, want no fabricated report", output)
	}
}

func runBacklogHealthError(t *testing.T, test backlogErrorFixture, fixture backlogFixture, sentinel error) (string, error) {
	t.Helper()
	var output bytes.Buffer
	application := app{
		ctx:            context.Background(),
		stdout:         &output,
		executeCommand: backlogErrorExecutor(t, test, fixture, sentinel),
	}
	err := application.run([]string{"backlog", "health", "--format", "json"})
	return output.String(), err
}

func backlogErrorExecutor(t *testing.T, test backlogErrorFixture, fixture backlogFixture, sentinel error) commandExecutor {
	t.Helper()
	encodedList, err := json.Marshal(fixture.list)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name == "git" {
			return "/repo", nil
		}
		if name != "gh" || len(args) == 0 {
			return "", fmt.Errorf("unexpected command %s %v", name, args)
		}
		if args[0] != "project" {
			return "", sentinel
		}
		if test.dependency {
			return string(encodedList), nil
		}
		if test.projectOut != "" {
			return test.projectOut, nil
		}
		return "", sentinel
	}
}

func TestBacklogHealthOutputErrorIsOrdinary(t *testing.T) {
	fixture, _ := backlogHealthFixture(0)
	sentinel := errors.New("output failed")
	application := app{
		ctx:            context.Background(),
		stdout:         backlogFailingWriter{err: sentinel},
		executeCommand: backlogFixtureExecutor(t, fixture, new([]int)),
	}
	err := application.run([]string{"backlog", "health", "--format", "json"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want wrapped %q", err, sentinel)
	}
	if backlogExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want ordinary exit code 1", backlogExitCode(err))
	}
}

func assertBacklogResult(t *testing.T, result backlogRunResult, want backlogHealthReport, fixture backlogFixture, format string) {
	t.Helper()
	if result.code != backlogExpectedExitCode(want) {
		t.Fatalf("%s exit code = %d, want %d (err=%v)", format, result.code, backlogExpectedExitCode(want), result.err)
	}
	if !reflect.DeepEqual(result.dependencyCall, append(append([]int(nil), fixture.selected...), fixture.blockedNumber)) {
		t.Fatalf("%s dependency order = %v, want %v", format, result.dependencyCall,
			append(append([]int(nil), fixture.selected...), fixture.blockedNumber))
	}
	if format == "json" {
		wantJSON := expectedBacklogJSON(want)
		if result.output != wantJSON {
			t.Fatalf("JSON = %q, want exact bytes %q", result.output, wantJSON)
		}
		if !want.Healthy && result.err.Error() != expectedBacklogStateError(want) {
			t.Fatalf("JSON error = %q, want %q", result.err, expectedBacklogStateError(want))
		}
		return
	}

	wantText := fmt.Sprintf("Ready: %d (XS=%d S=%d M=%d)\n", want.Counts.Ready, want.Counts.XS, want.Counts.S, want.Counts.M)
	if want.Healthy {
		wantText += "Ready-work buffer: healthy\n"
	}
	if result.output != wantText {
		t.Fatalf("text = %q, want exact bytes %q", result.output, wantText)
	}
	if !want.Healthy && result.err.Error() != expectedBacklogStateError(want) {
		t.Fatalf("text error = %q, want %q", result.err, expectedBacklogStateError(want))
	}
}

func runBacklogFixture(t *testing.T, args []string, fixture backlogFixture) backlogRunResult {
	t.Helper()
	var output bytes.Buffer
	var dependencyCalls []int
	application := app{
		ctx:            context.Background(),
		stdout:         &output,
		executeCommand: backlogFixtureExecutor(t, fixture, &dependencyCalls),
	}
	err := application.run(args)
	return backlogRunResult{
		code:           backlogExitCode(err),
		output:         output.String(),
		err:            err,
		dependencyCall: dependencyCalls,
	}
}

func backlogFixtureExecutor(t *testing.T, fixture backlogFixture, dependencyCalls *[]int) commandExecutor {
	t.Helper()
	encodedList, err := json.Marshal(fixture.list)
	if err != nil {
		t.Fatalf("encode Project fixture: %v", err)
	}
	return func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name == "git" && reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
			return "/repo", nil
		}
		if name == "gh" && strings.Join(args, " ") == "project item-list 1 --owner goxdra --format json --limit 500" {
			return string(encodedList), nil
		}
		number, ok := backlogDependencyNumber(args)
		if name != "gh" || !ok {
			return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
		*dependencyCalls = append(*dependencyCalls, number)
		relation, ok := fixture.relations[number]
		if !ok {
			return "", fmt.Errorf("unexpected dependency query for issue #%d", number)
		}
		response := issueRelationsResponse{}
		response.Data.Repository.Issue = &relation
		encoded, err := json.Marshal(response)
		if err != nil {
			return "", fmt.Errorf("encode dependency fixture: %w", err)
		}
		return string(encoded), nil
	}
}

func backlogDependencyNumber(args []string) (int, bool) {
	if len(args) != 10 || args[0] != "api" || args[1] != "graphql" || args[2] != "-f" ||
		args[3] != "query="+issueRelationsQuery || args[4] != "-f" || args[5] != "owner=goxdra" ||
		args[6] != "-f" || args[7] != "repository=goxsd9" || args[8] != "-F" {
		return 0, false
	}
	value, ok := strings.CutPrefix(args[9], "number=")
	if !ok {
		return 0, false
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return number, true
}

func backlogHealthFixture(mask int) (backlogFixture, backlogHealthReport) {
	floors := backlogHealthFloors{Ready: 8, XS: 2, S: 3, M: 2}
	counts := backlogHealthCounts(floors)
	if mask&backlogReadyDeficit != 0 {
		counts.Ready--
	}
	if mask&backlogXSDeficit != 0 {
		counts.XS--
	}
	if mask&backlogSDeficit != 0 {
		counts.S--
	}
	if mask&backlogMDeficit != 0 {
		counts.M--
	}

	items := []projectItem{
		backlogProjectItem(900, "In Progress", "Issue", repositoryKey, "M"),
		backlogProjectItem(901, "Ready", "PullRequest", repositoryKey, "XS"),
		backlogProjectItem(902, "Ready", "Issue", "other/example", "S"),
	}
	selected := make([]int, 0, counts.Ready)
	number := 1
	appendItems := func(effort string, count int) {
		for index := 0; index < count; index++ {
			items = append(items, backlogProjectItem(number, "Ready", "Issue", repositoryKey, effort))
			selected = append(selected, number)
			number++
		}
	}
	appendItems("XS", counts.XS)
	appendItems("S", counts.S)
	appendItems("M", counts.M)
	unknown := counts.Ready - counts.XS - counts.S - counts.M
	appendItems("XL", unknown)

	blockedNumber := 999
	items = append(items, backlogProjectItem(blockedNumber, "Ready", "Issue", repositoryKey, "XS"))
	relations := make(map[int]issueRelations, len(selected)+1)
	for _, issueNumber := range selected {
		relations[issueNumber] = issueRelations{}
	}
	if len(selected) != 0 {
		relations[selected[0]] = issueRelations{
			BlockedBy: issueConnection{Nodes: []relatedIssue{{Number: 700, State: "CLOSED"}}},
		}
	}
	relations[blockedNumber] = issueRelations{
		BlockedBy: issueConnection{Nodes: []relatedIssue{{Number: 701, State: "OPEN"}}},
	}
	fixture := backlogFixture{
		list:          projectList{Items: items, TotalCount: len(items)},
		relations:     relations,
		selected:      selected,
		blockedNumber: blockedNumber,
	}
	return fixture, newBacklogHealthReport(counts)
}

func backlogProjectItem(number int, status, itemType, repository, effort string) projectItem {
	return projectItem{
		Content: projectContent{Number: number, Repository: repository, Type: itemType},
		Effort:  effort,
		Status:  status,
	}
}

func parseBacklogTextCounts(t *testing.T, output string) backlogHealthCounts {
	t.Helper()
	firstLine := strings.SplitN(output, "\n", 2)[0]
	var counts backlogHealthCounts
	if fields, err := fmt.Sscanf(firstLine, "Ready: %d (XS=%d S=%d M=%d)", &counts.Ready, &counts.XS, &counts.S, &counts.M); err != nil || fields != 4 {
		t.Fatalf("parse text counts: fields=%d err=%v output=%q", fields, err, output)
	}
	return counts
}

func expectedBacklogJSON(report backlogHealthReport) string {
	return fmt.Sprintf("{\"counts\":{\"ready\":%d,\"xs\":%d,\"s\":%d,\"m\":%d},\"floors\":{\"ready\":8,\"xs\":2,\"s\":3,\"m\":2},\"deficits\":{\"ready\":%d,\"xs\":%d,\"s\":%d,\"m\":%d},\"healthy\":%t}\n",
		report.Counts.Ready, report.Counts.XS, report.Counts.S, report.Counts.M,
		report.Deficits.Ready, report.Deficits.XS, report.Deficits.S, report.Deficits.M, report.Healthy)
}

func expectedBacklogStateError(report backlogHealthReport) string {
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
	return fmt.Sprintf("ready-work buffer is below target: need %v", deficits)
}

func backlogExpectedExitCode(report backlogHealthReport) int {
	if report.Healthy {
		return 0
	}
	return 3
}

func backlogExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}
	return 1
}

type backlogFailingWriter struct {
	err error
}

func (w backlogFailingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
