package goxsd9

import (
	"fmt"
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
	if err := validateSchemaRootAttributes(root); err != nil {
		return err
	}

	phase := schemaGrammarMetadata
	for _, node := range root.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if xmlWhitespace([]byte(textNode.data)) {
				continue
			}
			return newSchemaCompositionDiagnostic(textNode.loc, "schema root contains non-whitespace character data")
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(Loc{}, "schema root contains an unknown syntax node")
		}
		nextPhase, err := validateSchemaRootChild(child, phase)
		if err != nil {
			return err
		}
		phase = nextPhase
	}
	return nil
}

func validateSchemaRootChild(child *syntaxElement, phase schemaGrammarPhase) (schemaGrammarPhase, error) {
	if child.name.namespace != xsdNamespaceURI {
		return phase, newSchemaCompositionDiagnostic(child.loc, "schema root contains a forbidden non-XSD construct")
	}
	switch child.name.local {
	case "annotation":
		return phase, validateSchemaAnnotationElement(child)
	case "include", "import":
		return validateSchemaRootComposition(child, phase)
	case "element", "attribute", "simpleType", "complexType", "group", "attributeGroup", "notation":
		return validateSchemaRootDeclaration(child)
	case "redefine", "override", "defaultOpenContent", "defaultAttributes":
		return phase, newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("XSD schema child <%s> is not implemented", child.name.local))
	default:
		return phase, validateSchemaRootForbiddenOrUnsupported(child)
	}
}

func validateSchemaRootComposition(child *syntaxElement, phase schemaGrammarPhase) (schemaGrammarPhase, error) {
	if phase == schemaGrammarDeclarations {
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

func validateSchemaRootDeclaration(child *syntaxElement) (schemaGrammarPhase, error) {
	if err := validateGlobalSchemaDeclaration(child); err != nil {
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

func validateSchemaRootAttributes(element *syntaxElement) error {
	for _, attribute := range element.attrs {
		if err := validateSchemaRootAttribute(attribute); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaRootAttribute(attribute syntaxAttribute) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		return validateSchemaQualifiedAttribute(attribute, "schema root")
	}
	return validateSchemaRootUnqualifiedAttribute(attribute)
}

func validateSchemaQualifiedAttribute(attribute syntaxAttribute, owner string) error {
	if attribute.name.namespace == xsdNamespaceURI {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", owner, attribute.name.local))
	}
	if attribute.name.namespace == xmlNamespaceURI && attribute.name.local == "lang" {
		return validateXMLLanguage(attribute)
	}
	return nil
}

func validateSchemaRootUnqualifiedAttribute(attribute syntaxAttribute) error {
	switch attribute.name.local {
	case "targetNamespace":
		if collapseXMLWhitespace(attribute.value) == "" {
			return newDiagnostic(FailureInvalid, invalidSchemaTargetNamespaceCode, attribute.loc, "schema targetNamespace cannot be empty when present", nil)
		}
	case "name":
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newSchemaCompositionDiagnostic(attribute.loc, "schema root name must be a valid NCName")
		}
	case "id":
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newSchemaCompositionDiagnostic(attribute.loc, "schema root id must be a valid NCName")
		}
	case "version":
		_ = collapseXMLWhitespace(attribute.value)
	case "attributeFormDefault", "blockDefault", "defaultAttributes", "defaultAttributesApply", "elementFormDefault", "finalDefault", "xpathDefaultNamespace":
		return newSchemaSyntaxUnsupported(attribute.loc, fmt.Sprintf("schema root attribute %q is not implemented", attribute.name.local))
	default:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("schema root has unknown attribute %q", attribute.name.local))
	}
	return nil
}

type schemaAttributeStatus uint8

const (
	schemaAttributeAllowed schemaAttributeStatus = iota + 1
	schemaAttributeUnsupported
	schemaAttributeForbidden
)

func validateGlobalSchemaDeclaration(element *syntaxElement) error {
	kind, ok := schemaDeclarationKind(element.name.local)
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "global declaration has an unknown kind")
	}
	for _, attribute := range element.attrs {
		if err := validateGlobalSchemaAttribute(element, kind, attribute); err != nil {
			return err
		}
	}
	return validateGlobalSchemaChildren(element)
}

func validateGlobalSchemaAttribute(element *syntaxElement, kind ComponentKind, attribute syntaxAttribute) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("global declaration has forbidden attribute %q", attribute.name.local))
		}
		if attribute.name.namespace == xmlNamespaceURI && attribute.name.local == "lang" {
			return validateXMLLanguage(attribute)
		}
		return nil
	}
	status := globalSchemaAttributeStatus(kind, attribute.name.local)
	switch status {
	case schemaAttributeAllowed:
		return validateAllowedGlobalSchemaAttribute(attribute)
	case schemaAttributeUnsupported:
		if err := validateRecognizedUnsupportedAttribute(element, attribute); err != nil {
			return err
		}
		return newSchemaSyntaxUnsupported(attribute.loc, fmt.Sprintf("global %s attribute %q is not implemented", element.name.local, attribute.name.local))
	case schemaAttributeForbidden:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("global %s attribute %q is not permitted", element.name.local, attribute.name.local))
	default:
		return newSchemaBridgeInvariant(attribute.loc, "global declaration attribute has an unknown status")
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
	case "abstract", "block", "default", "fixed", "nillable", "substitutionGroup", "type", "final":
		return schemaAttributeUnsupported
	default:
		return schemaAttributeForbidden
	}
}

func attributeSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "default", "fixed", "type", "inheritable":
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
	case "type", "ref", "substitutionGroup", "itemType", "base":
		return validateConditionalQName(element, attribute)
	case "memberTypes":
		return parseConditionalQNameList(element, attribute)
	case "abstract", "nillable", "mixed":
		if collapseXMLWhitespace(attribute.value) == "" {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("global attribute %q has an empty lexical value", attribute.name.local))
		}
	}
	return nil
}

func validateConditionalQName(element *syntaxElement, attribute syntaxAttribute) error {
	value := collapseXMLWhitespace(attribute.value)
	if value == "" {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an empty QName", attribute.name.local))
	}
	prefix, local, ok := splitConditionalQName(value)
	if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
		return newDiagnostic(FailureInvalid, invalidSchemaConditionalCode, attribute.loc, fmt.Sprintf("attribute %q has a malformed QName", attribute.name.local), nil)
	}
	_, bound := element.scope.lookup(prefix)
	if prefix != "" && !bound {
		return newDiagnostic(FailureInvalid, invalidSchemaConditionalCode, attribute.loc, fmt.Sprintf("attribute %q has an unbound QName prefix", attribute.name.local), nil)
	}
	return nil
}

func validateGlobalSchemaChildren(element *syntaxElement) error {
	annotationSeen := false
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if xmlWhitespace([]byte(textNode.data)) {
				continue
			}
			return newSchemaCompositionDiagnostic(textNode.loc, fmt.Sprintf("global %s contains non-whitespace character data", element.name.local))
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(Loc{}, "global declaration contains an unknown syntax node")
		}
		seen, err := validateGlobalSchemaChild(element, child, annotationSeen)
		if err != nil {
			return err
		}
		annotationSeen = seen
	}
	return nil
}

func validateGlobalSchemaChild(parent, child *syntaxElement, annotationSeen bool) (bool, error) {
	if child.name.namespace != xsdNamespaceURI {
		return annotationSeen, newSchemaCompositionDiagnostic(child.loc, "global declaration contains a forbidden non-XSD child")
	}
	if child.name.local == "annotation" {
		if annotationSeen {
			return annotationSeen, newSchemaCompositionDiagnostic(child.loc, "global declaration contains more than one annotation")
		}
		if err := validateSchemaAnnotationElement(child); err != nil {
			return annotationSeen, err
		}
		return true, nil
	}
	if globalChildIsStructurallyForbidden(parent.name.local, child.name.local) {
		return annotationSeen, newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("global %s contains forbidden child <%s>", parent.name.local, child.name.local))
	}
	return annotationSeen, newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("global %s child <%s> is not implemented", parent.name.local, child.name.local))
}

func globalChildIsStructurallyForbidden(parent, child string) bool {
	if !isKnownSchemaElement(child) {
		return false
	}
	switch parent {
	case "element":
		return !schemaElementChildAllowed(child, "annotation", "simpleType", "complexType", "alternative", "unique", "key", "keyref")
	case "attribute":
		return !schemaElementChildAllowed(child, "annotation", "simpleType")
	case "simpleType":
		return !schemaElementChildAllowed(child, "annotation", "restriction", "list", "union")
	case "complexType":
		return !schemaElementChildAllowed(child, "annotation", "simpleContent", "complexContent", "openContent", "group", "all", "choice", "sequence", "attribute", "attributeGroup", "anyAttribute", "assert")
	case "group":
		return !schemaElementChildAllowed(child, "annotation", "all", "choice", "sequence")
	case "attributeGroup":
		return !schemaElementChildAllowed(child, "annotation", "attribute", "attributeGroup", "anyAttribute")
	case "notation":
		return child != "annotation"
	default:
		return true
	}
}

func schemaElementChildAllowed(child string, allowed ...string) bool {
	for _, candidate := range allowed {
		if child == candidate {
			return true
		}
	}
	return false
}

func isKnownSchemaElement(local string) bool {
	switch local {
	case "all", "annotation", "any", "anyAttribute", "appinfo", "assert", "assertion", "alternative", "attribute", "attributeGroup", "choice", "complexContent", "complexType", "defaultOpenContent", "documentation", "element", "extension", "field", "group", "import", "include", "key", "keyref", "list", "notation", "openContent", "override", "redefine", "restriction", "schema", "selector", "sequence", "simpleContent", "simpleType", "union", "unique":
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
