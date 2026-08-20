package workflowctl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type issueTestBlocker struct {
	number int
	id     string
}

type issueTestEdge struct {
	target  string
	blocker string
}

type issueInvalidBlockerTest struct {
	name string
	args []string
	want string
}

type issueCreateFixture struct {
	blockers             []issueTestBlocker
	bodyPath             string
	commands             []string
	identityNumbers      []int
	blockedByIDs         []string
	edges                []issueTestEdge
	failMutationAt       int
	mutationGraphQLError bool
}

func (f *issueCreateFixture) execute(_ string, _ io.Reader, name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	f.commands = append(f.commands, command)
	if name == "git" {
		if reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
			return "/repo", nil
		}
		return "", fmt.Errorf("unexpected git command: %s", command)
	}
	if name != "gh" {
		return "", fmt.Errorf("unexpected command: %s", command)
	}
	if reflect.DeepEqual(args, []string{
		"issue", "create", "--repo", repositoryKey, "--title", "new issue", "--body-file", f.bodyFile(),
		"--label", "area/workflow", "--label", "type/tooling",
	}) {
		return "https://github.com/goxdra/goxsd9/issues/101\n", nil
	}
	if reflect.DeepEqual(args, []string{"project", "item-add", "1", "--owner", owner,
		"--url", "https://github.com/goxdra/goxsd9/issues/101", "--format", "json"}) {
		return `{"id":"item-101"}`, nil
	}
	if reflect.DeepEqual(args, []string{"project", "field-list", "1", "--owner", owner, "--format", "json"}) {
		return `{"fields":[` +
			`{"id":"status-field","name":"Status","options":[{"id":"backlog-option","name":"Backlog"}]},` +
			`{"id":"priority-field","name":"Priority","options":[{"id":"p2-option","name":"P2"}]},` +
			`{"id":"effort-field","name":"Effort","options":[{"id":"s-option","name":"S"}]},` +
			`{"id":"phase-field","name":"Phase","options":[{"id":"bootstrap-option","name":"Bootstrap"}]}` +
			`]}`, nil
	}
	if argsMatch(args, []string{"project", "item-edit", "--project-id", projectID, "--id", "item-101",
		"--field-id", "status-field", "--single-select-option-id", "backlog-option"}) ||
		argsMatch(args, []string{"project", "item-edit", "--project-id", projectID, "--id", "item-101",
			"--field-id", "priority-field", "--single-select-option-id", "p2-option"}) ||
		argsMatch(args, []string{"project", "item-edit", "--project-id", projectID, "--id", "item-101",
			"--field-id", "effort-field", "--single-select-option-id", "s-option"}) ||
		argsMatch(args, []string{"project", "item-edit", "--project-id", projectID, "--id", "item-101",
			"--field-id", "phase-field", "--single-select-option-id", "bootstrap-option"}) {
		return "", nil
	}
	if len(args) < 2 || args[0] != "api" || args[1] != "graphql" {
		return "", fmt.Errorf("unexpected gh command: %s", command)
	}
	query := issueTestFlagValue(args, "query=")
	switch query {
	case issueIdentityQuery:
		return f.issueIdentity(args)
	case issueBlockedByQuery:
		return f.issueBlockedBy(args)
	case addBlockedByMutation:
		return f.addBlockedBy(args)
	default:
		return "", fmt.Errorf("unexpected GraphQL query: %q", query)
	}
}

func (f *issueCreateFixture) bodyFile() string {
	return f.bodyPath
}

func (f *issueCreateFixture) issueIdentity(args []string) (string, error) {
	want := make([]string, 0, 10)
	want = append(want, "api", "graphql", "-f", "query="+issueIdentityQuery,
		"-f", "owner="+owner, "-f", "repository="+repository)
	numberText := issueTestFlagValue(args, "number=")
	number, err := strconv.Atoi(numberText)
	if err != nil {
		return "", fmt.Errorf("decode identity number %q: %w", numberText, err)
	}
	want = append(want, "-F", "number="+numberText)
	if !reflect.DeepEqual(args, want) {
		return "", fmt.Errorf("identity arguments = %#v, want %#v", args, want)
	}
	f.identityNumbers = append(f.identityNumbers, number)
	if number == 101 {
		return issueIdentityJSON("repo-id", "target-id", 101), nil
	}
	for _, blocker := range f.blockers {
		if blocker.number == number {
			return issueIdentityJSON("repo-id", blocker.id, blocker.number), nil
		}
	}
	return `{"data":{"repository":{"id":"repo-id","issue":null}}}`, nil
}

