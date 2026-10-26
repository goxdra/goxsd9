package workflowctl

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func runIssueScopedHandoff(t *testing.T, fixture *handoffFixture) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(path, "exact evidence\n"); err != nil {
		t.Fatalf("write handoff body: %v", err)
	}
	var output bytes.Buffer
	application := app{stdout: &output, executeCommand: fixture.execute}
	err := application.runHandoff([]string{"14", "--body-file", path, "--needs-human"})
	return output.String(), err
}

func assertNoHandoffMutation(t *testing.T, fixture *handoffFixture, output string) {
	t.Helper()
	for _, call := range fixture.commands {
		if strings.HasPrefix(call, "gh issue edit ") || strings.HasPrefix(call, "gh project item-edit ") || strings.HasPrefix(call, "gh issue comment ") {
			t.Fatalf("refused proof reached mutation: %s", call)
		}
	}
	if output != "" || fixture.needsHuman || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
		t.Fatalf("refused proof state: output %q, label %t, Project %s, comments %d", output, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
	}
}

func TestNeedsHumanProjectReadFootprint(t *testing.T) {
	// The previous deterministic handoff command sequence contained three
	// whole-Project item-list reads. This fixture fails on any such command.
	fixture := newHandoffFixture()
	if _, err := runIssueScopedHandoff(t, fixture); err != nil {
		t.Fatalf("handoff: %v", err)
	}
	if fixture.projectReads != 6 {
		t.Fatalf("Project footprint: repaired issue pages %d, want six", fixture.projectReads)
	}
	for _, call := range fixture.commands {
		if strings.HasPrefix(call, "gh project item-list ") {
			t.Fatalf("handoff enumerated Project: %s", call)
		}
		if !strings.HasPrefix(call, "gh api graphql ") {
			continue
		}
		if !strings.Contains(call, "-F number=14") || strings.Contains(call, "-f after=") {
			t.Fatalf("one-page handoff proof targeted wrong issue or cursor: %s", call)
		}
	}

	// The shared transition previously performed one full inventory read.
	shared := newHandoffFixture()
	application := app{executeCommand: shared.execute}
	if err := application.transitionIssueToNeedsHuman("/repo", 14); err != nil {
		t.Fatalf("shared transition: %v", err)
	}
	if shared.projectReads != 3 {
		t.Fatalf("shared footprint: repaired issue pages %d, want three", shared.projectReads)
	}
	if !shared.needsHuman || shared.projectStatus != "Backlog" {
		t.Fatalf("shared transition state = label %t, Project %s", shared.needsHuman, shared.projectStatus)
	}
}

