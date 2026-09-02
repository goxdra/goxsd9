package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type handoffFixture struct {
	issueState        string
	needsHuman        bool
	projectStatus     string
	projectMember     bool
	projectItemID     string
	projectType       string
	issueReads        int
	projectReads      int
	raceIssue         bool
	raceProject       bool
	bodyPath          string
	body              string
	comments          []issueCommentAPI
	commands          []string
	failCommand       string
	failAfterMutation string
}

func newHandoffFixture() *handoffFixture {
	return &handoffFixture{
		issueState:    "open",
		projectStatus: "Ready",
		projectMember: true,
		projectItemID: "item-14",
		projectType:   "Issue",
	}
}

func (f *handoffFixture) execute(_ string, _ io.Reader, name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	f.commands = append(f.commands, command)
	if command == f.failCommand {
		return "", errors.New("simulated handoff failure")
	}
	if command == f.failAfterMutation {
		switch command {
		case "gh issue edit 14 --repo goxdra/goxsd9 --add-label needs-human":
			f.needsHuman = true
		case "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id":
			f.projectStatus = "Backlog"
		case "gh issue comment 14 --repo goxdra/goxsd9 --body-file " + f.bodyPath:
			f.comments = append(f.comments, issueCommentAPI{Body: f.body, User: issueCommentUser(trustedActor)})
		}
		return "", errors.New("simulated ambiguous handoff response")
	}
	if name == "git" {
		return f.executeGit(command, args)
	}
	if name != "gh" {
		return "", fmt.Errorf("unexpected command: %s", command)
	}
	return f.executeGitHub(strings.TrimPrefix(command, "gh "))
}

func (f *handoffFixture) executeGit(command string, args []string) (string, error) {
	if reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
		return "/repo", nil
	}
	return "", fmt.Errorf("unexpected git command: %s", command)
}

func (f *handoffFixture) executeGitHub(command string) (string, error) {
	switch command {
	case "api repos/goxdra/goxsd9/issues/14":
		return f.issueStateJSON(), nil
	case "project item-list 1 --owner goxdra --format json --limit 500":
		return f.projectItemsJSON(), nil
	case "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100":
		return f.commentsJSON()
	case "issue edit 14 --repo goxdra/goxsd9 --add-label needs-human":
		f.needsHuman = true
		return "", nil
	case "project field-list 1 --owner goxdra --format json":
		return `{"fields":[{"id":"status-id","name":"Status","options":[{"id":"backlog-id","name":"Backlog"},{"id":"ready-id","name":"Ready"}]}]}`, nil
	case "project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id":
		f.projectStatus = "Backlog"
		return "", nil
	case "issue comment 14 --repo goxdra/goxsd9 --body-file " + f.bodyPath:
		f.comments = append(f.comments, issueCommentAPI{Body: f.body, User: issueCommentUser(trustedActor)})
		return "https://github.com/goxdra/goxsd9/issues/14#issuecomment-1", nil
	default:
		return "", fmt.Errorf("unexpected gh command: %s", command)
	}
}

func (f *handoffFixture) issueStateJSON() string {
	f.issueReads++
	if f.raceIssue && f.issueReads == 2 {
		f.issueState = "closed"
	}
	labels := "[]"
	if f.needsHuman {
		labels = `[{"name":"needs-human"}]`
	}
	return fmt.Sprintf(`{"state":%q,"labels":%s}`, f.issueState, labels)
}

func (f *handoffFixture) projectItemsJSON() string {
	f.projectReads++
	if f.raceProject && f.projectReads == 2 {
		f.projectItemID = "item-raced"
	}
	if !f.projectMember {
		return `{"items":[],"totalCount":0}`
	}
	return fmt.Sprintf(`{"items":[{"content":{"number":14,"repository":%q,"type":%q},"id":%q,"status":%q}],"totalCount":1}`,
		repositoryKey, f.projectType, f.projectItemID, f.projectStatus)
}

func (f *handoffFixture) commentsJSON() (string, error) {
	encoded, err := json.Marshal(f.comments)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (f *handoffFixture) withBody(path, body string) error {
	f.bodyPath = path
	f.body = body
	return os.WriteFile(path, []byte(body), 0o600)
}

func issueCommentUser(login string) struct {
	Login string `json:"login"`
} {
	return struct {
		Login string `json:"login"`
	}{Login: login}
}

func TestHandoffOrdinaryModeRemainsCommentOnly(t *testing.T) {
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, "ordinary handoff\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: fixture.execute}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath}); err != nil {
		t.Fatalf("ordinary handoff: %v", err)
	}
	want := []string{
		"git rev-parse --show-toplevel",
		"gh issue comment 14 --repo goxdra/goxsd9 --body-file " + bodyPath,
	}
	if !reflect.DeepEqual(fixture.commands, want) {
		t.Fatalf("ordinary commands = %#v, want %#v", fixture.commands, want)
	}
	if fixture.needsHuman || fixture.projectStatus != "Ready" {
		t.Fatal("ordinary handoff changed escalation state")
	}
	if got, want := stdout.String(), "https://github.com/goxdra/goxsd9/issues/14#issuecomment-1\n"; got != want {
		t.Fatalf("ordinary output = %q, want %q", got, want)
	}
}

