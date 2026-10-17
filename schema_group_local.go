package goxsd9

import (
	"errors"
	"fmt"
)

var errSchemaInlineDirectGroupAmbiguous = errors.New("inline direct group has ambiguous repetition")

type schemaGroupLeaf struct {
	name         QName
	declaredType QName
	loc          Loc
	local        bool
	reference    bool
}

// validateSchemaModelGroupLocalOverlaps keeps newly admitted local declarations
// out of content models whose declaration consistency or attribution cannot be
// proved by the bounded group representation.
func validateSchemaModelGroupLocalOverlaps(records []schemaComponentRecord, groups []schemaModelGroupResult, version XSDVersion) error { //nolint:gocognit // Compare leaves in lexical group order, including referenced groups.
	hasLocal := make(map[ComponentID]bool)
	for index, record := range records {
		if record.modelGroup == nil || !groups[index].present || groups[index].particle == nil {
			continue
		}
		if !schemaGroupParticleHasLocal(groups[index].particle, records, groups, hasLocal) {
			continue
		}
		seen := make([]schemaGroupLeaf, 0)
		seenReferences := make(map[QName]bool)
		visit := func(leaf schemaGroupLeaf) error {
			if leaf.reference && seenReferences[leaf.name] {
				return nil
			}
			for _, first := range seen {
				if err := schemaGroupLeafConflict(first, leaf, records, version); err != nil {
					return err
				}
			}
			seen = append(seen, leaf)
			if leaf.reference {
				seenReferences[leaf.name] = true
			}
			return nil
		}
		if err := walkSchemaGroupLeaves(groups[index].particle, records, groups, Loc{}, hasLocal, make(map[ComponentID]bool), visit); err != nil {
			return err
		}
	}
	return nil
}

func schemaGroupLeafConflict(first, later schemaGroupLeaf, records []schemaComponentRecord, version XSDVersion) error {
	if !first.local && !later.local {
		return nil
	}
	if first.name == later.name {
		if !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
			return newSchemaGroupElementConsistencyDiagnostic(later, first, version)
		}
		return newSchemaGroupOverlapDiagnostic(later, first, version)
	}
	if first.local && later.reference && schemaGroupSubstitutionMayMatch(records, later.name, first.name) ||
		later.local && first.reference && schemaGroupSubstitutionMayMatch(records, first.name, later.name) {
		return newSchemaGroupOverlapDiagnostic(later, first, version)
	}
	return nil
}

func schemaGroupParticleHasLocal(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, cache map[ComponentID]bool) bool {
	switch term := particle.(type) {
	case ElementParticle:
		return true
	case ModelGroupReferenceParticle:
		id := term.TargetID()
		if present, checked := cache[id]; checked {
			return present
		}
		index := schemaGroupRecordByID(records, id)
		if index < 0 || !groups[index].present || groups[index].particle == nil {
			return false
		}
		cache[id] = false
		present := schemaGroupParticleHasLocal(groups[index].particle, records, groups, cache)
		cache[id] = present
		return present
	case ChoiceParticle:
		return schemaGroupTermsHaveLocal(term.Alternatives(), records, groups, cache)
	case SequenceParticle:
		return schemaGroupTermsHaveLocal(term.Particles(), records, groups, cache)
	}
	return false
}

func schemaGroupTermsHaveLocal(terms []Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, cache map[ComponentID]bool) bool {
	present := false
	for _, term := range terms {
		if schemaGroupParticleHasLocal(term, records, groups, cache) {
			present = true
		}
	}
	return present
}

