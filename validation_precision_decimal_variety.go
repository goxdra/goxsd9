package goxsd9

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// InvalidInstanceListCode identifies a list item that fails its datatype.
	InvalidInstanceListCode = "XSD4008"
	// InvalidInstanceUnionCode identifies a value rejected by every union member.
	InvalidInstanceUnionCode = "XSD4009"
	instanceListSpecRef      = "xsd11-datatypes#list"
	instanceUnionSpecRef     = "xsd11-datatypes#union"
)

var (
	errInstanceListItem   = errors.New("list item is invalid")
	errInstanceUnionValue = errors.New("all union members rejected the value")
)

// An instanceVarietyPlan is built from completed schema facts only when a
// bounded list or union is used by the instance validator.
type instanceVarietyPlan struct {
	variety SimpleTypeVariety
	kind    schemaSimpleTypeAtomicKind
	scalar  instanceScalarType
	item    *instanceVarietyPlan
	members []instanceVarietyPlan
	related []Loc
	version XSDVersion
}

// The selected member and parsed atomic value remain together. In particular,
// a union string value is never coerced to a numeric value, and list items are
// never joined into one atomic value.
type instanceVarietyValue struct {
	kind         schemaSimpleTypeAtomicKind
	atomic       any
	items        []instanceVarietyValue
	activeMember int
	selected     *instanceVarietyValue
}

//nolint:gocognit // The three varieties are completed in schema order.
func instanceVarietyPlanFor(schema Schema, reference SimpleTypeReference, related []Loc, loc Loc, version XSDVersion) (instanceVarietyPlan, error) {
	if reference.facts == nil {
		return instanceVarietyPlan{}, newInstanceValidationInternal(loc, "simple type reference has no facts", related, errInstanceValidationInvariant)
	}
	related = appendInstanceRelated(relCopy(related), reference.Loc())
	definition, err := instanceVarietyDefinition(schema, reference, loc, related)
	if err != nil {
		return instanceVarietyPlan{}, err
	}
	plan := instanceVarietyPlan{variety: reference.Variety(), kind: reference.facts.atomicKind, related: related, version: version}
	if definition != nil {
		plan.related = appendInstanceRelated(plan.related, definition.loc)
		plan.related = appendInstanceRelated(plan.related, definition.varietyLoc)
	}
	switch reference.Variety() {
	case SimpleTypeVarietyAtomicRestriction:
		plan.scalar, err = instanceVarietyAtomicScalar(reference, plan.related, loc, version)
		return plan, err
	case SimpleTypeVarietyList:
		if definition == nil || !definition.hasItemType {
			return instanceVarietyPlan{}, newInstanceValidationInternal(loc, "list type lost its item reference", plan.related, errInstanceValidationInvariant)
		}
		item := SimpleTypeReference{facts: &definition.itemType}
		if item.Variety() != SimpleTypeVarietyAtomicRestriction || item.facts.atomicKind != schemaSimpleTypeAtomicPrecisionDecimal {
			return instanceVarietyPlan{}, newInstanceValidationUnsupported(loc, "list item type is outside bounded precisionDecimal validation", plan.related, version, errInstanceUnsupportedType)
		}
		resolved, itemErr := instanceVarietyPlanFor(schema, item, plan.related, loc, version)
		if itemErr != nil {
			return instanceVarietyPlan{}, itemErr
		}
		plan.item = &resolved
		return plan, nil
	case SimpleTypeVarietyUnion:
		if definition == nil || len(definition.memberTypes) == 0 {
			return instanceVarietyPlan{}, newInstanceValidationInternal(loc, "union type lost its members", plan.related, errInstanceValidationInvariant)
		}
		plan.members = make([]instanceVarietyPlan, 0, len(definition.memberTypes))
		for index := range definition.memberTypes {
			member := SimpleTypeReference{facts: &definition.memberTypes[index]}
			if member.Variety() != SimpleTypeVarietyAtomicRestriction || !instanceBoundedUnionAtomicKind(member.facts.atomicKind) {
				return instanceVarietyPlan{}, newInstanceValidationUnsupported(loc, "union member type is outside bounded precisionDecimal validation", appendInstanceRelated(relCopy(plan.related), member.Loc()), version, errInstanceUnsupportedType)
			}
			resolved, memberErr := instanceVarietyPlanFor(schema, member, plan.related, loc, version)
			if memberErr != nil {
				return instanceVarietyPlan{}, memberErr
			}
			plan.members = append(plan.members, resolved)
		}
		return plan, nil
	default:
		return instanceVarietyPlan{}, newInstanceValidationUnsupported(loc, "simple type variety is outside instance validation", plan.related, version, errInstanceUnsupportedType)
	}
}

