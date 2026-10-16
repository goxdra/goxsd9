package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func measuredChallengeApplication(backend *workflowBackend, stdout *bytes.Buffer, measurements *int) app {
	return app{
		ctx: context.Background(), executeCommand: backend.execute,
		buildDevelopmentSignalsReport: func(_ string, _, _ string, _ time.Duration,
			_ []coverageExplanation, _ []additionalFuzzTarget,
		) (developmentSignalsReport, error) {
			*measurements++
			return testWorkflowPREvidence(backend.head).DevelopmentSignals, nil
		},
		stdout: stdout, stderr: new(bytes.Buffer),
	}
}

func TestEvaluationChallengeNoopMeasuresOncePerCommand(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	measurements := 0
	application := measuredChallengeApplication(backend, &stdout, &measurements)
	first := requestTestChallenge(t, &application, &stdout)
	if measurements != 1 || backend.commentPostCount != 1 {
		t.Fatalf("first challenge: measurements=%d posts=%d, want 1 each", measurements, backend.commentPostCount)
	}
	stdout.Reset()
	second := requestTestChallenge(t, &application, &stdout)
	if measurements != 2 || backend.commentPostCount != 1 || second.Challenge != first.Challenge {
		t.Fatalf("separate retry: measurements=%d posts=%d challenge=%q, want independent measurement and exact replay",
			measurements, backend.commentPostCount, second.Challenge)
	}
}

func TestEvaluationChallengeRejectsCopiedSignalJSONBeforeConvergence(t *testing.T) {
	backend := newWorkflowBackend(t)
	makeSignalEvidenceStale(t, backend)
	var stdout bytes.Buffer
	measurements := 0
	application := measuredChallengeApplication(backend, &stdout, &measurements)
	err := application.runEvaluation([]string{"challenge", "14"})
	if err == nil || !strings.Contains(err.Error(), "complete v2 payload") {
		t.Fatalf("copied signal evidence error = %v, want exact JSON mismatch", err)
	}
	if measurements != 1 || backend.commentPostCount != 0 || stdout.Len() != 0 {
		t.Fatalf("copied evidence: measurements=%d posts=%d output=%q, want 1, 0, empty",
			measurements, backend.commentPostCount, stdout.String())
	}
}

func TestEvaluationChallengeClosureMutationRemeasures(t *testing.T) {
	backend := newWorkflowBackend(t)
	var stdout bytes.Buffer
	measurements := 0
	application := measuredChallengeApplication(backend, &stdout, &measurements)
	canonical := requestTestChallenge(t, &application, &stdout)
	appendFinalEquivalentChallenge(t, backend)
	receipt := evaluationTerminalConflictReceiptComment(t, canonical, "completed-before-closure", 1,
		canonical.RequestedAt.Add(2*time.Second))
	receipt.ID = int64(len(backend.comments) + 1)
	appendWorkflowEvaluationComment(t, backend, receipt)
	measurements = 0
	stdout.Reset()
	if err := application.runEvaluation([]string{"challenge", "14"}); err != nil {
		t.Fatalf("challenge after closure mutation: %v", err)
	}
	if measurements != 2 {
		t.Fatalf("closure mutation measured signals %d times, want two complete measurements", measurements)
	}
	history := workflowEvaluationHistory(t, backend, 14)
	if len(history.closures) != 1 || len(history.receipts) != 1 || len(history.challenges) != 3 {
		t.Fatalf("closure mutation history: closures=%d receipts=%d challenges=%d, want 1, 1, 3",
			len(history.closures), len(history.receipts), len(history.challenges))
	}
}

