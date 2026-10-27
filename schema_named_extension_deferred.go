package goxsd9

import "errors"

// The excluded body retains its authored references as phase input. It never
// becomes a completed complex-type body or a public component.
func schemaNamedNestedExtensionDeferredInput(
	extension, model *syntaxElement,
	facts schemaDocumentFacts,
	version XSDVersion,
	block schemaBlockPolicy,
	base QName,
	baseLoc Loc,
) (*schemaComplexTypeInput, error) {
	err := validateComplexDerivation(extension, version, true, false, true, false, false)
	if err == nil {
		return nil, nil
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax {
		return nil, err
	}
	references := make([]schemaNestedExtensionReferenceInput, 0)
	if collectErr := collectSchemaNestedExtensionReferences(model, facts, version, &references); collectErr != nil {
		return nil, collectErr
	}
	attributes, err := schemaAttributeUseInputsFromChildren(extension, facts, version, false)
	if err != nil {
		return nil, err
	}
	return &schemaComplexTypeInput{
		body: &schemaComplexTypeDeferredNestedBodyInput{
			base: schemaComplexTypeReferenceInput{
				kind: schemaComplexTypeQNameReferenceInput,
				name: base,
				loc:  baseLoc,
			},
			references: references,
			attributes: attributes,
			diagnostic: diagnostic,
		},
		prohibitedSubstitutions: block,
	}, nil
}

func collectSchemaNestedExtensionReferences( //nolint:gocognit // Visit only the bounded nested particle grammar in authored order.
	parent *syntaxElement,
	facts schemaDocumentFacts,
	version XSDVersion,
	references *[]schemaNestedExtensionReferenceInput,
) error {
	for _, node := range parent.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI {
			continue
		}
		switch child.name.local {
		case "element":
			if len(syntaxAttributesByLocal(child, "ref")) == 0 && len(syntaxAttributesByLocal(child, "type")) == 0 && inlineSimpleTypeChild(child) == nil {
				continue
			}
			element, err := schemaElementParticleInputFromElementWithFacts(child, facts, version, true, false)
			if err != nil {
				return err
			}
			*references = append(*references, schemaNestedExtensionReferenceInput{element: &element})
		case "group":
			occurrences, err := schemaParticleOccurrenceRange(child, version)
			if err != nil {
				return err
			}
			group, err := schemaModelGroupReferenceParticleInputFromElementWithFacts(child, occurrences, facts)
			if err != nil {
				return err
			}
			*references = append(*references, schemaNestedExtensionReferenceInput{group: group})
		case "choice", "sequence":
			if err := collectSchemaNestedExtensionReferences(child, facts, version, references); err != nil {
				return err
			}
		}
	}
	return nil
}

func (resolver *schemaComplexTypeResolver) resolveDeferredNestedBody( //nolint:gocognit // Preserve reference, group-cycle, attribute, then base precedence.
	body *schemaComplexTypeDeferredNestedBodyInput,
	owner schemaComponentRecord,
	ownerIndex int,
) (schemaComplexTypeBodyResult, error) {
	if body == nil {
		return nil, newSchemaBridgeInvariant(owner.loc, "deferred nested extension body is nil")
	}
	groupTargets := make([]int, 0)
	groupLocs := make([]Loc, 0)
	for _, reference := range body.references {
		if reference.element != nil {
			if reference.element.reference != nil {
				if _, err := resolveSchemaElementReferenceParticle(*reference.element, owner, resolver.records, resolver.byName, resolver.visibleSources, resolver.version); err != nil {
					return nil, err
				}
				continue
			}
			_, err := resolveSchemaElementParticle(*reference.element, owner, resolver.records, resolver.byName, resolver.visibleSources, resolver.simpleTypes, resolver.version, "sequence")
			if err != nil {
				var diagnostic Diagnostic
				if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
					return nil, err
				}
			}
			continue
		}
		if reference.group == nil {
			return nil, newSchemaBridgeInvariant(owner.loc, "deferred nested extension reference is empty")
		}
		target, err := schemaModelGroupReferenceTarget(reference.group, owner, resolver.records, resolver.byName, resolver.visibleSources, resolver.version)
		if err != nil {
			return nil, err
		}
		groupTargets = append(groupTargets, target)
		groupLocs = append(groupLocs, reference.group.reference.loc)
	}
	groupResolver := schemaModelGroupResolver{
		records: resolver.records, byName: resolver.byName, visibleSources: resolver.visibleSources,
		simpleTypes: resolver.simpleTypes, version: resolver.version,
		results: make([]schemaModelGroupResult, len(resolver.records)),
		states:  make([]uint8, len(resolver.records)),
	}
	for index, target := range groupTargets {
		if err := groupResolver.visit(target, groupLocs[index]); err != nil {
			return nil, err
		}
	}
	if _, err := resolveSchemaAttributeUses(body.attributes, owner, resolver.records, resolver.byName, resolver.visibleSources, resolver.simpleTypes, resolver.attributes, resolver.version); err != nil {
		var diagnostic Diagnostic
		if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureUnsupported {
			return nil, err
		}
	}
	if _, _, err := resolver.resolveExtensionBase(body.base, ownerIndex, false, nil, nil); err != nil {
		return nil, err
	}
	return nil, body.diagnostic
}
