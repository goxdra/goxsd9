package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// InvalidInstanceReaderCode identifies a missing instance reader at the
	// public validation boundary.
	InvalidInstanceReaderCode = "XML3007"
	// InvalidInstanceSourceCode identifies an empty caller-supplied instance
	// source identity.
	InvalidInstanceSourceCode = "XML3008"
	// InvalidInstanceSchemaCode identifies a zero or incomplete Schema value at
	// the public validation boundary.
	InvalidInstanceSchemaCode = "XSD4001"
	// UnknownInstanceSchemaRootCode identifies an instance root without a
	// matching global element declaration.
	UnknownInstanceSchemaRootCode = "XSD4002"
	// AmbiguousInstanceSchemaRootCode identifies an instance root with more
	// than one matching global element declaration.
	AmbiguousInstanceSchemaRootCode = "XSD4003"
	// UnsupportedInstanceValidationCode identifies semantic validation outside
	// the supported scalar element slice.
	UnsupportedInstanceValidationCode = "XSD4004"
	// InvalidInstanceChoiceCode identifies invalid direct-choice content in an
	// XML instance.
	InvalidInstanceChoiceCode = "XSD4005"
	// InvalidInstanceSequenceCode identifies invalid direct-sequence content in
	// an XML instance.
	InvalidInstanceSequenceCode = "XSD4006"
)

const (
	instanceValidationXSD10SpecRef = "xsd10-structures#cvc-elt"
	instanceValidationXSD11SpecRef = "xsd11-structures#cvc-elt"
	instanceIntegerXSD10SpecRef    = "xsd10-datatypes#integer"
	instanceIntegerXSD11SpecRef    = "xsd11-datatypes#integer"
	instanceDecimalXSD10SpecRef    = "xsd10-datatypes#decimal"
	instanceDecimalXSD11SpecRef    = "xsd11-datatypes#decimal"
	// Completed built-in element views do not retain their document version.
	// Compatibility validation uses the repository's XSD 1.1-compatible default.
	instanceBuiltInValidationVersion XSDVersion = XSDVersion11
	diagnosticInstanceValidationCode            = "GOXSD9034"
)

var (
	errInstanceReaderNil               = errors.New("instance reader is nil")
	errInstanceSourceIDEmpty           = errors.New("instance source ID is empty")
	errInstanceSchemaEmpty             = errors.New("instance schema is zero or incomplete")
	errInstanceUnknownSchemaRoot       = errors.New("instance root has no matching global element declaration")
	errInstanceAmbiguousSchemaRoot     = errors.New("instance root has ambiguous global element declarations")
	errInstanceNoDeclaredType          = errors.New("global element has no supported declared type")
	errInstanceUnsupportedType         = errors.New("global element type is outside scalar validation")
	errInstanceAttributes              = errors.New("instance attributes are outside scalar validation")
	errInstanceChildElements           = errors.New("instance child elements are outside scalar validation")
	errInstanceChoiceMissing           = errors.New("choice instance has no selected element")
	errInstanceChoiceUnknown           = errors.New("choice instance has an unknown element")
	errInstanceChoiceMultiple          = errors.New("choice instance has multiple elements")
	errInstanceChoiceText              = errors.New("choice instance has non-whitespace parent text")
	errInstanceChoiceNested            = errors.New("choice instance has nested element content")
	errInstanceChoiceParticle          = errors.New("choice type has an unsupported particle")
	errInstanceChoiceTarget            = errors.New("choice alternative has an unsupported target")
	errInstanceChoiceMixed             = errors.New("choice type mixes local declarations and element references")
	errInstanceSequenceParticle        = errors.New("sequence type has an unsupported particle")
	errInstanceSequenceTarget          = errors.New("sequence particle has an unsupported target")
	errInstanceSequenceMixed           = errors.New("sequence type mixes unsupported particle forms")
	errInstanceSequenceMissing         = errors.New("sequence instance is missing a required element")
	errInstanceSequenceUnexpected      = errors.New("sequence instance has an unexpected element")
	errInstanceSequenceText            = errors.New("sequence instance has non-whitespace parent text")
	errInstanceSequenceNested          = errors.New("sequence scalar element has nested content")
	errInstanceOpenAttrsType           = errors.New("openAttrs complex type is outside instance validation")
	errInstanceComplexContentExtension = errors.New("complex-content extension is outside instance validation")
	errInstanceElementFacts            = errors.New("global element abstract and nillable facts are outside instance validation")
	errInstanceLocalElementFacts       = errors.New("local element nillable facts are outside instance validation")
	errInstanceElementSubstitution     = errors.New("referenced global element substitution is outside instance validation")
	errInstanceValidationInvariant     = errors.New("scalar validation invariant is broken")
)

type instanceScalarType struct {
	value       instanceScalarValue
	enumeration instanceEnumerationFacet
	version     XSDVersion
	related     []Loc
}

type instanceEnumerationFacet interface {
	instanceEnumerationFacet()
}

type instanceIntegerEnumerationFacet struct {
	facets IntegerEnumerationFacets
}

func (instanceIntegerEnumerationFacet) instanceEnumerationFacet() {}

type instanceDecimalEnumerationFacet struct {
	facets DecimalEnumerationFacets
}

func (instanceDecimalEnumerationFacet) instanceEnumerationFacet() {}

type instanceScalarValue interface {
	instanceScalarValue()
}

type instanceDigitScalar struct {
	facets        DigitFacets
	integerBounds IntegerBoundFacets
	decimalBounds DecimalBoundFacets
}

func (instanceDigitScalar) instanceScalarValue() {}

type instanceBooleanScalar struct{}

func (instanceBooleanScalar) instanceScalarValue() {}

type instancePrecisionDecimalScalar struct {
	facets PrecisionDecimalFacets
}

func (instancePrecisionDecimalScalar) instanceScalarValue() {}

type instanceChoiceAlternative struct {
	name                QName
	loc                 Loc
	scalar              instanceScalarType
	substitutionMembers []instanceChoiceSubstitutionMember
}

type instanceChoiceSubstitutionMember struct {
	name    QName
	related []Loc
}

type instanceChoiceProgram struct {
	version      XSDVersion
	related      []Loc
	alternatives []instanceChoiceAlternative
}

