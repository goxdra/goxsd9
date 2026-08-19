package goxsd9

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	invalidSchemaConditionalCode = "XSD3012"
)

type schemaConditionalState struct {
	include       bool
	unsupportedAt Loc
	unsupported   bool
}

type schemaConditionalEvaluation struct {
	schemaConditionalState
	minVersion StrictDecimal
	maxVersion StrictDecimal
	hasMin     bool
	hasMax     bool
}

// applySchemaConditionals evaluates XSD 1.1 conditional inclusion before any
// schema grammar or reference work. Excluded elements are removed from the
// effective tree, so their syntax and references cannot affect discovery.
func applySchemaConditionals(document *syntaxDocument) error {
	if document == nil || document.root == nil {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxDocumentNoRootCode,
			Loc{},
			"syntax document has no root element",
			nil,
		)
	}

	state, err := evaluateSchemaConditional(document.root)
	if err != nil {
		return err
	}
	if !state.include {
		document.root.children = nil
		document.root.attrs = conditionalRootFacts(document.root.attrs)
		return nil
	}
	return pruneSchemaConditionalChildren(document.root)
}

func pruneSchemaConditionalChildren(parent *syntaxElement) error {
	children := make([]syntaxNode, 0, len(parent.children))
	for _, node := range parent.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			children = append(children, node)
			continue
		}
		state, err := evaluateSchemaConditional(child)
		if err != nil {
			return err
		}
		if !state.include {
			continue
		}
		if err := pruneSchemaConditionalChildren(child); err != nil {
			return err
		}
		children = append(children, child)
	}
	parent.children = children
	return nil
}

func evaluateSchemaConditional(element *syntaxElement) (schemaConditionalState, error) {
	evaluation, err := collectSchemaConditionalAttributes(element)
	if err != nil {
		return schemaConditionalState{}, err
	}
	if err := applySchemaConditionalVersion(element, &evaluation); err != nil {
		return schemaConditionalState{}, err
	}
	if !evaluation.include {
		return evaluation.schemaConditionalState, nil
	}
	if evaluation.unsupported {
		return schemaConditionalAvailabilityUnsupported(evaluation.unsupportedAt)
	}
	return evaluation.schemaConditionalState, nil
}

func collectSchemaConditionalAttributes(element *syntaxElement) (schemaConditionalEvaluation, error) {
	evaluation := schemaConditionalEvaluation{
		schemaConditionalState: schemaConditionalState{include: true},
	}
	for _, attribute := range element.attrs {
		if err := collectSchemaConditionalAttribute(element, attribute, &evaluation); err != nil {
			return schemaConditionalEvaluation{}, err
		}
	}
	return evaluation, nil
}

func collectSchemaConditionalAttribute(element *syntaxElement, attribute syntaxAttribute, evaluation *schemaConditionalEvaluation) error {
	if attribute.name.namespace != xsdVersioningNamespaceURI {
		return nil
	}
	switch attribute.name.local {
	case "minVersion", "maxVersion":
		return collectSchemaConditionalVersion(attribute, evaluation)
	case "typeAvailable", "typeUnavailable", "facetAvailable", "facetUnavailable":
		return collectSchemaConditionalAvailability(element, attribute, evaluation)
	default:
		// The XSD versioning namespace is extensible. Unknown attributes
		// are permitted and intentionally have no effect here.
		return nil
	}
}

func collectSchemaConditionalVersion(attribute syntaxAttribute, evaluation *schemaConditionalEvaluation) error {
	value, err := ParseStrictDecimalFor(XSDVersion11, attribute.value, attribute.loc)
	if err != nil {
		return err
	}
	if attribute.name.local == "minVersion" {
		evaluation.minVersion = value
		evaluation.hasMin = true
		return nil
	}
	evaluation.maxVersion = value
	evaluation.hasMax = true
	return nil
}

func collectSchemaConditionalAvailability(element *syntaxElement, attribute syntaxAttribute, evaluation *schemaConditionalEvaluation) error {
	if err := parseConditionalQNameList(element, attribute); err != nil {
		return err
	}
	if collapseXMLWhitespace(attribute.value) == "" {
		if strings.HasSuffix(attribute.name.local, "Unavailable") {
			evaluation.include = false
		}
		return nil
	}
	if !evaluation.unsupported {
		evaluation.unsupportedAt = attribute.loc
		evaluation.unsupported = true
	}
	return nil
}

func applySchemaConditionalVersion(element *syntaxElement, evaluation *schemaConditionalEvaluation) error {
	version, err := ParseStrictDecimalFor(XSDVersion11, "1.1", element.loc)
	if err != nil {
		return newSchemaBridgeInvariant(element.loc, "construct XSD 1.1 conditional version")
	}
	if evaluation.hasMin && version.Compare(evaluation.minVersion) < 0 {
		evaluation.include = false
	}
	if evaluation.hasMax && version.Compare(evaluation.maxVersion) >= 0 {
		evaluation.include = false
	}
	return nil
}

func schemaConditionalAvailabilityUnsupported(loc Loc) (schemaConditionalState, error) {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return schemaConditionalState{}, newDiagnostic(
			FailureInternal,
			diagnosticSyntaxFeatureCode,
			loc,
			"schema syntax feature is not registered",
			nil,
		)
	}
	return schemaConditionalState{}, newUnsupported(
		feature,
		UnsupportedSchemaSyntaxCode,
		loc,
		"conditional type or facet availability is not implemented",
	)
}

