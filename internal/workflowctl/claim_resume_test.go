package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const issue305TerminalHandoffBody = `# Handoff: issue #305

## Block

The claimed packet could not reach implementation because the Smith child
agent remained running without producing a handoff or an error state. The
child was given the complete issue contract and two bounded follow-ups. Six
wait windows totaling more than twenty minutes timed out; the implementation
worktree remained clean throughout. The child was closed only after its
previous status was reported as ` + "`running`" + `.

## Decisions and evidence

- Coordination checkout passed ` + "`go tool workflowctl doctor`" + ` on clean ` + "`main`" + `
  at ` + "`f7478ec`" + `; ` + "`sync`" + `, ` + "`pick`" + `, and claim acquisition selected issue #305.
- Issue #305 is the sole claimed packet: retain effective ` + "`whiteSpace`" + ` facts for supported atomic string restrictions, including inheritance, fixed
  constraints, XSD 1.0/1.1 coverage, and immutable views. Validation,
  generation, lists/unions, length/pattern, and built-in derived string
  families remain out of scope.
- Scribe evidence: XSD 1.0/1.1 ` + "`#rf-whiteSpace`" + `; pinned artifacts
  ` + "`internal/specs/testdata/bootstrap/xsd10-datatypes-schema.raw`" + ` lines 91-109,
  492-506 and ` + "`xsd11-datatypes-schema.raw`" + ` lines 236-242, 371-396. The
  required order is ` + "`preserve -> replace -> collapse`" + `; built-in ` + "`xs:string`" + ` is preserve and not fixed.
- Mason evidence: the intended representation is the existing
  ` + "`schemaStringFacetVariant`" + ` in ` + "`schema.go`" + `; parsing remains in
  ` + "`schemaStringFacetDeclarationSet`" + ` in ` + "`schema_build.go`" + `; derivation belongs
  in ` + "`restrictSchemaStringFacets`" + `; the anonymous bridge is
  ` + "`validateInlineSchemaType`" + `. No files were changed by Mason.
- The preserved issue worktree is
  ` + "`/home/paseouser/workspace/goxsd9-worktrees/issue-305-run-0e4ad40a7c3a6857`" + `.
  Its branch is ` + "`agent/issue-305-run-0e4ad40a7c3a6857`" + `, with no diff, commit,
  push, PR, check, evidence, challenge, or Examiner receipt.

## Risks

- The packet is unimplemented; no acceptance criterion has been verified.
- The claimed worktree contains no implementation changes but must be
  preserved for the next bounded selection/Smith attempt.
- No PR or review lifecycle has started, so there is no stale evidence to
  reuse.

## Next actions

Resume from the preserved claim/worktree with a fresh Smith implementation
agent, using the completed Scribe/Mason decisions above. Re-run focused tests,
` + "`go tool workflowctl check`" + `, and the full PR/evidence/Curator/Examiner workflow
only after implementation exists. Do not widen issue #305 or infer completion
from the clean worktree.
`

func TestClaimResumeAcceptsExactIssue305TerminalHandoff(t *testing.T) {
	if err := validateTerminalClaimHandoffBody(issue305TerminalHandoffBody, 305); err != nil {
		t.Fatalf("exact issue #305 terminal handoff: %v", err)
	}
	const (
		root        = "/home/paseouser/workspace/goxsd9-worktrees/issue-305-run-0e4ad40a7c3a6857"
		fixedBranch = "agent/issue-305"
		localBranch = "agent/issue-305-run-0e4ad40a7c3a6857"
		runID       = "run-0e4ad40a7c3a6857"
		head        = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	if err := validateClaimResumeHandoffBindings(issue305TerminalHandoffBody, 305, head, fixedBranch, localBranch, root, runID, time.Time{}); err != nil {
		t.Fatalf("exact issue #305 handoff bindings: %v", err)
	}
}

func TestClaimResumeExactIssue305DryRunHasZeroMutation(t *testing.T) {
	fixture := newClaimResumeIssueFixture(t, 305, "run-0e4ad40a7c3a6857", "")
	fixture.handoff = 5517524887
	fixture.handoffBody = strings.Replace(issue305TerminalHandoffBody,
		"/home/paseouser/workspace/goxsd9-worktrees/issue-305-run-0e4ad40a7c3a6857", fixture.worktree, 1)
	backend := newClaimResumeBackend(t, fixture)
	var output bytes.Buffer
	application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: &output}
	if err := application.run(claimResumeArgs(fixture, true)); err != nil {
		t.Fatalf("issue #305 claim resume dry-run: %v", err)
	}
	if backend.mutations != 0 {
		t.Fatalf("issue #305 dry-run mutations = %d, want zero", backend.mutations)
	}
	if !strings.Contains(output.String(), "handoff-comment 5517524887") {
		t.Fatalf("issue #305 dry-run output = %q", output.String())
	}
}

