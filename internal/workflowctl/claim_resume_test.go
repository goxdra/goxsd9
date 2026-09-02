package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClaimResumeTerminalEvidenceParser(t *testing.T) {
	worktree := "/worktrees/issue-14-run-proof"
	valid := fmt.Sprintf("## Blocker\n\nIssue #14 was claimed in %s.\n\n## Evidence\n\n- Issue #14 remained OPEN in the Project.\n- The claim worktree was clean at the final read.\n- No implementation, tests, commit, push, PR, or evaluation record was made.\n\n## Decisions and risks\n\n- Preserve the claim.\n\n## Next action\n\nResume after the blocker is cleared.\n", worktree)
	if err := validateTerminalClaimHandoffBody(valid, 14); err != nil {
		t.Fatalf("valid terminal evidence: %v", err)
	}
	workflowOnly := fmt.Sprintf("## Blocker\n\nIssue #14 was claimed successfully.\nNo source or test files were changed, no checks, push, PR, challenge, or evaluation record was made.\n\n## Evidence\n\n- Issue #14 remained OPEN in the Roadmap Project.\n- Claim worktree preserved at %s.\n- The claimed branch currently contains only the generated workflow claim\ncommit.\n\n## Risk and next action\n\nPreserve the branch and retry.\n", worktree)
	if err := validateTerminalClaimHandoffBody(workflowOnly, 14); err != nil {
		t.Fatalf("workflow-only terminal evidence: %v", err)
	}
	exact287 := strings.Join([]string{
		"## Blocker",
		"",
		"Issue #287 was claimed successfully, but the develop protocol is blocked at",
		"its mandatory fresh read-only Scribe/Mason consultation barrier. The Scribe",
		"and Mason role agents were spawned with the exact configured roles and",
		"read-only scope. Seven bounded waits returned `timed_out: true` with no final",
		"handoff or error status. One bounded follow-up to each existing agent was",
		"accepted, and the next bounded wait also timed out. No replacement agents",
		"were spawned and no scope was widened.",
		"",
		"Without those consultations, the root cannot assign Smith the required",
		"implementation contract under the develop protocol. No source or test files",
		"were changed, no checks, push, PR, challenge, or evaluation record was made.",
		"",
		"## Evidence",
		"",
		"- `go tool workflowctl doctor` passed: coordination `main` was clean and equal",
		"  to fetched `origin/main`, with recursive pins and required tools ready.",
		"- `go tool workflowctl pick` selected #287, “Model inline simple types on",
		"  global attributes”.",
		"- Claim worktree preserved at",
		"  `/home/paseouser/workspace/goxsd9-worktrees/issue-287-run-36de80f997095582`.",
		"- Scribe agent: `01a05b22-06da-7883-8245-16de2e761011`; Mason agent:",
		"  `01a05b22-0664-7881-aca3-3489d7ddc2f4`. Follow-up submissions were",
		"  `01a05b2a-8aa3-7cd3-921f-84566ec6d35a` and",
		"  `01a05b2a-8ab6-79c3-aa73-139fcbf7b0bd`.",
		"- Issue context and required `README.md`, `ARCHITECTURE.md`, `PLAN.md`, and",
		"  agent role configurations were read in the preserved worktree.",
		"",
		"## Risk and next action",
		"",
		"The claimed branch currently contains only the generated workflow claim",
		"commit. Proceeding without specification and architecture handoffs could",
		"mis-state XSD 1.0/1.1 behavior and violate the repository phase invariants.",
		"Keep the worktree and claim artifacts intact; resume issue #287 when the",
		"configured consultation agents can return their bounded handoffs.",
	}, "\n") + "\n"
	if err := validateTerminalClaimHandoffBody(exact287, 287); err != nil {
		t.Fatalf("exact #287 terminal evidence: %v", err)
	}
	malformed := []struct {
		name string
		body string
	}{
		{name: "missing final LF", body: strings.TrimSuffix(valid, "\n")},
		{name: "trailing space", body: strings.Replace(valid, "the Project.\n", "the Project. \n", 1)},
		{name: "wrong heading", body: strings.Replace(valid, "## Next action", "## Next steps", 1)},
		{name: "no PR proof", body: strings.Replace(valid, ", PR,", ", review,", 1)},
		{name: "unclean worktree", body: strings.Replace(valid, "worktree was clean", "worktree was unclean", 1)},
	}
	for _, test := range malformed {
		t.Run(test.name, func(t *testing.T) {
			if err := validateTerminalClaimHandoffBody(test.body, 14); err == nil {
				t.Fatal("malformed terminal evidence unexpectedly accepted")
			}
		})
	}
}

