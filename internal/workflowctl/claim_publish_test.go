package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type publicationFixture struct {
	claimResumeFixture
	pr               bool
	prBranch         string
	prRepository     string
	prHead           string
	pushURL          string
	beforePush       func()
	beforeRemoteRead func()
	pushes           int
	remoteReads      int
	output           bytes.Buffer
}

func newPublicationFixture(t *testing.T) *publicationFixture {
	t.Helper()
	f := &publicationFixture{claimResumeFixture: newClaimResumeIssueFixtureAtLease(t, 925, "run-publication-test",
		time.Now().UTC().Add(time.Hour).Truncate(time.Second))}
	f.commit(t, "fix(workflow): first publication")
	return f
}

func (f *publicationFixture) commit(t *testing.T, message string) string {
	t.Helper()
	writeFixtureFile(t, f.worktree, strings.ReplaceAll(message, " ", "-"), message+"\n")
	runGitTest(t, f.worktree, "add", ".")
	runGitTest(t, f.worktree, "-c", "user.name=Workflow Test", "-c", "user.email=workflow@example.test",
		"commit", "--no-gpg-sign", "-m", message)
	return runGitTest(t, f.worktree, "rev-parse", "HEAD")
}

func (f *publicationFixture) remote(t *testing.T) string {
	t.Helper()
	output := runGitTest(t, f.worktree, "ls-remote", "--heads", "origin", "refs/heads/"+claimBranch(f.issue))
	return strings.Fields(output)[0]
}

//nolint:gocognit // The injected executor records each publication boundary in one fixture.
func (f *publicationFixture) application(t *testing.T) app {
	t.Helper()
	return app{ctx: context.Background(), stdout: &f.output, stderr: io.Discard,
		prHeadObservationWait: func(time.Duration) {},
		claimMetadataReader: func(_ string, issue int, branch string) (time.Time, string, error) {
			if issue != f.issue || branch != claimLocalBranch(f.issue, f.runID) {
				return time.Time{}, "", fmt.Errorf("unexpected claim metadata request %d %s", issue, branch)
			}
			return f.lease, f.runID, nil
		},
		executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
			if name == "git" && strings.Join(args, " ") == "rev-parse --show-toplevel" {
				return f.worktree, nil
			}
			if name == "gh" {
				return f.github(t, args...)
			}
			if name == "git" && len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" {
				if len(args) == 5 && args[2] == "--push" && f.pushURL != "" {
					return f.pushURL, nil
				}
				return "https://github.com/" + repositoryKey + ".git", nil
			}
			if name == "git" && len(args) >= 4 && args[0] == "ls-remote" && args[1] == "--heads" && args[2] == "origin" {
				f.remoteReads++
				if f.beforeRemoteRead != nil {
					callback := f.beforeRemoteRead
					f.beforeRemoteRead = nil
					callback()
				}
			}
			if name == "git" && len(args) >= 3 && args[0] == "push" {
				f.pushes++
				if f.beforePush != nil {
					callback := f.beforePush
					f.beforePush = nil
					callback()
				}
			}
			// #nosec G204 -- fixture commands are selected by workflowctl and run against temporary local repositories.
			command := exec.CommandContext(context.Background(), name, args...)
			command.Dir = dir
			command.Stdin = input
			output, err := command.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(output)))
			}
			if name == "git" && len(args) > 0 && (args[0] == "cat-file" ||
				(args[0] == "rev-parse" && strings.HasSuffix(args[len(args)-1], "^{tree}"))) {
				return string(output), nil
			}
			return strings.TrimSpace(string(output)), nil
		}}
}

func (f *publicationFixture) github(t *testing.T, args ...string) (string, error) {
	t.Helper()
	request := strings.Join(args, " ")
	list := "api repos/" + repositoryKey + "/pulls?state=open&head=" + owner + ":" + claimBranch(f.issue) + "&per_page=100"
	if request != list && request != "api repos/"+repositoryKey+"/pulls/924" {
		return "", fmt.Errorf("unexpected GitHub request %s", request)
	}
	if !f.pr {
		return "[]", nil
	}
	pr := retryOpenPR(f.prHead, "Closes #925\n")
	pr.Number = 924
	pr.URL = "https://github.com/" + repositoryKey + "/pull/924"
	pr.Head.Ref = claimBranch(f.issue)
	if f.prBranch != "" {
		pr.Head.Ref = f.prBranch
	}
	if f.prRepository != "" {
		pr.Head.Repo.FullName = f.prRepository
	}
	if f.prHead == "" {
		pr.Head.SHA = f.remote(t)
	}
	var payload any = pr
	if request == list {
		payload = []pullRequestAPI{pr}
	}
	content, err := json.Marshal(payload)
	return string(content), err
}

