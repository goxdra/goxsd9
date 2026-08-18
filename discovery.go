package goxsd9

import (
	"context"
	"fmt"
)

const (
	// SourceResolveCode identifies a resolver failure for a schema reference.
	SourceResolveCode = "XSD3006"
	// SourceInvalidCode identifies an invalid source returned by a resolver.
	SourceInvalidCode = "XSD3007"
	// MissingSchemaLocationCode identifies an include without a location.
	MissingSchemaLocationCode = "XSD3008"
)

type syntaxReference struct {
	kind           syntaxReferenceKind
	namespaceURN   string
	hasNamespace   bool
	schemaLocation string
	conditional    bool
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
	conditional  bool
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
	result, err := discoverSyntax(root, resolver)
	if err != nil {
		return nil, err
	}
	return result.documents, nil
}

func discoverSyntax(root ResolvedSource, resolver Resolver) (syntaxDiscoveryResult, error) {
	if err := validateDiscoverySource(root, Loc{}, FailureInternal); err != nil {
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

		document, err := decodeResolvedSyntax(item.source)
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
		conditional:  reference.conditional,
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
		if !ok || element.name.namespace != xsdNamespaceURI {
			continue
		}
		schemaLocation, hasLocation := syntaxAttributeValue(element, "schemaLocation")
		switch element.name.local {
		case "include":
			if !hasLocation {
				return nil, newDiagnostic(
					FailureInvalid,
					MissingSchemaLocationCode,
					element.loc,
					"schema include has no schemaLocation attribute",
					nil,
				)
			}
			references = append(references, syntaxReference{
				kind:           syntaxReferenceInclude,
				namespaceURN:   "",
				hasNamespace:   false,
				schemaLocation: schemaLocation,
				conditional:    syntaxReferenceIsConditional(element),
				loc:            element.loc,
			})
		case "import":
			namespaceURN, hasNamespace := syntaxAttributeValue(element, "namespace")
			references = append(references, syntaxReference{
				kind:           syntaxReferenceImport,
				namespaceURN:   namespaceURN,
				hasNamespace:   hasNamespace,
				schemaLocation: schemaLocation,
				conditional:    syntaxReferenceIsConditional(element),
				loc:            element.loc,
			})
		}
	}
	return references, nil
}

func syntaxReferenceIsConditional(element *syntaxElement) bool {
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdVersioningNamespaceURI {
			return true
		}
	}
	return false
}

func syntaxAttributeValue(element *syntaxElement, local string) (string, bool) {
	for _, attribute := range element.attrs {
		if attribute.name.namespace == "" && attribute.name.local == local {
			return attribute.value, true
		}
	}
	return "", false
}
