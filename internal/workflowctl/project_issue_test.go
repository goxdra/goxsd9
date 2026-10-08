package workflowctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

func projectIssueTestNode(number int, id, project string) string {
	return fmt.Sprintf(`{"id":%q,"type":"ISSUE","isArchived":false,"project":{"id":%q,"number":1},"content":{"__typename":"Issue","id":"issue-%d","number":%d,"repository":{"id":"repo-id","nameWithOwner":"goxdra/goxsd9"}},"fieldValueByName":{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Backlog","optionId":"option-id","field":{"id":%q}}}`, id, project, number, number, claimResumeStatusFieldID)
}

func TestClaimResumeUsesFreshBoundedProjectProofs(t *testing.T) {
	fixture := newClaimResumeFixture(t)
	backend := newClaimResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(claimResumeArgs(fixture, false)); err != nil {
		t.Fatalf("claim resume: %v", err)
	}
	assertClaimResumeRenewed(t, fixture, backend)
	if got := backend.projectItemReads(); got != 6 {
		t.Fatalf("Project proof requests = %d, want six complete fresh issue-scoped reads", got)
	}
	for _, call := range backend.calls {
		if strings.HasPrefix(call, "gh project item-list ") {
			t.Fatalf("claim recovery invoked whole-Project inventory: %s", call)
		}
		if !strings.HasPrefix(call, "gh api graphql -f query="+claimResumeProjectIssueQuery) {
			continue
		}
		if !strings.Contains(call, fmt.Sprintf("-F number=%d", fixture.issue)) || strings.Contains(call, "-f after=") {
			t.Fatalf("one-page proof request has wrong issue or cursor: %s", call)
		}
	}
}

func TestClaimResumePaginatesEveryFreshProjectProof(t *testing.T) {
	fixture := newClaimResumeFixture(t)
	backend := newClaimResumeBackend(t, fixture)
	backend.projectPage = func(read int, after string) (string, error) {
		if read%2 == 1 {
			if after != "" {
				return "", fmt.Errorf("first page cursor = %q", after)
			}
			foreign := projectIssueTestNode(fixture.issue, "foreign", "other-project")
			return projectIssueTestPage(fixture.issue, 2, foreign, true, "next"), nil
		}
		if after != "next" {
			return "", fmt.Errorf("second page cursor = %q", after)
		}
		page := claimResumeProjectPageJSON(fixture.issue, backend.projectStatus)
		return strings.Replace(page, `"totalCount":1`, `"totalCount":2`, 1), nil
	}
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(claimResumeArgs(fixture, false)); err != nil {
		t.Fatalf("two-page claim resume: %v", err)
	}
	assertClaimResumeRenewed(t, fixture, backend)
	if got := backend.projectItemReads(); got != 12 {
		t.Fatalf("two-page Project proof requests = %d, want 12", got)
	}
	for _, call := range backend.calls {
		if strings.HasPrefix(call, "gh project item-list ") {
			t.Fatalf("claim recovery invoked inventory: %s", call)
		}
	}
}

func TestClaimResumeProjectProofChangesBeforeMutationFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		change func(string, int) string
	}{
		{name: "item ID", change: func(page string, issue int) string {
			return strings.Replace(page, fmt.Sprintf(`"id":"item-%d"`, issue), `"id":"replacement"`, 1)
		}},
		{name: "issue ID", change: func(page string, issue int) string {
			return strings.ReplaceAll(page, fmt.Sprintf("issue-%d", issue), "replacement-issue")
		}},
		{name: "repository ID", change: func(page string, _ int) string {
			return strings.ReplaceAll(page, "repo-id", "replacement-repo")
		}},
		{name: "status", change: func(page string, _ int) string {
			return strings.Replace(page, `"name":"Backlog"`, `"name":"Ready"`, 1)
		}},
		{name: "status option", change: func(page string, _ int) string {
			return strings.Replace(page, `"optionId":"option-id"`, `"optionId":"replacement-option"`, 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newClaimResumeFixture(t)
			backend := newClaimResumeBackend(t, fixture)
			backend.projectPage = func(read int, _ string) (string, error) {
				page := claimResumeProjectPageJSON(fixture.issue, backend.projectStatus)
				if read == 3 {
					page = test.change(page, fixture.issue)
				}
				return page, nil
			}
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(claimResumeArgs(fixture, false))
			var state *exitError
			if err == nil || !errors.As(err, &state) || state.code != 3 {
				t.Fatalf("changed %s proof = %v, want fail-closed state error", test.name, err)
			}
			if got := claimResumeGitHubMutations(backend.calls); len(got) != 0 {
				t.Fatalf("changed %s proof mutated GitHub: %v", test.name, got)
			}
		})
	}
}