//nolint:funlen,gocognit // The integration cases cover each mutation boundary and its convergence response.
func TestClaimResumeInjectedIntegration(t *testing.T) {
	t.Run("dry run has zero mutation", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		var output bytes.Buffer
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: &output}
		if err := application.run(claimResumeArgs(fixture, true)); err != nil {
			t.Fatalf("claim resume dry-run: %v", err)
		}
		if backend.mutations != 0 {
			t.Fatalf("dry-run mutations = %d, want zero", backend.mutations)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
			t.Fatalf("dry-run moved local head from %s to %s", fixture.expected, got)
		}
		if !strings.Contains(output.String(), "no mutation performed") {
			t.Fatalf("dry-run output = %q", output.String())
		}
	})

	t.Run("renews and reconciles issue", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("claim resume: %v", err)
		}
		assertClaimResumeRenewed(t, fixture, backend)
	})

	t.Run("completed recovery is idempotent", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("first claim resume: %v", err)
		}
		head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
		mutations := backend.mutations
		pushes := countClaimResumePushes(backend.calls)
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("second claim resume: %v", err)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != head {
			t.Fatalf("idempotent rerun moved local head from %s to %s", head, got)
		}
		if backend.mutations != mutations || countClaimResumePushes(backend.calls) != pushes {
			t.Fatalf("idempotent rerun mutations/pushes = %d/%d, want %d/%d", backend.mutations, countClaimResumePushes(backend.calls), mutations, pushes)
		}
	})

	t.Run("ambiguous push response converges", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		backend.ambiguousPush = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("claim resume after ambiguous push: %v", err)
		}
		assertClaimResumeRenewed(t, fixture, backend)
		if got := countClaimResumePushes(backend.calls); got != 1 {
			t.Fatalf("push count = %d, want one", got)
		}
	})

	t.Run("ambiguous label and Project responses converge", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		backend.ambiguousLabel = true
		backend.ambiguousProject = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("claim resume after ambiguous GitHub responses: %v", err)
		}
		assertClaimResumeRenewed(t, fixture, backend)
	})

	t.Run("transient label failure remains retryable", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		labelErr := errors.New("simulated label transport failure")
		backend.labelFailure = labelErr
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		err := application.run(claimResumeArgs(fixture, false))
		if err == nil || operationDispositionOf(err) != operationDispositionRetryable {
			t.Fatalf("claim resume error = %v, want retryable failure", err)
		}
		if !errors.Is(err, labelErr) {
			t.Fatalf("claim resume error = %v, want original label failure", err)
		}
		if backend.needsHuman != true || backend.projectStatus != "Backlog" {
			t.Fatalf("state after retryable label failure = needs-human %t, Project %s", backend.needsHuman, backend.projectStatus)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got == fixture.expected {
			t.Fatal("retryable label failure lost the verified renewal")
		}
	})
}

func TestClaimResumeRejectsWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, *claimResumeFixture, *claimResumeBackend)
		want   string
		status bool
	}{
		{name: "dirty worktree", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			writeFixtureFile(t, fixture.worktree, "dirty", "keep\n")
		}, want: "worktree", status: true},
		{name: "missing terminal evidence", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.comments = backend.comments[:1]
		}, want: "terminal handoff", status: true},
		{name: "untrusted terminal evidence", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.comments[1].User.Login = owner
		}, want: "trusted API actor", status: true},
		{name: "locked worktree", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			runGitTest(t, fixture.primary, "worktree", "lock", fixture.worktree)
		}, want: "current worktree registration", status: true},
		{name: "duplicate worktree", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			duplicate := filepath.Join(t.TempDir(), "issue-14-run-other")
			runGitTest(t, fixture.primary, "worktree", "add", "-b", "agent/issue-14-run-other", duplicate, fixture.expected)
		}, want: "duplicate/orphan", status: true},
		{name: "malformed remote ref listing", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.malformedRemoteRefs = true
		}, want: "malformed entry", status: true},
		{name: "open PR", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.openPR = true
		}, want: "open PR", status: true},
		{name: "moved fixed ref", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			moved := createResumeTestCommit(t, fixture.primary, fixture.expected, "moved artifact")
			runGitTest(t, fixture.primary, "push", "--force", "origin", moved+":refs/heads/agent/issue-14")
		}, want: "renewal child", status: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newClaimResumeFixture(t)
			backend := newClaimResumeBackend(t, fixture)
			test.setup(t, &fixture, backend)
			before := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(claimResumeArgs(fixture, true))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("claim resume error = %v, want %q", err, test.want)
			}
			if backend.mutations != 0 {
				t.Fatalf("refusal mutations = %d, want zero", backend.mutations)
			}
			if after := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); after != before {
				t.Fatalf("refusal moved local head from %s to %s", before, after)
			}
			if got := backend.needsHuman; got != test.status {
				t.Fatalf("needs-human = %t, want %t", got, test.status)
			}
		})
	}
}

