package workflowctl

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	envoyPromptPath         = "evals/envoy/prompt.md"
	envoyReportPath         = "evals/envoy/report.md"
	envoyFixturePath        = "evals/envoy/fixture"
	envoyConsumerPath       = "consumer"
	envoyCLIFixturePath     = "cli"
	envoyInventoryCommand   = "go tool conformance inventory -root ."
	envoyDocsCommand        = "go doc ."
	envoyConsumerCommand    = "go run ."
	envoyCLIParseCommand    = "go run ./cmd/goxsd9 parse examples/root.xsd"
	envoyCLIValidCommand    = "go run ./cmd/goxsd9 validate examples/root.xsd examples/valid.xml"
	envoyCLIInvalidCommand  = "go run ./cmd/goxsd9 validate examples/root.xsd examples/invalid.xml"
	envoyCLIGenerateCommand = "go run ./cmd/goxsd9 generate --package envoygenerated examples/root.xsd"
)

type envoyCLICommand struct {
	step           string
	text           string
	args           []string
	expectedStatus int
}

type envoyCommandSpec struct {
	step           string
	directory      string
	env            []string
	name           string
	args           []string
	expectedStatus int
}

var envoyCLIFixtureFiles = []envoyFixtureFile{
	{
		path: "cli/go.mod",
		markers: []string{
			"module envoy-cli-fixture",
			"require github.com/goxdra/goxsd9 v0.0.0",
			"replace github.com/goxdra/goxsd9 => ../../../..",
		},
	},
	{
		path: "cli/schema.xsd",
		markers: []string{
			"<xs:schema ",
			"<xs:element name=\"count\" type=\"xs:integer\"/>",
			"<xs:element name=\"amount\" type=\"xs:decimal\"/>",
		},
	},
	{
		path:    "cli/valid.xml",
		markers: []string{"<amount>12.50</amount>"},
	},
	{
		path:    "cli/invalid.xml",
		markers: []string{"<count>not-an-integer</count>"},
	},
}

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
	{
		path: "consumer/go.mod",
		markers: []string{
			"module envoy-consumer",
			"require github.com/goxdra/goxsd9 v0.0.0",
			"replace github.com/goxdra/goxsd9 => ../../../..",
		},
	},
	{
		path: "consumer/main.go",
		markers: []string{
			"package main",
			"func run() error",
			"errors.As",
			"goxsd9.NewResolvedSource",
			"goxsd9.ParseSchema",
			"goxsd9.ValidateInstance",
			"goxsd9.FailureInvalid",
			"goxsd9.InvalidIntegerLexicalCode",
			"diagnostic.Class()",
			"diagnostic.Code()",
			"diagnostic.Loc()",
			"diagnostic.Related()",
			"diagnostic.SpecRef()",
			"goxsd9.GenerateGo",
			"bytes.Equal",
			"os.MkdirTemp",
			"os.WriteFile",
			"generated.go",
			"go test",
			"GOWORK=off",
			"step=parse result=pass",
			"step=validate-valid result=pass",
			"step=validate-invalid result=pass",
			"step=generate-repeat result=pass",
			"step=compile-generated result=pass",
		},
	},
	{
		path: "consumer/schema.xsd",
		markers: []string{
			"<xs:schema ",
			"targetNamespace=\"urn:envoy-consumer\"",
			"name=\"count\" type=\"xs:integer\"",
		},
	},
	{
		path:    "consumer/valid.xml",
		markers: []string{"<count xmlns=\"urn:envoy-consumer\">7</count>"},
	},
	{
		path:    "consumer/invalid.xml",
		markers: []string{"<count xmlns=\"urn:envoy-consumer\">not-an-integer</count>"},
	},
}

var envoyConsumerAllowedImports = []string{
	"bytes",
	"context",
	"errors",
	"fmt",
	"io",
	"os",
	"os/exec",
	"path/filepath",
	"strings",
	"github.com/goxdra/goxsd9",
}

var envoyConsumerSourceMarkers = []string{
	"func run() error",
	"errors.As",
	"goxsd9.NewResolvedSource",
	"goxsd9.ParseSchema",
	"goxsd9.ValidateInstance",
	"goxsd9.FailureInvalid",
	"goxsd9.InvalidIntegerLexicalCode",
	"diagnostic.Class()",
	"diagnostic.Code()",
	"diagnostic.Loc()",
	"diagnostic.Related()",
	"diagnostic.SpecRef()",
	"goxsd9.GenerateGo",
	"bytes.Equal",
	"os.MkdirTemp",
	"os.WriteFile",
	"generated.go",
	"go test",
	"GOWORK=off",
}

