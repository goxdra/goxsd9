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

func TestPRResumeAcceptsSourceBearingAndMergeExpectedHeads(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T, *resumeFixture) string
	}{
		{name: "source-bearing PR head", build: makeSourceBearingResumeHead},
		{name: "merge PR head", build: makeMergeResumeHead},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			head := test.build(t, &fixture)
			fixture.expected = head
			backend := newResumeBackend(t, fixture)
			before := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(append(resumeArgs(head), "--dry-run")); err != nil {
				t.Fatalf("resume with %s: %v", test.name, err)
			}
			if backend.mutations != 0 {
				t.Fatalf("%s dry-run mutations = %d; calls=%v", test.name, backend.mutations, backend.calls)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != before {
				t.Fatalf("%s dry-run moved local head from %s to %s", test.name, before, got)
			}
		})
	}
}

func TestPRResumeRejectsSourceBearingAndMergeClaimMarkersBeforeMutation(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T, *resumeFixture) string
		want  string
	}{
		{name: "source-bearing claim marker", build: makeSourceBearingClaimMarkerResumeHead, want: "source-bearing"},
		{name: "merge claim marker", build: makeMergeClaimMarkerResumeHead, want: "non-canonical parent shape"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			head := test.build(t, &fixture)
			fixture.expected = head
			backend := newResumeBackend(t, fixture)
			before := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(append(resumeArgs(head), "--dry-run"))
			if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s error = %v, disposition %d, want terminal %q", test.name, err, operationDispositionOf(err), test.want)
			}
			if backend.mutations != 0 {
				t.Fatalf("%s mutations = %d; calls=%v", test.name, backend.mutations, backend.calls)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != before {
				t.Fatalf("%s moved local head from %s to %s", test.name, before, got)
			}
			if got := resumeRemoteHead(t, fixture); got != head {
				t.Fatalf("%s moved remote head to %s, want %s", test.name, got, head)
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
		sentinel := errors.New("ambiguous push sentinel")
		backend.ambiguousPushCause = sentinel
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		err := application.run(resumeArgs(fixture.expected))
		if err == nil || operationDispositionOf(err) != operationDispositionRetryable || !errors.Is(err, sentinel) ||
			!strings.Contains(err.Error(), strings.Join(resumeArgs(fixture.expected)[:6], " ")) {
			t.Fatalf("ambiguous push error = %v, disposition %d", err, operationDispositionOf(err))
		}
		head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
		if head == fixture.expected {
			t.Fatalf("ambiguous push did not create a renewal child")
		}
		if got := resumeRemoteHead(t, fixture); got != head {
			t.Fatalf("ambiguous push heads diverged: local=%s remote=%s", head, got)
		}
		commits := countResumeCalls(backend.calls, "git commit-tree ")
		updates := countResumeCalls(backend.calls, "git update-ref ")
		pushes := countResumeCalls(backend.calls, "git push ")
		backend.ambiguousPush = false
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("retry resume: %v", err)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != head {
			t.Fatalf("retry created duplicate renewal: before=%s after=%s", head, got)
		}
		if got := resumeRemoteHead(t, fixture); got != head {
			t.Fatalf("retry changed converged renewal: before=%s after=%s", head, got)
		}
		if got := countResumeCalls(backend.calls, "git commit-tree "); got != commits {
			t.Fatalf("retry created an additional renewal commit: before=%d after=%d", commits, got)
		}
		if got := countResumeCalls(backend.calls, "git update-ref "); got != updates {
			t.Fatalf("retry advanced the local renewal ref again: before=%d after=%d", updates, got)
		}
		if got := countResumeCalls(backend.calls, "git push "); got != pushes {
			t.Fatalf("retry issued an additional push: before=%d after=%d", pushes, got)
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

//nolint:gocognit,funlen // Table cases keep all initial read operation boundaries together.
func TestPRResumeInitialReadOperationBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		intercept  func(string, *resumeBackend, error) (string, bool, error)
		want       operationDisposition
		wantCause  bool
		wantOutput string
	}{
		{
			name: "PR transport",
			intercept: func(command string, _ *resumeBackend, sentinel error) (string, bool, error) {
				if command == "gh api repos/goxdra/goxsd9/pulls/14" {
					return "", true, sentinel
				}
				return "", false, nil
			},
			want:      operationDispositionRetryable,
			wantCause: true,
		},
		{
			name: "malformed PR success",
			intercept: func(command string, _ *resumeBackend, _ error) (string, bool, error) {
				if command == "gh api repos/goxdra/goxsd9/pulls/14" {
					return `{"number":14}`, true, nil
				}
				return "", false, nil
			},
			want: operationDispositionTerminal,
		},
		{
			name: "malformed comment page",
			intercept: func(command string, _ *resumeBackend, _ error) (string, bool, error) {
				if command == "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
					return "null", true, nil
				}
				return "", false, nil
			},
			want: operationDispositionTerminal,
		},
		{
			name: "comment body omitted",
			intercept: func(command string, _ *resumeBackend, _ error) (string, bool, error) {
				if command == "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
					return `[{"id":1,"user":{"login":"trusted"},"created_at":"2026-01-01T00:00:00Z"}]`, true, nil
				}
				return "", false, nil
			},
			want: operationDispositionTerminal,
		},
		{
			name: "comment body null",
			intercept: func(command string, _ *resumeBackend, _ error) (string, bool, error) {
				if command == "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
					return `[{"id":1,"body":null,"user":{"login":"trusted"},"created_at":"2026-01-01T00:00:00Z"}]`, true, nil
				}
				return "", false, nil
			},
			want: operationDispositionTerminal,
		},
		{
			name: "issue transport",
			intercept: func(command string, _ *resumeBackend, sentinel error) (string, bool, error) {
				if command == "gh api repos/goxdra/goxsd9/issues/14" {
					return "", true, sentinel
				}
				return "", false, nil
			},
			want:      operationDispositionRetryable,
			wantCause: true,
		},
		{
			name: "git transport",
			intercept: func(command string, _ *resumeBackend, sentinel error) (string, bool, error) {
				if command == "git rev-parse HEAD" {
					return "", true, sentinel
				}
				return "", false, nil
			},
			want:      operationDispositionRetryable,
			wantCause: true,
		},
		{
			name: "expected head object transport",
			intercept: func(command string, backend *resumeBackend, sentinel error) (string, bool, error) {
				if command == "git cat-file commit "+backend.fixture.expected {
					return "", true, sentinel
				}
				return "", false, nil
			},
			want:      operationDispositionRetryable,
			wantCause: true,
		},
		{
			name: "staged-check transport",
			intercept: func(command string, _ *resumeBackend, sentinel error) (string, bool, error) {
				if command == "git diff --cached --quiet" {
					return "", true, sentinel
				}
				return "", false, nil
			},
			want:      operationDispositionRetryable,
			wantCause: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			backend := newResumeBackend(t, fixture)
			sentinel := errors.New(test.name + " sentinel")
			application := app{ctx: context.Background(), executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				if output, handled, err := test.intercept(command, backend, sentinel); handled {
					return output, err
				}
				return backend.execute(dir, input, name, args...)
			}, stdout: io.Discard}
			err := application.run(resumeArgs(fixture.expected))
			if err == nil || operationDispositionOf(err) != test.want {
				t.Fatalf("resume error = %v, disposition %d, want %d", err, operationDispositionOf(err), test.want)
			}
			if test.wantCause && !errors.Is(err, sentinel) {
				t.Fatalf("resume error = %v, want cause %v", err, sentinel)
			}
			if test.wantOutput != "" && !strings.Contains(err.Error(), test.wantOutput) {
				t.Fatalf("resume error = %v, want %q", err, test.wantOutput)
			}
			if backend.mutations != 0 {
				t.Fatalf("initial read failure mutations = %d, want zero", backend.mutations)
			}
		})
	}
}

