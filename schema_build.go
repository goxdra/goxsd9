package goxsd9

import (
	"errors"
	"fmt"
	"strings"
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
	diagnosticSchemaGlobalDuplicateCode         = "XSD3023"
	diagnosticSchemaElementDuplicateCode        = diagnosticSchemaGlobalDuplicateCode
	diagnosticSchemaPrecisionDecimalVersionCode = "XSD3030"
	diagnosticSchemaAllOccurrenceVersionCode    = diagnosticSchemaPrecisionDecimalVersionCode
	diagnosticSchemaBridgeInvariantCode         = "GOXSD9025"
)

const (
	schemaSimpleTypeXSD10SpecRef             = "xsd10-structures#Simple_Type_Definitions"
	schemaSimpleTypeXSD11SpecRef             = "xsd11-structures#Simple_Type_Definition"
	schemaElementTypeXSD10SpecRef            = "xsd10-structures#Element_Declaration_details"
	schemaElementTypeXSD11SpecRef            = "xsd11-structures#Element_Declaration_details"
	schemaElementTargetNamespaceXSD11SpecRef = "xsd11-structures#dcl.elt.local"
	schemaBooleanDatatypeXSD10SpecRef        = "xsd10-datatypes#boolean"
	schemaBooleanDatatypeXSD11SpecRef        = "xsd11-datatypes#boolean"
	schemaGlobalDuplicateXSD10SpecRef        = "xsd10-structures#c-nmd"
	schemaGlobalDuplicateXSD11SpecRef        = "xsd11-structures#c-nmd"
	schemaElementDuplicateXSD10SpecRef       = schemaGlobalDuplicateXSD10SpecRef
	schemaElementDuplicateXSD11SpecRef       = schemaGlobalDuplicateXSD11SpecRef
)

var (
	errSchemaSimpleTypeBaseUnresolved         = errors.New("simple type base is unresolved")
	errSchemaSimpleTypeBaseWrongKind          = errors.New("simple type base has the wrong kind")
	errSchemaSimpleTypeBaseAmbiguous          = errors.New("simple type base is ambiguous")
	errSchemaSimpleTypeBaseCycle              = errors.New("simple type base is cyclic")
	errSchemaSimpleTypeInvalidDerivation      = errors.New("simple type derivation is invalid")
	errSchemaSimpleTypeRestrictionUnsupported = errors.New("simple type restriction variety is not implemented")
	errSchemaElementTypeUnresolved            = errors.New("element type is unresolved")
	errSchemaElementTypeWrongKind             = errors.New("element type has the wrong kind")
	errSchemaElementTypeAmbiguous             = errors.New("element type is ambiguous")
	errSchemaGlobalDeclarationDuplicate       = errors.New("global declaration is duplicated")
	errSchemaElementDuplicate                 = errors.New("global element declaration is duplicated")
	errSchemaElementTargetNamespace           = errors.New("local element targetNamespace is not representable in the supported direct-choice model")
	errSchemaPrecisionDecimalVersion          = errors.New("precisionDecimal is unavailable in the selected XSD version policy")
	errLanguagePolicyMismatch                 = errors.New("recognized XSD 1.1 behavior is outside the selected XSD 1.0 policy")
)

type schemaTargetNamespace struct {
	value   string
	present bool
	loc     Loc
}

