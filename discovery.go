package goxsd9

import (
	"context"
	"fmt"
)

const (
	// SourceResolveCode identifies a resolver failure for a schema reference.
	SourceResolveCode = "XSD3006"
	// SourceInvalidCode identifies an invalid resolved schema source.
	SourceInvalidCode = "XSD3007"
	// MissingSchemaLocationCode identifies an include without a location.
	MissingSchemaLocationCode = "XSD3008"
)

type syntaxReference struct {
	kind           syntaxReferenceKind
	namespaceURN   string
	hasNamespace   bool
	schemaLocation string
	loc            Loc
}

type syntaxReferenceKind uint8

const (
	syntaxReferenceInclude syntaxReferenceKind = iota + 1
	syntaxReferenceImport
)

type syntaxDocumentEdge struct {
	source       SourceID
	target       SourceID
	kind         syntaxReferenceKind
	namespaceURN string
	hasNamespace bool
	loc          Loc
}

type syntaxDiscoveryResult struct {
	documents []*syntaxDocument
	edges     []syntaxDocumentEdge
}

type syntaxDiscoveryQueueItem struct {
	source ResolvedSource
	loc    Loc
}

type syntaxDiscovery struct {
	resolver  Resolver
	seen      map[SourceID]struct{}
	queue     []syntaxDiscoveryQueueItem
	documents []*syntaxDocument
	edges     []syntaxDocumentEdge
}

// discoverSyntaxDocuments resolves the direct include and import references
// of each document in FIFO discovery order. Resolver policy remains outside
// this package: locations are passed through unchanged and only resolver
// identities are used for cycle detection.
func discoverSyntaxDocuments(root ResolvedSource, resolver Resolver) ([]*syntaxDocument, error) {
	result, err := discoverSyntaxWithPolicy(root, resolver, Compatibility)
	if err != nil {
		return nil, err
	}
	return result.documents, nil
}

func discoverSyntax(root ResolvedSource, resolver Resolver) (syntaxDiscoveryResult, error) {
	return discoverSyntaxWithPolicy(root, resolver, Compatibility)
}

//nolint:gocognit // FIFO discovery keeps resolution, filtering, and closure in one phase.
func discoverSyntaxWithPolicy(root ResolvedSource, resolver Resolver, policy LanguagePolicy) (syntaxDiscoveryResult, error) {
	version, policyErr := xsdVersionForLanguagePolicy(policy)
	if policyErr != nil {
		return syntaxDiscoveryResult{}, closeDiscoverySourceOnError(
			root,
			Loc{},
			invalidLanguagePolicyDiagnostic(policy, policyErr),
		)
	}
	if err := validateDiscoverySource(root, Loc{}, FailureInvalid); err != nil {
		return syntaxDiscoveryResult{}, closeDiscoverySourceOnError(root, Loc{}, err)
	}

	discovery := syntaxDiscovery{
		resolver:  resolver,
		seen:      map[SourceID]struct{}{root.SourceID(): {}},
		queue:     []syntaxDiscoveryQueueItem{{source: root}},
		documents: make([]*syntaxDocument, 0, 1),
		edges:     make([]syntaxDocumentEdge, 0),
	}
	for len(discovery.queue) > 0 {
		item := discovery.queue[0]
		discovery.queue = discovery.queue[1:]

		document, err := decodeResolvedSyntaxForDiscovery(item.source)
		if err != nil {
			return syntaxDiscoveryResult{}, discovery.finish(err)
		}
		err = applySchemaConditionalsWithPolicy(document, policy, version)
		if err != nil {
			return syntaxDiscoveryResult{}, discovery.finish(err)
		}
		err = validateSyntaxDocumentStructureWithPolicy(document, version)
		if err != nil {
			return syntaxDiscoveryResult{}, discovery.finish(err)
		}
		discovery.documents = append(discovery.documents, document)

		references, err := syntaxDocumentReferences(document)
		if err != nil {
			return syntaxDiscoveryResult{}, discovery.finish(err)
		}
		for _, reference := range references {
			if err := discovery.enqueue(item.source.Context(), item.source.SourceID(), reference); err != nil {
				return syntaxDiscoveryResult{}, discovery.finish(err)
			}
		}
	}
	return syntaxDiscoveryResult{
		documents: discovery.documents,
		edges:     discovery.edges,
	}, nil
}

