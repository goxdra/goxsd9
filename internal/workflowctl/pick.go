package workflowctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"
)

type issueRelations struct {
	BlockedBy issueConnection `json:"blockedBy"`
	Blocking  issueConnection `json:"blocking"`
	CreatedAt time.Time       `json:"createdAt"`
}

type issueRelationsResponse struct {
	Data struct {
		Repository struct {
			Issue *issueRelations `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

type issueConnection struct {
	Nodes []relatedIssue `json:"nodes"`
}

type relatedIssue struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Title  string `json:"title"`
}

type pickCandidate struct {
	Blocking int    `json:"blocking"`
	Effort   string `json:"effort"`
	Number   int    `json:"number"`
	Phase    string `json:"phase"`
	Priority string `json:"priority"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	created  time.Time
}

func (a app) runPick(args []string) error {
	flags := flag.NewFlagSet("pick", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return usageError("pick: %v", err)
	}
	if flags.NArg() != 0 {
		return usageError("pick takes no positional arguments")
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	candidates, err := a.pickCandidates(root)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return stateError("no unblocked Ready issue is available; run backlog curation")
	}
	selected := candidates[0]
	if *jsonOutput {
		encoder := json.NewEncoder(a.stdout)
		if err := encoder.Encode(selected); err != nil {
			return fmt.Errorf("encode selected issue: %w", err)
		}
		return nil
	}
	return writeLine(a.stdout, "#%d [%s/%s] %s\n%s", selected.Number, selected.Priority, selected.Effort,
		selected.Title, selected.URL)
}

func (a app) pickCandidates(root string) ([]pickCandidate, error) {
	list, err := a.projectItems(root)
	if err != nil {
		return nil, err
	}
	var candidates []pickCandidate
	for _, item := range list.Items {
		if item.Status != "Ready" || item.Content.Type != "Issue" {
			continue
		}
		relations, err := a.issueRelations(root, item.Content.Number)
		if err != nil {
			return nil, err
		}
		if hasOpenIssue(relations.BlockedBy.Nodes) {
			continue
		}
		candidates = append(candidates, pickCandidate{
			Blocking: countOpenIssues(relations.Blocking.Nodes),
			Effort:   item.Effort,
			Number:   item.Content.Number,
			Phase:    item.Phase,
			Priority: item.Priority,
			Title:    item.Title,
			URL:      item.Content.URL,
			created:  relations.CreatedAt,
		})
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidateLess(candidates[left], candidates[right])
	})
	return candidates, nil
}

func (a app) issueRelations(root string, number int) (issueRelations, error) {
	output, err := a.command(root, "gh", "api", "graphql", "-f", "query="+issueRelationsQuery,
		"-f", "owner="+owner, "-f", "repository="+repository, "-F", "number="+strconv.Itoa(number))
	if err != nil {
		return issueRelations{}, fmt.Errorf("read issue #%d dependencies: %w", number, err)
	}
	var response issueRelationsResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return issueRelations{}, fmt.Errorf("decode issue #%d dependencies: %w", number, err)
	}
	if response.Data.Repository.Issue == nil {
		return issueRelations{}, fmt.Errorf("read issue #%d dependencies: issue was not found", number)
	}
	return *response.Data.Repository.Issue, nil
}

const issueRelationsQuery = `query IssueRelations($owner: String!, $repository: String!, $number: Int!) {
  repository(owner: $owner, name: $repository) {
    issue(number: $number) {
      blockedBy(first: 100) {
        nodes {
          number
          state
          title
        }
      }
      blocking(first: 100) {
        nodes {
          number
          state
          title
        }
      }
      createdAt
    }
  }
}`

func candidateLess(left, right pickCandidate) bool {
	if priorityRank(left.Priority) != priorityRank(right.Priority) {
		return priorityRank(left.Priority) < priorityRank(right.Priority)
	}
	if left.Blocking != right.Blocking {
		return left.Blocking > right.Blocking
	}
	if effortRank(left.Effort) != effortRank(right.Effort) {
		return effortRank(left.Effort) < effortRank(right.Effort)
	}
	if !left.created.Equal(right.created) {
		return left.created.Before(right.created)
	}
	return left.Number < right.Number
}

func priorityRank(priority string) int {
	switch priority {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	case "P4":
		return 4
	default:
		return 5
	}
}

func effortRank(effort string) int {
	switch effort {
	case "XS":
		return 0
	case "S":
		return 1
	case "M":
		return 2
	default:
		return 3
	}
}

func hasOpenIssue(issues []relatedIssue) bool {
	return countOpenIssues(issues) != 0
}

func countOpenIssues(issues []relatedIssue) int {
	count := 0
	for _, issue := range issues {
		if issue.State == "OPEN" {
			count++
		}
	}
	return count
}
