package goxsd9

import (
	"errors"
	"fmt"
	"math/big"
)

const (
	instanceAttributeXSD10SpecRef = "xsd10-structures#cvc-attribute"
	instanceAttributeXSD11SpecRef = "xsd11-structures#cvc-attribute"
)

var (
	errInstanceAttributeMissing    = errors.New("required local attribute is missing")
	errInstanceAttributeUnexpected = errors.New("unexpected local attribute")
	errInstanceAttributeContent    = errors.New("invalid attribute-bearing element content")
	errInstanceAttributeSequence   = errors.New("invalid attribute-bearing sequence")
)

type instanceAttributeUsePlan struct {
	name QName
	use  LocalAttributeUse
}

type instanceAttributeLeafPlan struct {
	schema     Schema
	name       QName
	loc        Loc
	definition ComplexTypeDefinition
	target     ElementDeclaration
	refLoc     Loc
	uses       []instanceAttributeUsePlan
	content    *instanceScalarType
}

func instanceAttributeRootPlanFor(schema Schema, declaration ElementDeclaration) (instanceAttributeLeafPlan, bool) {
	definition, ok := declaration.InlineComplexType()
	if !ok {
		id, hasID := declaration.TypeID()
		if !hasID {
			return instanceAttributeLeafPlan{}, false
		}
		component, found := schema.Lookup(id)
		if !found {
			return instanceAttributeLeafPlan{}, false
		}
		definition, ok = component.ComplexTypeDefinition()
	}
	if !ok {
		return instanceAttributeLeafPlan{}, false
	}
	return instanceAttributeLeafForDefinition(schema, declaration.Name(), declaration.Loc(), definition)
}

type instanceAttributeChildPlan struct {
	leaf        instanceAttributeLeafPlan
	occurrences ParticleOccurrenceRange
}

type instanceAttributeSequenceProgram struct {
	version     XSDVersion
	rootLoc     Loc
	sequenceLoc Loc
	children    []instanceAttributeChildPlan
}

// The on-demand plan selects sequences whose children have local precisionDecimal
// uses or precisionDecimal simpleContent. Other sequence consumers keep their gates.
//
//nolint:gocognit // Selection keeps ordered schema shape and consumer gates together.
func instanceAttributeSequenceProgramFor(schema Schema, declaration ElementDeclaration, loc Loc) (instanceAttributeSequenceProgram, bool, error) {
	definition, ok := declaration.InlineComplexType()
	if !ok {
		id, hasID := declaration.TypeID()
		if !hasID {
			return instanceAttributeSequenceProgram{}, false, nil
		}
		component, found := schema.Lookup(id)
		if !found || component.Kind() != ComponentKindComplexTypeDefinition {
			return instanceAttributeSequenceProgram{}, false, nil
		}
		definition, ok = component.ComplexTypeDefinition()
		if !ok {
			return instanceAttributeSequenceProgram{}, false, nil
		}
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		return instanceAttributeSequenceProgram{}, false, nil
	}
	program := instanceAttributeSequenceProgram{
		version: instanceSchemaValidationVersion(schema), rootLoc: definition.Loc(),
		sequenceLoc: sequence.Loc(),
	}
	selected := false
	for _, particle := range sequence.Particles() {
		leaf, childOK := instanceAttributeLeafForParticle(schema, particle)
		if !childOK {
			if selected {
				return program, true, newInstanceValidationUnsupported(particle.Loc(), "mixed attribute sequence is outside instance validation", []Loc{declaration.Loc(), definition.Loc()}, program.version, errInstanceSequenceParticle)
			}
			return instanceAttributeSequenceProgram{}, false, nil
		}
		if len(leaf.uses) > 0 || instanceAttributeContentIsPrecision(leaf) {
			selected = true
		}
		program.children = append(program.children, instanceAttributeChildPlan{leaf: leaf, occurrences: particle.Occurrences()})
	}
	if !selected {
		return instanceAttributeSequenceProgram{}, false, nil
	}
	if err := rejectUnsupportedAttributeSequenceRoot(definition, declaration, sequence, loc, program.version); err != nil {
		return program, true, err
	}
	for _, child := range program.children {
		if len(child.leaf.uses) == 0 && !instanceAttributeContentIsPrecision(child.leaf) {
			return program, true, newInstanceValidationUnsupported(child.leaf.loc, "mixed attribute sequence is outside instance validation", []Loc{declaration.Loc(), definition.Loc()}, program.version, errInstanceSequenceParticle)
		}
	}
	if !sequence.Occurrences().IsDefault() {
		return program, true, newInstanceValidationUnsupported(sequence.Loc(), "outer attribute sequence occurrences are outside instance validation", []Loc{declaration.Loc(), definition.Loc()}, program.version, errInstanceSequenceParticle)
	}
	return program, true, nil
}