type schemaDocumentFacts struct {
	targetNamespace             schemaTargetNamespace
	elementFormDefaultQualified bool
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
		elementFormDefaultQualified, err := syntaxDocumentElementFormDefault(document)
		if err != nil {
			return nil, err
		}
		facts := schemaDocumentFacts{
			targetNamespace:             namespaces[index],
			elementFormDefaultQualified: elementFormDefaultQualified,
		}
		declarations, err := schemaDocumentDeclarationsWithFacts(document, facts, version)
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

func syntaxDocumentElementFormDefault(document *syntaxDocument) (bool, error) {
	if document == nil || document.root == nil {
		return false, newSchemaBridgeInvariant(Loc{}, "schema document has no root while reading elementFormDefault")
	}
	attributes := syntaxAttributesByLocal(document.root, "elementFormDefault")
	if len(attributes) == 0 {
		return false, nil
	}
	if len(attributes) != 1 {
		return false, newSchemaCompositionDiagnostic(attributes[1].loc, "attribute \"elementFormDefault\" must be unique")
	}
	if err := validateSchemaEnum(attributes[0], "qualified", "unqualified"); err != nil {
		return false, err
	}
	return collapseXMLWhitespace(attributes[0].value) == "qualified", nil
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
		loc:     attribute.loc,
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

func schemaDocumentDeclarationsWithFacts(document *syntaxDocument, facts schemaDocumentFacts, version XSDVersion) ([]schemaComponentInput, error) {
	declarations := make([]schemaComponentInput, 0)
	for _, node := range document.root.children {
		declaration, present, err := schemaDocumentDeclarationWithFacts(node, facts, version)
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

func schemaDocumentDeclaration(node syntaxNode, targetNamespace string, version XSDVersion) (schemaComponentInput, bool, error) {
	facts := schemaDocumentFacts{
		targetNamespace: schemaTargetNamespace{value: targetNamespace, present: targetNamespace != ""},
	}
	return schemaDocumentDeclarationWithFacts(node, facts, version)
}

func schemaDocumentDeclarationWithFacts(node syntaxNode, facts schemaDocumentFacts, version XSDVersion) (schemaComponentInput, bool, error) {
	element, kind, present, err := schemaDocumentDeclarationSyntax(node)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	if !present {
		return schemaComponentInput{}, false, nil
	}
	declaration, err := schemaDocumentDeclarationInput(element, kind, facts, version)
	if err != nil {
		return schemaComponentInput{}, false, err
	}
	return declaration, true, nil
}

func schemaDocumentDeclarationSyntax(node syntaxNode) (*syntaxElement, ComponentKind, bool, error) {
	textNode, ok := node.(syntaxText)
	if ok {
		if xmlWhitespace([]byte(textNode.data)) {
			return nil, "", false, nil
		}
		return nil, "", false, newDiagnostic(
			FailureInvalid,
			invalidSchemaCompositionCode,
			textNode.loc,
			"schema root contains non-whitespace character data",
			nil,
		)
	}
	element, ok := node.(*syntaxElement)
	if !ok {
		return nil, "", false, newSchemaBridgeInvariant(Loc{}, "schema root contains an unknown syntax node")
	}
	if element.name.namespace != xsdNamespaceURI {
		return nil, "", false, newSchemaSyntaxUnsupported(element.loc, "schema root contains an unsupported non-XSD construct")
	}
	kind, named := schemaDeclarationKind(element.name.local)
	if !named {
		ignored, err := schemaRootChildIgnored(element)
		if err != nil {
			return nil, "", false, err
		}
		if ignored {
			return nil, "", false, nil
		}
		return nil, "", false, newSchemaSyntaxUnsupported(
			element.loc,
			fmt.Sprintf("XSD schema child <%s> is not implemented", element.name.local),
		)
	}
	return element, kind, true, nil
}

func schemaDocumentDeclarationInput(element *syntaxElement, kind ComponentKind, facts schemaDocumentFacts, version XSDVersion) (schemaComponentInput, error) {
	name, err := schemaDeclarationName(element, facts.targetNamespace.value)
	if err != nil {
		return schemaComponentInput{}, err
	}
	declaration := schemaComponentInput{
		kind: kind,
		name: name,
		loc:  element.loc,
	}
	if kind == ComponentKindElementDeclaration {
		elementType, elementErr := schemaElementTypeInput(element, version)
		if elementErr != nil {
			return schemaComponentInput{}, elementErr
		}
		declaration.element = elementType
	}
	if kind == ComponentKindComplexTypeDefinition {
		complexType, complexErr := schemaComplexTypeInputFromElementWithFacts(element, facts, version)
		if complexErr != nil {
			return schemaComponentInput{}, complexErr
		}
		declaration.complexType = complexType
	}
	if kind != ComponentKindSimpleTypeDefinition {
		return declaration, nil
	}
	simpleType, err := schemaSimpleTypeInputFromElement(element)
	if err != nil {
		return schemaComponentInput{}, err
	}
	declaration.simpleType = simpleType
	return declaration, nil
}

func schemaElementTypeInput(element *syntaxElement, version XSDVersion) (*schemaElementInput, error) {
	attributes := syntaxAttributesByLocal(element, "type")
	inline := inlineSimpleTypeChild(element)
	if len(attributes) > 1 {
		return nil, newSchemaCompositionDiagnostic(element.loc, "element type attribute must be unique")
	}
	if len(attributes) == 0 && inline == nil {
		return nil, nil
	}
	if len(attributes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "element cannot combine type attribute with an inline simpleType")
	}
	if inline != nil {
		if err := validateInlineSchemaType(inline, version); err != nil {
			return nil, err
		}
		simpleType, err := schemaSimpleTypeInputFromElement(inline)
		if err != nil {
			return nil, err
		}
		return &schemaElementInput{
			typeLoc:          inline.loc,
			inlineSimpleType: simpleType,
		}, nil
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

func schemaComplexTypeInputFromElementWithFacts(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion) (*schemaComplexTypeInput, error) {
	model := schemaComplexTypeModel(element)
	if model == nil {
		return nil, nil
	}

	occurrences, err := schemaParticleOccurrenceRange(model, version)
	if err != nil {
		return nil, err
	}
	if model.name.local == "choice" {
		return schemaChoiceComplexTypeInput(model, occurrences, facts, version)
	}
	if facts.elementFormDefaultQualified && schemaModelHasElementChild(model) {
		return nil, newSchemaSyntaxUnsupported(
			model.loc,
			"schema elementFormDefault=qualified is not implemented for local sequence elements",
		)
	}
	return schemaSequenceComplexTypeInput(model, occurrences, facts, version)
}

func schemaComplexTypeModel(element *syntaxElement) *syntaxElement {
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "choice" && child.name.local != "sequence" {
			continue
		}
		return child
	}
	return nil
}

func schemaChoiceComplexTypeInput(model *syntaxElement, occurrences particleOccurrenceRange, facts schemaDocumentFacts, version XSDVersion) (*schemaComplexTypeInput, error) {
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
		alternative, err := schemaElementParticleInputFromElementWithFacts(child, facts, version, true)
		if err != nil {
			return nil, err
		}
		input.alternatives = append(input.alternatives, alternative)
	}
	return &schemaComplexTypeInput{particle: input}, nil
}

func schemaSequenceComplexTypeInput(model *syntaxElement, occurrences particleOccurrenceRange, facts schemaDocumentFacts, version XSDVersion) (*schemaComplexTypeInput, error) {
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
		alternative, err := schemaElementParticleInputFromElementWithFacts(child, facts, version, false)
		if err != nil {
			return nil, err
		}
		input.elements = append(input.elements, alternative)
	}
	return &schemaComplexTypeInput{particle: input}, nil
}

func schemaElementParticleInputFromElementWithFacts(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion, allowNamespacePolicy bool) (schemaElementParticleInput, error) {
	occurrences, err := schemaParticleOccurrenceRange(element, version)
	if err != nil {
		return schemaElementParticleInput{}, err
	}
	nameAttributes := syntaxAttributesByLocal(element, "name")
	if len(nameAttributes) != 1 {
		return schemaElementParticleInput{}, newSchemaBridgeInvariant(element.loc, "local element input has an invalid name attribute")
	}
	local := collapseXMLWhitespace(nameAttributes[0].value)
	name, err := schemaLocalElementParticleName(element, local, facts, version, allowNamespacePolicy)
	if err != nil {
		return schemaElementParticleInput{}, err
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

func schemaSimpleTypeInputFromElement(element *syntaxElement) (*schemaSimpleTypeInput, error) {
	if element == nil {
		return nil, newSchemaBridgeInvariant(Loc{}, "construct simple type input from a nil element")
	}
	var modelElement *syntaxElement
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI {
			continue
		}
		switch child.name.local {
		case "restriction", "list", "union":
			if modelElement != nil {
				return nil, newSchemaCompositionDiagnostic(child.loc, "simpleType requires exactly one model child")
			}
			modelElement = child
		}
	}
	if modelElement == nil {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simpleType requires exactly one model child")
	}
	var model schemaSimpleTypeModelInput
	var err error
	switch modelElement.name.local {
	case "restriction":
		model, err = schemaRestrictionModelInput(modelElement)
	case "list":
		model, err = schemaListModelInput(modelElement)
	case "union":
		model, err = schemaUnionModelInput(modelElement)
	default:
		return nil, newSchemaBridgeInvariant(modelElement.loc, "simple type has an unknown model child")
	}
	if err != nil {
		return nil, err
	}
	input := &schemaSimpleTypeInput{loc: element.loc, model: model}
	if restriction, ok := model.(*schemaSimpleTypeRestrictionModelInput); ok && restriction != nil && restriction.base.kind == schemaSimpleTypeQNameReferenceInput {
		input.base = restriction.base.name
		input.baseLoc = restriction.base.loc
		input.facets = cloneSchemaFacetInputs(restriction.facets)
	}
	return input, nil
}

func schemaModelHasElementChild(model *syntaxElement) bool {
	for _, node := range model.children {
		child, ok := node.(*syntaxElement)
		if ok && child.name.namespace == xsdNamespaceURI && child.name.local == "element" {
			return true
		}
	}
	return false
}

func schemaLocalElementParticleName(element *syntaxElement, local string, facts schemaDocumentFacts, version XSDVersion, allowNamespacePolicy bool) (QName, error) {
	namespace, err := schemaLocalElementParticleNamespace(element, facts, version, allowNamespacePolicy)
	if err != nil {
		return QName{}, err
	}
	name, err := NewQName(namespace, local)
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(element.loc, "construct local element name")
	}
	return name, nil
}

func schemaLocalElementParticleNamespace(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion, allowNamespacePolicy bool) (string, error) {
	if !allowNamespacePolicy {
		return "", nil
	}
	targetNamespaceAttributes := syntaxAttributesByLocal(element, "targetNamespace")
	if len(targetNamespaceAttributes) > 0 {
		return schemaLocalElementTargetNamespace(targetNamespaceAttributes[0], facts, version)
	}
	if schemaLocalElementIsQualified(element, facts) {
		return facts.targetNamespace.value, nil
	}
	return "", nil
}

func schemaLocalElementTargetNamespace(attribute syntaxAttribute, facts schemaDocumentFacts, version XSDVersion) (string, error) {
	targetNamespace := collapseXMLWhitespace(attribute.value)
	if version == XSDVersion11 && (!facts.targetNamespace.present || targetNamespace != facts.targetNamespace.value) {
		return "", invalidSchemaLocalElementTargetNamespace(attribute, facts.targetNamespace)
	}
	return targetNamespace, nil
}

func schemaLocalElementIsQualified(element *syntaxElement, facts schemaDocumentFacts) bool {
	formAttributes := syntaxAttributesByLocal(element, "form")
	if len(formAttributes) == 1 {
		return collapseXMLWhitespace(formAttributes[0].value) == "qualified"
	}
	return facts.elementFormDefaultQualified
}

func invalidSchemaLocalElementTargetNamespace(attribute syntaxAttribute, targetNamespace schemaTargetNamespace) Diagnostic {
	message := "local element targetNamespace requires a containing schema targetNamespace"
	if targetNamespace.present {
		message = "local element targetNamespace must match the containing schema targetNamespace"
	}
	related := make([]Loc, 0, 1)
	if targetNamespace.loc != (Loc{}) {
		related = append(related, targetNamespace.loc)
	}
	return Diagnostic{
		class:   FailureInvalid,
		code:    invalidSchemaCompositionCode,
		loc:     attribute.loc,
		message: message,
		related: related,
		specRef: schemaElementTargetNamespaceXSD11SpecRef,
		cause:   errSchemaElementTargetNamespace,
	}
}

//nolint:gocognit // Keep restriction source cardinality and facet collection together.
func schemaRestrictionModelInput(element *syntaxElement) (schemaSimpleTypeModelInput, error) {
	baseAttributes := syntaxAttributesByLocal(element, "base")
	inline := inlineSimpleTypeChild(element)
	if len(baseAttributes) > 1 {
		return nil, newSchemaCompositionDiagnostic(baseAttributes[1].loc, "simple type restriction base attribute must be unique")
	}
	if len(baseAttributes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "simple type restriction cannot combine base with an inline simpleType")
	}
	if len(baseAttributes) == 0 && inline == nil {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simple type restriction requires a base attribute or inline simpleType")
	}
	var base schemaSimpleTypeReferenceInput
	if len(baseAttributes) == 1 {
		name, err := expandSchemaQName(element, baseAttributes[0])
		if err != nil {
			return nil, err
		}
		base = schemaSimpleTypeReferenceInput{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: name,
			loc:  baseAttributes[0].loc,
		}
	}
	if inline != nil {
		anonymous, err := schemaSimpleTypeInputFromElement(inline)
		if err != nil {
			return nil, err
		}
		base = schemaSimpleTypeReferenceInput{
			kind:      schemaSimpleTypeAnonymousReferenceInput,
			loc:       inline.loc,
			anonymous: anonymous,
		}
	}
	facets := make([]schemaFacetInput, 0)
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
		facets = append(facets, *facet)
	}
	return &schemaSimpleTypeRestrictionModelInput{
		loc:    element.loc,
		base:   base,
		facets: facets,
	}, nil
}

