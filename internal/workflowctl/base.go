package workflowctl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const baseSyncCommand = "go tool workflowctl base-sync"

type gitWorktree struct {
	path     string
	head     string
	branch   string
	locked   bool
	bare     bool
	prunable string
}

type repositoryLayout struct {
	callerRoot  string
	commonDir   string
	primaryRoot string
	worktrees   []gitWorktree
}

type cleanPrimary struct {
	layout repositoryLayout
	before string
}

type fetchedBase struct {
	primary cleanPrimary
	origin  string
	ahead   int
	behind  int
}

type synchronizedBase struct {
	fetched fetchedBase
	after   string
}

func (a app) runBaseSync(args []string) error {
	if len(args) != 0 {
		return usageError("base-sync takes no arguments")
	}
	callerRoot, err := a.root()
	if err != nil {
		return err
	}
	layout, err := a.repositoryLayout(callerRoot)
	if err != nil {
		return err
	}
	result, err := a.synchronizeBase(layout, "")
	if err != nil {
		return err
	}
	if err := writeLine(a.stdout, "Git base synchronization: %s", layout.primaryRoot); err != nil {
		return fmt.Errorf("write base synchronization result: %w", err)
	}
	if err := writeLine(a.stdout, "Before: %s", result.fetched.primary.before); err != nil {
		return fmt.Errorf("write base synchronization result: %w", err)
	}
	if err := writeLine(a.stdout, "Fetched origin/main: %s", result.fetched.origin); err != nil {
		return fmt.Errorf("write base synchronization result: %w", err)
	}
	if err := writeLine(a.stdout, "After: %s", result.after); err != nil {
		return fmt.Errorf("write base synchronization result: %w", err)
	}
	return writeLine(a.stdout, "Pinned recursive submodules: ready")
}