func rejectUnsupportedAttributeSequenceRoot(definition ComplexTypeDefinition, declaration ElementDeclaration, sequence SequenceParticle, loc Loc, version XSDVersion) error {
	related := []Loc{declaration.Loc(), definition.Loc(), sequence.Loc()}
	if body := definition.extensionBody(); body != nil {
		related = appendInstanceRelated(related, body.complexContentLoc)
		related = appendInstanceRelated(related, body.extensionLoc)
		related = appendInstanceRelated(related, body.base.loc)
		return newInstanceValidationUnsupported(loc, "attribute sequence complex-content extension is outside instance validation", related, version, errInstanceComplexContentExtension)
	}
	if definition.facts == nil {
		return newInstanceValidationInternal(loc, "attribute sequence has incomplete complex type facts", related, errInstanceValidationInvariant)
	}
	if _, direct := definition.facts.body.(*schemaComplexTypeDirectBodyComponent); !direct {
		return newInstanceValidationUnsupported(loc, "attribute sequence complex type body is outside instance validation", related, version, errInstanceSequenceParticle)
	}
	uses := definition.AttributeUses()
	if len(uses) > 0 {
		for _, use := range uses {
			related = appendInstanceRelated(related, use.Loc())
		}
		return newInstanceValidationUnsupported(uses[0].Loc(), "root attribute uses are outside attribute sequence validation", related, version, errInstanceAttributes)
	}
	if wildcard, ok := definition.AnyAttribute(); ok {
		related = appendInstanceRelated(related, wildcard.Loc())
		return newInstanceValidationUnsupported(loc, "root attribute wildcard is outside attribute sequence validation", related, version, errInstanceAttributes)
	}
	if definition.IsAbstract() {
		return newInstanceAbstractComplexTypeUnsupported(definition, loc, related, version)
	}
	return nil
}

//nolint:gocognit // Both local and referenced target identities resolve in one phase.
func instanceAttributeLeafForParticle(schema Schema, particle Particle) (instanceAttributeLeafPlan, bool) {
	var name QName
	var definition ComplexTypeDefinition
	var targetDeclaration ElementDeclaration
	var refLoc Loc
	var ok bool
	switch typed := particle.(type) {
	case ElementReferenceParticle:
		name = typed.Name()
		refLoc = typed.RefLoc()
		target, found := schema.Lookup(typed.TargetID())
		if !found {
			return instanceAttributeLeafPlan{}, false
		}
		declaration, found := target.ElementDeclaration()
		if !found {
			return instanceAttributeLeafPlan{}, false
		}
		targetDeclaration = declaration
		definition, ok = declaration.InlineComplexType()
		if !ok {
			id, hasID := declaration.TypeID()
			if !hasID {
				return instanceAttributeLeafPlan{}, false
			}
			component, found := schema.Lookup(id)
			if !found {
				return instanceAttributeLeafPlan{}, false
			}
			definition, ok = component.ComplexTypeDefinition()
		}
	case ElementParticle:
		name = typed.Name()
		id, hasID := typed.TypeID()
		if !hasID {
			return instanceAttributeLeafPlan{}, false
		}
		target, found := schema.Lookup(id)
		if !found {
			return instanceAttributeLeafPlan{}, false
		}
		definition, ok = target.ComplexTypeDefinition()
	default:
		return instanceAttributeLeafPlan{}, false
	}
	if !ok {
		return instanceAttributeLeafPlan{}, false
	}
	leaf, selected := instanceAttributeLeafForDefinition(schema, name, particle.Loc(), definition)
	if !selected {
		return instanceAttributeLeafPlan{}, false
	}
	leaf.target = targetDeclaration
	leaf.refLoc = refLoc
	return leaf, true
}