func parseConditionalQNameList(element *syntaxElement, attribute syntaxAttribute) error {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		return nil
	}
	items := strings.Split(lexeme, " ")
	for _, item := range items {
		prefix, local, ok := splitConditionalQName(item)
		if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
			return newDiagnostic(
				FailureInvalid,
				invalidSchemaConditionalCode,
				attribute.loc,
				fmt.Sprintf("conditional QName %q is malformed", item),
				nil,
			)
		}
		_, bound := element.scope.lookup(prefix)
		if prefix != "" && !bound {
			return newDiagnostic(
				FailureInvalid,
				invalidSchemaConditionalCode,
				attribute.loc,
				fmt.Sprintf("conditional QName prefix %q is unbound", prefix),
				nil,
			)
		}
	}
	return nil
}

func splitConditionalQName(value string) (string, string, bool) {
	colon := strings.IndexByte(value, ':')
	if colon < 0 {
		return "", value, true
	}
	if colon == 0 || colon == len(value)-1 || strings.IndexByte(value[colon+1:], ':') >= 0 {
		return "", "", false
	}
	return value[:colon], value[colon+1:], true
}

func conditionalRootFacts(attributes []syntaxAttribute) []syntaxAttribute {
	result := make([]syntaxAttribute, 0, len(attributes))
	for _, attribute := range attributes {
		if attribute.name.namespace == xsdVersioningNamespaceURI &&
			(attribute.name.local == "minVersion" || attribute.name.local == "maxVersion") {
			result = append(result, attribute)
			continue
		}
		if attribute.name.namespace == "" && attribute.name.local == "targetNamespace" {
			result = append(result, attribute)
		}
	}
	return result
}

type schemaGrammarPhase uint8

const (
	schemaGrammarMetadata schemaGrammarPhase = iota
	schemaGrammarComposition
	schemaGrammarDefaultOpenContent
	schemaGrammarDeclarations
)

// validateSyntaxDocumentStructure validates active syntax before references
// are extracted. It deliberately does not construct public components.
func validateSyntaxDocumentStructure(document *syntaxDocument) error {
	if document == nil || document.root == nil {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxDocumentNoRootCode,
			Loc{},
			"syntax document has no root element",
			nil,
		)
	}
	root := document.root
	if root.name != (syntaxName{namespace: xsdNamespaceURI, local: "schema"}) {
		return newDiagnostic(
			FailureInvalid,
			InvalidSchemaRootCode,
			root.loc,
			fmt.Sprintf("expected XSD schema root, got <%s>", renderSyntaxName(root.name)),
			nil,
		)
	}
	return validateSchemaRootContents(document)
}

func validateSchemaRootContents(document *syntaxDocument) error {
	root := document.root
	unsupportedRootAttribute, unsupportedRootAttributeLoc, err := validateSchemaRootAttributes(root)
	if err != nil {
		return err
	}
	version, err := syntaxDocumentVersion(document)
	if err != nil {
		return err
	}
	return validateSchemaRootChildren(root, version, unsupportedRootAttributeLoc, unsupportedRootAttribute)
}

func validateSchemaRootChildren(root *syntaxElement, version XSDVersion, unsupportedRootAttributeLoc Loc, unsupportedRootAttribute string) error {
	phase := schemaGrammarMetadata
	for _, node := range root.children {
		child, err := schemaRootChildElement(node)
		if err != nil {
			return err
		}
		if child == nil {
			continue
		}
		nextPhase, err := validateSchemaRootChild(child, phase, version)
		if err != nil {
			return preferSchemaUnsupported(err, unsupportedRootAttributeLoc, unsupportedRootAttribute)
		}
		phase = nextPhase
	}
	if unsupportedRootAttribute != "" {
		return newSchemaSyntaxUnsupported(unsupportedRootAttributeLoc, unsupportedRootAttribute)
	}
	return nil
}

func schemaRootChildElement(node syntaxNode) (*syntaxElement, error) {
	textNode, ok := node.(syntaxText)
	if ok {
		if xmlWhitespace([]byte(textNode.data)) {
			return nil, nil
		}
		return nil, newSchemaCompositionDiagnostic(textNode.loc, "schema root contains non-whitespace character data")
	}
	child, ok := node.(*syntaxElement)
	if !ok {
		return nil, newSchemaBridgeInvariant(Loc{}, "schema root contains an unknown syntax node")
	}
	return child, nil
}

func validateSchemaRootChild(child *syntaxElement, phase schemaGrammarPhase, version XSDVersion) (schemaGrammarPhase, error) {
	if child.name.namespace != xsdNamespaceURI {
		return phase, newSchemaCompositionDiagnostic(child.loc, "schema root contains a forbidden non-XSD construct")
	}
	switch child.name.local {
	case "annotation":
		return phase, validateSchemaAnnotationElement(child)
	case "include", "import":
		return validateSchemaRootComposition(child, phase)
	case "element", "attribute", "simpleType", "complexType", "group", "attributeGroup", "notation":
		return validateSchemaRootDeclaration(child, version)
	case "redefine", "override":
		if phase == schemaGrammarDefaultOpenContent || phase == schemaGrammarDeclarations {
			return phase, newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("schema child <%s> follows global declarations", child.name.local))
		}
		return schemaGrammarComposition, newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("XSD schema child <%s> is not implemented", child.name.local))
	case "defaultOpenContent":
		if phase == schemaGrammarDefaultOpenContent || phase == schemaGrammarDeclarations {
			return phase, newSchemaCompositionDiagnostic(child.loc, "schema child <defaultOpenContent> is not permitted after global declarations")
		}
		return schemaGrammarDefaultOpenContent, newSchemaSyntaxUnsupported(child.loc, "XSD schema child <defaultOpenContent> is not implemented")
	case "defaultAttributes":
		return phase, newSchemaCompositionDiagnostic(child.loc, "schema root contains forbidden child <defaultAttributes>")
	default:
		return phase, validateSchemaRootForbiddenOrUnsupported(child)
	}
}