func TestClaimResumeTerminalEvidenceParser(t *testing.T) {
	worktree := "/worktrees/issue-14-run-proof"
	valid := fmt.Sprintf("## Blocker\n\nIssue #14 was claimed in %s.\n\n## Evidence\n\n- Issue #14 remained OPEN in the Project.\n- The claim worktree was clean at the final read.\n- No implementation, tests, commit, push, PR, or evaluation record was made.\n\n## Decisions and risks\n\n- Preserve the claim.\n\n## Next action\n\nResume after the blocker is cleared.\n", worktree)
	if err := validateTerminalClaimHandoffBody(valid, 14); err != nil {
		t.Fatalf("valid terminal evidence: %v", err)
	}
	workflowOnly := fmt.Sprintf("## Blocker\n\nIssue #14 was claimed successfully.\nNo source or test files were changed, no checks, push, PR, challenge, or evaluation record was made.\n\n## Evidence\n\n- Issue #14 remained OPEN in the Roadmap Project.\n- Claim worktree preserved at %s.\n- The claimed branch currently contains only the generated workflow claim\ncommit.\n\n## Risk and next action\n\nPreserve the branch and retry.\n", worktree)
	if err := validateTerminalClaimHandoffBody(workflowOnly, 14); err != nil {
		t.Fatalf("workflow-only terminal evidence: %v", err)
	}
	exact287 := strings.Join([]string{
		"## Blocker",
		"",
		"Issue #287 was claimed successfully, but the develop protocol is blocked at",
		"its mandatory fresh read-only Scribe/Mason consultation barrier. The Scribe",
		"and Mason role agents were spawned with the exact configured roles and",
		"read-only scope. Seven bounded waits returned `timed_out: true` with no final",
		"handoff or error status. One bounded follow-up to each existing agent was",
		"accepted, and the next bounded wait also timed out. No replacement agents",
		"were spawned and no scope was widened.",
		"",
		"Without those consultations, the root cannot assign Smith the required",
		"implementation contract under the develop protocol. No source or test files",
		"were changed, no checks, push, PR, challenge, or evaluation record was made.",
		"",
		"## Evidence",
		"",
		"- `go tool workflowctl doctor` passed: coordination `main` was clean and equal",
		"  to fetched `origin/main`, with recursive pins and required tools ready.",
		"- `go tool workflowctl pick` selected #287, “Model inline simple types on",
		"  global attributes”.",
		"- Claim worktree preserved at",
		"  `/home/paseouser/workspace/goxsd9-worktrees/issue-287-run-36de80f997095582`.",
		"- Scribe agent: `01a05b22-06da-7883-8245-16de2e761011`; Mason agent:",
		"  `01a05b22-0664-7881-aca3-3489d7ddc2f4`. Follow-up submissions were",
		"  `01a05b2a-8aa3-7cd3-921f-84566ec6d35a` and",
		"  `01a05b2a-8ab6-79c3-aa73-139fcbf7b0bd`.",
		"- Issue context and required `README.md`, `ARCHITECTURE.md`, `PLAN.md`, and",
		"  agent role configurations were read in the preserved worktree.",
		"",
		"## Risk and next action",
		"",
		"The claimed branch currently contains only the generated workflow claim",
		"commit. Proceeding without specification and architecture handoffs could",
		"mis-state XSD 1.0/1.1 behavior and violate the repository phase invariants.",
		"Keep the worktree and claim artifacts intact; resume issue #287 when the",
		"configured consultation agents can return their bounded handoffs.",
	}, "\n") + "\n"
	if err := validateTerminalClaimHandoffBody(exact287, 287); err != nil {
		t.Fatalf("exact #287 terminal evidence: %v", err)
	}
	malformed := []struct {
		name string
		body string
	}{
		{name: "missing final LF", body: strings.TrimSuffix(valid, "\n")},
		{name: "trailing space", body: strings.Replace(valid, "the Project.\n", "the Project. \n", 1)},
		{name: "wrong heading", body: strings.Replace(valid, "## Next action", "## Next steps", 1)},
		{name: "no PR proof", body: strings.Replace(valid, ", PR,", ", review,", 1)},
		{name: "unclean worktree", body: strings.Replace(valid, "worktree was clean", "worktree was unclean", 1)},
	}
	for _, test := range malformed {
		t.Run(test.name, func(t *testing.T) {
			if err := validateTerminalClaimHandoffBody(test.body, 14); err == nil {
				t.Fatal("malformed terminal evidence unexpectedly accepted")
			}
		})
	}
}

