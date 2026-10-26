package goxsd9

import "fmt"

// schemaBoundedRestrictionSources retains the two authored parts of an
// extension's effective content without inventing a source location for it.
type schemaBoundedRestrictionSources struct {
	annotation ElementReferenceParticle
	group      ModelGroupReferenceParticle
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
	sequenceInput, ok := input.particle.(*schemaSequenceParticleInput)
	if !ok || sequenceInput == nil {
		return newSchemaBridgeInvariant(input.restrictionLoc, "bounded restriction lost its sequence input")
	}
	if sequenceInput.occurrences.minimumOccurrence().String() == "0" {
		return resolver.invalidBoundedRestriction(sequenceInput.loc, "restriction sequence admits empty content excluded by its base", base.group.Loc())
	}
	if maximum := sequenceInput.occurrences.maximumOccurrence().String(); maximum != "0" && maximum != "1" {
		return resolver.invalidBoundedRestriction(sequenceInput.loc, "restriction sequence admits more occurrences than its base", base.group.Loc())
	}
	if len(sequenceInput.particles) == 2 {
		if groupInput, groupOK := sequenceInput.particles[1].(*schemaModelGroupReferenceParticleInput); groupOK && groupInput.occurrences.minimumOccurrence().String() == "0" {
			return resolver.invalidBoundedRestriction(groupInput.loc, "restriction group reference admits fewer occurrences than its base", base.group.Loc())
		}
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
	if err := resolver.proveBoundedRestrictionAttribution(annotation, group); err != nil {
		return err
	}
	if resolver.version == XSDVersion10 {
		return resolver.proveXSD10BoundedRestriction(base, annotation, group)
	}
	return resolver.proveXSD11BoundedRestriction(base, annotation, group)
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

func (resolver *schemaComplexTypeResolver) proveBoundedRestrictionAttribution(annotation ElementReferenceParticle, group ModelGroupReferenceParticle) error {
	index := schemaGroupRecordByID(resolver.records, group.TargetID())
	if index < 0 || !resolver.modelGroups[index].present {
		return newSchemaBridgeInvariant(group.RefLoc(), "restriction group target has no completed model")
	}
	choice, ok := resolver.modelGroups[index].particle.(ChoiceParticle)
	if !ok || !choice.Occurrences().IsDefault() || len(choice.Alternatives()) != 3 {
		return resolver.unsupportedBoundedRestriction(group.RefLoc(), "restriction group target is outside the copied three-reference choice", resolver.records[index].loc)
	}
	refs := []ElementReferenceParticle{annotation}
	for _, alternative := range choice.Alternatives() {
		ref, ok := alternative.(ElementReferenceParticle)
		if !ok || !ref.Occurrences().IsDefault() {
			return resolver.unsupportedBoundedRestriction(group.RefLoc(), "restriction group target has an unproved alternative", alternative.Loc())
		}
		refs = append(refs, ref)
	}
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
