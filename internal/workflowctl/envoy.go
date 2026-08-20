package workflowctl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envoyPromptPath       = "evals/envoy/prompt.md"
	envoyReportPath       = "evals/envoy/report.md"
	envoyFixturePath      = "evals/envoy/fixture"
	envoyInventoryCommand = "go tool conformance inventory -root ."
	envoyDocsCommand      = "go doc ."
)

type envoyFixtureFile struct {
	path    string
	markers []string
}

var envoyFixtureFiles = []envoyFixtureFile{
	{
		path: "testdata/w3c/xsdtests/suite.xml",
		markers: []string{
			"<testSuite ",
			"<testSetRef ",
			"xlink:href=\"sets/envoy.testSet\"",
		},
	},
	{
		path: "testdata/w3c/xsdtests/extra-suite.xml",
		markers: []string{
			"<testSuite ",
			"<testSetRef ",
			"xlink:href=\"sets/envoy.testSet\"",
		},
	},
	{
		path: "testdata/w3c/xsdtests/sets/envoy.testSet",
		markers: []string{
			"<testSet ",
			"<testGroup ",
			"<schemaTest ",
			"<schemaDocument ",
			"<current ",
		},
	},
	{
		path:    "testdata/w3c/xsdtests/sets/envoy.xsd",
		markers: []string{"<schema ", "targetNamespace=\"urn:envoy-fixture\""},
	},
}

var envoyPromptRequirements = []string{
	"Do not inspect repository source",
	"go tool conformance inventory -root .",
	"go doc .",
	"deterministic catalog metadata only",
	"not schema or instance test execution",
	"`ParseSchema` is the supported public parser API, but this Envoy surface evaluation deliberately does not execute parser APIs; it evaluates only the README, `go doc .`, and documented inventory CLI.",
	"report.md",
	"documentation, public API, CLI, or environment",
	"boundary evidence separate",
}

var envoyReportRequirements = []string{
	"# Envoy surface report",
	"## Documentation",
	"Failure class: `documentation`",
	"## Public API",
	"Failure class: `public-api`",
	"## CLI",
	"Failure class: `cli`",
	"## Environment",
	"Failure class: `environment`",
	"## Boundary evidence",
	"Source inspection: `not used`",
	"metadata only; it is not test execution evidence",
}

var envoyForbiddenPromptFragments = []string{
	"go doc -src",
	"go doc -all",
	"go doc -u",
	"go doc -cmd",
	"go list -json",
	"git grep",
	"git show",
	"git diff",
	"git log",
	"go run ",
	"go generate ",
	"rg ",
	"grep ",
	"find ",
	"sed ",
	"awk ",
	"cat ",
	"less ",
	"more ",
	"head ",
	"tail ",
	"internal/",
	"cmd/",
	".go",
	"import \"",
	"read source code",
	"inspect source code",
	"open source",
	"view source",
	"import ",
}

var envoyUnavailableAPIFragments = []string{
	"goxsd9.Parse(",
	"goxsd9.Validate(",
	"goxsd9.Generate(",
	"goxsd9.ValidateSchema(",
	"goxsd9.GenerateGo(",
	"NewParser(",
	"NewValidator(",
	"NewGenerator(",
	"ValidateSchema(",
	"GenerateGo(",
}

func (a app) checkEnvoySurface(root string) error {
	if err := validateEnvoySurfaceAssets(root); err != nil {
		return err
	}
	return a.checkEnvoyCommands(root)
}

func validateEnvoySurfaceAssets(root string) error {
	prompt, err := readEnvoyFile(root, envoyPromptPath)
	if err != nil {
		return err
	}
	if promptErr := validateEnvoyPrompt(prompt); promptErr != nil {
		return fmt.Errorf("validate %s: %w", envoyPromptPath, promptErr)
	}

	report, err := readEnvoyFile(root, envoyReportPath)
	if err != nil {
		return err
	}
	if err := validateEnvoyReport(report); err != nil {
		return fmt.Errorf("validate %s: %w", envoyReportPath, err)
	}

	return validateEnvoyFixture(root)
}

