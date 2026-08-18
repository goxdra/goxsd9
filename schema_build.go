package goxsd9

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	invalidSchemaTargetNamespaceCode    = "XSD3009"
	invalidSchemaCompositionCode        = "XSD3010"
	invalidSchemaDeclarationNameCode    = "XSD3011"
	diagnosticSchemaBridgeInvariantCode = "GOXSD9025"
)

type schemaTargetNamespace struct {
	value   string
	present bool
}

// discoverSchema completes the internal discovery-to-schema pipeline without
// exposing a public parser entrypoint.
func discoverSchema(root ResolvedSource, resolver Resolver) (Schema, error) {
	discovery, err := discoverSyntax(root, resolver)
	if err != nil {
		return Schema{}, err
	}
	return newSchemaFromDiscovery(discovery)
}

func newSchemaFromDiscovery(discovery syntaxDiscoveryResult) (Schema, error) {
	namespaces := make([]schemaTargetNamespace, len(discovery.documents))
	sourceIndices := make(map[SourceID]int, len(discovery.documents))
	for index, document := range discovery.documents {
		if document == nil || document.root == nil {
			return Schema{}, newDiagnostic(
				FailureInternal,
				diagnosticSyntaxDocumentNoRootCode,
				Loc{},
				"schema discovery result contains a document without a root",
				nil,
			)
		}
		if document.source == "" {
			return Schema{}, newDiagnostic(
				FailureInternal,
				diagnosticSchemaEmptySourceCode,
				document.root.loc,
				"schema discovery result contains an empty source identity",
				nil,
			)
		}
		if _, exists := sourceIndices[document.source]; exists {
			return Schema{}, newDiagnostic(
				FailureInternal,
				diagnosticSchemaRepeatedSourceCode,
				document.root.loc,
				fmt.Sprintf("schema discovery result repeats source %q", document.source),
				nil,
			)
		}

		namespace, err := syntaxDocumentTargetNamespace(document)
		if err != nil {
			return Schema{}, err
		}
		sourceIndices[document.source] = index
		namespaces[index] = namespace
	}

	if err := validateSchemaComposition(discovery.edges, sourceIndices, namespaces); err != nil {
		return Schema{}, err
	}

	inputs := make([]schemaDocumentInput, 0, len(discovery.documents))
	for index, document := range discovery.documents {
		declarations, err := schemaDocumentDeclarations(document, namespaces[index].value)
		if err != nil {
			return Schema{}, err
		}
		inputs = append(inputs, schemaDocumentInput{
			source:          document.source,
			targetNamespace: namespaces[index].value,
			declarations:    declarations,
		})
	}

	schema, err := newSchema(inputs)
	if err != nil {
		return Schema{}, err
	}
	return schema, nil
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
	value := collapseSchemaWhitespace(attribute.value)
	if value == "" {
		return schemaTargetNamespace{}, newDiagnostic(
			FailureInvalid,
			invalidSchemaTargetNamespaceCode,
			document.root.loc,
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
	if edge.conditional {
		return newSchemaSyntaxUnsupported(edge.loc, "conditional schema composition is not implemented")
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

func schemaDocumentDeclarations(document *syntaxDocument, targetNamespace string) ([]schemaComponentInput, error) {
	declarations := make([]schemaComponentInput, 0)
	for _, node := range document.root.children {
		declaration, present, err := schemaDocumentDeclaration(node, targetNamespace)
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

func schemaDocumentDeclaration(node syntaxNode, targetNamespace string) (schemaComponentInput, bool, error) {
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
	if err := rejectNestedSchemaConstructs(element); err != nil {
		return schemaComponentInput{}, false, err
	}
	return schemaComponentInput{
		kind: kind,
		name: name,
		loc:  element.loc,
	}, true, nil
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

func rejectNestedSchemaConstructs(element *syntaxElement) error {
	for _, node := range element.children {
		nested, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		if nested.name.namespace != xsdNamespaceURI {
			return newSchemaSyntaxUnsupported(nested.loc, "nested schema syntax is not implemented")
		}
		if nested.name.local == "annotation" {
			if err := validateSchemaAnnotationElement(nested); err != nil {
				return err
			}
			continue
		}
		if nestedSchemaConstruct(nested.name.local) {
			return newSchemaSyntaxUnsupported(
				nested.loc,
				fmt.Sprintf("nested XSD construct <%s> is not implemented", nested.name.local),
			)
		}
		if err := rejectNestedSchemaConstructs(nested); err != nil {
			return err
		}
	}
	return nil
}

func nestedSchemaConstruct(local string) bool {
	switch local {
	case "all", "attribute", "attributeGroup", "choice", "complexType", "element", "group", "simpleType", "sequence":
		return true
	default:
		return false
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
	value := collapseSchemaWhitespace(attributes[0].value)
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
		if attribute.name.local == local {
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

func collapseSchemaWhitespace(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	pendingSpace := false
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case ' ', '\t', '\r', '\n':
			if result.Len() > 0 {
				pendingSpace = true
			}
			continue
		}
		if pendingSpace {
			result.WriteByte(' ')
			pendingSpace = false
		}
		result.WriteByte(value[index])
	}
	return result.String()
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
