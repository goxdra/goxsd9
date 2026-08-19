package specs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	bootstrapArtifactCode   = "specs.bootstrap.artifact"
	bootstrapCycleCode      = "specs.bootstrap.cycle"
	bootstrapDependencyCode = "specs.bootstrap.dependency"
	bootstrapEntryCode      = "specs.bootstrap.entry"
	bootstrapVersionCode    = "specs.bootstrap.version"
)

// BootstrapPlan is an immutable, dependency-first view of one XSD version's
// manifest bootstrap artifacts.
type BootstrapPlan struct {
	version string
	entries []Entry
}

// Version returns the XSD version selected by the plan.
func (plan BootstrapPlan) Version() string {
	return plan.version
}

// Entries returns the planned artifacts in materialization order.
func (plan BootstrapPlan) Entries() []Entry {
	return cloneEntries(plan.entries)
}

// BootstrapPlan selects and orders the bootstrap artifacts for one XSD
// version. Dependencies are prepared before their dependents, with manifest
// order deciding between otherwise-ready artifacts.
func (manifest Manifest) BootstrapPlan(version string) (BootstrapPlan, error) {
	if version != "1.0" && version != "1.1" {
		return BootstrapPlan{}, corpusError(bootstrapVersionCode, "", "",
			fmt.Errorf("unsupported bootstrap XSD version %q", version))
	}

	artifactIndices, err := indexBootstrapArtifacts(manifest.BootstrapArtifacts)
	if err != nil {
		return BootstrapPlan{}, err
	}
	selected := selectBootstrapArtifacts(manifest.BootstrapArtifacts, version)
	if len(selected) == 0 {
		return BootstrapPlan{}, corpusError(bootstrapEntryCode, "", "",
			errors.New("bootstrap version has no selected artifacts"))
	}

	if entryErr := requireBootstrapEntry(version, selected); entryErr != nil {
		return BootstrapPlan{}, entryErr
	}

	indegree, dependents, err := bootstrapDependencyGraph(selected, artifactIndices, version)
	if err != nil {
		return BootstrapPlan{}, err
	}

	ordered, err := orderBootstrapArtifacts(selected, indegree, dependents)
	if err != nil {
		return BootstrapPlan{}, err
	}

	return BootstrapPlan{version: version, entries: ordered}, nil
}

func indexBootstrapArtifacts(artifacts []BootstrapArtifact) (map[string]int, error) {
	indices := make(map[string]int, len(artifacts))
	for index, artifact := range artifacts {
		if artifact.ID == "" {
			return nil, bootstrapArtifactError(artifact,
				fmt.Errorf("bootstrap artifact at manifest index %d has an empty ID", index))
		}
		if _, exists := indices[artifact.ID]; exists {
			return nil, bootstrapArtifactError(artifact,
				fmt.Errorf("bootstrap artifact ID %q is repeated", artifact.ID))
		}
		indices[artifact.ID] = index
	}
	return indices, nil
}

func selectBootstrapArtifacts(artifacts []BootstrapArtifact, version string) []BootstrapArtifact {
	selected := make([]BootstrapArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if hasXSDVersion(artifact.XSDVersions, version) {
			selected = append(selected, artifact)
		}
	}
	return selected
}

func requireBootstrapEntry(version string, artifacts []BootstrapArtifact) error {
	entryCount := 0
	for _, artifact := range artifacts {
		if artifact.Entry {
			entryCount++
		}
	}
	if entryCount == 1 {
		return nil
	}
	return corpusError(bootstrapEntryCode, "", "",
		fmt.Errorf("bootstrap XSD version %q has %d entry artifacts, want exactly one", version, entryCount))
}

func bootstrapDependencyGraph(
	artifacts []BootstrapArtifact,
	artifactIndices map[string]int,
	version string,
) ([]int, [][]int, error) {
	selectedIndices := make(map[string]int, len(artifacts))
	for index, artifact := range artifacts {
		selectedIndices[artifact.ID] = index
	}

	indegree := make([]int, len(artifacts))
	dependents := make([][]int, len(artifacts))
	for index, artifact := range artifacts {
		if err := addBootstrapDependencies(index, artifact, selectedIndices, artifactIndices, version, indegree, dependents); err != nil {
			return nil, nil, err
		}
	}
	return indegree, dependents, nil
}

