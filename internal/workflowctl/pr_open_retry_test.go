package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func retryOpenPR(head, body string) pullRequestAPI {
	pr := pullRequestAPI{Number: 14, Body: body, Draft: true, State: "open",
		Title: "fix(workflow): reuse existing draft", URL: "https://github.com/goxdra/goxsd9/pull/14"}
	pr.Base.Ref = "main"
	pr.Base.Repo.FullName = repositoryKey
	pr.Head.Ref = claimBranch(13)
	pr.Head.SHA = head
	pr.Head.Repo.FullName = repositoryKey
	return pr
}

type retryOpenBackend struct {
	localHead  string
	remoteHead string
	existing   []pullRequestAPI
	pushes     int
	posts      int
}

func (b *retryOpenBackend) execute(_ string, input io.Reader, name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	switch command {
	case "git rev-parse --show-toplevel":
		return "/repo", nil
	case "git branch --show-current":
		return "agent/issue-13-run-test", nil
	case "git status --porcelain":
		return "", nil
	case "git log --format=%x00%B%x00 origin/main..HEAD":
		return framedCommitLog("fix(workflow): reuse existing draft"), nil
	case "git fetch origin refs/heads/agent/issue-13:refs/remotes/origin/agent/issue-13":
		return "", nil
	case "git rev-parse HEAD":
		return b.localHead, nil
	case "git rev-parse origin/agent/issue-13":
		return b.remoteHead, nil
	case "git log -100 --format=%B":
		return claimMessage(13, "run-test", time.Now().UTC().Add(time.Hour)), nil
	case "git push origin HEAD:refs/heads/agent/issue-13":
		b.pushes++
		b.remoteHead = b.localHead
		if len(b.existing) != 0 {
			b.existing[0].Head.SHA = b.localHead
		}
		return "", nil
	case "gh api repos/goxdra/goxsd9/pulls?state=open&head=goxdra:agent/issue-13&per_page=100":
		encoded, err := json.Marshal(b.existing)
		return string(encoded), err
	case "gh api --method POST repos/goxdra/goxsd9/pulls --input -":
		return b.createDraft(input)
	default:
		return "", fmt.Errorf("unexpected command: %s", command)
	}
}

func (b *retryOpenBackend) createDraft(input io.Reader) (string, error) {
	b.posts++
	var request createPullRequestRequest
	if err := json.NewDecoder(input).Decode(&request); err != nil {
		return "", err
	}
	if request.Head != claimBranch(13) || request.Title != "fix(workflow): reuse existing draft" ||
		!containsNumber(closingIssueNumbers(request.Body), 13) {
		return "", fmt.Errorf("unexpected draft request: %+v", request)
	}
	b.existing = []pullRequestAPI{retryOpenPR(b.localHead, request.Body)}
	return `{"number":14,"html_url":"https://github.com/goxdra/goxsd9/pull/14"}`, nil
}

func TestPROpenRunLocalRetryPublishesRemediationAndReusesDraft(t *testing.T) {
	const firstHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const remediationHead = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	body := testPRReviewStateFrame(t, prReviewStatePending) + "\nCloses #13\n"
	path := filepath.Join(t.TempDir(), "pr.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	backend := &retryOpenBackend{localHead: firstHead, remoteHead: firstHead, existing: []pullRequestAPI{}}
	application := app{ctx: context.Background(), stdout: &stdout, stderr: &bytes.Buffer{},
		executeCommand: backend.execute}
	open := func() {
		t.Helper()
		if err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path}); err != nil {
			t.Fatalf("checked pr open: %v", err)
		}
	}
	open()
	if backend.posts != 1 || backend.pushes != 1 {
		t.Fatalf("initial open: posts=%d pushes=%d, want one each", backend.posts, backend.pushes)
	}
	backend.localHead = remediationHead
	backend.remoteHead = remediationHead
	backend.existing[0].Body = testPRReviewStateFrame(t, prReviewStateEvidenceReady) + "\nCloses #13\nPreserve evidence.\n"
	open()
	open()
	if backend.posts != 1 || backend.pushes != 3 {
		t.Fatalf("retry: posts=%d pushes=%d, want one POST and three checked publications", backend.posts, backend.pushes)
	}
	if backend.existing[0].Body != testPRReviewStateFrame(t, prReviewStateEvidenceReady)+"\nCloses #13\nPreserve evidence.\n" {
		t.Fatal("retry changed existing PR body")
	}
	if got := strings.Count(stdout.String(), backend.existing[0].URL+"\n"); got != 3 {
		t.Fatalf("returned PR URL count = %d, want 3; output %q", got, stdout.String())
	}
}

