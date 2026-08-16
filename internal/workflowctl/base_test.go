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
)

type baseRepositoryFixture struct {
	root    string
	origin  string
	seed    string
	primary string
	linked  string
}

func TestBaseSynchronizationConvergesCleanPrimary(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, false)
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	old := runGitTest(t, fixture.primary, "rev-parse", "HEAD")
	appendSeedCommit(t, fixture, "remote change")

	layout, err := application.repositoryLayout(fixture.linked)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	if layout.callerRoot != fixture.linked || layout.primaryRoot != fixture.primary {
		t.Fatalf("layout = %#v, want caller %s and primary %s", layout, fixture.linked, fixture.primary)
	}
	result, err := application.synchronizeBase(layout, "")
	if err != nil {
		t.Fatalf("synchronizeBase behind: %v", err)
	}
	want := runGitTest(t, fixture.seed, "rev-parse", "HEAD")
	if result.fetched.primary.before != old || result.after != want {
		t.Fatalf("sync result = %#v, want before %s and after %s", result, old, want)
	}

	layout, err = application.repositoryLayout(fixture.primary)
	if err != nil {
		t.Fatalf("repositoryLayout after sync: %v", err)
	}
	result, err = application.synchronizeBase(layout, "")
	if err != nil {
		t.Fatalf("synchronizeBase equal: %v", err)
	}
	if result.fetched.primary.before != want || result.after != want {
		t.Fatalf("idempotent sync result = %#v, want equal commit %s", result, want)
	}
}

func TestBaseSynchronizationPublishesMergedInstructionBeforeLaunch(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, false)
	writeFixtureFile(t, fixture.seed, "AGENTS.md", "canonical instruction from merged head\n")
	runGitTest(t, fixture.seed, "add", "AGENTS.md")
	runGitTest(t, fixture.seed, "commit", "--no-gpg-sign", "-m", "update launch instruction")
	runGitTest(t, fixture.seed, "push", "origin", "main")
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	if err := application.synchronizeBaseMust(fixture.primary); err != nil {
		t.Fatalf("synchronize merged instruction: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(fixture.primary, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read canonical merged instruction: %v", err)
	}
	if string(content) != "canonical instruction from merged head\n" {
		t.Fatalf("canonical instruction = %q, want merged-head content", content)
	}
}

func TestBaseSynchronizationInitializesPinnedRecursiveSubmodules(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, true)
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	oldProtocol, hadProtocol := os.LookupEnv("GIT_ALLOW_PROTOCOL")
	if err := os.Setenv("GIT_ALLOW_PROTOCOL", "file"); err != nil {
		t.Fatalf("set Git test protocol: %v", err)
	}
	defer func() {
		var err error
		if hadProtocol {
			err = os.Setenv("GIT_ALLOW_PROTOCOL", oldProtocol)
		}
		if !hadProtocol {
			err = os.Unsetenv("GIT_ALLOW_PROTOCOL")
		}
		if err != nil {
			t.Errorf("restore Git test protocol: %v", err)
		}
	}()
	layout, err := application.repositoryLayout(fixture.linked)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	if status := runGitTest(t, fixture.primary, "submodule", "status", "--recursive"); !strings.HasPrefix(status, "-") {
		t.Fatalf("submodule unexpectedly initialized: %q", status)
	}
	if _, err := application.synchronizeBase(layout, ""); err != nil {
		t.Fatalf("synchronizeBase with uninitialized submodule: %v", err)
	}
	if status := runGitTest(t, fixture.primary, "submodule", "status", "--recursive"); strings.HasPrefix(status, "-") {
		t.Fatalf("submodule is not pinned after sync: %q", status)
	}
	if _, err := os.Stat(filepath.Join(fixture.primary, "modules", "fixture", "README")); err != nil {
		t.Fatalf("initialized submodule content: %v", err)
	}

	if err := os.WriteFile(filepath.Join(fixture.primary, "modules", "fixture", "dirty"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write dirty submodule file: %v", err)
	}
	if err := application.checkPinnedSubmodules(fixture.primary); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty submodule check = %v, want refusal", err)
	}
}