func (a app) repositoryLayout(callerRoot string) (repositoryLayout, error) {
	common, err := a.command(callerRoot, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return repositoryLayout{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	common, err = absoluteCleanPath(common)
	if err != nil {
		return repositoryLayout{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	worktrees, err := a.readWorktreeInventory(callerRoot)
	if err != nil {
		return repositoryLayout{}, err
	}
	preflightErr := preflightRegisteredWorktreePaths(callerRoot, worktrees)
	if preflightErr != nil {
		return repositoryLayout{}, preflightErr
	}
	primaryIndex, err := a.canonicalPrimaryIndex(callerRoot, common, worktrees)
	if err != nil {
		return repositoryLayout{}, err
	}
	primary := worktrees[primaryIndex]
	if err := rejectNestedPrimary(primaryIndex, worktrees); err != nil {
		return repositoryLayout{}, err
	}
	return repositoryLayout{
		callerRoot:  callerRoot,
		commonDir:   common,
		primaryRoot: primary.path,
		worktrees:   worktrees,
	}, nil
}

func (a app) readWorktreeInventory(root string) ([]gitWorktree, error) {
	output, err := a.command(root, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("read Git worktree inventory: %w", err)
	}
	worktrees, err := parseWorktreeInventory(output)
	if err != nil {
		return nil, err
	}
	seenPaths := make(map[string]struct{}, len(worktrees))
	for index := range worktrees {
		path, pathErr := absoluteCleanPath(worktrees[index].path)
		if pathErr != nil {
			return nil, fmt.Errorf("resolve worktree %q: %w", worktrees[index].path, pathErr)
		}
		if _, seen := seenPaths[path]; seen {
			return nil, stateError("Git worktree inventory has duplicate path %q; primary checkout is ambiguous", path)
		}
		seenPaths[path] = struct{}{}
		worktrees[index].path = path
	}
	return worktrees, nil
}

func preflightRegisteredWorktreePaths(root string, worktrees []gitWorktree) error {
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect repository root %q: %w", root, err)
	}
	for _, worktree := range worktrees {
		_, err := os.Stat(worktree.path)
		if err == nil {
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect registered Git worktree %q: %w", worktree.path, err)
		}
		return missingWorktreeRegistrationError(worktree)
	}
	return nil
}

func missingWorktreeRegistrationError(worktree gitWorktree) error {
	remediation := "inspect with `git worktree prune --dry-run --verbose`; after confirming this exact path is stale, manually run `git worktree prune --verbose` from the repository"
	if worktree.prunable == "" {
		return stateError("Git worktree registration %q has a missing path; %s", worktree.path, remediation)
	}
	return stateError("Git worktree registration %q has a missing path; Git reported prunable reason %q; %s", worktree.path, worktree.prunable, remediation)
}

func (a app) canonicalPrimaryIndex(root, common string, worktrees []gitWorktree) (int, error) {
	primaryIndexes := make([]int, 0, 1)
	for index, worktree := range worktrees {
		gitDir, err := a.command(root, "git", "-C", worktree.path, "rev-parse", "--path-format=absolute", "--git-dir")
		if err != nil {
			return 0, fmt.Errorf("inspect Git worktree %q: %w", worktree.path, err)
		}
		gitDir, err = absoluteCleanPath(gitDir)
		if err != nil {
			return 0, fmt.Errorf("resolve Git directory for worktree %q: %w", worktree.path, err)
		}
		if gitDir == common {
			primaryIndexes = append(primaryIndexes, index)
		}
	}
	if len(primaryIndexes) != 1 {
		return 0, stateError("Git worktree inventory has %d canonical primary checkouts; primary checkout is ambiguous or linked", len(primaryIndexes))
	}
	return primaryIndexes[0], nil
}

func rejectNestedPrimary(primaryIndex int, worktrees []gitWorktree) error {
	primary := worktrees[primaryIndex]
	for index, worktree := range worktrees {
		if index == primaryIndex {
			continue
		}
		if pathContains(worktree.path, primary.path) {
			return stateError("canonical primary checkout %q is nested in linked worktree %q", primary.path, worktree.path)
		}
	}
	return nil
}

func parseWorktreeInventory(output string) ([]gitWorktree, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, stateError("Git worktree inventory is empty; canonical primary checkout is missing")
	}
	blocks := strings.Split(output, "\n\n")
	worktrees := make([]gitWorktree, 0, len(blocks))
	for _, block := range blocks {
		var worktree gitWorktree
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				worktree.path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			case strings.HasPrefix(line, "HEAD "):
				worktree.head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
			case strings.HasPrefix(line, "branch "):
				worktree.branch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			case line == "bare":
				worktree.bare = true
			case strings.HasPrefix(line, "locked"):
				worktree.locked = true
			case line == "prunable" || strings.HasPrefix(line, "prunable "):
				worktree.prunable = strings.TrimPrefix(line, "prunable")
				worktree.prunable = strings.TrimPrefix(worktree.prunable, " ")
			}
		}
		if worktree.path == "" {
			return nil, stateError("Git worktree inventory contains an entry without a path; canonical primary checkout is ambiguous")
		}
		if worktree.bare {
			continue
		}
		if worktree.head == "" {
			return nil, stateError("Git worktree %q has no HEAD; canonical primary checkout is ambiguous", worktree.path)
		}
		worktrees = append(worktrees, worktree)
	}
	if len(worktrees) == 0 {
		return nil, stateError("Git worktree inventory has no non-bare checkout; canonical primary checkout is missing")
	}
	return worktrees, nil
}

func absoluteCleanPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("git returned an empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (a app) cleanPrimary(layout repositoryLayout) (cleanPrimary, error) {
	status, err := a.command(layout.primaryRoot, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return cleanPrimary{}, fmt.Errorf("read canonical primary status: %w", err)
	}
	if status != "" {
		return cleanPrimary{}, stateError("canonical primary checkout %q is dirty; %s must not discard local state", layout.primaryRoot, baseSyncCommand)
	}
	branch, err := a.command(layout.primaryRoot, "git", "branch", "--show-current")
	if err != nil {
		return cleanPrimary{}, fmt.Errorf("read canonical primary branch: %w", err)
	}
	if branch == "" {
		return cleanPrimary{}, stateError("canonical primary checkout %q is detached; check out main and run %s", layout.primaryRoot, baseSyncCommand)
	}
	if branch != "main" {
		return cleanPrimary{}, stateError("canonical primary checkout %q is on %q, not main; run %s from the canonical checkout", layout.primaryRoot, branch, baseSyncCommand)
	}
	before, err := a.command(layout.primaryRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return cleanPrimary{}, fmt.Errorf("read canonical primary commit: %w", err)
	}
	primary, ok := layout.primaryWorktree()
	if !ok {
		return cleanPrimary{}, stateError("canonical primary checkout %q disappeared from Git worktree inventory; run %s", layout.primaryRoot, baseSyncCommand)
	}
	if primary.branch != "refs/heads/main" {
		return cleanPrimary{}, stateError("canonical primary checkout %q is not registered on main; run %s", layout.primaryRoot, baseSyncCommand)
	}
	if primary.head != before {
		return cleanPrimary{}, stateError("canonical primary checkout changed while resolving Git worktrees; run %s again", baseSyncCommand)
	}
	return cleanPrimary{layout: layout, before: before}, nil
}

