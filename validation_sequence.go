package goxsd9

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
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
	// Count bounds are affine in outer; the count fields are their intercepts.
	outer              *big.Int
	outerMaximum       *big.Int
	index              int
	count              *big.Int
	countMaximum       *big.Int
	countMinimumSlope  *big.Int
	countMaximumSlope  *big.Int
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
			if particle.name != childName {
				continue
			}
			for _, next := range sequenceCandidateConsume(candidate, particle.occurrences) {
				next.currentEmpty = false
				next.emptyRepeatBlocked = false
				child.paths = append(child.paths, next)
			}
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
		if sequenceCandidateOuterAllowed(candidate, program) {
			return true
		}
	}
	return false
}

func sequenceCandidateConsume(candidate instanceSequenceCandidate, occurrences particleOccurrenceRange) []instanceSequenceCandidate {
	maximum := occurrences.maximum
	if maximum.isUnbounded() {
		next := candidate.clone()
		next.count.Add(next.count, big.NewInt(1))
		next.countMaximum.Add(next.countMaximum, big.NewInt(1))
		return []instanceSequenceCandidate{next}
	}
	limit := maximum.finite.integerCopy()
	limit.Sub(limit, big.NewInt(1))
	valid := sequenceCandidateOuterWhereAtMost(candidate, candidate.countMinimumSlope, candidate.count, limit)
	if len(valid) == 0 {
		return nil
	}
	result := make([]instanceSequenceCandidate, 0, len(valid)*2)
	for _, segment := range valid {
		for _, belowMaximum := range sequenceCandidateOuterWhereAtMost(segment, segment.countMaximumSlope, segment.countMaximum, limit) {
			next := belowMaximum.clone()
			next.count.Add(next.count, big.NewInt(1))
			next.countMaximum.Add(next.countMaximum, big.NewInt(1))
			result = append(result, next)
		}
		for _, atMaximum := range sequenceCandidateOuterWhereAtLeast(segment, segment.countMaximumSlope, segment.countMaximum, new(big.Int).Add(limit, big.NewInt(1))) {
			next := atMaximum.clone()
			next.count.Add(next.count, big.NewInt(1))
			next.countMaximum.Set(limit)
			next.countMaximum.Add(next.countMaximum, big.NewInt(1))
			next.countMaximumSlope.SetInt64(0)
			result = append(result, next)
		}
	}
	return result
}

func sequenceCandidateStart(candidate instanceSequenceCandidate, occurrences particleOccurrenceRange) []instanceSequenceCandidate {
	maximum := occurrences.maximum
	if maximum.isUnbounded() {
		return []instanceSequenceCandidate{candidate.clone()}
	}
	limit := maximum.finite.integerCopy()
	limit.Sub(limit, big.NewInt(1))
	return sequenceCandidateOuterSubset(candidate, candidate.outer, limit)
}

func sequenceCandidateOuterAllowed(candidate instanceSequenceCandidate, program instanceSequenceProgram) bool {
	return len(sequenceCandidateOuterAllowedSubset(candidate, program)) != 0
}

func sequenceCandidateOuterAllowedSubset(candidate instanceSequenceCandidate, program instanceSequenceProgram) []instanceSequenceCandidate {
	minimum := program.occurrences.minimum.finite.integerCopy()
	if sequenceBodyCanBeEmpty(program) {
		minimum.SetInt64(0)
	}
	result := sequenceCandidateOuterSubset(candidate, minimum, nil)
	if len(result) == 0 {
		return nil
	}
	maximum := program.occurrences.maximum
	if maximum.isUnbounded() {
		return result
	}
	return sequenceCandidateOuterSubset(result[0], nil, maximum.finite.integerCopy())
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
			for _, start := range sequenceCandidateStart(candidate, program.occurrences) {
				if start.currentEmpty && start.emptyRepeatBlocked {
					continue
				}
				next := start.clone()
				next.index = 0
				next.count.SetInt64(0)
				next.countMaximum.SetInt64(0)
				next.countMinimumSlope.SetInt64(0)
				next.countMaximumSlope.SetInt64(0)
				next.currentEmpty = true
				next.emptyRepeatBlocked = start.currentEmpty
				queue = append(queue, next)
			}
			continue
		}
		if candidate.index == 0 && candidate.currentEmpty && sequenceCandidateCountIsZero(candidate) {
			for _, empty := range sequenceCandidateOuterAllowedSubset(candidate, program) {
				empty.index = len(program.particles)
				empty.count.SetInt64(0)
				empty.countMaximum.SetInt64(0)
				empty.countMinimumSlope.SetInt64(0)
				empty.countMaximumSlope.SetInt64(0)
				empty.currentEmpty = false
				empty.emptyRepeatBlocked = false
				queue = append(queue, empty)
			}
		}
		particle := program.particles[candidate.index]
		for _, skipped := range sequenceCandidateOuterWhereAtLeast(candidate, candidate.countMaximumSlope, candidate.countMaximum, particle.occurrences.minimum.finite.integerCopy()) {
			next := skipped.clone()
			next.index++
			next.count.SetInt64(0)
			next.countMaximum.SetInt64(0)
			next.countMinimumSlope.SetInt64(0)
			next.countMaximumSlope.SetInt64(0)
			if next.index == len(program.particles) && !next.currentEmpty {
				next.outer.Add(next.outer, big.NewInt(1))
				next.outerMaximum.Add(next.outerMaximum, big.NewInt(1))
			}
			queue = append(queue, next)
		}
	}
	return compactSequenceCandidates(result, len(program.particles))
}

