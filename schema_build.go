package goxsd9

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	invalidSchemaTargetNamespaceCode           = "XSD3009"
	invalidSchemaCompositionCode               = "XSD3010"
	invalidSchemaDeclarationNameCode           = "XSD3011"
	diagnosticSchemaSimpleTypeUnresolvedCode   = "XSD3014"
	diagnosticSchemaSimpleTypeWrongKindCode    = "XSD3015"
	diagnosticSchemaSimpleTypeAmbiguousCode    = "XSD3016"
	diagnosticSchemaSimpleTypeCycleCode        = "XSD3017"
	diagnosticSchemaSimpleTypeBaseCode         = "XSD3018"
	diagnosticSchemaElementTypeUnresolvedCode  = "XSD3019"
	diagnosticSchemaElementTypeWrongKindCode   = "XSD3020"
	diagnosticSchemaElementTypeAmbiguousCode   = "XSD3021"
	diagnosticSchemaElementTypeUnsupportedCode = "XSD3022"
	diagnosticSchemaBridgeInvariantCode        = "GOXSD9025"
)

const (
	schemaSimpleTypeXSD10SpecRef  = "xsd10-structures#Simple_Type_Definitions"
	schemaSimpleTypeXSD11SpecRef  = "xsd11-structures#Simple_Type_Definition"
	schemaElementTypeXSD10SpecRef = "xsd10-structures#Element_Declaration_details"
	schemaElementTypeXSD11SpecRef = "xsd11-structures#Element_Declaration_details"
)

var (
	errSchemaSimpleTypeBaseUnresolved = errors.New("simple type base is unresolved")
	errSchemaSimpleTypeBaseWrongKind  = errors.New("simple type base has the wrong kind")
	errSchemaSimpleTypeBaseAmbiguous  = errors.New("simple type base is ambiguous")
	errSchemaSimpleTypeBaseCycle      = errors.New("simple type base is cyclic")
	errSchemaElementTypeUnresolved    = errors.New("element type is unresolved")
	errSchemaElementTypeWrongKind     = errors.New("element type has the wrong kind")
	errSchemaElementTypeAmbiguous     = errors.New("element type is ambiguous")
)

type schemaTargetNamespace struct {
	value   string
	present bool
	version XSDVersion
}

// discoverSchema completes the internal pipeline used by ParseSchema.
func discoverSchema(root ResolvedSource, resolver Resolver) (Schema, error) {
	discovery, err := discoverSyntax(root, resolver)
	if err != nil {
		return Schema{}, err
	}
	return newSchemaFromDiscovery(discovery)
}

func newSchemaFromDiscovery(discovery syntaxDiscoveryResult) (Schema, error) {
	namespaces, sourceIndices, err := schemaDiscoveryNamespaces(discovery.documents)
	if err != nil {
		return Schema{}, err
	}

	err = validateSchemaComposition(discovery.edges, sourceIndices, namespaces)
	if err != nil {
		return Schema{}, err
	}

	inputs, err := schemaDocumentInputs(discovery.documents, namespaces)
	if err != nil {
		return Schema{}, err
	}

	schema, err := newSchema(inputs)
	if err != nil {
		return Schema{}, err
	}
	return schema, nil
}

func schemaDiscoveryNamespaces(documents []*syntaxDocument) ([]schemaTargetNamespace, map[SourceID]int, error) {
	namespaces := make([]schemaTargetNamespace, len(documents))
	sourceIndices := make(map[SourceID]int, len(documents))
	for index, document := range documents {
		if err := validateDiscoveredDocument(document, sourceIndices); err != nil {
			return nil, nil, err
		}
		namespace, err := syntaxDocumentTargetNamespace(document)
		if err != nil {
			return nil, nil, err
		}
		version, err := syntaxDocumentVersion(document)
		if err != nil {
			return nil, nil, err
		}
		sourceIndices[document.source] = index
		namespace.version = version
		namespaces[index] = namespace
	}
	return namespaces, sourceIndices, nil
}

func validateDiscoveredDocument(document *syntaxDocument, sourceIndices map[SourceID]int) error {
	if document == nil || document.root == nil {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxDocumentNoRootCode,
			Loc{},
			"schema discovery result contains a document without a root",
			nil,
		)
	}
	if document.source == "" {
		return newDiagnostic(
			FailureInternal,
			diagnosticSchemaEmptySourceCode,
			document.root.loc,
			"schema discovery result contains an empty source identity",
			nil,
		)
	}
	if _, exists := sourceIndices[document.source]; exists {
		return newDiagnostic(
			FailureInternal,
			diagnosticSchemaRepeatedSourceCode,
			document.root.loc,
			fmt.Sprintf("schema discovery result repeats source %q", document.source),
			nil,
		)
	}
	return validateSyntaxDocumentStructure(document)
}