// ValidateInstance consumes, drains, and closes reader exactly once, then
// validates one XML instance against schema. The supported semantic slice is
// a single global element whose declared type is built-in or named XSD
// boolean, integer, decimal, or precisionDecimal, or a named complex type with
// one direct scalar choice or sequence. Direct choices may use local scalar
// declarations, or default-occurrence references to global integer and decimal
// declarations. Direct sequences use local integer and decimal declarations.
// Comments and processing instructions are ignored by the decoder.
//
// Built-in element views do not retain a document version, so this entrypoint
// uses the repository's compatibility/default XSD 1.1-compatible datatype
// rules for built-in integer and decimal values. Boolean values use the
// selected graph-wide policy for their versioned datatype diagnostics. Named
// numeric types use the version retained by their completed effective facets;
// named boolean types use the selected graph-wide policy. A successful
// validation returns nil. Unsupported semantic structures return a
// registered xsd.instance.validation diagnostic.
func ValidateInstance(schema Schema, sourceID SourceID, reader io.ReadCloser) error {
	if reader == nil {
		return newDiagnostic(
			FailureInvalid,
			InvalidInstanceReaderCode,
			Loc{},
			"instance reader is nil",
			errInstanceReaderNil,
		)
	}

	var observer instanceDecoderObserver
	if sourceID != "" && schema.storage != nil {
		observer = newInstanceValidationObserver(schema)
	}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: sourceID, observer: observer})
	if err != nil {
		return err
	}
	if sourceID == "" {
		return newDiagnostic(
			FailureInvalid,
			InvalidInstanceSourceCode,
			Loc{},
			"instance source ID is empty",
			errInstanceSourceIDEmpty,
		)
	}
	if schema.storage == nil {
		return newDiagnostic(
			FailureInvalid,
			InvalidInstanceSchemaCode,
			Loc{},
			"instance schema is zero or incomplete",
			errInstanceSchemaEmpty,
		)
	}
	if document == nil || document.root == nil {
		if document != nil && document.streamed {
			return nil
		}
		return newInstanceValidationInternal(
			Loc{},
			"instance decoder returned no completed root",
			nil,
			errInstanceValidationInvariant,
		)
	}
	return validateScalarInstance(schema, document.root)
}

//nolint:gocognit // Keep root dispatch, target resolution, and scalar fallback together.
func validateScalarInstance(schema Schema, root *instanceElement) error {
	if root == nil {
		return newInstanceValidationInternal(
			Loc{},
			"instance validation received a nil root",
			nil,
			errInstanceValidationInvariant,
		)
	}
	rootName, err := NewQName(root.name.namespace, root.name.local)
	if err != nil {
		return newInstanceValidationInternal(
			root.loc,
			"instance root name could not be expanded",
			nil,
			err,
		)
	}
	declaration, err := instanceSchemaElement(schema, rootName, root.loc)
	if err != nil {
		return err
	}
	if factsErr := rejectUnsupportedInstanceElementFacts(schema, declaration, root.loc); factsErr != nil {
		return factsErr
	}
	if declaration.DeclaredType().Namespace() != xsdNamespaceURI {
		typeID, hasTypeID := declaration.TypeID()
		if !hasTypeID || typeID.IsZero() {
			return newInstanceValidationUnsupported(
				root.loc,
				fmt.Sprintf("global element type %q has no resolved named type", declaration.DeclaredType()),
				[]Loc{declaration.Loc()},
				instanceSchemaValidationVersion(schema),
				errInstanceNoDeclaredType,
			)
		}
		target, ok := schema.Lookup(typeID)
		if !ok {
			return newInstanceValidationInternal(
				root.loc,
				fmt.Sprintf("global element type ID %v is not in the completed schema", typeID),
				[]Loc{declaration.Loc()},
				errInstanceValidationInvariant,
			)
		}
		if target.Kind() == ComponentKindComplexTypeDefinition {
			definition, ok := target.ComplexTypeDefinition()
			if !ok {
				return newInstanceValidationInternal(
					root.loc,
					fmt.Sprintf("global element type ID %v has no complex type view", typeID),
					[]Loc{declaration.Loc(), target.Loc()},
					errInstanceValidationInvariant,
				)
			}
			program, programErr := instanceChoiceProgramFor(schema, declaration, definition, root.loc)
			if programErr != nil {
				return programErr
			}
			return validateChoiceInstance(root, program)
		}
	}
	scalar, err := instanceScalarTypeFor(schema, declaration, root.loc)
	if err != nil {
		return err
	}
	if err := validateScalarStructure(root, scalar); err != nil {
		return err
	}
	return validateScalarValue(root, scalar)
}

func rejectUnsupportedInstanceElementFacts(schema Schema, declaration ElementDeclaration, loc Loc) error {
	return rejectUnsupportedInstanceElementFactsWithRelated(
		declaration,
		loc,
		[]Loc{declaration.Loc()},
		instanceSchemaValidationVersion(schema),
	)
}

func rejectUnsupportedInstanceElementFactsWithRelated(declaration ElementDeclaration, loc Loc, related []Loc, version XSDVersion) error {
	if declaration.IsAbstract() {
		return newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("global element %q has abstract=true outside instance validation", declaration.Name()),
			related,
			version,
			errInstanceElementFacts,
		)
	}
	if declaration.IsNillable() {
		return newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("global element %q has nillable=true outside instance validation", declaration.Name()),
			related,
			version,
			errInstanceElementFacts,
		)
	}
	return nil
}

func instanceSchemaElement(schema Schema, rootName QName, loc Loc) (ElementDeclaration, error) {
	version := instanceSchemaValidationVersion(schema)
	matches := schema.FindKind(ComponentKindElementDeclaration, rootName)
	if len(matches) == 0 {
		return ElementDeclaration{}, newInstanceValidationInvalid(
			UnknownInstanceSchemaRootCode,
			loc,
			fmt.Sprintf("instance root %q has no matching global element declaration", rootName),
			nil,
			instanceValidationSpecRef(version),
			errInstanceUnknownSchemaRoot,
		)
	}
	if len(matches) > 1 {
		related := make([]Loc, 0, len(matches))
		for _, match := range matches {
			related = appendInstanceRelated(related, match.Loc())
		}
		return ElementDeclaration{}, newInstanceValidationInvalid(
			AmbiguousInstanceSchemaRootCode,
			loc,
			fmt.Sprintf("instance root %q matches more than one global element declaration", rootName),
			related,
			instanceValidationSpecRef(version),
			errInstanceAmbiguousSchemaRoot,
		)
	}

	component := matches[0]
	declaration, ok := component.ElementDeclaration()
	if !ok {
		return ElementDeclaration{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("global element %q has no supported declared type", component.Name()),
			[]Loc{component.Loc()},
			version,
			errInstanceNoDeclaredType,
		)
	}
	return declaration, nil
}

func instanceChoiceProgramFor(
	schema Schema,
	declaration ElementDeclaration,
	definition ComplexTypeDefinition,
	loc Loc,
) (instanceChoiceProgram, error) {
	version := instanceSchemaValidationVersion(schema)
	related := []Loc{declaration.Loc(), definition.Loc()}
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
		return instanceChoiceProgram{}, newInstanceValidationUnsupported(
			body.extensionLoc,
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
		return instanceChoiceProgram{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named complex type %q uses bounded openAttrs content outside instance validation", definition.Name()),
			related,
			version,
			errInstanceOpenAttrsType,
		)
	}
	choice, related, err := instanceChoiceParticleFor(definition, loc, related, version)
	if err != nil {
		return instanceChoiceProgram{}, err
	}
	if anyAttribute, ok := definition.AnyAttribute(); ok {
		related = appendInstanceRelated(related, anyAttribute.Loc())
		return instanceChoiceProgram{}, newInstanceValidationUnsupported(
			anyAttribute.Loc(),
			fmt.Sprintf("named complex type %q attribute wildcards are outside direct choice validation", definition.Name()),
			related,
			version,
			errInstanceAttributes,
		)
	}
	alternatives, related, err := instanceChoiceAlternativesFor(schema, declaration, definition, choice, related, loc, version)
	if err != nil {
		return instanceChoiceProgram{}, err
	}

	program := instanceChoiceProgram{
		version:      version,
		related:      related,
		alternatives: alternatives,
	}
	return program, nil
}