func (layout repositoryLayout) primaryWorktree() (gitWorktree, bool) {
	for _, worktree := range layout.worktrees {
		if samePath(worktree.path, layout.primaryRoot) {
			return worktree, true
		}
	}
	return gitWorktree{}, false
}

func (a app) fetchBase(primary cleanPrimary) (fetchedBase, error) {
	if _, err := a.command(primary.layout.primaryRoot, "git", "fetch", "origin", "main"); err != nil {
		return fetchedBase{}, fmt.Errorf("fetch origin/main for Git base synchronization: %w", err)
	}
	current, err := a.command(primary.layout.primaryRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return fetchedBase{}, fmt.Errorf("recheck canonical primary commit after fetch: %w", err)
	}
	if current != primary.before {
		return fetchedBase{}, stateError("canonical main changed concurrently during fetch (before=%s, after=%s); run %s again", primary.before, current, baseSyncCommand)
	}
	origin, err := a.command(primary.layout.primaryRoot, "git", "rev-parse", "origin/main")
	if err != nil {
		return fetchedBase{}, fmt.Errorf("read fetched origin/main: %w", err)
	}
	ahead, behind, err := a.commitRelation(primary.layout.primaryRoot, "HEAD", "origin/main")
	if err != nil {
		return fetchedBase{}, err
	}
	return fetchedBase{primary: primary, origin: origin, ahead: ahead, behind: behind}, nil
}

func (a app) synchronizeBase(layout repositoryLayout, requiredMerge string) (synchronizedBase, error) {
	primary, err := a.cleanPrimary(layout)
	if err != nil {
		return synchronizedBase{}, err
	}
	fetched, err := a.fetchBase(primary)
	if err != nil {
		return synchronizedBase{}, err
	}
	if containmentErr := a.requireMergeContainment(layout, fetched, requiredMerge); containmentErr != nil {
		return synchronizedBase{}, containmentErr
	}
	if relationErr := rejectUnsafeBaseRelation(fetched); relationErr != nil {
		return synchronizedBase{}, relationErr
	}
	if fastForwardErr := a.fastForwardBase(fetched); fastForwardErr != nil {
		return synchronizedBase{}, fastForwardErr
	}
	return a.finishBaseSynchronization(fetched, requiredMerge)
}

func (a app) requireMergeContainment(layout repositoryLayout, fetched fetchedBase, requiredMerge string) error {
	if requiredMerge == "" {
		return nil
	}
	return a.requireCommitContained(layout.primaryRoot, requiredMerge, fetched.origin)
}

func rejectUnsafeBaseRelation(fetched fetchedBase) error {
	if fetched.ahead == 0 {
		return nil
	}
	if fetched.behind != 0 {
		return stateError("canonical main diverged from fetched origin/main (local-only=%d, remote-only=%d); %s will not reset, rebase, stash, or discard state", fetched.ahead, fetched.behind, baseSyncCommand)
	}
	return stateError("canonical main has %d local-only commit(s) ahead of fetched origin/main; %s will not reset, rebase, stash, or discard state", fetched.ahead, baseSyncCommand)
}

func (a app) fastForwardBase(fetched fetchedBase) error {
	if fetched.behind == 0 {
		return nil
	}
	if _, err := a.command(fetched.primary.layout.primaryRoot, "git", "merge", "--ff-only", "origin/main"); err != nil {
		return fmt.Errorf("fast-forward canonical main to origin/main: %w", err)
	}
	return nil
}