func TestBaseSynchronizationRefusesUnsafePrimaryStates(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*testing.T, baseRepositoryFixture)
		want       string
		remotePush bool
	}{
		{name: "dirty", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			if err := os.WriteFile(filepath.Join(fixture.primary, "dirty"), []byte("dirty\n"), 0o600); err != nil {
				t.Fatalf("write dirty file: %v", err)
			}
		}, want: "dirty"},
		{name: "detached", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			runGitTest(t, fixture.primary, "checkout", "--detach", "HEAD")
		}, want: "detached"},
		{name: "non-main", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			runGitTest(t, fixture.primary, "switch", "-c", "topic")
		}, want: "not main"},
		{name: "ahead", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			writeFixtureFile(t, fixture.primary, "ahead", "ahead\n")
			runGitTest(t, fixture.primary, "add", "ahead")
			runGitTest(t, fixture.primary, "commit", "--no-gpg-sign", "-m", "ahead")
		}, want: "local-only"},
		{name: "diverged", prepare: func(t *testing.T, fixture baseRepositoryFixture) {
			writeFixtureFile(t, fixture.primary, "local", "local\n")
			runGitTest(t, fixture.primary, "add", "local")
			runGitTest(t, fixture.primary, "commit", "--no-gpg-sign", "-m", "local")
			appendSeedCommit(t, fixture, "remote")
		}, want: "diverged", remotePush: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBaseRepositoryFixture(t, false)
			test.prepare(t, fixture)
			application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
			layout, err := application.repositoryLayout(fixture.linked)
			if err != nil {
				t.Fatalf("repositoryLayout: %v", err)
			}
			before := runGitTest(t, fixture.primary, "rev-parse", "HEAD")
			_, err = application.synchronizeBase(layout, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("synchronizeBase error = %v, want %q", err, test.want)
			}
			if after := runGitTest(t, fixture.primary, "rev-parse", "HEAD"); after != before {
				t.Fatalf("refusal moved primary from %s to %s", before, after)
			}
		})
	}
}

func TestDoctorLaunchRequiresCanonicalFreshBase(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, false)
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	if _, err := application.checkDevelopLaunch(fixture.linked); err == nil ||
		!strings.Contains(err.Error(), "canonical primary") || !strings.Contains(err.Error(), baseSyncCommand) {
		t.Fatalf("linked launch error = %v", err)
	}
	appendSeedCommit(t, fixture, "stale primary")
	if _, err := application.checkDevelopLaunch(fixture.primary); err == nil ||
		!strings.Contains(err.Error(), "base-sync") || !strings.Contains(err.Error(), "behind") {
		t.Fatalf("stale launch error = %v", err)
	}
	if err := application.synchronizeBaseMust(fixture.primary); err != nil {
		t.Fatalf("synchronize stale primary: %v", err)
	}
	if _, err := application.checkDevelopLaunch(fixture.primary); err != nil {
		t.Fatalf("fresh canonical launch: %v", err)
	}
}

func TestDoctorFinalSnapshotRejectsRemoteAdvance(t *testing.T) {
	fixture := newBaseRepositoryFixture(t, false)
	fetches := 0
	application := app{
		ctx: context.Background(),
		executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
			if name == "git" && strings.Join(args, " ") == "fetch origin main" {
				fetches++
				if fetches == 2 {
					appendSeedCommit(t, fixture, "doctor race")
				}
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
		},
	}
	if _, err := application.checkDevelopLaunch(fixture.primary); err == nil || !strings.Contains(err.Error(), "changed after recursive submodule readiness") {
		t.Fatalf("checkDevelopLaunch error = %v, want final-snapshot refusal", err)
	}
}

func (a app) synchronizeBaseMust(root string) error {
	layout, err := a.repositoryLayout(root)
	if err != nil {
		return err
	}
	_, err = a.synchronizeBase(layout, "")
	return err
}

func newBaseRepositoryFixture(t *testing.T, withSubmodule bool) baseRepositoryFixture {
	t.Helper()
	root := t.TempDir()
	fixture := baseRepositoryFixture{
		root:    root,
		origin:  filepath.Join(root, "origin.git"),
		seed:    filepath.Join(root, "seed"),
		primary: filepath.Join(root, "primary"),
	}
	fixture.linked = claimWorktreePath(fixture.primary, "agent/issue-99")
	runGitTest(t, root, "init", "--bare", "--initial-branch=main", fixture.origin)
	runGitTest(t, root, "init", "--initial-branch=main", fixture.seed)
	runGitTest(t, fixture.seed, "config", "user.name", "Workflow Test")
	runGitTest(t, fixture.seed, "config", "user.email", "workflow@example.test")
	writeFixtureFile(t, fixture.seed, "README", "base\n")
	runGitTest(t, fixture.seed, "add", "README")
	runGitTest(t, fixture.seed, "commit", "--no-gpg-sign", "-m", "base")
	runGitTest(t, fixture.seed, "remote", "add", "origin", fixture.origin)
	if withSubmodule {
		addFixtureSubmodule(t, fixture)
	}
	runGitTest(t, fixture.seed, "push", "origin", "main")
	runGitTest(t, root, "clone", "--no-recurse-submodules", fixture.origin, fixture.primary)
	runGitTest(t, fixture.primary, "config", "user.name", "Workflow Test")
	runGitTest(t, fixture.primary, "config", "user.email", "workflow@example.test")
	if withSubmodule {
		runGitTest(t, fixture.primary, "config", "protocol.file.allow", "always")
	}
	if err := os.MkdirAll(filepath.Dir(fixture.linked), 0o700); err != nil {
		t.Fatalf("create claim worktree parent: %v", err)
	}
	runGitTest(t, fixture.primary, "worktree", "add", "-b", "agent/issue-99", fixture.linked, "origin/main")
	return fixture
}