func instanceChoiceParticleFor(
	definition ComplexTypeDefinition,
	loc Loc,
	related []Loc,
	version XSDVersion,
) (ChoiceParticle, []Loc, error) {
	particle := definition.Particle()
	choice, ok := particle.(ChoiceParticle)
	if !ok {
		return ChoiceParticle{}, nil, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named complex type %q does not have a direct choice particle", definition.Name()),
			related,
			version,
			errInstanceChoiceParticle,
		)
	}
	if choice.facts == nil {
		return ChoiceParticle{}, nil, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("named complex type %q has an incomplete choice particle", definition.Name()),
			related,
			errInstanceValidationInvariant,
		)
	}
	related = appendInstanceRelated(relCopy(related), choice.Loc())
	choiceOccurrences := choice.Occurrences()
	if !choiceOccurrences.IsDefault() {
		return ChoiceParticle{}, nil, newInstanceValidationUnsupported(
			choice.Loc(),
			fmt.Sprintf("choice particle occurrence bounds %s are outside instance validation", choiceOccurrences),
			related,
			version,
			errInstanceChoiceParticle,
		)
	}
	return choice, related, nil
}

//nolint:gocognit // Keep the direct-choice shape gate and alternative order together.
func instanceChoiceAlternativesFor(
	schema Schema,
	declaration ElementDeclaration,
	definition ComplexTypeDefinition,
	choice ChoiceParticle,
	related []Loc,
	loc Loc,
	version XSDVersion,
) ([]instanceChoiceAlternative, []Loc, error) {
	particleAlternatives := choice.Alternatives()
	particles := make([]Particle, 0, len(particleAlternatives))
	hasReference := false
	hasLocal := false
	for _, particleAlternative := range particleAlternatives {
		if reference, referenceOK := elementReferenceParticleValue(particleAlternative); referenceOK {
			if err := instanceChoiceReferenceParticleFor(reference, choice.Loc(), related, version); err != nil {
				return nil, nil, err
			}
			particles = append(particles, reference)
			hasReference = true
			related = appendInstanceRelated(related, reference.Loc())
			continue
		}
		element, err := instanceChoiceElementFor(choice, particleAlternative, related, version)
		if err != nil {
			return nil, nil, err
		}
		particles = append(particles, element)
		hasLocal = true
		related = appendInstanceRelated(related, element.Loc())
	}
	if hasReference && hasLocal {
		return nil, nil, newInstanceValidationUnsupported(
			loc,
			"direct choice mixes local element declarations and element references",
			related,
			version,
			errInstanceChoiceMixed,
		)
	}

	alternatives := make([]instanceChoiceAlternative, 0, len(particles))
	for _, particle := range particles {
		if reference, referenceOK := elementReferenceParticleValue(particle); referenceOK {
			alternative, err := instanceChoiceReferenceAlternativeFor(
				schema,
				declaration,
				definition,
				choice,
				reference,
				loc,
				version,
			)
			if err != nil {
				return nil, nil, err
			}
			alternatives = append(alternatives, alternative)
			continue
		}
		element, elementOK := elementParticleValue(particle)
		if !elementOK {
			return nil, nil, newInstanceValidationInternal(
				loc,
				"direct choice alternative changed during validation planning",
				related,
				errInstanceValidationInvariant,
			)
		}
		alternative, err := instanceChoiceAlternativeFor(schema, declaration, definition, choice, element, version)
		if err != nil {
			return nil, nil, err
		}
		alternatives = append(alternatives, alternative)
	}
	return alternatives, related, nil
}

func instanceChoiceReferenceParticleFor(reference ElementReferenceParticle, choiceLoc Loc, related []Loc, version XSDVersion) error {
	referenceLoc := reference.Loc()
	if referenceLoc.IsZero() {
		referenceLoc = choiceLoc
	}
	if reference.facts == nil {
		return newInstanceValidationInternal(
			referenceLoc,
			"direct choice element reference has incomplete particle facts",
			related,
			errInstanceValidationInvariant,
		)
	}
	referenceRelated := appendInstanceRelated(relCopy(related), reference.Loc())
	if reference.Name().IsZero() {
		return newInstanceValidationInternal(
			referenceLoc,
			"direct choice element reference has no expanded name",
			referenceRelated,
			errInstanceValidationInvariant,
		)
	}
	occurrences := reference.Occurrences()
	if occurrences.IsDefault() {
		return nil
	}
	occurrenceLoc := reference.RefLoc()
	if occurrenceLoc.IsZero() {
		occurrenceLoc = referenceLoc
	}
	return newInstanceValidationUnsupported(
		occurrenceLoc,
		fmt.Sprintf("choice reference occurrence bounds %s are outside instance validation", occurrences),
		referenceRelated,
		version,
		errInstanceChoiceParticle,
	)
}

func instanceChoiceElementFor(
	choice ChoiceParticle,
	particleAlternative Particle,
	related []Loc,
	version XSDVersion,
) (ElementParticle, error) {
	element, ok := elementParticleValue(particleAlternative)
	if !ok || element.facts == nil {
		return ElementParticle{}, newInstanceValidationUnsupported(
			choice.Loc(),
			"direct choice contains a non-element alternative",
			related,
			version,
			errInstanceChoiceParticle,
		)
	}
	if element.IsNillable() {
		return ElementParticle{}, newInstanceValidationUnsupported(
			element.Loc(),
			fmt.Sprintf("local element %q has nillable=true outside instance validation", element.Name()),
			appendInstanceRelated(relCopy(related), element.Loc()),
			version,
			errInstanceLocalElementFacts,
		)
	}
	elementOccurrences := element.Occurrences()
	if !elementOccurrences.IsDefault() {
		return ElementParticle{}, newInstanceValidationUnsupported(
			element.Loc(),
			fmt.Sprintf("choice alternative occurrence bounds %s are outside instance validation", elementOccurrences),
			appendInstanceRelated(relCopy(related), element.Loc()),
			version,
			errInstanceChoiceParticle,
		)
	}
	if element.Name().IsZero() {
		return ElementParticle{}, newInstanceValidationInternal(
			element.Loc(),
			"direct choice element has no expanded name",
			appendInstanceRelated(relCopy(related), element.Loc()),
			errInstanceValidationInvariant,
		)
	}
	return element, nil
}

