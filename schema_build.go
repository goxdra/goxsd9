package goxsd9

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	invalidSchemaTargetNamespaceCode            = "XSD3009"
	invalidSchemaCompositionCode                = "XSD3010"
	invalidSchemaDeclarationNameCode            = "XSD3011"
	diagnosticSchemaSimpleTypeUnresolvedCode    = "XSD3014"
	diagnosticSchemaSimpleTypeWrongKindCode     = "XSD3015"
	diagnosticSchemaSimpleTypeAmbiguousCode     = "XSD3016"
	diagnosticSchemaSimpleTypeCycleCode         = "XSD3017"
	diagnosticSchemaSimpleTypeBaseCode          = "XSD3018"
	diagnosticSchemaElementTypeUnresolvedCode   = "XSD3019"
	diagnosticSchemaElementTypeWrongKindCode    = "XSD3020"
	diagnosticSchemaElementTypeAmbiguousCode    = "XSD3021"
	diagnosticSchemaElementTypeUnsupportedCode  = "XSD3022"
	diagnosticSchemaElementDuplicateCode        = "XSD3023"
	diagnosticSchemaPrecisionDecimalVersionCode = "XSD3030"
	diagnosticSchemaAllOccurrenceVersionCode    = diagnosticSchemaPrecisionDecimalVersionCode
	diagnosticSchemaBridgeInvariantCode         = "GOXSD9025"
)

const (
	schemaSimpleTypeXSD10SpecRef       = "xsd10-structures#Simple_Type_Definitions"
	schemaSimpleTypeXSD11SpecRef       = "xsd11-structures#Simple_Type_Definition"
	schemaElementTypeXSD10SpecRef      = "xsd10-structures#Element_Declaration_details"
	schemaElementTypeXSD11SpecRef      = "xsd11-structures#Element_Declaration_details"
	schemaElementDuplicateXSD10SpecRef = "xsd10-structures#c-nmd"
	schemaElementDuplicateXSD11SpecRef = "xsd11-structures#c-nmd"
)

var (
	errSchemaSimpleTypeBaseUnresolved = errors.New("simple type base is unresolved")
	errSchemaSimpleTypeBaseWrongKind  = errors.New("simple type base has the wrong kind")
	errSchemaSimpleTypeBaseAmbiguous  = errors.New("simple type base is ambiguous")
	errSchemaSimpleTypeBaseCycle      = errors.New("simple type base is cyclic")
	errSchemaElementTypeUnresolved    = errors.New("element type is unresolved")
	errSchemaElementTypeWrongKind     = errors.New("element type has the wrong kind")
	errSchemaElementTypeAmbiguous     = errors.New("element type is ambiguous")
	errSchemaElementDuplicate         = errors.New("global element declaration is duplicated")
	errSchemaPrecisionDecimalVersion  = errors.New("precisionDecimal is unavailable in the selected XSD version policy")
	errLanguagePolicyMismatch         = errors.New("recognized XSD 1.1 behavior is outside the selected XSD 1.0 policy")
)

type schemaTargetNamespace struct {
	value   string
	present bool
}

// discoverSchema completes the internal pipeline used by ParseSchema.
func discoverSchema(root ResolvedSource, resolver Resolver) (Schema, error) {
	return discoverSchemaWithPolicy(root, resolver, Compatibility)
}

func discoverSchemaWithPolicy(root ResolvedSource, resolver Resolver, policy LanguagePolicy) (Schema, error) {
	discovery, err := discoverSyntaxWithPolicy(root, resolver, policy)
	if err != nil {
		return Schema{}, err
	}
	return newSchemaFromDiscoveryWithPolicy(discovery, policy)
}

func newSchemaFromDiscovery(discovery syntaxDiscoveryResult) (Schema, error) {
	return newSchemaFromDiscoveryWithPolicy(discovery, Compatibility)
}

func newSchemaFromDiscoveryWithPolicy(discovery syntaxDiscoveryResult, policy LanguagePolicy) (Schema, error) {
	namespaces, sourceIndices, err := schemaDiscoveryNamespacesWithPolicy(discovery.documents, policy)
	if err != nil {
		return Schema{}, err
	}

	err = validateSchemaComposition(discovery.edges, sourceIndices, namespaces)
	if err != nil {
		return Schema{}, err
	}

	version, err := xsdVersionForLanguagePolicy(policy)
	if err != nil {
		return Schema{}, invalidLanguagePolicyDiagnostic(policy, err)
	}
	inputs, err := schemaDocumentInputs(discovery.documents, namespaces, version)
	if err != nil {
		return Schema{}, err
	}

	schema, err := newSchemaWithPolicy(inputs, policy)
	if err != nil {
		return Schema{}, err
	}
	return schema, nil
}

func schemaDiscoveryNamespacesWithPolicy(documents []*syntaxDocument, policy LanguagePolicy) ([]schemaTargetNamespace, map[SourceID]int, error) {
	err := validateLanguagePolicy(policy)
	if err != nil {
		return nil, nil, invalidLanguagePolicyDiagnostic(policy, err)
	}
	namespaces := make([]schemaTargetNamespace, len(documents))
	sourceIndices := make(map[SourceID]int, len(documents))
	for index, document := range documents {
		if err := validateDiscoveredDocument(document, sourceIndices, policy); err != nil {
			return nil, nil, err
		}
		namespace, err := syntaxDocumentTargetNamespace(document)
		if err != nil {
			return nil, nil, err
		}
		sourceIndices[document.source] = index
		namespaces[index] = namespace
	}
	return namespaces, sourceIndices, nil
}

