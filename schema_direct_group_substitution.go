package goxsd9

// Declarations participate in element consistency even when abstract. Matches
// contain names that can compete for an instance item at this particle.
func schemaDirectGroupAcceptedLeaves(reference ElementReferenceParticle, records []schemaComponentRecord, elements []schemaElementTypeResult, simpleTypes schemaSimpleTypeResolution, version XSDVersion, useLoc Loc) ([]schemaDirectGroupLeaf, []schemaDirectGroupLeaf, error, error) { //nolint:gocognit // Keep ordered definite and uncertain substitution evidence together.
	index := schemaGroupRecordByID(records, reference.TargetID())
	if index < 0 || records[index].element == nil || !elements[index].present {
		return nil, nil, nil, newSchemaBridgeInvariant(reference.RefLoc(), "direct group proof cannot find completed element reference target")
	}
	loc := reference.RefLoc()
	if !useLoc.IsZero() {
		loc = useLoc
	}
	head := schemaDirectGroupLeaf{name: records[index].name, declaredType: elements[index].declaredType, loc: reference.RefLoc(), useLoc: useLoc, targetID: reference.TargetID()}
	declarations := []schemaDirectGroupLeaf{head}
	matches := make([]schemaDirectGroupLeaf, 0, 1)
	concrete := false
	if !elements[index].abstract {
		matches = append(matches, head)
		concrete = true
	}
	if elements[index].abstract && version == XSDVersion11 {
		head.uncertainMatch = true
		matches = append(matches, head)
	}
	var pending error
	for candidate, record := range records {
		if candidate == index || record.element == nil || !elements[candidate].present || len(elements[candidate].substitutionGroup) == 0 {
			continue
		}
		accepted, uncertain, err := schemaDirectGroupSubstitutionMember(candidate, index, records, elements, simpleTypes)
		if err != nil {
			return nil, nil, nil, err
		}
		if uncertain {
			if pending == nil {
				pending = schemaDirectGroupUnsupported(loc, []Loc{reference.RefLoc(), record.loc}, version, "substitution applicability with a direct model-group reference is not proved")
			}
			continue
		}
		if !accepted {
			continue
		}
		leaf := schemaDirectGroupLeaf{name: record.name, declaredType: elements[candidate].declaredType, loc: reference.RefLoc(), useLoc: useLoc, declLoc: record.loc, targetID: record.id}
		declarations = append(declarations, leaf)
		if !elements[candidate].abstract {
			matches = append(matches, leaf)
			concrete = true
		}
		if elements[candidate].abstract && version == XSDVersion11 {
			leaf.uncertainMatch = true
			matches = append(matches, leaf)
		}
	}
	if !concrete && pending == nil {
		pending = schemaDirectGroupUnsupported(loc, []Loc{reference.RefLoc(), records[index].loc}, version, "abstract element reference attribution is not proved")
	}
	return declarations, matches, pending, nil
}

// Affiliation reachability and substitutability are separate constraints. The
// original referenced head's block applies to the complete member derivation.
func schemaDirectGroupSubstitutionMember(candidate, head int, records []schemaComponentRecord, elements []schemaElementTypeResult, simpleTypes schemaSimpleTypeResolution) (bool, bool, error) {
	reachable, err := schemaDirectGroupAffiliated(candidate, head, records, elements)
	if err != nil || !reachable {
		return false, false, err
	}
	block := elements[head].block.set
	if block&schemaBlockSubstitution != 0 {
		return false, false, nil
	}
	if elements[candidate].declaredType == elements[head].declaredType {
		return true, false, nil
	}
	methods, known, err := schemaDirectGroupSimpleDerivation(candidate, head, records, elements, simpleTypes)
	if err != nil {
		return false, false, err
	}
	if !known {
		return false, true, nil
	}
	return methods&block == 0, false, nil
}

func schemaDirectGroupAffiliated(candidate, head int, records []schemaComponentRecord, elements []schemaElementTypeResult) (bool, error) {
	pending := []int{candidate}
	seen := make([]bool, len(records))
	seen[candidate] = true
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for _, affiliation := range elements[current].substitutionGroup {
			next := schemaGroupRecordByID(records, affiliation.targetID)
			if next < 0 || !elements[next].present {
				return false, newSchemaBridgeInvariant(affiliation.loc, "direct group proof cannot find substitution head")
			}
			if next == head {
				return true, nil
			}
			if seen[next] {
				continue
			}
			seen[next] = true
			pending = append(pending, next)
		}
	}
	return false, nil
}

// Supported named atomic simple types expose each restriction base. Complex
// derivations and built-in-to-built-in method chains remain unproved here.
func schemaDirectGroupSimpleDerivation(candidate, head int, records []schemaComponentRecord, elements []schemaElementTypeResult, simpleTypes schemaSimpleTypeResolution) (schemaBlockSet, bool, error) { //nolint:gocognit // Follow the resolved restriction chain without cached state.
	if !elements[candidate].hasTypeID || !elements[candidate].hasTypeReference {
		return 0, false, nil
	}
	currentID := elements[candidate].typeID
	seen := make([]bool, len(records))
	for {
		index := schemaGroupRecordByID(records, currentID)
		if index < 0 || index >= len(simpleTypes.results) {
			return 0, false, newSchemaBridgeInvariant(records[candidate].loc, "direct group proof cannot find named simple type")
		}
		if seen[index] || !simpleTypes.results[index].present {
			return 0, false, nil
		}
		seen[index] = true
		result := simpleTypes.results[index]
		if !result.hasBaseReference || result.variety != SimpleTypeVarietyAtomicRestriction {
			return 0, false, nil
		}
		base := result.baseReference
		if base.name == elements[head].declaredType {
			return schemaBlockRestriction, true, nil
		}
		if base.kind != SimpleTypeReferenceNamed || !base.hasID {
			return 0, false, nil
		}
		currentID = base.id
	}
}