//nolint:gocognit // The table exercises each staged-check status at one proof boundary.
func TestPRResumeStagedCheckExitBoundariesPreserveCauseAndMutation(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		captureErr error
		want       operationDisposition
		wantCause  bool
		wantOutput string
	}{
		{name: "clean", want: operationDispositionUnknown},
		{name: "staged changes", status: 1, want: operationDispositionTerminal, wantOutput: "staged changes"},
		{name: "git uncertainty", status: 128, want: operationDispositionRetryable},
		{name: "transport", captureErr: errors.New("staged-check transport sentinel"), want: operationDispositionRetryable, wantCause: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			backend := newResumeBackend(t, fixture)
			application := app{
				ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard,
				executeCommandCapture: func(_ string, _ []string, name string, args ...string) (commandCaptureResult, error) {
					if name != "git" || strings.Join(args, " ") != "diff --cached --quiet" {
						return commandCaptureResult{}, fmt.Errorf("unexpected captured command: %s %s", name, strings.Join(args, " "))
					}
					if test.captureErr != nil {
						return commandCaptureResult{}, test.captureErr
					}
					return commandCaptureResult{status: test.status}, nil
				},
			}
			args := append(resumeArgs(fixture.expected), "--dry-run")
			err := application.run(args)
			if test.want == operationDispositionUnknown {
				if err != nil {
					t.Fatalf("clean staged check: %v", err)
				}
			}
			if test.want != operationDispositionUnknown {
				if err == nil || operationDispositionOf(err) != test.want {
					t.Fatalf("staged check error = %v, disposition %d, want %d", err, operationDispositionOf(err), test.want)
				}
				if test.wantCause && !errors.Is(err, test.captureErr) {
					t.Fatalf("staged check error = %v, want cause %v", err, test.captureErr)
				}
				if test.wantOutput != "" && !strings.Contains(err.Error(), test.wantOutput) {
					t.Fatalf("staged check error = %v, want %q", err, test.wantOutput)
				}
			}
			if backend.mutations != 0 {
				t.Fatalf("staged check mutations = %d, want zero; args=%v calls=%v", backend.mutations, args, backend.calls)
			}
		})
	}
}

