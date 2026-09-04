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

func applySchemaConditionalsWithPolicy(document *syntaxDocument, policy LanguagePolicy) error {
	version, err := xsdVersionForLanguagePolicy(policy)
	if err != nil {
		return invalidLanguagePolicyDiagnostic(policy, err)
	}
	if document == nil || document.root == nil {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxDocumentNoRootCode,
			Loc{},
			"syntax document has no root element",
			nil,
		)
	}

	var candidate schemaChildUnsupportedCandidate
	state, err := evaluateSchemaConditional(document.root, policy, version)
	if err != nil {
		if !candidate.considerError(err) {
			return err
		}
		state = schemaConditionalState{include: true}
	}
	if !state.include {
		document.root.children = nil
		document.root.attrs = conditionalRootFacts(document.root.attrs)
		document.conditionalUnsupported = nil
		return nil
	}
	if err := pruneSchemaConditionalChildren(document.root, policy, version, &candidate); err != nil {
		return err
	}
	document.conditionalUnsupported = nil
	if candidate.present {
		diagnosticErr := candidate.err()
		var diagnostic Diagnostic
		if !errors.As(diagnosticErr, &diagnostic) {
			return newSchemaBridgeInvariant(document.root.loc, "conditional unsupported candidate is not diagnostic")
		}
		document.conditionalUnsupported = &schemaUnsupportedCandidate{
			loc:     diagnostic.Loc(),
			message: diagnostic.Message(),
		}
	}
	return nil
}

func pruneSchemaConditionalChildren(parent *syntaxElement, policy LanguagePolicy, version XSDVersion, candidate *schemaChildUnsupportedCandidate) error {
	children := make([]syntaxNode, 0, len(parent.children))
	for _, node := range parent.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			children = append(children, node)
			continue
		}
		state, err := evaluateSchemaConditional(child, policy, version)
		if err != nil {
			if !candidate.considerError(err) {
				return err
			}
			state = schemaConditionalState{include: true}
		}
		if !state.include {
			continue
		}
		if err := pruneSchemaConditionalChildren(child, policy, version, candidate); err != nil {
			return err
		}
		children = append(children, child)
	}
	parent.children = children
	return nil
}

func evaluateSchemaConditional(element *syntaxElement, policy LanguagePolicy, version XSDVersion) (schemaConditionalState, error) {
	evaluation, err := collectSchemaConditionalAttributes(element, policy)
	if err != nil {
		return schemaConditionalState{}, err
	}
	if err := applySchemaConditionalVersion(element, &evaluation, version); err != nil {
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

func collectSchemaConditionalAttributes(element *syntaxElement, policy LanguagePolicy) (schemaConditionalEvaluation, error) {
	evaluation := schemaConditionalEvaluation{
		schemaConditionalState: schemaConditionalState{include: true},
	}
	for _, attribute := range element.attrs {
		if err := collectSchemaConditionalAttribute(element, attribute, &evaluation, policy); err != nil {
			return schemaConditionalEvaluation{}, err
		}
	}
	return evaluation, nil
}

func collectSchemaConditionalAttribute(element *syntaxElement, attribute syntaxAttribute, evaluation *schemaConditionalEvaluation, policy LanguagePolicy) error {
	if attribute.name.namespace != xsdVersioningNamespaceURI {
		return nil
	}
	switch attribute.name.local {
	case "minVersion", "maxVersion":
		return collectSchemaConditionalVersion(attribute, evaluation, policy)
	case "typeAvailable", "typeUnavailable", "facetAvailable", "facetUnavailable":
		return collectSchemaConditionalAvailability(element, attribute, evaluation)
	default:
		// The XSD versioning namespace is extensible. Unknown attributes
		// are permitted and intentionally have no effect here.
		return nil
	}
}

func collectSchemaConditionalVersion(attribute syntaxAttribute, evaluation *schemaConditionalEvaluation, policy LanguagePolicy) error {
	version, err := xsdVersionForLanguagePolicy(policy)
	if err != nil {
		return invalidLanguagePolicyDiagnostic(policy, err)
	}
	value, err := ParseStrictDecimalFor(version, attribute.value, attribute.loc)
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

func applySchemaConditionalVersion(element *syntaxElement, evaluation *schemaConditionalEvaluation, version XSDVersion) error {
	processorLexical := "1.1"
	if version == XSDVersion10 {
		processorLexical = "1.0"
	}
	processorVersion, err := ParseStrictDecimalFor(version, processorLexical, element.loc)
	if err != nil {
		return newSchemaBridgeInvariant(element.loc, "construct policy conditional version")
	}
	if evaluation.hasMin && processorVersion.Compare(evaluation.minVersion) < 0 {
		evaluation.include = false
	}
	if evaluation.hasMax && processorVersion.Compare(evaluation.maxVersion) >= 0 {
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

func validateSyntaxDocumentStructureWithPolicy(document *syntaxDocument, policy LanguagePolicy) error {
	version, err := xsdVersionForLanguagePolicy(policy)
	if err != nil {
		return invalidLanguagePolicyDiagnostic(policy, err)
	}
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
	return validateSchemaRootContents(document, version)
}

func validateSchemaRootContents(document *syntaxDocument, version XSDVersion) error {
	root := document.root
	var candidate schemaChildUnsupportedCandidate
	if document.conditionalUnsupported != nil {
		candidate.considerAt(document.conditionalUnsupported.loc, document.conditionalUnsupported.message)
	}
	attributeCandidate, err := validateSchemaRootAttributes(root, version)
	if err != nil {
		return err
	}
	candidate.merge(attributeCandidate)
	return validateSchemaRootChildren(root, version, &candidate)
}

func validateSchemaRootChildren(root *syntaxElement, version XSDVersion, candidate *schemaChildUnsupportedCandidate) error {
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
			if !candidate.considerError(err) {
				return err
			}
			phase = nextPhase
			continue
		}
		phase = nextPhase
	}
	return candidate.err()
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

//nolint:gocognit // Keep root grammar dispatch and phase transitions together.
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
	case "redefine":
		if phase == schemaGrammarDefaultOpenContent || phase == schemaGrammarDeclarations {
			return phase, newSchemaCompositionDiagnostic(child.loc, "schema child <redefine> follows global declarations")
		}
		nextPhase := schemaGrammarComposition
		if err := validateRedefineElement(child, version); err != nil {
			return nextPhase, err
		}
		return nextPhase, newSchemaSyntaxUnsupportedForVersion(child.loc, "XSD schema child <redefine> is not implemented", version)
	case "override":
		if phase == schemaGrammarDefaultOpenContent || phase == schemaGrammarDeclarations {
			return phase, newSchemaCompositionDiagnostic(child.loc, "schema child <override> follows global declarations")
		}
		nextPhase := schemaGrammarComposition
		if err := validateOverrideElement(child, version); err != nil {
			return nextPhase, err
		}
		return nextPhase, newSchemaSyntaxUnsupportedForVersion(child.loc, "XSD schema child <override> is not implemented", version)
	case "defaultOpenContent":
		if phase == schemaGrammarDefaultOpenContent || phase == schemaGrammarDeclarations {
			return phase, newSchemaCompositionDiagnostic(child.loc, "schema child <defaultOpenContent> is not permitted after global declarations")
		}
		nextPhase := schemaGrammarDefaultOpenContent
		if err := validateOpenContentLike(child, version, "defaultOpenContent", true); err != nil {
			return nextPhase, err
		}
		return nextPhase, newSchemaSyntaxUnsupportedForVersion(child.loc, "XSD schema child <defaultOpenContent> is not implemented", version)
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

//nolint:gocognit,funlen // Keep redefine attribute and child grammar in one ordered phase.
func validateRedefineElement(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateUniqueSchemaAttributes(element, "id", "schemaLocation"); err != nil {
		return err
	}
	schemaLocationAttributes := syntaxAttributesByLocal(element, "schemaLocation")
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := stageSchemaCandidateError(&candidate, validateSchemaQualifiedAttribute(attribute, "schema redefine")); err != nil {
				return err
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "schema redefine id must be a valid NCName")
			}
		case "schemaLocation":
			if err := validateSchemaAnyURI(attribute); err != nil {
				return err
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("schema redefine has forbidden attribute %q", attribute.name.local))
		}
	}
	if len(schemaLocationAttributes) == 0 {
		return newDiagnostic(
			FailureInvalid,
			MissingSchemaLocationCode,
			element.loc,
			"schema redefine has no schemaLocation attribute",
			nil,
		)
	}
	annotationSeen := false
	contentSeen := false
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if !xmlWhitespace([]byte(textNode.data)) {
				return newSchemaCompositionDiagnostic(textNode.loc, "schema redefine contains non-whitespace character data")
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(Loc{}, "schema redefine contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(child.loc, "schema redefine contains a forbidden non-XSD child")
		}
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, "schema redefine annotation must be first and unique")
			}
			annotationSeen = true
			if err := stageSchemaCandidateError(&candidate, validateSchemaAnnotationElement(child)); err != nil {
				return err
			}
			continue
		}
		contentSeen = true
		switch child.name.local {
		case "simpleType", "complexType", "group", "attributeGroup":
			if err := stageSchemaCandidateError(&candidate, validateGlobalSchemaDeclaration(child, version)); err != nil {
				return err
			}
		default:
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("schema redefine contains forbidden child <%s>", child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("schema redefine child <%s> is not implemented", child.name.local))
		}
	}
	return candidate.err()
}

func validateOverrideElement(element *syntaxElement, version XSDVersion) error {
	if err := validateUniqueSchemaAttributes(element, "id", "schemaLocation"); err != nil {
		return err
	}
	schemaLocationAttributes := syntaxAttributesByLocal(element, "schemaLocation")
	if len(schemaLocationAttributes) == 0 {
		return newDiagnostic(
			FailureInvalid,
			MissingSchemaLocationCode,
			element.loc,
			"schema override has no schemaLocation attribute",
			nil,
		)
	}
	var candidate schemaChildUnsupportedCandidate
	if version == XSDVersion10 {
		candidate.considerError(newXSD11FeatureMismatch(
			FeatureSchemaSyntax,
			UnsupportedSchemaSyntaxCode,
			element.loc,
			"schema child <override> is an XSD 1.1-only construct",
		))
	}
	if err := validateOverrideAttributes(element, &candidate); err != nil {
		return err
	}
	if err := validateOverrideChildren(element, version, &candidate); err != nil {
		return err
	}
	if candidate.present {
		return candidate.err()
	}
	return nil
}

func validateOverrideAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	for _, attribute := range element.attrs {
		if err := validateOverrideAttribute(attribute); err != nil {
			if !candidate.considerError(err) {
				return err
			}
		}
	}
	return nil
}

func validateOverrideAttribute(attribute syntaxAttribute) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		return validateSchemaQualifiedAttribute(attribute, "schema override")
	}
	switch attribute.name.local {
	case "id":
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newSchemaCompositionDiagnostic(attribute.loc, "schema override id must be a valid NCName")
		}
	case "schemaLocation":
		if err := validateSchemaAnyURI(attribute); err != nil {
			return err
		}
	default:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("schema override has forbidden attribute %q", attribute.name.local))
	}
	return nil
}

func validateOverrideChildren(element *syntaxElement, version XSDVersion, candidate *schemaChildUnsupportedCandidate) error {
	annotationSeen := false
	contentSeen := false
	for _, node := range element.children {
		if err := validateOverrideChild(node, version, &annotationSeen, &contentSeen); err != nil {
			if !candidate.considerError(err) {
				return err
			}
		}
	}
	return nil
}

func validateOverrideChild(node syntaxNode, version XSDVersion, annotationSeen, contentSeen *bool) error {
	textNode, ok := node.(syntaxText)
	if ok {
		if !xmlWhitespace([]byte(textNode.data)) {
			return newSchemaCompositionDiagnostic(textNode.loc, "schema override contains non-whitespace character data")
		}
		return nil
	}
	child, ok := node.(*syntaxElement)
	if !ok {
		return newSchemaBridgeInvariant(Loc{}, "schema override contains an unknown syntax node")
	}
	if child.name.namespace != xsdNamespaceURI {
		return newSchemaCompositionDiagnostic(child.loc, "schema override contains a forbidden non-XSD child")
	}
	if child.name.local == "annotation" {
		if *annotationSeen || *contentSeen {
			return newSchemaCompositionDiagnostic(child.loc, "schema override annotation must be first and unique")
		}
		*annotationSeen = true
		return validateSchemaAnnotationElement(child)
	}
	if !isGlobalSchemaDeclarationName(child.name.local) {
		return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("schema override contains forbidden child <%s>", child.name.local))
	}
	*contentSeen = true
	return validateGlobalSchemaDeclaration(child, version)
}

func isGlobalSchemaDeclarationName(local string) bool {
	switch local {
	case "element", "attribute", "simpleType", "complexType", "group", "attributeGroup", "notation":
		return true
	default:
		return false
	}
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

func validateSchemaRootAttributes(element *syntaxElement, version XSDVersion) (schemaChildUnsupportedCandidate, error) {
	var candidate schemaChildUnsupportedCandidate
	for _, attribute := range element.attrs {
		message, err := validateSchemaRootAttribute(element, attribute, version)
		if err != nil {
			if !candidate.considerError(err) {
				return candidate, err
			}
			continue
		}
		if message != "" {
			candidate.considerAt(attribute.loc, message)
		}
	}
	return candidate, nil
}

func validateSchemaRootAttribute(element *syntaxElement, attribute syntaxAttribute, version XSDVersion) (string, error) {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return "", nil
	}
	if attribute.name.namespace != "" {
		return "", validateSchemaQualifiedAttribute(attribute, "schema root")
	}
	return validateSchemaRootUnqualifiedAttribute(element, attribute, version)
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
		if err := validateSchemaAnyURI(attribute); err != nil {
			return err
		}
		feature, ok := LookupUnsupportedFeature(featureSchemaXMLBase)
		if !ok {
			return newDiagnostic(
				FailureInternal,
				diagnosticSyntaxFeatureCode,
				attribute.loc,
				"XML Base feature is not registered",
				nil,
			)
		}
		return newUnsupported(
			feature,
			UnsupportedSchemaSyntaxCode,
			attribute.loc,
			"XML Base schema resolution is not implemented",
		)
	default:
		return nil
	}
}

