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
	"slices"
	"strings"
	"testing"
	"time"
)

type handoffFixture struct {
	issueState        string
	needsHuman        bool
	projectStatus     string
	projectMember     bool
	projectItemID     string
	projectType       string
	issueReads        int
	issueAtRead       func(int)
	projectReads      int
	projectPage       func(int, string) (string, error)
	raceIssue         bool
	raceProject       bool
	bodyPath          string
	body              string
	postedBody        string
	commentPath       string
	beforeComment     func()
	comments          []issueCommentAPI
	commands          []string
	failCommand       string
	failCause         error
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
		if f.failCause != nil {
			return "", f.failCause
		}
		return "", errors.New("simulated handoff failure")
	}
	if command == f.failAfterMutation {
		switch command {
		case "gh issue edit 14 --repo goxdra/goxsd9 --add-label needs-human":
			f.needsHuman = true
		case "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id":
			f.projectStatus = "Backlog"
		case "gh issue comment 14 --repo goxdra/goxsd9 --body-file " + f.bodyPath:
			f.comments = append(f.comments, issueCommentAPI{ID: int64(len(f.comments) + 1), Body: f.body, CreatedAt: time.Now().UTC(), User: issueCommentUser(trustedActor)})
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
		return "", errors.New("whole-Project enumeration is forbidden in needs-human handoff")
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
	default:
		const commentPrefix = "issue comment 14 --repo goxdra/goxsd9 --body-file "
		if strings.HasPrefix(command, commentPrefix) {
			return f.postComment(strings.TrimPrefix(command, commentPrefix))
		}
		if strings.HasPrefix(command, "api graphql -f query="+claimResumeProjectIssueQuery) {
			f.projectReads++
			if f.raceProject && f.projectReads == 2 {
				f.projectItemID = "item-raced"
			}
			if f.projectPage != nil {
				return f.projectPage(f.projectReads, claimResumeProjectAfter(strings.Fields(command)))
			}
			return f.projectItemsJSON(), nil
		}
		return "", fmt.Errorf("unexpected gh command: %s", command)
	}
}

func (f *handoffFixture) postComment(path string) (string, error) {
	if f.beforeComment != nil {
		f.beforeComment()
	}
	f.commentPath = path
	// #nosec G304 -- the test fixture reads the explicit body-file command argument.
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	f.postedBody = string(body)
	f.comments = append(f.comments, issueCommentAPI{ID: int64(len(f.comments) + 1), Body: f.postedBody, CreatedAt: time.Now().UTC(), User: issueCommentUser(trustedActor)})
	return "https://github.com/goxdra/goxsd9/issues/14#issuecomment-1", nil
}

func (f *handoffFixture) issueStateJSON() string {
	f.issueReads++
	if f.issueAtRead != nil {
		f.issueAtRead(f.issueReads)
	}
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
	if !f.projectMember {
		return projectIssueTestPage(14, 0, "", false, "")
	}
	page := claimResumeProjectPageJSON(14, f.projectStatus)
	page = strings.Replace(page, `"id":"item-14"`, fmt.Sprintf(`"id":%q`, f.projectItemID), 1)
	if f.projectType != "Issue" {
		page = strings.Replace(page, `"type":"ISSUE"`, `"type":"PULL_REQUEST"`, 1)
	}
	return page
}

func handoffCommandShapes(commands []string) []string {
	shapes := make([]string, len(commands))
	for index, command := range commands {
		shapes[index] = command
		if strings.HasPrefix(command, "gh api graphql -f query="+claimResumeProjectIssueQuery) {
			shapes[index] = "gh api graphql issue #14"
		}
	}
	return shapes
}