type existingPRLookupCase struct {
	name string
	prs  []pullRequestAPI
	data string
	read error
	want string
}

func assertExistingPRLookupConflict(t *testing.T, tt existingPRLookupCase, head string) {
	t.Helper()
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		if name != "gh" || strings.Join(args, " ") != "api repos/goxdra/goxsd9/pulls?state=open&head=goxdra:agent/issue-13&per_page=100" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		if tt.read != nil {
			return "", tt.read
		}
		if tt.data != "" {
			return tt.data, nil
		}
		encoded, err := json.Marshal(tt.prs)
		return string(encoded), err
	}}
	_, found, err := application.existingOpenPullRequest("/repo", claimBranch(13), 13, head)
	if err == nil || found || !strings.Contains(err.Error(), tt.want) {
		t.Fatalf("lookup = found %t, error %v; want %q", found, err, tt.want)
	}
	if tt.read != nil && !errors.Is(err, tt.read) {
		t.Fatalf("lookup error %v lost cause %v", err, tt.read)
	}
}

func TestExistingOpenPRLookupRejectsConflictsAndPreservesCause(t *testing.T) {
	const head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	valid := retryOpenPR(head, "Closes #13\n")
	tests := []existingPRLookupCase{
		{name: "ambiguous", prs: []pullRequestAPI{valid, valid}, want: "2 open PRs"},
		{name: "fork head", prs: func() []pullRequestAPI {
			pr := valid
			pr.Head.Repo.FullName = "other/goxsd9"
			return []pullRequestAPI{pr}
		}(), want: "conflicting identity"},
		{name: "wrong branch", prs: func() []pullRequestAPI {
			pr := valid
			pr.Head.Ref = "agent/issue-13-run-test"
			return []pullRequestAPI{pr}
		}(), want: "conflicting identity"},
		{name: "wrong head", prs: func() []pullRequestAPI {
			pr := valid
			pr.Head.SHA = strings.Repeat("b", 40)
			return []pullRequestAPI{pr}
		}(), want: "conflicting identity"},
		{name: "wrong base", prs: func() []pullRequestAPI { pr := valid; pr.Base.Ref = "other"; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "wrong base repository", prs: func() []pullRequestAPI {
			pr := valid
			pr.Base.Repo.FullName = "other/goxsd9"
			return []pullRequestAPI{pr}
		}(), want: "conflicting identity"},
		{name: "closed", prs: func() []pullRequestAPI { pr := valid; pr.State = "closed"; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "merged", prs: func() []pullRequestAPI { pr := valid; pr.Merged = true; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "missing number", prs: func() []pullRequestAPI { pr := valid; pr.Number = 0; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "missing URL", prs: func() []pullRequestAPI { pr := valid; pr.URL = ""; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "invalid title", prs: func() []pullRequestAPI { pr := valid; pr.Title = "Bad title"; return []pullRequestAPI{pr} }(), want: "invalid title"},
		{name: "wrong issue", prs: func() []pullRequestAPI { pr := valid; pr.Body = "Closes #12\n"; return []pullRequestAPI{pr} }(), want: "does not close issue #13"},
		{name: "null", data: "null", want: "returned no list"},
		{name: "malformed", data: "{", want: "decode open PRs"},
		{name: "transport", read: errors.New("service unavailable"), want: "service unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertExistingPRLookupConflict(t, tt, head)
		})
	}
}
