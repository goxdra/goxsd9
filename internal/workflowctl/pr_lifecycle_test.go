package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type historicalPRLifecycleFixture struct {
	name     string
	state    string
	statuses [len(prReviewStateSlotSpecs)]string
	prose    string
}

const (
	historicalPR144DevelopmentStale   = "Development-signal evidence will be installed."
	historicalPR144DocumentationStale = "Managed documentation will receive its audit and Curator review."
	historicalPR144EvaluationStale    = "A fresh challenge-bound Examiner evaluation will be created."
	historicalPR146DevelopmentStale   = "Pending exact-base evidence generation."
	historicalPR188DevelopmentStale   = "Exact-base development signals and documentation audit will be attached after the draft PR exists."
	historicalPR189DevelopmentStale   = "Pending exact-base/head recomputation by workflowctl after the draft PR exists."
)

var historicalPRLifecycleFixtures = [...]historicalPRLifecycleFixture{
	{
		name:  "pr-144",
		state: prReviewStatePending,
		statuses: [len(prReviewStateSlotSpecs)]string{
			historicalPR144DevelopmentStale,
			historicalPR144DocumentationStale,
			historicalPR144EvaluationStale,
		},
		prose: "Exact-base/head development signals were independently recomputed for this change.",
	},
	{
		name:  "pr-146",
		state: prReviewStatePending,
		statuses: [len(prReviewStateSlotSpecs)]string{
			historicalPR146DevelopmentStale,
			"Current documentation changes are covered by a Curator review.",
			"Pending fresh challenge-bound Examiner evaluation.",
		},
		prose: "The default text output is unchanged and JSON reports exact computed deltas.",
	},
	{
		name:  "pr-188",
		state: prReviewStateEvidenceReady,
		statuses: [len(prReviewStateSlotSpecs)]string{
			historicalPR188DevelopmentStale,
			"Curator result is attached in the evidence below.",
			"Fresh challenge-bound Examiner review is required before merge.",
		},
		prose: "Exact-base development signals, documentation audit, and Curator result are attached in the evidence below.",
	},
	{
		name:  "pr-189",
		state: prReviewStateEvidenceReady,
		statuses: [len(prReviewStateSlotSpecs)]string{
			historicalPR189DevelopmentStale,
			"The documentation audit and Curator result are current.",
			"Pending fresh challenge-bound Examiner review.",
		},
		prose: "Exact-base/head development signals were recomputed and the embedded payload is current.",
	},
}

var historicalPRLegacyBodies = [...]struct {
	name string
	body string
}{
	{
		name: "pr-144",
		body: "## Outcome\n\n" + historicalPR144DevelopmentStale + "\n\n" +
			"## Work packet\n\nCloses #13.\n\n## Conformance and documentation\n\n" +
			historicalPR144DocumentationStale + "\n\n## Evaluation\n\n" + historicalPR144EvaluationStale + "\n",
	},
	{
		name: "pr-146",
		body: "## Outcome\n\nThe default text output is unchanged and JSON reports exact computed deltas.\n\n" +
			"## Work packet\n\nCloses #13\n\n### Development signals\n\n" +
			historicalPR146DevelopmentStale + "\n\n" +
			"## Evaluation\n\nPending fresh challenge-bound Examiner evaluation.\n",
	},
	{
		name: "pr-188",
		body: prReviewStateToken(prReviewStateEvidenceReady) + "\n\n## Outcome\n\n" +
			historicalPR188DevelopmentStale + "\n\n" +
			"## Work packet\n\nCloses #13\n\n## Evaluation\n\nFresh challenge-bound Examiner review is required before merge.\n",
	},
	{
		name: "pr-189",
		body: prReviewStateToken(prReviewStateEvidenceReady) + "\n\n## Outcome\n\n" +
			historicalPR189DevelopmentStale + "\n\n" +
			"## Work packet\n\nCloses #13\n\n## Evaluation\n\nPending fresh challenge-bound Examiner review.\n",
	},
}

