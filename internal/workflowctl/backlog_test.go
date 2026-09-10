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
	name              string
	projectOut        string
	dependency        bool
	wantProjectDecode bool
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
			textFloors := parseBacklogTextFloors(t, textResult.output)
			if textCounts != jsonReport.Counts || textFloors != jsonReport.Floors || jsonReport.Healthy != want.Healthy {
				t.Fatalf("text and JSON disagree: text counts=%#v floors=%#v JSON=%#v", textCounts, textFloors, jsonReport)
			}
		})
	}
}

func TestBacklogHealthReadyFloorBoundary(t *testing.T) {
	nineFixture, nineReport := backlogReadyBoundaryFixture(9)
	if nineReport.Counts.Ready != 9 || nineReport.Deficits.Ready != 1 || nineReport.Healthy {
		t.Fatalf("nine-ready report = %#v, want count 9, deficit 1, unhealthy", nineReport)
	}
	assertBacklogHealthFormats(t, nineFixture, nineReport)

	tenFixture, tenReport := backlogReadyBoundaryFixture(10)
	if tenReport.Counts.Ready != 10 || tenReport.Deficits.Ready != 0 || !tenReport.Healthy {
		t.Fatalf("ten-ready report = %#v, want count 10, deficit 0, healthy", tenReport)
	}
	assertBacklogHealthFormats(t, tenFixture, tenReport)
}

func TestBacklogHealthReportsIncompleteProjectMetadata(t *testing.T) {
	fixture, want := backlogIncompleteMetadataFixture()

	textResult := runBacklogFixture(t, []string{"backlog", "health"}, fixture)
	assertBacklogResult(t, textResult, want, fixture, "text")

	jsonResult := runBacklogFixture(t, []string{"backlog", "health", "--format", "json"}, fixture)
	assertBacklogResult(t, jsonResult, want, fixture, "json")

	if want.Healthy {
		t.Fatal("incomplete metadata report is healthy")
	}
	if want.Counts != (backlogHealthCounts{Ready: 10, XS: 2, S: 3, M: 2}) {
		t.Fatalf("counts = %#v, want complete Ready floors", want.Counts)
	}
	if !strings.Contains(textResult.output, "#10: \"Picked missing phase\" (missing: Phase)\n") ||
		!strings.Contains(textResult.output, "#20: \"Ready missing planning fields\" (missing: Phase, Priority)\n") ||
		!strings.Contains(textResult.output, "#30: \"Backlog missing\\nplanning fields\" (missing: Effort, Phase, Priority)\n") {
		t.Fatalf("text omitted incomplete metadata findings:\n%s", textResult.output)
	}
}

func TestProjectListAcceptsTypedNonIssueWithoutIdentity(t *testing.T) {
	const response = `{
		"items": [
			{"content": {"type": "DraftIssue", "title": "draft"}},
			{"content": {"type": "PullRequest", "title": "pull request"}},
			{"content": {"type": "Issue", "number": 7, "repository": "goxdra/goxsd9"}, "title": "canonical"},
			{"content": {"type": "Issue", "number": 8, "repository": "other/example"}, "title": "foreign"}
		],
		"totalCount": 4
	}`

	var list projectList
	if err := json.Unmarshal([]byte(response), &list); err != nil {
		t.Fatalf("decode Project items: %v", err)
	}
	findings, err := incompleteProjectItems(list)
	if err != nil {
		t.Fatalf("collect incomplete Project items: %v", err)
	}
	want := []backlogHealthFinding{{
		Number:  7,
		Title:   "canonical",
		Missing: []string{"Effort", "Phase", "Priority", "Status"},
	}}
	if !reflect.DeepEqual(findings, want) {
		t.Fatalf("findings = %#v, want %#v", findings, want)
	}
}