func addBootstrapDependencies(
	index int,
	artifact BootstrapArtifact,
	selectedIndices map[string]int,
	artifactIndices map[string]int,
	version string,
	indegree []int,
	dependents [][]int,
) error {
	seenDependencies := make(map[string]struct{}, len(artifact.Dependencies))
	for _, dependency := range artifact.Dependencies {
		if _, seen := seenDependencies[dependency]; seen {
			return bootstrapDependencyError(artifact,
				fmt.Errorf("bootstrap dependency %q is repeated", dependency))
		}
		seenDependencies[dependency] = struct{}{}

		dependencyIndex, selectedDependency := selectedIndices[dependency]
		if !selectedDependency {
			if _, isArtifact := artifactIndices[dependency]; !isArtifact {
				return bootstrapDependencyError(artifact,
					fmt.Errorf("bootstrap dependency %q is missing from the manifest", dependency))
			}
			return bootstrapDependencyError(artifact,
				fmt.Errorf("bootstrap dependency %q is not selected for XSD version %s", dependency, version))
		}
		indegree[index]++
		dependents[dependencyIndex] = append(dependents[dependencyIndex], index)
	}
	return nil
}

func orderBootstrapArtifacts(artifacts []BootstrapArtifact, indegree []int, dependents [][]int) ([]Entry, error) {
	ordered := make([]Entry, 0, len(artifacts))
	completed := make([]bool, len(artifacts))
	for len(ordered) < len(artifacts) {
		next := nextBootstrapArtifact(completed, indegree)
		if next < 0 {
			return nil, bootstrapCycleError(artifacts[0],
				errors.New("bootstrap artifact dependencies contain a cycle"))
		}

		completed[next] = true
		ordered = append(ordered, artifactEntry(artifacts[next]))
		for _, dependent := range dependents[next] {
			indegree[dependent]--
		}
	}
	return ordered, nil
}

func nextBootstrapArtifact(completed []bool, indegree []int) int {
	for index := range completed {
		if completed[index] || indegree[index] != 0 {
			continue
		}
		return index
	}
	return -1
}

// GenerateBootstrap materializes one version's bootstrap plan sequentially.
// It returns no documents when any planned artifact fails; Generate's error is
// returned unchanged so its diagnostic code and cause remain available.
func GenerateBootstrap(ctx context.Context, client *http.Client, manifest Manifest, version string) ([]Document, error) {
	plan, err := manifest.BootstrapPlan(version)
	if err != nil {
		return nil, err
	}

	entries := plan.Entries()
	documents := make([]Document, 0, len(entries))
	for _, entry := range entries {
		document, err := Generate(ctx, client, entry)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func hasXSDVersion(versions []string, want string) bool {
	for _, version := range versions {
		if version == want {
			return true
		}
	}
	return false
}

func bootstrapArtifactError(artifact BootstrapArtifact, err error) error {
	return corpusError(bootstrapArtifactCode, artifact.ID, artifact.URL, err)
}

func bootstrapDependencyError(artifact BootstrapArtifact, err error) error {
	return corpusError(bootstrapDependencyCode, artifact.ID, artifact.URL, err)
}

func bootstrapCycleError(artifact BootstrapArtifact, err error) error {
	return corpusError(bootstrapCycleCode, artifact.ID, artifact.URL, err)
}

func cloneEntries(entries []Entry) []Entry {
	if entries == nil {
		return nil
	}
	cloned := make([]Entry, len(entries))
	for index, entry := range entries {
		cloned[index] = cloneEntry(entry)
	}
	return cloned
}

func cloneEntry(entry Entry) Entry {
	entry.Aliases = cloneStrings(entry.Aliases)
	entry.Dependencies = cloneStrings(entry.Dependencies)
	entry.XSDVersions = cloneStrings(entry.XSDVersions)
	return entry
}
