package workflowctl

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

const validIssueCommentPage = `[{"id":1,"body":"handoff","created_at":"2026-09-03T00:00:00Z","user":{"login":"goxdra[bot]"}}]`

//nolint:gocognit // Table cases cover authenticated page and comment shape before mutation.
func TestReadIssueCommentsValidatesEveryPageAndCommentBeforeReturning(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "valid pages", output: validIssueCommentPage + "\n" + strings.Replace(validIssueCommentPage, `"id":1`, `"id":2`, 1)},
		{name: "null page", output: "null"},
		{name: "null unrelated comment", output: `[{"id":1,"body":"handoff","created_at":"2026-09-03T00:00:00Z","user":{"login":"goxdra[bot]"}},null]`},
		{name: "malformed unrelated comment", output: `[{"id":1,"body":"handoff","created_at":"2026-09-03T00:00:00Z","user":{"login":"goxdra[bot]"}},{"id":2,"body":"","created_at":"2026-09-03T00:00:00Z","user":{"login":"other"}}]`},
		{name: "duplicate comment ID", output: validIssueCommentPage + "\n" + validIssueCommentPage},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutations := 0
			application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				if strings.Contains(command, "issue edit") || strings.Contains(command, "issue comment") {
					mutations++
				}
				if command != "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
					return "", fmt.Errorf("unexpected command: %s", command)
				}
				return test.output, nil
			}}
			comments, err := application.readIssueComments("/repo", 14)
			if test.name == "valid pages" {
				if err != nil {
					t.Fatalf("readIssueComments: %v", err)
				}
				if len(comments) != 2 || comments[0].ID != 1 || comments[1].ID != 2 {
					t.Fatalf("comments = %#v, want two validated pages", comments)
				}
			}
			if test.name != "valid pages" && (err == nil || operationDispositionOf(err) != operationDispositionTerminal) {
				t.Fatalf("readIssueComments error = %v, disposition %d, want terminal", err, operationDispositionOf(err))
			}
			if mutations != 0 {
				t.Fatalf("comment validation mutations = %d, want zero", mutations)
			}
		})
	}
}

func TestReadIssueCommentsCommandFailureIsRetryableWithCause(t *testing.T) {
	sentinel := errors.New("issue comments transport sentinel")
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "gh" || strings.Join(args, " ") != "api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
			return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
		return "", sentinel
	}}
	_, err := application.readIssueComments("/repo", 14)
	if err == nil || operationDispositionOf(err) != operationDispositionRetryable || !errors.Is(err, sentinel) {
		t.Fatalf("readIssueComments error = %v, disposition %d, want retryable cause", err, operationDispositionOf(err))
	}
}
