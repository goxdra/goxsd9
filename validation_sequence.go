package goxsd9

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

type instanceSequenceProgram struct {
	version     XSDVersion
	occurrences particleOccurrenceRange
	related     []Loc
	particles   []instanceSequenceParticle
}

type instanceSequenceParticle struct {
	name        QName
	loc         Loc
	occurrences particleOccurrenceRange
	scalar      instanceScalarType
}

type instanceSequenceCandidate struct {
	outer              *big.Int
	index              int
	count              *big.Int
	currentEmpty       bool
	emptyRepeatBlocked bool
}

type instanceSequenceOpenChild struct {
	name    syntaxName
	loc     Loc
	paths   []instanceSequenceCandidate
	text    strings.Builder
	textLoc Loc
	hasText bool
	nested  bool
}

type instanceSequenceValidator struct {
	program  instanceSequenceProgram
	rootLoc  Loc
	depth    int
	frontier []instanceSequenceCandidate
	open     *instanceSequenceOpenChild
}

type instanceValidationObserver struct {
	schema    Schema
	started   bool
	streaming bool
	validator *instanceSequenceValidator
}

func newInstanceValidationObserver(schema Schema) *instanceValidationObserver {
	return &instanceValidationObserver{schema: schema}
}

func (observer *instanceValidationObserver) startElement(name syntaxName, loc Loc, attrs []instanceAttribute) (bool, error) {
	if observer.started {
		if !observer.streaming {
			return true, nil
		}
		return false, observer.validator.startElement(name, loc, attrs)
	}
	observer.started = true
	rootName, err := NewQName(name.namespace, name.local)
	if err != nil {
		return true, newInstanceValidationInternal(
			loc,
			"instance root name could not be expanded",
			nil,
			err,
		)
	}
	declaration, err := instanceSchemaElement(observer.schema, rootName, loc)
	if err != nil {
		return true, err
	}
	definition, sequence, hasSequence, err := instanceSequenceDefinitionFor(observer.schema, declaration, loc)
	if err != nil {
		return false, err
	}
	if !hasSequence {
		return true, nil
	}
	if factsErr := rejectUnsupportedInstanceElementFacts(observer.schema, declaration, loc); factsErr != nil {
		return false, factsErr
	}
	program, err := instanceSequenceProgramFor(observer.schema, declaration, definition, sequence, loc)
	if err != nil {
		return false, err
	}
	observer.streaming = true
	observer.validator = &instanceSequenceValidator{program: program}
	return false, observer.validator.startElement(name, loc, attrs)
}

func (observer *instanceValidationObserver) endElement(name syntaxName, loc Loc) error {
	if !observer.streaming {
		return nil
	}
	return observer.validator.endElement(name, loc)
}

func (observer *instanceValidationObserver) characterData(data []byte, loc Loc) error {
	if !observer.streaming {
		return nil
	}
	return observer.validator.characterData(data, loc)
}

func instanceSequenceDefinitionFor(schema Schema, declaration ElementDeclaration, loc Loc) (ComplexTypeDefinition, SequenceParticle, bool, error) {
	if declaration.DeclaredType().Namespace() == xsdNamespaceURI {
		return ComplexTypeDefinition{}, SequenceParticle{}, false, nil
	}
	typeID, hasTypeID := declaration.TypeID()
	if !hasTypeID || typeID.IsZero() {
		return ComplexTypeDefinition{}, SequenceParticle{}, false, nil
	}
	target, ok := schema.Lookup(typeID)
	if !ok {
		return ComplexTypeDefinition{}, SequenceParticle{}, false, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("global element type ID %v is not in the completed schema", typeID),
			[]Loc{declaration.Loc()},
			errInstanceValidationInvariant,
		)
	}
	if target.Kind() != ComponentKindComplexTypeDefinition {
		return ComplexTypeDefinition{}, SequenceParticle{}, false, nil
	}
	definition, ok := target.ComplexTypeDefinition()
	if !ok {
		return ComplexTypeDefinition{}, SequenceParticle{}, false, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("global element type ID %v has no complex type view", typeID),
			[]Loc{declaration.Loc(), target.Loc()},
			errInstanceValidationInvariant,
		)
	}
	sequence, hasSequence := definition.Particle().(SequenceParticle)
	return definition, sequence, hasSequence, nil
}