//nolint:gocognit // Root attributes have distinct lexical and support outcomes.
func validateSchemaRootUnqualifiedAttribute(element *syntaxElement, attribute syntaxAttribute, version XSDVersion) (string, error) {
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
	case "attributeFormDefault":
		if err := validateSchemaEnum(attribute, "qualified", "unqualified"); err != nil {
			return "", err
		}
		if collapseXMLWhitespace(attribute.value) == "unqualified" {
			return "", nil
		}
		return fmt.Sprintf("schema root attribute %q is not implemented", attribute.name.local), nil
	case "elementFormDefault":
		return "", validateSchemaEnum(attribute, "qualified", "unqualified")
	case "blockDefault":
		return "", validateSchemaBlockAttribute(attribute, schemaBlockAll, version, schemaBlockDocumentDefault)
	case "finalDefault":
		if err := validateSchemaRestrictionList(attribute, "extension", "restriction", "list", "union"); err != nil {
			return "", err
		}
		if collapseXMLWhitespace(attribute.value) == "" {
			return "", nil
		}
		return "schema root attribute \"finalDefault\" is not implemented", nil
	case "defaultAttributes":
		if err := validateConditionalQNameForSchema(element, attribute); err != nil {
			return "", err
		}
		if version == XSDVersion10 {
			return "", newXSD11FeatureMismatch(
				FeatureSchemaSyntax,
				UnsupportedSchemaSyntaxCode,
				attribute.loc,
				"schema root attribute \"defaultAttributes\" is an XSD 1.1-only construct",
			)
		}
		return "schema root attribute \"defaultAttributes\" is not implemented", nil
	case "xpathDefaultNamespace":
		if err := validateSchemaXPathDefaultNamespace(attribute); err != nil {
			return "", err
		}
		if version == XSDVersion10 {
			return "", newXSD11FeatureMismatch(
				FeatureSchemaSyntax,
				UnsupportedSchemaSyntaxCode,
				attribute.loc,
				"schema root attribute \"xpathDefaultNamespace\" is an XSD 1.1-only construct",
			)
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

func schemaBooleanValue(attribute syntaxAttribute) (bool, error) {
	if err := validateSchemaBoolean(attribute); err != nil {
		return false, err
	}
	value := collapseXMLWhitespace(attribute.value)
	return value == "true" || value == "1", nil
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

func validateSchemaBlockAttribute(attribute syntaxAttribute, allowed schemaBlockSet, version XSDVersion, scope schemaBlockPolicyScope) error {
	_, err := schemaBlockPolicyFromAttribute(attribute, allowed, version, scope)
	return err
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
	var candidate schemaChildUnsupportedCandidate
	for _, attribute := range element.attrs {
		message, err := validateGlobalSchemaAttribute(element, kind, attribute, version)
		if err != nil {
			if !candidate.considerError(err) {
				return err
			}
			continue
		}
		if message != "" {
			candidate.considerAt(attribute.loc, message)
		}
	}
	if err := validateGlobalSchemaDeclarationRequirements(element, kind, version); err != nil {
		return err
	}
	if err := validateGlobalSchemaAttributeCooccurrence(element); err != nil {
		return err
	}
	childrenErr := validateGlobalSchemaChildren(element, version)
	if childrenErr != nil {
		childrenErr = preferSchemaUnsupported(childrenErr, candidate.loc, candidate.message)
		if !candidate.considerError(childrenErr) {
			return childrenErr
		}
	}
	return candidate.err()
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

func validateGlobalSchemaDeclarationRequirements(element *syntaxElement, kind ComponentKind, version XSDVersion) error {
	if err := validateGlobalSchemaNamePresence(element); err != nil {
		return err
	}
	return validateSchemaNotationPublicPresence(element, kind, version)
}

func preferSchemaUnsupported(err error, loc Loc, message string) error {
	if message == "" {
		return err
	}
	if errors.Is(err, errLanguagePolicyMismatch) {
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

func validateGlobalSchemaAttribute(element *syntaxElement, kind ComponentKind, attribute syntaxAttribute, version XSDVersion) (string, error) {
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
	if kind == ComponentKindElementDeclaration && attribute.name.local == "substitutionGroup" {
		return "", validateSchemaElementSubstitutionGroupAttribute(element, attribute, version)
	}
	if implementedGlobalElementBooleanAttribute(element, kind, attribute.name.local) {
		return "", validateSchemaBoolean(attribute)
	}
	status := globalSchemaAttributeStatus(kind, attribute.name.local)
	switch status {
	case schemaAttributeAllowed:
		return "", validateAllowedGlobalSchemaAttribute(element, kind, attribute, version)
	case schemaAttributeUnsupported:
		if err := validateRecognizedUnsupportedAttribute(element, attribute, version); err != nil {
			return "", err
		}
		if version == XSDVersion10 && isXSD11GlobalSchemaAttribute(kind, attribute.name.local) {
			return "", newXSD11FeatureMismatch(
				FeatureSchemaSyntax,
				UnsupportedSchemaSyntaxCode,
				attribute.loc,
				fmt.Sprintf("global %s attribute %q is an XSD 1.1-only construct", element.name.local, attribute.name.local),
			)
		}
		return fmt.Sprintf("global %s attribute %q is not implemented", element.name.local, attribute.name.local), nil
	case schemaAttributeForbidden:
		return "", newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("global %s attribute %q is not permitted", element.name.local, attribute.name.local))
	default:
		return "", newSchemaBridgeInvariant(attribute.loc, "global declaration attribute has an unknown status")
	}
}

func implementedGlobalElementBooleanAttribute(element *syntaxElement, kind ComponentKind, local string) bool {
	if kind != ComponentKindElementDeclaration {
		return false
	}
	if local != "abstract" && local != "nillable" {
		return false
	}
	if len(syntaxAttributesByLocal(element, "type")) == 1 {
		return true
	}
	return inlineSimpleTypeChild(element) != nil
}

func isXSD11GlobalSchemaAttribute(kind ComponentKind, local string) bool {
	switch kind {
	case ComponentKindElementDeclaration:
		return local == "targetNamespace"
	case ComponentKindAttributeDeclaration:
		return local == "targetNamespace" || local == "inheritable"
	case ComponentKindComplexTypeDefinition:
		return local == "defaultAttributesApply"
	case ComponentKindSimpleTypeDefinition,
		ComponentKindModelGroupDefinition,
		ComponentKindAttributeGroupDefinition,
		ComponentKindNotationDeclaration:
		return false
	default:
		return false
	}
}

func validateAllowedGlobalSchemaAttribute(element *syntaxElement, kind ComponentKind, attribute syntaxAttribute, version XSDVersion) error {
	if attribute.name.local == "name" {
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, attribute.loc, "schema declaration name must be an unqualified valid NCName", nil)
		}
		return nil
	}
	if attribute.name.local == "id" && !validNCName(collapseXMLWhitespace(attribute.value)) {
		return newSchemaCompositionDiagnostic(attribute.loc, "schema declaration id must be a valid NCName")
	}
	if attribute.name.local == "block" {
		allowed := schemaBlockElementMask
		scope := schemaBlockElement
		if kind == ComponentKindComplexTypeDefinition {
			allowed = schemaBlockComplexMask
			scope = schemaBlockComplex
		}
		return validateSchemaBlockAttribute(attribute, allowed, version, scope)
	}
	if kind == ComponentKindNotationDeclaration {
		return validateSchemaNotationAttribute(attribute, version)
	}
	if (kind == ComponentKindElementDeclaration || kind == ComponentKindAttributeDeclaration) && attribute.name.local == "type" {
		return validateConditionalQNameForSchema(element, attribute)
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
			return schemaAttributeAllowed
		}
	}
	return schemaAttributeForbidden
}

func validateSchemaNotationPublicPresence(element *syntaxElement, kind ComponentKind, version XSDVersion) error {
	if kind != ComponentKindNotationDeclaration {
		return nil
	}
	attributes := syntaxAttributesByLocal(element, "public")
	if len(attributes) == 0 {
		return newSchemaNotationDiagnostic(
			element.loc,
			"notation declaration requires a public attribute",
			version,
			errSchemaNotationPublic,
		)
	}
	if len(attributes) != 1 {
		return newSchemaCompositionDiagnostic(attributes[1].loc, "notation public attribute must be unique")
	}
	return nil
}

func validateSchemaNotationAttribute(attribute syntaxAttribute, version XSDVersion) error {
	switch attribute.name.local {
	case "public":
		if collapseXMLWhitespace(attribute.value) == "" {
			return newSchemaNotationDiagnostic(
				attribute.loc,
				"notation public identifier must be non-empty",
				version,
				errSchemaNotationPublic,
			)
		}
	case "system":
		if err := validateSchemaAnyURI(attribute); err != nil {
			return newSchemaNotationDiagnostic(
				attribute.loc,
				fmt.Sprintf("attribute %q has an invalid anyURI value", attribute.name.local),
				version,
				errors.Join(errSchemaNotationSystem, err),
			)
		}
	}
	return nil
}

func elementSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "abstract", "default", "fixed", "nillable", "substitutionGroup", "targetNamespace", "final":
		return schemaAttributeUnsupported
	case "block":
		return schemaAttributeAllowed
	case "type":
		return schemaAttributeAllowed
	default:
		return schemaAttributeForbidden
	}
}

func attributeSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "default", "fixed", "targetNamespace", "inheritable":
		return schemaAttributeUnsupported
	case "type":
		return schemaAttributeAllowed
	default:
		return schemaAttributeForbidden
	}
}

func complexTypeSchemaAttributeStatus(local string) schemaAttributeStatus {
	switch local {
	case "abstract", "final", "mixed", "defaultAttributesApply":
		return schemaAttributeUnsupported
	case "block":
		return schemaAttributeAllowed
	default:
		return schemaAttributeForbidden
	}
}

func validateRecognizedUnsupportedAttribute(element *syntaxElement, attribute syntaxAttribute, version XSDVersion) error {
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
			return validateSchemaBlockAttribute(attribute, schemaBlockElementMask, version, schemaBlockElement)
		}
		return validateSchemaBlockAttribute(attribute, schemaBlockComplexMask, version, schemaBlockComplex)
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

func validateSchemaElementSubstitutionGroupAttribute(element *syntaxElement, attribute syntaxAttribute, version XSDVersion) error {
	items, err := schemaElementSubstitutionGroupTokens(attribute, version)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := validateSchemaElementSubstitutionGroupQName(element, attribute, item, version); err != nil {
			return err
		}
	}
	return nil
}

func schemaElementSubstitutionGroupTokens(attribute syntaxAttribute, version XSDVersion) ([]string, error) {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		if version == XSDVersion11 {
			return nil, nil
		}
		return nil, newSchemaSubstitutionGroupCardinalityDiagnostic(
			attribute.loc,
			"substitutionGroup must contain one non-empty QName in XSD 1.0",
			version,
			fmt.Errorf("%w: empty substitutionGroup", errSchemaSubstitutionCardinality),
		)
	}
	items := strings.Split(lexeme, " ")
	if version == XSDVersion10 && len(items) != 1 {
		return nil, newSchemaSubstitutionGroupCardinalityDiagnostic(
			attribute.loc,
			"substitutionGroup must contain exactly one QName in XSD 1.0",
			version,
			fmt.Errorf("%w: %d QNames", errSchemaSubstitutionCardinality, len(items)),
		)
	}
	return items, nil
}

func validateSchemaElementSubstitutionGroupQName(element *syntaxElement, attribute syntaxAttribute, item string, version XSDVersion) error {
	prefix, local, ok := splitConditionalQName(item)
	if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
		return newSchemaSubstitutionGroupQNameDiagnostic(
			attribute.loc,
			fmt.Sprintf("substitutionGroup QName %q is malformed", item),
			version,
			fmt.Errorf("%w: %q", errSchemaSubstitutionQName, item),
		)
	}
	if prefix == "" {
		return nil
	}
	if element == nil {
		return newSchemaBridgeInvariant(attribute.loc, "substitutionGroup QName validation has no element scope")
	}
	_, bound := element.scope.lookup(prefix)
	if bound {
		return nil
	}
	return newSchemaSubstitutionGroupQNameDiagnosticAtReference(
		attribute.loc,
		fmt.Sprintf("substitutionGroup QName prefix %q is unbound", prefix),
		fmt.Errorf("%w: prefix %q", errSchemaSubstitutionQName, prefix),
		schemaSubstitutionResolveSpecRef(version),
	)
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
	return validateGlobalSchemaChildrenWithFacetBridge(element, version, true, false)
}

func validateGlobalSchemaChildrenWithFacetBridge(element *syntaxElement, version XSDVersion, bridgeFacets, bridgeStringEnumeration bool) error {
	children, candidate, err := collectGlobalSchemaChildren(element)
	if err != nil {
		return err
	}
	var childErr error
	switch element.name.local {
	case "element":
		childErr = validateElementGlobalChildren(element, children, len(syntaxAttributesByLocal(element, "type")) > 0, version)
	case "attribute":
		childErr = validateAttributeGlobalChildren(element, children, len(syntaxAttributesByLocal(element, "type")) > 0, version)
	case "simpleType":
		childErr = validateSimpleTypeGlobalChildren(element, children, version, bridgeFacets, bridgeStringEnumeration)
	case "complexType":
		childErr = validateComplexTypeGlobalChildren(element, children, version)
	case "group":
		childErr = validateGroupGlobalChildren(element, children, version)
	case "attributeGroup":
		childErr = validateAttributeGroupGlobalChildren(element, children, version)
	case "notation":
		childErr = validateNotationGlobalChildren(element, children)
	default:
		return newSchemaBridgeInvariant(element.loc, "global declaration has an unknown child model")
	}
	if childErr != nil && !candidate.considerError(childErr) {
		return childErr
	}
	return candidate.err()
}

//nolint:gocognit // Keep global child collection and annotation candidate staging together.
func collectGlobalSchemaChildren(element *syntaxElement) ([]*syntaxElement, schemaChildUnsupportedCandidate, error) {
	children := make([]*syntaxElement, 0, len(element.children))
	var candidate schemaChildUnsupportedCandidate
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if xmlWhitespace([]byte(textNode.data)) {
				continue
			}
			return nil, candidate, newSchemaCompositionDiagnostic(textNode.loc, fmt.Sprintf("global %s contains non-whitespace character data", element.name.local))
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return nil, candidate, newSchemaBridgeInvariant(Loc{}, "global declaration contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			return nil, candidate, newSchemaCompositionDiagnostic(child.loc, "global declaration contains a forbidden non-XSD child")
		}
		if child.name.local == "annotation" {
			if err := validateSchemaAnnotationElement(child); err != nil {
				if !candidate.considerError(err) {
					return nil, candidate, err
				}
			}
		}
		children = append(children, child)
	}
	return children, candidate, nil
}

type schemaChildUnsupportedCandidate struct {
	loc                       Loc
	message                   string
	version                   XSDVersion
	captured                  error
	present                   bool
	mismatchOwner             *syntaxElement
	mismatchAttributeIndex    int
	hasMismatchAttributeIndex bool
}

func (candidate *schemaChildUnsupportedCandidate) consider(child *syntaxElement, parent string) {
	candidate.considerAt(child.loc, fmt.Sprintf("global %s child <%s> is not implemented", parent, child.name.local))
}

func (candidate *schemaChildUnsupportedCandidate) considerAt(loc Loc, message string) {
	candidate.considerAtVersion(loc, message, "")
}

func (candidate *schemaChildUnsupportedCandidate) considerAtVersion(loc Loc, message string, version XSDVersion) {
	if candidate.present {
		return
	}
	candidate.loc = loc
	candidate.message = message
	candidate.version = version
	candidate.present = true
}

func (candidate *schemaChildUnsupportedCandidate) considerError(err error) bool {
	return candidate.considerErrorAt(err, nil, 0)
}

func (candidate *schemaChildUnsupportedCandidate) considerErrorAt(err error, owner *syntaxElement, attributeIndex int) bool {
	if err == nil {
		return false
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
		return false
	}
	if candidate.present && !errors.Is(err, errLanguagePolicyMismatch) {
		return true
	}
	if candidate.present && errors.Is(candidate.captured, errLanguagePolicyMismatch) {
		return true
	}
	candidate.captured = err
	candidate.present = true
	candidate.mismatchOwner = nil
	candidate.mismatchAttributeIndex = 0
	candidate.hasMismatchAttributeIndex = false
	if errors.Is(err, errLanguagePolicyMismatch) && owner != nil {
		candidate.mismatchOwner = owner
		candidate.mismatchAttributeIndex = attributeIndex
		candidate.hasMismatchAttributeIndex = true
	}
	return true
}

func stageSchemaCandidateError(candidate *schemaChildUnsupportedCandidate, err error) error {
	if err == nil {
		return nil
	}
	if candidate.considerError(err) {
		return nil
	}
	return err
}

func (candidate *schemaChildUnsupportedCandidate) merge(other schemaChildUnsupportedCandidate) {
	if !other.present {
		return
	}
	if other.captured != nil {
		if other.hasMismatchAttributeIndex {
			candidate.considerErrorAt(other.captured, other.mismatchOwner, other.mismatchAttributeIndex)
			return
		}
		candidate.considerError(other.captured)
		return
	}
	candidate.considerAtVersion(other.loc, other.message, other.version)
}

func (candidate schemaChildUnsupportedCandidate) err() error {
	if !candidate.present {
		return nil
	}
	if candidate.captured != nil {
		return candidate.captured
	}
	if candidate.version != "" {
		return newSchemaSyntaxUnsupportedForVersion(candidate.loc, candidate.message, candidate.version)
	}
	return newSchemaSyntaxUnsupported(candidate.loc, candidate.message)
}

//nolint:gocognit // Keep all-child mismatch precedence in one local phase.
func stageAllChildOccurrenceCandidate(candidate *schemaChildUnsupportedCandidate, child *syntaxElement, occurrenceErr error) error {
	if occurrenceErr == nil {
		return nil
	}
	if !errors.Is(occurrenceErr, errLanguagePolicyMismatch) {
		if candidate.considerError(occurrenceErr) {
			return nil
		}
		return occurrenceErr
	}
	if candidate.present && errors.Is(candidate.captured, errLanguagePolicyMismatch) && candidate.mismatchOwner == child && candidate.hasMismatchAttributeIndex {
		if occurrenceIndex, ok := schemaAttributeIndex(child, "maxOccurs"); ok && occurrenceIndex < candidate.mismatchAttributeIndex {
			candidate.captured = occurrenceErr
			candidate.mismatchAttributeIndex = occurrenceIndex
		}
		return nil
	}
	if candidate.present && child.name.local == "any" && errors.Is(candidate.captured, errLanguagePolicyMismatch) && candidate.mismatchOwner == nil {
		candidate.captured = occurrenceErr
		return nil
	}
	if candidate.present && errors.Is(candidate.captured, errLanguagePolicyMismatch) {
		occurrenceLoc, occurrenceLocated := schemaCandidateDiagnosticLoc(occurrenceErr)
		candidateLoc, candidateLocated := schemaCandidateDiagnosticLoc(candidate.captured)
		if occurrenceLocated && candidateLocated && schemaLocPrecedes(occurrenceLoc, candidateLoc) {
			candidate.captured = occurrenceErr
			candidate.mismatchOwner = nil
			candidate.mismatchAttributeIndex = 0
			candidate.hasMismatchAttributeIndex = false
			return nil
		}
	}
	candidate.considerError(occurrenceErr)
	return nil
}

func schemaCandidateDiagnosticLoc(err error) (Loc, bool) {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return Loc{}, false
	}
	loc := diagnostic.Loc()
	if loc.IsZero() {
		return Loc{}, false
	}
	return loc, true
}

