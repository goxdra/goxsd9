package workflowctl

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestSyncMapsClosedIssueToDone(t *testing.T) {
	edited := false
	application := app{stdout: io.Discard, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "gh api repos/goxdra/goxsd9/issues/141":
			return `{"state":"closed","labels":[]}`, nil
		case "gh project field-list 1 --owner goxdra --format json":
			return `{"fields":[{"id":"status-id","name":"Status","options":[{"id":"done-id","name":"Done"}]}]}`, nil
		case "gh project item-edit --project-id PVT_kwDOEupz2s4Bgc9A --id item-141 --field-id status-id --single-select-option-id done-id":
			edited = true
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}}
	item := projectItem{ID: "item-141", Status: "Ready"}
	item.Content.Number = 141
	item.Content.Repository = repositoryKey
	item.Content.Type = "Issue"
	fields := projectFieldList{Fields: []projectField{{ID: "status-id", Name: "Status", Options: []projectFieldOption{{ID: "done-id", Name: "Done"}}}}}
	changed, err := application.syncProjectItem("/repo", fields, item, nil)
	if err != nil {
		t.Fatalf("syncProjectItem: %v", err)
	}
	if changed != 1 || !edited {
		t.Fatalf("closed issue sync = changed %d, edited %t; want one Done edit", changed, edited)
	}
}

func TestSetIssueProjectStatusIsIdempotent(t *testing.T) {
	commands := 0
	application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		commands++
		command := name + " " + strings.Join(args, " ")
		if command == "gh project item-list 1 --owner goxdra --format json --limit 500" {
			return `{"items":[{"content":{"number":141,"repository":"goxdra/goxsd9","type":"Issue"},"id":"item-141","status":"Done"}]}`, nil
		}
		return "", fmt.Errorf("unexpected idempotent command: %s", command)
	}}
	if err := application.setIssueProjectStatus("/repo", 141, "Done"); err != nil {
		t.Fatalf("setIssueProjectStatus: %v", err)
	}
	if commands != 1 {
		t.Fatalf("idempotent Project status used %d commands, want only item read", commands)
	}
}

func TestMergedPrimaryReconciliationDoesNotTouchCompanion(t *testing.T) {
	issueState := "open"
	patches := 0
	application := app{executeCommand: func(_ string, input io.Reader, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "gh api repos/goxdra/goxsd9/issues/141":
			return fmt.Sprintf(`{"state":%q,"labels":[]}`, issueState), nil
		case "gh api --method PATCH repos/goxdra/goxsd9/issues/141 --input -":
			if input == nil {
				return "", errors.New("primary close did not send a request body")
			}
			patches++
			issueState = "closed"
			return `{"state":"closed"}`, nil
		default:
			return "", fmt.Errorf("unexpected issue mutation, including companion: %s", command)
		}
	}}
	proof := mergeEvaluationProof{
		bodySHA256:    "body-proof",
		baseRefName:   "main",
		claimProofs:   []evaluationClaimProof{{Issue: 141, Branch: "agent/issue-141", SHA: "primary-head"}, {Issue: 142, Branch: "agent/issue-142", SHA: "companion-head"}},
		closingIssues: []int{141, 142},
		head:          "primary-head",
		headRefName:   "agent/issue-141",
	}
	if err := application.reconcileMergedPrimaryIssue("/repo", 14, proof, 141); err != nil {
		t.Fatalf("reconcileMergedPrimaryIssue: %v", err)
	}
	if issueState != "closed" || patches != 1 {
		t.Fatalf("primary reconciliation state = %s, PATCH count %d; want closed and one primary PATCH", issueState, patches)
	}
}
