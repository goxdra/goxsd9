package workflowctl

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunGolangCILintUsesWorktreeCache(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, ".git", "worktrees", "issue-424", "golangci-lint-cache")
	wantGitArgs := []string{"rev-parse", "--path-format=absolute", "--git-path", "golangci-lint-cache"}
	wantLintArgs := []string{"run"}
	type call struct {
		name string
		args []string
		env  []string
	}
	var calls []call
	a := app{
		executeCommand: func(dir string, _ io.Reader, name string, args ...string) (string, error) {
			if dir != root {
				t.Fatalf("git directory = %q, want %q", dir, root)
			}
			calls = append(calls, call{name: name, args: append([]string(nil), args...)})
			return cache, nil
		},
		executeCommandWithEnv: func(dir string, env []string, _ io.Reader, name string, args ...string) (string, error) {
			if dir != root {
				t.Fatalf("lint directory = %q, want %q", dir, root)
			}
			calls = append(calls, call{name: name, args: append([]string(nil), args...), env: append([]string(nil), env...)})
			return "", nil
		},
	}

	if err := a.runGolangCILint(root); err != nil {
		t.Fatalf("runGolangCILint: %v", err)
	}

	if !reflect.DeepEqual(calls, []call{
		{name: "git", args: wantGitArgs},
		{name: "golangci-lint", args: wantLintArgs, env: []string{"GOLANGCI_LINT_CACHE=" + cache}},
	}) {
		t.Fatalf("commands = %#v, want exact git/linter boundary", calls)
	}
	info, err := os.Stat(cache)
	if err != nil {
		t.Fatalf("stat cache: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("cache path is %q, want directory", info.Mode())
	}
}

func TestRunGolangCILintGitPathFailureSkipsLint(t *testing.T) {
	root := t.TempDir()
	gitErr := errors.New("git path unavailable")
	gitCalls := 0
	a := app{
		executeCommand: func(dir string, _ io.Reader, name string, args ...string) (string, error) {
			gitCalls++
			if dir != root || name != "git" {
				t.Fatalf("unexpected git call: dir=%q name=%q args=%q", dir, name, args)
			}
			return "", gitErr
		},
		executeCommandWithEnv: func(string, []string, io.Reader, string, ...string) (string, error) {
			t.Fatal("golangci-lint ran after git path failure")
			return "", nil
		},
	}

	err := a.runGolangCILint(root)
	if err == nil {
		t.Fatal("runGolangCILint succeeded after git path failure")
	}
	if !errors.Is(err, gitErr) {
		t.Fatalf("runGolangCILint error = %v, want cause %v", err, gitErr)
	}
	if !strings.Contains(err.Error(), "resolve golangci-lint cache") {
		t.Fatalf("runGolangCILint error = %v, want cache-resolution context", err)
	}
	if gitCalls != 1 {
		t.Fatalf("git calls = %d, want 1", gitCalls)
	}
}

func TestRunGolangCILintRejectsUnusableGitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "relative", path: "relative/cache"},
		{name: "multiple", path: "/absolute/cache\n/another/cache"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			a := app{
				executeCommand: func(string, io.Reader, string, ...string) (string, error) {
					return test.path, nil
				},
				executeCommandWithEnv: func(string, []string, io.Reader, string, ...string) (string, error) {
					t.Fatal("golangci-lint ran with unusable cache path")
					return "", nil
				},
			}
			if err := a.runGolangCILint(root); err == nil {
				t.Fatalf("runGolangCILint accepted %s git path %q", test.name, test.path)
			}
		})
	}
}

func TestCommandEnvironmentReplacesAmbientCacheForNormalAndContextPaths(t *testing.T) {
	const stale = "/removed/worktree/golangci-lint-cache"
	fresh := filepath.Join(t.TempDir(), "golangci-lint-cache")
	t.Setenv("GOLANGCI_LINT_CACHE", stale)
	a := app{ctx: context.Background()}

	tests := []struct {
		name string
		run  func([]string) (string, error)
	}{
		{
			name: "normal",
			run: func(env []string) (string, error) {
				return a.commandWithEnv("", env, "env")
			},
		},
		{
			name: "context",
			run: func(env []string) (string, error) {
				return a.commandOutputWithContextAndEnv(context.Background(), "", env, nil, "env")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := test.run([]string{"GOLANGCI_LINT_CACHE=" + fresh})
			if err != nil {
				t.Fatalf("%s command: %v", test.name, err)
			}
			var matches []string
			for _, entry := range strings.Split(output, "\n") {
				if strings.HasPrefix(entry, "GOLANGCI_LINT_CACHE=") {
					matches = append(matches, entry)
				}
			}
			if !reflect.DeepEqual(matches, []string{"GOLANGCI_LINT_CACHE=" + fresh}) {
				t.Fatalf("%s cache environment = %#v, want exactly one fresh override", test.name, matches)
			}
		})
	}
}
