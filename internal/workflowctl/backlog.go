package workflowctl

import (
	"fmt"
)

func (a app) runBacklog(args []string) error {
	if len(args) != 1 || args[0] != "health" {
		return usageError("usage: workflowctl backlog health")
	}
	root, err := a.root()
	if err != nil {
		return err
	}
	list, err := a.projectItems(root)
	if err != nil {
		return err
	}
	ready, xs, small, medium, err := a.readyCounts(root, list)
	if err != nil {
		return err
	}
	if err := writeLine(a.stdout, "Ready: %d (XS=%d S=%d M=%d)", ready, xs, small, medium); err != nil {
		return err
	}

	var deficits []string
	if ready < 8 {
		deficits = append(deficits, fmt.Sprintf("%d total", 8-ready))
	}
	if xs < 2 {
		deficits = append(deficits, fmt.Sprintf("%d XS", 2-xs))
	}
	if small < 3 {
		deficits = append(deficits, fmt.Sprintf("%d S", 3-small))
	}
	if medium < 2 {
		deficits = append(deficits, fmt.Sprintf("%d M", 2-medium))
	}
	if len(deficits) != 0 {
		return stateError("ready-work buffer is below target: need %v", deficits)
	}
	return writeLine(a.stdout, "Ready-work buffer: healthy")
}

func (a app) readyCounts(root string, list projectList) (int, int, int, int, error) {
	ready := 0
	xs := 0
	small := 0
	medium := 0
	for _, item := range list.Items {
		if item.Status != "Ready" || item.Content.Type != "Issue" || item.Content.Repository != repositoryKey {
			continue
		}
		relations, err := a.issueRelations(root, item.Content.Number)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if hasOpenIssue(relations.BlockedBy.Nodes) {
			continue
		}
		ready++
		switch item.Effort {
		case "XS":
			xs++
		case "S":
			small++
		case "M":
			medium++
		}
	}
	return ready, xs, small, medium, nil
}