func instanceChoiceReferenceAlternativeFor(
	schema Schema,
	declaration ElementDeclaration,
	definition ComplexTypeDefinition,
	choice ChoiceParticle,
	reference ElementReferenceParticle,
	loc Loc,
	version XSDVersion,
) (instanceChoiceAlternative, error) {
	related := []Loc{declaration.Loc(), definition.Loc(), choice.Loc(), reference.Loc()}
	targetDeclaration, targetRelated, err := instanceChoiceReferenceTargetFor(schema, reference, related, loc, version)
	if err != nil {
		return instanceChoiceAlternative{}, err
	}
	scalar, err := instanceChoiceReferenceScalarFor(schema, targetDeclaration, targetRelated, loc, version)
	if err != nil {
		return instanceChoiceAlternative{}, err
	}
	substitutionMembers, err := instanceChoiceReferenceSubstitutionMembersFor(schema, targetDeclaration, targetRelated, loc)
	if err != nil {
		return instanceChoiceAlternative{}, err
	}
	return instanceChoiceAlternative{
		name:                reference.Name(),
		loc:                 reference.Loc(),
		scalar:              scalar,
		substitutionMembers: substitutionMembers,
	}, nil
}

func instanceChoiceReferenceTargetFor(
	schema Schema,
	reference ElementReferenceParticle,
	related []Loc,
	loc Loc,
	version XSDVersion,
) (ElementDeclaration, []Loc, error) {
	if reference.facts == nil {
		return ElementDeclaration{}, nil, newInstanceValidationInternal(
			loc,
			"direct choice element reference has incomplete particle facts",
			related,
			errInstanceValidationInvariant,
		)
	}
	if reference.TargetID().IsZero() {
		return ElementDeclaration{}, nil, newInstanceValidationInternal(
			loc,
			"direct choice element reference has no target component ID",
			related,
			errInstanceValidationInvariant,
		)
	}
	target, targetOK := schema.Lookup(reference.TargetID())
	if !targetOK {
		return ElementDeclaration{}, nil, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("direct choice element reference target ID %v is not in the completed schema", reference.TargetID()),
			related,
			errInstanceValidationInvariant,
		)
	}
	targetRelated := appendInstanceRelated(relCopy(related), target.Loc())
	if target.Kind() != ComponentKindElementDeclaration {
		return ElementDeclaration{}, nil, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("direct choice element reference target ID %v has component kind %q, want an element declaration", reference.TargetID(), target.Kind()),
			targetRelated,
			errInstanceValidationInvariant,
		)
	}
	if target.Name() != reference.Name() {
		return ElementDeclaration{}, nil, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("direct choice element reference target %v does not match referenced name %v", target.Name(), reference.Name()),
			targetRelated,
			errInstanceValidationInvariant,
		)
	}
	targetDeclaration, ok := target.ElementDeclaration()
	if !ok {
		return ElementDeclaration{}, nil, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("referenced global element %q has no supported declared type", target.Name()),
			targetRelated,
			version,
			errInstanceNoDeclaredType,
		)
	}
	if targetDeclaration.Name() != reference.Name() {
		return ElementDeclaration{}, nil, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("direct choice element reference target declaration %v does not match referenced name %v", targetDeclaration.Name(), reference.Name()),
			targetRelated,
			errInstanceValidationInvariant,
		)
	}
	return targetDeclaration, targetRelated, nil
}

func instanceChoiceReferenceSubstitutionMembersFor(
	schema Schema,
	head ElementDeclaration,
	related []Loc,
	loc Loc,
) ([]instanceChoiceSubstitutionMember, error) {
	if instanceChoiceSubstitutionDisallowed(head) {
		return nil, nil
	}
	members := make([]instanceChoiceSubstitutionMember, 0)
	for _, component := range schema.Components() {
		if component.Kind() != ComponentKindElementDeclaration {
			continue
		}
		member, ok := component.ElementDeclaration()
		if !ok || member.ID() == head.ID() || member.IsAbstract() {
			continue
		}
		affiliations, found, err := instanceChoiceSubstitutionPathFor(
			schema,
			member,
			head.ID(),
			make(map[ComponentID]struct{}),
			related,
			loc,
		)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		memberRelated := relCopy(related)
		for _, affiliationLoc := range affiliations {
			memberRelated = appendInstanceRelated(memberRelated, affiliationLoc)
		}
		members = append(members, instanceChoiceSubstitutionMember{
			name:    member.Name(),
			related: memberRelated,
		})
	}
	return members, nil
}

//nolint:gocognit // Keep ID lookup, invariant checks, blocking, and ordered path traversal together.
func instanceChoiceSubstitutionPathFor(
	schema Schema,
	member ElementDeclaration,
	headID ComponentID,
	visited map[ComponentID]struct{},
	related []Loc,
	loc Loc,
) ([]Loc, bool, error) {
	if member.ID() == headID {
		return nil, true, nil
	}
	if _, seen := visited[member.ID()]; seen {
		return nil, false, nil
	}
	visited[member.ID()] = struct{}{}
	affiliationIDs := member.SubstitutionGroupAffiliations()
	affiliationLocs := member.SubstitutionGroupAffiliationLocations()
	if len(affiliationIDs) != len(affiliationLocs) {
		return nil, false, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("global element %q has mismatched substitution-group facts", member.Name()),
			related,
			errInstanceValidationInvariant,
		)
	}
	for index, affiliationID := range affiliationIDs {
		affiliationLoc := affiliationLocs[index]
		parentComponent, ok := schema.Lookup(affiliationID)
		if !ok {
			return nil, false, newInstanceValidationInternal(
				loc,
				fmt.Sprintf("global element %q substitution-group target ID %v is not in the completed schema", member.Name(), affiliationID),
				appendInstanceRelated(relCopy(related), affiliationLoc),
				errInstanceValidationInvariant,
			)
		}
		if parentComponent.Kind() != ComponentKindElementDeclaration {
			return nil, false, newInstanceValidationInternal(
				loc,
				fmt.Sprintf("global element %q substitution-group target ID %v has component kind %q, want an element declaration", member.Name(), affiliationID, parentComponent.Kind()),
				appendInstanceRelated(relCopy(related), affiliationLoc),
				errInstanceValidationInvariant,
			)
		}
		parent, ok := parentComponent.ElementDeclaration()
		if !ok {
			return nil, false, newInstanceValidationInternal(
				loc,
				fmt.Sprintf("global element %q substitution-group target ID %v has no element declaration view", member.Name(), affiliationID),
				appendInstanceRelated(relCopy(related), affiliationLoc),
				errInstanceValidationInvariant,
			)
		}
		if parent.ID() == headID {
			return []Loc{affiliationLoc}, true, nil
		}
		path, found, err := instanceChoiceSubstitutionPathFor(schema, parent, headID, visited, related, loc)
		if err != nil {
			return nil, false, err
		}
		if found {
			return append([]Loc{affiliationLoc}, path...), true, nil
		}
	}
	return nil, false, nil
}

