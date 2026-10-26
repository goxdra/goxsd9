package goxsd9

import "fmt"

// schemaBoundedRestrictionSources retains the two authored parts of an
// extension's effective content without inventing a source location for it.
type schemaBoundedRestrictionSources struct {
	annotation ElementReferenceParticle
	group      ModelGroupReferenceParticle
}

// This transient witness proves that the completed base group cannot match empty.
type schemaBoundedRestrictionChoice struct {
	baseGroupLoc Loc
	alternatives [3]ElementReferenceParticle
}

//nolint:gocognit // Inspect both completed extension representations without flattening provenance.
func (resolver *schemaComplexTypeResolver) boundedRestrictionSources(body schemaComplexTypeBodyResult, baseLoc Loc) (schemaBoundedRestrictionSources, bool, error) {
	var groupParticle Particle
	var baseReference schemaComplexTypeReferenceComponent
	switch completed := body.(type) {
	case *schemaComplexTypeExtensionBodyResult:
		if completed == nil || len(completed.attributeUses) != 0 || completed.anyAttribute.present {
			return schemaBoundedRestrictionSources{}, false, nil
		}
		groupParticle, baseReference = completed.particle, completed.base
	case *schemaComplexTypeGroupedExtensionBodyResult:
		if completed == nil || completed.anyAttribute.present {
			return schemaBoundedRestrictionSources{}, false, nil
		}
		content, ok := completed.content.(*schemaComplexTypeDirectBodyResult)
		if !ok || content == nil || len(content.attributeUses) != 0 || content.anyAttribute.present {
			return schemaBoundedRestrictionSources{}, false, nil
		}
		groupParticle, baseReference = content.particle, completed.base
	default:
		return schemaBoundedRestrictionSources{}, false, nil
	}
	group, ok := groupParticle.(ModelGroupReferenceParticle)
	if !ok || !group.Occurrences().IsDefault() {
		return schemaBoundedRestrictionSources{}, false, nil
	}
	sequence, found, err := resolver.completedOptionalReferenceSequenceBase(baseReference)
	if err != nil {
		return schemaBoundedRestrictionSources{}, false, err
	}
	if !found {
		return schemaBoundedRestrictionSources{}, false, nil
	}
	members := sequence.Particles()
	if len(members) != 1 {
		return schemaBoundedRestrictionSources{}, false, newSchemaBridgeInvariant(baseLoc, "completed optional-reference sequence changed shape")
	}
	annotation, ok := members[0].(ElementReferenceParticle)
	if !ok || annotation.Occurrences().String() != "0/1" {
		return schemaBoundedRestrictionSources{}, false, newSchemaBridgeInvariant(baseLoc, "completed optional-reference sequence changed member")
	}
	return schemaBoundedRestrictionSources{annotation: annotation, group: group}, true, nil
}

//nolint:gocognit // Check each exact bounded obligation before publishing the restriction.
func (resolver *schemaComplexTypeResolver) proveBoundedParticleRestriction(owner schemaComponentRecord, input *schemaComplexTypeRestrictionBodyInput, baseBody schemaComplexTypeBodyResult, particle Particle, baseIndex int) error {
	baseLoc := resolver.records[baseIndex].loc
	if owner.complexType == nil {
		return resolver.unsupportedBoundedRestriction(input.restrictionLoc, "inline particle restriction is outside the copied named-type proof", baseLoc)
	}
	base, supported, err := resolver.boundedRestrictionSources(baseBody, baseLoc)
	if err != nil {
		return err
	}
	if !supported {
		return resolver.unsupportedBoundedRestriction(input.base.loc, "restriction base has content or uses outside the bounded particle proof", baseLoc)
	}
	if err := resolver.prepareBoundedRestrictionChoice(input, base, particle, owner); err != nil {
		return err
	}
	sequence, ok := particle.(SequenceParticle)
	if !ok {
		return resolver.unsupportedBoundedRestriction(input.particleLoc(), "restriction has no effective sequence particle", baseLoc)
	}
	if !sequence.Occurrences().IsDefault() {
		return resolver.unsupportedBoundedRestriction(sequence.Loc(), "restriction sequence bounds are outside the copied shape", baseLoc)
	}
	members := sequence.Particles()
	if len(members) != 2 {
		return resolver.unsupportedBoundedRestriction(sequence.Loc(), "restriction sequence requires the copied element and group references", baseLoc)
	}
	annotation, elementOK := members[0].(ElementReferenceParticle)
	group, groupOK := members[1].(ModelGroupReferenceParticle)
	if !elementOK || !groupOK {
		return resolver.unsupportedBoundedRestriction(sequence.Loc(), "restriction sequence requires ordered global element and group references", baseLoc)
	}
	if annotation.TargetID() != base.annotation.TargetID() || annotation.Ref() != base.annotation.Ref() {
		return resolver.unsupportedBoundedRestriction(annotation.RefLoc(), "restriction element reference differs from the completed base", base.annotation.RefLoc())
	}
	if group.TargetID() != base.group.TargetID() || group.Ref() != base.group.Ref() {
		return resolver.unsupportedBoundedRestriction(group.RefLoc(), "restriction group reference differs from the completed base", base.group.RefLoc())
	}
	if annotation.Occurrences().String() != "0/1" {
		if maximum := annotation.Occurrences().Maximum().String(); maximum != "0" && maximum != "1" {
			return resolver.invalidBoundedRestriction(annotation.Loc(), "restriction element reference admits more occurrences than its base", base.annotation.Loc())
		}
		return resolver.unsupportedBoundedRestriction(annotation.Loc(), "restriction element reference uses valid but unproved bounds", base.annotation.Loc())
	}
	if !group.Occurrences().IsDefault() {
		if maximum := group.Occurrences().Maximum().String(); maximum != "0" && maximum != "1" {
			return resolver.invalidBoundedRestriction(group.Loc(), "restriction group reference admits more occurrences than its base", base.group.Loc())
		}
		return resolver.unsupportedBoundedRestriction(group.Loc(), "restriction group reference uses valid but unproved bounds", base.group.Loc())
	}
	if resolver.version == XSDVersion10 {
		return resolver.proveXSD10BoundedRestriction(base, annotation, group)
	}
	return resolver.proveXSD11BoundedRestriction(base, annotation, group)
}