func TestClaimResumeProjectRateLimitRetriesWithOriginalCause(t *testing.T) {
	fixture := newClaimResumeFixture(t)
	backend := newClaimResumeBackend(t, fixture)
	sentinel := errors.New("GraphQL rate limit reset pending")
	backend.projectPage = func(read int, _ string) (string, error) {
		if read == 3 {
			return "", sentinel
		}
		return claimResumeProjectPageJSON(fixture.issue, backend.projectStatus), nil
	}
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	err := application.run(claimResumeArgs(fixture, false))
	if !errors.Is(err, sentinel) || operationDispositionOf(err) != operationDispositionRetryable {
		t.Fatalf("rate-limit proof error = %v, want retryable original cause", err)
	}
	if got := claimResumeGitHubMutations(backend.calls); len(got) != 0 {
		t.Fatalf("rate-limited proof mutated GitHub: %v", got)
	}
	if err := application.run(claimResumeArgs(fixture, false)); err != nil {
		t.Fatalf("retry after rate limit: %v", err)
	}
	assertClaimResumeRenewed(t, fixture, backend)
}

func projectIssueTestPage(number, total int, nodes string, next bool, cursor string) string {
	return fmt.Sprintf(`{"data":{"repository":{"id":"repo-id","nameWithOwner":"goxdra/goxsd9","issue":{"id":"issue-%d","number":%d,"projectItems":{"totalCount":%d,"nodes":[%s],"pageInfo":{"hasNextPage":%t,"endCursor":%q}}}}}}`, number, number, total, nodes, next, cursor)
}

func TestClaimResumeProjectIssueReaderPaginatesCompleteMembership(t *testing.T) {
	const issue = 641
	foreign := projectIssueTestNode(issue, "foreign", "other-project")
	canonical := projectIssueTestNode(issue, "canonical", projectID)
	var calls []string
	application := app{ctx: context.Background(), executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "gh" || !slices.Contains(args, "query="+claimResumeProjectIssueQuery) {
			return "", fmt.Errorf("unexpected command %s %s", name, strings.Join(args, " "))
		}
		if !slices.Contains(args, "owner="+owner) || !slices.Contains(args, "repository="+repository) || !slices.Contains(args, "number=641") {
			return "", fmt.Errorf("query omitted canonical issue variables: %v", args)
		}
		calls = append(calls, claimResumeProjectAfter(args))
		if len(calls) == 1 {
			return projectIssueTestPage(issue, 2, foreign, true, "next"), nil
		}
		if len(calls) == 2 {
			return projectIssueTestPage(issue, 2, canonical, false, "end"), nil
		}
		return "", errors.New("unexpected third page")
	}}
	item, err := application.readClaimResumeProjectItem(t.TempDir(), issue)
	if err != nil {
		t.Fatalf("read canonical item: %v", err)
	}
	if item != (claimResumeProjectItem{ID: "canonical", Status: "Backlog", StatusOptionID: "option-id", IssueID: "issue-641", RepositoryID: "repo-id"}) {
		t.Fatalf("canonical item = %+v", item)
	}
	if !slices.Equal(calls, []string{"", "next"}) {
		t.Fatalf("pagination cursors = %v", calls)
	}
}