func assertBacklogHealthFormats(t *testing.T, fixture backlogFixture, want backlogHealthReport) {
	t.Helper()
	textResult := runBacklogFixture(t, []string{"backlog", "health"}, fixture)
	assertBacklogResult(t, textResult, want, fixture, "text")
	jsonResult := runBacklogFixture(t, []string{"backlog", "health", "--format", "json"}, fixture)
	assertBacklogResult(t, jsonResult, want, fixture, "json")
}

func backlogReadyBoundaryFixture(ready int) (backlogFixture, backlogHealthReport) {
	counts := backlogHealthCounts{Ready: ready, XS: 2, S: 3, M: ready - 5}
	return backlogFixtureForCounts(counts)
}

func backlogFixtureForCounts(counts backlogHealthCounts) (backlogFixture, backlogHealthReport) {
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
	if unknown > 0 {
		appendItems("XL", unknown)
	}

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

func backlogIncompleteMetadataFixture() (backlogFixture, backlogHealthReport) {
	items := []projectItem{
		backlogProjectItemWithMetadata(60, "Backlog", "Complete backlog item", "S", "P2", "Schema Model"),
		backlogProjectItemWithMetadata(70, "Picked", "Complete picked item", "M", "P2", "Schema Model"),
		backlogProjectItemWithMetadata(80, "Done", "Complete done item", "M", "P2", "Schema Model"),
		backlogProjectItemWithMetadata(100, "Done", "Done missing all fields", "", "", ""),
		backlogProjectItemFields(40, "Backlog", "Issue", "other/example", "Foreign missing all fields", "", "", ""),
		backlogProjectItemFields(50, "Backlog", "PullRequest", repositoryKey, "Pull request missing all fields", "", "", ""),
	}

	selected := make([]int, 0, 10)
	number := 1
	appendReady := func(effort string, count int) {
		for index := 0; index < count; index++ {
			title := fmt.Sprintf("Complete ready item %d", number)
			if number == 1 {
				title = "Ready missing planning fields"
			}
			items = append(items, backlogProjectItemWithMetadata(number, "Ready", title, effort, "P2", "Schema Model"))
			selected = append(selected, number)
			number++
		}
	}
	appendReady("XS", 1)
	appendReady("S", 3)
	appendReady("M", 2)
	appendReady("XL", 3)
	items = append(items,
		backlogProjectItemWithMetadata(30, "Backlog", "Backlog missing\nplanning fields", "", "", ""),
		backlogProjectItemWithMetadata(20, "Ready", "Ready missing planning fields", "XS", "", ""),
		backlogProjectItemWithMetadata(10, "Picked", "Picked missing phase", "M", "P2", ""),
	)
	items[len(items)-3].Content.State = "CLOSED"
	selected = append(selected, 20)

	blockedNumber := 999
	items = append(items, backlogProjectItemWithMetadata(blockedNumber, "Ready", "Blocked ready item", "XS", "P2", "Schema Model"))
	relations := make(map[int]issueRelations, len(selected)+1)
	for _, issueNumber := range selected {
		relations[issueNumber] = issueRelations{}
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
	counts := backlogHealthCounts{Ready: 10, XS: 2, S: 3, M: 2}
	findings := []backlogHealthFinding{
		{Number: 10, Title: "Picked missing phase", Missing: []string{"Phase"}},
		{Number: 20, Title: "Ready missing planning fields", Missing: []string{"Phase", "Priority"}},
		{Number: 30, Title: "Backlog missing\nplanning fields", Missing: []string{"Effort", "Phase", "Priority"}},
	}
	return fixture, newBacklogHealthReportWithFindings(counts, findings)
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
		{name: "project decode", projectOut: "{", wantProjectDecode: true},
		{name: "project partial", projectOut: `{}`, wantProjectDecode: true},
		{name: "project missing total count", projectOut: `{"items":[]}`, wantProjectDecode: true},
		{name: "project null total count", projectOut: `{"items":[],"totalCount":null}`, wantProjectDecode: true},
		{name: "project unknown", projectOut: `{"items":[],"totalCount":0,"unexpected":true}`, wantProjectDecode: true},
		{name: "dependency transport", dependency: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertBacklogHealthError(t, test)
		})
	}
}

