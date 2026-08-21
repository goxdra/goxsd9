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
	diagnosticInstanceValidationCode            = "GOXSD9026"
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
	errInstanceValidationInvariant = errors.New("scalar validation invariant is broken")
)

type instanceScalarType struct {
	kind    DigitDatatype
	version XSDVersion
	facets  DigitFacets
	related []Loc
}

// ValidateInstance consumes, drains, and closes reader exactly once, then
// validates one XML instance against schema. The supported semantic slice is
// a single global element whose declared type is built-in or named XSD
// integer or decimal. The element has no attributes or child elements; its
// ordered character data is parsed and checked against effective digit
// facets. Comments and processing instructions are ignored by the decoder.
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
	matches := schema.FindKind(ComponentKindElementDeclaration, rootName)
	if len(matches) == 0 {
		return ElementDeclaration{}, newInstanceValidationInvalid(
			UnknownInstanceSchemaRootCode,
			loc,
			fmt.Sprintf("instance root %q has no matching global element declaration", rootName),
			nil,
			instanceValidationSpecRef(instanceBuiltInValidationVersion),
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
			instanceValidationSpecRef(instanceBuiltInValidationVersion),
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
			instanceBuiltInValidationVersion,
			errInstanceNoDeclaredType,
		)
	}
	return declaration, nil
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
	related := []Loc{declaration.Loc()}
	declaredType := declaration.DeclaredType()
	if declaredType.IsZero() {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			"global element has no declared type for scalar validation",
			related,
			instanceBuiltInValidationVersion,
			errInstanceNoDeclaredType,
		)
	}
	if declaredType.Namespace() == xsdNamespaceURI {
		return instanceBuiltInScalarType(declaredType, related, loc)
	}

	typeID, hasTypeID := declaration.TypeID()
	if !hasTypeID || typeID.IsZero() {
		return instanceScalarType{}, newInstanceValidationUnsupported(
			loc,
			fmt.Sprintf("global element type %q has no resolved simple type", declaredType),
			related,
			instanceBuiltInValidationVersion,
			errInstanceNoDeclaredType,
		)
	}
	definition, related, err := instanceNamedTypeDefinition(schema, typeID, related, loc)
	if err != nil {
		return instanceScalarType{}, err
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

func instanceNamedTypeDefinition(schema Schema, typeID ComponentID, related []Loc, loc Loc) (SimpleTypeDefinition, []Loc, error) {
	typeComponent, ok := schema.Lookup(typeID)
	if !ok {
		return SimpleTypeDefinition{}, related, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("global element type ID %v is not in the completed schema", typeID),
			related,
			errInstanceValidationInvariant,
		)
	}
	related = appendInstanceRelated(relCopy(related), typeComponent.Loc())
	if typeComponent.Kind() != ComponentKindSimpleTypeDefinition {
		return SimpleTypeDefinition{}, related, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("global element type ID %v does not identify a simple type", typeID),
			related,
			errInstanceValidationInvariant,
		)
	}
	definition, ok := typeComponent.SimpleTypeDefinition()
	if !ok {
		return SimpleTypeDefinition{}, related, newInstanceValidationInternal(
			loc,
			fmt.Sprintf("global element type ID %v has no simple type view", typeID),
			related,
			errInstanceValidationInvariant,
		)
	}
	return definition, related, nil
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