func historicalPRLifecycleFrame(t *testing.T, fixture historicalPRLifecycleFixture) string {
	t.Helper()
	var frame strings.Builder
	frame.WriteString(prReviewStateToken(fixture.state))
	for index, spec := range prReviewStateSlotSpecs {
		frame.WriteByte('\n')
		frame.WriteString(prReviewStateSlotToken(spec.slot))
		frame.WriteByte('\n')
		frame.WriteString(fixture.statuses[index])
	}
	return frame.String()
}

func historicalPRLifecycleBody(t *testing.T, backend *workflowBackend, fixture historicalPRLifecycleFixture) string {
	t.Helper()
	canonicalFrame := testPRReviewStateFrame(t, prReviewStateEvidenceReady)
	staleFrame := historicalPRLifecycleFrame(t, fixture)
	body := replaceHistoricalPRLifecycleFrame(t, backend.body, canonicalFrame, staleFrame, fixture.name)
	return body + "\n" + fixture.prose + "\n"
}

func replaceHistoricalPRLifecycleFrame(t *testing.T, body, oldFrame, newFrame, fixtureName string) string {
	t.Helper()
	if count := strings.Count(body, oldFrame); count != 1 {
		t.Fatalf("historical %s fixture has %d canonical lifecycle frames, want 1", fixtureName, count)
	}
	return strings.Replace(body, oldFrame, newFrame, 1)
}

func canonicalEvidenceReadyLifecycleFrame() string {
	return prReviewStateToken(prReviewStateEvidenceReady) + "\n" +
		prReviewStateSlotToken("development-signals") + "\n" +
		"Exact-base/head development signals are current in the workflow-owned evidence block.\n" +
		prReviewStateSlotToken("conformance-documentation") + "\n" +
		"Exact-base documentation audit and Curator result are current in the workflow-owned evidence block.\n" +
		prReviewStateSlotToken("evaluation") + "\n" +
		"Evidence is ready for a fresh challenge-bound Examiner evaluation."
}

func TestHistoricalPRLifecycleFixturesConvergeThroughEvidenceUpdate(t *testing.T) {
	for _, fixture := range historicalPRLifecycleFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			testHistoricalPRLifecycleFixture(t, fixture)
		})
	}
}

type historicalPREvidenceSourcePaths struct {
	signals string
	audit   string
}

func historicalPREvidenceSources(t *testing.T, backend *workflowBackend) historicalPREvidenceSourcePaths {
	t.Helper()
	evidence := testWorkflowPREvidence(backend.head)
	return historicalPREvidenceSourcePaths{
		signals: writePREvidenceJSONSource(t, "signals.json", evidence.DevelopmentSignals),
		audit:   writePREvidenceJSONSource(t, "audit.json", evidence.DocumentationAudit),
	}
}

func historicalPREvidenceApplication(backend *workflowBackend) app {
	return app{
		ctx:                            context.Background(),
		executeCommand:                 backend.execute,
		verifyDevelopmentSignalsReport: acceptDevelopmentSignalsForCommandFlow,
		stdout:                         new(bytes.Buffer),
		stderr:                         new(bytes.Buffer),
	}
}

