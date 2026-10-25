package goxsd9

import "fmt"

// This proof is transient. Direct owners retain only their authored particles.
type schemaDirectGroupLeaf struct {
	name         QName
	declaredType QName
	loc          Loc
	useLoc       Loc
	targetID     ComponentID
}

type schemaDirectGroupProof struct {
	first    []schemaDirectGroupLeaf
	leaves   []schemaDirectGroupLeaf
	nullable bool
	simple   bool
	variable bool
}

func validateSchemaDirectGroupOwners(records []schemaComponentRecord, byName map[QName][]int, visibleSources map[SourceID][]SourceID, complexes []schemaComplexTypeResult, groups []schemaModelGroupResult, version XSDVersion) error {
	if len(complexes) != len(records) || len(groups) != len(records) {
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
		if err := schemaProveOmittedDirectGroupTerms(body.particle, record, records, byName, visibleSources, groups, version); err != nil {
			return err
		}
		resolved, ok := complexes[index].body.(*schemaComplexTypeDirectBodyResult)
		if !ok || resolved == nil || resolved.particle == nil || !schemaResolvedDirectGroupTerm(resolved.particle) {
			continue
		}
		if _, err := schemaProveDirectGroupParticle(resolved.particle, records, groups, version, Loc{}); err != nil {
			return err
		}
	}
	return nil
}

func schemaProveOmittedDirectGroupTerms(model schemaComplexTypeParticleInput, owner schemaComponentRecord, records []schemaComponentRecord, byName map[QName][]int, visibleSources map[SourceID][]SourceID, groups []schemaModelGroupResult, version XSDVersion) error {
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
		if _, err := schemaProveDirectGroupParticle(groups[target].particle, records, groups, version, group.reference.loc); err != nil {
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
func schemaProveDirectGroupParticle(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, version XSDVersion, useLoc Loc) (schemaDirectGroupProof, error) { //nolint:gocognit,funlen // Each particle variant has distinct attribution facts.
	if particle == nil {
		return schemaDirectGroupProof{nullable: true}, nil
	}
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty local element")
		}
		leaf := schemaDirectGroupLeaf{name: term.Name(), declaredType: term.DeclaredType(), loc: term.Loc(), useLoc: useLoc}
		return schemaApplyDirectGroupOccurrences(schemaDirectGroupProof{first: []schemaDirectGroupLeaf{leaf}, leaves: []schemaDirectGroupLeaf{leaf}, simple: true}, term.Occurrences()), nil
	case ElementReferenceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty element reference")
		}
		index := schemaGroupRecordByID(records, term.TargetID())
		if index < 0 || records[index].element == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(term.RefLoc(), "direct group proof cannot find element reference target")
		}
		leaf := schemaDirectGroupLeaf{name: term.Ref(), declaredType: records[index].element.declaredType, loc: term.RefLoc(), useLoc: useLoc, targetID: term.TargetID()}
		return schemaApplyDirectGroupOccurrences(schemaDirectGroupProof{first: []schemaDirectGroupLeaf{leaf}, leaves: []schemaDirectGroupLeaf{leaf}, simple: true}, term.Occurrences()), nil
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
		proof, err := schemaProveDirectGroupParticle(groups[index].particle, records, groups, version, useLoc)
		if err != nil {
			return schemaDirectGroupProof{}, err
		}
		return schemaApplyDirectGroupOccurrences(proof, term.Occurrences()), nil
	case ChoiceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty choice")
		}
		proof := schemaDirectGroupProof{}
		for _, alternative := range term.Alternatives() {
			current, err := schemaProveDirectGroupParticle(alternative, records, groups, version, useLoc)
			if err != nil {
				return schemaDirectGroupProof{}, err
			}
			if err := schemaCheckDirectGroupLeaves(proof.leaves, current.leaves, version, useLoc); err != nil {
				return schemaDirectGroupProof{}, err
			}
			if err := schemaCheckDirectGroupFirst(proof.first, current.first, records, version, useLoc, true, false); err != nil {
				return schemaDirectGroupProof{}, err
			}
			proof.first = append(proof.first, current.first...)
			proof.leaves = append(proof.leaves, current.leaves...)
			proof.nullable = proof.nullable || current.nullable
			proof.simple = len(term.Alternatives()) == 1 && current.simple
			proof.variable = len(term.Alternatives()) == 1 && current.variable
		}
		return schemaApplyDirectGroupOccurrences(proof, term.Occurrences()), nil
	case SequenceParticle:
		if term.facts == nil {
			return schemaDirectGroupProof{}, newSchemaBridgeInvariant(Loc{}, "direct group proof has an empty sequence")
		}
		proof := schemaDirectGroupProof{nullable: true}
		previous := schemaDirectGroupProof{}
		for _, child := range term.Particles() {
			current, err := schemaProveDirectGroupParticle(child, records, groups, version, useLoc)
			if err != nil {
				return schemaDirectGroupProof{}, err
			}
			if err := schemaCheckDirectGroupLeaves(proof.leaves, current.leaves, version, useLoc); err != nil {
				return schemaDirectGroupProof{}, err
			}
			if previous.simple && current.simple && previous.variable {
				if err := schemaCheckDirectGroupFirst(previous.first, current.first, records, version, useLoc, false, true); err != nil {
					return schemaDirectGroupProof{}, err
				}
			}
			prior := proof.leaves
			if previous.simple && current.simple && !previous.variable && !previous.nullable && len(previous.leaves) == 1 && len(prior) > 0 {
				prior = prior[:len(prior)-1]
			}
			if err := schemaCheckDirectGroupFirst(prior, current.first, records, version, useLoc, false, false); err != nil {
				return schemaDirectGroupProof{}, err
			}
			if proof.nullable {
				proof.first = append(proof.first, current.first...)
			}
			proof.leaves = append(proof.leaves, current.leaves...)
			proof.nullable = proof.nullable && current.nullable
			proof.simple = len(term.Particles()) == 1 && current.simple
			proof.variable = len(term.Particles()) == 1 && current.variable
			previous = current
		}
		return schemaApplyDirectGroupOccurrences(proof, term.Occurrences()), nil
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