func newInstanceSequenceCandidate(index int, count int64, outer *big.Int, currentEmpty, emptyRepeatBlocked bool) instanceSequenceCandidate {
	return instanceSequenceCandidate{
		outer:              new(big.Int).Set(outer),
		outerMaximum:       new(big.Int).Set(outer),
		index:              index,
		count:              big.NewInt(count),
		countMaximum:       big.NewInt(count),
		countMinimumSlope:  big.NewInt(0),
		countMaximumSlope:  big.NewInt(0),
		currentEmpty:       currentEmpty,
		emptyRepeatBlocked: emptyRepeatBlocked,
	}
}

func (candidate instanceSequenceCandidate) clone() instanceSequenceCandidate {
	return instanceSequenceCandidate{
		outer:              new(big.Int).Set(candidate.outer),
		outerMaximum:       new(big.Int).Set(candidate.outerMaximum),
		index:              candidate.index,
		count:              new(big.Int).Set(candidate.count),
		countMaximum:       new(big.Int).Set(candidate.countMaximum),
		countMinimumSlope:  new(big.Int).Set(candidate.countMinimumSlope),
		countMaximumSlope:  new(big.Int).Set(candidate.countMaximumSlope),
		currentEmpty:       candidate.currentEmpty,
		emptyRepeatBlocked: candidate.emptyRepeatBlocked,
	}
}

func sequenceCandidateContains(candidates []instanceSequenceCandidate, candidate instanceSequenceCandidate) bool {
	for _, existing := range candidates {
		if !sequenceCandidateSameState(existing, candidate) || existing.outer.Cmp(candidate.outer) > 0 || existing.outerMaximum.Cmp(candidate.outerMaximum) < 0 {
			continue
		}
		if sequenceCandidateCountContainsAt(existing, candidate, candidate.outer) &&
			sequenceCandidateCountContainsAt(existing, candidate, candidate.outerMaximum) {
			return true
		}
	}
	return false
}

func sequenceCandidateSameState(first, second instanceSequenceCandidate) bool {
	return first.index == second.index &&
		first.currentEmpty == second.currentEmpty &&
		first.emptyRepeatBlocked == second.emptyRepeatBlocked
}

func sequenceCandidateCountContainsAt(container, candidate instanceSequenceCandidate, outer *big.Int) bool {
	containerMinimum, containerMaximum := sequenceCandidateCountAt(container, outer)
	candidateMinimum, candidateMaximum := sequenceCandidateCountAt(candidate, outer)
	return containerMinimum.Cmp(candidateMinimum) <= 0 && containerMaximum.Cmp(candidateMaximum) >= 0
}

func sequenceCandidateCountAt(candidate instanceSequenceCandidate, outer *big.Int) (*big.Int, *big.Int) {
	minimum := new(big.Int).Mul(candidate.countMinimumSlope, outer)
	minimum.Add(minimum, candidate.count)
	maximum := new(big.Int).Mul(candidate.countMaximumSlope, outer)
	maximum.Add(maximum, candidate.countMaximum)
	return minimum, maximum
}