//nolint:gocognit // Keep the bounded body gate, local uses, and simpleContent projection together.
func instanceAttributeLeafForDefinition(schema Schema, name QName, loc Loc, definition ComplexTypeDefinition) (instanceAttributeLeafPlan, bool) {
	if definition.facts == nil {
		return instanceAttributeLeafPlan{}, false
	}
	switch definition.facts.body.(type) {
	case *schemaComplexTypeDirectBodyComponent, *schemaComplexTypeAttributeOnlyBodyComponent, *schemaComplexTypeSimpleContentBodyComponent:
	default:
		return instanceAttributeLeafPlan{}, false
	}
	if definition.IsAbstract() {
		return instanceAttributeLeafPlan{}, false
	}
	if definition.Particle() != nil {
		sequence, isSequence := definition.Particle().(SequenceParticle)
		if !isSequence || len(sequence.Particles()) != 0 {
			return instanceAttributeLeafPlan{}, false
		}
	}
	if _, hasWildcard := definition.AnyAttribute(); hasWildcard {
		return instanceAttributeLeafPlan{}, false
	}
	uses := definition.AttributeUses()
	leaf := instanceAttributeLeafPlan{schema: schema, name: name, loc: loc, definition: definition}
	for _, raw := range uses {
		use, local := raw.(LocalAttributeUse)
		if !local || !instanceAttributeUseIsPrecisionDecimal(schema, use) {
			return instanceAttributeLeafPlan{}, false
		}
		leaf.uses = append(leaf.uses, instanceAttributeUsePlan{name: use.Name(), use: use})
	}
	if extension, hasContent := definition.SimpleContentExtension(); hasContent {
		scalar, supported := instanceAttributeContentScalar(schema, extension, []Loc{loc, definition.Loc(), extension.BaseLoc()}, instanceSchemaValidationVersion(schema))
		if !supported {
			return instanceAttributeLeafPlan{}, false
		}
		leaf.content = &scalar
	}
	if len(leaf.uses) == 0 && !instanceAttributeContentIsPrecision(leaf) {
		return instanceAttributeLeafPlan{}, false
	}
	return leaf, true
}

//nolint:gocognit // Built-in and named simpleContent bases share one scalar projection.
func instanceAttributeContentScalar(schema Schema, extension SimpleContentExtension, related []Loc, version XSDVersion) (instanceScalarType, bool) {
	reference, ok := extension.TypeReference()
	if !ok {
		return instanceScalarType{}, false
	}
	if reference.IsBuiltin() && extension.Base().Namespace() == xsdNamespaceURI {
		switch extension.Base().Local() {
		case "string":
			return instanceScalarType{value: instanceStringScalar{whiteSpace: *defaultStringWhiteSpaceFacet()}, version: version, related: related}, true
		case "precisionDecimal":
			facets, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{})
			if err != nil {
				return instanceScalarType{}, false
			}
			return instanceScalarType{value: instancePrecisionDecimalScalar{facets: facets}, version: version, related: related}, true
		}
	}
	if facets, ok := reference.facts.facets.(schemaPrecisionDecimalFacetVariant); ok {
		return instanceScalarType{value: instancePrecisionDecimalScalar{facets: facets.value}, version: version, related: appendInstanceRelated(related, reference.Loc())}, true
	}
	if id, hasID := extension.TypeID(); hasID {
		component, found := schema.Lookup(id)
		if found {
			definition, isSimple := component.SimpleTypeDefinition()
			if isSimple && definition.HasPrecisionDecimalFacets() {
				return instanceScalarType{value: instancePrecisionDecimalScalar{facets: definition.PrecisionDecimalFacets()}, version: version, related: appendInstanceRelated(related, definition.Loc())}, true
			}
		}
	}
	return instanceScalarType{}, false
}

func instanceAttributeContentIsPrecision(leaf instanceAttributeLeafPlan) bool {
	if leaf.content == nil {
		return false
	}
	_, ok := leaf.content.value.(instancePrecisionDecimalScalar)
	return ok
}

func instanceAttributeUseIsPrecisionDecimal(schema Schema, use LocalAttributeUse) bool {
	reference, ok := use.TypeReference()
	if !ok || reference.facts == nil {
		return false
	}
	if reference.IsBuiltin() {
		return reference.Name().Namespace() == xsdNamespaceURI && reference.Name().Local() == "precisionDecimal"
	}
	_, ok = reference.facts.facets.(schemaPrecisionDecimalFacetVariant)
	if ok {
		return true
	}
	id, hasID := use.TypeID()
	if !hasID {
		return false
	}
	component, found := schema.Lookup(id)
	if !found {
		return false
	}
	definition, isSimple := component.SimpleTypeDefinition()
	return isSimple && definition.HasPrecisionDecimalFacets()
}

