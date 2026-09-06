package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDocumentLimits(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		rule    documentRule
		wantErr string
	}{
		{name: "exact", content: []byte("one two\nthree four\n"), rule: documentRule{maxLines: 2, maxWords: 4}},
		{name: "unterminated line", content: []byte("one two"), rule: documentRule{maxLines: 1, maxWords: 2}},
		{name: "too many lines", content: []byte("one\ntwo\n"), rule: documentRule{maxLines: 1, maxWords: 2}, wantErr: "2 lines"},
		{name: "too many words", content: []byte("one two three\n"), rule: documentRule{maxLines: 1, maxWords: 2}, wantErr: "3 words"},
		{name: "template TODO", content: []byte("[TODO: replace]\n"), rule: documentRule{maxLines: 1, maxWords: 2}, wantErr: "template TODO"},
		{name: "invalid UTF-8", content: []byte{0xff, '\n'}, rule: documentRule{maxLines: 1, maxWords: 1}, wantErr: "valid UTF-8"},
	}
	for _, test := range tests {
		path := filepath.Join(t.TempDir(), "document.md")
		if err := os.WriteFile(path, test.content, 0o600); err != nil {
			t.Fatalf("%s: write document: %v", test.name, err)
		}
		err := checkDocument(path, test.rule)
		if test.wantErr == "" && err != nil {
			t.Fatalf("%s: checkDocument error = %v", test.name, err)
		}
		if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
			t.Fatalf("%s: checkDocument error = %v, want %q", test.name, err, test.wantErr)
		}
	}
}

func TestDocumentStatsRejectSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	link := filepath.Join(root, "link.md")
	if err := os.WriteFile(target, []byte("text\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := readDocumentStats(link); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("readDocumentStats error = %v, want non-regular rejection", err)
	}
}

func TestDocumentRegistryIsValid(t *testing.T) {
	if err := validateDocumentRegistry(); err != nil {
		t.Fatalf("validateDocumentRegistry: %v", err)
	}
	for _, rule := range documentRules {
		found, ok := documentRuleFor(rule.path)
		if !ok || found != rule {
			t.Fatalf("documentRuleFor(%q) = %#v, %t", rule.path, found, ok)
		}
	}
}

func TestDocumentRegistryRejectsDuplicateAndUnsortedPaths(t *testing.T) {
	rule := documentRule{path: "README.md", charter: "entrypoint", maxLines: 1, maxWords: 1}
	for _, rules := range [][]documentRule{
		{rule, rule},
		{{path: "README.md", charter: "entrypoint", maxLines: 1, maxWords: 1},
			{path: "AGENTS.md", charter: "rules", maxLines: 1, maxWords: 1}},
	} {
		if err := validateDocumentRules(rules); err == nil {
			t.Fatalf("validateDocumentRules(%#v) accepted invalid ordering", rules)
		}
	}
}