func (a app) readSynchronizedCommit(fetched fetchedBase, requiredMerge string) (fetchedBase, string, error) {
	after, err := a.command(fetched.primary.layout.primaryRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return fetchedBase{}, "", fmt.Errorf("read synchronized canonical main: %w", err)
	}
	origin, err := a.command(fetched.primary.layout.primaryRoot, "git", "rev-parse", "origin/main")
	if err != nil {
		return fetchedBase{}, "", fmt.Errorf("recheck fetched origin/main after base synchronization: %w", err)
	}
	if origin == fetched.origin {
		if after != fetched.origin {
			return fetchedBase{}, "", stateError("canonical main changed concurrently during base synchronization (after=%s, fetched origin/main=%s); run %s again", after, fetched.origin, baseSyncCommand)
		}
		if err := a.requireMergeAtTip(fetched.primary.layout.primaryRoot, requiredMerge, origin); err != nil {
			return fetchedBase{}, "", err
		}
		return fetched, after, nil
	}
	return a.advanceSynchronizedOrigin(fetched, after, origin, requiredMerge)
}

func (a app) advanceSynchronizedOrigin(fetched fetchedBase, after, origin, requiredMerge string) (fetchedBase, string, error) {
	root := fetched.primary.layout.primaryRoot
	if err := a.requireCommitContained(root, fetched.origin, origin); err != nil {
		return fetchedBase{}, "", stateError("fetched origin/main changed incompatibly during base synchronization (before=%s, after=%s); run %s again: %v", fetched.origin, origin, baseSyncCommand, err)
	}
	if err := a.requireMergeAtTip(root, requiredMerge, origin); err != nil {
		return fetchedBase{}, "", err
	}
	if after != fetched.origin && after != origin {
		return fetchedBase{}, "", stateError("canonical main changed concurrently during base synchronization (after=%s, fetched origin/main=%s); run %s again", after, origin, baseSyncCommand)
	}
	if after != origin {
		if _, mergeErr := a.command(root, "git", "merge", "--ff-only", "origin/main"); mergeErr != nil {
			return fetchedBase{}, "", fmt.Errorf("fast-forward canonical main to concurrently advanced origin/main: %w", mergeErr)
		}
		var readErr error
		after, readErr = a.command(root, "git", "rev-parse", "HEAD")
		if readErr != nil {
			return fetchedBase{}, "", fmt.Errorf("read canonical main after concurrently advanced fast-forward: %w", readErr)
		}
		if after != origin {
			return fetchedBase{}, "", stateError("canonical main did not converge to concurrently advanced origin/main (after=%s, origin=%s); run %s again", after, origin, baseSyncCommand)
		}
	}
	fetched.origin = origin
	fetched.ahead = 0
	fetched.behind = 0
	return fetched, after, nil
}

func (a app) requireMergeAtTip(root, requiredMerge, tip string) error {
	if requiredMerge == "" {
		return nil
	}
	return a.requireCommitContained(root, requiredMerge, tip)
}

func (a app) finishBaseSynchronization(fetched fetchedBase, requiredMerge string) (synchronizedBase, error) {
	for attempt := 0; attempt < 3; attempt++ {
		result, retry, err := a.synchronizeBaseAttempt(fetched, requiredMerge)
		if err != nil {
			return synchronizedBase{}, err
		}
		if retry {
			continue
		}
		return result, nil
	}
	return synchronizedBase{}, stateError("origin/main kept advancing during base synchronization; run %s again", baseSyncCommand)
}

func (a app) synchronizeBaseAttempt(fetched fetchedBase, requiredMerge string) (synchronizedBase, bool, error) {
	fetched, after, err := a.readSynchronizedCommit(fetched, requiredMerge)
	if err != nil {
		return synchronizedBase{}, false, err
	}
	origin, err := a.updateAndCheckSubmodules(fetched, after)
	if err != nil {
		return synchronizedBase{}, false, err
	}
	if origin != fetched.origin {
		if advanceErr := a.requireSynchronizedOriginAdvance(fetched, origin, requiredMerge); advanceErr != nil {
			return synchronizedBase{}, false, advanceErr
		}
		return synchronizedBase{}, true, nil
	}
	result, err := a.completeBaseSynchronization(fetched, after)
	if err != nil {
		return synchronizedBase{}, false, err
	}
	fetched, after, retry, err := a.finalizeBaseSnapshot(result.fetched, result.after, requiredMerge)
	if err != nil {
		return synchronizedBase{}, false, err
	}
	return synchronizedBase{fetched: fetched, after: after}, retry, nil
}