func walkSchemaGroupLeaves(particle Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, via Loc, hasLocal, visitedRefOnly map[ComponentID]bool, visit func(schemaGroupLeaf) error) error { //nolint:gocognit // Each resolved particle variant contributes its ordered leaf projection.
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group local element has no facts")
		}
		return visit(schemaGroupLeaf{name: term.Name(), declaredType: term.DeclaredType(), loc: schemaGroupLeafLoc(term.Loc(), via), local: true})
	case ElementReferenceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group element reference has no facts")
		}
		index := schemaGroupRecordByID(records, term.TargetID())
		if index < 0 || records[index].element == nil {
			return newSchemaBridgeInvariant(term.RefLoc(), "model-group element reference target has no element input")
		}
		return visit(schemaGroupLeaf{name: term.Ref(), declaredType: records[index].element.declaredType, loc: schemaGroupLeafLoc(term.RefLoc(), via), reference: true})
	case ModelGroupReferenceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group reference has no facts")
		}
		id := term.TargetID()
		index := schemaGroupRecordByID(records, id)
		if index < 0 || !groups[index].present || groups[index].particle == nil {
			return newSchemaBridgeInvariant(term.RefLoc(), "model-group reference target has no particle")
		}
		if !hasLocal[id] && visitedRefOnly[id] {
			return nil
		}
		if !hasLocal[id] {
			visitedRefOnly[id] = true
		}
		return walkSchemaGroupLeaves(groups[index].particle, records, groups, term.RefLoc(), hasLocal, visitedRefOnly, visit)
	case ChoiceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group choice has no facts")
		}
		return walkSchemaGroupTerms(term.facts.alternatives, records, groups, via, hasLocal, visitedRefOnly, visit)
	case SequenceParticle:
		if term.facts == nil {
			return newSchemaBridgeInvariant(Loc{}, "model-group sequence has no facts")
		}
		return walkSchemaGroupTerms(term.facts.particles, records, groups, via, hasLocal, visitedRefOnly, visit)
	}
	return newSchemaBridgeInvariant(particle.Loc(), "model-group has an unknown resolved particle")
}

func walkSchemaGroupTerms(terms []Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, via Loc, hasLocal, visitedRefOnly map[ComponentID]bool, visit func(schemaGroupLeaf) error) error {
	for _, term := range terms {
		if err := walkSchemaGroupLeaves(term, records, groups, via, hasLocal, visitedRefOnly, visit); err != nil {
			return err
		}
	}
	return nil
}

func schemaGroupLeafLoc(declaration, via Loc) Loc {
	if !via.IsZero() {
		return via
	}
	return declaration
}

func schemaGroupRecordByID(records []schemaComponentRecord, id ComponentID) int {
	for index, record := range records {
		if record.id == id {
			return index
		}
	}
	return -1
}

func schemaGroupSubstitutionMayMatch(records []schemaComponentRecord, head, member QName) bool {
	pending := []QName{member}
	seen := make(map[QName]bool)
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
		for _, record := range records {
			if record.element == nil || record.name != current {
				continue
			}
			for _, affiliation := range record.element.substitutionGroup {
				pending = append(pending, affiliation.name)
			}
		}
	}
	return false
}

func newSchemaGroupElementConsistencyDiagnostic(later, first schemaGroupLeaf, version XSDVersion) Diagnostic {
	return Diagnostic{
		class: FailureInvalid, code: diagnosticSchemaGroupElementConsistencyCode,
		loc: later.loc, related: []Loc{first.loc},
		message: fmt.Sprintf("named model-group element %q has inconsistent declared types", later.name),
		specRef: schemaGroupElementConsistencySpecRef(version),
		cause:   errSchemaGroupElementConsistency,
	}
}

func newSchemaGroupOverlapDiagnostic(later, first schemaGroupLeaf, version XSDVersion) Diagnostic {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(later.loc, fmt.Sprintf("named model-group elements %q and %q require unimplemented attribution checks", first.name, later.name), version)
	diagnostic.related = []Loc{first.loc}
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
	return diagnostic
}

func schemaGroupElementConsistencySpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#cos-element-consistent"
	}
	return "xsd11-structures#cos-element-consistent"
}

func schemaGroupNonAmbiguousSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#cos-nonambig"
	}
	return "xsd11-structures#cos-nonambig"
}

// validateSchemaInlineDirectGroupProof checks attribution only for the newly
// admitted sole direct group term after all named groups have been resolved.
func validateSchemaInlineDirectGroupProof(records []schemaComponentRecord, byName map[QName][]int, visibleSources map[SourceID][]SourceID, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion) error { //nolint:gocognit // Narrow post-resolution boundary selects only the admitted owner and term.
	for _, record := range records {
		if record.element == nil || record.element.inlineComplexType == nil {
			continue
		}
		body, ok := record.element.inlineComplexType.body.(*schemaComplexTypeExtensionBodyInput)
		if !ok || body == nil {
			continue
		}
		sequence, ok := body.particle.(*schemaSequenceParticleInput)
		if !ok || sequence == nil || len(sequence.particles) != 1 {
			continue
		}
		group, ok := sequence.particles[0].(*schemaModelGroupReferenceParticleInput)
		if !ok || group == nil {
			continue
		}
		targetIndex, err := schemaModelGroupReferenceTarget(group, record, records, byName, visibleSources, version)
		if err != nil {
			return err
		}
		if targetIndex >= len(groups) || !groups[targetIndex].present {
			return newSchemaBridgeInvariant(group.loc, "direct group target has no resolved model")
		}
		if err := proveSchemaInlineDirectGroup(sequence, group, groups[targetIndex].particle, records, groups, elements, version); err != nil {
			return err
		}
	}
	return nil
}