func TestRepositoryPathsPreservesLeadingWhitespace(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	writeTestDocument(t, root, " leading.md", "text\n")
	application := app{ctx: context.Background()}
	paths, err := application.repositoryPaths(root)
	if err != nil {
		t.Fatalf("repositoryPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != " leading.md" {
		t.Fatalf("repositoryPaths = %#v", paths)
	}
}

func TestMarkdownExtensionIsCaseInsensitive(t *testing.T) {
	if !isDurableMarkdown("GUIDE.MD") {
		t.Fatal("uppercase Markdown extension bypassed the durable document surface")
	}
}

func TestDocumentationAuditStableReport(t *testing.T) {
	root := t.TempDir()
	writeTestDocument(t, root, "README.md", "# Project\n\nCurrent entrypoint.\n")
	writeTestDocument(t, root, "AGENTS.md", "# Rules\n\nCurrent invariant.\n")
	status := "M\x00README.md\x00A\x00evals/agent/curator/accretion.md\x00M\x00AGENTS.md\x00"
	numstat := "3\t1\tREADME.md\x001\t0\tevals/agent/curator/accretion.md\x002\t2\tAGENTS.md\x00"
	application, stdout := testAuditApplication(t, status, numstat)
	if err := application.auditDocs(root, "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	output := stdout.String()
	agents := strings.Index(output, `"AGENTS.md"`)
	readme := strings.Index(output, `"README.md"`)
	if agents < 0 || readme < 0 || agents >= readme {
		t.Fatalf("managed changes are not stably sorted:\n%s", output)
	}
	for _, want := range []string{
		"Head: head-sha", "Base: base-sha", "Merge base: merge-sha",
		`"AGENTS.md" [modified]: +2 -2`, `"README.md" [modified]: +3 -1`,
		`"evals/agent/curator/accretion.md"`, "Current-state review triggers:", "Curator review: required",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("audit output missing %q:\n%s", want, output)
		}
	}
}

func TestDocumentationAuditJSONSeparatesDiffAndFixtures(t *testing.T) {
	root := t.TempDir()
	writeTestDocument(t, root, "README.md", "# Project\n\nCurrent entrypoint.\n")
	writeTestDocument(t, root, "AGENTS.md", "# Rules\n\nCurrent invariant.\n")
	var output bytes.Buffer
	application, _ := testAuditApplication(t,
		"M\x00README.md\x00A\x00evals/agent/curator/accretion.md\x00M\x00AGENTS.md\x00",
		"3\t1\tREADME.md\x001\t0\tevals/agent/curator/accretion.md\x002\t2\tAGENTS.md\x00")
	application.stdout = &output
	if err := application.auditDocsWithFormat(root, "base-ref", "json"); err != nil {
		t.Fatalf("auditDocsWithFormat: %v", err)
	}
	var report documentationAuditReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode JSON audit: %v\n%s", err, output.String())
	}
	if report.Schema != documentationAuditSchema || report.Base != "base-sha" || report.Head != "head-sha" ||
		report.MergeBase != "merge-sha" || len(report.ManagedChanges) != 2 || len(report.EvaluationFixtures) != 1 ||
		report.CurrentStateReviewTriggers == nil {
		t.Fatalf("audit report = %#v", report)
	}
	if report.ManagedChanges[0].Path != "AGENTS.md" || report.ManagedChanges[1].Path != "README.md" ||
		report.EvaluationFixtures[0] != "evals/agent/curator/accretion.md" {
		t.Fatalf("audit ordering = %#v / %#v", report.ManagedChanges, report.EvaluationFixtures)
	}
}

func TestDocumentationAuditFixtureDoesNotRequireCurator(t *testing.T) {
	application, stdout := testAuditApplication(t,
		"A\x00evals/agent/curator/replacement.md\x00", "1\t0\tevals/agent/curator/replacement.md\x00")
	if err := application.auditDocs(t.TempDir(), "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	if !strings.Contains(stdout.String(), "Curator review: not required") {
		t.Fatalf("fixture-only audit required Curator:\n%s", stdout.String())
	}
}

func TestDocumentationAuditUnchanged(t *testing.T) {
	application, stdout := testAuditApplication(t, "", "")
	if err := application.auditDocs(t.TempDir(), "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	output := stdout.String()
	if strings.Count(output, "- None") != 3 || !strings.Contains(output, "Curator review: not required") {
		t.Fatalf("unchanged audit output:\n%s", output)
	}
}

func TestCurrentStateReviewTriggerPathClassification(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "schema.go", want: true},
		{path: "internal/schema.go", want: true},
		{path: "cmd/goxsd9/main.go", want: true},
		{path: "internal/workflowctl/audit.go", want: false},
		{path: "cmd/workflowctl/main.go", want: false},
		{path: "schema_test.go", want: false},
		{path: "testdata/schema.go", want: false},
		{path: "pkg/testdata/schema.go", want: false},
		{path: "evals/agent/develop/schema.go", want: false},
		{path: "pkg/evals/schema.go", want: false},
		{path: "schema.txt", want: false},
		{path: "internal/workflowctlx/audit.go", want: true},
		{path: "cmd/workflowctlx/main.go", want: true},
	}
	for _, test := range tests {
		if got := isCurrentStateReviewTriggerPath(test.path); got != test.want {
			t.Errorf("isCurrentStateReviewTriggerPath(%q) = %t, want %t", test.path, got, test.want)
		}
	}
}

