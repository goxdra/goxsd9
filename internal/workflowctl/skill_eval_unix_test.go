//go:build unix

package workflowctl

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartSkillEvalProcessReadsAndCleansResult(t *testing.T) {
	directory := t.TempDir()
	codex := filepath.Join(directory, "codex")
	workingDirectoryFile := filepath.Join(directory, "working-directory.txt")
	script := `#!/bin/sh
pwd > "$SKILL_EVAL_WORKING_DIRECTORY"
while [ "$#" -gt 0 ]; do
    if [ "$1" = "--output-last-message" ]; then
        result="$2"
        shift 2
        continue
    fi
    shift
done
printf '%s' '{"decision":"safe","actions":[],"prohibitedActions":[]}' > "$result"
`
	// #nosec G306 -- the temporary test command must be executable by the child process.
	if err := os.WriteFile(codex, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	originalPath := os.Getenv("PATH")
	originalWorkingDirectory := os.Getenv("SKILL_EVAL_WORKING_DIRECTORY")
	if err := os.Setenv("PATH", directory+string(os.PathListSeparator)+originalPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	if err := os.Setenv("SKILL_EVAL_WORKING_DIRECTORY", workingDirectoryFile); err != nil {
		t.Fatalf("set working directory path: %v", err)
	}
	defer func() {
		if err := os.Setenv("PATH", originalPath); err != nil {
			t.Errorf("restore PATH: %v", err)
		}
		if err := os.Setenv("SKILL_EVAL_WORKING_DIRECTORY", originalWorkingDirectory); err != nil {
			t.Errorf("restore working directory path: %v", err)
		}
	}()

	application := app{ctx: context.Background(), stdout: io.Discard}
	for _, test := range []struct {
		role   string
		schema string
	}{
		{role: "subject", schema: skillEvalSubjectSchema},
		{role: "grader", schema: skillEvalGraderSchema},
	} {
		assertSkillEvalProcessWorkingDirectory(t, application, test.role, test.schema, workingDirectoryFile)
	}
}

func assertSkillEvalProcessWorkingDirectory(t *testing.T, application app, role, schema, workingDirectoryFile string) {
	t.Helper()
	process, err := application.startSkillEvalProcess(role, skillEvalAgentRequest{
		Model: "test-model", Prompt: "isolated prompt", Schema: schema,
	})
	if err != nil {
		t.Fatalf("startSkillEvalProcess(%s): %v", role, err)
	}
	codexProcess, ok := process.(*skillEvalCommandProcess)
	if !ok {
		t.Fatalf("process type = %T, want *skillEvalCommandProcess", process)
	}
	privateDirectory := codexProcess.directory
	result, err := process.wait()
	if err != nil {
		t.Fatalf("wait skill evaluation process(%s): %v", role, err)
	}
	if !strings.Contains(string(result), `"decision":"safe"`) {
		t.Fatalf("result(%s) = %s", role, result)
	}
	// #nosec G304 -- this is the test-owned path created above.
	workingDirectory, err := os.ReadFile(workingDirectoryFile)
	if err != nil {
		t.Fatalf("read working directory(%s): %v", role, err)
	}
	if got := strings.TrimSpace(string(workingDirectory)); got != privateDirectory {
		t.Fatalf("working directory(%s) = %q, want %q", role, got, privateDirectory)
	}
	if _, err := os.Stat(privateDirectory); !os.IsNotExist(err) {
		t.Fatalf("private directory(%s) remains after wait: %v", role, err)
	}
}