func validateDiscoveredDocument(document *syntaxDocument, sourceIndices map[SourceID]int, policy LanguagePolicy) error {
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
	return validateSyntaxDocumentStructureWithPolicy(document, policy)
}

func schemaDocumentInputs(documents []*syntaxDocument, namespaces []schemaTargetNamespace, version XSDVersion) ([]schemaDocumentInput, error) {
	inputs := make([]schemaDocumentInput, 0, len(documents))
	for index, document := range documents {
		declarations, err := schemaDocumentDeclarations(document, namespaces[index].value, version)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, schemaDocumentInput{
			source:          document.source,
			rootLoc:         document.root.loc,
			targetNamespace: namespaces[index].value,
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
		elementType, elementErr := schemaElementTypeInput(element)
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
	simpleType, err := schemaSimpleTypeRestrictionInput(element)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	declaration.simpleType = simpleType
	return declaration, true, nil
}

func schemaElementTypeInput(element *syntaxElement) (*schemaElementInput, error) {
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
	}, nil
}

//nolint:gocognit // Keep choice and sequence input construction together.
func schemaComplexTypeInputFromElement(element *syntaxElement, version XSDVersion) (*schemaComplexTypeInput, error) {
	var model *syntaxElement
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "choice" && child.name.local != "sequence" {
			continue
		}
		model = child
		break
	}
	if model == nil {
		return nil, nil
	}

	occurrences, err := schemaParticleOccurrenceRange(model, version)
	if err != nil {
		return nil, err
	}
	if model.name.local == "choice" {
		input := &schemaChoiceParticleInput{
			loc:          model.loc,
			occurrences:  occurrences,
			alternatives: make([]schemaElementParticleInput, 0),
		}
		for _, node := range model.children {
			child, ok := node.(*syntaxElement)
			if !ok || child.name.local != "element" {
				continue
			}
			alternative, err := schemaElementParticleInputFromElement(child, version)
			if err != nil {
				return nil, err
			}
			input.alternatives = append(input.alternatives, alternative)
		}
		return &schemaComplexTypeInput{particle: input}, nil
	}

	input := &schemaSequenceParticleInput{
		loc:         model.loc,
		occurrences: occurrences,
		elements:    make([]schemaElementParticleInput, 0),
	}
	for _, node := range model.children {
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
		input.elements = append(input.elements, alternative)
	}
	return &schemaComplexTypeInput{particle: input}, nil
}

func schemaElementParticleInputFromElement(element *syntaxElement, version XSDVersion) (schemaElementParticleInput, error) {
	occurrences, err := schemaParticleOccurrenceRange(element, version)
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	nameAttributes := syntaxAttributesByLocal(element, "name")
	if len(nameAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "local element input has an invalid name attribute")
	}
	local := collapseXMLWhitespace(nameAttributes[0].value)
	name, err := NewQName("", local)
	if err != nil {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(nameAttributes[0].loc, "construct local element name")
	}
	input := schemaElementParticleInput{
		loc:         element.loc,
		name:        name,
		occurrences: occurrences,
	}
	typeAttributes := syntaxAttributesByLocal(element, "type")
	if len(typeAttributes) == 0 {
		return input, nil
	}
	if len(typeAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "local element type attribute is not unique")
	}
	declaredType, err := expandSchemaQName(element, typeAttributes[0])
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	input.typeInput = &schemaElementInput{
		declaredType: declaredType,
		typeLoc:      typeAttributes[0].loc,
	}
	return input, nil
}

func schemaSimpleTypeRestrictionInput(element *syntaxElement) (*schemaSimpleTypeInput, error) {
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
	return schemaRestrictionInput(restriction)
}

func schemaRestrictionInput(element *syntaxElement) (*schemaSimpleTypeInput, error) {
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
		facets:  make([]schemaFacetInput, 0),
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if _, ok := schemaFacetKindFromName(child.name.local); !ok {
			continue
		}
		facet, err := schemaFacetInputFromElement(child)
		if err != nil {
			return nil, err
		}
		input.facets = append(input.facets, *facet)
	}
	return input, nil
}

func schemaFacetInputFromElement(element *syntaxElement) (*schemaFacetInput, error) {
	kind, ok := schemaFacetKindFromName(element.name.local)
	if !ok {
		return nil, newSchemaBridgeInvariant(element.loc, fmt.Sprintf("schema facet <%s> has an unknown kind", element.name.local))
	}
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
		lexical:  valueAttributes[0].value,
		loc:      element.loc,
		valueLoc: valueAttributes[0].loc,
		kind:     kind,
		fixed:    fixed,
	}, nil
}

