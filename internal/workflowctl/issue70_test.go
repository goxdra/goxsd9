package workflowctl

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaimLocalIdentityUsesRunIDAndIgnoresStaleFixedWorktree(t *testing.T) {
	const (
		issue = 70
		runID = "run-test"
	)
	remoteBranch := claimBranch(issue)
	localBranch := claimLocalBranch(issue, runID)
	if remoteBranch == localBranch {
		t.Fatal("remote and local claim branches unexpectedly match")
	}
	if !strings.Contains(localBranch, runID) {
		t.Fatalf("local claim branch %q does not contain run ID %q", localBranch, runID)
	}

	layout := repositoryLayout{
		primaryRoot: "/repo",
		worktrees: []gitWorktree{
			{path: claimWorktreePath("/repo", remoteBranch), head: "stale", branch: "refs/heads/" + remoteBranch},
			{path: claimWorktreePath("/repo", localBranch), head: "winning", branch: "refs/heads/" + localBranch},
		},
	}
	worktree, found, err := findClaimWorktree(layout, remoteBranch, "winning")
	if err != nil {
		t.Fatalf("findClaimWorktree: %v", err)
	}
	if !found || worktree.branch != "refs/heads/"+localBranch {
		t.Fatalf("findClaimWorktree = %#v, %t; want run-local worktree", worktree, found)
	}
}

func TestAddClaimWorktreeUsesRunLocalBranchAfterFixedRemoteWins(t *testing.T) {
	const (
		remoteBranch = "agent/issue-70"
		localBranch  = "agent/issue-70-run-test"
	)
	primary := t.TempDir()
	stale := t.TempDir()
	common := filepath.Join(primary, ".git")
	runLocal := claimWorktreePath(primary, localBranch)
	var added string
	application := app{
		executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
			command := name + " " + strings.Join(args, " ")
			switch command {
			case "git rev-parse --path-format=absolute --git-common-dir":
				return common, nil
			case "git worktree list --porcelain":
				return fmt.Sprintf("worktree %s\nHEAD base\nbranch refs/heads/main\n\n"+
					"worktree %s\nHEAD stale\nbranch refs/heads/agent/issue-70\n", primary, stale), nil
			case "git -C " + primary + " rev-parse --path-format=absolute --git-dir":
				return common, nil
			case "git -C " + stale + " rev-parse --path-format=absolute --git-dir":
				return filepath.Join(common, "worktrees", filepath.Base(stale)), nil
			case "git worktree add -b " + localBranch + " " + runLocal + " origin/" + remoteBranch:
				added = command
				return "", nil
			default:
				return "", fmt.Errorf("unexpected command: %s", command)
			}
		},
	}
	worktree, err := application.addClaimWorktree(primary, remoteBranch, localBranch)
	if err != nil {
		t.Fatalf("addClaimWorktree: %v", err)
	}
	if worktree != runLocal {
		t.Fatalf("worktree = %q, want %q", worktree, runLocal)
	}
	if added == "" {
		t.Fatal("run-local worktree was not added")
	}
}

func TestRenewClaimPreservesRunIDAndAdvancesFixedRemoteRef(t *testing.T) {
	const (
		issue       = 70
		localBranch = "agent/issue-70-run-test"
	)
	fixture := newBaseRepositoryFixture(t, false)
	lease := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	parent := runGitTest(t, fixture.primary, "rev-parse", "HEAD")
	oldHead := createResumeTestCommit(t, fixture.primary, parent, claimMessage(issue, "run-test", lease))
	runGitTest(t, fixture.primary, "push", "origin", oldHead+":refs/heads/"+claimBranch(issue))
	worktree := filepath.Join(fixture.root, "claim-70")
	runGitTest(t, fixture.primary, "worktree", "add", "-b", localBranch, worktree, oldHead)
	application, _ := claimGitApplication(t, worktree)
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renewClaim: %v", err)
	}
	newHead := runGitTest(t, worktree, "rev-parse", "HEAD")
	commitMessage := runGitTest(t, worktree, "log", "-1", "--format=%B")
	if !strings.Contains(commitMessage, "Agent-Run-ID: run-test\n") {
		t.Fatalf("renewal commit changed run ID: %q", commitMessage)
	}
	if strings.Count(commitMessage, "Agent-Run-ID:") != 1 {
		t.Fatalf("renewal emitted unexpected run ID trailers: %q", commitMessage)
	}
	if newHead == oldHead || runGitTest(t, worktree, "rev-parse", "origin/"+claimBranch(issue)) != newHead ||
		!strings.HasPrefix(runGitTest(t, worktree, "ls-remote", "origin", "refs/heads/"+claimBranch(issue)), newHead) {
		t.Fatal("renewal did not advance both local and fixed remote claim refs")
	}
}

func containsCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}