func (a app) completeBaseSynchronization(fetched fetchedBase, after string) (synchronizedBase, error) {
	freshLayout, err := a.repositoryLayout(fetched.primary.layout.primaryRoot)
	if err != nil {
		return synchronizedBase{}, fmt.Errorf("refresh canonical primary after base synchronization: %w", err)
	}
	if !samePath(freshLayout.primaryRoot, fetched.primary.layout.primaryRoot) {
		return synchronizedBase{}, stateError("canonical primary checkout changed during base synchronization; run %s again", baseSyncCommand)
	}
	if _, verifyErr := a.cleanPrimary(freshLayout); verifyErr != nil {
		return synchronizedBase{}, fmt.Errorf("verify canonical primary after base synchronization: %w", verifyErr)
	}
	if submoduleErr := a.checkPinnedSubmodules(freshLayout.primaryRoot); submoduleErr != nil {
		return synchronizedBase{}, fmt.Errorf("verify recursive pinned submodules after base synchronization: %w", submoduleErr)
	}
	fetched.primary.layout = freshLayout
	return synchronizedBase{fetched: fetched, after: after}, nil
}

func (a app) finalizeBaseSnapshot(fetched fetchedBase, after, requiredMerge string) (fetchedBase, string, bool, error) {
	root := fetched.primary.layout.primaryRoot
	current, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fetchedBase{}, "", false, fmt.Errorf("read final canonical primary commit after base synchronization: %w", err)
	}
	if current != after {
		return fetchedBase{}, "", false, stateError("canonical main changed concurrently during final base synchronization snapshot (after=%s, synchronized=%s); run %s again", current, after, baseSyncCommand)
	}
	origin, err := a.command(root, "git", "rev-parse", "origin/main")
	if err != nil {
		return fetchedBase{}, "", false, fmt.Errorf("read final origin/main after base synchronization: %w", err)
	}
	if origin == fetched.origin {
		if tipErr := a.requireMergeAtTip(root, requiredMerge, origin); tipErr != nil {
			return fetchedBase{}, "", false, tipErr
		}
		return fetched, after, false, nil
	}
	if containErr := a.requireCommitContained(root, fetched.origin, origin); containErr != nil {
		return fetchedBase{}, "", false, stateError("fetched origin/main changed incompatibly during final base synchronization snapshot (before=%s, after=%s); run %s again: %v", fetched.origin, origin, baseSyncCommand, containErr)
	}
	if tipErr := a.requireMergeAtTip(root, requiredMerge, origin); tipErr != nil {
		return fetchedBase{}, "", false, tipErr
	}
	if _, mergeErr := a.command(root, "git", "merge", "--ff-only", "origin/main"); mergeErr != nil {
		return fetchedBase{}, "", false, fmt.Errorf("fast-forward canonical main to final origin/main descendant: %w", mergeErr)
	}
	current, err = a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return fetchedBase{}, "", false, fmt.Errorf("read canonical primary after final origin/main fast-forward: %w", err)
	}
	if current != origin {
		return fetchedBase{}, "", false, stateError("canonical main did not converge to final origin/main descendant (after=%s, origin=%s); run %s again", current, origin, baseSyncCommand)
	}
	fetched.origin = origin
	fetched.ahead = 0
	fetched.behind = 0
	return fetched, current, true, nil
}

func (a app) updateAndCheckSubmodules(fetched fetchedBase, after string) (string, error) {
	root := fetched.primary.layout.primaryRoot
	if err := a.updatePinnedSubmodules(root); err != nil {
		return "", err
	}
	current, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("recheck canonical primary after submodule update: %w", err)
	}
	if current != after {
		return "", stateError("canonical main changed concurrently during submodule update (after=%s, synchronized=%s); run %s again", current, after, baseSyncCommand)
	}
	if _, fetchErr := a.command(root, "git", "fetch", "origin", "main"); fetchErr != nil {
		return "", fmt.Errorf("refresh origin/main after submodule update: %w", fetchErr)
	}
	current, err = a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("recheck canonical primary after origin/main refresh: %w", err)
	}
	if current != after {
		return "", stateError("canonical main changed concurrently during origin/main refresh (after=%s, synchronized=%s); run %s again", current, after, baseSyncCommand)
	}
	origin, err := a.command(root, "git", "rev-parse", "origin/main")
	if err != nil {
		return "", fmt.Errorf("recheck fetched origin/main after submodule update: %w", err)
	}
	return origin, nil
}

