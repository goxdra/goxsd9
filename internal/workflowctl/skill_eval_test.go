package workflowctl

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type capturedSkillEvalRequest struct {
	request skillEvalAgentRequest
	role    string
}

func TestLoadSkillEvalCasesSortsAndSeparatesExpectedBehavior(t *testing.T) {
	root := t.TempDir()
	writeSkillEvalTestFile(t, root, "AGENTS.md", "repository policy")
	writeSkillEvalTestFile(t, root, ".codex/agents/curator.toml", "curator policy")
	writeSkillEvalTestFile(t, root, ".agents/skills/develop/SKILL.md", "develop policy")
	writeSkillEvalTestFile(t, root, ".codex/agents/smith.toml", "smith policy")
	writeSkillEvalTestFile(t, root, ".codex/agents/examiner.toml", "examiner policy")
	writeSkillEvalTestFile(t, root, "evals/agent/develop/z-last.md", `# Last case

Scenario: last input marker.

Expected behavior: last expected marker.
`)
	writeSkillEvalTestFile(t, root, "evals/agent/curator/a-first.md", `# First case

Scenario: first input marker.

Expected behavior: first expected marker.
`)
	writeSkillEvalTestFile(t, root, "evals/agent/review/nested/middle.md", `# Nested case

Scenario: nested input marker.

Expected behavior: nested expected marker.
`)

	cases, err := loadSkillEvalCases(root, "")
	if err != nil {
		t.Fatalf("loadSkillEvalCases: %v", err)
	}
	paths := []string{cases[0].path, cases[1].path, cases[2].path}
	want := []string{
		"evals/agent/curator/a-first.md",
		"evals/agent/develop/z-last.md",
		"evals/agent/review/nested/middle.md",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("case paths = %#v, want %#v", paths, want)
	}
	if strings.Contains(cases[0].scenario, cases[0].expected) {
		t.Fatal("scenario leaked expected behavior")
	}
	if !strings.Contains(cases[0].suite.policy, "curator policy") {
		t.Fatal("curator case did not receive Curator policy")
	}
	if !strings.Contains(cases[1].suite.policy, "develop policy") || !strings.Contains(cases[1].suite.policy, "smith policy") {
		t.Fatal("develop case did not receive skill and Smith policies")
	}
	if cases[0].suite == cases[1].suite {
		t.Fatal("different suites shared one policy representation")
	}
	if cases[2].suite.name != "review" || !strings.Contains(cases[2].suite.policy, "examiner policy") {
		t.Fatal("nested review case did not resolve the top-level review suite")
	}
}

func TestLoadSkillEvalCasesRejectsMalformedCorpusBeforeEvaluation(t *testing.T) {
	root := t.TempDir()
	writeSkillEvalTestFile(t, root, "AGENTS.md", "repository policy")
	writeSkillEvalTestFile(t, root, ".codex/agents/examiner.toml", "examiner policy")
	writeSkillEvalTestFile(t, root, "evals/agent/review/malformed.md", "# Missing expected behavior\n\nScenario only.\n")

	_, err := loadSkillEvalCases(root, "")
	if err == nil || !strings.Contains(err.Error(), "Expected behavior") {
		t.Fatalf("loadSkillEvalCases error = %v, want expected-behavior error", err)
	}
}

func TestParseSkillEvalCaseRejectsSecondTitle(t *testing.T) {
	content := "# First title\n\nScenario.\n# Second title\n\nExpected behavior: result.\n"
	_, err := parseSkillEvalCase("evals/agent/review/malformed.md", &skillEvalSuite{}, content)
	if err == nil || !strings.Contains(err.Error(), "exactly one level-one title") {
		t.Fatalf("parseSkillEvalCase error = %v, want duplicate-title error", err)
	}
}

func TestLoadSkillEvalCasesRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	writeSkillEvalTestFile(t, root, "AGENTS.md", "repository policy")
	writeSkillEvalTestFile(t, root, ".codex/agents/examiner.toml", "examiner policy")
	targetRoot := t.TempDir()
	target := filepath.Join(targetRoot, "outside.md")
	writeSkillEvalTestFile(t, targetRoot, "outside.md", "# Outside\n\nScenario.\n\nExpected behavior: hidden.\n")
	link := filepath.Join(root, "evals", "agent", "review", "outside.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatalf("create symlink directory: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := loadSkillEvalCases(root, "")
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("loadSkillEvalCases error = %v, want non-regular-file error", err)
	}
}