func TestProjectListAcceptsExplicitEmptyTotalCount(t *testing.T) {
	var list projectList
	if err := json.Unmarshal([]byte(`{"items":[],"totalCount":0}`), &list); err != nil {
		t.Fatalf("decode empty Project items: %v", err)
	}
	if list.Items == nil || len(list.Items) != 0 || list.TotalCount != 0 {
		t.Fatalf("empty Project list = %#v, want explicit empty list", list)
	}
}

func TestBacklogHealthUsesContentTitleFallback(t *testing.T) {
	const projectOut = `{"items":[{"content":{"number":7,"repository":"goxdra/goxsd9","title":"Real title","type":"Issue"},"effort":"S","priority":"P2","status":"Picked"}],"totalCount":1}`
	want := newBacklogHealthReportWithFindings(backlogHealthCounts{}, []backlogHealthFinding{
		{Number: 7, Title: "Real title", Missing: []string{"Phase"}},
	})

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			output, err := runBacklogProjectResponse(t, projectOut, format)
			if backlogExitCode(err) != 3 {
				t.Fatalf("exit code = %d, want unhealthy code 3 (err=%v)", backlogExitCode(err), err)
			}
			if err == nil || err.Error() != expectedBacklogStateError(want) {
				t.Fatalf("error = %v, want %q", err, expectedBacklogStateError(want))
			}
			wantOutput := expectedBacklogText(t, want)
			if format == "json" {
				wantOutput = expectedBacklogJSON(want)
			}
			if output != wantOutput {
				t.Fatalf("output = %q, want %q", output, wantOutput)
			}
		})
	}
}

func TestBacklogHealthRejectsCanonicalProjectItemWithoutTitle(t *testing.T) {
	const projectOut = `{"items":[{"content":{"number":7,"repository":"goxdra/goxsd9","type":"Issue"},"effort":"S","phase":"Schema Model","priority":"P2","status":"Picked"}],"totalCount":1}`

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			output, err := runBacklogProjectResponse(t, projectOut, format)
			if err == nil || !strings.Contains(err.Error(), "no nonblank title") {
				t.Fatalf("error = %v, want missing-title error", err)
			}
			if backlogExitCode(err) != 1 {
				t.Fatalf("exit code = %d, want ordinary error code 1", backlogExitCode(err))
			}
			if output != "" {
				t.Fatalf("output = %q, want no report", output)
			}
		})
	}
}