//nolint:gocognit,funlen // Table cases keep mutation counts and state assertions aligned.
func TestPRResumeMutationBoundariesPreserveOperation(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*resumeBackend, error)
		wantMutations int
		assertState   func(*testing.T, resumeFixture, *resumeBackend)
	}{
		{
			name:          "fresh proof",
			wantMutations: 0,
			setup: func(backend *resumeBackend, sentinel error) {
				backend.prReadFailureAt = 2
				backend.prReadFailure = sentinel
			},
			assertState: func(t *testing.T, fixture resumeFixture, _ *resumeBackend) {
				if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
					t.Fatalf("fresh-proof failure moved local head from %s to %s", fixture.expected, got)
				}
				if got := resumeRemoteHead(t, fixture); got != fixture.expected {
					t.Fatalf("fresh-proof failure moved remote head from %s to %s", fixture.expected, got)
				}
			},
		},
		{
			name:          "push verification",
			wantMutations: 3,
			setup: func(backend *resumeBackend, sentinel error) {
				backend.prReadFailureAfterMutation = sentinel
			},
			assertState: func(t *testing.T, fixture resumeFixture, _ *resumeBackend) {
				assertResumeRenewalHeads(t, fixture)
			},
		},
		{
			name:          "status read",
			wantMutations: 0,
			setup: func(backend *resumeBackend, sentinel error) {
				backend.issueReadFailureAt = 5
				backend.issueReadFailure = sentinel
			},
			assertState: func(t *testing.T, fixture resumeFixture, _ *resumeBackend) {
				if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
					t.Fatalf("status-read failure moved local head from %s to %s", fixture.expected, got)
				}
				if got := resumeRemoteHead(t, fixture); got != fixture.expected {
					t.Fatalf("status-read failure moved remote head from %s to %s", fixture.expected, got)
				}
			},
		},
		{
			name:          "label mutation",
			wantMutations: 4,
			setup: func(backend *resumeBackend, sentinel error) {
				backend.labelFailure = sentinel
			},
			assertState: func(t *testing.T, fixture resumeFixture, backend *resumeBackend) {
				assertResumeRenewalHeads(t, fixture)
				if !backend.needsHuman {
					t.Fatal("label failure cleared needs-human")
				}
				if backend.projectStatus != "Backlog" {
					t.Fatalf("label failure changed Project status to %q", backend.projectStatus)
				}
			},
		},
		{
			name:          "Project mutation",
			wantMutations: 5,
			setup: func(backend *resumeBackend, sentinel error) {
				backend.projectFailure = sentinel
			},
			assertState: func(t *testing.T, fixture resumeFixture, backend *resumeBackend) {
				assertResumeRenewalHeads(t, fixture)
				if backend.needsHuman {
					t.Fatal("Project failure left needs-human label")
				}
				if backend.projectStatus != "Backlog" {
					t.Fatalf("Project failure changed status to %q", backend.projectStatus)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			backend := newResumeBackend(t, fixture)
			sentinel := errors.New(test.name + " sentinel")
			test.setup(backend, sentinel)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(resumeArgs(fixture.expected))
			if err == nil || operationDispositionOf(err) != operationDispositionRetryable || !errors.Is(err, sentinel) {
				t.Fatalf("resume error = %v, disposition %d, want retryable cause", err, operationDispositionOf(err))
			}
			if backend.mutations != test.wantMutations {
				t.Fatalf("%s mutations = %d, want %d; calls=%v", test.name, backend.mutations, test.wantMutations, backend.calls)
			}
			if test.assertState != nil {
				test.assertState(t, fixture, backend)
			}
		})
	}
}