func schemaDocumentInputs(documents []*syntaxDocument, namespaces []schemaTargetNamespace) ([]schemaDocumentInput, error) {
	inputs := make([]schemaDocumentInput, 0, len(documents))
	for index, document := range documents {
		declarations, err := schemaDocumentDeclarations(document, namespaces[index].value, namespaces[index].version)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, schemaDocumentInput{
			source:          document.source,
			rootLoc:         document.root.loc,
			targetNamespace: namespaces[index].value,
			version:         namespaces[index].version,
			declarations:    declarations,
		})
	}
	return inputs, nil
}

func syntaxDocumentTargetNamespace(document *syntaxDocument) (schemaTargetNamespace, error) {
	attributes := syntaxAttributesByLocal(document.root, "targetNamespace")
	if len(attributes) == 0 {
		return schemaTargetNamespace{}, nil
	}
	if len(attributes) != 1 || attributes[0].name.namespace != "" {
		return schemaTargetNamespace{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaTargetNamespaceCode,
			document.root.loc,
			"schema targetNamespace must be one unqualified non-empty attribute",
			nil,
		)
	}
	attribute := attributes[0]
	value := collapseXMLWhitespace(attribute.value)
	if value == "" {
		return schemaTargetNamespace{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaTargetNamespaceCode,
			attribute.loc,
			"schema targetNamespace cannot be empty when present",
			nil,
		)
	}
	return schemaTargetNamespace{
		value:   value,
		present: true,
	}, nil
}

func syntaxDocumentVersion(document *syntaxDocument) (XSDVersion, error) {
	attributes := syntaxAttributesByLocal(document.root, "version")
	if len(attributes) == 0 {
		return XSDVersion11, nil
	}
	if len(attributes) != 1 || attributes[0].name.namespace != "" {
		return "", newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			document.root.loc,
			"schema version must be one unqualified attribute",
			nil,
		)
	}
	attribute := attributes[0]
	switch collapseXMLWhitespace(attribute.value) {
	case "", string(XSDVersion11):
		return XSDVersion11, nil
	case string(XSDVersion10):
		return XSDVersion10, nil
	default:
		return "", newSchemaSyntaxUnsupported(
			attribute.loc,
			fmt.Sprintf("XSD schema version %q is not supported", collapseXMLWhitespace(attribute.value)),
		)
	}
}