func addFixtureSubmodule(t *testing.T, fixture baseRepositoryFixture) {
	t.Helper()
	subOrigin := filepath.Join(fixture.root, "sub-origin.git")
	subSeed := filepath.Join(fixture.root, "sub-seed")
	runGitTest(t, fixture.root, "init", "--bare", "--initial-branch=main", subOrigin)
	runGitTest(t, fixture.root, "init", "--initial-branch=main", subSeed)
	runGitTest(t, subSeed, "config", "user.name", "Workflow Test")
	runGitTest(t, subSeed, "config", "user.email", "workflow@example.test")
	writeFixtureFile(t, subSeed, "README", "submodule\n")
	runGitTest(t, subSeed, "add", "README")
	runGitTest(t, subSeed, "commit", "--no-gpg-sign", "-m", "submodule")
	runGitTest(t, subSeed, "remote", "add", "origin", subOrigin)
	runGitTest(t, subSeed, "push", "origin", "main")
	runGitTest(t, fixture.seed, "-c", "protocol.file.allow=always", "submodule", "add", subOrigin, "modules/fixture")
	runGitTest(t, fixture.seed, "add", ".gitmodules", "modules/fixture")
	runGitTest(t, fixture.seed, "commit", "--no-gpg-sign", "-m", "add submodule")
}

func appendSeedCommit(t *testing.T, fixture baseRepositoryFixture, message string) {
	t.Helper()
	name := strings.ReplaceAll(strings.ToLower(message), " ", "-")
	writeFixtureFile(t, fixture.seed, name, message+"\n")
	runGitTest(t, fixture.seed, "add", name)
	runGitTest(t, fixture.seed, "commit", "--no-gpg-sign", "-m", message)
	runGitTest(t, fixture.seed, "push", "origin", "main")
}

func writeFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture file %s: %v", name, err)
	}
}

func TestRepositoryLayoutRejectsAmbiguousInventory(t *testing.T) {
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git rev-parse --path-format=absolute --git-common-dir":
			return "/repo/.git", nil
		case "git worktree list --porcelain":
			return "worktree /repo\nHEAD one\nbranch refs/heads/main\n\nworktree /other\nHEAD two\nbranch refs/heads/main\n", nil
		case "git -C /repo rev-parse --path-format=absolute --git-dir", "git -C /other rev-parse --path-format=absolute --git-dir":
			return "/repo/.git", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	_, err := application.repositoryLayout("/caller")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous layout error = %v", err)
	}
}

func TestBaseSynchronizationPreservesGitFailureCause(t *testing.T) {
	cause := errors.New("fetch transport failed")
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
			return "", nil
		case "git branch --show-current":
			return "main", nil
		case "git rev-parse HEAD":
			return "before", nil
		case "git fetch origin main":
			return "", cause
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	layout := repositoryLayout{primaryRoot: "/repo", worktrees: []gitWorktree{{path: "/repo", head: "before", branch: "refs/heads/main"}}}
	_, err := application.synchronizeBase(layout, "")
	if !errors.Is(err, cause) {
		t.Fatalf("synchronizeBase error = %v, want cause %v", err, cause)
	}
}

func TestBaseSynchronizationRefusesConcurrentPrimaryMove(t *testing.T) {
	headReads := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
			return "", nil
		case "git branch --show-current":
			return "main", nil
		case "git rev-parse HEAD":
			headReads++
			if headReads == 1 {
				return "before", nil
			}
			return "moved", nil
		case "git fetch origin main":
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	layout := repositoryLayout{primaryRoot: "/repo", worktrees: []gitWorktree{{path: "/repo", head: "before", branch: "refs/heads/main"}}}
	_, err := application.synchronizeBase(layout, "")
	if err == nil || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("synchronizeBase error = %v, want concurrent-change refusal", err)
	}
}

func TestBaseSynchronizationRefusesMoveDuringSubmoduleUpdate(t *testing.T) {
	headReads := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git status --porcelain=v1 --untracked-files=all --ignore-submodules=none":
			return "", nil
		case "git branch --show-current":
			return "main", nil
		case "git rev-parse HEAD":
			headReads++
			if headReads <= 3 {
				return "before", nil
			}
			return "moved", nil
		case "git fetch origin main":
			return "", nil
		case "git rev-parse origin/main", "git rev-list --left-right --count HEAD...origin/main":
			if command == "git rev-parse origin/main" {
				return "before", nil
			}
			return "0 0", nil
		case "git config --get-regexp ^submodule\\..*\\.update$":
			return "", errors.New("exit status 1")
		case "git submodule update --init --recursive":
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	layout := repositoryLayout{primaryRoot: "/repo", worktrees: []gitWorktree{{path: "/repo", head: "before", branch: "refs/heads/main"}}}
	_, err := application.synchronizeBase(layout, "")
	if err == nil || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("synchronizeBase error = %v, want submodule-update concurrent refusal", err)
	}
}

