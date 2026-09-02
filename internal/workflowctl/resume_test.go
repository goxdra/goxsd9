package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPRResumeRequiresAcknowledgementAndExpectedHead(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "acknowledgement", args: []string{"pr", "resume", "14", "--expected-head", "abc"}, want: "--acknowledge-needs-human"},
		{name: "expected head", args: []string{"pr", "resume", "14", "--acknowledge-needs-human"}, want: "usage: workflowctl pr resume"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			application := app{ctx: context.Background(), stdout: &output, stderr: &output}
			err := application.run(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run(%q) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

//nolint:gocognit,funlen // The independent integration subtests share one real-Git harness.
func TestPRResumeInjectedIntegration(t *testing.T) {
	t.Run("dry run has zero mutation", func(t *testing.T) {
		fixture := newResumeFixture(t)
		backend := newResumeBackend(t, fixture)
		var output bytes.Buffer
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: &output}
		if err := application.run([]string{"pr", "resume", "14", "--expected-head", fixture.expected,
			"--acknowledge-needs-human", "--dry-run"}); err != nil {
			t.Fatalf("dry-run resume: %v", err)
		}
		if backend.mutations != 0 {
			t.Fatalf("dry-run mutations = %d, want zero; calls=%v", backend.mutations, backend.calls)
		}
		if !strings.Contains(output.String(), "no mutation performed") {
			t.Fatalf("dry-run output = %q", output.String())
		}
	})

	t.Run("primary closing issue may follow companion", func(t *testing.T) {
		fixture := newResumeFixture(t)
		backend := newResumeBackend(t, fixture)
		backend.companionFirst = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(append(resumeArgs(fixture.expected), "--dry-run")); err != nil {
			t.Fatalf("resume with primary closing issue second: %v", err)
		}
		if backend.mutations != 0 {
			t.Fatalf("dry-run mutations = %d, want zero; calls=%v", backend.mutations, backend.calls)
		}
	})

	t.Run("apply creates and pushes one empty renewal", func(t *testing.T) {
		fixture := newResumeFixture(t)
		backend := newResumeBackend(t, fixture)
		unstaged := filepath.Join(fixture.worktree, "unstaged.txt")
		if err := os.WriteFile(unstaged, []byte("preserve\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		untracked := filepath.Join(fixture.worktree, "untracked.txt")
		if err := os.WriteFile(untracked, []byte("preserve too\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("apply resume: %v", err)
		}
		head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^"); got != fixture.expected {
			t.Fatalf("renewal parent = %s, want %s", got, fixture.expected)
		}
		if got, want := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}"), runGitTest(t, fixture.worktree, "rev-parse", "HEAD^^{tree}"); got != want {
			t.Fatalf("renewal tree = %s, want parent tree %s", got, want)
		}
		message := runGitTest(t, fixture.worktree, "log", "-1", "--format=%B")
		if !strings.Contains(message, "Agent-Run-ID: "+fixture.runID) {
			t.Fatalf("renewal message = %q", message)
		}
		wantLease := "--force-with-lease=refs/heads/agent/issue-14:" + fixture.expected
		if !backend.hasCall("git push " + wantLease + " origin " + head + ":refs/heads/agent/issue-14") {
			t.Fatalf("push calls = %v, want exact lease", backend.calls)
		}
		for _, path := range []string{unstaged, untracked} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("preserved file %s: %v", path, err)
			}
		}
	})

	t.Run("ambiguous push retries already-pushed child", func(t *testing.T) {
		fixture := newResumeFixture(t)
		backend := newResumeBackend(t, fixture)
		backend.ambiguousPush = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		err := application.run(resumeArgs(fixture.expected))
		if err == nil || !strings.Contains(err.Error(), strings.Join(resumeArgs(fixture.expected)[:6], " ")) {
			t.Fatalf("ambiguous push error = %v", err)
		}
		head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
		backend.ambiguousPush = false
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("retry resume: %v", err)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != head {
			t.Fatalf("retry created duplicate renewal: before=%s after=%s", head, got)
		}
	})

	t.Run("local-only ambiguous retry pushes existing child", func(t *testing.T) {
		fixture := newResumeFixture(t)
		backend := newResumeBackend(t, fixture)
		lease := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
		child := createResumeTestCommit(t, fixture.worktree, fixture.expected, claimMessage(14, fixture.runID, lease))
		runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, child, fixture.expected)
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("local-only retry: %v", err)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != child {
			t.Fatalf("local-only retry head = %s, want existing child %s", got, child)
		}
	})

	t.Run("current and older run-local artifacts are preserved independently", func(t *testing.T) {
		fixture := newResumeFixture(t)
		staleBranch := "agent/issue-14-run-old"
		stalePath := claimWorktreePath(fixture.primary, staleBranch)
		runGitTest(t, fixture.primary, "worktree", "add", "-b", staleBranch, stalePath, fixture.expected)
		runGitTest(t, fixture.primary, "push", "origin", fixture.expected+":refs/heads/"+staleBranch)
		runGitTest(t, fixture.primary, "push", "origin", fixture.expected+":refs/heads/agent/issue-14-run-resume-test")
		backend := newResumeBackend(t, fixture)
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("resume with current and older run-local artifacts: %v", err)
		}
		if output := runGitTest(t, fixture.primary, "ls-remote", "--heads", "origin", "refs/heads/"+staleBranch); !strings.Contains(output, staleBranch) {
			t.Fatalf("stale remote ref = %q, want preserved %s", output, staleBranch)
		}
		if output := runGitTest(t, fixture.primary, "worktree", "list", "--porcelain"); !strings.Contains(output, stalePath) {
			t.Fatalf("stale worktree was removed:\n%s", output)
		}
	})
}

