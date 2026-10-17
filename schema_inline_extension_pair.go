package goxsd9

import "fmt"

// validateInlineExtensionSequencePairOverlaps checks the authored two-sibling
// admission after reference resolution, including surviving terms after omission.
func validateInlineExtensionSequencePairOverlaps(records []schemaComponentRecord, complexTypes []schemaComplexTypeResult, groups []schemaModelGroupResult, version XSDVersion) error { //nolint:gocognit // Select by authored shape, then check the completed live particle tree.
	for index, record := range records {
		if record.element == nil || record.element.inlineComplexType == nil || !complexTypes[index].present {
			continue
		}
		inputBody, ok := record.element.inlineComplexType.body.(*schemaComplexTypeExtensionBodyInput)
		if !ok || inputBody == nil {
			continue
		}
		inputOuter, ok := inputBody.particle.(*schemaSequenceParticleInput)
		if !ok || !inlineExtensionAuthoredSequencePair(inputOuter) {
			continue
		}
		body, ok := complexTypes[index].body.(*schemaComplexTypeExtensionBodyResult)
		if !ok || body == nil {
			return newSchemaBridgeInvariant(record.loc, "authored inline extension pair has no completed extension")
		}
		outer, ok := body.particle.(SequenceParticle)
		if !ok {
			if body.particle == nil && !inputOuter.occurrences.mapsToParticle() {
				continue
			}
			return newSchemaBridgeInvariant(record.loc, "authored inline extension pair has no completed sequence")
		}
		if err := validateInlineExtensionSequencePairTerms(outer.Particles(), records, groups, version); err != nil {
			return err
		}
	}
	return nil
}