func (f *handoffFixture) commentsJSON() (string, error) {
	if f.comments == nil {
		return "[]", nil
	}
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
		"gh api graphql issue #14",
		"gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh api graphql issue #14",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh api graphql issue #14",
		"gh issue edit 14 --repo goxdra/goxsd9 --add-label needs-human",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh api graphql issue #14",
		"gh project field-list 1 --owner goxdra --format json",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh api graphql issue #14",
		"gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-14 --field-id status-id --single-select-option-id backlog-id",
		"gh api repos/goxdra/goxsd9/issues/14",
		"gh api graphql issue #14",
		"gh issue comment 14 --repo goxdra/goxsd9 --body-file " + bodyPath,
	}
	if got := handoffCommandShapes(fixture.commands); !slices.Equal(got, want) {
		t.Fatalf("needs-human commands = %#v, want %#v", got, want)
	}
	if !fixture.needsHuman || fixture.projectStatus != "Backlog" {
		t.Fatalf("transition state = needs-human %t, Project %s", fixture.needsHuman, fixture.projectStatus)
	}
}

//nolint:gocognit // One command boundary checks posted bytes, cleanup, and mutation order.
func TestHandoffTerminalNoPRPostsValidatedSnapshot(t *testing.T) {
	body := claimResumeHandoffBodyWithNoPRForm(14, "/repo/claim", terminalGenericFormTwo)
	if err := validateTerminalClaimHandoffBody(body, 14); err != nil {
		t.Fatalf("recovery parser rejects accepted handoff: %v", err)
	}
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fixture.beforeComment = func() {
		if err := os.WriteFile(bodyPath, []byte("substituted body\n"), 0o600); err != nil {
			t.Fatalf("change caller body: %v", err)
		}
	}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: fixture.execute}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human", "--terminal-no-pr"}); err != nil {
		t.Fatalf("terminal handoff: %v", err)
	}
	if fixture.postedBody != body || len(fixture.comments) != 1 || fixture.comments[0].Body != body {
		t.Fatalf("posted body differs from validated body: %q", fixture.postedBody)
	}
	if err := validateTerminalClaimHandoffBody(fixture.comments[0].Body, 14); err != nil {
		t.Fatalf("posted body fails recovery parser: %v", err)
	}
	if fixture.commentPath == bodyPath || fixture.commentPath == "" {
		t.Fatalf("comment used caller body path %q", fixture.commentPath)
	}
	if _, err := os.Stat(fixture.commentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot was not removed: %v", err)
	}
	if !fixture.needsHuman || fixture.projectStatus != "Backlog" {
		t.Fatalf("transition state = needs-human %t, Project %s", fixture.needsHuman, fixture.projectStatus)
	}
	writes := make([]string, 0, 3)
	for _, command := range fixture.commands {
		if strings.HasPrefix(command, "gh issue edit ") || strings.HasPrefix(command, "gh project item-edit ") || strings.HasPrefix(command, "gh issue comment ") {
			writes = append(writes, command)
		}
	}
	if len(writes) != 3 || !strings.HasPrefix(writes[0], "gh issue edit ") || !strings.HasPrefix(writes[1], "gh project item-edit ") || !strings.HasPrefix(writes[2], "gh issue comment ") {
		t.Fatalf("terminal handoff mutation order = %#v", writes)
	}
	if !strings.Contains(stdout.String(), "issuecomment-1") {
		t.Fatalf("missing comment URL: %q", stdout.String())
	}
}

func TestHandoffTerminalNoPRDeduplicatesTrustedComment(t *testing.T) {
	body := claimResumeHandoffBodyWithNoPRForm(14, "/repo/claim", terminalGenericFormTwo)
	fixture := newHandoffFixture()
	fixture.comments = []issueCommentAPI{{ID: 17, Body: body, CreatedAt: time.Now().UTC(), User: issueCommentUser(trustedActor)}}
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: fixture.execute}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human", "--terminal-no-pr"}); err != nil {
		t.Fatalf("retry terminal handoff: %v", err)
	}
	if len(fixture.comments) != 1 || fixture.commentPath != "" {
		t.Fatalf("retry posted duplicate comment: %#v", fixture.comments)
	}
	if !fixture.needsHuman || fixture.projectStatus != "Backlog" || !strings.Contains(stdout.String(), "already recorded") {
		t.Fatalf("retry state = needs-human %t, Project %s, output %q", fixture.needsHuman, fixture.projectStatus, stdout.String())
	}
}

