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
	script := `#!/bin/sh
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
	if err := os.Setenv("PATH", directory+string(os.PathListSeparator)+originalPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer func() {
		if err := os.Setenv("PATH", originalPath); err != nil {
			t.Errorf("restore PATH: %v", err)
		}
	}()

	application := app{ctx: context.Background(), stdout: io.Discard}
	process, err := application.startSkillEvalProcess("subject", skillEvalAgentRequest{
		Model: "test-model", Prompt: "isolated prompt", Schema: skillEvalSubjectSchema,
	})
	if err != nil {
		t.Fatalf("startSkillEvalProcess: %v", err)
	}
	codexProcess, ok := process.(*skillEvalCommandProcess)
	if !ok {
		t.Fatalf("process type = %T, want *skillEvalCommandProcess", process)
	}
	privateDirectory := codexProcess.directory
	result, err := process.wait()
	if err != nil {
		t.Fatalf("wait skill evaluation process: %v", err)
	}
	if !strings.Contains(string(result), `"decision":"safe"`) {
		t.Fatalf("result = %s", result)
	}
	if _, err := os.Stat(privateDirectory); !os.IsNotExist(err) {
		t.Fatalf("private directory remains after wait: %v", err)
	}
}
