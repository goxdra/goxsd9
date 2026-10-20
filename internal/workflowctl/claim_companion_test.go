package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type companionClaimFixture struct {
	root       string
	issue      int
	companion  int
	runID      string
	worktree   string
	primary    string
	other      string
	merged     string
	lease      time.Time
	otherLease time.Time
}

func datedClaimCommit(t *testing.T, root, parent, message, date string) string {
	t.Helper()
	tree := runGitTest(t, root, "rev-parse", parent+"^{tree}")
	// #nosec G204 -- the test supplies its generated repository tree and parent.
	command := exec.CommandContext(context.Background(), "git", "commit-tree", tree, "-p", parent)
	command.Dir = root
	command.Stdin = strings.NewReader(message)
	command.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("dated claim commit: %v: %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func newCompanionClaimFixture(t *testing.T, primaryFirst bool, primaryLease, companionLease time.Time) companionClaimFixture {
	t.Helper()
	const primaryIssue, companionIssue = 808, 809
	const primaryRun, companionRun = "run-primary", "run-companion"
	base := newBaseRepositoryFixture(t, false)
	parent := runGitTest(t, base.primary, "rev-parse", "HEAD")
	firstDate, secondDate := "2026-10-20T00:00:00Z", "2026-10-20T00:01:00Z"
	fixture := companionClaimFixture{root: base.primary, issue: primaryIssue, companion: companionIssue,
		runID: primaryRun, lease: primaryLease, otherLease: companionLease}
	if primaryFirst {
		fixture.primary = datedClaimCommit(t, base.primary, parent, claimMessage(primaryIssue, primaryRun, primaryLease), firstDate)
		fixture.other = datedClaimCommit(t, base.primary, parent, claimMessage(companionIssue, companionRun, companionLease), secondDate)
	}
	if !primaryFirst {
		fixture.other = datedClaimCommit(t, base.primary, parent, claimMessage(companionIssue, companionRun, companionLease), firstDate)
		fixture.primary = datedClaimCommit(t, base.primary, parent, claimMessage(primaryIssue, primaryRun, primaryLease), secondDate)
	}
	runGitTest(t, base.primary, "push", "origin", fixture.primary+":refs/heads/"+claimBranch(primaryIssue))
	runGitTest(t, base.primary, "push", "origin", fixture.other+":refs/heads/"+claimBranch(companionIssue))
	fixture.worktree = filepath.Join(base.root, "claim-primary")
	runGitTest(t, base.primary, "worktree", "add", "-b", claimLocalBranch(primaryIssue, primaryRun), fixture.worktree, fixture.primary)
	runGitTest(t, fixture.worktree, "merge", "--no-ff", "-s", "ours", "-m", "fix(workflow): integrate companion claim", fixture.other)
	fixture.merged = runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(primaryIssue))
	return fixture
}

func (fixture companionClaimFixture) application(t *testing.T) (app, *[]string) {
	return claimGitApplication(t, fixture.worktree)
}

func claimGitApplication(t *testing.T, worktree string) (app, *[]string) {
	t.Helper()
	commands := make([]string, 0, 16)
	return app{ctx: context.Background(), stdout: &bytes.Buffer{}, executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if dir == "" {
			dir = worktree
		}
		commands = append(commands, name+" "+strings.Join(args, " "))
		// #nosec G204 -- the workflow under test supplies fixed local Git commands.
		command := exec.CommandContext(context.Background(), name, args...)
		command.Dir = dir
		command.Stdin = input
		output, err := command.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, output)
		}
		if name == "git" && (args[0] == "cat-file" || args[0] == "rev-parse" && len(args) > 1 && strings.HasSuffix(args[1], "^{tree}")) {
			return string(output), nil
		}
		return strings.TrimSpace(string(output)), nil
	}}, &commands
}