//nolint:gocognit // The table exercises each independently bound REST field at one command boundary.
func TestEvaluationChallengeNoopFinalRESTProofRejectsDrift(t *testing.T) {
	tests := []struct {
		name   string
		change func(*pullRequestAPI)
	}{
		{name: "state", change: func(pr *pullRequestAPI) { pr.State = "closed" }},
		{name: "base ref", change: func(pr *pullRequestAPI) { pr.Base.Ref = "release" }},
		{name: "base SHA", change: func(pr *pullRequestAPI) { pr.Base.SHA = "moved-base" }},
		{name: "head ref", change: func(pr *pullRequestAPI) { pr.Head.Ref = "other-branch" }},
		{name: "head SHA", change: func(pr *pullRequestAPI) { pr.Head.SHA = "moved-head" }},
		{name: "body", change: func(pr *pullRequestAPI) { pr.Body += "\nchanged" }},
		{name: "evidence", change: func(pr *pullRequestAPI) {
			pr.Body = strings.Replace(pr.Body, `"selection":"no-relevant-target"`,
				`"selection":"unsupported"`, 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			var stdout bytes.Buffer
			measurements := 0
			application := measuredChallengeApplication(backend, &stdout, &measurements)
			reads := 0
			application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
				output, err := backend.execute(dir, input, name, args...)
				if err != nil || name != "gh" || strings.Join(args, " ") != "api repos/goxdra/goxsd9/pulls/14" {
					return output, err
				}
				reads++
				if reads != 2 {
					return output, nil
				}
				var pr pullRequestAPI
				if decodeErr := json.Unmarshal([]byte(output), &pr); decodeErr != nil {
					t.Fatalf("decode PR response: %v", decodeErr)
				}
				test.change(&pr)
				modified, err := json.Marshal(pr)
				return string(modified), err
			}
			err := application.runEvaluation([]string{"challenge", "14"})
			if err == nil || !strings.Contains(err.Error(), "changed after evaluation evidence validation") {
				t.Fatalf("final %s drift = %v, want no-op proof refusal", test.name, err)
			}
			if measurements != 1 || backend.commentPostCount != 0 || stdout.Len() != 0 {
				t.Fatalf("final %s drift: measurements=%d posts=%d output=%q, want 1, 0, empty",
					test.name, measurements, backend.commentPostCount, stdout.String())
			}
		})
	}
}

//nolint:gocognit // The table exercises each independent final history and Git proof exit.
func TestEvaluationChallengeNoopFinalHistoryAndLocalProofRejectsDrift(t *testing.T) {
	tests := []struct {
		name string
		flip func(*testing.T, *workflowBackend, string) (string, error, bool)
	}{
		{name: "history advanced", flip: func(t *testing.T, backend *workflowBackend, command string) (string, error, bool) {
			if command != "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
				return "", nil, false
			}
			comment := issueCommentAPI{ID: 1, Body: "later comment", CreatedAt: time.Now().UTC().Truncate(time.Second)}
			comment.User.Login = trustedActor
			backend.comments = append(backend.comments, comment)
			return backend.commentsJSONResult(t), nil, true
		}},
		{name: "dirty worktree", flip: func(_ *testing.T, _ *workflowBackend, command string) (string, error, bool) {
			if command == "git status --porcelain=v1 --untracked-files=all --ignore-submodules=none" {
				return " M internal/workflowctl/evaluation.go", nil, true
			}
			return "", nil, false
		}},
		{name: "local HEAD moved", flip: func(_ *testing.T, _ *workflowBackend, command string) (string, error, bool) {
			if command == "git rev-parse --verify --end-of-options HEAD^{commit}" {
				return "moved-local-head", nil, true
			}
			return "", nil, false
		}},
		{name: "local base moved", flip: func(_ *testing.T, _ *workflowBackend, command string) (string, error, bool) {
			if command == "git rev-parse --verify --end-of-options base-sha^{commit}" {
				return "moved-local-base", nil, true
			}
			return "", nil, false
		}},
		{name: "ambiguous final read", flip: func(_ *testing.T, _ *workflowBackend, command string) (string, error, bool) {
			if command == "gh api repos/goxdra/goxsd9/pulls/14" {
				return "", errors.New("ambiguous REST read"), true
			}
			return "", nil, false
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowBackend(t)
			var stdout bytes.Buffer
			measurements := 0
			application := measuredChallengeApplication(backend, &stdout, &measurements)
			reads := 0
			application.executeCommand = func(dir string, input io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				if command == "gh api repos/goxdra/goxsd9/pulls/14" {
					reads++
				}
				if reads == 2 {
					if output, err, handled := test.flip(t, backend, command); handled {
						return output, err
					}
				}
				return backend.execute(dir, input, name, args...)
			}
			if err := application.runEvaluation([]string{"challenge", "14"}); err == nil {
				t.Fatal("changed or ambiguous final proof posted a challenge")
			}
			if measurements != 1 || backend.commentPostCount != 0 || stdout.Len() != 0 {
				t.Fatalf("final %s proof: measurements=%d posts=%d output=%q, want 1, 0, empty",
					test.name, measurements, backend.commentPostCount, stdout.String())
			}
		})
	}
}

func (b *workflowBackend) commentsJSONResult(t *testing.T) string {
	t.Helper()
	output, err := b.commentsJSON()
	if err != nil {
		t.Fatalf("encode advanced comments: %v", err)
	}
	return output
}