func (f *issueCreateFixture) issueBlockedBy(args []string) (string, error) {
	want := []string{"api", "graphql", "-f", "query=" + issueBlockedByQuery,
		"-f", "owner=" + owner, "-f", "repository=" + repository, "-F", "number=101"}
	if !reflect.DeepEqual(args, want) {
		return "", fmt.Errorf("blocked-by arguments = %#v, want %#v", args, want)
	}
	nodes := make([]string, 0, len(f.blockedByIDs))
	for _, id := range f.blockedByIDs {
		nodes = append(nodes, fmt.Sprintf(`{"id":%q}`, id))
	}
	return fmt.Sprintf(`{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[%s],"totalCount":%d,"pageInfo":{"hasNextPage":false}}}}}}`,
		strings.Join(nodes, ","), len(f.blockedByIDs)), nil
}

func (f *issueCreateFixture) addBlockedBy(args []string) (string, error) {
	targetID := issueTestFlagValue(args, "issueId=")
	blockerID := issueTestFlagValue(args, "blockingIssueId=")
	want := []string{"api", "graphql", "-f", "query=" + addBlockedByMutation,
		"-f", "issueId=" + targetID, "-f", "blockingIssueId=" + blockerID}
	if !reflect.DeepEqual(args, want) {
		return "", fmt.Errorf("mutation arguments = %#v, want %#v", args, want)
	}
	f.edges = append(f.edges, issueTestEdge{target: targetID, blocker: blockerID})
	if f.failMutationAt == len(f.edges) {
		if f.mutationGraphQLError {
			return `{"errors":[{"message":"edge rejected"}],"data":{"addBlockedBy":null}}`, nil
		}
		return "", errors.New("edge rejected")
	}
	f.blockedByIDs = append(f.blockedByIDs, blockerID)
	return fmt.Sprintf(`{"data":{"addBlockedBy":{"issue":{"id":%q},"blockingIssue":{"id":%q}}}}`,
		targetID, blockerID), nil
}

func argsMatch(got, want []string) bool {
	return reflect.DeepEqual(got, want)
}

func issueTestFlagValue(args []string, prefix string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "-f" || args[index] == "-F" {
			if strings.HasPrefix(args[index+1], prefix) {
				return strings.TrimPrefix(args[index+1], prefix)
			}
		}
	}
	return ""
}

func issueIdentityJSON(repositoryID, issueID string, number int) string {
	return fmt.Sprintf(`{"data":{"repository":{"id":%q,"issue":{"id":%q,"number":%d}}}}`,
		repositoryID, issueID, number)
}

func writeIssueTestBody(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("## Acceptance\n\nProof.\n"), 0o600); err != nil {
		t.Fatalf("write issue body: %v", err)
	}
}

func issueCreateArgs(bodyFile string, blockers ...int) []string {
	args := make([]string, 0, 8+2*len(blockers))
	args = append(args, "--title", "new issue", "--body-file", bodyFile, "--area", "workflow", "--type", "tooling")
	for _, blocker := range blockers {
		args = append(args, "--blocked-by", strconv.Itoa(blocker))
	}
	return args
}

func TestCreateIssueUsesNativeGraphQLDependenciesInFlagOrder(t *testing.T) {
	tests := []struct {
		name     string
		blockers []issueTestBlocker
		ordered  []int
	}{
		{name: "one blocker", blockers: []issueTestBlocker{{number: 7, id: "blocker-7"}}, ordered: []int{7}},
		{name: "multiple blockers", blockers: []issueTestBlocker{{number: 7, id: "blocker-7"}, {number: 3, id: "blocker-3"}}, ordered: []int{7, 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runNativeIssueCreateFixture(t, test.blockers, test.ordered) })
	}
}

