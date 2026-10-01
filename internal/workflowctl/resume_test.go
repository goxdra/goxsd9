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

func TestPRResumeUsageExplainsIntegration(t *testing.T) {
	var output bytes.Buffer
	application := app{ctx: context.Background(), stdout: &output, stderr: &output}
	if err := application.run([]string{"--help"}); err != nil {
		t.Fatalf("global help: %v", err)
	}
	for _, want := range []string{"[--integrate]", "original expired SHA", "resolved, committed, and clean"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("global help missing %q", want)
		}
	}
	for _, args := range [][]string{{"pr", "resume"}, {"pr", "resume", "14", "--acknowledge-needs-human"}} {
		err := application.run(args)
		if err == nil {
			t.Fatalf("run(%q) succeeded, want usage", args)
		}
		for _, want := range []string{"[--integrate]", "original expired PR head", "resolved and clean"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("run(%q) usage missing %q: %v", args, want, err)
			}
		}
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

func TestPRResumeRejectsConflictingCanonicalClaimMarkersBeforeMutation(t *testing.T) {
	fixture := newResumeFixture(t)
	head := makeConflictingClaimMarkersResumeHead(t, &fixture)
	fixture.expected = head
	backend := newResumeBackend(t, fixture)
	localBefore := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	remoteBefore := resumeRemoteHead(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	err := application.run(append(resumeArgs(head), "--dry-run"))
	if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !strings.Contains(err.Error(), "conflicting canonical claim markers") {
		t.Fatalf("conflicting marker error = %v, disposition %d, want terminal conflict refusal", err, operationDispositionOf(err))
	}
	if backend.mutations != 0 {
		t.Fatalf("conflicting marker mutations = %d, want zero", backend.mutations)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != localBefore {
		t.Fatalf("conflicting marker moved local head from %s to %s", localBefore, got)
	}
	if got := resumeRemoteHead(t, fixture); got != remoteBefore {
		t.Fatalf("conflicting marker moved remote head from %s to %s", remoteBefore, got)
	}
}

func TestPRResumeMissingRemoteObjectPreservesTrackingRefsOnRejection(t *testing.T) {
	fixture := newResumeFixture(t)
	writeFixtureFile(t, fixture.seed, "remote-movement", "remote movement\n")
	runGitTest(t, fixture.seed, "add", "remote-movement")
	runGitTest(t, fixture.seed, "commit", "--no-gpg-sign", "-m", "remote movement")
	remoteHead := runGitTest(t, fixture.seed, "rev-parse", "HEAD")
	runGitTest(t, fixture.seed, "push", "--force", "origin", remoteHead+":refs/heads/agent/issue-14")
	trackingBefore := runGitTest(t, fixture.primary, "for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes/origin/agent/*")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	err := application.run(resumeArgs(fixture.expected))
	if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !strings.Contains(err.Error(), "canonical renewal proof") {
		t.Fatalf("missing remote object rejection = %v, disposition %d, want terminal moved-head refusal", err, operationDispositionOf(err))
	}
	trackingAfter := runGitTest(t, fixture.primary, "for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes/origin/agent/*")
	if trackingAfter != trackingBefore {
		t.Fatalf("missing remote object rejection changed tracking refs from %q to %q", trackingBefore, trackingAfter)
	}
	temporaryRef := "refs/workflowctl/remote-agent-proof/agent/issue-14"
	if output := runGitAllowFailure(t, fixture.primary, "show-ref", "--verify", temporaryRef); output != "" {
		t.Fatalf("temporary remote proof ref remains after rejection: %s", output)
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
		head := resumeRemoteHead(t, fixture)
		if got := runGitTest(t, fixture.worktree, "rev-parse", head+"^"); got != fixture.expected {
			t.Fatalf("renewal parent = %s, want %s", got, fixture.expected)
		}
		if got, want := runGitTest(t, fixture.worktree, "rev-parse", head+"^{tree}"), runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^{tree}"); got != want {
			t.Fatalf("renewal tree = %s, want parent tree %s", got, want)
		}
		message := runGitTest(t, fixture.worktree, "log", "-1", "--format=%B", head)
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
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
			t.Fatalf("remote renewal changed local HEAD to %s", got)
		}
		if !backend.needsHuman || backend.projectStatus != "Backlog" {
			t.Fatalf("pending renewal changed issue state: needs-human=%v Project=%s", backend.needsHuman, backend.projectStatus)
		}
	})

	t.Run("ambiguous push retries already-pushed child", func(t *testing.T) {
		fixture := newResumeFixture(t)
		backend := newResumeBackend(t, fixture)
		backend.ambiguousPush = true
		sentinel := errors.New("ambiguous push sentinel")
		backend.ambiguousPushCause = sentinel
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("ambiguous push reconciles exact marker: %v", err)
		}
		head := resumeRemoteHead(t, fixture)
		if head == fixture.expected {
			t.Fatalf("ambiguous push did not create a renewal child")
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
			t.Fatalf("ambiguous push moved local head to %s", got)
		}
		commits := countResumeCalls(backend.calls, "git commit-tree ")
		updates := countResumeCalls(backend.calls, "git update-ref ")
		pushes := countResumeCalls(backend.calls, "git push ")
		backend.ambiguousPush = false
		if err := application.run(resumeArgs(fixture.expected)); err != nil {
			t.Fatalf("retry resume: %v", err)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
			t.Fatalf("retry moved local head: %s", got)
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
		if got := resumeRemoteHead(t, fixture); got == child {
			t.Fatal("remote-only renewal published preexisting local child")
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

//nolint:gocognit // The fixture checks the same local artifacts across both recovery phases.
func TestPRResumePreservesLocalIntegrationBytes(t *testing.T) {
	for _, test := range []struct {
		name           string
		stage          func(*testing.T, resumeFixture)
		integrateError string
	}{
		{name: "resolved staged merge at remote head", stage: stageResumeMerge, integrateError: "MERGE_HEAD"},
		{name: "unpublished merge with staged and untracked work", stage: stageResumeUnpublishedMerge, integrateError: "staged, unstaged, or untracked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			test.stage(t, fixture)
			paths := resumePreservedPaths(t, fixture.worktree)
			before := snapshotResumeLocal(t, fixture.worktree, paths)
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
				if test.integrateError == "MERGE_HEAD" && name == "git" && len(args) == 3 &&
					args[0] == "rev-parse" && args[1] == "--git-path" && args[2] == "MERGE_HEAD" {
					absolute := runGitTest(t, fixture.worktree, "rev-parse", "--git-path", "MERGE_HEAD")
					if !filepath.IsAbs(absolute) {
						absolute = filepath.Join(fixture.worktree, absolute)
					}
					return filepath.Rel(fixture.worktree, absolute)
				}
				return backend.execute(dir, input, name, args...)
			}, stdout: io.Discard}
			if err := application.run(resumeArgs(fixture.expected)); err != nil {
				t.Fatalf("remote-only renewal: %v", err)
			}
			assertResumeSnapshot(t, fixture.worktree, paths, before)
			remote := resumeRemoteHead(t, fixture)
			if remote == before.head || runGitTest(t, fixture.worktree, "rev-parse", remote+"^") != fixture.expected {
				t.Fatalf("remote marker %s is not a new child of %s", remote, fixture.expected)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", remote+"^{tree}"); got != runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^{tree}") {
				t.Fatalf("remote marker tree %s differs from expected tree", got)
			}
			if !backend.needsHuman || backend.projectStatus != "Backlog" {
				t.Fatalf("pending state changed issue: label=%v Project=%s", backend.needsHuman, backend.projectStatus)
			}
			if err := application.run(append(resumeArgs(fixture.expected), "--integrate")); err == nil || !strings.Contains(err.Error(), test.integrateError) {
				t.Fatalf("dirty local integration error = %v, want %q", err, test.integrateError)
			}
			assertResumeSnapshot(t, fixture.worktree, paths, before)
			if err := application.renewClaim(); err == nil || !strings.Contains(err.Error(), "integrate a pending PR renewal") {
				t.Fatalf("claim renew during pending = %v", err)
			}
			if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err == nil || !strings.Contains(err.Error(), "integrate pending PR renewal") {
				t.Fatalf("claim push during pending = %v", err)
			}
			assertResumeSnapshot(t, fixture.worktree, paths, before)
		})
	}
}