//nolint:funlen,gocognit // The integration cases cover each mutation boundary and its convergence response.
func TestClaimResumeInjectedIntegration(t *testing.T) {
	t.Run("dry run has zero mutation", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		var output bytes.Buffer
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: &output}
		if err := application.run(claimResumeArgs(fixture, true)); err != nil {
			t.Fatalf("claim resume dry-run: %v", err)
		}
		if backend.mutations != 0 {
			t.Fatalf("dry-run mutations = %d, want zero", backend.mutations)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
			t.Fatalf("dry-run moved local head from %s to %s", fixture.expected, got)
		}
		if !strings.Contains(output.String(), "no mutation performed") {
			t.Fatalf("dry-run output = %q", output.String())
		}
	})

	t.Run("renews and reconciles issue", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("claim resume: %v", err)
		}
		assertClaimResumeRenewed(t, fixture, backend)
	})

	t.Run("completed recovery is idempotent", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("first claim resume: %v", err)
		}
		head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
		mutations := backend.mutations
		pushes := countClaimResumePushes(backend.calls)
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("second claim resume: %v", err)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != head {
			t.Fatalf("idempotent rerun moved local head from %s to %s", head, got)
		}
		if backend.mutations != mutations || countClaimResumePushes(backend.calls) != pushes {
			t.Fatalf("idempotent rerun mutations/pushes = %d/%d, want %d/%d", backend.mutations, countClaimResumePushes(backend.calls), mutations, pushes)
		}
	})

	t.Run("ambiguous push response converges", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		backend.ambiguousPush = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("claim resume after ambiguous push: %v", err)
		}
		assertClaimResumeRenewed(t, fixture, backend)
		if got := countClaimResumePushes(backend.calls); got != 1 {
			t.Fatalf("push count = %d, want one", got)
		}
	})

	t.Run("ambiguous label and Project responses converge", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		backend.ambiguousLabel = true
		backend.ambiguousProject = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		if err := application.run(claimResumeArgs(fixture, false)); err != nil {
			t.Fatalf("claim resume after ambiguous GitHub responses: %v", err)
		}
		assertClaimResumeRenewed(t, fixture, backend)
	})

	t.Run("transient label failure remains retryable", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		labelErr := errors.New("simulated label transport failure")
		backend.labelFailure = labelErr
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		err := application.run(claimResumeArgs(fixture, false))
		if err == nil || operationDispositionOf(err) != operationDispositionRetryable {
			t.Fatalf("claim resume error = %v, want retryable failure", err)
		}
		if !errors.Is(err, labelErr) {
			t.Fatalf("claim resume error = %v, want original label failure", err)
		}
		if backend.needsHuman != true || backend.projectStatus != "Backlog" {
			t.Fatalf("state after retryable label failure = needs-human %t, Project %s", backend.needsHuman, backend.projectStatus)
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got == fixture.expected {
			t.Fatal("retryable label failure lost the verified renewal")
		}
	})

	t.Run("fresh proof transport failure remains retryable with cause", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		sentinel := errors.New("fresh proof transport failure")
		backend.freshProofFailure = sentinel
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		err := application.run(claimResumeArgs(fixture, false))
		if err == nil || operationDispositionOf(err) != operationDispositionRetryable || !errors.Is(err, sentinel) {
			t.Fatalf("fresh proof error = %v, disposition %d, want retryable cause", err, operationDispositionOf(err))
		}
		if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
			t.Fatalf("fresh proof failure moved local head to %s", got)
		}
		if backend.mutations != 0 {
			t.Fatalf("fresh proof failure mutations = %d, want zero", backend.mutations)
		}
	})

	t.Run("malformed successful issue API response is terminal", func(t *testing.T) {
		fixture := newClaimResumeFixture(t)
		backend := newClaimResumeBackend(t, fixture)
		backend.malformedIssue = true
		application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
		err := application.run(claimResumeArgs(fixture, true))
		if err == nil || operationDispositionOf(err) != operationDispositionTerminal {
			t.Fatalf("malformed issue response error = %v, disposition %d, want terminal", err, operationDispositionOf(err))
		}
		if backend.mutations != 0 {
			t.Fatalf("malformed issue response mutations = %d, want zero", backend.mutations)
		}
	})

	for _, test := range []struct {
		name string
		set  func(*claimResumeBackend)
		want string
	}{
		{name: "open PR race", set: func(backend *claimResumeBackend) { backend.raceOpenPR = true }, want: "open PR"},
		{name: "Project identity race", set: func(backend *claimResumeBackend) { backend.raceProjectPicked = true }, want: "OPEN+needs-human"},
	} {
		t.Run(test.name+" before first GitHub mutation", func(t *testing.T) {
			fixture := newClaimResumeFixture(t)
			backend := newClaimResumeBackend(t, fixture)
			test.set(backend)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(claimResumeArgs(fixture, false))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("race error = %v, want %q", err, test.want)
			}
			if mutations := claimResumeGitHubMutations(backend.calls); len(mutations) != 0 {
				t.Fatalf("race GitHub mutations = %v, want zero", mutations)
			}
		})
	}
}

func TestClaimResumeRejectsWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, *claimResumeFixture, *claimResumeBackend)
		want   string
		status bool
	}{
		{name: "dirty worktree", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			writeFixtureFile(t, fixture.worktree, "dirty", "keep\n")
		}, want: "worktree", status: true},
		{name: "missing terminal evidence", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.comments = backend.comments[:1]
		}, want: "terminal handoff", status: true},
		{name: "untrusted terminal evidence", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.comments[1].User.Login = owner
		}, want: "trusted API actor", status: true},
		{name: "locked worktree", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			runGitTest(t, fixture.primary, "worktree", "lock", fixture.worktree)
		}, want: "current worktree registration", status: true},
		{name: "duplicate worktree", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			duplicate := filepath.Join(t.TempDir(), "issue-14-run-other")
			runGitTest(t, fixture.primary, "worktree", "add", "-b", "agent/issue-14-run-other", duplicate, fixture.expected)
		}, want: "duplicate/orphan", status: true},
		{name: "malformed remote ref listing", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.malformedRemoteRefs = true
		}, want: "malformed entry", status: true},
		{name: "open PR", setup: func(_ *testing.T, _ *claimResumeFixture, backend *claimResumeBackend) {
			backend.openPR = true
		}, want: "open PR", status: true},
		{name: "moved fixed ref", setup: func(t *testing.T, fixture *claimResumeFixture, _ *claimResumeBackend) {
			moved := createResumeTestCommit(t, fixture.primary, fixture.expected, "moved artifact")
			runGitTest(t, fixture.primary, "push", "--force", "origin", moved+":refs/heads/agent/issue-14")
		}, want: "renewal child", status: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newClaimResumeFixture(t)
			backend := newClaimResumeBackend(t, fixture)
			test.setup(t, &fixture, backend)
			before := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(claimResumeArgs(fixture, true))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("claim resume error = %v, want %q", err, test.want)
			}
			if backend.mutations != 0 {
				t.Fatalf("refusal mutations = %d, want zero", backend.mutations)
			}
			if after := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); after != before {
				t.Fatalf("refusal moved local head from %s to %s", before, after)
			}
			if got := backend.needsHuman; got != test.status {
				t.Fatalf("needs-human = %t, want %t", got, test.status)
			}
		})
	}
}

func TestClaimResumeRejectsMalformedAdvertisedRunLocalBeforeMutation(t *testing.T) {
	fixture := newClaimResumeFixture(t)
	backend := newClaimResumeBackend(t, fixture)
	localBefore := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	remoteBefore := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14")
	application := app{ctx: context.Background(), executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if command == "git ls-remote --heads origin refs/heads/agent/*" {
			backend.calls = append(backend.calls, command)
			fixed := strings.Fields(remoteBefore)[0]
			return fixed + " refs/heads/agent/issue-14\nshort refs/heads/agent/issue-14-run-resume-test", nil
		}
		return backend.execute(dir, input, name, args...)
	}, stdout: io.Discard}
	err := application.run(claimResumeArgs(fixture, true))
	if err == nil || operationDispositionOf(err) != operationDispositionTerminal || !strings.Contains(err.Error(), "malformed object name") {
		t.Fatalf("malformed advertised run-local error = %v, disposition %d, want terminal malformed refusal", err, operationDispositionOf(err))
	}
	if backend.mutations != 0 {
		t.Fatalf("malformed advertised run-local mutations = %d, want zero", backend.mutations)
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != localBefore {
		t.Fatalf("malformed advertised run-local moved local head from %s to %s", localBefore, got)
	}
	if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14"); got != remoteBefore {
		t.Fatalf("malformed advertised run-local moved remote head from %q to %q", remoteBefore, got)
	}
}

func TestClaimResumeRequiresAcknowledgementAndExactFlags(t *testing.T) {
	fixture := newClaimResumeFixture(t)
	for _, args := range [][]string{
		{"claim", "resume", "14", "--expected-head", fixture.expected, "--run-id", fixture.runID, "--handoff-comment", "2"},
		{"claim", "resume", "14", "--expected-head", "short", "--run-id", fixture.runID, "--handoff-comment", "2", "--acknowledge-needs-human"},
		{"claim", "resume", "14", "--expected-head", fixture.expected, "--run-id", fixture.runID, "--handoff-comment", "2x", "--acknowledge-needs-human"},
	} {
		if err := (app{ctx: context.Background(), stdout: io.Discard}).run(args); err == nil {
			t.Fatalf("run(%q) unexpectedly succeeded", args)
		}
	}
}

type claimResumeFixture struct {
	baseRepositoryFixture
	issue       int
	expected    string
	runID       string
	worktree    string
	handoff     int64
	lease       time.Time
	handoffBody string
}

func newClaimResumeFixture(t *testing.T) claimResumeFixture {
	return newClaimResumeIssueFixture(t, 14, "run-resume-test", "")
}

