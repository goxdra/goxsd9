package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClaimResumeStateCommandReportsDigestWithoutMutation(t *testing.T) {
	fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", nil)
	writeFixtureFile(t, fixture.worktree, "new.go", "untracked bytes\n")
	state := dirtyClaimResumeSnapshot(t, fixture.worktree)
	backend := newClaimResumeBackend(t, fixture)
	var output bytes.Buffer
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: &output}
	if err := application.resumeClaimStateCommand(nil); err != nil {
		t.Fatalf("read resume state: %v", err)
	}
	if !strings.Contains(output.String(), "issue #309") || !strings.Contains(output.String(), "local state dirty SHA-256 "+state.digest) {
		t.Fatalf("state output = %q", output.String())
	}
	if backend.mutations != 0 || dirtyClaimResumeSnapshot(t, fixture.worktree) != state {
		t.Fatal("read-only state command mutated claim artifacts")
	}
	if err := application.resumeClaimStateCommand([]string{"unexpected"}); err == nil {
		t.Fatal("state command accepted extra arguments")
	}
}

func TestClaimResumeLocalStateDistinguishesIndexBytesModeTypeAndDeletion(t *testing.T) {
	fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", []string{"source.go"})
	baseline := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	writeFixtureFile(t, fixture.worktree, "source.go", "unstaged one\n")
	unstaged := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	writeFixtureFile(t, fixture.worktree, "source.go", "unstaged two\n")
	changedBytes := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	runGitTest(t, fixture.worktree, "add", "source.go")
	staged := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	// #nosec G302 -- the executable bit is the mode boundary under test.
	if err := os.Chmod(filepath.Join(fixture.worktree, "source.go"), 0o755); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	mode := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	if err := os.Remove(filepath.Join(fixture.worktree, "source.go")); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	deleted := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	if err := os.Symlink("README", filepath.Join(fixture.worktree, "source.go")); err != nil {
		t.Fatalf("symlink source: %v", err)
	}
	symlink := dirtyClaimResumeSnapshot(t, fixture.worktree).digest
	seen := map[string]bool{}
	for _, digest := range []string{baseline, unstaged, changedBytes, staged, mode, deleted, symlink} {
		if seen[digest] {
			t.Fatalf("distinct local states share digest %s", digest)
		}
		seen[digest] = true
	}
}

func newDirtyClaimResumeFixture(t *testing.T, issue int, runID string, tracked []string) claimResumeFixture {
	t.Helper()
	base := newBaseRepositoryFixture(t, false)
	for _, name := range tracked {
		parent := filepath.Dir(filepath.Join(base.primary, name))
		if err := os.MkdirAll(parent, 0o700); err != nil {
			t.Fatalf("create tracked fixture parent %s: %v", parent, err)
		}
		writeFixtureFile(t, base.primary, name, "original "+name+"\n")
		runGitTest(t, base.primary, "add", name)
	}
	if len(tracked) != 0 {
		runGitTest(t, base.primary, "commit", "--no-gpg-sign", "-m", "add fixture source")
		runGitTest(t, base.primary, "push", "origin", "main")
	}
	parent := runGitTest(t, base.primary, "rev-parse", "HEAD")
	lease := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	expected := createResumeTestCommit(t, base.primary, parent, claimMessage(issue, runID, lease))
	runGitTest(t, base.primary, "push", "origin", expected+":refs/heads/"+claimBranch(issue))
	worktree := filepath.Join(t.TempDir(), fmt.Sprintf("issue-%d-%s", issue, runID))
	runGitTest(t, base.primary, "worktree", "add", "-b", claimLocalBranch(issue, runID), worktree, expected)
	return claimResumeFixture{baseRepositoryFixture: base, issue: issue, expected: expected, runID: runID,
		worktree: worktree, handoff: 2, lease: lease}
}

