package workflowctl

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type specificationManifest struct {
	BootstrapArtifacts []bootstrapArtifact   `json:"bootstrapArtifacts"`
	Errata             []specificationSource `json:"errata"`
	FormatVersion      int                   `json:"formatVersion"`
	Specifications     []specificationSource `json:"specifications"`
}

type specificationSource struct {
	Dependencies []string `json:"dependencies,omitempty"`
	ID           string   `json:"id"`
	SHA256       string   `json:"sha256"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	XSDVersions  []string `json:"xsdVersions"`
}

type bootstrapArtifact struct {
	Aliases        []string `json:"aliases,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	Entry          bool     `json:"entry"`
	ID             string   `json:"id"`
	SHA256         string   `json:"sha256"`
	Title          string   `json:"title"`
	URL            string   `json:"url"`
	XSDVersions    []string `json:"xsdVersions"`
	Representation string   `json:"representation"`
}

func checkSpecManifest(root string) error {
	manifest, err := readSpecManifest(root)
	if err != nil {
		return err
	}
	if manifest.FormatVersion != 1 {
		return fmt.Errorf("unsupported specification manifest format %d", manifest.FormatVersion)
	}
	if len(manifest.Specifications) == 0 || len(manifest.Errata) == 0 || len(manifest.BootstrapArtifacts) == 0 {
		return errors.New("specification manifest requires specifications, errata, and bootstrap artifacts")
	}
	ids, err := validateManifestSources(manifest)
	if err != nil {
		return err
	}
	return validateManifestDependencies(manifest, ids)
}

func readSpecManifest(root string) (specificationManifest, error) {
	path := filepath.Join(root, "specs", "manifest.json")
	// #nosec G304 -- path is a fixed file below the repository root.
	data, err := os.ReadFile(path)
	if err != nil {
		return specificationManifest{}, fmt.Errorf("read specification manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest specificationManifest
	if err := decoder.Decode(&manifest); err != nil {
		return specificationManifest{}, fmt.Errorf("decode specification manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return specificationManifest{}, err
	}
	return manifest, nil
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

func validateManifestSources(manifest specificationManifest) (map[string]struct{}, error) {
	total := len(manifest.Specifications) + len(manifest.Errata) + len(manifest.BootstrapArtifacts)
	ids := make(map[string]struct{}, total)
	for index, source := range manifest.Specifications {
		if err := validateSource(index, "specification", source.ID, source.Title, source.URL, source.SHA256,
			source.XSDVersions, ids); err != nil {
			return nil, err
		}
		ids[source.ID] = struct{}{}
	}
	for index, source := range manifest.Errata {
		if err := validateSource(index, "errata", source.ID, source.Title, source.URL, source.SHA256,
			source.XSDVersions, ids); err != nil {
			return nil, err
		}
		ids[source.ID] = struct{}{}
	}
	for index, artifact := range manifest.BootstrapArtifacts {
		if err := validateSource(index, "bootstrap artifact", artifact.ID, artifact.Title, artifact.URL, artifact.SHA256,
			artifact.XSDVersions, ids); err != nil {
			return nil, err
		}
		if artifact.Representation != "xml" && artifact.Representation != "html-cdata-pre" {
			return nil, fmt.Errorf("bootstrap artifact %q has invalid representation %q", artifact.ID,
				artifact.Representation)
		}
		ids[artifact.ID] = struct{}{}
	}
	return ids, nil
}

func validateSource(index int, kind, id, title, sourceURL, digest string, versions []string, seen map[string]struct{}) error {
	if id == "" || title == "" || sourceURL == "" {
		return fmt.Errorf("%s %d has an empty field", kind, index)
	}
	if _, ok := seen[id]; ok {
		return fmt.Errorf("%s %d repeats ID %q", kind, index, id)
	}
	if !strings.HasPrefix(sourceURL, "https://www.w3.org/") {
		return fmt.Errorf("%s %q is not an HTTPS W3C source", kind, id)
	}
	if kind == "specification" && !strings.Contains(sourceURL, "/TR/19") && !strings.Contains(sourceURL, "/TR/20") {
		return fmt.Errorf("specification %q does not use a dated W3C URL", id)
	}
	if len(digest) != 64 {
		return fmt.Errorf("%s %q has an invalid SHA-256 digest", kind, id)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return fmt.Errorf("%s %q has an invalid SHA-256 digest: %w", kind, id, err)
	}
	return validateXSDVersions(kind, id, versions)
}

func validateXSDVersions(kind, id string, versions []string) error {
	if len(versions) == 0 {
		return fmt.Errorf("%s %q has no XSD version", kind, id)
	}
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		if version != "1.0" && version != "1.1" {
			return fmt.Errorf("%s %q has invalid XSD version %q", kind, id, version)
		}
		if _, ok := seen[version]; ok {
			return fmt.Errorf("%s %q repeats XSD version %q", kind, id, version)
		}
		seen[version] = struct{}{}
	}
	return nil
}

func validateManifestDependencies(manifest specificationManifest, ids map[string]struct{}) error {
	for _, source := range manifest.Specifications {
		if err := validateDependencies(source.ID, source.Dependencies, ids); err != nil {
			return err
		}
	}
	for _, source := range manifest.Errata {
		if err := validateDependencies(source.ID, source.Dependencies, ids); err != nil {
			return err
		}
	}
	return validateBootstrapArtifacts(manifest.BootstrapArtifacts, ids)
}

func validateBootstrapArtifacts(artifacts []bootstrapArtifact, ids map[string]struct{}) error {
	entries := make(map[string]bool, 2)
	aliases := make(map[string]string)
	for _, artifact := range artifacts {
		if err := validateBootstrapArtifact(artifact, ids, entries, aliases); err != nil {
			return err
		}
	}
	if !entries["1.0"] || !entries["1.1"] {
		return errors.New("bootstrap artifacts require entry documents for XSD 1.0 and 1.1")
	}
	if aliases["http://www.w3.org/2001/xml.xsd"] != "xml-schema" {
		return errors.New("xml-schema must resolve the lexical xml.xsd import URL")
	}
	return nil
}

func validateBootstrapArtifact(artifact bootstrapArtifact, ids map[string]struct{}, entries map[string]bool,
	aliases map[string]string,
) error {
	if err := validateDependencies(artifact.ID, artifact.Dependencies, ids); err != nil {
		return err
	}
	if artifact.Entry {
		for _, version := range artifact.XSDVersions {
			entries[version] = true
		}
	}
	for _, alias := range artifact.Aliases {
		if !strings.HasPrefix(alias, "http://") && !strings.HasPrefix(alias, "https://") {
			return fmt.Errorf("bootstrap artifact %q has invalid alias %q", artifact.ID, alias)
		}
		if existing, ok := aliases[alias]; ok {
			return fmt.Errorf("alias %q is shared by %q and %q", alias, existing, artifact.ID)
		}
		aliases[alias] = artifact.ID
	}
	return nil
}

func validateDependencies(id string, dependencies []string, ids map[string]struct{}) error {
	seen := make(map[string]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		if _, ok := ids[dependency]; !ok {
			return fmt.Errorf("source %q has unknown dependency %q", id, dependency)
		}
		if _, ok := seen[dependency]; ok {
			return fmt.Errorf("source %q repeats dependency %q", id, dependency)
		}
		seen[dependency] = struct{}{}
	}
	return nil
}