func TestClaimPublishExistingDraftUpdatesFixedAndExactPRHead(t *testing.T) {
	f := newPublicationFixture(t)
	a := f.application(t)
	a.claimMetadataReader = nil
	first := runGitTest(t, f.worktree, "rev-parse", "HEAD")
	if err := a.run([]string{"claim", "publish"}); err != nil {
		t.Fatalf("initial publication: %v", err)
	}
	if got := f.remote(t); got != first {
		t.Fatalf("initial fixed head = %s, want %s", got, first)
	}
	f.pr = true
	f.prHead = ""
	remediation := f.commit(t, "fix(workflow): publish remediation")
	if err := a.run([]string{"claim", "publish"}); err != nil {
		t.Fatalf("remediation publication: %v", err)
	}
	if got := f.remote(t); got != remediation {
		t.Fatalf("fixed head = %s, want remediation %s", got, remediation)
	}
	if got := runGitTest(t, f.worktree, "branch", "--show-current"); got != claimLocalBranch(f.issue, f.runID) {
		t.Fatalf("local run branch changed to %s", got)
	}
	if got := runGitTest(t, f.worktree, "rev-parse", "HEAD"); got != remediation {
		t.Fatalf("local head changed to %s", got)
	}
	if err := a.run([]string{"claim", "publish"}); err != nil {
		t.Fatalf("idempotent publication: %v", err)
	}
	if f.pushes != 2 || !strings.Contains(f.output.String(), "published claim #925") {
		t.Fatalf("publication count/output = %d %q", f.pushes, f.output.String())
	}
}

func TestClaimPublishExactIssueMarkerWithPrefixCompanion(t *testing.T) {
	f := newPublicationFixture(t)
	first := runGitTest(t, f.worktree, "rev-parse", "HEAD")
	companion := createResumeTestCommit(t, f.worktree, first, claimMessage(9250, "run-companion", f.lease))
	runGitTest(t, f.worktree, "reset", "--hard", companion)
	a := f.application(t)
	a.claimMetadataReader = nil
	if err := a.run([]string{"claim", "publish"}); err != nil {
		t.Fatalf("publish prefix companion: %v", err)
	}
	if got := f.remote(t); got != companion {
		t.Fatalf("fixed companion head = %s, want %s", got, companion)
	}
	if err := a.run([]string{"claim", "publish"}); err != nil {
		t.Fatalf("idempotent prefix companion publication: %v", err)
	}
	f.pr = true
	remediation := f.commit(t, "fix(workflow): publish after prefix companion")
	if err := a.run([]string{"claim", "publish"}); err != nil {
		t.Fatalf("remediation after prefix companion: %v", err)
	}
	if got := f.remote(t); got != remediation || f.pushes != 2 {
		t.Fatalf("fixed head = %s with %d pushes, want %s with two", got, f.pushes, remediation)
	}
}

func TestClaimPublishRejectsWithoutRemoteMutation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *publicationFixture)
		want    string
	}{
		{"dirty", func(t *testing.T, f *publicationFixture) { writeFixtureFile(t, f.worktree, "dirty", "x") }, "uncommitted"},
		{"wrong push repository", func(_ *testing.T, f *publicationFixture) { f.pushURL = "https://github.com/other/repo.git" }, "origin"},
		{"expired lease", func(_ *testing.T, f *publicationFixture) { f.lease = time.Now().UTC().Add(-time.Minute) }, "expired"},
		{"wrong PR branch", func(_ *testing.T, f *publicationFixture) { f.pr = true; f.prBranch = "agent/issue-925-run-other" }, "conflicting identity"},
		{"wrong PR repository", func(_ *testing.T, f *publicationFixture) { f.pr = true; f.prRepository = "other/repo" }, "conflicting identity"},
		{"lost fixed ref", func(t *testing.T, f *publicationFixture) {
			main := runGitTest(t, f.worktree, "rev-parse", "origin/main")
			runGitTest(t, f.worktree, "push", "--force", "origin", main+":refs/heads/"+claimBranch(f.issue))
		}, "no unambiguous"},
		{"other run owns fixed ref", func(t *testing.T, f *publicationFixture) {
			other := createResumeTestCommit(t, f.worktree, f.expected, claimMessage(f.issue, "run-other", f.lease))
			runGitTest(t, f.worktree, "push", "origin", other+":refs/heads/"+claimBranch(f.issue))
		}, "diverged"},
		{"divergent fixed ref", func(t *testing.T, f *publicationFixture) {
			other := createResumeTestCommit(t, f.worktree, f.expected, "fix(workflow): other commit\n")
			runGitTest(t, f.worktree, "push", "origin", other+":refs/heads/"+claimBranch(f.issue))
		}, "diverged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPublicationFixture(t)
			f.prHead = f.expected
			tc.prepare(t, f)
			before := f.remote(t)
			err := f.application(t).run([]string{"claim", "publish"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("publish error = %v, want %q", err, tc.want)
			}
			if after := f.remote(t); after != before || f.pushes != 0 {
				t.Fatalf("rejected publication changed remote %s -> %s, pushes %d", before, after, f.pushes)
			}
		})
	}
}