func runHistoricalPREvidenceUpdate(t *testing.T, application app, sources historicalPREvidenceSourcePaths, message string) {
	t.Helper()
	if err := application.runPREvidence([]string{"update", "14", "--signals-file", sources.signals, "--docs-audit-file", sources.audit}); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertHistoricalPREvidenceUpdate(t *testing.T, backend *workflowBackend, beforeBody string, fixture historicalPRLifecycleFixture) {
	t.Helper()
	if backend.bodyPatchCount != 1 {
		t.Fatalf("body PATCH count = %d, want 1", backend.bodyPatchCount)
	}
	wantBody := replaceHistoricalPRLifecycleFrame(t, beforeBody, historicalPRLifecycleFrame(t, fixture), canonicalEvidenceReadyLifecycleFrame(), fixture.name)
	if backend.body != wantBody {
		t.Fatalf("evidence update changed bytes outside lifecycle frame:\n got %q\nwant %q", backend.body, wantBody)
	}
	if !strings.Contains(backend.body, canonicalEvidenceReadyLifecycleFrame()) {
		t.Fatalf("updated body lacks exact canonical evidence-ready lifecycle frame: %q", backend.body)
	}
	if err := requirePRReviewStateReady(backend.body); err != nil {
		t.Fatalf("updated lifecycle is not evidence-ready: %v", err)
	}
}

func assertHistoricalPREvidenceRepeat(t *testing.T, backend *workflowBackend, wantBody string) {
	t.Helper()
	if backend.bodyPatchCount != 1 || backend.body != wantBody {
		t.Fatalf("repeat evidence update changed body or PATCH count: patches=%d body=%q", backend.bodyPatchCount, backend.body)
	}
}

func testHistoricalPRLifecycleFixture(t *testing.T, fixture historicalPRLifecycleFixture) {
	t.Helper()
	backend := newWorkflowBackend(t)
	backend.body = historicalPRLifecycleBody(t, backend, fixture)
	beforeBody := backend.body
	sources := historicalPREvidenceSources(t, backend)
	application := historicalPREvidenceApplication(backend)
	runHistoricalPREvidenceUpdate(t, application, sources, "historical lifecycle evidence update")
	assertHistoricalPREvidenceUpdate(t, backend, beforeBody, fixture)
	wantBody := backend.body
	runHistoricalPREvidenceUpdate(t, application, sources, "repeat historical lifecycle evidence update")
	assertHistoricalPREvidenceRepeat(t, backend, wantBody)
}

func TestHistoricalPRLegacyBodiesFailAtCommandBoundaries(t *testing.T) {
	for _, fixture := range historicalPRLegacyBodies {
		t.Run(fixture.name, func(t *testing.T) {
			for _, command := range []string{"open", "evidence update", "challenge", "finish"} {
				t.Run(command, func(t *testing.T) {
					testHistoricalPRLegacyCommand(t, fixture, command)
				})
			}
		})
	}
}

func runHistoricalPRLegacyCommand(application app, backend *workflowBackend, command string) error {
	switch command {
	case "evidence update":
		return application.runPREvidence([]string{"update", "14", "--signals-file", "missing-signals", "--docs-audit-file", "missing-audit"})
	case "challenge":
		return application.runEvaluation([]string{"challenge", "14"})
	case "finish":
		return application.runPR(backend.finishArgs())
	default:
		return fmt.Errorf("unsupported historical command %q", command)
	}
}

func assertHistoricalPRLegacyRejection(t *testing.T, command string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "review-state") {
		t.Fatalf("%s error = %v, want review-state rejection", command, err)
	}
}

func assertHistoricalPRLegacyNoMutation(t *testing.T, command string, backend *workflowBackend, beforeBody string) {
	t.Helper()
	if backend.body != beforeBody || len(backend.comments) != 0 || backend.bodyPatchCount != 0 ||
		backend.merged || backend.issuePatchCount != 0 || backend.projectDone {
		t.Fatalf("rejected %s changed remote state: body=%t comments=%d body patches=%d merged=%t issue patches=%d project done=%t",
			command, backend.body != beforeBody, len(backend.comments), backend.bodyPatchCount,
			backend.merged, backend.issuePatchCount, backend.projectDone)
	}
}

func testHistoricalPRLegacyCommand(t *testing.T, fixture struct {
	name string
	body string
}, command string) {
	t.Helper()
	if command == "open" {
		_, mutations, err := runOpenLifecycleCommand(t, fixture.body)
		assertHistoricalPRLegacyRejection(t, command, err)
		if len(mutations) != 0 {
			t.Fatalf("rejected open reached remote mutation: %v", mutations)
		}
		return
	}
	backend := newWorkflowBackend(t)
	backend.body = fixture.body
	beforeBody := backend.body
	application := historicalPREvidenceApplication(backend)
	err := runHistoricalPRLegacyCommand(application, backend, command)
	assertHistoricalPRLegacyRejection(t, command, err)
	assertHistoricalPRLegacyNoMutation(t, command, backend, beforeBody)
}

