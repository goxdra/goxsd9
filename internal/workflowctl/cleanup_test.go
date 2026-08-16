package workflowctl

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestCleanupRemovesOnlyMergedPrimaryAndCompanionClaims(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, false)
	companionPath := claimWorktreePath(fixture.primary, "agent/issue-100")
	runGitTest(t, fixture.primary, "worktree", "add", "-b", "agent/issue-100", companionPath, "origin/main")
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