func TestCreateIssueWithoutBlockersSkipsGraphQLTargetResolution(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "issue.md")
	writeIssueTestBody(t, bodyFile)
	fixture := &issueCreateFixture{bodyPath: bodyFile}
	graphqlCalls := 0
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "graphql" {
			graphqlCalls++
			return "", errors.New("GraphQL should not be called without blockers")
		}
		return fixture.execute(dir, input, name, args...)
	}}
	if err := application.createIssue(issueCreateArgs(bodyFile)); err != nil {
		t.Fatalf("createIssue without blockers: %v", err)
	}
	if graphqlCalls != 0 {
		t.Fatalf("no-blocker creation invoked GraphQL %d time(s)", graphqlCalls)
	}
	if got, want := stdout.String(), "https://github.com/goxdra/goxsd9/issues/101\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	assertProjectOrder(t, fixture.commands)
}

func runNativeIssueCreateFixture(t *testing.T, blockers []issueTestBlocker, ordered []int) {
	t.Helper()
	bodyFile := filepath.Join(t.TempDir(), "issue.md")
	writeIssueTestBody(t, bodyFile)
	fixture := &issueCreateFixture{blockers: blockers, bodyPath: bodyFile}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: fixture.execute}
	if err := application.createIssue(issueCreateArgs(bodyFile, ordered...)); err != nil {
		t.Fatalf("createIssue: %v", err)
	}
	if got, want := stdout.String(), "https://github.com/goxdra/goxsd9/issues/101\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	assertIdentityOrder(t, fixture.identityNumbers, ordered)
	assertNativeEdges(t, fixture.edges, blockers)
	assertNoLegacyDependencyCommand(t, fixture.commands)
	assertProjectOrder(t, fixture.commands)
}

func assertIdentityOrder(t *testing.T, got []int, ordered []int) {
	t.Helper()
	want := append(append([]int{}, ordered...), 101)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity resolution order = %#v, want blockers then target %#v", got, want)
	}
}

func assertNativeEdges(t *testing.T, got []issueTestEdge, blockers []issueTestBlocker) {
	t.Helper()
	want := make([]issueTestEdge, 0, len(blockers))
	for _, blocker := range blockers {
		want = append(want, issueTestEdge{target: "target-id", blocker: blocker.id})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native edges = %#v, want %#v", got, want)
	}
}

func assertNoLegacyDependencyCommand(t *testing.T, commands []string) {
	t.Helper()
	for _, command := range commands {
		if strings.Contains(command, "issue edit") || strings.Contains(command, "--add-blocked-by") {
			t.Fatalf("unsupported dependency command was invoked: %s", command)
		}
	}
}

func assertProjectOrder(t *testing.T, commands []string) {
	t.Helper()
	projectOrder := []string{
		"gh project item-add 1 --owner goxdra --url https://github.com/goxdra/goxsd9/issues/101 --format json",
		"gh project field-list 1 --owner goxdra --format json",
		"gh project item-edit --project-id " + projectID + " --id item-101 --field-id status-field --single-select-option-id backlog-option",
		"gh project item-edit --project-id " + projectID + " --id item-101 --field-id priority-field --single-select-option-id p2-option",
		"gh project item-edit --project-id " + projectID + " --id item-101 --field-id effort-field --single-select-option-id s-option",
		"gh project item-edit --project-id " + projectID + " --id item-101 --field-id phase-field --single-select-option-id bootstrap-option",
	}
	assertOrderedCommands(t, commands, projectOrder)
}

func assertOrderedCommands(t *testing.T, commands, expected []string) {
	t.Helper()
	position := 0
	for _, want := range expected {
		found := false
		for position < len(commands) {
			if commands[position] == want {
				position++
				found = true
				break
			}
			position++
		}
		if !found {
			t.Fatalf("command %q was not found in order in %#v", want, commands)
		}
	}
}