func (resolver *schemaComplexTypeResolver) prepareBoundedRestrictionChoice(input *schemaComplexTypeRestrictionBodyInput, base schemaBoundedRestrictionSources, particle Particle, owner schemaComponentRecord) error {
	sequence, ok := input.particle.(*schemaSequenceParticleInput)
	if !ok || sequence == nil {
		return newSchemaBridgeInvariant(input.restrictionLoc, "bounded restriction lost its sequence input")
	}
	if err := resolver.rejectXSD10MappedOptionalRestriction(base, particle); err != nil {
		return err
	}
	derivedGroupLoc := sequence.loc
	if len(sequence.particles) == 2 {
		group, groupOK := sequence.particles[1].(*schemaModelGroupReferenceParticleInput)
		if groupOK && group != nil && group.reference != nil {
			derivedGroupLoc = group.reference.loc
		}
	}
	choice, err := resolver.boundedRestrictionChoice(base, derivedGroupLoc)
	if err != nil {
		return err
	}
	if err := resolver.proveBoundedRestrictionAttribution(base.annotation, choice); err != nil {
		return err
	}
	if rangeErr := resolver.rejectBoundedRestrictionRanges(sequence, choice, base, particle, owner); rangeErr != nil {
		return rangeErr
	}
	return nil
}

// A mapped 0/1 particle fails XSD 1.0's structural range rule even when its
// group's language is emptiable. A 0/0 source has no mapped particle.
func (resolver *schemaComplexTypeResolver) rejectXSD10MappedOptionalRestriction(base schemaBoundedRestrictionSources, particle Particle) error {
	if resolver.version != XSDVersion10 {
		return nil
	}
	sequence, ok := particle.(SequenceParticle)
	if !ok {
		return nil
	}
	members := sequence.Particles()
	if len(members) != 2 {
		return nil
	}
	annotation, elementOK := members[0].(ElementReferenceParticle)
	group, groupOK := members[1].(ModelGroupReferenceParticle)
	if !elementOK || !groupOK || annotation.Occurrences().String() != "0/1" {
		return nil
	}
	if annotation.TargetID() != base.annotation.TargetID() || annotation.Ref() != base.annotation.Ref() || group.TargetID() != base.group.TargetID() || group.Ref() != base.group.Ref() {
		return nil
	}
	if sequence.Occurrences().String() == "0/1" && group.Occurrences().IsDefault() {
		return resolver.invalidBoundedRestriction(sequence.Loc(), "restriction sequence minimum is below its mapped base minimum", base.group.Loc())
	}
	if sequence.Occurrences().IsDefault() && group.Occurrences().String() == "0/1" {
		return resolver.invalidBoundedRestriction(group.Loc(), "restriction group minimum is below its mapped base minimum", base.group.Loc())
	}
	return nil
}