func schemaFacetKindFromName(name string) (schemaFacetKind, bool) {
	switch name {
	case "totalDigits":
		return schemaFacetTotalDigits, true
	case "fractionDigits":
		return schemaFacetFractionDigits, true
	case "minScale":
		return schemaFacetMinScale, true
	case "maxScale":
		return schemaFacetMaxScale, true
	case "pattern":
		return schemaFacetPattern, true
	case "enumeration":
		return schemaFacetEnumeration, true
	case "minInclusive":
		return schemaFacetMinInclusive, true
	case "minExclusive":
		return schemaFacetMinExclusive, true
	case "maxInclusive":
		return schemaFacetMaxInclusive, true
	case "maxExclusive":
		return schemaFacetMaxExclusive, true
	case "whiteSpace":
		return schemaFacetWhiteSpace, true
	case "length":
		return schemaFacetLength, true
	case "minLength":
		return schemaFacetMinLength, true
	case "maxLength":
		return schemaFacetMaxLength, true
	case "precision":
		return schemaFacetPrecision, true
	case "explicitTimezone":
		return schemaFacetExplicitTimezone, true
	default:
		return 0, false
	}
}

func schemaFacetName(kind schemaFacetKind) string {
	switch kind {
	case schemaFacetTotalDigits:
		return "totalDigits"
	case schemaFacetFractionDigits:
		return "fractionDigits"
	case schemaFacetMinScale:
		return "minScale"
	case schemaFacetMaxScale:
		return "maxScale"
	case schemaFacetPattern:
		return "pattern"
	case schemaFacetEnumeration:
		return "enumeration"
	case schemaFacetMinInclusive:
		return "minInclusive"
	case schemaFacetMinExclusive:
		return "minExclusive"
	case schemaFacetMaxInclusive:
		return "maxInclusive"
	case schemaFacetMaxExclusive:
		return "maxExclusive"
	case schemaFacetWhiteSpace:
		return "whiteSpace"
	case schemaFacetLength:
		return "length"
	case schemaFacetMinLength:
		return "minLength"
	case schemaFacetMaxLength:
		return "maxLength"
	case schemaFacetPrecision:
		return "precision"
	case schemaFacetExplicitTimezone:
		return "explicitTimezone"
	default:
		return ""
	}
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
	present   bool
	base      QName
	baseLoc   Loc
	baseID    ComponentID
	hasBaseID bool
	facets    schemaSimpleTypeFacetVariant
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

func resolveSchemaSimpleTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	version XSDVersion,
) ([]schemaSimpleTypeResult, error) {
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
		if _, err := resolver.resolve(index, version); err != nil {
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
	complexTypes []schemaComplexTypeResult,
	version XSDVersion,
) ([]schemaElementTypeResult, error) {
	if len(simpleTypes) != len(records) || len(complexTypes) != len(records) {
		return nil, newSchemaBridgeInvariant(Loc{}, "element type resolution has incomplete type results")
	}
	results := make([]schemaElementTypeResult, len(records))
	for index, record := range records {
		if record.element == nil {
			continue
		}
		result, err := resolveSchemaElementType(record, records, byName, simpleTypes, complexTypes, version)
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
	complexTypes []schemaComplexTypeResult,
	version XSDVersion,
) (schemaElementTypeResult, error) {
	input := record.element
	if input == nil {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(record.loc, "element type resolution has no type input")
	}
	if input.declaredType.Namespace() == xsdNamespaceURI {
		return resolveSchemaScalarType(input, records, byName, simpleTypes, version, "for global elements", true)
	}

	candidates := byName[input.declaredType]
	if len(candidates) == 0 {
		return unresolvedSchemaElementType(input, version)
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
		return ambiguousSchemaElementType(input, schemaComponentLocations(records, typeCandidates), version)
	}
	if len(typeCandidates) == 0 {
		return wrongKindSchemaElementType(input, schemaComponentLocations(records, candidates), version)
	}
	candidate := typeCandidates[0]
	if records[candidate].kind == ComponentKindComplexTypeDefinition {
		if !complexTypes[candidate].present {
			return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
				input.typeLoc,
				fmt.Sprintf("named complex type %q is not implemented for global elements", input.declaredType),
				version,
			)
		}
		return schemaElementTypeResult{
			present:      true,
			declaredType: input.declaredType,
			typeID:       records[candidate].id,
			hasTypeID:    true,
		}, nil
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

func resolveSchemaScalarType(
	input *schemaElementInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
	complexTargetSuffix string,
	allowPrecisionDecimal bool,
) (schemaElementTypeResult, error) {
	if input.declaredType.Namespace() == xsdNamespaceURI {
		return resolveBuiltinSchemaScalarType(input, version, allowPrecisionDecimal)
	}

	candidates := byName[input.declaredType]
	if len(candidates) == 0 {
		return unresolvedSchemaElementType(input, version)
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
		return ambiguousSchemaElementType(input, schemaComponentLocations(records, typeCandidates), version)
	}
	if len(typeCandidates) == 0 {
		return wrongKindSchemaElementType(input, schemaComponentLocations(records, candidates), version)
	}
	candidate := typeCandidates[0]
	if records[candidate].kind == ComponentKindComplexTypeDefinition {
		return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
			input.typeLoc,
			fmt.Sprintf("named complex type %q is not implemented %s", input.declaredType, complexTargetSuffix),
			version,
		)
	}
	if !simpleTypes[candidate].present {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(
			input.typeLoc,
			"element type resolution has an incomplete simple type result",
		)
	}
	if err := rejectSequencePrecisionDecimalType(input, simpleTypes[candidate], version, allowPrecisionDecimal); err != nil {
		return schemaElementTypeResult{}, err
	}
	return schemaElementTypeResult{
		present:      true,
		declaredType: input.declaredType,
		typeID:       records[candidate].id,
		hasTypeID:    true,
	}, nil
}

func resolveBuiltinSchemaScalarType(input *schemaElementInput, version XSDVersion, allowPrecisionDecimal bool) (schemaElementTypeResult, error) {
	switch input.declaredType.Local() {
	case "integer", "decimal":
		return schemaElementTypeResult{
			present:      true,
			declaredType: input.declaredType,
		}, nil
	case "precisionDecimal":
		if version == XSDVersion10 {
			return schemaElementTypeResult{}, precisionDecimalSchemaVersionDiagnostic(input.typeLoc, input.declaredType)
		}
		if !allowPrecisionDecimal {
			return schemaElementTypeResult{}, unsupportedSequencePrecisionDecimal(input, version)
		}
		return schemaElementTypeResult{
			present:      true,
			declaredType: input.declaredType,
		}, nil
	default:
		return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
			input.typeLoc,
			fmt.Sprintf("element type %q is not implemented", input.declaredType),
			version,
		)
	}
}

func rejectSequencePrecisionDecimalType(input *schemaElementInput, simpleType schemaSimpleTypeResult, version XSDVersion, allowPrecisionDecimal bool) error {
	if allowPrecisionDecimal {
		return nil
	}
	if _, ok := simpleType.facets.(schemaPrecisionDecimalFacetVariant); !ok {
		return nil
	}
	return unsupportedSequencePrecisionDecimal(input, version)
}

func unsupportedSequencePrecisionDecimal(input *schemaElementInput, version XSDVersion) Diagnostic {
	return newSchemaSyntaxUnsupportedForVersion(
		input.typeLoc,
		fmt.Sprintf("element type %q is not implemented for local sequence elements", input.declaredType),
		version,
	)
}

type schemaComplexTypeResult struct {
	present  bool
	particle Particle
}

func resolveSchemaComplexTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
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
		particle, err := resolveSchemaComplexTypeParticle(
			record.complexType.particle,
			records,
			byName,
			simpleTypes,
			version,
		)
		if err != nil {
			return nil, err
		}
		results[index] = schemaComplexTypeResult{
			present:  true,
			particle: particle,
		}
	}
	return results, nil
}

