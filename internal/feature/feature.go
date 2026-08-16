// Package feature owns the checked registry of unsupported specification
// capabilities.
package feature

import (
	"errors"
	"fmt"
	"strings"
)

// ID identifies a specification capability in the repository registry.
type ID string

// Reference identifies the pinned specification section for one XSD version.
type Reference struct {
	version string
	source  string
}

// XSDVersion returns the XSD version to which the reference applies.
func (reference Reference) XSDVersion() string {
	return reference.version
}

// Source returns the pinned specification ID and section anchor.
func (reference Reference) Source() string {
	return reference.source
}

// Feature is an opaque handle to a registered specification capability.
//
// The zero value is not a registered feature. Handles returned by Lookup and
// All remain valid only while the registry is unchanged.
type Feature struct {
	id    ID
	index int
}

// Registered reports whether the handle identifies its registry entry.
func (feature Feature) Registered() bool {
	if feature.id == "" || feature.index < 0 || feature.index >= len(registry) {
		return false
	}
	return registry[feature.index].id == feature.id
}

// ID returns the stable identifier of the feature.
func (feature Feature) ID() ID {
	return feature.id
}

// Title returns the semantic capability title.
func (feature Feature) Title() string {
	if !feature.Registered() {
		return ""
	}
	return registry[feature.index].title
}

// References returns the feature's specification references in registry order.
func (feature Feature) References() []Reference {
	if !feature.Registered() {
		return nil
	}
	references := registry[feature.index].references
	return append([]Reference(nil), references...)
}

// SpecRef returns the first applicable pinned specification reference.
func (feature Feature) SpecRef() string {
	references := feature.References()
	if len(references) == 0 {
		return ""
	}
	return references[0].Source()
}

type definition struct {
	id         ID
	title      string
	references []Reference
}

// registry is the ordered source of truth. Keep entries sorted by exact ID.
var registry = []definition{
	{
		id:    "xsd.assertion",
		title: "XSD assertions",
		references: []Reference{
			{version: "1.1", source: "xsd11-structures#cAssertions"},
		},
	},
	{
		id:    "xsd.datatype.facets",
		title: "XSD datatype facets",
		references: []Reference{
			{version: "1.0", source: "xsd10-datatypes#decimal"},
			{version: "1.1", source: "xsd11-datatypes#decimal"},
		},
	},
	{
		id:    "xsd.datatype.precision-decimal",
		title: "XSD precisionDecimal",
		references: []Reference{
			{version: "1.1", source: "xsd-precisionDecimal#precisionDecimal"},
		},
	},
	{
		id:    "xsd.schema.syntax",
		title: "XSD schema syntax outside the bootstrap kernel",
		references: []Reference{
			{version: "1.0", source: "xsd10-structures#schema-document"},
			{version: "1.1", source: "xsd11-structures#cSchemaDocument"},
		},
	},
}

// All returns registered features in their canonical order.
func All() []Feature {
	features := make([]Feature, 0, len(registry))
	for index, definition := range registry {
		features = append(features, Feature{id: definition.id, index: index})
	}
	return features
}

// Lookup finds a registered feature by exact ID.
func Lookup(id ID) (Feature, bool) {
	for index, definition := range registry {
		if definition.id == id {
			return Feature{id: definition.id, index: index}, true
		}
	}
	return Feature{}, false
}

// ValidateRegistry validates the canonical feature registry.
func ValidateRegistry() error {
	return validateDefinitions(registry)
}

func validateDefinitions(definitions []definition) error {
	if len(definitions) == 0 {
		return errors.New("feature registry is empty")
	}
	seen := make(map[ID]struct{}, len(definitions))
	for index, definition := range definitions {
		if err := validateID(definition.id); err != nil {
			return fmt.Errorf("feature %d: %w", index, err)
		}
		if _, ok := seen[definition.id]; ok {
			return fmt.Errorf("feature %d repeats ID %q", index, definition.id)
		}
		seen[definition.id] = struct{}{}
		if index > 0 && definitions[index-1].id >= definition.id {
			return fmt.Errorf("feature IDs are not unique and sorted at %q", definition.id)
		}
		if strings.TrimSpace(definition.title) == "" {
			return fmt.Errorf("feature %q has an empty title", definition.id)
		}
		if err := validateReferences(definition.id, definition.references); err != nil {
			return err
		}
	}
	return nil
}

func validateID(id ID) error {
	text := string(id)
	parts := strings.Split(text, ".")
	if len(parts) < 2 {
		return fmt.Errorf("feature ID %q must contain a namespace and name", id)
	}
	if parts[0] != "xsd" {
		return fmt.Errorf("feature ID %q must use the xsd namespace", id)
	}
	for _, part := range parts {
		if !validSegment(part) {
			return fmt.Errorf("feature ID %q is malformed", id)
		}
	}
	return nil
}

func validSegment(segment string) bool {
	if segment == "" || !isLowerASCII(segment[0]) {
		return false
	}
	previousHyphen := false
	for index := 1; index < len(segment); index++ {
		character := segment[index]
		if character == '-' {
			if previousHyphen {
				return false
			}
			previousHyphen = true
			continue
		}
		if !isLowerASCII(character) && (character < '0' || character > '9') {
			return false
		}
		previousHyphen = false
	}
	return !previousHyphen
}

func isLowerASCII(character byte) bool {
	return character >= 'a' && character <= 'z'
}

func validateReferences(id ID, references []Reference) error {
	if len(references) == 0 {
		return fmt.Errorf("feature %q has no specification reference", id)
	}
	seenVersions := make(map[string]struct{}, len(references))
	for index, reference := range references {
		if reference.version != "1.0" && reference.version != "1.1" {
			return fmt.Errorf("feature %q has invalid XSD version %q", id, reference.version)
		}
		if _, ok := seenVersions[reference.version]; ok {
			return fmt.Errorf("feature %q repeats XSD version %q", id, reference.version)
		}
		seenVersions[reference.version] = struct{}{}
		if index > 0 && references[index-1].version >= reference.version {
			return fmt.Errorf("feature %q references are not unique and sorted", id)
		}
		if err := validateReferenceSource(id, reference.source); err != nil {
			return err
		}
	}
	return nil
}

func validateReferenceSource(id ID, source string) error {
	parts := strings.Split(source, "#")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("feature %q has malformed specification reference %q", id, source)
	}
	if strings.TrimSpace(source) != source || strings.ContainsAny(source, " \t\r\n") {
		return fmt.Errorf("feature %q has malformed specification reference %q", id, source)
	}
	return nil
}