var envoyConsumerSourceMarkerCounts = []struct {
	marker string
	want   int
}{
	{marker: "goxsd9.ValidateInstance", want: 2},
	{marker: "goxsd9.GenerateGo", want: 2},
}

var envoyPromptRequirements = []string{
	"Use only README.md, `go doc .`, and public command/consumer behavior.",
	"source files, implementation-only details, test utilities, and repository import declarations are outside the permitted evidence",
	"Do not use source-viewing or search commands.",
	"go tool conformance inventory -root .",
	"go doc .",
	"go run .",
	"`ParseSchema` must succeed",
	"`ValidateInstance` must return nil for the valid instance and must return an invalid `Diagnostic`",
	"`GenerateGo` must be called twice",
	"complete returned Go source",
	"FailureInvalid",
	"XSD2001",
	"xsd11-datatypes#integer",
	"deterministic catalog metadata only",
	"not schema or instance test execution evidence",
	"product CLI, direct-choice generation, broader schema/validation features, and W3C conformance scoring",
	"documentation, api, command, generated-consumer, and environment",
	"public product CLI commands",
	"stdout",
	"stderr",
	"process status",
	"generated bytes",
	"temporary external module",
	"parse summary",
	"silent valid",
	"located invalid",
	"Do not add generated source",
	"boundary evidence separate",
	"report.md",
}

var envoyReportRequirements = []string{
	"# Envoy surface report",
	"## Documentation",
	"Failure class: `documentation`",
	"## API",
	"Failure class: `api`",
	"ParseSchema",
	"ValidateInstance",
	"GenerateGo",
	"FailureInvalid",
	"XSD2001",
	"xsd11-datatypes#integer",
	"## Command",
	"Failure class: `command`",
	"Product CLI",
	"parse summary",
	"stdout",
	"stderr",
	"process status",
	"generated bytes",
	"## Generated consumer",
	"Failure class: `generated-consumer`",
	"complete returned Go source",
	"byte-identical",
	"temporary external module",
	"## Environment",
	"Failure class: `environment`",
	"Result: <pass|fail|blocked|unevaluated>",
	"## Inventory boundary",
	"Inventory result: `unevaluated`",
	"metadata only; it is not execution evidence",
	"## Unevaluated scope",
	"Direct-choice generation: `unevaluated`",
	"Broader schema/validation features: `unevaluated`",
	"W3C conformance scoring: `unevaluated`",
	"## Boundary evidence",
	"Source inspection: `not used`",
	"Search/source-view commands: `not used`",
	"Private implementation: `not used`",
	"Test utilities: `not used`",
	"Repository imports: `not used`",
	"Allowed evidence: README, `go doc .`, and public consumer/CLI behavior.",
}

var envoyReportSections = []struct {
	heading string
	class   string
}{
	{heading: "## Documentation", class: "documentation"},
	{heading: "## API", class: "api"},
	{heading: "## Command", class: "command"},
	{heading: "## Generated consumer", class: "generated-consumer"},
	{heading: "## Environment", class: "environment"},
}

var envoyForbiddenPromptFragments = []string{
	"go doc -src",
	"go doc -all",
	"go doc -u",
	"go doc -cmd",
	"go list",
	"git grep",
	"git show",
	"git diff",
	"git log",
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
	"read source",
	"inspect source",
	"view source",
	"open source",
	"source code at",
	"use private api",
	"call private api",
	"inspect private api",
	"use test helper",
	"call test helper",
	"inspect test helper",
	"use repository imports",
	"inspect repository imports",
	"import \"github.com/goxdra/goxsd9/internal",
	"github.com/goxdra/goxsd9/internal/",
	"internal/",
	"cmd/",
	"_test.go",
}

var envoyUnavailableAPIFragments = []string{
	"goxsd9.Parse(",
	"goxsd9.Validate(",
	"goxsd9.Generate(",
	"goxsd9.ValidateSchema(",
	"NewParser(",
	"NewValidator(",
	"NewGenerator(",
	"ValidateSchema(",
}

