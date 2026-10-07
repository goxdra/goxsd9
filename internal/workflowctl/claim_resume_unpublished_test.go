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

func unpublishedClaimFixture(t *testing.T, staged bool) (claimResumeFixture, string, *claimResumeBackend) {
	t.Helper()
	fixture := newDirtyClaimResumeFixture(t, 643, "run-643-source", []string{"source.go"})
	writeFixtureFile(t, fixture.worktree, "source.go", "unpublished source\n")
	runGitTest(t, fixture.worktree, "add", "source.go")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "fix(workflow): preserve source")
	source := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if staged {
		writeFixtureFile(t, fixture.worktree, "source.go", "resolved staged integration\n")
		runGitTest(t, fixture.worktree, "add", "source.go")
		path := runGitTest(t, fixture.worktree, "rev-parse", "--git-path", "MERGE_HEAD")
		if !filepath.IsAbs(path) {
			path = filepath.Join(fixture.worktree, path)
		}
		if err := os.WriteFile(path, []byte(runGitTest(t, fixture.worktree, "rev-parse", "main")+"\n"), 0o600); err != nil {
			t.Fatalf("write pending merge head: %v", err)
		}
	}
	state := dirtyClaimResumeSnapshot(t, fixture.worktree)
	fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, state.digest)
	return fixture, source, newClaimResumeBackend(t, fixture)
}

func unpublishedClaimArgs(fixture claimResumeFixture, source string, integrate bool) []string {
	args := append(claimResumeArgs(fixture, false), "--unpublished-local-head", source)
	if integrate {
		args = append(args, "--integrate")
	}
	return args
}

//nolint:gocognit // The test asserts each remote and local phase observable.
func TestClaimResumeUnpublishedSourceRemoteOnlyThenIntegration(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "resolved-staged-merge"}[staged], func(t *testing.T) {
			fixture, source, backend := unpublishedClaimFixture(t, staged)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			before := dirtyClaimResumeSnapshot(t, fixture.worktree)
			index := claimResumeRawIndexBytes(t, fixture.worktree)
			merge, err := application.claimResumeMergeState(fixture.worktree)
			if err != nil {
				t.Fatalf("read merge state: %v", err)
			}
			if err := application.run(unpublishedClaimArgs(fixture, source, false)); err != nil {
				t.Fatalf("remote-only renewal: %v", err)
			}
			marker := runGitTest(t, fixture.worktree, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue))
			if !strings.Contains(marker, "\trefs/heads/") {
				t.Fatalf("remote marker response: %q", marker)
			}
			marker = strings.SplitN(marker, "\t", 2)[0]
			if runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != source || backend.projectStatus != "Backlog" || !backend.needsHuman {
				t.Fatal("remote-only renewal moved source or reconciled metadata prematurely")
			}
			if parent := runGitTest(t, fixture.worktree, "rev-parse", marker+"^"); parent != fixture.expected {
				t.Fatalf("remote renewal parent = %s, want original anchor %s", parent, fixture.expected)
			}
			if tree := runGitTest(t, fixture.worktree, "rev-parse", marker+"^{tree}"); tree != runGitTest(t, fixture.worktree, "rev-parse", fixture.expected+"^{tree}") {
				t.Fatal("remote renewal published source tree")
			}
			if after := dirtyClaimResumeSnapshot(t, fixture.worktree); after != before || !bytes.Equal(claimResumeRawIndexBytes(t, fixture.worktree), index) {
				t.Fatal("remote renewal changed preserved local state")
			}
			if err := application.run(unpublishedClaimArgs(fixture, source, true)); err != nil {
				t.Fatalf("local integration: %v", err)
			}
			integrated := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			if runGitTest(t, fixture.worktree, "rev-parse", integrated+"^1") != source ||
				runGitTest(t, fixture.worktree, "rev-parse", integrated+"^2") != marker ||
				runGitTest(t, fixture.worktree, "rev-parse", integrated+"^{tree}") != runGitTest(t, fixture.worktree, "rev-parse", source+"^{tree}") {
				t.Fatal("integration did not retain source and remote marker as exact parents with the source tree")
			}
			if after := dirtyClaimResumeSnapshot(t, fixture.worktree); after != before || !bytes.Equal(claimResumeRawIndexBytes(t, fixture.worktree), index) {
				t.Fatal("local integration changed preserved index or files")
			}
			if after, err := application.claimResumeMergeState(fixture.worktree); err != nil || after != merge {
				t.Fatalf("pending merge metadata changed: %s, %v", after, err)
			}
			if backend.projectStatus != "Picked" || backend.needsHuman {
				t.Fatalf("reconciled state = %s needs-human %t", backend.projectStatus, backend.needsHuman)
			}
			if err := application.verifyClaimForPush(fixture.worktree, claimLocalBranch(fixture.issue, fixture.runID), fixture.issue); err != nil {
				t.Fatalf("integrated source cannot pass normal push authority: %v", err)
			}
			if err := application.renewClaim(); err == nil || !strings.Contains(err.Error(), "unpublished integrated source") {
				t.Fatalf("claim renew could publish unchecked source: %v", err)
			}
			if err := application.run(unpublishedClaimArgs(fixture, source, true)); err != nil {
				t.Fatalf("idempotent integration: %v", err)
			}
			if runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != integrated || countClaimResumePushes(backend.calls) != 1 {
				t.Fatal("retry added a marker, integration, or push")
			}
		})
	}
}

