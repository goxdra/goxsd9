package goxsd9

import (
	"errors"
	"fmt"
)

const diagnosticSchemaNamedExtensionUPACode = "XSD3064"

const (
	schemaNamedExtensionMaxFixedCopies    = 8
	schemaNamedExtensionMaxProofPositions = 256
)

var errSchemaNamedExtensionUPA = errors.New("named extension particle attribution is ambiguous")

type schemaNamedExtensionPosition struct {
	schemaGroupLeaf
	target     ComponentID
	originPath []schemaNamedExtensionOriginStep
	termLoc    Loc
}

type schemaNamedExtensionOriginStep struct {
	index int
	loc   Loc
}

type schemaNamedExtensionExpression struct {
	first    []int
	last     []int
	nullable bool
}

type schemaNamedExtensionProof struct {
	records        []schemaComponentRecord
	groups         []schemaModelGroupResult
	elements       []schemaElementTypeResult
	version        XSDVersion
	positions      []schemaNamedExtensionPosition
	follow         [][]int
	uncertainFixed bool
}

// validateSchemaNamedNestedExtensions checks only the newly admitted named
// extension shape after all component dependencies have completed.
//
//nolint:gocognit // Keep ordered owner and transition diagnostics in one pass.
func validateSchemaNamedNestedExtensions(records []schemaComponentRecord, complexes []schemaComplexTypeResult, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion) error {
	for index, record := range records {
		if record.complexType == nil || !schemaNamedExtensionHasNestedTerm(record.complexType) {
			continue
		}
		body, ok := complexes[index].body.(*schemaComplexTypeExtensionBodyResult)
		if !ok {
			return newSchemaBridgeInvariant(record.loc, "named nested extension has no completed extension body")
		}
		if body.particle == nil {
			continue
		}
		proof := schemaNamedExtensionProof{records: records, groups: groups, elements: elements, version: version}
		expression, err := proof.expression(body.particle, Loc{}, nil)
		if err != nil {
			return err
		}
		if err := proof.elementConsistency(); err != nil {
			return err
		}
		if err := proof.checkCompeting(expression.first); err != nil {
			return err
		}
		for _, successors := range proof.follow {
			if err := proof.checkCompeting(successors); err != nil {
				return err
			}
		}
	}
	return nil
}

func schemaNamedExtensionHasNestedTerm(input *schemaComplexTypeInput) bool {
	if input == nil {
		return false
	}
	body, ok := input.body.(*schemaComplexTypeExtensionBodyInput)
	if !ok || body == nil {
		return false
	}
	sequence, ok := body.particle.(*schemaSequenceParticleInput)
	if !ok || sequence == nil {
		return false
	}
	for _, term := range sequence.particles {
		switch term.(type) {
		case *schemaChoiceParticleInput, *schemaSequenceParticleInput:
			return true
		}
	}
	return false
}

func (proof *schemaNamedExtensionProof) expression(particle Particle, via Loc, path []schemaNamedExtensionOriginStep) (schemaNamedExtensionExpression, error) { //nolint:gocognit // Preserve the exact-count branch and conservative fallback together.
	occurrences := particle.Occurrences()
	maximum, finite := occurrences.Maximum().Finite()
	if finite && maximum.Compare(occurrences.Minimum()) == 0 && maximum.Sign() > 0 && maximum.Canonical() != "1" {
		if maximum.value.IsInt64() && maximum.value.Int64() <= schemaNamedExtensionMaxFixedCopies && len(proof.positions) < schemaNamedExtensionMaxProofPositions {
			var result schemaNamedExtensionExpression
			result.nullable = true
			for copyIndex := int64(0); copyIndex < maximum.value.Int64(); copyIndex++ {
				member, err := proof.expressionOnce(particle, via, path)
				if err != nil {
					return result, err
				}
				result = proof.sequence(result, member)
				if len(proof.positions) > schemaNamedExtensionMaxProofPositions {
					return result, proof.unproved(particle.Loc(), nil, "named extension fixed repetition exceeds the attribution proof budget")
				}
			}
			return result, nil
		}
		proof.uncertainFixed = true
	}
	expression, err := proof.expressionOnce(particle, via, path)
	if err != nil {
		return expression, err
	}
	moreThanOne := occurrences.Maximum().IsUnbounded()
	if finite && maximum.Canonical() != "1" {
		moreThanOne = true
	}
	if moreThanOne {
		for _, previous := range expression.last {
			proof.follow[previous] = appendUniquePositions(proof.follow[previous], expression.first)
		}
	}
	expression.nullable = expression.nullable || occurrences.Minimum().Sign() == 0
	return expression, nil
}