func TestCreateIssueRejectsInvalidBlockersBeforeMutation(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "issue.md")
	writeIssueTestBody(t, bodyFile)
	tests := make([]issueInvalidBlockerTest, 0, 3)
	tests = append(tests,
		issueInvalidBlockerTest{name: "malformed", args: issueCreateArgs(bodyFile, 0), want: `invalid issue number "0"`},
		issueInvalidBlockerTest{name: "duplicate", args: issueCreateArgs(bodyFile, 7, 7), want: "duplicate --blocked-by issue #7"})
	tooMany := issueCreateArgs(bodyFile)
	for index := 1; index <= issueDependencyLimit+1; index++ {
		tooMany = append(tooMany, "--blocked-by", strconv.Itoa(index))
	}
	tests = append(tests, issueInvalidBlockerTest{name: "relationship limit", args: tooMany, want: "limit is 50"})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands := 0
			application := app{executeCommand: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
				commands++
				return "", errors.New("mutation was reached")
			}}
			err := application.createIssue(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("createIssue error = %v, want %q", err, test.want)
			}
			if commands != 0 {
				t.Fatalf("invalid blocker input reached %d command(s)", commands)
			}
		})
	}
}

func TestCreateIssueMissingBlockerDoesNotCreateIssueOrProjectItem(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "issue.md")
	writeIssueTestBody(t, bodyFile)
	fixture := &issueCreateFixture{bodyPath: bodyFile}
	application := app{executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if name == "gh" && len(args) >= 2 && args[0] == "api" && args[1] == "graphql" && issueTestFlagValue(args, "query=") == issueIdentityQuery {
			return `{"data":{"repository":{"id":"repo-id","issue":null}}}`, nil
		}
		return fixture.execute(dir, input, name, args...)
	}}
	err := application.createIssue(issueCreateArgs(bodyFile, 88))
	if err == nil || !strings.Contains(err.Error(), "resolve blocker #88") || !strings.Contains(err.Error(), "no issue") {
		t.Fatalf("createIssue error = %v, want missing blocker context", err)
	}
	for _, command := range fixture.commands {
		if strings.Contains(command, "issue create") || strings.Contains(command, "project") {
			t.Fatalf("missing blocker reached mutation command: %s", command)
		}
	}
}

func TestConfigureNewIssueRejectsSelfPairBeforeTargetOrProjectMutation(t *testing.T) {
	commands := 0
	application := app{executeCommand: func(_ string, _ io.Reader, _ string, _ ...string) (string, error) {
		commands++
		return "", errors.New("mutation was reached")
	}}
	err := application.configureNewIssue("/repo", "https://github.com/goxdra/goxsd9/issues/9", 9,
		"P2", "S", "Bootstrap", "Backlog", []int{9}, []issueNodeIdentity{{
			repositoryID: "repo-id",
			issueID:      "issue-9",
			number:       9,
		}})
	if err == nil || !strings.Contains(err.Error(), "edge 1") || !strings.Contains(err.Error(), "self-pair") {
		t.Fatalf("configureNewIssue error = %v, want self-pair context", err)
	}
	if commands != 0 {
		t.Fatalf("self-pair reached %d command(s)", commands)
	}
}

func TestCreateIssuePreservesURLAndPartialDependencyFailure(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "issue.md")
	writeIssueTestBody(t, bodyFile)
	fixture := &issueCreateFixture{
		blockers:             []issueTestBlocker{{number: 7, id: "blocker-7"}, {number: 3, id: "blocker-3"}},
		failMutationAt:       2,
		mutationGraphQLError: true,
	}
	var stdout bytes.Buffer
	application := app{stdout: &stdout, executeCommand: func(dir string, input io.Reader, name string, args ...string) (string, error) {
		if name == "gh" && len(args) >= 3 && args[0] == "issue" && args[1] == "create" {
			return "https://github.com/goxdra/goxsd9/issues/101\n", nil
		}
		return fixture.execute(dir, input, name, args...)
	}}
	err := application.createIssue(issueCreateArgs(bodyFile, 7, 3))
	if err == nil {
		t.Fatal("partial dependency failure was reported as success")
	}
	for _, want := range []string{
		"issue created at https://github.com/goxdra/goxsd9/issues/101",
		"incomplete operation",
		"edge 2",
		"blocked by #3",
		"edge rejected",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want context %q", err, want)
		}
	}
	if !reflect.DeepEqual(fixture.edges, []issueTestEdge{{target: "target-id", blocker: "blocker-7"}, {target: "target-id", blocker: "blocker-3"}}) {
		t.Fatalf("edge attempts = %#v", fixture.edges)
	}
	if !reflect.DeepEqual(fixture.blockedByIDs, []string{"blocker-7"}) {
		t.Fatalf("successful prior edge was not preserved: %#v", fixture.blockedByIDs)
	}
	if stdout.Len() != 0 {
		t.Fatalf("partial failure wrote success output %q", stdout.String())
	}
}

