package workflowctl

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type documentRule struct {
	path     string
	maxLines int
}

func (a app) runDocs(args []string) error {
	if len(args) != 1 || args[0] != "check" {
		return usageError("usage: workflowctl docs check")
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	return a.checkDocs(root, true)
}

func (a app) checkDocs(root string, report bool) error {
	rules := []documentRule{
		{path: "README.md", maxLines: 180},
		{path: "ARCHITECTURE.md", maxLines: 320},
		{path: "PLAN.md", maxLines: 220},
		{path: "AGENTS.md", maxLines: 300},
	}
	for _, rule := range rules {
		if err := checkDocument(filepath.Join(root, rule.path), rule.maxLines); err != nil {
			return err
		}
	}
	if err := validateSkills(root); err != nil {
		return err
	}
	if !report {
		return nil
	}
	return writeLine(a.stdout, "documentation and skill structure: ok")
}

func checkDocument(path string, maxLines int) error {
	// #nosec G304 -- path is built from the repository root and fixed filenames.
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	lines := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines++
		if strings.Contains(scanner.Text(), "[TODO:") {
			if err := file.Close(); err != nil {
				return fmt.Errorf("close %s: %w", path, err)
			}
			return fmt.Errorf("%s:%d contains a template TODO", path, lines)
		}
	}
	if err := scanner.Err(); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("scan %s: %w", path, errors.Join(err, closeErr))
		}
		return fmt.Errorf("scan %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if lines > maxLines {
		return fmt.Errorf("%s has %d lines; limit is %d", path, lines, maxLines)
	}
	return nil
}

func validateSkills(root string) error {
	names := []string{"backlog", "develop", "retro"}
	for _, name := range names {
		if err := validateSkill(root, name); err != nil {
			return err
		}
	}
	return nil
}

func validateSkill(root, name string) error {
	skillPath := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
	// #nosec G304 -- skillPath is built from the repository root and a fixed name.
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", skillPath, err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\nname: "+name+"\n") {
		return fmt.Errorf("%s has invalid frontmatter", skillPath)
	}
	if !strings.Contains(text, "\ndescription: ") {
		return fmt.Errorf("%s has no description", skillPath)
	}
	if strings.Contains(text, "[TODO:") {
		return fmt.Errorf("%s contains a template TODO", skillPath)
	}

	metadataPath := filepath.Join(root, ".agents", "skills", name, "agents", "openai.yaml")
	// #nosec G304 -- metadataPath is built from the repository root and a fixed name.
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", metadataPath, err)
	}
	if !strings.Contains(string(metadata), "$"+name) {
		return fmt.Errorf("%s default prompt does not invoke $%s", metadataPath, name)
	}
	return nil
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}
