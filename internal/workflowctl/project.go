package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (list *projectList) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var response struct {
		Items      json.RawMessage `json:"items"`
		TotalCount *int            `json:"totalCount"`
	}
	if err := decoder.Decode(&response); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple Project item responses")
		}
		return fmt.Errorf("trailing Project item response data: %w", err)
	}
	if len(response.Items) == 0 || bytes.Equal(bytes.TrimSpace(response.Items), []byte("null")) {
		return errors.New("project item response is missing items")
	}
	if response.TotalCount == nil {
		response.TotalCount = new(int)
	}
	if *response.TotalCount < 0 {
		return fmt.Errorf("project item response has negative totalCount %d", *response.TotalCount)
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(response.Items, &rawItems); err != nil {
		return fmt.Errorf("decode Project item list: %w", err)
	}
	items := make([]projectItem, len(rawItems))
	for index, rawItem := range rawItems {
		if err := validateProjectItemShape(rawItem, index); err != nil {
			return err
		}
		if err := json.Unmarshal(rawItem, &items[index]); err != nil {
			return fmt.Errorf("decode Project item %d: %w", index, err)
		}
	}
	*list = projectList{Items: items, TotalCount: *response.TotalCount}
	return nil
}

func validateProjectItemShape(data []byte, index int) error {
	var item struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return fmt.Errorf("decode Project item %d: %w", index, err)
	}
	if len(item.Content) == 0 || bytes.Equal(bytes.TrimSpace(item.Content), []byte("null")) {
		return fmt.Errorf("project item %d has no classifiable content", index)
	}
	var content struct {
		Number     *int    `json:"number"`
		Repository *string `json:"repository"`
		Type       *string `json:"type"`
	}
	if err := json.Unmarshal(item.Content, &content); err != nil {
		return fmt.Errorf("decode Project item %d content: %w", index, err)
	}
	if content.Type == nil || strings.TrimSpace(*content.Type) == "" {
		return fmt.Errorf("project item %d has no classifiable content", index)
	}
	if *content.Type != "Issue" {
		return nil
	}
	if content.Number == nil || *content.Number <= 0 || content.Repository == nil || strings.TrimSpace(*content.Repository) == "" {
		return fmt.Errorf("project item %d has no classifiable content", index)
	}
	return nil
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
		return projectList{}, fmt.Errorf("decode Project items: %w", err)
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
		return projectFieldList{}, fmt.Errorf("decode Project fields: %w", err)
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