func TestClaimResumeRequiresAcknowledgementAndExactFlags(t *testing.T) {
	fixture := newClaimResumeFixture(t)
	for _, args := range [][]string{
		{"claim", "resume", "14", "--expected-head", fixture.expected, "--run-id", fixture.runID, "--handoff-comment", "2"},
		{"claim", "resume", "14", "--expected-head", "short", "--run-id", fixture.runID, "--handoff-comment", "2", "--acknowledge-needs-human"},
		{"claim", "resume", "14", "--expected-head", fixture.expected, "--run-id", fixture.runID, "--handoff-comment", "2x", "--acknowledge-needs-human"},
	} {
		if err := (app{ctx: context.Background(), stdout: io.Discard}).run(args); err == nil {
			t.Fatalf("run(%q) unexpectedly succeeded", args)
		}
	}
}

type claimResumeFixture struct {
	baseRepositoryFixture
	expected string
	runID    string
	worktree string
	handoff  int64
	lease    time.Time
}

func newClaimResumeFixture(t *testing.T) claimResumeFixture {
	t.Helper()
	base := newBaseRepositoryFixture(t, false)
	runID := "run-resume-test"
	lease := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	parent := runGitTest(t, base.primary, "rev-parse", "HEAD")
	expected := createResumeTestCommit(t, base.primary, parent, claimMessage(14, runID, lease))
	runGitTest(t, base.primary, "push", "origin", expected+":refs/heads/agent/issue-14")
	worktree := filepath.Join(t.TempDir(), "issue-14-run-resume-test")
	runGitTest(t, base.primary, "worktree", "add", "-b", "agent/issue-14-"+runID, worktree, expected)
	return claimResumeFixture{baseRepositoryFixture: base, expected: expected, runID: runID, worktree: worktree, handoff: 2, lease: lease}
}

func claimResumeArgs(fixture claimResumeFixture, dryRun bool) []string {
	args := []string{"claim", "resume", "14", "--expected-head", fixture.expected, "--run-id", fixture.runID,
		"--handoff-comment", strconv.FormatInt(fixture.handoff, 10), "--acknowledge-needs-human"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	return args
}

type claimResumeBackend struct {
	t                   *testing.T
	fixture             claimResumeFixture
	comments            []issueCommentAPI
	needsHuman          bool
	projectStatus       string
	openPR              bool
	ambiguousPush       bool
	ambiguousLabel      bool
	ambiguousProject    bool
	malformedRemoteRefs bool
	labelFailure        error
	mutations           int
	calls               []string
}

func newClaimResumeBackend(t *testing.T, fixture claimResumeFixture) *claimResumeBackend {
	t.Helper()
	lease := fixture.lease
	claim := issueCommentAPI{ID: 1, Body: fmt.Sprintf("Claim acquired.\n\n- Branch: `%s`\n- Local branch: `%s`\n- Worktree: `%s`\n- Run: `%s`\n- Lease until: `%s`\n",
		claimBranch(14), claimLocalBranch(14, fixture.runID), fixture.worktree, fixture.runID, lease.Format(time.RFC3339))}
	claim.User.Login = trustedActor
	terminal := issueCommentAPI{ID: fixture.handoff, Body: fmt.Sprintf("## Blocker\n\nIssue #14 was claimed in %s.\n\n## Evidence\n\n- Issue #14 remained OPEN in the Roadmap Project.\n- The claim worktree was clean at the final read: %s.\n- No implementation, tests, documentation, commit, push, PR, or evaluation record was made.\n\n## Decisions and risks\n\n- The generated claim is preserved.\n\n## Next action\n\nResume the issue after the blocker is cleared.\n", fixture.worktree, fixture.worktree)}
	terminal.User.Login = trustedActor
	return &claimResumeBackend{t: t, fixture: fixture, comments: []issueCommentAPI{claim, terminal}, needsHuman: true, projectStatus: "Backlog"}
}

func (b *claimResumeBackend) execute(dir string, input io.Reader, name string, args ...string) (string, error) {
	if dir == "" {
		dir = b.fixture.worktree
	}
	call := name + " " + strings.Join(args, " ")
	b.calls = append(b.calls, call)
	if name == "gh" {
		return b.executeGH(args...)
	}
	if b.malformedRemoteRefs && name == "git" && len(args) > 0 && args[0] == "ls-remote" {
		return "not-a-ref", nil
	}
	if name == "git" && len(args) > 0 && (args[0] == "commit-tree" || args[0] == "update-ref" || args[0] == "push") {
		b.mutations++
	}
	// #nosec G204 -- the injected executor runs only workflowctl-generated test commands.
	command := exec.CommandContext(context.Background(), name, args...)
	command.Dir = dir
	command.Stdin = input
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s: %w: %s", call, err, strings.TrimSpace(string(output)))
	}
	if b.ambiguousPush && name == "git" && len(args) > 0 && args[0] == "push" {
		b.ambiguousPush = false
		return "", errors.New("simulated lost push response")
	}
	return strings.TrimSpace(string(output)), nil
}