func TestDocumentationAuditCurrentStateTriggersAreSeparateAndSorted(t *testing.T) {
	application, stdout := testAuditApplication(t,
		"T\x00pkg/z.go\x00M\x00internal/workflowctl/audit.go\x00A\x00pkg/a.go\x00D\x00pkg/a_test.go\x00M\x00evals/agent/develop/case.go\x00M\x00testdata/fixture.go\x00",
		"")
	if err := application.auditDocs(t.TempDir(), "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	output := stdout.String()
	section := strings.Index(output, "Current-state review triggers:")
	if section < 0 {
		t.Fatalf("missing trigger section:\n%s", output)
	}
	triggerOutput := output[section:]
	if !strings.Contains(triggerOutput, `- "pkg/a.go"`) || !strings.Contains(triggerOutput, `- "pkg/z.go"`) {
		t.Fatalf("trigger paths missing:\n%s", output)
	}
	if strings.Index(triggerOutput, `- "pkg/a.go"`) >= strings.Index(triggerOutput, `- "pkg/z.go"`) {
		t.Fatalf("trigger paths are not sorted:\n%s", output)
	}
	if strings.Contains(triggerOutput, "workflowctl") || strings.Contains(triggerOutput, "testdata") || strings.Contains(triggerOutput, "evals") || strings.Contains(triggerOutput, "_test.go") {
		t.Fatalf("excluded paths appeared as triggers:\n%s", output)
	}
	if !strings.Contains(output, "Curator review: required") {
		t.Fatalf("source-only triggers did not require Curator:\n%s", output)
	}
}

func TestDocumentationAuditWorkflowAndTestOnlyPathsRemainNotRequired(t *testing.T) {
	application, stdout := testAuditApplication(t,
		"M\x00internal/workflowctl/audit.go\x00M\x00cmd/workflowctl/main.go\x00M\x00schema_test.go\x00M\x00testdata/fixture.go\x00M\x00evals/agent/develop/case.go\x00",
		"")
	if err := application.auditDocs(t.TempDir(), "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	output := stdout.String()
	section := strings.Index(output, "Current-state review triggers:")
	if section < 0 || !strings.Contains(output[section:], "- None") {
		t.Fatalf("workflow/test-only paths produced triggers:\n%s", output)
	}
	if !strings.Contains(output, "Curator review: not required") {
		t.Fatalf("workflow/test-only paths required Curator:\n%s", output)
	}
}

func TestDocumentationAuditCurrentStateTriggersJSONIsDeterministic(t *testing.T) {
	application, _ := testAuditApplication(t,
		"M\x00pkg/z.go\x00A\x00pkg/a.go\x00", "")
	var first bytes.Buffer
	application.stdout = &first
	if err := application.auditDocsWithFormat(t.TempDir(), "base-ref", "json"); err != nil {
		t.Fatalf("first JSON audit: %v", err)
	}
	var report documentationAuditReport
	if err := json.Unmarshal(first.Bytes(), &report); err != nil {
		t.Fatalf("decode JSON audit: %v", err)
	}
	if report.CurrentStateReviewTriggers == nil || len(report.CurrentStateReviewTriggers) != 2 ||
		report.CurrentStateReviewTriggers[0] != "pkg/a.go" || report.CurrentStateReviewTriggers[1] != "pkg/z.go" {
		t.Fatalf("trigger report = %#v", report.CurrentStateReviewTriggers)
	}
	var second bytes.Buffer
	application.stdout = &second
	if err := application.auditDocsWithFormat(t.TempDir(), "base-ref", "json"); err != nil {
		t.Fatalf("second JSON audit: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("JSON audit is not deterministic:\n%s\n---\n%s", first.Bytes(), second.Bytes())
	}
}

func TestDocumentationAuditReportsRegisteredAddition(t *testing.T) {
	root := t.TempDir()
	writeTestDocument(t, root, "README.md", "# New entrypoint\n")
	application, stdout := testAuditApplication(t, "A\x00README.md\x00", "1\t0\tREADME.md\x00")
	if err := application.auditDocs(root, "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	if !strings.Contains(stdout.String(), `"README.md" [added]: +1 -0`) {
		t.Fatalf("registered addition missing:\n%s", stdout.String())
	}
}

func TestDocumentationAuditRejectsUnregisteredAndBinaryMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		numstat string
		wantErr string
	}{
		{
			name: "unregistered", status: "A\x00docs/new guide.md\x00",
			numstat: "1\t0\tdocs/new guide.md\x00", wantErr: "not registered",
		},
		{
			name: "binary", status: "M\x00AGENTS.md\x00",
			numstat: "-\t-\tAGENTS.md\x00", wantErr: "is binary",
		},
	}
	for _, test := range tests {
		application, _ := testAuditApplication(t, test.status, test.numstat)
		err := application.auditDocs(t.TempDir(), "base-ref")
		if err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Fatalf("%s: auditDocs error = %v, want %q", test.name, err, test.wantErr)
		}
	}
}