func assertBacklogHealthError(t *testing.T, test backlogErrorFixture) {
	t.Helper()
	fixture, _ := backlogHealthFixture(0)
	sentinel := errors.New(test.name)
	output, err := runBacklogHealthError(t, test, fixture, sentinel)
	if test.wantProjectDecode {
		if err == nil || !strings.Contains(err.Error(), "decode Project items") {
			t.Fatalf("error = %v, want Project decode context", err)
		}
	}
	if !test.wantProjectDecode && !errors.Is(err, sentinel) {
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

	wantText := expectedBacklogText(t, want)
	if result.output != wantText {
		t.Fatalf("text = %q, want exact bytes %q", result.output, wantText)
	}
	if !want.Healthy && result.err.Error() != expectedBacklogStateError(want) {
		t.Fatalf("text error = %q, want %q", result.err, expectedBacklogStateError(want))
	}
}

func expectedBacklogText(t *testing.T, report backlogHealthReport) string {
	t.Helper()
	var wantText strings.Builder
	if _, err := fmt.Fprintf(&wantText, "Ready: %d (XS=%d S=%d M=%d)\nReady floor: %d (XS=%d S=%d M=%d)\n",
		report.Counts.Ready, report.Counts.XS, report.Counts.S, report.Counts.M,
		report.Floors.Ready, report.Floors.XS, report.Floors.S, report.Floors.M); err != nil {
		t.Fatalf("format expected text: %v", err)
	}
	if len(report.Incomplete) != 0 {
		wantText.WriteString("Incomplete Project metadata:\n")
		for _, finding := range report.Incomplete {
			if _, err := fmt.Fprintf(&wantText, "#%d: %s (missing: %s)\n", finding.Number, strconv.Quote(finding.Title),
				strings.Join(finding.Missing, ", ")); err != nil {
				t.Fatalf("format expected finding: %v", err)
			}
		}
	}
	if report.Healthy {
		wantText.WriteString("Ready-work buffer: healthy\n")
	}
	return wantText.String()
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

func runBacklogProjectResponse(t *testing.T, projectOut, format string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	application := app{
		ctx:            context.Background(),
		stdout:         &output,
		executeCommand: backlogProjectResponseExecutor(t, projectOut),
	}
	err := application.run([]string{"backlog", "health", "--format", format})
	return output.String(), err
}

func backlogProjectResponseExecutor(t *testing.T, projectOut string) commandExecutor {
	t.Helper()
	return func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name == "git" && reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
			return "/repo", nil
		}
		if name == "gh" && strings.Join(args, " ") == "project item-list 1 --owner goxdra --format json --limit 500" {
			return projectOut, nil
		}
		return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
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
	floors := backlogHealthFloors{Ready: 10, XS: 2, S: 3, M: 2}
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

	return backlogFixtureForCounts(counts)
}

func backlogProjectItem(number int, status, itemType, repository, effort string) projectItem {
	title := fmt.Sprintf("Issue #%d", number)
	return backlogProjectItemFields(number, status, itemType, repository, title, effort, "P2", "Schema Model")
}

func backlogProjectItemWithMetadata(number int, status, title, effort, priority, phase string) projectItem {
	return backlogProjectItemFields(number, status, "Issue", repositoryKey, title, effort, priority, phase)
}

func backlogProjectItemFields(number int, status, itemType, repository, title, effort, priority, phase string) projectItem {
	return projectItem{
		Content:  projectContent{Number: number, Repository: repository, Title: title, Type: itemType},
		Effort:   effort,
		Phase:    phase,
		Priority: priority,
		Status:   status,
		Title:    title,
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

func parseBacklogTextFloors(t *testing.T, output string) backlogHealthFloors {
	t.Helper()
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatalf("parse text floors: output has no floor line: %q", output)
	}
	var floors backlogHealthFloors
	if fields, err := fmt.Sscanf(lines[1], "Ready floor: %d (XS=%d S=%d M=%d)", &floors.Ready, &floors.XS, &floors.S, &floors.M); err != nil || fields != 4 {
		t.Fatalf("parse text floors: fields=%d err=%v output=%q", fields, err, output)
	}
	return floors
}

func expectedBacklogJSON(report backlogHealthReport) string {
	incomplete := ""
	if len(report.Incomplete) != 0 {
		encoded, err := json.Marshal(report.Incomplete)
		if err != nil {
			panic(err)
		}
		incomplete = fmt.Sprintf(",\"incomplete\":%s", encoded)
	}
	return fmt.Sprintf("{\"counts\":{\"ready\":%d,\"xs\":%d,\"s\":%d,\"m\":%d},\"floors\":{\"ready\":10,\"xs\":2,\"s\":3,\"m\":2},\"deficits\":{\"ready\":%d,\"xs\":%d,\"s\":%d,\"m\":%d}%s,\"healthy\":%t}\n",
		report.Counts.Ready, report.Counts.XS, report.Counts.S, report.Counts.M,
		report.Deficits.Ready, report.Deficits.XS, report.Deficits.S, report.Deficits.M, incomplete, report.Healthy)
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
	if len(report.Incomplete) != 0 {
		if len(deficits) == 0 {
			return fmt.Sprintf("Project metadata is incomplete: %d item(s)", len(report.Incomplete))
		}
		return fmt.Sprintf("ready-work buffer is below target: need %v; Project metadata is incomplete: %d item(s)",
			deficits, len(report.Incomplete))
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