func instanceBoundedUnionAtomicKind(kind schemaSimpleTypeAtomicKind) bool {
	//nolint:exhaustive // The bounded packet rejects all other atomic kinds.
	switch kind {
	case schemaSimpleTypeAtomicPrecisionDecimal, schemaSimpleTypeAtomicInteger, schemaSimpleTypeAtomicNegativeInteger, schemaSimpleTypeAtomicString:
		return true
	default:
		return false
	}
}

func instanceVarietyDefinition(schema Schema, reference SimpleTypeReference, loc Loc, related []Loc) (*schemaSimpleTypeComponent, error) {
	if reference.IsAnonymous() {
		return reference.facts.anonymous, nil
	}
	if !reference.IsNamed() {
		return nil, nil
	}
	id, ok := reference.ComponentID()
	if !ok {
		return nil, newInstanceValidationInternal(loc, "named simple type has no identity", related, errInstanceValidationInvariant)
	}
	component, found := schema.Lookup(id)
	if !found {
		return nil, newInstanceValidationInternal(loc, "named simple type is absent from the completed schema", related, errInstanceValidationInvariant)
	}
	definition, simple := component.SimpleTypeDefinition()
	if !simple || definition.facts == nil {
		return nil, newInstanceValidationInternal(loc, "named simple type has no completed facts", related, errInstanceValidationInvariant)
	}
	if len(definition.Final()) != 0 {
		return nil, newInstanceValidationUnsupported(loc, "simple type final controls are outside instance validation", appendInstanceRelated(relCopy(related), definition.FinalLoc()), instanceSchemaValidationVersion(schema), errInstanceUnsupportedType)
	}
	return definition.facts, nil
}

func instanceVarietyAtomicScalar(reference SimpleTypeReference, related []Loc, loc Loc, version XSDVersion) (instanceScalarType, error) {
	plan := instanceScalarType{version: version, related: related}
	switch facets := reference.facts.facets.(type) {
	case schemaPrecisionDecimalFacetVariant:
		if reference.facts.atomicKind == schemaSimpleTypeAtomicPrecisionDecimal {
			plan.value = instancePrecisionDecimalScalar{facets: facets.value}
			return plan, nil
		}
	case schemaDigitFacetVariant:
		if reference.facts.atomicKind == schemaSimpleTypeAtomicInteger || reference.facts.atomicKind == schemaSimpleTypeAtomicNegativeInteger {
			plan.value = instanceDigitScalar{facets: facets.value, integerKind: reference.facts.atomicKind, integerBounds: facets.integerBounds}
			return plan, nil
		}
	case schemaIntegerFacetVariant:
		if reference.facts.atomicKind == schemaSimpleTypeAtomicInteger || reference.facts.atomicKind == schemaSimpleTypeAtomicNegativeInteger {
			plan.value = instanceDigitScalar{facets: facets.digits, integerKind: reference.facts.atomicKind, integerBounds: facets.bounds}
			if facets.enumeration.HasEnumeration() {
				plan.enumeration = instanceIntegerEnumerationFacet{facets: facets.enumeration}
			}
			return plan, nil
		}
	case schemaStringFacetVariant:
		if reference.facts.atomicKind == schemaSimpleTypeAtomicString && facets.whiteSpace != nil {
			plan.value = instanceStringScalar{enumeration: facets.enumeration, whiteSpace: *facets.whiteSpace}
			return plan, nil
		}
	}
	return instanceScalarType{}, newInstanceValidationUnsupported(loc, "atomic union or list member is outside bounded validation", related, version, errInstanceUnsupportedType)
}

