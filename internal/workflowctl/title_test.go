package workflowctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCommitTitleConvention(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr bool
	}{
		{name: "type", title: "feat: add schema parser"},
		{name: "scope", title: "fix(parser): reject malformed schema"},
		{name: "scoped breaking", title: "feat(schema)!: remove mutable query API"},
		{name: "unscoped breaking", title: "feat!: remove legacy parser API"},
		{name: "digit summary", title: "docs(specs): 1.1 conformance policy"},
		{name: "claim", title: "chore(workflow): claim issue #17"},
		{name: "later uppercase", title: "feat(parser): add XSD reader in Go"},
		{name: "exact length", title: "feat: " + strings.Repeat("a", commitTitleLimit-len("feat: "))},
		{name: "exact unicode length", title: "feat: " + strings.Repeat("é", commitTitleLimit-len("feat: "))},
		{name: "unknown type", title: "build: update module", wantErr: true},
		{name: "unknown scope", title: "feat(api): add query", wantErr: true},
		{name: "slash scope", title: "feat(parser/xml): add parser", wantErr: true},
		{name: "uppercase scope", title: "feat(Parser): add parser", wantErr: true},
		{name: "uppercase type", title: "Feat: add parser", wantErr: true},
		{name: "uppercase summary", title: "feat(parser): Add parser", wantErr: true},
		{name: "period", title: "fix(parser): reject input.", wantErr: true},
		{name: "missing separator", title: "feat(parser):add parser", wantErr: true},
		{name: "extra separator space", title: "feat(parser):  add parser", wantErr: true},
		{name: "space before colon", title: "feat(parser) : add parser", wantErr: true},
		{name: "empty summary", title: "feat(parser): ", wantErr: true},
		{name: "empty scope", title: "feat(): add parser", wantErr: true},
		{name: "unclosed scope", title: "feat(parser: add parser", wantErr: true},
		{name: "misplaced breaking", title: "feat!(parser): remove API", wantErr: true},
		{name: "double breaking", title: "feat(parser)!!: remove API", wantErr: true},
		{name: "leading whitespace", title: " feat: add parser", wantErr: true},
		{name: "newline", title: "feat: add parser\nnow", wantErr: true},
		{name: "tab", title: "feat: add\tparser", wantErr: true},
		{name: "long", title: "feat: " + strings.Repeat("a", commitTitleLimit-len("feat: ")+1), wantErr: true},
		{name: "long unicode", title: "feat: " + strings.Repeat("é", commitTitleLimit-len("feat: ")+1), wantErr: true},
	}
	for _, test := range tests {
		err := validateCommitTitle(test.title)
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: validateCommitTitle(%q) error = %v, want error %t", test.name, test.title, err, test.wantErr)
		}
	}
	for _, typeName := range commitTypes {
		if err := validateCommitTitle(typeName + ": add coverage"); err != nil {
			t.Fatalf("commit type %q rejected: %v", typeName, err)
		}
	}
	for _, scope := range validAreas {
		if err := validateCommitTitle("feat(" + scope + "): add coverage"); err != nil {
			t.Fatalf("commit scope %q rejected: %v", scope, err)
		}
	}
}

func TestPullRequestOpenRejectsInvalidTitleBeforeMutation(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	err := application.openPullRequest([]string{
		"17", "--title", "Invalid title", "--body-file", "missing",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid title") {
		t.Fatalf("openPullRequest error = %v, want invalid title", err)
	}
}

func TestPullRequestOpenRejectsMultilineCommitBeforePush(t *testing.T) {
	mutated := false
	application := app{ctx: context.Background(), executeCommand: func(_ string, _ io.Reader,
		name string, args ...string,
	) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git rev-parse --show-toplevel":
			return "/repo", nil
		case "git branch --show-current":
			return "agent/issue-17", nil
		case "git status --porcelain":
			return "", nil
		case "git log --format=%x00%B%x00 origin/main..HEAD":
			return framedRawCommitLog("fix(parser): reject invalid XML\ncontinue\n"), nil
		default:
			mutated = name == "gh" || command == "git push origin HEAD:refs/heads/agent/issue-17"
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	err := application.createPullRequest(17, "fix(parser): reject invalid XML", "unused")
	if err == nil || !strings.Contains(err.Error(), "blank line") {
		t.Fatalf("createPullRequest error = %v, want multiline title rejection", err)
	}
	if mutated {
		t.Fatal("multiline commit title reached a push or GitHub mutation")
	}
}

func TestWorkCommitTitlesAreValidatedBeforePush(t *testing.T) {
	tests := []struct {
		name    string
		log     string
		wantErr bool
	}{
		{name: "valid", log: framedCommitLog(
			"fix(parser): reject invalid XML", "chore(workflow): claim issue #17")},
		{name: "valid body", log: framedRawCommitLog(
			"fix(parser): reject invalid XML\n\nExplain why.\n",
			"chore(workflow): claim issue #17\n\nAgent-Run-ID: run-test\n")},
		{name: "invalid", log: framedCommitLog(
			"temporary checkpoint", "chore(workflow): claim issue #17"), wantErr: true},
		{name: "leading whitespace in newest", log: framedCommitLog(
			" fix(parser): reject invalid XML", "chore(workflow): claim issue #17"), wantErr: true},
		{name: "trailing whitespace in oldest", log: framedCommitLog(
			"fix(parser): reject invalid XML", "chore(workflow): claim issue #17 "), wantErr: true},
		{name: "multiline subject", log: framedRawCommitLog(
			"fix(parser): reject invalid XML\ncontinue\n", "chore(workflow): claim issue #17\n"), wantErr: true},
		{name: "malformed record", log: "fix(parser): reject invalid XML", wantErr: true},
		{name: "empty", wantErr: true},
	}
	for _, test := range tests {
		application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
			got := name + " " + strings.Join(args, " ")
			if got != "git log --format=%x00%B%x00 origin/main..test-head" {
				return "", fmt.Errorf("unexpected command: %s", got)
			}
			return test.log, nil
		}}
		err := application.validateWorkCommitTitles("/repo", "test-head")
		if (err != nil) != test.wantErr {
			t.Fatalf("%s: validateWorkCommitTitles error = %v, want error %t", test.name, err, test.wantErr)
		}
	}
}

func framedCommitLog(titles ...string) string {
	messages := make([]string, 0, len(titles))
	for _, title := range titles {
		messages = append(messages, title+"\n")
	}
	return framedRawCommitLog(messages...)
}

func framedRawCommitLog(messages ...string) string {
	return commitMessageNUL + strings.Join(messages, commitMessageNUL+"\n"+commitMessageNUL) + commitMessageNUL
}

func TestClaimMessageKeepsConventionalSubjectAndTrailers(t *testing.T) {
	lease := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	want := "chore(workflow): claim issue #17\n\n" +
		"Agent-Persona: Smith\n" +
		"Agent-Run-ID: run-test\n" +
		"Agent-Lease-Until: 2026-08-15T08:00:00Z\n" +
		"Agent-Issue: 17\n"
	got := claimMessage(17, "run-test", lease)
	if got != want {
		t.Fatalf("claimMessage = %q, want %q", got, want)
	}
	if err := validateCommitTitle(strings.Split(got, "\n")[0]); err != nil {
		t.Fatalf("claim subject rejected: %v", err)
	}
}