func validateSchemaComposition(
	edges []syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
	namespaces []schemaTargetNamespace,
) error {
	for _, edge := range edges {
		if err := validateSchemaCompositionEdge(edge, sourceIndices, namespaces); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaCompositionEdge(
	edge syntaxDocumentEdge,
	sourceIndices map[SourceID]int,
	namespaces []schemaTargetNamespace,
) error {
	sourceIndex, ok := sourceIndices[edge.source]
	if !ok {
		return newSchemaBridgeInvariant(edge.loc, fmt.Sprintf("schema edge source %q is unknown", edge.source))
	}
	targetIndex, ok := sourceIndices[edge.target]
	if !ok {
		return newSchemaBridgeInvariant(edge.loc, fmt.Sprintf("schema edge target %q is unknown", edge.target))
	}
	if sourceIndex < 0 || targetIndex < 0 || sourceIndex >= len(namespaces) || targetIndex >= len(namespaces) {
		return newSchemaBridgeInvariant(edge.loc, "schema edge points outside the namespace table")
	}
	parent := namespaces[sourceIndex]
	child := namespaces[targetIndex]
	switch edge.kind {
	case syntaxReferenceInclude:
		return validateSchemaInclude(edge, parent, child)
	case syntaxReferenceImport:
		return validateSchemaImport(edge, parent, child)
	default:
		return newSchemaBridgeInvariant(edge.loc, "schema edge has an unknown reference kind")
	}
}

func validateSchemaInclude(
	edge syntaxDocumentEdge,
	parent schemaTargetNamespace,
	child schemaTargetNamespace,
) error {
	if parent.present && !child.present {
		return newSchemaSyntaxUnsupported(edge.loc, "chameleon schema inclusion is not implemented")
	}
	if !parent.present && child.present {
		return newSchemaCompositionDiagnostic(edge.loc, "schema include adds a target namespace to a no-namespace document")
	}
	if parent.present && child.present && parent.value != child.value {
		return newSchemaCompositionDiagnostic(
			edge.loc,
			fmt.Sprintf("schema include target namespace %q does not match including namespace %q", child.value, parent.value),
		)
	}
	return nil
}

func validateSchemaImport(
	edge syntaxDocumentEdge,
	parent schemaTargetNamespace,
	child schemaTargetNamespace,
) error {
	if edge.hasNamespace {
		if edge.namespaceURN == "" {
			return newSchemaCompositionDiagnostic(edge.loc, "schema import namespace cannot be empty when present")
		}
		if parent.present && edge.namespaceURN == parent.value {
			return newSchemaCompositionDiagnostic(edge.loc, "schema import namespace must differ from the importing target namespace")
		}
		if !child.present || child.value != edge.namespaceURN {
			return newSchemaCompositionDiagnostic(
				edge.loc,
				fmt.Sprintf("schema import namespace %q does not match imported target namespace", edge.namespaceURN),
			)
		}
		return nil
	}
	if !parent.present {
		return newSchemaCompositionDiagnostic(edge.loc, "schema import without a namespace requires an importing target namespace")
	}
	if child.present {
		return newSchemaCompositionDiagnostic(edge.loc, "schema import without a namespace requires a no-namespace document")
	}
	return nil
}

func schemaDocumentDeclarations(document *syntaxDocument, targetNamespace string, version XSDVersion) ([]schemaComponentInput, error) {
	declarations := make([]schemaComponentInput, 0)
	for _, node := range document.root.children {
		declaration, present, err := schemaDocumentDeclaration(node, targetNamespace, version)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		declarations = append(declarations, declaration)
	}
	return declarations, nil
}

//nolint:gocognit // Keep root declaration classification and phase-specific input explicit.
func schemaDocumentDeclaration(node syntaxNode, targetNamespace string, version XSDVersion) (schemaComponentInput, bool, error) {
	textNode, ok := node.(syntaxText)
	if ok {
		if xmlWhitespace([]byte(textNode.data)) {
			return schemaComponentInput{}, false, nil
		}
		return schemaComponentInput{}, false, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			textNode.loc,
			"schema root contains non-whitespace character data",
			nil,
		)
	}
	element, ok := node.(*syntaxElement)
	if !ok {
		return schemaComponentInput{}, false, newSchemaBridgeInvariant(Loc{}, "schema root contains an unknown syntax node")
	}
	if element.name.namespace != xsdNamespaceURI {
		return schemaComponentInput{}, false, newSchemaSyntaxUnsupported(element.loc, "schema root contains an unsupported non-XSD construct")
	}
	kind, named := schemaDeclarationKind(element.name.local)
	if !named {
		ignored, err := schemaRootChildIgnored(element)
		if err != nil {
			return schemaComponentInput{}, false, err
		}
		if ignored {
			return schemaComponentInput{}, false, nil
		}
		return schemaComponentInput{}, false, newSchemaSyntaxUnsupported(
			element.loc,
			fmt.Sprintf("XSD schema child <%s> is not implemented", element.name.local),
		)
	}

	name, err := schemaDeclarationName(element, targetNamespace)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	declaration := schemaComponentInput{
		kind: kind,
		name: name,
		loc:  element.loc,
	}
	if kind == ComponentKindElementDeclaration {
		elementType, elementErr := schemaElementTypeInput(element, version)
		if elementErr != nil {
			return schemaComponentInput{}, false, elementErr
		}
		declaration.element = elementType
	}
	if kind == ComponentKindComplexTypeDefinition {
		complexType, complexErr := schemaComplexTypeInputFromElement(element, version)
		if complexErr != nil {
			return schemaComponentInput{}, false, complexErr
		}
		declaration.complexType = complexType
	}
	if kind != ComponentKindSimpleTypeDefinition {
		return declaration, true, nil
	}
	simpleType, err := schemaSimpleTypeRestrictionInput(element, version)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	declaration.simpleType = simpleType
	return declaration, true, nil
}

func schemaElementTypeInput(element *syntaxElement, version XSDVersion) (*schemaElementInput, error) {
	attributes := syntaxAttributesByLocal(element, "type")
	if len(attributes) == 0 {
		return nil, nil
	}
	if len(attributes) != 1 {
		return nil, newSchemaCompositionDiagnostic(element.loc, "element type attribute must be unique")
	}
	declaredType, err := expandSchemaQName(element, attributes[0])
	if err != nil {
		return nil, err
	}
	return &schemaElementInput{
		declaredType: declaredType,
		typeLoc:      attributes[0].loc,
		version:      version,
	}, nil
}

func schemaComplexTypeInputFromElement(element *syntaxElement, version XSDVersion) (*schemaComplexTypeInput, error) {
	var choice *syntaxElement
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "choice" {
			continue
		}
		choice = child
		break
	}
	if choice == nil {
		return nil, nil
	}

	input := &schemaComplexTypeInput{
		particle: &schemaChoiceParticleInput{
			loc:          choice.loc,
			minOccurs:    1,
			maxOccurs:    1,
			alternatives: make([]schemaElementParticleInput, 0),
		},
	}
	for _, node := range choice.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if child.name.local != "element" {
			continue
		}
		alternative, err := schemaElementParticleInputFromElement(child, version)
		if err != nil {
			return nil, err
		}
		input.particle.alternatives = append(input.particle.alternatives, alternative)
	}
	return input, nil
}

func schemaElementParticleInputFromElement(element *syntaxElement, version XSDVersion) (schemaElementParticleInput, error) {
	nameAttributes := syntaxAttributesByLocal(element, "name")
	if len(nameAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "choice element input has an invalid name attribute")
	}
	local := collapseXMLWhitespace(nameAttributes[0].value)
	name, err := NewQName("", local)
	if err != nil {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(nameAttributes[0].loc, "construct local choice element name")
	}
	input := schemaElementParticleInput{
		loc:  element.loc,
		name: name,
	}
	typeAttributes := syntaxAttributesByLocal(element, "type")
	if len(typeAttributes) == 0 {
		return input, nil
	}
	if len(typeAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "choice element type attribute is not unique")
	}
	declaredType, err := expandSchemaQName(element, typeAttributes[0])
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	input.typeInput = &schemaElementInput{
		declaredType: declaredType,
		typeLoc:      typeAttributes[0].loc,
		version:      version,
	}
	return input, nil
}

