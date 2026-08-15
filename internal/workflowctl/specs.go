package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type specificationManifest struct {
	Specifications []specificationSource `json:"specifications"`
}

type specificationSource struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func checkSpecManifest(root string) error {
	path := filepath.Join(root, "specs", "manifest.json")
	// #nosec G304 -- path is a fixed file below the repository root.
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read specification manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest specificationManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode specification manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return err
	}
	if len(manifest.Specifications) == 0 {
		return errors.New("specification manifest is empty")
	}
	seen := make(map[string]struct{}, len(manifest.Specifications))
	for index, source := range manifest.Specifications {
		if err := validateSpecificationSource(index, source, seen); err != nil {
			return err
		}
		seen[source.ID] = struct{}{}
	}
	return nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("finish specification manifest: %w", err)
	}
	return errors.New("specification manifest has trailing JSON values")
}

func validateSpecificationSource(index int, source specificationSource, seen map[string]struct{}) error {
	if source.ID == "" || source.Title == "" || source.URL == "" {
		return fmt.Errorf("specification %d has an empty field", index)
	}
	if _, ok := seen[source.ID]; ok {
		return fmt.Errorf("specification %d repeats ID %q", index, source.ID)
	}
	if !strings.HasPrefix(source.URL, "https://www.w3.org/TR/20") {
		return fmt.Errorf("specification %q does not use a dated W3C URL", source.ID)
	}
	return nil
}
