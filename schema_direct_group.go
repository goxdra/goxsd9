package goxsd9

import "fmt"

// This proof is transient. Direct owners retain only their authored particles.
type schemaDirectGroupLeaf struct {
	name         QName
	declaredType QName
	loc          Loc
	useLoc       Loc
	declLoc      Loc
	targetID     ComponentID
}

type schemaDirectGroupProof struct {
	first      []schemaDirectGroupLeaf
	leaves     []schemaDirectGroupLeaf
	nullable   bool
	simple     bool
	variable   bool
	repeatSafe bool
}

func validateSchemaDirectGroupOwners(records []schemaComponentRecord, byName map[QName][]int, visibleSources map[SourceID][]SourceID, complexes []schemaComplexTypeResult, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion) error {
	if len(complexes) != len(records) || len(groups) != len(records) || len(elements) != len(records) {
		return newSchemaBridgeInvariant(Loc{}, "direct group proof has incomplete phase results")
	}
	for index, record := range records {
		if record.complexType == nil {
			continue
		}
		body, ok := record.complexType.body.(*schemaComplexTypeDirectBodyInput)
		if !ok || body == nil || !schemaDirectBodyContainsGroupTerm(body.particle) {
			continue
		}
		if err := schemaProveOmittedDirectGroupTerms(body.particle, record, records, byName, visibleSources, groups, elements, version); err != nil {
			return err
		}
		resolved, ok := complexes[index].body.(*schemaComplexTypeDirectBodyResult)
		if !ok || resolved == nil || resolved.particle == nil || !schemaResolvedDirectGroupTerm(resolved.particle) {
			continue
		}
		if _, err := schemaProveDirectGroupParticle(resolved.particle, records, groups, elements, version, Loc{}); err != nil {
			return err
		}
	}
	return nil
}

func schemaProveOmittedDirectGroupTerms(model schemaComplexTypeParticleInput, owner schemaComponentRecord, records []schemaComponentRecord, byName map[QName][]int, visibleSources map[SourceID][]SourceID, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion) error {
	var terms []schemaParticleTermInput
	var ownerOmitted bool
	switch particle := model.(type) {
	case *schemaChoiceParticleInput:
		terms = particle.alternatives
		ownerOmitted = !particle.occurrences.mapsToParticle()
	case *schemaSequenceParticleInput:
		terms = particle.particles
		ownerOmitted = !particle.occurrences.mapsToParticle()
	default:
		return newSchemaBridgeInvariant(owner.loc, "direct group proof has an unknown model input")
	}
	for _, term := range terms {
		group, ok := term.(*schemaModelGroupReferenceParticleInput)
		if !ok || group == nil || !ownerOmitted && group.occurrences.mapsToParticle() {
			continue
		}
		target, err := schemaModelGroupReferenceTarget(group, owner, records, byName, visibleSources, version)
		if err != nil {
			return err
		}
		if !groups[target].present {
			return newSchemaBridgeInvariant(group.reference.loc, "omitted direct group target is incomplete")
		}
		if _, err := schemaProveDirectGroupParticle(groups[target].particle, records, groups, elements, version, group.reference.loc); err != nil {
			return err
		}
	}
	return nil
}

func schemaResolvedDirectGroupTerm(particle Particle) bool {
	switch model := particle.(type) {
	case ChoiceParticle:
		for _, term := range model.Alternatives() {
			if _, ok := term.(ModelGroupReferenceParticle); ok {
				return true
			}
		}
	case SequenceParticle:
		for _, term := range model.Particles() {
			if _, ok := term.(ModelGroupReferenceParticle); ok {
				return true
			}
		}
	}
	return false
}

func schemaDirectBodyContainsGroupTerm(particle schemaComplexTypeParticleInput) bool {
	switch model := particle.(type) {
	case *schemaChoiceParticleInput:
		if model == nil {
			return false
		}
		for _, term := range model.alternatives {
			if _, ok := term.(*schemaModelGroupReferenceParticleInput); ok {
				return true
			}
		}
	case *schemaSequenceParticleInput:
		if model == nil {
			return false
		}
		for _, term := range model.particles {
			if _, ok := term.(*schemaModelGroupReferenceParticleInput); ok {
				return true
			}
		}
	}
	return false
}