func TestClaimResumeUnpublishedRejectsChangedProofBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, claimResumeFixture, string, *claimResumeBackend)
		want   string
	}{
		{name: "source head moved", change: func(t *testing.T, f claimResumeFixture, _ string, _ *claimResumeBackend) {
			writeFixtureFile(t, f.worktree, "new.go", "new source\n")
			runGitTest(t, f.worktree, "add", "new.go")
			runGitTest(t, f.worktree, "commit", "--no-gpg-sign", "-m", "fix(workflow): later source")
		}, want: "moved before remote renewal"},
		{name: "dirty digest changed", change: func(t *testing.T, f claimResumeFixture, _ string, _ *claimResumeBackend) {
			writeFixtureFile(t, f.worktree, "new.go", "changed bytes\n")
		}, want: "local state does not match"},
		{name: "unmerged index", change: func(t *testing.T, f claimResumeFixture, _ string, _ *claimResumeBackend) {
			// Add a stage-1 entry without touching a tracked working file.
			blob := runGitTest(t, f.worktree, "rev-parse", "HEAD:source.go")
			cmd := []byte("100644 " + blob + " 1\tunmerged.go\n")
			if err := runGitInput(t, f.worktree, cmd, "update-index", "--index-info"); err != nil {
				t.Fatalf("make unmerged index: %v", err)
			}
		}, want: "unmerged"},
		{name: "source-bearing remote", change: func(t *testing.T, f claimResumeFixture, source string, _ *claimResumeBackend) {
			runGitTest(t, f.worktree, "push", "origin", source+":refs/heads/"+claimBranch(f.issue))
		}, want: "not the unique renewal child"},
		{name: "untrusted handoff", change: func(_ *testing.T, _ claimResumeFixture, _ string, b *claimResumeBackend) {
			b.comments[1].User.Login = "other"
		}, want: "not trusted API actor"},
		{name: "wrong-run remote marker", change: func(t *testing.T, f claimResumeFixture, _ string, _ *claimResumeBackend) {
			marker := createResumeTestCommit(t, f.worktree, f.expected, claimMessage(f.issue, "run-other", time.Now().UTC().Add(time.Hour).Truncate(time.Second)))
			runGitTest(t, f.worktree, "push", "origin", marker+":refs/heads/"+claimBranch(f.issue))
		}, want: "metadata binds issue"},
		{name: "expired remote marker", change: func(t *testing.T, f claimResumeFixture, _ string, _ *claimResumeBackend) {
			marker := createResumeTestCommit(t, f.worktree, f.expected, claimMessage(f.issue, f.runID, time.Now().UTC().Add(-time.Minute).Truncate(time.Second)))
			runGitTest(t, f.worktree, "push", "origin", marker+":refs/heads/"+claimBranch(f.issue))
		}, want: "expired claim renewal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, source, backend := unpublishedClaimFixture(t, true)
			test.change(t, fixture, source, backend)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(unpublishedClaimArgs(fixture, source, false))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("recovery error = %v, want %q", err, test.want)
			}
			if backend.mutations != 0 || len(claimResumeGitHubMutations(backend.calls)) != 0 {
				t.Fatalf("failed proof mutated claim: git=%d GitHub=%v", backend.mutations, claimResumeGitHubMutations(backend.calls))
			}
		})
	}
}