func dirtyClaimResumeHandoffBody(fixture claimResumeFixture, digest string) string {
	return fmt.Sprintf("%s%d\n\nRun: `%s`\nOriginal claim head: `%s`\nCurrent claim head: `%s`\nFixed branch: `%s`\nLocal branch: `%s`\nWorktree: `%s`\nPreserved state SHA-256: `%s`\nNo source commit or PR was published.\n",
		dirtyClaimResumeTitlePrefix, fixture.issue, fixture.runID, fixture.expected, fixture.expected,
		claimBranch(fixture.issue), claimLocalBranch(fixture.issue, fixture.runID), fixture.worktree, digest)
}

func dirtyClaimResumeSnapshot(t *testing.T, root string) claimResumeLocalSnapshot {
	t.Helper()
	application := app{ctx: context.Background()}
	state, err := application.claimResumeLocalState(root)
	if err != nil {
		t.Fatalf("snapshot claim local state: %v", err)
	}
	return state
}

//nolint:gocognit // This fixture checks every preserved byte and history observable.
func TestClaimResumeDirtyHistoricalShapesPreserveBytesAndHistory(t *testing.T) {
	tests := []struct {
		issue   int
		runID   string
		tracked []string
		staged  string
		changed []string
		newFile string
	}{
		{issue: 309, runID: "run-309-dirty", tracked: []string{"one.go", "two.go", "three.go", "four.go"},
			changed: []string{"one.go", "two.go", "three.go", "four.go"}, newFile: "schema_all_particle_test.go"},
		{issue: 354, runID: "run-354-dirty", tracked: []string{"codegen_scalar.go"}, staged: "codegen_scalar.go"},
		{issue: 417, runID: "run-417-dirty", tracked: []string{"one.go", "two.go", "three.go"},
			changed: []string{"one.go", "two.go", "three.go"}},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.issue), func(t *testing.T) {
			fixture := newDirtyClaimResumeFixture(t, test.issue, test.runID, test.tracked)
			for _, name := range test.changed {
				writeFixtureFile(t, fixture.worktree, name, "unstaged bytes for "+name+"\n")
			}
			if test.staged != "" {
				writeFixtureFile(t, fixture.worktree, test.staged, "staged bytes\n")
				runGitTest(t, fixture.worktree, "add", test.staged)
			}
			if test.newFile != "" {
				writeFixtureFile(t, fixture.worktree, test.newFile, "untracked bytes\n")
			}
			before := dirtyClaimResumeSnapshot(t, fixture.worktree)
			if !before.dirty {
				t.Fatal("fixture is clean")
			}
			statusBefore := runGitTest(t, fixture.worktree, "status", "--porcelain=v1", "--untracked-files=all")
			indexBefore := runGitTest(t, fixture.worktree, "ls-files", "--stage")
			fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
			backend := newClaimResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(claimResumeArgs(fixture, false)); err != nil {
				t.Fatalf("resume dirty claim: %v", err)
			}
			assertClaimResumeRenewed(t, fixture, backend)
			after := dirtyClaimResumeSnapshot(t, fixture.worktree)
			if after != before {
				t.Fatalf("local state changed: before %+v, after %+v", before, after)
			}
			if got := runGitTest(t, fixture.worktree, "status", "--porcelain=v1", "--untracked-files=all"); got != statusBefore {
				t.Fatalf("status changed: before %q, after %q", statusBefore, got)
			}
			if got := runGitTest(t, fixture.worktree, "ls-files", "--stage"); got != indexBefore {
				t.Fatalf("index changed: before %q, after %q", indexBefore, got)
			}
			for _, name := range test.changed {
				assertDirtyClaimResumeFile(t, fixture.worktree, name, "unstaged bytes for "+name+"\n")
			}
			if test.staged != "" {
				assertDirtyClaimResumeFile(t, fixture.worktree, test.staged, "staged bytes\n")
			}
			if test.newFile != "" {
				assertDirtyClaimResumeFile(t, fixture.worktree, test.newFile, "untracked bytes\n")
			}
			if len(backend.comments) != 2 || backend.comments[1].Body != fixture.handoffBody {
				t.Fatal("original claim or handoff history changed")
			}
			mutations := backend.mutations
			if err := application.run(claimResumeArgs(fixture, false)); err != nil {
				t.Fatalf("retry dirty claim: %v", err)
			}
			if backend.mutations != mutations || dirtyClaimResumeSnapshot(t, fixture.worktree) != before {
				t.Fatal("retry changed claim artifacts or local bytes")
			}
		})
	}
}

