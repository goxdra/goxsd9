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
	errInstanceReaderNil           = errors.New("instance reader is nil")
	errInstanceSourceIDEmpty       = errors.New("instance source ID is empty")
	errInstanceSchemaEmpty         = errors.New("instance schema is zero or incomplete")
	errInstanceUnknownSchemaRoot   = errors.New("instance root has no matching global element declaration")
	errInstanceAmbiguousSchemaRoot = errors.New("instance root has ambiguous global element declarations")
	errInstanceNoDeclaredType      = errors.New("global element has no supported declared type")
	errInstanceUnsupportedType     = errors.New("global element type is outside scalar validation")
	errInstanceAttributes          = errors.New("instance attributes are outside scalar validation")
	errInstanceChildElements       = errors.New("instance child elements are outside scalar validation")
	errInstanceChoiceMissing       = errors.New("choice instance has no selected element")
	errInstanceChoiceUnknown       = errors.New("choice instance has an unknown element")
	errInstanceChoiceMultiple      = errors.New("choice instance has multiple elements")
	errInstanceChoiceText          = errors.New("choice instance has non-whitespace parent text")
	errInstanceChoiceNested        = errors.New("choice instance has nested element content")
	errInstanceChoiceParticle      = errors.New("choice type has an unsupported particle")
	errInstanceChoiceTarget        = errors.New("choice alternative has an unsupported target")
	errInstanceValidationInvariant = errors.New("scalar validation invariant is broken")
)

type instanceScalarType struct {
	kind    DigitDatatype
	version XSDVersion
	facets  DigitFacets
	related []Loc
}

type instanceChoiceAlternative struct {
	name   QName
	loc    Loc
	scalar instanceScalarType
}

type instanceChoiceProgram struct {
	version      XSDVersion
	related      []Loc
	alternatives []instanceChoiceAlternative
}

// ValidateInstance consumes, drains, and closes reader exactly once, then
// validates one XML instance against schema. The supported semantic slice is
// a single global element whose declared type is built-in or named XSD
// integer or decimal, or a named complex type with one direct scalar choice.
// Comments and processing instructions are ignored by the decoder.
//
// Built-in element views do not retain a document version, so this entrypoint
// uses the repository's compatibility/default XSD 1.1-compatible datatype
// rules for built-in integer and decimal values. Named types use the version
// retained by their completed effective facets. A successful validation
// returns nil. Unsupported semantic structures return a registered
// xsd.instance.validation diagnostic.
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

	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: sourceID})
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

