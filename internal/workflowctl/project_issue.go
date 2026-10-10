package workflowctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const claimResumeProjectIssueQuery = `query($owner:String!,$repository:String!,$number:Int!,$after:String){repository(owner:$owner,name:$repository){id nameWithOwner issue(number:$number){id number projectItems(first:100,after:$after,includeArchived:true){totalCount nodes{id type isArchived project{id number} content{__typename ... on Issue{id number repository{id nameWithOwner}}} fieldValueByName(name:"Status"){__typename ... on ProjectV2ItemFieldSingleSelectValue{name optionId field{... on ProjectV2SingleSelectField{id}}}}} pageInfo{hasNextPage endCursor}}}}}`

const claimResumeStatusFieldID = "PVTSSF_lADOEupz2s4Bgc9Azhd1dsA"

type claimResumeProjectItem struct {
	ID             string
	Status         string
	StatusOptionID string
	IssueID        string
	RepositoryID   string
}

type claimResumeProjectPage struct {
	Data   *claimResumeProjectData `json:"data"`
	Errors []graphqlIssueError     `json:"errors"`
}

type claimResumeProjectData struct {
	Repository *claimResumeProjectRepository `json:"repository"`
}

type claimResumeProjectRepository struct {
	ID            string                   `json:"id"`
	NameWithOwner string                   `json:"nameWithOwner"`
	Issue         *claimResumeProjectIssue `json:"issue"`
}

type claimResumeProjectIssue struct {
	ID           string                        `json:"id"`
	Number       int                           `json:"number"`
	ProjectItems *claimResumeProjectConnection `json:"projectItems"`
}