func newClaimResumeIssueFixture(t *testing.T, issue int, runID, handoffBody string) claimResumeFixture {
	t.Helper()
	base := newBaseRepositoryFixture(t, false)
	lease := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	parent := runGitTest(t, base.primary, "rev-parse", "HEAD")
	expected := createResumeTestCommit(t, base.primary, parent, claimMessage(issue, runID, lease))
	runGitTest(t, base.primary, "push", "origin", expected+":refs/heads/"+claimBranch(issue))
	worktree := filepath.Join(t.TempDir(), fmt.Sprintf("issue-%d-%s", issue, runID))
	runGitTest(t, base.primary, "worktree", "add", "-b", claimLocalBranch(issue, runID), worktree, expected)
	return claimResumeFixture{baseRepositoryFixture: base, issue: issue, expected: expected, runID: runID, worktree: worktree, handoff: 2, lease: lease, handoffBody: handoffBody}
}

func claimResumeArgs(fixture claimResumeFixture, dryRun bool) []string {
	args := []string{"claim", "resume", strconv.Itoa(fixture.issue), "--expected-head", fixture.expected, "--run-id", fixture.runID,
		"--handoff-comment", strconv.FormatInt(fixture.handoff, 10), "--acknowledge-needs-human"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	return args
}

type claimResumeBackend struct {
	t                   *testing.T
	fixture             claimResumeFixture
	comments            []issueCommentAPI
	needsHuman          bool
	projectStatus       string
	openPR              bool
	ambiguousPush       bool
	ambiguousLabel      bool
	ambiguousProject    bool
	malformedRemoteRefs bool
	malformedIssue      bool
	freshProofFailure   error
	raceOpenPR          bool
	raceProjectPicked   bool
	labelFailure        error
	mutations           int
	calls               []string
}

func newClaimResumeBackend(t *testing.T, fixture claimResumeFixture) *claimResumeBackend {
	t.Helper()
	issue := fixture.issue
	lease := fixture.lease
	claim := issueCommentAPI{ID: 1, Body: fmt.Sprintf("Claim acquired.\n\n- Branch: `%s`\n- Local branch: `%s`\n- Worktree: `%s`\n- Run: `%s`\n- Lease until: `%s`\n",
		claimBranch(issue), claimLocalBranch(issue, fixture.runID), fixture.worktree, fixture.runID, lease.Format(time.RFC3339))}
	claim.User.Login = trustedActor
	claim.CreatedAt = lease.Add(-time.Minute)
	terminalBody := fixture.handoffBody
	if terminalBody == "" {
		terminalBody = fmt.Sprintf("## Blocker\n\nIssue #%d was claimed in %s.\n\n## Evidence\n\n- Issue #%d remained OPEN in the Roadmap Project.\n- The claim worktree was clean at the final read: %s.\n- No implementation, tests, documentation, commit, push, PR, or evaluation record was made.\n\n## Decisions and risks\n\n- The generated claim is preserved.\n\n## Next action\n\nResume the issue after the blocker is cleared.\n", issue, fixture.worktree, issue, fixture.worktree)
	}
	terminal := issueCommentAPI{ID: fixture.handoff, Body: terminalBody}
	terminal.User.Login = trustedActor
	terminal.CreatedAt = lease
	return &claimResumeBackend{t: t, fixture: fixture, comments: []issueCommentAPI{claim, terminal}, needsHuman: true, projectStatus: "Backlog"}
}

func (b *claimResumeBackend) execute(dir string, input io.Reader, name string, args ...string) (string, error) {
	if dir == "" {
		dir = b.fixture.worktree
	}
	call := name + " " + strings.Join(args, " ")
	b.calls = append(b.calls, call)
	if name == "gh" {
		return b.executeGH(args...)
	}
	if b.malformedRemoteRefs && name == "git" && len(args) > 0 && args[0] == "ls-remote" {
		return "not-a-ref", nil
	}
	if name == "git" && len(args) > 0 && (args[0] == "commit-tree" || args[0] == "update-ref" || args[0] == "push") {
		b.mutations++
	}
	// #nosec G204 -- the injected executor runs only workflowctl-generated test commands.
	command := exec.CommandContext(context.Background(), name, args...)
	command.Dir = dir
	command.Stdin = input
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s: %w: %s", call, err, strings.TrimSpace(string(output)))
	}
	if b.ambiguousPush && name == "git" && len(args) > 0 && args[0] == "push" {
		b.ambiguousPush = false
		return "", errors.New("simulated lost push response")
	}
	if name == "git" && len(args) > 0 && (args[0] == "cat-file" ||
		(args[0] == "rev-parse" && len(args) > 1 && strings.HasSuffix(args[1], "^{tree}")) ||
		(args[0] == "log" && len(args) > 2 && args[1] == "-1" && args[2] == "--format=%B")) {
		return string(output), nil
	}
	return strings.TrimSpace(string(output)), nil
}