func schemaListModelInput(element *syntaxElement) (schemaSimpleTypeModelInput, error) {
	itemTypes := syntaxAttributesByLocal(element, "itemType")
	inline := inlineSimpleTypeChild(element)
	if len(itemTypes) > 1 {
		return nil, newSchemaCompositionDiagnostic(itemTypes[1].loc, "simple type list itemType attribute must be unique")
	}
	if len(itemTypes) == 1 && inline != nil {
		return nil, newSchemaCompositionDiagnostic(inline.loc, "simple type list cannot combine itemType with an inline simpleType")
	}
	if len(itemTypes) == 0 && inline == nil {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simple type list requires an itemType or inline simpleType")
	}
	var itemType schemaSimpleTypeReferenceInput
	if len(itemTypes) == 1 {
		name, err := expandSchemaQName(element, itemTypes[0])
		if err != nil {
			return nil, err
		}
		itemType = schemaSimpleTypeReferenceInput{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: name,
			loc:  itemTypes[0].loc,
		}
	}
	if inline != nil {
		anonymous, err := schemaSimpleTypeInputFromElement(inline)
		if err != nil {
			return nil, err
		}
		itemType = schemaSimpleTypeReferenceInput{
			kind:      schemaSimpleTypeAnonymousReferenceInput,
			loc:       inline.loc,
			anonymous: anonymous,
		}
	}
	return &schemaSimpleTypeListModelInput{
		loc:      element.loc,
		itemType: itemType,
	}, nil
}

func schemaUnionModelInput(element *syntaxElement) (schemaSimpleTypeModelInput, error) {
	members := make([]schemaSimpleTypeReferenceInput, 0)
	memberTypes := syntaxAttributesByLocal(element, "memberTypes")
	if len(memberTypes) > 1 {
		return nil, newSchemaCompositionDiagnostic(memberTypes[1].loc, "simple type union memberTypes attribute must be unique")
	}
	if len(memberTypes) == 1 {
		for _, token := range strings.Fields(collapseXMLWhitespace(memberTypes[0].value)) {
			attribute := memberTypes[0]
			attribute.value = token
			name, err := expandSchemaQName(element, attribute)
			if err != nil {
				return nil, err
			}
			members = append(members, schemaSimpleTypeReferenceInput{
				kind: schemaSimpleTypeQNameReferenceInput,
				name: name,
				loc:  memberTypes[0].loc,
			})
		}
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "simpleType" {
			continue
		}
		anonymous, err := schemaSimpleTypeInputFromElement(child)
		if err != nil {
			return nil, err
		}
		members = append(members, schemaSimpleTypeReferenceInput{
			kind:      schemaSimpleTypeAnonymousReferenceInput,
			loc:       child.loc,
			anonymous: anonymous,
		})
	}
	if len(members) == 0 {
		return nil, newSchemaCompositionDiagnostic(element.loc, "simple type union requires memberTypes or an inline simpleType")
	}
	return &schemaSimpleTypeUnionModelInput{
		loc:     element.loc,
		members: members,
	}, nil
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
	present          bool
	loc              Loc
	nodeID           SimpleTypeID
	hasNodeID        bool
	anonymous        bool
	variety          SimpleTypeVariety
	varietyLoc       Loc
	atomicKind       schemaSimpleTypeAtomicKind
	base             QName
	baseLoc          Loc
	baseID           ComponentID
	hasBaseID        bool
	baseReference    schemaSimpleTypeReferenceComponent
	hasBaseReference bool
	itemType         schemaSimpleTypeReferenceComponent
	hasItemType      bool
	memberTypes      []schemaSimpleTypeReferenceComponent
	facets           schemaSimpleTypeFacetVariant
}

type schemaSimpleTypeAtomicKind uint8

const (
	schemaSimpleTypeAtomicUnknown schemaSimpleTypeAtomicKind = iota
	schemaSimpleTypeAtomicString
	schemaSimpleTypeAtomicInteger
	schemaSimpleTypeAtomicNegativeInteger
	schemaSimpleTypeAtomicDecimal
	schemaSimpleTypeAtomicPrecisionDecimal
)

type schemaSimpleTypeState uint8

const (
	schemaSimpleTypeUnvisited schemaSimpleTypeState = iota
	schemaSimpleTypeVisiting
	schemaSimpleTypeResolved
)

type schemaSimpleTypeResolver struct {
	records          []schemaComponentRecord
	byName           map[QName][]int
	states           map[*schemaSimpleTypeInput]schemaSimpleTypeState
	inputResults     map[*schemaSimpleTypeInput]schemaSimpleTypeResult
	results          []schemaSimpleTypeResult
	stack            []*schemaSimpleTypeInput
	stackFallbackLoc []Loc
}

type schemaSimpleTypeResolution struct {
	results []schemaSimpleTypeResult
	byInput map[*schemaSimpleTypeInput]schemaSimpleTypeResult
}

func resolveSchemaSimpleTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	version XSDVersion,
) (schemaSimpleTypeResolution, error) {
	resolver := schemaSimpleTypeResolver{
		records:          records,
		byName:           byName,
		states:           make(map[*schemaSimpleTypeInput]schemaSimpleTypeState),
		inputResults:     make(map[*schemaSimpleTypeInput]schemaSimpleTypeResult),
		results:          make([]schemaSimpleTypeResult, len(records)),
		stack:            make([]*schemaSimpleTypeInput, 0),
		stackFallbackLoc: make([]Loc, 0),
	}
	for index, record := range records {
		if record.simpleType == nil {
			continue
		}
		if _, err := resolver.resolve(index, version); err != nil {
			return schemaSimpleTypeResolution{}, err
		}
	}
	for _, record := range records {
		if record.element == nil || record.element.inlineSimpleType == nil {
			continue
		}
		if _, err := resolver.resolveInput(record.element.inlineSimpleType, record.element.inlineSimpleType.loc, true, version); err != nil {
			return schemaSimpleTypeResolution{}, err
		}
	}
	return schemaSimpleTypeResolution{
		results: resolver.results,
		byInput: resolver.inputResults,
	}, nil
}

type schemaElementTypeResult struct {
	present          bool
	declaredType     QName
	typeID           ComponentID
	hasTypeID        bool
	typeReference    schemaSimpleTypeReferenceComponent
	hasTypeReference bool
}

type schemaScalarTypeScope uint8

const (
	schemaScalarTypeGlobalElement schemaScalarTypeScope = iota + 1
	schemaScalarTypeLocalParticle
)

func (scope schemaScalarTypeScope) allowsBoolean() bool {
	return scope == schemaScalarTypeGlobalElement || scope == schemaScalarTypeLocalParticle
}