//nolint:gocognit,funlen // Keep direct sequence shape gates and ordered target planning together.
func instanceSequenceProgramFor(
	schema Schema,
	declaration ElementDeclaration,
	definition ComplexTypeDefinition,
	sequence SequenceParticle,
	loc Loc,
) (instanceSequenceProgram, error) {
	version := instanceSchemaValidationVersion(schema)
	if definition.facts == nil || sequence.facts == nil {
		return instanceSequenceProgram{}, newInstanceValidationInternal(
			loc,
			"direct sequence validation received incomplete schema facts",
			[]Loc{declaration.Loc(), definition.Loc()},
			errInstanceValidationInvariant,
		)
	}
	related := []Loc{declaration.Loc(), definition.Loc(), sequence.Loc()}
	if body := definition.extensionBody(); body != nil {
		related = appendInstanceRelated(related, body.complexContentLoc)
		related = appendInstanceRelated(related, body.extensionLoc)
		related = appendInstanceRelated(related, body.base.loc)
		if body.particle != nil {
			related = appendInstanceRelated(related, body.particle.Loc())
		}
		if body.anyAttribute != nil {
			related = appendInstanceRelated(related, body.anyAttribute.loc)
		}
		return instanceSequenceProgram{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named complex type %q uses complex-content extension outside instance validation", definition.Name()),
			related,
			version,
			errInstanceComplexContentExtension,
		)
	}
	if body, ok := definition.boundedOpenAttrsRestrictionBody(); ok {
		related = appendInstanceRelated(related, body.complexContentLoc)
		related = appendInstanceRelated(related, body.restrictionLoc)
		related = appendInstanceRelated(related, body.base.loc)
		related = appendInstanceRelated(related, body.anyAttribute.loc)
		return instanceSequenceProgram{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named complex type %q uses bounded openAttrs content outside instance validation", definition.Name()),
			related,
			version,
			errInstanceOpenAttrsType,
		)
	}
	if anyAttribute, ok := definition.AnyAttribute(); ok {
		related = appendInstanceRelated(related, anyAttribute.Loc())
		return instanceSequenceProgram{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named complex type %q attribute wildcards are outside direct sequence validation", definition.Name()),
			related,
			version,
			errInstanceAttributes,
		)
	}
	rawParticles := sequence.Particles()
	hasElement := false
	hasReference := false
	hasOther := false
	for _, rawParticle := range rawParticles {
		if rawParticle == nil {
			hasOther = true
			continue
		}
		if element, ok := elementParticleValue(rawParticle); ok {
			hasElement = true
			related = appendInstanceRelated(related, element.Loc())
			continue
		}
		if reference, ok := elementReferenceParticleValue(rawParticle); ok {
			hasReference = true
			related = appendInstanceRelated(related, reference.Loc())
			continue
		}
		hasOther = true
	}
	if hasElement && hasReference {
		return instanceSequenceProgram{}, newInstanceValidationUnsupported(
			loc,
			"direct sequence mixes local element declarations and element references",
			related,
			version,
			errInstanceSequenceMixed,
		)
	}
	if hasReference {
		return instanceSequenceProgram{}, newInstanceValidationUnsupported(
			loc,
			"direct sequence element references are outside direct sequence validation",
			related,
			version,
			errInstanceSequenceTarget,
		)
	}
	if hasOther {
		return instanceSequenceProgram{}, newInstanceValidationUnsupported(
			loc,
			"direct sequence contains an unsupported particle form",
			related,
			version,
			errInstanceSequenceParticle,
		)
	}
	particles := make([]instanceSequenceParticle, 0, len(rawParticles))
	for _, rawParticle := range rawParticles {
		element, ok := elementParticleValue(rawParticle)
		if !ok {
			return instanceSequenceProgram{}, newInstanceValidationUnsupported(
				loc,
				"direct sequence contains an unsupported particle form",
				related,
				version,
				errInstanceSequenceParticle,
			)
		}
		if element.facts == nil {
			return instanceSequenceProgram{}, newInstanceValidationInternal(
				loc,
				"direct sequence element has incomplete particle facts",
				related,
				errInstanceValidationInvariant,
			)
		}
		if element.Name().IsZero() {
			return instanceSequenceProgram{}, newInstanceValidationInternal(
				loc,
				"direct sequence element has no expanded name",
				related,
				errInstanceValidationInvariant,
			)
		}
		childRelated := []Loc{declaration.Loc(), definition.Loc(), sequence.Loc(), element.Loc()}
		if element.IsNillable() {
			return instanceSequenceProgram{}, newInstanceValidationUnsupported(
				loc,
				fmt.Sprintf("local sequence element %q has nillable=true outside instance validation", element.Name()),
				childRelated,
				version,
				errInstanceLocalElementFacts,
			)
		}
		typeID, hasTypeID := element.TypeID()
		scalar, err := instanceScalarTypeForTarget(
			schema,
			element.DeclaredType(),
			typeID,
			hasTypeID,
			childRelated,
			loc,
			version,
			false,
			false,
			version,
		)
		if err != nil {
			return instanceSequenceProgram{}, err
		}
		particles = append(particles, instanceSequenceParticle{
			name:        element.Name(),
			loc:         element.Loc(),
			occurrences: element.facts.occurrences.clone(),
			scalar:      scalar,
		})
	}
	return instanceSequenceProgram{
		version:     version,
		occurrences: sequence.facts.occurrences.clone(),
		related:     relCopy(related),
		particles:   particles,
	}, nil
}