//nolint:gocognit,funlen // Keep complete choice-program validation in one phase.
func instanceChoiceProgramFor(
	schema Schema,
	declaration ElementDeclaration,
	definition ComplexTypeDefinition,
	loc Loc,
) (instanceChoiceProgram, error) {
	version := instanceSchemaValidationVersion(schema)
	related := []Loc{declaration.Loc(), definition.Loc()}
	particle := definition.Particle()
	choice, ok := particle.(ChoiceParticle)
	if !ok {
		return instanceChoiceProgram{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named complex type %q does not have a direct choice particle", definition.Name()),
			related,
			version,
			errInstanceChoiceParticle,
		)
	}
	if choice.facts == nil {
		return instanceChoiceProgram{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("named complex type %q has an incomplete choice particle", definition.Name()),
			related,
			errInstanceValidationInvariant,
		)
	}
	related = appendInstanceRelated(relCopy(related), choice.Loc())
	if choice.MinOccurs() != 1 || choice.MaxOccurs() != 1 {
		return instanceChoiceProgram{}, newInstanceValidationUnsupported(
			choice.Loc(),
			fmt.Sprintf("choice particle occurrence bounds %d/%d are outside instance validation", choice.MinOccurs(), choice.MaxOccurs()),
			related,
			version,
			errInstanceChoiceParticle,
		)
	}

	particleAlternatives := choice.Alternatives()
	elements := make([]ElementParticle, 0, len(particleAlternatives))
	for _, particleAlternative := range particleAlternatives {
		element, ok := particleAlternative.(ElementParticle)
		if !ok || element.facts == nil {
			return instanceChoiceProgram{}, newInstanceValidationUnsupported(
				choice.Loc(),
				"direct choice contains a non-element alternative",
				related,
				version,
				errInstanceChoiceParticle,
			)
		}
		if element.MinOccurs() != 1 || element.MaxOccurs() != 1 {
			return instanceChoiceProgram{}, newInstanceValidationUnsupported(
				element.Loc(),
				fmt.Sprintf("choice alternative occurrence bounds %d/%d are outside instance validation", element.MinOccurs(), element.MaxOccurs()),
				appendInstanceRelated(relCopy(related), element.Loc()),
				version,
				errInstanceChoiceParticle,
			)
		}
		if element.Name().IsZero() {
			return instanceChoiceProgram{}, newInstanceValidationInternal(
				element.Loc(),
				"direct choice element has no expanded name",
				appendInstanceRelated(relCopy(related), element.Loc()),
				errInstanceValidationInvariant,
			)
		}
		elements = append(elements, element)
		related = appendInstanceRelated(related, element.Loc())
	}

	program := instanceChoiceProgram{
		version:      version,
		related:      related,
		alternatives: make([]instanceChoiceAlternative, 0, len(elements)),
	}
	for _, element := range elements {
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
		)
		if err != nil {
			return instanceChoiceProgram{}, err
		}
		program.alternatives = append(program.alternatives, instanceChoiceAlternative{
			name:   element.Name(),
			loc:    element.Loc(),
			scalar: scalar,
		})
	}
	return program, nil
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
	switch scalar.kind {
	case DigitDatatypeInteger:
		value, parseErr := ParseStrictInteger(lexical, valueLoc)
		if parseErr != nil {
			return instanceDecorateDiagnostic(parseErr, scalar.related, instanceIntegerSpecRef(scalar.version), valueLoc)
		}
		if facetErr := scalar.facets.ValidateInteger(value, valueLoc); facetErr != nil {
			return instanceDecorateDiagnostic(facetErr, scalar.related, instanceIntegerSpecRef(scalar.version), valueLoc)
		}
		return nil
	case DigitDatatypeDecimal:
		value, parseErr := ParseStrictDecimalFor(scalar.version, lexical, valueLoc)
		if parseErr != nil {
			return instanceDecorateDiagnostic(parseErr, scalar.related, instanceDecimalSpecRef(scalar.version), valueLoc)
		}
		if facetErr := scalar.facets.ValidateDecimal(value, valueLoc); facetErr != nil {
			return instanceDecorateDiagnostic(facetErr, scalar.related, instanceDecimalSpecRef(scalar.version), valueLoc)
		}
		return nil
	default:
		return newInstanceValidationInternal(
			valueLoc,
			fmt.Sprintf("global element %q has an unknown scalar datatype %q", renderSyntaxName(root.name), scalar.kind),
			scalar.related,
			errInstanceValidationInvariant,
		)
	}
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
	)
}

func instanceScalarTypeForTarget(
	schema Schema,
	declaredType QName,
	typeID ComponentID,
	hasTypeID bool,
	related []Loc,
	loc Loc,
	fallbackVersion XSDVersion,
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
		return instanceBuiltInScalarType(declaredType, related, loc)
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
	if definition.HasPrecisionDecimalFacets() {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("named simple type %q uses unsupported precisionDecimal validation", definition.Name()),
			related,
			fallbackVersion,
			errInstanceUnsupportedType,
		)
	}
	facets := definition.DigitFacets()
	if facets.Kind() != DigitDatatypeInteger && facets.Kind() != DigitDatatypeDecimal {
		return instanceScalarType{}, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("named simple type %q has an unknown digit datatype", definition.Name()),
			related,
			errInstanceValidationInvariant,
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
	return instanceScalarType{
		kind:    facets.Kind(),
		version: facets.Version(),
		facets:  facets,
		related: related,
	}, nil
}

func instanceBuiltInScalarType(declaredType QName, related []Loc, loc Loc) (instanceScalarType, error) {
	switch declaredType.Local() {
	case "integer":
		facets, err := NewIntegerDigitFacets(nil, instanceBuiltInValidationVersion)
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in integer digit facets", related, err)
		}
		return instanceScalarType{
			kind:    DigitDatatypeInteger,
			version: instanceBuiltInValidationVersion,
			facets:  facets,
			related: related,
		}, nil
	case "decimal":
		facets, err := NewDecimalDigitFacets(nil, nil, instanceBuiltInValidationVersion)
		if err != nil {
			return instanceScalarType{}, newInstanceValidationInternal(loc, "construct built-in decimal digit facets", related, err)
		}
		return instanceScalarType{
			kind:    DigitDatatypeDecimal,
			version: instanceBuiltInValidationVersion,
			facets:  facets,
			related: related,
		}, nil
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