func resolveSchemaElementTypes(
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes schemaSimpleTypeResolution,
	complexTypes []schemaComplexTypeResult,
	version XSDVersion,
) ([]schemaElementTypeResult, error) {
	if len(simpleTypes.results) != len(records) || len(complexTypes) != len(records) {
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

//nolint:funlen,gocognit // Keep the ordered global element type branches together.
func resolveSchemaElementType(
	record schemaComponentRecord,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes schemaSimpleTypeResolution,
	complexTypes []schemaComplexTypeResult,
	version XSDVersion,
) (schemaElementTypeResult, error) {
	input := record.element
	if input == nil {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(record.loc, "element type resolution has no type input")
	}
	if input.inlineSimpleType != nil {
		result, ok := simpleTypes.byInput[input.inlineSimpleType]
		if !ok || !result.present {
			return schemaElementTypeResult{}, newSchemaBridgeInvariant(
				input.typeLoc,
				"inline element simple type has no resolved result",
			)
		}
		if err := rejectUnsupportedSchemaSimpleTypeVariety(input, result, version, "for global elements"); err != nil {
			return schemaElementTypeResult{}, err
		}
		if !result.hasNodeID || result.nodeID.IsZero() {
			return schemaElementTypeResult{}, newSchemaBridgeInvariant(
				input.typeLoc,
				"inline element simple type has no allocated model identity",
			)
		}
		return schemaElementTypeResult{
			present: true,
			typeReference: schemaSimpleTypeReferenceComponent{
				kind:           SimpleTypeReferenceAnonymous,
				loc:            input.typeLoc,
				anonymousID:    result.nodeID,
				hasAnonymousID: true,
				anonymous:      schemaSimpleTypeComponentFromResult(result, true),
				variety:        result.variety,
				varietyLoc:     result.varietyLoc,
				atomicKind:     result.atomicKind,
				facets:         result.facets,
			},
			hasTypeReference: true,
		}, nil
	}
	if input.declaredType.Namespace() == xsdNamespaceURI {
		return resolveSchemaScalarType(input, records, byName, simpleTypes.results, version, "for global elements", schemaScalarTypeGlobalElement, true)
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
	if !simpleTypes.results[candidate].present {
		return schemaElementTypeResult{}, newSchemaBridgeInvariant(
			input.typeLoc,
			"element type resolution has an incomplete simple type result",
		)
	}
	if err := rejectUnsupportedSchemaSimpleTypeVariety(input, simpleTypes.results[candidate], version, "for global elements"); err != nil {
		return schemaElementTypeResult{}, err
	}
	return schemaElementTypeResult{
		present:      true,
		declaredType: input.declaredType,
		typeID:       records[candidate].id,
		hasTypeID:    true,
		typeReference: schemaSimpleTypeReferenceComponent{
			kind:       SimpleTypeReferenceNamed,
			name:       input.declaredType,
			loc:        input.typeLoc,
			id:         records[candidate].id,
			hasID:      true,
			variety:    simpleTypes.results[candidate].variety,
			varietyLoc: simpleTypes.results[candidate].varietyLoc,
			atomicKind: simpleTypes.results[candidate].atomicKind,
			facets:     simpleTypes.results[candidate].facets,
		},
		hasTypeReference: true,
	}, nil
}

func resolveSchemaScalarType(
	input *schemaElementInput,
	records []schemaComponentRecord,
	byName map[QName][]int,
	simpleTypes []schemaSimpleTypeResult,
	version XSDVersion,
	complexTargetSuffix string,
	scope schemaScalarTypeScope,
	allowPrecisionDecimal bool,
) (schemaElementTypeResult, error) {
	if input.declaredType.Namespace() == xsdNamespaceURI {
		return resolveBuiltinSchemaScalarType(input, version, complexTargetSuffix, scope, allowPrecisionDecimal)
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
	if err := rejectUnsupportedSchemaSimpleTypeVariety(input, simpleTypes[candidate], version, complexTargetSuffix); err != nil {
		return schemaElementTypeResult{}, err
	}
	if err := rejectUnsupportedLocalScalarType(input, simpleTypes[candidate], version, scope, allowPrecisionDecimal); err != nil {
		return schemaElementTypeResult{}, err
	}
	return schemaElementTypeResult{
		present:      true,
		declaredType: input.declaredType,
		typeID:       records[candidate].id,
		hasTypeID:    true,
	}, nil
}

func rejectUnsupportedSchemaSimpleTypeVariety(input *schemaElementInput, simpleType schemaSimpleTypeResult, version XSDVersion, context string) error {
	if simpleType.variety == SimpleTypeVarietyAtomicRestriction {
		return nil
	}
	message := fmt.Sprintf("element type %q has simple type variety %q, which is not implemented %s", input.declaredType, simpleType.variety, context)
	if input.declaredType.IsZero() {
		message = fmt.Sprintf("inline element simple type has variety %q, which is not implemented %s", simpleType.variety, context)
	}
	return newSchemaSyntaxUnsupportedForVersion(
		input.typeLoc,
		message,
		version,
	)
}

func resolveBuiltinSchemaScalarType(input *schemaElementInput, version XSDVersion, complexTargetSuffix string, scope schemaScalarTypeScope, allowPrecisionDecimal bool) (schemaElementTypeResult, error) {
	switch input.declaredType.Local() {
	case "integer", "decimal":
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
		}, nil
	case "boolean":
		if !scope.allowsBoolean() {
			return schemaElementTypeResult{}, unsupportedLocalSchemaScalarType(input, version, complexTargetSuffix)
		}
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
		}, nil
	case "precisionDecimal":
		if version == XSDVersion10 {
			return schemaElementTypeResult{}, precisionDecimalSchemaVersionDiagnostic(input.typeLoc, input.declaredType)
		}
		if !allowPrecisionDecimal {
			return schemaElementTypeResult{}, unsupportedSequencePrecisionDecimal(input, version)
		}
		reference, err := builtinSchemaElementTypeReference(input, version)
		if err != nil {
			return schemaElementTypeResult{}, err
		}
		return schemaElementTypeResult{
			present:          true,
			declaredType:     input.declaredType,
			typeReference:    reference,
			hasTypeReference: true,
		}, nil
	default:
		return schemaElementTypeResult{}, newSchemaSyntaxUnsupportedForVersion(
			input.typeLoc,
			fmt.Sprintf("element type %q is not implemented", input.declaredType),
			version,
		)
	}
}

func builtinSchemaElementTypeReference(input *schemaElementInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	return resolveBuiltinSchemaSimpleTypeReference(schemaSimpleTypeReferenceInput{
		kind: schemaSimpleTypeQNameReferenceInput,
		name: input.declaredType,
		loc:  input.typeLoc,
	}, version)
}

func rejectUnsupportedLocalScalarType(input *schemaElementInput, simpleType schemaSimpleTypeResult, version XSDVersion, scope schemaScalarTypeScope, allowPrecisionDecimal bool) error {
	if scope != schemaScalarTypeLocalParticle {
		return nil
	}
	if allowPrecisionDecimal {
		return nil
	}
	if _, ok := simpleType.facets.(schemaPrecisionDecimalFacetVariant); !ok {
		return nil
	}
	return unsupportedSequencePrecisionDecimal(input, version)
}

func unsupportedLocalSchemaScalarType(input *schemaElementInput, version XSDVersion, complexTargetSuffix string) Diagnostic {
	return newSchemaSyntaxUnsupportedForVersion(
		input.typeLoc,
		fmt.Sprintf("element type %q is not implemented %s", input.declaredType, complexTargetSuffix),
		version,
	)
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
		schemaScalarTypeLocalParticle,
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

func newSchemaDuplicateDiagnostic(later, earliest schemaComponentRecord, space schemaSymbolSpace, version XSDVersion) Diagnostic {
	if space == schemaSymbolSpaceElement {
		return newSchemaElementDuplicateDiagnostic(later, earliest, version)
	}
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticSchemaGlobalDuplicateCode,
		loc:     later.loc,
		message: fmt.Sprintf("global %s %q is duplicated", schemaSymbolSpaceDescription(space), later.name),
		related: []Loc{earliest.loc},
		specRef: schemaGlobalDuplicateSpecRef(version),
		cause:   errSchemaGlobalDeclarationDuplicate,
	}
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

func schemaSymbolSpaceDescription(space schemaSymbolSpace) string {
	switch space {
	case schemaSymbolSpaceElement:
		return "element declaration"
	case schemaSymbolSpaceAttribute:
		return "attribute declaration"
	case schemaSymbolSpaceType:
		return "type definition"
	case schemaSymbolSpaceModelGroup:
		return "model group definition"
	case schemaSymbolSpaceAttributeGroup:
		return "attribute group definition"
	case schemaSymbolSpaceNotation:
		return "notation declaration"
	default:
		return "declaration"
	}
}

func schemaGlobalDuplicateSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaGlobalDuplicateXSD10SpecRef
	}
	return schemaGlobalDuplicateXSD11SpecRef
}

func schemaElementDuplicateSpecRef(version XSDVersion) string {
	return schemaGlobalDuplicateSpecRef(version)
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

func (resolver *schemaSimpleTypeResolver) resolve(index int, version XSDVersion) (schemaSimpleTypeResult, error) {
	if index < 0 || index >= len(resolver.records) {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(Loc{}, "simple type resolution index is out of range")
	}
	input := resolver.records[index].simpleType
	if input == nil {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(
			resolver.records[index].loc,
			"simple type resolution has no model input",
		)
	}
	result, err := resolver.resolveInput(input, resolver.records[index].loc, false, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	resolver.results[index] = result
	return result, nil
}

func (resolver *schemaSimpleTypeResolver) resolveInput(input *schemaSimpleTypeInput, fallbackLoc Loc, anonymous bool, version XSDVersion) (schemaSimpleTypeResult, error) {
	if input == nil {
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(fallbackLoc, "simple type resolution received a nil model input")
	}
	switch resolver.states[input] {
	case schemaSimpleTypeUnvisited:
	case schemaSimpleTypeResolved:
		return resolver.inputResults[input], nil
	case schemaSimpleTypeVisiting:
		return schemaSimpleTypeResult{}, resolver.cycleDiagnostic(input, version)
	default:
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(fallbackLoc, "simple type resolution has an unknown state")
	}
	model, err := schemaSimpleTypeModel(input)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	resolver.states[input] = schemaSimpleTypeVisiting
	resolver.stack = append(resolver.stack, input)
	resolver.stackFallbackLoc = append(resolver.stackFallbackLoc, fallbackLoc)
	result, err := resolver.resolveModel(input, model, version)
	if err != nil {
		if popErr := resolver.popInput(input, fallbackLoc); popErr != nil {
			return schemaSimpleTypeResult{}, popErr
		}
		return schemaSimpleTypeResult{}, err
	}
	result.present = true
	result.anonymous = anonymous
	result.loc = input.loc
	if result.loc.IsZero() {
		result.loc = fallbackLoc
	}
	result.nodeID = input.nodeID
	result.hasNodeID = input.hasNodeID
	resolver.states[input] = schemaSimpleTypeResolved
	resolver.inputResults[input] = result
	if err := resolver.popInput(input, fallbackLoc); err != nil {
		return schemaSimpleTypeResult{}, err
	}
	return result, nil
}

func (resolver *schemaSimpleTypeResolver) popInput(input *schemaSimpleTypeInput, fallbackLoc Loc) error {
	last := len(resolver.stack) - 1
	if last < 0 || resolver.stack[last] != input {
		return newSchemaBridgeInvariant(fallbackLoc, "simple type resolution stack pop does not match current input")
	}
	if len(resolver.stackFallbackLoc) != len(resolver.stack) {
		return newSchemaBridgeInvariant(fallbackLoc, "simple type resolution stack locations are inconsistent")
	}
	resolver.stack = resolver.stack[:last]
	resolver.stackFallbackLoc = resolver.stackFallbackLoc[:last]
	return nil
}

func schemaSimpleTypeModel(input *schemaSimpleTypeInput) (schemaSimpleTypeModelInput, error) {
	if input.model != nil {
		return input.model, nil
	}
	if input.base.IsZero() {
		return nil, newSchemaBridgeInvariant(input.loc, "simple type input has no tagged model")
	}
	return &schemaSimpleTypeRestrictionModelInput{
		loc: input.loc,
		base: schemaSimpleTypeReferenceInput{
			kind: schemaSimpleTypeQNameReferenceInput,
			name: input.base,
			loc:  input.baseLoc,
		},
		facets: cloneSchemaFacetInputs(input.facets),
	}, nil
}

func (resolver *schemaSimpleTypeResolver) resolveModel(input *schemaSimpleTypeInput, model schemaSimpleTypeModelInput, version XSDVersion) (schemaSimpleTypeResult, error) {
	switch typed := model.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if typed == nil {
			return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type restriction model is nil")
		}
		return resolver.resolveRestrictionModel(input, typed, version)
	case *schemaSimpleTypeListModelInput:
		if typed == nil {
			return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type list model is nil")
		}
		return resolver.resolveListModel(input, typed, version)
	case *schemaSimpleTypeUnionModelInput:
		if typed == nil {
			return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type union model is nil")
		}
		return resolver.resolveUnionModel(input, typed, version)
	default:
		return schemaSimpleTypeResult{}, newSchemaBridgeInvariant(input.loc, "simple type has an unknown tagged model")
	}
}

func (resolver *schemaSimpleTypeResolver) resolveRestrictionModel(input *schemaSimpleTypeInput, model *schemaSimpleTypeRestrictionModelInput, version XSDVersion) (schemaSimpleTypeResult, error) {
	base, err := resolver.resolveReference(model.base, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	if base.variety != SimpleTypeVarietyAtomicRestriction {
		return schemaSimpleTypeResult{}, unsupportedSimpleTypeRestriction(
			model.base.loc,
			base,
			version,
		)
	}
	facets, err := restrictSchemaSimpleTypeFacets(base.facets, model.facets, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	result := schemaSimpleTypeResult{
		loc:              input.loc,
		variety:          SimpleTypeVarietyAtomicRestriction,
		varietyLoc:       model.loc,
		atomicKind:       base.atomicKind,
		baseReference:    base,
		hasBaseReference: true,
		facets:           facets,
		baseLoc:          model.base.loc,
	}
	if base.kind != SimpleTypeReferenceAnonymous {
		result.base = base.name
	}
	if base.hasID {
		result.baseID = base.id
		result.hasBaseID = true
	}
	return result, nil
}

func (resolver *schemaSimpleTypeResolver) resolveListModel(input *schemaSimpleTypeInput, model *schemaSimpleTypeListModelInput, version XSDVersion) (schemaSimpleTypeResult, error) {
	itemType, err := resolver.resolveReference(model.itemType, version)
	if err != nil {
		return schemaSimpleTypeResult{}, err
	}
	if itemType.variety != SimpleTypeVarietyAtomicRestriction && itemType.variety != SimpleTypeVarietyUnion {
		return schemaSimpleTypeResult{}, invalidSimpleTypeDerivation(
			model.itemType.loc,
			fmt.Sprintf("simple type list item type %q has variety %q", itemType.name, itemType.variety),
			resolver.simpleTypeReferenceRelatedLocations(itemType),
			version,
		)
	}
	if version == XSDVersion11 && itemType.variety == SimpleTypeVarietyUnion {
		if err := resolver.validateAtomicUnionMembers(itemType, version, make(map[schemaSimpleTypeReferenceIdentity]struct{})); err != nil {
			return schemaSimpleTypeResult{}, err
		}
	}
	return schemaSimpleTypeResult{
		loc:         input.loc,
		variety:     SimpleTypeVarietyList,
		varietyLoc:  model.loc,
		itemType:    itemType,
		hasItemType: true,
	}, nil
}

type schemaSimpleTypeReferenceIdentity struct {
	kind        SimpleTypeReferenceKind
	name        QName
	id          ComponentID
	anonymousID SimpleTypeID
}

func simpleTypeReferenceIdentity(reference schemaSimpleTypeReferenceComponent) schemaSimpleTypeReferenceIdentity {
	return schemaSimpleTypeReferenceIdentity{
		kind:        reference.kind,
		name:        reference.name,
		id:          reference.id,
		anonymousID: reference.anonymousID,
	}
}

func (resolver *schemaSimpleTypeResolver) validateAtomicUnionMembers(
	reference schemaSimpleTypeReferenceComponent,
	version XSDVersion,
	seen map[schemaSimpleTypeReferenceIdentity]struct{},
) error {
	identity := simpleTypeReferenceIdentity(reference)
	if _, ok := seen[identity]; ok {
		return nil
	}
	seen[identity] = struct{}{}
	if reference.variety == SimpleTypeVarietyAtomicRestriction {
		return nil
	}
	if reference.variety != SimpleTypeVarietyUnion {
		return invalidSimpleTypeDerivation(
			reference.loc,
			fmt.Sprintf("simple type list item type %q has non-atomic transitive member variety %q", reference.name, reference.variety),
			resolver.simpleTypeReferenceRelatedLocations(reference),
			version,
		)
	}
	memberTypes, err := resolver.simpleTypeUnionMembers(reference, version)
	if err != nil {
		return err
	}
	for _, member := range memberTypes {
		if member.variety == SimpleTypeVarietyAtomicRestriction {
			continue
		}
		if err := resolver.validateAtomicUnionMembers(member, version, seen); err != nil {
			return err
		}
	}
	return nil
}

func (resolver *schemaSimpleTypeResolver) simpleTypeUnionMembers(
	reference schemaSimpleTypeReferenceComponent,
	version XSDVersion,
) ([]schemaSimpleTypeReferenceComponent, error) {
	if reference.kind == SimpleTypeReferenceAnonymous {
		if reference.anonymous == nil {
			return nil, newSchemaBridgeInvariant(reference.loc, "anonymous union reference has no model")
		}
		return reference.anonymous.memberTypes, nil
	}
	if reference.kind != SimpleTypeReferenceNamed {
		return nil, newSchemaBridgeInvariant(reference.loc, "union reference is not named or anonymous")
	}
	if !reference.hasID || reference.id.IsZero() {
		return nil, newSchemaBridgeInvariant(reference.loc, "named union reference has no component identity")
	}
	for index, record := range resolver.records {
		if record.id != reference.id {
			continue
		}
		result, err := resolver.resolve(index, version)
		if err != nil {
			return nil, err
		}
		return result.memberTypes, nil
	}
	return nil, newSchemaBridgeInvariant(reference.loc, "named union reference component identity is unresolved")
}

func (resolver *schemaSimpleTypeResolver) simpleTypeReferenceDefinitionLoc(reference schemaSimpleTypeReferenceComponent) Loc {
	if reference.kind == SimpleTypeReferenceAnonymous {
		if reference.anonymous != nil {
			return reference.anonymous.loc
		}
		return Loc{}
	}
	if reference.kind != SimpleTypeReferenceNamed || !reference.hasID {
		return Loc{}
	}
	for _, record := range resolver.records {
		if record.id == reference.id {
			return record.loc
		}
	}
	return Loc{}
}

func (resolver *schemaSimpleTypeResolver) simpleTypeReferenceRelatedLocations(reference schemaSimpleTypeReferenceComponent) []Loc {
	related := make([]Loc, 0, 2)
	definitionLoc := resolver.simpleTypeReferenceDefinitionLoc(reference)
	if !definitionLoc.IsZero() && definitionLoc != reference.loc {
		related = append(related, definitionLoc)
	}
	if !reference.varietyLoc.IsZero() && reference.varietyLoc != reference.loc && reference.varietyLoc != definitionLoc {
		related = append(related, reference.varietyLoc)
	}
	return related
}

func unsupportedSimpleTypeRestriction(loc Loc, base schemaSimpleTypeReferenceComponent, version XSDVersion) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticUnregisteredFeatureCode,
			loc,
			"schema syntax feature is not registered",
			fmt.Errorf("%w: %s", errSchemaSimpleTypeRestrictionUnsupported, base.variety),
		)
	}
	message := fmt.Sprintf("simple type restriction base %q has variety %q, which is not implemented", base.name, base.variety)
	if base.name.IsZero() {
		message = fmt.Sprintf("simple type restriction base has variety %q, which is not implemented", base.variety)
	}
	diagnostic := newUnsupportedForVersionWithCause(
		feature,
		UnsupportedSchemaSyntaxCode,
		loc,
		message,
		version,
		fmt.Errorf("%w: %s", errSchemaSimpleTypeRestrictionUnsupported, message),
	)
	if diagnostic.Class() == FailureUnsupported {
		diagnostic.specRef = schemaSimpleTypeSpecRef(version)
		if !base.varietyLoc.IsZero() && base.varietyLoc != loc {
			diagnostic.related = []Loc{base.varietyLoc}
		}
	}
	return diagnostic
}

func (resolver *schemaSimpleTypeResolver) resolveUnionModel(input *schemaSimpleTypeInput, model *schemaSimpleTypeUnionModelInput, version XSDVersion) (schemaSimpleTypeResult, error) {
	if len(model.members) == 0 {
		return schemaSimpleTypeResult{}, newSchemaCompositionDiagnostic(model.loc, "simple type union requires at least one member type")
	}
	members := make([]schemaSimpleTypeReferenceComponent, 0, len(model.members))
	for _, member := range model.members {
		resolved, err := resolver.resolveReference(member, version)
		if err != nil {
			return schemaSimpleTypeResult{}, err
		}
		if version == XSDVersion10 && resolved.variety == SimpleTypeVarietyUnion {
			return schemaSimpleTypeResult{}, invalidSimpleTypeDerivation(
				member.loc,
				fmt.Sprintf("simple type union member type %q has variety %q", resolved.name, resolved.variety),
				resolver.simpleTypeReferenceRelatedLocations(resolved),
				version,
			)
		}
		members = append(members, resolved)
	}
	return schemaSimpleTypeResult{
		loc:         input.loc,
		variety:     SimpleTypeVarietyUnion,
		varietyLoc:  model.loc,
		memberTypes: members,
	}, nil
}

func (resolver *schemaSimpleTypeResolver) resolveReference(input schemaSimpleTypeReferenceInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	switch input.kind {
	case schemaSimpleTypeAnonymousReferenceInput:
		if input.anonymous == nil {
			return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "anonymous simple type reference has no model")
		}
		result, err := resolver.resolveInput(input.anonymous, input.loc, true, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		if !result.hasNodeID || result.nodeID.IsZero() {
			return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "anonymous simple type reference has no allocated model identity")
		}
		return schemaSimpleTypeReferenceComponent{
			kind:           SimpleTypeReferenceAnonymous,
			loc:            input.loc,
			anonymousID:    result.nodeID,
			hasAnonymousID: result.hasNodeID,
			anonymous:      schemaSimpleTypeComponentFromResult(result, true),
			variety:        result.variety,
			varietyLoc:     result.varietyLoc,
			atomicKind:     result.atomicKind,
			facets:         result.facets,
		}, nil
	case schemaSimpleTypeQNameReferenceInput:
		if input.name.IsZero() {
			return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "simple type QName reference is empty")
		}
		if input.name.Namespace() == xsdNamespaceURI {
			return resolveBuiltinSchemaSimpleTypeReference(input, version)
		}
		return resolver.resolveNamedSchemaSimpleTypeReference(input, version)
	default:
		return schemaSimpleTypeReferenceComponent{}, newSchemaBridgeInvariant(input.loc, "simple type reference has an unknown kind")
	}
}