func validateSchemaRootComposition(child *syntaxElement, phase schemaGrammarPhase) (schemaGrammarPhase, error) {
	if phase == schemaGrammarDefaultOpenContent || phase == schemaGrammarDeclarations {
		return phase, newSchemaCompositionDiagnostic(child.loc, "schema composition follows a global declaration")
	}
	kind := syntaxReferenceInclude
	if child.name.local == "import" {
		kind = syntaxReferenceImport
	}
	if err := validateSchemaReferenceElement(child, kind); err != nil {
		return phase, err
	}
	return schemaGrammarComposition, nil
}

func validateSchemaRootDeclaration(child *syntaxElement, version XSDVersion) (schemaGrammarPhase, error) {
	if err := validateGlobalSchemaDeclaration(child, version); err != nil {
		return schemaGrammarDeclarations, err
	}
	return schemaGrammarDeclarations, nil
}

func validateSchemaRootForbiddenOrUnsupported(child *syntaxElement) error {
	if isKnownSchemaElement(child.name.local) {
		return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("XSD schema child <%s> is not permitted here", child.name.local))
	}
	return newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("XSD schema child <%s> is not implemented", child.name.local))
}

func validateSchemaRootAttributes(element *syntaxElement) (string, Loc, error) {
	unsupportedMessage := ""
	unsupportedLoc := Loc{}
	for _, attribute := range element.attrs {
		message, err := validateSchemaRootAttribute(element, attribute)
		if err != nil {
			return "", Loc{}, err
		}
		if message != "" && unsupportedMessage == "" {
			unsupportedMessage = message
			unsupportedLoc = attribute.loc
		}
	}
	return unsupportedMessage, unsupportedLoc, nil
}

func validateSchemaRootAttribute(element *syntaxElement, attribute syntaxAttribute) (string, error) {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return "", nil
	}
	if attribute.name.namespace != "" {
		return "", validateSchemaQualifiedAttribute(attribute, "schema root")
	}
	return validateSchemaRootUnqualifiedAttribute(element, attribute)
}

func validateSchemaQualifiedAttribute(attribute syntaxAttribute, owner string) error {
	if attribute.name.namespace == xsdNamespaceURI {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", owner, attribute.name.local))
	}
	if attribute.name.namespace == xmlNamespaceURI {
		return validateSchemaXMLAttribute(attribute)
	}
	return nil
}

func validateSchemaXMLAttribute(attribute syntaxAttribute) error {
	switch attribute.name.local {
	case "lang":
		return validateXMLLanguage(attribute)
	case "base":
		return validateSchemaAnyURI(attribute)
	default:
		return nil
	}
}

//nolint:gocognit // Root attributes have distinct lexical and support outcomes.
func validateSchemaRootUnqualifiedAttribute(element *syntaxElement, attribute syntaxAttribute) (string, error) {
	switch attribute.name.local {
	case "targetNamespace":
		if err := validateSchemaAnyURI(attribute); err != nil {
			return "", err
		}
		if collapseXMLWhitespace(attribute.value) == "" {
			return "", newDiagnostic(FailureInvalid, invalidSchemaTargetNamespaceCode, attribute.loc, "schema targetNamespace cannot be empty when present", nil)
		}
	case "id":
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return "", newSchemaCompositionDiagnostic(attribute.loc, "schema root id must be a valid NCName")
		}
	case "version":
		_ = collapseXMLWhitespace(attribute.value)
	case "attributeFormDefault", "elementFormDefault":
		if err := validateSchemaEnum(attribute, "qualified", "unqualified"); err != nil {
			return "", err
		}
		return fmt.Sprintf("schema root attribute %q is not implemented", attribute.name.local), nil
	case "blockDefault":
		if err := validateSchemaRestrictionList(attribute, "extension", "restriction", "substitution"); err != nil {
			return "", err
		}
		return "schema root attribute \"blockDefault\" is not implemented", nil
	case "finalDefault":
		if err := validateSchemaRestrictionList(attribute, "extension", "restriction", "list", "union"); err != nil {
			return "", err
		}
		return "schema root attribute \"finalDefault\" is not implemented", nil
	case "defaultAttributes":
		if err := validateConditionalQNameForSchema(element, attribute); err != nil {
			return "", err
		}
		return "schema root attribute \"defaultAttributes\" is not implemented", nil
	case "xpathDefaultNamespace":
		if err := validateSchemaXPathDefaultNamespace(attribute); err != nil {
			return "", err
		}
		return fmt.Sprintf("schema root attribute %q is not implemented", attribute.name.local), nil
	default:
		return "", newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("schema root has unknown attribute %q", attribute.name.local))
	}
	return "", nil
}

func validateSchemaEnum(attribute syntaxAttribute, values ...string) error {
	value := collapseXMLWhitespace(attribute.value)
	for _, allowed := range values {
		if value == allowed {
			return nil
		}
	}
	return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid value", attribute.name.local))
}

func validateSchemaBoolean(attribute syntaxAttribute) error {
	switch collapseXMLWhitespace(attribute.value) {
	case "true", "false", "1", "0":
		return nil
	default:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid boolean value", attribute.name.local))
	}
}