func schemaSimpleTypeRestrictionInput(element *syntaxElement, version XSDVersion) (*schemaSimpleTypeInput, error) {
	var restriction *syntaxElement
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "restriction" {
			continue
		}
		if restriction != nil {
			return nil, newSchemaBridgeInvariant(child.loc, "simple type contains more than one restriction")
		}
		restriction = child
	}
	if restriction == nil {
		return nil, newSchemaBridgeInvariant(element.loc, "supported simple type has no restriction")
	}
	return schemaRestrictionInput(restriction, version)
}

func schemaRestrictionInput(element *syntaxElement, version XSDVersion) (*schemaSimpleTypeInput, error) {
	baseAttributes := syntaxAttributesByLocal(element, "base")
	if len(baseAttributes) != 1 {
		return nil, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			element.loc,
			"simple type restriction requires one base attribute",
			nil,
		)
	}
	base, err := expandSchemaQName(element, baseAttributes[0])
	if err != nil {
		return nil, err
	}
	input := &schemaSimpleTypeInput{
		base:    base,
		baseLoc: baseAttributes[0].loc,
		version: version,
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		switch child.name.local {
		case "totalDigits":
			facet, err := schemaFacetInputFromElement(child)
			if err != nil {
				return nil, err
			}
			input.totalDigits = facet
		case "fractionDigits":
			facet, err := schemaFacetInputFromElement(child)
			if err != nil {
				return nil, err
			}
			input.fractionDigits = facet
		}
	}
	return input, nil
}

func schemaFacetInputFromElement(element *syntaxElement) (*schemaFacetInput, error) {
	valueAttributes := syntaxAttributesByLocal(element, "value")
	if len(valueAttributes) != 1 {
		return nil, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			element.loc,
			element.name.local+" facet requires one value attribute",
			nil,
		)
	}
	fixedAttributes := syntaxAttributesByLocal(element, "fixed")
	if len(fixedAttributes) > 1 {
		return nil, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			element.loc,
			element.name.local+" facet fixed attribute must be unique",
			nil,
		)
	}
	fixed := false
	if len(fixedAttributes) == 1 {
		switch collapseXMLWhitespace(fixedAttributes[0].value) {
		case "true", "1":
			fixed = true
		case "false", "0":
		default:
			return nil, newSchemaCompositionDiagnostic(
				fixedAttributes[0].loc,
				fmt.Sprintf("attribute %q has an invalid boolean value", fixedAttributes[0].name.local),
			)
		}
	}
	return &schemaFacetInput{
		lexical: valueAttributes[0].value,
		loc:     element.loc,
		fixed:   fixed,
	}, nil
}

func expandSchemaQName(element *syntaxElement, attribute syntaxAttribute) (QName, error) {
	lexeme := collapseXMLWhitespace(attribute.value)
	prefix, local, ok := splitConditionalQName(lexeme)
	if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
		return QName{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaConditionalCode,
			attribute.loc,
			fmt.Sprintf("attribute %q has a malformed QName", attribute.name.local),
			nil,
		)
	}
	namespace := ""
	if prefix == "" {
		namespace, _ = element.scope.lookup("")
	}
	if prefix != "" {
		var bound bool
		namespace, bound = element.scope.lookup(prefix)
		if !bound {
			return QName{}, newDiagnostic(
				FailureInvalid,
				invalidSchemaConditionalCode,
				attribute.loc,
				fmt.Sprintf("attribute %q has an unbound QName prefix", attribute.name.local),
				nil,
			)
		}
	}
	name, err := NewQName(namespace, local)
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(attribute.loc, "construct expanded schema QName")
	}
	return name, nil
}

func schemaRootChildIgnored(element *syntaxElement) (bool, error) {
	switch element.name.local {
	case "annotation":
		if err := validateSchemaAnnotationElement(element); err != nil {
			return false, err
		}
		return true, nil
	case "include":
		if err := validateSchemaReferenceElement(element, syntaxReferenceInclude); err != nil {
			return false, err
		}
		return true, nil
	case "import":
		if err := validateSchemaReferenceElement(element, syntaxReferenceImport); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, nil
	}
}

func schemaDeclarationKind(local string) (ComponentKind, bool) {
	switch local {
	case "element":
		return ComponentKindElementDeclaration, true
	case "attribute":
		return ComponentKindAttributeDeclaration, true
	case "simpleType":
		return ComponentKindSimpleTypeDefinition, true
	case "complexType":
		return ComponentKindComplexTypeDefinition, true
	case "group":
		return ComponentKindModelGroupDefinition, true
	case "attributeGroup":
		return ComponentKindAttributeGroupDefinition, true
	case "notation":
		return ComponentKindNotationDeclaration, true
	default:
		return "", false
	}
}