func validateEnvoyPrompt(prompt string) error {
	for _, required := range envoyPromptRequirements {
		if !strings.Contains(prompt, required) {
			return fmt.Errorf("missing required boundary or command text %q", required)
		}
	}
	for _, forbidden := range envoyForbiddenPromptFragments {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			return fmt.Errorf("contains forbidden source or unsupported-API instruction %q", forbidden)
		}
	}
	for _, unavailable := range envoyUnavailableAPIFragments {
		if strings.Contains(prompt, unavailable) {
			return fmt.Errorf("contains unavailable API instruction %q", unavailable)
		}
	}

	commands, err := envoyShellCommands(prompt)
	if err != nil {
		return err
	}
	if len(commands) != 2 || commands[0] != envoyInventoryCommand || commands[1] != envoyDocsCommand {
		return fmt.Errorf("shell command surface must be %q followed by %q", envoyInventoryCommand, envoyDocsCommand)
	}
	return nil
}

func envoyShellCommands(prompt string) ([]string, error) {
	commands := make([]string, 0, 2)
	inShellFence := false
	for lineNumber, line := range strings.Split(prompt, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inShellFence {
			if trimmed == "```sh" {
				inShellFence = true
				continue
			}
			if strings.HasPrefix(trimmed, "```") {
				return nil, fmt.Errorf("line %d starts an unsupported code fence", lineNumber+1)
			}
			continue
		}
		if trimmed == "```" {
			inShellFence = false
			continue
		}
		if trimmed == "" {
			return nil, fmt.Errorf("line %d has an empty shell command", lineNumber+1)
		}
		commands = append(commands, trimmed)
	}
	if inShellFence {
		return nil, errors.New("shell code fence is not closed")
	}
	return commands, nil
}

func validateEnvoyReport(report string) error {
	for _, required := range envoyReportRequirements {
		if !strings.Contains(report, required) {
			return fmt.Errorf("missing required report text %q", required)
		}
	}
	lines := strings.Count(report, "\n") + 1
	if lines > 40 {
		return fmt.Errorf("report has %d lines; limit is 40", lines)
	}
	words := len(strings.Fields(report))
	if words > 240 {
		return fmt.Errorf("report has %d words; limit is 240", words)
	}
	if strings.Contains(report, "[TODO:") {
		return errors.New("report contains a template TODO")
	}
	return nil
}

func validateEnvoyFixture(root string) error {
	fixtureRoot := filepath.Join(root, filepath.FromSlash(envoyFixturePath))
	for _, fixtureFile := range envoyFixtureFiles {
		data, err := readEnvoyFile(fixtureRoot, fixtureFile.path)
		if err != nil {
			return err
		}
		for _, marker := range fixtureFile.markers {
			if !strings.Contains(data, marker) {
				return fmt.Errorf("fixture %s is missing %q", fixtureFile.path, marker)
			}
		}
	}
	return nil
}

func readEnvoyFile(root, relativePath string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	// #nosec G304 -- path is built from fixed repository-owned Envoy asset paths.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func (a app) checkEnvoyCommands(root string) error {
	fixtureRoot := filepath.Join(root, filepath.FromSlash(envoyFixturePath))
	first, err := a.command(fixtureRoot, "go", "tool", "conformance", "inventory", "-root", ".")
	if err != nil {
		return fmt.Errorf("run Envoy inventory fixture: %w", err)
	}
	second, err := a.command(fixtureRoot, "go", "tool", "conformance", "inventory", "-root", ".")
	if err != nil {
		return fmt.Errorf("repeat Envoy inventory fixture: %w", err)
	}
	if first != second {
		return errors.New("envoy inventory fixture output is not deterministic")
	}
	for _, marker := range []string{
		"W3C XML Schema test catalog inventory\n",
		"test-sets: ",
		"cases: ",
		"# Catalog metadata only; no schema or instance tests are executed.",
	} {
		if !strings.Contains(first, marker) {
			return fmt.Errorf("envoy inventory output is missing %q", marker)
		}
	}

	docs, err := a.command(root, "go", "doc", ".")
	if err != nil {
		return fmt.Errorf("inspect public package documentation: %w", err)
	}
	if !strings.Contains(docs, "package goxsd9") {
		return errors.New("public package documentation has no goxsd9 package declaration")
	}
	return nil
}