func validateSchemaRestrictionList(attribute syntaxAttribute, values ...string) error {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		return nil
	}
	tokens := strings.Split(lexeme, " ")
	for _, token := range tokens {
		if token == "#all" {
			if len(tokens) != 1 {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q cannot combine #all with other values", attribute.name.local))
			}
			continue
		}
		known := false
		for _, allowed := range values {
			if token == allowed {
				known = true
				break
			}
		}
		if !known {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid list value", attribute.name.local))
		}
	}
	return nil
}

func validateSchemaXPathDefaultNamespace(attribute syntaxAttribute) error {
	value := collapseXMLWhitespace(attribute.value)
	switch value {
	case "##defaultNamespace", "##targetNamespace", "##local":
		return nil
	default:
		return validateSchemaAnyURI(attribute)
	}
}

func validateSchemaAnyURI(attribute syntaxAttribute) error {
	value := collapseXMLWhitespace(attribute.value)
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character < 0x20 || character == 0x7f {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid anyURI value", attribute.name.local))
		}
		if character != '%' {
			continue
		}
		if index+2 >= len(value) || !isHexDigit(value[index+1]) || !isHexDigit(value[index+2]) {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid anyURI value", attribute.name.local))
		}
		index += 2
	}
	if _, err := url.Parse(value); err != nil {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid anyURI value", attribute.name.local))
	}
	return nil
}

func isHexDigit(character byte) bool {
	return character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F'
}

type schemaAttributeStatus uint8

const (
	schemaAttributeAllowed schemaAttributeStatus = iota + 1
	schemaAttributeUnsupported
	schemaAttributeForbidden
)

func validateGlobalSchemaDeclaration(element *syntaxElement, version XSDVersion) error {
	kind, ok := schemaDeclarationKind(element.name.local)
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "global declaration has an unknown kind")
	}
	unsupportedMessage := ""
	unsupportedLoc := Loc{}
	for _, attribute := range element.attrs {
		message, err := validateGlobalSchemaAttribute(element, kind, attribute)
		if err != nil {
			return err
		}
		if message != "" && unsupportedMessage == "" {
			unsupportedMessage = message
			unsupportedLoc = attribute.loc
		}
	}
	if err := validateGlobalSchemaNamePresence(element); err != nil {
		return err
	}
	if err := validateGlobalSchemaAttributeCooccurrence(element); err != nil {
		return err
	}
	if err := validateGlobalSchemaChildren(element, version); err != nil {
		return preferSchemaUnsupported(err, unsupportedLoc, unsupportedMessage)
	}
	if unsupportedMessage != "" {
		return newSchemaSyntaxUnsupported(unsupportedLoc, unsupportedMessage)
	}
	return nil
}

func validateGlobalSchemaNamePresence(element *syntaxElement) error {
	attributes := syntaxAttributesByLocal(element, "name")
	if len(attributes) == 0 {
		return newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, element.loc, "schema declaration has no name attribute", nil)
	}
	if len(attributes) != 1 {
		return newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, element.loc, "schema declaration name must be unique", nil)
	}
	return nil
}

func preferSchemaUnsupported(err error, loc Loc, message string) error {
	if message == "" {
		return err
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
		return err
	}
	return newSchemaSyntaxUnsupported(loc, message)
}

func validateGlobalSchemaAttributeCooccurrence(element *syntaxElement) error {
	if element.name.local != "element" && element.name.local != "attribute" {
		return nil
	}
	defaults := syntaxAttributesByLocal(element, "default")
	fixed := syntaxAttributesByLocal(element, "fixed")
	if len(defaults) > 0 && len(fixed) > 0 {
		return newSchemaCompositionDiagnostic(fixed[0].loc, fmt.Sprintf("global %s cannot specify both default and fixed", element.name.local))
	}
	return nil
}

func validateGlobalSchemaAttribute(element *syntaxElement, kind ComponentKind, attribute syntaxAttribute) (string, error) {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return "", nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return "", newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("global declaration has forbidden attribute %q", attribute.name.local))
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return "", validateSchemaXMLAttribute(attribute)
		}
		return "", nil
	}
	status := globalSchemaAttributeStatus(kind, attribute.name.local)
	switch status {
	case schemaAttributeAllowed:
		return "", validateAllowedGlobalSchemaAttribute(attribute)
	case schemaAttributeUnsupported:
		if err := validateRecognizedUnsupportedAttribute(element, attribute); err != nil {
			return "", err
		}
		return fmt.Sprintf("global %s attribute %q is not implemented", element.name.local, attribute.name.local), nil
	case schemaAttributeForbidden:
		return "", newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("global %s attribute %q is not permitted", element.name.local, attribute.name.local))
	default:
		return "", newSchemaBridgeInvariant(attribute.loc, "global declaration attribute has an unknown status")
	}
}

func validateAllowedGlobalSchemaAttribute(attribute syntaxAttribute) error {
	if attribute.name.local == "name" {
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, attribute.loc, "schema declaration name must be an unqualified valid NCName", nil)
		}
		return nil
	}
	if attribute.name.local == "id" && !validNCName(collapseXMLWhitespace(attribute.value)) {
		return newSchemaCompositionDiagnostic(attribute.loc, "schema declaration id must be a valid NCName")
	}
	return nil
}