func schemaDeclarationName(element *syntaxElement, targetNamespace string) (QName, error) {
	attributes := syntaxAttributesByLocal(element, "name")
	if len(attributes) == 0 {
		return QName{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaDeclarationNameCode,
			element.loc,
			"schema declaration has no name attribute",
			nil,
		)
	}
	value := collapseXMLWhitespace(attributes[0].value)
	if len(attributes) != 1 || attributes[0].name.namespace != "" || !validNCName(value) {
		return QName{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaDeclarationNameCode,
			element.loc,
			"schema declaration name must be an unqualified valid NCName",
			nil,
		)
	}
	name, err := NewQName(targetNamespace, value)
	if err != nil {
		return QName{}, newDiagnostic(
			FailureInternal,
			diagnosticSchemaBridgeInvariantCode,
			element.loc,
			"construct schema declaration name",
			err,
		)
	}
	return name, nil
}

func syntaxAttributesByLocal(element *syntaxElement, local string) []syntaxAttribute {
	attributes := make([]syntaxAttribute, 0, 1)
	for _, attribute := range element.attrs {
		if attribute.name.namespace == "" && attribute.name.local == local {
			attributes = append(attributes, attribute)
		}
	}
	return attributes
}

func validNCName(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	first := true
	for _, character := range value {
		if first {
			if !validNCNameStart(character) {
				return false
			}
			first = false
			continue
		}
		if !validNCNameChar(character) {
			return false
		}
	}
	return !first
}

type ncNameRange struct {
	first rune
	last  rune
}

var ncNameStartRanges = [...]ncNameRange{
	{first: 0xC0, last: 0xD6},
	{first: 0xD8, last: 0xF6},
	{first: 0xF8, last: 0x2FF},
	{first: 0x370, last: 0x37D},
	{first: 0x37F, last: 0x1FFF},
	{first: 0x200C, last: 0x200D},
	{first: 0x2070, last: 0x218F},
	{first: 0x2C00, last: 0x2FEF},
	{first: 0x3001, last: 0xD7FF},
	{first: 0xF900, last: 0xFDCF},
	{first: 0xFDF0, last: 0xFFFD},
	{first: 0x10000, last: 0xEFFFF},
}

func validNCNameStart(character rune) bool {
	if character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' {
		return true
	}
	for _, validRange := range ncNameStartRanges {
		if character >= validRange.first && character <= validRange.last {
			return true
		}
	}
	return false
}

func validNCNameChar(character rune) bool {
	if validNCNameStart(character) {
		return true
	}
	return character == '-' || character == '.' ||
		character >= '0' && character <= '9' ||
		character == 0xB7 ||
		character >= 0x300 && character <= 0x36F ||
		character >= 0x203F && character <= 0x2040
}

type schemaSimpleTypeResult struct {
	present     bool
	base        QName
	baseLoc     Loc
	baseID      ComponentID
	hasBaseID   bool
	digitFacets DigitFacets
}

type schemaSimpleTypeState uint8

const (
	schemaSimpleTypeUnvisited schemaSimpleTypeState = iota
	schemaSimpleTypeVisiting
	schemaSimpleTypeResolved
)

type schemaSimpleTypeResolver struct {
	records []schemaComponentRecord
	byName  map[QName][]int
	states  []schemaSimpleTypeState
	results []schemaSimpleTypeResult
	stack   []int
}

func resolveSchemaSimpleTypes(records []schemaComponentRecord, byName map[QName][]int) ([]schemaSimpleTypeResult, error) {
	resolver := schemaSimpleTypeResolver{
		records: records,
		byName:  byName,
		states:  make([]schemaSimpleTypeState, len(records)),
		results: make([]schemaSimpleTypeResult, len(records)),
		stack:   make([]int, 0),
	}
	for index, record := range records {
		if record.simpleType == nil {
			continue
		}
		if _, err := resolver.resolve(index); err != nil {
			return nil, err
		}
	}
	return resolver.results, nil
}

type schemaElementTypeResult struct {
	present      bool
	declaredType QName
	typeID       ComponentID
	hasTypeID    bool
}

func resolveSchemaElementTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
) ([]schemaElementTypeResult, error) {
	if len(simpleTypes) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "element type resolution has incomplete simple type results")
	}
	results := make([]schemaElementTypeResult, len(records))
	for index, record := range records {
		if record.element == nil {
			continue
		}
		result, err := resolveSchemaElementType(record, records, byName, simpleTypes)
		if err != nil {
			return nil, err
		}
		results[index] = result
	}
	return results, nil
}

func resolveSchemaElementType(
	record schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
) (schemaElementTypeResult, error) {
	input := record.element
	if input == nil {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(record.loc, "element type resolution has no type input")
	}
	return resolveSchemaScalarType(input, records, byName, simpleTypes, "for global elements")
}

