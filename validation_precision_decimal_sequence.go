package goxsd9

import "fmt"

const (
	instanceParticleXSD10SpecRef = "xsd10-structures#cvc-particle"
	instanceParticleXSD11SpecRef = "xsd11-structures#cvc-particle"
)

func instanceParticleSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceParticleXSD10SpecRef
	}
	return instanceParticleXSD11SpecRef
}

type instanceVarietyChildPlan struct {
	name        QName
	loc         Loc
	occurrences ParticleOccurrenceRange
	value       instanceVarietyPlan
	target      ElementDeclaration
	refLoc      Loc
}

type instanceVarietySequenceProgram struct {
	version     XSDVersion
	related     []Loc
	children    []instanceVarietyChildPlan
	sequenceLoc Loc
}

// This tree path is selected only for direct sequences containing a bounded
// precisionDecimal list or union. It preserves the lexical particle order.
//
//nolint:gocognit // Selection and ordered particle planning share schema gates.
func instanceVarietySequenceProgramFor(schema Schema, declaration ElementDeclaration, loc Loc) (instanceVarietySequenceProgram, bool, error) {
	definition, ok := declaration.InlineComplexType()
	if !ok {
		id, hasID := declaration.TypeID()
		if !hasID {
			return instanceVarietySequenceProgram{}, false, nil
		}
		component, found := schema.Lookup(id)
		if !found {
			return instanceVarietySequenceProgram{}, false, nil
		}
		definition, ok = component.ComplexTypeDefinition()
	}
	if !ok {
		return instanceVarietySequenceProgram{}, false, nil
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		return instanceVarietySequenceProgram{}, false, nil
	}
	particles := sequence.Particles()
	selected := false
	for _, particle := range particles {
		_, reference, _, _, found := instanceVarietyParticleReference(schema, particle)
		if found && (reference.Variety() == SimpleTypeVarietyList || reference.Variety() == SimpleTypeVarietyUnion) {
			selected = true
			break
		}
	}
	if !selected {
		return instanceVarietySequenceProgram{}, false, nil
	}
	version := instanceSchemaValidationVersion(schema)
	related := []Loc{declaration.Loc(), definition.Loc(), sequence.Loc()}
	if factsErr := rejectUnsupportedInstanceElementFacts(schema, declaration, loc); factsErr != nil {
		return instanceVarietySequenceProgram{}, true, factsErr
	}
	if err := rejectUnsupportedAttributeSequenceRoot(definition, declaration, sequence, loc, version); err != nil {
		return instanceVarietySequenceProgram{}, true, err
	}
	if !sequence.Occurrences().IsDefault() {
		return instanceVarietySequenceProgram{}, true, newInstanceValidationUnsupported(sequence.Loc(), "outer sequence occurrences are outside bounded list/union validation", related, version, errInstanceSequenceParticle)
	}
	program := instanceVarietySequenceProgram{version: version, related: related, sequenceLoc: sequence.Loc()}
	for _, particle := range particles {
		name, reference, target, refLoc, found := instanceVarietyParticleReference(schema, particle)
		if !found {
			return instanceVarietySequenceProgram{}, true, newInstanceValidationUnsupported(particle.Loc(), "sequence particle is outside bounded list/union validation", related, version, errInstanceSequenceParticle)
		}
		childRelated := appendInstanceRelated(relCopy(related), particle.Loc())
		if !refLoc.IsZero() {
			childRelated = appendInstanceRelated(childRelated, refLoc)
			childRelated = appendInstanceRelated(childRelated, target.Loc())
		}
		if element, local := elementParticleValue(particle); local && element.IsNillable() {
			return instanceVarietySequenceProgram{}, true, newInstanceValidationUnsupported(loc, "local sequence element has nillable=true outside validation", childRelated, version, errInstanceLocalElementFacts)
		}
		plan, err := instanceVarietyPlanFor(schema, reference, childRelated, particle.Loc(), version)
		if err != nil {
			return instanceVarietySequenceProgram{}, true, err
		}
		program.children = append(program.children, instanceVarietyChildPlan{
			name: name, loc: particle.Loc(), occurrences: particle.Occurrences(), value: plan, target: target, refLoc: refLoc,
		})
	}
	return program, true, nil
}