func schemaLocPrecedes(earlier, later Loc) bool {
	if earlier.Source() == "" || later.Source() == "" || earlier.Line() < 1 || later.Line() < 1 || earlier.Column() < 1 || later.Column() < 1 || earlier.Source() != later.Source() {
		return false
	}
	if earlier.Line() != later.Line() {
		return earlier.Line() < later.Line()
	}
	return earlier.Column() < later.Column()
}

func schemaAttributeIndex(element *syntaxElement, local string) (int, bool) {
	for index, attribute := range element.attrs {
		if attribute.name.namespace == "" && attribute.name.local == local {
			return index, true
		}
	}
	return 0, false
}

func schemaAnyMismatchAttributeIndex(element *syntaxElement) (int, bool) {
	for index, attribute := range element.attrs {
		if attribute.name.namespace != "" {
			continue
		}
		switch attribute.name.local {
		case "notNamespace", "notQName":
			return index, true
		}
	}
	return 0, false
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
func validateElementGlobalChildren(parent *syntaxElement, children []*syntaxElement, typeAttributeSeen bool, version XSDVersion) error {
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
			if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
			if child.name.local == "complexType" && !candidate.present {
				candidate.consider(child, parent.name.local)
			}
		case "alternative":
			if phase == elementGlobalConstraintPhase {
				return newSchemaCompositionDiagnostic(child.loc, "element alternative must precede identity constraints")
			}
			phase = elementGlobalAlternativePhase
			if err := validateChoiceElementAlternative(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
			if !candidate.present {
				candidate.considerAtVersion(child.loc, "global element alternatives are not implemented", version)
			}
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

//nolint:gocognit // Keep inline simpleType grammar and candidate staging together.
func validateAttributeGlobalChildren(parent *syntaxElement, children []*syntaxElement, typeAttributeSeen bool, version XSDVersion) error {
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
			if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
			if !candidate.present {
				candidate.consider(child, parent.name.local)
			}
			continue
		}
		if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
			return err
		}
		candidate.consider(child, parent.name.local)
	}
	return candidate.err()
}

func validateSimpleTypeGlobalChildren(parent *syntaxElement, children []*syntaxElement, version XSDVersion, bridgeFacets, bridgeStringEnumeration bool) error {
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
		if err := validateSimpleTypeGlobalModelChild(parent, child, &modelSeen, &candidate, version, bridgeFacets, bridgeStringEnumeration); err != nil {
			if candidate.considerError(err) {
				continue
			}
			return err
		}
	}
	if !modelSeen {
		return newSchemaCompositionDiagnostic(parent.loc, "simpleType requires one restriction, list, or union child")
	}
	return candidate.err()
}

func validateSimpleTypeGlobalModelChild(parent, child *syntaxElement, modelSeen *bool, candidate *schemaChildUnsupportedCandidate, version XSDVersion, bridgeFacets, bridgeStringEnumeration bool) error {
	switch child.name.local {
	case "restriction":
		return validateSimpleTypeGlobalRestrictionChild(child, modelSeen, version, bridgeFacets, bridgeStringEnumeration)
	case "list", "union":
		if *modelSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simpleType requires exactly one model child")
		}
		*modelSeen = true
		if child.name.local == "list" {
			return validateSimpleTypeList(child, version)
		}
		return validateSimpleTypeUnion(child, version)
	default:
		if child.name.local == "enumeration" {
			return invalidEnumerationDeclarationDiagnostic(child.loc, version, "enumeration facet is only permitted as a restriction child")
		}
		if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
			return err
		}
		candidate.consider(child, parent.name.local)
		return nil
	}
}

func validateSimpleTypeGlobalRestrictionChild(child *syntaxElement, modelSeen *bool, version XSDVersion, bridgeFacets, bridgeStringEnumeration bool) error {
	if *modelSeen {
		return newSchemaCompositionDiagnostic(child.loc, "simpleType requires exactly one model child")
	}
	*modelSeen = true
	restrictionErr := validateSimpleTypeRestrictionWithFacetBridge(child, version, bridgeFacets, bridgeStringEnumeration)
	return preferPrecisionDecimalRestrictionMismatch(child, version, restrictionErr)
}

func preferPrecisionDecimalRestrictionMismatch(element *syntaxElement, version XSDVersion, err error) error {
	if version != XSDVersion10 || !precisionDecimalRestrictionBase(element) {
		return err
	}
	if err == nil {
		return precisionDecimalSchemaVersionDiagnostic(element.loc, schemaRestrictionBaseName(element))
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
		return err
	}
	return precisionDecimalSchemaVersionDiagnostic(element.loc, schemaRestrictionBaseName(element))
}

func precisionDecimalRestrictionBase(element *syntaxElement) bool {
	attributes := syntaxAttributesByLocal(element, "base")
	if len(attributes) != 1 {
		return false
	}
	base, err := expandSchemaQName(element, attributes[0])
	if err != nil {
		return false
	}
	return base.Namespace() == xsdNamespaceURI && base.Local() == "precisionDecimal"
}

func directOrdinaryBuiltinScaleBase(element *syntaxElement) bool {
	base := schemaRestrictionBaseName(element)
	if base.Namespace() != xsdNamespaceURI {
		return false
	}
	return base.Local() == "decimal" || base.Local() == "integer"
}

func schemaRestrictionBaseName(element *syntaxElement) QName {
	attributes := syntaxAttributesByLocal(element, "base")
	if len(attributes) != 1 {
		return QName{}
	}
	base, err := expandSchemaQName(element, attributes[0])
	if err != nil {
		return QName{}
	}
	return base
}

//nolint:gocognit // Keep list source cardinality and recursive preflight together.
func validateSimpleTypeList(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSimpleTypeListAttributes(element, &candidate); err != nil {
		return err
	}
	children, err := collectSimpleTypeChildren(element, "simple type list", &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	inlineSeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, "simple type list annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		if child.name.local != "simpleType" {
			if child.name.local == "enumeration" {
				return invalidEnumerationDeclarationDiagnostic(child.loc, version, "enumeration facet is only permitted as a restriction child")
			}
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("simple type list contains forbidden child <%s>", child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("simple type list child <%s> is not implemented", child.name.local))
			continue
		}
		if inlineSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type list permits at most one inline simpleType")
		}
		inlineSeen = true
		if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
			return err
		}
	}
	itemTypes := syntaxAttributesByLocal(element, "itemType")
	if len(itemTypes) == 0 && !inlineSeen {
		return newSchemaCompositionDiagnostic(element.loc, "simple type list requires an itemType or inline simpleType")
	}
	if len(itemTypes) > 0 && inlineSeen {
		return newSchemaCompositionDiagnostic(itemTypes[0].loc, "simple type list cannot combine itemType with an inline simpleType")
	}
	if candidate.present {
		return candidate.err()
	}
	return nil
}

func validateSimpleTypeListAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	return validateSimpleTypeSourceAttributes(element, "list", "itemType", false, candidate)
}

//nolint:gocognit // Keep shared list/union attribute lexical checks together.
func validateSimpleTypeSourceAttributes(element *syntaxElement, kind, source string, sourceIsQNameList bool, candidate *schemaChildUnsupportedCandidate) error {
	if err := validateUniqueSchemaAttributes(element, "id", source); err != nil {
		return err
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("simple type %s has forbidden attribute %q", kind, attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := stageSchemaCandidateError(candidate, validateSchemaXMLAttribute(attribute)); err != nil {
					return err
				}
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "simple type "+kind+" id must be a valid NCName")
			}
		case "itemType", "memberTypes":
			if attribute.name.local != source {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("simple type %s has forbidden attribute %q", kind, attribute.name.local))
			}
			if sourceIsQNameList {
				if collapseXMLWhitespace(attribute.value) == "" {
					return newSchemaCompositionDiagnostic(attribute.loc, "simple type union memberTypes must not be empty")
				}
				if err := parseConditionalQNameList(element, attribute); err != nil {
					return err
				}
				continue
			}
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return err
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("simple type %s has forbidden attribute %q", kind, attribute.name.local))
		}
	}
	return nil
}

//nolint:gocognit // Keep union source cardinality and recursive preflight together.
func validateSimpleTypeUnion(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSimpleTypeUnionAttributes(element, &candidate); err != nil {
		return err
	}
	children, err := collectSimpleTypeChildren(element, "simple type union", &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	inlineCount := 0
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, "simple type union annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		if child.name.local != "simpleType" {
			if child.name.local == "enumeration" {
				return invalidEnumerationDeclarationDiagnostic(child.loc, version, "enumeration facet is only permitted as a restriction child")
			}
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("simple type union contains forbidden child <%s>", child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("simple type union child <%s> is not implemented", child.name.local))
			continue
		}
		inlineCount++
		if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
			return err
		}
	}
	memberTypes := syntaxAttributesByLocal(element, "memberTypes")
	memberTypesPresent := len(memberTypes) == 1 && collapseXMLWhitespace(memberTypes[0].value) != ""
	if !memberTypesPresent && inlineCount == 0 {
		return newSchemaCompositionDiagnostic(element.loc, "simple type union requires memberTypes or an inline simpleType")
	}
	if candidate.present {
		return candidate.err()
	}
	return nil
}

func validateSimpleTypeUnionAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	return validateSimpleTypeSourceAttributes(element, "union", "memberTypes", true, candidate)
}

func validateUniqueSchemaAttributes(element *syntaxElement, locals ...string) error {
	for _, local := range locals {
		attributes := syntaxAttributesByLocal(element, local)
		if len(attributes) > 1 {
			return newSchemaCompositionDiagnostic(attributes[1].loc, fmt.Sprintf("attribute %q must be unique", local))
		}
	}
	return nil
}

func validateSimpleTypeRestrictionWithFacetBridge(element *syntaxElement, version XSDVersion, bridgeFacets, bridgeStringEnumeration bool) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSimpleTypeRestrictionAttributes(element, &candidate); err != nil {
		return err
	}
	enforceNonNegativeScale := directOrdinaryBuiltinScaleBase(element) &&
		(!bridgeFacets || version == XSDVersion10)
	if err := validateSimpleTypeRestrictionChildren(element, version, bridgeFacets, bridgeStringEnumeration, enforceNonNegativeScale, &candidate); err != nil {
		return err
	}
	return candidate.err()
}

func validateSimpleTypeRestrictionAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	if err := validateUniqueSchemaAttributes(element, "base", "id"); err != nil {
		return err
	}
	baseAttributes := syntaxAttributesByLocal(element, "base")
	inline := inlineSimpleTypeChild(element)
	if len(baseAttributes) == 0 {
		if inline == nil {
			return newSchemaCompositionDiagnostic(element.loc, "simple type restriction requires a base attribute or inline simpleType")
		}
	}
	if len(baseAttributes) == 1 && inline != nil {
		return newSchemaCompositionDiagnostic(inline.loc, "simple type restriction cannot combine base with an inline simpleType")
	}
	for _, attribute := range element.attrs {
		if err := validateSimpleTypeRestrictionAttribute(element, attribute, candidate); err != nil {
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

func validateSimpleTypeRestrictionAttribute(element *syntaxElement, attribute syntaxAttribute, candidate *schemaChildUnsupportedCandidate) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("simple type restriction has forbidden attribute %q", attribute.name.local))
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return stageSchemaCandidateError(candidate, validateSchemaXMLAttribute(attribute))
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

func validateSimpleTypeRestrictionChildren(element *syntaxElement, version XSDVersion, bridgeFacets, bridgeStringEnumeration, enforceNonNegativeScale bool, candidate *schemaChildUnsupportedCandidate) error {
	children, err := collectSimpleTypeRestrictionChildren(element, version, candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	totalSeen := false
	fractionSeen := false
	inlineSeen := false
	baseSeen := len(syntaxAttributesByLocal(element, "base")) > 0
	facetSeen := make(map[string]bool)
	for _, child := range children {
		if err := validateSimpleTypeRestrictionChild(child, &annotationSeen, &contentSeen, &totalSeen, &fractionSeen, &inlineSeen, baseSeen, facetSeen, version, bridgeFacets, bridgeStringEnumeration, enforceNonNegativeScale); err != nil {
			if candidate.considerError(err) {
				continue
			}
			return err
		}
	}
	return candidate.err()
}

//nolint:gocognit // Keep the narrow version-aware restriction child collection together.
func collectSimpleTypeRestrictionChildren(element *syntaxElement, version XSDVersion, candidate *schemaChildUnsupportedCandidate) ([]*syntaxElement, error) {
	children := make([]*syntaxElement, 0, len(element.children))
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if xmlWhitespace([]byte(textNode.data)) {
				continue
			}
			return nil, newSchemaCompositionDiagnostic(textNode.loc, element.name.local+" facet contains non-whitespace character data")
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return nil, newSchemaBridgeInvariant(Loc{}, element.name.local+" facet contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			if version == XSDVersion10 {
				return nil, newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet contains a forbidden foreign child")
			}
			children = append(children, child)
			continue
		}
		if child.name.local == "annotation" {
			if err := stageSchemaCandidateError(candidate, validateSchemaAnnotationElement(child)); err != nil {
				return nil, err
			}
		}
		children = append(children, child)
	}
	return children, nil
}

//nolint:gocognit // Keep restriction ordering and recursive preflight together.
func validateSimpleTypeRestrictionChild(child *syntaxElement, annotationSeen, contentSeen, totalSeen, fractionSeen, inlineSeen *bool, baseSeen bool, facetSeen map[string]bool, version XSDVersion, bridgeFacets, bridgeStringEnumeration, enforceNonNegativeScale bool) error {
	if child.name.namespace != xsdNamespaceURI {
		if version == XSDVersion10 {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction contains a forbidden foreign child")
		}
		return newSchemaSyntaxUnsupported(child.loc, fmt.Sprintf("simple type restriction foreign child <%s> is not implemented", child.name.local))
	}
	if child.name.local == "annotation" {
		if *annotationSeen || *contentSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction annotation must be first and unique")
		}
		*annotationSeen = true
		return nil
	}
	*contentSeen = true
	if child.name.local == "simpleType" {
		if len(facetSeen) > 0 || *totalSeen || *fractionSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction inline simpleType must precede facets")
		}
		if *inlineSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction inline simpleType must be unique")
		}
		if baseSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction cannot combine base with an inline simpleType")
		}
		*inlineSeen = true
		if err := validateInlineSchemaType(child, version); err != nil {
			return err
		}
		return nil
	}
	return validateSimpleTypeRestrictionFacet(child, totalSeen, fractionSeen, facetSeen, version, bridgeFacets, bridgeStringEnumeration, enforceNonNegativeScale)
}

//nolint:gocognit // Keep facet classification and lexical preflight together.
func validateSimpleTypeRestrictionFacet(child *syntaxElement, totalSeen, fractionSeen *bool, facetSeen map[string]bool, version XSDVersion, bridgeFacets, bridgeStringEnumeration, enforceNonNegativeScale bool) error {
	switch child.name.local {
	case "totalDigits":
		if *totalSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction totalDigits facet must be unique")
		}
		*totalSeen = true
		return validateSimpleTypeDigitFacet(child, version)
	case "fractionDigits":
		if *fractionSeen {
			return newSchemaCompositionDiagnostic(child.loc, "simple type restriction fractionDigits facet must be unique")
		}
		*fractionSeen = true
		return validateSimpleTypeDigitFacet(child, version)
	default:
		if isUnsupportedSimpleTypeFacet(child.name.local) {
			if child.name.local == "assertion" {
				if facetSeen[child.name.local] && !repeatedSimpleTypeFacetAllowed(child.name.local, version) {
					return newSchemaCompositionDiagnostic(child.loc, "simple type restriction facet <assertion> must be unique")
				}
				facetSeen[child.name.local] = true
				var candidate schemaChildUnsupportedCandidate
				if err := validateSimpleTypeAssertionFacet(child, &candidate); err != nil {
					if !candidate.considerError(err) {
						return err
					}
				}
				if err := unsupportedSimpleTypeRestrictionChild(child, version); err != nil {
					if !candidate.considerError(err) {
						return err
					}
				}
				return candidate.err()
			}
			if facetSeen[child.name.local] && !repeatedSimpleTypeFacetAllowed(child.name.local, version) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("simple type restriction facet <%s> must be unique", child.name.local))
			}
			facetSeen[child.name.local] = true
			var candidate schemaChildUnsupportedCandidate
			if err := validateSimpleTypeFacetAttributes(child, &candidate, enforceNonNegativeScale, version); err != nil {
				return err
			}
			if err := validateSimpleTypeFacetChildren(child, &candidate); err != nil {
				return err
			}
			if version == XSDVersion10 && isXSD11SimpleTypeFacet(child.name.local) {
				candidate.considerError(unsupportedSimpleTypeRestrictionChild(child, version))
				return candidate.err()
			}
			if candidate.present {
				return candidate.err()
			}
			if bridgeFacets || bridgeStringEnumeration && child.name.local == "enumeration" {
				return nil
			}
		}
		return unsupportedSimpleTypeRestrictionChild(child, version)
	}
}

