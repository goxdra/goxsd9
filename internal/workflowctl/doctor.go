package workflowctl

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type doctorCheck struct {
	name string
	run  func() (string, error)
}

func (a app) runDoctor(args []string) error {
	if len(args) != 0 {
		return usageError("doctor takes no arguments")
	}

	root, err := a.root()
	if err != nil {
		return err
	}

	checks := []doctorCheck{
		{name: "Go", run: a.checkGo},
		{name: "Git", run: func() (string, error) { return a.command(root, "git", "--version") }},
		{name: "GitHub CLI", run: a.checkGH},
		{name: "ripgrep", run: func() (string, error) { return a.command(root, "rg", "--version") }},
		{name: "golangci-lint", run: a.checkGolangCILint},
		{name: "origin", run: func() (string, error) { return a.checkOrigin(root) }},
		{name: "GitHub auth", run: func() (string, error) { return a.command(root, "gh", "auth", "status") }},
		{name: "GitHub Project", run: func() (string, error) { return a.checkProject(root) }},
		{name: "W3C submodule", run: func() (string, error) { return a.checkSubmodule(root) }},
		{name: "repository skills", run: func() (string, error) { return checkSkills(root) }},
	}

	failures := 0
	for _, check := range checks {
		detail, checkErr := check.run()
		if checkErr != nil {
			failures++
			if err := writeLine(a.stdout, "[fail] %s: %v", check.name, checkErr); err != nil {
				return fmt.Errorf("write doctor result: %w", err)
			}
			continue
		}
		if err := writeLine(a.stdout, "[ok] %s: %s", check.name, firstLine(detail)); err != nil {
			return fmt.Errorf("write doctor result: %w", err)
		}
	}

	if failures != 0 {
		return stateError("doctor found %d environment failure(s)", failures)
	}
	return nil
}

func (a app) checkGo() (string, error) {
	version, err := a.command("", "go", "version")
	if err != nil {
		return "", err
	}
	if !strings.Contains(version, "go1.26.") {
		return "", fmt.Errorf("need Go 1.26.x, found %s", version)
	}
	return version, nil
}

func (a app) checkGH() (string, error) {
	return a.command("", "gh", "--version")
}

func (a app) checkGolangCILint() (string, error) {
	version, err := a.command("", "golangci-lint", "version")
	if err != nil {
		return "", err
	}
	if !strings.Contains(version, "2.12.2") {
		return "", fmt.Errorf("need golangci-lint 2.12.2, found %s", firstLine(version))
	}
	return version, nil
}

func (a app) checkOrigin(root string) (string, error) {
	remote, err := a.command(root, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	if !strings.Contains(remote, repositoryKey) {
		return "", fmt.Errorf("origin %q is not %s", remote, repositoryKey)
	}
	return remote, nil
}

func (a app) checkProject(root string) (string, error) {
	_, err := a.command(root, "gh", "project", "view", strconv.Itoa(projectNumber), "--owner", owner, "--format", "json")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s Roadmap (#%d)", repository, projectNumber), nil
}

func (a app) checkSubmodule(root string) (string, error) {
	status, err := a.command(root, "git", "submodule", "status", "testdata/w3c/xsdtests")
	if err != nil {
		return "", err
	}
	if status == "" {
		return "", errors.New("submodule has no status")
	}
	if status[0] == '-' {
		return "", errors.New("submodule is not initialized")
	}
	if status[0] == '+' {
		return "", errors.New("submodule is not at its pinned commit")
	}
	return strings.TrimSpace(status), nil
}

func checkSkills(root string) (string, error) {
	names := []string{"backlog", "develop", "retro"}
	for _, name := range names {
		path := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
		if err := requireRegularFile(path); err != nil {
			return "", err
		}
	}
	return strings.Join(names, ", "), nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