func instanceVarietyParticleReference(schema Schema, particle Particle) (QName, SimpleTypeReference, ElementDeclaration, Loc, bool) {
	if element, local := elementParticleValue(particle); local {
		reference, ok := element.TypeReference()
		return element.Name(), reference, ElementDeclaration{}, Loc{}, ok
	}
	referenceParticle, ref := elementReferenceParticleValue(particle)
	if !ref {
		return QName{}, SimpleTypeReference{}, ElementDeclaration{}, Loc{}, false
	}
	component, found := schema.Lookup(referenceParticle.TargetID())
	if !found {
		return QName{}, SimpleTypeReference{}, ElementDeclaration{}, Loc{}, false
	}
	target, ok := component.ElementDeclaration()
	if !ok {
		return QName{}, SimpleTypeReference{}, ElementDeclaration{}, Loc{}, false
	}
	typeReference, typed := target.TypeReference()
	return referenceParticle.Name(), typeReference, target, referenceParticle.RefLoc(), typed
}

//nolint:gocognit,funlen // Finish all ordered content checks before validating values.
func validateVarietySequenceInstance(root *instanceElement, program instanceVarietySequenceProgram) error {
	for _, attr := range root.attrs {
		if attr.name.namespace == schemaInstanceNamespaceURI && attr.name.local == "schemaLocation" {
			continue
		}
		if attr.name.namespace == schemaInstanceNamespaceURI {
			return unsupportedInstanceSchemaHint(attr, program.related, program.version)
		}
		return newInstanceAttributeInvalid(attr.loc, "unexpected root attribute", program.related, program.version, errInstanceAttributeUnexpected)
	}
	children := make([]*instanceElement, 0, len(root.children))
	for _, node := range root.children {
		if child, element := node.(*instanceElement); element {
			if child == nil {
				return newInstanceValidationInternal(root.loc, "nil sequence child", program.related, errInstanceValidationInvariant)
			}
			children = append(children, child)
			continue
		}
		text, ok := node.(instanceText)
		if !ok {
			return newInstanceValidationInternal(root.loc, "unknown sequence node", program.related, errInstanceValidationInvariant)
		}
		if !xmlWhitespace([]byte(text.data)) {
			return newInstanceValidationInvalid(InvalidInstanceSequenceCode, text.loc, "non-whitespace sequence text", program.related, instanceParticleSpecRef(program.version), errInstanceSequenceText)
		}
	}
	type matchedChild struct {
		element *instanceElement
		plan    instanceVarietyChildPlan
	}
	matched := make([]matchedChild, 0, len(children))
	index := 0
	for _, planned := range program.children {
		count := 0
		for index < len(children) && instanceNameMatches(children[index].name, planned.name) {
			if maximum, finite := planned.occurrences.Maximum().Finite(); finite && integerFromCount(count).Compare(maximum) >= 0 {
				break
			}
			matched = append(matched, matchedChild{element: children[index], plan: planned})
			index++
			count++
		}
		if integerFromCount(count).Compare(planned.occurrences.Minimum()) < 0 {
			return newInstanceValidationInvalid(InvalidInstanceSequenceCode, root.loc, fmt.Sprintf("missing required sequence child %q", planned.name), appendInstanceRelated(relCopy(program.related), planned.loc), instanceParticleSpecRef(program.version), errInstanceSequenceMissing)
		}
	}
	if index < len(children) {
		return newInstanceValidationInvalid(InvalidInstanceSequenceCode, children[index].loc, "unexpected sequence child", program.related, instanceParticleSpecRef(program.version), errInstanceSequenceUnexpected)
	}
	for _, entry := range matched {
		if entry.plan.target.facts != nil {
			related := appendInstanceRelated(relCopy(program.related), entry.plan.refLoc)
			related = appendInstanceRelated(related, entry.plan.target.Loc())
			if err := rejectUnsupportedInstanceElementFactsWithRelated(entry.plan.target, entry.element.loc, related, program.version); err != nil {
				return err
			}
		}
		for _, attr := range entry.element.attrs {
			return newInstanceValidationUnsupported(attr.loc, "sequence scalar attributes are outside bounded list/union validation", entry.plan.value.related, program.version, errInstanceAttributes)
		}
		for _, node := range entry.element.children {
			if nested, element := node.(*instanceElement); element {
				if nested == nil {
					return newInstanceValidationInternal(entry.element.loc, "nil nested sequence element", entry.plan.value.related, errInstanceValidationInvariant)
				}
				return newInstanceValidationInvalid(InvalidInstanceSequenceCode, nested.loc, "nested sequence scalar element", entry.plan.value.related, instanceParticleSpecRef(program.version), errInstanceSequenceNested)
			}
			if _, text := node.(instanceText); !text {
				return newInstanceValidationInternal(entry.element.loc, "unknown scalar node", entry.plan.value.related, errInstanceValidationInvariant)
			}
		}
	}
	for _, entry := range matched {
		lexical, valueLoc := instanceScalarText(entry.element)
		if _, err := validateInstanceVarietyValue(entry.element.name, lexical, valueLoc, entry.plan.value); err != nil {
			return err
		}
	}
	return nil
}