var envoyStaleAPIFragments = []string{
	"public api is unavailable",
	"public apis are unavailable",
	"deliberately does not execute parser APIs",
	"deliberately does not execute parser, validator, or generator APIs",
	"record `parseschema` as unevaluated",
	"record `validateinstance` as unevaluated",
	"record `generatego` as unevaluated",
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
	lower := strings.ToLower(prompt)
	for _, forbidden := range envoyForbiddenPromptFragments {
		if containsEnvoyForbiddenPromptFragment(lower, strings.ToLower(forbidden)) {
			return fmt.Errorf("contains forbidden source/private instruction %q", forbidden)
		}
	}
	for _, unavailable := range envoyUnavailableAPIFragments {
		if strings.Contains(prompt, unavailable) {
			return fmt.Errorf("contains unavailable API instruction %q", unavailable)
		}
	}
	if containsStaleEnvoyAPIClaim(prompt) {
		return errors.New("contains stale claim that a public API is unavailable or unevaluated")
	}
	if envoyStreamsConflated(prompt) {
		return errors.New("prompt merges stdout and stderr evidence")
	}
	if envoyInventoryConflatesExecution(prompt) {
		return errors.New("prompt conflates catalog inventory with execution")
	}
	commands, err := envoyShellCommands(prompt)
	if err != nil {
		return err
	}
	expected := envoyShellCommandSurface()
	if !sameStrings(commands, expected) {
		return fmt.Errorf("shell command surface must contain the fixed public commands in order: %q", expected)
	}
	for _, required := range envoyPromptRequirements {
		if !strings.Contains(prompt, required) {
			return fmt.Errorf("missing required boundary or command text %q", required)
		}
	}
	return nil
}

func containsEnvoyForbiddenPromptFragment(text, forbidden string) bool {
	if forbidden != "cmd/" {
		return strings.Contains(text, forbidden)
	}
	allowed := strings.ReplaceAll(text, "go run ./cmd/goxsd9 ", "go run ")
	return strings.Contains(allowed, forbidden)
}

func envoyShellCommandSurface() []string {
	cliCommands := envoyCLICommands()
	commands := make([]string, 0, 3+len(cliCommands))
	commands = append(commands, envoyInventoryCommand, envoyDocsCommand, envoyConsumerCommand)
	for _, command := range cliCommands {
		commands = append(commands, command.text)
	}
	return commands
}

func envoyCLICommands() []envoyCLICommand {
	return []envoyCLICommand{
		{
			step:           "parse",
			text:           envoyCLIParseCommand,
			args:           []string{"run", "./cmd/goxsd9", "parse", "examples/root.xsd"},
			expectedStatus: 0,
		},
		{
			step:           "validate-valid",
			text:           envoyCLIValidCommand,
			args:           []string{"run", "./cmd/goxsd9", "validate", "examples/root.xsd", "examples/valid.xml"},
			expectedStatus: 0,
		},
		{
			step:           "validate-invalid",
			text:           envoyCLIInvalidCommand,
			args:           []string{"run", "./cmd/goxsd9", "validate", "examples/root.xsd", "examples/invalid.xml"},
			expectedStatus: 1,
		},
		{
			step:           "generate",
			text:           envoyCLIGenerateCommand,
			args:           []string{"run", "./cmd/goxsd9", "generate", "--package", "envoygenerated", "examples/root.xsd"},
			expectedStatus: 0,
		},
	}
}

func containsStaleEnvoyAPIClaim(text string) bool {
	lower := strings.ToLower(text)
	return containsStaleEnvoyAPILine(lower) || containsStaleEnvoyAPIFragment(lower)
}

func containsStaleEnvoyAPILine(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if !containsEnvoyAPIName(line) {
			continue
		}
		if strings.Contains(line, "product cli") && (containsEnvoyResultPlaceholder(line) || containsEnvoyResultVocabulary(line)) {
			continue
		}
		if containsEnvoyStaleTerm(line) {
			return true
		}
	}
	return false
}

func containsEnvoyResultPlaceholder(line string) bool {
	return strings.Contains(line, "result=<pass|fail|blocked|unevaluated>") ||
		strings.Contains(line, "result: <pass|fail|blocked|unevaluated>")
}

func containsEnvoyResultVocabulary(line string) bool {
	for _, result := range []string{"pass", "fail", "blocked", "unevaluated"} {
		if !strings.Contains(line, result) {
			return false
		}
	}
	return true
}

func containsEnvoyAPIName(line string) bool {
	apiNames := []string{
		"parseschema",
		"validateinstance",
		"generatego",
		"product cli",
		"parser api",
		"validator api",
		"generator api",
	}
	for _, apiName := range apiNames {
		if strings.Contains(line, apiName) {
			return true
		}
	}
	return false
}

func containsEnvoyStaleTerm(line string) bool {
	staleTerms := []string{
		"unevaluated",
		"unavailable",
		"not executed",
		"does not execute",
		"not exercised",
	}
	for _, staleTerm := range staleTerms {
		if strings.Contains(line, staleTerm) {
			return true
		}
	}
	return false
}