func TestHandoffNeedsHumanParsesAndUsesSafeOrder(t *testing.T) {
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: fixture.execute}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
		t.Fatalf("needs-human handoff: %v", err)
	}
	want := []string{
		"git rev-parse --show-toplevel",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh project item-list 1 --owner goxdra --format json --limit 500",
		"gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh project item-list 1 --owner goxdra --format json --limit 500",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh project item-list 1 --owner goxdra --format json --limit 500",
		"gh issue edit 14 --repo goxdra/goxsd9 --add-label needs-human",
		"gh project field-list 1 --owner goxdra --format json",
		"gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id",
		"gh issue comment 14 --repo goxdra/goxsd9 --body-file " + bodyPath,
	}
	if !reflect.DeepEqual(fixture.commands, want) {
		t.Fatalf("needs-human commands = %#v, want %#v", fixture.commands, want)
	}
	if !fixture.needsHuman || fixture.projectStatus != "Backlog" {
		t.Fatalf("transition state = needs-human %t, Project %s", fixture.needsHuman, fixture.projectStatus)
	}
}

func TestHandoffNeedsHumanRejectsInvalidBodyBeforeMutation(t *testing.T) {
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, " \n\t"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	application := app{executeCommand: fixture.execute}
	err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"})
	if err == nil || !strings.Contains(err.Error(), "body must not be empty") {
		t.Fatalf("invalid body error = %v", err)
	}
	if fixture.needsHuman || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
		t.Fatal("invalid body reached mutation")
	}
}

func TestHandoffNeedsHumanRejectsClosedOrMissingProjectBeforeMutation(t *testing.T) {
	tests := []struct {
		name          string
		issueState    string
		projectMember bool
		projectType   string
		projectItemID string
		want          string
		commands      int
	}{
		{name: "closed issue", issueState: "closed", projectMember: true, projectType: "Issue", projectItemID: "item-14", want: "is CLOSED", commands: 2},
		{name: "missing Project", issueState: "open", projectMember: false, projectType: "Issue", projectItemID: "item-14", want: "not in canonical Project", commands: 3},
		{name: "PullRequest Project item", issueState: "open", projectMember: true, projectType: "PullRequest", projectItemID: "item-14", want: "not in canonical Project", commands: 3},
		{name: "missing Project item ID", issueState: "open", projectMember: true, projectType: "Issue", projectItemID: "", want: "not in canonical Project", commands: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			fixture.issueState = test.issueState
			fixture.projectMember = test.projectMember
			fixture.projectType = test.projectType
			fixture.projectItemID = test.projectItemID
			bodyPath := filepath.Join(t.TempDir(), "handoff.md")
			if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
				t.Fatalf("write body: %v", err)
			}
			application := app{executeCommand: fixture.execute}
			err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("handoff error = %v, want %q", err, test.want)
			}
			if len(fixture.commands) != test.commands {
				t.Fatalf("preflight commands = %#v, want %d commands", fixture.commands, test.commands)
			}
			if fixture.needsHuman || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
				t.Fatal("preflight rejection reached mutation")
			}
		})
	}
}

func TestHandoffNeedsHumanRejectsChangedPreMutationProof(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*handoffFixture)
		want     string
		commands int
	}{
		{name: "issue closed", mutate: func(fixture *handoffFixture) { fixture.raceIssue = true }, want: "issue is CLOSED", commands: 5},
		{name: "Project identity changed", mutate: func(fixture *handoffFixture) { fixture.raceProject = true }, want: "Project identity differs", commands: 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			test.mutate(fixture)
			bodyPath := filepath.Join(t.TempDir(), "handoff.md")
			if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
				t.Fatalf("write body: %v", err)
			}
			application := app{stdout: new(bytes.Buffer), executeCommand: fixture.execute}
			err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("race handoff error = %v, want %q", err, test.want)
			}
			if len(fixture.commands) != test.commands {
				t.Fatalf("race commands = %#v, want %d commands", fixture.commands, test.commands)
			}
			if fixture.needsHuman || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
				t.Fatal("changed proof reached mutation")
			}
		})
	}
}

func TestHandoffNeedsHumanRetryConvergesAfterEvidenceFailure(t *testing.T) {
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fixture.failCommand = "gh issue comment 14 --repo goxdra/goxsd9 --body-file " + bodyPath
	application := app{executeCommand: fixture.execute, stdout: new(bytes.Buffer)}
	err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"})
	if err == nil || !strings.Contains(err.Error(), "evidence comment phase incomplete") || !strings.Contains(err.Error(), "retry") {
		t.Fatalf("partial handoff error = %v, want retryable evidence phase", err)
	}
	if !fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 0 {
		t.Fatal("partial handoff did not preserve completed label and Project phases")
	}
	fixture.failCommand = ""
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
		t.Fatalf("retry handoff: %v", err)
	}
	if len(fixture.comments) != 1 {
		t.Fatalf("retry comments = %d, want one evidence comment", len(fixture.comments))
	}
}