func TestPRResumeTerminalOperationBoundaryPreserved(t *testing.T) {
	fixture := newResumeFixture(t)
	backend := newResumeBackend(t, fixture)
	sentinel := errors.New("authenticated PR verification sentinel")
	backend.prReadFailureAfterMutation = terminalOperation("authenticated PR verification", sentinel)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	err := application.run(resumeArgs(fixture.expected))
	if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !errors.Is(err, sentinel) {
		t.Fatalf("terminal PR verification error = %v, disposition %d, want terminal cause", err, operationDispositionOf(err))
	}
	if !strings.Contains(err.Error(), "claim push needs reconciliation") {
		t.Fatalf("terminal PR verification error = %v, want outer resume wrapper", err)
	}
	if backend.mutations != 3 {
		t.Fatalf("terminal PR verification mutations = %d, want commit/update/push only", backend.mutations)
	}
	assertResumeRenewalHeads(t, fixture)
}

func TestPRResumeRejectsProtectedHeadWorktreesBeforeMutation(t *testing.T) {
	tests := []resumeProtectedHeadTest{
		{name: "detached expected head", protectRenewal: false},
		{name: "locked expected head", protectRenewal: false, locked: true},
		{name: "detached expected head after renewal", provenRenewal: true, protectRenewal: false},
		{name: "detached proven renewal head", provenRenewal: true, protectRenewal: true},
		{name: "locked proven renewal head", provenRenewal: true, protectRenewal: true, locked: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testPRResumeProtectedHeadWorktree(t, test)
		})
	}
}

type resumeProtectedHeadTest struct {
	name           string
	provenRenewal  bool
	protectRenewal bool
	locked         bool
}