func TestDocumentationAuditReportsUnregisteredDeletion(t *testing.T) {
	application, stdout := testAuditApplication(t,
		"D\x00docs/retired.md\x00", "0\t12\tdocs/retired.md\x00")
	if err := application.auditDocs(t.TempDir(), "base-ref"); err != nil {
		t.Fatalf("auditDocs: %v", err)
	}
	if !strings.Contains(stdout.String(), `"docs/retired.md" [deleted, unregistered]: +0 -12`) {
		t.Fatalf("deleted document was hidden:\n%s", stdout.String())
	}
}

func TestDocumentationAuditRejectsInvalidBaseAndDirtyTree(t *testing.T) {
	tests := []struct {
		name    string
		command commandExecutor
		wantErr string
	}{
		{
			name: "dirty", wantErr: "clean worktree",
			command: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
				return " M README.md", nil
			},
		},
		{
			name: "invalid base", wantErr: "resolve documentation base",
			command: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				if command == "git status --porcelain" {
					return "", nil
				}
				if command == "git rev-parse --verify --end-of-options missing^{commit}" {
					return "", errors.New("unknown revision")
				}
				return "", fmt.Errorf("unexpected command: %s", command)
			},
		},
		{
			name: "unrelated base", wantErr: "unrelated to HEAD",
			command: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				switch command {
				case "git status --porcelain":
					return "", nil
				case "git rev-parse --verify --end-of-options missing^{commit}":
					return "base-sha", nil
				case "git rev-parse --verify --end-of-options HEAD^{commit}":
					return "head-sha", nil
				case "git merge-base base-sha head-sha":
					return "", errors.New("no common ancestor")
				default:
					return "", fmt.Errorf("unexpected command: %s", command)
				}
			},
		},
	}
	for _, test := range tests {
		application := app{ctx: context.Background(), executeCommand: test.command, stdout: &bytes.Buffer{}}
		err := application.auditDocs(t.TempDir(), "missing")
		if err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Fatalf("%s: auditDocs error = %v, want %q", test.name, err, test.wantErr)
		}
	}
}

func TestResolveCommitEndsOptions(t *testing.T) {
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if command != "git rev-parse --verify --end-of-options -surprising^{commit}" {
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		return "commit-sha", nil
	}}
	commit, err := application.resolveCommit("/repo", "-surprising")
	if err != nil || commit != "commit-sha" {
		t.Fatalf("resolveCommit = %q, %v", commit, err)
	}
}

func TestGitDocumentationRecordParsing(t *testing.T) {
	oddPath := "docs/decisions/a plan\tline\n.md"
	invalidPath := "docs/decisions/" + string([]byte{0xff}) + ".md"
	statuses, err := parseGitDocumentStatuses(
		"M\x00" + oddPath + "\x00A\x00-leading.md\x00M\x00" + invalidPath + "\x00")
	if err != nil {
		t.Fatalf("parseGitDocumentStatuses: %v", err)
	}
	if len(statuses) != 3 || statuses[0].path != "-leading.md" {
		t.Fatalf("statuses = %#v", statuses)
	}
	stats, err := parseGitLineStats("2\t1\t" + oddPath + "\x00")
	if err != nil {
		t.Fatalf("parseGitLineStats: %v", err)
	}
	if len(stats) != 1 || stats[0].path != oddPath || stats[0].additions != 2 || stats[0].deletions != 1 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestGitDocumentationRecordParsingRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		parse func() error
	}{
		{name: "missing NUL", parse: func() error { _, err := parseNULFields("M"); return err }},
		{name: "truncated status", parse: func() error { _, err := parseGitDocumentStatuses("M\x00"); return err }},
		{name: "unknown status", parse: func() error { _, err := parseGitDocumentStatuses("U\x00README.md\x00"); return err }},
		{name: "malformed numstat", parse: func() error { _, err := parseGitLineStats("1\tREADME.md\x00"); return err }},
		{name: "mixed binary", parse: func() error { _, err := parseGitLineStats("-\t1\tREADME.md\x00"); return err }},
		{name: "unsafe path", parse: func() error { _, err := parseGitLineStats("1\t0\t../README.md\x00"); return err }},
	}
	for _, test := range tests {
		if err := test.parse(); err == nil {
			t.Fatalf("%s: malformed input accepted", test.name)
		}
	}
}

