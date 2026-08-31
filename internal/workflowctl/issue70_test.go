package workflowctl

import (
	"bytes"
	"context"
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
		oldHead     = "old-head"
		newHead     = "new-head"
	)
	lease := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	var commitMessage string
	var commands []string
	application := app{
		ctx:    context.Background(),
		stdout: &bytes.Buffer{},
		executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
			command := name + " " + strings.Join(args, " ")
			commands = append(commands, command)
			switch command {
			case "git rev-parse --show-toplevel":
				return "/repo", nil
			case "git branch --show-current":
				return localBranch, nil
			case "git fetch origin refs/heads/agent/issue-70:refs/remotes/origin/agent/issue-70":
				return "", nil
			case "git rev-parse HEAD", "git rev-parse origin/agent/issue-70":
				return oldHead, nil
			case "git log -100 --format=%B":
				return claimMessage(issue, "run-test", lease), nil
			case "git rev-parse HEAD^{tree}":
				return "tree", nil
			case "git commit-tree tree -p HEAD":
				data, err := io.ReadAll(input)
				if err != nil {
					return "", err
				}
				commitMessage = string(data)
				return newHead, nil
			case "git update-ref refs/heads/agent/issue-70-run-test new-head old-head":
				return "", nil
			case "git push origin new-head:refs/heads/agent/issue-70":
				return "", nil
			default:
				return "", fmt.Errorf("unexpected command: %s", command)
			}
		},
	}
	if err := application.renewClaim(); err != nil {
		t.Fatalf("renewClaim: %v", err)
	}
	if !strings.Contains(commitMessage, "Agent-Run-ID: run-test\n") {
		t.Fatalf("renewal commit changed run ID: %q", commitMessage)
	}
	if strings.Count(commitMessage, "Agent-Run-ID:") != 1 {
		t.Fatalf("renewal emitted unexpected run ID trailers: %q", commitMessage)
	}
	if !containsCommand(commands, "git update-ref refs/heads/agent/issue-70-run-test new-head old-head") {
		t.Fatal("renewal did not advance the run-local branch")
	}
	if !containsCommand(commands, "git push origin new-head:refs/heads/agent/issue-70") {
		t.Fatal("renewal did not advance the fixed remote claim ref")
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