func (b *claimResumeBackend) executeGH(args ...string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case joined == "api repos/goxdra/goxsd9/issues/14":
		labels := "[]"
		if b.needsHuman {
			labels = `[{"name":"needs-human"}]`
		}
		return `{"state":"open","labels":` + labels + `}`, nil
	case joined == "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100":
		data, err := json.Marshal(b.comments)
		return string(data), err
	case joined == "pr list --repo goxdra/goxsd9 --head agent/issue-14 --state open --json number":
		if b.openPR {
			return `[{"number":55}]`, nil
		}
		return `[]`, nil
	case strings.HasPrefix(joined, "issue edit 14 "):
		b.mutations++
		if b.labelFailure != nil {
			err := b.labelFailure
			b.labelFailure = nil
			return "", err
		}
		b.needsHuman = false
		if b.ambiguousLabel {
			b.ambiguousLabel = false
			return "", errors.New("simulated lost label response")
		}
		return "", nil
	case strings.HasPrefix(joined, "project item-list "):
		return fmt.Sprintf(`{"items":[{"id":"item-14","status":%q,"content":{"number":14,"repository":"goxdra/goxsd9","type":"Issue"}}],"totalCount":1}`, b.projectStatus), nil
	case strings.HasPrefix(joined, "project field-list "):
		return `{"fields":[{"id":"status-field","name":"Status","options":[{"id":"backlog-id","name":"Backlog"},{"id":"picked-id","name":"Picked"}]}]}`, nil
	case strings.Contains(joined, "project item-edit"):
		b.mutations++
		b.projectStatus = "Picked"
		if b.ambiguousProject {
			b.ambiguousProject = false
			return "", errors.New("simulated lost Project response")
		}
		return "", nil
	default:
		return "", fmt.Errorf("unexpected gh command: %s", joined)
	}
}

func assertClaimResumeRenewed(t *testing.T, fixture claimResumeFixture, backend *claimResumeBackend) {
	t.Helper()
	head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if head == fixture.expected {
		t.Fatal("claim resume did not create a renewal child")
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^"); got != fixture.expected {
		t.Fatalf("renewal parent = %s, want %s", got, fixture.expected)
	}
	if got, want := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}"), runGitTest(t, fixture.worktree, "rev-parse", "HEAD^^{tree}"); got != want {
		t.Fatalf("renewal tree = %s, want %s", got, want)
	}
	if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14"); !strings.HasPrefix(got, head+"\t") {
		t.Fatalf("remote fixed branch = %q, want %s", got, head)
	}
	if backend.needsHuman || backend.projectStatus != "Picked" {
		t.Fatalf("reconciled state = needs-human %t, Project %s", backend.needsHuman, backend.projectStatus)
	}
}

func countClaimResumePushes(calls []string) int {
	count := 0
	for _, call := range calls {
		if strings.HasPrefix(call, "git push ") {
			count++
		}
	}
	return count
}

func TestClaimResumeMetadataAndClaimCommentStayExact(t *testing.T) {
	lease := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	comment, err := parseClaimAcquiredComment(fmt.Sprintf("Claim acquired.\n\n- Branch: `agent/issue-14`\n- Local branch: `agent/issue-14-run-proof`\n- Worktree: `/tmp/proof`\n- Run: `run-proof`\n- Lease until: `%s`\n", lease.Format(time.RFC3339)))
	if err != nil {
		t.Fatalf("parse generated claim comment: %v", err)
	}
	if comment.runID != "run-proof" || !comment.lease.Equal(lease) {
		t.Fatalf("parsed claim comment = %#v", comment)
	}
	if _, err := parseClaimAcquiredComment(strings.Replace(commentBodyForTest(lease), "Claim acquired.", "Claim acquired", 1)); err == nil {
		t.Fatal("malformed generated claim comment unexpectedly accepted")
	}
}

func commentBodyForTest(lease time.Time) string {
	return fmt.Sprintf("Claim acquired.\n\n- Branch: `agent/issue-14`\n- Local branch: `agent/issue-14-run-proof`\n- Worktree: `/tmp/proof`\n- Run: `run-proof`\n- Lease until: `%s`\n", lease.Format(time.RFC3339))
}