func proveSchemaInlineDirectGroup(sequence *schemaSequenceParticleInput, input *schemaModelGroupReferenceParticleInput, target Particle, records []schemaComponentRecord, groups []schemaModelGroupResult, elements []schemaElementTypeResult, version XSDVersion) error {
	if target == nil {
		return nil
	}
	proof := schemaInlineGroupProof{
		input: input, records: records, groups: groups, elements: elements, version: version,
		indices: make(map[ComponentID]int), elementIndices: make(map[ComponentID]int), memo: make(map[ComponentID]schemaInlineGroupSummary), active: make(map[ComponentID]bool),
	}
	for index, record := range records {
		if record.modelGroup != nil {
			proof.indices[record.id] = index
		}
		if record.element != nil {
			proof.elementIndices[record.id] = index
		}
	}
	summary, err := proof.summarize(target)
	if err != nil {
		return err
	}
	if input.occurrences.mapsToParticle() && !input.occurrences.isDefault() && !summary.one {
		return proof.unsupported(target.Loc(), "repeated direct group requires unimplemented boundary attribution proof")
	}
	if input.occurrences.mapsToParticle() && sequence.occurrences.maximum.String() != "0" && sequence.occurrences.maximum.String() != "1" && !summary.one {
		return proof.unsupported(sequence.loc, "repeated outer sequence requires unimplemented boundary attribution proof")
	}
	return nil
}

// These summaries live only for this proof. Keeping one summary per named group
// bounds shared reference DAGs without changing the opaque public particles.
type schemaInlineGroupSummary struct {
	first    []schemaGroupLeaf
	all      []schemaGroupLeaf
	nullable bool
	one      bool
}

type schemaInlineGroupProof struct {
	input          *schemaModelGroupReferenceParticleInput
	records        []schemaComponentRecord
	groups         []schemaModelGroupResult
	elements       []schemaElementTypeResult
	version        XSDVersion
	indices        map[ComponentID]int
	elementIndices map[ComponentID]int
	memo           map[ComponentID]schemaInlineGroupSummary
	active         map[ComponentID]bool
	work           int
}

const schemaInlineGroupProofWorkLimit = 8192

func (proof *schemaInlineGroupProof) charge(loc Loc) error {
	proof.work++
	if proof.work <= schemaInlineGroupProofWorkLimit {
		return nil
	}
	return proof.unsupported(loc, "direct group attribution proof exceeds the supported work bound")
}

func (proof *schemaInlineGroupProof) unsupported(loc Loc, message string) error {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(proof.input.reference.loc, message, proof.version)
	if !loc.IsZero() && loc != proof.input.reference.loc {
		diagnostic.related = []Loc{loc}
	}
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(proof.version)
	return diagnostic
}