//nolint:gocognit // Keep root, direct-child, and nested-content event handling together.
func (validator *instanceSequenceValidator) startElement(name syntaxName, loc Loc, attrs []instanceAttribute) error {
	if validator.depth == 0 {
		validator.rootLoc = loc
		validator.depth = 1
		validator.frontier = sequenceClosure(validator.program, []instanceSequenceCandidate{newInstanceSequenceCandidate(0, 0, big.NewInt(0), true, false)})
		if len(attrs) == 0 {
			return nil
		}
		return newInstanceValidationUnsupported(
			attrs[0].loc,
			fmt.Sprintf("attribute %q is not supported for direct sequence validation", renderSyntaxName(attrs[0].name)),
			validator.program.related,
			validator.program.version,
			errInstanceAttributes,
		)
	}
	if validator.depth == 1 {
		child := &instanceSequenceOpenChild{name: name, loc: loc}
		childName, err := NewQName(name.namespace, name.local)
		if err != nil {
			validator.depth = 2
			validator.open = child
			return newInstanceValidationInternal(
				loc,
				"direct sequence child name could not be expanded",
				validator.program.related,
				err,
			)
		}
		for _, candidate := range validator.frontier {
			if candidate.index >= len(validator.program.particles) {
				continue
			}
			particle := validator.program.particles[candidate.index]
			if particle.name != childName || !sequenceCandidateCanConsume(candidate, particle.occurrences) {
				continue
			}
			next := candidate.clone()
			next.count.Add(next.count, big.NewInt(1))
			next.currentEmpty = false
			next.emptyRepeatBlocked = false
			child.paths = append(child.paths, next)
		}
		validator.depth = 2
		validator.open = child
		if len(child.paths) == 0 {
			validator.frontier = nil
			return newInstanceSequenceInvalid(
				loc,
				fmt.Sprintf("direct sequence child %q is unexpected at this position", renderSyntaxName(name)),
				validator.program.related,
				validator.program.version,
				errInstanceSequenceUnexpected,
			)
		}
		if len(attrs) == 0 {
			return nil
		}
		return newInstanceValidationUnsupported(
			attrs[0].loc,
			fmt.Sprintf("attribute %q is not supported for direct sequence scalar validation", renderSyntaxName(attrs[0].name)),
			instanceSequenceOpenRelated(validator.program, child),
			validator.program.version,
			errInstanceAttributes,
		)
	}
	if validator.open != nil && validator.depth == 2 {
		validator.open.nested = true
		validator.frontier = nil
		validator.depth++
		return newInstanceSequenceInvalid(
			loc,
			fmt.Sprintf("direct sequence child %q contains nested element %q", renderSyntaxName(validator.open.name), renderSyntaxName(name)),
			instanceSequenceOpenRelated(validator.program, validator.open),
			validator.program.version,
			errInstanceSequenceNested,
		)
	}
	validator.depth++
	return nil
}