type issueIdempotenceFixture struct {
	blockedReads int
	mutations    int
}

func (f *issueIdempotenceFixture) execute(_ string, _ io.Reader, name string, args ...string) (string, error) {
	if name != "gh" || len(args) < 3 || args[0] != "api" || args[1] != "graphql" {
		return "", fmt.Errorf("unexpected command %s %v", name, args)
	}
	query := issueTestFlagValue(args, "query=")
	if query == issueBlockedByQuery {
		return f.readBlockedBy(args)
	}
	if query == addBlockedByMutation {
		return f.writeBlockedBy(args)
	}
	return "", fmt.Errorf("unexpected GraphQL query %q", query)
}

func (f *issueIdempotenceFixture) readBlockedBy(args []string) (string, error) {
	want := []string{"api", "graphql", "-f", "query=" + issueBlockedByQuery,
		"-f", "owner=" + owner, "-f", "repository=" + repository, "-F", "number=101"}
	if !reflect.DeepEqual(args, want) {
		return "", fmt.Errorf("blocked-by arguments = %#v, want %#v", args, want)
	}
	f.blockedReads++
	if f.blockedReads == 1 {
		return `{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[],"totalCount":0,"pageInfo":{"hasNextPage":false}}}}}}`, nil
	}
	return `{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[{"id":"blocker-id"}],"totalCount":1,"pageInfo":{"hasNextPage":false}}}}}}`, nil
}

func (f *issueIdempotenceFixture) writeBlockedBy(args []string) (string, error) {
	want := []string{"api", "graphql", "-f", "query=" + addBlockedByMutation,
		"-f", "issueId=target-id", "-f", "blockingIssueId=blocker-id"}
	if !reflect.DeepEqual(args, want) {
		return "", fmt.Errorf("mutation arguments = %#v, want %#v", args, want)
	}
	f.mutations++
	return `{"data":{"addBlockedBy":{"issue":{"id":"target-id"},"blockingIssue":{"id":"blocker-id"}}}}`, nil
}

func TestEnsureIssueDependencyIsIdempotent(t *testing.T) {
	target := issueNodeIdentity{repositoryID: "repo-id", issueID: "target-id", number: 101}
	blocker := issueNodeIdentity{repositoryID: "repo-id", issueID: "blocker-id", number: 7}
	fixture := &issueIdempotenceFixture{}
	application := app{executeCommand: fixture.execute}
	if err := application.ensureIssueDependency("/repo", target, blocker); err != nil {
		t.Fatalf("first ensureIssueDependency: %v", err)
	}
	if err := application.ensureIssueDependency("/repo", target, blocker); err != nil {
		t.Fatalf("second ensureIssueDependency: %v", err)
	}
	if fixture.blockedReads != 2 || fixture.mutations != 1 {
		t.Fatalf("blocked reads = %d, mutations = %d, want 2 and 1", fixture.blockedReads, fixture.mutations)
	}
}

type incompletePaginationFixture struct {
	body      string
	mutations int
}