func assertDirtyClaimResumeFile(t *testing.T, root, path, want string) {
	t.Helper()
	// #nosec G304 -- paths are fixture-owned names beneath a temporary worktree.
	contents, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(contents) != want {
		t.Fatalf("%s bytes = %q, want %q", path, contents, want)
	}
}

func TestClaimResumeDirtyPreflightRejectsMalformedEvidenceWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(claimResumeFixture, string) string
	}{
		{name: "wrong digest", mutate: func(_ claimResumeFixture, body string) string {
			start := strings.Index(body, "Preserved state SHA-256: `") + len("Preserved state SHA-256: `")
			return body[:start] + strings.Repeat("0", 64) + body[start+64:]
		}},
		{name: "wrong original head", mutate: func(f claimResumeFixture, body string) string {
			return strings.Replace(body, "Original claim head: `"+f.expected+"`", "Original claim head: `"+strings.Repeat("a", 40)+"`", 1)
		}},
		{name: "wrong current head", mutate: func(f claimResumeFixture, body string) string {
			return strings.Replace(body, "Current claim head: `"+f.expected+"`", "Current claim head: `"+strings.Repeat("a", 40)+"`", 1)
		}},
		{name: "wrong run", mutate: func(f claimResumeFixture, body string) string {
			return strings.Replace(body, "Run: `"+f.runID+"`", "Run: `run-other`", 1)
		}},
		{name: "extra prose", mutate: func(_ claimResumeFixture, body string) string {
			return body + "No source changes were made.\n"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", nil)
			writeFixtureFile(t, fixture.worktree, "new.go", "untracked bytes\n")
			before := dirtyClaimResumeSnapshot(t, fixture.worktree)
			fixture.handoffBody = test.mutate(fixture, dirtyClaimResumeHandoffBody(fixture, before.digest))
			backend := newClaimResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(claimResumeArgs(fixture, false)); err == nil {
				t.Fatal("invalid dirty proof was accepted")
			}
			if backend.mutations != 0 || dirtyClaimResumeSnapshot(t, fixture.worktree) != before {
				t.Fatal("invalid dirty proof mutated local or remote state")
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
				t.Fatalf("local head moved to %s", got)
			}
		})
	}
}

func TestClaimResumeDirtyContentDigestRejectsUnchangedStatus(t *testing.T) {
	fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", []string{"source.go"})
	writeFixtureFile(t, fixture.worktree, "source.go", "first bytes\n")
	writeFixtureFile(t, fixture.worktree, "new.go", "untracked first\n")
	before := dirtyClaimResumeSnapshot(t, fixture.worktree)
	fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
	status := runGitTest(t, fixture.worktree, "status", "--porcelain=v1", "--untracked-files=all")
	writeFixtureFile(t, fixture.worktree, "source.go", "other bytes\n")
	writeFixtureFile(t, fixture.worktree, "new.go", "untracked other\n")
	if got := runGitTest(t, fixture.worktree, "status", "--porcelain=v1", "--untracked-files=all"); got != status {
		t.Fatalf("status changed from %q to %q; fixture must isolate byte digest", status, got)
	}
	if dirtyClaimResumeSnapshot(t, fixture.worktree).digest == before.digest {
		t.Fatal("byte changes did not change local state digest")
	}
	backend := newClaimResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(claimResumeArgs(fixture, false)); err == nil {
		t.Fatal("changed bytes were accepted")
	}
	if backend.mutations != 0 || runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != fixture.expected {
		t.Fatal("changed bytes caused local or remote mutation")
	}
}

