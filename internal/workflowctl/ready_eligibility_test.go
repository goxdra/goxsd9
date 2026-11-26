package workflowctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadyConsumersExcludeHeldIssueAndPreserveSelection(t *testing.T) {
	created := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	items := []projectItem{
		readyTestItem(11, "Ready", "Issue", repositoryKey, "XS", "P0", []string{"needs-human"}),
		readyTestItem(12, "Ready", "Issue", repositoryKey, "S", "P1", nil),
		readyTestItem(13, "Ready", "Issue", repositoryKey, "XS", "P1", []string{"area/workflow", "needs-human-review"}),
		readyTestItem(17, "Ready", "Issue", repositoryKey, "M", "P2", nil),
		readyTestItem(14, "Ready", "Issue", repositoryKey, "M", "P0", nil),
		readyTestItem(15, "Backlog", "Issue", repositoryKey, "XS", "P0", nil),
		readyTestItem(16, "Ready", "PullRequest", repositoryKey, "XS", "P0", nil),
		readyTestItem(18, "Ready", "DraftIssue", repositoryKey, "XS", "P0", nil),
	}
	items[3].Content.State = "CLOSED"
	fixture := backlogFixture{
		list: projectList{Items: items},
		relations: map[int]issueRelations{
			12: {Blocking: issueConnection{Nodes: []relatedIssue{{Number: 90, State: "OPEN"}, {Number: 91, State: "OPEN"}}}, CreatedAt: created},
			13: {Blocking: issueConnection{Nodes: []relatedIssue{{Number: 92, State: "OPEN"}}}, CreatedAt: created},
			17: {CreatedAt: created},
			14: {BlockedBy: issueConnection{Nodes: []relatedIssue{{Number: 93, State: "OPEN"}}}, CreatedAt: created},
		},
		selected:      []int{12, 13, 17},
		blockedNumber: 14,
	}

	assertBacklogHealthFormats(t, fixture, newBacklogHealthReport(backlogHealthCounts{Ready: 3, XS: 1, S: 1, M: 1}))

	for _, args := range [][]string{{"pick"}, {"pick", "--json"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			output, calls, err := runReadyPickFixture(t, fixture, "", args)
			if err != nil {
				t.Fatalf("pick: %v", err)
			}
			if !reflect.DeepEqual(calls, []int{12, 13, 17, 14}) {
				t.Fatalf("dependency calls = %v, want [12 13 17 14]", calls)
			}
			want := "#12 [P1/S] Issue #12\nhttps://example.test/12\n"
			if len(args) == 2 {
				want = `{"blocking":2,"effort":"S","number":12,"phase":"Bootstrap","priority":"P1","title":"Issue #12","url":"https://example.test/12"}` + "\n"
			}
			if output != want {
				t.Fatalf("pick output = %q, want %q", output, want)
			}
		})
	}
}

func TestReadyConsumersHeldOnlyRetainNoWorkAndDeficits(t *testing.T) {
	fixture := backlogFixture{
		list: projectList{Items: []projectItem{
			readyTestItem(21, "Ready", "Issue", repositoryKey, "XS", "P0", []string{"area/workflow", "needs-human"}),
			readyTestItem(22, "Ready", "Issue", repositoryKey, "S", "P1", []string{"needs-human"}),
			readyTestItem(23, "Backlog", "Issue", repositoryKey, "M", "P2", nil),
			readyTestItem(24, "Ready", "PullRequest", repositoryKey, "XS", "P0", nil),
			readyTestItem(25, "Ready", "Issue", repositoryKey, "M", "P1", nil),
		}},
		relations:     map[int]issueRelations{25: {BlockedBy: issueConnection{Nodes: []relatedIssue{{Number: 99, State: "OPEN"}}}}},
		blockedNumber: 25,
	}
	output, calls, err := runReadyPickFixture(t, fixture, "", []string{"pick", "--json"})
	if backlogExitCode(err) != 3 || err == nil || err.Error() != "no unblocked Ready issue is available; run backlog curation" {
		t.Fatalf("pick error = %v (exit %d), want existing no-work diagnostic", err, backlogExitCode(err))
	}
	if output != "" || !reflect.DeepEqual(calls, []int{25}) {
		t.Fatalf("pick output = %q, dependency calls = %v, want no output and one blocked issue read", output, calls)
	}
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			args := []string{"backlog", "health"}
			if format == "json" {
				args = append(args, "--format", "json")
			}
			result := runBacklogFixture(t, args, fixture)
			assertBacklogResult(t, result, newBacklogHealthReport(backlogHealthCounts{}), fixture, format)
		})
	}
}