func (proof *schemaNamedExtensionProof) expressionOnce(particle Particle, via Loc, path []schemaNamedExtensionOriginStep) (schemaNamedExtensionExpression, error) { //nolint:gocognit // The four resolved particle variants form one bounded grammar.
	var expression schemaNamedExtensionExpression
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return expression, newSchemaBridgeInvariant(particle.Loc(), "named extension local element has no facts")
		}
		expression = proof.leaf(schemaNamedExtensionPosition{schemaGroupLeaf: schemaGroupLeaf{
			name: term.Name(), declaredType: term.DeclaredType(), loc: schemaGroupLeafLoc(term.Loc(), via), local: true,
		}, originPath: cloneSchemaNamedExtensionOriginPath(path), termLoc: term.Loc()})
	case ElementReferenceParticle:
		if term.facts == nil {
			return expression, newSchemaBridgeInvariant(particle.Loc(), "named extension element reference has no facts")
		}
		index := schemaGroupRecordByID(proof.records, term.TargetID())
		if index < 0 || proof.records[index].element == nil {
			return expression, newSchemaBridgeInvariant(term.RefLoc(), "named extension element target is absent")
		}
		expression = proof.leaf(schemaNamedExtensionPosition{schemaGroupLeaf: schemaGroupLeaf{
			name: term.Ref(), declaredType: proof.records[index].element.declaredType,
			loc: schemaGroupLeafLoc(term.RefLoc(), via), reference: true,
		}, target: term.TargetID(), originPath: cloneSchemaNamedExtensionOriginPath(path), termLoc: term.RefLoc()})
	case ModelGroupReferenceParticle:
		index := schemaGroupRecordByID(proof.records, term.TargetID())
		if index < 0 || index >= len(proof.groups) || !proof.groups[index].present || proof.groups[index].particle == nil {
			return expression, newSchemaBridgeInvariant(term.RefLoc(), "named extension group target is absent")
		}
		var err error
		expression, err = proof.expression(proof.groups[index].particle, schemaGroupLeafLoc(term.RefLoc(), via), path)
		if err != nil {
			return expression, err
		}
	case ChoiceParticle:
		for childIndex, child := range term.Alternatives() {
			member, err := proof.expression(child, via, appendSchemaNamedExtensionOriginStep(path, childIndex, child))
			if err != nil {
				return expression, err
			}
			expression.first = appendUniquePositions(expression.first, member.first)
			expression.last = appendUniquePositions(expression.last, member.last)
			expression.nullable = expression.nullable || member.nullable
		}
	case SequenceParticle:
		expression.nullable = true
		for childIndex, child := range term.Particles() {
			member, err := proof.expression(child, via, appendSchemaNamedExtensionOriginStep(path, childIndex, child))
			if err != nil {
				return expression, err
			}
			expression = proof.sequence(expression, member)
		}
	default:
		return expression, newSchemaBridgeInvariant(particle.Loc(), "named extension has an unexpected resolved particle")
	}
	return expression, nil
}

func cloneSchemaNamedExtensionOriginPath(path []schemaNamedExtensionOriginStep) []schemaNamedExtensionOriginStep {
	return append([]schemaNamedExtensionOriginStep(nil), path...)
}

func appendSchemaNamedExtensionOriginStep(path []schemaNamedExtensionOriginStep, index int, child Particle) []schemaNamedExtensionOriginStep {
	stepLoc := child.Loc()
	if reference, ok := child.(ModelGroupReferenceParticle); ok {
		stepLoc = reference.RefLoc()
	}
	if reference, ok := child.(ElementReferenceParticle); ok {
		stepLoc = reference.RefLoc()
	}
	next := make([]schemaNamedExtensionOriginStep, len(path), len(path)+1)
	copy(next, path)
	return append(next, schemaNamedExtensionOriginStep{index: index, loc: stepLoc})
}

func (proof *schemaNamedExtensionProof) sequence(left, right schemaNamedExtensionExpression) schemaNamedExtensionExpression {
	for _, previous := range left.last {
		proof.follow[previous] = appendUniquePositions(proof.follow[previous], right.first)
	}
	if left.nullable {
		left.first = appendUniquePositions(left.first, right.first)
	}
	if !right.nullable {
		left.last = right.last
	}
	if right.nullable {
		left.last = appendUniquePositions(left.last, right.last)
	}
	left.nullable = left.nullable && right.nullable
	return left
}

func (proof *schemaNamedExtensionProof) leaf(position schemaNamedExtensionPosition) schemaNamedExtensionExpression {
	index := len(proof.positions)
	proof.positions = append(proof.positions, position)
	proof.follow = append(proof.follow, nil)
	return schemaNamedExtensionExpression{first: []int{index}, last: []int{index}}
}

func appendUniquePositions(target, positions []int) []int {
	for _, position := range positions {
		found := false
		for _, existing := range target {
			if existing == position {
				found = true
				break
			}
		}
		if !found {
			target = append(target, position)
		}
	}
	return target
}