func TestHandoffPaginatesEveryFreshProofAcrossProjects(t *testing.T) {
	fixture := newHandoffFixture()
	fixture.projectPage = func(read int, after string) (string, error) {
		if read%2 == 1 {
			if after != "" {
				return "", fmt.Errorf("first page cursor = %q", after)
			}
			return projectIssueTestPage(14, 2, projectIssueTestNode(14, "foreign", "other-project"), true, "next"), nil
		}
		if after != "next" {
			return "", fmt.Errorf("second page cursor = %q", after)
		}
		return strings.Replace(fixture.projectItemsJSON(), `"totalCount":1`, `"totalCount":2`, 1), nil
	}
	if _, err := runIssueScopedHandoff(t, fixture); err != nil {
		t.Fatalf("two-page handoff: %v", err)
	}
	if fixture.projectReads != 12 || !fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 1 {
		t.Fatalf("two-page footprint/state = %d pages, label %t, Project %s, comments %d", fixture.projectReads, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
	}
}

func TestHandoffRejectsChangedProjectProofBeforeFirstWrite(t *testing.T) {
	tests := []struct {
		name   string
		change func(string) string
	}{
		{"item ID", func(page string) string { return strings.Replace(page, `"id":"item-14"`, `"id":"replacement"`, 1) }},
		{"issue ID", func(page string) string { return strings.ReplaceAll(page, "issue-14", "replacement-issue") }},
		{"repository ID", func(page string) string { return strings.ReplaceAll(page, "repo-id", "replacement-repo") }},
		{"status", func(page string) string { return strings.Replace(page, `"name":"Ready"`, `"name":"Picked"`, 1) }},
		{"status option", func(page string) string {
			return strings.Replace(page, `"optionId":"Ready-option"`, `"optionId":"changed-option"`, 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			fixture.projectPage = func(read int, _ string) (string, error) {
				page := fixture.projectItemsJSON()
				if read == 2 {
					page = test.change(page)
				}
				return page, nil
			}
			output, err := runIssueScopedHandoff(t, fixture)
			var state *exitError
			if err == nil || !errors.As(err, &state) || state.code != 3 || !strings.Contains(err.Error(), "pre-mutation proof changed") {
				t.Fatalf("changed %s proof = %v, want state refusal", test.name, err)
			}
			assertNoHandoffMutation(t, fixture, output)
		})
	}
}

func TestHandoffRejectsProjectChangeAtTransitionPreflight(t *testing.T) {
	fixture := newHandoffFixture()
	fixture.projectPage = func(read int, _ string) (string, error) {
		page := fixture.projectItemsJSON()
		if read == 3 {
			return strings.Replace(page, `"id":"item-14"`, `"id":"replacement"`, 1), nil
		}
		return page, nil
	}
	output, err := runIssueScopedHandoff(t, fixture)
	if err == nil || !strings.Contains(err.Error(), "transition preflight changed") {
		t.Fatalf("changed transition proof = %v", err)
	}
	assertNoHandoffMutation(t, fixture, output)
}

func TestHandoffRejectsIncompleteMembershipWithoutMutation(t *testing.T) {
	canonical := projectIssueTestNode(14, "item-14", projectID)
	base := claimResumeProjectPageJSON(14, "Ready")
	tests := []struct {
		name string
		page string
	}{
		{"duplicate canonical", projectIssueTestPage(14, 2, canonical+","+projectIssueTestNode(14, "second", projectID), false, "")},
		{"archived canonical", strings.Replace(base, `"isArchived":false`, `"isArchived":true`, 1)},
		{"partial count", strings.Replace(base, `"totalCount":1`, `"totalCount":2`, 1)},
		{"malformed status", strings.Replace(base, claimResumeStatusFieldID, "wrong-field", 1)},
		{"malformed JSON", `{"data":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			fixture.projectPage = func(_ int, _ string) (string, error) { return test.page, nil }
			output, err := runIssueScopedHandoff(t, fixture)
			if err == nil || operationDispositionOf(err) != operationDispositionTerminal {
				t.Fatalf("incomplete %s proof = %v, want terminal refusal", test.name, err)
			}
			assertNoHandoffMutation(t, fixture, output)
		})
	}
}

func TestHandoffProjectReadFailurePreservesCauseAndRetry(t *testing.T) {
	for _, failRead := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("read %d", failRead), func(t *testing.T) {
			fixture := newHandoffFixture()
			sentinel := errors.New("installation quota exhausted")
			fixture.projectPage = func(read int, _ string) (string, error) {
				if read == failRead {
					return "", sentinel
				}
				return fixture.projectItemsJSON(), nil
			}
			output, err := runIssueScopedHandoff(t, fixture)
			if !errors.Is(err, sentinel) || operationDispositionOf(err) != operationDispositionRetryable {
				t.Fatalf("Project read error = %v, want retryable original cause", err)
			}
			assertNoHandoffMutation(t, fixture, output)
			fixture.projectPage = nil
			if _, err := runIssueScopedHandoff(t, fixture); err != nil {
				t.Fatalf("retry after Project read failure: %v", err)
			}
			if len(fixture.comments) != 1 {
				t.Fatalf("retry comments = %d, want one", len(fixture.comments))
			}
		})
	}
}

func TestHandoffPartialGraphQLFailurePreservesCauseWithoutMutation(t *testing.T) {
	fixture := newHandoffFixture()
	fixture.projectPage = func(_ int, _ string) (string, error) {
		page := fixture.projectItemsJSON()
		return strings.Replace(page, `"data":`, `"errors":[{"message":"installation quota exhausted"}],"data":`, 1), nil
	}
	output, err := runIssueScopedHandoff(t, fixture)
	if err == nil || operationDispositionOf(err) != operationDispositionRetryable || !strings.Contains(err.Error(), "installation quota exhausted") {
		t.Fatalf("partial GraphQL error = %v, want retryable quota cause", err)
	}
	assertNoHandoffMutation(t, fixture, output)
}

//nolint:gocognit // Table cases assert each partial mutation boundary and retry.
func TestHandoffLaterProjectProofFailureReportsIncompletePhase(t *testing.T) {
	tests := []struct {
		read        int
		want        string
		wantProject string
		wantLabel   bool
	}{
		{read: 4, want: "project Backlog phase incomplete after needs-human label", wantProject: "Ready", wantLabel: true},
		{read: 5, want: "project Backlog phase incomplete after needs-human label", wantProject: "Ready", wantLabel: true},
		{read: 6, want: "evidence comment phase incomplete after needs-human transition", wantProject: "Backlog", wantLabel: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("read %d", test.read), func(t *testing.T) {
			fixture := newHandoffFixture()
			sentinel := errors.New("GraphQL transport unavailable")
			fixture.projectPage = func(read int, _ string) (string, error) {
				if read == test.read {
					return "", sentinel
				}
				return fixture.projectItemsJSON(), nil
			}
			output, err := runIssueScopedHandoff(t, fixture)
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, sentinel) || operationDispositionOf(err) != operationDispositionRetryable {
				t.Fatalf("later proof error = %v, want %q and retryable original cause", err, test.want)
			}
			if output != "" || fixture.needsHuman != test.wantLabel || fixture.projectStatus != test.wantProject || len(fixture.comments) != 0 {
				t.Fatalf("later proof state: output %q, label %t, Project %s, comments %d", output, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
			}
			fixture.projectPage = nil
			if _, err := runIssueScopedHandoff(t, fixture); err != nil {
				t.Fatalf("retry after later proof failure: %v", err)
			}
			if len(fixture.comments) != 1 {
				t.Fatalf("retry comments = %d, want one", len(fixture.comments))
			}
		})
	}
}

//nolint:gocognit // Table cases vary independent issue and Project races at one boundary.
func TestHandoffRefusesChangedProofAfterLabelBeforeBacklog(t *testing.T) {
	tests := []struct {
		name   string
		change func(*handoffFixture)
		want   string
	}{
		{"closed issue", func(f *handoffFixture) {
			f.issueAtRead = func(read int) {
				if read == 4 {
					f.issueState = "closed"
				}
			}
		}, "requires OPEN issue"},
		{"lost label", func(f *handoffFixture) {
			f.issueAtRead = func(read int) {
				if read == 4 {
					f.needsHuman = false
				}
			}
		}, "issue or Project proof changed"},
		{"changed item", func(f *handoffFixture) {
			f.projectPage = func(read int, _ string) (string, error) {
				page := f.projectItemsJSON()
				if read == 4 {
					return strings.Replace(page, `"id":"item-14"`, `"id":"changed-item"`, 1), nil
				}
				return page, nil
			}
		}, "issue or Project proof changed"},
		{"changed status option", func(f *handoffFixture) {
			f.projectPage = func(read int, _ string) (string, error) {
				page := f.projectItemsJSON()
				if read == 4 {
					return strings.Replace(page, `"optionId":"Ready-option"`, `"optionId":"changed-option"`, 1), nil
				}
				return page, nil
			}
		}, "issue or Project proof changed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			test.change(fixture)
			output, err := runIssueScopedHandoff(t, fixture)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("post-label proof error = %v, want %q", err, test.want)
			}
			if output != "" || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
				t.Fatalf("post-label refusal wrote Backlog/evidence: output %q, status %s, comments %d", output, fixture.projectStatus, len(fixture.comments))
			}
			for _, call := range fixture.commands {
				if strings.HasPrefix(call, "gh project item-edit ") || strings.HasPrefix(call, "gh issue comment ") {
					t.Fatalf("post-label refusal reached later mutation: %s", call)
				}
			}
		})
	}
}

//nolint:gocognit // Both entry points exercise the same immediate pre-write race.
func TestNeedsHumanRefusesChangedProofImmediatelyBeforeProjectWrite(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "handoff"
		if shared {
			name = "shared transition"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newHandoffFixture()
			readToChange := 5
			if shared {
				readToChange = 3
			}
			fixture.projectPage = func(read int, _ string) (string, error) {
				page := fixture.projectItemsJSON()
				if read == readToChange {
					return strings.Replace(page, `"id":"item-14"`, `"id":"changed-item"`, 1), nil
				}
				return page, nil
			}
			run := func() (string, error) { return runIssueScopedHandoff(t, fixture) }
			if shared {
				run = func() (string, error) {
					application := app{executeCommand: fixture.execute}
					return "", application.transitionIssueToNeedsHuman("/repo", 14)
				}
			}
			output, err := run()
			if err == nil || !strings.Contains(err.Error(), "pre-Project-write proof changed") {
				t.Fatalf("changed pre-write proof = %v", err)
			}
			if output != "" || !fixture.needsHuman || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
				t.Fatalf("pre-write refusal state: output %q, label %t, Project %s, comments %d", output, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
			}
			for _, call := range fixture.commands {
				if strings.HasPrefix(call, "gh project item-edit ") || strings.HasPrefix(call, "gh issue comment ") {
					t.Fatalf("pre-write refusal reached later mutation: %s", call)
				}
			}
		})
	}
}

//nolint:gocognit // Table cases vary independent issue and Project races at one boundary.
func TestHandoffRefusesChangedProofBeforeEvidenceComment(t *testing.T) {
	tests := []struct {
		name   string
		change func(*handoffFixture)
	}{
		{"closed issue", func(f *handoffFixture) {
			f.issueAtRead = func(read int) {
				if read == 6 {
					f.issueState = "closed"
				}
			}
		}},
		{"changed item", func(f *handoffFixture) {
			f.projectPage = func(read int, _ string) (string, error) {
				page := f.projectItemsJSON()
				if read == 6 {
					return strings.Replace(page, `"id":"item-14"`, `"id":"changed-item"`, 1), nil
				}
				return page, nil
			}
		}},
		{"not Backlog", func(f *handoffFixture) {
			f.projectPage = func(read int, _ string) (string, error) {
				page := f.projectItemsJSON()
				if read == 6 {
					return strings.Replace(page, `"name":"Backlog"`, `"name":"Picked"`, 1), nil
				}
				return page, nil
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			test.change(fixture)
			output, err := runIssueScopedHandoff(t, fixture)
			if err == nil || !strings.Contains(err.Error(), "evidence comment") {
				t.Fatalf("pre-comment proof error = %v, want evidence phase", err)
			}
			if output != "" || !fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 0 {
				t.Fatalf("pre-comment refusal state: output %q, label %t, Project %s, comments %d", output, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
			}
			for _, call := range fixture.commands {
				if strings.HasPrefix(call, "gh issue comment ") {
					t.Fatalf("changed evidence proof posted comment: %s", call)
				}
			}
		})
	}
}

func TestHandoffAmbiguousBacklogWriteRequiresSameCanonicalItem(t *testing.T) {
	fixture := newHandoffFixture()
	fixture.projectPage = func(read int, _ string) (string, error) {
		page := fixture.projectItemsJSON()
		if read == 6 {
			return strings.Replace(page, `"id":"item-14"`, `"id":"replacement"`, 1), nil
		}
		return page, nil
	}
	path := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(path, "exact evidence\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fixture.failAfterMutation = "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id"
	var output bytes.Buffer
	application := app{stdout: &output, executeCommand: fixture.execute}
	err := application.runHandoff([]string{"14", "--body-file", path, "--needs-human"})
	if err == nil || !strings.Contains(err.Error(), "project Backlog phase incomplete") || !strings.Contains(err.Error(), "simulated ambiguous handoff response") {
		t.Fatalf("ambiguous changed target error = %v", err)
	}
	if output.Len() != 0 || !fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 0 {
		t.Fatalf("ambiguous changed target state: output %q, label %t, Project %s, comments %d", output.String(), fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
	}
	fixture.failAfterMutation = ""
	fixture.projectPage = nil
	if err := application.runHandoff([]string{"14", "--body-file", path, "--needs-human"}); err != nil {
		t.Fatalf("retry ambiguous Backlog write: %v", err)
	}
	if len(fixture.comments) != 1 {
		t.Fatalf("retry comments = %d, want one", len(fixture.comments))
	}
}

func TestHandoffAmbiguousBacklogWriteRequiresCurrentLabel(t *testing.T) {
	fixture := newHandoffFixture()
	fixture.issueAtRead = func(read int) {
		if read == 6 {
			fixture.needsHuman = false
		}
	}
	path := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(path, "exact evidence\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fixture.failAfterMutation = "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id"
	var output bytes.Buffer
	application := app{stdout: &output, executeCommand: fixture.execute}
	err := application.runHandoff([]string{"14", "--body-file", path, "--needs-human"})
	if err == nil || !strings.Contains(err.Error(), "project Backlog phase incomplete") || !strings.Contains(err.Error(), "simulated ambiguous handoff response") {
		t.Fatalf("ambiguous missing-label error = %v", err)
	}
	if output.Len() != 0 || fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 0 {
		t.Fatalf("ambiguous missing-label state: output %q, label %t, Project %s, comments %d", output.String(), fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
	}
	fixture.failAfterMutation = ""
	fixture.issueAtRead = nil
	if err := application.runHandoff([]string{"14", "--body-file", path, "--needs-human"}); err != nil {
		t.Fatalf("retry missing-label Backlog write: %v", err)
	}
	if !fixture.needsHuman || len(fixture.comments) != 1 {
		t.Fatalf("retry state: label %t, comments %d", fixture.needsHuman, len(fixture.comments))
	}
}

//nolint:gocognit // Each write phase is checked for its own incomplete state and retry.
func TestHandoffRetriesIncompleteLabelAndBacklogWrites(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		phase       string
		wantLabel   bool
		wantProject string
	}{
		{"label", "gh issue edit 14 --repo goxdra/goxsd9 --add-label needs-human", "needs-human label phase incomplete", false, "Ready"},
		{"Backlog", "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id", "project Backlog phase incomplete", true, "Ready"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			fixture.failCommand = test.command
			sentinel := errors.New("specific write transport failure")
			fixture.failCause = sentinel
			output, err := runIssueScopedHandoff(t, fixture)
			if err == nil || !errors.Is(err, sentinel) || !strings.Contains(err.Error(), test.phase) || operationDispositionOf(err) != operationDispositionRetryable {
				t.Fatalf("incomplete %s write = %v, want retryable original cause and phase", test.name, err)
			}
			if output != "" || fixture.needsHuman != test.wantLabel || fixture.projectStatus != test.wantProject || len(fixture.comments) != 0 {
				t.Fatalf("incomplete %s state: output %q, label %t, Project %s, comments %d", test.name, output, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
			}
			fixture.failCommand = ""
			if _, err := runIssueScopedHandoff(t, fixture); err != nil {
				t.Fatalf("retry incomplete %s write: %v", test.name, err)
			}
			if !fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 1 {
				t.Fatalf("retry %s state: label %t, Project %s, comments %d", test.name, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
			}
		})
	}
}