//nolint:gocognit // Keep child scalar completion and outer occurrence acceptance ordered.
func (validator *instanceSequenceValidator) endElement(_ syntaxName, loc Loc) error {
	if validator.depth == 2 {
		child := validator.open
		validator.open = nil
		if child == nil {
			validator.depth = 1
			return newInstanceValidationInternal(
				loc,
				"direct sequence has no active child state",
				validator.program.related,
				errInstanceValidationInvariant,
			)
		}
		if child.nested {
			validator.depth = 1
			return nil
		}
		if len(child.paths) == 0 {
			validator.depth = 1
			return nil
		}
		lexical := child.text.String()
		valueLoc := child.loc
		if child.hasText {
			valueLoc = child.textLoc
		}
		valid := make([]instanceSequenceCandidate, 0, len(child.paths))
		var firstScalarErr error
		for _, path := range child.paths {
			particle := validator.program.particles[path.index]
			if scalarErr := validateScalarLexicalValue(
				syntaxName{namespace: particle.name.Namespace(), local: particle.name.Local()},
				lexical,
				valueLoc,
				particle.scalar,
			); scalarErr != nil {
				if firstScalarErr == nil {
					firstScalarErr = scalarErr
				}
				continue
			}
			valid = append(valid, path)
		}
		validator.depth = 1
		if len(valid) == 0 {
			validator.frontier = nil
			if firstScalarErr != nil {
				return firstScalarErr
			}
			return newInstanceValidationInternal(
				child.loc,
				"direct sequence scalar produced no validation result",
				instanceSequenceOpenRelated(validator.program, child),
				errInstanceValidationInvariant,
			)
		}
		validator.frontier = sequenceClosure(validator.program, valid)
		return nil
	}
	if validator.depth == 1 {
		validator.depth = 0
		if sequenceAccepts(validator.program, validator.frontier) {
			return nil
		}
		return newInstanceSequenceInvalid(
			validator.rootLoc,
			"direct sequence content does not satisfy its occurrence ranges",
			validator.program.related,
			validator.program.version,
			errInstanceSequenceMissing,
		)
	}
	if validator.depth > 2 {
		validator.depth--
	}
	return nil
}

func (validator *instanceSequenceValidator) characterData(data []byte, loc Loc) error {
	if validator.depth == 1 {
		if xmlWhitespace(data) {
			return nil
		}
		return newInstanceSequenceInvalid(
			loc,
			"direct sequence parent contains non-whitespace character data",
			validator.program.related,
			validator.program.version,
			errInstanceSequenceText,
		)
	}
	if validator.depth == 2 && validator.open != nil {
		if !validator.open.hasText {
			validator.open.textLoc = loc
			validator.open.hasText = true
		}
		validator.open.text.WriteString(string(data))
	}
	return nil
}