func (proof *schemaInlineGroupProof) summarize(particle Particle) (schemaInlineGroupSummary, error) {
	if err := proof.charge(particle.Loc()); err != nil {
		return schemaInlineGroupSummary{}, err
	}
	switch term := particle.(type) {
	case ElementParticle:
		if term.facts == nil {
			return schemaInlineGroupSummary{}, newSchemaBridgeInvariant(Loc{}, "model-group local element has no facts")
		}
		leaf := schemaGroupLeaf{name: term.Name(), declaredType: term.DeclaredType(), loc: term.Loc(), local: true}
		return schemaInlineGroupSummary{first: []schemaGroupLeaf{leaf}, all: []schemaGroupLeaf{leaf}, nullable: term.Occurrences().Minimum().Sign() == 0, one: term.Occurrences().IsDefault()}, nil
	case ElementReferenceParticle:
		if term.facts == nil {
			return schemaInlineGroupSummary{}, newSchemaBridgeInvariant(Loc{}, "model-group element reference has no facts")
		}
		index := schemaGroupRecordByID(proof.records, term.TargetID())
		if index < 0 || proof.records[index].element == nil {
			return schemaInlineGroupSummary{}, newSchemaBridgeInvariant(term.RefLoc(), "model-group element reference target has no element input")
		}
		leaf := schemaGroupLeaf{name: term.Ref(), declaredType: proof.records[index].element.declaredType, loc: term.RefLoc(), reference: true}
		return schemaInlineGroupSummary{first: []schemaGroupLeaf{leaf}, all: []schemaGroupLeaf{leaf}, nullable: term.Occurrences().Minimum().Sign() == 0, one: term.Occurrences().IsDefault()}, nil
	case ModelGroupReferenceParticle:
		return proof.summarizeReference(term)
	case ChoiceParticle:
		return proof.summarizeChoice(term)
	case SequenceParticle:
		return proof.summarizeSequence(term)
	}
	return schemaInlineGroupSummary{}, newSchemaBridgeInvariant(particle.Loc(), "model-group has an unknown resolved particle")
}

func (proof *schemaInlineGroupProof) summarizeReference(term ModelGroupReferenceParticle) (schemaInlineGroupSummary, error) {
	index, ok := proof.indices[term.TargetID()]
	if !ok || !proof.groups[index].present || proof.groups[index].particle == nil {
		return schemaInlineGroupSummary{}, newSchemaBridgeInvariant(term.RefLoc(), "model-group reference target has no particle")
	}
	summary, cached := proof.memo[term.TargetID()]
	if !cached {
		if proof.active[term.TargetID()] {
			return schemaInlineGroupSummary{}, newSchemaBridgeInvariant(term.RefLoc(), "resolved model-group graph contains a cycle")
		}
		proof.active[term.TargetID()] = true
		var err error
		summary, err = proof.summarize(proof.groups[index].particle)
		delete(proof.active, term.TargetID())
		if err != nil {
			return schemaInlineGroupSummary{}, err
		}
		proof.memo[term.TargetID()] = summary
	}
	if !term.Occurrences().IsDefault() && !summary.one {
		return schemaInlineGroupSummary{}, proof.unsupported(term.RefLoc(), "repeated model-group reference requires unimplemented boundary attribution proof")
	}
	first, err := proof.leavesAt(summary.first, term.RefLoc())
	if err != nil {
		return schemaInlineGroupSummary{}, err
	}
	all, err := proof.leavesAt(summary.all, term.RefLoc())
	if err != nil {
		return schemaInlineGroupSummary{}, err
	}
	return schemaInlineGroupSummary{
		first: first, all: all,
		nullable: term.Occurrences().Minimum().Sign() == 0 || summary.nullable,
		one:      term.Occurrences().IsDefault() && summary.one,
	}, nil
}

func (proof *schemaInlineGroupProof) leavesAt(leaves []schemaGroupLeaf, loc Loc) ([]schemaGroupLeaf, error) {
	if len(leaves) > schemaInlineGroupProofWorkLimit-proof.work {
		return nil, proof.unsupported(loc, "direct group attribution proof exceeds the supported work bound")
	}
	proof.work += len(leaves)
	copyLeaves := make([]schemaGroupLeaf, len(leaves))
	copy(copyLeaves, leaves)
	for index := range copyLeaves {
		copyLeaves[index].loc = loc
	}
	return copyLeaves, nil
}