func containsStaleEnvoyAPIFragment(text string) bool {
	for _, fragment := range envoyStaleAPIFragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func envoyShellCommands(prompt string) ([]string, error) {
	commands := make([]string, 0, len(envoyShellCommandSurface()))
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

func sameStrings(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func validateEnvoyReport(report string) error {
	for _, required := range envoyReportRequirements {
		if !strings.Contains(report, required) {
			return fmt.Errorf("missing required report text %q", required)
		}
	}
	if containsStaleEnvoyAPIClaim(report) {
		return errors.New("report contains stale claim that a public API is unavailable or unevaluated")
	}
	if envoyInventoryConflatesExecution(report) {
		return errors.New("report conflates catalog inventory with execution")
	}
	if err := validateEnvoyReportSections(report); err != nil {
		return err
	}
	if err := validateEnvoyReportCommandEvidence(report); err != nil {
		return err
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

func validateEnvoyReportCommandEvidence(report string) error {
	if envoyStreamsConflated(report) {
		return errors.New("report merges stdout and stderr evidence")
	}
	if err := validateEnvoyReportFixedCommandEvidence(report); err != nil {
		return err
	}
	for _, command := range []struct {
		label  string
		status string
		stdout string
		stderr string
	}{
		{label: "Product CLI: parse:", status: "`0`", stdout: "stdout=`documents=1 components=2\\n`", stderr: "stderr=`<empty>`"},
		{label: "validate-valid:", status: "`0`", stdout: "stdout=`<empty>`", stderr: "stderr=`<empty>`"},
		{label: "validate-invalid:", status: "`1`", stdout: "stdout=`<empty>`", stderr: "stderr=`<located source_id=instance/examples/invalid.xml/location=1:8/code=XSD2001/related=schema/root.xsd:2:3/spec_ref=xsd11-datatypes#integer>`"},
		{label: "generate:", status: "`0`", stdout: "stdout=`<complete generated bytes>`", stderr: "stderr=`<empty>`"},
	} {
		line, ok := envoyReportEvidenceLine(report, command.label)
		if !ok {
			return fmt.Errorf("report is missing separate product CLI evidence %q", command.label)
		}
		for _, field := range []string{command.stdout, command.stderr, "process status=" + command.status, "result=<pass|fail|blocked|unevaluated>"} {
			if !strings.Contains(line, field) {
				return fmt.Errorf("report is missing %s in product CLI evidence %q", field, command.label)
			}
		}
	}
	generateLine, ok := envoyReportEvidenceLine(report, "generate:")
	if !ok || !strings.Contains(generateLine, "stdout=`<complete generated bytes>`") || !strings.Contains(generateLine, "generated bytes=`<first untouched stdout>`") {
		return errors.New("report is missing untouched generated-byte evidence")
	}
	return nil
}

func validateEnvoyReportFixedCommandEvidence(report string) error {
	for _, command := range []struct {
		label  string
		stdout string
		stderr string
		status string
		result string
	}{
		{label: "go doc command:", stdout: "stdout=`<package docs>`", stderr: "stderr=`<empty>`", status: "`0`", result: "result=<pass|fail|blocked|unevaluated>"},
		{label: "API consumer command:", stdout: "stdout=`<API steps>`", stderr: "stderr=`<empty>`", status: "`0`", result: "result=<pass|fail|blocked|unevaluated>"},
		{label: "Inventory command:", stdout: "stdout=`<catalog>`", stderr: "stderr=`<empty>`", status: "`0`", result: "execution result=<pass|fail|blocked|unevaluated>"},
	} {
		line, ok := envoyReportEvidenceLine(report, command.label)
		if !ok {
			return fmt.Errorf("report is missing separate fixed-command evidence %q", command.label)
		}
		for _, field := range []string{command.stdout, command.stderr, "process status=" + command.status, command.result} {
			if !strings.Contains(line, field) {
				return fmt.Errorf("report is missing %s in fixed-command evidence %q", field, command.label)
			}
		}
	}
	return nil
}

func envoyReportEvidenceLine(report, label string) (string, bool) {
	for _, line := range strings.Split(report, "\n") {
		if strings.Contains(line, label) {
			return line, true
		}
	}
	return "", false
}

func envoyStreamsConflated(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		for _, forbidden := range []string{
			"combined output",
			"merged output",
			"stdout/stderr",
			"merged stdout",
			"merged stderr",
			"combined stdout",
			"combined stderr",
			"merge stdout",
			"merge stderr",
			"stdout and stderr together",
		} {
			if strings.Contains(line, forbidden) {
				return true
			}
		}
	}
	return false
}

func envoyInventoryConflatesExecution(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		if !strings.Contains(line, "inventory") {
			continue
		}
		if strings.Contains(line, "inventory is test execution") || strings.Contains(line, "inventory is execution evidence") || strings.Contains(line, "inventory proves execution") {
			return true
		}
	}
	return false
}

func validateEnvoyReportSections(report string) error {
	for index, section := range envoyReportSections {
		start := strings.Index(report, section.heading)
		if start < 0 {
			return fmt.Errorf("missing report section %q", section.heading)
		}
		end := len(report)
		if index+1 < len(envoyReportSections) {
			next := strings.Index(report[start+len(section.heading):], envoyReportSections[index+1].heading)
			if next < 0 {
				return fmt.Errorf("report section %q has no following section", section.heading)
			}
			end = start + len(section.heading) + next
		}
		contents := report[start:end]
		for _, required := range []string{
			"Result: <pass|fail|blocked|unevaluated>",
			"Failure class: `" + section.class + "`",
			"Evidence:",
		} {
			if !strings.Contains(contents, required) {
				return fmt.Errorf("report section %q is missing %q", section.heading, required)
			}
		}
	}
	return nil
}

func validateEnvoyFixture(root string) error {
	fixtureRoot := filepath.Join(root, filepath.FromSlash(envoyFixturePath))
	if err := validateEnvoyFixtureFiles(fixtureRoot, envoyFixtureFiles); err != nil {
		return err
	}
	if err := validateEnvoyFixtureFiles(fixtureRoot, envoyCLIFixtureFiles); err != nil {
		return err
	}
	if err := validateEnvoyCLIFixtureInputs(root, fixtureRoot); err != nil {
		return err
	}
	if err := validateEnvoyConsumerDirectory(fixtureRoot); err != nil {
		return err
	}
	if err := validateEnvoyConsumerSource(fixtureRoot); err != nil {
		return err
	}
	return validateEnvoyCLIFixtureDirectory(fixtureRoot)
}

func validateEnvoyFixtureFiles(fixtureRoot string, fixtureFiles []envoyFixtureFile) error {
	for _, fixtureFile := range fixtureFiles {
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

func validateEnvoyCLIFixtureDirectory(fixtureRoot string) error {
	cliRoot := filepath.Join(fixtureRoot, filepath.FromSlash(envoyCLIFixturePath))
	entries, err := os.ReadDir(cliRoot)
	if err != nil {
		return fmt.Errorf("read Envoy CLI fixture directory: %w", err)
	}
	expected := []string{"go.mod", "schema.xsd", "valid.xml", "invalid.xml"}
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("envoy CLI fixture contains directory %q", entry.Name())
		}
		if !containsString(expected, entry.Name()) {
			return fmt.Errorf("envoy CLI fixture contains unexpected file %q", entry.Name())
		}
	}
	if len(entries) != len(expected) {
		return fmt.Errorf("envoy CLI fixture has %d files; want %d fixed assets", len(entries), len(expected))
	}
	return nil
}

func validateEnvoyCLIFixtureInputs(root, fixtureRoot string) error {
	pairs := []struct {
		fixture   string
		canonical string
	}{
		{fixture: "cli/schema.xsd", canonical: "examples/root.xsd"},
		{fixture: "cli/valid.xml", canonical: "examples/valid.xml"},
		{fixture: "cli/invalid.xml", canonical: "examples/invalid.xml"},
	}
	for _, pair := range pairs {
		fixture, err := readEnvoyFile(fixtureRoot, pair.fixture)
		if err != nil {
			return err
		}
		canonical, err := readEnvoyFile(root, pair.canonical)
		if err != nil {
			return err
		}
		if fixture != canonical {
			return fmt.Errorf("CLI fixture %s differs from canonical %s", pair.fixture, pair.canonical)
		}
	}
	return nil
}

func validateEnvoyConsumerDirectory(fixtureRoot string) error {
	consumerRoot := filepath.Join(fixtureRoot, filepath.FromSlash(envoyConsumerPath))
	entries, err := os.ReadDir(consumerRoot)
	if err != nil {
		return fmt.Errorf("read Envoy consumer directory: %w", err)
	}
	expected := []string{"go.mod", "main.go", "schema.xsd", "valid.xml", "invalid.xml"}
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("envoy consumer contains directory %q", entry.Name())
		}
		if !containsString(expected, entry.Name()) {
			return fmt.Errorf("envoy consumer contains unexpected file %q", entry.Name())
		}
	}
	if len(entries) != len(expected) {
		return fmt.Errorf("envoy consumer has %d files; want %d fixed assets", len(entries), len(expected))
	}
	return nil
}

func validateEnvoyConsumerSource(fixtureRoot string) error {
	data, err := readEnvoyFile(fixtureRoot, filepath.ToSlash(filepath.Join(envoyConsumerPath, "main.go")))
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", data, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("parse Envoy consumer imports: %w", err)
	}
	if file.Name.Name != "main" {
		return fmt.Errorf("envoy consumer package is %q; want main", file.Name.Name)
	}
	importsErr := validateEnvoyConsumerImports(file)
	if importsErr != nil {
		return importsErr
	}
	return validateEnvoyConsumerMarkers(data)
}

