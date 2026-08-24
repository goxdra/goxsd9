package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCleanupRemovesOnlyMergedPrimaryAndCompanionClaims(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, true)
	companionPath := claimWorktreePath(fixture.primary, "agent/issue-100")
	runGitTest(t, fixture.primary, "worktree", "add", "-b", "agent/issue-100", companionPath, "origin/main")
	initializeFixtureSubmodule(t, fixture.linked)
	initializeFixtureSubmodule(t, companionPath)
	configureTestIdentity(t, companionPath)
	commitMessage := "chore(workflow): claim issue #100\n\nAgent-Lease-Until: 2099-01-01T00:00:00Z\n"
	writeFixtureFile(t, companionPath, "companion", "companion\n")
	runGitTest(t, companionPath, "add", "companion")
	runGitTest(t, companionPath, "commit", "--no-gpg-sign", "-m", commitMessage)
	companionSHA := runGitTest(t, companionPath, "rev-parse", "HEAD")
	runGitTest(t, companionPath, "push", "origin", "HEAD:refs/heads/agent/issue-100")

	runGitTest(t, fixture.linked, "merge", "--ff-only", "agent/issue-100")
	configureTestIdentity(t, fixture.linked)
	writeFixtureFile(t, fixture.linked, "primary", "primary\n")
	runGitTest(t, fixture.linked, "add", "primary")
	runGitTest(t, fixture.linked, "commit", "--no-gpg-sign", "-m", "chore(workflow): claim issue #99\n\nAgent-Lease-Until: 2099-01-01T00:00:00Z\n")
	primarySHA := runGitTest(t, fixture.linked, "rev-parse", "HEAD")
	runGitTest(t, fixture.linked, "push", "origin", "HEAD:refs/heads/agent/issue-99")

	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	layout, err := application.repositoryLayout(fixture.linked)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	view := pullRequestView{BaseRefName: "main", HeadRefName: "agent/issue-99", HeadRefOID: primarySHA}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences,
		struct {
			Number int `json:"number"`
		}{Number: 99},
		struct {
			Number int `json:"number"`
		}{Number: 100})
	plan, err := application.prepareCleanupPlan(fixture.linked, layout, view, 99)
	if err != nil {
		t.Fatalf("prepareCleanupPlan: %v", err)
	}
	if len(plan.claims) != 2 || plan.claims[0].sha != primarySHA || plan.claims[1].sha != companionSHA {
		t.Fatalf("cleanup claims = %#v, want primary and companion heads", plan.claims)
	}
	base := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: layout}}}
	packet := mergedPacket{number: 99, mergeSHA: "merge-proof", plan: plan}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("cleanupClaims: %v", err)
	}
	for _, branch := range []string{"agent/issue-99", "agent/issue-100"} {
		if output := runGitAllowFailure(t, fixture.primary, "show-ref", "--verify", "refs/heads/"+branch); output != "" {
			t.Fatalf("local claim %s remains: %s", branch, output)
		}
		if output := runGitAllowFailure(t, fixture.primary, "ls-remote", "--heads", "origin", "refs/heads/"+branch); output != "" {
			t.Fatalf("remote claim %s remains: %s", branch, output)
		}
	}
	if inventory := runGitTest(t, fixture.primary, "worktree", "list", "--porcelain"); strings.Contains(inventory, "agent/issue-99") || strings.Contains(inventory, "agent/issue-100") {
		t.Fatalf("claim worktree remains:\n%s", inventory)
	}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("repeated cleanupClaims: %v", err)
	}
}

