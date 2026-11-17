package workflowctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

func projectInventoryJSON(t *testing.T, returned int, total string) string {
	t.Helper()
	items := make([]projectItem, returned)
	for index := range items {
		items[index].Content.Type = "DraftIssue"
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("encode inventory: %v", err)
	}
	if total == "" {
		return fmt.Sprintf(`{"items":%s}`, encoded)
	}
	return fmt.Sprintf(`{"items":%s,"totalCount":%s}`, encoded, total)
}

// TestProjectInventoryCommandsRefuseIncompleteResponse covers the command boundary,
// including sync's guarantee that no remote reads or edits follow a bad inventory.
func TestProjectInventoryCommandsRefuseIncompleteResponse(t *testing.T) {
	tests := []struct {
		name     string
		returned int
		total    string
		want     string
	}{
		{name: "missing", returned: 1, want: "missing totalCount (returned 1)"},
		{name: "null", returned: 1, total: "null", want: "missing totalCount (returned 1)"},
		{name: "negative", returned: 1, total: "-1", want: "negative totalCount -1 (returned 1)"},
		{name: "malformed", returned: 1, total: `"one"`, want: "totalCount (returned 1)"},
		{name: "short below limit", returned: 1, total: "2", want: "totalCount 2, returned 1"},
		{name: "overreported limit", returned: 500, total: "501", want: "totalCount 501, returned 500"},
		{name: "above configured limit", returned: 501, total: "501", want: "exceeds limit 500: totalCount 501, returned 501"},
		{name: "underreported", returned: 2, total: "1", want: "totalCount 1, returned 2"},
	}
	for _, test := range tests {
		for _, command := range []string{"sync", "backlog"} {
			t.Run(test.name+"/"+command, func(t *testing.T) {
				assertInventoryRefusal(t, command, projectInventoryJSON(t, test.returned, test.total), test.want)
			})
		}
	}
}

func assertInventoryRefusal(t *testing.T, command, response, want string) {
	t.Helper()
	var output bytes.Buffer
	calls := make([]string, 0)
	application := app{ctx: context.Background(), stdout: &output, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if call == "git rev-parse --show-toplevel" {
			return "/repo", nil
		}
		if call == "gh project item-list 1 --owner goxdra --format json --limit 500" {
			return response, nil
		}
		return "", fmt.Errorf("unexpected command after incomplete inventory: %s", call)
	}}
	args := []string{command}
	if command == "backlog" {
		args = []string{"backlog", "health", "--format", "json"}
	}
	err := application.run(args)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if operationDispositionOf(err) != operationDispositionTerminal {
		t.Fatalf("error disposition = %d, want terminal", operationDispositionOf(err))
	}
	if len(calls) != 2 {
		t.Fatalf("commands = %v, want only root and inventory reads", calls)
	}
	if command == "backlog" && output.Len() != 0 {
		t.Fatalf("health rendered from incomplete inventory: %q", output.String())
	}
	if command == "sync" && strings.Contains(output.String(), "change(s)") {
		t.Fatalf("sync completed from incomplete inventory: %q", output.String())
	}
}

func TestProjectInventoryCompleteBoundaries(t *testing.T) {
	for _, count := range []int{0, 1, 499, 500} {
		for _, command := range []string{"sync", "backlog"} {
			t.Run(fmt.Sprintf("%d/%s", count, command), func(t *testing.T) {
				assertCompleteInventory(t, command, projectInventoryJSON(t, count, strconv.Itoa(count)))
			})
		}
	}
}

func assertCompleteInventory(t *testing.T, command, response string) {
	t.Helper()
	var output bytes.Buffer
	application := app{ctx: context.Background(), stdout: &output, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		switch call {
		case "git rev-parse --show-toplevel":
			return "/repo", nil
		case "gh project item-list 1 --owner goxdra --format json --limit 500":
			return response, nil
		case "gh project field-list 1 --owner goxdra --format json":
			return `{"fields":[]}`, nil
		case "git ls-remote --heads origin refs/heads/agent/*":
			return "", nil
		}
		return "", fmt.Errorf("unexpected command: %s", call)
	}}
	if command == "sync" {
		if err := application.run([]string{"sync"}); err != nil {
			t.Fatalf("sync complete inventory: %v", err)
		}
		if !strings.Contains(output.String(), "0 change(s)") {
			t.Fatalf("sync output = %q", output.String())
		}
		return
	}
	err := application.run([]string{"backlog", "health", "--format", "json"})
	if err == nil || !strings.Contains(err.Error(), "ready-work buffer is below target") {
		t.Fatalf("health error = %v, want normal Ready deficit", err)
	}
	if !strings.Contains(output.String(), `"counts":{"ready":0`) {
		t.Fatalf("health output = %q, want rendered zero counts", output.String())
	}
}

func TestProjectInventoryPreservesMalformedTotalCause(t *testing.T) {
	_, err := decodeProjectList([]byte(projectInventoryJSON(t, 1, `"one"`)))
	var typeError *json.UnmarshalTypeError
	if !errors.As(err, &typeError) {
		t.Fatalf("malformed total error = %v, want JSON type cause", err)
	}
}

func TestBacklogHealthKeepsDuplicateAndMetadataChecksAfterCountProof(t *testing.T) {
	items := []projectItem{
		{Content: projectContent{Number: 7, Repository: repositoryKey, Type: "Issue", Title: "Seven"}, Status: "Picked"},
		{Content: projectContent{Number: 7, Repository: repositoryKey, Type: "Issue", Title: "Seven again"}, Status: "Ready"},
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("encode duplicate items: %v", err)
	}
	response := fmt.Sprintf(`{"items":%s,"totalCount":2}`, encoded)
	output, err := runBacklogProjectResponse(t, response, "json")
	if err == nil || !strings.Contains(err.Error(), "duplicate canonical Issue item for issue #7") || output != "" {
		t.Fatalf("duplicate response = (%q, %v), want duplicate error and no report", output, err)
	}

	one, marshalErr := json.Marshal(items[:1])
	if marshalErr != nil {
		t.Fatalf("encode metadata item: %v", marshalErr)
	}
	response = fmt.Sprintf(`{"items":%s,"totalCount":1}`, one)
	output, err = runBacklogProjectResponse(t, response, "json")
	if err == nil || !strings.Contains(err.Error(), "ready-work buffer is below target") {
		t.Fatalf("metadata response = (%q, %v), want ordinary metadata finding", output, err)
	}
	if !strings.Contains(output, `"missing":["Effort","Phase","Priority"]`) {
		t.Fatalf("metadata output = %q, want missing fields", output)
	}
}