// schemaProveDirectGroupParticle expands only temporary proof facts. It never
// changes the published model-group reference or stores a consumer program.
func schemaProveDirectGroupParticle(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion, useLoc Loc) (schemaDirectGroupProof, error) { //nolint:gocognit,funlen // Each particle variant has distinct attribution facts.
	if particle == nil {
		return schemaDirectGroupProof{nullable: true}, nil
	}
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty local element")
		}
		leaf := schemaDirectGroupLeaf{name: term.Name(), declaredType: term.DeclaredType(), loc: term.Loc(), useLoc: useLoc}
		return schemaApplyDirectGroupOccurrences(schemaDirectGroupProof{first: []schemaDirectGroupLeaf{leaf}, leaves: []schemaDirectGroupLeaf{leaf}, simple: true, repeatSafe: true}, term.Occurrences(), term.Loc(), useLoc, version)
	case ElementReferenceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty element reference")
		}
		leaves, err := schemaDirectGroupAcceptedLeaves(term, records, elements, version, useLoc)
		if err != nil {
			return schemaDirectGroupProof{}, err
		}
		return schemaApplyDirectGroupOccurrences(schemaDirectGroupProof{first: leaves, leaves: leaves, simple: true, repeatSafe: true}, term.Occurrences(), term.Loc(), useLoc, version)
	case ModelGroupReferenceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty model-group reference")
		}
		index := schemaGroupRecordByID(records, term.TargetID())
		if index < 0 || !groups[index].present {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(term.RefLoc(), "direct group proof cannot find completed model-group target")
		}
		if useLoc.IsZero() {
			useLoc = term.RefLoc()
		}
		proof, err := schemaProveDirectGroupParticle(groups[index].particle, records, groups, elements, version, useLoc)
		if err != nil {
			return schemaDirectGroupProof{}, err
		}
		return schemaApplyDirectGroupOccurrences(proof, term.Occurrences(), term.Loc(), useLoc, version)
	case ChoiceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty choice")
		}
		proof := schemaDirectGroupProof{repeatSafe: true}
		for _, alternative := range term.Alternatives() {
			current, err := schemaProveDirectGroupParticle(alternative, records, groups, elements, version, useLoc)
			if err != nil {
				return schemaDirectGroupProof{}, err
			}
			if err := schemaCheckDirectGroupLeaves(proof.leaves, current.leaves, version, useLoc); err != nil {
				return schemaDirectGroupProof{}, err
			}
			if err := schemaCheckDirectGroupFirst(proof.first, current.first, version, useLoc, true, false); err != nil {
				return schemaDirectGroupProof{}, err
			}
			proof.first = append(proof.first, current.first...)
			proof.leaves = append(proof.leaves, current.leaves...)
			proof.nullable = proof.nullable || current.nullable
			proof.simple = len(term.Alternatives()) == 1 && current.simple
			proof.variable = len(term.Alternatives()) == 1 && current.variable
			proof.repeatSafe = proof.repeatSafe && current.simple && current.repeatSafe
		}
		return schemaApplyDirectGroupOccurrences(proof, term.Occurrences(), term.Loc(), useLoc, version)
	case SequenceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty sequence")
		}
		proof := schemaDirectGroupProof{nullable: true, repeatSafe: len(term.Particles()) == 1}
		previous := schemaDirectGroupProof{}
		for _, child := range term.Particles() {
			current, err := schemaProveDirectGroupParticle(child, records, groups, elements, version, useLoc)
			if err != nil {
				return schemaDirectGroupProof{}, err
			}
			if err := schemaCheckDirectGroupLeaves(proof.leaves, current.leaves, version, useLoc); err != nil {
				return schemaDirectGroupProof{}, err
			}
			if previous.simple && current.simple && previous.variable {
				if err := schemaCheckDirectGroupFirst(previous.first, current.first, version, useLoc, false, true); err != nil {
					return schemaDirectGroupProof{}, err
				}
			}
			prior := proof.leaves
			if previous.simple && current.simple && !previous.variable && !previous.nullable && len(previous.leaves) == 1 && len(prior) > 0 {
				prior = prior[:len(prior)-1]
			}
			if err := schemaCheckDirectGroupFirst(prior, current.first, version, useLoc, false, false); err != nil {
				return schemaDirectGroupProof{}, err
			}
			if proof.nullable {
				proof.first = append(proof.first, current.first...)
			}
			proof.leaves = append(proof.leaves, current.leaves...)
			proof.nullable = proof.nullable && current.nullable
			proof.simple = len(term.Particles()) == 1 && current.simple
			proof.variable = len(term.Particles()) == 1 && current.variable
			proof.repeatSafe = proof.repeatSafe && current.repeatSafe
			previous = current
		}
		return schemaApplyDirectGroupOccurrences(proof, term.Occurrences(), term.Loc(), useLoc, version)
	case WildcardParticle:
		loc := useLoc
		if loc.IsZero() {
			loc = term.Loc()
		}
		return schemaDirectGroupProof{}, schemaDirectGroupUnsupported(loc, []Loc{term.Loc()}, version, "wildcard attribution with a direct model-group reference is not proved")
	default:
		return schemaDirectGroupProof{}, newSchemaBridgeInvariant(particle.Loc(), "direct group proof has an unknown particle")
	}
}