func TestClaimResumeUnpublishedAmbiguousPushConverges(t *testing.T) {
	fixture, source, backend := unpublishedClaimFixture(t, false)
	backend.ambiguousPush = true
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(unpublishedClaimArgs(fixture, source, false)); err != nil {
		t.Fatalf("reconcile ambiguous marker push: %v", err)
	}
	remote := strings.SplitN(runGitTest(t, fixture.worktree, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue)), "\t", 2)[0]
	if remote == source || runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != source {
		t.Fatal("ambiguous push published or moved source")
	}
	if err := application.run(unpublishedClaimArgs(fixture, source, false)); err != nil {
		t.Fatalf("idempotent remote-only retry: %v", err)
	}
	if countClaimResumePushes(backend.calls) != 1 {
		t.Fatalf("remote-only retry made %d pushes", countClaimResumePushes(backend.calls))
	}
}

func TestClaimResumeIntegrationUsesRenewalMetadataThroughPublication(t *testing.T) {
	fixture, source, backend := unpublishedClaimFixture(t, false)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(unpublishedClaimArgs(fixture, source, true)); err != nil {
		t.Fatalf("resume and integrate source: %v", err)
	}
	lease, runID, err := application.readClaimMetadata(fixture.worktree)
	if err != nil || runID != fixture.runID || !lease.After(time.Now().UTC()) {
		t.Fatalf("integrated metadata lease=%s run=%s err=%v", lease, runID, err)
	}
	integrated := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	runGitTest(t, fixture.worktree, "push", "origin", integrated+":refs/heads/"+claimBranch(fixture.issue))
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renew after normal publication: %v", err)
	}
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("verify later normal renewal: %v", err)
	}
}

func TestClaimResumeUnpublishedRequiresBacklogUntilIntegration(t *testing.T) {
	fixture, source, backend := unpublishedClaimFixture(t, false)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(unpublishedClaimArgs(fixture, source, false)); err != nil {
		t.Fatalf("publish remote marker: %v", err)
	}
	mutations := backend.mutations
	backend.projectStatus = "Picked"
	err := application.run(unpublishedClaimArgs(fixture, source, true))
	if err == nil || !strings.Contains(err.Error(), "Backlog until local integration") {
		t.Fatalf("premature Project Picked = %v, want fail-closed proof", err)
	}
	if backend.mutations != mutations || runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != source {
		t.Fatal("premature metadata state mutated preserved local source")
	}
}

func TestClaimResumeUnpublishedMultipleSourceCommitsAndAllegedMarker(t *testing.T) {
	fixture, _, backend := unpublishedClaimFixture(t, false)
	writeFixtureFile(t, fixture.worktree, "second.go", "second unpublished change\n")
	runGitTest(t, fixture.worktree, "add", "second.go")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "fix(workflow): keep second source commit")
	source := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, dirtyClaimResumeSnapshot(t, fixture.worktree).digest)
	backend.comments[1].Body = fixture.handoffBody
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(unpublishedClaimArgs(fixture, source, true)); err != nil {
		t.Fatalf("recover two source commits: %v", err)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^1^1^1"); got != fixture.expected {
		t.Fatalf("integration first parent lost source ancestry: %s", got)
	}

	malformed, _, rejected := unpublishedClaimFixture(t, false)
	runGitTest(t, malformed.worktree, "commit", "--amend", "--no-gpg-sign", "-m", "chore(workflow): claim issue #643",
		"-m", "Agent-Persona: Smith\nAgent-Run-ID: run-643-source\nAgent-Lease-Until: 2027-01-01T00:00:00Z\nAgent-Issue: 643")
	claimedSource := runGitTest(t, malformed.worktree, "rev-parse", "HEAD")
	malformed.handoffBody = dirtyClaimResumeHandoffBody(malformed, dirtyClaimResumeSnapshot(t, malformed.worktree).digest)
	rejected.comments[1].Body = malformed.handoffBody
	rejectApp := app{ctx: context.Background(), executeCommand: rejected.execute, stdout: io.Discard}
	err := rejectApp.run(unpublishedClaimArgs(malformed, claimedSource, false))
	if err == nil || !strings.Contains(err.Error(), "alleges a claim marker") || rejected.mutations != 0 {
		t.Fatalf("source-bearing alleged marker = %v; mutations=%d", err, rejected.mutations)
	}
}

