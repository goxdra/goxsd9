// Package specs downloads and navigates the pinned W3C specification corpus.
package specs

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	goxsd9 "github.com/goxdra/goxsd9"
)

const (
	// KindSpecification identifies a normative W3C specification entry.
	KindSpecification EntryKind = "specification"
	// KindErratum identifies a W3C errata entry.
	KindErratum EntryKind = "errata"
	// KindBootstrapArtifact identifies a schema-for-schemas artifact.
	KindBootstrapArtifact EntryKind = "bootstrap artifact"
)

// EntryKind identifies the manifest section containing an entry.
type EntryKind string

// Manifest is the versioned, digest-pinned specification manifest.
type Manifest struct {
	BootstrapArtifacts []BootstrapArtifact `json:"bootstrapArtifacts"`
	Errata             []Source            `json:"errata"`
	FormatVersion      int                 `json:"formatVersion"`
	Specifications     []Source            `json:"specifications"`
}

// Source describes a normative specification or erratum.
type Source struct {
	Dependencies []string `json:"dependencies,omitempty"`
	ID           string   `json:"id"`
	SHA256       string   `json:"sha256"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	XSDVersions  []string `json:"xsdVersions"`
}

// BootstrapArtifact describes a schema-for-schemas input and its conversion.
type BootstrapArtifact struct {
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

// Entry is a manifest record with its section and representation resolved.
type Entry struct {
	Aliases        []string
	Dependencies   []string
	Entry          bool
	ID             string
	Kind           EntryKind
	Representation string
	SHA256         string
	Title          string
	URL            string
	XSDVersions    []string
	policy         goxsd9.LanguagePolicy
	policyErr      error
}

// Error classifies a corpus failure with a stable code.
type Error struct {
	Code string
	ID   string
	URL  string
	Err  error
}

func (e *Error) Error() string {
	location := e.ID
	if location == "" {
		location = e.URL
	}
	if location == "" {
		return fmt.Sprintf("[%s] %v", e.Code, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %v", e.Code, location, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// ReadManifest reads and validates specs/manifest.json below root.
func ReadManifest(root string) (Manifest, error) {
	path := filepath.Join(root, "specs", "manifest.json")
	// #nosec G304 -- the caller supplies a repository root and the manifest path is fixed.
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, corpusError("specs.manifest.read", "", path,
			fmt.Errorf("read specification manifest: %w", err))
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, corpusError("specs.manifest.decode", "", path,
			fmt.Errorf("decode specification manifest: %w", err))
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Manifest{}, corpusError("specs.manifest.decode", "", path, err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ValidateManifest checks manifest structure, provenance fields, and references.
func ValidateManifest(manifest Manifest) error {
	if manifest.FormatVersion != 1 {
		return corpusError("specs.manifest.validate", "", "",
			fmt.Errorf("unsupported specification manifest format %d", manifest.FormatVersion))
	}
	if len(manifest.Specifications) == 0 || len(manifest.Errata) == 0 || len(manifest.BootstrapArtifacts) == 0 {
		return corpusError("specs.manifest.validate", "", "",
			errors.New("specification manifest requires specifications, errata, and bootstrap artifacts"))
	}
	ids, err := validateManifestSources(manifest)
	if err != nil {
		return corpusError("specs.manifest.validate", "", "", err)
	}
	if err := validateManifestDependencies(manifest, ids); err != nil {
		return corpusError("specs.manifest.validate", "", "", err)
	}
	return nil
}

// Find returns an entry by its manifest ID in manifest order.
func (manifest Manifest) Find(id string) (Entry, error) {
	if id == "" {
		return Entry{}, corpusError("specs.manifest.lookup", id, "", errors.New("empty specification ID"))
	}
	for _, source := range manifest.Specifications {
		if source.ID == id {
			return sourceEntry(source, KindSpecification), nil
		}
	}
	for _, source := range manifest.Errata {
		if source.ID == id {
			return sourceEntry(source, KindErratum), nil
		}
	}
	for _, artifact := range manifest.BootstrapArtifacts {
		if artifact.ID == id {
			return artifactEntry(artifact), nil
		}
	}
	return Entry{}, corpusError("specs.manifest.lookup", id, "",
		fmt.Errorf("specification ID %q is not in the manifest", id))
}

func sourceEntry(source Source, kind EntryKind) Entry {
	return newEntry(Entry{
		Dependencies:   cloneStrings(source.Dependencies),
		ID:             source.ID,
		Kind:           kind,
		Representation: "html",
		SHA256:         source.SHA256,
		Title:          source.Title,
		URL:            source.URL,
		XSDVersions:    cloneStrings(source.XSDVersions),
	})
}

func artifactEntry(artifact BootstrapArtifact) Entry {
	return newEntry(Entry{
		Aliases:        cloneStrings(artifact.Aliases),
		Dependencies:   cloneStrings(artifact.Dependencies),
		Entry:          artifact.Entry,
		ID:             artifact.ID,
		Kind:           KindBootstrapArtifact,
		Representation: artifact.Representation,
		SHA256:         artifact.SHA256,
		Title:          artifact.Title,
		URL:            artifact.URL,
		XSDVersions:    cloneStrings(artifact.XSDVersions),
	})
}

func newEntry(entry Entry) Entry {
	entry.policy, entry.policyErr = LanguagePolicyForXSDVersions(entry.XSDVersions)
	return entry
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func corpusError(code, id, sourceURL string, err error) error {
	return &Error{Code: code, ID: id, URL: sourceURL, Err: err}
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("finish specification manifest: %w", err)
	}
	return errors.New("specification manifest has trailing JSON values")
}

func validateManifestSources(manifest Manifest) (map[string]struct{}, error) {
	total := len(manifest.Specifications) + len(manifest.Errata) + len(manifest.BootstrapArtifacts)
	ids := make(map[string]struct{}, total)
	for index, source := range manifest.Specifications {
		if err := validateSource(index, KindSpecification, source.ID, source.Title, source.URL, source.SHA256,
			source.XSDVersions, ids); err != nil {
			return nil, err
		}
		ids[source.ID] = struct{}{}
	}
	for index, source := range manifest.Errata {
		if err := validateSource(index, KindErratum, source.ID, source.Title, source.URL, source.SHA256,
			source.XSDVersions, ids); err != nil {
			return nil, err
		}
		ids[source.ID] = struct{}{}
	}
	for index, artifact := range manifest.BootstrapArtifacts {
		if err := validateSource(index, KindBootstrapArtifact, artifact.ID, artifact.Title, artifact.URL,
			artifact.SHA256, artifact.XSDVersions, ids); err != nil {
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

func validateSource(index int, kind EntryKind, id, title, sourceURL, digest string, versions []string,
	seen map[string]struct{},
) error {
	if id == "" || title == "" || sourceURL == "" {
		return fmt.Errorf("%s %d has an empty field", kind, index)
	}
	if !validID(id) {
		return fmt.Errorf("%s %q has an unsafe ID", kind, id)
	}
	if _, ok := seen[id]; ok {
		return fmt.Errorf("%s %d repeats ID %q", kind, index, id)
	}
	if err := validateW3CURL(kind, id, sourceURL); err != nil {
		return err
	}
	if kind == KindSpecification {
		parsed, err := url.Parse(sourceURL)
		if err != nil {
			return fmt.Errorf("specification %q has invalid URL: %w", id, err)
		}
		if !datedTRPath(parsed.Path) {
			return fmt.Errorf("specification %q does not use a dated W3C URL", id)
		}
	}
	if len(digest) != 64 {
		return fmt.Errorf("%s %q has an invalid SHA-256 digest", kind, id)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return fmt.Errorf("%s %q has an invalid SHA-256 digest: %w", kind, id, err)
	}
	return validateXSDVersions(kind, id, versions)
}

func datedTRPath(path string) bool {
	const prefix = "/TR/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	year := strings.TrimPrefix(path, prefix)
	if len(year) < 5 || year[4] != '/' {
		return false
	}
	for _, digit := range year[:4] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func validID(id string) bool {
	for _, char := range id {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return id != ""
}

func validateW3CURL(kind EntryKind, id, sourceURL string) error {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("%s %q has invalid URL: %w", kind, id, err)
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "www.w3.org" || parsed.Port() != "" ||
		parsed.User != nil || !strings.HasPrefix(parsed.Path, "/") || parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return fmt.Errorf("%s %q is not an HTTPS W3C source", kind, id)
	}
	return nil
}

func validateXSDVersions(kind EntryKind, id string, versions []string) error {
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

func validateManifestDependencies(manifest Manifest, ids map[string]struct{}) error {
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

func validateBootstrapArtifacts(artifacts []BootstrapArtifact, ids map[string]struct{}) error {
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

func validateBootstrapArtifact(artifact BootstrapArtifact, ids map[string]struct{}, entries map[string]bool,
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
		parsed, err := url.Parse(alias)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
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