func unsupportedSimpleTypeRestrictionChild(child *syntaxElement, version XSDVersion) error {
	if child.name.local == "assertion" {
		if version == XSDVersion10 {
			return newXSD11FeatureMismatch(
				FeatureID("xsd.assertion"),
				UnsupportedDatatypeFacetCode,
				child.loc,
				"XSD assertion facets are not permitted by the selected XSD 1.0 policy",
			)
		}
		feature, ok := LookupUnsupportedFeature("xsd.assertion")
		if !ok {
			return newDiagnostic(
				FailureInternal,
				diagnosticSyntaxAssertionFeatureCode,
				child.loc,
				"XSD assertion feature is not registered",
				nil,
			)
		}
		return newUnsupportedForVersion(
			feature,
			UnsupportedDatatypeFacetCode,
			child.loc,
			"XSD assertion facets are not implemented",
			version,
		)
	}
	if isUnsupportedSimpleTypeFacet(child.name.local) {
		if version == XSDVersion10 && isXSD11SimpleTypeFacet(child.name.local) {
			return newXSD11FeatureMismatch(
				FeatureDatatypeFacets,
				UnsupportedDatatypeFacetCode,
				child.loc,
				fmt.Sprintf("simple type restriction facet <%s> is an XSD 1.1-only construct", child.name.local),
			)
		}
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

func validateSimpleTypeDigitFacet(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSimpleTypeDigitFacetAttributes(element, &candidate); err != nil {
		return err
	}
	valueAttributes := syntaxAttributesByLocal(element, "value")
	if element.name.local == "totalDigits" {
		_, err := ParseTotalDigitsFor(version, valueAttributes[0].value, valueAttributes[0].loc)
		if err != nil {
			return err
		}
		if err := validateSimpleTypeDigitFacetChildren(element, &candidate); err != nil {
			return err
		}
		return candidate.err()
	}
	_, err := ParseFractionDigitsFor(version, valueAttributes[0].value, valueAttributes[0].loc)
	if err != nil {
		return err
	}
	if err := validateSimpleTypeDigitFacetChildren(element, &candidate); err != nil {
		return err
	}
	return candidate.err()
}

func validateSimpleTypeDigitFacetAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	if err := validateUniqueSchemaAttributes(element, "value", "fixed", "id"); err != nil {
		return err
	}
	valueAttributes := syntaxAttributesByLocal(element, "value")
	if len(valueAttributes) == 0 {
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" facet requires a value attribute")
	}
	for _, attribute := range element.attrs {
		if err := validateSimpleTypeDigitFacetAttribute(element, attribute, candidate); err != nil {
			return err
		}
	}
	return nil
}

func validateSimpleTypeDigitFacetAttribute(element *syntaxElement, attribute syntaxAttribute, candidate *schemaChildUnsupportedCandidate) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s facet has forbidden attribute %q", element.name.local, attribute.name.local))
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return stageSchemaCandidateError(candidate, validateSchemaXMLAttribute(attribute))
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

func validateSimpleTypeDigitFacetChildren(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	children, err := collectSimpleTypeChildren(element, element.name.local+" facet", candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		if isKnownSchemaElement(child.name.local) {
			return newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet permits only an annotation child")
		}
		candidate.considerAt(child.loc, element.name.local+" facet child <"+child.name.local+"> is not implemented")
	}
	return nil
}

func repeatedSimpleTypeFacetAllowed(local string, version XSDVersion) bool {
	// Pattern, enumeration, and assertions are declarations with repeatable
	// value spaces. XSD 1.1 additionally permits repeated assertions.
	if local == "pattern" || local == "enumeration" {
		return true
	}
	return local == "assertion" && version == XSDVersion11
}

//nolint:gocognit,funlen // Keep recognized facet attribute and value checks together.
func validateSimpleTypeFacetAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate, enforceNonNegativeScale bool, version XSDVersion) error {
	if err := validateUniqueSchemaAttributes(element, "value", "fixed", "id"); err != nil {
		return err
	}
	valueAttributes := syntaxAttributesByLocal(element, "value")
	if len(valueAttributes) == 0 {
		if element.name.local == "enumeration" && enumerationValueOnlyMissing(element) {
			return invalidEnumerationDeclarationDiagnostic(element.loc, version, "enumeration facet requires a value attribute")
		}
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" facet requires a value attribute")
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s facet has forbidden attribute %q", element.name.local, attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := stageSchemaCandidateError(candidate, validateSchemaXMLAttribute(attribute)); err != nil {
					return err
				}
			}
			continue
		}
		switch attribute.name.local {
		case "value":
			// Empty lexical values are valid for some datatypes (including
			// enumeration), so presence is the grammar check here.
		case "fixed":
			if element.name.local == "pattern" || element.name.local == "enumeration" || element.name.local == "assertion" {
				return newSchemaCompositionDiagnostic(attribute.loc, element.name.local+" facet does not permit fixed")
			}
			if err := validateSchemaBoolean(attribute); err != nil {
				return err
			}
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, element.name.local+" facet id must be a valid NCName")
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s facet has forbidden attribute %q", element.name.local, attribute.name.local))
		}
	}
	value := syntaxAttributesByLocal(element, "value")[0]
	switch element.name.local {
	case "length", "minLength", "maxLength":
		parsed, parseErr := ParseStrictInteger(collapseXMLWhitespace(value.value), value.loc)
		if parseErr != nil || parsed.Sign() < 0 {
			if parseErr != nil {
				return parseErr
			}
			return newSchemaCompositionDiagnostic(value.loc, element.name.local+" facet value must be non-negative")
		}
	case "minScale", "maxScale":
		parsed, parseErr := ParseStrictInteger(collapseXMLWhitespace(value.value), value.loc)
		if parseErr != nil {
			return parseErr
		}
		if enforceNonNegativeScale && parsed.Sign() < 0 {
			return newSchemaCompositionDiagnostic(value.loc, element.name.local+" facet value must be non-negative")
		}
	case "precision":
		parsed, parseErr := ParseStrictInteger(collapseXMLWhitespace(value.value), value.loc)
		if parseErr != nil {
			return parseErr
		}
		if parsed.Sign() <= 0 {
			return newSchemaCompositionDiagnostic(value.loc, "precision facet value must be positive")
		}
	case "whiteSpace":
		if err := validateSchemaEnum(value, "preserve", "replace", "collapse"); err != nil {
			return err
		}
	case "explicitTimezone":
		if err := validateSchemaEnum(value, "prohibited", "optional", "required"); err != nil {
			return err
		}
	case "assertion":
		if collapseXMLWhitespace(value.value) == "" {
			return newSchemaCompositionDiagnostic(value.loc, "assertion facet value cannot be empty")
		}
	}
	return nil
}

func enumerationValueOnlyMissing(element *syntaxElement) bool {
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace == "" && attribute.name.local == "id" {
			continue
		}
		return false
	}
	return true
}

func validateSimpleTypeFacetChildren(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	children, err := collectSimpleTypeChildren(element, element.name.local+" facet", candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	for _, child := range children {
		if child.name.local != "annotation" {
			contentSeen = true
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet permits only an annotation child")
			}
			candidate.considerAt(child.loc, element.name.local+" facet child <"+child.name.local+"> is not implemented")
			continue
		}
		if annotationSeen || contentSeen {
			return newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet annotation must be unique")
		}
		annotationSeen = true
	}
	return nil
}

//nolint:gocognit // Keep XSD 1.1 assertion facet lexical checks together.
func validateSimpleTypeAssertionFacet(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	if err := validateUniqueSchemaAttributes(element, "test", "xpathDefaultNamespace", "id"); err != nil {
		return err
	}
	testAttributes := syntaxAttributesByLocal(element, "test")
	if len(testAttributes) == 0 {
		return newSchemaCompositionDiagnostic(element.loc, "assertion facet requires a test attribute")
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := stageSchemaCandidateError(candidate, validateSchemaQualifiedAttribute(attribute, "assertion facet")); err != nil {
				return err
			}
			continue
		}
		switch attribute.name.local {
		case "test":
		case "xpathDefaultNamespace":
			if err := validateSchemaXPathDefaultNamespace(attribute); err != nil {
				return err
			}
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "assertion facet id must be a valid NCName")
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("assertion facet has forbidden attribute %q", attribute.name.local))
		}
	}
	if err := validateSimpleTypeFacetChildren(element, candidate); err != nil {
		return err
	}
	return nil
}

func collectSimpleTypeChildren(element *syntaxElement, owner string, candidate *schemaChildUnsupportedCandidate) ([]*syntaxElement, error) {
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
			return nil, newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("%s contains a forbidden foreign child <%s>", owner, child.name.local))
		}
		if child.name.local == "annotation" {
			if err := stageSchemaCandidateError(candidate, validateSchemaAnnotationElement(child)); err != nil {
				return nil, err
			}
		}
		children = append(children, child)
	}
	return children, nil
}

func isUnsupportedSimpleTypeFacet(local string) bool {
	switch local {
	case "assertion", "enumeration", "explicitTimezone", "length", "maxExclusive", "maxInclusive", "maxLength", "maxScale", "minExclusive", "minInclusive", "minLength", "minScale", "pattern", "precision", "whiteSpace":
		return true
	default:
		return false
	}
}

func isXSD11SimpleTypeFacet(local string) bool {
	switch local {
	case "assertion", "explicitTimezone", "maxScale", "minScale", "precision":
		return true
	default:
		return false
	}
}

//nolint:gocognit,funlen // Keep mutually-exclusive complexType grammar branches explicit.
func validateComplexTypeGlobalChildren(parent *syntaxElement, children []*syntaxElement, version XSDVersion) error {
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
			if version == XSDVersion11 && child.name.local == "simpleContent" {
				if mixedAttributes := syntaxAttributesByLocal(parent, "mixed"); len(mixedAttributes) > 0 {
					mixed, mixedErr := schemaBooleanValue(mixedAttributes[0])
					if mixedErr != nil {
						return mixedErr
					}
					if mixed {
						return newSchemaCompositionDiagnostic(mixedAttributes[0].loc, "complexType mixed cannot combine with simpleContent")
					}
				}
			}
			if version == XSDVersion11 && child.name.local == "complexContent" {
				if err := validateComplexTypeMixedAgreement(parent, child); err != nil {
					return err
				}
			}
			if specialSeen || openContentSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType content model is mutually exclusive")
			}
			specialSeen = true
			if err := validateComplexTypeContentChild(parent, child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "openContent":
			if specialSeen || openContentSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType openContent must precede the model and attributes")
			}
			openContentSeen = true
			if err := validateOpenContent(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "group", "all":
			if specialSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType model child must be unique and precede attributes")
			}
			modelSeen = true
			if err := validateUnsupportedModelParticle(child, version); err != nil {
				if !candidate.considerError(err) {
					return err
				}
			}
			if !candidate.present {
				candidate.consider(child, parent.name.local)
			}
		case "sequence":
			if specialSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType model child must be unique and precede attributes")
			}
			modelSeen = true
			sequenceErr := validateComplexTypeSequenceParticle(parent, child, version)
			if sequenceErr != nil && !candidate.considerError(sequenceErr) {
				return sequenceErr
			}
			if len(syntaxAttributesByLocal(parent, "name")) != 1 && !candidate.present {
				candidate.consider(child, parent.name.local)
			}
		case "choice":
			if specialSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType model child must be unique and precede attributes")
			}
			modelSeen = true
			allowDirectChoicePolicies := len(syntaxAttributesByLocal(parent, "name")) == 1
			if err := validateChoiceParticleWithOptions(child, version, allowDirectChoicePolicies, allowDirectChoicePolicies); err != nil {
				if !candidate.considerError(err) {
					return err
				}
			}
		case "attribute", "attributeGroup":
			if specialSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType attributes must follow the model and precede anyAttribute/assert")
			}
			attributesSeen = true
			childErr := validateAttributeGroupReference(child)
			if child.name.local == "attribute" {
				childErr = validateLocalAttribute(child, version)
			}
			if childErr != nil && !candidate.considerError(childErr) {
				return childErr
			}
		case "anyAttribute":
			if specialSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType anyAttribute must be unique and last among attributes")
			}
			anyAttributeSeen = true
			if err := validateDirectNamedComplexTypeAnyAttribute(parent, child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "assert":
			if specialSeen {
				return newSchemaCompositionDiagnostic(child.loc, "complexType assert cannot follow simpleContent or complexContent")
			}
			assertSeen = true
			if err := validateComplexTypeAssert(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		default:
			if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
				return err
			}
			candidate.consider(child, parent.name.local)
		}
	}
	return candidate.err()
}

func validateComplexTypeMixedAgreement(parent, content *syntaxElement) error {
	outerMixed := syntaxAttributesByLocal(parent, "mixed")
	innerMixed := syntaxAttributesByLocal(content, "mixed")
	if len(outerMixed) == 0 || len(innerMixed) == 0 {
		return nil
	}
	outer, err := schemaBooleanValue(outerMixed[0])
	if err != nil {
		return err
	}
	inner, err := schemaBooleanValue(innerMixed[0])
	if err != nil {
		return err
	}
	if outer != inner {
		return newSchemaCompositionDiagnostic(innerMixed[0].loc, "complexType and complexContent mixed values must agree")
	}
	return nil
}

//nolint:gocognit // Keep the ordered simple/complex content grammar explicit.
func validateComplexTypeContentChild(parent, element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateComplexTypeContentAttributes(element, &candidate); err != nil {
		return err
	}
	children, err := collectSimpleTypeChildren(element, element.name.local, &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	derivationSeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || derivationSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		if child.name.local != "restriction" && child.name.local != "extension" {
			return newSchemaCompositionDiagnostic(child.loc, element.name.local+" requires one restriction or extension child")
		}
		if derivationSeen {
			return newSchemaCompositionDiagnostic(child.loc, element.name.local+" permits exactly one derivation child")
		}
		derivationSeen = true
		if err := validateComplexDerivation(child, version, element.name.local == "complexContent", element.name.local == "simpleContent" && child.name.local == "restriction"); err != nil && !candidate.considerError(err) {
			return err
		}
	}
	if !derivationSeen {
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" requires one restriction or extension child")
	}
	if candidate.present {
		return candidate.err()
	}
	if element.name.local == "complexContent" {
		restriction := schemaComplexContentRestrictionChild(element)
		if boundedComplexContentRestrictionCandidate(restriction, true) {
			if schemaBooleanAttributeTrue(parent, "mixed") || schemaBooleanAttributeTrue(element, "mixed") {
				return newSchemaSyntaxUnsupportedForVersion(
					element.loc,
					"complexContent restriction with mixed content is not implemented",
					version,
				)
			}
			return nil
		}
	}
	return newSchemaSyntaxUnsupported(element.loc, element.name.local+" is not implemented")
}

func boundedComplexContentRestrictionCandidate(element *syntaxElement, complexContent bool) bool {
	if !complexContent || element == nil || element.name.namespace != xsdNamespaceURI || element.name.local != "restriction" {
		return false
	}
	if len(syntaxAttributesByLocal(element, "base")) != 1 {
		return false
	}
	anyAttributeSeen := false
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if child.name.namespace != xsdNamespaceURI {
			return false
		}
		switch child.name.local {
		case "annotation":
			continue
		case "anyAttribute":
			if anyAttributeSeen || !isSupportedAnyAttribute(child) {
				return false
			}
			anyAttributeSeen = true
		default:
			return false
		}
	}
	return anyAttributeSeen
}

func schemaBooleanAttributeTrue(element *syntaxElement, local string) bool {
	if element == nil {
		return false
	}
	attributes := syntaxAttributesByLocal(element, local)
	if len(attributes) != 1 {
		return false
	}
	value, err := schemaBooleanValue(attributes[0])
	return err == nil && value
}

//nolint:gocognit // Keep version-neutral content attribute checks together.
func validateComplexTypeContentAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate) error {
	if err := validateUniqueSchemaAttributes(element, "id", "mixed"); err != nil {
		return err
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := stageSchemaCandidateError(candidate, validateSchemaQualifiedAttribute(attribute, element.name.local)); err != nil {
				return err
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, element.name.local+" id must be a valid NCName")
			}
		case "mixed":
			if element.name.local != "complexContent" {
				return newSchemaCompositionDiagnostic(attribute.loc, "simpleContent has forbidden attribute \"mixed\"")
			}
			if err := validateSchemaBoolean(attribute); err != nil {
				return err
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", element.name.local, attribute.name.local))
		}
	}
	return nil
}