func TestClaimResumeProjectIssueReaderRejectsIncompleteAuthority(t *testing.T) {
	const issue = 641
	canonical := projectIssueTestNode(issue, "canonical", projectID)
	base := projectIssueTestPage(issue, 1, canonical, false, "end")
	tests := []struct {
		name  string
		pages []string
	}{
		{name: "missing canonical", pages: []string{projectIssueTestPage(issue, 1, projectIssueTestNode(issue, "foreign", "other-project"), false, "end")}},
		{name: "duplicate canonical", pages: []string{projectIssueTestPage(issue, 2, canonical+","+projectIssueTestNode(issue, "second", projectID), false, "end")}},
		{name: "duplicate item across pages", pages: []string{projectIssueTestPage(issue, 2, canonical, true, "next"), projectIssueTestPage(issue, 2, canonical, false, "end")}},
		{name: "count drift", pages: []string{projectIssueTestPage(issue, 2, canonical, true, "next"), projectIssueTestPage(issue, 3, projectIssueTestNode(issue, "foreign", "other-project"), false, "end")}},
		{name: "issue identity drift", pages: []string{projectIssueTestPage(issue, 2, projectIssueTestNode(issue, "foreign", "other-project"), true, "next"), strings.ReplaceAll(projectIssueTestPage(issue, 2, canonical, false, "end"), "issue-641", "replacement-issue")}},
		{name: "repository identity drift", pages: []string{projectIssueTestPage(issue, 2, projectIssueTestNode(issue, "foreign", "other-project"), true, "next"), strings.ReplaceAll(projectIssueTestPage(issue, 2, canonical, false, "end"), "repo-id", "replacement-repo")}},
		{name: "partial final count", pages: []string{projectIssueTestPage(issue, 2, canonical, false, "end")}},
		{name: "successor after full count", pages: []string{projectIssueTestPage(issue, 1, canonical, true, "next")}},
		{name: "empty successor page", pages: []string{projectIssueTestPage(issue, 1, "", true, "next")}},
		{name: "repeated cursor", pages: []string{projectIssueTestPage(issue, 2, canonical, true, "next"), projectIssueTestPage(issue, 2, projectIssueTestNode(issue, "foreign", "other-project"), true, "next")}},
		{name: "null node", pages: []string{projectIssueTestPage(issue, 1, "null", false, "end")}},
		{name: "null nodes", pages: []string{strings.Replace(base, `"nodes":[`+canonical+`]`, `"nodes":null`, 1)}},
		{name: "null connection", pages: []string{strings.Replace(base, `"projectItems":{"totalCount"`, `"projectItems":null,"unused":{"totalCount"`, 1)}},
		{name: "missing page info", pages: []string{strings.Replace(base, `"pageInfo":{"hasNextPage":false,"endCursor":"end"}`, `"pageInfo":null`, 1)}},
		{name: "wrong repository name", pages: []string{strings.Replace(base, `"nameWithOwner":"goxdra/goxsd9"`, `"nameWithOwner":"elsewhere/goxsd9"`, 1)}},
		{name: "wrong content repository", pages: []string{strings.Replace(base, `"repository":{"id":"repo-id","nameWithOwner":"goxdra/goxsd9"}},"fieldValueByName"`, `"repository":{"id":"wrong","nameWithOwner":"goxdra/goxsd9"}},"fieldValueByName"`, 1)}},
		{name: "wrong issue id", pages: []string{strings.Replace(base, `"content":{"__typename":"Issue","id":"issue-641"`, `"content":{"__typename":"Issue","id":"other"`, 1)}},
		{name: "wrong item type", pages: []string{strings.Replace(base, `"type":"ISSUE"`, `"type":"PULL_REQUEST"`, 1)}},
		{name: "archived canonical", pages: []string{strings.Replace(base, `"isArchived":false`, `"isArchived":true`, 1)}},
		{name: "wrong project number", pages: []string{strings.Replace(base, `"number":1},"content"`, `"number":2},"content"`, 1)}},
		{name: "missing status", pages: []string{strings.Replace(base, `"fieldValueByName":{"__typename"`, `"fieldValueByName":null,"unused":{"__typename"`, 1)}},
		{name: "wrong status field", pages: []string{strings.Replace(base, claimResumeStatusFieldID, "foreign-status-field", 1)}},
		{name: "partial GraphQL error", pages: []string{strings.Replace(base, `"data":`, `"errors":[{"message":"API rate limit exceeded"}],"data":`, 1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertClaimResumeProjectReaderRejects(t, issue, test.name, test.pages)
		})
	}
}

func assertClaimResumeProjectReaderRejects(t *testing.T, issue int, name string, pages []string) {
	t.Helper()
	read := 0
	application := app{ctx: context.Background(), executeCommand: func(_ string, _ io.Reader, command string, args ...string) (string, error) {
		if command != "gh" || !slices.Contains(args, "query="+claimResumeProjectIssueQuery) {
			return "", fmt.Errorf("unexpected command %s %s", command, strings.Join(args, " "))
		}
		read++
		if read > len(pages) {
			return "", errors.New("unexpected extra page")
		}
		return pages[read-1], nil
	}}
	item, err := application.readClaimResumeProjectItem(t.TempDir(), issue)
	if err == nil || item != (claimResumeProjectItem{}) {
		t.Fatalf("item/error = %+v/%v, want no item and failure", item, err)
	}
	if name == "partial GraphQL error" {
		if operationDispositionOf(err) != operationDispositionRetryable || !strings.Contains(err.Error(), "rate limit") {
			t.Fatalf("GraphQL limit error = %v, want retryable preserved cause", err)
		}
		return
	}
	if operationDispositionOf(err) != operationDispositionTerminal {
		t.Fatalf("malformed proof error = %v, want terminal", err)
	}
}

func TestClaimResumeProjectIssueReaderTransportFailureRetainsCause(t *testing.T) {
	sentinel := errors.New("rate limit reset pending")
	application := app{ctx: context.Background(), executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "gh" || !slices.Contains(args, "query="+claimResumeProjectIssueQuery) {
			return "", fmt.Errorf("unexpected command %s %s", name, strings.Join(args, " "))
		}
		return "", sentinel
	}}
	item, err := application.readClaimResumeProjectItem(t.TempDir(), 641)
	if item != (claimResumeProjectItem{}) || !errors.Is(err, sentinel) || operationDispositionOf(err) != operationDispositionRetryable {
		t.Fatalf("transport item/error = %+v/%v, want retryable original cause", item, err)
	}
}