func inlineExtensionAuthoredSequencePair(outer *schemaSequenceParticleInput) bool {
	if outer == nil {
		return false
	}
	count := 0
	for _, term := range outer.particles {
		if _, nested := term.(*schemaSequenceParticleInput); nested {
			count++
		}
	}
	return count == 2
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
				if inlineExtensionPairSameReferenceHasMatch(records, firstNested[0], leaves[0]) {
					// Adjacent optional terms can choose either first element.
					return inlineExtensionPairAmbiguous(leaves[0], firstNested[0], version)
				}
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
	if len(parts) == 0 || len(parts) > 2 {
		return newSchemaBridgeInvariant(nested.Loc(), "inline extension nested sequence has an unexpected resolved shape")
	}
	group, groupOK := parts[0].(ModelGroupReferenceParticle)
	if !groupOK {
		return newSchemaBridgeInvariant(nested.Loc(), "inline extension nested sequence lost its group reference")
	}
	if err := validateInlineExtensionReferencedGroup(group, records, groups, version); err != nil {
		return err
	}
	if len(parts) == 1 {
		return nil
	}
	element, elementOK := parts[1].(ElementReferenceParticle)
	if !elementOK {
		return newSchemaBridgeInvariant(nested.Loc(), "inline extension nested sequence lost its element reference")
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
		if leaf.name == elementLeaves[0].name && inlineExtensionGroupSingleOptionalElement(group, groups, records) && leaf.declaredType == elementLeaves[0].declaredType && inlineExtensionPairSameReferenceHasMatch(records, leaf, elementLeaves[0]) {
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

func validateInlineExtensionReferencedGroup(group ModelGroupReferenceParticle, records []schemaComponentRecord, groups []schemaModelGroupResult, version XSDVersion) error { //nolint:gocognit // Preserve lexical declaration locations while checking every pair of live group leaves.
	index := schemaGroupRecordByID(records, group.TargetID())
	if index < 0 || !groups[index].present {
		return newSchemaBridgeInvariant(group.RefLoc(), "inline extension group target is missing")
	}
	if groups[index].particle == nil {
		return nil
	}
	leaves := make([]schemaGroupLeaf, 0)
	if err := collectInlineExtensionLeaves(groups[index].particle, records, groups, Loc{}, &leaves); err != nil {
		return err
	}
	for laterIndex, later := range leaves {
		for _, first := range leaves[:laterIndex] {
			if first.name == later.name && !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
				diagnostic := newSchemaGroupElementConsistencyDiagnostic(later, first, version)
				diagnostic.related = append(diagnostic.related, group.RefLoc())
				return diagnostic
			}
			if !inlineExtensionPairMayOverlap(first, later, records) {
				continue
			}
			if inlineExtensionGroupAdjacentOptionalCompetes(groups[index].particle, first, later, records) {
				diagnostic := inlineExtensionPairAmbiguous(later, first, version)
				diagnostic.related = append(diagnostic.related, group.RefLoc())
				return diagnostic
			}
			diagnostic := inlineExtensionPairOverlapUnsupported(first, later, version)
			diagnostic.related = append(diagnostic.related, group.RefLoc())
			return diagnostic
		}
	}
	return nil
}

func inlineExtensionGroupAdjacentOptionalCompetes(particle Particle, first, later schemaGroupLeaf, records []schemaComponentRecord) bool {
	sequence, ok := particle.(SequenceParticle)
	if !ok {
		return false
	}
	parts := sequence.Particles()
	for index := 1; index < len(parts); index++ {
		firstRef, firstOK := parts[index-1].(ElementReferenceParticle)
		laterRef, laterOK := parts[index].(ElementReferenceParticle)
		if !firstOK || !laterOK || firstRef.RefLoc() != first.loc || laterRef.RefLoc() != later.loc || firstRef.Occurrences().Minimum().String() != "0" {
			continue
		}
		if inlineExtensionPairSameReferenceHasMatch(records, first, later) || inlineExtensionPairDirectSubstitutes(records, first.name, later.name) {
			return true
		}
	}
	return false
}

func inlineExtensionPairSameReferenceHasMatch(records []schemaComponentRecord, first, later schemaGroupLeaf) bool {
	if first.name != later.name || !first.reference || !later.reference {
		return false
	}
	for _, record := range records {
		if record.element == nil || record.name != first.name {
			continue
		}
		if !record.element.abstract {
			return true
		}
	}
	for _, record := range records {
		if record.element != nil && inlineExtensionPairDirectSubstitutes(records, first.name, record.name) {
			return true
		}
	}
	return false
}

func inlineExtensionPairDirectSubstitutes(records []schemaComponentRecord, head, member QName) bool {
	var headRecord, memberRecord *schemaElementInput
	for _, record := range records {
		if record.element == nil {
			continue
		}
		if record.name == head {
			headRecord = record.element
		}
		if record.name == member {
			memberRecord = record.element
		}
	}
	if headRecord == nil || memberRecord == nil || memberRecord.abstract || headRecord.block.set&schemaBlockSubstitution != 0 || headRecord.declaredType.IsZero() || headRecord.declaredType != memberRecord.declaredType {
		return false
	}
	for _, affiliation := range memberRecord.substitutionGroup {
		if affiliation.name == head {
			return true
		}
	}
	return false
}

func inlineExtensionPairLeafConflict(first, later schemaGroupLeaf, records []schemaComponentRecord, version XSDVersion) error {
	if first.name == later.name && !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
		return newSchemaGroupElementConsistencyDiagnostic(later, first, version)
	}
	if inlineExtensionPairMayOverlap(first, later, records) {
		return inlineExtensionPairOverlapUnsupported(first, later, version)
	}
	return nil
}

func inlineExtensionPairMayOverlap(first, later schemaGroupLeaf, records []schemaComponentRecord) bool {
	return first.name == later.name || schemaGroupSubstitutionMayMatch(records, first.name, later.name) || schemaGroupSubstitutionMayMatch(records, later.name, first.name) || inlineExtensionPairSharedMember(records, first, later)
}

func inlineExtensionPairOverlapUnsupported(first, later schemaGroupLeaf, version XSDVersion) Diagnostic {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(later.loc, fmt.Sprintf("inline extension sequence references %q and %q require further attribution proof", first.name, later.name), version)
	diagnostic.related = []Loc{first.loc}
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
	return diagnostic
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
		message: fmt.Sprintf("sequence particles have ambiguous matching element %q", later.name),
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
