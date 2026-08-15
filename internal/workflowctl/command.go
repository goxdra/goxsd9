package workflowctl

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func (a app) command(dir, name string, args ...string) (string, error) {
	return a.commandInput(dir, nil, name, args...)
}

func (a app) commandInput(dir string, input io.Reader, name string, args ...string) (string, error) {
	// #nosec G204 -- callers select repository-owned commands and fixed arguments.
	cmd := exec.CommandContext(a.ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = input

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	text := strings.TrimSpace(output.String())
	if err == nil {
		return text, nil
	}
	if text == "" {
		return "", fmt.Errorf("run %s: %w", name, err)
	}
	return "", fmt.Errorf("run %s: %w: %s", name, err, text)
}

func (a app) root() (string, error) {
	root, err := a.command("", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("find repository root: %w", err)
	}
	return root, nil
}

func writeLine(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format+"\n", args...)
	return err
}
