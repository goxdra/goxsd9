package workflowctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentHeadingLinksResolveRelativeAndNormalizedAnchors(t *testing.T) {
	root := t.TempDir()
	rules := []documentRule{{path: "README.md"}, {path: "docs/decisions/decision.md"}}
	writeTestDocument(t, root, "README.md", "# Home\n## Résumé: `Go` & XML!\n## Cafe\u0301 `Code`\n## More, Markup! ###\n## Repeat\n## Repeat\n## Repeat-1\n## Repeat\n")
	writeTestDocument(t, root, "docs/decisions/decision.md", strings.Join([]string{
		"[home](../../README.md#home)",
		"[clean path](../.././README.md#home)",
		"[normalized](../../README.md#résumé-go--xml)",
		"[encoded](../../README.md#r%C3%A9sum%C3%A9-go--xml)",
		"[decomposed and code](../../README.md#cafe%CC%81-code)",
		"[closing marks](../../README.md#more-markup)",
		"[duplicate](../../README.md#repeat-1)",
		"[collision](../../README.md#repeat-2)",
		"[same](#decision)",
		"# Decision",
	}, "\n")+"\n")
	if err := checkDocumentHeadingLinks(root, rules); err != nil {
		t.Fatalf("checkDocumentHeadingLinks: %v", err)
	}
}

func TestDocumentHeadingLinksReportStaleRenameAndPassRepair(t *testing.T) {
	root := t.TempDir()
	rules := []documentRule{{path: "ARCHITECTURE.md"}, {path: "docs/decisions/0006.md"}}
	writeTestDocument(t, root, "ARCHITECTURE.md", "# Architecture\n## Input and resolution\n")
	writeTestDocument(t, root, "docs/decisions/0006.md", "# Decision\n\nSee [resolution](../../ARCHITECTURE.md#input-and-resolution).\n")
	if err := checkDocumentHeadingLinks(root, rules); err != nil {
		t.Fatalf("original heading: %v", err)
	}
	writeTestDocument(t, root, "ARCHITECTURE.md", "# Architecture\n## Resolution\n")
	err := checkDocumentHeadingLinks(root, rules)
	for _, want := range []string{"DOCS001", "docs/decisions/0006.md:3", "#input-and-resolution", "ARCHITECTURE.md"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("renamed heading error = %v, want %q", err, want)
		}
	}
	writeTestDocument(t, root, "docs/decisions/0006.md", "# Decision\n\nSee [resolution](../../ARCHITECTURE.md#resolution).\n")
	if err := checkDocumentHeadingLinks(root, rules); err != nil {
		t.Fatalf("repaired link: %v", err)
	}
}

func TestDocumentHeadingLinksIgnoreCodeAndUnsupportedShapes(t *testing.T) {
	root := t.TempDir()
	rules := []documentRule{{path: "README.md"}, {path: "docs/target.md"}}
	writeTestDocument(t, root, "docs/target.md", strings.Join([]string{
		"# Target",
		"Setext heading",
		"==============",
		"<a id=\"html-id\"></a>",
		"```md",
		"## Fake backtick",
		"```",
		"~~~md",
		"## Fake tilde",
		"~~~",
		"<!--",
		"## Fake comment",
		"-->",
		"    ## Fake indent",
	}, "\n")+"\n")
	writeTestDocument(t, root, "README.md", strings.Join([]string{
		"# Source",
		"`[code](docs/target.md#missing)` [real](docs/target.md#target)",
		"```md",
		"[fenced](docs/target.md#missing)",
		"```",
		"~~~md",
		"[fenced](docs/target.md#missing)",
		"~~~",
		"<!-- [comment](docs/target.md#missing) -->",
		"    [indented](docs/target.md#missing)",
		"![image](docs/target.md#missing)",
		"[reference][id]",
		"[id]: docs/target.md#missing",
		"[title](docs/target.md#missing \"title\")",
		"[angle](<docs/target.md#missing>)",
		"[external](https://example.test/docs/target.md#missing)",
		"[other](docs/other.md#missing)",
	}, "\n")+"\n")
	if err := checkDocumentHeadingLinks(root, rules); err != nil {
		t.Fatalf("excluded syntax: %v", err)
	}
	for _, anchor := range []string{"fake-backtick", "fake-tilde", "fake-comment", "fake-indent", "setext-heading", "html-id"} {
		writeTestDocument(t, root, "README.md", "# Source\n[probe](docs/target.md#"+anchor+")\n")
		err := checkDocumentHeadingLinks(root, rules)
		if err == nil || !strings.Contains(err.Error(), "DOCS001 README.md:2") {
			t.Fatalf("excluded heading %q error = %v", anchor, err)
		}
	}
}