type claimResumeProjectConnection struct {
	TotalCount *int                      `json:"totalCount"`
	Nodes      []*claimResumeProjectNode `json:"nodes"`
	PageInfo   *struct {
		HasNextPage *bool  `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

type claimResumeProjectScan struct {
	issueID       string
	repositoryID  string
	cursor        string
	totalCount    int
	seen          int
	seenIDs       map[string]bool
	seenCursors   map[string]bool
	canonicalItem claimResumeProjectItem
}

type claimResumeProjectNode struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	IsArchived *bool  `json:"isArchived"`
	Project    *struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
	} `json:"project"`
	Content *struct {
		Type       string `json:"__typename"`
		ID         string `json:"id"`
		Number     int    `json:"number"`
		Repository *struct {
			ID            string `json:"id"`
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	} `json:"content"`
	Status *struct {
		Type     string `json:"__typename"`
		Name     string `json:"name"`
		OptionID string `json:"optionId"`
		Field    *struct {
			ID string `json:"id"`
		} `json:"field"`
	} `json:"fieldValueByName"`
}

// readClaimResumeProjectItem exhausts the issue's Project memberships on each
// authority check. No earlier page is trusted as a later mutation proof.
func (a app) readClaimResumeProjectItem(root string, number int) (claimResumeProjectItem, error) {
	scan := claimResumeProjectScan{seenIDs: make(map[string]bool), seenCursors: make(map[string]bool)}
	for {
		page, err := a.readClaimResumeProjectPage(root, number, scan.cursor)
		if err != nil {
			return claimResumeProjectItem{}, err
		}
		more, err := scan.addPage(page, number)
		if err != nil {
			return claimResumeProjectItem{}, err
		}
		if !more {
			return scan.canonicalItem, nil
		}
	}
}

func (a app) readClaimResumeProjectPage(root string, number int, cursor string) (claimResumeProjectPage, error) {
	args := []string{"api", "graphql", "-f", "query=" + claimResumeProjectIssueQuery,
		"-f", "owner=" + owner, "-f", "repository=" + repository, "-F", "number=" + strconv.Itoa(number)}
	if cursor != "" {
		args = append(args, "-f", "after="+cursor)
	}
	output, err := a.command(root, "gh", args...)
	if err != nil {
		return claimResumeProjectPage{}, retryableOperation("claim resume Project issue read", fmt.Errorf("read issue #%d Project memberships: %w", number, err))
	}
	var page claimResumeProjectPage
	if err := json.Unmarshal([]byte(output), &page); err != nil {
		return claimResumeProjectPage{}, terminalOperation("claim resume Project issue read", fmt.Errorf("decode issue #%d Project memberships: %w", number, err))
	}
	if err := graphQLErrors(page.Errors); err != nil {
		return claimResumeProjectPage{}, retryableOperation("claim resume Project issue read", fmt.Errorf("read issue #%d Project memberships: %w", number, err))
	}
	return page, nil
}

func (scan *claimResumeProjectScan) addPage(page claimResumeProjectPage, number int) (bool, error) {
	connection, err := scan.validatePage(page, number)
	if err != nil {
		return false, err
	}
	for _, node := range connection.Nodes {
		if err := scan.addNode(node, number); err != nil {
			return false, err
		}
	}
	if !*connection.PageInfo.HasNextPage {
		if scan.seen != scan.totalCount || scan.canonicalItem.ID == "" {
			return false, projectIssueProofError(number, "Project membership is incomplete or canonical item is missing")
		}
		return false, nil
	}
	if scan.seen == scan.totalCount {
		return false, projectIssueProofError(number, "Project membership reports another page after totalCount")
	}
	next := connection.PageInfo.EndCursor
	if strings.TrimSpace(next) == "" || next == scan.cursor || scan.seenCursors[next] {
		return false, projectIssueProofError(number, "Project membership cursor did not advance")
	}
	scan.seenCursors[next] = true
	scan.cursor = next
	return true, nil
}

func (scan *claimResumeProjectScan) validatePage(page claimResumeProjectPage, number int) (*claimResumeProjectConnection, error) {
	if page.Data == nil || page.Data.Repository == nil || page.Data.Repository.Issue == nil {
		return nil, projectIssueProofError(number, "missing repository or issue")
	}
	repo := page.Data.Repository
	issue := repo.Issue
	if strings.TrimSpace(repo.ID) == "" || repo.NameWithOwner != repositoryKey || strings.TrimSpace(issue.ID) == "" || issue.Number != number {
		return nil, projectIssueProofError(number, "repository or issue identity mismatch")
	}
	if scan.repositoryID != "" && (repo.ID != scan.repositoryID || issue.ID != scan.issueID) {
		return nil, projectIssueProofError(number, "repository or issue identity changed between pages")
	}
	connection := issue.ProjectItems
	if err := scan.validateConnection(connection, number); err != nil {
		return nil, err
	}
	if scan.repositoryID == "" {
		scan.repositoryID, scan.issueID, scan.totalCount = repo.ID, issue.ID, *connection.TotalCount
	}
	return connection, nil
}

func (scan *claimResumeProjectScan) validateConnection(connection *claimResumeProjectConnection, number int) error {
	if connection == nil || connection.PageInfo == nil || connection.PageInfo.HasNextPage == nil || connection.TotalCount == nil || connection.Nodes == nil {
		return projectIssueProofError(number, "incomplete Project membership connection")
	}
	if *connection.TotalCount < 0 || (scan.repositoryID != "" && *connection.TotalCount != scan.totalCount) {
		return projectIssueProofError(number, "Project membership count changed between pages")
	}
	if len(connection.Nodes) == 0 && *connection.PageInfo.HasNextPage {
		return projectIssueProofError(number, "empty Project membership page has a successor")
	}
	if len(connection.Nodes) > 100 {
		return projectIssueProofError(number, "Project membership page exceeds requested size")
	}
	return nil
}

func (scan *claimResumeProjectScan) addNode(node *claimResumeProjectNode, number int) error {
	if err := validateClaimResumeProjectNode(node, scan.issueID, scan.repositoryID, number); err != nil {
		return projectIssueProofCause(number, err)
	}
	if scan.seenIDs[node.ID] {
		return projectIssueProofError(number, "duplicate Project item %s", node.ID)
	}
	scan.seenIDs[node.ID] = true
	scan.seen++
	if scan.seen > scan.totalCount {
		return projectIssueProofError(number, "Project membership count exceeded totalCount")
	}
	if node.Project.ID != projectID {
		return nil
	}
	if node.Project.Number != projectNumber || *node.IsArchived {
		return projectIssueProofError(number, "canonical Project item has changed Project identity or is archived")
	}
	if scan.canonicalItem.ID != "" {
		return projectIssueProofError(number, "multiple canonical Project items")
	}
	if node.Status == nil || node.Status.Type != "ProjectV2ItemFieldSingleSelectValue" || strings.TrimSpace(node.Status.Name) == "" || strings.TrimSpace(node.Status.OptionID) == "" || node.Status.Field == nil || node.Status.Field.ID != claimResumeStatusFieldID {
		return projectIssueProofError(number, "canonical Project Status is missing or malformed")
	}
	scan.canonicalItem = claimResumeProjectItem{ID: node.ID, Status: node.Status.Name, StatusOptionID: node.Status.OptionID,
		IssueID: scan.issueID, RepositoryID: scan.repositoryID}
	return nil
}

func validateClaimResumeProjectNode(node *claimResumeProjectNode, issueID, repositoryID string, number int) error {
	if node == nil || strings.TrimSpace(node.ID) == "" || node.Project == nil || strings.TrimSpace(node.Project.ID) == "" || node.Project.Number < 1 || node.IsArchived == nil {
		return errors.New("malformed Project item")
	}
	if node.Type != "ISSUE" || node.Content == nil || node.Content.Type != "Issue" || node.Content.ID != issueID || node.Content.Number != number || node.Content.Repository == nil || node.Content.Repository.ID != repositoryID || node.Content.Repository.NameWithOwner != repositoryKey {
		return fmt.Errorf("project item %s has mismatched issue content", node.ID)
	}
	return nil
}

func projectIssueProofError(number int, format string, args ...any) error {
	return terminalOperation("claim resume Project issue read", fmt.Errorf("issue #%d Project proof: %s; preserve external state", number, fmt.Sprintf(format, args...)))
}

func projectIssueProofCause(number int, err error) error {
	return terminalOperation("claim resume Project issue read", fmt.Errorf("issue #%d Project proof: %w; preserve external state", number, err))
}
