package goxsd9

import (
	"errors"
	"fmt"
)

// ComponentKind identifies the kind of schema component represented by a
// Component.
type ComponentKind string

const (
	// ComponentKindElementDeclaration identifies an element declaration.
	ComponentKindElementDeclaration ComponentKind = "element-declaration"
	// ComponentKindAttributeDeclaration identifies an attribute declaration.
	ComponentKindAttributeDeclaration ComponentKind = "attribute-declaration"
	// ComponentKindSimpleTypeDefinition identifies a simple type definition.
	ComponentKindSimpleTypeDefinition ComponentKind = "simple-type-definition"
	// ComponentKindComplexTypeDefinition identifies a complex type definition.
	ComponentKindComplexTypeDefinition ComponentKind = "complex-type-definition"
	// ComponentKindModelGroupDefinition identifies a model group definition.
	ComponentKindModelGroupDefinition ComponentKind = "model-group-definition"
	// ComponentKindAttributeGroupDefinition identifies an attribute group
	// definition.
	ComponentKindAttributeGroupDefinition ComponentKind = "attribute-group-definition"
	// ComponentKindNotationDeclaration identifies a notation declaration.
	ComponentKindNotationDeclaration ComponentKind = "notation-declaration"
)

// QName is an expanded XML qualified name. It contains a namespace URI and a
// local name, never a lexical prefix.
type QName struct {
	namespace string
	local     string
}

// NewQName constructs an expanded name. Prefix resolution belongs to the
// syntax and parsing phases, so this constructor accepts an already expanded
// namespace URI.
func NewQName(namespaceURI, local string) (QName, error) {
	if local == "" {
		return QName{}, errors.New("qualified name has an empty local name")
	}
	return QName{namespace: namespaceURI, local: local}, nil
}

// Namespace returns the namespace URI, or an empty string for no namespace.
func (name QName) Namespace() string {
	return name.namespace
}

// Local returns the local part of the name.
func (name QName) Local() string {
	return name.local
}

// IsZero reports whether the name has no local name.
func (name QName) IsZero() bool {
	return name == QName{}
}

// String formats the name as {namespace}local, or local when no namespace is
// present.
func (name QName) String() string {
	if name.namespace == "" {
		return name.local
	}
	return "{" + name.namespace + "}" + name.local
}

// ComponentID is the stable identity of a schema component within a schema
// graph. The ordinal is one-based in the lexical declaration order of its
// source document.
type ComponentID struct {
	source  SourceID
	ordinal uint64
}

// Source returns the identity of the document that declares the component.
func (id ComponentID) Source() SourceID {
	return id.source
}

// Ordinal returns the one-based declaration ordinal within the source
// document, or zero for the zero ID.
func (id ComponentID) Ordinal() uint64 {
	return id.ordinal
}

// IsZero reports whether the ID does not identify a component.
func (id ComponentID) IsZero() bool {
	return id == ComponentID{}
}

// Component is an immutable schema component identity and its fundamental
// source facts. Derived validator and code-generator state is not stored here.
type Component struct {
	id   ComponentID
	kind ComponentKind
	name QName
	loc  Loc
}

// ID returns the stable identity of the component.
func (component Component) ID() ComponentID {
	return component.id
}

// Kind returns the component kind.
func (component Component) Kind() ComponentKind {
	return component.kind
}

// Name returns the expanded name of the component.
func (component Component) Name() QName {
	return component.name
}

// Loc returns the declaration location of the component.
func (component Component) Loc() Loc {
	return component.loc
}

// Document returns the source identity of the declaring document.
func (component Component) Document() SourceID {
	return component.id.Source()
}

// SchemaDocument is an immutable document in a Schema's discovery order.
type SchemaDocument struct {
	source          SourceID
	targetNamespace string
	storage         *schemaStorage
	start           int
	count           int
}

// Source returns the resolver-provided identity of the document.
func (document SchemaDocument) Source() SourceID {
	return document.source
}

// TargetNamespace returns the document's target namespace URI.
func (document SchemaDocument) TargetNamespace() string {
	return document.targetNamespace
}

// Components returns the document's components in lexical declaration order.
// The returned slice is independent of the completed schema.
func (document SchemaDocument) Components() []Component {
	if document.storage == nil || document.count == 0 {
		return nil
	}
	return append([]Component(nil), document.storage.components[document.start:document.start+document.count]...)
}

// Schema is an immutable schema component graph. Documents are stored in
// discovery order and components in each document are stored in lexical
// declaration order.
type Schema struct {
	documents []SchemaDocument
	storage   *schemaStorage
}

type schemaStorage struct {
	components []Component
	byID       map[ComponentID]int
	byName     map[QName][]int
}

// Documents returns the schema documents in discovery order. The returned
// slice and every document's component collection are independent copies.
func (schema Schema) Documents() []SchemaDocument {
	documents := make([]SchemaDocument, 0, len(schema.documents))
	for _, document := range schema.documents {
		documents = append(documents, SchemaDocument{
			source:          document.source,
			targetNamespace: document.targetNamespace,
			storage:         document.storage,
			start:           document.start,
			count:           document.count,
		})
	}
	return documents
}

