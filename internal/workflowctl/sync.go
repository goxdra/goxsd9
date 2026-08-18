package workflowctl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type issueStatus struct {
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	State string `json:"state"`
}

func (a app) runSync(args []string) error {
	if len(args) != 0 {
		return usageError("sync takes no arguments")
	}
	if err := writeLine(a.stdout, "Project status synchronization plus claim-ref fetches; canonical Git base synchronization is %s", baseSyncCommand); err != nil {
		return fmt.Errorf("write Project synchronization notice: %w", err)
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	list, err := a.projectItems(root)
	if err != nil {
		return err
	}
	fields, err := a.projectFields(root)
	if err != nil {
		return err
	}
	claims, err := a.remoteClaims(root)
	if err != nil {
		return err
	}
	changes := 0
	for _, item := range list.Items {
		changed, syncErr := a.syncProjectItem(root, fields, item, claims)
		if syncErr != nil {
			return syncErr
		}
		changes += changed
	}
	return writeLine(a.stdout, "Project status synchronization: %d change(s); claim refs fetched for lease classification", changes)
}

func (a app) syncProjectItem(root string, fields projectFieldList, item projectItem, claims map[int]bool) (int, error) {
	if item.Content.Type != "Issue" || item.Content.Repository != repositoryKey {
		return 0, nil
	}
	desired, err := a.desiredStatus(root, item, claims)
	if err != nil {
		return 0, err
	}
	if desired == item.Status {
		return 0, nil
	}
	if err := a.setProjectFieldFromList(root, fields, item.ID, "Status", desired); err != nil {
		return 0, err
	}
	if err := writeLine(a.stdout, "#%d: %s -> %s", item.Content.Number, item.Status, desired); err != nil {
		return 0, err
	}
	return 1, nil
}

func (a app) desiredStatus(root string, item projectItem, claims map[int]bool) (string, error) {
	status, err := a.readIssueStatus(root, item.Content.Number)
	if err != nil {
		return "", err
	}
	if status.State == "CLOSED" {
		return "Done", nil
	}
	if issueHasLabel(status, "needs-human") {
		return "Backlog", nil
	}
	if claims[item.Content.Number] {
		return "Picked", nil
	}
	if item.Status == "Picked" || item.Status == "Done" {
		return "Ready", nil
	}
	return item.Status, nil
}

func (a app) readIssueStatus(root string, number int) (issueStatus, error) {
	endpoint := "repos/" + repositoryKey + "/issues/" + strconv.Itoa(number)
	output, err := a.command(root, "gh", "api", endpoint)
	if err != nil {
		return issueStatus{}, fmt.Errorf("read issue #%d: %w", number, err)
	}
	var status issueStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return issueStatus{}, fmt.Errorf("decode issue #%d: %w", number, err)
	}
	status.State = strings.ToUpper(status.State)
	return status, nil
}

func issueHasLabel(status issueStatus, target string) bool {
	for _, label := range status.Labels {
		if label.Name == target {
			return true
		}
	}
	return false
}

func (a app) remoteClaims(root string) (map[int]bool, error) {
	listed, err := a.listRemoteClaims(root)
	if err != nil {
		return nil, err
	}
	claims := make(map[int]bool)
	for _, claim := range listed {
		if claim.active {
			claims[claim.number] = true
		}
	}
	return claims, nil
}

type remoteClaim struct {
	active bool
	branch string
	lease  time.Time
	number int
	sha    string
	source claimRefSource
}

type claimRefSource uint8

const (
	claimRefRemote claimRefSource = iota + 1
	claimRefLocal
	claimRefTracking
)

func (a app) listRemoteClaims(root string) ([]remoteClaim, error) {
	claims, err := a.remoteClaimRefs(root)
	if err != nil {
		return nil, fmt.Errorf("list remote claims: %w", err)
	}
	for index := range claims {
		claim, err := a.inspectRemoteClaim(root, claims[index].branch, claims[index].sha, claims[index].number)
		if err != nil {
			return nil, err
		}
		claims[index] = claim
	}
	sort.Slice(claims, func(left, right int) bool {
		return claims[left].branch < claims[right].branch
	})
	return claims, nil
}

func (a app) remoteClaimRefs(root string) ([]remoteClaim, error) {
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", "refs/heads/agent/issue-*")
	if err != nil {
		return nil, fmt.Errorf("list remote claim refs: %w", err)
	}
	var claims []remoteClaim
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		branch := strings.TrimPrefix(fields[1], "refs/heads/")
		number, ok := issueFromBranch(branch)
		if !ok || branch != claimBranch(number) {
			continue
		}
		claims = append(claims, remoteClaim{branch: branch, number: number, sha: fields[0], source: claimRefRemote})
	}
	sort.Slice(claims, func(left, right int) bool {
		return claims[left].branch < claims[right].branch
	})
	return claims, nil
}

func (a app) inspectRemoteClaim(root, branch, sha string, number int) (remoteClaim, error) {
	if _, err := a.command(root, "git", "fetch", "--no-tags", "origin", "refs/heads/"+branch); err != nil {
		return remoteClaim{}, fmt.Errorf("fetch claim %s: %w", branch, err)
	}
	message, err := a.command(root, "git", "log", "-100", "--format=%B", sha)
	if err != nil {
		return remoteClaim{}, fmt.Errorf("read claim %s: %w", branch, err)
	}
	lease, leaseErr := trailerTime(message)
	return remoteClaim{
		active: leaseErr == nil && lease.After(time.Now().UTC()),
		branch: branch,
		lease:  lease,
		number: number,
		sha:    sha,
		source: claimRefRemote,
	}, nil
}

func (a app) setProjectFieldFromList(root string, fields projectFieldList, itemID, fieldName, optionName string) error {
	fieldID, optionID, err := fields.option(fieldName, optionName)
	if err != nil {
		return err
	}
	_, err = a.command(root, "gh", "project", "item-edit", "--project-id", projectID, "--id", itemID,
		"--field-id", fieldID, "--single-select-option-id", optionID)
	if err != nil {
		return fmt.Errorf("set %s=%s: %w", fieldName, optionName, err)
	}
	return nil
}
