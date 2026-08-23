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
	if issueNeedsHuman(status) {
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

func issueNeedsHuman(status issueStatus) bool {
	for _, label := range status.Labels {
		if label.Name == "needs-human" {
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

type runLocalRef struct {
	branch string
	number int
	runID  string
	sha    string
	source claimRefSource
}

type agentRef struct {
	branch string
	sha    string
}

type agentRefInventory struct {
	claims    []remoteClaim
	runLocals []runLocalRef
	archives  []agentRef
	malformed []agentRef
	unrelated []agentRef
}

type agentRefKind uint8

const (
	agentRefUnrelated agentRefKind = iota
	agentRefClaim
	agentRefRunLocal
	agentRefArchive
	agentRefMalformed
)

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
		if claims[left].branch != claims[right].branch {
			return claims[left].branch < claims[right].branch
		}
		return claims[left].sha < claims[right].sha
	})
	return claims, nil
}

func (a app) remoteClaimRefs(root string) ([]remoteClaim, error) {
	inventory, err := a.remoteAgentRefInventory(root)
	if err != nil {
		return nil, fmt.Errorf("list remote claim refs: %w", err)
	}
	claims := inventory.claims
	sort.Slice(claims, func(left, right int) bool {
		if claims[left].branch != claims[right].branch {
			return claims[left].branch < claims[right].branch
		}
		return claims[left].sha < claims[right].sha
	})
	return claims, nil
}

func (a app) remoteAgentRefInventory(root string) (agentRefInventory, error) {
	output, err := a.command(root, "git", "ls-remote", "--heads", "origin", "refs/heads/agent/*")
	if err != nil {
		return agentRefInventory{}, fmt.Errorf("list remote agent refs: %w", err)
	}
	inventory := agentRefInventory{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return agentRefInventory{}, fmt.Errorf("remote agent ref listing contains malformed entry %q", line)
		}
		branch := strings.TrimPrefix(fields[1], "refs/heads/")
		ref := agentRef{branch: branch, sha: fields[0]}
		kind, number, runID := classifyAgentRef(branch)
		switch kind {
		case agentRefClaim:
			inventory.claims = append(inventory.claims, remoteClaim{branch: branch, number: number, sha: fields[0], source: claimRefRemote})
		case agentRefRunLocal:
			inventory.runLocals = append(inventory.runLocals, runLocalRef{branch: branch, number: number, runID: runID, sha: fields[0], source: claimRefRemote})
		case agentRefArchive:
			inventory.archives = append(inventory.archives, ref)
		case agentRefMalformed:
			inventory.malformed = append(inventory.malformed, ref)
		case agentRefUnrelated:
			inventory.unrelated = append(inventory.unrelated, ref)
		default:
			return agentRefInventory{}, fmt.Errorf("classify remote agent ref %q: unknown ref kind %d", branch, kind)
		}
	}
	sortRemoteAgentRefs(&inventory)
	return inventory, nil
}

func classifyAgentRef(branch string) (agentRefKind, int, string) {
	if strings.HasPrefix(branch, "agent/archive/") {
		return agentRefArchive, 0, ""
	}
	number, suffix, ok := issueBranchParts(branch)
	if !ok {
		if strings.HasPrefix(branch, "agent/issue-") {
			return agentRefMalformed, 0, ""
		}
		return agentRefUnrelated, 0, ""
	}
	if suffix == "" {
		if branch == claimBranch(number) {
			return agentRefClaim, number, ""
		}
		return agentRefMalformed, number, ""
	}
	if validRunID(suffix) {
		return agentRefRunLocal, number, suffix
	}
	return agentRefMalformed, number, ""
}

func fixedClaimIssue(branch string) (int, bool) {
	kind, number, _ := classifyAgentRef(branch)
	return number, kind == agentRefClaim
}

func issueBranchParts(branch string) (int, string, bool) {
	const prefix = "agent/issue-"
	value := strings.TrimPrefix(branch, prefix)
	if value == branch || value == "" {
		return 0, "", false
	}
	digits := value
	suffix := ""
	if index := strings.IndexByte(value, '-'); index >= 0 {
		digits = value[:index]
		suffix = value[index+1:]
	}
	if digits == "" {
		return 0, "", false
	}
	if len(digits) > 1 && digits[0] == '0' {
		return 0, "", false
	}
	for _, value := range digits {
		if value < '0' || value > '9' {
			return 0, "", false
		}
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 {
		return 0, "", false
	}
	return number, suffix, true
}

func validRunID(runID string) bool {
	if !strings.HasPrefix(runID, "run-") || len(runID) == len("run-") {
		return false
	}
	partStart := len("run-")
	for index, value := range runID[partStart:] {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
			continue
		}
		if value == '-' && index > 0 && index+partStart+1 < len(runID) && runID[index+partStart+1] != '-' {
			continue
		}
		return false
	}
	return true
}

func sortRemoteAgentRefs(inventory *agentRefInventory) {
	sort.Slice(inventory.claims, func(left, right int) bool {
		if inventory.claims[left].branch != inventory.claims[right].branch {
			return inventory.claims[left].branch < inventory.claims[right].branch
		}
		return inventory.claims[left].sha < inventory.claims[right].sha
	})
	sort.Slice(inventory.runLocals, func(left, right int) bool {
		if inventory.runLocals[left].branch != inventory.runLocals[right].branch {
			return inventory.runLocals[left].branch < inventory.runLocals[right].branch
		}
		return inventory.runLocals[left].sha < inventory.runLocals[right].sha
	})
	sort.Slice(inventory.archives, func(left, right int) bool {
		if inventory.archives[left].branch != inventory.archives[right].branch {
			return inventory.archives[left].branch < inventory.archives[right].branch
		}
		return inventory.archives[left].sha < inventory.archives[right].sha
	})
	sort.Slice(inventory.malformed, func(left, right int) bool {
		if inventory.malformed[left].branch != inventory.malformed[right].branch {
			return inventory.malformed[left].branch < inventory.malformed[right].branch
		}
		return inventory.malformed[left].sha < inventory.malformed[right].sha
	})
	sort.Slice(inventory.unrelated, func(left, right int) bool {
		if inventory.unrelated[left].branch != inventory.unrelated[right].branch {
			return inventory.unrelated[left].branch < inventory.unrelated[right].branch
		}
		return inventory.unrelated[left].sha < inventory.unrelated[right].sha
	})
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
