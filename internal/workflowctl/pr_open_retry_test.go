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
	"strconv"
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
	localHead             string
	remoteHead            string
	existing              []pullRequestAPI
	listedHead            string
	listAfterPush         []pullRequestAPI
	overrideListAfterPush bool
	published             bool
	detailHeads           []string
	detailMutate          func(*pullRequestAPI, int)
	detailErrAt           int
	detailErr             error
	ancestorErr           error
	details               int
	lists                 int
	pushes                int
	posts                 int
}

func (b *retryOpenBackend) execute(_ string, input io.Reader, name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	if name == "gh" {
		return b.executeGitHub(command, input)
	}
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
	case "git fetch origin main":
		return "", nil
	case "git rev-parse HEAD":
		return b.localHead, nil
	case "git rev-parse origin/agent/issue-13":
		return b.remoteHead, nil
	case "git merge-base --is-ancestor " + strings.Repeat("a", 40) + " " + strings.Repeat("b", 40):
		return "", b.ancestorErr
	case "git merge-base --is-ancestor " + b.remoteHead + " " + b.remoteHead:
		return "", b.ancestorErr
	case "git log -100 --format=%B":
		return claimMessage(13, "run-test", time.Now().UTC().Add(time.Hour)), nil
	case "git push origin HEAD:refs/heads/agent/issue-13":
		b.pushes++
		b.published = true
		b.remoteHead = b.localHead
		if len(b.existing) != 0 {
			b.existing[0].Head.SHA = b.localHead
		}
		return "", nil
	default:
		return "", fmt.Errorf("unexpected command: %s", command)
	}
}

func (b *retryOpenBackend) executeGitHub(command string, input io.Reader) (string, error) {
	switch command {
	case "gh api repos/goxdra/goxsd9/pulls?state=open&head=goxdra:agent/issue-13&per_page=100":
		b.lists++
		listed := append([]pullRequestAPI(nil), b.existing...)
		if b.published && b.overrideListAfterPush {
			listed = append([]pullRequestAPI(nil), b.listAfterPush...)
		}
		if len(listed) != 0 && b.listedHead != "" {
			listed[0].Head.SHA = b.listedHead
		}
		if len(listed) == 0 {
			listed = []pullRequestAPI{}
		}
		encoded, err := json.Marshal(listed)
		return string(encoded), err
	case "gh api repos/goxdra/goxsd9/pulls/14":
		b.details++
		if b.details == b.detailErrAt {
			return "", b.detailErr
		}
		pr := b.existing[0]
		if len(b.detailHeads) != 0 {
			pr.Head.SHA = b.detailHeads[0]
			b.detailHeads = b.detailHeads[1:]
		}
		if b.detailMutate != nil {
			b.detailMutate(&pr, b.details)
		}
		encoded, err := json.Marshal(pr)
		return string(encoded), err
	case "gh api repos/goxdra/goxsd9/pulls/15":
		encoded, err := json.Marshal(b.listAfterPush[0])
		return string(encoded), err
	case "gh api --method POST repos/goxdra/goxsd9/pulls --input -":
		return b.createDraft(input)
	default:
		return "", fmt.Errorf("unexpected command: %s", command)
	}
}

func TestPROpenRejectsChangedPostPublicationPRList(t *testing.T) {
	for _, tt := range []struct {
		name string
		list func(pullRequestAPI) []pullRequestAPI
		want string
	}{
		{name: "disappeared", list: func(pullRequestAPI) []pullRequestAPI { return nil }, want: "changed identity"},
		{name: "ambiguous", list: func(pr pullRequestAPI) []pullRequestAPI { return []pullRequestAPI{pr, pr} }, want: "2 open PRs"},
		{name: "replacement", list: func(pr pullRequestAPI) []pullRequestAPI {
			pr.Number = 15
			return []pullRequestAPI{pr}
		}, want: "changed identity"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := retryOpenExistingBackend(t)
			backend.detailHeads = []string{backend.existing[0].Head.SHA}
			backend.overrideListAfterPush = true
			backend.listAfterPush = tt.list(backend.existing[0])
			application, path, stdout, waits := openRetryFixture(t, backend)
			err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("changed list error = %v, want %q", err, tt.want)
			}
			if backend.pushes != 1 || backend.posts != 0 || backend.lists != 2 || backend.details != 1 || len(*waits) != 0 || strings.Contains(stdout.String(), backend.existing[0].URL) {
				t.Fatalf("changed list mutations/observations = push %d post %d list %d detail %d waits %v stdout %q",
					backend.pushes, backend.posts, backend.lists, backend.details, *waits, stdout.String())
			}
		})
	}
}