func runGitInput(t *testing.T, root string, input []byte, args ...string) error {
	t.Helper()
	// #nosec G204 -- fixture-supplied Git arguments are generated in this test.
	command := exec.CommandContext(context.Background(), "git", args...)
	command.Dir = root
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return nil
}

//nolint:gocognit // The race and retry share one preserved merge fixture.
func TestClaimResumeUnpublishedMergeMetadataRaceAndProjectRetry(t *testing.T) {
	fixture, source, backend := unpublishedClaimFixture(t, true)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	proof, err := application.readClaimResumeProof(fixture.issue, fixture.expected, fixture.runID, fixture.handoff, source)
	if err != nil {
		t.Fatalf("read sealed source proof: %v", err)
	}
	mergePath := runGitTest(t, fixture.worktree, "rev-parse", "--git-path", "MERGE_HEAD")
	if !filepath.IsAbs(mergePath) {
		mergePath = filepath.Join(fixture.worktree, mergePath)
	}
	// #nosec G304 -- Git resolves this test worktree's merge metadata path.
	before, err := os.ReadFile(mergePath)
	if err != nil {
		t.Fatalf("read MERGE_HEAD: %v", err)
	}
	// #nosec G703 -- Git resolves this test worktree's merge metadata path.
	if writeErr := os.WriteFile(mergePath, []byte(fixture.expected+"\n"), 0o600); writeErr != nil {
		t.Fatalf("change MERGE_HEAD: %v", writeErr)
	}
	err = application.applyClaimResume(proof, false)
	if err == nil || !strings.Contains(err.Error(), "mergeState") && !strings.Contains(err.Error(), "proof changed") {
		t.Fatalf("merge metadata race = %v, want sealed-proof rejection", err)
	}
	if backend.mutations != 0 {
		t.Fatal("merge metadata race mutated refs")
	}
	// #nosec G703 -- Git resolves this test worktree's merge metadata path.
	if writeErr := os.WriteFile(mergePath, before, 0o600); writeErr != nil {
		t.Fatalf("restore MERGE_HEAD: %v", writeErr)
	}
	projectFailure := errors.New("simulated Project failure")
	backend.projectFailure = projectFailure
	err = application.run(unpublishedClaimArgs(fixture, source, true))
	if err == nil || !errors.Is(err, projectFailure) || operationDispositionOf(err) != operationDispositionRetryable {
		t.Fatalf("Project failure = %v, want preserved cause", err)
	}
	marker := strings.SplitN(runGitTest(t, fixture.worktree, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue)), "\t", 2)[0]
	integrated := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if marker == integrated || backend.projectStatus != "Backlog" || backend.needsHuman {
		t.Fatal("Project failure discarded local integration or lost partial state")
	}
	backend.projectFailure = nil
	if err := application.run(unpublishedClaimArgs(fixture, source, true)); err != nil {
		t.Fatalf("retry Project reconciliation: %v", err)
	}
	if runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != integrated || countClaimResumePushes(backend.calls) != 1 || backend.projectStatus != "Picked" {
		t.Fatal("Project retry duplicated marker/integration or failed convergence")
	}
}