func stageResumeMerge(t *testing.T, fixture resumeFixture) {
	t.Helper()
	runGitTest(t, fixture.worktree, "switch", "-c", "resume-side")
	writeFixtureFile(t, fixture.worktree, "side", "merged side\n")
	runGitTest(t, fixture.worktree, "add", "side")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "test: side")
	runGitTest(t, fixture.worktree, "switch", "agent/issue-14-"+fixture.runID)
	runGitTest(t, fixture.worktree, "merge", "--no-commit", "--no-ff", "resume-side")
	writeFixtureFile(t, fixture.worktree, "README", "base\nunstaged\n")
	writeFixtureFile(t, fixture.worktree, "untracked", "keep me\n")
}

func stageResumeUnpublishedMerge(t *testing.T, fixture resumeFixture) {
	t.Helper()
	stageResumeMerge(t, fixture)
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "test: local merge")
	writeFixtureFile(t, fixture.worktree, "staged", "keep staged\n")
	runGitTest(t, fixture.worktree, "add", "staged")
}

type resumeLocalSnapshot struct {
	head  string
	files [][]byte
}

func resumePreservedPaths(t *testing.T, root string) []string {
	t.Helper()
	paths := make([]string, 0, 6)
	paths = append(paths, "README", "side", "staged", "untracked")
	for _, name := range []string{"index", "MERGE_HEAD"} {
		path := runGitTest(t, root, "rev-parse", "--git-path", name)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		paths = append(paths, path)
	}
	return paths
}

func snapshotResumeLocal(t *testing.T, root string, paths []string) resumeLocalSnapshot {
	t.Helper()
	result := resumeLocalSnapshot{head: runGitTest(t, root, "rev-parse", "HEAD")}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		// #nosec G304 -- paths are fixed artifacts in this test's temporary Git worktree.
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("snapshot %s: %v", path, err)
		}
		result.files = append(result.files, data)
	}
	return result
}

func assertResumeSnapshot(t *testing.T, root string, paths []string, before resumeLocalSnapshot) {
	t.Helper()
	after := snapshotResumeLocal(t, root, paths)
	if after.head != before.head {
		t.Fatalf("local HEAD changed from %s to %s", before.head, after.head)
	}
	for index, path := range paths {
		if !bytes.Equal(after.files[index], before.files[index]) {
			t.Fatalf("local artifact %s changed during remote renewal", path)
		}
	}
}

func TestPRResumeFailedPreflightPreservesStagedMergeBytes(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, resumeFixture, *resumeBackend)
	}{
		{name: "moved PR head", edit: func(_ *testing.T, _ resumeFixture, backend *resumeBackend) {
			backend.prHead = strings.Repeat("a", 40)
		}},
		{name: "moved remote head", edit: func(t *testing.T, fixture resumeFixture, _ *resumeBackend) {
			moved := createResumeTestCommit(t, fixture.worktree, fixture.expected, "test: moved remote\n")
			runGitTest(t, fixture.worktree, "push", "--force", "origin", moved+":refs/heads/agent/issue-14")
		}},
		{name: "missing needs-human", edit: func(_ *testing.T, _ resumeFixture, backend *resumeBackend) {
			backend.needsHuman = false
		}},
		{name: "Project already Picked", edit: func(_ *testing.T, _ resumeFixture, backend *resumeBackend) {
			backend.projectStatus = "Picked"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			stageResumeMerge(t, fixture)
			backend := newResumeBackend(t, fixture)
			test.edit(t, fixture, backend)
			paths := resumePreservedPaths(t, fixture.worktree)
			before := snapshotResumeLocal(t, fixture.worktree, paths)
			remoteBefore := resumeRemoteHead(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(resumeArgs(fixture.expected)); err == nil {
				t.Fatal("preflight unexpectedly accepted inconsistent proof")
			}
			if backend.mutations != 0 {
				t.Fatalf("failed preflight issued %d mutations", backend.mutations)
			}
			assertResumeSnapshot(t, fixture.worktree, paths, before)
			if got := resumeRemoteHead(t, fixture); got != remoteBefore {
				t.Fatalf("failed preflight moved remote head from %s to %s", remoteBefore, got)
			}
		})
	}
}