func (discovery *syntaxDiscovery) enqueue(ctx context.Context, sourceID SourceID, reference syntaxReference) error {
	if discovery.resolver == nil {
		return newDiagnostic(
			FailureResolution,
			SourceResolveCode,
			reference.loc,
			"schema resolver is nil",
			nil,
		)
	}

	source, err := discovery.resolver.Resolve(ctx, reference.namespaceURN, reference.schemaLocation)
	if err != nil {
		resolution := newDiagnostic(
			FailureResolution,
			SourceResolveCode,
			reference.loc,
			fmt.Sprintf("failed to resolve schema location %q", reference.schemaLocation),
			err,
		)
		return closeDiscoverySourceOnError(source, reference.loc, resolution)
	}
	if err := validateDiscoverySource(source, reference.loc, FailureResolution); err != nil {
		return closeDiscoverySourceOnError(source, reference.loc, err)
	}
	discovery.edges = append(discovery.edges, syntaxDocumentEdge{
		source:       sourceID,
		target:       source.SourceID(),
		kind:         reference.kind,
		namespaceURN: reference.namespaceURN,
		hasNamespace: reference.hasNamespace,
		loc:          reference.loc,
	})
	if _, ok := discovery.seen[source.SourceID()]; ok {
		return closeDiscoverySource(source, reference.loc)
	}

	// Intern before the source can be decoded so cycles cannot re-enqueue it.
	discovery.seen[source.SourceID()] = struct{}{}
	discovery.queue = append(discovery.queue, syntaxDiscoveryQueueItem{
		source: source,
		loc:    reference.loc,
	})
	return nil
}

func (discovery *syntaxDiscovery) finish(primary error) error {
	for _, item := range discovery.queue {
		closeErr := closeDiscoverySource(item.source, item.loc)
		if closeErr == nil {
			continue
		}
		primary = combineDiscoveryError(primary, closeErr)
	}
	discovery.queue = nil
	return primary
}

func validateDiscoverySource(source ResolvedSource, loc Loc, class FailureClass) error {
	if source.Context() == nil {
		return newDiagnostic(class, SourceInvalidCode, loc, "resolved source context is nil", nil)
	}
	if source.SourceID() == "" {
		return newDiagnostic(class, SourceInvalidCode, loc, "resolved source ID is empty", nil)
	}
	if source.reader == nil {
		return newDiagnostic(class, SourceInvalidCode, loc, "resolved source reader is nil", nil)
	}
	return nil
}

func closeDiscoverySourceOnError(source ResolvedSource, loc Loc, primary error) error {
	closeErr := closeDiscoverySource(source, loc)
	if closeErr == nil {
		return primary
	}
	return combineDiscoveryError(primary, closeErr)
}

func combineDiscoveryError(primary, additional error) error {
	for _, diagnostic := range syntaxDiagnostics(additional) {
		primary = combineSyntaxErrors(primary, diagnostic)
	}
	return primary
}

func closeDiscoverySource(source ResolvedSource, loc Loc) error {
	if source.reader == nil {
		return nil
	}
	if err := source.reader.Close(); err != nil {
		return newDiagnostic(
			FailureResolution,
			SourceCloseCode,
			loc,
			"failed to close schema source",
			err,
		)
	}
	return nil
}

func syntaxDocumentReferences(document *syntaxDocument) ([]syntaxReference, error) {
	if document == nil || document.root == nil {
		return nil, newDiagnostic(
			FailureInternal,
			diagnosticSyntaxDocumentNoRootCode,
			Loc{},
			"syntax document has no root element",
			nil,
		)
	}

	references := make([]syntaxReference, 0)
	for _, node := range document.root.children {
		element, ok := node.(*syntaxElement)
		if !ok {
			continue
		}
		reference, present, err := syntaxReferenceFromElement(element)
		if err != nil {
			return nil, err
		}
		if present {
			references = append(references, reference)
		}
	}
	return references, nil
}