func TestEvaluateSkillCasesUsesFreshSubjectAndGraderRequests(t *testing.T) {
	suite := &skillEvalSuite{name: "test", policy: "policy"}
	cases := []skillEvalCase{
		{path: "evals/agent/a.md", scenario: "scenario-a", expected: "secret-a", suite: suite, title: "A"},
		{path: "evals/agent/b.md", scenario: "scenario-b", expected: "secret-b", suite: suite, title: "B"},
	}
	var requests []capturedSkillEvalRequest
	var output strings.Builder
	application := app{
		ctx: context.Background(),
		skillEvalSubjectAgent: func(request skillEvalAgentRequest) ([]byte, error) {
			requests = append(requests, capturedSkillEvalRequest{request: request, role: "subject"})
			return []byte(`{"decision":"act safely","actions":["required action"],"prohibitedActions":["unsafe action"]}`), nil
		},
		skillEvalGraderAgent: func(request skillEvalAgentRequest) ([]byte, error) {
			requests = append(requests, capturedSkillEvalRequest{request: request, role: "grader"})
			return []byte(`{"verdict":"pass","summary":"matches","findings":[]}`), nil
		},
		stdout: &output,
	}

	if err := application.evaluateSkillCases(cases, "test-model"); err != nil {
		t.Fatalf("evaluateSkillCases: %v", err)
	}
	if len(requests) != 4 {
		t.Fatalf("agent requests = %d, want 4", len(requests))
	}
	for index := 0; index < len(requests); index += 2 {
		assertSkillEvalRequestPair(t, requests[index:index+2], cases[index/2])
	}
	if got := output.String(); strings.Index(got, cases[0].path) >= strings.Index(got, cases[1].path) {
		t.Fatalf("results are not in input order:\n%s", got)
	}
}

func TestEvaluateSkillCasesRejectsInconsistentGrade(t *testing.T) {
	evalCase := skillEvalCase{
		path: "evals/agent/case.md", scenario: "scenario", expected: "expected",
		suite: &skillEvalSuite{name: "test", policy: "policy"}, title: "Case",
	}
	calls := 0
	var output strings.Builder
	application := app{
		ctx: context.Background(),
		skillEvalSubjectAgent: func(_ skillEvalAgentRequest) ([]byte, error) {
			calls++
			return []byte(`{"decision":"decision","actions":[],"prohibitedActions":[]}`), nil
		},
		skillEvalGraderAgent: func(_ skillEvalAgentRequest) ([]byte, error) {
			calls++
			return []byte(`{"verdict":"pass","summary":"contradictory","findings":[{"expected":"x","observed":"y"}]}`), nil
		},
		stdout: &output,
	}

	err := application.evaluateSkillCases([]skillEvalCase{evalCase}, "test-model")
	if err == nil || !strings.Contains(err.Error(), "1 of 1") {
		t.Fatalf("evaluateSkillCases error = %v, want aggregate failure", err)
	}
	if calls != 2 {
		t.Fatalf("agent calls = %d, want 2", calls)
	}
	if !strings.Contains(output.String(), "passing result has findings") {
		t.Fatalf("output omitted structured result error:\n%s", output.String())
	}
}

func TestEvaluateSkillCasesRejectsNullSubjectArrays(t *testing.T) {
	evalCase := skillEvalCase{
		path: "evals/agent/case.md", scenario: "scenario", expected: "expected",
		suite: &skillEvalSuite{name: "test", policy: "policy"}, title: "Case",
	}
	graderCalled := false
	var output strings.Builder
	application := app{
		ctx: context.Background(),
		skillEvalSubjectAgent: func(_ skillEvalAgentRequest) ([]byte, error) {
			return []byte(`{"decision":"decision","actions":null,"prohibitedActions":[]}`), nil
		},
		skillEvalGraderAgent: func(_ skillEvalAgentRequest) ([]byte, error) {
			graderCalled = true
			return nil, nil
		},
		stdout: &output,
	}

	err := application.evaluateSkillCases([]skillEvalCase{evalCase}, "test-model")
	if err == nil || !strings.Contains(output.String(), "actions is null") {
		t.Fatalf("evaluateSkillCases error = %v, output = %q", err, output.String())
	}
	if graderCalled {
		t.Fatal("grader ran after malformed subject output")
	}
}

func TestValidateSkillEvalGradeRejectsMalformedResults(t *testing.T) {
	tests := []struct {
		name  string
		grade skillEvalGrade
	}{
		{name: "null findings", grade: skillEvalGrade{Verdict: "pass", Summary: "summary"}},
		{name: "empty finding", grade: skillEvalGrade{
			Verdict: "fail", Summary: "summary", Findings: []skillEvalFinding{{}},
		}},
		{name: "passing findings", grade: skillEvalGrade{
			Verdict: "pass", Summary: "summary",
			Findings: []skillEvalFinding{{Expected: "expected", Observed: "observed"}},
		}},
		{name: "failing without findings", grade: skillEvalGrade{
			Verdict: "fail", Summary: "summary", Findings: []skillEvalFinding{},
		}},
	}
	for _, test := range tests {
		if err := validateSkillEvalGrade(test.grade); err == nil {
			t.Fatalf("%s: validateSkillEvalGrade accepted malformed result", test.name)
		}
	}
}