func resolveSchemaComplexTypeParticle(
	input schemaComplexTypeParticleInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) (Particle, error) {
	switch particle := input.(type) {
	case *schemaChoiceParticleInput:
		if particle == nil {
			return nil, newSchemaBridgeInvariant(Loc{}, "choice particle input is nil")
		}
		return resolveSchemaChoiceParticle(particle, records, byName, simpleTypes, version)
	case *schemaSequenceParticleInput:
		if particle == nil {
			return nil, newSchemaBridgeInvariant(Loc{}, "sequence particle input is nil")
		}
		return resolveSchemaSequenceParticle(particle, records, byName, simpleTypes, version)
	default:
		return nil, newSchemaBridgeInvariant(Loc{}, "complex type has an unknown particle input")
	}
}

func resolveSchemaChoiceParticle(
	input *schemaChoiceParticleInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) (Particle, error) {
	alternatives := make([]Particle, 0, len(input.alternatives))
	for _, elementInput := range input.alternatives {
		element, err := resolveSchemaElementParticle(
			elementInput,
			records,
			byName,
			simpleTypes,
			version,
			"choice",
		)
		if err != nil {
			return nil, err
		}
		if element == nil {
			continue
		}
		alternatives = append(alternatives, element)
	}
	choice := &schemaChoiceParticle{
		loc:          input.loc,
		occurrences:  input.occurrences.clone(),
		alternatives: alternatives,
	}
	return ChoiceParticle{facts: choice}, nil
}

func resolveSchemaSequenceParticle(
	input *schemaSequenceParticleInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
) (Particle, error) {
	if !input.occurrences.mapsToParticle() {
		return nil, nil
	}
	elements := make([]ElementParticle, 0, len(input.elements))
	for _, elementInput := range input.elements {
		element, err := resolveSchemaElementParticle(
			elementInput,
			records,
			byName,
			simpleTypes,
			version,
			"sequence",
		)
		if err != nil {
			return nil, err
		}
		if element == nil {
			continue
		}
		resolved, ok := element.(ElementParticle)
		if !ok {
			return nil, newSchemaBridgeInvariant(input.loc, "sequence element resolution produced a non-element particle")
		}
		elements = append(elements, resolved)
	}
	sequence := &schemaSequenceParticle{
		loc:         input.loc,
		occurrences: input.occurrences.clone(),
		elements:    elements,
	}
	return SequenceParticle{facts: sequence}, nil
}