//nolint:gocognit // Keep the built-in datatype mapping explicit and versioned.
func resolveBuiltinSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	result := schemaSimpleTypeResult{
		variety:    SimpleTypeVarietyAtomicRestriction,
		varietyLoc: input.loc,
	}
	switch input.name.Local() {
	case "string":
		result.atomicKind = schemaSimpleTypeAtomicString
		result.facets = schemaStringFacetVariant{}
	case "integer":
		facets, err := NewIntegerDigitFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		bounds, err := NewIntegerBoundFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicInteger
		result.facets = schemaDigitFacetVariant{value: facets, integerBounds: bounds}
	case "negativeInteger":
		facets, err := NewIntegerDigitFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		maxInclusive, err := ParseIntegerMaxInclusiveFacet("-1", input.loc, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{maxInclusive}, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicNegativeInteger
		result.facets = schemaDigitFacetVariant{value: facets, integerBounds: bounds}
	case "decimal":
		facets, err := NewDecimalDigitFacets(nil, nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		bounds, err := NewDecimalBoundFacets(nil, version)
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicDecimal
		result.facets = schemaDigitFacetVariant{value: facets, decimalBounds: bounds}
	case "precisionDecimal":
		if version == XSDVersion10 {
			return schemaSimpleTypeReferenceComponent{}, precisionDecimalSchemaVersionDiagnostic(input.loc, input.name)
		}
		facets, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{})
		if err != nil {
			return schemaSimpleTypeReferenceComponent{}, err
		}
		result.atomicKind = schemaSimpleTypeAtomicPrecisionDecimal
		result.facets = schemaPrecisionDecimalFacetVariant{value: facets}
	case "boolean":
		result.facets = schemaBooleanFacetVariant{}
	default:
		return schemaSimpleTypeReferenceComponent{}, newSchemaSyntaxUnsupported(
			input.loc,
			fmt.Sprintf("simple type reference %q is not supported", input.name),
		)
	}
	return schemaSimpleTypeReferenceComponent{
		kind:       SimpleTypeReferenceBuiltin,
		name:       input.name,
		loc:        input.loc,
		variety:    result.variety,
		varietyLoc: result.varietyLoc,
		atomicKind: result.atomicKind,
		facets:     result.facets,
	}, nil
}

