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
	for _, term := range terms {
		leaves := make([]schemaGroupLeaf, 0)
		if err := collectInlineExtensionLeaves(term, records, groups, Loc{}, &leaves); err != nil {
			return err
		}
		if nested, ok := term.(SequenceParticle); ok {
			if firstNested != nil && firstNestedOptional && len(firstNested) > 0 && len(leaves) > 0 &&
				firstNested[0].name == leaves[0].name && nested.Occurrences().Minimum().String() == "0" {
				// The first nested term can also be skipped. Its first name then
				// competes with the second nested term's first name.
				return inlineExtensionPairAmbiguous(leaves[0], firstNested[0], version)
			}
			if firstNested == nil {
				firstNested = leaves
				firstNestedOptional = nested.Occurrences().Minimum().String() == "0"
			}
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

func inlineExtensionPairLeafConflict(first, later schemaGroupLeaf, records []schemaComponentRecord, version XSDVersion) error {
	if first.name == later.name && !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
		return newSchemaGroupElementConsistencyDiagnostic(later, first, version)
	}
	if first.name == later.name || schemaGroupSubstitutionMayMatch(records, first.name, later.name) || schemaGroupSubstitutionMayMatch(records, later.name, first.name) {
		diagnostic := newSchemaSyntaxUnsupportedForVersion(later.loc, fmt.Sprintf("inline extension sequence references %q and %q require further attribution proof", first.name, later.name), version)
		diagnostic.related = []Loc{first.loc}
		diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
		return diagnostic
	}
	return nil
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