func TestDocumentHeadingLinksErrorOrderAndCause(t *testing.T) {
	root := t.TempDir()
	rules := []documentRule{{path: "README.md"}, {path: "docs/target.md"}}
	writeTestDocument(t, root, "README.md", "# Source\n[second](docs/target.md#second) [first](docs/target.md#first)\n")
	writeTestDocument(t, root, "docs/target.md", "# Target\n")
	for attempt := 0; attempt < 3; attempt++ {
		err := checkDocumentHeadingLinks(root, rules)
		if err == nil || !strings.Contains(err.Error(), `DOCS001 README.md:2: heading fragment "#second"`) {
			t.Fatalf("attempt %d: first error = %v", attempt, err)
		}
	}
	writeTestDocument(t, root, "README.md", "# Source\n[malformed](docs/target.md#bad%escape)\n")
	err := checkDocumentHeadingLinks(root, rules)
	if err == nil || !strings.Contains(err.Error(), "DOCS002 README.md:2") || !strings.Contains(err.Error(), "#bad%escape") {
		t.Fatalf("malformed fragment error = %v", err)
	}
	if removeErr := os.Remove(filepath.Join(root, "docs", "target.md")); removeErr != nil {
		t.Fatalf("remove target: %v", removeErr)
	}
	err = checkDocumentHeadingLinks(root, rules)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read error = %v, want preserved missing-file cause", err)
	}
}

func TestDocumentHeadingLinksCommentMarkerInsideCodeDoesNotHideLink(t *testing.T) {
	root := t.TempDir()
	rules := []documentRule{{path: "README.md"}, {path: "docs/target.md"}}
	writeTestDocument(t, root, "README.md", "# Source\n`<!--` [live](docs/target.md#missing)\n")
	writeTestDocument(t, root, "docs/target.md", "# Target\n")
	err := checkDocumentHeadingLinks(root, rules)
	if err == nil || !strings.Contains(err.Error(), `DOCS001 README.md:2: heading fragment "#missing"`) {
		t.Fatalf("link after code marker error = %v", err)
	}
}

func TestDocumentHeadingLinksCommentClosureBeforeIndentedCode(t *testing.T) {
	tests := []struct {
		name     string
		block    string
		linkLine int
	}{
		{name: "space-indented closer", block: "<!-- comment\n    -->\n", linkLine: 5},
		{name: "tab-indented closer", block: "<!-- comment\n\t-->\n", linkLine: 5},
		{name: "unindented closer", block: "<!-- comment\n-->\n", linkLine: 5},
		{name: "indented continuation then closer", block: "<!-- comment\n    still comment\n-->\n", linkLine: 6},
		{name: "fence inside comment", block: "<!-- comment\n    ```\n    -->\n", linkLine: 6},
		{name: "comment markers inside fence", block: "```md\n<!-- comment\n    -->\n```\n", linkLine: 7},
		{name: "space-indented code opener", block: "    <!-- comment\n", linkLine: 4},
		{name: "tab-indented code opener", block: "\t<!-- comment\n", linkLine: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			rules := []documentRule{{path: "README.md"}}
			content := "# Source\n" + test.block + "\n[stale](#missing)\n## Present\n"
			writeTestDocument(t, root, "README.md", content)
			err := checkDocumentHeadingLinks(root, rules)
			want := fmt.Sprintf(`DOCS001 README.md:%d: heading fragment "#missing"`, test.linkLine)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("stale link error = %v, want %q", err, want)
			}
			writeTestDocument(t, root, "README.md", strings.Replace(content, "#missing", "#present", 1))
			if err := checkDocumentHeadingLinks(root, rules); err != nil {
				t.Fatalf("repaired link error = %v", err)
			}
		})
	}
}