func (proof *schemaInlineGroupProof) summarizeChoice(term ChoiceParticle) (schemaInlineGroupSummary, error) { //nolint:gocognit // Ordered alternative checks distinguish proven FIRST competition from consistency.
	summary := schemaInlineGroupSummary{one: len(term.Alternatives()) > 0}
	for _, alternative := range term.Alternatives() {
		child, err := proof.summarize(alternative)
		if err != nil {
			return schemaInlineGroupSummary{}, err
		}
		for _, later := range child.first {
			for _, first := range summary.first {
				if err := proof.charge(later.loc); err != nil {
					return schemaInlineGroupSummary{}, err
				}
				if first.name == later.name {
					return schemaInlineGroupSummary{}, Diagnostic{
						class: FailureInvalid, code: "XSD3064", loc: later.loc, related: []Loc{first.loc, proof.input.reference.loc},
						message: "choice alternatives compete for the same element",
						specRef: schemaGroupNonAmbiguousSpecRef(proof.version), cause: errSchemaInlineDirectGroupAmbiguous,
					}
				}
				overlap, memberLoc, err := proof.matchOverlap(first, later)
				if err != nil {
					return schemaInlineGroupSummary{}, err
				}
				if overlap == schemaInlineMatchProven {
					return schemaInlineGroupSummary{}, proof.ambiguousChoice(later, first, memberLoc)
				}
				if overlap == schemaInlineMatchUncertain {
					return schemaInlineGroupSummary{}, newSchemaInlineDirectGroupAttributionDiagnostic(proof.input, first, later, proof.version)
				}
			}
		}
		summary.first = append(summary.first, child.first...)
		if err := proof.mergeAll(&summary, child.all, false); err != nil {
			return schemaInlineGroupSummary{}, err
		}
		summary.nullable = summary.nullable || child.nullable
		summary.one = summary.one && child.one
	}
	if !term.Occurrences().IsDefault() && !summary.one {
		return schemaInlineGroupSummary{}, proof.unsupported(term.Loc(), "repeated choice requires unimplemented boundary attribution proof")
	}
	summary.nullable = summary.nullable || term.Occurrences().Minimum().Sign() == 0
	summary.one = summary.one && term.Occurrences().IsDefault()
	return summary, nil
}

func (proof *schemaInlineGroupProof) summarizeSequence(term SequenceParticle) (schemaInlineGroupSummary, error) {
	summary := schemaInlineGroupSummary{nullable: true, one: len(term.Particles()) == 1}
	for _, childParticle := range term.Particles() {
		child, err := proof.summarize(childParticle)
		if err != nil {
			return schemaInlineGroupSummary{}, err
		}
		if summary.nullable {
			summary.first = append(summary.first, child.first...)
		}
		if err := proof.mergeAll(&summary, child.all, true); err != nil {
			return schemaInlineGroupSummary{}, err
		}
		summary.nullable = summary.nullable && child.nullable
		summary.one = summary.one && child.one
	}
	if !term.Occurrences().IsDefault() && !summary.one {
		return schemaInlineGroupSummary{}, proof.unsupported(term.Loc(), "repeated sequence requires unimplemented boundary attribution proof")
	}
	summary.nullable = summary.nullable || term.Occurrences().Minimum().Sign() == 0
	summary.one = summary.one && term.Occurrences().IsDefault()
	return summary, nil
}

func (proof *schemaInlineGroupProof) mergeAll(summary *schemaInlineGroupSummary, leaves []schemaGroupLeaf, sequence bool) error { //nolint:gocognit // Preserve the distinct consistency and attribution exits while merging ordered leaves.
	for _, later := range leaves {
		matched := false
		for _, first := range summary.all {
			if err := proof.charge(later.loc); err != nil {
				return err
			}
			if first.name == later.name {
				if !first.declaredType.IsZero() && !later.declaredType.IsZero() && first.declaredType != later.declaredType {
					return newSchemaGroupElementConsistencyDiagnostic(later, first, proof.version)
				}
				if first.declaredType.IsZero() || later.declaredType.IsZero() {
					return newSchemaInlineDirectGroupAttributionDiagnostic(proof.input, first, later, proof.version)
				}
				if sequence {
					return newSchemaInlineDirectGroupAttributionDiagnostic(proof.input, first, later, proof.version)
				}
				matched = true
				continue
			}
			if sequence {
				overlap, _, err := proof.matchOverlap(first, later)
				if err != nil {
					return err
				}
				if overlap != schemaInlineMatchNone {
					return newSchemaInlineDirectGroupAttributionDiagnostic(proof.input, first, later, proof.version)
				}
			}
		}
		if !matched {
			summary.all = append(summary.all, later)
		}
	}
	return nil
}

type schemaInlineMatch uint8

const (
	schemaInlineMatchNone schemaInlineMatch = iota
	schemaInlineMatchUncertain
	schemaInlineMatchProven
)