//nolint:gocognit // The test binds cached status, spoofed stat, bytes, and preflight effects.
func TestClaimResumeDirtyDigestIncludesGitCachedCleanTrackedBytes(t *testing.T) {
	fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", []string{"source.go"})
	writeFixtureFile(t, fixture.worktree, "new.go", "untracked marker\n")
	backend := newClaimResumeBackend(t, fixture)
	plain := app{ctx: context.Background(), executeCommand: backend.execute}
	cachedStatus, err := plain.gitRaw(fixture.worktree, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		t.Fatalf("read baseline status: %v", err)
	}
	if strings.Contains(cachedStatus, "source.go") {
		t.Fatalf("tracked source unexpectedly dirty in baseline status %q", cachedStatus)
	}
	masked := func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if name == "git" && len(args) > 0 && args[0] == "status" {
			return cachedStatus, nil
		}
		return backend.execute(dir, input, name, args...)
	}
	application := app{ctx: context.Background(), executeCommand: masked, stdout: io.Discard}
	before, err := application.claimResumeLocalState(fixture.worktree)
	if err != nil {
		t.Fatalf("snapshot cached-clean tracked source: %v", err)
	}
	fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
	backend.comments[1].Body = fixture.handoffBody
	path := filepath.Join(fixture.worktree, "source.go")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat tracked source: %v", err)
	}
	writeFixtureFile(t, fixture.worktree, "source.go", "replaced source.go\n")
	// #nosec G304 -- the path is a named fixture file in a temporary worktree.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read changed tracked source: %v", err)
	}
	if int64(len(contents)) != info.Size() {
		t.Fatalf("changed tracked source size %d, want cached %d", len(contents), info.Size())
	}
	if timeErr := os.Chtimes(path, info.ModTime(), info.ModTime()); timeErr != nil {
		t.Fatalf("restore cached tracked source mtime: %v", timeErr)
	}
	after, err := application.claimResumeLocalState(fixture.worktree)
	if err != nil {
		t.Fatalf("snapshot spoofed tracked source: %v", err)
	}
	if after.digest == before.digest || after.dirty != before.dirty {
		t.Fatalf("cached-clean tracked bytes were omitted: before %+v, after %+v", before, after)
	}
	if err := application.run(claimResumeArgs(fixture, false)); err == nil || !strings.Contains(err.Error(), "local state does not match") {
		t.Fatalf("stale handoff with cached-clean tracked change error = %v", err)
	}
	if backend.mutations != 0 || runGitTest(t, fixture.worktree, "rev-parse", "HEAD") != fixture.expected {
		t.Fatal("stale cached-clean tracked bytes caused a claim mutation")
	}
}

//nolint:gocognit // Both mutation boundaries assert separate remote effects.
func TestClaimResumeDirtyStopsWhenBytesChangeDuringRenewal(t *testing.T) {
	for _, boundary := range []string{"local ref", "remote push"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := newDirtyClaimResumeFixture(t, 417, "run-417-dirty", []string{"source.go"})
			writeFixtureFile(t, fixture.worktree, "source.go", "first bytes\n")
			before := dirtyClaimResumeSnapshot(t, fixture.worktree)
			fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
			backend := newClaimResumeBackend(t, fixture)
			change := func() { writeFixtureFile(t, fixture.worktree, "source.go", "other bytes\n") }
			if boundary == "local ref" {
				backend.afterLocalRenewal = change
			}
			if boundary == "remote push" {
				backend.afterPush = change
			}
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(claimResumeArgs(fixture, false)); err == nil || !strings.Contains(err.Error(), "local state changed") {
				t.Fatalf("changed bytes error = %v", err)
			}
			if len(claimResumeGitHubMutations(backend.calls)) != 0 {
				t.Fatal("changed bytes reached Project or label mutation")
			}
			if boundary == "local ref" && countClaimResumePushes(backend.calls) != 0 {
				t.Fatal("changed bytes after local ref reached remote push")
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^"); got != fixture.expected {
				t.Fatalf("renewal parent changed to %s", got)
			}
			assertDirtyClaimResumeFile(t, fixture.worktree, "source.go", "other bytes\n")
		})
	}
}