//nolint:gocognit // Both acquisition orders assert the same CLI-visible authority and ancestry.
func TestClaimCompanionMarkerDoesNotReplacePrimaryAuthority(t *testing.T) {
	for _, primaryFirst := range []bool{true, false} {
		name := "companion acquired first"
		if primaryFirst {
			name = "primary acquired first"
		}
		t.Run(name, func(t *testing.T) {
			lease := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
			fixture := newCompanionClaimFixture(t, primaryFirst, lease, lease.Add(time.Hour))
			if got := runGitTest(t, fixture.worktree, "rev-parse", fixture.merged+"^2"); got != fixture.other {
				t.Fatalf("companion ancestry = %s, want exact marker %s", got, fixture.other)
			}
			if primaryFirst {
				oldLog := runGitTest(t, fixture.worktree, "log", "-100", "--format=%B")
				oldRun, err := trailerValue(oldLog, "Agent-Run-ID")
				if err != nil || oldRun != "run-companion" {
					t.Fatalf("unscoped log selected %q, %v; want newer companion run", oldRun, err)
				}
			}
			application, _ := fixture.application(t)
			if err := application.verifyClaim(); err != nil {
				t.Fatalf("verify primary with companion ancestry: %v", err)
			}
			if err := application.verifyClaimForPush(fixture.worktree, claimLocalBranch(fixture.issue, fixture.runID), fixture.issue); err != nil {
				t.Fatalf("PR publication guard with companion ancestry: %v", err)
			}
			if err := application.renewClaim(); err != nil {
				t.Fatalf("renew primary with companion ancestry: %v", err)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", fixture.other); got != fixture.other {
				t.Fatalf("companion marker changed: %s", got)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", fixture.merged+"^2"); got != fixture.other {
				t.Fatalf("renewal lost exact companion ancestry: %s", got)
			}
		})
	}
}

//nolint:gocognit // Each consumer must refuse before local, remote, or GitHub mutation.
func assertClaimGuardRefusalBeforeMutation(t *testing.T, fixture companionClaimFixture, want string) {
	t.Helper()
	application, commands := fixture.application(t)
	head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	remote := strings.SplitN(runGitTest(t, fixture.worktree, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue)), "\t", 2)[0]
	for _, check := range []struct {
		name string
		run  func() error
	}{
		{name: "verify", run: application.verifyClaim},
		{name: "renew", run: application.renewClaim},
		{name: "PR publication", run: func() error {
			return application.createPullRequest(fixture.issue, "fix(workflow): preserve primary claim", "Closes #808\n")
		}},
		{name: "evaluation", run: func() error {
			_, _, _, err := application.readEvaluationTarget(1)
			return err
		}},
	} {
		t.Run(check.name, func(t *testing.T) {
			before := len(*commands)
			err := check.run()
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("guard error = %v, want %q", err, want)
			}
			for _, command := range (*commands)[before:] {
				if strings.HasPrefix(command, "git push ") || strings.HasPrefix(command, "gh ") || strings.HasPrefix(command, "git update-ref ") {
					t.Fatalf("refused guard issued mutation %q", command)
				}
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != head {
				t.Fatalf("refused guard moved local head to %s", got)
			}
			if got := strings.SplitN(runGitTest(t, fixture.worktree, "ls-remote", "origin", "refs/heads/"+claimBranch(fixture.issue)), "\t", 2)[0]; got != remote {
				t.Fatalf("refused guard moved fixed remote head to %s", got)
			}
		})
	}
}

func TestClaimCompanionMetadataRefusesExpiredPrimaryChangedRunAndMalformedMarker(t *testing.T) {
	live := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	expired := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	for _, test := range []struct {
		name    string
		lease   time.Time
		message string
		want    string
	}{
		{name: "expired primary live companion", lease: expired, want: "claim #808 expired"},
		{name: "changed primary run", lease: live, message: claimMessage(808, "run-changed", live), want: "not local branch"},
		{name: "malformed primary marker", lease: live, message: "chore(workflow): claim issue #808\n\nAgent-Persona: Smith\nAgent-Run-ID: run-primary\nAgent-Lease-Until: broken\nAgent-Issue: 808\n", want: "non-canonical metadata"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCompanionClaimFixture(t, true, test.lease, live)
			if test.message != "" {
				marker := createResumeTestCommit(t, fixture.worktree, fixture.merged, test.message)
				runGitTest(t, fixture.worktree, "reset", "--hard", marker)
				runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(fixture.issue))
			}
			assertClaimGuardRefusalBeforeMutation(t, fixture, test.want)
		})
	}
}

