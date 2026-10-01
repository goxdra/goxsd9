package workflowctl

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type archivedClaimFixture struct {
	current claimResumeFixture
	backend *claimResumeBackend
	caller  string
	oldPath string
	oldHead string
	oldRun  string
	archive string
}

func newArchivedClaimFixture(t *testing.T) archivedClaimFixture {
	return newArchivedClaimFixtureWithSubmodule(t, false)
}

func newArchivedClaimFixtureWithSubmodule(t *testing.T, withSubmodule bool) archivedClaimFixture {
	t.Helper()
	base := newBaseRepositoryFixture(t, withSubmodule)
	parent := runGitTest(t, base.primary, "rev-parse", "HEAD")
	currentRun := "run-90414e34a638a766"
	currentLease := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	currentHead := createResumeTestCommit(t, base.primary, parent, claimMessage(333, currentRun, currentLease))
	runGitTest(t, base.primary, "push", "origin", currentHead+":refs/heads/"+claimBranch(333))
	current := claimResumeFixture{baseRepositoryFixture: base, issue: 333, expected: currentHead, runID: currentRun,
		worktree: filepath.Join(t.TempDir(), "current"), handoff: 2, lease: currentLease}
	runGitTest(t, base.primary, "worktree", "add", "-b", claimLocalBranch(333, currentRun), current.worktree, currentHead)
	currentPath := claimWorktreePath(current.primary, claimLocalBranch(333, current.runID))
	runGitTest(t, current.primary, "worktree", "move", current.worktree, currentPath)
	current.worktree = currentPath
	callerRun := "run-586proof"
	callerHead := createResumeTestCommit(t, current.primary, parent,
		claimMessage(586, callerRun, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	caller := claimWorktreePath(current.primary, claimLocalBranch(586, callerRun))
	runGitTest(t, current.primary, "worktree", "add", "-b", claimLocalBranch(586, callerRun), caller, callerHead)
	oldRun := "run-e9f268a4eece5222"
	initial := createResumeTestCommit(t, current.primary, parent,
		claimMessage(333, oldRun, time.Now().UTC().Add(-3*time.Hour).Truncate(time.Second)))
	writeFixtureFile(t, current.primary, "source", "historical implementation\n")
	runGitTest(t, current.primary, "add", "source")
	sourceTree := runGitTest(t, current.primary, "write-tree")
	runGitTest(t, current.primary, "reset", "--hard", "HEAD")
	source := createResumeCommitTree(t, current.primary, sourceTree, []string{initial}, "historical source work\n")
	merged := createResumeCommitTree(t, current.primary, sourceTree, []string{source, parent}, "historical merge\n")
	oldHead := createResumeTestCommit(t, current.primary, merged,
		claimMessage(333, oldRun, time.Now().UTC().Add(-2*time.Hour).Truncate(time.Second)))
	oldPath := claimWorktreePath(current.primary, claimLocalBranch(333, oldRun))
	runGitTest(t, current.primary, "worktree", "add", "-b", claimLocalBranch(333, oldRun), oldPath, oldHead)
	if withSubmodule {
		initializeFixtureSubmodule(t, oldPath)
	}
	archive := "agent/archive/issue-333-0b3555c5361679d4"
	runGitTest(t, current.primary, "push", "origin", oldHead+":refs/heads/"+archive)
	return archivedClaimFixture{current: current, backend: newClaimResumeBackend(t, current), caller: caller, oldPath: oldPath,
		oldHead: oldHead, oldRun: oldRun, archive: archive}
}

func archivedClaimArgs(f archivedClaimFixture, dryRun bool) []string {
	args := []string{"claim", "release-archived", strconv.Itoa(f.current.issue), "--run-id", f.oldRun, "--expected-head", f.oldHead}
	if dryRun {
		args = append(args, "--dry-run")
	}
	return args
}

func archivedClaimApp(f archivedClaimFixture, output io.Writer) app {
	return app{ctx: context.Background(), executeCommand: f.backend.execute, stdout: output}
}

func archivedClaimCallerApp(f archivedClaimFixture, output io.Writer) app {
	execute := func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if dir == "" {
			dir = f.caller
		}
		return f.backend.execute(dir, input, name, args...)
	}
	return app{ctx: context.Background(), executeCommand: execute, stdout: output}
}

func TestArchivedClaim333ReleaseEnablesResumeDryRun(t *testing.T) {
	f := newArchivedClaimFixture(t)
	resumeApplication := archivedClaimApp(f, io.Discard)
	if err := resumeApplication.run(claimResumeArgs(f.current, true)); err == nil || !strings.Contains(err.Error(), "stale duplicate/orphan") {
		t.Fatalf("resume with archived sibling = %v, want registration refusal", err)
	}
	var output bytes.Buffer
	application := archivedClaimCallerApp(f, &output)
	if err := application.run(archivedClaimArgs(f, true)); err != nil {
		t.Fatalf("archived claim dry-run: %v", err)
	}
	if !strings.Contains(output.String(), "dry-run: preflight complete") {
		t.Fatalf("dry-run output = %q", output.String())
	}
	if got := runGitTest(t, f.oldPath, "rev-parse", "HEAD"); got != f.oldHead {
		t.Fatalf("dry-run old head = %s, want %s", got, f.oldHead)
	}
	if err := application.run(archivedClaimArgs(f, false)); err != nil {
		t.Fatalf("release archived claim: %v", err)
	}
	if err := application.run(archivedClaimArgs(f, false)); err != nil {
		t.Fatalf("repeat archived release: %v", err)
	}
	if got := runGitTest(t, f.current.primary, "for-each-ref", "--format=%(objectname)", "refs/heads/"+claimLocalBranch(333, f.oldRun)); got != f.oldHead {
		t.Fatalf("preserved local run ref = %s, want %s", got, f.oldHead)
	}
	if got := runGitTest(t, f.current.primary, "ls-remote", "origin", "refs/heads/"+f.archive); !strings.HasPrefix(got, f.oldHead+"\t") {
		t.Fatalf("preserved remote archive = %q", got)
	}
	if got := runGitTest(t, f.current.primary, "rev-parse", "refs/heads/"+claimLocalBranch(333, f.current.runID)); got != f.current.expected {
		t.Fatalf("current claim head = %s, want %s", got, f.current.expected)
	}
	if err := resumeApplication.run(claimResumeArgs(f.current, true)); err != nil {
		t.Fatalf("current claim resume dry-run after archived release: %v", err)
	}
}

func TestArchivedClaimReleaseRejectsUnprovenSibling(t *testing.T) {
	tests := []struct {
		name  string
		alter func(*testing.T, *archivedClaimFixture)
	}{
		{name: "dirty untracked", alter: func(t *testing.T, f *archivedClaimFixture) {
			writeFixtureFile(t, f.oldPath, "dirty", "keep\n")
		}},
		{name: "dirty staged", alter: func(t *testing.T, f *archivedClaimFixture) {
			writeFixtureFile(t, f.oldPath, "staged", "keep\n")
			runGitTest(t, f.oldPath, "add", "staged")
		}},
		{name: "dirty unstaged", alter: func(t *testing.T, f *archivedClaimFixture) {
			writeFixtureFile(t, f.oldPath, "README", "modified\n")
		}},
		{name: "locked", alter: func(t *testing.T, f *archivedClaimFixture) {
			runGitTest(t, f.current.primary, "worktree", "lock", f.oldPath)
		}},
		{name: "detached", alter: func(t *testing.T, f *archivedClaimFixture) {
			runGitTest(t, f.oldPath, "checkout", "--detach", f.oldHead)
		}},
		{name: "archive moved", alter: func(t *testing.T, f *archivedClaimFixture) {
			runGitTest(t, f.current.primary, "push", "--force", "origin", f.current.expected+":refs/heads/"+f.archive)
		}},
		{name: "archive absent", alter: func(t *testing.T, f *archivedClaimFixture) {
			runGitTest(t, f.current.primary, "push", "origin", ":refs/heads/"+f.archive)
		}},
		{name: "ambiguous archive", alter: func(t *testing.T, f *archivedClaimFixture) {
			runGitTest(t, f.current.primary, "push", "origin", f.oldHead+":refs/heads/agent/archive/issue-333-aaaaaaaaaaaaaaaa")
		}},
		{name: "malformed matching archive", alter: func(t *testing.T, f *archivedClaimFixture) {
			runGitTest(t, f.current.primary, "push", "origin", f.oldHead+":refs/heads/agent/archive/issue-333-malformed")
		}},
		{name: "local head moved", alter: func(t *testing.T, f *archivedClaimFixture) {
			moved := createResumeTestCommit(t, f.current.primary, f.oldHead, claimMessage(333, f.oldRun, time.Now().UTC().Add(-time.Hour).Truncate(time.Second)))
			runGitTest(t, f.oldPath, "reset", "--hard", moved)
		}},
		{name: "marker wrong issue", alter: func(t *testing.T, f *archivedClaimFixture) {
			moved := createResumeTestCommit(t, f.current.primary, f.oldHead, claimMessage(334, f.oldRun, time.Now().UTC().Add(-time.Hour).Truncate(time.Second)))
			runGitTest(t, f.oldPath, "reset", "--hard", moved)
			runGitTest(t, f.current.primary, "push", "--force", "origin", moved+":refs/heads/"+f.archive)
			f.oldHead = moved
		}},
		{name: "worktree moved", alter: func(t *testing.T, f *archivedClaimFixture) {
			moved := filepath.Join(t.TempDir(), "moved")
			runGitTest(t, f.current.primary, "worktree", "move", f.oldPath, moved)
			f.oldPath = moved
		}},
		{name: "another same issue sibling", alter: func(t *testing.T, f *archivedClaimFixture) {
			other := filepath.Join(t.TempDir(), "other")
			runGitTest(t, f.current.primary, "worktree", "add", "-b", "agent/issue-333-run-other", other, f.oldHead)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newArchivedClaimFixture(t)
			test.alter(t, &f)
			before := runGitTest(t, f.current.primary, "worktree", "list", "--porcelain")
			err := archivedClaimApp(f, io.Discard).run(archivedClaimArgs(f, false))
			if err == nil {
				t.Fatal("unproven archived sibling was released")
			}
			if after := runGitTest(t, f.current.primary, "worktree", "list", "--porcelain"); after != before {
				t.Fatalf("refusal changed registrations: before %q, after %q", before, after)
			}
		})
	}
}

func TestArchivedClaimReleaseRejectsWrongIdentityAndCurrentRun(t *testing.T) {
	f := newArchivedClaimFixture(t)
	application := archivedClaimApp(f, io.Discard)
	for _, args := range [][]string{
		{"claim", "release-archived", "334", "--run-id", f.oldRun, "--expected-head", f.oldHead},
		{"claim", "release-archived", "333", "--run-id", "run-other", "--expected-head", f.oldHead},
		{"claim", "release-archived", "333", "--run-id", f.oldRun, "--expected-head", f.current.expected},
		{"claim", "release-archived", "333", "--run-id", f.current.runID, "--expected-head", f.current.expected},
	} {
		if err := application.run(args); err == nil {
			t.Fatalf("release %q unexpectedly accepted", args)
		}
		if got := runGitTest(t, f.oldPath, "rev-parse", "HEAD"); got != f.oldHead {
			t.Fatalf("refused release %q moved old head to %s", args, got)
		}
		if got := runGitTest(t, f.current.worktree, "rev-parse", "HEAD"); got != f.current.expected {
			t.Fatalf("refused release %q moved current head to %s", args, got)
		}
	}
}

func TestArchivedClaimReleaseInitializedSubmodule(t *testing.T) {
	f := newArchivedClaimFixtureWithSubmodule(t, true)
	if got := runGitTest(t, f.oldPath, "submodule", "status", "--recursive"); strings.HasPrefix(got, "-") || got == "" {
		t.Fatalf("archived worktree submodule is not initialized: %q", got)
	}
	application := archivedClaimCallerApp(f, io.Discard)
	if err := application.run(archivedClaimArgs(f, true)); err != nil {
		t.Fatalf("initialized submodule dry-run: %v", err)
	}
	if err := application.run(archivedClaimArgs(f, false)); err != nil {
		t.Fatalf("release clean initialized submodule: %v", err)
	}
	if got := runGitTest(t, f.current.primary, "for-each-ref", "--format=%(objectname)", "refs/heads/"+claimLocalBranch(333, f.oldRun)); got != f.oldHead {
		t.Fatalf("archived local branch changed to %s", got)
	}
	if got := runGitTest(t, f.current.primary, "ls-remote", "origin", "refs/heads/"+f.archive); !strings.HasPrefix(got, f.oldHead+"\t") {
		t.Fatalf("remote archive changed: %q", got)
	}
}

func TestArchivedClaimReleaseRejectsDirtyInitializedSubmodule(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*testing.T, string)
	}{
		{name: "tracked edit", alter: func(t *testing.T, path string) {
			writeFixtureFile(t, path, "README", "edited\n")
		}},
		{name: "untracked file", alter: func(t *testing.T, path string) {
			writeFixtureFile(t, path, "untracked", "keep\n")
		}},
		{name: "staged file", alter: func(t *testing.T, path string) {
			writeFixtureFile(t, path, "staged", "keep\n")
			runGitTest(t, path, "add", "staged")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newArchivedClaimFixtureWithSubmodule(t, true)
			test.alter(t, filepath.Join(f.oldPath, "modules/fixture"))
			before := runGitTest(t, f.current.primary, "worktree", "list", "--porcelain")
			if err := archivedClaimCallerApp(f, io.Discard).run(archivedClaimArgs(f, false)); err == nil || !strings.Contains(err.Error(), "dirty") {
				t.Fatalf("dirty initialized submodule refusal = %v, want dirty proof error", err)
			}
			if after := runGitTest(t, f.current.primary, "worktree", "list", "--porcelain"); after != before {
				t.Fatalf("dirty submodule refusal changed registrations: before %q, after %q", before, after)
			}
			if got := runGitTest(t, f.oldPath, "rev-parse", "HEAD"); got != f.oldHead {
				t.Fatalf("dirty submodule refusal moved archived head to %s", got)
			}
		})
	}
}
