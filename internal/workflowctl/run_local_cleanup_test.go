package workflowctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRemoteAgentRefInventorySeparatesAndSortsRefs(t *testing.T) {
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "git" || strings.Join(args, " ") != "ls-remote --heads origin refs/heads/agent/*" {
			return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
		return "sha-z refs/heads/agent/issue-12-run-z\n" +
			"sha-a refs/heads/agent/archive/issue-12-old\n" +
			"sha-fixed refs/heads/agent/issue-12\n" +
			"sha-b refs/heads/agent/issue-12-run-a\n" +
			"sha-leading refs/heads/agent/issue-01\n" +
			"sha-leading-run refs/heads/agent/issue-01-run-good\n" +
			"sha-empty-suffix refs/heads/agent/issue-12-\n" +
			"sha-bad refs/heads/agent/issue-12-old\n" +
			"sha-unrelated refs/heads/agent/other\n" +
			"sha-malformed refs/heads/agent/issue-no\n", nil
	}}
	inventory, err := application.remoteAgentRefInventory("/repo")
	if err != nil {
		t.Fatalf("remoteAgentRefInventory: %v", err)
	}
	if len(inventory.claims) != 1 || inventory.claims[0].branch != "agent/issue-12" {
		t.Fatalf("fixed claims = %#v, want only fixed claim", inventory.claims)
	}
	if len(inventory.runLocals) != 2 || inventory.runLocals[0].branch != "agent/issue-12-run-a" || inventory.runLocals[1].branch != "agent/issue-12-run-z" {
		t.Fatalf("run-local refs = %#v, want deterministic run-a/run-z order", inventory.runLocals)
	}
	if len(inventory.archives) != 1 || inventory.archives[0].branch != "agent/archive/issue-12-old" {
		t.Fatalf("archives = %#v, want historical archive only", inventory.archives)
	}
	if len(inventory.malformed) != 5 || inventory.malformed[0].branch != "agent/issue-01" || inventory.malformed[1].branch != "agent/issue-01-run-good" || inventory.malformed[2].branch != "agent/issue-12-" || inventory.malformed[3].branch != "agent/issue-12-old" || inventory.malformed[4].branch != "agent/issue-no" {
		t.Fatalf("malformed refs = %#v, want deterministic malformed order", inventory.malformed)
	}
	if len(inventory.unrelated) != 1 || inventory.unrelated[0].branch != "agent/other" {
		t.Fatalf("unrelated refs = %#v, want unrelated ref only", inventory.unrelated)
	}
}

func TestClassifyAgentRefRequiresCanonicalFixedAndRunLocalGrammar(t *testing.T) {
	tests := []struct {
		branch string
		kind   agentRefKind
		number int
		runID  string
	}{
		{branch: "agent/issue-12", kind: agentRefClaim, number: 12},
		{branch: "agent/issue-12-run-good", kind: agentRefRunLocal, number: 12, runID: "run-good"},
		{branch: "agent/issue-01", kind: agentRefMalformed},
		{branch: "agent/issue-12-", kind: agentRefMalformed, number: 12},
		{branch: "agent/issue-12-run-", kind: agentRefMalformed, number: 12},
		{branch: "agent/issue-12-run-a--b", kind: agentRefMalformed, number: 12},
		{branch: "agent/archive/issue-12-old", kind: agentRefArchive},
		{branch: "agent/other", kind: agentRefUnrelated},
	}
	for _, test := range tests {
		kind, number, runID := classifyAgentRef(test.branch)
		if kind != test.kind || number != test.number || runID != test.runID {
			t.Errorf("classifyAgentRef(%q) = (%d, %d, %q), want (%d, %d, %q)", test.branch, kind, number, runID, test.kind, test.number, test.runID)
		}
	}
}