func (resolver *schemaSimpleTypeResolver) resolveNamedSchemaSimpleTypeReference(input schemaSimpleTypeReferenceInput, version XSDVersion) (schemaSimpleTypeReferenceComponent, error) {
	candidates := resolver.byName[input.name]
	if len(candidates) == 0 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeUnresolvedCode,
			input.loc,
			fmt.Sprintf("simple type reference %q cannot be resolved", input.name),
			nil,
			version,
			errSchemaSimpleTypeBaseUnresolved,
		)
	}
	simpleCandidates := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if resolver.records[candidate].kind != ComponentKindSimpleTypeDefinition {
			continue
		}
		simpleCandidates = append(simpleCandidates, candidate)
	}
	if len(simpleCandidates) == 0 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeWrongKindCode,
			input.loc,
			fmt.Sprintf("simple type reference %q does not name a simple type", input.name),
			schemaComponentLocations(resolver.records, candidates),
			version,
			fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseWrongKind, input.name),
		)
	}
	if len(simpleCandidates) > 1 {
		return schemaSimpleTypeReferenceComponent{}, newSchemaSimpleTypeDiagnostic(
			diagnosticSchemaSimpleTypeAmbiguousCode,
			input.loc,
			fmt.Sprintf("simple type reference %q is ambiguous", input.name),
			schemaComponentLocations(resolver.records, simpleCandidates),
			version,
			fmt.Errorf("%w: %q", errSchemaSimpleTypeBaseAmbiguous, input.name),
		)
	}
	index := simpleCandidates[0]
	result, err := resolver.resolve(index, version)
	if err != nil {
		return schemaSimpleTypeReferenceComponent{}, err
	}
	return schemaSimpleTypeReferenceComponent{
		kind:       SimpleTypeReferenceNamed,
		name:       input.name,
		loc:        input.loc,
		id:         resolver.records[index].id,
		hasID:      true,
		variety:    result.variety,
		varietyLoc: result.varietyLoc,
		atomicKind: result.atomicKind,
		facets:     result.facets,
	}, nil
}

func schemaSimpleTypeComponentFromResult(result schemaSimpleTypeResult, anonymous bool) *schemaSimpleTypeComponent {
	return &schemaSimpleTypeComponent{
		loc:              result.loc,
		nodeID:           result.nodeID,
		hasNodeID:        result.hasNodeID,
		anonymous:        anonymous,
		variety:          result.variety,
		varietyLoc:       result.varietyLoc,
		atomicKind:       result.atomicKind,
		base:             result.base,
		baseLoc:          result.baseLoc,
		baseID:           result.baseID,
		hasBaseID:        result.hasBaseID,
		baseReference:    result.baseReference,
		hasBaseReference: result.hasBaseReference,
		itemType:         result.itemType,
		hasItemType:      result.hasItemType,
		memberTypes:      cloneSchemaSimpleTypeReferenceComponents(result.memberTypes),
		facets:           result.facets,
	}
}