func TestClaimPublishRejectsRemoteMovementBeforePush(t *testing.T) {
	f := newPublicationFixture(t)
	before := f.remote(t)
	var competing string
	f.beforeRemoteRead = func() {
		competing = createResumeTestCommit(t, f.worktree, before, "fix(workflow): moved before publication\n")
		runGitTest(t, f.worktree, "push", "origin", competing+":refs/heads/"+claimBranch(f.issue))
	}
	err := f.application(t).run([]string{"claim", "publish"})
	if err == nil || !strings.Contains(err.Error(), "moved") {
		t.Fatalf("moved prepush ref error = %v", err)
	}
	if got := f.remote(t); got != competing || f.pushes != 0 {
		t.Fatalf("moved prepush ref changed to %s with %d publication pushes", got, f.pushes)
	}
}

func TestClaimPublishRejectsMovedRefAtPush(t *testing.T) {
	f := newPublicationFixture(t)
	before := f.remote(t)
	f.beforePush = func() {
		other := createResumeTestCommit(t, f.worktree, before, "fix(workflow): competing publication\n")
		runGitTest(t, f.worktree, "push", "origin", other+":refs/heads/"+claimBranch(f.issue))
	}
	err := f.application(t).run([]string{"claim", "publish"})
	if err == nil || !strings.Contains(err.Error(), "publish fixed claim branch") {
		t.Fatalf("raced publication error = %v", err)
	}
	if f.remote(t) == before || f.remote(t) == runGitTest(t, f.worktree, "rev-parse", "HEAD") {
		t.Fatal("raced push overwrote the competing remote head")
	}
}

func TestClaimPublishRejectsDeletedRefAtPushWithoutRecreation(t *testing.T) {
	f := newPublicationFixture(t)
	f.beforePush = func() {
		runGitTest(t, f.worktree, "push", "origin", ":refs/heads/"+claimBranch(f.issue))
	}
	err := f.application(t).run([]string{"claim", "publish"})
	if err == nil || !strings.Contains(err.Error(), "publish fixed claim branch") {
		t.Fatalf("deleted-ref publication error = %v", err)
	}
	if got := runGitTest(t, f.worktree, "ls-remote", "--heads", "origin", "refs/heads/"+claimBranch(f.issue)); got != "" {
		t.Fatalf("deleted fixed ref was recreated: %q", got)
	}
	if f.pushes != 1 {
		t.Fatalf("publication push count = %d, want one rejected attempt", f.pushes)
	}
}

func TestClaimPublishRejectsAncestralRefMoveAtPush(t *testing.T) {
	f := newPublicationFixture(t)
	ancestor := runGitTest(t, f.worktree, "rev-parse", "origin/main")
	f.beforePush = func() {
		runGitTest(t, f.worktree, "push", "--force", "origin", ancestor+":refs/heads/"+claimBranch(f.issue))
	}
	err := f.application(t).run([]string{"claim", "publish"})
	if err == nil || !strings.Contains(err.Error(), "publish fixed claim branch") {
		t.Fatalf("ancestral-move publication error = %v", err)
	}
	if got := f.remote(t); got != ancestor || f.pushes != 1 {
		t.Fatalf("ancestral remote head = %s with %d publication pushes, want %s and one", got, f.pushes, ancestor)
	}
}

func TestClaimPublishRejectsStaleExactPRHead(t *testing.T) {
	f := newPublicationFixture(t)
	f.pr = true
	f.prHead = createResumeTestCommit(t, f.worktree, f.expected, "fix(workflow): unrelated PR head\n")
	before := f.remote(t)
	err := f.application(t).run([]string{"claim", "publish"})
	if err == nil || !strings.Contains(err.Error(), "outside verified claim branch history") {
		t.Fatalf("stale PR error = %v", err)
	}
	if got := f.remote(t); got != before || f.pushes != 0 {
		t.Fatalf("stale PR changed remote to %s with %d pushes", got, f.pushes)
	}
}

func TestClaimPublishReportsUnobservedPRHeadAfterFixedPush(t *testing.T) {
	f := newPublicationFixture(t)
	f.pr = true
	f.prHead = f.expected
	local := runGitTest(t, f.worktree, "rev-parse", "HEAD")
	err := f.application(t).run([]string{"claim", "publish"})
	if err == nil || !strings.Contains(err.Error(), "PR head observation") {
		t.Fatalf("unobserved PR head error = %v", err)
	}
	if got := f.remote(t); got != local || f.pushes != 1 {
		t.Fatalf("fixed publication = %s with %d pushes, want %s and one push", got, f.pushes, local)
	}
}
