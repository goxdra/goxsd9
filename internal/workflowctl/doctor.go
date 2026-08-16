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
	baseDetail, baseErr := a.checkDevelopLaunch(root)
	if baseErr != nil {
		if err := writeLine(a.stdout, "[fail] Git base: %v", baseErr); err != nil {
			return fmt.Errorf("write doctor result: %w", err)
		}
		return stateError("doctor found 1 environment failure(s)")
	}
	if err := writeLine(a.stdout, "[ok] Git base: %s", firstLine(baseDetail)); err != nil {
		return fmt.Errorf("write doctor result: %w", err)
	}

	checks := []doctorCheck{
		{name: "Go", run: a.checkGo},
		{name: "Git", run: func() (string, error) { return a.command(root, "git", "--version") }},
		{name: "Codex CLI", run: func() (string, error) { return a.command(root, "codex", "--version") }},
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

func (a app) checkDevelopLaunch(callerRoot string) (string, error) {
	layout, err := a.repositoryLayout(callerRoot)
	if err != nil {
		return "", err
	}
	if !samePath(callerRoot, layout.primaryRoot) {
		return "", stateError("scheduled Develop must launch from canonical primary checkout %q, not linked or nested checkout %q; run %s in the primary, then relaunch", layout.primaryRoot, callerRoot, baseSyncCommand)
	}
	primary, err := a.cleanPrimary(layout)
	if err != nil {
		return "", err
	}
	fetched, err := a.fetchBase(primary)
	if err != nil {
		return "", err
	}
	if fetched.ahead != 0 || fetched.behind != 0 {
		return "", stateError("canonical main %s fetched origin/main (local-only=%d, remote-only=%d); run %s before the next Develop launch", relationDescription(fetched.ahead, fetched.behind), fetched.ahead, fetched.behind, baseSyncCommand)
	}
	if err := a.checkPinnedSubmodules(layout.primaryRoot); err != nil {
		return "", fmt.Errorf("canonical primary is not recursively pinned and ready: %w; run %s", err, baseSyncCommand)
	}
	return fmt.Sprintf("%s (main %s equals fetched origin/main; recursive submodules ready)", layout.primaryRoot, fetched.origin), nil
}

func relationDescription(ahead, behind int) string {
	if ahead == 0 && behind == 0 {
		return "equals"
	}
	if ahead == 0 {
		return "is behind"
	}
	if behind == 0 {
		return "is ahead of"
	}
	return "diverges from"
}

func (a app) checkProject(root string) (string, error) {
	_, err := a.command(root, "gh", "project", "view", strconv.Itoa(projectNumber), "--owner", owner, "--format", "json")
	if err != nil {
		return "", err
	}
	items, err := a.projectItems(root)
	if err != nil {
		return "", err
	}
	fields, err := a.projectFields(root)
	if err != nil {
		return "", err
	}
	if err := verifyProjectFields(fields); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s Roadmap (#%d, %d items)", repository, projectNumber, items.TotalCount), nil
}

func verifyProjectFields(fields projectFieldList) error {
	required := []struct {
		field   string
		options []string
	}{
		{field: "Status", options: []string{"Backlog", "Ready", "Picked", "Done"}},
		{field: "Priority", options: []string{"P0", "P1", "P2", "P3", "P4"}},
		{field: "Effort", options: []string{"XS", "S", "M", "L", "XL"}},
		{field: "Phase", options: []string{"Bootstrap", "Vertical Slice", "Schema Model", "Validation", "Codegen", "Conformance", "XPath"}},
	}
	for _, requirement := range required {
		for _, option := range requirement.options {
			if _, _, err := fields.option(requirement.field, option); err != nil {
				return err
			}
		}
	}
	return nil
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