func invalidSimpleTypeDerivation(loc Loc, message string, related []Loc, version XSDVersion) Diagnostic {
	filteredRelated := make([]Loc, 0, len(related))
	for _, relatedLoc := range related {
		if relatedLoc.IsZero() || relatedLoc == loc {
			continue
		}
		filteredRelated = append(filteredRelated, relatedLoc)
	}
	return newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeBaseCode,
		loc,
		message,
		filteredRelated,
		version,
		fmt.Errorf("%w: %s", errSchemaSimpleTypeInvalidDerivation, message),
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
		switch typed.value.Kind() {
		case DigitDatatypeInteger:
			baseEnumeration, err := NewIntegerEnumerationFacets(nil, version)
			if err != nil {
				return nil, err
			}
			baseBounds, err := NewIntegerBoundFacets(nil, version)
			if err != nil {
				return nil, err
			}
			if typed.integerBounds.Version() == version {
				baseBounds = typed.integerBounds
			}
			return restrictSchemaIntegerFacets(typed.value, baseBounds, baseEnumeration, inputs, version)
		case DigitDatatypeDecimal:
			baseEnumeration, err := NewDecimalEnumerationFacets(nil, version)
			if err != nil {
				return nil, err
			}
			baseBounds, err := NewDecimalBoundFacets(nil, version)
			if err != nil {
				return nil, err
			}
			if typed.decimalBounds.Version() == version {
				baseBounds = typed.decimalBounds
			}
			return restrictSchemaDecimalFacets(typed.value, baseBounds, baseEnumeration, inputs, version)
		default:
			return nil, newSchemaBridgeInvariant(Loc{}, "simple type facet resolution has an unknown digit datatype")
		}
	case schemaIntegerFacetVariant:
		return restrictSchemaIntegerFacets(typed.digits, typed.bounds, typed.enumeration, inputs, version)
	case schemaDecimalFacetVariant:
		return restrictSchemaDecimalFacets(typed.digits, typed.bounds, typed.enumeration, inputs, version)
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
	case schemaStringFacetVariant:
		if len(inputs) == 0 {
			return typed, nil
		}
		return nil, unsupportedSchemaDatatypeFacet(inputs[0], version)
	case schemaBooleanFacetVariant:
		return restrictSchemaBooleanFacets(typed, inputs, version)
	default:
		return nil, newSchemaBridgeInvariant(Loc{}, "simple type facet resolution has an unknown datatype variant")
	}
}

func restrictSchemaIntegerFacets(
	base DigitFacets,
	baseBounds IntegerBoundFacets,
	baseEnumeration IntegerEnumerationFacets,
	inputs []schemaFacetInput,
	version XSDVersion,
) (schemaSimpleTypeFacetVariant, error) {
	local, err := schemaNumericFacetDeclarations(inputs, DigitDatatypeInteger, version)
	if err != nil {
		return nil, err
	}
	digits, err := RestrictDigitFacets(base, local.digits)
	if err != nil {
		return nil, err
	}
	bounds, err := RestrictIntegerBoundFacets(baseBounds, local.integerBounds)
	if err != nil {
		return nil, err
	}
	err = validateSchemaIntegerEnumerationBaseValueSpace(base, baseBounds, local.integerEnumeration, version)
	if err != nil {
		return nil, err
	}
	enumeration, err := RestrictIntegerEnumerationFacets(baseEnumeration, local.integerEnumeration)
	if err != nil {
		return nil, err
	}
	if local.deferredUnsupported != nil {
		return nil, local.deferredUnsupported
	}
	return schemaIntegerFacetVariant{digits: digits, enumeration: enumeration, bounds: bounds}, nil
}

func restrictSchemaDecimalFacets(
	base DigitFacets,
	baseBounds DecimalBoundFacets,
	baseEnumeration DecimalEnumerationFacets,
	inputs []schemaFacetInput,
	version XSDVersion,
) (schemaSimpleTypeFacetVariant, error) {
	local, err := schemaNumericFacetDeclarations(inputs, DigitDatatypeDecimal, version)
	if err != nil {
		return nil, err
	}
	digits, err := RestrictDigitFacets(base, local.digits)
	if err != nil {
		return nil, err
	}
	bounds, err := RestrictDecimalBoundFacets(baseBounds, local.decimalBounds)
	if err != nil {
		return nil, err
	}
	err = validateSchemaDecimalEnumerationBaseValueSpace(base, baseBounds, local.decimalEnumeration, version)
	if err != nil {
		return nil, err
	}
	enumeration, err := RestrictDecimalEnumerationFacets(baseEnumeration, local.decimalEnumeration)
	if err != nil {
		return nil, err
	}
	if local.deferredUnsupported != nil {
		return nil, local.deferredUnsupported
	}
	return schemaDecimalFacetVariant{digits: digits, enumeration: enumeration, bounds: bounds}, nil
}

func validateSchemaIntegerEnumerationBaseValueSpace(
	base DigitFacets,
	bounds IntegerBoundFacets,
	local IntegerEnumerationFacetDeclarations,
	version XSDVersion,
) error {
	if local.Values == nil {
		return nil
	}
	for index := range local.Values {
		declaration := local.Values[index]
		err := base.ValidateInteger(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errDigitFacetValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaDigitFacetViolationLocations(base, err),
				version,
				"integer",
				err,
			)
		}
		err = bounds.ValidateInteger(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errBoundValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaFacetViolationLocations(err),
				version,
				"integer",
				err,
			)
		}
	}
	return nil
}

func validateSchemaDecimalEnumerationBaseValueSpace(
	base DigitFacets,
	bounds DecimalBoundFacets,
	local DecimalEnumerationFacetDeclarations,
	version XSDVersion,
) error {
	if local.Values == nil {
		return nil
	}
	for index := range local.Values {
		declaration := local.Values[index]
		err := base.ValidateDecimal(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errDigitFacetValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaDigitFacetViolationLocations(base, err),
				version,
				"decimal",
				err,
			)
		}
		err = bounds.ValidateDecimal(declaration.Value(), declaration.Loc())
		if err != nil {
			if !errors.Is(err, errBoundValueViolation) {
				return err
			}
			return schemaEnumerationBaseValueSpaceDiagnostic(
				declaration.Loc(),
				schemaFacetViolationLocations(err),
				version,
				"decimal",
				err,
			)
		}
	}
	return nil
}

func schemaFacetViolationLocations(violation error) []Loc {
	var diagnostic Diagnostic
	if errors.As(violation, &diagnostic) {
		return diagnostic.Related()
	}
	return nil
}

func schemaDigitFacetViolationLocations(base DigitFacets, violation error) []Loc {
	var diagnostic Diagnostic
	if errors.As(violation, &diagnostic) {
		if related := diagnostic.Related(); len(related) != 0 {
			return related
		}
	}
	related := make([]Loc, 0, 2)
	if loc, ok := base.TotalDigitsLoc(); ok && !loc.IsZero() {
		related = append(related, loc)
	}
	if loc, ok := base.FractionDigitsLoc(); ok && !loc.IsZero() {
		related = append(related, loc)
	}
	return related
}

func schemaEnumerationBaseValueSpaceDiagnostic(
	loc Loc,
	related []Loc,
	version XSDVersion,
	datatype string,
	cause error,
) error {
	return newEnumerationDiagnostic(
		FailureInvalid,
		InvalidEnumerationRestrictionCode,
		loc,
		enumerationSpecRef(version, enumerationRestrictionRule),
		"local "+datatype+" enumeration value is not in the base value space",
		related,
		fmt.Errorf("%w: %w", errInvalidEnumerationRestriction, cause),
	)
}

type schemaNumericFacetDeclarationSet struct {
	digits              DigitFacetDeclarations
	integerEnumeration  IntegerEnumerationFacetDeclarations
	decimalEnumeration  DecimalEnumerationFacetDeclarations
	integerBounds       IntegerBoundFacetDeclarations
	decimalBounds       DecimalBoundFacetDeclarations
	deferredUnsupported error
}

func restrictSchemaBooleanFacets(base schemaBooleanFacetVariant, inputs []schemaFacetInput, version XSDVersion) (schemaSimpleTypeFacetVariant, error) {
	if len(inputs) == 0 {
		return base, nil
	}

	var unsupported error
	for _, input := range inputs {
		err := unsupportedSchemaBooleanFacet(input, version)
		if err == nil {
			return nil, newSchemaBridgeInvariant(input.loc, "boolean facet restriction produced no result")
		}
		var diagnostic Diagnostic
		if !errors.As(err, &diagnostic) {
			return nil, err
		}
		if diagnostic.Class() != FailureUnsupported {
			return nil, err
		}
		if unsupported == nil {
			unsupported = err
		}
	}
	return nil, unsupported
}