func instanceChoiceSubstitutionDisallowed(declaration ElementDeclaration) bool {
	for _, method := range declaration.DisallowedSubstitutions() {
		if method == "substitution" {
			return true
		}
	}
	return false
}

func instanceChoiceReferenceScalarFor(
	schema Schema,
	targetDeclaration ElementDeclaration,
	related []Loc,
	loc Loc,
	version XSDVersion,
) (instanceScalarType, error) {
	if factsErr := rejectUnsupportedInstanceElementFactsWithRelated(targetDeclaration, loc, related, version); factsErr != nil {
		return instanceScalarType{}, factsErr
	}
	if affiliations := targetDeclaration.SubstitutionGroupAffiliations(); len(affiliations) != 0 {
		substitutionRelated := relCopy(related)
		for _, affiliationLoc := range targetDeclaration.SubstitutionGroupAffiliationLocations() {
			substitutionRelated = appendInstanceRelated(substitutionRelated, affiliationLoc)
		}
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("referenced global element %q uses substitution-group affiliations outside direct reference validation", targetDeclaration.Name()),
			substitutionRelated,
			version,
			errInstanceElementSubstitution,
		)
	}
	typeReference, hasTypeReference := targetDeclaration.TypeReference()
	if !hasTypeReference {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("referenced global element %q has no resolved type reference", targetDeclaration.Name()),
			related,
			version,
			errInstanceNoDeclaredType,
		)
	}
	if typeReference.Kind() == SimpleTypeReferenceAnonymous {
		anonymousRelated := appendInstanceRelated(relCopy(related), typeReference.Loc())
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("referenced global element %q uses an anonymous simple type outside direct reference validation", targetDeclaration.Name()),
			anonymousRelated,
			version,
			errInstanceChoiceTarget,
		)
	}
	if typeReference.Kind() != SimpleTypeReferenceBuiltin && typeReference.Kind() != SimpleTypeReferenceNamed {
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("referenced global element %q has an unknown type-reference kind %q", targetDeclaration.Name(), typeReference.Kind()),
			appendInstanceRelated(relCopy(related), typeReference.Loc()),
			errInstanceValidationInvariant,
		)
	}
	if typeReference.Name() != targetDeclaration.DeclaredType() {
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("referenced global element %q has inconsistent declared type reference %v/%v", targetDeclaration.Name(), targetDeclaration.DeclaredType(), typeReference.Name()),
			related,
			errInstanceValidationInvariant,
		)
	}
	typeID, hasTypeID := targetDeclaration.TypeID()
	return instanceScalarTypeForTarget(
		schema,
		targetDeclaration.DeclaredType(),
		typeID,
		hasTypeID,
		related,
		loc,
		version,
		false,
		false,
		version,
	)
}

func instanceChoiceAlternativeFor(
	schema Schema,
	declaration ElementDeclaration,
	definition ComplexTypeDefinition,
	choice ChoiceParticle,
	element ElementParticle,
	version XSDVersion,
) (instanceChoiceAlternative, error) {
	alternativeRelated := []Loc{declaration.Loc(), definition.Loc(), choice.Loc(), element.Loc()}
	typeID, hasTypeID := element.TypeID()
	scalar, err := instanceScalarTypeForTarget(
		schema,
		element.DeclaredType(),
		typeID,
		hasTypeID,
		alternativeRelated,
		element.Loc(),
		version,
		false,
		true,
		version,
	)
	if err != nil {
		return instanceChoiceAlternative{}, err
	}
	return instanceChoiceAlternative{
		name:   element.Name(),
		loc:    element.Loc(),
		scalar: scalar,
	}, nil
}

//nolint:gocognit,funlen // Keep direct-child selection and selected scalar validation ordered.
func validateChoiceInstance(root *instanceElement, program instanceChoiceProgram) error {
	for _, attribute := range root.attrs {
		return newInstanceValidationUnsupported(
			attribute.loc,
			fmt.Sprintf("attribute %q is not supported for direct choice validation", renderSyntaxName(attribute.name)),
			program.related,
			program.version,
			errInstanceAttributes,
		)
	}

	selectedIndex := -1
	var selected *instanceElement
	for _, node := range root.children {
		text, isText := node.(instanceText)
		if isText {
			if xmlWhitespace([]byte(text.data)) {
				continue
			}
			return newInstanceChoiceInvalid(
				text.loc,
				"direct choice parent contains non-whitespace character data",
				program.related,
				program.version,
				errInstanceChoiceText,
			)
		}
		child, isElement := node.(*instanceElement)
		if !isElement {
			return newInstanceValidationInternal(
				root.loc,
				"instance tree contains an unknown direct choice node",
				program.related,
				errInstanceValidationInvariant,
			)
		}
		if child == nil {
			return newInstanceValidationInternal(
				root.loc,
				"instance tree contains a nil direct choice child",
				program.related,
				errInstanceValidationInvariant,
			)
		}
		if selected != nil {
			message := "direct choice contains more than one child element"
			if child.name == selected.name {
				message = fmt.Sprintf("direct choice repeats child element %q", renderSyntaxName(child.name))
			}
			return newInstanceChoiceInvalid(child.loc, message, program.related, program.version, errInstanceChoiceMultiple)
		}
		selected = child
		childName, err := NewQName(child.name.namespace, child.name.local)
		if err != nil {
			return newInstanceValidationInternal(
				child.loc,
				"direct choice child name could not be expanded",
				program.related,
				err,
			)
		}
		for _, alternative := range program.alternatives {
			for _, member := range alternative.substitutionMembers {
				if member.name != childName {
					continue
				}
				return newInstanceValidationUnsupported(
					child.loc,
					fmt.Sprintf("direct choice child %q uses a substitution-group member outside instance validation", renderSyntaxName(child.name)),
					member.related,
					program.version,
					errInstanceElementSubstitution,
				)
			}
		}
		for index := range program.alternatives {
			if program.alternatives[index].name == childName {
				selectedIndex = index
				break
			}
		}
		if selectedIndex < 0 {
			return newInstanceChoiceInvalid(
				child.loc,
				fmt.Sprintf("direct choice child %q does not match an alternative", renderSyntaxName(child.name)),
				program.related,
				program.version,
				errInstanceChoiceUnknown,
			)
		}
	}
	if selected == nil {
		return newInstanceChoiceInvalid(
			root.loc,
			"direct choice requires one child element",
			program.related,
			program.version,
			errInstanceChoiceMissing,
		)
	}
	alternative := program.alternatives[selectedIndex]
	for _, attribute := range selected.attrs {
		return newInstanceValidationUnsupported(
			attribute.loc,
			fmt.Sprintf("attribute %q is not supported for direct choice scalar validation", renderSyntaxName(attribute.name)),
			alternative.scalar.related,
			alternative.scalar.version,
			errInstanceAttributes,
		)
	}
	for _, node := range selected.children {
		nested, isElement := node.(*instanceElement)
		if isElement {
			if nested == nil {
				return newInstanceValidationInternal(
					selected.loc,
					"instance tree contains a nil nested choice element",
					alternative.scalar.related,
					errInstanceValidationInvariant,
				)
			}
			return newInstanceChoiceInvalid(
				nested.loc,
				fmt.Sprintf("direct choice child %q contains nested element %q", renderSyntaxName(selected.name), renderSyntaxName(nested.name)),
				alternative.scalar.related,
				program.version,
				errInstanceChoiceNested,
			)
		}
		if _, isText := node.(instanceText); !isText {
			return newInstanceValidationInternal(
				selected.loc,
				"instance tree contains an unknown nested choice node",
				alternative.scalar.related,
				errInstanceValidationInvariant,
			)
		}
	}
	return validateScalarValue(selected, alternative.scalar)
}

