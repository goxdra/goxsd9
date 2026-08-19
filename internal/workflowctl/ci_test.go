package workflowctl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCIUsesCurrentPullRequestMergeBaseForDevelopmentSignals(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not locate the test source")
	}
	workflowPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", ".github", "workflows", "ci.yml")
	// #nosec G304 -- the path is derived from this checked-in test source.
	content, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflow := string(content)
	for _, required := range []string{
		`BASE_REF: ${{ github.event.pull_request.base.ref }}`,
		`PUSH_BASE_SHA: ${{ github.event.before }}`,
		`git fetch --no-tags origin "refs/heads/$BASE_REF:refs/remotes/origin/$BASE_REF"`,
		`BASE_SHA="$(git merge-base HEAD "refs/remotes/origin/$BASE_REF")"`,
		`BASE_SHA="$PUSH_BASE_SHA"`,
		`git diff --quiet "$BASE_SHA" HEAD -- syntax.go datatype.go`,
		`go tool workflowctl develop-signals --base "$BASE_SHA" --format text`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("CI workflow missing %q", required)
		}
	}
	if strings.Contains(workflow, "github.event.pull_request.base.sha") {
		t.Fatal("CI workflow still uses the event snapshot base SHA")
	}
	if strings.Index(workflow, `git fetch --no-tags origin`) > strings.Index(workflow, `git diff --quiet "$BASE_SHA"`) {
		t.Fatal("CI workflow selects changed paths before fetching the current PR base")
	}
}