//nolint:gocognit // Table-driven cases keep partial-state convergence coverage aligned.
func TestHandoffNeedsHumanConvergesPartialStateWithoutRepeatingCompletedWrites(t *testing.T) {
	tests := []struct {
		name          string
		needsHuman    bool
		projectStatus string
		trustedBody   bool
		wantLabel     bool
		wantProject   bool
		wantComment   bool
	}{
		{name: "label-only", needsHuman: true, projectStatus: "Ready", wantProject: true, wantComment: true},
		{name: "Project-only", needsHuman: false, projectStatus: "Backlog", wantLabel: true, wantComment: true},
		{name: "evidence-only", needsHuman: true, projectStatus: "Backlog", trustedBody: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			fixture.needsHuman = test.needsHuman
			fixture.projectStatus = test.projectStatus
			bodyPath := filepath.Join(t.TempDir(), "handoff.md")
			if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
				t.Fatalf("write body: %v", err)
			}
			if test.trustedBody {
				fixture.comments = []issueCommentAPI{{Body: fixture.body, User: issueCommentUser(trustedActor)}}
			}
			application := app{executeCommand: fixture.execute, stdout: new(bytes.Buffer)}
			if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
				t.Fatalf("partial handoff: %v", err)
			}
			joined := strings.Join(fixture.commands, "\n")
			if got := strings.Contains(joined, "gh issue edit 14 "); got != test.wantLabel {
				t.Fatalf("label write present = %t, want %t; commands=%v", got, test.wantLabel, fixture.commands)
			}
			if got := strings.Contains(joined, "gh project item-edit "); got != test.wantProject {
				t.Fatalf("Project write present = %t, want %t; commands=%v", got, test.wantProject, fixture.commands)
			}
			if got := strings.Contains(joined, "gh issue comment 14 "); got != test.wantComment {
				t.Fatalf("evidence write present = %t, want %t; commands=%v", got, test.wantComment, fixture.commands)
			}
		})
	}
}

func TestHandoffNeedsHumanConvergesAmbiguousMutationResponsesByReread(t *testing.T) {
	tests := []struct {
		name      string
		failAfter string
	}{
		{name: "label", failAfter: "gh issue edit 14 --repo goxdra/goxsd9 --add-label needs-human"},
		{name: "Project", failAfter: "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id"},
		{name: "evidence", failAfter: "gh issue comment 14 --repo goxdra/goxsd9 --body-file BODY"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandoffFixture()
			bodyPath := filepath.Join(t.TempDir(), "handoff.md")
			if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
				t.Fatalf("write body: %v", err)
			}
			fixture.failAfterMutation = strings.Replace(test.failAfter, "BODY", bodyPath, 1)
			application := app{executeCommand: fixture.execute, stdout: new(bytes.Buffer)}
			if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
				t.Fatalf("ambiguous %s handoff: %v", test.name, err)
			}
			if !fixture.needsHuman || fixture.projectStatus != "Backlog" || len(fixture.comments) != 1 {
				t.Fatalf("ambiguous %s state = label %t, Project %s, comments %d", test.name, fixture.needsHuman, fixture.projectStatus, len(fixture.comments))
			}
		})
	}
}

func TestHandoffNeedsHumanDeduplicatesExactTrustedEvidence(t *testing.T) {
	for _, author := range []string{owner, trustedActor} {
		t.Run(author, func(t *testing.T) { runTrustedHandoffDedup(t, author) })
	}
}

func runTrustedHandoffDedup(t *testing.T, author string) {
	t.Helper()
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fixture.comments = []issueCommentAPI{{Body: fixture.body, User: issueCommentUser(author)}}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: fixture.execute}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
		t.Fatalf("deduplicated handoff: %v", err)
	}
	if got, want := len(fixture.comments), 1; got != want {
		t.Fatalf("comments = %d, want %d", got, want)
	}
	if !fixture.needsHuman || fixture.projectStatus != "Backlog" {
		t.Fatal("deduplicated handoff did not converge label and Project")
	}
	if strings.Contains(strings.Join(fixture.commands, "\n"), "issue comment 14") {
		t.Fatal("exact trusted evidence was posted twice")
	}
}

func TestHandoffNeedsHumanDoesNotInferFromUntrustedOrDifferentComments(t *testing.T) {
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, "terminal evidence\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fixture.comments = []issueCommentAPI{{Body: fixture.body, User: issueCommentUser("other-user")}}
	application := app{executeCommand: fixture.execute, stdout: new(bytes.Buffer)}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
		t.Fatalf("untrusted-comment handoff: %v", err)
	}
	if len(fixture.comments) != 2 {
		t.Fatalf("comments = %d, want untrusted exact body plus new evidence", len(fixture.comments))
	}
}
