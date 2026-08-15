package workflowctl

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	commitTitleLimit = 72
	commitMessageNUL = "\x00"
)

var commitTypes = []string{"feat", "fix", "test", "docs", "refactor", "perf", "ci", "chore"}

func validateCommitTitle(title string) error {
	if strings.TrimSpace(title) != title || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return errors.New("title must be one trimmed line")
	}
	if utf8.RuneCountInString(title) > commitTitleLimit {
		return fmt.Errorf("title exceeds %d characters", commitTitleLimit)
	}
	prefix, summary, ok := strings.Cut(title, ": ")
	if !ok || prefix == "" || summary == "" {
		return errors.New("title must match <type>(<scope>): <summary>")
	}
	if strings.TrimSpace(summary) != summary {
		return errors.New("summary must not start or end with whitespace")
	}
	if strings.HasSuffix(summary, ".") {
		return errors.New("summary must not end with a period")
	}
	first, _ := utf8.DecodeRuneInString(summary)
	if !unicode.IsLower(first) && !unicode.IsDigit(first) {
		return errors.New("summary must start with a lowercase letter or digit")
	}
	typeName, scope, err := parseCommitPrefix(prefix)
	if err != nil {
		return err
	}
	if !containsString(commitTypes, typeName) {
		return fmt.Errorf("unknown commit type %q", typeName)
	}
	if scope != "" && !containsString(validAreas, scope) {
		return fmt.Errorf("unknown commit scope %q", scope)
	}
	return nil
}

func (a app) validateWorkCommitTitles(root, head string) error {
	output, err := a.command(root, "git", "log", "--format=%x00%B%x00", "origin/main.."+head)
	if err != nil {
		return fmt.Errorf("read work commit titles: %w", err)
	}
	if output == "" {
		return errors.New("claim branch has no commits beyond origin/main")
	}
	messages, err := parseCommitMessages(output)
	if err != nil {
		return err
	}
	for _, message := range messages {
		title, rest, hasRest := strings.Cut(message, "\n")
		if hasRest && rest != "" && !strings.HasPrefix(rest, "\n") {
			return fmt.Errorf("commit title %q must be followed by a blank line before the body", title)
		}
		if err := validateCommitTitle(title); err != nil {
			return fmt.Errorf("commit title %q is invalid: %w", title, err)
		}
	}
	return nil
}

func parseCommitMessages(output string) ([]string, error) {
	parts := strings.Split(output, commitMessageNUL)
	if len(parts) < 3 || len(parts)%2 == 0 || parts[0] != "" || parts[len(parts)-1] != "" {
		return nil, errors.New("git returned malformed commit message framing")
	}
	messages := make([]string, 0, len(parts)/2)
	for index := 1; index < len(parts)-1; index += 2 {
		messages = append(messages, parts[index])
		if index+1 < len(parts)-1 && parts[index+1] != "\n" {
			return nil, errors.New("git returned malformed commit message separation")
		}
	}
	return messages, nil
}

func parseCommitPrefix(prefix string) (string, string, error) {
	prefix = strings.TrimSuffix(prefix, "!")
	open := strings.IndexByte(prefix, '(')
	if open < 0 {
		if prefix == "" || strings.ContainsAny(prefix, ")!") {
			return "", "", errors.New("commit type or breaking marker is malformed")
		}
		return prefix, "", nil
	}
	if open == 0 || !strings.HasSuffix(prefix, ")") || strings.Count(prefix, "(") != 1 ||
		strings.Count(prefix, ")") != 1 {
		return "", "", errors.New("commit scope is malformed")
	}
	typeName := prefix[:open]
	scope := prefix[open+1 : len(prefix)-1]
	if scope == "" || strings.ContainsAny(typeName+scope, "!") {
		return "", "", errors.New("commit type or scope is empty or malformed")
	}
	return typeName, scope, nil
}