//nolint:gocognit // The injected GitHub backend models each mutation boundary and response race.
func (b *claimResumeBackend) executeGH(args ...string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case joined == fmt.Sprintf("api repos/goxdra/goxsd9/issues/%d", b.fixture.issue):
		if b.malformedIssue {
			return "{", nil
		}
		if b.freshProofFailure != nil && b.issueStatusReads() == 2 {
			err := b.freshProofFailure
			b.freshProofFailure = nil
			return "", err
		}
		if b.raceOpenPR && b.issueStatusReads() == 3 {
			b.openPR = true
		}
		labels := "[]"
		if b.needsHuman {
			labels = `[{"name":"needs-human"}]`
		}
		return `{"state":"open","labels":` + labels + `}`, nil
	case joined == fmt.Sprintf("api --paginate repos/goxdra/goxsd9/issues/%d/comments?per_page=100", b.fixture.issue):
		data, err := json.Marshal(b.comments)
		return string(data), err
	case joined == fmt.Sprintf("pr list --repo goxdra/goxsd9 --head %s --state open --json number", claimBranch(b.fixture.issue)):
		if b.openPR {
			return `[{"number":55}]`, nil
		}
		return `[]`, nil
	case strings.HasPrefix(joined, fmt.Sprintf("issue edit %d ", b.fixture.issue)):
		b.mutations++
		if b.labelFailure != nil {
			err := b.labelFailure
			b.labelFailure = nil
			return "", err
		}
		b.needsHuman = false
		if b.ambiguousLabel {
			b.ambiguousLabel = false
			return "", errors.New("simulated lost label response")
		}
		return "", nil
	case strings.HasPrefix(joined, "project item-list "):
		if b.raceProjectPicked && b.projectItemReads() == 3 {
			b.projectStatus = "Picked"
		}
		return fmt.Sprintf(`{"items":[{"id":"item-%d","status":%q,"content":{"number":%d,"repository":"goxdra/goxsd9","type":"Issue"}}],"totalCount":1}`, b.fixture.issue, b.projectStatus, b.fixture.issue), nil
	case strings.HasPrefix(joined, "project field-list "):
		return `{"fields":[{"id":"status-field","name":"Status","options":[{"id":"backlog-id","name":"Backlog"},{"id":"picked-id","name":"Picked"}]}]}`, nil
	case strings.Contains(joined, "project item-edit"):
		b.mutations++
		b.projectStatus = "Picked"
		if b.ambiguousProject {
			b.ambiguousProject = false
			return "", errors.New("simulated lost Project response")
		}
		return "", nil
	default:
		return "", fmt.Errorf("unexpected gh command: %s", joined)
	}
}

func (b *claimResumeBackend) issueStatusReads() int {
	reads := 0
	for _, call := range b.calls {
		if call == fmt.Sprintf("gh api repos/goxdra/goxsd9/issues/%d", b.fixture.issue) {
			reads++
		}
	}
	return reads
}

func (b *claimResumeBackend) projectItemReads() int {
	reads := 0
	for _, call := range b.calls {
		if strings.HasPrefix(call, "gh project item-list ") {
			reads++
		}
	}
	return reads
}

func claimResumeGitHubMutations(calls []string) []string {
	mutations := make([]string, 0, 2)
	for _, call := range calls {
		if strings.HasPrefix(call, "gh issue edit ") || strings.Contains(call, "gh project item-edit") {
			mutations = append(mutations, call)
		}
	}
	return mutations
}

func assertClaimResumeRenewed(t *testing.T, fixture claimResumeFixture, backend *claimResumeBackend) {
	t.Helper()
	head := runGitTest(t, fixture.worktree, "rev-parse", "HEAD")
	if head == fixture.expected {
		t.Fatal("claim resume did not create a renewal child")
	}
	if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^"); got != fixture.expected {
		t.Fatalf("renewal parent = %s, want %s", got, fixture.expected)
	}
	if got, want := runGitTest(t, fixture.worktree, "rev-parse", "HEAD^{tree}"), runGitTest(t, fixture.worktree, "rev-parse", "HEAD^^{tree}"); got != want {
		t.Fatalf("renewal tree = %s, want %s", got, want)
	}
	if got := runGitTest(t, fixture.primary, "ls-remote", "origin", "refs/heads/agent/issue-14"); !strings.HasPrefix(got, head+"\t") {
		t.Fatalf("remote fixed branch = %q, want %s", got, head)
	}
	if backend.needsHuman || backend.projectStatus != "Picked" {
		t.Fatalf("reconciled state = needs-human %t, Project %s", backend.needsHuman, backend.projectStatus)
	}
}

func countClaimResumePushes(calls []string) int {
	count := 0
	for _, call := range calls {
		if strings.HasPrefix(call, "git push ") {
			count++
		}
	}
	return count
}

func TestClaimResumeMetadataAndClaimCommentStayExact(t *testing.T) {
	lease := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	comment, err := parseClaimAcquiredComment(fmt.Sprintf("Claim acquired.\n\n- Branch: `agent/issue-14`\n- Local branch: `agent/issue-14-run-proof`\n- Worktree: `/tmp/proof`\n- Run: `run-proof`\n- Lease until: `%s`\n", lease.Format(time.RFC3339)))
	if err != nil {
		t.Fatalf("parse generated claim comment: %v", err)
	}
	if comment.runID != "run-proof" || !comment.lease.Equal(lease) {
		t.Fatalf("parsed claim comment = %#v", comment)
	}
	if _, err := parseClaimAcquiredComment(strings.Replace(commentBodyForTest(lease), "Claim acquired.", "Claim acquired", 1)); err == nil {
		t.Fatal("malformed generated claim comment unexpectedly accepted")
	}
}