func resolveSchemaElementParticle(
	input schemaElementParticleInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
	model string,
) (Particle, error) {
	if !input.occurrences.mapsToParticle() {
		return nil, nil
	}
	if input.typeInput == nil {
		return nil, newSchemaSyntaxUnsupported(
			input.loc,
			fmt.Sprintf("local %s elements without declared types are not implemented", model),
		)
	}
	resolved, err := resolveSchemaScalarType(
		input.typeInput,
		records,
		byName,
		simpleTypes,
		version,
		"for local "+model+" elements",
		model != "sequence",
	)
	if err != nil {
		return nil, err
	}
	facts := &schemaElementParticle{
		loc:          input.loc,
		occurrences:  input.occurrences.clone(),
		name:         input.name,
		declaredType: resolved.declaredType,
		typeID:       resolved.typeID,
		hasTypeID:    resolved.hasTypeID,
	}
	return ElementParticle{facts: facts}, nil
}

func unresolvedSchemaElementType(input *schemaElementInput, version XSDVersion) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeUnresolvedCode,
		input.typeLoc,
		fmt.Sprintf("element type %q cannot be resolved", input.declaredType),
		nil,
		version,
		errSchemaElementTypeUnresolved,
	)
}

func newSchemaElementDuplicateDiagnostic(later, earliest schemaComponentRecord, version XSDVersion) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaElementDuplicateCode,
		loc:     later.loc,
		message: fmt.Sprintf("global element declaration %q is duplicated", later.name),
		related: []Loc{earliest.loc},
		specRef: schemaElementDuplicateSpecRef(version),
		cause:   errSchemaElementDuplicate,
	}
}

func schemaElementDuplicateSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaElementDuplicateXSD10SpecRef
	}
	return schemaElementDuplicateXSD11SpecRef
}

func wrongKindSchemaElementType(input *schemaElementInput, related []Loc, version XSDVersion) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeWrongKindCode,
		input.typeLoc,
		fmt.Sprintf("element type %q does not name a simple type", input.declaredType),
		related,
		version,
		fmt.Errorf("%w: %q", errSchemaElementTypeWrongKind, input.declaredType),
	)
}

func ambiguousSchemaElementType(input *schemaElementInput, related []Loc, version XSDVersion) (schemaElementTypeResult, error) {
	return schemaElementTypeResult{}, newSchemaElementTypeDiagnostic(
		diagnosticSchemaElementTypeAmbiguousCode,
		input.typeLoc,
		fmt.Sprintf("element type %q is ambiguous", input.declaredType),
		related,
		version,
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

func (resolver *schemaSimpleTypeResolver) resolve(index int, version XSDVersion) (schemaSimpleTypeResult, error) {
	switch resolver.states[index] {
	case schemaSimpleTypeUnvisited:
	case schemaSimpleTypeResolved:
		return resolver.results[index], nil
	case schemaSimpleTypeVisiting:
		return schemaSimpleTypeResult{}, resolver.cycleDiagnostic(index, version)
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
	base, err := resolver.resolveBase(index, version)
	if err != nil {
		return resolver.finishResolve(index, err)
	}
	facets, err := restrictSchemaSimpleTypeFacets(base.facets, input.facets, version)
	if err != nil {
		return resolver.finishResolve(index, err)
	}
	resolver.results[index] = schemaSimpleTypeResult{
		present:   true,
		base:      input.base,
		baseLoc:   input.baseLoc,
		baseID:    base.id,
		hasBaseID: base.hasID,
		facets:    facets,
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
	facets schemaSimpleTypeFacetVariant
	id     ComponentID
	hasID  bool
}

func (resolver *schemaSimpleTypeResolver) resolveBase(index int, version XSDVersion) (schemaSimpleTypeBase, error) {
	input := resolver.records[index].simpleType
	if input.base.Namespace() == xsdNamespaceURI {
		return resolveBuiltinSchemaSimpleTypeBase(input, version)
	}
	return resolver.resolveNamedSchemaSimpleTypeBase(input, version)
}

func resolveBuiltinSchemaSimpleTypeBase(input *schemaSimpleTypeInput, version XSDVersion) (schemaSimpleTypeBase, error) {
	switch input.base.Local() {
	case "integer":
		facets, err := NewIntegerDigitFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		bounds, err := NewIntegerBoundFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		return schemaSimpleTypeBase{facets: schemaDigitFacetVariant{value: facets, integerBounds: bounds}}, nil
	case "decimal":
		facets, err := NewDecimalDigitFacets(nil, nil, version)
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		bounds, err := NewDecimalBoundFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		return schemaSimpleTypeBase{facets: schemaDigitFacetVariant{value: facets, decimalBounds: bounds}}, nil
	case "precisionDecimal":
		if version == XSDVersion10 {
			return schemaSimpleTypeBase{}, precisionDecimalSchemaVersionDiagnostic(input.baseLoc, input.base)
		}
		facets, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{})
		if err != nil {
			return schemaSimpleTypeBase{}, err
		}
		return schemaSimpleTypeBase{facets: schemaPrecisionDecimalFacetVariant{value: facets}}, nil
	default:
		return schemaSimpleTypeBase{}, newSchemaSyntaxUnsupported(
			input.baseLoc,
			fmt.Sprintf("simple type restriction base %q is not supported", input.base),
		)
	}
}

func precisionDecimalSchemaVersionDiagnostic(loc Loc, name QName) Diagnostic {
	return newXSD11FeatureMismatchAtReference(
		FeatureDatatypeFacets,
		diagnosticSchemaPrecisionDecimalVersionCode,
		loc,
		fmt.Sprintf("precisionDecimal type %q is not available under the selected XSD 1.0 policy", name),
		"xsd11-datatypes#dt-primitive",
		fmt.Errorf("%w: %q", errSchemaPrecisionDecimalVersion, name),
	)
}

func (resolver *schemaSimpleTypeResolver) resolveNamedSchemaSimpleTypeBase(input *schemaSimpleTypeInput, version XSDVersion) (schemaSimpleTypeBase, error) {
	candidates := resolver.byName[input.base]
	if len(candidates) == 0 {
		return unresolvedSchemaSimpleTypeBase(input, version)
	}
	simpleCandidates := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if resolver.records[candidate].kind == ComponentKindSimpleTypeDefinition {
			simpleCandidates = append(simpleCandidates, candidate)
		}
	}
	if len(simpleCandidates) == 0 {
		return wrongKindSchemaSimpleTypeBase(input, schemaComponentLocations(resolver.records, candidates), version)
	}
	if len(simpleCandidates) > 1 {
		return ambiguousSchemaSimpleTypeBase(input, schemaComponentLocations(resolver.records, simpleCandidates), version)
	}
	baseIndex := simpleCandidates[0]
	base, err := resolver.resolve(baseIndex, version)
	if err != nil {
		return schemaSimpleTypeBase{}, err
	}
	return schemaSimpleTypeBase{
		facets: base.facets,
		id:     resolver.records[baseIndex].id,
		hasID:  true,
	}, nil
}

func unresolvedSchemaSimpleTypeBase(input *schemaSimpleTypeInput, version XSDVersion) (schemaSimpleTypeBase, error) {
	return schemaSimpleTypeBase{}, newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeUnresolvedCode,
		input.baseLoc,
		fmt.Sprintf("simple type restriction base %q cannot be resolved", input.base),
		nil,
		version,
		errSchemaSimpleTypeBaseUnresolved,
	)
}

func wrongKindSchemaSimpleTypeBase(input *schemaSimpleTypeInput, related []Loc, version XSDVersion) (schemaSimpleTypeBase, error) {
	return schemaSimpleTypeBase{}, newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeWrongKindCode,
		input.baseLoc,
		fmt.Sprintf("simple type restriction base %q does not name a simple type", input.base),
		related,
		version,
		fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseWrongKind, input.base),
	)
}