func schemaApplyDirectGroupOccurrences(proof schemaDirectGroupProof, occurrences ParticleOccurrenceRange, particleLoc, useLoc Loc, version XSDVersion) (schemaDirectGroupProof, error) {
	maximum := occurrences.Maximum().String()
	if maximum != "0" && maximum != "1" && !proof.repeatSafe {
		loc := particleLoc
		if !useLoc.IsZero() {
			loc = useLoc
		}
		related := []Loc{particleLoc}
		if len(proof.first) > 0 {
			related = append(related, proof.first[0].loc)
		}
		return schemaDirectGroupProof{}, schemaDirectGroupUnsupported(loc, related, version, "attribution across a repeated model-group boundary is not proved")
	}
	if occurrences.Minimum().String() == "0" {
		proof.nullable = true
	}
	proof.variable = proof.variable || occurrences.Minimum().String() != maximum
	return proof, nil
}

func schemaCheckDirectGroupLeaves(first, later []schemaDirectGroupLeaf, version XSDVersion, useLoc Loc) error {
	for _, current := range later {
		for _, prior := range first {
			if err := schemaDirectGroupTypeConflict(prior, current, version, useLoc); err != nil {
				return err
			}
		}
	}
	return nil
}

func schemaDirectGroupTypeConflict(prior, current schemaDirectGroupLeaf, version XSDVersion, useLoc Loc) error {
	if prior.name != current.name || !prior.targetID.IsZero() && prior.targetID == current.targetID {
		return nil
	}
	loc := schemaDirectGroupPrimary(current, useLoc)
	related := schemaDirectGroupConflictRelated(prior, current)
	if prior.declaredType.IsZero() || current.declaredType.IsZero() {
		return schemaDirectGroupUnsupported(loc, related, version, "element type consistency cannot be proved for a direct model-group reference")
	}
	if prior.declaredType == current.declaredType {
		return nil
	}
	return Diagnostic{class: FailureInvalid, code: diagnosticSchemaGroupElementConsistencyCode, loc: loc, related: related, message: fmt.Sprintf("elements %q have inconsistent declared types", current.name), specRef: schemaGroupElementConsistencySpecRef(version), cause: errSchemaGroupElementConsistency}
}

func schemaCheckDirectGroupFirst(first, later []schemaDirectGroupLeaf, version XSDVersion, useLoc Loc, choice, definiteSequence bool) error {
	for _, current := range later {
		for _, prior := range first {
			if prior.name != current.name {
				continue
			}
			loc := schemaDirectGroupPrimary(current, useLoc)
			related := schemaDirectGroupConflictRelated(prior, current)
			if choice || definiteSequence {
				return Diagnostic{class: FailureInvalid, code: diagnosticSchemaDirectGroupUPACode, loc: loc, related: related, message: fmt.Sprintf("elements %q have ambiguous attribution", current.name), specRef: schemaGroupNonAmbiguousSpecRef(version), cause: errSchemaDirectGroupUPA}
			}
			return schemaDirectGroupUnsupported(loc, related, version, "direct model-group attribution cannot be proved")
		}
	}
	return nil
}

func schemaDirectGroupConflictRelated(prior, current schemaDirectGroupLeaf) []Loc {
	related := []Loc{prior.loc, current.loc}
	if !prior.declLoc.IsZero() {
		related = appendSchemaRelatedLocation(related, prior.declLoc, Loc{})
	}
	if !current.declLoc.IsZero() {
		related = appendSchemaRelatedLocation(related, current.declLoc, Loc{})
	}
	return related
}

func schemaDirectGroupPrimary(leaf schemaDirectGroupLeaf, useLoc Loc) Loc {
	if !useLoc.IsZero() {
		return useLoc
	}
	if !leaf.useLoc.IsZero() {
		return leaf.useLoc
	}
	return leaf.loc
}

func schemaDirectGroupUnsupported(loc Loc, related []Loc, version XSDVersion, message string) error {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(loc, message, version)
	diagnostic.related = append([]Loc(nil), related...)
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
	diagnostic.cause = errSchemaDirectGroupProofUnsupported
	return diagnostic
}