//nolint:gocognit // Table cases keep all canonical object rejection proofs together.
func TestCanonicalClaimCommitRejectsNonCanonicalHistoryAndMessage(t *testing.T) {
	const (
		head       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		parent     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		tree       = "cccccccccccccccccccccccccccccccccccccccc"
		parentTree = "cccccccccccccccccccccccccccccccccccccccc"
	)
	lease := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	message := claimMessage(14, "run-proof", lease)
	object := fmt.Sprintf("tree %s\nparent %s\nauthor Smith <smith@example.invalid> 0 +0000\ncommitter Smith <smith@example.invalid> 0 +0000\n\n%s", tree, parent, message)

	tests := []struct {
		name       string
		object     string
		claimTree  string
		parentTree string
		wantError  string
	}{
		{name: "valid", object: object, claimTree: tree + "\n", parentTree: parentTree + "\n"},
		{name: "source-bearing", object: strings.Replace(object, "tree "+tree+"\n", "tree dddddddddddddddddddddddddddddddddddddddd\n", 1), claimTree: "dddddddddddddddddddddddddddddddddddddddd\n", parentTree: parentTree + "\n", wantError: "source-bearing"},
		{name: "merge", object: strings.Replace(object, "parent "+parent+"\n", "parent "+parent+"\nparent "+strings.Repeat("e", 40)+"\n", 1), claimTree: tree + "\n", parentTree: parentTree + "\n", wantError: "exactly one parent"},
		{name: "missing final LF", object: strings.TrimSuffix(object, "\n"), claimTree: tree + "\n", parentTree: parentTree + "\n", wantError: "LF-terminated"},
		{name: "extra final LF", object: object + "\n", claimTree: tree + "\n", parentTree: parentTree + "\n", wantError: "LF-terminated"},
		{name: "trailing message byte", object: strings.Replace(object, "Agent-Issue: 14\n", "Agent-Issue: 14 \n", 1), claimTree: tree + "\n", parentTree: parentTree + "\n", wantError: "non-canonical metadata"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := app{executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
				if name != "git" {
					return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
				}
				switch strings.Join(args, " ") {
				case "cat-file commit " + head:
					return test.object, nil
				case "cat-file --batch-check=%(objectname) %(objecttype)":
					value, err := io.ReadAll(input)
					if err != nil {
						return "", fmt.Errorf("read object query: %w", err)
					}
					return strings.TrimSpace(string(value)) + " commit", nil
				case "rev-parse " + head + "^{tree}":
					return test.claimTree, nil
				case "rev-parse " + parent + "^{tree}":
					return test.parentTree, nil
				default:
					return "", fmt.Errorf("unexpected command: git %s", strings.Join(args, " "))
				}
			}}
			_, err := application.readCanonicalClaimCommit("/repo", head, 14, "run-proof", parent)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("readCanonicalClaimCommit: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("readCanonicalClaimCommit error = %v, want %q", err, test.wantError)
			}
			classified := retryableOperationIfRecoverable("canonical claim test", err)
			if operationDispositionOf(classified) != operationDispositionTerminal {
				t.Fatalf("classified error disposition = %d, want terminal", operationDispositionOf(classified))
			}
		})
	}
}

func TestClaimResumeHandoffBindingsRejectSpoofedTokens(t *testing.T) {
	const (
		issue       = 14
		root        = "/worktrees/issue-14-run-proof"
		fixedBranch = "agent/issue-14"
		localBranch = "agent/issue-14-run-proof"
		runID       = "run-proof"
		head        = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	lease := time.Date(2026, time.August, 15, 6, 0, 0, 0, time.UTC)
	valid := fmt.Sprintf("## Blocker\n\nIssue #14 was claimed.\n\n## Evidence\n\n- The worktree was clean and preserved at `%s`.\n- No source or implementation changed and no PR was opened.\n\n## Decisions and risks\n\n- Preserve the claim.\n\n## Next action\n\nResume after the blocker is cleared.\n", root)
	if err := validateClaimResumeHandoffBindings(valid, issue, head, fixedBranch, localBranch, root, runID, lease); err != nil {
		t.Fatalf("valid handoff bindings: %v", err)
	}
	tests := []struct {
		name string
		text string
	}{
		{name: "issue suffix", text: "\nRelated issue #140 was mentioned."},
		{name: "standalone wrong issue", text: "\nRelated #15 was mentioned."},
		{name: "wrong path", text: "\nA second path `/other/worktree` was recorded."},
		{name: "wrong branch", text: "\nBranch `agent/issue-15` was used."},
		{name: "wrong local branch", text: "\nLocal branch `agent/issue-14-run-other` was used."},
		{name: "wrong run", text: "\nRun `run-other` was used."},
		{name: "wrong lease", text: "\nLease until `2026-08-15T07:00:00Z`."},
		{name: "malformed lease", text: "\nLease until `not-a-time`."},
		{name: "wrong head", text: "\nHead `bbbbbbb` was recorded."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateClaimResumeHandoffBindings(valid+test.text, issue, head, fixedBranch, localBranch, root, runID, lease); err == nil {
				t.Fatal("spoofed handoff bindings unexpectedly accepted")
			}
		})
	}
	if err := validateTerminalClaimHandoffBody(strings.Replace(valid, "No source or implementation changed", "No source or implementation changed; a PR exists", 1), issue); err == nil {
		t.Fatal("contradictory PR handoff unexpectedly accepted")
	}
}