func globalSchemaAttributeStatus(kind ComponentKind, local string) schemaAttributeStatus {
	if local == "name" || local == "id" {
		return schemaAttributeAllowed
	}
	switch kind {
	case ComponentKindElementDeclaration:
		return elementSchemaAttributeStatus(local)
	case ComponentKindAttributeDeclaration:
		return attributeSchemaAttributeStatus(local)
	case ComponentKindSimpleTypeDefinition:
		if local == "final" {
			return schemaAttributeUnsupported
		}
	case ComponentKindComplexTypeDefinition:
		return complexTypeSchemaAttributeStatus(local)
	case ComponentKindModelGroupDefinition:
		return schemaAttributeForbidden
	case ComponentKindAttributeGroupDefinition:
		return schemaAttributeForbidden
	case ComponentKindNotationDeclaration:
		if local == "public" || local == "system" {
			return schemaAttributeUnsupported
		}
	}
	return schemaAttributeForbidden
}

func elementSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "abstract", "block", "default", "fixed", "nillable", "substitutionGroup", "targetNamespace", "type", "final":
		return schemaAttributeUnsupported
	default:
		return schemaAttributeForbidden
	}
}

func attributeSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "default", "fixed", "targetNamespace", "type", "inheritable":
		return schemaAttributeUnsupported
	default:
		return schemaAttributeForbidden
	}
}

func complexTypeSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "abstract", "block", "final", "mixed", "defaultAttributesApply":
		return schemaAttributeUnsupported
	default:
		return schemaAttributeForbidden
	}
}

func validateRecognizedUnsupportedAttribute(element *syntaxElement, attribute syntaxAttribute) error {
	switch attribute.name.local {
	case "type", "ref", "itemType", "base":
		return validateConditionalQName(element, attribute)
	case "substitutionGroup":
		return parseConditionalQNameList(element, attribute)
	case "memberTypes":
		return parseConditionalQNameList(element, attribute)
	case "abstract", "nillable", "mixed", "inheritable", "defaultAttributesApply":
		return validateSchemaBoolean(attribute)
	case "block":
		if element.name.local == "element" {
			return validateSchemaRestrictionList(attribute, "extension", "restriction", "substitution")
		}
		return validateSchemaRestrictionList(attribute, "extension", "restriction")
	case "final":
		if element.name.local == "simpleType" {
			return validateSchemaRestrictionList(attribute, "extension", "restriction", "list", "union")
		}
		return validateSchemaRestrictionList(attribute, "extension", "restriction")
	case "targetNamespace", "system":
		return validateSchemaAnyURI(attribute)
	case "public":
		_ = collapseXMLWhitespace(attribute.value)
	}
	return nil
}

func validateConditionalQName(element *syntaxElement, attribute syntaxAttribute) error {
	return validateConditionalQNameForSchema(element, attribute)
}

func validateConditionalQNameForSchema(element *syntaxElement, attribute syntaxAttribute) error {
	value := collapseXMLWhitespace(attribute.value)
	if value == "" {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an empty QName", attribute.name.local))
	}
	prefix, local, ok := splitConditionalQName(value)
	if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
		return newDiagnostic(FailureInvalid, invalidSchemaConditionalCode, attribute.loc, fmt.Sprintf("attribute %q has a malformed QName", attribute.name.local), nil)
	}
	if prefix == "" || element == nil {
		return nil
	}
	_, bound := element.scope.lookup(prefix)
	if !bound {
		return newDiagnostic(FailureInvalid, invalidSchemaConditionalCode, attribute.loc, fmt.Sprintf("attribute %q has an unbound QName prefix", attribute.name.local), nil)
	}
	return nil
}

func validateGlobalSchemaChildren(element *syntaxElement, version XSDVersion) error {
	children, err := collectGlobalSchemaChildren(element)
	if err != nil {
		return err
	}
	switch element.name.local {
	case "element":
		return validateElementGlobalChildren(element, children, len(syntaxAttributesByLocal(element, "type")) > 0)
	case "attribute":
		return validateAttributeGlobalChildren(element, children, len(syntaxAttributesByLocal(element, "type")) > 0)
	case "simpleType":
		return validateSimpleTypeGlobalChildren(element, children, version)
	case "complexType":
		return validateComplexTypeGlobalChildren(element, children)
	case "group":
		return validateGroupGlobalChildren(element, children)
	case "attributeGroup":
		return validateAttributeGroupGlobalChildren(element, children)
	case "notation":
		return validateNotationGlobalChildren(element, children)
	default:
		return newSchemaBridgeInvariant(element.loc, "global declaration has an unknown child model")
	}
}

func collectGlobalSchemaChildren(element *syntaxElement) ([]*syntaxElement, error) {
	children := make([]*syntaxElement, 0, len(element.children))
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if xmlWhitespace([]byte(textNode.data)) {
				continue
			}
			return nil, newSchemaCompositionDiagnostic(textNode.loc, fmt.Sprintf("global %s contains non-whitespace character data", element.name.local))
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return nil, newSchemaBridgeInvariant(Loc{}, "global declaration contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			return nil, newSchemaCompositionDiagnostic(child.loc, "global declaration contains a forbidden non-XSD child")
		}
		if child.name.local == "annotation" {
			if err := validateSchemaAnnotationElement(child); err != nil {
				return nil, err
			}
		}
		children = append(children, child)
	}
	return children, nil
}

type schemaChildUnsupportedCandidate struct {
	loc     Loc
	message string
	present bool
}

func (candidate *schemaChildUnsupportedCandidate) consider(child *syntaxElement, parent string) {
	if candidate.present {
		return
	}
	candidate.loc = child.loc
	candidate.message = fmt.Sprintf("global %s child <%s> is not implemented", parent, child.name.local)
	candidate.present = true
}

func (candidate schemaChildUnsupportedCandidate) err() error {
	if !candidate.present {
		return nil
	}
	return newSchemaSyntaxUnsupported(candidate.loc, candidate.message)
}