func validateEnvoyConsumerImports(file *ast.File) error {
	if len(file.Imports) != len(envoyConsumerAllowedImports) {
		return fmt.Errorf("envoy consumer imports %d packages; want %d public-only imports", len(file.Imports), len(envoyConsumerAllowedImports))
	}
	if err := validateEnvoyConsumerImportPaths(file); err != nil {
		return err
	}
	return validateEnvoyConsumerImportMultiplicity(file)
}

func validateEnvoyConsumerImportPaths(file *ast.File) error {
	for _, spec := range file.Imports {
		path, err := envoyImportPath(spec)
		if err != nil {
			return err
		}
		if !containsString(envoyConsumerAllowedImports, path) {
			return fmt.Errorf("envoy consumer import %q is not public or standard library", path)
		}
	}
	return nil
}

func validateEnvoyConsumerImportMultiplicity(file *ast.File) error {
	for _, importPath := range envoyConsumerAllowedImports {
		count := 0
		for _, spec := range file.Imports {
			path, err := envoyImportPath(spec)
			if err != nil {
				return err
			}
			if path == importPath {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("envoy consumer import %q appears %d times; want once", importPath, count)
		}
	}
	return nil
}

func envoyImportPath(spec *ast.ImportSpec) (string, error) {
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return "", fmt.Errorf("parse Envoy consumer import: %w", err)
	}
	return path, nil
}

