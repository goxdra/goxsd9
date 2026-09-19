package workflowctl

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goxdra/goxsd9/internal/feature"
)

type qualityCheck struct {
	name string
	run  func() error
}

func (a app) runCheck(args []string) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	skipLint := flags.Bool("skip-lint", false, "skip golangci-lint")
	if err := flags.Parse(args); err != nil {
		return usageError("check: %v", err)
	}
	if flags.NArg() != 0 {
		return usageError("check takes no positional arguments")
	}

	root, err := a.root()
	if err != nil {
		return err
	}
	checks := a.qualityChecks(root, *skipLint)
	for _, check := range checks {
		if err := check.run(); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
		if err := writeLine(a.stdout, "[ok] %s", check.name); err != nil {
			return fmt.Errorf("write check result: %w", err)
		}
	}
	return nil
}

func (a app) qualityChecks(root string, skipLint bool) []qualityCheck {
	checks := make([]qualityCheck, 0, 10)
	checks = append(checks,
		qualityCheck{name: "documentation", run: func() error { return a.checkDocs(root, false) }},
		qualityCheck{name: "unsupported feature registry", run: feature.ValidateRegistry},
		qualityCheck{name: "specification manifest", run: func() error { return checkSpecManifest(root) }},
		qualityCheck{name: "source guard", run: func() error { return guardSource(root) }},
		qualityCheck{name: "Envoy surface", run: func() error { return a.checkEnvoySurface(root) }},
		qualityCheck{name: "gofmt", run: func() error { return a.checkGofmt(root) }},
		qualityCheck{name: "git whitespace", run: func() error { return a.runQuiet(root, "git", "diff", "--check") }},
		qualityCheck{name: "module tidy", run: func() error { return a.runQuiet(root, "go", "mod", "tidy", "-diff") }},
		qualityCheck{name: "go vet", run: func() error { return a.runQuiet(root, "go", "vet", "./...") }},
		qualityCheck{name: "go test", run: func() error { return a.runQuiet(root, "go", "test", "./...") }},
		qualityCheck{name: "submodule pin", run: func() error { _, err := a.checkSubmodule(root); return err }},
	)
	if skipLint {
		return checks
	}
	return append(checks, qualityCheck{
		name: "golangci-lint",
		run:  func() error { return a.runGolangCILint(root) },
	})
}

func (a app) runGolangCILint(root string) error {
	cache, err := a.command(root, "git", "rev-parse", "--path-format=absolute", "--git-path", "golangci-lint-cache")
	if err != nil {
		return fmt.Errorf("resolve golangci-lint cache: %w", err)
	}
	cache = strings.TrimSpace(cache)
	if cache == "" {
		return errors.New("resolve golangci-lint cache: git returned an empty path")
	}
	if strings.ContainsAny(cache, "\r\n\x00") {
		return errors.New("resolve golangci-lint cache: git returned an invalid path")
	}
	if !filepath.IsAbs(cache) {
		return fmt.Errorf("resolve golangci-lint cache: git returned non-absolute path %q", cache)
	}
	cache = filepath.Clean(cache)
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return fmt.Errorf("create golangci-lint cache %q: %w", cache, err)
	}
	if err := a.runWithEnv(root, []string{"GOLANGCI_LINT_CACHE=" + cache}, "golangci-lint", "run"); err != nil {
		return fmt.Errorf("run golangci-lint: %w", err)
	}
	return nil
}

func (a app) checkGofmt(root string) error {
	files, err := goFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"-l"}, files...)
	output, err := a.command(root, "gofmt", args...)
	if err != nil {
		return err
	}
	if output != "" {
		return fmt.Errorf("unformatted files: %s", strings.ReplaceAll(output, "\n", ", "))
	}
	return nil
}

func (a app) runQuiet(dir, name string, args ...string) error {
	_, err := a.command(dir, name, args...)
	return err
}

func (a app) runWithEnv(dir string, env []string, name string, args ...string) error {
	_, err := a.commandWithEnv(dir, env, name, args...)
	return err
}