func TestPRResumeRejectsConflictingUnpublishedRunBeforeMutation(t *testing.T) {
	fixture := newResumeFixture(t)
	other := createResumeTestCommit(t, fixture.worktree, fixture.expected,
		claimMessage(14, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, other, fixture.expected)
	backend := newResumeBackend(t, fixture)
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(resumeArgs(fixture.expected)); err == nil || !strings.Contains(err.Error(), "conflicting canonical claim markers") {
		t.Fatalf("conflicting unpublished run error = %v", err)
	}
	if backend.mutations != 0 {
		t.Fatalf("conflicting run preflight mutations = %d", backend.mutations)
	}
	assertResumeSnapshot(t, fixture.worktree, paths, before)
	if got := resumeRemoteHead(t, fixture); got != fixture.expected {
		t.Fatalf("conflicting run moved remote head to %s", got)
	}
}

func TestPRResumeCASRacePreservesLocalIntegration(t *testing.T) {
	fixture := newResumeFixture(t)
	stageResumeUnpublishedMerge(t, fixture)
	backend := newResumeBackend(t, fixture)
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	rogue := createResumeTestCommit(t, fixture.worktree, fixture.expected, "test: moved PR head\n")
	interposed := false
	application := app{ctx: context.Background(), executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if name == "git" && len(args) > 0 && args[0] == "push" && !interposed {
			interposed = true
			runGitTest(t, fixture.worktree, "push", "--force", "origin", rogue+":refs/heads/agent/issue-14")
		}
		return backend.execute(dir, input, name, args...)
	}, stdout: io.Discard}
	err := application.run(resumeArgs(fixture.expected))
	if err == nil || !strings.Contains(err.Error(), "reconciliation") {
		t.Fatalf("CAS race error = %v", err)
	}
	if !interposed {
		t.Fatal("CAS push was not attempted")
	}
	assertResumeSnapshot(t, fixture.worktree, paths, before)
	if got := resumeRemoteHead(t, fixture); got != rogue {
		t.Fatalf("CAS push displaced competing remote head: %s", got)
	}
}

//nolint:gocognit,funlen // The published integration regression asserts each durable claim boundary.
func TestPRResumeIntegratesMarkerAfterLocalWorkResolves(t *testing.T) {
	fixture := newResumeFixture(t)
	stageResumeUnpublishedMerge(t, fixture)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(resumeArgs(fixture.expected)); err != nil {
		t.Fatalf("remote renewal: %v", err)
	}
	remote := resumeRemoteHead(t, fixture)
	before := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if err := application.run(append(resumeArgs(fixture.expected), "--integrate")); err == nil || !strings.Contains(err.Error(), "staged, unstaged, or untracked") {
		t.Fatalf("dirty integration error = %v", err)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != before {
		t.Fatalf("failed integration moved local head to %s", got)
	}
	runGitTest(t, fixture.worktree, "add", "-A")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "test: finish local work")
	local := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	localTree := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}")
	if err := application.run(append(resumeArgs(fixture.expected), "--integrate")); err != nil {
		t.Fatalf("integrate remote marker: %v", err)
	}
	integrated := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := runGitTest(t, fixture.worktree, "rev-parse", integrated+"^1"); got != local {
		t.Fatalf("integration first parent = %s, want local %s", got, local)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", integrated+"^2"); got != remote {
		t.Fatalf("integration second parent = %s, want remote marker %s", got, remote)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}"); got != localTree {
		t.Fatalf("integration changed local tree from %s to %s", localTree, got)
	}
	if backend.needsHuman || backend.projectStatus != "Picked" {
		t.Fatalf("integration did not complete issue state: label=%v Project=%s", backend.needsHuman, backend.projectStatus)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("integrated claim cannot pass push guard: %v", err)
	}
	if got := resumeRemoteHead(t, fixture); got != remote {
		t.Fatalf("integration published local source: remote=%s want=%s", got, remote)
	}
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/agent/issue-14")
	if got := resumeRemoteHead(t, fixture); got != integrated {
		t.Fatalf("published integration head = %s, want %s", got, integrated)
	}
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("published integration claim verification: %v", err)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("published integration push verification: %v", err)
	}
	writeFixtureFile(t, fixture.worktree, "after-integration", "later source work\n")
	runGitTest(t, fixture.worktree, "add", "after-integration")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "test: later source work")
	later := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("later source claim verification: %v", err)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("later source push verification: %v", err)
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("published integration claim renewal: %v", err)
	}
	renewed := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := runGitTest(t, fixture.worktree, "rev-parse", renewed+"^1"); got != later {
		t.Fatalf("post-integration renewal parent = %s, want %s", got, later)
	}
	if got := resumeRemoteHead(t, fixture); got != renewed {
		t.Fatalf("post-integration renewal remote = %s, want %s", got, renewed)
	}
}

func TestPRResumePublishedIntegrationWithExpectedHeadOnLocalSideParent(t *testing.T) {
	fixture := newResumeFixture(t)
	base := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
	source := createResumeTestCommit(t, fixture.worktree, base, "test: local source branch\n")
	tree := runGitTest(t, fixture.worktree, "rev-parse", source+"^{tree}")
	local := createResumeCommitTree(t, fixture.worktree, tree, []string{source, fixture.expected}, "Merge preserved local work\n")
	runGitTest(t, fixture.worktree, "reset", "--hard", local)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(resumeArgs(fixture.expected)); err != nil {
		t.Fatalf("remote renewal with side-parent expected head: %v", err)
	}
	marker := resumeRemoteHead(t, fixture)
	if err := application.run(append(resumeArgs(fixture.expected), "--integrate")); err != nil {
		t.Fatalf("integrate renewal with side-parent expected head: %v", err)
	}
	integrated := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := runGitTest(t, fixture.worktree, "rev-parse", integrated+"^1"); got != local {
		t.Fatalf("integration source parent = %s, want %s", got, local)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", integrated+"^2"); got != marker {
		t.Fatalf("integration renewal parent = %s, want %s", got, marker)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("side-parent integration push guard: %v", err)
	}
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/agent/issue-14")
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("published side-parent integration verification: %v", err)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("published side-parent integration push guard: %v", err)
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("published side-parent integration renewal: %v", err)
	}
	if got := resumeRemoteHead(t, fixture); got != runGitTest(t, fixture.worktree, "rev-parse", "HEAD") {
		t.Fatalf("side-parent renewal remote = %s, want local HEAD", got)
	}
}

