package goxsd9

import "fmt"

// validateInlineExtensionSequencePairOverlaps checks only the two-sibling
// anonymous-extension admission after all referenced groups have resolved.
func validateInlineExtensionSequencePairOverlaps(records []schemaComponentRecord, complexTypes []schemaComplexTypeResult, groups []schemaModelGroupResult, version XSDVersion) error { //nolint:gocognit // Only completed inline owners with two surviving nested terms enter the bounded proof.
	for index, record := range records {
		if record.element == nil || record.element.inlineComplexType == nil || !complexTypes[index].present {
			continue
		}
		body, ok := complexTypes[index].body.(*schemaComplexTypeExtensionBodyResult)
		if !ok || body == nil {
			continue
		}
		outer, ok := body.particle.(SequenceParticle)
		if !ok {
			continue
		}
		terms := outer.Particles()
		nestedCount := 0
		for _, term := range terms {
			if _, nested := term.(SequenceParticle); nested {
				nestedCount++
			}
		}
		if nestedCount != 2 {
			continue
		}
		if err := validateInlineExtensionSequencePairTerms(terms, records, groups, version); err != nil {
			return err
		}
	}
	return nil
}

func validateInlineExtensionSequencePairTerms(terms []Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, version XSDVersion) error { //nolint:gocognit // Compare source-ordered leaves and return the first classified conflict.
	seen := make([]schemaGroupLeaf, 0)
	var firstNested []schemaGroupLeaf
	firstNestedOptional := false
	previousNested := false
	for _, term := range terms {
		wasPreviousNested := previousNested
		previousNested = false
		leaves := make([]schemaGroupLeaf, 0)
		if err := collectInlineExtensionLeaves(term, records, groups, Loc{}, &leaves); err != nil {
			return err
		}
		if nested, ok := term.(SequenceParticle); ok {
			if err := validateInlineExtensionNestedTerm(nested, records, groups, version); err != nil {
				return err
			}
			if wasPreviousNested && firstNested != nil && firstNestedOptional && len(firstNested) > 0 && len(leaves) > 0 &&
				firstNested[0].name == leaves[0].name && nested.Occurrences().Minimum().String() == "0" {
				if !firstNested[0].declaredType.IsZero() && !leaves[0].declaredType.IsZero() && firstNested[0].declaredType != leaves[0].declaredType {
					return newSchemaGroupElementConsistencyDiagnostic(leaves[0], firstNested[0], version)
				}
				// Adjacent optional terms can choose either first element.
				return inlineExtensionPairAmbiguous(leaves[0], firstNested[0], version)
			}
			if firstNested == nil {
				firstNested = leaves
				firstNestedOptional = nested.Occurrences().Minimum().String() == "0"
			}
			previousNested = true
		}
		for _, leaf := range leaves {
			for _, first := range seen {
				if err := inlineExtensionPairLeafConflict(first, leaf, records, version); err != nil {
					return err
				}
			}
		}
		seen = append(seen, leaves...)
	}
	return nil
}

func validateInlineExtensionNestedTerm(nested SequenceParticle, records []schemaComponentRecord, groups []schemaModelGroupResult, version XSDVersion) error {
	parts := nested.Particles()
	if len(parts) != 2 {
		return newSchemaBridgeInvariant(nested.Loc(), "inline extension nested sequence lost its two terms")
	}
	group, groupOK := parts[0].(ModelGroupReferenceParticle)
	element, elementOK := parts[1].(ElementReferenceParticle)
	if !groupOK || !elementOK {
		return newSchemaBridgeInvariant(nested.Loc(), "inline extension nested sequence lost its group and element references")
	}
	groupLeaves := make([]schemaGroupLeaf, 0)
	if err := collectInlineExtensionLeaves(group, records, groups, Loc{}, &groupLeaves); err != nil {
		return err
	}
	elementLeaves := make([]schemaGroupLeaf, 0, 1)
	if err := collectInlineExtensionLeaves(element, records, groups, Loc{}, &elementLeaves); err != nil {
		return err
	}
	if len(elementLeaves) != 1 {
		return newSchemaBridgeInvariant(element.RefLoc(), "inline extension element reference lost its declaration")
	}
	for _, leaf := range groupLeaves {
		if leaf.name == elementLeaves[0].name && inlineExtensionGroupSingleOptionalElement(group, groups, records) && leaf.declaredType == elementLeaves[0].declaredType {
			return inlineExtensionPairAmbiguous(elementLeaves[0], leaf, version)
		}
		if err := inlineExtensionPairLeafConflict(leaf, elementLeaves[0], records, version); err != nil {
			return err
		}
	}
	return nil
}