func TestClaimCompanionExpiredMarkerCannotCloseOrProveEvaluation(t *testing.T) {
	live := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	expired := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	fixture := newCompanionClaimFixture(t, true, live, expired)
	application, _ := fixture.application(t)
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("primary verify with expired companion: %v", err)
	}
	view := pullRequestView{HeadRefOID: fixture.merged}
	for _, issue := range []int{fixture.issue, fixture.companion} {
		view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
			Number int `json:"number"`
		}{Number: issue})
	}
	if err := application.validateClosingClaims(fixture.worktree, view, fixture.issue); err == nil || !strings.Contains(err.Error(), "no active claim") {
		t.Fatalf("PR companion closure = %v, want inactive refusal", err)
	}
	if _, err := application.evaluationClaimProofs(fixture.worktree, view, fixture.issue); err == nil || !strings.Contains(err.Error(), "no active claim") {
		t.Fatalf("evaluation companion proof = %v, want inactive refusal", err)
	}
}

func TestClaimCompanionMalformedSideMarkerRefusesAuthority(t *testing.T) {
	live := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	fixture := newCompanionClaimFixture(t, true, live, live)
	base := runGitTest(t, fixture.worktree, "rev-parse", fixture.primary+"^")
	malformed := createResumeTestCommit(t, fixture.worktree, base,
		"chore(workflow): claim issue #810\n\nAgent-Persona: Smith\nAgent-Run-ID: run-other\nAgent-Lease-Until: broken\nAgent-Issue: 810\n")
	runGitTest(t, fixture.worktree, "merge", "--no-ff", "-s", "ours", "-m", "fix(workflow): include malformed companion marker", malformed)
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(fixture.issue))
	assertClaimGuardRefusalBeforeMutation(t, fixture, "non-canonical metadata")
}

func TestClaimCompanionPrimarySideMarkerIsAmbiguous(t *testing.T) {
	live := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	fixture := newCompanionClaimFixture(t, true, live, live)
	base := runGitTest(t, fixture.worktree, "rev-parse", fixture.primary+"^")
	side := createResumeTestCommit(t, fixture.worktree, base, claimMessage(fixture.issue, "run-other", live))
	runGitTest(t, fixture.worktree, "merge", "--no-ff", "-s", "ours", "-m", "fix(workflow): include conflicting primary marker", side)
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(fixture.issue))
	assertClaimGuardRefusalBeforeMutation(t, fixture, "incomparable issue #808 marker")
}

func TestClaimCompanionPrimaryRenewalLeaseCannotRegress(t *testing.T) {
	live := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	fixture := newCompanionClaimFixture(t, true, live, live)
	first := createResumeTestCommit(t, fixture.worktree, fixture.merged, claimMessage(fixture.issue, fixture.runID, live.Add(3*time.Hour)))
	last := createResumeTestCommit(t, fixture.worktree, first, claimMessage(fixture.issue, fixture.runID, live.Add(2*time.Hour)))
	runGitTest(t, fixture.worktree, "reset", "--hard", last)
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(fixture.issue))
	assertClaimGuardRefusalBeforeMutation(t, fixture, "lease newer than later marker")
}