func ambiguousSchemaSimpleTypeBase(input *schemaSimpleTypeInput, related []Loc, version XSDVersion) (schemaSimpleTypeBase, error) {
	return schemaSimpleTypeBase{}, newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeAmbiguousCode,
		input.baseLoc,
		fmt.Sprintf("simple type restriction base %q is ambiguous", input.base),
		related,
		version,
		fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseAmbiguous, input.base),
	)
}

//nolint:gocognit // Keep numeric and precision facet construction in one phase.
func restrictSchemaSimpleTypeFacets(
	base schemaSimpleTypeFacetVariant,
	inputs []schemaFacetInput,
	version XSDVersion,
) (schemaSimpleTypeFacetVariant, error) {
	switch typed := base.(type) {
	case schemaDigitFacetVariant:
		local, err := schemaDigitFacetDeclarations(inputs, typed.value.Kind(), version)
		if err != nil {
			return nil, err
		}
		facets, err := RestrictDigitFacets(typed.value, local.digit)
		if err != nil {
			return nil, err
		}
		variant := schemaDigitFacetVariant{value: facets}
		switch facets.Kind() {
		case DigitDatatypeInteger:
			bounds, boundErr := RestrictIntegerBoundFacets(typed.integerBounds, local.integerBounds)
			if boundErr != nil {
				return nil, boundErr
			}
			variant.integerBounds = bounds
		case DigitDatatypeDecimal:
			bounds, boundErr := RestrictDecimalBoundFacets(typed.decimalBounds, local.decimalBounds)
			if boundErr != nil {
				return nil, boundErr
			}
			variant.decimalBounds = bounds
		default:
			return nil, newSchemaBridgeInvariant(Loc{}, "simple type facet resolution has an unknown digit datatype")
		}
		return variant, nil
	case schemaPrecisionDecimalFacetVariant:
		local, err := schemaPrecisionDecimalFacetDeclarations(inputs)
		if err != nil {
			return nil, err
		}
		facets, err := RestrictPrecisionDecimalFacets(typed.value, local)
		if err != nil {
			return nil, err
		}
		return schemaPrecisionDecimalFacetVariant{value: facets}, nil
	default:
		return nil, newSchemaBridgeInvariant(Loc{}, "simple type facet resolution has an unknown datatype variant")
	}
}

type schemaDigitFacetDeclarationSet struct {
	digit         DigitFacetDeclarations
	integerBounds IntegerBoundFacetDeclarations
	decimalBounds DecimalBoundFacetDeclarations
}

