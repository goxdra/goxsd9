// Package workflowctl implements the repository workflow command.
package workflowctl

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const (
	owner         = "kud360"
	repository    = "goxsd9"
	repositoryKey = owner + "/" + repository
	projectID     = "PVT_kwHOABOZgs4BgaQE"
	projectNumber = 2
)

type app struct {
	ctx    context.Context
	stdout io.Writer
	stderr io.Writer
}

// Run executes workflowctl and returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	a := app{ctx: ctx, stdout: stdout, stderr: stderr}
	err := a.run(args)
	if err == nil {
		return 0
	}

	var exitErr *exitError
	if errors.As(err, &exitErr) {
		if _, writeErr := fmt.Fprintln(stderr, exitErr.Error()); writeErr != nil {
			return 1
		}
		return exitErr.code
	}

	if _, writeErr := fmt.Fprintf(stderr, "workflowctl: %v\n", err); writeErr != nil {
		return 1
	}
	return 1
}

func (a app) run(args []string) error {
	if len(args) == 0 {
		return a.usage()
	}

	switch args[0] {
	case "backlog":
		return a.runBacklog(args[1:])
	case "check":
		return a.runCheck(args[1:])
	case "claim":
		return a.runClaim(args[1:])
	case "docs":
		return a.runDocs(args[1:])
	case "doctor":
		return a.runDoctor(args[1:])
	case "evaluation":
		return a.runEvaluation(args[1:])
	case "handoff":
		return a.runHandoff(args[1:])
	case "history":
		return a.runHistory(args[1:])
	case "issue":
		return a.runIssue(args[1:])
	case "pick":
		return a.runPick(args[1:])
	case "pr":
		return a.runPR(args[1:])
	case "sync":
		return a.runSync(args[1:])
	case "help", "-h", "--help":
		return a.usage()
	default:
		return usageError("unknown command %q", args[0])
	}
}

func (a app) usage() error {
	_, err := fmt.Fprint(a.stdout, `workflowctl mechanizes goxsd9 development.

Usage:
  go tool workflowctl doctor
  go tool workflowctl check [--skip-lint]
  go tool workflowctl docs check
  go tool workflowctl history [--since 7d]
  go tool workflowctl sync
  go tool workflowctl pick [--json]
  go tool workflowctl claim acquire ISSUE
  go tool workflowctl claim renew
  go tool workflowctl claim verify
  go tool workflowctl backlog health
  go tool workflowctl issue create [flags]
  go tool workflowctl handoff ISSUE --body-file FILE
  go tool workflowctl pr open ISSUE --title TITLE --body-file FILE
  go tool workflowctl pr finish PR
  go tool workflowctl evaluation record PR --verdict pass|fail --body-file FILE
`)
	return err
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

func usageError(format string, args ...any) error {
	return &exitError{code: 2, err: fmt.Errorf(format, args...)}
}

func stateError(format string, args ...any) error {
	return &exitError{code: 3, err: fmt.Errorf(format, args...)}
}