func TestClaimResumeDirtyInterruptedGitHubOperationRetries(t *testing.T) {
	fixture := newDirtyClaimResumeFixture(t, 354, "run-354-dirty", []string{"source.go"})
	writeFixtureFile(t, fixture.worktree, "source.go", "staged bytes\n")
	runGitTest(t, fixture.worktree, "add", "source.go")
	before := dirtyClaimResumeSnapshot(t, fixture.worktree)
	fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
	backend := newClaimResumeBackend(t, fixture)
	backend.labelFailure = errors.New("temporary label transport failure")
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	if err := application.run(claimResumeArgs(fixture, false)); err == nil {
		t.Fatal("interrupted label operation returned success")
	}
	assertClaimResumeRenewalArtifacts(t, fixture)
	if dirtyClaimResumeSnapshot(t, fixture.worktree) != before {
		t.Fatal("interrupted operation changed staged state")
	}
	if err := application.run(claimResumeArgs(fixture, false)); err != nil {
		t.Fatalf("retry interrupted dirty claim: %v", err)
	}
	assertClaimResumeRenewed(t, fixture, backend)
	if dirtyClaimResumeSnapshot(t, fixture.worktree) != before {
		t.Fatal("retry changed staged state")
	}
}

func TestClaimResumeDirtyAmbiguousRemoteAndProjectResponsesConverge(t *testing.T) {
	for _, boundary := range []string{"remote push", "Project Picked"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := newDirtyClaimResumeFixture(t, 354, "run-354-dirty", []string{"source.go"})
			writeFixtureFile(t, fixture.worktree, "source.go", "staged bytes\n")
			runGitTest(t, fixture.worktree, "add", "source.go")
			writeFixtureFile(t, fixture.worktree, "new.go", "untracked bytes\n")
			before := dirtyClaimResumeSnapshot(t, fixture.worktree)
			fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
			backend := newClaimResumeBackend(t, fixture)
			if boundary == "remote push" {
				backend.ambiguousPush = true
			}
			if boundary == "Project Picked" {
				backend.ambiguousProject = true
			}
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(claimResumeArgs(fixture, false)); err != nil {
				t.Fatalf("converge ambiguous %s response: %v", boundary, err)
			}
			assertClaimResumeRenewed(t, fixture, backend)
			if dirtyClaimResumeSnapshot(t, fixture.worktree) != before {
				t.Fatal("ambiguous operation changed staged or untracked state")
			}
			assertDirtyClaimResumeFile(t, fixture.worktree, "source.go", "staged bytes\n")
			assertDirtyClaimResumeFile(t, fixture.worktree, "new.go", "untracked bytes\n")
		})
	}
}

//nolint:gocognit // Each ambiguous artifact is checked before any renewal mutation.
func TestClaimResumeDirtyRejectsMovedRefAndDuplicateWorktree(t *testing.T) {
	for _, boundary := range []string{"moved remote ref", "duplicate worktree"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", nil)
			writeFixtureFile(t, fixture.worktree, "new.go", "untracked bytes\n")
			before := dirtyClaimResumeSnapshot(t, fixture.worktree)
			fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, before.digest)
			if boundary == "moved remote ref" {
				moved := createResumeTestCommit(t, fixture.primary, fixture.expected,
					claimMessage(fixture.issue, "run-other", time.Now().UTC().Add(time.Hour)))
				runGitTest(t, fixture.primary, "push", "origin", "+"+moved+":refs/heads/"+claimBranch(fixture.issue))
			}
			if boundary == "duplicate worktree" {
				duplicate := filepath.Join(t.TempDir(), "duplicate")
				runGitTest(t, fixture.primary, "worktree", "add", "--detach", duplicate, fixture.expected)
			}
			backend := newClaimResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			if err := application.run(claimResumeArgs(fixture, false)); err == nil {
				t.Fatal("ambiguous dirty claim proof was accepted")
			}
			if backend.mutations != 0 || dirtyClaimResumeSnapshot(t, fixture.worktree) != before {
				t.Fatal("ambiguous dirty claim proof mutated preserved state")
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
				t.Fatalf("local head moved to %s", got)
			}
		})
	}
}