//nolint:gocognit // Ordered structure is completed before the value pass.
func validateAttributeSequenceInstance(root *instanceElement, program instanceAttributeSequenceProgram) error {
	related := []Loc{program.rootLoc, program.sequenceLoc}
	for _, attr := range root.attrs {
		if attr.name.namespace == "http://www.w3.org/2001/XMLSchema-instance" && attr.name.local == "schemaLocation" {
			continue
		}
		return newInstanceAttributeInvalid(attr.loc, "unexpected root attribute", related, program.version, errInstanceAttributeUnexpected)
	}
	children := make([]*instanceElement, 0, len(root.children))
	for _, node := range root.children {
		child, isElement := node.(*instanceElement)
		if isElement {
			if child == nil {
				return newInstanceValidationInternal(root.loc, "nil attribute sequence child", related, errInstanceValidationInvariant)
			}
			children = append(children, child)
			continue
		}
		text, isText := node.(instanceText)
		if !isText {
			return newInstanceValidationInternal(root.loc, "unknown attribute sequence node", related, errInstanceValidationInvariant)
		}
		if !xmlWhitespace([]byte(text.data)) {
			return newInstanceValidationInvalid(InvalidInstanceSequenceCode, text.loc, "non-whitespace sequence text", related, instanceValidationSpecRef(program.version), errInstanceAttributeSequence)
		}
	}
	matched := make([]struct {
		child *instanceElement
		leaf  instanceAttributeLeafPlan
	}, 0, len(children))
	index := 0
	for _, planned := range program.children {
		count := 0
		for index < len(children) && instanceNameMatches(children[index].name, planned.leaf.name) {
			if maximum, finite := planned.occurrences.Maximum().Finite(); finite && integerFromCount(count).Compare(maximum) >= 0 {
				break
			}
			matched = append(matched, struct {
				child *instanceElement
				leaf  instanceAttributeLeafPlan
			}{children[index], planned.leaf})
			index++
			count++
		}
		if integerFromCount(count).Compare(planned.occurrences.Minimum()) < 0 {
			return newInstanceValidationInvalid(InvalidInstanceSequenceCode, root.loc, "missing required sequence child", appendInstanceRelated(relCopy(related), planned.leaf.loc), instanceValidationSpecRef(program.version), errInstanceAttributeSequence)
		}
	}
	if index < len(children) {
		return newInstanceValidationInvalid(InvalidInstanceSequenceCode, children[index].loc, "unexpected sequence child", related, instanceValidationSpecRef(program.version), errInstanceAttributeSequence)
	}
	for _, entry := range matched {
		if err := validateAttributeLeafStructure(entry.child, entry.leaf, program.version); err != nil {
			return err
		}
	}
	for _, entry := range matched {
		if err := validateAttributeLeafValues(entry.child, entry.leaf, program.version); err != nil {
			return err
		}
	}
	return nil
}

func integerFromCount(count int) StrictInteger {
	return StrictInteger{value: big.NewInt(int64(count))}
}

func instanceNameMatches(name syntaxName, expected QName) bool {
	return name.namespace == expected.Namespace() && name.local == expected.Local()
}

//nolint:gocognit // Required, unexpected, and content checks share one structural boundary.
func validateAttributeLeafStructure(child *instanceElement, leaf instanceAttributeLeafPlan, version XSDVersion) error {
	related := []Loc{leaf.loc, leaf.definition.Loc()}
	if leaf.target.facts != nil {
		related = appendInstanceRelated(related, leaf.refLoc)
		related = appendInstanceRelated(related, leaf.target.Loc())
		if factsErr := rejectUnsupportedInstanceElementFactsWithRelated(leaf.target, child.loc, related, version); factsErr != nil {
			return factsErr
		}
		if affiliations := leaf.target.SubstitutionGroupAffiliations(); len(affiliations) > 0 {
			for _, affiliationLoc := range leaf.target.SubstitutionGroupAffiliationLocations() {
				related = appendInstanceRelated(related, affiliationLoc)
			}
			return newInstanceValidationUnsupported(child.loc, "referenced attribute-bearing element has substitution affiliations outside validation", related, version, errInstanceElementSubstitution)
		}
	}
	for _, node := range child.children {
		if nested, isElement := node.(*instanceElement); isElement {
			if nested == nil {
				return newInstanceValidationInternal(child.loc, "nil attribute leaf child", related, errInstanceValidationInvariant)
			}
			return newInstanceAttributeInvalid(nested.loc, "attribute-bearing element has a child element", related, version, errInstanceAttributeContent)
		}
		text, isText := node.(instanceText)
		if !isText {
			return newInstanceValidationInternal(child.loc, "unknown attribute leaf node", related, errInstanceValidationInvariant)
		}
		if leaf.content == nil && text.data != "" {
			return newInstanceAttributeInvalid(text.loc, "attribute-bearing empty content has text", related, version, errInstanceAttributeContent)
		}
	}
	for _, attr := range child.attrs {
		found := false
		for _, use := range leaf.uses {
			if instanceNameMatches(attr.name, use.name) {
				found = true
				break
			}
		}
		if !found {
			return newInstanceAttributeInvalid(attr.loc, fmt.Sprintf("unexpected attribute %q", renderSyntaxName(attr.name)), related, version, errInstanceAttributeUnexpected)
		}
	}
	for _, use := range leaf.uses {
		if use.use.Use() != AttributeUseRequired {
			continue
		}
		found := false
		for _, attr := range child.attrs {
			if instanceNameMatches(attr.name, use.name) {
				found = true
				break
			}
		}
		if !found {
			return newInstanceAttributeInvalid(child.loc, fmt.Sprintf("required attribute %q is missing", use.name), appendInstanceRelated(relCopy(related), use.use.Loc()), version, errInstanceAttributeMissing)
		}
	}
	return nil
}

