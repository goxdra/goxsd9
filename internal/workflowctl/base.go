package workflowctl

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const baseSyncCommand = "go tool workflowctl base-sync"

type gitWorktree struct {
	path   string
	head   string
	branch string
	locked bool
	bare   bool
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
	after, err := a.readSynchronizedCommit(fetched)
	if err != nil {
		return synchronizedBase{}, err
	}
	return a.finishBaseSynchronization(fetched, after)
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

func (a app) readSynchronizedCommit(fetched fetchedBase) (string, error) {
	after, err := a.command(fetched.primary.layout.primaryRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("read synchronized canonical main: %w", err)
	}
	if after != fetched.origin {
		return "", stateError("canonical main changed concurrently during base synchronization (after=%s, fetched origin/main=%s); run %s again", after, fetched.origin, baseSyncCommand)
	}
	origin, err := a.command(fetched.primary.layout.primaryRoot, "git", "rev-parse", "origin/main")
	if err != nil {
		return "", fmt.Errorf("recheck fetched origin/main after base synchronization: %w", err)
	}
	if origin != fetched.origin {
		return "", stateError("fetched origin/main changed concurrently during base synchronization (after=%s, fetched=%s); run %s again", origin, fetched.origin, baseSyncCommand)
	}
	return after, nil
}

func (a app) finishBaseSynchronization(fetched fetchedBase, after string) (synchronizedBase, error) {
	if updateErr := a.updatePinnedSubmodules(fetched.primary.layout.primaryRoot); updateErr != nil {
		return synchronizedBase{}, updateErr
	}
	current, err := a.command(fetched.primary.layout.primaryRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return synchronizedBase{}, fmt.Errorf("recheck canonical primary after submodule update: %w", err)
	}
	if current != after {
		return synchronizedBase{}, stateError("canonical main changed concurrently during submodule update (after=%s, synchronized=%s); run %s again", current, after, baseSyncCommand)
	}
	origin, err := a.command(fetched.primary.layout.primaryRoot, "git", "rev-parse", "origin/main")
	if err != nil {
		return synchronizedBase{}, fmt.Errorf("recheck fetched origin/main after submodule update: %w", err)
	}
	if origin != fetched.origin {
		return synchronizedBase{}, stateError("fetched origin/main changed concurrently during submodule update (after=%s, fetched=%s); run %s again", origin, fetched.origin, baseSyncCommand)
	}
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

func (a app) updatePinnedSubmodules(root string) error {
	if _, err := a.command(root, "git", "submodule", "update", "--init", "--recursive"); err != nil {
		return fmt.Errorf("update pinned recursive submodules: %w", err)
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