func resolveSchemaScalarType(
	input *schemaElementInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	complexTargetSuffix string,
) (schemaElementTypeResult, error) {
	if input.declaredType.Namespace() == xsdNamespaceURI {
		switch input.declaredType.Local() {
		case "integer", "decimal":
			return schemaElementTypeResult{
				present:      true,
				declaredType: input.declaredType,
			}, nil
		case "precisionDecimal":
			return schemaElementTypeResult{}, unsupportedSchemaElementPrecisionDecimal(input)
		default:
			return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
				input.typeLoc,
				fmt.Sprintf("element type %q is not implemented", input.declaredType),
				input.version,
			)
		}
	}

	candidates := byName[input.declaredType]
	if len(candidates) == 0 {
		return unresolvedSchemaElementType(input)
	}
	typeCandidates := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		kind := records[candidate].kind
		if kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
			continue
		}
		typeCandidates = append(typeCandidates, candidate)
	}
	if len(typeCandidates) > 1 {
		return ambiguousSchemaElementType(input, schemaComponentLocations(records, typeCandidates))
	}
	if len(typeCandidates) == 0 {
		return wrongKindSchemaElementType(input, schemaComponentLocations(records, candidates))
	}
	candidate := typeCandidates[0]
	if records[candidate].kind == ComponentKindComplexTypeDefinition {
		return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
			input.typeLoc,
			fmt.Sprintf("named complex type %q is not implemented %s", input.declaredType, complexTargetSuffix),
			input.version,
		)
	}
	if !simpleTypes[candidate].present {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(
			input.typeLoc,
			"element type resolution has an incomplete simple type result",
		)
	}
	return schemaElementTypeResult{
		present:      true,
		declaredType: input.declaredType,
		typeID:       records[candidate].id,
		hasTypeID:    true,
	}, nil
}

type schemaComplexTypeResult struct {
	present  bool
	particle Particle
}

func resolveSchemaComplexTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
) ([]schemaComplexTypeResult, error) {
	if len(simpleTypes) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "complex type resolution has incomplete simple type results")
	}
	results := make([]schemaComplexTypeResult, len(records))
	for index, record := range records {
		if record.complexType == nil {
			continue
		}
		if record.complexType.particle == nil {
			return nil, newSchemaBridgeInvariant(record.loc, "complex type resolution has no particle input")
		}
		alternatives := make([]Particle, 0, len(record.complexType.particle.alternatives))
		for _, input := range record.complexType.particle.alternatives {
			if input.typeInput == nil {
				return nil, newSchemaSyntaxUnsupported(
					input.loc,
					"local choice elements without declared types are not implemented",
				)
			}
			resolved, err := resolveSchemaScalarType(
				input.typeInput,
				records,
				byName,
				simpleTypes,
				"for local choice elements",
			)
			if err != nil {
				return nil, err
			}
			facts := &schemaElementParticle{
				loc:          input.loc,
				minOccurs:    1,
				maxOccurs:    1,
				name:         input.name,
				declaredType: resolved.declaredType,
				typeID:       resolved.typeID,
				hasTypeID:    resolved.hasTypeID,
			}
			alternatives = append(alternatives, ElementParticle{facts: facts})
		}
		choice := &schemaChoiceParticle{
			loc:          record.complexType.particle.loc,
			minOccurs:    record.complexType.particle.minOccurs,
			maxOccurs:    record.complexType.particle.maxOccurs,
			alternatives: alternatives,
		}
		results[index] = schemaComplexTypeResult{
			present:  true,
			particle: ChoiceParticle{facts: choice},
		}
	}
	return results, nil
}

func unresolvedSchemaElementType(input *schemaElementInput) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeUnresolvedCode,
		input.typeLoc,
		fmt.Sprintf("element type %q cannot be resolved", input.declaredType),
		nil,
		input.version,
		errSchemaElementTypeUnresolved,
	)
}

func wrongKindSchemaElementType(input *schemaElementInput, related []Loc) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeWrongKindCode,
		input.typeLoc,
		fmt.Sprintf("element type %q does not name a simple type", input.declaredType),
		related,
		input.version,
		fmt.Errorf("%w: %q", errSchemaElementTypeWrongKind, input.declaredType),
	)
}

func ambiguousSchemaElementType(input *schemaElementInput, related []Loc) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeAmbiguousCode,
		input.typeLoc,
		fmt.Sprintf("element type %q is ambiguous", input.declaredType),
		related,
		input.version,
		fmt.Errorf("%w: %q", errSchemaElementTypeAmbiguous, input.declaredType),
	)
}

func newSchemaElementTypeDiagnostic(code string, loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaElementTypeSpecRef(version),
		cause:   cause,
	}
}

func schemaElementTypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementTypeXSD10SpecRef
	}
	return schemaElementTypeXSD11SpecRef
}

