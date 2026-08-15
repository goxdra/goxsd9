package workflowctl

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type commandExecutor func(dir string, input io.Reader, name string, args ...string) (string, error)

func (a app) command(dir, name string, args ...string) (string, error) {
	return a.commandInput(dir, nil, name, args...)
}

func (a app) commandInput(dir string, input io.Reader, name string, args ...string) (string, error) {
	return a.commandOutput(dir, input, true, name, args...)
}

func (a app) gitRaw(dir string, args ...string) (string, error) {
	return a.commandOutput(dir, nil, false, "git", args...)
}

func (a app) commandOutput(dir string, input io.Reader, trim bool, name string, args ...string) (string, error) {
	if a.executeCommand != nil {
		return a.executeCommand(dir, input, name, args...)
	}
	// #nosec G204 -- callers select repository-owned commands and fixed arguments.
	cmd := exec.CommandContext(a.ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = input

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err == nil {
		if trim {
			return strings.TrimSpace(output.String()), nil
		}
		return output.String(), nil
	}
	text := strings.TrimSpace(output.String())
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