func (proof *schemaInlineGroupProof) ambiguousChoice(later, first schemaGroupLeaf, member Loc) Diagnostic {
	related := []Loc{first.loc, proof.input.reference.loc}
	if !member.IsZero() {
		related = append(related, member)
	}
	return Diagnostic{
		class: FailureInvalid, code: "XSD3064", loc: later.loc, related: related,
		message: "choice alternatives compete for the same element",
		specRef: schemaGroupNonAmbiguousSpecRef(proof.version), cause: errSchemaInlineDirectGroupAmbiguous,
	}
}

// matchOverlap scans resolved concrete declarations in discovery order. An
// affiliation with unproved effective matching remains an explicit boundary.
func (proof *schemaInlineGroupProof) matchOverlap(first, later schemaGroupLeaf) (schemaInlineMatch, Loc, error) { //nolint:gocognit // Compare concrete resolved members in declaration order with one proof budget.
	if len(proof.elements) != len(proof.records) {
		return schemaInlineMatchNone, Loc{}, newSchemaBridgeInvariant(proof.input.loc, "inline group proof has incomplete element results")
	}
	var uncertain Loc
	for index, record := range proof.records {
		if err := proof.charge(record.loc); err != nil {
			return schemaInlineMatchNone, Loc{}, err
		}
		if record.element == nil || (!first.local && !later.local && proof.elements[index].abstract) {
			continue
		}
		firstMatch, err := proof.matchLeaf(index, first)
		if err != nil {
			return schemaInlineMatchNone, Loc{}, err
		}
		if firstMatch == schemaInlineMatchNone {
			continue
		}
		laterMatch, err := proof.matchLeaf(index, later)
		if err != nil {
			return schemaInlineMatchNone, Loc{}, err
		}
		if laterMatch == schemaInlineMatchNone {
			continue
		}
		if first.local || later.local {
			return schemaInlineMatchUncertain, record.loc, nil
		}
		if firstMatch == schemaInlineMatchProven && laterMatch == schemaInlineMatchProven {
			return schemaInlineMatchProven, record.loc, nil
		}
		if uncertain.IsZero() {
			uncertain = record.loc
		}
	}
	if !uncertain.IsZero() {
		return schemaInlineMatchUncertain, uncertain, nil
	}
	return schemaInlineMatchNone, Loc{}, nil
}

func (proof *schemaInlineGroupProof) matchLeaf(index int, leaf schemaGroupLeaf) (schemaInlineMatch, error) { //nolint:gocognit // Bounded affiliation traversal distinguishes blocked, certain, and uncertain paths.
	if leaf.local {
		if proof.records[index].name == leaf.name {
			return schemaInlineMatchProven, nil
		}
		return schemaInlineMatchNone, nil
	}
	type path struct {
		index     int
		uncertain bool
	}
	pending := []path{{index: index}}
	result := schemaInlineMatchNone
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if err := proof.charge(proof.records[current.index].loc); err != nil {
			return schemaInlineMatchNone, err
		}
		if proof.records[current.index].name == leaf.name {
			if !current.uncertain {
				return schemaInlineMatchProven, nil
			}
			result = schemaInlineMatchUncertain
			continue
		}
		affiliations := proof.elements[current.index].substitutionGroup
		for affiliationIndex := len(affiliations) - 1; affiliationIndex >= 0; affiliationIndex-- {
			affiliation := affiliations[affiliationIndex]
			if err := proof.charge(affiliation.loc); err != nil {
				return schemaInlineMatchNone, err
			}
			target, ok := proof.elementIndices[affiliation.targetID]
			if !ok {
				return schemaInlineMatchNone, newSchemaBridgeInvariant(affiliation.loc, "resolved substitution target is missing")
			}
			if proof.elements[target].block.set&schemaBlockSubstitution != 0 {
				continue
			}
			uncertain := current.uncertain || proof.elements[current.index].declaredType != proof.elements[target].declaredType || proof.elements[target].block.set != 0
			pending = append(pending, path{index: target, uncertain: uncertain})
		}
	}
	return result, nil
}

func newSchemaInlineDirectGroupAttributionDiagnostic(input *schemaModelGroupReferenceParticleInput, first, later schemaGroupLeaf, version XSDVersion) Diagnostic {
	diagnostic := newSchemaSyntaxUnsupportedForVersion(input.reference.loc, "direct group target requires unimplemented particle attribution proof", version)
	diagnostic.related = []Loc{first.loc, later.loc}
	diagnostic.specRef = schemaGroupNonAmbiguousSpecRef(version)
	return diagnostic
}