// Components returns all schema components in document-discovery order,
// followed by lexical declaration order within each document. The returned
// slice is independent of the completed schema.
func (schema Schema) Components() []Component {
	if schema.storage == nil {
		return nil
	}
	return append([]Component(nil), schema.storage.components...)
}

// Lookup returns the component with id, if it belongs to the schema.
func (schema Schema) Lookup(id ComponentID) (Component, bool) {
	if schema.storage == nil {
		return Component{}, false
	}
	index, ok := schema.storage.byID[id]
	if !ok {
		return Component{}, false
	}
	return schema.storage.components[index], true
}

// Find returns named components with name in deterministic schema walk order.
// A copy of the result is returned, so changing the slice cannot change the
// completed schema.
func (schema Schema) Find(name QName) []Component {
	if schema.storage == nil {
		return nil
	}
	indices := schema.storage.byName[name]
	return schema.componentsAt(indices)
}

// FindKind returns components with both kind and name in deterministic schema
// walk order.
func (schema Schema) FindKind(kind ComponentKind, name QName) []Component {
	if schema.storage == nil {
		return nil
	}
	indices := schema.storage.byName[name]
	if len(indices) == 0 {
		return nil
	}
	components := make([]Component, 0, len(indices))
	for _, index := range indices {
		component := schema.storage.components[index]
		if component.kind != kind {
			continue
		}
		components = append(components, component)
	}
	return components
}

// Walk visits components in deterministic schema walk order. A nil visitor
// is rejected, and the first visitor error stops the walk and is preserved.
func (schema Schema) Walk(visitor func(Component) error) error {
	if visitor == nil {
		return errors.New("schema walk visitor is nil")
	}
	if schema.storage == nil {
		return nil
	}
	for _, component := range schema.storage.components {
		if err := visitor(component); err != nil {
			return fmt.Errorf("walk schema component %s: %w", component.ID().Source(), err)
		}
	}
	return nil
}

func (schema Schema) componentsAt(indices []int) []Component {
	if len(indices) == 0 {
		return nil
	}
	components := make([]Component, 0, len(indices))
	for _, index := range indices {
		components = append(components, schema.storage.components[index])
	}
	return components
}

type schemaDocumentInput struct {
	source          SourceID
	targetNamespace string
	// declarations contains the named schema-level declarations in lexical
	// order. Local particle components will use a separate scoped model.
	declarations []schemaComponentInput
}

type schemaComponentInput struct {
	kind ComponentKind
	name QName
	loc  Loc
}

// newSchema completes the ordered component representation after discovery
// and declaration phases have supplied all document identities and facts.
// Input slices are consumed only for construction; the returned schema owns
// its ordered storage.
func newSchema(inputs []schemaDocumentInput) (Schema, error) {
	documents := make([]SchemaDocument, 0, len(inputs))
	components := make([]Component, 0)
	byID := make(map[ComponentID]int)
	byName := make(map[QName][]int)
	seenSources := make(map[SourceID]struct{}, len(inputs))

	for _, input := range inputs {
		if input.source == "" {
			return Schema{}, newDiagnostic(
				FailureInternal,
				"GOXSD9012",
				Loc{},
				"schema document has an empty source identity",
				nil,
			)
		}
		if _, exists := seenSources[input.source]; exists {
			return Schema{}, newDiagnostic(
				FailureInternal,
				"GOXSD9013",
				Loc{},
				fmt.Sprintf("schema document source %q is repeated", input.source),
				nil,
			)
		}
		seenSources[input.source] = struct{}{}

		documentStart := len(components)
		for declarationIndex, declaration := range input.declarations {
			if declaration.kind == "" {
				return Schema{}, newDiagnostic(
					FailureInternal,
					"GOXSD9014",
					declaration.loc,
					"schema component has an empty kind",
					nil,
				)
			}
			if declaration.name.IsZero() {
				return Schema{}, newDiagnostic(
					FailureInternal,
					"GOXSD9015",
					declaration.loc,
					"schema component has an empty name",
					nil,
				)
			}

			id := ComponentID{
				source:  input.source,
				ordinal: uint64(declarationIndex + 1),
			}
			component := Component{
				id:   id,
				kind: declaration.kind,
				name: declaration.name,
				loc:  declaration.loc,
			}
			byID[id] = len(components)
			byName[component.name] = append(byName[component.name], len(components))
			components = append(components, component)
		}

		documents = append(documents, SchemaDocument{
			source:          input.source,
			targetNamespace: input.targetNamespace,
			start:           documentStart,
			count:           len(components) - documentStart,
		})
	}
	storage := &schemaStorage{
		components: components,
		byID:       byID,
		byName:     byName,
	}
	for index := range documents {
		documents[index].storage = storage
	}

	return Schema{
		documents: documents,
		storage:   storage,
	}, nil
}