func sequenceCandidateCountIsZero(candidate instanceSequenceCandidate) bool {
	minimum, maximum := sequenceCandidateCountAt(candidate, candidate.outer)
	if minimum.Sign() != 0 || maximum.Sign() != 0 {
		return false
	}
	minimum, maximum = sequenceCandidateCountAt(candidate, candidate.outerMaximum)
	return minimum.Sign() == 0 && maximum.Sign() == 0
}

func sequenceCandidateOuterSubset(candidate instanceSequenceCandidate, minimum, maximum *big.Int) []instanceSequenceCandidate {
	lower := candidate.outer
	if minimum != nil && lower.Cmp(minimum) < 0 {
		lower = minimum
	}
	upper := candidate.outerMaximum
	if maximum != nil && upper.Cmp(maximum) > 0 {
		upper = maximum
	}
	if lower.Cmp(upper) > 0 {
		return nil
	}
	result := candidate.clone()
	result.outer.Set(lower)
	result.outerMaximum.Set(upper)
	return []instanceSequenceCandidate{result}
}

type sequenceCandidateLinearRelation uint8

const (
	sequenceCandidateLinearAtMost sequenceCandidateLinearRelation = iota
	sequenceCandidateLinearAtLeast
)

func sequenceCandidateOuterWhereAtMost(candidate instanceSequenceCandidate, slope, intercept, limit *big.Int) []instanceSequenceCandidate {
	return sequenceCandidateOuterWhere(candidate, slope, intercept, sequenceCandidateLinearAtMost, limit)
}

func sequenceCandidateOuterWhereAtLeast(candidate instanceSequenceCandidate, slope, intercept, limit *big.Int) []instanceSequenceCandidate {
	return sequenceCandidateOuterWhere(candidate, slope, intercept, sequenceCandidateLinearAtLeast, limit)
}

//nolint:gocognit // Keep signed affine interval clipping explicit.
func sequenceCandidateOuterWhere(candidate instanceSequenceCandidate, slope, intercept *big.Int, relation sequenceCandidateLinearRelation, limit *big.Int) []instanceSequenceCandidate {
	lower := candidate.outer
	upper := candidate.outerMaximum
	if slope.Sign() == 0 {
		comparison := intercept.Cmp(limit)
		if (relation == sequenceCandidateLinearAtMost && comparison > 0) ||
			(relation == sequenceCandidateLinearAtLeast && comparison < 0) {
			return nil
		}
		return []instanceSequenceCandidate{candidate.clone()}
	}
	difference := new(big.Int).Sub(limit, intercept)
	if relation == sequenceCandidateLinearAtMost {
		if slope.Sign() > 0 {
			upper = minBigInt(upper, floorBigIntQuotient(difference, slope))
		}
		if slope.Sign() < 0 {
			lower = maxBigInt(lower, ceilBigIntQuotient(difference, slope))
		}
	}
	if relation == sequenceCandidateLinearAtLeast {
		if slope.Sign() > 0 {
			lower = maxBigInt(lower, ceilBigIntQuotient(difference, slope))
		}
		if slope.Sign() < 0 {
			upper = minBigInt(upper, floorBigIntQuotient(difference, slope))
		}
	}
	return sequenceCandidateOuterSubset(candidate, lower, upper)
}

func floorBigIntQuotient(numerator, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 && remainder.Sign() != denominator.Sign() {
		quotient.Sub(quotient, big.NewInt(1))
	}
	return quotient
}

