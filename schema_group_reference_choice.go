package goxsd9

import (
	"errors"
	"fmt"
)

const diagnosticSchemaGroupReferenceChoiceUPACode = "XSD3064"

var errSchemaGroupReferenceChoiceUPA = errors.New("named model-group choice has overlapping element references")
var errSchemaGroupReferenceChoiceDerivationProof = errors.New("named model-group choice derivation-block attribution is not implemented")

// validateSchemaModelGroupReferenceChoices checks completed named-group choices
// independently of their use sites, including unused and zero-use definitions.
func validateSchemaModelGroupReferenceChoices(records []schemaComponentRecord, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion) error {
	if len(records) != len(groups) || len(records) != len(elements) {
		return newSchemaBridgeInvariant(Loc{}, "named-group attribution has incomplete component results")
	}
	for index, record := range records {
		if record.modelGroup == nil || !groups[index].present || groups[index].particle == nil {
			continue
		}
		if err := checkSchemaGroupReferenceChoices(groups[index].particle, records, elements, version); err != nil {
			return err
		}
	}
	return nil
}

func checkSchemaGroupReferenceChoices(particle Particle, records []schemaComponentRecord, elements []schemaElementTypeResult, version XSDVersion) error {
	switch term := particle.(type) {
	case ChoiceParticle:
		alternatives := term.Alternatives()
		if err := checkSchemaReferenceChoiceAlternatives(alternatives, records, elements, version); err != nil {
			return err
		}
		for _, alternative := range alternatives {
			if err := checkSchemaGroupReferenceChoices(alternative, records, elements, version); err != nil {
				return err
			}
		}
	case SequenceParticle:
		for _, child := range term.Particles() {
			if err := checkSchemaGroupReferenceChoices(child, records, elements, version); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSchemaReferenceChoiceAlternatives(alternatives []Particle, records []schemaComponentRecord, elements []schemaElementTypeResult, version XSDVersion) error { //nolint:gocognit // Ordered pairs and candidate declarations choose one reproducible witness.
	for laterIndex := 1; laterIndex < len(alternatives); laterIndex++ {
		later, ok := alternatives[laterIndex].(ElementReferenceParticle)
		if !ok {
			continue
		}
		for firstIndex := 0; firstIndex < laterIndex; firstIndex++ {
			first, ok := alternatives[firstIndex].(ElementReferenceParticle)
			if !ok {
				continue
			}
			for memberIndex, member := range records {
				if member.element == nil || !elements[memberIndex].present {
					continue
				}
				firstPath, firstMatch := schemaReferenceChoiceMatches(first.TargetID(), memberIndex, records, elements)
				if !firstMatch {
					continue
				}
				laterPath, laterMatch := schemaReferenceChoiceMatches(later.TargetID(), memberIndex, records, elements)
				if !laterMatch {
					continue
				}
				if schemaReferenceChoiceNeedsDerivationProof(memberIndex, firstPath, records, elements) || schemaReferenceChoiceNeedsDerivationProof(memberIndex, laterPath, records, elements) {
					diagnostic := newSchemaSyntaxUnsupportedForVersion(later.RefLoc(), "named model-group choice attribution through a blocked type derivation is not implemented", version)
					diagnostic.related = []Loc{first.RefLoc(), member.loc}
					diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
					diagnostic.cause = errors.Join(diagnostic.cause, errSchemaGroupReferenceChoiceDerivationProof)
					return diagnostic
				}
				return newSchemaGroupReferenceChoiceUPADiagnostic(first, later, member, firstPath, laterPath, records, elements, version)
			}
		}
	}
	return nil
}

func schemaReferenceChoiceNeedsDerivationProof(memberIndex int, path []int, records []schemaComponentRecord, elements []schemaElementTypeResult) bool {
	headIndex := path[len(path)-1]
	if memberIndex == headIndex || records[memberIndex].element.declaredType == records[headIndex].element.declaredType {
		return false
	}
	return elements[headIndex].block.set&(schemaBlockExtension|schemaBlockRestriction) != 0
}

// A substitution block belongs to the tested head. Intermediate affiliations
// remain traversable when testing a member against an ancestor head.
func schemaReferenceChoiceMatches(headID ComponentID, memberIndex int, records []schemaComponentRecord, elements []schemaElementTypeResult) ([]int, bool) {
	if records[memberIndex].id == headID {
		return []int{memberIndex}, true
	}
	headIndex := schemaGroupRecordByID(records, headID)
	if headIndex < 0 || elements[headIndex].block.set&schemaBlockSubstitution != 0 {
		return nil, false
	}
	visited := make(map[ComponentID]bool)
	return schemaReferenceChoiceAffiliationPath(headID, memberIndex, records, elements, visited)
}

func schemaReferenceChoiceAffiliationPath(headID ComponentID, index int, records []schemaComponentRecord, elements []schemaElementTypeResult, visited map[ComponentID]bool) ([]int, bool) {
	if visited[records[index].id] {
		return nil, false
	}
	visited[records[index].id] = true
	defer delete(visited, records[index].id)
	for _, affiliation := range elements[index].substitutionGroup {
		target := schemaGroupRecordByID(records, affiliation.targetID)
		if target < 0 {
			continue
		}
		if affiliation.targetID == headID {
			return []int{index, target}, true
		}
		path, ok := schemaReferenceChoiceAffiliationPath(headID, target, records, elements, visited)
		if ok {
			return append([]int{index}, path...), true
		}
	}
	return nil, false
}

func newSchemaGroupReferenceChoiceUPADiagnostic(first, later ElementReferenceParticle, member schemaComponentRecord, firstPath, laterPath []int, records []schemaComponentRecord, elements []schemaElementTypeResult, version XSDVersion) Diagnostic { //nolint:gocognit // The witness records ordered locations from both resolved affiliation paths.
	related := make([]Loc, 0, 8)
	appendRelated := func(loc Loc) {
		if loc.IsZero() || loc == later.RefLoc() {
			return
		}
		for _, previous := range related {
			if previous == loc {
				return
			}
		}
		related = append(related, loc)
	}
	appendRelated(first.RefLoc())
	appendRelated(records[firstPath[len(firstPath)-1]].loc)
	appendRelated(records[laterPath[len(laterPath)-1]].loc)
	appendRelated(member.loc)
	for _, path := range [][]int{firstPath, laterPath} {
		for offset := 0; offset+1 < len(path); offset++ {
			from, to := path[offset], path[offset+1]
			for _, affiliation := range elements[from].substitutionGroup {
				if affiliation.targetID == records[to].id {
					appendRelated(affiliation.loc)
					break
				}
			}
		}
	}
	return Diagnostic{
		class: FailureInvalid, code: diagnosticSchemaGroupReferenceChoiceUPACode,
		loc: later.RefLoc(), related: related,
		message: fmt.Sprintf("named model-group choice references %q and %q both match %q", first.Ref(), later.Ref(), member.name),
		specRef: schemaGroupNonAmbiguousSpecRef(version),
		cause:   errSchemaGroupReferenceChoiceUPA,
	}
}