func validateEnvoyConsumerMarkers(data string) error {
	for _, marker := range envoyConsumerSourceMarkers {
		if !strings.Contains(data, marker) {
			return fmt.Errorf("envoy consumer is missing public behavior marker %q", marker)
		}
	}
	for _, requirement := range envoyConsumerSourceMarkerCounts {
		if count := strings.Count(data, requirement.marker); count != requirement.want {
			return fmt.Errorf("envoy consumer marker %q appears %d times; want %d", requirement.marker, count, requirement.want)
		}
	}
	for _, forbidden := range []string{
		"github.com/goxdra/goxsd9/internal/",
		"github.com/goxdra/goxsd9/cmd/",
		"diagnostic.Error()",
		"err.Error()",
	} {
		if strings.Contains(data, forbidden) {
			return fmt.Errorf("envoy consumer contains forbidden private or text-only check %q", forbidden)
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

func (a app) runEnvoyCommand(spec envoyCommandSpec) (commandCaptureResult, error) {
	first, err := a.commandCaptureWithEnv(spec.directory, spec.env, spec.name, spec.args...)
	if err != nil {
		return commandCaptureResult{}, fmt.Errorf("run Envoy %s command: %w", spec.step, err)
	}
	second, err := a.commandCaptureWithEnv(spec.directory, spec.env, spec.name, spec.args...)
	if err != nil {
		return commandCaptureResult{}, fmt.Errorf("repeat Envoy %s command: %w", spec.step, err)
	}
	if !sameCommandCapture(first, second) {
		return commandCaptureResult{}, fmt.Errorf("envoy %s command result is not deterministic", spec.step)
	}
	if first.status != spec.expectedStatus {
		return commandCaptureResult{}, fmt.Errorf("envoy %s command status = %d; want %d", spec.step, first.status, spec.expectedStatus)
	}
	return first, nil
}

func (a app) checkEnvoyCommands(root string) error {
	fixtureRoot := filepath.Join(root, filepath.FromSlash(envoyFixturePath))
	firstInventory, err := a.runEnvoyCommand(envoyCommandSpec{
		step:           "inventory metadata",
		directory:      fixtureRoot,
		name:           "go",
		args:           []string{"tool", "conformance", "inventory", "-root", "."},
		expectedStatus: 0,
	})
	if err != nil {
		return err
	}
	streamErr := validateEnvoyCommandStreams("inventory metadata", firstInventory)
	if streamErr != nil {
		return streamErr
	}
	inventoryErr := validateEnvoyInventoryOutput(firstInventory.stdout)
	if inventoryErr != nil {
		return inventoryErr
	}

	firstDocs, err := a.runEnvoyCommand(envoyCommandSpec{
		step:           "public documentation",
		directory:      root,
		name:           "go",
		args:           []string{"doc", "."},
		expectedStatus: 0,
	})
	if err != nil {
		return err
	}
	streamErr = validateEnvoyCommandStreams("public documentation", firstDocs)
	if streamErr != nil {
		return streamErr
	}
	docsErr := validateEnvoyDocsOutput(firstDocs.stdout)
	if docsErr != nil {
		return docsErr
	}

	consumerRoot := filepath.Join(fixtureRoot, filepath.FromSlash(envoyConsumerPath))
	firstConsumer, err := a.runEnvoyCommand(envoyCommandSpec{
		step:           "public consumer",
		directory:      consumerRoot,
		env:            []string{"GOWORK=off"},
		name:           "go",
		args:           []string{"run", "."},
		expectedStatus: 0,
	})
	if err != nil {
		return err
	}
	streamErr = validateEnvoyCommandStreams("public consumer", firstConsumer)
	if streamErr != nil {
		return streamErr
	}
	consumerErr := validateEnvoyConsumerOutput(firstConsumer.stdout)
	if consumerErr != nil {
		return fmt.Errorf("validate Envoy generated-consumer evidence: %w", consumerErr)
	}
	return a.checkEnvoyCLICommands(root)
}

const envoyCLIInvalidStderr = "validate stage=validate class=invalid kind=processing source_id=instance/examples/invalid.xml location=1:8 code=XSD2001 related=schema/root.xsd:2:3 spec_ref=xsd11-datatypes#integer invalid xs:integer lexical representation\nexit status 1\n"

func (a app) checkEnvoyCLICommands(root string) error {
	commands := envoyCLICommands()
	var generated []byte
	for _, command := range commands {
		first, err := a.runEnvoyCommand(envoyCommandSpec{
			step:           "CLI " + command.step,
			directory:      root,
			name:           "go",
			args:           command.args,
			expectedStatus: command.expectedStatus,
		})
		if err != nil {
			return err
		}
		if err := validateEnvoyCLICommand(command, first); err != nil {
			return err
		}
		if command.step == "generate" {
			generated = []byte(first.stdout)
		}
	}
	if len(generated) == 0 {
		return errors.New("envoy CLI generation returned no source bytes")
	}
	if err := a.compileEnvoyGeneratedSource(root, generated); err != nil {
		return fmt.Errorf("compile Envoy CLI generated consumer: %w", err)
	}
	return nil
}

func sameCommandCapture(first, second commandCaptureResult) bool {
	return first.status == second.status && first.stdout == second.stdout && first.stderr == second.stderr
}

func validateEnvoyCommandStreams(step string, result commandCaptureResult) error {
	if result.stderr != "" {
		return fmt.Errorf("envoy %s stderr = %q; want empty", step, result.stderr)
	}
	return nil
}

func validateEnvoyCLICommand(command envoyCLICommand, result commandCaptureResult) error {
	if result.status != command.expectedStatus {
		return fmt.Errorf("envoy CLI %s status = %d; want %d", command.step, result.status, command.expectedStatus)
	}
	switch command.step {
	case "parse":
		return validateEnvoyCLIParse(result)
	case "validate-valid":
		return validateEnvoyCLIValid(result)
	case "validate-invalid":
		return validateEnvoyCLIInvalid(result)
	case "generate":
		return validateEnvoyCLIGenerate(result)
	default:
		return fmt.Errorf("envoy CLI has unknown command step %q", command.step)
	}
}

func validateEnvoyCLIParse(result commandCaptureResult) error {
	if result.stdout != "documents=1 components=2\n" {
		return fmt.Errorf("envoy CLI parse stdout = %q; want exact summary", result.stdout)
	}
	if result.stderr != "" {
		return fmt.Errorf("envoy CLI parse stderr = %q; want empty", result.stderr)
	}
	return nil
}

func validateEnvoyCLIValid(result commandCaptureResult) error {
	if result.stdout != "" || result.stderr != "" {
		return fmt.Errorf("envoy CLI valid validation streams = stdout %q stderr %q; want both empty", result.stdout, result.stderr)
	}
	return nil
}

func validateEnvoyCLIInvalid(result commandCaptureResult) error {
	if result.stdout != "" {
		return fmt.Errorf("envoy CLI invalid validation stdout = %q; want empty", result.stdout)
	}
	if result.stderr != envoyCLIInvalidStderr {
		return fmt.Errorf("envoy CLI invalid validation stderr = %q; want located diagnostic", result.stderr)
	}
	return nil
}

func validateEnvoyCLIGenerate(result commandCaptureResult) error {
	if result.stderr != "" {
		return fmt.Errorf("envoy CLI generate stderr = %q; want empty", result.stderr)
	}
	for _, marker := range []string{"package envoygenerated\n", "type Count struct", "type Amount struct", "github.com/goxdra/goxsd9"} {
		if !strings.Contains(result.stdout, marker) {
			return fmt.Errorf("envoy CLI generated stdout is missing %q", marker)
		}
	}
	return nil
}

func (a app) compileEnvoyGeneratedSource(root string, source []byte) error {
	temporary, err := os.MkdirTemp("", "goxsd9-envoy-cli-generated-")
	if err != nil {
		return fmt.Errorf("create temporary external module: %w", err)
	}

	moduleRoot, err := filepath.Abs(root)
	if err != nil {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("find repository module root: %w", err))
	}
	goMod := fmt.Sprintf("module envoy-cli-generated-consumer\n\ngo 1.26.0\n\nrequire github.com/goxdra/goxsd9 v0.0.0\n\nreplace github.com/goxdra/goxsd9 => %s\n", moduleRoot)
	if writeErr := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(goMod), 0o600); writeErr != nil {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("write temporary external module go.mod: %w", writeErr))
	}
	generatedPath := filepath.Join(temporary, "generated.go")
	if writeErr := os.WriteFile(generatedPath, source, 0o600); writeErr != nil {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("write exact generated source: %w", writeErr))
	}
	// #nosec G304 -- generatedPath is the fixed file in the private temporary module.
	readBack, err := os.ReadFile(generatedPath)
	if err != nil {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("read exact generated source: %w", err))
	}
	if !bytes.Equal(readBack, source) {
		return cleanupEnvoyGeneratedModule(temporary, errors.New("temporary generated source changed before compilation"))
	}

	result, err := a.commandCaptureWithEnv(temporary, []string{"GOWORK=off"}, "go", "test", "./...")
	if err != nil {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("run generated consumer tests: %w", err))
	}
	if result.status != 0 {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("generated consumer tests exited with status %d: stdout %q stderr %q", result.status, result.stdout, result.stderr))
	}
	// #nosec G304 -- generatedPath is the fixed file in the private temporary module.
	readBack, err = os.ReadFile(generatedPath)
	if err != nil {
		return cleanupEnvoyGeneratedModule(temporary, fmt.Errorf("re-read exact generated source: %w", err))
	}
	if !bytes.Equal(readBack, source) {
		return cleanupEnvoyGeneratedModule(temporary, errors.New("generated source changed during compilation"))
	}
	return cleanupEnvoyGeneratedModule(temporary, nil)
}