func newInstanceChoiceInvalid(loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return newInstanceValidationInvalid(
		InvalidInstanceChoiceCode,
		loc,
		message,
		related,
		instanceValidationSpecRef(version),
		cause,
	)
}

func validateScalarStructure(root *instanceElement, scalar instanceScalarType) error {
	for _, attribute := range root.attrs {
		return newInstanceValidationUnsupported(
			attribute.loc,
			fmt.Sprintf("attribute %q is not supported for scalar instance validation", renderSyntaxName(attribute.name)),
			scalar.related,
			scalar.version,
			errInstanceAttributes,
		)
	}
	for _, node := range root.children {
		child, isElement := node.(*instanceElement)
		if isElement {
			if child == nil {
				return newInstanceValidationInternal(
					root.loc,
					"instance tree contains a nil child element",
					scalar.related,
					errInstanceValidationInvariant,
				)
			}
			return newInstanceValidationUnsupported(
				child.loc,
				fmt.Sprintf("child element %q is not supported for scalar instance validation", renderSyntaxName(child.name)),
				scalar.related,
				scalar.version,
				errInstanceChildElements,
			)
		}
		if _, isText := node.(instanceText); !isText {
			return newInstanceValidationInternal(
				root.loc,
				"instance tree contains an unknown scalar child node",
				scalar.related,
				errInstanceValidationInvariant,
			)
		}
	}
	return nil
}

func validateScalarValue(root *instanceElement, scalar instanceScalarType) error {
	lexical, valueLoc := instanceScalarText(root)
	return validateScalarLexicalValue(root.name, lexical, valueLoc, scalar)
}

func validateScalarLexicalValue(name syntaxName, lexical string, valueLoc Loc, scalar instanceScalarType) error {
	switch typed := scalar.value.(type) {
	case instanceDigitScalar:
		return validateDigitScalarValue(name, lexical, valueLoc, scalar, typed)
	case instancePrecisionDecimalScalar:
		return validatePrecisionDecimalScalarValue(lexical, valueLoc, scalar, typed)
	case instanceBooleanScalar:
		return validateBooleanScalarValue(lexical, valueLoc, scalar)
	default:
		return newInstanceValidationInternal(
			valueLoc,
			fmt.Sprintf("element %q has an unknown scalar value representation", renderSyntaxName(name)),
			scalar.related,
			errInstanceValidationInvariant,
		)
	}
}

func validateDigitScalarValue(name syntaxName, lexical string, valueLoc Loc, scalar instanceScalarType, typed instanceDigitScalar) error {
	switch typed.facets.Kind() {
	case DigitDatatypeInteger:
		return validateIntegerScalarValue(lexical, valueLoc, scalar, typed.facets, typed.integerBounds)
	case DigitDatatypeDecimal:
		return validateDecimalScalarValue(lexical, valueLoc, scalar, typed.facets, typed.decimalBounds)
	default:
		return newInstanceValidationInternal(
			valueLoc,
			fmt.Sprintf("element %q has an unknown scalar datatype %q", renderSyntaxName(name), typed.facets.Kind()),
			scalar.related,
			errInstanceValidationInvariant,
		)
	}
}

func validateIntegerScalarValue(lexical string, valueLoc Loc, scalar instanceScalarType, facets DigitFacets, bounds IntegerBoundFacets) error {
	value, parseErr := ParseStrictInteger(lexical, valueLoc)
	if parseErr != nil {
		return instanceDecorateDiagnostic(parseErr, scalar.related, instanceIntegerSpecRef(scalar.version), valueLoc)
	}
	if facetErr := facets.ValidateInteger(value, valueLoc); facetErr != nil {
		return instanceDecorateDiagnostic(facetErr, scalar.related, instanceIntegerSpecRef(scalar.version), valueLoc)
	}
	if boundErr := bounds.ValidateInteger(value, valueLoc); boundErr != nil {
		return instanceDecorateDiagnostic(boundErr, scalar.related, instanceIntegerSpecRef(scalar.version), valueLoc)
	}
	if enumerationErr := validateIntegerEnumerationValue(value, valueLoc, scalar); enumerationErr != nil {
		return instanceDecorateDiagnostic(enumerationErr, scalar.related, instanceIntegerSpecRef(scalar.version), valueLoc)
	}
	return nil
}

func validateDecimalScalarValue(lexical string, valueLoc Loc, scalar instanceScalarType, facets DigitFacets, bounds DecimalBoundFacets) error {
	value, parseErr := ParseStrictDecimalFor(scalar.version, lexical, valueLoc)
	if parseErr != nil {
		return instanceDecorateDiagnostic(parseErr, scalar.related, instanceDecimalSpecRef(scalar.version), valueLoc)
	}
	if facetErr := facets.ValidateDecimal(value, valueLoc); facetErr != nil {
		return instanceDecorateDiagnostic(facetErr, scalar.related, instanceDecimalSpecRef(scalar.version), valueLoc)
	}
	if boundErr := bounds.ValidateDecimal(value, valueLoc); boundErr != nil {
		return instanceDecorateDiagnostic(boundErr, scalar.related, instanceDecimalSpecRef(scalar.version), valueLoc)
	}
	if enumerationErr := validateDecimalEnumerationValue(value, valueLoc, scalar); enumerationErr != nil {
		return instanceDecorateDiagnostic(enumerationErr, scalar.related, instanceDecimalSpecRef(scalar.version), valueLoc)
	}
	return nil
}

func validateIntegerEnumerationValue(value StrictInteger, valueLoc Loc, scalar instanceScalarType) error {
	if scalar.enumeration == nil {
		return nil
	}
	enumeration, ok := scalar.enumeration.(instanceIntegerEnumerationFacet)
	if !ok {
		return newInstanceValidationInternal(
			valueLoc,
			"integer scalar validation has a non-integer enumeration facet",
			scalar.related,
			errInstanceValidationInvariant,
		)
	}
	return enumeration.facets.ValidateInteger(value, valueLoc)
}