//nolint:gocognit,funlen // Keep derivation ordering and recursive preflight explicit.
func validateComplexDerivation(element *syntaxElement, version XSDVersion, complexContent, simpleRestriction bool) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateUniqueSchemaAttributes(element, "base", "id"); err != nil {
		return err
	}
	baseAttributes := syntaxAttributesByLocal(element, "base")
	if len(baseAttributes) == 0 {
		if complexContent && element.name.local == "restriction" {
			return newSchemaComplexTypeBaseDiagnostic(
				element.loc,
				element.name.local+" requires a base attribute",
				nil,
				version,
				errSchemaComplexTypeBaseRequired,
			)
		}
		return newSchemaCompositionDiagnostic(element.loc, element.name.local+" requires a base attribute")
	}
	if err := validateConditionalQNameForSchema(element, baseAttributes[0]); err != nil {
		return err
	}
	enforceNonNegativeScale := directOrdinaryBuiltinScaleBase(element)
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := stageSchemaCandidateError(&candidate, validateSchemaQualifiedAttribute(attribute, element.name.local)); err != nil {
				return err
			}
			continue
		}
		switch attribute.name.local {
		case "base":
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, element.name.local+" id must be a valid NCName")
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", element.name.local, attribute.name.local))
		}
	}
	children, err := collectSimpleTypeChildren(element, element.name.local, &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	modelSeen := false
	openContentSeen := false
	openContentLoc := Loc{}
	attributesSeen := false
	anyAttributeSeen := false
	assertSeen := false
	simpleInlineSeen := false
	totalSeen := false
	fractionSeen := false
	facetSeen := make(map[string]bool)
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		switch child.name.local {
		case "simpleType":
			if !simpleRestriction || attributesSeen || anyAttributeSeen || assertSeen || simpleInlineSeen || len(facetSeen) > 0 || totalSeen || fractionSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" simpleType child is not permitted here")
			}
			simpleInlineSeen = true
			if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "totalDigits", "fractionDigits", "assertion", "enumeration", "explicitTimezone", "length", "maxExclusive", "maxInclusive", "maxLength", "maxScale", "minExclusive", "minInclusive", "minLength", "minScale", "pattern", "precision", "whiteSpace":
			if !simpleRestriction || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" facet is not permitted here")
			}
			if err := validateSimpleTypeRestrictionFacet(child, &totalSeen, &fractionSeen, facetSeen, version, false, false, enforceNonNegativeScale); err != nil && !candidate.considerError(err) {
				return err
			}
		case "openContent":
			if !complexContent {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" does not permit openContent")
			}
			if openContentSeen || modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" openContent must precede model and attributes")
			}
			openContentSeen = true
			openContentLoc = child.loc
			if err := validateOpenContent(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "group", "all", "sequence":
			if !complexContent {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" does not permit a model particle")
			}
			if modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" model child must be unique and precede attributes")
			}
			modelSeen = true
			if err := versionNamedModelGroupUnsupported(validateUnsupportedModelParticle(child, version), version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "choice":
			if !complexContent {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" does not permit a model particle")
			}
			if modelSeen || attributesSeen || anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" model child must be unique and precede attributes")
			}
			modelSeen = true
			if err := versionNamedModelGroupUnsupported(validateChoiceParticleWithNamespacePolicy(child, version, false), version); err != nil && !candidate.considerError(err) {
				return err
			}
		case "attribute", "attributeGroup":
			if anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" attributes must precede anyAttribute and assert")
			}
			attributesSeen = true
			childErr := validateLocalAttribute(child, version)
			if child.name.local == "attributeGroup" {
				childErr = validateAttributeGroupReference(child)
			}
			if childErr != nil && !candidate.considerError(childErr) {
				return childErr
			}
		case "anyAttribute":
			if anyAttributeSeen || assertSeen {
				return newSchemaCompositionDiagnostic(child.loc, element.name.local+" anyAttribute must be unique and last")
			}
			anyAttributeSeen = true
			boundedRestriction := complexContent && element.name.local == "restriction" && boundedComplexContentRestrictionCandidate(element, true)
			var anyAttributeErr error
			if boundedRestriction {
				anyAttributeErr = validateAnyAttributeSyntax(child, version)
			}
			if !boundedRestriction {
				anyAttributeErr = validateAnyAttribute(child, version)
			}
			if err := anyAttributeErr; err != nil && !candidate.considerError(err) {
				return err
			}
		case "assert":
			assertSeen = true
			if err := validateComplexTypeAssert(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		default:
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("%s contains forbidden child <%s>", element.name.local, child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("%s child <%s> is not implemented", element.name.local, child.name.local))
			continue
		}
	}
	if complexContent && element.name.local == "restriction" && openContentSeen && !modelSeen {
		return newSchemaCompositionDiagnostic(openContentLoc, "complexContent restriction openContent requires a model particle")
	}
	derivationErr := candidate.err()
	if derivationErr == nil && boundedComplexContentRestrictionCandidate(element, complexContent) {
		return nil
	}
	if derivationErr == nil {
		derivationErr = newSchemaSyntaxUnsupported(element.loc, element.name.local+" derivation is not implemented")
	}
	if simpleRestriction {
		return preferPrecisionDecimalRestrictionMismatch(element, version, derivationErr)
	}
	return derivationErr
}

func validateOpenContent(element *syntaxElement, version XSDVersion) error {
	return validateOpenContentLike(element, version, "openContent", false)
}

//nolint:gocognit // Keep the shared open-content grammar and policy boundary together.
func validateOpenContentLike(element *syntaxElement, version XSDVersion, owner string, defaultOpenContent bool) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateOpenContentAttributesLike(element, owner, defaultOpenContent, &candidate); err != nil {
		return err
	}
	children, err := collectSimpleTypeChildren(element, owner, &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	anySeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, "openContent annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		switch child.name.local {
		case "any":
			if anySeen {
				return newSchemaCompositionDiagnostic(child.loc, "openContent any child must be unique")
			}
			modeAttributes := syntaxAttributesByLocal(element, "mode")
			if len(modeAttributes) == 1 && collapseXMLWhitespace(modeAttributes[0].value) == "none" {
				return newSchemaCompositionDiagnostic(child.loc, "openContent mode none cannot have an any child")
			}
			anySeen = true
			if err := validateOpenContentAny(child, version, owner); err != nil && !candidate.considerError(err) {
				return err
			}
		default:
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("%s contains forbidden child <%s>", owner, child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("%s child <%s> is not implemented", owner, child.name.local))
			continue
		}
	}
	mode := "interleave"
	if modeAttributes := syntaxAttributesByLocal(element, "mode"); len(modeAttributes) == 1 {
		mode = collapseXMLWhitespace(modeAttributes[0].value)
	}
	if defaultOpenContent && !anySeen {
		return newSchemaCompositionDiagnostic(element.loc, owner+" requires exactly one any child")
	}
	if !defaultOpenContent && mode != "none" && !anySeen {
		return newSchemaCompositionDiagnostic(element.loc, owner+" requires an any child unless mode is none")
	}
	if version == XSDVersion10 {
		candidate.considerError(newXSD11FeatureMismatch(
			FeatureSchemaSyntax,
			UnsupportedSchemaSyntaxCode,
			element.loc,
			owner+" is an XSD 1.1-only construct",
		))
		return candidate.err()
	}
	if candidate.present {
		return candidate.err()
	}
	return newSchemaSyntaxUnsupportedForVersion(element.loc, owner+" is not implemented", version)
}

func validateOpenContentAny(element *syntaxElement, version XSDVersion, owner string) error {
	for _, local := range []string{"minOccurs", "maxOccurs"} {
		attributes := syntaxAttributesByLocal(element, local)
		if len(attributes) > 0 {
			return newSchemaCompositionDiagnostic(attributes[0].loc, owner+" any does not permit "+local)
		}
	}
	return validateAnyParticle(element, version)
}

func validateOpenContentAttributesLike(element *syntaxElement, owner string, defaultOpenContent bool, candidate *schemaChildUnsupportedCandidate) error {
	uniqueAttributes := []string{"id", "mode"}
	if defaultOpenContent {
		uniqueAttributes = append(uniqueAttributes, "appliesToEmpty")
	}
	if err := validateUniqueSchemaAttributes(element, uniqueAttributes...); err != nil {
		return err
	}
	for _, attribute := range element.attrs {
		if err := stageSchemaCandidateError(candidate, validateOpenContentAttribute(attribute, owner, defaultOpenContent)); err != nil {
			return err
		}
	}
	return nil
}

func validateOpenContentAttribute(attribute syntaxAttribute, owner string, defaultOpenContent bool) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		return validateSchemaQualifiedAttribute(attribute, owner)
	}
	switch attribute.name.local {
	case "id":
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newSchemaCompositionDiagnostic(attribute.loc, owner+" id must be a valid NCName")
		}
	case "mode":
		values := []string{"none", "interleave", "suffix"}
		if defaultOpenContent {
			values = []string{"interleave", "suffix"}
		}
		if err := validateSchemaEnum(attribute, values...); err != nil {
			return err
		}
	case "appliesToEmpty":
		if !defaultOpenContent {
			return newSchemaCompositionDiagnostic(attribute.loc, "openContent has forbidden attribute \"appliesToEmpty\"")
		}
		if err := validateSchemaBoolean(attribute); err != nil {
			return err
		}
	default:
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", owner, attribute.name.local))
	}
	return nil
}

//nolint:gocognit,funlen // Keep local attribute grammar and version checks together.
func validateLocalAttribute(element *syntaxElement, version XSDVersion) error {
	if err := validateUniqueSchemaAttributes(element, "name", "ref", "type", "use", "form", "default", "fixed", "targetNamespace", "inheritable", "id"); err != nil {
		return err
	}
	nameAttributes := syntaxAttributesByLocal(element, "name")
	refAttributes := syntaxAttributesByLocal(element, "ref")
	typeAttributes := syntaxAttributesByLocal(element, "type")
	targetNamespaceAttributes := syntaxAttributesByLocal(element, "targetNamespace")
	nameSeen := len(nameAttributes) == 1
	refSeen := len(refAttributes) == 1
	typeSeen := len(typeAttributes) == 1
	var candidate schemaChildUnsupportedCandidate
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := validateSchemaQualifiedAttribute(attribute, "local attribute"); err != nil {
				if !candidate.considerError(err) {
					return err
				}
			}
			continue
		}
		switch attribute.name.local {
		case "name":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "local attribute name must be a valid NCName")
			}
		case "ref", "type":
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return err
			}
		case "use":
			if err := validateSchemaEnum(attribute, "optional", "prohibited", "required"); err != nil {
				return err
			}
		case "form":
			if err := validateSchemaEnum(attribute, "qualified", "unqualified"); err != nil {
				return err
			}
		case "default", "fixed":
		case "targetNamespace":
			if err := validateSchemaAnyURI(attribute); err != nil {
				return err
			}
			if version == XSDVersion10 {
				candidate.considerError(newXSD11FeatureMismatch(
					FeatureSchemaSyntax,
					UnsupportedSchemaSyntaxCode,
					attribute.loc,
					"local attribute targetNamespace is an XSD 1.1-only construct",
				))
			}
			if version != XSDVersion10 {
				candidate.considerAtVersion(attribute.loc, "local attribute targetNamespace is not implemented", version)
			}
		case "inheritable":
			if err := validateSchemaBoolean(attribute); err != nil {
				return err
			}
			if version == XSDVersion10 {
				candidate.considerError(newXSD11FeatureMismatch(
					FeatureSchemaSyntax,
					UnsupportedSchemaSyntaxCode,
					attribute.loc,
					"local attribute inheritable is an XSD 1.1-only construct",
				))
			}
			if version != XSDVersion10 {
				candidate.considerAtVersion(attribute.loc, "local attribute inheritable is not implemented", version)
			}
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "local attribute id must be a valid NCName")
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("local attribute has forbidden attribute %q", attribute.name.local))
		}
	}
	if !nameSeen && !refSeen {
		return newSchemaCompositionDiagnostic(element.loc, "local attribute requires a name or ref attribute")
	}
	if nameSeen && refSeen {
		return newSchemaCompositionDiagnostic(refAttributes[0].loc, "local attribute cannot specify both name and ref")
	}
	if refSeen && (typeSeen || len(targetNamespaceAttributes) > 0 || len(syntaxAttributesByLocal(element, "form")) > 0) {
		return newSchemaCompositionDiagnostic(refAttributes[0].loc, "local attribute ref cannot combine with type, form, or targetNamespace")
	}
	formAttributes := syntaxAttributesByLocal(element, "form")
	if len(targetNamespaceAttributes) > 0 && len(formAttributes) > 0 {
		return newSchemaCompositionDiagnostic(formAttributes[0].loc, "local attribute targetNamespace cannot combine with form")
	}
	defaults := syntaxAttributesByLocal(element, "default")
	fixed := syntaxAttributesByLocal(element, "fixed")
	if len(defaults) > 0 && len(fixed) > 0 {
		return newSchemaCompositionDiagnostic(fixed[0].loc, "local attribute cannot specify both default and fixed")
	}
	useAttributes := syntaxAttributesByLocal(element, "use")
	use := "optional"
	if len(useAttributes) == 1 {
		use = collapseXMLWhitespace(useAttributes[0].value)
	}
	if len(defaults) > 0 && use != "optional" {
		return newSchemaCompositionDiagnostic(useAttributes[0].loc, "local attribute default requires use=optional")
	}
	if version == XSDVersion11 && len(fixed) > 0 && use == "prohibited" {
		return newSchemaCompositionDiagnostic(useAttributes[0].loc, "local attribute fixed cannot combine with use=prohibited")
	}
	children, err := collectSimpleTypeChildren(element, "local attribute", &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	contentSeen := false
	typeChildSeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, "local attribute annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		if child.name.local != "simpleType" {
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("local attribute contains forbidden child <%s>", child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("local attribute child <%s> is not implemented", child.name.local))
			continue
		}
		if refSeen || typeSeen || typeChildSeen {
			return newSchemaCompositionDiagnostic(child.loc, "local attribute cannot combine type or ref with an inline simpleType")
		}
		typeChildSeen = true
		if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
			return err
		}
	}
	if candidate.present {
		return candidate.err()
	}
	return newSchemaSyntaxUnsupported(element.loc, "local attribute declarations are not implemented")
}

//nolint:gocognit // Keep nested attribute-group reference grammar explicit.
func validateAttributeGroupReference(element *syntaxElement) error {
	if err := validateUniqueSchemaAttributes(element, "ref", "id"); err != nil {
		return err
	}
	var candidate schemaChildUnsupportedCandidate
	refAttributes := syntaxAttributesByLocal(element, "ref")
	if len(refAttributes) == 0 {
		return newSchemaCompositionDiagnostic(element.loc, "attributeGroup reference requires a ref attribute")
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := stageSchemaCandidateError(&candidate, validateSchemaQualifiedAttribute(attribute, "attributeGroup reference")); err != nil {
				return err
			}
			continue
		}
		switch attribute.name.local {
		case "ref":
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return err
			}
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "attributeGroup reference id must be a valid NCName")
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attributeGroup reference has forbidden attribute %q", attribute.name.local))
		}
	}
	children, err := collectSimpleTypeChildren(element, "attributeGroup reference", &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	for _, child := range children {
		if child.name.local != "annotation" {
			return newSchemaCompositionDiagnostic(child.loc, "attributeGroup reference permits only an annotation child")
		}
		if annotationSeen {
			return newSchemaCompositionDiagnostic(child.loc, "attributeGroup reference annotation must be unique")
		}
		annotationSeen = true
	}
	if candidate.present {
		return candidate.err()
	}
	return newSchemaSyntaxUnsupported(element.loc, "attributeGroup references are not implemented")
}

func validateAnyAttribute(element *syntaxElement, version XSDVersion) error {
	if err := validateAnyAttributeSyntax(element, version); err != nil {
		return err
	}
	return newSchemaAnyAttributeUnsupported(element.loc, version)
}

func validateDirectNamedComplexTypeAnyAttribute(parent, element *syntaxElement, version XSDVersion) error {
	if err := validateAnyAttributeSyntax(element, version); err != nil {
		return err
	}
	if !supportsDirectNamedComplexTypeAnyAttribute(parent) || !isSupportedDirectNamedComplexTypeAnyAttribute(element) {
		return newSchemaAnyAttributeUnsupported(element.loc, version)
	}
	return nil
}

func supportsDirectNamedComplexTypeAnyAttribute(parent *syntaxElement) bool {
	if parent == nil || parent.name.namespace != xsdNamespaceURI || parent.name.local != "complexType" {
		return false
	}
	if len(syntaxAttributesByLocal(parent, "name")) != 1 {
		return false
	}
	model := schemaComplexTypeModel(parent)
	if model == nil || model.name.namespace != xsdNamespaceURI {
		return false
	}
	return model.name.local == "sequence" || model.name.local == "choice"
}

func isSupportedAnyAttribute(element *syntaxElement) bool {
	namespaceAttributes := syntaxAttributesByLocal(element, "namespace")
	processContentsAttributes := syntaxAttributesByLocal(element, "processContents")
	if len(namespaceAttributes) != 1 || len(processContentsAttributes) != 1 {
		return false
	}
	return collapseXMLWhitespace(namespaceAttributes[0].value) == "##other" && collapseXMLWhitespace(processContentsAttributes[0].value) == "lax"
}

func isSupportedDirectNamedComplexTypeAnyAttribute(element *syntaxElement) bool {
	namespaceAttributes := syntaxAttributesByLocal(element, "namespace")
	processContentsAttributes := syntaxAttributesByLocal(element, "processContents")
	if len(syntaxAttributesByLocal(element, "notNamespace")) > 0 || len(syntaxAttributesByLocal(element, "notQName")) > 0 {
		return false
	}
	if len(namespaceAttributes) == 0 && len(processContentsAttributes) == 0 {
		return true
	}
	return isSupportedAnyAttribute(element)
}