func openRetryFixture(t *testing.T, backend *retryOpenBackend) (app, string, *bytes.Buffer, *[]time.Duration) {
	t.Helper()
	body := testPRReviewStateFrame(t, prReviewStatePending) + "\nCloses #13\n"
	path := filepath.Join(t.TempDir(), "pr.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout := new(bytes.Buffer)
	waits := new([]time.Duration)
	application := app{ctx: context.Background(), stdout: stdout, stderr: new(bytes.Buffer),
		prHeadObservationWait: func(duration time.Duration) { *waits = append(*waits, duration) },
		claimMetadataReader: func(_ string, issue int, branch string) (time.Time, string, error) {
			if issue != 13 || branch != claimLocalBranch(13, "run-test") {
				return time.Time{}, "", fmt.Errorf("unexpected claim %d/%s", issue, branch)
			}
			return time.Now().UTC().Add(time.Hour), "run-test", nil
		}, executeCommand: backend.execute}
	return application, path, stdout, waits
}

func retryOpenExistingBackend(t *testing.T) *retryOpenBackend {
	t.Helper()
	const oldHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newHead = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	return &retryOpenBackend{localHead: newHead, remoteHead: newHead,
		existing:   []pullRequestAPI{retryOpenPR(oldHead, "Closes #13\nPreserve evidence.\n")},
		listedHead: oldHead}
}

func TestPROpenWaitsForPreviouslyVerifiedPRHead(t *testing.T) {
	backend := retryOpenExistingBackend(t)
	oldHead, newHead := backend.existing[0].Head.SHA, backend.localHead
	backend.detailHeads = []string{oldHead, oldHead, newHead}
	application, path, stdout, waits := openRetryFixture(t, backend)
	if err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path}); err != nil {
		t.Fatalf("open after delayed PR head propagation: %v", err)
	}
	if backend.pushes != 1 || backend.posts != 0 || backend.lists != 2 || backend.details != 3 {
		t.Fatalf("observations/mutations = push %d post %d list %d detail %d, want 1/0/2/3",
			backend.pushes, backend.posts, backend.lists, backend.details)
	}
	if len(*waits) != 1 || (*waits)[0] != prHeadObservationInterval {
		t.Fatalf("waits = %v, want one interval %s", *waits, prHeadObservationInterval)
	}
	if !strings.HasSuffix(stdout.String(), backend.existing[0].URL+"\n") || strings.Count(stdout.String(), backend.existing[0].URL) != 1 {
		t.Fatalf("stdout = %q, want checked existing PR URL", stdout.String())
	}
	if backend.existing[0].Body != "Closes #13\nPreserve evidence.\n" {
		t.Fatalf("existing PR body changed: %q", backend.existing[0].Body)
	}
}

func TestPROpenPRHeadObservationExhaustionIsRetryable(t *testing.T) {
	backend := retryOpenExistingBackend(t)
	oldHead := backend.existing[0].Head.SHA
	backend.detailHeads = []string{oldHead, oldHead, oldHead, oldHead, oldHead, oldHead}
	application, path, stdout, waits := openRetryFixture(t, backend)
	err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path})
	if err == nil || operationDispositionOf(err) != operationDispositionRetryable ||
		!strings.Contains(err.Error(), oldHead) || !strings.Contains(err.Error(), backend.localHead) ||
		!strings.Contains(err.Error(), strconv.Itoa(prHeadObservationAttempts)) {
		t.Fatalf("exhaustion error = %v, want retryable with old/new heads and bound", err)
	}
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		t.Fatalf("exhaustion error = %v, unexpectedly classified as state exit %d", err, exitErr.code)
	}
	if backend.pushes != 1 || backend.posts != 0 || backend.lists != 2 || backend.details != prHeadObservationAttempts+1 {
		t.Fatalf("exhaustion observations/mutations = push %d post %d list %d detail %d",
			backend.pushes, backend.posts, backend.lists, backend.details)
	}
	if len(*waits) != prHeadObservationAttempts-1 || strings.Contains(stdout.String(), backend.existing[0].URL) {
		t.Fatalf("exhaustion waits = %v, stdout = %q", *waits, stdout.String())
	}
	for _, duration := range *waits {
		if duration != prHeadObservationInterval {
			t.Fatalf("wait duration = %s, want %s", duration, prHeadObservationInterval)
		}
	}
}

func TestPROpenRejectsConflictingPRHeadWithoutWaiting(t *testing.T) {
	backend := retryOpenExistingBackend(t)
	backend.detailHeads = []string{backend.existing[0].Head.SHA, strings.Repeat("c", 40)}
	application, path, stdout, waits := openRetryFixture(t, backend)
	err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path})
	if err == nil || !strings.Contains(err.Error(), "differs from published head") || operationDispositionOf(err) == operationDispositionRetryable {
		t.Fatalf("conflicting-head error = %v, want immediate state rejection", err)
	}
	if backend.pushes != 1 || backend.posts != 0 || backend.details != 2 || len(*waits) != 0 || strings.Contains(stdout.String(), backend.existing[0].URL) {
		t.Fatalf("conflict mutations/observations = push %d post %d detail %d waits %v stdout %q",
			backend.pushes, backend.posts, backend.details, *waits, stdout.String())
	}
}