func TestPrepareCleanupPlanAllowsExpectedRunLocalWorktreeBeforeMerge(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, false)
	configureTestIdentity(t, fixture.linked)
	writeFixtureFile(t, fixture.linked, "claim", "claim\n")
	runGitTest(t, fixture.linked, "add", "claim")
	runGitTest(t, fixture.linked, "commit", "--no-gpg-sign", "-m", claimMessage(55, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)))
	writeFixtureFile(t, fixture.linked, "evaluated", "evaluated\n")
	runGitTest(t, fixture.linked, "add", "evaluated")
	runGitTest(t, fixture.linked, "commit", "--no-gpg-sign", "-m", "feat: evaluated work")
	head := runGitTest(t, fixture.linked, "rev-parse", "HEAD")
	runGitTest(t, fixture.linked, "push", "origin", "HEAD:refs/heads/agent/issue-55")

	runBranch := "agent/issue-55-run-good"
	runPath := claimWorktreePath(fixture.primary, runBranch)
	runGitTest(t, fixture.primary, "worktree", "add", "-b", runBranch, runPath, head)

	recordedAt := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	view := pullRequestView{
		BaseRefName: "main",
		Body:        recoveryBody,
		HeadRefName: "agent/issue-55",
		HeadRefOID:  head,
		Comments:    recoveryEvaluationHistory(t, 15, head, recordedAt),
	}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 55})

	application := app{
		ctx:            context.Background(),
		stdout:         &bytes.Buffer{},
		executeCommand: realGitWithNoOpenPRExecutor(t, nil),
	}
	layout, err := application.repositoryLayout(runPath)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	plan, err := application.prepareCleanupPlan(runPath, layout, view, 55, 15)
	if err != nil {
		t.Fatalf("prepareCleanupPlan: %v", err)
	}
	if len(plan.claims) != 1 || plan.claims[0].localBranch != runBranch || !samePath(plan.claims[0].worktreePath, runPath) {
		t.Fatalf("cleanup plan claims = %#v, want expected run-local worktree", plan.claims)
	}
}

func TestAttachProvenRunLocalWorktreeAllowsExpectedAncestorCaller(t *testing.T) {
	const (
		fixedBranch = "agent/issue-86"
		runBranch   = "agent/issue-86-run-good"
		ancestor    = "canonical-anchor"
		proofHead   = "evaluated-head"
	)
	runPath := claimWorktreePath("/repo", runBranch)
	layout := repositoryLayout{
		primaryRoot: "/repo",
		worktrees:   []gitWorktree{{path: runPath, head: ancestor, branch: "refs/heads/" + runBranch}},
	}
	claims := []claimArtifact{{issue: 86, branch: fixedBranch, sha: proofHead}}
	attached, err := attachClaimWorktrees(layout, claims)
	if err != nil {
		t.Fatalf("attachClaimWorktrees: %v", err)
	}
	if hasWorktreeForRoot(attached, runPath) {
		t.Fatal("unproven ancestor worktree was attached before shared proof")
	}
	proven := []provenRunLocalRef{{branch: runBranch, sha: ancestor, localPresent: true}}
	attached, err = attachProvenRunLocalWorktrees(layout, claims, proven, runPath)
	if err != nil {
		t.Fatalf("attachProvenRunLocalWorktrees: %v", err)
	}
	if len(attached) != 1 || attached[0].localBranch != runBranch || !samePath(attached[0].worktreePath, runPath) {
		t.Fatalf("proven ancestor attachment = %#v, want %s at %s", attached, runBranch, runPath)
	}
}