func consumeGlobalSchemaAnnotation(child *syntaxElement, annotationSeen, contentSeen *bool) (bool, error) {
	if child.name.local != "annotation" {
		*contentSeen = true
		return false, nil
	}
	if *annotationSeen || *contentSeen {
		return false, newSchemaCompositionDiagnostic(child.loc, "global declaration annotation must be first and unique")
	}
	*annotationSeen = true
	return true, nil
}

func forbiddenGlobalSchemaChild(parent string, child *syntaxElement) error {
	if isKnownSchemaElement(child.name.local) {
		return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("global %s contains forbidden child <%s>", parent, child.name.local))
	}
	return nil
}

type elementGlobalChildPhase uint8

const (
	elementGlobalTypePhase elementGlobalChildPhase = iota
	elementGlobalAlternativePhase
	elementGlobalConstraintPhase
)

//nolint:gocognit // Keep the ordered element child grammar explicit.
func validateElementGlobalChildren(parent *syntaxElement, children []*syntaxElement, typeAttributeSeen bool) error {
	annotationSeen := false
	contentSeen := false
	typeSeen := false
	phase := elementGlobalTypePhase
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		switch child.name.local {
		case "simpleType", "complexType":
			if typeAttributeSeen {
				return newSchemaCompositionDiagnostic(child.loc, "element cannot combine type attribute with an inline type")
			}
			if typeSeen || phase != elementGlobalTypePhase {
				return newSchemaCompositionDiagnostic(child.loc, "element type child must be unique and precede constraints")
			}
			typeSeen = true
			phase = elementGlobalAlternativePhase
			candidate.consider(child, parent.name.local)
		case "alternative":
			if phase == elementGlobalConstraintPhase {
				return newSchemaCompositionDiagnostic(child.loc, "element alternative must precede identity constraints")
			}
			phase = elementGlobalAlternativePhase
			candidate.consider(child, parent.name.local)
		case "unique", "key", "keyref":
			phase = elementGlobalConstraintPhase
			candidate.consider(child, parent.name.local)
		default:
			if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
				return err
			}
			candidate.consider(child, parent.name.local)
		}
	}
	return candidate.err()
}

func validateAttributeGlobalChildren(parent *syntaxElement, children []*syntaxElement, typeAttributeSeen bool) error {
	annotationSeen := false
	contentSeen := false
	simpleTypeSeen := false
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		if child.name.local == "simpleType" {
			if typeAttributeSeen {
				return newSchemaCompositionDiagnostic(child.loc, "attribute cannot combine type attribute with an inline simpleType")
			}
			if simpleTypeSeen {
				return newSchemaCompositionDiagnostic(child.loc, "attribute simpleType child must be unique")
			}
			simpleTypeSeen = true
			candidate.consider(child, parent.name.local)
			continue
		}
		if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
			return err
		}
		candidate.consider(child, parent.name.local)
	}
	return candidate.err()
}

func validateSimpleTypeGlobalChildren(parent *syntaxElement, children []*syntaxElement, version XSDVersion) error {
	annotationSeen := false
	contentSeen := false
	modelSeen := false
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		if err := validateSimpleTypeGlobalModelChild(parent, child, &modelSeen, &candidate, version); err != nil {
			return err
		}
	}
	if !modelSeen {
		return newSchemaCompositionDiagnostic(parent.loc, "simpleType requires one restriction, list, or union child")
	}
	return candidate.err()
}

func validateSimpleTypeGlobalModelChild(parent, child *syntaxElement, modelSeen *bool, candidate *schemaChildUnsupportedCandidate, version XSDVersion) error {
	switch child.name.local {
	case "restriction":
		if *modelSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simpleType requires exactly one model child")
		}
		*modelSeen = true
		return validateSimpleTypeRestriction(child, version)
	case "list", "union":
		if *modelSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simpleType requires exactly one model child")
		}
		*modelSeen = true
		candidate.consider(child, parent.name.local)
		return nil
	default:
		if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
			return err
		}
		candidate.consider(child, parent.name.local)
		return nil
	}
}

func validateSimpleTypeRestriction(element *syntaxElement, version XSDVersion) error {
	if err := validateSimpleTypeRestrictionAttributes(element); err != nil {
		return err
	}
	return validateSimpleTypeRestrictionChildren(element, version)
}

func validateSimpleTypeRestrictionAttributes(element *syntaxElement) error {
	baseAttributes := syntaxAttributesByLocal(element, "base")
	if len(baseAttributes) == 0 {
		if inline := inlineSimpleTypeChild(element); inline != nil {
			return newSchemaSyntaxUnsupported(inline.loc, "inline anonymous simple types in restrictions are not implemented")
		}
		return newSchemaCompositionDiagnostic(element.loc, "simple type restriction requires a base attribute")
	}
	if len(baseAttributes) != 1 {
		return newSchemaCompositionDiagnostic(element.loc, "simple type restriction base attribute must be unique")
	}
	for _, attribute := range element.attrs {
		if err := validateSimpleTypeRestrictionAttribute(element, attribute); err != nil {
			return err
		}
	}
	return nil
}

func inlineSimpleTypeChild(element *syntaxElement) *syntaxElement {
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if child.name.namespace == xsdNamespaceURI && child.name.local == "simpleType" {
			return child
		}
	}
	return nil
}