func TestDocumentHeadingLinksIndentedCommentCloserRemainsCode(t *testing.T) {
	for _, closer := range []string{"    -->", "\t-->"} {
		t.Run(fmt.Sprintf("%q", closer), func(t *testing.T) {
			root := t.TempDir()
			content := "# Source\n<!-- comment\n" + closer + " [hidden](#missing)\n[real](#present)\n## Present\n"
			writeTestDocument(t, root, "README.md", content)
			if err := checkDocumentHeadingLinks(root, []documentRule{{path: "README.md"}}); err != nil {
				t.Fatalf("indented closer line supplied a link: %v", err)
			}
		})
	}
}

func TestDocumentHeadingLinksInlineCodeBoundaries(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "literal backslash before closer", line: "`C:\\` [stale](#missing) `tail`"},
		{name: "literal comment marker and backslash", line: "`<!-- C:\\` [stale](#missing) `tail`"},
		{name: "equal single runs hide code link", line: "`[hidden](#hidden)` [stale](#missing)"},
		{name: "equal double runs skip shorter run", line: "``[hidden](#hidden)` more`` [stale](#missing)"},
		{name: "equal double runs skip longer run", line: "``[hidden](#hidden)``` more`` [stale](#missing)"},
		{name: "escaped opener odd slash", line: "\\`[stale](#missing)`"},
		{name: "opener even slashes", line: "\\\\`[hidden](#hidden)` [stale](#missing)"},
		{name: "unmatched opener remains text", line: "`[stale](#missing)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDocumentHeadingLinkRepair(t, "# Source\n"+test.line+"\n## Present\n", 2)
		})
	}
}

func TestDocumentHeadingLinksFenceBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		block string
	}{
		{name: "backtick info", block: "```md\n[hidden](#hidden)\n## Fake\n```\n"},
		{name: "tilde info with backticks", block: "~~~ md```literal\n[hidden](#hidden)\n## Fake\n~~~\n"},
		{name: "longer closer", block: "```md\n[hidden](#hidden)\n## Fake\n````\n"},
		{name: "short closer stays content", block: "````md\n[hidden](#hidden)\n## Fake\n```\n[hidden2](#hidden2)\n````\n"},
		{name: "wrong-character closer stays content", block: "```md\n[hidden](#hidden)\n## Fake\n~~~\n[hidden2](#hidden2)\n```\n"},
		{name: "trailing-text closer stays content", block: "```md\n[hidden](#hidden)\n## Fake\n``` trailing\n[hidden2](#hidden2)\n```\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := "# Source\n" + test.block + "[stale](#missing)\n## Present\n"
			linkLine := 2 + strings.Count(test.block, "\n")
			assertDocumentHeadingLinkRepair(t, content, linkLine)
			root := t.TempDir()
			writeTestDocument(t, root, "README.md", strings.Replace(content, "[stale](#missing)", "[probe](#fake)", 1))
			err := checkDocumentHeadingLinks(root, []documentRule{{path: "README.md"}})
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf(`DOCS001 README.md:%d: heading fragment "#fake"`, linkLine)) {
				t.Fatalf("fenced heading error = %v", err)
			}
		})
	}
}

func TestDocumentHeadingLinksRejectInvalidBacktickFenceInfo(t *testing.T) {
	for _, opener := range []string{"```literal```", "```<!--```"} {
		t.Run(opener, func(t *testing.T) {
			assertDocumentHeadingLinkRepair(t, "# Source\n"+opener+"\n[stale](#missing)\n## Present\n", 3)
		})
	}
}

func TestDocumentHeadingLinksUnclosedValidFence(t *testing.T) {
	for _, opener := range []string{"```md", "~~~ md```literal"} {
		t.Run(opener, func(t *testing.T) {
			root := t.TempDir()
			rules := []documentRule{{path: "README.md"}, {path: "docs/source.md"}}
			writeTestDocument(t, root, "README.md", "# Source\n"+opener+"\n[hidden](#missing)\n## Fake\n")
			writeTestDocument(t, root, "docs/source.md", "[probe](../README.md#fake)\n")
			err := checkDocumentHeadingLinks(root, rules)
			if err == nil || !strings.Contains(err.Error(), `DOCS001 docs/source.md:1: heading fragment "#fake"`) {
				t.Fatalf("unclosed fence heading error = %v", err)
			}
		})
	}
}