func TestPROpenRequiresPrepublicationPRHeadAncestry(t *testing.T) {
	for _, tt := range []struct {
		name  string
		cause error
		want  string
	}{
		{name: "unrelated head", cause: errors.New("exit status 1"), want: "outside verified claim branch history"},
		{name: "ancestry transport", cause: errors.New("git unavailable"), want: "prove open PR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := retryOpenExistingBackend(t)
			backend.ancestorErr = tt.cause
			backend.detailHeads = []string{backend.existing[0].Head.SHA}
			application, path, stdout, waits := openRetryFixture(t, backend)
			err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ancestry error = %v, want %q", err, tt.want)
			}
			if tt.name == "ancestry transport" && (!errors.Is(err, tt.cause) || operationDispositionOf(err) != operationDispositionRetryable) {
				t.Fatalf("ancestry error %v lost retryable cause %v", err, tt.cause)
			}
			if backend.pushes != 0 || backend.posts != 0 || backend.details != 1 || len(*waits) != 0 || stdout.Len() != 0 {
				t.Fatalf("ancestry failure reached mutation or retry: push %d post %d detail %d waits %v stdout %q",
					backend.pushes, backend.posts, backend.details, *waits, stdout.String())
			}
		})
	}
}

func TestPROpenPreservesPRHeadObservationTransportCause(t *testing.T) {
	backend := retryOpenExistingBackend(t)
	oldHead := backend.existing[0].Head.SHA
	backend.detailHeads = []string{oldHead, oldHead}
	backend.detailErrAt = 3
	backend.detailErr = errors.New("PR detail unavailable")
	application, path, stdout, waits := openRetryFixture(t, backend)
	err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path})
	if err == nil || !errors.Is(err, backend.detailErr) || !strings.Contains(err.Error(), "read open PR #14") {
		t.Fatalf("observation transport error = %v, want preserved detail cause", err)
	}
	if backend.pushes != 1 || backend.posts != 0 || backend.lists != 2 || backend.details != 3 || len(*waits) != 1 || strings.Contains(stdout.String(), backend.existing[0].URL) {
		t.Fatalf("transport observations/mutations = push %d post %d list %d detail %d waits %v stdout %q",
			backend.pushes, backend.posts, backend.lists, backend.details, *waits, stdout.String())
	}
}

func TestPROpenRevalidatesPRDetailsDuringHeadObservation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*pullRequestAPI)
		want   string
	}{
		{name: "branch ownership", mutate: func(pr *pullRequestAPI) { pr.Head.Ref = "agent/issue-999" }, want: "conflicting identity"},
		{name: "base ownership", mutate: func(pr *pullRequestAPI) { pr.Base.Repo.FullName = "other/goxsd9" }, want: "conflicting identity"},
		{name: "body closure", mutate: func(pr *pullRequestAPI) { pr.Body = "Closes #12\n" }, want: "does not close issue #13"},
		{name: "title", mutate: func(pr *pullRequestAPI) { pr.Title = "Bad title" }, want: "invalid title"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := retryOpenExistingBackend(t)
			oldHead := backend.existing[0].Head.SHA
			backend.detailHeads = []string{oldHead, oldHead, backend.localHead}
			backend.detailMutate = func(pr *pullRequestAPI, read int) {
				if read == 3 {
					tt.mutate(pr)
				}
			}
			application, path, stdout, waits := openRetryFixture(t, backend)
			err := application.openPullRequest([]string{"13", "--title", "fix(workflow): reuse existing draft", "--body-file", path})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("revalidation error = %v, want %q", err, tt.want)
			}
			if backend.pushes != 1 || backend.posts != 0 || backend.details != 3 || len(*waits) != 1 || strings.Contains(stdout.String(), backend.existing[0].URL) {
				t.Fatalf("revalidation mutation/observation = push %d post %d detail %d waits %v stdout %q",
					backend.pushes, backend.posts, backend.details, *waits, stdout.String())
			}
		})
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
	var waits []time.Duration
	backend := &retryOpenBackend{localHead: firstHead, remoteHead: firstHead, existing: []pullRequestAPI{}}
	application := app{ctx: context.Background(), stdout: &stdout, stderr: &bytes.Buffer{},
		prHeadObservationWait: func(duration time.Duration) { waits = append(waits, duration) },
		claimMetadataReader: func(_ string, issue int, branch string) (time.Time, string, error) {
			if issue != 13 || branch != claimLocalBranch(13, "run-test") {
				return time.Time{}, "", fmt.Errorf("unexpected claim %d/%s", issue, branch)
			}
			return time.Now().UTC().Add(time.Hour), "run-test", nil
		}, executeCommand: backend.execute}
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
	backend.listedHead = firstHead
	backend.detailHeads = []string{firstHead, firstHead, remediationHead}
	backend.existing[0].Body = testPRReviewStateFrame(t, prReviewStateEvidenceReady) + "\nCloses #13\nPreserve evidence.\n"
	open()
	open()
	if backend.posts != 1 || backend.pushes != 3 || backend.details != 5 || backend.lists != 6 {
		t.Fatalf("retry: posts=%d pushes=%d details=%d lists=%d, want one POST, three publications, five detail reads, six list reads",
			backend.posts, backend.pushes, backend.details, backend.lists)
	}
	if len(waits) != 1 || waits[0] != prHeadObservationInterval {
		t.Fatalf("waits = %v, want one bounded observation interval", waits)
	}
	if backend.existing[0].Body != testPRReviewStateFrame(t, prReviewStateEvidenceReady)+"\nCloses #13\nPreserve evidence.\n" {
		t.Fatal("retry changed existing PR body")
	}
	if got := strings.Count(stdout.String(), backend.existing[0].URL+"\n"); got != 3 {
		t.Fatalf("returned PR URL count = %d, want 3; output %q", got, stdout.String())
	}
}