func TestPRResumeAcceptsCurrentRunLocalAncestorSourceDivergence(t *testing.T) {
	for _, name := range []string{"#274 remote ancestor", "#284 remote ancestor", "#286 remote ancestor"} {
		t.Run(name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			ancestor := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
			runGitTest(t, fixture.worktree, "push", "origin", ancestor+":refs/heads/agent/issue-14-run-resume-test")
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(append(resumeArgs(fixture.expected), "--dry-run")); err != nil {
				t.Fatalf("resume with current strict-ancestor source: %v", err)
			}
			if backend.mutations != 0 {
				t.Fatalf("ancestor source dry-run mutations = %d; calls=%v", backend.mutations, backend.calls)
			}
		})
	}
}

func TestPRResumeFastForwardsCleanLocalAncestorWithRemoteAncestor(t *testing.T) {
	fixture := newResumeFixture(t)
	ancestor := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
	runGitTest(t, fixture.worktree, "reset", "--hard", ancestor)
	runGitTest(t, fixture.worktree, "push", "origin", ancestor+":refs/heads/agent/issue-14-run-resume-test")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(resumeArgs(fixture.expected)); err != nil {
		t.Fatalf("resume with clean local and remote ancestors: %v", err)
	}
	head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^"); got != fixture.expected {
		t.Fatalf("renewal parent = %s, want expected head %s", got, fixture.expected)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != head {
		t.Fatalf("renewal head changed during inspection: got %s, want %s", got, head)
	}
	if output := runGitTest(t, fixture.worktree, "ls-remote", "--heads", "origin", "refs/heads/agent/issue-14-run-resume-test"); !strings.Contains(output, ancestor) {
		t.Fatalf("remote run-local ancestor = %q, want preserved %s", output, ancestor)
	}
}