func TestFinalizeBaseSnapshotAcceptsDescendantContainingMerge(t *testing.T) {
	headReads := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git rev-parse HEAD":
			headReads++
			if headReads == 1 {
				return "base", nil
			}
			return "descendant", nil
		case "git rev-parse origin/main":
			return "descendant", nil
		case "git merge-base --is-ancestor base descendant", "git merge-base --is-ancestor merge descendant":
			return "", nil
		case "git merge --ff-only origin/main":
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	fetched := fetchedBase{
		primary: cleanPrimary{layout: repositoryLayout{primaryRoot: "/repo"}},
		origin:  "base",
	}
	got, after, retry, err := application.finalizeBaseSnapshot(fetched, "base", "merge")
	if err != nil {
		t.Fatalf("finalizeBaseSnapshot: %v", err)
	}
	if !retry || after != "descendant" || got.origin != "descendant" {
		t.Fatalf("final snapshot = %#v, %q, retry=%t; want descendant retry", got, after, retry)
	}
}

func TestFinalizeBaseSnapshotRejectsDescendantWithoutMerge(t *testing.T) {
	headReads := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git rev-parse HEAD":
			headReads++
			return "base", nil
		case "git rev-parse origin/main":
			return "descendant", nil
		case "git merge-base --is-ancestor base descendant":
			return "", nil
		case "git merge-base --is-ancestor merge descendant":
			return "", errors.New("exit status 1")
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	fetched := fetchedBase{
		primary: cleanPrimary{layout: repositoryLayout{primaryRoot: "/repo"}},
		origin:  "base",
	}
	_, _, _, err := application.finalizeBaseSnapshot(fetched, "base", "merge")
	if err == nil || !strings.Contains(err.Error(), "does not contain completed merge") {
		t.Fatalf("finalizeBaseSnapshot error = %v, want merge-containment refusal", err)
	}
}

func TestSubmoduleRefreshObservesConcurrentOriginDescendant(t *testing.T) {
	headReads := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git config --get-regexp ^submodule\\..*\\.update$":
			return "", errors.New("exit status 1")
		case "git submodule update --init --recursive", "git fetch origin main":
			return "", nil
		case "git rev-parse HEAD":
			headReads++
			return "base", nil
		case "git rev-parse origin/main":
			return "descendant", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	fetched := fetchedBase{
		primary: cleanPrimary{layout: repositoryLayout{primaryRoot: "/repo"}},
		origin:  "base",
	}
	origin, err := application.updateAndCheckSubmodules(fetched, "base")
	if err != nil {
		t.Fatalf("updateAndCheckSubmodules: %v", err)
	}
	if origin != "descendant" || headReads != 2 {
		t.Fatalf("refreshed origin = %q with %d HEAD reads, want descendant and two reads", origin, headReads)
	}
}

func TestSubmoduleUpdatePolicyAllowsCheckoutOnly(t *testing.T) {
	for _, policy := range []string{"submodule.foo.update checkout", "submodule.foo.update checkout\nsubmodule.bar.update checkout\n"} {
		if err := validateSubmoduleUpdatePolicy(policy); err != nil {
			t.Fatalf("validateSubmoduleUpdatePolicy(%q): %v", policy, err)
		}
	}
	for _, policy := range []string{"merge", "rebase", "!custom command", "none"} {
		if err := validateSubmoduleUpdatePolicy("submodule.foo.update " + policy); err == nil {
			t.Fatalf("validateSubmoduleUpdatePolicy(%q) accepted unsupported policy", policy)
		}
	}
}

func TestSubmoduleUpdatePolicyChecksGitConfigWithoutGitmodules(t *testing.T) {
	root := t.TempDir()
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if command == "git config --get-regexp ^submodule\\..*\\.update$" {
			return "submodule.foo.update rebase", nil
		}
		return "", errors.New("exit status 1")
	}}
	if err := application.checkSubmoduleUpdatePolicy(root); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("checkSubmoduleUpdatePolicy error = %v, want unsupported-policy refusal", err)
	}
}

func TestWorkflowHelpExposesBaseSyncRecovery(t *testing.T) {
	var output bytes.Buffer
	application := app{stdout: &output}
	if err := application.usage(); err != nil {
		t.Fatalf("usage: %v", err)
	}
	for _, text := range []string{"workflowctl base-sync", "sync              # Project status + claim-ref fetches; no base sync", "workflowctl pr recover PR"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("usage omits %q:\n%s", text, output.String())
		}
	}
}