//nolint:gocognit // Keep the supported numeric facet parsers in one lexical pass.
func schemaDigitFacetDeclarations(inputs []schemaFacetInput, kind DigitDatatype, version XSDVersion) (schemaDigitFacetDeclarationSet, error) {
	var totalDigits *TotalDigitsFacet
	var fractionDigits *FractionDigitsFacet
	integerBounds := make([]IntegerBoundFacet, 0)
	decimalBounds := make([]DecimalBoundFacet, 0)
	if kind != DigitDatatypeInteger && kind != DigitDatatypeDecimal {
		return schemaDigitFacetDeclarationSet{}, newSchemaBridgeInvariant(Loc{}, "simple type facet collection has an unknown digit datatype")
	}
	for _, input := range inputs {
		loc := schemaFacetValueLocation(input)
		switch input.kind {
		case schemaFacetTotalDigits:
			facet, err := ParseTotalDigitsFacetWithFixed(input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaDigitFacetDeclarationSet{}, err
			}
			totalDigits = &facet
		case schemaFacetFractionDigits:
			facet, err := ParseFractionDigitsFacetWithFixed(input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaDigitFacetDeclarationSet{}, err
			}
			fractionDigits = &facet
		case schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive:
			boundKind, ok := schemaBoundKindFromFacet(input.kind)
			if !ok {
				return schemaDigitFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "simple type bound facet has an unknown kind")
			}
			if kind == DigitDatatypeInteger {
				facet, err := ParseIntegerBoundFacetWithFixed(boundKind, input.lexical, loc, input.fixed, version)
				if err != nil {
					return schemaDigitFacetDeclarationSet{}, err
				}
				integerBounds = append(integerBounds, facet)
				continue
			}
			facet, err := ParseDecimalBoundFacetWithFixed(boundKind, input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaDigitFacetDeclarationSet{}, err
			}
			decimalBounds = append(decimalBounds, facet)
		case schemaFacetMinScale, schemaFacetMaxScale, schemaFacetPattern, schemaFacetEnumeration,
			schemaFacetWhiteSpace, schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength,
			schemaFacetPrecision, schemaFacetExplicitTimezone:
			return schemaDigitFacetDeclarationSet{}, unsupportedSchemaDatatypeFacet(input, version)
		default:
			return schemaDigitFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "simple type facet has an unknown kind")
		}
	}
	return schemaDigitFacetDeclarationSet{
		digit:         NewDigitFacetDeclarations(totalDigits, fractionDigits),
		integerBounds: NewIntegerBoundFacetDeclarations(integerBounds),
		decimalBounds: NewDecimalBoundFacetDeclarations(decimalBounds),
	}, nil
}

func schemaBoundKindFromFacet(kind schemaFacetKind) (BoundKind, bool) {
	switch kind {
	case schemaFacetMinInclusive:
		return BoundMinInclusive, true
	case schemaFacetMinExclusive:
		return BoundMinExclusive, true
	case schemaFacetMaxInclusive:
		return BoundMaxInclusive, true
	case schemaFacetMaxExclusive:
		return BoundMaxExclusive, true
	case schemaFacetTotalDigits, schemaFacetFractionDigits, schemaFacetMinScale, schemaFacetMaxScale,
		schemaFacetPattern, schemaFacetEnumeration, schemaFacetWhiteSpace, schemaFacetLength,
		schemaFacetMinLength, schemaFacetMaxLength, schemaFacetPrecision, schemaFacetExplicitTimezone:
		return 0, false
	default:
		return 0, false
	}
}

//nolint:funlen // Keep the facet-kind to parser mapping explicit and located.
func schemaPrecisionDecimalFacetDeclarations(inputs []schemaFacetInput) (PrecisionDecimalFacetDeclarations, error) {
	var totalDigits *PrecisionDecimalTotalDigitsFacet
	var minScale *PrecisionDecimalMinScaleFacet
	var maxScale *PrecisionDecimalMaxScaleFacet
	var whiteSpace *PrecisionDecimalWhiteSpaceFacet
	var patterns []PrecisionDecimalPatternFacet
	var enumeration []PrecisionDecimalEnumerationFacet
	var minInclusive *PrecisionDecimalMinInclusiveFacet
	var minExclusive *PrecisionDecimalMinExclusiveFacet
	var maxInclusive *PrecisionDecimalMaxInclusiveFacet
	var maxExclusive *PrecisionDecimalMaxExclusiveFacet
	for _, input := range inputs {
		loc := schemaFacetValueLocation(input)
		var err error
		switch input.kind {
		case schemaFacetTotalDigits:
			facet, parseErr := ParsePrecisionDecimalTotalDigitsFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			totalDigits = &facet
		case schemaFacetMinScale:
			facet, parseErr := ParsePrecisionDecimalMinScaleFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			minScale = &facet
		case schemaFacetMaxScale:
			facet, parseErr := ParsePrecisionDecimalMaxScaleFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			maxScale = &facet
		case schemaFacetPattern:
			facet, parseErr := ParsePrecisionDecimalPatternFacet(input.lexical, loc)
			err = parseErr
			patterns = append(patterns, facet)
		case schemaFacetEnumeration:
			facet, parseErr := ParsePrecisionDecimalEnumerationFacet(input.lexical, loc)
			err = parseErr
			enumeration = append(enumeration, facet)
		case schemaFacetMinInclusive:
			facet, parseErr := ParsePrecisionDecimalMinInclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			minInclusive = &facet
		case schemaFacetMinExclusive:
			facet, parseErr := ParsePrecisionDecimalMinExclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			minExclusive = &facet
		case schemaFacetMaxInclusive:
			facet, parseErr := ParsePrecisionDecimalMaxInclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			maxInclusive = &facet
		case schemaFacetMaxExclusive:
			facet, parseErr := ParsePrecisionDecimalMaxExclusiveFacetWithFixed(input.lexical, loc, input.fixed)
			err = parseErr
			maxExclusive = &facet
		case schemaFacetWhiteSpace:
			facet, parseErr := ParsePrecisionDecimalWhiteSpaceFacet(input.lexical, loc)
			facet.fixed = input.fixed
			err = parseErr
			whiteSpace = &facet
		case schemaFacetFractionDigits, schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength, schemaFacetPrecision, schemaFacetExplicitTimezone:
			return PrecisionDecimalFacetDeclarations{}, ValidatePrecisionDecimalFacetName(schemaFacetName(input.kind), loc)
		default:
			return PrecisionDecimalFacetDeclarations{}, ValidatePrecisionDecimalFacetName(schemaFacetName(input.kind), loc)
		}
		if err != nil {
			return PrecisionDecimalFacetDeclarations{}, err
		}
	}
	return NewPrecisionDecimalFacetDeclarationsAll(
		totalDigits,
		minScale,
		maxScale,
		patterns,
		enumeration,
		minInclusive,
		minExclusive,
		maxInclusive,
		maxExclusive,
		whiteSpace,
	), nil
}