func inlineExtensionGroupSingleOptionalElement(group ModelGroupReferenceParticle, groups []schemaModelGroupResult, records []schemaComponentRecord) bool {
	index := schemaGroupRecordByID(records, group.TargetID())
	if index < 0 || !groups[index].present {
		return false
	}
	sequence, ok := groups[index].particle.(SequenceParticle)
	if !ok || len(sequence.Particles()) != 1 {
		return false
	}
	element, ok := sequence.Particles()[0].(ElementReferenceParticle)
	return ok && element.Occurrences().Minimum().String() == "0"
}

func inlineExtensionPairLeafConflict(first, later schemaGroupLeaf, records []schemaComponentRecord, version XSDVersion) error {
	if first.name == later.name && !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
		return newSchemaGroupElementConsistencyDiagnostic(later, first, version)
	}
	if first.name == later.name || schemaGroupSubstitutionMayMatch(records, first.name, later.name) || schemaGroupSubstitutionMayMatch(records, later.name, first.name) || inlineExtensionPairSharedMember(records, first, later) {
		diagnostic := newSchemaSyntaxUnsupportedForVersion(later.loc, fmt.Sprintf("inline extension sequence references %q and %q require further attribution proof", first.name, later.name), version)
		diagnostic.related = []Loc{first.loc}
		diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
		return diagnostic
	}
	return nil
}

func inlineExtensionPairSharedMember(records []schemaComponentRecord, first, later schemaGroupLeaf) bool {
	if !first.reference || !later.reference {
		return false
	}
	for _, record := range records {
		if record.element == nil || record.name.IsZero() {
			continue
		}
		if schemaGroupSubstitutionMayMatch(records, first.name, record.name) && schemaGroupSubstitutionMayMatch(records, later.name, record.name) {
			return true
		}
	}
	return false
}

func inlineExtensionPairAmbiguous(later, first schemaGroupLeaf, version XSDVersion) Diagnostic {
	return Diagnostic{
		class: FailureInvalid, code: diagnosticSchemaInlineExtensionPairAmbiguousCode,
		loc: later.loc, related: []Loc{first.loc},
		message: fmt.Sprintf("optional nested sequences have ambiguous first element %q", later.name),
		specRef: schemaGroupNonAmbiguousSpecRef(version),
		cause:   errSchemaInlineExtensionPairAmbiguous,
	}
}

func collectInlineExtensionLeaves(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, via Loc, leaves *[]schemaGroupLeaf) error { //nolint:gocognit // Every resolved particle variant contributes ordered leaves without publishing expansion.
	switch term := particle.(type) {
	case ElementReferenceParticle:
		index := schemaGroupRecordByID(records, term.TargetID())
		if index < 0 || records[index].element == nil {
			return newSchemaBridgeInvariant(term.RefLoc(), "inline extension element target is missing")
		}
		*leaves = append(*leaves, schemaGroupLeaf{name: term.Ref(), declaredType: records[index].element.declaredType, loc: schemaGroupLeafLoc(term.RefLoc(), via), reference: true})
	case ElementParticle:
		*leaves = append(*leaves, schemaGroupLeaf{name: term.Name(), declaredType: term.DeclaredType(), loc: schemaGroupLeafLoc(term.Loc(), via), local: true})
	case ModelGroupReferenceParticle:
		index := schemaGroupRecordByID(records, term.TargetID())
		if index < 0 || !groups[index].present {
			return newSchemaBridgeInvariant(term.RefLoc(), "inline extension group target is missing")
		}
		if groups[index].particle == nil {
			return nil
		}
		return collectInlineExtensionLeaves(groups[index].particle, records, groups, term.RefLoc(), leaves)
	case SequenceParticle:
		for _, child := range term.Particles() {
			if err := collectInlineExtensionLeaves(child, records, groups, via, leaves); err != nil {
				return err
			}
		}
	case ChoiceParticle:
		for _, child := range term.Alternatives() {
			if err := collectInlineExtensionLeaves(child, records, groups, via, leaves); err != nil {
				return err
			}
		}
	default:
		return newSchemaBridgeInvariant(particle.Loc(), "inline extension pair has an unexpected particle")
	}
	return nil
}