func syntaxReferenceFromElement(element *syntaxElement) (syntaxReference, bool, error) {
	if element.name.namespace != xsdNamespaceURI {
		return syntaxReference{}, false, nil
	}
	schemaLocation, hasLocation := syntaxAttributeValue(element, "schemaLocation")
	switch element.name.local {
	case "include":
		if err := validateSchemaReferenceElement(element, syntaxReferenceInclude); err != nil {
			return syntaxReference{}, false, err
		}
		if !hasLocation {
			return syntaxReference{}, false, newDiagnostic(
				FailureInvalid,
				MissingSchemaLocationCode,
				element.loc,
				"schema include has no schemaLocation attribute",
				nil,
			)
		}
		return syntaxReference{
			kind:           syntaxReferenceInclude,
			schemaLocation: schemaLocation,
			loc:            element.loc,
		}, true, nil
	case "import":
		if err := validateSchemaReferenceElement(element, syntaxReferenceImport); err != nil {
			return syntaxReference{}, false, err
		}
		namespaceURN, hasNamespace := syntaxAttributeValue(element, "namespace")
		if hasNamespace {
			namespaceURN = collapseXMLWhitespace(namespaceURN)
		}
		return syntaxReference{
			kind:           syntaxReferenceImport,
			namespaceURN:   namespaceURN,
			hasNamespace:   hasNamespace,
			schemaLocation: schemaLocation,
			loc:            element.loc,
		}, true, nil
	default:
		return syntaxReference{}, false, nil
	}
}

func validateSchemaReferenceElement(element *syntaxElement, kind syntaxReferenceKind) error {
	for _, attribute := range element.attrs {
		if err := validateSchemaReferenceAttribute(attribute, kind); err != nil {
			return err
		}
	}
	return validateSchemaReferenceChildren(element)
}

func validateSchemaReferenceAttribute(attribute syntaxAttribute, kind syntaxReferenceKind) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		// The versioning namespace is extensible. Conditional inclusion has
		// already validated the six defined attributes; unknown names are
		// permitted and ignored.
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(
				attribute.loc,
				fmt.Sprintf("schema composition has forbidden XSD attribute %q", attribute.name.local),
			)
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return validateSchemaXMLAttribute(attribute)
		}
		return nil
	}
	if attribute.name.local == "id" {
		if !validNCName(collapseXMLWhitespace(attribute.value)) {
			return newSchemaCompositionDiagnostic(attribute.loc, "schema composition id must be a valid NCName")
		}
		return nil
	}
	if attribute.name.local == "schemaLocation" {
		return validateSchemaAnyURI(attribute)
	}
	if attribute.name.local == "namespace" && kind == syntaxReferenceImport {
		return validateSchemaAnyURI(attribute)
	}
	return newSchemaCompositionDiagnostic(
		attribute.loc,
		fmt.Sprintf("schema %s has forbidden unqualified attribute %q", schemaReferenceName(kind), attribute.name.local),
	)
}

func schemaReferenceName(kind syntaxReferenceKind) string {
	switch kind {
	case syntaxReferenceInclude:
		return "include"
	case syntaxReferenceImport:
		return "import"
	default:
		return "composition reference"
	}
}

func validateSchemaReferenceChildren(element *syntaxElement) error {
	annotationSeen := false
	for _, node := range element.children {
		seen, err := validateSchemaReferenceChild(node, annotationSeen)
		if err != nil {
			return err
		}
		annotationSeen = seen
	}
	return nil
}