// The choice witness is required before these language-dependent conclusions.
func (resolver *schemaComplexTypeResolver) rejectBoundedRestrictionRanges(sequence *schemaSequenceParticleInput, choice schemaBoundedRestrictionChoice, base schemaBoundedRestrictionSources, particle Particle, owner schemaComponentRecord) error {
	if sequence.occurrences.minimumOccurrence().String() == "0" {
		return resolver.invalidBoundedRestriction(sequence.loc, "restriction sequence admits empty content excluded by its base", choice.baseGroupLoc)
	}
	corresponds, err := resolver.boundedRestrictionReferencesCorrespond(sequence, base, particle, owner)
	if err != nil {
		return err
	}
	if maximum := sequence.occurrences.maximumOccurrence().String(); maximum != "0" && maximum != "1" {
		if len(sequence.particles) == 2 {
			group, groupOK := sequence.particles[1].(*schemaModelGroupReferenceParticleInput)
			if corresponds && groupOK && group != nil && group.occurrences.isDefault() {
				return resolver.invalidBoundedRestriction(sequence.loc, "restriction sequence admits more occurrences than its base", choice.baseGroupLoc)
			}
		}
		return resolver.unsupportedBoundedRestriction(sequence.loc, "restriction sequence widening has no bounded occurrence witness", choice.baseGroupLoc)
	}
	if len(sequence.particles) != 2 {
		return nil
	}
	group, ok := sequence.particles[1].(*schemaModelGroupReferenceParticleInput)
	if ok && group != nil && group.occurrences.minimumOccurrence().String() == "0" {
		if corresponds {
			return resolver.invalidBoundedRestriction(group.loc, "restriction group reference permits omission of its required base group", choice.baseGroupLoc)
		}
		return resolver.unsupportedBoundedRestriction(group.loc, "restriction group omission lacks copied reference correspondence", choice.baseGroupLoc)
	}
	return nil
}

// The prerequisite resolver has checked even 0/0 refs; look up only the
// omitted group target so a later range conclusion uses resolved identity.
func (resolver *schemaComplexTypeResolver) boundedRestrictionReferencesCorrespond(sequence *schemaSequenceParticleInput, base schemaBoundedRestrictionSources, particle Particle, owner schemaComponentRecord) (bool, error) {
	if len(sequence.particles) != 2 {
		return false, nil
	}
	groupInput, ok := sequence.particles[1].(*schemaModelGroupReferenceParticleInput)
	if !ok || groupInput == nil || groupInput.reference == nil {
		return false, nil
	}
	effective, ok := particle.(SequenceParticle)
	if !ok {
		return false, nil
	}
	members := effective.Particles()
	if len(members) == 0 || len(members) > 2 {
		return false, nil
	}
	annotation, ok := members[0].(ElementReferenceParticle)
	if !ok || annotation.Occurrences().String() != "0/1" || annotation.TargetID() != base.annotation.TargetID() || annotation.Ref() != base.annotation.Ref() {
		return false, nil
	}
	if len(members) == 2 {
		group, ok := members[1].(ModelGroupReferenceParticle)
		return ok && group.TargetID() == base.group.TargetID() && group.Ref() == base.group.Ref(), nil
	}
	if groupInput.occurrences.mapsToParticle() {
		return false, nil
	}
	index, err := schemaModelGroupReferenceTarget(groupInput, owner, resolver.records, resolver.byName, resolver.visibleSources, resolver.version)
	if err != nil {
		return false, err
	}
	return resolver.records[index].id == base.group.TargetID() && groupInput.reference.name == base.group.Ref(), nil
}

func (input *schemaComplexTypeRestrictionBodyInput) particleLoc() Loc {
	if input == nil || input.particle == nil {
		return Loc{}
	}
	if sequence, ok := input.particle.(*schemaSequenceParticleInput); ok && sequence != nil {
		return sequence.loc
	}
	return input.restrictionLoc
}

func (resolver *schemaComplexTypeResolver) proveXSD10BoundedRestriction(base schemaBoundedRestrictionSources, annotation ElementReferenceParticle, group ModelGroupReferenceParticle) error {
	// The base's 1/1 singleton sequence is pointless. Sequence:sequence Recurse
	// matches ordered refs; Elt:Elt NameAndTypeOK and choice RecurseLax hold for
	// the same resolved global targets and exact bounds.
	if base.annotation.TargetID() != annotation.TargetID() || base.group.TargetID() != group.TargetID() {
		return newSchemaBridgeInvariant(annotation.RefLoc(), "XSD 1.0 restriction proof lost resolved identity")
	}
	return nil
}