func TestRecoveryCleansInitializedPinnedSubmoduleAfterPostMergeFailure(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, true)
	initializeFixtureSubmodule(t, fixture.linked)
	configureTestIdentity(t, fixture.linked)
	writeFixtureFile(t, fixture.linked, "primary", "primary\n")
	runGitTest(t, fixture.linked, "add", "primary")
	runGitTest(t, fixture.linked, "commit", "--no-gpg-sign", "-m", "chore(workflow): claim issue #99\n\nAgent-Lease-Until: 2099-01-01T00:00:00Z\n")
	head := runGitTest(t, fixture.linked, "rev-parse", "HEAD")
	runGitTest(t, fixture.linked, "push", "origin", "HEAD:refs/heads/agent/issue-99")

	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	layout, plan := primaryCleanupPlan(t, application, fixture, head)
	base := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: layout}}}
	failurePending := true
	failingApplication := application
	failingApplication.executeCommand = cleanupTestExecutor(&failurePending, "agent/issue-99")
	packet := mergedPacket{number: 99, mergeSHA: "merge-proof", plan: plan}
	if err := failingApplication.cleanupClaims(base, packet); err == nil || !strings.Contains(err.Error(), "simulated post-merge cleanup failure") {
		t.Fatalf("post-merge cleanup error = %v, want simulated failure", err)
	}
	if inventory := runGitTest(t, fixture.primary, "worktree", "list", "--porcelain"); strings.Contains(inventory, "issue-99") {
		t.Fatalf("failed cleanup preserved removed claim worktree:\n%s", inventory)
	}
	if output := runGitAllowFailure(t, fixture.primary, "show-ref", "--verify", "refs/heads/agent/issue-99"); output == "" {
		t.Fatal("failed cleanup removed local claim before recovery")
	}
	if output := runGitAllowFailure(t, fixture.primary, "ls-remote", "--heads", "origin", "refs/heads/agent/issue-99"); output == "" {
		t.Fatal("failed cleanup removed remote claim before recovery")
	}

	const body = "Closes #99\n"
	recoveryView := pullRequestView{BaseRefName: "main", Body: body, HeadRefName: "agent/issue-99", HeadRefOID: head}
	recoveryView.ClosingIssuesReferences = append(recoveryView.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 99})
	proof := mergeEvaluationProof{
		bodySHA256:    sha256Hex([]byte(body)),
		baseRefName:   "main",
		closingIssues: []int{99},
		head:          head,
		headRefName:   "agent/issue-99",
	}
	recoveryLayout, err := application.repositoryLayout(fixture.primary)
	if err != nil {
		t.Fatalf("recovery repositoryLayout: %v", err)
	}
	recoveryPlan, err := application.prepareRecoveryCleanupPlanWithProof(fixture.primary, recoveryLayout, recoveryView, 99, proof)
	if err != nil {
		t.Fatalf("prepare recovery cleanup plan: %v", err)
	}
	recoveryBase := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: recoveryLayout}}}
	recoveryPacket := mergedPacket{number: 99, mergeSHA: "merge-proof", plan: recoveryPlan}
	if err := application.cleanupClaims(recoveryBase, recoveryPacket); err != nil {
		t.Fatalf("recovery cleanup: %v", err)
	}
	if err := application.cleanupClaims(recoveryBase, recoveryPacket); err != nil {
		t.Fatalf("idempotent recovery cleanup: %v", err)
	}
	for _, ref := range []string{"refs/heads/agent/issue-99"} {
		if output := runGitAllowFailure(t, fixture.primary, "show-ref", "--verify", ref); output != "" {
			t.Fatalf("local claim %s remains after recovery: %s", ref, output)
		}
	}
	if output := runGitAllowFailure(t, fixture.primary, "ls-remote", "--heads", "origin", "refs/heads/agent/issue-99"); output != "" {
		t.Fatalf("remote claim remains after recovery: %s", output)
	}
}

func TestCleanupPreservesDirtyAndLockedClaimWorktrees(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*testing.T, baseRepositoryFixture)
		want    string
	}{
		{name: "dirty", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			writeFixtureFile(t, fixture.linked, "modules/fixture/dirty", "dirty\n")
		}, want: "dirty"},
		{name: "locked", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			runGitTest(t, fixture.primary, "worktree", "lock", fixture.linked)
		}, want: "locked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBaseRepositoryFixture(t, true)
			initializeFixtureSubmodule(t, fixture.linked)
			application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
			layout, plan := primaryCleanupPlan(t, application, fixture, runGitTest(t, fixture.linked, "rev-parse", "HEAD"))
			test.prepare(t, fixture)
			if test.name == "locked" {
				defer runGitTest(t, fixture.primary, "worktree", "unlock", fixture.linked)
			}
			base := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: layout}}}
			packet := mergedPacket{number: 99, mergeSHA: "merge-proof", plan: plan}
			if err := application.cleanupClaims(base, packet); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("cleanupClaims error = %v, want %q preservation", err, test.want)
			}
			if inventory := runGitTest(t, fixture.primary, "worktree", "list", "--porcelain"); !strings.Contains(inventory, fixture.linked) {
				t.Fatalf("preserved %s claim worktree missing from inventory:\n%s", test.name, inventory)
			}
		})
	}
}

func primaryCleanupPlan(t *testing.T, application app, fixture baseRepositoryFixture, head string) (repositoryLayout, cleanupPlan) {
	t.Helper()
	layout, err := application.repositoryLayout(fixture.linked)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	view := pullRequestView{BaseRefName: "main", HeadRefName: "agent/issue-99", HeadRefOID: head}
	view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
		Number int `json:"number"`
	}{Number: 99})
	plan, err := application.prepareCleanupPlan(fixture.linked, layout, view, 99)
	if err != nil {
		t.Fatalf("prepareCleanupPlan: %v", err)
	}
	return layout, plan
}