func assertDocumentHeadingLinkRepair(t *testing.T, content string, linkLine int) {
	t.Helper()
	root := t.TempDir()
	rules := []documentRule{{path: "README.md"}}
	writeTestDocument(t, root, "README.md", content)
	err := checkDocumentHeadingLinks(root, rules)
	want := fmt.Sprintf(`DOCS001 README.md:%d: heading fragment "#missing"`, linkLine)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("stale link error = %v, want %q", err, want)
	}
	if strings.Count(content, "[stale](#missing)") != 1 {
		t.Fatalf("fixture has %d stale links, want 1", strings.Count(content, "[stale](#missing)"))
	}
	writeTestDocument(t, root, "README.md", strings.Replace(content, "[stale](#missing)", "[stale](#present)", 1))
	if err := checkDocumentHeadingLinks(root, rules); err != nil {
		t.Fatalf("repaired link error = %v", err)
	}
}

func TestCheckDocsChecksCanonicalAndRejectsStaleDecisionLink(t *testing.T) {
	root := newManagedDocsCheckFixture(t)
	application := app{ctx: context.Background()}
	if err := application.checkDocs(root, false); err != nil {
		t.Fatalf("canonical docs check: %v", err)
	}
	architecture := filepath.Join(root, "ARCHITECTURE.md")
	// #nosec G304 -- architecture is a fixed path below this test's temporary directory.
	contents, err := os.ReadFile(architecture)
	if err != nil {
		t.Fatalf("read architecture: %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "## Resolution\n", "## Renamed resolution\n", 1))
	// #nosec G703 -- architecture is a fixed path below this test's temporary directory.
	if writeErr := os.WriteFile(architecture, contents, 0o600); writeErr != nil {
		t.Fatalf("rename heading: %v", writeErr)
	}
	err = application.checkDocs(root, false)
	for _, want := range []string{"DOCS001", "docs/decisions/0006-vertical-slice-cli.md:319", "#resolution", "ARCHITECTURE.md"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("docs check after rename = %v, want %q", err, want)
		}
	}
}

func TestCheckDocsReportsInlineBoundaryFailures(t *testing.T) {
	tests := []struct {
		name     string
		addition string
		linkLine int
	}{
		{name: "backtick fence info", addition: "\n```literal```\n[stale](#missing)\n## Present\n", linkLine: 34},
		{name: "literal slash in code span", addition: "\n`C:\\` [stale](#missing) `tail`\n## Present\n", linkLine: 33},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newManagedDocsCheckFixture(t)
			path := filepath.Join(root, "docs", "decisions", "0001-foundations.md")
			// #nosec G304 -- path is a fixed managed document below the temporary fixture root.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			writeTestDocument(t, root, "docs/decisions/0001-foundations.md", string(data)+test.addition)
			application := app{ctx: context.Background()}
			err = application.checkDocs(root, false)
			want := fmt.Sprintf(`DOCS001 docs/decisions/0001-foundations.md:%d: heading fragment "#missing"`, test.linkLine)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("docs check error = %v, want %q", err, want)
			}
			writeTestDocument(t, root, "docs/decisions/0001-foundations.md", strings.Replace(string(data)+test.addition, "[stale](#missing)", "[stale](#present)", 1))
			if err := application.checkDocs(root, false); err != nil {
				t.Fatalf("repaired docs check: %v", err)
			}
		})
	}
}

func newManagedDocsCheckFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	sourceRoot := filepath.Join("..", "..")
	for _, rule := range documentRules {
		// #nosec G304 -- rules are fixed repository-managed document paths.
		contents, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(rule.path)))
		if err != nil {
			t.Fatalf("read fixture %s: %v", rule.path, err)
		}
		writeTestDocument(t, root, rule.path, string(contents))
	}
	for _, name := range []string{"backlog", "develop", "retro"} {
		source := filepath.Join(sourceRoot, ".agents", "skills", name, "agents", "openai.yaml")
		// #nosec G304 -- names are fixed skill fixture paths.
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read skill metadata: %v", err)
		}
		writeTestDocument(t, root, filepath.Join(".agents", "skills", name, "agents", "openai.yaml"), string(contents))
	}
	return root
}