func TestEvaluateSkillCasesContinuesAfterBackendError(t *testing.T) {
	suite := &skillEvalSuite{name: "test", policy: "policy"}
	cases := []skillEvalCase{
		{path: "evals/agent/a.md", scenario: "scenario-a", expected: "expected-a", suite: suite, title: "A"},
		{path: "evals/agent/b.md", scenario: "scenario-b", expected: "expected-b", suite: suite, title: "B"},
	}
	graders := 0
	var output strings.Builder
	application := app{
		ctx: context.Background(),
		skillEvalSubjectAgent: func(request skillEvalAgentRequest) ([]byte, error) {
			if strings.Contains(request.Prompt, "scenario-a") {
				return nil, errors.New("subject unavailable")
			}
			return []byte(`{"decision":"decision","actions":[],"prohibitedActions":[]}`), nil
		},
		skillEvalGraderAgent: func(_ skillEvalAgentRequest) ([]byte, error) {
			graders++
			return []byte(`{"verdict":"pass","summary":"matches","findings":[]}`), nil
		},
		stdout: &output,
	}

	err := application.evaluateSkillCases(cases, "test-model")
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("evaluateSkillCases error = %v, want one aggregate failure", err)
	}
	if graders != 1 {
		t.Fatalf("grader calls = %d, want 1", graders)
	}
	got := output.String()
	if !strings.Contains(got, "[error] evals/agent/a.md") || !strings.Contains(got, "[pass] evals/agent/b.md") {
		t.Fatalf("output omitted continued result:\n%s", got)
	}
}

func TestRunCodexSkillEvalAgentUsesEphemeralReadOnlyInvocation(t *testing.T) {
	var isolatedDirectory string
	application := app{
		ctx: context.Background(),
		executeCommand: func(directory string, input io.Reader, name string, args ...string) (string, error) {
			isolatedDirectory = directory
			if name != "codex" {
				t.Fatalf("command = %q, want codex", name)
			}
			joined := strings.Join(args, " ")
			requireSkillEvalArguments(t, joined)
			prompt, err := io.ReadAll(input)
			if err != nil {
				t.Fatalf("read prompt: %v", err)
			}
			if string(prompt) != "isolated prompt" {
				t.Fatalf("prompt = %q", prompt)
			}
			resultPath := argumentValue(t, args, "--output-last-message")
			if err := os.WriteFile(resultPath, []byte(`{"decision":"safe","actions":[],"prohibitedActions":[]}`), 0o600); err != nil {
				t.Fatalf("write fake result: %v", err)
			}
			return "", nil
		},
	}

	result, err := application.runCodexSkillEvalAgent("subject", skillEvalAgentRequest{
		Model: "test-model", Prompt: "isolated prompt", Schema: skillEvalSubjectSchema,
	})
	if err != nil {
		t.Fatalf("runCodexSkillEvalAgent: %v", err)
	}
	if !strings.Contains(string(result), `"decision":"safe"`) {
		t.Fatalf("result = %s", result)
	}
	if _, err := os.Stat(isolatedDirectory); !os.IsNotExist(err) {
		t.Fatalf("isolated directory remains after run: %v", err)
	}
}

func writeSkillEvalTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func assertSkillEvalRequestPair(t *testing.T, requests []capturedSkillEvalRequest, evalCase skillEvalCase) {
	t.Helper()
	subject := requests[0]
	grader := requests[1]
	if subject.role != "subject" || grader.role != "grader" {
		t.Fatalf("request roles = %q, %q", subject.role, grader.role)
	}
	if strings.Contains(subject.request.Prompt, evalCase.expected) {
		t.Fatalf("subject prompt leaked expected behavior for %s", evalCase.path)
	}
	if !strings.Contains(grader.request.Prompt, evalCase.expected) {
		t.Fatalf("grader prompt omitted expected behavior for %s", evalCase.path)
	}
	if !strings.Contains(grader.request.Prompt, evalCase.scenario) {
		t.Fatalf("grader prompt omitted scenario for %s", evalCase.path)
	}
	if subject.request.Model != "test-model" || grader.request.Model != "test-model" {
		t.Fatal("model was not propagated to both fresh requests")
	}
}

func requireSkillEvalArguments(t *testing.T, command string) {
	t.Helper()
	requiredArguments := []string{
		"exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check",
		"--sandbox read-only", "shell_environment_policy.inherit=none",
	}
	for _, required := range requiredArguments {
		if !strings.Contains(command, required) {
			t.Fatalf("command omitted %q: %s", required, command)
		}
	}
}

func argumentValue(t *testing.T, args []string, name string) string {
	t.Helper()
	for index := range args {
		if args[index] != name {
			continue
		}
		if index+1 == len(args) {
			t.Fatalf("argument %s has no value", name)
		}
		return args[index+1]
	}
	t.Fatalf("argument %s was not provided", name)
	return ""
}