func validateSimpleTypeRestrictionAttribute(element *syntaxElement, attribute syntaxAttribute) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("simple type restriction has forbidden attribute %q", attribute.name.local))
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return validateSchemaXMLAttribute(attribute)
		}
		return nil
	}
	switch attribute.name.local {
	case "base":
		return validateConditionalQNameForSchema(element, attribute)
	case "id":
		if validNCName(collapseXMLWhitespace(attribute.value)) {
			return nil
		}
		return newSchemaCompositionDiagnostic(attribute.loc, "simple type restriction id must be a valid NCName")
	default:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("simple type restriction has forbidden attribute %q", attribute.name.local))
	}
}

func validateSimpleTypeRestrictionChildren(element *syntaxElement, version XSDVersion) error {
	children, err := collectSimpleTypeChildren(element, element.name.local+" facet")
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	totalSeen := false
	fractionSeen := false
	for _, child := range children {
		if err := validateSimpleTypeRestrictionChild(child, &annotationSeen, &contentSeen, &totalSeen, &fractionSeen, version); err != nil {
			return err
		}
	}
	return nil
}

func validateSimpleTypeRestrictionChild(child *syntaxElement, annotationSeen, contentSeen, totalSeen, fractionSeen *bool, version XSDVersion) error {
	if child.name.local == "annotation" {
		if *annotationSeen || *contentSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction annotation must be first and unique")
		}
		*annotationSeen = true
		return nil
	}
	*contentSeen = true
	return validateSimpleTypeRestrictionFacet(child, totalSeen, fractionSeen, version)
}

func validateSimpleTypeRestrictionFacet(child *syntaxElement, totalSeen, fractionSeen *bool, version XSDVersion) error {
	switch child.name.local {
	case "totalDigits":
		if *totalSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction totalDigits facet must be unique")
		}
		*totalSeen = true
		return validateSimpleTypeDigitFacet(child)
	case "fractionDigits":
		if *fractionSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction fractionDigits facet must be unique")
		}
		*fractionSeen = true
		return validateSimpleTypeDigitFacet(child)
	default:
		return unsupportedSimpleTypeRestrictionChild(child, version)
	}
}

func unsupportedSimpleTypeRestrictionChild(child *syntaxElement, version XSDVersion) error {
	if isUnsupportedSimpleTypeFacet(child.name.local) {
		feature, ok := LookupUnsupportedFeature(FeatureDatatypeFacets)
		if !ok {
			return newDiagnostic(
				FailureInternal,
				diagnosticUnregisteredFeatureCode,
				child.loc,
				"datatype facet feature is not registered",
				nil,
			)
		}
		return newUnsupportedForVersion(
			feature,
			UnsupportedDatatypeFacetCode,
			child.loc,
			fmt.Sprintf("simple type restriction facet <%s> is not implemented", child.name.local),
			version,
		)
	}
	if isKnownSchemaElement(child.name.local) {
		return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("simple type restriction contains forbidden child <%s>", child.name.local))
	}
	return newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("simple type restriction child <%s> is not implemented", child.name.local))
}

func validateSimpleTypeDigitFacet(element *syntaxElement) error {
	if err := validateSimpleTypeDigitFacetAttributes(element); err != nil {
		return err
	}
	return validateSimpleTypeDigitFacetChildren(element)
}

func validateSimpleTypeDigitFacetAttributes(element *syntaxElement) error {
	valueAttributes := syntaxAttributesByLocal(element, "value")
	if len(valueAttributes) == 0 {
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" facet requires a value attribute")
	}
	if len(valueAttributes) != 1 {
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" facet value attribute must be unique")
	}
	fixedAttributes := syntaxAttributesByLocal(element, "fixed")
	if len(fixedAttributes) > 1 {
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" facet fixed attribute must be unique")
	}
	for _, attribute := range element.attrs {
		if err := validateSimpleTypeDigitFacetAttribute(element, attribute); err != nil {
			return err
		}
	}
	return nil
}

func validateSimpleTypeDigitFacetAttribute(element *syntaxElement, attribute syntaxAttribute) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s facet has forbidden attribute %q", element.name.local, attribute.name.local))
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return validateSchemaXMLAttribute(attribute)
		}
		return nil
	}
	switch attribute.name.local {
	case "value":
		return nil
	case "fixed":
		return validateSchemaBoolean(attribute)
	case "id":
		if validNCName(collapseXMLWhitespace(attribute.value)) {
			return nil
		}
		return newSchemaCompositionDiagnostic(attribute.loc, element.name.local+" facet id must be a valid NCName")
	default:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s facet has forbidden attribute %q", element.name.local, attribute.name.local))
	}
}

func validateSimpleTypeDigitFacetChildren(element *syntaxElement) error {
	children, err := collectSimpleTypeChildren(element, element.name.local+" facet")
	if err != nil {
		return err
	}
	annotationSeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		return newSchemaSyntaxUnsupported(child.loc, element.name.local+" facet child <"+child.name.local+"> is not implemented")
	}
	return nil
}

func collectSimpleTypeChildren(element *syntaxElement, owner string) ([]*syntaxElement, error) {
	children := make([]*syntaxElement, 0, len(element.children))
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if xmlWhitespace([]byte(textNode.data)) {
				continue
			}
			return nil, newSchemaCompositionDiagnostic(textNode.loc, owner+" contains non-whitespace character data")
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return nil, newSchemaBridgeInvariant(Loc{}, owner+" contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			return nil, newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("%s contains an unsupported foreign child <%s>", owner, child.name.local))
		}
		if child.name.local == "annotation" {
			if err := validateSchemaAnnotationElement(child); err != nil {
				return nil, err
			}
		}
		children = append(children, child)
	}
	return children, nil
}