//nolint:gocognit // Keep wildcard lexical/co-occurrence checks together.
func validateAnyAttributeSyntax(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateUniqueSchemaAttributes(element, "id", "namespace", "processContents", "notNamespace", "notQName"); err != nil {
		return err
	}
	namespaceAttributes := syntaxAttributesByLocal(element, "namespace")
	notNamespaceAttributes := syntaxAttributesByLocal(element, "notNamespace")
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := validateSchemaQualifiedAttribute(attribute, "anyAttribute"); err != nil {
				if !candidate.considerError(err) {
					return err
				}
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "anyAttribute id must be a valid NCName")
			}
		case "namespace":
			if err := validateWildcardNamespace(attribute); err != nil {
				return err
			}
		case "processContents":
			if err := validateSchemaEnum(attribute, "skip", "lax", "strict"); err != nil {
				return err
			}
		case "notNamespace":
			if err := validateWildcardNotNamespace(attribute); err != nil {
				return err
			}
			if version == XSDVersion10 {
				candidate.considerError(newXSD11FeatureMismatch(
					FeatureSchemaSyntax,
					UnsupportedSchemaSyntaxCode,
					attribute.loc,
					"anyAttribute notNamespace is an XSD 1.1-only construct",
				))
			}
		case "notQName":
			if err := validateWildcardNotQName(element, attribute, false); err != nil {
				return err
			}
			if version == XSDVersion10 {
				candidate.considerError(newXSD11FeatureMismatch(
					FeatureSchemaSyntax,
					UnsupportedSchemaSyntaxCode,
					attribute.loc,
					"anyAttribute notQName is an XSD 1.1-only construct",
				))
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("anyAttribute has forbidden attribute %q", attribute.name.local))
		}
	}
	if len(namespaceAttributes) > 0 && len(notNamespaceAttributes) > 0 {
		return newSchemaCompositionDiagnostic(namespaceAttributes[0].loc, "anyAttribute namespace cannot combine with notNamespace")
	}
	children, err := collectSimpleTypeChildren(element, "anyAttribute", &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	for _, child := range children {
		if child.name.local != "annotation" {
			return newSchemaCompositionDiagnostic(child.loc, "anyAttribute permits only an annotation child")
		}
		if annotationSeen {
			return newSchemaCompositionDiagnostic(child.loc, "anyAttribute annotation must be unique")
		}
		annotationSeen = true
	}
	if candidate.present {
		return candidate.err()
	}
	return nil
}

func validateWildcardNamespace(attribute syntaxAttribute) error {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid wildcard namespace", attribute.name.local))
	}
	tokens := strings.Split(lexeme, " ")
	for _, token := range tokens {
		switch token {
		case "##any", "##other", "##local", "##targetNamespace":
			if (token == "##any" || token == "##other") && len(tokens) != 1 {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid wildcard namespace", attribute.name.local))
			}
			continue
		}
		if strings.HasPrefix(token, "##") {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid wildcard namespace", attribute.name.local))
		}
		if err := validateSchemaAnyURI(syntaxAttribute{value: token, loc: attribute.loc, name: attribute.name}); err != nil {
			return err
		}
	}
	return nil
}

func validateWildcardNotNamespace(attribute syntaxAttribute) error {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid wildcard namespace", attribute.name.local))
	}
	for _, token := range strings.Split(lexeme, " ") {
		if token == "##local" || token == "##targetNamespace" { //nolint:gosec // XSD wildcard keywords are not credentials.
			continue
		}
		if strings.HasPrefix(token, "##") {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("attribute %q has an invalid wildcard namespace", attribute.name.local))
		}
		if err := validateSchemaAnyURI(syntaxAttribute{value: token, loc: attribute.loc, name: attribute.name}); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocognit // Keep wildcard sentinel and QName lexical checks together.
func validateWildcardNotQName(element *syntaxElement, attribute syntaxAttribute, allowDefinedSibling bool) error {
	lexeme := collapseXMLWhitespace(attribute.value)
	if lexeme == "" {
		return newSchemaCompositionDiagnostic(attribute.loc, "attribute \"notQName\" has an invalid wildcard value")
	}
	for _, token := range strings.Split(lexeme, " ") {
		if token == "##defined" || allowDefinedSibling && token == "##definedSibling" {
			continue
		}
		if token == "##definedSibling" {
			return newSchemaCompositionDiagnostic(attribute.loc, "attribute \"notQName\" does not permit ##definedSibling")
		}
		prefix, local, ok := splitConditionalQName(token)
		if !ok || !validNCName(local) || prefix != "" && !validNCName(prefix) {
			return newDiagnostic(FailureInvalid, invalidSchemaConditionalCode, attribute.loc, "attribute \"notQName\" has a malformed QName", nil)
		}
		if prefix != "" {
			if _, bound := element.scope.lookup(prefix); !bound {
				return newDiagnostic(FailureInvalid, invalidSchemaConditionalCode, attribute.loc, "attribute \"notQName\" has an unbound QName prefix", nil)
			}
		}
	}
	return nil
}

//nolint:gocognit // Keep XSD 1.1 assertion lexical checks together.
func validateComplexTypeAssert(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateUniqueSchemaAttributes(element, "id", "test", "xpathDefaultNamespace"); err != nil {
		return err
	}
	testAttributes := syntaxAttributesByLocal(element, "test")
	if len(testAttributes) == 0 {
		return newSchemaCompositionDiagnostic(element.loc, "assert requires a test attribute")
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if err := stageSchemaCandidateError(&candidate, validateSchemaQualifiedAttribute(attribute, "assert")); err != nil {
				return err
			}
			continue
		}
		switch attribute.name.local {
		case "test":
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "assert id must be a valid NCName")
			}
		case "xpathDefaultNamespace":
			if err := validateSchemaXPathDefaultNamespace(attribute); err != nil {
				return err
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("assert has forbidden attribute %q", attribute.name.local))
		}
	}
	children, err := collectSimpleTypeChildren(element, "assert", &candidate)
	if err != nil {
		return err
	}
	annotationSeen := false
	for _, child := range children {
		if child.name.local != "annotation" {
			return newSchemaCompositionDiagnostic(child.loc, "assert permits only an annotation child")
		}
		if annotationSeen {
			return newSchemaCompositionDiagnostic(child.loc, "assert annotation must be unique")
		}
		annotationSeen = true
	}
	if version == XSDVersion10 {
		candidate.considerError(newXSD11FeatureMismatch(
			FeatureID("xsd.assertion"),
			UnsupportedSchemaSyntaxCode,
			element.loc,
			"assert is an XSD 1.1-only construct",
		))
		return candidate.err()
	}
	if candidate.present {
		return candidate.err()
	}
	return newSchemaSyntaxUnsupportedForVersion(element.loc, "assertions are not implemented", version)
}

func validateChoiceParticleWithNamespacePolicy(element *syntaxElement, version XSDVersion, allowNamespacePolicy bool) error {
	return validateChoiceParticleWithOptions(element, version, false, allowNamespacePolicy)
}

func validateChoiceParticleWithOptions(element *syntaxElement, version XSDVersion, allowOccurrences, allowNamespacePolicy bool) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSchemaParticleAttributesWithOccurrencePolicy(element, &candidate, version, allowOccurrences); err != nil {
		return err
	}
	childrenCandidate, err := validateModelParticleChildrenWithOptions(element, "choice", version, allowOccurrences, allowNamespacePolicy)
	if err != nil {
		return err
	}
	candidate.merge(childrenCandidate)
	return candidate.err()
}

func validateModelParticleChildren(element *syntaxElement, model string, version XSDVersion) (schemaChildUnsupportedCandidate, error) {
	return validateModelParticleChildrenWithOptions(element, model, version, false, false)
}

//nolint:gocognit // Keep the supported direct-sequence grammar in the shared traversal.
func validateModelParticleChildrenWithOptions(element *syntaxElement, model string, version XSDVersion, allowElementOccurrences, allowNamespacePolicy bool) (schemaChildUnsupportedCandidate, error) {
	var candidate schemaChildUnsupportedCandidate
	annotationSeen := false
	contentSeen := false
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if !xmlWhitespace([]byte(textNode.data)) {
				return candidate, newSchemaCompositionDiagnostic(textNode.loc, model+" contains non-whitespace character data")
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return candidate, newSchemaBridgeInvariant(Loc{}, model+" contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			return candidate, newSchemaCompositionDiagnostic(child.loc, model+" contains a forbidden non-XSD child")
		}
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return candidate, newSchemaCompositionDiagnostic(child.loc, model+" annotation must be first and unique")
			}
			annotationSeen = true
			if err := stageSchemaCandidateError(&candidate, validateSchemaAnnotationElement(child)); err != nil {
				return candidate, err
			}
			continue
		}
		contentSeen = true
		switch child.name.local {
		case "element":
			localCandidate, err := validateLocalElementParticle(child, version, allowElementOccurrences, model, allowNamespacePolicy)
			if err != nil {
				return candidate, err
			}
			candidate.merge(localCandidate)
		case "group", "choice", "sequence", "any":
			if err := validateUnsupportedParticle(child, version); err != nil {
				if !candidate.considerError(err) {
					return candidate, err
				}
				continue
			}
			if !candidate.present {
				candidate.considerAt(child.loc, fmt.Sprintf("%s child <%s> is not implemented", model, child.name.local))
			}
		case "all":
			return candidate, newSchemaCompositionDiagnostic(child.loc, model+" cannot contain an all particle")
		default:
			if err := forbiddenGlobalSchemaChild(model, child); err != nil {
				return candidate, err
			}
			candidate.considerAt(child.loc, fmt.Sprintf("%s child <%s> is not implemented", model, child.name.local))
		}
	}
	return candidate, nil
}

func validateSchemaParticleAttributes(element *syntaxElement, candidate *schemaChildUnsupportedCandidate, version XSDVersion) error {
	return validateSchemaParticleAttributesWithOccurrencePolicy(element, candidate, version, false)
}

//nolint:gocognit // Keep particle attribute validation and occurrence policy together.
func validateSchemaParticleAttributesWithOccurrencePolicy(element *syntaxElement, candidate *schemaChildUnsupportedCandidate, version XSDVersion, allowOccurrences bool) error {
	if err := validateSchemaParticleOccurrences(element, version); err != nil {
		return err
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", element.name.local, attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := stageSchemaCandidateError(candidate, validateSchemaXMLAttribute(attribute)); err != nil {
					return err
				}
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "particle id must be a valid NCName")
			}
		case "minOccurs", "maxOccurs":
			if !allowOccurrences {
				candidate.considerAt(attribute.loc, fmt.Sprintf("particle attribute %q is not implemented", attribute.name.local))
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("%s has forbidden attribute %q", element.name.local, attribute.name.local))
		}
	}
	return nil
}

func validateSchemaParticleOccurrences(element *syntaxElement, version XSDVersion) error {
	_, err := schemaParticleOccurrenceRange(element, version)
	return err
}

type schemaParticleOccurrenceLexical struct {
	present bool
	lexical string
	loc     Loc
}

//nolint:gocognit // Keep lexical parsing, exact range construction, and diagnostics together.
func schemaParticleOccurrenceRange(element *syntaxElement, version XSDVersion) (particleOccurrenceRange, error) {
	minimum := schemaParticleOccurrenceLexical{lexical: "1", loc: element.loc}
	maximum := schemaParticleOccurrenceLexical{lexical: "1", loc: element.loc}
	for _, attribute := range element.attrs {
		if attribute.name.namespace != "" {
			continue
		}
		if attribute.name.local != "minOccurs" && attribute.name.local != "maxOccurs" {
			continue
		}
		value := &minimum
		if attribute.name.local == "maxOccurs" {
			value = &maximum
		}
		if value.present {
			return particleOccurrenceRange{}, newSchemaCompositionDiagnostic(
				attribute.loc,
				fmt.Sprintf("particle attribute %q must be unique", attribute.name.local),
			)
		}
		value.present = true
		value.lexical = attribute.value
		value.loc = attribute.loc
	}

	minimumValue, err := parseParticleOccurrence(minimum.lexical, false, minimum.loc)
	if err != nil {
		return particleOccurrenceRange{}, invalidSchemaParticleOccurrence("minOccurs", minimum, err, version)
	}
	maximumValue, err := parseParticleOccurrence(maximum.lexical, true, maximum.loc)
	if err != nil {
		return particleOccurrenceRange{}, invalidSchemaParticleOccurrence("maxOccurs", maximum, err, version)
	}
	occurrences, err := newParticleOccurrenceRange(minimumValue, maximumValue)
	if errors.Is(err, errParticleOccurrenceMinimumExceedsMaximum) {
		related := make([]Loc, 0, 2)
		if minimum.present {
			related = append(related, minimum.loc)
		}
		if maximum.present {
			related = append(related, maximum.loc)
		}
		return particleOccurrenceRange{}, newSchemaParticleOccurrenceDiagnostic(
			element.loc,
			"particle minOccurs cannot exceed maxOccurs",
			related,
			version,
			err,
		)
	}
	if err != nil {
		return particleOccurrenceRange{}, newSchemaBridgeInvariant(
			element.loc,
			"construct particle occurrence range",
		)
	}
	return occurrences, nil
}

func newSchemaParticleOccurrenceDiagnostic(loc Loc, message string, related []Loc, version XSDVersion, cause error) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    invalidSchemaCompositionCode,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: schemaParticleCorrectSpecRef(version),
		cause:   cause,
	}
}

func schemaParticleCorrectSpecRef(version XSDVersion) string {
	switch version {
	case XSDVersion10:
		return "xsd10-structures#coss-particle"
	case XSDVersion11:
		return "xsd11-structures#coss-particle"
	default:
		return ""
	}
}

func invalidSchemaParticleOccurrence(name string, occurrence schemaParticleOccurrenceLexical, cause error, version XSDVersion) Diagnostic {
	specRef := schemaParticleOccurrenceDatatypeSpecRef(version)
	if name == "minOccurs" && collapseXMLWhitespace(occurrence.lexical) == "unbounded" {
		specRef = schemaParticleMinimumSpecRef(version)
	}
	return Diagnostic{
		class:   FailureInvalid,
		code:    invalidSchemaCompositionCode,
		loc:     occurrence.loc,
		message: fmt.Sprintf("attribute %q has an invalid occurrence value", name),
		specRef: specRef,
		cause:   cause,
	}
}

func schemaParticleOccurrenceDatatypeSpecRef(version XSDVersion) string {
	switch version {
	case XSDVersion10:
		return "xsd10-datatypes#nonNegativeInteger"
	case XSDVersion11:
		return "xsd11-datatypes#nonNegativeInteger"
	default:
		return ""
	}
}

func schemaParticleMinimumSpecRef(version XSDVersion) string {
	switch version {
	case XSDVersion10:
		return "xsd10-structures#p-min_occurs"
	case XSDVersion11:
		return "xsd11-structures#p-min_occurs"
	default:
		return ""
	}
}

//nolint:gocognit,funlen // Keep XSD 1.1 alternative lexical and structural checks together.
func validateChoiceElementAlternative(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	idAttributes := syntaxAttributesByLocal(element, "id")
	testAttributes := syntaxAttributesByLocal(element, "test")
	typeAttributes := syntaxAttributesByLocal(element, "type")
	xpathNamespaceAttributes := syntaxAttributesByLocal(element, "xpathDefaultNamespace")
	if len(idAttributes) > 1 {
		return newSchemaCompositionDiagnostic(element.loc, "alternative id must be unique")
	}
	if len(testAttributes) > 1 {
		return newSchemaCompositionDiagnostic(element.loc, "alternative test must be unique")
	}
	if len(typeAttributes) > 1 {
		return newSchemaCompositionDiagnostic(element.loc, "alternative type must be unique")
	}
	if len(xpathNamespaceAttributes) > 1 {
		return newSchemaCompositionDiagnostic(element.loc, "alternative xpathDefaultNamespace must be unique")
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("alternative has forbidden attribute %q", attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := validateSchemaXMLAttribute(attribute); err != nil {
					if !candidate.considerError(err) {
						return err
					}
				}
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "alternative id must be a valid NCName")
			}
		case "test":
			if collapseXMLWhitespace(attribute.value) == "" {
				return newSchemaCompositionDiagnostic(attribute.loc, "alternative test cannot be empty")
			}
		case "type":
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return err
			}
		case "xpathDefaultNamespace":
			if err := validateSchemaXPathDefaultNamespace(attribute); err != nil {
				return err
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("alternative has forbidden attribute %q", attribute.name.local))
		}
	}
	children, collectedCandidate, err := collectGlobalSchemaChildren(element)
	if err != nil {
		return err
	}
	candidate.merge(collectedCandidate)
	annotationSeen := false
	contentSeen := false
	typeChildSeen := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return newSchemaCompositionDiagnostic(child.loc, "alternative annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		switch child.name.local {
		case "simpleType", "complexType":
			if typeChildSeen {
				return newSchemaCompositionDiagnostic(child.loc, "alternative type child must be unique")
			}
			if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
			typeChildSeen = true
		default:
			if isKnownSchemaElement(child.name.local) {
				return newSchemaCompositionDiagnostic(child.loc, fmt.Sprintf("alternative contains forbidden child <%s>", child.name.local))
			}
			candidate.considerAt(child.loc, fmt.Sprintf("alternative child <%s> is not implemented", child.name.local))
			continue
		}
	}
	typeAttributeSeen := len(typeAttributes) == 1
	if typeAttributeSeen && typeChildSeen {
		return newSchemaCompositionDiagnostic(element.loc, "alternative cannot combine type with an inline type")
	}
	if !typeAttributeSeen && !typeChildSeen {
		return newSchemaCompositionDiagnostic(element.loc, "alternative requires a type or inline type")
	}
	if version == XSDVersion10 {
		candidate.considerError(newXSD11FeatureMismatch(
			FeatureSchemaSyntax,
			UnsupportedSchemaSyntaxCode,
			element.loc,
			"alternative is an XSD 1.1-only construct",
		))
	}
	return candidate.err()
}