func TestDocumentationAuditUsesCommittedHead(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	runGitTest(t, root, "config", "user.name", "Workflow Test")
	runGitTest(t, root, "config", "user.email", "workflow@example.test")
	writeTestDocument(t, root, "README.md", "base\n")
	runGitTest(t, root, "add", "README.md")
	runGitTest(t, root, "commit", "--no-gpg-sign", "-m", "base")
	base := runGitTest(t, root, "rev-parse", "HEAD")
	writeTestDocument(t, root, "README.md", "base\ncurrent\n")
	runGitTest(t, root, "add", "README.md")
	runGitTest(t, root, "commit", "--no-gpg-sign", "-m", "current")

	var stdout bytes.Buffer
	application := app{ctx: context.Background(), stdout: &stdout, stderr: &bytes.Buffer{}}
	if err := application.auditDocs(root, base); err != nil {
		t.Fatalf("audit committed head: %v", err)
	}
	if !strings.Contains(stdout.String(), `"README.md" [modified]: +1 -0`) {
		t.Fatalf("committed change missing:\n%s", stdout.String())
	}
	writeTestDocument(t, root, "README.md", "dirty\n")
	if err := application.auditDocs(root, base); err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("dirty audit error = %v", err)
	}
}

func TestDocumentationHistorySumsChurn(t *testing.T) {
	since := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git rev-list --first-parent --since=2026-08-08T12:00:00Z HEAD":
			return "commit-two\ncommit-one", nil
		case "git diff-tree --root --first-parent --no-commit-id --numstat -r -z --no-renames commit-two --":
			return "2\t3\tREADME.md\x001\t0\tevals/agent/review/case.md\x00", nil
		case "git diff-tree --root --first-parent --no-commit-id --numstat -r -z --no-renames commit-one --":
			return "4\t1\tREADME.md\x001\t0\tAGENTS.md\x00", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	if err := application.writeDocumentationHistory("/repo", since); err != nil {
		t.Fatalf("writeDocumentationHistory: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"AGENTS.md": +1 -0`) || !strings.Contains(output, `"README.md": +6 -4`) {
		t.Fatalf("history did not sum churn:\n%s", output)
	}
	if strings.Contains(output, "evals/agent") {
		t.Fatalf("agent fixture counted as documentation churn:\n%s", output)
	}
}

func testAuditApplication(t *testing.T, status, numstat string) (app, *bytes.Buffer) {
	t.Helper()
	var stdout bytes.Buffer
	application := app{ctx: context.Background(), stdout: &stdout, executeCommand: func(_ string, _ io.Reader,
		name string, args ...string,
	) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git status --porcelain":
			return "", nil
		case "git rev-parse --verify --end-of-options base-ref^{commit}":
			return "base-sha", nil
		case "git rev-parse --verify --end-of-options HEAD^{commit}":
			return "head-sha", nil
		case "git merge-base base-sha head-sha":
			return "merge-sha", nil
		case "git diff --name-status -z --no-renames merge-sha head-sha --":
			return status, nil
		case "git diff --numstat -z --no-renames merge-sha head-sha --":
			return numstat, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	return application, &stdout
}

func writeTestDocument(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create document directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}
}

func runGitTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	// #nosec G204 -- each test supplies repository-local Git arguments without user input.
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