func TestSyncAndPickIgnoreRunLocalRefs(t *testing.T) {
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git ls-remote --heads origin refs/heads/agent/*":
			return "head refs/heads/agent/issue-55-run-test", nil
		case "gh api repos/goxdra/goxsd9/issues/55":
			return `{"state":"OPEN","labels":[]}`, nil
		case "gh project item-list 1 --owner goxdra --format json --limit 500":
			return `{"items":[{"content":{"number":55,"repository":"goxdra/goxsd9","type":"Issue","url":"https://example.test/55"},"status":"Ready","title":"ready"}]}`, nil
		case "gh api graphql -f query=" + issueRelationsQuery + " -f owner=goxdra -f repository=goxsd9 -F number=55":
			return `{"data":{"repository":{"issue":{"createdAt":"2026-08-19T00:00:00Z","blockedBy":{"nodes":[]},"blocking":{"nodes":[]}}}}}`, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	claims, err := application.remoteClaims("/repo")
	if err != nil {
		t.Fatalf("remoteClaims: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("run-local refs became active claims: %#v", claims)
	}
	item := projectItem{Content: projectContent{Number: 55, Repository: repositoryKey, Type: "Issue"}, Status: "Picked"}
	desired, err := application.desiredStatus("/repo", item, claims)
	if err != nil {
		t.Fatalf("desiredStatus: %v", err)
	}
	if desired != "Ready" {
		t.Fatalf("desired status = %q, want Ready when only run-local ref exists", desired)
	}
	candidates, err := application.pickCandidates("/repo")
	if err != nil {
		t.Fatalf("pickCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Number != 55 {
		t.Fatalf("pick candidates = %#v, want Ready issue #55", candidates)
	}
}

func TestRunLocalCleanupAcceptanceMatrixPreservesBeforeMutation(t *testing.T) {
	const (
		issue       = 55
		fixedBranch = "agent/issue-55"
		goodBranch  = "agent/issue-55-run-good"
		goodSHA     = "evaluated-head"
	)
	matrix := []struct {
		name        string
		inventory   string
		currentSHA  string
		openPR      bool
		localBranch string
		want        string
	}{
		{name: "moved", inventory: goodSHA + " refs/heads/" + goodBranch, currentSHA: "moved-head", want: "found moved"},
		{name: "open PR", inventory: goodSHA + " refs/heads/" + goodBranch, currentSHA: goodSHA, openPR: true, want: "open PR"},
		{name: "wrong run", inventory: goodSHA + " refs/heads/agent/issue-55-run-other", currentSHA: goodSHA, want: "no canonical claim marker"},
		{name: "wrong SHA", inventory: "unrelated-head refs/heads/" + goodBranch, currentSHA: "unrelated-head", want: "immutable claim head"},
		{name: "malformed", inventory: "bad refs/heads/agent/issue-55-run-", currentSHA: "bad", want: "malformed run-local"},
		{name: "ambiguous", inventory: goodSHA + " refs/heads/agent/issue-55-run-a\n" + goodSHA + " refs/heads/agent/issue-55-run-b", currentSHA: goodSHA, want: "ambiguous run-local"},
		{name: "unrelated commit", inventory: "unrelated-head refs/heads/" + goodBranch, currentSHA: "unrelated-head", want: "immutable claim head"},
	}
	for _, test := range matrix {
		t.Run(test.name, func(t *testing.T) {
			commands := []string{}
			application := scriptedRunLocalApplication(test.inventory, test.currentSHA, test.openPR, &commands)
			packet := mergedPacket{
				number:   issue,
				mergeSHA: "merge-proof",
				plan: cleanupPlan{
					claims:            []claimArtifact{{issue: issue, branch: fixedBranch, sha: goodSHA, localBranch: test.localBranch}},
					proofHead:         goodSHA,
					primaryIssue:      issue,
					validateArtifacts: true,
				},
			}
			_, err := application.prepareRunLocalCleanup("/repo", packet)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepareRunLocalCleanup error = %v, want %q", err, test.want)
			}
			if containsRunLocalDelete(commands) {
				t.Fatalf("validation attempted deletion commands: %v", commands)
			}
		})
	}
}

func TestRunLocalCleanupIgnoresUnrelatedDivergenceAndMalformedRefs(t *testing.T) {
	const (
		goodSHA    = "evaluated-head"
		goodBranch = "agent/issue-55-run-good"
	)
	commands := []string{}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch {
		case command == "git ls-remote --heads origin refs/heads/agent/*":
			return goodSHA + " refs/heads/" + goodBranch + "\n" +
				"remote-unrelated refs/heads/agent/issue-56-run-other\n" +
				"bad-remote refs/heads/agent/issue-56-run-", nil
		case command == "git for-each-ref --format=%(refname:short) %(objectname) refs/heads/agent/issue-*":
			return "local-unrelated refs/heads/agent/issue-56-run-other\n" +
				"bad-local refs/heads/agent/issue-56-run-", nil
		case command == "git for-each-ref --format=%(refname:short) %(objectname) refs/remotes/origin/agent/issue-*":
			return "tracking-unrelated origin/agent/issue-56-run-other\n" +
				"bad-tracking origin/agent/issue-56-run-", nil
		case strings.HasPrefix(command, "git log --format=%H%x00%B%x00 "):
			return "evaluated-commit\x00" + claimMessage(55, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00", nil
		case command == "git ls-remote --heads origin refs/heads/"+goodBranch:
			return goodSHA + " refs/heads/" + goodBranch, nil
		case strings.HasPrefix(command, "gh pr list "):
			return "[]", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	packet := mergedPacket{plan: cleanupPlan{
		claims:            []claimArtifact{{issue: 55, branch: "agent/issue-55", sha: goodSHA}},
		proofHead:         goodSHA,
		validateArtifacts: true,
	}}
	refs, err := application.prepareRunLocalCleanup("/repo", packet)
	if err != nil {
		t.Fatalf("prepareRunLocalCleanup: %v", err)
	}
	if len(refs) != 1 || refs[0].branch != goodBranch || refs[0].sha != goodSHA {
		t.Fatalf("proven run-local refs = %#v, want only %s at %s", refs, goodBranch, goodSHA)
	}
	for _, command := range commands {
		if !strings.Contains(command, "agent/issue-56-run-other") {
			continue
		}
		if strings.Contains(command, "push --force-with-lease") || strings.Contains(command, "update-ref") || strings.Contains(command, "worktree remove") {
			t.Fatalf("unrelated ref became a mutation target: %v", commands)
		}
	}
}

func TestRunLocalCandidatePreservesOpenPRWithoutRemoteRef(t *testing.T) {
	const (
		branch = "agent/issue-55-run-good"
		sha    = "evaluated-head"
	)
	for _, test := range []struct {
		name     string
		local    bool
		tracking bool
	}{
		{name: "local-only", local: true},
		{name: "tracking-only", tracking: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			commands := []string{}
			application := app{executeCommand: openPRRunLocalCandidateExecutor(test.local, test.tracking, &commands)}
			candidate := runLocalRefCandidate{branch: branch, sha: sha, localPresent: test.local, trackingPresent: test.tracking}
			packet := mergedPacket{plan: cleanupPlan{
				claims:            []claimArtifact{{issue: 55, branch: "agent/issue-55", sha: sha}},
				proofHead:         sha,
				validateArtifacts: true,
			}}
			_, found, err := application.validateRunLocalCandidate("/repo", packet, candidate)
			if err == nil || !strings.Contains(err.Error(), "open PR") {
				t.Fatalf("open PR validation = (%t, %v), want preservation refusal", found, err)
			}
			if !containsCommand(commands, "gh pr list --repo "+repositoryKey+" --head "+branch+" --state open --json number") {
				t.Fatalf("open PR query missing for %s candidate: %v", test.name, commands)
			}
		})
	}
}

func openPRRunLocalCandidateExecutor(local, tracking bool, commands *[]string) commandExecutor {
	const (
		branch = "agent/issue-55-run-good"
		sha    = "evaluated-head"
	)
	localSHA := ""
	if local {
		localSHA = sha
	}
	trackingSHA := ""
	if tracking {
		trackingSHA = sha
	}
	return func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		*commands = append(*commands, command)
		switch {
		case strings.HasPrefix(command, "git log --format=%H%x00%B%x00 "):
			return "evaluated-commit\x00" + claimMessage(55, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00", nil
		case command == "git ls-remote --heads origin refs/heads/"+branch:
			return "", nil
		case command == "git for-each-ref --format=%(objectname) refs/heads/"+branch:
			return localSHA, nil
		case command == "git for-each-ref --format=%(objectname) refs/remotes/origin/"+branch:
			return trackingSHA, nil
		case strings.HasPrefix(command, "gh pr list "):
			return `[{"number":123}]`, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}
}

func TestUnexpectedRunLocalRefRequiresValidatedTarget(t *testing.T) {
	claims := []claimArtifact{{issue: 86, branch: "agent/issue-86", sha: "evaluated-head"}}
	all := []remoteClaim{{branch: "agent/issue-86-run-good", number: 86, sha: "evaluated-head", source: claimRefRemote}}
	expected := map[string]claimArtifact{"agent/issue-86": claims[0]}
	if err := validateUnexpectedClaimRefs(all, expected, claims); err == nil || !strings.Contains(err.Error(), "no immutable evaluated-head proof") {
		t.Fatalf("unvalidated run-local ref = %v, want preservation refusal", err)
	}
	target := []provenRunLocalRef{{branch: "agent/issue-86-run-good", sha: "evaluated-head", remotePresent: true}}
	if err := validateUnexpectedClaimRefs(all, expected, claims, target); err != nil {
		t.Fatalf("validated run-local ref = %v, want accepted target", err)
	}
}

func TestPreMergeRunLocalWorktreeAllowanceRejectsExtraWorktree(t *testing.T) {
	claims := []claimArtifact{{issue: 86, branch: "agent/issue-86", sha: "claim-head", localBranch: "agent/issue-86-run-good"}}
	expected, err := claimBranchIndex(claims)
	if err != nil {
		t.Fatalf("claimBranchIndex: %v", err)
	}
	layout := repositoryLayout{worktrees: []gitWorktree{
		{path: "/good", head: "claim-head", branch: "refs/heads/agent/issue-86-run-good"},
		{path: "/extra", head: "claim-head", branch: "refs/heads/agent/issue-86-run-other"},
	}}
	if err := validateUnexpectedClaimWorktrees(layout, expected, claims); err == nil || !strings.Contains(err.Error(), "leftover run-local claim worktree") {
		t.Fatalf("extra pre-merge run-local worktree = %v, want preservation refusal", err)
	}
}

func TestRunLocalHistoryPreservesConflictingIdentities(t *testing.T) {
	history := "tip\x00work\x00current\x00" + claimMessage(86, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00" +
		"conflicting\x00" + claimMessage(86, "run-other", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00" +
		"oldest-current\x00" + claimMessage(86, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00" +
		"f7d1ce\x00inherited squash body\nAgent-Run-ID: old-run\nAgent-Run-ID: another-old-run\nAgent-Issue: 1\nAgent-Issue: 2\n\x00"
	identities, err := parseBoundedRunLocalHistory(history, "agent/issue-86-run-good")
	if err != nil {
		t.Fatalf("parseBoundedRunLocalHistory: %v", err)
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "git" || name+" "+strings.Join(args, " ") != "git log --format=%H%x00%B%x00 evaluated-head" {
			return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
		return history, nil
	}}
	ref := runLocalRefCandidate{branch: "agent/issue-86-run-good", sha: "evaluated-head"}
	err = application.validateRunLocalProof("/repo", ref, claimArtifact{issue: 86, sha: "evaluated-head"}, "evaluated-head")
	if err == nil || !strings.Contains(err.Error(), "conflicting runs") {
		t.Fatalf("conflicting history proof = %v, want preservation refusal; identities=%#v", err, identities)
	}
}

func TestRunLocalHistoryRejectsAmbiguousMetadataInsideBoundedRange(t *testing.T) {
	const branch = "agent/issue-86-run-f9"
	for _, test := range []struct {
		name     string
		metadata string
	}{
		{name: "repeated run ID", metadata: "Agent-Run-ID: run-f9\nAgent-Run-ID: run-other\nAgent-Issue: 86\n"},
		{name: "repeated issue", metadata: "Agent-Run-ID: run-f9\nAgent-Issue: 86\nAgent-Issue: 87\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := "tip\x00work\x00current\x00" + claimMessage(86, "run-f9", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00" +
				"ambiguous\x00" + test.metadata + "\x00" +
				"oldest-current\x00" + claimMessage(86, "run-f9", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00" +
				"f7d1ce\x00inherited squash body\nAgent-Run-ID: historical-a\nAgent-Run-ID: historical-b\nAgent-Issue: 1\nAgent-Issue: 2\n\x00"
			commands := []string{}
			application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				command := name + " " + strings.Join(args, " ")
				commands = append(commands, command)
				if command != "git log --format=%H%x00%B%x00 evaluated-head" {
					return "", fmt.Errorf("unexpected command: %s", command)
				}
				return history, nil
			}}
			err := application.validateRunLocalProof("/repo", runLocalRefCandidate{branch: branch, sha: "evaluated-head"}, claimArtifact{issue: 86, sha: "evaluated-head"}, "evaluated-head")
			if err == nil || !strings.Contains(err.Error(), "ambiguous") {
				t.Fatalf("ambiguous bounded history proof = %v, want preservation refusal", err)
			}
			if len(commands) != 1 {
				t.Fatalf("bounded history refusal issued mutation commands: %v", commands)
			}
		})
	}
}

func TestRunLocalHistoryIgnoresInheritedRepeatedTrailers(t *testing.T) {
	const branch = "agent/issue-86-run-f9"
	history := "tip\x00work\x00claim\x00" + claimMessage(86, "run-f9", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00" +
		"f7d1ce\x00inherited squash body\nAgent-Run-ID: historical-a\nAgent-Run-ID: historical-b\nAgent-Issue: 1\nAgent-Issue: 2\n\x00"
	identities, err := parseBoundedRunLocalHistory(history, branch)
	if err != nil {
		t.Fatalf("parseBoundedRunLocalHistory: %v", err)
	}
	if len(identities) != 1 || identities[0] != (runLocalHistoryIdentity{runID: "run-f9", issue: 86}) {
		t.Fatalf("bounded identities = %#v, want current claim only", identities)
	}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		if command != "git log --format=%H%x00%B%x00 evaluated-head" {
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		return history, nil
	}}
	err = application.validateRunLocalProof("/repo", runLocalRefCandidate{branch: branch, sha: "evaluated-head"}, claimArtifact{issue: 86, sha: "evaluated-head"}, "evaluated-head")
	if err != nil {
		t.Fatalf("inherited squash trailers rejected current packet: %v", err)
	}
}

func TestCleanupRemovesOnlyProvenRunLocalDuplicateFromFourRefShape(t *testing.T) {
	fixture := newIssue86FourRefFixture(t)

	commands := []string{}
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, executeCommand: realGitWithOpenPRExecutor(t, false, &commands)}
	layout, err := application.repositoryLayout(fixture.repository.linked)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	plan := cleanupPlan{
		layout:            layout,
		callerRoot:        fixture.repository.linked,
		claims:            []claimArtifact{{issue: fixture.issue, branch: fixture.fixedBranch, sha: fixture.sha, localBranch: fixture.runBranch, worktreePath: fixture.runWorktree}},
		proofHead:         fixture.sha,
		primaryIssue:      fixture.issue,
		validateArtifacts: true,
	}
	base := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: layout}}}
	packet := mergedPacket{number: fixture.issue, mergeSHA: "merge-proof", plan: plan}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("cleanupClaims: %v", err)
	}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("idempotent cleanupClaims: %v", err)
	}
	assertIssue86FourRefCleanup(t, fixture, commands)
}

func TestCleanupRemovesOnlyProvenRunLocalAncestorFromPR154Shape(t *testing.T) {
	fixture := newIssue86AncestorFixture(t)
	commands := []string{}
	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}, executeCommand: realGitWithOpenPRExecutor(t, false, &commands)}
	layout, err := application.repositoryLayout(fixture.repository.primary)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	claims := []claimArtifact{{issue: fixture.issue, branch: fixture.fixedBranch, sha: fixture.proofHead}}
	attached, err := attachClaimWorktrees(layout, claims)
	if err != nil {
		t.Fatalf("attachClaimWorktrees: %v", err)
	}
	if attached[0].localBranch != "" || attached[0].worktreePath != "" {
		t.Fatalf("pre-proof attachment = %#v, want deferred ancestor worktree", attached[0])
	}
	plan := cleanupPlan{
		layout:            layout,
		callerRoot:        fixture.repository.primary,
		claims:            claims,
		proofHead:         fixture.proofHead,
		primaryIssue:      fixture.issue,
		validateArtifacts: true,
	}
	base := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: layout}}}
	packet := mergedPacket{number: fixture.issue, mergeSHA: "merge-proof", plan: plan}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("cleanupClaims: %v", err)
	}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("idempotent cleanupClaims: %v", err)
	}
	assertIssue86AncestorCleanup(t, fixture, commands)
}

type ancestorRunLocalProofCase struct {
	name       string
	branch     string
	sha        string
	history    string
	graph      map[string]string
	openPR     bool
	currentSHA string
	want       string
}

func TestAncestorRunLocalProofPreservesInvalidCandidates(t *testing.T) {
	const (
		branch    = "agent/issue-86-run-good"
		otherRun  = "agent/issue-86-run-other"
		proofHead = "evaluated-head"
		candidate = "candidate-after-anchor"
	)
	runAncestorRunLocalProofCases(t, []ancestorRunLocalProofCase{
		{name: "different run ancestor", branch: otherRun, sha: "canonical-anchor", history: ancestorRunLocalHistory(), want: "no canonical claim marker"},
		{name: "below anchor", branch: branch, sha: "before-anchor", history: ancestorRunLocalHistory(), graph: map[string]string{"before-anchor|" + proofHead: "", "canonical-anchor|before-anchor": "exit status 1"}, want: "before canonical claim anchor"},
		{name: "sibling", branch: branch, sha: "sibling", history: ancestorRunLocalHistory(), graph: map[string]string{"sibling|" + proofHead: "exit status 1"}, want: "not an ancestor"},
		{name: "descendant", branch: branch, sha: "descendant", history: ancestorRunLocalHistory(), graph: map[string]string{"descendant|" + proofHead: "exit status 1"}, want: "not an ancestor"},
		{name: "multiple identities in bounded history", branch: branch, sha: candidate, history: ancestorRunLocalHistoryWithIdentity(), graph: map[string]string{candidate + "|" + proofHead: "", "canonical-anchor|" + candidate: ""}, want: "conflicting runs"},
	})
}

func TestAncestorRunLocalProofPreservesMovedAndOpenPR(t *testing.T) {
	const (
		branch    = "agent/issue-86-run-good"
		proofHead = "evaluated-head"
		candidate = "candidate-after-anchor"
	)
	runAncestorRunLocalProofCases(t, []ancestorRunLocalProofCase{
		{name: "moved", branch: branch, sha: candidate, history: ancestorRunLocalHistory(), graph: map[string]string{candidate + "|" + proofHead: "", "canonical-anchor|" + candidate: ""}, currentSHA: "moved-head", want: "expected candidate-after-anchor, found moved-head"},
		{name: "open PR", branch: branch, sha: candidate, history: ancestorRunLocalHistory(), graph: map[string]string{candidate + "|" + proofHead: "", "canonical-anchor|" + candidate: ""}, openPR: true, want: "open PR"},
	})
}

func runAncestorRunLocalProofCases(t *testing.T, tests []ancestorRunLocalProofCase) {
	t.Helper()
	const proofHead = "evaluated-head"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			currentSHA := test.currentSHA
			if currentSHA == "" {
				currentSHA = test.sha
			}
			commands := []string{}
			application := scriptedAncestorCandidateApplication(test.history, test.graph, currentSHA, test.openPR, &commands)
			packet := mergedPacket{plan: cleanupPlan{
				claims:            []claimArtifact{{issue: 86, branch: "agent/issue-86", sha: proofHead}},
				proofHead:         proofHead,
				primaryIssue:      86,
				validateArtifacts: true,
			}}
			candidate := runLocalRefCandidate{branch: test.branch, sha: test.sha, remotePresent: true}
			_, found, err := application.validateRunLocalCandidate("/repo", packet, candidate)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ancestor candidate validation = (%t, %v), want %q", found, err, test.want)
			}
			if containsRunLocalDelete(commands) {
				t.Fatalf("ancestor candidate validation attempted deletion: %v", commands)
			}
		})
	}
}

func TestAncestorRunLocalCleanupPreservesIncompleteInventory(t *testing.T) {
	const branch = "agent/issue-86-run-good"
	commands := []string{}
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "git ls-remote --heads origin refs/heads/agent/*":
			return "ancestor refs/heads/" + branch, nil
		case "git for-each-ref --format=%(refname:short) %(objectname) refs/heads/agent/issue-*":
			return "incomplete-inventory-entry", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	packet := mergedPacket{plan: cleanupPlan{
		claims:            []claimArtifact{{issue: 86, branch: "agent/issue-86", sha: "evaluated-head"}},
		proofHead:         "evaluated-head",
		primaryIssue:      86,
		validateArtifacts: true,
	}}
	_, err := application.prepareRunLocalCleanup("/repo", packet)
	if err == nil || !strings.Contains(err.Error(), "malformed entry") {
		t.Fatalf("incomplete inventory error = %v, want preservation refusal", err)
	}
	if containsRunLocalDelete(commands) {
		t.Fatalf("incomplete inventory attempted deletion: %v", commands)
	}
}

func TestCleanupRemoteOnlyRunLocalProofDoesNotLeaveTrackingRef(t *testing.T) {
	repository := newBaseRepositoryFixture(t, false)
	remoteClone := repository.root + "/remote-only"
	runGitTest(t, repository.root, "clone", repository.origin, remoteClone)
	configureTestIdentity(t, remoteClone)
	runGitTest(t, remoteClone, "switch", "-c", "agent/issue-86-run-good", "origin/main")
	writeFixtureFile(t, remoteClone, "claim", "claim\n")
	runGitTest(t, remoteClone, "add", "claim")
	runGitTest(t, remoteClone, "commit", "--no-gpg-sign", "-m", claimMessage(86, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)))
	head := runGitTest(t, remoteClone, "rev-parse", "HEAD")
	runGitTest(t, remoteClone, "push", "origin", "HEAD:refs/heads/agent/issue-86")
	runGitTest(t, remoteClone, "push", "origin", "HEAD:refs/heads/agent/issue-86-run-good")

	application := app{ctx: context.Background(), stdout: &bytes.Buffer{}}
	layout, err := application.repositoryLayout(repository.primary)
	if err != nil {
		t.Fatalf("repositoryLayout: %v", err)
	}
	plan := cleanupPlan{
		layout:            layout,
		callerRoot:        repository.primary,
		claims:            []claimArtifact{{issue: 86, branch: "agent/issue-86", sha: head}},
		proofHead:         head,
		primaryIssue:      86,
		validateArtifacts: true,
	}
	base := synchronizedBase{fetched: fetchedBase{primary: cleanPrimary{layout: layout}}}
	commands := []string{}
	application.executeCommand = realGitWithOpenPRExecutor(t, false, &commands)
	packet := mergedPacket{number: 86, mergeSHA: "merge-proof", plan: plan}
	if err := application.cleanupClaims(base, packet); err != nil {
		t.Fatalf("cleanupClaims: %v", err)
	}

	runBranch := "agent/issue-86-run-good"
	for _, ref := range []string{
		"refs/heads/agent/issue-86",
		"refs/heads/" + runBranch,
		"refs/remotes/origin/" + runBranch,
		"refs/workflowctl/run-local-proof/" + runBranch,
	} {
		if output := runGitAllowFailure(t, repository.primary, "show-ref", "--verify", ref); output != "" {
			t.Fatalf("temporary or proven ref %s remains: %s", ref, output)
		}
	}
	fetchCommand := "git fetch --no-tags --no-write-fetch-head origin refs/heads/" + runBranch + ":refs/workflowctl/run-local-proof/" + runBranch
	if !containsCommand(commands, fetchCommand) {
		t.Fatalf("explicit temporary proof fetch = %v, want %q", commands, fetchCommand)
	}
	trackingDelete := "git update-ref -d refs/remotes/origin/" + runBranch + " " + head
	if containsCommand(commands, trackingDelete) {
		t.Fatalf("remote-only proof unexpectedly deleted tracking ref: %v", commands)
	}
}

type issue86FourRefFixture struct {
	repository  baseRepositoryFixture
	issue       int
	sha         string
	fixedBranch string
	runBranch   string
	runWorktree string
	archive     string
	unrelated   string
}

func newIssue86FourRefFixture(t *testing.T) issue86FourRefFixture {
	t.Helper()
	repository := newBaseRepositoryFixture(t, false)
	configureTestIdentity(t, repository.linked)
	const issue = 86
	commitMessage := claimMessage(issue, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC))
	writeFixtureFile(t, repository.linked, "claim", "claim\n")
	runGitTest(t, repository.linked, "add", "claim")
	runGitTest(t, repository.linked, "commit", "--no-gpg-sign", "-m", commitMessage)
	claimSHA := runGitTest(t, repository.linked, "rev-parse", "HEAD")
	writeFixtureFile(t, repository.linked, "work", "evaluated work\n")
	runGitTest(t, repository.linked, "add", "work")
	runGitTest(t, repository.linked, "commit", "--no-gpg-sign", "-m", "feat: evaluated work")
	sha := runGitTest(t, repository.linked, "rev-parse", "HEAD")
	if sha == claimSHA {
		t.Fatal("evaluated head did not advance beyond claim metadata commit")
	}
	fixedBranch := "agent/issue-86"
	runBranch := "agent/issue-86-run-good"
	runWorktree := claimWorktreePath(repository.primary, runBranch)
	runGitTest(t, repository.primary, "worktree", "add", "-b", runBranch, runWorktree, sha)
	archive := "agent/archive/issue-86-old"
	unrelated := "agent/other"
	for _, branch := range []string{fixedBranch, runBranch, archive, unrelated} {
		runGitTest(t, repository.linked, "push", "origin", "HEAD:refs/heads/"+branch)
	}
	runGitTest(t, repository.primary, "fetch", "origin", "refs/heads/"+runBranch+":refs/remotes/origin/"+runBranch)
	return issue86FourRefFixture{
		repository:  repository,
		issue:       issue,
		sha:         sha,
		fixedBranch: fixedBranch,
		runBranch:   runBranch,
		runWorktree: runWorktree,
		archive:     archive,
		unrelated:   unrelated,
	}
}

func assertIssue86FourRefCleanup(t *testing.T, fixture issue86FourRefFixture, commands []string) {
	t.Helper()
	for _, branch := range []string{fixture.fixedBranch, fixture.runBranch} {
		if output := runGitAllowFailure(t, fixture.repository.primary, "ls-remote", "--heads", "origin", "refs/heads/"+branch); output != "" {
			t.Fatalf("proven claim ref %s remains: %s", branch, output)
		}
	}
	for _, branch := range []string{fixture.archive, fixture.unrelated} {
		if output := runGitAllowFailure(t, fixture.repository.primary, "ls-remote", "--heads", "origin", "refs/heads/"+branch); output == "" {
			t.Fatalf("preserved ref %s was removed", branch)
		}
	}
	if output := runGitAllowFailure(t, fixture.repository.primary, "show-ref", "--verify", "refs/remotes/origin/"+fixture.runBranch); output != "" {
		t.Fatalf("matching run-local tracking ref remains: %s", output)
	}
	if output := runGitAllowFailure(t, fixture.repository.primary, "show-ref", "--verify", "refs/heads/"+fixture.runBranch); output != "" {
		t.Fatalf("matching local run-local ref remains: %s", output)
	}
	if inventory := runGitTest(t, fixture.repository.primary, "worktree", "list", "--porcelain"); strings.Contains(inventory, fixture.runWorktree) {
		t.Fatalf("matching local run-local worktree remains:\n%s", inventory)
	}
	wantLeaseDelete := "git push --force-with-lease=refs/heads/" + fixture.runBranch + ":" + fixture.sha + " origin :refs/heads/" + fixture.runBranch
	if !containsCommand(commands, wantLeaseDelete) {
		t.Fatalf("exact run-local lease deletion = %v, want %q", commands, wantLeaseDelete)
	}
	wantLocalDelete := "git update-ref -d refs/heads/" + fixture.runBranch + " " + fixture.sha
	if !containsCommand(commands, wantLocalDelete) {
		t.Fatalf("exact local run-local deletion = %v, want %q", commands, wantLocalDelete)
	}
	for _, command := range commands {
		if strings.Contains(command, ":refs/heads/agent/*") {
			t.Fatalf("wildcard ref deletion was attempted: %q", command)
		}
	}
}

type issue86AncestorFixture struct {
	repository      baseRepositoryFixture
	issue           int
	anchor          string
	proofHead       string
	fixedBranch     string
	runBranch       string
	runWorktree     string
	archive         string
	unrelated       string
	archiveBefore   string
	unrelatedBefore string
}

func newIssue86AncestorFixture(t *testing.T) issue86AncestorFixture {
	t.Helper()
	repository := newBaseRepositoryFixture(t, false)
	configureTestIdentity(t, repository.linked)
	const issue = 86
	runID := "run-good"
	commitClaim := func(lease time.Time) string {
		runGitTest(t, repository.linked, "commit", "--no-gpg-sign", "--allow-empty", "-m", claimMessage(issue, runID, lease))
		return runGitTest(t, repository.linked, "rev-parse", "HEAD")
	}
	anchor := commitClaim(time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC))
	writeFixtureFile(t, repository.linked, "run-one", "run one\n")
	runGitTest(t, repository.linked, "add", "run-one")
	runGitTest(t, repository.linked, "commit", "--no-gpg-sign", "-m", "feat: run one")
	commitClaim(time.Date(2099, time.January, 1, 0, 0, 1, 0, time.UTC))
	writeFixtureFile(t, repository.linked, "run-two", "run two\n")
	runGitTest(t, repository.linked, "add", "run-two")
	runGitTest(t, repository.linked, "commit", "--no-gpg-sign", "-m", "feat: run two")
	commitClaim(time.Date(2099, time.January, 1, 0, 0, 2, 0, time.UTC))
	writeFixtureFile(t, repository.linked, "evaluated", "evaluated\n")
	runGitTest(t, repository.linked, "add", "evaluated")
	runGitTest(t, repository.linked, "commit", "--no-gpg-sign", "-m", "feat: evaluated work")
	proofHead := runGitTest(t, repository.linked, "rev-parse", "HEAD")
	if anchor == proofHead {
		t.Fatal("ancestor fixture did not advance beyond canonical anchor")
	}
	fixedBranch := "agent/issue-86"
	runBranch := "agent/issue-86-run-good"
	archive := "agent/archive/issue-86-old"
	unrelated := "agent/other"
	for _, branch := range []string{fixedBranch, archive, unrelated} {
		runGitTest(t, repository.linked, "push", "origin", "HEAD:refs/heads/"+branch)
	}
	runWorktree := claimWorktreePath(repository.primary, runBranch)
	runGitTest(t, repository.primary, "worktree", "add", "-b", runBranch, runWorktree, anchor)
	runGitTest(t, repository.linked, "push", "origin", anchor+":refs/heads/"+runBranch)
	runGitTest(t, repository.primary, "fetch", "origin", "refs/heads/"+runBranch+":refs/remotes/origin/"+runBranch)
	return issue86AncestorFixture{
		repository:      repository,
		issue:           issue,
		anchor:          anchor,
		proofHead:       proofHead,
		fixedBranch:     fixedBranch,
		runBranch:       runBranch,
		runWorktree:     runWorktree,
		archive:         archive,
		unrelated:       unrelated,
		archiveBefore:   runGitTest(t, repository.primary, "ls-remote", "--heads", "origin", "refs/heads/"+archive),
		unrelatedBefore: runGitTest(t, repository.primary, "ls-remote", "--heads", "origin", "refs/heads/"+unrelated),
	}
}

func assertIssue86AncestorCleanup(t *testing.T, fixture issue86AncestorFixture, commands []string) {
	t.Helper()
	assertIssue86AncestorRemoteRefs(t, fixture)
	assertIssue86AncestorLocalArtifacts(t, fixture)
	assertIssue86AncestorDeletes(t, fixture, commands)
}

func assertIssue86AncestorRemoteRefs(t *testing.T, fixture issue86AncestorFixture) {
	t.Helper()
	for _, branch := range []string{fixture.fixedBranch, fixture.runBranch} {
		if output := runGitAllowFailure(t, fixture.repository.primary, "ls-remote", "--heads", "origin", "refs/heads/"+branch); output != "" {
			t.Fatalf("proven ref %s remains: %s", branch, output)
		}
	}
	for _, preserved := range []struct {
		branch string
		want   string
	}{
		{branch: fixture.archive, want: fixture.archiveBefore},
		{branch: fixture.unrelated, want: fixture.unrelatedBefore},
	} {
		if output := runGitTest(t, fixture.repository.primary, "ls-remote", "--heads", "origin", "refs/heads/"+preserved.branch); output != preserved.want {
			t.Fatalf("preserved ref %s changed from %q to %q", preserved.branch, preserved.want, output)
		}
	}
}

func assertIssue86AncestorLocalArtifacts(t *testing.T, fixture issue86AncestorFixture) {
	t.Helper()
	for _, ref := range []string{
		"refs/heads/" + fixture.runBranch,
		"refs/remotes/origin/" + fixture.runBranch,
	} {
		if output := runGitAllowFailure(t, fixture.repository.primary, "show-ref", "--verify", ref); output != "" {
			t.Fatalf("proven local ref %s remains: %s", ref, output)
		}
	}
	if inventory := runGitTest(t, fixture.repository.primary, "worktree", "list", "--porcelain"); strings.Contains(inventory, fixture.runWorktree) {
		t.Fatalf("proven run-local worktree remains:\n%s", inventory)
	}
}

func assertIssue86AncestorDeletes(t *testing.T, fixture issue86AncestorFixture, commands []string) {
	t.Helper()
	wantLeaseDelete := "git push --force-with-lease=refs/heads/" + fixture.runBranch + ":" + fixture.anchor + " origin :refs/heads/" + fixture.runBranch
	if !containsCommand(commands, wantLeaseDelete) {
		t.Fatalf("exact ancestor lease deletion = %v, want %q", commands, wantLeaseDelete)
	}
	wantLocalDelete := "git update-ref -d refs/heads/" + fixture.runBranch + " " + fixture.anchor
	if !containsCommand(commands, wantLocalDelete) {
		t.Fatalf("exact ancestor local deletion = %v, want %q", commands, wantLocalDelete)
	}
	wantTrackingDelete := "git update-ref -d refs/remotes/origin/" + fixture.runBranch + " " + fixture.anchor
	if !containsCommand(commands, wantTrackingDelete) {
		t.Fatalf("exact ancestor tracking deletion = %v, want %q", commands, wantTrackingDelete)
	}
	for _, command := range commands {
		if strings.Contains(command, ":refs/heads/agent/*") {
			t.Fatalf("wildcard ref deletion was attempted: %q", command)
		}
	}
}

func ancestorRunLocalHistory() string {
	const (
		anchor = "canonical-anchor"
		runID  = "run-good"
	)
	lease := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	return "evaluated\x00evaluated work\x00" +
		"renewal\x00" + claimMessage(86, runID, lease.Add(time.Minute)) + "\x00" +
		anchor + "\x00" + claimMessage(86, runID, lease) + "\x00" +
		"inherited\x00base history\x00"
}

func ancestorRunLocalHistoryWithIdentity() string {
	const (
		anchor           = "canonical-anchor"
		runID            = "run-good"
		conflictingRunID = "run-other"
	)
	lease := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	return "evaluated\x00evaluated work\x00" +
		"renewal\x00" + claimMessage(86, runID, lease.Add(2*time.Minute)) + "\x00" +
		"conflicting\x00" + claimMessage(86, conflictingRunID, lease.Add(time.Minute)) + "\x00" +
		anchor + "\x00" + claimMessage(86, runID, lease) + "\x00" +
		"inherited\x00base history\x00"
}

func scriptedAncestorCandidateApplication(history string, graph map[string]string, currentSHA string, openPR bool, commands *[]string) app {
	return app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		*commands = append(*commands, command)
		switch {
		case strings.HasPrefix(command, "git log --format=%H%x00%B%x00 "):
			return history, nil
		case strings.HasPrefix(command, "git merge-base --is-ancestor "):
			values := strings.Fields(command)
			if len(values) != 5 {
				return "", fmt.Errorf("unexpected merge-base command: %s", command)
			}
			result, ok := graph[values[3]+"|"+values[4]]
			if !ok {
				return "", fmt.Errorf("unexpected merge-base command: %s", command)
			}
			if result == "exit status 1" {
				return "", errors.New("exit status 1")
			}
			return result, nil
		case strings.HasPrefix(command, "git ls-remote --heads origin refs/heads/"):
			branch := strings.TrimPrefix(command, "git ls-remote --heads origin refs/heads/")
			return currentSHA + " refs/heads/" + branch, nil
		case strings.HasPrefix(command, "gh pr list "):
			if openPR {
				return `[{"number":99}]`, nil
			}
			return "[]", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
}

func scriptedRunLocalApplication(inventory, currentSHA string, openPR bool, commands *[]string) app {
	return app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		*commands = append(*commands, command)
		switch {
		case command == "git ls-remote --heads origin refs/heads/agent/*":
			return inventory, nil
		case command == "git for-each-ref --format=%(refname:short) %(objectname) refs/heads/agent/issue-*":
			return "", nil
		case command == "git for-each-ref --format=%(refname:short) %(objectname) refs/remotes/origin/agent/issue-*":
			return "", nil
		case strings.HasPrefix(command, "git log --format=%H%x00%B%x00 "):
			return "evaluated-commit\x00" + claimMessage(55, "run-good", time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)) + "\x00", nil
		case strings.HasPrefix(command, "git ls-remote --heads origin refs/heads/"):
			branch := strings.TrimPrefix(command, "git ls-remote --heads origin refs/heads/")
			return currentSHA + " refs/heads/" + branch, nil
		case strings.HasPrefix(command, "gh pr list "):
			if openPR {
				return `[{"number":99}]`, nil
			}
			return "[]", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
}

func realGitWithOpenPRExecutor(t *testing.T, openPR bool, commands *[]string) commandExecutor {
	t.Helper()
	return func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if commands != nil {
			*commands = append(*commands, name+" "+strings.Join(args, " "))
		}
		if name == "gh" {
			if openPR {
				return `[{"number":99}]`, nil
			}
			return "[]", nil
		}
		// #nosec G204 -- this test executor invokes fixed Git commands in temporary repositories.
		command := exec.CommandContext(context.Background(), name, args...)
		command.Dir = dir
		command.Stdin = input
		output, err := command.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
		return strings.TrimSpace(string(output)), nil
	}
}

func containsRunLocalDelete(commands []string) bool {
	for _, command := range commands {
		if strings.Contains(command, "push --force-with-lease=refs/heads/agent/") ||
			strings.Contains(command, "update-ref -d refs/remotes/origin/agent/") ||
			strings.Contains(command, "update-ref -d refs/heads/agent/") {
			return true
		}
	}
	return false
}