func unsupportedSchemaElementPrecisionDecimal(input *schemaElementInput) error {
	feature, ok := LookupUnsupportedFeature(FeaturePrecisionDecimal)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			input.typeLoc,
			"precisionDecimal feature is not registered",
			nil,
		)
	}
	return newUnsupportedForVersion(
		feature,
		diagnosticSchemaElementTypeUnsupportedCode,
		input.typeLoc,
		fmt.Sprintf("element type %q is not implemented", input.declaredType),
		input.version,
	)
}

func (resolver *schemaSimpleTypeResolver) resolve(index int) (schemaSimpleTypeResult, error) {
	switch resolver.states[index] {
	case schemaSimpleTypeUnvisited:
	case schemaSimpleTypeResolved:
		return resolver.results[index], nil
	case schemaSimpleTypeVisiting:
		return schemaSimpleTypeResult{}, resolver.cycleDiagnostic(index)
	default:
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(
			resolver.records[index].loc,
			"simple type resolution has an unknown state",
		)
	}
	input := resolver.records[index].simpleType
	if input == nil {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(
			resolver.records[index].loc,
			"simple type resolution has no restriction input",
		)
	}
	resolver.states[index] = schemaSimpleTypeVisiting
	resolver.stack = append(resolver.stack, index)
	local, err := schemaDigitFacetDeclarations(input)
	if err != nil {
		return resolver.finishResolve(index, err)
	}
	base, err := resolver.resolveBase(index)
	if err != nil {
		return resolver.finishResolve(index, err)
	}
	baseFacets, err := reversionDigitFacets(base.facets, input.version)
	if err != nil {
		return resolver.finishResolve(index, err)
	}
	facets, err := RestrictDigitFacets(baseFacets, local)
	if err != nil {
		return resolver.finishResolve(index, err)
	}
	resolver.results[index] = schemaSimpleTypeResult{
		present:     true,
		base:        input.base,
		baseLoc:     input.baseLoc,
		baseID:      base.id,
		hasBaseID:   base.hasID,
		digitFacets: facets,
	}
	return resolver.finishResolve(index, nil)
}

func (resolver *schemaSimpleTypeResolver) finishResolve(index int, err error) (schemaSimpleTypeResult, error) {
	resolver.stack = resolver.stack[:len(resolver.stack)-1]
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	resolver.states[index] = schemaSimpleTypeResolved
	return resolver.results[index], nil
}

type schemaSimpleTypeBase struct {
	facets DigitFacets
	id     ComponentID
	hasID  bool
}

func (resolver *schemaSimpleTypeResolver) resolveBase(index int) (schemaSimpleTypeBase, error) {
	input := resolver.records[index].simpleType
	if input.base.Namespace() == xsdNamespaceURI {
		return resolveBuiltinSchemaSimpleTypeBase(input)
	}
	return resolver.resolveNamedSchemaSimpleTypeBase(input)
}

func resolveBuiltinSchemaSimpleTypeBase(input *schemaSimpleTypeInput) (schemaSimpleTypeBase, error) {
	switch input.base.Local() {
	case "integer":
		facets, err := NewIntegerDigitFacets(nil, input.version)
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		return schemaSimpleTypeBase{facets: facets}, nil
	case "decimal":
		facets, err := NewDecimalDigitFacets(nil, nil, input.version)
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		return schemaSimpleTypeBase{facets: facets}, nil
	case "precisionDecimal":
		return schemaSimpleTypeBase{}, unsupportedSchemaSimpleTypeBase(input.baseLoc, input.base)
	default:
		return schemaSimpleTypeBase{}, newSchemaSyntaxUnsupported(
			input.baseLoc,
			fmt.Sprintf("simple type restriction base %q is not supported", input.base),
		)
	}
}

func (resolver *schemaSimpleTypeResolver) resolveNamedSchemaSimpleTypeBase(input *schemaSimpleTypeInput) (schemaSimpleTypeBase, error) {
	candidates := resolver.byName[input.base]
	if len(candidates) == 0 {
		return unresolvedSchemaSimpleTypeBase(input)
	}
	simpleCandidates := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if resolver.records[candidate].kind == ComponentKindSimpleTypeDefinition {
			simpleCandidates = append(simpleCandidates, candidate)
		}
	}
	if len(simpleCandidates) == 0 {
		return wrongKindSchemaSimpleTypeBase(input, schemaComponentLocations(resolver.records, candidates))
	}
	if len(simpleCandidates) > 1 {
		return ambiguousSchemaSimpleTypeBase(input, schemaComponentLocations(resolver.records, simpleCandidates))
	}
	baseIndex := simpleCandidates[0]
	base, err := resolver.resolve(baseIndex)
	if err != nil {
		return schemaSimpleTypeBase{}, err
	}
	return schemaSimpleTypeBase{
		facets: base.digitFacets,
		id:     resolver.records[baseIndex].id,
		hasID:  true,
	}, nil
}

func unresolvedSchemaSimpleTypeBase(input *schemaSimpleTypeInput) (schemaSimpleTypeBase, error) {
	return schemaSimpleTypeBase{}, newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeUnresolvedCode,
		input.baseLoc,
		fmt.Sprintf("simple type restriction base %q cannot be resolved", input.base),
		nil,
		input.version,
		errSchemaSimpleTypeBaseUnresolved,
	)
}