func schemaFacetValueLocation(input schemaFacetInput) Loc {
	if !input.valueLoc.IsZero() {
		return input.valueLoc
	}
	return input.loc
}

func unsupportedSchemaDatatypeFacet(input schemaFacetInput, version XSDVersion) error {
	if err := validateOrdinarySchemaFacetInput(input); err != nil {
		return err
	}
	if version == XSDVersion10 && isXSD11SimpleTypeFacet(schemaFacetName(input.kind)) {
		return newXSD11FeatureMismatch(
			FeatureDatatypeFacets,
			UnsupportedDatatypeFacetCode,
			input.loc,
			fmt.Sprintf("simple type restriction facet <%s> is an XSD 1.1-only construct", schemaFacetName(input.kind)),
		)
	}
	feature, ok := LookupUnsupportedFeature(FeatureDatatypeFacets)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			schemaFacetValueLocation(input),
			"datatype facet feature is not registered",
			nil,
		)
	}
	return newUnsupportedForVersion(
		feature,
		UnsupportedDatatypeFacetCode,
		input.loc,
		fmt.Sprintf("simple type restriction facet <%s> is not implemented for this datatype", schemaFacetName(input.kind)),
		version,
	)
}

func validateOrdinarySchemaFacetInput(input schemaFacetInput) error {
	valueLoc := schemaFacetValueLocation(input)
	switch input.kind {
	case schemaFacetMinScale, schemaFacetMaxScale:
		value, err := ParseStrictInteger(collapseXMLWhitespace(input.lexical), valueLoc)
		if err != nil {
			return err
		}
		if value.Sign() < 0 {
			return newSchemaCompositionDiagnostic(valueLoc, schemaFacetName(input.kind)+" facet value must be non-negative")
		}
	case schemaFacetPrecision:
		value, err := ParseStrictInteger(collapseXMLWhitespace(input.lexical), valueLoc)
		if err != nil {
			return err
		}
		if value.Sign() <= 0 {
			return newSchemaCompositionDiagnostic(valueLoc, "precision facet value must be positive")
		}
	case schemaFacetTotalDigits, schemaFacetFractionDigits, schemaFacetPattern, schemaFacetEnumeration,
		schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive,
		schemaFacetWhiteSpace, schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength,
		schemaFacetExplicitTimezone:
		return nil
	default:
		return newSchemaBridgeInvariant(input.loc, "simple type facet has an unknown kind")
	}
	return nil
}

func (resolver *schemaSimpleTypeResolver) cycleDiagnostic(index int, version XSDVersion) error {
	loc := resolver.records[index].loc
	input := resolver.records[index].simpleType
	if input != nil {
		loc = input.baseLoc
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

func newXSD11FeatureMismatch(featureID FeatureID, code string, loc Loc, message string) Diagnostic {
	return newXSD11FeatureMismatchAtReference(featureID, code, loc, message, "", nil)
}

func newXSD11FeatureMismatchAtReference(featureID FeatureID, code string, loc Loc, message, specRef string, cause error) Diagnostic {
	feature, ok := LookupUnsupportedFeature(featureID)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			fmt.Sprintf("unsupported diagnostic references unregistered feature %q", featureID),
			cause,
		)
	}
	mismatchCause := fmt.Errorf("%w: feature %q is not available in the selected XSD 1.0 profile", errLanguagePolicyMismatch, featureID)
	if cause != nil {
		cause = errors.Join(cause, mismatchCause)
	}
	if cause == nil {
		cause = mismatchCause
	}
	diagnostic := newUnsupportedForVersionWithCause(feature, code, loc, message, XSDVersion11, cause)
	if diagnostic.Class() != FailureUnsupported || specRef == "" {
		return diagnostic
	}
	diagnostic.specRef = specRef
	return diagnostic
}