func TestPROpenInstallsCanonicalPendingLifecycleBeforeMutation(t *testing.T) {
	readyFixture := historicalPRLifecycleFixtures[2]
	readyBody := "Opaque prefix with stale pending prose.\n" + historicalPRLifecycleFrame(t, readyFixture) +
		"\n## Outcome\n\nCloses #13\nOpaque suffix with links.\n"
	wantBody, err := replacePRReviewState(readyBody, prReviewStatePending)
	if err != nil {
		t.Fatalf("build expected pending body: %v", err)
	}
	posted, mutations, err := runOpenLifecycleCommand(t, readyBody)
	if err != nil {
		t.Fatalf("open canonical lifecycle body: %v", err)
	}
	if posted.Body != wantBody {
		t.Fatalf("posted body = %q, want %q", posted.Body, wantBody)
	}
	if len(mutations) != 2 || mutations[0] != "git push origin HEAD:refs/heads/agent/issue-13" ||
		mutations[1] != "gh api --method POST repos/goxdra/goxsd9/pulls --input -" {
		t.Fatalf("open mutations = %v, want push followed by PR creation", mutations)
	}
	if marker, err := parsePRReviewStateMarker(posted.Body); err != nil || marker.state != prReviewStatePending {
		t.Fatalf("posted lifecycle state = %v, want pending", err)
	}
}

func runOpenLifecycleCommand(t *testing.T, body string) (createPullRequestRequest, []string, error) {
	t.Helper()
	bodyPath := filepath.Join(t.TempDir(), "pr.md")
	if err := os.WriteFile(bodyPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write PR body: %v", err)
	}
	var posted createPullRequestRequest
	mutations := []string{}
	application := app{
		ctx:    context.Background(),
		stdout: new(bytes.Buffer),
		stderr: new(bytes.Buffer),
		executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
			command := name + " " + strings.Join(args, " ")
			var data []byte
			if input != nil {
				readData, err := io.ReadAll(input)
				if err != nil {
					return "", fmt.Errorf("read %s input: %w", command, err)
				}
				data = readData
			}
			switch command {
			case "git rev-parse --show-toplevel":
				return "/repo", nil
			case "git branch --show-current":
				return "agent/issue-13", nil
			case "git status --porcelain":
				return "", nil
			case "git log --format=%x00%B%x00 origin/main..HEAD":
				return framedCommitLog("fix(workflow): validate lifecycle frame"), nil
			case "git fetch origin refs/heads/agent/issue-13:refs/remotes/origin/agent/issue-13":
				return "", nil
			case "git rev-parse HEAD", "git rev-parse origin/agent/issue-13":
				return "head", nil
			case "git log -100 --format=%B":
				return claimMessage(13, "run-test", time.Now().UTC().Add(time.Hour)), nil
			case "git push origin HEAD:refs/heads/agent/issue-13":
				mutations = append(mutations, command)
				return "", nil
			case "gh api --method POST repos/goxdra/goxsd9/pulls --input -":
				mutations = append(mutations, command)
				if err := json.Unmarshal(data, &posted); err != nil {
					return "", fmt.Errorf("decode posted PR: %w", err)
				}
				return `{"number":14,"html_url":"https://github.com/goxdra/goxsd9/pull/14"}`, nil
			default:
				return "", fmt.Errorf("unexpected command: %s", command)
			}
		},
	}
	err := application.openPullRequest([]string{"13", "--title", "fix(workflow): validate lifecycle frame", "--body-file", bodyPath})
	return posted, mutations, err
}