func validateDecimalEnumerationValue(value StrictDecimal, valueLoc Loc, scalar instanceScalarType) error {
	if scalar.enumeration == nil {
		return nil
	}
	enumeration, ok := scalar.enumeration.(instanceDecimalEnumerationFacet)
	if !ok {
		return newInstanceValidationInternal(
			valueLoc,
			"decimal scalar validation has a non-decimal enumeration facet",
			scalar.related,
			errInstanceValidationInvariant,
		)
	}
	return enumeration.facets.ValidateDecimal(value, valueLoc)
}

func validatePrecisionDecimalScalarValue(lexical string, valueLoc Loc, scalar instanceScalarType, typed instancePrecisionDecimalScalar) error {
	facetErr := typed.facets.Validate(lexical, valueLoc)
	if facetErr == nil {
		return nil
	}
	return instanceDecorateDiagnostic(facetErr, scalar.related, precisionDecimalLexicalSpecRef, valueLoc)
}

func validateBooleanScalarValue(lexical string, valueLoc Loc, scalar instanceScalarType) error {
	_, parseErr := ParseStrictBooleanFor(scalar.version, lexical, valueLoc)
	if parseErr == nil {
		return nil
	}
	return instanceDecorateDiagnostic(parseErr, scalar.related, strictBooleanSpecRef(scalar.version), valueLoc)
}

func instanceScalarTypeFor(schema Schema, declaration ElementDeclaration, loc Loc) (instanceScalarType, error) {
	typeID, hasTypeID := declaration.TypeID()
	return instanceScalarTypeForTarget(
		schema,
		declaration.DeclaredType(),
		typeID,
		hasTypeID,
		[]Loc{declaration.Loc()},
		loc,
		instanceBuiltInValidationVersion,
		true,
		true,
		instanceSchemaValidationVersion(schema),
	)
}

//nolint:gocognit,funlen // Keep target resolution and scalar-plan construction ordered.
func instanceScalarTypeForTarget(
	schema Schema,
	declaredType QName,
	typeID ComponentID,
	hasTypeID bool,
	related []Loc,
	loc Loc,
	fallbackVersion XSDVersion,
	allowBoolean bool,
	allowPrecisionDecimal bool,
	booleanVersion XSDVersion,
) (instanceScalarType, error) {
	if declaredType.IsZero() {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			"element has no declared type for scalar validation",
			related,
			fallbackVersion,
			errInstanceNoDeclaredType,
		)
	}
	if declaredType.Namespace() == xsdNamespaceURI {
		if declaredType.Local() == "boolean" && !allowBoolean {
			return instanceScalarType{}, newInstanceValidationUnsupported(
				loc,
				fmt.Sprintf("element type %q is outside scalar validation", declaredType),
				related,
				fallbackVersion,
				errInstanceUnsupportedType,
			)
		}
		return instanceBuiltInScalarType(declaredType, related, loc, allowPrecisionDecimal, booleanVersion)
	}
	if !hasTypeID || typeID.IsZero() {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("element type %q has no resolved simple type", declaredType),
			related,
			fallbackVersion,
			errInstanceNoDeclaredType,
		)
	}
	typeComponent, ok := schema.Lookup(typeID)
	if !ok {
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("element type ID %v is not in the completed schema", typeID),
			related,
			errInstanceValidationInvariant,
		)
	}
	related = appendInstanceRelated(relCopy(related), typeComponent.Loc())
	if typeComponent.Kind() != ComponentKindSimpleTypeDefinition {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("element type %q is outside scalar validation", declaredType),
			related,
			fallbackVersion,
			errInstanceChoiceTarget,
		)
	}
	definition, ok := typeComponent.SimpleTypeDefinition()
	if !ok {
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("element type ID %v has no simple type view", typeID),
			related,
			errInstanceValidationInvariant,
		)
	}
	if definition.Variety() != SimpleTypeVarietyAtomicRestriction {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named simple type %q has variety %q outside scalar validation", definition.Name(), definition.Variety()),
			related,
			fallbackVersion,
			errInstanceUnsupportedType,
		)
	}
	if definition.facts != nil && schemaSimpleTypeAtomicKindIsUnsupported(definition.facts.atomicKind) {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named simple type %q is outside scalar validation", definition.Name()),
			related,
			fallbackVersion,
			errInstanceUnsupportedType,
		)
	}
	if definition.HasPrecisionDecimalFacets() {
		if !allowPrecisionDecimal {
			return instanceScalarType{}, newInstanceValidationUnsupported(
				loc,
				fmt.Sprintf("named simple type %q is outside direct choice reference validation", definition.Name()),
				related,
				fallbackVersion,
				errInstanceUnsupportedType,
			)
		}
		return instanceScalarType{
			value:   instancePrecisionDecimalScalar{facets: definition.PrecisionDecimalFacets()},
			version: fallbackVersion,
			related: related,
		}, nil
	}
	if definition.IsBoolean() {
		if !allowBoolean {
			return instanceScalarType{}, newInstanceValidationUnsupported(
				loc,
				fmt.Sprintf("named simple type %q is outside scalar validation", definition.Name()),
				related,
				fallbackVersion,
				errInstanceUnsupportedType,
			)
		}
		return instanceScalarType{
			value:   instanceBooleanScalar{},
			version: booleanVersion,
			related: related,
		}, nil
	}
	if definition.facts == nil || definition.facts.atomicKind != schemaSimpleTypeAtomicInteger && definition.facts.atomicKind != schemaSimpleTypeAtomicDecimal && definition.facts.atomicKind != schemaSimpleTypeAtomicPrecisionDecimal {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named simple type %q has an unsupported atomic datatype", definition.Name()),
			related,
			fallbackVersion,
			errInstanceUnsupportedType,
		)
	}
	facets := definition.DigitFacets()
	if facets.Kind() != DigitDatatypeInteger && facets.Kind() != DigitDatatypeDecimal {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named simple type %q has unsupported scalar datatype facts", definition.Name()),
			related,
			fallbackVersion,
			errInstanceUnsupportedType,
		)
	}
	if facets.Version() != XSDVersion10 && facets.Version() != XSDVersion11 {
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("named simple type %q has an unknown XSD version", definition.Name()),
			related,
			errInstanceValidationInvariant,
		)
	}
	var digitScalar instanceDigitScalar
	switch facets.Kind() {
	case DigitDatatypeInteger:
		bounds, hasBounds := definition.IntegerBounds()
		if !hasBounds || bounds.Version() != facets.Version() {
			return instanceScalarType{}, newInstanceValidationInternal(
				loc,
				fmt.Sprintf("named simple type %q has incomplete integer bounds", definition.Name()),
				related,
				errInstanceValidationInvariant,
			)
		}
		digitScalar = instanceDigitScalar{facets: facets, integerBounds: bounds}
	case DigitDatatypeDecimal:
		bounds, hasBounds := definition.DecimalBounds()
		if !hasBounds || bounds.Version() != facets.Version() {
			return instanceScalarType{}, newInstanceValidationInternal(
				loc,
				fmt.Sprintf("named simple type %q has incomplete decimal bounds", definition.Name()),
				related,
				errInstanceValidationInvariant,
			)
		}
		digitScalar = instanceDigitScalar{facets: facets, decimalBounds: bounds}
	default:
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("named simple type %q has an unknown digit datatype", definition.Name()),
			related,
			errInstanceValidationInvariant,
		)
	}
	scalar := instanceScalarType{
		value:   digitScalar,
		version: facets.Version(),
		related: related,
	}
	var enumerationLocations []Loc
	scalar.enumeration, enumerationLocations = instanceScalarEnumerationFor(definition, facets.Kind())
	for _, location := range enumerationLocations {
		scalar.related = appendInstanceRelated(scalar.related, location)
	}
	return scalar, nil
}