func TestClaimResumeRetryPreservesOperationBoundaryAndCause(t *testing.T) {
	proof := claimResumeProof{preflight: claimResumePreflight{
		issue: 14, expectedHead: strings.Repeat("a", 40), runID: "run-proof", handoffCommentID: 9,
	}}
	sentinel := errors.New("fresh proof transport")
	retryable := retryableOperation("fresh proof", sentinel)
	err := claimResumeProofFailure(proof, "fresh proof failed", retryable)
	if operationDispositionOf(err) != operationDispositionRetryable || !errors.Is(err, sentinel) {
		t.Fatalf("retryable proof error = %v, disposition %d, want cause and retryable", err, operationDispositionOf(err))
	}
	terminal := terminalOperation("fresh proof", stateError("untrusted response"))
	err = claimResumeProofFailure(proof, "fresh proof failed", terminal)
	if operationDispositionOf(err) != operationDispositionTerminal {
		t.Fatalf("terminal proof error = %v, disposition %d, want terminal", err, operationDispositionOf(err))
	}
}

func TestValidateResumeWorktreeRejectsDetachedDuplicateLineage(t *testing.T) {
	const (
		root = "/worktrees/issue-55-run-good"
		head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	layout := repositoryLayout{worktrees: []gitWorktree{
		{path: root, branch: "refs/heads/agent/issue-55-run-good", head: head},
		{path: "/worktrees/detached", head: head},
	}}
	err := validateResumeWorktree(layout, root, "agent/issue-55-run-good", 55, head)
	if err == nil || !strings.Contains(err.Error(), "detached duplicate/orphan") {
		t.Fatalf("validateResumeWorktree error = %v, want detached duplicate refusal", err)
	}
}

//nolint:gocognit // The table covers detached and locked renewal-head artifacts at one mutation boundary.
func TestClaimResumeRejectsRenewalHeadWorktreeBeforeMutation(t *testing.T) {
	for _, locked := range []bool{false, true} {
		name := "detached renewal head"
		if locked {
			name = "locked renewal head"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newClaimResumeFixture(t)
			lease := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
			renewal := createResumeTestCommit(t, fixture.primary, fixture.expected, claimMessage(14, fixture.runID, lease))
			runGitTest(t, fixture.primary, "push", "--force", "origin", renewal+":refs/heads/agent/issue-14")
			duplicate := filepath.Join(t.TempDir(), "renewal-head")
			runGitTest(t, fixture.primary, "worktree", "add", "--detach", duplicate, renewal)
			if locked {
				runGitTest(t, fixture.primary, "worktree", "lock", duplicate)
			}
			backend := newClaimResumeBackend(t, fixture)
			application := app{ctx: context.Background(), executeCommand: backend.execute, stdout: io.Discard}
			err := application.run(claimResumeArgs(fixture, true))
			if err == nil || !strings.Contains(err.Error(), "detached duplicate/orphan") {
				t.Fatalf("renewal-head worktree error = %v, want detached duplicate refusal", err)
			}
			if backend.mutations != 0 {
				t.Fatalf("renewal-head worktree mutations = %d, want zero", backend.mutations)
			}
			if got := runGitTest(t, fixture.worktree, "rev-parse", "HEAD"); got != fixture.expected {
				t.Fatalf("renewal-head worktree refusal moved local head from %s to %s", fixture.expected, got)
			}
			if got := runGitTest(t, fixture.primary, "ls-remote", "--heads", "origin", "refs/heads/agent/issue-14"); !strings.HasPrefix(got, renewal+"\t") {
				t.Fatalf("renewal-head worktree refusal moved remote head: %q", got)
			}
		})
	}
}

func commentBodyForTest(lease time.Time) string {
	return fmt.Sprintf("Claim acquired.\n\n- Branch: `agent/issue-14`\n- Local branch: `agent/issue-14-run-proof`\n- Worktree: `/tmp/proof`\n- Run: `run-proof`\n- Lease until: `%s`\n", lease.Format(time.RFC3339))
}