func (proof *schemaNamedExtensionProof) elementConsistency() error { //nolint:gocognit // Compare each source-ordered declaration pair once.
	for later := range proof.positions {
		for first := 0; first < later; first++ {
			a, b := proof.positions[first], proof.positions[later]
			if a.name != b.name || a.target == b.target && !a.target.IsZero() {
				continue
			}
			if !a.declaredType.IsZero() && !b.declaredType.IsZero() {
				if a.declaredType != b.declaredType {
					return newSchemaGroupElementConsistencyDiagnostic(b.schemaGroupLeaf, a.schemaGroupLeaf, proof.version)
				}
				continue
			}
			diagnostic := newSchemaSyntaxUnsupportedForVersion(b.loc, "named extension element consistency cannot be proved for an anonymous or untyped declaration", proof.version)
			diagnostic.related = []Loc{a.loc}
			diagnostic.specRef = schemaGroupElementConsistencySpecRef(proof.version)
			return diagnostic
		}
	}
	return nil
}

func (proof *schemaNamedExtensionProof) checkCompeting(positions []int) error { //nolint:gocognit // Keep source-ordered pair diagnostics in one place.
	for later := range positions {
		for first := 0; first < later; first++ {
			a, b := proof.positions[positions[first]], proof.positions[positions[later]]
			if sameSchemaNamedExtensionOrigin(a.originPath, b.originPath) {
				continue
			}
			if a.name != b.name && !proof.mayShareSubstitutionMember(a, b) {
				continue
			}
			related := schemaNamedExtensionCompetingRelated(a, b)
			if a.name != b.name {
				return proof.unproved(b.loc, related, "named extension substitution-group attribution is not proved")
			}
			if proof.uncertainFixed {
				return proof.unproved(b.loc, related, "named extension fixed repetition attribution is not proved")
			}
			return Diagnostic{
				class: FailureInvalid, code: diagnosticSchemaNamedExtensionUPACode,
				loc: b.loc, related: related,
				message: fmt.Sprintf("named extension elements %q compete for one input", b.name),
				specRef: schemaGroupNonAmbiguousSpecRef(proof.version), cause: errSchemaNamedExtensionUPA,
			}
		}
	}
	return nil
}

func sameSchemaNamedExtensionOrigin(a, b []schemaNamedExtensionOriginStep) bool {
	if len(a) != len(b) {
		return false
	}
	for index, step := range a {
		if step.index != b[index].index {
			return false
		}
	}
	return true
}

func schemaNamedExtensionCompetingRelated(a, b schemaNamedExtensionPosition) []Loc {
	if a.loc != b.loc {
		return []Loc{a.loc}
	}
	for index, step := range a.originPath {
		if index >= len(b.originPath) {
			break
		}
		if step.index == b.originPath[index].index {
			continue
		}
		if !step.loc.IsZero() && !b.originPath[index].loc.IsZero() && step.loc != b.originPath[index].loc {
			return []Loc{step.loc, b.originPath[index].loc}
		}
		break
	}
	if !a.termLoc.IsZero() && !b.termLoc.IsZero() {
		return []Loc{a.termLoc, b.termLoc}
	}
	return []Loc{a.loc}
}

func (proof *schemaNamedExtensionProof) unproved(loc Loc, related []Loc, message string) Diagnostic {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(loc, message, proof.version)
	diagnostic.related = related
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(proof.version)
	return diagnostic
}

func (proof *schemaNamedExtensionProof) mayShareSubstitutionMember(a, b schemaNamedExtensionPosition) bool { //nolint:gocognit // Distinguish global references from local declarations.
	if a.reference && b.reference {
		for index, record := range proof.records {
			if record.element == nil || index >= len(proof.elements) || !proof.elements[index].present {
				continue
			}
			if proof.substitutesFor(record.id, a.target) && proof.substitutesFor(record.id, b.target) {
				return true
			}
		}
		return false
	}
	if a.reference && b.local {
		return schemaGroupSubstitutionMayMatch(proof.records, a.name, b.name)
	}
	if b.reference && a.local {
		return schemaGroupSubstitutionMayMatch(proof.records, b.name, a.name)
	}
	return false
}

func (proof *schemaNamedExtensionProof) substitutesFor(member, head ComponentID) bool {
	pending := []ComponentID{member}
	seen := make(map[ComponentID]bool)
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if current == head {
			return true
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		index := schemaGroupRecordByID(proof.records, current)
		if index < 0 || index >= len(proof.elements) {
			continue
		}
		for _, affiliation := range proof.elements[index].substitutionGroup {
			pending = append(pending, affiliation.targetID)
		}
	}
	return false
}
