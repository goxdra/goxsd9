package goxsd9

import (
	"errors"
	"fmt"
	"strconv"
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

// SimpleTypeID is the stable identity of a simple-type model node. Named
// nodes also have a ComponentID; anonymous nodes use this identity because
// they are not schema components.
type SimpleTypeID struct {
	source  SourceID
	ordinal uint64
}

// Source returns the identity of the document containing the model node.
func (id SimpleTypeID) Source() SourceID {
	return id.source
}

// Ordinal returns the one-based model-node ordinal within the source
// document, or zero for the zero ID.
func (id SimpleTypeID) Ordinal() uint64 {
	return id.ordinal
}

// IsZero reports whether the ID does not identify a model node.
func (id SimpleTypeID) IsZero() bool {
	return id == SimpleTypeID{}
}

// SimpleTypeVariety identifies the model of a simple type.
type SimpleTypeVariety string

const (
	// SimpleTypeVarietyAtomicRestriction identifies an atomic restriction.
	SimpleTypeVarietyAtomicRestriction SimpleTypeVariety = "atomic-restriction"
	// SimpleTypeVarietyList identifies a list variety.
	SimpleTypeVarietyList SimpleTypeVariety = "list"
	// SimpleTypeVarietyUnion identifies a union variety.
	SimpleTypeVarietyUnion SimpleTypeVariety = "union"

	// SimpleTypeVarietyAtomic is a concise alias for an atomic restriction.
	SimpleTypeVarietyAtomic = SimpleTypeVarietyAtomicRestriction
	// SimpleTypeVarietyRestriction is a compatibility alias for an atomic
	// restriction.
	SimpleTypeVarietyRestriction = SimpleTypeVarietyAtomicRestriction
)

// SimpleTypeReferenceKind identifies how a simple-type reference is resolved.
type SimpleTypeReferenceKind string

const (
	// SimpleTypeReferenceBuiltin identifies an XSD built-in datatype.
	SimpleTypeReferenceBuiltin SimpleTypeReferenceKind = "builtin"
	// SimpleTypeReferenceNamed identifies a named schema simple type.
	SimpleTypeReferenceNamed SimpleTypeReferenceKind = "named"
	// SimpleTypeReferenceAnonymous identifies an inline simple type.
	SimpleTypeReferenceAnonymous SimpleTypeReferenceKind = "anonymous"
)

// SimpleTypeReference is an immutable resolved simple-type reference. Named
// references retain only their QName and component identity; anonymous
// references expose their owned model node through AnonymousType.
type SimpleTypeReference struct {
	facts *schemaSimpleTypeReferenceComponent
}

// Kind returns the resolved reference kind.
func (reference SimpleTypeReference) Kind() SimpleTypeReferenceKind {
	if reference.facts == nil {
		return ""
	}
	return reference.facts.kind
}

// Name returns the expanded QName of a built-in or named reference. Anonymous
// references have no QName and return the zero QName.
func (reference SimpleTypeReference) Name() QName {
	if reference.facts == nil {
		return QName{}
	}
	return reference.facts.name
}

// QName returns the expanded QName of a built-in or named reference.
func (reference SimpleTypeReference) QName() QName {
	return reference.Name()
}

// Loc returns the source location of the reference expression.
func (reference SimpleTypeReference) Loc() Loc {
	if reference.facts == nil {
		return Loc{}
	}
	return reference.facts.loc
}

// Variety returns the resolved variety of the referenced simple type.
func (reference SimpleTypeReference) Variety() SimpleTypeVariety {
	if reference.facts == nil {
		return ""
	}
	return reference.facts.variety
}

// VarietyLoc returns the location of the referenced type's model child.
func (reference SimpleTypeReference) VarietyLoc() Loc {
	if reference.facts == nil {
		return Loc{}
	}
	return reference.facts.varietyLoc
}

// ComponentID returns the schema component identity of a named reference.
// Built-ins and anonymous references do not have component identities.
func (reference SimpleTypeReference) ComponentID() (ComponentID, bool) {
	if reference.facts == nil || !reference.facts.hasID {
		return ComponentID{}, false
	}
	return reference.facts.id, true
}

// ID is an alias for ComponentID.
func (reference SimpleTypeReference) ID() (ComponentID, bool) {
	return reference.ComponentID()
}

// AnonymousID returns the model-node identity of an anonymous reference.
func (reference SimpleTypeReference) AnonymousID() (SimpleTypeID, bool) {
	if reference.facts == nil || !reference.facts.hasAnonymousID {
		return SimpleTypeID{}, false
	}
	return reference.facts.anonymousID, true
}

// AnonymousType returns the immutable inline model node referenced by an
// anonymous reference.
func (reference SimpleTypeReference) AnonymousType() (SimpleTypeDefinition, bool) {
	if reference.facts == nil || reference.facts.kind != SimpleTypeReferenceAnonymous || reference.facts.anonymous == nil {
		return SimpleTypeDefinition{}, false
	}
	return SimpleTypeDefinition{facts: reference.facts.anonymous}, true
}

// IsBuiltin reports whether the reference names an XSD built-in datatype.
func (reference SimpleTypeReference) IsBuiltin() bool {
	return reference.Kind() == SimpleTypeReferenceBuiltin
}

// IsNamed reports whether the reference names a schema component.
func (reference SimpleTypeReference) IsNamed() bool {
	return reference.Kind() == SimpleTypeReferenceNamed
}

// IsAnonymous reports whether the reference points to an inline model node.
func (reference SimpleTypeReference) IsAnonymous() bool {
	return reference.Kind() == SimpleTypeReferenceAnonymous
}

// Component is an immutable schema component identity and its fundamental
// source facts. Derived validator and code-generator state is not stored here.
type Component struct {
	id          ComponentID
	kind        ComponentKind
	name        QName
	loc         Loc
	element     *schemaElementComponent
	notation    *schemaNotationComponent
	simpleType  *schemaSimpleTypeComponent
	complexType *schemaComplexTypeComponent
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

// Element returns the immutable element-declaration view for a supported
// global element with a resolved type.
func (component Component) Element() (ElementDeclaration, bool) {
	if component.element == nil {
		return ElementDeclaration{}, false
	}
	return ElementDeclaration{
		component: component,
		facts:     component.element,
	}, true
}

// ElementDeclaration returns the immutable element-declaration view for a
// supported global element with a resolved type.
func (component Component) ElementDeclaration() (ElementDeclaration, bool) {
	return component.Element()
}

// ElementDeclaration is the immutable type-specific view of a supported
// global element declaration with a resolved type.
type ElementDeclaration struct {
	component Component
	facts     *schemaElementComponent
}

// Component returns the generic component represented by the view.
func (declaration ElementDeclaration) Component() Component {
	return declaration.component
}

// ID returns the stable identity of the element declaration.
func (declaration ElementDeclaration) ID() ComponentID {
	return declaration.component.ID()
}

// Name returns the expanded name of the element declaration.
func (declaration ElementDeclaration) Name() QName {
	return declaration.component.Name()
}

// Loc returns the declaration location of the element declaration.
func (declaration ElementDeclaration) Loc() Loc {
	return declaration.component.Loc()
}

// DeclaredType returns the expanded QName written in the element's type
// attribute.
func (declaration ElementDeclaration) DeclaredType() QName {
	if declaration.facts == nil {
		return QName{}
	}
	return declaration.facts.declaredType
}

// IsAbstract reports the effective abstract fact of the element declaration.
func (declaration ElementDeclaration) IsAbstract() bool {
	if declaration.facts == nil {
		return false
	}
	return declaration.facts.abstract
}

// IsNillable reports the effective nillable fact of the element declaration.
func (declaration ElementDeclaration) IsNillable() bool {
	if declaration.facts == nil {
		return false
	}
	return declaration.facts.nillable
}

// TypeID returns the identity of a named declared type. Built-in datatypes do
// not have synthetic component identities and return the zero ID.
func (declaration ElementDeclaration) TypeID() (ComponentID, bool) {
	if declaration.facts == nil || !declaration.facts.hasTypeID {
		return ComponentID{}, false
	}
	return declaration.facts.typeID, true
}

// TypeReference returns the resolved simple-type reference used by the
// declaration, when it has one.
func (declaration ElementDeclaration) TypeReference() (SimpleTypeReference, bool) {
	if declaration.facts == nil || !declaration.facts.hasTypeReference {
		return SimpleTypeReference{}, false
	}
	return SimpleTypeReference{facts: &declaration.facts.typeReference}, true
}

// InlineSimpleType returns the anonymous simple type declared inside the
// element, when it has one.
func (declaration ElementDeclaration) InlineSimpleType() (SimpleTypeDefinition, bool) {
	reference, ok := declaration.TypeReference()
	if !ok {
		return SimpleTypeDefinition{}, false
	}
	return reference.AnonymousType()
}

// Notation returns the immutable notation-declaration view for a global
// notation declaration.
func (component Component) Notation() (NotationDeclaration, bool) {
	if component.notation == nil {
		return NotationDeclaration{}, false
	}
	return NotationDeclaration{
		component: component,
		facts:     component.notation,
	}, true
}

// NotationDeclaration returns the immutable notation-declaration view for a
// global notation declaration.
func (component Component) NotationDeclaration() (NotationDeclaration, bool) {
	return component.Notation()
}

// NotationDeclaration is the immutable type-specific view of a global
// notation declaration.
type NotationDeclaration struct {
	component Component
	facts     *schemaNotationComponent
}

// Component returns the generic component represented by the view.
func (declaration NotationDeclaration) Component() Component {
	return declaration.component
}

// ID returns the stable identity of the notation declaration.
func (declaration NotationDeclaration) ID() ComponentID {
	return declaration.component.ID()
}

// Name returns the expanded QName of the notation declaration.
func (declaration NotationDeclaration) Name() QName {
	return declaration.component.Name()
}

// Loc returns the declaration location of the notation declaration.
func (declaration NotationDeclaration) Loc() Loc {
	return declaration.component.Loc()
}

// Public returns the collapsed public identifier value.
func (declaration NotationDeclaration) Public() string {
	if declaration.facts == nil {
		return ""
	}
	return declaration.facts.public
}

// PublicLoc returns the source location of the public identifier.
func (declaration NotationDeclaration) PublicLoc() Loc {
	if declaration.facts == nil {
		return Loc{}
	}
	return declaration.facts.publicLoc
}

// System returns the collapsed optional system identifier and whether it was
// present in the source declaration.
func (declaration NotationDeclaration) System() (string, bool) {
	if declaration.facts == nil || !declaration.facts.hasSystem {
		return "", false
	}
	return declaration.facts.system, true
}

// SystemLoc returns the source location of the optional system identifier and
// whether it was present in the source declaration.
func (declaration NotationDeclaration) SystemLoc() (Loc, bool) {
	if declaration.facts == nil || !declaration.facts.hasSystem {
		return Loc{}, false
	}
	return declaration.facts.systemLoc, true
}

// SimpleType returns the immutable simple-type view for a supported simple
// type definition.
func (component Component) SimpleType() (SimpleTypeDefinition, bool) {
	if component.simpleType == nil {
		return SimpleTypeDefinition{}, false
	}
	return SimpleTypeDefinition{
		component: component,
		facts:     component.simpleType,
	}, true
}

// SimpleTypeDefinition returns the immutable simple-type view for a supported
// simple type definition.
func (component Component) SimpleTypeDefinition() (SimpleTypeDefinition, bool) {
	return component.SimpleType()
}

// SimpleTypeDefinition is the immutable type-specific view of a supported
// simple type model node.
type SimpleTypeDefinition struct {
	component Component
	facts     *schemaSimpleTypeComponent
}

// Component returns the generic component represented by the view.
func (definition SimpleTypeDefinition) Component() Component {
	return definition.component
}

// ID returns the stable identity of the simple type definition.
func (definition SimpleTypeDefinition) ID() ComponentID {
	return definition.component.ID()
}

// NodeID returns the stable identity of the simple-type model node.
func (definition SimpleTypeDefinition) NodeID() (SimpleTypeID, bool) {
	if definition.facts == nil || !definition.facts.hasNodeID {
		return SimpleTypeID{}, false
	}
	return definition.facts.nodeID, true
}

// Name returns the expanded name of the simple type definition.
func (definition SimpleTypeDefinition) Name() QName {
	if definition.facts != nil && definition.facts.anonymous {
		return QName{}
	}
	return definition.component.Name()
}

// Loc returns the declaration location of the simple type definition.
func (definition SimpleTypeDefinition) Loc() Loc {
	if definition.facts != nil && definition.facts.anonymous {
		return definition.facts.loc
	}
	return definition.component.Loc()
}

// IsAnonymous reports whether this definition is an inline simple type.
func (definition SimpleTypeDefinition) IsAnonymous() bool {
	return definition.facts != nil && definition.facts.anonymous
}

// Variety returns the one resolved simple-type variety.
func (definition SimpleTypeDefinition) Variety() SimpleTypeVariety {
	if definition.facts == nil {
		return ""
	}
	return definition.facts.variety
}

// VarietyLoc returns the source location of the restriction, list, or union
// model child.
func (definition SimpleTypeDefinition) VarietyLoc() Loc {
	if definition.facts == nil {
		return Loc{}
	}
	return definition.facts.varietyLoc
}

// Base returns the expanded name written in the restriction's base attribute.
// It returns the zero QName when the restriction derives from an inline type.
func (definition SimpleTypeDefinition) Base() QName {
	if definition.facts == nil {
		return QName{}
	}
	return definition.facts.base
}

// BaseLoc returns the location of the restriction's base expression.
func (definition SimpleTypeDefinition) BaseLoc() Loc {
	if definition.facts == nil {
		return Loc{}
	}
	return definition.facts.baseLoc
}

// BaseID returns the identity of a named base type, when the base is not a
// built-in datatype descriptor.
func (definition SimpleTypeDefinition) BaseID() (ComponentID, bool) {
	if definition.facts == nil || !definition.facts.hasBaseID {
		return ComponentID{}, false
	}
	return definition.facts.baseID, true
}

// BaseReference returns the resolved restriction base reference.
func (definition SimpleTypeDefinition) BaseReference() (SimpleTypeReference, bool) {
	if definition.facts == nil || !definition.facts.hasBaseReference {
		return SimpleTypeReference{}, false
	}
	return SimpleTypeReference{facts: &definition.facts.baseReference}, true
}

// ItemType returns the resolved list item type.
func (definition SimpleTypeDefinition) ItemType() (SimpleTypeReference, bool) {
	if definition.facts == nil || !definition.facts.hasItemType {
		return SimpleTypeReference{}, false
	}
	return SimpleTypeReference{facts: &definition.facts.itemType}, true
}

// ListItemType is an alias for ItemType.
func (definition SimpleTypeDefinition) ListItemType() (SimpleTypeReference, bool) {
	return definition.ItemType()
}

// MemberTypes returns union members in schema lexical order. The returned
// slice is independent of the completed schema.
func (definition SimpleTypeDefinition) MemberTypes() []SimpleTypeReference {
	if definition.facts == nil || len(definition.facts.memberTypes) == 0 {
		return nil
	}
	members := make([]SimpleTypeReference, len(definition.facts.memberTypes))
	for index := range definition.facts.memberTypes {
		members[index] = SimpleTypeReference{facts: &definition.facts.memberTypes[index]}
	}
	return members
}

// UnionMemberTypes is an alias for MemberTypes.
func (definition SimpleTypeDefinition) UnionMemberTypes() []SimpleTypeReference {
	return definition.MemberTypes()
}

// IsBoolean reports whether the simple type is derived from the XSD boolean
// datatype.
func (definition SimpleTypeDefinition) IsBoolean() bool {
	if definition.facts == nil {
		return false
	}
	_, ok := definition.facts.facets.(schemaBooleanFacetVariant)
	return ok
}

// DigitFacets returns the effective totalDigits and fractionDigits facets.
func (definition SimpleTypeDefinition) DigitFacets() DigitFacets {
	if definition.facts == nil {
		return DigitFacets{}
	}
	switch facets := definition.facts.facets.(type) {
	case schemaDigitFacetVariant:
		return facets.value
	case schemaIntegerFacetVariant:
		return facets.digits
	case schemaDecimalFacetVariant:
		return facets.digits
	default:
		return DigitFacets{}
	}
}

// IntegerEnumerationFacets returns the effective integer enumeration facets.
// It returns the zero value for a decimal or precisionDecimal simple type.
func (definition SimpleTypeDefinition) IntegerEnumerationFacets() IntegerEnumerationFacets {
	if definition.facts == nil {
		return IntegerEnumerationFacets{}
	}
	facets, ok := definition.facts.facets.(schemaIntegerFacetVariant)
	if !ok {
		return IntegerEnumerationFacets{}
	}
	return facets.enumeration
}

// DecimalEnumerationFacets returns the effective decimal enumeration facets.
// It returns the zero value for an integer or precisionDecimal simple type.
func (definition SimpleTypeDefinition) DecimalEnumerationFacets() DecimalEnumerationFacets {
	if definition.facts == nil {
		return DecimalEnumerationFacets{}
	}
	facets, ok := definition.facts.facets.(schemaDecimalFacetVariant)
	if !ok {
		return DecimalEnumerationFacets{}
	}
	return facets.enumeration
}

// IntegerBounds returns the effective ordered integer bounds and their
// presence for an integer restriction.
func (definition SimpleTypeDefinition) IntegerBounds() (IntegerBoundFacets, bool) {
	if definition.facts == nil {
		return IntegerBoundFacets{}, false
	}
	switch facets := definition.facts.facets.(type) {
	case schemaDigitFacetVariant:
		if facets.value.Kind() != DigitDatatypeInteger {
			return IntegerBoundFacets{}, false
		}
		return facets.integerBounds, true
	case schemaIntegerFacetVariant:
		return facets.bounds, true
	default:
		return IntegerBoundFacets{}, false
	}
}

// DecimalBounds returns the effective ordered decimal bounds and their
// presence for a decimal restriction.
func (definition SimpleTypeDefinition) DecimalBounds() (DecimalBoundFacets, bool) {
	if definition.facts == nil {
		return DecimalBoundFacets{}, false
	}
	switch facets := definition.facts.facets.(type) {
	case schemaDigitFacetVariant:
		if facets.value.Kind() != DigitDatatypeDecimal {
			return DecimalBoundFacets{}, false
		}
		return facets.decimalBounds, true
	case schemaDecimalFacetVariant:
		return facets.bounds, true
	default:
		return DecimalBoundFacets{}, false
	}
}

// PrecisionDecimalFacets returns the effective precisionDecimal facets. It
// returns the zero value for an integer or decimal simple type.
func (definition SimpleTypeDefinition) PrecisionDecimalFacets() PrecisionDecimalFacets {
	if definition.facts == nil {
		return PrecisionDecimalFacets{}
	}
	facets, ok := definition.facts.facets.(schemaPrecisionDecimalFacetVariant)
	if !ok {
		return PrecisionDecimalFacets{}
	}
	return facets.value
}

// HasPrecisionDecimalFacets reports whether the simple type is backed by the
// optional precisionDecimal datatype.
func (definition SimpleTypeDefinition) HasPrecisionDecimalFacets() bool {
	if definition.facts == nil {
		return false
	}
	_, ok := definition.facts.facets.(schemaPrecisionDecimalFacetVariant)
	return ok
}

// ComplexType returns the immutable complex-type view for a supported named
// complex type definition.
func (component Component) ComplexType() (ComplexTypeDefinition, bool) {
	if component.complexType == nil {
		return ComplexTypeDefinition{}, false
	}
	return ComplexTypeDefinition{
		component: component,
		facts:     component.complexType,
	}, true
}

// ComplexTypeDefinition returns the immutable complex-type view for a
// supported named complex type definition.
func (component Component) ComplexTypeDefinition() (ComplexTypeDefinition, bool) {
	return component.ComplexType()
}

// ComplexTypeDefinition is the immutable type-specific view of a supported
// named complex type with a particle model.
type ComplexTypeDefinition struct {
	component Component
	facts     *schemaComplexTypeComponent
}

// Component returns the generic component represented by the view.
func (definition ComplexTypeDefinition) Component() Component {
	return definition.component
}

// ID returns the stable identity of the complex type definition.
func (definition ComplexTypeDefinition) ID() ComponentID {
	return definition.component.ID()
}

// Name returns the expanded name of the complex type definition.
func (definition ComplexTypeDefinition) Name() QName {
	return definition.component.Name()
}

// Loc returns the declaration location of the complex type definition.
func (definition ComplexTypeDefinition) Loc() Loc {
	return definition.component.Loc()
}

// Particle returns the immutable content particle of the complex type.
func (definition ComplexTypeDefinition) Particle() Particle {
	if definition.facts == nil {
		return nil
	}
	return definition.facts.particle
}

// ParticleOccurrenceMaximumKind identifies the complete maximum occurrence
// variant exposed by a particle occurrence range.
type ParticleOccurrenceMaximumKind uint8

const (
	// ParticleOccurrenceMaximumFinite identifies an exact finite maximum.
	ParticleOccurrenceMaximumFinite ParticleOccurrenceMaximumKind = iota + 1
	// ParticleOccurrenceMaximumUnbounded identifies an unbounded maximum.
	ParticleOccurrenceMaximumUnbounded
)

// ParticleOccurrenceMaximum is the exact maximum occurrence value. A finite
// value is arbitrary precision; the unbounded variant has no numeric value.
type ParticleOccurrenceMaximum struct {
	value particleOccurrence
}

// Kind reports whether the maximum is finite or unbounded.
func (maximum ParticleOccurrenceMaximum) Kind() ParticleOccurrenceMaximumKind {
	if maximum.value.isUnbounded() {
		return ParticleOccurrenceMaximumUnbounded
	}
	return ParticleOccurrenceMaximumFinite
}

// IsUnbounded reports whether the maximum has the distinct unbounded value.
func (maximum ParticleOccurrenceMaximum) IsUnbounded() bool {
	return maximum.value.isUnbounded()
}

// Finite returns an owned exact maximum when the value is finite.
func (maximum ParticleOccurrenceMaximum) Finite() (StrictInteger, bool) {
	return maximum.value.finiteValue()
}

// String returns the canonical finite value or the unbounded keyword.
func (maximum ParticleOccurrenceMaximum) String() string {
	return maximum.value.String()
}

// ParticleOccurrenceRange is an immutable exact particle occurrence range.
// Its minimum is always finite and its maximum is finite or unbounded.
type ParticleOccurrenceRange struct {
	value particleOccurrenceRange
}

// Minimum returns an owned exact finite minimum occurrence value.
func (occurrences ParticleOccurrenceRange) Minimum() StrictInteger {
	minimum, ok := occurrences.value.minimumOccurrence().finiteValue()
	if !ok {
		return StrictInteger{}
	}
	return minimum
}

// Maximum returns the complete finite or unbounded maximum occurrence value.
func (occurrences ParticleOccurrenceRange) Maximum() ParticleOccurrenceMaximum {
	return ParticleOccurrenceMaximum{
		value: occurrences.value.maximumOccurrence(),
	}
}

// IsDefault reports whether the effective range is the default 1/1 range.
func (occurrences ParticleOccurrenceRange) IsDefault() bool {
	return occurrences.value.isDefault()
}

// String returns the canonical minimum/maximum range.
func (occurrences ParticleOccurrenceRange) String() string {
	return occurrences.value.String()
}

// Particle is a concrete immutable content-model alternative.
type Particle interface {
	Loc() Loc
	MinOccurs() uint64
	MaxOccurs() uint64
	Occurrences() ParticleOccurrenceRange
	particle()
}

// ChoiceParticle is a particle whose alternatives are tried in lexical
// declaration order.
type ChoiceParticle struct {
	facts *schemaChoiceParticle
}

func (ChoiceParticle) particle() {}

// Loc returns the location of the choice particle.
func (particle ChoiceParticle) Loc() Loc {
	if particle.facts == nil {
		return Loc{}
	}
	return particle.facts.loc
}

// Occurrences returns the exact immutable occurrence range.
func (particle ChoiceParticle) Occurrences() ParticleOccurrenceRange {
	if particle.facts == nil {
		return ParticleOccurrenceRange{}
	}
	return newPublicParticleOccurrenceRange(particle.facts.occurrences)
}

// MinOccurs returns the default minimum occurrence bound.
//
// Deprecated: use Occurrences().Minimum(). This compatibility accessor is
// defined only for default-only choice particles and returns zero otherwise.
func (particle ChoiceParticle) MinOccurs() uint64 {
	if particle.facts == nil || !particle.facts.occurrences.isDefault() {
		return 0
	}
	return 1
}

// MaxOccurs returns the default maximum occurrence bound.
//
// Deprecated: use Occurrences().Maximum(). This compatibility accessor is
// defined only for default-only choice particles and returns zero otherwise.
func (particle ChoiceParticle) MaxOccurs() uint64 {
	if particle.facts == nil || !particle.facts.occurrences.isDefault() {
		return 0
	}
	return 1
}

// Alternatives returns the choice alternatives in lexical declaration order.
// The returned slice is independent of the completed schema.
func (particle ChoiceParticle) Alternatives() []Particle {
	if particle.facts == nil || len(particle.facts.alternatives) == 0 {
		return nil
	}
	return append([]Particle(nil), particle.facts.alternatives...)
}

// ElementParticle is a local element declaration particle.
type ElementParticle struct {
	facts *schemaElementParticle
}

func (ElementParticle) particle() {}

// Loc returns the location of the local element particle.
func (particle ElementParticle) Loc() Loc {
	if particle.facts == nil {
		return Loc{}
	}
	return particle.facts.loc
}

// Occurrences returns the exact immutable occurrence range.
func (particle ElementParticle) Occurrences() ParticleOccurrenceRange {
	if particle.facts == nil {
		return ParticleOccurrenceRange{}
	}
	return newPublicParticleOccurrenceRange(particle.facts.occurrences)
}

// MinOccurs returns the default minimum occurrence bound.
//
// Deprecated: use Occurrences().Minimum(). This compatibility accessor is
// defined only for default-only element particles and returns zero otherwise.
func (particle ElementParticle) MinOccurs() uint64 {
	if particle.facts == nil || !particle.facts.occurrences.isDefault() {
		return 0
	}
	return 1
}

// MaxOccurs returns the default maximum occurrence bound.
//
// Deprecated: use Occurrences().Maximum(). This compatibility accessor is
// defined only for default-only element particles and returns zero otherwise.
func (particle ElementParticle) MaxOccurs() uint64 {
	if particle.facts == nil || !particle.facts.occurrences.isDefault() {
		return 0
	}
	return 1
}

// Name returns the expanded name of the local element declaration.
func (particle ElementParticle) Name() QName {
	if particle.facts == nil {
		return QName{}
	}
	return particle.facts.name
}

// DeclaredType returns the expanded QName written in the element's type
// attribute.
func (particle ElementParticle) DeclaredType() QName {
	if particle.facts == nil {
		return QName{}
	}
	return particle.facts.declaredType
}

// TypeID returns the identity of the named declared type. Built-in datatypes
// do not have synthetic component identities and return the zero ID.
func (particle ElementParticle) TypeID() (ComponentID, bool) {
	if particle.facts == nil || !particle.facts.hasTypeID {
		return ComponentID{}, false
	}
	return particle.facts.typeID, true
}

// SequenceParticle is an ordered direct sequence of local element particles.
type SequenceParticle struct {
	facts *schemaSequenceParticle
}

func (SequenceParticle) particle() {}

// Loc returns the location of the sequence particle.
func (particle SequenceParticle) Loc() Loc {
	if particle.facts == nil {
		return Loc{}
	}
	return particle.facts.loc
}

// Occurrences returns the exact immutable occurrence range.
func (particle SequenceParticle) Occurrences() ParticleOccurrenceRange {
	if particle.facts == nil {
		return ParticleOccurrenceRange{}
	}
	return newPublicParticleOccurrenceRange(particle.facts.occurrences)
}

// MinOccurs returns the default minimum occurrence bound.
//
// Deprecated: use Occurrences().Minimum(). This compatibility accessor is
// defined only for default-only sequence particles and returns zero otherwise.
func (particle SequenceParticle) MinOccurs() uint64 {
	if particle.facts == nil || !particle.facts.occurrences.isDefault() {
		return 0
	}
	return 1
}

// MaxOccurs returns the default maximum occurrence bound.
//
// Deprecated: use Occurrences().Maximum(). This compatibility accessor is
// defined only for default-only sequence particles and returns zero otherwise.
func (particle SequenceParticle) MaxOccurs() uint64 {
	if particle.facts == nil || !particle.facts.occurrences.isDefault() {
		return 0
	}
	return 1
}

// Elements returns direct local element particles in lexical declaration
// order. The returned slice is independent of the completed schema.
func (particle SequenceParticle) Elements() []ElementParticle {
	if particle.facts == nil || len(particle.facts.elements) == 0 {
		return nil
	}
	return append([]ElementParticle(nil), particle.facts.elements...)
}

// SchemaDocument is an immutable document in a Schema's discovery order.
type SchemaDocument struct {
	source          SourceID
	rootLoc         Loc
	targetNamespace string
	storage         *schemaStorage
	start           int
	count           int
}

// Source returns the resolver-provided identity of the document.
func (document SchemaDocument) Source() SourceID {
	return document.source
}

// RootLoc returns the lexical location of the document's root element.
func (document SchemaDocument) RootLoc() Loc {
	return document.rootLoc
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
	policy    LanguagePolicy
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
			rootLoc:         document.rootLoc,
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
	rootLoc         Loc
	targetNamespace string
	// declarations contains the named schema-level declarations in lexical
	// order. Local particle components will use a separate scoped model.
	declarations []schemaComponentInput
}

type schemaComponentInput struct {
	kind        ComponentKind
	name        QName
	loc         Loc
	element     *schemaElementInput
	notation    *schemaNotationInput
	simpleType  *schemaSimpleTypeInput
	complexType *schemaComplexTypeInput
}

type schemaElementInput struct {
	declaredType     QName
	typeLoc          Loc
	inlineSimpleType *schemaSimpleTypeInput
	abstract         bool
	nillable         bool
}

type schemaNotationInput struct {
	public    string
	publicLoc Loc
	system    string
	systemLoc Loc
	hasSystem bool
}

type schemaSimpleTypeInput struct {
	loc       Loc
	nodeID    SimpleTypeID
	hasNodeID bool
	model     schemaSimpleTypeModelInput

	// These fields keep the phase-local construction helpers used by existing
	// callers source-compatible. New syntax construction stores the tagged
	// model above; resolution normalizes this legacy restriction shape.
	base    QName
	baseLoc Loc
	facets  []schemaFacetInput
}

type schemaSimpleTypeReferenceInputKind uint8

const (
	schemaSimpleTypeQNameReferenceInput schemaSimpleTypeReferenceInputKind = iota + 1
	schemaSimpleTypeAnonymousReferenceInput
)

type schemaSimpleTypeReferenceInput struct {
	kind      schemaSimpleTypeReferenceInputKind
	name      QName
	loc       Loc
	anonymous *schemaSimpleTypeInput
}

type schemaSimpleTypeModelInput interface {
	schemaSimpleTypeModelInput()
}

type schemaSimpleTypeRestrictionModelInput struct {
	loc    Loc
	base   schemaSimpleTypeReferenceInput
	facets []schemaFacetInput
}

func (*schemaSimpleTypeRestrictionModelInput) schemaSimpleTypeModelInput() {}

type schemaSimpleTypeListModelInput struct {
	loc      Loc
	itemType schemaSimpleTypeReferenceInput
}

func (*schemaSimpleTypeListModelInput) schemaSimpleTypeModelInput() {}

type schemaSimpleTypeUnionModelInput struct {
	loc     Loc
	members []schemaSimpleTypeReferenceInput
}

func (*schemaSimpleTypeUnionModelInput) schemaSimpleTypeModelInput() {}

type schemaFacetKind uint8

const (
	schemaFacetTotalDigits schemaFacetKind = iota + 1
	schemaFacetFractionDigits
	schemaFacetMinScale
	schemaFacetMaxScale
	schemaFacetPattern
	schemaFacetEnumeration
	schemaFacetMinInclusive
	schemaFacetMinExclusive
	schemaFacetMaxInclusive
	schemaFacetMaxExclusive
	schemaFacetWhiteSpace
	schemaFacetLength
	schemaFacetMinLength
	schemaFacetMaxLength
	schemaFacetPrecision
	schemaFacetExplicitTimezone
)

type schemaFacetInput struct {
	lexical  string
	loc      Loc
	valueLoc Loc
	kind     schemaFacetKind
	fixed    bool
}

type schemaSimpleTypeReferenceComponent struct {
	kind           SimpleTypeReferenceKind
	name           QName
	loc            Loc
	id             ComponentID
	hasID          bool
	anonymousID    SimpleTypeID
	hasAnonymousID bool
	anonymous      *schemaSimpleTypeComponent
	variety        SimpleTypeVariety
	varietyLoc     Loc
	atomicKind     schemaSimpleTypeAtomicKind
	facets         schemaSimpleTypeFacetVariant
}

type schemaSimpleTypeComponent struct {
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

type schemaSimpleTypeFacetVariant interface {
	schemaSimpleTypeFacetVariant()
}

type schemaDigitFacetVariant struct {
	value         DigitFacets
	integerBounds IntegerBoundFacets
	decimalBounds DecimalBoundFacets
}

func (schemaDigitFacetVariant) schemaSimpleTypeFacetVariant() {}

type schemaIntegerFacetVariant struct {
	digits      DigitFacets
	enumeration IntegerEnumerationFacets
	bounds      IntegerBoundFacets
}

func (schemaIntegerFacetVariant) schemaSimpleTypeFacetVariant() {}

type schemaDecimalFacetVariant struct {
	digits      DigitFacets
	enumeration DecimalEnumerationFacets
	bounds      DecimalBoundFacets
}

func (schemaDecimalFacetVariant) schemaSimpleTypeFacetVariant() {}

type schemaPrecisionDecimalFacetVariant struct {
	value PrecisionDecimalFacets
}

func (schemaPrecisionDecimalFacetVariant) schemaSimpleTypeFacetVariant() {}

type schemaStringFacetVariant struct{}

func (schemaStringFacetVariant) schemaSimpleTypeFacetVariant() {}

type schemaBooleanFacetVariant struct{}

func (schemaBooleanFacetVariant) schemaSimpleTypeFacetVariant() {}

type schemaComplexTypeInput struct {
	particle schemaComplexTypeParticleInput
}

type schemaComplexTypeParticleInput interface {
	schemaComplexTypeParticleInput()
}

type schemaChoiceParticleInput struct {
	loc          Loc
	occurrences  particleOccurrenceRange
	alternatives []schemaElementParticleInput
}

func (*schemaChoiceParticleInput) schemaComplexTypeParticleInput() {}

type schemaSequenceParticleInput struct {
	loc         Loc
	occurrences particleOccurrenceRange
	elements    []schemaElementParticleInput
}

func (*schemaSequenceParticleInput) schemaComplexTypeParticleInput() {}

type schemaElementParticleInput struct {
	loc         Loc
	name        QName
	occurrences particleOccurrenceRange
	typeInput   *schemaElementInput
}

type schemaElementComponent struct {
	declaredType     QName
	typeID           ComponentID
	hasTypeID        bool
	typeReference    schemaSimpleTypeReferenceComponent
	hasTypeReference bool
	abstract         bool
	nillable         bool
}

type schemaNotationComponent struct {
	public    string
	publicLoc Loc
	system    string
	systemLoc Loc
	hasSystem bool
}

type schemaComplexTypeComponent struct {
	particle Particle
}

type schemaChoiceParticle struct {
	loc          Loc
	occurrences  particleOccurrenceRange
	alternatives []Particle
}

type schemaElementParticle struct {
	loc          Loc
	occurrences  particleOccurrenceRange
	name         QName
	declaredType QName
	typeID       ComponentID
	hasTypeID    bool
}

type schemaSequenceParticle struct {
	loc         Loc
	occurrences particleOccurrenceRange
	elements    []ElementParticle
}

type schemaComponentRecord struct {
	id          ComponentID
	kind        ComponentKind
	name        QName
	loc         Loc
	element     *schemaElementInput
	notation    *schemaNotationInput
	simpleType  *schemaSimpleTypeInput
	complexType *schemaComplexTypeInput
}

type schemaSymbolSpace uint8

const (
	schemaSymbolSpaceElement schemaSymbolSpace = iota + 1
	schemaSymbolSpaceAttribute
	schemaSymbolSpaceType
	schemaSymbolSpaceModelGroup
	schemaSymbolSpaceAttributeGroup
	schemaSymbolSpaceNotation
)

type schemaSymbolKey struct {
	space schemaSymbolSpace
	name  QName
}

func schemaSymbolSpaceForComponentKind(kind ComponentKind) (schemaSymbolSpace, bool) {
	switch kind {
	case ComponentKindElementDeclaration:
		return schemaSymbolSpaceElement, true
	case ComponentKindAttributeDeclaration:
		return schemaSymbolSpaceAttribute, true
	case ComponentKindSimpleTypeDefinition, ComponentKindComplexTypeDefinition:
		return schemaSymbolSpaceType, true
	case ComponentKindModelGroupDefinition:
		return schemaSymbolSpaceModelGroup, true
	case ComponentKindAttributeGroupDefinition:
		return schemaSymbolSpaceAttributeGroup, true
	case ComponentKindNotationDeclaration:
		return schemaSymbolSpaceNotation, true
	default:
		return 0, false
	}
}

// newSchema completes the ordered component representation after discovery
// and declaration phases have supplied all document identities and facts.
// Input slices are consumed only for construction; the returned schema owns
// its ordered storage.
func newSchema(inputs []schemaDocumentInput) (Schema, error) {
	return newSchemaWithPolicy(inputs, Compatibility)
}

// newSchemaWithPolicy derives the construction version once from the validated
// graph policy and passes it through component resolution.
func newSchemaWithPolicy(inputs []schemaDocumentInput, policy LanguagePolicy) (Schema, error) {
	version, err := xsdVersionForLanguagePolicy(policy)
	if err != nil {
		return Schema{}, invalidLanguagePolicyDiagnostic(policy, err)
	}
	documents, records, byName, err := allocateSchemaRecords(inputs)
	if err != nil {
		return Schema{}, err
	}
	if allocationErr := allocateSchemaSimpleTypeNodeIDs(records); allocationErr != nil {
		return Schema{}, allocationErr
	}
	if duplicateErr := rejectDuplicateSchemaDeclarations(records, version); duplicateErr != nil {
		return Schema{}, duplicateErr
	}
	simpleTypes, err := resolveSchemaSimpleTypes(records, byName, version)
	if err != nil {
		return Schema{}, err
	}
	complexTypes, err := resolveSchemaComplexTypes(records, byName, simpleTypes.results, version)
	if err != nil {
		return Schema{}, err
	}
	elements, err := resolveSchemaElementTypes(records, byName, simpleTypes, complexTypes, version)
	if err != nil {
		return Schema{}, err
	}
	components, byID := completeSchemaComponents(records, simpleTypes.results, elements, complexTypes)
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
		policy:    policy,
	}, nil
}

func rejectDuplicateSchemaDeclarations(records []schemaComponentRecord, version XSDVersion) error {
	earliest := make(map[schemaSymbolKey]int)
	for index, record := range records {
		space, ok := schemaSymbolSpaceForComponentKind(record.kind)
		if !ok {
			continue
		}
		key := schemaSymbolKey{space: space, name: record.name}
		first, ok := earliest[key]
		if ok {
			return newSchemaDuplicateDiagnostic(record, records[first], space, version)
		}
		earliest[key] = index
	}
	return nil
}

func allocateSchemaRecords(inputs []schemaDocumentInput) ([]SchemaDocument, []schemaComponentRecord, map[QName][]int, error) {
	documents := make([]SchemaDocument, 0, len(inputs))
	records := make([]schemaComponentRecord, 0)
	byName := make(map[QName][]int)
	seenSources := make(map[SourceID]struct{}, len(inputs))

	for _, input := range inputs {
		if err := validateSchemaDocumentInput(input, seenSources); err != nil {
			return nil, nil, nil, err
		}
		seenSources[input.source] = struct{}{}

		documentStart := len(records)
		for declarationIndex, declaration := range input.declarations {
			record, err := newSchemaComponentRecord(input.source, declarationIndex, declaration)
			if err != nil {
				return nil, nil, nil, err
			}
			byName[record.name] = append(byName[record.name], len(records))
			records = append(records, record)
		}

		documents = append(documents, SchemaDocument{
			source:          input.source,
			rootLoc:         input.rootLoc,
			targetNamespace: input.targetNamespace,
			start:           documentStart,
			count:           len(records) - documentStart,
		})
	}
	return documents, records, byName, nil
}

func validateSchemaDocumentInput(input schemaDocumentInput, seenSources map[SourceID]struct{}) error {
	if input.source == "" {
		return newDiagnostic(
			FailureInternal,
			diagnosticSchemaEmptySourceCode,
			Loc{},
			"schema document has an empty source identity",
			nil,
		)
	}
	if _, exists := seenSources[input.source]; exists {
		return newDiagnostic(
			FailureInternal,
			diagnosticSchemaRepeatedSourceCode,
			Loc{},
			fmt.Sprintf("schema document source %q is repeated", input.source),
			nil,
		)
	}
	if input.rootLoc.IsZero() {
		return newSchemaBridgeInvariant(Loc{}, "schema document has no root location")
	}
	if input.rootLoc.Line() <= 0 || input.rootLoc.Column() <= 0 {
		return newSchemaBridgeInvariant(input.rootLoc, "schema document root location has non-positive coordinates")
	}
	if input.rootLoc.Source() != input.source {
		return newSchemaBridgeInvariant(
			input.rootLoc,
			fmt.Sprintf("schema document root location source %q does not match source %q", input.rootLoc.Source(), input.source),
		)
	}
	return nil
}

func newSchemaComponentRecord(source SourceID, declarationIndex int, declaration schemaComponentInput) (schemaComponentRecord, error) {
	if declaration.kind == "" {
		return schemaComponentRecord{}, newDiagnostic(
			FailureInternal,
			diagnosticSchemaEmptyKindCode,
			declaration.loc,
			"schema component has an empty kind",
			nil,
		)
	}
	if declaration.name.IsZero() {
		return schemaComponentRecord{}, newDiagnostic(
			FailureInternal,
			diagnosticSchemaEmptyNameCode,
			declaration.loc,
			"schema component has an empty name",
			nil,
		)
	}
	ordinal, err := schemaComponentOrdinal(declarationIndex, declaration.loc)
	if err != nil {
		return schemaComponentRecord{}, err
	}
	return schemaComponentRecord{
		id: ComponentID{
			source:  source,
			ordinal: ordinal,
		},
		kind:        declaration.kind,
		name:        declaration.name,
		loc:         declaration.loc,
		element:     cloneSchemaElementInput(declaration.element),
		notation:    cloneSchemaNotationInput(declaration.notation),
		simpleType:  cloneSchemaSimpleTypeInput(declaration.simpleType),
		complexType: cloneSchemaComplexTypeInput(declaration.complexType),
	}, nil
}

func schemaComponentOrdinal(declarationIndex int, loc Loc) (uint64, error) {
	if declarationIndex < 0 {
		return 0, newSchemaBridgeInvariant(loc, "schema component ordinal index is negative")
	}
	ordinal, err := strconv.ParseUint(strconv.Itoa(declarationIndex), 10, 64)
	if err != nil {
		return 0, newSchemaBridgeInvariant(loc, "schema component ordinal index overflows uint64")
	}
	if ordinal == ^uint64(0) {
		return 0, newSchemaBridgeInvariant(loc, "schema component ordinal overflows uint64")
	}
	return ordinal + 1, nil
}

func completeSchemaComponents(
	records []schemaComponentRecord,
	simpleTypes []schemaSimpleTypeResult,
	elements []schemaElementTypeResult,
	complexTypes []schemaComplexTypeResult,
) ([]Component, map[ComponentID]int) {
	components := make([]Component, 0, len(records))
	byID := make(map[ComponentID]int, len(records))
	for index, record := range records {
		component := completeSchemaComponent(record, simpleTypes[index], elements[index], complexTypes[index])
		byID[record.id] = len(components)
		components = append(components, component)
	}
	return components, byID
}

func completeSchemaComponent(
	record schemaComponentRecord,
	simpleType schemaSimpleTypeResult,
	element schemaElementTypeResult,
	complexType schemaComplexTypeResult,
) Component {
	component := Component{
		id:   record.id,
		kind: record.kind,
		name: record.name,
		loc:  record.loc,
	}
	if element.present {
		component.element = &schemaElementComponent{
			declaredType:     element.declaredType,
			typeID:           element.typeID,
			hasTypeID:        element.hasTypeID,
			typeReference:    element.typeReference,
			hasTypeReference: element.hasTypeReference,
			abstract:         element.abstract,
			nillable:         element.nillable,
		}
	}
	if record.notation != nil {
		component.notation = &schemaNotationComponent{
			public:    record.notation.public,
			publicLoc: record.notation.publicLoc,
			system:    record.notation.system,
			systemLoc: record.notation.systemLoc,
			hasSystem: record.notation.hasSystem,
		}
	}
	if simpleType.present {
		component.simpleType = &schemaSimpleTypeComponent{
			loc:              simpleType.loc,
			nodeID:           simpleType.nodeID,
			hasNodeID:        simpleType.hasNodeID,
			anonymous:        simpleType.anonymous,
			variety:          simpleType.variety,
			varietyLoc:       simpleType.varietyLoc,
			atomicKind:       simpleType.atomicKind,
			base:             simpleType.base,
			baseLoc:          simpleType.baseLoc,
			baseID:           simpleType.baseID,
			hasBaseID:        simpleType.hasBaseID,
			baseReference:    simpleType.baseReference,
			hasBaseReference: simpleType.hasBaseReference,
			itemType:         simpleType.itemType,
			hasItemType:      simpleType.hasItemType,
			memberTypes:      cloneSchemaSimpleTypeReferenceComponents(simpleType.memberTypes),
			facets:           simpleType.facets,
		}
	}
	if complexType.present {
		component.complexType = &schemaComplexTypeComponent{
			particle: complexType.particle,
		}
	}
	return component
}

func cloneSchemaNotationInput(input *schemaNotationInput) *schemaNotationInput {
	if input == nil {
		return nil
	}
	return &schemaNotationInput{
		public:    input.public,
		publicLoc: input.publicLoc,
		system:    input.system,
		systemLoc: input.systemLoc,
		hasSystem: input.hasSystem,
	}
}

func cloneSchemaComplexTypeInput(input *schemaComplexTypeInput) *schemaComplexTypeInput {
	if input == nil {
		return nil
	}
	clone := &schemaComplexTypeInput{particle: input.particle}
	switch particle := input.particle.(type) {
	case *schemaChoiceParticleInput:
		if particle == nil {
			return clone
		}
		clone.particle = &schemaChoiceParticleInput{
			loc:          particle.loc,
			occurrences:  particle.occurrences.clone(),
			alternatives: cloneSchemaElementParticleInputs(particle.alternatives),
		}
	case *schemaSequenceParticleInput:
		if particle == nil {
			return clone
		}
		clone.particle = &schemaSequenceParticleInput{
			loc:         particle.loc,
			occurrences: particle.occurrences.clone(),
			elements:    cloneSchemaElementParticleInputs(particle.elements),
		}
	}
	return clone
}

func cloneSchemaSimpleTypeInput(input *schemaSimpleTypeInput) *schemaSimpleTypeInput {
	if input == nil {
		return nil
	}
	clone := &schemaSimpleTypeInput{
		loc:       input.loc,
		nodeID:    input.nodeID,
		hasNodeID: input.hasNodeID,
		base:      input.base,
		baseLoc:   input.baseLoc,
		facets:    cloneSchemaFacetInputs(input.facets),
	}
	clone.model = cloneSchemaSimpleTypeModelInput(input.model)
	return clone
}

func cloneSchemaElementInput(input *schemaElementInput) *schemaElementInput {
	if input == nil {
		return nil
	}
	return &schemaElementInput{
		declaredType:     input.declaredType,
		typeLoc:          input.typeLoc,
		inlineSimpleType: cloneSchemaSimpleTypeInput(input.inlineSimpleType),
		abstract:         input.abstract,
		nillable:         input.nillable,
	}
}

func cloneSchemaSimpleTypeModelInput(input schemaSimpleTypeModelInput) schemaSimpleTypeModelInput {
	switch model := input.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if model == nil {
			return (*schemaSimpleTypeRestrictionModelInput)(nil)
		}
		return &schemaSimpleTypeRestrictionModelInput{
			loc:    model.loc,
			base:   cloneSchemaSimpleTypeReferenceInput(model.base),
			facets: cloneSchemaFacetInputs(model.facets),
		}
	case *schemaSimpleTypeListModelInput:
		if model == nil {
			return (*schemaSimpleTypeListModelInput)(nil)
		}
		return &schemaSimpleTypeListModelInput{
			loc:      model.loc,
			itemType: cloneSchemaSimpleTypeReferenceInput(model.itemType),
		}
	case *schemaSimpleTypeUnionModelInput:
		if model == nil {
			return (*schemaSimpleTypeUnionModelInput)(nil)
		}
		members := make([]schemaSimpleTypeReferenceInput, len(model.members))
		for index, member := range model.members {
			members[index] = cloneSchemaSimpleTypeReferenceInput(member)
		}
		return &schemaSimpleTypeUnionModelInput{
			loc:     model.loc,
			members: members,
		}
	default:
		return nil
	}
}

func cloneSchemaSimpleTypeReferenceInput(input schemaSimpleTypeReferenceInput) schemaSimpleTypeReferenceInput {
	clone := input
	clone.anonymous = cloneSchemaSimpleTypeInput(input.anonymous)
	return clone
}

func cloneSchemaFacetInputs(inputs []schemaFacetInput) []schemaFacetInput {
	if len(inputs) == 0 {
		return nil
	}
	return append([]schemaFacetInput(nil), inputs...)
}

func cloneSchemaSimpleTypeReferenceComponents(inputs []schemaSimpleTypeReferenceComponent) []schemaSimpleTypeReferenceComponent {
	if len(inputs) == 0 {
		return nil
	}
	clones := make([]schemaSimpleTypeReferenceComponent, len(inputs))
	copy(clones, inputs)
	return clones
}

func allocateSchemaSimpleTypeNodeIDs(records []schemaComponentRecord) error {
	nextBySource := make(map[SourceID]uint64)
	seen := make(map[*schemaSimpleTypeInput]SimpleTypeID)
	for _, record := range records {
		if record.simpleType != nil {
			if err := allocateSchemaSimpleTypeNodeID(record.simpleType, record.id.Source(), nextBySource, seen); err != nil {
				return err
			}
		}
		if record.element == nil || record.element.inlineSimpleType == nil {
			continue
		}
		if err := allocateSchemaSimpleTypeNodeID(record.element.inlineSimpleType, record.id.Source(), nextBySource, seen); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocognit // Keep deterministic recursive model-node allocation explicit.
func allocateSchemaSimpleTypeNodeID(
	input *schemaSimpleTypeInput,
	source SourceID,
	nextBySource map[SourceID]uint64,
	seen map[*schemaSimpleTypeInput]SimpleTypeID,
) error {
	if input == nil {
		return newSchemaBridgeInvariant(Loc{}, "simple type node allocation received a nil input")
	}
	if assigned, ok := seen[input]; ok {
		if assigned.Source() != source {
			return newSchemaBridgeInvariant(input.loc, "simple type node is shared across source documents")
		}
		return nil
	}
	next := nextBySource[source]
	if next == 0 {
		next = 1
	}
	if next == ^uint64(0) {
		return newSchemaBridgeInvariant(input.loc, "simple type node ordinal overflows uint64")
	}
	input.nodeID = SimpleTypeID{source: source, ordinal: next}
	input.hasNodeID = true
	seen[input] = input.nodeID
	nextBySource[source] = next + 1

	model := input.model
	if model == nil {
		return nil
	}
	switch typed := model.(type) {
	case *schemaSimpleTypeRestrictionModelInput:
		if typed == nil || typed.base.kind != schemaSimpleTypeAnonymousReferenceInput {
			return nil
		}
		return allocateSchemaSimpleTypeNodeID(typed.base.anonymous, source, nextBySource, seen)
	case *schemaSimpleTypeListModelInput:
		if typed == nil || typed.itemType.kind != schemaSimpleTypeAnonymousReferenceInput {
			return nil
		}
		return allocateSchemaSimpleTypeNodeID(typed.itemType.anonymous, source, nextBySource, seen)
	case *schemaSimpleTypeUnionModelInput:
		if typed == nil {
			return nil
		}
		for _, member := range typed.members {
			if member.kind != schemaSimpleTypeAnonymousReferenceInput {
				continue
			}
			if err := allocateSchemaSimpleTypeNodeID(member.anonymous, source, nextBySource, seen); err != nil {
				return err
			}
		}
		return nil
	default:
		return newSchemaBridgeInvariant(input.loc, "simple type node allocation has an unknown model")
	}
}

func cloneSchemaElementParticleInputs(inputs []schemaElementParticleInput) []schemaElementParticleInput {
	if len(inputs) == 0 {
		return nil
	}
	clones := make([]schemaElementParticleInput, len(inputs))
	for index, input := range inputs {
		clones[index] = input
		clones[index].occurrences = input.occurrences.clone()
		if input.typeInput == nil {
			continue
		}
		clones[index].typeInput = cloneSchemaElementInput(input.typeInput)
	}
	return clones
}