func instanceSequenceOpenRelated(program instanceSequenceProgram, child *instanceSequenceOpenChild) []Loc {
	if child == nil || len(child.paths) == 0 {
		return program.related
	}
	particle := program.particles[child.paths[0].index]
	return particle.scalar.related
}

func newInstanceSequenceInvalid(loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return newInstanceValidationInvalid(
		InvalidInstanceSequenceCode,
		loc,
		message,
		related,
		instanceValidationSpecRef(version),
		cause,
	)
}

func sequenceAccepts(program instanceSequenceProgram, frontier []instanceSequenceCandidate) bool {
	if len(program.particles) == 0 {
		return true
	}
	for _, candidate := range sequenceClosure(program, frontier) {
		if candidate.index != len(program.particles) {
			continue
		}
		if sequenceCandidateOuterAllowed(candidate.outer, program) {
			return true
		}
	}
	return false
}

func sequenceCandidateCanConsume(candidate instanceSequenceCandidate, occurrences particleOccurrenceRange) bool {
	maximum := occurrences.maximum
	if maximum.isUnbounded() {
		return true
	}
	return candidate.count.Cmp(maximum.finite.integerCopy()) < 0
}

func sequenceCandidateCanStart(outer *big.Int, occurrences particleOccurrenceRange) bool {
	maximum := occurrences.maximum
	if maximum.isUnbounded() {
		return true
	}
	return outer.Cmp(maximum.finite.integerCopy()) < 0
}

func sequenceCandidateOuterAllowed(outer *big.Int, program instanceSequenceProgram) bool {
	if sequenceBodyCanBeEmpty(program) {
		maximum := program.occurrences.maximum
		if maximum.isUnbounded() {
			return true
		}
		return outer.Cmp(maximum.finite.integerCopy()) <= 0
	}
	if outer.Cmp(program.occurrences.minimum.finite.integerCopy()) < 0 {
		return false
	}
	maximum := program.occurrences.maximum
	if maximum.isUnbounded() {
		return true
	}
	return outer.Cmp(maximum.finite.integerCopy()) <= 0
}

func sequenceBodyCanBeEmpty(program instanceSequenceProgram) bool {
	for _, particle := range program.particles {
		if particle.occurrences.minimum.finite.integerCopy().Sign() != 0 {
			return false
		}
	}
	return true
}

//nolint:gocognit // Keep bounded epsilon closure transitions explicit and deterministic.
func sequenceClosure(program instanceSequenceProgram, initial []instanceSequenceCandidate) []instanceSequenceCandidate {
	if len(program.particles) == 0 {
		return nil
	}
	queue := make([]instanceSequenceCandidate, 0, len(initial))
	for _, candidate := range initial {
		queue = append(queue, candidate.clone())
	}
	result := make([]instanceSequenceCandidate, 0, len(initial))
	for queueIndex := 0; queueIndex < len(queue); queueIndex++ {
		candidate := queue[queueIndex]
		if sequenceCandidateContains(result, candidate) {
			continue
		}
		result = append(result, candidate)
		if candidate.index == len(program.particles) {
			if !sequenceCandidateCanStart(candidate.outer, program.occurrences) {
				continue
			}
			if candidate.currentEmpty && candidate.emptyRepeatBlocked {
				continue
			}
			queue = append(queue, newInstanceSequenceCandidate(0, 0, candidate.outer, true, candidate.currentEmpty))
			continue
		}
		if candidate.index == 0 && candidate.currentEmpty && candidate.count.Sign() == 0 && sequenceCandidateOuterAllowed(candidate.outer, program) {
			queue = append(queue, newInstanceSequenceCandidate(len(program.particles), 0, candidate.outer, false, false))
		}
		particle := program.particles[candidate.index]
		if candidate.count.Cmp(particle.occurrences.minimum.finite.integerCopy()) < 0 {
			continue
		}
		next := candidate.clone()
		next.index++
		next.count.SetInt64(0)
		if next.index == len(program.particles) && !next.currentEmpty {
			next.outer.Add(next.outer, big.NewInt(1))
		}
		queue = append(queue, next)
	}
	return compactSequenceCandidates(result, len(program.particles))
}