func TestReadyHealthKeepsRepositoryFilterBeforeDependencies(t *testing.T) {
	fixture := backlogFixture{list: projectList{Items: []projectItem{
		readyTestItem(31, "Ready", "Issue", "other/example", "XS", "P0", nil),
		readyTestItem(32, "Ready", "Issue", repositoryKey, "S", "P1", []string{"needs-human"}),
		readyTestItem(33, "Ready", "Issue", repositoryKey, "M", "P2", []string{"area/workflow"}),
		readyTestItem(34, "Ready", "Issue", repositoryKey, "XS", "P1", nil),
	}}, relations: map[int]issueRelations{
		33: {},
		34: {BlockedBy: issueConnection{Nodes: []relatedIssue{{Number: 99, State: "OPEN"}}}},
	}, selected: []int{33}, blockedNumber: 34}
	assertBacklogHealthFormats(t, fixture, newBacklogHealthReport(backlogHealthCounts{Ready: 1, M: 1}))
}

func TestReadyPickFiltersForeignRepositoryBeforeDependencies(t *testing.T) {
	for _, foreignNumber := range []int{5, 42} {
		t.Run(fmt.Sprintf("foreign-%d", foreignNumber), func(t *testing.T) {
			assertReadyPickForeignCase(t, foreignNumber)
		})
	}
}

func assertReadyPickForeignCase(t *testing.T, foreignNumber int) {
	t.Helper()
	foreign := readyTestItem(foreignNumber, "Ready", "Issue", "other/example", "XS", "P0", nil)
	foreign.Title = "Foreign issue"
	foreign.Content.URL = "https://example.test/foreign"
	fixture := backlogFixture{
		list: projectList{Items: []projectItem{
			foreign,
			readyTestItem(44, "Ready", "Issue", repositoryKey, "XS", "P2", nil),
			readyTestItem(42, "Ready", "Issue", repositoryKey, "S", "P1", nil),
		}},
		relations: map[int]issueRelations{
			44: {},
			42: {BlockedBy: issueConnection{Nodes: []relatedIssue{{Number: 99, State: "CLOSED"}}}},
		},
	}
	assertReadyRepositoryHealth(t, fixture, backlogHealthCounts{Ready: 2, XS: 1, S: 1}, []int{44, 42})
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"pick"}, want: "#42 [P1/S] Issue #42\nhttps://example.test/42\n"},
		{args: []string{"pick", "--json"}, want: `{"blocking":0,"effort":"S","number":42,"phase":"Bootstrap","priority":"P1","title":"Issue #42","url":"https://example.test/42"}` + "\n"},
	} {
		output, calls, err := runReadyPickFixture(t, fixture, "", test.args)
		if err != nil {
			t.Fatalf("pick %v: %v", test.args, err)
		}
		if !reflect.DeepEqual(calls, []int{44, 42}) {
			t.Fatalf("dependency calls = %v, want only canonical [44 42]", calls)
		}
		if output != test.want {
			t.Fatalf("pick output = %q, want %q", output, test.want)
		}
	}
}