func instanceScalarEnumerationFor(definition SimpleTypeDefinition, kind DigitDatatype) (instanceEnumerationFacet, []Loc) {
	switch kind {
	case DigitDatatypeInteger:
		enumeration := definition.IntegerEnumerationFacets()
		if !enumeration.HasEnumeration() {
			return nil, nil
		}
		return instanceIntegerEnumerationFacet{facets: enumeration}, enumeration.Locations()
	case DigitDatatypeDecimal:
		enumeration := definition.DecimalEnumerationFacets()
		if !enumeration.HasEnumeration() {
			return nil, nil
		}
		return instanceDecimalEnumerationFacet{facets: enumeration}, enumeration.Locations()
	default:
		return nil, nil
	}
}

func instanceBuiltInScalarType(declaredType QName, related []Loc, loc Loc, allowPrecisionDecimal bool, booleanVersion XSDVersion) (instanceScalarType, error) {
	switch declaredType.Local() {
	case "integer":
		facets, err := NewIntegerDigitFacets(nil, instanceBuiltInValidationVersion)
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in integer digit facets", related, err)
		}
		bounds, err := NewIntegerBoundFacets(nil, instanceBuiltInValidationVersion)
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in integer bounds", related, err)
		}
		return instanceScalarType{
			value:   instanceDigitScalar{facets: facets, integerBounds: bounds},
			version: instanceBuiltInValidationVersion,
			related: related,
		}, nil
	case "decimal":
		facets, err := NewDecimalDigitFacets(nil, nil, instanceBuiltInValidationVersion)
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in decimal digit facets", related, err)
		}
		bounds, err := NewDecimalBoundFacets(nil, instanceBuiltInValidationVersion)
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in decimal bounds", related, err)
		}
		return instanceScalarType{
			value:   instanceDigitScalar{facets: facets, decimalBounds: bounds},
			version: instanceBuiltInValidationVersion,
			related: related,
		}, nil
	case "precisionDecimal":
		if !allowPrecisionDecimal {
			return instanceScalarType{}, newInstanceValidationUnsupported(
				loc,
				fmt.Sprintf("global element type %q is outside direct choice reference validation", declaredType),
				related,
				instanceBuiltInValidationVersion,
				errInstanceUnsupportedType,
			)
		}
		facets, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{})
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in precisionDecimal facets", related, err)
		}
		return instanceScalarType{
			value:   instancePrecisionDecimalScalar{facets: facets},
			version: instanceBuiltInValidationVersion,
			related: related,
		}, nil
	case "boolean":
		return instanceScalarType{
			value:   instanceBooleanScalar{},
			version: booleanVersion,
			related: related,
		}, nil
	case "language", "NCName", "anyURI", "ID":
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("global element type %q is outside scalar validation", declaredType),
			related,
			instanceBuiltInValidationVersion,
			errInstanceUnsupportedType,
		)
	default:
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("global element type %q is outside scalar validation", declaredType),
			related,
			instanceBuiltInValidationVersion,
			errInstanceUnsupportedType,
		)
	}
}

func instanceScalarText(root *instanceElement) (string, Loc) {
	var content strings.Builder
	valueLoc := root.loc
	for index, node := range root.children {
		text, ok := node.(instanceText)
		if !ok {
			continue
		}
		if index == 0 || valueLoc == root.loc && content.Len() == 0 {
			valueLoc = text.loc
		}
		content.WriteString(text.data)
	}
	return content.String(), valueLoc
}

func instanceValidationSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceValidationXSD10SpecRef
	}
	return instanceValidationXSD11SpecRef
}

func instanceSchemaValidationVersion(schema Schema) XSDVersion {
	version, err := xsdVersionForLanguagePolicy(schema.policy)
	if err != nil {
		return instanceBuiltInValidationVersion
	}
	return version
}

func instanceIntegerSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceIntegerXSD10SpecRef
	}
	return instanceIntegerXSD11SpecRef
}

func instanceDecimalSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return instanceDecimalXSD10SpecRef
	}
	return instanceDecimalXSD11SpecRef
}

func newInstanceValidationInvalid(code string, loc Loc, message string, related []Loc, specRef string, cause error) Diagnostic {
	diagnostic := newDiagnostic(FailureInvalid, code, loc, message, cause)
	diagnostic.related = relCopy(related)
	diagnostic.specRef = specRef
	return diagnostic
}

func newInstanceValidationUnsupported(loc Loc, message string, related []Loc, version XSDVersion, cause error) error {
	feature, ok := LookupUnsupportedFeature(FeatureInstanceValidation)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			"instance validation feature is not registered",
			cause,
		)
	}
	diagnostic := newUnsupportedForVersion(feature, UnsupportedInstanceValidationCode, loc, message, version)
	if diagnostic.Class() != FailureUnsupported {
		return diagnostic
	}
	diagnostic.related = relCopy(related)
	diagnostic.cause = cause
	return diagnostic
}

func newInstanceValidationInternal(loc Loc, message string, related []Loc, cause error) Diagnostic {
	diagnostic := newDiagnostic(FailureInternal, diagnosticInstanceValidationCode, loc, message, cause)
	diagnostic.related = relCopy(related)
	return diagnostic
}

func instanceDecorateDiagnostic(err error, related []Loc, specRef string, fallbackLoc Loc) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return newInstanceValidationInternal(fallbackLoc, "scalar datatype validation returned an unclassified error", related, err)
	}
	decorated := diagnostic
	decorated.related = mergeInstanceRelated(related, diagnostic.related)
	if decorated.specRef == "" {
		decorated.specRef = specRef
	}
	return decorated
}

func mergeInstanceRelated(primary, additional []Loc) []Loc {
	result := relCopy(primary)
	for _, loc := range additional {
		result = appendInstanceRelated(result, loc)
	}
	return result
}

func appendInstanceRelated(locations []Loc, loc Loc) []Loc {
	if loc.IsZero() {
		return locations
	}
	for _, existing := range locations {
		if existing == loc {
			return locations
		}
	}
	return append(locations, loc)
}

func relCopy(locations []Loc) []Loc {
	return append([]Loc(nil), locations...)
}