func (resolver *schemaComplexTypeResolver) proveXSD11BoundedRestriction(base schemaBoundedRestrictionSources, annotation ElementReferenceParticle, group ModelGroupReferenceParticle) error {
	// Identical ordered refs and bounds accept identical sequences. Each ref
	// selects the same global declaration, so its default binding is identical.
	if base.annotation.TargetID() != annotation.TargetID() || base.group.TargetID() != group.TargetID() {
		return newSchemaBridgeInvariant(annotation.RefLoc(), "XSD 1.1 content inclusion proof lost resolved identity")
	}
	return nil
}

func (resolver *schemaComplexTypeResolver) boundedRestrictionChoice(base schemaBoundedRestrictionSources, derivedGroupLoc Loc) (schemaBoundedRestrictionChoice, error) {
	index := schemaGroupRecordByID(resolver.records, base.group.TargetID())
	if index < 0 || !resolver.modelGroups[index].present {
		return schemaBoundedRestrictionChoice{}, newSchemaBridgeInvariant(base.group.RefLoc(), "restriction group target has no completed model")
	}
	choice, ok := resolver.modelGroups[index].particle.(ChoiceParticle)
	if !ok || !choice.Occurrences().IsDefault() {
		return schemaBoundedRestrictionChoice{}, resolver.unsupportedBoundedRestriction(derivedGroupLoc, "restriction group target is outside the copied three-reference choice", resolver.records[index].loc)
	}
	alternatives := choice.Alternatives()
	if len(alternatives) != 3 {
		return schemaBoundedRestrictionChoice{}, resolver.unsupportedBoundedRestriction(derivedGroupLoc, "restriction group target is outside the copied three-reference choice", resolver.records[index].loc)
	}
	witness := schemaBoundedRestrictionChoice{baseGroupLoc: base.group.Loc()}
	for alternativeIndex, alternative := range alternatives {
		ref, ok := alternative.(ElementReferenceParticle)
		if !ok || !ref.Occurrences().IsDefault() {
			return schemaBoundedRestrictionChoice{}, resolver.unsupportedBoundedRestriction(derivedGroupLoc, "restriction group target has an unproved alternative", alternative.Loc())
		}
		witness.alternatives[alternativeIndex] = ref
	}
	return witness, nil
}

func (resolver *schemaComplexTypeResolver) proveBoundedRestrictionAttribution(annotation ElementReferenceParticle, choice schemaBoundedRestrictionChoice) error {
	refs := []ElementReferenceParticle{annotation, choice.alternatives[0], choice.alternatives[1], choice.alternatives[2]}
	for later := 1; later < len(refs); later++ {
		for first := 0; first < later; first++ {
			if !resolver.referencesMayOverlap(refs[first], refs[later]) {
				continue
			}
			return Diagnostic{class: FailureInvalid, code: invalidSchemaCompositionCode,
				loc: refs[later].RefLoc(), related: []Loc{refs[first].RefLoc()},
				message: fmt.Sprintf("restriction references %q and %q overlap", refs[first].Ref(), refs[later].Ref()),
				specRef: schemaGroupNonAmbiguousSpecRef(resolver.version), cause: errSchemaComplexTypeParticleRestriction}
		}
	}
	return nil
}

func (resolver *schemaComplexTypeResolver) referencesMayOverlap(first, later ElementReferenceParticle) bool {
	if first.Ref() == later.Ref() {
		return true
	}
	for _, record := range resolver.records {
		if record.element == nil {
			continue
		}
		if schemaGroupSubstitutionMayMatch(resolver.records, first.Ref(), record.name) &&
			schemaGroupSubstitutionMayMatch(resolver.records, later.Ref(), record.name) {
			return true
		}
	}
	return false
}

func (resolver *schemaComplexTypeResolver) invalidBoundedRestriction(loc Loc, message string, baseLoc Loc) error {
	return Diagnostic{class: FailureInvalid, code: invalidSchemaCompositionCode, loc: loc,
		message: message, related: []Loc{baseLoc}, specRef: resolver.boundedRestrictionSpecRef(),
		cause: errSchemaComplexTypeParticleRestriction}
}

func (resolver *schemaComplexTypeResolver) unsupportedBoundedRestriction(loc Loc, message string, baseLoc Loc) error {
	return newSchemaComplexTypeUnsupportedWithSpec(loc, message, []Loc{baseLoc}, resolver.version,
		errSchemaComplexTypeParticleUnsupported, resolver.boundedRestrictionSpecRef())
}

func (resolver *schemaComplexTypeResolver) boundedRestrictionSpecRef() string {
	if resolver.version == XSDVersion10 {
		return "xsd10-structures#cos-particle-restrict"
	}
	return "xsd11-structures#cos-content-act-restrict"
}