func TestClaimCompanionMarkerAfterRecoveredIntegrationKeepsPrimaryAuthority(t *testing.T) {
	live := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	fixture := newCompanionClaimFixture(t, true, live, live)
	writeFixtureFile(t, fixture.worktree, "source.txt", "preserved source\n")
	runGitTest(t, fixture.worktree, "add", "source.txt")
	runGitTest(t, fixture.worktree, "commit", "--no-gpg-sign", "-m", "fix(workflow): preserve local source")
	source := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	primaryLease := live.Add(time.Hour)
	marker := createResumeTestCommit(t, fixture.worktree, fixture.merged, claimMessage(fixture.issue, fixture.runID, primaryLease))
	runGitTest(t, fixture.worktree, "push", "origin", marker+":refs/heads/"+claimBranch(fixture.issue))
	tree := runGitTest(t, fixture.worktree, "rev-parse", source+"^{tree}")
	integration := createResumeCommitTree(t, fixture.worktree, tree, []string{source, marker}, claimResumeIntegrationMessage(fixture.issue))
	runGitTest(t, fixture.worktree, "reset", "--hard", integration)
	application, _ := fixture.application(t)
	if err := application.verifyClaimForPush(fixture.worktree, claimLocalBranch(fixture.issue, fixture.runID), fixture.issue); err != nil {
		t.Fatalf("recovered integration push authority: %v", err)
	}
	companionMarker := createResumeTestCommit(t, fixture.worktree, integration, claimMessage(fixture.companion, "run-companion", live.Add(2*time.Hour)))
	runGitTest(t, fixture.worktree, "reset", "--hard", companionMarker)
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(fixture.issue))
	if err := application.verifyClaim(); err != nil {
		t.Fatalf("recovered integration verify with later companion marker: %v", err)
	}
	output, ok := application.stdout.(*bytes.Buffer)
	if !ok {
		t.Fatal("claim output is not buffered")
	}
	if !strings.Contains(output.String(), primaryLease.Format(time.RFC3339)) {
		t.Fatalf("recovered primary authority output = %q, want lease %s", output.String(), primaryLease)
	}
	base := runGitTest(t, fixture.worktree, "rev-parse", fixture.primary+"^")
	side := createResumeTestCommit(t, fixture.worktree, base, claimMessage(fixture.issue, "run-other", live))
	runGitTest(t, fixture.worktree, "merge", "--no-ff", "-s", "ours", "-m", "fix(workflow): include ambiguous recovered marker", side)
	runGitTest(t, fixture.worktree, "push", "origin", "HEAD:refs/heads/"+claimBranch(fixture.issue))
	assertClaimGuardRefusalBeforeMutation(t, fixture, "incomparable issue #808 marker")
}

//nolint:gocognit // Publication and evaluation share one offline GitHub fixture.
func TestClaimCompanionPublicationAndEvaluationUsePrimaryGuard(t *testing.T) {
	live := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	fixture := newCompanionClaimFixture(t, true, live, live.Add(time.Hour))
	application, _ := fixture.application(t)
	gitCommand := application.executeCommand
	var published bool
	var prReads int
	application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if name != "gh" {
			return gitCommand(dir, input, name, args...)
		}
		if len(args) >= 4 && args[1] == "--method" && args[2] == "POST" && args[3] == "repos/"+repositoryKey+"/pulls" {
			published = true
			return `{"number":1,"html_url":"https://example.invalid/pr/1"}`, nil
		}
		endpoint := args[len(args)-1]
		if strings.Contains(endpoint, "/pulls?state=open") {
			return "[]", nil
		}
		if endpoint == "repos/"+repositoryKey+"/pulls/1" {
			prReads++
			response := pullRequestAPI{Number: 1, Body: "Closes #808\nCloses #809\n", State: "open", Title: "fix(workflow): preserve primary claim"}
			response.Base.Ref = "main"
			response.Base.Repo.FullName = repositoryKey
			response.Head.Ref = claimBranch(fixture.issue)
			response.Head.SHA = fixture.merged
			response.Head.Repo.FullName = repositoryKey
			encoded, err := json.Marshal(response)
			return string(encoded), err
		}
		if strings.Contains(endpoint, "/issues/1/comments") {
			return "[]", nil
		}
		return "", fmt.Errorf("unexpected GitHub command: %v", args)
	}
	body := testPRReviewStateFrame(t, prReviewStatePending) + "\nCloses #808\nCloses #809\n"
	if err := application.createPullRequest(fixture.issue, "fix(workflow): preserve primary claim", body); err != nil {
		t.Fatalf("publish primary with exact companion marker: %v", err)
	}
	if !published || runGitTest(t, fixture.worktree, "rev-parse", fixture.merged+"^2") != fixture.other {
		t.Fatal("PR publication did not preserve exact companion marker")
	}
	root, view, primary, err := application.readEvaluationTarget(1)
	if err != nil {
		t.Fatalf("evaluation target with companion marker: %v", err)
	}
	if root != fixture.worktree || primary != fixture.issue || view.HeadRefOID != fixture.merged || prReads != 1 {
		t.Fatalf("evaluation target = %s/#%d/%s, PR reads %d", root, primary, view.HeadRefOID, prReads)
	}
}