func (a app) requireSynchronizedOriginAdvance(fetched fetchedBase, origin, requiredMerge string) error {
	if err := a.requireCommitContained(fetched.primary.layout.primaryRoot, fetched.origin, origin); err != nil {
		return stateError("fetched origin/main changed incompatibly during submodule update (before=%s, after=%s); run %s again: %v", fetched.origin, origin, baseSyncCommand, err)
	}
	if requiredMerge != "" {
		if err := a.requireCommitContained(fetched.primary.layout.primaryRoot, requiredMerge, origin); err != nil {
			return err
		}
	}
	if _, mergeErr := a.command(fetched.primary.layout.primaryRoot, "git", "merge", "--ff-only", "origin/main"); mergeErr != nil {
		return fmt.Errorf("fast-forward canonical main to concurrently advanced origin/main: %w", mergeErr)
	}
	return nil
}

func (a app) updatePinnedSubmodules(root string) error {
	if err := a.checkSubmoduleUpdatePolicy(root); err != nil {
		return err
	}
	if _, err := a.command(root, "git", "submodule", "update", "--init", "--recursive"); err != nil {
		return fmt.Errorf("update pinned recursive submodules: %w", err)
	}
	return nil
}

func (a app) checkSubmoduleUpdatePolicy(root string) error {
	_, statErr := os.Stat(filepath.Join(root, ".gitmodules"))
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect recursive submodule configuration: %w", statErr)
	}
	queries := [][]string{{"config", "--get-regexp", `^submodule\..*\.update$`}}
	if statErr == nil {
		queries = append(queries, []string{"config", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.update$`})
	}
	for _, args := range queries {
		output, found, err := a.readSubmoduleUpdatePolicy(root, args...)
		if err != nil {
			return err
		}
		if found && strings.TrimSpace(output) != "" {
			if err := validateSubmoduleUpdatePolicy(output); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a app) readSubmoduleUpdatePolicy(root string, args ...string) (string, bool, error) {
	output, err := a.command(root, "git", args...)
	if err == nil {
		return output, true, nil
	}
	if isGitNonAncestor(err) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read recursive submodule update policy: %w", err)
}

func validateSubmoduleUpdatePolicy(output string) error {
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("recursive submodule update policy is malformed: %q", line)
		}
		policy := fields[len(fields)-1]
		if policy != "checkout" {
			return fmt.Errorf("recursive submodule update policy %q is unsupported; only checkout is allowed", policy)
		}
	}
	return nil
}

func (a app) checkPinnedSubmodules(root string) error {
	status, err := a.gitRaw(root, "submodule", "status", "--recursive")
	if err != nil {
		return fmt.Errorf("read recursive submodule status: %w", err)
	}
	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case ' ':
		case '-', '+', 'U':
			return fmt.Errorf("recursive submodule is not pinned and ready: %s", line)
		default:
			return fmt.Errorf("recursive submodule status has unknown state: %s", line)
		}
	}
	dirty, err := a.command(root, "git", "submodule", "foreach", "--recursive", "--quiet", "git status --porcelain=v1 --untracked-files=all")
	if err != nil {
		return fmt.Errorf("read recursive submodule worktree status: %w", err)
	}
	if dirty != "" {
		return fmt.Errorf("recursive submodule has uncommitted changes: %s", firstLine(dirty))
	}
	return nil
}

func (a app) commitRelation(root, left, right string) (int, int, error) {
	output, err := a.command(root, "git", "rev-list", "--left-right", "--count", left+"..."+right)
	if err != nil {
		return 0, 0, fmt.Errorf("compare %s and %s: %w", left, right, err)
	}
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("compare %s and %s: invalid Git commit relation %q", left, right, output)
	}
	ahead, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse local-only commit count %q: %w", fields[0], err)
	}
	behind, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse remote-only commit count %q: %w", fields[1], err)
	}
	return ahead, behind, nil
}

func (a app) requireCommitContained(root, commit, tip string) error {
	if _, err := a.command(root, "git", "merge-base", "--is-ancestor", commit, tip); err != nil {
		return stateError("fetched origin/main %s does not contain completed merge %s; preserve state and run %s: %v", tip, commit, baseSyncCommand, err)
	}
	return nil
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