func isUnsupportedSimpleTypeFacet(local string) bool {
	switch local {
	case "assertion", "assertions", "enumeration", "explicitTimezone", "length", "maxExclusive", "maxInclusive", "maxLength", "maxScale", "minExclusive", "minInclusive", "minLength", "minScale", "pattern", "precision", "whiteSpace":
		return true
	default:
		return false
	}
}

//nolint:gocognit,funlen // Keep mutually-exclusive complexType grammar branches explicit.
func validateComplexTypeGlobalChildren(parent *syntaxElement, children []*syntaxElement) error {
	annotationSeen := false
	contentSeen := false
	specialSeen := false
	openContentSeen := false
	modelSeen := false
	attributesSeen := false
	anyAttributeSeen := false
	assertSeen := false
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		switch child.name.local {
		case "simpleContent", "complexContent":
			if specialSeen || openContentSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType content model is mutually exclusive")
			}
			specialSeen = true
			candidate.consider(child, parent.name.local)
		case "openContent":
			if specialSeen || openContentSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType openContent must precede the model and attributes")
			}
			openContentSeen = true
			candidate.consider(child, parent.name.local)
		case "group", "all", "choice", "sequence":
			if specialSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType model child must be unique and precede attributes")
			}
			modelSeen = true
			candidate.consider(child, parent.name.local)
		case "attribute", "attributeGroup":
			if specialSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType attributes must follow the model and precede anyAttribute/assert")
			}
			attributesSeen = true
			candidate.consider(child, parent.name.local)
		case "anyAttribute":
			if specialSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType anyAttribute must be unique and last among attributes")
			}
			anyAttributeSeen = true
			candidate.consider(child, parent.name.local)
		case "assert":
			if specialSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType assert cannot follow simpleContent or complexContent")
			}
			assertSeen = true
			candidate.consider(child, parent.name.local)
		default:
			if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
				return err
			}
			candidate.consider(child, parent.name.local)
		}
	}
	return candidate.err()
}

func validateGroupGlobalChildren(parent *syntaxElement, children []*syntaxElement) error {
	annotationSeen := false
	contentSeen := false
	modelSeen := false
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		switch child.name.local {
		case "all", "choice", "sequence":
			if modelSeen {
				return newSchemaCompositionDiagnostic(child.loc, "group requires exactly one model child")
			}
			modelSeen = true
			candidate.consider(child, parent.name.local)
		default:
			if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
				return err
			}
			candidate.consider(child, parent.name.local)
		}
	}
	if !modelSeen {
		return newSchemaCompositionDiagnostic(parent.loc, "group requires one all, choice, or sequence child")
	}
	return candidate.err()
}

//nolint:gocognit // Keep the attribute-group order and cardinality grammar explicit.
func validateAttributeGroupGlobalChildren(parent *syntaxElement, children []*syntaxElement) error {
	annotationSeen := false
	contentSeen := false
	anyAttributeSeen := false
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		switch child.name.local {
		case "attribute", "attributeGroup":
			if anyAttributeSeen {
				return newSchemaCompositionDiagnostic(child.loc, "attributeGroup attributes must precede anyAttribute")
			}
			candidate.consider(child, parent.name.local)
		case "anyAttribute":
			if anyAttributeSeen {
				return newSchemaCompositionDiagnostic(child.loc, "attributeGroup anyAttribute must be unique")
			}
			anyAttributeSeen = true
			candidate.consider(child, parent.name.local)
		default:
			if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
				return err
			}
			candidate.consider(child, parent.name.local)
		}
	}
	return candidate.err()
}

func validateNotationGlobalChildren(parent *syntaxElement, children []*syntaxElement) error {
	annotationSeen := false
	contentSeen := false
	var candidate schemaChildUnsupportedCandidate
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
			return err
		}
		candidate.consider(child, parent.name.local)
	}
	return candidate.err()
}

func isKnownSchemaElement(local string) bool {
	switch local {
	case "all", "annotation", "any", "anyAttribute", "appinfo", "assert", "assertion", "assertions", "alternative", "attribute", "attributeGroup", "choice", "complexContent", "complexType", "defaultOpenContent", "documentation", "element", "enumeration", "explicitTimezone", "extension", "field", "fractionDigits", "group", "import", "include", "key", "keyref", "length", "maxExclusive", "maxInclusive", "maxLength", "maxScale", "minExclusive", "minInclusive", "minLength", "minScale", "list", "notation", "openContent", "override", "pattern", "precision", "redefine", "restriction", "schema", "selector", "sequence", "simpleContent", "simpleType", "totalDigits", "union", "unique", "whiteSpace":
		return true
	default:
		return false
	}
}

func validateXMLLanguage(attribute syntaxAttribute) error {
	value := collapseXMLWhitespace(attribute.value)
	if value == "" {
		return newSchemaCompositionDiagnostic(attribute.loc, "xml:lang cannot be empty")
	}
	parts := strings.Split(value, "-")
	if !validLanguageSubtag(parts[0], true) {
		return newSchemaCompositionDiagnostic(attribute.loc, "xml:lang must be a language tag")
	}
	for _, part := range parts[1:] {
		if !validLanguageSubtag(part, false) {
			return newSchemaCompositionDiagnostic(attribute.loc, "xml:lang must be a language tag")
		}
	}
	return nil
}

func validLanguageSubtag(value string, first bool) bool {
	if len(value) < 1 || len(value) > 8 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		letter := character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
		digit := character >= '0' && character <= '9'
		if !letter && (!digit || first) {
			return false
		}
	}
	return true
}