//nolint:gocognit,funlen // Each successful branch returns a complete typed value.
func validateInstanceVarietyValue(name syntaxName, lexical string, valueLoc Loc, plan instanceVarietyPlan) (instanceVarietyValue, error) {
	switch plan.variety {
	case SimpleTypeVarietyAtomicRestriction:
		if err := validateScalarLexicalValue(name, lexical, valueLoc, plan.scalar); err != nil {
			return instanceVarietyValue{}, err
		}
		value := instanceVarietyValue{kind: plan.kind}
		//nolint:exhaustive // Only the bounded atomic kinds can enter the plan.
		switch plan.kind {
		case schemaSimpleTypeAtomicPrecisionDecimal:
			parsed, err := parsePrecisionDecimal(collapseXMLWhitespace(lexical), valueLoc)
			if err != nil {
				return instanceVarietyValue{}, newInstanceValidationInternal(valueLoc, "validated precisionDecimal could not be parsed", plan.related, err)
			}
			value.atomic = parsed
		case schemaSimpleTypeAtomicInteger, schemaSimpleTypeAtomicNegativeInteger:
			parsed, err := ParseStrictInteger(lexical, valueLoc)
			if err != nil {
				return instanceVarietyValue{}, newInstanceValidationInternal(valueLoc, "validated integer could not be parsed", plan.related, err)
			}
			value.atomic = parsed
		case schemaSimpleTypeAtomicString:
			stringScalar, ok := plan.scalar.value.(instanceStringScalar)
			if !ok {
				return instanceVarietyValue{}, newInstanceValidationInternal(valueLoc, "string member has no string scalar facts", plan.related, errInstanceValidationInvariant)
			}
			space := stringScalar.whiteSpace.Value()
			value.atomic = lexical
			if space == "replace" {
				value.atomic = replaceXMLWhitespace(lexical)
			}
			if space == "collapse" {
				value.atomic = collapseXMLWhitespace(lexical)
			}
		default:
			return instanceVarietyValue{}, newInstanceValidationInternal(valueLoc, "validated atomic kind is unknown", plan.related, errInstanceValidationInvariant)
		}
		return value, nil
	case SimpleTypeVarietyList:
		if plan.item == nil {
			return instanceVarietyValue{}, newInstanceValidationInternal(valueLoc, "list plan has no item", plan.related, errInstanceValidationInvariant)
		}
		collapsed := collapseXMLWhitespace(lexical)
		value := instanceVarietyValue{items: make([]instanceVarietyValue, 0)}
		if collapsed == "" {
			return value, nil
		}
		for index, item := range strings.Split(collapsed, " ") {
			itemValue, err := validateInstanceVarietyValue(name, item, valueLoc, *plan.item)
			if err != nil {
				var diagnostic Diagnostic
				if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureInvalid {
					return instanceVarietyValue{}, err
				}
				return instanceVarietyValue{}, newInstanceValidationInvalid(InvalidInstanceListCode, valueLoc, fmt.Sprintf("list item %d is invalid", index+1), mergeInstanceRelated(plan.related, diagnostic.Related()), instanceListSpecRef, errors.Join(errInstanceListItem, err))
			}
			value.items = append(value.items, itemValue)
		}
		return value, nil
	case SimpleTypeVarietyUnion:
		var rejected []error
		related := relCopy(plan.related)
		for index, member := range plan.members {
			memberValue, err := validateInstanceVarietyValue(name, lexical, valueLoc, member)
			if err == nil {
				return instanceVarietyValue{activeMember: index, selected: &memberValue}, nil
			}
			var diagnostic Diagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureInvalid {
				return instanceVarietyValue{}, err
			}
			rejected = append(rejected, err)
			for _, memberLoc := range member.related {
				related = appendInstanceRelated(related, memberLoc)
			}
		}
		return instanceVarietyValue{}, newInstanceValidationInvalid(InvalidInstanceUnionCode, valueLoc, "all union members rejected the value", related, instanceUnionSpecRef, errors.Join(append([]error{errInstanceUnionValue}, rejected...)...))
	default:
		return instanceVarietyValue{}, newInstanceValidationInternal(valueLoc, "unknown simple type variety", plan.related, errInstanceValidationInvariant)
	}
}