func testPRResumeProtectedHeadWorktree(t *testing.T, test resumeProtectedHeadTest) {
	t.Helper()
	fixture := newResumeFixture(t)
	protectedRenewal := fixture.expected
	if test.provenRenewal {
		lease := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
		protectedRenewal = createResumeTestCommit(t, fixture.worktree, fixture.expected,
			claimMessage(14, fixture.runID, lease))
		runGitTest(t, fixture.worktree, "reset", "--hard", protectedRenewal)
		runGitTest(t, fixture.worktree, "push", "--force", "origin",
			protectedRenewal+":refs/heads/agent/issue-14")
	}
	protectedHead := fixture.expected
	if test.protectRenewal {
		protectedHead = protectedRenewal
	}
	duplicate := filepath.Join(t.TempDir(), "duplicate")
	runGitTest(t, fixture.primary, "worktree", "add", "--detach", duplicate, protectedHead)
	if test.locked {
		runGitTest(t, fixture.primary, "worktree", "lock", duplicate)
	}
	localBefore := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	remoteBefore := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	err := application.run(resumeArgs(fixture.expected))
	if err == nil || !strings.Contains(err.Error(), "detached duplicate/orphan claim worktree") {
		t.Fatalf("protected head error = %v, want detached/unknown worktree refusal", err)
	}
	if backend.mutations != 0 {
		t.Fatalf("protected head mutations = %d; calls=%v", backend.mutations, backend.calls)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != localBefore {
		t.Fatalf("local head changed: got %s, want %s", got, localBefore)
	}
	if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14"); got != remoteBefore {
		t.Fatalf("remote fixed head changed: got %q, want %q", got, remoteBefore)
	}
	if inventory := runGitTest(t, fixture.primary, "worktree", "list", "--porcelain"); !strings.Contains(inventory, duplicate) {
		t.Fatalf("protected worktree was removed:\n%s", inventory)
	}
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
		fixedHead   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		ancestor    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		moved       = "cccccccccccccccccccccccccccccccccccccccc"
	)
	inventoryReads := 0
	application := app{executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
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
		case "git cat-file --batch-check=%(objectname) %(objecttype)":
			value, err := io.ReadAll(input)
			if err != nil {
				return "", fmt.Errorf("read cat-file input: %w", err)
			}
			sha := strings.TrimSpace(string(value))
			return sha + " commit", nil
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

func TestPRResumeRejectsMalformedAdvertisedRunLocalBeforeMutation(t *testing.T) {
	fixture := newResumeFixture(t)
	backend := newResumeBackend(t, fixture)
	localBefore := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	remoteBefore := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14")
	application := app{ctx: context.Background(), executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if command == "git ls-remote --heads origin refs/heads/agent/*" {
			backend.calls = append(backend.calls, command)
			fixed := strings.Fields(remoteBefore)[0]
			return fixed + " refs/heads/agent/issue-14\nshort refs/heads/agent/issue-14-run-resume-test", nil
		}
		return backend.execute(dir, input, name, args...)
	}, stdout: io.Discard}
	err := application.run(resumeArgs(fixture.expected))
	if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !strings.Contains(err.Error(), "malformed object name") {
		t.Fatalf("malformed advertised run-local error = %v, disposition %d, want terminal malformed refusal", err, operationDispositionOf(err))
	}
	if backend.mutations != 0 {
		t.Fatalf("malformed advertised run-local mutations = %d, want zero", backend.mutations)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != localBefore {
		t.Fatalf("malformed advertised run-local moved local head from %s to %s", localBefore, got)
	}
	if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14"); got != remoteBefore {
		t.Fatalf("malformed advertised run-local moved remote head from %q to %q", remoteBefore, got)
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

func makeSourceBearingResumeHead(t *testing.T, fixture *resumeFixture) string {
	t.Helper()
	writeFixtureFile(t, fixture.worktree, "source", "source-bearing PR head\n")
	runGitTest(t, fixture.worktree, "add", "source")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "feat: source-bearing PR head")
	head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	runGitTest(t, fixture.primary, "push", "--force", "origin", head+":refs/heads/agent/issue-14")
	return head
}

func makeMergeResumeHead(t *testing.T, fixture *resumeFixture) string {
	t.Helper()
	side := createResumeTestCommit(t, fixture.worktree, fixture.expected, "test: merge side parent\n")
	tree := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^{tree}")
	head := createResumeCommitTree(t, fixture.worktree, tree, []string{fixture.expected, side}, "Merge test PR head\n")
	runGitTest(t, fixture.worktree, "reset", "--hard", head)
	runGitTest(t, fixture.primary, "push", "--force", "origin", head+":refs/heads/agent/issue-14")
	return head
}

func makeSourceBearingClaimMarkerResumeHead(t *testing.T, fixture *resumeFixture) string {
	t.Helper()
	base := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
	writeFixtureFile(t, fixture.worktree, "marker-source", "source-bearing marker\n")
	runGitTest(t, fixture.worktree, "add", "marker-source")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "feat: marker source")
	tree := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}")
	marker := createResumeCommitTree(t, fixture.worktree, tree, []string{base}, claimMessage(14, fixture.runID, time.Now().UTC().Add(-time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "reset", "--hard", marker)
	runGitTest(t, fixture.primary, "push", "--force", "origin", marker+":refs/heads/agent/issue-14")
	return marker
}

func makeMergeClaimMarkerResumeHead(t *testing.T, fixture *resumeFixture) string {
	t.Helper()
	side := createResumeTestCommit(t, fixture.worktree, fixture.expected, "test: merge marker side\n")
	tree := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^{tree}")
	marker := createResumeCommitTree(t, fixture.worktree, tree, []string{fixture.expected, side}, claimMessage(14, fixture.runID, time.Now().UTC().Add(-time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "reset", "--hard", marker)
	runGitTest(t, fixture.primary, "push", "--force", "origin", marker+":refs/heads/agent/issue-14")
	return marker
}

func createResumeCommitTree(t *testing.T, root, tree string, parents []string, message string) string {
	t.Helper()
	args := make([]string, 0, 2+2*len(parents))
	args = append(args, "commit-tree", tree)
	for _, parent := range parents {
		args = append(args, "-p", parent)
	}
	// #nosec G204 -- the test executes fixed Git commit-tree arguments with fixture-owned object IDs.
	command := exec.CommandContext(context.Background(), "git", args...)
	command.Dir = root
	command.Stdin = strings.NewReader(message)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("create test commit tree: %v: %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func createResumeTestCommit(t *testing.T, root, parent, message string) string {
	t.Helper()
	tree := runGitTest(t, root, "rev-parse", parent+"^{tree}")
	return createResumeCommitTree(t, root, tree, []string{parent}, message)
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
	prReadFailureAt              int
	prReadFailure                error
	prReadFailureAfterMutation   error
	threeClosingOnSeal           bool
	needsHuman                   bool
	issueState                   string
	issueReads                   int
	issueReadFailureAt           int
	issueReadFailure             error
	dropNeedsHumanBeforeMutation bool
	ambiguousPush                bool
	ambiguousPushCause           error
	projectStatus                string
	labelFailure                 error
	projectFailure               error
	mutations                    int
	calls                        []string
}

func newResumeBackend(t *testing.T, fixture resumeFixture) *resumeBackend {
	return &resumeBackend{t: t, fixture: fixture, base: "main", prBranch: "agent/issue-14", prState: "open",
		closing: 14, closingCount: 1, needsHuman: true, issueState: "open", projectStatus: "Backlog"}
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

func countResumeCalls(calls []string, prefix string) int {
	count := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			count++
		}
	}
	return count
}

func resumeRemoteHead(t *testing.T, fixture resumeFixture) string {
	t.Helper()
	output := runGitTest(t, fixture.primary, "ls-remote", "--heads", "origin", "refs/heads/agent/issue-14")
	fields := strings.Fields(output)
	if len(fields) != 2 || fields[1] != "refs/heads/agent/issue-14" {
		t.Fatalf("remote fixed claim head = %q, want one agent/issue-14 ref", output)
	}
	return fields[0]
}

func assertResumeRenewalHeads(t *testing.T, fixture resumeFixture) {
	t.Helper()
	local := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if local == fixture.expected {
		t.Fatalf("resume did not create a renewal child")
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^"); got != fixture.expected {
		t.Fatalf("renewal parent = %s, want expected head %s", got, fixture.expected)
	}
	if got := resumeRemoteHead(t, fixture); got != local {
		t.Fatalf("local and remote renewal heads diverged: local=%s remote=%s", local, got)
	}
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
		if b.ambiguousPushCause != nil {
			err := b.ambiguousPushCause
			b.ambiguousPushCause = nil
			return "", err
		}
		return "", errors.New("simulated lost push response")
	}
	if name == "git" && len(args) > 0 && (args[0] == "cat-file" ||
		(args[0] == "rev-parse" && len(args) > 1 && strings.HasSuffix(args[1], "^{tree}")) ||
		(args[0] == "log" && len(args) > 2 && args[1] == "-1" && args[2] == "--format=%B")) {
		return string(output), nil
	}
	return strings.TrimSpace(string(output)), nil
}

//nolint:gocognit // The deterministic fake dispatches each GitHub boundary in one place.
func (b *resumeBackend) executeGH(args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if joined == "api repos/goxdra/goxsd9/pulls/14" {
		if b.prReadFailureAfterMutation != nil && b.mutations > 0 {
			err := b.prReadFailureAfterMutation
			b.prReadFailureAfterMutation = nil
			return "", err
		}
		if b.prReadFailure != nil && b.prReads >= b.prReadFailureAt {
			err := b.prReadFailure
			b.prReadFailure = nil
			return "", err
		}
		return b.pullRequestResponse(), nil
	}
	if joined == "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
		return "[]", nil
	}
	if joined == "api repos/goxdra/goxsd9/issues/14" {
		b.issueReads++
		if b.issueReadFailure != nil && b.issueReads >= b.issueReadFailureAt {
			err := b.issueReadFailure
			b.issueReadFailure = nil
			return "", err
		}
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
		if b.labelFailure != nil {
			err := b.labelFailure
			b.labelFailure = nil
			return "", err
		}
		b.needsHuman = false
		return "", nil
	}
	if strings.HasPrefix(joined, "project item-list ") {
		return fmt.Sprintf(`{"items":[{"id":"item","status":%q,"content":{"number":14,"repository":"goxdra/goxsd9","type":"Issue"}}],"totalCount":1}`, b.projectStatus), nil
	}
	if strings.HasPrefix(joined, "project field-list ") {
		return `{"fields":[{"id":"status-id","name":"Status","options":[{"id":"backlog-id","name":"Backlog"},{"id":"picked-id","name":"Picked"}]}]}`, nil
	}
	if strings.Contains(joined, "project item-edit") {
		b.mutations++
		if b.projectFailure != nil {
			err := b.projectFailure
			b.projectFailure = nil
			return "", err
		}
		b.projectStatus = "Picked"
		return "", nil
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
	return fmt.Sprintf(`{"number":14,"base":{"ref":%q,"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},"body":%q,"head":{"ref":%q,"sha":%q},"state":%q,"merged":%t}`,
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

//nolint:gocognit // The table keeps terminal and retryable object-boundary cases together.
func TestRemoteClaimHeadObjectDisposition(t *testing.T) {
	const (
		branch = "agent/issue-12"
		sha    = "dddddddddddddddddddddddddddddddddddddddd"
	)
	sentinel := errors.New("remote claim transport")
	for _, test := range []struct {
		name      string
		object    string
		want      operationDisposition
		wantCause error
	}{
		{name: "missing object", object: "missing", want: operationDispositionTerminal},
		{name: "non-commit object", object: "blob", want: operationDispositionTerminal},
		{name: "transport failure", object: "transport", want: operationDispositionRetryable, wantCause: sentinel},
		{name: "commit", object: "commit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			application := app{executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				switch command {
				case "git ls-remote --heads origin refs/heads/" + branch:
					return sha + " refs/heads/" + branch, nil
				case "git cat-file --batch-check=%(objectname) %(objecttype)":
					value, err := io.ReadAll(input)
					if err != nil {
						return "", fmt.Errorf("read object query: %w", err)
					}
					if test.object == "transport" {
						return "", sentinel
					}
					return strings.TrimSpace(string(value)) + " " + test.object, nil
				case "git fetch --no-tags --no-write-fetch-head origin refs/heads/" + branch:
					return "", nil
				default:
					return "", fmt.Errorf("unexpected command: %s", command)
				}
			}}
			_, err := application.remoteClaimHead("/repo", branch)
			if test.want == operationDispositionUnknown {
				if err != nil {
					t.Fatalf("remote claim head error = %v, want success", err)
				}
				return
			}
			if err == nil || operationDispositionOf(err) != test.want {
				t.Fatalf("remote claim head error = %v, disposition %d, want %d", err, operationDispositionOf(err), test.want)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("remote claim head error = %v, want cause %v", err, test.wantCause)
			}
		})
	}
}