func TestPRResumeRejectsDirtyLocalAncestor(t *testing.T) {
	for _, test := range []struct {
		name string
		file string
	}{
		{name: "unstaged", file: "dirty.txt"},
		{name: "untracked", file: "untracked.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			ancestor := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
			runGitTest(t, fixture.worktree, "reset", "--hard", ancestor)
			if err := os.WriteFile(filepath.Join(fixture.worktree, test.file), []byte("preserve\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			runGitTest(t, fixture.worktree, "push", "origin", ancestor+":refs/heads/agent/issue-14-run-resume-test")
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(resumeArgs(fixture.expected))
			if err == nil || !strings.Contains(err.Error(), "not clean before fast-forward") {
				t.Fatalf("dirty local ancestor error = %v, want clean-worktree refusal", err)
			}
			if backend.mutations != 0 {
				t.Fatalf("dirty local ancestor mutations = %d; calls=%v", backend.mutations, backend.calls)
			}
		})
	}
}

func TestResumeRunLocalAncestorSourceIsRecheckedAtSeal(t *testing.T) {
	const (
		fixedBranch = "agent/issue-274"
		runBranch   = "agent/issue-274-run-current"
		fixedHead   = "fixed-head"
		ancestor    = "ancestor-head"
		moved       = "moved-head"
	)
	inventoryReads := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git ls-remote --heads origin refs/heads/agent/*":
			inventoryReads++
			runHead := ancestor
			if inventoryReads > 1 {
				runHead = moved
			}
			return fixedHead + " refs/heads/" + fixedBranch + "\n" + runHead + " refs/heads/" + runBranch, nil
		case "git merge-base --is-ancestor " + ancestor + " " + fixedHead:
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	observation, err := application.inspectResumeClaimConflicts("/repo", 274, fixedBranch, fixedHead, runBranch, "run-current",
		resumeRunLocalExpectation{}, nil)
	if err != nil {
		t.Fatalf("initial ancestor source inspection: %v", err)
	}
	if observation.sha != ancestor || !observation.present {
		t.Fatalf("initial source observation = %#v, want %s", observation, ancestor)
	}
	_, err = application.inspectResumeClaimConflicts("/repo", 274, fixedBranch, fixedHead, runBranch, "run-current",
		resumeRunLocalExpectation{sha: observation.sha, present: true, set: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "moved during proof") {
		t.Fatalf("moved ancestor source inspection = %v, want exact source race", err)
	}
	if !isRunLocalSourceRace(err) {
		t.Fatalf("moved ancestor source error = %v, want typed source race", err)
	}
}

//nolint:gocognit // The rejection table keeps each proof failure and its zero-mutation assertion together.
func TestPRResumeInjectedRejectionsPrecedeMutation(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*resumeFixture, *resumeBackend)
		want      string
		wantRetry bool
	}{
		{name: "moved PR head", edit: func(_ *resumeFixture, b *resumeBackend) { b.prHead = strings.Repeat("a", 40) }, want: "resume heads moved"},
		{name: "moved remote head", edit: func(f *resumeFixture, _ *resumeBackend) {
			moved := createResumeTestCommit(t, f.worktree, f.expected, "test: remote movement\n")
			runGitTest(t, f.worktree, "push", "--force", "origin", moved+":refs/heads/agent/issue-14")
		}, want: "differs from local head"},
		{name: "wrong base", edit: func(_ *resumeFixture, b *resumeBackend) { b.base = "develop" }, want: "not main"},
		{name: "wrong PR branch", edit: func(_ *resumeFixture, b *resumeBackend) { b.prBranch = "topic" }, want: "fixed claim branch"},
		{name: "wrong closing issue", edit: func(_ *resumeFixture, b *resumeBackend) { b.closing = 99 }, want: "primary issue"},
		{name: "three closing issues", edit: func(_ *resumeFixture, b *resumeBackend) { b.closingCount = 3 }, want: "closes 3 issues; a work packet permits one primary and one companion"},
		{name: "three closing issues while sealing", edit: func(_ *resumeFixture, b *resumeBackend) {
			b.threeClosingOnSeal = true
		}, want: "changed while sealing resume proof"},
		{name: "closed PR", edit: func(_ *resumeFixture, b *resumeBackend) { b.prState = "closed" }, want: "not an open unmerged PR"},
		{name: "merged PR", edit: func(_ *resumeFixture, b *resumeBackend) { b.merged = true }, want: "not an open unmerged PR"},
		{name: "closed issue", edit: func(_ *resumeFixture, b *resumeBackend) { b.issueState = "closed" }, want: "must be open"},
		{name: "missing needs-human", edit: func(_ *resumeFixture, b *resumeBackend) { b.needsHuman = false }, want: "must be labeled needs-human"},
		{name: "needs-human race immediately before mutation", edit: func(_ *resumeFixture, b *resumeBackend) {
			b.dropNeedsHumanBeforeMutation = true
		}, want: "must remain open and labeled needs-human immediately before", wantRetry: true},
		{name: "staged changes", edit: func(f *resumeFixture, _ *resumeBackend) {
			if err := os.WriteFile(filepath.Join(f.worktree, "staged"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			runGitTest(t, f.worktree, "add", "staged")
		}, want: "staged changes"},
		{name: "local movement", edit: func(f *resumeFixture, _ *resumeBackend) {
			runGitTest(t, f.worktree, "commit", "--allow-empty", "-m", "test: move local")
		}, want: "not a retryable renewal"},
		{name: "wrong run branch", edit: func(f *resumeFixture, _ *resumeBackend) {
			runGitTest(t, f.worktree, "branch", "-m", "agent/issue-14-run-wrong")
		}, want: "does not match Agent-Run-ID"},
		{name: "current run-local head race", edit: func(f *resumeFixture, _ *resumeBackend) {
			moved := createResumeTestCommit(t, f.worktree, f.expected, "test: move current run-local ref\n")
			runGitTest(t, f.worktree, "push", "origin", moved+":refs/heads/agent/issue-14-run-resume-test")
		}, want: "conflicting run-local ref"},
		{name: "duplicate fixed claim worktree", edit: func(f *resumeFixture, _ *resumeBackend) {
			path := filepath.Join(t.TempDir(), "duplicate")
			runGitTest(t, f.worktree, "worktree", "add", "-b", "agent/issue-14", path, f.expected)
		}, want: "stale duplicate/orphan claim worktree"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			backend := newResumeBackend(t, fixture)
			test.edit(&fixture, backend)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(resumeArgs(fixture.expected))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("resume error = %v, want %q", err, test.want)
			}
			if test.wantRetry {
				wantRetry := fmt.Sprintf(resumeRecoveryTemplate, 14, fixture.expected)
				if !strings.Contains(err.Error(), wantRetry) {
					t.Fatalf("resume error = %v, want exact retry %q", err, wantRetry)
				}
			}
			if backend.mutations != 0 {
				t.Fatalf("rejection mutations = %d; calls=%v", backend.mutations, backend.calls)
			}
		})
	}
}

type resumeFixture struct {
	baseRepositoryFixture
	expected string
	runID    string
	worktree string
}

func newResumeFixture(t *testing.T) resumeFixture {
	t.Helper()
	base := newBaseRepositoryFixture(t, false)
	runID := "run-resume-test"
	lease := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	parent := runGitTest(t, base.primary, "rev-parse", "HEAD")
	expected := createResumeTestCommit(t, base.primary, parent, claimMessage(14, runID, lease))
	runGitTest(t, base.primary, "push", "origin", expected+":refs/heads/agent/issue-14")
	worktree := filepath.Join(t.TempDir(), "issue-14-run-resume-test")
	runGitTest(t, base.primary, "worktree", "add", "-b", "agent/issue-14-"+runID, worktree, expected)
	return resumeFixture{baseRepositoryFixture: base, expected: expected, runID: runID, worktree: worktree}
}

func createResumeTestCommit(t *testing.T, root, parent, message string) string {
	t.Helper()
	tree := runGitTest(t, root, "rev-parse", parent+"^{tree}")
	// #nosec G204 -- the test executes a fixed Git subcommand with test-owned object IDs.
	command := exec.CommandContext(context.Background(), "git", "commit-tree", tree, "-p", parent)
	command.Dir = root
	command.Stdin = strings.NewReader(message)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("create test commit: %v: %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

type resumeBackend struct {
	t                            *testing.T
	fixture                      resumeFixture
	base                         string
	prBranch                     string
	prHead                       string
	prState                      string
	merged                       bool
	closing                      int
	closingCount                 int
	companionFirst               bool
	prReads                      int
	threeClosingOnSeal           bool
	needsHuman                   bool
	issueState                   string
	issueReads                   int
	dropNeedsHumanBeforeMutation bool
	ambiguousPush                bool
	mutations                    int
	calls                        []string
}

func newResumeBackend(t *testing.T, fixture resumeFixture) *resumeBackend {
	return &resumeBackend{t: t, fixture: fixture, base: "main", prBranch: "agent/issue-14", prState: "open",
		closing: 14, closingCount: 1, needsHuman: true, issueState: "open"}
}

func resumeArgs(head string) []string {
	return []string{"pr", "resume", "14", "--expected-head", head, "--acknowledge-needs-human"}
}

func (b *resumeBackend) hasCall(want string) bool {
	for _, call := range b.calls {
		if call == want {
			return true
		}
	}
	return false
}

func (b *resumeBackend) execute(dir string, input io.Reader, name string, args ...string) (string, error) {
	if dir == "" {
		dir = b.fixture.worktree
	}
	call := name + " " + strings.Join(args, " ")
	b.calls = append(b.calls, call)
	if name == "gh" {
		return b.executeGH(args...)
	}
	mutating := name == "git" && len(args) > 0 && (args[0] == "commit-tree" || args[0] == "update-ref" || args[0] == "push")
	if mutating {
		b.mutations++
	}
	// #nosec G204 -- the injected test executor runs only workflowctl-generated commands.
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
	if name == "git" && len(args) > 0 && (args[0] == "cat-file" ||
		(args[0] == "rev-parse" && len(args) > 1 && strings.HasSuffix(args[1], "^{tree}")) ||
		(args[0] == "log" && len(args) > 2 && args[1] == "-1" && args[2] == "--format=%B")) {
		return string(output), nil
	}
	return strings.TrimSpace(string(output)), nil
}

func (b *resumeBackend) executeGH(args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if joined == "api repos/goxdra/goxsd9/pulls/14" {
		return b.pullRequestResponse(), nil
	}
	if joined == "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
		return "[]", nil
	}
	if joined == "api repos/goxdra/goxsd9/issues/14" {
		b.issueReads++
		if b.dropNeedsHumanBeforeMutation && b.issueReads == 5 {
			b.needsHuman = false
		}
		labels := "[]"
		if b.needsHuman {
			labels = `[{"name":"needs-human"}]`
		}
		return `{"state":` + fmt.Sprintf("%q", b.issueState) + `,"labels":` + labels + `}`, nil
	}
	if strings.HasPrefix(joined, "issue edit 14 ") {
		b.mutations++
		b.needsHuman = false
		return "", nil
	}
	if strings.HasPrefix(joined, "project item-list ") {
		return `{"items":[{"id":"item","status":"Picked","content":{"number":14,"repository":"goxdra/goxsd9","type":"Issue"}}],"totalCount":1}`, nil
	}
	return "", fmt.Errorf("unexpected gh command: %s", joined)
}

func (b *resumeBackend) pullRequestResponse() string {
	b.prReads++
	head := b.prHead
	if head == "" {
		head = strings.Fields(runGitTest(b.t, b.fixture.worktree, "ls-remote", "origin", "refs/heads/agent/issue-14"))[0]
	}
	body := fmt.Sprintf("Closes #%d", b.closing)
	if b.companionFirst {
		body = fmt.Sprintf("Closes #15\n\nCloses #%d", b.closing)
	}
	closingCount := b.closingCount
	if b.threeClosingOnSeal && b.prReads >= 2 {
		closingCount = 3
	}
	if closingCount == 3 {
		body = fmt.Sprintf("Closes #%d\n\nCloses #15\n\nCloses #16", b.closing)
	}
	return fmt.Sprintf(`{"base":{"ref":%q,"sha":"base"},"body":%q,"head":{"ref":%q,"sha":%q},"state":%q,"merged":%t}`,
		b.base, body, b.prBranch, head, b.prState, b.merged)
}

func TestValidateResumeWorktreeBindsRegistration(t *testing.T) {
	const (
		root   = "/repo-worktrees/issue-55-run-good"
		branch = "agent/issue-55-run-good"
		head   = "head"
	)
	valid := repositoryLayout{worktrees: []gitWorktree{{path: root, branch: "refs/heads/" + branch, head: head}}}
	if err := validateResumeWorktree(valid, root, branch, 55, head); err != nil {
		t.Fatalf("validateResumeWorktree valid proof: %v", err)
	}

	tests := []struct {
		name   string
		layout repositoryLayout
		want   string
	}{
		{name: "wrong run", layout: repositoryLayout{worktrees: []gitWorktree{{path: root, branch: "refs/heads/agent/issue-55-run-other", head: head}}}, want: "does not match"},
		{name: "wrong head", layout: repositoryLayout{worktrees: []gitWorktree{{path: root, branch: "refs/heads/" + branch, head: "moved"}}}, want: "does not match"},
		{name: "locked", layout: repositoryLayout{worktrees: []gitWorktree{{path: root, branch: "refs/heads/" + branch, head: head, locked: true}}}, want: "does not match"},
		{name: "duplicate", layout: repositoryLayout{worktrees: []gitWorktree{
			{path: root, branch: "refs/heads/" + branch, head: head},
			{path: "/orphan", branch: "refs/heads/agent/issue-55-run-old", head: "old"},
		}}, want: "prove its branch, run ID, head reachability, cleanliness, and archive ref"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateResumeWorktree(test.layout, root, branch, 55, head)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateResumeWorktree error = %v, want %q", err, test.want)
			}
		})
	}
}