func unsupportedSchemaBooleanFacet(input schemaFacetInput, version XSDVersion) error {
	if err := validateSchemaBooleanFacetInput(input, version); err != nil {
		return err
	}
	err := unsupportedSchemaDatatypeFacet(input, version)
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
		return err
	}
	diagnostic.specRef = schemaBooleanDatatypeSpecRef(version)
	return diagnostic
}

func validateSchemaBooleanFacetInput(input schemaFacetInput, version XSDVersion) error {
	if err := validateOrdinarySchemaFacetInput(input); err != nil {
		return err
	}
	valueLoc := schemaFacetValueLocation(input)
	switch input.kind {
	case schemaFacetEnumeration, schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive:
		_, err := ParseStrictBooleanFor(version, input.lexical, valueLoc)
		return err
	case schemaFacetTotalDigits:
		_, err := ParseTotalDigitsFor(version, input.lexical, valueLoc)
		return err
	case schemaFacetFractionDigits:
		_, err := ParseFractionDigitsFor(version, input.lexical, valueLoc)
		return err
	case schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength:
		value, err := ParseStrictInteger(input.lexical, valueLoc)
		if err != nil {
			return err
		}
		if value.Sign() < 0 {
			return newSchemaCompositionDiagnostic(valueLoc, schemaFacetName(input.kind)+" facet value must be non-negative")
		}
		return nil
	case schemaFacetWhiteSpace:
		return validateSchemaBooleanFacetEnum(input, "collapse")
	case schemaFacetExplicitTimezone:
		return validateSchemaBooleanFacetEnum(input, "prohibited", "optional", "required")
	case schemaFacetPattern:
		return nil
	case schemaFacetMinScale, schemaFacetMaxScale, schemaFacetPrecision:
		return nil
	default:
		return newSchemaBridgeInvariant(input.loc, "boolean facet has an unknown kind")
	}
}

func validateSchemaBooleanFacetEnum(input schemaFacetInput, values ...string) error {
	value := collapseXMLWhitespace(input.lexical)
	for _, allowed := range values {
		if value == allowed {
			return nil
		}
	}
	return newSchemaCompositionDiagnostic(
		schemaFacetValueLocation(input),
		schemaFacetName(input.kind)+" facet value is invalid",
	)
}

func schemaBooleanDatatypeSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return schemaBooleanDatatypeXSD10SpecRef
	}
	return schemaBooleanDatatypeXSD11SpecRef
}

//nolint:gocognit,funlen // Keep the supported numeric facet mapping and unsupported facet boundary in one lexical pass.
func schemaNumericFacetDeclarations(
	inputs []schemaFacetInput,
	kind DigitDatatype,
	version XSDVersion,
) (schemaNumericFacetDeclarationSet, error) {
	var totalDigits *TotalDigitsFacet
	var fractionDigits *FractionDigitsFacet
	var integerEnumeration []IntegerEnumerationFacet
	var decimalEnumeration []DecimalEnumerationFacet
	var integerBounds []IntegerBoundFacet
	var decimalBounds []DecimalBoundFacet
	var deferredUnsupported error
	if kind != DigitDatatypeInteger && kind != DigitDatatypeDecimal {
		return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(Loc{}, "simple type facet collection has an unknown digit datatype")
	}
	for _, input := range inputs {
		loc := schemaFacetValueLocation(input)
		switch input.kind {
		case schemaFacetTotalDigits:
			facet, err := ParseTotalDigitsFacetWithFixed(input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			totalDigits = &facet
		case schemaFacetFractionDigits:
			facet, err := ParseFractionDigitsFacetWithFixed(input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			fractionDigits = &facet
		case schemaFacetEnumeration:
			enumerationLoc := input.loc
			switch kind {
			case DigitDatatypeInteger:
				facet, err := ParseIntegerEnumerationFacetFor(version, input.lexical, enumerationLoc)
				if err != nil {
					return schemaNumericFacetDeclarationSet{}, err
				}
				integerEnumeration = append(integerEnumeration, facet)
			case DigitDatatypeDecimal:
				facet, err := ParseDecimalEnumerationFacetFor(version, input.lexical, enumerationLoc)
				if err != nil {
					return schemaNumericFacetDeclarationSet{}, err
				}
				decimalEnumeration = append(decimalEnumeration, facet)
			default:
				return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "numeric facet declarations have an unknown datatype")
			}
		case schemaFacetMinInclusive, schemaFacetMinExclusive, schemaFacetMaxInclusive, schemaFacetMaxExclusive:
			boundKind, ok := schemaBoundKindFromFacet(input.kind)
			if !ok {
				return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "simple type bound facet has an unknown kind")
			}
			if kind == DigitDatatypeInteger {
				facet, err := ParseIntegerBoundFacetWithFixed(boundKind, input.lexical, loc, input.fixed, version)
				if err != nil {
					return schemaNumericFacetDeclarationSet{}, err
				}
				integerBounds = append(integerBounds, facet)
				continue
			}
			facet, err := ParseDecimalBoundFacetWithFixed(boundKind, input.lexical, loc, input.fixed, version)
			if err != nil {
				return schemaNumericFacetDeclarationSet{}, err
			}
			decimalBounds = append(decimalBounds, facet)
		case schemaFacetMinScale, schemaFacetMaxScale, schemaFacetPattern,
			schemaFacetWhiteSpace, schemaFacetLength, schemaFacetMinLength, schemaFacetMaxLength,
			schemaFacetPrecision, schemaFacetExplicitTimezone:
			err := unsupportedSchemaDatatypeFacet(input, version)
			if err == nil {
				continue
			}
			var diagnostic Diagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
				return schemaNumericFacetDeclarationSet{}, err
			}
			if deferredUnsupported == nil {
				deferredUnsupported = err
			}
		default:
			return schemaNumericFacetDeclarationSet{}, newSchemaBridgeInvariant(input.loc, "simple type facet has an unknown kind")
		}
	}
	return schemaNumericFacetDeclarationSet{
		digits:              NewDigitFacetDeclarations(totalDigits, fractionDigits),
		integerEnumeration:  NewIntegerEnumerationFacetDeclarations(integerEnumeration),
		decimalEnumeration:  NewDecimalEnumerationFacetDeclarations(decimalEnumeration),
		integerBounds:       NewIntegerBoundFacetDeclarations(integerBounds),
		decimalBounds:       NewDecimalBoundFacetDeclarations(decimalBounds),
		deferredUnsupported: deferredUnsupported,
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

func (resolver *schemaSimpleTypeResolver) cycleDiagnostic(input *schemaSimpleTypeInput, version XSDVersion) error {
	loc := simpleTypeInputReferenceLoc(input)
	start := 0
	found := false
	for position, current := range resolver.stack {
		if current == input {
			start = position
			found = true
			break
		}
	}
	if !found {
		return newSchemaBridgeInvariant(loc, "simple type cycle state is missing its stack entry")
	}
	if loc.IsZero() {
		loc = resolver.stackFallbackLoc[start]
	}
	related := make([]Loc, 0, len(resolver.stack)-start)
	for position, current := range resolver.stack[start:] {
		currentLoc := simpleTypeInputReferenceLoc(current)
		if currentLoc.IsZero() {
			fallbackIndex := start + position
			if fallbackIndex < len(resolver.stackFallbackLoc) {
				currentLoc = resolver.stackFallbackLoc[fallbackIndex]
			}
		}
		if currentLoc.IsZero() || currentLoc == loc {
			continue
		}
		related = append(related, currentLoc)
	}
	return newSchemaSimpleTypeDiagnostic(
		diagnosticSchemaSimpleTypeCycleCode,
		loc,
		"simple type definitions form a cycle",
		related,
		version,
		fmt.Errorf("%w: source %s", errSchemaSimpleTypeBaseCycle, loc.Source()),
	)
}

func simpleTypeInputReferenceLoc(input *schemaSimpleTypeInput) Loc {
	if input == nil {
		return Loc{}
	}
	model := input.model
	if model == nil {
		if !input.baseLoc.IsZero() {
			return input.baseLoc
		}
		return input.loc
	}
	switch typed := model.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if typed != nil && !typed.base.loc.IsZero() {
			return typed.base.loc
		}
	case *schemaSimpleTypeListModelInput:
		if typed != nil && !typed.itemType.loc.IsZero() {
			return typed.itemType.loc
		}
	case *schemaSimpleTypeUnionModelInput:
		if typed != nil && len(typed.members) > 0 && !typed.members[0].loc.IsZero() {
			return typed.members[0].loc
		}
	}
	return input.loc
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
