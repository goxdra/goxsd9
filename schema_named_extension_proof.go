package goxsd9

import (
	"errors"
	"fmt"
)

const diagnosticSchemaNamedExtensionUPACode = "XSD3064"

var errSchemaNamedExtensionUPA = errors.New("named extension particle attribution is ambiguous")

type schemaNamedExtensionPosition struct {
	schemaGroupLeaf
	target ComponentID
}

type schemaNamedExtensionExpression struct {
	first    []int
	last     []int
	nullable bool
}

type schemaNamedExtensionProof struct {
	records   []schemaComponentRecord
	groups    []schemaModelGroupResult
	version   XSDVersion
	positions []schemaNamedExtensionPosition
	follow    [][]int
}

// validateSchemaNamedNestedExtensions checks only the newly admitted named
// extension shape after all component dependencies have completed.
//
//nolint:gocognit // Keep ordered owner and transition diagnostics in one pass.
func validateSchemaNamedNestedExtensions(records []schemaComponentRecord, complexes []schemaComplexTypeResult, groups []schemaModelGroupResult, version XSDVersion) error {
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
		proof := schemaNamedExtensionProof{records: records, groups: groups, version: version}
		expression, err := proof.expression(body.particle, Loc{})
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

func (proof *schemaNamedExtensionProof) expression(particle Particle, via Loc) (schemaNamedExtensionExpression, error) { //nolint:gocognit,funlen // The four resolved particle variants form one bounded grammar.
	var expression schemaNamedExtensionExpression
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return expression, newSchemaBridgeInvariant(particle.Loc(), "named extension local element has no facts")
		}
		expression = proof.leaf(schemaNamedExtensionPosition{schemaGroupLeaf: schemaGroupLeaf{
			name: term.Name(), declaredType: term.DeclaredType(), loc: schemaGroupLeafLoc(term.Loc(), via), local: true,
		}})
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
		}, target: term.TargetID()})
	case ModelGroupReferenceParticle:
		index := schemaGroupRecordByID(proof.records, term.TargetID())
		if index < 0 || index >= len(proof.groups) || !proof.groups[index].present || proof.groups[index].particle == nil {
			return expression, newSchemaBridgeInvariant(term.RefLoc(), "named extension group target is absent")
		}
		var err error
		expression, err = proof.expression(proof.groups[index].particle, schemaGroupLeafLoc(term.RefLoc(), via))
		if err != nil {
			return expression, err
		}
	case ChoiceParticle:
		for _, child := range term.Alternatives() {
			member, err := proof.expression(child, via)
			if err != nil {
				return expression, err
			}
			expression.first = appendUniquePositions(expression.first, member.first)
			expression.last = appendUniquePositions(expression.last, member.last)
			expression.nullable = expression.nullable || member.nullable
		}
	case SequenceParticle:
		expression.nullable = true
		for _, child := range term.Particles() {
			member, err := proof.expression(child, via)
			if err != nil {
				return expression, err
			}
			for _, previous := range expression.last {
				proof.follow[previous] = appendUniquePositions(proof.follow[previous], member.first)
			}
			if expression.nullable {
				expression.first = appendUniquePositions(expression.first, member.first)
			}
			if !member.nullable {
				expression.last = member.last
			}
			if member.nullable {
				expression.last = appendUniquePositions(expression.last, member.last)
			}
			expression.nullable = expression.nullable && member.nullable
		}
	default:
		return expression, newSchemaBridgeInvariant(particle.Loc(), "named extension has an unexpected resolved particle")
	}
	occurrences := particle.Occurrences()
	maximum := occurrences.Maximum()
	moreThanOne := maximum.IsUnbounded()
	if finite, ok := maximum.Finite(); ok && finite.Canonical() != "1" {
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

func (proof *schemaNamedExtensionProof) checkCompeting(positions []int) error {
	for later := range positions {
		for first := 0; first < later; first++ {
			a, b := proof.positions[positions[first]], proof.positions[positions[later]]
			if a.name != b.name && !schemaGroupSubstitutionMayMatch(proof.records, a.name, b.name) && !schemaGroupSubstitutionMayMatch(proof.records, b.name, a.name) {
				continue
			}
			if a.name != b.name {
				diagnostic := newSchemaSyntaxUnsupportedForVersion(b.loc, "named extension substitution-group attribution is not proved", proof.version)
				diagnostic.related = []Loc{a.loc}
				diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(proof.version)
				return diagnostic
			}
			return Diagnostic{
				class: FailureInvalid, code: diagnosticSchemaNamedExtensionUPACode,
				loc: b.loc, related: []Loc{a.loc},
				message: fmt.Sprintf("named extension elements %q compete for one input", b.name),
				specRef: schemaGroupNonAmbiguousSpecRef(proof.version), cause: errSchemaNamedExtensionUPA,
			}
		}
	}
	return nil
}