func TestPublishedClaimIntegrationRejectsUntrustedAdoption(t *testing.T) {
	tests := []struct {
		name string
		make func(*testing.T, resumeFixture, string, string, string) string
		want string
	}{
		{name: "wrong issue", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			other := createResumeTestCommit(t, f.worktree, source, claimMessage(15, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{source, other}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "renews issue #15"},
		{name: "wrong run", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			other := createResumeTestCommit(t, f.worktree, source, claimMessage(14, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{source, other}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "conflicts with original run"},
		{name: "renewal parent outside source ancestry", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			base := runGitTest(t, f.worktree, "rev-parse", source+"^")
			sibling := createResumeTestCommit(t, f.worktree, base, "test: sibling\n")
			other := createResumeTestCommit(t, f.worktree, sibling, claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{source, other}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "outside local source ancestry"},
		{name: "source-bearing renewal", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			otherTree := resumeChangedTree(t, f)
			other := createResumeCommitTree(t, f.worktree, otherTree, []string{source},
				claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{source, other}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "source-bearing"},
		{name: "integration changes source tree", make: func(t *testing.T, f resumeFixture, source, renewal, _ string) string {
			return createResumeCommitTree(t, f.worktree, resumeChangedTree(t, f), []string{source, renewal}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "changes its source parent's tree"},
		{name: "extra integration parent", make: func(t *testing.T, f resumeFixture, source, renewal, tree string) string {
			base := runGitTest(t, f.worktree, "rev-parse", source+"^")
			return createResumeCommitTree(t, f.worktree, tree, []string{source, renewal, base}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "non-canonical commit shape"},
		{name: "misleading integration message", make: func(t *testing.T, f resumeFixture, source, renewal, tree string) string {
			return createResumeCommitTree(t, f.worktree, tree, []string{source, renewal}, "chore(workflow): integrate claim renewal #14\n\nextra\n")
		}, want: "non-canonical message"},
		{name: "conflicting source marker", make: func(t *testing.T, f resumeFixture, source, renewal, tree string) string {
			conflict := createResumeTestCommit(t, f.worktree, source, claimMessage(14, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{conflict, renewal}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "conflicting first-parent marker"},
		{name: "source conflict also reachable from expected side", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			conflict := createResumeTestCommit(t, f.worktree, source, claimMessage(14, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			expected := createResumeCommitTree(t, f.worktree, tree, []string{source, conflict}, "Merge expected PR head\n")
			local := createResumeCommitTree(t, f.worktree, tree, []string{conflict, expected}, "Merge local work\n")
			renewed := createResumeTestCommit(t, f.worktree, expected,
				claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{local, renewed}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "conflicting first-parent marker"},
		{name: "malformed shared integration", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			bad := createResumeCommitTree(t, f.worktree, tree, []string{source}, "chore(workflow): integrate claim renewal #14\n")
			expected := createResumeTestCommit(t, f.worktree, bad,
				claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			local := createResumeCommitTree(t, f.worktree, tree, []string{bad, expected}, "Merge local work\n")
			renewed := createResumeTestCommit(t, f.worktree, expected,
				claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{local, renewed}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "non-canonical commit shape"},
		{name: "foreign-run shared integration", make: func(t *testing.T, f resumeFixture, source, _, tree string) string {
			foreign := createResumeTestCommit(t, f.worktree, source,
				claimMessage(14, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			bad := createResumeCommitTree(t, f.worktree, tree, []string{source, foreign}, "chore(workflow): integrate claim renewal #14\n")
			expected := createResumeTestCommit(t, f.worktree, bad,
				claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			local := createResumeCommitTree(t, f.worktree, tree, []string{bad, expected}, "Merge local work\n")
			renewed := createResumeTestCommit(t, f.worktree, expected,
				claimMessage(14, f.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			return createResumeCommitTree(t, f.worktree, tree, []string{local, renewed}, "chore(workflow): integrate claim renewal #14\n")
		}, want: "conflicts with original run"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			source := fixture.expected
			tree := runGitTest(t, fixture.worktree, "rev-parse", source+"^{tree}")
			renewal := createResumeTestCommit(t, fixture.worktree, source,
				claimMessage(14, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			head := test.make(t, fixture, source, renewal, tree)
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if _, err := application.readAuthoritativeClaimMarker(fixture.worktree, head, 14); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("untrusted integration authority error = %v, want %q", err, test.want)
			}
		})
	}
}

func resumeChangedTree(t *testing.T, fixture resumeFixture) string {
	t.Helper()
	writeFixtureFile(t, fixture.worktree, "changed-tree", "different tree\n")
	runGitTest(t, fixture.worktree, "add", "changed-tree")
	tree := runGitTest(t, fixture.worktree, "write-tree")
	runGitTest(t, fixture.worktree, "reset", "--hard", fixture.expected)
	return tree
}

//nolint:gocognit // Each gate must reject the same malformed unpublished integration.
func TestUnpublishedClaimIntegrationRejectsUntrustedAdoptionBeforePush(t *testing.T) {
	for _, test := range []struct {
		name    string
		message string
		runID   string
		want    string
	}{
		{name: "wrong run", message: "chore(workflow): integrate claim renewal #14\n", runID: "run-other", want: "conflicts with original run"},
		{name: "misleading message", message: "chore(workflow): integrate claim renewal #14\n\nextra\n", runID: "run-resume-test", want: "non-canonical message"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newActiveResumeClaimFixture(t)
			source := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			tree := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}")
			renewal := createResumeTestCommit(t, fixture.worktree, source,
				claimMessage(14, test.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			integration := createResumeCommitTree(t, fixture.worktree, tree, []string{source, renewal}, test.message)
			runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, integration, source)
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			for _, gate := range []struct {
				name string
				run  func() error
			}{
				{name: "verify", run: application.verifyClaim},
				{name: "push guard", run: func() error {
					return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
				}},
				{name: "renew", run: application.renewClaim},
			} {
				if err := gate.run(); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s accepted untrusted unpublished integration: %v, want %q", gate.name, err, test.want)
				}
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != integration {
				t.Fatalf("rejected integration moved local head to %s", got)
			}
			if got := resumeRemoteHead(t, fixture); got != source {
				t.Fatalf("rejected integration moved remote head to %s", got)
			}
			if backend.mutations != 0 {
				t.Fatalf("rejected integration performed %d mutations", backend.mutations)
			}
		})
	}
}

//nolint:gocognit // Each gate must reject older and mismatched unpublished markers.
func TestUnpublishedClaimIntegrationCannotRegressPublishedLease(t *testing.T) {
	for _, test := range []struct {
		name  string
		lease time.Duration
		want  string
	}{
		{name: "older expired marker", lease: -time.Hour, want: "renewal lease regresses proven authority"},
		{name: "different newer marker", lease: 2 * time.Hour, want: "not remote authoritative marker"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newActiveResumeClaimFixture(t)
			remote := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			original := fixture.expected
			other := createResumeTestCommit(t, fixture.worktree, original,
				claimMessage(14, fixture.runID, time.Now().UTC().Add(test.lease).Truncate(time.Second)))
			tree := runGitTest(t, fixture.worktree, "rev-parse", remote+"^{tree}")
			integration := createResumeCommitTree(t, fixture.worktree, tree, []string{remote, other}, "chore(workflow): integrate claim renewal #14\n")
			runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, integration, remote)
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			for _, gate := range []struct {
				name string
				run  func() error
			}{
				{name: "verify", run: application.verifyClaim},
				{name: "push guard", run: func() error {
					return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
				}},
				{name: "renew", run: application.renewClaim},
			} {
				if err := gate.run(); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s accepted %s integration: %v, want %q", gate.name, test.name, err, test.want)
				}
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != integration {
				t.Fatalf("rejected integration moved local head to %s", got)
			}
			if got := resumeRemoteHead(t, fixture); got != remote {
				t.Fatalf("rejected integration moved remote head to %s", got)
			}
			if backend.mutations != 0 {
				t.Fatalf("rejected integration performed %d mutations", backend.mutations)
			}
		})
	}
}

func TestUnpublishedClaimMarkerCannotRegressPublishedLease(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	remote := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	local := createResumeTestCommit(t, fixture.worktree, remote,
		claimMessage(14, fixture.runID, time.Now().UTC().Add(-time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, local, remote)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	for _, gate := range []struct {
		name string
		run  func() error
	}{
		{name: "verify", run: application.verifyClaim},
		{name: "push guard", run: func() error {
			return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
		}},
		{name: "renew", run: application.renewClaim},
	} {
		if err := gate.run(); err == nil || !strings.Contains(err.Error(), "regresses remote") {
			t.Fatalf("%s accepted expired unpublished marker: %v", gate.name, err)
		}
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != local {
		t.Fatalf("rejected local marker moved head to %s", got)
	}
	if got := resumeRemoteHead(t, fixture); got != remote {
		t.Fatalf("rejected local marker moved remote to %s", got)
	}
	if backend.mutations != 0 {
		t.Fatalf("rejected local marker performed %d mutations", backend.mutations)
	}
}

func TestPublishedClaimIntegrationAuthorityHasBoundedRepeatedProof(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	for range 7 {
		tree := runGitTest(t, fixture.worktree, "rev-parse", head+"^{tree}")
		renewal := createResumeTestCommit(t, fixture.worktree, head,
			claimMessage(14, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
		head = createResumeCommitTree(t, fixture.worktree, tree, []string{head, renewal},
			"chore(workflow): integrate claim renewal #14\n")
	}
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	marker, err := application.readAuthoritativeClaimMarker(fixture.worktree, head, 14)
	if err != nil {
		t.Fatalf("repeated integration authority: %v", err)
	}
	if marker.runID != fixture.runID {
		t.Fatalf("repeated integration authority run = %s, want %s", marker.runID, fixture.runID)
	}
	if len(backend.calls) > 500 {
		t.Fatalf("repeated integration proof used %d Git calls, want bounded growth", len(backend.calls))
	}
}

//nolint:gocognit // Both public claim gates must reject the same published lease regression without mutation.
func TestPublishedClaimIntegrationCannotRegressSourceLease(t *testing.T) {
	for _, test := range []struct {
		name         string
		renewalAfter time.Duration
		wantError    bool
	}{
		{name: "older renewal", renewalAfter: 2 * time.Hour, wantError: true},
		{name: "later renewal", renewalAfter: 4 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResumeFixture(t)
			original := fixture.expected
			lease := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
			source := createResumeTestCommit(t, fixture.worktree, original,
				claimMessage(14, fixture.runID, lease))
			renewal := createResumeTestCommit(t, fixture.worktree, original,
				claimMessage(14, fixture.runID, time.Now().UTC().Add(test.renewalAfter).Truncate(time.Second)))
			tree := runGitTest(t, fixture.worktree, "rev-parse", source+"^{tree}")
			integration := createResumeCommitTree(t, fixture.worktree, tree, []string{source, renewal},
				"chore(workflow): integrate claim renewal #14\n")
			runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, integration, original)
			runGitTest(t, fixture.primary, "push", "origin", integration+":refs/heads/agent/issue-14")
			if got := resumeRemoteHead(t, fixture); got != integration {
				t.Fatalf("published integration = %s, want %s", got, integration)
			}
			writeFixtureFile(t, fixture.worktree, "staged", "preserved staged work\n")
			runGitTest(t, fixture.worktree, "add", "staged")
			writeFixtureFile(t, fixture.worktree, "untracked", "preserved untracked work\n")
			paths := resumePreservedPaths(t, fixture.worktree)
			before := snapshotResumeLocal(t, fixture.worktree, paths)
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			for _, gate := range []struct {
				name string
				run  func() error
			}{
				{name: "verify", run: application.verifyClaim},
				{name: "push guard", run: func() error {
					return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
				}},
			} {
				err := gate.run()
				if test.wantError {
					if err == nil || !strings.Contains(err.Error(), "renewal lease regresses proven authority") {
						t.Fatalf("%s accepted published lease regression: %v", gate.name, err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s rejected later renewal: %v", gate.name, err)
				}
			}
			assertResumeSnapshot(t, fixture.worktree, paths, before)
			if got := resumeRemoteHead(t, fixture); got != integration {
				t.Fatalf("claim gates moved remote head to %s, want %s", got, integration)
			}
			if backend.mutations != 0 {
				t.Fatalf("claim gates performed %d mutations", backend.mutations)
			}
		})
	}
}

func TestPublishedClaimIntegrationCannotRegressRenewalBaseLease(t *testing.T) {
	fixture := newResumeFixture(t)
	original := fixture.expected
	base := createResumeTestCommit(t, fixture.worktree, original,
		claimMessage(14, fixture.runID, time.Now().UTC().Add(3*time.Hour).Truncate(time.Second)))
	tree := runGitTest(t, fixture.worktree, "rev-parse", original+"^{tree}")
	source := createResumeCommitTree(t, fixture.worktree, tree, []string{original, base}, "Merge source work\n")
	renewal := createResumeTestCommit(t, fixture.worktree, base,
		claimMessage(14, fixture.runID, time.Now().UTC().Add(2*time.Hour).Truncate(time.Second)))
	integration := createResumeCommitTree(t, fixture.worktree, tree, []string{source, renewal},
		"chore(workflow): integrate claim renewal #14\n")
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, integration, original)
	runGitTest(t, fixture.primary, "push", "origin", integration+":refs/heads/agent/issue-14")
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	for _, gate := range []struct {
		name string
		run  func() error
	}{
		{name: "verify", run: application.verifyClaim},
		{name: "push guard", run: func() error {
			return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
		}},
	} {
		if err := gate.run(); err == nil || !strings.Contains(err.Error(), "renewal lease regresses proven authority") {
			t.Fatalf("%s accepted renewal below its proven base lease: %v", gate.name, err)
		}
	}
	assertResumeSnapshot(t, fixture.worktree, paths, before)
	if got := resumeRemoteHead(t, fixture); got != integration {
		t.Fatalf("claim gates moved remote head to %s, want %s", got, integration)
	}
	if backend.mutations != 0 {
		t.Fatalf("claim gates performed %d mutations", backend.mutations)
	}
}

//nolint:gocognit // The same shared-boundary fault must fail before and after an adversarial push.
func TestClaimIntegrationRejectsUnprovedSharedBoundaryBeforeAndAfterPush(t *testing.T) {
	for _, test := range []struct {
		name    string
		foreign bool
		want    string
	}{
		{name: "malformed integration", want: "non-canonical commit shape"},
		{name: "foreign-run integration", foreign: true, want: "conflicts with original run"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newActiveResumeClaimFixture(t)
			active := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			tree := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}")
			parents := []string{active}
			if test.foreign {
				foreign := createResumeTestCommit(t, fixture.worktree, active,
					claimMessage(14, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
				parents = append(parents, foreign)
			}
			bad := createResumeCommitTree(t, fixture.worktree, tree, parents, "chore(workflow): integrate claim renewal #14\n")
			expected := createResumeTestCommit(t, fixture.worktree, bad,
				claimMessage(14, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			localSource := createResumeCommitTree(t, fixture.worktree, tree, []string{bad, expected}, "Merge local work\n")
			renewal := createResumeTestCommit(t, fixture.worktree, expected,
				claimMessage(14, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			integration := createResumeCommitTree(t, fixture.worktree, tree, []string{localSource, renewal},
				"chore(workflow): integrate claim renewal #14\n")
			runGitTest(t, fixture.primary, "push", "origin", renewal+":refs/heads/agent/issue-14")
			runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, integration, active)
			backend := newResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			for _, phase := range []string{"before push", "after push"} {
				for _, gate := range []struct {
					name string
					run  func() error
				}{
					{name: "verify", run: application.verifyClaim},
					{name: "push guard", run: func() error {
						return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
					}},
					{name: "renew", run: application.renewClaim},
				} {
					if err := gate.run(); err == nil || !strings.Contains(err.Error(), test.want) {
						t.Fatalf("%s %s accepted unproved boundary: %v, want %q", phase, gate.name, err, test.want)
					}
				}
				if backend.mutations != 0 {
					t.Fatalf("%s workflow gates performed %d mutations", phase, backend.mutations)
				}
				if phase == "before push" {
					runGitTest(t, fixture.worktree, "push", "origin", integration+":refs/heads/agent/issue-14")
				}
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != integration {
				t.Fatalf("rejected integration moved local head to %s", got)
			}
			if got := resumeRemoteHead(t, fixture); got != integration {
				t.Fatalf("adversarial publication head = %s, want %s", got, integration)
			}
		})
	}
}

func TestPRResumeStatusFailureReconcilesWithoutSecondMarker(t *testing.T) {
	for _, test := range []struct {
		name string
		fail func(*resumeBackend, error)
	}{
		{name: "label mutation", fail: func(b *resumeBackend, err error) { b.labelFailure = err }},
		{name: "Project mutation", fail: func(b *resumeBackend, err error) { b.projectFailure = err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runResumeReconciliationRetryScenario(t, test.name, test.fail)
		})
	}
}

func runResumeReconciliationRetryScenario(t *testing.T, name string, fail func(*resumeBackend, error)) {
	t.Helper()
	fixture := newResumeFixture(t)
	backend := newResumeBackend(t, fixture)
	var output bytes.Buffer
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: &output}
	if runErr := application.run(resumeArgs(fixture.expected)); runErr != nil {
		t.Fatalf("remote renewal: %v", runErr)
	}
	marker := resumeRemoteHead(t, fixture)
	sentinel := errors.New(name + " failed")
	fail(backend, sentinel)
	err := application.run(append(resumeArgs(fixture.expected), "--integrate"))
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("%s failure = %v, want preserved cause", name, err)
	}
	integrated := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if backend.projectStatus != "Backlog" {
		t.Fatalf("partial reconciliation Project=%s", backend.projectStatus)
	}
	commits := countResumeCalls(backend.calls, "git commit-tree ")
	pushes := countResumeCalls(backend.calls, "git push ")
	assertResumeIntegratedStatusMessage(t, &application, &output, fixture.expected)
	retry := resumeRetryArgsFromError(t, err)
	if got, want := strings.Join(retry, " "), strings.Join(append(resumeArgs(fixture.expected), "--integrate"), " "); got != want {
		t.Fatalf("emitted retry = %q, want %q", got, want)
	}
	if runErr := application.run(retry); runErr != nil {
		t.Fatalf("emitted reconciliation retry: %v", runErr)
	}
	assertResumeRetryDidNotRepeat(t, fixture, backend, integrated, marker, commits, pushes)
}

func assertResumeIntegratedStatusMessage(t *testing.T, application *app, output *bytes.Buffer, expected string) {
	t.Helper()
	output.Reset()
	if err := application.run(resumeArgs(expected)); err != nil {
		t.Fatalf("remote-only retry: %v", err)
	}
	got := output.String()
	if !strings.Contains(got, "local renewal marker is integrated") || !strings.Contains(got, "status reconciliation pending") || strings.Contains(got, "local integration pending") {
		t.Fatalf("remote-only retry reported wrong phase: %q", got)
	}
}

func assertResumeRetryDidNotRepeat(t *testing.T, fixture resumeFixture, backend *resumeBackend, integrated, marker string, commits, pushes int) {
	t.Helper()
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != integrated {
		t.Fatalf("retry created another integration commit: %s", got)
	}
	if got := resumeRemoteHead(t, fixture); got != marker {
		t.Fatalf("retry created another remote marker: %s", got)
	}
	if got := countResumeCalls(backend.calls, "git commit-tree "); got != commits {
		t.Fatalf("retry created another commit: %d versus %d", got, commits)
	}
	if got := countResumeCalls(backend.calls, "git push "); got != pushes {
		t.Fatalf("retry pushed again: %d versus %d", got, pushes)
	}
	if backend.needsHuman || backend.projectStatus != "Picked" {
		t.Fatalf("retry did not finish reconciliation: label=%v Project=%s", backend.needsHuman, backend.projectStatus)
	}
}

func resumeRetryArgsFromError(t *testing.T, err error) []string {
	t.Helper()
	const prefix = "run `go tool workflowctl "
	message := err.Error()
	start := strings.Index(message, prefix)
	if start < 0 {
		t.Fatalf("no retry command in %q", message)
	}
	command := message[start+len(prefix):]
	end := strings.IndexByte(command, '`')
	if end < 0 {
		t.Fatalf("unterminated retry command in %q", message)
	}
	return strings.Fields(command[:end])
}

func TestPRResumePushGuardRejectsUnfinishedMergeAfterIntegration(t *testing.T) {
	fixture := newResumeFixture(t)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(resumeArgs(fixture.expected)); err != nil {
		t.Fatalf("remote renewal: %v", err)
	}
	if err := application.run(append(resumeArgs(fixture.expected), "--integrate")); err != nil {
		t.Fatalf("integrate renewal: %v", err)
	}
	marker := resumeRemoteHead(t, fixture)
	runGitTest(t, fixture.worktree, "switch", "-c", "push-side")
	writeFixtureFile(t, fixture.worktree, "push-side", "side work\n")
	runGitTest(t, fixture.worktree, "add", "push-side")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "test: side work")
	runGitTest(t, fixture.worktree, "switch", "agent/issue-14-"+fixture.runID)
	runGitTest(t, fixture.worktree, "merge", "--no-commit", "--no-ff", "push-side")
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
	if err == nil || !strings.Contains(err.Error(), "MERGE_HEAD") {
		t.Fatalf("push guard during unfinished merge = %v", err)
	}
	assertResumeSnapshot(t, fixture.worktree, paths, before)
	if got := resumeRemoteHead(t, fixture); got != marker {
		t.Fatalf("push guard changed remote marker to %s", got)
	}
}

func TestClaimRenewPreservesOrdinaryLocalAheadWork(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	writeFixtureFile(t, fixture.worktree, "source", "ordinary unpublished source\n")
	runGitTest(t, fixture.worktree, "add", "source")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "test: ordinary local source")
	source := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	writeFixtureFile(t, fixture.worktree, "staged", "preserved staged work\n")
	runGitTest(t, fixture.worktree, "add", "staged")
	writeFixtureFile(t, fixture.worktree, "untracked", "preserved untracked work\n")
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("verify ordinary local-ahead claim: %v", err)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("verify ordinary local-ahead push: %v", err)
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renew ordinary local-ahead claim: %v", err)
	}
	marker := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := resumeRemoteHead(t, fixture); got != marker {
		t.Fatalf("renewed fixed branch = %s, want local marker %s", got, marker)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", marker+"^"); got != source {
		t.Fatalf("renewal parent = %s, want unpublished source %s", got, source)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", marker+"^{tree}"); got != runGitTest(t, fixture.worktree, "rev-parse", source+"^{tree}") {
		t.Fatalf("renewal changed source tree to %s", got)
	}
	after := snapshotResumeLocal(t, fixture.worktree, paths)
	for index, path := range paths {
		if !bytes.Equal(after.files[index], before.files[index]) {
			t.Fatalf("ordinary renewal changed local artifact %s", path)
		}
	}
}

func TestLocalAheadClaimMarkersRejectConflictingIdentityWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		markers []struct {
			issue int
			runID string
		}
	}{
		{name: "wrong run", markers: []struct {
			issue int
			runID string
		}{{14, "run-other"}}},
		{name: "wrong issue", markers: []struct {
			issue int
			runID string
		}{{15, "run-resume-test"}}},
		{name: "hidden wrong run", markers: []struct {
			issue int
			runID string
		}{{14, "run-other"}, {14, "run-resume-test"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertConflictingLocalClaimRejected(t, test.name, test.markers)
		})
	}
}

func TestLocalAheadClaimIgnoresForeignMarkerOnMergedSideParent(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	active := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	foreign := createResumeTestCommit(t, fixture.worktree, active,
		claimMessage(15, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "branch", "upstream-marker", foreign)
	runGitTest(t, fixture.worktree, "merge", "--no-ff", "--no-edit", "upstream-marker")
	merge := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("verify claim with upstream marker side parent: %v", err)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("verify push with upstream marker side parent: %v", err)
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renew claim with upstream marker side parent: %v", err)
	}
	renewed := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := runGitTest(t, fixture.worktree, "rev-parse", renewed+"^1"); got != merge {
		t.Fatalf("renewal parent = %s, want local merge %s", got, merge)
	}
	if got := resumeRemoteHead(t, fixture); got != renewed {
		t.Fatalf("remote renewal = %s, want %s", got, renewed)
	}
}

func TestPublishedClaimMergeIgnoresForeignMarkerOnSideParent(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	active := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	foreign := createResumeTestCommit(t, fixture.worktree, active,
		claimMessage(15, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "branch", "upstream-marker", foreign)
	runGitTest(t, fixture.worktree, "merge", "--no-ff", "--no-edit", "upstream-marker")
	merge := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	runGitTest(t, fixture.primary, "push", "origin", merge+":refs/heads/agent/issue-14")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("verify published merge with upstream marker: %v", err)
	}
	if err := application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14); err != nil {
		t.Fatalf("verify published merge push: %v", err)
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renew published merge with upstream marker: %v", err)
	}
	renewed := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if got := runGitTest(t, fixture.worktree, "rev-parse", renewed+"^1"); got != merge {
		t.Fatalf("renewal parent = %s, want published merge %s", got, merge)
	}
	if got := resumeRemoteHead(t, fixture); got != renewed {
		t.Fatalf("remote renewal = %s, want %s", got, renewed)
	}
}

func TestPublishedClaimRejectsWrongIssueMarkerBeforeOlderValidMarker(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	active := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	wrong := createResumeTestCommit(t, fixture.worktree, active,
		claimMessage(15, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, wrong, active)
	runGitTest(t, fixture.primary, "push", "origin", wrong+":refs/heads/agent/issue-14")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	for _, gate := range []struct {
		name string
		run  func() error
	}{
		{name: "verify", run: application.verifyClaim},
		{name: "push guard", run: func() error {
			return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
		}},
		{name: "renew", run: application.renewClaim},
	} {
		if err := gate.run(); err == nil || !strings.Contains(err.Error(), "binds issue #15, not claim issue #14") {
			t.Fatalf("%s accepted wrong-issue remote marker: %v", gate.name, err)
		}
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != wrong {
		t.Fatalf("rejected renewal moved local head to %s", got)
	}
	if got := resumeRemoteHead(t, fixture); got != wrong {
		t.Fatalf("rejected renewal moved remote head to %s", got)
	}
}

func TestPublishedClaimFindsMarkerBeyondOneHundredSourceCommits(t *testing.T) {
	fixture := newActiveResumeClaimFixture(t)
	active := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	head := active
	for range 101 {
		head = createResumeTestCommit(t, fixture.worktree, head, "test: ordinary source commit\n")
	}
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, head, active)
	runGitTest(t, fixture.primary, "push", "origin", head+":refs/heads/agent/issue-14")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("verify claim beyond 100 ordinary commits: %v", err)
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renew claim beyond 100 ordinary commits: %v", err)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^1"); got != head {
		t.Fatalf("renewal parent = %s, want %s", got, head)
	}
}

func TestLocalAheadClaimMarkerCannotOverrideExpiredRemoteLease(t *testing.T) {
	fixture := newResumeFixture(t)
	local := createResumeTestCommit(t, fixture.worktree, fixture.expected,
		claimMessage(14, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, local, fixture.expected)
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	remote := resumeRemoteHead(t, fixture)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	for _, gate := range []struct {
		name string
		run  func() error
	}{
		{name: "verify", run: application.verifyClaim},
		{name: "push guard", run: func() error {
			return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
		}},
		{name: "renew", run: application.renewClaim},
	} {
		if err := gate.run(); err == nil || !strings.Contains(err.Error(), "expired") {
			t.Fatalf("%s accepted local lease over expired remote: %v", gate.name, err)
		}
	}
	assertResumeSnapshot(t, fixture.worktree, paths, before)
	if got := resumeRemoteHead(t, fixture); got != remote {
		t.Fatalf("expired renewal moved remote head from %s to %s", remote, got)
	}
}

func assertConflictingLocalClaimRejected(t *testing.T, name string, markers []struct {
	issue int
	runID string
}) {
	t.Helper()
	fixture := newActiveResumeClaimFixture(t)
	for _, marker := range markers {
		parent := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
		commit := createResumeTestCommit(t, fixture.worktree, parent,
			claimMessage(marker.issue, marker.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
		runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, commit, parent)
	}
	writeFixtureFile(t, fixture.worktree, "staged", "preserved staged work\n")
	runGitTest(t, fixture.worktree, "add", "staged")
	writeFixtureFile(t, fixture.worktree, "untracked", "preserved untracked work\n")
	paths := resumePreservedPaths(t, fixture.worktree)
	before := snapshotResumeLocal(t, fixture.worktree, paths)
	remote := resumeRemoteHead(t, fixture)
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	for _, gate := range []struct {
		name string
		run  func() error
	}{
		{name: "verify", run: application.verifyClaim},
		{name: "push guard", run: func() error {
			return application.verifyClaimForPush(fixture.worktree, "agent/issue-14-"+fixture.runID, 14)
		}},
		{name: "renew", run: application.renewClaim},
	} {
		if err := gate.run(); err == nil || !strings.Contains(err.Error(), "unpublished claim marker") {
			t.Fatalf("%s accepted %s marker: %v", gate.name, name, err)
		}
	}
	assertResumeSnapshot(t, fixture.worktree, paths, before)
	if got := resumeRemoteHead(t, fixture); got != remote {
		t.Fatalf("rejected renewal moved remote head from %s to %s", remote, got)
	}
	if got := countResumeCalls(backend.calls, "git commit-tree ") + countResumeCalls(backend.calls, "git update-ref ") + countResumeCalls(backend.calls, "git push "); got != 0 {
		t.Fatalf("rejected claim proof performed %d commit/ref/push mutations", got)
	}
}

func newActiveResumeClaimFixture(t *testing.T) resumeFixture {
	t.Helper()
	fixture := newResumeFixture(t)
	active := createResumeTestCommit(t, fixture.worktree, fixture.expected,
		claimMessage(14, fixture.runID, time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
	runGitTest(t, fixture.worktree, "update-ref", "refs/heads/agent/issue-14-"+fixture.runID, active, fixture.expected)
	runGitTest(t, fixture.primary, "push", "origin", active+":refs/heads/agent/issue-14")
	return fixture
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

//nolint:gocognit // Table cases keep mutation counts and state assertions aligned.
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
			wantMutations: 2,
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
	if backend.mutations != 2 {
		t.Fatalf("terminal PR verification mutations = %d, want commit/push only", backend.mutations)
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

func TestPRResumeRejectsCleanLocalAncestorWithRemoteAncestor(t *testing.T) {
	fixture := newResumeFixture(t)
	ancestor := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
	runGitTest(t, fixture.worktree, "reset", "--hard", ancestor)
	runGitTest(t, fixture.worktree, "push", "origin", ancestor+":refs/heads/agent/issue-14-run-resume-test")
	backend := newResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(resumeArgs(fixture.expected)); err == nil || !strings.Contains(err.Error(), "predates expected PR head") {
		t.Fatalf("resume with local ancestor error = %v", err)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != ancestor {
		t.Fatalf("local head changed to %s", got)
	}
	if backend.mutations != 0 {
		t.Fatalf("ancestor rejection mutations = %d", backend.mutations)
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
			if err == nil || !strings.Contains(err.Error(), "predates expected PR head") {
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
		}, want: "canonical renewal proof"},
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

func makeConflictingClaimMarkersResumeHead(t *testing.T, fixture *resumeFixture) string {
	t.Helper()
	base := runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^")
	tree := runGitTest(t, fixture.worktree, "rev-parse", base+"^{tree}")
	other := createResumeCommitTree(t, fixture.worktree, tree, []string{base},
		claimMessage(14, "run-other", time.Now().UTC().Add(-time.Hour).Truncate(time.Second)))
	head := createResumeCommitTree(t, fixture.worktree, tree, []string{fixture.expected, other}, "Merge test PR head\n")
	runGitTest(t, fixture.worktree, "reset", "--hard", head)
	runGitTest(t, fixture.primary, "push", "--force", "origin", head+":refs/heads/agent/issue-14")
	return head
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
	if local != fixture.expected {
		t.Fatalf("remote renewal moved local HEAD from %s to %s", fixture.expected, local)
	}
	remote := resumeRemoteHead(t, fixture)
	if remote == fixture.expected {
		t.Fatal("resume did not create a remote renewal child")
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", remote+"^"); got != fixture.expected {
		t.Fatalf("renewal parent = %s, want expected head %s", got, fixture.expected)
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
			fetchRef := "refs/workflowctl/remote-agent-proof/" + branch
			temporaryRefPresent := false
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
				case "git for-each-ref --format=%(objectname) " + fetchRef:
					if temporaryRefPresent {
						return sha, nil
					}
					return "", nil
				case "git fetch --no-tags --no-write-fetch-head --refmap= origin refs/heads/" + branch + ":" + fetchRef:
					temporaryRefPresent = true
					return "", nil
				case "git rev-parse " + fetchRef:
					return sha, nil
				case "git update-ref -d " + fetchRef + " " + sha:
					temporaryRefPresent = false
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