func TestReadyPickForeignOnlyRetainsNoWorkDiagnostic(t *testing.T) {
	fixture := backlogFixture{list: projectList{Items: []projectItem{
		readyTestItem(51, "Ready", "Issue", "other/example", "XS", "P0", nil),
		readyTestItem(52, "Ready", "Issue", "another/example", "S", "P1", nil),
	}}, relations: map[int]issueRelations{}}
	output, calls, err := runReadyPickFixture(t, fixture, "", []string{"pick", "--json"})
	if backlogExitCode(err) != 3 || err == nil || err.Error() != "no unblocked Ready issue is available; run backlog curation" {
		t.Fatalf("pick error = %v (exit %d), want existing no-work diagnostic", err, backlogExitCode(err))
	}
	if output != "" || len(calls) != 0 {
		t.Fatalf("pick output = %q, dependency calls = %v, want neither", output, calls)
	}
	assertReadyRepositoryHealth(t, fixture, backlogHealthCounts{}, nil)
}

func assertReadyRepositoryHealth(t *testing.T, fixture backlogFixture, counts backlogHealthCounts, calls []int) {
	t.Helper()
	result := runBacklogFixture(t, []string{"backlog", "health", "--format", "json"}, fixture)
	want := newBacklogHealthReport(counts)
	if result.code != backlogExpectedExitCode(want) || result.output != expectedBacklogJSON(want) ||
		!reflect.DeepEqual(result.dependencyCall, calls) {
		t.Fatalf("health = %#v, want counts %#v and dependency calls %v", result, counts, calls)
	}
}

func TestReadyPickRefusesIncompleteInventoryBeforeSelecting(t *testing.T) {
	fixture := backlogFixture{relations: map[int]issueRelations{}}
	for _, response := range []string{
		`{"items":[],"totalCount":1}`,
		`{"items":[{"content":{"type":"Issue","number":41,"repository":"goxdra/goxsd9"},"status":"Ready","labels":["needs-human"]}],"totalCount":1,"unexpected":true}`,
		`{"items":[{"content":{"type":"Issue","number":41,"repository":"other/example"},"status":"Ready"}],"totalCount":2}`,
		`{"items":[{"content":{"type":"Issue","number":0,"repository":"other/example"},"status":"Ready"}],"totalCount":1}`,
	} {
		t.Run(response, func(t *testing.T) {
			output, calls, err := runReadyPickFixture(t, fixture, response, []string{"pick", "--json"})
			if err == nil || backlogExitCode(err) != 1 || !strings.Contains(err.Error(), "decode Project items") {
				t.Fatalf("pick error = %v, want Project decode error", err)
			}
			if output != "" || len(calls) != 0 {
				t.Fatalf("pick output = %q, dependency calls = %v, want neither", output, calls)
			}
		})
	}
}

func readyTestItem(number int, status, kind, repo, effort, priority string, labels []string) projectItem {
	item := backlogProjectItemFields(number, status, kind, repo, fmt.Sprintf("Issue #%d", number), effort, priority, "Bootstrap")
	item.Content.URL = fmt.Sprintf("https://example.test/%d", number)
	item.Labels = labels
	return item
}

func runReadyPickFixture(t *testing.T, fixture backlogFixture, projectOutput string, args []string) (string, []int, error) {
	t.Helper()
	base := newBaseRepositoryFixture(t, false)
	var output bytes.Buffer
	var calls []int
	backend := backlogFixtureExecutor(t, fixture, &calls)
	realCommand := app{ctx: context.Background()}
	application := app{ctx: context.Background(), stdout: &output, executeCommand: func(dir string, input io.Reader, name string, commandArgs ...string) (string, error) {
		if name == "git" {
			if reflect.DeepEqual(commandArgs, []string{"rev-parse", "--show-toplevel"}) {
				return base.primary, nil
			}
			return realCommand.command(dir, name, commandArgs...)
		}
		if projectOutput != "" && name == "gh" && strings.HasPrefix(strings.Join(commandArgs, " "), "project item-list ") {
			return projectOutput, nil
		}
		return backend(dir, input, name, commandArgs...)
	}}
	err := application.run(args)
	return output.String(), calls, err
}
