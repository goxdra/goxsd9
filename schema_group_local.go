package goxsd9

import "fmt"

type schemaGroupLeaf struct {
	name         QName
	declaredType QName
	loc          Loc
	local        bool
	reference    bool
}

// validateSchemaModelGroupLocalOverlaps keeps newly admitted local declarations
// out of content models whose declaration consistency or attribution cannot be
// proved by the bounded group representation.
func validateSchemaModelGroupLocalOverlaps(records []schemaComponentRecord, groups []schemaModelGroupResult, version XSDVersion) error { //nolint:gocognit // Compare leaves in lexical group order, including referenced groups.
	hasLocal := make(map[ComponentID]bool)
	for index, record := range records {
		if record.modelGroup == nil || !groups[index].present || groups[index].particle == nil {
			continue
		}
		if !schemaGroupParticleHasLocal(groups[index].particle, records, groups, hasLocal) {
			continue
		}
		seen := make([]schemaGroupLeaf, 0)
		seenReferences := make(map[QName]bool)
		visit := func(leaf schemaGroupLeaf) error {
			if leaf.reference && seenReferences[leaf.name] {
				return nil
			}
			for _, first := range seen {
				if err := schemaGroupLeafConflict(first, leaf, records, version); err != nil {
					return err
				}
			}
			seen = append(seen, leaf)
			if leaf.reference {
				seenReferences[leaf.name] = true
			}
			return nil
		}
		if err := walkSchemaGroupLeaves(groups[index].particle, records, groups, Loc{}, hasLocal, make(map[ComponentID]bool), visit); err != nil {
			return err
		}
	}
	return nil
}

func schemaGroupLeafConflict(first, later schemaGroupLeaf, records []schemaComponentRecord, version XSDVersion) error {
	if !first.local && !later.local {
		return nil
	}
	if first.name == later.name {
		if !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
			return newSchemaGroupElementConsistencyDiagnostic(later, first, version)
		}
		return newSchemaGroupOverlapDiagnostic(later, first, version)
	}
	if first.local && later.reference && schemaGroupSubstitutionMayMatch(records, later.name, first.name) ||
		later.local && first.reference && schemaGroupSubstitutionMayMatch(records, first.name, later.name) {
		return newSchemaGroupOverlapDiagnostic(later, first, version)
	}
	return nil
}

func schemaGroupParticleHasLocal(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, cache map[ComponentID]bool) bool {
	switch term := particle.(type) {
	case ElementParticle:
		return true
	case ModelGroupReferenceParticle:
		id := term.TargetID()
		if present, checked := cache[id]; checked {
			return present
		}
		index := schemaGroupRecordByID(records, id)
		if index < 0 || !groups[index].present || groups[index].particle == nil {
			return false
		}
		cache[id] = false
		present := schemaGroupParticleHasLocal(groups[index].particle, records, groups, cache)
		cache[id] = present
		return present
	case ChoiceParticle:
		return schemaGroupTermsHaveLocal(term.Alternatives(), records, groups, cache)
	case SequenceParticle:
		return schemaGroupTermsHaveLocal(term.Particles(), records, groups, cache)
	}
	return false
}

func schemaGroupTermsHaveLocal(terms []Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, cache map[ComponentID]bool) bool {
	present := false
	for _, term := range terms {
		if schemaGroupParticleHasLocal(term, records, groups, cache) {
			present = true
		}
	}
	return present
}

func walkSchemaGroupLeaves(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, via Loc, hasLocal, visitedRefOnly map[ComponentID]bool, visit func(schemaGroupLeaf) error) error { //nolint:gocognit // Each resolved particle variant contributes its ordered leaf projection.
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group local element has no facts")
		}
		return visit(schemaGroupLeaf{name: term.Name(), declaredType: term.DeclaredType(), loc: schemaGroupLeafLoc(term.Loc(), via), local: true})
	case ElementReferenceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group element reference has no facts")
		}
		index := schemaGroupRecordByID(records, term.TargetID())
		if index < 0 || records[index].element == nil {
			return newSchemaBridgeInvariant(term.RefLoc(), "model-group element reference target has no element input")
		}
		return visit(schemaGroupLeaf{name: term.Ref(), declaredType: records[index].element.declaredType, loc: schemaGroupLeafLoc(term.RefLoc(), via), reference: true})
	case ModelGroupReferenceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group reference has no facts")
		}
		id := term.TargetID()
		index := schemaGroupRecordByID(records, id)
		if index < 0 || !groups[index].present || groups[index].particle == nil {
			return newSchemaBridgeInvariant(term.RefLoc(), "model-group reference target has no particle")
		}
		if !hasLocal[id] && visitedRefOnly[id] {
			return nil
		}
		if !hasLocal[id] {
			visitedRefOnly[id] = true
		}
		return walkSchemaGroupLeaves(groups[index].particle, records, groups, term.RefLoc(), hasLocal, visitedRefOnly, visit)
	case ChoiceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group choice has no facts")
		}
		return walkSchemaGroupTerms(term.facts.alternatives, records, groups, via, hasLocal, visitedRefOnly, visit)
	case SequenceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group sequence has no facts")
		}
		return walkSchemaGroupTerms(term.facts.particles, records, groups, via, hasLocal, visitedRefOnly, visit)
	}
	return newSchemaBridgeInvariant(particle.Loc(), "model-group has an unknown resolved particle")
}

func walkSchemaGroupTerms(terms []Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, via Loc, hasLocal, visitedRefOnly map[ComponentID]bool, visit func(schemaGroupLeaf) error) error {
	for _, term := range terms {
		if err := walkSchemaGroupLeaves(term, records, groups, via, hasLocal, visitedRefOnly, visit); err != nil {
			return err
		}
	}
	return nil
}

func schemaGroupLeafLoc(declaration, via Loc) Loc {
	if !via.IsZero() {
		return via
	}
	return declaration
}

func schemaGroupRecordByID(records []schemaComponentRecord, id ComponentID) int {
	for index, record := range records {
		if record.id == id {
			return index
		}
	}
	return -1
}

func schemaGroupSubstitutionMayMatch(records []schemaComponentRecord, head, member QName) bool {
	pending := []QName{member}
	seen := make(map[QName]bool)
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if current == head {
			return true
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		for _, record := range records {
			if record.element == nil || record.name != current {
				continue
			}
			for _, affiliation := range record.element.substitutionGroup {
				pending = append(pending, affiliation.name)
			}
		}
	}
	return false
}

func newSchemaGroupElementConsistencyDiagnostic(later, first schemaGroupLeaf, version XSDVersion) Diagnostic {
	return Diagnostic{
		class: FailureInvalid, code: diagnosticSchemaGroupElementConsistencyCode,
		loc: later.loc, related: []Loc{first.loc},
		message: fmt.Sprintf("named model-group element %q has inconsistent declared types", later.name),
		specRef: schemaGroupElementConsistencySpecRef(version),
		cause:   errSchemaGroupElementConsistency,
	}
}

func newSchemaGroupOverlapDiagnostic(later, first schemaGroupLeaf, version XSDVersion) Diagnostic {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(later.loc, fmt.Sprintf("named model-group elements %q and %q require unimplemented attribution checks", first.name, later.name), version)
	diagnostic.related = []Loc{first.loc}
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
	return diagnostic
}

func schemaGroupElementConsistencySpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#cos-element-consistent"
	}
	return "xsd11-structures#cos-element-consistent"
}

func schemaGroupNonAmbiguousSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#cos-nonambig"
	}
	return "xsd11-structures#cos-nonambig"
}