func validateSchemaReferenceChild(node syntaxNode, annotationSeen bool) (bool, error) {
	textNode, ok := node.(syntaxText)
	if ok {
		if !xmlWhitespace([]byte(textNode.data)) {
			return false, newSchemaCompositionDiagnostic(textNode.loc, "schema composition contains non-whitespace character data")
		}
		return annotationSeen, nil
	}
	child, ok := node.(*syntaxElement)
	if !ok {
		return false, newSchemaBridgeInvariant(Loc{}, "schema composition contains an unknown syntax node")
	}
	if child.name.namespace != xsdNamespaceURI {
		return false, newSchemaCompositionDiagnostic(child.loc, "schema composition contains a forbidden non-XSD child")
	}
	if child.name.local != "annotation" {
		return false, newSchemaCompositionDiagnostic(
			child.loc,
			fmt.Sprintf("schema composition contains forbidden nested XSD construct <%s>", child.name.local),
		)
	}
	if annotationSeen {
		return false, newSchemaCompositionDiagnostic(child.loc, "schema composition contains more than one annotation")
	}
	if err := validateSchemaAnnotationElement(child); err != nil {
		return false, err
	}
	return true, nil
}

func validateSchemaAnnotationElement(element *syntaxElement) error {
	if err := validateSchemaAnnotationAttributes(element); err != nil {
		return err
	}
	for _, node := range element.children {
		if err := validateSchemaAnnotationChild(node); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaAnnotationChild(node syntaxNode) error {
	textNode, ok := node.(syntaxText)
	if ok {
		if !xmlWhitespace([]byte(textNode.data)) {
			return newSchemaCompositionDiagnostic(textNode.loc, "schema annotation contains non-whitespace character data")
		}
		return nil
	}
	child, ok := node.(*syntaxElement)
	if !ok {
		return newSchemaBridgeInvariant(Loc{}, "schema annotation contains an unknown syntax node")
	}
	if child.name.namespace != xsdNamespaceURI {
		return newSchemaCompositionDiagnostic(child.loc, "schema annotation contains a forbidden non-XSD child")
	}
	switch child.name.local {
	case "appinfo", "documentation":
		return validateSchemaAnnotationItem(child)
	default:
		return newSchemaCompositionDiagnostic(
			child.loc,
			fmt.Sprintf("schema annotation contains forbidden XSD construct <%s>", child.name.local),
		)
	}
}

func validateSchemaAnnotationItem(element *syntaxElement) error {
	if err := validateSchemaAnnotationAttributes(element); err != nil {
		return err
	}
	// appinfo and documentation are mixed content. Their descendants are
	// application payload and are intentionally not interpreted as schema
	// grammar.
	return nil
}

func validateSchemaAnnotationAttributes(element *syntaxElement) error {
	for _, attribute := range element.attrs {
		if err := validateSchemaAnnotationAttribute(element.name.local, attribute); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaAnnotationAttribute(element string, attribute syntaxAttribute) error {
	if attribute.name.namespace == xsdVersioningNamespaceURI {
		return nil
	}
	if attribute.name.namespace != "" {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaCompositionDiagnostic(
				attribute.loc,
				fmt.Sprintf("schema annotation has forbidden XSD attribute %q", attribute.name.local),
			)
		}
		if attribute.name.namespace == xmlNamespaceURI {
			return validateSchemaXMLAttribute(attribute)
		}
		return nil
	}
	if !annotationAttributeAllowed(element, attribute.name.local) {
		return newSchemaCompositionDiagnostic(
			attribute.loc,
			fmt.Sprintf("schema %s has forbidden unqualified attribute %q", element, attribute.name.local),
		)
	}
	if attribute.name.local == "source" {
		return validateSchemaAnyURI(attribute)
	}
	if attribute.name.local == "id" && !validNCName(collapseXMLWhitespace(attribute.value)) {
		return newSchemaCompositionDiagnostic(attribute.loc, "schema annotation id must be a valid NCName")
	}
	return nil
}

func annotationAttributeAllowed(element, attribute string) bool {
	switch element {
	case "annotation":
		return attribute == "id"
	case "appinfo", "documentation":
		return attribute == "source"
	default:
		return false
	}
}

func syntaxAttributeValue(element *syntaxElement, local string) (string, bool) {
	for _, attribute := range element.attrs {
		if attribute.name.namespace == "" && attribute.name.local == local {
			return attribute.value, true
		}
	}
	return "", false
}