func schemaApplyDirectGroupOccurrences(proof schemaDirectGroupProof, occurrences ParticleOccurrenceRange) schemaDirectGroupProof {
	if occurrences.Minimum().String() == "0" {
		proof.nullable = true
	}
	maximum, finite := occurrences.Maximum().Finite()
	proof.variable = proof.variable || !finite || occurrences.Minimum().String() != maximum.String()
	return proof
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
	related := []Loc{prior.loc, current.loc}
	if prior.declaredType.IsZero() || current.declaredType.IsZero() {
		return schemaDirectGroupUnsupported(loc, related, version, "element type consistency cannot be proved for a direct model-group reference")
	}
	if prior.declaredType == current.declaredType {
		return nil
	}
	return Diagnostic{class: FailureInvalid, code: diagnosticSchemaGroupElementConsistencyCode, loc: loc, related: related, message: fmt.Sprintf("elements %q have inconsistent declared types", current.name), specRef: schemaGroupElementConsistencySpecRef(version), cause: errSchemaGroupElementConsistency}
}

func schemaCheckDirectGroupFirst(first, later []schemaDirectGroupLeaf, records []schemaComponentRecord, version XSDVersion, useLoc Loc, choice, definiteSequence bool) error {
	for _, current := range later {
		for _, prior := range first {
			same := prior.name == current.name
			substitution := schemaGroupSubstitutionMayMatch(records, prior.name, current.name) || schemaGroupSubstitutionMayMatch(records, current.name, prior.name)
			if !same && !substitution {
				continue
			}
			loc := schemaDirectGroupPrimary(current, useLoc)
			related := []Loc{prior.loc, current.loc}
			if same && (choice || definiteSequence) {
				return Diagnostic{class: FailureInvalid, code: diagnosticSchemaDirectGroupUPACode, loc: loc, related: related, message: fmt.Sprintf("elements %q have ambiguous attribution", current.name), specRef: schemaGroupNonAmbiguousSpecRef(version), cause: errSchemaDirectGroupUPA}
			}
			return schemaDirectGroupUnsupported(loc, related, version, "direct model-group attribution cannot be proved")
		}
	}
	return nil
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