func wrongKindSchemaSimpleTypeBase(input *schemaSimpleTypeInput, related []Loc) (schemaSimpleTypeBase, error) {
	return schemaSimpleTypeBase{}, newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeWrongKindCode,
		input.baseLoc,
		fmt.Sprintf("simple type restriction base %q does not name a simple type", input.base),
		related,
		input.version,
		fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseWrongKind, input.base),
	)
}

func ambiguousSchemaSimpleTypeBase(input *schemaSimpleTypeInput, related []Loc) (schemaSimpleTypeBase, error) {
	return schemaSimpleTypeBase{}, newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeAmbiguousCode,
		input.baseLoc,
		fmt.Sprintf("simple type restriction base %q is ambiguous", input.base),
		related,
		input.version,
		fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseAmbiguous, input.base),
	)
}

func schemaDigitFacetDeclarations(input *schemaSimpleTypeInput) (DigitFacetDeclarations, error) {
	var totalDigits *TotalDigitsFacet
	if input.totalDigits != nil {
		facet, err := ParseTotalDigitsFacetWithFixed(
			input.totalDigits.lexical,
			input.totalDigits.loc,
			input.totalDigits.fixed,
			input.version,
		)
		if err != nil {
			return DigitFacetDeclarations{}, err
		}
		totalDigits = &facet
	}
	var fractionDigits *FractionDigitsFacet
	if input.fractionDigits != nil {
		facet, err := ParseFractionDigitsFacetWithFixed(
			input.fractionDigits.lexical,
			input.fractionDigits.loc,
			input.fractionDigits.fixed,
			input.version,
		)
		if err != nil {
			return DigitFacetDeclarations{}, err
		}
		fractionDigits = &facet
	}
	return NewDigitFacetDeclarations(totalDigits, fractionDigits), nil
}

func (resolver *schemaSimpleTypeResolver) cycleDiagnostic(index int) error {
	loc := resolver.records[index].loc
	version := XSDVersion11
	input := resolver.records[index].simpleType
	if input != nil {
		loc = input.baseLoc
		version = input.version
	}
	start := 0
	for position, current := range resolver.stack {
		if current == index {
			start = position
			break
		}
	}
	related := make([]Loc, 0, len(resolver.stack)-start)
	for _, current := range resolver.stack[start:] {
		currentLoc := resolver.records[current].loc
		if resolver.records[current].simpleType != nil {
			currentLoc = resolver.records[current].simpleType.baseLoc
		}
		if currentLoc.IsZero() || currentLoc == loc {
			continue
		}
		related = append(related, currentLoc)
	}
	return newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeCycleCode,
		loc,
		"simple type restriction bases form a cycle",
		related,
		version,
		fmt.Errorf("%w: component %s", errSchemaSimpleTypeBaseCycle, resolver.records[index].id.Source()),
	)
}

func schemaComponentLocations(records []schemaComponentRecord, indices []int) []Loc {
	locations := make([]Loc, 0, len(indices))
	for _, index := range indices {
		if records[index].loc.IsZero() {
			continue
		}
		locations = append(locations, records[index].loc)
	}
	return locations
}

func newSchemaSimpleTypeDiagnostic(code string, loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaSimpleTypeSpecRef(version),
		cause:   cause,
	}
}

func schemaSimpleTypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaSimpleTypeXSD10SpecRef
	}
	return schemaSimpleTypeXSD11SpecRef
}

func unsupportedSchemaSimpleTypeBase(loc Loc, name QName) error {
	feature, ok := LookupUnsupportedFeature(FeaturePrecisionDecimal)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			"precisionDecimal feature is not registered",
			nil,
		)
	}
	return newUnsupported(
		feature,
		diagnosticSchemaSimpleTypeBaseCode,
		loc,
		fmt.Sprintf("simple type restriction base %q is not implemented", name),
	)
}

func newSchemaCompositionDiagnostic(loc Loc, message string) Diagnostic {
	return newDiagnostic(FailureInvalid, invalidSchemaCompositionCode, loc, message, nil)
}

func newSchemaBridgeInvariant(loc Loc, message string) Diagnostic {
	return newDiagnostic(FailureInternal, diagnosticSchemaBridgeInvariantCode, loc, message, nil)
}

func newSchemaSyntaxUnsupported(loc Loc, message string) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxFeatureCode,
			loc,
			"schema syntax feature is not registered",
			nil,
		)
	}
	return newUnsupported(feature, UnsupportedSchemaSyntaxCode, loc, message)
}

func newSchemaSyntaxUnsupportedForVersion(loc Loc, message string, version XSDVersion) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxFeatureCode,
			loc,
			"schema syntax feature is not registered",
			nil,
		)
	}
	return newUnsupportedForVersion(feature, UnsupportedSchemaSyntaxCode, loc, message, version)
}
