package goxsd9

// These accepted leaves exist only while proving a direct owner's content model.
func schemaDirectGroupAcceptedLeaves(reference ElementReferenceParticle, records []schemaComponentRecord, elements []schemaElementTypeResult, version XSDVersion, useLoc Loc) ([]schemaDirectGroupLeaf, error) {
	index := schemaGroupRecordByID(records, reference.TargetID())
	if index < 0 || records[index].element == nil || !elements[index].present {
		return nil, newSchemaBridgeInvariant(reference.RefLoc(), "direct group proof cannot find completed element reference target")
	}
	loc := reference.RefLoc()
	if !useLoc.IsZero() {
		loc = useLoc
	}
	leaves := make([]schemaDirectGroupLeaf, 0, 1)
	if !elements[index].abstract {
		leaves = append(leaves, schemaDirectGroupLeaf{name: records[index].name, declaredType: elements[index].declaredType, loc: reference.RefLoc(), useLoc: useLoc, targetID: reference.TargetID()})
	}
	for candidate, record := range records {
		if candidate == index || record.element == nil || !elements[candidate].present || elements[candidate].abstract || len(elements[candidate].substitutionGroup) == 0 {
			continue
		}
		accepted, uncertain, err := schemaDirectGroupSubstitutionMember(candidate, index, records, elements)
		if err != nil {
			return nil, err
		}
		if uncertain {
			return nil, schemaDirectGroupUnsupported(loc, []Loc{reference.RefLoc(), record.loc}, version, "substitution applicability with a direct model-group reference is not proved")
		}
		if !accepted {
			continue
		}
		leaves = append(leaves, schemaDirectGroupLeaf{name: record.name, declaredType: elements[candidate].declaredType, loc: reference.RefLoc(), useLoc: useLoc, declLoc: record.loc, targetID: record.id})
	}
	if len(leaves) == 0 {
		return nil, schemaDirectGroupUnsupported(loc, []Loc{reference.RefLoc(), records[index].loc}, version, "abstract element reference attribution is not proved")
	}
	return leaves, nil
}

type schemaDirectGroupSubstitutionStep struct {
	index     int
	uncertain bool
}

// Resolved affiliations prove each edge. Blocks whose derivation method is not
// represented by this phase cause a located unsupported result at the use site.
func schemaDirectGroupSubstitutionMember(candidate, head int, records []schemaComponentRecord, elements []schemaElementTypeResult) (bool, bool, error) { //nolint:gocognit // Keep the ordered certain/uncertain affiliation walk together.
	pending := []schemaDirectGroupSubstitutionStep{{index: candidate}}
	seenCertain := make([]bool, len(records))
	seenUncertain := make([]bool, len(records))
	uncertainMatch := false
	for len(pending) > 0 {
		step := pending[0]
		pending = pending[1:]
		for _, affiliation := range elements[step.index].substitutionGroup {
			next, enabled, err := schemaDirectGroupNextSubstitutionStep(step, affiliation, records, elements)
			if err != nil {
				return false, false, err
			}
			if !enabled {
				continue
			}
			if next.index == head {
				if !next.uncertain {
					return true, false, nil
				}
				uncertainMatch = true
				continue
			}
			if next.uncertain {
				if seenUncertain[next.index] {
					continue
				}
				seenUncertain[next.index] = true
				pending = append(pending, next)
				continue
			}
			if seenCertain[next.index] {
				continue
			}
			seenCertain[next.index] = true
			pending = append(pending, next)
		}
	}
	return false, uncertainMatch, nil
}

func schemaDirectGroupNextSubstitutionStep(step schemaDirectGroupSubstitutionStep, affiliation schemaElementSubstitutionGroup, records []schemaComponentRecord, elements []schemaElementTypeResult) (schemaDirectGroupSubstitutionStep, bool, error) {
	next := schemaGroupRecordByID(records, affiliation.targetID)
	if next < 0 || !elements[next].present {
		return schemaDirectGroupSubstitutionStep{}, false, newSchemaBridgeInvariant(affiliation.loc, "direct group proof cannot find substitution head")
	}
	block := elements[next].block.set
	if block&schemaBlockSubstitution != 0 {
		return schemaDirectGroupSubstitutionStep{}, false, nil
	}
	uncertain := step.uncertain || block&(schemaBlockExtension|schemaBlockRestriction) != 0 && elements[step.index].declaredType != elements[next].declaredType
	return schemaDirectGroupSubstitutionStep{index: next, uncertain: uncertain}, true, nil
}
