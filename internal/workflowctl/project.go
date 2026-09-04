package workflowctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type projectList struct {
	Items      []projectItem `json:"items"`
	TotalCount int           `json:"totalCount"`
}

type projectItem struct {
	Content  projectContent `json:"content"`
	Effort   string         `json:"effort"`
	ID       string         `json:"id"`
	Labels   []string       `json:"labels"`
	Phase    string         `json:"phase"`
	Priority string         `json:"priority"`
	Status   string         `json:"status"`
	Title    string         `json:"title"`
}

type projectContent struct {
	Number     int    `json:"number"`
	Repository string `json:"repository"`
	State      string `json:"state"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	URL        string `json:"url"`
}

type projectFieldList struct {
	Fields []projectField `json:"fields"`
}

type projectField struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Options []projectFieldOption `json:"options"`
}

type projectFieldOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a app) projectItems(root string) (projectList, error) {
	output, err := a.command(root, "gh", "project", "item-list", strconv.Itoa(projectNumber), "--owner", owner,
		"--format", "json", "--limit", "500")
	if err != nil {
		return projectList{}, fmt.Errorf("list Project items: %w", err)
	}
	var list projectList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return projectList{}, terminalOperation("Project item list", fmt.Errorf("decode Project items: %w", err))
	}
	return list, nil
}

func (a app) projectFields(root string) (projectFieldList, error) {
	output, err := a.command(root, "gh", "project", "field-list", strconv.Itoa(projectNumber), "--owner", owner,
		"--format", "json")
	if err != nil {
		return projectFieldList{}, fmt.Errorf("list Project fields: %w", err)
	}
	var list projectFieldList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return projectFieldList{}, terminalOperation("Project field list", fmt.Errorf("decode Project fields: %w", err))
	}
	return list, nil
}

func (list projectFieldList) option(fieldName, optionName string) (string, string, error) {
	for _, field := range list.Fields {
		if field.Name != fieldName {
			continue
		}
		for _, option := range field.Options {
			if option.Name == optionName {
				return field.ID, option.ID, nil
			}
		}
		return "", "", fmt.Errorf("project field %s has no option %s", fieldName, optionName)
	}
	return "", "", fmt.Errorf("project has no field %s", fieldName)
}

func (a app) setProjectField(root, itemID, fieldName, optionName string) error {
	fields, err := a.projectFields(root)
	if err != nil {
		return err
	}
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

func findProjectIssue(list projectList, number int) (projectItem, error) {
	for _, item := range list.Items {
		if item.Content.Number != number || item.Content.Repository != repositoryKey {
			continue
		}
		if item.Content.Type != "Issue" || strings.TrimSpace(item.ID) == "" {
			continue
		}
		return item, nil
	}
	return projectItem{}, fmt.Errorf("issue #%d is not in Project #%d as a canonical Issue item", number, projectNumber)
}