//nolint:gocognit // Malformed forms share one pre-mutation refusal boundary.
func TestHandoffTerminalNoPRRejectsBeforeRemoteMutation(t *testing.T) {
	valid := claimResumeHandoffBodyWithNoPRForm(14, "/repo/claim", terminalGenericFormTwo)
	tests := []struct {
		name string
		body string
	}{
		{name: "extra workflow path assertion", body: valid + "Run `go tool workflowctl check` before the PR workflow.\n"},
		{name: "extra PR assertion", body: valid + "No PR was published.\n"},
		{name: "malformed heading", body: strings.Replace(valid, "## Next action", "## Next steps", 1)},
		{name: "wrong issue mention", body: strings.Replace(valid, "Issue #14", "Issue #904", 1)},
		{name: "trailing whitespace", body: strings.Replace(valid, "## Blocker\n", "## Blocker \n", 1)},
		{name: "missing newline", body: strings.TrimSuffix(valid, "\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parserErr := validateTerminalClaimHandoffBody(test.body, 14)
			if parserErr == nil {
				t.Fatal("test body unexpectedly accepted by recovery parser")
			}
			fixture := newHandoffFixture()
			bodyPath := filepath.Join(t.TempDir(), "handoff.md")
			if err := fixture.withBody(bodyPath, test.body); err != nil {
				t.Fatalf("write body: %v", err)
			}
			application := app{executeCommand: fixture.execute}
			err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human", "--terminal-no-pr"})
			if err == nil || !strings.Contains(err.Error(), parserErr.Error()) {
				t.Fatalf("handoff error = %v, want parser cause %v", err, parserErr)
			}
			if len(fixture.commands) != 1 || fixture.commands[0] != "git rev-parse --show-toplevel" {
				t.Fatalf("invalid terminal body reached remote command: %#v", fixture.commands)
			}
			if fixture.needsHuman || fixture.projectStatus != "Ready" || len(fixture.comments) != 0 {
				t.Fatal("invalid terminal body changed issue state")
			}
		})
	}
}

func TestHandoffTerminalNoPRRequiresNeedsHuman(t *testing.T) {
	fixture := newHandoffFixture()
	bodyPath := filepath.Join(t.TempDir(), "handoff.md")
	if err := fixture.withBody(bodyPath, "ordinary handoff\n"); err != nil {
		t.Fatalf("write body: %v", err)
	}
	application := app{executeCommand: fixture.execute}
	err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--terminal-no-pr"})
	if err == nil || !strings.Contains(err.Error(), "--terminal-no-pr requires --needs-human") {
		t.Fatalf("mode error = %v", err)
	}
	if len(fixture.commands) != 0 {
		t.Fatalf("invalid mode ran commands: %#v", fixture.commands)
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
		{name: "missing Project", issueState: "open", projectMember: false, projectType: "Issue", projectItemID: "item-14", want: "canonical item is missing", commands: 3},
		{name: "PullRequest Project item", issueState: "open", projectMember: true, projectType: "PullRequest", projectItemID: "item-14", want: "Project proof", commands: 3},
		{name: "missing Project item ID", issueState: "open", projectMember: true, projectType: "Issue", projectItemID: "", want: "Project proof", commands: 3},
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
		{name: "Project identity changed", mutate: func(fixture *handoffFixture) { fixture.raceProject = true }, want: "Project identity or status differs", commands: 6},
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
				fixture.comments = []issueCommentAPI{{ID: 1, Body: fixture.body, CreatedAt: time.Unix(1, 0).UTC(), User: issueCommentUser(trustedActor)}}
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
	fixture.comments = []issueCommentAPI{{ID: 1, Body: fixture.body, CreatedAt: time.Unix(1, 0).UTC(), User: issueCommentUser(author)}}
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
	fixture.comments = []issueCommentAPI{{ID: 1, Body: fixture.body, CreatedAt: time.Unix(1, 0).UTC(), User: issueCommentUser("other-user")}}
	application := app{executeCommand: fixture.execute, stdout: new(bytes.Buffer)}
	if err := application.runHandoff([]string{"14", "--body-file", bodyPath, "--needs-human"}); err != nil {
		t.Fatalf("untrusted-comment handoff: %v", err)
	}
	if len(fixture.comments) != 2 {
		t.Fatalf("comments = %d, want untrusted exact body plus new evidence", len(fixture.comments))
	}
}