func ceilBigIntQuotient(numerator, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 && remainder.Sign() == denominator.Sign() {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func minBigInt(first, second *big.Int) *big.Int {
	if first.Cmp(second) <= 0 {
		return new(big.Int).Set(first)
	}
	return new(big.Int).Set(second)
}

func maxBigInt(first, second *big.Int) *big.Int {
	if first.Cmp(second) >= 0 {
		return new(big.Int).Set(first)
	}
	return new(big.Int).Set(second)
}

func sequenceCandidateCountRelationEqual(first, second instanceSequenceCandidate) bool {
	return first.countMinimumSlope.Cmp(second.countMinimumSlope) == 0 &&
		first.countMaximumSlope.Cmp(second.countMaximumSlope) == 0 &&
		first.count.Cmp(second.count) == 0 &&
		first.countMaximum.Cmp(second.countMaximum) == 0
}

func sequenceCandidateOuterRangesEqual(first, second instanceSequenceCandidate) bool {
	return first.outer.Cmp(second.outer) == 0 && first.outerMaximum.Cmp(second.outerMaximum) == 0
}

func sequenceCandidateIntegerIntervalsTouch(firstMinimum, firstMaximum, secondMinimum, secondMaximum *big.Int) bool {
	firstAfter := new(big.Int).Add(firstMaximum, big.NewInt(1))
	if firstAfter.Cmp(secondMinimum) < 0 {
		return false
	}
	secondAfter := new(big.Int).Add(secondMaximum, big.NewInt(1))
	return secondAfter.Cmp(firstMinimum) >= 0
}

func sequenceCandidateCountIntervalsTouchAt(first, second instanceSequenceCandidate, outer *big.Int) bool {
	firstMinimum, firstMaximum := sequenceCandidateCountAt(first, outer)
	secondMinimum, secondMaximum := sequenceCandidateCountAt(second, outer)
	return sequenceCandidateIntegerIntervalsTouch(firstMinimum, firstMaximum, secondMinimum, secondMaximum)
}

func sequenceCandidateMerge(first, second instanceSequenceCandidate) (instanceSequenceCandidate, bool) {
	if !sequenceCandidateSameState(first, second) {
		return instanceSequenceCandidate{}, false
	}
	if sequenceCandidateOuterRangesEqual(first, second) &&
		first.countMinimumSlope.Cmp(second.countMinimumSlope) == 0 &&
		first.countMaximumSlope.Cmp(second.countMaximumSlope) == 0 &&
		sequenceCandidateCountIntervalsTouchAt(first, second, first.outer) {
		merged := first.clone()
		merged.count = minBigInt(first.count, second.count)
		merged.countMaximum = maxBigInt(first.countMaximum, second.countMaximum)
		return merged, true
	}
	if sequenceCandidateCountRelationEqual(first, second) &&
		sequenceCandidateIntegerIntervalsTouch(first.outer, first.outerMaximum, second.outer, second.outerMaximum) {
		merged := first.clone()
		merged.outer = minBigInt(first.outer, second.outer)
		merged.outerMaximum = maxBigInt(first.outerMaximum, second.outerMaximum)
		return merged, true
	}
	return instanceSequenceCandidate{}, false
}

func appendSequenceCandidateRegion(regions []instanceSequenceCandidate, candidate instanceSequenceCandidate) []instanceSequenceCandidate {
	for index := 0; index < len(regions); index++ {
		existing := regions[index]
		if sequenceCandidateContains([]instanceSequenceCandidate{existing}, candidate) {
			return regions
		}
		if sequenceCandidateContains([]instanceSequenceCandidate{candidate}, existing) {
			regions = append(regions[:index], regions[index+1:]...)
			index--
			continue
		}
		merged, ok := sequenceCandidateMerge(existing, candidate)
		if !ok {
			continue
		}
		regions = append(regions[:index], regions[index+1:]...)
		candidate = merged
		index = -1
	}
	return append(regions, candidate.clone())
}

type sequenceCandidateLine struct {
	slope     *big.Int
	intercept *big.Int
}

func sequenceCandidateLines(candidate instanceSequenceCandidate) [2]sequenceCandidateLine {
	return [2]sequenceCandidateLine{
		{slope: candidate.countMinimumSlope, intercept: candidate.count},
		{slope: candidate.countMaximumSlope, intercept: candidate.countMaximum},
	}
}

func appendSequenceCandidateBreakpoint(breakpoints []*big.Int, breakpoint *big.Int) []*big.Int {
	for _, existing := range breakpoints {
		if existing.Cmp(breakpoint) == 0 {
			return breakpoints
		}
	}
	return append(breakpoints, new(big.Int).Set(breakpoint))
}

func sequenceCandidateLineTransition(first, second sequenceCandidateLine, offset int64) *big.Int {
	slope := new(big.Int).Sub(first.slope, second.slope)
	if slope.Sign() == 0 {
		return nil
	}
	limit := new(big.Int).Add(second.intercept, big.NewInt(offset))
	limit.Sub(limit, first.intercept)
	if slope.Sign() > 0 {
		transition := floorBigIntQuotient(limit, slope)
		return transition.Add(transition, big.NewInt(1))
	}
	return ceilBigIntQuotient(limit, slope)
}

func sequenceCandidateBreakpoints(regions []instanceSequenceCandidate) []*big.Int {
	breakpoints := make([]*big.Int, 0, len(regions)*2)
	lines := make([]sequenceCandidateLine, 0, len(regions)*2)
	for _, candidate := range regions {
		breakpoints = appendSequenceCandidateBreakpoint(breakpoints, candidate.outer)
		end := new(big.Int).Add(candidate.outerMaximum, big.NewInt(1))
		breakpoints = appendSequenceCandidateBreakpoint(breakpoints, end)
		candidateLines := sequenceCandidateLines(candidate)
		lines = append(lines, candidateLines[0], candidateLines[1])
	}
	for first := 0; first < len(lines); first++ {
		for second := first + 1; second < len(lines); second++ {
			for _, offset := range []int64{-1, 0, 1} {
				transition := sequenceCandidateLineTransition(lines[first], lines[second], offset)
				if transition == nil {
					continue
				}
				breakpoints = appendSequenceCandidateBreakpoint(breakpoints, transition)
			}
		}
	}
	sort.Slice(breakpoints, func(first, second int) bool {
		return breakpoints[first].Cmp(breakpoints[second]) < 0
	})
	return breakpoints
}

func sequenceCandidateActiveAt(candidate instanceSequenceCandidate, outer *big.Int) bool {
	return candidate.outer.Cmp(outer) <= 0 && candidate.outerMaximum.Cmp(outer) >= 0
}

func sequenceCandidateRegionForLines(source instanceSequenceCandidate, outerMinimum, outerMaximum *big.Int, minimum, maximum sequenceCandidateLine) instanceSequenceCandidate {
	result := source.clone()
	result.outer.Set(outerMinimum)
	result.outerMaximum.Set(outerMaximum)
	result.count.Set(minimum.intercept)
	result.countMaximum.Set(maximum.intercept)
	result.countMinimumSlope.Set(minimum.slope)
	result.countMaximumSlope.Set(maximum.slope)
	return result
}

func sortSequenceCandidateRegions(regions []instanceSequenceCandidate) {
	sort.SliceStable(regions, func(first, second int) bool {
		left, right := regions[first], regions[second]
		if comparison := left.outer.Cmp(right.outer); comparison != 0 {
			return comparison < 0
		}
		if comparison := left.outerMaximum.Cmp(right.outerMaximum); comparison != 0 {
			return comparison < 0
		}
		leftMinimum, leftMaximum := sequenceCandidateCountAt(left, left.outer)
		rightMinimum, rightMaximum := sequenceCandidateCountAt(right, right.outer)
		if comparison := leftMinimum.Cmp(rightMinimum); comparison != 0 {
			return comparison < 0
		}
		return leftMaximum.Cmp(rightMaximum) < 0
	})
}

func sequenceCandidateRegionFromRows(first, last instanceSequenceCandidate) instanceSequenceCandidate {
	firstMinimum, firstMaximum := sequenceCandidateCountAt(first, first.outer)
	lastMinimum, lastMaximum := sequenceCandidateCountAt(last, last.outer)
	outerDistance := new(big.Int).Sub(last.outer, first.outer)
	minimumSlope := new(big.Int).Sub(lastMinimum, firstMinimum)
	minimumSlope.Quo(minimumSlope, outerDistance)
	maximumSlope := new(big.Int).Sub(lastMaximum, firstMaximum)
	maximumSlope.Quo(maximumSlope, outerDistance)
	minimumIntercept := new(big.Int).Mul(minimumSlope, first.outer)
	minimumIntercept.Sub(firstMinimum, minimumIntercept)
	maximumIntercept := new(big.Int).Mul(maximumSlope, first.outer)
	maximumIntercept.Sub(firstMaximum, maximumIntercept)
	result := first.clone()
	result.outerMaximum.Set(last.outer)
	result.count.Set(minimumIntercept)
	result.countMaximum.Set(maximumIntercept)
	result.countMinimumSlope.Set(minimumSlope)
	result.countMaximumSlope.Set(maximumSlope)
	return result
}

func sequenceCandidateRowMatches(candidate, region instanceSequenceCandidate) bool {
	minimum, maximum := sequenceCandidateCountAt(candidate, candidate.outer)
	expectedMinimum, expectedMaximum := sequenceCandidateCountAt(region, candidate.outer)
	return minimum.Cmp(expectedMinimum) == 0 && maximum.Cmp(expectedMaximum) == 0
}

func compactSequenceCandidateRows(regions []instanceSequenceCandidate) []instanceSequenceCandidate {
	sortSequenceCandidateRegions(regions)
	result := make([]instanceSequenceCandidate, 0, len(regions))
	for index := 0; index < len(regions); {
		first := regions[index]
		if first.outer.Cmp(first.outerMaximum) != 0 || index+1 >= len(regions) {
			result = appendSequenceCandidateRegion(result, first)
			index++
			continue
		}
		nextOuter := new(big.Int).Add(first.outer, big.NewInt(1))
		second := regions[index+1]
		if second.outer.Cmp(nextOuter) != 0 || second.outer.Cmp(second.outerMaximum) != 0 {
			result = appendSequenceCandidateRegion(result, first)
			index++
			continue
		}
		region := sequenceCandidateRegionFromRows(first, second)
		end := index + 1
		for end+1 < len(regions) {
			previous := regions[end]
			candidate := regions[end+1]
			nextOuter = new(big.Int).Add(previous.outer, big.NewInt(1))
			if candidate.outer.Cmp(nextOuter) != 0 || candidate.outer.Cmp(candidate.outerMaximum) != 0 || !sequenceCandidateRowMatches(candidate, region) {
				break
			}
			end++
		}
		result = appendSequenceCandidateRegion(result, sequenceCandidateRegionFromRows(first, regions[end]))
		index = end + 1
	}
	return result
}

func sequenceCandidateLineIntervalTouch(first, second instanceSequenceCandidate, outer *big.Int) bool {
	firstMinimum, firstMaximum := sequenceCandidateCountAt(first, outer)
	secondMinimum, secondMaximum := sequenceCandidateCountAt(second, outer)
	return sequenceCandidateIntegerIntervalsTouch(firstMinimum, firstMaximum, secondMinimum, secondMaximum)
}

//nolint:gocognit // Keep bounded interval compaction explicit and deterministic.
func compactSequenceCandidateGroup(group []instanceSequenceCandidate) []instanceSequenceCandidate {
	breakpoints := sequenceCandidateBreakpoints(group)
	result := make([]instanceSequenceCandidate, 0, len(group))
	for index := 0; index+1 < len(breakpoints); index++ {
		outerMinimum := breakpoints[index]
		outerMaximum := new(big.Int).Sub(breakpoints[index+1], big.NewInt(1))
		if outerMinimum.Cmp(outerMaximum) > 0 {
			continue
		}
		active := make([]instanceSequenceCandidate, 0, len(group))
		for _, candidate := range group {
			if sequenceCandidateActiveAt(candidate, outerMinimum) {
				active = append(active, candidate)
			}
		}
		if len(active) == 0 {
			continue
		}
		sort.SliceStable(active, func(first, second int) bool {
			firstMinimum, firstMaximum := sequenceCandidateCountAt(active[first], outerMinimum)
			secondMinimum, secondMaximum := sequenceCandidateCountAt(active[second], outerMinimum)
			if comparison := firstMinimum.Cmp(secondMinimum); comparison != 0 {
				return comparison < 0
			}
			return firstMaximum.Cmp(secondMaximum) < 0
		})
		components := make([]instanceSequenceCandidate, 0, len(active))
		for _, candidate := range active {
			candidateLines := sequenceCandidateLines(candidate)
			current := sequenceCandidateRegionForLines(candidate, outerMinimum, outerMaximum, candidateLines[0], candidateLines[1])
			if len(components) == 0 {
				components = append(components, current)
				continue
			}
			last := &components[len(components)-1]
			if !sequenceCandidateLineIntervalTouch(*last, current, outerMinimum) {
				components = append(components, current)
				continue
			}
			lastMinimum, lastMaximum := sequenceCandidateCountAt(*last, outerMinimum)
			currentMinimum, currentMaximum := sequenceCandidateCountAt(current, outerMinimum)
			if currentMinimum.Cmp(lastMinimum) < 0 {
				last.countMinimumSlope.Set(current.countMinimumSlope)
				last.count.Set(current.count)
			}
			if currentMaximum.Cmp(lastMaximum) > 0 {
				last.countMaximumSlope.Set(current.countMaximumSlope)
				last.countMaximum.Set(current.countMaximum)
			}
		}
		for _, component := range components {
			result = appendSequenceCandidateRegion(result, component)
		}
	}
	return compactSequenceCandidateRows(result)
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
				result = append(result, compactSequenceCandidateGroup(group)...)
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