func (f *incompletePaginationFixture) execute(_ string, _ io.Reader, name string, args ...string) (string, error) {
	if name != "gh" || len(args) < 3 || args[0] != "api" || args[1] != "graphql" {
		return "", fmt.Errorf("unexpected command %s %v", name, args)
	}
	switch issueTestFlagValue(args, "query=") {
	case issueBlockedByQuery:
		return f.body, nil
	case addBlockedByMutation:
		f.mutations++
		return `{"data":{"addBlockedBy":{"issue":{"id":"target-id"},"blockingIssue":{"id":"blocker-id"}}}}`, nil
	default:
		return "", fmt.Errorf("unexpected GraphQL query %q", issueTestFlagValue(args, "query="))
	}
}

func TestEnsureIssueDependencyRejectsMissingPaginationBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "omitted hasNextPage",
			body: `{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[],"totalCount":0,"pageInfo":{}}}}}}`,
		},
		{
			name: "null hasNextPage",
			body: `{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[],"totalCount":0,"pageInfo":{"hasNextPage":null}}}}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runMissingPaginationFixture(t, test.body) })
	}
}

func runMissingPaginationFixture(t *testing.T, body string) {
	t.Helper()
	target := issueNodeIdentity{repositoryID: "repo-id", issueID: "target-id", number: 101}
	blocker := issueNodeIdentity{repositoryID: "repo-id", issueID: "blocker-id", number: 7}
	fixture := &incompletePaginationFixture{body: body}
	application := app{executeCommand: fixture.execute}
	err := application.ensureIssueDependency("/repo", target, blocker)
	if err == nil || !strings.Contains(err.Error(), "omitted pagination proof") {
		t.Fatalf("ensureIssueDependency error = %v, want missing pagination proof", err)
	}
	if fixture.mutations != 0 {
		t.Fatalf("incomplete pagination reached addBlockedBy %d time(s)", fixture.mutations)
	}
}

func TestReadIssueBlockedByRejectsIncompletePagination(t *testing.T) {
	target := issueNodeIdentity{repositoryID: "repo-id", issueID: "target-id", number: 101}
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "next page",
			body: `{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[],"totalCount":0,"pageInfo":{"hasNextPage":true}}}}}}`,
			want: "incomplete",
		},
		{
			name: "over limit",
			body: `{"data":{"repository":{"id":"repo-id","issue":{"id":"target-id","number":101,"blockedBy":{"nodes":[],"totalCount":51,"pageInfo":{"hasNextPage":false}}}}}}`,
			want: "unsupported relationship count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				if name != "gh" || issueTestFlagValue(args, "query=") != issueBlockedByQuery {
					return "", fmt.Errorf("unexpected command %s %v", name, args)
				}
				return test.body, nil
			}}
			_, err := application.readIssueBlockedBy("/repo", target)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readIssueBlockedBy error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAddBlockedByRejectsGraphQLErrorsAndMismatchedPayload(t *testing.T) {
	target := issueNodeIdentity{repositoryID: "repo-id", issueID: "target-id", number: 101}
	blocker := issueNodeIdentity{repositoryID: "repo-id", issueID: "blocker-id", number: 7}
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "GraphQL error",
			body: `{"errors":[{"message":"permission denied"}],"data":{"addBlockedBy":null}}`,
			want: "permission denied",
		},
		{
			name: "null data",
			body: `{"data":null}`,
			want: "null data",
		},
		{
			name: "null payload",
			body: `{"data":{"addBlockedBy":null}}`,
			want: "null payload",
		},
		{
			name: "mismatched target",
			body: `{"data":{"addBlockedBy":{"issue":{"id":"other-target"},"blockingIssue":{"id":"blocker-id"}}}}`,
			want: "other-target",
		},
		{
			name: "mismatched blocker",
			body: `{"data":{"addBlockedBy":{"issue":{"id":"target-id"},"blockingIssue":{"id":"other-blocker"}}}}`,
			want: "other-blocker",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				if name != "gh" || issueTestFlagValue(args, "query=") != addBlockedByMutation {
					return "", fmt.Errorf("unexpected command %s %v", name, args)
				}
				return test.body, nil
			}}
			err := application.addBlockedBy("/repo", target, blocker)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("addBlockedBy error = %v, want %q", err, test.want)
			}
		})
	}
}