//nolint:gocognit // Each flag must preserve ref, Project, index, and source state.
func TestClaimResumeDirtyRejectsHiddenIndexFlagsBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		flag string
		want string
	}{
		{name: "assume unchanged", flag: "--assume-unchanged", want: "h "},
		{name: "skip worktree", flag: "--skip-worktree", want: "S "},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDirtyClaimResumeFixture(t, 309, "run-309-dirty", []string{"source.go"})
			writeFixtureFile(t, fixture.worktree, "new.go", "untracked bytes\n")
			runGitTest(t, fixture.worktree, "update-index", test.flag, "source.go")
			fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, strings.Repeat("0", 64))
			backend := newClaimResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			localBefore := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			remoteBefore := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue))
			err := application.run(claimResumeArgs(fixture, false))
			if err == nil || operationDispositionOf(err) != operationDispositionTerminal ||
				!strings.Contains(err.Error(), "skip-worktree or assume-unchanged") {
				t.Fatalf("hidden index flag error = %v, disposition %d", err, operationDispositionOf(err))
			}
			if backend.mutations != 0 || !backend.needsHuman || backend.projectStatus != "Backlog" {
				t.Fatalf("hidden index flag changed claim state: mutations %d, needs-human %t, Project %s",
					backend.mutations, backend.needsHuman, backend.projectStatus)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != localBefore {
				t.Fatalf("local claim head moved from %s to %s", localBefore, got)
			}
			if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue)); got != remoteBefore {
				t.Fatalf("remote claim ref moved from %q to %q", remoteBefore, got)
			}
			assertDirtyClaimResumeFile(t, fixture.worktree, "new.go", "untracked bytes\n")
			if flags := runGitTest(t, fixture.worktree, "ls-files", "-v", "source.go"); !strings.HasPrefix(flags, test.want) {
				t.Fatalf("hidden index flag was cleared: %q", flags)
			}
		})
	}
}

func TestClaimResumeDirtyRejectsTrackedSymlinkParentBeforeMutation(t *testing.T) {
	fixture := newDirtyClaimResumeFixture(t, 417, "run-417-dirty", []string{"pkg/source.go"})
	writeFixtureFile(t, fixture.worktree, "new.go", "untracked bytes\n")
	parent := filepath.Join(fixture.worktree, "pkg")
	if err := os.Remove(filepath.Join(parent, "source.go")); err != nil {
		t.Fatalf("remove tracked source before symlink: %v", err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatalf("remove tracked parent before symlink: %v", err)
	}
	external := t.TempDir()
	writeFixtureFile(t, external, "source.go", "external bytes\n")
	if err := os.Symlink(external, parent); err != nil {
		t.Fatalf("replace tracked parent with symlink: %v", err)
	}
	fixture.handoffBody = dirtyClaimResumeHandoffBody(fixture, strings.Repeat("0", 64))
	backend := newClaimResumeBackend(t, fixture)
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
	localBefore := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	remoteBefore := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue))
	err := application.run(claimResumeArgs(fixture, false))
	if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !strings.Contains(err.Error(), "not a plain directory") {
		t.Fatalf("symlink parent error = %v, disposition %d", err, operationDispositionOf(err))
	}
	if backend.mutations != 0 || !backend.needsHuman || backend.projectStatus != "Backlog" {
		t.Fatalf("symlink parent changed claim state: mutations %d, needs-human %t, Project %s",
			backend.mutations, backend.needsHuman, backend.projectStatus)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != localBefore {
		t.Fatalf("local claim head moved from %s to %s", localBefore, got)
	}
	if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue)); got != remoteBefore {
		t.Fatalf("remote claim ref moved from %q to %q", remoteBefore, got)
	}
	assertDirtyClaimResumeFile(t, fixture.worktree, "new.go", "untracked bytes\n")
	assertDirtyClaimResumeFile(t, external, "source.go", "external bytes\n")
	if target, linkErr := os.Readlink(parent); linkErr != nil || target != external {
		t.Fatalf("symlink parent changed: target %q, error %v", target, linkErr)
	}
}