//nolint:gocognit // Keep inline type attribute support and child preflight together.
func validateInlineSchemaType(element *syntaxElement, version XSDVersion) error {
	kind, ok := schemaDeclarationKind(element.name.local)
	if !ok || kind != ComponentKindSimpleTypeDefinition && kind != ComponentKindComplexTypeDefinition {
		return newSchemaBridgeInvariant(element.loc, "inline schema type has an unknown kind")
	}
	var candidate schemaChildUnsupportedCandidate
	for _, attribute := range element.attrs {
		if attribute.name.namespace == "" && attribute.name.local == "name" {
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("inline %s cannot specify a name", element.name.local))
		}
		if element.name.local == "simpleType" && attribute.name.namespace == "" && attribute.name.local == "final" {
			return newSchemaCompositionDiagnostic(attribute.loc, "inline simpleType cannot specify final")
		}
		if element.name.local == "complexType" && attribute.name.namespace == "" {
			switch attribute.name.local {
			case "abstract", "block", "final":
				return newSchemaCompositionDiagnostic(attribute.loc, "inline complexType cannot specify "+attribute.name.local)
			}
		}
		message, err := validateGlobalSchemaAttribute(element, kind, attribute, version)
		if err != nil {
			if !candidate.considerError(err) {
				return err
			}
			continue
		}
		if message != "" {
			candidate.considerAt(attribute.loc, message)
		}
	}
	bridgeStringEnumeration := element.name.local == "simpleType" && inlineSimpleTypeMayHaveStringRestrictionBase(element)
	if err := validateGlobalSchemaChildrenWithFacetBridge(element, version, false, bridgeStringEnumeration); err != nil {
		if !candidate.considerError(err) {
			return err
		}
	}
	if candidate.present {
		return candidate.err()
	}
	return nil
}

func inlineSimpleTypeMayHaveStringRestrictionBase(element *syntaxElement) bool {
	if element == nil || element.name.local != "simpleType" {
		return false
	}
	for _, node := range element.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI || child.name.local != "restriction" {
			continue
		}
		baseAttributes := syntaxAttributesByLocal(child, "base")
		if len(baseAttributes) == 0 {
			return inlineSimpleTypeMayHaveStringRestrictionBase(inlineSimpleTypeChild(child))
		}
		if len(baseAttributes) != 1 {
			return false
		}
		base, err := expandSchemaQName(child, baseAttributes[0])
		if err != nil {
			return false
		}
		if base.Namespace() == xsdNamespaceURI {
			return base.Local() == "string"
		}
		return true
	}
	return false
}

//nolint:gocognit,funlen // Keep local element grammar, lexical checks, and support boundaries together.
func validateLocalElementParticle(element *syntaxElement, version XSDVersion, allowOccurrences bool, model string, allowNamespacePolicy bool) (schemaChildUnsupportedCandidate, error) {
	var candidate schemaChildUnsupportedCandidate
	nameAttributes := syntaxAttributesByLocal(element, "name")
	refAttributes := syntaxAttributesByLocal(element, "ref")
	typeAttributes := syntaxAttributesByLocal(element, "type")
	targetNamespaceAttributes := syntaxAttributesByLocal(element, "targetNamespace")
	if len(nameAttributes) > 1 {
		return candidate, newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, element.loc, "local element name must be unique", nil)
	}
	if len(refAttributes) > 1 {
		return candidate, newSchemaCompositionDiagnostic(element.loc, "local element ref must be unique")
	}
	if len(typeAttributes) > 1 {
		return candidate, newSchemaCompositionDiagnostic(element.loc, "local element type must be unique")
	}
	if len(targetNamespaceAttributes) > 1 {
		return candidate, newSchemaCompositionDiagnostic(element.loc, "local element targetNamespace must be unique")
	}
	nameSeen := len(nameAttributes) == 1
	refSeen := len(refAttributes) == 1
	typeSeen := len(typeAttributes) == 1
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return candidate, newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("local element has forbidden attribute %q", attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := validateSchemaXMLAttribute(attribute); err != nil {
					if !candidate.considerError(err) {
						return candidate, err
					}
				}
			}
			continue
		}
		switch attribute.name.local {
		case "name":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return candidate, newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, attribute.loc, "local element name must be an unqualified valid NCName", nil)
			}
		case "ref":
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return candidate, err
			}
		case "type":
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return candidate, err
			}
		case "form":
			if err := validateSchemaEnum(attribute, "qualified", "unqualified"); err != nil {
				return candidate, err
			}
			if !allowNamespacePolicy {
				candidate.considerAt(attribute.loc, "local element form policy is not implemented")
			}
		case "targetNamespace":
			if err := validateSchemaAnyURI(attribute); err != nil {
				return candidate, err
			}
		case "minOccurs", "maxOccurs":
			if err := validateSchemaParticleOccurrences(element, version); err != nil {
				return candidate, err
			}
			if !allowOccurrences {
				candidate.considerAt(attribute.loc, fmt.Sprintf("local element attribute %q is not implemented", attribute.name.local))
			}
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return candidate, newSchemaCompositionDiagnostic(attribute.loc, "local element id must be a valid NCName")
			}
		case "abstract", "substitutionGroup":
			return candidate, newSchemaCompositionDiagnostic(attribute.loc, "local element has forbidden attribute "+attribute.name.local)
		case "default", "fixed", "block", "nillable":
			if err := validateRecognizedUnsupportedAttribute(element, attribute, version); err != nil {
				return candidate, err
			}
			candidate.considerAt(attribute.loc, fmt.Sprintf("local element attribute %q is not implemented", attribute.name.local))
		default:
			return candidate, newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("local element has forbidden attribute %q", attribute.name.local))
		}
	}
	if !nameSeen && !refSeen {
		return candidate, newDiagnostic(FailureInvalid, invalidSchemaDeclarationNameCode, element.loc, "local element requires a name or ref attribute", nil)
	}
	if nameSeen && refSeen {
		return candidate, newSchemaCompositionDiagnostic(refAttributes[0].loc, "local element cannot specify both name and ref")
	}
	if refSeen {
		for _, local := range []string{"type", "form", "targetNamespace", "default", "fixed", "abstract", "block", "nillable", "substitutionGroup"} {
			attributes := syntaxAttributesByLocal(element, local)
			if len(attributes) == 0 {
				continue
			}
			return candidate, newSchemaCompositionDiagnostic(attributes[0].loc, fmt.Sprintf("local element ref cannot combine with %q", local))
		}
	}
	formAttributes := syntaxAttributesByLocal(element, "form")
	if len(targetNamespaceAttributes) > 0 && len(formAttributes) > 0 {
		return candidate, newSchemaCompositionDiagnostic(formAttributes[0].loc, "local element targetNamespace cannot combine with form")
	}
	if len(targetNamespaceAttributes) > 0 {
		if version == XSDVersion10 {
			if err := validateSchemaAnyURI(targetNamespaceAttributes[0]); err != nil {
				return candidate, err
			}
			attributeIndex, _ := schemaAttributeIndex(element, "targetNamespace")
			candidate.considerErrorAt(newXSD11FeatureMismatch(
				FeatureSchemaSyntax,
				UnsupportedSchemaSyntaxCode,
				targetNamespaceAttributes[0].loc,
				"local element targetNamespace is an XSD 1.1-only construct",
			), element, attributeIndex)
		}
		if version != XSDVersion10 && !allowNamespacePolicy {
			candidate.considerAtVersion(targetNamespaceAttributes[0].loc, "local element targetNamespace is not implemented", version)
		}
	}
	defaults := syntaxAttributesByLocal(element, "default")
	fixed := syntaxAttributesByLocal(element, "fixed")
	if len(defaults) > 0 && len(fixed) > 0 {
		return candidate, newSchemaCompositionDiagnostic(fixed[0].loc, "local element cannot specify both default and fixed")
	}
	children, collectedCandidate, err := collectGlobalSchemaChildren(element)
	if err != nil {
		return candidate, err
	}
	candidate.merge(collectedCandidate)
	annotationSeen := false
	contentSeen := false
	typeChildSeen := false
	constraintPhase := false
	for _, child := range children {
		if child.name.local == "annotation" {
			if annotationSeen || contentSeen {
				return candidate, newSchemaCompositionDiagnostic(child.loc, "local element annotation must be first and unique")
			}
			annotationSeen = true
			continue
		}
		contentSeen = true
		switch child.name.local {
		case "simpleType", "complexType":
			if refSeen || typeSeen {
				return candidate, newSchemaCompositionDiagnostic(child.loc, "local element cannot combine type or ref with an inline type")
			}
			if typeChildSeen || constraintPhase {
				return candidate, newSchemaCompositionDiagnostic(child.loc, "local element type child must be unique and precede constraints")
			}
			if err := validateInlineSchemaType(child, version); err != nil && !candidate.considerError(err) {
				return candidate, err
			}
			typeChildSeen = true
			if !candidate.present {
				candidate.considerAt(child.loc, fmt.Sprintf("local element child <%s> is not implemented", child.name.local))
			}
		case "alternative":
			if refSeen || constraintPhase {
				return candidate, newSchemaCompositionDiagnostic(child.loc, "local element alternative must precede identity constraints")
			}
			alternativeErr := validateChoiceElementAlternative(child, version)
			if alternativeErr != nil && !candidate.considerError(alternativeErr) {
				return candidate, alternativeErr
			}
			if !candidate.present {
				candidate.considerAtVersion(child.loc, "local element alternatives are not implemented", version)
			}
		case "unique", "key", "keyref":
			if refSeen {
				return candidate, newSchemaCompositionDiagnostic(child.loc, "local element ref cannot have identity constraints")
			}
			constraintPhase = true
			candidate.considerAt(child.loc, fmt.Sprintf("local element child <%s> is not implemented", child.name.local))
		default:
			if err := forbiddenGlobalSchemaChild("local element", child); err != nil {
				return candidate, err
			}
			candidate.considerAt(child.loc, fmt.Sprintf("local element child <%s> is not implemented", child.name.local))
		}
	}
	if refSeen || typeSeen || typeChildSeen {
		return candidate, nil
	}
	candidate.considerAt(element.loc, fmt.Sprintf("local %s elements without declared types are not implemented", model))
	return candidate, nil
}

func validateUnsupportedParticle(element *syntaxElement, version XSDVersion) error {
	switch element.name.local {
	case "choice":
		return validateChoiceParticleWithNamespacePolicy(element, version, false)
	case "sequence":
		return validateSequenceParticle(element, version)
	case "group":
		return validateGroupParticle(element, version)
	case "any":
		return validateAnyParticle(element, version)
	default:
		return newSchemaBridgeInvariant(element.loc, "unsupported particle has an unknown kind")
	}
}

func validateUnsupportedModelParticle(element *syntaxElement, version XSDVersion) error {
	switch element.name.local {
	case "all":
		return validateAllParticle(element, version)
	case "choice":
		return validateChoiceParticleWithNamespacePolicy(element, version, false)
	case "sequence", "group":
		return validateUnsupportedParticle(element, version)
	default:
		return newSchemaBridgeInvariant(element.loc, "unsupported model particle has an unknown kind")
	}
}

func validateSequenceParticle(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSchemaParticleAttributes(element, &candidate, version); err != nil {
		return err
	}
	childrenCandidate, err := validateModelParticleChildren(element, "sequence", version)
	if err != nil {
		return err
	}
	candidate.merge(childrenCandidate)
	return candidate.err()
}

func validateSupportedSequenceParticle(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSchemaParticleAttributesWithOccurrencePolicy(element, &candidate, version, true); err != nil {
		return err
	}
	childrenCandidate, err := validateModelParticleChildrenWithOptions(element, "sequence", version, true, true)
	if err != nil {
		return err
	}
	candidate.merge(childrenCandidate)
	return candidate.err()
}

func validateComplexTypeSequenceParticle(parent, sequence *syntaxElement, version XSDVersion) error {
	if len(syntaxAttributesByLocal(parent, "name")) != 1 {
		return validateUnsupportedParticle(sequence, version)
	}
	return validateSupportedSequenceParticle(sequence, version)
}

//nolint:gocognit // Keep group particle grammar and support classification together.
func validateGroupParticle(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSchemaParticleOccurrences(element, version); err != nil {
		return err
	}
	refSeen := false
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("group has forbidden attribute %q", attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := stageSchemaCandidateError(&candidate, validateSchemaXMLAttribute(attribute)); err != nil {
					return err
				}
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "group id must be a valid NCName")
			}
		case "ref":
			if refSeen {
				return newSchemaCompositionDiagnostic(attribute.loc, "group ref must be unique")
			}
			if err := validateConditionalQNameForSchema(element, attribute); err != nil {
				return err
			}
			refSeen = true
			candidate.considerAt(attribute.loc, "group reference particles are not implemented")
		case "minOccurs", "maxOccurs":
			candidate.considerAt(attribute.loc, fmt.Sprintf("group attribute %q is not implemented", attribute.name.local))
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("group has forbidden attribute %q", attribute.name.local))
		}
	}
	if !refSeen {
		return newSchemaCompositionDiagnostic(element.loc, "group particle requires a ref attribute")
	}
	annotationSeen := false
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if !xmlWhitespace([]byte(textNode.data)) {
				return newSchemaCompositionDiagnostic(textNode.loc, "group contains non-whitespace character data")
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(Loc{}, "group contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI || child.name.local != "annotation" {
			return newSchemaCompositionDiagnostic(child.loc, "group particle permits only an annotation child")
		}
		if annotationSeen {
			return newSchemaCompositionDiagnostic(child.loc, "group annotation must be unique")
		}
		annotationSeen = true
		if err := stageSchemaCandidateError(&candidate, validateSchemaAnnotationElement(child)); err != nil {
			return err
		}
	}
	return candidate.err()
}

func validateAllParticle(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateAllParticleOccurrences(element, "all particle", version); err != nil {
		if !candidate.considerError(err) {
			return err
		}
	}
	if err := validateSchemaParticleAttributes(element, &candidate, version); err != nil {
		return err
	}
	if err := validateAllParticleChildren(element, version, &candidate); err != nil {
		return err
	}
	return candidate.err()
}

func validateAllParticleChildren(element *syntaxElement, version XSDVersion, candidate *schemaChildUnsupportedCandidate) error {
	annotationSeen := false
	contentSeen := false
	for _, node := range element.children {
		if err := validateAllParticleChild(node, version, &annotationSeen, &contentSeen, candidate); err != nil {
			return err
		}
	}
	return nil
}

func validateAllParticleChild(node syntaxNode, version XSDVersion, annotationSeen, contentSeen *bool, candidate *schemaChildUnsupportedCandidate) error {
	textNode, ok := node.(syntaxText)
	if ok {
		if !xmlWhitespace([]byte(textNode.data)) {
			return newSchemaCompositionDiagnostic(textNode.loc, "all contains non-whitespace character data")
		}
		return nil
	}
	child, ok := node.(*syntaxElement)
	if !ok {
		return newSchemaBridgeInvariant(Loc{}, "all contains an unknown syntax node")
	}
	if child.name.namespace != xsdNamespaceURI {
		return newSchemaCompositionDiagnostic(child.loc, "all contains a forbidden non-XSD child")
	}
	if child.name.local == "annotation" {
		if *annotationSeen || *contentSeen {
			return newSchemaCompositionDiagnostic(child.loc, "all annotation must be first and unique")
		}
		*annotationSeen = true
		return stageSchemaCandidateError(candidate, validateSchemaAnnotationElement(child))
	}
	*contentSeen = true
	return validateAllParticleContentChild(child, version, candidate)
}