func cleanupEnvoyGeneratedModule(temporary string, cause error) error {
	removeErr := os.RemoveAll(temporary)
	if cause == nil {
		if removeErr != nil {
			return fmt.Errorf("remove temporary external module: %w", removeErr)
		}
		return nil
	}
	if removeErr != nil {
		return errors.Join(cause, fmt.Errorf("remove temporary external module: %w", removeErr))
	}
	return cause
}

func validateEnvoyInventoryOutput(output string) error {
	for _, marker := range []string{
		"W3C XML Schema test catalog inventory\n",
		"test-sets: ",
		"cases: ",
		"valid invalid other submitted accepted stable queried disputed-test disputed-spec status-missing unusable headline\n",
		"# Catalog metadata only; no schema or instance tests are executed.",
	} {
		if !strings.Contains(output, marker) {
			return fmt.Errorf("envoy inventory output is missing metadata marker %q", marker)
		}
	}
	return nil
}

func validateEnvoyDocsOutput(output string) error {
	for _, marker := range []string{
		"package goxsd9",
		"func ParseSchema(",
		"func ValidateInstance(",
		"func GenerateGo(",
		"type Diagnostic struct",
		"const InvalidIntegerLexicalCode",
		"type FailureClass string",
	} {
		if !strings.Contains(output, marker) {
			return fmt.Errorf("envoy public documentation is missing %q", marker)
		}
	}
	return nil
}