func validateAttributeLeafValues(child *instanceElement, leaf instanceAttributeLeafPlan, version XSDVersion) error {
	for _, attr := range child.attrs {
		for _, use := range leaf.uses {
			if !instanceNameMatches(attr.name, use.name) {
				continue
			}
			scalar, err := instancePrecisionAttributeScalar(use.use, leaf, version, attr.loc)
			if err != nil {
				return err
			}
			if err := validateScalarLexicalValue(attr.name, attr.value, attr.loc, scalar); err != nil {
				return err
			}
			break
		}
	}
	if leaf.content != nil {
		return validateScalarValue(child, *leaf.content)
	}
	return nil
}

func instancePrecisionAttributeScalar(use LocalAttributeUse, leaf instanceAttributeLeafPlan, version XSDVersion, loc Loc) (instanceScalarType, error) {
	related := []Loc{leaf.loc, leaf.definition.Loc(), use.Loc(), use.TypeLoc()}
	reference, ok := use.TypeReference()
	if !ok {
		return instanceScalarType{}, newInstanceValidationInternal(loc, "local attribute lost type reference", related, errInstanceValidationInvariant)
	}
	if reference.IsAnonymous() {
		definition, found := reference.AnonymousType()
		if !found || !definition.HasPrecisionDecimalFacets() {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "local inline precisionDecimal type is incomplete", related, errInstanceValidationInvariant)
		}
		return instanceScalarType{value: instancePrecisionDecimalScalar{facets: definition.PrecisionDecimalFacets()}, version: version, related: appendInstanceRelated(related, definition.Loc())}, nil
	}
	if reference.IsBuiltin() {
		return instanceBuiltInPrecisionDecimalScalarType(reference.Name(), related, loc, true)
	}
	if id, hasID := use.TypeID(); hasID {
		component, found := leaf.schema.Lookup(id)
		if !found {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "named attribute type is missing", related, errInstanceValidationInvariant)
		}
		definition, isSimple := component.SimpleTypeDefinition()
		if !isSimple || !definition.HasPrecisionDecimalFacets() {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "named precisionDecimal attribute type is incomplete", related, errInstanceValidationInvariant)
		}
		return instanceScalarType{value: instancePrecisionDecimalScalar{facets: definition.PrecisionDecimalFacets()}, version: version, related: appendInstanceRelated(related, definition.Loc())}, nil
	}
	facets, found := reference.facts.facets.(schemaPrecisionDecimalFacetVariant)
	if !found {
		return instanceScalarType{}, newInstanceValidationInternal(loc, "named precisionDecimal attribute type is incomplete", related, errInstanceValidationInvariant)
	}
	return instanceScalarType{value: instancePrecisionDecimalScalar{facets: facets.value}, version: version, related: appendInstanceRelated(related, reference.Loc())}, nil
}

func newInstanceAttributeInvalid(loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	specRef := instanceAttributeXSD11SpecRef
	if version == XSDVersion10 {
		specRef = instanceAttributeXSD10SpecRef
	}
	return newInstanceValidationInvalid(InvalidInstanceAttributeCode, loc, message, related, specRef, cause)
}