func newInstanceSequenceCandidate(index int, count int64, outer *big.Int, currentEmpty, emptyRepeatBlocked bool) instanceSequenceCandidate {
	return instanceSequenceCandidate{
		outer:              new(big.Int).Set(outer),
		index:              index,
		count:              big.NewInt(count),
		currentEmpty:       currentEmpty,
		emptyRepeatBlocked: emptyRepeatBlocked,
	}
}

func (candidate instanceSequenceCandidate) clone() instanceSequenceCandidate {
	return instanceSequenceCandidate{
		outer:              new(big.Int).Set(candidate.outer),
		index:              candidate.index,
		count:              new(big.Int).Set(candidate.count),
		currentEmpty:       candidate.currentEmpty,
		emptyRepeatBlocked: candidate.emptyRepeatBlocked,
	}
}

func sequenceCandidateContains(candidates []instanceSequenceCandidate, candidate instanceSequenceCandidate) bool {
	for _, existing := range candidates {
		if existing.index != candidate.index || existing.currentEmpty != candidate.currentEmpty || existing.emptyRepeatBlocked != candidate.emptyRepeatBlocked {
			continue
		}
		if existing.outer.Cmp(candidate.outer) == 0 && existing.count.Cmp(candidate.count) == 0 {
			return true
		}
	}
	return false
}

//nolint:gocognit // Keep bounded candidate compaction grouped by its state dimensions.
func compactSequenceCandidates(candidates []instanceSequenceCandidate, particleCount int) []instanceSequenceCandidate {
	result := make([]instanceSequenceCandidate, 0)
	for index := 0; index <= particleCount; index++ {
		for _, currentEmpty := range []bool{false, true} {
			for _, emptyRepeatBlocked := range []bool{false, true} {
				group := make([]instanceSequenceCandidate, 0)
				for _, candidate := range candidates {
					if candidate.index != index || candidate.currentEmpty != currentEmpty || candidate.emptyRepeatBlocked != emptyRepeatBlocked {
						continue
					}
					group = append(group, candidate)
				}
				if len(group) == 0 {
					continue
				}
				minOuter, maxOuter := group[0], group[0]
				for _, candidate := range group[1:] {
					if candidate.outer.Cmp(minOuter.outer) < 0 {
						minOuter = candidate
					}
					if candidate.outer.Cmp(maxOuter.outer) > 0 {
						maxOuter = candidate
					}
				}
				for _, outer := range []instanceSequenceCandidate{minOuter, maxOuter} {
					minCount, maxCount := outer, outer
					for _, candidate := range group {
						if candidate.outer.Cmp(outer.outer) != 0 {
							continue
						}
						if candidate.count.Cmp(minCount.count) < 0 {
							minCount = candidate
						}
						if candidate.count.Cmp(maxCount.count) > 0 {
							maxCount = candidate
						}
					}
					for _, selected := range []instanceSequenceCandidate{minCount, maxCount} {
						if !sequenceCandidateContains(result, selected) {
							result = append(result, selected.clone())
						}
					}
				}
			}
		}
	}
	return result
}

func combineInstanceErrors(primary, additional error) error {
	if primary == nil {
		return additional
	}
	if additional == nil {
		return primary
	}
	for _, diagnostic := range syntaxDiagnostics(additional) {
		primary = combineSyntaxErrors(primary, diagnostic)
	}
	return primary
}

func instanceDiagnosticsMatch(first Diagnostic, second error) bool {
	var diagnostic Diagnostic
	if !errors.As(second, &diagnostic) {
		return false
	}
	return first.class == diagnostic.class &&
		first.code == diagnostic.code &&
		first.feature == diagnostic.feature &&
		first.loc == diagnostic.loc &&
		first.message == diagnostic.message &&
		first.specRef == diagnostic.specRef
}