type existingPRLookupCase struct {
	name       string
	prs        []pullRequestAPI
	detail     *pullRequestAPI
	data       string
	detailData string
	read       error
	detailRead error
	want       string
}

func (tt existingPRLookupCase) execute(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	if name != "gh" {
		t.Fatalf("unexpected command: %s %v", name, args)
	}
	switch strings.Join(args, " ") {
	case "api repos/goxdra/goxsd9/pulls?state=open&head=goxdra:agent/issue-13&per_page=100":
		return tt.listResponse()
	case "api repos/goxdra/goxsd9/pulls/14":
		return tt.detailResponse()
	default:
		t.Fatalf("unexpected command: %s %v", name, args)
		return "", nil
	}
}

func (tt existingPRLookupCase) listResponse() (string, error) {
	if tt.read != nil {
		return "", tt.read
	}
	if tt.data != "" {
		return tt.data, nil
	}
	encoded, err := json.Marshal(tt.prs)
	return string(encoded), err
}

func (tt existingPRLookupCase) detailResponse() (string, error) {
	if tt.detailRead != nil {
		return "", tt.detailRead
	}
	if tt.detailData != "" {
		return tt.detailData, nil
	}
	pr := tt.prs[0]
	if tt.detail != nil {
		pr = *tt.detail
	}
	encoded, err := json.Marshal(pr)
	return string(encoded), err
}

func assertExistingPRLookupConflict(t *testing.T, tt existingPRLookupCase, head string) {
	t.Helper()
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		return tt.execute(t, name, args...)
	}}
	_, found, err := application.existingOpenPullRequest("/repo", claimBranch(13), 13, head)
	if err == nil || found || !strings.Contains(err.Error(), tt.want) {
		t.Fatalf("lookup = found %t, error %v; want %q", found, err, tt.want)
	}
	if tt.read != nil && !errors.Is(err, tt.read) {
		t.Fatalf("lookup error %v lost cause %v", err, tt.read)
	}
	if tt.detailRead != nil && !errors.Is(err, tt.detailRead) {
		t.Fatalf("lookup error %v lost detail cause %v", err, tt.detailRead)
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
		}(), want: "differs from published head"},
		{name: "wrong base", prs: func() []pullRequestAPI { pr := valid; pr.Base.Ref = "other"; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "wrong base repository", prs: func() []pullRequestAPI {
			pr := valid
			pr.Base.Repo.FullName = "other/goxsd9"
			return []pullRequestAPI{pr}
		}(), want: "conflicting identity"},
		{name: "closed", prs: func() []pullRequestAPI { pr := valid; pr.State = "closed"; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "merged", prs: func() []pullRequestAPI { pr := valid; pr.Merged = true; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "missing number", prs: func() []pullRequestAPI { pr := valid; pr.Number = 0; return []pullRequestAPI{pr} }(), want: "no valid number"},
		{name: "missing URL", prs: func() []pullRequestAPI { pr := valid; pr.URL = ""; return []pullRequestAPI{pr} }(), want: "conflicting identity"},
		{name: "detail number", prs: []pullRequestAPI{valid}, detail: func() *pullRequestAPI {
			pr := valid
			pr.Number = 15
			return &pr
		}(), want: "conflicting identity"},
		{name: "detail malformed", prs: []pullRequestAPI{valid}, detailData: "{", want: "decode open PR #14"},
		{name: "detail transport", prs: []pullRequestAPI{valid}, detailRead: errors.New("detail unavailable"), want: "detail unavailable"},
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