func validateAllParticleContentChild(child *syntaxElement, version XSDVersion, candidate *schemaChildUnsupportedCandidate) error {
	switch child.name.local {
	case "element":
		localCandidate, err := validateLocalElementParticle(child, version, false, "all", false)
		if err != nil {
			if !localCandidate.considerError(err) {
				return err
			}
		}
		occurrenceErr := validateAllChildParticleOccurrences(child, "all element", version)
		if err := stageAllChildOccurrenceCandidate(&localCandidate, child, occurrenceErr); err != nil {
			return err
		}
		candidate.merge(localCandidate)
		return nil
	case "any":
		var childCandidate schemaChildUnsupportedCandidate
		childErr := validateAllParticleUnsupportedChild(child, version, &childCandidate)
		if childErr != nil {
			if !childCandidate.considerError(childErr) {
				return childErr
			}
		}
		occurrenceErr := validateAllChildParticleOccurrences(child, "all any", version)
		if err := stageAllChildOccurrenceCandidate(&childCandidate, child, occurrenceErr); err != nil {
			return err
		}
		candidate.merge(childCandidate)
		return nil
	case "group":
		return validateAllParticleUnsupportedChild(child, version, candidate)
	default:
		return newSchemaCompositionDiagnostic(child.loc, "all particle contains a forbidden child")
	}
}

//nolint:gocognit // Keep all-child version classification and candidate staging together.
func validateAllParticleUnsupportedChild(child *syntaxElement, version XSDVersion, candidate *schemaChildUnsupportedCandidate) error {
	if child.name.local == "group" {
		if err := validateAllGroupOccurrences(child, version); err != nil {
			return err
		}
	}
	childErr := validateUnsupportedParticle(child, version)
	if childErr != nil {
		if version == XSDVersion10 {
			if mismatchErr := validateAllParticleXSD10Error(child, childErr); mismatchErr != nil {
				if errors.Is(childErr, errLanguagePolicyMismatch) {
					if attributeIndex, ok := schemaAnyMismatchAttributeIndex(child); ok {
						if candidate.considerErrorAt(mismatchErr, child, attributeIndex) {
							return nil
						}
						return mismatchErr
					}
				}
				if candidate.considerError(mismatchErr) {
					return nil
				}
				return mismatchErr
			}
		}
		if !candidate.considerError(childErr) {
			return childErr
		}
		return nil
	}
	if version == XSDVersion10 {
		candidate.considerError(newXSD11FeatureMismatch(
			FeatureSchemaSyntax,
			UnsupportedSchemaSyntaxCode,
			child.loc,
			fmt.Sprintf("all child <%s> is an XSD 1.1-only construct", child.name.local),
		))
		return nil
	}
	candidate.considerAt(child.loc, fmt.Sprintf("all child <%s> is not implemented", child.name.local))
	return nil
}

func validateAllParticleXSD10Error(child *syntaxElement, childErr error) error {
	if errors.Is(childErr, errLanguagePolicyMismatch) {
		return childErr
	}
	var diagnostic Diagnostic
	if errors.As(childErr, &diagnostic) && diagnostic.Class() == FailureUnsupported {
		return newXSD11FeatureMismatch(
			FeatureSchemaSyntax,
			UnsupportedSchemaSyntaxCode,
			child.loc,
			fmt.Sprintf("all child <%s> is an XSD 1.1-only construct", child.name.local),
		)
	}
	return nil
}

func validateAllParticleOccurrences(element *syntaxElement, owner string, version XSDVersion) error {
	occurrences, err := schemaParticleOccurrenceRange(element, version)
	if err != nil {
		return err
	}
	one, err := ParseStrictInteger("1", element.loc)
	if err != nil {
		return newSchemaBridgeInvariant(element.loc, "construct all particle occurrence bound")
	}
	minimumValue, ok := occurrences.minimumOccurrence().finiteValue()
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "all particle minimum occurrence is not finite")
	}
	if minimumValue.Compare(one) > 0 {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "minOccurs"), owner+" minOccurs must be 0 or 1")
	}
	maximumOccurrence := occurrences.maximumOccurrence()
	if maximumOccurrence.isUnbounded() {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "maxOccurs"), owner+" maxOccurs must be 1")
	}
	maximumValue, ok := maximumOccurrence.finiteValue()
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "all particle maximum occurrence is not finite")
	}
	if maximumValue.IsZero() && minimumValue.IsZero() {
		if version == XSDVersion10 {
			return newXSD11FeatureMismatch(
				FeatureSchemaSyntax,
				UnsupportedSchemaSyntaxCode,
				schemaParticleOccurrenceLoc(element, "maxOccurs"),
				owner+" maxOccurs=0 is an XSD 1.1-only construct",
			)
		}
		return nil
	}
	if maximumValue.Compare(one) != 0 {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "maxOccurs"), owner+" maxOccurs must be 1")
	}
	return nil
}

func validateAllChildParticleOccurrences(element *syntaxElement, owner string, version XSDVersion) error {
	occurrences, err := schemaParticleOccurrenceRange(element, version)
	if err != nil {
		return err
	}
	one, err := ParseStrictInteger("1", element.loc)
	if err != nil {
		return newSchemaBridgeInvariant(element.loc, "construct all child occurrence bound")
	}
	minimumValue, ok := occurrences.minimumOccurrence().finiteValue()
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "all child minimum occurrence is not finite")
	}
	if minimumValue.Compare(one) > 0 {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "minOccurs"), owner+" minOccurs must be 0 or 1")
	}
	maximumOccurrence := occurrences.maximumOccurrence()
	if maximumOccurrence.isUnbounded() {
		if version == XSDVersion10 {
			return newAllChildOccurrenceVersionMismatch(element, owner, "maxOccurs=unbounded")
		}
		return nil
	}
	maximumValue, ok := maximumOccurrence.finiteValue()
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "all child maximum occurrence is not finite")
	}
	if maximumValue.Compare(one) <= 0 {
		if maximumValue.IsZero() && minimumValue.IsZero() && version == XSDVersion10 {
			return newAllChildOccurrenceVersionMismatch(element, owner, "maxOccurs=0")
		}
		return nil
	}
	if version == XSDVersion10 {
		return newAllChildOccurrenceVersionMismatch(element, owner, "maxOccurs greater than 1")
	}
	return nil
}

func newAllChildOccurrenceVersionMismatch(element *syntaxElement, owner, occurrence string) error {
	return newXSD11FeatureMismatch(
		FeatureSchemaSyntax,
		diagnosticSchemaAllOccurrenceVersionCode,
		schemaParticleOccurrenceLoc(element, "maxOccurs"),
		fmt.Sprintf("%s %s is an XSD 1.1-only construct", owner, occurrence),
	)
}

func validateAllGroupOccurrences(element *syntaxElement, version XSDVersion) error {
	occurrences, err := schemaParticleOccurrenceRange(element, version)
	if err != nil {
		return err
	}
	one, err := ParseStrictInteger("1", element.loc)
	if err != nil {
		return newSchemaBridgeInvariant(element.loc, "construct all group occurrence bound")
	}
	minimumValue, ok := occurrences.minimumOccurrence().finiteValue()
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "all group minimum occurrence is not finite")
	}
	if minimumValue.Compare(one) != 0 {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "minOccurs"), "all group minOccurs must be 1")
	}
	maximumOccurrence := occurrences.maximumOccurrence()
	if maximumOccurrence.isUnbounded() {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "maxOccurs"), "all group maxOccurs must be 1")
	}
	maximumValue, ok := maximumOccurrence.finiteValue()
	if !ok {
		return newSchemaBridgeInvariant(element.loc, "all group maximum occurrence is not finite")
	}
	if maximumValue.Compare(one) != 0 {
		return newSchemaCompositionDiagnostic(schemaParticleOccurrenceLoc(element, "maxOccurs"), "all group maxOccurs must be 1")
	}
	return nil
}

func schemaParticleOccurrenceLoc(element *syntaxElement, local string) Loc {
	attributes := syntaxAttributesByLocal(element, local)
	if len(attributes) == 1 {
		return attributes[0].loc
	}
	return element.loc
}

//nolint:gocognit,funlen // Keep wildcard particle grammar and unsupported classification together.
func validateAnyParticle(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateUniqueSchemaAttributes(element, "id", "minOccurs", "maxOccurs", "namespace", "notNamespace", "processContents", "notQName"); err != nil {
		return err
	}
	namespaceAttributes := syntaxAttributesByLocal(element, "namespace")
	notNamespaceAttributes := syntaxAttributesByLocal(element, "notNamespace")
	if err := validateSchemaParticleOccurrences(element, version); err != nil {
		return err
	}
	annotationSeen := false
	for attributeIndex, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			continue
		}
		if attribute.name.namespace != "" {
			if attribute.name.namespace == xsdNamespaceURI {
				return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("any has forbidden attribute %q", attribute.name.local))
			}
			if attribute.name.namespace == xmlNamespaceURI {
				if err := validateSchemaXMLAttribute(attribute); err != nil {
					if !candidate.considerError(err) {
						return err
					}
				}
			}
			continue
		}
		switch attribute.name.local {
		case "id":
			if !validNCName(collapseXMLWhitespace(attribute.value)) {
				return newSchemaCompositionDiagnostic(attribute.loc, "any id must be a valid NCName")
			}
		case "minOccurs", "maxOccurs":
			candidate.considerAt(attribute.loc, fmt.Sprintf("any attribute %q is not implemented", attribute.name.local))
		case "namespace", "notNamespace":
			if attribute.name.local == "namespace" {
				if err := validateWildcardNamespace(attribute); err != nil {
					return err
				}
				candidate.considerAt(attribute.loc, fmt.Sprintf("any attribute %q is not implemented", attribute.name.local))
				continue
			}
			if err := validateWildcardNotNamespace(attribute); err != nil {
				return err
			}
			if version == XSDVersion10 {
				candidate.considerErrorAt(newXSD11FeatureMismatch(
					FeatureSchemaSyntax,
					UnsupportedSchemaSyntaxCode,
					attribute.loc,
					"any notNamespace is an XSD 1.1-only construct",
				), element, attributeIndex)
			}
			if version != XSDVersion10 {
				candidate.considerAt(attribute.loc, fmt.Sprintf("any attribute %q is not implemented", attribute.name.local))
			}
		case "processContents":
			if err := validateSchemaEnum(attribute, "lax", "skip", "strict"); err != nil {
				return err
			}
			candidate.considerAt(attribute.loc, "wildcard particles are not implemented")
		case "notQName":
			if err := validateWildcardNotQName(element, attribute, true); err != nil {
				return err
			}
			if version == XSDVersion10 {
				candidate.considerErrorAt(newXSD11FeatureMismatch(
					FeatureSchemaSyntax,
					UnsupportedSchemaSyntaxCode,
					attribute.loc,
					"any notQName is an XSD 1.1-only construct",
				), element, attributeIndex)
			}
			if version != XSDVersion10 {
				candidate.considerAt(attribute.loc, "wildcard particles are not implemented")
			}
		default:
			return newSchemaCompositionDiagnostic(attribute.loc, fmt.Sprintf("any has forbidden attribute %q", attribute.name.local))
		}
	}
	if len(namespaceAttributes) > 0 && len(notNamespaceAttributes) > 0 {
		return newSchemaCompositionDiagnostic(namespaceAttributes[0].loc, "any wildcard namespace cannot combine with notNamespace")
	}
	for _, node := range element.children {
		textNode, ok := node.(syntaxText)
		if ok {
			if !xmlWhitespace([]byte(textNode.data)) {
				return newSchemaCompositionDiagnostic(textNode.loc, "any contains non-whitespace character data")
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(Loc{}, "any contains an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI || child.name.local != "annotation" {
			return newSchemaCompositionDiagnostic(child.loc, "any particle permits only an annotation child")
		}
		if annotationSeen {
			return newSchemaCompositionDiagnostic(child.loc, "any annotation must be first and unique")
		}
		annotationSeen = true
		if err := stageSchemaCandidateError(&candidate, validateSchemaAnnotationElement(child)); err != nil {
			return err
		}
	}
	return candidate.err()
}

//nolint:gocognit // Keep global group model grammar and candidate staging together.
func validateGroupGlobalChildren(parent *syntaxElement, children []*syntaxElement, version XSDVersion) error {
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
		case "all", "sequence":
			if modelSeen {
				return newSchemaCompositionDiagnostic(child.loc, "group requires exactly one model child")
			}
			modelSeen = true
			unsupportedErr := versionNamedModelGroupUnsupported(validateUnsupportedModelParticle(child, version), version)
			if unsupportedErr != nil && !candidate.considerError(unsupportedErr) {
				return unsupportedErr
			}
			if !candidate.present {
				candidate.considerAtVersion(child.loc, fmt.Sprintf("global %s child <%s> is not implemented", parent.name.local, child.name.local), version)
			}
		case "choice":
			if modelSeen {
				return newSchemaCompositionDiagnostic(child.loc, "group requires exactly one model child")
			}
			modelSeen = true
			if err := validateNamedModelGroupChoice(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
		default:
			if err := forbiddenGlobalSchemaChild(parent.name.local, child); err != nil {
				return err
			}
			candidate.considerAtVersion(child.loc, fmt.Sprintf("global %s child <%s> is not implemented", parent.name.local, child.name.local), version)
		}
	}
	if !modelSeen {
		return newSchemaCompositionDiagnostic(parent.loc, "group requires one all, choice, or sequence child")
	}
	return candidate.err()
}

//nolint:gocognit // Keep the narrow named-group choice grammar and support boundary together.
func validateNamedModelGroupChoice(element *syntaxElement, version XSDVersion) error {
	var candidate schemaChildUnsupportedCandidate
	if err := validateSchemaParticleOccurrences(element, version); err != nil {
		return err
	}
	for _, attribute := range element.attrs {
		if attribute.name.namespace == "" && (attribute.name.local == "minOccurs" || attribute.name.local == "maxOccurs") {
			return newSchemaCompositionDiagnostic(attribute.loc, "named model-group choice does not permit occurrence attributes")
		}
	}
	if err := validateSchemaParticleAttributesWithOccurrencePolicy(element, &candidate, version, true); err != nil {
		return err
	}
	children, collectedCandidate, err := collectGlobalSchemaChildren(element)
	if err != nil {
		return err
	}
	candidate.merge(versionNamedModelGroupCandidate(collectedCandidate, version))
	annotationSeen := false
	contentSeen := false
	for _, child := range children {
		handled, err := consumeGlobalSchemaAnnotation(child, &annotationSeen, &contentSeen)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		switch child.name.local {
		case "element":
			childCandidate, childErr := validateLocalElementParticle(child, version, true, "model-group choice", false)
			if childErr != nil {
				return childErr
			}
			if childCandidate.present {
				candidate.merge(versionNamedModelGroupCandidate(childCandidate, version))
				continue
			}
			if len(syntaxAttributesByLocal(child, "ref")) == 1 {
				continue
			}
			candidate.considerAtVersion(child.loc, "named model-group choice local element declarations are not implemented", version)
		case "group", "choice", "sequence", "any":
			unsupportedErr := versionNamedModelGroupUnsupported(validateUnsupportedParticle(child, version), version)
			if unsupportedErr != nil {
				if !candidate.considerError(unsupportedErr) {
					return unsupportedErr
				}
				continue
			}
			candidate.considerAtVersion(child.loc, fmt.Sprintf("named model-group choice child <%s> is not implemented", child.name.local), version)
		case "all":
			return newSchemaCompositionDiagnostic(child.loc, "choice cannot contain an all particle")
		default:
			if err := forbiddenGlobalSchemaChild("model-group choice", child); err != nil {
				return err
			}
			candidate.considerAtVersion(child.loc, fmt.Sprintf("named model-group choice child <%s> is not implemented", child.name.local), version)
		}
	}
	return candidate.err()
}

func versionNamedModelGroupUnsupported(err error, version XSDVersion) error {
	if err == nil || errors.Is(err, errLanguagePolicyMismatch) {
		return err
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		return err
	}
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return err
	}
	for _, reference := range feature.References() {
		if reference.Version() != string(version) {
			continue
		}
		diagnostic.specRef = reference.Source()
		break
	}
	return diagnostic
}

func versionNamedModelGroupCandidate(candidate schemaChildUnsupportedCandidate, version XSDVersion) schemaChildUnsupportedCandidate {
	if !candidate.present {
		return candidate
	}
	if candidate.captured != nil {
		candidate.captured = versionNamedModelGroupUnsupported(candidate.captured, version)
		return candidate
	}
	if candidate.version == "" {
		candidate.version = version
	}
	return candidate
}

//nolint:gocognit // Keep the attribute-group order and cardinality grammar explicit.
func validateAttributeGroupGlobalChildren(parent *syntaxElement, children []*syntaxElement, version XSDVersion) error {
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
			childErr := validateLocalAttribute(child, version)
			if child.name.local == "attributeGroup" {
				childErr = validateAttributeGroupReference(child)
			}
			if childErr != nil && !candidate.considerError(childErr) {
				return childErr
			}
		case "anyAttribute":
			if anyAttributeSeen {
				return newSchemaCompositionDiagnostic(child.loc, "attributeGroup anyAttribute must be unique")
			}
			anyAttributeSeen = true
			if err := validateAnyAttribute(child, version); err != nil && !candidate.considerError(err) {
				return err
			}
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