func validateEnvoyConsumerOutput(output string) error {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 5 {
		return fmt.Errorf("envoy consumer returned %d output lines; want 5 fixed steps", len(lines))
	}
	if lines[0] != "step=parse result=pass" {
		return fmt.Errorf("envoy consumer first step = %q; want successful parse", lines[0])
	}
	if lines[1] != "step=validate-valid result=pass" {
		return fmt.Errorf("envoy consumer second step = %q; want successful valid validation", lines[1])
	}
	for _, marker := range []string{
		"step=validate-invalid result=pass class=invalid code=XSD2001 loc=invalid.xml:1:",
		"related=schema.xsd:",
		"spec_ref=xsd11-datatypes#integer",
	} {
		if !strings.Contains(lines[2], marker) {
			return fmt.Errorf("envoy consumer invalid-validation evidence is missing %q", marker)
		}
	}
	if !strings.HasPrefix(lines[3], "step=generate-repeat result=pass bytes=") {
		return fmt.Errorf("envoy consumer generation step = %q; want repeated byte-identical generation", lines[3])
	}
	byteCount := strings.TrimPrefix(lines[3], "step=generate-repeat result=pass bytes=")
	parsedCount, err := strconv.Atoi(byteCount)
	if err != nil || parsedCount <= 0 {
		return fmt.Errorf("envoy consumer generated byte count = %q; want positive decimal", byteCount)
	}
	if lines[4] != "step=compile-generated result=pass" {
		return fmt.Errorf("envoy consumer compile step = %q; want complete-source compilation", lines[4])
	}
	return nil
}