func cleanupTestExecutor(failDelete *bool, branch string) commandExecutor {
	return func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if *failDelete && name == "git" && isClaimDeletePush(args, branch) {
			*failDelete = false
			return "", errors.New("simulated post-merge cleanup failure")
		}
		// #nosec G204 -- the test executor invokes fixed repository-local commands.
		command := exec.CommandContext(context.Background(), name, args...)
		command.Dir = dir
		command.Stdin = input
		output, err := command.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
		return strings.TrimSpace(string(output)), nil
	}
}

func isClaimDeletePush(args []string, branch string) bool {
	if len(args) != 4 || args[0] != "push" || args[2] != "origin" || args[3] != ":refs/heads/"+branch {
		return false
	}
	return strings.HasPrefix(args[1], "--force-with-lease=refs/heads/"+branch+":")
}

func TestFindClaimWorktreePreservesExternalPath(t *testing.T) {
	layout := repositoryLayout{
		primaryRoot: "/repo",
		worktrees: []gitWorktree{{
			path:   "/external/issue-100",
			head:   "companion-head",
			branch: "refs/heads/agent/issue-100",
		}},
	}
	_, _, err := findClaimWorktree(layout, "agent/issue-100", "companion-head")
	if err == nil || !strings.Contains(err.Error(), "external") {
		t.Fatalf("findClaimWorktree error = %v, want external-path refusal", err)
	}
}

func TestFindClaimWorktreePreservesAmbiguousRegistration(t *testing.T) {
	layout := repositoryLayout{
		primaryRoot: "/repo",
		worktrees: []gitWorktree{
			{path: "/repo-worktrees/issue-100", head: "head", branch: "refs/heads/agent/issue-100"},
			{path: "/other-worktrees/issue-100", head: "head", branch: "refs/heads/agent/issue-100"},
		},
	}
	_, _, err := findClaimWorktree(layout, "agent/issue-100", "head")
	if err == nil || !strings.Contains(err.Error(), "multiple worktrees") {
		t.Fatalf("ambiguous claim worktree error = %v, want preservation refusal", err)
	}
}

func TestClaimArtifactValidationDistinguishesAbsentAndPartialProof(t *testing.T) {
	claims := []claimArtifact{
		{issue: 55, branch: "agent/issue-55", sha: "primary"},
		{issue: 56, branch: "agent/issue-56", sha: "companion"},
	}
	if err := validateMissingClaimArtifacts(claims, map[string]bool{}, 55, true); err != nil {
		t.Fatalf("all absent recovery artifacts = %v, want idempotent success", err)
	}
	if err := validateMissingClaimArtifacts(claims, map[string]bool{"agent/issue-56": true}, 55, true); err != nil {
		t.Fatalf("fully removed primary with companion remaining = %v, want recoverable partial cleanup", err)
	}
	if err := validateMissingClaimArtifacts(claims, map[string]bool{"agent/issue-55": true}, 55, true); err == nil || !strings.Contains(err.Error(), "issue-56") {
		t.Fatalf("missing companion proof = %v, want preservation refusal", err)
	}
}

func TestClaimArtifactValidationRejectsUnprovenSameIssueRefs(t *testing.T) {
	claims := []claimArtifact{{issue: 55, branch: "agent/issue-55", sha: "primary"}}
	all := []remoteClaim{
		{branch: "agent/issue-55", number: 55, sha: "primary"},
		{branch: "agent/issue-55-old", number: 55, sha: "old"},
		{branch: "origin/agent/issue-55", number: 55, sha: "tracking"},
	}
	if err := validateUnexpectedClaimRefs(all, map[string]claimArtifact{"agent/issue-55": claims[0]}, claims); err == nil || !strings.Contains(err.Error(), "leftover claim artifact") {
		t.Fatalf("unproven same-issue refs = %v, want leftover refusal", err)
	}
}

func configureTestIdentity(t *testing.T, root string) {
	t.Helper()
	runGitTest(t, root, "config", "user.name", "Workflow Test")
	runGitTest(t, root, "config", "user.email", "workflow@example.test")
}

func runGitAllowFailure(t *testing.T, root string, args ...string) string {
	t.Helper()
	// #nosec G204 -- each test supplies repository-local Git arguments without user input.
	command := exec.CommandContext(context.Background(), "git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(output))
	}
	return ""
}
