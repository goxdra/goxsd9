package workflowctl

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type intValues []int

const issueDependencyLimit = 50

type issueNodeIdentity struct {
	repositoryID string
	issueID      string
	number       int
}

type graphqlIssueError struct {
	Message string `json:"message"`
}

type issueIdentityResponse struct {
	Data *struct {
		Repository *struct {
			ID    string `json:"id"`
			Issue *struct {
				ID     string `json:"id"`
				Number int    `json:"number"`
			} `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
	Errors []graphqlIssueError `json:"errors"`
}

type issueBlockedByNode struct {
	ID string `json:"id"`
}

type issueBlockedByPageInfo struct {
	HasNextPage *bool `json:"hasNextPage"`
}

type issueBlockedByConnection struct {
	Nodes      []issueBlockedByNode    `json:"nodes"`
	TotalCount *int                    `json:"totalCount"`
	PageInfo   *issueBlockedByPageInfo `json:"pageInfo"`
}

type issueBlockedByIssue struct {
	ID        string                    `json:"id"`
	Number    int                       `json:"number"`
	BlockedBy *issueBlockedByConnection `json:"blockedBy"`
}

type issueBlockedByRepository struct {
	ID    string               `json:"id"`
	Issue *issueBlockedByIssue `json:"issue"`
}

type issueBlockedByResponse struct {
	Data *struct {
		Repository *issueBlockedByRepository `json:"repository"`
	} `json:"data"`
	Errors []graphqlIssueError `json:"errors"`
}

type addBlockedByResponse struct {
	Data *struct {
		AddBlockedBy *struct {
			Issue *struct {
				ID string `json:"id"`
			} `json:"issue"`
			BlockingIssue *struct {
				ID string `json:"id"`
			} `json:"blockingIssue"`
		} `json:"addBlockedBy"`
	} `json:"data"`
	Errors []graphqlIssueError `json:"errors"`
}

var (
	validAreas    = []string{"codegen", "datatypes", "docs", "parser", "resolver", "schema", "specs", "validator", "workflow", "xpath"}
	validTypes    = []string{"bug", "conformance", "docs", "feature", "refactor", "research", "tooling"}
	validEfforts  = []string{"XS", "S", "M", "L", "XL"}
	validPhases   = []string{"Bootstrap", "Vertical Slice", "Schema Model", "Validation", "Codegen", "Conformance", "XPath"}
	validStatuses = []string{"Backlog", "Ready"}
)

func (values *intValues) String() string {
	parts := make([]string, 0, len(*values))
	for _, value := range *values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func (values *intValues) Set(text string) error {
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 {
		return fmt.Errorf("invalid issue number %q", text)
	}
	*values = append(*values, value)
	return nil
}

func (a app) runIssue(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return usageError("usage: workflowctl issue create [flags]")
	}
	return a.createIssue(args[1:])
}

func (a app) createIssue(args []string) error {
	flags := flag.NewFlagSet("issue create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	title := flags.String("title", "", "issue title")
	bodyFile := flags.String("body-file", "", "Markdown body file")
	area := flags.String("area", "", "area label suffix")
	typeName := flags.String("type", "", "type label suffix")
	priority := flags.String("priority", "P2", "Project priority")
	effort := flags.String("effort", "S", "Project effort")
	phase := flags.String("phase", "Bootstrap", "Project phase")
	status := flags.String("status", "Backlog", "Project status")
	var blockedBy intValues
	flags.Var(&blockedBy, "blocked-by", "blocking issue number; repeatable")
	if err := flags.Parse(args); err != nil {
		return usageError("issue create: %v", err)
	}
	if flags.NArg() != 0 {
		return usageError("issue create takes flags only")
	}
	if err := validateIssueInput(*title, *bodyFile, *area, *typeName, *priority, *effort, *phase, *status); err != nil {
		return usageError("issue create: %v", err)
	}
	if err := validateBlockedBy(blockedBy); err != nil {
		return usageError("issue create: %v", err)
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	blockerIdentities, err := a.resolveBlockerIdentities(root, blockedBy)
	if err != nil {
		return fmt.Errorf("issue phase: resolve blockers before issue creation: %w", err)
	}
	url, err := a.createGitHubIssue(root, *title, *bodyFile, *area, *typeName)
	if err != nil {
		return err
	}
	number, err := issueNumberFromURL(url)
	if err != nil {
		return fmt.Errorf("issue created at %s but its number was not understood: %w", url, err)
	}
	if err := a.configureNewIssue(root, url, number, *priority, *effort, *phase, *status, blockedBy, blockerIdentities); err != nil {
		return fmt.Errorf("issue created at %s; incomplete operation: %w", url, err)
	}
	return writeLine(a.stdout, "%s", url)
}

func validateBlockedBy(blockedBy []int) error {
	if len(blockedBy) > issueDependencyLimit {
		return fmt.Errorf("too many --blocked-by values: got %d, limit is %d", len(blockedBy), issueDependencyLimit)
	}
	seen := make(map[int]struct{}, len(blockedBy))
	for index, blocker := range blockedBy {
		if blocker < 1 {
			return fmt.Errorf("invalid --blocked-by value %d at position %d", blocker, index+1)
		}
		if _, ok := seen[blocker]; ok {
			return fmt.Errorf("duplicate --blocked-by issue #%d at position %d", blocker, index+1)
		}
		seen[blocker] = struct{}{}
	}
	return nil
}

func validateIssueInput(title, bodyFile, area, typeName, priority, effort, phase, status string) error {
	if strings.TrimSpace(title) == "" || bodyFile == "" || area == "" || typeName == "" {
		return errors.New("--title, --body-file, --area, and --type are required")
	}
	if err := requireRegularFile(bodyFile); err != nil {
		return err
	}
	// #nosec G304 -- bodyFile is an explicit operator-supplied input.
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		return fmt.Errorf("read issue body: %w", err)
	}
	if !strings.Contains(string(body), "## Acceptance") {
		return errors.New("issue body must contain an Acceptance section")
	}
	if priorityRank(priority) > 4 {
		return fmt.Errorf("invalid priority %q", priority)
	}
	if !containsString(validAreas, area) {
		return fmt.Errorf("invalid area %q", area)
	}
	if !containsString(validTypes, typeName) {
		return fmt.Errorf("invalid type %q", typeName)
	}
	if !containsString(validEfforts, effort) {
		return fmt.Errorf("invalid effort %q", effort)
	}
	if !containsString(validPhases, phase) {
		return fmt.Errorf("invalid phase %q", phase)
	}
	if !containsString(validStatuses, status) {
		return fmt.Errorf("invalid status %q", status)
	}
	if effortRank(effort) > 2 && status == "Ready" {
		return errors.New("ready issues must be XS, S, or M")
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a app) createGitHubIssue(root, title, bodyFile, area, typeName string) (string, error) {
	output, err := a.command(root, "gh", "issue", "create", "--repo", repositoryKey, "--title", title,
		"--body-file", bodyFile, "--label", "area/"+area, "--label", "type/"+typeName)
	if err != nil {
		return "", fmt.Errorf("create issue: %w", err)
	}
	return firstLine(output), nil
}

func (a app) configureNewIssue(root, url string, number int, priority, effort, phase, status string,
	blockedBy []int, resolved ...[]issueNodeIdentity,
) error {
	if len(resolved) > 1 {
		return errors.New("dependency phase: received more than one resolved blocker list")
	}
	blockerIdentities := []issueNodeIdentity(nil)
	if len(resolved) == 1 {
		blockerIdentities = resolved[0]
	}
	if len(resolved) == 0 {
		if err := validateBlockedBy(blockedBy); err != nil {
			return fmt.Errorf("dependency phase: %w", err)
		}
		for index, blocker := range blockedBy {
			if blocker == number {
				return fmt.Errorf("dependency phase: edge %d blocked by #%d is a self-pair", index+1, blocker)
			}
		}
		var err error
		blockerIdentities, err = a.resolveBlockerIdentities(root, blockedBy)
		if err != nil {
			return fmt.Errorf("dependency phase: resolve blockers: %w", err)
		}
	}
	return a.configureNewIssueResolved(root, url, number, priority, effort, phase, status, blockedBy, blockerIdentities)
}

func (a app) configureNewIssueResolved(root, url string, number int, priority, effort, phase, status string,
	blockedBy []int, blockerIdentities []issueNodeIdentity,
) error {
	if err := validateResolvedBlockers(number, blockedBy, blockerIdentities); err != nil {
		return err
	}
	target, err := a.resolveTargetForDependencies(root, number, blockedBy, blockerIdentities)
	if err != nil {
		return err
	}
	output, err := a.command(root, "gh", "project", "item-add", strconv.Itoa(projectNumber), "--owner", owner,
		"--url", url, "--format", "json")
	if err != nil {
		return fmt.Errorf("project phase: add issue to Project: %w", err)
	}
	var item projectItem
	if decodeErr := json.Unmarshal([]byte(output), &item); decodeErr != nil {
		return fmt.Errorf("project phase: decode added Project item: %w", decodeErr)
	}
	if strings.TrimSpace(item.ID) == "" {
		return errors.New("project phase: add issue to Project returned no item ID")
	}
	fields, err := a.projectFields(root)
	if err != nil {
		return fmt.Errorf("project phase: read fields: %w", err)
	}
	values := []struct{ field, option string }{
		{field: "Status", option: status},
		{field: "Priority", option: priority},
		{field: "Effort", option: effort},
		{field: "Phase", option: phase},
	}
	for _, value := range values {
		if err := a.setProjectFieldFromList(root, fields, item.ID, value.field, value.option); err != nil {
			return fmt.Errorf("project phase: set %s=%s: %w", value.field, value.option, err)
		}
	}
	for index, blocker := range blockedBy {
		if err := a.ensureIssueDependency(root, target, blockerIdentities[index]); err != nil {
			return fmt.Errorf("dependency phase: edge %d blocked by #%d: %w", index+1, blocker, err)
		}
	}
	return nil
}

func (a app) resolveTargetForDependencies(root string, number int, blockedBy []int,
	blockerIdentities []issueNodeIdentity,
) (issueNodeIdentity, error) {
	if len(blockedBy) == 0 {
		return issueNodeIdentity{}, nil
	}
	target, err := a.resolveIssueIdentity(root, number)
	if err != nil {
		return issueNodeIdentity{}, fmt.Errorf("issue phase: resolve created issue #%d node: %w", number, err)
	}
	if targetErr := validateTargetBlockers(target, blockedBy, blockerIdentities); targetErr != nil {
		return issueNodeIdentity{}, targetErr
	}
	return target, nil
}

func validateResolvedBlockers(number int, blockedBy []int, identities []issueNodeIdentity) error {
	if err := validateBlockedBy(blockedBy); err != nil {
		return fmt.Errorf("dependency phase: %w", err)
	}
	if len(blockedBy) != len(identities) {
		return fmt.Errorf("dependency phase: resolved blocker count %d does not match requested count %d",
			len(identities), len(blockedBy))
	}
	for index, blocker := range blockedBy {
		if blocker == number {
			return fmt.Errorf("dependency phase: edge %d blocked by #%d is a self-pair", index+1, blocker)
		}
		identity := identities[index]
		if identity.number != blocker {
			return fmt.Errorf("dependency phase: edge %d blocked by #%d resolved to issue #%d",
				index+1, blocker, identity.number)
		}
		if strings.TrimSpace(identity.repositoryID) == "" || strings.TrimSpace(identity.issueID) == "" {
			return fmt.Errorf("dependency phase: edge %d blocked by #%d has an incomplete node identity",
				index+1, blocker)
		}
	}
	return nil
}

func validateTargetBlockers(target issueNodeIdentity, blockedBy []int, identities []issueNodeIdentity) error {
	for index, blocker := range blockedBy {
		identity := identities[index]
		if identity.repositoryID != target.repositoryID {
			return fmt.Errorf("dependency phase: edge %d blocked by #%d belongs to repository %q, target belongs to %q",
				index+1, blocker, identity.repositoryID, target.repositoryID)
		}
		if identity.issueID == target.issueID {
			return fmt.Errorf("dependency phase: edge %d blocked by #%d is a self-pair", index+1, blocker)
		}
	}
	return nil
}

func (a app) resolveBlockerIdentities(root string, blockedBy []int) ([]issueNodeIdentity, error) {
	identities := make([]issueNodeIdentity, 0, len(blockedBy))
	var repositoryID string
	for index, blocker := range blockedBy {
		identity, err := a.resolveIssueIdentity(root, blocker)
		if err != nil {
			return nil, fmt.Errorf("resolve blocker #%d at flag position %d: %w", blocker, index+1, err)
		}
		if repositoryID == "" {
			repositoryID = identity.repositoryID
		}
		if identity.repositoryID != repositoryID {
			return nil, fmt.Errorf("resolve blocker #%d at flag position %d: repository identity %q differs from %q",
				blocker, index+1, identity.repositoryID, repositoryID)
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

func (a app) resolveIssueIdentity(root string, number int) (issueNodeIdentity, error) {
	output, err := a.command(root, "gh", "api", "graphql", "-f", "query="+issueIdentityQuery,
		"-f", "owner="+owner, "-f", "repository="+repository, "-F", "number="+strconv.Itoa(number))
	if err != nil {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity: %w", number, err)
	}
	var response issueIdentityResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return issueNodeIdentity{}, fmt.Errorf("decode issue #%d identity: %w", number, err)
	}
	if err := graphQLErrors(response.Errors); err != nil {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity: %w", number, err)
	}
	if response.Data == nil {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned null data", number)
	}
	if response.Data.Repository == nil {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned null repository", number)
	}
	if strings.TrimSpace(response.Data.Repository.ID) == "" {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned no repository ID", number)
	}
	if response.Data.Repository.Issue == nil {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned no issue", number)
	}
	issue := response.Data.Repository.Issue
	if strings.TrimSpace(issue.ID) == "" {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned no issue ID", number)
	}
	if issue.Number < 1 {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned invalid issue number %d", number, issue.Number)
	}
	if issue.Number != number {
		return issueNodeIdentity{}, fmt.Errorf("read issue #%d identity returned issue #%d", number, issue.Number)
	}
	return issueNodeIdentity{
		repositoryID: response.Data.Repository.ID,
		issueID:      issue.ID,
		number:       issue.Number,
	}, nil
}

func graphQLErrors(items []graphqlIssueError) error {
	if len(items) == 0 {
		return nil
	}
	messages := make([]string, 0, len(items))
	for _, item := range items {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			message = "unspecified error"
		}
		messages = append(messages, message)
	}
	return fmt.Errorf("GitHub GraphQL returned %d error(s): %s", len(items), strings.Join(messages, "; "))
}

func (a app) ensureIssueDependency(root string, target, blocker issueNodeIdentity) error {
	if target.issueID == blocker.issueID {
		return errors.New("target and blocker node IDs are identical")
	}
	if target.repositoryID != blocker.repositoryID {
		return fmt.Errorf("target repository %q differs from blocker repository %q", target.repositoryID, blocker.repositoryID)
	}
	blockedBy, err := a.readIssueBlockedBy(root, target)
	if err != nil {
		return fmt.Errorf("read target blocked-by IDs: %w", err)
	}
	for _, issueID := range blockedBy {
		if issueID == blocker.issueID {
			return nil
		}
	}
	if err := a.addBlockedBy(root, target, blocker); err != nil {
		return fmt.Errorf("write target blocked-by edge: %w", err)
	}
	return nil
}

func (a app) readIssueBlockedBy(root string, target issueNodeIdentity) ([]string, error) {
	output, err := a.command(root, "gh", "api", "graphql", "-f", "query="+issueBlockedByQuery,
		"-f", "owner="+owner, "-f", "repository="+repository, "-F", "number="+strconv.Itoa(target.number))
	if err != nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs: %w", target.number, err)
	}
	var response issueBlockedByResponse
	if decodeErr := json.Unmarshal([]byte(output), &response); decodeErr != nil {
		return nil, fmt.Errorf("decode issue #%d blocked-by IDs: %w", target.number, decodeErr)
	}
	if graphqlErr := graphQLErrors(response.Errors); graphqlErr != nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs: %w", target.number, graphqlErr)
	}
	connection, err := validateIssueBlockedByTarget(response, target)
	if err != nil {
		return nil, err
	}
	return issueBlockedByIDs(target.number, connection)
}

func validateIssueBlockedByTarget(response issueBlockedByResponse, target issueNodeIdentity) (*issueBlockedByConnection, error) {
	if response.Data == nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned null data", target.number)
	}
	if response.Data.Repository == nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned null repository", target.number)
	}
	repository := response.Data.Repository
	if repository.ID == "" {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned no repository ID", target.number)
	}
	if repository.ID != target.repositoryID {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned repository %q, want %q",
			target.number, repository.ID, target.repositoryID)
	}
	if repository.Issue == nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned no issue", target.number)
	}
	issue := repository.Issue
	if issue.ID == "" {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned no issue ID", target.number)
	}
	if issue.ID != target.issueID {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned issue %q, want %q",
			target.number, issue.ID, target.issueID)
	}
	if issue.Number != target.number {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned issue #%d", target.number, issue.Number)
	}
	if issue.BlockedBy == nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs returned null connection", target.number)
	}
	return issue.BlockedBy, nil
}

func issueBlockedByIDs(number int, connection *issueBlockedByConnection) ([]string, error) {
	if connection.TotalCount == nil || connection.PageInfo == nil || connection.PageInfo.HasNextPage == nil {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs omitted pagination proof", number)
	}
	if *connection.TotalCount < 0 || *connection.TotalCount > issueDependencyLimit {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs reported unsupported relationship count %d (limit %d)",
			number, *connection.TotalCount, issueDependencyLimit)
	}
	if *connection.PageInfo.HasNextPage || *connection.TotalCount != len(connection.Nodes) {
		return nil, fmt.Errorf("read issue #%d blocked-by IDs was incomplete: total %d, returned %d, hasNextPage=%t",
			number, *connection.TotalCount, len(connection.Nodes), *connection.PageInfo.HasNextPage)
	}
	ids := make([]string, 0, len(connection.Nodes))
	for index, node := range connection.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return nil, fmt.Errorf("read issue #%d blocked-by IDs returned no ID for relationship %d",
				number, index+1)
		}
		ids = append(ids, node.ID)
	}
	return ids, nil
}

func (a app) addBlockedBy(root string, target, blocker issueNodeIdentity) error {
	output, err := a.command(root, "gh", "api", "graphql", "-f", "query="+addBlockedByMutation,
		"-f", "issueId="+target.issueID, "-f", "blockingIssueId="+blocker.issueID)
	if err != nil {
		return fmt.Errorf("add blocked-by mutation: %w", err)
	}
	var response addBlockedByResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return fmt.Errorf("decode add blocked-by mutation: %w", err)
	}
	if err := graphQLErrors(response.Errors); err != nil {
		return fmt.Errorf("add blocked-by mutation: %w", err)
	}
	if response.Data == nil {
		return errors.New("add blocked-by mutation returned null data")
	}
	if response.Data.AddBlockedBy == nil {
		return errors.New("add blocked-by mutation returned null payload")
	}
	payload := response.Data.AddBlockedBy
	if payload.Issue == nil || strings.TrimSpace(payload.Issue.ID) == "" {
		return errors.New("add blocked-by mutation returned no target issue ID")
	}
	if payload.BlockingIssue == nil || strings.TrimSpace(payload.BlockingIssue.ID) == "" {
		return errors.New("add blocked-by mutation returned no blocker issue ID")
	}
	if payload.Issue.ID != target.issueID {
		return fmt.Errorf("add blocked-by mutation returned target %q, want %q", payload.Issue.ID, target.issueID)
	}
	if payload.BlockingIssue.ID != blocker.issueID {
		return fmt.Errorf("add blocked-by mutation returned blocker %q, want %q", payload.BlockingIssue.ID, blocker.issueID)
	}
	return nil
}

const issueIdentityQuery = `query IssueIdentity($owner: String!, $repository: String!, $number: Int!) {
  repository(owner: $owner, name: $repository) {
    id
    issue(number: $number) {
      id
      number
    }
  }
}`

const issueBlockedByQuery = `query IssueBlockedBy($owner: String!, $repository: String!, $number: Int!) {
  repository(owner: $owner, name: $repository) {
    id
    issue(number: $number) {
      id
      number
      blockedBy(first: 50) {
        nodes {
          id
        }
        totalCount
        pageInfo {
          hasNextPage
        }
      }
    }
  }
}`

const addBlockedByMutation = `mutation AddBlockedBy($issueId: ID!, $blockingIssueId: ID!) {
  addBlockedBy(input: {issueId: $issueId, blockingIssueId: $blockingIssueId}) {
    issue {
      id
    }
    blockingIssue {
      id
    }
  }
}`

func issueNumberFromURL(url string) (int, error) {
	part := url[strings.LastIndex(url, "/")+1:]
	number, err := strconv.Atoi(part)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("invalid issue URL %q", url)
	}
	return number, nil
}

func (a app) runHandoff(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl handoff ISSUE --body-file FILE [--needs-human]")
	}
	number, err := positiveNumber(args[0])
	if err != nil {
		return usageError("handoff: %v", err)
	}
	flags := flag.NewFlagSet("handoff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	bodyFile := flags.String("body-file", "", "Markdown body file")
	needsHuman := flags.Bool("needs-human", false, "mark the issue needs-human before recording the handoff")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("handoff: %v", parseErr)
	}
	if flags.NArg() != 0 || *bodyFile == "" {
		return usageError("usage: workflowctl handoff ISSUE --body-file FILE [--needs-human]")
	}
	if fileErr := requireRegularFile(*bodyFile); fileErr != nil {
		return fileErr
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	if *needsHuman {
		return a.handoffNeedsHuman(root, number, *bodyFile)
	}
	output, err := a.command(root, "gh", "issue", "comment", strconv.Itoa(number), "--repo", repositoryKey,
		"--body-file", *bodyFile)
	if err != nil {
		return fmt.Errorf("comment on issue #%d: %w", number, err)
	}
	return writeLine(a.stdout, "%s", output)
}

func (a app) handoffNeedsHuman(root string, number int, bodyFile string) error {
	body, err := readHandoffBody(bodyFile)
	if err != nil {
		return fmt.Errorf("handoff issue #%d: validate body: %w", number, err)
	}
	status, err := a.readIssueStatus(root, number)
	if err != nil {
		return fmt.Errorf("handoff issue #%d: preflight issue state: %w", number, err)
	}
	if status.State != "OPEN" {
		return stateError("handoff issue #%d is %s; no mutation performed", number, status.State)
	}
	items, err := a.projectItems(root)
	if err != nil {
		return fmt.Errorf("handoff issue #%d: preflight Project membership: %w", number, err)
	}
	initialItem, projectErr := findProjectIssue(items, number)
	if projectErr != nil {
		return stateError("handoff issue #%d is not in canonical Project #%d; no mutation performed: %w",
			number, projectNumber, projectErr)
	}
	comments, err := a.readIssueComments(root, number)
	if err != nil {
		return fmt.Errorf("handoff issue #%d: preflight evidence comments: %w", number, err)
	}
	alreadyPosted := exactTrustedIssueComment(comments, body)
	if proofErr := a.verifyHandoffTarget(root, number, status, initialItem); proofErr != nil {
		return proofErr
	}
	if transitionErr := a.transitionIssueToNeedsHuman(root, number); transitionErr != nil {
		return fmt.Errorf("handoff issue #%d transition incomplete; retry: %w", number, transitionErr)
	}
	if alreadyPosted {
		return writeLine(a.stdout, "issue #%d needs-human handoff already recorded", number)
	}
	output, err := a.command(root, "gh", "issue", "comment", strconv.Itoa(number), "--repo", repositoryKey,
		"--body-file", bodyFile)
	if err != nil {
		comments, readErr := a.readIssueComments(root, number)
		if readErr == nil && exactTrustedIssueComment(comments, body) {
			return writeLine(a.stdout, "issue #%d needs-human handoff already recorded", number)
		}
		return retryableOperation("needs-human evidence comment", fmt.Errorf("handoff issue #%d evidence comment phase incomplete after needs-human and Project Backlog; retry: %w",
			number, errors.Join(err, readErr)))
	}
	return writeLine(a.stdout, "%s", output)
}

func (a app) verifyHandoffTarget(root string, number int, initialStatus issueStatus,
	initialItem projectItem,
) error {
	latestStatus, err := a.readIssueStatus(root, number)
	if err != nil {
		return fmt.Errorf("handoff issue #%d pre-mutation proof: read issue state: %w", number, err)
	}
	if latestStatus.State != "OPEN" || latestStatus.State != initialStatus.State {
		return stateError("handoff issue #%d pre-mutation proof changed: issue is %s; no mutation performed",
			number, latestStatus.State)
	}
	items, err := a.projectItems(root)
	if err != nil {
		return fmt.Errorf("handoff issue #%d pre-mutation proof: read Project: %w", number, err)
	}
	latestItem, err := findProjectIssue(items, number)
	if err != nil {
		return stateError("handoff issue #%d pre-mutation proof changed: Project membership is invalid; no mutation performed: %w",
			number, err)
	}
	if !sameCanonicalProjectItem(initialItem, latestItem) {
		return stateError("handoff issue #%d pre-mutation proof changed: Project identity differs; no mutation performed",
			number)
	}
	return nil
}

func sameCanonicalProjectItem(left, right projectItem) bool {
	return left.ID == right.ID && left.Content.Number == right.Content.Number &&
		left.Content.Repository == right.Content.Repository && left.Content.Type == right.Content.Type
}

func readHandoffBody(path string) (string, error) {
	if err := requireRegularFile(path); err != nil {
		return "", err
	}
	// #nosec G304 -- path is an explicit operator-supplied input.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", errors.New("body must not be empty")
	}
	return string(data), nil
}

func (a app) readIssueComments(root string, number int) ([]issueCommentAPI, error) {
	endpoint := "repos/" + repositoryKey + "/issues/" + strconv.Itoa(number) + "/comments?per_page=100"
	output, err := a.command(root, "gh", "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("read issue #%d comments: %w", number, err)
	}
	pages, err := decodeJSONDocuments[[]issueCommentAPI](output)
	if err != nil {
		return nil, terminalOperation("issue comments", fmt.Errorf("decode issue #%d comments: %w", number, err))
	}
	comments := make([]issueCommentAPI, 0)
	for _, page := range pages {
		comments = append(comments, page...)
	}
	return comments, nil
}

func exactTrustedIssueComment(comments []issueCommentAPI, body string) bool {
	for _, comment := range comments {
		if comment.Body != body {
			continue
		}
		if comment.User.Login == owner || comment.User.Login == trustedActor {
			return true
		}
	}
	return false
}
