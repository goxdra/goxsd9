package workflowctl

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type documentRule struct {
	path     string
	charter  string
	maxLines int
	maxWords int
}

var documentRules = []documentRule{
	{path: ".agents/skills/backlog/SKILL.md", charter: "executable backlog procedure", maxLines: 55, maxWords: 375},
	{path: ".agents/skills/develop/SKILL.md", charter: "executable development procedure", maxLines: 120, maxWords: 825},
	{path: ".agents/skills/retro/SKILL.md", charter: "executable retrospective procedure", maxLines: 50, maxWords: 300},
	{path: ".github/pull_request_template.md", charter: "pull request evidence headings", maxLines: 35, maxWords: 175},
	{path: "AGENTS.md", charter: "durable repository invariants", maxLines: 145, maxWords: 1150},
	{path: "ARCHITECTURE.md", charter: "current design", maxLines: 160, maxWords: 950},
	{path: "PLAN.md", charter: "phased outcomes and exit measures", maxLines: 110, maxWords: 650},
	{path: "README.md", charter: "concise user entrypoint", maxLines: 70, maxWords: 375},
	{path: "docs/decisions/0001-foundations.md", charter: "durable rationale and supersession", maxLines: 40, maxWords: 250},
	{path: "docs/decisions/0002-precision-decimal.md", charter: "precisionDecimal semantic and representation contract", maxLines: 140, maxWords: 1050},
	{path: "docs/operations.md", charter: "scheduler and operator contract", maxLines: 60, maxWords: 525},
}

func (a app) runDocs(args []string) error {
	if len(args) == 1 && args[0] == "check" {
		root, err := a.root()
		if err != nil {
			return err
		}
		return a.checkDocs(root, true)
	}
	if len(args) == 0 || args[0] != "audit" {
		return usageError("usage: workflowctl docs check | docs audit --base REF")
	}
	flags := flag.NewFlagSet("docs audit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("base", "", "Git base reference")
	if err := flags.Parse(args[1:]); err != nil {
		return usageError("docs audit: %v", err)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*base) == "" {
		return usageError("usage: workflowctl docs audit --base REF")
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	return a.auditDocs(root, *base)
}

func (a app) checkDocs(root string, report bool) error {
	if err := validateDocumentRegistry(); err != nil {
		return err
	}
	paths, err := a.repositoryPaths(root)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if !isDurableMarkdown(path) {
			continue
		}
		if _, ok := documentRuleFor(path); !ok {
			return fmt.Errorf("unregistered durable Markdown document %q", path)
		}
	}
	for _, rule := range documentRules {
		if err := checkDocument(filepath.Join(root, filepath.FromSlash(rule.path)), rule); err != nil {
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

func validateDocumentRegistry() error {
	return validateDocumentRules(documentRules)
}

func validateDocumentRules(rules []documentRule) error {
	for index, rule := range rules {
		if rule.path == "" || rule.charter == "" || rule.maxLines < 1 || rule.maxWords < 1 {
			return fmt.Errorf("invalid documentation rule at index %d", index)
		}
		if filepath.IsAbs(rule.path) || filepath.ToSlash(filepath.Clean(rule.path)) != rule.path {
			return fmt.Errorf("documentation rule path %q is not a clean repository path", rule.path)
		}
		if index > 0 && rules[index-1].path >= rule.path {
			return fmt.Errorf("documentation rules are not unique and sorted at %q", rule.path)
		}
	}
	return nil
}

func (a app) repositoryPaths(root string) ([]string, error) {
	output, err := a.gitRaw(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("list repository paths: %w", err)
	}
	paths, err := parseNULFields(output)
	if err != nil {
		return nil, fmt.Errorf("parse repository paths: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func documentRuleFor(path string) (documentRule, bool) {
	for _, rule := range documentRules {
		if rule.path == path {
			return rule, true
		}
	}
	return documentRule{}, false
}

func isDurableMarkdown(path string) bool {
	if !isMarkdownPath(path) {
		return false
	}
	if strings.HasPrefix(path, "evals/agent/") || strings.HasPrefix(path, "evals/envoy/") {
		return false
	}
	return !strings.HasPrefix(path, "testdata/w3c/xsdtests/")
}

func isMarkdownPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

type documentStats struct {
	lines int
	words int
}

func checkDocument(path string, rule documentRule) error {
	stats, err := readDocumentStats(path)
	if err != nil {
		return err
	}
	return validateDocumentStats(path, stats, rule)
}

func validateDocumentStats(path string, stats documentStats, rule documentRule) error {
	if stats.lines > rule.maxLines {
		return fmt.Errorf("%s has %d lines; limit is %d", path, stats.lines, rule.maxLines)
	}
	if stats.words > rule.maxWords {
		return fmt.Errorf("%s has %d words; limit is %d", path, stats.words, rule.maxWords)
	}
	return nil
}

func readDocumentStats(path string) (documentStats, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return documentStats{}, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return documentStats{}, fmt.Errorf("%s is not a regular file", path)
	}
	// #nosec G304 -- callers provide fixed registry paths or validated repository paths.
	file, err := os.Open(path)
	if err != nil {
		return documentStats{}, fmt.Errorf("open %s: %w", path, err)
	}
	stats, scanErr := scanDocument(file)
	closeErr := file.Close()
	if scanErr != nil {
		if closeErr != nil {
			return documentStats{}, fmt.Errorf("scan %s: %w", path, errors.Join(scanErr, closeErr))
		}
		return documentStats{}, fmt.Errorf("scan %s: %w", path, scanErr)
	}
	if closeErr != nil {
		return documentStats{}, fmt.Errorf("close %s: %w", path, closeErr)
	}
	return stats, nil
}

func scanDocument(file *os.File) (documentStats, error) {
	stats := documentStats{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !utf8.ValidString(line) {
			return documentStats{}, fmt.Errorf("line %d is not valid UTF-8", stats.lines+1)
		}
		stats.lines++
		stats.words += len(strings.Fields(line))
		if strings.Contains(line, "[TODO:") {
			return documentStats{}, fmt.Errorf("line %d contains a template TODO", stats.lines)
		}
	}
	if err := scanner.Err(); err != nil {
		return documentStats{}, err
	}
	return stats, nil
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
